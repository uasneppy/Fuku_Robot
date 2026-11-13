---
phase: 19-blacklist-filter-and-note-pages
plan: 12
subsystem: phase-closing
tags: [rulebook, docs, owner-checklist, phase-gate, requirements]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01 to 19-11, the apply functions, the list engine, the three pages, the sweeps and the end-to-end proof
provides:
  - AGENTS.md checked against the code and corrected where a Phase 19 plan left it incomplete
  - README, callbacks.md and permissions.md describing the Blacklist, Filters and Notes pages
  - 19-SPIKE.md, the owner's live checks, the six owner-visible command changes with their revert commits and the open confirmations
  - the phase gate run with its documented fallbacks, and PAGE-07, PAGE-08 and PAGE-09 completed
affects: []

plan_head_before: b5be876
plan_head_after: a698358

actuals:
  tokens: 9100    # chars/4 over the added lines of b5be876..a698358 outside .planning (8542 chars) plus 19-SPIKE.md (27848 chars)
  tasks: 3
  commits: 3      # MEASURED: git rev-list --count b5be876..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "the closing plan changes no production code; the gate runs on the tree 19-11 left plus documentation"

key-files:
  created:
    - .planning/phases/19-blacklist-filter-and-note-pages/19-SPIKE.md
  modified:
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "No production code changed and no fix(19-12) commit was needed: every gate command passed or hit only the documented sandbox fallback."
  - "SPIKE records that a plain git revert of an owner-confirmation commit now conflicts in tests and AGENTS.md (checked with git apply -R --check), because later plans added to the same files; the production hunk of each is small and named."

requirements-completed: [PAGE-07, PAGE-08, PAGE-09]

coverage:
  - id: G1
    description: "the whole fuku/modules package passes under the CI environment on the final code"
    requirement: "PAGE-07, PAGE-08, PAGE-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -json -race -count=1 -timeout 40m ./fuku/modules (cienv.sh): ok 1897.185 s, 1841 top-level PASS, 0 FAIL, 0 SKIP"
        status: pass
    human_judgment: false
  - id: G2
    description: "every Phase 19 test name passes, bare and under the CI environment, through scripts/check_test_results"
    requirement: "PAGE-07, PAGE-08, PAGE-09"
    verification:
      - kind: unit
        ref: "100 names over ./fuku/modules ./fuku/i18n ./fuku/db/blacklists ./fuku/db/filters ./fuku/db/notes ./fuku/db/migrations ./fuku/utils/content: 99 pass, 1 allowed PostgreSQL-only skip, check_test_results exit 0 in both environments"
        status: pass
    human_judgment: false
  - id: G3
    description: "the PostgreSQL integrity list passes on a chain-built throwaway cluster, the new blacklist action test included"
    requirement: "PAGE-07"
    verification:
      - kind: integration
        ref: "make test-postgres-integrity with FUKU_TEST_DATABASE=true on PostgreSQL 16: exit 0, 175 PASS, 0 SKIP, TestBlacklistActionValuesOnPostgres PASS, only chk_blacklist_action left on blacklists"
        status: pass
    human_judgment: false
  - id: G4
    description: "the owner has every live check and every owner-visible command change with its revert commit"
    requirement: "PAGE-07, PAGE-08, PAGE-09"
    verification:
      - kind: manual
        ref: "19-SPIKE.md: 15 numbered checks, six command changes, A19-1 to A19-11, A19-A1 to A19-A17"
        status: pass
    human_judgment: true

---

# Phase 19 Plan 12: the closing plan Summary

**The rulebook was checked against the code and completed, README and the docs describe the three list pages, the owner has a 15-point checklist with the six command changes and their revert commits, the phase gate passed with the documented sandbox fallbacks, and PAGE-07, PAGE-08 and PAGE-09 are complete.**

## What was built

