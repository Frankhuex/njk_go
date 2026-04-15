package config

import (
	"os"
	"testing"
)

func TestLoadOverridesLegacyPostgresUser(t *testing.T) {
	previous := os.Getenv("DB_USER")
	t.Cleanup(func() {
		if previous == "" {
			_ = os.Unsetenv("DB_USER")
			return
		}
		_ = os.Setenv("DB_USER", previous)
	})

	if err := os.Setenv("DB_USER", "postgres"); err != nil {
		t.Fatalf("setenv failed: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.DBUser != "njk" {
		t.Fatalf("unexpected db user: %s", cfg.DBUser)
	}
}
