//go:build testtools

package modules

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
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
	{group: "zero-width", first: "Пе\u200bтр"},
	{group: "zero-width", first: "Э\u200b", ru: true},
	{group: "zero-width", first: "\u200b王", han: true},
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
	wantCodes := []string{"ln", "un", "ru", "ar", "zh", "ph", "ch"}
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
		// Exactly one of the two judges the rule: a name rule has a pure Broken test, a lookup rule
		// (the photo and the channel) has a Lookup function and no Broken (D-13).
		if (rule.Broken == nil) == (rule.Lookup == nil) {
			t.Errorf("rule %q has Broken %v and Lookup %v, want exactly one of them", rule.Code, rule.Broken != nil, rule.Lookup != nil)
		}
		if wantLookup := i >= 5; (rule.Lookup != nil) != wantLookup {
			t.Errorf("rule %q Lookup set = %v, want %v (only photo and channel look up)", rule.Code, rule.Lookup != nil, wantLookup)
		}

		found, ok := memberReqRuleByCode(rule.Code)
		if !ok || found.Code != rule.Code || found.Bit != rule.Bit {
			t.Errorf("memberReqRuleByCode(%q) = %+v, %v", rule.Code, found, ok)
		}
	}

	if memberReqRules[5].Bit != 32 || memberReqRules[6].Bit != 64 {
		t.Errorf("the lookup rules have bits %d and %d, want 32 and 64", memberReqRules[5].Bit, memberReqRules[6].Bit)
	}
	if memberReqRules[0].HintKey == "" || memberReqRules[5].HintKey == "" {
		t.Error("the last-name and photo rules must carry a hint key")
	}
	for _, code := range []string{"xx", "", "LN", "last_name"} {
		if _, ok := memberReqRuleByCode(code); ok {
			t.Errorf("memberReqRuleByCode(%q) found a rule", code)
		}
	}
}

// TestMemberReqRuleKeysExist translates every key the rule table names in all 8 real locales: the
// keys are table fields and not literals, so make check-translations cannot see them and a typo
// would ship an empty message.
func TestMemberReqRuleKeysExist(t *testing.T) {
	withRealLocales(t)
	for _, code := range memberReqScreenLanguages {
		tr := i18n.MustNewTranslator(code)
		for _, rule := range memberReqRules {
			for name, key := range map[string]string{
				"LineKey": rule.LineKey, "CellKey": rule.CellKey, "DescKey": rule.DescKey,
				"PhraseKey": rule.PhraseKey, "HintKey": rule.HintKey,
			} {
				if key == "" {
					if name != "HintKey" {
						t.Errorf("%s: rule %s has an empty %s", code, rule.Code, name)
					}
					continue
				}
				text, err := tr.GetString(key, i18n.TranslationParams{"state": "x"})
				if err != nil || strings.TrimSpace(text) == "" {
					t.Errorf("%s: rule %s %s %q does not translate: %q, %v", code, rule.Code, name, key, text, err)
				}
			}
		}
	}
}

