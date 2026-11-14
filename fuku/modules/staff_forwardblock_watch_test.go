//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// TestForwardBlockWatcherDeletes is the tracer: with a Staff Group, a link, a listed channel and the
// action none, a member's message forwarded from that channel into the linked group is deleted
// through the real dispatcher and nothing else is sent or written.
func TestForwardBlockWatcherDeletes(t *testing.T) {
	env := newFwdWatchEnv(t, "none")
	member := env.member()

	msg := env.forward(member)

	if got := env.deletedIDs(); len(got) != 1 || got[0] != msg.MessageId {
		t.Fatalf("deleted messages = %v, want only %d", got, msg.MessageId)
	}
	if moderation, sent := env.groupWrites(); moderation != 0 || sent != 0 {
		t.Fatalf("the bot moderated %d and sent %d, want a delete only", moderation, sent)
	}
	if lines := env.warnLines(); len(lines) != 0 {
		t.Fatalf("a clean delete logged %q", strings.Join(lines, "; "))
	}
}

// fwdForbiddenInText fails when text names the source, the Staff Group or the linked group's title.
func fwdForbiddenInText(t *testing.T, env *fwdWatchEnv, what, text string) {
	t.Helper()
	for _, forbidden := range []string{"Blocked", "Staff HQ", fmt.Sprint(env.source), fmt.Sprint(env.staffChat)} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("%s names %q: %q", what, forbidden, text)
		}
	}
}

// fwdNoticeRow reads the queue row of a notice the bot sent to the linked group.
func fwdNoticeRow(t *testing.T, env *fwdWatchEnv, messageID int64) models.ScheduledDeletion {
	t.Helper()
	var row models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ? AND message_id = ?", env.group, messageID).First(&row).Error; err != nil {
		t.Fatalf("queue row of notice %d: %v", messageID, err)
	}
	return row
}

