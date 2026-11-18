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
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Goldens recorded on the code as it was before Phase 20 (the event messages); <chat> stands for the
// case's chat ID. They are never re-recorded: a later change that alters one of these requests for a
// group with nothing stored is a regression.
const (
	goldenModerationBanReason      = "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Розбанити (лише адміни)\",\"callback_data\":\"unrestrict|v1|a=unban\\u0026u=42\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Видалення з об'єкта виконано: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e забанено.\\n\\u003cb\\u003eПричина:\\u003c/b\\u003e spam \\u0026lt;b\\u0026gt;\"}"
	goldenModerationBanNoReason    = "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Розбанити (лише адміни)\",\"callback_data\":\"unrestrict|v1|a=unban\\u0026u=42\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Видалення з об'єкта виконано: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e забанено.\"}"
	goldenModerationBanChannel     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Забанено: \\u003ca href=\\\"tg://user?id=-1001234567890\\\"\\u003eSpam Channel\\u003c/a\\u003e\"}"
	goldenModerationDban           = "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Розбанити (лише адміни)\",\"callback_data\":\"unrestrict|v1|a=unban\\u0026u=42\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Видалення з об'єкта виконано: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e забанено.\\n\\u003cb\\u003eПричина:\\u003c/b\\u003e bad post\"}"
	goldenModerationTban           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Тимчасове видалення з об'єкта: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e забанено на 2 hours.\\n\\u003cb\\u003eПричина:\\u003c/b\\u003e flood\"}"
	goldenModerationKickReason     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"\\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e вигнано з об'єкта.\\n\\u003cb\\u003eПричина:\\u003c/b\\u003e spam\"}"
	goldenModerationDkick          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"\\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e вигнано з об'єкта.\"}"
	goldenModerationMuteReason     = "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Зняти мут (лише адміни)\",\"callback_data\":\"unrestrict|v1|a=unmute\\u0026u=42\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Режим тиші запроваджено: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e отримує мут.\\n\\u003cb\\u003eПричина:\\u003c/b\\u003e noise\"}"
	goldenModerationDmute          = "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Зняти мут (лише адміни)\",\"callback_data\":\"unrestrict|v1|a=unmute\\u0026u=42\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Режим тиші запроваджено: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e отримує мут.\"}"
	goldenModerationTmute          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим тиші запроваджено: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e отримує мут на 30 minutes.\"}"
	goldenModerationUnban          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Доступ відновлено: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e розбанено.\"}"
	goldenModerationUnbanChannel   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Розбанено: \\u003ca href=\\\"tg://user?id=-1001234567890\\\"\\u003eSpam Channel\\u003c/a\\u003e\"}"
	goldenModerationUnmute         = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим тиші скасовано: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e знову може писати.\"}"
	goldenModerationUnmuteLockdown = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим тиші скасовано: \\u003ca href=\\\"tg://user?id=42\\\"\\u003eAnn \\u0026amp; \\u0026lt;Bo\\u0026gt;\\u003c/a\\u003e знову може писати.\\nЦя група під локдауном, тож писати можна буде, щойно адмін виконає /unlockdown.\"}"
)

// moderationGoldenTarget is the member the moderation goldens punish. The name carries an ampersand and
// angle brackets, so the goldens pin how the mention is escaped today.
var moderationGoldenTarget = gotgbot.Chat{
	Id: 42, Type: "private", FirstName: "Ann & <Bo>", LastName: "Lee", Username: "ann_lee",
}

// moderationTargetClient is the site's scripted client with one change: a getChat of the target's ID
// answers the target as a private chat (the base fake answers every getChat with a group record).
// Every other request, the group's own getChat included, is the embedded client's.
type moderationTargetClient struct {
	*moduleBotClient
}

