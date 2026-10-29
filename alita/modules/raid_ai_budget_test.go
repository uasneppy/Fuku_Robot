//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// photoNamed is a one-size photo whose file ID is tag and whose file_unique_id is "u"+tag, so
// two photos with different tags are two different images.
func photoNamed(tag string) []gotgbot.PhotoSize {
	return []gotgbot.PhotoSize{{FileId: tag, FileUniqueId: "u" + tag, Width: 800, Height: 600}}
}

// sendMediaIn has user send msg in chat through the env's real dispatcher.
func (e *raidAIImageEnv) sendMediaIn(chat gotgbot.Chat, user gotgbot.User, msg gotgbot.Message) {
	e.t.Helper()
	msg.From = &user
	e.dispatchMessage(chat, msg)
}

// withRaidAIBudgets sets the hourly budgets for the test and restores them when it ends.
func withRaidAIBudgets(t *testing.T, perChat, total int64) {
	t.Helper()
	previousChat, previousTotal := raidAIChatHourlyBudget, raidAIGlobalHourlyBudget
	raidAIChatHourlyBudget, raidAIGlobalHourlyBudget = perChat, total
	t.Cleanup(func() { raidAIChatHourlyBudget, raidAIGlobalHourlyBudget = previousChat, previousTotal })
}

// withRaidAIUserBudget sets the hourly per-account budget for the test and restores it when it ends.
func withRaidAIUserBudget(t *testing.T, perAccount int64) {
	t.Helper()
	previous := raidAIUserHourlyBudget
	raidAIUserHourlyBudget = perAccount
	t.Cleanup(func() { raidAIUserHourlyBudget = previous })
}

// movableClock makes raidNow a clock the test can move forward, starting at the real time.
func movableClock(t *testing.T) (advance func(time.Duration)) {
	t.Helper()
	base := time.Now()
	var offset atomic.Int64
	raidSetClock(t, func() time.Time { return base.Add(time.Duration(offset.Load())) })
	return func(d time.Duration) { offset.Add(int64(d)) }
}

