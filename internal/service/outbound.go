package service

import (
	"fmt"
	"sync"
	"time"

	"njk_go/internal/napcat"
)

type OutboundAction struct {
	GroupID            string
	Message            string
	Segments           []napcat.MessageSegment
	ImageURLs          []string
	ImageSegmentType   napcat.SegmentType
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

func (s *Service) RecordPending(groupID string, message string, sentAt time.Time, shouldSave bool) {
	if s == nil || s.pending == nil {
		return
	}
	s.pending.Push(pendingMessage{
		GroupID:    groupID,
		Message:    message,
		SentAt:     sentAt,
		ShouldSave: shouldSave,
	})
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

func (s *Service) systemPrompt(key commandKey) (string, error) {
	command := s.commandByKey(key)
	if command == nil {
		return "", fmt.Errorf("%s command not configured", key)
	}
	return command.Command.SystemPrompt, nil
}

func simpleOutbound(groupID string, message string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, Message: message, ShouldSave: false}
}

func imageOutbound(groupID string, imageURLs []string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeImage, ShouldSave: false}
}

func fileOutbound(groupID string, imageURLs []string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeFile, ShouldSave: false}
}

func segmentsOutbound(groupID string, segments []napcat.MessageSegment) *OutboundAction {
	return &OutboundAction{GroupID: groupID, Segments: segments, ShouldSave: false}
}

func insufficientHistory(groupID string) *OutboundAction {
	return simpleOutbound(groupID, "历史消息不足")
}

func savedReplyOutbound(groupID string, replyMessageID string, message string) *OutboundAction {
	return &OutboundAction{
		GroupID:    groupID,
		Message:    fmt.Sprintf("[CQ:reply,id=%s]%s", replyMessageID, message),
		ShouldSave: true,
	}
}
