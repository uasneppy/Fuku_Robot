//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

// Fixed chat IDs of the Phase 19 filter block keep the goldens below stable, one per case.
const filterGoldenChatBase int64 = -1009919001200

// filterGoldenCardMarker stands for the random value the overwrite card carries in its callback data.
const filterGoldenCardMarker = "<card>"

// filterGoldenNoInfoUser is an administrator of the case's chat that lacks can_change_info.
const filterGoldenNoInfoUser int64 = 4201

var filterGoldenCardRe = regexp.MustCompile(`\\u0026t=([A-Za-z0-9_-]+)`)

// filterGoldenClient answers the member and administrator lookups with one administrator
// that lacks can_change_info.
type filterGoldenClient struct {
	*moduleBotClient
}

func filterGoldenAdmin(id int64, name string, bot, changeInfo bool) string {
	return fmt.Sprintf(`{"status":"administrator","user":{"id":%d,"is_bot":%t,"first_name":%q},"can_pin_messages":true,"can_delete_messages":true,"can_restrict_members":true,"can_promote_members":true,"can_change_info":%t,"can_invite_users":true,"can_manage_chat":true}`, id, bot, name, changeInfo)
}

func (c filterGoldenClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	user := fmt.Sprint(params["user_id"])
	switch {
	case method == "getChatMember" && user == strconv.FormatInt(filterGoldenNoInfoUser, 10):
		return json.RawMessage(filterGoldenAdmin(filterGoldenNoInfoUser, "Helper", false, false)), nil
	case method == "getChatAdministrators":
		list := []string{
			filterGoldenAdmin(999, "Fuku", true, true),
			filterGoldenAdmin(filterGoldenNoInfoUser, "Helper", false, false),
		}
		return json.RawMessage("[" + strings.Join(list, ",") + "]"), nil
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// filterGoldenFail makes one kind of write of the case's filter rows fail until the test ends,
// through a real GORM callback matching only that table and chat.
func filterGoldenFail(t *testing.T, kind string, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_filter_%s_%d", kind, chatID)
	var err error
	switch kind {
	case "create":
		err = db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
			row, ok := tx.Statement.Dest.(map[string]any)
			if ok && tx.Statement.Table == "filters" && row["chat_id"] == chatID {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
	default:
		err = db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "filters" && blacklistGoldenWhereHas(tx, chatID) {
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

// filterGoldenStored reads the chat's stored filter rows straight from the table, never through
// the cache, as a sorted "keyword|type|file|reply|buttons" list.
func filterGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var rows []models.ChatFilters
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read filters: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		buttons, err := json.Marshal(row.Buttons)
		if err != nil {
			t.Fatalf("encode buttons: %v", err)
		}
		out = append(out, fmt.Sprintf("%s|%d|%s|%s|%s", row.KeyWord, row.MsgType, row.FileID, row.FilterReply, buttons))
	}
	slices.Sort(out)
	return strings.Join(out, ";")
}

// filterGoldenCleanup deletes the chat's rows and cache entries before the case and again when it
// ends.
func filterGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatFilters{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("filter_list", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("filters_optimized", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyFilterRequests records what /filter, /stop and /filters sent and stored before the
// extraction into the apply functions. It is committed alone, before any production change, and is
// never re-recorded or weakened. It holds no single-code-point keyword and no case where the
// existing-keyword or count read fails: those are owner-confirmation changes of this phase with
// their own tests.
func TestLegacyFilterRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	helper := gotgbot.User{Id: filterGoldenNoInfoUser, FirstName: "Helper"}

	textReply := func(chat gotgbot.Chat) *gotgbot.Message {
		return &gotgbot.Message{MessageId: 55, Date: 1, Chat: chat, From: &member, Text: "reply body"}
	}
	type seedRow struct{ keyword, reply string }
	cases := []struct {
		name     string
		text     string
		run      func(*gotgbot.Bot, *ext.Context) error
		from     gotgbot.User
		reply    func(gotgbot.Chat) *gotgbot.Message
		seed     []seedRow
		seedMany int
		noInfo   bool
		noCache  bool
		failKind string
		card     bool
		rowGold  bool
		want     string
	}{
		{name: "add_not_admin", text: "/filter hello world", run: filtersModule.addFilter, from: member},
		{name: "add_no_change_info_right", text: "/filter hello world", run: filtersModule.addFilter, from: helper, noInfo: true},
		{name: "add_limit_reached", text: "/filter overflow nope", run: filtersModule.addFilter, from: admin, seedMany: 150},
		{name: "add_reply_without_keyword", text: "/filter", run: filtersModule.addFilter, from: admin, reply: textReply},
		{name: "add_no_reply_invalid", text: "/filter hello", run: filtersModule.addFilter, from: admin},
		{name: "add_reply_to_poll", text: "/filter quiz", run: filtersModule.addFilter, from: admin, reply: func(chat gotgbot.Chat) *gotgbot.Message {
			return &gotgbot.Message{MessageId: 56, Date: 1, Chat: chat, From: &member, Poll: &gotgbot.Poll{Id: "poll-1", Question: "Q"}}
		}},
		{name: "add_reply_text_too_long", text: "/filter big", run: filtersModule.addFilter, from: admin, reply: func(chat gotgbot.Chat) *gotgbot.Message {
			return &gotgbot.Message{MessageId: 57, Date: 1, Chat: chat, From: &member, Text: strings.Repeat("a", 4097)}
		}},
		{name: "add_keyword_too_long", text: "/filter " + strings.Repeat("x", 101) + " value", run: filtersModule.addFilter, from: admin},
		{name: "add_inline_mixed_case", text: "/filter Hello there friend", run: filtersModule.addFilter, from: admin, want: "hello|1||there friend|[]"},
		{name: "add_reply_to_text", text: "/filter note1", run: filtersModule.addFilter, from: admin, reply: textReply, want: "note1|1||reply body|[]"},
		{name: "add_reply_photo_caption_button", text: "/filter pic", run: filtersModule.addFilter, from: admin, rowGold: true, reply: func(chat gotgbot.Chat) *gotgbot.Message {
			return &gotgbot.Message{
				MessageId: 58, Date: 1, Chat: chat, From: &member,
				Photo:   []gotgbot.PhotoSize{{FileId: "photo-small", FileUniqueId: "u1"}, {FileId: "photo-big", FileUniqueId: "u2"}},
				Caption: "look [Site](buttonurl://https://example.com)",
			}
		}},
		{name: "add_alias", text: "/addfilter greet welcome aboard", run: filtersModule.addFilter, from: admin, want: "greet|1||welcome aboard|[]"},
		{name: "add_existing_overwrite_card", text: "/filter dupe new text", run: filtersModule.addFilter, from: admin, seed: []seedRow{{"dupe", "old"}}, card: true, want: "dupe|1||old|[]"},
		{name: "add_existing_without_cache", text: "/filter dupe new text", run: filtersModule.addFilter, from: admin, seed: []seedRow{{"dupe", "old"}}, noCache: true, want: "dupe|1||old|[]"},
		{name: "add_write_fails", text: "/filter price tag", run: filtersModule.addFilter, from: admin, failKind: "create"},
		{name: "remove_no_argument", text: "/stop", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"hello", "hi"}}, want: "hello|1||hi|[]"},
		{name: "remove_absent_keyword", text: "/stop ghost", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"hello", "hi"}}, want: "hello|1||hi|[]"},
		{name: "remove_original_case", text: "/stop Hello", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"hello", "hi"}, {"keep", "k"}}, want: "keep|1||k|[]"},
		{name: "remove_quoted_two_words", text: "/stop \"two words\"", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"two words", "hi"}}},
		{name: "remove_alias", text: "/rmfilter hello", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"hello", "hi"}}},
		{name: "remove_delete_fails", text: "/stop hello", run: filtersModule.rmFilter, from: admin, seed: []seedRow{{"hello", "hi"}}, failKind: "delete", want: "hello|1||hi|[]"},
		{name: "list_empty", text: "/filters", run: filtersModule.filtersList, from: admin},
		{name: "list_filled_escaped", text: "/filters", run: filtersModule.filtersList, from: admin, seed: []seedRow{{"a<b>&c", "hi"}}, want: "a<b>&c|1||hi|[]"},
	}

	for i, tc := range cases {
		chatID := filterGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			filterGoldenCleanup(t, chatID)
			for _, row := range tc.seed {
				entry := &models.ChatFilters{ChatId: chatID, KeyWord: row.keyword, FilterReply: row.reply, MsgType: db.TEXT}
				if err := db.DB.Create(entry).Error; err != nil {
					t.Fatalf("seed %q: %v", row.keyword, err)
				}
			}
			if tc.seedMany > 0 {
				batch := make([]models.ChatFilters, 0, tc.seedMany)
				for n := 0; n < tc.seedMany; n++ {
					batch = append(batch, models.ChatFilters{ChatId: chatID, KeyWord: "word" + strconv.Itoa(n), FilterReply: "r", MsgType: db.TEXT})
				}
				if err := db.DB.CreateInBatches(batch, 50).Error; err != nil {
					t.Fatalf("seed many: %v", err)
				}
			}
			if tc.failKind != "" {
				filterGoldenFail(t, tc.failKind, chatID)
			}
			if tc.noCache {
				withNilCacheMarshal(t)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.noInfo {
				bot.BotClient = filterGoldenClient{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Filter Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.text)
			if tc.reply != nil {
				ctx.EffectiveMessage.ReplyToMessage = tc.reply(chat)
			}
			if err := tc.run(bot, ctx); err != ext.EndGroups {
				t.Fatalf("command error = %v, want EndGroups", err)
			}
			calls := client.callsFor("sendMessage")
			requests := make([]string, 0, len(calls))
			for _, call := range calls {
				raw := strings.ReplaceAll(legacyRequestJSON(t, call), strconv.FormatInt(chatID, 10), "<chat>")
				if match := filterGoldenCardRe.FindStringSubmatch(raw); tc.card && match != nil {
					if _, err := getOverwriteCache[overwriteFilter](filterOverwriteCacheKey(match[1])); err != nil {
						t.Errorf("overwrite key of the card is not stored: %v", err)
					}
					raw = strings.ReplaceAll(raw, match[1], filterGoldenCardMarker)
				}
				requests = append(requests, raw)
			}
			if tc.card && !strings.Contains(strings.Join(requests, "\n"), filterGoldenCardMarker) {
				t.Errorf("no overwrite card was sent: %v", requests)
			}
			got := strings.Join(requests, "\n")
			want, ok := filterGoldens[tc.name]
			if !ok {
				t.Logf("GOLDEN\t%s\t%q", tc.name, got)
			} else if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			if tc.seedMany > 0 {
				var count int64
				if err := db.DB.Model(&models.ChatFilters{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil || count != int64(tc.seedMany) {
					t.Errorf("filter rows = %d (%v), want %d", count, err, tc.seedMany)
				}
				return
			}
			stored := filterGoldenStored(t, chatID)
			if tc.rowGold {
				filterGoldenRecordRow(t, tc.name, stored)
				return
			}
			if stored != tc.want {
				t.Errorf("stored rows = %q, want %q", stored, tc.want)
			}
		})
	}
}

// filterGoldenRecordRow checks the stored row of a case whose row is a recorded golden too.
func filterGoldenRecordRow(t *testing.T, name, stored string) {
	t.Helper()
	want, ok := filterGoldenRows[name]
	if !ok {
		t.Logf("ROW\t%s\t%q", name, stored)
		return
	}
	if stored != want {
		t.Errorf("stored rows = %q, want %q", stored, want)
	}
}

// Stored rows of the cases whose row is part of what the command does.
var filterGoldenRows = map[string]string{
	"add_reply_photo_caption_button": "pic|4|photo-big|look|[{\"name\":\"Site\",\"url\":\"https://example.com\",\"btn_sameline\":false}]",
}

// Goldens recorded on the code as it was before Phase 19 (one sendMessage request per line,
// <chat> standing for the case's chat ID).
var filterGoldens = map[string]string{
	"add_not_admin":                  "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}",
	"add_no_change_info_right":       "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У вас немає прав змінювати інформацію в цій групі!\"}",
	"add_limit_reached":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Перевищено ліміт фільтрів: у групі може бути щонайбільше 150 фільтрів!\\nЦе обмеження існує, бо бот працює безкоштовно, без пожертв від користувачів.\\n\"}",
	"add_reply_without_keyword":      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть ключове слово, на яке відповідати!\"}",
	"add_no_reply_invalid":           "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Недійсний фільтр!\"}",
	"add_reply_to_poll":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Потрібно додати до фільтра якийсь вміст!\"}",
	"add_reply_text_too_long":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Текст вашого повідомлення містить 4097 символів. Максимальна довжина тексту — 4096; скоротіть його. Зверніть увагу, що символи markdown можуть займати більше місця, ніж очікується.\"}",
	"add_keyword_too_long":           "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ключове слово фільтра надто довге! Дозволено не більше 100 символів.\"}",
	"add_inline_mixed_case":          "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Додано відповідь для слова фільтра \\u003ccode\\u003ehello\\u003c/code\\u003e\"}",
	"add_reply_to_text":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Додано відповідь для слова фільтра \\u003ccode\\u003enote1\\u003c/code\\u003e\"}",
	"add_reply_photo_caption_button": "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Додано відповідь для слова фільтра \\u003ccode\\u003epic\\u003c/code\\u003e\"}",
	"add_alias":                      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Додано відповідь для слова фільтра \\u003ccode\\u003egreet\\u003c/code\\u003e\"}",
	"add_existing_overwrite_card":    "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Так\",\"callback_data\":\"filters_overwrite|v1|a=yes\\u0026t=<card>\"},{\"text\":\"Ні\",\"callback_data\":\"filters_overwrite|v1|a=cancel\\u0026t=<card>\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Фільтр уже існує!\\nБажаєте перезаписати його?\"}",
	"add_existing_without_cache":     "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося створити підтвердження перезапису. Спробуйте ще раз.\"}",
	"add_write_fails":                "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}",
	"remove_no_argument":             "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть слово фільтра для видалення!\"}",
	"remove_absent_keyword":          "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Фільтра не існує!\"}",
	"remove_original_case":           "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Готово!\\nЯ більше не відповідатиму на \\u003ccode\\u003eHello\\u003c/code\\u003e\"}",
	"remove_quoted_two_words":        "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Готово!\\nЯ більше не відповідатиму на \\u003ccode\\u003etwo words\\u003c/code\\u003e\"}",
	"remove_alias":                   "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Готово!\\nЯ більше не відповідатиму на \\u003ccode\\u003ehello\\u003c/code\\u003e\"}",
	"remove_delete_fails":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}",
	"list_empty":                     "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У цьому чаті немає фільтрів!\"}",
	"list_filled_escaped":            "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Поточні фільтри в цьому чаті:\\n - \\u003ccode\\u003ea\\u0026lt;b\\u0026gt;\\u0026amp;c\\u003c/code\\u003e\"}",
}
