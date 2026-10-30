//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

// callsForUser returns the calls of method whose user_id is userID.
func callsForUser(client *moduleBotClient, method string, userID int64) []moduleBotCall {
	var out []moduleBotCall
	for _, call := range client.callsFor(method) {
		if fmt.Sprint(call.Params["user_id"]) == fmt.Sprint(userID) {
			out = append(out, call)
		}
	}
	return out
}

// kicksOf counts the member-removing unbans (only_if_banned false) sent for userID: how a
// kick is applied in this codebase, one call that removes a current member and lets them
// rejoin.
func kicksOf(client *moduleBotClient, userID int64) int {
	var kicks int
	for _, call := range callsForUser(client, "unbanChatMember", userID) {
		if onlyIfBanned, _ := call.Params["only_if_banned"].(bool); !onlyIfBanned {
			kicks++
		}
	}
	return kicks
}

// sentTextContaining counts the sendMessage calls whose text contains marker.
func sentTextContaining(client *moduleBotClient, marker string) int {
	var n int
	for _, call := range client.callsFor("sendMessage") {
		if strings.Contains(fmt.Sprint(call.Params["text"]), marker) {
			n++
		}
	}
	return n
}

// unmutesOf counts the restrictChatMember calls that give userID the right to send messages.
func unmutesOf(t *testing.T, client *moduleBotClient, userID int64) int {
	t.Helper()
	var n int
	for _, call := range callsForUser(client, "restrictChatMember", userID) {
		if permissionsOf(t, call).CanSendMessages {
			n++
		}
	}
	return n
}

// outcomeReplica is a second bot process of the same deployment: its own Telegram client and
// bot, its own web captcha runtime, and the database and Redis of the env it was built from.
type outcomeReplica struct {
	client *moduleBotClient
	bot    *gotgbot.Bot
	rt     *captchaWebRuntime
}

// primary is the env's own replica.
func (e *captchaWebEnv) primary() *outcomeReplica {
	return &outcomeReplica{client: e.client, bot: e.bot, rt: e.rt}
}

