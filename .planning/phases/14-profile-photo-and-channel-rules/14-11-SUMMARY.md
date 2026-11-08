---
phase: 14-profile-photo-and-channel-rules
plan: 11
subsystem: member-requirements
tags: [testing, pacer-independence, log-privacy, alert-size, backup-exclusion, plat-06]

requires:
  - phase: 14-profile-photo-and-channel-rules
    provides: 14-03 (the lookup pacer and flights), 14-04 (the channel lookups), 14-06 (shutdown), 14-08 (I fixed it lookups), 14-09 (the page status block), 14-10 (the channel gate)
provides:
  - TestMemberReqPacersAreIndependent (the lookup and punishment budgets at the job, join, press and page entries)
  - TestMemberReqLookupLogsCarryIDsOnly and TestMemberReqLookupAlertsFitEveryLocale
  - the Phase 14 part of TestSettingsStaffPrivacy (every channel status, a basic group, Main with every rule on, the alerts, a private and a group-menu run)
  - settingsMreqNoLinkAlert, the one builder of the refused channel press's alert
  - AGENTS.md Testing: the Phase 14 families, the lookup toolkit, its hygiene, the locale counts
affects: [14-12, 15, 16, 17]

plan_head_before: da9ba53dc221f1fce169d4199f3a2e4960620de1
plan_head_after: 35b85216cf9e761e19eac6cf01588fc21b6c23cf
actuals:
  tokens: 32000
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "A budget-independence test counts observable effects only: Redis keys written, requests that reach the fake, elapsed time, notices delivered"
    - "A log-privacy test installs a debug-level logrus hook, drives every failure shape with recognisable sentinels in the fake's answers, and also fails when it captured nothing it should have seen"

key-files:
  created:
    - fuku/modules/memberreq_lookup_budget_test.go
  modified:
    - fuku/modules/settings_staff_privacy_test.go
    - fuku/modules/memberreq_lookup_fake_test.go
    - fuku/modules/settings_mreq.go
    - fuku/db/backup/memberreq_backup_test.go
    - AGENTS.md

key-decisions:
  - "The bot credential never reaching a log line is proved with a real gotgbot.BaseBotClient over a failing transport (gotgbot scrubs the URL of a transport error) rather than by changing memberReqErrText: production logging is unchanged"
  - "The refused channel press's alert is built by settingsMreqNoLinkAlert (a behaviour-neutral extraction from the gate) so the alert-size test measures what the gate sends"

requirements-completed: [PLAT-06]

coverage:
  - id: D1
    description: "a 30 s block on the lookup pacer's keys sheds the lookups of the enforcement job, the join entry, the I fixed it press and the page at once with no request and no waiting, delays no notice or removal, and each entry's lookups write the lookup keys; a block on the punishment pacer's keys does not delay a page open and the page never writes them"
    requirement: "PLAT-06"
    verification:
      - kind: unit
        ref: "TestMemberReqPacersAreIndependent"
        status: pass
    human_judgment: false
  - id: D2
    description: "no log line of any lookup path carries a photo file ID, a name, a title, Telegram's description, a notice text or the bot's credential, under every failure shape; a recovered flight panic is one line naming no ID"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupLogsCarryIDsOnly"
        status: pass
    human_judgment: false
  - id: D3
    description: "the refused-press alert and the could-not-check alert are at most 200 code points in all 8 languages and name no ID, Staff Group or member; no Phase 14 screen names the Staff Group, the channel's title or ID or a member in a group menu or the private menu of an administrator who is not the creator"
    verification:
      - kind: unit
        ref: "TestMemberReqLookupAlertsFitEveryLocale, TestSettingsStaffPrivacy"
        status: pass
    human_judgment: false
  - id: D4
    description: "the photo and channel columns never travel in a backup: the export file names no requirement column or table, an import writes no row, a reset leaves both columns"
    verification:
      - kind: unit
        ref: "TestMemberRequirementsStayOutOfBackup"
        status: pass
    human_judgment: false

duration: 150min
completed: 2026-11-08
status: complete
---

# Phase 14 Plan 11: Cross-entry budget, log and screen privacy, alert size and backup exclusion Summary

The whole Phase 14 surface is now proved to keep the lookup and punishment budgets apart at all four entries, to leak nothing in a log line, an alert or a screen (group menu and the private menu of an administrator who is not the creator, in 8 languages), to fit Telegram's alert limit, and to stay out of every backup; AGENTS.md's Testing section describes it. This completes PLAT-06 (cross-cutting, mapped to its first phase like PLAT-05; Phases 15 to 17 inherit it and must keep meeting it).

