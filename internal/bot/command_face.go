package bot

import (
	"context"
	"encoding/json"
	"strconv"

	"njk_go/internal/napcat"
)

func (s *Service) handleFaceCommand(ctx context.Context, groupID string, messageID string, match matchedCommand) (*pendingOutbound, error) {
	count, _ := strconv.Atoi(match.Groups[1])
	history, err := s.store.RecentMessages(ctx, groupID, count)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return insufficientHistory(groupID), nil
	}

	emojiIDs := make([]string, 0)
	for _, item := range history {
		faceIDs, err := extractFaceIDsFromRawJSON(item.RawJSON)
		if err != nil {
			continue
		}
		emojiIDs = append(emojiIDs, faceIDs...)
	}
	if len(emojiIDs) == 0 {
		return nil, nil
	}

	return &pendingOutbound{
		GroupID:            groupID,
		EmojiLikeMessageID: messageID,
		EmojiLikeIDs:       emojiIDs,
	}, nil
}

func extractFaceIDsFromRawJSON(rawJSON string) ([]string, error) {
	if rawJSON == "" {
		return nil, nil
	}

	var segments []napcat.MessageSegment
	if err := json.Unmarshal([]byte(rawJSON), &segments); err != nil {
		return nil, err
	}

	emojiIDs := make([]string, 0)
	for _, segment := range segments {
		if segment.Type != "face" || segment.Data.ID == "" {
			continue
		}
		emojiIDs = append(emojiIDs, segment.Data.ID.String())
	}
	return emojiIDs, nil
}
