---
phase: 18-remaining-settings-pages
plan: 15
subsystem: settings-menu
tags: [docs, agents-md, readme, spike, phase-gate, requirements]

requires:
  - phase: 18-remaining-settings-pages
    provides: plans 18-01 to 18-14 (the nine apply functions and six readers, the six pages Warnings, Locks, Switches, Language, Log channel and Anonymous admins, the two composite rights, the 55 strings, the Main grid at 15 cells, the cross-page sweeps and the end-to-end proof)
provides:
  - AGENTS.md checked against the code of plans 18-01 to 18-14 and corrected where a Phase 18 change left a sentence imprecise
  - README, callbacks.md and permissions.md checked against the code (no change needed)
  - 18-SPIKE.md, the owner's 25 live checks and open confirmations (none blocks the phase, no result recorded)
  - the phase gate run with real results, and PAGE-01 to PAGE-06 and PAGE-10 completed
affects: [phase-18-verification, phase-19-pages]

plan_head_before: 5c7e68aaf5b430e6460f7a14051f1bd3b75bbf60
plan_head_after: 58aa6431ad0683e9fa77dd764a40105968e54226

actuals:
  tokens: 6400    # chars/4 over the added lines of 5c7e68a..58aa643 (25499 chars, all documentation)
  tasks: 2
  commits: 3      # MEASURED: git rev-list --count 5c7e68a..58aa643, before this SUMMARY commit

tech-stack:
  added: []
  patterns:
    - "the closing plan checks every identifier, test name, number and order the Phase 18 AGENTS.md bullets state against the code by script (217 backticked identifiers, 76 test names, page orders and icons, the 22-row lock table, the key counts), instead of trusting each plan's own lines"

key-files:
  created: []
  modified:
    - AGENTS.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "No production or test code changed: the gate passed against the code of plan 18-14, so no fix(18-15) commit was needed."
  - "README, callbacks.md and permissions.md were already consistent with the code (keys, controls, v, kg, the lock codes, the largest buttons, the rights, the notice texts) and are unchanged; make generate-docs and make check-docs pass."
  - "The stale /settings help text (settings_help_msg, copied into docs/src/content/docs/commands/settings/index.md) is NOT edited: it is a text change in 8 locale files under a one-message ceiling, outside a plan that changes no production code; it is recorded as 18-SPIKE.md check 20 (A18-A15, A16-A11 stay open)."

patterns-established: []

requirements-completed: [PAGE-01, PAGE-02, PAGE-03, PAGE-04, PAGE-05, PAGE-06, PAGE-10]
status: complete
---

# Phase 18 Plan 15: Docs, live checks and gate Summary

**The rulebook, README and docs describe the six new settings pages as built, the owner has a 25-point live-check list with the open confirmations A18-1 to A18-9, the phase gate passed (with the documented covdata fallback and a throwaway PostgreSQL for the migration package), and PAGE-01 to PAGE-06 and PAGE-10 are complete.**

## What changed

### Task 1: the rulebook, README and docs (commit `8ef9530`)

AGENTS.md was checked against the code of plans 18-01 to 18-14. How the file was read, stated plainly: lines 1 to 1490 (Startup, Handlers, Callbacks, Permissions and the start of Data) were read directly, and every hunk Phase 18 added or changed anywhere in the file (`git diff 29aca33 HEAD -- AGENTS.md`, 12 hunks: the Commands line, the Main bullet, the six page bullets, the Locks bullet, the rights bullets, the "One setting, one code path" bullet, the goldens sentence, the Main-shape constants, the locale count and the Phase 18 test families) was read in full with its surrounding bullets. The unchanged Phase 10 to 17 sections between line 1490 and line 3150 and in the Go rules and Toolchain sections were not re-read line by line in this plan: they were closed by plan 17-12, and they were searched for Phase 18 effects (`anonadmin`, `locks_map`, `warn_settings`, `/reset warnings`, `/aispam`, the stale-phrase greps below) with nothing found.