func TestForwardBlockWatcherPunishments(t *testing.T) {
	env := newFwdWatchEnv(t, "warn")
	env.fake.sendDate = func() int64 { return env.clock.Now().Unix() }
	if err := deletion.SetRule(context.Background(), env.group, deletion.KindBotAuto, 60); err != nil {
		t.Fatalf("set the punishment notice rule: %v", err)
	}
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", env.group).Delete(&models.ScheduledDeletion{})
		db.DB.Where("chat_id = ?", env.group).Delete(&models.DeleteRule{})
		dbcache.DeleteCache(dbcache.CacheKey("delete_rules", env.group))
	})
	reasonMarker := staffMarker("staff_fwdblock_reason")

	t.Run("warn", func(t *testing.T) {
		member := env.member()
		msg := env.forward(member)
		if got := env.deletedIDs(); len(got) != 1 || got[0] != msg.MessageId {
			t.Fatalf("deleted messages = %v, want only %d", got, msg.MessageId)
		}
		count, reasons := warns.GetWarns(member.Id, env.group)
		if count != 1 || len(reasons) != 1 || !strings.Contains(reasons[0], reasonMarker) {
			t.Fatalf("warns = %d %q, want one warn with the blocked source reason", count, reasons)
		}
		fwdForbiddenInText(t, env, "the warn reason", reasons[0])
		for _, sent := range env.fake.sentTo(env.group) {
			fwdForbiddenInText(t, env, "the warn message", fmt.Sprint(sent.Params["text"]))
		}
		if moderation, _ := env.groupWrites(); moderation != 0 {
			t.Fatalf("a warn below the limit moderated %d times", moderation)
		}
	})

	notices := []struct {
		action string
		key    string
		method string
	}{
		{"mute", "blacklists_bl_watcher_muted_user", "restrictChatMember"},
		{"kick", "blacklists_bl_watcher_kicked_user", "unbanChatMember"},
		{"ban", "blacklists_bl_watcher_banned_user", "banChatMember"},
	}
	for _, tc := range notices {
		t.Run(tc.action, func(t *testing.T) {
			env.setAction(tc.action)
			member := env.member()
			sentBefore := len(env.fake.sentTo(env.group))
			callsBefore := len(env.callsTo(tc.method, env.group))
			msg := env.forward(member)

			if got := env.deletedIDs(); got[len(got)-1] != msg.MessageId {
				t.Fatalf("the forward %d was not deleted: %v", msg.MessageId, got)
			}
			calls := env.callsTo(tc.method, env.group)
			if len(calls) != callsBefore+1 {
				t.Fatalf("%s calls = %d, want one more than %d", tc.method, len(calls), callsBefore)
			}
			call := calls[len(calls)-1]
			if staffParamInt(call.Params, "user_id") != member.Id || staffParamInt(call.Params, "until_date") != 0 {
				t.Fatalf("%s params = %v, want the member and no end date", tc.method, call.Params)
			}
			switch tc.action {
			case "mute":
				if got, _ := call.Params["permissions"].(gotgbot.ChatPermissions); !reflect.DeepEqual(got, MutedPermissions) {
					t.Fatalf("the mute permissions = %+v, want MutedPermissions", got)
				}
			case "kick":
				if staffParamBool(call.Params, "only_if_banned") {
					t.Fatalf("a kick must remove a current member: %v", call.Params)
				}
			}
			sent := env.fake.sentTo(env.group)
			if len(sent) != sentBefore+1 {
				t.Fatalf("notices sent = %d, want exactly one more than %d", len(sent), sentBefore)
			}
			notice := sent[len(sent)-1]
			text := fmt.Sprint(notice.Params["text"])
			if !strings.Contains(text, staffMarker(tc.key)) || !strings.Contains(text, reasonMarker) {
				t.Fatalf("the notice = %q, want %s and the blocked source reason", text, tc.key)
			}
			fwdForbiddenInText(t, env, "the notice", text)
			row := fwdNoticeRow(t, env, notice.MessageID)
			if row.Kind != deletion.KindBotAuto {
				t.Fatalf("notice queued as %q, want %q", row.Kind, deletion.KindBotAuto)
			}
		})
	}

	t.Run("album", func(t *testing.T) {
		env.setAction("mute")
		member := env.member()
		restrictBefore := len(env.callsTo("restrictChatMember", env.group))
		var ids []int64
		for i := 0; i < 3; i++ {
			update := env.forwardUpdate(env.groupChat, &member, channelOrigin(env.source))
			update.Message.MediaGroupId = "album-1"
			env.process(update)
			ids = append(ids, update.Message.MessageId)
		}
		deleted := env.deletedIDs()
		for _, id := range ids {
			found := false
			for _, got := range deleted {
				found = found || got == id
			}
			if !found {
				t.Fatalf("album item %d was not deleted: %v", id, deleted)
			}
		}
		if got := len(env.callsTo("restrictChatMember", env.group)) - restrictBefore; got != 1 {
			t.Fatalf("an album of three punished %d times, want once", got)
		}
		env.clock.advance(forwardBlockPunishWindow + time.Second)
		env.forward(member)
		if got := len(env.callsTo("restrictChatMember", env.group)) - restrictBefore; got != 2 {
			t.Fatalf("a forward after the window punished %d times in all, want 2", got)
		}
	})
}

