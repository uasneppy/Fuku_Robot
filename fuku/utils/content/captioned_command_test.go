//go:build testtools

package content

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
)

// TestExtractCaptionedCommandDoesNotPanic proves that a command sent as a media caption (no text) and a
// message with neither text nor caption are read without a panic, and that a captioned command's words are
// its arguments.
func TestExtractCaptionedCommandDoesNotPanic(t *testing.T) {
	photo := []gotgbot.PhotoSize{{FileId: "p"}}
	for _, msg := range []*gotgbot.Message{
		{Caption: "/save", Photo: photo},
		{Caption: "/setwelcome", Photo: photo},
		{Photo: photo},
	} {
		_ = ExtractNoteAndFilter(msg, false, "en")
		_ = ExtractWelcome(msg, "welcome", "en")
	}

	note := ExtractNoteAndFilter(&gotgbot.Message{Caption: "/save rules be kind", Photo: photo}, false, "en")
	if note.KeyWord != "rules" || note.DataType != db.TEXT {
		t.Fatalf("captioned /save: keyword %q type %d, want rules and text", note.KeyWord, note.DataType)
	}
}
