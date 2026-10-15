package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Env            string
	HTTPAddr       string
	UniverseID     string
	DatabaseURL    string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	GameConfigPath string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	redisDB, err := strconv.Atoi(getenv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	return &Config{
		Env:            getenv("APP_ENV", "development"),
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		UniverseID:     getenv("UNIVERSE_ID", "universe_1_alpha"),
		DatabaseURL:    dbURL,
		RedisAddr:      getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        redisDB,
		GameConfigPath: getenv("GAME_CONFIG_PATH", "config/game.json"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