func TestForwardBlockWatcherSpares(t *testing.T) {
	env := newFwdWatchEnv(t, "ban")
	other := uniqueModuleChatID()
	unlinked := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Unlinked"}
	telegram := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	channelChat := gotgbot.Chat{Id: env.source, Type: "channel", Title: fwdSourceTitle}

	type spare struct {
		name  string
		chat  int64
		setup func(user gotgbot.User)
		build func(user gotgbot.User) *gotgbot.Update
	}
	forwarded := func(chat gotgbot.Chat, origin gotgbot.MessageOrigin) func(gotgbot.User) *gotgbot.Update {
		return func(user gotgbot.User) *gotgbot.Update { return env.forwardUpdate(chat, &user, origin) }
	}
	asUser := func(user gotgbot.User, mutate func(*gotgbot.Message)) func(gotgbot.User) *gotgbot.Update {
		return func(gotgbot.User) *gotgbot.Update {
			update := env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source))
			mutate(update.Message)
			return update
		}
	}
	nothing := func(*gotgbot.Message) {}

	cases := []spare{
		{name: "not in the list", chat: env.group, build: forwarded(env.groupChat, channelOrigin(other))},
		{name: "unlinked group", chat: unlinked.Id, build: forwarded(unlinked, channelOrigin(env.source))},
		{name: "the Staff Group itself", chat: env.staffChat, build: forwarded(env.staffChatObj(), channelOrigin(env.source))},
		{name: "not a forward", chat: env.group, build: func(user gotgbot.User) *gotgbot.Update {
			return env.messageUpdate(env.groupChat, &user, "plain")
		}},
		{name: "a user origin", chat: env.group, build: forwarded(env.groupChat,
			gotgbot.MessageOriginUser{Date: 1, SenderUser: gotgbot.User{Id: 5, FirstName: "Someone"}})},
		{name: "a hidden user origin", chat: env.group, build: forwarded(env.groupChat,
			gotgbot.MessageOriginHiddenUser{Date: 1, SenderUserName: "Hidden"})},
		{name: "an administrator", chat: env.group,
			setup: func(user gotgbot.User) { env.fake.listAdministrator(env.group, user.Id) },
			build: forwarded(env.groupChat, channelOrigin(env.source))},
		{name: "an approved user", chat: env.group,
			setup: func(user gotgbot.User) {
				if err := approvals.AddApprovedUser(env.group, user.Id, env.owner, ""); err != nil {
					t.Fatalf("approve: %v", err)
				}
			},
			build: forwarded(env.groupChat, channelOrigin(env.source))},
		{name: "an anonymous admin", chat: env.group,
			build: asUser(gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"}, func(m *gotgbot.Message) {
				m.SenderChat = &env.groupChat
			})},
		{name: "a channel identity", chat: env.group,
			build: asUser(gotgbot.User{Id: 136817688, IsBot: true, FirstName: "Channel"}, func(m *gotgbot.Message) {
				m.SenderChat = &channelChat
			})},
		{name: "an automatic forward", chat: env.group,
			build: asUser(telegram, func(m *gotgbot.Message) {
				m.SenderChat = &channelChat
				m.IsAutomaticForward = true
			})},
		{name: "a bot", chat: env.group,
			build: asUser(gotgbot.User{Id: uniqueLinkOwnerID() + 3, IsBot: true, FirstName: "Bot"}, nothing)},
		{name: "a Telegram service account", chat: env.group, build: asUser(telegram, nothing)},
		{name: "the bot itself", chat: env.group,
			build: asUser(gotgbot.User{Id: env.bot.Id, IsBot: true, FirstName: "Fuku"}, nothing)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := env.member()
			if tc.setup != nil {
				tc.setup(user)
			}
			env.process(tc.build(user))
			env.wantUntouchedIn(t, tc.chat, tc.name)
		})
	}

	// The positive control: the same environment acts on a plain member's forward, so each case above
	// was spared for its own reason and not because nothing could act.
	member := env.member()
	env.forward(member)
	if deleted, moderated, _ := env.activity(env.group); deleted != 1 || moderated != 1 {
		t.Fatalf("a plain member's forward: deleted %d, moderated %d, want 1 and 1", deleted, moderated)
	}
}

