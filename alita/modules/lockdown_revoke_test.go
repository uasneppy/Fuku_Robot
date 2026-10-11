//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// revokeTestLink is a link the bot made and Telegram reported in full.
const revokeTestLink = "https://t.me/+FullLink1"

// revokeScene is a locked group whose alert reached both a Staff Group and a log
// channel, two pre-lockdown joins through one invite link, and a live administrator
// who may invite users but not restrict members.
type revokeScene struct {
	env      *lockdownEnv
	staff    gotgbot.Chat
	channel  int64
	row      *models.ChatLockdown
	staffMsg staffSentMessage
	logMsg   staffSentMessage
	asker    gotgbot.User
}

// newRevokeScene locks a group after two joins through link and lets the worker post the
// alert in both destinations.
func newRevokeScene(t *testing.T, link string) *revokeScene {
	t.Helper()
	env := newLockdownEnv(t)
	staffChat := lockdownBurstEnv(env)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.chat.Id, channel)

	for i := 0; i < 2; i++ {
		joiner := env.newJoiner(fmt.Sprintf("Raider%d", i))
		env.join(joiner, joiner, link)
	}
	row := env.lockAsAdmin("raid")
	env.cycle()

	toStaff, toLog := env.fake.sentTo(staffChat.Id), env.fake.sentTo(channel)
	if len(toStaff) != 1 || len(toLog) != 1 {
		t.Fatalf("%d alerts reached the Staff Group and %d the log channel, want 1 each", len(toStaff), len(toLog))
	}
	asker := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "V<i>"}
	env.fake.setMemberRaw(env.chat.Id, asker.Id, lockdownAdminJSON(asker.Id, false, true))
	return &revokeScene{env: env, staff: staffChat, channel: channel, row: row, staffMsg: toStaff[0], logMsg: toLog[0], asker: asker}
}

// press presses a button of the lockdown| namespace on the Staff Group's alert or card.
func (s *revokeScene) press(from gotgbot.User, msgID int64, data string) {
	s.env.t.Helper()
	pressLockdownButton(s.env, from, s.staff, msgID, data)
}

// ask presses Revoke on the Staff Group's alert and returns the card it posted, or false
// when no card came.
func (s *revokeScene) ask(from gotgbot.User) (staffSentMessage, bool) {
	s.env.t.Helper()
	before := len(s.env.fake.sentTo(s.staff.Id))
	s.press(from, s.staffMsg.MessageID, lockdownButtonData(s.env.t, s.staffMsg.Params["reply_markup"], "rk"))
	sent := s.env.fake.sentTo(s.staff.Id)
	if len(sent) == before {
		return staffSentMessage{}, false
	}
	return sent[len(sent)-1], true
}

// revokeCalls are the revokeChatInviteLink requests sent for the locked group.
func (s *revokeScene) revokeCalls() []moduleBotCall {
	return s.env.calls("revokeChatInviteLink")
}

// cardText is the text of the last edit of the card.
func (s *revokeScene) cardText(card staffSentMessage) string {
	s.env.t.Helper()
	edits := s.env.alertEdits(s.staff.Id, card.MessageID)
	if len(edits) == 0 {
		s.env.t.Fatalf("the card was never edited")
	}
	return fmt.Sprint(edits[len(edits)-1].Params["text"])
}

// wantNoRevokeButton fails when the keyboard of the posted alert, or of the last edit
// when edits exist, holds a Revoke button.
func wantKeyboardWithout(t *testing.T, call moduleBotCall, act string) {
	t.Helper()
	for _, row := range staffKeyboardOf(call.Params["reply_markup"]) {
		for _, button := range row {
			if decoded, ok := decodeCallbackData(button.CallbackData, "lockdown"); ok && decoded.Fields["a"] == act {
				t.Errorf("keyboard %+v still has the %q button", row, act)
			}
		}
	}
}

