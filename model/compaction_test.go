package model

import (
	"testing"
	"time"

	"otui/config"
	"otui/storage"
)

// TestTokenCounter tests the character-based token estimation
func TestTokenCounter(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty string", "", 0},
		{"single char", "a", 0},
		{"four chars", "abcd", 1},
		{"short text", "hello", 1},
		{"medium text", "hello world this is a test", 6},
		{"long text", "The quick brown fox jumps over the lazy dog", 10},
		{"with newlines", "line1\nline2\nline3\n", 4},
		{"with spaces", "a b c d e f g h i j k l", 5},
	}

	counter := &TokenCounter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := counter.CountTokens(tt.text)
			if result != tt.expected {
				t.Errorf("CountTokens(%q) = %d, expected %d", tt.text, result, tt.expected)
			}
		})
	}
}

// TestCountMessages tests token counting for message arrays
func TestCountMessages(t *testing.T) {
	messages := []storage.Message{
		{Role: "user", Content: "Hello there"},        // 11 chars = 2 tokens
		{Role: "assistant", Content: "Hi, how are you?"}, // 17 chars = 4 tokens
		{Role: "user", Content: "I'm doing great!"},   // 16 chars = 4 tokens
	}

	counter := &TokenCounter{}
	total := counter.CountMessages(messages)
	expected := 2 + 4 + 4 // 10 tokens
	if total != expected {
		t.Errorf("CountMessages() = %d, expected %d", total, expected)
	}
}

// TestCalculateTokenUsage tests token usage calculation
func TestCalculateTokenUsage(t *testing.T) {
	// Create test session
	session := &storage.Session{
		ID:      "test-session",
		Name:    "Test Session",
		Model:   "test-model",
		CreatedAt: time.Now(),
		Messages: []storage.Message{
			{Role: "user", Content: "Message 1 with some text here"},      // 30 chars = 7 tokens
			{Role: "assistant", Content: "Response 1 with more text"},     // 29 chars = 7 tokens
			{Role: "user", Content: "Message 2 continuing the chat"},      // 31 chars = 7 tokens
			{Role: "assistant", Content: "Response 2 with even more text"}, // 33 chars = 8 tokens
		},
		CompactionMarker: 2, // First 2 messages are compacted
	}

	// Create model with test config
	cfg := &config.Config{
		Compaction: config.CompactionConfig{
			AutoCompact:          false,
			AutoCompactThreshold: 0.75,
			KeepPercentage:       0.50,
			WarnAtPercentage:     0.85,
		},
	}

	m := &Model{
		Config:         cfg,
		CurrentSession: session,
	}

	// Populate m.Messages (CalculateTokenUsage uses this)
	for _, sMsg := range session.Messages {
		m.Messages = append(m.Messages, Message{
			Role:    sMsg.Role,
			Content: sMsg.Content,
		})
	}

	usage := m.CalculateTokenUsage()

	// Actual character-based estimation results (divide by 4, integer division)
	// The exact counts depend on the actual string lengths
	expectedTotal := 27      // Actual total tokens from all messages
	expectedActive := 14     // Actual active tokens (after marker)
	expectedCompacted := 13  // Actual compacted tokens (before marker)

	if usage.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, expected %d", usage.TotalTokens, expectedTotal)
	}
	if usage.ActiveTokens != expectedActive {
		t.Errorf("ActiveTokens = %d, expected %d", usage.ActiveTokens, expectedActive)
	}
	if usage.CompactedTokens != expectedCompacted {
		t.Errorf("CompactedTokens = %d, expected %d", usage.CompactedTokens, expectedCompacted)
	}
	if usage.EstimationMethod != "character_based" {
		t.Errorf("EstimationMethod = %q, expected 'character_based'", usage.EstimationMethod)
	}
}

// TestGetContextUsagePercentage tests percentage calculation
func TestGetContextUsagePercentage(t *testing.T) {
	session := &storage.Session{
		ID:      "test-session",
		Name:    "Test Session",
		Model:   "test-model",
		CreatedAt: time.Now(),
		Messages: []storage.Message{
			{Role: "user", Content: string(make([]byte, 4000))},    // 1000 tokens
			{Role: "assistant", Content: string(make([]byte, 4000))}, // 1000 tokens
		},
		CompactionMarker: 0, // No compaction
	}

	// Mock metadata: 10000 token context window
	// Total: 2000 tokens, so 20% usage
	cfg := &config.Config{
		Compaction: config.CompactionConfig{
			AutoCompact:          false,
			AutoCompactThreshold: 0.75,
			KeepPercentage:       0.50,
			WarnAtPercentage:     0.85,
		},
		ModelContextOverrides: map[string]int{
			"test-model": 10000,
		},
	}

	m := &Model{
		Config:         cfg,
		CurrentSession: session,
	}

	// Populate m.Messages (GetContextUsagePercentage uses this)
	for _, sMsg := range session.Messages {
		m.Messages = append(m.Messages, Message{
			Role:    sMsg.Role,
			Content: sMsg.Content,
		})
	}

	percentage := m.GetContextUsagePercentage()
	expected := 0.2 // 2000 / 10000 = 0.2 (20%)

	if percentage != expected {
		t.Errorf("GetContextUsagePercentage() = %.2f, expected %.2f", percentage, expected)
	}
}

