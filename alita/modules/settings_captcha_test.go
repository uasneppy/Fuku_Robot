//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// settingsCaptchaStored reads the captcha_settings row of chatID straight from the
// database, past every cache layer. ok is false when the chat has no row.
func settingsCaptchaStored(t *testing.T, chatID int64) (row models.CaptchaSettings, ok bool) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Limit(1).Find(&row).Error; err != nil {
		t.Fatalf("read captcha_settings of %d: %v", chatID, err)
	}
	return row, row.ID != 0
}

// settingsCaptchaStoreMode writes a mode into the row directly, the way the Phase 8 tests
// do (the row is created first when it is missing), and drops the cached copy.
func settingsCaptchaStoreMode(t *testing.T, chatID int64, mode string) {
	t.Helper()
	if err := captcha.SetCaptchaMode(chatID, "math"); err != nil {
		t.Fatalf("SetCaptchaMode(math) setup: %v", err)
	}
	if err := db.DB.Model(&models.CaptchaSettings{}).Where("chat_id = ?", chatID).
		Update("captcha_mode", mode).Error; err != nil {
		t.Fatalf("store mode %s: %v", mode, err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("captcha_settings", chatID))
	dbcache.ResetLocalForTest()
}

// settingsCaptchaNoWeb makes the web captcha absent for the test and puts the shared
// runtime and probe back when it ends.
func settingsCaptchaNoWeb(t *testing.T) {
	t.Helper()
	previous := captchaWebCurrent.Load()
	captchaWebCurrent.Store(nil)
	captchaWebProbe.Store(nil)
	t.Cleanup(func() {
		captchaWebCurrent.Store(previous)
		captchaWebProbe.Store(nil)
	})
}

// settingsCaptchaConfig is a web captcha config over the given Redis client.
func settingsCaptchaConfig(rc *redis.Client) CaptchaWebConfig {
	return CaptchaWebConfig{
		BaseURL:   "https://captcha.example.com",
		ShortName: "verify",
		SiteKey:   "settings-site-key",
		Secret:    "settings-turnstile-secret",
		Redis:     rc,
	}
}

// settingsCaptchaInstallWeb installs a web captcha runtime whose Redis is the env's
// miniredis, which is up.
func settingsCaptchaInstallWeb(t *testing.T, env *settingsEnv) {
	t.Helper()
	settingsCaptchaNoWeb(t)
	rc := redis.NewClient(&redis.Options{Addr: env.redis.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	rt, err := newCaptchaWebRuntime(env.bot, settingsCaptchaConfig(rc))
	if err != nil {
		t.Fatalf("newCaptchaWebRuntime: %v", err)
	}
	captchaWebCurrent.Store(rt)
}

// settingsCaptchaInstallDeadWeb installs a runtime whose own Redis is gone; the shared
// cache Redis of the env stays up.
func settingsCaptchaInstallDeadWeb(t *testing.T, env *settingsEnv) {
	t.Helper()
	settingsCaptchaNoWeb(t)
	dead := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: dead.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	rt, err := newCaptchaWebRuntime(env.bot, settingsCaptchaConfig(rc))
	if err != nil {
		t.Fatalf("newCaptchaWebRuntime: %v", err)
	}
	captchaWebCurrent.Store(rt)
	dead.Close()
}

// settingsCaptchaButton finds the button of the control t on a keyboard; a non-empty
// value must match the button's v as well.
func settingsCaptchaButton(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, control, value string) gotgbot.InlineKeyboardButton {
	t.Helper()
	for _, button := range settingsAllButtons(keyboard) {
		fields := settingsButtonFields(t, button)
		if fields["t"] == control && (value == "" || fields["v"] == value) {
			return button
		}
	}
	t.Fatalf("no button for control %q value %q on %+v", control, value, keyboard)
	return gotgbot.InlineKeyboardButton{}
}

// settingsCaptchaRow returns the row of the control t.
func settingsCaptchaRow(t *testing.T, keyboard [][]gotgbot.InlineKeyboardButton, control string) []gotgbot.InlineKeyboardButton {
	t.Helper()
	for _, row := range keyboard {
		if len(row) > 0 && settingsButtonFields(t, row[0])["t"] == control {
			return row
		}
	}
	t.Fatalf("no row for control %q on %+v", control, keyboard)
	return nil
}

// settingsCaptchaRefresh redraws the Captcha page from the stored row with the stepper's
// value button, which opens the page again.
func settingsCaptchaRefresh(t *testing.T, env *settingsEnv, msgID int64) {
	t.Helper()
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	for _, button := range settingsAllButtons(keyboard) {
		fields := settingsButtonFields(t, button)
		if fields["a"] == "op" && fields["p"] == settingsPageCaptcha {
			env.pressRaw(env.chat, env.admin, msgID, button.CallbackData)
			return
		}
	}
	t.Fatal("the Captcha page has no refresh button")
}

// settingsCaptchaSelected returns the values of a choice row whose label carries the
// selected prefix.
func settingsCaptchaSelected(t *testing.T, row []gotgbot.InlineKeyboardButton) []string {
	t.Helper()
	var selected []string
	for _, button := range row {
		if strings.HasPrefix(button.Text, settingsSelectedPrefix) {
			selected = append(selected, settingsButtonFields(t, button)["v"])
		}
	}
	return selected
}

// settingsCaptchaCallIndex is the position of the first recorded call of method at or after
// from, or -1.
func settingsCaptchaCallIndex(env *settingsEnv, method string, from int) int {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	for i := from; i < len(env.fake.calls); i++ {
		if env.fake.calls[i].Method == method {
			return i
		}
	}
	return -1
}

// settingsCaptchaCallCount is how many calls the fake has recorded so far.
func settingsCaptchaCallCount(env *settingsEnv) int {
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	return len(env.fake.calls)
}

func TestSettingsCaptchaPage(t *testing.T) {
	t.Run("a fresh chat", func(t *testing.T) {
		settingsCaptchaNoWeb(t)
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)

		last := -1
		for _, want := range []string{
			staffMarker("settings_captcha_status") + " ❌ " + staffMarker("common_status_off"),
			staffMarker("settings_captcha_mode") + " " + staffMarker("settings_opt_math"),
			staffMarker("captcha_mode_math_desc"),
			staffMarker("settings_captcha_time") + " 2",
			staffMarker("settings_captcha_action") + " " + staffMarker("settings_opt_kick"),
		} {
			at := strings.Index(text, want)
			if at < 0 {
				t.Fatalf("Captcha page %q does not contain %q", text, want)
			}
			if at <= last {
				t.Errorf("Captcha page %q lists %q out of order", text, want)
			}
			last = at
		}
		if strings.Contains(text, staffMarker("settings_captcha_fallback_note")) {
			t.Errorf("a math group's page carries the fallback note: %q", text)
		}

		if len(keyboard) != 5 {
			t.Fatalf("Captcha page has %d keyboard rows, want toggle, mode, time, action and navigation", len(keyboard))
		}
		admin := fmt.Sprint(env.admin.Id)

		toggle := settingsCaptchaRow(t, keyboard, "e")
		if len(toggle) != 1 || !strings.Contains(toggle[0].Text, staffMarker("settings_btn_captcha_toggle")) {
			t.Fatalf("toggle row = %+v, want one settings_btn_captcha_toggle button", toggle)
		}
		if got, want := fmt.Sprint(settingsButtonFields(t, toggle[0])), fmt.Sprint(map[string]string{"a": "tg", "u": admin, "p": "c", "t": "e", "v": "1"}); got != want {
			t.Errorf("toggle fields = %s, want %s", got, want)
		}

		mode := settingsCaptchaRow(t, keyboard, "m")
		if len(mode) != 3 {
			t.Fatalf("mode row = %+v, want Math, Text, Turnstile", mode)
		}
		for i, want := range []string{"math", "text", "turnstile"} {
			fields := settingsButtonFields(t, mode[i])
			if fields["a"] != "st" || fields["p"] != "c" || fields["u"] != admin || fields["v"] != want {
				t.Errorf("mode button %d fields = %v, want a=st p=c v=%s", i, fields, want)
			}
		}
		if got := settingsCaptchaSelected(t, mode); fmt.Sprint(got) != "[math]" {
			t.Errorf("selected modes = %v, want only math", got)
		}
		if !strings.Contains(mode[0].Text, staffMarker("settings_opt_math")) || !strings.Contains(mode[1].Text, staffMarker("settings_opt_text")) {
			t.Errorf("mode labels = %q, %q", mode[0].Text, mode[1].Text)
		}
		if !strings.HasPrefix(mode[2].Text, "🚫 ") || !strings.Contains(mode[2].Text, staffMarker("settings_opt_turnstile")) {
			t.Errorf("Turnstile label %q with no web captcha, want the 🚫 mark", mode[2].Text)
		}

		stepper := settingsCaptchaRow(t, keyboard, "t")
		if len(stepper) != 3 {
			t.Fatalf("time row = %+v, want a three button stepper", stepper)
		}

		action := settingsCaptchaRow(t, keyboard, "f")
		if len(action) != 3 {
			t.Fatalf("action row = %+v, want Kick, Ban, Mute", action)
		}
		for i, want := range []string{"kick", "ban", "mute"} {
			fields := settingsButtonFields(t, action[i])
			if fields["a"] != "st" || fields["p"] != "c" || fields["v"] != want {
				t.Errorf("action button %d fields = %v, want a=st p=c v=%s", i, fields, want)
			}
		}
		if got := settingsCaptchaSelected(t, action); fmt.Sprint(got) != "[kick]" {
			t.Errorf("selected actions = %v, want only kick", got)
		}

		nav := keyboard[len(keyboard)-1]
		if len(nav) != 2 || settingsButtonFields(t, nav[0])["a"] != "bk" || settingsButtonFields(t, nav[1])["a"] != "cl" {
			t.Errorf("navigation row = %+v, want [Back | Close]", nav)
		}
	})

	t.Run("Turnstile looks available when a runtime is installed", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsCaptchaInstallWeb(t, env)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		turnstile := settingsCaptchaButton(t, keyboard, "m", "turnstile")
		if strings.Contains(turnstile.Text, "🚫") {
			t.Errorf("Turnstile label %q carries the unavailable mark although a runtime is installed", turnstile.Text)
		}
	})

	t.Run("a stored Turnstile that cannot run adds the fallback note", func(t *testing.T) {
		settingsCaptchaNoWeb(t)
		env := newSettingsEnv(t)
		settingsCaptchaStoreMode(t, env.chat.Id, "turnstile")
		msgID := settingsOpenCaptchaPage(t, env)
		text, keyboard := env.lastEdit(env.chat.Id, msgID)

		mode := strings.Index(text, staffMarker("settings_captcha_mode"))
		note := strings.Index(text, staffMarker("settings_captcha_fallback_note"))
		timeLine := strings.Index(text, staffMarker("settings_captcha_time"))
		if mode < 0 || note < 0 || timeLine < 0 || !(mode < note && note < timeLine) {
			t.Fatalf("Captcha page %q: want the fallback note under the mode line and above the time line", text)
		}
		if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "m")); fmt.Sprint(got) != "[turnstile]" {
			t.Errorf("selected modes = %v, want the stored turnstile", got)
		}
	})

	t.Run("a stored Turnstile that can run has no note", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsCaptchaInstallWeb(t, env)
		settingsCaptchaStoreMode(t, env.chat.Id, "turnstile")
		msgID := settingsOpenCaptchaPage(t, env)
		text, _ := env.lastEdit(env.chat.Id, msgID)
		if strings.Contains(text, staffMarker("settings_captcha_fallback_note")) {
			t.Errorf("Captcha page %q carries the fallback note although the web captcha is available", text)
		}
	})
}

