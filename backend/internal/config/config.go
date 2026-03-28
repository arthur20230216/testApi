package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	ProbeTimeout           time.Duration
	AllowOrigin            string
	ChannelAuditEnabled    bool
	ChannelAuditTimeout    time.Duration
	OpenAIAPIKey           string
	OpenAIBaseURL          string
	OpenAIModel            string
	AdminInitUsername      string
	AdminInitPassword      string
	AdminSessionTTL        time.Duration
	AdminSessionCookieName string
}

func Load() Config {
	return Config{
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/modelprobe?sslmode=disable"),
		ProbeTimeout:           time.Duration(getEnvInt("PROBE_TIMEOUT_MS", 10000)) * time.Millisecond,
		AllowOrigin:            getEnv("ALLOW_ORIGIN", "*"),
		ChannelAuditEnabled:    getEnvBool("CHANNEL_AUDIT_ENABLED", false),
		ChannelAuditTimeout:    time.Duration(getEnvInt("CHANNEL_AUDIT_TIMEOUT_MS", 15000)) * time.Millisecond,
		OpenAIAPIKey:           getEnv("OPENAI_API_KEY", ""),
		OpenAIBaseURL:          getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:            getEnv("OPENAI_MODEL", ""),
		AdminInitUsername:      getEnv("ADMIN_INIT_USERNAME", ""),
		AdminInitPassword:      getEnv("ADMIN_INIT_PASSWORD", ""),
		AdminSessionTTL:        time.Duration(getEnvInt("ADMIN_SESSION_TTL_HOURS", 168)) * time.Hour,
		AdminSessionCookieName: getEnv("ADMIN_SESSION_COOKIE_NAME", "modelprobe_admin_session"),
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

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}
