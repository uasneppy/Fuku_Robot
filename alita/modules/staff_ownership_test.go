//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// ownershipEnv is a scripted bot and a Staff Group S whose live creator is ownerID.
// Every group made by ownershipEnv.addGroup is a supergroup linked to S.
type ownershipEnv struct {
	client  *staffBotClient
	bot     *gotgbot.Bot
	ownerID int64
	staffID int64
}

func newOwnershipEnv(t *testing.T) *ownershipEnv {
	t.Helper()
	withStaffLocale(t)

	env := &ownershipEnv{
		client:  newStaffBotClient(),
		ownerID: uniqueLinkOwnerID(),
		staffID: uniqueModuleChatID(),
	}
	env.bot = newModuleTestBot(env.client.moduleBotClient)
	env.bot.BotClient = env.client
	staffCleanup(t, env.staffID)
	env.client.setCreator(env.staffID, env.ownerID)
	if _, err := staff.CreateStaffGroup(env.staffID, env.ownerID, "Staff HQ"); err != nil {
		t.Fatalf("create Staff Group: %v", err)
	}
	return env
}

// addGroup links a new supergroup titled title to S with maker as the link's
// maker, and makes maker the live creator of the group.
func (e *ownershipEnv) addGroup(t *testing.T, maker int64, title string) models.StaffGroupLink {
	t.Helper()
	groupID := uniqueModuleChatID()
	staffCleanup(t, groupID)
	e.client.setCreator(groupID, maker)
	link := models.StaffGroupLink{
		GroupChatID: groupID, StaffChatID: e.staffID, OwnerUserID: maker, GroupTitle: title,
	}
	if err := db.DB.Create(&link).Error; err != nil {
		t.Fatalf("seed link %q: %v", title, err)
	}
	return link
}

// ownerChangedContext builds an update whose message is a chat_owner_changed
// service message in chat naming newOwner.
func ownerChangedContext(bot *gotgbot.Bot, chat gotgbot.Chat, newOwner int64) *ext.Context {
	msg := &gotgbot.Message{
		MessageId: 104,
		Date:      1,
		Chat:      chat,
		ChatOwnerChanged: &gotgbot.ChatOwnerChanged{
			NewOwner: gotgbot.User{Id: newOwner, FirstName: "NewOwner"},
		},
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 4, Message: msg}, nil)
}

// ownerLeftContext builds an update whose message is a chat_owner_left service
// message in chat (no successor named).
func ownerLeftContext(bot *gotgbot.Bot, chat gotgbot.Chat) *ext.Context {
	msg := &gotgbot.Message{
		MessageId:     105,
		Date:          1,
		Chat:          chat,
		ChatOwnerLeft: &gotgbot.ChatOwnerLeft{},
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 5, Message: msg}, nil)
}

// deliver runs the ownership watcher on ctx and requires ext.ContinueGroups.
func (e *ownershipEnv) deliver(t *testing.T, ctx *ext.Context) {
	t.Helper()
	if err := staffWatchersModule.onOwnershipMessage(e.bot, ctx); err != ext.ContinueGroups {
		t.Fatalf("onOwnershipMessage returned %v, want ext.ContinueGroups", err)
	}
}

// wantLinkGone fails when the group still has a link row.
func wantLinkGone(t *testing.T, groupID int64) {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(groupID)
	if err != nil || link != nil {
		t.Fatalf("link of group %d = (%+v, %v), want none", groupID, link, err)
	}
}

// wantLinkKept fails unless the group still has a link row.
func wantLinkKept(t *testing.T, groupID int64) {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(groupID)
	if err != nil || link == nil {
		t.Fatalf("link of group %d = (%+v, %v), want it to remain", groupID, link, err)
	}
}

// wantOnlyNoticeToStaff requires exactly one sendMessage in the whole run, to the
// Staff Group, carrying key, and none to a linked group or a private chat.
func (e *ownershipEnv) wantOnlyNoticeToStaff(t *testing.T, key string) string {
	t.Helper()
	all := e.client.callsFor("sendMessage")
	if len(all) != 1 {
		t.Fatalf("%d messages were sent in total (%+v), want exactly one", len(all), all)
	}
	notices := textsToChat(e.client, e.staffID)
	if len(notices) != 1 || !strings.Contains(notices[0], staffMarker(key)) {
		t.Fatalf("notices to the Staff Group = %q, want exactly one %s", notices, key)
	}
	for _, call := range all {
		var chatID int64
		if _, err := fmt.Sscan(fmt.Sprint(call.Params["chat_id"]), &chatID); err != nil || chatID >= 0 {
			t.Fatalf("a message went to chat %v, want only the Staff Group (D-13: no private message)",
				call.Params["chat_id"])
		}
	}
	return notices[0]
}

func TestStaffOwnershipTracer(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group <b>One</b>")
	const newOwner int64 = 777001

	env.client.setCreator(link.GroupChatID, newOwner)
	chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}
	env.deliver(t, ownerChangedContext(env.bot, chat, newOwner))

	wantLinkGone(t, link.GroupChatID)
	notice := env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_group_owner_changed")
	if !strings.Contains(notice, "Group &lt;b&gt;One&lt;/b&gt;") {
		t.Errorf("notice %q must name the group, escaped, spliced after translation", notice)
	}
	if got := staff.GetLinkOfGroup(link.GroupChatID); got != nil {
		t.Fatalf("GetLinkOfGroup after the removal = %+v, want nil (cache invalidated)", got)
	}
}

func TestStaffOwnershipPayloadIsOnlyAHint(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")

	// The service message names P, but the live creator is still the link's maker.
	chat := gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: "Group One"}
	env.deliver(t, ownerChangedContext(env.bot, chat, 777002))

	wantLinkKept(t, link.GroupChatID)
	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Fatalf("%d messages were sent, want none: the payload must not decide", len(sent))
	}
}
