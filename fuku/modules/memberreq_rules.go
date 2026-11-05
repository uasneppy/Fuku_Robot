package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
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

// memberReqRule is one row of the rule table.
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

func memberReqHasRussian(string) bool { return false }

func memberReqHasArabic(string) bool { return false }

func memberReqHasHan(string) bool { return false }

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

// memberReqRuleByCode finds a rule by its callback code.
func memberReqRuleByCode(code string) (memberReqRule, bool) {
	for _, rule := range memberReqRules {
		if rule.Code == code {
			return rule, true
		}
	}
	return memberReqRule{}, false
}
