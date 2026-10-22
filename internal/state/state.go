package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type TransferState struct {
	Code       string    `json:"code"`
	FileName   string    `json:"file_name"`
	FileSize   int64     `json:"file_size"`
	MimeType   string    `json:"mime_type"`
	BytesDone  int64     `json:"bytes_done"`
	TransferSize int64   `json:"transfer_size,omitempty"`
	Encrypted  bool      `json:"encrypted,omitempty"`
	TotalChunks int      `json:"total_chunks,omitempty"`
	ChunksDone []bool    `json:"chunks_done,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
	IsUpload   bool      `json:"is_upload"`
	OutDir     string    `json:"out_dir,omitempty"`
}

func StateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".junkyard", "tmp")
}

func StatePath(code string) string {
	return filepath.Join(StateDir(), code+".json")
}

func Save(state *TransferState) error {
	if err := os.MkdirAll(StateDir(), 0755); err != nil {
		return err
	}
	state.UpdatedAt = time.Now()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(state.Code), data, 0644)
}

func Load(code string) (*TransferState, error) {
	data, err := os.ReadFile(StatePath(code))
	if err != nil {
		return nil, err
	}
	var state TransferState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func Delete(code string) error {
	return os.Remove(StatePath(code))
}