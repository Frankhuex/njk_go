package service

import (
	"context"

	"njk_go/internal/napcat"
	"njk_go/internal/util/unapcat"
)

func (s *Service) IsGroupAllowed(groupID string) bool {
	if len(s.cfg.AllowedGroupIDs) == 0 {
		return true
	}
	_, ok := s.cfg.AllowedGroupIDs[groupID]
	return ok
}

func (s *Service) IsUserBanned(userID string) bool {
	_, banned := s.cfg.BannedUserIDs[userID]
	return banned
}

func (s *Service) MentionsBot(message napcat.MessagePayload) bool {
	return unapcat.MentionsUser(message, s.cfg.BotUserID)
}

func (s *Service) MatchCommand(rawMessage string) *CommandMatch {
	return s.matchCommand(rawMessage)
}

func (s *Service) NJKCommand() *CommandMatch {
	return s.commandByKey(commandNJK)
}

func (s *Service) SaveFacesFromGroupMessage(ctx context.Context, event *napcat.GroupMessageEvent) {
	s.saveFacesFromGroupMessage(ctx, event)
}

func (s *Service) SaveIncomingMessageAndCheckImages(ctx context.Context, event *napcat.GroupMessageEvent) ([]DuplicateImage, error) {
	return s.saveIncomingMessageAndCheckImages(ctx, event)
}

func (s *Service) ExecuteCommand(ctx context.Context, event *napcat.GroupMessageEvent, match *CommandMatch) (*OutboundAction, error) {
	if match == nil {
		return nil, nil
	}
	return s.handleMatchedCommand(ctx, event, *match)
}

func (s *Service) GenerateNJKReply(ctx context.Context, event *napcat.GroupMessageEvent, groupID string) (*OutboundAction, error) {
	return s.handleNJKReply(ctx, event, groupID)
}

func (s *Service) ShouldRandomReply() bool {
	return s.rng.Float64() < 0.08
}

func (s *Service) HandleGroupMsgEmojiLikeNotice(ctx context.Context, event *napcat.NoticeEvent) {
	s.handleGroupMsgEmojiLikeNotice(ctx, event)
}

func (s *Service) CompleteActionResult(ctx context.Context, status string, retcode int, messageID string) error {
	if status != "ok" || retcode != 0 || messageID == "" {
		return nil
	}

	pending := s.pending.Pop()
	if pending == nil || !pending.ShouldSave {
		return nil
	}

	return s.saveSelfMessage(ctx, pending, messageID)
}
