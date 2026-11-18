//go:build testtools

package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The locale style test (plan of 2026-11-18, Part 4) holds the Ukrainian and English texts to the rules of
// docs/src/content/docs/contributing/locale-style.md that a program can check. localeStyleAllowList
// (locale_style_allowlist_test.go) names the keys not yet converted; it may only shrink, and a key that
// passes must leave it, so the list ends empty when the cleanup is done.

// localeStyleLangs are the languages the cleanup covers.
var localeStyleLangs = []string{"uk", "en"}

var (
	// localeStyleMarkdown finds markdown emphasis: *bold* or `code`.
	localeStyleMarkdown = regexp.MustCompile("(^|[\\s(«\"])\\*[^*\\s][^*\\n]*\\*|`")
	// localeStyleTag finds an HTML tag.
	localeStyleTag = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)[^>]*>`)
	// localeStyleAllowedTags are the tags Telegram's HTML takes that the texts may use.
	localeStyleAllowedTags = []string{"b", "i", "u", "s", "code", "pre", "a", "blockquote", "tg-spoiler"}
)

// localeStyleProblems lists what is wrong with one text. A text may start with one newline: such a
// text is a fragment appended to another (a reason line).
func localeStyleProblems(key, text string) []string {
	var problems []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "× ") {
			problems = append(problems, "a × bullet")
			break
		}
	}
	if localeStyleMarkdown.MatchString(text) {
		problems = append(problems, "markdown emphasis or a backtick")
	}
	body := strings.TrimPrefix(text, "\n")
	if strings.TrimSpace(body) != body {
		problems = append(problems, "leading or trailing whitespace")
	}
	if strings.Contains(text, "\n\n\n") {
		problems = append(problems, "three newlines in a row")
	}
	if strings.Contains(text, "  ") {
		problems = append(problems, "a double space")
	}
	if problem := localeStyleTagProblem(text); problem != "" {
		problems = append(problems, problem)
	}
	if strings.HasSuffix(key, "_help_msg") {
		first, rest, _ := strings.Cut(text, "\n")
		if !strings.HasPrefix(first, "<b>") || !strings.HasSuffix(first, "</b>") || !strings.HasPrefix(rest, "\n") {
			problems = append(problems, "no bold title line")
		}
	}
	return problems
}

// localeStyleTagProblem reports an unbalanced or disallowed HTML tag.
func localeStyleTagProblem(text string) string {
	var stack []string
	for _, m := range localeStyleTag.FindAllStringSubmatch(text, -1) {
		name := strings.ToLower(m[2])
		if !slices.Contains(localeStyleAllowedTags, name) {
			return "the tag <" + name + ">"
		}
		if m[1] == "" {
			stack = append(stack, name)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1] != name {
			return "unbalanced tags"
		}
		stack = stack[:len(stack)-1]
	}
	if len(stack) > 0 {
		return "unbalanced tags"
	}
	return ""
}

func TestLocaleStyle(t *testing.T) {
	var failures []string
	for _, lang := range localeStyleLangs {
		data := loadStaffLocale(t, lang)
		for key, value := range data {
			text, ok := value.(string)
			if !ok {
				continue
			}
			id := lang + ":" + key
			problems := localeStyleProblems(key, text)
			allowed := localeStyleAllowList[id]
			switch {
			case len(problems) > 0 && !allowed:
				failures = append(failures, fmt.Sprintf("%s: %s", id, strings.Join(problems, ", ")))
			case len(problems) == 0 && allowed:
				failures = append(failures, fmt.Sprintf("%s passes the style rules now: take it off localeStyleAllowList", id))
			}
		}
	}
	for id := range localeStyleAllowList {
		lang, key, _ := strings.Cut(id, ":")
		if _, ok := loadStaffLocale(t, lang)[key]; !ok {
			failures = append(failures, fmt.Sprintf("%s is on localeStyleAllowList but no longer exists", id))
		}
	}
	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}

func TestLocaleStyleProblems(t *testing.T) {
	for _, tc := range []struct {
		key, text string
		want      int
	}{
		{"x", "Plain text.", 0},
		{"x", "\n<b>Reason:</b> %s", 0},
		{"x", "× /ban — bans.", 1},
		{"x", "Use *bold* here.", 1},
		{"x", "Use `code` here.", 1},
		{"x", "Two  spaces.", 1},
		{"x", "Ends with a space ", 1},
		{"x", "a\n\n\nb", 1},
		{"x", "<b>open", 1},
		{"x", "<div>x</div>", 1},
		{"x", "2 * 3 * 4", 0},
		{"a_help_msg", "<b>🔨 Bans</b>\n\nText.", 0},
		{"a_help_msg", "Text first.\n\n<b>Admin commands</b>", 1},
	} {
		if got := localeStyleProblems(tc.key, tc.text); len(got) != tc.want {
			t.Errorf("localeStyleProblems(%q) = %v, want %d problems", tc.text, got, tc.want)
		}
	}
}
