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

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
	"github.com/uasneppy/fuku_robot/fuku/db/warns"
)

// botSite is one chat whose sends are answered by a scripted client: every message the bot sends
// there is answered with the chat's own ID and type and the fixed clock's date, so the helper reads
// what Telegram would say. The clock is fixed, so no queued row is in 1970.
type botSite struct {
	t      *testing.T
	client *moduleBotClient
	bot    *gotgbot.Bot
	chat   gotgbot.Chat
	now    time.Time
}

// newBotSite returns a site for a fresh chat of the given type. Its rules, queue rows, greetings and
// cache entries are removed when the test ends.
func newBotSite(t *testing.T, chatType string) *botSite {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	mreqSwap(t, &deletionNow, func() time.Time { return now })
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: chatType, Title: "Bot Site Chat"}
	if chatType == "private" {
		chat = gotgbot.Chat{Id: uniqueLinkOwnerID(), Type: "private", FirstName: "Member"}
	}
	deletionCleanup(t, chat.Id)
	legacyCleanup(t, chat.Id)
	site := &botSite{t: t, chat: chat, now: now}
	site.client = rulesTestClient()
	site.answerAs(chat)
	site.bot = newModuleTestBot(site.client)
	return site
}

// answerAs makes every send answer carry chat and the fixed clock's date.
func (s *botSite) answerAs(chat gotgbot.Chat) {
	chatJSON := fmt.Sprintf(`"chat":{"id":%d,"type":%q,"title":"Bot Site Chat"}`, chat.Id, chat.Type)
	date := s.now.Unix()
	for method, id := range map[string]int{
		"sendMessage": 9001, "sendPhoto": 9003, "sendVideo": 9004, "sendAnimation": 9005, "sendDocument": 9002,
	} {
		s.client.responses[method] = json.RawMessage(fmt.Sprintf(`{"message_id":%d,"date":%d,%s}`, id, date, chatJSON))
	}
}

func (s *botSite) setRule(chatID int64, kind string, delay int) {
	s.t.Helper()
	if err := deletion.SetRule(s.t.Context(), chatID, kind, delay); err != nil {
		s.t.Fatalf("SetRule(%d, %s, %d) error = %v", chatID, kind, delay, err)
	}
}

// rows returns every queue row of the chat.
func (s *botSite) rows(chatID int64) []models.ScheduledDeletion {
	s.t.Helper()
	var out []models.ScheduledDeletion
	if err := db.DB.Where("chat_id = ?", chatID).Order("message_id").Find(&out).Error; err != nil {
		s.t.Fatalf("read the queue of chat %d: %v", chatID, err)
	}
	return out
}

// wantRow fails unless the chat's queue holds exactly one row of the kind, for messageID, sent now and
// due delay later.
func (s *botSite) wantRow(chatID, messageID int64, kind string, delay time.Duration) {
	s.t.Helper()
	rows := s.rows(chatID)
	if len(rows) != 1 {
		s.t.Fatalf("queue rows of chat %d = %+v, want exactly one", chatID, rows)
	}
	row := rows[0]
	if row.MessageID != messageID || row.Kind != kind {
		s.t.Fatalf("queue row = message %d kind %s, want message %d kind %s", row.MessageID, row.Kind, messageID, kind)
	}
	if !row.SentAt.Equal(s.now) || !row.DeleteAt.Equal(s.now.Add(delay)) {
		s.t.Fatalf("queue row sent_at %s delete_at %s, want %s and %s", row.SentAt, row.DeleteAt, s.now, s.now.Add(delay))
	}
}

// wantNoRows fails when the chat has any queue row.
func (s *botSite) wantNoRows(chatID int64) {
	s.t.Helper()
	if rows := s.rows(chatID); len(rows) != 0 {
		s.t.Fatalf("queue rows of chat %d = %+v, want none", chatID, rows)
	}
}

