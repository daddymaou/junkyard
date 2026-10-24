package tui

import (
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
)

var (
	PanelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(1, 2)

	TitleStyle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true)

	CodeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0B0D0F")).
			Background(Accent).
			Bold(true).
			Padding(0, 1)

	ProgressStyle = progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	MetaStyle = lipgloss.NewStyle().
			Foreground(TextMuted)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success)

	FocusedBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(BorderBright)

	DimBorderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Border)
)
