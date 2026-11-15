package models

import "testing"

// TestHasContent pins the one definition of "Rules is set" (D-05): a non-blank text, a media
// file or at least one button. A row saved before the media columns existed holds text
// only, so it reads exactly as it did.
func TestHasContent(t *testing.T) {
	cases := []struct {
		name string
		row  RulesSettings
		want bool
	}{
		{"zero row", RulesSettings{}, false},
		{"blank text", RulesSettings{Rules: "   \n"}, false},
		{"text", RulesSettings{Rules: "x"}, true},
		{"only a file", RulesSettings{FileID: "file"}, true},
		{"only a button", RulesSettings{Buttons: ButtonArray{{Name: "a", Url: "https://example.com"}}}, true},
		{"only the switch", RulesSettings{MediaBelow: true}, false},
		{"only a media type", RulesSettings{MediaType: 4}, false},
		{"legacy row with a button label", RulesSettings{Rules: "be kind", RulesBtn: "Read"}, true},
		{"legacy button label without text", RulesSettings{RulesBtn: "Read"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.row.HasContent(); got != tc.want {
				t.Errorf("HasContent() = %v, want %v", got, tc.want)
			}
		})
	}
}
