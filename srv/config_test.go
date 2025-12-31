package srv

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.APIRateLimit != 30 {
		t.Errorf("expected APIRateLimit 30, got %d", cfg.APIRateLimit)
	}
	if cfg.APIRateInterval != time.Minute {
		t.Errorf("expected APIRateInterval 1m, got %v", cfg.APIRateInterval)
	}
	if cfg.APIRateBurst != 10 {
		t.Errorf("expected APIRateBurst 10, got %d", cfg.APIRateBurst)
	}
	if cfg.SuggestionRateLimit != 15 {
		t.Errorf("expected SuggestionRateLimit 15, got %d", cfg.SuggestionRateLimit)
	}
	if cfg.SuggestionRateInterval != time.Hour {
		t.Errorf("expected SuggestionRateInterval 1h, got %v", cfg.SuggestionRateInterval)
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		"DB_PATH",
		"API_RATE_LIMIT",
		"API_RATE_INTERVAL",
		"API_RATE_BURST",
		"SUGGESTION_RATE_LIMIT",
		"SUGGESTION_RATE_INTERVAL",
	}
	original := make(map[string]string)
	for _, k := range envVars {
		original[k] = os.Getenv(k)
	}
	t.Cleanup(func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})

	// Clear env vars first
	for _, k := range envVars {
		os.Unsetenv(k)
	}

	t.Run("defaults when env empty", func(t *testing.T) {
		cfg := ConfigFromEnv()
		defaults := DefaultConfig()

		if cfg.APIRateLimit != defaults.APIRateLimit {
			t.Errorf("expected default APIRateLimit")
		}
	})

	t.Run("overrides from env", func(t *testing.T) {
		os.Setenv("DB_PATH", "custom.db")
		os.Setenv("API_RATE_LIMIT", "100")
		os.Setenv("API_RATE_INTERVAL", "30s")
		os.Setenv("API_RATE_BURST", "20")
		os.Setenv("SUGGESTION_RATE_LIMIT", "10")
		os.Setenv("SUGGESTION_RATE_INTERVAL", "2h")

		cfg := ConfigFromEnv()

		if cfg.DBPath != "custom.db" {
			t.Errorf("expected DBPath custom.db, got %s", cfg.DBPath)
		}
		if cfg.APIRateLimit != 100 {
			t.Errorf("expected APIRateLimit 100, got %d", cfg.APIRateLimit)
		}
		if cfg.APIRateInterval != 30*time.Second {
			t.Errorf("expected APIRateInterval 30s, got %v", cfg.APIRateInterval)
		}
		if cfg.APIRateBurst != 20 {
			t.Errorf("expected APIRateBurst 20, got %d", cfg.APIRateBurst)
		}
		if cfg.SuggestionRateLimit != 10 {
			t.Errorf("expected SuggestionRateLimit 10, got %d", cfg.SuggestionRateLimit)
		}
		if cfg.SuggestionRateInterval != 2*time.Hour {
			t.Errorf("expected SuggestionRateInterval 2h, got %v", cfg.SuggestionRateInterval)
		}
	})

	t.Run("invalid values use defaults", func(t *testing.T) {
		os.Setenv("API_RATE_LIMIT", "invalid")
		os.Setenv("API_RATE_INTERVAL", "bad")
		os.Setenv("API_RATE_BURST", "-5")

		cfg := ConfigFromEnv()
		defaults := DefaultConfig()

		if cfg.APIRateLimit != defaults.APIRateLimit {
			t.Errorf("expected default for invalid APIRateLimit")
		}
		if cfg.APIRateInterval != defaults.APIRateInterval {
			t.Errorf("expected default for invalid APIRateInterval")
		}
		if cfg.APIRateBurst != defaults.APIRateBurst {
			t.Errorf("expected default for invalid APIRateBurst")
		}
	})
}
