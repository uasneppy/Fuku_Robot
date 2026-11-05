---
phase: 13-member-requirements
plan: 07
subsystem: modules
tags: [member-requirements, executor, pacer, punishments, notices, approvals, tri-state, test-env]
requires:
  - phase: 13-member-requirements
    provides: hold repository (ClaimHold, ReclaimStaleHold, GetHoldFresh, MarkHoldPunished, SetHoldNotice, StaleClaim) (13-01); memberReqBroken, memberReqStrictest, memberReqMention, memberReqRules (13-02); applyWarn with afterRecord, kickMemberContext (13-03); the 25 mreq_ locale keys (13-04)
provides:
  - memberReqEnforce, the one executor (exemptions, hold read, one live getChatMember, claim before punishment, strictest punishment, one notice) with memberReqCase, memberReqResult and six outcomes
  - memberReqPacer and memberReqPaced (fuku:memberreq:pace:next and :pace:block, 100 ms, 3 retries, 60 s, each call bounded by 10 s)
  - memberReqLiveMember, memberReqMuteUntil, isMemberReqMute (the marker is the only proof of ownership)
  - memberReqNoticeText, memberReqNoticeKey, memberReqRulesText, memberReqFixedButton, memberReqNoticeKeyboard, memberReqPostNotice
  - approvals.IsUserApprovedContext (tri-state reader, one shared load path with GetApprovedUsersContext)
  - the shared Phase 13 test environment (memberReqEnv, memberReqCleanup) and per-user profiles in the staff fake (setProfile)
affects: [13-08, 13-09, 13-10, 13-11, 13-12, 13-13, 13-14, member-requirements]

actuals:
  tokens: 21000
  tasks: 3
  commits: 7

commits: 7
plan_head_before: 7f340aa34d263a950519c744d9975683a50a3090
plan_head_after: 8f563b31d07643dbec0c358f7a02bbf08d96166d

tech-stack:
  added: []
  patterns:
    - "Every unknown (approvals, captcha, getChatMember) is exempt: the executor returns None and leaves no hold"
    - "The hold is claimed before the punishment and a warn is marked punished inside the warn lock through applyWarn's afterRecord, so a reclaim cannot repeat a recorded warn"
    - "A member's name is a token spliced after translation (lockdownSplice with memberReqMention), never passed through the translator"
    - "Package-level seams (memberReqApplyWarn, memberReqApprovedLookup, memberReqCaptchaPendingLookup, memberReqCaptchaAttemptLookup) let a test look inside the warn lock or make a read fail"

key-files:
  created:
    - fuku/modules/memberreq_enforce.go
    - fuku/modules/memberreq_notice.go
    - fuku/modules/memberreq_enforce_test.go
    - fuku/modules/memberreq_screens_test.go
    - fuku/modules/memberreq_env_test.go
  modified:
    - fuku/modules/staff_action_fake_test.go
    - fuku/db/approvals/repository.go
    - fuku/db/approvals/repository_test.go
    - fuku/modules/settings_mreq_test.go
    - AGENTS.md

key-decisions:
  - "memberReqFixedButton and memberReqNoticeKeyboard take the translator (the plan's signature had none, but the button label mreq_btn_fixed needs one); the keyboard needs only the action, so the warn result is not a parameter"
  - "The outcome constants are prefixed (memberReqNone, memberReqHeld, memberReqWarned, memberReqRemoved, memberReqMuted, memberReqNotApplied)"
  - "A won claim whose warn recorded but whose warn-limit reset failed is NotApplied with the hold already punished (the warn is stored, nothing is announced): a database failure right after a success, accepted as research S-05 says"
  - "memberReqMarkPunished and the notice-ID write run on context.WithoutCancel with their own 5 s bound, so a shutdown that cancels the run never strands a recorded punishment unmarked"
  - "Logs name a Telegram error by its code alone (memberReqErrText), because Telegram's description can echo request text"

patterns-established:
  - "memberReqEnv on newSettingsEnv: newMemberReqEnv(t), newMemberReqEnvWith(t, opts), mreqMember, mreqRules, mreqCase, mreqEnforce, mreqHold, mreqNotices, mreqWrites, mreqWarnRow, memberReqCleanup(t, chatIDs, userIDs), withFastMemberReqPacer"