// newOutcomeReplica starts another replica on the env's database, Redis and fake Cloudflare.
// botID is its bot's user ID: the fake answers 999 as an administrator with every right and
// any other ID as a plain member. The replica is not installed as the current runtime.
func newOutcomeReplica(t *testing.T, env *captchaWebEnv, botID int64) *outcomeReplica {
	t.Helper()
	client := newModuleBotClient()
	client.mu.Lock()
	client.responses["getChat"] = json.RawMessage(
		fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Captcha Chat"}`, env.chat.Id))
	client.mu.Unlock()
	bot := newModuleTestBot(client)
	bot.Id = botID
	rt, err := newCaptchaWebRuntime(bot, CaptchaWebConfig{
		BaseURL:    "https://" + captchaWebTestHost,
		ShortName:  "verify",
		SiteKey:    "test-site-key",
		Secret:     env.secret,
		Redis:      env.redis,
		Siteverify: env.rt.cfg.Siteverify,
	})
	if err != nil {
		t.Fatalf("newCaptchaWebRuntime(replica) error = %v", err)
	}
	return &outcomeReplica{client: client, bot: bot, rt: rt}
}

// verify posts what a real page posts for user and token to this replica's handler. It
// returns an error instead of failing, so goroutines can use it.
func (r *outcomeReplica) verify(user gotgbot.User, token string) (int, captchaweb.Answer, error) {
	initData := captchaweb.InitDataForTest(r.bot.Token, user.Id, user.FirstName, "en", token, time.Now())
	body, err := json.Marshal(map[string]string{
		"init_data":       initData,
		"turnstile_token": captchaweb.TurnstileTokenForTest(token),
		"challenge":       token,
	})
	if err != nil {
		return 0, captchaweb.Answer{}, err
	}
	req := httptest.NewRequest(http.MethodPost, "/captcha/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.rt.handler.Verify(rec, req)
	var answer captchaweb.Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		return rec.Code, answer, fmt.Errorf("verify answer is not JSON: %w", err)
	}
	return rec.Code, answer, nil
}

// outcomeRace joins iterations fresh members and, for each, starts the timeout of the
// attempt on timer and the member's verify on checker at the same moment. Whatever the
// interleaving, exactly one of two things must have happened to each member: they passed and
// were unmuted once with nobody kicked, or they were kicked once and the check answered
// expired with the action (CAPT-07, D-16).
func outcomeRace(t *testing.T, env *captchaWebEnv, timer, checker *outcomeReplica, iterations int) {
	t.Helper()
	clients := []*moduleBotClient{env.client}
	for _, r := range []*outcomeReplica{timer, checker} {
		known := false
		for _, c := range clients {
			known = known || c == r.client
		}
		if !known {
			clients = append(clients, r.client)
		}
	}

	var passed, expired int
	for i := 0; i < iterations; i++ {
		member := gotgbot.User{Id: int64(6000 + i), FirstName: "Racer"}
		token := env.join(member)
		attempt := env.attempt(member.Id)
		if attempt == nil {
			t.Fatalf("iteration %d: no attempt after the join", i)
		}

		var (
			wg         sync.WaitGroup
			start      = make(chan struct{})
			status     int
			answer     captchaweb.Answer
			verifyErr  error
			timerWon   bool
			timerError error
		)
		wg.Add(2)
		// The timeout starts a little later each iteration, so over the run the check
		// reaches its claim both before and after the timeout does.
		lag := time.Duration(i) * 600 * time.Microsecond
		go func() {
			defer wg.Done()
			<-start
			time.Sleep(lag)
			timerWon, timerError = expireCaptchaAttempt(timer.bot, attempt)
		}()
		go func() {
			defer wg.Done()
			<-start
			status, answer, verifyErr = checker.verify(member, token)
		}()
		close(start)
		wg.Wait()

		if verifyErr != nil || timerError != nil {
			t.Fatalf("iteration %d: verify error = %v, timeout error = %v", i, verifyErr, timerError)
		}
		var kicks, unmutes int
		for _, c := range clients {
			kicks += kicksOf(c, member.Id)
			unmutes += unmutesOf(t, c, member.Id)
		}
		switch {
		case answer.Result == captchaweb.ResultPassed:
			passed++
			if status != http.StatusOK || kicks != 0 || unmutes != 1 || timerWon {
				t.Fatalf("iteration %d: passed, but status %d, kicks %d, unmutes %d, timeout claimed %v; want 200, 0, 1, false",
					i, status, kicks, unmutes, timerWon)
			}
		case answer.Result == captchaweb.ResultExpired:
			expired++
			if status != http.StatusOK || answer.Action != "kick" || kicks != 1 || unmutes != 0 || !timerWon {
				t.Fatalf("iteration %d: expired, but status %d, action %q, kicks %d, unmutes %d, timeout claimed %v; want 200, kick, 1, 0, true",
					i, status, answer.Action, kicks, unmutes, timerWon)
			}
		default:
			t.Fatalf("iteration %d: verify = %d %+v, want passed or expired", i, status, answer)
		}
		if env.attemptRow(member.Id) != nil {
			t.Fatalf("iteration %d: the attempt row survived", i)
		}
	}
	t.Logf("%d passed, %d expired out of %d", passed, expired, iterations)
}

// withCaptchaLifecycleForTest runs the captcha lifecycle's task bookkeeping for the test and
// puts the saved state back afterwards, so StopCaptchaLifecycle can be called inside.
func withCaptchaLifecycleForTest(t *testing.T) {
	t.Helper()
	captchaLifecycleMu.Lock()
	savedCtx, savedStop, savedDone := captchaLifecycleCtx, captchaLifecycleStop, captchaLifecycleDone
	ctx, cancel := context.WithCancel(context.Background())
	captchaLifecycleCtx, captchaLifecycleStop, captchaLifecycleDone = ctx, cancel, false
	captchaLifecycleMu.Unlock()

	t.Cleanup(func() {
		cancel()
		captchaLifecycleWG.Wait()
		captchaLifecycleMu.Lock()
		captchaLifecycleCtx, captchaLifecycleStop, captchaLifecycleDone = savedCtx, savedStop, savedDone
		captchaLifecycleMu.Unlock()
	})
}

// gatedBotClient is a hand-written gotgbot.BotClient that holds back one send, the success
// note, until released, and passes everything else to the recording client.
type gatedBotClient struct {
	inner   *moduleBotClient
	marker  string
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedBotClient(inner *moduleBotClient, marker string) *gatedBotClient {
	return &gatedBotClient{inner: inner, marker: marker, reached: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedBotClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method == "sendMessage" && strings.Contains(fmt.Sprint(params["text"]), g.marker) {
		g.once.Do(func() { close(g.reached) })
		<-g.release
	}
	return g.inner.RequestWithContext(ctx, token, method, params, opts)
}

func (g *gatedBotClient) GetAPIURL(opts *gotgbot.RequestOpts) string { return g.inner.GetAPIURL(opts) }

func (g *gatedBotClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return g.inner.FileURL(token, path, opts)
}

func TestCaptchaWebPassAfterExpiryExpiresAtOnce(t *testing.T) {
	env := newCaptchaWebEnv(t)
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	token := env.join(member)
	attempt := env.attempt(member.Id)

	// No timer has run: the attempt's time is simply up.
	if err := db.DB.Model(&db.CaptchaAttempts{}).Where("id = ?", attempt.ID).
		Updates(map[string]any{
			"created_at": time.Now().Add(-2 * time.Minute),
			"expires_at": time.Now().Add(-time.Minute),
		}).Error; err != nil {
		t.Fatalf("expire the attempt: %v", err)
	}

	status, answer := env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultExpired || answer.Action != "kick" {
		t.Fatalf("verify = %d %+v, want 200 expired with action kick", status, answer)
	}
	if got := kicksOf(env.client, member.Id); got != 1 {
		t.Fatalf("kicks during the request = %d, want exactly one", got)
	}
	if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
		t.Fatal("the challenge message was not deleted")
	}
	if env.attemptRow(member.Id) != nil {
		t.Fatal("the attempt row survived the expiry")
	}
	if got := unmutesOf(t, env.client, member.Id); got != 0 {
		t.Fatalf("unmutes = %d, want none after the time was up", got)
	}
	if got := env.cfRequests.Load(); got != 0 {
		t.Fatalf("Cloudflare requests = %d, want none for an expired attempt", got)
	}

	// The challenge is spent: a second try from the same member gets the remembered answer
	// again (D-03) and acts on nothing, which the assertions below pin.
	if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultExpired || answer.Action != "kick" {
		t.Fatalf("second verify = %d %+v, want 200 expired with kick", status, answer)
	}
	if got := kicksOf(env.client, member.Id); got != 1 {
		t.Fatalf("kicks after the second verify = %d, want still one", got)
	}
}

func TestCaptchaWebTimeoutAndVerifyOnDifferentReplicas(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	t.Run("a challenge issued on A passes on B", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		replicaB := newOutcomeReplica(t, env, 999)
		token := env.join(member)

		status, answer, err := replicaB.verify(member, token)
		if err != nil || status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
			t.Fatalf("verify on B = %d %+v, %v; want 200 passed", status, answer, err)
		}
		if got := unmutesOf(t, replicaB.client, member.Id); got != 1 {
			t.Fatalf("unmutes made by B = %d, want 1", got)
		}
		if got := unmutesOf(t, env.client, member.Id); got != 0 {
			t.Fatalf("unmutes made by A = %d, want none", got)
		}
		if env.attempt(member.Id) != nil {
			t.Fatal("the attempt survived a pass on B")
		}
	})

	t.Run("the timeout on A races a check on B", func(t *testing.T) {
		env := newCaptchaWebEnv(t)
		outcomeRace(t, env, env.primary(), newOutcomeReplica(t, env, 999), 20)
	})
}

