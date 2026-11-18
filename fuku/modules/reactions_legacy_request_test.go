//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the reactions block keep the goldens below stable, one per case.
const reactionGoldenChatBase int64 = -1009920001400

// reactionGoldenFail makes one kind of write of the case's reaction rows fail until the test ends,
// through a real GORM callback matching only that table and chat.
func reactionGoldenFail(t *testing.T, kind string, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_reaction_%s_%d", kind, chatID)
	var err error
	switch kind {
	case "create":
		err = db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
			row, ok := tx.Statement.Dest.(*models.Reactions)
			if ok && tx.Statement.Table == "reactions" && row.ChatID == chatID {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
	default:
		err = db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "reactions" && blacklistGoldenWhereHas(tx, chatID) {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
	}
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		var rmErr error
		if kind == "create" {
			rmErr = db.DB.Callback().Create().Remove(name)
		} else {
			rmErr = db.DB.Callback().Delete().Remove(name)
		}
		if rmErr != nil {
			t.Errorf("remove callback: %v", rmErr)
		}
	})
}

// reactionGoldenStored reads the chat's stored reaction rows straight from the table as a sorted
// "keyword|emoji" list.
func reactionGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var rows []models.Reactions
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read reactions: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Keyword+"|"+row.Emoji)
	}
	slices.Sort(out)
	return strings.Join(out, ";")
}

