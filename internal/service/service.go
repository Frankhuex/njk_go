package service

import (
	"context"
	"math/rand"
	"regexp"
	"sync"
	"time"

	"njk_go/internal/client/bbh"
	"njk_go/internal/client/imagestore"
	"njk_go/internal/config"
	"njk_go/internal/napcat"
	"njk_go/internal/util/unapcat"

	"gorm.io/gorm"
)

type AICompleter interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string, temperature *float64) (string, error)
}

type Service struct {
	cfg          config.Config
	store        *Store
	aiClient     AICompleter
	freeAIClient AICompleter
	bbhClient    *bbh.BBHClient
	imageService *ImageService
	imageStore   *imagestore.ImageStoreClient
	commands     []compiledCommand
	commandMap   map[commandKey]compiledCommand
	rng          *rand.Rand
	pending      *pendingQueue
	lastAIMu     sync.Mutex
	lastAI       map[string]time.Time
}

func NewService(cfg config.Config, db *gorm.DB, aiClient AICompleter, freeAIClient AICompleter, bbhClient *bbh.BBHClient) *Service {
	defs := commandDefs(cfg.BotUserID)
	commands := make([]compiledCommand, 0, len(defs))
	commandMap := make(map[commandKey]compiledCommand, len(defs))
	store := NewStore(db)
	service := &Service{
		cfg:          cfg,
		store:        store,
		aiClient:     aiClient,
		freeAIClient: freeAIClient,
		bbhClient:    bbhClient,
		imageService: NewImageService(store),
		imageStore:   imagestore.NewClient(".", cfg.MyURL),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		pending:      &pendingQueue{},
		lastAI:       map[string]time.Time{},
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
	return s.rng.Float64() < 0.08
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
