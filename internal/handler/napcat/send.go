package napcathandler

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	napcatproto "njk_go/internal/napcat"
	"njk_go/internal/util/uimage"
	"njk_go/internal/util/urand"
)

func (h *Handler) multiSendGroupImages(ctx context.Context, conn outboundWriter, groupID string, imgURLs []string, segmentType napcatproto.SegmentType) error {
	segments := []napcatproto.MessageSegment{}
	sentURLs := make([]string, 0, len(imgURLs))
	for idx, imgURL := range imgURLs {
		url := strings.TrimSpace(imgURL)
		if url == "" {
			continue
		}
		sentURLs = append(sentURLs, url)
		data := napcatproto.MessageSegmentData{File: url}
		if segmentType == napcatproto.SegmentTypeFile {
			data.Name = h.fileSegmentName(ctx, idx, url)
		}
		segments = append(segments, napcatproto.MessageSegment{
			Type: segmentType,
			Data: data,
		})
	}
	if len(segments) == 0 {
		return nil
	}
	req := napcatproto.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcatproto.SendGroupMsgParams{
			GroupID: napcatproto.ID(groupID),
			Message: napcatproto.NewSegmentMessage(segments...),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	h.service.RecordPending(groupID, "", time.Now(), false)
	log.Printf("【发送群图片】group=%s should_save=%t img_url=%s", groupID, false, strings.Join(sentURLs, ","))
	return nil
}

func (h *Handler) multiSendSegments(ctx context.Context, conn outboundWriter, groupID string, segments []napcatproto.MessageSegment) error {
	if len(segments) == 0 {
		return nil
	}
	req := napcatproto.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcatproto.SendGroupMsgParams{
			GroupID: napcatproto.ID(groupID),
			Message: napcatproto.NewSegmentMessage(segments...),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	h.service.RecordPending(groupID, "", time.Now(), false)
	log.Printf("【发送群消息段】group=%s should_save=%t segment_count=%d", groupID, false, len(segments))
	return nil
}

func (h *Handler) setMsgEmojiLike(ctx context.Context, conn outboundWriter, messageID string, emojiID string) error {
	if err := urand.SleepMillis(ctx, 1000, 2000); err != nil {
		return err
	}

	req := napcatproto.SetMsgEmojiLikeRequest{
		Action: "set_msg_emoji_like",
		Params: napcatproto.SetMsgEmojiLikeParams{
			MessageID: napcatproto.ID(messageID),
			EmojiID:   emojiID,
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	log.Printf("【发送表情回复】message_id=%s emoji_id=%s", messageID, emojiID)
	return nil
}

func (h *Handler) fileSegmentName(ctx context.Context, index int, sourceURL string) string {
	data, err := h.service.DownloadImage(ctx, sourceURL)
	if err != nil {
		log.Printf("【识别文件类型失败】url=%s err=%v", sourceURL, err)
		return uimage.FallbackFileSegmentName(index, sourceURL)
	}
	return uimage.FileSegmentNameFromData(index, sourceURL, data)
}
