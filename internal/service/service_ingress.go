package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/gif"
	"log"
	"net/http"
	"strings"
	"time"

	"njk_go/internal/napcat"
)

func (s *Service) HandleNotice(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.NoticeEvent) {
	if event == nil {
		return
	}

	log.Printf("【处理Notice】%s - 群ID: %s target_id=%s self_id=%s", clientAddr, event.GroupID, event.TargetID, event.SelfID)
	if event.NoticeType == napcat.EventNoticeTypeGroupMsgEmojiLike {
		s.handleGroupMsgEmojiLikeNotice(ctx, event)
		return
	}

	if event.TargetID == "" || event.SelfID == "" || event.TargetID != event.SelfID {
		return
	}

	if err := s.sendGroupText(ctx, conn, event.GroupID.String(), "灰色中分已然绽放", false); err != nil {
		log.Printf("【发送Notice响应失败】%s - %v", clientAddr, err)
		return
	}

	log.Printf("【发送Notice响应】%s - 群ID: %s", clientAddr, event.GroupID)
}

func (s *Service) HandleGroupMessage(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.GroupMessageEvent) {
	if event == nil {
		return
	}

	groupID := event.GroupID.String()
	if len(s.cfg.AllowedGroupIDs) > 0 {
		if _, ok := s.cfg.AllowedGroupIDs[groupID]; !ok {
			log.Printf("【忽略群消息】%s - 群:%s 不在白名单", clientAddr, groupID)
			return
		}
	}
	senderID := event.Sender.UserID.String()
	if senderID == "" {
		senderID = event.UserID.String()
	}
	if _, banned := s.cfg.BannedUserIDs[senderID]; banned {
		log.Printf("【忽略群消息】%s - 用户:%s 在黑名单", clientAddr, senderID)
		return
	}
	s.saveFacesFromGroupMessage(ctx, event)

	rawMessage := event.RawMessage
	match := s.matchCommand(rawMessage)
	if match == nil && mentionsBot(event.Message, s.cfg.BotUserID) {
		match = s.commandByKey(commandNJK)
	}
	commandName := "none"
	if match != nil {
		commandName = string(match.Command.Key)
	}
	log.Printf("【处理群消息】%s - 群:%s 消息:%s 命中:%s", clientAddr, groupID, rawMessage, commandName)

	responses := []pendingOutbound{}

	if match == nil || match.Command.Key == commandNJK {
		duplicates, err := s.saveIncomingMessageAndCheckImages(ctx, event)
		if err != nil {
			log.Printf("【消息落库失败】%s - %v", clientAddr, err)
		}
		for _, duplicate := range duplicates {
			text := fmt.Sprintf("[CQ:reply,id=%s]🇫🇷%d遍了。%s在%s就🇫🇷了。", duplicate.MessageID, duplicate.Count, duplicate.SenderName, formatDisplayTime(duplicate.SentAt))
			responses = append(responses, pendingOutbound{
				GroupID:    groupID,
				Message:    text,
				ShouldSave: false,
			})
		}
	}

	if match != nil {
		outbound, err := s.handleMatchedCommand(ctx, event, *match)
		if err != nil {
			log.Printf("【命令处理失败】%s - %v", clientAddr, err)
		} else if outbound != nil {
			responses = append(responses, *outbound)
		}
	} else if s.rng.Float64() < 0.08 {
		outbound, err := s.handleNJKReply(ctx, event, groupID)
		if err != nil {
			log.Printf("【随机发言失败】%s - %v", clientAddr, err)
		} else if outbound != nil {
			responses = append(responses, *outbound)
		}
	}

	for _, response := range responses {
		if response.Message != "" {
			if err := s.sendGroupText(ctx, conn, response.GroupID, response.Message, response.ShouldSave); err != nil {
				log.Printf("【发送响应失败】%s - %v", clientAddr, err)
			}
		}
		if len(response.Segments) > 0 {
			if err := s.multiSendSegments(ctx, conn, response.GroupID, response.Segments); err != nil {
				log.Printf("【发送消息段响应失败】%s - %v", clientAddr, err)
			}
		}
		if len(response.ImageURLs) > 0 {
			segmentType := response.ImageSegmentType
			if segmentType == "" {
				segmentType = napcat.SegmentTypeImage
			}
			if err := s.multiSendGroupImages(ctx, conn, response.GroupID, response.ImageURLs, segmentType); err != nil {
				log.Printf("【发送图片响应失败】%s - %v", clientAddr, err)
			}
		}
		for _, emojiID := range response.EmojiLikeIDs {
			if err := s.setMsgEmojiLike(ctx, conn, response.EmojiLikeMessageID, emojiID); err != nil {
				log.Printf("【发送表情回复失败】%s - %v", clientAddr, err)
			}
		}
	}
}

