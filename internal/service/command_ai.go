package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"njk_go/internal/client/pgstore"
	"njk_go/internal/util/urand"
	"njk_go/internal/util/utext"
	"njk_go/internal/util/utime"
)

func (s *Service) handleAIPromptCommand(ctx context.Context, groupID string, match CommandMatch) (*OutboundAction, error) {
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

func (s *Service) handleAICommand(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
	count, _ := strconv.Atoi(match.Groups[1])
	history, err := s.store.RecentMessages(ctx, cmdCtx.GroupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(cmdCtx.GroupID), nil
	}
	historyPrompt := fmt.Sprintf("%v", formatStoredMessages(history))
	queryText := strings.Join(formatStoredMessages(history), "\n")
	historyPrompt = s.enrichPromptWithMemory(ctx, cmdCtx.GroupID, cmdCtx.SenderID, queryText, historyPrompt)
	result, err := s.aiClient.Complete(ctx, match.Command.SystemPrompt, historyPrompt, nil)
	if err != nil {
		return nil, err
	}
	s.setLastAI(cmdCtx.GroupID, history[0].Time)
	return savedReplyOutbound(cmdCtx.GroupID, history[0].MessageID, result), nil
}

func (s *Service) handleAICCommand(ctx context.Context, cmdCtx CommandContext) (*OutboundAction, error) {
	start, ok := s.getLastAI(cmdCtx.GroupID)
	if !ok {
		return simpleOutbound(cmdCtx.GroupID, "请先发起一次「.ai后接数字」"), nil
	}
	history, err := s.store.MessagesSince(ctx, cmdCtx.GroupID, start)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(cmdCtx.GroupID), nil
	}
	systemPrompt, err := s.systemPrompt(commandAI)
	if err != nil {
		return nil, err
	}
	historyPrompt := fmt.Sprintf("%v", formatStoredMessages(history))
	queryText := strings.Join(formatStoredMessages(history), "\n")
	historyPrompt = s.enrichPromptWithMemory(ctx, cmdCtx.GroupID, cmdCtx.SenderID, queryText, historyPrompt)
	result, err := s.aiClient.Complete(ctx, systemPrompt, historyPrompt, nil)
	if err != nil {
		return nil, err
	}
	return savedReplyOutbound(cmdCtx.GroupID, history[0].MessageID, result), nil
}

func (s *Service) handleReportCommand(ctx context.Context, groupID string, match CommandMatch) (*OutboundAction, error) {
	dayNum, _ := strconv.Atoi(match.Groups[1])
	stats, err := s.store.ReportStats(ctx, groupID, utime.StartOfReport(dayNum, time.Now()), 10)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, formatReport(stats, dayNum, 10)), nil
}

func (s *Service) GenerateNJKReply(ctx context.Context, cmdCtx CommandContext) (*OutboundAction, error) {
	history, err := s.historyStrings(ctx, cmdCtx.GroupID, urand.Range(10, 30))
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
	historyPrompt := fmt.Sprintf("%v", history)
	queryText := strings.TrimSpace(cmdCtx.RawMessage)
	if queryText == "" {
		queryText = strings.Join(history, "\n")
	}
	historyPrompt = s.enrichPromptWithMemory(ctx, cmdCtx.GroupID, cmdCtx.SenderID, queryText, historyPrompt)
	for i := 0; i < 5; i++ {
		temperature := 0.8 + urand.Float64()*0.1
		candidate, err := s.aiClient.Complete(ctx, systemPrompt, historyPrompt, &temperature)
		if err != nil {
			return nil, err
		}
		if !utext.ContainsExact(history, candidate) {
			result = candidate
			break
		}
		result = candidate
	}
	if result == "" {
		return nil, nil
	}
	return &OutboundAction{GroupID: cmdCtx.GroupID, Message: result, ShouldSave: true}, nil
}

func (s *Service) historyStrings(ctx context.Context, groupID string, count int) ([]string, error) {
	messages, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	return formatStoredMessages(messages), nil
}

func formatStoredMessages(messages []pgstore.StoredMessage) []string {
	result := make([]string, 0, len(messages))
	for _, item := range messages {
		result = append(result, item.Format())
	}
	return result
}
