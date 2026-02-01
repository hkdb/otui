package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const GitHubURL = "github.com/hkdb/otui"

func renderAboutModal(a AppView, width, height int, version, license string) string {
	var sb strings.Builder

	asciiStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true).
		Align(lipgloss.Center)

	sb.WriteString(asciiStyle.Render(" " + ASCIIArt))
	sb.WriteString("\n")

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

	sb.WriteString(featureStyle.Render(fmt.Sprintf("Press Esc or %s to close", a.formatKeyDisplay("secondary", "A"))))
	sb.WriteString("\n")

	content := sb.String()

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, boxStyle.Render(content))
}
