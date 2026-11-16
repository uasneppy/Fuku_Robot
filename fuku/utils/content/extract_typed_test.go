//go:build testtools

package content

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
)

// TestExtractTypedNoteOrFilter proves that the typed content of a settings page is extracted exactly as
// /filter K or /save K extracts the message it replies to, for every reply-form message of the
// characterization table and a one-word keyword.
func TestExtractTypedNoteOrFilter(t *testing.T) {
	t.Parallel()
	const keyword = "hello"
	parity := 0
	for _, tc := range extractCharCases() {
		replied := tc.msg.ReplyToMessage
		if replied == nil || len(strings.Fields(tc.msg.Text)) != 2 {
			continue
		}
		parity++
		for _, mode := range []struct {
			name     string
			command  string
			isFilter bool
		}{{"filter", "/filter ", true}, {"note", "/save ", false}} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				want := ExtractNoteAndFilter(&gotgbot.Message{Text: mode.command + keyword, ReplyToMessage: replied}, mode.isFilter, "en")
				got := ExtractTypedNoteOrFilter(replied, keyword, mode.isFilter, "en")
				if extractCharString(got) != extractCharString(want) {
					t.Errorf("typed content differs from the command\n got %s\nwant %s", extractCharString(got), extractCharString(want))
				}
			})
		}
	}
	if parity < 15 {
		t.Fatalf("only %d reply-form messages in the parity table", parity)
	}

	t.Run("a two-word keyword keeps the content's own text", func(t *testing.T) {
		t.Parallel()
		replied := &gotgbot.Message{Text: "original"}
		got := ExtractTypedNoteOrFilter(replied, "hello world", true, "en")
		if got.KeyWord != "hello world" || got.Text != "original" || got.DataType != db.TEXT {
			t.Errorf("typed two-word keyword = %s", extractCharString(got))
		}
		command := ExtractNoteAndFilter(&gotgbot.Message{Text: "/filter hello world", ReplyToMessage: replied}, true, "en")
		if command.Text != "world" {
			t.Errorf("the command's two-word quirk changed: text = %q", command.Text)
		}
	})

	t.Run("a nil message is the invalid-message refusal", func(t *testing.T) {
		t.Parallel()
		got := ExtractTypedNoteOrFilter(nil, keyword, true, "en")
		want := ExtractNoteAndFilter(nil, true, "en")
		if got.DataType != -1 || got.ErrorMsg == "" || extractCharString(got) != extractCharString(want) {
			t.Errorf("nil message = %s", extractCharString(got))
		}
	})
}
