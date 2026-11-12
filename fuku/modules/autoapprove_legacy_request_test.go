//go:build testtools

package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the auto-approve goldens below stable, one per case.
const autoApproveGoldenChatBase int64 = -1009918001300

// autoApproveGoldenFailWrite makes every update of the greetings table fail until the test
// ends, through a real GORM callback on the update chain the write uses.
func autoApproveGoldenFailWrite(t *testing.T, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_greetings_write_%d", chatID)
	err := db.DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "greetings" {
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

// autoApproveGoldenRow is the whole greetings row of the chat read with a plain query, as JSON
// with the row ID, the chat ID and both timestamps blanked, or "none" when there is no row.
func autoApproveGoldenRow(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.GreetingSettings
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "none"
	}
	if err != nil {
		t.Fatalf("read greetings: %v", err)
	}
	row.ID, row.ChatID = 0, 0
	row.CreatedAt, row.UpdatedAt = time.Time{}, time.Time{}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("encode greetings row: %v", err)
	}
	return string(raw)
}

// TestLegacyAutoApproveRequests records what /autoapprove sent and stored before the extraction
// into applyAutoApprove, including the whole greetings row a first write creates on a chat with
// none. It is committed alone, before any production change, and is never re-recorded or
// weakened.
func TestLegacyAutoApproveRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	const (
		seedNone = iota
		seedOn
		seedOff
	)
	cases := []struct {
		name        string
		text        string
		from        gotgbot.User
		seed        int
		failWrite   bool
		notAdminBot bool
		golden      string
		wantRow     string
	}{
		{name: "on_first_write_whole_row", text: "/autoapprove on", from: admin, seed: seedNone, golden: goldenAutoApproveEnable, wantRow: goldenAutoApproveRowOn},
		{name: "yes", text: "/autoapprove yes", from: admin, seed: seedOff, golden: goldenAutoApproveEnable, wantRow: goldenAutoApproveRowOn},
		{name: "on_upper", text: "/autoapprove ON", from: admin, seed: seedOff, golden: goldenAutoApproveEnable, wantRow: goldenAutoApproveRowOn},
		{name: "off", text: "/autoapprove off", from: admin, seed: seedOn, golden: goldenAutoApproveDisable, wantRow: goldenAutoApproveRowOff},
		{name: "no", text: "/autoapprove no", from: admin, seed: seedOn, golden: goldenAutoApproveDisable, wantRow: goldenAutoApproveRowOff},
		{name: "off_first_write_whole_row", text: "/autoapprove off", from: admin, seed: seedNone, golden: goldenAutoApproveDisable, wantRow: goldenAutoApproveRowOff},
		{name: "status_enabled_markdown", text: "/autoapprove", from: admin, seed: seedOn, golden: goldenAutoApproveStatusOn, wantRow: goldenAutoApproveRowOn},
		{name: "status_disabled", text: "/autoapprove", from: admin, seed: seedOff, golden: goldenAutoApproveStatusOff, wantRow: goldenAutoApproveRowOff},
		{name: "status_new_chat", text: "/autoapprove", from: admin, seed: seedNone, golden: goldenAutoApproveStatusOff, wantRow: goldenAutoApproveRowNewStatus},
		{name: "invalid_word", text: "/autoapprove maybe", from: admin, seed: seedOn, golden: goldenAutoApproveInvalid, wantRow: goldenAutoApproveRowOn},
		{name: "save_failed", text: "/autoapprove off", from: admin, seed: seedOn, failWrite: true, golden: goldenAutoApproveSaveFailed, wantRow: goldenAutoApproveRowOn},
		{name: "save_failed_first_write", text: "/autoapprove on", from: admin, seed: seedNone, failWrite: true, golden: goldenAutoApproveSaveFailed, wantRow: goldenAutoApproveRowOff},
		{name: "not_change_info", text: "/autoapprove on", from: member, seed: seedOff, golden: goldenAutoApproveNotAllowed, wantRow: goldenAutoApproveRowOff},
		{name: "bot_not_admin", text: "/autoapprove on", from: admin, seed: seedOff, notAdminBot: true, golden: goldenAutoApproveBotNotAdmin, wantRow: goldenAutoApproveRowOff},
	}

	for i, tc := range cases {
		chatID := autoApproveGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			legacyCleanup(t, chatID)
			cache.InvalidateAdminCache(chatID)
			t.Cleanup(func() { cache.InvalidateAdminCache(chatID) })
			switch tc.seed {
			case seedOn:
				mustLegacy(t, greetings.SetShouldAutoApprove(chatID, true))
			case seedOff:
				mustLegacy(t, greetings.SetShouldAutoApprove(chatID, false))
			}
			if tc.failWrite {
				autoApproveGoldenFailWrite(t, chatID)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.notAdminBot {
				bot.BotClient = warnGoldenNotAdminBot{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Auto Approve Golden"}
			ctx := newGreetingMessageContext(bot, chat, tc.from, tc.text)
			if err := greetingsModule.autoApprove(bot, ctx); err != ext.EndGroups {
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
			if row := autoApproveGoldenRow(t, chatID); row != tc.wantRow {
				t.Errorf("greetings row changed\n got %s\nwant %s", row, tc.wantRow)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
const (
	goldenAutoApproveEnable       = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я намагатимуся автоматично затверджувати нові запити на вступ!\"}"
	goldenAutoApproveDisable      = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не затверджуватиму нові запити на вступ автоматично!\"}"
	goldenAutoApproveStatusOn     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"Markdown\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Тепер я автоматично затверджую нові запити на вступ до чату.\"}"
	goldenAutoApproveStatusOff    = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Зараз я не затверджую нові запити на вступ до чату автоматично.\"}"
	goldenAutoApproveInvalid      = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я розумію лише 'on/yes' або 'off/no'!\"}"
	goldenAutoApproveSaveFailed   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
	goldenAutoApproveNotAllowed   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenAutoApproveBotNotAdmin  = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
	goldenAutoApproveRowOn        = "{\"clean_service_settings\":false,\"welcome_settings\":{\"clean_old\":false,\"welcome_enabled\":true,\"welcome_text\":\"Привіт, {first}, як справи?\",\"welcome_type\":1,\"media_below\":false,\"once\":false},\"goodbye_settings\":{\"clean_old\":false,\"enabled\":true,\"text\":\"Шкода, що ти йдеш, {first}\",\"type\":1,\"media_below\":false},\"auto_approve\":true,\"created_at\":\"0001-01-01T00:00:00Z\",\"updated_at\":\"0001-01-01T00:00:00Z\"}"
	goldenAutoApproveRowOff       = "{\"clean_service_settings\":false,\"welcome_settings\":{\"clean_old\":false,\"welcome_enabled\":true,\"welcome_text\":\"Привіт, {first}, як справи?\",\"welcome_type\":1,\"media_below\":false,\"once\":false},\"goodbye_settings\":{\"clean_old\":false,\"enabled\":true,\"text\":\"Шкода, що ти йдеш, {first}\",\"type\":1,\"media_below\":false},\"auto_approve\":false,\"created_at\":\"0001-01-01T00:00:00Z\",\"updated_at\":\"0001-01-01T00:00:00Z\"}"
	goldenAutoApproveRowNewStatus = "none"
)
