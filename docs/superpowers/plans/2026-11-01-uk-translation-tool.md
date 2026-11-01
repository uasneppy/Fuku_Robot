# Ukrainian Translation Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `scripts/translate_uk`, a local editor that lets the owner translate all of `locales/en.yml` into `locales/uk.yml` by hand, then register `uk` as the 8th locale once every key is confirmed.

**Architecture:** One Go program in the main module (`package main`, standard library plus `gopkg.in/yaml.v3`). Small files with one job each: locale file IO, validators, progress store, glossary, a `Project` that ties them together, a CLI (`-init`, `-sync`, `-check`) and a loopback-only HTTP server serving one embedded HTML page. Everything is re-read from disk on every request, so git pulls and outside edits never leave the tool stale.

**Tech Stack:** Go 1.26.0, `gopkg.in/yaml.v3` v3.0.1 (already in `go.mod`), `net/http`, `go:embed`, vanilla JS. Module path `github.com/divkix/Alita_Robot`.

**Spec:** `docs/superpowers/specs/2026-11-01-uk-translation-tool-design.md` (including its addendum). Read it first.

## Global Constraints

- The tool imports the standard library and `gopkg.in/yaml.v3` only. It must not import `alita/config` or `alita/db` (they connect to PostgreSQL before `main`).
- No CDN, no Node, no network: the page works offline.
- The server listens on a loopback address only (default `127.0.0.1:8080`).
- Progress file: `scripts/translate_uk/confirmed.txt`, one `key<TAB>8 hex chars of sha256(English)` per line. Never put it in `locales/` (that folder is embedded and scanned by `scripts/check_translations`).
- `uk` is registered nowhere (`language.go`, `PageLanguages`, `captcha.html`, `ENABLED_LOCALES`) until Task 10, which runs only after `go run ./scripts/translate_uk -check` exits 0.
- `lang_sample` = `Українська`, `language_flag` = `🇺🇦`, `language_name` = `Ukrainian` in the draft.
- Warnings never block; blocking checks are placeholders, tags, angle-bracket counts and list length.
- Tests: `go test -tags testtools -race -count=1 ./scripts/translate_uk/` (CGO on). Real fixtures, no mock libraries, assert behaviour not source text. After writing each test, break the behaviour once and see it fail.
- Commits: Conventional Commits. Tool tasks use `chore(i18n):`; Task 10 is user-visible, `feat(i18n):`.
- The repo's `CLAUDE.md` requires file changes to start through a GSD command (`/gsd-quick` or `/gsd-execute-phase`). Run the tasks under one.
- Follow `AGENTS.md`: comments say why, not what; `gofmt`; `make lint` clean.

## Review Focus

The spec implies these inputs but no happy-path task would exercise them. Most likely to bite first:

1. `misc_runs`, the only list-valued key (4 items): a Ukrainian list with another item count must be refused, and a list must stay a YAML sequence after a write. Tested in Tasks 1, 2 and 5.
2. English text that looks like markup but is not: `100% of` (`%` then space), `{first}` in help prose, `&lt;`, and pseudo-tags `<userhandle>` / `<yes>`. None may produce a false "missing placeholder", and a stray `<` typed by the translator must be refused. Task 2.
3. Russian-leftover detection on Cyrillic text: Go's `\b` is ASCII-only, so matching words by regex boundary silently never fires. Task 3 tokenises by `unicode.IsLetter`, and its test uses a Cyrillic word.
4. A web page open in the same browser posting to the local server (DNS rebinding / CSRF): wrong `Host`, wrong `Origin` or non-JSON content type must be refused with nothing written. Task 7.
5. `en.yml` or `uk.yml` changed while the tool runs (`git pull`, a merge, an edit in another editor): new English keys appear unconfirmed, an edited English string re-opens its key, a removed key is an orphan, and the next save keeps outside changes. Tasks 4 and 5.

## File Structure

