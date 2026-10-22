package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"junkyard/internal/code"
	"junkyard/internal/client"

	"github.com/spf13/cobra"
)

var receiveCmd = &cobra.Command{
	Use:   "receive [code]",
	Short: "Download a file from Junkyard",
	Long:  "Enter a short code to download the uploaded file",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReceive,
}

var (
	receiveJSONFlag bool
	receiveNoColor  bool
	outFlag         string
)

func init() {
	receiveCmd.Flags().BoolVarP(&receiveJSONFlag, "json", "j", false, "output as JSON")
	receiveCmd.Flags().BoolVarP(&receiveNoColor, "no-color", "", false, "disable color")
	receiveCmd.Flags().StringVarP(&outFlag, "out", "o", ".", "output directory")
}

func runReceive(cmd *cobra.Command, args []string) error {
	transferCode := ""
	if len(args) > 0 {
		transferCode = strings.TrimSpace(args[0])
	} else if receiveJSONFlag {
		return fmt.Errorf("a transfer code is required with --json")
	} else {
		fmt.Fprint(cmd.OutOrStdout(), "Enter transfer code: ")
		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("read transfer code: %w", err)
		}
		transferCode = strings.TrimSpace(line)
	}
	transferCode = strings.ToUpper(transferCode)
	if !code.Valid(transferCode) {
		return fmt.Errorf("transfer code must be six letters or digits")
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	if receiveJSONFlag {
		if err := encoder.Encode(map[string]any{"event": "download_start", "code": strings.ToUpper(transferCode)}); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Downloading file...")
	}
	lastPercent := -1
	progress := func(done, total int64) error {
		pct := percent(done, total)
		if receiveJSONFlag {
			return encoder.Encode(map[string]any{
				"event": "progress",
				"bytes": done,
				"total": total,
				"pct":   pct,
			})
		}
		if (receiveNoColor || os.Getenv("NO_COLOR") != "") && pct == lastPercent {
			return nil
		}
		lastPercent = pct
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "\rDownloading: %d%%", pct)
		return err
	}

	serverURL := os.Getenv("JUNKYARD_SERVER")
	if serverURL == "" {
		serverURL = "http://localhost:45845"
	}
	receivedPath, err := client.NewClient(serverURL).DownloadWithProgress(
		cmd.Context(),
		strings.ToUpper(transferCode),
		outFlag,
		progress,
	)
	if err != nil {
		return err
	}
	if receiveJSONFlag {
		return encoder.Encode(map[string]any{"event": "done", "path": receivedPath})
	}
	fmt.Fprintln(cmd.OutOrStdout())
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Received: %s\n", receivedPath)
	return err
}