func (s *Service) HandleActionResponse(ctx context.Context, action *napcat.ActionEnvelope) {
	if action == nil || action.Status != "ok" || action.Retcode != 0 {
		return
	}

	var data napcat.SendMsgResponseData
	if err := json.Unmarshal(action.Data, &data); err != nil {
		return
	}
	if data.MessageID == "" {
		return
	}

	pending := s.pending.Pop()
	if pending == nil || !pending.ShouldSave {
		return
	}

	if err := s.saveSelfMessage(ctx, pending, data.MessageID.String()); err != nil {
		log.Printf("【保存自己消息失败】message_id=%s err=%v", data.MessageID, err)
		return
	}
	log.Printf("【完成自己消息存储】消息ID: %s", data.MessageID)
}

func (s *Service) sendGroupText(ctx context.Context, conn outboundWriter, groupID string, message string, shouldSave bool) error {
	message = normalizeOutboundText(message)
	req := napcat.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcat.SendGroupMsgParams{
			GroupID: napcat.ID(groupID),
			Message: napcat.NewTextMessage(message),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	s.pending.Push(pendingMessage{
		GroupID:    groupID,
		Message:    message,
		SentAt:     time.Now(),
		ShouldSave: shouldSave,
	})
	log.Printf("【发送群消息】group=%s should_save=%t message=%s", groupID, shouldSave, message)
	return nil
}

// segmentType必须为image或file
func (s *Service) multiSendGroupImages(ctx context.Context, conn outboundWriter, groupID string, imgURLs []string, segmentType napcat.SegmentType) error {
	segments := []napcat.MessageSegment{}
	sentURLs := make([]string, 0, len(imgURLs))
	for idx, imgURL := range imgURLs {
		url := strings.TrimSpace(imgURL)
		if url == "" {
			continue
		}
		sentURLs = append(sentURLs, url)
		data := napcat.MessageSegmentData{File: url}
		if segmentType == napcat.SegmentTypeFile {
			data.Name = s.fileSegmentName(ctx, idx, url)
		}
		segments = append(segments, napcat.MessageSegment{
			Type: segmentType,
			Data: data,
		})
	}
	if len(segments) == 0 {
		return nil
	}
	req := napcat.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcat.SendGroupMsgParams{
			GroupID: napcat.ID(groupID),
			Message: napcat.NewSegmentMessage(segments...),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	s.pending.Push(pendingMessage{
		GroupID:    groupID,
		Message:    "",
		SentAt:     time.Now(),
		ShouldSave: false,
	})
	log.Printf("【发送群图片】group=%s should_save=%t img_url=%s", groupID, false, strings.Join(sentURLs, ","))
	return nil
}

func (s *Service) multiSendSegments(ctx context.Context, conn outboundWriter, groupID string, segments []napcat.MessageSegment) error {
	if len(segments) == 0 {
		return nil
	}
	req := napcat.SendGroupMsgRequest{
		Action: "send_group_msg",
		Params: napcat.SendGroupMsgParams{
			GroupID: napcat.ID(groupID),
			Message: napcat.NewSegmentMessage(segments...),
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := conn.WriteText(data); err != nil {
		return err
	}
	s.pending.Push(pendingMessage{
		GroupID:    groupID,
		Message:    "",
		SentAt:     time.Now(),
		ShouldSave: false,
	})
	log.Printf("【发送群消息段】group=%s should_save=%t segment_count=%d", groupID, false, len(segments))
	return nil
}

func (s *Service) fileSegmentName(ctx context.Context, index int, sourceURL string) string {
	data, err := s.imageService.download(ctx, sourceURL)
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

func (s *Service) setMsgEmojiLike(ctx context.Context, conn outboundWriter, messageID string, emojiID string) error {
	if err := sleepRandomMillis(ctx, s.rng, 1000, 2000); err != nil {
		return err
	}

	req := napcat.SetMsgEmojiLikeRequest{
		Action: "set_msg_emoji_like",
		Params: napcat.SetMsgEmojiLikeParams{
			MessageID: napcat.ID(messageID),
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
