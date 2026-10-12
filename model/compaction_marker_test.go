package model

import (
	"context"
	"strings"
	"testing"

	"otui/config"
	"otui/storage"
)

// summaryProvider returns a fixed summary and records the summary prompts
type summaryProvider struct {
	metaProvider
	summary string
	prompts []string
}

func (p *summaryProvider) Chat(_ context.Context, messages []Message, callback StreamCallback) error {
	p.prompts = append(p.prompts, messages[len(messages)-1].Content)
	return callback(p.summary, nil)
}

// mixedSession returns a model whose messages interleave persistent system
// messages (step messages, summaries) with user/assistant turns
func mixedSession(provider Provider) *Model {
	m := &Model{
		Config: &config.Config{ModelContextOverrides: map[string]int{"test-model": 100000}},
		CurrentSession: &storage.Session{
			ID:       "s1",
			Model:    "test-model",
			Provider: "test",
		},
		Providers: map[string]Provider{"test": provider},
		Messages: []Message{
			{Role: "user", Content: "u1"},
			{Role: "system", Content: "step 1", Persistent: true},
			{Role: "assistant", Content: "a1"},
			{Role: "system", Content: "iteration summary", Persistent: true},
			{Role: "user", Content: "u2"},
			{Role: "assistant", Content: "a2"},
			{Role: "system", Content: "step 2", Persistent: true},
			{Role: "user", Content: "u3"},
			{Role: "assistant", Content: "a3"},
		},
	}
	return m
}

func contents(msgs []Message) string {
	var parts []string
	for _, msg := range msgs {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, ",")
}

// TestCompactionMarkerCountsUserAssistantOnly checks that compacting N
// messages hides exactly the first N user/assistant messages, with system
// messages interleaved
func TestCompactionMarkerCountsUserAssistantOnly(t *testing.T) {
	provider := &summaryProvider{summary: "summary of u1-a2"}
	m := mixedSession(provider)

	if err := m.CompactSession(4); err != nil {
		t.Fatalf("CompactSession: %v", err)
	}

	// Summarized exactly u1, a1, u2, a2 - no system messages, nothing else
	prompt := provider.prompts[0]
	for _, want := range []string{"user: u1", "assistant: a1", "user: u2", "assistant: a2"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("summary prompt missing %q", want)
		}
	}
	for _, unwanted := range []string{"u3", "a3", "step 1", "iteration summary"} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("summary prompt should not contain %q", unwanted)
		}
	}

	// Sent to the LLM: exactly what wasn't summarized
	if got := contents(messagesAfterMarker(m.Messages, m.CurrentSession.CompactionMarker)); got != "u3,a3" {
		t.Errorf("messages after marker = %q, want %q", got, "u3,a3")
	}
	if m.CurrentSession.LLMSummary != "summary of u1-a2" {
		t.Errorf("LLMSummary = %q", m.CurrentSession.LLMSummary)
	}
}

// TestCompactionWithUnsavedMessagesDoesNotPanic reproduces the crash: after
// Esc, m.Messages has turns that aren't in CurrentSession.Messages yet, and
// manual compaction compacts everything in m.Messages
func TestCompactionWithUnsavedMessagesDoesNotPanic(t *testing.T) {
	m := mixedSession(&summaryProvider{summary: "s"})
	// Saved session is stale: only the first turn
	m.CurrentSession.Messages = []storage.Message{{Role: "user", Content: "u1"}}

	marker := len(userAssistantMessages(m.Messages)) // manual compaction
	if err := m.CompactSession(marker); err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if m.CurrentSession.CompactionMarker != marker {
		t.Errorf("marker = %d, want %d", m.CurrentSession.CompactionMarker, marker)
	}
	if got := len(messagesAfterMarker(m.Messages, marker)); got != 0 {
		t.Errorf("%d messages still sent after compacting everything", got)
	}
}

func TestSuggestCompactionPointIgnoresSystemMessages(t *testing.T) {
	m := mixedSession(&summaryProvider{})
	m.Config.ModelContextOverrides["test-model"] = 4 // tiny window: keep target is 2 tokens
	for i := range m.Messages {
		m.Messages[i].Content = strings.Repeat("x", 4) // 1 token each
	}

	// Keep 2 tokens = the last 2 user/assistant messages, so compact 4.
	// Counting system messages would give a different (raw) index.
	if got := m.SuggestCompactionPoint(0.5); got != 4 {
		t.Errorf("SuggestCompactionPoint = %d, want 4 user/assistant messages", got)
	}
}

func TestIncrementalCompactionSummarizesOnlyNewMessages(t *testing.T) {
	provider := &summaryProvider{summary: "first summary"}
	m := mixedSession(provider)

	if err := m.CompactSession(2); err != nil {
		t.Fatalf("first compaction: %v", err)
	}

	provider.summary = "second summary"
	if err := m.CompactSession(4); err != nil {
		t.Fatalf("second compaction: %v", err)
	}

	prompt := provider.prompts[1]
	if !strings.Contains(prompt, "first summary") {
		t.Error("second summary prompt should include the previous summary")
	}
	if strings.Contains(prompt, "user: u1") || strings.Contains(prompt, "assistant: a1") {
		t.Error("second summary prompt should not resend already compacted messages")
	}
	if !strings.Contains(prompt, "user: u2") || !strings.Contains(prompt, "assistant: a2") {
		t.Error("second summary prompt should contain the newly compacted messages")
	}
	if m.CurrentSession.LLMSummary != "second summary" {
		t.Errorf("LLMSummary = %q", m.CurrentSession.LLMSummary)
	}
}

func TestCompactionFailureKeepsPreviousSummary(t *testing.T) {
	provider := &summaryProvider{summary: "good summary"}
	m := mixedSession(provider)
	if err := m.CompactSession(2); err != nil {
		t.Fatal(err)
	}

	provider.summary = "" // LLM returns nothing -> summary fails
	if err := m.CompactSession(4); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(m.CurrentSession.LLMSummary, "good summary") {
		t.Errorf("LLMSummary = %q, want previous summary kept", m.CurrentSession.LLMSummary)
	}
}

func TestApplyCompactionIgnoresStaleResults(t *testing.T) {
	m := mixedSession(&summaryProvider{summary: "s"})

	job, err := m.prepareCompaction(2)
	if err != nil {
		t.Fatal(err)
	}
	result := job.run()

	// Session switched while the summary was generated
	m.CurrentSession = &storage.Session{ID: "other", Model: "test-model", Provider: "test"}
	if err := m.ApplyCompaction(result); err != ErrStaleCompaction {
		t.Errorf("other session: err = %v, want ErrStaleCompaction", err)
	}
	if m.CurrentSession.CompactionMarker != 0 {
		t.Error("stale result applied to another session")
	}

	// Compacted further in the meantime
	m.CurrentSession = &storage.Session{ID: "s1", Model: "test-model", Provider: "test", CompactionMarker: 4}
	if err := m.ApplyCompaction(result); err != ErrStaleCompaction {
		t.Errorf("older marker: err = %v, want ErrStaleCompaction", err)
	}
	if m.CurrentSession.CompactionMarker != 4 {
		t.Error("stale result moved the marker backwards")
	}
}

func TestCompactSessionCmdNothingNewToCompact(t *testing.T) {
	m := mixedSession(&summaryProvider{summary: "s"})
	m.CurrentSession.CompactionMarker = 6 // everything already compacted
	if cmd := m.CompactSessionCmd(0.5); cmd != nil {
		t.Error("CompactSessionCmd should be nil when there is nothing new to compact")
	}
}
