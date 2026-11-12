//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
		// The bot is a plain member here, so the roster repair that follows a group /settings
		// stops after its bot lookup and makes no administrator list call: any list call this
		// test counts is the gate's own.
		env.fake.setBotRole(env.chat.Id, staffRoleMember)

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
		// The bot is a plain member here, so the roster repair that follows a group /settings
		// stops after its bot lookup and makes no administrator list call: any list call this
		// test counts is the gate's own.
		env.fake.setBotRole(env.chat.Id, staffRoleMember)

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

// gateTestUsers are the people of a crafted administrator list.
const (
	gateTestPresser int64 = 4_100_001
	gateTestOwner   int64 = 4_100_002
)

// gateTestChatSeq makes every crafted-list test use its own chat ID, so the memo of one never
// answers for another even when a test forgets to reset it.
var gateTestChatSeq atomic.Int64

func gateTestChatID() int64 { return -1_004_200_000_000 - gateTestChatSeq.Add(1) }

// gateTestMember is a crafted getChatMember record.
func gateTestMember(id int64, status string) gotgbot.MergedChatMember {
	return gotgbot.MergedChatMember{Status: status, User: gotgbot.User{Id: id, FirstName: fmt.Sprintf("User%d", id)}}
}

// gateTestOwnerEntry is a crafted creator entry of an administrator list.
func gateTestOwnerEntry(firstName, lastName, username string) gotgbot.MergedChatMember {
	return gotgbot.MergedChatMember{
		Status: gotgbot.ChatMemberStatusCreator,
		User:   gotgbot.User{Id: gateTestOwner, FirstName: firstName, LastName: lastName, Username: username},
	}
}

// gateTestSeams scripts the two reads of the gate: the administrator list and the switch.
type gateTestSeams struct {
	list           []gotgbot.MergedChatMember
	fetchErr       error
	allowed        bool
	accessErr      error
	fetchBlocksFor func(ctx context.Context)
}

// gateTestCounts counts how often each scripted read ran.
type gateTestCounts struct {
	fetches       atomic.Int64
	accessLookups atomic.Int64
}

// installGateSeams replaces the two reads of the gate with the script and returns counters of
// how often each ran. Both are restored when the test ends.
func installGateSeams(t *testing.T, seams gateTestSeams) *gateTestCounts {
	t.Helper()
	counts := &gateTestCounts{}
	previousFetch, previousAccess := settingsOwnerFetch, settingsAccessLookup
	t.Cleanup(func() { settingsOwnerFetch, settingsAccessLookup = previousFetch, previousAccess })
	settingsOwnerFetch = func(ctx context.Context, _ *gotgbot.Bot, _ int64) ([]gotgbot.MergedChatMember, error) {
		counts.fetches.Add(1)
		if seams.fetchBlocksFor != nil {
			seams.fetchBlocksFor(ctx)
		}
		return seams.list, seams.fetchErr
	}
	settingsAccessLookup = func(context.Context, int64) (bool, error) {
		counts.accessLookups.Add(1)
		return seams.allowed, seams.accessErr
	}
	return counts
}

