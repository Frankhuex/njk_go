package bot

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"

	"njk_go/internal/napcat"
)

func faceIDsFromSegments(segments []napcat.MessageSegment) []string {
	seen := make(map[string]struct{}, len(segments))
	faceIDs := make([]string, 0)
	for _, segment := range segments {
		if segment.Type != napcat.SegmentTypeFace {
			continue
		}
		faceID := strings.TrimSpace(segment.Data.ID.String())
		if faceID == "" {
			continue
		}
		if _, ok := seen[faceID]; ok {
			continue
		}
		seen[faceID] = struct{}{}
		faceIDs = append(faceIDs, faceID)
	}
	return faceIDs
}

func sortFaceIDs(faceIDs []string) {
	sort.SliceStable(faceIDs, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(faceIDs[i], 10, 64)
		right, rightErr := strconv.ParseInt(faceIDs[j], 10, 64)
		leftOK := leftErr == nil
		rightOK := rightErr == nil
		if leftOK && rightOK {
			if left == right {
				return faceIDs[i] < faceIDs[j]
			}
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return faceIDs[i] < faceIDs[j]
	})
}

func emojiLikeFaceIDs(event *napcat.NoticeEvent) []string {
	if event == nil {
		return nil
	}
	seen := map[string]struct{}{}
	faceIDs := make([]string, 0, len(event.Likes))
	add := func(faceID string) {
		faceID = strings.TrimSpace(faceID)
		if faceID == "" {
			return
		}
		if _, ok := seen[faceID]; ok {
			return
		}
		seen[faceID] = struct{}{}
		faceIDs = append(faceIDs, faceID)
	}
	for _, like := range event.Likes {
		add(like.EmojiID)
	}
	return faceIDs
}

func (s *Service) saveFacesFromGroupMessage(ctx context.Context, event *napcat.GroupMessageEvent) {
	if s == nil || s.store == nil || s.store.db == nil || event == nil {
		return
	}
	for _, faceID := range faceIDsFromSegments(event.Message.Segments) {
		if err := s.store.UpsertFace(ctx, faceID); err != nil {
			log.Printf("【系统表情入库失败】face_id=%s err=%v", faceID, err)
		}
		log.Printf("【系统表情入库成功】face_id=%s", faceID)
	}
}

func (s *Service) handleGroupMsgEmojiLikeNotice(ctx context.Context, event *napcat.NoticeEvent) {
	if s == nil || s.store == nil || s.store.db == nil || event == nil {
		return
	}
	messageID := strings.TrimSpace(event.MessageID.String())
	userID := strings.TrimSpace(event.UserID.String())
	if userID == "" {
		userID = strings.TrimSpace(event.OperatorID.String())
	}
	if messageID == "" || userID == "" {
		log.Printf("【表情回应入库跳过】message_id=%s user_id=%s", messageID, userID)
		return
	}
	faceIDs := emojiLikeFaceIDs(event)
	if len(faceIDs) == 0 {
		log.Printf("【表情回应入库跳过】message_id=%s user_id=%s face_id为空", messageID, userID)
		return
	}
	for _, faceID := range faceIDs {
		if err := s.store.SaveEmojiLike(ctx, messageID, userID, faceID); err != nil {
			log.Printf("【表情回应入库失败】message_id=%s user_id=%s face_id=%s err=%v", messageID, userID, faceID, err)
		}
		log.Printf("【表情回应入库成功】message_id=%s user_id=%s face_id=%s", messageID, userID, faceID)
	}
}