func TestCaptchaWebPassVersusTimeoutRace(t *testing.T) {
	env := newCaptchaWebEnv(t)
	outcomeRace(t, env, env.primary(), env.primary(), 20)
}

func TestCaptchaWebUnmuteFailureSchedulesRetry(t *testing.T) {
	env := newCaptchaWebEnv(t)
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	token := env.join(member)

	env.client.mu.Lock()
	env.client.errors["restrictChatMember"] = errors.New("telegram is down")
	env.client.mu.Unlock()

	status, answer := env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
		t.Fatalf("verify = %d %+v, want 200 retry, never passed", status, answer)
	}
	if env.attemptRow(member.Id) != nil {
		t.Fatal("the attempt row survived the claim")
	}
	row, err := captcha.GetMutedUser(member.Id, env.chat.Id)
	if err != nil || row == nil {
		t.Fatalf("captcha_muted_users row = %v, %v; want the durable retry", row, err)
	}
	if row.UnmuteAt.After(time.Now()) {
		t.Fatalf("unmute due at %v, want due now", row.UnmuteAt)
	}
	due, err := captcha.GetUsersToUnmute()
	if err != nil {
		t.Fatalf("GetUsersToUnmute() error = %v", err)
	}
	var found bool
	for _, u := range due {
		found = found || (u.UserID == member.Id && u.ChatID == env.chat.Id)
	}
	if !found {
		t.Fatal("the 5-minute worker would not find the member")
	}
	if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
		t.Fatal("the challenge message was not deleted")
	}
	if got := kicksOf(env.client, member.Id); got != 0 {
		t.Fatalf("kicks = %d, want none after a claimed pass", got)
	}

	// The pass is decided, so a second tap (Try again on the page) is told so, with no side
	// effect, instead of ending on a dead "invalid" screen (WR-02).
	calls := callCount(env.client)
	if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("second verify = %d %+v, want 200 passed (the remembered decision)", status, answer)
	}
	if got := callCount(env.client); got != calls {
		t.Fatalf("the replay made %d Telegram calls, want none", got-calls)
	}
	if got := env.cfRequests.Load(); got != 1 {
		t.Fatalf("Cloudflare requests = %d, want 1", got)
	}
}

