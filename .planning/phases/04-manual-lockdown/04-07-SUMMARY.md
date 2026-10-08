---
phase: 04-manual-lockdown
plan: 07
subsystem: lockdown
tags: [telegram, lockdown, unmute, permissions, captcha, staff-actions, gotgbot, i18n]

requires:
  - phase: 04-manual-lockdown
    provides: "plans 04-01..04-06: the lockdown repository (GetActiveFresh, BeginLift), the raw pre_permissions snapshot stored at /lockdown, the lift, the lockdown test fake (newLockdownEnv)"
provides:
  - "resolveUnmutePermissions(chatInfo) (gotgbot.ChatPermissions, error): the one choke point of every unmute; during an active lockdown it returns the lockdown's stored pre-lockdown permissions instead of the group's live (locked) defaults"
  - "lockdownSnapshotLookup test seam (defaults to lockdown.GetActiveFresh)"
  - "All four unmute paths (/unmute, unrestrict button, captcha pass, staff /unmute and so the undo of a staff mute) take the error and never restrict or announce an unmute when the lockdown lookup fails"
  - "/unmute reply adds mutes_unmute_lockdown_note during a lockdown (all 7 locales)"
affects: [04-08, live-Telegram UAT, Phase 6 (retires antiraid)]

actuals:
  tokens: 7300
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "A test seam for a repository read (package variable defaulting to the real function) lets tests force a lookup error without a mock library"
    - "A guard on chatInfo.Id != 0 keeps the existing parallel pure tests (Id 0) away from the database"

key-files:
  created:
    - alita/modules/lockdown_unmute_test.go
  modified:
    - alita/modules/chat_permissions.go
    - alita/modules/chat_permissions_test.go
    - alita/modules/mute.go
    - alita/modules/bans.go
    - alita/modules/captcha.go
    - alita/modules/staff_action_run.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - AGENTS.md

key-decisions:
  - "Only an active lockdown row counts: once a lift starts the restore has already put the snapshot back, so the live defaults are right again (a lifting row returns the live permissions)"
  - "An unreadable stored snapshot (decode error) is returned as an error, so the unmute fails closed instead of guessing a permission set"
  - "The /unmute reply reads the lockdown a second time for the note; a failed read there only skips the note, because the unmute itself already went through"
  - "Per-user restrict calls keep their current form (no independent-permissions flag), as the plan decided"

patterns-established:
  - "Every unmute goes through resolveUnmutePermissions; none may announce an unmute when it returns an error"

requirements-completed: [LOCK-08]

coverage:
  - id: D1
    description: "/unmute during a lockdown restricts the user with the lockdown's stored pre-lockdown permissions (not the locked set), the reply says they can talk once the lockdown lifts, and after /unlockdown the group default is the snapshot again and the user's own record allows sending"
    requirement: "LOCK-08"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_unmute_test.go#TestUnmuteDuringLockdownUsesSnapshot/unmute_command"
        status: pass
    human_judgment: false
  - id: D2
    description: "The unrestrict button's Unmute, a captcha pass and staff /unmute during a lockdown all send the stored pre-lockdown permissions"
    requirement: "LOCK-08"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_unmute_test.go#TestUnmuteDuringLockdownUsesSnapshot"
        status: pass
    human_judgment: false
  - id: D3
    description: "Without a lockdown every unmute path behaves as before: the live group defaults, no lockdown note in the reply"
    requirement: "LOCK-08"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_unmute_test.go#TestUnmuteDuringLockdownUsesSnapshot/no_lockdown"
        status: pass
      - kind: unit
        ref: "alita/modules/chat_permissions_test.go#TestResolveUnmutePermissions"
        status: pass
    human_judgment: false
  - id: D4
    description: "When the lockdown lookup fails no unmute is sent and none is announced: /unmute and the button send nothing, a captcha pass schedules its retry, staff /unmute fails the group"
    requirement: "LOCK-08"
    verification:
      - kind: integration
        ref: "alita/modules/lockdown_unmute_test.go#TestUnmuteLockdownLookupFails"
        status: pass
    human_judgment: false
  - id: D5
    description: "resolveUnmutePermissions: active row returns the snapshot, no row or a lifting row returns the live permissions, an unreadable snapshot and a lookup error are errors, chat ID 0 never reaches the lookup"
    requirement: "LOCK-08"
    verification:
      - kind: unit
        ref: "alita/modules/lockdown_unmute_test.go#TestResolveUnmutePermissionsLockdown"
        status: pass
    human_judgment: false
  - id: D6
    description: "In a real supergroup, a user unmuted during a lockdown stays muted by the locked default until /unlockdown and can talk right after it (Telegram's effective-permission rule, assumption A7)"
    requirement: "LOCK-08"
    verification: []
    human_judgment: true
    rationale: "The tests prove the permission set the bot sends; whether Telegram's server combines the group default with the per-user record as assumed (A7) needs a live supergroup"