// runRules runs /rules from the member in the site's chat.
func (s *botSite) runRules() {
	s.t.Helper()
	ctx := newModuleMessageContext(s.bot, s.chat, gotgbot.User{Id: 42, FirstName: "Member"}, "/rules")
	ctx.EffectiveMessage.Date = s.now.Unix()
	if err := rulesModule.sendRules(s.bot, ctx); err != ext.EndGroups {
		s.t.Fatalf("sendRules() error = %v, want EndGroups", err)
	}
}

func TestBotRulesScheduled(t *testing.T) {
	withRealLocales(t)
	site := []models.Button{{Name: "Site", Url: "https://example.com"}}

	cases := []struct {
		name      string
		fileID    string
		mediaType int
		buttons   []models.Button
		private   bool
		kindOn    bool
		wantID    int64 // the message the row names; 0 for no row
	}{
		{name: "text only", kindOn: true, wantID: 9001},
		{name: "with buttons", buttons: site, kindOn: true, wantID: 9001},
		{name: "with media", fileID: "rules-file", mediaType: db.PHOTO, buttons: site, kindOn: true, wantID: 9003},
		{name: "click to read with private rules on", private: true, kindOn: true, wantID: 9001},
		{name: "kind off", kindOn: false, wantID: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			rulesSeed(t, s.chat.Id, "Be kind", tc.fileID, tc.mediaType, tc.buttons)
			if tc.private {
				mustLegacy(t, rules.SetPrivateRules(s.chat.Id, true))
			}
			if tc.kindOn {
				s.setRule(s.chat.Id, deletion.KindBotRules, 120)
			}

			s.runRules()

			if tc.wantID == 0 {
				s.wantNoRows(s.chat.Id)
				return
			}
			s.wantRow(s.chat.Id, tc.wantID, deletion.KindBotRules, 120*time.Second)
			if tc.private {
				if got := len(s.client.callsFor("sendMessage")); got != 1 {
					t.Fatalf("sendMessage calls = %d, want only the click-to-read message", got)
				}
			}
		})
	}
}

