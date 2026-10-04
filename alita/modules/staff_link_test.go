//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

const linkOwnerID int64 = 6161

// linkEnv is a scripted bot, a Staff Group and a supergroup that both have
// linkOwnerID as creator. The Staff Group row exists; the supergroup is unlinked.
type linkEnv struct {
	client  *staffBotClient
	bot     *gotgbot.Bot
	staffID int64
	groupID int64
}

func newLinkEnv(t *testing.T) *linkEnv {
	t.Helper()
	withStaffLocale(t)

	env := &linkEnv{
		client:  newStaffBotClient(),
		staffID: uniqueModuleChatID(),
		groupID: uniqueModuleChatID(),
	}
	env.bot = newModuleTestBot(env.client.moduleBotClient)
	env.bot.BotClient = env.client
	env.client.setCreator(env.staffID, linkOwnerID)
	env.client.setCreator(env.groupID, linkOwnerID)
	staffCleanup(t, env.staffID, env.groupID)
	if _, err := staff.CreateStaffGroup(env.staffID, linkOwnerID, "Staff HQ"); err != nil {
		t.Fatalf("create Staff Group: %v", err)
	}
	return env
}

func (e *linkEnv) groupChat(title string) gotgbot.Chat {
	return gotgbot.Chat{Id: e.groupID, Type: "supergroup", Title: title}
}

// sendLink runs /linkstaff in chat as userID with the given message text.
func (e *linkEnv) sendLink(t *testing.T, chat gotgbot.Chat, userID int64, text string) {
	t.Helper()
	user := gotgbot.User{Id: userID, FirstName: "User"}
	ctx := newModuleMessageContext(e.bot, chat, user, text)
	if err := runStaffCommand(t, e.bot, ctx, linkStaffDesc, staffModule.linkStaff); err != ext.EndGroups {
		t.Fatalf("/linkstaff returned %v, want ext.EndGroups", err)
	}
}

// callsToChat returns the recorded calls of method addressed to chatID.
func callsToChat(client *staffBotClient, method string, chatID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range client.callsFor(method) {
		if fmt.Sprint(call.Params["chat_id"]) == strconv.FormatInt(chatID, 10) {
			matched = append(matched, call)
		}
	}
	return matched
}

// textsToChat returns the text of every sendMessage addressed to chatID.
func textsToChat(client *staffBotClient, chatID int64) []string {
	var texts []string
	for _, call := range callsToChat(client, "sendMessage", chatID) {
		texts = append(texts, fmt.Sprint(call.Params["text"]))
	}
	return texts
}

func (e *linkEnv) wantLink(t *testing.T, health string) *models.StaffGroupLink {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(e.groupID)
	if err != nil || link == nil {
		t.Fatalf("link of group = (%+v, %v), want a link", link, err)
	}
	if link.OwnerUserID != linkOwnerID || link.StaffChatID != e.staffID || link.Health != health {
		t.Fatalf("link = %+v, want owner %d, staff %d, health %q", link, linkOwnerID, e.staffID, health)
	}
	return link
}

func TestLinkStaffTracer(t *testing.T) {
	env := newLinkEnv(t)
	const title = "Group <b>One</b>"

	env.sendLink(t, env.groupChat(title), linkOwnerID, fmt.Sprintf("/linkstaff %d", env.staffID))

	deletes := callsToChat(env.client, "deleteMessage", env.groupID)
	if len(deletes) != 1 || fmt.Sprint(deletes[0].Params["message_id"]) != "101" {
		t.Fatalf("deleteMessage calls in the linked group = %+v, want one for the command message", deletes)
	}
	env.wantLink(t, models.StaffHealthOK)

	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_link_done")) {
		t.Fatalf("Staff Group notices = %q, want exactly one staff_link_done", notices)
	}
	if !strings.Contains(notices[0], "Group &lt;b&gt;One&lt;/b&gt;") || !strings.Contains(notices[0], strconv.FormatInt(env.groupID, 10)) {
		t.Errorf("notice %q must carry the escaped title and the group ID", notices[0])
	}
	if strings.Contains(notices[0], "staff-link-warn") {
		t.Errorf("notice %q carries a warning although the bot has every right", notices[0])
	}
	if sent := callsToChat(env.client, "sendMessage", env.groupID); len(sent) != 0 {
		t.Fatalf("%d messages were sent to the linked group, want none (D-06, D-07)", len(sent))
	}

	staffCtx := newModuleMessageContext(env.bot, staffSupergroup(env.staffID), gotgbot.User{Id: linkOwnerID, FirstName: "Owner"}, "/staff")
	if err := runStaffCommand(t, env.bot, staffCtx, staffDesc, staffModule.staffPanel); err != ext.EndGroups {
		t.Fatalf("/staff returned %v, want ext.EndGroups", err)
	}
	panel := textsToChat(env.client, env.staffID)
	if len(panel) != 2 {
		t.Fatalf("Staff Group messages = %q, want the notice and the panel", panel)
	}
	for _, want := range []string{
		staffMarker("staff_panel_links_header"),
		"Group &lt;b&gt;One&lt;/b&gt;",
		"<code>" + strconv.FormatInt(env.groupID, 10) + "</code>",
	} {
		if !strings.Contains(panel[1], want) {
			t.Errorf("/staff text %q missing %q", panel[1], want)
		}
	}
	if strings.Contains(panel[1], staffMarker("staff_panel_no_links")) {
		t.Errorf("/staff text %q still says no groups are linked", panel[1])
	}
}

