//go:build testtools

package modules

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// memberSelfDestructRuleDelay is the delay of the member:selfdestruct rule most tests store.
const memberSelfDestructRuleDelay = 600

// selfDestructClock moves deletionNow for the test and returns the setter; the message dates the
// env sends and the worker's clock both follow it.
func selfDestructClock(t *testing.T, start time.Time) func(time.Time) {
	t.Helper()
	current := start
	mreqSwap(t, &deletionNow, func() time.Time { return current })
	return func(at time.Time) { current = at }
}

// TestMemberSelfDestructQueuesMessages is the plan's tracer (D-09, D-10, D-27, D-29): every kind of
// member message is offered by the watcher with no write, the flush writes one pending row each at
// the message's own date plus the delay, and the real worker deletes them at plus 601 s and not at
// plus 599 s. An administrator's and an approved member's messages are never queued, and a queued
// message named by a pinned_message service message loses its row.
func TestMemberSelfDestructQueuesMessages(t *testing.T) {
	t.Run("every member message is queued and deleted on time", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		member := env.member()
		via := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Inline"}
		album := "album-" + time.Now().Format("150405.000000000")
		kinds := []struct {
			name  string
			apply func(*gotgbot.Message)
		}{
			{"text", func(m *gotgbot.Message) { m.Text = "hello" }},
			{"photo", func(m *gotgbot.Message) {
				m.Photo = []gotgbot.PhotoSize{{FileId: "f", FileUniqueId: "u", Width: 1, Height: 1}}
			}},
			{"sticker", func(m *gotgbot.Message) { m.Sticker = &gotgbot.Sticker{FileId: "s", FileUniqueId: "su"} }},
			{"poll", func(m *gotgbot.Message) { m.Poll = &gotgbot.Poll{Id: "p"} }},
			{"forward", func(m *gotgbot.Message) {
				m.ForwardOrigin = gotgbot.MessageOriginHiddenUser{Date: 1, SenderUserName: "hidden"}
			}},
			{"reply", func(m *gotgbot.Message) {
				m.Text = "reply"
				m.ReplyToMessage = &gotgbot.Message{MessageId: 1, Chat: env.chat}
			}},
			{"via bot", func(m *gotgbot.Message) { m.Text = "via"; m.ViaBot = &via }},
			{"command", commandMessage("/anything now")},
			{"album item one", func(m *gotgbot.Message) {
				m.MediaGroupId = album
				m.Photo = []gotgbot.PhotoSize{{FileId: "a1", FileUniqueId: "a1u", Width: 1, Height: 1}}
			}},
			{"album item two", func(m *gotgbot.Message) {
				m.MediaGroupId = album
				m.Photo = []gotgbot.PhotoSize{{FileId: "a2", FileUniqueId: "a2u", Width: 1, Height: 1}}
			}},
		}
		var ids []int64
		for _, kind := range kinds {
			ids = append(ids, env.sendMember(&member, kind.apply))
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows before the flush = %+v, want none: the watcher writes nothing", rows)
		}
		if got := memberDeleteTestFlush(t); got != len(kinds) {
			t.Fatalf("memberDeleteFlush() = %d, want %d written", got, len(kinds))
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != len(kinds) {
			t.Fatalf("rows after the flush = %d, want %d (one per message)", len(rows), len(kinds))
		}
		wantAt := env.now.Add(memberSelfDestructRuleDelay * time.Second)
		for i, row := range rows {
			if row.State != models.ScheduledDeletionPending || !row.DeleteAt.Equal(wantAt) || !row.SentAt.Equal(env.now) ||
				row.MessageID != ids[i] {
				t.Fatalf("row %d = %+v, want pending message %d sent %s due %s", i, row, ids[i], env.now, wantAt)
			}
		}

		setNow(env.now.Add(599 * time.Second))
		env.pass()
		if calls := env.batchCalls(env.chat.Id); len(calls) != 0 {
			t.Fatalf("deleteMessages calls at date+599s = %v, want none", calls)
		}
		setNow(env.now.Add(601 * time.Second))
		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 {
			t.Fatalf("deleteMessages calls at date+601s = %v, want one batch", calls)
		}
		got := slices.Clone(calls[0])
		slices.Sort(got)
		if !slices.Equal(got, ids) {
			t.Fatalf("deleted ids = %v, want %v", got, ids)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows after the delete = %+v, want none", rows)
		}
	})

	t.Run("an administrator's and an approved member's messages are never queued", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		selfDestructClock(t, env.now)
		approved := env.member()
		if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		env.sendMember(&env.admin, func(m *gotgbot.Message) { m.Text = "admin" })
		env.sendMember(&approved, func(m *gotgbot.Message) { m.Text = "approved" })
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0: nothing was offered", got)
		}
		if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})

	t.Run("a pinned_message service message drops the row of the message it names", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		member := env.member()
		pinned := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "pin me" })
		other := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "not pinned" })
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2", got)
		}
		env.sendPinOf(env.chat, &env.admin, pinned)
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != other {
			t.Fatalf("rows after the pin = %+v, want only message %d", rows, other)
		}
		setNow(env.now.Add(601 * time.Second))
		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0] != other {
			t.Fatalf("deleteMessages calls = %v, want one call with message %d and never the pinned one", calls, other)
		}
	})
}

