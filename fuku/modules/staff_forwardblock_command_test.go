//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// fwdWantReply sends text as from in chat and requires the reply to carry marker.
func fwdWantReply(t *testing.T, env *staffActionEnv, chat gotgbot.Chat, from gotgbot.User, text, marker string) string {
	t.Helper()
	before := len(env.fake.sentTo(chat.Id))
	env.process(env.messageUpdate(chat, &from, text))
	sent := env.fake.sentTo(chat.Id)
	if len(sent) != before+1 {
		t.Fatalf("%q sent %d messages, want exactly one reply", text, len(sent)-before)
	}
	reply := fmt.Sprint(sent[len(sent)-1].Params["text"])
	if !strings.Contains(reply, staffMarker(marker)) {
		t.Fatalf("%q replied %q, want %s", text, reply, marker)
	}
	return reply
}

func TestForwardBlockCommands(t *testing.T) {
	env := newStaffActionEnv(t, 0)
	owner := gotgbot.User{Id: env.owner, FirstName: "Owner"}
	admin := gotgbot.User{Id: uniqueLinkOwnerID() + 11, FirstName: "Admin"}
	env.fake.listAdministrator(env.staffChat, admin.Id)
	staffChat := env.staffChatObj()
	ctx := context.Background()

	resolver := &fwdResolver{
		answers: map[any]forwardBlockChat{
			"@newschan":           {ID: -1001555000001, Type: "channel", Title: "News <Chan>"},
			int64(-1001555000002): {ID: -1001555000002, Type: "channel", Title: "Second"},
		},
	}
	resolver.install(t)
	rowCount := func() int64 { return fwdRowCount(t, env.staffChat) }
	storedAction := func() string {
		rules, err := staff.GetForwardBlockRulesFresh(ctx, env.staffChat)
		if err != nil {
			t.Fatal(err)
		}
		return rules.Action
	}

	// An empty list and the default action, read by an administrator who is not the creator.
	fwdWantReply(t, env, staffChat, admin, "/fwdblocklist", "staff_fwdblock_list_empty")
	fwdWantReply(t, env, staffChat, admin, "/fwdblockaction", "staff_fwdblock_action_current")

	// The creator adds by @name and by ID; the cached rules are dropped by each write.
	if _, err := staff.GetForwardBlockRules(ctx, env.staffChat); err != nil {
		t.Fatal(err)
	}
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock @newschan", "staff_fwdblock_added")
	rules, err := staff.GetForwardBlockRules(ctx, env.staffChat)
	if err != nil || !rules.IsBlocked(-1001555000001) {
		t.Fatalf("the cached rules after an add = (%+v, %v), want the new source", rules, err)
	}
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock -1001555000002", "staff_fwdblock_added")
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock -1001555000002", "staff_fwdblock_already")
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock", "staff_fwdblock_usage_add")
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock one two", "staff_fwdblock_usage_add")
	fwdWantReply(t, env, staffChat, owner, "/addfwdblock @unknownone", "staff_fwdblock_not_found")
	if rowCount() != 2 {
		t.Fatalf("%d sources stored, want 2", rowCount())
	}

	// The list shows IDs in code and the escaped title, to any administrator.
	list := fwdWantReply(t, env, staffChat, admin, "/fwdblocklist", "staff_fwdblock_list_header")
	for _, want := range []string{"<code>-1001555000001</code>", "News &lt;Chan&gt;", "<code>-1001555000002</code>"} {
		if !strings.Contains(list, want) {
			t.Errorf("the list %q lacks %q", list, want)
		}
	}

	// The action: read, set, refuse an unknown word, and the stored state follows.
	fwdWantReply(t, env, staffChat, owner, "/fwdblockaction BAN", "staff_fwdblock_action_changed")
	if storedAction() != "ban" {
		t.Fatalf("stored action = %q, want ban", storedAction())
	}
	if r, _ := staff.GetForwardBlockRules(ctx, env.staffChat); r.Action != "ban" {
		t.Fatalf("the cached action after a change = %q, want ban", r.Action)
	}
	fwdWantReply(t, env, staffChat, owner, "/fwdblockaction tban", "staff_fwdblock_action_usage")
	fwdWantReply(t, env, staffChat, owner, "/fwdblockaction warn mute", "staff_fwdblock_action_usage")
	if storedAction() != "ban" {
		t.Fatalf("a refused action changed the stored one to %q", storedAction())
	}

	// A live administrator who is not the creator can read but every write is refused.
	lookups := len(resolver.calls)
	getChatCalls := len(env.callsTo("getChat", env.staffChat))
	for _, text := range []string{"/addfwdblock -1001555000003", "/addfwdblock @newschan", "/rmfwdblock -1001555000001", "/fwdblockaction none"} {
		fwdWantReply(t, env, staffChat, admin, text, "staff_fwdblock_owner_only")
	}
	if rowCount() != 2 || storedAction() != "ban" {
		t.Fatalf("a refused write changed state: %d rows, action %q", rowCount(), storedAction())
	}
	if len(resolver.calls) != lookups || len(env.callsTo("getChat", env.staffChat)) != getChatCalls {
		t.Fatal("a refused write reached Telegram")
	}
	if r, _ := staff.GetForwardBlockRules(ctx, env.staffChat); len(r.Sources) != 2 || r.Action != "ban" {
		t.Fatalf("the cache disagrees with the stored state after refused writes: %+v", r)
	}

	// Remove by the ID the list shows; a second removal says it is not listed.
	fwdWantReply(t, env, staffChat, owner, "/rmfwdblock -1001555000001", "staff_fwdblock_removed")
	fwdWantReply(t, env, staffChat, owner, "/rmfwdblock -1001555000001", "staff_fwdblock_not_listed")
	fwdWantReply(t, env, staffChat, owner, "/rmfwdblock", "staff_fwdblock_usage_rm")
	fwdWantReply(t, env, staffChat, owner, "/rmfwdblock abc", "staff_fwdblock_usage_rm")
	if r, _ := staff.GetForwardBlockRules(ctx, env.staffChat); r.IsBlocked(-1001555000001) || !r.IsBlocked(-1001555000002) {
		t.Fatalf("the cache after a removal = %+v", r)
	}
}

