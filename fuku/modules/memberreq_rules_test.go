//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// corpusRow is one member of the Unicode corpus (research S-08, probe-verified): the first
// and last name as Telegram sends them and the verdict each name rule must give. A pair of
// names is judged as first || last, never on the username.
type corpusRow struct {
	group string
	first string
	last  string
	ru    bool
	ar    bool
	han   bool
}

// memberReqCorpus is the pinned corpus. Its verdicts come from the research probe and are
// never edited to make the code pass.
//
// Accepted limits, documented here on purpose (research S-08): a Latin "ë" (U+00EB) written
// in place of "ё" evades the Russian rule, and characters added after Unicode 15.0 (CJK
// Extension I, Arabic letters of Unicode 16.0) are not detected until Go updates its tables.
var memberReqCorpus = []corpusRow{
	// Ukrainian: must pass the Russian rule (D-10, T-13-07). Ukrainian uses none of ы ъ э ё.
	{group: "ukrainian", first: "Олександр", last: "Ковальчук"},
	{group: "ukrainian", first: "Тарас", last: "Шевченко"},
	{group: "ukrainian", first: "Ірина", last: "Їжакевич"},
	{group: "ukrainian", first: "Євген", last: "Ґалаґан"},
	{group: "ukrainian", first: "Наталія", last: "Бойко"},
	{group: "ukrainian", first: "Андрій", last: "Мельник"},
	{group: "ukrainian", first: "Віталій", last: "Кличко"},
	{group: "ukrainian", first: "Юлія", last: "Тимошенко"},
	{group: "ukrainian", first: "Марʼяна", last: "Зозуля"},
	{group: "ukrainian", first: "Мар'яна", last: "Під'ячий"},
	{group: "ukrainian", first: "Олена", last: "Пчілка"},
	{group: "ukrainian", first: "Ґудзь", last: "Єлизавета"},
	{group: "ukrainian", first: "Їжак"},
	{group: "ukrainian", first: "Леся", last: "Українка"},
	{group: "ukrainian", first: "Богдан", last: "Хмельницький"},

	// Russian with the letters: caught.
	{group: "russian", first: "Эдуард", ru: true},
	{group: "russian", first: "Пётр", last: "Быков", ru: true},
	{group: "russian", first: "Семён", ru: true},
	{group: "russian", first: "Артём", last: "Рыбаков", ru: true},
	{group: "russian", first: "Объедков", ru: true},
	{group: "russian", first: "ЁЖИК", ru: true},
	{group: "russian", first: "Фёдор", last: "Достоевский", ru: true},
	{group: "russian decomposed", first: "Пётр", ru: true},
	{group: "russian decomposed", first: "Ёлка", ru: true},
	{group: "russian", first: "Ёлка", ru: true},

	// Russian without them: not caught (the rule is letter-based and the page says so).
	{group: "russian without", first: "Иван", last: "Петров"},
	{group: "russian without", first: "Мария", last: "Иванова"},
	{group: "russian without", first: "Александр", last: "Смирнов"},

	// Belarusian and Bulgarian: caught only when they carry one of the letters.
	{group: "belarusian", first: "Аляксандр", last: "Лукашэнка", ru: true},
	{group: "belarusian", first: "Рыгор", last: "Барадулін", ru: true},
	{group: "belarusian", first: "Святлана", last: "Ціханоўская"},
	{group: "bulgarian", first: "Ангел", last: "Бъчваров", ru: true},
	{group: "bulgarian", first: "Ъгъл", ru: true},
	{group: "bulgarian", first: "Цветан", last: "Цветков"},

	// Other Cyrillic: not caught.
	{group: "other cyrillic", first: "Новак", last: "Ђоковић"},
	{group: "other cyrillic", first: "Нұрсұлтан", last: "Назарбаев"},
	{group: "other cyrillic", first: "Сүхбаатар"},

	// Arabic script: Arabic, Urdu, Persian, presentation forms.
	{group: "arabic", first: "محمد", ar: true},
	{group: "arabic", first: "Ahmed", last: "محمد", ar: true},
	{group: "arabic urdu", first: "عمران", last: "خان", ar: true},
	{group: "arabic persian", first: "محمد", last: "رضا شجریان", ar: true},
	{group: "arabic presentation", first: "ﷲ", ar: true},
	{group: "arabic presentation", first: "ﻣﺤﻤﺪ", ar: true},

	// Not Arabic letters: digits and a lone mark.
	{group: "not arabic", first: "٣٤٥"},
	{group: "not arabic", first: "ً"},

	// Han: Chinese, Japanese kanji, Korean hanja, radicals, compatibility ideographs.
	{group: "han", first: "王伟", han: true},
	{group: "han", first: "陳大文", han: true},
	{group: "han", first: "田中", last: "太郎", han: true},
	{group: "han", first: "佐々木", han: true},
	{group: "han", first: "々", han: true},
	{group: "han", first: "〇", han: true},
	{group: "han", first: "⼀", han: true},
	{group: "han", first: "丽", han: true},
	{group: "han extension B", first: "\U00020000", han: true},

	// Not Han: kana and Hangul.
	{group: "kana", first: "さくら"},
	{group: "kana", first: "サクラ"},
	{group: "hangul", first: "김민준"},

	// Latin and mixed script.
	{group: "latin", first: "Alex"},
	{group: "latin", first: "Alex", last: "Иванов"},
	{group: "latin", first: "José", last: "Müller"},
	{group: "latin", first: "Zoë"},
	{group: "mixed", first: "Alex", last: "Пётр", ru: true},
	{group: "mixed", first: "Ёzhik", ru: true},
	// The accepted evasion: a Latin ë (U+00EB) in place of ё is not the letter ё.
	{group: "latin evasion", first: "Пëтр"},

	// A zero-width character inside a name changes no verdict.
	{group: "zero-width", first: "Пе​тр"},
	{group: "zero-width", first: "Э​", ru: true},
	{group: "zero-width", first: "​王", han: true},
}

