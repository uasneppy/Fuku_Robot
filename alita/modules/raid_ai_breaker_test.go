//go:build testtools

package modules

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
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

	t.Run("gemini: five failures pause one group", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		advance := movableClock(t)
		withRaidAIGeminiCooldown(t, 10*time.Second)
		other := env.addChat()
		var failing atomic.Bool
		failing.Store(true)
		env.answerWith(brokenGemini(&failing))
		members := env.newMembers(raidAIGeminiFailures+2, 2*time.Hour)
		elsewhere := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, elsewhere.Id, time.Hour)
		before := GetRaidAIStats()

		for i := 0; i < raidAIGeminiFailures; i++ {
			env.sendMedia(members[i], gotgbot.Message{Photo: photoNamed(fmt.Sprintf("fail%d", i))})
		}
		env.settleAI()
		if got := env.geminiRequests(); got != raidAIGeminiFailures {
			t.Fatalf("Gemini requests = %d, want %d", got, raidAIGeminiFailures)
		}
		if got := GetRaidAIStats().Failed - before.Failed; got != raidAIGeminiFailures {
			t.Errorf("failed checks = %d, want %d", got, raidAIGeminiFailures)
		}
		if !raidAIGeminiPaused(env.chat.Id) {
			t.Fatal("the failures did not pause the group")
		}

		files := len(env.client.getFiles())
		env.sendMedia(members[raidAIGeminiFailures], gotgbot.Message{Photo: photoNamed("paused")})
		env.settleAI()
		if got := env.geminiRequests(); got != raidAIGeminiFailures {
			t.Errorf("Gemini requests = %d, want still %d: a paused group sends nothing", got, raidAIGeminiFailures)
		}
		if got := len(env.client.getFiles()); got != files {
			t.Errorf("getFile calls = %d, want still %d: a paused group downloads nothing", got, files)
		}
		if got := GetRaidAIStats().Shed - before.Shed; got != 1 {
			t.Errorf("shed checks = %d, want 1", got)
		}

		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("elsewhere")})
		env.settleAI()
		if got := env.geminiRequests(); got != raidAIGeminiFailures+1 {
			t.Errorf("Gemini requests = %d, want %d: another group is still checked", got, raidAIGeminiFailures+1)
		}

		advance(11 * time.Second)
		failing.Store(false)
		env.sendMedia(members[raidAIGeminiFailures+1], gotgbot.Message{Photo: photoNamed("resumed")})
		env.settleAI()
		if got := env.geminiRequests(); got != raidAIGeminiFailures+2 {
			t.Errorf("Gemini requests = %d, want %d: checks resume after the cooldown", got, raidAIGeminiFailures+2)
		}
		if raidAIGeminiPaused(env.chat.Id) {
			t.Error("the group stayed paused after its cooldown and a successful check")
		}
		wantNoNotice(env, env.chat, other)
	})

	t.Run("gemini: a success clears the count", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		var failing atomic.Bool
		failing.Store(true)
		env.answerWith(brokenGemini(&failing))
		members := env.newMembers(9, 2*time.Hour)

		send := func(i int, tag string) {
			env.sendMedia(members[i], gotgbot.Message{Photo: photoNamed(tag)})
			env.settleAI()
		}
		for i := 0; i < 3; i++ {
			send(i, fmt.Sprintf("before%d", i))
		}
		failing.Store(false)
		send(3, "success")
		failing.Store(true)
		for i := 4; i < 8; i++ {
			send(i, fmt.Sprintf("after%d", i))
		}
		if raidAIGeminiPaused(env.chat.Id) {
			t.Fatal("3 failures, a success and 4 failures paused the group: the success did not clear the count")
		}
		send(8, "last")
		if got := env.geminiRequests(); got != 9 {
			t.Errorf("Gemini requests = %d, want 9: the group was never paused", got)
		}
	})

	t.Run("gemini: a rejected key pauses everyone", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		advance := movableClock(t)
		withRaidAIGeminiKeyPause(t, 20*time.Second)
		other := env.addChat()
		var rejecting atomic.Bool
		rejecting.Store(true)
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			if rejecting.Load() {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":400,"message":"API key not valid.","status":"INVALID_ARGUMENT",` +
					`"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID"}]}}`))
				return
			}
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.1, "")))
		})
		members := env.newMembers(2, 2*time.Hour)
		elsewhere := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, elsewhere.Id, time.Hour)
		hook := captureLogs(t, log.WarnLevel)

		env.sendMedia(members[0], gotgbot.Message{Photo: photoNamed("rejected")})
		env.settleAI()
		if got := env.geminiRequests(); got != 1 {
			t.Fatalf("Gemini requests = %d, want 1", got)
		}

		files := len(env.client.getFiles())
		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("blocked-b")})
		env.sendMedia(members[1], gotgbot.Message{Photo: photoNamed("blocked-a")})
		env.settleAI()
		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want still 1: a rejected key pauses every group", got)
		}
		if got := len(env.client.getFiles()); got != files {
			t.Errorf("getFile calls = %d, want still %d", got, files)
		}
		if got := countPauseWarnings(hook); got != 1 {
			t.Errorf("pause warnings = %d, want exactly 1", got)
		}

		advance(21 * time.Second)
		rejecting.Store(false)
		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("resumed-b")})
		env.settleAI()
		if got := env.geminiRequests(); got != 2 {
			t.Errorf("Gemini requests = %d, want 2: checks resume when the pause ends", got)
		}
		wantNoNotice(env, env.chat, other)
	})

	t.Run("gemini: a long 429 pauses for its delay", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		advance := movableClock(t)
		other := env.addChat()
		var limited atomic.Bool
		limited.Store(true)
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			if limited.Load() {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED",` +
					`"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`))
				return
			}
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.1, "")))
		})
		members := env.newMembers(1, 2*time.Hour)
		elsewhere := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, elsewhere.Id, time.Hour)
		hook := captureLogs(t, log.WarnLevel)

		env.sendMedia(members[0], gotgbot.Message{Photo: photoNamed("limited")})
		env.settleAI()
		if got := env.geminiRequests(); got != 1 {
			t.Fatalf("Gemini requests = %d, want 1: a 30 s backoff is not retried", got)
		}

		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("limited-b")})
		env.settleAI()
		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want still 1: a long rate limit pauses every group", got)
		}
		if got := countPauseWarnings(hook); got != 1 {
			t.Errorf("pause warnings = %d, want exactly 1", got)
		}

		advance(31 * time.Second)
		limited.Store(false)
		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("limited-c")})
		env.settleAI()
		if got := env.geminiRequests(); got != 2 {
			t.Errorf("Gemini requests = %d, want 2: checks resume once the delay has passed", got)
		}
		wantNoNotice(env, env.chat, other)
	})

	t.Run("gemini: a short 429 is retried and pauses nothing", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		other := env.addChat()
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1s"}]}}`))
		})
		members := env.newMembers(1, 2*time.Hour)
		elsewhere := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, elsewhere.Id, time.Hour)
		hook := captureLogs(t, log.WarnLevel)

		env.sendMedia(members[0], gotgbot.Message{Photo: photoNamed("short")})
		env.settleAI()
		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2: the client retries a 1 s backoff once", got)
		}

		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("short-b")})
		env.settleAI()
		if got := env.geminiRequests(); got != 4 {
			t.Errorf("Gemini requests = %d, want 4: another group was still checked", got)
		}
		if got := countPauseWarnings(hook); got != 0 {
			t.Errorf("pause warnings = %d, want 0: a short rate limit pauses nothing", got)
		}
	})

	t.Run("gemini: downloads never trip it", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		resetRaidAIGeminiBreakers(t)
		members := env.newMembers(raidAIGeminiFailures+1, 2*time.Hour)
		before := GetRaidAIStats()

		for i := 0; i < raidAIGeminiFailures; i++ {
			tag := fmt.Sprintf("gone%d", i)
			env.serve("files/"+tag, raidAIServed{status: http.StatusInternalServerError, body: []byte("down")})
			env.sendMedia(members[i], gotgbot.Message{Photo: photoNamed(tag)})
		}
		env.settleAI()
		if got := GetRaidAIStats().Failed - before.Failed; got != raidAIGeminiFailures {
			t.Fatalf("failed checks = %d, want %d: the case would prove nothing", got, raidAIGeminiFailures)
		}
		if raidAIGeminiPaused(env.chat.Id) {
			t.Fatal("download failures paused the group")
		}

		env.sendMedia(members[raidAIGeminiFailures], gotgbot.Message{Photo: photoNamed("fine")})
		env.settleAI()
		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1: the group was not paused", got)
		}
	})
}

