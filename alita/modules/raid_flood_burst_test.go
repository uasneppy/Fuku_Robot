//go:build testtools

package modules

import (
	"context"
	"fmt"
	"html"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
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

	t.Run("ban the flooders", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		flooders := env.seedFlooders(4, 3*time.Hour, true)
		established := env.floodTheChat(flooders)
		env.cycle()

		a := lastAlert(t, env, staffChat)
		keyboard := staffKeyboardOf(a.alert.Params["reply_markup"])
		if len(keyboard) < 2 || len(keyboard[1]) != 1 {
			t.Fatalf("alert keyboard = %+v, want a second row with the Ban button", keyboard)
		}
		if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 4") {
			t.Errorf("Ban button label = %q, want it to carry %q with 4", label, staffMarker("lockdown_alert_btn_ban"))
		}

		card := env.askBan(a, env.admin)
		if text := fmt.Sprint(card.msg.Params["text"]); !strings.Contains(text, staffMarker("lockdown_ban_ask")+" 4 ") {
			t.Errorf("card text = %q, want %q with 4", text, staffMarker("lockdown_ban_ask"))
		}
		pressLockdownButton(env, env.admin, staffChat, card.msg.MessageID, card.confirm)
		env.cycleUntilIdle()

		for i, account := range flooders {
			env.wantBannedForGood(account.user.Id, fmt.Sprintf("flooder %d", i+1))
			if row := env.burstRowOf(account.user.Id); row.State != models.BurstStateBanned {
				t.Errorf("burst row of flooder %d = %s, want %s", i+1, row.State, models.BurstStateBanned)
			}
		}
		env.wantNotBanned(established.Id, "the established member")
		env.wantNotBanned(staffTestBotID, "the bot itself")
		if runs := env.banRuns(); len(runs) != 1 || runs[0].State != models.BanRunStateDone {
			t.Errorf("ban runs = %+v, want one done run", runs)
		}
	})

	t.Run("surge alert unchanged", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		var reads atomic.Int32
		previous := raidFloodersLookup
		raidFloodersLookup = func(chatID int64) ([]lockdownRecentJoin, error) {
			reads.Add(1)
			return previous(chatID)
		}
		t.Cleanup(func() { raidFloodersLookup = previous })

		env.selfJoins(15)
		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Fatalf("active lockdown = %+v, want one started by the join surge", row)
		}
		env.cycle()

		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_counts") + " 15 "; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the join counts %q", text, want)
		}
		if strings.Contains(text, staffMarker("lockdown_alert_counts_flood")) {
			t.Errorf("alert %q uses the flood counts line for a join surge", text)
		}
		if got := reads.Load(); got != 0 {
			t.Errorf("the flooders snapshot was read %d times for a join surge, want 0", got)
		}
	})

	t.Run("snapshot gone", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		first, second := env.newJoiner("Joiner1"), env.newJoiner("Joiner2")
		env.join(first, first, "")
		env.join(second, second, "")
		flooders := env.seedFlooders(4, 3*time.Hour, true)
		env.floodTheChat(flooders)
		if row := env.activeLockdown(); row == nil || row.TriggerKind != models.LockdownTriggerFlood {
			t.Fatalf("active lockdown = %+v, want one started by the flood", row)
		}

		client := cache.GetRedisClient()
		key := raidKey(env.chat.Id, raidPartFlooders)
		if saved, err := client.ZCard(context.Background(), key).Result(); err != nil || saved != 4 {
			t.Fatalf("the snapshot holds %d accounts (error %v), want 4: the case would prove nothing", saved, err)
		}
		if err := client.Del(context.Background(), key).Err(); err != nil {
			t.Fatalf("delete the snapshot: %v", err)
		}
		env.cycle()

		text := alertText(t, env, staffChat)
		if want := staffMarker("lockdown_alert_counts_flood") + " 2 0"; !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain the counts %q: the tracker's two joins only", text, want)
		}
		for _, joiner := range []gotgbot.User{first, second} {
			if want := fmt.Sprintf("<code>%d</code>", joiner.Id); !strings.Contains(text, want) {
				t.Errorf("alert %q does not list the tracker's joiner %q", text, want)
			}
		}
		for _, account := range flooders {
			if unwanted := fmt.Sprintf("<code>%d</code>", account.user.Id); strings.Contains(text, unwanted) {
				t.Errorf("alert %q lists flooder %d although the snapshot was gone", text, account.user.Id)
			}
		}
		if rows := env.burstRows(); len(rows) != 2 {
			t.Errorf("%d burst rows, want the tracker's 2", len(rows))
		}
	})

	t.Run("names unknown", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		flooders := env.seedFlooders(4, 3*time.Hour, false)
		env.floodTheChat(flooders)
		env.cycle()

		text := alertText(t, env, staffChat)
		for _, account := range flooders {
			if want := fmt.Sprintf("<code>%d</code>", account.user.Id); !strings.Contains(text, want) {
				t.Errorf("alert %q does not list the unnamed flooder %q", text, want)
			}
		}
		if strings.Contains(text, "@flooder") || strings.Contains(text, "Flood") {
			t.Errorf("alert %q shows a name for accounts the users table does not know", text)
		}
		if got := sampleLines(text); got != 4 {
			t.Errorf("%d account lines, want 4\n%s", got, text)
		}
		if rows := env.burstRows(); len(rows) != 4 {
			t.Errorf("%d burst rows, want 4", len(rows))
		}
	})

	t.Run("exempt senders and the bot are not saved", func(t *testing.T) {
		env, staffChat := floodBurstEnv(t)
		flooders := env.seedFlooders(4, 3*time.Hour, true)

		approved := env.newJoiner("Approved")
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, ""); err != nil {
			t.Fatalf("approve %d: %v", approved.Id, err)
		}
		seedRecentJoin(env, approved.Id, 2*time.Hour)
		seedRecentJoin(env, env.admin.Id, time.Hour)
		env.sendItems([]gotgbot.User{approved, env.admin}, 8)
		env.floodTheChat(flooders)
		if row := env.activeLockdown(); row == nil || row.TriggerKind != models.LockdownTriggerFlood {
			t.Fatalf("active lockdown = %+v, want one started by the flood", row)
		}

		// Even a snapshot that held the bot would not put it on the alert.
		err := cache.GetRedisClient().ZAdd(context.Background(), raidKey(env.chat.Id, raidPartFlooders),
			redis.Z{Score: float64(raidNow().Add(-time.Hour).UnixMilli()), Member: fmt.Sprint(staffTestBotID)}).Err()
		if err != nil {
			t.Fatalf("add the bot to the snapshot: %v", err)
		}
		env.cycle()

		rows := env.burstRows()
		if len(rows) != 4 {
			t.Fatalf("%d burst rows, want only the 4 flooders", len(rows))
		}
		for _, row := range rows {
			switch row.UserID {
			case approved.Id, env.admin.Id, staffTestBotID:
				t.Errorf("burst rows hold %d, who is approved, an admin or the bot", row.UserID)
			}
		}
		if want := staffMarker("lockdown_alert_counts_flood") + " 4 0"; !strings.Contains(alertText(t, env, staffChat), want) {
			t.Errorf("alert does not contain the counts %q", want)
		}
	})
}
