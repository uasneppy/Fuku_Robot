package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfirmedRequiresMatchingHash(t *testing.T) {
	p, _ := LoadProgress(filepath.Join(t.TempDir(), "none.txt"))
	p.Confirm("ban_done", []string{"User banned"})
	if !p.Confirmed("ban_done", []string{"User banned"}) {
		t.Fatal("a freshly confirmed key must read as confirmed")
	}
	if p.Confirmed("ban_done", []string{"User was banned"}) {
		t.Fatal("changing the English must re-open the key")
	}
	if p.Confirmed("other", []string{"User banned"}) {
		t.Fatal("an unknown key is not confirmed")
	}
}

func TestListKeyHashCoversEveryItem(t *testing.T) {
	a := SourceHash([]string{"one", "two", "three"})
	b := SourceHash([]string{"one", "two", "THREE"})
	c := SourceHash([]string{"one", "two three"})
	if a == b || a == c {
		t.Fatalf("hashes must differ when any item changes or items merge: %s %s %s", a, b, c)
	}
	if len(a) != 8 {
		t.Fatalf("want 8 hex characters, got %q", a)
	}
}

func TestSaveIsSortedAndStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "confirmed.txt")
	p, _ := LoadProgress(path)
	p.Confirm("zeta", []string{"z"})
	p.Confirm("alpha", []string{"a"})
	p.Confirm("mid_key-1", []string{"m"})
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	again, err := LoadProgress(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Save(path); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatalf("save, load, save changed the bytes:\n%s\n---\n%s", first, second)
	}
	var keys []string
	for _, line := range strings.Split(string(first), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			keys = append(keys, strings.Split(line, "\t")[0])
		}
	}
	if strings.Join(keys, ",") != "alpha,mid_key-1,zeta" {
		t.Fatalf("keys not sorted: %v", keys)
	}
	if !strings.HasPrefix(string(first), "#") {
		t.Fatal("the file should start with a header comment saying what it is")
	}
}

func TestLoadRejectsMalformedLine(t *testing.T) {
	cases := map[string]string{
		"no tab":          "# c\nalpha\n",
		"short hash":      "alpha\tabc\n",
		"non-hex hash":    "alpha\tzzzzzzzz\n",
		"extra column":    "alpha\t01234567\textra\n",
		"duplicate key":   "alpha\t01234567\nalpha\t89abcdef\n",
		"conflict marker": "<<<<<<< HEAD\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "c.txt")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadProgress(path)
			if err == nil || !strings.Contains(err.Error(), "line") {
				t.Fatalf("want an error naming the line, got %v", err)
			}
		})
	}
}

func TestLoadIgnoresCommentsAndBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.txt")
	if err := os.WriteFile(path, []byte("# header\n\nalpha\t01234567\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProgress(path)
	if err != nil || len(p.Keys()) != 1 {
		t.Fatalf("got %v, %v", p, err)
	}
}

func TestUnconfirmRemovesTheKey(t *testing.T) {
	p, _ := LoadProgress(filepath.Join(t.TempDir(), "none.txt"))
	p.Confirm("a", []string{"x"})
	p.Unconfirm("a")
	p.Unconfirm("never-there")
	if p.Confirmed("a", []string{"x"}) || len(p.Keys()) != 0 {
		t.Fatal("key should be gone")
	}
}