## Accomplishments

- `TestMemberReqPacersAreIndependent` (`memberreq_lookup_budget_test.go`): for the enforcement job, the join entry, the I fixed it press and the page, a 30 s block on `fuku:memberreq:lkp:pace:block` sheds every lookup as unknown with no request and within 2 s, nobody is punished by a lookup and the press answers `mreq_fixed_check_failed`; unblocked, each entry's lookups write `fuku:memberreq:lkp:pace:next`. In the same minute a name violator is muted (and a last-name kick happens with a channel Ban rule's lookup shed) with its notice delivered at once. A 30 s block on `fuku:memberreq:pace:block` does not delay a page open, which still draws its status, and the page never writes the punishment pacer's `next` key or touches its block.
- `TestMemberReqLookupLogsCarryIDsOnly`: a debug-level logrus hook over the job path and the I fixed it path under every shape of `mreqUnanswerableShapes` (plain error, 400, 403, 429 past the retry, 500, null, shed, timeout, cancelled root, for the photo, getChat, member, the bot's own standing), the held release (500, success, 403 with an inert channel), the page's status reader and the gate under seven failure shapes, pass paths with a recognisable file ID, channel title and names, a recovered flight panic (one line, no ID), and a real `gotgbot.BaseBotClient` over a failing transport and over a server that refuses with a description. Forbidden in any line: the file ID, names, title, description, notice and rule-name markers, and the bot's credential; the test also fails if it did not see "telegram error 400/403/500", "rate limited" and "had an empty answer". A mutation that logged `err.Error()` made it fail.
- `TestMemberReqLookupAlertsFitEveryLocale`: the refused-press alert (built through the new `settingsMreqNoLinkAlert`, the function the gate sends) and `mreq_fixed_check_failed` are at most 200 code points in all 8 languages, name the channel rule, and hold no chat or channel ID, Staff Group marker, member name or unfilled token.
- `TestSettingsStaffPrivacy`: the page and every one of the seven rule screens under unread, ok, notadmin, nolink, unknown and basic, for rules all Off and with every punishment, under a plain draw and a warning notice, in a group view and a private non-creator view; Main with every rule on; both alerts; plus a private and a group-menu Phase 14 run (page, photo screen, Warn and a Mute card on the photo rule, the channel screen, a Warn press, a Mute card and its Yes, a lost bot standing, a lost link with its refused strict presses, an unreadable link in the group menu, both switches, Off) over a Staff Group, a channel whose title and ID the fake knows and a held member whose name and user row exist. None of the leak list appears in any edit, message or answer.
- `TestMemberRequirementsStayOutOfBackup` sets `PhotoAction` and `ChannelAction` in the source row and also asserts the export file names no requirement column or table and a reset leaves both columns.
- AGENTS.md Testing: the Phase 14 families, the lookup toolkit, its hygiene (including never `t.Skip`), the locale counts (247 `settings_`, 29 `mreq_`) and the two PostgreSQL facts; the lookups bullet now states the cost of a channel the bot cannot read (one runner job and one paced lookup per message of every non-exempt member until the owner fixes the standing or turns the rule Off) and what the logs of a lookup path carry.

## Task Commits

1. `e6aaa86` test(14-11): the lookup budget and the punishment budget are independent at every entry (tracer; it passes on the code of 14-03 to 14-10 as the plan expected, 10 flake-safety runs green)
2. `7d95b6a` test(14-11): no log, alert or screen of the phase leaks and every alert fits
3. `6608451` test(14-11): the new columns never travel in a backup
4. `f0fe623` docs(14-11): describe the Phase 14 test families, toolkit and hygiene
5. `35b8521` test(14-11): wait for a cancelled lookup's flight to unwind before asking for a fresh one (see Deviations)

## Verification (real results)

- Task 1: `TestMemberReqPacersAreIndependent` -v: 10 subtests PASS; `-count=10`: ok.
- Task 2: `TestMemberReqLookupLogsCarryIDsOnly|TestMemberReqLookupAlertsFitEveryLocale|TestSettingsStaffPrivacy`: ok (twice).
- Task 3: `TestMemberRequirementsStayOutOfBackup|TestGreetedMembersStayOutOfBackup`: both PASS; `grep -c TestMemberReqPacersAreIndependent AGENTS.md` prints 1; `grep -c mreqLookupCleanup AGENTS.md` at least 1; `grep -c PhotoAction fuku/db/backup/memberreq_backup_test.go` at least 1.
- CI-gate proof, as CI does it: one `go test -tags testtools -json -race -count=1 -timeout 30m -run '^(TestSettings|TestLoadSettings|TestMemberReq|TestMemberRequirement|TestShutdownEndsMemberReqLookups|TestShutdownWaitsForMemberReqJobs|TestChatRequirementsSettings|TestApplyMemberReqAction|TestGreetedMembersStayOutOfBackup)' ./fuku/modules ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n` (the whole settings family, every `TestMemberReq*` test and the Phase 14 families), exit 0, 2321 test passes, no `fail` event, modules 689 s; piped through `go run ./scripts/check_test_results`: exit 0, with exactly one skip, the allowed PostgreSQL-only `TestMemberRequirementLookupMigrationDefaults`. No `t.Skip`, `t.Skipf` or `t.SkipNow` in any test file this plan added or changed (`TestMemberRequirementsStayOutOfBackup` keeps its pre-existing `skipIfNoDb`, which skips only when there is no database).
- The first run of that command failed on `TestShutdownEndsMemberReqLookups` (see Deviations); the run above is the one after the fix.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/db/backup` clean; `gofmt -l` clean on every file touched; lint stand-in `lint-local.py 8c0c827` with its exit read directly: 1 once (errcheck on an unchecked `fmt.Fprintf` in my httptest handler, fixed), then 0 (also after the last code commit); `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0.
- Not run: `make lint` (the installed golangci-lint refuses Go 1.26.0), `make test` (14-12 is the gate plan), `make check-translations` and `make generate-docs` (no locale or doc-source change), `make test-postgres-integrity` (no new PostgreSQL guarantee), a PostgreSQL run (no database guarantee changed; the backup test runs on SQLite, as the plan says).

## Deviations from Plan

**1. [Rule 1 - Bug] A flake in 14-06's `TestShutdownEndsMemberReqLookups`.** The first CI-style family run failed it once ("flight result = {Val:0 Err:<nil> Shared:true}, want a fresh flight of its own"): `StopMemberReqJobs` returns when the job has left its lookup, which can be a moment before the cancelled flight's goroutine has unwound, so on a loaded machine the fresh flight joined it. It passed alone three times and five times after the fix. The test now asks for the fresh flight again for up to 2 s and fails only when the flight stays joined (a flight stuck in the held request still fails). Test-only change in a file outside the plan's list, commit `35b8521`. CI would have hit the same race.

**2. [Rule 2 - small extraction] `settingsMreqNoLinkAlert`** in `settings_mreq.go`: the gate's inline alert text moved into a function the gate and the alert-size test both call, so the test measures exactly what is sent. No behaviour change (the settings family, including `TestSettingsMreqChannelRefusal`, passes unchanged).

**3. Lookup fake extended** (`memberreq_lookup_fake_test.go`): `mreqPhotoAnswer.FileID` and `mreqLookupState.setTitle`, used only by the new leak detectors. A channel's title can never reach the bot (nothing asks for a channel's own record), so the title is a detector for a future call, not a present risk.

