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
		Broken: func(memberReqNames) bool { return false }},
	{Code: "un", Bit: 2, Store: memberreq.RuleUsername, LineKey: "settings_mreq_rule_username", CellKey: "settings_btn_mreq_username", DescKey: "settings_mreq_desc_username", PhraseKey: "mreq_rule_username",
		Broken: func(memberReqNames) bool { return false }},
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

func memberReqBlank(string) bool { return false }

func memberReqBroken(*models.ChatRequirements, memberReqNames) int { return 0 }

func memberReqSeverity(int) int { return 0 }

func memberReqStrictest(*models.ChatRequirements, int) int { return 0 }
