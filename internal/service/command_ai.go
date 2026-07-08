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
	history, err := s.store.RecentMessageAndImages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}
	textPrompt, imageURLs := buildMultimodalPrompt(history)
	result, err := s.completeWithMultimodalFallback(ctx, groupID, match.Command.SystemPrompt, fmt.Sprintf("%v", textPrompt), imageURLs, nil)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, result), nil
}

func (s *Service) handleAICommand(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
	count, _ := strconv.Atoi(match.Groups[1])
	history, err := s.store.RecentMessageAndImages(ctx, cmdCtx.GroupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(cmdCtx.GroupID), nil
	}
	formattedHistory := formatStoredMessagesWithImages(history)
	historyPrompt := fmt.Sprintf("%v", formattedHistory)
	queryText := strings.Join(formattedHistory, "\n")
	historyPrompt = s.enrichPromptWithMemory(ctx, cmdCtx.GroupID, cmdCtx.SenderID, queryText, historyPrompt)
	_, imageURLs := buildMultimodalPrompt(history)
	result, err := s.completeWithMultimodalFallback(ctx, cmdCtx.GroupID, match.Command.SystemPrompt, historyPrompt, imageURLs, nil)
	if err != nil {
		return nil, err
	}
	s.setLastAI(cmdCtx.GroupID, history[0].Message.Time)
	return savedReplyOutbound(cmdCtx.GroupID, history[0].Message.MessageID, result), nil
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
	imageURLs, err := s.messageImagesSince(ctx, cmdCtx.GroupID, start)
	if err != nil {
		return nil, err
	}
	result, err := s.completeWithMultimodalFallback(ctx, cmdCtx.GroupID, systemPrompt, historyPrompt, imageURLs, nil)
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
	count := urand.Range(10, 30)
	history, err := s.store.RecentMessageAndImages(ctx, cmdCtx.GroupID, count)
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
	formattedHistory := formatStoredMessagesWithImages(history)
	historyPrompt := fmt.Sprintf("%v", formattedHistory)
	queryText := strings.TrimSpace(cmdCtx.RawMessage)
	if queryText == "" {
		queryText = strings.Join(formattedHistory, "\n")
	}
	historyPrompt = s.enrichPromptWithMemory(ctx, cmdCtx.GroupID, cmdCtx.SenderID, queryText, historyPrompt)
	_, imageURLs := buildMultimodalPrompt(history)
	for i := 0; i < 5; i++ {
		temperature := 0.8 + urand.Float64()*0.1
		candidate, err := s.completeWithMultimodalFallback(ctx, cmdCtx.GroupID, systemPrompt, historyPrompt, imageURLs, &temperature)
		if err != nil {
			return nil, err
		}
		if !utext.ContainsExact(formattedHistory, candidate) {
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

func formatStoredMessagesWithImages(messages []pgstore.MessageWithImages) []string {
	result := make([]string, 0, len(messages))
	for _, item := range messages {
		result = append(result, item.Message.Format())
	}
	return result
}

func buildMultimodalPrompt(messages []pgstore.MessageWithImages) (string, []string) {
	texts := formatStoredMessagesWithImages(messages)
	imageURLs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range messages {
		for _, image := range item.Images {
			if image.URL == nil {
				continue
			}
			url := strings.TrimSpace(*image.URL)
			if url == "" {
				continue
			}
			if _, exists := seen[url]; exists {
				continue
			}
			seen[url] = struct{}{}
			imageURLs = append(imageURLs, url)
		}
	}
	return strings.Join(texts, "\n"), imageURLs
}

func (s *Service) messageImagesSince(ctx context.Context, groupID string, start time.Time) ([]string, error) {
	messages, err := s.store.MessagesSince(ctx, groupID, start)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	latest := messages[len(messages)-1].Time
	rows, err := s.store.RecentMessageAndImages(ctx, groupID, len(messages))
	if err != nil {
		return nil, err
	}
	imageURLs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, row := range rows {
		if row.Message.Time.Before(start) || row.Message.Time.After(latest) {
			continue
		}
		for _, image := range row.Images {
			if image.URL == nil {
				continue
			}
			url := strings.TrimSpace(*image.URL)
			if url == "" {
				continue
			}
			if _, exists := seen[url]; exists {
				continue
			}
			seen[url] = struct{}{}
			imageURLs = append(imageURLs, url)
		}
	}
	return imageURLs, nil
}

func formatStoredMessages(messages []pgstore.StoredMessage) []string {
	result := make([]string, 0, len(messages))
	for _, item := range messages {
		result = append(result, item.Format())
	}
	return result
}
