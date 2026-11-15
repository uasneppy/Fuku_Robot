//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the log category goldens below stable: case i uses
// the chat logGoldenChatBase-2i-1 and the log channel logGoldenChatBase-2i-2.
const logGoldenChatBase int64 = -1009918001600

// logGoldenCleanup deletes the log channel, chats and cache rows of the case before it runs and
// again when it ends.
func logGoldenCleanup(t *testing.T, chatID, channelID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", []int64{chatID, channelID}).Delete(&models.LogChannel{})
		db.DB.Where("chat_id IN ?", []int64{chatID, channelID}).Delete(&models.Chat{})
		for _, id := range []int64{chatID, channelID} {
			dbcache.DeleteCache(dbcache.CacheKey("log_channel", id))
			dbcache.DeleteCache(dbcache.CacheKey("chat", id))
			cache.InvalidateAdminCache(id)
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// logGoldenFailWrite makes the update of the named category column fail until the test ends (an
// empty column fails every category write), through a real GORM callback on the update chain
// the category write uses.
func logGoldenFailWrite(t *testing.T, column string) {
	t.Helper()
	name := fmt.Sprintf("test:fail_log_channels_update_%s", t.Name())
	err := db.DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "log_channels" {
			return
		}
		if updates, ok := tx.Statement.Dest.(map[string]any); ok {
			if _, hit := updates[column]; column == "" || hit {
				_ = tx.AddError(errors.New("database is down"))
			}
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

// logGoldenCalls is every sendMessage request the case made, in order, one per line, with the
// chat ID and the log channel ID blanked, so a reply in the group and the audit post to the
// log channel can be told apart.
func logGoldenCalls(t *testing.T, client *moduleBotClient, chatID, channelID int64) string {
	t.Helper()
	client.mu.Lock()
	calls := append([]moduleBotCall(nil), client.calls...)
	client.mu.Unlock()
	var lines []string
	for _, call := range calls {
		if call.Method != "sendMessage" {
			continue
		}
		line := call.Method + " " + legacyRequestJSON(t, call)
		line = strings.ReplaceAll(line, strconv.FormatInt(channelID, 10), "<chan>")
		line = strings.ReplaceAll(line, strconv.FormatInt(chatID, 10), "<chat>")
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// logGoldenFlags is the six category flags of the chat's stored row, in the order of
// logchannels.AllCategories, as a string of 1 and 0, or "none" when the chat has no row.
func logGoldenFlags(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.LogChannel
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "none"
	}
	if err != nil {
		t.Fatalf("read log_channels row: %v", err)
	}
	var out strings.Builder
	for _, name := range logchannels.AllCategories {
		if logchannels.CategoryEnabled(&row, name) {
			out.WriteByte('1')
		} else {
			out.WriteByte('0')
		}
	}
	return out.String()
}

// TestLegacyLogCategoryRequests records what /log and /nolog sent and stored before each
// category write moved behind applyLogCategory: a private chat, a non-admin, no linked channel,
// no argument, unknown categories (alone and next to a known one), one category, several, a
// repeated one, one of several failing to save, every save failing (the loop goes on, so the reply
// and the audit post carry an empty list), and the #SETTINGS audit post each makes to the log
// channel (which a /nolog settings switches off before it can be sent). It is committed alone,
// before any production change, and is never re-recorded or weakened.
func TestLegacyLogCategoryRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name    string
		command string
		enable  bool
		private bool
		from    gotgbot.User
		// noChannel leaves the chat with no linked log channel.
		noChannel bool
		// off lists the categories turned off before the command runs.
		off []string
		// failColumn makes the write of that column fail; failAll fails every category write.
		failColumn string
		failAll    bool
		wantFlags  string
	}{
		{name: "private_chat", command: "/log settings", enable: true, private: true, from: admin, wantFlags: "none"},
		{name: "private_chat_nolog", command: "/nolog settings", private: true, from: admin, wantFlags: "none"},
		{name: "non_admin", command: "/log admin", enable: true, from: member, off: []string{"admin"}, wantFlags: "101111"},
		{name: "non_admin_nolog", command: "/nolog admin", from: member, wantFlags: "111111"},
		{name: "no_channel", command: "/log admin", enable: true, from: admin, noChannel: true, wantFlags: "none"},
		{name: "no_channel_nolog", command: "/nolog admin", from: admin, noChannel: true, wantFlags: "none"},
		{name: "no_argument", command: "/log", enable: true, from: admin, wantFlags: "111111"},
		{name: "no_argument_nolog", command: "/nolog", from: admin, wantFlags: "111111"},
		{name: "unknown_category", command: "/log bogus", enable: true, from: admin, wantFlags: "111111"},
		{name: "unknown_categories_nolog", command: "/nolog bogus nonsense", from: admin, wantFlags: "111111"},
		{name: "unknown_with_known", command: "/log bogus admin", enable: true, from: admin, off: []string{"admin"}, wantFlags: "111111"},
		{name: "one_category_off", command: "/nolog admin", from: admin, wantFlags: "101111"},
		{name: "one_category_on", command: "/log admin", enable: true, from: admin, off: []string{"admin"}, wantFlags: "111111"},
		{name: "one_category_mixed_case", command: "/log RePorts", enable: true, from: admin, off: []string{"reports"}, wantFlags: "111111"},
		{name: "several_on", command: "/log admin user", enable: true, from: admin, off: []string{"admin", "user"}, wantFlags: "111111"},
		{name: "several_off", command: "/nolog admin user other", from: admin, wantFlags: "100110"},
		{name: "repeated_category", command: "/nolog admin admin", from: admin, wantFlags: "101111"},
		{name: "already_in_state", command: "/log admin", enable: true, from: admin, wantFlags: "111111"},
		{name: "nolog_settings_itself", command: "/nolog settings", from: admin, wantFlags: "011111"},
		{name: "nolog_settings_with_another", command: "/nolog admin settings", from: admin, wantFlags: "001111"},
		{name: "one_of_several_fails", command: "/log admin user", enable: true, from: admin, off: []string{"admin", "user"}, failColumn: "cat_user", wantFlags: "110111"},
		{name: "one_of_several_fails_off", command: "/nolog admin user", from: admin, failColumn: "cat_user", wantFlags: "101111"},
		{name: "all_fail", command: "/log admin user", enable: true, from: admin, off: []string{"admin", "user"}, failAll: true, wantFlags: "100111"},
		{name: "all_fail_one_category", command: "/nolog admin", from: admin, failAll: true, wantFlags: "111111"},
		{name: "all_fail_with_unknown", command: "/log bogus admin", enable: true, from: admin, off: []string{"admin"}, failAll: true, wantFlags: "101111"},
	}

	for i, tc := range cases {
		chatID := logGoldenChatBase - 2*int64(i) - 1
		channelID := logGoldenChatBase - 2*int64(i) - 2
		t.Run(tc.name, func(t *testing.T) {
			logGoldenCleanup(t, chatID, channelID)
			mustLegacy(t, chats.EnsureChatInDb(chatID, "Log Golden"))
			mustLegacy(t, chats.EnsureChatInDb(channelID, "Log Golden Channel"))
			if !tc.noChannel && !tc.private {
				mustLegacy(t, logchannels.Set(chatID, "Log Golden", channelID))
				for _, name := range tc.off {
					mustLegacy(t, logchannels.SetCategory(chatID, name, false))
				}
			}
			switch {
			case tc.failAll:
				logGoldenFailWrite(t, "")
			case tc.failColumn != "":
				logGoldenFailWrite(t, tc.failColumn)
			}

			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chatType := "supergroup"
			if tc.private {
				chatType = "private"
			}
			chat := gotgbot.Chat{Id: chatID, Type: chatType, Title: "Log Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.command)

			var err error
			if tc.enable {
				err = logChannelsModule.enableLog(bot, ctx)
			} else {
				err = logChannelsModule.disableLog(bot, ctx)
			}
			if err != ext.EndGroups {
				t.Fatalf("handler error = %v, want EndGroups", err)
			}

			got := logGoldenCalls(t, client, chatID, channelID)
			want, ok := goldenLogCategoryRequests[tc.name]
			if !ok {
				t.Logf("GOLDEN %s=%s", tc.name, strconv.Quote(got))
				t.Fatalf("no golden recorded for %s", tc.name)
			}
			if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			if flags := logGoldenFlags(t, chatID); flags != tc.wantFlags {
				t.Errorf("stored flags = %s, want %s", flags, tc.wantFlags)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> and <chan> stand for the case's
// chat and log channel IDs.
var goldenLogCategoryRequests = map[string]string{
	"private_chat":                "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Цю команду можна використовувати лише в групах.\"}",
	"private_chat_nolog":          "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Цю команду можна використовувати лише в групах.\"}",
	"non_admin":                   "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Цю команду можуть виконувати лише адміни!\"}",
	"non_admin_nolog":             "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Цю команду можуть виконувати лише адміни!\"}",
	"no_channel":                  "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Для цього чату не задано канал журналу.\"}",
	"no_channel_nolog":            "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Для цього чату не задано канал журналу.\"}",
	"no_argument":                 "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Використання: /log \\u003ccategory\\u003e або /nolog \\u003ccategory\\u003e\"}",
	"no_argument_nolog":           "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Використання: /log \\u003ccategory\\u003e або /nolog \\u003ccategory\\u003e\"}",
	"unknown_category":            "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідома категорія. Дивіться /logcategories.\"}",
	"unknown_categories_nolog":    "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідома категорія. Дивіться /logcategories.\"}",
	"unknown_with_known":          "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled admin\"}",
	"one_category_off":            "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_disabled admin\"}",
	"one_category_on":             "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled admin\"}",
	"one_category_mixed_case":     "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: reports\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled reports\"}",
	"several_on":                  "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: admin, user\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled admin, user\"}",
	"several_off":                 "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: admin, user, other\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_disabled admin, user, other\"}",
	"repeated_category":           "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: admin, admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_disabled admin, admin\"}",
	"already_in_state":            "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled admin\"}",
	"nolog_settings_itself":       "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: settings\"}",
	"nolog_settings_with_another": "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: admin, settings\"}",
	"one_of_several_fails":        "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled admin\"}",
	"one_of_several_fails_off":    "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: admin\"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_disabled admin\"}",
	"all_fail":                    "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання ввімкнено для: \"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_enabled \"}",
	"all_fail_one_category":       "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Журналювання вимкнено для: \"}\nsendMessage {\"chat_id\":<chan>,\"parse_mode\":\"HTML\",\"text\":\"\\u003cb\\u003eLog Golden\\u003c/b\\u003e (\\u003ccode\\u003e<chat>\\u003c/code\\u003e)\\n#SETTINGS\\nAdmin: \\u003ca href=\\\"tg://user?id=777000\\\"\\u003eTelegram\\u003c/a\\u003e\\nlogs_disabled \"}",
	"all_fail_with_unknown":       "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідома категорія. Дивіться /logcategories.\"}",
}
