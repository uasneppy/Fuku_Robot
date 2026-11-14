//go:build testtools

package modules

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// The notice's trailing remedy sentences (plan 15-07; D-15, amendment M20): one sentence per
// remedy-bearing broken rule, in rule order, appended after translation to the plain warn, kick
// and mute notices only. The photo rule's sentence and the AI rule's sentence are the two today.

// mreqNoticeKinds are the seven notices of a punishment and whether each carries the tails.
type mreqNoticeKind struct {
	name   string
	action int
	warn   *warnApplyResult
	tails  bool
}

func mreqNoticeKinds() []mreqNoticeKind {
	limit := func(mode string) *warnApplyResult {
		return &warnApplyResult{Count: 2, Limit: 3, Mode: mode, LimitReached: true, LimitApplied: true}
	}
	return []mreqNoticeKind{
		{"warn", models.RequirementActionWarn, &warnApplyResult{Count: 1, Limit: 3}, true},
		{"kick", models.RequirementActionKick, nil, true},
		{"mute", models.RequirementActionMute, nil, true},
		{"ban", models.RequirementActionBan, nil, false},
		{"warn limit kick", models.RequirementActionWarn, limit("kick"), false},
		{"warn limit ban", models.RequirementActionWarn, limit("ban"), false},
		{"warn limit mute", models.RequirementActionWarn, limit("mute"), false},
	}
}

