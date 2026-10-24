package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print version, commit hash, build date, Go version, OS, arch",
	RunE:  runVersion,
}

var verboseFlag bool

func init() {
	versionCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "verbose output")
}

func runVersion(cmd *cobra.Command, args []string) error {
	var version, commit, date string
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.time":
				date = setting.Value
			}
		}
	}
	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "unknown"
	}
	commitShort := commit
	if len(commitShort) > 8 {
		commitShort = commitShort[:8]
	}
	if date == "" {
		date = "unknown"
	}

	if verboseFlag {
		panel := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFB000")).
			Padding(1, 2).
			Width(50).
			Render(fmt.Sprintf("J U N K Y A R D\n\n%s\ncommit: %s\nbuilt: %s\nGo: %s\nOS: %s\nArch: %s",
				version, commitShort, date, runtime.Version(), runtime.GOOS, runtime.GOARCH))
		fmt.Println(panel)
	} else {
		fmt.Printf("junkyard %s (%s) %s/%s\n", version, commitShort, runtime.GOOS, runtime.GOARCH)
	}
	return nil
}
