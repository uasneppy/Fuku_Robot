//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// postedLink is the link stored on the Staff Group's alert copy of the env's chat.
func postedLink(t *testing.T, env *lockdownEnv) string {
	t.Helper()
	var copies []models.ChatLockdownAlert
	if err := db.DB.Where("chat_id = ? AND dest = ?", env.chat.Id, models.AlertDestStaff).Find(&copies).Error; err != nil {
		t.Fatalf("read alert copies: %v", err)
	}
	if len(copies) != 1 {
		t.Fatalf("%d staff alert copies, want 1", len(copies))
	}
	return copies[0].Link
}

func TestLockdownAlertInviteLink(t *testing.T) {
	t.Run("top link", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		for i := 0; i < 3; i++ {
			user := env.newJoiner(fmt.Sprintf("A%d", i))
			env.join(user, user, "https://t.me/+AAAA")
		}
		other := env.newJoiner("B")
		env.join(other, other, "https://t.me/+BBBB")
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_link_line") + " <code>https://t.me/+AAAA</code> 3"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the link line %q", text, want)
		}
		if want := staffMarker("lockdown_alert_link_others") + " 1"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the others line %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_link_foreign")) {
			t.Errorf("alert %q flags a full link as not revocable", text)
		}
		if got := postedLink(t, env); got != "https://t.me/+AAAA" {
			t.Errorf("stored link = %q, want the link the alert shows", got)
		}
	})

	t.Run("joiner links count", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		env.send(env.admin, "/lockdown raid")
		time.Sleep(3 * time.Millisecond)
		first, second := env.newJoiner("Raider1"), env.newJoiner("Raider2")
		env.join(first, first, "https://t.me/+CCCC")
		env.join(second, second, "https://t.me/+CCCC")
		env.cycle()

		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_link_line") + " <code>https://t.me/+CCCC</code> 2"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the link line %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_link_others")) {
			t.Errorf("alert %q has an others line, want none for one link", text)
		}
		if got := postedLink(t, env); got != "https://t.me/+CCCC" {
			t.Errorf("stored link = %q, want the link the alert shows", got)
		}
	})

	t.Run("foreign link", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		user := env.newJoiner("Foreign")
		env.join(user, user, "https://t.me/+DdDd...")
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		line := staffMarker("lockdown_alert_link_line") + " <code>https://t.me/+DdDd...</code> 1"
		foreign := staffMarker("lockdown_alert_link_foreign")
		at, foreignAt := strings.Index(text, line), strings.Index(text, foreign)
		if at < 0 {
			t.Fatalf("alert %q does not contain the link line %q", text, line)
		}
		if foreignAt < at {
			t.Errorf("alert %q must carry %q after the link line", text, foreign)
		}
		if got := postedLink(t, env); got != "https://t.me/+DdDd..." {
			t.Errorf("stored link = %q, want the link the alert shows", got)
		}
	})

	t.Run("revocable", func(t *testing.T) {
		for link, want := range map[string]bool{
			"https://t.me/+DdDd...":     false,
			"https://t.me/+EeEe…":       false,
			"https://t.me/+FullLink1":   true,
			"":                          false,
			"https://t.me/joinchat/Abc": true,
		} {
			if got := lockdownLinkRevocable(link); got != want {
				t.Errorf("lockdownLinkRevocable(%q) = %v, want %v", link, got, want)
			}
		}
	})

	t.Run("escaped", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		user := env.newJoiner("Escaper")
		env.join(user, user, "https://t.me/+a<b>&\"<<lockdown-link>>")
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		if want := "<code>https://t.me/+a&lt;b&gt;&amp;&#34;&lt;&lt;lockdown-link&gt;&gt;</code>"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the escaped link %q", text, want)
		}
		for _, raw := range []string{"+a<b>", "<<lockdown-link>>"} {
			if strings.Contains(text, raw) {
				t.Errorf("alert %q contains the raw link text %q", text, raw)
			}
		}
	})

	t.Run("no link", func(t *testing.T) {
		env := newLockdownEnv(t)
		staffChat := lockdownBurstEnv(env)

		user := env.newJoiner("Plain")
		env.join(user, user, "")
		env.send(env.admin, "/lockdown raid")
		env.cycle()

		text := alertText(t, env, staffChat)
		for _, key := range []string{"lockdown_alert_link_line", "lockdown_alert_link_others", "lockdown_alert_link_foreign"} {
			if strings.Contains(text, staffMarker(key)) {
				t.Errorf("alert %q holds %s although Telegram reported no link", text, key)
			}
		}
		if got := postedLink(t, env); got != "" {
			t.Errorf("stored link = %q, want none", got)
		}
	})
}
