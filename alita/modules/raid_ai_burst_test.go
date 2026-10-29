//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/aispam"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// aiSpamRequests counts the requests the TypeSafe fake of the AI burst tests received.
var aiSpamRequests atomic.Int64

// newAIBurstEnv is a flood env (Staff Group, clean raid settings, env.admin the chat's
// creator) with /aispam enabled for its chat and answering from a TypeSafe fake: a text
// that holds SPAM is a delete verdict at 0.95, any other text a keep verdict at 0.1.
func newAIBurstEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	env, _ := floodBurstEnv(t)
	env.enableAI()
	return env
}

// enableAI turns /aispam on for the env's chat, answering from the TypeSafe fake of
// newAIBurstEnv, and loads the AI spam module on the env's dispatcher.
func (e *lockdownEnv) enableAI() {
	t, env := e.t, e
	t.Helper()
	resetAISpamRuntime()
	withAISpamTestConfig(t, "typesafe-test-key", true, 0)
	aiSpamRequests.Store(0)
	withAISpamJevServer(t, func(w http.ResponseWriter, r *http.Request) {
		aiSpamRequests.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the TypeSafe request: %v", err)
		}
		var body struct {
			State struct {
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"state"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode the TypeSafe request: %v", err)
		}
		if strings.Contains(body.State.Message.Text, "SPAM") {
			_, _ = w.Write([]byte(aispamDecisionBody(0.95, "scam")))
			return
		}
		_, _ = w.Write([]byte(aispamDecisionBody(0.1, "none")))
	})
	if err := aispam.SetAISpamEnabled(env.chat.Id, true); err != nil {
		t.Fatalf("enable /aispam: %v", err)
	}
	t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.AISpamSettings{}) })
	LoadAISpam(env.dispatcher)
}

// staffChatOf is the Staff Group the env's chat is linked to.
func (e *lockdownEnv) staffChatOf() gotgbot.Chat {
	e.t.Helper()
	link, err := staff.GetLinkOfGroupFresh(e.chat.Id)
	if err != nil || link == nil {
		e.t.Fatalf("the env's chat has no Staff Group link (error %v)", err)
	}
	return gotgbot.Chat{Id: link.StaffChatID, Type: "supergroup", Title: "Staff HQ"}
}

// aiRequests is how many texts reached the TypeSafe fake.
func (e *lockdownEnv) aiRequests() int {
	return int(aiSpamRequests.Load())
}

// spamFrom has user send n texts in the env's chat that the TypeSafe fake judges spam.
func (e *lockdownEnv) spamFrom(user gotgbot.User, n int) {
	e.t.Helper()
	e.textsFrom(e.chat, user, n, "SPAM offer t.me/scam")
}

// textsFrom has user send n texts that start with prefix in chat.
func (e *lockdownEnv) textsFrom(chat gotgbot.Chat, user gotgbot.User, n int, prefix string) {
	e.t.Helper()
	for i := 0; i < n; i++ {
		e.dispatchMessage(chat, gotgbot.Message{From: &user, Text: fmt.Sprintf("%s %d", prefix, e.updateID())})
	}
}

// settleAI waits until every /aispam check the env's messages queued has finished,
// together with the public notice or warning an automatic lockdown posts.
func (e *lockdownEnv) settleAI() {
	e.t.Helper()
	DrainAISpamChecks()
	raidPosts.wait()
}

// aiBurstLockdown has three new accounts send 2, 2 and 1 spam texts, waits for the
// verdicts and returns the active AI burst lockdown, failing when there is none.
func (e *lockdownEnv) aiBurstLockdown() *models.ChatLockdown {
	e.t.Helper()
	accounts := e.newMembers(3, 0)
	e.spamFrom(accounts[0], 2)
	e.spamFrom(accounts[1], 2)
	e.spamFrom(accounts[2], 1)
	e.settleAI()
	row := e.activeLockdown()
	if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst || row.LockedAt == nil {
		e.t.Fatalf("setup: active lockdown = %+v, want a confirmed AI burst lockdown", row)
	}
	return row
}

