//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
)

// fwdTestStaffGroup stores a Staff Group row for a fresh chat and cleans up after the test.
func fwdTestStaffGroup(t *testing.T) int64 {
	t.Helper()
	withMiniredis(t)
	staffChat := uniqueModuleChatID()
	staffCleanup(t, staffChat)
	if _, err := staff.CreateStaffGroup(staffChat, 1, "Staff"); err != nil {
		t.Fatalf("create Staff Group: %v", err)
	}
	return staffChat
}

// fwdResolver scripts forwardBlockResolve: answers maps the queried chat (an int64 or an @name) to
// a chat or an error, and calls counts the lookups.
type fwdResolver struct {
	answers map[any]forwardBlockChat
	errs    map[any]error
	calls   []any
}

func (r *fwdResolver) install(t *testing.T) {
	t.Helper()
	mreqSwap(t, &forwardBlockResolve, func(_ context.Context, _ *gotgbot.Bot, chat any) (forwardBlockChat, error) {
		r.calls = append(r.calls, chat)
		if err := r.errs[chat]; err != nil {
			return forwardBlockChat{}, err
		}
		if answer, ok := r.answers[chat]; ok {
			return answer, nil
		}
		return forwardBlockChat{}, &gotgbot.TelegramError{Method: "getChat", Code: 400, Description: "Bad Request: chat not found"}
	})
}

func fwdRowCount(t *testing.T, staffChat int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", staffChat).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyForwardBlockAdd(t *testing.T) {
	staffChat := fwdTestStaffGroup(t)
	longTitle := strings.Repeat("Ж", 100)
	tg := func(code int) error { return &gotgbot.TelegramError{Method: "getChat", Code: code, Description: "x"} }
	resolver := &fwdResolver{
		answers: map[any]forwardBlockChat{
			"@goodchan":           {ID: -1001000000001, Type: "channel", Title: "Good Channel"},
			"@longtitle":          {ID: -1001000000002, Type: "supergroup", Title: "  " + longTitle + "  "},
			"@aprivate":           {ID: -1001000000003, Type: "private", Title: "Someone"},
			"@positive":           {ID: 4242, Type: "channel", Title: "Odd"},
			int64(-1009000000001): {ID: -1009000000001, Type: "channel", Title: "Numeric Channel"},
			int64(-1009000000002): {ID: -1009000000002, Type: "private", Title: "Not a chat"},
			int64(-1009000000003): {ID: -1009000000099, Type: "channel", Title: "Other ID"},
		},
		errs: map[any]error{
			"@limited":            tg(429),
			"@broken":             tg(502),
			"@gone1":              tg(400),
			int64(-1009000000004): tg(400),
			int64(-1009000000005): tg(500),
		},
	}
	resolver.install(t)

	cases := []struct {
		name        string
		ref         string
		want        forwardBlockAddOutcome
		wantStored  int64 // the stored source ID, 0 when nothing is stored
		wantTitle   string
		wantLookups int
	}{
		{"name resolves to a channel", "@goodchan", forwardBlockAdded, -1001000000001, "Good Channel", 1},
		{"a repeat is already listed", "@goodchan", forwardBlockAlready, 0, "", 1},
		{"title is trimmed and cut at 64 code points", "@longtitle", forwardBlockAdded, -1001000000002, strings.Repeat("Ж", 64), 1},
		{"name not found", "@gone1", forwardBlockNotFound, 0, "", 1},
		{"name that resolves to a private chat", "@aprivate", forwardBlockNotSource, 0, "", 1},
		{"name that resolves to a positive ID", "@positive", forwardBlockNotSource, 0, "", 1},
		{"name with a 429", "@limited", forwardBlockCheckFailed, 0, "", 1},
		{"name with a 5xx", "@broken", forwardBlockCheckFailed, 0, "", 1},
		{"numeric ID that resolves", "-1009000000001", forwardBlockAdded, -1009000000001, "Numeric Channel", 1},
		{"numeric ID Telegram refuses with a 400 is stored unverified", "-1009000000004", forwardBlockAdded, -1009000000004, "", 1},
		{"numeric ID that resolves to a non-chat", "-1009000000002", forwardBlockNotSource, 0, "", 1},
		{"numeric ID answered with another ID", "-1009000000003", forwardBlockNotSource, 0, "", 1},
		{"numeric ID with a 5xx", "-1009000000005", forwardBlockCheckFailed, 0, "", 1},
		{"malformed: plain word", "goodchan", forwardBlockInvalidRef, 0, "", 0},
		{"malformed: too short name", "@ab", forwardBlockInvalidRef, 0, "", 0},
		{"malformed: positive ID", "4242", forwardBlockInvalidRef, 0, "", 0},
		{"malformed: minus only", "-", forwardBlockInvalidRef, 0, "", 0},
		{"malformed: two tokens", "-1001 2", forwardBlockInvalidRef, 0, "", 0},
		{"malformed: empty", "", forwardBlockInvalidRef, 0, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := fwdRowCount(t, staffChat)
			lookupsBefore := len(resolver.calls)
			result := applyForwardBlockAdd(context.Background(), nil, staffChat, 7, tc.ref)
			if result.Outcome != tc.want {
				t.Fatalf("outcome = %d, want %d", result.Outcome, tc.want)
			}
			if got := len(resolver.calls) - lookupsBefore; got != tc.wantLookups {
				t.Errorf("%d lookups, want %d", got, tc.wantLookups)
			}
			after := fwdRowCount(t, staffChat)
			if tc.wantStored == 0 {
				if after != before {
					t.Fatalf("a refused add changed the row count from %d to %d", before, after)
				}
				return
			}
			if after != before+1 {
				t.Fatalf("row count %d -> %d, want one more", before, after)
			}
			row, err := staff.ListForwardBlocksFresh(context.Background(), staffChat)
			if err != nil {
				t.Fatal(err)
			}
			last := row[len(row)-1]
			if last.SourceChatID != tc.wantStored || last.SourceTitle != tc.wantTitle || last.CreatedBy != 7 {
				t.Fatalf("stored %+v, want source %d title %q by user 7", last, tc.wantStored, tc.wantTitle)
			}
		})
	}

	t.Run("a full list is refused", func(t *testing.T) {
		full := fwdTestStaffGroup(t)
		rows := make([]models.StaffForwardBlock, staff.ForwardBlockMax)
		for i := range rows {
			rows[i] = models.StaffForwardBlock{StaffChatID: full, SourceChatID: -int64(i) - 1}
		}
		if err := db.DB.CreateInBatches(&rows, 50).Error; err != nil {
			t.Fatal(err)
		}
		result := applyForwardBlockAdd(context.Background(), nil, full, 7, "-1009000000001")
		if result.Outcome != forwardBlockFull {
			t.Fatalf("outcome = %d, want full", result.Outcome)
		}
		if fwdRowCount(t, full) != staff.ForwardBlockMax {
			t.Fatal("a refused add changed the list")
		}
	})

	t.Run("no Staff Group", func(t *testing.T) {
		result := applyForwardBlockAdd(context.Background(), nil, uniqueModuleChatID(), 7, "-1009000000001")
		if result.Outcome != forwardBlockNoStaffGroup {
			t.Fatalf("outcome = %d, want no Staff Group", result.Outcome)
		}
	})

	t.Run("a failing write is a save failure", func(t *testing.T) {
		seams := forwardBlockWrites
		seams.Add = func(context.Context, int64, int64, string, int64) (bool, error) { return false, errors.New("boom") }
		mreqSwap(t, &forwardBlockWrites, seams)
		result := applyForwardBlockAdd(context.Background(), nil, staffChat, 7, "-1009000000001")
		if result.Outcome != forwardBlockSaveFailed {
			t.Fatalf("outcome = %d, want save failed", result.Outcome)
		}
	})
}

