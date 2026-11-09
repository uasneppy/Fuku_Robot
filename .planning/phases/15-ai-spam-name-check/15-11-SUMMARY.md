---
phase: 15-ai-spam-name-check
plan: 11
subsystem: member-requirements
tags: [member-requirements, ai-spam-name, end-to-end, rulebook, docs, phase-gate, requirements]

requires:
  - phase: 15-ai-spam-name-check
    provides: 15-01 to 15-10 (the migration and column, the names request and canonical form, the AI rule, the baseline and marker, the cost guards, the notice and button, the page and its gate, the spike tool, the sweeps)
provides:
  - "TestMemberReqAIEndToEnd: the phase's vertical slice through the real dispatcher, only Telegram and the provider faked"
  - "AGENTS.md read whole against the code and corrected (eight stale or promissory sentences)"
  - "README, callbacks.md, permissions.md, environment.md and the aispam page describe the eight rules, the AI gate, what is sent and what turns the rule off"
  - "15-SPIKE.md read against the finished code (no correction needed)"
  - "MREQ-01, MREQ-10 and MREQ-14 completed in REQUIREMENTS.md (checkbox and traceability row)"
affects: []

plan_head_before: 3e314ddf4552447ae380786cc946e9f4af08b741
plan_head_after: 6da59a97b66acad309a65eeaa7dcedcecb7fa806
actuals:
  tokens: 14100
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "The end-to-end test builds its environment from the Phase 13 to 15 toolkit only (mreqJoinEnv, loadMemberReq, mreqAIMember, mreqPressFixed, settingsMreqOpenPage, mreqE2ESetRule) and adds four small helpers of its own, each called by the test"

key-files:
  created:
    - fuku/modules/memberreq_ai_e2e_test.go
    - .planning/phases/15-ai-spam-name-check/15-11-SUMMARY.md
  modified:
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/environment.md
    - docs/src/content/docs/api-reference/permissions.md
    - docs/src/content/docs/commands/aispam/index.md
    - .planning/phases/15-ai-spam-name-check/15-SPIKE.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "No production file changed and no fix commit was needed: the end-to-end test passed on the code of plans 15-01 to 15-10 at its first run"
  - "commands/settings/index.md is generated from the settings_help_msg locale text, which names the member requirements without counting rules and says they have no command; both stay true, so no locale or generated page was edited"
  - "The aispam page (frozen, manual) gained the names note and the key row because it is where owners read what leaves the server"

requirements-completed: [MREQ-01, MREQ-10, MREQ-14]

coverage:
  - id: D1
    description: "An owner sets the AI rule to Mute from the page (the AI cell, Mute, the card's Yes); a joiner rated 0.97 is muted once with one notice that names the AI phrase, ends with the AI remedy and carries the button; a second delivery of the join punishes nobody again; after a rename the button frees the member with exactly one provider call, the baseline holds the new hash, the marker is gone, the held welcome is sent once and the next message starts no job"
    requirement: "MREQ-14, MREQ-01"
    verification:
      - kind: integration
        ref: "TestMemberReqAIEndToEnd/owner,_joiner,_rename,_button"
        status: pass
    human_judgment: false
  - id: D2
    description: "An established member with no join record is recorded with no call; a rename to a spam-like name is judged once (one warn, one notice ending with the AI remedy); renamed back, a later message releases the hold with one call and no second warn"
    requirement: "MREQ-14"
    verification:
      - kind: integration
        ref: "TestMemberReqAIEndToEnd/established_member:_recorded,_then_judged_on_a_rename"
        status: pass
    human_judgment: false
  - id: D3
    description: "Approved members and administrators are never judged (no request, welcomed, no hold, no restrict)"
    requirement: "MREQ-14"
    verification:
      - kind: integration
        ref: "TestMemberReqAIEndToEnd/approved_members_and_administrators_are_never_judged"
        status: pass
    human_judgment: false
  - id: D4
    description: "With the AI not configured (no key, and ENABLE_AISPAM off) the page lists the AI line, the rule's screen is the Off-only form with the stored Mute visible, Warn, Mute and a forged Yes are refused with the alert and nothing is written, Off is accepted and clears the value, and a joiner with a Mute stored is welcomed with no request and no punishment"
    requirement: "MREQ-10"
    verification:
      - kind: integration
        ref: "TestMemberReqAIEndToEnd/the_AI_not_configured"
        status: pass
    human_judgment: false
  - id: D5
    description: "A 401 at join punishes nobody and keeps the marker; the provider-wide pause makes the next join make no request; after the pause (clock moved past it) a working answer mutes the next joiner; a 500 at join welcomes the joiner and the first message is judged once"
    requirement: "MREQ-10, MREQ-14"
    verification:
      - kind: integration
        ref: "TestMemberReqAIEndToEnd/a_rejected_key_and_the_breaker"
        status: pass
    human_judgment: false
  - id: D6
    description: "The rulebook, docs and README match the finished code; the phase gate passes"
    requirement: "MREQ-14"
    verification:
      - kind: other
        ref: "Phase gate below"
        status: pass
    human_judgment: false