func TestSettingsGatePress(t *testing.T) {
	t.Run("an open menu is refused on its next press once the owner shuts the administrator out", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		settingsCaptchaNoWeb(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, true)
		msgID := env.openMenu(env.admin)
		env.setAdminsAllowed(env.chat.Id, false)

		for _, press := range []struct {
			name   string
			fields []string
		}{
			{"a control", []string{"a", "st", "p", "c", "t", "m", "v", "text"}},
			{"Back", []string{"a", "bk"}},
			{"Close", []string{"a", "cl"}},
		} {
			editsBefore := len(env.edits(env.chat.Id, msgID))
			answersBefore := env.answerCount()
			deletesBefore := len(callsToChat(env.fake.staffBotClient, "deleteMessage", env.chat.Id))

			env.press(env.admin, msgID, env.groupFields(env.admin, press.fields...))

			if got := env.answerCount() - answersBefore; got != 1 {
				t.Errorf("%s: the press was answered %d times, want once", press.name, got)
			}
			if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
				t.Errorf("%s: answer = %q (alert %v), want the alert settings_gate_owner_only", press.name, answer, alert)
			}
			if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore {
				t.Errorf("%s: the refused press edited the menu (%d edits, was %d)", press.name, got, editsBefore)
			}
			if got := len(callsToChat(env.fake.staffBotClient, "deleteMessage", env.chat.Id)); got != deletesBefore {
				t.Errorf("%s: the refused press deleted the menu message", press.name)
			}
		}
		if row, ok := settingsCaptchaStored(t, env.chat.Id); ok && row.CaptchaMode == "text" {
			t.Errorf("a refused control press stored %+v", row)
		}

		// The owner's own menu keeps working.
		ownerMenu := env.openMenu(owner)
		text, _ := env.lastEdit(env.chat.Id, ownerMenu)
		if !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("the owner's menu drew %q, want the Main page", text)
		}
	})

	t.Run("a private menu press is refused before any getChat call", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		chatCalls := len(callsToChat(env.fake.staffBotClient, "getChat", env.chat.Id))

		env.pressPrivate(env.admin, 7301, env.privateFields(env.chat.Id, "a", "bk"))

		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_gate_owner_only", answer, alert)
		}
		if got := len(callsToChat(env.fake.staffBotClient, "getChat", env.chat.Id)); got != chatCalls {
			t.Errorf("getChat was called %d times for a gated press, want %d", got, chatCalls)
		}
		if got := len(env.edits(env.admin.Id, 7301)); got != 0 {
			t.Errorf("the gated private press edited the menu %d times", got)
		}
	})

	t.Run("an unreadable switch refuses with settings_check_failed", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		msgID := env.openMenu(env.admin)
		counts := installGateSeams(t, gateTestSeams{accessErr: errors.New("database unavailable")})
		editsBefore := len(env.edits(env.chat.Id, msgID))

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "bk"))

		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_check_failed")) ||
			strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Errorf("answer = %q (alert %v), want the alert settings_check_failed", answer, alert)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != editsBefore {
			t.Errorf("the refused press edited the menu")
		}
		if got := counts.fetches.Load(); got != 0 {
			t.Errorf("the owner state was read %d times although the switch was unreadable", got)
		}
	})

	t.Run("the owner and an allowed administrator pass", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		// The bot is a plain member here, so the roster repair that follows a group /settings
		// stops after its bot lookup and makes no administrator list call: any list call this
		// test counts is the gate's own.
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		ownerMenu := env.openMenu(owner)
		env.press(owner, ownerMenu, env.groupFields(owner, "a", "bk"))
		if text, _ := env.lastEdit(env.chat.Id, ownerMenu); !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("the owner's Back drew %q", text)
		}

		env.setAdminsAllowed(env.chat.Id, true)
		before := settingsAdminListCalls(env, env.chat.Id)
		adminMenu := env.openMenu(env.admin)
		if text, _ := env.lastEdit(env.chat.Id, adminMenu); !strings.Contains(text, staffMarker("settings_header_main")) {
			t.Errorf("the allowed administrator's menu drew %q", text)
		}
		if got := settingsAdminListCalls(env, env.chat.Id); got != before {
			t.Errorf("an allowed administrator caused %d administrator list calls, want none", got-before)
		}
	})
}

