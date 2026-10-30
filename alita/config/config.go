package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/claude"
	"github.com/divkix/Alita_Robot/alita/utils/logredact"
)

func isCliModeActive() bool {
	if len(os.Args) < 2 {
		return false
	}

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "-version", "-v", "--health", "-health":
			return true
		}
	}
	return false
}

// REDIS_URL format: redis://user:password@host:port (standard Redis URL format)
func getRedisAddress() string {
	if addr := os.Getenv("REDIS_ADDRESS"); addr != "" {
		return addr
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return ""
	}

	parsed, err := url.Parse(redisURL)
	if err != nil {
		log.Warnf("Failed to parse REDIS_URL: %v", err)
		return ""
	}

	return parsed.Host
}

func getRedisURL() string {
	if os.Getenv("REDIS_ADDRESS") != "" {
		return ""
	}
	return os.Getenv("REDIS_URL")
}

func getRedisPassword() string {
	if pass := os.Getenv("REDIS_PASSWORD"); pass != "" {
		return pass
	}
	if os.Getenv("REDIS_ADDRESS") != "" {
		return ""
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return ""
	}

	parsed, err := url.Parse(redisURL)
	if err != nil {
		return ""
	}

	if parsed.User != nil {
		pass, _ := parsed.User.Password()
		return pass
	}

	return ""
}

func parseRedisDB() (int, error) {
	raw := os.Getenv("REDIS_DB")
	if raw == "" {
		return 1, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("REDIS_DB must be an integer 0-15")
	}
	if v < 0 || v > 15 {
		return 0, fmt.Errorf("REDIS_DB must be an integer 0-15")
	}
	return v, nil
}

func getHTTPPort() int {
	value := os.Getenv("HTTP_PORT")
	if value == "" {
		value = os.Getenv("PORT")
	}
	return typeConvertor{str: value}.Int()
}

// Local read-through cache defaults (CACHE_LOCAL_TTL, CACHE_LOCAL_MAX_ENTRIES).
const (
	DefaultCacheLocalTTLSeconds = 10
	DefaultCacheLocalMaxEntries = 50000
)

// getCacheLocalTTL reads CACHE_LOCAL_TTL. Unlike other integers, 0 is a real
// setting (layer off), so only an unset or unparsable value falls back to the
// default; a typo must not silently disable the layer.
func getCacheLocalTTL() int {
	raw := os.Getenv("CACHE_LOCAL_TTL")
	if raw == "" {
		return DefaultCacheLocalTTLSeconds
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.WithError(err).WithField("value", raw).Warn("Failed to convert CACHE_LOCAL_TTL to int, using default")
		return DefaultCacheLocalTTLSeconds
	}
	return v
}

