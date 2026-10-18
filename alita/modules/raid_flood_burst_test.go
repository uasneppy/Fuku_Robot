//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// floodBurstEnv is a ban env (lockdown env, Staff Group, burst and approval cleanup) with
// clean raid settings and env.admin as the chat's creator, so the cached admin list holds
// them. It returns the env and the Staff Group.
func floodBurstEnv(t *testing.T) (*lockdownEnv, gotgbot.Chat) {
	t.Helper()
	env, staffChat := banEnv(t)
	raidCleanup(t, env.chat.Id)
	env.fake.setCreator(env.chat.Id, env.admin.Id)
	return env, staffChat
}

// floodAccount is a flooder of a test: the Telegram user and the name the users table
// holds for it.
type floodAccount struct {
	user     gotgbot.User
	username string
}

// seedFlooders makes n recent joiners with markup in their names, each with a users row
// when withRows is set, and seeds each as having joined age ago. The names are
// "Flood<b>1</b>", "Flood<b>2</b>"..., the usernames "flooder1"...
func (e *lockdownEnv) seedFlooders(n int, age time.Duration, withRows bool) []floodAccount {
	e.t.Helper()
	accounts := make([]floodAccount, 0, n)
	ids := make([]int64, 0, n)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("Flood<b>%d</b>", i)
		username := fmt.Sprintf("flooder%d", i)
		user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: name, Username: username}
		if withRows {
			row := models.User{UserId: user.Id, UserName: username, Name: name, LastActivity: time.Now()}
			if err := db.DB.Create(&row).Error; err != nil {
				e.t.Fatalf("seed the users row of %d: %v", user.Id, err)
			}
		}
		seedRecentJoin(e, user.Id, age)
		accounts = append(accounts, floodAccount{user: user, username: username})
		ids = append(ids, user.Id)
	}
	e.t.Cleanup(func() { db.DB.Where("user_id IN ?", ids).Delete(&models.User{}) })
	return accounts
}

// floodUsers lists the Telegram users of the accounts.
func floodUsers(accounts []floodAccount) []gotgbot.User {
	users := make([]gotgbot.User, 0, len(accounts))
	for _, account := range accounts {
		users = append(users, account.user)
	}
	return users
}

// floodTheChat has an established member post 5 items and the flooders 25 more, which
// is the 30 items in 30 seconds, 83% of them from recent joiners, that lock the chat. It
// returns the established member.
func (e *lockdownEnv) floodTheChat(flooders []floodAccount) gotgbot.User {
	e.t.Helper()
	established := e.newJoiner("Established")
	e.sendItems([]gotgbot.User{established}, 5)
	e.sendItems(floodUsers(flooders), 25)
	return established
}

func TestRaidFloodBurst(t *testing.T) {
	t.Run("flooders listed", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		flooders := env.seedFlooders(4, 3*time.Hour, true)

		established := env.floodTheChat(flooders)

		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerFlood {
			t.Fatalf("active lockdown = %+v, want one started by the flood", row)
		}
		env.cycle()

		text := alertText(t, env, staffChat)
		for i, account := range flooders {
			for _, want := range []string{
				html.EscapeString(fmt.Sprintf("Flood<b>%d</b>", i+1)),
				"@" + account.username,
				fmt.Sprintf("<code>%d</code>", account.user.Id),
			} {
				if !strings.Contains(text, want) {
					t.Errorf("alert %q does not contain %q", text, want)
				}
			}
		}
		if strings.Contains(text, "<b>1</b>") {
			t.Errorf("alert %q holds an unescaped name tag", text)
		}
		if strings.Contains(text, fmt.Sprintf("<code>%d</code>", established.Id)) {
			t.Errorf("alert %q lists the established member %d, who is no flooding account", text, established.Id)
		}
		if want := staffMarker("lockdown_alert_counts_flood") + " 4 0"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the flood counts %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_counts")+" ") {
			t.Errorf("alert %q uses the join counts line, want the flood one", text)
		}

		rows := env.burstRows()
		if len(rows) != 4 {
			t.Fatalf("%d burst rows, want 4", len(rows))
		}
		locked := env.freshRow(row.ID).LockedAt
		if locked == nil {
			t.Fatal("the lockdown has no locked_at")
		}
		for _, burst := range rows {
			if burst.Source != models.BurstSourcePre || burst.JoinedAt == nil {
				t.Errorf("burst row = %+v, want a pre row with a join time", burst)
				continue
			}
			if ago := locked.Sub(*burst.JoinedAt); ago < 2*time.Hour+55*time.Minute || ago > 3*time.Hour+5*time.Minute {
				t.Errorf("burst row of %d joined %s before the lock, want about 3 hours", burst.UserID, ago)
			}
		}
	})
}
