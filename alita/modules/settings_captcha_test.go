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
	"github.com/divkix/Alita_Robot/alita/i18n"
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

// settingsCaptchaSettingsLines are the three lines of /captcha's settings reply for the
// given values, in the real English locale.
func settingsCaptchaSettingsLines(t *testing.T, status, mode, action string) []string {
	t.Helper()
	tr := i18n.MustNewTranslator("en")
	var lines []string
	for _, line := range []struct{ key, value string }{
		{"captcha_settings_status", status},
		{"captcha_settings_mode", mode},
		{"captcha_settings_failure_action", action},
	} {
		text, err := tr.GetString(line.key, i18n.TranslationParams{"s": line.value})
		if err != nil || text == "" {
			t.Fatalf("real %s text: %q, %v", line.key, text, err)
		}
		lines = append(lines, text)
	}
	return lines
}

func TestSettingsParityCaptcha(t *testing.T) {
	settingsCaptchaNoWeb(t)
	env := newSettingsEnvWith(t, settingsEnvOptions{RealLocale: true})
	msgID := settingsOpenCaptchaPage(t, env)

	// pageShows redraws the page from the stored row and returns its keyboard.
	pageShows := func(t *testing.T) [][]gotgbot.InlineKeyboardButton {
		t.Helper()
		settingsCaptchaRefresh(t, env, msgID)
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		return keyboard
	}
	// pressMenu presses the button of control t that sets value v on the current page.
	pressMenu := func(t *testing.T, control, value string) {
		t.Helper()
		_, keyboard := env.lastEdit(env.chat.Id, msgID)
		env.pressRaw(env.chat, env.admin, msgID, settingsCaptchaButton(t, keyboard, control, value).CallbackData)
	}
	// stored is the row both sides must agree with, read past every cache.
	stored := func(t *testing.T) models.CaptchaSettings {
		t.Helper()
		dbcache.ResetLocalForTest()
		row, ok := settingsCaptchaStored(t, env.chat.Id)
		if !ok {
			t.Fatal("no captcha_settings row")
		}
		return row
	}

	// (1) the command writes, the page shows it.
	env.send(env.admin, "/captcha on")
	env.send(env.admin, "/captchamode text")
	env.send(env.admin, "/captchaaction ban")
	keyboard := pageShows(t)
	if v := settingsButtonFields(t, settingsCaptchaButton(t, keyboard, "e", ""))["v"]; v != "0" {
		t.Errorf("after /captcha on the toggle targets %q, want 0 (it is On)", v)
	}
	if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "m")); fmt.Sprint(got) != "[text]" {
		t.Errorf("after /captchamode text the page selects %v", got)
	}
	if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "f")); fmt.Sprint(got) != "[ban]" {
		t.Errorf("after /captchaaction ban the page selects %v", got)
	}
	if row := stored(t); !row.Enabled || row.CaptchaMode != "text" || row.FailureAction != "ban" {
		t.Errorf("stored row = %+v after the three commands", row)
	}

	// (2) the menu writes, /captcha shows it.
	pressMenu(t, "e", "0")
	pressMenu(t, "m", "math")
	pressMenu(t, "f", "mute")
	env.send(env.admin, "/captcha")
	env.wantReplyHas(settingsCaptchaSettingsLines(t, "disabled", "math", "mute")...)
	if row := stored(t); row.Enabled || row.CaptchaMode != "math" || row.FailureAction != "mute" {
		t.Errorf("stored row = %+v after the three menu presses", row)
	}

	// (3) the menu sets the other values, and the same row shows on both sides.
	pressMenu(t, "e", "1")
	pressMenu(t, "m", "text")
	pressMenu(t, "f", "ban")
	env.send(env.admin, "/captcha")
	env.wantReplyHas(settingsCaptchaSettingsLines(t, "enabled", "text", "ban")...)
	row := stored(t)
	if !row.Enabled || row.CaptchaMode != "text" || row.FailureAction != "ban" {
		t.Errorf("stored row = %+v after the menu set enabled, text and ban", row)
	}
	keyboard = pageShows(t)
	if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "m")); fmt.Sprint(got) != "[text]" {
		t.Errorf("the page selects %v for the stored text", got)
	}
	if got := settingsCaptchaSelected(t, settingsCaptchaRow(t, keyboard, "f")); fmt.Sprint(got) != "[ban]" {
		t.Errorf("the page selects %v for the stored ban", got)
	}
}

