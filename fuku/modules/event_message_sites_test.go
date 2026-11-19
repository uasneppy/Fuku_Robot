//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
)

// eventMsgSiteText is the custom text every site test stores: the target's name and the event's own
// values, so one text shows which values an event fills and which it leaves empty.
const eventMsgSiteText = "{first}|{reason}|{duration}|{until}|{admin}"

// eventMsgSiteCase is one manual punishment command and the event its reply belongs to. want is the
// filled custom text, with "<until>" standing for the end date the punishment call carried.
type eventMsgSiteCase struct {
	name     string
	text     string
	reply    bool
	call     func(b *gotgbot.Bot, ctx *ext.Context) error
	event    string
	method   string // the punishment call, made before the reply
	want     string
	golden   string // the default reply with nothing stored
	lockdown bool
}

// eventMsgUnmuteLockdownNote is the Ukrainian mutes_unmute_lockdown_note, the line the golden of an
// unmute during a lockdown ends with.
const eventMsgUnmuteLockdownNote = "Ця група під локдауном, тож писати можна буде, щойно адмін виконає /unlockdown."

func eventMsgSiteCases() []eventMsgSiteCase {
	const name = "Ann &amp; &lt;Bo&gt;"
	return []eventMsgSiteCase{
		{name: "tban", text: "/tban 2h flood", reply: true, call: bansModule.tBan, event: eventmsg.EventTimedBan,
			method: "banChatMember", want: name + "|flood|2 hours|<until>|Telegram", golden: goldenModerationTban},
		{name: "kick", text: "/kick spam", reply: true, call: bansModule.kick, event: eventmsg.EventKick,
			method: "unbanChatMember", want: name + "|spam|||Telegram", golden: goldenModerationKickReason},
		{name: "dkick", text: "/dkick", reply: true, call: bansModule.dkick, event: eventmsg.EventKick,
			method: "unbanChatMember", want: name + "||||Telegram", golden: goldenModerationDkick},
		{name: "unban", text: "/unban 42", call: bansModule.unban, event: eventmsg.EventUnban,
			method: "unbanChatMember", want: name + "||||Telegram", golden: goldenModerationUnban},
		{name: "mute", text: "/mute noise", reply: true, call: mutesModule.mute, event: eventmsg.EventMute,
			method: "restrictChatMember", want: name + "|noise|||Telegram", golden: goldenModerationMuteReason},
		{name: "dmute", text: "/dmute", reply: true, call: mutesModule.dMute, event: eventmsg.EventMute,
			method: "restrictChatMember", want: name + "||||Telegram", golden: goldenModerationDmute},
		{name: "tmute", text: "/tmute 30m", reply: true, call: mutesModule.tMute, event: eventmsg.EventTimedMute,
			method: "restrictChatMember", want: name + "||30 minutes|<until>|Telegram", golden: goldenModerationTmute},
		{name: "unmute", text: "/unmute 42", call: mutesModule.unmute, event: eventmsg.EventUnmute,
			method: "restrictChatMember", want: name + "||||Telegram", golden: goldenModerationUnmute},
		{name: "unmute during a lockdown", text: "/unmute 42", call: mutesModule.unmute, event: eventmsg.EventUnmute,
			method: "restrictChatMember", want: name + "||||Telegram\n" + eventMsgUnmuteLockdownNote,
			golden: goldenModerationUnmuteLockdown, lockdown: true},
	}
}

// eventMsgGoldenParams decodes a golden request of the moderation goldens.
func eventMsgGoldenParams(t *testing.T, golden string) map[string]any {
	t.Helper()
	var params map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(golden, "<chat>", "1")), &params); err != nil {
		t.Fatalf("decode the golden: %v", err)
	}
	return params
}

// eventMsgSiteUntil is the {until} value of a timed punishment: the until_date the punishment call
// carried, in the chat's zone; "" when the call carried none.
func eventMsgSiteUntil(t *testing.T, s *botSite, method string) string {
	t.Helper()
	calls := s.client.callsFor(method)
	if len(calls) != 1 {
		t.Fatalf("%s calls = %d, want 1", method, len(calls))
	}
	raw, ok := calls[0].Params["until_date"]
	if !ok {
		return ""
	}
	until, err := strconv.ParseInt(fmt.Sprint(raw), 10, 64)
	if err != nil || until <= 0 {
		return ""
	}
	return formatChatTime(time.Unix(until, 0), chatClockOf(s.chat.Id), "02.01.2006 15:04")
}

