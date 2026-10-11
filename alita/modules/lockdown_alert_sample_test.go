//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// lockdownBurstEnv is linkLockdownStaffGroup plus the cleanup of the env's chat's burst
// rows, and returns the Staff Group.
func lockdownBurstEnv(env *lockdownEnv) gotgbot.Chat {
	env.t.Helper()
	staffChat := linkLockdownStaffGroup(env)
	env.t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownBurst{})
	})
	return staffChat
}

// burstRows reads every burst row of the env's chat, oldest first.
func (e *lockdownEnv) burstRows() []models.ChatLockdownBurst {
	e.t.Helper()
	var rows []models.ChatLockdownBurst
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Order("id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read burst rows: %v", err)
	}
	return rows
}

// alertText is the text of the one alert the Staff Group received.
func alertText(t *testing.T, env *lockdownEnv, staffChat gotgbot.Chat) string {
	t.Helper()
	sent := env.fake.sentTo(staffChat.Id)
	if len(sent) != 1 {
		t.Fatalf("%d messages reached the Staff Group, want exactly 1", len(sent))
	}
	return fmt.Sprint(sent[0].Params["text"])
}

// sampleLineOf is the alert line that holds needle, or "" when no line does.
func sampleLineOf(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// sampleLines counts the account lines of an alert: the lines that start with a bullet
// and name an ID in code tags.
func sampleLines(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "• ") && strings.Contains(line, "(<code>") {
			count++
		}
	}
	return count
}

