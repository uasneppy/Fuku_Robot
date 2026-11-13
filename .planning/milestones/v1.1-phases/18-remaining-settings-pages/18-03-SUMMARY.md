---
phase: 18-remaining-settings-pages
plan: 03
subsystem: settings-menu
tags: [telegram, settings-menu, warns, apply-function, parity, drift, authority, i18n, docs]

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-01 (applyWarnLimit, applyWarnMode, the reader, the limit stepper) and plan 18-02 (the Warnings keys in 8 locales)"
provides:
  - "the finished Warnings page: action line, [Mute | Ban | Kick] choice row (control m) through applyWarnMode, the inert line, the bound toasts"
  - "per-setting proofs: parity, inert action, bounds, forged presses, drift, authority parity, reads-create-nothing, callback bytes, fit in 8 languages"
  - "AGENTS.md, README, callbacks.md, permissions.md and the start of 18-SPIKE.md"
affects: [18-04, 18-05, 18-06, 18-07, 18-08, 18-09, 18-10, 18-11, 18-12, 18-13, 18-14, 18-15]

plan_head_before: aec96eb31b5fe1d285d975339e8756e3eb8f62be
plan_head_after: 6d89502033c787d0823a393c48b1f12cd51cab9b

actuals:
  tokens: 12800   # chars/4 over the added lines of aec96eb..6d89502 (51059 chars), tests and docs included
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count aec96eb..HEAD (before this SUMMARY commit)

key-files:
  created:
    - fuku/modules/settings_warns_screens_test.go
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md
  modified:
    - fuku/modules/settings_warns.go
    - fuku/modules/settings_warns_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md

key-decisions:
  - "The action control m reuses the Antiflood word shape (settingsFloodModeWord) as its Validate and settingsFloodActionWord for the stored word; no shared 'settable' function was needed because the page derives the marked choice from warnSettingsModes, the list applyWarnMode itself accepts, so settings_antiflood.go is untouched."
  - "settingsWarnsRedraw and settingsWarnsReadBefore are shared by both controls (one redraw path, no dupl)."

requirements-completed: []

coverage:
  - id: D1
    description: "An administrator picks Mute, Ban or Kick on the Warnings page and the stored row and cache equal what /setwarnmode leaves; /warnings and Main show it"
    requirement: "PAGE-01 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestSettingsParityWarnMode$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "A stored tban, tmute or empty action says plainly it is not acted on; bounds answer a toast; forged presses are refused before any lookup"
    requirement: "PAGE-01 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsWarnsInertAction|TestSettingsWarnsBounds|TestSettingsWarnsForged)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "Drift, authority parity (with the owner-gate carve-out), reads-create-nothing, 64-byte callbacks (48 private, 44 group) and fit in 8 languages"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsDriftWarns|TestSettingsAuthorityParityWarns|TestSettingsWarnsReadsCreateNothing|TestSettingsWarnsCallbackBytes|TestSettingsWarnsPageFitsEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false

duration: 95min
completed: 2026-11-12
status: complete
---

# Phase 18 Plan 03: the finished Warnings page Summary

The Warnings page now sets the action at the warn limit as well as the limit: a `[Mute | Ban | Kick]` row writes through `applyWarnMode`, the same function `/setwarnmode` calls, a stored `tban`, `tmute` or empty action is shown with a plain "I don't act on this action" line, and the page is proved against both commands on drift, rights, bytes and length.

## What was built

