//go:build testtools

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// staffUntilSlack is how far a recorded until_date may sit from Confirm time plus
// the duration: the card computes it from the clock at Confirm.
const staffUntilSlack = 5

// lastCardText is the text of the last message sent to the Staff Group.
func (e *staffActionEnv) lastCardText() string {
	e.t.Helper()
	sent := e.fake.sentTo(e.staffChat)
	if len(sent) == 0 {
		e.t.Fatal("no message was sent to the Staff Group")
	}
	text, _ := sent[len(sent)-1].Params["text"].(string)
	return text
}

// confirmWindow sends a staff command, confirms its card and waits for the fan-out.
// It returns the summary and the Unix times just before the tap and just after the
// run finished, which bracket the moment the end date was computed.
func (e *staffActionEnv) confirmWindow(command string) (summary string, before, after int64) {
	e.t.Helper()
	e.send(e.issuer, command)
	token, msgID := e.card()
	before = time.Now().Unix()
	e.tap(e.issuer, staffActRunConfirm, token, msgID)
	e.waitRuns()
	after = time.Now().Unix()
	return e.lastEditText(e.staffChat, msgID), before, after
}

// wantUntilNear fails unless until is the Confirm time plus seconds, give or take
// staffUntilSlack.
func wantUntilNear(t *testing.T, what string, until, before, after, seconds int64) {
	t.Helper()
	if until < before+seconds-staffUntilSlack || until > after+seconds+staffUntilSlack {
		t.Fatalf("%s until_date = %d, want Confirm time + %d (between %d and %d)", what, until, seconds, before+seconds, after+seconds)
	}
}

func TestStaffActionTimedBan(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban 4242 2d spamming")
	card := env.lastCardText()
	if !strings.Contains(card, staffMarker("staff_act_duration_days")+" 2") || !strings.Contains(card, "spamming") {
		t.Fatalf("card = %q, want the 2-day label and the reason", card)
	}
	if strings.Contains(card, staffMarker("staff_act_duration_permanent")) {
		t.Fatalf("card = %q, a timed ban must not read permanent", card)
	}
	if calls := env.fake.callsFor("banChatMember"); len(calls) != 0 {
		t.Fatalf("banChatMember calls before Confirm = %d, want 0", len(calls))
	}

	token, msgID := env.card()
	before := time.Now().Unix()
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	after := time.Now().Unix()

	var untils []int64
	for _, group := range []int64{g1, g2} {
		calls := env.callsTo("banChatMember", group)
		if len(calls) != 1 {
			t.Fatalf("banChatMember calls in group %d = %+v, want one", group, calls)
		}
		until := staffParamInt(calls[0].Params, "until_date")
		wantUntilNear(t, "ban", until, before, after, 172800)
		untils = append(untils, until)
		wantBanned(t, env.fake, group, staffTestTarget)
	}
	if untils[0] != untils[1] {
		t.Fatalf("until_date differs between groups: %v, want one value computed once at Confirm", untils)
	}
	summary := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(summary, staffMarker("staff_act_duration_days")+" 2") {
		t.Fatalf("summary = %q, want the duration label in the header", summary)
	}
}

func TestStaffActionTimedLabelIsExact(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"/ban 4242 90m", staffMarker("staff_act_duration_minutes") + " 90"},
		{"/ban 4242 48h", staffMarker("staff_act_duration_hours") + " 48"},
		{"/ban 4242 14d", staffMarker("staff_act_duration_days") + " 14"},
		{"/ban 4242 3w", staffMarker("staff_act_duration_weeks") + " 3"},
		{"/mute 4242 366d", staffMarker("staff_act_duration_days") + " 366"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.send(env.issuer, tc.command)
			if card := env.lastCardText(); !strings.Contains(card, tc.want) {
				t.Fatalf("card = %q, want %q exactly as typed", card, tc.want)
			}
		})
	}
}

