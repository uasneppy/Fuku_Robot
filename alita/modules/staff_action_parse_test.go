//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

func parseCommand(text string, spec staffCommandSpec) (staffActionRequest, staffParseResult) {
	return parseStaffActionArgs(&gotgbot.Message{Text: text}, spec)
}

var (
	staffSpecBan  = staffCommandSpec{Name: "ban", Kind: staffKindBan, Duration: staffDurationOptional}
	staffSpecTban = staffCommandSpec{Name: "tban", Kind: staffKindBan, Duration: staffDurationRequired}
	staffSpecKick = staffCommandSpec{Name: "kick", Kind: staffKindKick, Duration: staffDurationNone}
)

func TestStaffActionParseOptionalDuration(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantResult staffParseResult
		wantSec    int64
		wantAmount int64
		wantUnit   string
		wantOver   bool
		wantReason string
	}{
		{name: "timed with reason", text: "/ban 42 2d spam", wantSec: 172800, wantAmount: 2, wantUnit: "d", wantReason: "spam"},
		{name: "timed without reason", text: "/ban 42 30m", wantSec: 1800, wantAmount: 30, wantUnit: "m"},
		{name: "no duration", text: "/ban 42 spam", wantReason: "spam"},
		{name: "no reason either", text: "/ban 42"},
		{name: "uppercase unit is part of the reason", text: "/ban 42 2D spam", wantReason: "2D spam"},
		{name: "unit-less number is part of the reason", text: "/ban 42 12 times", wantReason: "12 times"},
		{name: "a duration later in the text is not read", text: "/ban 42 spam 2d", wantReason: "spam 2d"},
		{name: "over the limit is permanent", text: "/ban 42 400d x", wantOver: true, wantAmount: 400, wantUnit: "d", wantReason: "x"},
		{name: "exactly 366d is timed", text: "/ban 42 366d x", wantSec: 366 * 86400, wantAmount: 366, wantUnit: "d", wantReason: "x"},
		{name: "367d is over the limit", text: "/ban 42 367d", wantOver: true, wantAmount: 367, wantUnit: "d"},
		{name: "overflowing amount is over the limit", text: "/ban 42 99999999999999999999d x", wantOver: true, wantUnit: "d", wantReason: "x"},
		{name: "zero is a bad duration", text: "/ban 42 0d x", wantResult: staffParseBadDuration},
		{name: "command with bot name", text: "/ban@FukuBot 42 1w", wantSec: 604800, wantAmount: 1, wantUnit: "w"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, result := parseCommand(tc.text, staffSpecBan)
			if result != tc.wantResult {
				t.Fatalf("parse %q result = %d, want %d", tc.text, result, tc.wantResult)
			}
			if result != staffParseOK {
				return
			}
			if req.DurationSec != tc.wantSec || req.DurationAmount != tc.wantAmount || req.DurationUnit != tc.wantUnit ||
				req.OverLimit != tc.wantOver || req.Reason != tc.wantReason {
				t.Fatalf("parse %q = {sec %d, amount %d, unit %q, over %t, reason %q}, want {sec %d, amount %d, unit %q, over %t, reason %q}",
					tc.text, req.DurationSec, req.DurationAmount, req.DurationUnit, req.OverLimit, req.Reason,
					tc.wantSec, tc.wantAmount, tc.wantUnit, tc.wantOver, tc.wantReason)
			}
			if req.Target.UserID != 42 {
				t.Fatalf("parse %q target = %d, want 42", tc.text, req.Target.UserID)
			}
		})
	}
}

func TestStaffActionParseRequiredDuration(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantResult staffParseResult
		wantSec    int64
		wantReason string
	}{
		{name: "duration only", text: "/tban 42 1h", wantSec: 3600},
		{name: "duration and reason", text: "/tban 42 1h flooding", wantSec: 3600, wantReason: "flooding"},
		{name: "a reason without a duration", text: "/tban 42 spam", wantResult: staffParseNeedDuration},
		{name: "nothing after the target", text: "/tban 42", wantResult: staffParseNeedDuration},
		{name: "uppercase unit is not a duration", text: "/tban 42 2D", wantResult: staffParseNeedDuration},
		{name: "zero is a bad duration", text: "/tban 42 0m", wantResult: staffParseBadDuration},
		{name: "no target at all", text: "/tban", wantResult: staffParseNoTarget},
		{name: "a bad target comes first", text: "/tban spam 1h", wantResult: staffParseBadTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, result := parseCommand(tc.text, staffSpecTban)
			if result != tc.wantResult {
				t.Fatalf("parse %q result = %d, want %d", tc.text, result, tc.wantResult)
			}
			if result != staffParseOK {
				return
			}
			if req.DurationSec != tc.wantSec || req.Reason != tc.wantReason || req.OverLimit {
				t.Fatalf("parse %q = {sec %d, reason %q, over %t}, want {sec %d, reason %q, over false}",
					tc.text, req.DurationSec, req.Reason, req.OverLimit, tc.wantSec, tc.wantReason)
			}
		})
	}

	t.Run("an over-limit duration is permanent and flagged", func(t *testing.T) {
		req, result := parseCommand("/tban 42 400d x", staffSpecTban)
		if result != staffParseOK || !req.OverLimit || req.DurationSec != 0 || req.Reason != "x" {
			t.Fatalf("parse = (%+v, %d), want an OK over-limit request with no seconds", req, result)
		}
	})
}

func TestStaffActionParseNoDuration(t *testing.T) {
	for _, text := range []string{"/kick 42 2d spam", "/kick 42 30m"} {
		req, result := parseCommand(text, staffSpecKick)
		if result != staffParseOK {
			t.Fatalf("parse %q result = %d, want OK", text, result)
		}
		if req.DurationSec != 0 || req.DurationAmount != 0 || req.DurationUnit != "" || req.OverLimit {
			t.Fatalf("parse %q read a duration: %+v", text, req)
		}
		want := text[len("/kick 42 "):]
		if req.Reason != want {
			t.Fatalf("parse %q reason = %q, want %q", text, req.Reason, want)
		}
	}
}

func TestStaffActionParseCommandTable(t *testing.T) {
	want := map[string]staffDurationMode{
		"ban": staffDurationOptional, "mute": staffDurationOptional,
		"kick": staffDurationNone, "unban": staffDurationNone, "unmute": staffDurationNone,
	}
	for _, spec := range staffActionCommands {
		mode, ok := want[spec.Name]
		if !ok {
			continue
		}
		if spec.Duration != mode {
			t.Fatalf("/%s duration mode = %d, want %d", spec.Name, spec.Duration, mode)
		}
	}
}
