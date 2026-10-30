//go:build testtools

package modules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

const captchaWebTestHost = "captcha.example.com"

// captchaWebEnv is one Turnstile group on a fresh miniredis with a fake Cloudflare. Every
// test builds its own, so the verify route's per-test limits (plan 08-08) are never shared.
type captchaWebEnv struct {
	t          *testing.T
	client     *moduleBotClient
	bot        *gotgbot.Bot
	chat       gotgbot.Chat
	mr         *miniredis.Miniredis
	redis      *redis.Client
	rt         *captchaWebRuntime
	secret     string
	siteverify atomic.Value // http.HandlerFunc
	cfRequests atomic.Int64
	cfLast     atomic.Value // string: the last request body Cloudflare's fake saw
}

func newCaptchaWebEnv(t *testing.T) *captchaWebEnv {
	t.Helper()
	withStaffLocale(t)

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	cache.SetRedisClientForTest(t, rc)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Captcha Chat"}
	// The web pass reads the chat from Telegram, so the fake must echo this chat's ID.
	client.responses["getChat"] = json.RawMessage(
		fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Captcha Chat"}`, chat.Id))
	if err := captcha.SetCaptchaEnabled(chat.Id, true); err != nil {
		t.Fatalf("SetCaptchaEnabled() error = %v", err)
	}
	// SetCaptchaMode accepts turnstile only from plan 08-04, so the stored mode is set directly.
	if err := db.DB.Model(&db.CaptchaSettings{}).Where("chat_id = ?", chat.Id).
		Update("captcha_mode", "turnstile").Error; err != nil {
		t.Fatalf("set turnstile mode error = %v", err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("captcha_settings", chat.Id))

	env := &captchaWebEnv{t: t, client: client, bot: bot, chat: chat, mr: mr, redis: rc}
	env.setSiteverify(captchaweb.SiteverifyPassForTest(captchaWebTestHost))
	fake := captchaweb.SiteverifyServerForTest(t, func(w http.ResponseWriter, r *http.Request) {
		env.cfRequests.Add(1)
		body, _ := io.ReadAll(r.Body)
		env.cfLast.Store(string(body))
		r.Body = io.NopCloser(bytes.NewReader(body))
		env.siteverify.Load().(http.HandlerFunc)(w, r)
	})
	env.secret = fake.Secret

	rt, err := newCaptchaWebRuntime(bot, CaptchaWebConfig{
		BaseURL:    "https://" + captchaWebTestHost,
		ShortName:  "verify",
		SiteKey:    "test-site-key",
		Secret:     fake.Secret,
		Redis:      rc,
		Siteverify: fake,
	})
	if err != nil {
		t.Fatalf("newCaptchaWebRuntime() error = %v", err)
	}
	env.rt = rt
	captchaWebCurrent.Store(rt)
	captchaWebProbeAt.Store(0)
	t.Cleanup(func() {
		captchaWebCurrent.Store(nil)
		captchaWebProbeAt.Store(0)
	})
	return env
}

func (e *captchaWebEnv) setSiteverify(h http.HandlerFunc) {
	e.siteverify.Store(h)
}

// join runs SendCaptcha for user as the join flow does and returns the challenge token
// read from the only URL button of the message it sent.
func (e *captchaWebEnv) join(user gotgbot.User) string {
	e.t.Helper()
	ctx := newModuleMessageContext(e.bot, e.chat, gotgbot.User{Id: 777000, FirstName: "Telegram"}, "join")
	if err := SendCaptcha(e.bot, ctx, user.Id, user.FirstName); err != nil {
		e.t.Fatalf("SendCaptcha(%d) error = %v", user.Id, err)
	}
	token := e.lastToken()
	if token == "" {
		e.t.Fatal("SendCaptcha sent no Mini App button")
	}
	return token
}

// lastToken is the startapp value of the newest URL button sent, or "" when there is none.
func (e *captchaWebEnv) lastToken() string {
	e.t.Helper()
	calls := e.client.callsFor("sendMessage")
	for i := len(calls) - 1; i >= 0; i-- {
		markup, ok := calls[i].Params["reply_markup"].(gotgbot.InlineKeyboardMarkup)
		if !ok {
			continue
		}
		for _, row := range markup.InlineKeyboard {
			for _, button := range row {
				if button.Url == "" {
					continue
				}
				parsed, err := url.Parse(button.Url)
				if err != nil {
					e.t.Fatalf("button url %q does not parse: %v", button.Url, err)
				}
				return parsed.Query().Get("startapp")
			}
		}
	}
	return ""
}

// verifyBody posts body to the verify handler and returns the status and the answer.
func (e *captchaWebEnv) verifyBody(body string) (int, captchaweb.Answer) {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/captcha/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.rt.handler.Verify(rec, req)

	var answer captchaweb.Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		e.t.Fatalf("verify answer %q is not JSON: %v", rec.Body.String(), err)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		e.t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		e.t.Fatalf("Content-Type = %q, want application/json", got)
	}
	return rec.Code, answer
}

// verifyJSON builds the verify body for user and the given pieces.
func (e *captchaWebEnv) verifyJSON(initData, turnstileToken, challenge string) string {
	e.t.Helper()
	body, err := json.Marshal(map[string]string{
		"init_data":       initData,
		"turnstile_token": turnstileToken,
		"challenge":       challenge,
	})
	if err != nil {
		e.t.Fatalf("marshal verify body: %v", err)
	}
	return string(body)
}

// verify posts the body a real page would post for user and token.
func (e *captchaWebEnv) verify(user gotgbot.User, token string) (int, captchaweb.Answer) {
	e.t.Helper()
	initData := captchaweb.InitDataForTest(e.bot.Token, user.Id, user.FirstName, "en", token, time.Now())
	return e.verifyBody(e.verifyJSON(initData, captchaweb.TurnstileTokenForTest(token), token))
}

// attempt is the live attempt row of userID in the env's chat, or nil.
func (e *captchaWebEnv) attempt(userID int64) *db.CaptchaAttempts {
	e.t.Helper()
	attempt, err := captcha.GetCaptchaAttempt(userID, e.chat.Id)
	if err != nil {
		e.t.Fatalf("GetCaptchaAttempt(%d) error = %v", userID, err)
	}
	return attempt
}

func (e *captchaWebEnv) challengeKey(token string) string {
	return "alita:captcha:ch:" + token
}

// callsWithMessageID returns the calls of method whose message_id is id.
func callsWithMessageID(client *moduleBotClient, method string, id int64) []moduleBotCall {
	var out []moduleBotCall
	for _, call := range client.callsFor(method) {
		if fmt.Sprint(call.Params["message_id"]) == fmt.Sprint(id) {
			out = append(out, call)
		}
	}
	return out
}

func permissionsOf(t *testing.T, call moduleBotCall) gotgbot.ChatPermissions {
	t.Helper()
	permissions, ok := call.Params["permissions"].(gotgbot.ChatPermissions)
	if !ok {
		t.Fatalf("permissions type = %T, want ChatPermissions", call.Params["permissions"])
	}
	return permissions
}

func TestCaptchaWebTracerJoinVerifyUnmute(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	other := gotgbot.User{Id: 43, FirstName: "Other"}

	t.Run("join posts a Mini App button", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)

		attempt := env.attempt(member.Id)
		if attempt == nil {
			t.Fatal("no captcha attempt row for the joiner")
		}
		if !captchaweb.IsReservedAnswer(attempt.Answer) || len(attempt.Answer) != 36 {
			t.Fatalf("attempt answer %q is not a ts1: reserved answer of 36 characters", attempt.Answer)
		}
		if len(token) != 22 || !captchaweb.ValidToken(token) {
			t.Fatalf("token %q is not 22 characters of [A-Za-z0-9_-]", token)
		}

		restricts := env.client.callsFor("restrictChatMember")
		if len(restricts) != 1 || permissionsOf(t, restricts[0]).CanSendMessages {
			t.Fatalf("restrictChatMember calls = %d, want one mute", len(restricts))
		}
		if photos := env.client.callsFor("sendPhoto"); len(photos) != 0 {
			t.Fatalf("sendPhoto calls = %d, want none for a Turnstile challenge", len(photos))
		}
		sends := env.client.callsFor("sendMessage")
		if len(sends) != 1 {
			t.Fatalf("sendMessage calls = %d, want exactly one challenge message", len(sends))
		}
		if text := fmt.Sprint(sends[0].Params["text"]); !strings.Contains(text, staffMarker("captcha_web_welcome")) {
			t.Fatalf("challenge text %q does not use captcha_web_welcome", text)
		}
		markup, ok := sends[0].Params["reply_markup"].(gotgbot.InlineKeyboardMarkup)
		if !ok || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
			t.Fatalf("reply_markup = %#v, want one row with one button", sends[0].Params["reply_markup"])
		}
		button := markup.InlineKeyboard[0][0]
		if want := captchaweb.MiniAppLink("AlitaTestBot", "verify", token); button.Url != want {
			t.Fatalf("button url = %q, want %q", button.Url, want)
		}
		if !strings.Contains(button.Text, staffMarker("captcha_web_button")) {
			t.Fatalf("button text %q does not use captcha_web_button", button.Text)
		}
		if attempt.MessageID != 9001 {
			t.Fatalf("attempt MessageID = %d, want the sent message id 9001", attempt.MessageID)
		}

		key := env.challengeKey(token)
		if !env.mr.Exists(key) {
			t.Fatalf("redis key %s does not exist", key)
		}
		if got := env.mr.HGet(key, "u"); got != "42" {
			t.Fatalf("challenge user = %q, want 42", got)
		}
		if got := env.mr.HGet(key, "a"); got != fmt.Sprint(attempt.ID) {
			t.Fatalf("challenge attempt = %q, want %d", got, attempt.ID)
		}
		if got := env.mr.HGet(key, "c"); got != fmt.Sprint(env.chat.Id) {
			t.Fatalf("challenge chat = %q, want %d", got, env.chat.Id)
		}
		ttl := env.mr.TTL(key)
		if ttl <= captchaweb.ChallengeGrace || ttl > time.Until(attempt.ExpiresAt)+captchaweb.ChallengeGrace+time.Second {
			t.Fatalf("challenge ttl = %v, want the attempt's time left plus %v", ttl, captchaweb.ChallengeGrace)
		}
	})

	t.Run("the target passes", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)

		status, answer := env.verify(member, token)
		if status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify = %d %q, want 200 passed", status, answer.Result)
		}
		if env.attempt(member.Id) != nil {
			t.Fatal("attempt row survived a pass")
		}
		restricts := env.client.callsFor("restrictChatMember")
		if len(restricts) != 2 || !permissionsOf(t, restricts[1]).CanSendMessages {
			t.Fatalf("restrictChatMember calls = %d, want the mute then an unmute", len(restricts))
		}
		if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
			t.Fatal("the challenge message was not deleted")
		}
		if got := env.cfRequests.Load(); got != 1 {
			t.Fatalf("Cloudflare requests = %d, want 1", got)
		}
		var sent struct {
			Secret   string `json:"secret"`
			Response string `json:"response"`
		}
		last, _ := env.cfLast.Load().(string)
		if err := json.Unmarshal([]byte(last), &sent); err != nil {
			t.Fatalf("Cloudflare request %q is not JSON: %v", last, err)
		}
		if sent.Response != captchaweb.TurnstileTokenForTest(token) || sent.Secret != env.secret {
			t.Fatalf("Cloudflare request = %+v, want the page's token and the configured secret", sent)
		}
		if env.mr.Exists(env.challengeKey(token)) {
			t.Fatal("the challenge key survived a pass")
		}
		if sends := env.client.callsFor("sendMessage"); len(sends) < 2 {
			t.Fatalf("sendMessage calls = %d, want the challenge plus the success note", len(sends))
		}
	})

	t.Run("held messages are summarised", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		attempt := env.attempt(member.Id)
		if err := captcha.StoreMessageForCaptcha(member.Id, env.chat.Id, attempt.ID, db.TEXT, "blocked text", "", ""); err != nil {
			t.Fatalf("StoreMessageForCaptcha() error = %v", err)
		}

		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify = %d %q, want 200 passed", status, answer.Result)
		}
		var summarised bool
		for _, call := range env.client.callsFor("sendMessage") {
			if strings.Contains(fmt.Sprint(call.Params["text"]), staffMarker("captcha_stored_messages_summary")) {
				summarised = true
			}
		}
		if !summarised {
			t.Fatal("no held-message summary was posted")
		}
	})

	t.Run("another member cannot pass or burn it", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		before := env.attempt(member.Id)

		status, answer := env.verify(other, token)
		if status != http.StatusOK || answer.Result != captchaweb.ResultNotForYou {
			t.Fatalf("verify(other) = %d %q, want 200 not_for_you", status, answer.Result)
		}
		if !env.mr.Exists(env.challengeKey(token)) || env.mr.HGet(env.challengeKey(token), "u") != "42" {
			t.Fatal("another member's verify changed the challenge")
		}
		after := env.attempt(member.Id)
		if after == nil || after.ID != before.ID || after.Answer != before.Answer || after.Attempts != before.Attempts {
			t.Fatalf("attempt changed: before %+v after %+v", before, after)
		}
		if restricts := env.client.callsFor("restrictChatMember"); len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls = %d, want only the join mute", len(restricts))
		}
		if got := env.cfRequests.Load(); got != 0 {
			t.Fatalf("Cloudflare requests = %d, want none for another member", got)
		}

		if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify(target) after = %d %q, want 200 passed", status, answer.Result)
		}
	})

	t.Run("a token Cloudflare rejects never unmutes", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		env.setSiteverify(captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`))

		status, answer := env.verify(member, token)
		if status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
			t.Fatalf("verify = %d %q, want 200 retry", status, answer.Result)
		}
		if env.attempt(member.Id) == nil {
			t.Fatal("attempt row was removed by a rejected Turnstile token")
		}
		if restricts := env.client.callsFor("restrictChatMember"); len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls = %d, want only the join mute", len(restricts))
		}
	})

	t.Run("a malformed body is invalid", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		signed := captchaweb.InitDataForTest(env.bot.Token, member.Id, member.FirstName, "en", token, time.Now())
		goodToken := captchaweb.TurnstileTokenForTest(token)

		cases := map[string]string{
			"empty turnstile token": env.verifyJSON(signed, "", token),
			"challenge with space":  env.verifyJSON(signed, goodToken, "a b"),
			"unknown field":         `{"init_data":"x","turnstile_token":"t","challenge":"abc","extra":1}`,
			"not json":              `not json`,
			"empty init data":       env.verifyJSON("", goodToken, token),
		}
		for name, body := range cases {
			status, answer := env.verifyBody(body)
			if status != http.StatusBadRequest || answer.Result != captchaweb.ResultInvalid {
				t.Fatalf("%s: verify = %d %q, want 400 invalid", name, status, answer.Result)
			}
		}
		if !env.mr.Exists(env.challengeKey(token)) || env.attempt(member.Id) == nil {
			t.Fatal("a malformed body changed the challenge or the attempt")
		}
		if got := env.cfRequests.Load(); got != 0 {
			t.Fatalf("Cloudflare requests = %d, want none for malformed bodies", got)
		}

		req := httptest.NewRequest(http.MethodGet, "/captcha/verify", nil)
		rec := httptest.NewRecorder()
		env.rt.handler.Verify(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET status = %d, want 405", rec.Code)
		}
	})

	t.Run("stale initData is invalid", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		token := env.join(member)
		stale := captchaweb.InitDataForTest(env.bot.Token, member.Id, member.FirstName, "en", token, time.Now().Add(-6*time.Minute))

		status, answer := env.verifyBody(env.verifyJSON(stale, captchaweb.TurnstileTokenForTest(token), token))
		if status != http.StatusOK || answer.Result != captchaweb.ResultInvalid {
			t.Fatalf("verify(stale) = %d %q, want 200 invalid", status, answer.Result)
		}
		if !env.mr.Exists(env.challengeKey(token)) || env.attempt(member.Id) == nil {
			t.Fatal("stale initData changed the challenge or the attempt")
		}
	})
}
