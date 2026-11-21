//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// eventPMTelegramError is an error Telegram would answer a send with; retryAfter, when positive, is
// the 429's retry_after (in the fast pacer's 5 ms units).
func eventPMTelegramError(code int, description string, retryAfter int64) error {
	err := &gotgbot.TelegramError{Method: "sendMessage", Code: code, Description: description}
	if retryAfter > 0 {
		err.ResponseParams = &gotgbot.ResponseParameters{RetryAfter: retryAfter}
	}
	return err
}

// eventPMBanCommand runs /ban as a reply to the member's message with the given words after it.
func eventPMBanCommand(t *testing.T, s *botSite, words string) {
	t.Helper()
	if err := bansModule.ban(s.bot, s.commandCtx(strings.TrimSpace("/ban "+words), true, false)); err != ext.EndGroups {
		t.Fatalf("/ban error = %v, want EndGroups", err)
	}
}

// eventPMOfferTo offers a private copy of event straight to the engine, as a send site would.
func eventPMOfferTo(s *botSite, chatID int64, event string, userID int64, text string) {
	offerEventPM(eventPMOffer{
		Bot:     s.bot,
		Chat:    gotgbot.Chat{Id: chatID, Type: "supergroup", Title: s.chat.Title},
		Event:   event,
		UserID:  userID,
		Message: media.GreetingMessage{Text: text, MsgType: db.TEXT},
	})
}

// eventPMSentTo counts the sends, after the jobs, whose chat is one of users.
func eventPMSentTo(t *testing.T, s *botSite, users []int64) int {
	t.Helper()
	waitEventPM(t)
	want := map[string]bool{}
	for _, id := range users {
		want[strconv.FormatInt(id, 10)] = true
	}
	n := 0
	for _, call := range eventPMAllCalls(s) {
		if strings.HasPrefix(call.Method, "send") && want[fmt.Sprint(call.Params["chat_id"])] {
			n++
		}
	}
	return n
}

// eventPMAllCalls returns every request the site recorded.
func eventPMAllCalls(s *botSite) []moduleBotCall {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	return append([]moduleBotCall(nil), s.client.calls...)
}

// eventPMUsers returns n fresh user IDs.
func eventPMUsers(n int) []int64 {
	users := make([]int64, n)
	for i := range users {
		users[i] = uniqueLinkOwnerID()
	}
	return users
}

// eventPMWantLine fails unless one captured line holds every text of parts.
func eventPMWantLine(t *testing.T, hook *logrustest.Hook, parts ...string) {
	t.Helper()
	lines := mreqLogLines(hook)
	for _, line := range lines {
		all := true
		for _, part := range parts {
			all = all && strings.Contains(line, part)
		}
		if all {
			return
		}
	}
	t.Fatalf("no log line holds all of %q; lines:\n%s", parts, strings.Join(lines, "\n"))
}

// eventPMWaitBeforeRestore makes the test wait for the jobs before the variables swapped after it are
// put back, because t.Cleanup runs last in, first out.
func eventPMWaitBeforeRestore(t *testing.T) {
	t.Helper()
	t.Cleanup(pmCopyJobs.wait)
}

