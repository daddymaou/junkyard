package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func RenderHero(title string, content string, version string) string {
	wordmark := TitleStyle.Render("J U N K Y A R D")
	ver := lipgloss.NewStyle().Foreground(TextDim).Render(version)

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		wordmark,
		strings.Repeat(" ", 40),
		ver,
	)

	rule := lipgloss.NewStyle().
		Foreground(Border).
		Render(strings.Repeat("─", 60))

	outer := PanelStyle.
		Width(64).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			header,
			"",
			content,
			"",
			rule,
			"",
		))

	return outer
}

func RenderCodeBlock(code string) string {
	return CodeBlockStyle.Render(fmt.Sprintf(" %s ", code))
}

func RenderMeta(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		MetaStyle.Render(label),
		"  ",
		MetaStyle.Render(value),
	)
}