// settingsCaptchaPendingFixture creates a pending captcha attempt for one user and a muted
// member for another in chatID, and removes what is left of them at the end of the test.
func settingsCaptchaPendingFixture(t *testing.T, chatID int64) (attemptID uint, pendingUser, mutedUser int64) {
	t.Helper()
	pendingUser, mutedUser = 4301, 4302
	if err := captcha.SetCaptchaEnabled(chatID, true); err != nil {
		t.Fatalf("SetCaptchaEnabled: %v", err)
	}
	attempt, err := captcha.CreateCaptchaAttemptPreMessage(pendingUser, chatID, "7", 2)
	if err != nil {
		t.Fatalf("CreateCaptchaAttemptPreMessage: %v", err)
	}
	if err := captcha.UpdateCaptchaAttemptMessageID(attempt.ID, 123); err != nil {
		t.Fatalf("UpdateCaptchaAttemptMessageID: %v", err)
	}
	if err := captcha.CreateMutedUser(mutedUser, chatID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateMutedUser: %v", err)
	}
	t.Cleanup(func() {
		_ = captcha.DeleteCaptchaAttempt(pendingUser, chatID)
		_ = captcha.DeleteMutedUser(pendingUser, chatID)
		_ = captcha.DeleteMutedUser(mutedUser, chatID)
	})
	return attempt.ID, pendingUser, mutedUser
}

