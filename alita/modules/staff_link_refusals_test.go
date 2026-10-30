//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// linkStrangerID is a user who created none of the groups in a linkEnv.
const linkStrangerID int64 = 5151

// wantNoLink fails when the group has a link row.
func (e *linkEnv) wantNoLink(t *testing.T) {
	t.Helper()
	if link, err := staff.GetLinkOfGroupFresh(e.groupID); err != nil || link != nil {
		t.Fatalf("link of group = (%+v, %v), want none", link, err)
	}
}

// wantInPlace expects exactly one reply, carrying key, in the issuing group and
// nothing at all in the Staff Group.
func (e *linkEnv) wantInPlace(t *testing.T, key string) string {
	t.Helper()
	inGroup := textsToChat(e.client, e.groupID)
	if len(inGroup) != 1 || !strings.Contains(inGroup[0], staffMarker(key)) {
		t.Fatalf("replies in the issuing group = %q, want exactly one %s", inGroup, key)
	}
	if inStaff := callsToChat(e.client, "sendMessage", e.staffID); len(inStaff) != 0 {
		t.Fatalf("%d messages were sent to the Staff Group, want none", len(inStaff))
	}
	e.wantNoLink(t)
	return inGroup[0]
}

// wantInStaff expects exactly one notice, carrying key, in the Staff Group and
// nothing at all in the issuing group.
func (e *linkEnv) wantInStaff(t *testing.T, key string) string {
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

func (e *linkEnv) lookups() int {
	return len(e.client.callsFor("getChatAdministrators")) + len(e.client.callsFor("getChatMember"))
}

// anonymousLinkContext builds a command the way Telegram delivers an anonymous
// admin: SenderChat is the group itself and From is the GroupAnonymousBot.
func anonymousLinkContext(bot *gotgbot.Bot, chat gotgbot.Chat, text string) *ext.Context {
	senderChat := chat
	msg := &gotgbot.Message{
		MessageId:  101,
		Date:       1,
		Chat:       chat,
		From:       &gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"},
		SenderChat: &senderChat,
		Text:       text,
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
}

// withShortSelfDelete makes self-deleting replies vanish after 10 ms for the
// duration of a test.
func withShortSelfDelete(t *testing.T) {
	t.Helper()
	previous := staffSelfDeleteAfter
	staffSelfDeleteAfter = 10 * time.Millisecond
	t.Cleanup(func() { staffSelfDeleteAfter = previous })
}

func TestLinkStaffRefusalsAnonymous(t *testing.T) {
	env := newLinkEnv(t)
	command := fmt.Sprintf("/linkstaff %d", env.staffID)

	ctx := anonymousLinkContext(env.bot, env.groupChat("Anon Group"), command)
	if err := runStaffCommand(t, env.bot, ctx, linkStaffDesc, staffModule.linkStaff); err != ext.EndGroups {
		t.Fatalf("/linkstaff returned %v, want ext.EndGroups", err)
	}

	env.wantInPlace(t, "staff_post_as_yourself")
	if env.lookups() != 0 {
		t.Fatalf("an anonymous admin triggered %d admin or member lookups, want 0", env.lookups())
	}
}

func TestLinkStaffRefusalsInvalidID(t *testing.T) {
	for name, argument := range map[string]string{
		"text":      "abc",
		"positive":  "123",
		"zero":      "0",
		"too long":  strings.Repeat("9", 30),
		"signs":     "--5",
		"two words": "-5 -6",
	} {
		t.Run(name, func(t *testing.T) {
			env := newLinkEnv(t)

			env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff "+argument)

			env.wantInPlace(t, "staff_link_invalid_id")
		})
	}
}

// Naming a Staff Group the issuer does not own and naming a chat that is not a
// Staff Group at all must be indistinguishable, and neither may post anything
// into the named chat.
func TestLinkStaffRefusalsUniformForStranger(t *testing.T) {
	env := newLinkEnv(t)
	notStaff := uniqueModuleChatID()
	env.client.setCreator(notStaff, linkStrangerID)

	env.sendLink(t, env.groupChat("Group"), linkStrangerID, fmt.Sprintf("/linkstaff %d", env.staffID))
	env.sendLink(t, env.groupChat("Group"), linkStrangerID, fmt.Sprintf("/linkstaff %d", notStaff))

	texts := textsToChat(env.client, env.groupID)
	if len(texts) != 2 {
		t.Fatalf("replies in the issuing group = %q, want one per attempt", texts)
	}
	for _, text := range texts {
		if !strings.Contains(text, staffMarker("staff_link_refuse_not_staff_owner")) {
			t.Fatalf("reply %q, want staff_link_refuse_not_staff_owner", text)
		}
	}
	if texts[0] != texts[1] {
		t.Fatalf("refusals differ (%q vs %q): the wording tells the two cases apart", texts[0], texts[1])
	}
	for _, chatID := range []int64{env.staffID, notStaff} {
		if sent := callsToChat(env.client, "sendMessage", chatID); len(sent) != 0 {
			t.Fatalf("%d messages were sent to chat %d, want none", len(sent), chatID)
		}
	}
	env.wantNoLink(t)
}

// A recorded owner who is no longer the live creator is a stranger too.
func TestLinkStaffRefusalsRecordedOwnerNoLongerCreator(t *testing.T) {
	env := newLinkEnv(t)
	env.client.setCreator(env.staffID, linkStrangerID)

	env.sendLink(t, env.groupChat("Group"), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

	env.wantInPlace(t, "staff_link_refuse_not_staff_owner")
}

func TestLinkStaffRefusalsStaffCheckUnknown(t *testing.T) {
	env := newLinkEnv(t)
	env.client.setFailure("getChatAdministrators", env.staffID, &gotgbot.TelegramError{
		Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error",
	})

	env.sendLink(t, env.groupChat("Group"), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

	env.wantInPlace(t, "staff_check_failed")
}

func TestLinkStaffRefusalsToStaffGroup(t *testing.T) {
	const title = "Target <i>Group</i>"

	t.Run("target is a basic group", func(t *testing.T) {
		env := newLinkEnv(t)
		basic := gotgbot.Chat{Id: env.groupID, Type: "group", Title: title}

		env.sendLink(t, basic, env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		notice := env.wantInStaff(t, "staff_link_refuse_not_supergroup")
		if !strings.Contains(notice, "Target &lt;i&gt;Group&lt;/i&gt;") {
			t.Errorf("notice %q must carry the escaped title", notice)
		}
		env.wantNoLink(t)
	})

	t.Run("target is another Staff Group", func(t *testing.T) {
		env := newLinkEnv(t)
		if _, err := staff.CreateStaffGroup(env.groupID, env.ownerID, "Second Staff"); err != nil {
			t.Fatal(err)
		}

		env.sendLink(t, env.groupChat(title), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantInStaff(t, "staff_link_refuse_target_is_staff")
		env.wantNoLink(t)
	})

	t.Run("target is the Staff Group itself", func(t *testing.T) {
		env := newLinkEnv(t)
		self := gotgbot.Chat{Id: env.staffID, Type: "supergroup", Title: "Staff HQ"}

		env.sendLink(t, self, env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantInStaff(t, "staff_link_refuse_target_is_staff")
		if links, err := staff.ListLinksByStaffFresh(env.staffID); err != nil || len(links) != 0 {
			t.Fatalf("links = (%+v, %v), want none", links, err)
		}
	})

	t.Run("issuer is not the target's creator", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setCreator(env.groupID, linkStrangerID)

		env.sendLink(t, env.groupChat(title), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantInStaff(t, "staff_link_refuse_not_target_owner")
		env.wantNoLink(t)
	})

	t.Run("target is already linked", func(t *testing.T) {
		env := newLinkEnv(t)
		otherStaff := uniqueModuleChatID()
		staffCleanup(t, otherStaff)
		seeded := &models.StaffGroupLink{GroupChatID: env.groupID, StaffChatID: otherStaff, OwnerUserID: 77, GroupTitle: "Seeded"}
		if err := db.DB.Create(seeded).Error; err != nil {
			t.Fatalf("seed link: %v", err)
		}

		env.sendLink(t, env.groupChat(title), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantInStaff(t, "staff_link_refuse_already_linked")
		link, err := staff.GetLinkOfGroupFresh(env.groupID)
		if err != nil || link == nil || link.StaffChatID != otherStaff || link.OwnerUserID != 77 {
			t.Fatalf("link = (%+v, %v), want the seeded link untouched", link, err)
		}
	})

	t.Run("target owner lookup fails", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setFailure("getChatAdministrators", env.groupID, &gotgbot.TelegramError{
			Method: "getChatAdministrators", Code: 429, Description: "Too Many Requests: retry after 5",
		})

		env.sendLink(t, env.groupChat(title), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantInStaff(t, "staff_link_check_failed_group")
		env.wantNoLink(t)
	})
}

// When the Staff Group cannot be reached, a refusal falls back to a self-deleting
// reply in the issuing group, but a successful link never shows up there.
func TestLinkStaffNoticeFailure(t *testing.T) {
	failure := &gotgbot.TelegramError{Method: "sendMessage", Code: 403, Description: "Forbidden: bot was kicked from the supergroup chat"}

	t.Run("refusal falls back to the issuing group", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setCreator(env.groupID, linkStrangerID)
		env.client.setFailure("sendMessage", env.staffID, failure)

		env.sendLink(t, env.groupChat("Group"), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		inGroup := textsToChat(env.client, env.groupID)
		if len(inGroup) != 1 || !strings.Contains(inGroup[0], staffMarker("staff_link_refuse_not_target_owner")) {
			t.Fatalf("replies in the issuing group = %q, want the fallback refusal", inGroup)
		}
	})

	t.Run("success stays out of the linked group", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setFailure("sendMessage", env.staffID, failure)

		env.sendLink(t, env.groupChat("Group"), env.ownerID, fmt.Sprintf("/linkstaff %d", env.staffID))

		env.wantLink(t, models.StaffHealthOK)
		if sent := callsToChat(env.client, "sendMessage", env.groupID); len(sent) != 0 {
			t.Fatalf("%d messages were sent to the linked group, want none (D-06)", len(sent))
		}
	})
}

func TestLinkStaffNoArgument(t *testing.T) {
	t.Run("one verified Staff Group is used", func(t *testing.T) {
		env := newLinkEnv(t)

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantLink(t, models.StaffHealthOK)
		env.wantInStaff(t, "staff_link_done")
	})

	t.Run("two verified Staff Groups need an explicit ID", func(t *testing.T) {
		env := newLinkEnv(t)
		second := uniqueModuleChatID()
		staffCleanup(t, second)
		env.client.setCreator(second, env.ownerID)
		if _, err := staff.CreateStaffGroup(second, env.ownerID, "Second"); err != nil {
			t.Fatal(err)
		}

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantInPlace(t, "staff_link_need_staff_id")
		if sent := callsToChat(env.client, "sendMessage", second); len(sent) != 0 {
			t.Fatalf("%d messages were sent to a candidate Staff Group, want none", len(sent))
		}
	})

	t.Run("an owner of none is told so", func(t *testing.T) {
		env := newLinkEnv(t)

		env.sendLink(t, env.groupChat("Group"), uniqueLinkOwnerID(), "/linkstaff")

		env.wantInPlace(t, "staff_link_no_staff_group")
	})

	t.Run("a recorded Staff Group whose live creator changed is not a candidate", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setCreator(env.staffID, linkStrangerID)

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantInPlace(t, "staff_link_no_staff_group")
	})

	t.Run("only the live match is used when another recorded group changed hands", func(t *testing.T) {
		env := newLinkEnv(t)
		stale := uniqueModuleChatID()
		staffCleanup(t, stale)
		env.client.setCreator(stale, linkStrangerID)
		if _, err := staff.CreateStaffGroup(stale, env.ownerID, "Stale"); err != nil {
			t.Fatal(err)
		}

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantLink(t, models.StaffHealthOK)
		if sent := callsToChat(env.client, "sendMessage", stale); len(sent) != 0 {
			t.Fatalf("%d messages were sent to the stale Staff Group, want none", len(sent))
		}
	})

	t.Run("an unverifiable candidate fails closed", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setFailure("getChatAdministrators", env.staffID, &gotgbot.TelegramError{
			Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error",
		})

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantInPlace(t, "staff_check_failed")
	})

	t.Run("a match next to an unverifiable candidate is not picked", func(t *testing.T) {
		env := newLinkEnv(t)
		unreachable := uniqueModuleChatID()
		staffCleanup(t, unreachable)
		env.client.setCreator(unreachable, env.ownerID)
		env.client.setFailure("getChatAdministrators", unreachable, &gotgbot.TelegramError{
			Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error",
		})
		if _, err := staff.CreateStaffGroup(unreachable, env.ownerID, "Unreachable"); err != nil {
			t.Fatal(err)
		}

		env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff")

		env.wantInPlace(t, "staff_check_failed")
	})
}

func TestLinkStaffSelfDeletingReply(t *testing.T) {
	withShortSelfDelete(t)
	env := newLinkEnv(t)

	env.sendLink(t, env.groupChat("Group"), env.ownerID, "/linkstaff abc")

	env.wantInPlace(t, "staff_link_invalid_id")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, call := range callsToChat(env.client, "deleteMessage", env.groupID) {
			if fmt.Sprint(call.Params["message_id"]) == strconv.Itoa(9001) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the reply was not deleted within 1s; deleteMessage calls = %+v", callsToChat(env.client, "deleteMessage", env.groupID))
}
