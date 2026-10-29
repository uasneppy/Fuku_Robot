//go:build testtools

package modules

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// brokenTypeSafe answers every request with a 500 and a backoff longer than the client waits,
// so each check costs exactly one request, while fail holds; after that it answers a keep
// verdict. It counts the requests in requests.
func brokenTypeSafe(requests *atomic.Int64, fail *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(aispamDecisionBody(0.1, "none")))
	}
}

func TestRaidAIBreaker(t *testing.T) {
	t.Run("five failures pause the chat and nothing is posted", func(t *testing.T) {
		env := newRaidAIEnv(t)
		other := env.addChat()
		var requests atomic.Int64
		var fail atomic.Bool
		fail.Store(true)
		withAISpamJevServer(t, brokenTypeSafe(&requests, &fail))
		members := env.newMembers(6, time.Hour)

		for _, member := range members[:aispamBreakerFailures] {
			env.sendText(member, spamLink)
		}
		env.settleAI()

		if got := requests.Load(); got != aispamBreakerFailures {
			t.Fatalf("TypeSafe requests = %d, want %d", got, aispamBreakerFailures)
		}
		if got := GetRaidAIStats().Failed; got != aispamBreakerFailures {
			t.Errorf("failed checks = %d, want %d", got, aispamBreakerFailures)
		}
		if !aispamBreakerPaused(env.chat.Id) {
			t.Fatal("the failures did not pause the chat")
		}

		env.sendText(members[aispamBreakerFailures], spamLink)
		env.settleAI()

		if got := requests.Load(); got != aispamBreakerFailures {
			t.Errorf("TypeSafe requests = %d, want still %d: a paused chat sends nothing", got, aispamBreakerFailures)
		}
		if got := GetRaidAIStats().Shed; got != 1 {
			t.Errorf("shed checks = %d, want 1", got)
		}
		if got := len(env.calls("sendMessage")); got != 0 {
			t.Errorf("sendMessage calls = %d, want 0: the raid path never posts the /aispam breaker notice", got)
		}

		// Another chat has its own breaker and is still checked.
		member := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, member.Id, time.Hour)
		env.dispatchMessage(other, gotgbot.Message{From: &member, Text: spamLink})
		env.settleAI()
		if got := requests.Load(); got != aispamBreakerFailures+1 {
			t.Errorf("TypeSafe requests = %d, want %d: another chat is still checked", got, aispamBreakerFailures+1)
		}
		if aispamBreakerPaused(other.Id) {
			t.Error("one failure paused the other chat")
		}
	})

	t.Run("checks resume after the cooldown", func(t *testing.T) {
		env := newRaidAIEnv(t)
		previous := aispamBreakerCooldown
		aispamBreakerCooldown = 50 * time.Millisecond
		t.Cleanup(func() { aispamBreakerCooldown = previous })
		var requests atomic.Int64
		var fail atomic.Bool
		fail.Store(true)
		withAISpamJevServer(t, brokenTypeSafe(&requests, &fail))
		members := env.newMembers(aispamBreakerFailures+1, time.Hour)

		for _, member := range members[:aispamBreakerFailures] {
			env.sendText(member, spamLink)
		}
		env.settleAI()
		if !aispamBreakerPaused(env.chat.Id) {
			t.Fatal("the failures did not pause the chat")
		}

		time.Sleep(aispamBreakerCooldown + 20*time.Millisecond)
		fail.Store(false)
		env.sendText(members[aispamBreakerFailures], spamLink)
		env.settleAI()

		if got := requests.Load(); got != aispamBreakerFailures+1 {
			t.Errorf("TypeSafe requests = %d, want %d: checks resume after the cooldown", got, aispamBreakerFailures+1)
		}
		if aispamBreakerPaused(env.chat.Id) {
			t.Error("the chat stayed paused after its cooldown and a successful check")
		}
		if got := GetRaidAIStats().Checked; got != 1 {
			t.Errorf("checked = %d, want 1", got)
		}
		if got := len(env.calls("sendMessage")); got != 0 {
			t.Errorf("sendMessage calls = %d, want 0", got)
		}
	})
}
