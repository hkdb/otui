package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const GitHubURL = "github.com/hkdb/otui"

func renderAboutModal(width, height int, version, license string) string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true)

	for _, line := range strings.Split(ASCIIArt, "\n") {
		sb.WriteString(titleStyle.Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")

	featureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	for _, feature := range Features {
		sb.WriteString(featureStyle.Render(feature))
		sb.WriteString("\n")
	}

	sb.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	sb.WriteString(labelStyle.Render("Version: "))
	sb.WriteString(valueStyle.Render(version))
	sb.WriteString("\n")
	sb.WriteString(labelStyle.Render("License: "))
	sb.WriteString(valueStyle.Render(license))
	sb.WriteString("\n\n")
	sb.WriteString(labelStyle.Render("GitHub: "))
	sb.WriteString(valueStyle.Render(GitHubURL))
	sb.WriteString("\n\n\n")

	sb.WriteString(featureStyle.Render("Press Esc or Alt+Shift+A to close"))
	sb.WriteString("\n")

	content := sb.String()

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, boxStyle.Render(content))
}