Checked against the code by script and found true:

| Claim in AGENTS.md | Found in the code | State |
|---|---|---|
| 217 backticked identifiers in the Phase 18 bullets (apply functions, readers, seams, helpers, keys) | each name occurs in `fuku/` or `main.go` | true |
| 76 named tests in the Phase 18 bullets (the page families, the goldens, `TestSettingsPhase18EndToEnd`, `TestSettingsRights`) | each `func Test...` exists in a test file (the 13 wildcard prefixes are prefixes) | true |
| page keys, `Order` and icons: `n` 42 🚩, `k` 44 🔐, `t` 86 🔘, `u` 87 🌐, `o` 88 🧾, `y` 89 🎭 | `registerSettingsPage` calls in the six files; none sets `Hidden` or `OwnerOnly` | true |
| the 22 locks, 12 and 10 a page, two-letter codes, the pager `kg` | `settingsLockRows` (22 rows, codes `sk gf ... al`), `settingsActLocksPage = "kg"` | true |
| eight rights, the two composite ones appended after `settingsRightChangeInfo`, refusal keys | `settings_menu.go` (the only framework edit; `settings_pages.go` is byte-identical to `29aca33`) | true |
| 385 `settings_` keys, 31 `mreq_` keys (330 before, 55 added) | `grep -c '^settings_'` over `locales/en.yml` and `uk.yml` prints 385, `mreq_` 31 | true |
| Main: 15 grid cells, ten rows for the owner, nine for an administrator, 16 state lines | `settingsMainShapeGroup`/`Private` and `settingsMainOwnerRows = 10` in `settings_main_test.go` | true |
| `/reset warnings` stores an empty mode | `backup.go:1072` writes `WarnSettings{ChatId: chatID, WarnLimit: 3}` | true |
| none of the Phase 18 cache keys is in `skipLocal`; reports and `admin` are uncached | `fuku/db/cache/local.go` lists only captcha pending, staff, raid detection, member requirements and delete rules; `greetings` reader is a plain query | true |

Sentences a Phase 18 plan had left imprecise or missing, corrected in `AGENTS.md`:

1. Main bullet: "over the content-page budget by two rows for the owner and one for an administrator" is true for the private menu only; Main has the same ten and nine rows in a group menu, whose budget is nine, so the sentence now says two and one in a private menu and one and none in a group menu.
2. `settings|v1` namespace bullet: the Locks pager `kg` (field `v`, the absolute page) is now named where the namespace's fields and page keys are listed (it was only in the Locks bullet).
3. Rights bullet: "eight rights in all" (the six older plus the two composite ones).
4. "One setting, one code path": a closing paragraph lists the nine Phase 18 apply functions and the six new non-creating, error-returning readers (and the two existing readers the Switches and Locks pages use), says each command keeps its gates and replies, restates that the goldens are never re-recorded, and lists the cache keys with the accepted `CACHE_LOCAL_TTL` skew (`warn_settings`, `ai_spam_settings`, `greetings`, `chat_lang`, `chat_settings`, `chat`, `log_channel`, `locks_map`; reports and the `admin` row are uncached).

Stale-phrase greps, all clear: the old count sentence (`330 `settings_` keys`), "Main lists the pages after Antiflood (`s|l / c|f / w|g` ..." (the final grid is in the sentence now), "nine cells", "seven rows", "six rights", "no registered page". README (six paragraphs, one per page, none repeating another), `callbacks.md` (the six page sections: codes, `v`, `kg`, largest buttons, notice texts that exist in `locales/en.yml`) and `permissions.md` (eight rows of the "Settings menu" table, including the Anonymous admins viewing looseness) were checked against the code and needed no edit. Both frozen pages keep their `MANUALLY MAINTAINED` marker; `make generate-docs` left the tree clean and `make check-docs` printed "No drift detected".

