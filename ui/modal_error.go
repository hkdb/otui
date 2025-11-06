package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrorModal is a simple standalone modal for showing errors before the main UI starts
// Uses the standard OTUI borderless three-section modal pattern
type ErrorModal struct {
	title   string
	message string
	width   int
	height  int
}

func NewErrorModal(title, message string) ErrorModal {
	return ErrorModal{
		title:   title,
		message: message,
	}
}

func (m ErrorModal) Init() tea.Cmd {
	return nil
}

func (m ErrorModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m ErrorModal) View() string {
	if m.width < 20 || m.height < 10 {
		return "Terminal too small"
	}

	// Use the standard OTUI modal pattern (borderless, three sections)
	// Custom footer: "Press Enter to quit" instead of "Press Enter to acknowledge"
	modalWidth := 60
	if m.width < modalWidth+10 {
		modalWidth = m.width - 10
	}

	// Title section (no borders)
	titleSection := lipgloss.NewStyle().
		Bold(true).
		Foreground(dangerColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		Render(m.title)

	// Message section (with top border)
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center)

	for _, line := range strings.Split(m.message, "\n") {
		styledLine := messageStyle.Render(line)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border only) - Custom "Press Enter to quit"
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render("Press Enter to quit")

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
