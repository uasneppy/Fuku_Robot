//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/approvals"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestMemberEditedDeletesLateEdit drives edited_message updates through the real dispatcher with
// member:edited stored as a grace of 60 seconds (plan 17-05, D-04, D-05, D-33): an edit 90 s after
// the message was sent is deleted at once, one 20 s after is kept, a group with no rule keeps both,
// and an administrator's and an approved member's late edits are kept.
func TestMemberEditedDeletesLateEdit(t *testing.T) {
	t.Run("a late edit is deleted at once", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		env.wantDeletedOnce(env.lateEdit(&member, 90, nil))
	})
	t.Run("an edit within the grace is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		env.wantKept(env.lateEdit(&member, 20, nil))
	})
	t.Run("a group with no rule keeps a late edit", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.primeAdminCache()
		member := env.member()
		env.wantKept(env.lateEdit(&member, 90, nil))
	})
	t.Run("an administrator's late edit is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		env.wantKept(env.lateEdit(&env.admin, 90, nil))
	})
	t.Run("an approved member's late edit is kept", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
		env.primeAdminCache()
		member := env.member()
		if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
			t.Fatalf("AddApprovedUser error = %v", err)
		}
		env.wantKept(env.lateEdit(&member, 90, nil))
	})
}

// TestMemberEditedGrace holds the arithmetic of the pure time rule at its edges (D-04, D-05, D-30).
func TestMemberEditedGrace(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	date := now.Add(-5 * time.Minute).Unix()
	tests := []struct {
		name     string
		date     int64
		editDate int64
		grace    int
		want     bool
	}{
		{"no edit date", date, 0, 60, false},
		{"edit date equal to the date", date, date, 60, false},
		{"edit date before the date", date, date - 5, 60, false},
		{"a gap equal to the grace is kept", date, date + 60, 60, false},
		{"a gap one second over the grace", date, date + 61, 60, true},
		{"no date", 0, date, 60, false},
		{"47 hours old exactly is judged", now.Add(-47 * time.Hour).Unix(), now.Add(-47*time.Hour).Unix() + 61, 60, true},
		{"47 hours and a minute old is skipped", now.Add(-47*time.Hour - time.Minute).Unix(),
			now.Add(-47*time.Hour-time.Minute).Unix() + 90, 60, false},
	}
	for _, presetGrace := range []int{30, 60, 300, 900, 3600} {
		tests = append(tests, struct {
			name     string
			date     int64
			editDate int64
			grace    int
			want     bool
		}{fmt.Sprintf("preset %d plus one second", presetGrace), date, date + int64(presetGrace) + 1, presetGrace, true})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := memberEditDue(tc.date, tc.editDate, tc.grace, now); got != tc.want {
				t.Fatalf("memberEditDue(%d, %d, %d) = %v, want %v", tc.date, tc.editDate, tc.grace, got, tc.want)
			}
		})
	}
}

// memberDeletionRecordJSON is a getChatMember answer for a user with the given status.
func memberDeletionRecordJSON(userID int64, status string) string {
	extra := ""
	if status == gotgbot.ChatMemberStatusRestricted {
		extra = `,"is_member":true`
	}
	return fmt.Sprintf(`{"status":%q,"user":{"id":%d,"is_bot":false,"first_name":"M%d"}%s}`, status, userID, userID, extra)
}

// memberDeletionTelegramError is the error the bot client answers with for a Telegram HTTP status.
func memberDeletionTelegramError(code int) error {
	return &gotgbot.TelegramError{Method: "getChatMember", Code: code, Description: "scripted"}
}

