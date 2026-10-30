//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// unlinkStrangerID is a user who created none of the groups in an unlinkEnv.
const unlinkStrangerID int64 = 5151

// unlinkEnv is a scripted bot, a Staff Group S, and two supergroups G and Other
// that are both linked to S. ownerID is the live creator of all three.
type unlinkEnv struct {
	client  *staffBotClient
	bot     *gotgbot.Bot
	ownerID int64
	staffID int64
	groupID int64
	otherID int64
	link    *models.StaffGroupLink
	other   *models.StaffGroupLink
}

func newUnlinkEnv(t *testing.T) *unlinkEnv {
	t.Helper()
	withStaffLocale(t)

	env := &unlinkEnv{
		client:  newStaffBotClient(),
		ownerID: uniqueLinkOwnerID(),
		staffID: uniqueModuleChatID(),
		groupID: uniqueModuleChatID(),
		otherID: uniqueModuleChatID(),
	}
	env.bot = newModuleTestBot(env.client.moduleBotClient)
	env.bot.BotClient = env.client
	for _, id := range []int64{env.staffID, env.groupID, env.otherID} {
		env.client.setCreator(id, env.ownerID)
	}
	staffCleanup(t, env.staffID, env.groupID, env.otherID)
	if _, err := staff.CreateStaffGroup(env.staffID, env.ownerID, "Staff HQ"); err != nil {
		t.Fatalf("create Staff Group: %v", err)
	}
	env.link = &models.StaffGroupLink{
		GroupChatID: env.groupID, StaffChatID: env.staffID, OwnerUserID: env.ownerID, GroupTitle: "Group <b>One</b>",
	}
	env.other = &models.StaffGroupLink{
		GroupChatID: env.otherID, StaffChatID: env.staffID, OwnerUserID: env.ownerID, GroupTitle: "Other",
	}
	for _, link := range []*models.StaffGroupLink{env.link, env.other} {
		if err := staff.CreateLink(link); err != nil {
			t.Fatalf("create link: %v", err)
		}
	}
	return env
}

func (e *unlinkEnv) groupChat() gotgbot.Chat {
	return gotgbot.Chat{Id: e.groupID, Type: "supergroup", Title: "Group <b>One</b>"}
}

// send runs /unlinkstaff in chat as userID.
func (e *unlinkEnv) send(t *testing.T, chat gotgbot.Chat, userID int64) {
	t.Helper()
	user := gotgbot.User{Id: userID, FirstName: "Sender <i>"}
	ctx := newModuleMessageContext(e.bot, chat, user, "/unlinkstaff")
	if err := runStaffCommand(t, e.bot, ctx, unlinkStaffDesc, staffModule.unlinkStaff); err != ext.EndGroups {
		t.Fatalf("/unlinkstaff returned %v, want ext.EndGroups", err)
	}
}

func (e *unlinkEnv) lookups() int {
	return len(e.client.callsFor("getChatAdministrators")) + len(e.client.callsFor("getChatMember"))
}

// wantLinked fails unless the group still has exactly the link row it started with.
func (e *unlinkEnv) wantLinked(t *testing.T, groupID int64) {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(groupID)
	if err != nil || link == nil || link.StaffChatID != e.staffID {
		t.Fatalf("link of group %d = (%+v, %v), want the link to remain", groupID, link, err)
	}
}

// wantUnlinked fails when the group still has a link row.
func (e *unlinkEnv) wantUnlinked(t *testing.T, groupID int64) {
	t.Helper()
	link, err := staff.GetLinkOfGroupFresh(groupID)
	if err != nil || link != nil {
		t.Fatalf("link of group %d = (%+v, %v), want none", groupID, link, err)
	}
}

// wantInPlace expects exactly one reply, carrying key, in chatID and nothing at
// all in the Staff Group.
func (e *unlinkEnv) wantInPlace(t *testing.T, chatID int64, key string) {
	t.Helper()
	inChat := textsToChat(e.client, chatID)
	if len(inChat) != 1 || !strings.Contains(inChat[0], staffMarker(key)) {
		t.Fatalf("replies in chat %d = %q, want exactly one %s", chatID, inChat, key)
	}
	if inStaff := callsToChat(e.client, "sendMessage", e.staffID); len(inStaff) != 0 {
		t.Fatalf("%d messages were sent to the Staff Group, want none", len(inStaff))
	}
}

