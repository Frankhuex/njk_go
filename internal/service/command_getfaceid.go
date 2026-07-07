package service

import (
	"context"
	"strconv"
	"strings"
)

func (s *Service) handleGetFaceIDCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error) {
	if len(match.Groups) < 2 {
		return simpleOutbound(groupID, "参数错误"), nil
	}
	limit, err := strconv.Atoi(match.Groups[1])
	if err != nil || limit <= 0 {
		return simpleOutbound(groupID, "参数错误"), nil
	}

	rows, err := s.store.RecentFaceIDRows(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	message := formatGetFaceIDRows(rows)
	if message == "" {
		return simpleOutbound(groupID, "没有查到"), nil
	}
	return simpleOutbound(groupID, message), nil
}

func formatGetFaceIDRows(rows []GetFaceIDMessageRow) string {
	lines := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		if len(row.SegmentFaceIDs) > 0 {
			lines = append(lines, "发："+strings.Join(row.SegmentFaceIDs, "，"))
		}
		if len(row.EmojiLikeFaceIDs) > 0 {
			lines = append(lines, "贴："+strings.Join(row.EmojiLikeFaceIDs, "，"))
		}
	}
	return strings.Join(lines, "\n")
}