func TestBotRulesPrivateNeverScheduled(t *testing.T) {
	withRealLocales(t)

	t.Run("the deep link's private message", func(t *testing.T) {
		group := newBotSite(t, "supergroup")
		rulesSeed(t, group.chat.Id, "Be kind", "", 0, nil)
		group.setRule(group.chat.Id, deletion.KindBotRules, 120)
		private := newBotSite(t, "private")
		private.setRule(private.chat.Id, deletion.KindBotRules, 120)
		private.client.responses["getChat"] = json.RawMessage(fmt.Sprintf(
			`{"id":%d,"type":"supergroup","title":"Bot Site Chat"}`, group.chat.Id))

		user := gotgbot.User{Id: private.chat.Id, FirstName: "Member"}
		arg := "rules_" + strconv.FormatInt(group.chat.Id, 10)
		ctx := newModuleMessageContext(private.bot, private.chat, user, "/start "+arg)
		ctx.EffectiveMessage.Date = private.now.Unix()
		if err := rulesDeepLinkHandler(private.bot, ctx, &user, arg); err != ext.EndGroups {
			t.Fatalf("rulesDeepLinkHandler() error = %v, want EndGroups", err)
		}
		if got := len(private.client.callsFor("sendMessage")); got != 1 {
			t.Fatalf("sendMessage calls = %d, want the one private rules message", got)
		}
		private.wantNoRows(private.chat.Id)
		group.wantNoRows(group.chat.Id)
	})

	t.Run("/rules in a private chat connected to a group", func(t *testing.T) {
		group := newBotSite(t, "supergroup")
		rulesSeed(t, group.chat.Id, "Be kind", "", 0, nil)
		group.setRule(group.chat.Id, deletion.KindBotRules, 120)
		private := newBotSite(t, "private")
		private.setRule(private.chat.Id, deletion.KindBotRules, 120)
		private.client.responses["getChat"] = json.RawMessage(fmt.Sprintf(
			`{"id":%d,"type":"supergroup","title":"Bot Site Chat"}`, group.chat.Id))
		if err := connections.ConnectId(private.chat.Id, group.chat.Id); err != nil {
			t.Fatalf("ConnectId error = %v", err)
		}
		t.Cleanup(func() {
			db.DB.Where("user_id = ?", private.chat.Id).Delete(&models.ConnectionSettings{})
			db.DB.Where("user_id = ?", private.chat.Id).Delete(&models.User{})
			dbcache.DeleteCache(dbcache.CacheKey("connection", private.chat.Id))
			dbcache.DeleteCache(dbcache.CacheKey("user", private.chat.Id))
			dbcache.ResetLocalForTest()
		})

		ctx := newModuleMessageContext(private.bot, private.chat, gotgbot.User{Id: private.chat.Id, FirstName: "Member"}, "/rules")
		ctx.EffectiveMessage.Date = private.now.Unix()
		if err := rulesModule.sendRules(private.bot, ctx); err != ext.EndGroups {
			t.Fatalf("sendRules() error = %v, want EndGroups", err)
		}
		calls := private.client.callsFor("sendMessage")
		if len(calls) != 1 {
			t.Fatalf("sendMessage calls = %d, want the one rules message", len(calls))
		}
		if text, _ := calls[0].Params["text"].(string); !strings.Contains(text, "Be kind") {
			t.Fatalf("the message sent = %q, want the group's rules (the connected path ran)", text)
		}
		private.wantNoRows(private.chat.Id)
		group.wantNoRows(group.chat.Id)
	})

	t.Run("the settings editor's Full preview", func(t *testing.T) {
		env, owner := settingsEditRulesPreviewEnv(t)
		t.Cleanup(func() {
			db.DB.Where("chat_id IN ?", []int64{env.chat.Id, owner.Id}).Delete(&models.ScheduledDeletion{})
			db.DB.Where("chat_id IN ?", []int64{env.chat.Id}).Delete(&models.DeleteRule{})
		})
		settingsEditRulesSeed(t, env.chat.Id, "Be kind", "", 0, nil, false)
		if err := deletion.SetRule(t.Context(), env.chat.Id, deletion.KindBotRules, 120); err != nil {
			t.Fatalf("SetRule error = %v", err)
		}

		settingsEditRulesFullPreview(t, env, owner)

		var rows []models.ScheduledDeletion
		if err := db.DB.Where("chat_id IN ?", []int64{env.chat.Id, owner.Id}).Find(&rows).Error; err != nil {
			t.Fatalf("read the queue: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("queue rows after a preview = %+v, want none", rows)
		}
	})
}

// botSiteAdmin is the administrator every command of the site tests comes from (the fake's creator),
// and botSiteTarget the member they act on.
var (
	botSiteAdmin  = gotgbot.User{Id: 777000, FirstName: "Telegram"}
	botSiteTarget = gotgbot.User{Id: 42, FirstName: "Member"}
)

// commandCtx is a command message of the admin in the site's chat, dated by the fixed clock. With
// reply it answers a message of the target, and with channel the replied message is a channel post.
func (s *botSite) commandCtx(text string, reply, channel bool) *ext.Context {
	ctx := newModuleMessageContext(s.bot, s.chat, botSiteAdmin, text)
	ctx.EffectiveMessage.Date = s.now.Unix()
	if reply {
		replied := &gotgbot.Message{MessageId: 303, Date: 1, Chat: s.chat, From: &botSiteTarget, Text: "message being moderated"}
		if channel {
			replied = &gotgbot.Message{
				MessageId: 304, Date: 1, Chat: s.chat, Text: "channel post",
				SenderChat: &gotgbot.Chat{Id: -1001234567890, Type: "channel", Title: "Spam Channel"},
			}
		}
		ctx.EffectiveMessage.ReplyToMessage = replied
	}
	return ctx
}

// botSiteCommand is one command of the punishment family: the handler, the message it is sent as and,
// when the command re-enters after the anonymous admin's proof, the name it is registered under.
type botSiteCommand struct {
	name    string
	text    string
	reply   bool
	channel bool
	anon    string
	call    func(b *gotgbot.Bot, ctx *ext.Context) error
}

func botSiteCommands() []botSiteCommand {
	return []botSiteCommand{
		{name: "ban", text: "/ban spam", reply: true, anon: "ban", call: bansModule.ban},
		{name: "ban of a channel sender", text: "/ban -1001234567890", reply: true, channel: true, anon: "ban", call: bansModule.ban},
		{name: "dban", text: "/dban bad post", reply: true, anon: "dban", call: bansModule.dBan},
		{name: "tban", text: "/tban 1h temp reason", reply: true, anon: "tban", call: bansModule.tBan},
		{name: "kick", text: "/kick", reply: true, call: bansModule.kick},
		{name: "dkick", text: "/dkick bad post", reply: true, call: bansModule.dkick},
		{name: "unban", text: "/unban 42", anon: "unban", call: bansModule.unban},
		{name: "mute", text: "/mute", reply: true, anon: "mute", call: mutesModule.mute},
		{name: "dmute", text: "/dmute bad post", reply: true, anon: "dmute", call: mutesModule.dMute},
		{name: "tmute", text: "/tmute 1h", reply: true, anon: "tmute", call: mutesModule.tMute},
		{name: "unmute", text: "/unmute 42", anon: "unmute", call: mutesModule.unmute},
		{name: "warn", text: "/warn too noisy", reply: true, anon: "warn", call: warnsModule.warnUser},
		{name: "dwarn", text: "/dwarn too noisy", reply: true, anon: "dwarn", call: warnsModule.dWarnUser},
		{name: "swarn", text: "/swarn too noisy", reply: true, anon: "swarn", call: warnsModule.sWarnUser},
	}
}

// botSiteWarnsCleanup removes the warn rows a command of the site tests wrote.
func botSiteWarnsCleanup(t *testing.T, chatID int64) {
	t.Helper()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Warns{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{})
	})
}