func TestMemberReqRulesCorpus(t *testing.T) {
	ruRule, okRu := memberReqRuleByCode("ru")
	arRule, okAr := memberReqRuleByCode("ar")
	zhRule, okZh := memberReqRuleByCode("zh")
	if !okRu || !okAr || !okZh {
		t.Fatalf("rule table lacks ru, ar or zh: %v %v %v", okRu, okAr, okZh)
	}

	for _, row := range memberReqCorpus {
		row := row
		t.Run(row.group+"/"+row.first+" "+row.last, func(t *testing.T) {
			names := memberReqNames{First: row.first, Last: row.last}

			if got := memberReqHasRussian(row.first) || memberReqHasRussian(row.last); got != row.ru {
				t.Errorf("memberReqHasRussian(%q, %q) = %v, want %v", row.first, row.last, got, row.ru)
			}
			if got := memberReqHasArabic(row.first) || memberReqHasArabic(row.last); got != row.ar {
				t.Errorf("memberReqHasArabic(%q, %q) = %v, want %v", row.first, row.last, got, row.ar)
			}
			if got := memberReqHasHan(row.first) || memberReqHasHan(row.last); got != row.han {
				t.Errorf("memberReqHasHan(%q, %q) = %v, want %v", row.first, row.last, got, row.han)
			}

			// The rule table drives the verdicts: each row's Broken agrees with the classifier.
			if got := ruRule.Broken(names); got != row.ru {
				t.Errorf("ru Broken(%q, %q) = %v, want %v", row.first, row.last, got, row.ru)
			}
			if got := arRule.Broken(names); got != row.ar {
				t.Errorf("ar Broken(%q, %q) = %v, want %v", row.first, row.last, got, row.ar)
			}
			if got := zhRule.Broken(names); got != row.han {
				t.Errorf("zh Broken(%q, %q) = %v, want %v", row.first, row.last, got, row.han)
			}
		})
	}
}

// invisibleRunes is every invisible or whitespace-like code point of research S-08's table
// that the last-name rule must count as no last name.
func invisibleRunes() []rune {
	var out []rune
	add := func(lo, hi rune) {
		for r := lo; r <= hi; r++ {
			out = append(out, r)
		}
	}
	add(0x20, 0x20)
	add(0xA0, 0xA0)
	add(0x2000, 0x200A)
	add(0x202F, 0x202F)
	add(0x205F, 0x205F)
	add(0x3000, 0x3000)
	add(0x2028, 0x2029)
	add(0xAD, 0xAD)
	add(0x61C, 0x61C)
	add(0x180E, 0x180E)
	add(0x200B, 0x200F)
	add(0x202A, 0x202E)
	add(0x2060, 0x2064)
	add(0x2066, 0x2069)
	add(0x206F, 0x206F)
	add(0xFEFF, 0xFEFF)
	add(0x1D173, 0x1D173)
	add(0xE0020, 0xE0020)
	add(0x115F, 0x115F)
	add(0x1160, 0x1160)
	add(0x3164, 0x3164)
	add(0xFFA0, 0xFFA0)
	add(0x2800, 0x2800)
	add(0x34F, 0x34F)
	add(0x17B4, 0x17B5)
	add(0x300, 0x300)
	add(0x308, 0x308)
	add(0xFE0F, 0xFE0F)
	add(0xE0100, 0xE0100)
	return out
}

