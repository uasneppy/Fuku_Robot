//go:build testtools

package modules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// deletionCleanup removes the rule rows and greetings rows of the chats and their cached reads
// before the test and again when it ends, so a test in the shared CI database leaves nothing a
// later one reads.
func deletionCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.DeleteRule{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ScheduledDeletion{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.Chat{})
		for _, id := range chatIDs {
			dbcache.DeleteCache(dbcache.CacheKey("delete_rules", id))
			dbcache.DeleteCache(dbcache.CacheKey("greetings", id))
		}
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// deletionStoredRow reads one rule row straight from the database.
func deletionStoredRow(t *testing.T, chatID int64, kind string) (models.DeleteRule, bool) {
	t.Helper()
	var rows []models.DeleteRule
	if err := db.DB.Where("chat_id = ? AND kind = ?", chatID, kind).Find(&rows).Error; err != nil {
		t.Fatalf("read rule row: %v", err)
	}
	if len(rows) == 0 {
		return models.DeleteRule{}, false
	}
	return rows[0], true
}

func TestApplyDeleteRule(t *testing.T) {
	ctx := context.Background()

	t.Run("row lookups", func(t *testing.T) {
		pinned, ok := deletionRowOf("sn")
		byKind, okKind := deletionRowOfKind(deletion.KindPinned)
		if !ok || !okKind || pinned.Code != "sn" || byKind.Code != pinned.Code || pinned.Kind != deletion.KindPinned {
			t.Fatalf("pinned lookups = %+v (%v) and %+v (%v), want the same row", pinned, ok, byKind, okKind)
		}
		for _, code := range []string{"bw", "bg", "br", "ba", "bm"} {
			byCode, ok := deletionRowOf(code)
			if !ok {
				t.Fatalf("no row for code %q", code)
			}
			again, ok := deletionRowOfKind(byCode.Kind)
			if !ok || again.Code != code {
				t.Errorf("kind %q looked up as %+v (%v), want code %q", byCode.Kind, again, ok, code)
			}
			// The apply function is called with the kind of the row the code lookup returned.
			chatID := uniqueModuleChatID()
			deletionCleanup(t, chatID)
			if result := applyDeleteRule(ctx, chatID, byCode.Kind, deletionSet(byCode.Presets[0])); result.Outcome != deletionApplied {
				t.Errorf("code %q: outcome = %v, want applied", code, result.Outcome)
			}
		}
		if _, ok := deletionRowOf("zz"); ok {
			t.Error("an unknown code returned a row")
		}
		if _, ok := deletionRowOfKind("service:nothing"); ok {
			t.Error("an unknown kind returned a row")
		}
	})

	t.Run("set", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		// A cached read first: the apply must drop the key so the next cached read sees the write.
		if rules, err := deletion.GetRules(ctx, chatID); err != nil || len(rules.Delays) != 0 {
			t.Fatalf("GetRules before = %+v, %v, want no rules", rules, err)
		}
		row, _ := deletionRowOf("sn")
		result := applyDeleteRule(ctx, chatID, row.Kind, deletionSet(600))
		if result.Outcome != deletionApplied || result.Err != nil || result.Rules == nil || result.Rules.Delays[deletion.KindPinned] != 600 {
			t.Fatalf("result = %+v, want applied with 600 read back", result)
		}
		cached, err := deletion.GetRules(ctx, chatID)
		if err != nil || cached.Delays[deletion.KindPinned] != 600 {
			t.Errorf("the cached read after the write = %+v, %v, want 600 (the key was dropped)", cached, err)
		}
		var count int64
		if err := db.DB.Model(&models.DeleteRule{}).Where("chat_id = ?", chatID).Count(&count).Error; err != nil || count != 1 {
			t.Errorf("rows = %d, %v, want exactly one", count, err)
		}
		// A pinned notice is routed to the kind, and the stored delay is what it gets.
		msg := &gotgbot.Message{PinnedMessage: gotgbot.Message{MessageId: 7}}
		routed, ok := deletionServiceKindOf(msg)
		if !ok {
			t.Fatal("a pinned notice matched no row")
		}
		if delay, on := deletionEffectiveDelay(routed.Kind, result.Rules, false); delay != 600 || !on {
			t.Errorf("effective delay of a pinned notice = (%d, %v), want (600, true)", delay, on)
		}
	})

	t.Run("zero", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		result := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(0))
		if result.Outcome != deletionApplied || result.Rules == nil {
			t.Fatalf("result = %+v, want applied", result)
		}
		if delay, present := result.Rules.Delays[deletion.KindPinned]; !present || delay != 0 {
			t.Errorf("read back = %d, present %v, want a stored Go 0 (delete now)", delay, present)
		}
		if row, has := deletionStoredRow(t, chatID, deletion.KindPinned); !has || row.DelaySecs != 0 {
			t.Errorf("stored row = %+v (%v), want delay 0", row, has)
		}
	})

	t.Run("remove", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		if r := applyDeleteRule(ctx, chatID, deletion.KindBotRules, deletionSet(3600)); r.Outcome != deletionApplied {
			t.Fatalf("set: %+v", r)
		}
		result := applyDeleteRule(ctx, chatID, deletion.KindBotRules, deletionRemove)
		if result.Outcome != deletionApplied || result.Rules == nil {
			t.Fatalf("remove result = %+v, want applied", result)
		}
		if _, present := result.Rules.Delays[deletion.KindBotRules]; present {
			t.Error("the rule is still read back after a remove")
		}
		if _, has := deletionStoredRow(t, chatID, deletion.KindBotRules); has {
			t.Error("the row survived a remove")
		}
		if again := applyDeleteRule(ctx, chatID, deletion.KindBotRules, deletionRemove); again.Outcome != deletionUnchanged {
			t.Errorf("second remove outcome = %v, want unchanged", again.Outcome)
		}
	})

	t.Run("unchanged", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		if r := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(600)); r.Outcome != deletionApplied {
			t.Fatalf("first set: %+v", r)
		}
		// Age the row so a rewrite would visibly move updated_at.
		old := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
		if err := db.DB.Model(&models.DeleteRule{}).Where("chat_id = ?", chatID).Update("updated_at", old).Error; err != nil {
			t.Fatalf("age the row: %v", err)
		}
		dbcache.DeleteCache(dbcache.CacheKey("delete_rules", chatID))
		again := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(600))
		if again.Outcome != deletionUnchanged || again.Rules == nil || again.Rules.Delays[deletion.KindPinned] != 600 {
			t.Fatalf("second set = %+v, want unchanged with the stored state", again)
		}
		row, _ := deletionStoredRow(t, chatID, deletion.KindPinned)
		if !row.UpdatedAt.Equal(old) {
			t.Errorf("updated_at moved from %v to %v: an unchanged press wrote", old, row.UpdatedAt)
		}
		// A different value is a write.
		if r := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(60)); r.Outcome != deletionApplied || r.Rules.Delays[deletion.KindPinned] != 60 {
			t.Errorf("a changed value = %+v, want applied with 60", r)
		}
	})

	t.Run("refused", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		writes := 0
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
			Set: func(context.Context, int64, string, int) error { writes++; return nil },
			Remove: func(context.Context, int64, string) error {
				writes++
				return nil
			},
		})
		lookups := 0
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) {
			lookups++
			return &deletion.Rules{ChatID: chatID, Delays: map[string]int{}}, nil
		})
		for _, c := range []struct {
			name   string
			kind   string
			target deletionTarget
		}{
			{"an unknown kind", "service:nothing", deletionSet(60)},
			{"an empty kind", "", deletionSet(60)},
			{"a delay of -1", deletion.KindPinned, deletionSet(-1)},
			{"a delay of 86401", deletion.KindPinned, deletionSet(86401)},
			{"joined", deletion.KindJoined, deletionSet(60)},
			{"left", deletion.KindLeft, deletionSet(60)},
			{"a remove of joined", deletion.KindJoined, deletionRemove},
		} {
			result := applyDeleteRule(ctx, chatID, c.kind, c.target)
			if result.Outcome != deletionRefused || result.Err == nil {
				t.Errorf("%s: result = %+v, want refused with an error", c.name, result)
			}
		}
		if writes != 0 || lookups != 0 {
			t.Errorf("a refused call reached a seam: %d writes, %d lookups", writes, lookups)
		}
	})

	t.Run("save failed", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		if r := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(600)); r.Outcome != deletionApplied {
			t.Fatalf("seed: %+v", r)
		}
		boom := errors.New("write refused")
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
			Set:    func(context.Context, int64, string, int) error { return boom },
			Remove: func(context.Context, int64, string) error { return boom },
		})
		for _, target := range []deletionTarget{deletionSet(60), deletionRemove} {
			result := applyDeleteRule(ctx, chatID, deletion.KindPinned, target)
			if result.Outcome != deletionSaveFailed || !errors.Is(result.Err, boom) {
				t.Errorf("target %+v: result = %+v, want save failed carrying the error", target, result)
			}
		}
		if row, has := deletionStoredRow(t, chatID, deletion.KindPinned); !has || row.DelaySecs != 600 {
			t.Errorf("stored row = %+v (%v), want the old 600", row, has)
		}
	})

	t.Run("unreadable state", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		writes := 0
		mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
			Set:    func(context.Context, int64, string, int) error { writes++; return nil },
			Remove: func(context.Context, int64, string) error { writes++; return nil },
		})
		boom := errors.New("read refused")
		mreqSwap(t, &deletionRulesLookup, func(context.Context, int64) (*deletion.Rules, error) { return nil, boom })
		result := applyDeleteRule(ctx, chatID, deletion.KindPinned, deletionSet(60))
		if result.Outcome != deletionSaveFailed || !errors.Is(result.Err, boom) || writes != 0 {
			t.Errorf("result = %+v with %d writes, want a save failure and no write", result, writes)
		}
	})
}

