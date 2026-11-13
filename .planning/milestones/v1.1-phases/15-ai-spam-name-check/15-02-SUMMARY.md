---
phase: 15-ai-spam-name-check
plan: 02
subsystem: api
tags: [claude, anthropic, names, unicode, nfkc, sha256, leaf-package, member-requirements]

requires:
  - phase: 07-raid-ai-image-checks
    provides: fuku/utils/claude client, buildRequest, parseVerdict, exec, the strict forced-tool image request
  - phase: 15-ai-spam-name-check
    provides: plan 15-01 (ai_action column, memberreq.RuleAI)
provides:
  - claude.Client.ClassifyNames, claude.Names, claude.NamesInstruction, claude.ErrUnsupportedNames
  - the shared newRequest / requestSpec assembler (the one place Options is applied)
  - fuku/utils/namecanon: Of (canonical form), the names hash (tag mrn1), VerdictKey (tag mrv1)
affects: [15-04, 15-05, 15-06, 15-09, 15-11]

plan_head_before: 6ba62adce179591997761d6d15fe98202f3a7ace
plan_head_after: 1e3137e1a4d86a89afe349ea298b6a80c462f86a
actuals:
  tokens: 13500
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "One newRequest(model, opts, requestSpec) owns the Options switch; each request kind only supplies its spec"
    - "A wire format shared by every replica and by scripts/claude-spike is pinned by one literal vector per hash, computed independently of the code under test"
    - "Non-ASCII test inputs are written as Go escapes, never as literal invisible characters"

key-files:
  created:
    - fuku/utils/claude/names.go
    - fuku/utils/claude/names_test.go
    - fuku/utils/namecanon/canon.go
    - fuku/utils/namecanon/canon_test.go
    - fuku/utils/namecanon/leaf_test.go
  modified:
    - fuku/utils/claude/request.go
    - AGENTS.md

key-decisions:
  - "Invisible and default-ignorable runes are dropped before NFKC (and once more after it), so U+3164 and U+FFA0 vanish instead of becoming spaces; whitespace (including the Cc controls tab and newline) is kept through that step and collapsed afterwards"
  - "Marks (Mn, Mc) are kept: dropping everything memberReqVisible calls not visible would strip the vowel signs of Devanagari, Arabic and other scripts; only Cc, Cf, Other_Default_Ignorable_Code_Point and U+2800 go"
  - "The version tags are unexported constants (namesTag, verdictTag) so namecanon exports Canon, Of and VerdictKey only"

requirements-completed: []

coverage:
  - id: D1
    description: "ClassifyNames sends exactly the three canonical strings in one strict forced request and reads the answer strictly; the image request is unchanged"
    requirement: "MREQ-14 (contribution only; completes none)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestClassifyNames' ./fuku/utils/claude"
        status: pass
      - kind: unit
        ref: "TestClaudeRequestShape and TestClaudeSpikeOptionsChangeOnlyTheirOwnField, unedited"
        status: pass
    human_judgment: false
  - id: D2
    description: "the canonical form, the names hash and the verdict key are defined once, pinned, and the package is a leaf"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestCanon' ./fuku/utils/namecanon"
        status: pass
    human_judgment: false

duration: 40min
completed: 2026-11-08
status: complete
---

# Phase 15 Plan 02: names request and canonical names Summary

**`claude.Client.ClassifyNames` sends a member's three canonical strings to the model as one strict, forced, single-number request and reads the answer with the image path's strict parser, and the leaf package `fuku/utils/namecanon` is now the one definition of the canonical names, the names hash and the verdict key. Nothing in the bot calls either yet.**

## Accomplishments

