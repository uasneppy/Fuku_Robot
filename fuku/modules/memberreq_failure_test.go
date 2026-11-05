//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqShortenStale makes a pending hold reclaimable after d for the test.
func mreqShortenStale(t *testing.T, d time.Duration) {
	t.Helper()
	previous := memberreq.StaleClaim
	memberreq.StaleClaim = d
	t.Cleanup(func() { memberreq.StaleClaim = previous })
}

// mreqLostAnswerClient is the Bot API fake with one fault the fake's scripted errors cannot
// make: Telegram applies the named method and the answer is lost on the way back (a 502 after
// the effect). Every other request goes through unchanged.
type mreqLostAnswerClient struct {
	*lockdownFake

	mu     sync.Mutex
	method string
	lose   int
}

// loseAnswers makes the next n answers of method an error after the fake applied the request.
func (e *memberReqEnv) loseAnswers(method string, n int) {
	e.t.Helper()
	e.bot.BotClient = &mreqLostAnswerClient{lockdownFake: e.fake, method: method, lose: n}
}

func (c *mreqLostAnswerClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	raw, err := c.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
	if err != nil || method != c.method {
		return raw, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lose == 0 {
		return raw, nil
	}
	c.lose--
	return nil, staffFakeError(502, "Bad Gateway")
}

// mreqRateLimited scripts the four 429 answers after which the pacer gives up with
// ratelimit.ErrRateLimited.
func mreqRateLimited() []error {
	return []error{staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1)}
}

// mreqPunishmentCase names one punishment, the Telegram method it makes and what a taken
// punishment returns.
type mreqPunishmentCase struct {
	name    string
	action  int
	method  string
	outcome memberReqOutcome
}

var mreqPunishmentCases = []mreqPunishmentCase{
	{"mute", models.RequirementActionMute, "restrictChatMember", memberReqMuted},
	{"ban", models.RequirementActionBan, "banChatMember", memberReqRemoved},
	{"kick", models.RequirementActionKick, "unbanChatMember", memberReqRemoved},
}

func TestMemberReqPunishRefused(t *testing.T) {
	refusals := []struct {
		name string
		errs []error
	}{
		{"400", []error{staffFakeError(400, "Bad Request: not enough rights")}},
		{"403", []error{staffFakeError(403, "Forbidden: bot was kicked")}},
		{"the pacer gives up on 429", mreqRateLimited()},
	}
	for _, punishment := range mreqPunishmentCases {
		for _, refusal := range refusals {
			t.Run(punishment.name+" refused by "+refusal.name, func(t *testing.T) {
				mreqShortenStale(t, 300*time.Millisecond)
				env := newMemberReqEnv(t)
				member := env.mreqMember("Alex", "", "alex")
				env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: punishment.action})
				// The refused call takes longer than StaleClaim, so only a backoff that
				// refreshes created_at keeps the hold from being reclaimed at once.
				env.fake.setDelay(punishment.method, env.chat.Id, 350*time.Millisecond)
				env.fake.script(punishment.method, env.chat.Id, refusal.errs...)

				got := env.mreqEnforce(env.mreqCase(member))
				if got.Outcome != memberReqNotApplied {
					t.Fatalf("outcome = %+v, want NotApplied", got)
				}
				if n := len(env.mreqNotices()); n != 0 {
					t.Errorf("messages after a refused punishment = %d, want none", n)
				}
				hold := env.mreqHold(member.Id)
				if hold == nil || hold.PunishedAt != nil {
					t.Fatalf("hold = %+v, want a pending hold", hold)
				}
				if age := time.Since(hold.CreatedAt); age > 150*time.Millisecond {
					t.Errorf("hold created_at is %s old, want a backoff refreshed when the call was refused", age)
				}

				before := env.mreqCallCount("getChatMember", "sendMessage", punishment.method)
				if again := env.mreqEnforce(env.mreqCase(member)); again.Outcome != memberReqHeld {
					t.Fatalf("next case inside the backoff = %+v, want Held", again)
				}
				if after := env.mreqCallCount("getChatMember", "sendMessage", punishment.method); after != before {
					t.Errorf("a held member made %d Telegram calls, want none", after-before)
				}

				env.fake.setDelay(punishment.method, env.chat.Id, 0)
				time.Sleep(350 * time.Millisecond)
				retried := env.mreqEnforce(env.mreqCase(member))
				if retried.Outcome != punishment.outcome {
					t.Fatalf("case after the backoff = %+v, want the punishment retried (%d)", retried, punishment.outcome)
				}
				if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil {
					t.Errorf("hold after the retry = %+v, want punished", hold)
				}
				if n := len(env.mreqNotices()); n != 1 {
					t.Errorf("messages after the retry = %d, want exactly one notice", n)
				}
			})
		}
	}

	t.Run("a warn that cannot be recorded backs off", func(t *testing.T) {
		mreqShortenStale(t, 300*time.Millisecond)
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})

		calls := 0
		previous := memberReqApplyWarn
		t.Cleanup(func() { memberReqApplyWarn = previous })
		memberReqApplyWarn = func(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64, reason string, afterRecord func()) (warnApplyResult, error) {
			calls++
			if calls > 1 {
				return applyWarn(ctx, b, chat, userID, reason, afterRecord)
			}
			time.Sleep(350 * time.Millisecond)
			restore := captchaApplyBrokenDB(t)
			defer restore()
			return applyWarn(ctx, b, chat, userID, reason, afterRecord)
		}

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNotApplied {
			t.Fatalf("outcome = %+v, want NotApplied", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold = %+v, want a pending hold", hold)
		}
		if age := time.Since(hold.CreatedAt); age > 150*time.Millisecond {
			t.Errorf("hold created_at is %s old, want a backoff refreshed after the failed warn", age)
		}
		if again := env.mreqEnforce(env.mreqCase(member)); again.Outcome != memberReqHeld {
			t.Fatalf("next case inside the backoff = %+v, want Held", again)
		}
		if calls != 1 {
			t.Errorf("warn attempts inside the backoff = %d, want 1", calls)
		}
		time.Sleep(350 * time.Millisecond)
		if retried := env.mreqEnforce(env.mreqCase(member)); retried.Outcome != memberReqWarned {
			t.Fatalf("case after the backoff = %+v, want Warned", retried)
		}
	})
}