func TestCaptchaWebBotWithoutRightsIsRetry(t *testing.T) {
	env := newCaptchaWebEnv(t)
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	token := env.join(member)
	before := env.attempt(member.Id)
	// The fake answers every ID but 999 as a plain member, so this bot cannot restrict.
	powerless := newOutcomeReplica(t, env, 998)

	status, answer, err := powerless.verify(member, token)
	if err != nil || status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
		t.Fatalf("verify = %d %+v, %v; want 200 retry", status, answer, err)
	}
	if got := env.cfRequests.Load(); got != 0 {
		t.Fatalf("Cloudflare requests = %d, want none before the bot's rights are known", got)
	}
	if !env.mr.Exists(env.challengeKey(token)) {
		t.Fatal("the challenge was not given back")
	}
	after := env.attempt(member.Id)
	if after == nil || after.ID != before.ID || after.Answer != before.Answer || after.Attempts != before.Attempts {
		t.Fatalf("attempt changed: before %+v after %+v", before, after)
	}
	if got := unmutesOf(t, powerless.client, member.Id) + unmutesOf(t, env.client, member.Id); got != 0 {
		t.Fatalf("unmutes = %d, want none", got)
	}
	if got := kicksOf(powerless.client, member.Id) + kicksOf(env.client, member.Id); got != 0 {
		t.Fatalf("kicks = %d, want none", got)
	}

	// An unknown answer is no better than a no.
	powerless.client.mu.Lock()
	powerless.client.errors["getChatMember"] = errors.New("flood wait")
	powerless.client.mu.Unlock()
	if status, answer, err := powerless.verify(member, token); err != nil || status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
		t.Fatalf("verify with an unreadable bot status = %d %+v, %v; want 200 retry", status, answer, err)
	}

	// The same token passes on a replica whose bot can restrict.
	if status, answer := env.verify(member, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("verify on a capable bot = %d %+v, want 200 passed", status, answer)
	}
}

func TestCaptchaWebFinishJoinsLifecycleDrain(t *testing.T) {
	env := newCaptchaWebEnv(t)
	withCaptchaLifecycleForTest(t)
	first := gotgbot.User{Id: 42, FirstName: "First"}
	second := gotgbot.User{Id: 43, FirstName: "Second"}

	successMarker := staffMarker("greetings_captcha_verified_success")
	gate := newGatedBotClient(env.client, successMarker)
	env.bot.BotClient = gate

	token := env.join(first)
	if status, answer := env.verify(first, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("verify = %d %+v, want 200 passed", status, answer)
	}
	select {
	case <-gate.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the finish step never reached the success note")
	}

	stopped := make(chan struct{})
	go func() {
		StopCaptchaLifecycle()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("StopCaptchaLifecycle returned while the finish step was still running")
	case <-time.After(300 * time.Millisecond):
	}
	close(gate.release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("StopCaptchaLifecycle did not return after the finish step ended")
	}
	if got := sentTextContaining(env.client, successMarker); got != 1 {
		t.Fatalf("success notes after the drain = %d, want 1", got)
	}
	if sends := env.client.callsFor("sendMessage"); len(sends) < 3 {
		t.Fatalf("sendMessage calls = %d, want the challenge, the success note and the greeting", len(sends))
	}

	// The lifecycle has stopped: the next pass finishes inline, before the handler returns.
	token = env.join(second)
	if status, answer := env.verify(second, token); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("verify after the stop = %d %+v, want 200 passed", status, answer)
	}
	if got := sentTextContaining(env.client, successMarker); got != 2 {
		t.Fatalf("success notes right after the second pass = %d, want 2 (finish ran inline)", got)
	}
}