func TestSettingsGate(t *testing.T) {
	bot := &gotgbot.Bot{}
	presser := gotgbot.User{Id: gateTestPresser}
	adminListWith := func(owner ...gotgbot.MergedChatMember) []gotgbot.MergedChatMember {
		list := append([]gotgbot.MergedChatMember{}, owner...)
		return append(list, gateTestMember(gateTestPresser, gotgbot.ChatMemberStatusAdministrator))
	}

	for _, tc := range []struct {
		name        string
		status      string
		seams       gateTestSeams
		timeout     time.Duration
		want        settingsGateVerdict
		wantFetches int64
		wantAccess  int64
	}{
		{name: "creator is in with no read and no call", status: gotgbot.ChatMemberStatusCreator, want: settingsGateIn},
		{name: "administrator with the switch on is in with no list call", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{allowed: true}, want: settingsGateIn, wantAccess: 1},
		{name: "a live owner keeps an administrator out", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: adminListWith(gateTestOwnerEntry("Owner", "", ""))},
			want:  settingsGateOwnerOnly, wantFetches: 1, wantAccess: 1},
		{name: "no creator entry lets an administrator in", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: adminListWith()}, want: settingsGateIn, wantFetches: 1, wantAccess: 1},
		{name: "a deleted-account creator lets an administrator in", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: adminListWith(gateTestOwnerEntry("Deleted Account", "", ""))},
			want:  settingsGateIn, wantFetches: 1, wantAccess: 1},
		{name: "a creator named Deleted Account who keeps a username is a live owner", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: adminListWith(gateTestOwnerEntry("Deleted Account", "", "realowner"))},
			want:  settingsGateOwnerOnly, wantFetches: 1, wantAccess: 1},
		{name: "a creator named Deleted Account who keeps a last name is a live owner", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: adminListWith(gateTestOwnerEntry("Deleted Account", "Person", ""))},
			want:  settingsGateOwnerOnly, wantFetches: 1, wantAccess: 1},
		{name: "a failing list is undecided", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{fetchErr: errors.New("telegram is down")},
			want:  settingsGateUndecided, wantFetches: 1, wantAccess: 1},
		{name: "a list that arrives after the deadline is undecided", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{
				list: adminListWith(gateTestOwnerEntry("Owner", "", "")),
				// Ignores the context and answers late: the answer must not be used.
				fetchBlocksFor: func(ctx context.Context) { <-ctx.Done(); time.Sleep(10 * time.Millisecond) },
			},
			timeout: 50 * time.Millisecond,
			want:    settingsGateUndecided, wantFetches: 1, wantAccess: 1},
		{name: "an empty list is undecided", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: []gotgbot.MergedChatMember{}}, want: settingsGateUndecided, wantFetches: 1, wantAccess: 1},
		{name: "a list without the presser is undecided", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: []gotgbot.MergedChatMember{gateTestOwnerEntry("Owner", "", "")}},
			want:  settingsGateUndecided, wantFetches: 1, wantAccess: 1},
		{name: "a creator entry that is the presser is undecided", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{list: []gotgbot.MergedChatMember{{
				Status: gotgbot.ChatMemberStatusCreator,
				User:   gotgbot.User{Id: gateTestPresser, FirstName: "Me"},
			}}},
			want: settingsGateUndecided, wantFetches: 1, wantAccess: 1},
		{name: "an unreadable switch is undecided and reads no list", status: gotgbot.ChatMemberStatusAdministrator,
			seams: gateTestSeams{accessErr: errors.New("database unavailable")},
			want:  settingsGateUndecided, wantAccess: 1},
		{name: "a plain member is undecided with no read", status: gotgbot.ChatMemberStatusMember, want: settingsGateUndecided},
		{name: "a restricted member is undecided with no read", status: gotgbot.ChatMemberStatusRestricted, want: settingsGateUndecided},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetSettingsOwnerMemo(t)
			counts := installGateSeams(t, tc.seams)
			if tc.timeout > 0 {
				previous := settingsOwnerLookupTimeout
				settingsOwnerLookupTimeout = tc.timeout
				t.Cleanup(func() { settingsOwnerLookupTimeout = previous })
			}

			got := settingsGate(bot, gateTestChatID(), presser, gateTestMember(gateTestPresser, tc.status))

			if got != tc.want {
				t.Errorf("settingsGate = %d, want %d", got, tc.want)
			}
			if f := counts.fetches.Load(); f != tc.wantFetches {
				t.Errorf("administrator list fetches = %d, want %d", f, tc.wantFetches)
			}
			if a := counts.accessLookups.Load(); a != tc.wantAccess {
				t.Errorf("switch reads = %d, want %d", a, tc.wantAccess)
			}
		})
	}
}

