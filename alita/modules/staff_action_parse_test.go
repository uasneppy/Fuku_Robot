//go:build testtools

package modules

import (
	"strings"
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

func TestStaffActionParseRefusedTable(t *testing.T) {
	refused := map[string]bool{"sban": true, "dban": true, "skick": true, "dkick": true, "smute": true, "dmute": true}
	seen := map[string]bool{}
	for _, spec := range staffActionCommands {
		if seen[spec.Name] {
			t.Fatalf("/%s is listed twice", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Refused != refused[spec.Name] {
			t.Fatalf("/%s Refused = %t, want %t", spec.Name, spec.Refused, refused[spec.Name])
		}
	}
	for name := range refused {
		if !seen[name] {
			t.Fatalf("/%s is not intercepted in a Staff Group", name)
		}
	}
}

// textMentionMessage builds a text message whose entities are the given ones.
func textMentionMessage(text string, entities ...gotgbot.MessageEntity) *gotgbot.Message {
	return &gotgbot.Message{Text: text, Entities: entities}
}

func TestStaffActionParseUsername(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantResult   staffParseResult
		wantUsername string
		wantSec      int64
		wantReason   string
	}{
		{name: "username with duration and reason", text: "/ban @SpamBot1 2d x", wantUsername: "SpamBot1", wantSec: 172800, wantReason: "x"},
		{name: "username only", text: "/kick @spam_bot", wantUsername: "spam_bot"},
		{name: "shortest allowed username", text: "/ban @abcd", wantUsername: "abcd"},
		{name: "longest allowed username", text: "/ban @" + strings.Repeat("a", 32), wantUsername: strings.Repeat("a", 32)},
		{name: "too short", text: "/ban @ab x", wantResult: staffParseBadUsername},
		{name: "too long", text: "/ban @" + strings.Repeat("a", 33), wantResult: staffParseBadUsername},
		{name: "illegal character", text: "/ban @bad-name x", wantResult: staffParseBadUsername},
		{name: "an at sign alone", text: "/ban @ x", wantResult: staffParseBadUsername},
		{name: "a second at sign", text: "/ban @@name_ok x", wantResult: staffParseBadUsername},
		{name: "command with bot name", text: "/ban@FukuBot @spam_bot x", wantUsername: "spam_bot", wantReason: "x"},
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
			if req.Target.Username != tc.wantUsername || req.Target.UserID != 0 || req.DurationSec != tc.wantSec || req.Reason != tc.wantReason {
				t.Fatalf("parse %q = {username %q, id %d, sec %d, reason %q}, want {username %q, id 0, sec %d, reason %q}",
					tc.text, req.Target.Username, req.Target.UserID, req.DurationSec, req.Reason, tc.wantUsername, tc.wantSec, tc.wantReason)
			}
		})
	}
}

