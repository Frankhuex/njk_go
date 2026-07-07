package napcathandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/gif"
	"log"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	napcatproto "njk_go/internal/napcat"
	svc "njk_go/internal/service"
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
	if err := svc.SleepRandomMillis(ctx, h.service.Random(), 1000, 2000); err != nil {
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
		return fallbackFileSegmentName(index, sourceURL)
	}
	return fileSegmentNameFromImageData(index, sourceURL, data)
}

func fileSegmentNameFromImageData(index int, sourceURL string, data []byte) string {
	ext := fallbackImageExt(sourceURL, data)
	if _, err := gif.DecodeAll(bytes.NewReader(data)); err == nil {
		ext = ".gif"
	}
	return fmt.Sprintf("image_%d%s", index+1, ext)
}

func fallbackFileSegmentName(index int, sourceURL string) string {
	ext := normalizedImageExt(sourceURL)
	if ext == "" {
		ext = ".png"
	}
	return fmt.Sprintf("image_%d%s", index+1, ext)
}

func fallbackImageExt(sourceURL string, data []byte) string {
	contentType := strings.ToLower(http.DetectContentType(data))
	switch {
	case strings.Contains(contentType, "image/jpeg"), strings.Contains(contentType, "image/jpg"):
		return ".jpg"
	case strings.Contains(contentType, "image/png"):
		return ".png"
	}
	if ext := normalizedImageExt(sourceURL); ext != "" {
		return ext
	}
	return ".png"
}

func normalizeOutboundText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, `\r\n`, "\n")
	text = strings.ReplaceAll(text, `\n`, "\n")
	text = strings.ReplaceAll(text, `\t`, "\t")
	return text
}

func normalizedImageExt(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Path != "" {
		return strings.ToLower(path.Ext(parsed.Path))
	}
	return strings.ToLower(filepath.Ext(sourceURL))
}