func TestSettingsOwnerState(t *testing.T) {
	t.Run("a live owner is found in a real administrator answer", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id); got != settingsOwnerExists {
			t.Errorf("settingsOwnerState = %d, want exists", got)
		}
	})

	t.Run("no creator entry is none", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.fake.clearCreator(env.chat.Id)
		env.listAdmin(env.chat.Id, env.admin.Id)
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id); got != settingsOwnerNone {
			t.Errorf("settingsOwnerState = %d, want none", got)
		}
	})

	t.Run("a deleted-account creator is none", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.fake.setDeletedCreator(env.chat.Id)
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id); got != settingsOwnerNone {
			t.Errorf("settingsOwnerState = %d, want none", got)
		}
	})

	t.Run("a failing call is unknown", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.fake.setFailure("getChatAdministrators", env.chat.Id, errors.New("telegram is down"))
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id); got != settingsOwnerUnknown {
			t.Errorf("settingsOwnerState = %d, want unknown", got)
		}
	})

	t.Run("a presser who is not listed is unknown", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id+99); got != settingsOwnerUnknown {
			t.Errorf("settingsOwnerState = %d, want unknown", got)
		}
	})

	t.Run("an empty list is unknown", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.fake.clearCreator(env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleAbsent)
		if got := settingsOwnerState(env.bot, env.chat.Id, env.admin.Id); got != settingsOwnerUnknown {
			t.Errorf("settingsOwnerState = %d, want unknown", got)
		}
	})
}

func TestSettingsOwnerStateSingleflight(t *testing.T) {
	resetSettingsOwnerMemo(t)
	release := make(chan struct{})
	seams := installGateSeams(t, gateTestSeams{
		list:           []gotgbot.MergedChatMember{gateTestOwnerEntry("Owner", "", ""), gateTestMember(gateTestPresser, gotgbot.ChatMemberStatusAdministrator)},
		fetchBlocksFor: func(context.Context) { <-release },
	})
	chatID := gateTestChatID()

	const readers = 8
	var started, done sync.WaitGroup
	results := make([]settingsOwnerStateResult, readers)
	for i := 0; i < readers; i++ {
		started.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			started.Done()
			results[i] = settingsOwnerState(&gotgbot.Bot{}, chatID, gateTestPresser)
		}()
	}
	started.Wait()
	// Give every reader time to join the flight before the one call is allowed to finish.
	time.Sleep(150 * time.Millisecond)
	close(release)
	done.Wait()

	if got := seams.fetches.Load(); got != 1 {
		t.Errorf("%d concurrent readers made %d getChatAdministrators calls, want 1", readers, got)
	}
	for i, got := range results {
		if got != settingsOwnerExists {
			t.Errorf("reader %d got %d, want exists", i, got)
		}
	}
}