func TestLockdownRevokeLink(t *testing.T) {
	t.Run("full link", func(t *testing.T) {
		s := newRevokeScene(t, revokeTestLink)
		env := s.env

		// Both copies carry Lift, Ban and, on a third row, Revoke.
		for name, msg := range map[string]staffSentMessage{"staff": s.staffMsg, "log": s.logMsg} {
			keyboard := staffKeyboardOf(msg.Params["reply_markup"])
			if len(keyboard) != 3 || len(keyboard[2]) != 1 {
				t.Fatalf("%s copy keyboard = %+v, want three rows with Revoke alone on the third", name, keyboard)
			}
			if !strings.Contains(keyboard[2][0].Text, staffMarker("lockdown_alert_btn_revoke")) {
				t.Errorf("%s Revoke label = %q, want %q", name, keyboard[2][0].Text, staffMarker("lockdown_alert_btn_revoke"))
			}
			lockdownButtonData(t, msg.Params["reply_markup"], "rk")
		}

		// The asker gets a card with the link, Confirm for them and Cancel.
		card, ok := s.ask(s.asker)
		if !ok {
			t.Fatal("a live administrator with can_invite_users got no confirm card")
		}
		text := fmt.Sprint(card.Params["text"])
		for _, want := range []string{staffMarker("lockdown_revoke_ask"), revokeTestLink} {
			if !strings.Contains(text, want) {
				t.Errorf("card %q does not contain %q", text, want)
			}
		}
		confirmData := lockdownButtonData(t, card.Params["reply_markup"], "rc")
		lockdownButtonData(t, card.Params["reply_markup"], "x")
		confirm, _ := decodeCallbackData(confirmData, "lockdown")
		if confirm.Fields["u"] != fmt.Sprint(s.asker.Id) {
			t.Errorf("Confirm carries asker %q, want %d", confirm.Fields["u"], s.asker.Id)
		}
		if got := len(s.revokeCalls()); got != 0 {
			t.Fatalf("%d revokeChatInviteLink calls before Confirm, want none", got)
		}

		// Confirm revokes the stored link once and records who did.
		s.press(s.asker, card.MessageID, confirmData)
		calls := s.revokeCalls()
		if len(calls) != 1 {
			t.Fatalf("%d revokeChatInviteLink calls, want exactly 1", len(calls))
		}
		if got := fmt.Sprint(calls[0].Params["invite_link"]); got != revokeTestLink {
			t.Errorf("revoked %q, want %q", got, revokeTestLink)
		}
		if got := s.cardText(card); !strings.Contains(got, staffMarker("lockdown_revoke_done")) {
			t.Errorf("card reads %q, want %q", got, staffMarker("lockdown_revoke_done"))
		}
		row := env.freshRow(s.row.ID)
		if row.LinkRevokedAt == nil || row.LinkRevokedBy == nil || *row.LinkRevokedBy != s.asker.Id ||
			row.LinkRevokedByName != "V<i>" || row.LinkRevokedLink != revokeTestLink {
			t.Errorf("lockdown row = %+v, want the revoke recorded for %d, V<i>, %s", row, s.asker.Id, revokeTestLink)
		}

		// The next cycle edits each copy once: body plus the revoked footer, no Revoke button.
		env.cycle()
		for name, msg := range map[string]struct {
			chat int64
			sent staffSentMessage
		}{"staff": {s.staff.Id, s.staffMsg}, "log": {s.channel, s.logMsg}} {
			edits := env.alertEdits(msg.chat, msg.sent.MessageID)
			if len(edits) != 1 {
				t.Fatalf("%s copy was edited %d times, want once", name, len(edits))
			}
			edited := fmt.Sprint(edits[0].Params["text"])
			body := fmt.Sprint(msg.sent.Params["text"])
			if !strings.HasPrefix(edited, body) {
				t.Errorf("%s edit %q does not start with the posted body", name, edited)
			}
			footer := edited[min(len(body), len(edited)):]
			for _, want := range []string{staffMarker("lockdown_alert_link_revoked"), "V&lt;i&gt;"} {
				if !strings.Contains(footer, want) {
					t.Errorf("%s footer %q does not contain %q", name, footer, want)
				}
			}
			lockdownButtonData(t, edits[0].Params["reply_markup"], "lk")
			lockdownButtonData(t, edits[0].Params["reply_markup"], "bn")
			wantKeyboardWithout(t, edits[0], "rk")
		}
		env.cycle()
		if got := len(env.alertEdits(s.staff.Id, s.staffMsg.MessageID)); got != 1 {
			t.Errorf("staff copy was edited %d times after a further cycle, want still 1", got)
		}
		if got := len(env.alertEdits(s.channel, s.logMsg.MessageID)); got != 1 {
			t.Errorf("log copy was edited %d times after a further cycle, want still 1", got)
		}

		// The old Revoke button answers "already revoked" and revokes nothing.
		answers := env.answerTotal()
		s.press(s.asker, s.staffMsg.MessageID, lockdownButtonData(t, s.staffMsg.Params["reply_markup"], "rk"))
		if env.answerTotal() != answers+1 {
			t.Fatalf("an old Revoke press made %d answers, want 1", env.answerTotal()-answers)
		}
		if text, _ := env.lastAnswer(); !strings.Contains(text, staffMarker("lockdown_revoke_already")) {
			t.Errorf("answer = %q, want %q", text, staffMarker("lockdown_revoke_already"))
		}
		if got := len(s.revokeCalls()); got != 1 {
			t.Errorf("%d revokeChatInviteLink calls after an old press, want still 1", got)
		}

		// The lift closes both copies: revoked footer, then lifted footer, no buttons.
		env.send(env.admin, "/unlockdown")
		for i := 0; i < 3; i++ {
			env.cycle()
		}
		for name, msg := range map[string]struct {
			chat int64
			sent staffSentMessage
		}{"staff": {s.staff.Id, s.staffMsg}, "log": {s.channel, s.logMsg}} {
			edits := env.alertEdits(msg.chat, msg.sent.MessageID)
			if len(edits) == 0 {
				t.Fatalf("%s copy was never edited at the lift", name)
			}
			last := edits[len(edits)-1]
			closed := fmt.Sprint(last.Params["text"])
			revokedAt := strings.Index(closed, staffMarker("lockdown_alert_link_revoked"))
			liftedAt := strings.Index(closed, staffMarker("lockdown_alert_lifted_footer"))
			if revokedAt < 0 || liftedAt < revokedAt {
				t.Errorf("%s closed text %q must carry the revoked footer before the lifted footer", name, closed)
			}
			wantNoKeyboard(t, last)
		}
	})

	t.Run("cut short link", func(t *testing.T) {
		s := newRevokeScene(t, "https://t.me/+DdDd...")
		for name, msg := range map[string]staffSentMessage{"staff": s.staffMsg, "log": s.logMsg} {
			if keyboard := staffKeyboardOf(msg.Params["reply_markup"]); len(keyboard) != 2 {
				t.Errorf("%s copy keyboard = %+v, want only the Lift and Ban rows for a link the bot did not make", name, keyboard)
			}
			wantKeyboardWithout(t, moduleBotCall{Params: msg.Params}, "rk")
		}
	})
}