// storeRawSelfDestruct writes a member:selfdestruct row straight to the table, as a value the apply
// function would refuse, and drops the cache key.
func storeRawSelfDestruct(t *testing.T, chatID int64, delay int) {
	t.Helper()
	if err := db.DB.Where("chat_id = ? AND kind = ?", chatID, deletion.KindMemberSelfDestruct).
		Delete(&models.DeleteRule{}).Error; err != nil {
		t.Fatalf("clear the rule: %v", err)
	}
	row := models.DeleteRule{ChatID: chatID, Kind: deletion.KindMemberSelfDestruct, DelaySecs: delay, UpdatedAt: time.Now().UTC()}
	if err := db.DB.Create(&row).Error; err != nil {
		t.Fatalf("write the rule: %v", err)
	}
	dbcache.DeleteCache(dbcache.CacheKey("delete_rules", chatID))
}

// selfDestructCaptchaCleanup removes a pending-captcha fixture's rows and its pending key.
func selfDestructCaptchaCleanup(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.StoredMessages{})
		db.DB.Where("chat_id = ?", chatID).Delete(&db.CaptchaAttempts{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.CaptchaSettings{})
		dbcache.DeleteCache(dbcache.CacheKey("captcha_settings", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("captcha_pending", chatID))
		dbcache.ResetLocalForTest()
	})
}

