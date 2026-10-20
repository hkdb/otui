package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func searchStorage(t testing.TB, n int) (*SessionStorage, *SearchIndex) {
	t.Helper()
	ss, err := NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		msgs := []Message{}
		for j := 0; j < 40; j++ {
			msgs = append(msgs, Message{Role: "user", Content: fmt.Sprintf("message %d of session %d about tea and toast", j, i)})
		}
		if err := ss.Save(&Session{ID: fmt.Sprintf("s%03d", i), Name: fmt.Sprintf("Session %d", i), Messages: msgs}); err != nil {
			t.Fatal(err)
		}
	}
	return ss, NewSearchIndex(ss)
}

func TestSearchSeesChangesAndDeletes(t *testing.T) {
	ss, si := searchStorage(t, 3)
	if m, _ := si.SearchAllSessions("toast"); len(m) != 120 {
		t.Fatalf("got %d matches, want 120", len(m))
	}

	// Edited session: re-read (new mtime/size)
	s, _ := ss.Load("s001")
	s.Messages = append(s.Messages, Message{Role: "assistant", Content: "a unique zebra"})
	time.Sleep(10 * time.Millisecond)
	if err := ss.Save(s); err != nil {
		t.Fatal(err)
	}
	if m, _ := si.SearchAllSessions("zebra"); len(m) != 1 || m[0].SessionID != "s001" {
		t.Errorf("edited session not searched: %+v", m)
	}

	// Deleted session: gone
	os.Remove(filepath.Join(ss.sessionsDir, "s002.json"))
	if m, _ := si.SearchAllSessions("toast"); len(m) != 80 {
		t.Errorf("got %d matches after delete, want 80", len(m))
	}
}

func TestSearchPreviewKeepsRunesWhole(t *testing.T) {
	ss, err := NewSessionStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ss.Save(&Session{ID: "u", Name: "u", Messages: []Message{{Role: "user", Content: strings.Repeat("日本語", 50)}}})
	m, _ := NewSearchIndex(ss).SearchAllSessions("日本")
	if len(m) != 1 || !strings.HasSuffix(m[0].Preview, "...") || strings.ContainsRune(m[0].Preview, '�') {
		t.Errorf("preview = %q", m[0].Preview)
	}
}

func BenchmarkSearch300Sessions(b *testing.B) {
	_, si := searchStorage(b, 300)
	b.Run("cold", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			si.mu.Lock()
			si.cache = make(map[string]cachedSession)
			si.mu.Unlock()
			si.SearchAllSessions("toast")
		}
	})
	b.Run("cached", func(b *testing.B) {
		si.SearchAllSessions("toast")
		for i := 0; i < b.N; i++ {
			si.SearchAllSessions("toast")
		}
	})
	b.Run("cached-few-matches", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			si.SearchAllSessions("session 7 ")
		}
	})
}
