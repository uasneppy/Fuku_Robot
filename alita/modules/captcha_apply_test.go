//go:build testtools

package modules

import (
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// captchaApplyStored reads a chat's captcha row straight from the database (no cache), or nil
// when the chat has none.
func captchaApplyStored(t *testing.T, chatID int64) *models.CaptchaSettings {
	t.Helper()
	var rows []models.CaptchaSettings
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read captcha row error = %v", err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

// captchaApplyBrokenDB swaps db.DB for an empty SQLite database with no tables, so every read
// and write fails. The returned function puts the real database back early.
func captchaApplyBrokenDB(t *testing.T) (restore func()) {
	t.Helper()
	original := db.DB
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := broken.DB()
	if err != nil {
		t.Fatal(err)
	}
	db.DB = broken
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		db.DB = original
		_ = sqlDB.Close()
	}
	t.Cleanup(restore)
	return restore
}

func TestApplyCaptchaEnabled(t *testing.T) {
	t.Run("on stores the switch and reads the row back", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chatID := uniqueModuleChatID()

		got := applyCaptchaEnabled(bot, chatID, true)
		if got.Outcome != captchaApplied || got.Err != nil || got.Key != "" {
			t.Fatalf("applyCaptchaEnabled(on) = %+v, want captchaApplied", got)
		}
		if got.Settings == nil || !got.Settings.Enabled {
			t.Fatalf("Settings = %+v, want Enabled true", got.Settings)
		}
		if row := captchaApplyStored(t, chatID); row == nil || !row.Enabled {
			t.Fatalf("stored row = %+v, want Enabled true", row)
		}
	})

	t.Run("off releases pending attempts and restores members", func(t *testing.T) {
		client := newModuleBotClient()
		bot := newModuleTestBot(client)
		chatID := uniqueModuleChatID()
		if err := captcha.SetCaptchaEnabled(chatID, true); err != nil {
			t.Fatalf("SetCaptchaEnabled() error = %v", err)
		}
		attempt, err := captcha.CreateCaptchaAttemptPreMessage(4301, chatID, "7", 2)
		if err != nil {
			t.Fatalf("CreateCaptchaAttemptPreMessage() error = %v", err)
		}
		if err := captcha.UpdateCaptchaAttemptMessageID(attempt.ID, 123); err != nil {
			t.Fatalf("UpdateCaptchaAttemptMessageID() error = %v", err)
		}
		if err := captcha.CreateMutedUser(4302, chatID, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("CreateMutedUser() error = %v", err)
		}

		got := applyCaptchaEnabled(bot, chatID, false)
		if got.Outcome != captchaApplied || got.Err != nil {
			t.Fatalf("applyCaptchaEnabled(off) = %+v, want captchaApplied", got)
		}
		if got.Settings == nil || got.Settings.Enabled {
			t.Fatalf("Settings = %+v, want Enabled false", got.Settings)
		}
		if current, err := captcha.GetCaptchaAttemptByID(attempt.ID); err != nil || current != nil {
			t.Fatalf("attempt after off = %#v, %v; want released", current, err)
		}
		if users, err := captcha.GetMutedUsersForChat(chatID); err != nil || len(users) != 0 {
			t.Fatalf("muted users after off = %#v, %v; want none", users, err)
		}
		if calls := client.callsFor("restrictChatMember"); len(calls) != 2 {
			t.Fatalf("restrictChatMember calls = %d, want both members restored", len(calls))
		}
		if calls := client.callsFor("deleteMessage"); len(calls) != 1 {
			t.Fatalf("deleteMessage calls = %d, want the challenge deleted", len(calls))
		}
	})

	t.Run("a failed cleanup keeps the switch saved and reports captcha_disable_failed", func(t *testing.T) {
		client := newModuleBotClient()
		client.errors["restrictChatMember"] = errors.New("restrict refused")
		bot := newModuleTestBot(client)
		chatID := uniqueModuleChatID()
		if err := captcha.SetCaptchaEnabled(chatID, true); err != nil {
			t.Fatalf("SetCaptchaEnabled() error = %v", err)
		}
		if err := captcha.CreateMutedUser(4303, chatID, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("CreateMutedUser() error = %v", err)
		}

		got := applyCaptchaEnabled(bot, chatID, false)
		if got.Outcome != captchaCleanupFailed || got.Key != "captcha_disable_failed" || got.Err == nil {
			t.Fatalf("applyCaptchaEnabled(off) = %+v, want captchaCleanupFailed with captcha_disable_failed", got)
		}
		if row := captchaApplyStored(t, chatID); row == nil || row.Enabled {
			t.Fatalf("stored row = %+v, want the switch saved as false", row)
		}
	})

	t.Run("a failed write reports the command key and stores nothing", func(t *testing.T) {
		for _, tc := range []struct {
			on  bool
			key string
		}{
			{true, "captcha_enable_failed"},
			{false, "captcha_disable_failed"},
		} {
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chatID := uniqueModuleChatID()
			restore := captchaApplyBrokenDB(t)

			got := applyCaptchaEnabled(bot, chatID, tc.on)
			if got.Outcome != captchaSaveFailed || got.Key != tc.key || got.Err == nil {
				t.Fatalf("applyCaptchaEnabled(%v) = %+v, want captchaSaveFailed with %s", tc.on, got, tc.key)
			}
			if got.Settings != nil {
				t.Fatalf("Settings = %+v, want nil on a failed write", got.Settings)
			}
			if calls := client.callsFor("restrictChatMember"); len(calls) != 0 {
				t.Fatalf("restrictChatMember calls = %d, want none after a failed write", len(calls))
			}
			restore()
			if row := captchaApplyStored(t, chatID); row != nil {
				t.Fatalf("stored row = %+v, want nothing stored", row)
			}
		}
	})
}

