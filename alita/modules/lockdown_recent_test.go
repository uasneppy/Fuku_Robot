//go:build testtools

package modules

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// recentSnapshot reads the join tracker of the env's chat for the last nine minutes
// and a little into the future, keyed by user ID.
func (e *lockdownEnv) recentSnapshot() map[int64]lockdownRecentJoin {
	e.t.Helper()
	now := time.Now()
	joins, err := lockdownRecentLookup(e.chat.Id, now.Add(-9*time.Minute), now.Add(time.Minute))
	if err != nil {
		e.t.Fatalf("lockdownRecentLookup error = %v", err)
	}
	byUser := make(map[int64]lockdownRecentJoin, len(joins))
	for _, join := range joins {
		if _, dup := byUser[join.UserID]; dup {
			e.t.Fatalf("user %d is in the snapshot twice", join.UserID)
		}
		byUser[join.UserID] = join
	}
	return byUser
}

func TestLockdownRecentTracked(t *testing.T) {
	t.Run("chat_member", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		p := env.newJoiner("Pre<i>")
		p.Username = "early"

		env.join(p, p, "https://t.me/+pre")

		got, ok := env.recentSnapshot()[p.Id]
		if !ok {
			t.Fatalf("user %d joined before any lockdown but is not in the snapshot", p.Id)
		}
		if got.Name != "Pre<i>" || got.Username != "early" || got.Link != "https://t.me/+pre" || got.Bot {
			t.Errorf("snapshot entry = %+v, want name Pre<i>, username early, link https://t.me/+pre, not a bot", got)
		}
	})

	t.Run("service message", func(t *testing.T) {
		env := newLockdownEnv(t)
		q := env.newJoiner("Q")
		r := env.newJoiner("Robo")
		r.IsBot = true

		env.serviceJoin(env.admin, nil, q, r)

		snapshot := env.recentSnapshot()
		if len(snapshot) != 2 {
			t.Fatalf("snapshot holds %d users, want 2: %+v", len(snapshot), snapshot)
		}
		if got := snapshot[q.Id]; got.Name != "Q" || got.Bot || got.Link != "" {
			t.Errorf("entry of Q = %+v, want name Q, not a bot, no link", got)
		}
		if got := snapshot[r.Id]; !got.Bot || got.Link != "" {
			t.Errorf("entry of the bot account = %+v, want a bot with no link", got)
		}
	})

	t.Run("bot itself", func(t *testing.T) {
		env := newLockdownEnv(t)
		self := gotgbot.User{Id: env.bot.Id, FirstName: "Me", IsBot: true}

		env.join(self, env.admin, "")
		env.serviceJoin(env.admin, nil, self)

		if snapshot := env.recentSnapshot(); len(snapshot) != 0 {
			t.Errorf("the bot's own join is in the snapshot: %+v", snapshot)
		}
	})

	t.Run("window", func(t *testing.T) {
		env := newLockdownEnv(t)
		p := env.newJoiner("P")

		env.join(p, p, "")

		now := time.Now()
		joins, err := lockdownRecentLookup(env.chat.Id, now.Add(-20*time.Minute), now.Add(-5*time.Minute))
		if err != nil {
			t.Fatalf("lockdownRecentLookup error = %v", err)
		}
		if len(joins) != 0 {
			t.Errorf("a window that ended before the join returned %+v", joins)
		}
	})
}

// trackerMembers returns how many members the env chat's tracker key holds.
func (e *lockdownEnv) trackerMembers() int64 {
	e.t.Helper()
	n, err := cache.GetRedisClient().ZCard(context.Background(), lockdownRecentKey(e.chat.Id)).Result()
	if err != nil {
		e.t.Fatalf("ZCARD of the tracker: %v", err)
	}
	return n
}

