package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"junkyard/internal/client"
	"junkyard/internal/server"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := server.NewHandlerAt(t.TempDir())
	t.Cleanup(h.Close)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestEncryptedClientTransferRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	payload := make([]byte, 2*client.ChunkSize+317)
	for i := range payload {
		payload[i] = byte((i*37 + 11) % 251)
	}
	source := filepath.Join(t.TempDir(), "private.bin")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}

	c := client.NewClient(srv.URL)
	uploaded, err := c.Upload(context.Background(), source, 600)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	encryptedResp, err := http.Get(srv.URL + "/download/" + uploaded.Code)
	if err != nil {
		t.Fatalf("inspect stored transfer: %v", err)
	}
	if encryptedResp.Header.Get("X-Junkyard-Encrypted") != "true" {
		encryptedResp.Body.Close()
		t.Fatal("server did not mark transfer as encrypted")
	}
	stored, err := io.ReadAll(encryptedResp.Body)
	encryptedResp.Body.Close()
	if err != nil {
		t.Fatalf("read stored transfer: %v", err)
	}
	if bytes.Equal(stored, payload) {
		t.Fatal("server stored plaintext instead of ciphertext")
	}

	outDir := filepath.Join(t.TempDir(), "received")
	receivedPath, err := c.Download(context.Background(), uploaded.Code, outDir)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	got, err := os.ReadFile(receivedPath)
	if err != nil {
		t.Fatalf("read received file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("decrypted transfer differs from source")
	}
}

func upload(t *testing.T, srv *httptest.Server, name string, data []byte) string {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file: %v", err)
	}
	w.Close()

	resp, err := http.Post(srv.URL+"/upload", w.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}

	var out struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Code) != 6 {
		t.Fatalf("code = %q, want 6 chars", out.Code)
	}
	return out.Code
}

func decodeJSON(r io.Reader, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func TestUploadDownloadRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	want := []byte("junkyard end to end payload 0123456789")
	code := upload(t, srv, "file.zip", want)

	resp, err := http.Get(srv.URL + "/download/" + code)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("bytes mismatch:\nwant %q\ngot  %q", want, got)
	}
}

func TestResumeWithRangeRequest(t *testing.T) {
	srv := newTestServer(t)

	want := make([]byte, 256*1024)
	for i := range want {
		want[i] = byte(i % 251)
	}
	code := upload(t, srv, "file.zip", want)

	req, _ := http.NewRequest("GET", srv.URL+"/download/"+code, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", 100000))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	tail, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(want[100000:], tail) {
		t.Fatal("resumed bytes mismatch")
	}
}

func TestUnknownCodeReturns404(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/download/ZZZZZZ")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMultipleSequentialTransfers(t *testing.T) {
	srv := newTestServer(t)

	for i := 0; i < 3; i++ {
		name := filepath.Join(t.TempDir(), fmt.Sprintf("f%d.txt", i))
		want := []byte(fmt.Sprintf("payload number %d", i))
		if err := os.WriteFile(name, want, 0644); err != nil {
			t.Fatal(err)
		}
		code := upload(t, srv, filepath.Base(name), want)

		resp, err := http.Get(srv.URL + "/download/" + code)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.Equal(want, got) {
			t.Fatalf("transfer %d mismatch", i)
		}
	}
}