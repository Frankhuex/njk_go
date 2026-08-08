package service

import (
	"context"
	"strings"
)

func (s *Service) handleAllFaceCommand(ctx context.Context, groupID string) (*OutboundAction, error) {
	allFaceIDs, err := s.store.AllFaceIDs(ctx)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, formatAllFaceIDs(allFaceIDs)), nil
}

func formatAllFaceIDs(allFaceIDs []string) string {
	return "全部：" + strings.Join(allFaceIDs, "，")
}