// eventMsgWantSiteCustom checks that the one sendMessage is the custom text, filled for tc, sent
// after the punishment as a reply to the command with the default's own keyboard row first, and
// queued under the manual kind.
func eventMsgWantSiteCustom(t *testing.T, s *botSite, tc eventMsgSiteCase, want string) {
	t.Helper()
	methods := eventMsgMethods(s.client)
	punish, send := eventMsgIndex(methods, tc.method), eventMsgIndex(methods, "sendMessage")
	if punish < 0 || send < 0 || punish > send {
		t.Fatalf("calls = %v, want %s before sendMessage", methods, tc.method)
	}
	params := rulesOnlyCall(t, s.client, "sendMessage")
	if params["text"] != want {
		t.Fatalf("text = %q, want %q", params["text"], want)
	}
	reply, _ := json.Marshal(params["reply_parameters"])
	if string(reply) != `{"allow_sending_without_reply":true,"message_id":101}` {
		t.Fatalf("reply_parameters = %s, want message 101 with allow_sending_without_reply", reply)
	}
	if params["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %v, want HTML", params["parse_mode"])
	}
	if got, wantRow := eventMsgFirstRow(params), eventMsgFirstRow(eventMsgGoldenParams(t, tc.golden)); got != wantRow {
		t.Fatalf("first keyboard row = %q, want the default's own row %q", got, wantRow)
	}
	s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
}

// eventMsgWantSiteGolden checks that the one sendMessage is the command's golden default reply and
// that it is queued under the manual kind.
func eventMsgWantSiteGolden(t *testing.T, s *botSite, golden string) {
	t.Helper()
	sends := s.client.callsFor("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d, want the one default reply", len(sends))
	}
	got := strings.ReplaceAll(legacyRequestJSON(t, sends[0]), strconv.FormatInt(s.chat.Id, 10), "<chat>")
	if got != golden {
		t.Fatalf("default reply changed\n got %q\nwant %q", got, golden)
	}
	s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
}

func TestEventMessageSites(t *testing.T) {
	withModerationGoldenLocales(t)
	for _, tc := range eventMsgSiteCases() {
		t.Run(tc.name+" custom", func(t *testing.T) {
			s := newModerationGoldenSite(t)
			if tc.lockdown {
				moderationGoldenActiveLockdown(t, s.chat.Id)
			}
			eventMsgStore(t, s.chat.Id, tc.event, eventMsgSiteText)
			if err := tc.call(s.bot, s.commandCtx(tc.text, tc.reply, false)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.text, err)
			}
			until := eventMsgSiteUntil(t, s, tc.method)
			if strings.Contains(tc.want, "<until>") && until == "" {
				t.Fatalf("%s carried no until_date, want a timed punishment", tc.method)
			}
			want := strings.ReplaceAll(tc.want, "<until>", until)
			eventMsgWantSiteCustom(t, s, tc, want)
		})
		t.Run(tc.name+" nothing stored", func(t *testing.T) {
			s := newModerationGoldenSite(t)
			if tc.lockdown {
				moderationGoldenActiveLockdown(t, s.chat.Id)
			}
			eventMsgCleanup(t, s.chat.Id)
			if err := tc.call(s.bot, s.commandCtx(tc.text, tc.reply, false)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.text, err)
			}
			eventMsgWantSiteGolden(t, s, tc.golden)
		})
	}
}

// eventMsgProofTap stores text as an anonymous admin's cached command in the site's chat and taps
// its proof button as the site's admin, so verifyAnonymousAdmin re-enters the command.
func eventMsgProofTap(t *testing.T, s *botSite, text string) {
	t.Helper()
	if cache.GetMarshal() == nil {
		t.Fatal("the cache marshaler is not initialized")
	}
	cached := &gotgbot.Message{
		MessageId: 101, Date: s.now.Unix(), Chat: s.chat, Text: text,
		From:       &gotgbot.User{Id: 1087968824, IsBot: true, FirstName: "Group", Username: "GroupAnonymousBot"},
		SenderChat: &gotgbot.Chat{Id: s.chat.Id, Type: s.chat.Type, Title: s.chat.Title},
		ReplyToMessage: &gotgbot.Message{MessageId: 303, Date: 1, Chat: s.chat, From: &botSiteTarget,
			Text: "message being moderated"},
	}
	if err := chat_status.StoreAnonAdminCommand(s.chat.Id, cached); err != nil {
		t.Fatalf("store the anonymous admin's command: %v", err)
	}
	t.Cleanup(func() {
		_ = cache.GetMarshal().Delete(cache.Context, fmt.Sprintf("fuku:anonAdmin:%d:%d", s.chat.Id, cached.MessageId))
	})
	data := encodeCallbackData("anon_admin", map[string]string{
		"c": strconv.FormatInt(s.chat.Id, 10),
		"m": strconv.FormatInt(cached.MessageId, 10),
	})
	if err := verifyAnonymousAdmin(s.bot, newModuleCallbackContext(s.bot, s.chat, botSiteAdmin, data)); err != ext.EndGroups {
		t.Fatalf("verifyAnonymousAdmin() error = %v, want EndGroups", err)
	}
}

