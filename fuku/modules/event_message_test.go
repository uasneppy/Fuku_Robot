//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/admin"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

// eventMsgBanText is the custom ban text the tests store: Welcome and event placeholders, HTML, and
// two placeholders a ban has no value for.
const eventMsgBanText = "<b>{first}</b> is out: {reason} (by {admin}){duration}{count}"

// eventMsgGoldenFirstRow is the Unban row of the golden ban reply, as JSON.
func eventMsgGoldenFirstRow(t *testing.T) string {
	t.Helper()
	var golden map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(goldenModerationBanReason, "<chat>", "1")), &golden); err != nil {
		t.Fatalf("decode the golden: %v", err)
	}
	rows := golden["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	raw, _ := json.Marshal(rows[0])
	return string(raw)
}

// eventMsgFirstRow is the first keyboard row of decoded send parameters, as JSON ("" for none).
func eventMsgFirstRow(params map[string]any) string {
	markup, _ := params["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	if len(rows) == 0 {
		return ""
	}
	raw, _ := json.Marshal(rows[0])
	return string(raw)
}

// eventMsgWantCustom checks that the one sendMessage is the custom ban text, filled for want, as a
// reply to the command after the ban, and queued under the manual kind.
func eventMsgWantCustom(t *testing.T, s *botSite, want string) {
	t.Helper()
	methods := eventMsgMethods(s.client)
	ban, send := eventMsgIndex(methods, "banChatMember"), eventMsgIndex(methods, "sendMessage")
	if ban < 0 || send < 0 || ban > send {
		t.Fatalf("calls = %v, want banChatMember before sendMessage", methods)
	}
	sends := s.client.callsFor("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d, want one", len(sends))
	}
	params := rulesCallParams(t, sends[0])
	if params["text"] != want {
		t.Fatalf("text = %q, want %q", params["text"], want)
	}
	reply, _ := json.Marshal(params["reply_parameters"])
	if string(reply) != `{"allow_sending_without_reply":true,"message_id":101}` {
		t.Fatalf("reply_parameters = %s, want message 101 with allow_sending_without_reply", reply)
	}
	preview, _ := json.Marshal(params["link_preview_options"])
	if params["parse_mode"] != "HTML" || string(preview) != `{"is_disabled":true}` {
		t.Fatalf("parse_mode = %v, link_preview_options = %s, want HTML with previews off", params["parse_mode"], preview)
	}
	if got, wantRow := eventMsgFirstRow(params), eventMsgGoldenFirstRow(t); got != wantRow {
		t.Fatalf("first keyboard row = %s, want the golden Unban row %s", got, wantRow)
	}
	s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
}

// eventMsgWantGolden checks that the one sendMessage is the golden ban reply and that it is queued.
func eventMsgWantGolden(t *testing.T, s *botSite) {
	t.Helper()
	sends := s.client.callsFor("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d, want the one default reply", len(sends))
	}
	got := strings.ReplaceAll(legacyRequestJSON(t, sends[0]), strconv.FormatInt(s.chat.Id, 10), "<chat>")
	if got != goldenModerationBanReason {
		t.Fatalf("default reply changed\n got %q\nwant %q", got, goldenModerationBanReason)
	}
	s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
}

// eventMsgRun runs one moderation command of the site and requires EndGroups.
func eventMsgRun(t *testing.T, s *botSite, call func(*botSite) error, text string) {
	t.Helper()
	if err := call(s); err != ext.EndGroups {
		t.Fatalf("%s error = %v, want EndGroups", text, err)
	}
}

func eventMsgBan(text string) func(*botSite) error {
	return func(s *botSite) error { return bansModule.ban(s.bot, s.commandCtx(text, true, false)) }
}

