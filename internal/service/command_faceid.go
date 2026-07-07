package service

import (
	"context"
	"fmt"
	"strconv"

	"njk_go/internal/napcat"
)

const (
	maxFaceIDSegments = 50
	maxFaceStickers   = 10
)

func (s *Service) handleFaceIDCommand(ctx context.Context, groupID string, messageID string, match CommandMatch) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	left, err := strconv.Atoi(match.Groups[1])
	if err != nil {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	right := left
	if len(match.Groups) > 2 && match.Groups[2] != "" {
		right, err = strconv.Atoi(match.Groups[2])
		if err != nil {
			return simpleOutbound(groupID, "参数错误"), nil
		}
	}

	if left <= 0 || right <= 0 || left > right {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	if right-left+1 > maxFaceIDSegments {
		return simpleOutbound(groupID, fmt.Sprintf("太多啦，最多%d个", maxFaceIDSegments)), nil
	}

	segments := make([]napcat.MessageSegment, 0, right-left+1)
	emojiIDs := make([]string, 0, right-left+1)
	for id := left; id <= right; id++ {
		segments = append(segments, napcat.NewFaceSegment(napcat.ID(strconv.Itoa(id))))
		if id-left+1 <= maxFaceStickers {
			emojiIDs = append(emojiIDs, strconv.FormatInt(int64(id), 10))
		}
	}

	return &pendingOutbound{
		GroupID:            groupID,
		Segments:           segments,
		EmojiLikeMessageID: messageID,
		EmojiLikeIDs:       emojiIDs,
		ShouldSave:         true,
	}, nil
}