| Path | Responsibility |
| --- | --- |
| `scripts/translate_uk/locale.go` | `Entry`, `LoadLocale`, `WriteLocale` (ordered, list-aware, atomic) |
| `scripts/translate_uk/validate.go` | `Issue`, `BlockingIssues`, `Warnings` |
| `scripts/translate_uk/glossary.go` | `Glossary`, `LoadGlossary`, term and Russian-word checks |
| `scripts/translate_uk/progress.go` | `Progress`, `SourceHash`, load/save `confirmed.txt` |
| `scripts/translate_uk/project.go` | `Project`: `Init`, `Sync`, `Load`, `Save`, `Confirm`, `Check`, `ModuleOf` |
| `scripts/translate_uk/main.go` | `run(args, stdout, stderr) int`, flags |
| `scripts/translate_uk/server.go` | `newServer(*Project) http.Handler`, `serve`, guards |
| `scripts/translate_uk/web/index.html` | the editor page (embedded) |
| `scripts/translate_uk/README.md` | how the owner runs it |
| `scripts/translate_uk/testdata/` | small `en.yml`, `ru.yml`, `glossary.yml` fixtures |
| `scripts/translate_uk/*_test.go` | one test file per source file |

---

### Task 1: Locale file IO

**Files:**
- Create: `scripts/translate_uk/locale.go`, `scripts/translate_uk/locale_test.go`, `scripts/translate_uk/testdata/en.yml`, `scripts/translate_uk/testdata/ru.yml`

**Interfaces:**
- Produces:
  - `type Entry struct { Key string; Parts []string; List bool }`. A scalar has one part and `List` false; a sequence has its items as parts.
  - `func LoadLocale(path string) ([]Entry, error)`: entries in file order.
  - `func WriteLocale(path string, entries []Entry) error`: atomic (temp file in the same directory, then rename).

- [ ] **Step 1: Fixtures.** `testdata/en.yml` holds: `admin_ok` (`Done <b>%s</b>`), `misc_runs` (list of 2), `help_configuration_step-1`, a multi-line value with a trailing newline, `plain_yes` with the string value `"yes"`, and `pseudo` (`Use <code>/ban <userhandle></code>, 100% of {first}`). `ru.yml` has the same keys with Cyrillic text, except it lacks one key present in `en.yml`.
- [ ] **Step 2: Write failing tests** in `locale_test.go`:
  - `TestLoadLocaleKeepsFileOrderAndShapes`: key order equals the file's; `misc_runs` has `List` true and 2 parts; the hyphen key loads.
  - `TestWriteLocaleRoundTripsAwkwardScalars`: values `yes`, `null`, `123`, `~`, `a: b`, `#x`, leading and trailing spaces, an empty string, `🇺🇦`, and multi-line text with and without a trailing newline are all equal after write then load; the list stays a list.
  - `TestWriteLocaleLeavesNoTempFiles`: the directory holds only the target file afterwards.
  - `TestLoadLocaleRejectsNonStringValues`: a mapping or number value returns an error that names the key.
  - `TestRealLocalesHaveTheSameKeys`: `../../locales/en.yml` and `ru.yml` load with equal key sets, and exactly one list key.
- [ ] **Step 3: Run, expect FAIL** (`undefined: LoadLocale`).
- [ ] **Step 4: Implement** `LoadLocale` / `WriteLocale` with the `yaml.Node` API so order is kept. Read scalar text from `Node.Value` whatever its tag. Write every string as a `!!str` node so special scalars are quoted; let yaml.v3 choose block or quoted style (only exact round trip is required). Put a comment on why nodes are used instead of `map[string]any`.
- [ ] **Step 5: Run, expect PASS.** Then break `WriteLocale` (drop the `!!str` tag) and confirm the awkward-scalar test fails.
- [ ] **Step 6: Commit** `chore(i18n): add locale file IO for the Ukrainian translation tool`.

---

### Task 2: Validators

**Files:**
- Create: `scripts/translate_uk/validate.go`, `scripts/translate_uk/validate_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type Issue struct { Code, Message string }` with stable codes: `placeholders`, `tags`, `angle_brackets`, `entities`, `list_length`, `nesting`, `cyrillic_ru` (letters), `length`.
  - `func BlockingIssues(en, uk []string) []Issue`
  - `func Warnings(en, uk []string) []Issue`
  - `func PlaceholdersOf(s string) []string`, `func TagsOf(s string) []string` (sorted multisets; also used by the UI payload in Task 7).

