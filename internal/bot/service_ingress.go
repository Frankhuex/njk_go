package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"njk_go/internal/napcat"
)

func (s *Service) HandleNotice(ctx context.Context, conn outboundWriter, clientAddr string, event *napcat.NoticeEvent) {
	if event == nil {
		return
	}

	log.Printf("【处理Notice】%s - 群ID: %s target_id=%s self_id=%s", clientAddr, event.GroupID, event.TargetID, event.SelfID)

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
		if err := s.sendGroupText(ctx, conn, response.GroupID, response.Message, response.ShouldSave); err != nil {
			log.Printf("【发送响应失败】%s - %v", clientAddr, err)
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
