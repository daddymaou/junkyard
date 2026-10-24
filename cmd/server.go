package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"junkyard/internal/server"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Junkyard file transfer server",
	Long:  "Start the HTTP server for Junkyard file transfers",
	RunE:  runServer,
}

var (
	serverPort    int
	serverStorage string
)

func init() {
	serverCmd.Flags().IntVar(&serverPort, "port", 45845, "port to listen on")
	serverCmd.Flags().StringVar(&serverStorage, "storage", "", "directory for staged files")
}

func runServer(cmd *cobra.Command, args []string) error {
	storage := serverStorage
	if storage == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		storage = filepath.Join(home, ".junkyard", "tmp")
	}
	if serverPort < 1 || serverPort > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	h := server.NewHandlerAt(storage)
	defer h.Close()
	if err := h.InitError(); err != nil {
		return err
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	address := ":" + strconv.Itoa(serverPort)
	log.Printf("Starting Junkyard server on port %d with storage %s", serverPort, storage)
	return http.ListenAndServe(address, mux)
}