func TestSettingsOwnerStateMemo(t *testing.T) {
	ownerList := []gotgbot.MergedChatMember{
		gateTestOwnerEntry("Owner", "", ""),
		gateTestMember(gateTestPresser, gotgbot.ChatMemberStatusAdministrator),
	}
	var clockMu sync.Mutex
	base := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	clock := base
	setClock := func(at time.Time) {
		clockMu.Lock()
		defer clockMu.Unlock()
		clock = at
	}
	installClock := func(t *testing.T) {
		t.Helper()
		previous := settingsOwnerNow
		settingsOwnerNow = func() time.Time {
			clockMu.Lock()
			defer clockMu.Unlock()
			return clock
		}
		t.Cleanup(func() { settingsOwnerNow = previous })
		setClock(base)
	}

	t.Run("an owner-exists answer is served from memory until exactly the TTL", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		installClock(t)
		seams := installGateSeams(t, gateTestSeams{list: ownerList})
		chatID := gateTestChatID()

		if got := settingsOwnerState(&gotgbot.Bot{}, chatID, gateTestPresser); got != settingsOwnerExists {
			t.Fatalf("first read = %d, want exists", got)
		}
		setClock(base.Add(settingsOwnerMemoTTL - time.Nanosecond))
		if got := settingsOwnerState(&gotgbot.Bot{}, chatID, gateTestPresser); got != settingsOwnerExists {
			t.Fatalf("read inside the TTL = %d, want exists", got)
		}
		if got := seams.fetches.Load(); got != 1 {
			t.Fatalf("fetches inside the TTL = %d, want 1", got)
		}
		setClock(base.Add(settingsOwnerMemoTTL))
		if got := settingsOwnerState(&gotgbot.Bot{}, chatID, gateTestPresser); got != settingsOwnerExists {
			t.Fatalf("read at the TTL = %d, want exists", got)
		}
		if got := seams.fetches.Load(); got != 2 {
			t.Errorf("fetches at exactly the TTL = %d, want 2 (a memo that old is expired)", got)
		}
	})

	t.Run("none, an error and a timeout are never remembered", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			seams   gateTestSeams
			timeout time.Duration
			want    settingsOwnerStateResult
		}{
			{"none", gateTestSeams{list: []gotgbot.MergedChatMember{gateTestMember(gateTestPresser, gotgbot.ChatMemberStatusAdministrator)}}, 0, settingsOwnerNone},
			{"error", gateTestSeams{fetchErr: errors.New("telegram is down")}, 0, settingsOwnerUnknown},
			{"timeout", gateTestSeams{list: ownerList, fetchBlocksFor: func(ctx context.Context) { <-ctx.Done() }}, 50 * time.Millisecond, settingsOwnerUnknown},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resetSettingsOwnerMemo(t)
				installClock(t)
				seams := installGateSeams(t, tc.seams)
				if tc.timeout > 0 {
					previous := settingsOwnerLookupTimeout
					settingsOwnerLookupTimeout = tc.timeout
					t.Cleanup(func() { settingsOwnerLookupTimeout = previous })
				}
				chatID := gateTestChatID()
				for i := 0; i < 2; i++ {
					if got := settingsOwnerState(&gotgbot.Bot{}, chatID, gateTestPresser); got != tc.want {
						t.Fatalf("read %d = %d, want %d", i, got, tc.want)
					}
				}
				if got := seams.fetches.Load(); got != 2 {
					t.Errorf("fetches = %d, want 2: the answer must not be remembered", got)
				}
			})
		}
	})

	t.Run("the switch is read before the memo, so a flip to on is seen at once", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		member := gateTestMember(env.admin.Id, gotgbot.ChatMemberStatusAdministrator)

		if got := settingsGate(env.bot, env.chat.Id, env.admin, member); got != settingsGateOwnerOnly {
			t.Fatalf("switch off = %d, want owner only", got)
		}
		if calls := settingsAdminListCalls(env, env.chat.Id); calls != 1 {
			t.Fatalf("administrator list calls = %d, want 1", calls)
		}

		env.setAdminsAllowed(env.chat.Id, true)
		if got := settingsGate(env.bot, env.chat.Id, env.admin, member); got != settingsGateIn {
			t.Errorf("switch flipped on = %d, want in at the very next press", got)
		}

		env.setAdminsAllowed(env.chat.Id, false)
		if got := settingsGate(env.bot, env.chat.Id, env.admin, member); got != settingsGateOwnerOnly {
			t.Errorf("switch flipped off again = %d, want owner only", got)
		}
		if calls := settingsAdminListCalls(env, env.chat.Id); calls != 1 {
			t.Errorf("administrator list calls = %d, want the memo to answer the later presses", calls)
		}
	})
}

