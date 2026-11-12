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
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
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
