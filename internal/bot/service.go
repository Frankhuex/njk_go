package bot

import (
	"context"
	"math/rand"
	"regexp"
	"sync"
	"time"

	"njk_go/internal/bbh"
	"njk_go/internal/config"

	"gorm.io/gorm"
)

type AICompleter interface {
	Complete(ctx context.Context, systemPrompt string, userPrompt string, temperature *float64) (string, error)
}

type outboundWriter interface {
	WriteText(payload []byte) error
}

type Service struct {
	cfg          config.Config
	store        *Store
	aiClient     AICompleter
	freeAIClient AICompleter
	bbhClient    *bbh.Client
	imageService *ImageService
	commands     []compiledCommand
	commandMap   map[commandKey]compiledCommand
	rng          *rand.Rand
	pending      *pendingQueue
	lastAIMu     sync.Mutex
	lastAI       map[string]time.Time
}

func NewService(cfg config.Config, db *gorm.DB, aiClient AICompleter, freeAIClient AICompleter, bbhClient *bbh.Client) *Service {
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
