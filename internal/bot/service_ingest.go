package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"njk_go/internal/model"
	"njk_go/internal/napcat"
)

func (s *Service) saveIncomingMessageAndCheckImages(ctx context.Context, event *napcat.GroupMessageEvent) ([]DuplicateImage, error) {
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
		case "reply":
			id := segment.Data.ID.String()
			if id != "" {
				replyID = &id
			}
		case "at":
			userID := strings.TrimSpace(segment.Data.QQ)
			if userID == "" {
				continue
			}
			user, err := s.store.FindUser(ctx, userID)
			if err != nil {
				return nil, err
			}
			if user != nil {
				textParts = append(textParts, "@"+user.Nickname)
				atUsers = append(atUsers, user.UserID)
			} else {
				textParts = append(textParts, "@"+userID)
			}
		case "text":
			textParts = append(textParts, segment.Data.Text)
		case "image":
			if isEmojiImage(segment) {
				if err := s.imageService.EnsureEmojiWhitelist(ctx, groupID, segment.Data.URL); err != nil {
					log.Printf("【表情白名单处理失败】group=%s err=%v", groupID, err)
				}
				continue
			}
			if segment.Data.URL != "" {
				imageURLs = append(imageURLs, segment.Data.URL)
			}
		default:
			if segment.Data.Summary != "" {
				textParts = append(textParts, fmt.Sprintf("[%s: %s]", segment.Type, segment.Data.Summary))
			} else {
				textParts = append(textParts, "["+segment.Type+"]")
			}
		}
	}

	rawJSON, err := json.Marshal(event.Message.Segments)
	if err != nil {
		return nil, err
	}

	messageID := event.MessageID.String()
	messageText := strings.Join(textParts, "")
	senderIDCopy := senderID
	groupIDCopy := groupID
	card := emptyToNil(event.Sender.Card)
	text := emptyToNil(messageText)
	rawJSONString := string(rawJSON)
	rawMessage := emptyToNil(event.RawMessage)

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

	duplicates := []DuplicateImage{}
	for _, url := range imageURLs {
		duplicate, err := s.imageService.SaveAndCheckDuplicate(ctx, groupID, url, messageID)
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

	return s.store.SaveMessage(ctx, &model.Message{
		MessageID:  messageID,
		Time:       pending.SentAt,
		SenderID:   &botUserID,
		GroupID:    &groupID,
		Text:       &text,
		RawJSON:    &rawJSON,
		RawMessage: &text,
	})
}

func isEmojiImage(segment napcat.MessageSegment) bool {
	data := segment.Data
	return data.EmojiID != "" || data.EmojiPackageID != 0 || data.Key != "" || data.SubType == 1 || strings.Contains(data.Summary, "动画表情")
}

func mentionsBot(message napcat.MessagePayload, botUserID string) bool {
	for _, segment := range message.Segments {
		if segment.Type == "at" && strings.TrimSpace(segment.Data.QQ) == botUserID {
			return true
		}
	}
	return false
}

func emptyToNil(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}