- [ ] **Step 1: Write failing tests**, table-driven, in `validate_test.go`:
  - `TestPlaceholdersMustMatch`: missing `%s`, extra `%d`, renamed `{name}`, `%[1]s` kept, `%%` kept, reordered placeholders accepted, and `100% of` / `% o` ignored (they are not verbs).
  - `TestTagsMustMatch`: missing `</b>`, extra `<i>`, altered `<a href="...">` target, reordered tags accepted, `<userhandle>` and `<yes>` compared as opaque tokens, tag-name case is not normalised for pseudo-tags.
  - `TestNestingChecked`: `<b><i>x</b></i>` is refused; when the English itself is not well nested the nesting check is skipped (existing quirks must not block).
  - `TestAngleBracketsAndEntitiesMustMatch`: a stray `<` in Ukrainian is refused; `&lt;` replaced by a literal `<` is refused.
  - `TestListLengthMustMatch`: 4-item English list against a 3-item Ukrainian list is refused; parts are compared index by index.
  - `TestWarnings`: `ы э ъ ё` (either case) warn; Ukrainian over 1.4x the English rune count warns only when it is at least 40 runes; a clean Ukrainian string returns no warnings.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Verb pattern `%%|%(?:\[\d+\])?[sdvfbtoxX]` plus `\{[^{}]+\}` (same shapes `alita/i18n/translator.go` substitutes). Tags: every `<...>` token as written, compared as a multiset; nesting only for `b i u s code pre a em strong blockquote tg-spoiler span`. Say in a comment why pseudo-tags are compared as opaque tokens.
- [ ] **Step 4: Run, expect PASS; mutate** (remove the `100% of` exclusion) and see the test fail.
- [ ] **Step 5: Run the validators over the real locales once, with a throwaway `go run` or test you do not commit,** comparing `en.yml` against `ru.yml` key by key. Expect no blocking issues; if some appear they are `ru.yml` defects, so list them in the commit message body, because the owner will meet them in the editor.
- [ ] **Step 6: Commit** `chore(i18n): add placeholder and tag validation for the translation tool`.

---

### Task 3: Glossary and Russian-word check

**Files:**
- Create: `scripts/translate_uk/glossary.go`, `scripts/translate_uk/glossary_test.go`, `scripts/translate_uk/testdata/glossary.yml`

**Interfaces:**
- Produces:
  - `type Term struct { English string \`yaml:"en"\`; Ukrainian string \`yaml:"uk"\`; Forbidden []string \`yaml:"forbidden"\` }`
  - `type Glossary struct { Terms []Term \`yaml:"terms"\`; RussianWords []string \`yaml:"russian_words"\` }`
  - `func LoadGlossary(path string) (*Glossary, error)` (a missing file is an empty glossary, not an error)
  - `func (g *Glossary) Check(en, uk []string) []Issue` (code `glossary`)
  - `func (g *Glossary) RussianWords(uk []string) []Issue` (code `ru_word`)
  - `func (g *Glossary) Hints(en []string) []Term` (terms present in the English text, for the UI)

- [ ] **Step 1: Write failing tests:**
  - `TestGlossaryWarnsWhenChosenRenderingMissing`: English contains a whole word `ban` (case-insensitive), `uk` is a stem such as `бан`; Ukrainian containing `бану` passes, Ukrainian without the stem warns.
  - `TestGlossaryWarnsOnForbiddenVariant` (substring, case-insensitive).
  - `TestGlossarySkipsTermsWithoutUkrainian`: a seeded term with empty `uk` never warns.
  - `TestGlossaryMatchesEnglishOnWholeWordsOnly`: `banner` does not trigger `ban`.
  - `TestRussianWordsAreMatchedAsWholeCyrillicWords`: with `что` in the list, `что-то` warns and `що` and `якщо` do not; a Cyrillic fixture proves the tokeniser is not an ASCII `\b`.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Tokenise with `unicode.IsLetter` / `unicode.IsDigit`; compare lower-cased tokens. Comment why: RE2's `\b` is ASCII-only.
- [ ] **Step 4: Run, expect PASS; mutate** (swap the tokeniser for `\b`) and see the Cyrillic test fail.
- [ ] **Step 5: Commit** `chore(i18n): add glossary and Russian-word checks to the translation tool`.

