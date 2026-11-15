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
	"github.com/uasneppy/fuku_robot/fuku/db/locks"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// Fixed chat IDs of the Phase 18 block keep the locks goldens below stable, one per case.
const lockGoldenChatBase int64 = -1009918001800

// lockGoldenFailWrite makes every write of one lock type of the chat fail until the test ends,
// through a real GORM callback on the create chain the repository's upsert uses.
func lockGoldenFailWrite(t *testing.T, chatID int64, lockType string) {
	t.Helper()
	name := fmt.Sprintf("test:fail_lock_write_%d_%s", chatID, lockType)
	err := db.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*models.LockSettings)
		if ok && tx.Statement.Table == "locks" && row.ChatId == chatID && row.LockType == lockType {
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

// lockGoldenStored reads the chat's stored lock rows straight from the table, never through
// the cache, as a sorted "type=value" list.
func lockGoldenStored(t *testing.T, chatID int64) string {
	t.Helper()
	var rows []models.LockSettings
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read locks: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.LockType+"="+strconv.FormatBool(row.Locked))
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

// lockGoldenCleanup deletes the chat's lock rows and their cache entry before the case and
// again when it ends.
func lockGoldenCleanup(t *testing.T, chatID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.LockSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("locks_map", chatID))
		cache.InvalidateAdminCache(chatID)
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// TestLegacyLockRequests records what /lock, /unlock, /locks and /locktypes sent and stored
// before the extraction into applyLock. It is committed alone, before any production change,
// and is never re-recorded or weakened.
func TestLegacyLockRequests(t *testing.T) {
	withRealLocales(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name        string
		text        string
		run         func(*gotgbot.Bot, *ext.Context) error
		from        gotgbot.User
		seed        []string
		failType    string
		notAdminBot bool
		follow      bool
		golden      string
		goldenAfter string
		want        string
	}{
		{name: "lock_bot_not_admin", text: "/lock url", run: locksModule.lockPerm, from: admin, notAdminBot: true, golden: goldenLockBotNotAdmin},
		{name: "lock_not_admin", text: "/lock url", run: locksModule.lockPerm, from: member, golden: goldenLockNotAdmin},
		{name: "lock_no_argument", text: "/lock", run: locksModule.lockPerm, from: admin, golden: goldenLockNoArgument},
		{name: "lock_unknown_refuses_all", text: "/lock url bogus", run: locksModule.lockPerm, from: admin, golden: goldenLockUnknown},
		{name: "lock_unknown_upper_case", text: "/lock URL", run: locksModule.lockPerm, from: admin, golden: goldenLockUnknownUpper},
		{name: "lock_one", text: "/lock url", run: locksModule.lockPerm, from: admin, follow: true, golden: goldenLockOne, goldenAfter: goldenLocksAfterLockOne, want: "url=true"},
		{name: "lock_several", text: "/lock sticker gif", run: locksModule.lockPerm, from: admin, follow: true, golden: goldenLockSeveral, goldenAfter: goldenLocksAfterLockSeveral, want: "gif=true,sticker=true"},
		{name: "lock_restriction_and_lock", text: "/lock all url", run: locksModule.lockPerm, from: admin, follow: true, golden: goldenLockRestriction, goldenAfter: goldenLocksAfterLockRestriction, want: "all=true,url=true"},
		{name: "lock_one_of_several_fails", text: "/lock sticker gif", run: locksModule.lockPerm, from: admin, failType: "gif", follow: true, golden: goldenLockPartial, goldenAfter: goldenLocksAfterLockPartial, want: "sticker=true"},
		{name: "unlock_bot_not_admin", text: "/unlock url", run: locksModule.unlockPerm, from: admin, notAdminBot: true, seed: []string{"url"}, golden: goldenUnlockBotNotAdmin, want: "url=true"},
		{name: "unlock_not_admin", text: "/unlock url", run: locksModule.unlockPerm, from: member, seed: []string{"url"}, golden: goldenUnlockNotAdmin, want: "url=true"},
		{name: "unlock_no_argument", text: "/unlock", run: locksModule.unlockPerm, from: admin, seed: []string{"url"}, golden: goldenUnlockNoArgument, want: "url=true"},
		{name: "unlock_unknown_refuses_all", text: "/unlock url bogus", run: locksModule.unlockPerm, from: admin, seed: []string{"url"}, golden: goldenUnlockUnknown, want: "url=true"},
		{name: "unlock_one", text: "/unlock url", run: locksModule.unlockPerm, from: admin, seed: []string{"url", "sticker"}, follow: true, golden: goldenUnlockOne, goldenAfter: goldenLocksAfterUnlockOne, want: "sticker=true,url=false"},
		{name: "unlock_several", text: "/unlock sticker gif", run: locksModule.unlockPerm, from: admin, seed: []string{"url", "sticker", "gif"}, follow: true, golden: goldenUnlockSeveral, goldenAfter: goldenLocksAfterUnlockSeveral, want: "gif=false,sticker=false,url=true"},
		{name: "unlock_never_locked", text: "/unlock photo", run: locksModule.unlockPerm, from: admin, follow: true, golden: goldenUnlockNeverLocked, goldenAfter: goldenLocksAfterUnlockNeverLocked, want: "photo=false"},
		{name: "unlock_one_of_several_fails", text: "/unlock sticker gif", run: locksModule.unlockPerm, from: admin, seed: []string{"sticker", "gif"}, failType: "gif", follow: true, golden: goldenUnlockPartial, goldenAfter: goldenLocksAfterUnlockPartial, want: "gif=true,sticker=false"},
		{name: "locktypes", text: "/locktypes", run: locksModule.locktypes, from: admin, golden: goldenLocktypes},
		{name: "locks_empty", text: "/locks", run: locksModule.locks, from: admin, golden: goldenLocksEmpty},
	}

	for i, tc := range cases {
		chatID := lockGoldenChatBase - int64(i) - 1
		t.Run(tc.name, func(t *testing.T) {
			lockGoldenCleanup(t, chatID)
			for _, kind := range tc.seed {
				if err := locks.UpdateLock(chatID, kind, true); err != nil {
					t.Fatalf("seed lock %s: %v", kind, err)
				}
			}
			if tc.failType != "" {
				lockGoldenFailWrite(t, chatID, tc.failType)
			}
			client := newModuleBotClient()
			bot := newModuleTestBot(client)
			if tc.notAdminBot {
				bot.BotClient = warnGoldenNotAdminBot{client}
			}
			chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Locks Golden"}
			ctx := newModuleMessageContext(bot, chat, tc.from, tc.text)
			if err := tc.run(bot, ctx); err != ext.EndGroups {
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
			if stored := lockGoldenStored(t, chatID); stored != tc.want {
				t.Errorf("stored rows = %q, want %q", stored, tc.want)
			}
			if !tc.follow {
				return
			}
			afterCtx := newModuleMessageContext(bot, chat, admin, "/locks")
			if err := locksModule.locks(bot, afterCtx); err != ext.EndGroups {
				t.Fatalf("/locks error = %v, want EndGroups", err)
			}
			calls = client.callsFor("sendMessage")
			if len(calls) != 2 {
				t.Fatalf("sendMessage calls = %d, want 2", len(calls))
			}
			after := strings.ReplaceAll(legacyRequestJSON(t, calls[1]), strconv.FormatInt(chatID, 10), "<chat>")
			if after != tc.goldenAfter {
				t.Errorf("/locks request changed\n got %s\nwant %s", after, tc.goldenAfter)
			}
		})
	}
}

// Goldens recorded on the code as it was before Phase 18; <chat> stands for the case's chat ID.
const (
	goldenLockBotNotAdmin             = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
	goldenLockNotAdmin                = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenLockNoArgument              = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Що ви хочете заблокувати? Доступні варіанти дивіться в /locktypes.\"}"
	goldenLockUnknown                 = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"`bogus` не є правильним типом блокування, дивіться /locktypes.\"}"
	goldenLockUnknownUpper            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"`URL` не є правильним типом блокування, дивіться /locktypes.\"}"
	goldenLockOne                     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Заблоковано в цій групі: - url\"}"
	goldenLocksAfterLockOne           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - url = true\"}"
	goldenLockSeveral                 = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Заблоковано в цій групі: - sticker\\n - gif\"}"
	goldenLocksAfterLockSeveral       = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - gif = true\\n - sticker = true\"}"
	goldenLockRestriction             = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Заблоковано в цій групі: - all\\n - url\"}"
	goldenLocksAfterLockRestriction   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - all = true\\n - url = true\"}"
	goldenLockPartial                 = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося заблокувати: gif. Спробуйте пізніше.\"}"
	goldenLocksAfterLockPartial       = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - sticker = true\"}"
	goldenUnlockBotNotAdmin           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Я не адміністратор у цьому чаті!\"}"
	goldenUnlockNotAdmin              = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Щоб це зробити, потрібно бути адміністратором.\"}"
	goldenUnlockNoArgument            = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Що ви хочете розблокувати? Доступні варіанти дивіться в /locktypes.\"}"
	goldenUnlockUnknown               = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"`bogus` не є правильним типом блокування, дивіться /locktypes.\"}"
	goldenUnlockOne                   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Розблоковано в цій групі: - url\"}"
	goldenLocksAfterUnlockOne         = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - sticker = true\\n - url = false\"}"
	goldenUnlockSeveral               = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Розблоковано в цій групі: - sticker\\n - gif\"}"
	goldenLocksAfterUnlockSeveral     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - gif = false\\n - sticker = false\\n - url = true\"}"
	goldenUnlockNeverLocked           = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Розблоковано в цій групі: - photo\"}"
	goldenLocksAfterUnlockNeverLocked = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - photo = false\"}"
	goldenUnlockPartial               = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Не вдалося розблокувати: gif. Спробуйте пізніше.\"}"
	goldenLocksAfterUnlockPartial     = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\\n - gif = true\\n - sticker = false\"}"
	goldenLocktypes                   = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування: - all\\n - anonchannel\\n - audio\\n - bots\\n - comments\\n - contact\\n - document\\n - forward\\n - game\\n - gif\\n - location\\n - media\\n - messages\\n - other\\n - photo\\n - previews\\n - rtl\\n - sticker\\n - url\\n - video\\n - videonote\\n - voice\"}"
	goldenLocksEmpty                  = "{\"chat_id\":<chat>,\"link_preview_options\":{\"is_disabled\":true},\"parse_mode\":\"HTML\",\"reply_parameters\":{\"message_id\":101,\"allow_sending_without_reply\":true},\"text\":\"Блокування в цьому чаті:\"}"
)