duration: ~11 min
completed: 2026-10-07
status: complete

plan_head_before: df0e0e4e2fc3ebb6a6e98386c16e5ba69e62b5ad
plan_head_after: b30175a822fcec116e70b193c37bf998126ed947
---

# Phase 4 Plan 07: Unmute during a lockdown Summary

**resolveUnmutePermissions now returns the active lockdown's stored pre-lockdown permissions (and an error on a failed lookup), so /unmute, the unrestrict button, a captcha pass and staff /unmute never leave someone muted after the lift.**

## Performance

- **Duration:** ~11 min
- **Started:** 2026-10-07T03:20:00Z
- **Completed:** 2026-10-07T03:31:00Z
- **Tasks:** 2
- **Files modified:** 15 (1 created, 14 modified)

## Accomplishments

- During an active lockdown the group's live default permissions are the locked set, so the old `resolveUnmutePermissions` copied the locked set into the user's own restriction and they stayed muted after the lift (Pitfall 7b). It now returns the lockdown's stored `pre_permissions` decoded into a `gotgbot.ChatPermissions`, read fresh through the `lockdownSnapshotLookup` seam.
- A failed lockdown lookup (or an undecodable snapshot) is an error. `/unmute` and the button log it and return it before any restrict or reply, `unmuteCaptchaUser` returns it so `restoreCaptchaPermissions` schedules its retry, and staff `/unmute` fails that group before the paced restrict.
- The `/unmute` reply adds `mutes_unmute_lockdown_note` ("... they can talk once an admin runs /unlockdown.") during a lockdown, in all 7 locale files.
- Outside a lockdown nothing changed: live defaults, else the built-in fallback; the existing parallel pure tests (chat ID 0) still never touch the database.
- AGENTS.md Subsystem traps has the new rule in the same commit as the code.

## Task Commits

1. **Task 1: tracer, /unmute during a lockdown** (TDD)
   - RED `a567a31` test(04-07): show /unmute during a lockdown copies the locked set (failed on the permission-set and lockdown-note assertions; the "no lockdown" subtest already passed)
   - GREEN `ac54625` fix(04-07): unmute during a lockdown with the pre-lockdown permissions
2. **Task 2: the other three callers and the failure path** - `b30175a` test(04-07): cover every unmute path and a failed lockdown lookup (all subtests passed on first run because Task 1 changed every caller; no separate fix commit was needed)

**Plan metadata:** the docs(04-07) commit that adds this SUMMARY.

## Files Created/Modified

- `alita/modules/chat_permissions.go` - `lockdownSnapshotLookup` seam and the lockdown-aware `resolveUnmutePermissions`
- `alita/modules/mute.go` - `/unmute` takes the error; the reply appends the lockdown note
- `alita/modules/bans.go` - unrestrict button's Unmute takes the error before `RestrictMember`
- `alita/modules/captcha.go` - `unmuteCaptchaUser` returns the error
- `alita/modules/staff_action_run.go` - staff unmute resolves between its paced `getChat` and restrict, fails the group on an error
- `alita/modules/chat_permissions_test.go` - calls adapted to the two-value form
- `alita/modules/lockdown_unmute_test.go` - `TestUnmuteDuringLockdownUsesSnapshot` (5 subtests), `TestUnmuteLockdownLookupFails` (4), `TestResolveUnmutePermissionsLockdown` (7)
- `locales/{en,es,fr,hi,id,pt,ru}.yml` - `mutes_unmute_lockdown_note`
- `AGENTS.md` - the `resolveUnmutePermissions` rule