func TestBotManualPunishmentReplies(t *testing.T) {
	const delay = 600

	for _, cmd := range botSiteCommands() {
		t.Run(cmd.name, func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, deletion.KindBotManual, delay)

			if err := cmd.call(s.bot, s.commandCtx(cmd.text, cmd.reply, cmd.channel)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", cmd.name, err)
			}
			s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, delay*time.Second)
		})

		t.Run(cmd.name+" with the kind off", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, deletion.KindBotAuto, delay) // another kind's rule must not matter

			if err := cmd.call(s.bot, s.commandCtx(cmd.text, cmd.reply, cmd.channel)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", cmd.name, err)
			}
			if got := len(s.client.callsFor("sendMessage")); got != 1 {
				t.Fatalf("sendMessage calls = %d, want the one reply", got)
			}
			s.wantNoRows(s.chat.Id)
		})

		if cmd.anon == "" {
			continue
		}
		// The anonymous admin's tap re-enters the same command with the tapper as sender, so the hook
		// needs no special case: the registered re-entry handler leaves the same row.
		t.Run(cmd.name+" after an anonymous admin's proof", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, deletion.KindBotManual, delay)
			dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
			LoadBans(dispatcher)
			LoadMutes(dispatcher)
			LoadWarns(dispatcher)

			if err := HandleAnonymousAdmin(s.bot, s.commandCtx(cmd.text, cmd.reply, cmd.channel), cmd.anon); err != ext.EndGroups {
				t.Fatalf("anonymous re-entry of %s error = %v, want EndGroups", cmd.anon, err)
			}
			s.wantRow(s.chat.Id, 9001, deletion.KindBotManual, delay*time.Second)
		})
	}
}

