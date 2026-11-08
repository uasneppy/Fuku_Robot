//go:build testtools

package modules

import (
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// mreqRuleMask is the broken-rule mask of the rules with the given callback codes.
func mreqRuleMask(t *testing.T, codes ...string) int {
	t.Helper()
	mask := 0
	for _, code := range codes {
		mask |= mreqBitOf(t, code)
	}
	return mask
}

// mreqAllRulesMask is the mask of every row of the rule table.
func mreqAllRulesMask() int {
	mask := 0
	for _, rule := range memberReqRules {
		mask |= rule.Bit
	}
	return mask
}

func TestMemberReqNoticePhotoRemedy(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	remedy := " " + staffMarker("mreq_remedy_photo")
	user := gotgbot.User{Id: 4001, FirstName: "{rules} {n}"}
	limit := func(mode string) *warnApplyResult {
		return &warnApplyResult{Count: 2, Limit: 3, Mode: mode, LimitReached: true, LimitApplied: true}
	}
	type notice struct {
		name   string
		action int
		warn   *warnApplyResult
		tail   bool
	}
	notices := []notice{
		{"warn", models.RequirementActionWarn, &warnApplyResult{Count: 1, Limit: 3}, true},
		{"kick", models.RequirementActionKick, nil, true},
		{"mute", models.RequirementActionMute, nil, true},
		{"ban", models.RequirementActionBan, nil, false},
		{"warn limit kick", models.RequirementActionWarn, limit("kick"), false},
		{"warn limit ban", models.RequirementActionWarn, limit("ban"), false},
		{"warn limit mute", models.RequirementActionWarn, limit("mute"), false},
	}

	t.Run("the photo rule among the broken rules adds the remedy to warn, kick and mute only", func(t *testing.T) {
		for _, n := range notices {
			text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ln", "ph"), n.action, n.warn)
			if got := strings.HasSuffix(text, remedy); got != n.tail {
				t.Errorf("%s notice %q: ends with the remedy = %t, want %t", n.name, text, got, n.tail)
			}
			wantCount := 0
			if n.tail {
				wantCount = 1
			}
			if got := strings.Count(text, staffMarker("mreq_remedy_photo")); got != wantCount {
				t.Errorf("%s notice has the remedy %d times, want %d", n.name, got, wantCount)
			}
		}
	})

	t.Run("no notice carries the remedy while the photo rule is not broken", func(t *testing.T) {
		for _, codes := range [][]string{{"ch"}, {"ln", "ch"}, {"ln", "un", "ru", "ar", "zh", "ch"}} {
			for _, n := range notices {
				text := memberReqNoticeText(tr, user, mreqRuleMask(t, codes...), n.action, n.warn)
				if strings.Contains(text, staffMarker("mreq_remedy_photo")) {
					t.Errorf("%s notice for %v carries the photo remedy: %q", n.name, codes, text)
				}
			}
		}
	})

	t.Run("the photo alone, a lone channel and all seven rules name their phrases in rule order", func(t *testing.T) {
		text := memberReqNoticeText(tr, user, mreqAllRulesMask(), models.RequirementActionMute, nil)
		last := -1
		for _, rule := range memberReqRules {
			at := strings.Index(text, staffMarker(rule.PhraseKey))
			if at < 0 || at <= last {
				t.Fatalf("phrase %s at %d (previous at %d) in %q, want every phrase in table order", rule.PhraseKey, at, last, text)
			}
			last = at
		}
		if !strings.HasSuffix(text, remedy) {
			t.Errorf("mute notice for all rules %q does not end with the remedy", text)
		}
	})

	t.Run("the mute notice still has exactly one button and the name is spliced after the tail", func(t *testing.T) {
		keyboard := memberReqNoticeKeyboard(tr, models.RequirementActionMute, 12, user.Id)
		if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
			t.Errorf("mute keyboard = %+v, want one row of one button", keyboard.InlineKeyboard)
		}
		text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ph"), models.RequirementActionMute, nil)
		if !strings.Contains(text, "{rules} {n}") || !strings.HasSuffix(text, remedy) {
			t.Errorf("notice %q: the name must be printed literally (never translated) and the tail must end the text", text)
		}
	})
}

func TestMemberReqStillBrokenText(t *testing.T) {
	t.Run("one to four rules list exactly them in rule order and five or more count them", func(t *testing.T) {
		withStaffLocale(t)
		tr := i18n.MustNewTranslator("en")
		list, many := staffMarker("mreq_fixed_still_broken"), staffMarker("mreq_fixed_still_broken_many")
		for k := 1; k <= len(memberReqRules); k++ {
			mask := 0
			for _, rule := range memberReqRules[:k] {
				mask |= rule.Bit
			}
			text := memberReqStillBrokenText(tr, mask)
			if k <= 4 {
				if !strings.Contains(text, list) || strings.Contains(text, many) {
					t.Errorf("%d rules: %q, want the list answer", k, text)
				}
				if want := memberReqRulesText(tr, mask); !strings.Contains(text, want) {
					t.Errorf("%d rules: %q, want it to hold %q", k, text, want)
				}
				continue
			}
			if !strings.Contains(text, many) || !strings.Contains(text, " "+strconv.Itoa(k)) {
				t.Errorf("%d rules: %q, want the count answer with %d", k, text, k)
			}
			for _, rule := range memberReqRules {
				if strings.Contains(text, staffMarker(rule.PhraseKey)) {
					t.Errorf("%d rules: the count answer names %s: %q", k, rule.PhraseKey, text)
				}
			}
		}
		// A list of the last four (photo, channel and two name rules) is still a list.
		mask := mreqRuleMask(t, "ru", "zh", "ph", "ch")
		if text := memberReqStillBrokenText(tr, mask); !strings.Contains(text, memberReqRulesText(tr, mask)) {
			t.Errorf("four rules %q does not list them", text)
		}
	})

	t.Run("the real locales keep the answers inside the 200 code point limit", func(t *testing.T) {
		withRealLocales(t)
		for _, code := range memberReqScreenLanguages {
			tr := i18n.MustNewTranslator(code)
			limit := 200
			if code == "en" {
				limit = 160
			}
			// The four longest phrases of the language together.
			rules := append([]memberReqRule(nil), memberReqRules...)
			phrase := func(r memberReqRule) int { return utf8.RuneCountInString(memberReqText(tr, r.PhraseKey)) }
			sort.SliceStable(rules, func(i, j int) bool { return phrase(rules[i]) > phrase(rules[j]) })
			mask := 0
			for _, rule := range rules[:4] {
				mask |= rule.Bit
			}
			if got := utf8.RuneCountInString(memberReqStillBrokenText(tr, mask)); got == 0 || got > limit {
				t.Errorf("%s: four longest phrases answer has %d code points, want 1 to %d", code, got, limit)
			}
			for _, n := range []int{5, 6, 7} {
				text := memberReqStillBrokenText(tr, maskOfFirst(n))
				if got := utf8.RuneCountInString(text); got == 0 || got > 160 {
					t.Errorf("%s: count answer for %d has %d code points, want 1 to 160", code, n, got)
				}
				if !strings.Contains(text, strconv.Itoa(n)) || strings.Contains(text, "{") {
					t.Errorf("%s: count answer %q does not carry %d or has an unfilled token", code, text, n)
				}
			}
		}
	})
}

// maskOfFirst is the mask of the first n rows of the rule table.
func maskOfFirst(n int) int {
	mask := 0
	for _, rule := range memberReqRules[:n] {
		mask |= rule.Bit
	}
	return mask
}
