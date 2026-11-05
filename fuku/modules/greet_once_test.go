//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// greetOnceSetup stores a welcome text for a fresh supergroup and returns the chat and a bot over a fresh fake.
func greetOnceSetup(t *testing.T) (gotgbot.Chat, *moduleBotClient, *gotgbot.Bot) {
	t.Helper()
	client := newModuleBotClient()
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Greeting Chat"}
	if err := greetings.SetWelcomeText(chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText setup error = %v", err)
	}
	greetOnceCleanup(t, chat.Id)
	return chat, client, newModuleTestBot(client)
}

// greetOnceMemberIDs are the user IDs the tests of this file join. The captcha path stores a users row for the
// member, named by the fake's getChatMember answer ("Member"), and caches it, so a later test that expects the
// same ID to be unknown (the staff action card of /ban 4242) would otherwise read that stored name.
var greetOnceMemberIDs = []int64{4242, 4343, 777000}

// greetOnceCleanup deletes the greeted_members rows of the chat, and the users rows and cached copies of the
// members the tests join, when the test ends.
func greetOnceCleanup(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetedMember{}).Error; err != nil {
			t.Errorf("cleanup greeted_members: %v", err)
		}
		if err := db.DB.Where("user_id IN ?", greetOnceMemberIDs).Delete(&models.User{}).Error; err != nil {
			t.Errorf("cleanup users: %v", err)
		}
		for _, id := range greetOnceMemberIDs {
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	})
}

// greetOnceSwitchOn turns Greet once on for the chat through the apply function the Welcome page will call.
func greetOnceSwitchOn(t *testing.T, chatID int64) {
	t.Helper()
	if res := applyWelcomeOnce(t.Context(), chatID, true); res.Outcome != greetingApplied {
		t.Fatalf("applyWelcomeOnce(true) outcome = %v, err = %v", res.Outcome, res.Err)
	}
}

// greetOnceJoin drives one join of the member through the chat_member handler, with the in-process dedupe cleared
// first so a second call is a real second join and not the same one delivered twice.
func greetOnceJoin(t *testing.T, bot *gotgbot.Bot, chat gotgbot.Chat, member gotgbot.User) {
	t.Helper()
	if err := greetOnceJoinErr(bot, chat, member); err != ext.EndGroups {
		t.Fatalf("newMember error = %v, want EndGroups", err)
	}
}

// greetOnceJoinErr is greetOnceJoin for a join whose welcome send may fail, so it returns what the handler returned.
func greetOnceJoinErr(bot *gotgbot.Bot, chat gotgbot.Chat, member gotgbot.User) error {
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
	clearRecentJoinProcessing(chat.Id, member.Id)
	return greetingsModule.newMember(bot, ctx)
}

// greetOnceLeave drives the member's leave through the chat_member handler.
func greetOnceLeave(t *testing.T, bot *gotgbot.Bot, chat gotgbot.Chat, member gotgbot.User) {
	t.Helper()
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberMember{User: member}, gotgbot.ChatMemberLeft{User: member})
	if err := greetingsModule.leftMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("leftMember error = %v, want EndGroups", err)
	}
}

// greetedRows counts the greeted_members rows of one chat.
func greetedRows(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.GreetedMember{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count greeted_members: %v", err)
	}
	return n
}

// greetOnceWelcomes counts the welcome messages the fake received (the goodbye a leave sends is not one).
func greetOnceWelcomes(client *moduleBotClient) int {
	n := 0
	for _, call := range client.callsFor("sendMessage") {
		if text, _ := call.Params["text"].(string); strings.Contains(text, "Welcome") {
			n++
		}
	}
	return n
}