type Config struct {
	BotToken    string `validate:"required"`
	BotVersion  string
	ApiServer   string
	WorkingMode string
	Debug       bool

	OwnerId            int64 `validate:"required,min=1"`
	MessageDump        int64 `validate:"required,min=1"`
	DropPendingUpdates bool
	AllowedUpdates     []string
	ValidLangCodes     []string

	DatabaseURL string `validate:"required"`

	DBMaxIdleConns       int `validate:"min=1,max=100"`
	DBMaxOpenConns       int `validate:"min=1,max=1000"`
	DBConnMaxLifetimeMin int `validate:"min=1,max=1440"`
	DBConnMaxIdleTimeMin int `validate:"min=1,max=60"`

	EnableDBMonitoring bool `env:"ENABLE_DB_MONITORING" envDefault:"false"`

	RedisAddress  string `validate:"required"`
	RedisURL      string
	RedisPassword string
	RedisDB       int

	HTTPPort int `validate:"min=1,max=65535"`

	UseWebhooks   bool
	WebhookDomain string
	WebhookSecret string

	EnablePerformanceMonitoring bool
	EnableBackgroundStats       bool
	DispatcherMaxRoutines       int `validate:"min=1,max=1000"`
	ClearCacheOnStartup         bool
	DisableCache                bool
	InactivityThresholdDays     int `validate:"min=1,max=365"`
	ActivityCheckInterval       int `validate:"min=1,max=24"`
	EnableAutoCleanup           bool

	// CacheLocalTTLSeconds bounds how long a replica serves a read-through
	// value from process memory before asking Redis again; 0 turns the local
	// layer off.
	CacheLocalTTLSeconds int `validate:"min=0,max=300"`
	CacheLocalMaxEntries int `validate:"min=100,max=1000000"`

	HTTPMaxIdleConns        int `validate:"min=10,max=1000"`
	HTTPMaxIdleConnsPerHost int `validate:"min=5,max=500"`

	AutoMigrate           bool
	AutoMigrateSilentFail bool
	MigrationsPath        string

	ResourceMaxGoroutines int `validate:"min=100,max=10000"`
	ResourceMaxMemoryMB   int `validate:"min=100,max=10000"`
	ResourceGCThresholdMB int `validate:"min=100,max=5000"`

	EnablePPROF bool

	MetricsAuthToken string

	// AI spam filter (TypeSafe Jev). EnableAISpam is the global AI kill switch: when
	// false it stops /aispam and every raid AI check, text (TypeSafe) and image
	// (Anthropic Claude) alike. The text feature is inert without TypeSafeAPIKey.
	EnableAISpam   bool
	TypeSafeAPIKey string

	// Image checks of the AI raid trigger, through the Anthropic API. The feature
	// is inert without AnthropicAPIKey (with no key no image leaves the server)
	// and while EnableAISpam is false. The key is a secret and is registered with
	// logredact.RegisterSecret. AnthropicModel is checked against the claude
	// package's model-name rule in setDefaults, so a pasted secret or odd value
	// is replaced by the default and never used.
	AnthropicAPIKey string
	AnthropicModel  string

	// Web captcha (Cloudflare Turnstile in a Telegram Mini App). It is on only when
	// all four of CAPTCHA_WEB_BASE_URL, TELEGRAM_MINI_APP_SHORT_NAME,
	// TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY are set; a partial set stops
	// startup. TurnstileSecretKey is registered with logredact and is sent only to
	// Cloudflare. CaptchaWebTrustedProxies is a comma-separated list of IPs or CIDRs
	// of the reverse proxies whose X-Forwarded-For the verify route may read.
	CaptchaWebBaseURL        string
	TelegramMiniAppShortName string
	TurnstileSiteKey         string
	TurnstileSecretKey       string
	CaptchaWebTrustedProxies string
}

var AppConfig *Config

// captchaWebShortNameRe is a loose superset of BotFather's Mini App short names.
var captchaWebShortNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// captchaWebMinSecretLen is the log redactor's floor: a shorter secret could not be scrubbed.
const captchaWebMinSecretLen = 6

// CaptchaWebConfigured reports whether all four required web captcha settings are set.
func (c *Config) CaptchaWebConfigured() bool {
	return c.CaptchaWebBaseURL != "" &&
		c.TelegramMiniAppShortName != "" &&
		c.TurnstileSiteKey != "" &&
		c.TurnstileSecretKey != ""
}

// validateCaptchaWeb checks the web captcha settings all-or-nothing. Every error
// names the variable and the rule only, never a value.
func validateCaptchaWeb(cfg *Config) error {
	required := []struct {
		name  string
		value string
	}{
		{"CAPTCHA_WEB_BASE_URL", cfg.CaptchaWebBaseURL},
		{"TELEGRAM_MINI_APP_SHORT_NAME", cfg.TelegramMiniAppShortName},
		{"TURNSTILE_SITE_KEY", cfg.TurnstileSiteKey},
		{"TURNSTILE_SECRET_KEY", cfg.TurnstileSecretKey},
	}
	var missing []string
	for _, r := range required {
		if r.value == "" {
			missing = append(missing, r.name)
		}
	}

	switch {
	case len(missing) == len(required):
		if cfg.CaptchaWebTrustedProxies != "" {
			return fmt.Errorf("CAPTCHA_WEB_TRUSTED_PROXIES is set but the web captcha is not configured; set CAPTCHA_WEB_BASE_URL, TELEGRAM_MINI_APP_SHORT_NAME, TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY, or remove it")
		}
		return nil
	case len(missing) > 0:
		return fmt.Errorf("web captcha settings are incomplete: missing %s", strings.Join(missing, ", "))
	}

	if !validCaptchaWebBaseURL(cfg.CaptchaWebBaseURL) {
		return fmt.Errorf("CAPTCHA_WEB_BASE_URL must be an https URL with a host, no user info, query or fragment, and a path that is empty or /")
	}
	if !captchaWebShortNameRe.MatchString(cfg.TelegramMiniAppShortName) {
		return fmt.Errorf("TELEGRAM_MINI_APP_SHORT_NAME must be 1 to 64 letters, digits or underscores")
	}
	if len(cfg.TurnstileSecretKey) < captchaWebMinSecretLen {
		return fmt.Errorf("TURNSTILE_SECRET_KEY must be at least %d characters so it can be scrubbed from logs", captchaWebMinSecretLen)
	}
	if !validCaptchaWebProxies(cfg.CaptchaWebTrustedProxies) {
		return fmt.Errorf("CAPTCHA_WEB_TRUSTED_PROXIES must be a comma-separated list of IP addresses or CIDR prefixes")
	}
	return nil
}

