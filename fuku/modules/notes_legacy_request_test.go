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

// Fixed chat IDs of the Phase 19 note block keep the goldens below stable, one per case.
const noteGoldenChatBase int64 = -1009919001300

// noteGoldenCardMarker stands for the random value the overwrite card carries in its callback data.
const noteGoldenCardMarker = "<card>"

// noteGoldenNoInfoUser is an administrator of the case's chat that lacks can_change_info.
const noteGoldenNoInfoUser int64 = 4301

var noteGoldenCardRe = regexp.MustCompile(`\\u0026t=([A-Za-z0-9_-]+)`)

// noteGoldenClient answers the member and administrator lookups with one administrator that
// lacks can_change_info and, when botNotAdmin is set, shows the bot as a plain member.
type noteGoldenClient struct {
	*moduleBotClient
	botNotAdmin bool
}

func noteGoldenAdmin(id int64, name string, bot, changeInfo bool) string {
	return fmt.Sprintf(`{"status":"administrator","user":{"id":%d,"is_bot":%t,"first_name":%q},"can_pin_messages":true,"can_delete_messages":true,"can_restrict_members":true,"can_promote_members":true,"can_change_info":%t,"can_invite_users":true,"can_manage_chat":true}`, id, bot, name, changeInfo)
}

func (c noteGoldenClient) RequestWithContext(ctx context.Context, token string, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	user := fmt.Sprint(params["user_id"])
	switch {
	case method == "getChatMember" && user == strconv.FormatInt(noteGoldenNoInfoUser, 10):
		return json.RawMessage(noteGoldenAdmin(noteGoldenNoInfoUser, "Helper", false, false)), nil
	case method == "getChatMember" && user == "999" && c.botNotAdmin:
		return json.RawMessage(`{"status":"member","user":{"id":999,"is_bot":true,"first_name":"Fuku"}}`), nil
	case method == "getChatAdministrators":
		list := []string{noteGoldenAdmin(noteGoldenNoInfoUser, "Helper", false, false)}
		if !c.botNotAdmin {
			list = append(list, noteGoldenAdmin(999, "Fuku", true, true))
		}
		return json.RawMessage("[" + strings.Join(list, ",") + "]"), nil
	}
	return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// noteGoldenFail makes one kind of write of the case's note rows fail until the test ends,
// through a real GORM callback matching only that table and chat.
func noteGoldenFail(t *testing.T, kind string, chatID int64) {
	t.Helper()
	name := fmt.Sprintf("test:fail_note_%s_%d", kind, chatID)
	var err error
	switch kind {
	case "create":
		err = db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
			row, ok := tx.Statement.Dest.(map[string]any)
			if ok && tx.Statement.Table == "notes" && row["chat_id"] == chatID {
				_ = tx.AddError(errors.New("database is down"))
			}
		})
	default:
		err = db.DB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "notes" && blacklistGoldenWhereHas(tx, chatID) {
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

// noteGoldenStored reads the chat's stored note rows straight from the table, never through the
// cache, as a sorted "name|type|file|content|buttons|private,group,admin,web,protected,nonotif" list.
func noteGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var rows []models.Notes
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read notes: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		buttons, err := json.Marshal(row.Buttons)
		if err != nil {
			t.Fatalf("encode buttons: %v", err)
		}
		out = append(out, fmt.Sprintf("%s|%d|%s|%s|%s|%t,%t,%t,%t,%t,%t", row.NoteName, row.MsgType, row.FileID,
			row.NoteContent, buttons, row.PrivateOnly, row.GroupOnly, row.AdminOnly, row.WebPreview, row.IsProtected, row.NoNotif))
	}
	slices.Sort(out)
	return strings.Join(out, ";")
}