func (c moderationTargetClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	response, err := c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
	if err != nil || method != "getChat" || fmt.Sprint(params["chat_id"]) != strconv.FormatInt(moderationGoldenTarget.Id, 10) {
		return response, err
	}
	raw, marshalErr := json.Marshal(moderationGoldenTarget)
	if marshalErr != nil {
		return nil, marshalErr
	}
	return raw, nil
}

// moderationGoldenLocales counts the tests that installed the real locales for every site they make
// (withModerationGoldenLocales), so a site made inside one does not load the locale files again.
var moderationGoldenLocales int

// withModerationGoldenLocales installs the real locales once for a test whose subtests each make a
// site: loading the eight locale files takes about a second under the race detector.
func withModerationGoldenLocales(t *testing.T) {
	t.Helper()
	withRealLocales(t)
	moderationGoldenLocales++
	t.Cleanup(func() { moderationGoldenLocales-- })
}

// newModerationGoldenSite is a bot site in a fresh supergroup whose client answers the target's
// getChat, with the real locales and a 600 s rule for the manual punishment kind.
func newModerationGoldenSite(t *testing.T) *botSite {
	t.Helper()
	if moderationGoldenLocales == 0 {
		withRealLocales(t)
	}
	s := newBotSite(t, "supergroup")
	s.bot.BotClient = moderationTargetClient{moduleBotClient: s.client}
	s.setRule(s.chat.Id, deletion.KindBotManual, 600)
	botSiteWarnsCleanup(t, s.chat.Id)
	return s
}

// moderationGoldenCase is one command of TestLegacyModerationRequests.
type moderationGoldenCase struct {
	name     string
	text     string
	reply    bool
	channel  bool
	call     func(b *gotgbot.Bot, ctx *ext.Context) error
	method   string // the punishment call
	idKey    string // the parameter naming who is punished
	wantID   int64
	golden   string // the sendMessage request; empty for a silent command
	silent   bool
	lockdown bool
	deletes  int64 // the message the command deletes before punishing; 0 for none checked
}

func moderationGoldenCases() []moderationGoldenCase {
	const channel int64 = -1001234567890
	user := moderationGoldenTarget.Id
	return []moderationGoldenCase{
		{name: "ban_reason", text: "/ban spam <b>", reply: true, call: bansModule.ban,
			method: "banChatMember", idKey: "user_id", wantID: user, golden: goldenModerationBanReason},
		{name: "ban_no_reason", text: "/ban", reply: true, call: bansModule.ban,
			method: "banChatMember", idKey: "user_id", wantID: user, golden: goldenModerationBanNoReason},
		{name: "ban_channel", text: "/ban -1001234567890", reply: true, channel: true, call: bansModule.ban,
			method: "banChatSenderChat", idKey: "sender_chat_id", wantID: channel, golden: goldenModerationBanChannel},
		{name: "dban", text: "/dban bad post", reply: true, call: bansModule.dBan,
			method: "banChatMember", idKey: "user_id", wantID: user, golden: goldenModerationDban, deletes: 303},
		{name: "tban", text: "/tban 2h flood", reply: true, call: bansModule.tBan,
			method: "banChatMember", idKey: "user_id", wantID: user, golden: goldenModerationTban},
		{name: "kick_reason", text: "/kick spam", reply: true, call: bansModule.kick,
			method: "unbanChatMember", idKey: "user_id", wantID: user, golden: goldenModerationKickReason},
		{name: "dkick", text: "/dkick", reply: true, call: bansModule.dkick,
			method: "unbanChatMember", idKey: "user_id", wantID: user, golden: goldenModerationDkick},
		{name: "mute_reason", text: "/mute noise", reply: true, call: mutesModule.mute,
			method: "restrictChatMember", idKey: "user_id", wantID: user, golden: goldenModerationMuteReason},
		{name: "dmute", text: "/dmute", reply: true, call: mutesModule.dMute,
			method: "restrictChatMember", idKey: "user_id", wantID: user, golden: goldenModerationDmute},
		{name: "tmute", text: "/tmute 30m", reply: true, call: mutesModule.tMute,
			method: "restrictChatMember", idKey: "user_id", wantID: user, golden: goldenModerationTmute},
		{name: "unban", text: "/unban 42", call: bansModule.unban,
			method: "unbanChatMember", idKey: "user_id", wantID: user, golden: goldenModerationUnban},
		{name: "unban_channel", text: "/unban -1001234567890", reply: true, channel: true, call: bansModule.unban,
			method: "unbanChatSenderChat", idKey: "sender_chat_id", wantID: channel, golden: goldenModerationUnbanChannel},
		{name: "unmute", text: "/unmute 42", call: mutesModule.unmute,
			method: "restrictChatMember", idKey: "user_id", wantID: user, golden: goldenModerationUnmute},
		{name: "unmute_lockdown", text: "/unmute 42", call: mutesModule.unmute, lockdown: true,
			method: "restrictChatMember", idKey: "user_id", wantID: user, golden: goldenModerationUnmuteLockdown},
		{name: "sban", text: "/sban 42", call: bansModule.sBan,
			method: "banChatMember", idKey: "user_id", wantID: user, silent: true},
		{name: "skick", text: "/skick", reply: true, call: bansModule.sKick,
			method: "unbanChatMember", idKey: "user_id", wantID: user, silent: true},
		{name: "smute", text: "/smute 42", call: mutesModule.sMute,
			method: "restrictChatMember", idKey: "user_id", wantID: user, silent: true},
	}
}

