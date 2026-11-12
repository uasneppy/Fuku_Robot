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
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/reports"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the reports goldens below stable, one per case.
const reportsGoldenChatBase int64 = -1009918001200

// reportsGoldenRow is what the report_chat_settings table holds for a chat after a command.
type reportsGoldenRow struct {
	exists  bool
	enabled bool
	status  bool
}

// reportsGoldenFailWrite makes every update of report_chat_settings (or of the per-user table
// when user is true) fail until the test ends, through a real GORM callback.
func reportsGoldenFailWrite(t *testing.T, table string, id int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_%s_write_%d", table, id)
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

// reportsGoldenStored reads the stored chat row straight from the table.
func reportsGoldenStored(t *testing.T, chatID int64) reportsGoldenRow {
	t.Helper()
	var row models.ReportChatSettings
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reportsGoldenRow{}
	}
	if err != nil {
		t.Fatalf("read report_chat_settings: %v", err)
	}
	return reportsGoldenRow{exists: true, enabled: row.Enabled, status: row.Status}
}

// reportsGoldenCleanup deletes the chat's report row and chats row before the case and again
// when it ends.
func reportsGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ReportChatSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyReportsRequests records what /reports sent and stored before the extraction into
// applyReportsEnabled. It is committed alone, before any production change, and is never
// re-recorded or weakened.
func TestLegacyReportsRequests(t *testing.T) {
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
		want        reportsGoldenRow
	}{
		{name: "on", text: "/reports on", from: admin, seed: seedOff, golden: goldenReportsOnGroup, want: reportsGoldenRow{true, true, true}},
		{name: "yes", text: "/reports yes", from: admin, seed: seedOff, golden: goldenReportsOnGroup, want: reportsGoldenRow{true, true, true}},
		{name: "true", text: "/reports true", from: admin, seed: seedOff, golden: goldenReportsOnGroup, want: reportsGoldenRow{true, true, true}},
		{name: "on_upper_new_chat", text: "/reports ON", from: admin, seed: seedNone, golden: goldenReportsOnGroup, want: reportsGoldenRow{true, true, true}},
		{name: "off", text: "/reports off", from: admin, seed: seedOn, golden: goldenReportsOffGroup, want: reportsGoldenRow{true, false, false}},
		{name: "no", text: "/reports no", from: admin, seed: seedOn, golden: goldenReportsOffGroup, want: reportsGoldenRow{true, false, false}},
		{name: "false", text: "/reports false", from: admin, seed: seedOn, golden: goldenReportsOffGroup, want: reportsGoldenRow{true, false, false}},
		{name: "off_new_chat", text: "/reports off", from: admin, seed: seedNone, golden: goldenReportsOffGroup, want: reportsGoldenRow{true, false, false}},
		{name: "status_on", text: "/reports", from: admin, seed: seedOn, golden: goldenReportsStatusOn, want: reportsGoldenRow{true, true, true}},
		{name: "status_off", text: "/reports", from: admin, seed: seedOff, golden: goldenReportsStatusOff, want: reportsGoldenRow{true, false, false}},
		{name: "status_new_chat_creates_row", text: "/reports", from: admin, seed: seedNone, golden: goldenReportsStatusOn, want: reportsGoldenRow{true, true, true}},
		{name: "invalid_word", text: "/reports maybe", from: admin, seed: seedOn, golden: goldenReportsInvalid, want: reportsGoldenRow{true, true, true}},
		{name: "save_failed", text: "/reports off", from: admin, seed: seedOn, failWrite: true, golden: goldenReportsSaveFailed, want: reportsGoldenRow{true, true, true}},
		{name: "not_admin", text: "/reports off", from: member, seed: seedOn, golden: goldenReportsNotAdmin, want: reportsGoldenRow{true, true, true}},
		{name: "bot_not_admin", text: "/reports off", from: admin, seed: seedOn, notAdminBot: true, golden: goldenReportsBotNotAdmin, want: reportsGoldenRow{true, true, true}},
	}

	for i, tc := range cases {
		chatID := reportsGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			reportsGoldenCleanup(t, chatID)
			switch tc.seed {
			case seedOn:
				reports.GetChatReportSettings(chatID)
			case seedOff:
				if err := reports.SetChatReportStatus(chatID, false); err != nil {
					t.Fatalf("seed report_chat_settings: %v", err)
				}
			}
			if tc.failWrite {
				reportsGoldenFailWrite(t, "report_chat_settings", chatID)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.notAdminBot {
				bot.BotClient = warnGoldenNotAdminBot{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Reports Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.text)
			if err := reportsModule.reports(bot, ctx); err != ext.EndGroups {
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
			if stored := reportsGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored row = %+v, want %+v", stored, tc.want)
			}
		})
	}
}

