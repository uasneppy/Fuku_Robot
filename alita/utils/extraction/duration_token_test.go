//go:build testtools

package extraction

import (
	"errors"
	"testing"
)

func TestParseDurationToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		token       string
		want        DurationSpec
		wantMatched bool
		wantErr     error
	}{
		{name: "minutes", token: "30m", want: DurationSpec{Amount: 30, Unit: 'm', Seconds: 1800}, wantMatched: true},
		{name: "hours", token: "1h", want: DurationSpec{Amount: 1, Unit: 'h', Seconds: 3600}, wantMatched: true},
		{name: "days", token: "2d", want: DurationSpec{Amount: 2, Unit: 'd', Seconds: 172800}, wantMatched: true},
		{name: "weeks", token: "1w", want: DurationSpec{Amount: 1, Unit: 'w', Seconds: 604800}, wantMatched: true},
		{name: "exactly the limit", token: "366d", want: DurationSpec{Amount: 366, Unit: 'd', Seconds: 31622400}, wantMatched: true},
		{name: "one day over the limit", token: "367d", want: DurationSpec{Amount: 367, Unit: 'd', Seconds: 367 * 86400}, wantMatched: true, wantErr: ErrDurationTooLong},
		{name: "weeks over the limit", token: "53w", want: DurationSpec{Amount: 53, Unit: 'w', Seconds: 53 * 604800}, wantMatched: true, wantErr: ErrDurationTooLong},
		{name: "zero amount", token: "0d", want: DurationSpec{Amount: 0, Unit: 'd'}, wantMatched: true, wantErr: ErrDurationInvalid},
		{name: "amount that overflows int64", token: "99999999999999999999d", want: DurationSpec{Unit: 'd'}, wantMatched: true, wantErr: ErrDurationTooLong},
		{name: "amount that overflows once multiplied", token: "9223372036854775807w", want: DurationSpec{Amount: 9223372036854775807, Unit: 'w'}, wantMatched: true, wantErr: ErrDurationTooLong},
		{name: "uppercase unit", token: "2D"},
		{name: "unit only", token: "d"},
		{name: "no unit", token: "12"},
		{name: "negative", token: "-1d"},
		{name: "fraction", token: "1.5d"},
		{name: "explicit plus sign", token: "+2d"},
		{name: "unknown unit", token: "2y"},
		{name: "empty", token: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, matched, err := ParseDurationToken(tc.token)
			if matched != tc.wantMatched {
				t.Fatalf("ParseDurationToken(%q) matched = %t, want %t", tc.token, matched, tc.wantMatched)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseDurationToken(%q) error = %v, want %v", tc.token, err, tc.wantErr)
			}
			if !matched {
				return
			}
			if got.Amount != tc.want.Amount || got.Unit != tc.want.Unit {
				t.Fatalf("ParseDurationToken(%q) amount and unit = (%d, %q), want (%d, %q)",
					tc.token, got.Amount, got.Unit, tc.want.Amount, tc.want.Unit)
			}
			if err == nil && got.Seconds != tc.want.Seconds {
				t.Fatalf("ParseDurationToken(%q) seconds = %d, want %d", tc.token, got.Seconds, tc.want.Seconds)
			}
		})
	}
}

// TestParseDurationTokenAgreesWithExistingGrammar proves the staff parser and the
// per-group parser accept the same m/h/d/w amounts: every in-range token gives the
// same seconds, and the end date is the same one TemporaryUntilDate produces.
func TestParseDurationTokenAgreesWithExistingGrammar(t *testing.T) {
	t.Parallel()

	const now int64 = 1_700_000_000
	for _, token := range []string{"1m", "30m", "59m", "1h", "23h", "1d", "6d", "1w", "52w", "366d"} {
		spec, matched, err := ParseDurationToken(token)
		if !matched || err != nil {
			t.Fatalf("ParseDurationToken(%q) = (%+v, %t, %v), want a valid duration", token, spec, matched, err)
		}
		want, _, _, perr := parseTemporaryDuration(token, now)
		if perr != nil {
			t.Fatalf("parseTemporaryDuration(%q) error = %v", token, perr)
		}
		got, ok := TemporaryUntilDate(now, spec.Seconds)
		if !ok || got != want {
			t.Fatalf("%q: staff end date = (%d, %t), per-group end date = %d", token, got, ok, want)
		}
	}
}

// TestParseTemporaryDurationUnchanged pins the per-group parser's answers, so
// adding the exported parser cannot change /tban or /tmute.
func TestParseTemporaryDurationUnchanged(t *testing.T) {
	t.Parallel()

	const now int64 = 1_700_000_000
	tests := []struct {
		name       string
		input      string
		wantUntil  int64
		wantLabel  string
		wantReason string
		wantErr    error
	}{
		{name: "days with reason", input: "2d spam links", wantUntil: now + 172800, wantLabel: "2 days", wantReason: "spam links"},
		{name: "minutes", input: "30m", wantUntil: now + 1800, wantLabel: "30 minutes"},
		{name: "over the limit still refused", input: "367d x", wantUntil: -1, wantErr: errTimeLimitExceeded},
		{name: "uppercase is an invalid type", input: "2D", wantUntil: -1, wantErr: errInvalidTimeType},
		{name: "zero amount", input: "0d", wantUntil: -1, wantErr: errInvalidTimeAmount},
		{name: "nothing", input: "", wantUntil: -1, wantErr: errNoTimeSpecified},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			until, label, reason, err := parseTemporaryDuration(tc.input, now)
			if until != tc.wantUntil || label != tc.wantLabel || reason != tc.wantReason || !errors.Is(err, tc.wantErr) {
				t.Fatalf("parseTemporaryDuration(%q) = (%d, %q, %q, %v), want (%d, %q, %q, %v)",
					tc.input, until, label, reason, err, tc.wantUntil, tc.wantLabel, tc.wantReason, tc.wantErr)
			}
		})
	}
}