func TestBotManualRepliesNotCovered(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		reply     bool
		call      func(b *gotgbot.Bot, ctx *ext.Context) error
		wantSends int // the messages the command sends, so a case cannot pass by sending nothing
	}{
		{name: "ban without a target", text: "/ban", call: bansModule.ban, wantSends: 1},
		{name: "ban of a protected admin", text: "/ban 777000", call: bansModule.ban, wantSends: 1},
		{name: "ban of the bot itself", text: "/ban 999", call: bansModule.ban, wantSends: 1},
		{name: "tban with a bad duration", text: "/tban 99x", reply: true, call: bansModule.tBan, wantSends: 1},
		{name: "kick without a target", text: "/kick", call: bansModule.kick, wantSends: 1},
		{name: "unban without a target", text: "/unban", call: bansModule.unban, wantSends: 1},
		// IN-01: the "reply to a message" hint for a channel target is a usage hint, not a success reply.
		{name: "ban of a channel without a reply", text: "/ban -1001234567890", call: bansModule.ban, wantSends: 1},
		{name: "unban of a channel without a reply", text: "/unban -1001234567890", call: bansModule.unban, wantSends: 1},
		{name: "mute without a target", text: "/mute", call: mutesModule.mute, wantSends: 1},
		{name: "tmute with a bad duration", text: "/tmute 99x", reply: true, call: mutesModule.tMute, wantSends: 1},
		{name: "unmute without a target", text: "/unmute", call: mutesModule.unmute, wantSends: 1},
		{name: "warn without a target", text: "/warn", call: warnsModule.warnUser, wantSends: 1},
		{name: "warn list", text: "/warns", reply: true, call: warnsModule.warns, wantSends: 1},
		{name: "warn count", text: "/warnings", call: warnsModule.warnings, wantSends: 1},
		{name: "resetwarns", text: "/resetwarns", reply: true, call: warnsModule.resetWarns, wantSends: 1},
		{name: "rmwarn", text: "/rmwarn", reply: true, call: warnsModule.removeWarn, wantSends: 1},
		{name: "restrict question card", text: "/restrict 42", call: bansModule.restrict, wantSends: 1},
		{name: "unrestrict question card", text: "/unrestrict 42", call: bansModule.unrestrict, wantSends: 1},
		{name: "sban", text: "/sban spam", reply: true, call: bansModule.sBan, wantSends: 0},
		{name: "skick", text: "/skick", reply: true, call: bansModule.sKick, wantSends: 0},
		{name: "smute", text: "/smute", reply: true, call: mutesModule.sMute, wantSends: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			for _, kind := range []string{deletion.KindBotManual, deletion.KindBotAuto} {
				s.setRule(s.chat.Id, kind, 600)
			}

			if err := tc.call(s.bot, s.commandCtx(tc.text, tc.reply, false)); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", tc.name, err)
			}
			if got := len(s.client.callsFor("sendMessage")); got != tc.wantSends {
				t.Fatalf("sendMessage calls = %d, want %d", got, tc.wantSends)
			}
			s.wantNoRows(s.chat.Id)
		})
	}

	// /kickme and /banme are the member's own request, not a moderator's punishment.
	for name, call := range map[string]func(b *gotgbot.Bot, ctx *ext.Context) error{
		"kickme": bansModule.kickme, "banme": bansModule.banme,
	} {
		t.Run(name, func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			s.setRule(s.chat.Id, deletion.KindBotManual, 600)
			ctx := newModuleMessageContext(s.bot, s.chat, botSiteTarget, "/"+name)
			ctx.EffectiveMessage.Date = s.now.Unix()
			if err := call(s.bot, ctx); err != ext.EndGroups {
				t.Fatalf("%s error = %v, want EndGroups", name, err)
			}
			if got := len(s.client.callsFor("sendMessage")); got != 1 {
				t.Fatalf("sendMessage calls = %d, want the one reply", got)
			}
			s.wantNoRows(s.chat.Id)
		})
	}
}