func TestMemberReqBlankLastName(t *testing.T) {
	lastName, ok := memberReqRuleByCode("ln")
	if !ok {
		t.Fatal("rule table lacks ln")
	}

	blankNames := []string{""}
	all := invisibleRunes()
	for _, r := range all {
		blankNames = append(blankNames, string(r))
	}
	// Mixed: every pair of neighbours, and all of them together.
	for i := 0; i+1 < len(all); i++ {
		blankNames = append(blankNames, string(all[i])+string(all[i+1]))
	}
	blankNames = append(blankNames, string(all))

	for _, name := range blankNames {
		if !memberReqBlank(name) {
			t.Errorf("memberReqBlank(%q) = false, want true", name)
		}
		if !lastName.Broken(memberReqNames{First: "Alex", Last: name, Username: "alex"}) {
			t.Errorf("last-name rule not broken by blank last name %q", name)
		}
	}

	visibleNames := []string{
		"Ковальчук",
		"\u0640",       // tatweel: Common script, a visible stretch
		"\ufffc",       // object replacement: renders as a box
		"\u200bК",      // a zero-width character beside a letter
		" K ",          // spaces around a letter
		"\u2800a",      // the Braille blank beside a letter
		"1",            // a digit
		".",            // punctuation
		"\u3164\u0416", // a Hangul filler beside a letter
	}
	for _, name := range visibleNames {
		if memberReqBlank(name) {
			t.Errorf("memberReqBlank(%q) = true, want false", name)
		}
		if lastName.Broken(memberReqNames{First: "", Last: name, Username: ""}) {
			t.Errorf("last-name rule broken by visible last name %q", name)
		}
	}
}

func TestMemberReqUsernameRule(t *testing.T) {
	username, ok := memberReqRuleByCode("un")
	if !ok {
		t.Fatal("rule table lacks un")
	}

	if !username.Broken(memberReqNames{}) {
		t.Error("an empty username must break the username rule")
	}
	if username.Broken(memberReqNames{Username: "alex"}) {
		t.Error("a username must not break the username rule")
	}
	for _, first := range []string{"Alex", "Пётр", "محمد", "王伟", "\u200b", ""} {
		for _, last := range []string{"", "Ковальчук", "\u2800", "Иванов"} {
			if !username.Broken(memberReqNames{First: first, Last: last}) {
				t.Errorf("names %q %q with no username must break the rule", first, last)
			}
			if username.Broken(memberReqNames{First: first, Last: last, Username: "u1"}) {
				t.Errorf("names %q %q with a username must not break the rule", first, last)
			}
		}
	}

	// The name rules judge the first and last name and never the username.
	for _, name := range []string{"Пётр", "محمد", "王伟", "\u200b"} {
		names := memberReqNames{First: "Alex", Last: "Ivanov", Username: name}
		for _, code := range []string{"ln", "ru", "ar", "zh"} {
			rule, found := memberReqRuleByCode(code)
			if !found {
				t.Fatalf("rule table lacks %s", code)
			}
			if rule.Broken(names) {
				t.Errorf("rule %s broken by the username %q", code, name)
			}
		}
	}

	got := memberReqNamesOf(gotgbot.User{Id: 7, FirstName: "A", LastName: "B", Username: "c"})
	if got != (memberReqNames{First: "A", Last: "B", Username: "c"}) {
		t.Errorf("memberReqNamesOf = %+v", got)
	}
}

func TestMemberReqRuleTable(t *testing.T) {
	wantCodes := []string{"ln", "un", "ru", "ar", "zh"}
	if len(memberReqRules) != len(wantCodes) {
		t.Fatalf("%d rules, want %d", len(memberReqRules), len(wantCodes))
	}

	seenCode := map[string]bool{}
	seenBit := map[int]bool{}
	for i, rule := range memberReqRules {
		if rule.Code != wantCodes[i] {
			t.Errorf("rule %d code = %q, want %q", i, rule.Code, wantCodes[i])
		}
		if seenCode[rule.Code] {
			t.Errorf("duplicate code %q", rule.Code)
		}
		seenCode[rule.Code] = true

		if rule.Bit != 1<<i {
			t.Errorf("rule %q bit = %d, want %d", rule.Code, rule.Bit, 1<<i)
		}
		if rule.Bit <= 0 || rule.Bit&(rule.Bit-1) != 0 || seenBit[rule.Bit] {
			t.Errorf("rule %q bit %d is not a distinct power of two", rule.Code, rule.Bit)
		}
		seenBit[rule.Bit] = true

		if rule.Store != memberreq.Rules[i] {
			t.Errorf("rule %q store = %q, want %q", rule.Code, rule.Store, memberreq.Rules[i])
		}
		for name, key := range map[string]string{"LineKey": rule.LineKey, "CellKey": rule.CellKey, "DescKey": rule.DescKey, "PhraseKey": rule.PhraseKey} {
			if key == "" {
				t.Errorf("rule %q has an empty %s", rule.Code, name)
			}
		}
		if rule.Broken == nil {
			t.Errorf("rule %q has no Broken test", rule.Code)
		}

		found, ok := memberReqRuleByCode(rule.Code)
		if !ok || found.Code != rule.Code || found.Bit != rule.Bit {
			t.Errorf("memberReqRuleByCode(%q) = %+v, %v", rule.Code, found, ok)
		}
	}

	for _, code := range []string{"xx", "", "LN", "last_name"} {
		if _, ok := memberReqRuleByCode(code); ok {
			t.Errorf("memberReqRuleByCode(%q) found a rule", code)
		}
	}
}

