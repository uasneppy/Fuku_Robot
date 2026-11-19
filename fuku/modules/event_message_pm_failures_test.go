//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
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

// eventPMAllCalls returns every request the site recorded.
func eventPMAllCalls(s *botSite) []moduleBotCall {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	return append([]moduleBotCall(nil), s.client.calls...)
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
