---
phase: 11-message-editor
plan: 02
subsystem: utils
tags: [content, formatting, placeholders, message-editor, tdd, parity]
requires:
  - phase: 11-message-editor
    provides: plan 11-01 editor strings (not used by this plan's code)
provides:
  - content.TextLengthRefusal, content.ValidButtonURL and content.ExtractMedia, called by the commands today and reusable by the editor
  - formatting.FillPlaceholders (no Telegram call, no database read) and formatting.VisibleLength (UTF-16 units of the visible text)
affects: [11-10, 11-11, 11-13, settings-message-editor]

actuals:
  tokens: 21000
  tasks: 2
  commits: 3

commits: 3
plan_head_before: 8f3a1a02930fcfb4c0043757a55431e48d5ef6be
plan_head_after: a37a5a5083c4dfc32b6d6e12cf4f237404cc5c73

tech-stack:
  added: []
  patterns:
    - "One exported rule function that the command path already calls, so an editor dry run and the commands cannot drift (parity by construction)"
    - "FormattingReplacer keeps its rules-button logic and builds its text through the pure FillPlaceholders"

key-files:
  created:
    - fuku/utils/content/editor_helpers_test.go
    - fuku/utils/formatting/fill_placeholders_test.go
  modified:
    - fuku/utils/content/content.go
    - fuku/utils/formatting/formatting.go

key-decisions:
  - "The content package test binary installs the real uk and en locale files once in a TestMain, because its tests had no locale manager and a refusal text could not be compared; installing it per test would race with the package's parallel tests."
  - "The RED commit carries two stub functions in formatting.go so the tests compile and fail on their assertions (a build failure is not a valid RED); the GREEN commit replaces them."

requirements-completed: []

duration: 8min
completed: 2026-11-04
status: complete

coverage:
  - deliverable: "TextLengthRefusal returns the commands' own over-long text/caption reply and preFixes calls it"
    verification:
      - kind: test
        ref: "fuku/utils/content/editor_helpers_test.go#TestTextLengthRefusal"
        status: pass
      - kind: test
        ref: "fuku/utils/content/editor_helpers_test.go#TestTextLengthRefusalMatchesExtractWelcome"
        status: pass
    human_judgment: false
  - deliverable: "ValidButtonURL accepts only http or https with a host and the commands' button filter calls it"
    verification:
      - kind: test
        ref: "fuku/utils/content/editor_helpers_test.go#TestValidButtonURL"
        status: pass
    human_judgment: false
  - deliverable: "ExtractMedia gives the same file ID and stored type the commands store"
    verification:
      - kind: test
        ref: "fuku/utils/content/editor_helpers_test.go#TestExtractMedia"
        status: pass
    human_judgment: false
  - deliverable: "FillPlaceholders fills like FormattingReplacer with escaped values, one pass, rules tokens stripped, count as an argument"
    verification:
      - kind: test
        ref: "fuku/utils/formatting/fill_placeholders_test.go#TestFillPlaceholders"
        status: pass
      - kind: test
        ref: "fuku/utils/formatting/fill_placeholders_test.go#TestFillPlaceholdersMatchesFormattingReplacer"
        status: pass
    human_judgment: false
  - deliverable: "VisibleLength counts UTF-16 units of the decoded visible text"
    verification:
      - kind: test
        ref: "fuku/utils/formatting/fill_placeholders_test.go#TestVisibleLength"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 02: Shared length, link, media and placeholder rules Summary

**Four behaviour-preserving helpers the commands already call and the message editor will reuse: `content.TextLengthRefusal`, `ValidButtonURL`, `ExtractMedia`, and `formatting.FillPlaceholders` (pure, escaped, single pass) plus `VisibleLength` (UTF-16 units of the visible text).**

## Accomplishments

- Task 1 (tracer): `TextLengthRefusal` holds the two length checks that `preFixes` had inline (code points of the stored HTML, 4096 for text, 1024 for a caption, the same locale keys and counts); `ValidButtonURL` holds the button-link predicate (http or https with a host); `ExtractMedia` exposes `extractMediaFromReply`. `preFixes` now calls the first two, so `/setwelcome`, `/setgoodbye`, `/save` and `/filter` answer exactly as before.
- Task 2 (TDD): `FillPlaceholders` is the placeholder fill of `FormattingReplacer` without the Telegram member-count call and without the rules database read: every value `html.EscapeString`-ed, one left-to-right pass (a name holding `{last}` is not expanded again), `{rules}`, `{rules:same}` and `{rules:up}` stripped, the member count passed in. `FormattingReplacer` keeps its signature, its `{count}` lookup and its rules-button logic and calls it. `VisibleLength` is the UTF-16 length of `tgmd2html.StripHTMLV2(html)`.

## Task Commits

1. Task 1 (tracer): `b17c792` feat(11-02): share the length, link and media rules between the commands and the editor
2. Task 2 RED: `cd6ee86` test(11-02): add failing tests for FillPlaceholders and VisibleLength
3. Task 2 GREEN: `a37a5a5` feat(11-02): fill placeholders and measure visible length without Telegram or the database

## TDD Gate Compliance

Task 2 is the plan's `tdd="true"` task (the plan itself is `type: execute`, so no plan-level gate check applies).

- RED: `TestFillPlaceholders`, `TestFillPlaceholdersMatchesFormattingReplacer` and `TestVisibleLength` were written first and committed with two stub functions so the package compiled. The real `go test` run showed all three `--- FAIL` on their assertions (for example `FillPlaceholders = "", FormattingReplacer = "hello"` and `VisibleLength` giving -1), not on a build, load or fixture fault. Semantic assessment: the target tests executed and failed on the planned assertions for the intended reason. The `gsd_run check tdd-red-evidence` classifier was not run (no persisted evidence record; the plan is not a `type: tdd` plan).
- GREEN: the real implementation made all three pass; the whole `fuku/utils/formatting` package, including the pre-existing `TestFormattingReplacer*` tests, passes unchanged.
- REFACTOR: none needed (a parameter of `VisibleLength` was renamed in the GREEN commit so it does not shadow the `html` import).

Task 1 (a tracer) was written test first but committed as one `feat` commit with its tests, as the plan states one commit per task.

## Verification (real results)

- `go test -tags testtools -race -count=1 ./fuku/utils/content ./fuku/utils/formatting`: both `ok` (every pre-existing `TestPreFixes*`, `TestExtractWelcome*`, `TestExtractMediaFromReply` and `TestFormattingReplacer*` test passes).
- `go test -tags testtools -race -count=1 -run '^(TestTextLengthRefusal|TestValidButtonURL|TestExtractMedia|TestTextLengthRefusalMatchesExtractWelcome)$' -v ./fuku/utils/content`: `--- PASS` for all four.
- `go test -tags testtools -race -count=1 -run '^(TestFillPlaceholders|TestFillPlaceholdersMatchesFormattingReplacer|TestVisibleLength)$' -v ./fuku/utils/formatting`: `--- PASS` for all three.
- `go test -tags testtools -race -count=1 -run '^(TestSetAndResetGreetingTextCommands|TestSetRulesStoresCommandTextAndReplies)' -v ./fuku/modules`: both PASS. A wider `-run 'Greeting|Welcome|Goodbye|Filter|Note' ./fuku/modules`: ok.
- `grep -c 'TextLengthRefusal(\|ValidButtonURL(' fuku/utils/content/content.go` prints 4 (baseline 0); `grep -c 'func ExtractMedia(' ...` prints 1; `grep -c 'FillPlaceholders(' fuku/utils/formatting/formatting.go` prints 2; `grep -c 'func VisibleLength(' ...` prints 1.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/utils/content ./fuku/utils/formatting` and `gofmt -l` on both packages: clean.
- `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0, a sandbox fact); `make test` (not asked by this plan); PostgreSQL (nothing here needs it).

