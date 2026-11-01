package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

// ErrUnknownKey is wrapped by Save and Confirm for a key that en.yml does not have.
var ErrUnknownKey = errors.New("unknown key")

// ValidationError carries the blocking issues that stopped a save.
type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		msgs[i] = is.Message
	}
	return "translation refused: " + strings.Join(msgs, "; ")
}

// KeyState is everything the editor needs to show one key.
type KeyState struct {
	Key       string   `json:"key"`
	Module    string   `json:"module"`
	EN        []string `json:"en"`
	UK        []string `json:"uk"`
	List      bool     `json:"list"`
	Confirmed bool     `json:"confirmed"`
	// Reopened: it was confirmed once, but the English changed since.
	Reopened bool    `json:"reopened"`
	Blocking []Issue `json:"blocking"`
	Warnings []Issue `json:"warnings"`
	Hints    []Term  `json:"hints"`
	// Tokens are the distinct placeholders and tags of the English text, so the page
	// can show which ones the Ukrainian is still missing.
	Tokens []string `json:"tokens"`
}

type ModuleSummary struct {
	Name      string `json:"name"`
	Confirmed int    `json:"confirmed"`
	Total     int    `json:"total"`
}

type State struct {
	Keys    []KeyState      `json:"keys"`
	Modules []ModuleSummary `json:"modules"`
	// Orphans are keys in uk.yml that en.yml no longer has.
	Orphans []string `json:"orphans"`
}

// Project ties the files together. Every call re-reads them from disk, so a git
// pull or an edit in another program is never hidden behind a cached copy; the
// mutex only keeps the tool's own read-modify-write cycles apart.
type Project struct {
	EnPath, RuPath, UkPath, ProgressPath, GlossaryPath string
	mu                                                 sync.Mutex
}

type sources struct {
	en, ru, uk []Entry
	prog       *Progress
	gloss      *Glossary
}

// ModuleOf groups keys the way the editor lists them: the text before the first "_".
func ModuleOf(key string) string {
	if i := strings.Index(key, "_"); i > 0 {
		return key[:i]
	}
	return "other"
}

