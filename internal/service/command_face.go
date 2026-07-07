package service

import (
	"context"
	"strconv"

	"njk_go/internal/util/uface"
)

func (s *Service) handleFaceCommand(ctx context.Context, groupID string, messageID string, match CommandMatch) (*pendingOutbound, error) {
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
		faceIDs, err := uface.ExtractFaceIDsFromRawJSON(item.RawJSON)
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
