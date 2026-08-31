package fuku

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/uasneppy/Fuku_Robot/fuku/db"
	"github.com/uasneppy/Fuku_Robot/fuku/modules"
)

type fukuTestBotClient struct{}

func (fukuTestBotClient) RequestWithContext(_ context.Context, _ string, method string, _ map[string]any, _ *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method == "getMe" {
		return json.RawMessage(`{"id":999,"is_bot":true,"first_name":"Fuku","username":"FukuTestBot"}`), nil
	}
	return json.RawMessage(`true`), nil
}

func (fukuTestBotClient) GetAPIURL(*gotgbot.RequestOpts) string {
	return "https://api.telegram.org"
}

func (fukuTestBotClient) FileURL(token string, path string, _ *gotgbot.RequestOpts) string {
	return "https://api.telegram.org/file/bot" + token + "/" + path
}

var (
	fukuMainDBOnce sync.Once
	fukuMainDBErr  error
)

func setupFukuMainDB(t *testing.T) {
	t.Helper()

	fukuMainDBOnce.Do(func() {
		if db.DB == nil {
			db.DB, fukuMainDBErr = gorm.Open(
				sqlite.Open("file:fuku_main_test?mode=memory&cache=shared"),
				&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
			)
			if fukuMainDBErr != nil {
				return
			}
		}
		fukuMainDBErr = db.DB.AutoMigrate(&db.User{})
	})
	if fukuMainDBErr != nil {
		t.Fatalf("setup fuku main DB: %v", fukuMainDBErr)
	}
}

func resetHelpRegistryForTest(t *testing.T) {
	t.Helper()

	registry := modules.DefaultHelpRegistry()
	registry.AbleMap = make(map[string]bool)
	registry.AltHelpOptions = make(map[string][]string)
	t.Cleanup(func() {
		registry.AbleMap = make(map[string]bool)
		registry.AltHelpOptions = make(map[string][]string)
	})
}

func TestListModulesSortsEnabledModuleNames(t *testing.T) {
	resetHelpRegistryForTest(t)

	registry := modules.DefaultHelpRegistry()

	registry.AbleMap["Warns"] = true
	registry.AbleMap["Admin"] = true
	registry.AbleMap["Filters"] = true

	if got, want := ListModules(), "[Admin, Filters, Warns]"; got != want {
		t.Fatalf("ListModules() = %q, want %q", got, want)
	}
}

func TestLoadModulesLoadsRegistryAndHelp(t *testing.T) {
	resetHelpRegistryForTest(t)

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadModules(dispatcher)

	for _, moduleName := range []string{"Admin", "Captcha", "Filters", "Greetings", "Warns"} {
		if !modules.DefaultHelpRegistry().AbleMap[moduleName] {
			t.Fatalf("%s was not enabled after LoadModules", moduleName)
		}
	}
}

func TestInitialChecksEnsuresBot(t *testing.T) {
	setupFukuMainDB(t)
	bot := &gotgbot.Bot{
		Token:     "999:test",
		BotClient: fukuTestBotClient{},
		User: gotgbot.User{
			Id:        999,
			IsBot:     true,
			FirstName: "Fallback",
			Username:  "fallback_bot",
		},
	}

	if err := InitialChecks(bot); err != nil {
		t.Fatalf("InitialChecks() error = %v", err)
	}

	var user db.User
	if err := db.DB.Where("user_id = ?", int64(999)).First(&user).Error; err != nil {
		t.Fatalf("bot user was not created: %v", err)
	}
	if user.UserName != "FukuTestBot" || user.Name != "Fuku" {
		t.Fatalf("bot user = username %q name %q, want FukuTestBot/Fuku", user.UserName, user.Name)
	}
}

func TestInitialChecksReturnsDatabaseError(t *testing.T) {
	originalDB := db.DB
	t.Cleanup(func() {
		db.DB = originalDB
	})

	var err error
	db.DB, err = gorm.Open(
		sqlite.Open("file:fuku_main_missing_schema?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open schema-less database: %v", err)
	}

	bot := &gotgbot.Bot{
		Token:     "999:test",
		BotClient: fukuTestBotClient{},
		User:      gotgbot.User{Id: 999, IsBot: true, FirstName: "Fuku"},
	}
	if err := InitialChecks(bot); err == nil {
		t.Fatal("InitialChecks() error = nil, want missing-schema error")
	}
}
