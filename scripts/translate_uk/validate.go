package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Issue is one finding about a translation. Code is stable and machine-readable
// (the page and the tests switch on it); Message is for the translator.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	// Same shapes alita/i18n/translator.go substitutes: printf verbs and {name}.
	// "%" must be followed directly by a verb, so prose like "100% of users" or
	// "50% o" is not a placeholder.
	verbRe  = regexp.MustCompile(`%%|%(?:\[\d+\])?[sdvfbtoxX]`)
	braceRe = regexp.MustCompile(`\{[^{}]+\}`)
	// A tag must start with a letter or "/letter", so "if a < b and c > d" is text.
	// Any Unicode letter counts: a localized argument such as <користувач> is
	// still a token the translator has to keep.
	tagRe    = regexp.MustCompile(`</?\p{L}[^<>]*>`)
	tagName  = regexp.MustCompile(`^</?(\p{L}[\p{L}\p{N}-]*)`)
	entityRe = regexp.MustCompile(`&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)
)

// telegramTags are the tags Telegram's HTML mode understands and needs balanced.
// Anything else tag-shaped (<userhandle>, <keyword>, <yes>) is an argument
// placeholder in a usage line: ru.yml localizes those, so only their number is
// compared, never their names.
var telegramTags = map[string]bool{
	"b": true, "i": true, "u": true, "s": true, "em": true, "strong": true,
	"code": true, "pre": true, "a": true, "blockquote": true, "tg-spoiler": true, "span": true,
	"strike": true, "del": true, "ins": true, "tg-emoji": true,
}

// PlaceholdersOf returns the sorted multiset of printf verbs and {name} tokens.
func PlaceholdersOf(s string) []string {
	out := append(verbRe.FindAllString(s, -1), braceRe.FindAllString(s, -1)...)
	sort.Strings(out)
	return out
}

// TagsOf returns every tag-shaped token exactly as written, sorted.
func TagsOf(s string) []string {
	out := tagRe.FindAllString(s, -1)
	sort.Strings(out)
	return out
}

// splitTags separates Telegram tags, kept whole so an altered href is a mismatch,
// from argument-placeholder look-alikes, of which only the count is kept.
func splitTags(s string) (telegram []string, placeholders int) {
	for _, tok := range TagsOf(s) {
		m := tagName.FindStringSubmatch(tok)
		if m != nil && telegramTags[strings.ToLower(m[1])] {
			telegram = append(telegram, tok)
		} else {
			placeholders++
		}
	}
	return telegram, placeholders
}

func entitiesOf(s string) []string {
	out := entityRe.FindAllString(s, -1)
	sort.Strings(out)
	return out
}

// diffMultiset returns what is in want but not in got, and what is in got but not
// in want.
func diffMultiset(want, got []string) (missing, extra []string) {
	count := map[string]int{}
	for _, w := range want {
		count[w]++
	}
	for _, g := range got {
		if count[g] > 0 {
			count[g]--
		} else {
			extra = append(extra, g)
		}
	}
	for _, w := range want {
		if count[w] > 0 {
			count[w]--
			missing = append(missing, w)
		}
	}
	return missing, extra
}

func describeDiff(missing, extra []string) string {
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing "+strings.Join(missing, " "))
	}
	if len(extra) > 0 {
		parts = append(parts, "unexpected "+strings.Join(extra, " "))
	}
	return strings.Join(parts, "; ")
}

func wellNested(s string) bool {
	var stack []string
	for _, tok := range tagRe.FindAllString(s, -1) {
		m := tagName.FindStringSubmatch(tok)
		if m == nil || !telegramTags[strings.ToLower(m[1])] {
			continue
		}
		name := strings.ToLower(m[1])
		if strings.HasPrefix(tok, "</") {
			if len(stack) == 0 || stack[len(stack)-1] != name {
				return false
			}
			stack = stack[:len(stack)-1]
		} else {
			stack = append(stack, name)
		}
	}
	return len(stack) == 0
}

// BlockingIssues are the findings that must stop a save: each one would make the
// message print a raw %s, drop a link, or be rejected whole by Telegram.
func BlockingIssues(en, uk []string) []Issue {
	if len(en) != len(uk) {
		return []Issue{{"list_length", fmt.Sprintf("the English has %d items, this has %d: keep the same number", len(en), len(uk))}}
	}
	var out []Issue
	for i := range en {
		prefix := ""
		if len(en) > 1 {
			prefix = fmt.Sprintf("item %d: ", i+1)
		}
		out = append(out, comparePair(en[i], uk[i], prefix)...)
	}
	return out
}

func comparePair(en, uk, prefix string) []Issue {
	var out []Issue
	add := func(code, msg string) { out = append(out, Issue{code, prefix + msg}) }

	if m, e := diffMultiset(PlaceholdersOf(en), PlaceholdersOf(uk)); len(m)+len(e) > 0 {
		add("placeholders", "placeholders differ from the English: "+describeDiff(m, e))
	}
	enTags, enArgs := splitTags(en)
	ukTags, ukArgs := splitTags(uk)
	if m, e := diffMultiset(enTags, ukTags); len(m)+len(e) > 0 {
		add("tags", "tags differ from the English: "+describeDiff(m, e))
	}
	if enArgs != ukArgs {
		add("tags", fmt.Sprintf("the English has %d <argument> placeholders and this has %d; you may translate their names but not drop or add one", enArgs, ukArgs))
	}
	if m, e := diffMultiset(entitiesOf(en), entitiesOf(uk)); len(m)+len(e) > 0 {
		add("entities", "HTML entities differ from the English (write &lt; and &gt; as in the English): "+describeDiff(m, e))
	}
	if strings.Count(en, "<") != strings.Count(uk, "<") || strings.Count(en, ">") != strings.Count(uk, ">") {
		add("angle_brackets", "the number of < and > characters differs from the English; Telegram rejects a stray one")
	}
	// Nesting is only judged when the English is well nested: a quirk that already
	// ships must not block its translation.
	if wellNested(en) && !wellNested(uk) {
		add("nesting", "tags are not properly nested or not closed")
	}
	return out
}

// Warnings never block a save. They point at the usual leftovers of a draft that
// was pre-filled from Russian, and at text likely to overflow a Telegram limit.
func Warnings(en, uk []string) []Issue {
	var out []Issue
	n := min(len(en), len(uk))
	for i := 0; i < n; i++ {
		prefix := ""
		if len(en) > 1 {
			prefix = fmt.Sprintf("item %d: ", i+1)
		}
		if found := nonUkrainianLetters(uk[i]); found != "" {
			out = append(out, Issue{"cyrillic_ru", prefix + "letters that do not exist in Ukrainian: " + found})
		}
		enLen, ukLen := utf8.RuneCountInString(en[i]), utf8.RuneCountInString(uk[i])
		// The floor keeps short labels quiet; the real limits are enforced by the
		// repo's own locale tests.
		if ukLen >= 40 && float64(ukLen) > 1.4*float64(enLen) {
			out = append(out, Issue{"length", fmt.Sprintf("%sUkrainian is %d characters against %d in English; check it still fits Telegram's limits", prefix, ukLen, enLen)})
		}
	}
	return out
}

func nonUkrainianLetters(s string) string {
	seen := map[rune]bool{}
	var found []string
	for _, r := range s {
		switch r {
		case 'ы', 'э', 'ъ', 'ё', 'Ы', 'Э', 'Ъ', 'Ё':
			if !seen[r] {
				seen[r] = true
				found = append(found, string(r))
			}
		}
	}
	return strings.Join(found, " ")
}
