//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestLockdownStartLiftedDuringLock covers a /unlockdown that restores and records the
// lift of the unconfirmed row while the lock call is still on its way: the lock lands
// after the restore, so the group would stay locked with no active lockdown. The start
// must see that its confirmation lost, put the stored permissions back and report the
// lift, never a started lockdown.
func TestLockdownStartLiftedDuringLock(t *testing.T) {
	env := newLockdownEnv(t)

	var once sync.Once
	var hookErr error
	env.fake.setOnSetPermissions(func(chatID int64) {
		once.Do(func() {
			row, err := lockdown.GetActiveFresh(chatID)
			if err != nil || row == nil {
				hookErr = fmt.Errorf("no active row at the lock call: %v", err)
				return
			}
			// The lift restores the stored permissions first, then records itself.
			env.fake.setChatPermsRaw(chatID, row.PrePermissions)
			if won, err := lockdown.BeginLift(row.ID, env.admin.Id, "Admin", false); err != nil || !won {
				hookErr = fmt.Errorf("BeginLift = %v, %v", won, err)
			}
		})
	})

	res := env.startRow(env.chat)
	if hookErr != nil {
		t.Fatal(hookErr)
	}

	wantStartOutcome(t, res, lockdownStartLiftedMeanwhile)
	same, err := samePermissions(env.fake.chatPermsRaw(env.chat.Id), lockdownTestPrePermissions)
	if err != nil {
		t.Fatalf("compare permissions: %v", err)
	}
	if !same {
		t.Errorf("chat permissions = %s, want the stored pre-lockdown set put back", env.fake.chatPermsRaw(env.chat.Id))
	}
	if active, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || active != nil {
		t.Errorf("active lockdown = %v, %v, want none", active, err)
	}
}

// TestLockdownAlertForgedCardPress covers callback data of a confirm card (Confirm or
// Cancel, with the presser's own ID as the asker) sent on a message that is not a card
// of the copy: the alert copy itself, or another message of the Staff Group that is no
// reply to the copy. Callback data is not authenticated, so such a press must be
// answered as no longer valid, edit nothing and lift nothing.
func TestLockdownAlertForgedCardPress(t *testing.T) {
	env := newLockdownEnv(t)
	fix := postLockdownAlert(t, env)
	copies := env.alertRows()
	if len(copies) != 1 {
		t.Fatalf("alert copies = %d, want 1", len(copies))
	}
	copyID := fmt.Sprint(copies[0].ID)
	asker := fmt.Sprint(env.admin.Id)
	expiry := fmt.Sprint(time.Now().Unix()/60 + 5)
	forged := map[string]string{
		"cancel":       encodeCallbackData(lockdownCallbackNamespace, map[string]string{"a": lockdownActCancel, "r": copyID, "u": asker}),
		"lift confirm": encodeCallbackData(lockdownCallbackNamespace, map[string]string{"a": lockdownActLiftConfirm, "r": copyID, "u": asker, "e": expiry}),
	}
	targets := map[string]int64{
		"on the alert copy":            fix.alert.MessageID,
		"on a message that is no card": fix.alert.MessageID + 1000,
	}
	for dataName, data := range forged {
		for targetName, msgID := range targets {
			edits := len(env.staffCalls("editMessageText", fix.staffChat.Id))
			before := env.snapshotAlert(fix)

			pressLockdownButton(env, env.admin, fix.staffChat, msgID, data)

			if got := len(env.staffCalls("editMessageText", fix.staffChat.Id)); got != edits {
				t.Errorf("%s %s: editMessageText calls = %d, were %d: a forged card press edits nothing", dataName, targetName, got, edits)
			}
			env.wantRefused(fix, before, staffMarker("lockdown_alert_expired"))
		}
	}
}

// lockdownUnbanRaceClient is the lockdown fake with one change: the first
// unbanChatMember for user is answered 429 after before ran, so a test can change
// the member while the unban waits for its retry.
type lockdownUnbanRaceClient struct {
	*lockdownFake
	user   int64
	before func()
	once   sync.Once
}

func (c *lockdownUnbanRaceClient) RequestWithContext(
	ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == "unbanChatMember" && staffParamInt(params, "user_id") == c.user {
		raced := false
		c.once.Do(func() {
			raced = true
			c.before()
		})
		if raced {
			return nil, staffFake429(1)
		}
	}
	return c.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
}