func TestEventPMFailures(t *testing.T) {
	withModerationGoldenLocales(t)

	t.Run("classification", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want pmCopyOutcome
		}{
			{"nil", nil, pmCopyDelivered},
			{"403", eventPMTelegramError(403, "Forbidden: bot was blocked by the user", 0), pmCopyBlocked},
			{"403 worded as a missing chat", eventPMTelegramError(403, "Bad Request: chat not found", 0), pmCopyBlocked},
			{"400", eventPMTelegramError(400, "Bad Request: wrong file identifier", 0), pmCopyRefused},
			{"404", eventPMTelegramError(404, "Not Found", 0), pmCopyRefused},
			{"no permission", media.ErrNoPermission, pmCopyRefused},
			{"429", eventPMTelegramError(429, "Too Many Requests", 3), pmCopyBusy},
			{"rate limited", fmt.Errorf("%w: next slot too far", ratelimit.ErrRateLimited), pmCopyBusy},
			{"500", eventPMTelegramError(500, "Internal Server Error", 0), pmCopyUnknown},
			{"deadline", context.DeadlineExceeded, pmCopyUnknown},
			{"plain", errors.New("connection reset"), pmCopyUnknown},
			{"wrapped 403", fmt.Errorf("failed to send: %w", eventPMTelegramError(403, "Forbidden", 0)), pmCopyBlocked},
		}
		for _, tc := range cases {
			if got := pmCopyClassify(tc.err); got != tc.want {
				t.Errorf("pmCopyClassify(%s) = %s, want %s", tc.name, got, tc.want)
			}
		}
	})

	for _, tc := range []struct{ name, description string }{
		{"never started", "Forbidden: bot can't initiate conversation with a user"},
		{"blocked", "Forbidden: bot was blocked by the user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newEventPMSite(t)
			hook := mreqLogCapture(t)
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "out: {reason}", true)
			eventPMRefuse(t, s, eventPMTelegramError(403, tc.description, 0))
			eventPMBanCommand(t, s, "spam <b>")
			eventPMWantBan(t, s)
			eventPMWantGroupGolden(t, s, goldenModerationBanReason)
			if _, private := eventPMCalls(t, s); len(private) != 1 {
				t.Fatalf("sends to the member's private chat = %d, want the one copy and no retry", len(private))
			}
			eventPMWantLine(t, hook, "not delivered: blocked (code 403)", strconv.FormatInt(s.chat.Id, 10))
			for _, entry := range hook.AllEntries() {
				if strings.Contains(entry.Message, "blocked") && entry.Level.String() != "debug" {
					t.Fatalf("a blocked copy was logged at %s, want debug: %q", entry.Level, entry.Message)
				}
			}
		})
	}

	t.Run("refused custom", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "custom {reason}", true)
		if err := eventmsg.SetMedia(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestPM, "pm-photo", db.PHOTO, 1); err != nil {
			t.Fatalf("SetMedia(pm) error = %v", err)
		}
		if err := eventmsg.SetButtons(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestPM,
			[]models.Button{{Name: "Site", Url: "https://example.com"}}, 1); err != nil {
			t.Fatalf("SetButtons(pm) error = %v", err)
		}
		eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: wrong file identifier", 0))
		eventPMBanCommand(t, s, "spam <b>")
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		_, private := eventPMCalls(t, s)
		if len(private) != 2 || private[0].Method != "sendPhoto" || private[1].Method != "sendMessage" {
			t.Fatalf("sends to the member's private chat = %v, want the refused photo then the default text", private)
		}
		params := eventPMParams(t, private[1])
		want := eventPMDefault(t, "eventmsg_pm_ban", "Bot Site Chat", "", "spam &lt;b&gt;", "")
		if params["text"] != want {
			t.Fatalf("fallback text = %q, want %q", params["text"], want)
		}
		if params["reply_markup"] != nil {
			t.Fatalf("fallback reply_markup = %v, want none", params["reply_markup"])
		}
		eventPMWantLine(t, hook, "was refused (code 400), the default was sent")
		eventPMWantLine(t, hook, "copy of user 42 in chat "+strconv.FormatInt(s.chat.Id, 10)+" was delivered")
	})

	t.Run("refused default", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: message is too long", 0))
		eventPMBanCommand(t, s, "spam <b>")
		if _, private := eventPMCalls(t, s); len(private) != 1 {
			t.Fatalf("sends to the member's private chat = %d, want the one refused default and no second", len(private))
		}
		eventPMWantLine(t, hook, "not delivered: refused (code 400)")
	})

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		// retry_after 4000 in the fast pacer's 5 ms unit is 20 s, over the pacer's 15 s MaxWait.
		{"busy", eventPMTelegramError(429, "Too Many Requests: retry after 4000", 4000), "not delivered: busy (code 0)"},
		{"unknown", errors.New("dial tcp: connection refused"), "not delivered: unknown (code 0)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newEventPMSite(t)
			hook := mreqLogCapture(t)
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "custom {reason}", true)
			eventPMRefuse(t, s, tc.err)
			eventPMBanCommand(t, s, "spam")
			eventPMWantBan(t, s)
			if _, private := eventPMCalls(t, s); len(private) != 1 {
				t.Fatalf("sends to the member's private chat = %d, want one and no fallback or retry", len(private))
			}
			eventPMWantLine(t, hook, tc.want)
		})
	}

	t.Run("impossible targets", func(t *testing.T) {
		s := newEventPMSite(t)
		for _, id := range []int64{0, -5, 777000, s.bot.Id} {
			eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, id, "never")
		}
		waitEventPM(t)
		for _, call := range eventPMAllCalls(s) {
			if strings.HasPrefix(call.Method, "send") {
				t.Fatalf("%s to %v was sent, want nothing for an impossible target", call.Method, call.Params["chat_id"])
			}
		}
	})
}