func TestGreetOnceFirstJoinOnly(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	memberA := gotgbot.User{Id: 4242, FirstName: "Alice"}
	memberB := gotgbot.User{Id: 4343, FirstName: "Bob"}
	if res := applyWelcomeOnce(t.Context(), chat.Id, true); res.Outcome != greetingApplied {
		t.Fatalf("applyWelcomeOnce(true) outcome = %v, err = %v", res.Outcome, res.Err)
	}

	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes after A's first join = %d, want 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows after A's first join = %d, want 1", got)
	}

	greetOnceLeave(t, bot, chat, memberA)
	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes after A rejoined = %d, want still 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows after A rejoined = %d, want still 1", got)
	}

	greetOnceJoin(t, bot, chat, memberB)
	if got := greetOnceWelcomes(client); got != 2 {
		t.Fatalf("welcomes after B's first join = %d, want 2", got)
	}

	if res := applyWelcomeOnce(t.Context(), chat.Id, false); res.Outcome != greetingApplied {
		t.Fatalf("applyWelcomeOnce(false) outcome = %v, err = %v", res.Outcome, res.Err)
	}
	greetOnceJoin(t, bot, chat, memberA)
	if got := greetOnceWelcomes(client); got != 3 {
		t.Fatalf("welcomes after A joined with Greet once off = %d, want 3", got)
	}
}

func TestGreetOnceWelcomeOffNoClaim(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	member := gotgbot.User{Id: 4242, FirstName: "Alice"}
	greetOnceSwitchOn(t, chat.Id)
	if err := greetings.SetWelcomeToggle(chat.Id, false); err != nil {
		t.Fatalf("SetWelcomeToggle setup error = %v", err)
	}

	greetOnceJoin(t, bot, chat, member)

	if got := greetOnceWelcomes(client); got != 0 {
		t.Fatalf("welcomes with the Welcome switch off = %d, want 0", got)
	}
	if got := greetedRows(t, chat.Id); got != 0 {
		t.Fatalf("greeted rows with the Welcome switch off = %d, want 0: a welcome that was never sent records nobody", got)
	}
}

func TestGreetOnceOffNoRecord(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	member := gotgbot.User{Id: 4242, FirstName: "Alice"}

	greetOnceJoin(t, bot, chat, member)
	greetOnceJoin(t, bot, chat, member)

	if got := greetOnceWelcomes(client); got != 2 {
		t.Fatalf("welcomes with Greet once off = %d, want 2", got)
	}
	if got := greetedRows(t, chat.Id); got != 0 {
		t.Fatalf("greeted rows with Greet once off = %d, want 0", got)
	}
}

func TestGreetOnceCaptchaPathAndFailure(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	passer := gotgbot.User{Id: 4242, FirstName: "Alice"}
	failer := gotgbot.User{Id: 4343, FirstName: "Bob"}
	greetOnceSwitchOn(t, chat.Id)
	if err := captcha.SetCaptchaEnabled(chat.Id, true); err != nil {
		t.Fatalf("SetCaptchaEnabled setup error = %v", err)
	}
	if err := captcha.SetCaptchaFailureAction(chat.Id, "ban"); err != nil {
		t.Fatalf("SetCaptchaFailureAction setup error = %v", err)
	}
	if err := captcha.SetCaptchaMaxAttempts(chat.Id, 1); err != nil {
		t.Fatalf("SetCaptchaMaxAttempts setup error = %v", err)
	}

	greetOnceJoin(t, bot, chat, passer)
	greetOnceJoin(t, bot, chat, failer)
	if got := greetOnceWelcomes(client); got != 0 {
		t.Fatalf("welcomes before any captcha is solved = %d, want 0", got)
	}
	if got := greetedRows(t, chat.Id); got != 0 {
		t.Fatalf("greeted rows before any captcha is solved = %d, want 0: the challenge is not a welcome", got)
	}

	press := func(member gotgbot.User, answer string) {
		t.Helper()
		attempt, err := captcha.GetCaptchaAttempt(member.Id, chat.Id)
		if err != nil || attempt == nil {
			t.Fatalf("GetCaptchaAttempt(%d) = %v, %v, want an attempt", member.Id, attempt, err)
		}
		if answer == "" {
			answer = attempt.Answer
		}
		data := encodeCallbackData("captcha_verify",
			map[string]string{"a": fmt.Sprint(attempt.ID), "u": fmt.Sprint(member.Id), "s": answer})
		ctx := newModuleCallbackContext(bot, chat, member, data)
		if err := captchaModule.captchaVerifyCallback(bot, ctx); err != nil {
			t.Fatalf("captchaVerifyCallback(%d) error = %v", member.Id, err)
		}
	}

	press(failer, "not-the-answer")
	if got := len(client.callsFor("banChatMember")); got != 1 {
		t.Fatalf("banChatMember calls after the final wrong answer = %d, want the failure action once", got)
	}
	if got := greetOnceWelcomes(client); got != 0 {
		t.Fatalf("welcomes after a captcha failure = %d, want 0", got)
	}
	if got := greetedRows(t, chat.Id); got != 0 {
		t.Fatalf("greeted rows after a captcha failure = %d, want 0", got)
	}

	press(passer, "")
	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes after the captcha pass = %d, want 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows after the captcha pass = %d, want 1", got)
	}
}

