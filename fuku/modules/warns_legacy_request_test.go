//go:build testtools

package modules

import (
	"context"
	"encoding/json"
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
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the warn goldens below stable, one per case.
const warnGoldenChatBase int64 = -1009918001100

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
const (
	goldenWarnLimitNoArgument    = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть, скільки попереджень може отримати користувач, перш ніж до нього буде застосовано дію. Напр. `/setwarnlimit 5`\"}"
	goldenWarnLimitNotANumber    = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"abc не є дійсним цілим числом.\"}"
	goldenWarnLimitZero          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ліміт попереджень має бути від 1 до 100.\"}"
	goldenWarnLimitOver          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ліміт попереджень має бути від 1 до 100.\"}"
	goldenWarnLimitOne           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ліміт попереджень для цього чату оновлено до 1.\"}"
	goldenWarnLimitHundred       = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ліміт попереджень для цього чату оновлено до 100.\"}"
	goldenWarnLimitFive          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ліміт попереджень для цього чату оновлено до 5.\"}"
	goldenWarnLimitNotAdmin      = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenWarnLimitBotNotAdmin   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
	goldenWarnSaveFailedMarkdown = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
	goldenWarnModeNoArgument     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Потрібно вказати дію при надто великій кількості попереджень. Доступні режими: ban/kick/mute\"}"
	goldenWarnModeBan            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим попереджень змінено на: ban\"}"
	goldenWarnModeKick           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим попереджень змінено на: kick\"}"
	goldenWarnModeMute           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим попереджень змінено на: mute\"}"
	goldenWarnModeTban           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідомий тип 'tban'. Використайте один із: ban/kick/mute\"}"
	goldenWarnModeXyz            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідомий тип 'xyz'. Використайте один із: ban/kick/mute\"}"
	goldenWarnModeAmpersand      = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Невідомий тип 'a\\u0026b\\u003cc'. Використайте один із: ban/kick/mute\"}"
	goldenWarnSaveFailedHTML     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
	goldenWarnModeNotAdmin       = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenWarnModeBotNotAdmin    = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
)

// warnGoldenRow is what the warns_settings table holds for a chat after a command.
type warnGoldenRow struct {
	exists bool
	limit  int
	mode   string
}

// warnGoldenNotAdminBot answers the bot's own member lookup as a plain member, so the
// bot-admin gate of a command refuses.
type warnGoldenNotAdminBot struct {
	*moduleBotClient
}

func (c warnGoldenNotAdminBot) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method == "getChatMember" && fmt.Sprint(params["user_id"]) == "999" {
		return json.RawMessage(`{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`), nil
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// warnGoldenFailWrite makes every write of the chat's warns_settings row fail until the test
// ends, through a real GORM callback on the create chain the commands' upsert uses.
func warnGoldenFailWrite(t *testing.T, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_warn_write_%d", chatID)
	err := db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*models.WarnSettings)
		if ok && tx.Statement.Table == "warns_settings" && row.ChatId == chatID {
			_ = tx.AddError(errors.New("database is down"))
		}
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Create().Remove(name); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})
}

// warnGoldenStored reads the stored row straight from the table, never through the cache.
func warnGoldenStored(t *testing.T, chatID int64) warnGoldenRow {
	t.Helper()
	var row models.WarnSettings
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return warnGoldenRow{}
	}
	if err != nil {
		t.Fatalf("read warns_settings: %v", err)
	}
	return warnGoldenRow{exists: true, limit: row.WarnLimit, mode: row.WarnMode}
}

