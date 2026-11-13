//go:build testtools

package modules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/filters"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// filterApplyChat returns a fresh chat ID whose filter rows and cache entries are deleted before the
// test and when it ends.
func filterApplyChat(t *testing.T) int64 {
	t.Helper()
	chatID := uniqueModuleChatID()
	clean := func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatFilters{})
		dbcache.DeleteCache(dbcache.CacheKey("filter_list", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("filters_optimized", chatID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
	return chatID
}

// filterApplySeams swaps the write seams for the test and restores them when it ends.
func filterApplySeams(t *testing.T, seams filterWriteSeams) {
	t.Helper()
	previous := filterWrites
	filterWrites = seams
	t.Cleanup(func() { filterWrites = previous })
}

// filterApplySeed stores filters straight in the table, without the caches.
func filterApplySeed(t *testing.T, chatID int64, keywords ...string) {
	t.Helper()
	rows := make([]models.ChatFilters, 0, len(keywords))
	for _, keyword := range keywords {
		rows = append(rows, models.ChatFilters{ChatId: chatID, KeyWord: keyword, FilterReply: "r", MsgType: db.TEXT})
	}
	if err := db.DB.CreateInBatches(rows, 50).Error; err != nil {
		t.Fatalf("seed filters: %v", err)
	}
}

func filterApplyInput(keyword string) filterAddInput {
	return filterAddInput{Keyword: keyword, Text: "reply", DataType: db.TEXT}
}

func TestApplyFilter(t *testing.T) {
	ctx := context.Background()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	const secretKeyword = "zzsecretkeywordzz"
	assertNoKeywordLogged := func(t *testing.T) {
		t.Helper()
		for _, entry := range hook.AllEntries() {
			if strings.Contains(strings.ToLower(entry.Message), secretKeyword) {
				t.Errorf("log line carries a keyword: %q", entry.Message)
			}
		}
	}

	t.Run("refusals before any write", func(t *testing.T) {
		chatID := filterApplyChat(t)
		if got := applyFilterAdd(ctx, chatID, filterApplyInput(strings.Repeat("x", 101))).Outcome; got != filterKeywordTooLong {
			t.Errorf("101 runes = %v, want too long", got)
		}
		if got := applyFilterAdd(ctx, chatID, filterApplyInput(strings.Repeat("界", 100))).Outcome; got != filterAdded {
			t.Errorf("100 runes = %v, want added", got)
		}
		if got := applyFilterAdd(ctx, chatID, filterApplyInput("  ")).Outcome; got != filterKeywordBlank {
			t.Errorf("blank = %v, want blank", got)
		}
		filterApplySeed(t, chatID, "taken")
		if got := applyFilterAdd(ctx, chatID, filterApplyInput("TAKEN")).Outcome; got != filterExists {
			t.Errorf("stored keyword = %v, want exists", got)
		}
	})

	t.Run("cap reached at 150 rows", func(t *testing.T) {
		chatID := filterApplyChat(t)
		keywords := make([]string, 0, filtersCapMax)
		for n := 0; n < filtersCapMax; n++ {
			keywords = append(keywords, "word"+strconv.Itoa(n))
		}
		filterApplySeed(t, chatID, keywords...)
		if got := applyFilterAdd(ctx, chatID, filterApplyInput("overflow")).Outcome; got != filterCapReached {
			t.Errorf("151st = %v, want cap reached", got)
		}
		if filters.DoesFilterExists(chatID, "overflow") {
			t.Error("a filter was stored past the cap")
		}
	})

	t.Run("added drops both caches", func(t *testing.T) {
		chatID := filterApplyChat(t)
		filterApplySeed(t, chatID, "old")
		if list := filters.GetFiltersList(chatID); len(list) != 1 {
			t.Fatalf("primed list = %v", list)
		}
		if all, err := filters.GetChatFiltersCached(chatID); err != nil || len(all) != 1 {
			t.Fatalf("primed optimized list = %d, %v", len(all), err)
		}
		in := filterAddInput{Keyword: "Fresh", Text: "body", FileID: "file-1", DataType: db.PHOTO, Buttons: []db.Button{{Name: "Site", Url: "https://example.com"}}}
		if got := applyFilterAdd(ctx, chatID, in); got.Outcome != filterAdded || got.Err != nil {
			t.Fatalf("add = %+v", got)
		}
		if list := filters.GetFiltersList(chatID); len(list) != 2 {
			t.Errorf("filter_list still stale: %v", list)
		}
		if all, err := filters.GetChatFiltersCached(chatID); err != nil || len(all) != 2 {
			t.Errorf("filters_optimized still stale: %d, %v", len(all), err)
		}
		var row models.ChatFilters
		if err := db.DB.Where("chat_id = ? AND keyword = ?", chatID, "fresh").Take(&row).Error; err != nil {
			t.Fatalf("stored row: %v", err)
		}
		if row.FilterReply != "body" || row.FileID != "file-1" || row.MsgType != db.PHOTO || len(row.Buttons) != 1 {
			t.Errorf("stored row = %+v", row)
		}
	})

	t.Run("raced when the insert lands no row", func(t *testing.T) {
		chatID := filterApplyChat(t)
		seams := filterWrites
		seams.Add = func(context.Context, int64, string, string, string, []db.Button, int) (bool, error) {
			return false, nil
		}
		filterApplySeams(t, seams)
		if got := applyFilterAdd(ctx, chatID, filterApplyInput("hello")).Outcome; got != filterRaced {
			t.Errorf("no insert = %v, want raced", got)
		}
	})

	t.Run("save failed with the error", func(t *testing.T) {
		chatID := filterApplyChat(t)
		boom := errors.New("database is down")
		seams := filterWrites
		seams.Add = func(context.Context, int64, string, string, string, []db.Button, int) (bool, error) {
			return false, boom
		}
		filterApplySeams(t, seams)
		got := applyFilterAdd(ctx, chatID, filterApplyInput(secretKeyword))
		if got.Outcome != filterSaveFailed || !errors.Is(got.Err, boom) {
			t.Errorf("failing add = %+v, want save failed with the error", got)
		}
		assertNoKeywordLogged(t)
	})

	for _, failing := range []string{"exists", "count"} {
		t.Run("a failed "+failing+" lookup stores nothing", func(t *testing.T) {
			chatID := filterApplyChat(t)
			boom := errors.New("lookup failed")
			seams := filterWrites
			if failing == "exists" {
				seams.Exists = func(context.Context, int64, string) (bool, error) { return false, boom }
			} else {
				seams.Count = func(context.Context, int64) (int64, error) { return 0, boom }
			}
			filterApplySeams(t, seams)
			got := applyFilterAdd(ctx, chatID, filterApplyInput("price"))
			if got.Outcome != filterSaveFailed || !errors.Is(got.Err, boom) {
				t.Errorf("failing %s lookup = %+v, want save failed with the error", failing, got)
			}
			if filters.DoesFilterExists(chatID, "price") {
				t.Errorf("a filter was stored although the %s lookup failed", failing)
			}
		})
	}

	t.Run("remove drops both caches", func(t *testing.T) {
		chatID := filterApplyChat(t)
		filterApplySeed(t, chatID, "hello", "keep")
		if list := filters.GetFiltersList(chatID); len(list) != 2 {
			t.Fatalf("primed list = %v", list)
		}
		if all, err := filters.GetChatFiltersCached(chatID); err != nil || len(all) != 2 {
			t.Fatalf("primed optimized list = %d, %v", len(all), err)
		}
		if got := applyFilterRemove(ctx, chatID, "HELLO"); got.Outcome != filterRemoved || got.Err != nil {
			t.Fatalf("remove = %+v", got)
		}
		if list := filters.GetFiltersList(chatID); len(list) != 1 || list[0] != "keep" {
			t.Errorf("filter_list after remove = %v", list)
		}
		if all, err := filters.GetChatFiltersCached(chatID); err != nil || len(all) != 1 {
			t.Errorf("filters_optimized after remove = %d, %v", len(all), err)
		}
	})

	t.Run("remove save failed", func(t *testing.T) {
		chatID := filterApplyChat(t)
		boom := errors.New("database is down")
		seams := filterWrites
		seams.Remove = func(context.Context, int64, string) error { return boom }
		filterApplySeams(t, seams)
		got := applyFilterRemove(ctx, chatID, secretKeyword)
		if got.Outcome != filterRemoveSaveFailed || !errors.Is(got.Err, boom) {
			t.Errorf("failing remove = %+v", got)
		}
		assertNoKeywordLogged(t)
	})
}

// TestFilterCommandsWriteThroughApplyFunctions presses the real /filter and /stop entry points with
// counting seams, so a command that bypassed the apply functions would leave the counters at zero.
func TestFilterCommandsWriteThroughApplyFunctions(t *testing.T) {
	chatID := filterApplyChat(t)
	var added, removed int
	seams := filterWrites
	realAdd, realRemove := seams.Add, seams.Remove
	seams.Add = func(ctx context.Context, chat int64, keyword, reply, fileID string, buttons []db.Button, dataType int) (bool, error) {
		added++
		return realAdd(ctx, chat, keyword, reply, fileID, buttons, dataType)
	}
	seams.Remove = func(ctx context.Context, chat int64, keyword string) error {
		removed++
		return realRemove(ctx, chat, keyword)
	}
	filterApplySeams(t, seams)

	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Filter Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	if err := filtersModule.addFilter(bot, newModuleMessageContext(bot, chat, admin, "/filter hello there")); err != ext.EndGroups {
		t.Fatalf("addFilter error = %v", err)
	}
	if err := filtersModule.rmFilter(bot, newModuleMessageContext(bot, chat, admin, "/stop hello")); err != ext.EndGroups {
		t.Fatalf("rmFilter error = %v", err)
	}
	if added != 1 || removed != 1 {
		t.Errorf("seam calls: add %d, remove %d, want 1 and 1", added, removed)
	}
	if filters.DoesFilterExists(chatID, "hello") {
		t.Error("the filter is still stored after /stop")
	}
}

// filterCommandReply runs /filter or /stop as the chat's creator, replying to a text message when
// replyText is not empty, and returns the text of the one sendMessage the command answered with.
func filterCommandReply(t *testing.T, chatID int64, text, replyText string) string {
	t.Helper()
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Filter Chat"}
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	ctx := newModuleMessageContext(bot, chat, admin, text)
	if replyText != "" {
		ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{MessageId: 55, Date: 1, Chat: chat, From: &gotgbot.User{Id: 42, FirstName: "Member"}, Text: replyText}
	}
	if err := filtersModule.addFilter(bot, ctx); err != ext.EndGroups {
		t.Fatalf("addFilter error = %v", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(calls))
	}
	reply, _ := calls[0].Params["text"].(string)
	return reply
}

// TestApplyFilterTooBroad covers A19-2 for filters: a keyword of one code point that is not a symbol
// is refused by the shared check and by /filter, and a one-symbol or two-character keyword is stored.
func TestApplyFilterTooBroad(t *testing.T) {
	withRealLocales(t)
	ctx := context.Background()
	chatID := filterApplyChat(t)
	for _, keyword := range []string{"a", "?", "€", "7"} {
		if got := checkFilterKeyword(ctx, chatID, keyword).Outcome; got != filterKeywordTooBroad {
			t.Errorf("checkFilterKeyword(%q) = %v, want too broad", keyword, got)
		}
		if got := applyFilterAdd(ctx, chatID, filterApplyInput(keyword)).Outcome; got != filterKeywordTooBroad {
			t.Errorf("applyFilterAdd(%q) = %v, want too broad", keyword, got)
		}
	}
	if got := applyFilterAdd(ctx, chatID, filterApplyInput("🙂")).Outcome; got != filterAdded {
		t.Errorf("applyFilterAdd(emoji) = %v, want added", got)
	}

	command := filterApplyChat(t)
	want, err := i18n.MustNewTranslator("uk").GetString("filters_keyword_too_broad")
	if err != nil || want == "" {
		t.Fatalf("locale key filters_keyword_too_broad missing: %v", err)
	}
	if got := filterCommandReply(t, command, "/filter a", "reply body"); got != want {
		t.Errorf("/filter a replied %q, want %q", got, want)
	}
	if filters.DoesFilterExists(command, "a") {
		t.Error("the one-character keyword was stored")
	}
	if got := filterCommandReply(t, command, "/filter ab", "reply body"); got == want {
		t.Error("/filter ab was refused as too broad")
	}
	if !filters.DoesFilterExists(command, "ab") {
		t.Error("the two-character keyword was not stored")
	}
}

// TestFilterCommandRefusesWhenLookupFails presses /filter with a failing existence or count lookup:
// the owner sees the save-failed reply and no filter is stored, where the command used to add it.
func TestFilterCommandRefusesWhenLookupFails(t *testing.T) {
	withRealLocales(t)
	want, err := i18n.MustNewTranslator("uk").GetString("common_settings_save_failed")
	if err != nil || want == "" {
		t.Fatalf("locale key common_settings_save_failed missing: %v", err)
	}
	for _, failing := range []string{"exists", "count"} {
		t.Run(failing, func(t *testing.T) {
			chatID := filterApplyChat(t)
			seams := filterWrites
			if failing == "exists" {
				seams.Exists = func(context.Context, int64, string) (bool, error) { return false, errors.New("lookup failed") }
			} else {
				seams.Count = func(context.Context, int64) (int64, error) { return 0, errors.New("lookup failed") }
			}
			filterApplySeams(t, seams)
			if got := filterCommandReply(t, chatID, "/filter price", "reply body"); got != want {
				t.Errorf("/filter replied %q, want %q", got, want)
			}
			if filters.DoesFilterExists(chatID, "price") {
				t.Error("a filter was stored although the lookup failed")
			}
		})
	}
}

// TestFilterCommandCapPreCheckStays keeps the command's own cap answer at 150 rows, before any
// argument check, with the apply functions' lookups healthy.
func TestFilterCommandCapPreCheckStays(t *testing.T) {
	withRealLocales(t)
	want, err := i18n.MustNewTranslator("uk").GetString("filters_limit_exceeded")
	if err != nil || want == "" {
		t.Fatalf("locale key filters_limit_exceeded missing: %v", err)
	}
	chatID := filterApplyChat(t)
	keywords := make([]string, 0, filtersCapMax)
	for n := 0; n < filtersCapMax; n++ {
		keywords = append(keywords, "word"+strconv.Itoa(n))
	}
	filterApplySeed(t, chatID, keywords...)
	if got := filterCommandReply(t, chatID, "/filter overflow nope", ""); got != want {
		t.Errorf("/filter at the cap replied %q, want %q", got, want)
	}
}