func TestGreetOnceLockdownBanNoRecord(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	env.enableJoinWelcome(false)
	if err := greetings.SetWelcomeText(env.chat.Id, "Welcome {first}", "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText setup error = %v", err)
	}
	greetOnceCleanup(t, env.chat.Id)
	greetOnceSwitchOn(t, env.chat.Id)

	control := env.newJoiner("Control")
	env.join(control, control, "https://t.me/+ctl")
	if got := greetedRows(t, env.chat.Id); got != 1 {
		t.Fatalf("greeted rows after the control join = %d, want 1: before the lockdown a join is welcomed and recorded", got)
	}

	env.lockAsAdmin("raid")
	messagesBefore, _, _ := env.chatSends()
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")

	if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
		t.Fatalf("raider state = %q, want pending: the guard recorded them for a ban", row.State)
	}
	if messagesAfter, _, _ := env.chatSends(); messagesAfter != messagesBefore {
		t.Fatalf("messages sent to a banned joiner's chat = %d, want %d: no welcome", messagesAfter, messagesBefore)
	}
	if got := greetedRows(t, env.chat.Id); got != 1 {
		t.Fatalf("greeted rows after the raider's join = %d, want still 1: a lockdown-banned joiner is never recorded", got)
	}
}

func TestGreetOnceWithoutRedis(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	member := gotgbot.User{Id: 4242, FirstName: "Alice"}
	greetOnceSwitchOn(t, chat.Id)

	marshal, manager, redisClient := cache.GetCacheState()
	cache.SetCacheState(nil, nil, nil)
	t.Cleanup(func() { cache.SetCacheState(marshal, manager, redisClient) })

	greetOnceJoin(t, bot, chat, member)
	greetOnceJoin(t, bot, chat, member)

	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes without Redis = %d, want 1", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows without Redis = %d, want 1", got)
	}
}

func TestGreetOnceConcurrentOneWelcome(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	member := gotgbot.User{Id: 4242, FirstName: "Alice"}
	greetOnceSwitchOn(t, chat.Id)

	const joins = 16
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	start := make(chan struct{})
	errs := make(chan error, joins)
	var wg sync.WaitGroup
	for range joins {
		ctx := newChatMemberContext(bot, chat, admin,
			gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- SendWelcomeMessage(bot, ctx, member.Id, member.FirstName)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("SendWelcomeMessage error = %v, want nil", err)
		}
	}

	if got := greetOnceWelcomes(client); got != 1 {
		t.Fatalf("welcomes for %d concurrent joins of one member = %d, want exactly 1", joins, got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows = %d, want 1", got)
	}
}

func TestGreetOnceDefiniteRefusalReleases(t *testing.T) {
	withMiniredis(t)
	cases := []struct {
		name string
		err  error
		// restricted marks the chat restricted before the join, so the send fails with media.ErrNoPermission and no request.
		restricted bool
	}{
		{name: "400 wrong file identifier", err: &gotgbot.TelegramError{
			Method: "sendMessage", Code: 400, Description: "Bad Request: wrong file identifier"}},
		{name: "403 bot kicked", err: &gotgbot.TelegramError{
			Method: "sendMessage", Code: 403, Description: "Forbidden: bot was kicked from the supergroup chat"}},
		{name: "429 too many requests", err: &gotgbot.TelegramError{
			Method: "sendMessage", Code: 429, Description: "Too Many Requests: retry after 5"}},
		{name: "chat already restricted", restricted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, client, bot := greetOnceSetup(t)
			member := gotgbot.User{Id: 4242, FirstName: "Alice"}
			greetOnceSwitchOn(t, chat.Id)
			if tc.restricted {
				cache.MarkChatRestricted(chat.Id)
				t.Cleanup(func() { cache.MarkChatNotRestricted(chat.Id) })
			} else {
				client.errors["sendMessage"] = tc.err
			}

			_ = greetOnceJoinErr(bot, chat, member)
			if got := greetedRows(t, chat.Id); got != 0 {
				t.Fatalf("greeted rows after a refused welcome = %d, want 0: a definite refusal gives the claim back", got)
			}
			refused := greetOnceWelcomes(client)

			delete(client.errors, "sendMessage")
			cache.MarkChatNotRestricted(chat.Id)
			greetOnceJoin(t, bot, chat, member)
			if got := greetOnceWelcomes(client); got != refused+1 {
				t.Fatalf("welcome requests after the retry = %d, want %d: the next join is welcomed", got, refused+1)
			}
			if got := greetedRows(t, chat.Id); got != 1 {
				t.Fatalf("greeted rows after the welcome went through = %d, want 1", got)
			}
		})
	}
}