// warnGoldenCleanup deletes the chat's warn settings row and its cache entry before the case
// and again when it ends.
func warnGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{})
		dbcache.DeleteCache(dbcache.CacheKey("warn_settings", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyWarnSettingsRequests records what /setwarnlimit and /setwarnmode sent and stored
// before the extraction into applyWarnLimit and applyWarnMode. It is committed alone, before
// any production change, and is never re-recorded or weakened.
func TestLegacyWarnSettingsRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	seeded := &models.WarnSettings{WarnLimit: 3, WarnMode: "ban"}

	cases := []struct {
		name        string
		text        string
		from        gotgbot.User
		seed        *models.WarnSettings
		failWrite   bool
		notAdminBot bool
		golden      string
		want        warnGoldenRow
	}{
		{name: "limit_no_argument", text: "/setwarnlimit", from: admin, golden: goldenWarnLimitNoArgument},
		{name: "limit_not_a_number", text: "/setwarnlimit abc", from: admin, golden: goldenWarnLimitNotANumber},
		{name: "limit_zero", text: "/setwarnlimit 0", from: admin, golden: goldenWarnLimitZero},
		{name: "limit_over_range", text: "/setwarnlimit 101", from: admin, golden: goldenWarnLimitOver},
		{name: "limit_one", text: "/setwarnlimit 1", from: admin, golden: goldenWarnLimitOne, want: warnGoldenRow{true, 1, "mute"}},
		{name: "limit_hundred", text: "/setwarnlimit 100", from: admin, golden: goldenWarnLimitHundred, want: warnGoldenRow{true, 100, "mute"}},
		{name: "limit_five_new_chat", text: "/setwarnlimit 5", from: admin, golden: goldenWarnLimitFive, want: warnGoldenRow{true, 5, "mute"}},
		{name: "limit_five_keeps_mode", text: "/setwarnlimit 5", from: admin, seed: seeded, golden: goldenWarnLimitFive, want: warnGoldenRow{true, 5, "ban"}},
		{name: "limit_range_keeps_row", text: "/setwarnlimit 0", from: admin, seed: seeded, golden: goldenWarnLimitZero, want: warnGoldenRow{true, 3, "ban"}},
		{name: "limit_save_failed", text: "/setwarnlimit 7", from: admin, seed: seeded, failWrite: true, golden: goldenWarnSaveFailedMarkdown, want: warnGoldenRow{true, 3, "ban"}},
		{name: "limit_not_admin", text: "/setwarnlimit 5", from: member, golden: goldenWarnLimitNotAdmin},
		{name: "limit_bot_not_admin", text: "/setwarnlimit 5", from: admin, notAdminBot: true, golden: goldenWarnLimitBotNotAdmin},

		{name: "mode_no_argument", text: "/setwarnmode", from: admin, golden: goldenWarnModeNoArgument},
		{name: "mode_ban_upper", text: "/setwarnmode BAN", from: admin, golden: goldenWarnModeBan, want: warnGoldenRow{true, 3, "ban"}},
		{name: "mode_kick", text: "/setwarnmode kick", from: admin, golden: goldenWarnModeKick, want: warnGoldenRow{true, 3, "kick"}},
		{name: "mode_mute", text: "/setwarnmode mute", from: admin, golden: goldenWarnModeMute, want: warnGoldenRow{true, 3, "mute"}},
		{name: "mode_ban_keeps_limit", text: "/setwarnmode ban", from: admin, seed: &models.WarnSettings{WarnLimit: 9, WarnMode: "mute"}, golden: goldenWarnModeBan, want: warnGoldenRow{true, 9, "ban"}},
		{name: "mode_tban_unknown", text: "/setwarnmode tban", from: admin, golden: goldenWarnModeTban},
		{name: "mode_xyz_unknown", text: "/setwarnmode xyz", from: admin, seed: seeded, golden: goldenWarnModeXyz, want: warnGoldenRow{true, 3, "ban"}},
		{name: "mode_unknown_unescaped", text: "/setwarnmode a&b<c", from: admin, golden: goldenWarnModeAmpersand},
		{name: "mode_save_failed", text: "/setwarnmode kick", from: admin, seed: seeded, failWrite: true, golden: goldenWarnSaveFailedHTML, want: warnGoldenRow{true, 3, "ban"}},
		{name: "mode_not_admin", text: "/setwarnmode ban", from: member, golden: goldenWarnModeNotAdmin},
		{name: "mode_bot_not_admin", text: "/setwarnmode ban", from: admin, notAdminBot: true, golden: goldenWarnModeBotNotAdmin},
	}

	for i, tc := range cases {
		chatID := warnGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			warnGoldenCleanup(t, chatID)
			if tc.seed != nil {
				row := *tc.seed
				row.ChatId = chatID
				if err := db.DB.Create(&row).Error; err != nil {
					t.Fatalf("seed warns_settings: %v", err)
				}
			}
			if tc.failWrite {
				warnGoldenFailWrite(t, chatID)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.notAdminBot {
				bot.BotClient = warnGoldenNotAdminBot{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Warn Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.text)
			var err error
			if strings.HasPrefix(tc.text, "/setwarnlimit") {
				err = warnsModule.setWarnLimit(bot, ctx)
			} else {
				err = warnsModule.setWarnMode(bot, ctx)
			}
			if err != ext.EndGroups {
				t.Fatalf("command error = %v, want EndGroups", err)
			}
			calls := client.callsFor("sendMessage")
			if len(calls) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(calls))
			}
			got := strings.ReplaceAll(legacyRequestJSON(t, calls[0]), strconv.FormatInt(chatID, 10), "<chat>")
			if got != tc.golden {
				t.Errorf("request changed\n got %s\nwant %s", got, tc.golden)
			}
			if stored := warnGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored row = %+v, want %+v", stored, tc.want)
			}
		})
	}
}