func loadOptional(path string) ([]Entry, error) {
	entries, err := LoadLocale(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

func (p *Project) read() (*sources, error) {
	var s sources
	var err error
	if s.en, err = LoadLocale(p.EnPath); err != nil {
		return nil, err
	}
	if s.ru, err = loadOptional(p.RuPath); err != nil {
		return nil, err
	}
	if s.uk, err = LoadLocale(p.UkPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist yet: run with -init first", p.UkPath)
		}
		return nil, err
	}
	if s.prog, err = LoadProgress(p.ProgressPath); err != nil {
		return nil, err
	}
	if s.gloss, err = LoadGlossary(p.GlossaryPath); err != nil {
		return nil, err
	}
	return &s, nil
}

func indexEntries(entries []Entry) map[string]Entry {
	m := make(map[string]Entry, len(entries))
	for _, e := range entries {
		m[e.Key] = e
	}
	return m
}

// draftEntry is the starting text for a key that uk.yml does not have yet: the
// Russian if there is some, else the English, so the file is always complete.
func draftEntry(en Entry, ruBy map[string]Entry) Entry {
	if r, ok := ruBy[en.Key]; ok {
		return Entry{Key: en.Key, Parts: slices.Clone(r.Parts), List: r.List}
	}
	return Entry{Key: en.Key, Parts: slices.Clone(en.Parts), List: en.List}
}

// merge lines uk up with en: English order, uk's text where it has some, a draft for
// the rest. Keys only uk has are returned as orphans and left out.
func merge(en, ru, uk []Entry) (merged []Entry, missing, orphans []string) {
	ukBy, ruBy := indexEntries(uk), indexEntries(ru)
	enSet := make(map[string]bool, len(en))
	for _, e := range en {
		enSet[e.Key] = true
		if u, ok := ukBy[e.Key]; ok {
			merged = append(merged, u)
			continue
		}
		missing = append(missing, e.Key)
		merged = append(merged, draftEntry(e, ruBy))
	}
	for _, u := range uk {
		if !enSet[u.Key] {
			orphans = append(orphans, u.Key)
		}
	}
	return merged, missing, orphans
}

func tokensOf(en []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range en {
		for _, t := range append(PlaceholdersOf(part), TagsOf(part)...) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

// keyStateFor judges one key's text; Preview uses it alone so a keystroke does not
// re-judge all 1,400 keys.
func keyStateFor(s *sources, en, uk Entry) KeyState {
	k := KeyState{
		Key: en.Key, Module: ModuleOf(en.Key), EN: en.Parts, UK: uk.Parts, List: en.List,
		Confirmed: s.prog.Confirmed(en.Key, en.Parts),
		Blocking:  BlockingIssues(en.Parts, uk.Parts),
		Hints:     s.gloss.Hints(en.Parts),
		Tokens:    tokensOf(en.Parts),
	}
	k.Reopened = !k.Confirmed && s.prog.Has(en.Key)
	k.Warnings = append(k.Warnings, Warnings(en.Parts, uk.Parts)...)
	k.Warnings = append(k.Warnings, s.gloss.Check(en.Parts, uk.Parts)...)
	k.Warnings = append(k.Warnings, s.gloss.FlagRussian(uk.Parts)...)
	return k
}

func buildState(s *sources) *State {
	merged, _, orphans := merge(s.en, s.ru, s.uk)
	st := &State{Orphans: orphans}
	mods := map[string]*ModuleSummary{}
	for i, e := range s.en {
		k := keyStateFor(s, e, merged[i])
		st.Keys = append(st.Keys, k)
		m := mods[k.Module]
		if m == nil {
			m = &ModuleSummary{Name: k.Module}
			mods[k.Module] = m
		}
		m.Total++
		if k.Confirmed {
			m.Confirmed++
		}
	}
	for _, m := range mods {
		st.Modules = append(st.Modules, *m)
	}
	sort.Slice(st.Modules, func(i, j int) bool { return st.Modules[i].Name < st.Modules[j].Name })
	return st
}

// Load returns the current state. It never writes.
func (p *Project) Load() (*State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.read()
	if err != nil {
		return nil, err
	}
	return buildState(s), nil
}

var seedTerms = []string{
	"ban", "unban", "mute", "unmute", "kick", "warn", "captcha", "lockdown", "staff group",
	"approved user", "admin", "federation", "blacklist", "antiflood", "filter", "note",
	"rules", "report", "log channel", "raid",
}

var seedRussianWords = []string{"что", "это", "или", "как", "только", "если", "чтобы", "когда"}

func glossarySeed() string {
	var b strings.Builder
	b.WriteString("# Glossary for scripts/translate_uk.\n")
	b.WriteString("# Fill in \"uk\" with the stem you want for each term (a stem, because Ukrainian inflects:\n")
	b.WriteString("# \"бан\" matches бан, бану, забанити). The editor warns when a text that contains the\n")
	b.WriteString("# English term does not use it. List rejected variants under \"forbidden\". A term with an\n")
	b.WriteString("# empty \"uk\" is ignored. russian_words are flagged as likely Russian leftovers.\n")
	b.WriteString("terms:\n")
	for _, t := range seedTerms {
		fmt.Fprintf(&b, "  - en: %s\n    uk: \"\"\n", t)
	}
	b.WriteString("russian_words: [" + strings.Join(seedRussianWords, ", ") + "]\n")
	return b.String()
}

// Init writes the draft uk.yml from ru.yml (English where Russian has no key), an
// empty progress file, and a glossary seed if there is none. It refuses to touch an
// existing uk.yml without force; force also empties the progress file, because a
// confirmation of text that was just replaced would be a lie.
func (p *Project) Init(force bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := os.Stat(p.UkPath); err == nil && !force {
		return fmt.Errorf("%s already exists; -init would overwrite your translation. Use -init -force only to start over (it also clears the confirmed list)", p.UkPath)
	}
	en, err := LoadLocale(p.EnPath)
	if err != nil {
		return err
	}
	ru, err := loadOptional(p.RuPath)
	if err != nil {
		return err
	}
	draft, _, _ := merge(en, ru, nil)
	own := map[string]string{"lang_sample": "Українська", "language_flag": "🇺🇦", "language_name": "Ukrainian"}
	for i := range draft {
		if v, ok := own[draft[i].Key]; ok {
			draft[i].Parts = []string{v}
		}
	}
	if err := WriteLocale(p.UkPath, draft); err != nil {
		return err
	}
	if err := (&Progress{hashes: map[string]string{}}).Save(p.ProgressPath); err != nil {
		return err
	}
	if _, err := os.Stat(p.GlossaryPath); errors.Is(err, os.ErrNotExist) {
		return writeFileAtomic(p.GlossaryPath, []byte(glossarySeed()))
	}
	return nil
}

// Sync appends the keys that en.yml has and uk.yml lacks (text from ru.yml, else
// English, unconfirmed) and returns how many. It leaves everything else alone and
// writes nothing when there is nothing to add.
func (p *Project) Sync() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.read()
	if err != nil {
		return 0, err
	}
	ukBy, ruBy := indexEntries(s.uk), indexEntries(s.ru)
	out := slices.Clone(s.uk)
	added := 0
	for _, e := range s.en {
		if _, ok := ukBy[e.Key]; !ok {
			out = append(out, draftEntry(e, ruBy))
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	return added, WriteLocale(p.UkPath, out)
}

// Preview judges candidate text for a key exactly as Save would, but writes nothing,
// so the editor can show problems while the owner types. The returned state carries
// the candidate text and the key's stored confirmation, not a new one.
func (p *Project) Preview(key string, uk []string) (KeyState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.read()
	if err != nil {
		return KeyState{}, err
	}
	pos := slices.IndexFunc(s.en, func(e Entry) bool { return e.Key == key })
	if pos < 0 {
		return KeyState{}, fmt.Errorf("%w %q", ErrUnknownKey, key)
	}
	return keyStateFor(s, s.en[pos], Entry{Key: key, Parts: slices.Clone(uk), List: s.en[pos].List}), nil
}

// Save stores a translation without confirming it. Changed text loses any
// confirmation; identical text keeps it.
func (p *Project) Save(key string, uk []string) (KeyState, error) { return p.write(key, uk, false) }

// Confirm stores the translation and marks the key confirmed against the English
// text as it is now.
func (p *Project) Confirm(key string, uk []string) (KeyState, error) { return p.write(key, uk, true) }

func (p *Project) write(key string, uk []string, confirm bool) (KeyState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.read()
	if err != nil {
		return KeyState{}, err
	}
	pos := slices.IndexFunc(s.en, func(e Entry) bool { return e.Key == key })
	if pos < 0 {
		return KeyState{}, fmt.Errorf("%w %q", ErrUnknownKey, key)
	}
	en := s.en[pos]
	if issues := BlockingIssues(en.Parts, uk); len(issues) > 0 {
		return KeyState{}, &ValidationError{Issues: issues}
	}
	merged, _, _ := merge(s.en, s.ru, s.uk)
	changed := !slices.Equal(merged[pos].Parts, uk)
	merged[pos] = Entry{Key: key, Parts: slices.Clone(uk), List: en.List}

	enSet := make(map[string]bool, len(s.en))
	for _, e := range s.en {
		enSet[e.Key] = true
	}
	for _, k := range s.prog.Keys() {
		if !enSet[k] {
			s.prog.Unconfirm(k)
		}
	}
	// The order of the two writes decides which half-finished state a crash leaves:
	// a confirmation must never outlive the text it was given for, and must never
	// precede it.
	if confirm {
		if err := WriteLocale(p.UkPath, merged); err != nil {
			return KeyState{}, err
		}
		s.prog.Confirm(key, en.Parts)
		if err := s.prog.Save(p.ProgressPath); err != nil {
			return KeyState{}, err
		}
	} else {
		if changed {
			s.prog.Unconfirm(key)
		}
		if err := s.prog.Save(p.ProgressPath); err != nil {
			return KeyState{}, err
		}
		if err := WriteLocale(p.UkPath, merged); err != nil {
			return KeyState{}, err
		}
	}
	s.uk = merged
	return buildState(s).Keys[pos], nil
}

var apostropheRe = regexp.MustCompile(`[а-яіїєґА-ЯІЇЄҐ](['’ʼ])[а-яіїєґА-ЯІЇЄҐ]`)

// Check reports progress and whether the translation is finished: every key
// confirmed, no blocking issue, no orphan, one apostrophe character throughout.
func (p *Project) Check() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.read()
	if err != nil {
		return "cannot check: " + err.Error() + "\n", false
	}
	st := buildState(s)
	var b strings.Builder
	var unconfirmed, blocked []string
	var blockedMsgs []string
	apostrophes := map[string]int{}
	apostropheKeys := map[string][]string{}
	warned, confirmed := 0, 0
	for _, k := range st.Keys {
		if k.Confirmed {
			confirmed++
			for _, part := range k.UK {
				for _, m := range apostropheRe.FindAllStringSubmatch(part, -1) {
					apostrophes[m[1]]++
					if keys := apostropheKeys[m[1]]; len(keys) == 0 || keys[len(keys)-1] != k.Key {
						apostropheKeys[m[1]] = append(keys, k.Key)
					}
				}
			}
		} else {
			unconfirmed = append(unconfirmed, k.Key)
		}
		if len(k.Blocking) > 0 {
			blocked = append(blocked, k.Key)
			blockedMsgs = append(blockedMsgs, fmt.Sprintf("  %s: %s", k.Key, k.Blocking[0].Message))
		}
		if len(k.Warnings) > 0 {
			warned++
		}
	}
	fmt.Fprintf(&b, "Ukrainian translation: %d / %d keys confirmed\n\n", confirmed, len(st.Keys))
	for _, m := range st.Modules {
		fmt.Fprintf(&b, "  %-24s %4d / %-4d\n", m.Name, m.Confirmed, m.Total)
	}
	ok := true
	if len(unconfirmed) > 0 {
		ok = false
		shown := unconfirmed
		if len(shown) > 25 {
			shown = shown[:25]
		}
		fmt.Fprintf(&b, "\nNot confirmed yet (%d): %s", len(unconfirmed), strings.Join(shown, ", "))
		if len(unconfirmed) > len(shown) {
			b.WriteString(", ...")
		}
		b.WriteString("\n")
	}
	if len(blocked) > 0 {
		ok = false
		fmt.Fprintf(&b, "\nTexts that would break a message (%d):\n%s\n", len(blocked), strings.Join(blockedMsgs, "\n"))
	}
	if len(st.Orphans) > 0 {
		ok = false
		fmt.Fprintf(&b, "\nKeys in uk.yml that en.yml no longer has (%d): %s\n", len(st.Orphans), strings.Join(st.Orphans, ", "))
	}
	if len(apostrophes) > 1 {
		ok = false
		// The most used character wins; the owner has to fix the keys that use any
		// other one, so those are the ones named (counts alone would send them
		// grepping through a thousand keys).
		chars := make([]string, 0, len(apostrophes))
		for ch := range apostrophes {
			chars = append(chars, ch)
		}
		sort.Slice(chars, func(i, j int) bool {
			if apostrophes[chars[i]] != apostrophes[chars[j]] {
				return apostrophes[chars[i]] > apostrophes[chars[j]]
			}
			return chars[i] < chars[j]
		})
		var counts []string
		for _, ch := range chars {
			counts = append(counts, fmt.Sprintf("%s x%d", ch, apostrophes[ch]))
		}
		var fix []string
		for _, ch := range chars[1:] {
			fix = append(fix, apostropheKeys[ch]...)
		}
		shown := fix
		if len(shown) > 25 {
			shown = shown[:25]
		}
		fmt.Fprintf(&b, "\nMixed apostrophe characters in confirmed texts (%s). Change these to %s: %s", strings.Join(counts, ", "), chars[0], strings.Join(shown, ", "))
		if len(fix) > len(shown) {
			fmt.Fprintf(&b, ", ... (%d more)", len(fix)-len(shown))
		}
		b.WriteString("\n")
	}
	if warned > 0 {
		fmt.Fprintf(&b, "\n%d keys have warnings (these never block).\n", warned)
	}
	if ok {
		fmt.Fprintf(&b, "\nDone: all %d keys are confirmed and valid.\n", len(st.Keys))
	}
	return b.String(), ok
}
