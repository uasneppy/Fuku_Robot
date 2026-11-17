//go:build testtools

package formatting

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAISpamHelpRendersWithoutRawMarkdown renders aispam_help_msg of every real locale file the
// way /help does. A text that holds an HTML tag is kept as HTML, so any Markdown in it would be
// shown raw: the rendered help must hold no asterisk or backtick, keep its bold heading and its
// code placeholder, and show the on/off placeholder as text.
func TestAISpamHelpRendersWithoutRawMarkdown(t *testing.T) {
	for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
		raw, err := os.ReadFile("../../../locales/" + code + ".yml")
		if err != nil {
			t.Fatalf("read %s: %v", code, err)
		}
		var data map[string]any
		if err := yaml.Unmarshal(raw, &data); err != nil {
			t.Fatalf("parse %s: %v", code, err)
		}
		help, _ := data["aispam_help_msg"].(string)
		if help == "" {
			t.Fatalf("%s: aispam_help_msg is missing", code)
		}
		got := ToTelegramHTML(help)
		if strings.ContainsAny(got, "*`") {
			t.Errorf("%s: the help shows raw Markdown: %q", code, got)
		}
		for _, want := range []string{"<b>", "<code>&lt;on/off&gt;</code>"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the help lacks %q: %q", code, want, got)
			}
		}
	}
}