func TestSettingsDriftCaptcha(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		control string
		value   string
		// refusal is the locale key both sides refuse with; empty when both store the value.
		refusal string
	}{
		{"mode image", "/captchamode image", "m", "image", "captcha_mode_invalid"},
		{"mode math", "/captchamode math", "m", "math", ""},
		{"mode turnstile without a web captcha", "/captchamode turnstile", "m", "turnstile", "captcha_web_refuse_not_configured"},
		{"action warn", "/captchaaction warn", "f", "warn", "captcha_action_invalid"},
		{"action kick", "/captchaaction kick", "f", "kick", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settingsCaptchaNoWeb(t)
			env := newSettingsEnv(t)
			commandChat := env.chat
			menuChat := env.addChat()

			env.send(env.admin, tc.command)
			if tc.refusal != "" {
				env.wantReplyHas(staffMarker(tc.refusal))
			}

			fields := map[string]string{"a": "st", "u": fmt.Sprint(env.admin.Id), "p": "c", "t": tc.control, "v": tc.value}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("hand-built press %v did not encode", fields)
			}
			const menuMsgID = 4343
			env.pressRaw(menuChat, env.admin, menuMsgID, data)

			commandRow, commandHas := settingsCaptchaStored(t, commandChat.Id)
			menuRow, menuHas := settingsCaptchaStored(t, menuChat.Id)
			if commandHas != menuHas || commandRow.CaptchaMode != menuRow.CaptchaMode || commandRow.FailureAction != menuRow.FailureAction {
				t.Fatalf("command stored (%+v, row %v), menu stored (%+v, row %v): they disagree", commandRow, commandHas, menuRow, menuHas)
			}
			if tc.refusal == "" {
				if !menuHas {
					t.Fatalf("both sides refused %s, want %s stored", tc.command, tc.value)
				}
				got := map[string]string{"m": menuRow.CaptchaMode, "f": menuRow.FailureAction}[tc.control]
				if got != tc.value {
					t.Errorf("stored %q, want %q", got, tc.value)
				}
				return
			}
			if menuHas && (menuRow.CaptchaMode == tc.value || menuRow.FailureAction == tc.value) {
				t.Errorf("the refused value %q was stored: %+v", tc.value, menuRow)
			}
			text, _ := env.lastEdit(menuChat.Id, menuMsgID)
			if !strings.Contains(text, staffMarker(tc.refusal)) {
				t.Errorf("the menu redrew %q, want a notice with the command's own %s", text, tc.refusal)
			}
		})
	}

	t.Run("a value of no word shape is an invalid request", func(t *testing.T) {
		env := newSettingsEnv(t)
		for _, bad := range []string{"Math", "m4th", "turn-stile", "mathmathmathmathmath"} {
			fields := map[string]string{"a": "st", "u": fmt.Sprint(env.admin.Id), "p": "c", "t": "m", "v": bad}
			data := encodeCallbackData(settingsCallbackNamespace, fields)
			if data == "" {
				t.Fatalf("hand-built press %v did not encode", fields)
			}
			env.pressRaw(env.chat, env.admin, 4344, data)
			answer, alert := env.lastAnswer()
			if !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("value %q answered %q (alert %v), want the invalid request alert", bad, answer, alert)
			}
		}
		if row, ok := settingsCaptchaStored(t, env.chat.Id); ok && row.CaptchaMode != "math" {
			t.Errorf("a malformed value changed the mode to %q", row.CaptchaMode)
		}
	})
}

func TestSettingsAuthorityParityCaptcha(t *testing.T) {
	settingsCaptchaNoWeb(t)
	env := newSettingsEnv(t)
	// The bot is a plain member of the group from the start, so its role is never cached
	// as an administrator.
	env.fake.setBotRole(env.chat.Id, staffRoleMember)
	plain := gotgbot.User{Id: env.admin.Id + 31, FirstName: "Plain"}
	env.fake.setMember(env.chat.Id, plain.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	// A plain member is refused by both, and nothing is stored.
	env.send(plain, "/captchamode text")
	env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
	env.pressPrivate(plain, 7001, env.privateFields(env.chat.Id, "a", "st", "p", "c", "t", "m", "v", "text"))
	if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_not_admin")) {
		t.Errorf("a plain member's private press answered %q (alert %v), want the alert settings_not_admin", answer, alert)
	}
	if row, ok := settingsCaptchaStored(t, env.chat.Id); ok && row.CaptchaMode != "math" {
		t.Errorf("a plain member changed the mode to %q", row.CaptchaMode)
	}

	// /captcha on needs the bot as an administrator; the menu refuses the same way.
	env.send(env.admin, "/captcha on")
	env.wantReplyHas(staffMarker("chat_status_bot_not_admin"))
	msgID := settingsOpenCaptchaPage(t, env)
	_, keyboard := env.lastEdit(env.chat.Id, msgID)
	env.pressRaw(env.chat, env.admin, msgID, settingsCaptchaButton(t, keyboard, "e", "").CallbackData)
	text, _ := env.lastEdit(env.chat.Id, msgID)
	if !strings.Contains(text, staffMarker("chat_status_bot_not_admin")) {
		t.Errorf("the refused toggle redrew %q, want the notice chat_status_bot_not_admin", text)
	}
	if row, ok := settingsCaptchaStored(t, env.chat.Id); ok && row.Enabled {
		t.Errorf("a refused /captcha on or toggle stored Enabled: %+v", row)
	}

	// The mode needs only the admin: both succeed whatever the bot's role is.
	env.send(env.admin, "/captchamode text")
	if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.CaptchaMode != "text" {
		t.Fatalf("/captchamode text stored %+v (row %v), want text", row, ok)
	}
	settingsCaptchaRefresh(t, env, msgID)
	_, keyboard = env.lastEdit(env.chat.Id, msgID)
	env.pressRaw(env.chat, env.admin, msgID, settingsCaptchaButton(t, keyboard, "m", "math").CallbackData)
	if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.CaptchaMode != "math" {
		t.Errorf("the menu's Math stored %+v (row %v) with the bot a plain member, want math", row, ok)
	}
	_, keyboard = env.lastEdit(env.chat.Id, msgID)
	env.pressRaw(env.chat, env.admin, msgID, settingsCaptchaButton(t, keyboard, "m", "text").CallbackData)
	if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.CaptchaMode != "text" {
		t.Errorf("the menu's Text stored %+v (row %v) with the bot a plain member, want text", row, ok)
	}
}