func TestApplyCaptchaAction(t *testing.T) {
	t.Run("kick, ban and mute are stored and read back", func(t *testing.T) {
		for _, action := range []string{"kick", "ban", "mute"} {
			chatID := uniqueModuleChatID()
			got := applyCaptchaAction(chatID, action)
			if got.Outcome != captchaApplied || got.Err != nil || got.Key != "" {
				t.Fatalf("applyCaptchaAction(%q) = %+v, want captchaApplied", action, got)
			}
			if got.Settings == nil || got.Settings.FailureAction != action {
				t.Fatalf("Settings = %+v, want FailureAction %q", got.Settings, action)
			}
			if row := captchaApplyStored(t, chatID); row == nil || row.FailureAction != action {
				t.Fatalf("stored row = %+v, want FailureAction %q", row, action)
			}
		}
	})

	t.Run("the action is lower-cased first", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		got := applyCaptchaAction(chatID, "Ban")
		if got.Outcome != captchaApplied {
			t.Fatalf("applyCaptchaAction(Ban) = %+v, want captchaApplied", got)
		}
		if row := captchaApplyStored(t, chatID); row == nil || row.FailureAction != "ban" {
			t.Fatalf("stored row = %+v, want ban", row)
		}
	})

	t.Run("anything else is refused and nothing is written", func(t *testing.T) {
		for _, action := range []string{"warn", "", "KICK "} {
			chatID := uniqueModuleChatID()
			got := applyCaptchaAction(chatID, action)
			if got.Outcome != captchaInvalid || got.Key != "captcha_action_invalid" {
				t.Fatalf("applyCaptchaAction(%q) = %+v, want captchaInvalid with captcha_action_invalid", action, got)
			}
			if row := captchaApplyStored(t, chatID); row != nil {
				t.Fatalf("stored row = %+v after a refused %q, want none", row, action)
			}
		}
	})

	t.Run("a failed write reports captcha_action_failed", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		captchaApplyBrokenDB(t)
		got := applyCaptchaAction(chatID, "ban")
		if got.Outcome != captchaSaveFailed || got.Key != "captcha_action_failed" || got.Err == nil {
			t.Fatalf("applyCaptchaAction(ban) = %+v, want captchaSaveFailed with captcha_action_failed", got)
		}
	})
}

