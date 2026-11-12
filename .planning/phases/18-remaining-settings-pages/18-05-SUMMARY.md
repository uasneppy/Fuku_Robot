---
phase: 18-remaining-settings-pages
plan: 05
subsystem: settings-menu
tags: [telegram, settings-menu, aispam, apply-function, golden, reader, docs]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plans 18-01 to 18-04 (the apply-function pattern, the goldens technique, the Phase 18 string keys)"
provides:
  - "TestLegacyAISpamRequests: the /aispam golden (27 cases) recorded and committed alone before the extraction"
  - "aispamRefusalKey and aispamConfigured (the TypeSafe value set and ENABLE_AISPAM on, one definition, no network), aispamWrites, aispamApplyResult and applyAISpamEnabled"
  - "aispam.LoadEnabledContext, the error-returning, non-creating AI spam reader on the command's ai_spam_settings key"
  - "withAISpamConfigured and withAISpamNotConfigured test helpers (reused by 18-06)"
affects: [18-06, 18-15]

plan_head_before: 1035c46
plan_head_after: fafb387

actuals:
  tokens: 9400    # chars/4 over the added non-planning lines of 1035c46..fafb387 (37749 chars), tests and docs included
  tasks: 2
  commits: 3      # MEASURED: git rev-list --count 1035c46..fafb387 (before this SUMMARY commit)

key-files:
  created:
    - fuku/modules/aispam_legacy_request_test.go
    - fuku/modules/aispam_settings_apply_test.go
    - fuku/db/aispam/load_enabled_test.go
  modified:
    - fuku/modules/aispam.go
    - fuku/db/aispam/repository.go
    - AGENTS.md

key-decisions:
  - "Availability lives in one function, aispamRefusalKey (the locale key of the reason the filter cannot run, or empty); aispamConfigured is its empty test and the apply function's refusals use its keys, so the command and the page cannot drift. The Anthropic key is not part of it (D-22)."
  - "The save-failure log line moved from the command into applyAISpamEnabled so the page logs exactly what the command logs and nothing new."
  - "The command still reads its current value through aispam.IsAISpamEnabled (hides a read error as off); only the page uses LoadEnabledContext."

requirements-completed: []

coverage:
  - id: D1
    description: "/aispam sends, stores and returns exactly what it did before, through one apply function"
    requirement: "PAGE-02, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyAISpamRequests|TestAISpamCommandTogglesChatSetting)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyAISpamEnabled refuses On exactly where /aispam on refuses; the reader returns its error and creates no row"
    requirement: "PAGE-02, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestApplyAISpamEnabled|TestAISpamConfiguredMatchesCommand)$' ./fuku/modules; ... -run '^TestLoadEnabledContext$' ./fuku/db/aispam"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 05: /aispam golden, aispamConfigured, applyAISpamEnabled and the AI spam reader Summary

`/aispam on|off` now writes through `applyAISpamEnabled` behind a golden recorded first, filter availability is one pure function shared by the command and the page, and the switch reads with its error through `aispam.LoadEnabledContext`.

## What changed

