package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const progressHeader = "# Confirmed Ukrainian translations: key<TAB>first 8 hex characters of sha256(English text).\n" +
	"# Written by scripts/translate_uk. Delete a line to un-confirm that key.\n"

// SourceHash fingerprints the English text of a key (every item of a list). A
// confirmation is stored with it, so a later edit to the English re-opens the key
// instead of silently keeping a translation of text that no longer exists.
func SourceHash(en []string) string {
	sum := sha256.Sum256([]byte(strings.Join(en, "\x00")))
	return hex.EncodeToString(sum[:])[:8]
}

// Progress is the set of keys the owner has confirmed, each with the hash of the
// English text it was confirmed against.
type Progress struct {
	hashes map[string]string
}

func isHex8(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LoadProgress reads path; a missing file is empty. A malformed line is an error
// that names its number, because the usual cause is an unresolved git merge and
// guessing past it would drop confirmations.
func LoadProgress(path string) (*Progress, error) {
	p := &Progress{hashes: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "# ") || line == "#" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || fields[0] == "" || !isHex8(fields[1]) {
			return nil, fmt.Errorf("%s: line %d is not \"key<TAB>8 hex characters\": %q", path, n, line)
		}
		if _, dup := p.hashes[fields[0]]; dup {
			return nil, fmt.Errorf("%s: line %d repeats key %q", path, n, fields[0])
		}
		p.hashes[fields[0]] = fields[1]
	}
	return p, sc.Err()
}

// Confirmed is true only while the stored hash still matches the English.
func (p *Progress) Confirmed(key string, en []string) bool {
	h, ok := p.hashes[key]
	return ok && h == SourceHash(en)
}

// Has is true when the key has a stored confirmation, whether or not it still matches.
func (p *Progress) Has(key string) bool { _, ok := p.hashes[key]; return ok }

func (p *Progress) Confirm(key string, en []string) { p.hashes[key] = SourceHash(en) }

func (p *Progress) Unconfirm(key string) { delete(p.hashes, key) }

// Keys returns every key with a stored confirmation, sorted.
func (p *Progress) Keys() []string {
	keys := make([]string, 0, len(p.hashes))
	for k := range p.hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Save writes the file sorted by key so the diff of each translation session is
// only the keys that changed.
func (p *Progress) Save(path string) error {
	var buf bytes.Buffer
	buf.WriteString(progressHeader)
	for _, k := range p.Keys() {
		fmt.Fprintf(&buf, "%s\t%s\n", k, p.hashes[k])
	}
	return writeFileAtomic(path, buf.Bytes())
}
