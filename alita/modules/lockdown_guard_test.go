//go:build testtools

package modules

import (
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/greetings"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// joinerRows returns every joiner row of the env's chat for userID.
func (e *lockdownEnv) joinerRows(userID int64) []models.LockdownJoiner {
	e.t.Helper()
	var rows []models.LockdownJoiner
	if err := db.DB.Where("chat_id = ? AND user_id = ?", e.chat.Id, userID).Order("id").Find(&rows).Error; err != nil {
		e.t.Fatalf("read joiner rows: %v", err)
	}
	return rows
}

// joinerRow returns the one joiner row of userID, or fails the test.
func (e *lockdownEnv) joinerRow(userID int64) models.LockdownJoiner {
	e.t.Helper()
	rows := e.joinerRows(userID)
	if len(rows) != 1 {
		e.t.Fatalf("joiner rows of user %d = %d, want 1", userID, len(rows))
	}
	return rows[0]
}

// chatSends counts what the bot has posted to the chat that a welcome or a captcha
// challenge would use: messages, photos and restrictions.
func (e *lockdownEnv) chatSends() (messages, photos, restricts int) {
	return len(e.replies()), len(e.calls("sendPhoto")), len(e.calls("restrictChatMember"))
}

// bansOf returns the banChatMember calls for userID in the env's chat.
func (e *lockdownEnv) bansOf(userID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.calls("banChatMember") {
		if staffParamInt(call.Params, "user_id") == userID {
			matched = append(matched, call)
		}
	}
	return matched
}

// unbansOf returns the unbanChatMember calls for userID in the env's chat.
func (e *lockdownEnv) unbansOf(userID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.calls("unbanChatMember") {
		if staffParamInt(call.Params, "user_id") == userID {
			matched = append(matched, call)
		}
	}
	return matched
}

// enableJoinWelcome turns on the welcome message and the captcha of the env's chat.
func (e *lockdownEnv) enableJoinWelcome(withCaptcha bool) {
	e.t.Helper()
	if err := greetings.SetWelcomeToggle(e.chat.Id, true); err != nil {
		e.t.Fatalf("enable welcome: %v", err)
	}
	if withCaptcha {
		if err := captcha.SetCaptchaEnabled(e.chat.Id, true); err != nil {
			e.t.Fatalf("enable captcha: %v", err)
		}
	}
}

func TestLockdownGuardBansChatMemberJoin(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	env.enableJoinWelcome(true)

	// Control: before any lockdown a join gets a welcome or a captcha challenge, so
	// the silence below is the guard's doing and not a quiet fixture.
	control := env.newJoiner("Control")
	env.join(control, control, "https://t.me/+ctl")
	messages, photos, _ := env.chatSends()
	if messages+photos == 0 {
		t.Fatal("control: a join before the lockdown sent no message or photo, so the test proves nothing")
	}

	env.lockAsAdmin("raid")
	messagesBefore, photosBefore, restrictsBefore := env.chatSends()

	raider := gotgbot.User{Id: env.newJoiner("").Id, FirstName: "Ra<i>d", Username: "raider"}
	env.join(raider, raider, "https://t.me/+abc")

	row := env.joinerRow(raider.Id)
	if row.State != models.JoinerStatePending || row.JoinPath != models.JoinPathMember ||
		row.InviteLink != "https://t.me/+abc" || row.FirstName != "Ra<i>d" || row.Username != "raider" ||
		row.PerformerID != raider.Id || row.ChatID != env.chat.Id {
		t.Errorf("joiner row = %+v, want pending, member path, the invite link, the raw name and username, performer = joiner", row)
	}
	if want := time.Now().Add(lockdownBanSpan).Unix(); row.BanUntil < want-5 || row.BanUntil > want+5 {
		t.Errorf("ban_until = %d, want within 5 s of %d (now + 330 days)", row.BanUntil, want)
	}

	messagesAfter, photosAfter, restrictsAfter := env.chatSends()
	if messagesAfter != messagesBefore || photosAfter != photosBefore || restrictsAfter != restrictsBefore {
		t.Errorf("after the guarded join the bot sent %d message(s), %d photo(s), %d restriction(s), want none: no welcome and no captcha",
			messagesAfter-messagesBefore, photosAfter-photosBefore, restrictsAfter-restrictsBefore)
	}
	if attempt, err := captcha.GetCaptchaAttemptIncludingExpired(raider.Id, env.chat.Id); err != nil || attempt != nil {
		t.Errorf("captcha attempt = %v, %v, want none", attempt, err)
	}
	if bans := env.bansOf(raider.Id); len(bans) != 0 {
		t.Errorf("the guard made %d ban call(s), want none: the worker bans, never the handler", len(bans))
	}

	env.cycle()

	bans := env.bansOf(raider.Id)
	if len(bans) != 1 {
		t.Fatalf("banChatMember calls = %d, want exactly 1", len(bans))
	}
	if got := staffParamInt(bans[0].Params, "until_date"); got != row.BanUntil {
		t.Errorf("until_date = %d, want the row's ban_until %d", got, row.BanUntil)
	}
	if banned := env.joinerRow(raider.Id); banned.State != models.JoinerStateBanned {
		t.Errorf("state after the cycle = %q, want banned", banned.State)
	}

	env.cycle()
	if bans := env.bansOf(raider.Id); len(bans) != 1 {
		t.Errorf("banChatMember calls after a second cycle = %d, want still 1", len(bans))
	}
}

func TestLockdownGuardBansAfterManualReopen(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	locks := len(env.calls("setChatPermissions"))

	// An admin reopened the group by hand.
	env.fake.setChatPermsRaw(env.chat.Id, lockdownTestPrePermissions)

	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
		t.Fatalf("state = %q, want pending: a reopened group still records its joiners", row.State)
	}

	env.cycle()

	if bans := env.bansOf(raider.Id); len(bans) != 1 {
		t.Errorf("banChatMember calls = %d, want 1", len(bans))
	}
	if got := len(env.calls("setChatPermissions")); got != locks {
		t.Errorf("setChatPermissions calls = %d after the reopen, want %d: the bot does not re-lock", got, locks)
	}
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStateBanned {
		t.Errorf("state = %q, want banned", row.State)
	}
}

