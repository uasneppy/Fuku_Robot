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

	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
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