func TestMemberReqPunishUnknown(t *testing.T) {
	t.Run("a mute whose answer was lost but whose marker is live counts as applied", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.loseAnswers("restrictChatMember", 1)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrict calls = %d, want 1", n)
		}
		if n := len(env.calls("getChatMember")); n != 2 {
			t.Errorf("getChatMember calls = %d, want the first live read and exactly one verification", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold = %+v, want punished", hold)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want exactly one", n)
		}
	})

	t.Run("a mute answered 502 with no marker live backs off and posts nothing", func(t *testing.T) {
		mreqShortenStale(t, 300*time.Millisecond)
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.fake.setDelay("restrictChatMember", env.chat.Id, 350*time.Millisecond)
		env.fake.script("restrictChatMember", env.chat.Id, staffFakeError(502, "Bad Gateway"))

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqNotApplied {
			t.Fatalf("outcome = %+v, want NotApplied", got)
		}
		if n := len(env.calls("getChatMember")); n != 2 {
			t.Errorf("getChatMember calls = %d, want the first live read and exactly one verification", n)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices = %d, want none", n)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold = %+v, want a pending hold", hold)
		}
		if age := time.Since(hold.CreatedAt); age > 150*time.Millisecond {
			t.Errorf("hold created_at is %s old, want a backoff", age)
		}
		if again := env.mreqEnforce(env.mreqCase(member)); again.Outcome != memberReqHeld {
			t.Errorf("next case inside the backoff = %+v, want Held", again)
		}
	})

	t.Run("a mute whose verification also fails stays pending and the reclaim mutes once", func(t *testing.T) {
		mreqShortenStale(t, 50*time.Millisecond)
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.fake.script("restrictChatMember", env.chat.Id, staffFakeError(502, "Bad Gateway"))
		env.fake.script("getChatMember", env.chat.Id, nil, staffFakeError(502, "Bad Gateway"))

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqNotApplied {
			t.Fatalf("outcome = %+v, want NotApplied", got)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold = %+v, want a pending hold", hold)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices = %d, want none", n)
		}

		time.Sleep(80 * time.Millisecond)
		retried := env.mreqEnforce(env.mreqCase(member))
		if retried.Outcome != memberReqMuted {
			t.Fatalf("reclaim = %+v, want Muted", retried)
		}
		if n := len(env.calls("restrictChatMember")); n != 2 {
			t.Errorf("restrict calls = %d, want the refused one and one retry", n)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want one", n)
		}
	})

	t.Run("a mute applied with its answer lost is never applied twice by the reclaim", func(t *testing.T) {
		mreqShortenStale(t, 50*time.Millisecond)
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.loseAnswers("restrictChatMember", 1)
		env.fake.script("getChatMember", env.chat.Id, nil, staffFakeError(502, "Bad Gateway"))

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNotApplied {
			t.Fatalf("outcome = %+v, want NotApplied (the verification failed)", got)
		}
		first := env.mreqHold(member.Id)
		if first == nil || first.PunishedAt != nil {
			t.Fatalf("hold = %+v, want a pending hold", first)
		}

		time.Sleep(80 * time.Millisecond)
		retried := env.mreqEnforce(env.mreqCase(member))
		if retried.Outcome != memberReqMuted {
			t.Fatalf("reclaim = %+v, want Muted", retried)
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrict calls = %d, want only the first (the live marker was found)", n)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt == nil {
			t.Fatalf("hold after the reclaim = %+v, want punished", hold)
		}
		if hold.MuteUntil != first.MuteUntil {
			t.Errorf("mute_until = %d, want the marker %d that is live", hold.MuteUntil, first.MuteUntil)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want one", n)
		}
	})

	for _, punishment := range mreqPunishmentCases[1:] {
		t.Run(punishment.name+" answered 502 stays pending with no notice", func(t *testing.T) {
			env := newMemberReqEnv(t)
			member := env.mreqMember("Alex", "", "alex")
			env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: punishment.action})
			env.fake.script(punishment.method, env.chat.Id, staffFakeError(502, "Bad Gateway"))

			got := env.mreqEnforce(env.mreqCase(member))
			if got.Outcome != memberReqNotApplied {
				t.Fatalf("outcome = %+v, want NotApplied", got)
			}
			if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt != nil {
				t.Errorf("hold = %+v, want a pending hold", hold)
			}
			if n := len(env.mreqNotices()); n != 0 {
				t.Errorf("notices = %d, want none", n)
			}
			if n := len(env.calls("getChatMember")); n != 1 {
				t.Errorf("getChatMember calls = %d, want only the first live read (no verification for %s)", n, punishment.name)
			}
		})
	}
}