func TestForwardBlockCommandRefusals(t *testing.T) {
	env := newStaffActionEnv(t, 0)
	owner := gotgbot.User{Id: env.owner, FirstName: "Owner"}
	staffChat := env.staffChatObj()
	resolver := &fwdResolver{answers: map[any]forwardBlockChat{int64(-1001555000009): {ID: -1001555000009, Type: "channel", Title: "C"}}}
	resolver.install(t)

	t.Run("anonymous sender", func(t *testing.T) {
		anonymous := gotgbot.User{Id: 1087968824, FirstName: "Group"}
		fwdWantReply(t, env, staffChat, anonymous, "/addfwdblock -1001555000009", "staff_post_as_yourself")
	})

	t.Run("a group that is not a Staff Group", func(t *testing.T) {
		plain := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Plain"}
		staffCleanup(t, plain.Id)
		env.fake.setCreator(plain.Id, env.owner)
		fwdWantReply(t, env, plain, owner, "/addfwdblock -1001555000009", "staff_unset_not_staff")
		fwdWantReply(t, env, plain, owner, "/fwdblocklist", "staff_unset_not_staff")
		if fwdRowCount(t, plain.Id) != 0 {
			t.Fatal("a row was stored for a chat that is not a Staff Group")
		}
	})

	t.Run("a private chat", func(t *testing.T) {
		private := gotgbot.Chat{Id: env.owner, Type: "private", FirstName: "Owner"}
		before := len(env.fake.sentTo(private.Id))
		env.process(env.messageUpdate(private, &owner, "/addfwdblock -1001555000009"))
		if got := fwdRowCount(t, private.Id); got != 0 {
			t.Fatalf("%d rows stored for a private chat", got)
		}
		if after := len(env.fake.sentTo(private.Id)); after != before+1 {
			t.Fatalf("a private chat got %d replies, want the group-only refusal", after-before)
		}
	})

	t.Run("an unknown owner check stores nothing", func(t *testing.T) {
		// Warm the admin cache so the one scripted failure lands on the live owner check.
		fwdWantReply(t, env, staffChat, owner, "/fwdblocklist", "staff_fwdblock_list_empty")
		env.fake.script("getChatAdministrators", env.staffChat, staffFake429(1))
		fwdWantReply(t, env, staffChat, owner, "/addfwdblock -1001555000009", "staff_check_failed")
		env.fake.script("getChatAdministrators", env.staffChat, staffFake429(1))
		fwdWantReply(t, env, staffChat, owner, "/fwdblockaction mute", "staff_check_failed")
		rules, err := staff.GetForwardBlockRulesFresh(context.Background(), env.staffChat)
		if err != nil || len(rules.Sources) != 0 || rules.Action != "none" {
			t.Fatalf("an unknown owner check changed state: (%+v, %v)", rules, err)
		}
	})

	t.Run("a plain member is not an administrator", func(t *testing.T) {
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 31, FirstName: "Member"}
		before := len(env.fake.sentTo(env.staffChat))
		env.process(env.messageUpdate(staffChat, &member, "/addfwdblock -1001555000009"))
		if len(env.fake.sentTo(env.staffChat)) != before+1 || fwdRowCount(t, env.staffChat) != 0 {
			t.Fatal("a plain member's add was not refused with one reply and no write")
		}
	})
}

func TestForwardBlockCommandsRegistered(t *testing.T) {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
	LoadStaff(dispatcher)
	for _, name := range []string{"addfwdblock", "rmfwdblock", "fwdblocklist", "fwdblockaction"} {
		if !helpers.IsBotCommand(name) {
			t.Errorf("IsBotCommand(%q) = false after LoadStaff", name)
		}
	}
}

// TestForwardBlockListText covers the list command's text at its limits: the more-line past 40
// sources and an entry with no title.
func TestForwardBlockListText(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	rows := make([]models.StaffForwardBlock, 43)
	for i := range rows {
		rows[i] = models.StaffForwardBlock{SourceChatID: -1002000000000 - int64(i)}
	}
	text := forwardBlockListText(tr, rows, "none")
	if strings.Count(text, "<code>") != forwardBlockListMax || !strings.Contains(text, staffMarker("staff_fwdblock_list_more")) {
		t.Fatalf("the list holds %d entries and %q, want %d entries and the more-line", strings.Count(text, "<code>"), text, forwardBlockListMax)
	}
	if strings.Contains(text, "<code>-1002000000040</code>") {
		t.Fatal("the list shows an entry past the cap")
	}
	if got := forwardBlockListText(tr, nil, "none"); !strings.Contains(got, staffMarker("staff_fwdblock_list_empty")) {
		t.Fatalf("empty list = %q", got)
	}
}
