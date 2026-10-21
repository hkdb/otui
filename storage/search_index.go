package storage

import (
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionMessageMatch struct {
	SessionID    string
	SessionName  string
	MessageIndex int
	Role         string
	Content      string
	Preview      string
	Timestamp    time.Time
	Score        int
}

// SearchIndex searches the messages of all sessions. Parsed sessions are
// cached and only re-read when their file changes, so searching while typing
// doesn't re-read every session file. Safe for concurrent use.
type SearchIndex struct {
	storage *SessionStorage

	mu    sync.Mutex
	cache map[string]cachedSession // By session ID
}

type cachedSession struct {
	modTime time.Time
	size    int64
	session *Session
	lower   []string // Lowercased message contents (searched case-insensitively)
}

func NewSearchIndex(storage *SessionStorage) *SearchIndex {
	return &SearchIndex{storage: storage, cache: make(map[string]cachedSession)}
}

// previewRunes is the length of a match preview
const previewRunes = 100

func (si *SearchIndex) SearchAllSessions(query string) ([]SessionMessageMatch, error) {
	if query == "" {
		return []SessionMessageMatch{}, nil
	}

	sessions, err := si.sessions()
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var matches []SessionMessageMatch

	for _, cached := range sessions {
		session := cached.session
		for i, msg := range session.Messages {
			if msg.Role == "system" {
				continue
			}

			if strings.Contains(cached.lower[i], queryLower) {
				preview := msg.Content
				if runes := []rune(preview); len(runes) > previewRunes {
					preview = string(runes[:previewRunes]) + "..."
				}

				matches = append(matches, SessionMessageMatch{
					SessionID:    session.ID,
					SessionName:  session.Name,
					MessageIndex: i,
					Role:         msg.Role,
					Content:      msg.Content,
					Preview:      preview,
					Timestamp:    msg.Timestamp,
					Score:        0,
				})
			}
		}
	}

	return matches, nil
}

// sessions returns all sessions, newest first (like SessionStorage.List),
// reading only files that changed since the last search
func (si *SearchIndex) sessions() ([]cachedSession, error) {
	entries, err := os.ReadDir(si.storage.sessionsDir)
	if err != nil {
		return nil, err
	}

	si.mu.Lock()
	defer si.mu.Unlock()

	seen := make(map[string]bool, len(entries))
	var sessions []cachedSession
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		seen[id] = true

		cached, ok := si.cache[id]
		if !ok || !cached.modTime.Equal(info.ModTime()) || cached.size != info.Size() {
			session, err := si.storage.Load(id)
			if err != nil {
				delete(si.cache, id)
				continue // Skip corrupted files
			}
			lower := make([]string, len(session.Messages))
			for i, msg := range session.Messages {
				lower[i] = strings.ToLower(msg.Content)
			}
			cached = cachedSession{modTime: info.ModTime(), size: info.Size(), session: session, lower: lower}
			si.cache[id] = cached
		}
		sessions = append(sessions, cached)
	}

	// Forget deleted sessions
	for id := range si.cache {
		if !seen[id] {
			delete(si.cache, id)
		}
	}

	sortNewestFirst(sessions)
	return sessions, nil
}

func sortNewestFirst(sessions []cachedSession) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].session.UpdatedAt.After(sessions[j].session.UpdatedAt)
	})
}