// TestMemberSelfDestructKeeps runs a message of every exempt sender and every failing exemption read
// through the real dispatcher with member:selfdestruct at 600 (D-10, D-33): nothing is offered, no
// row exists after a flush, nothing is deleted at date+601 s; the last row is the control that is
// queued.
func TestMemberSelfDestructKeeps(t *testing.T) {
	type fixture struct {
		sender gotgbot.User
		chat   *gotgbot.Chat
		apply  func(*gotgbot.Message)
		// ownDeletes is how many deleteMessage calls the fixture's own module makes (the captcha hold
		// deletes the held message itself).
		ownDeletes int
	}
	tests := []struct {
		name       string
		wantQueued bool
		setup      func(t *testing.T, env *memberDeletionEnv) fixture
	}{
		{name: "an administrator in the cached list", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			return fixture{sender: env.admin}
		}},
		{name: "the creator", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			creator := env.member()
			env.fake.setCreator(env.chat.Id, creator.Id)
			env.primeAdminCache()
			return fixture{sender: creator}
		}},
		{name: "an administrator behind a cold admin cache", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			admin := env.member()
			env.fake.setMemberRaw(env.chat.Id, admin.Id, memberDeletionRecordJSON(admin.Id, "administrator"))
			env.coldAdminCache()
			return fixture{sender: admin}
		}},
		{name: "a member behind a cold cache whose live read answers 429", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			env.failMemberLookup(member.Id, memberDeletionTelegramError(429))
			env.coldAdminCache()
			return fixture{sender: member}
		}},
		{name: "a member behind a cold cache whose live read answers 500", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			env.failMemberLookup(member.Id, memberDeletionTelegramError(500))
			env.coldAdminCache()
			return fixture{sender: member}
		}},
		{name: "an approved user", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
				t.Fatalf("AddApprovedUser error = %v", err)
			}
			return fixture{sender: member}
		}},
		{name: "a member whose approvals read fails", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			mreqSwap(t, &memberDeletionApprovedLookup,
				func(context.Context, int64, int64) (bool, error) { return false, errors.New("approvals unreadable") })
			return fixture{sender: env.member()}
		}},
		{name: "a channel post as the sender", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			channel := gotgbot.Chat{Id: -1001234567890, Type: "channel", Title: "Channel"}
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.SenderChat = &channel }}
		}},
		{name: "an anonymous admin", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			chat := env.chat
			return fixture{
				sender: gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"},
				apply:  func(m *gotgbot.Message) { m.SenderChat = &chat },
			}
		}},
		{name: "a linked channel's automatic forward", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.IsAutomaticForward = true }}
		}},
		{name: "a Telegram service ID", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			return fixture{sender: gotgbot.User{Id: 777000, FirstName: "Telegram"}}
		}},
		{name: "a bot user", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			bot := env.member()
			bot.IsBot = true
			return fixture{sender: bot}
		}},
		{name: "a new_chat_members service message", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			joined := env.member()
			return fixture{sender: member, apply: func(m *gotgbot.Message) { m.NewChatMembers = []gotgbot.User{joined} }}
		}},
		{name: "a chat_title service message", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.NewChatTitle = "A new title" }}
		}},
		{name: "a private chat", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			private := gotgbot.Chat{Id: member.Id, Type: "private"}
			return fixture{sender: member, chat: &private}
		}},
		{name: "a member with a pending captcha", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.dispatcher.AddHandlerToGroup(handlers.NewMessage(nil, captchaModule.handlePendingCaptchaMessage), -10)
			selfDestructCaptchaCleanup(t, env.chat.Id)
			// Group -10 holds a pending member's message only while the captcha is enabled.
			if err := captcha.SetCaptchaEnabled(env.chat.Id, true); err != nil {
				t.Fatalf("SetCaptchaEnabled error = %v", err)
			}
			member := env.member()
			if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "answer", 5); err != nil {
				t.Fatalf("CreateCaptchaAttemptPreMessage error = %v", err)
			}
			return fixture{sender: member, ownDeletes: 1}
		}},
		{name: "a stored delay of 45 is Off", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			storeRawSelfDestruct(t, env.chat.Id, 45)
			return fixture{sender: env.member()}
		}},
		{name: "control: a plain member behind a cold cache is queued", wantQueued: true,
			setup: func(t *testing.T, env *memberDeletionEnv) fixture {
				member := env.member()
				env.fake.setMemberRaw(env.chat.Id, member.Id, memberDeletionRecordJSON(member.Id, "member"))
				env.coldAdminCache()
				return fixture{sender: member}
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
			setNow := selfDestructClock(t, env.now)
			fx := tc.setup(t, env)
			chat := env.chat
			if fx.chat != nil {
				chat = *fx.chat
			}
			id := env.sendMemberIn(chat, &fx.sender, fx.apply)
			written := memberDeleteTestFlush(t)
			if tc.wantQueued {
				rows := env.selfDestructRows(env.chat.Id)
				if written != 1 || len(rows) != 1 || rows[0].MessageID != id {
					t.Fatalf("written = %d, rows = %+v, want message %d queued", written, rows, id)
				}
				return
			}
			if written != 0 {
				t.Fatalf("memberDeleteFlush() = %d, want 0: nothing may be offered", written)
			}
			if rows := env.queuedNoWait(env.chat.Id); len(rows) != 0 {
				t.Fatalf("queue rows = %+v, want none", rows)
			}
			setNow(env.now.Add(601 * time.Second))
			env.pass()
			if calls := env.batchCalls(env.chat.Id); len(calls) != 0 {
				t.Fatalf("deleteMessages calls = %v, want none", calls)
			}
			if got := len(env.deletes(env.chat.Id)); got != fx.ownDeletes {
				t.Fatalf("deleteMessage calls = %d, want %d (the fixture's own)", got, fx.ownDeletes)
			}
		})
	}
}