// TestMemberDeletionExemptTriState holds the shared exemption of D-33: an unknown is exempt.
func TestMemberDeletionExemptTriState(t *testing.T) {
	statusAnswer := func(status string) func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error) {
		return func(_ context.Context, _ *gotgbot.Bot, _, userID int64) (gotgbot.MergedChatMember, error) {
			return gotgbot.MergedChatMember{Status: status, User: gotgbot.User{Id: userID}}, nil
		}
	}
	failWith := func(err error) func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error) {
		return func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error) {
			return gotgbot.MergedChatMember{}, err
		}
	}
	notApproved := func(context.Context, int64, int64) (bool, error) { return false, nil }

	t.Run("the approvals read", func(t *testing.T) {
		tests := []struct {
			name     string
			approved func(context.Context, int64, int64) (bool, error)
			live     func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error)
			want     bool
		}{
			{"approved", func(context.Context, int64, int64) (bool, error) { return true, nil }, statusAnswer("member"), true},
			{"a read error", func(context.Context, int64, int64) (bool, error) { return false, errors.New("load failed") },
				statusAnswer("member"), true},
			{"not approved and an admin", notApproved, statusAnswer("administrator"), true},
			{"not approved and unknown", notApproved, failWith(memberDeletionTelegramError(500)), true},
			{"not approved and definitely not an admin", notApproved, statusAnswer("member"), false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				env := newMemberDeletionEnv(t)
				env.coldAdminCache()
				mreqSwap(t, &memberDeletionApprovedLookup, tc.approved)
				mreqSwap(t, &memberDeletionMemberLookup, tc.live)
				member := env.member()
				if got := memberDeletionExempt(env.bot, env.chat.Id, member.Id); got != tc.want {
					t.Fatalf("memberDeletionExempt() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("the admin status", func(t *testing.T) {
		tests := []struct {
			name string
			live func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error)
			want memberDeletionAdmin
		}{
			{"a live creator", statusAnswer("creator"), memberDeletionIsAdmin},
			{"a live administrator", statusAnswer("administrator"), memberDeletionIsAdmin},
			{"a live member", statusAnswer("member"), memberDeletionNotAdmin},
			{"a live restricted member", statusAnswer("restricted"), memberDeletionNotAdmin},
			{"a live left record", statusAnswer("left"), memberDeletionNotAdmin},
			{"a live kicked record", statusAnswer("kicked"), memberDeletionNotAdmin},
			{"an unrecognised status", statusAnswer("weird"), memberDeletionAdminUnknown},
			{"a 429", failWith(memberDeletionTelegramError(429)), memberDeletionAdminUnknown},
			{"a 500", failWith(memberDeletionTelegramError(500)), memberDeletionAdminUnknown},
			{"a transport error", failWith(errors.New("connection reset")), memberDeletionAdminUnknown},
			{"an empty answer", failWith(errors.New("empty chat member answer")), memberDeletionAdminUnknown},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				env := newMemberDeletionEnv(t)
				env.coldAdminCache()
				mreqSwap(t, &memberDeletionMemberLookup, tc.live)
				member := env.member()
				if got := memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id); got != tc.want {
					t.Fatalf("memberDeletionAdminStatus() = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("the cached list answers without a live read", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.primeAdminCache()
		lookups := 0
		mreqSwap(t, &memberDeletionMemberLookup,
			func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error) {
				lookups++
				return gotgbot.MergedChatMember{}, errors.New("must not be called")
			})
		member := env.member()
		if got := memberDeletionAdminStatus(env.bot, env.chat.Id, env.admin.Id); got != memberDeletionIsAdmin {
			t.Fatalf("cached administrator = %v, want admin", got)
		}
		if got := memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id); got != memberDeletionNotAdmin {
			t.Fatalf("cached member = %v, want not an admin", got)
		}
		if lookups != 0 {
			t.Fatalf("live reads = %d, want 0", lookups)
		}
	})

	t.Run("a timeout is unknown", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.coldAdminCache()
		mreqSwap(t, &memberDeletionAdminTimeout, 20*time.Millisecond)
		mreqSwap(t, &memberDeletionMemberLookup,
			func(ctx context.Context, _ *gotgbot.Bot, _, _ int64) (gotgbot.MergedChatMember, error) {
				<-ctx.Done()
				return gotgbot.MergedChatMember{}, ctx.Err()
			})
		member := env.member()
		if got := memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id); got != memberDeletionAdminUnknown {
			t.Fatalf("memberDeletionAdminStatus() = %v, want unknown", got)
		}
	})

	t.Run("a full semaphore is unknown at once", func(t *testing.T) {
		env := newMemberDeletionEnv(t)
		env.coldAdminCache()
		full := make(chan struct{}, 1)
		full <- struct{}{}
		mreqSwap(t, &memberDeletionAdminSlots, full)
		lookups := 0
		mreqSwap(t, &memberDeletionMemberLookup,
			func(context.Context, *gotgbot.Bot, int64, int64) (gotgbot.MergedChatMember, error) {
				lookups++
				return gotgbot.MergedChatMember{Status: "member"}, nil
			})
		member := env.member()
		if got := memberDeletionAdminStatus(env.bot, env.chat.Id, member.Id); got != memberDeletionAdminUnknown {
			t.Fatalf("memberDeletionAdminStatus() = %v, want unknown", got)
		}
		if lookups != 0 {
			t.Fatalf("live reads = %d, want 0 (a full semaphore never waits or reads)", lookups)
		}
	})
}

// botMemberLookups counts the getChatMember calls that name the bot itself.
func (e *memberDeletionEnv) botMemberLookups() int {
	count := 0
	for _, call := range e.fake.callsFor("getChatMember") {
		if staffParamInt(call.Params, "user_id") == staffTestBotID {
			count++
		}
	}
	return count
}

// failMemberLookup makes the bot's getChatMember of userID answer with err.
func (e *memberDeletionEnv) failMemberLookup(userID int64, err error) {
	e.script.set(func(_ context.Context, method string, params map[string]any) error {
		if method == "getChatMember" && staffParamInt(params, "user_id") == userID {
			return err
		}
		return nil
	})
}

