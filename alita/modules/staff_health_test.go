//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// botUser is the bot as it appears in my_chat_member payloads.
var botUser = gotgbot.User{Id: staffTestBotID, IsBot: true, FirstName: "Bot"}

// myChatMemberContext builds an update whose my_chat_member says the bot's status
// in chat became newMember.
func myChatMemberContext(bot *gotgbot.Bot, chat gotgbot.Chat, newMember gotgbot.ChatMember) *ext.Context {
	return ext.NewContext(bot, &gotgbot.Update{
		UpdateId: 6,
		MyChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          chat,
			From:          gotgbot.User{Id: 424242, FirstName: "Someone"},
			Date:          1,
			OldChatMember: gotgbot.ChatMemberAdministrator{User: botUser, CanRestrictMembers: true},
			NewChatMember: newMember,
		},
	}, nil)
}

// deliverBotChange runs the bot-health watcher on ctx and requires ext.ContinueGroups.
func (e *ownershipEnv) deliverBotChange(t *testing.T, ctx *ext.Context) {
	t.Helper()
	if err := staffWatchersModule.onBotChatMember(e.bot, ctx); err != ext.ContinueGroups {
		t.Fatalf("onBotChatMember returned %v, want ext.ContinueGroups", err)
	}
}

// wantHealth fails unless the group's link exists with the given health.
func wantHealth(t *testing.T, groupID int64, want string) *models.StaffGroupLink {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(groupID)
	if err != nil || link == nil {
		t.Fatalf("link of group %d = (%+v, %v), want it to remain", groupID, link, err)
	}
	if link.Health != want {
		t.Fatalf("health of group %d = %q, want %q", groupID, link.Health, want)
	}
	return link
}

// wantHeadsUps requires exactly want heads-ups in the Staff Group carrying key and
// no message anywhere else.
func (e *ownershipEnv) wantHeadsUps(t *testing.T, key string, want int) {
	t.Helper()
	if all := e.client.callsFor("sendMessage"); len(all) != want {
		t.Fatalf("%d messages were sent in total (%+v), want %d", len(all), all, want)
	}
	notices := textsToChat(e.client, e.staffID)
	if len(notices) != want {
		t.Fatalf("notices to the Staff Group = %q, want %d", notices, want)
	}
	for _, text := range notices {
		if !strings.Contains(text, staffMarker(key)) {
			t.Fatalf("notice %q does not carry %s", text, key)
		}
	}
}

func TestStaffHealthTracer(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group <b>One</b>")
	wantHealth(t, link.GroupChatID, models.StaffHealthOK)

	chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}
	env.deliverBotChange(t, myChatMemberContext(env.bot, chat, gotgbot.ChatMemberLeft{User: botUser}))

	wantHealth(t, link.GroupChatID, models.StaffHealthBotMissing)
	env.wantHeadsUps(t, "staff_notice_health_bot_missing", 1)
	if notices := textsToChat(env.client, env.staffID); !strings.Contains(notices[0], "Group &lt;b&gt;One&lt;/b&gt;") {
		t.Errorf("heads-up %q must name the group, escaped, spliced after translation", notices[0])
	}
	if sent := callsToChat(env.client, "sendMessage", link.GroupChatID); len(sent) != 0 {
		t.Fatalf("%d messages went to the linked group, want none", len(sent))
	}
	if got := staff.GetLinkOfGroup(link.GroupChatID); got == nil || got.Health != models.StaffHealthBotMissing {
		t.Fatalf("GetLinkOfGroup after the change = %+v, want health bot_missing (cache invalidated)", got)
	}
}
