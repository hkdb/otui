package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"otui/config"
	"otui/storage"
)

// TokenCounter estimates token count from text
type TokenCounter struct{}

// CountTokens estimates token count using character-based estimation
// Approximation: ~4 characters per token (reasonable for most models)
func (tc *TokenCounter) CountTokens(text string) int {
	return len(text) / 4
}

// CountMessages calculates total token count for a slice of messages
func (tc *TokenCounter) CountMessages(messages []storage.Message) int {
	total := 0
	for _, msg := range messages {
		total += tc.CountTokens(msg.Content)
	}
	return total
}

// CalculateTokenUsage calculates current token usage for the current session
func (m *Model) CalculateTokenUsage() storage.TokenUsage {
	if m.CurrentSession == nil {
		return storage.TokenUsage{}
	}

	counter := &TokenCounter{}

	// Convert UI messages to storage messages for counting
	// Only count user and assistant messages (exclude system messages)
	var storageMessages []storage.Message
	for _, msg := range m.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			storageMessages = append(storageMessages, storage.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Debug: Log message counts
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] CalculateTokenUsage: total_ui_messages=%d, user_assistant_count=%d, compaction_marker=%d",
			len(m.Messages), len(storageMessages), m.CurrentSession.CompactionMarker)
	}

	// Total tokens in all messages
	totalTokens := counter.CountMessages(storageMessages)

	// Active tokens (after compaction marker)
	activeMessages := storageMessages
	compactionMarker := m.CurrentSession.CompactionMarker
	if compactionMarker > 0 && compactionMarker <= len(storageMessages) {
		activeMessages = storageMessages[compactionMarker:]
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] Applying compaction marker %d: keeping %d of %d messages",
				compactionMarker, len(activeMessages), len(storageMessages))
		}
	}
	activeTokens := counter.CountMessages(activeMessages)

	// Compacted tokens
	compactedTokens := totalTokens - activeTokens

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Token usage: total=%d active=%d compacted=%d",
			totalTokens, activeTokens, compactedTokens)
	}

	return storage.TokenUsage{
		TotalTokens:      totalTokens,
		ActiveTokens:     activeTokens,
		CompactedTokens:  compactedTokens,
		LastUpdated:      time.Now(),
		EstimationMethod: "character_based",
	}
}

// GetContextUsagePercentage returns the percentage of context window used (0.0 to 1.0)
// This is cached based on message count to avoid expensive recalculation on every render frame
func (m *Model) GetContextUsagePercentage() float64 {
	if m.CurrentSession == nil {
		return 0.0
	}

	// Count user/assistant messages for cache key
	messageCount := 0
	for _, msg := range m.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			messageCount++
		}
	}

	// Get current compaction marker
	compactionMarker := 0
	if m.CurrentSession != nil {
		compactionMarker = m.CurrentSession.CompactionMarker
	}

	// Context window is part of the cache key so the percentage updates when
	// the model changes or its real context window is fetched
	metadata := m.GetModelMetadata()
	if metadata.ContextWindow == 0 {
		return 0.0
	}

	// Return cached value if message count, compaction marker and context
	// window haven't changed
	m.usageMu.Lock()
	if m.usageCacheValid &&
		messageCount == m.cachedMessageCount &&
		compactionMarker == m.cachedCompactionMarker &&
		metadata.ContextWindow == m.cachedContextWindow {
		percentage := m.cachedUsagePercentage
		m.usageMu.Unlock()
		return percentage
	}
	m.usageMu.Unlock()

	// Recalculate
	usage := m.CalculateTokenUsage()
	percentage := float64(usage.ActiveTokens) / float64(metadata.ContextWindow)

	// Update cache
	m.usageMu.Lock()
	m.cachedUsagePercentage = percentage
	m.cachedMessageCount = messageCount
	m.cachedCompactionMarker = compactionMarker
	m.cachedContextWindow = metadata.ContextWindow
	m.usageCacheValid = true
	m.usageMu.Unlock()

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Context usage recalculated: %.1f%% (%d / %d tokens, %d messages)",
			percentage*100, usage.ActiveTokens, metadata.ContextWindow, messageCount)
	}

	return percentage
}

// The compaction marker (Session.CompactionMarker) is the number of
// user/assistant messages, in conversation order, that have been compacted.
// System messages (step messages, summaries, notices) are not counted, so
// every function here works on userAssistantMessages(m.Messages).

// userAssistantMessages returns the messages the compaction marker counts
func userAssistantMessages(msgs []Message) []Message {
	var result []Message
	for _, msg := range msgs {
		if msg.Role == "user" || msg.Role == "assistant" {
			result = append(result, msg)
		}
	}
	return result
}

// messagesAfterMarker returns the messages to send to the LLM: everything if
// nothing is compacted, otherwise the user/assistant messages after the marker
func messagesAfterMarker(msgs []Message, marker int) []Message {
	if marker <= 0 {
		return msgs
	}
	return userAssistantMessagesAfter(msgs, marker)
}