// aiVerdictSetSize is the size of one of the chat's AI verdict sets (av or aa).
func aiVerdictSetSize(e *lockdownEnv, part string) int {
	e.t.Helper()
	members, err := e.redis.ZMembers(raidKey(e.chat.Id, part))
	if err != nil {
		return 0
	}
	return len(members)
}

func TestRaidAIBurstFires(t *testing.T) {
	t.Run("five verdicts from three accounts lock", func(t *testing.T) {
		env := newAIBurstEnv(t)

		row := env.aiBurstLockdown()

		if row.TriggerCount != 5 || row.TriggerWindow != 120 || row.TriggerPercent != 0 {
			t.Errorf("trigger numbers = %d verdicts in %d s, %d%%, want 5 in 120 s, 0%%",
				row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
		}
		if row.StartedBy != staffTestBotID {
			t.Errorf("StartedBy = %d, want the bot %d", row.StartedBy, staffTestBotID)
		}
		if row.Reason != "" {
			t.Errorf("Reason = %q, want it empty: the numbers are columns", row.Reason)
		}
		if got := len(env.calls("setChatPermissions")); got != 1 {
			t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
		}
		if got := len(env.calls("deleteMessage")); got != 5 {
			t.Errorf("deleteMessage calls = %d, want the 5 /aispam deletions", got)
		}
		if got := env.aiRequests(); got != 5 {
			t.Errorf("TypeSafe requests = %d, want exactly one per message (5)", got)
		}
		if got := len(env.calls("banChatMember")) + len(env.calls("restrictChatMember")); got != 0 {
			t.Errorf("%d ban or restrict calls, want none: only the lockdown acts", got)
		}
	})

	t.Run("four verdicts do not lock", func(t *testing.T) {
		env := newAIBurstEnv(t)
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 1)
		env.spamFrom(accounts[2], 1)
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 4 verdicts, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 4 {
			t.Errorf("verdict items counted = %d, want 4", got)
		}
	})

	t.Run("two accounts do not lock", func(t *testing.T) {
		env := newAIBurstEnv(t)
		accounts := env.newMembers(2, 0)

		env.spamFrom(accounts[0], 3)
		env.spamFrom(accounts[1], 2)
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists for 5 verdicts from 2 accounts, want none", row.TriggerKind)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
		if got := aiVerdictSetSize(env, "aa"); got != 2 {
			t.Errorf("accounts counted = %d, want 2", got)
		}
	})

	t.Run("keep verdicts count for nothing", func(t *testing.T) {
		env := newAIBurstEnv(t)
		accounts := env.newMembers(3, 0)

		for _, account := range accounts {
			env.textsFrom(env.chat, account, 2, "hello there")
		}
		env.settleAI()

		if got := env.aiRequests(); got != 6 {
			t.Fatalf("TypeSafe requests = %d, want 6: the case would prove nothing", got)
		}
		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists after keep verdicts, want none", row.TriggerKind)
		}
		if got := aiVerdictSetSize(env, "av"); got != 0 {
			t.Errorf("verdict items counted = %d, want 0", got)
		}
		if got := len(env.calls("deleteMessage")); got != 0 {
			t.Errorf("deleteMessage calls = %d, want 0", got)
		}
	})

	t.Run("a basic group never counts", func(t *testing.T) {
		env := newAIBurstEnv(t)
		basic := env.chat
		basic.Type = "group"
		accounts := env.newMembers(3, 0)

		env.textsFrom(basic, accounts[0], 2, "SPAM offer t.me/scam")
		env.textsFrom(basic, accounts[1], 2, "SPAM offer t.me/scam")
		env.textsFrom(basic, accounts[2], 1, "SPAM offer t.me/scam")
		env.settleAI()

		if got := env.aiRequests(); got != 5 {
			t.Fatalf("TypeSafe requests = %d, want 5: /aispam still judges a basic group", got)
		}
		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists in a basic group, want none", row.TriggerKind)
		}
		if env.redis.Exists(raidKey(env.chat.Id, "av")) {
			t.Error("the verdict set exists for a basic group, want no key")
		}
	})
}