// wantInStaff expects exactly one notice, carrying key, in the Staff Group and
// nothing at all in the linked group.
func (e *unlinkEnv) wantInStaff(t *testing.T, key string) string {
	t.Helper()
	inStaff := textsToChat(e.client, e.staffID)
	if len(inStaff) != 1 || !strings.Contains(inStaff[0], staffMarker(key)) {
		t.Fatalf("notices in the Staff Group = %q, want exactly one %s", inStaff, key)
	}
	if inGroup := callsToChat(e.client, "sendMessage", e.groupID); len(inGroup) != 0 {
		t.Fatalf("%d messages were sent to the linked group, want none", len(inGroup))
	}
	return inStaff[0]
}

// wantSelfDeleted waits for the bot to delete its reply (message 9001) in chatID.
func (e *unlinkEnv) wantSelfDeleted(t *testing.T, chatID int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, call := range callsToChat(e.client, "deleteMessage", chatID) {
			if fmt.Sprint(call.Params["message_id"]) == strconv.Itoa(9001) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the reply in chat %d was not deleted within 1s; deleteMessage calls = %+v",
		chatID, callsToChat(e.client, "deleteMessage", chatID))
}

func TestUnlinkStaffTracer(t *testing.T) {
	env := newUnlinkEnv(t)

	env.send(t, env.groupChat(), env.ownerID)

	deletes := callsToChat(env.client, "deleteMessage", env.groupID)
	if len(deletes) != 1 || fmt.Sprint(deletes[0].Params["message_id"]) != "101" {
		t.Fatalf("deleteMessage calls in the linked group = %+v, want one for the command message", deletes)
	}
	env.wantUnlinked(t, env.groupID)
	env.wantLinked(t, env.otherID)

	notice := env.wantInStaff(t, "staff_notice_unlinked_by")
	for _, want := range []string{
		"Group &lt;b&gt;One&lt;/b&gt;",
		fmt.Sprintf(`<a href="tg://user?id=%d">Sender &lt;i&gt;</a>`, env.ownerID),
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice %q must contain %q (escaped once, spliced after translation)", notice, want)
		}
	}
	if sent := callsToChat(env.client, "sendMessage", env.groupID); len(sent) != 0 {
		t.Fatalf("%d messages were sent to the linked group, want none (D-06, D-07)", len(sent))
	}

	// The group no longer has a link, so the cached gate must agree.
	if got := staff.GetLinkOfGroup(env.groupID); got != nil {
		t.Fatalf("GetLinkOfGroup after unlink = %+v, want nil", got)
	}
}