func TestEventMessageBanEndToEnd(t *testing.T) {
	withModerationGoldenLocales(t)
	const filled = "<b>Ann &amp; &lt;Bo&gt;</b> is out: spam &lt;b&gt; (by Telegram)"

	t.Run("custom text", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		eventMsgWantCustom(t, s, filled)
	})

	t.Run("dban uses the ban event", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
		if err := bansModule.dBan(s.bot, s.commandCtx("/dban bad post", true, false)); err != ext.EndGroups {
			t.Fatalf("/dban error = %v, want EndGroups", err)
		}
		deleted := false
		for _, call := range s.client.callsFor("deleteMessage") {
			if fmt.Sprint(call.Params["message_id"]) == "303" {
				deleted = true
			}
		}
		if !deleted {
			t.Fatalf("deleteMessage of message 303 not sent")
		}
		eventMsgWantCustom(t, s, "<b>Ann &amp; &lt;Bo&gt;</b> is out: bad post (by Telegram)")
	})

	t.Run("silent", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
		if err := eventmsg.SetSilent(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, true, 1); err != nil {
			t.Fatalf("SetSilent error = %v", err)
		}
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		if got := len(s.client.callsFor("banChatMember")); got != 1 {
			t.Fatalf("banChatMember calls = %d, want 1", got)
		}
		if got := len(s.client.callsFor("sendMessage")); got != 0 {
			t.Fatalf("sendMessage calls = %d, want none when silent", got)
		}
		s.wantNoRows(s.chat.Id)
	})

	t.Run("no row", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgCleanup(t, s.chat.Id)
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		eventMsgWantGolden(t, s)
	})

	t.Run("empty row", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
		ctx := t.Context()
		if err := eventmsg.SetText(ctx, s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, "", 1); err != nil {
			t.Fatalf("SetText error = %v", err)
		}
		if err := eventmsg.SetMedia(ctx, s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, "", 0, 1); err != nil {
			t.Fatalf("SetMedia error = %v", err)
		}
		if err := eventmsg.SetButtons(ctx, s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, nil, 1); err != nil {
			t.Fatalf("SetButtons error = %v", err)
		}
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		eventMsgWantGolden(t, s)
	})

	t.Run("read error", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
		mreqSwap(t, &eventMessagesLookup, func(context.Context, int64) (*eventmsg.Messages, error) {
			return nil, errors.New("database is down")
		})
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		eventMsgWantGolden(t, s)
	})
}

// eventMsgSiteButton is the custom button the compose tests store, as stored and as sent.
var eventMsgSiteButton = models.Button{Name: "Site", Url: "https://example.com"}

const eventMsgSiteRow = `[{"text":"Site","url":"https://example.com"}]`

// eventMsgGoldenText is the text of the golden ban reply (the default, reason line included).
func eventMsgGoldenText(t *testing.T) string {
	t.Helper()
	var golden map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(goldenModerationBanReason, "<chat>", "1")), &golden); err != nil {
		t.Fatalf("decode the golden: %v", err)
	}
	return golden["text"].(string)
}

// eventMsgRows is every keyboard row of decoded send parameters, each as JSON.
func eventMsgRows(params map[string]any) []string {
	markup, _ := params["reply_markup"].(map[string]any)
	rows, _ := markup["inline_keyboard"].([]any)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		raw, _ := json.Marshal(row)
		out = append(out, string(raw))
	}
	return out
}

func TestRenderEventText(t *testing.T) {
	withModerationGoldenLocales(t)
	s := newModerationGoldenSite(t)
	mreqSwap(t, &chatTimezoneLookup, func(context.Context, int64) (string, error) { return "UTC+05:00", nil })
	target := gotgbot.User{Id: 42, FirstName: "{reason}", LastName: "Lee", Username: "ann_lee"}
	admin := &gotgbot.User{Id: 7, FirstName: "Ad <min>", LastName: "& Co"}
	until := time.Date(2026, 3, 1, 12, 4, 0, 0, time.UTC)

	cases := []struct {
		name   string
		stored string
		values eventValues
		actor  eventActor
		want   string
	}{
		{name: "every token with a value", stored: "{reason}|{admin}|{duration}|{until}",
			values: eventValues{Reason: "a < b & c", Duration: "2 hours", Until: until}, actor: eventActor{User: admin},
			want: "a &lt; b &amp; c|Ad &lt;min&gt; &amp; Co|2 hours|01.03.2026 17:04 (UTC+5)"},
		{name: "tokens with no value render as nothing", stored: "[{reason}{duration}{until}{action}{count}{limit}{window}]",
			actor: eventActor{User: admin}, want: "[]"},
		{name: "a reason {first} and a name {reason} stay as typed", stored: "{first} said {reason}",
			values: eventValues{Reason: "{first}"}, actor: eventActor{User: admin}, want: "{reason} said {first}"},
		{name: "an unknown word stays", stored: "{foo} {first}", actor: eventActor{User: admin}, want: "{foo} {reason}"},
		{name: "an anonymous admin", stored: "by {admin}", actor: eventActor{User: admin, Anonymous: true}, want: "by адмін"},
		{name: "no text to fill", stored: "plain <b>text</b>", actor: eventActor{User: admin}, want: "plain <b>text</b>"},
	}
	tr := i18n.MustNewTranslator("uk")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := eventPlaceholderValues(tr, s.chat.Id, tc.actor, tc.values)
			got, buttons := renderEventText(s.bot, &s.chat, &target, tc.stored, nil, values)
			if got != tc.want {
				t.Fatalf("renderEventText(%q) = %q, want %q", tc.stored, got, tc.want)
			}
			if len(buttons) != 0 {
				t.Fatalf("buttons = %+v, want none", buttons)
			}
		})
	}
	if calls := s.client.callsFor("getChatMemberCount"); len(calls) != 0 {
		t.Fatalf("getChatMemberCount calls = %d, want none: {count} is the event's own value", len(calls))
	}

	t.Run("the rules button", func(t *testing.T) {
		values := eventPlaceholderValues(tr, s.chat.Id, eventActor{User: admin}, eventValues{})
		_, buttons := renderEventText(s.bot, &s.chat, &target, "read {rules}", nil, values)
		if len(buttons) != 0 {
			t.Fatalf("buttons with no rules = %+v, want none", buttons)
		}
		rulesSeed(t, s.chat.Id, "Be kind", "", 0, nil)
		text, buttons := renderEventText(s.bot, &s.chat, &target, "read {rules}", nil, values)
		if text != "read " || len(buttons) != 1 || !strings.Contains(buttons[0].Url, "start=rules_"+strconv.FormatInt(s.chat.Id, 10)) {
			t.Fatalf("with rules: text %q, buttons %+v, want the rules button", text, buttons)
		}
	})
}

