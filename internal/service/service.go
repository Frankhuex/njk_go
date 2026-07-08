package service

import (
	"context"
	"regexp"
	"sync"
	"time"

	"njk_go/internal/client/bbh"
	httpclient "njk_go/internal/client/http"
	"njk_go/internal/client/imagestore"
	"njk_go/internal/client/pgstore"
	"njk_go/internal/config"
	"njk_go/internal/napcat"
	"njk_go/internal/util/unapcat"
	"njk_go/internal/util/urand"
)

type AICompleter interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string, temperature *float64) (string, error)
}

type AIClient interface {
	AICompleter
	Embed(ctx context.Context, input string) ([]float32, error)
	EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error)
}

type Service struct {
	cfg           config.Config
	store         *pgstore.Store
	aiClient      AIClient
	freeAIClient  AICompleter
	bbhClient     *bbh.BBHClient
	httpClient    *httpclient.HttpClient
	imageStore    *imagestore.ImageStoreClient
	commands      []compiledCommand
	commandMap    map[commandKey]compiledCommand
	pending       *pendingQueue
	memoryPending *pendingMemoryQueue
	lastAIMu      sync.Mutex
	lastAI        map[string]time.Time
}

func NewService(cfg config.Config, store *pgstore.Store, aiClient AIClient, freeAIClient AICompleter, bbhClient *bbh.BBHClient) *Service {
	defs := commandDefs(cfg.BotUserID)
	commands := make([]compiledCommand, 0, len(defs))
	commandMap := make(map[commandKey]compiledCommand, len(defs))
	service := &Service{
		cfg:           cfg,
		store:         store,
		aiClient:      aiClient,
		freeAIClient:  freeAIClient,
		bbhClient:     bbhClient,
		httpClient:    httpclient.NewClient(15 * time.Second),
		imageStore:    imagestore.NewClient(".", cfg.MyURL),
		pending:       &pendingQueue{},
		memoryPending: newPendingMemoryQueue(memoryBatchSize, memoryBatchMaxIdle),
		lastAI:        map[string]time.Time{},
	}
	for _, def := range defs {
		source := def.Pattern
		if def.PatternFunc != nil {
			source = def.PatternFunc(cfg.BotUserID)
		}
		command := compiledCommand{
			Key:          def.Key,
			Pattern:      regexp.MustCompile(source),
			SystemPrompt: def.SystemPrompt,
			Handler:      service.buildCommandHandler(def.Key),
		}
		commands = append(commands, command)
		commandMap[command.Key] = command
	}
	service.commands = commands
	service.commandMap = commandMap
	if service.store != nil && service.aiClient != nil {
		go service.runMemoryBatchLoop()
	}
	return service
}

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

func (s *Service) ShouldRandomReply() bool {
	return urand.Float64() < 0.08
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