func TestForwardBlockWatcherUnknownNeverActs(t *testing.T) {
	env := newFwdWatchEnv(t, "ban")
	recheck := func(result staffRecheckResult) func(*gotgbot.Bot, models.StaffGroupLink, *staffOwnerPass) staffRecheckResult {
		return func(*gotgbot.Bot, models.StaffGroupLink, *staffOwnerPass) staffRecheckResult { return result }
	}
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{"an unreadable list", func(t *testing.T) {
			mreqSwap(t, &forwardBlockRulesLookup, func(context.Context, int64) (staff.ForwardBlockRules, error) {
				return staff.ForwardBlockRules{}, errors.New("database is down")
			})
		}},
		{"an approvals error", func(t *testing.T) {
			mreqSwap(t, &memberDeletionApprovedLookup, func(context.Context, int64, int64) (bool, error) {
				return false, errors.New("approvals unreadable")
			})
		}},
		{"an unknown admin status", func(t *testing.T) {
			cache.InvalidateAdminCache(env.group)
			mreqSwap(t, &memberDeletionAdminWarm, func(*gotgbot.Bot, int64) {})
			env.fake.script("getChatMember", env.group, staffFakeError(500, "Internal Server Error"))
		}},
		{"a failed fresh link read", func(t *testing.T) {
			mreqSwap(t, &forwardBlockFreshLink, func(int64) (*models.StaffGroupLink, error) {
				return nil, errors.New("database is down")
			})
		}},
		{"a link that moved to another Staff Group", func(t *testing.T) {
			mreqSwap(t, &forwardBlockFreshLink, func(chatID int64) (*models.StaffGroupLink, error) {
				return &models.StaffGroupLink{ID: 1, GroupChatID: chatID, StaffChatID: env.staffChat - 1, OwnerUserID: env.owner}, nil
			})
		}},
		{"a recheck that is unknown", func(t *testing.T) { mreqSwap(t, &forwardBlockRecheck, recheck(staffRecheckUnknown)) }},
		{"a recheck that removed the link", func(t *testing.T) { mreqSwap(t, &forwardBlockRecheck, recheck(staffRecheckRemoved)) }},
		{"a recheck that found the link gone", func(t *testing.T) { mreqSwap(t, &forwardBlockRecheck, recheck(staffRecheckGone)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetForwardBlockState(t)
			tc.setup(t)
			user := env.member()
			env.process(env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source)))
			env.wantUntouched(t, tc.name)
		})
	}

	t.Run("an unreadable list logs once an hour", func(t *testing.T) {
		resetForwardBlockState(t)
		env.logs.Reset()
		mreqSwap(t, &forwardBlockRulesLookup, func(context.Context, int64) (staff.ForwardBlockRules, error) {
			return staff.ForwardBlockRules{}, errors.New("database is down")
		})
		for i := 0; i < 3; i++ {
			user := env.member()
			env.process(env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source)))
		}
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], fmt.Sprint(env.group)) || strings.Contains(lines[0], "database is down") {
			t.Fatalf("log lines = %q, want one line with the chat and no error text", lines)
		}
	})
}