func TestEventMessageAfterAnonymousProof(t *testing.T) {
	withModerationGoldenLocales(t)
	const stored = "{first} is out (by {admin})"
	cases := []struct {
		name, text, event, method string
	}{
		{name: "ban", text: "/ban spam", event: eventmsg.EventBan, method: "banChatMember"},
		{name: "mute", text: "/mute noise", event: eventmsg.EventMute, method: "restrictChatMember"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" after the proof tap", func(t *testing.T) {
			s := newModerationGoldenSite(t)
			eventMsgStore(t, s.chat.Id, tc.event, stored)
			eventMsgProofTap(t, s, tc.text)
			if got := len(s.client.callsFor(tc.method)); got != 1 {
				t.Fatalf("%s calls = %d, want the punishment applied once", tc.method, got)
			}
			params := rulesOnlyCall(t, s.client, "sendMessage")
			if params["text"] != "Ann &amp; &lt;Bo&gt; is out (by адмін)" {
				t.Fatalf("text = %q, want {admin} as the group language's \"an admin\", not the tapper", params["text"])
			}
			s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
		})
		t.Run(tc.name+" sent directly", func(t *testing.T) {
			s := newModerationGoldenSite(t)
			eventMsgStore(t, s.chat.Id, tc.event, stored)
			if err := bansModuleOrMutes(tc.event)(s.bot, s.commandCtx(tc.text, true, false)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.text, err)
			}
			params := rulesOnlyCall(t, s.client, "sendMessage")
			if params["text"] != "Ann &amp; &lt;Bo&gt; is out (by Telegram)" {
				t.Fatalf("text = %q, want {admin} naming the admin who sent the command", params["text"])
			}
		})
	}
}

// bansModuleOrMutes is the handler of the ban or the mute command, by event.
func bansModuleOrMutes(event string) func(b *gotgbot.Bot, ctx *ext.Context) error {
	if event == eventmsg.EventMute {
		return mutesModule.mute
	}
	return bansModule.ban
}

func TestEventMessageSilentVariants(t *testing.T) {
	withModerationGoldenLocales(t)
	silentCommands := []struct {
		name, text, event, method string
		reply                     bool
		call                      func(b *gotgbot.Bot, ctx *ext.Context) error
	}{
		{name: "sban", text: "/sban 42", event: eventmsg.EventBan, method: "banChatMember", call: bansModule.sBan},
		{name: "skick", text: "/skick", reply: true, event: eventmsg.EventKick, method: "unbanChatMember", call: bansModule.sKick},
		{name: "smute", text: "/smute 42", event: eventmsg.EventMute, method: "restrictChatMember", call: mutesModule.sMute},
	}
	for _, cmd := range silentCommands {
		for _, silent := range []bool{false, true} {
			name := cmd.name + " with a custom text"
			if silent {
				name = cmd.name + " with Silent"
			}
			t.Run(name, func(t *testing.T) {
				s := newModerationGoldenSite(t)
				eventMsgStore(t, s.chat.Id, cmd.event, eventMsgSiteText)
				if silent {
					if err := eventmsg.SetSilent(t.Context(), s.chat.Id, cmd.event, eventmsg.DestGroup, true, 1); err != nil {
						t.Fatalf("SetSilent error = %v", err)
					}
				}
				if err := cmd.call(s.bot, s.commandCtx(cmd.text, cmd.reply, false)); err != ext.EndGroups {
					t.Fatalf("%s error = %v, want EndGroups", cmd.text, err)
				}
				if got := len(s.client.callsFor(cmd.method)); got != 1 {
					t.Fatalf("%s calls = %d, want the punishment applied once", cmd.method, got)
				}
				if got := len(s.client.callsFor("sendMessage")); got != 0 {
					t.Fatalf("sendMessage calls = %d, want none from a silent command", got)
				}
				s.wantNoRows(s.chat.Id)
			})
		}
	}

	t.Run("the channel unban keeps its reply", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventUnban, eventMsgSiteText)
		if err := bansModule.unban(s.bot, s.commandCtx("/unban -1001234567890", true, true)); err != ext.EndGroups {
			t.Fatalf("/unban error = %v, want EndGroups", err)
		}
		if got := len(s.client.callsFor("unbanChatSenderChat")); got != 1 {
			t.Fatalf("unbanChatSenderChat calls = %d, want 1", got)
		}
		eventMsgWantSiteGolden(t, s, goldenModerationUnbanChannel)
	})

	t.Run("unmute with Silent", func(t *testing.T) {
		s := newModerationGoldenSite(t)
		eventMsgStore(t, s.chat.Id, eventmsg.EventUnmute, eventMsgSiteText)
		if err := eventmsg.SetSilent(t.Context(), s.chat.Id, eventmsg.EventUnmute, eventmsg.DestGroup, true, 1); err != nil {
			t.Fatalf("SetSilent error = %v", err)
		}
		if err := mutesModule.unmute(s.bot, s.commandCtx("/unmute 42", false, false)); err != ext.EndGroups {
			t.Fatalf("/unmute error = %v, want EndGroups", err)
		}
		restricts := s.client.callsFor("restrictChatMember")
		if len(restricts) != 1 {
			t.Fatalf("restrictChatMember calls = %d, want 1", len(restricts))
		}
		permissions, _ := json.Marshal(restricts[0].Params["permissions"])
		if !strings.Contains(string(permissions), `"can_send_messages":true`) {
			t.Fatalf("permissions = %s, want the unmute permissions (messages allowed)", permissions)
		}
		if got := len(s.client.callsFor("sendMessage")); got != 0 {
			t.Fatalf("sendMessage calls = %d, want none when silent", got)
		}
		s.wantNoRows(s.chat.Id)
	})
}
