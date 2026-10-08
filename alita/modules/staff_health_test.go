//go:build testtools

package modules

import (
	"strings"
	"sync"
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

func TestStaffHealthTransitions(t *testing.T) {
	cases := []struct {
		name   string
		member gotgbot.ChatMember
		health string
		key    string
	}{
		{
			name:   "administrator without the restrict right",
			member: gotgbot.ChatMemberAdministrator{User: botUser, CanRestrictMembers: false},
			health: models.StaffHealthBotCannotRestrict,
			key:    "staff_notice_health_bot_cannot_restrict",
		},
		{
			name:   "demoted to a plain member",
			member: gotgbot.ChatMemberMember{User: botUser},
			health: models.StaffHealthBotNotAdmin,
			key:    "staff_notice_health_bot_not_admin",
		},
		{
			name:   "kicked from the group",
			member: gotgbot.ChatMemberBanned{User: botUser},
			health: models.StaffHealthBotMissing,
			key:    "staff_notice_health_bot_missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newOwnershipEnv(t)
			link := env.addGroup(t, env.ownerID, "Group One")
			chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}

			env.deliverBotChange(t, myChatMemberContext(env.bot, chat, tc.member))

			wantHealth(t, link.GroupChatID, tc.health)
			env.wantHeadsUps(t, tc.key, 1)
		})
	}
}

func TestStaffHealthRecovery(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}

	env.deliverBotChange(t, myChatMemberContext(env.bot, chat, gotgbot.ChatMemberLeft{User: botUser}))
	lost := wantHealth(t, link.GroupChatID, models.StaffHealthBotMissing)

	env.deliverBotChange(t, myChatMemberContext(env.bot, chat,
		gotgbot.ChatMemberAdministrator{User: botUser, CanRestrictMembers: true}))

	back := wantHealth(t, link.GroupChatID, models.StaffHealthOK)
	if back.ID != lost.ID || back.ID != link.ID {
		t.Fatalf("link ID after recovery = %d, was %d: recovery must not relink", back.ID, link.ID)
	}
	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 2 ||
		!strings.Contains(notices[0], staffMarker("staff_notice_health_bot_missing")) ||
		!strings.Contains(notices[1], staffMarker("staff_notice_health_ok")) {
		t.Fatalf("notices to the Staff Group = %q, want the problem heads-up then one healthy-again heads-up", notices)
	}
	if all := env.client.callsFor("sendMessage"); len(all) != 2 {
		t.Fatalf("%d messages were sent in total, want 2", len(all))
	}
}

func TestStaffHealthNoRepeat(t *testing.T) {
	t.Run("the same update twice", func(t *testing.T) {
		env := newOwnershipEnv(t)
		link := env.addGroup(t, env.ownerID, "Group One")
		chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}

		env.deliverBotChange(t, myChatMemberContext(env.bot, chat, gotgbot.ChatMemberLeft{User: botUser}))
		env.deliverBotChange(t, myChatMemberContext(env.bot, chat, gotgbot.ChatMemberLeft{User: botUser}))

		wantHealth(t, link.GroupChatID, models.StaffHealthBotMissing)
		env.wantHeadsUps(t, "staff_notice_health_bot_missing", 1)
	})
	t.Run("applyLinkHealth twice", func(t *testing.T) {
		env := newOwnershipEnv(t)
		link := env.addGroup(t, env.ownerID, "Group One")

		if !applyLinkHealth(env.bot, link, models.StaffHealthBotMissing) {
			t.Fatal("the first applyLinkHealth reported no change")
		}
		if applyLinkHealth(env.bot, link, models.StaffHealthBotMissing) {
			t.Fatal("the repeated applyLinkHealth reported a change")
		}
		env.wantHeadsUps(t, "staff_notice_health_bot_missing", 1)
	})
}

func TestStaffHealthConcurrentPostsOnce(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			applyLinkHealth(env.bot, link, models.StaffHealthBotNotAdmin)
		}()
	}
	close(start)
	wg.Wait()

	wantHealth(t, link.GroupChatID, models.StaffHealthBotNotAdmin)
	env.wantHeadsUps(t, "staff_notice_health_bot_not_admin", 1)
}

func TestStaffHealthIgnoresUnrelatedChats(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	unlinked := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Unlinked"}
	staffChat := env.staffChat()

	for name, chat := range map[string]gotgbot.Chat{"an unlinked group": unlinked, "the Staff Group itself": staffChat} {
		t.Run(name, func(t *testing.T) {
			env.deliverBotChange(t, myChatMemberContext(env.bot, chat, gotgbot.ChatMemberLeft{User: botUser}))
		})
	}

	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Fatalf("%d messages were sent, want none for chats that are not linked groups", len(sent))
	}
	wantHealth(t, link.GroupChatID, models.StaffHealthOK)
	if sg, err := staff.GetStaffGroupFresh(env.staffID); err != nil || sg == nil {
		t.Fatalf("Staff Group row = (%+v, %v), want it untouched", sg, err)
	}
	if rows, err := staff.ListLinksByStaffFresh(env.staffID); err != nil || len(rows) != 1 || rows[0].Health != models.StaffHealthOK {
		t.Fatalf("links of the Staff Group = (%+v, %v), want the one link, still ok", rows, err)
	}
	if got := staff.GetLinkOfGroup(unlinked.Id); got != nil {
		t.Fatalf("an unlinked group gained a link: %+v", got)
	}

	// An update with no my_chat_member payload is ignored as well.
	empty := ext.NewContext(env.bot, &gotgbot.Update{UpdateId: 7}, nil)
	env.deliverBotChange(t, empty)
}