// reactionGoldenCleanup deletes the chat's rows and cache entries before the case and again when it ends.
func reactionGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Reactions{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("reactions", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyReactionRequests records what /addreaction, /removereaction, /reactions and
// /resetreactions sent and stored before the extraction into the apply functions. It is committed
// alone, before any production change, and is never re-recorded or weakened.
func TestLegacyReactionRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	helper := gotgbot.User{Id: filterGoldenNoInfoUser, FirstName: "Helper"}

	type seedRow struct{ keyword, emoji string }
	cases := []struct {
		name     string
		text     string
		run      func(*gotgbot.Bot, *ext.Context) error
		from     gotgbot.User
		seed     []seedRow
		noInfo   bool
		failKind string
		want     string
	}{
		{name: "add_not_admin", text: "/addreaction hello 👍", run: reactionsModule.addReaction, from: member},
		{name: "add_no_change_info_right", text: "/addreaction hello 👍", run: reactionsModule.addReaction, from: helper, noInfo: true},
		{name: "add_usage", text: "/addreaction hello", run: reactionsModule.addReaction, from: admin},
		{name: "add_invalid_emoji", text: "/addreaction hello x", run: reactionsModule.addReaction, from: admin},
		{name: "add_mixed_case", text: "/addreaction Hello 👍", run: reactionsModule.addReaction, from: admin, want: "hello|👍"},
		{name: "add_variation_selector", text: "/addreaction love ❤️", run: reactionsModule.addReaction, from: admin, want: "love|❤"},
		{name: "add_escaped", text: "/addreaction a<b>&c 🔥", run: reactionsModule.addReaction, from: admin, want: "a<b>&c|🔥"},
		{name: "add_existing_overwrites", text: "/addreaction hello 👍", run: reactionsModule.addReaction, from: admin, seed: []seedRow{{"hello", "👎"}}, want: "hello|👍"},
		{name: "add_write_fails", text: "/addreaction hello 👍", run: reactionsModule.addReaction, from: admin, failKind: "create"},
		{name: "remove_not_admin", text: "/removereaction hello", run: reactionsModule.removeReaction, from: member, seed: []seedRow{{"hello", "👍"}}, want: "hello|👍"},
		{name: "remove_usage", text: "/removereaction", run: reactionsModule.removeReaction, from: admin, seed: []seedRow{{"hello", "👍"}}, want: "hello|👍"},
		{name: "remove_absent", text: "/removereaction ghost", run: reactionsModule.removeReaction, from: admin, seed: []seedRow{{"hello", "👍"}}, want: "hello|👍"},
		{name: "remove_mixed_case", text: "/removereaction Hello", run: reactionsModule.removeReaction, from: admin, seed: []seedRow{{"hello", "👍"}, {"keep", "🔥"}}, want: "keep|🔥"},
		{name: "remove_delete_fails", text: "/removereaction hello", run: reactionsModule.removeReaction, from: admin, seed: []seedRow{{"hello", "👍"}}, failKind: "delete", want: "hello|👍"},
		{name: "list_empty", text: "/reactions", run: reactionsModule.listReactions, from: member},
		{name: "list_one_escaped", text: "/reactions", run: reactionsModule.listReactions, from: member, seed: []seedRow{{"a<b>", "👍"}}, want: "a<b>|👍"},
		{name: "reset_not_admin", text: "/resetreactions", run: reactionsModule.resetReactions, from: member, seed: []seedRow{{"hello", "👍"}}, want: "hello|👍"},
		{name: "reset_all", text: "/resetreactions", run: reactionsModule.resetReactions, from: admin, seed: []seedRow{{"hello", "👍"}, {"bye", "🔥"}}},
		{name: "reset_fails", text: "/resetreactions", run: reactionsModule.resetReactions, from: admin, seed: []seedRow{{"hello", "👍"}}, failKind: "delete", want: "hello|👍"},
	}

	for i, tc := range cases {
		chatID := reactionGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			reactionGoldenCleanup(t, chatID)
			for _, row := range tc.seed {
				if err := db.DB.Create(&models.Reactions{ChatID: chatID, Keyword: row.keyword, Emoji: row.emoji}).Error; err != nil {
					t.Fatalf("seed %q: %v", row.keyword, err)
				}
			}
			if tc.failKind != "" {
				reactionGoldenFail(t, tc.failKind, chatID)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.noInfo {
				bot.BotClient = filterGoldenClient{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Reaction Golden"}
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
			want, ok := reactionGoldens[tc.name]
			if !ok {
				t.Logf("GOLDEN\t%s\t%q", tc.name, got)
			} else if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			if stored := reactionGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored rows = %q, want %q", stored, tc.want)
			}
		})
	}
}

// Goldens recorded on the code as it was before the Reactions settings page (one sendMessage request
// per line, <chat> standing for the case's chat ID).
var reactionGoldens = map[string]string{
	"add_not_admin":            "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Доступ заборонено. У вас немає права змінювати дані цієї групи.\"}",
	"add_no_change_info_right": "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Доступ заборонено. У вас немає права змінювати дані цієї групи.\"}",
	"add_usage":                "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Використання: /addreaction \\u0026lt;keyword\\u0026gt; \\u0026lt;emoji\\u0026gt;\"}",
	"add_invalid_emoji":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть коректний емодзі.\"}",
	"add_mixed_case":           "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Реакцію внесено: \\u003ccode\\u003ehello\\u003c/code\\u003e -\\u003e 👍\"}",
	"add_variation_selector":   "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Реакцію внесено: \\u003ccode\\u003elove\\u003c/code\\u003e -\\u003e ❤\"}",
	"add_escaped":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Реакцію внесено: \\u003ccode\\u003ea\\u0026lt;b\\u0026gt;\\u0026amp;c\\u003c/code\\u003e -\\u003e 🔥\"}",
	"add_existing_overwrites":  "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Реакцію внесено: \\u003ccode\\u003ehello\\u003c/code\\u003e -\\u003e 👍\"}",
	"add_write_fails":          "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Збій: не вдалося зберегти реакцію. Спробуйте ще раз.\"}",
	"remove_not_admin":         "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Доступ заборонено. У вас немає права змінювати дані цієї групи.\"}",
	"remove_usage":             "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Використання: /removereaction \\u0026lt;keyword\\u0026gt;\"}",
	"remove_absent":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Для ключового слова \\u003ccode\\u003eghost\\u003c/code\\u003e реакції немає.\"}",
	"remove_mixed_case":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Реакцію для ключового слова \\u003ccode\\u003ehello\\u003c/code\\u003e прибрано.\"}",
	"remove_delete_fails":      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Збій: не вдалося прибрати реакцію. Спробуйте ще раз.\"}",
	"list_empty":               "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У цьому чаті реакцій не налаштовано.\"}",
	"list_one_escaped":         "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"\\u003cb\\u003eНалаштовані реакції:\\u003c/b\\u003e\\n• a\\u0026lt;b\\u0026gt; → 👍\\n\"}",
	"reset_not_admin":          "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Доступ заборонено. У вас немає права змінювати дані цієї групи.\"}",
	"reset_all":                "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Усі реакції цього чату скинуто.\"}",
	"reset_fails":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Збій: не вдалося прибрати реакцію. Спробуйте ще раз.\"}",
}
