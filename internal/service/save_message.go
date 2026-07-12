package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"njk_go/internal/dal/model"
	"njk_go/internal/napcat"
	"njk_go/internal/util/uconvert"
)

func (s *Service) SaveIncomingMessageAndCheckImages(ctx context.Context, event *napcat.GroupMessageEvent) ([]DuplicateImage, error) {
	senderID := event.Sender.UserID.String()
	groupID := event.GroupID.String()
	if err := s.store.UpsertUser(ctx, senderID, event.Sender.Nickname); err != nil {
		return nil, err
	}
	groupName := event.GroupName
	if groupName == "" {
		groupName = groupID
	}
	if err := s.store.UpsertGroup(ctx, groupID, groupName); err != nil {
		return nil, err
	}

	textParts := []string{}
	atUsers := []string{}
	imageURLs := []string{}

	var replyID *string
	for _, segment := range event.Message.Segments {
		switch segment.Type {
		case napcat.SegmentTypeReply:
			id := segment.Data.ID.String()
			textParts = append(textParts, fmt.Sprintf("[CQ:reply,id=%s]", id))
			replyID = &id

		case napcat.SegmentTypeAt:
			userID := strings.TrimSpace(segment.Data.QQ)
			if userID == "" {
				continue
			}

			textParts = append(textParts, fmt.Sprintf("[CQ:at,qq=%s]", userID))
			atUsers = append(atUsers, userID)

		case napcat.SegmentTypeText:
			textParts = append(textParts, segment.Data.Text)
		case napcat.SegmentTypeImage:
			if isEmojiImage(segment) {
				if err := s.EnsureEmojiWhitelist(ctx, groupID, segment.Data.URL); err != nil {
					log.Printf("【表情白名单处理失败】group=%s err=%v", groupID, err)
				}
			}
			if segment.Data.URL != "" {
				imageURLs = append(imageURLs, segment.Data.URL)
			}
		default:
			keyValStr, err := uconvert.StructToKeyValue(segment.Data)
			if err == nil {
				textParts = append(textParts, fmt.Sprintf("[CQ:%s,%s]", segment.Type, keyValStr))
			}
		}
	}

	rawJSON, err := json.Marshal(event.Message.Segments)
	if err != nil {
		return nil, err
	}

	messageID := event.MessageID.String()
	messageText := strings.Join(textParts, "")
	log.Printf("【处理后消息文本】%s", messageText)
	senderIDCopy := senderID
	groupIDCopy := groupID
	card := uconvert.EmptyToNil(event.Sender.Card)
	text := uconvert.EmptyToNil(messageText)
	rawJSONString := string(rawJSON)
	rawMessage := uconvert.EmptyToNil(event.RawMessage)

	message := &model.Message{
		MessageID:  messageID,
		Time:       time.Unix(event.Time, 0),
		SenderID:   &senderIDCopy,
		GroupID:    &groupIDCopy,
		Card:       card,
		Text:       text,
		ReplyID:    replyID,
		RawJSON:    &rawJSONString,
		RawMessage: rawMessage,
	}
	if err := s.store.SaveMessage(ctx, message); err != nil {
		return nil, err
	}

	for _, userID := range atUsers {
		if err := s.store.SaveAtUser(ctx, messageID, userID); err != nil {
			return nil, err
		}
	}

	s.rememberIncomingMessage(ctx, &memorySource{
		GroupID:   groupID,
		UserID:    senderID,
		MessageID: messageID,
		Content:   firstNonEmpty(event.RawMessage, messageText),
		ActorName: firstNonEmpty(event.Sender.Card, event.Sender.Nickname, senderID),
	})

	duplicates := []DuplicateImage{}
	for _, url := range imageURLs {
		duplicate, err := s.SaveAndCheckDuplicate(ctx, groupID, url, messageID)
		if err != nil {
			log.Printf("【图片消重失败】message=%s err=%v", messageID, err)
			continue
		}
		if duplicate != nil {
			duplicates = append(duplicates, *duplicate)
		}
	}

	return duplicates, nil
}

func (s *Service) saveSelfMessage(ctx context.Context, pending *pendingMessage, messageID string) error {
	if err := s.store.UpsertUser(ctx, s.cfg.BotUserID, s.cfg.BotNickname); err != nil {
		return err
	}
	if err := s.store.UpsertGroup(ctx, pending.GroupID, pending.GroupID); err != nil {
		return err
	}

	botUserID := s.cfg.BotUserID
	groupID := pending.GroupID
	text := pending.Message
	rawJSONBytes, err := json.Marshal(pending.Message)
	if err != nil {
		return err
	}
	rawJSON := string(rawJSONBytes)

	record := &model.Message{
		MessageID:  messageID,
		Time:       pending.SentAt,
		SenderID:   &botUserID,
		GroupID:    &groupID,
		Text:       &text,
		RawJSON:    &rawJSON,
		RawMessage: &text,
	}
	if err := s.store.SaveMessage(ctx, record); err != nil {
		return err
	}
	log.Printf("【bot回复入库成功】group=%s message=%s should_save=%t content=%q", groupID, messageID, pending.ShouldSave, text)
	s.rememberBotReply(ctx, &memorySource{
		GroupID:    groupID,
		UserID:     botUserID,
		MessageID:  messageID,
		Content:    text,
		ActorName:  s.cfg.BotNickname,
		IsBotReply: true,
	})
	return nil
}