func TestForwardBlockWatcherAuthority(t *testing.T) {
	staffAdminChecks := func(env *fwdWatchEnv) int {
		return len(env.callsTo("getChatAdministrators", env.staffChat))
	}

	t.Run("the owner changed", func(t *testing.T) {
		env := newFwdWatchEnv(t, "ban")
		env.fake.setCreator(env.group, uniqueLinkOwnerID()+9)
		user := env.member()
		env.process(env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source)))
		env.wantUntouched(t, "a link whose maker no longer owns the group")
		link, err := staff.GetLinkOfGroupFresh(env.group)
		if err != nil || link != nil {
			t.Fatalf("link after the recheck = (%v, %v), want it removed", link, err)
		}
		other := env.member()
		env.process(env.forwardUpdate(env.groupChat, &other, channelOrigin(env.source)))
		env.wantUntouched(t, "a later forward in the now unlinked group")
	})

	t.Run("the memo", func(t *testing.T) {
		env := newFwdWatchEnv(t, "none")
		env.forward(env.member())
		first := staffAdminChecks(env)
		if first == 0 {
			t.Fatal("the first listed forward made no owner check of the Staff Group")
		}
		env.forward(env.member())
		if got := staffAdminChecks(env); got != first {
			t.Fatalf("a second forward inside the memo made %d owner checks, want none", got-first)
		}
		env.clock.advance(forwardBlockAuthMemoTTL + time.Second)
		env.forward(env.member())
		second := staffAdminChecks(env)
		if second <= first {
			t.Fatal("a forward after the memo expired made no owner check")
		}
		link, err := staff.GetLinkOfGroupFresh(env.group)
		if err != nil || link == nil {
			t.Fatalf("link = (%v, %v)", link, err)
		}
		if deleted, err := staff.DeleteLinkIfOwner(link.ID, link.OwnerUserID); err != nil || !deleted {
			t.Fatalf("delete the link = (%v, %v)", deleted, err)
		}
		if err := staff.CreateLink(&models.StaffGroupLink{
			GroupChatID: env.group, StaffChatID: env.staffChat, OwnerUserID: env.owner, GroupTitle: "Linked Group",
		}); err != nil {
			t.Fatalf("make the link again: %v", err)
		}
		env.forward(env.member())
		if got := staffAdminChecks(env); got <= second {
			t.Fatal("a forward after the link row changed made no owner check")
		}
	})

	t.Run("unknown is not remembered", func(t *testing.T) {
		env := newFwdWatchEnv(t, "none")
		calls := 0
		mreqSwap(t, &forwardBlockRecheck, func(b *gotgbot.Bot, link models.StaffGroupLink, pass *staffOwnerPass) staffRecheckResult {
			calls++
			if calls == 1 {
				return staffRecheckUnknown
			}
			return recheckLink(b, link, pass)
		})
		env.forward(env.member())
		if deleted, _, _ := env.activity(env.group); deleted != 0 {
			t.Fatalf("an unknown recheck deleted %d messages", deleted)
		}
		env.forward(env.member())
		if deleted, _, _ := env.activity(env.group); deleted != 1 || calls != 2 {
			t.Fatalf("after the retry deleted = %d, rechecks = %d, want 1 and 2", deleted, calls)
		}
		env.forward(env.member())
		if calls != 2 {
			t.Fatalf("a forward inside the memo rechecked again: %d calls", calls)
		}
	})
}

func TestForwardBlockWatcherBotRights(t *testing.T) {
	t.Run("not an administrator", func(t *testing.T) {
		env := newFwdWatchEnv(t, "ban")
		env.fake.setBotRole(env.group, staffRoleMember)
		env.forward(env.member())
		env.forward(env.member())
		env.wantUntouched(t, "a bot that is not an administrator")
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], "not an administrator") {
			t.Fatalf("log lines = %q, want one cannot-act line", lines)
		}
	})

	t.Run("no delete right still punishes", func(t *testing.T) {
		env := newFwdWatchEnv(t, "mute")
		env.client.takeDeleteRight(env.group)
		env.forward(env.member())
		env.forward(env.member())
		deleted, moderated, sent := env.activity(env.group)
		if deleted != 0 || moderated != 2 || sent != 2 {
			t.Fatalf("deleted %d, moderated %d, sent %d, want 0, 2, 2", deleted, moderated, sent)
		}
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], "no delete right") {
			t.Fatalf("log lines = %q, want one no-delete-right line", lines)
		}
	})

	t.Run("no restrict right still deletes", func(t *testing.T) {
		env := newFwdWatchEnv(t, "mute")
		env.fake.setBotRole(env.group, staffRoleAdminNoRestrict)
		env.forward(env.member())
		env.forward(env.member())
		deleted, moderated, sent := env.activity(env.group)
		if deleted != 2 || moderated != 0 || sent != 0 {
			t.Fatalf("deleted %d, moderated %d, sent %d, want 2, 0, 0", deleted, moderated, sent)
		}
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], "no restrict right") {
			t.Fatalf("log lines = %q, want one no-restrict-right line", lines)
		}
	})

	t.Run("a refused delete is logged once and never retried", func(t *testing.T) {
		env := newFwdWatchEnv(t, "mute")
		env.fake.script("deleteMessage", env.group, staffFakeError(403, "Forbidden: not enough rights"),
			staffFakeError(403, "Forbidden: not enough rights"))
		first, second := env.member(), env.member()
		env.forward(first)
		env.forward(second)
		deleted, moderated, _ := env.activity(env.group)
		if deleted != 2 || moderated != 2 {
			t.Fatalf("deleted %d, moderated %d, want one delete call and one mute per forward", deleted, moderated)
		}
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], "telegram error 403") || !strings.Contains(lines[0], fmt.Sprint(env.group)) {
			t.Fatalf("log lines = %q, want one line with the chat and the error code", lines)
		}
		fwdForbiddenInText(t, env, "the log line", lines[0])
	})

	t.Run("a refused punishment is logged once and never retried", func(t *testing.T) {
		env := newFwdWatchEnv(t, "mute")
		env.fake.script("restrictChatMember", env.group, staffFakeError(400, "Bad Request: not enough rights"))
		member := env.member()
		env.forward(member)
		env.forward(member)
		if got := len(env.callsTo("restrictChatMember", env.group)); got != 1 {
			t.Fatalf("restrict calls = %d, want the refused one and no retry", got)
		}
		if _, _, sent := env.activity(env.group); sent != 0 {
			t.Fatalf("a refused mute sent %d notices", sent)
		}
		lines := env.warnLines()
		if len(lines) != 1 || !strings.Contains(lines[0], "telegram error 400") || !strings.Contains(lines[0], fmt.Sprint(member.Id)) {
			t.Fatalf("log lines = %q, want one line with the user and the error code", lines)
		}
	})
}

