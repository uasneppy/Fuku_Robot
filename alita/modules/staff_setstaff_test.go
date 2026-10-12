//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

const (
	setStaffOwnerID    int64 = 4242
	setStaffStrangerID int64 = 5151
)

// setStaffEnv is a scripted bot plus a fresh chat whose creator is setStaffOwnerID.
type setStaffEnv struct {
	client *staffBotClient
	bot    *gotgbot.Bot
	chatID int64
}

func newSetStaffEnv(t *testing.T) *setStaffEnv {
	t.Helper()
	withStaffLocale(t)

	env := &setStaffEnv{client: newStaffBotClient(), chatID: uniqueModuleChatID()}
	env.bot = newModuleTestBot(env.client.moduleBotClient)
	env.bot.BotClient = env.client
	env.client.setCreator(env.chatID, setStaffOwnerID)
	staffCleanup(t, env.chatID)
	return env
}

func (e *setStaffEnv) run(t *testing.T, ctx *ext.Context) {
	t.Helper()
	if err := runStaffCommand(t, e.bot, ctx, setStaffDesc, staffModule.setStaff); err != ext.EndGroups {
		t.Fatalf("/setstaff returned %v, want ext.EndGroups", err)
	}
}

// send runs /setstaff as userID in the given chat.
func (e *setStaffEnv) send(t *testing.T, chat gotgbot.Chat, userID int64, text string) {
	t.Helper()
	user := gotgbot.User{Id: userID, FirstName: "User"}
	e.run(t, newModuleMessageContext(e.bot, chat, user, text))
}

func (e *setStaffEnv) wantReply(t *testing.T, key string) {
	t.Helper()
	texts := sentTexts(e.client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker(key)) {
		t.Fatalf("replies = %q, want exactly one %s", texts, key)
	}
}

func (e *setStaffEnv) wantNoRow(t *testing.T) {
	t.Helper()
	if row, err := staff.GetStaffGroupFresh(e.chatID); err != nil || row != nil {
		t.Fatalf("a staff_groups row exists: %+v (err %v)", row, err)
	}
}

func (e *setStaffEnv) methodsCalled() []string {
	e.client.mu.Lock()
	defer e.client.mu.Unlock()
	methods := make([]string, 0, len(e.client.calls))
	for _, call := range e.client.calls {
		methods = append(methods, call.Method)
	}
	return methods
}

func (e *setStaffEnv) lookups() int {
	return len(e.client.callsFor("getChatAdministrators")) + len(e.client.callsFor("getChatMember"))
}

// anonymousAdminContext builds /setstaff the way Telegram delivers an anonymous
// admin: SenderChat is the group itself and From is the GroupAnonymousBot.
func anonymousAdminContext(bot *gotgbot.Bot, chat gotgbot.Chat) *ext.Context {
	senderChat := chat
	msg := &gotgbot.Message{
		MessageId:  101,
		Date:       1,
		Chat:       chat,
		From:       &gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"},
		SenderChat: &senderChat,
		Text:       "/setstaff",
	}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
}

func TestSetStaffRejectsAnonymousAdmin(t *testing.T) {
	env := newSetStaffEnv(t)

	env.run(t, anonymousAdminContext(env.bot, staffSupergroup(env.chatID)))

	env.wantReply(t, "staff_post_as_yourself")
	env.wantNoRow(t)
	if env.lookups() != 0 {
		t.Fatalf("an anonymous admin triggered %d admin or member lookups, want 0", env.lookups())
	}
	if methods := env.methodsCalled(); len(methods) != 1 || methods[0] != "sendMessage" {
		t.Fatalf("Telegram calls = %v, want exactly [sendMessage]", methods)
	}
}

