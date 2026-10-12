package model

import (
	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
)

// CompactSessionMarkerCmd compacts the first markerIndex user/assistant
// messages. The messages are snapshotted now (call from Update); only the LLM
// summary is generated in the background. The result is applied by
// ApplyCompaction when CompactionCompleteMsg arrives.
func (m *Model) CompactSessionMarkerCmd(markerIndex int) tea.Cmd {
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] CompactSessionMarkerCmd with marker=%d", markerIndex)
	}

	job, err := m.prepareCompaction(markerIndex)
	if err != nil {
		return func() tea.Msg {
			return CompactionCompleteMsg{Success: false, Err: err}
		}
	}

	return func() tea.Msg {
		return job.run()
	}
}

// CompactSessionCmd compacts enough messages to keep about keepPercentage of
// the context window. Returns nil if there's nothing new to compact.
// Call from Update (see CompactSessionMarkerCmd).
func (m *Model) CompactSessionCmd(keepPercentage float64) tea.Cmd {
	if m.CurrentSession == nil {
		return nil
	}

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] CompactSessionCmd with keep=%.1f%%", keepPercentage*100)
	}

	// Calculate marker index
	markerIndex := m.SuggestCompactionPoint(keepPercentage)

	// Fallback: if marker is 0, use half of messages
	if markerIndex == 0 {
		messageCount := len(userAssistantMessages(m.Messages))
		markerIndex = messageCount / 2
		if markerIndex == 0 {
			markerIndex = 1
		}

		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] Using fallback marker: %d (half of %d messages)",
				markerIndex, messageCount)
		}
	}

	// Nothing new to compact (the kept part alone exceeds the threshold)
	if markerIndex <= m.CurrentSession.CompactionMarker {
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] Nothing new to compact (suggested %d, current marker %d)",
				markerIndex, m.CurrentSession.CompactionMarker)
		}
		return nil
	}

	return m.CompactSessionMarkerCmd(markerIndex)
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