## Decisions Made

See `key-decisions` above. All follow the plan's recorded decisions; the unreadable-snapshot error and the note's best-effort second read are the only details the plan left open.

## Deviations from Plan

None - plan executed exactly as written. (Task 2 produced no separate `fix` commit because nothing needed fixing, which the plan allowed; the commit message says so.)

**Total deviations:** 0.

## TDD Gate Compliance

Task 1: RED (`a567a31`, test) precedes GREEN (`ac54625`, fix). The RED failed on the planned assertions (permission set equal to the snapshot, sending allowed, lockdown note present), not on a setup error. Task 2's tests were written after the implementation and passed at once; they are coverage of the other callers and the failure path, not a RED/GREEN pair. No refactor commit.

## Issues Encountered

- **`make lint` not run:** the installed golangci-lint 2.5.0 was built with go1.25 and refuses the go1.26.0 module (known environment limit; no other linter installed). `gofmt -l` is clean on every touched file and `go vet -tags testtools ./alita/modules` passes. Recorded as an unrun verify for the orchestrator.
- **Pre-existing, out of scope:** `gofmt -l alita/modules` also lists `alita/modules/greetings_command_test.go`, which this plan did not touch.
- The first RED attempt failed for a setup reason (the pipeline's admin check reads `getChatAdministrators`, and the fake only knows a creator set with `setCreator`); fixed in the test helper before the RED commit so the RED reason is the assertion on the permission set.

## Verification Results

- `go test -tags testtools -race -count=1 -run '^TestUnmuteDuringLockdownUsesSnapshot$|^TestUnmuteLockdownLookupFails$|^TestResolveUnmutePermissionsLockdown$|^TestResolveUnmutePermissions$|^TestDefaultUnmutePermissions$' -v ./alita/modules` - all PASS
- `go test -tags testtools -race -count=1 -run 'Unmute|Mute|Unrestrict|Captcha|^TestStaffAction|^TestStaffUndo' ./alita/modules` - ok (62 s)
- `make test` - exit 0, 61 packages ok, `alita/modules` 138 s, coverage 70.1%
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./alita/modules` ok; `make check-translations` all present; `make check-docs` no drift; `git diff --exit-code go.mod go.sum` clean
- Acceptance greps: signature line present; 4 non-test callers, all `, err`; `mutes_unmute_lockdown_note` in mute.go and once in each of the 7 locale files; AGENTS.md mentions `resolveUnmutePermissions`; `git log` shows the `test(04-07)` commit before the `fix(04-07)` commit; `t.Run("` count in the new test file is 16 (>= 6)
- Not run: `make lint` (see above); no PostgreSQL run was needed or attempted (tests ran on SQLite)

## End-of-phase UAT item (tracer human check)

In a live Telegram supergroup: mute a member, run `/lockdown`, run `/unmute <member>`; confirm the reply carries the "can talk once an admin runs /unlockdown" note and the member still cannot talk; run `/unlockdown`; confirm the member can talk. Repeat once with the unrestrict button. This checks assumption A7 (Telegram combines the group default with the per-user record), which the fake cannot prove.

## Known Stubs

None.

## Threat Flags

None. No new network endpoint, auth path, file access or schema change; the change narrows what an unmute writes.

## Next Phase Readiness

Plan 04-08 can rely on every unmute path being lockdown-safe. The only open item is the live UAT above.

## Self-Check: PASSED

- Files exist: `alita/modules/lockdown_unmute_test.go`, `alita/modules/chat_permissions.go` (and the 14 modified files)
- Commits exist: `a567a31`, `ac54625`, `b30175a`

---
*Phase: 04-manual-lockdown*
*Completed: 2026-10-07*