// SuggestCompactionPoint calculates how many user/assistant messages to
// compact so that about keepPercentage of the context window is kept.
// Returns 0 if everything fits within the keep target.
func (m *Model) SuggestCompactionPoint(keepPercentage float64) int {
	if m.CurrentSession == nil {
		return 0
	}

	metadata := m.GetModelMetadata()
	targetTokens := int(float64(metadata.ContextWindow) * keepPercentage)

	counter := &TokenCounter{}
	currentTokens := 0

	// Count backwards from end until we reach target
	ua := userAssistantMessages(m.Messages)
	for i := len(ua) - 1; i >= 0; i-- {
		currentTokens += counter.CountTokens(ua[i].Content)
		if currentTokens >= targetTokens {
			if config.Debug && config.DebugLog != nil {
				config.DebugLog.Printf("[compaction] Suggested marker: %d (keeps %d tokens, target was %d)",
					i, currentTokens, targetTokens)
			}
			return i
		}
	}

	return 0 // Keep all messages if we never hit the target
}

// GenerateCompactionSummary uses the LLM to summarize messagesToCompact.
// If previousSummary is set (an earlier compaction), it's included so the
// new summary covers the whole compacted history without resending it.
func GenerateCompactionSummary(provider Provider, previousSummary string, messagesToCompact []Message) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("no provider available")
	}

	// Build a prompt for the LLM to summarize
	var conversationText strings.Builder
	if previousSummary != "" {
		conversationText.WriteString("Summary of the conversation before these messages:\n")
		conversationText.WriteString(previousSummary)
		conversationText.WriteString("\n\nMessages since then:\n\n")
	}
	for _, msg := range messagesToCompact {
		conversationText.WriteString(fmt.Sprintf("%s: %s\n\n", msg.Role, msg.Content))
	}

	summaryPrompt := fmt.Sprintf(`Please provide a concise 2-3 sentence summary and a bullet point list of key points of the conversation above. Focus on the key topics discussed, main questions asked, and important decisions or conclusions reached. This summary will be used to provide context when the older messages are compacted.

Conversation to summarize:
%s

Summary:`, conversationText.String())

	// Create messages for the summary request
	summaryMessages := []Message{
		{
			Role:      "user",
			Content:   summaryPrompt,
			Timestamp: time.Now(),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var summary strings.Builder
	callback := func(chunk string, toolCalls []ToolCall) error {
		summary.WriteString(chunk)
		return nil
	}

	// Use Chat method to get summary
	err := provider.Chat(ctx, summaryMessages, callback)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	summaryText := strings.TrimSpace(summary.String())
	if summaryText == "" {
		return "", fmt.Errorf("LLM returned empty summary")
	}

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Generated summary: %s", summaryText)
	}

	return summaryText, nil
}

// compactionJob is a snapshot of everything needed to compact a session, so
// the (slow) summary can be generated off the Update loop without touching
// shared state
type compactionJob struct {
	sessionID       string
	marker          int       // New marker (user/assistant messages compacted)
	messages        []Message // Newly compacted messages: [old marker : new marker]
	previousSummary string    // LLMSummary from an earlier compaction, if any
	provider        Provider
}

// prepareCompaction validates a new marker and snapshots the messages to
// compact. Must be called from Update (reads m.Messages); does no I/O.
func (m *Model) prepareCompaction(marker int) (compactionJob, error) {
	if m.CurrentSession == nil {
		return compactionJob{}, fmt.Errorf("no session loaded")
	}

	if marker <= 0 {
		return compactionJob{}, fmt.Errorf("invalid marker index: %d", marker)
	}

	ua := userAssistantMessages(m.Messages)
	if marker > len(ua) {
		return compactionJob{}, fmt.Errorf("marker index %d is beyond message count %d", marker, len(ua))
	}

	// Don't compact if already compacted at or beyond this point
	oldMarker := m.CurrentSession.CompactionMarker
	if oldMarker >= marker {
		return compactionJob{}, fmt.Errorf("session already compacted at this level (current marker: %d, requested: %d)",
			oldMarker, marker)
	}

	// A marker beyond the messages (e.g. session edited outside OTUI) means
	// nothing before the new marker is reliably summarized - start over
	if oldMarker > len(ua) {
		oldMarker = 0
	}

	job := compactionJob{
		sessionID: m.CurrentSession.ID,
		marker:    marker,
		messages:  append([]Message(nil), ua[oldMarker:marker]...),
	}
	if oldMarker > 0 {
		job.previousSummary = m.CurrentSession.LLMSummary
	}
	_, job.provider = m.sessionProvider()

	return job, nil
}