// TestSuggestCompactionPoint tests marker calculation
func TestSuggestCompactionPoint(t *testing.T) {
	tests := []struct {
		name           string
		messages       []storage.Message
		contextWindow  int
		keepPercentage float64
		expectedMarker int
	}{
		{
			name: "compact half",
			messages: []storage.Message{
				{Role: "user", Content: string(make([]byte, 4000))},      // 1000 tokens
				{Role: "assistant", Content: string(make([]byte, 4000))}, // 1000 tokens
				{Role: "user", Content: string(make([]byte, 4000))},      // 1000 tokens
				{Role: "assistant", Content: string(make([]byte, 4000))}, // 1000 tokens
			},
			contextWindow:  10000,
			keepPercentage: 0.5,   // Keep 5000 tokens
			expectedMarker: 0,     // Keep last 2 messages (2000 tokens < 5000, so keep more)
		},
		{
			name: "compact to 25%",
			messages: []storage.Message{
				{Role: "user", Content: string(make([]byte, 4000))},      // 1000 tokens
				{Role: "assistant", Content: string(make([]byte, 4000))}, // 1000 tokens
				{Role: "user", Content: string(make([]byte, 4000))},      // 1000 tokens
				{Role: "assistant", Content: string(make([]byte, 4000))}, // 1000 tokens
			},
			contextWindow:  10000,
			keepPercentage: 0.25,  // Keep 2500 tokens
			expectedMarker: 1,     // Keep last 3 messages (counts backward until >=2500)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &storage.Session{
				ID:       "test-session",
				Name:     "Test Session",
				Model:    "test-model",
				CreatedAt:  time.Now(),
				Messages: tt.messages,
			}

			cfg := &config.Config{
				Compaction: config.CompactionConfig{
					AutoCompact:          false,
					AutoCompactThreshold: 0.75,
					KeepPercentage:       tt.keepPercentage,
					WarnAtPercentage:     0.85,
				},
				ModelContextOverrides: map[string]int{
					"test-model": tt.contextWindow,
				},
			}

			m := &Model{
				Config:         cfg,
				CurrentSession: session,
			}

			marker := m.SuggestCompactionPoint(tt.keepPercentage)
			if marker != tt.expectedMarker {
				t.Errorf("SuggestCompactionPoint() = %d, expected %d", marker, tt.expectedMarker)
			}
		})
	}
}

// TestGetActiveContext tests context filtering
func TestGetActiveContext(t *testing.T) {
	messages := []storage.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
		{Role: "assistant", Content: "Response 2"},
		{Role: "user", Content: "Message 3"},
	}

	tests := []struct {
		name           string
		compactionMarker int
		expectedCount  int
		expectedFirst  string
	}{
		{"no compaction", 0, 5, "Message 1"},
		{"compact first 2", 2, 3, "Message 2"},
		{"compact first 4", 4, 1, "Message 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &storage.Session{
				ID:               "test-session",
				Name:             "Test Session",
				Model:            "test-model",
				CreatedAt:          time.Now(),
				Messages:         messages,
				CompactionMarker: tt.compactionMarker,
			}

			m := &Model{
				CurrentSession: session,
			}

			active := m.GetActiveContext()
			if len(active) != tt.expectedCount {
				t.Errorf("GetActiveContext() returned %d messages, expected %d", len(active), tt.expectedCount)
			}
			if len(active) > 0 && active[0].Content != tt.expectedFirst {
				t.Errorf("First active message = %q, expected %q", active[0].Content, tt.expectedFirst)
			}
		})
	}
}

// TestGetCompactedContext tests compacted message retrieval
func TestGetCompactedContext(t *testing.T) {
	messages := []storage.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
		{Role: "assistant", Content: "Response 2"},
	}

	tests := []struct {
		name             string
		compactionMarker int
		expectedCount    int
	}{
		{"no compaction", 0, 0},
		{"compact first 2", 2, 2},
		{"marker at end", 4, 0}, // Marker at/beyond end means nothing compacted
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &storage.Session{
				ID:               "test-session",
				Name:             "Test Session",
				Model:            "test-model",
				CreatedAt:          time.Now(),
				Messages:         messages,
				CompactionMarker: tt.compactionMarker,
			}

			m := &Model{
				CurrentSession: session,
			}

			compacted := m.GetCompactedContext()
			if len(compacted) != tt.expectedCount {
				t.Errorf("GetCompactedContext() returned %d messages, expected %d", len(compacted), tt.expectedCount)
			}
		})
	}
}