// validCaptchaWebBaseURL reports whether raw is an https URL with a host and
// nothing but an empty or "/" path. It never returns the parse error, which
// would echo the URL.
func validCaptchaWebBaseURL(raw string) bool {
	if strings.ContainsAny(raw, "?#") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "https" &&
		u.Hostname() != "" &&
		u.User == nil &&
		u.RawQuery == "" &&
		u.Fragment == "" &&
		(u.Path == "" || u.Path == "/")
}

// validCaptchaWebProxies reports whether every comma-separated entry of list is
// an IP address or a CIDR prefix. Empty entries (an empty list, a trailing or a
// doubled comma) are skipped, which is the rule of captchaweb.ParseTrustedProxies
// that parses the same setting at runtime; the package is a leaf and cannot be
// shared, so the two tests hold the same table of inputs.
func validCaptchaWebProxies(list string) bool {
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, err := netip.ParsePrefix(entry); err != nil {
				return false
			}
			continue
		}
		if _, err := netip.ParseAddr(entry); err != nil {
			return false
		}
	}
	return true
}

func ValidateConfig(cfg *Config) error {
	if cfg.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.OwnerId == 0 {
		return fmt.Errorf("OWNER_ID is required and must be greater than 0")
	}
	if cfg.MessageDump == 0 {
		return fmt.Errorf("MESSAGE_DUMP is required and must be greater than 0")
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if !cfg.DisableCache && cfg.RedisAddress == "" {
		return fmt.Errorf("REDIS_ADDRESS or REDIS_URL is required")
	}

	if cfg.UseWebhooks {
		if cfg.WebhookDomain == "" {
			return fmt.Errorf("WEBHOOK_DOMAIN is required when USE_WEBHOOKS is enabled")
		}
		if cfg.WebhookSecret == "" {
			return fmt.Errorf("WEBHOOK_SECRET is required when USE_WEBHOOKS is enabled for security")
		}
	}

	if cfg.HTTPPort <= 0 || cfg.HTTPPort > 65535 {
		return fmt.Errorf("HTTP_PORT must be between 1 and 65535")
	}

	if cfg.DispatcherMaxRoutines != 0 && (cfg.DispatcherMaxRoutines < 1 || cfg.DispatcherMaxRoutines > 1000) {
		return fmt.Errorf("DISPATCHER_MAX_ROUTINES must be between 1 and 1000")
	}

	if cfg.CacheLocalTTLSeconds < 0 || cfg.CacheLocalTTLSeconds > 300 {
		return fmt.Errorf("CACHE_LOCAL_TTL must be between 0 and 300 seconds")
	}
	if cfg.CacheLocalMaxEntries != 0 && (cfg.CacheLocalMaxEntries < 100 || cfg.CacheLocalMaxEntries > 1000000) {
		return fmt.Errorf("CACHE_LOCAL_MAX_ENTRIES must be between 100 and 1000000")
	}

	if cfg.DBMaxIdleConns != 0 && (cfg.DBMaxIdleConns < 1 || cfg.DBMaxIdleConns > 100) {
		return fmt.Errorf("DB_MAX_IDLE_CONNS must be between 1 and 100")
	}
	if cfg.DBMaxOpenConns != 0 && (cfg.DBMaxOpenConns < 1 || cfg.DBMaxOpenConns > 1000) {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be between 1 and 1000")
	}
	if cfg.DBConnMaxLifetimeMin != 0 && (cfg.DBConnMaxLifetimeMin < 1 || cfg.DBConnMaxLifetimeMin > 1440) {
		return fmt.Errorf("DB_CONN_MAX_LIFETIME_MIN must be between 1 and 1440 minutes")
	}
	if cfg.DBConnMaxIdleTimeMin != 0 && (cfg.DBConnMaxIdleTimeMin < 1 || cfg.DBConnMaxIdleTimeMin > 60) {
		return fmt.Errorf("DB_CONN_MAX_IDLE_TIME_MIN must be between 1 and 60 minutes")
	}

	if cfg.RedisDB < 0 || cfg.RedisDB > 15 {
		return fmt.Errorf("REDIS_DB must be an integer 0-15")
	}

	return validateCaptchaWeb(cfg)
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	redisDB, err := parseRedisDB()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BotToken:    os.Getenv("BOT_TOKEN"),
		BotVersion:  "2.23.14",
		ApiServer:   os.Getenv("API_SERVER"),
		WorkingMode: "worker",
		Debug:       typeConvertor{str: os.Getenv("DEBUG")}.Bool(),

		OwnerId:            typeConvertor{str: os.Getenv("OWNER_ID")}.Int64(),
		MessageDump:        typeConvertor{str: os.Getenv("MESSAGE_DUMP")}.Int64(),
		DropPendingUpdates: typeConvertor{str: os.Getenv("DROP_PENDING_UPDATES")}.Bool(),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		DBMaxIdleConns:       typeConvertor{str: os.Getenv("DB_MAX_IDLE_CONNS")}.Int(),
		DBMaxOpenConns:       typeConvertor{str: os.Getenv("DB_MAX_OPEN_CONNS")}.Int(),
		DBConnMaxLifetimeMin: typeConvertor{str: os.Getenv("DB_CONN_MAX_LIFETIME_MIN")}.Int(),
		DBConnMaxIdleTimeMin: typeConvertor{str: os.Getenv("DB_CONN_MAX_IDLE_TIME_MIN")}.Int(),

		EnableDBMonitoring: typeConvertor{str: os.Getenv("ENABLE_DB_MONITORING")}.Bool(),

		RedisAddress:  getRedisAddress(),
		RedisURL:      getRedisURL(),
		RedisPassword: getRedisPassword(),
		RedisDB:       redisDB,

		HTTPPort: getHTTPPort(),

		UseWebhooks:   typeConvertor{str: os.Getenv("USE_WEBHOOKS")}.Bool(),
		WebhookDomain: os.Getenv("WEBHOOK_DOMAIN"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),

		EnablePerformanceMonitoring: typeConvertor{str: os.Getenv("ENABLE_PERFORMANCE_MONITORING")}.Bool(),
		EnableBackgroundStats:       typeConvertor{str: os.Getenv("ENABLE_BACKGROUND_STATS")}.Bool(),
		DispatcherMaxRoutines:       typeConvertor{str: os.Getenv("DISPATCHER_MAX_ROUTINES")}.Int(),

		ClearCacheOnStartup: typeConvertor{str: os.Getenv("CLEAR_CACHE_ON_STARTUP")}.Bool(),
		DisableCache:        typeConvertor{str: os.Getenv("DISABLE_CACHE")}.Bool(),

		CacheLocalTTLSeconds: getCacheLocalTTL(),
		CacheLocalMaxEntries: typeConvertor{str: os.Getenv("CACHE_LOCAL_MAX_ENTRIES")}.Int(),

		InactivityThresholdDays: typeConvertor{str: os.Getenv("INACTIVITY_THRESHOLD_DAYS")}.Int(),
		ActivityCheckInterval:   typeConvertor{str: os.Getenv("ACTIVITY_CHECK_INTERVAL")}.Int(),
		EnableAutoCleanup:       typeConvertor{str: os.Getenv("ENABLE_AUTO_CLEANUP")}.Bool(),

		HTTPMaxIdleConns:        typeConvertor{str: os.Getenv("HTTP_MAX_IDLE_CONNS")}.Int(),
		HTTPMaxIdleConnsPerHost: typeConvertor{str: os.Getenv("HTTP_MAX_IDLE_CONNS_PER_HOST")}.Int(),

		AutoMigrate:           typeConvertor{str: os.Getenv("AUTO_MIGRATE")}.Bool(),
		AutoMigrateSilentFail: typeConvertor{str: os.Getenv("AUTO_MIGRATE_SILENT_FAIL")}.Bool(),
		MigrationsPath:        os.Getenv("MIGRATIONS_PATH"),

		ResourceMaxGoroutines: typeConvertor{str: os.Getenv("RESOURCE_MAX_GOROUTINES")}.Int(),
		ResourceMaxMemoryMB:   typeConvertor{str: os.Getenv("RESOURCE_MAX_MEMORY_MB")}.Int(),
		ResourceGCThresholdMB: typeConvertor{str: os.Getenv("RESOURCE_GC_THRESHOLD_MB")}.Int(),

		EnablePPROF: typeConvertor{str: os.Getenv("ENABLE_PPROF")}.Bool(),

		MetricsAuthToken: os.Getenv("METRICS_AUTH_TOKEN"),

		EnableAISpam:   typeConvertor{str: os.Getenv("ENABLE_AISPAM")}.Bool(),
		TypeSafeAPIKey: os.Getenv("TYPESAFE_API_KEY"),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  os.Getenv("ANTHROPIC_MODEL"),

		CaptchaWebBaseURL:        strings.TrimSpace(os.Getenv("CAPTCHA_WEB_BASE_URL")),
		TelegramMiniAppShortName: strings.TrimSpace(os.Getenv("TELEGRAM_MINI_APP_SHORT_NAME")),
		TurnstileSiteKey:         strings.TrimSpace(os.Getenv("TURNSTILE_SITE_KEY")),
		TurnstileSecretKey:       strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY")),
		CaptchaWebTrustedProxies: strings.TrimSpace(os.Getenv("CAPTCHA_WEB_TRUSTED_PROXIES")),
	}

	cfg.setDefaults()

	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	cfg.AllowedUpdates = []string{
		"message",
		"edited_message",
		"channel_post",
		"edited_channel_post",
		"inline_query",
		"chosen_inline_result",
		"callback_query",
		"shipping_query",
		"pre_checkout_query",
		"poll",
		"poll_answer",
		"my_chat_member",
		"chat_member",
		"chat_join_request",
	}

	cfg.ValidLangCodes = typeConvertor{str: os.Getenv("ENABLED_LOCALES")}.StringArray()
	if (len(cfg.ValidLangCodes) == 1 && cfg.ValidLangCodes[0] == "") || (len(cfg.ValidLangCodes) == 0) {
		cfg.ValidLangCodes = []string{"en"}
	}

	return cfg, nil
}

