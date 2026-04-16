package bot

import (
	"context"
	"regexp"

	"njk_go/internal/napcat"
)

func (s *Service) matchCommand(rawMessage string) *matchedCommand {
	for i := len(s.commands) - 1; i >= 0; i-- {
		groups := s.commands[i].Pattern.FindStringSubmatch(rawMessage)
		if groups != nil {
			return &matchedCommand{
				Command: s.commands[i],
				Groups:  groups,
			}
		}
	}
	return nil
}

func (s *Service) commandByKey(key commandKey) *matchedCommand {
	command, ok := s.commandMap[key]
	if !ok {
		return nil
	}
	return &matchedCommand{
		Command: command,
		Groups:  []string{},
	}
}

func (s *Service) handleMatchedCommand(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
	if match.Command.Handler == nil {
		return nil, nil
	}
	return match.Command.Handler(ctx, event, match)
}

func (s *Service) buildCommandHandler(key commandKey) commandHandler {
	switch key {
	case commandSummarize, commandAnalyze, commandHaiku, commandWuzhiyin, commandMost, commandVS, commandCCB, commandXmas:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return s.handleAIPromptCommand(ctx, event.GroupID.String(), match)
		}
	case commandAI:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return s.handleAICommand(ctx, event.GroupID.String(), match)
		}
	case commandNJK:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return s.handleNJKReply(ctx, event, event.GroupID.String())
		}
	case commandAIC:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return s.handleAICCommand(ctx, event.GroupID.String())
		}
	case commandReport:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return s.handleReportCommand(ctx, event.GroupID.String(), match)
		}
	case commandHelp:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return simpleOutbound(event.GroupID.String(), helpText), nil
		}
	case commandHelpBBH:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
			return simpleOutbound(event.GroupID.String(), helpBBHText), nil
		}
	case commandBBHPlaza, commandBBHBook, commandBBHPara, commandBBHRange, commandBBHAdd, commandBBHAI:
		return func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error) {
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

type matchedCommand struct {
	Command compiledCommand
	Groups  []string
}

type commandHandler func(ctx context.Context, event *napcat.GroupMessageEvent, match matchedCommand) (*pendingOutbound, error)
