//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TestSettingsPageActionValidate drives a test page whose action refuses every part code but
// "x" through the real dispatcher: a forged field set is answered as an invalid request before
// any getChatMember, runs nothing and edits nothing, and an action without Validate keeps its
// old behaviour.
func TestSettingsPageActionValidate(t *testing.T) {
	const strictCode, plainCode, pageKey = "zv", "zw", "zv"
	ran := map[string]int{}
	registerSettingsPage(settingsPage{
		Key:      pageKey,
		Order:    9999,
		TitleKey: "settings_page_captcha",
		Actions: map[string]settingsPageAction{
			strictCode: {
				Fields:   []string{"t"},
				Right:    settingsRightAdmin,
				Validate: func(fields map[string]string) bool { return fields["t"] == "x" },
				Run: func(*settingsPress) *settingsScreen {
					ran[strictCode]++
					return nil
				},
			},
			plainCode: {
				Fields: []string{"t"},
				Right:  settingsRightAdmin,
				Run: func(*settingsPress) *settingsScreen {
					ran[plainCode]++
					return nil
				},
			},
		},
	})
	t.Cleanup(func() {
		delete(settingsPages, pageKey)
		delete(settingsActions, strictCode)
		delete(settingsActions, plainCode)
	})

	env := newSettingsEnv(t)
	msgID := env.openMenu(env.admin)

	t.Run("a field set the action accepts runs it and answers once", func(t *testing.T) {
		answers := env.answerCount()
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", strictCode, "t", "x"))
		if ran[strictCode] != 1 {
			t.Errorf("the action ran %d times, want once", ran[strictCode])
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("the press was answered %d times, want once", got)
		}
	})

	forged := []struct {
		name string
		kv   []string
	}{
		{"a missing part", []string{"a", strictCode}},
		{"a part the page does not have", []string{"a", strictCode, "t", "m"}},
		{"a non-ASCII part", []string{"a", strictCode, "t", "é"}},
		{"an extra key", []string{"a", strictCode, "t", "x", "z", "1"}},
	}
	for _, tc := range forged {
		t.Run("refuses "+tc.name+" before any lookup", func(t *testing.T) {
			ranBefore := ran[strictCode]
			answers := env.answerCount()
			edits := len(env.edits(env.chat.Id, msgID))
			lookups := env.memberLookupsOf(env.chat.Id, env.admin.Id)

			env.press(env.admin, msgID, env.groupFields(env.admin, tc.kv...))

			if got := env.answerCount() - answers; got != 1 {
				t.Fatalf("the press was answered %d times, want once", got)
			}
			if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
				t.Errorf("answer = %q (alert %v), want the alert common_callback_invalid_request", text, alert)
			}
			if ran[strictCode] != ranBefore {
				t.Error("a forged press ran the action")
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != edits {
				t.Errorf("a forged press edited the menu %d times, want none", got-edits)
			}
			if got := env.memberLookupsOf(env.chat.Id, env.admin.Id); got != lookups {
				t.Errorf("a forged press made %d getChatMember calls, want none", got-lookups)
			}
		})
	}

	t.Run("an action with no Validate allows its fields and requires none", func(t *testing.T) {
		for _, kv := range [][]string{{"a", plainCode}, {"a", plainCode, "t", "anything"}} {
			before := ran[plainCode]
			env.press(env.admin, msgID, env.groupFields(env.admin, kv...))
			if ran[plainCode] != before+1 {
				t.Errorf("fields %v did not run the action", kv)
			}
		}
	})
}

// settingsHooksMember is a member record of the given status for the pure right tests.
func settingsHooksMember(status string, canRestrict, canChangeInfo bool) gotgbot.MergedChatMember {
	return gotgbot.MergedChatMember{
		Status:             status,
		CanRestrictMembers: canRestrict,
		CanChangeInfo:      canChangeInfo,
	}
}

// settingsHooksChangeInfoJSON is a getChatMember answer for userID as an administrator whose
// only difference is can_change_info.
func settingsHooksChangeInfoJSON(userID int64, canChangeInfo bool) string {
	return fmt.Sprintf(
		`{"status":"administrator","user":{"id":%d,"is_bot":false,"first_name":"Admin%d"},"can_be_edited":false,`+
			`"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":false,"can_manage_video_chats":false,`+
			`"can_restrict_members":false,"can_promote_members":false,"can_change_info":%t,"can_invite_users":false}`,
		userID, userID, canChangeInfo)
}

// settingsHooksCreatorJSON is a getChatMember answer for userID as the creator.
func settingsHooksCreatorJSON(userID int64) string {
	return fmt.Sprintf(
		`{"status":"creator","user":{"id":%d,"is_bot":false,"first_name":"Owner%d"},"is_anonymous":false}`,
		userID, userID)
}