**4. The bot credential is proved, not coded.** The brief offered two ways to settle `memberReqErrText` returning `err.Error()` for a non-Telegram error. gotgbot's `BaseBotClient` already replaces the token in a transport error's URL (`sanitizeError`), so production logging is unchanged and `TestMemberReqLookupLogsCarryIDsOnly` proves it with a real client over a failing transport. It would not hold for a client other than gotgbot's, which the bot does not use; AGENTS.md says so.

**5. Spike note.** The 14-07 report asked that the cost of an unreadable channel also reach the SPIKE; there is no Phase 14 spike file in the phase directory, so only AGENTS.md carries it (the runner bullet already had it from 14-07; the lookups bullet now repeats it plainly).

## Requirement bookkeeping

`requirements-completed: [PLAT-06]`. PLAT-06's three promises are shown across the phase: the lookups share one fleet-wide budget separate from punishments and notices (14-03, shown at every entry here), recover from crashes (a flight recovers its own panic, `TestMemberReqLookupLogsCarryIDsOnly`; a lookup cut off leaves nothing, 14-06), and are drained inside the existing shutdown budget with no new drain (`TestShutdownEndsMemberReqLookups`). Phases 15 to 17 inherit PLAT-06 as a cross-cutting constraint: the AI name checks and the deletion work must share a budget of their own, recover and drain the same way. MREQ-10 (ends with "AI not configured", Phase 15's line) and MREQ-01 stay Pending; MREQ-11 and MREQ-12 were ticked by 14-08 and 14-10.

## Known Stubs

None.

## Threat Flags

None: tests, a behaviour-neutral alert extraction and documentation only.

## Self-Check

## Self-Check: PASSED

The created and changed files exist and the five commits are ancestors of HEAD.
