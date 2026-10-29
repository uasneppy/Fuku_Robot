package config

import (
	"fmt"
	"os"
	"strings"
	"testing"

	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
	"github.com/divkix/Alita_Robot/alita/utils/logredact"
)

// t.Parallel() because it mutates os.Args and must run sequentially.
func TestIsCliModeActive(t *testing.T) {
	saveArgs := os.Args
	defer func() { os.Args = saveArgs }()

	t.Run("no args returns false", func(t *testing.T) {
		os.Args = []string{"binary"}
		if isCliModeActive() {
			t.Errorf("isCliModeActive() = true, want false")
		}
	})

	t.Run("single positional arg returns false", func(t *testing.T) {
		os.Args = []string{"binary", "start"}
		if isCliModeActive() {
			t.Errorf("isCliModeActive() = true, want false")
		}
	})

	t.Run("--version returns true", func(t *testing.T) {
		os.Args = []string{"binary", "--version"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})

	t.Run("-v returns true", func(t *testing.T) {
		os.Args = []string{"binary", "-v"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})

	t.Run("--health returns true", func(t *testing.T) {
		os.Args = []string{"binary", "--health"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})

	t.Run("mixed args with flag returns true", func(t *testing.T) {
		os.Args = []string{"binary", "run", "--version"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})

	t.Run("-version returns true", func(t *testing.T) {
		os.Args = []string{"binary", "-version"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})

	t.Run("-health returns true", func(t *testing.T) {
		os.Args = []string{"binary", "-health"}
		if !isCliModeActive() {
			t.Errorf("isCliModeActive() = false, want true")
		}
	})
}

// t.Parallel() because t.Setenv() is incompatible with parallel execution.
func TestLoadConfig(t *testing.T) {
	t.Run("returns error when required fields missing", func(t *testing.T) {
		t.Setenv("BOT_TOKEN", "")
		t.Setenv("OWNER_ID", "")
		t.Setenv("MESSAGE_DUMP", "")
		t.Setenv("DATABASE_URL", "")
		t.Setenv("REDIS_ADDRESS", "")
		t.Setenv("REDIS_URL", "")

		_, err := LoadConfig()
		if err == nil {
			t.Fatalf("expected error for missing required fields, got nil")
		}
	})

	t.Run("loads config with all required env vars", func(t *testing.T) {
		t.Setenv("BOT_TOKEN", "test-token")
		t.Setenv("OWNER_ID", "12345")
		t.Setenv("MESSAGE_DUMP", "67890")
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("REDIS_ADDRESS", "redis:6379")
		t.Setenv("REDIS_PASSWORD", "")
		t.Setenv("REDIS_URL", "")
		t.Setenv("HTTP_PORT", "9090")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.BotToken != "test-token" {
			t.Errorf("BotToken: got %q, want %q", cfg.BotToken, "test-token")
		}
		if cfg.OwnerId != 12345 {
			t.Errorf("OwnerId: got %d, want %d", cfg.OwnerId, 12345)
		}
		if cfg.MessageDump != 67890 {
			t.Errorf("MessageDump: got %d, want %d", cfg.MessageDump, 67890)
		}
		if cfg.DatabaseURL != "postgres://localhost/test" {
			t.Errorf("DatabaseURL: got %q, want %q", cfg.DatabaseURL, "postgres://localhost/test")
		}
		if cfg.RedisAddress != "redis:6379" {
			t.Errorf("RedisAddress: got %q, want %q", cfg.RedisAddress, "redis:6379")
		}
		if cfg.HTTPPort != 9090 {
			t.Errorf("HTTPPort: got %d, want %d", cfg.HTTPPort, 9090)
		}
		if cfg.ApiServer != "https://api.telegram.org" {
			t.Errorf("ApiServer: got %q, want %q", cfg.ApiServer, "https://api.telegram.org")
		}
		if cfg.BotVersion == "" {
			t.Errorf("BotVersion: got empty string, want non-empty")
		}
		if len(cfg.AllowedUpdates) == 0 {
			t.Errorf("AllowedUpdates: expected non-empty slice")
		}
		if len(cfg.ValidLangCodes) != 1 || cfg.ValidLangCodes[0] != "en" {
			t.Errorf("ValidLangCodes: got %v, want [en]", cfg.ValidLangCodes)
		}
	})

	t.Run("webhook config loaded correctly", func(t *testing.T) {
		t.Setenv("BOT_TOKEN", "tk")
		t.Setenv("OWNER_ID", "1")
		t.Setenv("MESSAGE_DUMP", "1")
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("REDIS_ADDRESS", "localhost:6379")
		t.Setenv("USE_WEBHOOKS", "true")
		t.Setenv("WEBHOOK_DOMAIN", "example.com")
		t.Setenv("WEBHOOK_SECRET", "shh")
		t.Setenv("HTTP_PORT", "8080")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.UseWebhooks {
			t.Errorf("UseWebhooks: got false, want true")
		}
		if cfg.WebhookDomain != "example.com" {
			t.Errorf("WebhookDomain: got %q, want %q", cfg.WebhookDomain, "example.com")
		}
		if cfg.WebhookSecret != "shh" {
			t.Errorf("WebhookSecret: got %q, want %q", cfg.WebhookSecret, "shh")
		}
	})

	t.Run("ENABLE_PPROF parsed as bool", func(t *testing.T) {
		t.Setenv("BOT_TOKEN", "tk")
		t.Setenv("OWNER_ID", "1")
		t.Setenv("MESSAGE_DUMP", "1")
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("REDIS_ADDRESS", "localhost:6379")
		t.Setenv("ENABLE_PPROF", "yes")
		t.Setenv("HTTP_PORT", "8080")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.EnablePPROF {
			t.Errorf("EnablePPROF: got false, want true")
		}
	})

	t.Run("local cache settings", func(t *testing.T) {
		cases := []struct {
			name, ttl, maxEntries string
			wantTTL, wantMax      int
		}{
			{name: "unset uses defaults", wantTTL: 10, wantMax: 50000},
			{name: "explicit zero disables", ttl: "0", wantTTL: 0, wantMax: 50000},
			{name: "typo falls back to default", ttl: "ten", maxEntries: "lots", wantTTL: 10, wantMax: 50000},
			{name: "explicit values kept", ttl: "30", maxEntries: "1000", wantTTL: 30, wantMax: 1000},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("BOT_TOKEN", "tk")
				t.Setenv("OWNER_ID", "1")
				t.Setenv("MESSAGE_DUMP", "1")
				t.Setenv("DATABASE_URL", "postgres://localhost/test")
				t.Setenv("REDIS_ADDRESS", "localhost:6379")
				t.Setenv("HTTP_PORT", "8080")
				t.Setenv("CACHE_LOCAL_TTL", tc.ttl)
				t.Setenv("CACHE_LOCAL_MAX_ENTRIES", tc.maxEntries)

				cfg, err := LoadConfig()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.CacheLocalTTLSeconds != tc.wantTTL {
					t.Errorf("CacheLocalTTLSeconds: got %d, want %d", cfg.CacheLocalTTLSeconds, tc.wantTTL)
				}
				if cfg.CacheLocalMaxEntries != tc.wantMax {
					t.Errorf("CacheLocalMaxEntries: got %d, want %d", cfg.CacheLocalMaxEntries, tc.wantMax)
				}
			})
		}
	})

	t.Run("out-of-range CACHE_LOCAL_TTL is rejected", func(t *testing.T) {
		t.Setenv("BOT_TOKEN", "tk")
		t.Setenv("OWNER_ID", "1")
		t.Setenv("MESSAGE_DUMP", "1")
		t.Setenv("DATABASE_URL", "postgres://localhost/test")
		t.Setenv("REDIS_ADDRESS", "localhost:6379")
		t.Setenv("HTTP_PORT", "8080")
		t.Setenv("CACHE_LOCAL_TTL", "301")

		if _, err := LoadConfig(); err == nil {
			t.Fatal("LoadConfig() error = nil, want CACHE_LOCAL_TTL range error")
		}
	})
}

func TestValidateConfigPure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr string
	}{
		{name: "valid base config", setup: func(_ *Config) {}},
		{
			name:    "missing bot token",
			setup:   func(c *Config) { c.BotToken = "" },
			wantErr: "BOT_TOKEN is required",
		},
		{
			name:    "missing owner",
			setup:   func(c *Config) { c.OwnerId = 0 },
			wantErr: "OWNER_ID is required",
		},
		{
			name:    "missing message dump",
			setup:   func(c *Config) { c.MessageDump = 0 },
			wantErr: "MESSAGE_DUMP is required",
		},
		{
			name:    "missing database URL",
			setup:   func(c *Config) { c.DatabaseURL = "" },
			wantErr: "DATABASE_URL is required",
		},
		{
			name:    "missing redis address",
			setup:   func(c *Config) { c.RedisAddress = "" },
			wantErr: "REDIS_ADDRESS or REDIS_URL is required",
		},
		{
			name: "webhook requires domain",
			setup: func(c *Config) {
				c.UseWebhooks = true
				c.WebhookDomain = ""
				c.WebhookSecret = "secret"
			},
			wantErr: "WEBHOOK_DOMAIN is required",
		},
		{
			name: "webhook requires secret",
			setup: func(c *Config) {
				c.UseWebhooks = true
				c.WebhookDomain = "https://example.com"
				c.WebhookSecret = ""
			},
			wantErr: "WEBHOOK_SECRET is required",
		},
		{
			name:    "invalid HTTP port",
			setup:   func(c *Config) { c.HTTPPort = 70000 },
			wantErr: "HTTP_PORT must be between 1 and 65535",
		},
		{
			name:    "invalid dispatcher routines",
			setup:   func(c *Config) { c.DispatcherMaxRoutines = 1001 },
			wantErr: "DISPATCHER_MAX_ROUTINES must be between 1 and 1000",
		},
		{
			name:    "negative local cache TTL",
			setup:   func(c *Config) { c.CacheLocalTTLSeconds = -1 },
			wantErr: "CACHE_LOCAL_TTL must be between 0 and 300 seconds",
		},
		{
			name:    "local cache TTL too long",
			setup:   func(c *Config) { c.CacheLocalTTLSeconds = 301 },
			wantErr: "CACHE_LOCAL_TTL must be between 0 and 300 seconds",
		},
		{
			name:  "local cache TTL at bounds",
			setup: func(c *Config) { c.CacheLocalTTLSeconds = 300 },
		},
		{
			name:    "local cache too small",
			setup:   func(c *Config) { c.CacheLocalMaxEntries = 99 },
			wantErr: "CACHE_LOCAL_MAX_ENTRIES must be between 100 and 1000000",
		},
		{
			name:    "local cache too large",
			setup:   func(c *Config) { c.CacheLocalMaxEntries = 1000001 },
			wantErr: "CACHE_LOCAL_MAX_ENTRIES must be between 100 and 1000000",
		},
		{
			name:    "invalid idle connections",
			setup:   func(c *Config) { c.DBMaxIdleConns = 101 },
			wantErr: "DB_MAX_IDLE_CONNS must be between 1 and 100",
		},
		{
			name:    "invalid open connections",
			setup:   func(c *Config) { c.DBMaxOpenConns = 1001 },
			wantErr: "DB_MAX_OPEN_CONNS must be between 1 and 1000",
		},
		{
			name:    "invalid connection lifetime",
			setup:   func(c *Config) { c.DBConnMaxLifetimeMin = 1441 },
			wantErr: "DB_CONN_MAX_LIFETIME_MIN must be between 1 and 1440 minutes",
		},
		{
			name:    "invalid idle time",
			setup:   func(c *Config) { c.DBConnMaxIdleTimeMin = 61 },
			wantErr: "DB_CONN_MAX_IDLE_TIME_MIN must be between 1 and 60 minutes",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validBaseConfig()
			cfg.DispatcherMaxRoutines = 200
			cfg.DBMaxIdleConns = 50
			cfg.DBMaxOpenConns = 200
			cfg.DBConnMaxLifetimeMin = 240
			cfg.DBConnMaxIdleTimeMin = 60
			tc.setup(cfg)

			err := ValidateConfig(cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateConfig() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateConfig() error = nil, want %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateConfig() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestRedisEnvParsingPure(t *testing.T) {
	tests := []struct {
		name         string
		redisAddr    string
		redisPass    string
		redisURL     string
		wantAddr     string
		wantPassword string
	}{
		{
			name:         "explicit address and password take precedence",
			redisAddr:    "redis.internal:6379",
			redisPass:    "direct-password",
			redisURL:     "redis://:url-password@example.com:6380",
			wantAddr:     "redis.internal:6379",
			wantPassword: "direct-password",
		},
		{
			name:      "explicit address ignores URL credentials",
			redisAddr: "redis.internal:6379",
			redisURL:  "redis://:stale@example.com:6380",
			wantAddr:  "redis.internal:6379",
		},
		{
			name:         "url supplies host and password",
			redisURL:     "redis://:secret@example.com:6380/1",
			wantAddr:     "example.com:6380",
			wantPassword: "secret",
		},
		{
			name:     "invalid url returns empty values",
			redisURL: "://bad-url",
		},
		{
			name: "empty env returns empty values",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REDIS_ADDRESS", tc.redisAddr)
			t.Setenv("REDIS_PASSWORD", tc.redisPass)
			t.Setenv("REDIS_URL", tc.redisURL)

			if got := getRedisAddress(); got != tc.wantAddr {
				t.Fatalf("getRedisAddress() = %q, want %q", got, tc.wantAddr)
			}
			if got := getRedisPassword(); got != tc.wantPassword {
				t.Fatalf("getRedisPassword() = %q, want %q", got, tc.wantPassword)
			}
		})
	}
}

// setRequiredConfigEnv sets the variables LoadConfig cannot start without.
func setRequiredConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("OWNER_ID", "12345")
	t.Setenv("MESSAGE_DUMP", "67890")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_ADDRESS", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_URL", "")
}

// t.Setenv() is incompatible with parallel execution.
func TestGeminiConfig(t *testing.T) {
	t.Run("key and model load", func(t *testing.T) {
		setRequiredConfigEnv(t)
		t.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyForConfig_0123456789abcdefg")
		t.Setenv("GEMINI_MODEL", "gemini-3.5-flash")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GeminiAPIKey != "AIzaSyTestKeyForConfig_0123456789abcdefg" {
			t.Errorf("GeminiAPIKey: got %q", cfg.GeminiAPIKey)
		}
		if cfg.GeminiModel != "gemini-3.5-flash" {
			t.Errorf("GeminiModel: got %q, want %q", cfg.GeminiModel, "gemini-3.5-flash")
		}
	})

	t.Run("model defaults", func(t *testing.T) {
		setRequiredConfigEnv(t)
		t.Setenv("GEMINI_API_KEY", "")
		t.Setenv("GEMINI_MODEL", "")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("a missing key must not fail LoadConfig: %v", err)
		}
		if cfg.GeminiAPIKey != "" {
			t.Errorf("GeminiAPIKey: got %q, want empty", cfg.GeminiAPIKey)
		}
		if cfg.GeminiModel != gemini.DefaultModel {
			t.Errorf("GeminiModel: got %q, want %q", cfg.GeminiModel, gemini.DefaultModel)
		}
	})

	t.Run("bad model falls back", func(t *testing.T) {
		for name, model := range map[string]string{
			"path traversal": "../v1/files",
			"too long":       strings.Repeat("a", 65),
			"query":          "gemini?key=x",
		} {
			t.Run(name, func(t *testing.T) {
				setRequiredConfigEnv(t)
				t.Setenv("GEMINI_MODEL", model)

				cfg, err := LoadConfig()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.GeminiModel != gemini.DefaultModel {
					t.Errorf("GeminiModel: got %q, want %q", cfg.GeminiModel, gemini.DefaultModel)
				}
			})
		}
	})

	t.Run("a rejected model is not logged", func(t *testing.T) {
		const pasted = "AIzaSyPastedIntoTheWrongVariable 0123456789"
		hook := logrustest.NewGlobal()
		defer hook.Reset()
		setRequiredConfigEnv(t)
		t.Setenv("GEMINI_MODEL", pasted)

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GeminiModel != gemini.DefaultModel {
			t.Fatalf("GeminiModel: got %q, want the default", cfg.GeminiModel)
		}
		warned := false
		for _, entry := range hook.AllEntries() {
			line := entry.Message
			for _, value := range entry.Data {
				line += " " + fmt.Sprint(value)
			}
			if strings.Contains(line, pasted) || strings.Contains(line, "AIzaSy") {
				t.Errorf("a log line carries the rejected GEMINI_MODEL value: %q", line)
			}
			if strings.Contains(entry.Message, "GEMINI_MODEL") {
				warned = true
			}
		}
		if !warned {
			t.Error("no warning names GEMINI_MODEL: the case would prove nothing")
		}
	})

	t.Run("the key is registered", func(t *testing.T) {
		const key = "AIzaSyRegisteredSecretForTest_abcdefghij"
		const botToken = "registersecretsbottoken"
		registerSecrets(&Config{GeminiAPIKey: key, BotToken: botToken})

		got := logredact.Scrub("calling model with key=" + key + " and " + botToken)
		if strings.Contains(got, key) {
			t.Errorf("Gemini key leaked: %q", got)
		}
		if strings.Contains(got, botToken) {
			t.Errorf("bot token no longer registered: %q", got)
		}
		if !strings.Contains(got, logredact.Placeholder) {
			t.Errorf("expected %q in %q", logredact.Placeholder, got)
		}

		// A value the AIza shape pattern cannot match proves the exact-value
		// registration on its own.
		const plain = "plain-gemini-key-registered-by-exact-value"
		registerSecrets(&Config{GeminiAPIKey: plain})
		if got := logredact.Scrub("key=" + plain); strings.Contains(got, plain) {
			t.Errorf("exact-value registration missing: %q", got)
		}
	})
}