func TestEventMessageCompose(t *testing.T) {
	withModerationGoldenLocales(t)
	const filled = "<b>Ann &amp; &lt;Bo&gt;</b> is out: spam &lt;b&gt; (by Telegram)"

	t.Run("custom text with a photo and buttons", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStoreRow(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText, "photo-file", db.PHOTO, []models.Button{eventMsgSiteButton})
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		params := rulesOnlyCall(t, s.client, "sendPhoto")
		if params["caption"] != filled || params["photo"] != "photo-file" {
			t.Fatalf("sendPhoto caption %q photo %v, want the filled text and the stored file", params["caption"], params["photo"])
		}
		if reply, _ := params["reply_parameters"].(map[string]any); reply["message_id"] != float64(101) {
			t.Fatalf("reply_parameters = %v, want message 101", params["reply_parameters"])
		}
		rows := eventMsgRows(params)
		if len(rows) != 2 || rows[0] != eventMsgGoldenFirstRow(t) || rows[1] != eventMsgSiteRow {
			t.Fatalf("keyboard rows = %v, want the Unban row then the custom row", rows)
		}
		s.wantRow(s.chat.Id, 9003, deletion.KindBotManual, 600*time.Second)
	})

	t.Run("a photo with no custom text carries the default text", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStoreRow(t, s.chat.Id, eventmsg.EventBan, "", "photo-file", db.PHOTO, nil)
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		params := rulesOnlyCall(t, s.client, "sendPhoto")
		if params["caption"] != eventMsgGoldenText(t) {
			t.Fatalf("caption = %q, want the default text %q", params["caption"], eventMsgGoldenText(t))
		}
		if rows := eventMsgRows(params); len(rows) != 1 || rows[0] != eventMsgGoldenFirstRow(t) {
			t.Fatalf("keyboard rows = %v, want the Unban row alone", rows)
		}
	})

	t.Run("buttons only", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStoreRow(t, s.chat.Id, eventmsg.EventBan, "", "", 0, []models.Button{eventMsgSiteButton})
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		params := rulesOnlyCall(t, s.client, "sendMessage")
		if params["text"] != eventMsgGoldenText(t) {
			t.Fatalf("text = %q, want the default text", params["text"])
		}
		if rows := eventMsgRows(params); len(rows) != 2 || rows[0] != eventMsgGoldenFirstRow(t) || rows[1] != eventMsgSiteRow {
			t.Fatalf("keyboard rows = %v, want the Unban row then the custom row", rows)
		}
		s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
	})

	t.Run("no bot row and no buttons, and the note", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		tr := i18n.MustNewTranslator("uk")
		r := eventReply{Event: eventmsg.EventUnmute, Target: gotgbot.User{Id: 42, FirstName: "Ann"},
			Default: "default text\nlockdown note", Opts: formatting.Shtml(), Note: "lockdown note"}
		custom := composeEventMessage(s.bot, &s.chat, tr, r, eventmsg.Message{Text: "{first} may speak"}, eventActor{})
		if custom.Keyboard != nil {
			t.Fatalf("keyboard = %+v, want none", custom.Keyboard)
		}
		if custom.Text != "Ann may speak\nlockdown note" || custom.MsgType != db.TEXT {
			t.Fatalf("custom text = %q type %d, want the filled text, a newline and the note", custom.Text, custom.MsgType)
		}
		media := composeEventMessage(s.bot, &s.chat, tr, r, eventmsg.Message{FileID: "photo-file", MediaType: db.PHOTO}, eventActor{})
		if media.Text != r.Default || media.FileID != "photo-file" || media.MsgType != db.PHOTO {
			t.Fatalf("default with media = %+v, want the default text once (no second note)", media)
		}
	})
}

