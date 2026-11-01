package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newProject copies the fixtures into a temp dir and appends the three language
// keys that Init sets, so tests never touch the checked-in testdata.
func newProject(t *testing.T) *Project {
	t.Helper()
	dir := t.TempDir()
	extraEN := "lang_sample: US English\nlanguage_flag: \"🇺🇸\"\nlanguage_name: English\n"
	extraRU := "lang_sample: Русский\nlanguage_flag: \"🇷🇺\"\nlanguage_name: Russian\n"
	for name, extra := range map[string]string{"en.yml": extraEN, "ru.yml": extraRU} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(data, []byte(extra)...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Project{
		EnPath:       filepath.Join(dir, "en.yml"),
		RuPath:       filepath.Join(dir, "ru.yml"),
		UkPath:       filepath.Join(dir, "uk.yml"),
		ProgressPath: filepath.Join(dir, "confirmed.txt"),
		GlossaryPath: filepath.Join(dir, "glossary.yml"),
	}
}

func mustInit(t *testing.T) *Project {
	t.Helper()
	p := newProject(t)
	if err := p.Init(false); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustLoad(t *testing.T, p *Project) *State {
	t.Helper()
	s, err := p.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func keyOf(t *testing.T, s *State, key string) KeyState {
	t.Helper()
	for _, k := range s.Keys {
		if k.Key == key {
			return k
		}
	}
	t.Fatalf("key %q not in state", key)
	return KeyState{}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// editUK changes one key of uk.yml behind the tool's back, like an editor or a git merge.
func editUK(t *testing.T, p *Project, key string, parts []string) {
	t.Helper()
	entries, err := LoadLocale(p.UkPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i].Key == key {
			entries[i].Parts = parts
		}
	}
	if err := WriteLocale(p.UkPath, entries); err != nil {
		t.Fatal(err)
	}
}

func TestModuleOf(t *testing.T) {
	cases := map[string]string{
		"admin_ok":                  "admin",
		"help_configuration_step-1": "help",
		"captcha_web_close_button":  "captcha",
		"alone":                     "other",
		"_leading":                  "other",
	}
	for key, want := range cases {
		if got := ModuleOf(key); got != want {
			t.Errorf("ModuleOf(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestInitBuildsDraftFromRussianInEnglishOrder(t *testing.T) {
	p := newProject(t)
	if err := p.Init(false); err != nil {
		t.Fatal(err)
	}
	en, _ := LoadLocale(p.EnPath)
	ru, _ := LoadLocale(p.RuPath)
	uk, err := LoadLocale(p.UkPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keysOf(uk), ",") != strings.Join(keysOf(en), ",") {
		t.Fatalf("draft keys %v, want English order %v", keysOf(uk), keysOf(en))
	}
	byKey := func(es []Entry, k string) Entry {
		for _, e := range es {
			if e.Key == k {
				return e
			}
		}
		t.Fatalf("no %s", k)
		return Entry{}
	}
	if got, want := byKey(uk, "admin_ok").Parts, byKey(ru, "admin_ok").Parts; got[0] != want[0] {
		t.Errorf("admin_ok = %q, want the Russian %q", got, want)
	}
	if got := byKey(uk, "only_in_english").Parts[0]; got != "Only here" {
		t.Errorf("a key missing from ru.yml must fall back to English, got %q", got)
	}
	if got := byKey(uk, "misc_runs"); !got.List || len(got.Parts) != 2 {
		t.Errorf("misc_runs = %+v, want a 2-item list", got)
	}
	for key, want := range map[string]string{"lang_sample": "Українська", "language_flag": "🇺🇦", "language_name": "Ukrainian"} {
		if got := byKey(uk, key).Parts[0]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	prog, err := LoadProgress(p.ProgressPath)
	if err != nil || len(prog.Keys()) != 0 {
		t.Errorf("progress should exist and be empty: %v %v", prog, err)
	}
	g, err := LoadGlossary(p.GlossaryPath)
	if err != nil || len(g.Terms) < 15 || len(g.RussianWords) < 5 {
		t.Fatalf("glossary should be seeded: %+v %v", g, err)
	}
	for _, term := range g.Terms {
		if term.Ukrainian != "" {
			t.Errorf("the seed must not pick translations, %q has %q", term.English, term.Ukrainian)
		}
	}
}

func TestInitRefusesToOverwriteAndForceResetsProgressOnly(t *testing.T) {
	p := mustInit(t)
	if _, err := p.Confirm("admin_ok", []string{"Готово <b>%s</b>"}); err != nil {
		t.Fatal(err)
	}
	editUK(t, p, "pseudo", []string{"my own edit"})
	before := readFile(t, p.UkPath)
	if err := p.Init(false); err == nil || !strings.Contains(err.Error(), "force") {
		t.Fatalf("want a refusal that names -force, got %v", err)
	}
	if readFile(t, p.UkPath) != before {
		t.Fatal("a refused Init changed uk.yml")
	}
	if err := os.WriteFile(p.GlossaryPath, []byte("terms: []\nrussian_words: [marker]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Init(true); err != nil {
		t.Fatal(err)
	}
	if readFile(t, p.UkPath) == before {
		t.Fatal("Init(true) should rewrite uk.yml")
	}
	if prog, _ := LoadProgress(p.ProgressPath); len(prog.Keys()) != 0 {
		t.Fatal("Init(true) must empty the progress file")
	}
	if !strings.Contains(readFile(t, p.GlossaryPath), "marker") {
		t.Fatal("Init(true) must leave glossary.yml alone")
	}
}

func TestLoadSyncsNewEnglishKeysAsUnconfirmed(t *testing.T) {
	p := mustInit(t)
	appendTo(t, p.EnPath, "brand_new: Brand new\nboth_new: Both new\n")
	appendTo(t, p.RuPath, "both_new: Обе новые\n")
	before := readFile(t, p.UkPath)
	s := mustLoad(t, p)
	if k := keyOf(t, s, "brand_new"); k.Confirmed || k.UK[0] != "Brand new" {
		t.Errorf("brand_new = %+v, want unconfirmed English text", k)
	}
	if k := keyOf(t, s, "both_new"); k.Confirmed || k.UK[0] != "Обе новые" {
		t.Errorf("both_new = %+v, want unconfirmed Russian text", k)
	}
	if readFile(t, p.UkPath) != before {
		t.Error("Load must not write")
	}
}

func TestLoadReportsRemovedEnglishKeysAsOrphans(t *testing.T) {
	p := mustInit(t)
	en, _ := LoadLocale(p.EnPath)
	var kept []Entry
	for _, e := range en {
		if e.Key != "only_in_english" {
			kept = append(kept, e)
		}
	}
	if err := WriteLocale(p.EnPath, kept); err != nil {
		t.Fatal(err)
	}
	s := mustLoad(t, p)
	if len(s.Orphans) != 1 || s.Orphans[0] != "only_in_english" {
		t.Fatalf("orphans = %v", s.Orphans)
	}
	for _, k := range s.Keys {
		if k.Key == "only_in_english" {
			t.Fatal("an orphan must not be listed as a key")
		}
	}
}

func TestLoadReadsDiskEveryTime(t *testing.T) {
	p := mustInit(t)
	_ = mustLoad(t, p)
	editUK(t, p, "plain_yes", []string{"так, змінено ззовні"})
	if got := keyOf(t, mustLoad(t, p), "plain_yes").UK[0]; got != "так, змінено ззовні" {
		t.Fatalf("Load served a stale copy: %q", got)
	}
}

func TestSaveRejectsBlockingIssuesAndWritesNothing(t *testing.T) {
	p := mustInit(t)
	ukBefore, progBefore := readFile(t, p.UkPath), readFile(t, p.ProgressPath)

	for name, c := range map[string]struct {
		key   string
		parts []string
		code  string
	}{
		"missing placeholder and tags": {"admin_ok", []string{"Готово"}, "placeholders"},
		"list with a different length": {"misc_runs", []string{"лише один"}, "list_length"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := p.Save(c.key, c.parts)
			var ve *ValidationError
			if !errors.As(err, &ve) || !hasCode(ve.Issues, c.code) {
				t.Fatalf("want a ValidationError with %q, got %v", c.code, err)
			}
			if _, err := p.Confirm(c.key, c.parts); !errors.As(err, &ve) {
				t.Fatalf("Confirm must refuse too, got %v", err)
			}
		})
	}
	if readFile(t, p.UkPath) != ukBefore || readFile(t, p.ProgressPath) != progBefore {
		t.Fatal("a refused save changed a file")
	}
	if _, err := p.Save("no_such_key", []string{"x"}); err == nil {
		t.Fatal("an unknown key must be an error")
	}
}

func TestSaveUnconfirmsChangedKeyOnly(t *testing.T) {
	p := mustInit(t)
	good := []string{"Готово <b>%s</b>"}
	if _, err := p.Confirm("admin_ok", good); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Confirm("plain_yes", []string{"так"}); err != nil {
		t.Fatal(err)
	}
	if k, err := p.Save("admin_ok", good); err != nil || !k.Confirmed {
		t.Fatalf("saving identical text must keep the confirmation: %+v %v", k, err)
	}
	if k, err := p.Save("admin_ok", []string{"Зроблено <b>%s</b>"}); err != nil || k.Confirmed {
		t.Fatalf("changed text must drop the confirmation: %+v %v", k, err)
	}
	s := mustLoad(t, p)
	if keyOf(t, s, "admin_ok").Confirmed || !keyOf(t, s, "plain_yes").Confirmed {
		t.Fatal("only the edited key may lose its confirmation")
	}
}

func TestConfirmRecordsHashAndEnglishEditReopens(t *testing.T) {
	p := mustInit(t)
	if k, err := p.Confirm("admin_ok", []string{"Готово <b>%s</b>"}); err != nil || !k.Confirmed {
		t.Fatalf("%+v %v", k, err)
	}
	en, _ := LoadLocale(p.EnPath)
	for i := range en {
		if en[i].Key == "admin_ok" {
			en[i].Parts = []string{"All done <b>%s</b>"}
		}
	}
	if err := WriteLocale(p.EnPath, en); err != nil {
		t.Fatal(err)
	}
	k := keyOf(t, mustLoad(t, p), "admin_ok")
	if k.Confirmed || !k.Reopened {
		t.Fatalf("an English edit must re-open the key: %+v", k)
	}
	if fresh := keyOf(t, mustLoad(t, p), "plain_yes"); fresh.Reopened {
		t.Fatal("a key that was never confirmed is not 'reopened'")
	}
}

func TestSaveKeepsOutsideEdits(t *testing.T) {
	p := mustInit(t)
	if _, err := p.Save("plain_yes", []string{"так"}); err != nil {
		t.Fatal(err)
	}
	editUK(t, p, "pseudo", []string{"змінено в іншому редакторі"})
	if _, err := p.Save("admin_ok", []string{"Готово <b>%s</b>"}); err != nil {
		t.Fatal(err)
	}
	if got := keyOf(t, mustLoad(t, p), "pseudo").UK[0]; got != "змінено в іншому редакторі" {
		t.Fatalf("a save overwrote an outside edit: %q", got)
	}
}

func TestSaveRewritesUkInEnglishOrderAndDropsOrphans(t *testing.T) {
	p := mustInit(t)
	appendTo(t, p.UkPath, "stale_key: Застарілий\n")
	if _, err := p.Save("plain_yes", []string{"так"}); err != nil {
		t.Fatal(err)
	}
	uk, _ := LoadLocale(p.UkPath)
	en, _ := LoadLocale(p.EnPath)
	if strings.Join(keysOf(uk), ",") != strings.Join(keysOf(en), ",") {
		t.Fatalf("uk keys %v, want exactly English's %v", keysOf(uk), keysOf(en))
	}
}

func TestConcurrentConfirmsAreNotLost(t *testing.T) {
	p := mustInit(t)
	s := mustLoad(t, p)
	var wg sync.WaitGroup
	for _, k := range s.Keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Confirm(k.Key, k.UK); err != nil {
				t.Errorf("%s: %v", k.Key, err)
			}
		}()
	}
	wg.Wait()
	for _, k := range mustLoad(t, p).Keys {
		if !k.Confirmed {
			t.Errorf("%s lost its confirmation", k.Key)
		}
	}
}

func TestModulesCountConfirmedKeys(t *testing.T) {
	p := mustInit(t)
	if _, err := p.Confirm("admin_ok", []string{"Готово <b>%s</b>"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range mustLoad(t, p).Modules {
		if m.Name == "admin" && (m.Confirmed != 1 || m.Total != 1) {
			t.Fatalf("admin module = %+v", m)
		}
		if m.Name == "language" && m.Confirmed != 0 {
			t.Fatalf("language module = %+v", m)
		}
	}
}

func confirmAll(t *testing.T, p *Project) {
	t.Helper()
	for _, k := range mustLoad(t, p).Keys {
		if _, err := p.Confirm(k.Key, k.UK); err != nil {
			t.Fatalf("%s: %v", k.Key, err)
		}
	}
}

func TestCheckFailsUntilEveryKeyIsConfirmed(t *testing.T) {
	p := mustInit(t)
	report, ok := p.Check()
	if ok || !strings.Contains(report, "plain_yes") {
		t.Fatalf("with nothing confirmed Check must fail and name unconfirmed keys:\n%s", report)
	}
	confirmAll(t, p)
	if report, ok := p.Check(); !ok {
		t.Fatalf("everything confirmed, Check should pass:\n%s", report)
	}
	appendTo(t, p.EnPath, "brand_new: Brand new\n")
	if report, ok := p.Check(); ok || !strings.Contains(report, "brand_new") {
		t.Fatalf("a new English key must fail Check:\n%s", report)
	}
}

func TestCheckFailsOnMixedApostrophes(t *testing.T) {
	p := mustInit(t)
	confirmAll(t, p)
	if _, err := p.Confirm("plain_yes", []string{"м'ят"}); err != nil {
		t.Fatal(err)
	}
	if report, ok := p.Check(); !ok {
		t.Fatalf("a single apostrophe style is fine:\n%s", report)
	}
	if _, err := p.Confirm("only_in_english", []string{"м'яч"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Confirm("pseudo", []string{"Використовуйте <code>/ban <userhandle></code>, 100% від {first} м’ят"}); err != nil {
		t.Fatal(err)
	}
	report, ok := p.Check()
	if ok || !strings.Contains(report, "apostrophe") {
		t.Fatalf("two apostrophe characters must fail Check:\n%s", report)
	}
	// At the end of 1,400 keys "’ x5" is useless: the owner must be told which keys.
	section := report[strings.Index(report, "apostrophe"):]
	if !strings.Contains(section, "pseudo") {
		t.Fatalf("the report must name the key that uses the rarer character:\n%s", report)
	}
	if strings.Contains(section, "plain_yes") || strings.Contains(section, "only_in_english") {
		t.Fatalf("keys that use the common character must not be listed as the ones to fix:\n%s", report)
	}
}

func TestCheckFailsOnBlockingIssueAndOrphan(t *testing.T) {
	p := mustInit(t)
	confirmAll(t, p)
	editUK(t, p, "admin_ok", []string{"Готово без розмітки"}) // edited outside, now invalid
	if report, ok := p.Check(); ok || !strings.Contains(report, "admin_ok") {
		t.Fatalf("an invalid text must fail Check:\n%s", report)
	}
	editUK(t, p, "admin_ok", []string{"Готово <b>%s</b>"})
	appendTo(t, p.UkPath, "stale_key: Застарілий\n")
	if report, ok := p.Check(); ok || !strings.Contains(report, "stale_key") {
		t.Fatalf("an orphan must fail Check:\n%s", report)
	}
}

func TestSyncAppendsMissingKeysOnly(t *testing.T) {
	p := mustInit(t)
	if _, err := p.Confirm("admin_ok", []string{"Готово <b>%s</b>"}); err != nil {
		t.Fatal(err)
	}
	editUK(t, p, "plain_yes", []string{"власний переклад"})
	appendTo(t, p.EnPath, "brand_new: Brand new\n")
	added, err := p.Sync()
	if err != nil || added != 1 {
		t.Fatalf("Sync = %d, %v; want 1", added, err)
	}
	s := mustLoad(t, p)
	if k := keyOf(t, s, "brand_new"); k.Confirmed || k.UK[0] != "Brand new" {
		t.Errorf("brand_new = %+v", k)
	}
	if !keyOf(t, s, "admin_ok").Confirmed || keyOf(t, s, "plain_yes").UK[0] != "власний переклад" {
		t.Error("Sync touched an existing translation or confirmation")
	}
	before := readFile(t, p.UkPath)
	if added, err := p.Sync(); err != nil || added != 0 {
		t.Fatalf("second Sync = %d, %v; want 0", added, err)
	}
	if readFile(t, p.UkPath) != before {
		t.Error("a Sync that adds nothing must not rewrite the file")
	}
}

func TestLoadWithoutDraftExplainsInit(t *testing.T) {
	p := newProject(t)
	_, err := p.Load()
	if err == nil || !strings.Contains(err.Error(), "-init") {
		t.Fatalf("want an error pointing at -init, got %v", err)
	}
}

func TestStateCarriesTokensHintsAndWarnings(t *testing.T) {
	p := mustInit(t)
	if err := os.WriteFile(p.GlossaryPath, []byte("terms:\n  - en: done\n    uk: готов\nrussian_words: [готово]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	k := keyOf(t, mustLoad(t, p), "admin_ok")
	if fmt.Sprint(k.Tokens) != "[%s </b> <b>]" {
		t.Errorf("tokens = %v", k.Tokens)
	}
	if len(k.Hints) != 1 || k.Hints[0].English != "done" {
		t.Errorf("hints = %+v", k.Hints)
	}
	if !hasCode(k.Warnings, "ru_word") {
		t.Errorf("warnings = %+v, want ru_word from the glossary list", k.Warnings)
	}
}

func TestPreviewJudgesTextWithoutWriting(t *testing.T) {
	p := mustInit(t)
	ukBefore, progBefore := readFile(t, p.UkPath), readFile(t, p.ProgressPath)

	k, err := p.Preview("admin_ok", []string{"Готово"})
	if err != nil || !hasCode(k.Blocking, "placeholders") || k.UK[0] != "Готово" {
		t.Fatalf("preview of broken text: %+v %v", k, err)
	}
	k, err = p.Preview("admin_ok", []string{"Всё готово <b>%s</b>"})
	if err != nil || len(k.Blocking) != 0 || !hasCode(k.Warnings, "cyrillic_ru") {
		t.Fatalf("preview of valid but Russian text: %+v %v", k, err)
	}
	if k.Confirmed {
		t.Fatal("a preview never confirms")
	}
	if _, err := p.Preview("no_such_key", []string{"x"}); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("want ErrUnknownKey, got %v", err)
	}
	if readFile(t, p.UkPath) != ukBefore || readFile(t, p.ProgressPath) != progBefore {
		t.Fatal("Preview wrote a file")
	}
}