requirements-completed: []

coverage:
  - id: D1
    description: "A member with no last name in a group whose last-name rule is Warn is warned once through the warn module, the hold is punished, and one notice names them; a second enforcement is Held and adds no warn, message or write; the warn module's reply and Remove warn button never appear"
    requirement: "MREQ-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqEnforceWarnOnce$' ./fuku/modules (RED at ca4dad4, GREEN at cff5066)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The hold is marked punished inside the warn lock, right after the warn is stored, so a reclaim after a recorded warn never warns twice; a warn that could not be recorded leaves a pending hold that the stale reclaim retries once"
    requirement: "MREQ-04"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqWarnMarkInsideLock$' ./fuku/modules (checks warnPunishLock is held and the hold is punished at that moment)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Kick is the member-removing unban, ban has no end date and no revoke_messages, mute is restrictChatMember with MutedPermissions and the marker until_date stored on the hold before the call, with one button decoding to exactly a=f, h, u within 64 bytes; isMemberReqMute is true only for the marker"
    requirement: "MREQ-08"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqEnforcePunishments|TestMemberReqFixedButtonBytes)$' ./fuku/modules (RED at 7aac5e0, GREEN at 61303f8)"
        status: pass
    human_judgment: false
  - id: D4
    description: "The strictest of the broken rules wins (ban over kick over mute over warn), the one notice names every broken rule not Off in rule order, and a rule set to Off is never named"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqEnforceStrictest$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D5
    description: "The warn-limit notice variants follow the group's warn mode when the limit action took effect and fall back to the plain warn notice when Telegram refused it; a warn-limit mute has no button"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqNoticeWarnLimit$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D6
    description: "With the real locales the notice and the stored warn reason are in the group's language, and a member name of {rules}<b> is printed literally, escaped and isolated; every notice variant with a 64-rune name, all five rules and numbers 99 is at most 400 visible code points in all 8 languages and names no chat ID or Staff Group"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqNoticeLanguage|TestMemberReqScreensFitEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D7
    description: "A live creator or administrator, a member who left or was kicked, a restricted non-member, an approved member, a pending captcha, a bot, the bot itself and 777000, 1087968824, 136817688 are never punished, and an approvals read error, a captcha read error, getChatMember answering 400, 403, a 429 that never clears, 502 or a timeout, or a renamed member punish nobody and leave no hold, warn, write or notice"
    requirement: "MREQ-09"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqEnforceExemptions$' ./fuku/modules (RED at 588ad21, GREEN at 8f563b3)"
        status: pass
      - kind: unit
        ref: "go test -tags testtools -race -count=1 ./fuku/db/approvals (TestIsUserApprovedContext: true, false, an error on a failed read, a write seen by the next read)"
        status: pass
    human_judgment: false

duration: 100min
completed: 2026-11-05
status: complete
---

# Phase 13 Plan 07: The member requirements executor Summary

**`memberReqEnforce` punishes a member who breaks a rule once (the strictest punishment of the broken rules, claimed before it is applied, a warn marked punished inside the warn lock), posts one notice in the group's language that names the member and the rules, and punishes nobody exempt or unknown; the tri-state approvals reader, the requirement pacer and the shared test environment come with it. Nothing in production calls it yet.**

## Performance