// TestLockdownUnbanRereadsAfterWait covers a lift's unban that has to wait (here a
// 429) while an admin bans the joiner on purpose: the live status the unban decided on
// is stale by then, so the read and the unban must be one paced unit and the retry
// must read again and keep the deliberate ban.
func TestLockdownUnbanRereadsAfterWait(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.cycle()
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateBanned {
		t.Fatalf("setup: joiner is %q, want banned", got.State)
	}
	deliberate := time.Now().Add(48 * time.Hour).Unix()
	env.bot.BotClient = &lockdownUnbanRaceClient{
		lockdownFake: env.fake,
		user:         raider.Id,
		before: func() {
			env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
				Status:    gotgbot.ChatMemberStatusKicked,
				UntilDate: deliberate,
			})
		},
	}

	env.send(env.admin, "/unlockdown")
	env.cycle()

	member := env.fake.member(env.chat.Id, raider.Id)
	if member == nil || member.Status != gotgbot.ChatMemberStatusKicked || member.UntilDate != deliberate {
		t.Errorf("raider = %+v, want still banned until the deliberate date", member)
	}
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateKept {
		t.Errorf("joiner state = %q, want kept", got.State)
	}
}

// TestLockdownGuardBansWhenABotAdminAddsSomeone covers a join performed by another
// bot that is an administrator of the group: only a human creator or administrator, or
// the anonymous admin, lets a joiner in during a lockdown, so a bot account (which a
// raider may control) is no gate past it. Both join paths are judged alike.
func TestLockdownGuardBansWhenABotAdminAddsSomeone(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	helper := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "HelperBot", IsBot: true}
	env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
		Status:             gotgbot.ChatMemberStatusAdministrator,
		CanRestrictMembers: true,
	})

	byUpdate := env.newJoiner("Guest")
	env.join(byUpdate, helper, "")
	if row := env.joinerRow(byUpdate.Id); row.State != models.JoinerStatePending {
		t.Errorf("chat_member join: state = %q, want pending: a bot administrator does not exempt", row.State)
	}

	byMessage := env.newJoiner("Guest2")
	env.serviceJoin(helper, nil, byMessage)
	if row := env.joinerRow(byMessage.Id); row.State != models.JoinerStatePending {
		t.Errorf("service join: state = %q, want pending: a bot administrator does not exempt", row.State)
	}
}

// TestLockdownRemindersReachEveryLockdown covers more due lockdowns than one batch of
// the reminder planner: every one of them must get its reminder within a few cycles,
// not only the lowest IDs over and over.
func TestLockdownRemindersReachEveryLockdown(t *testing.T) {
	env := newLockdownEnv(t)
	previous := lockdownReminderBatch
	lockdownReminderBatch = 1
	lockdownReminderCursor.Store(0)
	t.Cleanup(func() {
		lockdownReminderBatch = previous
		lockdownReminderCursor.Store(0)
	})

	second := env.addChat()
	for _, chatID := range []int64{env.chat.Id, second.Id} {
		setStaffLogChannel(t, chatID, uniqueModuleChatID())
		chatID := chatID
		t.Cleanup(func() { db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatLockdownAlert{}) })
	}
	first := env.lockAsAdmin("raid")
	if res := env.startRow(second); res.Outcome != lockdownStartStarted {
		t.Fatalf("second lockdown outcome = %d, want started", res.Outcome)
	}
	env.cycle()
	ids := []uint{first.ID}
	if row, err := lockdown.GetActiveFresh(second.Id); err != nil || row == nil {
		t.Fatalf("second lockdown = %v, %v", row, err)
	} else {
		ids = append(ids, row.ID)
	}
	for _, id := range ids {
		ageAlert(t, id, 61*time.Minute)
	}

	for i := 0; i < 5; i++ {
		lockdownPlanReminders(context.Background(), env.bot)
	}

	for _, id := range ids {
		planned, err := lockdown.ReminderPlanned(id, 1)
		if err != nil || !planned {
			t.Errorf("reminder 1 of lockdown %d planned = %v, %v, want true", id, planned, err)
		}
	}
}

