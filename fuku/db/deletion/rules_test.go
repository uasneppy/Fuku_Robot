//go:build testtools

package deletion

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilsCache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// uniqueDeletionChatID returns a random large negative chat ID, so tests sharing one
// database never collide.
func uniqueDeletionChatID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("uniqueDeletionChatID: crypto/rand failed: " + err.Error())
	}
	return -1000000000000 - int64(binary.BigEndian.Uint64(buf[:])&0x7fffffffffffffff)
}

// deletionChat returns a fresh chat ID whose rule rows, queue rows, greetings row and cached
// reads are removed when the test ends.
func deletionChat(t *testing.T) int64 {
	t.Helper()
	utilsCache.SetupTestMemoryMarshaler(t)
	chatID := uniqueDeletionChatID()
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.DeleteRule{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.ScheduledDeletion{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetingSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.StaffGroup{})
		cache.DeleteCache(cache.CacheKey("delete_rules", chatID))
		cache.DeleteCache(cache.CacheKey("greetings", chatID))
		cache.ResetLocalForTest()
	})
	return chatID
}

// storedRules reads the rule rows of a chat straight from the database.
func storedRules(t *testing.T, chatID int64) map[string]int {
	t.Helper()
	var rows []models.DeleteRule
	if err := db.DB.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		t.Fatalf("read the rule rows: %v", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Kind] = row.DelaySecs
	}
	return out
}