func TestCaptchaWebFailedSolveAppliesAction(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	invalid := captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`)

	for _, action := range []string{"kick", "ban", "mute"} {
		t.Run(action, func(t *testing.T) {
			env := newCaptchaWebEnv(t)
			if err := captcha.SetCaptchaFailureAction(env.chat.Id, action); err != nil {
				t.Fatalf("SetCaptchaFailureAction() error = %v", err)
			}
			token := env.join(member)
			env.setSiteverify(invalid)

			status, answer := env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultFailed || answer.Action != action {
				t.Fatalf("verify = %d %+v, want 200 failed with action %s", status, answer, action)
			}

			switch action {
			case "kick":
				if got := kicksOf(env.client, member.Id); got != 1 {
					t.Fatalf("kicks = %d, want exactly one", got)
				}
				if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
					t.Fatalf("banChatMember calls = %d, want none for a kick", len(bans))
				}
			case "ban":
				if bans := callsForUser(env.client, "banChatMember", member.Id); len(bans) != 1 {
					t.Fatalf("banChatMember calls = %d, want exactly one", len(bans))
				}
				if got := kicksOf(env.client, member.Id); got != 0 {
					t.Fatalf("unbanChatMember calls = %d, want none after a ban", got)
				}
			case "mute":
				restricts := env.client.callsFor("restrictChatMember")
				if len(restricts) != 2 || permissionsOf(t, restricts[1]).CanSendMessages {
					t.Fatalf("restrictChatMember calls = %d, want the join mute then a mute that keeps messages off", len(restricts))
				}
				row, err := captcha.GetMutedUser(member.Id, env.chat.Id)
				if err != nil || row == nil {
					t.Fatalf("captcha_muted_users row = %v, %v; want the 24 h unmute", row, err)
				}
				if left := time.Until(row.UnmuteAt); left < 23*time.Hour || left > 25*time.Hour {
					t.Fatalf("unmute in %v, want about 24 h", left)
				}
				if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
					t.Fatalf("banChatMember calls = %d, want none for a mute", len(bans))
				}
			}

			if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
				t.Fatal("the challenge message was not deleted")
			}
			marker := staffMarker("captcha_timeout_failure_" + map[string]string{"kick": "kicked", "ban": "banned", "mute": "muted"}[action])
			if got := sentTextContaining(env.client, marker); got != 1 {
				t.Fatalf("failure notices with %s = %d, want one", marker, got)
			}
			if env.attempt(member.Id) != nil {
				t.Fatal("the attempt row survived a failed solve")
			}

			status, answer = env.verify(member, token)
			// One solve only: the repeat gets the remembered answer (D-03) and changes nothing.
			if status != http.StatusOK || answer.Result != captchaweb.ResultFailed || answer.Action != action {
				t.Fatalf("second verify = %d %+v, want 200 failed with %s (one solve only)", status, answer, action)
			}
			if got := env.cfRequests.Load(); got != 1 {
				t.Fatalf("Cloudflare requests = %d, want 1", got)
			}
		})
	}
}

func TestCaptchaWebCloudflareErrorIsNotAFailure(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"500", captchaweb.SiteverifyAnswerForTest(http.StatusInternalServerError, `{"error":"upstream"}`)},
		{"internal-error", captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["internal-error"]}`)},
		{"invalid-input-secret", captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-secret"]}`)},
		{"hostname mismatch", captchaweb.SiteverifyPassForTest("not-our-host.example.net")},
		{"network down", func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("the fake Cloudflare cannot hijack")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptchaWebEnv(t)
			token := env.join(member)
			before := env.attempt(member.Id)
			env.setSiteverify(tc.handler)

			status, answer := env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
				t.Fatalf("verify = %d %+v, want 200 retry", status, answer)
			}
			if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %d, want none for a Cloudflare fault", len(bans))
			}
			if got := kicksOf(env.client, member.Id); got != 0 {
				t.Fatalf("kicks = %d, want none for a Cloudflare fault", got)
			}
			if restricts := env.client.callsFor("restrictChatMember"); len(restricts) != 1 {
				t.Fatalf("restrictChatMember calls = %d, want only the join mute (the member stays muted)", len(restricts))
			}
			after := env.attempt(member.Id)
			if after == nil || after.ID != before.ID || after.Answer != before.Answer ||
				after.Attempts != before.Attempts || !after.ExpiresAt.Equal(before.ExpiresAt) {
				t.Fatalf("attempt changed: before %+v after %+v", before, after)
			}
			if !env.mr.Exists(env.challengeKey(token)) {
				t.Fatal("the challenge was not given back")
			}
			if got := env.mr.HGet(env.challengeKey(token), "u"); got != "42" {
				t.Fatalf("re-armed challenge user = %q, want 42", got)
			}

			env.setSiteverify(captchaweb.SiteverifyPassForTest(captchaWebTestHost))
			status, answer = env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
				t.Fatalf("verify after Cloudflare is back = %d %+v, want 200 passed", status, answer)
			}
			if env.attempt(member.Id) != nil {
				t.Fatal("attempt row survived the pass")
			}
			var rows int64
			if err := db.DB.Model(&models.CaptchaMutedUsers{}).
				Where("user_id = ? AND chat_id = ?", member.Id, env.chat.Id).Count(&rows).Error; err != nil || rows != 0 {
				t.Fatalf("captcha_muted_users rows = %d, %v; want none after the pass", rows, err)
			}
		})
	}
}