// TestSettingsRightRefusalForParity pins the member-record part of every right: the three
// rights that existed before answer exactly what they answered, the two bot rights have no
// member part, the ninth right (Phase 19) judges the member as settingsRightRestrict does, and settingsRightRefusal agrees with the pure function wherever it needs no bot.
func TestSettingsRightRefusalForParity(t *testing.T) {
	const (
		notAdmin   = "settings_not_admin"
		noRestrict = "settings_no_restrict"
		ownerOnly  = "settings_access_owner_only"
		noInfo     = "settings_no_change_info"
		ownerBtn   = "chat_status_owner_button_error"
	)
	type verdict struct {
		key   string
		alert bool
	}
	creator := settingsHooksMember(gotgbot.ChatMemberStatusCreator, false, false)
	adminRestrict := settingsHooksMember(gotgbot.ChatMemberStatusAdministrator, true, false)
	adminPlain := settingsHooksMember(gotgbot.ChatMemberStatusAdministrator, false, false)
	plainMember := settingsHooksMember(gotgbot.ChatMemberStatusMember, false, false)
	adminInfo := settingsHooksMember(gotgbot.ChatMemberStatusAdministrator, false, true)
	restricted := settingsHooksMember(gotgbot.ChatMemberStatusRestricted, true, true)
	left := settingsHooksMember(gotgbot.ChatMemberStatusLeft, true, true)
	kicked := settingsHooksMember(gotgbot.ChatMemberStatusKicked, true, true)

	cases := []struct {
		name   string
		right  settingsRight
		member gotgbot.MergedChatMember
		want   verdict
	}{
		{"admin: creator", settingsRightAdmin, creator, verdict{}},
		{"admin: administrator", settingsRightAdmin, adminPlain, verdict{}},
		{"restrict: creator", settingsRightRestrict, creator, verdict{}},
		{"restrict: administrator with the right", settingsRightRestrict, adminRestrict, verdict{}},
		{"restrict: administrator without it", settingsRightRestrict, adminPlain, verdict{noRestrict, true}},
		{"restrict: member", settingsRightRestrict, plainMember, verdict{notAdmin, true}},
		{"restrict: restricted", settingsRightRestrict, restricted, verdict{notAdmin, true}},
		{"restrict: left", settingsRightRestrict, left, verdict{notAdmin, true}},
		{"restrict: kicked", settingsRightRestrict, kicked, verdict{notAdmin, true}},
		{"owner: creator", settingsRightOwner, creator, verdict{}},
		{"owner: administrator", settingsRightOwner, adminRestrict, verdict{ownerOnly, true}},
		{"owner: member", settingsRightOwner, plainMember, verdict{ownerOnly, true}},
		{"change info and bot: creator", settingsRightChangeInfoBotAdmin, creator, verdict{}},
		{"change info and bot: administrator with the right", settingsRightChangeInfoBotAdmin, adminInfo, verdict{}},
		{"change info and bot: administrator without it", settingsRightChangeInfoBotAdmin, adminPlain, verdict{noInfo, true}},
		{"change info and bot: member", settingsRightChangeInfoBotAdmin, plainMember, verdict{notAdmin, true}},
		{"change info and bot: restricted", settingsRightChangeInfoBotAdmin, restricted, verdict{notAdmin, true}},
		{"change info and bot: left", settingsRightChangeInfoBotAdmin, left, verdict{notAdmin, true}},
		{"change info and bot: kicked", settingsRightChangeInfoBotAdmin, kicked, verdict{notAdmin, true}},
		{"owner and bot: creator", settingsRightOwnerBotAdmin, creator, verdict{}},
		{"owner and bot: administrator with every right", settingsRightOwnerBotAdmin, adminInfo, verdict{ownerBtn, true}},
		{"owner and bot: administrator", settingsRightOwnerBotAdmin, adminRestrict, verdict{ownerBtn, true}},
		{"owner and bot: member", settingsRightOwnerBotAdmin, plainMember, verdict{ownerBtn, true}},
		{"owner and bot: restricted", settingsRightOwnerBotAdmin, restricted, verdict{ownerBtn, true}},
		{"owner and bot: left", settingsRightOwnerBotAdmin, left, verdict{ownerBtn, true}},
		{"owner and bot: kicked", settingsRightOwnerBotAdmin, kicked, verdict{ownerBtn, true}},
		{"restrict and bot: creator", settingsRightRestrictBotRestrict, creator, verdict{}},
		{"restrict and bot: administrator with the right", settingsRightRestrictBotRestrict, adminRestrict, verdict{}},
		{"restrict and bot: administrator without it", settingsRightRestrictBotRestrict, adminPlain, verdict{noRestrict, true}},
		{"restrict and bot: member", settingsRightRestrictBotRestrict, plainMember, verdict{notAdmin, true}},
		{"restrict and bot: restricted", settingsRightRestrictBotRestrict, restricted, verdict{notAdmin, true}},
		{"restrict and bot: left", settingsRightRestrictBotRestrict, left, verdict{notAdmin, true}},
		{"restrict and bot: kicked", settingsRightRestrictBotRestrict, kicked, verdict{notAdmin, true}},
		{"bot admin right has no member part", settingsRightAdminBotAdmin, plainMember, verdict{}},
		{"flood bot right has no member part", settingsRightAdminFloodBot, plainMember, verdict{}},
		{"an unknown right fails closed", settingsRight(99), creator, verdict{notAdmin, true}},
	}

	env := newSettingsEnv(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, alert := settingsRightRefusalFor(tc.member, tc.right)
			if (verdict{key, alert}) != tc.want {
				t.Errorf("settingsRightRefusalFor = (%q, %v), want (%q, %v)", key, alert, tc.want.key, tc.want.alert)
			}
			// The press judges the member part through the same function. The bot rights and
			// the unknown one are left out: the former need a bot lookup of their own, and so
			// do the two composite rights, whose bot part is judged first (TestSettingsRights).
			switch tc.right {
			case settingsRightAdminBotAdmin, settingsRightAdminFloodBot, settingsRightChangeInfoBotAdmin, settingsRightOwnerBotAdmin, settingsRightRestrictBotRestrict, settingsRight(99):
				return
			}
			press := &settingsPress{b: env.bot, group: &env.chat, member: tc.member}
			if pk, pa := settingsRightRefusal(press, tc.right); (verdict{pk, pa}) != tc.want {
				t.Errorf("settingsRightRefusal = (%q, %v), want (%q, %v)", pk, pa, tc.want.key, tc.want.alert)
			}
		})
	}

	t.Run("the zero right is still the plain administrator right", func(t *testing.T) {
		if settingsRight(0) != settingsRightAdmin {
			t.Error("the zero settingsRight is not settingsRightAdmin")
		}
	})
}

