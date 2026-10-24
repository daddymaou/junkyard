package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{Use: "junkyard", SilenceUsage: true, SilenceErrors: true}
	rootCmd.AddCommand(sendCmd, receiveCmd, serverCmd, doctorCmd, versionCmd)
	if err := rootCmd.Execute(); err != nil {
		if sendJSONFlag || receiveJSONFlag {
			emitJSONError(os.Stderr, err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
