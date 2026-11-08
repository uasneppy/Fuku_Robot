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

	"github.com/vmihailenco/msgpack/v5"

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

// actionsOf returns the eight actions in rule order.
func actionsOf(row *models.ChatRequirements) [8]int {
	var out [8]int
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
			if got.ChatID != chatID || actionsOf(got) != [8]int{} || got.CheckOnJoin || got.DeleteMessages {
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
		if want := [8]int{0, 0, 0, 3, 0, 0, 0, 0}; actionsOf(got) != want {
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
		if want := [8]int{0, 2, 0, 0, 0, 0, 0, 0}; actionsOf(got) != want {
			t.Errorf("actions after the switch writes = %v, want %v (switches never touch the actions)", actionsOf(got), want)
		}

		// The photo and channel rules have their own columns and touch nothing else.
		if err := SetRuleAction(ctx, chatID, RulePhoto, models.RequirementActionMute); err != nil {
			t.Fatalf("SetRuleAction(photo): %v", err)
		}
		got, err = GetSettings(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if want := [8]int{0, 2, 0, 0, 0, 4, 0, 0}; actionsOf(got) != want {
			t.Errorf("actions after the photo write = %v, want %v (the write must delete the cached read)", actionsOf(got), want)
		}
		if err := SetRuleAction(ctx, chatID, RuleChannel, models.RequirementActionKick); err != nil {
			t.Fatalf("SetRuleAction(channel, 2): %v", err)
		}
		if err := SetRuleAction(ctx, chatID, RuleChannel, models.RequirementActionOff); err != nil {
			t.Fatalf("SetRuleAction(channel, 0): %v", err)
		}
		row, _ = storedRequirements(t, chatID)
		if row.ChannelAction != 0 || row.PhotoAction != 4 || row.UsernameAction != 2 {
			t.Errorf("stored (channel, photo, username) = (%d, %d, %d), want (0, 4, 2): a Go 0 must be written and no write may touch another column",
				row.ChannelAction, row.PhotoAction, row.UsernameAction)
		}

		// The AI spam-name rule has its own column and touches nothing else.
		if err := SetRuleAction(ctx, chatID, RuleAI, models.RequirementActionMute); err != nil {
			t.Fatalf("SetRuleAction(ai): %v", err)
		}
		got, err = GetSettings(ctx, chatID)
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		if want := [8]int{0, 2, 0, 0, 0, 4, 0, 4}; actionsOf(got) != want {
			t.Errorf("actions after the AI write = %v, want %v (the write must delete the cached read)", actionsOf(got), want)
		}
		if err := SetRuleAction(ctx, chatID, RuleAI, models.RequirementActionKick); err != nil {
			t.Fatalf("SetRuleAction(ai, 2): %v", err)
		}
		if err := SetRuleAction(ctx, chatID, RuleAI, models.RequirementActionOff); err != nil {
			t.Fatalf("SetRuleAction(ai, 0): %v", err)
		}
		row, _ = storedRequirements(t, chatID)
		if row.AIAction != 0 || row.PhotoAction != 4 || row.UsernameAction != 2 || row.CheckOnJoin || row.DeleteMessages {
			t.Errorf("stored (ai, photo, username, switches) = (%d, %d, %d, %v, %v), want (0, 4, 2, false, false): a Go 0 must be written and no write may touch another column",
				row.AIAction, row.PhotoAction, row.UsernameAction, row.CheckOnJoin, row.DeleteMessages)
		}
	})

	t.Run("several rules at once", func(t *testing.T) {
		chatID := requirementsChat(t)
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 5)
		for rule, action := range map[Rule]int{
			RuleRussian: models.RequirementActionWarn, RuleChinese: models.RequirementActionMute,
			RulePhoto: models.RequirementActionBan, RuleChannel: models.RequirementActionKick,
			RuleAI: models.RequirementActionWarn,
		} {
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
		if want := [8]int{0, 0, 1, 0, 4, 3, 2, 1}; actionsOf(got) != want {
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
		for _, rule := range []Rule{RulePhoto, RuleChannel, RuleAI} {
			for _, action := range []int{5, -1} {
				if err := SetRuleAction(ctx, chatID, rule, action); !errors.Is(err, ErrActionOutOfRange) {
					t.Errorf("%s action %d: err = %v, want ErrActionOutOfRange", rule, action, err)
				}
			}
		}
		for _, unknown := range []Rule{"nope", "profile_photo"} {
			if err := SetRuleAction(ctx, chatID, unknown, 1); !errors.Is(err, ErrUnknownRule) {
				t.Errorf("unknown rule %q: err = %v, want ErrUnknownRule", unknown, err)
			}
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

	t.Run("an old row", func(t *testing.T) {
		chatID := requirementsChat(t)
		// The SQLite harness builds its table from the model, which has no column defaults,
		// so a row holding only chat_id can be inserted on PostgreSQL alone, where the
		// migration-built schema carries the DEFAULT 0 that makes an old row read as off.
		if db.DB.Name() == "postgres" {
			if err := db.DB.Exec("INSERT INTO chat_requirements (chat_id) VALUES (?)", chatID).Error; err != nil {
				t.Fatalf("insert an old-style row: %v", err)
			}
			got, err := GetSettingsFresh(ctx, chatID)
			if err != nil {
				t.Fatalf("GetSettingsFresh: %v", err)
			}
			if actionsOf(got) != [8]int{} || AnyRuleOn(got) {
				t.Errorf("old row read %v, want all eight rules off", actionsOf(got))
			}
		}

		// A value cached by the previous build has no photo, channel or AI field; msgpack must
		// decode all three as 0 (off), never as a rule that is on.
		type previousBuild struct {
			ChatID         int64 `json:"chat_id,omitempty"`
			LastNameAction int   `json:"last_name_action"`
			UsernameAction int   `json:"username_action"`
			RussianAction  int   `json:"russian_action"`
			ArabicAction   int   `json:"arabic_action"`
			ChineseAction  int   `json:"chinese_action"`
			CheckOnJoin    bool  `json:"check_on_join"`
			DeleteMessages bool  `json:"delete_messages"`
		}
		raw, err := msgpack.Marshal(previousBuild{ChatID: chatID, UsernameAction: models.RequirementActionKick, CheckOnJoin: true})
		if err != nil {
			t.Fatalf("encode the previous build's value: %v", err)
		}
		var decoded models.ChatRequirements
		if err := msgpack.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("decode the previous build's value: %v", err)
		}
		if want := [8]int{0, 2, 0, 0, 0, 0, 0, 0}; actionsOf(&decoded) != want || !decoded.CheckOnJoin {
			t.Errorf("previous build's cached value decoded as %v (join %v), want %v with photo, channel and AI off", actionsOf(&decoded), decoded.CheckOnJoin, want)
		}
	})

	t.Run("rule order", func(t *testing.T) {
		want := []Rule{"last_name", "username", "russian", "arabic", "chinese", "photo", "channel", "ai"}
		if len(Rules) != len(want) {
			t.Fatalf("Rules = %v, want %v", Rules, want)
		}
		for i := range want {
			if Rules[i] != want[i] {
				t.Errorf("Rules[%d] = %q, want %q", i, Rules[i], want[i])
			}
		}
		row := &models.ChatRequirements{LastNameAction: 1, UsernameAction: 2, RussianAction: 3, ArabicAction: 4, PhotoAction: 2, ChannelAction: 3, AIAction: 1}
		if got := actionsOf(row); got != [8]int{1, 2, 3, 4, 0, 2, 3, 1} {
			t.Errorf("ActionOf per rule = %v, want [1 2 3 4 0 2 3 1]", got)
		}
		if !AnyRuleOn(&models.ChatRequirements{ChineseAction: 1}) {
			t.Error("AnyRuleOn is false with Chinese on")
		}
		if !AnyRuleOn(&models.ChatRequirements{PhotoAction: 1}) {
			t.Error("AnyRuleOn is false with only the photo rule on")
		}
		if !AnyRuleOn(&models.ChatRequirements{ChannelAction: 4}) {
			t.Error("AnyRuleOn is false with only the channel rule on")
		}
		if !AnyRuleOn(&models.ChatRequirements{AIAction: 3}) {
			t.Error("AnyRuleOn is false with only the AI rule on")
		}
		if AnyRuleOn(&models.ChatRequirements{}) {
			t.Error("AnyRuleOn is true with every rule off")
		}
	})
}
