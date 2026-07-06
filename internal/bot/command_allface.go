package bot

import (
	"context"
	"strings"
)

func (s *Service) handleAllFaceCommand(ctx context.Context, groupID string) (*pendingOutbound, error) {
	allFaceIDs, likedFaceIDs, err := s.store.AllFaceIDs(ctx)
	if err != nil {
		return nil, err
	}
	return simpleOutbound(groupID, formatAllFaceIDs(allFaceIDs, likedFaceIDs)), nil
}

func formatAllFaceIDs(allFaceIDs []string, likedFaceIDs []string) string {
	return "全部：" + strings.Join(allFaceIDs, "，") + "\n" +
		"贴过的：" + strings.Join(likedFaceIDs, "，")
}
