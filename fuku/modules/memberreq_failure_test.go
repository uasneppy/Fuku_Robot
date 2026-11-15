//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
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

// mreqShortenNoticeWait makes the wait between two sends of a rate-limited mute notice short.
func mreqShortenNoticeWait(t *testing.T) {
	t.Helper()
	previous := memberReqNoticeRateLimitWait
	memberReqNoticeRateLimitWait = 5 * time.Millisecond
	t.Cleanup(func() { memberReqNoticeRateLimitWait = previous })
}

// mreqRefusals scripts n Telegram refusals (400) of one call.
func mreqRefusals(n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = staffFakeError(400, "Bad Request: chat not found")
	}
	return errs
}

// mreqWarnLines returns the Warn lines of the requirements that the hook caught.
func mreqWarnLines(hook *logrustest.Hook) []string {
	var lines []string
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.WarnLevel && strings.Contains(entry.Message, "[MemberReq]") {
			lines = append(lines, entry.Message)
		}
	}
	return lines
}

// mreqRestrictPermissions is the permission set carried by the n-th (0-based) restrictChatMember
// call of the group.
func mreqRestrictPermissions(t *testing.T, env *memberReqEnv, n int) (gotgbot.ChatPermissions, moduleBotCall) {
	t.Helper()
	calls := env.calls("restrictChatMember")
	if len(calls) <= n {
		t.Fatalf("restrictChatMember calls = %d, want at least %d", len(calls), n+1)
	}
	perms, ok := calls[n].Params["permissions"].(gotgbot.ChatPermissions)
	if !ok {
		t.Fatalf("restrictChatMember permissions parameter is %T, want gotgbot.ChatPermissions", calls[n].Params["permissions"])
	}
	return perms, calls[n]
}

// mreqStartLockdown starts a manual lockdown of chatID that stores pre as the pre-lockdown
// permissions.
func mreqStartLockdown(t *testing.T, chatID int64, pre string) {
	t.Helper()
	lockdownCleanup(t, chatID)
	started, err := lockdown.Start(&models.ChatLockdown{
		ChatID:         chatID,
		TriggerKind:    models.LockdownTriggerManual,
		StartedBy:      1,
		PrePermissions: pre,
	})
	if err != nil || !started {
		t.Fatalf("lockdown.Start = %v, %v", started, err)
	}
}

