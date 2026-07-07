package service

import (
	"context"
	"regexp"

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

func (s *Service) ExecuteCommand(ctx context.Context, event *napcat.GroupMessageEvent, match *CommandMatch) (*pendingOutbound, error) {
	if match == nil {
		return nil, nil
	}
	return s.handleMatchedCommand(ctx, event, *match)
}

func (s *Service) handleMatchedCommand(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
	if match.Command.Handler == nil {
		return nil, nil
	}
	return match.Command.Handler(ctx, event, match)
}

func (s *Service) NJKCommand() *CommandMatch {
	return s.commandByKey(commandNJK)
}

func (s *Service) buildCommandHandler(key commandKey) commandHandler {
	switch key {
	case commandSummarize, commandAnalyze, commandHaiku, commandWuzhiyin, commandMost, commandVS, commandCCB, commandXmas:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleAIPromptCommand(ctx, event.GroupID.String(), match)
		}
	case commandAI:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleAICommand(ctx, event.GroupID.String(), match)
		}
	case commandNJK:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.GenerateNJKReply(ctx, event, event.GroupID.String())
		}
	case commandAIC:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleAICCommand(ctx, event.GroupID.String())
		}
	case commandReport:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleReportCommand(ctx, event.GroupID.String(), match)
		}
	case commandFace:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleFaceCommand(ctx, event.GroupID.String(), event.MessageID.String(), match)
		}
	case commandFaceID:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleFaceIDCommand(ctx, event.GroupID.String(), event.MessageID.String(), match)
		}
	case commandGetFaceID:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleGetFaceIDCommand(ctx, event.GroupID.String(), match)
		}
	case commandAllFace:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleAllFaceCommand(ctx, event.GroupID.String())
		}
	case commandJSON:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleJSONCommand(ctx, event.GroupID.String(), match)
		}
	case commandFile:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleImageToFileCommand(ctx, event.GroupID.String(), match)
		}
	case commandDice:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleDiceCommand(ctx, event.GroupID.String(), match)
		}
	case commandSymmetricLeft, commandSymmetricRight, commandSymmetricUp, commandSymmetricDown, commandSymmetricLeftUp, commandSymmetricRightUp, commandSymmetricLeftDown, commandSymmetricRightDown:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleSymmetricCommand(ctx, event.GroupID.String(), match)
		}
	case commandHelp:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return simpleOutbound(event.GroupID.String(), helpText), nil
		}
	case commandHelpBBH:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return simpleOutbound(event.GroupID.String(), helpBBHText), nil
		}
	case commandBBHPlaza, commandBBHBook, commandBBHPara, commandBBHRange, commandBBHAdd, commandBBHAI:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error) {
			return s.handleBBHCommand(ctx, event.GroupID.String(), match)
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

func (m *CommandMatch) Key() string {
	if m == nil {
		return ""
	}
	return string(m.Command.Key)
}

type commandHandler func(ctx context.Context, event *napcat.GroupMessageEvent, match CommandMatch) (*pendingOutbound, error)
