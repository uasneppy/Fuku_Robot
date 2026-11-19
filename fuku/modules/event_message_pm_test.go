//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// eventPMWantGroupGolden checks that the one send to the group is the golden default reply.
func eventPMWantGroupGolden(t *testing.T, s *botSite, golden string) {
	t.Helper()
	group, _ := eventPMCalls(t, s)
	if len(group) != 1 || group[0].Method != "sendMessage" {
		t.Fatalf("sends to the group = %d, want the one default reply", len(group))
	}
	got := strings.ReplaceAll(legacyRequestJSON(t, moduleBotCall{Method: group[0].Method, Params: group[0].Params}),
		strconv.FormatInt(s.chat.Id, 10), "<chat>")
	if got != golden {
		t.Fatalf("group reply changed\n got %q\nwant %q", got, golden)
	}
}

// eventPMWantBan checks that the member was banned once, before any reply.
func eventPMWantBan(t *testing.T, s *botSite) {
	t.Helper()
	bans := s.client.callsFor("banChatMember")
	if len(bans) != 1 || fmt.Sprint(bans[0].Params["user_id"]) != strconv.FormatInt(eventPMMember, 10) {
		t.Fatalf("banChatMember calls = %v, want one of user %d", bans, eventPMMember)
	}
	methods := eventMsgMethods(s.client)
	if ban, send := eventMsgIndex(methods, "banChatMember"), eventMsgIndex(methods, "sendMessage"); send >= 0 && send < ban {
		t.Fatalf("calls = %v, want the ban before every send", methods)
	}
}

func TestEventPMEndToEnd(t *testing.T) {
	withModerationGoldenLocales(t)

	t.Run("custom copy", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "<b>{first}</b>, out of {chatname}: {reason} (by {admin})", true)
		if err := bansModule.ban(s.bot, s.commandCtx("/ban spam <b>", true, false)); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		eventPMWantBan(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		params := eventPMOnly(t, s, "sendMessage")
		want := "<b>Ann &amp; &lt;Bo&gt;</b>, out of Bot Site Chat: spam &lt;b&gt; (by Telegram)"
		if params["text"] != want {
			t.Fatalf("private copy text = %q, want %q", params["text"], want)
		}
		if preview, _ := params["link_preview_options"].(map[string]any); preview["is_disabled"] != true {
			t.Fatalf("private copy link_preview_options = %v, want previews off", params["link_preview_options"])
		}
		if params["reply_markup"] != nil {
			t.Fatalf("private copy reply_markup = %v, want none (no Unban row)", params["reply_markup"])
		}
		client := cache.GetRedisClient()
		if n, err := client.Exists(context.Background(), "fuku:pmcopy:pace:next").Result(); err != nil || n != 1 {
			t.Fatalf("the private copies' pacer key exists = %d (%v), want the copy paced on its own keys", n, err)
		}
	})

	t.Run("dban uses the ban copy", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "out: {reason}", true)
		if err := bansModule.dBan(s.bot, s.commandCtx("/dban bad post", true, false)); err != ext.EndGroups {
			t.Fatalf("/dban error = %v, want EndGroups", err)
		}
		deleted := false
		for _, call := range s.client.callsFor("deleteMessage") {
			deleted = deleted || fmt.Sprint(call.Params["message_id"]) == "303"
		}
		if !deleted {
			t.Fatal("deleteMessage of message 303 not sent")
		}
		eventPMWantBan(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationDban)
		if params := eventPMOnly(t, s, "sendMessage"); params["text"] != "out: bad post" {
			t.Fatalf("private copy text = %q, want the ban copy with the reason", params["text"])
		}
	})

	t.Run("off by default", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "never sent {reason}", false)
		if err := bansModule.ban(s.bot, s.commandCtx("/ban spam <b>", true, false)); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		eventPMWantBan(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMNone(t, s)
	})

	t.Run("no pm row", func(t *testing.T) {
		s := newEventPMSite(t)
		if err := bansModule.ban(s.bot, s.commandCtx("/ban spam <b>", true, false)); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMNone(t, s)
	})

	t.Run("read error", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "never sent", true)
		mreqSwap(t, &eventMessagesLookup, func(context.Context, int64) (*eventmsg.Messages, error) {
			return nil, errors.New("database unavailable")
		})
		if err := bansModule.ban(s.bot, s.commandCtx("/ban spam <b>", true, false)); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		eventPMWantBan(t, s)
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMNone(t, s)
	})

	t.Run("group silent", func(t *testing.T) {
		s := newEventPMSite(t)
		if err := eventmsg.SetSilent(t.Context(), s.chat.Id, eventmsg.EventBan, eventmsg.DestGroup, true, 1); err != nil {
			t.Fatalf("SetSilent(group) error = %v", err)
		}
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "out: {reason}", true)
		if err := bansModule.ban(s.bot, s.commandCtx("/ban spam", true, false)); err != ext.EndGroups {
			t.Fatalf("/ban error = %v, want EndGroups", err)
		}
		eventPMWantBan(t, s)
		if group, _ := eventPMCalls(t, s); len(group) != 0 {
			t.Fatalf("sends to the group = %d, want none when the group message is silent", len(group))
		}
		if params := eventPMOnly(t, s, "sendMessage"); params["text"] != "out: spam" {
			t.Fatalf("private copy text = %q, want the copy although the group message is silent", params["text"])
		}
	})
}
