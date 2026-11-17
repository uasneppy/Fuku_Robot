package extraction

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TestExtractUserAndTextWhitespaceAndCaptions holds M4: the command, the target
// and the reason are separated by any run of Unicode white space, a caption is
// read like a text, and the reason keeps its own inner white space.
func TestExtractUserAndTextWhitespaceAndCaptions(t *testing.T) {
	bot := newExtractionBot(&extractionBotClient{})
	cases := []struct {
		name     string
		text     string
		caption  bool
		reply    bool
		wantID   int64
		wantText string
	}{
		{name: "space", text: "/ban 555123 repeated spam", wantID: 555123, wantText: "repeated spam"},
		{name: "newline after target", text: "/ban 555123\nrepeated spam", wantID: 555123, wantText: "repeated spam"},
		{name: "newline after command", text: "/ban\n555123 repeated spam", wantID: 555123, wantText: "repeated spam"},
		{name: "tabs", text: "/ban\t555123\treason  here", wantID: 555123, wantText: "reason  here"},
		{name: "target alone after newline", text: "/ban\n555123", wantID: 555123, wantText: ""},
		{name: "caption", text: "/ban 555123 caption reason", caption: true, wantID: 555123, wantText: "caption reason"},
		{name: "reply with newline reason", text: "/warn\nspam words", reply: true, wantID: 1234, wantText: "spam words"},
		{name: "reply caption reason", text: "/warn spam words", caption: true, reply: true, wantID: 1234, wantText: "spam words"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newExtractionContext(bot, tc.text)
			if tc.caption {
				ctx.EffectiveMessage.Text = ""
				ctx.EffectiveMessage.Caption = tc.text
			}
			if tc.reply {
				ctx.EffectiveMessage.ReplyToMessage = &gotgbot.Message{
					MessageId: 50,
					From:      &gotgbot.User{Id: 1234, FirstName: "Reply Target"},
				}
			}
			gotID, gotText := ExtractUserAndText(bot, ctx)
			if gotID != tc.wantID || gotText != tc.wantText {
				t.Fatalf("ExtractUserAndText(%q) = (%d, %q), want (%d, %q)", tc.text, gotID, gotText, tc.wantID, tc.wantText)
			}
		})
	}
}