// deletionSeedJoinLeft stores the shared flag and the two delay rows directly, as a chat that was
// configured earlier would have them. A negative delay stores no row.
func deletionSeedJoinLeft(t *testing.T, chatID int64, flag bool, joined, left int) {
	t.Helper()
	if err := greetings.SetShouldCleanService(chatID, flag); err != nil {
		t.Fatalf("seed the flag: %v", err)
	}
	for kind, delay := range map[string]int{deletion.KindJoined: joined, deletion.KindLeft: left} {
		if delay < 0 {
			continue
		}
		if err := deletion.SetRule(context.Background(), chatID, kind, delay); err != nil {
			t.Fatalf("seed the %s rule: %v", kind, err)
		}
	}
}

// deletionStoredFlag reads the shared /cleanservice flag straight from the database.
func deletionStoredFlag(t *testing.T, chatID int64) bool {
	t.Helper()
	var row models.GreetingSettings
	if err := db.DB.Where("chat_id = ?", chatID).First(&row).Error; err != nil {
		t.Fatalf("read the greetings row: %v", err)
	}
	return row.ShouldCleanService
}

// deletionCountHook counts how often the transaction of ApplyJoinLeftRule and
// ApplyCleanServiceFlag ran, so a test can prove a call wrote nothing (no transaction opened).
func deletionCountHook(t *testing.T, fail error) *int {
	t.Helper()
	calls := 0
	t.Cleanup(deletion.SetApplyHookForTest(func() error { calls++; return fail }))
	return &calls
}