- **Action control** (`settings_warns.go`, commit aa2b1e1): control `m` (`settingsWarnsModeKey`), `st` only, `Validate` `settingsFloodModeWord` (1 to 10 lower-case letters), right `settingsRightAdminFloodBot`. The body gains `settings_warns_action` (`settingsWarnsActionWord`: Antiflood words, stored text escaped, Not set for empty) and the choice row marks only a mode in `warnSettingsModes`. `settingsWarnsSetMode` reads before, calls `applyWarnMode` and redraws from the read-back; an unknown lower-case word redraws the stored state with `warns_mode_unknown` formatted with the escaped word, a failed save with `common_settings_save_failed`. The limit and action controls share `settingsWarnsReadBefore` and `settingsWarnsRedraw`. Main's state uses the same word function.
- **Inert line** (commit f06141d): `settings_warns_action_inert` under the action line and no marked choice for any stored action outside ban, kick, mute; Mute, Ban or Kick replaces it. The bound toasts (`settings_min_value` / `settings_max_value` at 1 and 100, no write) already came with 18-01; this plan proves them, including a stored limit above 100 (every step clamps to 100) and that a bound press never reaches the write seam.
- **Proofs** (commits aa2b1e1, f06141d, 6d89502): `TestSettingsParityWarnMode`, `TestSettingsWarnsInertAction`, `TestSettingsWarnsBounds`, `TestSettingsWarnsForged` (13 forged shapes refused with no `getChatMember`, plus well-formed `tban`, `tmute`, `warn`, `xyz` refused by the apply function as a notice), `TestSettingsDriftWarns` (commands, direct row edits, a second replica), `TestSettingsAuthorityParityWarns` (bot not admin, bot admin, plain member, owner-gate carve-out), `TestSettingsWarnsReadsCreateNothing`, `TestSettingsWarnsCallbackBytes` (largest 48 private, 44 group, widths `[5 3 2]`) and `TestSettingsWarnsPageFitsEveryLocale` (8 languages, three 64-rune titles, the longest notices).
- **Docs** (commit cac23d1): AGENTS.md's Warnings bullet rewritten for the finished page (including the `/reset warnings` empty-mode defect left to the owner); README sentence; `callbacks.md` Warnings table; `permissions.md` row; new `18-SPIKE.md` with checks 1 to 3.

## Deviations from Plan

None to the production design. Edits to earlier-plan tests (expectations the new keyboard shape legitimately changes, nothing weakened or re-recorded): in `settings_warns_test.go` (18-01's file) `settingsWarnsStepper` now expects rows of 5, 3 and 2 buttons instead of 2 rows, `TestSettingsWarnsOpens` expects 3 keyboard rows (navigation row is now row 3) and gained assertions for the action line and the marked Mute; `settingsWarnsCommand` now also dispatches `/setwarnmode` and `/warnings`. `TestSettingsParityWarnLimit` and the golden `TestLegacyWarnSettingsRequests` are untouched. No Main-shape constants changed (`settings_main_test.go` untouched). `settings_antiflood.go` is unchanged (listed in the plan only as a possible edit).

**Total deviations:** 0 auto-fixes. **Impact:** none.

## Verification (real results)

- Task 1 gate: `TestSettingsParityWarnMode|TestSettingsParityWarnLimit|TestSettingsWarnsOpens|TestLegacyWarnSettingsRequests|TestSettingsAntiflood.*|TestSettingsParityAntiflood|TestSettingsDriftAntiflood` all PASS (ok, 22.6s).
- Task 2 gate: `TestSettingsWarnsInertAction|Bounds|Forged|Opens` PASS; `make check-docs`: "No drift detected".
- Task 3 gate: `TestSettingsDriftWarns|AuthorityParityWarns|CallbackBytes|PageFitsEveryLocale|ReadsCreateNothing` PASS (ok, 7.5s).
- Whole `fuku/modules` package, race, background, `-v`, log in the scratchpad (`modules-18-03.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1670.004s`, `exit 0`, 1678 top-level `--- PASS`, 0 matches of `--- (FAIL|SKIP)|^FAIL`. The settings family is part of it.
- `go test -tags testtools -race -count=1 ./fuku/i18n`: ok (24.4s). `make check-translations`: all present.
- `gofmt -l` on the four touched Go files: nothing. `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean.
- `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go migrations fuku/db/models`: exit 0. `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Lint stand-in `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings"). `golangci-lint` itself cannot run here (Go 1.25 build); CI's govet `inline` analyzer not run locally, and no legacy aliases (`reflect.Ptr`, `ioutil`) are used.
- Acceptance greps: all the plan's test-name greps print `1`; `applyWarnMode(` count 1; `settings_warns_action_inert` present; both docs pages keep the frozen marker in the first 512 bytes; `/setwarnmode` in `permissions.md`; `18-SPIKE.md` exists.
- Not run: PostgreSQL (the `fuku/modules` harness does not run on it here; this plan has no migration or database guarantee), live Telegram checks (recorded as owner checks in `18-SPIKE.md`).

## Known Stubs

None.

## Threat Flags

None: no new endpoint, auth path or schema; T-18-07 to T-18-10 mitigations are the tests above.

## Requirements

Contributes to PAGE-01 and PAGE-10; completes none (`requirements-completed: []`). Left Pending for plan 18-15.

## Self-Check: PASSED

Files exist (`settings_warns.go`, `settings_warns_screens_test.go`, `18-SPIKE.md`); commits aa2b1e1, f06141d, cac23d1, 6d89502 are on the branch.
