package config

import (
	"strings"
	"testing"

	"github.com/divkix/Alita_Robot/alita/utils/logredact"
)

const (
	testCaptchaBaseURL   = "https://captcha.example.com"
	testCaptchaShortName = "verify"
	testCaptchaSiteKey   = "0x4AAAAAAAsitekeyfortests"
	testCaptchaSecret    = "0x4AAAAAAAsecretkeyfortests0123456789abcd"
)

// captchaWebBaseConfig returns a config that passes ValidateConfig before any
// web captcha value is added.
func captchaWebBaseConfig() *Config {
	return &Config{
		BotToken:     "test-token",
		OwnerId:      12345,
		MessageDump:  67890,
		DatabaseURL:  "postgres://localhost/test",
		RedisAddress: "redis:6379",
		HTTPPort:     8080,
	}
}

// captchaWebFullConfig returns a base config with all four web captcha values set.
func captchaWebFullConfig() *Config {
	cfg := captchaWebBaseConfig()
	cfg.CaptchaWebBaseURL = testCaptchaBaseURL
	cfg.TelegramMiniAppShortName = testCaptchaShortName
	cfg.TurnstileSiteKey = testCaptchaSiteKey
	cfg.TurnstileSecretKey = testCaptchaSecret
	return cfg
}

// requireCaptchaWebError fails unless err names every wanted variable and
// carries none of the forbidden values.
func requireCaptchaWebError(t *testing.T, err error, mustName []string, mustNotName []string, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %v, got nil", mustName)
	}
	msg := err.Error()
	for _, name := range mustName {
		if !strings.Contains(msg, name) {
			t.Errorf("error %q does not name %s", msg, name)
		}
	}
	for _, name := range mustNotName {
		if strings.Contains(msg, name) {
			t.Errorf("error %q names %s, which is not missing", msg, name)
		}
	}
	for _, v := range values {
		if v != "" && strings.Contains(msg, v) {
			t.Errorf("error %q carries the value %q", msg, v)
		}
	}
}

