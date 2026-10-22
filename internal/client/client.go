package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"junkyard/internal/code"
	"junkyard/internal/crypto"
	"junkyard/internal/lan"
	"junkyard/internal/state"
)

const ChunkSize = 1024 * 1024

type UploadResponse struct {
	Code        string `json:"code"`
	ExpiresIn   int    `json:"expires_in"`
	TotalChunks int    `json:"total_chunks,omitempty"`
}

type ProgressFunc func(done, total int64) error

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Upload(ctx context.Context, filePath string, ttl int) (*UploadResponse, error) {
	return c.UploadWithProgress(ctx, filePath, ttl, nil)
}

func (c *Client) UploadLAN(ctx context.Context, filePath string, ttl int, onReady func(*UploadResponse) error) error {
	if ttl < 1 {
		return fmt.Errorf("transfer lifetime must be greater than zero")
	}
	if ttl > int((24*time.Hour)/time.Second) {
		ttl = int((24 * time.Hour) / time.Second)
	}
	input, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", filePath)
	}
	transferCode, err := code.Generate(6)
	if err != nil {
		return err
	}
	encrypted, err := os.CreateTemp("", "junkyard-lan-*")
	if err != nil {
		return err
	}
	encryptedPath := encrypted.Name()
	defer os.Remove(encryptedPath)
	if err := crypto.EncryptStream(crypto.DeriveKey(transferCode, "enc"), input, encrypted); err != nil {
		encrypted.Close()
		return err
	}
	if err := encrypted.Close(); err != nil {
		return err
	}
	payload, err := os.Open(encryptedPath)
	if err != nil {
		return err
	}
	defer payload.Close()
	payloadInfo, err := payload.Stat()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp4", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()
	ip, err := lan.LocalIPv4()
	if err != nil {
		return err
	}
	address := net.JoinHostPort(ip.String(), strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	finished := make(chan struct{})
	var finishOnce sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/download/"+transferCode {
			http.NotFound(w, r)
			return
		}
		download, err := os.Open(encryptedPath)
		if err != nil {
			http.Error(w, "transfer file unavailable", http.StatusInternalServerError)
			return
		}
		defer download.Close()
		w.Header().Set("X-Junkyard-File-Name", filepath.Base(info.Name()))
		w.Header().Set("X-Junkyard-File-Size", strconv.FormatInt(info.Size(), 10))
		w.Header().Set("X-Junkyard-Encrypted", "true")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(info.Name())))
		tracked := &transferResponseWriter{ResponseWriter: w, expected: payloadInfo.Size(), done: func() {
			finishOnce.Do(func() { close(finished) })
		}}
		http.ServeContent(tracked, r, filepath.Base(info.Name()), time.Time{}, download)
	})
	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()
	defer httpServer.Close()

	resp := &UploadResponse{Code: transferCode, ExpiresIn: ttl}
	if onReady != nil {
		if err := onReady(resp); err != nil {
			return err
		}
	}
	if err := lan.SendDiscovery(transferCode, address); err != nil {
		return err
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(time.Duration(ttl) * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-finished:
			return nil
		case err := <-serverErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("LAN transfer expired before a receiver connected")
		case <-ticker.C:
			if err := lan.SendDiscovery(transferCode, address); err != nil {
				return err
			}
		}
	}
}

func (c *Client) ReceiveLAN(ctx context.Context, outDir string, onCode func(string) error, progress ProgressFunc) (string, string, error) {
	type discovered struct {
		code string
		addr string
	}
	found := make(chan discovered, 1)
	discoveryCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- lan.ListenDiscovery(discoveryCtx.Done(), func(transferCode, address string) {
			if !code.Valid(transferCode) {
				return
			}
			select {
			case found <- discovered{code: transferCode, addr: address}:
				cancel()
			default:
			}
		})
	}()
	var transfer discovered
	select {
	case transfer = <-found:
	case err := <-listenErr:
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		return "", "", err
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	if err := <-listenErr; err != nil {
		return "", "", err
	}
	if onCode != nil {
		if err := onCode(transfer.code); err != nil {
			return "", "", err
		}
	}
	url := "http://" + transfer.addr
	path, err := NewClient(url).DownloadWithProgress(ctx, transfer.code, outDir, progress)
	if err != nil {
		return transfer.code, "", err
	}
	return transfer.code, path, nil
}

type transferResponseWriter struct {
	http.ResponseWriter
	expected int64
	written  int64
	done     func()
}

func (w *transferResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.written += int64(n)
	if w.written >= w.expected {
		w.done()
	}
	return n, err
}

func (c *Client) UploadWithProgress(ctx context.Context, filePath string, ttl int, progress ProgressFunc) (*UploadResponse, error) {
	input, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filePath)
	}

	codeStr, err := code.Generate(6)
	if err != nil {
		return nil, err
	}
	key := crypto.DeriveKey(codeStr, "enc")
	encrypted, err := os.CreateTemp("", "junkyard-encrypted-*")
	if err != nil {
		return nil, err
	}
	encryptedPath := encrypted.Name()
	defer os.Remove(encryptedPath)
	if err := crypto.EncryptStream(key, input, encrypted); err != nil {
		encrypted.Close()
		return nil, err
	}
	if err := encrypted.Sync(); err != nil {
		encrypted.Close()
		return nil, err
	}
	encryptedInfo, err := encrypted.Stat()
	if err != nil {
		encrypted.Close()
		return nil, err
	}
	if err := encrypted.Close(); err != nil {
		return nil, err
	}

	payload, err := os.Open(encryptedPath)
	if err != nil {
		return nil, err
	}
	defer payload.Close()
	totalSize := encryptedInfo.Size()
	totalChunks := int((totalSize + ChunkSize - 1) / ChunkSize)
	if totalChunks == 0 {
		totalChunks = 1
	}

	buf := make([]byte, ChunkSize)
	var sent int64
	for i := 0; i < totalChunks; i++ {
		n, readErr := io.ReadFull(payload, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, readErr
		}
		if err := c.uploadChunk(ctx, codeStr, i, totalChunks, buf[:n], filepath.Base(info.Name()), info.Size(), ttl); err != nil {
			return nil, err
		}
		sent += int64(n)
		if progress != nil {
			if err := progress(sent, totalSize); err != nil {
				return nil, err
			}
		}
	}

	return &UploadResponse{Code: codeStr, ExpiresIn: ttl, TotalChunks: totalChunks}, nil
}

