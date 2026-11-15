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
	"github.com/uasneppy/fuku_robot/fuku/db/admin"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// Fixed chat IDs of the Phase 18 block keep the /anonadmin goldens below stable: case i uses the
// chat anonGoldenChatBase-i.
const anonGoldenChatBase int64 = -1009918001701

// anonGoldenCleanup deletes the admin row of the chat before the case runs and again when it ends.
func anonGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.AdminSettings{})
	}
	clean()
	t.Cleanup(clean)
}

// anonGoldenFailWrite makes the update of the admin row fail until the test ends, through a real
// GORM callback on the update chain admin.SetAnonAdminMode uses.
func anonGoldenFailWrite(t *testing.T) {
	t.Helper()
	name := fmt.Sprintf("test:fail_admin_update_%s", t.Name())
	err := db.DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "admin" {
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

// anonGoldenCalls is every sendMessage request the case made, in order, one per line, with the
// chat ID blanked.
func anonGoldenCalls(t *testing.T, client *moduleBotClient, chatID int64) string {
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
		lines = append(lines, strings.ReplaceAll(line, strconv.FormatInt(chatID, 10), "<chat>"))
	}
	return strings.Join(lines, "\n")
}

// anonGoldenStored is the stored mode of the chat: "none" for no admin row, "on" or "off".
func anonGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var row models.AdminSettings
	err := db.DB.Where("chat_id = ?", chatID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "none"
	}
	if err != nil {
		t.Fatalf("read admin row: %v", err)
	}
	if row.AnonAdmin {
		return "on"
	}
	return "off"
}

