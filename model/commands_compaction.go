package model

import (
	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
)

// CompactSessionMarkerCmd performs compaction with a specific marker index
func (m *Model) CompactSessionMarkerCmd(markerIndex int) tea.Cmd {
	return func() tea.Msg {
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] CompactSessionMarkerCmd started with marker=%d", markerIndex)
		}

		err := m.CompactSession(markerIndex)

		summary := ""
		llmSummary := ""
		if m.CurrentSession != nil {
			summary = m.CurrentSession.CompactedSummary
			llmSummary = m.CurrentSession.LLMSummary
		}

		return CompactionCompleteMsg{
			Success:    err == nil,
			Err:        err,
			Summary:    summary,
			LLMSummary: llmSummary,
		}
	}
}

// CompactSessionCmd performs compaction asynchronously
func (m *Model) CompactSessionCmd(keepPercentage float64) tea.Cmd {
	return func() tea.Msg {
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] CompactSessionCmd started with keep=%.1f%%",
				keepPercentage*100)
		}

		// Calculate marker index
		markerIndex := m.SuggestCompactionPoint(keepPercentage)

		// Fallback: if marker is 0, use half of messages
		if markerIndex == 0 {
			messageCount := 0
			for _, msg := range m.Messages {
				if msg.Role == "user" || msg.Role == "assistant" {
					messageCount++
				}
			}
			markerIndex = messageCount / 2
			if markerIndex == 0 {
				markerIndex = 1
			}

			if config.Debug && config.DebugLog != nil {
				config.DebugLog.Printf("[compaction] Using fallback marker: %d (half of %d messages)",
					markerIndex, messageCount)
			}
		}

		err := m.CompactSession(markerIndex)

		summary := ""
		llmSummary := ""
		if m.CurrentSession != nil {
			summary = m.CurrentSession.CompactedSummary
			llmSummary = m.CurrentSession.LLMSummary
		}

		return CompactionCompleteMsg{
			Success:    err == nil,
			Err:        err,
			Summary:    summary,
			LLMSummary: llmSummary,
		}
	}
}

// UpdateTokenUsageCmd updates token usage for current session
func (m *Model) UpdateTokenUsageCmd() tea.Cmd {
	return func() tea.Msg {
		if m.CurrentSession == nil {
			return TokenUsageUpdatedMsg{}
		}

		usage := m.CalculateTokenUsage()
		m.CurrentSession.TokenUsage = usage

		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] Token usage updated: %d active / %d total tokens",
				usage.ActiveTokens, usage.TotalTokens)
		}

		return TokenUsageUpdatedMsg{
			Usage: usage,
		}
	}
}

// CheckAutoCompactionCmd checks if auto-compaction should trigger and returns compaction command if needed
func (m *Model) CheckAutoCompactionCmd() tea.Cmd {
	if !m.ShouldAutoCompact() {
		return nil
	}

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Auto-compaction check: triggering compaction")
	}

	// Return compaction command with configured keep percentage
	return m.CompactSessionCmd(m.Config.Compaction.KeepPercentage)
}