func TestLinkStaffWarnsWhenBotLacksRights(t *testing.T) {
	cases := []struct {
		role       string
		wantHealth string
		wantKey    string
	}{
		{staffRoleMember, models.StaffHealthBotNotAdmin, "staff_link_warn_bot_not_admin"},
		{staffRoleAdminNoRestrict, models.StaffHealthBotCannotRestrict, "staff_link_warn_bot_cannot_restrict"},
		{staffRoleAbsent, models.StaffHealthBotMissing, "staff_link_warn_bot_missing"},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			env := newLinkEnv(t)
			env.client.botRole[env.groupID] = tc.role

			env.sendLink(t, env.groupChat("Weak Bot Group"), linkOwnerID, fmt.Sprintf("/linkstaff %d", env.staffID))

			env.wantLink(t, tc.wantHealth)
			notices := textsToChat(env.client, env.staffID)
			if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_link_done")) ||
				!strings.Contains(notices[0], staffMarker(tc.wantKey)) {
				t.Fatalf("Staff Group notices = %q, want one staff_link_done with %s", notices, tc.wantKey)
			}
			if got := strings.Count(notices[0], "staff-link-warn"); got != 1 {
				t.Errorf("notice %q carries %d warning lines, want exactly one", notices[0], got)
			}
		})
	}
}

func TestStaffHealthFromBot(t *testing.T) {
	admin := gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true}
	cases := []struct {
		name   string
		member gotgbot.MergedChatMember
		res    chat_status.BotMemberResult
		want   string
	}{
		{"admin with restrict", admin, chat_status.BotMemberFound, models.StaffHealthOK},
		{"admin without restrict", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusAdministrator}, chat_status.BotMemberFound, models.StaffHealthBotCannotRestrict},
		{"plain member", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusMember}, chat_status.BotMemberFound, models.StaffHealthBotNotAdmin},
		{"left", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusLeft}, chat_status.BotMemberFound, models.StaffHealthBotMissing},
		{"kicked", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusKicked}, chat_status.BotMemberFound, models.StaffHealthBotMissing},
		{"missing result", gotgbot.MergedChatMember{}, chat_status.BotMemberMissing, models.StaffHealthBotMissing},
		{"unknown result stays ok", gotgbot.MergedChatMember{}, chat_status.BotMemberUnknown, models.StaffHealthOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := staffHealthFromBot(tc.member, tc.res); got != tc.want {
				t.Fatalf("staffHealthFromBot = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseStaffChatIDArg(t *testing.T) {
	valid := map[string]int64{
		"-1001234567890": -1001234567890,
		"-5":             -5,
	}
	for arg, want := range valid {
		if got, ok := parseStaffChatIDArg(arg); !ok || got != want {
			t.Errorf("parseStaffChatIDArg(%q) = (%d, %v), want (%d, true)", arg, got, ok, want)
		}
	}
	for _, arg := range []string{
		"", "-", "abc", "123", "0", "-0", "--5", "+5", "-12x", " -5", "-5 ", "-1.5",
		"-99999999999999999999999", strings.Repeat("9", 40),
	} {
		if got, ok := parseStaffChatIDArg(arg); ok {
			t.Errorf("parseStaffChatIDArg(%q) = (%d, true), want a rejection", arg, got)
		}
	}
}