// TestMemberEditedKeeps runs a late edit (90 s after the message, the rule at 60) of every exempt
// sender and every failing exemption read through the real dispatcher: no deleteMessage, no
// getChatMember of the bot and no queue row; the last row is the control that is deleted.
func TestMemberEditedKeeps(t *testing.T) {
	type fixture struct {
		sender gotgbot.User
		chat   *gotgbot.Chat
		apply  func(*gotgbot.Message)
	}
	tests := []struct {
		name       string
		wantDelete bool
		setup      func(t *testing.T, env *memberDeletionEnv) fixture
	}{
		{name: "an administrator in the cached list", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
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
			env.primeAdminCache()
			member := env.member()
			if err := approvals.AddApprovedUser(env.chat.Id, member.Id, env.admin.Id, "trusted"); err != nil {
				t.Fatalf("AddApprovedUser error = %v", err)
			}
			return fixture{sender: member}
		}},
		{name: "a member whose approvals read fails", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			mreqSwap(t, &memberDeletionApprovedLookup,
				func(context.Context, int64, int64) (bool, error) { return false, errors.New("approvals unreadable") })
			return fixture{sender: env.member()}
		}},
		{name: "a channel post as the sender", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			channel := gotgbot.Chat{Id: -1001234567890, Type: "channel", Title: "Channel"}
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.SenderChat = &channel }}
		}},
		{name: "an anonymous admin", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			chat := env.chat
			return fixture{
				sender: gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group"},
				apply:  func(m *gotgbot.Message) { m.SenderChat = &chat },
			}
		}},
		{name: "a bot user", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			bot := env.member()
			bot.IsBot = true
			return fixture{sender: bot}
		}},
		{name: "a message sent through an inline bot", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			via := gotgbot.User{Id: uniqueLinkOwnerID(), IsBot: true, FirstName: "Inline"}
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.ViaBot = &via }}
		}},
		{name: "an automatic forward", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			return fixture{sender: env.member(), apply: func(m *gotgbot.Message) { m.IsAutomaticForward = true }}
		}},
		{name: "a Telegram service ID", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			return fixture{sender: gotgbot.User{Id: 777000, FirstName: "Telegram"}}
		}},
		{name: "a member with a pending captcha", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			member := env.member()
			if _, err := captcha.CreateCaptchaAttemptPreMessage(member.Id, env.chat.Id, "answer", 5); err != nil {
				t.Fatalf("CreateCaptchaAttemptPreMessage error = %v", err)
			}
			return fixture{sender: member}
		}},
		{name: "a member whose captcha read fails", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			mreqSwap(t, &memberReqCaptchaPendingLookup,
				func(context.Context, int64) (bool, error) { return false, errors.New("captcha unreadable") })
			return fixture{sender: env.member()}
		}},
		{name: "a private chat", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			member := env.member()
			private := gotgbot.Chat{Id: member.Id, Type: "private"}
			return fixture{sender: member, chat: &private}
		}},
		{name: "a stored grace of 45 is Off", setup: func(t *testing.T, env *memberDeletionEnv) fixture {
			env.primeAdminCache()
			if err := db.DB.Where("chat_id = ? AND kind = ?", env.chat.Id, deletion.KindMemberEdited).
				Delete(&models.DeleteRule{}).Error; err != nil {
				t.Fatalf("clear the rule: %v", err)
			}
			row := models.DeleteRule{ChatID: env.chat.Id, Kind: deletion.KindMemberEdited, DelaySecs: 45, UpdatedAt: time.Now().UTC()}
			if err := db.DB.Create(&row).Error; err != nil {
				t.Fatalf("write the rule: %v", err)
			}
			dbcache.DeleteCache(dbcache.CacheKey("delete_rules", env.chat.Id))
			return fixture{sender: env.member()}
		}},
		{name: "control: a plain member behind a cold cache is deleted", wantDelete: true,
			setup: func(t *testing.T, env *memberDeletionEnv) fixture {
				member := env.member()
				env.fake.setMemberRaw(env.chat.Id, member.Id, memberDeletionRecordJSON(member.Id, "member"))
				env.coldAdminCache()
				return fixture{sender: member}
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newMemberDeletionEnv(t)
			env.storeMemberRule(deletion.KindMemberEdited, memberEditedRuleDelay)
			fx := tc.setup(t, env)
			botLookups := env.botMemberLookups()
			chat := env.chat
			if fx.chat != nil {
				chat = *fx.chat
			}
			date := deletionNow().Unix() - 300
			id := env.sendEditIn(chat, &fx.sender, date, date+90, fx.apply)
			if tc.wantDelete {
				env.wantDeletedOnce(id)
				return
			}
			env.wantKept(id)
			if calls := env.fake.callsFor("deleteMessage"); len(calls) != 0 {
				t.Fatalf("deleteMessage calls = %d, want none", len(calls))
			}
			if got := env.botMemberLookups(); got != botLookups {
				t.Fatalf("getChatMember of the bot = %d, want %d: no delete was refused", got, botLookups)
			}
		})
	}
}