// TestMemberSelfDestructPinnedStays: a pinned message is never deleted (D-11, T-17-30). A
// pinned_message service message drops the pending self-destruct row of exactly the message it names
// in exactly its own chat, an acting row and rows of other kinds stay, and the bot's own /pin drops
// the row through the hook of pins.go.
func TestMemberSelfDestructPinnedStays(t *testing.T) {
	t.Run("the drop is exact", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		other := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
		deletionCleanup(t, other.Id)
		member := env.member()
		pinned := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "pin me" })
		kept := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "keep me queued" })
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2", got)
		}
		due := env.now.Add(memberSelfDestructRuleDelay * time.Second)
		// The same message ID in another chat, another kind in this chat, and an acting row.
		seedDeletionRow(t, other.Id, pinned, due, env.now, deletion.KindMemberSelfDestruct)
		otherKind := seedDeletionRow(t, env.chat.Id, pinned+500000, due, env.now, deletion.KindPinned)
		acting := seedDeletionRow(t, env.chat.Id, pinned+500001, due, env.now, deletion.KindMemberSelfDestruct)
		if err := db.DB.Model(&models.ScheduledDeletion{}).Where("id = ?", acting.ID).
			Update("state", models.ScheduledDeletionActing).Error; err != nil {
			t.Fatalf("mark the row acting: %v", err)
		}

		// A pin in the other chat naming this chat's message, and a pin in this chat naming the
		// acting row's message, change nothing of this chat's three rows; a pin in this chat naming
		// the pinned message drops that row only, and the other chat's row with the same message ID
		// stays.
		env.sendPinOf(other, &env.admin, kept)
		env.sendPinOf(env.chat, &env.admin, acting.MessageID)
		if before := env.selfDestructRows(env.chat.Id); len(before) != 3 {
			t.Fatalf("self-destruct rows after the two foreign pins = %+v, want 3 (the pinned, the kept and the acting)", before)
		}
		env.sendPinOf(env.chat, &env.admin, pinned)
		var left []int64
		for _, row := range env.queuedNoWait(env.chat.Id) {
			left = append(left, row.MessageID)
		}
		want := []int64{kept, otherKind.MessageID, acting.MessageID}
		slices.Sort(left)
		slices.Sort(want)
		if !slices.Equal(left, want) {
			t.Fatalf("rows left = %v, want %v (only the pinned message's pending row is gone)", left, want)
		}
		if rows := env.queuedNoWait(other.Id); len(rows) != 1 || rows[0].MessageID != pinned {
			t.Fatalf("rows of the other chat = %+v, want its row untouched", rows)
		}
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() after the pins = %d, want 0: a pin and an unpin queue nothing", got)
		}

		setNow(env.now.Add(601 * time.Second))
		env.pass()
		for _, call := range env.batchCalls(env.chat.Id) {
			if slices.Contains(call, pinned) {
				t.Fatalf("deleteMessages call %v holds the pinned message", call)
			}
		}
	})

	t.Run("the bot's own /pin drops the row", func(t *testing.T) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		setNow := selfDestructClock(t, env.now)
		member := env.member()
		pinned := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "pin me" })
		kept := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "stay queued" })
		if got := memberDeleteTestFlush(t); got != 2 {
			t.Fatalf("memberDeleteFlush() = %d, want 2", got)
		}
		ctx := newModuleMessageContext(env.bot, env.chat, env.admin, "/pin")
		ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{MessageId: pinned, Date: env.now.Unix(), Chat: env.chat, From: &member, Text: "pin me"}
		cmdCtx, err := helpers.BuildCommandContext(env.bot, ctx)
		if err != nil {
			t.Fatalf("BuildCommandContext failed: %v", err)
		}
		if err := pinsModule.pin(cmdCtx); err != ext.EndGroups {
			t.Fatalf("pin() error = %v, want EndGroups", err)
		}
		if calls := env.fake.callsFor("pinChatMessage"); len(calls) != 1 {
			t.Fatalf("pinChatMessage calls = %d, want 1", len(calls))
		}
		rows := env.selfDestructRows(env.chat.Id)
		if len(rows) != 1 || rows[0].MessageID != kept {
			t.Fatalf("rows after /pin = %+v, want only message %d", rows, kept)
		}
		setNow(env.now.Add(601 * time.Second))
		env.pass()
		calls := env.batchCalls(env.chat.Id)
		if len(calls) != 1 || !slices.Equal(calls[0], []int64{kept}) {
			t.Fatalf("deleteMessages calls = %v, want one call with message %d", calls, kept)
		}
	})
}