// TestLegacyAnonAdminRequests records what /anonadmin sent and stored before the write moved
// behind applyAnonAdmin: the status for a mode on and off (the group title escaped, also for a
// member), on and off by the creator in each spelling, on when on and off when off, an invalid
// word, a non-owner refused after the command's own read had already created the admin row
// (Pitfall 3), and a failing save. It is committed alone, before any production change, and is
// never re-recorded or weakened.
func TestLegacyAnonAdminRequests(t *testing.T) {
	withRealLocales(t)
	creator := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name    string
		command string
		from    gotgbot.User
		// before is the stored mode before the command: "none", "on" or "off".
		before string
		fail   bool
		want   string
	}{
		{name: "status_off_no_row", command: "/anonadmin", from: creator, before: "none", want: "off"},
		{name: "status_off", command: "/anonadmin", from: creator, before: "off", want: "off"},
		{name: "status_on", command: "/anonadmin", from: creator, before: "on", want: "on"},
		{name: "status_on_by_member", command: "/anonadmin", from: member, before: "on", want: "on"},
		{name: "on_by_creator", command: "/anonadmin on", from: creator, before: "off", want: "on"},
		{name: "on_by_creator_no_row", command: "/anonadmin on", from: creator, before: "none", want: "on"},
		{name: "on_yes", command: "/anonadmin yes", from: creator, before: "off", want: "on"},
		{name: "on_true", command: "/anonadmin true", from: creator, before: "off", want: "on"},
		{name: "off_by_creator", command: "/anonadmin off", from: creator, before: "on", want: "off"},
		{name: "off_no", command: "/anonadmin no", from: creator, before: "on", want: "off"},
		{name: "off_false", command: "/anonadmin false", from: creator, before: "on", want: "off"},
		{name: "already_on", command: "/anonadmin on", from: creator, before: "on", want: "on"},
		{name: "already_off", command: "/anonadmin off", from: creator, before: "off", want: "off"},
		{name: "already_off_no_row", command: "/anonadmin off", from: creator, before: "none", want: "off"},
		{name: "invalid_word", command: "/anonadmin maybe", from: creator, before: "off", want: "off"},
		{name: "invalid_word_mode_on", command: "/anonadmin maybe", from: creator, before: "on", want: "on"},
		{name: "non_owner_on", command: "/anonadmin on", from: member, before: "off", want: "off"},
		{name: "non_owner_off", command: "/anonadmin off", from: member, before: "on", want: "on"},
		{name: "non_owner_no_row", command: "/anonadmin on", from: member, before: "none", want: "off"},
		{name: "non_owner_invalid_word", command: "/anonadmin maybe", from: member, before: "off", want: "off"},
		{name: "save_fails_on", command: "/anonadmin on", from: creator, before: "off", fail: true, want: "off"},
		{name: "save_fails_off", command: "/anonadmin off", from: creator, before: "on", fail: true, want: "on"},
		{name: "save_fails_but_already", command: "/anonadmin on", from: creator, before: "on", fail: true, want: "on"},
	}

	for i, tc := range cases {
		chatID := anonGoldenChatBase - int64(i)
		t.Run(tc.name, func(t *testing.T) {
			anonGoldenCleanup(t, chatID)
			switch tc.before {
			case "on":
				mustLegacy(t, admin.SetAnonAdminMode(chatID, true))
			case "off":
				mustLegacy(t, admin.SetAnonAdminMode(chatID, false))
			}
			if got := anonGoldenStored(t, chatID); got != tc.before {
				t.Fatalf("fixture stored = %s, want %s", got, tc.before)
			}
			if tc.fail {
				anonGoldenFailWrite(t)
			}

			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Anon <Golden> & Co"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.command)
			cmdCtx, err := helpers.BuildCommandContext(bot, ctx)
			if err != nil {
				t.Fatalf("BuildCommandContext: %v", err)
			}
			if err := adminModule.anonAdmin(cmdCtx); err != ext.EndGroups {
				t.Fatalf("handler error = %v, want EndGroups", err)
			}

			got := anonGoldenCalls(t, client, chatID)
			want, ok := goldenAnonAdminRequests[tc.name]
			if !ok {
				t.Logf("GOLDEN %s=%s", tc.name, strconv.Quote(got))
				t.Fatalf("no golden recorded for %s", tc.name)
			}
			if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			// Every case leaves an admin row behind, because the command reads through
			// admin.GetAdminSettings first, which creates it, even for a non-owner it then refuses.
			if stored := anonGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored mode = %s, want %s", stored, tc.want)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
var goldenAnonAdminRequests = map[string]string{
	"status_off_no_row":      "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co зараз \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\\n\\nАнонімні адміністратори мають натиснути кнопку, щоб підтвердити свої права.\"}",
	"status_off":             "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co зараз \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\\n\\nАнонімні адміністратори мають натиснути кнопку, щоб підтвердити свої права.\"}",
	"status_on":              "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co зараз \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nУсі анонімні адміністратори можуть виконувати адмінські дії без обмежень.\"}",
	"status_on_by_member":    "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co зараз \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nУсі анонімні адміністратори можуть виконувати адмінські дії без обмежень.\"}",
	"on_by_creator":          "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nВіднині я проситиму анонімних адміністраторів підтверджувати права.\"}",
	"on_by_creator_no_row":   "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nВіднині я проситиму анонімних адміністраторів підтверджувати права.\"}",
	"on_yes":                 "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nВіднині я проситиму анонімних адміністраторів підтверджувати права.\"}",
	"on_true":                "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eувімкнено\\u003c/b\\u003e.\\n\\nВіднині я проситиму анонімних адміністраторів підтверджувати права.\"}",
	"off_by_creator":         "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\\n\\nВіднині я більше не проситиму анонімних адміністраторів підтверджувати права.\"}",
	"off_no":                 "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\\n\\nВіднині я більше не проситиму анонімних адміністраторів підтверджувати права.\"}",
	"off_false":              "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co тепер \\u003cb\\u003eвимкнено\\u003c/b\\u003e.\\n\\nВіднині я більше не проситиму анонімних адміністраторів підтверджувати права.\"}",
	"already_on":             "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co уже \\u003cb\\u003eувімкнено\\u003c/b\\u003e\"}",
	"already_off":            "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co уже \\u003cb\\u003eвимкнено\\u003c/b\\u003e\"}",
	"already_off_no_row":     "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co уже \\u003cb\\u003eвимкнено\\u003c/b\\u003e\"}",
	"invalid_word":           "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Недійсний аргумент, я розумію лише \\u003ccode\\u003eon\\u003c/code\\u003e, \\u003ccode\\u003eoff\\u003c/code\\u003e, \\u003ccode\\u003eyes\\u003c/code\\u003e, \\u003ccode\\u003eno\\u003c/code\\u003e\"}",
	"invalid_word_mode_on":   "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Недійсний аргумент, я розумію лише \\u003ccode\\u003eon\\u003c/code\\u003e, \\u003ccode\\u003eoff\\u003c/code\\u003e, \\u003ccode\\u003eyes\\u003c/code\\u003e, \\u003ccode\\u003eno\\u003c/code\\u003e\"}",
	"non_owner_on":           "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Це може робити лише творець групи!\"}",
	"non_owner_off":          "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Це може робити лише творець групи!\"}",
	"non_owner_no_row":       "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Це може робити лише творець групи!\"}",
	"non_owner_invalid_word": "sendMessage {\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101},\"text\":\"Це може робити лише творець групи!\"}",
	"save_fails_on":          "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося оновити налаштування анонімного адміністратора. Спробуйте ще раз.\"}",
	"save_fails_off":         "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося оновити налаштування анонімного адміністратора. Спробуйте ще раз.\"}",
	"save_fails_but_already": "sendMessage {\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Режим AnonAdmin для Anon \\u0026lt;Golden\\u0026gt; \\u0026amp; Co уже \\u003cb\\u003eувімкнено\\u003c/b\\u003e\"}",
}