func TestResolveUnmutePermissionsWithLockdown(t *testing.T) {
	live := gotgbot.ChatPermissions{CanSendMessages: false, CanPinMessages: true}
	lookups := 0
	previous := lockdownSnapshotLookup
	lockdownSnapshotLookup = func(chatID int64) (*models.ChatLockdown, error) {
		lookups++
		return previous(chatID)
	}
	t.Cleanup(func() { lockdownSnapshotLookup = previous })

	cases := []struct {
		name       string
		setup      func(t *testing.T, chatID int64)
		info       func(chatID int64) *gotgbot.ChatFullInfo
		wantPerms  func(t *testing.T) gotgbot.ChatPermissions
		wantActive bool
		wantErr    bool
		wantLookup bool
	}{
		{
			name:       "no lockdown returns the live defaults",
			info:       func(id int64) *gotgbot.ChatFullInfo { return &gotgbot.ChatFullInfo{Id: id, Permissions: &live} },
			wantPerms:  func(*testing.T) gotgbot.ChatPermissions { return live },
			wantLookup: true,
		},
		{
			name:       "no lockdown and no live defaults returns the fallback",
			info:       func(id int64) *gotgbot.ChatFullInfo { return &gotgbot.ChatFullInfo{Id: id} },
			wantPerms:  func(*testing.T) gotgbot.ChatPermissions { return defaultUnmutePermissions() },
			wantLookup: true,
		},
		{
			name:      "a chat with no ID is never looked up",
			info:      func(int64) *gotgbot.ChatFullInfo { return &gotgbot.ChatFullInfo{Permissions: &live} },
			wantPerms: func(*testing.T) gotgbot.ChatPermissions { return live },
		},
		{
			name:      "a missing chat returns the fallback",
			info:      func(int64) *gotgbot.ChatFullInfo { return nil },
			wantPerms: func(*testing.T) gotgbot.ChatPermissions { return defaultUnmutePermissions() },
		},
		{
			name:       "an active lockdown returns the stored permissions and says so",
			setup:      func(t *testing.T, id int64) { mreqStartLockdown(t, id, lockdownTestPrePermissions) },
			info:       func(id int64) *gotgbot.ChatFullInfo { return &gotgbot.ChatFullInfo{Id: id, Permissions: &live} },
			wantPerms:  lockdownSnapshotPermissions,
			wantActive: true,
			wantLookup: true,
		},
		{
			name: "a stored snapshot that does not decode is an error",
			setup: func(t *testing.T, id int64) {
				mreqStartLockdown(t, id, `{"can_send_messages":`)
			},
			info:       func(id int64) *gotgbot.ChatFullInfo { return &gotgbot.ChatFullInfo{Id: id, Permissions: &live} },
			wantErr:    true,
			wantLookup: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chatID := uniqueModuleChatID()
			lockdownCleanup(t, chatID)
			if tc.setup != nil {
				tc.setup(t, chatID)
			}
			lookups = 0
			info := tc.info(chatID)
			perms, active, err := resolveUnmutePermissionsWithLockdown(info)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %t", err, tc.wantErr)
			}
			if active != tc.wantActive {
				t.Errorf("lockdown active = %t, want %t", active, tc.wantActive)
			}
			if got := lookups > 0; got != tc.wantLookup {
				t.Errorf("lockdown looked up = %t, want %t", got, tc.wantLookup)
			}
			want := gotgbot.ChatPermissions{}
			if tc.wantPerms != nil {
				want = tc.wantPerms(t)
			}
			if !reflect.DeepEqual(perms, want) {
				t.Errorf("permissions = %+v, want %+v", perms, want)
			}
			// The wrapper every existing caller uses answers exactly the same.
			wrapped, wrappedErr := resolveUnmutePermissions(info)
			if !reflect.DeepEqual(wrapped, perms) || (wrappedErr != nil) != (err != nil) {
				t.Errorf("resolveUnmutePermissions = %+v, %v; want the same as %+v, %v", wrapped, wrappedErr, perms, err)
			}
		})
	}

	t.Run("a failed lookup is an error with no permissions", func(t *testing.T) {
		lockdownSnapshotLookup = func(int64) (*models.ChatLockdown, error) { return nil, errors.New("database down") }
		perms, active, err := resolveUnmutePermissionsWithLockdown(&gotgbot.ChatFullInfo{Id: uniqueModuleChatID(), Permissions: &live})
		if err == nil {
			t.Fatal("error = nil, want the lookup failure")
		}
		if active || !reflect.DeepEqual(perms, gotgbot.ChatPermissions{}) {
			t.Errorf("got %+v, active %t with an error, want zero permissions and false", perms, active)
		}
		if _, wrappedErr := resolveUnmutePermissions(&gotgbot.ChatFullInfo{Id: 1}); wrappedErr == nil {
			t.Error("resolveUnmutePermissions error = nil, want the same failure")
		}
	})
}