func TestUnlinkStaffRefusals(t *testing.T) {
	t.Run("not linked", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)
		unlinked := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Free"}

		env.send(t, unlinked, env.ownerID)

		env.wantInPlace(t, unlinked.Id, "staff_unlink_not_linked")
		env.wantSelfDeleted(t, unlinked.Id)
		env.wantLinked(t, env.groupID)
		env.wantLinked(t, env.otherID)
	})

	t.Run("stranger gets an in-place self-deleting refusal", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)

		env.send(t, env.groupChat(), unlinkStrangerID)

		env.wantInPlace(t, env.groupID, "staff_unlink_refuse_not_owner")
		env.wantSelfDeleted(t, env.groupID)
		env.wantLinked(t, env.groupID)
		env.wantLinked(t, env.otherID)
	})

	t.Run("group creator who does not own the Staff Group", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)
		env.client.setCreator(env.staffID, 777) // the Staff Group's creator changed

		env.send(t, env.groupChat(), env.ownerID)

		env.wantInPlace(t, env.groupID, "staff_unlink_refuse_not_owner")
		env.wantLinked(t, env.groupID)
	})

	t.Run("Staff Group creator who no longer owns the group", func(t *testing.T) {
		env := newUnlinkEnv(t)
		env.client.setCreator(env.groupID, 777)

		env.send(t, env.groupChat(), env.ownerID)

		notice := env.wantInStaff(t, "staff_unlink_refuse_not_group_owner")
		if !strings.Contains(notice, "Group &lt;b&gt;One&lt;/b&gt;") {
			t.Errorf("notice %q must name the escaped group title", notice)
		}
		env.wantLinked(t, env.groupID)
		env.wantLinked(t, env.otherID)
	})

	t.Run("owner check on the Staff Group fails", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)
		env.client.setFailure("getChatAdministrators", env.staffID, errors.New("telegram unreachable"))

		env.send(t, env.groupChat(), env.ownerID)

		env.wantInPlace(t, env.groupID, "staff_check_failed")
		env.wantSelfDeleted(t, env.groupID)
		env.wantLinked(t, env.groupID)
	})

	t.Run("owner check on the group fails", func(t *testing.T) {
		env := newUnlinkEnv(t)
		env.client.setFailure("getChatAdministrators", env.groupID, errors.New("telegram unreachable"))

		env.send(t, env.groupChat(), env.ownerID)

		env.wantInStaff(t, "staff_unlink_check_failed_group")
		env.wantLinked(t, env.groupID)
	})

	t.Run("anonymous admin", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)

		ctx := anonymousLinkContext(env.bot, env.groupChat(), "/unlinkstaff")
		if err := runStaffCommand(t, env.bot, ctx, unlinkStaffDesc, staffModule.unlinkStaff); err != ext.EndGroups {
			t.Fatalf("/unlinkstaff returned %v, want ext.EndGroups", err)
		}

		env.wantInPlace(t, env.groupID, "staff_post_as_yourself")
		env.wantSelfDeleted(t, env.groupID)
		if env.lookups() != 0 {
			t.Fatalf("an anonymous admin triggered %d admin or member lookups, want 0", env.lookups())
		}
		env.wantLinked(t, env.groupID)
	})

	t.Run("refusal falls back in place when the Staff Group cannot be reached", func(t *testing.T) {
		withShortSelfDelete(t)
		env := newUnlinkEnv(t)
		env.client.setCreator(env.groupID, 777)
		env.client.setFailure("sendMessage", env.staffID, errors.New("bot was kicked"))

		env.send(t, env.groupChat(), env.ownerID)

		inGroup := textsToChat(env.client, env.groupID)
		if len(inGroup) != 1 || !strings.Contains(inGroup[0], staffMarker("staff_unlink_refuse_not_group_owner")) {
			t.Fatalf("replies in the linked group = %q, want one staff_unlink_refuse_not_group_owner", inGroup)
		}
		env.wantSelfDeleted(t, env.groupID)
		env.wantLinked(t, env.groupID)
	})
}

// A stranger's refusal must never reach the Staff Group, whichever group of the
// Staff Group they try it in.
func TestUnlinkStaffStrangerNeverPostsInStaffGroup(t *testing.T) {
	withShortSelfDelete(t)
	env := newUnlinkEnv(t)
	otherChat := gotgbot.Chat{Id: env.otherID, Type: "supergroup", Title: "Other"}

	env.send(t, env.groupChat(), unlinkStrangerID)
	env.send(t, otherChat, unlinkStrangerID)

	if inStaff := callsToChat(env.client, "sendMessage", env.staffID); len(inStaff) != 0 {
		t.Fatalf("a stranger caused %d messages in the Staff Group, want none", len(inStaff))
	}
	env.wantLinked(t, env.groupID)
	env.wantLinked(t, env.otherID)
}

func TestStaffUnlinkRefusalTextSplicesTitleAfterTranslation(t *testing.T) {
	withStaffLocale(t)
	tr := staffChatTranslator(uniqueModuleChatID())

	text := staffUnlinkRefusalText(tr, staffUnlinkNotGroupOwner, "100%d <b>")

	if !strings.Contains(text, "100%d &lt;b&gt;") {
		t.Fatalf("text %q must carry the title escaped and unmangled by the translator", text)
	}
	for _, r := range []staffUnlinkRefusal{staffUnlinkOK, staffUnlinkGone} {
		if got := staffUnlinkRefusalText(tr, r, "x"); got != "" {
			t.Errorf("refusal %d has text %q, want none", r, got)
		}
	}
}
