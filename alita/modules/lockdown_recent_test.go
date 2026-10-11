//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// recentSnapshot reads the join tracker of the env's chat for the last minute and a
// little into the future, keyed by user ID.
func (e *lockdownEnv) recentSnapshot() map[int64]lockdownRecentJoin {
	e.t.Helper()
	now := time.Now()
	joins, err := lockdownRecentLookup(e.chat.Id, now.Add(-time.Minute), now.Add(time.Minute))
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
