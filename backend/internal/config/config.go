package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	DatabaseURL  string
	ProbeTimeout time.Duration
	AllowOrigin  string
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/modelprobe?sslmode=disable"),
		ProbeTimeout: time.Duration(getEnvInt("PROBE_TIMEOUT_MS", 10000)) * time.Millisecond,
		AllowOrigin:  getEnv("ALLOW_ORIGIN", "*"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