func TestSettingsTelegramClientError(t *testing.T) {
	telegram := func(code int) error { return &gotgbot.TelegramError{Method: "getChatMember", Code: code} }
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"400", telegram(400), true},
		{"403", telegram(403), true},
		{"404", telegram(404), true},
		{"499", telegram(499), true},
		{"a wrapped 403", fmt.Errorf("live check: %w", telegram(403)), true},
		{"429 is a rate limit", telegram(429), false},
		{"a wrapped 429", fmt.Errorf("live check: %w", telegram(429)), false},
		{"500", telegram(500), false},
		{"502", telegram(502), false},
		{"399", telegram(399), false},
		{"a transport error", errors.New("timeout"), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTelegramClientError(tc.err); got != tc.want {
				t.Errorf("isTelegramClientError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSettingsAuthorityParityOwnerGate(t *testing.T) {
	// The documented D-19 carve-out of the authority-parity rule. With the owner switch off and a
	// live owner in the group, the menu is stricter than the commands for an administrator who is
	// not the owner: the menu refuses and the commands in the group still work. The rule that
	// remains is one way: where the menu allows a control, the command allows it too. The
	// commands are not tightened.
	t.Run("a gated administrator is refused by the menu and allowed by the commands", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		settingsCaptchaNoWeb(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		const msgID = 7401

		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "st", "p", "c", "t", "m", "v", "text"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Fatalf("Captcha mode press answered %q (alert %v), want the alert settings_gate_owner_only", answer, alert)
		}
		if row, ok := settingsCaptchaStored(t, env.chat.Id); ok {
			t.Fatalf("the refused Captcha mode press stored %+v", row)
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "st", "p", "f", "t", "l", "v", "6"))
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_gate_owner_only")) {
			t.Fatalf("Antiflood limit press answered %q (alert %v), want the alert settings_gate_owner_only", answer, alert)
		}
		if row, ok := settingsAntifloodRow(t, env.chat.Id); ok {
			t.Fatalf("the refused Antiflood limit press stored %+v", row)
		}

		env.send(env.admin, "/captchamode text")
		if row, ok := settingsCaptchaStored(t, env.chat.Id); !ok || row.CaptchaMode != "text" {
			t.Errorf("/captchamode text stored %+v (row %v), want text", row, ok)
		}
		env.send(env.admin, "/setflood 7")
		if row, ok := settingsAntifloodRow(t, env.chat.Id); !ok || row.Limit != 7 {
			t.Errorf("/setflood 7 stored %+v (row %v), want limit 7", row, ok)
		}
	})

	t.Run("where the menu allows a control the command allows it too", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		settingsCaptchaNoWeb(t)
		env := newSettingsEnv(t)
		menuChat := env.addChat()
		env.newOwner(menuChat)
		env.setAdminsAllowed(menuChat.Id, true)

		env.pressRaw(menuChat, env.admin, 7402,
			encodeCallbackData(settingsCallbackNamespace, env.groupFields(env.admin, "a", "st", "p", "c", "t", "m", "v", "text")))
		menuRow, menuHas := settingsCaptchaStored(t, menuChat.Id)
		if !menuHas || menuRow.CaptchaMode != "text" {
			t.Fatalf("the allowed administrator's press stored %+v (row %v), want text", menuRow, menuHas)
		}

		env.send(env.admin, "/captchamode text")
		commandRow, commandHas := settingsCaptchaStored(t, env.chat.Id)
		if !commandHas || commandRow.CaptchaMode != menuRow.CaptchaMode {
			t.Errorf("the command stored %+v (row %v), want what the menu stored: %+v", commandRow, commandHas, menuRow)
		}
	})
}

// settingsGateFixture is one world the owner gate judges: how the group looks after setup and
// what every door must decide for the person. The decisions are "in", "owner_only" and
// "check_failed".
type settingsGateFixture struct {
	name string
	// owner makes the person the group's owner (setup creates them); otherwise it is the
	// default administrator.
	owner bool
	want  string
	setup func(env *settingsEnv)
}

// settingsGateCase is what a door driver gets: a fresh environment, the fixture's setup and
// the person. A door calls apply exactly once, at the point where the world changes; the
// press door opens its menu first, because a menu opened while the switch was on is gated
// again on every press (D-10).
type settingsGateCase struct {
	env         *settingsEnv
	apply       func()
	person      func() gotgbot.User
	ownerPerson bool
}

// settingsGateDoor is one way into the settings. The pick of the private list is the fifth door
// and the typed reply of the message editor the sixth.
type settingsGateDoor struct {
	name   string
	decide func(t *testing.T, c settingsGateCase) string
}

// settingsGateClassify reduces the texts a door answered with to its decision.
func settingsGateClassify(texts []string) string {
	decision := "none"
	for _, text := range texts {
		switch {
		case strings.Contains(text, staffMarker("settings_refuse_owner_only")),
			strings.Contains(text, staffMarker("settings_start_owner_only")),
			strings.Contains(text, staffMarker("settings_gate_owner_only")):
			return "owner_only"
		case strings.Contains(text, staffMarker("settings_check_failed")):
			return "check_failed"
		case strings.Contains(text, staffMarker("settings_choose")),
			strings.Contains(text, staffMarker("settings_header_main")),
			strings.Contains(text, staffMarker("settings_edit_saved_text")):
			decision = "in"
		}
	}
	return decision
}