- **Golden first (commit 45d7082).** `TestLegacyAISpamRequests` drives the real dispatcher with `LoadAISpam` and records, in 27 cases: the status in the four configuration states (value set or empty, global switch on or off) with the stored state on and off, plus a chat with no row (no row is created by a status); `on`, `yes`, `true`, `enable`, `ON`, `off`, `no`, `false`, `disable`; the invalid word; `on` refused with the value empty, with the switch off and with both missing (nothing written); `off` while not configured (writes); and a save failure for on and off through a real GORM create callback, where the reply is `common_settings_save_failed` and the dispatcher's error handler receives exactly the save error (every other case receives none). The request JSON goldens (Ukrainian, the default) and the stored row (none, on, off) are asserted. Equal cases were checked equal when the goldens were generated; green on the unchanged code before the commit.
- **Extraction (commit 75a633e).** In `aispam.go`: `aispamRefusalKey` (value first, then the global switch), `aispamConfigured`, `aispamWrites` (default `aispam.SetAISpamEnabled`), `aispamApplyOutcome` (`aispamApplied` zero value, `aispamNoKey`, `aispamGloballyDisabled`, `aispamSaveFailed`), `aispamApplyResult` and `applyAISpamEnabled` (refuses On before any write, never refuses Off, logs a failed save once, reads the switch back through the reader under `greetingApplyTimeout` and falls back to the requested value when only the read-back fails). The command keeps `parseToggleArg`, the status text and the `return err` after the failed-save reply; the unused `aispamEnableRefusal` was removed because the apply result's key replaces it. In `fuku/db/aispam/repository.go`: `LoadEnabledContext(ctx, chatID) (bool, error)` over the existing cached getter and `ai_spam_settings:<chat>` key. AGENTS.md: the "One setting, one code path" bullet gains the AI spam paragraph.
- **Tests (commit fafb387).** `TestApplyAISpamEnabled` (configured on/off/on with read-back; On refused by an empty value or the switch off with zero write-seam calls and no row; Off writes in both unconfigured states; a failing write is `aispamSaveFailed` with the error; the write drops the cached copy a prior read filled), `TestAISpamConfiguredMatchesCommand` (four combinations, real `/aispam on`: reply text and write count agree with `aispamConfigured`), `TestLoadEnabledContext` (missing row off, nil error, no row created; a write is seen; an injected read error is returned while the hiding getter still reads off), the helpers `withAISpamConfigured` and `withAISpamNotConfigured(t, how)` (`aispamProviderValueEmpty`, `aispamSwitchOff`) and a Testing bullet in AGENTS.md. The stand-in provider value is a constant named `...ProviderValue`.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestLegacyAISpamRequests|TestAISpamCommandTogglesChatSetting)$' -v ./fuku/modules`: PASS; `-run '^TestLoadEnabledContext$' ./fuku/db/aispam`: PASS (`ok 1.043s`).
- `-run '^(TestApplyAISpamEnabled|TestAISpamConfiguredMatchesCommand|TestLegacyAISpamRequests)$' -v ./fuku/modules`: `--- PASS` for all three, `ok ... 3.524s`.
- Whole `fuku/modules` package, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log in the scratchpad `modules-18-05.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1563.765s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1686 top-level PASS lines.
- Acceptance greps: `func TestLegacyAISpamRequests(` 1; `func TestLoadEnabledContext(` 1; `func applyAISpamEnabled(\|func aispamConfigured(` 2; `func LoadEnabledContext(` 1; the two apply tests 1 each; the two helpers 2; the gosec G101 identifier grep over both new test files 0; `git log --reverse --format=%s 29aca33..HEAD -- <golden file> aispam.go | head -n 1` prints `test(18-05): pin /aispam requests before the extraction`.
- `go vet -tags testtools ./fuku/modules ./fuku/db/aispam` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the five files prints nothing; `python3 .../lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0.

## Deviations from Plan

- **Removed `aispamEnableRefusal`** (a small, non-planned deletion). The plan says to make it call the shared definition; after the command mapped onto the apply result's key nothing called it, and an unused function would fail CI's `unused`. The shared definition is `aispamRefusalKey`, and the status text (`aispamStatusText`) is unchanged. The golden passes unchanged.
- Task 2 needed no production change, as the plan expected.
- The plan's second `<automated>` block ran as a plain background `go test` with the log in the scratchpad (the run was interrupted by an API rate limit while polling and finished on its own; the result above is from that complete run).

## Known Stubs

None. `aispamConfigured`, `applyAISpamEnabled` and `LoadEnabledContext` have no production caller beyond the command until 18-06; each is exercised by tests of this plan.

## Threat Flags

None. T-18-15 is mitigated by `applyAISpamEnabled` and `TestAISpamConfiguredMatchesCommand`; T-18-16 by the naming rule and grep gate (the value is never logged); T-18-17 by the golden recorded first.

## Requirements

PAGE-02 and PAGE-10 are contributed to and **left pending**; `requirements-completed: []`, `requirements.mark-complete` not run (plan 18-15 completes them).

## Self-Check: PASSED

- Files: aispam_legacy_request_test.go, aispam_settings_apply_test.go, load_enabled_test.go, aispam.go, repository.go present.
- Commits 45d7082, 75a633e, fafb387 are ancestors of HEAD.