- **Tasks:** 3 of 3 (task 1 tracer, tasks 2 and 3 TDD)
- **Commits:** 7 (three RED `test(13-07)`, three GREEN `feat(13-07)`, plus one `fix(13-06)` for a lint finding that is not this plan's code; see deviations)
- No handler is registered, no kill switch exists (`memberReqEnforcementLive` is plan 13-09's) and nothing restricts, kicks, bans, warns or deletes a member in a real group: the only callers of the executor are tests.

## Accomplishments

- **Executor (`memberreq_enforce.go`).** Fixed order: the bot itself, bots and `staffServiceUserIDs`; approvals through `approvals.IsUserApprovedContext` and the pending captcha through the two context reads (each yes, no or unknown, an unknown is exempt); the hold read fresh (a hold that is not a stale pending one is `Held`); one live `getChatMember` through `memberReqPaced`, the admin check and the source of the names; `memberReqBroken` on the live names and `memberReqStrictest`; the claim (`ClaimHold`, or `ReclaimStaleHold` for a pending hold older than `memberreq.StaleClaim`) before any punishment; the punishment; the notice. No code line uses `IsUserAdmin(`, `IsUserApproved(` or `aispamCarvedOut(`.
- **Punishments.** Warn through `applyWarn` with `memberreq.MarkHoldPunished` in `afterRecord` (inside `warnPunishLock`, right after the warn is stored) and the stored reason `mreq_warn_reason`; kick through `kickMemberContext`; ban through `BanChatMemberWithContext` with nil options; mute through `RestrictChatMemberWithContext` with `MutedPermissions` and `until_date` the marker (`memberReqMuteUntil`, now plus 330 days), stored on the hold at the claim. A warn whose limit action took effect as a kick or ban is `Removed`, one whose limit action was the warn module's mute is `Warned`. A refused punishment leaves the pending hold and posts nothing (`NotApplied`).
- **Marker.** `isMemberReqMute` (restricted, still a member, `until_date` within 2 s of `mute_until`, a zero never matches) is the only proof the requirements placed a mute.
- **Notices (`memberreq_notice.go`).** One notice per episode as a new message (HTML, previews off, no reply): `memberReqNoticeKey` picks `mreq_notice_warn`, the three `_warn_limit_*` variants (only when the limit action took effect), `mreq_notice_kick`, `_ban` or `_mute`; `{rules}` is the phrases in rule order joined by ", "; the name is the token `<<mreq-name>>` spliced after translation with `memberReqMention`. Only a mute carries the one `mreq|v1` button `[✅ I fixed it]` (`a=f`, `h`, `u`, at most 52 bytes).
- **Approvals.** `IsUserApprovedContext(ctx, chat, user) (bool, error)` over one shared load helper (`loadApprovedUsers`) that `GetApprovedUsersContext` also uses, whose behaviour is unchanged.
- **Test environment (binding for later Phase 13 plans).** `memberReqEnv` on `newSettingsEnv`; the staff fake gained per-user profiles (`setProfile`, `profile`, `staffFakeUserJSON`) that every member record answer carries, zero value keeping `User<id>`. `memberReqCleanup` deletes the requirement settings and holds, warns, warn settings, approved users and captcha attempts of its chats, the users rows and `user` cache keys of every minted member, the `member_requirements`, `approvals`, `captcha_pending` and `warn_settings` cache keys, the `fuku:memberreq:*` keys of its chats and resets the local cache.
- **AGENTS.md:** `fuku:memberreq:*` in the operational Redis keys sentence, a Data bullet for the pacer keys, and a Subsystem-traps bullet "Member requirements, the executor".

## Task Commits

1. Task 1 (tracer): `ca4dad4` test(13-07) RED, `cff5066` feat(13-07) warn a member who breaks a rule once, with one notice
2. Deviation fix: `8a61a90` fix(13-06): apply De Morgan's law in the rule screen hint check
3. Task 2: `7aac5e0` test(13-07) RED, `61303f8` feat(13-07) kick, ban and mute members who break a rule, strictest first
4. Task 3: `588ad21` test(13-07) RED, `8f563b3` feat(13-07) never punish an exempt member or on an unknown state (also AGENTS.md)

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | ca4dad4 | `first enforcement = {Outcome:0 Hold:<nil>}, want Warned with the hold`; the mark-inside-lock subtests failed too. Compile-only stand-in: `memberreq_enforce.go` had the types, the pacer and a `memberReqEnforce` that returned None, so the tests ran and failed on assertions, not on a build error |
| GREEN task 1 | cff5066 | both tests pass |
| RED task 2 | 7aac5e0 | 11 subtests failed on assertions (Punishments 4, Strictest 3, WarnLimit 3, plus the screens and button-bytes tests). Compile-only stand-ins: `isMemberReqMute` returning false, `memberReqFixedButton` and `memberReqNoticeKeyboard` returning an empty keyboard. Three subtests (mute stored before the call, refused limit action, language) passed at once against the stand-ins: they are guards of behaviour the tracer's code already had |
| GREEN task 2 | 61303f8 | all pass |
| RED task 3 | 588ad21 | `TestIsUserApprovedContext`: `IsUserApprovedContext(approved) = false, <nil>, want true, nil` (compile-only stand-in returning false, nil); `TestMemberReqEnforceExemptions`: the approved member and the approvals read error subtests got a hold and a mute |
| GREEN task 3 | 8f563b3 | all pass |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## Verification (real results)

- Focused tests per task, all PASS: `TestMemberReqEnforceWarnOnce|TestMemberReqWarnMarkInsideLock`; `TestMemberReqEnforcePunishments|TestMemberReqEnforceStrictest|TestMemberReqNoticeWarnLimit|TestMemberReqNoticeLanguage|TestMemberReqScreensFitEveryLocale|TestMemberReqFixedButtonBytes`; `TestMemberReqEnforceExemptions` (17 subtests); `go test ... ./fuku/db/approvals` (`ok`).
- **Family run at plan end** (background, output in the scratchpad): `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestApplyWarn|TestWarn|TestBlacklist|TestStaff)' ./fuku/modules` gave `ok fuku/modules 113.797s`, `family exit=0` (the fake change touches every staff test). `go test -tags testtools -race -count=1 ./fuku/db/approvals ./fuku/db/memberreq`: both `ok`.
- **Settings family, run once** because the staff fake (a shared test helper under every settings test) changed: `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` gave `ok fuku/modules 303.638s`, `ok fuku/i18n 5.281s`, `settings exit=0`.
- `gofmt -l` on every file I touched: empty. `go vet -tags testtools ./fuku/modules ./fuku/db/approvals`: clean. `CGO_ENABLED=0 go build ./...`: ok. `make check-translations`: "All translations are present!". `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0 (no package was installed).
- **Lint stand-in** `python3 /tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/lint-local.py 8c0c827` (the orchestrator's fixed version, which runs the QF quickfix checks): exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827", after the last code commit (the transient U1000 findings for `memberReqNone`, `memberReqRemoved`, `memberReqMuted` and `mreqWrites` it showed mid-task are gone: each is used by a test of this plan). `make lint` itself cannot run here (golangci-lint built with Go 1.25).
- Acceptance greps: one `fuku:memberreq:pace:next` in `memberreq_enforce.go`; no `IsUserAdmin(`, `IsUserApproved(` or `aispamCarvedOut(` on code lines; one `setProfile(` on the fake; one `func IsUserApprovedContext(`; `isMemberReqMute`, `afterRecord` and `fuku:memberreq:pace:next` each appear in AGENTS.md.
- Not run: `make lint`, `make test` (only plan 13-14 runs it), PostgreSQL (nothing in this plan rests on it; the `fuku/modules` harness runs on SQLite).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - blocking, CI lint of the 13-06 push] De Morgan finding in the rule screen test**
- **Found during:** the orchestrator's note after CI's Code Quality job failed on the pushed 13-06 head `7f340aa`.
- **Issue:** staticcheck QF1001 at `fuku/modules/settings_mreq_test.go:861`, the argument `!(i == 0)` of a `t.Errorf`. The lint stand-in had never captured it.
- **Fix:** `i != 0`, nothing else in that file, in its own commit before the last code commit, as the orchestrator asked.
- **Files modified:** `fuku/modules/settings_mreq_test.go`
- **Commit:** `8a61a90`

### Judgment calls

- **Signatures.** `memberReqFixedButton(tr, holdID, userID)` and `memberReqNoticeKeyboard(tr, action, holdID, userID)` take the translator the plan's signature list left out (the label needs it); the outcome constants carry the `memberReq` prefix. `memberReqNoticeText` keeps the planned `(tr, user, broken, action, warn)`.
- **Extra test seams, extra tests.** Four package variables (`memberReqApplyWarn`, `memberReqApprovedLookup`, `memberReqCaptchaPendingLookup`, `memberReqCaptchaAttemptLookup`) so the tests can look inside the warn lock (a `TryLock` and a hold read at that moment), make the warn write fail for one call only (`captchaApplyBrokenDB`, as the plan says) and make an approvals or captcha read fail. `TestMemberReqFixedButtonBytes` (all 8 languages, 19-digit hold ID and 16-digit user ID) was added beyond the list.
- **Lock-order claim of `TestMemberReqWarnMarkInsideLock`** is proved by the wrapper: inside `afterRecord` the warn lock is held (`TryLock` fails) and the hold is already punished.
- **Plan-listed wording "reads the hold fresh... returns Held with it"**: a claim lost to a concurrent winner returns `Held` with a nil hold (the winner's hold is not re-read).
- **Plan's lint path.** `.planning/phases/11-message-editor/handoff/lint-local.py` does not exist; the scratchpad stand-in was used.

No Rule 2 or Rule 4 changes were needed.

## First callers (what a later plan wires)

- `memberReqEnforce`, `memberReqCase`, `memberReqResult`, `memberReqPaced`, `memberReqLiveMember`: first production caller is the message watcher of **13-09** (kill switch `memberReqEnforcementLive` false), then the join entry of **13-11**; **13-08** adds the failure classification and the undeliverable-notice rule on top of `memberReqSendStep` and `memberReqAnnounce`; **13-10** adds the delete, the cap and the pause.
- `isMemberReqMute`: **13-13** ("I fixed it"); also the verification of an unknown mute answer in 13-08.
- `memberReqFixedButton`, `memberReqNoticeKeyboard`, `memberReqCallbackNamespace`: posted with the mute notice from 13-09; the handler for the `mreq|` namespace and the kill switch turning on arrive in **13-13**.
- `approvals.IsUserApprovedContext`: the executor (13-09 onward) and the watcher's job.
- `memberReqRulesText` and `memberReqText`: also by 13-13 for "still broken". `memberReqMuteUntil`: the claim, and the captcha guard of 13-12.

## Known Stubs

None. The RED commits carried compile-only stand-ins; all are replaced. The executor is not reached from any handler by design (13-09 and 13-13), which is a planned boundary and not a stub.

## Threat Flags

None. No new endpoint, auth path or file access. T-13-19 (punishing an admin on a stale list or failed lookup) is mitigated by the one live `getChatMember` and `TestMemberReqEnforceExemptions`; T-13-20 (an unknown read as not exempt) by the tri-state reads and the grep-checked prohibition; T-13-21 (a double warn) by the claim before the punishment, the hold marked inside the warn lock and `TestMemberReqEnforceWarnOnce`, `TestMemberReqWarnMarkInsideLock`; T-13-22 (HTML or bidi injection) by `memberReqMention` spliced after translation and `TestMemberReqNoticeLanguage`; T-13-23 and T-13-24 (disclosure, size) by `TestMemberReqScreensFitEveryLocale`; T-13-SC by the unchanged `go.mod` and `go.sum`.

## Requirements

`requirements-completed: []`. The plan contributes to MREQ-04, MREQ-05, MREQ-07, MREQ-08 and MREQ-09 and completes none: all five stay Pending in REQUIREMENTS.md and `requirements.mark-complete` was not run. MREQ-05 and MREQ-09 are completed by 13-11, MREQ-04 and MREQ-08 by 13-12, MREQ-07 by 13-13, as the plan says.

## Notes for later plans

- Test hygiene: a test that enforces mints members with `env.mreqMember` (unique IDs, cleanup registered) and uses the settings environment's own unique chat; `warns.WarnUser` creates a `users` row, which `memberReqCleanup` deletes. 429 handling in the fake: a scripted 429 is retried by the pacer, so a lookup that must never clear scripts five of them.
- STATE.md "Plan:" reads `8 of 14`; ROADMAP.md was not marked complete.

## Self-Check: PASSED

- FOUND: fuku/modules/memberreq_enforce.go, memberreq_notice.go, memberreq_enforce_test.go, memberreq_screens_test.go, memberreq_env_test.go, fuku/db/approvals/repository.go (`func IsUserApprovedContext(` once).
- FOUND commits (ancestors of HEAD): ca4dad4, cff5066, 8a61a90, 7aac5e0, 61303f8, 588ad21, 8f563b3.
- `git rev-list --count 7f340aa..HEAD` was 7 before this SUMMARY commit.
