//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// eventPMDetailsOn switches the chat's Details option on through the repository.
func eventPMDetailsOn(t *testing.T, chatID int64) {
	t.Helper()
	if err := eventmsg.SetPMDetails(t.Context(), chatID, true, 1); err != nil {
		t.Fatalf("SetPMDetails(%d) error = %v", chatID, err)
	}
}

// eventPMDetailsEdits returns the site's editMessageReplyMarkup requests, after the jobs, decoded through JSON.
func eventPMDetailsEdits(t *testing.T, s *botSite) []map[string]any {
	t.Helper()
	waitEventPM(t)
	var edits []map[string]any
	for _, call := range eventPMAllCalls(s) {
		if call.Method != "editMessageReplyMarkup" {
			continue
		}
		edit := rulesCallParams(t, call)
		edit["chat_id"], edit["message_id"] = call.Params["chat_id"], call.Params["message_id"]
		edits = append(edits, edit)
	}
	return edits
}

// eventPMDetailsNothing fails unless the site made no Details edit and Redis holds no Details record.
func eventPMDetailsNothing(t *testing.T, s *botSite) {
	t.Helper()
	if edits := eventPMDetailsEdits(t, s); len(edits) != 0 {
		t.Fatalf("Details edits = %v, want none", edits)
	}
	if tokens := eventPMDetailsTokens(t); len(tokens) != 0 {
		t.Fatalf("Details records = %d, want none", len(tokens))
	}
}

// eventPMKeyboardRows is the inline keyboard rows of decoded request parameters (nil when there is none).
func eventPMKeyboardRows(params map[string]any) []any {
	markup, _ := params["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	return rows
}

// eventPMJSON is v as JSON, for comparing decoded keyboards.
func eventPMJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode %v: %v", v, err)
	}
	return string(raw)
}

// eventPMDetailsWantEdit checks the one Details edit: it edits the group message the group step posted, keeps the
// rows that message was sent with in their order and adds one row with one URL button labelled in the group's
// language. It returns the token of the URL.
func eventPMDetailsWantEdit(t *testing.T, s *botSite) string {
	t.Helper()
	edits := eventPMDetailsEdits(t, s)
	if len(edits) != 1 {
		t.Fatalf("Details edits = %d, want one", len(edits))
	}
	group, _ := eventPMCalls(t, s)
	if len(group) != 1 {
		t.Fatalf("sends to the group = %d, want the one group message", len(group))
	}
	edit, sent := edits[0], eventPMParams(t, group[0])
	if fmt.Sprint(edit["chat_id"]) != strconv.FormatInt(s.chat.Id, 10) || fmt.Sprint(edit["message_id"]) != "9001" {
		t.Fatalf("the Details edit targets chat %v message %v, want chat %d message 9001", edit["chat_id"], edit["message_id"], s.chat.Id)
	}
	if _, ok := edit["reply_parameters"]; ok {
		t.Fatal("the Details edit carries reply parameters")
	}
	rows, sentRows := eventPMKeyboardRows(edit), eventPMKeyboardRows(sent)
	if len(rows) != len(sentRows)+1 || eventPMJSON(t, rows[:len(sentRows)]) != eventPMJSON(t, sentRows) {
		t.Fatalf("the Details edit's rows = %s, want the group message's rows %s and one more", eventPMJSON(t, rows), eventPMJSON(t, sentRows))
	}
	label, _ := i18n.MustNewTranslator("uk").GetString("eventmsg_btn_details")
	last, _ := rows[len(rows)-1].([]any)
	if button, _ := last[0].(map[string]any); button["text"] != label || button["callback_data"] != nil {
		t.Fatalf("the Details button = %v, want the URL button %q", button, label)
	}
	return eventPMDetailsTokenOf(t, rows)
}