func TestForwardBlockWatcherNeverEndsUpdate(t *testing.T) {
	env := newFwdWatchEnv(t, "mute")
	siblings := 0
	env.dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, func(*gotgbot.Bot, *ext.Context) error {
		siblings++
		return ext.ContinueGroups
	}), 1)

	run := func(update *gotgbot.Update) error {
		return staffModule.forwardBlockWatcher(env.bot, ext.NewContext(env.bot, update, nil))
	}
	user := env.member()
	paths := map[string]*gotgbot.Update{
		"an action":     env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source)),
		"a spare":       env.messageUpdate(env.groupChat, &user, "plain"),
		"unlisted":      env.forwardUpdate(env.groupChat, &user, channelOrigin(uniqueModuleChatID())),
		"a no message":  {UpdateId: 1},
		"a private one": env.forwardUpdate(gotgbot.Chat{Id: 5, Type: "private"}, &user, channelOrigin(env.source)),
	}
	for name, update := range paths {
		if got := run(update); got != ext.ContinueGroups {
			t.Fatalf("%s returned %v, want ContinueGroups", name, got)
		}
	}

	mreqSwap(t, &forwardBlockRulesLookup, func(context.Context, int64) (staff.ForwardBlockRules, error) {
		panic("injected panic")
	})
	if got := run(env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source))); got != ext.ContinueGroups {
		t.Fatalf("a recovered panic returned %v, want ContinueGroups", got)
	}
	before := siblings
	other := env.member()
	env.process(env.forwardUpdate(env.groupChat, &other, channelOrigin(env.source)))
	if siblings != before+1 {
		t.Fatalf("a handler of a later group ran %d times after the panic, want once", siblings-before)
	}
}

