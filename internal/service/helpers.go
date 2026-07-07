package service

import (
	"fmt"

	"njk_go/internal/napcat"
)

func (s *Service) systemPrompt(key commandKey) (string, error) {
	command := s.commandByKey(key)
	if command == nil {
		return "", fmt.Errorf("%s command not configured", key)
	}
	return command.Command.SystemPrompt, nil
}

func simpleOutbound(groupID string, message string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, Message: message, ShouldSave: false}
}

func imageOutbound(groupID string, imageURLs []string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeImage, ShouldSave: false}
}

func fileOutbound(groupID string, imageURLs []string) *OutboundAction {
	return &OutboundAction{GroupID: groupID, ImageURLs: imageURLs, ImageSegmentType: napcat.SegmentTypeFile, ShouldSave: false}
}

func segmentsOutbound(groupID string, segments []napcat.MessageSegment) *OutboundAction {
	return &OutboundAction{GroupID: groupID, Segments: segments, ShouldSave: false}
}

func insufficientHistory(groupID string) *OutboundAction {
	return simpleOutbound(groupID, "历史消息不足")
}

func savedReplyOutbound(groupID string, replyMessageID string, message string) *OutboundAction {
	return &OutboundAction{
		GroupID:    groupID,
		Message:    fmt.Sprintf("[CQ:reply,id=%s]%s", replyMessageID, message),
		ShouldSave: true,
	}
}