func TestEventMessageFallback(t *testing.T) {
	withModerationGoldenLocales(t)
	refused := &gotgbot.TelegramError{Method: "sendMessage", Code: 400, Description: "Bad Request: can't parse entities"}

	t.Run("a refused photo sends the default once", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStoreRow(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText, "foreign-file", db.PHOTO, nil)
		eventMsgRefuseOnce(s, "sendPhoto", &gotgbot.TelegramError{Method: "sendPhoto", Code: 400, Description: "Bad Request: wrong file identifier/HTTP URL specified"})
		eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
		if got := len(s.client.callsFor("sendPhoto")); got != 1 {
			t.Fatalf("sendPhoto calls = %d, want the one refused", got)
		}
		eventMsgWantGolden(t, s)
	})

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "a refused custom text sends the default once", err: refused},
		{name: "ErrNoPermission sends the default once", err: errors.New("Forbidden: CHAT_WRITE_FORBIDDEN")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newModerationGoldenSite(t)
			eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
			eventMsgRefuseOnce(s, "sendMessage", tc.err)
			eventMsgRun(t, s, eventMsgBan("/ban spam <b>"), "/ban")
			sends := s.client.callsFor("sendMessage")
			if len(sends) != 2 {
				t.Fatalf("sendMessage calls = %d, want the refused custom text and the default", len(sends))
			}
			methods := eventMsgMethods(s.client)
			if eventMsgIndex(methods, "banChatMember") > eventMsgIndex(methods, "sendMessage") {
				t.Fatalf("calls = %v, want the ban first", methods)
			}
			got := strings.ReplaceAll(legacyRequestJSON(t, sends[1]), strconv.FormatInt(s.chat.Id, 10), "<chat>")
			if got != goldenModerationBanReason {
				t.Fatalf("fallback reply\n got %q\nwant %q", got, goldenModerationBanReason)
			}
			s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
		})
	}

	for _, code := range []int{429, 502} {
		t.Run("a "+strconv.Itoa(code)+" makes no second send", func(t *testing.T) {
			s := newModerationGoldenSite(t)
			eventMsgStore(t, s.chat.Id, eventmsg.EventBan, eventMsgBanText)
			eventMsgRefuseOnce(s, "sendMessage", &gotgbot.TelegramError{Method: "sendMessage", Code: code, Description: "busy"})
			err := bansModule.ban(s.bot, s.commandCtx("/ban spam <b>", true, false))
			var tgErr *gotgbot.TelegramError
			if !errors.As(err, &tgErr) || tgErr.Code != code {
				t.Fatalf("/ban error = %v, want the %d returned", err, code)
			}
			if got := len(s.client.callsFor("banChatMember")); got != 1 {
				t.Fatalf("banChatMember calls = %d, want the ban applied", got)
			}
			if got := len(s.client.callsFor("sendMessage")); got != 1 {
				t.Fatalf("sendMessage calls = %d, want no second send", got)
			}
			s.wantNoRows(s.chat.Id)
		})
	}
}

// eventMsgAnonCtx is a /ban of the group's anonymous admin replying to the target: sender_chat the
// group, from the Group Anonymous Bot.
func eventMsgAnonCtx(s *botSite, text string) *ext.Context {
	msg := &gotgbot.Message{
		MessageId: 101, Date: s.now.Unix(), Chat: s.chat, Text: text,
		From:       &gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"},
		SenderChat: &gotgbot.Chat{Id: s.chat.Id, Type: s.chat.Type, Title: s.chat.Title},
		ReplyToMessage: &gotgbot.Message{MessageId: 303, Date: 1, Chat: s.chat, From: &botSiteTarget,
			Text: "message being moderated"},
	}
	return ext.NewContext(s.bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)
}