func TestMemberReqMuteNoticeRetry(t *testing.T) {
	mreqShortenNoticeWait(t)

	t.Run("a mute notice the pacer gave up on is sent once the block passes", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.fake.script("sendMessage", env.chat.Id, mreqRateLimited()...)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("outcome = %+v, want Muted", got)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want exactly one", len(notices))
		}
		if n := len(env.calls("sendMessage")); n != 5 {
			t.Errorf("sendMessage calls = %d, want the four the pacer gave up on and one more", n)
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrictChatMember calls = %d, want only the mute (nothing lifted)", n)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt == nil || hold.NoticeMsgID != notices[0].MessageID {
			t.Errorf("hold = %+v, want punished with the notice %d stored", hold, notices[0].MessageID)
		}
		if len(mreqKeyboardData(notices[0])) != 1 {
			t.Errorf("notice buttons = %v, want the one I fixed it button", mreqKeyboardData(notices[0]))
		}
	})

	t.Run("a notice Telegram refused once is sent on the retry", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
		env.fake.script("sendMessage", env.chat.Id, mreqRefusals(1)...)

		env.mreqEnforce(env.mreqCase(member))
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want exactly one", len(notices))
		}
		if n := len(env.calls("sendMessage")); n != 2 {
			t.Errorf("sendMessage calls = %d, want the refused one and one retry", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil || hold.NoticeMsgID != notices[0].MessageID {
			t.Errorf("hold = %+v, want punished with the notice %d stored", hold, notices[0].MessageID)
		}
		if n := len(env.calls("restrictChatMember")); n != 1 {
			t.Errorf("restrictChatMember calls = %d, want only the mute", n)
		}
	})

	t.Run("a notice without a button refused twice is logged and changes no punishment", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionBan})
		env.fake.script("sendMessage", env.chat.Id, mreqRefusals(2)...)

		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqRemoved {
			t.Fatalf("outcome = %+v, want Removed", got)
		}
		if n := len(env.calls("sendMessage")); n != 2 {
			t.Errorf("sendMessage calls = %d, want the refused one and one retry", n)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices = %d, want none delivered", n)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt == nil || hold.NoticeMsgID != 0 {
			t.Errorf("hold = %+v, want punished with no notice stored", hold)
		}
		if n := len(env.calls("unbanChatMember")); n != 0 {
			t.Errorf("unbanChatMember calls = %d, want the ban left alone", n)
		}
	})

	t.Run("a notice without a button is not retried when the pacer gives up", func(t *testing.T) {
		env := newMemberReqEnv(t)
		member := env.mreqMember("Alex", "", "alex")
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionKick})
		env.fake.script("sendMessage", env.chat.Id, mreqRateLimited()...)

		env.mreqEnforce(env.mreqCase(member))
		if n := len(env.calls("sendMessage")); n != 4 {
			t.Errorf("sendMessage calls = %d, want the four the pacer made and no more", n)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold = %+v, want the kick to stand", hold)
		}
	})

	t.Run("a mute notice stops waiting when its context ends", func(t *testing.T) {
		env := newMemberReqEnv(t)
		blocked := make([]error, 0, 400)
		for i := 0; i < 100; i++ {
			blocked = append(blocked, mreqRateLimited()...)
		}
		env.fake.script("sendMessage", env.chat.Id, blocked...)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		started := time.Now()
		_, err := memberReqPostNotice(ctx, env.bot, env.chat.Id, "text", memberReqFixedButton(memberReqTranslator(ctx, env.chat.Id), 1, 2))
		if err == nil {
			t.Fatal("error = nil, want the failure when the context ends")
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Errorf("waited %s, want to stop soon after the context ended", elapsed)
		}
		if n := len(env.calls("sendMessage")); n < 8 {
			t.Errorf("sendMessage calls = %d, want the notice retried while the pacer kept giving up", n)
		}
	})
}

// mreqUndeliverable is a group whose mute notice Telegram refuses twice: a member who breaks
// the last-name rule with a mute, and the group's live default permissions.
func mreqUndeliverable(t *testing.T) (*memberReqEnv, gotgbot.User, *logrustest.Hook) {
	t.Helper()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	env := newMemberReqEnv(t)
	member := env.mreqMember("Alex", "", "alex")
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
	env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":true,"can_pin_messages":true}`)
	env.fake.script("sendMessage", env.chat.Id, mreqRefusals(2)...)
	return env, member, hook
}

