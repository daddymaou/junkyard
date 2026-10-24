package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ReceiveModel struct {
	input    textinput.Model
	code     string
	stage    int
	progress progress.Model
	spinner  spinner.Model
	expires  time.Time
	complete bool
	err      error
	fileName string
	fileSize int64
	mimeType string
}

func NewReceiveModel() *ReceiveModel {
	ti := textinput.New()
	ti.Placeholder = "X7K9M2"
	ti.CharLimit = 6
	ti.Width = 20
	ti.Prompt = ""
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(Accent)
	ti.Cursor.BlinkSpeed = 530 * time.Millisecond
	ti.Focus()

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)
	p.SetSpringOptions(18.0, 1.0)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(Accent)

	return &ReceiveModel{
		input:    ti,
		stage:    0,
		progress: p,
		spinner:  s,
	}
}

func (m *ReceiveModel) SetCode(code string) {
	m.code = code
	m.stage = 1
	m.expires = time.Now().Add(10 * time.Minute)
}

func (m *ReceiveModel) SetOutDir(dir string) {
	// store outDir for later use
}

func (m *ReceiveModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
		m.progress.Init(),
		tickCmd(),
	)
}

func (m *ReceiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		if m.stage == 0 {
			if msg.String() == "enter" {
				m.code = m.input.Value()
				if len(m.code) == 6 {
					m.stage = 1
					m.expires = time.Now().Add(10 * time.Minute)
					return m, tea.Batch(
						m.spinner.Tick,
						m.progress.Init(),
						tickCmd(),
					)
				}
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	case tickMsg:
		if m.stage == 1 && !m.complete {
			return m, tea.Batch(tickCmd(), m.spinner.Tick)
		}
	case progress.FrameMsg:
		newProgress, cmd := m.progress.Update(msg)
		m.progress = newProgress.(progress.Model)
		return m, cmd
	case spinner.TickMsg:
		newSpinner, cmd := m.spinner.Update(msg)
		m.spinner = newSpinner
		return m, cmd
	}
	return m, nil
}

func (m *ReceiveModel) View() string {
	if m.err != nil {
		return ErrorStyle.Render("Error: " + m.err.Error())
	}

	if m.stage == 0 {
		inputView := m.input.View()
		border := DimBorderStyle
		if m.input.Focused() {
			border = FocusedBorderStyle
		}
		inputBox := border.Render(fmt.Sprintf("╭────────────────────────────╮\n│ %-26s │\n╰────────────────────────────╯", inputView))

		content := lipgloss.JoinVertical(
			lipgloss.Center,
			"",
			inputBox,
			"",
			MetaStyle.Render("Enter the 6-character code"),
		)
		return RenderHero("", content, "v0.1.0")
	}

	progressView := m.renderProgress()
	meta := fmt.Sprintf("%s · %s · %s", m.fileName, formatSize(m.fileSize), m.mimeType)
	expiresIn := formatDuration(time.Until(m.expires))
	expiresLine := MetaStyle.Render(fmt.Sprintf("Expires in %s", expiresIn))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		MetaStyle.Render("Downloading..."),
		"",
		expiresLine,
		"",
		MetaStyle.Render(meta),
		"",
		progressView,
	)

	return RenderHero("", content, "v0.1.0")
}

func (m *ReceiveModel) renderProgress() string {
	pct := int(m.progress.Percent() * 100)
	bar := m.progress.View()
	return fmt.Sprintf("%s  %d%%", bar, pct)
}