// settingsGateNewTexts returns the texts of the messages the bot sent to chatID after the
// first `before` ones.
func settingsGateNewTexts(env *settingsEnv, chatID int64, before int) []string {
	sent := env.fake.sentTo(chatID)
	var texts []string
	for i := before; i < len(sent); i++ {
		texts = append(texts, fmt.Sprint(sent[i].Params["text"]))
	}
	return texts
}

func settingsGateDoorGroupCommand(t *testing.T, c settingsGateCase) string {
	t.Helper()
	c.apply()
	person := c.person()
	before := len(c.env.fake.sentTo(c.env.chat.Id))
	c.env.send(person, "/settings")
	return settingsGateClassify(settingsGateNewTexts(c.env, c.env.chat.Id, before))
}

func settingsGateDoorAnonymous(t *testing.T, c settingsGateCase) string {
	t.Helper()
	c.apply()
	person := c.person()
	c.env.sendAnonymous("/settings")
	c.env.wantProofPrompt()
	before := len(c.env.fake.sentTo(c.env.chat.Id))
	c.env.tapProof(person)
	return settingsGateClassify(settingsGateNewTexts(c.env, c.env.chat.Id, before))
}

func settingsGateDoorDeepLink(t *testing.T, c settingsGateCase) string {
	t.Helper()
	c.apply()
	person := c.person()
	before := len(c.env.fake.sentTo(person.Id))
	c.env.sendPrivate(person, fmt.Sprintf("/start settings_%d", c.env.chat.Id))
	return settingsGateClassify(settingsGateNewTexts(c.env, person.Id, before))
}

func settingsGateDoorPress(t *testing.T, c settingsGateCase) string {
	t.Helper()
	var msgID int64
	if !c.ownerPerson {
		// The administrator opens the menu while the switch is on, before the world changes.
		msgID = c.env.openMenu(c.env.admin)
	}
	c.apply()
	person := c.person()
	if c.ownerPerson {
		msgID = c.env.openMenu(person)
	}
	edits := len(c.env.edits(c.env.chat.Id, msgID))
	answers := c.env.answerCount()

	c.env.press(person, msgID, c.env.groupFields(person, "a", "bk"))

	if got := c.env.answerCount() - answers; got != 1 {
		t.Fatalf("the press was answered %d times, want once", got)
	}
	answer, alert := c.env.lastAnswer()
	if alert {
		return settingsGateClassify([]string{answer})
	}
	if len(c.env.edits(c.env.chat.Id, msgID)) > edits {
		return "in"
	}
	return "none"
}

// settingsGateDoorDeletionYes presses the Self-destruct card's Yes (the page action dy, plan 17-10) as
// the person, who holds can_change_info when an administrator, so the gate alone decides. A press that
// is refused answers an alert; one that is let in writes the rule and edits the menu.
func settingsGateDoorDeletionYes(t *testing.T, c settingsGateCase) string {
	t.Helper()
	withFastDeletionPacer(t)
	deletionCleanup(t, c.env.chat.Id)
	var msgID int64
	if !c.ownerPerson {
		msgID = c.env.openMenu(c.env.admin)
	}
	c.apply()
	person := c.person()
	if c.ownerPerson {
		msgID = c.env.openMenu(person)
	} else {
		listEditAdmin(c.env, c.env.chat, person, true)
	}
	edits := len(c.env.edits(c.env.chat.Id, msgID))
	answers := c.env.answerCount()

	c.env.press(person, msgID, c.env.groupFields(person, "a", "dy", "t", "ms", "v", "600", "e", settingsCardExpiry(time.Now())))

	if got := c.env.answerCount() - answers; got != 1 {
		t.Fatalf("the Yes was answered %d times, want once", got)
	}
	answer, alert := c.env.lastAnswer()
	if alert {
		return settingsGateClassify([]string{answer})
	}
	if len(c.env.edits(c.env.chat.Id, msgID)) > edits {
		return "in"
	}
	return "none"
}