// run generates the summary for the job. Safe to call from a Cmd goroutine:
// it only uses the snapshot.
func (job compactionJob) run() CompactionCompleteMsg {
	userCount, assistantCount := 0, 0
	for _, msg := range job.messages {
		switch msg.Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		}
	}
	compactedCount := userCount + assistantCount

	msg := CompactionCompleteMsg{
		Success:   true,
		SessionID: job.sessionID,
		Marker:    job.marker,
	}

	llmSummary, err := GenerateCompactionSummary(job.provider, job.previousSummary, job.messages)
	switch {
	case err == nil:
		// Stats-only for system marker; LLM summary goes to assistant message
		msg.Summary = fmt.Sprintf("Compacted %d messages (%d user, %d assistant)",
			compactedCount, userCount, assistantCount)
		msg.LLMSummary = llmSummary

	case job.previousSummary != "":
		// Keep the earlier summary rather than replacing it with a generic one
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] LLM summary failed, keeping previous summary: %v", err)
		}
		msg.Summary = fmt.Sprintf("Compacted %d more messages (%d user, %d assistant) on %s",
			compactedCount, userCount, assistantCount, time.Now().Format("Jan 2, 2006"))
		msg.LLMSummary = fmt.Sprintf("%s\n\n(%d more messages - %d user, %d assistant - were compacted after this summary.)",
			job.previousSummary, compactedCount, userCount, assistantCount)

	default:
		// Fallback to simple summary if LLM summary fails
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] LLM summary failed, using fallback: %v", err)
		}
		msg.Summary = fmt.Sprintf("Compacted %d messages (%d user, %d assistant) on %s",
			compactedCount, userCount, assistantCount, time.Now().Format("Jan 2, 2006"))
		msg.LLMSummary = fmt.Sprintf(
			"Earlier messages in this conversation (%d user, %d assistant) were compacted to free up context space.",
			userCount, assistantCount,
		)
	}

	return msg
}

// ErrStaleCompaction is returned by ApplyCompaction for results that no
// longer apply (session switched, or compacted further in the meantime)
var ErrStaleCompaction = fmt.Errorf("stale compaction result")

// ApplyCompaction stores a finished compaction on the current session.
// Must be called from Update.
func (m *Model) ApplyCompaction(msg CompactionCompleteMsg) error {
	if !msg.Success {
		return msg.Err
	}
	if m.CurrentSession == nil || m.CurrentSession.ID != msg.SessionID ||
		msg.Marker <= m.CurrentSession.CompactionMarker {
		return ErrStaleCompaction
	}

	m.CurrentSession.CompactionMarker = msg.Marker
	m.CurrentSession.CompactionTimestamp = time.Now()
	m.CurrentSession.CompactedSummary = msg.Summary
	m.CurrentSession.LLMSummary = msg.LLMSummary
	m.CurrentSession.TokenUsage = m.CalculateTokenUsage()

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Session compacted: marker=%d summary=%s",
			msg.Marker, msg.Summary)
	}

	// Mark session as dirty so it gets saved
	m.SessionDirty = true

	return nil
}

// CompactSession compacts the current session up to marker (user/assistant
// messages) synchronously. The UI uses CompactSessionMarkerCmd instead.
func (m *Model) CompactSession(markerIndex int) error {
	job, err := m.prepareCompaction(markerIndex)
	if err != nil {
		return err
	}
	return m.ApplyCompaction(job.run())
}

// GetActiveContext returns the user/assistant messages after the marker
func (m *Model) GetActiveContext() []Message {
	if m.CurrentSession == nil {
		return []Message{}
	}
	return userAssistantMessagesAfter(m.Messages, m.CurrentSession.CompactionMarker)
}

// userAssistantMessagesAfter returns the user/assistant messages after marker
func userAssistantMessagesAfter(msgs []Message, marker int) []Message {
	ua := userAssistantMessages(msgs)
	if marker <= 0 {
		return ua
	}
	if marker >= len(ua) {
		return []Message{}
	}
	return ua[marker:]
}

// GetCompactedContext returns the user/assistant messages before the marker
func (m *Model) GetCompactedContext() []Message {
	if m.CurrentSession == nil || m.CurrentSession.CompactionMarker <= 0 {
		return []Message{}
	}

	ua := userAssistantMessages(m.Messages)
	marker := m.CurrentSession.CompactionMarker
	if marker > len(ua) {
		marker = len(ua)
	}
	return ua[:marker]
}

// ShouldAutoCompact checks if auto-compaction should trigger
func (m *Model) ShouldAutoCompact() bool {
	if m.CurrentSession == nil {
		return false
	}

	if !m.Config.Compaction.AutoCompact {
		return false
	}

	percentage := m.GetContextUsagePercentage()
	shouldCompact := percentage >= m.Config.Compaction.AutoCompactThreshold

	if config.Debug && config.DebugLog != nil && shouldCompact {
		config.DebugLog.Printf("[compaction] Auto-compact triggered: %.1f%% >= %.1f%%",
			percentage*100, m.Config.Compaction.AutoCompactThreshold*100)
	}

	return shouldCompact
}

// ShouldShowWarning checks if context usage warning should be displayed
func (m *Model) ShouldShowWarning() bool {
	if m.CurrentSession == nil {
		return false
	}

	percentage := m.GetContextUsagePercentage()
	return percentage >= m.Config.Compaction.WarnAtPercentage
}