// TestShouldAutoCompact tests auto-compaction trigger logic
func TestShouldAutoCompact(t *testing.T) {
	tests := []struct {
		name          string
		autoCompact   bool
		activeTokens  int
		contextWindow int
		threshold     float64
		expected      bool
	}{
		{"disabled", false, 8000, 10000, 0.75, false},
		{"below threshold", true, 7000, 10000, 0.75, false},
		{"at threshold", true, 7500, 10000, 0.75, true},
		{"above threshold", true, 9000, 10000, 0.75, true},
		{"no session", true, 0, 10000, 0.75, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var session *storage.Session
			if tt.activeTokens > 0 {
				// Create message with enough content to reach target tokens
				content := string(make([]byte, tt.activeTokens*4))
				session = &storage.Session{
					ID:      "test-session",
					Name:    "Test Session",
					Model:   "test-model",
					CreatedAt: time.Now(),
					Messages: []storage.Message{
						{Role: "user", Content: content},
					},
					CompactionMarker: 0,
				}
			}

			cfg := &config.Config{
				Compaction: config.CompactionConfig{
					AutoCompact:          tt.autoCompact,
					AutoCompactThreshold: tt.threshold,
					KeepPercentage:       0.50,
					WarnAtPercentage:     0.85,
				},
				ModelContextOverrides: map[string]int{
					"test-model": tt.contextWindow,
				},
			}

			m := &Model{
				Config:         cfg,
				CurrentSession: session,
			}

			// Populate m.Messages (ShouldAutoCompact -> GetContextUsagePercentage uses this)
			if session != nil {
				for _, sMsg := range session.Messages {
					m.Messages = append(m.Messages, Message{
						Role:    sMsg.Role,
						Content: sMsg.Content,
					})
				}
			}

			result := m.ShouldAutoCompact()
			if result != tt.expected {
				t.Errorf("ShouldAutoCompact() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestCompactSession tests the full compaction operation
func TestCompactSession(t *testing.T) {
	// Create enough messages with enough content to be compactable
	messages := []storage.Message{
		{Role: "user", Content: string(make([]byte, 8000))},      // 2000 tokens
		{Role: "assistant", Content: string(make([]byte, 8000))}, // 2000 tokens
		{Role: "user", Content: string(make([]byte, 8000))},      // 2000 tokens
		{Role: "assistant", Content: string(make([]byte, 8000))}, // 2000 tokens
		{Role: "user", Content: string(make([]byte, 8000))},      // 2000 tokens
		{Role: "assistant", Content: string(make([]byte, 8000))}, // 2000 tokens
	}

	session := &storage.Session{
		ID:       "test-session",
		Name:     "Test Session",
		Model:    "test-model",
		CreatedAt:  time.Now(),
		Messages: messages,
	}

	cfg := &config.Config{
		Compaction: config.CompactionConfig{
			AutoCompact:          false,
			AutoCompactThreshold: 0.75,
			KeepPercentage:       0.50,
			WarnAtPercentage:     0.85,
		},
		ModelContextOverrides: map[string]int{
			"test-model": 10000,
		},
	}

	m := &Model{
		Config:         cfg,
		CurrentSession: session,
	}

	// Copy messages to m.Messages (CompactSession uses this for validation)
	for _, sMsg := range messages {
		m.Messages = append(m.Messages, Message{
			Role:    sMsg.Role,
			Content: sMsg.Content,
		})
	}

	// Calculate marker index (keep last 50% of context)
	markerIndex := m.SuggestCompactionPoint(0.50)
	if markerIndex == 0 {
		// Fallback to half of messages
		markerIndex = len(messages) / 2
	}

	// Perform compaction
	err := m.CompactSession(markerIndex)
	if err != nil {
		t.Fatalf("CompactSession() error = %v", err)
	}

	// Verify marker was set
	if session.CompactionMarker == 0 {
		t.Error("CompactionMarker should be set after compaction")
	}

	// Verify summary was generated
	if session.CompactedSummary == "" {
		t.Error("CompactedSummary should be set after compaction")
	}

	// Verify timestamp was set
	if session.CompactionTimestamp.IsZero() {
		t.Error("CompactionTimestamp should be set after compaction")
	}

	// Verify token usage was updated
	if session.TokenUsage.ActiveTokens == 0 {
		t.Error("TokenUsage should be updated after compaction")
	}
}

// Note: TestGetModelMetadata cannot be tested here due to import cycle with provider package
// The metadata retrieval logic is tested through integration tests instead
