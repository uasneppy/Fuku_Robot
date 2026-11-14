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
	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// Fixed chat IDs of the Phase 18 block keep the /aispam goldens below stable, one per case.
const aispamGoldenChatBase int64 = -1009918001400

// aispamGoldenProviderValue is the stand-in provider value the goldens configure; it is never logged.
const aispamGoldenProviderValue = "provider-test-value"

// aispamGoldenFailWrite makes every insert into ai_spam_settings fail until the test ends,
// through a real GORM callback on the create chain the write uses.
func aispamGoldenFailWrite(t *testing.T, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_ai_spam_write_%d", chatID)
	err := db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "ai_spam_settings" {
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

// aispamGoldenStored reads the stored row straight from the table: "none", "on" or "off".
func aispamGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.AISpamSettings
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "none"
	}
	if err != nil {
		t.Fatalf("read ai_spam_settings: %v", err)
	}
	if row.Enabled {
		return "on"
	}
	return "off"
}

// aispamGoldenCleanup deletes the chat's AI spam row, chats row and cached copy before the
// case and again when it ends.
func aispamGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.AISpamSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("ai_spam_settings", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyAISpamRequests records what /aispam sent, stored and returned before the extraction
// into applyAISpamEnabled. It is committed alone, before any production change, and is never
// re-recorded or weakened.
func TestLegacyAISpamRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 42, FirstName: "Admin"}

	const (
		seedNone = iota
		seedOn
		seedOff
	)
	cases := []struct {
		name      string
		text      string
		provider  string
		switchOn  bool
		seed      int
		failWrite bool
		golden    string
		wantRow   string
		wantErr   bool
	}{
		{name: "status_configured_on", text: "/aispam", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamStatusOn, wantRow: "on"},
		{name: "status_configured_off", text: "/aispam", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, golden: goldenAISpamStatusOff, wantRow: "off"},
		{name: "status_no_value_on", text: "/aispam", provider: "", switchOn: true, seed: seedOn, golden: goldenAISpamStatusOnNoValue, wantRow: "on"},
		{name: "status_no_value_off", text: "/aispam", provider: "", switchOn: true, seed: seedOff, golden: goldenAISpamStatusOffNoValue, wantRow: "off"},
		{name: "status_switch_off_on", text: "/aispam", provider: aispamGoldenProviderValue, switchOn: false, seed: seedOn, golden: goldenAISpamStatusOnSwitchOff, wantRow: "on"},
		{name: "status_switch_off_off", text: "/aispam", provider: aispamGoldenProviderValue, switchOn: false, seed: seedOff, golden: goldenAISpamStatusOffSwitchOff, wantRow: "off"},
		{name: "status_both_missing_on", text: "/aispam", provider: "", switchOn: false, seed: seedOn, golden: goldenAISpamStatusOnNoValue, wantRow: "on"},
		{name: "status_both_missing_off", text: "/aispam", provider: "", switchOn: false, seed: seedOff, golden: goldenAISpamStatusOffNoValue, wantRow: "off"},
		{name: "status_new_chat_creates_no_row", text: "/aispam", provider: aispamGoldenProviderValue, switchOn: true, seed: seedNone, golden: goldenAISpamStatusOff, wantRow: "none"},
		{name: "on", text: "/aispam on", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, golden: goldenAISpamEnabled, wantRow: "on"},
		{name: "yes", text: "/aispam yes", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, golden: goldenAISpamEnabled, wantRow: "on"},
		{name: "true", text: "/aispam true", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, golden: goldenAISpamEnabled, wantRow: "on"},
		{name: "enable", text: "/aispam enable", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, golden: goldenAISpamEnabled, wantRow: "on"},
		{name: "on_upper_new_chat", text: "/aispam ON", provider: aispamGoldenProviderValue, switchOn: true, seed: seedNone, golden: goldenAISpamEnabled, wantRow: "on"},
		{name: "off", text: "/aispam off", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "no", text: "/aispam no", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "false", text: "/aispam false", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "disable", text: "/aispam disable", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "off_new_chat", text: "/aispam off", provider: aispamGoldenProviderValue, switchOn: true, seed: seedNone, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "off_while_no_value_writes", text: "/aispam off", provider: "", switchOn: true, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "off_while_switch_off_writes", text: "/aispam off", provider: aispamGoldenProviderValue, switchOn: false, seed: seedOn, golden: goldenAISpamDisabled, wantRow: "off"},
		{name: "invalid_word", text: "/aispam maybe", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, golden: goldenAISpamInvalid, wantRow: "on"},
		{name: "on_refused_no_value", text: "/aispam on", provider: "", switchOn: true, seed: seedOff, golden: goldenAISpamNoValue, wantRow: "off"},
		{name: "on_refused_switch_off", text: "/aispam on", provider: aispamGoldenProviderValue, switchOn: false, seed: seedOff, golden: goldenAISpamGloballyOff, wantRow: "off"},
		{name: "on_refused_both_missing", text: "/aispam on", provider: "", switchOn: false, seed: seedNone, golden: goldenAISpamNoValue, wantRow: "none"},
		{name: "save_failed_returns_error", text: "/aispam on", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOff, failWrite: true, golden: goldenAISpamSaveFailed, wantRow: "off", wantErr: true},
		{name: "save_failed_off_returns_error", text: "/aispam off", provider: aispamGoldenProviderValue, switchOn: true, seed: seedOn, failWrite: true, golden: goldenAISpamSaveFailed, wantRow: "on", wantErr: true},
	}

	for i, tc := range cases {
		chatID := aispamGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			resetAISpamRuntime()
			aispamGoldenCleanup(t, chatID)
			withAISpamTestConfig(t, tc.provider, tc.switchOn, 0)
			switch tc.seed {
			case seedOn:
				mustLegacy(t, aispam.SetAISpamEnabled(chatID, true))
			case seedOff:
				mustLegacy(t, aispam.SetAISpamEnabled(chatID, false))
			}
			if tc.failWrite {
				aispamGoldenFailWrite(t, chatID)
			}
			client := aispamAdminClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "AI Spam Golden"}

			var handlerErrs []error
			dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
				MaxRoutines: -1,
				Error: func(_ *gotgbot.Bot, _ *ext.Context, err error) ext.DispatcherAction {
					handlerErrs = append(handlerErrs, err)
					return ext.DispatcherActionNoop
				},
			})
			LoadAISpam(dispatcher)
			if err := dispatcher.ProcessUpdate(bot, aispamMessageUpdate(chat, admin, 101, tc.text), nil); err != nil {
				t.Fatalf("ProcessUpdate error = %v", err)
			}

			calls := client.callsFor("sendMessage")
			if len(calls) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(calls))
			}
			got := strings.ReplaceAll(legacyRequestJSON(t, calls[0]), strconv.FormatInt(chatID, 10), "<chat>")
			if got != tc.golden {
				t.Errorf("request changed\n got %s\nwant %s", got, tc.golden)
			}
			if row := aispamGoldenStored(t, chatID); row != tc.wantRow {
				t.Errorf("stored row = %s, want %s", row, tc.wantRow)
			}
			if tc.wantErr && len(handlerErrs) != 1 {
				t.Errorf("handler errors = %v, want exactly one (the save error)", handlerErrs)
			}
			if !tc.wantErr && len(handlerErrs) != 0 {
				t.Errorf("handler errors = %v, want none", handlerErrs)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
const (
	goldenAISpamStatusOn           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamStatusOff          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eвимкнено\\u003c/b\\u003e. Увімкніть його командою \\u003ccode\\u003e/aispam on\\u003c/code\\u003e.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamStatusOnNoValue    = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\n⚠️ Недоступно: оператор бота не налаштував ключ API, тож жодне повідомлення не перевіряється.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamStatusOffNoValue   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eвимкнено\\u003c/b\\u003e. Увімкніть його командою \\u003ccode\\u003e/aispam on\\u003c/code\\u003e.\\n\\n⚠️ Недоступно: оператор бота не налаштував ключ API, тож жодне повідомлення не перевіряється.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamStatusOnSwitchOff  = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\n⚠️ Недоступно: оператор бота глобально вимкнув ШІ-фільтр спаму, тож жодне повідомлення не перевіряється.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamStatusOffSwitchOff = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"ШІ-фільтр спаму в цьому чаті \\u003cb\\u003eвимкнено\\u003c/b\\u003e. Увімкніть його командою \\u003ccode\\u003e/aispam on\\u003c/code\\u003e.\\n\\n⚠️ Недоступно: оператор бота глобально вимкнув ШІ-фільтр спаму, тож жодне повідомлення не перевіряється.\\n\\n\\u003cb\\u003eЩо надсилається для кожного перевірюваного повідомлення:\\u003c/b\\u003e\\n• Назва, опис, правила та мова чату\\n• Профіль відправника: кількість днів відколи бот уперше його побачив, повідомлення за останню годину, кількість попереджень\\n• Текст або підпис повідомлення, домени посилань, а також чи це пересилання або відповідь\\n• Кілька останніх повідомлень відправника в цьому чаті\\n\\n\\nІмена, юзернейми, ID користувачів, номери телефонів і повідомлення з інших чатів не надсилаються.\\n\"}"
	goldenAISpamEnabled            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"✅ ШІ-фільтр спаму тепер \\u003cb\\u003eувімкнено\\u003c/b\\u003e. Повідомлення, які модель упевнено вважає спамом, будуть видалятися.\"}"
	goldenAISpamDisabled           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"❌ ШІ-фільтр спаму тепер \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\"}"
	goldenAISpamInvalid            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я розумію лише один із варіантів: \\u003ccode\\u003eon\\u003c/code\\u003e/\\u003ccode\\u003eoff\\u003c/code\\u003e/\\u003ccode\\u003eyes\\u003c/code\\u003e/\\u003ccode\\u003eno\\u003c/code\\u003e\"}"
	goldenAISpamNoValue            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"❌ Не можу увімкнути ШІ-фільтр спаму: оператор бота не налаштував ключ API. Нічого не змінено.\"}"
	goldenAISpamGloballyOff        = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"❌ Не можу увімкнути ШІ-фільтр спаму: оператор бота вимкнув його глобально. Нічого не змінено.\"}"
	goldenAISpamSaveFailed         = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
)
