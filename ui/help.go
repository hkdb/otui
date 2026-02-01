package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (a AppView) renderHelpModal(width, height int) string {
	kb := a.dataModel.Config.Keybindings

	green := lipgloss.NewStyle().
		Bold(true).
		Foreground(successColor)

	title := green.Render("OTUI - Keyboard Shortcuts")

	blue := lipgloss.NewStyle().Foreground(accentColor)

	globalActions := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Global Actions"),
		fmt.Sprintf("• %-13s New chat", kb.DisplayActionKey("new_session")),
		fmt.Sprintf("• %-13s Session Manager", kb.DisplayActionKey("session_manager")),
		fmt.Sprintf("• %-13s Edit session", kb.DisplayActionKey("edit_session")),
		fmt.Sprintf("• %-13s Model selection", kb.DisplayActionKey("model_selector")),
		fmt.Sprintf("• %-13s Search session", kb.DisplayActionKey("search_messages")),
		fmt.Sprintf("• %-13s Search all", kb.DisplayActionKey("search_all_sessions")),
		fmt.Sprintf("• %-13s Plugin Manager", kb.DisplayActionKey("plugin_manager")),
		fmt.Sprintf("• %-13s Settings", kb.DisplayActionKey("settings")),
		fmt.Sprintf("• %-13s About", kb.DisplayActionKey("about")),
		fmt.Sprintf("• %-13s Toggle this help", kb.DisplayActionKey("help")),
		fmt.Sprintf("• %-13s Quit", kb.DisplayActionKey("quit")),
	)

	chatNavigation := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Chat Navigation"),
		fmt.Sprintf("• %-13s Scroll down 1 line", kb.DisplayActionKey("scroll_down")),
		fmt.Sprintf("• %-13s Scroll up 1 line", kb.DisplayActionKey("scroll_up")),
		fmt.Sprintf("• %-13s Half page down", kb.DisplayActionKey("half_page_down")),
		fmt.Sprintf("• %-13s Half page up", kb.DisplayActionKey("half_page_up")),
		fmt.Sprintf("• %-13s Full page down", kb.DisplayActionKey("page_down")),
		fmt.Sprintf("• %-13s Full page up", kb.DisplayActionKey("page_up")),
		fmt.Sprintf("• %-13s Jump to top", kb.DisplayActionKey("scroll_to_top")),
		fmt.Sprintf("• %-13s Jump to bottom", kb.DisplayActionKey("scroll_to_bottom")),
	)

	chatActions := lipgloss.JoinVertical(
		lipgloss.Left,
		blue.Render("## Chat Actions"),
		"• Enter         Send message",
		fmt.Sprintf("• %-13s Copy last response", kb.DisplayActionKey("yank_last_response")),
		fmt.Sprintf("• %-13s Copy conversation", kb.DisplayActionKey("yank_conversation")),
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
		Render(fmt.Sprintf("      Press %s or Esc to close this help", kb.DisplayActionKey("help")))

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
