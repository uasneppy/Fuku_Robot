//go:build testtools

package modules

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// wantFailedLine fails unless the group's line is a failure for the given reason key.
func wantFailedLine(t *testing.T, summary, title, reasonKey string) string {
	t.Helper()
	line := summaryLine(t, summary, title)
	if !strings.Contains(line, staffMarker("staff_act_line_failed")) || !strings.Contains(line, staffMarker(reasonKey)) {
		t.Fatalf("line for %s = %q, want failed with %s", title, line, reasonKey)
	}
	return line
}

// repeatErr returns n copies of err, for scripting a call that never recovers.
func repeatErr(err error, n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}
	return errs
}

// seedMembers puts the target in every group as a plain member.
func seedMembers(env *staffActionEnv) {
	for _, group := range env.groups {
		env.fake.setMember(group, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	}
}

func TestStaffActionRetriesOn429(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	g1 := env.groups[0]
	env.fake.script("banChatMember", g1, staffFake429(1), staffFake429(1))

	summary, _ := env.runStaffCommand("/ban 4242 1h flooding")

	wantDoneLine(t, summary, "Group A")
	wantDoneLine(t, summary, "Group B")
	calls := env.callsTo("banChatMember", g1)
	if len(calls) != 3 {
		t.Fatalf("banChatMember calls for group A = %d, want 3 (two 429s, then success)", len(calls))
	}
	if staffParamInt(calls[0].Params, "until_date") == 0 {
		t.Fatalf("until_date = 0, want the timed ban's end date")
	}
	for i, call := range calls[1:] {
		if !reflect.DeepEqual(call.Params, calls[0].Params) {
			t.Fatalf("retry %d params = %v, want the same as the first call %v", i+1, call.Params, calls[0].Params)
		}
	}
	wantMember(t, env.fake, g1, staffTestTarget, gotgbot.ChatMemberStatusKicked)
}

func TestStaffActionRateLimitedFails(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	g1 := env.groups[0]
	env.fake.script("banChatMember", g1, repeatErr(staffFake429(1), 20)...)

	summary, _ := env.runStaffCommand("/ban 4242")

	wantFailedLine(t, summary, "Group A", "staff_act_fail_rate_limited")
	wantDoneLine(t, summary, "Group B")
	if calls := env.callsTo("banChatMember", g1); len(calls) != 4 {
		t.Fatalf("banChatMember calls for group A = %d, want 4 (one try and 3 retries)", len(calls))
	}
}

func TestStaffActionFailureClassification(t *testing.T) {
	cases := []struct {
		name       string
		botRole    string
		failure    error
		wantMarker string
		wantDetail string
	}{
		{
			name:       "bot is only a member",
			botRole:    staffRoleMember,
			failure:    staffFakeError(400, "Bad Request: not enough rights"),
			wantMarker: "staff_act_fail_bot_not_admin",
		},
		{
			name:       "bot is an admin without the restrict right",
			botRole:    staffRoleAdminNoRestrict,
			failure:    staffFakeError(400, "Bad Request: not enough rights"),
			wantMarker: "staff_act_fail_bot_no_rights",
		},
		{
			name:       "bot was kicked",
			botRole:    staffRoleAbsent,
			failure:    staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"),
			wantMarker: "staff_act_fail_group_not_found",
		},
		{
			name:       "anything else is shown as Telegram's own text",
			botRole:    staffRoleAdmin,
			failure:    staffFakeError(400, "Bad Request: method is available only in supergroups"),
			wantMarker: "staff_act_fail_telegram",
			wantDetail: "method is available only in supergroups",
		},
		{
			name:       "Telegram's text is escaped",
			botRole:    staffRoleAdmin,
			failure:    staffFakeError(400, "Bad Request: no <b>such</b> thing"),
			wantMarker: "staff_act_fail_telegram",
			wantDetail: "no &lt;b&gt;such&lt;/b&gt; thing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			seedMembers(env)
			g := env.groups[0]
			env.fake.setBotRole(g, tc.botRole)
			env.fake.script("banChatMember", g, tc.failure)

			summary, _ := env.runStaffCommand("/ban 4242")

			line := wantFailedLine(t, summary, "Group A", tc.wantMarker)
			if tc.wantDetail != "" && !strings.Contains(line, tc.wantDetail) {
				t.Fatalf("line = %q, want Telegram's text %q", line, tc.wantDetail)
			}
			if calls := env.callsTo("banChatMember", g); len(calls) != 1 {
				t.Fatalf("banChatMember calls = %d, want 1 (a 400 or 403 is never retried)", len(calls))
			}
			if probes := env.memberLookups(g, staffTestBotID); probes != 1 {
				t.Fatalf("probes of the bot's own rights = %d, want 1", probes)
			}
		})
	}
}