func TestSetStaffRejectsServiceAccountAndMissingUser(t *testing.T) {
	env := newSetStaffEnv(t)
	chat := staffSupergroup(env.chatID)

	env.send(t, chat, 777000, "/setstaff")
	noUser := &gotgbot.Message{MessageId: 101, Date: 1, Chat: chat, Text: "/setstaff"}
	env.run(t, ext.NewContext(env.bot, &gotgbot.Update{UpdateId: 1, Message: noUser}, nil))

	texts := sentTexts(env.client)
	if len(texts) != 2 {
		t.Fatalf("replies = %q, want one refusal per sender", texts)
	}
	if !strings.Contains(texts[0], staffMarker("staff_post_as_yourself")) {
		t.Errorf("service account reply = %q, want staff_post_as_yourself", texts[0])
	}
	// A message with no user at all is stopped by the pipeline before any check
	// runs; either refusal is acceptable as long as nothing else happens.
	if !strings.Contains(texts[1], staffMarker("staff_post_as_yourself")) &&
		!strings.Contains(texts[1], staffMarker("common_cannot_identify_user")) {
		t.Errorf("no-user reply = %q, want a refusal", texts[1])
	}
	env.wantNoRowAfterRefusals(t)
}

func (e *setStaffEnv) wantNoRowAfterRefusals(t *testing.T) {
	t.Helper()
	e.wantNoRow(t)
	if e.lookups() != 0 {
		t.Fatalf("refused senders triggered %d lookups, want 0", e.lookups())
	}
}

func TestSetStaffRejectsChannelType(t *testing.T) {
	env := newSetStaffEnv(t)
	channel := gotgbot.Chat{Id: env.chatID, Type: "channel", Title: "A Channel"}

	env.send(t, channel, setStaffOwnerID, "/setstaff")

	env.wantReply(t, "staff_refuse_not_group")
	env.wantNoRow(t)
	if env.lookups() != 0 {
		t.Fatalf("a channel triggered %d lookups before the type refusal, want 0", env.lookups())
	}
}

func TestSetStaffPrivateChatGetsGroupOnlyReply(t *testing.T) {
	env := newSetStaffEnv(t)
	private := gotgbot.Chat{Id: setStaffOwnerID, Type: "private", FirstName: "Owner"}

	env.send(t, private, setStaffOwnerID, "/setstaff")

	env.wantReply(t, "chat_status_group_only_error")
	if row, err := staff.GetStaffGroupFresh(private.Id); err != nil || row != nil {
		t.Fatalf("private chat got a row: %+v (err %v)", row, err)
	}
}

func TestSetStaffRejectsBotNotAdmin(t *testing.T) {
	for _, role := range []string{staffRoleMember, staffRoleAbsent, staffRoleLeft, staffRoleKicked} {
		t.Run(role, func(t *testing.T) {
			env := newSetStaffEnv(t)
			env.client.botRole[env.chatID] = role

			env.send(t, staffSupergroup(env.chatID), setStaffOwnerID, "/setstaff")

			env.wantReply(t, "staff_refuse_bot_not_admin")
			env.wantNoRow(t)
		})
	}
}

func TestSetStaffAcceptsBotAdminWithoutRestrictRights(t *testing.T) {
	env := newSetStaffEnv(t)
	env.client.botRole[env.chatID] = staffRoleAdminNoRestrict

	env.send(t, staffSupergroup(env.chatID), setStaffOwnerID, "/setstaff")

	env.wantReply(t, "staff_set_done")
	if row, err := staff.GetStaffGroupFresh(env.chatID); err != nil || row == nil {
		t.Fatalf("row = (%+v, %v), want a Staff Group (the bot only needs to be an admin)", row, err)
	}
}

func TestSetStaffBotCheckUnknown(t *testing.T) {
	env := newSetStaffEnv(t)
	env.client.setFailure("getChatMember", env.chatID, &gotgbot.TelegramError{
		Method: "getChatMember", Code: 429, Description: "Too Many Requests: retry after 5",
	})

	env.send(t, staffSupergroup(env.chatID), setStaffOwnerID, "/setstaff")

	env.wantReply(t, "staff_check_failed")
	env.wantNoRow(t)
}

