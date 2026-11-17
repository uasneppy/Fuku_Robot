//go:build testtools

package modules

import (
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestCaptchaPendingLetsJoinServiceMessageThrough proves, through the real dispatcher, that group
// -10 never holds a pending member's own service messages (audit item 30): the joiner's
// new_chat_members message and their left_chat_member message are not stored, not deleted by the
// captcha hold and reach later groups (here a probe at -7, where the lockdown join guard runs),
// while the same member's ordinary text is still stored, deleted and kept from later groups.
func TestCaptchaPendingLetsJoinServiceMessageThrough(t *testing.T) {
	env := newMemberDeletionEnv(t)
	env.dispatcher.AddHandlerToGroup(handlers.NewMessage(nil, captchaModule.handlePendingCaptchaMessage), -10)
	selfDestructCaptchaCleanup(t, env.chat.Id)

	var mu sync.Mutex
	seen := map[int64]bool{}
	env.dispatcher.AddHandlerToGroup(handlers.NewMessage(nil, func(_ *gotgbot.Bot, ctx *ext.Context) error {
		mu.Lock()
		seen[ctx.EffectiveMessage.MessageId] = true
		mu.Unlock()
		return ext.ContinueGroups
	}), -7)
	probed := func(id int64) bool {
		mu.Lock()
		defer mu.Unlock()
		return seen[id]
	}

	if err := captcha.SetCaptchaEnabled(env.chat.Id, true); err != nil {
		t.Fatalf("SetCaptchaEnabled error = %v", err)
	}
	member := env.member()
	_, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "answer", 5)
	if err != nil {
		t.Fatalf("CreateCaptchaAttemptPreMessage error = %v", err)
	}
	// Counted by member and chat, not by attempt: the join message reaches the join path of group 0
	// in this fixture (no dedupe claim was taken), which replaces the attempt with a fresh one.
	stored := func() int64 {
		t.Helper()
		var count int64
		if err := db.DB.Model(&models.StoredMessages{}).Where("user_id = ? AND chat_id = ?", member.Id, env.chat.Id).Count(&count).Error; err != nil {
			t.Fatalf("count stored messages: %v", err)
		}
		return count
	}

	for _, tc := range []struct {
		name  string
		apply func(*gotgbot.Message)
	}{
		{name: "own join message", apply: func(m *gotgbot.Message) { m.NewChatMembers = []gotgbot.User{member} }},
		{name: "own leave message", apply: func(m *gotgbot.Message) { m.LeftChatMember = &member }},
	} {
		id := env.sendMember(&member, tc.apply)
		if !probed(id) {
			t.Errorf("%s: group -7 did not see the message, group -10 ended the update", tc.name)
		}
		if got := env.deletedMessage(env.chat.Id, id); got != 0 {
			t.Errorf("%s: deletes = %d, want none", tc.name, got)
		}
		if got := stored(); got != 0 {
			t.Errorf("%s: stored messages = %d, want none", tc.name, got)
		}
	}

	// Control: an ordinary message of the same pending member is still held.
	id := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "hello" })
	if probed(id) {
		t.Error("control: group -7 saw an ordinary message of a pending member")
	}
	if got := env.deletedMessage(env.chat.Id, id); got != 1 {
		t.Errorf("control: deletes = %d, want 1", got)
	}
	if got := stored(); got != 1 {
		t.Errorf("control: stored messages = %d, want 1", got)
	}
}
