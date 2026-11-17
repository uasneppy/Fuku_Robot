//go:build testtools

package i18n

import "testing"

// TestInterpolateParamsIgnoresVerbsInValues holds I4: the %s and %d verbs of a template are
// counted and filled from the template itself, so a {name} value that holds a verb (a reason
// typed by a user, "100%sure %d") is printed as typed and never shifts or blocks the template's
// own verbs, and a value that holds a {name} is never substituted again.
func TestInterpolateParamsIgnoresVerbsInValues(t *testing.T) {
	tr, err := NewTestTranslator(`
mixed: "Reason: {reason}, count %d"
twice: "Name {name} and %s"
percent: "100%% of %d"
`)
	if err != nil {
		t.Fatalf("NewTestTranslator: %v", err)
	}
	cases := []struct {
		key    string
		params TranslationParams
		want   string
	}{
		{"mixed", TranslationParams{"reason": "100%sure %d", "count": 5}, "Reason: 100%sure %d, count 5"},
		{"twice", TranslationParams{"name": "{name}%s", "0": "zero"}, "Name {name}%s and zero"},
		{"percent", TranslationParams{"count": 7}, "100% of 7"},
	}
	for _, tc := range cases {
		got, err := tr.GetString(tc.key, tc.params)
		if err != nil {
			t.Fatalf("%s: %v", tc.key, err)
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}
