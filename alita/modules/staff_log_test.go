//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/logchannels"
)

// setStaffLogChannel gives groupID a log channel with every category on, and
// removes it again when the test ends.
func setStaffLogChannel(t *testing.T, groupID, channelID int64) {
	t.Helper()
	if err := logchannels.Set(groupID, "Group", channelID); err != nil {
		t.Fatalf("set log channel %d for group %d: %v", channelID, groupID, err)
	}
	t.Cleanup(func() { _ = logchannels.Unset(groupID) })
}

// logTextsTo returns the text of every sendMessage the fake answered for chatID.
func (e *staffActionEnv) logTextsTo(chatID int64) []string {
	var texts []string
	for _, sent := range e.fake.sentTo(chatID) {
		texts = append(texts, fmt.Sprint(sent.Params["text"]))
	}
	return texts
}

// callIndex returns the position in the fake's single, ordered call log of the
// first call of method addressed to chatID, or -1.
func (e *staffActionEnv) callIndex(method string, chatID int64) int {
	client := e.fake.staffBotClient.moduleBotClient
	client.mu.Lock()
	defer client.mu.Unlock()
	want := strconv.FormatInt(chatID, 10)
	for i, call := range client.calls {
		if call.Method == method && fmt.Sprint(call.Params["chat_id"]) == want {
			return i
		}
	}
	return -1
}

func TestStaffActionLogPosts(t *testing.T) {
	env := newStaffActionEnv(t, 3)
	channel0, channel1 := uniqueModuleChatID(), uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel0)
	setStaffLogChannel(t, env.groups[1], channel1)
	// The issuer is only a plain member in group B, so group B is skipped.
	env.fake.setMember(env.groups[1], env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	posts := env.logTextsTo(channel0)
	if len(posts) != 1 {
		t.Fatalf("channel of group A received %d posts, want 1: %q", len(posts), posts)
	}
	post := posts[0]
	wantHeader := fmt.Sprintf("<b>Group A</b> (<code>%d</code>)\n", env.groups[0])
	if !strings.HasPrefix(post, wantHeader) {
		t.Fatalf("post does not start with the group header %q:\n%s", wantHeader, post)
	}
	for _, want := range []string{
		"#STAFF_BAN",
		staffMarker("staff_log_admin_label"),
		fmt.Sprintf("tg://user?id=%d", env.issuer.Id),
		staffMarker("staff_log_user_label"),
		"(<code>4242</code>)",
		staffMarker("staff_act_duration_days"),
		"spamming",
		staffMarker("staff_log_via_staff_group"),
	} {
		if !strings.Contains(post, want) {
			t.Fatalf("post is missing %q:\n%s", want, post)
		}
	}

	if got := env.logTextsTo(channel1); len(got) != 0 {
		t.Fatalf("channel of the skipped group B received %q, want nothing", got)
	}
	// Group C has no log channel, so nothing is sent for it anywhere.
	for _, group := range env.groups {
		if got := env.fake.sentTo(group); len(got) != 0 {
			t.Fatalf("linked group %d's own chat received %d messages, want none", group, len(got))
		}
	}
	staffID := strconv.FormatInt(env.staffChat, 10)
	for _, channel := range []int64{channel0, channel1} {
		for _, text := range env.logTextsTo(channel) {
			if strings.Contains(text, "Staff HQ") || strings.Contains(text, staffID) {
				t.Fatalf("a log post reveals the Staff Group:\n%s", text)
			}
		}
	}

	ban := env.callIndex("banChatMember", env.groups[0])
	send := env.callIndex("sendMessage", channel0)
	if ban < 0 || send < 0 || ban > send {
		t.Fatalf("banChatMember to group A is call %d and the log post is call %d, want the ban first", ban, send)
	}
}

func TestStaffActionLogSharedChannel(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	shared := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], shared)
	setStaffLogChannel(t, env.groups[1], shared)

	env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	posts := env.logTextsTo(shared)
	if len(posts) != 2 {
		t.Fatalf("shared channel received %d posts, want one per group: %q", len(posts), posts)
	}
	for i, title := range []string{"Group A", "Group B"} {
		header := fmt.Sprintf("<b>%s</b> (<code>%d</code>)\n", title, env.groups[i])
		found := 0
		for _, post := range posts {
			if strings.HasPrefix(post, header) {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("%d posts start with %q, want exactly 1: %q", found, header, posts)
		}
	}
}

func TestStaffActionLogNoneWhenNothingApplied(t *testing.T) {
	t.Run("nothing applied", func(t *testing.T) {
		env := newStaffActionEnv(t, 2)
		channels := []int64{uniqueModuleChatID(), uniqueModuleChatID()}
		for i, group := range env.groups {
			setStaffLogChannel(t, group, channels[i])
			env.fake.setMember(group, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		}

		env.startRun("/ban 4242 2d spamming")
		env.waitRuns()

		for _, channel := range channels {
			if got := env.logTextsTo(channel); len(got) != 0 {
				t.Fatalf("channel %d received %q, want nothing when no group applied the action", channel, got)
			}
		}
	})

	t.Run("no reason", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		channel := uniqueModuleChatID()
		setStaffLogChannel(t, env.groups[0], channel)

		env.startRun("/ban 4242")
		env.waitRuns()

		posts := env.logTextsTo(channel)
		if len(posts) != 1 {
			t.Fatalf("channel received %d posts, want 1: %q", len(posts), posts)
		}
		if !strings.Contains(posts[0], staffMarker("staff_act_no_reason")) {
			t.Fatalf("post of an action without a reason is missing %q:\n%s", staffMarker("staff_act_no_reason"), posts[0])
		}
	})
}