func TestRaidAIAlert(t *testing.T) {
	t.Run("the alert names the AI trigger", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		env.aiBurstLockdown()

		env.cycle()

		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_trigger_ai_burst") + " 5 120"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_trigger_manual")) {
			t.Errorf("alert %q names a manual lockdown", text)
		}
	})

	t.Run("status and already active", func(t *testing.T) {
		env := newAIBurstEnv(t)
		env.aiBurstLockdown()
		locksBefore := len(env.calls("setChatPermissions"))

		env.askStatus(env.admin)

		env.wantReplyHas(
			staffMarker("lockdown_status_active_auto"),
			staffMarker("lockdown_trigger_ai_burst")+" 5 120",
		)
		env.wantReplyLacks(staffMarker("lockdown_status_active"), staffMarker("lockdown_reason_line"))

		env.send(env.admin, "/lockdown")

		env.wantReplyHas(
			staffMarker("lockdown_already_active_auto"),
			staffMarker("lockdown_trigger_ai_burst")+" 5 120",
		)
		if got := len(env.calls("setChatPermissions")); got != locksBefore {
			t.Errorf("setChatPermissions calls = %d after /lockdown, want still %d", got, locksBefore)
		}
	})

	t.Run("reminder", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		row := env.aiBurstLockdown()

		env.cycle()
		if got := len(env.fake.sentTo(staffChat.Id)); got != 1 {
			t.Fatalf("%d messages reached the Staff Group after the alert cycle, want 1", got)
		}
		ageAlert(t, row.ID, 61*time.Minute)
		env.cycle()

		sent := env.fake.sentTo(staffChat.Id)
		if len(sent) != 2 {
			t.Fatalf("%d messages reached the Staff Group, want the alert and one reminder", len(sent))
		}
		text := fmt.Sprint(sent[1].Params["text"])
		for _, want := range []string{
			staffMarker("lockdown_reminder_title"),
			staffMarker("lockdown_trigger_ai_burst") + " 5 120",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("reminder %q does not contain %q", text, want)
			}
		}
		if strings.Contains(text, staffMarker("lockdown_reason_line")) {
			t.Errorf("reminder %q has a reason line", text)
		}
	})

	t.Run("notice", func(t *testing.T) {
		env := newAIBurstEnv(t)
		env.aiBurstLockdown()

		text := env.wantOneNoticeIn()

		if want := staffMarker("lockdown_auto_started_ai_burst") + " 5 120"; !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
		if !strings.Contains(text, staffMarker("lockdown_note_approved_muted")) {
			t.Errorf("notice %q does not say approved users are muted too", text)
		}
	})

	t.Run("cannot lock", func(t *testing.T) {
		env, staffChat, channel := warnEnv(t)
		env.enableAI()
		env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict)
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 2)
		env.spamFrom(accounts[2], 1)
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists although the bot cannot restrict members: %+v", row)
		}
		if got := len(env.calls("setChatPermissions")); got != 0 {
			t.Errorf("setChatPermissions calls = %d, want 0", got)
		}
		env.wantWarnings(staffChat, channel, 1)
		texts := env.textsTo(staffChat)
		for _, want := range []string{
			staffMarker("lockdown_auto_cant_lock_rights"),
			staffMarker("lockdown_trigger_ai_burst") + " 5 120",
		} {
			if len(texts) == 1 && !strings.Contains(texts[0], want) {
				t.Errorf("Staff Group warning %q does not contain %q", texts[0], want)
			}
		}
	})
}