func (cfg *Config) setDefaults() {
	if cfg.ApiServer == "" {
		cfg.ApiServer = "https://api.telegram.org"
	}
	if cfg.WorkingMode == "" {
		cfg.WorkingMode = "worker"
	}
	if cfg.RedisAddress == "" {
		cfg.RedisAddress = "localhost:6379"
	}
	if cfg.RedisDB == 0 && os.Getenv("REDIS_DB") != "0" {
		cfg.RedisDB = 1
	}

	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 8080
	}

	if cfg.InactivityThresholdDays == 0 {
		cfg.InactivityThresholdDays = 30
	}
	if cfg.ActivityCheckInterval == 0 {
		cfg.ActivityCheckInterval = 1
	}
	if os.Getenv("ENABLE_AUTO_CLEANUP") == "" {
		cfg.EnableAutoCleanup = true
	}

	if cfg.DBMaxIdleConns == 0 {
		cfg.DBMaxIdleConns = 50
	}
	if cfg.DBMaxOpenConns == 0 {
		cfg.DBMaxOpenConns = 200
	}
	if cfg.DBConnMaxLifetimeMin == 0 {
		cfg.DBConnMaxLifetimeMin = 240
	}
	if cfg.DBConnMaxIdleTimeMin == 0 {
		cfg.DBConnMaxIdleTimeMin = 60
	}

	if cfg.DispatcherMaxRoutines == 0 {
		cfg.DispatcherMaxRoutines = 200
	}

	if cfg.CacheLocalMaxEntries == 0 {
		cfg.CacheLocalMaxEntries = DefaultCacheLocalMaxEntries
	}

	if !cfg.Debug {
		if os.Getenv("ENABLE_PERFORMANCE_MONITORING") == "" {
			cfg.EnablePerformanceMonitoring = true
		}
		if os.Getenv("ENABLE_BACKGROUND_STATS") == "" {
			cfg.EnableBackgroundStats = true
		}
	}

	if cfg.HTTPMaxIdleConns == 0 {
		cfg.HTTPMaxIdleConns = 100
	}
	if cfg.HTTPMaxIdleConnsPerHost == 0 {
		cfg.HTTPMaxIdleConnsPerHost = 50
	}

	if cfg.MigrationsPath == "" {
		cfg.MigrationsPath = "migrations"
	}

	if cfg.ResourceMaxGoroutines == 0 {
		cfg.ResourceMaxGoroutines = 1000
	}
	if cfg.ResourceMaxMemoryMB == 0 {
		cfg.ResourceMaxMemoryMB = 500
	}
	if cfg.ResourceGCThresholdMB == 0 {
		cfg.ResourceGCThresholdMB = 400
	}

	// Global kill switch: on unless explicitly disabled. The feature stays
	// inert without TYPESAFE_API_KEY, so an unset flag is not a decision to
	// delete anything.
	if os.Getenv("ENABLE_AISPAM") == "" {
		cfg.EnableAISpam = true
	}

	// The model name ends up in a request body, so one that is not a plain
	// model id is replaced by the default instead of being used.
	if cfg.AnthropicModel == "" {
		cfg.AnthropicModel = claude.DefaultModel
	} else if !claude.ValidModel(cfg.AnthropicModel) {
		// The rejected value is never logged: setDefaults runs before registerSecrets, and a
		// variable pasted into the wrong place could be a key.
		log.Warnf("[Config] ANTHROPIC_MODEL (%d characters) is not a valid model name, using %s",
			utf8.RuneCountInString(cfg.AnthropicModel), claude.DefaultModel)
		cfg.AnthropicModel = claude.DefaultModel
	}
}

