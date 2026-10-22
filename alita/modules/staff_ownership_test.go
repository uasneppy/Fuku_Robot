//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
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

// clearCreator makes the fake list no creator for chatID, as Telegram does after
// the owner left a chat.
func (c *staffBotClient) clearCreator(chatID int64) {
	c.smu.Lock()
	defer c.smu.Unlock()
	delete(c.creators, chatID)
}

func ownershipRateLimit(chatID int64) error {
	return &gotgbot.TelegramError{
		Method:         "getChatAdministrators",
		Code:           429,
		Description:    fmt.Sprintf("Too Many Requests: retry after 5 (chat %d)", chatID),
		ResponseParams: &gotgbot.ResponseParameters{RetryAfter: 5},
	}
}

// adminListCalls counts the live getChatAdministrators lookups the fake saw.
func (e *ownershipEnv) adminListCalls() int {
	return len(e.client.callsFor("getChatAdministrators"))
}

func (e *ownershipEnv) staffChat() gotgbot.Chat {
	return gotgbot.Chat{Id: e.staffID, Type: "supergroup", Title: "Staff HQ"}
}

func groupChat(link models.StaffGroupLink) gotgbot.Chat {
	return gotgbot.Chat{Id: link.GroupChatID, Type: "supergroup", Title: link.GroupTitle}
}

func (e *ownershipEnv) wantStaffOwner(t *testing.T, want int64) {
	t.Helper()
	row, err := staff.GetStaffGroupFresh(e.staffID)
	if err != nil || row == nil || row.OwnerUserID != want {
		t.Fatalf("staff group = (%+v, %v), want it to exist with owner %d", row, err, want)
	}
}

func TestStaffOwnershipStaffGroupChanged(t *testing.T) {
	env := newOwnershipEnv(t)
	const newOwner int64 = 777003
	g1 := env.addGroup(t, env.ownerID, "Group One")
	g2 := env.addGroup(t, env.ownerID, "Group Two")
	g3 := env.addGroup(t, newOwner, "Group Three")

	env.client.setCreator(env.staffID, newOwner)
	env.deliver(t, ownerChangedContext(env.bot, env.staffChat(), newOwner))

	wantLinkGone(t, g1.GroupChatID)
	wantLinkGone(t, g2.GroupChatID)
	wantLinkKept(t, g3.GroupChatID)
	notice := env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_staff_owner_changed")
	one, two := strings.Index(notice, "Group One"), strings.Index(notice, "Group Two")
	if one < 0 || two < 0 || one > two {
		t.Errorf("notice %q must list Group One then Group Two, in link-id order", notice)
	}
	if strings.Contains(notice, "Group Three") {
		t.Errorf("notice %q lists Group Three, whose link is still valid", notice)
	}
	env.wantStaffOwner(t, newOwner)
	if got := staff.GetStaffGroup(env.staffID); got == nil || got.OwnerUserID != newOwner {
		t.Fatalf("cached GetStaffGroup = %+v, want owner %d (owner refresh must invalidate)", got, newOwner)
	}
}

func TestStaffOwnershipStaffGroupHasNoCreatorRemovesAllLinks(t *testing.T) {
	env := newOwnershipEnv(t)
	g1 := env.addGroup(t, env.ownerID, "Group One")
	g2 := env.addGroup(t, env.ownerID, "Group Two")

	env.client.clearCreator(env.staffID)
	env.deliver(t, ownerLeftContext(env.bot, env.staffChat()))

	wantLinkGone(t, g1.GroupChatID)
	wantLinkGone(t, g2.GroupChatID)
	env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_staff_owner_changed")
	env.wantStaffOwner(t, env.ownerID)
}

func TestStaffOwnershipOwnerLeft(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	other := env.addGroup(t, env.ownerID, "Group Two")

	env.client.clearCreator(link.GroupChatID)
	env.deliver(t, ownerLeftContext(env.bot, groupChat(link)))

	wantLinkGone(t, link.GroupChatID)
	wantLinkKept(t, other.GroupChatID)
	env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_group_owner_changed")
}

func TestStaffOwnershipSameNewOwnerStillUnlinks(t *testing.T) {
	const newOwner int64 = 777004
	for _, firstInStaff := range []bool{false, true} {
		name := "group message first"
		if firstInStaff {
			name = "staff message first"
		}
		t.Run(name, func(t *testing.T) {
			env := newOwnershipEnv(t)
			link := env.addGroup(t, env.ownerID, "Group One")
			env.client.setCreator(env.staffID, newOwner)
			env.client.setCreator(link.GroupChatID, newOwner)

			inGroup := ownerChangedContext(env.bot, groupChat(link), newOwner)
			inStaff := ownerChangedContext(env.bot, env.staffChat(), newOwner)
			if firstInStaff {
				env.deliver(t, inStaff)
				env.deliver(t, inGroup)
			} else {
				env.deliver(t, inGroup)
				env.deliver(t, inStaff)
			}

			wantLinkGone(t, link.GroupChatID)
			if all := env.client.callsFor("sendMessage"); len(all) != 1 {
				t.Fatalf("%d notices were sent, want exactly one however the messages are ordered", len(all))
			}
			env.wantStaffOwner(t, newOwner)
		})
	}
}