// settingsGateDoorPick presses a row of the private group list for the env's group as the
// person. A pick that is refused answers an alert; one that is let in edits the list message
// into the group's Main page.
func settingsGateDoorPick(t *testing.T, c settingsGateCase) string {
	t.Helper()
	const listMsg = 6301
	c.apply()
	person := c.person()
	edits := len(c.env.edits(person.Id, listMsg))
	answers := c.env.answerCount()

	c.env.pressPrivate(person, listMsg, c.env.privateFields(c.env.chat.Id, "a", settingsActPick))

	if got := c.env.answerCount() - answers; got != 1 {
		t.Fatalf("the pick was answered %d times, want once", got)
	}
	if answer, alert := c.env.lastAnswer(); alert {
		return settingsGateClassify([]string{answer})
	}
	if len(c.env.edits(person.Id, listMsg)) > edits {
		return "in"
	}
	return "none"
}

// settingsGateDoorTypedReply is the typed-reply door: the person presses Set text of the Welcome
// page in a private menu while the world still lets them in (the administrator before apply, the
// owner after it, as the press door opens its menu), then sends a private text. A let-in reply is
// saved and answered with the saved page; a refused one gets the gate's own words.
func settingsGateDoorTypedReply(t *testing.T, c settingsGateCase) string {
	t.Helper()
	if !c.ownerPerson {
		// The administrator may change the greetings: can_change_info on the live record.
		listEditAdmin(c.env, c.env.chat, c.env.admin, true)
		settingsEditSetWait(c.env, c.env.admin, settingsPageWelcome, settingsEditPartText)
	}
	c.apply()
	person := c.person()
	if c.ownerPerson {
		settingsEditSetWait(c.env, person, settingsPageWelcome, settingsEditPartText)
	}
	before := len(c.env.fake.sentTo(person.Id))

	sendPrivateMessage(c.env, person, gotgbot.Message{Text: "a typed welcome"})

	return settingsGateClassify(settingsGateNewTexts(c.env, person.Id, before))
}

func TestSettingsGateAllDoors(t *testing.T) {
	fixtures := []settingsGateFixture{
		{
			name: "owner exists, switch off: the administrator", want: "owner_only",
			setup: func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, false)
			},
		},
		{
			name: "owner exists, switch off: the owner", owner: true, want: "in",
			setup: func(env *settingsEnv) {
				env.setAdminsAllowed(env.chat.Id, false)
			},
		},
		{
			name: "switch on", want: "in",
			setup: func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, true)
			},
		},
		{
			name: "no owner", want: "in",
			setup: func(env *settingsEnv) {
				env.fake.clearCreator(env.chat.Id)
				env.listAdmin(env.chat.Id, env.admin.Id)
				env.setAdminsAllowed(env.chat.Id, false)
			},
		},
		{
			name: "owner state unknown", want: "check_failed",
			setup: func(env *settingsEnv) {
				env.newOwner(env.chat)
				env.setAdminsAllowed(env.chat.Id, false)
				env.fake.setFailure("getChatAdministrators", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
		},
	}
	doors := []settingsGateDoor{
		{name: "group /settings", decide: settingsGateDoorGroupCommand},
		{name: "anonymous re-entry", decide: settingsGateDoorAnonymous},
		{name: "deep link", decide: settingsGateDoorDeepLink},
		{name: "press", decide: settingsGateDoorPress},
		{name: "deletion Yes", decide: settingsGateDoorDeletionYes},
		{name: "pick", decide: settingsGateDoorPick},
		{name: "typed reply", decide: settingsGateDoorTypedReply},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			decisions := make(map[string]string, len(doors))
			for _, door := range doors {
				resetSettingsOwnerMemo(t)
				env := newSettingsEnv(t)
				var owner gotgbot.User
				c := settingsGateCase{
					env:         env,
					ownerPerson: fixture.owner,
					apply: func() {
						if fixture.owner {
							owner = env.newOwner(env.chat)
						}
						fixture.setup(env)
					},
					person: func() gotgbot.User {
						if fixture.owner {
							return owner
						}
						return env.admin
					},
				}
				decisions[door.name] = door.decide(t, c)
			}
			for _, door := range doors {
				if decisions[door.name] != fixture.want {
					t.Errorf("fixture %q: door %q decided %q, want %q (all doors: %v)",
						fixture.name, door.name, decisions[door.name], fixture.want, decisions)
				}
			}
		})
	}
}