// TestMemberSelfDestructOffCancels: switching the rule Off drops the chat's pending self-destruct rows
// in the same write and nothing else; turning it on or changing the delay drops nothing, and rows
// queued under the old delay keep their time (D-12, D-26).
func TestMemberSelfDestructOffCancels(t *testing.T) {
	env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
	other := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Other"}
	deletionCleanup(t, other.Id)
	member := env.member()
	first := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "one" })
	second := env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "two" })
	if got := memberDeleteTestFlush(t); got != 2 {
		t.Fatalf("memberDeleteFlush() = %d, want 2", got)
	}
	due := env.now.Add(memberSelfDestructRuleDelay * time.Second)
	seedDeletionRow(t, other.Id, 990001, due, env.now, deletion.KindMemberSelfDestruct)
	otherKind := seedDeletionRow(t, env.chat.Id, 990002, due, env.now, deletion.KindPinned)
	acting := seedDeletionRow(t, env.chat.Id, 990003, due, env.now, deletion.KindMemberSelfDestruct)
	if err := db.DB.Model(&models.ScheduledDeletion{}).Where("id = ?", acting.ID).
		Update("state", models.ScheduledDeletionActing).Error; err != nil {
		t.Fatalf("mark the row acting: %v", err)
	}
	dueOf := func() map[int64]time.Time {
		out := map[int64]time.Time{}
		for _, row := range env.queuedNoWait(env.chat.Id) {
			out[row.MessageID] = row.DeleteAt
		}
		return out
	}
	want := map[int64]time.Time{first: due, second: due, 990002: due, 990003: due}
	if got := dueOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows before = %v, want %v", got, want)
	}

	// A delay change and a switch on again drop nothing.
	env.storeMemberRule(deletion.KindMemberSelfDestruct, 3600)
	env.storeMemberRule(deletion.KindMemberSelfDestruct, memberSelfDestructRuleDelay)
	if got := dueOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows after a delay change = %v, want %v: nothing is dropped and times are kept", got, want)
	}

	res := applyDeleteRule(context.Background(), env.chat.Id, deletion.KindMemberSelfDestruct, deletionTarget{Remove: true})
	if res.Outcome != deletionApplied {
		t.Fatalf("Off: outcome = %v, err = %v", res.Outcome, res.Err)
	}
	wantAfter := map[int64]time.Time{otherKind.MessageID: due, acting.MessageID: due}
	if got := dueOf(); !reflect.DeepEqual(got, wantAfter) {
		t.Fatalf("rows after Off = %v, want only the other kind's and the acting row %v", got, wantAfter)
	}
	if rows := env.queuedNoWait(other.Id); len(rows) != 1 {
		t.Fatalf("rows of the other chat after Off = %+v, want 1", rows)
	}
}

// memberWatchProbe is a handler that matches every new message and counts what it sees.
type memberWatchProbe struct {
	seen   atomic.Int32
	result error
}

func (p *memberWatchProbe) CheckUpdate(_ *gotgbot.Bot, ctx *ext.Context) bool {
	return ctx.Message != nil
}

