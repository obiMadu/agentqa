package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                        string
	DatabaseURL                 string
	DBConnectRetries            int
	DBConnectDelay              time.Duration
	APIKeyEncryptionKey         string
	OIDCIssuer                  string
	OIDCAudiences               []string
	OIDCTimeout                 time.Duration
	SuperwallWebhookSecret      string
	SubscriptionOverridePlan    string
	SubscriptionOverrideUserIDs []string
	ExpoPushURL                 string
	QuestionTTL                 time.Duration
}

func Load() Config {
	return Config{
		Addr:                        envOr("ADDR", ":8080"),
		DatabaseURL:                 os.Getenv("DATABASE_URL"),
		DBConnectRetries:            envInt("DB_CONNECT_RETRIES", 12),
		DBConnectDelay:              envDuration("DB_CONNECT_DELAY", 2*time.Second),
		APIKeyEncryptionKey:         os.Getenv("API_KEY_ENCRYPTION_KEY"),
		OIDCIssuer:                  os.Getenv("OIDC_ISSUER"),
		OIDCAudiences:               envCSV("OIDC_AUDIENCE"),
		OIDCTimeout:                 envDuration("OIDC_TIMEOUT", 5*time.Second),
		SuperwallWebhookSecret:      os.Getenv("SUPERWALL_WEBHOOK_SECRET"),
		SubscriptionOverridePlan:    strings.TrimSpace(os.Getenv("SUBSCRIPTION_OVERRIDE_PLAN")),
		SubscriptionOverrideUserIDs: envCSV("SUBSCRIPTION_OVERRIDE_USER_IDS"),
		ExpoPushURL:                 strings.TrimSpace(os.Getenv("EXPO_PUSH_URL")),
		QuestionTTL:                 envDuration("QUESTION_TTL", 7*24*time.Hour),
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envCSV(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
