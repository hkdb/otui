package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"otui/storage"
)

func renderMessageSearch(a AppView, searchInput textinput.Model, results []storage.MessageMatch, selectedIdx, scrollIdx, width, height int) string {
	modalWidth := width - 4
	if modalWidth > 100 {
		modalWidth = 100
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(1, 2)

	title := TitleStyle.Render("🔍 Search Current Session")
	searchView := searchInput.View()

	resultsView := ""
	if len(results) == 0 {
		if searchInput.Value() == "" {
			resultsView = DimStyle.Render("Type to search messages in current session...")
		} else {
			resultsView = DimStyle.Render("No matches found")
		}
	} else {
		// Calculate fixed overhead precisely
		// Border(2) + Padding(2) + Title(1) + Blank(1) + SearchInput(1) + Blank(1) +
		// "Found X matches:"(1) + Blank(1) + Footer(1) + Blank(1) = 12 lines
		fixedOverhead := 12

		// Reserve space for scroll indicators if needed
		scrollIndicatorSpace := 4 // "↑ X more above" (2) + "↓ X more below" (2)

		availableLines := height - fixedOverhead - scrollIndicatorSpace
		if availableLines < 3 {
			availableLines = 3 // Minimum to show at least 1 result
		}

		// Use very conservative estimate for lines per result (accounts for worst-case wrapping)
		linesPerResult := 6
		maxVisibleResults := availableLines / linesPerResult
		if maxVisibleResults < 1 {
			maxVisibleResults = 1
		}

		startIdx := scrollIdx
		endIdx := scrollIdx + maxVisibleResults
		if endIdx > len(results) {
			endIdx = len(results)
		}

		resultsView = fmt.Sprintf("Found %d matches:\n\n", len(results))

		if startIdx > 0 {
			resultsView += DimStyle.Render(fmt.Sprintf("↑ %d more above\n\n", startIdx))
		}

		for i := startIdx; i < endIdx; i++ {
			match := results[i]

			roleStyle := UserStyle
			if match.Role == "assistant" {
				roleStyle = AssistantStyle
			}

			matchText := fmt.Sprintf("%s [%s]\n  %s",
				roleStyle.Render(match.Role),
				match.Timestamp.Format("Jan 2, 3:04 PM"),
				match.Preview,
			)

			if i == selectedIdx {
				matchText = SelectedStyle.Render("> " + matchText)
			} else {
				matchText = "  " + matchText
			}

			resultsView += matchText + "\n\n"
		}

		if endIdx < len(results) {
			resultsView += DimStyle.Render(fmt.Sprintf("↓ %d more below", len(results)-endIdx))
		}
	}

	footer := FormatFooter("Type", "to search", a.formatKeyDisplay("primary", "J/K"), "Navigate", "Enter", "Select", "Esc", "Close")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchView,
		"",
		resultsView,
		"",
		footer,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		modalStyle.Width(modalWidth).Render(content))
}

func renderGlobalSearch(a AppView, searchInput textinput.Model, results []storage.SessionMessageMatch, selectedIdx, scrollIdx, width, height int) string {
	modalWidth := width - 4
	if modalWidth > 100 {
		modalWidth = 100
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dimColor).
		Padding(1, 2)

	title := TitleStyle.Render("🔍 Search All Sessions")
	searchView := searchInput.View()

	resultsView := ""
	if len(results) == 0 {
		if searchInput.Value() == "" {
			resultsView = DimStyle.Render("Type to search across all sessions...")
		} else {
			resultsView = DimStyle.Render("No matches found")
		}
	} else {
		// Calculate fixed overhead precisely
		// Border(2) + Padding(2) + Title(1) + Blank(1) + SearchInput(1) + Blank(1) +
		// "Found X matches:"(1) + Blank(1) + Footer(1) + Blank(1) = 12 lines
		fixedOverhead := 12

		// Reserve space for scroll indicators if needed
		scrollIndicatorSpace := 4 // "↑ X more above" (2) + "↓ X more below" (2)

		availableLines := height - fixedOverhead - scrollIndicatorSpace
		if availableLines < 3 {
			availableLines = 3 // Minimum to show at least 1 result
		}

		// Use very conservative estimate for lines per result (accounts for worst-case wrapping)
		linesPerResult := 6
		maxVisibleResults := availableLines / linesPerResult
		if maxVisibleResults < 1 {
			maxVisibleResults = 1
		}

		startIdx := scrollIdx
		endIdx := scrollIdx + maxVisibleResults
		if endIdx > len(results) {
			endIdx = len(results)
		}

		resultsView = fmt.Sprintf("Found %d matches:\n\n", len(results))

		if startIdx > 0 {
			resultsView += DimStyle.Render(fmt.Sprintf("↑ %d more above\n\n", startIdx))
		}

		for i := startIdx; i < endIdx; i++ {
			match := results[i]

			roleStyle := UserStyle
			if match.Role == "assistant" {
				roleStyle = AssistantStyle
			}

			matchText := fmt.Sprintf("%s [%s] %s\n  %s",
				roleStyle.Render(match.SessionName),
				match.Timestamp.Format("Jan 2, 3:04 PM"),
				DimStyle.Render(match.Role),
				match.Preview,
			)

			if i == selectedIdx {
				matchText = SelectedStyle.Render("> " + matchText)
			} else {
				matchText = "  " + matchText
			}

			resultsView += matchText + "\n\n"
		}

		if endIdx < len(results) {
			resultsView += DimStyle.Render(fmt.Sprintf("↓ %d more below", len(results)-endIdx))
		}
	}

	footer := FormatFooter("Type", "to search", a.formatKeyDisplay("primary", "J/K"), "Navigate", "Enter", "View Session", "Esc", "Close")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchView,
		"",
		resultsView,
		"",
		footer,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		modalStyle.Width(modalWidth).Render(content))
}