func (c *Client) uploadChunk(ctx context.Context, transferCode string, index, total int, data []byte, fileName string, fileSize int64, ttl int) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := map[string]string{
		"code":          transferCode,
		"chunk_index":   strconv.Itoa(index),
		"total_chunks":  strconv.Itoa(total),
		"file_name":     fileName,
		"file_size":     strconv.FormatInt(fileSize, 10),
		"ttl":           strconv.Itoa(ttl),
		"encrypted":     "true",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("chunk", fmt.Sprintf("chunk_%d", index))
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/upload/chunk", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload failed: %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func (c *Client) Download(ctx context.Context, transferCode, outDir string) (string, error) {
	return c.DownloadWithProgress(ctx, transferCode, outDir, nil)
}

func (c *Client) DownloadWithProgress(ctx context.Context, transferCode, outDir string, progress ProgressFunc) (string, error) {
	transferCode = strings.ToUpper(transferCode)
	if !code.Valid(transferCode) {
		return "", fmt.Errorf("transfer code must be six letters or digits")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	st, err := state.Load(transferCode)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if st != nil && st.IsUpload {
		st = nil
	}

	partPath := filepath.Join(state.StateDir(), transferCode+".part")
	if err := os.MkdirAll(state.StateDir(), 0755); err != nil {
		return "", err
	}
	var offset int64
	if st != nil {
		if info, statErr := os.Stat(partPath); statErr == nil {
			offset = info.Size()
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		if offset != st.BytesDone {
			st.BytesDone = offset
		}
		if st.TransferSize > 0 && offset >= st.TransferSize {
			offset = 0
		}
	}

	url := c.baseURL + "/download/" + transferCode
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("transfer code not found or expired")
	}
	if offset > 0 && resp.StatusCode == http.StatusOK {
		offset = 0
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	fileName := strings.ReplaceAll(resp.Header.Get("X-Junkyard-File-Name"), "\\", "/")
	fileName = filepath.Base(fileName)
	if fileName == "" || fileName == "." {
		fileName = "file"
	}
	encrypted := resp.Header.Get("X-Junkyard-Encrypted") == "true"
	plainSize, _ := strconv.ParseInt(resp.Header.Get("X-Junkyard-File-Size"), 10, 64)
	totalSize, err := responseTotalSize(resp)
	if err != nil {
		return "", err
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	part, err := os.OpenFile(partPath, flags, 0600)
	if err != nil {
		return "", err
	}
	st = &state.TransferState{
		Code:         transferCode,
		FileName:     fileName,
		FileSize:     plainSize,
		TransferSize: totalSize,
		BytesDone:    offset,
		Encrypted:    encrypted,
		IsUpload:     false,
	}
	if err := state.Save(st); err != nil {
		part.Close()
		return "", err
	}

	buf := make([]byte, 64*1024)
	var completed = offset
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := part.Write(buf[:n]); err != nil {
				part.Close()
				return "", err
			}
			completed += int64(n)
			st.BytesDone = completed
			if err := state.Save(st); err != nil {
				part.Close()
				return "", err
			}
			if progress != nil {
				if err := progress(completed, totalSize); err != nil {
					part.Close()
					return "", err
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			part.Close()
			return "", readErr
		}
	}
	if err := part.Close(); err != nil {
		return "", err
	}
	if totalSize >= 0 && completed != totalSize {
		return "", fmt.Errorf("download incomplete: received %d of %d bytes", completed, totalSize)
	}

	finalPath := filepath.Join(outDir, fileName)
	if _, err := os.Stat(finalPath); err == nil {
		return "", fmt.Errorf("destination already exists: %s", finalPath)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	output, err := os.OpenFile(finalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	cleanupOutput := true
	defer func() {
		output.Close()
		if cleanupOutput {
			os.Remove(finalPath)
		}
	}()

	input, err := os.Open(partPath)
	if err != nil {
		return "", err
	}
	if encrypted {
		err = crypto.DecryptStream(crypto.DeriveKey(transferCode, "enc"), input, output)
	} else {
		_, err = io.Copy(output, input)
	}
	input.Close()
	if err != nil {
		return "", fmt.Errorf("could not decrypt or write received file: %w", err)
	}
	if err := output.Sync(); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	cleanupOutput = false
	if err := os.Remove(partPath); err != nil {
		return "", err
	}
	if err := state.Delete(transferCode); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return finalPath, nil
}

func responseTotalSize(resp *http.Response) (int64, error) {
	if resp.StatusCode == http.StatusPartialContent {
		rangeHeader := resp.Header.Get("Content-Range")
		_, total, ok := strings.Cut(rangeHeader, "/")
		if !ok {
			return 0, fmt.Errorf("server returned invalid Content-Range %q", rangeHeader)
		}
		size, err := strconv.ParseInt(total, 10, 64)
		if err != nil || size < 0 {
			return 0, fmt.Errorf("server returned invalid transfer size %q", total)
		}
		return size, nil
	}
	return resp.ContentLength, nil
}

func ReadJSONLine(r io.Reader, value any) error {
	return json.NewDecoder(r).Decode(value)
}
