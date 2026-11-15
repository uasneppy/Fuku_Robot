//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 19 block keep the blacklist goldens below stable, one per case.
const blacklistGoldenChatBase int64 = -1009919001100

// blacklistGoldenNoRestrictUser is an administrator of the case's chat that lacks
// can_restrict_members; blacklistGoldenBotFlags below changes the bot's own right.
const blacklistGoldenNoRestrictUser int64 = 4101

// blacklistGoldenClient answers the administrator list and member lookups with one
// administrator without can_restrict_members and, on request, a bot without it.
type blacklistGoldenClient struct {
	*moduleBotClient
	noRestrictAdmin bool
	botNoRestrict   bool
}

func blacklistGoldenAdmin(id int64, name string, bot, restrict bool) string {
	return fmt.Sprintf(`{"status":"administrator","user":{"id":%d,"is_bot":%t,"first_name":%q},"can_pin_messages":true,"can_delete_messages":true,"can_restrict_members":%t,"can_promote_members":true,"can_change_info":true,"can_invite_users":true,"can_manage_chat":true}`, id, bot, name, restrict)
}

func (c blacklistGoldenClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	user := fmt.Sprint(params["user_id"])
	switch {
	case method == "getChatMember" && c.noRestrictAdmin && user == strconv.FormatInt(blacklistGoldenNoRestrictUser, 10):
		return json.RawMessage(blacklistGoldenAdmin(blacklistGoldenNoRestrictUser, "Helper", false, false)), nil
	case method == "getChatMember" && c.botNoRestrict && user == "999":
		return json.RawMessage(blacklistGoldenAdmin(999, "Fuku", true, false)), nil
	case method == "getChatAdministrators" && (c.noRestrictAdmin || c.botNoRestrict):
		list := []string{blacklistGoldenAdmin(999, "Fuku", true, !c.botNoRestrict)}
		if c.noRestrictAdmin {
			list = append(list, blacklistGoldenAdmin(blacklistGoldenNoRestrictUser, "Helper", false, false))
		}
		return json.RawMessage("[" + strings.Join(list, ",") + "]"), nil
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// blacklistGoldenWhereHas reports whether the statement's WHERE clause binds every value.
func blacklistGoldenWhereHas(tx *gorm.DB, values ...any) bool {
	where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
	if !ok {
		return false
	}
	for _, want := range values {
		found := false
		for _, expr := range where.Exprs {
			if e, ok := expr.(clause.Expr); ok && slices.Contains(e.Vars, want) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// blacklistGoldenFail makes one kind of write of the case's blacklist rows fail until the
// test ends, through a real GORM callback matching only that table and chat (and word).
func blacklistGoldenFail(t *testing.T, kind string, chatID int64, word string) {
	t.Helper()
	name := fmt.Sprintf("test:fail_blacklist_%s_%d_%s", kind, chatID, word)
	fail := func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("database is down"))
	}
	var err error
	switch kind {
	case "create":
		err = db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
			row, ok := tx.Statement.Dest.(*models.BlacklistSettings)
			if ok && tx.Statement.Table == "blacklists" && row.ChatId == chatID && row.Word == word {
				fail(tx)
			}
		})
	case "delete":
		err = db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "blacklists" && blacklistGoldenWhereHas(tx, chatID, word) {
				fail(tx)
			}
		})
	default:
		err = db.DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "blacklists" && blacklistGoldenWhereHas(tx, chatID) {
				fail(tx)
			}
		})
	}
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		var rmErr error
		switch kind {
		case "create":
			rmErr = db.DB.Callback().Create().Remove(name)
		case "delete":
			rmErr = db.DB.Callback().Delete().Remove(name)
		default:
			rmErr = db.DB.Callback().Update().Remove(name)
		}
		if rmErr != nil {
			t.Errorf("remove callback: %v", rmErr)
		}
	})
}

// blacklistGoldenStored reads the chat's stored blacklist rows straight from the table,
// never through the cache, as a sorted "word=action" list, and checks every reason text.
func blacklistGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var rows []models.BlacklistSettings
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read blacklists: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Reason != "Blacklisted word: '%s'" {
			t.Errorf("row %q has reason %q", row.Word, row.Reason)
		}
		out = append(out, row.Word+"="+row.Action)
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