func TestMemberReqNoticeAIRemedy(t *testing.T) {
	withStaffLocale(t)
	tr := i18n.MustNewTranslator("en")
	photo, ai := staffMarker("mreq_remedy_photo"), staffMarker("mreq_remedy_ai")
	user := gotgbot.User{Id: 4002, FirstName: "{rules} {n}"}

	t.Run("the AI rule alone ends the warn, kick and mute notices with its remedy and lists its phrase", func(t *testing.T) {
		for _, n := range mreqNoticeKinds() {
			text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ai"), n.action, n.warn)
			if got := strings.HasSuffix(text, " "+ai); got != n.tails {
				t.Errorf("%s notice %q: ends with the AI remedy after one space = %t, want %t", n.name, text, got, n.tails)
			}
			if got := strings.Contains(text, ai); got != n.tails {
				t.Errorf("%s notice %q: carries the AI remedy = %t, want %t", n.name, text, got, n.tails)
			}
			if strings.Contains(text, photo) {
				t.Errorf("%s notice %q carries the photo remedy for the AI rule alone", n.name, text)
			}
			if !strings.Contains(text, staffMarker("mreq_rule_ai")) {
				t.Errorf("%s notice %q does not list the AI phrase", n.name, text)
			}
		}
	})

	t.Run("a muted notice for the AI rule still has exactly one button", func(t *testing.T) {
		keyboard := memberReqNoticeKeyboard(tr, models.RequirementActionMute, 12, user.Id)
		if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
			t.Errorf("mute keyboard = %+v, want one row of one button", keyboard.InlineKeyboard)
		}
	})

	t.Run("the photo rule alone gives the photo remedy and no AI remedy", func(t *testing.T) {
		for _, n := range mreqNoticeKinds() {
			text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ph"), n.action, n.warn)
			if got := strings.HasSuffix(text, " "+photo); got != n.tails {
				t.Errorf("%s notice %q: ends with the photo remedy = %t, want %t", n.name, text, got, n.tails)
			}
			if strings.Contains(text, ai) {
				t.Errorf("%s notice %q carries the AI remedy for the photo rule alone", n.name, text)
			}
		}
	})

	t.Run("both rules give both remedies, the photo one first, one space apart", func(t *testing.T) {
		for _, n := range mreqNoticeKinds() {
			text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ph", "ai"), n.action, n.warn)
			if got := strings.HasSuffix(text, " "+photo+" "+ai); got != n.tails {
				t.Errorf("%s notice %q: ends with the photo then the AI remedy = %t, want %t", n.name, text, got, n.tails)
			}
			if !n.tails && (strings.Contains(text, photo) || strings.Contains(text, ai)) {
				t.Errorf("%s notice %q carries a remedy, want none", n.name, text)
			}
		}
	})

	t.Run("neither rule gives no remedy and the ban and warn-limit notices never do", func(t *testing.T) {
		for _, n := range mreqNoticeKinds() {
			text := memberReqNoticeText(tr, user, mreqRuleMask(t, "ln", "ch"), n.action, n.warn)
			if strings.Contains(text, photo) || strings.Contains(text, ai) {
				t.Errorf("%s notice %q carries a remedy, want none", n.name, text)
			}
			if !n.tails {
				all := memberReqNoticeText(tr, user, mreqAllRulesMask(), n.action, n.warn)
				if strings.Contains(all, photo) || strings.Contains(all, ai) {
					t.Errorf("%s notice %q carries a remedy with every rule broken, want none", n.name, all)
				}
			}
		}
	})

	t.Run("the rules list names the eight phrases in rule order", func(t *testing.T) {
		text := memberReqNoticeText(tr, user, mreqAllRulesMask(), models.RequirementActionMute, nil)
		last, seen := -1, 0
		for _, rule := range memberReqRules {
			at := strings.Index(text, staffMarker(rule.PhraseKey))
			if at < 0 || at <= last {
				t.Fatalf("phrase %s at %d (previous at %d) in %q, want every phrase in table order", rule.PhraseKey, at, last, text)
			}
			last, seen = at, seen+1
		}
		if seen != 8 {
			t.Errorf("phrases found = %d, want 8", seen)
		}
		if strings.Index(text, staffMarker("mreq_rule_ai")) < strings.Index(text, staffMarker("mreq_rule_channel")) {
			t.Errorf("the AI phrase comes before the channel phrase in %q", text)
		}
	})

	t.Run("the real locales keep every notice inside 520 code points with all eight phrases and both tails", func(t *testing.T) {
		withRealLocales(t)
		name := strings.Repeat("Ж", memberReqNameCap)
		realUser := gotgbot.User{Id: 9007199254740991, FirstName: name}
		for _, code := range memberReqScreenLanguages {
			real := i18n.MustNewTranslator(code)
			for _, n := range mreqNoticeKinds() {
				warn := n.warn
				if warn != nil {
					warn = &warnApplyResult{Count: 99, Limit: 99, Mode: warn.Mode, LimitReached: warn.LimitReached, LimitApplied: warn.LimitApplied}
				}
				text := memberReqNoticeText(real, realUser, mreqAllRulesMask(), n.action, warn)
				if got := memberReqVisibleLength(text); got == 0 || got > memberReqNoticeCap {
					t.Errorf("%s %s notice has %d visible code points, want 1 to %d", code, n.name, got, memberReqNoticeCap)
				}
				if got := strings.HasSuffix(text, " "+memberReqText(real, "mreq_remedy_photo")+" "+memberReqText(real, "mreq_remedy_ai")); got != n.tails {
					t.Errorf("%s %s notice: ends with both remedies = %t, want %t", code, n.name, got, n.tails)
				}
				if strings.Count(text, name) != 1 {
					t.Errorf("%s %s notice names the member %d times, want the one mention", code, n.name, strings.Count(text, name))
				}
			}
			reason := memberReqText(real, "mreq_warn_reason", i18n.TranslationParams{"rules": memberReqRulesText(real, mreqAllRulesMask())})
			if got := utf8.RuneCountInString(reason); got == 0 || got > 300 {
				t.Errorf("%s mreq_warn_reason with all eight phrases has %d code points, want 1 to 300", code, got)
			}
		}
	})

	t.Run("no AI string names the model, a probability or a member", func(t *testing.T) {
		withRealLocales(t)
		forbidden := []string{"claude", "anthropic", "haiku", "model", strings.ToLower(raidAITestModel), "probab", "%"}
		for _, code := range memberReqScreenLanguages {
			real := i18n.MustNewTranslator(code)
			for _, key := range []string{"mreq_rule_ai", "mreq_remedy_ai"} {
				text := memberReqText(real, key)
				if text == "" || strings.ContainsAny(text, "{}") {
					t.Errorf("%s %s = %q, want a plain sentence with no token", code, key, text)
				}
				for _, bad := range forbidden {
					if strings.Contains(strings.ToLower(text), bad) {
						t.Errorf("%s %s %q holds %q", code, key, text, bad)
					}
				}
				if strings.ContainsAny(text, "0123456789") {
					t.Errorf("%s %s %q holds a digit, so it could carry a count or a probability", code, key, text)
				}
			}
			flagged := gotgbot.User{Id: 77, FirstName: "ZZFlagged" + strconv.Itoa(len(code))}
			text := memberReqNoticeText(real, flagged, mreqRuleMask(t, "ai"), models.RequirementActionMute, nil)
			if strings.Count(text, flagged.FirstName) != 1 {
				t.Errorf("%s mute notice %q names the member %d times, want the one mention", code, text, strings.Count(text, flagged.FirstName))
			}
		}
	})
}