// noteGoldenCleanup deletes the chat's rows and cache entries before the case and again when it
// ends.
func noteGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Notes{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.NotesSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("notes_list", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("notes_settings", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyNoteRequests records what /save, /clear and /notes sent and stored before the
// extraction into the apply functions. It is committed alone, before any production change, and is
// never re-recorded or weakened. It holds no name with whitespace, none starting with # and none
// over 100 code points (the name rule is an owner-confirmation change of this phase with its own
// test), and no case where the existing-name read fails (the fail-closed lookup is another one).
func TestLegacyNoteRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	helper := gotgbot.User{Id: noteGoldenNoInfoUser, FirstName: "Helper"}

	textReply := func(chat gotgbot.Chat) *gotgbot.Message {
		return &gotgbot.Message{MessageId: 55, Date: 1, Chat: chat, From: &member, Text: "reply body"}
	}
	seedRules := []string{"rules"}
	cases := []struct {
		name        string
		text        string
		run         func(*gotgbot.Bot, *ext.Context) error
		from        gotgbot.User
		reply       func(gotgbot.Chat) *gotgbot.Message
		seed        []string
		noInfo      bool
		botNotAdmin bool
		noCache     bool
		failKind    string
		card        bool
	}{
		{name: "save_not_admin", text: "/save rules hello there", run: notesModule.addNote, from: member},
		{name: "save_bot_not_admin", text: "/save rules hello there", run: notesModule.addNote, from: admin, botNotAdmin: true},
		{name: "save_no_change_info_right", text: "/save rules hello there", run: notesModule.addNote, from: helper, noInfo: true},
		{name: "save_reply_without_keyword", text: "/save", run: notesModule.addNote, from: admin, reply: textReply},
		{name: "save_no_reply_invalid", text: "/save rules", run: notesModule.addNote, from: admin},
		{name: "save_reply_to_poll", text: "/save quiz", run: notesModule.addNote, from: admin, reply: func(chat gotgbot.Chat) *gotgbot.Message {
			return &gotgbot.Message{MessageId: 56, Date: 1, Chat: chat, From: &member, Poll: &gotgbot.Poll{Id: "poll-1", Question: "Q"}}
		}},
		{name: "save_inline_multiline", text: "/save rules line one\nline *two*", run: notesModule.addNote, from: admin},
		{name: "save_inline_mixed_case", text: "/save Rules Be kind", run: notesModule.addNote, from: admin},
		{name: "save_reply_to_text", text: "/save note1", run: notesModule.addNote, from: admin, reply: textReply},
		{name: "save_reply_photo_admin_directive", text: "/save pic", run: notesModule.addNote, from: admin, reply: func(chat gotgbot.Chat) *gotgbot.Message {
			return &gotgbot.Message{
				MessageId: 58, Date: 1, Chat: chat, From: &member,
				Photo:   []gotgbot.PhotoSize{{FileId: "photo-small", FileUniqueId: "u1"}, {FileId: "photo-big", FileUniqueId: "u2"}},
				Caption: "see {admin} [Site](buttonurl://https://example.com)",
			}
		}},
		{name: "save_private_conflict", text: "/save x hi {private} {noprivate}", run: notesModule.addNote, from: admin},
		{name: "save_alias", text: "/addnote greet welcome aboard", run: notesModule.addNote, from: admin},
		{name: "save_existing_overwrite_card", text: "/save rules new text", run: notesModule.addNote, from: admin, seed: seedRules, card: true},
		{name: "save_existing_without_cache", text: "/save rules new text", run: notesModule.addNote, from: admin, seed: seedRules, noCache: true},
		{name: "save_write_fails", text: "/save price tag", run: notesModule.addNote, from: admin, failKind: "create"},
		{name: "clear_no_argument", text: "/clear", run: notesModule.rmNote, from: admin, seed: seedRules},
		{name: "clear_absent_name", text: "/clear ghost", run: notesModule.rmNote, from: admin, seed: seedRules},
		{name: "clear_removes", text: "/clear rules", run: notesModule.rmNote, from: admin, seed: seedRules},
		{name: "clear_hash_stripped", text: "/clear #pic", run: notesModule.rmNote, from: admin, seed: []string{"pic", "keep"}},
		{name: "clear_original_case", text: "/clear Rules", run: notesModule.rmNote, from: admin, seed: seedRules},
		{name: "clear_alias", text: "/rmnote rules", run: notesModule.rmNote, from: admin, seed: seedRules},
		{name: "clear_no_change_info_right", text: "/clear rules", run: notesModule.rmNote, from: helper, seed: seedRules, noInfo: true},
		{name: "clear_delete_fails", text: "/clear rules", run: notesModule.rmNote, from: admin, seed: seedRules, failKind: "delete"},
		{name: "list_empty", text: "/notes", run: notesModule.notesList, from: admin},
		{name: "list_filled", text: "/notes", run: notesModule.notesList, from: admin, seed: []string{"rules", "faq"}},
	}

	for i, tc := range cases {
		chatID := noteGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			noteGoldenCleanup(t, chatID)
			for _, name := range tc.seed {
				entry := &models.Notes{ChatId: chatID, NoteName: name, NoteContent: "old", MsgType: db.TEXT, WebPreview: true}
				if err := db.DB.Create(entry).Error; err != nil {
					t.Fatalf("seed %q: %v", name, err)
				}
			}
			if tc.failKind != "" {
				noteGoldenFail(t, tc.failKind, chatID)
			}
			if tc.noCache {
				withNilCacheMarshal(t)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.noInfo || tc.botNotAdmin {
				bot.BotClient = noteGoldenClient{moduleBotClient: client, botNotAdmin: tc.botNotAdmin}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Note Golden"}
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
				if match := noteGoldenCardRe.FindStringSubmatch(raw); tc.card && match != nil {
					if _, err := getOverwriteCache[overwriteNote](noteOverwriteCacheKey(match[1])); err != nil {
						t.Errorf("overwrite key of the card is not stored: %v", err)
					}
					raw = strings.ReplaceAll(raw, match[1], noteGoldenCardMarker)
				}
				requests = append(requests, raw)
			}
			if tc.card && !strings.Contains(strings.Join(requests, "\n"), noteGoldenCardMarker) {
				t.Errorf("no overwrite card was sent: %v", requests)
			}
			got := strings.Join(requests, "\n")
			want, ok := noteGoldens[tc.name]
			if !ok {
				t.Logf("GOLDEN\t%s\t%q", tc.name, got)
			} else if got != want {
				t.Errorf("requests changed\n got %s\nwant %s", got, want)
			}
			stored := noteGoldenStored(t, chatID)
			wantRow, ok := noteGoldenRows[tc.name]
			if !ok {
				t.Logf("ROW\t%s\t%q", tc.name, stored)
			} else if stored != wantRow {
				t.Errorf("stored rows = %q, want %q", stored, wantRow)
			}
		})
	}
}

// Stored rows after each case, recorded on the code as it was before Phase 19.
var noteGoldenRows = map[string]string{
	"save_not_admin":                   "",
	"save_bot_not_admin":               "",
	"save_no_change_info_right":        "",
	"save_reply_without_keyword":       "",
	"save_no_reply_invalid":            "",
	"save_reply_to_poll":               "",
	"save_inline_multiline":            "rules|1||line one line <b>two</b>|[]|false,false,false,false,false,false",
	"save_inline_mixed_case":           "rules|1||Be kind|[]|false,false,false,false,false,false",
	"save_reply_to_text":               "note1|1||reply body|[]|false,false,false,false,false,false",
	"save_reply_photo_admin_directive": "pic|4|photo-big|see {admin}|[{\"name\":\"Site\",\"url\":\"https://example.com\",\"btn_sameline\":false}]|false,false,true,false,false,false",
	"save_private_conflict":            "x|1||hi {private} {noprivate}|[]|false,false,false,false,false,false",
	"save_alias":                       "greet|1||welcome aboard|[]|false,false,false,false,false,false",
	"save_existing_overwrite_card":     "rules|1||old|[]|false,false,false,true,false,false",
	"save_existing_without_cache":      "rules|1||old|[]|false,false,false,true,false,false",
	"save_write_fails":                 "",
	"clear_no_argument":                "rules|1||old|[]|false,false,false,true,false,false",
	"clear_absent_name":                "rules|1||old|[]|false,false,false,true,false,false",
	"clear_removes":                    "",
	"clear_hash_stripped":              "keep|1||old|[]|false,false,false,true,false,false",
	"clear_original_case":              "",
	"clear_alias":                      "",
	"clear_no_change_info_right":       "rules|1||old|[]|false,false,false,true,false,false",
	"clear_delete_fails":               "rules|1||old|[]|false,false,false,true,false,false",
	"list_empty":                       "",
	"list_filled":                      "faq|1||old|[]|false,false,false,true,false,false;rules|1||old|[]|false,false,false,true,false,false",
}

// Goldens recorded on the code as it was before Phase 19 (one sendMessage request per line,
// <chat> standing for the case's chat ID).
var noteGoldens = map[string]string{
	"save_not_admin":                   "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}",
	"save_bot_not_admin":               "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}",
	"save_no_change_info_right":        "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У вас немає прав змінювати інформацію в цій групі!\"}",
	"save_reply_without_keyword":       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть ключове слово, на яке відповідати!\"}",
	"save_no_reply_invalid":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Недійсна нотатка!\"}",
	"save_reply_to_poll":               "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Потрібно додати до нотатки якийсь вміст!\"}",
	"save_inline_multiline":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003erules\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#rules\\u003c/code\\u003e або \\u003ccode\\u003e/get rules\\u003c/code\\u003e.\"}",
	"save_inline_mixed_case":           "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003erules\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#rules\\u003c/code\\u003e або \\u003ccode\\u003e/get rules\\u003c/code\\u003e.\"}",
	"save_reply_to_text":               "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003enote1\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#note1\\u003c/code\\u003e або \\u003ccode\\u003e/get note1\\u003c/code\\u003e.\"}",
	"save_reply_photo_admin_directive": "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003epic\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#pic\\u003c/code\\u003e або \\u003ccode\\u003e/get pic\\u003c/code\\u003e.\"}",
	"save_private_conflict":            "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003ex\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#x\\u003c/code\\u003e або \\u003ccode\\u003e/get x\\u003c/code\\u003e.\\n\\n\\u003cb\\u003eПримітка:\\u003c/b\\u003e цю нотатку буде надіслано згідно з типовим налаштуванням групових нотаток, оскільки вона містить і \\u003ccode\\u003e{private}\\u003c/code\\u003e, і \\u003ccode\\u003e{noprivate}\\u003c/code\\u003e.\"}",
	"save_alias":                       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003egreet\\u003c/b\\u003e збережено!\\nОтримати її можна через \\u003ccode\\u003e#greet\\u003c/code\\u003e або \\u003ccode\\u003e/get greet\\u003c/code\\u003e.\"}",
	"save_existing_overwrite_card":     "{\"chat_id\":<chat>,\"parse_mode\":\"HTML\",\"reply_markup\":{\"inline_keyboard\":[[{\"text\":\"Так\",\"callback_data\":\"notes.overwrite|v1|a=yes\\u0026t=<card>\"},{\"text\":\"Ні\",\"callback_data\":\"notes.overwrite|v1|a=no\\u0026t=<card>\"}]]},\"reply_parameters\":{\"message_id\":101},\"text\":\"Нотатка вже існує!\\nБажаєте перезаписати її?\"}",
	"save_existing_without_cache":      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося створити підтвердження перезапису. Спробуйте ще раз.\"}",
	"save_write_fails":                 "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося зберегти нотатку. Спробуйте пізніше.\"}",
	"clear_no_argument":                "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Вкажіть ключове слово для видалення!\"}",
	"clear_absent_name":                "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатки не існує!\"}",
	"clear_removes":                    "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003erules\\u003c/b\\u003e видалено.\"}",
	"clear_hash_stripped":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003epic\\u003c/b\\u003e видалено.\"}",
	"clear_original_case":              "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003eRules\\u003c/b\\u003e видалено.\"}",
	"clear_alias":                      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Нотатку \\u003cb\\u003erules\\u003c/b\\u003e видалено.\"}",
	"clear_no_change_info_right":       "{\"chat_id\":<chat>,\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У вас немає прав змінювати інформацію в цій групі!\"}",
	"clear_delete_fails":               "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Сталася помилка. Спробуйте пізніше.\"}",
	"list_empty":                       "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"У цьому чаті немає нотаток!\"}",
	"list_filled":                      "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Поточні нотатки в цьому чаті:\\n - \\u003ccode\\u003e#faq\\u003c/code\\u003e\\n - \\u003ccode\\u003e#rules\\u003c/code\\u003e\\n\\nОтримати нотатку можна через \\u003ccode\\u003e#notename\\u003c/code\\u003e або \\u003ccode\\u003e/get notename\\u003c/code\\u003e\"}",
}
