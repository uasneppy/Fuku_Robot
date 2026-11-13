---
phase: 15-ai-spam-name-check
plan: 09
subsystem: spike-tool
tags: [member-requirements, ai-spam-name, claude-spike, names-mode, owner-checklist]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-02 (fuku/utils/namecanon and claude.Client.ClassifyNames), 15-08 (the page the checklist walks through)
provides:
  - "scripts/claude-spike -names <csv> and -repeat N: the owner's measurement of Haiku's verdicts on a labelled list of names, through the bot's own canonical form and request"
  - "names.go: readNames, namesRowOf, judgeName, judgeAll, repeatReportOf, falsePositiveRate, writeNamesCSV, writeNamesSummary"
  - "shared helpers extracted from the image mode: describeError, finishResult, forEachParallel, withCSVOut, and the validate split (validateBasics, validateModes, validateOptions)"
  - "15-SPIKE.md: the owner's non-blocking checklist (11 numbered checks, the pre-enable section, follow-ups, accepted limits)"
affects: [15-11]

plan_head_before: 06b1b3e76ad805c52f68d061c1f063f3895c64dd
plan_head_after: 33a9439935f7d17694fd9a12e57b77c3015a8804
actuals:
  tokens: 19500
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "The names mode builds result values (File holds the row number, never a name) and calls the image mode's summarize, sweep, totalsOf and writeSummary; a Unit field on summary and totals changes the printed noun from images to names, so the image summary text is unchanged"
    - "Each judgement of a row is one result; the summary uses the first judgement of each row, latency, tokens and cost count every request, so -repeat 5 does not weight a row five times"

key-files:
  created:
    - scripts/claude-spike/names.go
    - scripts/claude-spike/names_test.go
    - .planning/phases/15-ai-spam-name-check/15-SPIKE.md
  modified:
    - scripts/claude-spike/main.go
    - scripts/claude-spike/report.go
    - AGENTS.md

key-decisions:
  - "validate was already over the gocyclo limit (24) at HEAD, so touching it made CI's new-code check fail and the plan's own gocyclo gate impossible; it is split into three helpers, each well under 19"
  - "The summary and sweep are computed from the first judgement of each row; the repeat statistics use all of them"
  - "The names CSV echoes the three strings as the owner wrote them (the one place a name is printed); stderr carries row numbers and counts only"

requirements-completed: []

coverage:
  - id: D1
    description: "-names reads a labelled CSV, canonicalises each row with namecanon.Of, sends it through ClassifyNames and prints counts by label, accuracy, precision, recall, the false-positive rate at 0.9 (or -bar), a sweep, latency, tokens and the cost per 1,000 names; exits 1 when every request failed; no request before the key is found"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: unit
        ref: "TestRunNamesMode (four subtests, -race), TestNamesSummaryAtBar"
        status: pass
    human_judgment: false
  - id: D2
    description: "the file format: columns in any order plus note, a byte order mark, quoted fields, non-Latin names, bad rows skipped and counted by kind and never sent, the 5000-row cap, -limit, an empty or unusable file sends nothing and exits 1"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: unit
        ref: "TestNamesFileParse"
        status: pass
    human_judgment: false
  - id: D3
    description: "-repeat N reports per row the minimum, maximum and spread, the rows that straddle the bar and the largest spread; a row with a failed or refused repeat is excluded and counted; -repeat 1 prints no repeat line"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: unit
        ref: "TestNamesRepeatStability"
        status: pass
    human_judgment: false
  - id: D4
    description: "the tool's contract holds in names mode: the key never reaches stdout, stderr or the CSV (also when the server echoes it as an error message, an error type or a header), no name on stderr or on stdout when -csv is set, -names is exclusive with -dir, -probe and -models-get, a bad -repeat or a non-https endpoint is refused before anything is sent, the bar defaults to 0.9 only in names mode"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: unit
        ref: "TestNamesModeKeyNeverPrinted, TestNamesModeExclusiveFlags, and the unedited TestSpike* tests"
        status: pass
    human_judgment: false
  - id: D5
    description: "15-SPIKE.md exists with the pre-enable section, 11 numbered checks, the native-speaker table and what new member means; the live Haiku spike is the owner's step"
    requirement: "MREQ-14 (contribution only)"
    verification:
      - kind: manual
        ref: "owner runs go run ./scripts/claude-spike -names names.csv -repeat 5 -csv out.csv with their key"
        status: pending
    human_judgment: true

duration: about 45 min (not timed at start; estimated from the session)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 09: the names spike and the owner's checklist Summary