func TestSettingsCaptchaToggle(t *testing.T) {
	t.Run("on, then off releases the pending members", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		toggle := settingsCaptchaButton(t, keyboard, "e", "")
		if v := settingsButtonFields(t, toggle)["v"]; v != "1" {
			t.Fatalf("the toggle of a fresh chat targets %q, want 1", v)
		}

		answers := env.answerCount()
		env.pressRaw(env.chat, env.admin, msgID, toggle.CallbackData)
		if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || !row.Enabled {
			t.Fatalf("stored row after the toggle = %+v (row %v), want Enabled", row, ok)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the toggle answered the callback %d times, want once", got)
		}
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_captcha_status")+" ✅ "+staffMarker("common_status_on")) {
			t.Errorf("redrawn page %q does not show the On state", text)
		}
		if v := settingsButtonFields(t, settingsCaptchaButton(t, keyboard, "e", ""))["v"]; v != "0" {
			t.Errorf("redrawn toggle targets %q, want 0", v)
		}

		attemptID, _, mutedUser := settingsCaptchaPendingFixture(t, env.chat.Id)
		_, keyboard = env.lastEdit(env.chat.Id, msgID)
		off := settingsCaptchaButton(t, keyboard, "e", "0")
		start := settingsCaptchaCallCount(env)
		answers = env.answerCount()
		env.pressRaw(env.chat, env.admin, msgID, off.CallbackData)

		if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.Enabled {
			t.Errorf("stored row after Off = %+v (row %v), want not Enabled", row, ok)
		}
		if current, err := captcha.GetCaptchaAttemptByID(attemptID); err != nil || current != nil {
			t.Errorf("attempt after Off = %+v, %v; want released", current, err)
		}
		if users, err := captcha.GetMutedUsersForChat(env.chat.Id); err != nil || len(users) != 0 {
			t.Errorf("muted users after Off = %+v, %v (member %d); want none", users, err, mutedUser)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "restrictChatMember", env.chat.Id)); got != 2 {
			t.Errorf("restrictChatMember calls = %d, want both members restored as /captcha off does", got)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the Off press answered the callback %d times, want once", got)
		}
		answered := settingsCaptchaCallIndex(env, "answerCallbackQuery", start)
		restored := settingsCaptchaCallIndex(env, "restrictChatMember", start)
		if answered < 0 || restored < 0 || answered > restored {
			t.Errorf("answerCallbackQuery at call %d, first restrictChatMember at %d: the answer must come first", answered, restored)
		}
		text, _ = env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_captcha_status")+" ❌ "+staffMarker("common_status_off")) {
			t.Errorf("page after Off %q does not show Off", text)
		}
	})

	t.Run("a failed cleanup is a notice on a page that shows Off", func(t *testing.T) {
		env := newSettingsEnv(t)
		msgID := settingsOpenCaptchaPage(t, env)
		settingsCaptchaPendingFixture(t, env.chat.Id)
		settingsCaptchaRefresh(t, env, msgID)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		off := settingsCaptchaButton(t, keyboard, "e", "0")

		env.fake.script("restrictChatMember", env.chat.Id,
			staffFakeError(500, "Internal Server Error"), staffFakeError(500, "Internal Server Error"))
		env.pressRaw(env.chat, env.admin, msgID, off.CallbackData)

		text, _ := env.lastEdit(env.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("captcha_disable_failed")) {
			t.Errorf("page %q does not carry the command's captcha_disable_failed", text)
		}
		if !strings.Contains(text, staffMarker("settings_captcha_status")+" ❌ "+staffMarker("common_status_off")) {
			t.Errorf("page %q does not show the stored Off", text)
		}
		if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.Enabled {
			t.Errorf("stored row = %+v (row %v), want the switch saved Off", row, ok)
		}
	})
}

