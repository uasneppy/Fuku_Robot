//go:build testtools

package memberreq

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// uniqueRequirementChatID returns a random large negative chat ID, so tests sharing one
// database never collide.
func uniqueRequirementChatID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("uniqueRequirementChatID: crypto/rand failed: " + err.Error())
	}
	return -1000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x7fffffffffffffff)
}

// requirementsChat returns a fresh chat ID whose settings row, holds and cached settings
// are removed when the test ends.
func requirementsChat(t *testing.T) int64 {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatID := uniqueRequirementChatID()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatRequirements{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatRequirementHold{})
		cache.DeleteCache(cache.CacheKey("member_requirements", chatID))
		cache.ResetLocalForTest()
	})
	return chatID
}

func storedRequirements(t *testing.T, chatID int64) (models.ChatRequirements, bool) {
	t.Helper()
	var rows []models.ChatRequirements
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the settings row: %v", err)
	}
	if len(rows) == 0 {
		return models.ChatRequirements{}, false
	}
	return rows[0], true
}

// actionsOf returns the five actions in rule order.
func actionsOf(row *models.ChatRequirements) [5]int {
	var out [5]int
	for i, rule := range Rules {
		out[i] = ActionOf(row, rule)
	}
	return out
}

func TestChatRequirementsSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("no row", func(t *testing.T) {
		chatID := requirementsChat(t)
		for name, read := range map[string]func(context.Context, int64) (*models.ChatRequirements, error){
			"cached": GetSettings,
			"fresh":  GetSettingsFresh,
		} {
			got, err := read(ctx, chatID)
			if err != nil {
				t.Fatalf("%s read: %v", name, err)
			}
			if got.ChatID != chatID || actionsOf(got) != [5]int{} || got.CheckOnJoin || got.DeleteMessages {
				t.Errorf("%s read = %+v, want every rule off and both switches off for chat %d", name, got, chatID)
			}
			if AnyRuleOn(got) {
				t.Errorf("%s read: AnyRuleOn is true for a chat with no row", name)
			}
		}
		if _, ok := storedRequirements(t, chatID); ok {
			t.Error("reading a chat with no row created one")
		}
	})

	t.Run("one column per write", func(t *testing.T) {
		chatID := requirementsChat(t)
		if _, err := GetSettings(ctx, chatID); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if err := SetRuleAction(ctx, chatID, RuleArabic, models.RequirementActionBan); err != nil {
			t.Fatalf("SetRuleAction: %v", err)
		}
		got, err := GetSettings(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if want := [5]int{0, 0, 0, 3, 0}; actionsOf(got) != want {
			t.Errorf("actions = %v, want %v (the write must delete the cached read)", actionsOf(got), want)
		}
		if !AnyRuleOn(got) {
			t.Error("AnyRuleOn is false with Arabic on")
		}

		if err := SetRuleAction(ctx, chatID, RuleArabic, models.RequirementActionMute); err != nil {
			t.Fatalf("SetRuleAction(4): %v", err)
		}
		if err := SetRuleAction(ctx, chatID, RuleArabic, models.RequirementActionOff); err != nil {
			t.Fatalf("SetRuleAction(0): %v", err)
		}
		row, _ := storedRequirements(t, chatID)
		if row.ArabicAction != 0 {
			t.Errorf("stored arabic_action = %d, want the 0 that was written", row.ArabicAction)
		}

		if err := SetRuleAction(ctx, chatID, RuleUsername, models.RequirementActionKick); err != nil {
			t.Fatalf("SetRuleAction(username): %v", err)
		}
		for _, on := range []bool{true, false} {
			if err := SetCheckOnJoin(ctx, chatID, on); err != nil {
				t.Fatalf("SetCheckOnJoin(%v): %v", on, err)
			}
			if err := SetDeleteMessages(ctx, chatID, on); err != nil {
				t.Fatalf("SetDeleteMessages(%v): %v", on, err)
			}
			row, _ = storedRequirements(t, chatID)
			if row.CheckOnJoin != on || row.DeleteMessages != on {
				t.Errorf("switches = (%v, %v), want both %v", row.CheckOnJoin, row.DeleteMessages, on)
			}
		}
		got, err = GetSettingsFresh(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettingsFresh: %v", err)
		}
		if want := [5]int{0, 2, 0, 0, 0}; actionsOf(got) != want {
			t.Errorf("actions after the switch writes = %v, want %v (switches never touch the actions)", actionsOf(got), want)
		}
	})

	t.Run("two rules at once", func(t *testing.T) {
		chatID := requirementsChat(t)
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for rule, action := range map[Rule]int{RuleRussian: models.RequirementActionWarn, RuleChinese: models.RequirementActionMute} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- SetRuleAction(ctx, chatID, rule, action)
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("a concurrent SetRuleAction failed: %v", err)
			}
		}
		got, err := GetSettingsFresh(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettingsFresh: %v", err)
		}
		if want := [5]int{0, 0, 1, 0, 4}; actionsOf(got) != want {
			t.Errorf("actions = %v, want %v", actionsOf(got), want)
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := requirementsChat(t)
		for _, action := range []int{5, -1} {
			if err := SetRuleAction(ctx, chatID, RuleUsername, action); !errors.Is(err, ErrActionOutOfRange) {
				t.Errorf("action %d: err = %v, want ErrActionOutOfRange", action, err)
			}
		}
		if err := SetRuleAction(ctx, chatID, Rule("nope"), 1); !errors.Is(err, ErrUnknownRule) {
			t.Errorf("unknown rule: err = %v, want ErrUnknownRule", err)
		}
		if _, ok := storedRequirements(t, chatID); ok {
			t.Error("a refused write created a row")
		}
	})

	t.Run("the CHECKs", func(t *testing.T) {
		chatID := requirementsChat(t)
		if err := SetRuleAction(ctx, chatID, RuleLastName, models.RequirementActionWarn); err != nil {
			t.Fatalf("seed row: %v", err)
		}
		for _, rule := range Rules {
			for _, bad := range []int{5, -1} {
				err := db.DB.Exec(fmt.Sprintf("UPDATE chat_requirements SET %s_action = ? WHERE chat_id = ?", rule), bad, chatID).Error
				if err == nil {
					t.Errorf("%s_action = %d was accepted by the database", rule, bad)
					continue
				}
				msg := err.Error()
				if !strings.Contains(msg, "SQLSTATE 23514") && !strings.Contains(msg, "CHECK constraint failed") {
					t.Errorf("%s_action = %d: error %q is not a CHECK violation", rule, bad, msg)
				}
			}
		}
	})

	t.Run("fresh read", func(t *testing.T) {
		chatID := requirementsChat(t)
		if err := SetRuleAction(ctx, chatID, RuleLastName, models.RequirementActionWarn); err != nil {
			t.Fatalf("seed row: %v", err)
		}
		if _, err := GetSettings(ctx, chatID); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if err := db.DB.Exec("UPDATE chat_requirements SET last_name_action = 3 WHERE chat_id = ?", chatID).Error; err != nil {
			t.Fatalf("direct update: %v", err)
		}
		fresh, err := GetSettingsFresh(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettingsFresh: %v", err)
		}
		if fresh.LastNameAction != 3 {
			t.Errorf("fresh last_name_action = %d, want 3", fresh.LastNameAction)
		}
		cached, err := GetSettings(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if cached.LastNameAction != 1 {
			t.Errorf("cached last_name_action = %d, want the cached 1 until the key is deleted", cached.LastNameAction)
		}
		cache.DeleteCache(cache.CacheKey("member_requirements", chatID))
		cached, err = GetSettings(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if cached.LastNameAction != 3 {
			t.Errorf("cached last_name_action after DeleteCache = %d, want 3", cached.LastNameAction)
		}
	})

	t.Run("rule order", func(t *testing.T) {
		want := []Rule{"last_name", "username", "russian", "arabic", "chinese"}
		if len(Rules) != len(want) {
			t.Fatalf("Rules = %v, want %v", Rules, want)
		}
		for i := range want {
			if Rules[i] != want[i] {
				t.Errorf("Rules[%d] = %q, want %q", i, Rules[i], want[i])
			}
		}
		row := &models.ChatRequirements{LastNameAction: 1, UsernameAction: 2, RussianAction: 3, ArabicAction: 4}
		if got := actionsOf(row); got != [5]int{1, 2, 3, 4, 0} {
			t.Errorf("ActionOf per rule = %v, want [1 2 3 4 0]", got)
		}
		if !AnyRuleOn(&models.ChatRequirements{ChineseAction: 1}) {
			t.Error("AnyRuleOn is false with Chinese on")
		}
		if AnyRuleOn(&models.ChatRequirements{}) {
			t.Error("AnyRuleOn is true with every rule off")
		}
	})
}