func TestStaffActionTimedMute(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	summary, before, after := env.confirmWindow("/tmute 4242 30m")

	for _, group := range []int64{g1, g2} {
		calls := env.callsTo("restrictChatMember", group)
		if len(calls) != 1 {
			t.Fatalf("restrictChatMember calls in group %d = %+v, want one", group, calls)
		}
		wantUntilNear(t, "mute", staffParamInt(calls[0].Params, "until_date"), before, after, 1800)
		if got, want := permsJSON(t, calls[0].Params["permissions"]), permsJSON(t, MutedPermissions); got != want {
			t.Fatalf("permissions in group %d = %s, want MutedPermissions %s", group, got, want)
		}
		if m := wantMember(t, env.fake, group, staffTestTarget, gotgbot.ChatMemberStatusRestricted); m.CanSendMessages {
			t.Fatalf("target in group %d = %+v, want muted", group, m)
		}
	}
	if !strings.Contains(summary, staffMarker("staff_act_duration_minutes")+" 30") {
		t.Fatalf("summary = %q, want the 30-minute label", summary)
	}
}

func TestStaffActionTimedNeverShortens(t *testing.T) {
	t.Run("a shorter ban over a permanent ban is skipped", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})

		summary, _, _ := env.confirmWindow("/ban 4242 1d")

		if writes := env.writes(g1); len(writes) != 0 {
			t.Fatalf("write calls = %+v, want none over a permanent ban", writes)
		}
		wantSkippedLine(t, summary, "Group A", "staff_act_skip_already_banned")
		if m := env.fake.member(g1, staffTestTarget); m.UntilDate != 0 {
			t.Fatalf("ban end = %d, want it still permanent", m.UntilDate)
		}
	})

	t.Run("a longer ban upgrades a shorter one", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		soon := time.Now().Add(time.Hour).Unix()
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: soon})

		summary, before, after := env.confirmWindow("/ban 4242 1d")

		calls := env.callsTo("banChatMember", g1)
		if len(calls) != 1 {
			t.Fatalf("banChatMember calls = %+v, want the upgrade applied", calls)
		}
		wantUntilNear(t, "upgraded ban", staffParamInt(calls[0].Params, "until_date"), before, after, 86400)
		wantDoneLine(t, summary, "Group A")
	})

	t.Run("a shorter ban over a longer timed ban is skipped", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		later := time.Now().Add(48 * time.Hour).Unix()
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: later})

		summary, _, _ := env.confirmWindow("/ban 4242 1d")

		if writes := env.writes(g1); len(writes) != 0 {
			t.Fatalf("write calls = %+v, want none", writes)
		}
		wantSkippedLine(t, summary, "Group A", "staff_act_skip_already_banned")
	})

	t.Run("a longer mute upgrades a shorter one", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		until := time.Now().Add(24 * time.Hour).Unix()
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until})

		summary, before, after := env.confirmWindow("/mute 4242 2d")

		calls := env.callsTo("restrictChatMember", g1)
		if len(calls) != 1 {
			t.Fatalf("restrictChatMember calls = %+v, want the longer mute applied", calls)
		}
		wantUntilNear(t, "upgraded mute", staffParamInt(calls[0].Params, "until_date"), before, after, 172800)
		wantDoneLine(t, summary, "Group A")
	})

	t.Run("a shorter mute over a longer one is skipped", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		until := time.Now().Add(24 * time.Hour).Unix()
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusRestricted, IsMember: true, UntilDate: until})

		summary, _, _ := env.confirmWindow("/mute 4242 1h")

		if writes := env.writes(g1); len(writes) != 0 {
			t.Fatalf("write calls = %+v, want none", writes)
		}
		wantSkippedLine(t, summary, "Group A", "staff_act_skip_already_muted")
		if m := env.fake.member(g1, staffTestTarget); m.UntilDate != until {
			t.Fatalf("mute end = %d, want it unchanged at %d", m.UntilDate, until)
		}
	})
}

func TestStaffActionOverLimitIsPermanent(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g1 := env.groups[0]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban 4242 400d x")
	card := env.lastCardText()
	if !strings.Contains(card, staffMarker("staff_act_duration_over_limit")) {
		t.Fatalf("card = %q, want the over-limit label", card)
	}
	if strings.Contains(card, staffMarker("staff_act_duration_days")) || !strings.Contains(card, "x") {
		t.Fatalf("card = %q, want no day count and the reason", card)
	}

	token, msgID := env.card()
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()

	calls := env.callsTo("banChatMember", g1)
	if len(calls) != 1 {
		t.Fatalf("banChatMember calls = %+v, want one", calls)
	}
	if until := staffParamInt(calls[0].Params, "until_date"); until != 0 {
		t.Fatalf("until_date = %d, want none (permanent), never a clamped value", until)
	}
}