// t.Setenv() is incompatible with parallel execution.
func TestCaptchaWebConfig(t *testing.T) {
	t.Run("none set", func(t *testing.T) {
		cfg := captchaWebBaseConfig()
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("an unconfigured web captcha must not fail: %v", err)
		}
		if cfg.CaptchaWebConfigured() {
			t.Error("CaptchaWebConfigured() = true with nothing set")
		}
	})

	t.Run("all four set", func(t *testing.T) {
		cfg := captchaWebFullConfig()
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("a complete set must pass: %v", err)
		}
		if !cfg.CaptchaWebConfigured() {
			t.Error("CaptchaWebConfigured() = false with all four set")
		}

		cfg = captchaWebFullConfig()
		cfg.CaptchaWebBaseURL = testCaptchaBaseURL + "/"
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("a base URL with the path / must pass: %v", err)
		}
		if !cfg.CaptchaWebConfigured() {
			t.Error("CaptchaWebConfigured() = false with the path /")
		}
	})

	t.Run("partial", func(t *testing.T) {
		names := []string{"CAPTCHA_WEB_BASE_URL", "TELEGRAM_MINI_APP_SHORT_NAME", "TURNSTILE_SITE_KEY", "TURNSTILE_SECRET_KEY"}
		checked := 0
		for mask := 1; mask < 15; mask++ {
			cfg := captchaWebBaseConfig()
			var missing, present []string
			set := []struct {
				bit   int
				field *string
				value string
			}{
				{0, &cfg.CaptchaWebBaseURL, testCaptchaBaseURL},
				{1, &cfg.TelegramMiniAppShortName, testCaptchaShortName},
				{2, &cfg.TurnstileSiteKey, testCaptchaSiteKey},
				{3, &cfg.TurnstileSecretKey, testCaptchaSecret},
			}
			for _, s := range set {
				if mask&(1<<s.bit) != 0 {
					*s.field = s.value
					present = append(present, names[s.bit])
				} else {
					missing = append(missing, names[s.bit])
				}
			}
			err := ValidateConfig(cfg)
			requireCaptchaWebError(t, err, missing, present,
				testCaptchaBaseURL, testCaptchaShortName, testCaptchaSiteKey, testCaptchaSecret)
			if cfg.CaptchaWebConfigured() {
				t.Errorf("mask %04b: CaptchaWebConfigured() = true for a partial set", mask)
			}
			checked++
		}
		if checked != 14 {
			t.Fatalf("checked %d subsets, want 14", checked)
		}
	})

	t.Run("proxies without the four", func(t *testing.T) {
		cfg := captchaWebBaseConfig()
		cfg.CaptchaWebTrustedProxies = "10.0.0.0/8"
		err := ValidateConfig(cfg)
		requireCaptchaWebError(t, err, []string{"CAPTCHA_WEB_TRUSTED_PROXIES"}, nil, "10.0.0.0/8")
	})

	t.Run("bad base URL", func(t *testing.T) {
		for name, raw := range map[string]string{
			"http scheme": "http://captcha.example.com",
			"no host":     "https://",
			"user info":   "https://user:pass@captcha.example.com",
			"query":       "https://captcha.example.com/?a=1",
			"empty query": "https://captcha.example.com?",
			"fragment":    "https://captcha.example.com/#frag",
			"path":        "https://captcha.example.com/captcha",
			"not a URL":   "captcha.example.com",
			"space":       "https://captcha example.com",
		} {
			t.Run(name, func(t *testing.T) {
				cfg := captchaWebFullConfig()
				cfg.CaptchaWebBaseURL = raw
				err := ValidateConfig(cfg)
				requireCaptchaWebError(t, err, []string{"CAPTCHA_WEB_BASE_URL"}, nil, raw)
			})
		}
	})

	t.Run("bad short name", func(t *testing.T) {
		for name, short := range map[string]string{
			"space":    "my app",
			"too long": strings.Repeat("a", 65),
			"dash":     "my-app",
			"empty":    "",
		} {
			t.Run(name, func(t *testing.T) {
				cfg := captchaWebFullConfig()
				cfg.TelegramMiniAppShortName = short
				err := ValidateConfig(cfg)
				requireCaptchaWebError(t, err, []string{"TELEGRAM_MINI_APP_SHORT_NAME"}, nil, short)
			})
		}

		cfg := captchaWebFullConfig()
		cfg.TelegramMiniAppShortName = strings.Repeat("a", 64)
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("a 64-character short name must pass: %v", err)
		}
	})

	t.Run("short secret", func(t *testing.T) {
		cfg := captchaWebFullConfig()
		cfg.TurnstileSecretKey = "abcde"
		err := ValidateConfig(cfg)
		requireCaptchaWebError(t, err, []string{"TURNSTILE_SECRET_KEY"}, nil, "abcde")

		cfg.TurnstileSecretKey = "abcdef"
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("a 6-character secret is the redactor's floor and must pass: %v", err)
		}
	})

	t.Run("bad proxy", func(t *testing.T) {
		cfg := captchaWebFullConfig()
		cfg.CaptchaWebTrustedProxies = "10.0.0.0/8, not-an-ip"
		err := ValidateConfig(cfg)
		requireCaptchaWebError(t, err, []string{"CAPTCHA_WEB_TRUSTED_PROXIES"}, nil, "not-an-ip", "10.0.0.0/8")

		cfg.CaptchaWebTrustedProxies = "10.0.0.0/8, 192.168.1.10, ::1"
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("a list of CIDRs and addresses must pass: %v", err)
		}

		cfg.CaptchaWebTrustedProxies = ""
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("an empty proxy list must pass: %v", err)
		}
	})

	t.Run("loaded from the environment", func(t *testing.T) {
		setRequiredConfigEnv(t)
		t.Setenv("CAPTCHA_WEB_BASE_URL", "  "+testCaptchaBaseURL+" ")
		t.Setenv("TELEGRAM_MINI_APP_SHORT_NAME", " "+testCaptchaShortName+"  ")
		t.Setenv("TURNSTILE_SITE_KEY", "  "+testCaptchaSiteKey)
		t.Setenv("TURNSTILE_SECRET_KEY", testCaptchaSecret+" ")
		t.Setenv("CAPTCHA_WEB_TRUSTED_PROXIES", " 10.0.0.0/8, ::1 ")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CaptchaWebBaseURL != testCaptchaBaseURL {
			t.Errorf("CaptchaWebBaseURL: got %q", cfg.CaptchaWebBaseURL)
		}
		if cfg.TelegramMiniAppShortName != testCaptchaShortName {
			t.Errorf("TelegramMiniAppShortName: got %q", cfg.TelegramMiniAppShortName)
		}
		if cfg.TurnstileSiteKey != testCaptchaSiteKey {
			t.Errorf("TurnstileSiteKey: got %q", cfg.TurnstileSiteKey)
		}
		if cfg.TurnstileSecretKey != testCaptchaSecret {
			t.Errorf("TurnstileSecretKey: got %q", cfg.TurnstileSecretKey)
		}
		if cfg.CaptchaWebTrustedProxies != "10.0.0.0/8, ::1" {
			t.Errorf("CaptchaWebTrustedProxies: got %q", cfg.CaptchaWebTrustedProxies)
		}
		if !cfg.CaptchaWebConfigured() {
			t.Error("CaptchaWebConfigured() = false after loading all four")
		}
	})

	t.Run("a partial environment stops LoadConfig", func(t *testing.T) {
		setRequiredConfigEnv(t)
		t.Setenv("CAPTCHA_WEB_BASE_URL", testCaptchaBaseURL)
		t.Setenv("TELEGRAM_MINI_APP_SHORT_NAME", "")
		t.Setenv("TURNSTILE_SITE_KEY", "")
		t.Setenv("TURNSTILE_SECRET_KEY", "")
		t.Setenv("CAPTCHA_WEB_TRUSTED_PROXIES", "")

		cfg, err := LoadConfig()
		if err == nil {
			t.Fatalf("expected a startup error, got config %+v", cfg)
		}
		requireCaptchaWebError(t, err,
			[]string{"TELEGRAM_MINI_APP_SHORT_NAME", "TURNSTILE_SITE_KEY", "TURNSTILE_SECRET_KEY"},
			[]string{"CAPTCHA_WEB_BASE_URL"}, testCaptchaBaseURL)
	})

	t.Run("the secret is registered", func(t *testing.T) {
		forgetRegisteredSecrets(t)
		registerSecrets(&Config{TurnstileSecretKey: testCaptchaSecret, TurnstileSiteKey: testCaptchaSiteKey})

		got := logredact.Scrub("secret=" + testCaptchaSecret)
		if strings.Contains(got, testCaptchaSecret) {
			t.Errorf("Turnstile secret leaked: %q", got)
		}
		if !strings.Contains(got, logredact.Placeholder) {
			t.Errorf("expected %q in %q", logredact.Placeholder, got)
		}

		site := logredact.Scrub("sitekey=" + testCaptchaSiteKey)
		if !strings.Contains(site, testCaptchaSiteKey) {
			t.Errorf("the public site key must not be scrubbed: %q", site)
		}
	})
}
