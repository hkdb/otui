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

// GetModelMetadata returns metadata for the current session's model
// Checks user overrides first, then provider metadata, then fallbacks
func (m *Model) GetModelMetadata() ModelMetadata {
	if m.CurrentSession == nil {
		// No session - return conservative default
		return ModelMetadata{
			ContextWindow: 8192,
			MaxOutput:     2048,
			SupportsTools: false,
		}
	}

	modelName := m.CurrentSession.Model

	// Check cache first (avoid provider calls on every render)
	if modelName == m.cachedModelName && m.cachedModelMetadata.ContextWindow > 0 {
		return m.cachedModelMetadata
	}

	// 1. Check user config overrides first (highest priority)
	if m.Config.ModelContextOverrides != nil {
		if contextWindow, ok := m.Config.ModelContextOverrides[modelName]; ok {
			if config.Debug && config.DebugLog != nil {
				config.DebugLog.Printf("[compaction] Using user override for %s: %d tokens",
					modelName, contextWindow)
			}

			metadata := ModelMetadata{
				ContextWindow: contextWindow,
				MaxOutput:     8192,  // Reasonable default
				SupportsTools: true,  // Assume yes if user is overriding
			}

			// Cache the result
			m.cachedModelName = modelName
			m.cachedModelMetadata = metadata
			return metadata
		}
	}

	// 2. Try to get from provider
	if m.Provider != nil {
		ctx := context.Background()
		meta, err := m.Provider.GetModelMetadata(ctx, modelName)
		if err == nil {
			// Cache the result
			m.cachedModelName = modelName
			m.cachedModelMetadata = meta
			return meta
		}

		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] Provider metadata lookup failed for %s: %v",
				modelName, err)
		}
	}

	// 3. Ultimate fallback (conservative)
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Using fallback metadata for %s: 8192 tokens", modelName)
	}

	metadata := ModelMetadata{
		ContextWindow: 8192,
		MaxOutput:     2048,
		SupportsTools: false,
	}

	// Cache the fallback result
	m.cachedModelName = modelName
	m.cachedModelMetadata = metadata
	return metadata
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

	// Return cached value if message count AND compaction marker haven't changed
	if messageCount == m.cachedMessageCount &&
		compactionMarker == m.cachedCompactionMarker &&
		m.cachedUsagePercentage >= 0 {
		return m.cachedUsagePercentage
	}

	// Recalculate
	metadata := m.GetModelMetadata()
	if metadata.ContextWindow == 0 {
		return 0.0
	}

	usage := m.CalculateTokenUsage()
	percentage := float64(usage.ActiveTokens) / float64(metadata.ContextWindow)

	// Update cache
	m.cachedUsagePercentage = percentage
	m.cachedMessageCount = messageCount
	m.cachedCompactionMarker = compactionMarker

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Context usage recalculated: %.1f%% (%d / %d tokens, %d messages)",
			percentage*100, usage.ActiveTokens, metadata.ContextWindow, messageCount)
	}

	return percentage
}

