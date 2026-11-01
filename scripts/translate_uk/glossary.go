package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Term is one glossary entry. Ukrainian is a stem, matched as a case-insensitive
// substring, because Ukrainian inflects (бан, бану, забанити) and a whole-word match
// would warn on nearly every correct sentence.
type Term struct {
	English   string   `yaml:"en" json:"en"`
	Ukrainian string   `yaml:"uk" json:"uk"`
	Forbidden []string `yaml:"forbidden,omitempty" json:"forbidden,omitempty"`
}

// Glossary is the owner's list of fixed renderings plus the Russian function words
// that betray a draft pre-filled from ru.yml.
type Glossary struct {
	Terms        []Term   `yaml:"terms"`
	RussianWords []string `yaml:"russian_words"`
}

// LoadGlossary reads path; a missing file is an empty glossary so the tool works
// before the owner has created one.
func LoadGlossary(path string) (*Glossary, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Glossary{}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Glossary
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &g, nil
}

// words splits on anything that is not a letter or digit. It does not use a
// regexp \b: RE2's word boundary is ASCII-only, so on Cyrillic it never fires.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// termIn reports whether the English term occurs as whole words, allowing a plural
// "s"/"es" on its last word ("bans" counts, "banner" does not).
func termIn(term string, enWords []string) bool {
	tw := words(term)
	if len(tw) == 0 {
		return false
	}
	last := len(tw) - 1
	for i := 0; i+len(tw) <= len(enWords); i++ {
		ok := true
		for j, w := range tw {
			got := enWords[i+j]
			if j == last {
				ok = got == w || got == w+"s" || got == w+"es"
			} else {
				ok = got == w
			}
			if !ok {
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func partPrefix(total, i int) string {
	if total > 1 {
		return fmt.Sprintf("item %d: ", i+1)
	}
	return ""
}

// Hints returns the terms present in the English text, for the editor's side panel.
func (g *Glossary) Hints(en []string) []Term {
	var all []string
	for _, p := range en {
		all = append(all, words(p)...)
	}
	var out []Term
	for _, t := range g.Terms {
		if termIn(t.English, all) {
			out = append(out, t)
		}
	}
	return out
}

// Check warns when the English contains a term and the Ukrainian does not use the
// chosen rendering, or uses a forbidden variant. A term with no Ukrainian yet and
// no forbidden variants never warns.
func (g *Glossary) Check(en, uk []string) []Issue {
	var out []Issue
	n := min(len(en), len(uk))
	for i := 0; i < n; i++ {
		enWords := words(en[i])
		ukLower := strings.ToLower(uk[i])
		for _, t := range g.Terms {
			if !termIn(t.English, enWords) {
				continue
			}
			if t.Ukrainian != "" && !strings.Contains(ukLower, strings.ToLower(t.Ukrainian)) {
				out = append(out, Issue{"glossary", fmt.Sprintf("%s\"%s\" should be rendered \"%s\" (glossary), but this text does not use it", partPrefix(len(en), i), t.English, t.Ukrainian)})
			}
			for _, f := range t.Forbidden {
				if f != "" && strings.Contains(ukLower, strings.ToLower(f)) {
					out = append(out, Issue{"glossary", fmt.Sprintf("%s\"%s\" must not be rendered \"%s\" (glossary)", partPrefix(len(en), i), t.English, f)})
				}
			}
		}
	}
	return out
}

// FlagRussian flags whole words from the owner's list of Russian function words,
// which catch pre-fill leftovers that the letter check (ы э ъ ё) cannot see.
func (g *Glossary) FlagRussian(uk []string) []Issue {
	set := make(map[string]bool, len(g.RussianWords))
	for _, w := range g.RussianWords {
		set[strings.ToLower(w)] = true
	}
	var out []Issue
	for i, part := range uk {
		seen := map[string]bool{}
		var found []string
		for _, w := range words(part) {
			if set[w] && !seen[w] {
				seen[w] = true
				found = append(found, w)
			}
		}
		if len(found) > 0 {
			out = append(out, Issue{"ru_word", partPrefix(len(uk), i) + "looks Russian: " + strings.Join(found, " ")})
		}
	}
	return out
}
