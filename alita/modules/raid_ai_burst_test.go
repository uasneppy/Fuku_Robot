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

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/aispam"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// aiBurstKind is the trigger_kind of a lockdown the AI spam verdict counter started.
const aiBurstKind = "ai_burst"

// aiSpamRequests counts the requests the TypeSafe fake of the AI burst tests received.
var aiSpamRequests atomic.Int64

// newAIBurstEnv is a flood env (Staff Group, clean raid settings, env.admin the chat's
// creator) with /aispam enabled for its chat and answering from a TypeSafe fake: a text
// that holds SPAM is a delete verdict at 0.95, any other text a keep verdict at 0.1.
func newAIBurstEnv(t *testing.T) *lockdownEnv {
	t.Helper()
	env, _ := floodBurstEnv(t)
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
	return env
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
	if row == nil || row.TriggerKind != aiBurstKind || row.LockedAt == nil {
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
}