func TestStaffOwnershipUnknownKeepsLink(t *testing.T) {
	cases := []struct {
		name string
		fail func(env *ownershipEnv, link models.StaffGroupLink)
		in   func(env *ownershipEnv, link models.StaffGroupLink) gotgbot.Chat
	}{
		{
			name: "linked group lookup fails",
			fail: func(env *ownershipEnv, link models.StaffGroupLink) {
				env.client.setFailure("getChatAdministrators", link.GroupChatID, ownershipRateLimit(link.GroupChatID))
			},
			in: func(_ *ownershipEnv, link models.StaffGroupLink) gotgbot.Chat { return groupChat(link) },
		},
		{
			name: "staff group lookup fails, message in the linked group",
			fail: func(env *ownershipEnv, _ models.StaffGroupLink) {
				env.client.setFailure("getChatAdministrators", env.staffID, ownershipRateLimit(env.staffID))
			},
			in: func(_ *ownershipEnv, link models.StaffGroupLink) gotgbot.Chat { return groupChat(link) },
		},
		{
			name: "staff group lookup fails, message in the staff group",
			fail: func(env *ownershipEnv, _ models.StaffGroupLink) {
				env.client.setFailure("getChatAdministrators", env.staffID, ownershipRateLimit(env.staffID))
			},
			in: func(env *ownershipEnv, _ models.StaffGroupLink) gotgbot.Chat { return env.staffChat() },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newOwnershipEnv(t)
			link := env.addGroup(t, env.ownerID, "Group One")
			// Ownership really did change, but Telegram cannot say so right now. The
			// group still lists the maker where only the Staff Group lookup fails, so
			// the failing side is the only one that could have decided anything.
			if tc.name == "linked group lookup fails" {
				env.client.setCreator(env.staffID, env.ownerID)
			} else {
				env.client.setCreator(link.GroupChatID, env.ownerID)
			}
			tc.fail(env, link)

			env.deliver(t, ownerChangedContext(env.bot, tc.in(env, link), 777005))

			wantLinkKept(t, link.GroupChatID)
			env.wantStaffOwner(t, env.ownerID)
			if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
				t.Fatalf("%d messages were sent, want none after an unknown answer", len(sent))
			}
		})
	}
}

func TestStaffOwnershipDefiniteSideWinsOverUnknownSide(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	env.client.setFailure("getChatAdministrators", link.GroupChatID, ownershipRateLimit(link.GroupChatID))
	env.client.setCreator(env.staffID, 777007)

	env.deliver(t, ownerChangedContext(env.bot, groupChat(link), 777007))

	wantLinkGone(t, link.GroupChatID)
	env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_staff_owner_changed")
}