// TestLegacyReportsPrivateRequests pins the private-chat branch of /reports, which writes the
// user's own preference and never the connected group's row (the branch stays in the command).
func TestLegacyReportsPrivateRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	cases := []struct {
		name      string
		text      string
		seed      bool
		failWrite bool
		golden    string
		want      bool
	}{
		{name: "on", text: "/reports on", seed: false, golden: goldenReportsOnPersonal, want: true},
		{name: "off", text: "/reports off", seed: true, golden: goldenReportsOffPersonal, want: false},
		{name: "status_on", text: "/reports", seed: true, golden: goldenReportsPrivateStatusOn, want: true},
		{name: "status_off", text: "/reports", seed: false, golden: goldenReportsPrivateStatusOff, want: false},
		{name: "save_failed", text: "/reports off", seed: true, failWrite: true, golden: goldenReportsPersonalSaveFailed, want: true},
	}
	for i, tc := range cases {
		connectedID := reportsGoldenChatBase - 100 - int64(i)
		t.Run(tc.name, func(t *testing.T) {
			reportsGoldenCleanup(t, connectedID)
			clean := func() {
				db.DB.Where("user_id = ?", admin.Id).Delete(&models.ReportUserSettings{})
				db.DB.Where("user_id = ?", admin.Id).Delete(&models.User{})
				dbcache.DeleteCache(dbcache.CacheKey("user", admin.Id))
				dbcache.ResetLocalForTest()
			}
			clean()
			t.Cleanup(clean)
			if tc.seed {
				if err := reports.SetUserReportSettings(admin.Id, true); err != nil {
					t.Fatalf("seed report_user_settings: %v", err)
				}
			} else if tc.name == "status_off" {
				if err := reports.SetUserReportSettings(admin.Id, false); err != nil {
					t.Fatalf("seed report_user_settings: %v", err)
				}
			}
			if tc.failWrite {
				reportsGoldenFailWrite(t, "report_user_settings", admin.Id)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			privateChat := gotgbot.Chat{Id: admin.Id, Type: "private", FirstName: "Telegram"}
			client.responses["getChat"] = []byte(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Connected Report Chat"}`, connectedID))
			connections.ConnectId(admin.Id, connectedID)
			t.Cleanup(func() { connections.DisconnectId(admin.Id) })

			ctx := newModuleMessageContext(bot, privateChat, admin, tc.text)
			if err := reportsModule.reports(bot, ctx); err != ext.EndGroups {
				t.Fatalf("command error = %v, want EndGroups", err)
			}
			calls := client.callsFor("sendMessage")
			if len(calls) != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", len(calls))
			}
			got := strings.ReplaceAll(legacyRequestJSON(t, calls[0]), strconv.FormatInt(connectedID, 10), "<chat>")
			if got != tc.golden {
				t.Errorf("request changed\n got %s\nwant %s", got, tc.golden)
			}
			var row models.ReportUserSettings
			if err := db.DB.Where("user_id = ?", admin.Id).First(&row).Error; err != nil {
				t.Fatalf("read report_user_settings: %v", err)
			}
			if row.Status != tc.want || row.Enabled != tc.want {
				t.Errorf("user row = enabled %v status %v, want both %v", row.Enabled, row.Status, tc.want)
			}
			if stored := reportsGoldenStored(t, connectedID); stored.exists {
				t.Errorf("a private /reports wrote the connected group's row: %+v", stored)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
const (
	goldenReportsOnGroup            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Тепер користувачі зможуть скаржитися на повідомлення.\"}"
	goldenReportsOffGroup           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Користувачі більше не зможуть скаржитися через @admin або /report.\"}"
	goldenReportsStatusOn           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Скарги в цьому чаті зараз увімкнено.\\nКористувачі можуть скористатися командою /report або згадати @admin, щоб позначити всіх адмінів.\\n\\nЩоб змінити це налаштування, повторіть команду з одним із аргументів: yes/no/on/off\"}"
	goldenReportsStatusOff          = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Скарги в цьому чаті зараз вимкнено.\\n\\nЩоб змінити це налаштування, повторіть команду з одним із аргументів: yes/no/on/off\"}"
	goldenReportsInvalid            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ваш ввід не розпізнано як одне з: \\u003ccode\\u003e\\u003cyes/on/no/off\\u003e or \\u003cblock/unblock/showblocklist\\u003e\\u003c/code\\u003e\"}"
	goldenReportsSaveFailed         = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
	goldenReportsNotAdmin           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenReportsBotNotAdmin        = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
	goldenReportsOnPersonal         = "{\"chat_id\":777000,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Скарги увімкнено! Ви отримуватимете сповіщення, коли хтось поскаржиться в групах, де ви адмін.\"}"
	goldenReportsOffPersonal        = "{\"chat_id\":777000,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Скарги вимкнено! Ви більше не отримуватимете сповіщень, коли хтось скаржиться в групах, де ви адмін.\"}"
	goldenReportsPrivateStatusOn    = "{\"chat_id\":777000,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ваше поточне налаштування: увімкнено. Ви отримуватимете сповіщення, коли хтось поскаржиться в групах, де ви адмін.\\n\\nЩоб змінити це налаштування, повторіть команду з одним із аргументів: yes/no/on/off\"}"
	goldenReportsPersonalSaveFailed = "{\"chat_id\":777000,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти налаштування. Спробуйте пізніше.\"}"
	goldenReportsPrivateStatusOff   = "{\"chat_id\":777000,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Ви не ввімкнули скарги. Сповіщень не буде.\\n\\nЩоб змінити це налаштування, повторіть команду з одним із аргументів: yes/no/on/off\"}"
)
