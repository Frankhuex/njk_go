package bot

import (
	"context"
	"fmt"
	"strconv"

	"njk_go/internal/napcat"
)

func (s *Service) handleAIPromptCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	count, _ := strconv.Atoi(match.Groups[1])
	history, err := s.historyStrings(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}
	result, err := s.aiClient.Complete(ctx, match.Command.SystemPrompt, fmt.Sprintf("%v", history), nil)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, result), nil
}

func (s *Service) handleAICommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	count, _ := strconv.Atoi(match.Groups[1])
	history, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}
	result, err := s.aiClient.Complete(ctx, match.Command.SystemPrompt, fmt.Sprintf("%v", formatStoredMessages(history)), nil)
	if err != nil {
		return nil, err
	}
	s.setLastAI(groupID, history[0].Time)
	return savedReplyOutbound(groupID, history[0].MessageID, result), nil
}

func (s *Service) handleAICCommand(ctx context.Context, groupID string) (*pendingOutbound, error) {
	start, ok := s.getLastAI(groupID)
	if !ok {
		return simpleOutbound(groupID, "请先发起一次「.ai后接数字」"), nil
	}
	history, err := s.store.MessagesSince(ctx, groupID, start)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}
	systemPrompt, err := s.systemPrompt(commandAI)
	if err != nil {
		return nil, err
	}
	result, err := s.aiClient.Complete(ctx, systemPrompt, fmt.Sprintf("%v", formatStoredMessages(history)), nil)
	if err != nil {
		return nil, err
	}
	return savedReplyOutbound(groupID, history[0].MessageID, result), nil
}

func (s *Service) handleReportCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	dayNum, _ := strconv.Atoi(match.Groups[1])
	stats, err := s.store.ReportStats(ctx, groupID, startOfReport(dayNum), 10)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, formatReport(stats, dayNum, 10)), nil
}

func (s *Service) handleNJKReply(ctx context.Context, event *napcat.GroupMessageEvent, groupID string) (*pendingOutbound, error) {
	history, err := s.historyStrings(ctx, groupID, randomRange(s.rng, 10, 30))
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, nil
	}

	var result string
	systemPrompt, err := s.systemPrompt(commandNJK)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 5; i++ {
		temperature := 0.8 + s.rng.Float64()*0.1
		candidate, err := s.aiClient.Complete(ctx, systemPrompt, fmt.Sprintf("%v", history), &temperature)
		if err != nil {
			return nil, err
		}
		if !containsExact(history, candidate) {
			result = candidate
			break
		}
		result = candidate
	}
	if result == "" {
		return nil, nil
	}
	return &pendingOutbound{GroupID: groupID, Message: result, ShouldSave: true}, nil
}

func (s *Service) historyStrings(ctx context.Context, groupID string, count int) ([]string, error) {
	messages, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	return formatStoredMessages(messages), nil
}

func formatStoredMessages(messages []StoredMessage) []string {
	result := make([]string, 0, len(messages))
	for _, item := range messages {
		result = append(result, item.Format())
	}
	return result
}