func TestLockdownAlertSample(t *testing.T) {
	t.Run("pre-lockdown joiner listed", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		early := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Pre<i>", Username: "early"}
		env.join(early, early, "https://t.me/+pre")

		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		for _, want := range []string{
			"Pre&lt;i&gt;",
			"@early",
			fmt.Sprintf("<code>%d</code>", early.Id),
			staffMarker("lockdown_alert_counts"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("alert %q does not contain %q", text, want)
			}
		}

		rows := env.burstRows()
		if len(rows) != 1 {
			t.Fatalf("%d burst rows, want 1", len(rows))
		}
		row := rows[0]
		if row.UserID != early.Id || row.Source != models.BurstSourcePre || row.InviteLink != "https://t.me/+pre" {
			t.Errorf("burst row = %+v, want user %d, source pre and the invite link", row, early.Id)
		}
		if row.FirstName != "Pre<i>" || row.Username != "early" || row.JoinedAt == nil {
			t.Errorf("burst row = %+v, want the stored name, username and join time", row)
		}
	})

	t.Run("joiners after pre", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		early := env.newJoiner("Early")
		env.join(early, early, "")
		env.send(env.admin, "/lockdown raid")
		time.Sleep(3 * time.Millisecond)
		first, second := env.newJoiner("Raider1"), env.newJoiner("Raider2")
		env.join(first, first, "")
		env.join(second, second, "")
		env.cycle()

		text := alertText(t, env, staffChat)
		positions := make([]int, 0, 3)
		for _, user := range []gotgbot.User{early, first, second} {
			at := strings.Index(text, fmt.Sprintf("<code>%d</code>", user.Id))
			if at < 0 {
				t.Fatalf("alert %q does not list user %d", text, user.Id)
			}
			positions = append(positions, at)
		}
		if !(positions[0] < positions[1] && positions[1] < positions[2]) {
			t.Errorf("positions %v, want the pre-lockdown joiner first, then the raiders in order\n%s", positions, text)
		}
		if want := staffMarker("lockdown_alert_counts") + " 1 2"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_sample_more")) {
			t.Errorf("alert %q has an \"and more\" line, want none for 3 accounts", text)
		}
	})

	t.Run("more than ten", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		for i := 0; i < 12; i++ {
			user := env.newJoiner(fmt.Sprintf("Early%d", i))
			env.join(user, user, "")
		}
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		if got := sampleLines(text); got != 10 {
			t.Errorf("%d account lines, want exactly 10\n%s", got, text)
		}
		if want := staffMarker("lockdown_alert_sample_more") + " 2"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain %q", text, want)
		}
		if want := staffMarker("lockdown_alert_counts") + " 12 0"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q", text, want)
		}
		if len(text) > 4096 {
			t.Errorf("alert is %d characters, want under Telegram's 4096", len(text))
		}
		if rows := env.burstRows(); len(rows) != 12 {
			t.Errorf("%d burst rows saved, want all 12 and not only the 10 shown", len(rows))
		}
	})

	t.Run("bot marked", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		bot := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Spambot", IsBot: true}
		human := env.newJoiner("Human")
		env.join(bot, human, "")
		env.join(human, human, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		botLine := sampleLineOf(text, fmt.Sprintf("<code>%d</code>", bot.Id))
		humanLine := sampleLineOf(text, fmt.Sprintf("<code>%d</code>", human.Id))
		if botLine == "" || humanLine == "" {
			t.Fatalf("alert %q must list both accounts", text)
		}
		if !strings.Contains(botLine, staffMarker("lockdown_alert_bot_mark")) {
			t.Errorf("bot line %q has no bot mark", botLine)
		}
		if strings.Contains(humanLine, staffMarker("lockdown_alert_bot_mark")) {
			t.Errorf("human line %q has a bot mark", humanLine)
		}
	})

	t.Run("escaped", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		evil := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "<b>x</b>", Username: "a<i>&b"}
		env.join(evil, evil, "")
		env.send(env.admin, "/lockdown raid")
		raider := gotgbot.User{
			Id:        uniqueLinkOwnerID(),
			FirstName: "<<lockdown-group>><a href=\"tg://user?id=1\">z</a>",
			Username:  "<<lockdown-sample>>",
		}
		time.Sleep(3 * time.Millisecond)
		env.join(raider, raider, "")
		env.cycle()

		text := alertText(t, env, staffChat)
		for _, raw := range []string{"<b>x</b>", "a<i>&b", "<a href=\"tg://user?id=1\">z</a>"} {
			if strings.Contains(text, raw) {
				t.Errorf("alert %q contains the raw text %q", text, raw)
			}
		}
		for _, want := range []string{
			"&lt;b&gt;x&lt;/b&gt;",
			"@a&lt;i&gt;&amp;b",
			"&lt;&lt;lockdown-group&gt;&gt;&lt;a href=&#34;tg://user?id=1&#34;&gt;z&lt;/a&gt;",
			"@&lt;&lt;lockdown-sample&gt;&gt;",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("alert %q does not contain the escaped text %q", text, want)
			}
		}
		if strings.Contains(text, lockdownSampleToken) || strings.Contains(text, lockdownGroupToken) {
			t.Errorf("alert %q still holds a splice token", text)
		}
	})

	t.Run("no redis", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		restore := cache.DisableRedisForTest()
		defer restore()
		early := env.newJoiner("Early")
		env.join(early, early, "")
		env.send(env.admin, "/lockdown raid")
		raider := env.newJoiner("Raider")
		time.Sleep(3 * time.Millisecond)
		env.join(raider, raider, "")
		env.cycle()

		text := alertText(t, env, staffChat)
		if !strings.Contains(text, fmt.Sprintf("<code>%d</code>", raider.Id)) {
			t.Errorf("alert %q does not list the recorded joiner", text)
		}
		if strings.Contains(text, fmt.Sprintf("<code>%d</code>", early.Id)) {
			t.Errorf("alert %q lists a pre-lockdown joiner that nothing recorded", text)
		}
		if want := staffMarker("lockdown_alert_counts") + " 0 1"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q", text, want)
		}
		if rows := env.burstRows(); len(rows) != 0 {
			t.Errorf("%d burst rows, want none without the tracker", len(rows))
		}
	})

	t.Run("lookup fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		previous := lockdownRecentLookup
		lockdownRecentLookup = func(int64, time.Time, time.Time) ([]lockdownRecentJoin, error) {
			return nil, errors.New("redis went away")
		}
		t.Cleanup(func() { lockdownRecentLookup = previous })

		early := env.newJoiner("Early")
		env.join(early, early, "")
		env.send(env.admin, "/lockdown raid")
		raider := env.newJoiner("Raider")
		time.Sleep(3 * time.Millisecond)
		env.join(raider, raider, "")
		env.cycle()

		text := alertText(t, env, staffChat)
		if !strings.Contains(text, fmt.Sprintf("<code>%d</code>", raider.Id)) {
			t.Errorf("alert %q does not list the recorded joiner", text)
		}
		if strings.Contains(text, fmt.Sprintf("<code>%d</code>", early.Id)) {
			t.Errorf("alert %q lists a pre-lockdown joiner although the tracker read failed", text)
		}
		if want := staffMarker("lockdown_alert_counts") + " 0 1"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q", text, want)
		}
	})
}