func TestSetStaffRejectsLinkedGroup(t *testing.T) {
	env := newSetStaffEnv(t)
	otherStaff := uniqueModuleChatID()
	staffCleanup(t, otherStaff)
	link := &models.StaffGroupLink{GroupChatID: env.chatID, StaffChatID: otherStaff, OwnerUserID: 1}
	if err := db.DB.Create(link).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	env.send(t, staffSupergroup(env.chatID), setStaffOwnerID, "/setstaff")

	env.wantReply(t, "staff_refuse_group_is_linked")
	env.wantNoRow(t)
	var links int64
	if err := db.DB.Model(&models.StaffGroupLink{}).Where("group_chat_id = ?", env.chatID).Count(&links).Error; err != nil || links != 1 {
		t.Fatalf("link rows = %d (err %v), want the original link untouched", links, err)
	}
}

func TestSetStaffAlreadyStaffIsIdempotent(t *testing.T) {
	env := newSetStaffEnv(t)
	chat := staffSupergroup(env.chatID)

	env.send(t, chat, setStaffOwnerID, "/setstaff")
	env.send(t, chat, setStaffOwnerID, "/setstaff please ignore these arguments 123")

	texts := sentTexts(env.client)
	if len(texts) != 2 ||
		!strings.Contains(texts[0], staffMarker("staff_set_done")) ||
		!strings.Contains(texts[1], staffMarker("staff_set_already")) {
		t.Fatalf("replies = %q, want staff_set_done then staff_set_already", texts)
	}
	var rows []models.StaffGroup
	if err := db.DB.Where("chat_id = ?", env.chatID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OwnerUserID != setStaffOwnerID {
		t.Fatalf("rows = %+v, want exactly one owned by %d", rows, setStaffOwnerID)
	}
}

// The reason reported when several checks fail at once follows a fixed order:
// anonymous, chat type, live creator, bot administrator, role conflict.
func TestSetStaffRefusalOrder(t *testing.T) {
	t.Run("non-creator with bot not admin gets not-owner", func(t *testing.T) {
		env := newSetStaffEnv(t)
		env.client.botRole[env.chatID] = staffRoleMember

		env.send(t, staffSupergroup(env.chatID), setStaffStrangerID, "/setstaff")

		env.wantReply(t, "staff_refuse_not_owner")
	})
	t.Run("channel with non-creator gets not-group", func(t *testing.T) {
		env := newSetStaffEnv(t)

		env.send(t, gotgbot.Chat{Id: env.chatID, Type: "channel", Title: "Chan"}, setStaffStrangerID, "/setstaff")

		env.wantReply(t, "staff_refuse_not_group")
	})
	t.Run("anonymous admin with bot not admin gets post-as-yourself", func(t *testing.T) {
		env := newSetStaffEnv(t)
		// A supergroup-typed anonymous admin whose bot is also not admin.
		env.client.botRole[env.chatID] = staffRoleMember

		env.run(t, anonymousAdminContext(env.bot, staffSupergroup(env.chatID)))

		env.wantReply(t, "staff_post_as_yourself")
	})
	t.Run("creator with bot not admin and linked chat gets bot-not-admin", func(t *testing.T) {
		env := newSetStaffEnv(t)
		env.client.botRole[env.chatID] = staffRoleMember
		otherStaff := uniqueModuleChatID()
		staffCleanup(t, otherStaff)
		link := &models.StaffGroupLink{GroupChatID: env.chatID, StaffChatID: otherStaff, OwnerUserID: 1}
		if err := db.DB.Create(link).Error; err != nil {
			t.Fatalf("create link: %v", err)
		}

		env.send(t, staffSupergroup(env.chatID), setStaffOwnerID, "/setstaff")

		env.wantReply(t, "staff_refuse_bot_not_admin")
	})
}
