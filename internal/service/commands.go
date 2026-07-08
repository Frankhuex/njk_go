package service

import (
	"context"
	"regexp"
	"strings"

	"njk_go/internal/napcat"
)

func (s *Service) MatchCommand(rawMessage string) *CommandMatch {
	for i := len(s.commands) - 1; i >= 0; i-- {
		groups := s.commands[i].Pattern.FindStringSubmatch(rawMessage)
		if groups != nil {
			return &CommandMatch{
				Command: s.commands[i],
				Groups:  groups,
			}
		}
	}
	return nil
}

func (s *Service) commandByKey(key commandKey) *CommandMatch {
	command, ok := s.commandMap[key]
	if !ok {
		return nil
	}
	return &CommandMatch{
		Command: command,
		Groups:  []string{},
	}
}

func (s *Service) ExecuteCommand(ctx context.Context, cmdCtx CommandContext, match *CommandMatch) (*OutboundAction, error) {
	if match == nil {
		return nil, nil
	}
	return s.handleMatchedCommand(ctx, cmdCtx, *match)
}

func (s *Service) handleMatchedCommand(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
	if match.Command.Handler == nil {
		return nil, nil
	}
	return match.Command.Handler(ctx, cmdCtx, match)
}

func (s *Service) NJKCommand() *CommandMatch {
	return s.commandByKey(commandNJK)
}

func (s *Service) buildCommandHandler(key commandKey) commandHandler {
	switch key {
	case commandSummarize, commandAnalyze, commandHaiku, commandWuzhiyin, commandMost, commandVS, commandCCB, commandXmas:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleAIPromptCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandAI:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleAICommand(ctx, cmdCtx, match)
		}
	case commandNJK:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.GenerateNJKReply(ctx, cmdCtx)
		}
	case commandAIC:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleAICCommand(ctx, cmdCtx)
		}
	case commandReport:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleReportCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandFace:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleFaceCommand(ctx, cmdCtx.GroupID, cmdCtx.MessageID, match)
		}
	case commandFaceID:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleFaceIDCommand(ctx, cmdCtx.GroupID, cmdCtx.MessageID, match)
		}
	case commandGetFaceID:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleGetFaceIDCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandAllFace:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleAllFaceCommand(ctx, cmdCtx.GroupID)
		}
	case commandJSON:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleJSONCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandFile:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleImageToFileCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandGenerateImage:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleGenerateImageCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandDice:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleDiceCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandSymmetricLeft, commandSymmetricRight, commandSymmetricUp, commandSymmetricDown, commandSymmetricLeftUp, commandSymmetricRightUp, commandSymmetricLeftDown, commandSymmetricRightDown:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleSymmetricCommand(ctx, cmdCtx.GroupID, match)
		}
	case commandHelp:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return simpleOutbound(cmdCtx.GroupID, buildHelpText(s.cfg)), nil
		}
	case commandHelpBBH:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return simpleOutbound(cmdCtx.GroupID, helpBBHText), nil
		}
	case commandBBHPlaza, commandBBHBook, commandBBHPara, commandBBHRange, commandBBHAdd, commandBBHAI:
		return func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error) {
			return s.handleBBHCommand(ctx, cmdCtx.GroupID, match)
		}
	default:
		return nil
	}
}

type compiledCommand struct {
	Key          commandKey
	Pattern      *regexp.Regexp
	SystemPrompt string
	Handler      commandHandler
}

type CommandMatch struct {
	Command compiledCommand
	Groups  []string
}

type CommandContext struct {
	GroupID    string
	MessageID  string
	SenderID   string
	RawMessage string
}

func CommandContextFromGroupMessageEvent(event *napcat.GroupMessageEvent) CommandContext {
	if event == nil {
		return CommandContext{}
	}
	senderID := strings.TrimSpace(event.Sender.UserID.String())
	if senderID == "" {
		senderID = strings.TrimSpace(event.UserID.String())
	}
	return CommandContext{
		GroupID:    event.GroupID.String(),
		MessageID:  event.MessageID.String(),
		SenderID:   senderID,
		RawMessage: strings.TrimSpace(event.RawMessage),
	}
}

func (m *CommandMatch) Key() string {
	if m == nil {
		return ""
	}
	return string(m.Command.Key)
}

type commandHandler func(ctx context.Context, cmdCtx CommandContext, match CommandMatch) (*OutboundAction, error)
