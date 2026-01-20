package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InstanceLockedModal is a modal shown when another OTUI instance is detected
// Allows the user to either exit or force delete the lock file
type InstanceLockedModal struct {
	runningPID  int
	width       int
	height      int
	forceDelete bool
}

func NewInstanceLockedModal(runningPID int) InstanceLockedModal {
	return InstanceLockedModal{
		runningPID:  runningPID,
		forceDelete: false,
	}
}

func (m InstanceLockedModal) Init() tea.Cmd {
	return nil
}

func (m InstanceLockedModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "d", "D":
			m.forceDelete = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// ForceDelete returns true if the user chose to force delete the lock file
func (m InstanceLockedModal) ForceDelete() bool {
	return m.forceDelete
}

func (m InstanceLockedModal) View() string {
	if m.width < 20 || m.height < 10 {
		return "Terminal too small"
	}

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
		Render("⚠️  OTUI Already Running  ⚠️")

	// Build message
	message := fmt.Sprintf(
		"Another OTUI instance is already running (PID %d).\n\n"+
			"Only one instance of OTUI can run per system.\n\n"+
			"To run multiple instances:\n"+
			"• Use system containers (Incus/Podman)\n"+
			"• Each container provides isolated environment\n\n"+
			"Close the other instance or run OTUI in a container.\n\n"+
			"If you think this is a mistake, press D to force delete\n"+
			"the lock file and open otui anyway.",
		m.runningPID)

	// Message section (with top border)
	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Align(lipgloss.Center)

	for _, line := range strings.Split(message, "\n") {
		styledLine := messageStyle.Render(line)
		messageLines = append(messageLines, styledLine)
	}

	messageLines = append(messageLines, strings.Repeat(" ", modalWidth)) // Empty line

	messageSection := lipgloss.NewStyle().
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render(strings.Join(messageLines, "\n"))

	// Footer section (with top border only)
	footerSection := lipgloss.NewStyle().
		Foreground(dimColor).
		Align(lipgloss.Center).
		Width(modalWidth).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(dimColor).
		Render("Enter Exit │ D Force delete lock file")

	// Combine sections
	sections := []string{titleSection, messageSection, footerSection}
	content := strings.Join(sections, "\n")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
