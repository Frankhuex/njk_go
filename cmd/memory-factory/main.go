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

	botService := service.NewService(
		cfg,
		store,
		ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.ModelName, cfg.EmbedModelName),
		ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.FreeModelName, cfg.EmbedModelName),
		bbh.NewClient(cfg.BBHBaseURL),
	)

	if err := botService.RunMemoryBackfill(context.Background()); err != nil {
		log.Fatalf("记忆回填任务失败: %v", err)
		return
	}
}