func (p *memberWatchProbe) HandleUpdate(*gotgbot.Bot, *ext.Context) error {
	p.seen.Add(1)
	return p.result
}

func (p *memberWatchProbe) Name() string { return "member_watch_probe" }

// TestMemberSelfDestructNeverEndsUpdate: the watcher is a watcher (D-10, D-29). A sibling probe in
// its own group and a probe at a later group see every message it saw, queued, exempt or skipped,
// and also when a panic is forced in its rule read.
func TestMemberSelfDestructNeverEndsUpdate(t *testing.T) {
	setup := func(t *testing.T) (*memberDeletionEnv, *memberWatchProbe, *memberWatchProbe) {
		env := newSelfDestructEnv(t, memberSelfDestructRuleDelay)
		selfDestructClock(t, env.now)
		sibling := &memberWatchProbe{result: ext.ContinueGroups}
		later := &memberWatchProbe{result: ext.EndGroups}
		env.dispatcher.AddHandlerToGroup(sibling, memberDeleteGroup)
		env.dispatcher.AddHandlerToGroup(later, 5)
		return env, sibling, later
	}
	wantSeen := func(t *testing.T, want int32, probes map[string]*memberWatchProbe) {
		t.Helper()
		for name, probe := range probes {
			if got := probe.seen.Load(); got != want {
				t.Fatalf("%s probe saw %d messages, want %d: the watcher ended an update", name, got, want)
			}
		}
	}
	t.Run("queued, exempt and skipped messages all reach the probes", func(t *testing.T) {
		env, sibling, later := setup(t)
		member := env.member()
		env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "queued" })
		env.sendMember(&env.admin, func(m *gotgbot.Message) { m.Text = "exempt" })
		env.sendMember(&member, func(m *gotgbot.Message) { m.NewChatTitle = "service" })
		env.sendPinOf(env.chat, &env.admin, 1)
		wantSeen(t, 4, map[string]*memberWatchProbe{"sibling": sibling, "later": later})
		if got := memberDeleteTestFlush(t); got != 1 {
			t.Fatalf("memberDeleteFlush() = %d, want 1 queued", got)
		}
	})
	t.Run("a panic inside the rule read still lets the probes see the message", func(t *testing.T) {
		env, sibling, later := setup(t)
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			panic("scripted panic in the rule read")
		})
		member := env.member()
		env.sendMember(&member, func(m *gotgbot.Message) { m.Text = "boom" })
		wantSeen(t, 1, map[string]*memberWatchProbe{"sibling": sibling, "later": later})
		if got := memberDeleteTestFlush(t); got != 0 {
			t.Fatalf("memberDeleteFlush() = %d, want 0 after a panic", got)
		}
	})
}

// TestMemberContentMessageExcludesServiceFields: memberContentMessage is false for every service
// field TestServiceKindsCoverMessageFields lists and for the creation and migration fields, and true
// for member content.
func TestMemberContentMessageExcludesServiceFields(t *testing.T) {
	for name := range deletionServiceFields {
		if memberContentMessage(deletionMessageWith(t, name)) {
			t.Errorf("a message with only %s counts as member content", name)
		}
	}
	for _, name := range deletionNeverDeletedFields {
		if name == "Giveaway" {
			// The giveaway post is content (deletionNeverDeletedFields says so): it is the member's own post.
			if !memberContentMessage(deletionMessageWith(t, name)) {
				t.Errorf("a giveaway post does not count as member content")
			}
			continue
		}
		if memberContentMessage(deletionMessageWith(t, name)) {
			t.Errorf("a message with only %s counts as member content", name)
		}
	}
	for _, name := range []string{"Text", "Photo", "Sticker", "Poll", "ForwardOrigin", "Caption", "Document", "Voice"} {
		if !memberContentMessage(deletionMessageWith(t, name)) {
			t.Errorf("a message with only %s does not count as member content", name)
		}
	}
	if memberContentMessage(nil) {
		t.Error("memberContentMessage(nil) = true")
	}
}