func TestEventPMLimits(t *testing.T) {
	withModerationGoldenLocales(t)

	t.Run("full pool", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		release := eventPMHold(t, s)
		for range pmCopyMaxInFlight {
			eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, eventPMMember, "held")
		}
		eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, eventPMMember, "shed")
		eventPMWantLine(t, hook, "was shed", "user 42", strconv.FormatInt(s.chat.Id, 10))
		release()
		if n := eventPMSentTo(t, s, []int64{eventPMMember}); n != pmCopyMaxInFlight {
			t.Fatalf("copies sent = %d, want the %d held jobs and not the shed one", n, pmCopyMaxInFlight)
		}
	})

	autoEnv := func(t *testing.T) (*botSite, func(time.Duration)) {
		t.Helper()
		s := newEventPMSite(t)
		withTestEventPM(t, "pmtest_auto", eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: true})
		var mu sync.Mutex
		now := time.Date(2026, 11, 19, 12, 0, 5, 0, time.UTC)
		mreqSwap(t, &pmCopyNow, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
		mreqSwap(t, &pmCopyAutoCapPerMinute, 3)
		eventPMWaitBeforeRestore(t)
		return s, func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	}

	t.Run("automatic cap", func(t *testing.T) {
		s, advance := autoEnv(t)
		first := eventPMUsers(5)
		for _, id := range first {
			eventPMOfferTo(s, s.chat.Id, "pmtest_auto", id, "auto")
		}
		if n := eventPMSentTo(t, s, first); n != 3 {
			t.Fatalf("automatic copies sent in one minute = %d, want the cap of 3", n)
		}
		other := uniqueModuleChatID()
		second := eventPMUsers(2)
		for _, id := range second {
			eventPMOfferTo(s, other, "pmtest_auto", id, "auto")
		}
		if n := eventPMSentTo(t, s, second); n != 2 {
			t.Fatalf("automatic copies of another chat = %d, want 2 counted apart", n)
		}
		advance(time.Minute)
		third := eventPMUsers(5)
		for _, id := range third {
			eventPMOfferTo(s, s.chat.Id, "pmtest_auto", id, "auto")
		}
		if n := eventPMSentTo(t, s, third); n != 3 {
			t.Fatalf("automatic copies in the next minute = %d, want 3 again", n)
		}
	})

	failClosed := func(t *testing.T, client func() *redis.Client) {
		t.Helper()
		s, _ := autoEnv(t)
		mreqSwap(t, &pmCopyRedisClient, client)
		eventPMWaitBeforeRestore(t)
		auto, manual := eventPMUsers(1), eventPMUsers(1)
		eventPMOfferTo(s, s.chat.Id, "pmtest_auto", auto[0], "auto")
		eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, manual[0], "manual")
		if n := eventPMSentTo(t, s, auto); n != 0 {
			t.Fatalf("automatic copies sent without a count = %d, want none (fail closed)", n)
		}
		if n := eventPMSentTo(t, s, manual); n != 1 {
			t.Fatalf("manual copies sent = %d, want 1: a manual copy reads no counter", n)
		}
	}

	t.Run("fail closed without Redis", func(t *testing.T) {
		failClosed(t, func() *redis.Client { return nil })
	})

	t.Run("fail closed on a Redis error", func(t *testing.T) {
		broken := miniredis.RunT(t)
		broken.SetError("ERR the counter is down")
		client := redis.NewClient(&redis.Options{Addr: broken.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		failClosed(t, func() *redis.Client { return client })
	})

	t.Run("manual events are not capped", func(t *testing.T) {
		s, _ := autoEnv(t)
		users := eventPMUsers(12)
		for _, id := range users {
			eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, id, "manual")
		}
		if n := eventPMSentTo(t, s, users); n != 12 {
			t.Fatalf("manual copies sent in one minute = %d, want all 12", n)
		}
	})
}

