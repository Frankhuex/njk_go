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
	if len(cfg.BannedUserIDs) != 0 {
		t.Fatal("expected default banned user ids to be empty")
	}
}

func TestLoadUsesEmptyDefaultsForBotAndGroupConfig(t *testing.T) {
	keys := []string{"BOT_USER_ID", "BOT_NICKNAME", "GROUP_IDS", "BANNED_USER_IDS", "WS_ADDR"}
	previous := map[string]string{}
	for _, key := range keys {
		previous[key] = os.Getenv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unsetenv %s failed: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if previous[key] == "" {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, previous[key])
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.ListenAddr != ":11003" {
		t.Fatalf("unexpected listen addr: %s", cfg.ListenAddr)
	}
	if cfg.BotUserID != "" {
		t.Fatalf("expected empty bot user id, got: %s", cfg.BotUserID)
	}
	if cfg.BotNickname != "" {
		t.Fatalf("expected empty bot nickname, got: %s", cfg.BotNickname)
	}
	if len(cfg.AllowedGroupIDs) != 0 {
		t.Fatal("expected empty allowed group ids")
	}
	if len(cfg.BannedUserIDs) != 0 {
		t.Fatal("expected empty banned user ids")
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	cases := map[string]string{
		"":             ":11003",
		"11003":        ":11003",
		":11003":       ":11003",
		"127.0.0.1:80": "127.0.0.1:80",
	}
	for input, want := range cases {
		if got := normalizeListenAddr(input); got != want {
			t.Fatalf("normalizeListenAddr(%q) = %q, want %q", input, got, want)
		}
	}
}