func TestMemberReqMuteNoticeUndeliverable(t *testing.T) {
	livePermissions := gotgbot.ChatPermissions{CanSendMessages: true, CanPinMessages: true}

	t.Run("the mute is lifted and the hold is left as a backoff row", func(t *testing.T) {
		env, member, hook := mreqUndeliverable(t)

		env.mreqEnforce(env.mreqCase(member))

		if n := len(env.calls("sendMessage")); n != 2 {
			t.Errorf("sendMessage calls = %d, want the refused notice and its one retry", n)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("messages posted = %d, want nothing posted", n)
		}
		if n := len(env.calls("restrictChatMember")); n != 2 {
			t.Fatalf("restrictChatMember calls = %d, want the mute and its lift", n)
		}
		_, mute := mreqRestrictPermissions(t, env, 0)
		if staffParamInt(mute.Params, "until_date") == 0 {
			t.Error("the mute carried no marker end date")
		}
		lifted, lift := mreqRestrictPermissions(t, env, 1)
		if !reflect.DeepEqual(lifted, livePermissions) {
			t.Errorf("lift permissions = %+v, want the group's live defaults %+v", lifted, livePermissions)
		}
		if until := staffParamInt(lift.Params, "until_date"); until != 0 {
			t.Errorf("lift until_date = %d, want none", until)
		}
		rec := env.fake.member(env.chat.Id, member.Id)
		if rec == nil || rec.Status != gotgbot.ChatMemberStatusRestricted || !rec.CanSendMessages || rec.UntilDate != 0 {
			t.Errorf("member after the lift = %+v, want restricted to the live defaults with no end date", rec)
		}

		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold = %+v, want a pending backoff row", hold)
		}
		if age := time.Since(hold.CreatedAt); age > 5*time.Second {
			t.Errorf("hold created_at is %s old, want refreshed when the mute was lifted", age)
		}

		lines := mreqWarnLines(hook)
		if len(lines) != 1 {
			t.Fatalf("Warn lines = %q, want exactly one", lines)
		}
		for _, want := range []string{fmt.Sprint(env.chat.Id), fmt.Sprint(member.Id)} {
			if !strings.Contains(lines[0], want) {
				t.Errorf("Warn line %q does not hold %s", lines[0], want)
			}
		}
		for _, secret := range []string{"Alex", "alex"} {
			if strings.Contains(lines[0], secret) {
				t.Errorf("Warn line %q holds the member's name", lines[0])
			}
		}
	})

	t.Run("the member's next message inside the backoff adds nothing", func(t *testing.T) {
		env, member, _ := mreqUndeliverable(t)
		env.mreqEnforce(env.mreqCase(member))

		before := env.mreqCallCount("getChatMember", "sendMessage", "restrictChatMember", "banChatMember", "unbanChatMember", "getChat")
		if again := env.mreqEnforce(env.mreqCase(member)); again.Outcome != memberReqHeld {
			t.Fatalf("next case inside the backoff = %+v, want Held", again)
		}
		if after := env.mreqCallCount("getChatMember", "sendMessage", "restrictChatMember", "banChatMember", "unbanChatMember", "getChat"); after != before {
			t.Errorf("a held member made %d Telegram calls, want none", after-before)
		}
	})

	t.Run("the first message after StaleClaim mutes the member again with a delivered notice", func(t *testing.T) {
		mreqShortenStale(t, 600*time.Millisecond)
		env, member, _ := mreqUndeliverable(t)
		env.mreqEnforce(env.mreqCase(member))

		time.Sleep(700 * time.Millisecond)
		got := env.mreqEnforce(env.mreqCase(member))
		if got.Outcome != memberReqMuted {
			t.Fatalf("case after the backoff = %+v, want Muted again", got)
		}
		if n := len(env.calls("restrictChatMember")); n != 3 {
			t.Errorf("restrictChatMember calls = %d, want the mute, its lift and the new mute", n)
		}
		notices := env.mreqNotices()
		if len(notices) != 1 {
			t.Fatalf("notices = %d, want the one delivered notice", len(notices))
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt == nil || hold.NoticeMsgID != notices[0].MessageID {
			t.Fatalf("hold = %+v, want punished with the notice %d stored", hold, notices[0].MessageID)
		}
		data := mreqKeyboardData(notices[0])
		if len(data) != 1 {
			t.Fatalf("notice buttons = %v, want the one I fixed it button", data)
		}
		decoded, ok := decodeCallbackData(data[0], memberReqCallbackNamespace)
		if !ok || decoded.Fields["h"] != fmt.Sprint(hold.ID) || decoded.Fields["u"] != fmt.Sprint(member.Id) {
			t.Errorf("button data = %+v, want the hold %d and the member %d", decoded.Fields, hold.ID, member.Id)
		}
	})

	t.Run("during a lockdown the lift uses the stored pre-lockdown permissions", func(t *testing.T) {
		env, member, _ := mreqUndeliverable(t)
		mreqStartLockdown(t, env.chat.Id, lockdownTestPrePermissions)
		// The group's live defaults are the locked set while the lockdown lasts.
		env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":false}`)

		env.mreqEnforce(env.mreqCase(member))

		if n := len(env.calls("restrictChatMember")); n != 2 {
			t.Fatalf("restrictChatMember calls = %d, want the mute and its lift", n)
		}
		lifted, lift := mreqRestrictPermissions(t, env, 1)
		wantSnapshotPermissions(t, lifted)
		if until := staffParamInt(lift.Params, "until_date"); until != 0 {
			t.Errorf("lift until_date = %d, want none", until)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt != nil {
			t.Errorf("hold = %+v, want a pending backoff row", hold)
		}
	})

	refusedLifts := []struct {
		name   string
		method string
		errs   []error
		calls  int
	}{
		{"the lift is refused", "restrictChatMember", append([]error{nil}, mreqRefusals(1)...), 2},
		{"the group cannot be read", "getChat", mreqRefusals(1), 1},
	}
	for _, refused := range refusedLifts {
		t.Run("the mute stays, with the hold punished, when "+refused.name, func(t *testing.T) {
			env, member, hook := mreqUndeliverable(t)
			env.fake.script(refused.method, env.chat.Id, refused.errs...)

			env.mreqEnforce(env.mreqCase(member))

			if n := len(env.calls("restrictChatMember")); n != refused.calls {
				t.Errorf("restrictChatMember calls = %d, want %d", n, refused.calls)
			}
			hold := env.mreqHold(member.Id)
			if hold == nil || hold.PunishedAt == nil {
				t.Fatalf("hold = %+v, want it to stay punished", hold)
			}
			rec := env.fake.member(env.chat.Id, member.Id)
			if rec == nil || rec.Status != gotgbot.ChatMemberStatusRestricted || rec.UntilDate != hold.MuteUntil {
				t.Errorf("member = %+v, want the mute with marker %d still in place", rec, hold.MuteUntil)
			}
			lines := mreqWarnLines(hook)
			if len(lines) != 1 {
				t.Fatalf("Warn lines = %q, want exactly one", lines)
			}
			if !strings.Contains(lines[0], "could not be lifted") {
				t.Errorf("Warn line %q does not say the mute could not be lifted", lines[0])
			}
			for _, want := range []string{fmt.Sprint(env.chat.Id), fmt.Sprint(member.Id)} {
				if !strings.Contains(lines[0], want) {
					t.Errorf("Warn line %q does not hold %s", lines[0], want)
				}
			}
			for _, secret := range []string{"Alex", "alex"} {
				if strings.Contains(lines[0], secret) {
					t.Errorf("Warn line %q holds the member's name", lines[0])
				}
			}
		})
	}
}

// mreqDeliveredMute is a group where a member was muted for the last-name rule and the notice
// was delivered, so the hold is punished with its marker. The test then calls
// memberReqUndeliveredMute itself, as the executor does after a notice that was rate limited
// for minutes: whatever the member's record became in that window is set by the test first.
func mreqDeliveredMute(t *testing.T) (*memberReqEnv, memberReqCase, *models.ChatRequirementHold, *logrustest.Hook) {
	t.Helper()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	env := newMemberReqEnv(t)
	member := env.mreqMember("Alex", "", "alex")
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionMute})
	env.fake.setChatPermsRaw(env.chat.Id, `{"can_send_messages":true,"can_pin_messages":true}`)
	c := env.mreqCase(member)
	if got := env.mreqEnforce(c); got.Outcome != memberReqMuted {
		t.Fatalf("outcome = %+v, want Muted", got)
	}
	hold := env.mreqHold(member.Id)
	if hold == nil || hold.PunishedAt == nil || hold.MuteUntil == 0 {
		t.Fatalf("hold = %+v, want punished with a marker", hold)
	}
	if n := len(env.calls("restrictChatMember")); n != 1 {
		t.Fatalf("restrictChatMember calls = %d, want the mute alone", n)
	}
	return env, c, hold, hook
}

