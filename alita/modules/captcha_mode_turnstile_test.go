//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
)

// turnstileBotClient answers the bot's own getChatMember the way a test asks (a plain
// member, or an error) and hands every other call to the shared fake client.
type turnstileBotClient struct {
	*moduleBotClient
	botID     int64
	botStatus string // "member", "administrator" without rights, or "error"
}

func (c *turnstileBotClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method == "getChatMember" && fmt.Sprint(params["user_id"]) == fmt.Sprint(c.botID) {
		c.moduleBotClient.mu.Lock()
		c.moduleBotClient.calls = append(c.moduleBotClient.calls, moduleBotCall{Method: method, Params: params})
		c.moduleBotClient.mu.Unlock()
		switch c.botStatus {
		case "error":
			return nil, errors.New("getChatMember: network down")
		case "administrator":
			return json.RawMessage(fmt.Sprintf(
				`{"status":"administrator","user":{"id":%d,"is_bot":true,"first_name":"Alita"},"can_restrict_members":false,"can_delete_messages":true}`,
				c.botID)), nil
		default:
			return json.RawMessage(fmt.Sprintf(
				`{"status":"member","user":{"id":%d,"is_bot":true,"first_name":"Alita"}}`, c.botID)), nil
		}
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// turnstileModeBot is a bot with its own ID whose live member record is botStatus.
func turnstileModeBot(base *moduleBotClient, botStatus string) *gotgbot.Bot {
	const botID = int64(555)
	return &gotgbot.Bot{
		Token: "555:test",
		BotClient: &turnstileBotClient{
			moduleBotClient: base,
			botID:           botID,
			botStatus:       botStatus,
		},
		User: gotgbot.User{Id: botID, IsBot: true, Username: "AlitaTestBot"},
	}
}

func runCaptchaModeAs(bot *gotgbot.Bot, chat gotgbot.Chat, user gotgbot.User, text string) error {
	return captchaModule.captchaModeCommand(bot, newModuleMessageContext(bot, chat, user, text))
}

func TestCaptchaModeTurnstile(t *testing.T) {
	env := newCaptchaWebEnv(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}

	if err := runCaptchaModeAs(env.bot, env.chat, admin, "/captchamode math"); err != nil {
		t.Fatalf("/captchamode math error = %v", err)
	}
	assertCaptchaSettings(t, env.chat.Id, db.CaptchaSettings{CaptchaMode: "math", Timeout: 2, FailureAction: "kick", MaxAttempts: 3})

	before := len(env.client.callsFor("sendMessage"))
	if err := runCaptchaModeAs(env.bot, env.chat, admin, "/captchamode turnstile"); err != nil {
		t.Fatalf("/captchamode turnstile error = %v", err)
	}
	assertCaptchaSettings(t, env.chat.Id, db.CaptchaSettings{CaptchaMode: "turnstile", Timeout: 2, FailureAction: "kick", MaxAttempts: 3})

	sends := env.client.callsFor("sendMessage")
	if len(sends) != before+1 {
		t.Fatalf("sendMessage calls = %d, want one reply for the command", len(sends)-before)
	}
	text := fmt.Sprint(sends[len(sends)-1].Params["text"])
	for _, want := range []string{
		staffMarker("captcha_mode_set_formatted"), "turnstile", staffMarker("captcha_mode_turnstile_desc"),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reply %q does not contain %q", text, want)
		}
	}

	// The next join gets the Mini App button.
	if token := env.join(gotgbot.User{Id: 42, FirstName: "Member"}); token == "" {
		t.Fatal("a join after /captchamode turnstile got no challenge token")
	}

	// Math and text still work.
	for _, mode := range []string{"text", "math"} {
		if err := runCaptchaModeAs(env.bot, env.chat, admin, "/captchamode "+mode); err != nil {
			t.Fatalf("/captchamode %s error = %v", mode, err)
		}
		assertCaptchaSettings(t, env.chat.Id, db.CaptchaSettings{CaptchaMode: mode, Timeout: 2, FailureAction: "kick", MaxAttempts: 3})
	}
}

func TestCaptchaModeTurnstileRefusals(t *testing.T) {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	// start builds a Turnstile-ready env whose group is stored on math, and returns the
	// number of replies already sent.
	start := func(t *testing.T) (*captchaWebEnv, int) {
		t.Helper()
		env := newCaptchaWebEnv(t)
		if err := captcha.SetCaptchaMode(env.chat.Id, "math"); err != nil {
			t.Fatalf("SetCaptchaMode(math) error = %v", err)
		}
		return env, len(env.client.callsFor("sendMessage"))
	}
	// expectOnlyRefusal asserts exactly one reply carrying key and an untouched math mode.
	expectOnlyRefusal := func(t *testing.T, env *captchaWebEnv, before int, key string) {
		t.Helper()
		sends := env.client.callsFor("sendMessage")
		if len(sends) != before+1 {
			t.Fatalf("sendMessage calls = %d, want exactly one reply", len(sends)-before)
		}
		text := fmt.Sprint(sends[len(sends)-1].Params["text"])
		if !strings.Contains(text, staffMarker(key)) {
			t.Fatalf("reply %q does not use %s", text, key)
		}
		assertCaptchaSettings(t, env.chat.Id, db.CaptchaSettings{CaptchaMode: "math", Timeout: 2, FailureAction: "kick", MaxAttempts: 3})
	}

	t.Run("not configured", func(t *testing.T) {
		env, before := start(t)
		captchaWebCurrent.Store(nil)
		if err := runCaptchaModeAs(env.bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_not_configured")
	})

	t.Run("no Redis", func(t *testing.T) {
		env, before := start(t)
		// A runtime whose own Redis is gone; the shared cache Redis stays up.
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

		if err := runCaptchaModeAs(env.bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_no_redis")
	})

	t.Run("bot is a plain member", func(t *testing.T) {
		env, before := start(t)
		bot := turnstileModeBot(env.client, "member")
		if err := runCaptchaModeAs(bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_no_rights")
	})

	t.Run("bot is an administrator without Restrict members", func(t *testing.T) {
		env, before := start(t)
		bot := turnstileModeBot(env.client, "administrator")
		if err := runCaptchaModeAs(bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_no_rights")
	})

	t.Run("rights unknown", func(t *testing.T) {
		env, before := start(t)
		bot := turnstileModeBot(env.client, "error")
		if err := runCaptchaModeAs(bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_no_rights")
	})

	t.Run("order: not configured wins over no rights", func(t *testing.T) {
		env, before := start(t)
		captchaWebCurrent.Store(nil)
		bot := turnstileModeBot(env.client, "member")
		if err := runCaptchaModeAs(bot, env.chat, admin, "/captchamode turnstile"); err != nil {
			t.Fatalf("error = %v", err)
		}
		expectOnlyRefusal(t, env, before, "captcha_web_refuse_not_configured")
	})

	t.Run("not an admin", func(t *testing.T) {
		env, before := start(t)
		if err := runCaptchaModeAs(env.bot, env.chat, member, "/captchamode turnstile"); !errors.Is(err, ext.EndGroups) {
			t.Fatalf("error = %v, want ext.EndGroups", err)
		}
		sends := env.client.callsFor("sendMessage")
		for _, call := range sends[before:] {
			text := fmt.Sprint(call.Params["text"])
			for _, key := range []string{
				"captcha_web_refuse_not_configured", "captcha_web_refuse_no_redis", "captcha_web_refuse_no_rights",
				"captcha_mode_set_formatted",
			} {
				if strings.Contains(text, staffMarker(key)) {
					t.Fatalf("a non-admin got %s: %q", key, text)
				}
			}
		}
		assertCaptchaSettings(t, env.chat.Id, db.CaptchaSettings{CaptchaMode: "math", Timeout: 2, FailureAction: "kick", MaxAttempts: 3})
	})
}

// TestCaptchaModeListsTurnstileInEveryLanguage holds the four texts that list the captcha
// modes to the truth in all 7 languages, and checks the two replies that carry them.
func TestCaptchaModeListsTurnstileInEveryLanguage(t *testing.T) {
	for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
		raw, err := os.ReadFile("../../locales/" + code + ".yml")
		if err != nil {
			t.Fatalf("read %s.yml: %v", code, err)
		}
		var data map[string]any
		if err := yaml.Unmarshal(raw, &data); err != nil {
			t.Fatalf("parse %s.yml: %v", code, err)
		}
		for _, key := range []string{"captcha_mode_specify", "captcha_mode_invalid", "captcha_invalid_mode_error"} {
			text, _ := data[key].(string)
			for _, mode := range []string{"math", "text", "turnstile"} {
				if !strings.Contains(text, mode) {
					t.Errorf("%s: %s = %q does not name %q", code, key, text, mode)
				}
			}
		}
		help, _ := data["captcha_help_msg"].(string)
		if !strings.Contains(help, "turnstile") {
			t.Errorf("%s: captcha_help_msg does not name turnstile", code)
		}
	}

	withStaffLocale(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Captcha Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	for _, tc := range []struct{ text, key string }{
		{"/captchamode", "captcha_mode_specify"},
		{"/captchamode bogus", "captcha_mode_invalid"},
	} {
		before := len(client.callsFor("sendMessage"))
		if err := runCaptchaModeAs(bot, chat, admin, tc.text); err != nil {
			t.Fatalf("%s error = %v", tc.text, err)
		}
		sends := client.callsFor("sendMessage")
		if len(sends) != before+1 || !strings.Contains(fmt.Sprint(sends[len(sends)-1].Params["text"]), staffMarker(tc.key)) {
			t.Fatalf("%s did not reply with %s", tc.text, tc.key)
		}
	}
}