func TestDeleteRules(t *testing.T) {
	ctx := context.Background()

	t.Run("no row", func(t *testing.T) {
		chatID := deletionChat(t)
		for name, read := range map[string]func(context.Context, int64) (*Rules, error){
			"cached": GetRules,
			"fresh":  GetRulesFresh,
		} {
			got, err := read(ctx, chatID)
			if err != nil {
				t.Fatalf("%s read: %v", name, err)
			}
			if got == nil || got.Delays == nil || len(got.Delays) != 0 {
				t.Fatalf("%s read of a chat with no row = %+v, want an empty non-nil map", name, got)
			}
		}
		if n := len(storedRules(t, chatID)); n != 0 {
			t.Fatalf("a read created %d rows, want 0", n)
		}
	})

	t.Run("set and read", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := SetRule(ctx, chatID, KindPinned, 600); err != nil {
			t.Fatalf("SetRule: %v", err)
		}
		got, err := GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules: %v", err)
		}
		if delay, ok := got.Delays[KindPinned]; !ok || delay != 600 {
			t.Fatalf("pinned delay = %d (present %v), want 600", delay, ok)
		}

		// A delay of 0 is Delete now, distinct from no row.
		if err := SetRule(ctx, chatID, KindPinned, 0); err != nil {
			t.Fatalf("SetRule 0: %v", err)
		}
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules after 0: %v", err)
		}
		if delay, ok := got.Delays[KindPinned]; !ok || delay != 0 {
			t.Fatalf("pinned delay = %d (present %v), want a stored 0 (the write must delete the cached read)", delay, ok)
		}
		if stored := storedRules(t, chatID); len(stored) != 1 {
			t.Fatalf("stored rules = %v, want one row", stored)
		}

		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatalf("RemoveRule: %v", err)
		}
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatalf("GetRules after remove: %v", err)
		}
		if _, ok := got.Delays[KindPinned]; ok {
			t.Fatal("pinned is still present after RemoveRule")
		}
		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatalf("RemoveRule of an absent kind: %v", err)
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := deletionChat(t)
		cases := []struct {
			name  string
			kind  string
			delay int
			want  error
		}{
			{"unknown kind", "service:nope", 60, ErrUnknownKind},
			{"empty kind", "", 60, ErrUnknownKind},
			{"negative delay", KindPinned, -1, ErrDelayOutOfRange},
			{"delay over a day", KindPinned, MaxDelaySecs + 1, ErrDelayOutOfRange},
		}
		for _, tc := range cases {
			if err := SetRule(ctx, chatID, tc.kind, tc.delay); !errors.Is(err, tc.want) {
				t.Errorf("%s: SetRule error = %v, want %v", tc.name, err, tc.want)
			}
		}
		if err := RemoveRule(ctx, chatID, "service:nope"); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("RemoveRule of an unknown kind = %v, want ErrUnknownKind", err)
		}
		if err := SetRule(ctx, chatID, KindPinned, MaxDelaySecs); err != nil {
			t.Errorf("the largest delay is refused: %v", err)
		}
		if err := RemoveRule(ctx, chatID, KindPinned); err != nil {
			t.Fatal(err)
		}
		if n := len(storedRules(t, chatID)); n != 0 {
			t.Fatalf("%d rows exist after refused writes, want 0", n)
		}
	})

	t.Run("member kinds are checked by value", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := SetRule(ctx, chatID, KindMemberEdited, 60); err != nil {
			t.Fatalf("SetRule(edited, 60): %v", err)
		}
		got, err := GetRules(ctx, chatID)
		if err != nil || got.Delays[KindMemberEdited] != 60 {
			t.Fatalf("read back = %+v, %v, want 60", got, err)
		}
		for _, tc := range []struct {
			kind  string
			delay int
		}{{KindMemberEdited, 45}, {KindMemberCommands, 60}, {KindMemberSelfDestruct, 30}} {
			if err := SetRule(ctx, chatID, tc.kind, tc.delay); !errors.Is(err, ErrDelayOutOfRange) {
				t.Errorf("SetRule(%s, %d) = %v, want ErrDelayOutOfRange", tc.kind, tc.delay, err)
			}
		}
		if stored := storedRules(t, chatID); len(stored) != 1 {
			t.Fatalf("stored rules = %v, want only the edited row (refused writes create none)", stored)
		}
		if err := SetRule(ctx, chatID, KindMemberCommands, 0); err != nil {
			t.Fatalf("SetRule(commands, 0): %v", err)
		}
		got, err = GetRules(ctx, chatID)
		if delay, ok := got.Delays[KindMemberCommands]; err != nil || !ok || delay != 0 {
			t.Fatalf("commands = %d (present %v, %v), want a stored 0", delay, ok, err)
		}
	})

	t.Run("removing a self-destruct rule drops its pending rows only", func(t *testing.T) {
		chatID, other := deletionChat(t), deletionChat(t)
		now := time.Now().UTC().Truncate(time.Second)
		if err := SetRule(ctx, chatID, KindMemberSelfDestruct, 60); err != nil {
			t.Fatal(err)
		}
		rows := []models.ScheduledDeletion{
			{ChatID: chatID, MessageID: 1, Kind: KindMemberSelfDestruct, SentAt: now, DeleteAt: now.Add(time.Hour), State: models.ScheduledDeletionPending, CreatedAt: now},
			{ChatID: chatID, MessageID: 2, Kind: KindMemberSelfDestruct, SentAt: now, DeleteAt: now.Add(time.Hour), State: models.ScheduledDeletionActing, ClaimToken: "tok", CreatedAt: now},
			{ChatID: chatID, MessageID: 3, Kind: KindMemberEdited, SentAt: now, DeleteAt: now, State: models.ScheduledDeletionPending, CreatedAt: now},
			{ChatID: other, MessageID: 1, Kind: KindMemberSelfDestruct, SentAt: now, DeleteAt: now.Add(time.Hour), State: models.ScheduledDeletionPending, CreatedAt: now},
		}
		if err := db.DB.Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if err := RemoveRule(ctx, chatID, KindMemberSelfDestruct); err != nil {
			t.Fatal(err)
		}
		left := map[int64]bool{}
		for _, row := range storedQueue(t, chatID) {
			left[row.MessageID] = true
		}
		if left[1] || !left[2] || !left[3] {
			t.Errorf("rows left = %v, want the acting row 2 and the other kind's row 3 only", left)
		}
		if n := len(storedQueue(t, other)); n != 1 {
			t.Errorf("another chat has %d rows, want its 1 untouched", n)
		}
	})

	t.Run("chats are independent", func(t *testing.T) {
		a, b := deletionChat(t), deletionChat(t)
		if err := SetRule(ctx, a, KindTitle, 30); err != nil {
			t.Fatal(err)
		}
		if err := SetRule(ctx, b, KindTitle, 90); err != nil {
			t.Fatal(err)
		}
		gotA, errA := GetRules(ctx, a)
		gotB, errB := GetRules(ctx, b)
		if errA != nil || errB != nil {
			t.Fatalf("reads: %v %v", errA, errB)
		}
		if gotA.Delays[KindTitle] != 30 || gotB.Delays[KindTitle] != 90 {
			t.Fatalf("delays = %d and %d, want 30 and 90", gotA.Delays[KindTitle], gotB.Delays[KindTitle])
		}
	})

	t.Run("served from the cache", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := SetRule(ctx, chatID, KindBoost, 45); err != nil {
			t.Fatal(err)
		}
		if _, err := GetRules(ctx, chatID); err != nil {
			t.Fatal(err)
		}
		// Change the row behind the cache's back: the cached read must still answer 45.
		if err := db.DB.Model(&models.DeleteRule{}).Where("chat_id = ? AND kind = ?", chatID, KindBoost).
			Update("delay_secs", 99).Error; err != nil {
			t.Fatal(err)
		}
		cache.ResetLocalForTest()
		got, err := GetRules(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Delays[KindBoost] != 45 {
			t.Fatalf("cached read = %d, want the cached 45, not the database's 99", got.Delays[KindBoost])
		}
		fresh, err := GetRulesFresh(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.Delays[KindBoost] != 99 {
			t.Fatalf("fresh read = %d, want 99", fresh.Delays[KindBoost])
		}
		cache.DeleteCache(cache.CacheKey("delete_rules", chatID))
		got, err = GetRules(ctx, chatID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Delays[KindBoost] != 99 {
			t.Fatalf("read after DeleteCache = %d, want 99", got.Delays[KindBoost])
		}
	})
}

func TestKindRegistry(t *testing.T) {
	kinds := Kinds()
	if len(kinds) != 20 {
		t.Fatalf("Kinds() has %d entries, want 20", len(kinds))
	}
	seen := map[string]bool{}
	members := 0
	for _, kind := range kinds {
		if seen[kind] {
			t.Errorf("kind %q is listed twice", kind)
		}
		seen[kind] = true
		if !ValidKind(kind) {
			t.Errorf("registered kind %q is not valid", kind)
		}
		classes := 0
		for _, is := range []bool{IsServiceKind(kind), IsBotKind(kind), IsMemberKind(kind)} {
			if is {
				classes++
			}
		}
		if classes != 1 {
			t.Errorf("kind %q must be exactly one of service, bot and member", kind)
		}
		if len(kind) > 40 {
			t.Errorf("kind %q does not fit VARCHAR(40)", kind)
		}
		if IsMemberKind(kind) {
			members++
		}
		if want := !IsBotKind(kind); NeedsDeleteRight(kind) != want {
			t.Errorf("NeedsDeleteRight(%q) = %v, want %v", kind, !want, want)
		}
	}
	if members != 3 {
		t.Errorf("%d member kinds, want 3", members)
	}
	for _, kind := range []string{KindMemberCommands, KindMemberEdited, KindMemberSelfDestruct} {
		if !IsMemberKind(kind) || !ValidKind(kind) {
			t.Errorf("%q is not a valid member kind", kind)
		}
	}
	for _, kind := range []string{"", "service:", "bot:", "member:", "member:left", "member:commands ", "service:new_chat_members "} {
		if ValidKind(kind) {
			t.Errorf("ValidKind(%q) = true, want false", kind)
		}
	}
	for _, kind := range kinds {
		want := kind == KindJoined || kind == KindLeft
		if SharesCleanServiceFlag(kind) != want {
			t.Errorf("SharesCleanServiceFlag(%q) = %v, want %v", kind, !want, want)
		}
	}
	if IsMemberKind("service:pinned_message") || IsMemberKind("bot:welcome") || IsMemberKind("") {
		t.Error("IsMemberKind accepted a kind that is not a member kind")
	}
}

// TestMemberKindDelays holds the per-kind delay rules by value: a member rule is checked against
// its presets, not a range (C-05), so a database edit cannot create an unlisted grace.
func TestMemberKindDelays(t *testing.T) {
	candidates := []int{-1, 0, 10, 30, 45, 60, 300, 600, 900, 3600, 21600, 86400, 86401}
	sets := func(values ...int) map[int]bool {
		m := map[int]bool{}
		for _, v := range values {
			m[v] = true
		}
		return m
	}
	inRange := func(delay int) bool { return delay >= 0 && delay <= MaxDelaySecs }
	for _, kind := range Kinds() {
		for _, delay := range candidates {
			var wantRule, wantEntry bool
			switch kind {
			case KindMemberCommands:
				wantRule, wantEntry = sets(0)[delay], sets(0)[delay]
			case KindMemberEdited:
				wantRule, wantEntry = sets(30, 60, 300, 900, 3600)[delay], sets(0)[delay]
			case KindMemberSelfDestruct:
				wantRule = sets(60, 300, 600, 3600, 21600, 86400)[delay]
				wantEntry = wantRule
			default:
				wantRule, wantEntry = inRange(delay), inRange(delay)
			}
			checkDelay(t, "ValidateRuleDelay", kind, delay, ValidateRuleDelay(kind, delay), wantRule)
			checkDelay(t, "ValidateEntryDelay", kind, delay, ValidateEntryDelay(kind, delay), wantEntry)
		}
	}
	for _, kind := range []string{"", "service:nope", "member:left"} {
		if err := ValidateRuleDelay(kind, 0); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("ValidateRuleDelay(%q) = %v, want ErrUnknownKind", kind, err)
		}
		if err := ValidateEntryDelay(kind, 0); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("ValidateEntryDelay(%q) = %v, want ErrUnknownKind", kind, err)
		}
	}
}