// TestCaptchaWebFailedActionNotAppliedIsNeutral pins WR-07: a failed solve whose failure action
// Telegram refused leaves the member unmuted and in the group, so the page is never told they
// were removed, and the remembered answer is the same neutral one.
func TestCaptchaWebFailedActionNotAppliedIsNeutral(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	env := newCaptchaWebEnv(t)
	if err := captcha.SetCaptchaFailureAction(env.chat.Id, "ban"); err != nil {
		t.Fatalf("SetCaptchaFailureAction() error = %v", err)
	}
	token := env.join(member)
	env.setSiteverify(captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`))
	env.client.mu.Lock()
	env.client.errors["banChatMember"] = errors.New("Bad Request: not enough rights to restrict")
	env.client.mu.Unlock()

	status, answer := env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultInvalid || answer.Action != "" {
		t.Fatalf("verify = %d %+v, want 200 invalid with no action", status, answer)
	}
	if env.attempt(member.Id) != nil {
		t.Fatal("the attempt row survived a claimed failed solve")
	}
	if got := unmutesOf(t, env.client, member.Id); got == 0 {
		t.Fatal("the member was not unmuted after the action failed")
	}
	if failures := sentTextContaining(env.client, staffMarker("captcha_timeout_failure_banned")); failures != 0 {
		t.Fatalf("removal notices = %d, want none for an action that did not apply", failures)
	}

	status, answer = env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultInvalid || answer.Action != "" {
		t.Fatalf("replay = %d %+v, want the same neutral answer", status, answer)
	}
}

// TestCaptchaWebFailedSolveSparesApprovedAndDisabled pins WR-05: a failed solve takes the
// decision the timer takes. A member approved mid-challenge, or a group that turned the captcha
// off, is released and unmuted instead of kicked, and the page is not told of a removal.
func TestCaptchaWebFailedSolveSparesApprovedAndDisabled(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	invalid := captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`)

	cases := []struct {
		name  string
		spare func(t *testing.T, env *captchaWebEnv)
	}{
		{"approved mid-challenge", func(t *testing.T, env *captchaWebEnv) {
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, 1, ""); err != nil {
				t.Fatalf("AddApprovedUser() error = %v", err)
			}
		}},
		{"captcha turned off mid-challenge", func(t *testing.T, env *captchaWebEnv) {
			if err := captcha.SetCaptchaEnabled(env.chat.Id, false); err != nil {
				t.Fatalf("SetCaptchaEnabled() error = %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptchaWebEnv(t)
			token := env.join(member)
			tc.spare(t, env)
			env.setSiteverify(invalid)

			status, answer := env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultInvalid || answer.Action != "" {
				t.Fatalf("verify = %d %+v, want 200 invalid with no action", status, answer)
			}
			if got := kicksOf(env.client, member.Id); got != 0 {
				t.Fatalf("kicks = %d, want none for a spared member", got)
			}
			if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %d, want none", len(bans))
			}
			if got := unmutesOf(t, env.client, member.Id); got == 0 {
				t.Fatal("the spared member was not unmuted")
			}
			if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
				t.Fatal("the challenge message was not deleted")
			}
			if got := sentTextContaining(env.client, staffMarker("captcha_timeout_failure_kicked")); got != 0 {
				t.Fatalf("removal notices = %d, want none", got)
			}
			if env.attempt(member.Id) != nil {
				t.Fatal("the attempt row survived")
			}
		})
	}
}