func TestEventPMPacerIndependent(t *testing.T) {
	withModerationGoldenLocales(t)

	block := func(t *testing.T, keys ...string) {
		t.Helper()
		client := cache.GetRedisClient()
		for _, key := range keys {
			if err := client.Set(context.Background(), key, "1", 30*time.Second).Err(); err != nil {
				t.Fatalf("set %s: %v", key, err)
			}
		}
	}

	t.Run("own block", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "custom {reason}", true)
		block(t, pmCopyPacerOptions.BlockKey)
		started := time.Now()
		eventPMBanCommand(t, s, "spam <b>")
		if took := time.Since(started); took > 5*time.Second {
			t.Fatalf("/ban took %s under a private copy block, want it at once", took)
		}
		eventPMWantBan(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMNone(t, s)
		eventPMWantLine(t, hook, "not delivered: busy")
	})

	t.Run("other blocks", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "custom {reason}", true)
		block(t, "fuku:staff:pace:block", "fuku:delete:pace:block", "fuku:memberreq:pace:block",
			"fuku:lockdown:pace:block", "fuku:fed:pace:block")
		started := time.Now()
		eventPMBanCommand(t, s, "spam")
		if params := eventPMOnly(t, s, "sendMessage"); params["text"] != "custom spam" {
			t.Fatalf("private copy text = %q, want the copy sent although every other pacer is blocked", params["text"])
		}
		if took := time.Since(started); took > 5*time.Second {
			t.Fatalf("the copy took %s, want it at once", took)
		}
	})
}