func checkDelay(t *testing.T, fn, kind string, delay int, err error, want bool) {
	t.Helper()
	if want {
		if err != nil {
			t.Errorf("%s(%s, %d) = %v, want nil", fn, kind, delay, err)
		}
		return
	}
	if !errors.Is(err, ErrDelayOutOfRange) {
		t.Errorf("%s(%s, %d) = %v, want ErrDelayOutOfRange", fn, kind, delay, err)
	}
}

// cleanServiceFlag reads the shared /cleanservice flag of a chat straight from the database.
func cleanServiceFlag(t *testing.T, chatID int64) bool {
	t.Helper()
	settings, err := greetings.LoadSettingsContext(context.Background(), chatID)
	if err != nil {
		t.Fatalf("read the greetings row: %v", err)
	}
	return settings.ShouldCleanService
}

func TestApplyJoinLeftRule(t *testing.T) {
	ctx := context.Background()

	seed := func(t *testing.T, flag bool, joined, left int) int64 {
		t.Helper()
		chatID := deletionChat(t)
		if err := greetings.SetShouldCleanService(chatID, flag); err != nil {
			t.Fatalf("seed the flag: %v", err)
		}
		if joined >= 0 {
			if err := SetRule(ctx, chatID, KindJoined, joined); err != nil {
				t.Fatal(err)
			}
		}
		if left >= 0 {
			if err := SetRule(ctx, chatID, KindLeft, left); err != nil {
				t.Fatal(err)
			}
		}
		return chatID
	}

	t.Run("keep", func(t *testing.T) {
		chatID := seed(t, true, 600, 30)
		if err := ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftKeep}); err != nil {
			t.Fatal(err)
		}
		if cleanServiceFlag(t, chatID) {
			t.Error("the flag is still on after Keep")
		}
		if rules := storedRules(t, chatID); len(rules) != 0 {
			t.Errorf("rows = %v, want both joined and left removed", rules)
		}
	})

	t.Run("now", func(t *testing.T) {
		chatID := seed(t, false, 600, 30)
		if err := ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftNow}); err != nil {
			t.Fatal(err)
		}
		if !cleanServiceFlag(t, chatID) {
			t.Error("the flag is off after Now")
		}
		rules := storedRules(t, chatID)
		if _, ok := rules[KindJoined]; ok || rules[KindLeft] != 30 || len(rules) != 1 {
			t.Errorf("rows = %v, want only the left row at 30", rules)
		}
	})

	t.Run("after N", func(t *testing.T) {
		chatID := seed(t, false, -1, 30)
		if err := ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: 600}); err != nil {
			t.Fatal(err)
		}
		if !cleanServiceFlag(t, chatID) {
			t.Error("the flag is off after After N")
		}
		rules := storedRules(t, chatID)
		if rules[KindJoined] != 600 || rules[KindLeft] != 30 || len(rules) != 2 {
			t.Errorf("rows = %v, want joined 600 and the left row untouched at 30", rules)
		}
		if err := ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: 90}); err != nil {
			t.Fatal(err)
		}
		if storedRules(t, chatID)[KindJoined] != 90 {
			t.Error("a second After N did not replace the delay")
		}
	})

	t.Run("works on a chat with no rows at all", func(t *testing.T) {
		chatID := deletionChat(t)
		if err := ApplyJoinLeftRule(ctx, chatID, KindLeft, JoinLeftState{Mode: JoinLeftNow}); err != nil {
			t.Fatal(err)
		}
		if !cleanServiceFlag(t, chatID) {
			t.Error("the flag is off after Now on a fresh chat")
		}
	})

	t.Run("refused before any write", func(t *testing.T) {
		chatID := seed(t, false, 600, -1)
		refused := []struct {
			name  string
			kind  string
			state JoinLeftState
			want  error
		}{
			{"a kind that does not share the flag", KindPinned, JoinLeftState{Mode: JoinLeftNow}, ErrNotJoinLeft},
			{"an unknown kind", "service:nope", JoinLeftState{Mode: JoinLeftNow}, ErrNotJoinLeft},
			{"zero delay after N", KindJoined, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: 0}, ErrDelayOutOfRange},
			{"delay over a day", KindJoined, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: MaxDelaySecs + 1}, ErrDelayOutOfRange},
			{"an unknown mode", KindJoined, JoinLeftState{Mode: 99}, ErrInvalidJoinLeftState},
		}
		for _, tc := range refused {
			if err := ApplyJoinLeftRule(ctx, chatID, tc.kind, tc.state); !errors.Is(err, tc.want) {
				t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
			}
		}
		if cleanServiceFlag(t, chatID) {
			t.Error("a refused call changed the flag")
		}
		if rules := storedRules(t, chatID); rules[KindJoined] != 600 || len(rules) != 1 {
			t.Errorf("rows = %v, a refused call changed them", rules)
		}
	})

	t.Run("a failure inside the transaction changes nothing", func(t *testing.T) {
		chatID := seed(t, true, 600, 30)
		boom := errors.New("injected failure")
		restore := SetApplyHookForTest(func() error { return boom })
		t.Cleanup(restore)
		cases := []func() error{
			func() error { return ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftKeep}) },
			func() error {
				return ApplyJoinLeftRule(ctx, chatID, KindLeft, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: 5})
			},
			func() error { return ApplyCleanServiceFlag(ctx, chatID, false) },
		}
		for i, call := range cases {
			if err := call(); !errors.Is(err, boom) {
				t.Fatalf("case %d: error = %v, want the injected failure", i, err)
			}
			if !cleanServiceFlag(t, chatID) {
				t.Errorf("case %d: the flag changed although the transaction failed", i)
			}
			if rules := storedRules(t, chatID); rules[KindJoined] != 600 || rules[KindLeft] != 30 || len(rules) != 2 {
				t.Errorf("case %d: rows = %v, want them as they were", i, rules)
			}
		}
	})

	t.Run("the cache keys are dropped", func(t *testing.T) {
		chatID := seed(t, true, 600, 30)
		before, err := GetRules(ctx, chatID)
		if err != nil || before.Delays[KindJoined] != 600 {
			t.Fatalf("priming read = %+v, %v", before, err)
		}
		greetingsKey := cache.CacheKey("greetings", chatID)
		load := func(value string) func(context.Context) (string, error) {
			return func(context.Context) (string, error) { return value, nil }
		}
		if got, err := cache.GetFromCacheOrLoad(ctx, greetingsKey, cache.DefaultCacheTTL, load("stale")); err != nil || got != "stale" {
			t.Fatalf("priming the greetings key = %q, %v", got, err)
		}
		if err := ApplyJoinLeftRule(ctx, chatID, KindJoined, JoinLeftState{Mode: JoinLeftAfter, DelaySecs: 90}); err != nil {
			t.Fatal(err)
		}
		after, err := GetRules(ctx, chatID)
		if err != nil || after.Delays[KindJoined] != 90 {
			t.Fatalf("read after the write = %+v, %v, want 90 (the delete_rules key must be dropped)", after, err)
		}
		if got, err := cache.GetFromCacheOrLoad(ctx, greetingsKey, cache.DefaultCacheTTL, load("fresh")); err != nil || got != "fresh" {
			t.Fatalf("greetings key read = %q, %v, want fresh (the key must be dropped)", got, err)
		}
	})

	t.Run("the command's write", func(t *testing.T) {
		for _, on := range []bool{true, false} {
			chatID := seed(t, !on, 600, 30)
			if err := ApplyCleanServiceFlag(ctx, chatID, on); err != nil {
				t.Fatal(err)
			}
			if cleanServiceFlag(t, chatID) != on {
				t.Errorf("flag after ApplyCleanServiceFlag(%v) = %v", on, !on)
			}
			if rules := storedRules(t, chatID); len(rules) != 0 {
				t.Errorf("on=%v: rows = %v, want both removed", on, rules)
			}
			got, err := GetRules(ctx, chatID)
			if err != nil || len(got.Delays) != 0 {
				t.Errorf("on=%v: cached read = %+v, %v, want no rules (the key must be dropped)", on, got, err)
			}
		}
	})
}