- **Task 1 (`efc39e4`, tracer): rulebook, README, docs.** AGENTS.md: the Phase 19 bullets written by 19-01 to 19-11 were checked sentence by sentence against the code (see Corrections below) and four gaps closed. README gets one paragraph per page. `callbacks.md` gets the list pages' action table (17 codes, fields, rights, byte sizes) and `permissions.md` gets six rows for the three pages plus a paragraph on the two-step typed add. `make generate-docs` and `make check-docs`: no drift; both frozen pages keep their marker.
- **Task 2 (`c2cc48c`): `19-SPIKE.md`.** The owner's checklist in the format of `18-SPIKE.md`: a "Before you rely on it" section, "Command behaviour this phase changed" (six items, each with what it does now, its reply key and its commit: `12ee6a4` A19-2 `/addblacklist` `blacklists_blacklist_word_too_broad`; `f0dd7f7` A19-2 `/filter` `filters_keyword_too_broad`; `3025b0f` A19-3; `2eeac8f` A19-4 `notes_name_invalid`; `4f794f1` `/filter` fail-closed `common_settings_save_failed`; `ed7988a` `/save` fail-closed `notes_save_failed`; plus migration commit `76408de` for the C-16 constraint), 15 numbered checks (C-16 on production, Main at 11 rows, long mixed-script entries, the two-step add in a real client, the effect in the group, a group menu showing nothing, the six changes live, the action screen, the native read of the 67 keys, A19-1 to A19-11, A19-A1 to A19-A17, deviations and the time overshoot, follow-ups, accepted limits, CI).
- **Task 3 (`a698358`): the gate, then PAGE-07, PAGE-08 and PAGE-09.** Gate results below. The three requirements were ticked through `gsd_run query requirements.mark-complete PAGE-07 PAGE-08 PAGE-09`; the diff of `.planning/REQUIREMENTS.md` was read and holds exactly the three checkboxes and the three traceability rows (Pending to Complete). PAGE-10 and PLAT-05 stay ticked. **PAGE-07, PAGE-08 and PAGE-09 are complete** because the end-to-end test of 19-11 (blacklist, filters, notes, Staff Group, no Redis, two replicas) and this gate both pass: each page lists its entries paged, deletes one behind a card, and adds by typing in the bot's private chat, with the write going through its command's apply function.

## AGENTS.md corrections (the Phase 19 bullets, read against the code)

Read: every Phase 19 region in full (the door's list branch and two-step flow, `PrivateOnly`, the Main bullet, the list engine, Blacklist, Filters and Notes bullets, the rights bullets, the wait's Data bullet, the apply-function bullet, the migration line, the key count and the Phase 19 test families) and the rest of the 4,322-line file by grep for the stale terms (385, 387, sixteen, seven apply functions, eight rights, seven scripts, fifteen/sixteen/seventeen Main cells, "no user text"). I did not read the Phase 1 to 18 regions that Phase 19 did not touch line by line; the inlined copy the session started with covered them and earlier closing plans checked them. Findings:

