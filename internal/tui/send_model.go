package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mdp/qrterminal/v3"
	tea "github.com/charmbracelet/bubbletea"
)

type SendModel struct {
	fileName string
	fileSize int64
	mimeType string
	code     string
	showQR   bool
	progress progress.Model
	spinner  spinner.Model
	expires  time.Time
	complete bool
	err      error
}

func NewSendModel(fileName string, fileSize int64, mimeType string) *SendModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(Accent)

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)
	p.SetSpringOptions(18.0, 1.0)

	return &SendModel{
		fileName: fileName,
		fileSize: fileSize,
		mimeType: mimeType,
		progress: p,
		spinner:  s,
		expires:  time.Now().Add(10 * time.Minute),
		showQR:  true,
	}
}

func (m *SendModel) SetCode(code string) {
	m.code = code
}

func (m *SendModel) SetQR(show bool) {
	m.showQR = show
}

func (m *SendModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.progress.Init(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{t}
	})
}

type tickMsg struct{ time.Time }

func (m *SendModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case tickMsg:
		if !m.complete {
			return m, tea.Batch(tickCmd(), m.spinner.Tick)
		}
	case progress.FrameMsg:
		newProgress, cmd := m.progress.Update(msg)
		m.progress = newProgress.(progress.Model)
		return m, cmd
	case spinner.TickMsg:
		newSpinner, cmd := m.spinner.Update(msg)
		m.spinner = newSpinner.(spinner.Model)
		return m, cmd
	}
	return m, nil
}

func (m *SendModel) View() string {
	if m.err != nil {
		return ErrorStyle.Render("Error: " + m.err.Error())
	}

	qrPanel := m.renderQRPanel()
	if !m.showQR {
		qrPanel = PanelStyle.Render("Code: " + RenderCodeBlock(m.code))
	}
	progressView := m.renderProgress()
	meta := fmt.Sprintf("%s · %s · %s", m.fileName, formatSize(m.fileSize), m.mimeType)
	expiresIn := formatDuration(time.Until(m.expires))
	expiresLine := MetaStyle.Render(fmt.Sprintf("Expires in %s", expiresIn))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		qrPanel,
		"",
		expiresLine,
		"",
		MetaStyle.Render(meta),
		"",
		progressView,
	)

	return RenderHero("", content, "v0.1.0")
}

func (m *SendModel) renderQRPanel() string {
	cfg := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    nil,
		BlackChar: qrterminal.WHITE,
		WhiteChar: qrterminal.BLACK,
		QuietZone: 1,
	}
	var b strings.Builder
	cfg.Writer = &b
	qrterminal.GenerateWithConfig(m.code, cfg)

	qrLines := strings.Split(strings.TrimSpace(b.String()), "\n")

	var panel strings.Builder
	panel.WriteString("╭────────────────────────────╮\n")
	for _, line := range qrLines {
		panel.WriteString("│ " + line + " │\n")
	}
	panel.WriteString("│                            │\n")
	panel.WriteString("│   Scan to receive          │\n")
	panel.WriteString("│                            │\n")
	panel.WriteString("│   Or enter the code:       │\n")
	panel.WriteString("│                            │\n")
	panel.WriteString("│   " + RenderCodeBlock(m.code) + "   │\n")
	panel.WriteString("╰────────────────────────────╯")

	return PanelStyle.Render(panel.String())
}

func (m *SendModel) renderProgress() string {
	pct := int(m.progress.Percent() * 100)
	bar := m.progress.View()
	return fmt.Sprintf("%s  %d%%", bar, pct)
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}