func TestStaffActionParseTextMention(t *testing.T) {
	person := &gotgbot.User{Id: 777, FirstName: "Zoë 🙂", LastName: "Smith"}
	command := gotgbot.MessageEntity{Type: "bot_command", Offset: 0, Length: 4}

	t.Run("a multi-word non-ASCII name does not shift the reason", func(t *testing.T) {
		// "/ban " is 5 UTF-16 units; "Zoë 🙂 Smith" is 3+1+2+1+5 = 12.
		msg := textMentionMessage("/ban Zoë 🙂 Smith spam", command,
			gotgbot.MessageEntity{Type: "text_mention", Offset: 5, Length: 12, User: person})
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK {
			t.Fatalf("result = %d, want OK", result)
		}
		if req.Target.UserID != 777 || req.Target.MentionName != "Zoë 🙂 Smith" || req.Target.Username != "" || req.Reason != "spam" {
			t.Fatalf("request = %+v, want user 777, name \"Zoë 🙂 Smith\" and reason \"spam\"", req)
		}
	})

	t.Run("astral characters before the mention count as two units", func(t *testing.T) {
		msg := textMentionMessage("/ban 🙂🙂 Zed x",
			gotgbot.MessageEntity{Type: "text_mention", Offset: 5, Length: 8, User: &gotgbot.User{Id: 778, FirstName: "🙂🙂", LastName: "Zed"}})
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.UserID != 778 || req.Reason != "x" {
			t.Fatalf("parse = (%+v, %d), want user 778 and reason \"x\"", req, result)
		}
	})

	t.Run("a duration after the mention is read", func(t *testing.T) {
		msg := textMentionMessage("/tban Zoë 🙂 Smith 2d spam",
			gotgbot.MessageEntity{Type: "text_mention", Offset: 6, Length: 12, User: person})
		req, result := parseStaffActionArgs(msg, staffSpecTban)
		if result != staffParseOK || req.Target.UserID != 777 || req.DurationSec != 172800 || req.Reason != "spam" {
			t.Fatalf("parse = (%+v, %d), want user 777, 2d and reason \"spam\"", req, result)
		}
	})

	t.Run("a caption uses the caption entities", func(t *testing.T) {
		msg := &gotgbot.Message{
			Caption: "/ban Zoë 🙂 Smith spam",
			CaptionEntities: []gotgbot.MessageEntity{
				{Type: "text_mention", Offset: 5, Length: 12, User: person},
			},
		}
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.UserID != 777 || req.Target.MentionName != "Zoë 🙂 Smith" || req.Reason != "spam" {
			t.Fatalf("parse = (%+v, %d), want user 777 and reason \"spam\"", req, result)
		}
	})

	t.Run("a mention that is not at the first argument is ignored", func(t *testing.T) {
		msg := textMentionMessage("/ban spam Zoë 🙂 Smith",
			gotgbot.MessageEntity{Type: "text_mention", Offset: 10, Length: 12, User: person})
		if _, result := parseStaffActionArgs(msg, staffSpecBan); result != staffParseBadTarget {
			t.Fatalf("result = %d, want BadTarget: the first field is parsed normally", result)
		}

		msg = textMentionMessage("/ban 4242 Zoë 🙂 Smith",
			gotgbot.MessageEntity{Type: "text_mention", Offset: 10, Length: 12, User: person})
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.UserID != 4242 || req.Target.MentionName != "" || req.Reason != "Zoë 🙂 Smith" {
			t.Fatalf("parse = (%+v, %d), want the numeric target 4242 and the mention left in the reason", req, result)
		}
	})

	t.Run("a mention one unit off the first argument is ignored", func(t *testing.T) {
		msg := textMentionMessage("/ban Zoë 🙂 Smith spam",
			gotgbot.MessageEntity{Type: "text_mention", Offset: 6, Length: 11, User: person})
		if _, result := parseStaffActionArgs(msg, staffSpecBan); result != staffParseBadTarget {
			t.Fatalf("result = %d, want BadTarget", result)
		}
	})

	t.Run("a mention entity without a user is ignored", func(t *testing.T) {
		msg := textMentionMessage("/ban Zoë spam", gotgbot.MessageEntity{Type: "text_mention", Offset: 5, Length: 3})
		if _, result := parseStaffActionArgs(msg, staffSpecBan); result != staffParseBadTarget {
			t.Fatalf("result = %d, want BadTarget", result)
		}
	})

	t.Run("an at-mention entity is not a text mention", func(t *testing.T) {
		msg := textMentionMessage("/ban @spam_bot x",
			gotgbot.MessageEntity{Type: "mention", Offset: 5, Length: 9})
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.Username != "spam_bot" || req.Target.UserID != 0 {
			t.Fatalf("parse = (%+v, %d), want the @username target", req, result)
		}
	})
}

func TestStaffActionParseBareReply(t *testing.T) {
	replied := &gotgbot.Message{From: &gotgbot.User{Id: 999, FirstName: "Replied"}, Text: "moderated"}

	t.Run("a reply with no argument asks for a target", func(t *testing.T) {
		msg := &gotgbot.Message{Text: "/ban", ReplyToMessage: replied}
		if _, result := parseStaffActionArgs(msg, staffSpecBan); result != staffParseBareReply {
			t.Fatalf("result = %d, want BareReply", result)
		}
	})

	t.Run("no reply and no argument has no target", func(t *testing.T) {
		if _, result := parseCommand("/ban", staffSpecBan); result != staffParseNoTarget {
			t.Fatalf("result = %d, want NoTarget", result)
		}
	})

	t.Run("a reply with an explicit ID uses the ID", func(t *testing.T) {
		msg := &gotgbot.Message{Text: "/ban 4242", ReplyToMessage: replied}
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.UserID != 4242 {
			t.Fatalf("parse = (%+v, %d), want target 4242, never the replied-to sender", req, result)
		}
	})

	t.Run("a reply with an explicit username uses the username", func(t *testing.T) {
		msg := &gotgbot.Message{Text: "/mute @spam_bot 1h", ReplyToMessage: replied}
		req, result := parseStaffActionArgs(msg, staffSpecBan)
		if result != staffParseOK || req.Target.Username != "spam_bot" || req.Target.UserID != 0 {
			t.Fatalf("parse = (%+v, %d), want the username, never the replied-to sender", req, result)
		}
	})

	t.Run("a reply with a non-target word is a bad target", func(t *testing.T) {
		msg := &gotgbot.Message{Text: "/ban spam", ReplyToMessage: replied}
		if _, result := parseStaffActionArgs(msg, staffSpecBan); result != staffParseBadTarget {
			t.Fatalf("result = %d, want BadTarget", result)
		}
	})
}