- **Already correct** (written by earlier plans and verified against the code): 452 `settings_` keys per locale file (counted from `locales/*.yml`), 18 grid cells and the grid order `s|l / c|f / n|k / b|i / w|g / r|e / m|d / t|u / o|y`, 11 owner rows and 10 for an administrator, nine Lua scripts of the wait (counted: nine `redis.NewScript` in `settings_input.go`), the bounded `w` and `q` fields, the ninth right, the list parts table `(b,w) (i,k) (i,y) (e,n) (e,c)`, the 17 list codes, the 64-byte Yes at a 17-digit ID, the 19 page keys, the migration line, every Phase 19 test name (all exist; a script compared all 449 `Test...` tokens of the file with the repository's test functions, and the 50 unmatched are family prefixes like `TestSettingsList`).
- **Fixed**: (1) the unbuilt fallbacks sentence named only Phase 18's hub; it now names both (the Phase 18 "More" hub and the Phase 19 `📚 Lists` hub, A19-A1) and the live check; (2) the `settings|v1` namespace bullet's code list stopped at the Blacklist's pager and delete codes; it now gives the seven Blacklist codes and the Filters and Notes codes (17 in all, all with `c`); (3) the rights bullet said "eight rights in all" in the Phase 18 sentence and said no page declares the ninth right until the Blacklist page does; it now says eight at the end of Phase 18, names which right each list page declares (Filters `settingsRightChangeInfo`, Notes `settingsRightChangeInfoBotAdmin`) and that viewing, paging and Cancel need only `settingsRightAdmin`; (4) the "One setting, one code path" bullet gets one Phase 19 summary: ten apply functions, the four caches outside `skipLocal`, the six on-purpose command changes and that every other byte of the ten commands is pinned.

## Gate (every command with its real result)

| Command | Result |
|---------|--------|
| `make test` as one command | **not run as one command.** Documented fallback of 12-16, 16-09, 17-12 and 18-15 used instead (below). `make test`'s `fuku/modules` package takes about 32 minutes and would have been run twice. |
| fallback part 1: `go test -tags testtools -json -race -count=1 -timeout 40m ./fuku/modules`, CI environment (`cienv.sh`), background, output in the scratchpad | exit 0, `ok ... 1897.185s` (limit 2400 s), 1841 top-level PASS, 0 FAIL, 0 SKIP |
| fallback part 2: `-json -race -coverprofile -coverpkg=<all fuku packages> -count=1 -timeout 40m` over every other package (`go list ./... \| grep -v /fuku/modules$`), CI environment, throwaway PostgreSQL 16 on 127.0.0.1:5432 (trust auth, data directory deleted afterwards, no DSN written), after `TestRepositoryMigrationChain` | 68 packages `ok`; `go run ./scripts/check_test_results` exit 0 after removing the three `fuku/utils/actionlog` events (two `build-output`, one `build-fail`) that report only `go: no such tool "covdata"` (the documented sandbox case, a package with no test files); the only skips are 10 PostgreSQL-only tests (`TestUpdateChat`, `TestBlacklistActionValuesOnPostgres`, `TestCaptchaModeTurnstileConstraint`, `TestMemberRequirementAIMigrationDefaults`, `TestMemberRequirementLookupMigrationDefaults`, `TestRaidDetectionCarryOver`, `TestRepositoryMigrationChain`, `TestRulesMediaMigrationDefaults`, `TestScheduledDeletionMigrationDefaults`, `TestStaffExclusivityTrigger`), because `FUKU_TEST_DATABASE` is not part of `cienv.sh`; they ran and passed in the PostgreSQL step below |
| every Phase 19 test name, one `-run '^(...)$'` (100 names: every test added by this phase plus the older tests its hunks touch) over `./fuku/modules ./fuku/i18n ./fuku/db/blacklists ./fuku/db/filters ./fuku/db/notes ./fuku/db/migrations ./fuku/utils/content`, `-json -race -count=1 -timeout 30m` | CI environment: exit 0, `check_test_results` exit 0, 99 of 100 passed and one allowed PostgreSQL-only skip (`TestBlacklistActionValuesOnPostgres`), `fuku/modules` 101.9 s. Bare (no `cienv.sh`): exit 0, `check_test_results` exit 0, `fuku/modules` 102.5 s |
| `make check-translations` | "All translations are present!" |
| `make generate-docs` then `make check-docs` | "No drift detected"; both frozen pages keep `MANUALLY MAINTAINED` in their first 512 bytes |
| `make lint` | **could not run** (golangci-lint built with Go 1.25 refuses Go 1.26.0), fallback below |
| fallback: `go vet -tags testtools ./...` | exit 0 |
| fallback: `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`, read directly | exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827" |
| fallback: `gofmt -l` over the 52 Go files changed since `d93ec1d` minus `main.go` and `fuku/modules/help.go` | prints nothing |
| `FUKU_TEST_DATABASE=true make test-postgres-integrity` on a throwaway PostgreSQL 16 cluster (127.0.0.1, a free port, trust auth, data directory deleted, built by `TestRepositoryMigrationChain` first as CI's chain step does) | exit 0, 18 packages `ok`, 175 PASS, 0 SKIP; `TestBlacklistActionValuesOnPostgres` PASS; `pg_constraint` on `blacklists` lists only `chk_blacklist_action`; the cluster was stopped and its data directory deleted (no `postgres -D` process left, `/var/lib/postgresql` holds only `16`) |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| path invariant: `git diff --name-only d93ec1d -- migrations fuku/db/models main.go .github/workflows/ci.yml` | exactly `migrations/20261113120000_drop_legacy_blacklist_action_check.sql`; no model, no `main.go`, no CI change |
| `t.Skip` grep over the test files changed since `d93ec1d` | only the two PostgreSQL-only skips of `TestBlacklistActionValuesOnPostgres` (same pattern as its sibling tests in `fuku/db/migrations`); no other added skip |
| reflect.Ptr / ioutil grep over the changed Go files | nothing |

**Not run:** `make lint` and gosec (cannot run in the sandbox; govet's `inline` analyzer is not in the stand-in), `make test` as a single command, the Node captcha page script tests (no page change), every item of `19-SPIKE.md` (live checks).

## Seconds the phase added to the fuku/modules run (D-12 budget about 40 s)

By the plans' own reports: 19-01 2.2, 19-02 2.5, 19-03 16, 19-04 4.5, 19-05 4.5, 19-06 1, 19-07 9, 19-08 4.8, 19-09 9, 19-10 6, 19-11 2.3, a total of **about 62 s, over the budget by about 22 s**. The whole-package run measured 1730 s (19-01) to 1971 s (19-11) and 1897 s here, against the 40 minute per-package limit and the 45 minute CI job ceiling. In this run the 89 Phase 19 top-level tests of `fuku/modules` sum to about 103 s of test time, but that includes two older tests the phase extended (`TestSettingsStaffPrivacy` 24 s and `TestSettingsGateAllDoors` 17 s, which grew by about 2 s and 1.2 s). The overshoot has one cause, recorded in the SPIKE: the fixed 0.3 s of a real settings environment per test (about 60 tests) and about 1 s each for tests that install the real locales; no new test is over 3 s. The plan 19-12 itself added no test.

## Deviations from Plan

- **`make test` was not run as one command** (documented fallback, above). The plan's verify block names `make test`; the fallback is the one its own text allows ("make test or its documented fallback").
- **The AGENTS.md read was whole for Phase 19's regions and by grep elsewhere**, not line by line over all 4,322 lines (see the corrections section). No stale Phase 19 sentence was found by any grep after the four fixes.
- **The SPIKE says "one commit reverts it" with a caveat.** The brief asks for the one commit that reverts each owner-visible change. `git apply -R --check` shows that a plain revert of `12ee6a4`, `3025b0f`, `f0dd7f7`, `4f794f1`, `2eeac8f` and `ed7988a` now conflicts in the test files and in AGENTS.md (later plans added to the same files); the migration commit `76408de` reverts cleanly. The SPIKE names each commit and its small production hunk and says so.
- **The reply form of `/save`.** 19-08's summary says the first word is the name in the reply form, but `ExtractNoteAndFilter` joins all arguments in that form; I did not assert either in the SPIKE (it names only what the tests drive: the quoted form and `/save #x`). Not investigated further, no behaviour change.
- No production file, test or migration changed in this plan.

## Known Stubs

None.

## Threat Flags

None: the plan changed documentation and the requirements ledger only.

## Self-Check: PASSED

- Files exist: `.planning/phases/19-blacklist-filter-and-note-pages/19-SPIKE.md`, `AGENTS.md`, `README.md`, `docs/src/content/docs/api-reference/callbacks.md`, `docs/src/content/docs/api-reference/permissions.md`.
- Commits are ancestors of HEAD: `efc39e4`, `c2cc48c`, `a698358`.
- Acceptance greps: `settingsRightRestrictBotRestrict` and `452` in AGENTS.md (present), `385 \`settings_\` keys` 0, `settingsInputAdvance` 2, `Blacklist page|Filters page|Notes page` in README 3, `\`b\` (Blacklist)` in callbacks.md 1; PAGE-07/08/09 ticked with traceability Complete, PAGE-10 and PLAT-05 still ticked.
