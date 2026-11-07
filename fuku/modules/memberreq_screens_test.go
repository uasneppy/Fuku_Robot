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
// It renders every group notice with the real locales, a 64-rune name, all five rules and
// two-digit numbers, then every "I fixed it" answer and the two notices that button edits in.
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
			if strings.Contains(text, "{") || strings.Contains(text, memberReqNamePlaceholder) {
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
		assertMemberReqFixedAnswers(t, code, tr, allRules, user, chatID)
	}
}

// memberReqFixedAdminName is the name of the admin who set a rule in the answer checks below:
// no answer or notice is built from it, so it must never appear in one.
const memberReqFixedAdminName = "ZZ Rule Setter"

// assertMemberReqFixedAnswers measures every answer of the "I fixed it" button as the code
// builds it (memberreq_fixed.go): each is an alert or a toast of at most 200 code points, which
// is Telegram's limit for an answer, and none holds the chat ID, the Staff Group, the admin who
// set the rule or an unfilled token. The two notices the button edits in follow the notice limit.
func assertMemberReqFixedAnswers(t *testing.T, code string, tr *i18n.Translator, allRules int, user gotgbot.User, chatID int64) {
	t.Helper()
	answers := map[string]string{
		"done":             memberReqText(tr, "mreq_fixed_done"),
		"done lockdown":    memberReqText(tr, "mreq_fixed_done_lockdown"),
		"still broken all": memberReqText(tr, "mreq_fixed_still_broken", i18n.TranslationParams{"rules": memberReqRulesText(tr, allRules)}),
		"not yours":        memberReqText(tr, "mreq_fixed_not_yours"),
		"not ours":         memberReqText(tr, "mreq_fixed_not_ours"),
		"check failed":     memberReqText(tr, "mreq_fixed_check_failed"),
		"stale":            memberReqText(tr, "mreq_fixed_stale"),
		"invalid request":  memberReqText(tr, "common_callback_invalid_request"),
	}
	for _, rule := range memberReqRules {
		answers["still broken "+rule.Code] = memberReqText(tr, "mreq_fixed_still_broken", i18n.TranslationParams{"rules": memberReqRulesText(tr, rule.Bit)})
	}
	for name, text := range answers {
		if text == "" {
			t.Errorf("%s answer %q: empty", code, name)
			continue
		}
		if got := utf8.RuneCountInString(text); got > 200 {
			t.Errorf("%s answer %q: %d code points, want at most 200", code, name, got)
		}
		assertMemberReqNoLeak(t, code+" answer "+name, text, chatID)
	}

	fallback := memberReqText(tr, "mreq_name_unknown", i18n.TranslationParams{"id": strconv.FormatInt(user.Id, 10)})
	for _, key := range []string{"mreq_notice_fixed", "mreq_notice_fixed_lockdown"} {
		text := memberReqText(tr, key, i18n.TranslationParams{"name": memberReqNamePlaceholder})
		text = lockdownSplice(text, memberReqNamePlaceholder, memberReqMention(user, fallback))
		if got := memberReqVisibleLength(text); got > 400 {
			t.Errorf("%s %s: %d visible code points, want at most 400", code, key, got)
		}
		assertMemberReqNoLeak(t, code+" "+key, text, chatID)
	}
}

// assertMemberReqNoLeak fails when text names the chat, a Staff Group, the admin who set a
// rule, or holds an unfilled placeholder.
func assertMemberReqNoLeak(t *testing.T, name, text string, chatID int64) {
	t.Helper()
	if strings.Contains(text, strconv.FormatInt(-chatID, 10)) || strings.Contains(strings.ToLower(text), "staff") ||
		strings.Contains(text, memberReqFixedAdminName) {
		t.Errorf("%s: names a chat ID, the Staff Group or the admin who set the rule: %q", name, text)
	}
	if strings.Contains(text, "{") || strings.Contains(text, memberReqNamePlaceholder) {
		t.Errorf("%s: unfilled placeholder in %q", name, text)
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
