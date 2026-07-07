package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func (s *Service) handleJSONCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	count, err := strconv.Atoi(match.Groups[1])
	if err != nil || count <= 0 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	history, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}

	message, err := formatRawJSONMessages(history)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, message), nil
}

func formatRawJSONMessages(messages []StoredMessage) (string, error) {
	items := make([]json.RawMessage, 0, len(messages))
	for _, message := range messages {
		rawJSON := strings.TrimSpace(message.RawJSON)
		if rawJSON == "" {
			items = append(items, json.RawMessage("null"))
			continue
		}
		if json.Valid([]byte(rawJSON)) {
			items = append(items, json.RawMessage(rawJSON))
			continue
		}

		encoded, _ := json.Marshal(rawJSON)
		items = append(items, json.RawMessage(encoded))
	}

	itemStrs := make([]string, 0, len(items))
	for _, item := range items {
		itemStr, _ := json.MarshalIndent(item, "", "    ")
		itemStrs = append(itemStrs, string(itemStr))
	}
	return strings.Join(itemStrs, "\n\n"), nil
}