func TestSettingsCaptchaCallbackBytes(t *testing.T) {
	tr := i18n.MustNewTranslator("en")
	for _, variant := range []struct {
		name  string
		view  settingsView
		wantT int // exact size of the Turnstile choice, 0 for no check
	}{
		{"private", settingsView{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}, 53},
		{"group", settingsView{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: "G", OpenerName: "N"}, 0},
	} {
		t.Run(variant.name, func(t *testing.T) {
			sawTurnstile := false
			for _, web := range []settingsCaptchaWeb{{}, {Installed: true}, {Installed: true, Available: true}} {
				for _, enabled := range []bool{false, true} {
					for _, mode := range []string{"math", "text", "turnstile"} {
						for _, action := range []string{"kick", "ban", "mute"} {
							s := &models.CaptchaSettings{Enabled: enabled, CaptchaMode: mode, FailureAction: action, Timeout: 5}
							screen := renderSettingsCaptcha(variant.view, s, web, "")
							name := fmt.Sprintf("web %+v enabled %v %s %s", web, enabled, mode, action)
							if screen.Omitted != 0 {
								t.Errorf("%s omitted %d buttons", name, screen.Omitted)
							}
							buttons := settingsAllButtons(screen.Keyboard.InlineKeyboard)
							if len(buttons) != 12 {
								t.Errorf("%s has %d buttons, want toggle, 3 modes, 3 time, 3 actions, Back and Close", name, len(buttons))
							}
							for _, button := range buttons {
								if n := len(button.CallbackData); n < 1 || n > 64 {
									t.Errorf("%s button %q has %d bytes of callback data, want 1..64", name, button.Text, n)
								}
								fields := settingsButtonFields(t, button)
								if fields["t"] == "m" && fields["v"] == "turnstile" && variant.wantT != 0 {
									sawTurnstile = true
									if n := len(button.CallbackData); n != variant.wantT {
										t.Errorf("%s Turnstile choice has %d bytes, want exactly %d", name, n, variant.wantT)
									}
								}
							}
						}
					}
				}
			}
			if variant.wantT != 0 && !sawTurnstile {
				t.Error("no Turnstile choice was checked")
			}
		})
	}
}

func TestSettingsCaptchaPageFitsEveryLocale(t *testing.T) {
	withRealLocales(t)
	// 64 runes of characters that grow when escaped or take two UTF-16 units.
	title := strings.Repeat("&<ж😀", 16)
	for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			for _, view := range []settingsView{
				{Tr: tr, OwnerKey: "u", OwnerValue: "9999999999", GroupID: -1001234567890, GroupTitle: title, OpenerName: "N"},
				{Tr: tr, Private: true, OwnerKey: "c", OwnerValue: "-1001234567890", GroupID: -1001234567890, GroupTitle: title, OpenerName: "N"},
			} {
				// The longest refusal is the worst notice line.
				notice := ""
				for _, key := range []string{"captcha_web_refuse_not_configured", "captcha_web_refuse_no_redis", "captcha_web_refuse_no_rights", "chat_status_bot_not_admin"} {
					if line := settingsNoticeWarn(settingsText(tr, key)); len(line) > len(notice) {
						notice = line
					}
				}
				if notice == "" {
					t.Fatalf("%s: no refusal text", code)
				}
				s := &models.CaptchaSettings{Enabled: true, CaptchaMode: "turnstile", FailureAction: "mute", Timeout: 10}
				screen := renderSettingsCaptcha(view, s, settingsCaptchaWeb{}, notice)

				if got := panelUTF16Len(screen.Text); got > 3800 {
					t.Errorf("%s: the page is %d UTF-16 units, want at most 3800", code, got)
				}
				if note := settingsText(tr, "settings_captcha_fallback_note"); note == "" || !strings.Contains(screen.Text, note) {
					t.Errorf("%s: the page does not carry the fallback note", code)
				}
				for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
					if strings.TrimSpace(button.Text) == "" {
						t.Errorf("%s: a button has an empty label (data %q)", code, button.CallbackData)
					}
				}
			}
		})
	}
}