// TestLockdownInlineLiftPostsTally covers a lift with nothing left for the worker
// (here the only joiner's ban is a deliberate one, kept): /unlockdown itself wins
// FinishLift, and the tally must still be posted, exactly once, as the worker posts it
// when it finishes a lift.
func TestLockdownInlineLiftPostsTally(t *testing.T) {
	env := newLockdownEnv(t)
	locked := env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
		Status:    gotgbot.ChatMemberStatusKicked,
		UntilDate: 0,
	})
	env.cycle()
	if got := env.joinerRow(raider.Id); got.State != models.JoinerStateKept {
		t.Fatalf("setup: joiner is %q, want kept", got.State)
	}

	env.send(env.admin, "/unlockdown")
	if row, err := lockdown.GetFresh(locked.ID); err != nil || row == nil || row.State != models.LockdownStateLifted {
		t.Fatalf("lockdown after /unlockdown = %+v, %v, want lifted at once", row, err)
	}
	env.cycle()

	if got := env.messagesWith(staffMarker("lockdown_lift_tally_kept") + " 1"); got != 1 {
		t.Errorf("tally messages naming the one kept ban = %d, want 1", got)
	}
}

// TestLockdownStaleClaimOutlastsAPacedCall holds the stale-claim release above the
// longest one paced call of the worker can take: a slot wait of up to MaxWait and the
// call itself, repeated after every 429 up to MaxRetries times. A shorter one would
// hand a row that a live worker is still banning or unbanning to a second worker.
func TestLockdownStaleClaimOutlastsAPacedCall(t *testing.T) {
	opts := lockdownPacerOptions
	worst := time.Duration(opts.MaxRetries+1) * (opts.MaxWait + lockdownCallTimeout)
	if lockdownStaleClaim <= worst {
		t.Errorf("lockdownStaleClaim = %v, want more than the worst paced call, %v", lockdownStaleClaim, worst)
	}
}

// TestLockdownLiftChecksBansWhoseAnswerFailed covers a ban call that answered a 5xx
// but may have taken effect: the row is left pending with an attempt counted, or ends
// ban_failed after three. The lift must still look at those joiners live and remove
// a ban that carries the row's marker, and leave a joiner whose ban never landed
// out of the tally.
func TestLockdownLiftChecksBansWhoseAnswerFailed(t *testing.T) {
	serverError := staffFakeError(500, "Internal Server Error")
	cases := []struct {
		name      string
		failures  int
		landed    bool
		wantState string
	}{
		{"pending after one failed answer, ban in place", 1, true, models.JoinerStateUnbanned},
		{"ban_failed after three failed answers, ban in place", 3, true, models.JoinerStateUnbanned},
		{"ban_failed and the ban never landed", 3, false, models.JoinerStateCancelled},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.lockAsAdmin("raid")
			raider := env.newJoiner("Raider")
			env.join(raider, raider, "https://t.me/+abc")
			for i := 0; i < tt.failures; i++ {
				env.fake.script("banChatMember", env.chat.Id, serverError)
				env.cycle()
			}
			row := env.joinerRow(raider.Id)
			wantBefore := models.JoinerStatePending
			if tt.failures >= lockdownMaxAttempts {
				wantBefore = models.JoinerStateBanFailed
			}
			if row.State != wantBefore {
				t.Fatalf("setup: joiner is %q, want %q", row.State, wantBefore)
			}
			if tt.landed {
				// The ban took effect although its answer was lost.
				env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{
					Status:    gotgbot.ChatMemberStatusKicked,
					UntilDate: row.BanUntil,
				})
			}

			env.send(env.admin, "/unlockdown")
			env.cycle()

			if got := env.joinerRow(raider.Id); got.State != tt.wantState {
				t.Errorf("joiner state after the lift = %q, want %q", got.State, tt.wantState)
			}
			unbans := env.unbansOf(raider.Id)
			if tt.landed && len(unbans) != 1 {
				t.Errorf("unbanChatMember calls = %d, want 1: the lockdown's ban must be lifted", len(unbans))
			}
			if !tt.landed && len(unbans) != 0 {
				t.Errorf("unbanChatMember calls = %d, want 0: there was no ban to lift", len(unbans))
			}
			if !tt.landed {
				if got := env.messagesWith(staffMarker("lockdown_lift_tally_kept")); got != 0 {
					t.Errorf("tally messages with a kept line = %d, want 0", got)
				}
			}
		})
	}
}
