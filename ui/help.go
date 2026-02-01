package ui

import (
	"github.com/charmbracelet/lipgloss"
)

func renderHelpModal(width, height int) string {
	green := lipgloss.NewStyle().
		Bold(true).
		Foreground(successColor)

	title := green.Render("OTUI - Keyboard Shortcuts")

	blue := lipgloss.NewStyle().Foreground(accentColor)

	globalActions := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Global Actions"),
		"• "+"Alt+N"+"        "+"New chat",
		"• "+"Alt+S"+"        "+"Session Manager",
		"• "+"Alt+E"+"        "+"Edit session",
		"• "+"Alt+M"+"        "+"Model selection",
		"• "+"Alt+P"+"        "+"Plugin Manager",
		"• "+"Alt+F"+"        "+"Search session",
		"• "+"Alt+Shift+F"+"  "+"Search all sessions",
		"• "+"Alt+Shift+S"+"  "+"Settings",
		"• "+"Alt+Shift+A"+"  "+"About",
		"• "+"Alt+H"+"        "+"Toggle this help",
		"• "+"Alt+Q"+"        "+"Quit",
	)

	chatNavigation := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Chat Navigation"),
		"• "+"Alt+J/K"+"        "+"Scroll 1 line",
		"• "+"Alt+Shift+J/K"+"  "+"Half page scroll",
		"• "+"Alt+PgUp/PgDn"+"  "+"Full page scroll",
		"• "+"Alt+G"+"          "+"Jump to top",
		"• "+"Alt+Shift+G"+"    "+"Jump to bottom",
	)

	chatActions := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Chat Actions"),
		"• "+"Enter"+"  "+"Send message",
		"• "+"Alt+Y"+"  "+"Copy last response",
		"• "+"Alt+C"+"  "+"Copy entire conversation",
	)

	tips := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Tips"),
		"• "+"Text selection works! (Mouse)",
		"• "+"Transparency Preserved",
		"• "+"Otherwise, keyboard only!",
	)

	column1 := lipgloss.JoinVertical(
		lipgloss.Left,
		globalActions,
		"",
		tips,
	)

	column2 := lipgloss.JoinVertical(
		lipgloss.Left,
		chatNavigation,
		"",
		chatActions,
	)

	columnStyle := lipgloss.NewStyle().Width(42).PaddingLeft(8)

	twoColumns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(column1),
		"    ",
		columnStyle.Render(column2),
	)

	footer := lipgloss.NewStyle().
		Foreground(dimColor).
		Render("      Press Alt+H or Esc to close this help")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		twoColumns,
		"",
		footer,
	)

	helpBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2).
		Width(100)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		helpBox.Render(content),
	)
}
