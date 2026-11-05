//go:build testtools

package modules

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

var memberReqTagRe = regexp.MustCompile(`<[^>]*>`)

// memberReqVisibleLength is the code points a member reads of an HTML notice: the tags go, the
// entities become characters, and the mention's link text stays (isolates included).
func memberReqVisibleLength(text string) int {
	return utf8.RuneCountInString(html.UnescapeString(memberReqTagRe.ReplaceAllString(text, "")))
}

// memberReqScreenLanguages are the 8 languages of the locale files.
var memberReqScreenLanguages = []string{"en", "es", "fr", "hi", "id", "pt", "ru", "uk"}

// TestMemberReqScreensFitEveryLocale is the rendered-size check of the requirement screens.
// This half renders every group notice with the real locales, a 64-rune name, all five rules
// and two-digit numbers; the "I fixed it" answers are added by the plan that builds them.
func TestMemberReqScreensFitEveryLocale(t *testing.T) {
	withRealLocales(t)
	const chatID int64 = -1001234567890
	allRules := 0
	for _, rule := range memberReqRules {
		allRules |= rule.Bit
	}
	user := gotgbot.User{Id: 9007199254740991, FirstName: strings.Repeat("Ж", memberReqNameCap)}

	type screen struct {
		name   string
		action int
		warn   *warnApplyResult
	}
	limit := func(mode string) *warnApplyResult {
		return &warnApplyResult{Count: 99, Limit: 99, Mode: mode, LimitReached: true, LimitApplied: true}
	}
	screens := []screen{
		{"warn", models.RequirementActionWarn, &warnApplyResult{Count: 99, Limit: 99}},
		{"warn limit kick", models.RequirementActionWarn, limit("kick")},
		{"warn limit ban", models.RequirementActionWarn, limit("ban")},
		{"warn limit mute", models.RequirementActionWarn, limit("mute")},
		{"kick", models.RequirementActionKick, nil},
		{"ban", models.RequirementActionBan, nil},
		{"mute", models.RequirementActionMute, nil},
	}
	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		for _, sc := range screens {
			text := memberReqNoticeText(tr, user, allRules, sc.action, sc.warn)
			if text == "" {
				t.Errorf("%s %s: empty notice", code, sc.name)
				continue
			}
			if got := memberReqVisibleLength(text); got > 400 {
				t.Errorf("%s %s: %d visible code points, want at most 400", code, sc.name, got)
			}
			if strings.Contains(text, strconv.FormatInt(-chatID, 10)) || strings.Contains(strings.ToLower(text), "staff") {
				t.Errorf("%s %s: the notice names a chat ID or the Staff Group: %q", code, sc.name, text)
			}
			if strings.Contains(text, "{") || strings.Contains(text, memberReqNameToken) {
				t.Errorf("%s %s: unfilled placeholder in %q", code, sc.name, text)
			}

			keyboard := memberReqNoticeKeyboard(tr, sc.action, 123, user.Id)
			wantButtons := 0
			if sc.action == models.RequirementActionMute {
				wantButtons = 1
			}
			count := 0
			for _, row := range keyboard.InlineKeyboard {
				count += len(row)
			}
			if count != wantButtons || len(keyboard.InlineKeyboard) != wantButtons {
				t.Errorf("%s %s: keyboard %+v, want %d button", code, sc.name, keyboard.InlineKeyboard, wantButtons)
			}
		}
	}
}

// TestMemberReqFixedButtonBytes holds the "I fixed it" button to Telegram's 64 bytes at the
// worst case: a 19-digit hold ID and a 16-digit user ID.
func TestMemberReqFixedButtonBytes(t *testing.T) {
	withRealLocales(t)
	for _, code := range memberReqScreenLanguages {
		assertMemberReqFixedButton(t, i18n.MustNewTranslator(code))
	}
}

func assertMemberReqFixedButton(t *testing.T, tr *i18n.Translator) {
	t.Helper()
	keyboard := memberReqFixedButton(tr, 9223372036854775807, 9007199254740991)
	if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
		t.Fatalf("keyboard = %+v, want one row of one button", keyboard.InlineKeyboard)
	}
	data := keyboard.InlineKeyboard[0][0].CallbackData
	if data == "" || len(data) > 64 {
		t.Fatalf("callback data %q is %d bytes, want 1 to 64", data, len(data))
	}
}