// lookupStubTable replaces the lookup rows of memberReqRules with recording stubs for one test and
// restores the table in t.Cleanup. verdicts maps a rule code to the verdict its stub answers; a code
// with no entry answers the zero verdict. The returned slice records every call as "code:chat:user:fresh:classify".
func lookupStubTable(t *testing.T, verdicts map[string]memberReqVerdict) *[]string {
	t.Helper()
	saved := memberReqRules
	t.Cleanup(func() { memberReqRules = saved })
	calls := &[]string{}
	stubbed := make([]memberReqRule, len(saved))
	copy(stubbed, saved)
	for i := range stubbed {
		if stubbed[i].Lookup == nil {
			continue
		}
		code := stubbed[i].Code
		stubbed[i].Lookup = func(_ context.Context, _ *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict {
			*calls = append(*calls, fmt.Sprintf("%s:%d:%d:%v:%v", code, chatID, userID, mode.Fresh, mode.Classify))
			return verdicts[code]
		}
	}
	memberReqRules = stubbed
	return calls
}

// TestMemberReqLookupBroken proves the fold of lookup verdicts into a mask: table order, rows with
// no Lookup and rows that are Off skipped, only memberReqBreach adds a bit, memberReqUnknown sets
// the flag and no bit, memberReqPass and memberReqInert add nothing (D-02, MREQ-10). It is the only
// caller of memberReqLookupBroken until plan 14-06 folds it into the executor.
func TestMemberReqLookupBroken(t *testing.T) {
	ctx := context.Background()
	const chatID, userID int64 = -100777, 4711
	mode := memberReqLookupMode{Fresh: true, Classify: true}
	on := requirementsOf(1, 1, 1, 1, 1, models.RequirementActionWarn, models.RequirementActionMute)

	t.Run("every lookup breaching sets both bits, called in table order with the arguments given", func(t *testing.T) {
		calls := lookupStubTable(t, map[string]memberReqVerdict{"ph": memberReqBreach, "ch": memberReqBreach})
		mask, unknown := memberReqLookupBroken(ctx, nil, on, chatID, userID, mode)
		if mask != 32|64 || unknown {
			t.Errorf("mask = %d, unknown = %v, want 96 and false", mask, unknown)
		}
		want := []string{fmt.Sprintf("ph:%d:%d:true:true", chatID, userID), fmt.Sprintf("ch:%d:%d:true:true", chatID, userID)}
		if fmt.Sprint(*calls) != fmt.Sprint(want) {
			t.Errorf("calls = %v, want %v (photo first, then channel, name rules never)", *calls, want)
		}
	})

	t.Run("one verdict per rule", func(t *testing.T) {
		for _, c := range []struct {
			name         string
			photo, chann memberReqVerdict
			wantMask     int
			wantUnknown  bool
		}{
			{"photo breach only", memberReqBreach, memberReqPass, 32, false},
			{"channel breach only", memberReqPass, memberReqBreach, 64, false},
			{"pass and inert add nothing", memberReqPass, memberReqInert, 0, false},
			{"inert and inert add nothing", memberReqInert, memberReqInert, 0, false},
			{"unknown sets the flag and no bit", memberReqUnknown, memberReqPass, 0, true},
			{"a breach beside an unknown keeps the bit and the flag", memberReqBreach, memberReqUnknown, 32, true},
			{"the zero verdict is unknown", memberReqVerdict(0), memberReqVerdict(0), 0, true},
		} {
			lookupStubTable(t, map[string]memberReqVerdict{"ph": c.photo, "ch": c.chann})
			mask, unknown := memberReqLookupBroken(ctx, nil, on, chatID, userID, mode)
			if mask != c.wantMask || unknown != c.wantUnknown {
				t.Errorf("%s: mask = %d, unknown = %v, want %d and %v", c.name, mask, unknown, c.wantMask, c.wantUnknown)
			}
		}
	})

	t.Run("a rule that is Off is never called", func(t *testing.T) {
		calls := lookupStubTable(t, map[string]memberReqVerdict{"ph": memberReqBreach, "ch": memberReqBreach})
		photoOff := requirementsOf(1, 1, 1, 1, 1, models.RequirementActionOff, models.RequirementActionKick)
		mask, unknown := memberReqLookupBroken(ctx, nil, photoOff, chatID, userID, mode)
		if mask != 64 || unknown || len(*calls) != 1 || (*calls)[0][:2] != "ch" {
			t.Errorf("mask = %d, unknown = %v, calls = %v, want only the channel rule asked", mask, unknown, *calls)
		}
		*calls = nil
		if mask, unknown = memberReqLookupBroken(ctx, nil, requirementsOf(1, 1, 1, 1, 1), chatID, userID, mode); mask != 0 || unknown || len(*calls) != 0 {
			t.Errorf("both lookup rules Off: mask = %d, unknown = %v, calls = %v, want nothing asked", mask, unknown, *calls)
		}
	})

	t.Run("nil settings ask nothing", func(t *testing.T) {
		calls := lookupStubTable(t, map[string]memberReqVerdict{"ph": memberReqBreach, "ch": memberReqBreach})
		if mask, unknown := memberReqLookupBroken(ctx, nil, nil, chatID, userID, mode); mask != 0 || unknown || len(*calls) != 0 {
			t.Errorf("nil settings: mask = %d, unknown = %v, calls = %v, want 0, false and no call", mask, unknown, *calls)
		}
	})
}

// requirementsOf builds settings with one action per rule, in rule order: up to seven actions
// (last name, username, Russian, Arabic, Chinese, photo, channel); a missing trailing one is 0.
func requirementsOf(actions ...int) *models.ChatRequirements {
	var all [7]int
	copy(all[:], actions)
	return &models.ChatRequirements{
		LastNameAction: all[0],
		UsernameAction: all[1],
		RussianAction:  all[2],
		ArabicAction:   all[3],
		ChineseAction:  all[4],
		PhotoAction:    all[5],
		ChannelAction:  all[6],
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
	// memberReqBroken judges the name rules only, so its full mask is the five name bits (31);
	// the photo and channel bits (32 and 64) come from memberReqLookupBroken.
	allBits := 0
	for _, rule := range memberReqRules {
		if rule.Broken != nil {
			allBits |= rule.Bit
		}
	}
	if allBits != 31 {
		t.Fatalf("the name rules' mask = %d, want 31", allBits)
	}

	if got := memberReqBroken(requirementsOf(0, 0, 0, 0, 0, 0, 0), everything); got != 0 {
		t.Errorf("broken with every rule off = %d, want 0", got)
	}
	if got := memberReqBroken(nil, everything); got != 0 {
		t.Errorf("broken with nil settings = %d, want 0", got)
	}
	if got := memberReqBroken(requirementsOf(1, 1, 1, 1, 1, 1, 1), everything); got != allBits {
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
	if got := memberReqStrictest(requirementsOf(1, 2, 3, 4, 1, 2, 3), 0); got != models.RequirementActionOff {
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
			// memberReqBroken reports the name bits of the subset only; the whole subset (the
			// photo and channel bits too) goes to memberReqStrictest, so the precedence is
			// checked for every rule including bits 32 and 64.
			broken := memberReqBroken(settings, everything)
			if nameBits := subset & allBits; broken != nameBits {
				t.Fatalf("subset %b actions %v: broken = %b, want the name bits %b", subset, actions, broken, nameBits)
			}
			if got := memberReqStrictest(settings, subset); got != want {
				t.Fatalf("subset %b actions %v: strictest = %d, want %d", subset, actions, got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no combination was checked")
	}
}

// TestMemberReqBrokenIgnoresLookupRules proves memberReqBroken judges the name rules only: the
// photo and channel rows have no Broken function, so they are skipped (a nil call would panic) and
// never add a bit, whatever their stored actions say (D-13, T-14-17).
func TestMemberReqBrokenIgnoresLookupRules(t *testing.T) {
	lookupOn := requirementsOf(0, 0, 0, 0, 0, models.RequirementActionBan, models.RequirementActionKick)
	clean := memberReqNames{First: "Alex", Last: "Ivanov", Username: "alex"}
	if got := memberReqBroken(lookupOn, clean); got != 0 {
		t.Errorf("broken with only the lookup rules on = %d, want 0", got)
	}
	if got := memberReqBroken(lookupOn, memberReqNames{}); got != 0 {
		t.Errorf("broken for empty names with only the lookup rules on = %d, want 0", got)
	}

	mixed := requirementsOf(models.RequirementActionWarn, 0, 0, 0, 0, models.RequirementActionMute, models.RequirementActionBan)
	if got := memberReqBroken(mixed, memberReqNames{First: "Alex", Username: "alex"}); got != 1 {
		t.Errorf("broken = %d, want the last-name bit (1) alone", got)
	}
}

// TestMemberReqStrictestLookupBits proves the lookup bits take part in the precedence: 32 alone,
// 64 alone and 32|64 give the strictest of the stored photo and channel actions.
func TestMemberReqStrictestLookupBits(t *testing.T) {
	const photo, channel = 32, 64
	actions := []int{models.RequirementActionWarn, models.RequirementActionMute, models.RequirementActionKick, models.RequirementActionBan}
	for _, p := range actions {
		for _, c := range actions {
			settings := requirementsOf(0, 0, 0, 0, 0, p, c)
			if got := memberReqStrictest(settings, photo); got != p {
				t.Errorf("photo alone with photo %d channel %d: strictest = %d, want %d", p, c, got, p)
			}
			if got := memberReqStrictest(settings, channel); got != c {
				t.Errorf("channel alone with photo %d channel %d: strictest = %d, want %d", p, c, got, c)
			}
			want := p
			if memberReqSeverity(c) > memberReqSeverity(p) {
				want = c
			}
			if got := memberReqStrictest(settings, photo|channel); got != want {
				t.Errorf("both with photo %d channel %d: strictest = %d, want %d", p, c, got, want)
			}
		}
	}
	if got := memberReqStrictest(requirementsOf(0, 0, 0, 0, 0, models.RequirementActionBan, models.RequirementActionBan), 0); got != models.RequirementActionOff {
		t.Errorf("strictest of an empty mask = %d, want Off", got)
	}
}

func TestMemberReqPassValue(t *testing.T) {
	base := requirementsOf(1, 2, 3, 4, 0, 1, 2)
	names := memberReqNames{First: "Олена", Last: "Пчілка", Username: "olena"}

	got := memberReqPassValue(base, names)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(got) {
		t.Fatalf("pass value %q is not 16 lowercase hex characters", got)
	}
	if again := memberReqPassValue(requirementsOf(1, 2, 3, 4, 0, 1, 2), names); again != got {
		t.Errorf("same settings and names gave %q then %q", got, again)
	}

	// Any one of the seven actions, the photo and the channel included, changes the value.
	for i := 0; i < 7; i++ {
		actions := []int{1, 2, 3, 4, 0, 1, 2}
		actions[i] = (actions[i] + 1) % 5
		if memberReqPassValue(requirementsOf(actions...), names) == got {
			t.Errorf("changing the action of rule %d did not change the pass value", i)
		}
	}

	// Any one of the three names changes it, and so does a username of one space.
	variants := map[string]memberReqNames{
		"first":          {First: "Олена!", Last: names.Last, Username: names.Username},
		"last":           {First: names.First, Last: "Пчілка!", Username: names.Username},
		"username":       {First: names.First, Last: names.Last, Username: "olena2"},
		"empty username": {First: names.First, Last: names.Last},
	}
	seen := map[string]string{got: "base"}
	for label, variant := range variants {
		value := memberReqPassValue(base, variant)
		if other, dup := seen[value]; dup {
			t.Errorf("%s gave the pass value of %s", label, other)
		}
		seen[value] = label
	}
	if memberReqPassValue(base, memberReqNames{First: "A"}) == memberReqPassValue(base, memberReqNames{First: "A", Username: " "}) {
		t.Error("an empty username and a username of one space gave the same value")
	}

	// A name cannot slide into its neighbour.
	if memberReqPassValue(base, memberReqNames{First: "a", Last: "bc"}) == memberReqPassValue(base, memberReqNames{First: "ab", Last: "c"}) {
		t.Error("moving a character from the last name to the first name kept the value")
	}
	if memberReqPassValue(base, memberReqNames{First: "a", Last: "", Username: "b"}) == memberReqPassValue(base, memberReqNames{First: "a", Last: "b", Username: ""}) {
		t.Error("moving a name from the username to the last name kept the value")
	}

	// No settings reads as every rule off.
	if memberReqPassValue(nil, names) != memberReqPassValue(requirementsOf(0, 0, 0, 0, 0, 0, 0), names) {
		t.Error("nil settings must fingerprint like every rule off")
	}
}

// mentionLinkText returns the link text of a mention built by memberReqMention.
func mentionLinkText(t *testing.T, mention string) string {
	t.Helper()
	open := strings.Index(mention, `">`)
	if !strings.HasPrefix(mention, "<a href=\"") || open < 0 || !strings.HasSuffix(mention, "</a>") {
		t.Fatalf("mention %q is not a link", mention)
	}
	return mention[open+2 : len(mention)-len("</a>")]
}

func TestMemberReqMention(t *testing.T) {
	const id int64 = 123456789
	fallback := fmt.Sprintf("user %d", id)

	t.Run("the first name when it has a visible character", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "Олена", LastName: "Пчілка", Username: "olena"}
		name, ok := memberReqDisplayName(u)
		if name != "Олена" || !ok {
			t.Errorf("memberReqDisplayName = %q, %v", name, ok)
		}
		mention := memberReqMention(u, fallback)
		want := `<a href="tg://user?id=123456789">` + "\u2068Олена\u2069" + `</a>`
		if mention != want {
			t.Errorf("mention = %q, want %q", mention, want)
		}
	})

	t.Run("an invisible first name falls back to the username", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "\u200b\u3164\u2800", Username: "olena_u"}
		name, ok := memberReqDisplayName(u)
		if name != "@olena_u" || !ok {
			t.Errorf("memberReqDisplayName = %q, %v", name, ok)
		}
		if text := mentionLinkText(t, memberReqMention(u, fallback)); text != "\u2068@olena_u\u2069" {
			t.Errorf("link text = %q", text)
		}
	})

	t.Run("no visible name and no username falls back to the caller's text", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "\u200b"}
		if name, ok := memberReqDisplayName(u); name != "" || ok {
			t.Errorf("memberReqDisplayName = %q, %v, want empty and false", name, ok)
		}
		mention := memberReqMention(u, fallback)
		if text := mentionLinkText(t, mention); text != "\u2068"+fallback+"\u2069" {
			t.Errorf("link text = %q", text)
		}
		if !strings.Contains(mention, `href="tg://user?id=123456789"`) {
			t.Errorf("mention %q lacks the user link", mention)
		}
	})

	t.Run("a long name is cut to 64 runes before the isolates", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: strings.Repeat("Ж", 70)}
		text := mentionLinkText(t, memberReqMention(u, fallback))
		runes := []rune(text)
		if len(runes) != 66 {
			t.Fatalf("link text has %d runes, want 66", len(runes))
		}
		if runes[0] != '\u2068' || runes[len(runes)-1] != '\u2069' {
			t.Errorf("link text %q is not wrapped in U+2068 and U+2069", text)
		}
		if inner := string(runes[1 : len(runes)-1]); inner != strings.Repeat("Ж", 64) {
			t.Errorf("inner name = %q, want 64 letters", inner)
		}
	})

	t.Run("a name is escaped", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: `<b>&"`}
		mention := memberReqMention(u, fallback)
		if strings.Contains(mention, "<b>") {
			t.Errorf("mention %q has a raw tag", mention)
		}
		if !strings.Contains(mention, "&lt;b&gt;&amp;") {
			t.Errorf("mention %q does not carry the escaped name", mention)
		}
		if strings.Count(mention, "<") != 2 {
			t.Errorf("mention %q has more than the link's own two tags", mention)
		}
	})

	t.Run("bidirectional controls in a name are stripped (NEW-01)", func(t *testing.T) {
		controls := []rune{'\u061c', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069'}
		for _, control := range controls {
			u := gotgbot.User{Id: id, FirstName: "a" + string(control) + "b"}
			if text := mentionLinkText(t, memberReqMention(u, fallback)); text != "\u2068ab\u2069" {
				t.Errorf("name with U+%04X: link text = %q, want the controls dropped and only the wrapper kept", control, text)
			}
		}
	})

	t.Run("a name cannot close its own isolate and reorder the line (NEW-01)", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "x\u2069\u202eevil\u2066\u2067z"}
		mention := memberReqMention(u, fallback)
		text := mentionLinkText(t, mention)
		if text != "\u2068xevilz\u2069" {
			t.Errorf("link text = %q, want only the letters inside the wrapper", text)
		}
		if n := strings.Count(mention, "\u2068"); n != 1 {
			t.Errorf("mention has %d U+2068, want the wrapper's one", n)
		}
		if n := strings.Count(mention, "\u2069"); n != 1 {
			t.Errorf("mention has %d U+2069, want the wrapper's one", n)
		}
		for _, control := range []rune{'\u061c', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067'} {
			if strings.ContainsRune(mention, control) {
				t.Errorf("mention holds U+%04X", control)
			}
		}
	})

	t.Run("the controls go before the cut to 64 runes (NEW-01)", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: strings.Repeat("\u202e", 30) + strings.Repeat("Ж", 70)}
		text := mentionLinkText(t, memberReqMention(u, fallback))
		if inner := strings.TrimSuffix(strings.TrimPrefix(text, "\u2068"), "\u2069"); inner != strings.Repeat("Ж", 64) {
			t.Errorf("inner name = %q, want 64 letters with no control counted in the cut", inner)
		}
	})

	t.Run("escaping survives the strip (NEW-01)", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "<b>\u202e&"}
		mention := memberReqMention(u, fallback)
		if strings.Contains(mention, "<b>") || !strings.Contains(mention, "&lt;b&gt;&amp;") {
			t.Errorf("mention %q is not escaped", mention)
		}
	})

	t.Run("a name made only of controls falls back (NEW-01)", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "\u202e\u2069\u2066\u061c\u200f"}
		mention := memberReqMention(u, fallback)
		if text := mentionLinkText(t, mention); text != "\u2068"+fallback+"\u2069" {
			t.Errorf("link text = %q, want the fallback", text)
		}
		// Even a caller's fallback that carries a control is stripped.
		if text := mentionLinkText(t, memberReqMention(u, "user\u202e 7")); text != "\u2068user 7\u2069" {
			t.Errorf("link text = %q, want the fallback stripped too", text)
		}
	})

	t.Run("an ordinary Arabic name is unchanged (NEW-01)", func(t *testing.T) {
		u := gotgbot.User{Id: id, FirstName: "محمد\u200d علي"}
		if text := mentionLinkText(t, memberReqMention(u, fallback)); text != "\u2068محمد\u200d علي\u2069" {
			t.Errorf("link text = %q, want the name with its joiner kept", text)
		}
	})
}