func TestMemberReqUndeliveredMuteChecksMarker(t *testing.T) {
	t.Run("the marker still live: the mute is lifted after one paced live read", func(t *testing.T) {
		env, c, hold, _ := mreqDeliveredMute(t)
		lookups := env.mreqCallCount("getChatMember")

		memberReqUndeliveredMute(context.Background(), env.bot, c, hold)

		if n := env.mreqCallCount("getChatMember") - lookups; n != 1 {
			t.Errorf("getChatMember calls = %d, want one live read before the lift", n)
		}
		if n := len(env.calls("restrictChatMember")); n != 2 {
			t.Fatalf("restrictChatMember calls = %d, want the mute and its lift", n)
		}
		if rec := env.fake.member(env.chat.Id, c.User.Id); rec == nil || rec.UntilDate != 0 || !rec.CanSendMessages {
			t.Errorf("member after the lift = %+v, want the group's defaults with no end date", rec)
		}
		if got := env.mreqHold(c.User.Id); got == nil || got.PunishedAt != nil {
			t.Errorf("hold = %+v, want a pending backoff row", got)
		}
	})

	replaced := []struct {
		name   string
		record staffFakeMember
	}{
		{"an admin restricted the member with no end date", staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true}},
		{"an admin restricted the member with an end date of their own", staffFakeMember{
			Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: time.Now().Add(48 * time.Hour).Unix(),
		}},
		{"the restriction was lifted and the member is a plain member", staffFakeMember{
			Status: gotgbot.ChatMemberStatusMember, IsMember: true, CanSendMessages: true,
		}},
		{"the member left", staffFakeMember{Status: gotgbot.ChatMemberStatusLeft}},
		{"the member was banned", staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: 1}},
	}
	for _, tc := range replaced {
		t.Run("the record is left alone when "+tc.name, func(t *testing.T) {
			env, c, hold, hook := mreqDeliveredMute(t)
			env.fake.setMember(env.chat.Id, c.User.Id, tc.record)
			hook.Reset()

			memberReqUndeliveredMute(context.Background(), env.bot, c, hold)

			if n := len(env.calls("restrictChatMember")); n != 1 {
				t.Errorf("restrictChatMember calls = %d, want only the mute and no lift", n)
			}
			rec := env.fake.member(env.chat.Id, c.User.Id)
			if rec == nil || *rec != tc.record {
				t.Errorf("member record = %+v, want %+v untouched", rec, tc.record)
			}
			got := env.mreqHold(c.User.Id)
			if got == nil || got.PunishedAt != nil {
				t.Errorf("hold = %+v, want a pending backoff row", got)
			}
			lines := mreqWarnLines(hook)
			if len(lines) != 1 {
				t.Fatalf("Warn lines = %q, want exactly one", lines)
			}
			for _, want := range []string{fmt.Sprint(env.chat.Id), fmt.Sprint(c.User.Id)} {
				if !strings.Contains(lines[0], want) {
					t.Errorf("Warn line %q does not hold %s", lines[0], want)
				}
			}
			for _, secret := range []string{"Alex", "alex"} {
				if strings.Contains(lines[0], secret) {
					t.Errorf("Warn line %q holds the member's name", lines[0])
				}
			}
		})
	}

	t.Run("a live read that fails still lifts the mute", func(t *testing.T) {
		env, c, hold, _ := mreqDeliveredMute(t)
		env.fake.script("getChatMember", env.chat.Id, mreqRefusals(1)...)

		memberReqUndeliveredMute(context.Background(), env.bot, c, hold)

		if n := len(env.calls("restrictChatMember")); n != 2 {
			t.Fatalf("restrictChatMember calls = %d, want the mute and its lift", n)
		}
		if rec := env.fake.member(env.chat.Id, c.User.Id); rec == nil || rec.UntilDate != 0 {
			t.Errorf("member after the lift = %+v, want no end date", rec)
		}
		if got := env.mreqHold(c.User.Id); got == nil || got.PunishedAt != nil {
			t.Errorf("hold = %+v, want a pending backoff row", got)
		}
	})
}