func TestEventMessageAnonymousAdmin(t *testing.T) {
	withModerationGoldenLocales(t)
	const stored = "{first} is out (by {admin})"

	t.Run("an anonymous admin in AnonAdmin mode", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, stored)
		if err := admin.SetAnonAdminMode(s.chat.Id, true); err != nil {
			t.Fatalf("SetAnonAdminMode error = %v", err)
		}
		t.Cleanup(func() { db.DB.Where("chat_id = ?", s.chat.Id).Delete(&models.AdminSettings{}) })
		if err := bansModule.ban(s.bot, eventMsgAnonCtx(s, "/ban spam")); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		params := rulesOnlyCall(t, s.client, "sendMessage")
		if params["text"] != "Ann &amp; &lt;Bo&gt; is out (by адмін)" {
			t.Fatalf("text = %q, want {admin} as the group language's \"an admin\"", params["text"])
		}
	})

	t.Run("the proof tap's mark on a named admin", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventBan, stored)
		ctx := s.commandCtx("/ban spam", true, false)
		markAnonymousActor(ctx)
		if err := bansModule.ban(s.bot, ctx); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		params := rulesOnlyCall(t, s.client, "sendMessage")
		if params["text"] != "Ann &amp; &lt;Bo&gt; is out (by адмін)" {
			t.Fatalf("text = %q, want {admin} as \"an admin\"", params["text"])
		}
	})
}

func TestEventMessageLogsCarryIDsOnly(t *testing.T) {
	withModerationGoldenLocales(t)
	const (
		secretText   = "SECRET-TEXT {first}"
		secretFile   = "SECRET-FILE-ID"
		secretButton = "SECRET-BUTTON"
		secretLink   = "https://secret.example/link"
	)
	forbidden := []string{"SECRET-TEXT", secretFile, secretButton, secretLink, "Ann", "Bo&"}
	store := func(t *testing.T, s *botSite) {
		eventMsgStoreRow(t, s.chat.Id, eventmsg.EventBan, secretText, secretFile, db.PHOTO,
			[]models.Button{{Name: secretButton, Url: secretLink}})
	}

	t.Run("fallback", func(t *testing.T) {
		hook := mreqLogCapture(t)
		s := newModerationGoldenSite(t)
		store(t, s)
		eventMsgRefuseOnce(s, "sendPhoto", &gotgbot.TelegramError{Method: "sendPhoto", Code: 400, Description: "Bad Request: wrong file identifier"})
		eventMsgRun(t, s, eventMsgBan("/ban spam"), "/ban")
		mreqWantCleanLogs(t, hook, forbidden, "custom ban message of chat "+strconv.FormatInt(s.chat.Id, 10)+" was refused")
	})

	t.Run("429", func(t *testing.T) {
		hook := mreqLogCapture(t)
		s := newModerationGoldenSite(t)
		store(t, s)
		eventMsgRefuseOnce(s, "sendPhoto", &gotgbot.TelegramError{Method: "sendPhoto", Code: 429, Description: "Too Many Requests: retry after 3"})
		_ = bansModule.ban(s.bot, s.commandCtx("/ban spam", true, false))
		mreqWantCleanLogs(t, hook, forbidden, "Too Many Requests")
	})

	t.Run("read error", func(t *testing.T) {
		hook := mreqLogCapture(t)
		s := newModerationGoldenSite(t)
		store(t, s)
		mreqSwap(t, &eventMessagesLookup, func(context.Context, int64) (*eventmsg.Messages, error) {
			return nil, errors.New("database is down")
		})
		eventMsgRun(t, s, eventMsgBan("/ban spam"), "/ban")
		mreqWantCleanLogs(t, hook, forbidden, "messages of chat "+strconv.FormatInt(s.chat.Id, 10)+" could not be read for ban")
	})

	t.Run("silent", func(t *testing.T) {
		hook := mreqLogCapture(t)
		s := newModerationGoldenSite(t)
		store(t, s)
		if err := eventmsg.SetSilent(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, true, 1); err != nil {
			t.Fatalf("SetSilent error = %v", err)
		}
		eventMsgRun(t, s, eventMsgBan("/ban spam"), "/ban")
		mreqWantCleanLogs(t, hook, forbidden, "ban message of chat "+strconv.FormatInt(s.chat.Id, 10)+" is silent")
	})
}
