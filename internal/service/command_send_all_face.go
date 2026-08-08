package service

import (
	"context"

	"njk_go/internal/napcat"
)

func (s *Service) handleSendAllFaceCommand(ctx context.Context, groupID string) (*OutboundAction, error) {
	allFaceIDs, err := s.store.AllFaceIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(allFaceIDs) == 0 {
		return simpleOutbound(groupID, "暂无系统表情"), nil
	}
	return segmentsOutbound(groupID, buildSendAllFaceSegments(allFaceIDs)), nil
}

// buildSendAllFaceSegments 为每个 face id 依次放入：text(id 原文)、face(id)、中文逗号。
// 最后一个 face id 后不再追加逗号。
func buildSendAllFaceSegments(faceIDs []string) []napcat.MessageSegment {
	segments := make([]napcat.MessageSegment, 0, len(faceIDs)*3)
	for i, id := range faceIDs {
		segments = append(segments,
			napcat.NewTextSegment(id),
			napcat.NewFaceSegment(napcat.ID(id)),
		)
		if i < len(faceIDs)-1 {
			segments = append(segments, napcat.NewTextSegment("，"))
		}
	}
	return segments
}
