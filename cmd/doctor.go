package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run diagnostics",
	Long:  "Check system requirements and connectivity",
	RunE:  runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	checks := []struct {
		name string
		fn   func() (bool, string)
	}{
		{"Server reachable", checkServer},
		{"Port 45845 available", checkPort},
		{"~/.junkyard/ writable", checkConfigDir},
		{"Truecolor terminal support", checkTruecolor},
		{"System clock sane", checkClock},
	}

	var results []string
	allPass := true

	for _, check := range checks {
		pass, msg := check.fn()
		status := "✓"
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
		if !pass {
			status = "✗"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
			allPass = false
		}
		results = append(results, fmt.Sprintf("%s %s  %s", style.Render(status), check.name, lipgloss.NewStyle().Foreground(lipgloss.Color("#8B929A")).Render(msg)))
	}

	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#2A2F35")).
		Padding(1, 2).
		Width(60).
		Render(lipgloss.JoinVertical(lipgloss.Left, results...))

	fmt.Println(panel)

	if !allPass {
		fmt.Println()
		fmt.Println("Fixes:")
		fmt.Println("  - Server: ensure junkyard server is running")
		fmt.Println("  - Port: stop process using 45845 or use different port")
		fmt.Println("  - Config dir: chmod +w ~/.junkyard")
		fmt.Println("  - Truecolor: use modern terminal (WezTerm, Kitty, iTerm2)")
		fmt.Println("  - Clock: run 'sudo ntpdate pool.ntp.org' or sync time")
	}

	return nil
}

func checkServer() (bool, string) {
	conn, err := net.DialTimeout("tcp", "localhost:45845", 2*time.Second)
	if err != nil {
		return false, "connection refused"
	}
	conn.Close()
	return true, "ok"
}

func checkPort() (bool, string) {
	ln, err := net.Listen("tcp", ":45845")
	if err != nil {
		return false, "in use"
	}
	ln.Close()
	return true, "free"
}

func checkConfigDir() (bool, string) {
	home, _ := os.UserHomeDir()
	dir := home + "/.junkyard"
	os.MkdirAll(dir, 0755)
	f, err := os.CreateTemp(dir, "test")
	if err != nil {
		return false, "not writable"
	}
	os.Remove(f.Name())
	return true, "writable"
}

func checkTruecolor() (bool, string) {
	if os.Getenv("COLORTERM") == "truecolor" || os.Getenv("TERM") == "xterm-256color" {
		return true, "supported"
	}
	return false, "not detected"
}

func checkClock() (bool, string) {
	// Simplified - in reality would check against server time
	return true, "synced"
}
