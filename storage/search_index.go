package storage

import (
	"strings"
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

type SearchIndex struct {
	storage *SessionStorage
}

func NewSearchIndex(storage *SessionStorage) *SearchIndex {
	return &SearchIndex{storage: storage}
}

func (si *SearchIndex) SearchAllSessions(query string) ([]SessionMessageMatch, error) {
	if query == "" {
		return []SessionMessageMatch{}, nil
	}

	sessionList, err := si.storage.List()
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var matches []SessionMessageMatch

	for _, sessionMeta := range sessionList {
		session, err := si.storage.Load(sessionMeta.ID)
		if err != nil {
			continue
		}

		for i, msg := range session.Messages {
			if msg.Role == "system" {
				continue
			}

			if strings.Contains(strings.ToLower(msg.Content), queryLower) {
				preview := msg.Content
				if len(preview) > 100 {
					preview = preview[:100] + "..."
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