// requirementsOf builds settings with one action per rule, in rule order.
func requirementsOf(actions ...int) *models.ChatRequirements {
	return &models.ChatRequirements{
		LastNameAction: actions[0],
		UsernameAction: actions[1],
		RussianAction:  actions[2],
		ArabicAction:   actions[3],
		ChineseAction:  actions[4],
	}
}

func TestMemberReqSeverity(t *testing.T) {
	order := []int{
		models.RequirementActionOff,
		models.RequirementActionWarn,
		models.RequirementActionMute,
		models.RequirementActionKick,
		models.RequirementActionBan,
	}
	for i, action := range order {
		if got := memberReqSeverity(action); got != i {
			t.Errorf("memberReqSeverity(%d) = %d, want %d", action, got, i)
		}
	}
	for _, unknown := range []int{-1, 5, 99} {
		if got := memberReqSeverity(unknown); got != 0 {
			t.Errorf("memberReqSeverity(%d) = %d, want 0", unknown, got)
		}
	}
}

func TestMemberReqStrictest(t *testing.T) {
	// A member who breaks every rule: no last name, no username, a Russian letter, an
	// Arabic letter and a Han character.
	everything := memberReqNames{First: "Эдуард محمد 王", Last: "\u200b"}
	allBits := 0
	for _, rule := range memberReqRules {
		allBits |= rule.Bit
	}

	if got := memberReqBroken(requirementsOf(0, 0, 0, 0, 0), everything); got != 0 {
		t.Errorf("broken with every rule off = %d, want 0", got)
	}
	if got := memberReqBroken(nil, everything); got != 0 {
		t.Errorf("broken with nil settings = %d, want 0", got)
	}
	if got := memberReqBroken(requirementsOf(1, 1, 1, 1, 1), everything); got != allBits {
		t.Errorf("broken with every rule on = %d, want %d", got, allBits)
	}
	// A broken rule set to Off is absent from the mask.
	if got := memberReqBroken(requirementsOf(models.RequirementActionWarn, 0, 0, models.RequirementActionMute, 0), everything); got != 1|8 {
		t.Errorf("broken = %d, want %d (last name and Arabic only)", got, 1|8)
	}
	// A rule that is on but not broken is absent too.
	if got := memberReqBroken(requirementsOf(1, 1, 1, 1, 1), memberReqNames{First: "Alex", Last: "Ivanov", Username: "alex"}); got != 0 {
		t.Errorf("broken for a clean member = %d, want 0", got)
	}

	// No broken rule gives Off.
	if got := memberReqStrictest(requirementsOf(1, 2, 3, 4, 1), 0); got != models.RequirementActionOff {
		t.Errorf("strictest of no broken rule = %d, want Off", got)
	}

	// The precedence, written down here independently of the stored numbers
	// (warn 1, kick 2, ban 3, mute 4): ban over kick over mute over warn.
	precedence := []int{
		models.RequirementActionBan,
		models.RequirementActionKick,
		models.RequirementActionMute,
		models.RequirementActionWarn,
	}
	rank := func(action int) int {
		for i, candidate := range precedence {
			if candidate == action {
				return i
			}
		}
		t.Fatalf("action %d not in the precedence list", action)
		return -1
	}

	checked := 0
	for subset := 1; subset < 1<<len(memberReqRules); subset++ {
		var chosen []int
		for i := range memberReqRules {
			if subset&(1<<i) != 0 {
				chosen = append(chosen, i)
			}
		}
		if len(chosen) < 2 || len(chosen) > 3 {
			continue
		}
		assignments := 1
		for range chosen {
			assignments *= len(precedence)
		}
		for n := 0; n < assignments; n++ {
			actions := make([]int, len(memberReqRules))
			code := n
			want := precedence[len(precedence)-1]
			for _, idx := range chosen {
				actions[idx] = precedence[code%len(precedence)]
				code /= len(precedence)
				if rank(actions[idx]) < rank(want) {
					want = actions[idx]
				}
			}
			settings := requirementsOf(actions...)
			broken := memberReqBroken(settings, everything)
			if broken != subset {
				t.Fatalf("subset %b actions %v: broken = %b", subset, actions, broken)
			}
			if got := memberReqStrictest(settings, broken); got != want {
				t.Fatalf("subset %b actions %v: strictest = %d, want %d", subset, actions, got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no combination was checked")
	}
}