// brokenGemini answers 500 with a backoff longer than the client waits, so each check costs
// exactly one request, while failing holds; after that it answers a keep verdict.
func brokenGemini(failing *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(raidAIGeminiAnswer(0.1, "")))
	}
}

// resetRaidAIGeminiBreakers clears the Gemini breakers and the provider-wide pause now and
// when the test ends.
func resetRaidAIGeminiBreakers(t *testing.T) {
	t.Helper()
	reset := func() {
		raidAIGeminiBreakers.mu.Lock()
		raidAIGeminiBreakers.m = make(map[int64]*raidAIGeminiBreaker)
		raidAIGeminiBreakers.mu.Unlock()
		raidAIGeminiPause.mu.Lock()
		raidAIGeminiPause.pausedTill = time.Time{}
		raidAIGeminiPause.mu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// withRaidAIGeminiCooldown shortens the per-group pause for the test.
func withRaidAIGeminiCooldown(t *testing.T, d time.Duration) {
	t.Helper()
	previous := raidAIGeminiCooldown
	raidAIGeminiCooldown = d
	t.Cleanup(func() { raidAIGeminiCooldown = previous })
}

// withRaidAIGeminiKeyPause shortens the rejected-key pause for the test.
func withRaidAIGeminiKeyPause(t *testing.T, d time.Duration) {
	t.Helper()
	previous := raidAIGeminiKeyPause
	raidAIGeminiKeyPause = d
	t.Cleanup(func() { raidAIGeminiKeyPause = previous })
}

// countPauseWarnings counts the warnings that announce a pause of the image checks.
func countPauseWarnings(hook *logrustest.Hook) int {
	count := 0
	for _, entry := range hook.AllEntries() {
		if entry.Level == log.WarnLevel && strings.Contains(entry.Message, "image checks paused") {
			count++
		}
	}
	return count
}

// wantNoNotice fails when the bot sent any message to any of the chats: a paused check is
// silent.
func wantNoNotice(env *raidAIImageEnv, chats ...gotgbot.Chat) {
	env.t.Helper()
	for _, chat := range chats {
		if got := len(env.callsIn(chat.Id, "sendMessage")); got != 0 {
			env.t.Errorf("sendMessage calls to %d = %d, want 0: a paused image check posts no notice", chat.Id, got)
		}
	}
}