## Requirements

Contributes to EDIT-03 (the length rules) and EDIT-05 (the link rule). It does not complete either: EDIT-03 is completed by plan 11-11 and EDIT-05 by plan 11-13, so `requirements-completed` is empty, `requirements.mark-complete` was not run, and both stay Pending in REQUIREMENTS.md.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] The content package's tests had no locale manager**
- **Found during:** Task 1 (first run of `TestTextLengthRefusal`)
- **Issue:** the plan asks for a real translator, but in this package's test binary no locale manager exists, so `MustNewTranslator("en")` returned empty strings and the refusal texts could not be compared with the locale.
- **Fix:** a `TestMain` in `fuku/utils/content/editor_helpers_test.go` installs the real `uk` and `en` locale files once through `i18n.OverrideManagerLanguagesForTest` before any test runs (not per test, so the package's parallel tests do not race). The existing tests still pass with it.
- **Files modified:** `fuku/utils/content/editor_helpers_test.go`
- **Commit:** `b17c792`

**2. [Process] Two stub functions in the RED commit**
- A build failure is not a valid RED, so `cd6ee86` adds `FillPlaceholders` and `VisibleLength` stubs beside the tests; `a37a5a5` replaces them. Not a deviation from behaviour, noted for the gate history.

**Total deviations:** 1 auto-fixed (1 blocker, test-only) plus 1 process note. **Impact:** none on production behaviour; the commands answer exactly as before.

## Known Stubs

None. (The two RED-step stubs are gone in `a37a5a5`.)

## Threat Flags

None. No new network endpoint, auth path, file access or schema change. T-11-04 (escape, single pass), T-11-05 (link scheme) and T-11-06 (command behaviour unchanged) are mitigated and tested as the plan's register says; no package was installed (T-11-SC).

## Issues Encountered

None.

## Next Phase Readiness

Plan 11-10 (the dry run) can call `FillPlaceholders` and `VisibleLength`, and plans 11-11 and 11-13 can call `TextLengthRefusal`, `ValidButtonURL` and `ExtractMedia`. Nothing in this plan blocks them.

## Self-Check: PASSED

- Created files exist: `fuku/utils/content/editor_helpers_test.go`, `fuku/utils/formatting/fill_placeholders_test.go` (found on disk).
- Commits `b17c792`, `cd6ee86` and `a37a5a5` are ancestors of HEAD; `git rev-list --count 8f3a1a0..HEAD` was 3 before this SUMMARY commit.
- Every acceptance criterion of both tasks was re-run after the last code commit and passes (results above).
