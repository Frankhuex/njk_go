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

func TestLoadIncludesDefaultBannedUserID(t *testing.T) {
	previous := os.Getenv("BANNED_USER_IDS")
	t.Cleanup(func() {
		if previous == "" {
			_ = os.Unsetenv("BANNED_USER_IDS")
			return
		}
		_ = os.Setenv("BANNED_USER_IDS", previous)
	})

	if err := os.Unsetenv("BANNED_USER_IDS"); err != nil {
		t.Fatalf("unsetenv failed: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if _, ok := cfg.BannedUserIDs["3889001802"]; !ok {
		t.Fatal("expected default banned user id to be loaded")
	}
}