func TestLockdownGuardIgnoresUnlockedChats(t *testing.T) {
	t.Run("no lockdown", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)

		user := env.newJoiner("Plain")
		env.join(user, user, "https://t.me/+abc")

		if rows := env.joinerRows(user.Id); len(rows) != 0 {
			t.Errorf("joiner rows = %d, want none without a lockdown", len(rows))
		}
		if messages, photos, _ := env.chatSends(); messages+photos == 0 {
			t.Error("the welcome did not run for a join in a chat without a lockdown")
		}
	})

	t.Run("lockdown whose lock Telegram never confirmed", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		started, err := lockdown.Start(&models.ChatLockdown{
			ChatID:         env.chat.Id,
			TriggerKind:    models.LockdownTriggerManual,
			StartedBy:      env.admin.Id,
			PrePermissions: lockdownTestPrePermissions,
		})
		if err != nil || !started {
			t.Fatalf("Start = %v, %v", started, err)
		}

		user := env.newJoiner("Plain")
		env.join(user, user, "https://t.me/+abc")

		if rows := env.joinerRows(user.Id); len(rows) != 0 {
			t.Errorf("joiner rows = %d, want none while locked_at is unset", len(rows))
		}
		if messages, photos, _ := env.chatSends(); messages+photos == 0 {
			t.Error("the welcome did not run for a join while the lock was unconfirmed")
		}
	})
}

func TestLockdownWorkerStartStop(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "")

	previousTick := lockdownWorkerTick
	lockdownWorkerTick = 10 * time.Millisecond
	t.Cleanup(func() { lockdownWorkerTick = previousTick })
	t.Cleanup(StopLockdownWorker)

	StartLockdownWorker(env.bot)
	StartLockdownWorker(env.bot)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rows := env.joinerRows(raider.Id); len(rows) == 1 && rows[0].State == models.JoinerStateBanned {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStateBanned {
		t.Fatalf("state = %q after 2 s, want banned by the running worker", row.State)
	}
	if bans := env.bansOf(raider.Id); len(bans) != 1 {
		t.Errorf("banChatMember calls = %d, want exactly 1 even with two Start calls", len(bans))
	}

	started := time.Now()
	StopLockdownWorker()
	StopLockdownWorker()
	if took := time.Since(started); took > lockdownWorkerStopWait+time.Second {
		t.Errorf("StopLockdownWorker took %v, want within %v", took, lockdownWorkerStopWait+time.Second)
	}
}
