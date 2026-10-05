//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// postsWith returns the texts among texts that contain needle.
func postsWith(texts []string, needle string) []string {
	var matched []string
	for _, text := range texts {
		if strings.Contains(text, needle) {
			matched = append(matched, text)
		}
	}
	return matched
}

func TestStaffUndoLogPosts(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	bob := env.undoPresser("Bob")
	channels := []int64{uniqueModuleChatID(), uniqueModuleChatID(), uniqueModuleChatID()}
	for i, group := range env.groups {
		setStaffLogChannel(t, group, channels[i])
	}
	groupB := env.groups[1]
	// The issuer is only a plain member in group C, so the ban applies in A and B.
	env.fake.setMember(env.groups[2], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	msgID, action := env.ranStaffCommand("/ban 4242 spamming")
	// In group B a group admin lifted the ban afterwards, so Bob's undo skips it.
	env.fake.setMember(groupB, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
	env.runUndo(bob, msgID, action.ID)

	postsA := env.logTextsTo(channels[0])
	if got := postsWith(postsA, "#STAFF_BAN"); len(got) != 1 {
		t.Fatalf("channel A has %d #STAFF_BAN posts, want the original one: %q", len(got), postsA)
	}
	undos := postsWith(postsA, "#STAFF_UNDO")
	if len(undos) != 1 {
		t.Fatalf("channel A has %d #STAFF_UNDO posts, want exactly 1: %q", len(undos), postsA)
	}
	post := undos[0]
	wantHeader := fmt.Sprintf("<b>Group A</b> (<code>%d</code>)\n", env.groups[0])
	if !strings.HasPrefix(post, wantHeader) {
		t.Fatalf("undo post does not start with the group header %q:\n%s", wantHeader, post)
	}
	for _, want := range []string{
		staffMarker("staff_act_name_unban"),
		staffMarker("staff_log_admin_label"),
		fmt.Sprintf("tg://user?id=%d", bob.Id),
		staffMarker("staff_log_user_label"),
		"(<code>4242</code>)",
		staffMarker("staff_log_undoes"),
		staffMarker("staff_act_name_ban"),
		html.EscapeString(env.issuer.FirstName),
		staffMarker("staff_log_via_staff_group"),
	} {
		if !strings.Contains(post, want) {
			t.Fatalf("undo post is missing %q:\n%s", want, post)
		}
	}
	if strings.Contains(post, fmt.Sprintf("tg://user?id=%d", env.issuer.Id)) {
		t.Fatalf("the undo post mentions the original issuer as its admin:\n%s", post)
	}

	postsB := env.logTextsTo(channels[1])
	if got := postsWith(postsB, "#STAFF_UNDO"); len(got) != 0 {
		t.Fatalf("channel B received %q, want no undo post for a group the undo skipped", got)
	}
	if got := postsWith(postsB, "#STAFF_BAN"); len(got) != 1 {
		t.Fatalf("channel B has %d #STAFF_BAN posts, want the original one", len(got))
	}
	if got := env.logTextsTo(channels[2]); len(got) != 0 {
		t.Fatalf("channel C received %q, want nothing: neither the ban nor the undo applied there", got)
	}

	staffID := strconv.FormatInt(env.staffChat, 10)
	for _, channel := range channels {
		for _, text := range env.logTextsTo(channel) {
			if strings.Contains(text, "Staff HQ") || strings.Contains(text, staffID) {
				t.Fatalf("a log post reveals the Staff Group:\n%s", text)
			}
		}
	}
	for _, group := range env.groups {
		if got := env.fake.sentTo(group); len(got) != 0 {
			t.Fatalf("linked group %d's own chat received %d messages, want none", group, len(got))
		}
	}
}

func TestStaffUndoLogFailureKeepsDone(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	bob := env.undoPresser("Bob")
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)

	msgID, action := env.ranStaffCommand("/ban 4242")
	// Queued after the original run finished, so it hits the undo post.
	env.fake.script("sendMessage", channel, staffFakeError(403, "Forbidden: bot is not a member of the channel chat"))
	cardMsgID := env.runUndo(bob, msgID, action.ID)

	if got := len(env.callsTo("sendMessage", channel)); got != 2 {
		t.Fatalf("log post attempts = %d, want 2 (the ban post and the failed undo post)", got)
	}
	if got := postsWith(env.logTextsTo(channel), "#STAFF_UNDO"); len(got) != 0 {
		t.Fatalf("channel received %q, want the undo post to have failed", got)
	}
	wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_unbanned")
	_, rows := recordOfCard(t, msgID)
	if rows[0].UndoOutcome != models.StaffActionOutcomeDone {
		t.Fatalf("undo outcome = %q, want done: a failed post never changes the result", rows[0].UndoOutcome)
	}
}

func TestStaffUndoLogCategoryOff(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	bob := env.undoPresser("Bob")
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)

	msgID, action := env.ranStaffCommand("/ban 4242")
	if err := logchannels.SetCategory(env.groups[0], logchannels.CategoryAdmin, false); err != nil {
		t.Fatalf("switch the admin category off: %v", err)
	}
	cardMsgID := env.runUndo(bob, msgID, action.ID)

	if got := len(env.callsTo("sendMessage", channel)); got != 1 {
		t.Fatalf("log channel got %d send attempts, want only the ban post made before the category was off", got)
	}
	if got := postsWith(env.logTextsTo(channel), "#STAFF_UNDO"); len(got) != 0 {
		t.Fatalf("channel received %q, want no undo post with the admin category off", got)
	}
	wantUndoLine(t, env.lastEditText(env.staffChat, cardMsgID), "Group A", "✅ Group A", "staff_undo_unbanned")
}

func TestStaffReverseKind(t *testing.T) {
	cases := map[staffActionKind]staffActionKind{
		staffKindBan:    staffKindUnban,
		staffKindMute:   staffKindUnmute,
		staffKindUnban:  staffKindBan,
		staffKindUnmute: staffKindMute,
	}
	for kind, want := range cases {
		if got := staffReverseKind(kind); got != want {
			t.Errorf("staffReverseKind(%q) = %q, want %q", kind, got, want)
		}
	}
}
