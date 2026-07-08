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
		OutPath:       "./internal/dal/query",
		ModelPkgPath:  "./internal/dal/model",
		Mode:          gen.WithDefaultQuery | gen.WithQueryInterface,
		FieldNullable: true,
	})
	g.WithImportPkgPath("github.com/pgvector/pgvector-go")
	g.WithDataTypeMap(map[string]func(columnType gorm.ColumnType) string{
		"USER-DEFINED": func(columnType gorm.ColumnType) string {
			return "pgvector.Vector"
		},
	})
	g.UseDB(db)
	g.ApplyBasic(g.GenerateAllTable(
		gen.FieldType("embedding", "pgvector.Vector"),
	)...)
	g.Execute()
}
