package service

import (
	"context"
	"log"
	"strings"

	"njk_go/internal/napcat"
	"njk_go/internal/util/uface"
)

func (s *Service) SaveFacesFromGroupMessage(ctx context.Context, event *napcat.GroupMessageEvent) {
	if s == nil || s.store == nil || event == nil {
		return
	}
	for _, faceID := range uface.FaceIDsFromSegments(event.Message.Segments) {
		if err := s.store.UpsertFace(ctx, faceID); err != nil {
			log.Printf("【系统表情入库失败】face_id=%s err=%v", faceID, err)
		}
		log.Printf("【系统表情入库成功】face_id=%s", faceID)
	}
}

func (s *Service) HandleGroupMsgEmojiLikeNotice(ctx context.Context, event *napcat.NoticeEvent) {
	if s == nil || s.store == nil || event == nil {
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
	faceIDs := uface.EmojiLikeFaceIDs(event.Likes)
	if len(faceIDs) == 0 {
		log.Printf("【表情回应入库跳过】message_id=%s user_id=%s face_id为空", messageID, userID)
		return
	}
	if err := s.store.EnsureNoticeMessage(ctx, messageID, event.GroupID.String(), userID, event.Time); err != nil {
		log.Printf("【表情回应消息补写失败】message_id=%s group_id=%s user_id=%s err=%v", messageID, event.GroupID, userID, err)
	}
	for _, faceID := range faceIDs {
		if err := s.store.SaveEmojiLike(ctx, messageID, userID, faceID); err != nil {
			log.Printf("【表情回应入库失败】message_id=%s user_id=%s face_id=%s err=%v", messageID, userID, faceID, err)
		}
		log.Printf("【表情回应入库成功】message_id=%s user_id=%s face_id=%s", messageID, userID, faceID)
	}
}
