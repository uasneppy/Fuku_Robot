//go:build testtools

package modules

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// ownerStatsText has the owner run /stats through the real getStats handler and returns the
// text the bot edited its "fetching" message to.
func ownerStatsText(t *testing.T) string {
	t.Helper()
	withOwnerID(t, 777000)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "private", FirstName: "Owner"}
	owner := gotgbot.User{Id: 777000, FirstName: "Owner"}

	if err := devsModule.getStats(bot, newModuleMessageContext(bot, chat, owner, "/stats")); err != nil && err != ext.ContinueGroups {
		t.Fatalf("getStats returned %v", err)
	}
	edits := client.callsFor("editMessageText")
	if len(edits) != 1 {
		t.Fatalf("editMessageText calls = %d, want the one edit that carries the stats", len(edits))
	}
	text, ok := edits[0].Params["text"].(string)
	if !ok {
		t.Fatalf("the stats edit carries no text: %v", edits[0].Params)
	}
	return text
}

func TestRaidAIStats(t *testing.T) {
	env := newRaidAIImageEnv(t)
	withRaidAIBudgets(t, 2, 1000)
	resetRaidAIStats()

	// TypeSafe: a text with SPAM is a spam verdict, a text with BROKEN is a provider failure
	// (one request, no retry), anything else is kept.
	var typeSafeRequests atomic.Int64
	withAISpamJevServer(t, func(w http.ResponseWriter, r *http.Request) {
		typeSafeRequests.Add(1)
		raw, _ := io.ReadAll(r.Body)
		switch body := string(raw); {
		case strings.Contains(body, "BROKEN"):
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(body, "SPAM"):
			_, _ = w.Write([]byte(aispamDecisionBody(0.95, "scam")))
		default:
			_, _ = w.Write([]byte(aispamDecisionBody(0.1, "none")))
		}
	})
	// Gemini: the first call is a spam verdict, the second a safety block.
	var geminiCalls atomic.Int64
	env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
		if geminiCalls.Add(1) == 1 {
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.9, "")))
			return
		}
		_, _ = w.Write([]byte(`{"promptFeedback":{"blockReason":"IMAGE_SAFETY"}}`))
	})

	members := env.newMembers(7, 2*time.Hour)
	send := func(member gotgbot.User, msg gotgbot.Message) {
		env.sendMedia(member, msg)
		env.settleAI()
	}

	// Texts: spam, keep, and a provider failure.
	send(members[0], gotgbot.Message{Text: spamLink})
	send(members[1], gotgbot.Message{Text: "my site is https://example.org/page"})
	send(members[2], gotgbot.Message{Text: "BROKEN deal t.me/scam"})
	if got := typeSafeRequests.Load(); got != 3 {
		t.Fatalf("TypeSafe requests = %d, want 3 (the scenario would prove nothing)", got)
	}

	// Images: spam, the same image again (cache), a blocked one, and one over the budget.
	send(members[3], gotgbot.Message{Photo: photoNamed("st-spam")})
	send(members[4], gotgbot.Message{Photo: photoNamed("st-spam")})
	send(members[5], gotgbot.Message{Photo: photoNamed("st-blocked")})
	send(members[6], gotgbot.Message{Photo: photoNamed("st-capped")})
	if got := env.geminiRequests(); got != 2 {
		t.Fatalf("Gemini requests = %d, want 2: the repeat is cached and the third image is over the budget", got)
	}
	env.wantNoActions()

	// A group with /aispam on: its delete verdict is counted by /aispam, not by the raid path.
	env.enableAI()
	extra := env.newMembers(1, 2*time.Hour)[0]
	env.sendText(extra, "SPAM offer t.me/scam again")
	env.settleAI()

	text := ownerStatsText(t)

	if !strings.Contains(text, "AI checks (this replica") {
		t.Errorf("the /stats text has no per-replica AI heading:\n%s", text)
	}
	const raidLine = "checked 3, spam 3, kept 1, shed 0, failed 1, blocked 1, capped 1, cached 1"
	if !strings.Contains(text, raidLine) {
		t.Errorf("the /stats text lacks the raid AI line %q:\n%s", raidLine, text)
	}
	const aispamLine = "checked 1, deleted 1, kept 0, shed 0, failed 0"
	if !strings.Contains(text, aispamLine) {
		t.Errorf("the /stats text lacks the /aispam line %q:\n%s", aispamLine, text)
	}
	if !strings.Contains(text, "Raid detection") || !strings.Contains(text, "AI spam off") {
		t.Errorf("the /stats text lost the raid detection section or its AI spam off line:\n%s", text)
	}

	// Counters only: no message text, file ID, name or key may reach the owner's chat.
	for _, leak := range []string{
		"t.me/scam", "example.org", "BROKEN", "SPAM offer",
		"st-spam", "st-blocked", "st-capped",
		raidAITestKey, "typesafe-test-key",
	} {
		if strings.Contains(text, leak) {
			t.Errorf("the /stats text contains %q: only counters may appear", leak)
		}
	}
}

func TestRaidAIStatsDevsOnly(t *testing.T) {
	withOwnerID(t, 777000)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "private", FirstName: "Guest"}
	guest := gotgbot.User{Id: 4040404, FirstName: "Guest"}

	if err := devsModule.getStats(bot, newModuleMessageContext(bot, chat, guest, "/stats")); err != ext.ContinueGroups {
		t.Fatalf("getStats(guest) = %v, want ext.ContinueGroups", err)
	}
	for _, method := range []string{"sendMessage", "editMessageText"} {
		if got := len(client.callsFor(method)); got != 0 {
			t.Errorf("%s calls = %d, want 0: /stats is for the owner and developers only", method, got)
		}
	}
}