// SuggestCompactionPoint calculates which message index to compact to
// Returns the index where the compaction marker should be placed
func (m *Model) SuggestCompactionPoint(keepPercentage float64) int {
	if m.CurrentSession == nil {
		return 0
	}

	metadata := m.GetModelMetadata()
	targetTokens := int(float64(metadata.ContextWindow) * keepPercentage)

	counter := &TokenCounter{}
	currentTokens := 0

	// Count backwards from end until we reach target
	for i := len(m.CurrentSession.Messages) - 1; i >= 0; i-- {
		currentTokens += counter.CountTokens(m.CurrentSession.Messages[i].Content)
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

// GenerateCompactionSummary uses the LLM to create a summary of compacted messages
func (m *Model) GenerateCompactionSummary(messagesToCompact []storage.Message) (string, error) {
	if m.Provider == nil {
		return "", fmt.Errorf("no provider available")
	}

	// Build a prompt for the LLM to summarize
	var conversationText strings.Builder
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
	err := m.Provider.Chat(ctx, summaryMessages, callback)
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

// CompactSession performs compaction on the current session at the specified marker
func (m *Model) CompactSession(markerIndex int) error {
	if m.CurrentSession == nil {
		return fmt.Errorf("no session loaded")
	}

	// Validate marker
	if markerIndex <= 0 {
		return fmt.Errorf("invalid marker index: %d", markerIndex)
	}

	// Get message count for validation
	messageCount := 0
	for _, msg := range m.Messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			messageCount++
		}
	}

	if markerIndex > messageCount {
		return fmt.Errorf("marker index %d is beyond message count %d", markerIndex, messageCount)
	}

	// Don't compact if already compacted at or beyond this point
	if m.CurrentSession.CompactionMarker >= markerIndex {
		return fmt.Errorf("session already compacted at this level (current marker: %d, requested: %d)",
			m.CurrentSession.CompactionMarker, markerIndex)
	}

	// Set marker
	m.CurrentSession.CompactionMarker = markerIndex
	m.CurrentSession.CompactionTimestamp = time.Now()
	m.CurrentSession.LLMSummary = "" // Clear stale value from prior compaction

	// Count messages being compacted
	compactedCount := markerIndex
	userCount := 0
	assistantCount := 0
	for i := 0; i < markerIndex; i++ {
		switch m.CurrentSession.Messages[i].Role {
		case "user":
			userCount++
		case "assistant":
			assistantCount++
		}
	}

	// Generate LLM-based summary of compacted content
	messagesToCompact := m.CurrentSession.Messages[:markerIndex]
	llmSummary, err := m.GenerateCompactionSummary(messagesToCompact)

	if err != nil {
		// Fallback to simple summary if LLM summary fails
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[compaction] LLM summary failed, using fallback: %v", err)
		}
		m.CurrentSession.CompactedSummary = fmt.Sprintf(
			"Compacted %d messages (%d user, %d assistant) on %s",
			compactedCount, userCount, assistantCount,
			time.Now().Format("Jan 2, 2006"),
		)
		m.CurrentSession.LLMSummary = fmt.Sprintf(
			"Earlier messages in this conversation (%d user, %d assistant) were compacted to free up context space.",
			userCount, assistantCount,
		)
	} else {
		// Stats-only for system marker; LLM summary goes to assistant message
		m.CurrentSession.CompactedSummary = fmt.Sprintf(
			"Compacted %d messages (%d user, %d assistant)",
			compactedCount, userCount, assistantCount,
		)
		m.CurrentSession.LLMSummary = llmSummary
	}

	// Update token usage
	m.CurrentSession.TokenUsage = m.CalculateTokenUsage()

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[compaction] Session compacted: marker=%d summary=%s",
			markerIndex, m.CurrentSession.CompactedSummary)
	}

	// Mark session as dirty so it gets saved
	m.SessionDirty = true

	return nil
}

// GetActiveContext returns messages that should be sent to LLM (after marker)
func (m *Model) GetActiveContext() []storage.Message {
	if m.CurrentSession == nil {
		return []storage.Message{}
	}

	// No compaction marker or marker is 0 - return all messages
	if m.CurrentSession.CompactionMarker == 0 {
		return m.CurrentSession.Messages
	}

	// Ensure marker is within bounds
	if m.CurrentSession.CompactionMarker >= len(m.CurrentSession.Messages) {
		// Marker is beyond message list - return all messages
		return m.CurrentSession.Messages
	}

	// Return messages after the marker
	return m.CurrentSession.Messages[m.CurrentSession.CompactionMarker:]
}

// GetCompactedContext returns messages that have been compacted (before marker)
func (m *Model) GetCompactedContext() []storage.Message {
	if m.CurrentSession == nil || m.CurrentSession.CompactionMarker == 0 {
		return []storage.Message{}
	}

	// Ensure marker is within bounds
	if m.CurrentSession.CompactionMarker >= len(m.CurrentSession.Messages) {
		// Marker is beyond message list - nothing is compacted
		return []storage.Message{}
	}

	return m.CurrentSession.Messages[:m.CurrentSession.CompactionMarker]
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
