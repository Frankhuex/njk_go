package napcathandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"njk_go/internal/napcat"
	"njk_go/internal/service"
)

type outboundWriter interface {
	WriteText(payload []byte) error
}

type Handler struct {
	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleNotice(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.NoticeEvent) {
	if h == nil || h.service == nil || event == nil {
		return
	}
	log.Printf("【处理Notice】%s - 群ID: %s target_id=%s self_id=%s", clientAddr, event.GroupID, event.TargetID, event.SelfID)
	if event.NoticeType == napcat.EventNoticeTypeGroupMsgEmojiLike {
		h.service.HandleGroupMsgEmojiLikeNotice(ctx, event)
		return
	}
	if event.TargetID == "" || event.SelfID == "" || event.TargetID != event.SelfID {
		return
	}
	h.executeActions(ctx, conn, clientAddr, []service.OutboundAction{{
		GroupID:    event.GroupID.String(),
		Message:    "灰色中分已然绽放",
		ShouldSave: false,
	}})
}

func (h *Handler) HandleGroupMessage(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.GroupMessageEvent) {
	if h == nil || h.service == nil || event == nil {
		return
	}
	senderID := event.Sender.UserID.String()
	if senderID == "" {
		senderID = event.UserID.String()
	}
	groupID := event.GroupID.String()
	if !h.service.IsGroupAllowed(groupID) {
		log.Printf("【忽略群消息】%s - 群:%s 不在白名单", clientAddr, groupID)
		return
	}
	if h.service.IsUserBanned(senderID) {
		log.Printf("【忽略群消息】%s - 用户:%s 在黑名单", clientAddr, senderID)
		return
	}

	h.service.SaveFacesFromGroupMessage(ctx, event)

	match := h.service.MatchCommand(event.RawMessage)
	if match == nil && h.service.MentionsBot(event.Message) {
		match = h.service.NJKCommand()
	}
	commandName := "none"
	if match != nil {
		commandName = match.Key()
	}
	log.Printf("【处理群消息】%s - 群:%s 消息:%s 命中:%s", clientAddr, groupID, event.RawMessage, commandName)

	actions := make([]service.OutboundAction, 0)
	if match == nil || match.Key() == "njk" {
		duplicates, err := h.service.SaveIncomingMessageAndCheckImages(ctx, event)
		if err != nil {
			log.Printf("【消息落库失败】%s - %v", clientAddr, err)
		}
		for _, duplicate := range duplicates {
			text := fmt.Sprintf("[CQ:reply,id=%s]🇫🇷%d遍了。%s在%s就🇫🇷了。", duplicate.MessageID, duplicate.Count, duplicate.SenderName, duplicate.SentAt.Format("2006-01-02 15:04:05"))
			actions = append(actions, service.OutboundAction{
				GroupID:    groupID,
				Message:    text,
				ShouldSave: false,
			})
		}
	}

	if match != nil {
		action, err := h.service.ExecuteCommand(ctx, event, match)
		if err != nil {
			log.Printf("【命令处理失败】%s - %v", clientAddr, err)
		} else if action != nil {
			actions = append(actions, *action)
		}
	} else if h.service.ShouldRandomReply() {
		action, err := h.service.GenerateNJKReply(ctx, event, groupID)
		if err != nil {
			log.Printf("【随机发言失败】%s - %v", clientAddr, err)
		} else if action != nil {
			actions = append(actions, *action)
		}
	}

	h.executeActions(ctx, conn, clientAddr, actions)
}

func (h *Handler) HandleActionResponse(ctx context.Context, action *napcat.ActionEnvelope) {
	if h == nil || h.service == nil || action == nil {
		return
	}
	var data napcat.SendMsgResponseData
	if err := json.Unmarshal(action.Data, &data); err != nil {
		return
	}
	if err := h.service.CompleteActionResult(ctx, action.Status, action.Retcode, data.MessageID.String()); err != nil {
		log.Printf("【处理回执失败】message_id=%s err=%v", data.MessageID, err)
	}
}

func (h *Handler) executeActions(ctx context.Context, conn outboundWriter, clientAddr string, actions []service.OutboundAction) {
	for _, action := range actions {
		if action.Message != "" {
			if err := h.sendGroupText(ctx, conn, action.GroupID, action.Message, action.ShouldSave); err != nil {
				log.Printf("【发送响应失败】%s - %v", clientAddr, err)
			}
		}
		if len(action.Segments) > 0 {
			if err := h.multiSendSegments(ctx, conn, action.GroupID, action.Segments); err != nil {
				log.Printf("【发送消息段响应失败】%s - %v", clientAddr, err)
			}
		}
		if len(action.ImageURLs) > 0 {
			segmentType := action.ImageSegmentType
			if segmentType == "" {
				segmentType = napcat.SegmentTypeImage
			}
			if err := h.multiSendGroupImages(ctx, conn, action.GroupID, action.ImageURLs, segmentType); err != nil {
				log.Printf("【发送图片响应失败】%s - %v", clientAddr, err)
			}
		}
		for _, emojiID := range action.EmojiLikeIDs {
			if err := h.setMsgEmojiLike(ctx, conn, action.EmojiLikeMessageID, emojiID); err != nil {
				log.Printf("【发送表情回复失败】%s - %v", clientAddr, err)
			}
		}
	}
}

func (h *Handler) sendGroupText(ctx context.Context, conn outboundWriter, groupID string, message string, shouldSave bool) error {
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
	h.service.RecordPending(groupID, message, time.Now(), shouldSave)
	log.Printf("【发送群消息】group=%s should_save=%t message=%s", groupID, shouldSave, message)
	return nil
}
