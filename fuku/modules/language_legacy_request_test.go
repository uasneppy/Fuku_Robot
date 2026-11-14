//go:build testtools

package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the language goldens below stable, one per case.
const languageGoldenChatBase int64 = -1009918001500

// languageGoldenCleanup deletes the chats rows and the users rows the case touched, their
// language, settings and user cache keys and the admin cache of the chats, before the case and
// again when it ends.
func languageGoldenCleanup(t *testing.T, chatIDs, userIDs []int64) {
	t.Helper()
	clean := func() {
		if len(chatIDs) > 0 {
			db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.Chat{})
		}
		if len(userIDs) > 0 {
			db.DB.Where("user_id IN ?", userIDs).Delete(&models.User{})
		}
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("chat_lang", id))
			dbcache.DeleteCache(dbcache.CacheKey("chat_settings", id))
			dbcache.DeleteCache(dbcache.CacheKey("chat", id))
			cache.InvalidateAdminCache(id)
		}
		for _, id := range userIDs {
			dbcache.DeleteCache(dbcache.CacheKey("user_lang", id))
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// languageGoldenFailUpdate makes every update of the table fail until the test ends, through a
// real GORM callback on the update chain the language write uses.
func languageGoldenFailUpdate(t *testing.T, table string) {
	t.Helper()
	name := fmt.Sprintf("test:fail_%s_update_%s", table, t.Name())
	err := db.DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("database is down"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Update().Remove(name); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})
}

// languageGoldenCalls is the outbound requests of the three methods a language command or
// button uses, in the order they were made, one per line, with the chat ID blanked.
func languageGoldenCalls(t *testing.T, client *moduleBotClient, chatID int64) string {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()
	var lines []string
	for _, call := range client.calls {
		switch call.Method {
		case "answerCallbackQuery", "editMessageText", "sendMessage":
		default:
			continue
		}
		raw, err := json.Marshal(call.Params)
		if err != nil {
			t.Fatalf("encode %s params: %v", call.Method, err)
		}
		lines = append(lines, call.Method+" "+strings.ReplaceAll(string(raw), strconv.FormatInt(chatID, 10), "<chat>"))
	}
	return strings.Join(lines, "\n")
}