- `request.go`: the `Options` switch moved out of `buildRequest` into `newRequest(model, opts, requestSpec)`; `inputSchema` takes the property description. `buildRequest` keeps its signature and builds its image spec. Before the refactor I recorded the marshalled image request for six `Options` values (zero, auto tool choice, adaptive and omitted thinking, effort, max tokens) from a temporary test, and after it compared them: the files are byte-identical (`cmp` exit 0). The temporary test was deleted and never committed.
- `names.go`: `Names`, the code-owned `NamesInstruction` (research S-05's text with `ToolName` spliced in), `ErrUnsupportedNames`, `buildNamesRequest` and `ClassifyNames`: `ErrNotConfigured`, `ErrInvalidModel`, `ErrUnsupportedNames` (all three empty) before any request, a defensive rune cut (64, 64, 32), `json.Marshal` of `{first_name,last_name,username}` as the single user text block, `max_tokens` 128, the same 8 s `Timeout`, `exec` and `parseVerdict`. No new import; `TestClaudePackageIsLeaf` passes unedited.
- `namecanon`: `Of` (per field: drop Cc/Cf/default-ignorable/U+2800 keeping whitespace, NFKC, drop again, collapse whitespace and trim, cut by runes, username loses one leading `@` and is lower-cased), `Canon{First,Last,Username,Hash}`, the length-prefixed names hash, `VerdictKey`. Not judgeable (all empty, or a lone "Deleted Account") returns the zero `Canon` and false.
- `AGENTS.md`: the `fuku/utils/claude` Go-rules bullet now covers `ClassifyNames`, the shared `newRequest` and the forced-tool-call model caveat; a new bullet defines `namecanon`, both hashes and the bump-the-tag-in-the-same-commit rule.

## Task Commits

1. `2d787c9` test(15-02): a names request for the model (red: does not compile until the method exists)
2. `2c076be` feat(15-02): ask the model about a member's names with one forced tool call (green; the refactor)
3. `e304a77` test(15-02): the canonical form of a member's names (red; see deviations)
4. `674f6e7` feat(15-02): one canonical form and two hashes for a member's names (green, plus the test file's escape fix)
5. `2108ec8` test(15-02): no answer, no verdict (characterization, passed on first run, as the plan says)
6. `1e3137e` docs(15-02): describe the names request and the canonical form (AGENTS.md and the last lint fix in the test file)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestClassifyNames$' -v ./fuku/utils/claude`: PASS (exit 0). The four other `TestClassifyNames*` tests pass in the same package run.
- `go test -tags testtools -race -count=1 ./fuku/utils/claude ./fuku/utils/namecanon`: ok, ok (exit 0). This includes `TestClaudeRequestShape`, `TestClaudeSpikeOptionsChangeOnlyTheirOwnField` and `TestClaudePackageIsLeaf`, none edited (`git diff --exit-code 8c0c827 -- fuku/utils/claude/claude_test.go` exit 0).
- `go test -tags testtools -race -count=1 -run '^TestCanon' -v ./fuku/utils/namecanon`: all of `TestCanonForm`, `TestCanonBounds`, `TestCanonHash`, `TestCanonJudgeable`, `TestCanonVerdictKey`, `TestCanonPackageIsLeaf` PASS.
- `go test -tags testtools -race -count=1 -run '^(TestRaidAIBreaker|TestRaidAIImageBreakerCap)$' ./fuku/modules`: ok (SQLite).
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/utils/claude ./fuku/utils/namecanon`: exit 0. `gofmt -l` on both packages: nothing.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0, read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827"). It first reported one staticcheck S1016 in my test (a struct literal where a conversion fits); fixed in commit 6, then exit 0. gocyclo stand-in over the production files of both packages: empty output (exit 0). No `dupl` finding, so `Classify` and `ClassifyNames` (key, model and validity checks, marshal, timeout, `exec`, `parseVerdict`) were left as two methods and `client.go` is not edited.
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0. Acceptance greps: `ClassifyNames(` in names.go 1; `newRequest(` in request.go 2; `func TestClassifyNames` 5; `func TestCanon` 5 + 1 = 6; `namecanon` in AGENTS.md 2; `ClassifyNames` in AGENTS.md 2.
- Not run: `make lint` and gosec (cannot run here), `make test` (this plan does not require it), the settings test family (the menu is not touched).

## Deviations from Plan

**1. [Rule 1 - own mistake] The first red commit of the canon tests did not parse.** My shell command carried the test inputs as `\u` escapes, but they reached the file as literal invisible and look-alike characters, so `canon_test.go` in `e304a77` held a literal byte order mark that `gofmt` refused. I fixed it before writing any production code by rewriting every non-ASCII character of the file as a Go escape, and shipped the fix in `674f6e7` (no history rewrite). The tests are unchanged in meaning. `canon.go` was written with numeric constants (`0x2800`) for the same reason.

**2. [Judgement within the plan] Dropped before NFKC.** The planner assumption says U+3164 and U+FFA0 "become spaces" under NFKC alone. I drop every default-ignorable rune before NFKC (and once after), so they vanish instead; the result is the same for all fields where the plan states a result (an all-filler first name is empty). Whitespace is exempted from the drop so that tab and newline, which are Cc, still collapse to a space. Marks are kept (see key-decisions).

**3. Test input fixed, not production.** The first run of `TestClassifyNamesInjection` failed because my hostile first name was over 64 runes and the defensive cut shortened it, which is the intended behaviour; I shortened the test inputs (and added a guard that each input is within its limit) rather than touch `names.go`. In `TestClassifyNamesErrors` the `%#v` dump of an `*HTTPError` prints its `Message` field (the documented spike-only place a server's own words are kept, key redacted), so the names-and-model check covers `Error()`, `%v` and `%+v` and the key check covers all four forms plus `Message`, as the existing `TestClaudeErrorsNeverContainKey` does for the key.

No other deviation. No production fix was needed after any new test: `names.go` is as first written.

## Requirements

`requirements-completed: []`. MREQ-14 gets its request and canonical form here and stays Pending; MREQ-01 and MREQ-10 are untouched. REQUIREMENTS.md is unchanged. PLAT-06 stays as it was.

## Known Stubs

None. Nothing calls `ClassifyNames` or `namecanon` yet by design: plan 15-04 (the rule) and plan 15-09 (the spike tool) are the first users, and both are exercised by this plan's own tests, so no unused identifier is left for CI.

## Threat Flags

None. No new endpoint, auth path or file access. T-15-04 to T-15-07 mitigated as planned (injection stays inside JSON string values, no key or name in an error, strict parser, length-prefixed pinned hashes); T-15-SC: `go.mod` and `go.sum` unchanged.

## Self-Check: PASSED

All five created files exist and all six commits (2d787c9, 2c076be, e304a77, 674f6e7, 2108ec8, 1e3137e) are ancestors of HEAD, checked with `git merge-base --is-ancestor`.