func TestRaidAIBudget(t *testing.T) {
	t.Run("per-group cap", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		withRaidAIBudgets(t, 2, 1000)
		other := env.addChat()
		members := env.newMembers(3, 2*time.Hour)
		elsewhere := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, elsewhere.Id, time.Hour)
		before := GetRaidAIStats()

		for i, member := range members {
			env.sendMedia(member, gotgbot.Message{Photo: photoNamed(fmt.Sprintf("cap%d", i))})
		}
		env.settleAI()

		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2: the third photo is over the group's budget", got)
		}
		if got := GetRaidAIStats().Capped - before.Capped; got != 1 {
			t.Errorf("Capped grew by %d, want 1", got)
		}

		env.sendMediaIn(other, elsewhere, gotgbot.Message{Photo: photoNamed("elsewhere")})
		env.settleAI()
		if got := env.geminiRequests(); got != 3 {
			t.Errorf("Gemini requests = %d, want 3: another group has its own budget", got)
		}
		env.wantNoActions()
	})

	t.Run("global cap", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		withRaidAIBudgets(t, 100, 3)
		chats := []gotgbot.Chat{env.chat, env.addChat(), env.addChat()}
		before := GetRaidAIStats()
		for i, chat := range chats {
			member := env.newJoiner(fmt.Sprintf("Global%d", i))
			seedJoinedIn(t, chat.Id, member.Id, time.Hour)
			env.sendMediaIn(chat, member, gotgbot.Message{Photo: photoNamed(fmt.Sprintf("glob%d", i))})
		}
		env.settleAI()
		if got := env.geminiRequests(); got != 3 {
			t.Fatalf("Gemini requests = %d, want 3 across three groups", got)
		}

		late := env.newJoiner("Late")
		seedJoinedIn(t, env.chat.Id, late.Id, time.Hour)
		env.sendMedia(late, gotgbot.Message{Photo: photoNamed("glob-late")})
		env.settleAI()

		if got := env.geminiRequests(); got != 3 {
			t.Errorf("Gemini requests = %d, want still 3: the fleet is out of budget", got)
		}
		if got := GetRaidAIStats().Capped - before.Capped; got != 1 {
			t.Errorf("Capped grew by %d, want 1", got)
		}
	})

	t.Run("one account cannot exhaust the group or the fleet budget", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		const perAccount, distinctPhotos = 4, 12
		withRaidAIUserBudget(t, perAccount)
		withRaidAIBudgets(t, 10, 10)
		spammer, bystander := env.newMembers(2, 2*time.Hour)[0], env.newJoiner("Bystander")
		seedJoinedIn(t, env.chat.Id, bystander.Id, time.Hour)
		before := GetRaidAIStats()

		for i := 0; i < distinctPhotos; i++ {
			env.sendMedia(spammer, gotgbot.Message{Photo: photoNamed(fmt.Sprintf("flood%d", i))})
			env.settleAI()
		}

		if got := env.geminiRequests(); got != perAccount {
			t.Fatalf("Gemini requests = %d, want %d: one account's cap", got, perAccount)
		}
		if got := GetRaidAIStats().Capped - before.Capped; got != distinctPhotos-perAccount {
			t.Errorf("Capped grew by %d, want %d", got, distinctPhotos-perAccount)
		}
		stamp := raidAIHourStamp(raidNow())
		for _, shared := range []string{raidKey(env.chat.Id, raidPartAIBudget+":"+stamp), raidAIGlobalBudgetPrefix + stamp} {
			if got, err := env.redis.Get(shared); err != nil || got != fmt.Sprint(perAccount) {
				t.Errorf("shared budget %q = %q (error %v), want %d: an account over its cap spends no shared unit", shared, got, err, perAccount)
			}
		}

		env.sendMedia(bystander, gotgbot.Message{Photo: photoNamed("bystander")})
		env.settleAI()
		if got := env.geminiRequests(); got != perAccount+1 {
			t.Errorf("Gemini requests = %d, want %d: another member's photo is still judged", got, perAccount+1)
		}
	})

	t.Run("a cached verdict spends none of the account's budget", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		withRaidAIUserBudget(t, 1)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.serve("files/stk", raidAIServed{status: http.StatusOK, body: raidAITestWebP("sticker")})
		sticker := gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk-user", Type: "regular"}}
		before := GetRaidAIStats()

		for i := 0; i < 4; i++ {
			env.sendMedia(member, sticker)
			env.settleAI()
		}

		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1 for one image posted four times", got)
		}
		if got := GetRaidAIStats().Capped - before.Capped; got != 0 {
			t.Errorf("Capped grew by %d, want 0: the cache answers before any budget", got)
		}
	})

	t.Run("a new hour resets", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		advance := movableClock(t)
		withRaidAIBudgets(t, 2, 1000)
		members := env.newMembers(4, 2*time.Hour)

		for i := 0; i < 3; i++ {
			env.sendMedia(members[i], gotgbot.Message{Photo: photoNamed(fmt.Sprintf("hour%d", i))})
		}
		env.settleAI()
		if got := env.geminiRequests(); got != 2 {
			t.Fatalf("Gemini requests = %d, want 2 before the hour turns", got)
		}

		advance(time.Hour)
		env.sendMedia(members[3], gotgbot.Message{Photo: photoNamed("hour3")})
		env.settleAI()

		if got := env.geminiRequests(); got != 3 {
			t.Errorf("Gemini requests = %d, want 3: the next hour has a fresh budget", got)
		}
	})

	t.Run("a cached verdict costs nothing", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		withRaidAIBudgets(t, 1, 1)
		members := env.newMembers(3, 2*time.Hour)
		env.serve("files/stk", raidAIServed{status: http.StatusOK, body: raidAITestWebP("sticker")})
		sticker := gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk-same", Type: "regular"}}
		before := GetRaidAIStats()

		for _, member := range members {
			env.sendMedia(member, sticker)
			env.settleAI()
		}

		if got := env.geminiRequests(); got != 1 {
			t.Fatalf("Gemini requests = %d, want 1 for one image posted three times", got)
		}
		if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 3 {
			t.Errorf("verdict items = %d, want 3: each post counts as a verdict from its own message", got)
		}
		got := GetRaidAIStats()
		if got.Cached-before.Cached != 2 || got.Capped != before.Capped {
			t.Errorf("stats = %+v (before %+v), want 2 more cached and none capped: a cached verdict spends no budget", got, before)
		}
		if got.Checked-before.Checked != 1 || got.Spam-before.Spam != 3 {
			t.Errorf("stats = %+v (before %+v), want 1 checked and 3 spam", got, before)
		}
	})

	t.Run("the same image at the same moment is judged once", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		members := env.newMembers(3, 2*time.Hour)
		env.serve("files/stk", raidAIServed{status: http.StatusOK, body: raidAITestWebP("sticker")})
		release := make(chan struct{})
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			<-release
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.9, "")))
		})
		sticker := gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk-race", Type: "regular"}}

		for _, member := range members {
			env.sendMedia(member, sticker)
		}
		time.Sleep(200 * time.Millisecond)
		close(release)
		env.settleAI()

		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1: the posts that overlap share the one call", got)
		}
		if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 3 {
			t.Errorf("verdict items = %d, want 3", got)
		}
	})

	t.Run("an album is judged once", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		before := GetRaidAIStats()

		for i := 0; i < 10; i++ {
			env.sendMedia(member, gotgbot.Message{Photo: photoNamed(fmt.Sprintf("album%d", i)), MediaGroupId: "alb1"})
		}
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 1 {
			t.Errorf("getFile asked for %v, want exactly one for a ten-photo album", got)
		}
		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1", got)
		}
		items, err := env.redis.ZMembers(raidKey(env.chat.Id, "av"))
		if err != nil || len(items) != 1 || items[0] != "g:alb1" {
			t.Errorf("verdict items = %v (error %v), want exactly g:alb1", items, err)
		}
		if got := GetRaidAIStats().Cached - before.Cached; got != 9 {
			t.Errorf("Cached grew by %d, want 9 (the other photos of the album)", got)
		}
	})

	t.Run("an album photo refused for a moment does not drop the rest of the album", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		advance := movableClock(t)
		withRaidAIGeminiCooldown(t, time.Minute)
		member := env.newMembers(1, 2*time.Hour)[0]
		for range raidAIGeminiFailures {
			raidAIGeminiRecordFailure(env.chat.Id, errors.New("provider down"))
		}
		if !raidAIGeminiPaused(env.chat.Id) {
			t.Fatal("the chat is not paused: the case would prove nothing")
		}

		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("alb-first"), MediaGroupId: "alb-retry"})
		env.settleAI()
		if got := env.geminiRequests(); got != 0 {
			t.Fatalf("Gemini requests = %d while paused, want 0", got)
		}

		advance(2 * time.Minute)
		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("alb-second"), MediaGroupId: "alb-retry"})
		env.settleAI()

		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1: the album was not judged because its first photo met a pause", got)
		}
	})

	t.Run("an album is not judged twice once a photo reached Gemini", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) })

		for i := 0; i < 3; i++ {
			env.sendMedia(member, gotgbot.Message{Photo: photoNamed(fmt.Sprintf("alb-fail%d", i)), MediaGroupId: "alb-fail"})
			env.settleAI()
		}

		if got := env.geminiRequests(); got != 1 {
			t.Errorf("Gemini requests = %d, want 1: the failing album costs its first photo's call only", got)
		}
	})

	t.Run("a refusal in one chat is not inherited by another chat", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		other := env.addChat()
		first := env.newMembers(1, 2*time.Hour)[0]
		second := env.newJoiner("Elsewhere")
		seedJoinedIn(t, other.Id, second.Id, time.Hour)
		env.serve("files/stk", raidAIServed{status: http.StatusOK, body: raidAITestWebP("sticker")})
		sticker := gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "stk", FileUniqueId: "ustk-cross", Type: "regular"}}

		release := make(chan struct{})
		var releaseOnce sync.Once
		open := func() { releaseOnce.Do(func() { close(release) }) }
		// Opened on every exit, so a failed wait cannot leave the Gemini handler blocked.
		t.Cleanup(open)
		var calls atomic.Int64
		env.answerWith(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				<-release
				// Not retried by the client, so the leader's one call fails for good.
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(raidAIGeminiAnswer(0.9, "")))
		})
		before := GetRaidAIStats()

		env.sendMedia(first, sticker)
		waitUntil(t, 10*time.Second, "the first chat's call to reach Gemini", func() bool { return calls.Load() == 1 })
		env.sendMediaIn(other, second, sticker)
		waitUntil(t, 10*time.Second, "the other chat's own call to reach Gemini", func() bool { return calls.Load() == 2 })
		open()
		env.settleAI()

		if got := GetRaidAIStats().Checked - before.Checked; got != 1 {
			t.Errorf("Checked grew by %d, want 1: the other chat gets its own verdict, not the failed leader's", got)
		}
		if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
			t.Errorf("verdict items of the first chat = %d, want 0 (its call failed)", got)
		}
	})

	t.Run("no checks during a fire", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		if err := env.redis.Set(raidKey(env.chat.Id, raidPartFire), "1"); err != nil {
			t.Fatalf("set the fire latch: %v", err)
		}
		before := GetRaidAIStats()

		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("fire")})
		env.settleAI()

		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing while a lockdown is being started", got)
		}
		if got := env.geminiRequests(); got != 0 {
			t.Errorf("Gemini requests = %d, want 0", got)
		}
		if got := GetRaidAIStats().Cached - before.Cached; got != 1 {
			t.Errorf("Cached grew by %d, want 1 for the skipped check", got)
		}
	})

	t.Run("Redis refuses", func(t *testing.T) {
		env := newRaidAIImageEnv(t)
		member := env.newMembers(1, 2*time.Hour)[0]
		// A budget key that is not a number makes INCR fail: Redis answers the candidate
		// script and everything before the budget, and refuses only the budget.
		if err := env.redis.Set(raidKey(env.chat.Id, raidPartAIBudget+":"+raidAIHourStamp(raidNow())), "not a number"); err != nil {
			t.Fatalf("poison the budget key: %v", err)
		}

		env.sendMedia(member, gotgbot.Message{Photo: photoNamed("refused")})
		env.settleAI()

		if got := env.geminiRequests(); got != 0 {
			t.Errorf("Gemini requests = %d, want 0: a budget that cannot be counted sends nothing", got)
		}
		if got := env.client.getFiles(); len(got) != 0 {
			t.Errorf("getFile asked for %v, want nothing", got)
		}
	})
}
