package bot

import (
	"context"
	"fmt"
	"math/rand"
	"time"
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

func randomRange(rng *rand.Rand, left int, right int) int {
	if right <= left {
		return left
	}
	return left + rng.Intn(right-left+1)
}

func sleepRandomMillis(ctx context.Context, rng *rand.Rand, left int, right int) error {
	delay := time.Duration(randomRange(rng, left, right)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func startOfReport(dayNum int) time.Time {
	now := time.Now()
	todayFive := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
	return todayFive.AddDate(0, 0, -dayNum)
}