**`scripts/claude-spike` gained a names mode (`-names <csv>`, `-repeat N`) that sends a labelled list through the bot's own `namecanon.Of` and `claude.Client.ClassifyNames` and reports accuracy, precision, recall, the false-positive rate at the 0.9 bar, a sweep of bars, latency, tokens, cost per 1,000 names and the stability of repeated verdicts, and `15-SPIKE.md` is the owner's checklist for everything no test can prove; the live Haiku spike needs the owner's key and was not run.**

## Accomplishments

- **Names mode (`names.go`).** `readNames` reads RFC 4180 CSV with `encoding/csv` (header columns in any order plus an optional `note`, a byte order mark tolerated, `label` spam or ham in any case, at most 5,000 rows, `-limit`), counts bad rows by kind (malformed or wrong field count, bad label, nothing to judge, past the cap) and never sends them. `judgeAll` judges each row `-repeat` times in sequence on one worker, through `namecanon.Of` and `ClassifyNames`. `writeNamesSummary` calls the image mode's `summarize`, `sweep` (with 0.5, 0.7, 0.8, 0.85, 0.9, 0.95, 0.97), `totalsOf` and `writeSummary`, adds the counts by label, one false-positive-rate line, the mean tokens, and with `-repeat` the stability report (names with all judgements answered, the excluded, the names straddling the bar, the largest and mean spread with the row number). The names CSV has the row number, label, the three strings as written, and the first judgement's probability, finish text, milliseconds and tokens; with `-repeat` it adds min, max and spread.
- **Flags (`main.go`).** `-names` and `-repeat` (1 to 10, only with `-names`); `-bar` is 0.9 in names mode unless given (detected with the flag set's `Visit`, so an explicit `-bar 0.85` wins); exactly one of `-dir`, `-names`, `-probe` and `-models-get`; the usage and header comment describe the mode and the data warning. The tool imports only `fuku/utils/claude` and `fuku/utils/namecanon` of the repository (`go list` confirms); it never reaches `fuku/config` or `fuku/db`.
- **Shared code, not copies.** `describeError` and `finishResult` (the finish text and result fill of one request), `forEachParallel` (the worker pool) and `withCSVOut` (the `-csv` file or stdout) were extracted from the image mode and are used by both modes, so `classifyOne`, `runLabelled` and `writeOutputs` behave as before and `dupl` finds no clone. `report.go` gained a `Unit` field on `summary` and `totals` (zero means images), so the names run prints "names" in the same lines; the image summary text is byte-identical.
- **`15-SPIKE.md`.** Opens by saying nothing blocks the phase and the live spike cannot run in this build, then "Before you enable Mute, Kick or Ban for the AI rule in a real group" (every rule Off by default, no key or `ENABLE_AISPAM=false` means the rule is off and the page says so, Warn or Mute recommended and Ban never, the model must accept a forced tool call, **"new" means recorded as joining while the rule is on** with the no-Redis and missed-join limits per UI-CHECK F-b), what the tests prove and cannot, before you start (the labelled file of at least 200 rows, the run command, what it prints, the data warning), 11 numbered checks each with steps, "Pass looks like" and "If not" (accuracy at 0.9, non-determinism, latency and tokens, request acceptance, refusals, the test-group walk-through, the phone check of the cell, two replicas, a restart during joins, the native-speaker read with the table copied from the 15-03 SUMMARY, CI), follow-ups and accepted limits.
- **AGENTS.md.** The Go-rules bullet on `fuku/utils/claude` also names `namecanon`; the Testing bullet on the live spike describes the names mode, its flags, its output handling, its six tests and the checklist.

## Task Commits

1. `e8062f4` test(15-09): the spike tool judges a list of names (red: `TestRunNamesMode` failed, the tool had no `-names`)
2. `db49eff` feat(15-09): measure Claude's verdicts on a labelled list of names (green; also the `validate` split and the shared helpers)
3. `309c7c8` test(15-09): the names file, the summary, the repeats and the flags (the other five tests)
4. `33a9439` docs(15-09): the owner's live checks for the AI spam-name rule and the names spike

`commits: 4` is `git rev-list --count 06b1b3e..HEAD` before this SUMMARY's own commit (`plan_head_before` above).

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestRunNamesMode$' -v ./scripts/claude-spike`: PASS (four subtests); before the implementation it failed (`every_request_failing_exits_1` exit 2, want 1).
- The six names tests, `-v`: all PASS, no FAIL or SKIP. `go test -tags testtools -race -count=3 ./scripts/claude-spike`: ok (5.9 s). `go test -tags testtools -race -count=1 ./scripts/claude-spike ./fuku/utils/claude ./fuku/utils/namecanon`: three `ok` lines.
- `go vet -tags testtools ./scripts/claude-spike`: exit 0. `gofmt -l scripts/claude-spike`: nothing. `CGO_ENABLED=0 go build -o /dev/null ./scripts/claude-spike`: exit 0.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, exit read directly: 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"), run before the code commit and again before the docs commit.
- `test -z "$(~/.cache/fuku-lint-tools/gocyclo -over 19 $(ls scripts/claude-spike/*.go | grep -v _test.go))"`: exit 0 (empty). At HEAD `validate` was 24, so this gate could only pass after the split.
- Acceptance greps: `grep -c 'func runNames(' names.go` 1, `grep -c 'namecanon.Of' names.go` 1, `grep -c 'func TestNames' names_test.go` 5 (plus `TestRunNamesMode`, six tests in all), `grep -c 'summarize(' names.go` 1, `grep -c 'func percentile' names.go` 0, `grep -c 'Skip(\|Parallel()' names_test.go` 0, `grep -cE '^### [0-9]+\.' 15-SPIKE.md` 11, `grep -q 'Before you enable'` and `grep -qi 'recorded as joining'` exit 0, `grep -c 'claude-spike' AGENTS.md` 4, `grep -c -e '-names' AGENTS.md` 3.
- Invariants: `git diff --exit-code 8c0c827 -- scripts/claude-spike/main_test.go scripts/claude-spike/report_test.go` exit 0 (the existing tests are not edited and all pass); `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `go list -f '{{join .Imports "\n"}}' ./scripts/claude-spike | grep fuku` prints only `fuku/utils/claude` and `fuku/utils/namecanon`.
- Not run: `make lint` and gosec (cannot run in this sandbox; the local stand-in covered `dupl`, `gocyclo`, `godox`, `staticcheck` and `errcheck` on the added lines), `make test` (this plan does not require it; 15-11 does), PostgreSQL (nothing here rests on it), the live Haiku spike (no Anthropic key in this build: the owner's step, never CI).

## Deviations from Plan

1. **[Rule 1 - Bug/blocker] `validate` split into three helpers.** `validate` was already at cyclomatic complexity 24 at HEAD, over CI's 19, so adding the mode and `-repeat` checks to it would have been flagged as new-code debt and the plan's own gocyclo gate (empty output over the spike tool's production files) could not pass. It is now `validate` calling `validateBasics`, `validateModes` and `validateOptions`; every check and message of the image mode is unchanged.
2. **[Rule 3 - dupl] Shared helpers extracted from the image mode** (`describeError`, `finishResult`, `forEachParallel`, `withCSVOut`) so the names mode copies neither `classifyOne`'s error switch, `runLabelled`'s worker pool nor `writeOutputs`'s file handling. `classifyOne`, `runLabelled` and `writeOutputs` keep their behaviour; the existing `TestSpike*` tests pass unedited.
3. **[Judgement] A `Unit` field on `summary` and `totals` in `report.go`** (zero means images) so the names run prints "names" instead of "images" in the same lines; the plan said the image summary text stays pinned, and it does (zero value).
4. **[Judgement] Task 2's tests all passed on first run, and no Task 2 production commit exists.** The reader, the skip counters, the repeat loop and statistics, the cap and `-limit`, and the `validate` mode and `-repeat` checks landed in the Task 1 commit because they are the same loop and the same structure; the plan expected this ("several pass at once"). Task 2's commit is the five tests. The one failure met while writing them was in the test: the client retries a 500 once, so the flaky-row fixture answers 401 instead.
5. **[Judgement] `-repeat` summary basis.** The confusion matrix and sweep use the first judgement of each row; latency, tokens and cost count every request (so "per 1000 names" is per request). The plan did not say which judgement the matrix takes.
6. **[Judgement] A missing required header column is an error exiting 1** (like an empty file), not a usage error, because the file is the input rather than a flag.

No unused identifier is left: every new function and constant is read by the names mode or a test, and the lint stand-in's staticcheck `unused` check is clean.

## Known Stubs

None.

## Requirements

`requirements-completed: []`. MREQ-14 gets its measurement tool and checklist here and stays Pending, as do MREQ-01 and MREQ-10; they complete in plan 15-11 after its end-to-end test. REQUIREMENTS.md is unchanged.

## Threat Flags

None. No endpoint, auth path or schema; T-15-26 (key or name on stderr, in an error or in another file) is mitigated and tested (`TestNamesModeKeyNeverPrinted`, also with a server that echoes the key as a message, as the error type and as a header); T-15-27 (key to a plain-http endpoint) by the existing `validEndpoint` in names mode (`TestNamesModeExclusiveFlags`); T-15-SC: `go.mod` and `go.sum` unchanged.

## Self-Check: PASSED

Created files exist (`names.go`, `names_test.go`, `15-SPIKE.md`), and commits `e8062f4`, `db49eff`, `309c7c8` and `33a9439` are ancestors of HEAD.