// registerSecrets registers every configured secret with logredact so its exact
// value is scrubbed from all log output.
func registerSecrets(cfg *Config) {
	logredact.RegisterSecret(
		cfg.BotToken,
		cfg.DatabaseURL,
		cfg.RedisPassword,
		cfg.WebhookSecret,
		cfg.MetricsAuthToken,
		cfg.TypeSafeAPIKey,
		cfg.AnthropicAPIKey,
		cfg.TurnstileSecretKey,
	)
}

func init() {
	if isCliModeActive() {
		AppConfig = &Config{}
		return
	}

	log.SetLevel(log.DebugLevel)
	log.SetFormatter(
		&log.JSONFormatter{
			DisableHTMLEscape: true,
			PrettyPrint:       false,
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				return f.Function, fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
			},
		},
	)

	logredact.Install(nil)

	cfg, err := LoadConfig()
	if err != nil {
		if os.Getenv("BOT_TOKEN") == "" {
			// Keep the local cache layer at its production default so test
			// binaries exercise the same read path as a deployed bot.
			AppConfig = &Config{
				CacheLocalTTLSeconds: DefaultCacheLocalTTLSeconds,
				CacheLocalMaxEntries: DefaultCacheLocalMaxEntries,
			}
			return
		}
		log.Fatalf("[Config] Failed to load configuration: %v", err)
	}

	AppConfig = cfg

	registerSecrets(cfg)

	if cfg.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.SetReportCaller(cfg.Debug)

	log.Info("[Config] Configuration loaded and validated successfully")
}
