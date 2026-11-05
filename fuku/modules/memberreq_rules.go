package modules

import (
	"unicode"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"golang.org/x/text/unicode/norm"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// memberReqNames are the three profile fields the member requirement rules judge.
type memberReqNames struct {
	First    string
	Last     string
	Username string
}

// memberReqNamesOf takes the names of a Telegram user.
func memberReqNamesOf(u gotgbot.User) memberReqNames {
	return memberReqNames{First: u.FirstName, Last: u.LastName, Username: u.Username}
}

// memberReqRule is one row of the rule table: the rule's callback code, its bit in a
// broken-rule mask, the stored rule ID, the locale keys the page and the notices read, and
// the test of a member's names. The name rules judge the first and the last name and never
// the username.
type memberReqRule struct {
	Code      string
	Bit       int
	Store     memberreq.Rule
	LineKey   string
	CellKey   string
	DescKey   string
	PhraseKey string
	Broken    func(memberReqNames) bool
}

// memberReqRussianLetters are the four letters that Russian uses and Ukrainian does not:
// ы, ъ, э and ё, each in both cases. The rule is letter-based on purpose: a Russian name
// without them is not caught, and a Ukrainian name (which has none of them) never is (D-10).
func memberReqRussianLetters(r rune) bool {
	switch r {
	case 'ы', 'ъ', 'э', 'ё', 'Ы', 'Ъ', 'Э', 'Ё':
		return true
	}
	return false
}

// memberReqHasRussian reports whether s has one of the letters ы ъ э ё. The text is put
// in NFC first, so a decomposed ё (е and U+0308) is the letter ё too. NFKC would add
// nothing here (research S-08).
func memberReqHasRussian(s string) bool {
	for _, r := range norm.NFC.String(s) {
		if memberReqRussianLetters(r) {
			return true
		}
	}
	return false
}

// memberReqHasArabic reports whether s has a letter of the Arabic script. Digits and
// marks of that script are not letters and do not count.
func memberReqHasArabic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// memberReqHasHan reports whether s has any Han code point, which includes the kanji of
// Japanese names. Kana and Hangul are other scripts and do not count.
func memberReqHasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// memberReqRules is the rule table in rule order: last name, username, Russian, Arabic,
// Chinese. A later phase appends rows (the next free bit is 32); the order is the order of
// the settings page and of the {rules} list in a notice.
var memberReqRules = []memberReqRule{
	{Code: "ln", Bit: 1, Store: memberreq.RuleLastName, LineKey: "settings_mreq_rule_lastname", CellKey: "settings_btn_mreq_lastname", DescKey: "settings_mreq_desc_lastname", PhraseKey: "mreq_rule_lastname",
		Broken: func(n memberReqNames) bool { return memberReqBlank(n.Last) }},
	{Code: "un", Bit: 2, Store: memberreq.RuleUsername, LineKey: "settings_mreq_rule_username", CellKey: "settings_btn_mreq_username", DescKey: "settings_mreq_desc_username", PhraseKey: "mreq_rule_username",
		Broken: func(n memberReqNames) bool { return n.Username == "" }},
	{Code: "ru", Bit: 4, Store: memberreq.RuleRussian, LineKey: "settings_mreq_rule_russian", CellKey: "settings_btn_mreq_russian", DescKey: "settings_mreq_desc_russian", PhraseKey: "mreq_rule_russian",
		Broken: func(n memberReqNames) bool { return memberReqHasRussian(n.First) || memberReqHasRussian(n.Last) }},
	{Code: "ar", Bit: 8, Store: memberreq.RuleArabic, LineKey: "settings_mreq_rule_arabic", CellKey: "settings_btn_mreq_arabic", DescKey: "settings_mreq_desc_arabic", PhraseKey: "mreq_rule_arabic",
		Broken: func(n memberReqNames) bool { return memberReqHasArabic(n.First) || memberReqHasArabic(n.Last) }},
	{Code: "zh", Bit: 16, Store: memberreq.RuleChinese, LineKey: "settings_mreq_rule_chinese", CellKey: "settings_btn_mreq_chinese", DescKey: "settings_mreq_desc_chinese", PhraseKey: "mreq_rule_chinese",
		Broken: func(n memberReqNames) bool { return memberReqHasHan(n.First) || memberReqHasHan(n.Last) }},
}

// memberReqRuleByCode finds a rule by its callback code (ln un ru ar zh).
func memberReqRuleByCode(code string) (memberReqRule, bool) {
	for _, rule := range memberReqRules {
		if rule.Code == code {
			return rule, true
		}
	}
	return memberReqRule{}, false
}

// memberReqVisible reports whether r is something a person can see: a letter, number,
// punctuation or symbol that is not a default-ignorable code point and not the Braille
// blank U+2800 (a symbol that draws nothing). Whitespace, format characters (zero-width
// space and joiners, direction marks, the byte order mark, soft hyphen), marks and the
// Hangul fillers are not visible, so none of them can stand in for a name. U+FFFC is
// visible because it renders as a box.
func memberReqVisible(r rune) bool {
	return unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S) &&
		!unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) && r != 0x2800
}

// memberReqBlank reports whether s has no visible character, which is the test of "no
// last name" (and of a first name that cannot be shown). An empty string is blank.
func memberReqBlank(s string) bool {
	for _, r := range s {
		if memberReqVisible(r) {
			return false
		}
	}
	return true
}

// memberReqBroken returns the mask of the rules that are on for this chat (action not
// Off) and that the member's names break. Rules that are off are never listed, so a
// caller sees only what it may act on.
func memberReqBroken(settings *models.ChatRequirements, names memberReqNames) int {
	if settings == nil {
		return 0
	}
	mask := 0
	for _, rule := range memberReqRules {
		if memberreq.ActionOf(settings, rule.Store) == models.RequirementActionOff {
			continue
		}
		if rule.Broken(names) {
			mask |= rule.Bit
		}
	}
	return mask
}

// memberReqSeverity ranks a punishment: ban 4, kick 3, mute 2, warn 1, off (and any value
// it does not know) 0. The stored values are warn 1, kick 2, ban 3, mute 4, which are not
// in order of severity, so a comparison never uses them directly (D-04).
func memberReqSeverity(action int) int {
	switch action {
	case models.RequirementActionBan:
		return 4
	case models.RequirementActionKick:
		return 3
	case models.RequirementActionMute:
		return 2
	case models.RequirementActionWarn:
		return 1
	}
	return 0
}

// memberReqStrictest returns the punishment of the strictest rule in the broken mask,
// chosen by memberReqSeverity: ban over kick over mute over warn. With no broken rule it
// returns RequirementActionOff.
func memberReqStrictest(settings *models.ChatRequirements, broken int) int {
	if settings == nil {
		return models.RequirementActionOff
	}
	best := models.RequirementActionOff
	for _, rule := range memberReqRules {
		if broken&rule.Bit == 0 {
			continue
		}
		action := memberreq.ActionOf(settings, rule.Store)
		if memberReqSeverity(action) > memberReqSeverity(best) {
			best = action
		}
	}
	return best
}
