//go:build testtools

package modules

import (
	"strings"
	"testing"

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
}
