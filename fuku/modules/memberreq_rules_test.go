//go:build testtools

package modules

import (
	"testing"
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