// moderationGoldenActiveLockdown makes every lockdown read of an unmute see an active lockdown whose
// stored permissions grant messages, until the test ends.
func moderationGoldenActiveLockdown(t *testing.T, chatID int64) {
	t.Helper()
	row := &models.ChatLockdown{
		ChatID:         chatID,
		State:          models.LockdownStateActive,
		PrePermissions: `{"can_send_messages":true,"can_send_photos":true,"can_send_videos":true,"can_send_polls":true}`,
	}
	mreqSwap(t, &lockdownSnapshotLookup, func(int64) (*models.ChatLockdown, error) { return row, nil })
}

func TestLegacyModerationRequests(t *testing.T) {
	withModerationGoldenLocales(t)
	for _, tc := range moderationGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newModerationGoldenSite(t)
			if tc.lockdown {
				moderationGoldenActiveLockdown(t, s.chat.Id)
			}

			if err := tc.call(s.bot, s.commandCtx(tc.text, tc.reply, tc.channel)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.text, err)
			}

			punish := s.client.callsFor(tc.method)
			if len(punish) != 1 {
				t.Fatalf("%s calls = %d, want 1", tc.method, len(punish))
			}
			if got := fmt.Sprint(punish[0].Params[tc.idKey]); got != strconv.FormatInt(tc.wantID, 10) {
				t.Fatalf("%s %s = %s, want %d", tc.method, tc.idKey, got, tc.wantID)
			}
			if tc.deletes != 0 {
				deleted := false
				for _, call := range s.client.callsFor("deleteMessage") {
					if fmt.Sprint(call.Params["message_id"]) == strconv.FormatInt(tc.deletes, 10) {
						deleted = true
					}
				}
				if !deleted {
					t.Fatalf("deleteMessage of message %d not sent", tc.deletes)
				}
			}

			sends := s.client.callsFor("sendMessage")
			if tc.silent {
				if len(sends) != 0 {
					t.Fatalf("sendMessage calls = %d, want none from a silent command", len(sends))
				}
				s.wantNoRows(s.chat.Id)
				return
			}
			if len(sends) != 1 {
				t.Fatalf("sendMessage calls = %d, want the one reply", len(sends))
			}
			got := strings.ReplaceAll(legacyRequestJSON(t, sends[0]), strconv.FormatInt(s.chat.Id, 10), "<chat>")
			if got != tc.golden {
				t.Fatalf("sendMessage request changed\n got %q\nwant %q", got, tc.golden)
			}
			s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, 600*time.Second)
		})
	}
}
