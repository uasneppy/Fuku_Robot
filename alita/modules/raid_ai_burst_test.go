//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/aispam"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
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

// serveTypeSafe configures TypeSafe for the test and answers every request from a fake:
// a text that holds SPAM is a delete verdict at 0.95, any other text a keep verdict at
// 0.1. It does not turn /aispam on for any chat, so the raid path alone checks.
func (e *lockdownEnv) serveTypeSafe() {
	t := e.t
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
}

// enableAI turns /aispam on for the env's chat, answering from the TypeSafe fake of
// serveTypeSafe, and loads the AI spam module on the env's dispatcher.
func (e *lockdownEnv) enableAI() {
	t, env := e.t, e
	t.Helper()
	env.serveTypeSafe()
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

// settleAI waits until every raid AI job and /aispam check the env's messages queued has
// finished, together with the public notice or warning an automatic lockdown posts. The
// raid AI jobs come first: a verdict of one can start the lockdown whose posts are waited
// for last.
func (e *lockdownEnv) settleAI() {
	e.t.Helper()
	raidAIJobs.wait()
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

// TestRaidAISpamSharesVerdict shows a text gets one TypeSafe call whichever path judges it:
// where /aispam judges, the raid path sends nothing and reuses /aispam's verdict (D-03,
// D-19); elsewhere the raid path checks and never deletes.
func TestRaidAISpamSharesVerdict(t *testing.T) {
	t.Run("/aispam on, one call per text", func(t *testing.T) {
		env := newAIBurstEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.sendText(member, spamLink)
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Errorf("TypeSafe requests = %d, want exactly 1: /aispam judges, the raid path sends nothing", got)
		}
		if got := len(env.calls("deleteMessage")); got != 1 {
			t.Errorf("deleteMessage calls = %d, want the 1 /aispam deletion", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 1 {
			t.Errorf("verdict items = %d, want the message counted once", got)
		}
	})

	t.Run("/aispam off, the raid path checks and deletes nothing", func(t *testing.T) {
		env := newRaidAIEnv(t)
		LoadAISpam(env.dispatcher)
		member := env.newMembers(1, 2*time.Hour)[0]

		env.sendText(member, spamLink)
		env.settleAI()

		if got := env.aiRequests(); got != 1 {
			t.Errorf("TypeSafe requests = %d, want exactly 1 from the raid path", got)
		}
		if got := len(env.calls("deleteMessage")); got != 0 {
			t.Errorf("deleteMessage calls = %d, want 0: the raid path never deletes", got)
		}
		if got := aiVerdictSetSize(env, "av"); got != 1 {
			t.Errorf("verdict items = %d, want the message counted once", got)
		}
	})

	for _, tc := range []struct {
		name string
		env  func(t *testing.T) *lockdownEnv
	}{
		{"/aispam on", newAIBurstEnv},
		{"/aispam off", func(t *testing.T) *lockdownEnv {
			env := newRaidAIEnv(t)
			LoadAISpam(env.dispatcher)
			return env
		}},
	} {
		t.Run("a keep verdict counts on neither path: "+tc.name, func(t *testing.T) {
			env := tc.env(t)
			member := env.newMembers(1, 2*time.Hour)[0]

			env.sendText(member, "see https://example.org/page")
			env.settleAI()

			if got := env.aiRequests(); got != 1 {
				t.Fatalf("TypeSafe requests = %d, want 1: the case would prove nothing", got)
			}
			if got := aiVerdictSetSize(env, "av"); got != 0 {
				t.Errorf("verdict items = %d, want 0 for a keep verdict", got)
			}
			if got := len(env.calls("deleteMessage")); got != 0 {
				t.Errorf("deleteMessage calls = %d, want 0", got)
			}
		})
	}
}

func TestRaidAIBurstSettings(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		env := newAIBurstEnv(t)
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 1)
		env.spamFrom(accounts[2], 1)
		env.settleAI()
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 4 verdicts with no settings row, want none", row.TriggerKind)
		}

		env.spamFrom(accounts[1], 1)
		env.settleAI()
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst || row.TriggerCount != 5 {
			t.Fatalf("active lockdown = %+v after the 5th verdict, want an AI burst lockdown of 5 verdicts (the default)", row)
		}
	})

	t.Run("a stored threshold", func(t *testing.T) {
		env := newAIBurstEnv(t)
		seedRaidSettings(t, models.ChatRaidDetection{
			ChatID: env.chat.Id, JoinEnabled: true, JoinThreshold: 15, FloodEnabled: true, FloodThreshold: 30,
			AIEnabled: true, AIThreshold: 3,
		})
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 1)
		env.spamFrom(accounts[1], 1)
		env.settleAI()
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown exists after 2 verdicts with a threshold of 3")
		}

		env.spamFrom(accounts[2], 1)
		env.settleAI()
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst {
			t.Fatalf("active lockdown = %+v after 3 verdicts from 3 accounts with a threshold of 3, want an AI burst lockdown", row)
		}
		if row.TriggerCount != 3 {
			t.Errorf("TriggerCount = %d, want 3", row.TriggerCount)
		}
	})

	t.Run("AI off", func(t *testing.T) {
		env := newAIBurstEnv(t)
		seedRaidSettings(t, models.ChatRaidDetection{
			ChatID: env.chat.Id, JoinEnabled: true, JoinThreshold: 15, FloodEnabled: true, FloodThreshold: 30,
			AIEnabled: false, AIThreshold: 3,
		})
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 2)
		env.spamFrom(accounts[2], 2)
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists with the AI trigger off", row.TriggerKind)
		}
		if got := aiVerdictSetSize(env, "av"); got != 0 {
			t.Errorf("verdict items counted = %d, want 0: nothing is counted while the trigger is off", got)
		}
		if got := len(env.calls("deleteMessage")); got != 6 {
			t.Errorf("deleteMessage calls = %d, want the 6 /aispam deletions: the switch only stops the counting", got)
		}
	})

	t.Run("settings unreadable", func(t *testing.T) {
		env := newAIBurstEnv(t)
		previous := raidSettingsLookup
		raidSettingsLookup = func(context.Context, int64) (*models.ChatRaidDetection, error) {
			return nil, errors.New("settings unreadable")
		}
		t.Cleanup(func() { raidSettingsLookup = previous })
		accounts := env.newMembers(3, 0)

		env.spamFrom(accounts[0], 2)
		env.spamFrom(accounts[1], 2)
		env.spamFrom(accounts[2], 2)
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists although the settings could not be read", row.TriggerKind)
		}
		if got := aiVerdictSetSize(env, "av"); got != 0 {
			t.Errorf("verdict items counted = %d, want 0: an unreadable setting counts nothing", got)
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

	t.Run("spammers listed", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		accounts := env.aiSpammers()
		env.aiRaidFrom(accounts)
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst {
			t.Fatalf("active lockdown = %+v, want one started by the AI burst", row)
		}

		env.cycle()

		text := alertText(t, env, staffChat)
		for i, account := range accounts {
			for _, want := range []string{
				html.EscapeString(account.user.FirstName),
				"@" + account.username,
				fmt.Sprintf("<code>%d</code>", account.user.Id),
			} {
				if !strings.Contains(text, want) {
					t.Errorf("alert %q does not contain %q (account %d)", text, want, i+1)
				}
			}
		}
		for _, unescaped := range []string{"<b>1</b>", "<b>2</b>", "<b>3</b>"} {
			if strings.Contains(text, unescaped) {
				t.Errorf("alert %q holds an unescaped name tag %q", text, unescaped)
			}
		}
		if want := staffMarker("lockdown_alert_counts_ai") + " 3 0"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the AI counts %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_counts")+" ") {
			t.Errorf("alert %q uses the join counts line, want the AI one", text)
		}

		rows := env.burstRows()
		if len(rows) != 3 {
			t.Fatalf("%d burst rows, want 3", len(rows))
		}
		locked := env.freshRow(row.ID).LockedAt
		if locked == nil {
			t.Fatal("the lockdown has no locked_at")
		}
		established := accounts[2].user.Id
		for _, burst := range rows {
			if burst.Source != models.BurstSourcePre || burst.JoinedAt == nil {
				t.Errorf("burst row = %+v, want a pre row with a time", burst)
				continue
			}
			ago := locked.Sub(*burst.JoinedAt)
			if burst.UserID == established {
				if ago < -2*time.Second || ago > time.Minute {
					t.Errorf("burst row of the established member is %s before the lock, want within a minute (the verdict time)", ago)
				}
				continue
			}
			if ago < 2*time.Hour+55*time.Minute || ago > 3*time.Hour+5*time.Minute {
				t.Errorf("burst row of %d joined %s before the lock, want about 3 hours", burst.UserID, ago)
			}
		}
	})

	t.Run("ban the spammers", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		accounts := env.aiSpammers()
		env.aiRaidFrom(accounts)
		env.cycle()

		a := lastAlert(t, env, staffChat)
		keyboard := staffKeyboardOf(a.alert.Params["reply_markup"])
		if len(keyboard) < 2 || len(keyboard[1]) != 1 {
			t.Fatalf("alert keyboard = %+v, want a second row with the Ban button", keyboard)
		}
		if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 3") {
			t.Errorf("Ban button label = %q, want it to carry %q with 3", label, staffMarker("lockdown_alert_btn_ban"))
		}

		card := env.askBan(a, env.admin)
		if text := fmt.Sprint(card.msg.Params["text"]); !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 3 ") {
			t.Errorf("card text = %q, want %q with 3", text, staffMarker("lockdown_ban_ask"))
		}
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		for i, account := range accounts {
			env.wantBannedForGood(account.user.Id, fmt.Sprintf("spammer %d", i+1))
			if row := env.burstRowOf(account.user.Id); row.State != models.BurstStateBanned {
				t.Errorf("burst row of spammer %d = %s, want %s", i+1, row.State, models.BurstStateBanned)
			}
		}
		env.wantNotBanned(staffTestBotID, "the bot itself")
	})

	t.Run("an approved spammer is left out", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		accounts := env.aiSpammers()
		env.aiRaidFrom(accounts)
		approved := accounts[1].user
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, ""); err != nil {
			t.Fatalf("approve %d: %v", approved.Id, err)
		}
		env.cycle()

		a := lastAlert(t, env, staffChat)
		keyboard := staffKeyboardOf(a.alert.Params["reply_markup"])
		if len(keyboard) < 2 || len(keyboard[1]) != 1 {
			t.Fatalf("alert keyboard = %+v, want a second row with the Ban button", keyboard)
		}
		if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 2") {
			t.Errorf("Ban button label = %q, want it to carry %q with 2", label, staffMarker("lockdown_alert_btn_ban"))
		}
		card := env.askBan(a, env.admin)
		if text := fmt.Sprint(card.msg.Params["text"]); !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 2 ") {
			t.Errorf("card text = %q, want %q with 2", text, staffMarker("lockdown_ban_ask"))
		}
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		env.wantNotBanned(approved.Id, "the approved spammer")
		env.wantBannedForGood(accounts[0].user.Id, "spammer 1")
		env.wantBannedForGood(accounts[2].user.Id, "spammer 3")
	})

	t.Run("snapshot unreadable", func(t *testing.T) {
		env := newAIBurstEnv(t)
		staffChat := env.staffChatOf()
		first, second := env.newJoiner("Joiner1"), env.newJoiner("Joiner2")
		env.join(first, first, "")
		env.join(second, second, "")
		accounts := env.aiSpammers()
		env.aiRaidFrom(accounts)
		if row := env.activeLockdown(); row == nil || row.TriggerKind != models.LockdownTriggerAIBurst {
			t.Fatalf("active lockdown = %+v, want one started by the AI burst", row)
		}

		var reads atomic.Int32
		previous := raidAISpammersLookup
		raidAISpammersLookup = func(int64) ([]lockdownRecentJoin, error) {
			reads.Add(1)
			return nil, errors.New("the snapshot could not be read")
		}
		t.Cleanup(func() { raidAISpammersLookup = previous })
		env.cycle()

		if reads.Load() == 0 {
			t.Fatal("the spammer snapshot was never read: the case would prove nothing")
		}
		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_counts_ai") + " 2 0"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q: the tracker's two joins only", text, want)
		}
		for _, joiner := range []gotgbot.User{first, second} {
			if want := fmt.Sprintf("<code>%d</code>", joiner.Id); !strings.Contains(text, want) {
				t.Errorf("alert %q does not list the tracker's joiner %q", text, want)
			}
		}
		for _, account := range accounts {
			if unwanted := fmt.Sprintf("<code>%d</code>", account.user.Id); strings.Contains(text, unwanted) {
				t.Errorf("alert %q lists spammer %d although the snapshot was unreadable", text, account.user.Id)
			}
		}
	})
}

