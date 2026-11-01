//go:build testtools

package modules

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// runStaffCommand has the issuer send a staff command in the Staff Group, confirm
// the card it produced and wait for the fan-out. It returns the final summary
// text and the card's message ID.
func (e *staffActionEnv) runStaffCommand(command string) (summary string, msgID int64) {
	e.t.Helper()
	e.send(e.issuer, command)
	token, msgID := e.card()
	e.tap(e.issuer, staffActRunConfirm, token, msgID)
	e.waitRuns()
	return e.lastEditText(e.staffChat, msgID), msgID
}

// summaryLine returns the line of a summary that names the group title.
func summaryLine(t *testing.T, summary, title string) string {
	t.Helper()
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, title) {
			return line
		}
	}
	t.Fatalf("summary has no line for %q:\n%s", title, summary)
	return ""
}

// wantDoneLine fails unless the group's line is a plain success.
func wantDoneLine(t *testing.T, summary, title string) {
	t.Helper()
	if line := summaryLine(t, summary, title); !strings.HasPrefix(line, "✅") {
		t.Fatalf("line for %s = %q, want a done line", title, line)
	}
}

// wantSkippedLine fails unless the group's line is a skip for the given reason key.
func wantSkippedLine(t *testing.T, summary, title, reasonKey string) {
	t.Helper()
	line := summaryLine(t, summary, title)
	if !strings.Contains(line, staffMarker("staff_act_line_skipped")) || !strings.Contains(line, staffMarker(reasonKey)) {
		t.Fatalf("line for %s = %q, want skipped with %s", title, line, reasonKey)
	}
}

// permsJSON renders permissions as JSON so a recorded request parameter and an
// expected value compare without caring about pointer fields.
func permsJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal permissions: %v", err)
	}
	return string(raw)
}

// wantMember fails unless the fake shows userID in chatID with the status.
func wantMember(t *testing.T, fake *staffActionFake, chatID, userID int64, status string) *staffFakeMember {
	t.Helper()
	m := fake.member(chatID, userID)
	if m == nil || m.Status != status {
		t.Fatalf("member %d of chat %d = %+v, want status %s", userID, chatID, m, status)
	}
	return m
}