---

### Task 4: Progress store

**Files:**
- Create: `scripts/translate_uk/progress.go`, `scripts/translate_uk/progress_test.go`

**Interfaces:**
- Produces:
  - `func SourceHash(en []string) string`: first 8 hex characters of sha256 of the parts joined with `"\x00"`.
  - `type Progress struct{ /* unexported map key -> hash */ }`
  - `func LoadProgress(path string) (*Progress, error)` (missing file is empty; `#` lines and blank lines ignored; a malformed line returns an error naming the line number)
  - `func (p *Progress) Confirmed(key string, en []string) bool` (true only if the stored hash equals `SourceHash(en)`)
  - `func (p *Progress) Confirm(key string, en []string)`, `func (p *Progress) Unconfirm(key string)`, `func (p *Progress) Keys() []string`
  - `func (p *Progress) Save(path string) error` (sorted by key, atomic, keeps a header comment)

- [ ] **Step 1: Write failing tests:** `TestConfirmedRequiresMatchingHash` (changing the English re-opens the key); `TestSaveIsSortedAndStable` (save, load, save gives identical bytes); `TestLoadRejectsMalformedLine` (error names the line); `TestListKeyHashCoversEveryItem` (changing one item of `misc_runs` changes the hash).
- [ ] **Step 2: Run, expect FAIL. Step 3: Implement. Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** `chore(i18n): add progress tracking to the translation tool`.

---

### Task 5: Project core

**Files:**
- Create: `scripts/translate_uk/project.go`, `scripts/translate_uk/project_test.go`

**Interfaces:**
- Consumes: `LoadLocale`, `WriteLocale` (Task 1); `BlockingIssues`, `Warnings`, `Issue`, `PlaceholdersOf`, `TagsOf` (Task 2); `Glossary` (Task 3); `Progress`, `SourceHash` (Task 4).
- Produces:
  - `type Project struct { EnPath, RuPath, UkPath, ProgressPath, GlossaryPath string }` (a `sync.Mutex` inside guards writes)
  - `type KeyState struct { Key, Module string; EN, UK []string; List, Confirmed, Reopened bool; Blocking, Warnings []Issue; Hints []Term }`. `Reopened` means it was confirmed before the English changed.
  - `type State struct { Keys []KeyState; Modules []ModuleSummary; Orphans []string }`, `type ModuleSummary struct { Name string; Confirmed, Total int }`
  - `type ValidationError struct { Issues []Issue }` (implements `error`)
  - `func (p *Project) Init(force bool) error`
  - `func (p *Project) Sync() (added int, err error)`
  - `func (p *Project) Load() (*State, error)` (read-only; reads every file from disk each call)
  - `func (p *Project) Save(key string, uk []string) (KeyState, error)`
  - `func (p *Project) Confirm(key string, uk []string) (KeyState, error)`
  - `func (p *Project) Check() (report string, ok bool)`
  - `func ModuleOf(key string) string` (text before the first `_`; a key with no `_` is module `other`)

- [ ] **Step 1: Write failing tests** against the Task 1 fixtures copied into `t.TempDir()`:
  - `TestInitBuildsDraftFromRussianInEnglishOrder`: key order equals `en.yml`; values equal `ru.yml`; a key absent from `ru.yml` falls back to English; the three `lang_*` keys are the Ukrainian values; `confirmed.txt` and `glossary.yml` are created; a second `Init(false)` returns an error and changes nothing; `Init(true)` rewrites `uk.yml` and empties the progress file but leaves `glossary.yml` alone.
  - `TestLoadSyncsNewEnglishKeysAsUnconfirmed`: a key added to `en.yml` appears with the Russian or English text and `Confirmed` false; a key removed from `en.yml` appears in `State.Orphans`.
  - `TestLoadReadsDiskEveryTime`: edit `uk.yml` outside the tool, `Load` shows it.
  - `TestSaveRejectsBlockingIssuesAndWritesNothing`: returns `*ValidationError`; the file bytes are unchanged; list-length mismatch on `misc_runs` is refused.
  - `TestSaveUnconfirmsChangedKeyOnly`: saving identical text keeps the confirmation; changed text removes it; other keys are untouched.
  - `TestConfirmRecordsHashAndEnglishEditReopens`: after `Confirm`, editing the English text makes `Confirmed` false and `Reopened` true.
  - `TestSaveKeepsOutsideEdits`: a key changed in `uk.yml` by another process between two saves survives the second save.
  - `TestCheckFailsUntilEveryKeyIsConfirmed`: `ok` is false with one unconfirmed key, true when all are confirmed and valid; the report lists per-module counts; mixed apostrophe characters between Ukrainian letters (`'`, `’`, `ʼ`) make `Check` fail with a clear line.
  - `TestSyncAppendsMissingKeysOnly`: existing translations and confirmations are untouched.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** `Save` and `Confirm` re-read `uk.yml`, replace one entry, run `Sync`'s merge, then `WriteLocale`. Seed `glossary.yml` on `Init` with the term list (ban, unban, mute, unmute, kick, warn, captcha, lockdown, staff group, approved user, admin, federation, blacklist, antiflood, filter, note, rules, report, log channel, raid) with empty `uk`, and `russian_words` = `что, это, или, как, только, если, чтобы, когда`. Terms are for the owner to fill in; do not pick translations.
