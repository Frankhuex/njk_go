package main

import (
	"context"
	"log"

	"njk_go/internal/client/ai"
	"njk_go/internal/client/bbh"
	"njk_go/internal/client/pgstore"
	"njk_go/internal/config"
	"njk_go/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
		return
	}

	store, err := pgstore.InitStore(cfg.DSN())
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
		return
	}

	freeModelBaseURL := cfg.FreeModelBaseURL
	if freeModelBaseURL == "" {
		freeModelBaseURL = cfg.BaseURL
	}
	freeModelAPIKey := cfg.FreeModelAPIKey
	if freeModelAPIKey == "" {
		freeModelAPIKey = cfg.APIKey
	}

	botService := service.NewService(
		cfg,
		store,
		ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.ModelName),
		ai.NewClient(cfg.EmbedBaseURL, cfg.EmbedAPIKey, cfg.EmbedModelName),
		ai.NewClient(freeModelBaseURL, freeModelAPIKey, cfg.FreeModelName),
		bbh.NewClient(cfg.BBHBaseURL),
	)

	if err := botService.RunMemoryBackfill(context.Background()); err != nil {
		log.Fatalf("记忆回填任务失败: %v", err)
		return
	}
}