func TestStaffActionFiveActions(t *testing.T) {
	t.Run("mute", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		g1, g2, g3 := env.groups[0], env.groups[1], env.groups[2]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		env.fake.setMember(g3, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		summary, _ := env.runStaffCommand("/mute 4242 flooding")

		for _, group := range []int64{g1, g3} {
			calls := env.callsTo("restrictChatMember", group)
			if len(calls) != 1 || staffParamInt(calls[0].Params, "user_id") != staffTestTarget {
				t.Fatalf("restrictChatMember calls in group %d = %+v, want one for the target", group, calls)
			}
			if until := staffParamInt(calls[0].Params, "until_date"); until != 0 {
				t.Fatalf("until_date in group %d = %d, want a permanent mute", group, until)
			}
			if got, want := permsJSON(t, calls[0].Params["permissions"]), permsJSON(t, MutedPermissions); got != want {
				t.Fatalf("permissions in group %d = %s, want MutedPermissions %s", group, got, want)
			}
			m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted)
			if m.CanSendMessages || !m.IsMember {
				t.Fatalf("target in group %d = %+v, want a muted member", group, m)
			}
		}
		if writes := env.writes(g2); len(writes) != 0 {
			t.Fatalf("write calls in the group the target left = %+v, want none", writes)
		}
		wantDoneLine(t, summary, "Group A")
		wantSkippedLine(t, summary, "Group B", "staff_act_skip_not_in_group")
		wantDoneLine(t, summary, "Group C")
	})

	t.Run("kick", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		g1, g2, g3 := env.groups[0], env.groups[1], env.groups[2]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
		env.fake.setMember(g3, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true})

		summary, _ := env.runStaffCommand("/kick 4242")

		for _, group := range []int64{g1, g3} {
			calls := env.callsTo("unbanChatMember", group)
			if len(calls) != 1 || staffParamInt(calls[0].Params, "user_id") != staffTestTarget {
				t.Fatalf("unbanChatMember calls in group %d = %+v, want one for the target", group, calls)
			}
			if staffParamBool(calls[0].Params, "only_if_banned") {
				t.Fatalf("a kick in group %d sent only_if_banned=true, which would not remove the member", group)
			}
			wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusLeft)
		}
		if writes := env.writes(g2); len(writes) != 0 {
			t.Fatalf("write calls in the group the target left = %+v, want none", writes)
		}
		wantDoneLine(t, summary, "Group A")
		wantSkippedLine(t, summary, "Group B", "staff_act_skip_not_in_group")
		wantDoneLine(t, summary, "Group C")
	})

	t.Run("unban", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		g1, g2, g3 := env.groups[0], env.groups[1], env.groups[2]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
		env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.fake.setMember(g3, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

		summary, _ := env.runStaffCommand("/unban 4242")

		calls := env.callsTo("unbanChatMember", g1)
		if len(calls) != 1 || !staffParamBool(calls[0].Params, "only_if_banned") {
			t.Fatalf("unbanChatMember calls in group A = %+v, want one with only_if_banned=true", calls)
		}
		wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusLeft)
		for _, group := range []int64{g2, g3} {
			if writes := env.writes(group); len(writes) != 0 {
				t.Fatalf("write calls in group %d = %+v, want none for a target who is not banned", group, writes)
			}
		}
		wantMember(t, env.fake, g2, staffTestTarget, gotgbot.ChatMemberStatusMember)
		wantDoneLine(t, summary, "Group A")
		wantSkippedLine(t, summary, "Group B", "staff_act_skip_not_banned")
		wantSkippedLine(t, summary, "Group C", "staff_act_skip_not_banned")
	})

	t.Run("unmute", func(t *testing.T) {
		env := newStaffActionEnv(t, 3)
		g1, g2, g3 := env.groups[0], env.groups[1], env.groups[2]
		custom := gotgbot.ChatPermissions{CanSendMessages: true, CanSendPhotos: true, CanInviteUsers: true}
		env.fake.chatPerms[g1] = custom
		muted := staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true}
		env.fake.setMember(g1, staffTestTarget, muted)
		env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		env.fake.setMember(g3, staffTestTarget, muted)
		env.fake.script("getChat", g3, staffFakeError(400, "Bad Request: chat not found"))

		summary, _ := env.runStaffCommand("/unmute 4242")

		calls := env.callsTo("restrictChatMember", g1)
		if len(calls) != 1 {
			t.Fatalf("restrictChatMember calls in group A = %+v, want one", calls)
		}
		if got, want := permsJSON(t, calls[0].Params["permissions"]), permsJSON(t, custom); got != want {
			t.Fatalf("restored permissions = %s, want the group's own %s", got, want)
		}
		if m := wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusRestricted); !m.CanSendMessages || !m.IsMember {
			t.Fatalf("target in group A = %+v, want an unmuted member", m)
		}
		if writes := env.writes(g2); len(writes) != 0 {
			t.Fatalf("write calls in group B = %+v, want none for a member who is not muted", writes)
		}
		if writes := env.writes(g3); len(writes) != 0 {
			t.Fatalf("write calls in group C = %+v, want none when getChat failed", writes)
		}
		if m := wantMember(t, env.fake, g3, staffTestTarget, gotgbot.ChatMemberStatusRestricted); m.CanSendMessages {
			t.Fatalf("target in group C = %+v, want still muted", m)
		}
		wantDoneLine(t, summary, "Group A")
		wantSkippedLine(t, summary, "Group B", "staff_act_skip_not_muted")
		failed := summaryLine(t, summary, "Group C")
		if !strings.HasPrefix(failed, "❌") || !strings.Contains(failed, "Bad Request: chat not found") {
			t.Fatalf("line for group C = %q, want a failed line carrying Telegram's error", failed)
		}
	})
}

// TestStaffActionNeverLiftsBan is the proof of the research flag: a mute, an
// unmute and a kick aimed at a target banned in one group never send a restrict
// or an unban there, while the group where the target is a member is acted on.
func TestStaffActionNeverLiftsBan(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	steps := []struct {
		command string
		check   func(m *staffFakeMember)
	}{
		{"/mute 4242", func(m *staffFakeMember) {
			if m == nil || m.Status != gotgbot.ChatMemberStatusRestricted || m.CanSendMessages || !m.IsMember {
				t.Fatalf("after /mute the target in group B = %+v, want a muted member", m)
			}
		}},
		{"/unmute 4242", func(m *staffFakeMember) {
			if m == nil || m.Status != gotgbot.ChatMemberStatusRestricted || !m.CanSendMessages || !m.IsMember {
				t.Fatalf("after /unmute the target in group B = %+v, want an unmuted member", m)
			}
		}},
		{"/kick 4242", func(m *staffFakeMember) {
			if m == nil || m.Status != gotgbot.ChatMemberStatusLeft {
				t.Fatalf("after /kick the target in group B = %+v, want left", m)
			}
		}},
	}
	for _, step := range steps {
		summary, _ := env.runStaffCommand(step.command)
		wantSkippedLine(t, summary, "Group A", "staff_act_skip_not_in_group")
		wantDoneLine(t, summary, "Group B")
		step.check(env.fake.member(g2, staffTestTarget))

		if writes := env.writes(g1); len(writes) != 0 {
			t.Fatalf("after %s the banned group got write calls %+v, want none", step.command, writes)
		}
		wantBanned(t, env.fake, g1, staffTestTarget)
	}
	if calls := env.callsTo("restrictChatMember", g1); len(calls) != 0 {
		t.Fatalf("restrictChatMember calls to the banned group = %+v, want none", calls)
	}
	if calls := env.callsTo("unbanChatMember", g1); len(calls) != 0 {
		t.Fatalf("unbanChatMember calls to the banned group = %+v, want none", calls)
	}
}