func TestWarnThisUserSource(t *testing.T) {
	sources := []struct {
		name   string
		source warnSource
		set    string // the kind whose rule is stored
		other  string // the other kind, never stored in the first case
		want   string
	}{
		{name: "manual", source: warnSourceManual, set: deletion.KindBotManual, other: deletion.KindBotAuto, want: deletion.KindBotManual},
		{name: "automatic", source: warnSourceAuto, set: deletion.KindBotAuto, other: deletion.KindBotManual, want: deletion.KindBotAuto},
	}
	for _, tc := range sources {
		t.Run(tc.name+" warn schedules its own kind", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, tc.set, 300)

			ctx := s.commandCtx("/warn too noisy", true, false)
			if err := warnsModule.warnThisUser(s.bot, ctx, botSiteTarget.Id, "too noisy", "warn", tc.source); err != ext.EndGroups {
				t.Fatalf("warnThisUser error = %v, want EndGroups", err)
			}
			s.wantRow(s.chat.Id, 9001, tc.want, 300*time.Second)
		})

		t.Run(tc.name+" warn ignores the other kind's rule", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, tc.other, 300)

			ctx := s.commandCtx("/warn too noisy", true, false)
			if err := warnsModule.warnThisUser(s.bot, ctx, botSiteTarget.Id, "too noisy", "warn", tc.source); err != ext.EndGroups {
				t.Fatalf("warnThisUser error = %v, want EndGroups", err)
			}
			if got := len(s.client.callsFor("sendMessage")); got != 1 {
				t.Fatalf("sendMessage calls = %d, want the one warning", got)
			}
			s.wantNoRows(s.chat.Id)
		})

		// The warn-limit text goes through the same send, so it is scheduled too.
		t.Run(tc.name+" warn at the limit is scheduled", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			botSiteWarnsCleanup(t, s.chat.Id)
			s.setRule(s.chat.Id, tc.set, 300)
			if err := warns.SetWarnLimit(s.chat.Id, 1); err != nil {
				t.Fatalf("SetWarnLimit() error = %v", err)
			}
			if err := warns.SetWarnMode(s.chat.Id, "mute"); err != nil {
				t.Fatalf("SetWarnMode() error = %v", err)
			}

			ctx := s.commandCtx("/warn too noisy", true, false)
			if err := warnsModule.warnThisUser(s.bot, ctx, botSiteTarget.Id, "too noisy", "warn", tc.source); err != ext.EndGroups {
				t.Fatalf("warnThisUser error = %v, want EndGroups", err)
			}
			if got := len(s.client.callsFor("restrictChatMember")); got != 1 {
				t.Fatalf("restrictChatMember calls = %d, want the limit action", got)
			}
			s.wantRow(s.chat.Id, 9001, tc.want, 300*time.Second)
		})
	}

	// The blacklist's warn action is the automatic source, driven through the real watcher.
	t.Run("the blacklist's warn is automatic", func(t *testing.T) {
		s := newBotSite(t, "supergroup")
		botSiteWarnsCleanup(t, s.chat.Id)
		t.Cleanup(func() {
			if err := blacklists.RemoveAllBlacklist(s.chat.Id); err != nil {
				t.Errorf("RemoveAllBlacklist error = %v", err)
			}
		})
		s.setRule(s.chat.Id, deletion.KindBotAuto, 300)
		s.setRule(s.chat.Id, deletion.KindBotManual, 900)
		if err := blacklists.AddBlacklist(s.chat.Id, "spam"); err != nil {
			t.Fatalf("AddBlacklist error = %v", err)
		}
		if err := blacklists.SetBlacklistAction(s.chat.Id, "warn"); err != nil {
			t.Fatalf("SetBlacklistAction error = %v", err)
		}

		ctx := newModuleMessageContext(s.bot, s.chat, botSiteTarget, "this has spam inside")
		ctx.EffectiveMessage.Date = s.now.Unix()
		if err := blacklistsModule.blacklistWatcher(s.bot, ctx); err != ext.EndGroups {
			t.Fatalf("blacklistWatcher error = %v, want EndGroups", err)
		}
		s.wantRow(s.chat.Id, 9001, deletion.KindBotAuto, 300*time.Second)
	})
}
