package main

import (
	"log"

	"njk_go/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath:       "./internal/query",
		ModelPkgPath:  "./internal/model",
		Mode:          gen.WithDefaultQuery | gen.WithQueryInterface,
		FieldNullable: true,
	})
	g.UseDB(db)
	g.ApplyBasic(g.GenerateAllTable()...)
	g.Execute()
}