duration: about 2 h 10 min (not timed at start; estimated from the session clock, most of it the two full test runs)
completed: 2026-11-09
status: complete
---

# Phase 15 Plan 11: Close the AI spam-name check Summary

**One end-to-end test proves the AI spam-name rule through the real dispatcher (the owner's page press, a muted joiner who renames and taps the button, an established member recorded then judged on a rename, the AI not configured, a rejected key), AGENTS.md was read whole against the code and corrected, the docs and README describe the eight rules, the phase gate passes, and MREQ-01, MREQ-10 and MREQ-14 are complete.**

## What was done

**Task 1 (tracer, `f035cd3`):** `TestMemberReqAIEndToEnd` in `fuku/modules/memberreq_ai_e2e_test.go`, five subtests. Nothing is faked but Telegram (the hand-written bot fake) and Anthropic (the `httptest` stand-in of plan 15-04); the page, the card, the dispatcher's join and message paths, the executor, the hold and the button are the real handlers.

- `owner, joiner, rename, button`: the settings environment's creator opens the Requirements page, presses the AI cell, Mute and the card's Yes (stored `ai_action` 4, the card wrote nothing, no provider request yet). A member whose names the stand-in rates 0.97 joins (Check on join on): one `restrictChatMember` with the marker, one punished hold with bit 128 that holds the welcome, no welcome, one notice that names the AI phrase and ends with `mreq_remedy_ai`, one provider request, the marker set. The second delivery of the join (service message) adds no restrict, no notice and no request. The fake renames the member to an ordinary string rated 0.05 and the member taps the button: one restrict with the group's permissions, the hold gone, the notice edited to `mreq_notice_fixed`, one toast, exactly one provider request for the new string, the baseline holds the new hash, the marker is gone, the held welcome is sent once, and the next message starts no job.
- `established member`: the AI rule on Warn, an existing member with an ordinary name and no join record writes: no request, a baseline stored, no hold, no notice, and the second message starts no job. A rename to a spam-like name (0.95) is judged once: one warn, one punished AI warn hold, one notice ending with the AI remedy, the marker set. Renamed back and written again (dated 5 s later so the clean predicate's two-second margin holds): the hold is released with one more request and no second warn or notice.
- `approved members and administrators`: both join and write with spam-like names; no request, one welcome each, no hold, no restrict.
- `the AI not configured`: with no key, the page body lists the AI line once and the AI screen is the Off-only form (`[1 2]` rows, the stored Mute visible); Warn is refused with the one alert `settings_mreq_here_ai` plus the rule name and nothing is written; Off is accepted (no alert) and clears the stored value; Mute and a hand-built Yes of its card are refused with the same alert and draw no card; with a Mute stored behind the settings' back, a joiner is welcomed with no request, no hold, no restrict, no marker and no baseline, for both ways of not being configured (no key, then `ENABLE_AISPAM` off with the key set).
- `a rejected key and the breaker`: a 401 at join welcomes the joiner, punishes nobody and keeps the marker (one request); the next join during the pause makes no request; after the test clock passes the 5 minute pause (the clock is restored by `mreqSwap`) a 0.97 answer mutes the next joiner (hold with bit 128); a 500 at join welcomes the joiner and leaves the marker, and the joiner's first message makes one request and holds them.

`-race -count=1 -v`: all five subtests PASS (3.0 s); `-race -count=10`: `ok` (20.6 s, exit 0). It never skips and never calls `t.Parallel`; members come from the earlier toolkits (`mreqAIMember`, `mreqMember`), so IDs are fresh and their cleanups apply. Its four own helpers (`mreqAIE2EEnv`, `mreqAIE2ENoNewJobs`, `mreqAIE2ENewNotice`, `mreqAIE2EWantAINotice`) are each called by the test.

**Task 2 (`a6cef33`):** AGENTS.md was read front to back in the file itself (the copy in the session's project instructions was stale: it still said the shutdown test of the phase was "plan 15-10's"). Every region a Phase 15 plan touched was checked against the code (constants for the budgets 5/100/400, the TTLs, the gate size 4 and wait 3 s, the verdict-key and baseline shapes, the table order, `memberReqAILive`, `memberReqAIConfigured`, the absence of a `/stats` section). Corrections:

1. The watcher bullet's lookup-rule sentence named only the photo and channel rules; it now says the AI rule's part is described after it.
2. The lookups trap said the photo and channel rules answer `memberReqVerdict`; it now says the AI rule answers the same type and has its own bullet.
3. The AI rule bullet ended "Later plans of Phase 15 extend this bullet"; it now points at the executor, `mreq|v1` and page `m` bullets where the notice, button and page status live.
4. The executor bullet said the lookups run "photo then channel"; it now says photo, channel, then the AI rule.
5. The join bullet said the 4 s join budget covers "a rule of the photo or channel kind"; it now says photo, channel or AI (the budget test `TestMemberReqAIJoinBudget` proves it).
6. The image checks bullet said only the image goes to Anthropic, "never the names"; it now says the separate AI spam-name rule sends the three names and nothing else.
7. "Defaults that surprise" said a key exported for another tool turns the image checks on; it now says the AI name check too, wherever the rule is already set to a punishment.
8. The Testing section said `TestChatRequirementsSettings` "covers seven columns since Phase 14" and, a line later, "now covers eight"; it now says eight in all. "The closing plan adds `TestMemberReqAIEndToEnd`" became a description of what the test proves.

Checked and found already true, no edit: the drain bullet (the AI call derives from `memberReqContext()`, Redis and Anthropic only, no drain, 65 s worst case unchanged, `TestShutdownEndsMemberReqAI`), the watcher's three single-key GETs, the `mreq|v1` step 5 with the AI row and D-23, the page `m` bullet (eight cells, six rows, the AI line, the Off-only form, `settingsMreqAIGate`), the Permissions bullets (the rights and the gate after them, `ENABLE_AISPAM` also switching the name rule off), the Data keys `nv`, `nm`, `nu`, `aib:u|c|g` and the eight columns, the rule-table, AI-rule and executor Subsystem traps (sticky unknown, stale-names guard, notice tails), the Go rules (claude, namecanon), the locale counts (255 `settings_`, 31 `mreq_`, verified against `locales/en.yml`), and the "eight rule screens and twenty-four cards" privacy sentence (`grep 'twenty-one cards'` prints 0). No "seven rules", "seven cells" or "five or more of up to seven" remains.

README: the Member requirements paragraph says eight rules, adds the AI rule (names only, to Anthropic's Claude, never messages; new members at join or first message, any member after a rename, members already here only after one; off with `ENABLE_AISPAM=false` or no key and the page says so; Warn or Mute recommended; an unanswerable check punishes nobody), and the environment table gained `ANTHROPIC_API_KEY` and `ANTHROPIC_MODEL` rows. `callbacks.md`: page `m` code `ai`, the eight cells, the AI status line, the Off-only screen, the AI gate and the AI card consequence, and `mreq` step 5 with the AI row (cache-only, deterministic for unchanged names, count answer up to eight). `permissions.md`: the AI rule in the open and choice rows, a new row for a punishing AI value (the server must be configured), and the "I fixed it" row now says how a member held for a spam-like name gets out. `environment.md`: `ENABLE_AISPAM` also switches the name check off, `ANTHROPIC_API_KEY` also enables it (what is sent, when, the cache and the budgets), `ANTHROPIC_MODEL` serves both checks and must accept a forced tool call, and the optional-variable listing. The frozen aispam page gained a "Names are a separate check" bullet and the key row. The three api-reference pages keep their MANUALLY MAINTAINED marker.

**Task 3 (`e349176`, `6da59a9`):** `15-SPIKE.md` was read against the finished code: the keys, the meaning of "new member", the budgets (5, 100, 400), the 5 minute pauses, the notice and confirmation texts (`mreq_remedy_ai`, `settings_mreq_confirm_ai_mute`, `settings_mreq_here_ai`) and the model rule all match; it still has 11 numbered checks. It now names the end-to-end test as the automatic twin of check 6. After the gate passed, `requirements.mark-complete MREQ-01 MREQ-10 MREQ-14` ticked exactly those three (checkbox and traceability row; the tool changed nothing else) and the two rows carry a note that Phases 13, 14 and 15 (MREQ-01) and 14 and 15 (MREQ-10) completed them. PLAT-06 stays ticked, unchanged.

## Phase gate (real results)

| Gate | Result |
|------|--------|
| `make test`, as the Makefile defines it | ran to completion in the background (`maketest.txt` in the session scratchpad): 68 packages `ok`, `fuku/modules` `ok` in 1079 s (76.0% coverage), no FAIL, no panic; the target exited non-zero only for `go: no such tool "covdata"` on `fuku/utils/actionlog` (a package with no test files: a `build-fail` event), the same sandbox case as 12-16 and 14-12 |
| `make test` fallback | the same flags (`-json -race -coverprofile -coverpkg -count=1`), every package except `fuku/modules` with `-timeout 30m`, then `./fuku/modules` alone with `-timeout 40m`; the three covdata events of `fuku/utils/actionlog` (two `build-output` lines and the `build-fail`; they carry `ImportPath`, not `Package`, so a first filter on `Package` removed none and the check failed, then the filter was fixed and the saved JSON re-fed without re-running the tests) removed; merged and fed to `go run ./scripts/check_test_results`: **exit 0**, no failure, no unexpected skip (the skips are the PostgreSQL-only tests, `TestUpdateChat` and `TestStaffExclusivityTrigger`, which the gate allows); `fuku/modules` `ok` in 1094 s, 77.2% coverage. CI's own `make test` stays the authority |
| The new families through CI's gate | `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestMemberReqAI\|TestMemberReqWatcherAI\|TestMemberReqPassCacheWithAI\|TestMemberReqHeldReleaseAI\|TestMemberReqNotice...\|TestMemberReqFixedAI\|TestSettingsMreq\|TestShutdownEndsMemberReq\|TestMemberReqLookupLogsCarryIDsOnly\|...\|TestClassifyNames\|TestCanon\|TestNames\|TestRunNamesMode)' ./fuku/modules ./fuku/i18n ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/utils/claude ./fuku/utils/namecanon ./scripts/claude-spike` (the full pattern is every Phase 15 name of `15-VALIDATION.md` plus the Phase 13 and 14 page, card, locale and privacy tests that Phase 15 edited): exit 0, then `go run ./scripts/check_test_results < fam.json` **exit 0**; 90 top-level tests PASS, 1 skip, `TestMemberRequirementAIMigrationDefaults` (PostgreSQL only, allowed), all eight packages pass |
| `make check-translations` | exit 0, "All translations are present"; no file changed |
| `make generate-docs` then `make check-docs` | exit 0, "No drift detected", run after the docs edits and before their commit |
| `make lint` | could not run: the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0. Fallback: `go vet -tags testtools ./...` exit 0; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0, read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827", staticcheck and errcheck included); `gofmt -l` over every Go file changed since 8c0c827 prints only `main.go`, which is not formatted at 8c0c827 either (checked with `git show 8c0c827:main.go \| gofmt -l`) |
| gosec v2.29.0 (the CI version, `$(go env GOPATH)/bin/gosec -quiet -no-fail -fmt json ./...`) | 17 issues in the whole repository; the only one in a file changed since 8c0c827 that is not a test file is `fuku/modules/warns.go:36 G115` (baseline, CI has always passed it); the other 16 are in files this phase never touched. No G101, G404 or discarded-error finding in code Phase 15 added |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `make -n test-postgres-integrity` | lists `TestMemberRequirementAIMigrationDefaults` once and `TestChatRequirementsSettings` once, with `./fuku/db/migrations` and `./fuku/db/memberreq` |
| PostgreSQL | not run here: nothing in this plan rests on it (15-01 proved the migration on a throwaway cluster; the module harness does not run on PostgreSQL) |
| `node --test scripts/captcha-page/*.test.mjs` | not run (the Turnstile page is untouched) |
| the live Haiku spike and the test-group walk-through | not run: they need the owner's key and a live group (`15-SPIKE.md`, non-blocking) |

## Requirement coverage

**MREQ-14, MREQ-10 and MREQ-01 are complete, and each text is true as written.** MREQ-01: the page offers all eight rules (last name, username, photo, channel, AI, Russian, Arabic, Chinese), each with its own off, warn, kick, ban or mute, the owner or an admin the owner let in presses them, and every rule is Off by default (`TestSettingsMreqCard`, `TestSettingsMreqRuleScreen`, `TestMemberReqAIEndToEnd`; a group with no row reads every rule Off). MREQ-10: a check that cannot be answered punishes nobody for the Telegram, lookup and channel cases (Phase 14) and for the AI cases (an unknown, a paused breaker, a spent budget, a 401, a 500, no Redis: `TestMemberReqAIUnanswerable`, the end-to-end test), and the page shows "AI not configured" (`TestSettingsMreqAIStatus`, the end-to-end test) beside the no-linked-channel and bot-not-admin lines. MREQ-14: only the three canonical names go to Claude (`TestMemberReqAIRequestIsMinimal`), on join or first message and on a name change, never message text; an established member's names are first recorded with no call (`TestMemberReqAIBaseline`, the end-to-end test); verdicts are cached for 24 hours; hourly budgets of 5, 100 and 400 cap the cost; `ENABLE_AISPAM=false` or no key turns the rule off (`TestMemberReqAINotConfigured`, the end-to-end test); no verdict means no punishment. The wording "Claude Haiku" is the default model; `ANTHROPIC_MODEL` can name another Haiku-class model, and a model that cannot take a forced tool call pauses the checks and punishes nobody. PLAT-06 (cross-cutting) was already ticked and is unchanged. `requirements-completed: [MREQ-01, MREQ-10, MREQ-14]`.

## Deviations from Plan

1. **[Judgement] `commands/settings/index.md` was not edited.** It is generated from `settings_help_msg` in the locale files; the sentence it carries ("member requirements ... have no command") is still true and carries no rule count, and a locale edit would have needed eight files and the one-message length test for no gain. The aispam page, which is manual and where owners read what leaves the server, got the names note instead (not in the plan's file list).
2. **[Judgement] The fallback's first filter keyed on `Package`.** The covdata build events carry `ImportPath`; the first merge therefore removed nothing and `check_test_results` failed on the build-fail. The filter was corrected and the saved JSON fed again with no test re-run; the numbers above are from that second feed.
3. **[Judgement] The families gate uses prefix patterns.** Three names in `15-VALIDATION.md` do not exist because later plans renamed or folded them: `TestMemberReqAIRequestCarriesOnlyNames` is `TestMemberReqAIRequestIsMinimal`, `TestMemberReqAILogsCarryIDsOnly` is the AI shapes inside `TestMemberReqLookupLogsCarryIDsOnly`, and `TestSettingsMreqAIUnconfigured` is `TestSettingsMreqAIStatus` and `TestSettingsMreqAIGate`. Prefix patterns (`TestMemberReqAI`, `TestSettingsMreq`) run all of them.
4. No `fix(15-11)` commit: the end-to-end test passed on the first run with no production change.

## Concerns recorded for the owner (nothing here blocks the phase)

- **The name check's counters are dead in production.** `memberReqAIStatsSnapshot` (checked, passed, breached, cached, capped, shed, failed, blocked) is read only by tests; no `/stats` section, log line or metric shows it, as AGENTS.md says ("there is no `/stats` section for them"). The owner sees what the name check costs only in Anthropic's dashboard. A `/stats` section like `raidAIStatsSection` would be one small follow-up.
- **`15-VALIDATION.md` names three tests that no longer exist** (see deviation 3). The file was not edited (it is a planning record), but a later reader should map them as above.
- **The watcher gap and the free pass** are recorded and accepted (AGENTS.md, `15-SPIKE.md`): a baseline-equal member with no marker stays satisfied in the watcher although another account's judgement of the same string cached a breach, and with no Redis or a missed join a real new member is baselined unjudged until they rename. Both err toward not punishing.
- **`gosec` baseline.** 16 issues remain in code this phase did not touch (G101 on identifiers in `staff_action*.go`, `staff_unlink.go` and `captchaweb/limiter.go`, G115 in `settings_staff.go`, `settings_lockdown.go` and `warns.go`, G404 in `staff_sweeper.go`); CI has passed them, so they are listed, not fixed.
- **Live behaviour no test proves** is unchanged from `15-SPIKE.md`: the model's accuracy on real names, its latency and cost, its stability between calls, a rejected key and a rate limit live, and the wording of ten keys in seven languages.

## Known Stubs

None.

## Threat Flags

None. This plan added one test file, documentation and requirement ticks; no endpoint, auth path, file access or schema. T-15-31 (the rulebook or docs claiming what the code does not do) is mitigated by the whole-file read, the stale-count greps and the end-to-end test; T-15-32 (a tick before its proof) by ticking only after the gate, with one tool call of exactly three IDs read back; T-15-SC: `go.mod` and `go.sum` are unchanged.

## Self-Check: PASSED

`fuku/modules/memberreq_ai_e2e_test.go` and this SUMMARY exist, and the commits `f035cd3`, `a6cef33`, `e349176` and `6da59a9` are ancestors of HEAD.