func TestStaffActionBanOverMute(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g1 := env.groups[0]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true})

	summary, _ := env.runStaffCommand("/ban 4242")

	if calls := env.callsTo("banChatMember", g1); len(calls) != 1 {
		t.Fatalf("banChatMember calls = %+v, want one", calls)
	}
	wantBanned(t, env.fake, g1, staffTestTarget)
	wantDoneLine(t, summary, "Group A")
}

func TestStaffActionMuteNeverShortens(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	later := time.Now().Add(2 * time.Hour).Unix()
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: later})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true})

	summary, _ := env.runStaffCommand("/mute 4242")

	if calls := env.callsTo("restrictChatMember", g1); len(calls) != 1 {
		t.Fatalf("restrictChatMember calls in group A = %+v, want the permanent mute applied", calls)
	}
	if m := wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusRestricted); m.UntilDate != 0 {
		t.Fatalf("mute end in group A = %d, want permanent", m.UntilDate)
	}
	wantDoneLine(t, summary, "Group A")

	if writes := env.writes(g2); len(writes) != 0 {
		t.Fatalf("write calls in group B = %+v, want none for a permanent mute", writes)
	}
	wantSkippedLine(t, summary, "Group B", "staff_act_skip_already_muted")
}

func TestStaffActionKickClearsMute(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g1 := env.groups[0]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true})

	summary, _ := env.runStaffCommand("/kick 4242")

	wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusLeft)
	wantDoneLine(t, summary, "Group A")
}

func TestStaffActionUnmuteNonMember(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g1 := env.groups[0]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted})

	summary, _ := env.runStaffCommand("/unmute 4242")

	calls := env.callsTo("restrictChatMember", g1)
	if len(calls) != 1 {
		t.Fatalf("restrictChatMember calls = %+v, want one", calls)
	}
	if got, want := permsJSON(t, calls[0].Params["permissions"]), permsJSON(t, defaultUnmutePermissions()); got != want {
		t.Fatalf("restored permissions = %s, want the defaults %s", got, want)
	}
	m := wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusRestricted)
	if m.IsMember || !m.CanSendMessages {
		t.Fatalf("target = %+v, want an unmuted non-member", m)
	}
	wantDoneLine(t, summary, "Group A")
}

func TestStaffActionHeaderDuration(t *testing.T) {
	cases := []struct {
		command     string
		nameKey     string
		wantPerm    bool
		targetState staffFakeMember
	}{
		{"/ban 4242", "staff_act_name_ban", true, staffFakeMember{Status: gotgbot.ChatMemberStatusMember}},
		{"/mute 4242", "staff_act_name_mute", true, staffFakeMember{Status: gotgbot.ChatMemberStatusMember}},
		{"/kick 4242", "staff_act_name_kick", false, staffFakeMember{Status: gotgbot.ChatMemberStatusMember}},
		{"/unban 4242", "staff_act_name_unban", false, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked}},
		{"/unmute 4242", "staff_act_name_unmute", false, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true}},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.fake.setMember(env.groups[0], staffTestTarget, tc.targetState)

			env.send(env.issuer, tc.command)
			sent := env.fake.sentTo(env.staffChat)
			if len(sent) != 1 {
				t.Fatalf("messages to the Staff Group = %d, want the card", len(sent))
			}
			card, _ := sent[0].Params["text"].(string)
			if !strings.Contains(card, staffMarker(tc.nameKey)) {
				t.Fatalf("card = %q, want the action name %s", card, tc.nameKey)
			}
			if got := strings.Contains(card, staffMarker("staff_act_duration_permanent")); got != tc.wantPerm {
				t.Fatalf("card shows a duration = %t, want %t: %q", got, tc.wantPerm, card)
			}

			token, msgID := env.card()
			env.tap(env.issuer, staffActRunConfirm, token, msgID)
			env.waitRuns()
			summary := env.lastEditText(env.staffChat, msgID)
			if got := strings.Contains(summary, staffMarker("staff_act_duration_permanent")); got != tc.wantPerm {
				t.Fatalf("summary shows a duration = %t, want %t: %q", got, tc.wantPerm, summary)
			}
		})
	}
}