func TestApplyForwardBlockRemove(t *testing.T) {
	staffChat := fwdTestStaffGroup(t)
	other := fwdTestStaffGroup(t)
	ctx := context.Background()
	for _, id := range []int64{-1005000000001, -1005000000002} {
		if _, err := staff.AddForwardBlock(ctx, staffChat, id, "T"+fmt.Sprint(-id%10), 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := staff.AddForwardBlock(ctx, other, -1005000000001, "Other", 1); err != nil {
		t.Fatal(err)
	}

	if r := applyForwardBlockRemove(ctx, staffChat, -1005000000001); r.Outcome != forwardBlockRemoved {
		t.Fatalf("remove by source = %d, want removed", r.Outcome)
	}
	if r := applyForwardBlockRemove(ctx, staffChat, -1005000000001); r.Outcome != forwardBlockGone {
		t.Fatalf("a second removal = %d, want gone", r.Outcome)
	}
	if fwdRowCount(t, other) != 1 {
		t.Fatal("removing a source touched another Staff Group")
	}

	rows, _ := staff.ListForwardBlocksFresh(ctx, staffChat)
	foreign, _ := staff.ListForwardBlocksFresh(ctx, other)
	if r := applyForwardBlockRemoveByID(ctx, staffChat, foreign[0].ID); r.Outcome != forwardBlockGone {
		t.Fatalf("remove by a foreign row ID = %d, want gone", r.Outcome)
	}
	if r := applyForwardBlockRemoveByID(ctx, staffChat, 987654321); r.Outcome != forwardBlockGone {
		t.Fatalf("remove by an unknown ID = %d, want gone", r.Outcome)
	}
	r := applyForwardBlockRemoveByID(ctx, staffChat, rows[0].ID)
	if r.Outcome != forwardBlockRemoved || r.SourceChatID != -1005000000002 || r.Title == "" {
		t.Fatalf("remove by ID = %+v, want the removed row's source and title", r)
	}

	seams := forwardBlockWrites
	seams.RemoveBySource = func(context.Context, int64, int64) (bool, error) { return false, errors.New("boom") }
	seams.RemoveByID = func(context.Context, int64, int64) (*models.StaffForwardBlock, error) { return nil, errors.New("boom") }
	mreqSwap(t, &forwardBlockWrites, seams)
	if r := applyForwardBlockRemove(ctx, staffChat, -1); r.Outcome != forwardBlockRemoveFailed {
		t.Fatalf("a failing remove = %d, want failed", r.Outcome)
	}
	if r := applyForwardBlockRemoveByID(ctx, staffChat, 1); r.Outcome != forwardBlockRemoveFailed {
		t.Fatalf("a failing remove by ID = %d, want failed", r.Outcome)
	}
}

func TestApplyForwardBlockAction(t *testing.T) {
	staffChat := fwdTestStaffGroup(t)
	ctx := context.Background()
	stored := func() string {
		t.Helper()
		rules, err := staff.GetForwardBlockRulesFresh(ctx, staffChat)
		if err != nil {
			t.Fatal(err)
		}
		return rules.Action
	}

	for _, action := range []string{"warn", "mute", "kick", "ban", "none"} {
		if r := applyForwardBlockAction(ctx, staffChat, 9, action); r.Outcome != forwardBlockActionApplied || stored() != action {
			t.Fatalf("action %q: result %+v, stored %q", action, r, stored())
		}
	}
	applyForwardBlockAction(ctx, staffChat, 9, "ban")
	for _, bad := range []string{"", "tban", "tmute", "delete", "Ban", "banish"} {
		if r := applyForwardBlockAction(ctx, staffChat, 9, bad); r.Outcome != forwardBlockActionUnknown {
			t.Errorf("action %q = %d, want unknown", bad, r.Outcome)
		}
	}
	if stored() != "ban" {
		t.Fatalf("a refused action changed the stored one to %q", stored())
	}
	if r := applyForwardBlockAction(ctx, uniqueModuleChatID(), 9, "warn"); r.Outcome != forwardBlockActionNoStaffGroup {
		t.Fatalf("no Staff Group = %d", r.Outcome)
	}

	seams := forwardBlockWrites
	called := false
	seams.SetAction = func(context.Context, int64, int64, string) error { called = true; return errors.New("boom") }
	mreqSwap(t, &forwardBlockWrites, seams)
	if r := applyForwardBlockAction(ctx, staffChat, 9, "bogus"); r.Outcome != forwardBlockActionUnknown || called {
		t.Fatalf("a refused action reached the write seam (called=%v, outcome=%d)", called, r.Outcome)
	}
	if r := applyForwardBlockAction(ctx, staffChat, 9, "warn"); r.Outcome != forwardBlockActionSaveFailed {
		t.Fatalf("a failing write = %d, want save failed", r.Outcome)
	}
}

// TestForwardBlockVocabulary pins that the forward blocklist and the blacklist share one action
// vocabulary.
func TestForwardBlockVocabulary(t *testing.T) {
	a, b := slices.Clone(staff.ForwardBlockActions), slices.Clone(blacklistActionValues)
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		t.Fatalf("staff.ForwardBlockActions = %v, blacklistActionValues = %v: the vocabularies differ", a, b)
	}
}

func TestParseForwardBlockRef(t *testing.T) {
	good := map[string]forwardBlockRef{
		"-1001234567890": {ID: -1001234567890},
		"@some_chan":     {Username: "@some_chan"},
		"  @Chan42  ":    {Username: "@Chan42"},
	}
	for raw, want := range good {
		if got, ok := parseForwardBlockRef(raw); !ok || got != want {
			t.Errorf("parseForwardBlockRef(%q) = (%+v, %v), want %+v", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"", "0", "5", "-0", "@abc", "@bad-name", "name", "-12a", strings.Repeat("9", 70), "@" + strings.Repeat("a", 40)} {
		if _, ok := parseForwardBlockRef(raw); ok {
			t.Errorf("parseForwardBlockRef(%q) accepted", raw)
		}
	}
}

// TestForwardBlockResolveChatRequest drives the default resolver through a real bot client: one
// raw getChat with the numeric ID or the @name as chat_id, decoded into id, type and title.
func TestForwardBlockResolveChatRequest(t *testing.T) {
	client := newModuleBotClient()
	client.responses["getChat"] = []byte(`{"id":-1001777000111,"type":"channel","title":"Resolved <b>"}`)
	bot := newModuleTestBot(client)

	got, err := forwardBlockResolveChat(context.Background(), bot, "@somechan")
	if err != nil || got.ID != -1001777000111 || got.Type != "channel" || got.Title != "Resolved <b>" {
		t.Fatalf("resolve = (%+v, %v)", got, err)
	}
	if _, err := forwardBlockResolveChat(context.Background(), bot, int64(-1001777000111)); err != nil {
		t.Fatal(err)
	}
	calls := client.callsFor("getChat")
	if len(calls) != 2 || fmt.Sprint(calls[0].Params["chat_id"]) != "@somechan" || fmt.Sprint(calls[1].Params["chat_id"]) != "-1001777000111" {
		t.Fatalf("getChat calls = %+v", calls)
	}

	client.responses["getChat"] = []byte(`not json`)
	if _, err := forwardBlockResolveChat(context.Background(), bot, "@somechan"); err == nil {
		t.Fatal("an unreadable answer was accepted")
	}
}
