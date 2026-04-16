package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr      string
	DBHost          string
	DBPort          int
	DBUser          string
	DBPassword      string
	DBName          string
	APIKey          string
	BaseURL         string
	ModelName       string
	FreeModelName   string
	BBHBaseURL      string
	BotUserID       string
	BotNickname     string
	AllowedGroupIDs map[string]struct{}
	BannedUserIDs   map[string]struct{}
}

func Load() (Config, error) {
	values := map[string]string{}
	for _, path := range candidateEnvFiles() {
		fileValues, err := readEnvFile(path)
		if err != nil {
			return Config{}, err
		}
		for k, v := range fileValues {
			if _, ok := values[k]; !ok {
				values[k] = v
			}
		}
	}

	cfg := Config{
		ListenAddr:    value("WS_ADDR", values, ":11003"),
		DBHost:        value("DB_HOST", values, "localhost"),
		DBUser:        value("DB_USER", values, "njk"),
		DBPassword:    value("DB_PWD", values, ""),
		DBName:        value("DB_NAME", values, "njk"),
		APIKey:        value("API_KEY", values, ""),
		BaseURL:       strings.TrimRight(value("BASE_URL", values, ""), "/"),
		ModelName:     value("MODEL_NAME", values, ""),
		FreeModelName: value("FREE_MODEL_NAME", values, ""),
		BBHBaseURL:    strings.TrimRight(value("BBH_BASE_URL", values, "http://106.13.161.72:10000/api"), "/"),
		BotUserID:     value("BOT_USER_ID", values, "1558109748"),
		BotNickname:   value("BOT_NICKNAME", values, "你居垦"),
	}

	port, err := strconv.Atoi(value("DB_PORT", values, "5432"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid DB_PORT: %w", err)
	}
	cfg.DBPort = port
	cfg.AllowedGroupIDs = parseGroupIDs(value("GROUP_IDS", values, "897830548,979088841,1050660050,665074632,238872980,876274089"))
	cfg.BannedUserIDs = parseIDSet(value("BANNED_USER_IDS", values, "3889001802"))
	if cfg.DBUser == "postgres" {
		cfg.DBUser = "njk"
	}

	return cfg, nil
}

func (c Config) DSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", c.DBHost, c.DBUser, c.DBPassword, c.DBName, c.DBPort)
}

func candidateEnvFiles() []string {
	paths := []string{}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths,
			filepath.Join(cwd, ".env"),
			filepath.Join(cwd, "..", "NJK", ".env"),
			filepath.Join(cwd, "..", ".env"),
		)
	}
	return paths
}

func readEnvFile(path string) (map[string]string, error) {
	result := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		result[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func value(key string, fileValues map[string]string, fallback string) string {
	if env := strings.TrimSpace(os.Getenv(key)); env != "" {
		return env
	}
	if fromFile := strings.TrimSpace(fileValues[key]); fromFile != "" {
		return fromFile
	}
	return fallback
}

func parseGroupIDs(raw string) map[string]struct{} {
	return parseIDSet(raw)
}

func parseIDSet(raw string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		result[id] = struct{}{}
	}
	return result
}
