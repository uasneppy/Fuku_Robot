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