func TestLockdownRecentJoins(t *testing.T) {
	t.Run("dedupe", func(t *testing.T) {
		env := newLockdownEnv(t)
		u := env.newJoiner("Dup")

		env.join(u, u, "https://t.me/+link")
		env.serviceJoin(env.admin, nil, u)

		snapshot := env.recentSnapshot()
		if len(snapshot) != 1 {
			t.Fatalf("snapshot holds %d users, want 1: %+v", len(snapshot), snapshot)
		}
		if got := snapshot[u.Id]; got.Link != "https://t.me/+link" || got.Name != "Dup" {
			t.Errorf("merged entry = %+v, want the link of the chat_member delivery and the name", got)
		}
	})

	t.Run("dedupe keeps the earliest time and fills in the link", func(t *testing.T) {
		env := newLockdownEnv(t)
		u := env.newJoiner("Dup")
		first := time.Now().Add(-3 * time.Minute)
		second := time.Now().Add(-time.Minute)

		trackLockdownRecentJoins(env.chat.Id, []lockdownRecentJoin{{UserID: u.Id, Name: "Dup", Link: "https://t.me/+a", At: second}})
		trackLockdownRecentJoins(env.chat.Id, []lockdownRecentJoin{{UserID: u.Id, Name: "Dup", At: first}})

		got, ok := env.recentSnapshot()[u.Id]
		if !ok {
			t.Fatal("user missing from the snapshot")
		}
		if d := got.At.Sub(first); d < -time.Millisecond || d > time.Millisecond {
			t.Errorf("join time = %v, want the earliest one %v", got.At, first)
		}
		if got.Link != "https://t.me/+a" {
			t.Errorf("link = %q, want the one the later delivery carried", got.Link)
		}
	})

	t.Run("trim", func(t *testing.T) {
		env := newLockdownEnv(t)
		old := env.newJoiner("Old")
		fresh := env.newJoiner("Fresh")

		// An entry 11 minutes old, written straight to the key as a quiet group leaves it.
		if err := cache.GetRedisClient().ZAdd(context.Background(), lockdownRecentKey(env.chat.Id), redis.Z{
			Score:  float64(time.Now().Add(-11 * time.Minute).UnixMilli()),
			Member: fmt.Sprintf(`{"i":%d,"n":"Old"}`, old.Id),
		}).Err(); err != nil {
			t.Fatalf("seed old entry: %v", err)
		}

		now := time.Now()
		joins, err := lockdownRecentLookup(env.chat.Id, now.Add(-10*time.Minute), now.Add(time.Minute))
		if err != nil {
			t.Fatalf("lockdownRecentLookup error = %v", err)
		}
		if len(joins) != 0 {
			t.Errorf("a snapshot of the last 10 minutes returned %+v", joins)
		}
		if n := env.trackerMembers(); n != 1 {
			t.Fatalf("tracker holds %d member(s) before the next join, want the seeded one", n)
		}

		env.join(fresh, fresh, "")

		if n := env.trackerMembers(); n != 1 {
			t.Errorf("tracker holds %d member(s) after the next join, want only the fresh one", n)
		}
		snapshot := env.recentSnapshot()
		if _, ok := snapshot[fresh.Id]; !ok {
			t.Error("the fresh join is not in the snapshot")
		}
		if _, ok := snapshot[old.Id]; ok {
			t.Error("the 11-minute-old join is still in the snapshot")
		}
	})

	t.Run("cap", func(t *testing.T) {
		env := newLockdownEnv(t)
		base := time.Now().Add(-5 * time.Minute)
		total := lockdownRecentCap + 20
		for i := range total {
			trackLockdownRecentJoins(env.chat.Id, []lockdownRecentJoin{{
				UserID: int64(1000 + i),
				Name:   "Raider",
				At:     base.Add(time.Duration(i) * time.Millisecond),
			}})
		}

		if n := env.trackerMembers(); n != lockdownRecentCap {
			t.Fatalf("tracker holds %d members after %d joins, want %d", n, total, lockdownRecentCap)
		}
		snapshot := env.recentSnapshot()
		if _, ok := snapshot[int64(1000+total-1)]; !ok {
			t.Error("the newest join was trimmed")
		}
		if _, ok := snapshot[1000]; ok {
			t.Error("the oldest join survived the cap")
		}
	})

	t.Run("cap holds for one batch", func(t *testing.T) {
		env := newLockdownEnv(t)
		base := time.Now().Add(-5 * time.Minute)
		joins := make([]lockdownRecentJoin, 0, lockdownRecentCap+20)
		for i := range lockdownRecentCap + 20 {
			joins = append(joins, lockdownRecentJoin{UserID: int64(5000 + i), At: base.Add(time.Duration(i) * time.Millisecond)})
		}

		trackLockdownRecentJoins(env.chat.Id, joins)

		if n := env.trackerMembers(); n != lockdownRecentCap {
			t.Errorf("tracker holds %d members, want %d", n, lockdownRecentCap)
		}
	})

	t.Run("ttl", func(t *testing.T) {
		env := newLockdownEnv(t)
		u := env.newJoiner("Ttl")

		env.join(u, u, "")

		ttl, err := cache.GetRedisClient().TTL(context.Background(), lockdownRecentKey(env.chat.Id)).Result()
		if err != nil {
			t.Fatalf("TTL: %v", err)
		}
		if ttl <= 0 || ttl > lockdownRecentTTL {
			t.Errorf("tracker TTL = %v, want within (0, %v]", ttl, lockdownRecentTTL)
		}
	})

	t.Run("stored text is capped", func(t *testing.T) {
		env := newLockdownEnv(t)
		long := make([]rune, 400)
		for i := range long {
			long[i] = 'é'
		}

		trackLockdownRecentJoins(env.chat.Id, []lockdownRecentJoin{{
			UserID: 77, Name: string(long), Username: string(long), Link: string(long), At: time.Now(),
		}})

		got := env.recentSnapshot()[77]
		if n := len([]rune(got.Name)); n != lockdownNameMaxRunes {
			t.Errorf("name has %d runes, want %d", n, lockdownNameMaxRunes)
		}
		if n := len([]rune(got.Username)); n != lockdownRecentUsernameMaxRunes {
			t.Errorf("username has %d runes, want %d", n, lockdownRecentUsernameMaxRunes)
		}
		if n := len([]rune(got.Link)); n != lockdownRecentLinkMaxRunes {
			t.Errorf("link has %d runes, want %d", n, lockdownRecentLinkMaxRunes)
		}
	})

	t.Run("unparsable members are skipped", func(t *testing.T) {
		env := newLockdownEnv(t)
		u := env.newJoiner("Ok")
		env.join(u, u, "")
		if err := cache.GetRedisClient().ZAdd(context.Background(), lockdownRecentKey(env.chat.Id), redis.Z{
			Score: float64(time.Now().UnixMilli()), Member: "not json",
		}).Err(); err != nil {
			t.Fatalf("seed junk member: %v", err)
		}

		snapshot := env.recentSnapshot()
		if _, ok := snapshot[u.Id]; !ok || len(snapshot) != 1 {
			t.Errorf("snapshot = %+v, want only the parsable join", snapshot)
		}
	})

	t.Run("no redis", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		restore := cache.DisableRedisForTest()
		defer restore()

		trackLockdownRecentJoins(env.chat.Id, []lockdownRecentJoin{{UserID: 1, At: time.Now()}})
		joins, err := lockdownRecentLookup(env.chat.Id, time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
		if err != nil || len(joins) != 0 {
			t.Errorf("lookup without Redis = %+v, %v, want nothing and no error", joins, err)
		}

		raider := env.newJoiner("Raider")
		env.join(raider, raider, "")
		if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
			t.Errorf("joiner state = %q, want pending", row.State)
		}
		env.cycle()
		if bans := env.bansOf(raider.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want 1: the guard and the worker never need Redis", len(bans))
		}
	})

	t.Run("redis failure", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		// A closed client fails every command, the way a Redis that went away does.
		if err := cache.GetRedisClient().Close(); err != nil {
			t.Fatalf("close redis client: %v", err)
		}

		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+x")
		if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
			t.Errorf("joiner state = %q, want pending: a tracker error must not reach the guard", row.State)
		}

		second := env.newJoiner("Raider2")
		env.serviceJoin(second, nil, second)
		if row := env.joinerRow(second.Id); row.State != models.JoinerStatePending {
			t.Errorf("service-message joiner state = %q, want pending", row.State)
		}

		if _, err := lockdownRecentLookup(env.chat.Id, time.Now().Add(-time.Minute), time.Now().Add(time.Minute)); err == nil {
			t.Error("lookup on a failed Redis returned no error, want the error for the caller to handle")
		}
	})
}