// aiSpammers makes the three accounts of an AI burst, each with a users row and markup in
// its name: the first two joined three hours ago, the third is an established member
// (no joined score). They are returned in that order.
func (e *lockdownEnv) aiSpammers() []floodAccount {
	e.t.Helper()
	accounts := e.seedFlooders(2, 3*time.Hour, true)

	established := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Old<b>3</b>", Username: "oldtimer3"}
	row := models.User{UserId: established.Id, UserName: established.Username, Name: established.FirstName, LastActivity: time.Now()}
	if err := db.DB.Create(&row).Error; err != nil {
		e.t.Fatalf("seed the users row of %d: %v", established.Id, err)
	}
	e.t.Cleanup(func() { db.DB.Where("user_id = ?", established.Id).Delete(&models.User{}) })
	return append(accounts, floodAccount{user: established, username: established.Username})
}

// aiRaidFrom has the three accounts send 2, 2 and 1 spam texts and waits for the verdicts,
// which is the burst that locks the chat.
func (e *lockdownEnv) aiRaidFrom(accounts []floodAccount) {
	e.t.Helper()
	e.spamFrom(accounts[0].user, 2)
	e.spamFrom(accounts[1].user, 2)
	e.spamFrom(accounts[2].user, 1)
	e.settleAI()
}

