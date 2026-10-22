package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"junkyard/internal/client"
	"junkyard/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send [file...]",
	Short: "Upload file(s) to Junkyard",
	Long:  "Upload one or more files and get a short code to share. Multiple files are auto-zipped.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSend,
}

var (
	sendJSONFlag bool
	sendNoColor  bool
	qrFlag       bool
	quietFlag    bool
	ttlFlag      string
)

func init() {
	sendCmd.Flags().BoolVarP(&sendJSONFlag, "json", "j", false, "output as JSON")
	sendCmd.Flags().BoolVarP(&sendNoColor, "no-color", "", false, "disable color")
	sendCmd.Flags().BoolVarP(&qrFlag, "qr", "", true, "show QR code")
	sendCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "print only the code")
	sendCmd.Flags().StringVarP(&ttlFlag, "ttl", "", "1h", "time to live (max 24h)")
}

func runSend(cmd *cobra.Command, args []string) error {
	if quietFlag && sendJSONFlag {
		return fmt.Errorf("--quiet and --json cannot be used together")
	}
	var filePath string
	var fileSize int64
	var mimeType string
	if len(args) == 1 {
		info, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", args[0])
		}
		filePath = args[0]
		fileSize = info.Size()
		mimeType = "application/octet-stream"
	} else {
		tmp, err := os.CreateTemp("", "junkyard-*.zip")
		if err != nil {
			return err
		}
		filePath = tmp.Name()
		if err := tmp.Close(); err != nil {
			os.Remove(filePath)
			return err
		}
		defer os.Remove(filePath)
		if err := createZip(args, filePath); err != nil {
			return err
		}
		info, err := os.Stat(filePath)
		if err != nil {
			return err
		}
		fileSize = info.Size()
		mimeType = "application/zip"
	}

	ttl, err := parseTTL(ttlFlag)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("TTL must be greater than zero")
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	progress := func(done, total int64) error {
		if quietFlag {
			return nil
		}
		if sendJSONFlag {
			return encoder.Encode(map[string]any{
				"event": "progress",
				"bytes": done,
				"total": total,
				"pct":   percent(done, total),
			})
		}
		if sendNoColor || os.Getenv("NO_COLOR") != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\rUploading: %d%%", percent(done, total))
		}
		return nil
	}
	if sendJSONFlag {
		if err := encoder.Encode(map[string]any{"event": "upload_start", "file": filepath.Base(filePath)}); err != nil {
			return err
		}
	}

	serverURL := os.Getenv("JUNKYARD_SERVER")
	if serverURL == "" {
		serverURL = "http://localhost:45845"
	}
	c := client.NewClient(serverURL)
	resp, err := c.UploadWithProgress(cmd.Context(), filePath, int(ttl.Seconds()), progress)
	if err != nil {
		return err
	}

	if sendJSONFlag {
		if err := encoder.Encode(map[string]any{"event": "code", "value": resp.Code, "expires_in": resp.ExpiresIn}); err != nil {
			return err
		}
		return encoder.Encode(map[string]any{"event": "done"})
	}
	if quietFlag {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), resp.Code)
		return err
	}
	if sendNoColor || os.Getenv("NO_COLOR") != "" {
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintf(cmd.OutOrStdout(), "File uploaded. Code: %s\n", resp.Code)
		return nil
	}

	model := tui.NewSendModel(filepath.Base(filePath), fileSize, mimeType)
	model.SetCode(resp.Code)
	if !qrFlag {
		model.SetQR(false)
	}
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func createZip(files []string, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(zipFile)
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			zw.Close()
			zipFile.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			zw.Close()
			zipFile.Close()
			return fmt.Errorf("%s is not a regular file", file)
		}
		input, err := os.Open(file)
		if err != nil {
			zw.Close()
			zipFile.Close()
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			input.Close()
			zw.Close()
			zipFile.Close()
			return err
		}
		header.Name = filepath.Base(file)
		header.Method = zip.Deflate
		part, err := zw.CreateHeader(header)
		if err != nil {
			input.Close()
			zw.Close()
			zipFile.Close()
			return err
		}
		_, copyErr := io.Copy(part, input)
		closeErr := input.Close()
		if copyErr != nil {
			zw.Close()
			zipFile.Close()
			return copyErr
		}
		if closeErr != nil {
			zw.Close()
			zipFile.Close()
			return closeErr
		}
	}
	if err := zw.Close(); err != nil {
		zipFile.Close()
		return err
	}
	if err := zipFile.Close(); err != nil {
		return err
	}
	return nil
}

func parseTTL(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "h") || strings.HasSuffix(s, "m") || strings.HasSuffix(s, "s") {
		return time.ParseDuration(s)
	}
	seconds, err := time.ParseDuration(s + "s")
	if err != nil {
		return 0, err
	}
	return seconds, nil
}

func percent(done, total int64) int {
	if total <= 0 {
		return 100
	}
	pct := done * 100 / total
	if pct > 100 {
		return 100
	}
	return int(pct)
}