func TestEventPMDetails(t *testing.T) {
	withModerationGoldenLocales(t)

	// banned runs /ban with the ban's private copy on (the default private text unless text is set) and the
	// Details option on, after setup prepared the site.
	banned := func(t *testing.T, s *botSite) {
		t.Helper()
		eventPMBanCommand(t, s, "spam <b>")
		eventPMWantBan(t, s)
	}

	t.Run("blocked adds the button", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMDetailsOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: bot was blocked by the user", 0))
		banned(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		token := eventPMDetailsWantEdit(t, s)
		group, private := eventPMCalls(t, s)
		if len(private) != 1 {
			t.Fatalf("sends to the member's private chat = %d, want the one blocked copy", len(private))
		}
		if rows := eventPMKeyboardRows(eventPMParams(t, group[0])); len(rows) != 1 {
			t.Fatalf("the group reply's rows = %v, want the Unban row alone", rows)
		}
		rec, ttl := eventPMDetailsRecord(t, token)
		copyText := eventPMParams(t, private[0])["text"]
		if rec.UserID != eventPMMember || rec.ChatID != s.chat.Id || rec.Event != eventmsg.EventBan || rec.Text != copyText || rec.MediaType != db.TEXT {
			t.Fatalf("the Details record = %+v, want the copy %q of user 42 in chat %d", rec, copyText, s.chat.Id)
		}
		if ttl <= 0 || ttl > pmCopyDetailsTTL {
			t.Fatalf("the Details record lives %s, want at most %s", ttl, pmCopyDetailsTTL)
		}
		for _, line := range mreqLogLines(hook) {
			if strings.Contains(line, token) {
				t.Fatalf("a log line carries the Details token: %q", line)
			}
		}
	})

	t.Run("custom group message keeps its rows", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMDetailsOn(t, s.chat.Id)
		if err := eventmsg.SetText(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, "Out: {reason}", 1); err != nil {
			t.Fatalf("SetText(group) error = %v", err)
		}
		if err := eventmsg.SetButtons(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup,
			[]models.Button{{Name: "Rules", Url: "https://example.com/rules"}, {Name: "Site", Url: "https://example.com"}}, 1); err != nil {
			t.Fatalf("SetButtons(group) error = %v", err)
		}
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: bot can't initiate conversation with a user", 0))
		banned(t, s)
		eventPMDetailsWantEdit(t, s)
		group, _ := eventPMCalls(t, s)
		rows := eventPMKeyboardRows(eventPMParams(t, group[0]))
		if text := eventPMJSON(t, rows); len(rows) != 3 || !strings.Contains(text, "callback_data") ||
			!strings.Contains(text, "https://example.com/rules") || !strings.Contains(text, `"https://example.com"`) {
			t.Fatalf("the custom group message's rows = %s, want the Unban row and both buttons", text)
		}
	})

	// The cases that add nothing: each prepares the site, then /ban runs.
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, s *botSite)
	}{
		{"option off", func(t *testing.T, s *botSite) {
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"delivered", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
		}},
		{"group silent", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
			settingsMsgSeed(t, s.chat.Id, eventmsg.EventBan, "", true)
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"group send failed", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
			s.client.mu.Lock()
			s.client.errors["sendMessage"] = eventPMTelegramError(400, "Bad Request: not enough rights", 0)
			s.client.mu.Unlock()
			t.Cleanup(func() { s.client.mu.Lock(); delete(s.client.errors, "sendMessage"); s.client.mu.Unlock() })
		}},
		{"no username", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
			mreqSwap(t, &pmCopyBotUsername, func(*gotgbot.Bot) string { return "" })
			eventPMWaitBeforeRestore(t)
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"no redis", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
			mreqSwap(t, &pmCopyRedisClient, func() *redis.Client { return nil })
			eventPMWaitBeforeRestore(t)
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"refused then default delivered", func(t *testing.T, s *botSite) {
			eventPMDetailsOn(t, s.chat.Id)
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "custom {reason}", true)
			if err := eventmsg.SetMedia(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestPM, "pm-photo", db.PHOTO, 1); err != nil {
				t.Fatalf("SetMedia(pm) error = %v", err)
			}
			eventPMRefuse(t, s, eventPMTelegramError(400, "Bad Request: wrong file identifier", 0))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newEventPMSite(t)
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
			tc.setup(t, s)
			_ = bansModule.ban(s.bot, s.commandCtx("/ban spam", true, false))
			eventPMWantBan(t, s)
			if _, private := eventPMCalls(t, s); len(private) == 0 {
				t.Fatal("no private copy was tried, so the case proves nothing")
			}
			eventPMDetailsNothing(t, s)
		})
	}

	// offer is a blocked copy offered straight to the engine with the Details option on and a group message of rows.
	offer := func(s *botSite, event string, rows [][]gotgbot.InlineKeyboardButton) eventPMOffer {
		return eventPMOffer{
			Bot: s.bot, Chat: s.chat, Event: event, UserID: eventPMMember,
			Message:    media.GreetingMessage{Text: "copy", MsgType: db.TEXT},
			Options:    eventmsg.Options{PMDetails: true},
			GroupMsgID: 9001, GroupRows: rows, DetailsLabel: "Details",
		}
	}
	buttons := func(n int) [][]gotgbot.InlineKeyboardButton {
		rows := make([][]gotgbot.InlineKeyboardButton, n)
		for i := range rows {
			rows[i] = []gotgbot.InlineKeyboardButton{{Text: "b", Url: "https://example.com"}}
		}
		return rows
	}

	t.Run("100 buttons", func(t *testing.T) {
		s := newEventPMSite(t)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		offerEventPM(offer(s, eventmsg.EventBan, buttons(100)))
		eventPMDetailsNothing(t, s)

		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		offerEventPM(offer(s, eventmsg.EventBan, buttons(99)))
		edits := eventPMDetailsEdits(t, s)
		if len(edits) != 1 || len(eventPMKeyboardRows(edits[0])) != 100 {
			t.Fatalf("a group message of 99 buttons got edits %v, want one with 100 rows", edits)
		}
	})

	t.Run("capped", func(t *testing.T) {
		s := newEventPMSite(t)
		withTestEventPM(t, "pmtest_details_auto", eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: true})
		mreqSwap(t, &pmCopyAutoCapPerMinute, 0)
		eventPMWaitBeforeRestore(t)
		offerEventPM(offer(s, "pmtest_details_auto", nil))
		eventPMNone(t, s)
		eventPMDetailsNothing(t, s)
	})

	t.Run("edit fails", func(t *testing.T) {
		s := newEventPMSite(t)
		hook := mreqLogCapture(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMDetailsOn(t, s.chat.Id)
		s.client.mu.Lock()
		s.client.errors["editMessageReplyMarkup"] = &gotgbot.TelegramError{
			Method: "editMessageReplyMarkup", Code: 400, Description: "Bad Request: message to edit not found",
		}
		s.client.mu.Unlock()
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		banned(t, s)
		if edits := eventPMDetailsEdits(t, s); len(edits) != 1 {
			t.Fatalf("Details edits = %d, want the one refused edit and no retry", len(edits))
		}
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMWantLine(t, hook, "the Details button of chat "+strconv.FormatInt(s.chat.Id, 10)+" could not be added (code 400)")
		for _, call := range eventPMAllCalls(s) {
			if call.Method == "deleteMessage" || call.Method == "editMessageText" {
				t.Fatalf("a failed Details edit was followed by %s", call.Method)
			}
		}
	})
}