func TestRaidAIVerdictScript(t *testing.T) {
	const chatID int64 = -1001234500001
	setup := func(t *testing.T) (*miniredis.Miniredis, *redis.Client, *time.Time) {
		t.Helper()
		server := withMiniredisServer(t)
		now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
		raidSetClock(t, func() time.Time { return now })
		return server, cache.GetRedisClient(), &now
	}
	record := func(t *testing.T, client *redis.Client, userID int64, itemKey string) (int, int) {
		t.Helper()
		items, accounts, err := recordRaidAIVerdict(client, chatID, userID, itemKey)
		if err != nil {
			t.Fatalf("recordRaidAIVerdict(%d, %s) error = %v", userID, itemKey, err)
		}
		return items, accounts
	}
	members := func(t *testing.T, server *miniredis.Miniredis, part string) []string {
		t.Helper()
		got, err := server.ZMembers(raidKey(chatID, part))
		if err != nil {
			return nil
		}
		return got
	}

	t.Run("the same item twice is one item and one account", func(t *testing.T) {
		_, client, _ := setup(t)
		if items, accounts := record(t, client, 11, "m:1"); items != 1 || accounts != 1 {
			t.Fatalf("first report = %d items, %d accounts, want 1, 1", items, accounts)
		}
		if items, accounts := record(t, client, 11, "m:1"); items != 1 || accounts != 1 {
			t.Errorf("second report of the same message = %d items, %d accounts, want 1, 1", items, accounts)
		}
	})

	t.Run("two items of one account are two items and one account", func(t *testing.T) {
		_, client, _ := setup(t)
		record(t, client, 11, "m:1")
		if items, accounts := record(t, client, 11, "m:2"); items != 2 || accounts != 1 {
			t.Errorf("two messages of one account = %d items, %d accounts, want 2, 1", items, accounts)
		}
	})

	t.Run("an album is one item", func(t *testing.T) {
		_, client, _ := setup(t)
		var items, accounts int
		for id := int64(1); id <= 3; id++ {
			msg := gotgbot.Message{MessageId: id, MediaGroupId: "album-9"}
			if key := raidAIItemKey(&msg); key != "g:album-9" {
				t.Fatalf("raidAIItemKey(album part %d) = %q, want g:album-9", id, key)
			}
			items, accounts = record(t, client, 11, raidAIItemKey(&msg))
		}
		if items != 1 || accounts != 1 {
			t.Errorf("three photos of one album = %d items, %d accounts, want 1, 1", items, accounts)
		}
		if key := raidAIItemKey(&gotgbot.Message{MessageId: 42}); key != "m:42" {
			t.Errorf("raidAIItemKey(plain message) = %q, want m:42", key)
		}
	})

	t.Run("items older than the window are trimmed with their accounts", func(t *testing.T) {
		server, client, now := setup(t)
		record(t, client, 11, "m:1")
		*now = now.Add(raidAIWindow + time.Second)

		items, accounts := record(t, client, 12, "m:2")

		if items != 1 || accounts != 1 {
			t.Errorf("after the window = %d items, %d accounts, want only the new 1, 1", items, accounts)
		}
		if got := members(t, server, raidPartAIVerdicts); len(got) != 1 || got[0] != "m:2" {
			t.Errorf("verdict items = %v, want [m:2]", got)
		}
		if got := members(t, server, raidPartAIAccounts); len(got) != 1 || got[0] != "12" {
			t.Errorf("accounts = %v, want [12]", got)
		}
	})

	t.Run("a repeated report does not move its account", func(t *testing.T) {
		server, client, now := setup(t)
		first := *now
		record(t, client, 11, "m:1")
		*now = now.Add(time.Minute)

		record(t, client, 11, "m:1")

		score, err := server.ZScore(raidKey(chatID, raidPartAIAccounts), "11")
		if err != nil {
			t.Fatalf("read the account's score: %v", err)
		}
		if int64(score) != first.UnixMilli() {
			t.Errorf("account score = %d, want the first report's %d: a duplicate must not refresh it", int64(score), first.UnixMilli())
		}

		// A duplicate of an item that has aged out cannot bring its account back.
		*now = first.Add(raidAIWindow + time.Second)
		if items, accounts := record(t, client, 11, "m:1"); items != 0 || accounts != 0 {
			t.Errorf("duplicate of an aged-out item = %d items, %d accounts, want 0, 0", items, accounts)
		}
	})

	t.Run("the cap keeps the newest", func(t *testing.T) {
		server, client, now := setup(t)
		previous := raidAICap
		raidAICap = 3
		t.Cleanup(func() { raidAICap = previous })

		for id := int64(1); id <= 5; id++ {
			record(t, client, 100+id, fmt.Sprintf("m:%d", id))
			*now = now.Add(time.Second)
		}

		if got := members(t, server, raidPartAIVerdicts); len(got) != 3 || got[0] != "m:3" || got[2] != "m:5" {
			t.Errorf("verdict items = %v, want the newest 3 (m:3 to m:5)", got)
		}
		if got := members(t, server, raidPartAIAccounts); len(got) != 3 || got[0] != "103" || got[2] != "105" {
			t.Errorf("accounts = %v, want the newest 3 (103 to 105)", got)
		}
	})

	t.Run("both sets expire on their own", func(t *testing.T) {
		server, client, _ := setup(t)
		record(t, client, 11, "m:1")

		for _, part := range []string{raidPartAIVerdicts, raidPartAIAccounts} {
			ttl := server.TTL(raidKey(chatID, part))
			if ttl < 4*time.Minute+30*time.Second || ttl > 5*time.Minute {
				t.Errorf("TTL of %s = %s, want about 5 minutes", part, ttl)
			}
		}
	})

	t.Run("the rule", func(t *testing.T) {
		for _, tc := range []struct {
			items, accounts, threshold int
			want                       bool
		}{
			{5, 3, 5, true},
			{4, 3, 5, false},
			{5, 2, 5, false},
			{500, 3, 500, true},
			{2, 3, 2, false},
			{1, 9, 1, false},
			{0, 0, 0, false},
		} {
			if got := raidAICounts(tc.items, tc.accounts, tc.threshold); got != tc.want {
				t.Errorf("raidAICounts(%d items, %d accounts, threshold %d) = %v, want %v",
					tc.items, tc.accounts, tc.threshold, got, tc.want)
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
