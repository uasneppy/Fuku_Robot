//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// resetSettingsOwnerMemo clears the owner-state memo before and after a test, because the
// memo is package state shared by every test of the package.
func resetSettingsOwnerMemo(t *testing.T) {
	t.Helper()
	clear := func() {
		settingsOwnerMemoMu.Lock()
		defer settingsOwnerMemoMu.Unlock()
		for chatID := range settingsOwnerMemo {
			delete(settingsOwnerMemo, chatID)
		}
	}
	clear()
	t.Cleanup(clear)
}

// settingsAdminListCalls counts the getChatAdministrators calls the env's fake saw for chatID.
func settingsAdminListCalls(env *settingsEnv, chatID int64) int {
	return len(callsToChat(env.fake.staffBotClient, "getChatAdministrators", chatID))
}

// settingsDropAccessRow deletes the chat's switch row, so the chat has no row at all.
func settingsDropAccessRow(t *testing.T, chatID int64) {
	t.Helper()
	if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatSettingsAccess{}).Error; err != nil {
		t.Fatalf("delete chat_settings_access of %d: %v", chatID, err)
	}
}

// settingsReplyHasNoSettingsText reports whether none of the bot's messages to the chat carries
// any settings_ marker.
func settingsReplyHasNoSettingsText(env *settingsEnv) bool {
	for _, sent := range env.replies() {
		if strings.Contains(fmt.Sprint(sent.Params["text"]), "@@settings-") {
			return false
		}
	}
	return true
}

func TestSettingsInGroupRefusal(t *testing.T) {
	t.Run("the owner gets the choice message with no administrator list call", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.send(owner, "/settings")

		if _, button := env.newestChoice(); button.CallbackData == "" {
			t.Fatal("the owner got no choice message")
		}
		if got := settingsAdminListCalls(env, env.chat.Id); got != 0 {
			t.Errorf("getChatAdministrators was called %d times for the owner, want 0", got)
		}
	})

	t.Run("an administrator who is not the owner is refused while the switch is off", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.send(env.admin, "/settings")

		env.wantReplyHas(staffMarker("settings_refuse_owner_only"))
		sent := env.replies()
		if markup := staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"]); len(markup) != 0 {
			t.Errorf("the refusal carries the keyboard %+v, want a plain reply", markup)
		}
		for _, reply := range sent {
			if strings.Contains(fmt.Sprint(reply.Params["text"]), staffMarker("settings_choose")) {
				t.Error("the refused administrator got the choice message")
			}
		}
	})

	t.Run("a group with no switch row is owner only", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		settingsDropAccessRow(t, env.chat.Id)

		env.send(env.admin, "/settings")

		env.wantReplyHas(staffMarker("settings_refuse_owner_only"))
	})

	t.Run("an administrator the owner let in gets the choice message with no list call", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, true)

		env.send(env.admin, "/settings")

		if _, button := env.newestChoice(); button.CallbackData == "" {
			t.Fatal("the allowed administrator got no choice message")
		}
		if got := settingsAdminListCalls(env, env.chat.Id); got != 0 {
			t.Errorf("getChatAdministrators was called %d times with the switch on, want 0", got)
		}
	})

	t.Run("a group with no creator entry lets a listed administrator in", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.fake.clearCreator(env.chat.Id)
		env.listAdmin(env.chat.Id, env.admin.Id)
		env.setAdminsAllowed(env.chat.Id, false)

		env.send(env.admin, "/settings")

		if _, button := env.newestChoice(); button.CallbackData == "" {
			t.Fatal("an administrator of an ownerless group got no choice message (D-15)")
		}
	})

	t.Run("a failing administrator list refuses with settings_check_failed", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		env.fake.setFailure("getChatAdministrators", env.chat.Id, errors.New("telegram is down"))

		env.send(env.admin, "/settings")

		env.wantReplyHas(staffMarker("settings_check_failed"))
		if !strings.Contains(env.lastReply(), staffMarker("settings_check_failed")) ||
			strings.Contains(env.lastReply(), staffMarker("settings_refuse_owner_only")) {
			t.Errorf("reply %q must be the check-failed text and never the owner-only one", env.lastReply())
		}
	})

	t.Run("a plain member gets the existing refusal and no settings text", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		plain := gotgbot.User{Id: env.admin.Id + 41, FirstName: "Plain"}
		env.fake.setMember(env.chat.Id, plain.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(plain, "/settings")

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		if !settingsReplyHasNoSettingsText(env) {
			t.Error("a plain member was answered with a settings_ text")
		}
		if got := settingsAdminListCalls(env, env.chat.Id); got != 0 {
			t.Errorf("getChatAdministrators was called %d times for a plain member, want 0", got)
		}
	})
}

func TestSettingsCommandsUngated(t *testing.T) {
	// The documented authority-parity carve-out of D-19: the menu gate refuses this administrator,
	// the setting commands in the group do not.
	resetSettingsOwnerMemo(t)
	settingsCaptchaNoWeb(t)
	env := newSettingsEnv(t)
	env.newOwner(env.chat)
	env.setAdminsAllowed(env.chat.Id, false)

	env.send(env.admin, "/settings")
	env.wantReplyHas(staffMarker("settings_refuse_owner_only"))

	env.send(env.admin, "/captchamode text")
	if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.CaptchaMode != "text" {
		t.Errorf("/captchamode text stored %+v (row %v) for the gated administrator, want text", row, ok)
	}
	env.send(env.admin, "/setflood 7")
	if row, ok := settingsAntifloodRow(t, env.chat.Id); !ok || row.Limit != 7 {
		t.Errorf("/setflood 7 stored %+v (row %v) for the gated administrator, want limit 7", row, ok)
	}
	env.send(env.admin, "/autolockdown")
	env.wantReplyHas(staffMarker("lockdown_auto_title"))
}

func TestSettingsAnonymousOwnerGate(t *testing.T) {
	t.Run("an anonymous owner gets the proof and then the choice message", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.sendAnonymous("/settings")
		env.wantProofPrompt()
		if _, _, buttons := env.proofButton(); buttons != 1 {
			t.Fatalf("proof prompt has %d buttons, want 1", buttons)
		}
		for _, sent := range env.replies() {
			if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("settings_choose")) {
				t.Fatal("an anonymous admin got the choice message before the proof")
			}
		}

		env.tapProof(owner)

		msgID, here := env.newestChoice()
		if fields := settingsButtonFields(t, here); fields["u"] != fmt.Sprint(owner.Id) {
			t.Errorf("Open here carries u = %q, want the tapper %d (msg %d)", fields["u"], owner.Id, msgID)
		}
	})

	t.Run("an anonymous gated administrator is refused after the proof", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)

		env.sendAnonymous("/settings")
		env.wantProofPrompt()

		env.tapProof(env.admin)

		env.wantReplyHas(staffMarker("settings_refuse_owner_only"))
		for _, sent := range env.replies() {
			if strings.Contains(fmt.Sprint(sent.Params["text"]), staffMarker("settings_choose")) {
				t.Error("the gated administrator got the choice message through the anonymous re-entry")
			}
		}
	})
}