func TestStaffActionLookupClassification(t *testing.T) {
	t.Run("a kicked bot is a group that was not found", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		seedMembers(env)
		g := env.groups[0]
		env.fake.setBotRole(g, staffRoleAbsent)
		env.fake.script("getChatMember", g, staffFakeError(403, "Forbidden: bot was kicked from the supergroup chat"))

		summary, _ := env.runStaffCommand("/ban 4242")

		wantFailedLine(t, summary, "Group A", "staff_act_fail_group_not_found")
		if writes := env.writes(g); len(writes) != 0 {
			t.Fatalf("write calls = %+v, want none", writes)
		}
	})

	t.Run("a lookup failure with the bot in place stays a lookup failure", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		seedMembers(env)
		g := env.groups[0]
		env.fake.script("getChatMember", g, staffFakeError(400, "Bad Request: user not found"))

		summary, _ := env.runStaffCommand("/ban 4242")

		wantFailedLine(t, summary, "Group A", "staff_act_fail_lookup")
	})
}

func TestStaffActionRecheckPaced(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	g1 := env.groups[0]
	env.fake.script("getChatAdministrators", g1, staffFake429(1))

	summary, _ := env.runStaffCommand("/ban 4242")

	wantDoneLine(t, summary, "Group A")
	wantDoneLine(t, summary, "Group B")
	if calls := env.callsTo("getChatAdministrators", g1); len(calls) != 2 {
		t.Fatalf("getChatAdministrators calls for group A = %d, want 2 (the 429, then the retry)", len(calls))
	}
	links, err := staff.ListLinksByStaffFresh(env.staffChat)
	if err != nil || len(links) != 2 {
		t.Fatalf("links = %d, err = %v, want both links kept", len(links), err)
	}
}

func TestStaffActionRecheckRateLimited(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	g1 := env.groups[0]
	env.fake.script("getChatAdministrators", g1, repeatErr(staffFake429(1), 20)...)

	summary, _ := env.runStaffCommand("/ban 4242")

	line := wantFailedLine(t, summary, "Group A", "staff_act_fail_rate_limited")
	if strings.Contains(line, staffMarker("staff_act_fail_owner_unknown")) {
		t.Fatalf("line = %q, want rate limited and not 'could not verify the group owner'", line)
	}
	wantDoneLine(t, summary, "Group B")
	if writes := env.writes(g1); len(writes) != 0 {
		t.Fatalf("write calls in group A = %+v, want none after a failed owner check", writes)
	}
	links, err := staff.ListLinksByStaffFresh(env.staffChat)
	if err != nil || len(links) != 2 {
		t.Fatalf("links = %d, err = %v, want both links kept: a rate limit never removes one", len(links), err)
	}
}

func TestStaffActionWorkersBounded(t *testing.T) {
	env := newStaffActionEnv(t, 12)
	seedMembers(env)
	for _, group := range env.groups {
		env.fake.setDelay("getChatMember", group, 30*time.Millisecond)
	}

	summary, _ := env.runStaffCommand("/ban 4242")

	for i := range env.groups {
		wantDoneLine(t, summary, "Group "+string(rune('A'+i)))
	}
	if peak := env.fake.maxConcurrent(); peak < 2 || peak > 4 {
		t.Fatalf("most requests in flight at once = %d, want between 2 and 4", peak)
	}
}

func TestStaffActionWorkerPanicReported(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	seedMembers(env)
	env.fake.setPanic("getChatMember", env.groups[1])

	summary, _ := env.runStaffCommand("/ban 4242")

	wantDoneLine(t, summary, "Group A")
	wantFailedLine(t, summary, "Group B", "staff_act_fail_internal")
	wantDoneLine(t, summary, "Group C")
}
