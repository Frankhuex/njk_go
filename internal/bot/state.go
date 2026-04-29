package bot

import (
	"sync"
	"time"
)

type pendingOutbound struct {
	GroupID            string
	Message            string
	ShouldSave         bool
	EmojiLikeMessageID string
	EmojiLikeIDs       []string
}

type pendingMessage struct {
	GroupID    string
	Message    string
	SentAt     time.Time
	ShouldSave bool
}

type pendingQueue struct {
	mu    sync.Mutex
	items []pendingMessage
}

func (q *pendingQueue) Push(item pendingMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *pendingQueue) Pop() *pendingMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	item := q.items[0]
	q.items = q.items[1:]
	return &item
}

func (s *Service) setLastAI(groupID string, at time.Time) {
	s.lastAIMu.Lock()
	defer s.lastAIMu.Unlock()
	s.lastAI[groupID] = at
}

func (s *Service) getLastAI(groupID string) (time.Time, bool) {
	s.lastAIMu.Lock()
	defer s.lastAIMu.Unlock()
	at, ok := s.lastAI[groupID]
	return at, ok
}