// TestSettingsRightChangeInfo covers the new right: the pure judgement of every status, and a
// press of a test page action that declares it, judged on the presser's own live record.
func TestSettingsRightChangeInfo(t *testing.T) {
	t.Run("the pure judgement", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			member    gotgbot.MergedChatMember
			wantKey   string
			wantAlert bool
		}{
			{"creator", settingsHooksMember(gotgbot.ChatMemberStatusCreator, false, false), "", false},
			{"administrator with can_change_info", settingsHooksMember(gotgbot.ChatMemberStatusAdministrator, false, true), "", false},
			{"administrator without it", settingsHooksMember(gotgbot.ChatMemberStatusAdministrator, true, false), "settings_no_change_info", true},
			{"member", settingsHooksMember(gotgbot.ChatMemberStatusMember, true, true), "settings_not_admin", true},
			{"restricted", settingsHooksMember(gotgbot.ChatMemberStatusRestricted, true, true), "settings_not_admin", true},
			{"left", settingsHooksMember(gotgbot.ChatMemberStatusLeft, true, true), "settings_not_admin", true},
			{"kicked", settingsHooksMember(gotgbot.ChatMemberStatusKicked, true, true), "settings_not_admin", true},
		} {
			key, alert := settingsRightRefusalFor(tc.member, settingsRightChangeInfo)
			if key != tc.wantKey || alert != tc.wantAlert {
				t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, key, alert, tc.wantKey, tc.wantAlert)
			}
		}
	})

	const code, pageKey = "zr", "zr"
	ran := 0
	registerSettingsPage(settingsPage{
		Key:      pageKey,
		Order:    9999,
		TitleKey: "settings_page_captcha",
		Actions: map[string]settingsPageAction{
			code: {Right: settingsRightChangeInfo, Run: func(*settingsPress) *settingsScreen {
				ran++
				return nil
			}},
		},
	})
	t.Cleanup(func() {
		delete(settingsPages, pageKey)
		delete(settingsActions, code)
	})

	env := newSettingsEnv(t)
	msgID := env.openMenu(env.admin)

	t.Run("an administrator without can_change_info is refused with the alert and nothing runs", func(t *testing.T) {
		env.fake.setMemberRaw(env.chat.Id, env.admin.Id, settingsHooksChangeInfoJSON(env.admin.Id, false))
		before, answers, edits := ran, env.answerCount(), len(env.edits(env.chat.Id, msgID))

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", code))

		if got := env.answerCount() - answers; got != 1 {
			t.Fatalf("answered %d times, want once", got)
		}
		if text, alert := env.lastAnswer(); !alert || !strings.Contains(text, staffMarker("settings_no_change_info")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_no_change_info", text, alert)
		}
		if ran != before {
			t.Error("the action ran for an administrator without can_change_info")
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Errorf("the refused press edited the menu %d times, want none", got-edits)
		}
	})

	t.Run("an administrator with can_change_info runs it", func(t *testing.T) {
		env.fake.setMemberRaw(env.chat.Id, env.admin.Id, settingsHooksChangeInfoJSON(env.admin.Id, true))
		before := ran
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", code))
		if ran != before+1 {
			t.Errorf("the action ran %d times, want once", ran-before)
		}
	})

	t.Run("the creator runs it", func(t *testing.T) {
		env.fake.setMemberRaw(env.chat.Id, env.admin.Id, settingsHooksCreatorJSON(env.admin.Id))
		before := ran
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", code))
		if ran != before+1 {
			t.Errorf("the action ran %d times, want once", ran-before)
		}
	})
}