// beforeCallBotClient runs before once, before the first call of method, then passes every call
// to the recording client. It lets a test land a database write at an exact point of a request.
type beforeCallBotClient struct {
	inner  *moduleBotClient
	method string
	before func()
	once   sync.Once
}

func (b *beforeCallBotClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method == b.method {
		b.once.Do(b.before)
	}
	return b.inner.RequestWithContext(ctx, token, method, params, opts)
}

func (b *beforeCallBotClient) GetAPIURL(opts *gotgbot.RequestOpts) string {
	return b.inner.GetAPIURL(opts)
}

func (b *beforeCallBotClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return b.inner.FileURL(token, path, opts)
}

// TestCaptchaWebLostClaimOfLiveAttemptIsRetry pins WR-01 for a pass: the claim also matches the
// message ID read a moment before, and SendCaptcha stores that ID after its send returns. A tap
// in that window loses the claim while the attempt is still live; the member must be told to
// try again, not that their time ran out, and the challenge must be given back.
func TestCaptchaWebLostClaimOfLiveAttemptIsRetry(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	env := newCaptchaWebEnv(t)
	token := env.join(member)
	attempt := env.attempt(member.Id)
	stored := attempt.MessageID

	// The tap arrives before SendCaptcha stored the message ID, and the store lands while the
	// pass reads the chat (between its read of the attempt and its claim).
	if err := db.DB.Model(&models.CaptchaAttempts{}).Where("id = ?", attempt.ID).Update("message_id", 0).Error; err != nil {
		t.Fatalf("clear message_id: %v", err)
	}
	env.bot.BotClient = &beforeCallBotClient{inner: env.client, method: "getChat", before: func() {
		if err := captcha.UpdateCaptchaAttemptMessageID(attempt.ID, stored); err != nil {
			t.Errorf("UpdateCaptchaAttemptMessageID() error = %v", err)
		}
	}}

	status, answer := env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultRetry || answer.Action != "" {
		t.Fatalf("verify = %d %+v, want 200 retry (the attempt is still live)", status, answer)
	}
	if env.attempt(member.Id) == nil {
		t.Fatal("the attempt row was lost")
	}
	if got := kicksOf(env.client, member.Id); got != 0 {
		t.Fatalf("kicks = %d, want none", got)
	}
	if !env.mr.Exists(env.challengeKey(token)) {
		t.Fatal("the challenge was not given back")
	}

	status, answer = env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("second verify = %d %+v, want 200 passed", status, answer)
	}
	if env.attempt(member.Id) != nil {
		t.Fatal("the attempt row survived the pass")
	}
}

