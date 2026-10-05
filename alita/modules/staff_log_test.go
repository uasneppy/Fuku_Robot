//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/actionlog"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
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

// wantSingleGroupDone fails unless the card of msgID ended with its only linked
// group (Group A) applied: a done line, a done tally and a done record row.
func wantSingleGroupDone(t *testing.T, env *staffActionEnv, msgID int64) {
	t.Helper()
	final := env.lastEditText(env.staffChat, msgID)
	if !hasLinePrefix(final, "✅ Group A") {
		t.Fatalf("final summary has no done line for Group A:\n%s", final)
	}
	if want := staffSummaryTallyLine(1, 0, 0); !strings.Contains(final, want) {
		t.Fatalf("final summary tally is not %q:\n%s", want, final)
	}
	_, rows := recordOfCard(t, msgID)
	if len(rows) != 1 || rows[0].Outcome != models.StaffActionOutcomeDone {
		t.Fatalf("record rows = %+v, want exactly one done row", rows)
	}
}

func TestStaffActionLogFailureKeepsDone(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)
	env.fake.script("sendMessage", channel, staffFakeError(403, "Forbidden: bot is not a member of the channel chat"))

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	if got := len(env.callsTo("sendMessage", channel)); got != 1 {
		t.Fatalf("log post attempts = %d, want 1 (a 403 is not retried)", got)
	}
	wantSingleGroupDone(t, env, msgID)
}

func TestStaffActionLogPaced(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)
	env.fake.script("sendMessage", channel, staffFake429(1))

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	if got := len(env.callsTo("sendMessage", channel)); got != 2 {
		t.Fatalf("log post attempts = %d, want 2 (the 429 is waited out and the post retried)", got)
	}
	if got := env.logTextsTo(channel); len(got) != 1 {
		t.Fatalf("channel received %d posts, want exactly 1: %q", len(got), got)
	}
	wantSingleGroupDone(t, env, msgID)
}

func TestStaffActionLogRateLimitedKeepsDone(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)
	// One more 429 than the pacer retries, so the post is given up.
	env.fake.script("sendMessage", channel,
		staffFake429(1), staffFake429(1), staffFake429(1), staffFake429(1))

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	if got := env.logTextsTo(channel); len(got) != 0 {
		t.Fatalf("channel received %q, want no post after the retries ran out", got)
	}
	if got := len(env.callsTo("sendMessage", channel)); got < 2 {
		t.Fatalf("log post attempts = %d, want the post retried after a 429", got)
	}
	wantSingleGroupDone(t, env, msgID)
}

func TestStaffActionLogCategoryOff(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	channel := uniqueModuleChatID()
	setStaffLogChannel(t, env.groups[0], channel)
	if err := logchannels.SetCategory(env.groups[0], logchannels.CategoryAdmin, false); err != nil {
		t.Fatalf("switch the admin category off: %v", err)
	}

	_, msgID := env.startRun("/ban 4242 2d spamming")
	env.waitRuns()

	if got := len(env.callsTo("sendMessage", channel)); got != 0 {
		t.Fatalf("log channel got %d send attempts, want none with the admin category off", got)
	}
	wantSingleGroupDone(t, env, msgID)
}

func TestStaffActionLogReasonCapped(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	reason := `<b>&"x` + strings.Repeat("y", 400-utf8.RuneCountInString(`<b>&"x`))
	if utf8.RuneCountInString(reason) != 400 {
		t.Fatalf("test reason has %d runes, want 400", utf8.RuneCountInString(reason))
	}

	card := &staffActionCard{
		Kind:       staffKindBan,
		Issuer:     77,
		IssuerName: "Issuer",
		Target:     4242,
		TargetName: "Target",
		Reason:     reason,
	}
	text := composeStaffActionLog(tr, card)
	if !strings.Contains(text, "&lt;b&gt;&amp;&#34;x") {
		t.Fatalf("log text does not carry the escaped reason:\n%s", text)
	}
	if strings.Contains(text, "<b>&") {
		t.Fatalf("a raw tag from the reason reached the log text:\n%s", text)
	}

	segment := html.UnescapeString(staffLogReason(tr, reason))
	if got := utf8.RuneCountInString(segment); got != 301 {
		t.Fatalf("reason segment has %d runes, want 300 plus the ellipsis", got)
	}
	if want := string([]rune(reason)[:300]) + "…"; segment != want {
		t.Fatal("reason segment is not the first 300 runes plus an ellipsis")
	}
}

func TestActionLogDestination(t *testing.T) {
	withMiniredis(t)
	chatID, channelID := uniqueModuleChatID(), uniqueModuleChatID()
	setStaffLogChannel(t, chatID, channelID)
	chat := &gotgbot.Chat{Id: chatID, Title: "Title", Type: "supergroup"}

	gotChannel, header, ok := actionlog.Destination(chat, logchannels.CategoryAdmin)
	if !ok || gotChannel != channelID {
		t.Fatalf("Destination = channel %d ok=%v, want channel %d and true", gotChannel, ok, channelID)
	}
	if want := fmt.Sprintf("<b>Title</b> (<code>%d</code>)\n", chatID); header != want {
		t.Fatalf("header = %q, want %q", header, want)
	}

	if err := logchannels.SetCategory(chatID, logchannels.CategoryAdmin, false); err != nil {
		t.Fatalf("switch the admin category off: %v", err)
	}
	if _, _, ok := actionlog.Destination(chat, logchannels.CategoryAdmin); ok {
		t.Fatal("Destination reported a post with the admin category off")
	}

	asChannel := &gotgbot.Chat{Id: chatID, Title: "Title", Type: "channel"}
	if _, _, ok := actionlog.Destination(asChannel, logchannels.CategoryUser); ok {
		t.Fatal("Destination reported a post for a chat of type channel")
	}
	if _, _, ok := actionlog.Destination(nil, logchannels.CategoryAdmin); ok {
		t.Fatal("Destination reported a post for a nil chat")
	}
}

func TestActionLogAdminUnchanged(t *testing.T) {
	withMiniredis(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chatID, channelID := uniqueModuleChatID(), uniqueModuleChatID()
	setStaffLogChannel(t, chatID, channelID)
	chat := &gotgbot.Chat{Id: chatID, Title: "Title", Type: "supergroup"}
	actor := &gotgbot.User{Id: 55, FirstName: "Act<or>"}

	actionlog.Admin(bot, chat, actor, "BAN", 42, "why")

	var sent []moduleBotCall
	for _, call := range client.callsFor("sendMessage") {
		if fmt.Sprint(call.Params["chat_id"]) == strconv.FormatInt(channelID, 10) {
			sent = append(sent, call)
		}
	}
	if len(sent) != 1 {
		t.Fatalf("sendMessage calls to the log channel = %d, want 1", len(sent))
	}
	want := fmt.Sprintf("<b>Title</b> (<code>%d</code>)\n", chatID) +
		"#BAN\nAdmin: " + formatting.MentionHtml(actor.Id, actor.FirstName) + "\nUser: <code>42</code>\nReason: why"
	if got := fmt.Sprint(sent[0].Params["text"]); got != want {
		t.Fatalf("admin log text = %q, want %q", got, want)
	}
}