func TestEventPMLogsCarryIDsOnly(t *testing.T) {
	withModerationGoldenLocales(t)
	const (
		storedText  = "SentinelStoredText"
		caption     = "SentinelCaption"
		fileID      = "SentinelFileID"
		buttonName  = "SentinelButton"
		buttonLink  = "https://example.com/SentinelLink"
		reason      = "SentinelReason"
		memberName  = "SentinelMember"
		description = "SentinelDescription"
		transport   = "SentinelTransport"
	)
	forbidden := []string{storedText, caption, fileID, buttonName, "SentinelLink", reason, memberName, description, transport, "Bot Site Chat"}

	custom := func(t *testing.T, s *botSite) {
		t.Helper()
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "{first} "+storedText+" "+caption+" {reason}", true)
		if err := eventmsg.SetMedia(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestPM, fileID, db.PHOTO, 1); err != nil {
			t.Fatalf("SetMedia(pm) error = %v", err)
		}
		if err := eventmsg.SetButtons(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestPM,
			[]models.Button{{Name: buttonName, Url: buttonLink}}, 1); err != nil {
			t.Fatalf("SetButtons(pm) error = %v", err)
		}
	}
	site := func(t *testing.T) *botSite {
		t.Helper()
		s := newEventPMSite(t)
		script := eventPMScriptOf(t, s)
		script.mu.Lock()
		script.memberName = memberName
		script.mu.Unlock()
		return s
	}

	// open runs /start <arg> in the private chat of user, as the deep link router does.
	open := func(t *testing.T, s *botSite, user int64, arg string) {
		t.Helper()
		from := gotgbot.User{Id: user, FirstName: "Opener"}
		ctx := newModuleMessageContext(s.bot, gotgbot.Chat{Id: user, Type: "private", FirstName: "Opener"}, from, "/start "+arg)
		if err := HandleDeepLink(s.bot, ctx, &from, arg); err == nil {
			t.Fatalf("/start %s returned nil, want EndGroups", arg)
		}
	}
	// blockedWithDetails runs a blocked /ban of the custom copy with the Details option on and returns the record's token.
	blockedWithDetails := func(t *testing.T, s *botSite) string {
		t.Helper()
		custom(t, s)
		eventPMDetailsOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: "+description, 0))
		eventPMBanCommand(t, s, reason)
		waitEventPM(t)
		tokens := eventPMDetailsTokens(t)
		if len(tokens) != 1 {
			t.Fatalf("Details records = %d, want the one of the blocked copy", len(tokens))
		}
		return tokens[0]
	}

	cases := []struct {
		name    string
		run     func(t *testing.T, s *botSite)
		mustSee string
		failure bool
		// lines, when set, are the failure lines the case must log instead of the private copy's own line.
		lines [][]string
	}{
		{name: "delivered", mustSee: "was delivered", run: func(t *testing.T, s *botSite) {
			custom(t, s)
			eventPMBanCommand(t, s, reason)
		}},
		{name: "blocked", mustSee: "blocked (code 403)", failure: true, run: func(t *testing.T, s *botSite) {
			custom(t, s)
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: "+description, 0))
			eventPMBanCommand(t, s, reason)
		}},
		{name: "refused custom", mustSee: "the default was sent", failure: true, run: func(t *testing.T, s *botSite) {
			custom(t, s)
			eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: "+description, 0))
			eventPMBanCommand(t, s, reason)
		}},
		{name: "refused default", mustSee: "refused (code 400)", failure: true, run: func(t *testing.T, s *botSite) {
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
			eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: "+description, 0))
			eventPMBanCommand(t, s, reason)
		}},
		{name: "busy", mustSee: "busy", failure: true, run: func(t *testing.T, s *botSite) {
			custom(t, s)
			eventPMRefuse(t, s, eventPMTelegramError(429, "Too Many Requests: "+description, 4000))
			eventPMBanCommand(t, s, reason)
		}},
		{name: "unknown", mustSee: "unknown (code 0)", failure: true, run: func(t *testing.T, s *botSite) {
			custom(t, s)
			eventPMRefuse(t, s, errors.New("dial tcp: "+transport))
			eventPMBanCommand(t, s, reason)
		}},
		{name: "full pool", mustSee: "was shed", failure: true, run: func(t *testing.T, s *botSite) {
			release := eventPMHold(t, s)
			for range pmCopyMaxInFlight + 1 {
				eventPMOfferTo(s, s.chat.Id, eventmsg.EventBan, eventPMMember, storedText+" "+reason)
			}
			release()
		}},
		{name: "capped", mustSee: "automatic cap", failure: true, run: func(t *testing.T, s *botSite) {
			withTestEventPM(t, "pmtest_auto", eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: true})
			mreqSwap(t, &pmCopyAutoCapPerMinute, 0)
			eventPMWaitBeforeRestore(t)
			eventPMOfferTo(s, s.chat.Id, "pmtest_auto", eventPMMember, storedText+" "+reason)
		}},
		{name: "fail closed", mustSee: "automatic cap", failure: true, run: func(t *testing.T, s *botSite) {
			withTestEventPM(t, "pmtest_auto", eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: true})
			mreqSwap(t, &pmCopyRedisClient, func() *redis.Client { return nil })
			eventPMWaitBeforeRestore(t)
			eventPMOfferTo(s, s.chat.Id, "pmtest_auto", eventPMMember, storedText+" "+reason)
		}},
		// Phase 22 plan 22-08: the Details record is written and the edit that would add its button is refused.
		{name: "details record and a refused edit", mustSee: "could not be added", failure: true,
			lines: [][]string{{"the Details button of chat", "could not be added (code 400)"}},
			run: func(t *testing.T, s *botSite) {
				s.client.mu.Lock()
				s.client.errors["editMessageReplyMarkup"] = &gotgbot.TelegramError{
					Method: "editMessageReplyMarkup", Code: 400, Description: "Bad Request: " + description,
				}
				s.client.mu.Unlock()
				blockedWithDetails(t, s)
			}},
		// The deep link: a view by the person (twice, the second refused by Telegram), then every refusal: someone else, a
		// malformed payload, an unknown token and a Redis error. Only the refused view and the Redis error log a line.
		{name: "details deep link", mustSee: "could not be shown", failure: true,
			lines: [][]string{{"the Details copy of user 42 in chat", "could not be shown (code 400)"}, {"a Details record opened by user 42 could not be read"}},
			run: func(t *testing.T, s *botSite) {
				token := blockedWithDetails(t, s)
				open(t, s, eventPMMember, pmCopyDetailsPrefix+token)
				eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: "+description, 0))
				open(t, s, eventPMMember, pmCopyDetailsPrefix+token)
				open(t, s, eventPMMember+1, pmCopyDetailsPrefix+token)
				open(t, s, eventPMMember, pmCopyDetailsPrefix+token[:21]+"!")
				open(t, s, eventPMMember, pmCopyDetailsPrefix+"AAAAAAAAAAAAAAAAAAAAAA")
				broken := miniredis.RunT(t)
				broken.SetError("ERR " + description)
				client := redis.NewClient(&redis.Options{Addr: broken.Addr()})
				t.Cleanup(func() { _ = client.Close() })
				mreqSwap(t, &pmCopyRedisClient, func() *redis.Client { return client })
				open(t, s, eventPMMember, pmCopyDetailsPrefix+token)
			}},
		// The log line of the blocked copy is refused by the log channel.
		{name: "a failed log post", mustSee: "could not be posted", failure: true,
			lines: [][]string{{"the " + pmCopyLogTag + " log line of chat", "could not be posted (code 400)"}},
			run: func(t *testing.T, s *botSite) {
				channel := eventPMLogLink(t, s)
				custom(t, s)
				eventPMLogOn(t, s.chat.Id)
				eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: "+description, 0))
				s.bot.BotClient = eventPMRefuseLogClient{BotClient: s.bot.BotClient, channel: channel,
					err: &gotgbot.TelegramError{Method: "sendMessage", Code: 400, Description: "Bad Request: " + description}}
				eventPMBanCommand(t, s, reason)
				if posts := eventPMLogPosts(t, s, channel); len(posts) != 1 {
					t.Fatalf("#PM_COPY posts tried = %d, want the one refused post", len(posts))
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := site(t)
			s.chat.Title = "Bot Site Chat"
			hook := mreqLogCapture(t)
			tc.run(t, s)
			waitEventPM(t)
			// No line may carry a Details token either.
			mreqWantCleanLogs(t, hook, append(slices.Clone(forbidden), eventPMDetailsTokens(t)...), tc.mustSee)
			for _, parts := range tc.lines {
				eventPMWantLine(t, hook, parts...)
			}
			if tc.failure && tc.lines == nil {
				event := eventmsg.EventBan
				if tc.name == "capped" || tc.name == "fail closed" {
					event = "pmtest_auto"
				}
				eventPMWantLine(t, hook, tc.mustSee, "chat "+strconv.FormatInt(s.chat.Id, 10), "private "+event+" copy")
			}
		})
	}
}

// eventPMRefuseLogClient wraps a site's client and refuses every #PM_COPY post to channel with err, after the wrapped client
// recorded it; every other request, the ban's own log post included, is the wrapped client's.
type eventPMRefuseLogClient struct {
	gotgbot.BotClient
	channel int64
	err     error
}

func (c eventPMRefuseLogClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	response, err := c.BotClient.RequestWithContext(ctx, token, method, params, opts)
	if method == "sendMessage" && fmt.Sprint(params["chat_id"]) == strconv.FormatInt(c.channel, 10) &&
		strings.Contains(fmt.Sprint(params["text"]), pmCopyLogTag) {
		return nil, c.err
	}
	return response, err
}