- [ ] **Step 4: Run, expect PASS; mutate** (skip the re-read in `Save`) and see `TestSaveKeepsOutsideEdits` fail.
- [ ] **Step 5: Commit** `chore(i18n): add the translation project core`.

---

### Task 6: Command line

**Files:**
- Create: `scripts/translate_uk/main.go`, `scripts/translate_uk/main_test.go`

**Interfaces:**
- Consumes: `Project` (Task 5).
- Produces: `func run(args []string, stdout, stderr io.Writer) int`; flags `-init`, `-force`, `-sync`, `-check`, `-addr` (default `127.0.0.1:8080`), `-locales` (default `locales`), `-data` (default `scripts/translate_uk`). With no mode flag it serves (Task 7's `serve`). Exit 0 ok, 1 check failed, 2 usage or environment error.

- [ ] **Step 1: Write failing tests:**
  - `TestCheckExitCodes`: exit 1 with an unconfirmed key, 0 when all are confirmed.
  - `TestInitRefusesToOverwrite`: second `-init` exits 2 and names `-force`.
  - `TestSyncPrintsHowManyKeysWereAdded`.
  - `TestRunFromWrongDirectoryExplainsItself`: no `locales/en.yml` gives exit 2 and a message that says to run from the repo root.
  - `TestToolDoesNotImportDatabasePackages`: `go list -deps` of the package contains neither `alita/config` nor `alita/db`.
- [ ] **Step 2: Run, expect FAIL. Step 3: Implement. Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** `chore(i18n): add -init, -sync and -check to the translation tool`.

---

### Task 7: HTTP server

**Files:**
- Create: `scripts/translate_uk/server.go`, `scripts/translate_uk/server_test.go`, a placeholder `scripts/translate_uk/web/index.html` (replaced in Task 8)

**Interfaces:**
- Consumes: `Project`, `State`, `KeyState`, `ValidationError` (Task 5).
- Produces:
  - `func newServer(p *Project) http.Handler`
  - `func serve(addr string, p *Project, stdout io.Writer) error` (refuses any non-loopback address)
  - Routes: `GET /` (embedded page), `GET /api/state` (the `State` as JSON), `POST /api/save` and `POST /api/confirm` with body `{"key": string, "uk": []string}`; success returns the `KeyState`, a refusal returns `422` with `{"issues": [...]}`, an unknown key `404`.

- [ ] **Step 1: Write failing tests** with `httptest`:
  - `TestStateReflectsDiskOnEveryRequest`.
  - `TestSaveAndConfirmRoundTrip`, including a `422` body that lists the issue codes and leaves the file unchanged.
  - `TestForeignHostIsRefused`: `Host: evil.example` gets `403` and nothing is written.
  - `TestForeignOriginIsRefused` and `TestNonJSONContentTypeIsRefused` (`403` and `415`).
  - `TestRequestBodyIsCapped`: a body over 1 MiB gets `413`.
  - `TestServeRefusesNonLoopbackAddress`: `0.0.0.0:8080` returns an error.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Allowed hostnames: `127.0.0.1`, `localhost`, `::1`, any port. Writes need `Content-Type: application/json` and, when `Origin` is present, an origin whose host is allowed. Embed with `//go:embed web/index.html`. Say in a comment that the guards exist because this server writes files and a page in the same browser could otherwise reach it.
- [ ] **Step 4: Run, expect PASS; mutate** (drop the Host check) and see the test fail.
- [ ] **Step 5: Commit** `chore(i18n): add the local editor server`.

---

### Task 8: Editor page

**Files:**
- Modify: `scripts/translate_uk/web/index.html`
- Create: `scripts/translate_uk/README.md`

**Interfaces:**
- Consumes: the routes of Task 7. No new Go interface.

Acceptance, checked by hand and by a headless-browser run (Chromium and Playwright are preinstalled in the cloud session; keep the script out of the repo):

- [ ] **Step 1: Build the page**, one self-contained HTML file with no external requests:
  - Left: modules with `confirmed / total`, a text filter, an "unconfirmed only" toggle, and a total counter.
  - Main: for the selected key, English read-only on the left, editable Ukrainian on the right (one box per item for a list key). English placeholders and tags are shown as chips; chips missing from the Ukrainian box are highlighted.
  - Blocking issues and warnings are shown under the box; Save and Confirm are disabled while a blocking issue exists (the server still enforces it).
  - A "English changed, re-check" badge for `Reopened` keys; glossary hints for the key (`Hints`).
  - `Ctrl+Enter` confirms and jumps to the next unconfirmed key in the current module list; unsaved text survives switching keys and warns before the page is closed.
  - Works at laptop width and shows long help texts without clipping.
- [ ] **Step 2: Write `README.md`**: run from the repo root with `go run ./scripts/translate_uk`, open the printed address; the daily flow (translate, `Ctrl+Enter`, commit `locales/uk.yml` and `scripts/translate_uk/confirmed.txt` often); `-check`, `-sync`; fill in `glossary.yml` first; why Russian leftovers are flagged.
- [ ] **Step 3: Verify in a browser.** Start the server on the Task 1 fixtures in a temp copy, drive it with Playwright: pick a key, break a `%s` and see Confirm disabled, fix it, press `Ctrl+Enter`, see the counter go up and the next key open; reload and see the state persisted. Take a screenshot of the main view and look at it.
- [ ] **Step 4: Commit** `chore(i18n): add the translation editor page`.

---

### Task 9: Generate the draft and prove the existing suite accepts it

**Files:**
- Create (generated): `locales/uk.yml`, `scripts/translate_uk/confirmed.txt`, `scripts/translate_uk/glossary.yml`

This is the check the spec marked "not verified yet".

- [ ] **Step 1:** `go run ./scripts/translate_uk -init`. Expect a draft with 1,418 keys and no `confirmed.txt` entries.
- [ ] **Step 2:** `go run ./scripts/translate_uk -check`. Expect exit 1, `0 / 1418` confirmed and no blocking issues; any blocking issue is a `ru.yml` defect, so list it in the PR.
- [ ] **Step 3:** `make check-translations`. Expect `uk` listed with "All translations present".
- [ ] **Step 4:** `go test -tags testtools -race -count=1 ./alita/i18n/... ./alita/modules/... ./alita/utils/captchaweb/...`, then `make test`. Expect green. If a test fails because `uk.yml` exists, fix the cause (not the test) and say what it was.
- [ ] **Step 5:** Confirm `uk` is still unreachable: `grep -rn '"uk"' alita main.go` finds nothing in non-test code, and the `ENABLED_LOCALES` default in `alita/config/config.go` is still `en`.
- [ ] **Step 6: Commit and push** `chore(i18n): add the draft Ukrainian locale (unreachable until registered)`. Tell the owner how to start (README).

---

### Task 10: Register `uk` (gated)

**Gate:** start only when the owner reports the translation done and `go run ./scripts/translate_uk -check` exits 0. Do not start earlier, whatever the draft looks like.

**Files:**
- Modify: `alita/modules/language.go:107`, `alita/utils/captchaweb/page.go:52`, `alita/utils/captchaweb/page/captcha.html:236`, `alita/i18n/staff_locale_test.go:18`, `alita/modules/settings_env_test.go:73`, `alita/modules/staff_panel_render_test.go:306`, `alita/modules/captcha_mode_turnstile_test.go:232`, `README.md:772`, `docs/src/content/docs/api-reference/environment.md` (the `ENABLED_LOCALES` section), `sample.env:58`, `AGENTS.md`, `.claude/CLAUDE.md`

- [ ] **Step 1: Red first.** Add `uk` to the four test lists and run `go test -tags testtools -race -count=1 ./alita/i18n/... ./alita/modules/... ./alita/utils/captchaweb/...`. Expect failures that name `uk` (language not selectable, `PageLanguages` mismatch).
- [ ] **Step 2:** Add `uk` to the `language.go` allow-list, `PageLanguages` (after `ru`) and the `LANGS` array in `captcha.html`. Then `grep -rn '"hi", "id", "pt", "ru"\|"hi", "ru", "pt", "id"' --include=*_test.go .` and add `uk` wherever a list means "every supported language" (leave single-language uses like `initdata_test.go`).
- [ ] **Step 3:** Update the docs: the locale list in `README.md:772`, `environment.md`, and a comment in `sample.env`; run `make generate-docs` and `make check-docs`.
- [ ] **Step 4:** Update the rules in the same commit (`AGENTS.md` requires it): every "7 locale files", "all 7 locales" and "7 languages" becomes 8 in `AGENTS.md` and `.claude/CLAUDE.md`, the language list gains `uk`, and `AGENTS.md` "Adding a module" step 7 says to run `go run ./scripts/translate_uk -sync` after adding keys.
- [ ] **Step 5:** `node --test scripts/captcha-page/*.test.mjs`, `make check-translations`, `make lint`, `make test`. Expect all green.
- [ ] **Step 6:** Commit `feat(i18n): add the Ukrainian locale`. Tell the owner to add `uk` to `ENABLED_LOCALES` in the deployment to show it in `/lang`.

---

## Not in this plan (needs its own spec first)

The owner wants everything Ukrainian, including admin commands and notifications. The locale files already contain every command help text, command description and notification text, so Tasks 1-10 cover them. These are still English in Go code:

- **Telegram command menu** (`main.go:492`): `SetMyCommands` registers `start` and `help` for `en` only. Needs a `uk` call using `main_bot_command_start` / `main_bot_command_help`. Command names themselves must stay Latin (a Telegram limit).
- **Hard-coded texts:** `alita/modules/reactions.go:54,58` ("Add Reaction", "Remove Reaction"), `alita/modules/filters.go:417`, `alita/modules/disabling.go:230,237`, `alita/utils/content/content.go:57,65,70,148`, and the fallbacks in `alita/utils/keyboard/keyboard.go:62,66`. These move into locale keys in all 8 locales (the tool's `-sync` then offers them for translation).
- **English stored in the database:** `db.DefaultWelcome` / `db.DefaultGoodbye` (`alita/db/db.go:45`) written at chat creation, the blacklist default reason (`alita/db/blacklists/repository.go:19`, `alita/db/models/blacklists.go:48`, `alita/modules/blacklists.go:499`), and "No Reason" for warns (`alita/db/warns/repository.go:151`, hard-wired to the `en` translator). Making these follow the chat's language changes what is stored, so it needs a decision on existing rows and on `backup` / `import`.

## Self-review

- **Spec coverage:** tool and files (Tasks 1-7), progress file with hash (4), editor behaviour, blocking and warning checks, glossary (2, 3, 8), `-init`/`-check` and draft contents (5, 6), draft safe in `locales/` and unverified-tests check (9), registration list (10), testing section (each task), rollout order (9 then the owner's work then 10). Addendum items: list key (1, 2, 5), misread-prone text (2), `-sync` and orphans (5, 6), server guards (7), wider scope (last section).
- **Type consistency:** `Entry`, `Issue`, `Term`, `Glossary`, `Progress`, `KeyState`, `State`, `Project` and the function names are used identically in every task that consumes them.
- **Proportion:** the plan states interfaces, test names and the values the spec fixes; function bodies are left to the implementer.
- **Open risk:** Task 9 is where the "existing tests accept a draft `uk.yml`" assumption is finally tested; the plan does not assume the result.
