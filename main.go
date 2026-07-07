package main

import (
	"log"

	"njk_go/internal/ai"
	"njk_go/internal/bbh"
	"njk_go/internal/config"
	"njk_go/internal/service"
	"njk_go/internal/transport/ws"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	botService := service.NewService(
		cfg,
		db,
		ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.ModelName),
		ai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.FreeModelName),
		bbh.NewClient(cfg.BBHBaseURL),
	)

	server := ws.NewServer(cfg.ListenAddr, botService)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("WebSocket 服务启动失败: %v", err)
	}
}