func TestStaffOwnershipChatMemberCreatorTransition(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	user := func(id int64) gotgbot.User { return gotgbot.User{Id: id, FirstName: fmt.Sprintf("User%d", id)} }
	update := func(chat gotgbot.Chat, oldMember, newMember gotgbot.ChatMember) *ext.Context {
		return ext.NewContext(env.bot, &gotgbot.Update{UpdateId: 6, ChatMember: &gotgbot.ChatMemberUpdated{
			Chat: chat, From: user(env.ownerID), Date: 1, OldChatMember: oldMember, NewChatMember: newMember,
		}}, nil)
	}
	deliver := func(t *testing.T, ctx *ext.Context) {
		t.Helper()
		if err := staffWatchersModule.onCreatorChatMember(env.bot, ctx); err != ext.ContinueGroups {
			t.Fatalf("onCreatorChatMember returned %v, want ext.ContinueGroups", err)
		}
	}

	t.Run("non-creator transition does no Telegram work", func(t *testing.T) {
		ctx := update(groupChat(link),
			gotgbot.ChatMemberMember{User: user(10)}, gotgbot.ChatMemberAdministrator{User: user(10)})
		if staffCreatorTransition(ctx.ChatMember) {
			t.Fatal("staffCreatorTransition(member -> administrator) = true, want false")
		}
		env.client.setCreator(link.GroupChatID, 777008)
		deliver(t, ctx)
		if got := env.adminListCalls(); got != 0 {
			t.Fatalf("%d getChatAdministrators calls for a non-creator transition, want 0", got)
		}
		wantLinkKept(t, link.GroupChatID)
	})

	t.Run("creator transition in an unrelated chat does no Telegram work", func(t *testing.T) {
		unrelated := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Unrelated"}
		ctx := update(unrelated,
			gotgbot.ChatMemberOwner{User: user(env.ownerID)}, gotgbot.ChatMemberAdministrator{User: user(env.ownerID)})
		if !staffCreatorTransition(ctx.ChatMember) {
			t.Fatal("staffCreatorTransition(creator -> administrator) = false, want true")
		}
		deliver(t, ctx)
		if got := env.adminListCalls(); got != 0 {
			t.Fatalf("%d getChatAdministrators calls for an unrelated chat, want 0", got)
		}
		if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
			t.Fatalf("%d messages were sent for an unrelated chat, want none", len(sent))
		}
	})

	t.Run("creator transition in a linked group rechecks live and removes", func(t *testing.T) {
		// The live creator is now P (set above); the old creator merely lost the role.
		ctx := update(groupChat(link),
			gotgbot.ChatMemberOwner{User: user(env.ownerID)}, gotgbot.ChatMemberAdministrator{User: user(env.ownerID)})
		deliver(t, ctx)
		wantLinkGone(t, link.GroupChatID)
		env.wantOnlyNoticeToStaff(t, "staff_notice_unlinked_group_owner_changed")
		if got := env.adminListCalls(); got == 0 {
			t.Fatal("no live getChatAdministrators call: the update must not decide on its own")
		}
	})

	t.Run("new creator in the staff group refreshes the owner", func(t *testing.T) {
		other := env.addGroup(t, env.ownerID, "Group Two")
		env.client.setCreator(env.staffID, 777009)
		ctx := update(env.staffChat(),
			gotgbot.ChatMemberAdministrator{User: user(777009)}, gotgbot.ChatMemberOwner{User: user(777009)})
		deliver(t, ctx)
		wantLinkGone(t, other.GroupChatID)
		env.wantStaffOwner(t, 777009)
	})
}

func TestStaffOwnershipConcurrentRechecksPostOnce(t *testing.T) {
	env := newOwnershipEnv(t)
	link := env.addGroup(t, env.ownerID, "Group One")
	env.client.setCreator(link.GroupChatID, 777010)

	const callers = 8
	results := make(chan staffRecheckResult, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- recheckLink(env.bot, link, nil)
		}()
	}
	wg.Wait()
	close(results)

	counts := map[staffRecheckResult]int{}
	for result := range results {
		counts[result]++
	}
	if counts[staffRecheckRemoved] != 1 {
		t.Fatalf("results = %v, want exactly one staffRecheckRemoved", counts)
	}
	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_notice_unlinked_group_owner_changed")) {
		t.Fatalf("notices = %q, want exactly one removal notice", notices)
	}
	var rows int64
	if err := db.DB.Model(&models.StaffGroupLink{}).Where("id = ?", link.ID).Count(&rows).Error; err != nil || rows != 0 {
		t.Fatalf("link rows = (%d, %v), want 0", rows, err)
	}
}

func TestStaffOwnershipPassAsksOncePerKey(t *testing.T) {
	env := newOwnershipEnv(t)
	pass := newStaffOwnerPass()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, live, err := pass.check(env.bot, env.staffID, env.ownerID)
			if result != chat_status.OwnerMatch || live != env.ownerID || err != nil {
				t.Errorf("check = (%v, %d, %v), want (OwnerMatch, %d, nil)", result, live, err, env.ownerID)
			}
		}()
	}
	wg.Wait()
	if got := env.adminListCalls(); got != 1 {
		t.Fatalf("%d lookups for one key, want 1", got)
	}

	if result, _, _ := pass.check(env.bot, env.staffID, env.ownerID+1); result != chat_status.OwnerMismatch {
		t.Fatalf("check(other user) = %v, want OwnerMismatch", result)
	}
	if got := env.adminListCalls(); got != 2 {
		t.Fatalf("%d lookups after a second key, want 2", got)
	}
}

func TestStaffOwnershipRecheckStaffGroupPaceStopsLoop(t *testing.T) {
	env := newOwnershipEnv(t)
	g1 := env.addGroup(t, env.ownerID, "Group One")
	g2 := env.addGroup(t, env.ownerID, "Group Two")
	env.client.setCreator(g1.GroupChatID, 777011)
	env.client.setCreator(g2.GroupChatID, 777012)

	paced := 0
	summary := recheckStaffGroup(context.Background(), env.bot, env.staffID, func(context.Context) bool {
		paced++
		return false
	})

	if paced != 1 {
		t.Fatalf("pace was called %d times, want 1 (a false return stops the loop)", paced)
	}
	if !summary.Unknown || len(summary.RemovedGroupIDs) != 0 {
		t.Fatalf("summary = %+v, want Unknown with nothing removed", summary)
	}
	wantLinkKept(t, g1.GroupChatID)
	wantLinkKept(t, g2.GroupChatID)
}