func TestStaffActionLimitBoundary(t *testing.T) {
	t.Run("366d is still timed", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		_, before, after := env.confirmWindow("/ban 4242 366d")

		calls := env.callsTo("banChatMember", g1)
		if len(calls) != 1 {
			t.Fatalf("banChatMember calls = %+v, want one", calls)
		}
		wantUntilNear(t, "366d ban", staffParamInt(calls[0].Params, "until_date"), before, after, 366*86400)
	})

	t.Run("367d is permanent", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		g1 := env.groups[0]
		env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(env.issuer, "/ban 4242 367d")
		if card := env.lastCardText(); !strings.Contains(card, staffMarker("staff_act_duration_over_limit")) {
			t.Fatalf("card = %q, want the over-limit label", card)
		}
		token, msgID := env.card()
		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()
		calls := env.callsTo("banChatMember", g1)
		if len(calls) != 1 || staffParamInt(calls[0].Params, "until_date") != 0 {
			t.Fatalf("banChatMember calls = %+v, want one permanent ban", calls)
		}
	})
}

func TestStaffActionTbanTmuteNeedDuration(t *testing.T) {
	cases := []struct {
		command string
		hint    string
	}{
		{"/tban 4242 spam", "staff_act_hint_need_duration"},
		{"/tmute 4242", "staff_act_hint_need_duration"},
		{"/tban 4242 2D", "staff_act_hint_need_duration"},
		{"/tban 4242 0d", "staff_act_hint_bad_duration"},
		{"/tmute 4242 0m", "staff_act_hint_bad_duration"},
		{"/ban 4242 0d", "staff_act_hint_bad_duration"},
		{"/mute 4242 0h spam", "staff_act_hint_bad_duration"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.send(env.issuer, tc.command)
			env.wantReplyMarker(tc.hint)
			if calls := env.fake.callsFor("getChatMember"); len(calls) != 0 {
				t.Fatalf("getChatMember calls = %d, want none before a card exists", len(calls))
			}
		})
	}
}

func TestStaffActionTbanTmuteWork(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g1 := env.groups[0]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	summary, before, after := env.confirmWindow("/tban 4242 1w flooding")

	calls := env.callsTo("banChatMember", g1)
	if len(calls) != 1 {
		t.Fatalf("banChatMember calls = %+v, want one", calls)
	}
	wantUntilNear(t, "tban", staffParamInt(calls[0].Params, "until_date"), before, after, 604800)
	if !strings.Contains(summary, staffMarker("staff_act_duration_weeks")+" 1") || !strings.Contains(summary, "flooding") {
		t.Fatalf("summary = %q, want the week label and the reason", summary)
	}
}

func TestStaffActionDurationTokenRules(t *testing.T) {
	cases := []struct {
		command    string
		wantReason string
	}{
		{"/ban 4242 2D spam", "2D spam"},
		{"/ban 4242 12 times", "12 times"},
		{"/ban 4242 spam 2d", "spam 2d"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			g1 := env.groups[0]
			env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

			env.send(env.issuer, tc.command)
			card := env.lastCardText()
			if !strings.Contains(card, staffMarker("staff_act_duration_permanent")) || !strings.Contains(card, tc.wantReason) {
				t.Fatalf("card = %q, want permanent with the reason %q so the misparse is visible", card, tc.wantReason)
			}
			token, msgID := env.card()
			env.tap(env.issuer, staffActRunConfirm, token, msgID)
			env.waitRuns()
			calls := env.callsTo("banChatMember", g1)
			if len(calls) != 1 || staffParamInt(calls[0].Params, "until_date") != 0 {
				t.Fatalf("banChatMember calls = %+v, want one permanent ban", calls)
			}
		})
	}
}

func TestStaffActionKickIgnoresDuration(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	env.send(env.issuer, "/kick 4242 2d spam")
	card := env.lastCardText()
	for _, key := range []string{"staff_act_duration_permanent", "staff_act_duration_days"} {
		if strings.Contains(card, staffMarker(key)) {
			t.Fatalf("card = %q, a kick has no duration but shows %s", card, key)
		}
	}
	if !strings.Contains(card, "2d spam") {
		t.Fatalf("card = %q, want the whole text after the target as the reason", card)
	}
}