func TestGreetOnceUnknownOutcomeKeeps(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "502 bad gateway", err: &gotgbot.TelegramError{Method: "sendMessage", Code: 502, Description: "Bad Gateway"}},
		{name: "transport error", err: errors.New("read tcp: connection reset by peer")},
		{name: "deadline exceeded", err: context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, client, bot := greetOnceSetup(t)
			member := gotgbot.User{Id: 4242, FirstName: "Alice"}
			greetOnceSwitchOn(t, chat.Id)
			client.errors["sendMessage"] = tc.err

			_ = greetOnceJoinErr(bot, chat, member)
			if got := greetedRows(t, chat.Id); got != 1 {
				t.Fatalf("greeted rows after an unknown outcome = %d, want 1: the welcome may have been delivered, so the claim stays", got)
			}
			attempted := greetOnceWelcomes(client)

			delete(client.errors, "sendMessage")
			greetOnceJoin(t, bot, chat, member)
			if got := greetOnceWelcomes(client); got != attempted {
				t.Fatalf("welcome requests after the next join = %d, want %d: a double welcome is worse than a missing one", got, attempted)
			}
			if got := greetedRows(t, chat.Id); got != 1 {
				t.Fatalf("greeted rows after the next join = %d, want 1", got)
			}
		})
	}
}

func TestGreetOnceClaimErrorFailsClosed(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	member := gotgbot.User{Id: 4242, FirstName: "Alice"}
	greetOnceSwitchOn(t, chat.Id)

	previous := greetOnceStore
	greetOnceStore.Claim = func(int64, int64) (bool, error) { return false, errors.New("database is down") }
	t.Cleanup(func() { greetOnceStore = previous })

	greetOnceJoin(t, bot, chat, member)

	if got := greetOnceWelcomes(client); got != 0 {
		t.Fatalf("welcomes with a failing claim = %d, want 0: the join fails closed", got)
	}
	if got := greetedRows(t, chat.Id); got != 0 {
		t.Fatalf("greeted rows with a failing claim = %d, want 0", got)
	}
}

// TestGreetOnceReadErrorFailsClosed pins WR-01 of the Phase 12 review: when the greetings read fails with a
// database error, the welcome path cannot know whether Greet once is on, so it sends nothing and claims nothing. The
// read used to fall back to Welcome on and Greet once off, which welcomed an already greeted member a second time.
func TestGreetOnceReadErrorFailsClosed(t *testing.T) {
	chat, client, bot := greetOnceSetup(t)
	greeted := gotgbot.User{Id: 4242, FirstName: "Alice"}
	fresh := gotgbot.User{Id: 4343, FirstName: "Bob"}
	greetOnceSwitchOn(t, chat.Id)
	if won, err := greetings.ClaimGreeting(chat.Id, greeted.Id); err != nil || !won {
		t.Fatalf("ClaimGreeting setup = %v, %v, want a won claim", won, err)
	}

	settingsStaffTestFailRead(t, "greetings")
	greetOnceJoin(t, bot, chat, greeted)
	greetOnceJoin(t, bot, chat, fresh)

	if got := len(client.callsFor("sendMessage")); got != 0 {
		t.Fatalf("sendMessage calls with a failing greetings read = %d, want 0: a double welcome is worse than a missing one", got)
	}
	if got := greetedRows(t, chat.Id); got != 1 {
		t.Fatalf("greeted rows with a failing greetings read = %d, want 1: nothing is claimed and the old row stays", got)
	}
}