// TestLockdownTriggerKindsAllSurfaces renders every trigger kind on every surface that
// switches on it. A kind added to models.LockdownTriggerKinds without a case here fails
// the table, and an automatic kind that any surface renders as the manual wording, or as
// nothing, fails its own case.
func TestLockdownTriggerKindsAllSurfaces(t *testing.T) {
	type surfaces struct {
		// automatic is whether the bot started the lockdown on its own.
		automatic bool
		// counts is the locale key of the alert's counts line.
		counts string
	}
	cases := map[string]surfaces{
		models.LockdownTriggerManual:    {automatic: false, counts: "lockdown_alert_counts"},
		models.LockdownTriggerJoinSurge: {automatic: true, counts: "lockdown_alert_counts"},
		models.LockdownTriggerFlood:     {automatic: true, counts: "lockdown_alert_counts_flood"},
		models.LockdownTriggerAIBurst:   {automatic: true, counts: "lockdown_alert_counts_ai"},
	}
	for _, kind := range models.LockdownTriggerKinds() {
		if _, ok := cases[kind]; !ok {
			t.Errorf("trigger kind %q has no case in the surface table", kind)
		}
	}

	env := newLockdownEnv(t)
	tr := staffChatTranslator(env.chat.Id)
	for kind, want := range cases {
		t.Run(kind, func(t *testing.T) {
			ld := &models.ChatLockdown{
				ChatID: env.chat.Id, TriggerKind: kind, TriggerCount: 7, TriggerWindow: 120, TriggerPercent: 40,
				StartedBy: env.admin.Id, StartedByName: "Starter",
			}

			sentence := lockdownTriggerSentence(tr, kind, 7, 120, 40)
			line := lockdownAlertTriggerLine(tr, ld, time.Now())
			if want.automatic {
				if !strings.Contains(sentence, staffMarker("lockdown_trigger_"+kind)) {
					t.Errorf("trigger sentence %q does not carry %q", sentence, staffMarker("lockdown_trigger_"+kind))
				}
				if !strings.Contains(line, staffMarker("lockdown_alert_trigger_"+kind)) {
					t.Errorf("alert line %q does not carry %q", line, staffMarker("lockdown_alert_trigger_"+kind))
				}
				if strings.Contains(line, staffMarker("lockdown_alert_trigger_manual")) {
					t.Errorf("alert line %q is the manual wording", line)
				}
			} else {
				if sentence != "" {
					t.Errorf("trigger sentence = %q, want none for a manual lockdown", sentence)
				}
				if !strings.Contains(line, staffMarker("lockdown_alert_trigger_manual")) {
					t.Errorf("alert line %q is not the manual wording", line)
				}
			}

			if got := lockdownStartedAutomatically(ld); got != want.automatic {
				t.Errorf("lockdownStartedAutomatically = %v, want %v", got, want.automatic)
			}

			sample := lockdownRenderSample(tr, nil, lockdown.BurstCounts{}, kind)
			if !strings.Contains(sample, staffMarker(want.counts)+" ") {
				t.Errorf("sample %q does not use the counts line %q", sample, want.counts)
			}

			before := len(env.fake.sentTo(env.chat.Id))
			raidPostNotice(env.bot, &env.chat, raidTrigger{Kind: kind, Count: 7, Window: 120, Percent: 40}, lockdownStartResult{})
			sent := env.fake.sentTo(env.chat.Id)
			if !want.automatic {
				if len(sent) != before {
					t.Errorf("a manual kind posted %d public notices, want none", len(sent)-before)
				}
				return
			}
			if len(sent) != before+1 {
				t.Fatalf("%d public notices, want exactly 1", len(sent)-before)
			}
			notice := fmt.Sprint(sent[len(sent)-1].Params["text"])
			if !strings.Contains(notice, staffMarker("lockdown_auto_started_"+kind)) {
				t.Errorf("notice %q does not carry %q", notice, staffMarker("lockdown_auto_started_"+kind))
			}
		})
	}
}
