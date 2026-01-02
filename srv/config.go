package srv

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configurable server settings.
type Config struct {
	// Database
	DBPath string

	// Server
	Hostname    string
	AdminEmails []string

	// API Rate Limiting
	APIRateLimit    int           // requests per interval
	APIRateInterval time.Duration // interval for rate limit
	APIRateBurst    int           // max burst capacity

	// Suggestion Rate Limiting
	SuggestionRateLimit    int           // suggestions per interval per IP/channel
	SuggestionRateInterval time.Duration // interval for suggestion rate limit

	// Nightbot OAuth
	NightbotClientID     string
	NightbotClientSecret string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DBPath:   "db.sqlite3",
		Hostname: "localhost",

		// API: 30 requests per minute, burst of 10
		APIRateLimit:    30,
		APIRateInterval: time.Minute,
		APIRateBurst:    10,

		// Suggestions: 15 per hour
		SuggestionRateLimit:    15,
		SuggestionRateInterval: time.Hour,
	}
}

// ConfigFromEnv returns a Config populated from environment variables,
// falling back to defaults for unset values.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	if v := os.Getenv("API_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.APIRateLimit = n
		}
	}

	if v := os.Getenv("API_RATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.APIRateInterval = d
		}
	}

	if v := os.Getenv("API_RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.APIRateBurst = n
		}
	}

	if v := os.Getenv("SUGGESTION_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.SuggestionRateLimit = n
		}
	}

	if v := os.Getenv("SUGGESTION_RATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SuggestionRateInterval = d
		}
	}

	cfg.NightbotClientID = os.Getenv("NIGHTBOT_CLIENT_ID")
	cfg.NightbotClientSecret = os.Getenv("NIGHTBOT_CLIENT_SECRET")

	return cfg
}