func TestPMCopyDetailsDeepLink(t *testing.T) {
	withModerationGoldenLocales(t)
	const other int64 = 43

	// open runs /start <arg> in the private chat of user and returns the requests it made.
	open := func(t *testing.T, s *botSite, user int64, arg string) []moduleBotCall {
		t.Helper()
		before := len(eventPMAllCalls(s))
		from := gotgbot.User{Id: user, FirstName: "Opener"}
		ctx := newModuleMessageContext(s.bot, gotgbot.Chat{Id: user, Type: "private", FirstName: "Opener"}, from, "/start "+arg)
		if err := HandleDeepLink(s.bot, ctx, &from, arg); err == nil {
			t.Fatalf("/start %s returned nil, want EndGroups", arg)
		}
		return eventPMAllCalls(s)[before:]
	}
	neutral := func(t *testing.T, s *botSite, user int64, arg string) {
		t.Helper()
		calls := open(t, s, user, arg)
		if len(calls) != 1 || calls[0].Method != "sendMessage" || fmt.Sprint(calls[0].Params["chat_id"]) != strconv.FormatInt(user, 10) {
			t.Fatalf("/start %q by %d made %v, want the one neutral reply", arg, user, calls)
		}
		ctx := newModuleMessageContext(s.bot, gotgbot.Chat{Id: user, Type: "private"}, gotgbot.User{Id: user}, "/start")
		want, _ := i18n.MustNewTranslator(lang.GetLanguage(ctx)).GetString("eventmsg_pm_details_none")
		if got := calls[0].Params["text"]; got != want || want == "" {
			t.Fatalf("/start %q by %d answered %q, want the neutral reply %q", arg, user, got, want)
		}
	}
	store := func(t *testing.T) string {
		t.Helper()
		token, err := pmCopyDetailsPut(t.Context(), pmCopyDetailsRecord{
			UserID: eventPMMember, ChatID: -1001234567890, Event: eventmsg.EventBan,
			Text: "You were banned: <b>spam</b>", FileID: "pm-photo", MediaType: db.PHOTO,
			Buttons: [][]gotgbot.InlineKeyboardButton{{{Text: "Site", Url: "https://example.com"}}},
		})
		if err != nil {
			t.Fatalf("pmCopyDetailsPut() error = %v", err)
		}
		return token
	}

	t.Run("the person", func(t *testing.T) {
		s := newEventPMSite(t)
		token := store(t)
		for range 2 {
			calls := open(t, s, eventPMMember, pmCopyDetailsPrefix+token)
			if len(calls) != 1 || calls[0].Method != "sendPhoto" {
				t.Fatalf("the punished person's /start made %v, want the one stored copy", calls)
			}
			params := rulesCallParams(t, calls[0])
			if fmt.Sprint(params["chat_id"]) != "42" || params["caption"] != "You were banned: <b>spam</b>" || params["photo"] != "pm-photo" {
				t.Fatalf("the shown copy = %v, want the stored photo and text in chat 42", params)
			}
			if rows := eventPMKeyboardRows(params); eventPMJSON(t, rows) != `[[{"text":"Site","url":"https://example.com"}]]` {
				t.Fatalf("the shown copy's buttons = %s, want the stored button", eventPMJSON(t, rows))
			}
			if _, ok := params["reply_parameters"]; ok {
				t.Fatal("the shown copy is a reply")
			}
		}
	})

	t.Run("someone else", func(t *testing.T) {
		s := newEventPMSite(t)
		token := store(t)
		neutral(t, s, other, pmCopyDetailsPrefix+token)
	})

	t.Run("forged", func(t *testing.T) {
		s := newEventPMSite(t)
		token := store(t)
		for _, arg := range []string{
			pmCopyDetailsPrefix,
			pmCopyDetailsPrefix + token[:21],
			pmCopyDetailsPrefix + token + "A",
			pmCopyDetailsPrefix + token[:21] + "!",
			pmCopyDetailsPrefix + token[:21] + "=",
			pmCopyDetailsPrefix + "AAAAAAAAAAAAAAAAAAAAAA",
		} {
			neutral(t, s, eventPMMember, arg)
		}

		if err := cache.GetRedisClient().Set(context.Background(), pmCopyDetailsKey("BBBBBBBBBBBBBBBBBBBBBB"), "not json", time.Hour).Err(); err != nil {
			t.Fatalf("store an unparsable record: %v", err)
		}
		neutral(t, s, eventPMMember, pmCopyDetailsPrefix+"BBBBBBBBBBBBBBBBBBBBBB")

		broken := miniredis.RunT(t)
		broken.SetError("ERR the records are down")
		client := redis.NewClient(&redis.Options{Addr: broken.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		mreqSwap(t, &pmCopyRedisClient, func() *redis.Client { return client })
		neutral(t, s, eventPMMember, pmCopyDetailsPrefix+token)
	})
}