// TestLegacyLanguageRequests records what the change_language callback and /lang sent and stored
// before the group branch moved behind applyGroupLanguage: the valid codes, a code stored again,
// a missing code, an unsupported code refused before the admin check, a non-admin, a failed
// save and a press with no message in a group, the private branch that writes the user's own
// language, and /lang's display. It is committed alone, before any production change, and is
// never re-recorded or weakened.
func TestLegacyLanguageRequests(t *testing.T) {
	withRealLocales(t)
	// /lang's keyboard has one cell per enabled language, and the enabled set is process state: a
	// test binary started without BOT_TOKEN has none and CI has the eight defaults. Pin the
	// defaults, so the golden reads the same everywhere.
	previousCodes := config.AppConfig.ValidLangCodes
	config.AppConfig.ValidLangCodes = []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}
	t.Cleanup(func() { config.AppConfig.ValidLangCodes = previousCodes })
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	const (
		kindGroupButton = iota
		kindGroupNoMessage
		kindPrivateButton
		kindGroupCommand
		kindPrivateCommand
	)
	cases := []struct {
		name string
		kind int
		// code is the l field; noCode sends data with no l at all.
		code   string
		noCode bool
		from   gotgbot.User
		// seed is the language stored before the press ("" stores nothing).
		seed       string
		failUpdate bool
		// wantStored is the language the chats row (group) or users row (private) holds after.
		wantStored string
	}{
		{name: "group_en", kind: kindGroupButton, code: "en", from: admin, seed: "uk", wantStored: "en"},
		{name: "group_es", kind: kindGroupButton, code: "es", from: admin, seed: "en", wantStored: "es"},
		{name: "group_hi", kind: kindGroupButton, code: "hi", from: admin, seed: "en", wantStored: "hi"},
		{name: "group_uk", kind: kindGroupButton, code: "uk", from: admin, seed: "en", wantStored: "uk"},
		{name: "group_id", kind: kindGroupButton, code: "id", from: admin, seed: "en", wantStored: "id"},
		{name: "group_same_code_again", kind: kindGroupButton, code: "es", from: admin, seed: "es", wantStored: "es"},
		{name: "group_first_write_no_row", kind: kindGroupButton, code: "fr", from: admin, seed: "", wantStored: "fr"},
		{name: "group_missing_code", kind: kindGroupButton, noCode: true, from: admin, seed: "en", wantStored: "en"},
		{name: "group_unsupported_code", kind: kindGroupButton, code: "de", from: admin, seed: "en", wantStored: "en"},
		{name: "group_unsupported_before_admin_check", kind: kindGroupButton, code: "de", from: member, seed: "en", wantStored: "en"},
		{name: "group_upper_case_code", kind: kindGroupButton, code: "UK", from: admin, seed: "en", wantStored: "en"},
		{name: "group_non_admin", kind: kindGroupButton, code: "es", from: member, seed: "en", wantStored: "en"},
		{name: "group_save_failed", kind: kindGroupButton, code: "es", from: admin, seed: "en", failUpdate: true, wantStored: "en"},
		{name: "group_no_message", kind: kindGroupNoMessage, code: "es", from: admin, seed: "en", wantStored: "es"},
		{name: "private_new_user", kind: kindPrivateButton, code: "es", seed: "", wantStored: "es"},
		{name: "private_existing_user", kind: kindPrivateButton, code: "fr", seed: "en", wantStored: "fr"},
		{name: "private_unsupported_code", kind: kindPrivateButton, code: "de", seed: "en", wantStored: "en"},
		{name: "private_save_failed", kind: kindPrivateButton, code: "es", seed: "en", failUpdate: true, wantStored: "en"},
		{name: "command_group_admin", kind: kindGroupCommand, from: admin, seed: "en", wantStored: "en"},
		{name: "command_group_member", kind: kindGroupCommand, from: member, seed: "en", wantStored: "en"},
		{name: "command_private", kind: kindPrivateCommand, seed: "es", wantStored: "es"},
	}

	for i, tc := range cases {
		chatID := languageGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			private := tc.kind == kindPrivateButton || tc.kind == kindPrivateCommand
			userID := uniqueLinkOwnerID()
			var chatIDs, userIDs []int64
			if private {
				chatID = userID
				userIDs = []int64{userID}
			} else {
				chatIDs = []int64{chatID}
			}
			languageGoldenCleanup(t, chatIDs, userIDs)

			if tc.seed != "" {
				if private {
					mustLegacy(t, lang.ChangeUserLanguage(userID, tc.seed))
				} else {
					mustLegacy(t, lang.ChangeGroupLanguage(chatID, tc.seed))
				}
			}
			if tc.failUpdate {
				table := "chats"
				if private {
					table = "users"
				}
				languageGoldenFailUpdate(t, table)
			}

			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			from := tc.from
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Language Golden"}
			if private {
				from = gotgbot.User{Id: userID, FirstName: "Member"}
				chat = gotgbot.Chat{Id: userID, Type: "private", FirstName: "Member"}
			}

			fields := map[string]string{"l": tc.code}
			if tc.noCode {
				fields = map[string]string{}
			}
			data := encodeCallbackData("change_language", fields)

			var err error
			switch tc.kind {
			case kindGroupButton, kindPrivateButton:
				err = languagesModule.langBtnHandler(bot, newModuleCallbackContext(bot, chat, from, data))
			case kindGroupNoMessage:
				query := &gotgbot.CallbackQuery{Id: "callback-2", From: from, Data: data, ChatInstance: "test-chat-instance"}
				ctx := ext.NewContext(bot, &gotgbot.Update{UpdateId: 3, CallbackQuery: query}, nil)
				ctx.EffectiveChat = &chat
				err = languagesModule.langBtnHandler(bot, ctx)
			case kindGroupCommand, kindPrivateCommand:
				err = languagesModule.changeLanguage(bot, newModuleMessageContext(bot, chat, from, "/lang"))
			}
			if err != ext.EndGroups {
				t.Fatalf("handler error = %v, want EndGroups", err)
			}

			got := languageGoldenCalls(t, client, chatID)
			want, ok := goldenLanguageRequests[tc.name]
			if !ok {
				t.Logf("GOLDEN %s=%s", tc.name, strconv.Quote(got))
				t.Fatalf("no golden recorded for %s", tc.name)
			}
			if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}

			var stored string
			if private {
				var row models.User
				if err := db.DB.Where("user_id = ?", userID).First(&row).Error; err != nil {
					t.Fatalf("read users row: %v", err)
				}
				stored = row.Language
			} else {
				var row models.Chat
				readErr := db.DB.Where("chat_id = ?", chatID).First(&row).Error
				if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
					t.Fatalf("read chats row: %v", readErr)
				}
				stored = row.Language
			}
			if stored != tc.wantStored {
				t.Errorf("stored language = %q, want %q", stored, tc.wantStored)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
var goldenLanguageRequests = map[string]string{
	"group_en":                             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"This group's language has been changed to English 🇺🇸\"}",
	"group_es":                             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"El idioma de este grupo ha sido cambiado a Español 🇪🇸\"}",
	"group_hi":                             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"इस ग्रुप की भाषा हिन्दी 🇮🇳 में बदल दी गई है\"}",
	"group_uk":                             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"Мову цієї групи змінено на Українська 🇺🇦\"}",
	"group_id":                             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"Bahasa grup ini telah diubah ke Bahasa Indonesia 🇮🇩\"}",
	"group_same_code_again":                "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"El idioma de este grupo ha sido cambiado a Español 🇪🇸\"}",
	"group_first_write_no_row":             "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"La langue de ce groupe a été changée en Français 🇫🇷\"}",
	"group_missing_code":                   "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Invalid language selection.\"}",
	"group_unsupported_code":               "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Invalid language selection.\"}",
	"group_unsupported_before_admin_check": "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Invalid language selection.\"}",
	"group_upper_case_code":                "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Invalid language selection.\"}",
	"group_non_admin":                      "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"You need to be an admin to do this!\"}",
	"group_save_failed":                    "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Error al guardar la configuracion. Intentalo de nuevo mas tarde.\"}",
	"group_no_message":                     "answerCallbackQuery {\"callback_query_id\":\"callback-2\",\"text\":\"El idioma de este grupo ha sido cambiado a Español 🇪🇸\"}",
	"private_new_user":                     "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"Tu idioma ha sido cambiado a Español 🇪🇸\"}",
	"private_existing_user":                "answerCallbackQuery {\"callback_query_id\":\"callback-1\"}\neditMessageText {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"message_id\":102,\"parse_mode\":\"HTML\",\"reply_markup\":{},\"text\":\"Votre langue a été changée en Français 🇫🇷\"}",
	"private_unsupported_code":             "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Invalid language selection.\"}",
	"private_save_failed":                  "answerCallbackQuery {\"callback_query_id\":\"callback-1\",\"text\":\"Error al guardar la configuracion. Intentalo de nuevo mas tarde.\"}",
	"command_group_admin":                  "sendMessage {\"chat_id\":<chat>,\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Українська 🇺🇦\",\"callback_data\":\"change_language|v1|l=uk\"},{\"text\":\"English 🇺🇸\",\"callback_data\":\"change_language|v1|l=en\"}],[{\"text\":\"Español 🇪🇸\",\"callback_data\":\"change_language|v1|l=es\"},{\"text\":\"Français 🇫🇷\",\"callback_data\":\"change_language|v1|l=fr\"}],[{\"text\":\"हिन्दी 🇮🇳\",\"callback_data\":\"change_language|v1|l=hi\"},{\"text\":\"Bahasa Indonesia 🇮🇩\",\"callback_data\":\"change_language|v1|l=id\"}],[{\"text\":\"Português 🇵🇹\",\"callback_data\":\"change_language|v1|l=pt\"},{\"text\":\"Русский 🇷🇺\",\"callback_data\":\"change_language|v1|l=ru\"}],[{\"text\":\"Допоможіть нам перекладати 🌎\",\"url\":\"https://crowdin.com/project/fuku_robot\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"This Group's Current Language is English 🇺🇸\\nChoose a language from keyboard below.\"}",
	"command_group_member":                 "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Only admins can execute this command!\"}",
	"command_private":                      "sendMessage {\"chat_id\":<chat>,\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Українська 🇺🇦\",\"callback_data\":\"change_language|v1|l=uk\"},{\"text\":\"English 🇺🇸\",\"callback_data\":\"change_language|v1|l=en\"}],[{\"text\":\"Español 🇪🇸\",\"callback_data\":\"change_language|v1|l=es\"},{\"text\":\"Français 🇫🇷\",\"callback_data\":\"change_language|v1|l=fr\"}],[{\"text\":\"हिन्दी 🇮🇳\",\"callback_data\":\"change_language|v1|l=hi\"},{\"text\":\"Bahasa Indonesia 🇮🇩\",\"callback_data\":\"change_language|v1|l=id\"}],[{\"text\":\"Português 🇵🇹\",\"callback_data\":\"change_language|v1|l=pt\"},{\"text\":\"Русский 🇷🇺\",\"callback_data\":\"change_language|v1|l=ru\"}],[{\"text\":\"Допоможіть нам перекладати 🌎\",\"url\":\"https://crowdin.com/project/fuku_robot\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Tu Idioma Actual es Español 🇪🇸\\nElige un idioma del teclado de abajo.\"}",
}