func TestApplyCaptchaMode(t *testing.T) {
	t.Run("math and text are stored", func(t *testing.T) {
		for _, mode := range []string{"math", "text"} {
			chatID := uniqueModuleChatID()
			bot := newModuleTestBot(newModuleBotClient())
			got := applyCaptchaMode(bot, chatID, mode)
			if got.Outcome != captchaApplied || got.Err != nil || got.Key != "" {
				t.Fatalf("applyCaptchaMode(%q) = %+v, want captchaApplied", mode, got)
			}
			if got.Settings == nil || got.Settings.CaptchaMode != mode {
				t.Fatalf("Settings = %+v, want CaptchaMode %q", got.Settings, mode)
			}
			if row := captchaApplyStored(t, chatID); row == nil || row.CaptchaMode != mode {
				t.Fatalf("stored row = %+v, want %q", row, mode)
			}
		}
	})

	t.Run("an unknown mode is refused before anything is written", func(t *testing.T) {
		for _, mode := range []string{"Image", ""} {
			chatID := uniqueModuleChatID()
			bot := newModuleTestBot(newModuleBotClient())
			got := applyCaptchaMode(bot, chatID, mode)
			if got.Outcome != captchaInvalid || got.Key != "captcha_mode_invalid" {
				t.Fatalf("applyCaptchaMode(%q) = %+v, want captchaInvalid with captcha_mode_invalid", mode, got)
			}
			if row := captchaApplyStored(t, chatID); row != nil {
				t.Fatalf("stored row = %+v after a refused %q, want none", row, mode)
			}
		}
	})

	// start builds a Turnstile-ready env whose group is stored on math.
	start := func(t *testing.T) *captchaWebEnv {
		t.Helper()
		env := newCaptchaWebEnv(t)
		if err := captcha.SetCaptchaMode(env.chat.Id, "math"); err != nil {
			t.Fatalf("SetCaptchaMode(math) error = %v", err)
		}
		return env
	}
	expectRefused := func(t *testing.T, env *captchaWebEnv, got captchaApplyResult, key string) {
		t.Helper()
		if got.Outcome != captchaRefused || got.Key != key || got.Err != nil {
			t.Fatalf("applyCaptchaMode(turnstile) = %+v, want captchaRefused with %s", got, key)
		}
		if row := captchaApplyStored(t, env.chat.Id); row == nil || row.CaptchaMode != "math" {
			t.Fatalf("stored row = %+v, want the mode unchanged on math", row)
		}
	}

	t.Run("turnstile with no runtime is refused without any call", func(t *testing.T) {
		env := start(t)
		captchaWebCurrent.Store(nil)
		got := applyCaptchaMode(env.bot, env.chat.Id, "turnstile")
		expectRefused(t, env, got, "captcha_web_refuse_not_configured")
		if calls := env.client.callsFor("getChatMember"); len(calls) != 0 {
			t.Fatalf("getChatMember calls = %d, want none", len(calls))
		}
	})

	t.Run("turnstile with Redis down is refused before the bot's rights are read", func(t *testing.T) {
		env := start(t)
		dead := miniredis.RunT(t)
		deadClient := redis.NewClient(&redis.Options{Addr: dead.Addr()})
		t.Cleanup(func() { _ = deadClient.Close() })
		rt, err := newCaptchaWebRuntime(env.bot, CaptchaWebConfig{
			BaseURL: "https://" + captchaWebTestHost, ShortName: "verify", SiteKey: "k",
			Secret: env.secret, Redis: deadClient,
		})
		if err != nil {
			t.Fatalf("newCaptchaWebRuntime() error = %v", err)
		}
		captchaWebCurrent.Store(rt)
		dead.Close()

		got := applyCaptchaMode(env.bot, env.chat.Id, "turnstile")
		expectRefused(t, env, got, "captcha_web_refuse_no_redis")
		if calls := env.client.callsFor("getChatMember"); len(calls) != 0 {
			t.Fatalf("getChatMember calls = %d, want none", len(calls))
		}
	})

	t.Run("turnstile with the bot a plain member is refused", func(t *testing.T) {
		env := start(t)
		bot := turnstileModeBot(env.client, "member")
		got := applyCaptchaMode(bot, env.chat.Id, "turnstile")
		expectRefused(t, env, got, "captcha_web_refuse_no_rights")
	})

	t.Run("turnstile is stored once the bot may restrict members", func(t *testing.T) {
		env := start(t)
		got := applyCaptchaMode(env.bot, env.chat.Id, "Turnstile")
		if got.Outcome != captchaApplied || got.Err != nil {
			t.Fatalf("applyCaptchaMode(Turnstile) = %+v, want captchaApplied", got)
		}
		if got.Settings == nil || got.Settings.CaptchaMode != "turnstile" {
			t.Fatalf("Settings = %+v, want CaptchaMode turnstile", got.Settings)
		}
		if row := captchaApplyStored(t, env.chat.Id); row == nil || row.CaptchaMode != "turnstile" {
			t.Fatalf("stored row = %+v, want turnstile", row)
		}
	})

	t.Run("a failed write reports captcha_mode_failed", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		bot := newModuleTestBot(newModuleBotClient())
		captchaApplyBrokenDB(t)
		got := applyCaptchaMode(bot, chatID, "text")
		if got.Outcome != captchaSaveFailed || got.Key != "captcha_mode_failed" || got.Err == nil {
			t.Fatalf("applyCaptchaMode(text) = %+v, want captchaSaveFailed with captcha_mode_failed", got)
		}
	})
}

func TestCaptchaWebInstalled(t *testing.T) {
	env := newCaptchaWebEnv(t)

	if !captchaWebInstalled(env.bot) {
		t.Fatal("captchaWebInstalled() = false with a runtime, a username and a Redis client")
	}
	if got := captchaWebModeRefusal(env.bot, &env.chat); got == "captcha_web_refuse_not_configured" {
		t.Fatalf("captchaWebModeRefusal() = %s although the web captcha is installed", got)
	}

	noName := *env.bot
	noName.User.Username = ""
	if captchaWebInstalled(&noName) {
		t.Fatal("captchaWebInstalled() = true for a bot without a username")
	}
	if got := captchaWebModeRefusal(&noName, &env.chat); got != "captcha_web_refuse_not_configured" {
		t.Fatalf("captchaWebModeRefusal(no username) = %q, want captcha_web_refuse_not_configured", got)
	}

	if captchaWebInstalled(nil) {
		t.Fatal("captchaWebInstalled(nil) = true")
	}

	captchaWebCurrent.Store(nil)
	if captchaWebInstalled(env.bot) {
		t.Fatal("captchaWebInstalled() = true with no runtime")
	}
	if got := captchaWebModeRefusal(env.bot, &gotgbot.Chat{Id: env.chat.Id}); got != "captcha_web_refuse_not_configured" {
		t.Fatalf("captchaWebModeRefusal(no runtime) = %q, want captcha_web_refuse_not_configured", got)
	}
}