One finding, not fixed here (see deviation 3): `settings_help_msg` and the generated `docs/src/content/docs/commands/settings/index.md` still say the menu covers the Staff Group link, lockdown, captcha, antiflood, member requirements and the welcome, goodbye and rules messages. They name neither the Deletion page (Phase 16 and 17) nor the six Phase 18 pages.

### Task 2: 18-SPIKE.md, the gate, the requirement ticks (commits `f8233da`, `58aa643`)

`18-SPIKE.md` is completed in the format of `17-SPIKE.md`: an opening that says nothing blocks the phase, that the code is safe under every outcome and that the file records no result; "Before you rely on it in a real group" (the pages change the same settings as the commands, a Warnings change applies at the next warn, anonymous-admin mode lowers a check, `all` and `messages` act at once, the language change is for every member, the bot must be an administrator); 25 numbered checks renumbered once against the finished code (warn limit and actions, a stored `tban`, the `/reset warnings` follow-up, the AI line in each configuration and a stale On press, Auto-approve's first press on PostgreSQL, reports with `/report`, the language redraw on two replicas and a private menu, the log categories and the `#SETTINGS` line with a real channel, no channel title or ID, the anonymous-admin flow, the non-owner and demoted-bot refusals and the page opening while the bot is not an administrator, the broad locks, the lock kinds that need a live message, the Locks right line, phone widths for the 22 lock cells, the eight language cells and the six Main cells in hi, ru, uk and id, the flag width, Main at ten rows, the native read of the 55 strings, the stale `/settings` help text); the owner confirmations A18-1 to A18-9 as amended; the flagged UI points A18-A1, A18-A3, A18-A4, A18-A7, A18-A8, A18-A9, A18-A10, A18-A12 and A18-A13; the four deviation notes (18-01, 18-04, 18-06, 18-11) and the planning-level choices; the accepted limits; and the CI check. The acceptance greps: 25 `### N.` headings (at least 12 required), the owner confirmations present.

## Gate results (every command that ran, and every one that could not)

| Gate | Result |
|---|---|
| `make test` exactly as the Makefile defines it, CI environment (`source cienv.sh`), background, output in the scratchpad | ran 03:14:22 to 03:44:35 (30 minutes). 68 packages `ok`, `fuku/modules` `ok 1731.7 s` (limit 2400 s), `fuku/i18n` `ok`, coverage 78.0%. Exit 2 for two environment reasons, neither a test of this phase: (a) `go: no such tool "covdata"` for `fuku/utils/actionlog` (no test files), the documented sandbox case; (b) ten tests of `fuku/db/migrations` failed with `sql: database is closed`, because `cienv.sh` sets `DATABASE_URL` to a PostgreSQL on `localhost:5432` that does not run in this sandbox (CI has a PostgreSQL service); the package passes bare and passes against a real PostgreSQL (next two rows) |
| the documented fallback (12-16, 16-09, 17-12): `-json -race -coverprofile -coverpkg` over every package but `fuku/modules`, with a throwaway PostgreSQL 16 on `127.0.0.1:5432` (trust auth, data directory under `/var/lib/postgresql`, created for the run and deleted, no password or DSN written anywhere), after `TestRepositoryMigrationChain` ran against it as CI's chain step does | the three `actionlog` covdata events (two `build-output`, one `build-fail`) removed, the stream fed to `go run ./scripts/check_test_results`: exit 0, 68 packages `ok` including `fuku/db/migrations`, the only skips are the nine allowed PostgreSQL-only ones (`TestUpdateChat`, `TestCaptchaModeTurnstileConstraint`, `TestMemberRequirementAIMigrationDefaults`, `TestMemberRequirementLookupMigrationDefaults`, `TestRaidDetectionCarryOver`, `TestRepositoryMigrationChain`, `TestRulesMediaMigrationDefaults`, `TestScheduledDeletionMigrationDefaults`, `TestStaffExclusivityTrigger`). `fuku/modules` is the full run's `ok 1731.7 s` on the same code (only documentation commits came after) |
| every Phase 18 test name in one `-run '^(...)$'` (111 names: every top-level test of the 35 test files this phase added, plus the 13 older tests it changed or extended: `TestSettingsMainGrid`, `TestSettingsLabelLengths`, `TestSettingsStaffPrivacy`, `TestSettingsControlActionAllowList`, `TestSettingsRights`, `TestSettingsRightRefusalForParity`, `TestSettingsGateAllDoors`, `TestSettingsHelpFitsOneMessage` and the four Main-shape tests) over `./fuku/modules ./fuku/i18n ./fuku/db/warns ./fuku/db/reports ./fuku/db/aispam ./fuku/db/admin ./fuku/db/logchannels ./fuku/db/lang`, `-json -race -count=1 -timeout 40m`, CI environment | exit 0 in 3 min 19 s; `scripts/check_test_results` exit 0, no unexpected skip; all 8 packages `ok` (`fuku/modules` 197 s); 111 of 111 names ran and passed |
| `make test-postgres-integrity` (extra; CI runs it; the phase adds no guarantee to it) with the throwaway PostgreSQL and `FUKU_TEST_DATABASE=true` | exit 0, 18 packages `ok` |
| `make check-translations` | exit 0, "All translations are present" |
| `make generate-docs` then `make check-docs` | "No drift detected"; both frozen pages keep their marker (`head -c 512 ... | grep -c 'MANUALLY MAINTAINED'` prints 1 for each) |
| `make lint` | could not run (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0). Fallback: `go vet -tags testtools ./...` exit 0; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 read directly (output to a file): "no dupl, gocyclo or godox findings on lines added since 8c0c827"; `gofmt -l` over the 66 Go files changed since `29aca33` minus `main.go` and `fuku/modules/help.go` prints nothing (`fuku/modules/blacklists_command_test.go`, the pre-existing unformatted baseline file, is not in the changed set and was not formatted). gosec and the CI-only govet `inline` analyzer cannot run here; a grep over the added Go lines for `reflect.Ptr`, `ioutil.`, `t.Skip` and `//nolint` finds none |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `git diff --exit-code 29aca33 -- migrations fuku/db/models fuku/modules/settings_pages.go main.go` | exit 0 (no migration, model, framework page change, handler group or drain) |
| `git diff --exit-code bc424c4 -- Makefile .github/workflows/ci.yml` | exit 0 (see deviation 1: against `29aca33` the Makefile differs by the one orchestrator line) |
| no added `t.Skip` | no added `t.Skip` line in any Go file since `29aca33` |
| `node --test scripts/captcha-page/*.test.mjs` | not run (the page was not touched) |
| live checks of `18-SPIKE.md` | not run: they need a live Telegram client, are the owner's, and block nothing |

## Requirement coverage

**PAGE-01, PAGE-02, PAGE-03, PAGE-04, PAGE-05, PAGE-06 and PAGE-10 are complete**, ticked (checkbox and traceability row) by one call of `requirements.mark-complete` with exactly those seven IDs after the end-to-end proof of plan 18-14 and this plan's gate passed, then read back: the diff of `.planning/REQUIREMENTS.md` is exactly the seven checkboxes and the seven table rows (14 lines). PAGE-01 (warn limit and action stored as the warn commands store them: 18-01, 18-03, 18-14), PAGE-02 (reports, the AI spam filter and auto-approve from one page: 18-04 to 18-06, 18-14), PAGE-03 (the group's language set and the menu redrawn in it: 18-07, 18-14), PAGE-04 (the log channel shown, each category switched, linking left to `/setlog`: 18-08, 18-09, 18-14), PAGE-05 (anonymous-admin mode with `/anonadmin`'s rights: 18-04, 18-10, 18-14), PAGE-06 (each lock, paged: 18-11, 18-12, 18-14) and PAGE-10 (every setting stored, cached and refused exactly as by its command: the goldens, parity, drift and authority tests of every page plan, 18-13 and 18-14). PLAT-05 stays ticked (re-confirmed by 18-02, 18-11's keys test and 18-13's privacy run); PAGE-07 to PAGE-09 stay open for Phase 19. No earlier plan ticked anything.

## Deviations from Plan

### Orchestrator change outside the plan

**1. The Makefile and the AGENTS.md "Commands" line changed in commit `bc424c4`** (made by the orchestrator, not this plan): `make test` now uses `-timeout 40m` per package instead of `30m`, because CI measured the `fuku/modules` binary at 1129 s to 1760 s against the old 1800 s limit (this plan's own run: 1731.7 s). The plan's invariant `git diff --exit-code 29aca33 -- ... Makefile ...` would therefore show that one line; the gate ran `git diff --exit-code bc424c4 -- Makefile .github/workflows/ci.yml` instead (exit 0) and kept the `29aca33` check for `migrations`, `fuku/db/models`, `fuku/modules/settings_pages.go` and `main.go`.

### Auto-fixed Issues

None. No production or test code changed, so there is no `fix(18-15)` commit.

### Plan text and process notes

**2. [Gate] `make test` could not report success by itself**, for the two reasons in the gate table: the documented `covdata` case, and ten `fuku/db/migrations` tests that need a PostgreSQL the `cienv.sh` `DATABASE_URL` names. The fallback ran against a throwaway PostgreSQL and passed; the cluster was stopped and its data directory deleted.

**3. [Finding, not fixed] The `/settings` help text and its generated docs page are stale** (named neither the Deletion page nor any Phase 18 page). The plan says to give any other docs page that describes the settings menu the same facts; the page is generated from `locales/en.yml`'s `settings_help_msg`, so fixing it means a text change in all 8 locale files under `TestSettingsHelpFitsOneMessage`, i.e. production data, which this plan does not change. The planner already left `settings_help_msg` unchanged on purpose (A16-A11 and A18-A15). It is `18-SPIKE.md` check 20 for the owner's decision.

**4. [Process] The whole of AGENTS.md was not re-read line by line** (see Task 1): the Phase 18 hunks and everything they touch were read in full and checked by script; the unchanged older sections were searched, not re-read.

**Total deviations:** 0 auto-fixed, 1 orchestrator change, 3 notes. **Impact:** none on the contract.

## Known Stubs

None (documentation only).

## Threat Flags

None (no code or surface added).

## Issues Encountered

- `make test` exited 2 for environment reasons (above); resolved by the documented fallback plus a throwaway PostgreSQL, not by changing any code or test.
- A stray file `/warn-got.txt` in the filesystem root (from 18-01) belongs to no repository path and was not touched.

## Next Phase Readiness

All 15 plans of Phase 18 are executed and the phase gate passed. The phase stays In Progress in ROADMAP.md and `18-VERIFICATION.md` stays `human_needed` like Phases 10 to 17 (live checks deferred, `18-SPIKE.md`); the next steps are the phase closing steps (security, verification). `phase.complete` was not run.

## Self-Check: PASSED

- `18-SPIKE.md`, `AGENTS.md`, `REQUIREMENTS.md` exist and carry the commits `8ef9530`, `f8233da` and `58aa643`, all ancestors of HEAD (`git merge-base --is-ancestor`).
- `grep -c 'settingsRightOwnerBotAdmin' AGENTS.md` prints 3 and `grep -c '385' AGENTS.md` prints 1; `grep -c '330 `settings_` keys' AGENTS.md` prints 0; the README grep prints 6; `grep -cE '^### [0-9]+\.' 18-SPIKE.md` prints 25.
- The seven requirement lines read `[x]` and `Complete`; `PLAT-05` is `[x]`; `PAGE-07`, `PAGE-08` and `PAGE-09` are `[ ]`.