func TestSettingsCaptchaTurnstile(t *testing.T) {
	pressTurnstile := func(t *testing.T, env *settingsEnv) (msgID int64) {
		t.Helper()
		msgID = settingsOpenCaptchaPage(t, env)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		env.pressRaw(env.chat, env.admin, msgID, settingsCaptchaButton(t, keyboard, "m", "turnstile").CallbackData)
		return msgID
	}
	storedMode := func(t *testing.T, env *settingsEnv) string {
		t.Helper()
		row, ok := settingsCaptchaStored(t, env.chat.Id)
		if !ok {
			return ""
		}
		return row.CaptchaMode
	}

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, env *settingsEnv)
		key   string
	}{
		{"not configured", func(t *testing.T, env *settingsEnv) { settingsCaptchaNoWeb(t) }, "captcha_web_refuse_not_configured"},
		{"Redis is down", func(t *testing.T, env *settingsEnv) { settingsCaptchaInstallDeadWeb(t, env) }, "captcha_web_refuse_no_redis"},
		{"the bot is a plain member", func(t *testing.T, env *settingsEnv) {
			settingsCaptchaInstallWeb(t, env)
			env.fake.setBotRole(env.chat.Id, staffRoleMember)
		}, "captcha_web_refuse_no_rights"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSettingsEnv(t)
			tc.setup(t, env)
			msgID := pressTurnstile(t, env)

			text, _ := env.lastEdit(env.chat.Id, msgID)
			if !strings.Contains(text, staffMarker(tc.key)) {
				t.Errorf("page %q does not carry the notice %s", text, tc.key)
			}
			if got := storedMode(t, env); got == "turnstile" {
				t.Errorf("the refused press stored %q", got)
			}
		})
	}

	t.Run("an administrator bot with Restrict members stores it", func(t *testing.T) {
		env := newSettingsEnv(t)
		settingsCaptchaInstallWeb(t, env)
		env.fake.setBotRole(env.chat.Id, staffRoleAdmin)
		msgID := pressTurnstile(t, env)

		if got := storedMode(t, env); got != "turnstile" {
			t.Fatalf("stored mode = %q, want turnstile", got)
		}
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "m")); fmt.Sprint(got) != "[turnstile]" {
			t.Errorf("selected modes after the press = %v, want turnstile", got)
		}
	})
}