func TestApplyCleanServiceRule(t *testing.T) {
	ctx := context.Background()

	t.Run("keep removes both rows and turns the flag off", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, true, 600, 30)
		result := applyCleanServiceRule(ctx, chatID, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftKeep})
		if result.Outcome != deletionApplied || result.Err != nil || result.Rules == nil {
			t.Fatalf("result = %+v, want applied with the rules read back", result)
		}
		if deletionStoredFlag(t, chatID) || result.CleanService {
			t.Error("the flag is still on after Keep")
		}
		if len(result.Rules.Delays) != 0 {
			t.Errorf("rules read back = %v, want both rows gone", result.Rules.Delays)
		}
		if result.On || result.Delay != 0 {
			t.Errorf("effective state = (%d, %v), want Keep", result.Delay, result.On)
		}
		for _, kind := range []string{deletion.KindJoined, deletion.KindLeft} {
			if _, has := deletionStoredRow(t, chatID, kind); has {
				t.Errorf("the %s row survived Keep", kind)
			}
		}
	})

	t.Run("now removes only this kind's row", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, false, 600, 30)
		result := applyCleanServiceRule(ctx, chatID, deletion.KindLeft, deletion.JoinLeftState{Mode: deletion.JoinLeftNow})
		if result.Outcome != deletionApplied || result.Rules == nil {
			t.Fatalf("result = %+v, want applied", result)
		}
		if !deletionStoredFlag(t, chatID) || !result.CleanService {
			t.Error("the flag is off after Now")
		}
		if _, has := deletionStoredRow(t, chatID, deletion.KindLeft); has {
			t.Error("the left row survived Now")
		}
		if row, has := deletionStoredRow(t, chatID, deletion.KindJoined); !has || row.DelaySecs != 600 {
			t.Errorf("joined row = %+v (%v), want the stored 600 to stay", row, has)
		}
		if result.Delay != 0 || !result.On {
			t.Errorf("effective left = (%d, %v), want Delete now", result.Delay, result.On)
		}
	})

	t.Run("after turns the flag on and the other kind reads Delete now", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, false, -1, -1)
		result := applyCleanServiceRule(ctx, chatID, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 600})
		if result.Outcome != deletionApplied || result.Rules == nil || result.Delay != 600 || !result.On {
			t.Fatalf("result = %+v, want applied with joined at 600", result)
		}
		if row, has := deletionStoredRow(t, chatID, deletion.KindJoined); !has || row.DelaySecs != 600 || !deletionStoredFlag(t, chatID) {
			t.Errorf("joined row = %+v (%v) or the flag is off", row, has)
		}
		// The documented coupling (D-02): the left kind has no row and the shared flag is on now.
		if delay, on := deletionEffectiveDelay(deletion.KindLeft, result.Rules, result.CleanService); delay != 0 || !on {
			t.Errorf("effective left = (%d, %v), want Delete now through the shared flag", delay, on)
		}
	})

	t.Run("a repeated press writes nothing", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, true, 600, -1)
		calls := deletionCountHook(t, nil)
		for _, c := range []struct {
			name  string
			kind  string
			state deletion.JoinLeftState
		}{
			{"After 600 on joined", deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 600}},
			{"Now on left", deletion.KindLeft, deletion.JoinLeftState{Mode: deletion.JoinLeftNow}},
		} {
			result := applyCleanServiceRule(ctx, chatID, c.kind, c.state)
			if result.Outcome != deletionUnchanged || result.Rules == nil {
				t.Errorf("%s: result = %+v, want unchanged with the stored state", c.name, result)
			}
		}
		if *calls != 0 {
			t.Errorf("an unchanged press opened %d transactions", *calls)
		}

		// Keep on a chat that is already Keep with no rows is unchanged too, but a stale row makes it a write.
		other := uniqueModuleChatID()
		deletionCleanup(t, other)
		deletionSeedJoinLeft(t, other, false, -1, -1)
		keep := deletion.JoinLeftState{Mode: deletion.JoinLeftKeep}
		if r := applyCleanServiceRule(ctx, other, deletion.KindJoined, keep); r.Outcome != deletionUnchanged {
			t.Errorf("Keep on a clean Keep chat = %+v, want unchanged", r)
		}
		deletionSeedJoinLeft(t, other, false, -1, 30)
		if r := applyCleanServiceRule(ctx, other, deletion.KindJoined, keep); r.Outcome != deletionApplied {
			t.Errorf("Keep with a stale left row = %+v, want applied (the row is removed)", r)
		}
		if _, has := deletionStoredRow(t, other, deletion.KindLeft); has {
			t.Error("a stale row survived Keep")
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, false, -1, 30)
		calls := deletionCountHook(t, nil)
		lookups := 0
		mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) {
			lookups++
			return &models.GreetingSettings{}, nil
		})
		for _, c := range []struct {
			name  string
			kind  string
			state deletion.JoinLeftState
		}{
			{"a delay of 0", deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 0}},
			{"a negative delay", deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: -5}},
			{"a delay of 86401", deletion.KindLeft, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 86401}},
			{"a kind that is not joined or left", deletion.KindPinned, deletion.JoinLeftState{Mode: deletion.JoinLeftNow}},
			{"a bot kind", deletion.KindBotRules, deletion.JoinLeftState{Mode: deletion.JoinLeftKeep}},
			{"no mode", deletion.KindJoined, deletion.JoinLeftState{}},
		} {
			if result := applyCleanServiceRule(ctx, chatID, c.kind, c.state); result.Outcome != deletionRefused || result.Err == nil {
				t.Errorf("%s: result = %+v, want refused with an error", c.name, result)
			}
		}
		if *calls != 0 || lookups != 0 {
			t.Errorf("a refused call reached a seam: %d transactions, %d reads", *calls, lookups)
		}
		if deletionStoredFlag(t, chatID) {
			t.Error("a refused call turned the flag on")
		}
	})

	t.Run("both caches are dropped", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, false, -1, -1)
		// Cached reads first: the apply must drop both keys.
		if rules, err := deletion.GetRules(ctx, chatID); err != nil || len(rules.Delays) != 0 {
			t.Fatalf("GetRules before = %+v, %v", rules, err)
		}
		if greetings.GetGreetingSettings(chatID).ShouldCleanService {
			t.Fatal("the flag reads on before the write")
		}
		result := applyCleanServiceRule(ctx, chatID, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 60})
		if result.Outcome != deletionApplied {
			t.Fatalf("result = %+v", result)
		}
		if rules, err := deletion.GetRules(ctx, chatID); err != nil || rules.Delays[deletion.KindJoined] != 60 {
			t.Errorf("cached rules after = %+v, %v, want joined 60", rules, err)
		}
		if !greetings.GetGreetingSettings(chatID).ShouldCleanService {
			t.Error("the cached greetings read after the write still has the flag off")
		}
	})

	t.Run("a failed transaction changes nothing", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		deletionSeedJoinLeft(t, chatID, false, -1, 30)
		boom := errors.New("second step failed")
		calls := deletionCountHook(t, boom)
		result := applyCleanServiceRule(ctx, chatID, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftAfter, DelaySecs: 600})
		if result.Outcome != deletionSaveFailed || !errors.Is(result.Err, boom) {
			t.Fatalf("result = %+v, want save failed carrying the error", result)
		}
		if *calls != 1 {
			t.Errorf("hook ran %d times, want once", *calls)
		}
		if deletionStoredFlag(t, chatID) {
			t.Error("the flag changed although the transaction failed")
		}
		if _, has := deletionStoredRow(t, chatID, deletion.KindJoined); has {
			t.Error("a joined row was written although the transaction failed")
		}
		if row, has := deletionStoredRow(t, chatID, deletion.KindLeft); !has || row.DelaySecs != 30 {
			t.Errorf("left row = %+v (%v), want the old 30", row, has)
		}
	})

	t.Run("an unreadable state writes nothing", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		deletionCleanup(t, chatID)
		calls := deletionCountHook(t, nil)
		boom := errors.New("read refused")
		mreqSwap(t, &deletionFlagLookup, func(context.Context, int64) (*models.GreetingSettings, error) { return nil, boom })
		result := applyCleanServiceRule(ctx, chatID, deletion.KindJoined, deletion.JoinLeftState{Mode: deletion.JoinLeftNow})
		if result.Outcome != deletionSaveFailed || !errors.Is(result.Err, boom) || *calls != 0 {
			t.Errorf("result = %+v with %d transactions, want a save failure and no write", result, *calls)
		}
	})
}
