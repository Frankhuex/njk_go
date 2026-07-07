package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"njk_go/internal/napcat"
)

func (s *Service) systemPrompt(key commandKey) (string, error) {
	command := s.commandByKey(key)
	if command == nil {
		return "", fmt.Errorf("%s command not configured", key)
	}
	return command.Command.SystemPrompt, nil
}

func simpleOutbound(groupID string, message string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, Message: message, ShouldSave: false}
}

func imageOutbound(groupID string, imageURLs []string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeImage, ShouldSave: false}
}

func fileOutbound(groupID string, imageURLs []string) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeFile, ShouldSave: false}
}

func segmentsOutbound(groupID string, segments []napcat.MessageSegment) *pendingOutbound {
	return &pendingOutbound{GroupID: groupID, Segments: segments, ShouldSave: false}
}

func insufficientHistory(groupID string) *pendingOutbound {
	return simpleOutbound(groupID, "历史消息不足")
}

func savedReplyOutbound(groupID string, replyMessageID string, message string) *pendingOutbound {
	return &pendingOutbound{
		GroupID:    groupID,
		Message:    fmt.Sprintf("[CQ:reply,id=%s]%s", replyMessageID, message),
		ShouldSave: true,
	}
}

func containsExact(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (s *Service) Random() *rand.Rand {
	if s == nil {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rng
}

func (s *Service) DownloadImage(ctx context.Context, sourceURL string) ([]byte, error) {
	if s == nil || s.imageService == nil {
		return nil, fmt.Errorf("image service not available")
	}
	return s.imageService.download(ctx, sourceURL)
}

func startOfReport(dayNum int) time.Time {
	now := time.Now()
	todayFive := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
	return todayFive.AddDate(0, 0, -dayNum)
}
