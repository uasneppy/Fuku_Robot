package config

import (
	"strings"
	"testing"
)

// TestDefaultTimezone holds DEFAULT_TIMEZONE: unset means Europe/Kyiv, a valid name is loaded, and an
// invalid one stops startup with a message that names the variable.
func TestDefaultTimezone(t *testing.T) {
	cfg := captchaWebBaseConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if got := cfg.DefaultLocation.String(); got != "Europe/Kyiv" {
		t.Errorf("unset loads %q, want Europe/Kyiv", got)
	}
	cfg = captchaWebBaseConfig()
	cfg.DefaultTimezone = "UTC"
	if err := ValidateConfig(cfg); err != nil || cfg.DefaultLocation.String() != "UTC" {
		t.Errorf("UTC: %v, %v", cfg.DefaultLocation, err)
	}
	for _, bad := range []string{"Mars/Olympus", "Local", "kyiv"} {
		cfg = captchaWebBaseConfig()
		cfg.DefaultTimezone = bad
		err := ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "DEFAULT_TIMEZONE") {
			t.Errorf("%q: error %v, want one naming DEFAULT_TIMEZONE", bad, err)
		}
	}
}
