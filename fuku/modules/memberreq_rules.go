package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"golang.org/x/text/unicode/norm"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
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
// how the rule is judged. Exactly one of Broken and Lookup is set on every row. A name rule has
// a pure Broken test of the member's names; it judges the first and the last name and never the
// username. A lookup rule (the photo, the channel and the AI rule) has a Lookup function that
// asks Telegram or the AI provider and answers a verdict, and no Broken: only
// memberReqLookupBroken calls it, and only an explicit memberReqBreach adds the rule's bit. HintKey, when set, is a locale key the rule's
// settings screen shows right after the description (the blank last name hint, the photo rule's
// Warn or Mute recommendation); it is the one field a renderer reads in place of a test of the
// rule's identity.
type memberReqRule struct {
	Code      string
	Bit       int
	Store     memberreq.Rule
	LineKey   string
	CellKey   string
	DescKey   string
	PhraseKey string
	Broken    func(memberReqNames) bool
	Lookup    func(ctx context.Context, b *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict
	HintKey   string
	// NamesBound is true on a rule whose verdict is about the names the lookups judged (the AI rule):
	// the executor drops its bit when the member's names changed between the lookup and the claim
	// (D-29), so a member is never punished for a string they no longer carry.
	NamesBound bool
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
// Chinese, profile photo, channel subscription, AI spam names. A later phase appends rows (the
// next free bit is 256); the order is the order of the settings page and of the {rules} list in a notice, and it
// equals memberreq.Rules.
var memberReqRules = []memberReqRule{
	{Code: "ln", Bit: 1, Store: memberreq.RuleLastName, LineKey: "settings_mreq_rule_lastname", CellKey: "settings_btn_mreq_lastname", DescKey: "settings_mreq_desc_lastname", PhraseKey: "mreq_rule_lastname", HintKey: "settings_mreq_hint_blank",
		Broken: func(n memberReqNames) bool { return memberReqBlank(n.Last) }},
	{Code: "un", Bit: 2, Store: memberreq.RuleUsername, LineKey: "settings_mreq_rule_username", CellKey: "settings_btn_mreq_username", DescKey: "settings_mreq_desc_username", PhraseKey: "mreq_rule_username",
		Broken: func(n memberReqNames) bool { return n.Username == "" }},
	{Code: "ru", Bit: 4, Store: memberreq.RuleRussian, LineKey: "settings_mreq_rule_russian", CellKey: "settings_btn_mreq_russian", DescKey: "settings_mreq_desc_russian", PhraseKey: "mreq_rule_russian",
		Broken: func(n memberReqNames) bool { return memberReqHasRussian(n.First) || memberReqHasRussian(n.Last) }},
	{Code: "ar", Bit: 8, Store: memberreq.RuleArabic, LineKey: "settings_mreq_rule_arabic", CellKey: "settings_btn_mreq_arabic", DescKey: "settings_mreq_desc_arabic", PhraseKey: "mreq_rule_arabic",
		Broken: func(n memberReqNames) bool { return memberReqHasArabic(n.First) || memberReqHasArabic(n.Last) }},
	{Code: "zh", Bit: 16, Store: memberreq.RuleChinese, LineKey: "settings_mreq_rule_chinese", CellKey: "settings_btn_mreq_chinese", DescKey: "settings_mreq_desc_chinese", PhraseKey: "mreq_rule_chinese",
		Broken: func(n memberReqNames) bool { return memberReqHasHan(n.First) || memberReqHasHan(n.Last) }},
	{Code: "ph", Bit: 32, Store: memberreq.RulePhoto, LineKey: "settings_mreq_rule_photo", CellKey: "settings_btn_mreq_photo", DescKey: "settings_mreq_desc_photo", PhraseKey: "mreq_rule_photo",
		Lookup: memberReqPhotoRule, HintKey: "settings_mreq_hint_photo"},
	{Code: "ch", Bit: 64, Store: memberreq.RuleChannel, LineKey: "settings_mreq_rule_channel", CellKey: "settings_btn_mreq_channel", DescKey: "settings_mreq_desc_channel", PhraseKey: "mreq_rule_channel",
		Lookup: memberReqChannelRule},
	{Code: "ai", Bit: 128, Store: memberreq.RuleAI, LineKey: "settings_mreq_rule_ai", CellKey: "settings_btn_mreq_ai", DescKey: "settings_mreq_desc_ai", PhraseKey: "mreq_rule_ai",
		Lookup: memberReqAIRule, HintKey: "settings_mreq_hint_ai", NamesBound: true},
}

// memberReqNamesBoundMask is the mask of the rules whose verdict is bound to the names they judged
// (the rows with NamesBound).
func memberReqNamesBoundMask() int {
	mask := 0
	for _, rule := range memberReqRules {
		if rule.NamesBound {
			mask |= rule.Bit
		}
	}
	return mask
}

// memberReqRuleByCode finds a rule by its callback code (ln un ru ar zh ph ch ai).
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
// Off) and that the member's names break. It judges the name rules only: a row with no Broken
// function (the lookup rules) is skipped, so it adds no bit and a nil function is never called.
// Rules that are off are never listed, so a caller sees only what it may act on.
func memberReqBroken(settings *models.ChatRequirements, names memberReqNames) int {
	if settings == nil {
		return 0
	}
	mask := 0
	for _, rule := range memberReqRules {
		if memberreq.ActionOf(settings, rule.Store) == models.RequirementActionOff {
			continue
		}
		if rule.Broken != nil && rule.Broken(names) {
			mask |= rule.Bit
		}
	}
	return mask
}

// memberReqLookupBroken folds the verdicts of the lookup rules into a broken mask, in table
// order (photo, channel, then the AI rule). It skips a row with no Lookup (a name rule) and a row whose stored
// action is Off, and asks the others with the chat, the member and the mode it was given. A
// memberReqBreach sets the rule's bit; a memberReqUnknown (also the zero verdict) sets the
// returned unknown flag and no bit; memberReqPass and memberReqInert add nothing. A lookup can
// therefore add a bit only through an explicit breach, so a failed or unanswered lookup never
// leads to a punishment (D-02, MREQ-10). It calls no network function itself and judges no rule
// by its identity; where it is called is the executor's decision.
func memberReqLookupBroken(
	ctx context.Context, b *gotgbot.Bot, settings *models.ChatRequirements, chatID, userID int64, mode memberReqLookupMode,
) (mask int, unknown bool) {
	mask, unknownMask := memberReqLookupMasks(ctx, b, settings, chatID, userID, mode)
	return mask, unknownMask != 0
}

// memberReqLookupMasks is memberReqLookupBroken with the unknown verdicts as a mask of the rules
// that could not be answered (bit per rule), not a flag, so the executor can tell which rules
// are still unjudged (WR-02). The broken mask is the same: a bit only through an explicit breach.
func memberReqLookupMasks(
	ctx context.Context, b *gotgbot.Bot, settings *models.ChatRequirements, chatID, userID int64, mode memberReqLookupMode,
) (broken, unknown int) {
	if settings == nil {
		return 0, 0
	}
	for _, rule := range memberReqRules {
		if rule.Lookup == nil || memberreq.ActionOf(settings, rule.Store) == models.RequirementActionOff {
			continue
		}
		switch rule.Lookup(context.WithValue(ctx, memberReqLookupBitKey{}, rule.Bit), b, chatID, userID, mode) {
		case memberReqBreach:
			broken |= rule.Bit
		case memberReqUnknown:
			unknown |= rule.Bit
		}
	}
	return broken, unknown
}

// memberReqUnknownDefers reports whether the rules that could not be answered (the unknown mask)
// include one whose punishment is stricter, by memberReqSeverity, than the strictest punishment
// of the rules found broken (WR-02). With a breach established and such a rule unjudged, claiming
// the hold now would let the weaker punishment and its hold stand and keep the stricter rule from
// ever being judged (a held member's messages make no lookup), so the executor claims nothing and
// the member is judged again at their next message. An unknown rule that is equal or weaker than
// the punishment chosen defers nothing, and with no breach at all there is nothing to defer.
func memberReqUnknownDefers(settings *models.ChatRequirements, broken, unknown int) bool {
	if settings == nil || broken == 0 || unknown == 0 {
		return false
	}
	chosen := memberReqSeverity(memberReqStrictest(settings, broken))
	for _, rule := range memberReqRules {
		if unknown&rule.Bit != 0 && memberReqSeverity(memberreq.ActionOf(settings, rule.Store)) > chosen {
			return true
		}
	}
	return false
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

// memberReqPassValue is a fingerprint of everything that decides whether a member is
// clean in a chat: the eight actions in rule order and the three names. It is the first 16
// hex characters of a SHA-256, not a secret. The watcher stores it per member so it can
// skip the clean-path DELETE while nothing changed, and any rename or settings change
// gives another value (D-12, D-20). Each field is length-prefixed, so no name can slide
// into its neighbour. Nil settings read as every rule off.
func memberReqPassValue(settings *models.ChatRequirements, names memberReqNames) string {
	if settings == nil {
		settings = &models.ChatRequirements{}
	}
	var sb strings.Builder
	for _, rule := range memberreq.Rules {
		sb.WriteString(strconv.Itoa(memberreq.ActionOf(settings, rule)))
		sb.WriteByte(';')
	}
	for _, field := range []string{names.First, names.Last, names.Username} {
		sb.WriteString(strconv.Itoa(len(field)))
		sb.WriteByte(':')
		sb.WriteString(field)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// memberReqNameCap is the most runes of a name a notice carries. The isolates are added
// around it, so a link text has at most memberReqNameCap + 2 code points.
const memberReqNameCap = 64

// memberReqDisplayName chooses the name a notice shows for u: the first name when it has
// a visible character (the test of the last-name rule), else "@" and the username, else
// nothing (false), and the caller supplies its own text.
func memberReqDisplayName(u gotgbot.User) (string, bool) {
	if !memberReqBlank(u.FirstName) {
		return u.FirstName, true
	}
	if u.Username != "" {
		return "@" + u.Username, true
	}
	return "", false
}

// memberReqStripBidi drops the bidirectional controls U+061C, U+200E, U+200F, U+202A to
// U+202E and U+2066 to U+2069 from s and keeps every other character (a zero-width joiner and
// every letter stay). A name that carried U+2069 could close the isolate memberReqMention
// opens, and a U+202E after it would reorder the rest of the notice line.
func memberReqStripBidi(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x061c, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			return -1
		}
		return r
	}, s)
}

// memberReqMention names u in a group notice as a link to the user (so the member is
// notified), with the chosen name, or fallback when there is none, stripped of its
// bidirectional controls (memberReqStripBidi), cut to 64 runes and wrapped in the isolates
// U+2068 and U+2069 so a right-to-left name cannot reorder the sentence around it and cannot
// close the isolate itself. A name that is empty after the strip shows the fallback.
// formatting.MentionHtml escapes the text. Only the display is stripped: the rules judge the
// original name. The result is spliced into a notice after translation; a member's name never
// passes through the translator. It logs nothing.
func memberReqMention(u gotgbot.User, fallback string) string {
	name, ok := memberReqDisplayName(u)
	if !ok {
		name = fallback
	}
	name = memberReqStripBidi(name)
	if name == "" {
		name = memberReqStripBidi(fallback)
	}
	if runes := []rune(name); len(runes) > memberReqNameCap {
		name = string(runes[:memberReqNameCap])
	}
	return formatting.MentionHtml(u.Id, "\u2068"+name+"\u2069")
}