// failNthAttemptRead makes the nth read of captcha_attempts after it is called fail with a
// database error, once, through a real GORM callback on the test database.
func failNthAttemptRead(t *testing.T, nth int32) {
	t.Helper()
	var seen atomic.Int32
	name := "test:fail_attempt_read_" + t.Name()
	err := db.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "captcha_attempts" && seen.Add(1) == nth {
			_ = tx.AddError(errors.New("database is down"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Query().Remove(name); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})
}

// TestCaptchaWebFailedSolveDatabaseBlipIsRetry pins WR-01 for a failed solve: the timeout path
// reports a database error as "not claimed", which looked like a lost claim. The row is still
// there, nothing was decided, so the member must get their challenge back and nobody is kicked.
func TestCaptchaWebFailedSolveDatabaseBlipIsRetry(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	env := newCaptchaWebEnv(t)
	token := env.join(member)
	env.setSiteverify(captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`))

	// The reads of captcha_attempts in one verify: the preflight Attempt (1), the Fail's own
	// read (2), then the read inside the timeout path (3), which is the one that fails.
	failNthAttemptRead(t, 3)

	status, answer := env.verify(member, token)
	if status != http.StatusServiceUnavailable || answer.Result != captchaweb.ResultRetry {
		t.Fatalf("verify = %d %+v, want 503 retry", status, answer)
	}
	if env.attempt(member.Id) == nil {
		t.Fatal("the attempt row was lost to a database blip")
	}
	if got := kicksOf(env.client, member.Id); got != 0 {
		t.Fatalf("kicks = %d, want none", got)
	}
	if !env.mr.Exists(env.challengeKey(token)) {
		t.Fatal("the challenge was not given back")
	}

	// The blip is over: the same challenge now decides the failed solve once.
	status, answer = env.verify(member, token)
	if status != http.StatusOK || answer.Result != captchaweb.ResultFailed || answer.Action != "kick" {
		t.Fatalf("second verify = %d %+v, want 200 failed with kick", status, answer)
	}
	if got := kicksOf(env.client, member.Id); got != 1 {
		t.Fatalf("kicks = %d, want exactly one", got)
	}
}

// TestCaptchaWebKeyMismatchCannotMassKick pins WR-04 end to end: with a site key and secret
// that do not belong together Cloudflare calls every solve invalid. The first
// captchaweb.StreakLimit are enforced as D-28 says; after that the members stay muted with their
// attempts intact, and once the owner fixes the keys a member passes normally.
func TestCaptchaWebKeyMismatchCannotMassKick(t *testing.T) {
	env := newCaptchaWebEnv(t)
	env.setSiteverify(captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`))

	total := captchaweb.StreakLimit + 3
	members := make([]gotgbot.User, total)
	tokens := make([]string, total)
	for i := range members {
		members[i] = gotgbot.User{Id: int64(100 + i), FirstName: "Member"}
		tokens[i] = env.join(members[i])
	}
	for i, member := range members {
		status, answer := env.verify(member, tokens[i])
		if i < captchaweb.StreakLimit {
			if status != http.StatusOK || answer.Result != captchaweb.ResultFailed {
				t.Fatalf("member %d = %d %+v, want 200 failed", i, status, answer)
			}
			if got := kicksOf(env.client, member.Id); got != 1 {
				t.Fatalf("member %d kicks = %d, want 1", i, got)
			}
			continue
		}
		if status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
			t.Fatalf("member %d = %d %+v, want 200 retry once the breaker tripped", i, status, answer)
		}
		if got := kicksOf(env.client, member.Id); got != 0 {
			t.Fatalf("member %d kicks = %d, want none", i, got)
		}
		if env.attempt(member.Id) == nil {
			t.Fatalf("member %d lost their attempt", i)
		}
		if !env.mr.Exists(env.challengeKey(tokens[i])) {
			t.Fatalf("member %d: the challenge was not given back", i)
		}
		if got := unmutesOf(t, env.client, member.Id); got != 0 {
			t.Fatalf("member %d was unmuted %d times, want still muted", i, got)
		}
	}

	// The owner fixes the keys.
	env.setSiteverify(captchaweb.SiteverifyPassForTest(captchaWebTestHost))
	last := captchaweb.StreakLimit
	if status, answer := env.verify(members[last], tokens[last]); status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
		t.Fatalf("pass after the fix = %d %+v, want 200 passed", status, answer)
	}
	if got := unmutesOf(t, env.client, members[last].Id); got != 1 {
		t.Fatalf("unmutes after the fix = %d, want 1", got)
	}
	if env.mr.Exists(captchaweb.StreakKey) {
		t.Fatal("the pass did not end the streak")
	}
}
