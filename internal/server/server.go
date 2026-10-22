package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"junkyard/internal/code"
)

const maxTransferTTL = 24 * time.Hour

type Handler struct {
	store    map[string]*fileEntry
	chunks   map[string]*uploadAssembly
	mu       sync.RWMutex
	tmpDir   string
	initErr  error
	stop     chan struct{}
	stopOnce sync.Once
}

type fileEntry struct {
	path      string
	size      int64
	fileSize  int64
	fileName  string
	encrypted bool
	expires   time.Time
}

type uploadAssembly struct {
	chunks     map[int][]byte
	total      int
	fileName   string
	fileSize   int64
	encrypted  bool
	expires    time.Time
}

func NewHandler() *Handler {
	home, err := os.UserHomeDir()
	if err != nil {
		return newHandler("", err)
	}
	return NewHandlerAt(filepath.Join(home, ".junkyard", "tmp"))
}

func NewHandlerAt(tmpDir string) *Handler {
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return newHandler(tmpDir, err)
	}
	return newHandler(tmpDir, nil)
}

func newHandler(tmpDir string, initErr error) *Handler {
	h := &Handler{
		store:   make(map[string]*fileEntry),
		chunks:  make(map[string]*uploadAssembly),
		tmpDir:  tmpDir,
		initErr: initErr,
		stop:    make(chan struct{}),
	}
	go h.expiryRoutine()
	return h
}

func (h *Handler) Close() {
	h.stopOnce.Do(func() { close(h.stop) })
}

func (h *Handler) InitError() error {
	return h.initErr
}

func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.initErr == nil {
		return true
	}
	http.Error(w, "transfer storage is unavailable: "+h.initErr.Error(), http.StatusInternalServerError)
	return false
}