func TestForwardBlockWatcherCost(t *testing.T) {
	env := newFwdWatchEnv(t, "ban")
	links, rules := 0, 0
	realLink, realRules := forwardBlockLinkLookup, forwardBlockRulesLookup
	mreqSwap(t, &forwardBlockLinkLookup, func(chatID int64) *models.StaffGroupLink {
		links++
		return realLink(chatID)
	})
	mreqSwap(t, &forwardBlockRulesLookup, func(ctx context.Context, staffChatID int64) (staff.ForwardBlockRules, error) {
		rules++
		return realRules(ctx, staffChatID)
	})
	user := env.member()
	unlinked := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Unlinked"}

	for name, update := range map[string]*gotgbot.Update{
		"a non-forward":      env.messageUpdate(env.groupChat, &user, "plain"),
		"a user forward":     env.forwardUpdate(env.groupChat, &user, gotgbot.MessageOriginUser{Date: 1, SenderUser: gotgbot.User{Id: 5}}),
		"a hidden user":      env.forwardUpdate(env.groupChat, &user, gotgbot.MessageOriginHiddenUser{Date: 1, SenderUserName: "H"}),
		"a staff group chat": env.forwardUpdate(env.staffChatObj(), &user, gotgbot.MessageOriginUser{Date: 1}),
	} {
		calls := env.totalCalls()
		env.process(update)
		if links != 0 || rules != 0 || env.totalCalls() != calls {
			t.Fatalf("%s: link reads %d, list reads %d, telegram calls %d, want none", name, links, rules, env.totalCalls()-calls)
		}
	}

	calls := env.totalCalls()
	env.process(env.forwardUpdate(unlinked, &user, channelOrigin(env.source)))
	if links != 1 || rules != 0 || env.totalCalls() != calls {
		t.Fatalf("a channel forward in an unlinked group: link reads %d, list reads %d, telegram calls %d, want 1, 0, 0",
			links, rules, env.totalCalls()-calls)
	}
}

func TestForwardBlockSourceOf(t *testing.T) {
	chat := func(id int64, kind string) gotgbot.Chat { return gotgbot.Chat{Id: id, Type: kind} }
	cases := []struct {
		name   string
		origin gotgbot.MessageOrigin
		want   int64
		ok     bool
	}{
		{"a channel", gotgbot.MessageOriginChannel{Chat: chat(-1001, "channel")}, -1001, true},
		{"a supergroup admin post", gotgbot.MessageOriginChat{SenderChat: chat(-1002, "supergroup")}, -1002, true},
		{"a group admin post", gotgbot.MessageOriginChat{SenderChat: chat(-3, "group")}, -3, true},
		{"a chat of the wrong type", gotgbot.MessageOriginChat{SenderChat: chat(-4, "private")}, 0, false},
		{"a channel with a positive ID", gotgbot.MessageOriginChannel{Chat: chat(5, "channel")}, 0, false},
		{"a user", gotgbot.MessageOriginUser{SenderUser: gotgbot.User{Id: 7}}, 0, false},
		{"a hidden user", gotgbot.MessageOriginHiddenUser{SenderUserName: "x"}, 0, false},
		{"no origin", nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := forwardBlockSourceOf(&gotgbot.Message{ForwardOrigin: tc.origin})
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%s = (%d, %v), want (%d, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := forwardBlockSourceOf(nil); ok {
		t.Fatal("a nil message has no source")
	}
}

func TestForwardBlockWatcherGroup(t *testing.T) {
	if staffForwardBlockGroup != -9 {
		t.Fatalf("staffForwardBlockGroup = %d, want -9", staffForwardBlockGroup)
	}
	env := newFwdWatchEnv(t, "none")
	var atFirst, atSecond = -1, -1
	env.dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, func(*gotgbot.Bot, *ext.Context) error {
		atFirst = len(env.deletes())
		return ext.ContinueGroups
	}), -10)
	env.dispatcher.AddHandlerToGroup(handlers.NewMessage(message.All, func(*gotgbot.Bot, *ext.Context) error {
		atSecond = len(env.deletes())
		return ext.ContinueGroups
	}), -8)

	user := env.member()
	env.forward(user)
	if atFirst != 0 || atSecond != 1 {
		t.Fatalf("deletes seen at group -10 and -8 = %d and %d, want 0 and 1: the watcher runs between them", atFirst, atSecond)
	}

	edited := env.forwardUpdate(env.groupChat, &user, channelOrigin(env.source))
	edited.EditedMessage, edited.Message = edited.Message, nil
	before := len(env.deletes())
	env.process(edited)
	if got := len(env.deletes()); got != before {
		t.Fatalf("the edit of a forwarded message was deleted (%d calls, was %d)", got, before)
	}
}