// blacklistGoldenCleanup deletes the chat's rows and cache entries before the case and again
// when it ends.
func blacklistGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.BlacklistSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("blacklist", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyBlacklistRequests records what /addblacklist, /rmblacklist, /blacklistaction and
// /blacklists sent and stored before the extraction into the apply functions. It is committed
// alone, before any production change, and is never re-recorded or weakened. It holds no
// single-code-point word and no add after a non-warn action: those two behaviours are
// owner-confirmation changes of this phase with their own tests.
func TestLegacyBlacklistRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	helper := gotgbot.User{Id: blacklistGoldenNoRestrictUser, FirstName: "Helper"}

	type seedRow struct{ word, action string }
	cases := []struct {
		name          string
		text          string
		run           func(*gotgbot.Bot, *ext.Context) error
		from          gotgbot.User
		seed          []seedRow
		notAdminBot   bool
		noRestrict    bool
		botNoRestrict bool
		failKind      string
		failWord      string
		want          string
	}{
		{name: "add_bot_not_admin", text: "/addblacklist spam", run: blacklistsModule.addBlacklist, from: admin, notAdminBot: true},
		{name: "add_not_admin", text: "/addblacklist spam", run: blacklistsModule.addBlacklist, from: member},
		{name: "add_no_restrict_right", text: "/addblacklist spam", run: blacklistsModule.addBlacklist, from: helper, noRestrict: true},
		{name: "add_bot_cannot_restrict", text: "/addblacklist spam", run: blacklistsModule.addBlacklist, from: admin, botNoRestrict: true},
		{name: "add_no_argument", text: "/addblacklist", run: blacklistsModule.addBlacklist, from: admin},
		{name: "add_word_too_long", text: "/addblacklist " + strings.Repeat("x", 101), run: blacklistsModule.addBlacklist, from: admin},
		{name: "add_one_mixed_case", text: "/addblacklist SpAm", run: blacklistsModule.addBlacklist, from: admin, want: "spam=warn"},
		{name: "add_several_with_repeat", text: "/blocklist spam scam spam", run: blacklistsModule.addBlacklist, from: admin, want: "scam=warn,spam=warn"},
		{name: "add_already_present", text: "/addblacklist spam", run: blacklistsModule.addBlacklist, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "add_one_of_two_fails", text: "/addblacklist spam scam", run: blacklistsModule.addBlacklist, from: admin, failKind: "create", failWord: "scam", want: "spam=warn"},
		{name: "remove_no_argument", text: "/rmblacklist", run: blacklistsModule.removeBlacklist, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "remove_bot_cannot_restrict", text: "/rmblacklist spam", run: blacklistsModule.removeBlacklist, from: admin, botNoRestrict: true, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "remove_absent_word", text: "/rmblacklist ghost", run: blacklistsModule.removeBlacklist, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "remove_one", text: "/rmblacklist spam", run: blacklistsModule.removeBlacklist, from: admin, seed: []seedRow{{"spam", "warn"}, {"eggs", "warn"}}, want: "eggs=warn"},
		{name: "remove_two", text: "/unblacklist spam EGGS", run: blacklistsModule.removeBlacklist, from: admin, seed: []seedRow{{"spam", "warn"}, {"eggs", "warn"}, {"keep", "warn"}}, want: "keep=warn"},
		{name: "remove_delete_fails", text: "/rmblacklist spam", run: blacklistsModule.removeBlacklist, from: admin, failKind: "delete", failWord: "spam", seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "action_current_empty", text: "/blacklistaction", run: blacklistsModule.setBlacklistAction, from: admin},
		{name: "action_current_filled", text: "/blacklistaction", run: blacklistsModule.setBlacklistAction, from: admin, seed: []seedRow{{"spam", "kick"}}, want: "spam=kick"},
		{name: "action_no_restrict_right", text: "/blacklistaction ban", run: blacklistsModule.setBlacklistAction, from: helper, noRestrict: true, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "action_ban", text: "/blacklistaction ban", run: blacklistsModule.setBlacklistAction, from: admin, seed: []seedRow{{"spam", "warn"}, {"eggs", "mute"}}, want: "eggs=ban,spam=ban"},
		{name: "action_none_upper_case", text: "/blaction NONE", run: blacklistsModule.setBlacklistAction, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=none"},
		{name: "action_unknown_value", text: "/blacklistaction bogus", run: blacklistsModule.setBlacklistAction, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "action_two_arguments", text: "/blacklistaction ban mute", run: blacklistsModule.setBlacklistAction, from: admin, seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "action_ban_on_empty_list", text: "/blacklistaction ban", run: blacklistsModule.setBlacklistAction, from: admin},
		{name: "action_update_fails", text: "/blacklistaction ban", run: blacklistsModule.setBlacklistAction, from: admin, failKind: "update", seed: []seedRow{{"spam", "warn"}}, want: "spam=warn"},
		{name: "list_empty", text: "/blacklists", run: blacklistsModule.listBlacklists, from: admin},
		{name: "list_filled_sorted_escaped", text: "/blacklists", run: blacklistsModule.listBlacklists, from: admin, seed: []seedRow{{"zeta", "warn"}, {"a<b>&c", "warn"}, {"alpha", "warn"}}, want: "a<b>&c=warn,alpha=warn,zeta=warn"},
	}

	for i, tc := range cases {
		chatID := blacklistGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			blacklistGoldenCleanup(t, chatID)
			for _, row := range tc.seed {
				entry := &models.BlacklistSettings{ChatId: chatID, Word: row.word, Action: row.action, Reason: "Blacklisted word: '%s'"}
				if err := db.DB.Create(entry).Error; err != nil {
					t.Fatalf("seed %q: %v", row.word, err)
				}
			}
			if tc.failKind != "" {
				blacklistGoldenFail(t, tc.failKind, chatID, tc.failWord)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			switch {
			case tc.notAdminBot:
				bot.BotClient = warnGoldenNotAdminBot{client}
			case tc.noRestrict || tc.botNoRestrict:
				bot.BotClient = blacklistGoldenClient{moduleBotClient: client, noRestrictAdmin: tc.noRestrict, botNoRestrict: tc.botNoRestrict}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Blacklist Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.text)
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("command error = %v, want EndGroups", err)
			}
			calls := client.callsFor("sendMessage")
			requests := make([]string, 0, len(calls))
			for _, call := range calls {
				requests = append(requests, strings.ReplaceAll(legacyRequestJSON(t, call), strconv.FormatInt(chatID, 10), "<chat>"))
			}
			got := strings.Join(requests, "\n")
			want, ok := blacklistGoldens[tc.name]
			if !ok {
				t.Logf("GOLDEN\t%s\t%q", tc.name, got)
			} else if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			if stored := blacklistGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored rows = %q, want %q", stored, tc.want)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 19 (one sendMessage request per line,
// <chat> standing for the case's chat ID).
var blacklistGoldens = map[string]string{
	"add_bot_not_admin":          "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}",
	"add_not_admin":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}",
	"add_no_restrict_right":      "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У вас немає прав обмежувати користувачів у цій групі!\"}",
	"add_bot_cannot_restrict":    "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не можу тут обмежувати людей! Переконайтеся, що я адмін і маю право обмежувати інших учасників.\"}",
	"add_no_argument":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть слово, яке потрібно додати до чорного списку!\"}",
	"add_word_too_long":          "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова задовгі (макс. 100 символів): xxxxxxxxxxxxxxxxxxxx...\"}",
	"add_one_mixed_case":         "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова додано до чорного списку:\\n - \\u003ccode\\u003espam\\u003c/code\\u003e\\n\\n\"}",
	"add_several_with_repeat":    "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова вже в чорному списку:\\n - spam\\n\\nЦі слова додано до чорного списку:\\n - \\u003ccode\\u003espam\\u003c/code\\u003e\\n - \\u003ccode\\u003escam\\u003c/code\\u003e\\n\\n\"}",
	"add_already_present":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова вже в чорному списку:\\n - spam\\n\\n\"}",
	"add_one_of_two_fails":       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова додано до чорного списку:\\n - \\u003ccode\\u003espam\\u003c/code\\u003e\\n\\nНе вдалося зберегти налаштування. Спробуйте пізніше.\"}",
	"remove_no_argument":         "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть слово, яке потрібно вилучити з чорного списку!\"}",
	"remove_bot_cannot_restrict": "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не можу тут обмежувати людей! Переконайтеся, що я адмін і маю право обмежувати інших учасників.\"}",
	"remove_absent_word":         "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Жодного з указаних слів немає в чорному списку, тож вилучати нічого!\"}",
	"remove_one":                 "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"'spam' успішно вилучено з чорного списку!\"}",
	"remove_two":                 "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"'spam, eggs' успішно вилучено з чорного списку!\"}",
	"remove_delete_fails":        "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Жодного з указаних слів немає в чорному списку, тож вилучати нічого!\"}",
	"action_current_empty":       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Поточний режим чорного списку для цього чату: warn\"}",
	"action_current_filled":      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Поточний режим чорного списку для цього чату: kick\"}",
	"action_no_restrict_right":   "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У вас немає прав обмежувати користувачів у цій групі!\"}",
	"action_ban":                 "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим чорного списку змінено на: *ban*\"}",
	"action_none_upper_case":     "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим чорного списку змінено на: *none*\"}",
	"action_unknown_value":       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Виберіть один із варіантів: \\u003cmute/kick/ban/warn/none\\u003e\"}",
	"action_two_arguments":       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Виберіть один із варіантів: \\u003cmute/kick/ban/warn/none\\u003e\"}",
	"action_ban_on_empty_list":   "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим чорного списку змінено на: *ban*\"}",
	"action_update_fails":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося оновити дію чорного списку. Спробуйте ще раз.\"}",
	"list_empty":                 "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У цьому чаті немає слів у чорному списку.\"}",
	"list_filled_sorted_escaped": "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ці слова в чорному списку цього чату:\\n - \\u003ccode\\u003ea\\u0026lt;b\\u0026gt;\\u0026amp;c\\u003c/code\\u003e\\n - \\u003ccode\\u003ealpha\\u003c/code\\u003e\\n - \\u003ccode\\u003ezeta\\u003c/code\\u003e\"}",
}