func (h *Handler) expiryRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.stop:
			return
		case now := <-ticker.C:
			h.mu.Lock()
			for transferCode, entry := range h.store {
				if !now.After(entry.expires) {
					continue
				}
				if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
					log.Printf("remove expired transfer %s: %v", transferCode, err)
				}
				delete(h.store, transferCode)
				delete(h.chunks, transferCode)
			}
			for transferCode, assembly := range h.chunks {
				if now.After(assembly.expires) {
					delete(h.chunks, transferCode)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.ready(w) {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	transferCode, err := code.Generate(6)
	if err != nil {
		http.Error(w, "failed to generate code", http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(h.tmpDir, "legacy-*")
	if err != nil {
		http.Error(w, "failed to create transfer file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		http.Error(w, "failed to save transfer file", http.StatusInternalServerError)
		return
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		http.Error(w, "failed to inspect transfer file", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		http.Error(w, "failed to finish transfer file", http.StatusInternalServerError)
		return
	}
	name := filepath.Base(filepath.Base(handler.Filename))
	h.mu.Lock()
	h.store[transferCode] = &fileEntry{
		path:     tmpPath,
		size:     info.Size(),
		fileSize: info.Size(),
		fileName: name,
		expires:  time.Now().Add(10 * time.Minute),
	}
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"code": transferCode, "expires_in": 600})
}

func (h *Handler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.ready(w) {
		return
	}
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		http.Error(w, "failed to parse chunk form", http.StatusBadRequest)
		return
	}
	transferCode := strings.ToUpper(r.FormValue("code"))
	if !code.Valid(transferCode) {
		http.Error(w, "invalid transfer code", http.StatusBadRequest)
		return
	}
	chunkIndex, err := strconv.Atoi(r.FormValue("chunk_index"))
	if err != nil || chunkIndex < 0 {
		http.Error(w, "invalid chunk index", http.StatusBadRequest)
		return
	}
	totalChunks, err := strconv.Atoi(r.FormValue("total_chunks"))
	if err != nil || totalChunks < 1 || totalChunks > 1<<16 || chunkIndex >= totalChunks {
		http.Error(w, "invalid total chunk count", http.StatusBadRequest)
		return
	}
	fileSize, err := strconv.ParseInt(r.FormValue("file_size"), 10, 64)
	if err != nil || fileSize < 0 {
		http.Error(w, "invalid original file size", http.StatusBadRequest)
		return
	}
	ttlSeconds, err := strconv.Atoi(r.FormValue("ttl"))
	if err != nil || ttlSeconds < 1 {
		http.Error(w, "invalid transfer lifetime", http.StatusBadRequest)
		return
	}
	var ttl time.Duration
	if ttlSeconds > int(maxTransferTTL/time.Second) {
		ttl = maxTransferTTL
	} else {
		ttl = time.Duration(ttlSeconds) * time.Second
	}
	fileName := strings.ReplaceAll(r.FormValue("file_name"), "\\", "/")
	fileName = filepath.Base(fileName)
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	encrypted := r.FormValue("encrypted") == "true"
	chunk, _, err := r.FormFile("chunk")
	if err != nil {
		http.Error(w, "failed to get chunk", http.StatusBadRequest)
		return
	}
	defer chunk.Close()
	data, err := io.ReadAll(io.LimitReader(chunk, (2<<20)+1))
	if err != nil {
		http.Error(w, "failed to read chunk", http.StatusInternalServerError)
		return
	}
	if len(data) > 2<<20 {
		http.Error(w, "chunk exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if existing := h.store[transferCode]; existing != nil {
		http.Error(w, "transfer code is already in use", http.StatusConflict)
		return
	}
	assembly := h.chunks[transferCode]
	if assembly == nil {
		assembly = &uploadAssembly{
			chunks:    make(map[int][]byte),
			total:     totalChunks,
			fileName:  fileName,
			fileSize:  fileSize,
			encrypted: encrypted,
			expires:   time.Now().Add(ttl),
		}
		h.chunks[transferCode] = assembly
	} else if assembly.total != totalChunks || assembly.fileName != fileName || assembly.fileSize != fileSize || assembly.encrypted != encrypted {
		http.Error(w, "chunk metadata does not match this transfer", http.StatusConflict)
		return
	}
	if old, exists := assembly.chunks[chunkIndex]; exists {
		if !bytesEqual(old, data) {
			http.Error(w, "chunk index was already uploaded with different data", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	assembly.chunks[chunkIndex] = data
	if len(assembly.chunks) != assembly.total {
		w.WriteHeader(http.StatusOK)
		return
	}

	tmp, err := os.CreateTemp(h.tmpDir, "transfer-*")
	if err != nil {
		delete(h.chunks, transferCode)
		http.Error(w, "failed to create transfer file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	var storedSize int64
	for i := 0; i < assembly.total; i++ {
		n, writeErr := tmp.Write(assembly.chunks[i])
		storedSize += int64(n)
		if writeErr != nil {
			tmp.Close()
			os.Remove(tmpPath)
			delete(h.chunks, transferCode)
			http.Error(w, "failed to assemble transfer", http.StatusInternalServerError)
			return
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		delete(h.chunks, transferCode)
		http.Error(w, "failed to finish transfer", http.StatusInternalServerError)
		return
	}
	h.store[transferCode] = &fileEntry{
		path:      tmpPath,
		size:      storedSize,
		fileSize:  assembly.fileSize,
		fileName:  assembly.fileName,
		encrypted: assembly.encrypted,
		expires:   assembly.expires,
	}
	delete(h.chunks, transferCode)
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	transferCode := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/download/"))
	h.mu.RLock()
	entry, ok := h.store[transferCode]
	h.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		http.Error(w, "code not found or expired", http.StatusNotFound)
		return
	}
	file, err := os.Open(entry.path)
	if err != nil {
		http.Error(w, "transfer file is unavailable", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("X-Junkyard-File-Name", entry.fileName)
	w.Header().Set("X-Junkyard-File-Size", strconv.FormatInt(entry.fileSize, 10))
	w.Header().Set("X-Junkyard-Encrypted", strconv.FormatBool(entry.encrypted))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", entry.fileName))
	contentType := mime.TypeByExtension(filepath.Ext(entry.fileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, entry.fileName, time.Time{}, file)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/upload", h.Upload)
	mux.HandleFunc("/upload/chunk", h.UploadChunk)
	mux.HandleFunc("/download/", h.Download)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
