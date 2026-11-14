---
phase: 16-scheduled-deletion-of-service-and-bot-messages
plan: 09
subsystem: verification
tags: [deletion, end-to-end, exclusion, privacy, docs, spike, phase-gate, tracer]
status: complete

requires:
  - phase: 16-scheduled-deletion-of-service-and-bot-messages
    provides: "16-01 to 16-08: the tables and queue, the kind tables and apply functions, the watcher and the deletion core, the worker, the bot-message hooks, the notices, the 57 locale keys and the Deletion page"
provides:
  - "TestDeletionEndToEnd: the whole slice through the real dispatcher with only Telegram faked (six groups of subtests)"
  - "TestDeletionExcludedMessagesNeverScheduled: a Staff Group card and summary, a lockdown alert copy, a captcha challenge and a log post, each through its real sender, leave no row and no delete"
  - "TestDeletionLogsCarryIDsOnly: no message text, claim token or message ID in any log line of the watcher, the helper, the hooks or the worker"
  - "TestSettingsStaffPrivacy extended with the Deletion page (8 languages, group menu and a private menu of an administrator who is not the creator)"
  - "AGENTS.md, README, the handler-groups page and the frozen callbacks and permissions pages describe the phase as built; 16-SPIKE.md lists the owner's deferred checks"
  - "DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 complete"
affects: []

plan_head_before: 40e67bbfa6cc675ec94f43d0850c4de8dddd0c1b
plan_head_after: 0b5f694970882d37ab954c365ece8f581743235e

actuals:
  tokens: 20800   # chars/4 over the added lines of 40e67bb..0b5f694 (83349 chars: two test files, the spike, docs, README, AGENTS.md, REQUIREMENTS.md)
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count 40e67bb..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one end-to-end environment built from the settings Deletion environment, the member requirements toolkit and a settable deletion clock (deletionE2E); time moves only through deletionNow and the worker's own cycle function"
    - "the exclusion test drives each excluded family through its existing fixture and then runs a worker cycle a day and a half later, so a hidden row would have gone out"

key-files:
  created:
    - fuku/modules/deletion_e2e_test.go
    - .planning/phases/16-scheduled-deletion-of-service-and-bot-messages/16-SPIKE.md
  modified:
    - fuku/modules/settings_staff_privacy_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/architecture/handler-groups.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/REQUIREMENTS.md

key-decisions:
  - "The closing plan changed no production code: every step of the end-to-end test passed on first run, so there is no fix(16-09) commit"
  - "The gate's make test was run as the Makefile defines it and failed only on the missing covdata tool; the documented fallback was then run and fed to check_test_results"
  - "PLAT-05 and PLAT-06 stay ticked and are not passed to requirements.mark-complete: PLAT-05 is true (57 keys in 8 locales, AGENTS.md says 312), PLAT-06 holds with the 68 s worst case documented"

patterns-established:
  - "deletionShortestRules(t, chats...): every kind at its shortest delay for each chat, rows removed in t.Cleanup"
  - "deletionWantUntouched: no row and no delete request for the chats, again after a worker cycle a day and a half later"

requirements-completed: [DEL-01, DEL-02, DEL-06, DEL-07, DEL-08, MREQ-15]

coverage:
  - id: D1
    description: "an owner sets a pin rule (10 min) and a title rule (Delete now) from the page; the title notice is deleted at once with no row, the pin notice is queued at its date plus 600 s and the worker deletes it at +601 s and not at +599 s; a type with no rule is left alone"
    requirement: "DEL-01"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/owner_sets_rules_and_notices_are_deleted"
        status: pass
  - id: D2
    description: "Welcome 1 min, Manual 10 min and Auto 1 h set through the page: a join welcome, an admin's /ban reply, a requirement Mute notice (24 h cap, moved to the tap plus 1 h by the button) and a Warn notice are queued by their kinds and the worker deletes each at its time and not a second before"
    requirement: "DEL-02"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/bot_messages"
        status: pass
  - id: D3
    description: "a requirement Mute notice is held at 24 h for its button and moved by the tap; every automatic notice follows the timer"
    requirement: "MREQ-15"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/bot_messages, TestMemberReqNoticeScheduled, TestMemberReqMuteNoticeAtCap, TestMemberReqFixedRescheduleMovesUp"
        status: pass
  - id: D4
    description: "joined and left share /cleanservice both ways (on, off, After 10 min from the page, a bot-caused join) and the join message is deleted after its welcome"
    requirement: "DEL-01"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/joined_and_left_share_/cleanservice, TestSettingsParityCleanService"
        status: pass
  - id: D5
    description: "queued rows survive a restart (a stale claim is released), two workers over one database delete each message once, a 48 hour old row is dropped with no call and no log line, a delay over 24 h is refused"
    requirement: "DEL-06"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/restart,_replicas,_old_rows, TestDeletionWorkerResumesAfterRestart, TestDeletionWorkerOneDeleteAcrossReplicas, TestDeletionWorkerDropsOldRows"
        status: pass
  - id: D6
    description: "a Staff Group card and summary, a lockdown alert copy, a captcha challenge and a log post leave no row and make no delete request, with every rule at its shortest delay, also a day and a half later; a Staff Group chat gets no row, no delete and no page for every service type and bot kind"
    requirement: "DEL-07"
    verification:
      - kind: integration
        ref: "TestDeletionExcludedMessagesNeverScheduled, TestDeletionEndToEnd/Staff_Group, TestServiceWatcherSkipsStaffGroup, TestScheduleRefusesStaffGroup, TestDeletionWorkerSkipsStaffGroup"
        status: pass
  - id: D7
    description: "with the bot a plain member every deletion screen draws the warning once while the rule stays stored, a refused service row latches the chat for service kinds only after one live read and bot rows still go; an unreadable right draws the unknown line and never fails a page"
    requirement: "DEL-08"
    verification:
      - kind: integration
        ref: "TestDeletionEndToEnd/the_bot_cannot_delete, TestSettingsDeletionRightLine, TestDeletionServiceLatchNeedsLiveCheck"
        status: pass
  - id: D8
    description: "no log line carries message text, a claim token or a message ID next to a chat ID; the Warn lines name a chat and a kind; the Deletion hub, lists, 17 rows, every right line, a failed write and the refusals name no Staff Group in 8 languages"
    requirement: "PLAT-06"
    verification:
      - kind: integration
        ref: "TestDeletionLogsCarryIDsOnly, TestSettingsStaffPrivacy (pure renders in every language, Phase 16 private and group runs)"
        status: pass
---

# Phase 16 Plan 09: the closing plan Summary

**The phase's whole vertical slice is proven end to end through the real dispatcher with only Telegram faked, the four excluded families are proven untouched through their real senders, no log line or screen leaks, the rulebook, README and docs describe the phase, the owner has a 14-point live-check list, the gate passed (with the documented covdata fallback), and DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15 are complete.**

Task 1 is this plan's tracer: the owner's page presses, the real dispatcher and the worker's cycle ran green on the first run
and every later subtest extends that proven path.

## Task commits

1. Task 1 `1ac05e3` test(16-09): scheduled deletion works end to end (the three new tests and the Deletion privacy cases)
2. Task 2 `63ca0f3` docs(16-09): describe scheduled deletion in the rulebook, docs and README
3. Task 3 `ecef438` docs(16-09): the owner's live checks, read against the finished code
4. Task 3 `0b5f694` docs(16-09): complete DEL-01, DEL-02, DEL-06, DEL-07, DEL-08 and MREQ-15

## What changed

- `fuku/modules/deletion_e2e_test.go` (new): `deletionE2E` (the settings Deletion environment, the member requirements toolkit and a
  settable `deletionNow`, a second bot process `replicaBot`), `TestDeletionEndToEnd` with six groups of subtests, `TestDeletionExcludedMessagesNeverScheduled`
  (four families) and `TestDeletionLogsCarryIDsOnly`.
  - owner sets rules and notices are deleted: Main, Deletion, Service messages, Pinned message, 10 min, Chat title, Delete now through
    the real page; a title notice is deleted by one inline call and writes no row, a photo notice with no rule is left alone, a pin notice
    is queued at its date plus 600 s, the worker at +599 s deletes nothing and at +601 s deletes it in one `deleteMessages` call.
  - bot messages: Welcome 1 min, Manual 10 min and Auto 1 h set through the page; a join writes one welcome row at +60 s, the owner's `/ban`
    reply a `bot:punish_manual` row at +600 s, a requirement Mute notice a `bot:punish_auto` row at +86400 s that the "I fixed it" tap
    (`mreqPressFixed`) moves to the tap +3600 s, a Warn notice a row at +3600 s; the worker deletes each one second after its time and not before.
  - joined and left share /cleanservice: `/cleanservice on` (both read Delete now, the join message deleted after its welcome, the call order
    checked), a bot-caused join (the watcher), `/cleanservice off` (both Keep, both delay rows removed, nothing deleted), After 10 min from the
    page (row and flag written, Member left reads Delete now, one row per join message, one for a bot-caused join), the worker deletes both at +601 s.
  - the bot cannot delete: a plain member bot (every screen of `settingsDelScreens` draws the warning exactly once while the rule stays stored; a
    refused pin notice latches the chat for service kinds after exactly one live read, the welcome of the same chat is still deleted by its single
    call, a later pin writes no row and a later welcome still does) and an unreadable right (the unknown line, no cannot-open edit, a rule can still be set).
  - Staff Group: every `deletionServiceFields` type, joined and left and every bot kind with every rule stored first: no row, no delete; Main has no
    Deletion cell and the five forged presses are answered invalid once each.
  - restart, replicas, old rows: three real pin rows, a row a crashed worker left acting with a 3-minute-old claim and a 48 hour old row; two workers
    (the second on its own fake) delete each message exactly once, the old one is never sent and no log line names a message ID; a delay over 24 h is refused.
- `fuku/modules/settings_staff_privacy_test.go`: Main's explicit entry list gains the Deletion entry; every language renders the hub, both lists and
  all 17 rows for a group menu and a private admin view under the three right lines, with every rule set and with none, with and without the
  save-failed notice, plus eight refusal alerts; `settingsPrivacyPhase16Run` drives a private and a group menu of an administrator who is not the
  creator through a refused change (no `can_change_info`), the presets with the right, the cannot and unknown right lines and a failed write.
- AGENTS.md: Main's page-list wording ("three pages" was stale), a Subsystem traps bullet for the joined and left coupling through one flag, a bullet
  for the supergroup-migration limit, a Testing bullet for this plan's families and toolkit. README: a Deletion paragraph. `handler-groups.md`: the
  group `-5` row and the corrected raid-mode sentence. `callbacks.md` (frozen, by hand): page `d` with `dl`, `dr`, `st`, the row codes and presets. `permissions.md`
  (frozen, by hand): two rows. `16-SPIKE.md`: 14 numbered checks.
- `.planning/REQUIREMENTS.md`: the six boxes and traceability rows.

## Verification (real results)

| Gate | Result |
|------|--------|
| Task 1 verify 1 `go test -tags testtools -race -count=1 -run '^(TestDeletionEndToEnd\|TestDeletionExcludedMessagesNeverScheduled\|TestDeletionLogsCarryIDsOnly\|TestSettingsStaffPrivacy)$' -v ./fuku/modules` | PASS x4, no SKIP, no FAIL (the privacy test 17.5 s) |
| Task 1 verify 2 `-race -count=10 -run '^TestDeletionEndToEnd$'` | ok, 10 runs |
| Mutation check (restored from a copy afterwards) | `deletion.ClaimDue(ctx, deletionNow().Add(time.Hour), ...)` in `deletion_worker.go` fails two subtests: "the worker at +599 s deleted the pin notice 1 times" and "bot:punish_manual deleted 1 times one second before its time" |
| Task 1 acceptance greps | `func Test...` count 3; `mreqPressFixed\|runDeletionCycle` count 4; the structural DEL-07 grep over `staff*.go lockdown*.go captcha*.go raid*.go logchannels.go fuku/utils/actionlog` prints 0; the skip grep over `git diff dd61b2c` exits 0 |
| Task 2 `make generate-docs` then `make check-docs` | "No drift detected" |
| Task 2 frozen pages | both keep `MANUALLY MAINTAINED` in their first 512 bytes; `grep -c '68 s' AGENTS.md` 1; the old `group -5 is gone` sentence 0; README `Scheduled deletion` found; handler-groups `ServiceDeletion` found; `callbacks.md` `a=dl`/`dl` and `permissions.md` `Deletion` found |
| `make test` exactly as the Makefile defines it (background, output in the scratchpad) | ran 23 minutes; 69 packages `ok` (`fuku/modules` 1365 s), no FAIL line, the only error is `go: no such tool "covdata"` on `fuku/utils/actionlog` (no test files), so `make` exited 2 with an empty `test failure:` line |
| The documented fallback (12-16-SUMMARY, 15-11-SUMMARY): `-json -race -coverprofile -coverpkg` over every package but `fuku/modules` (`-timeout 30m`, exit 1 for the same covdata cause), then `./fuku/modules` alone (`-timeout 40m`, `ok 1407.6 s`, exit 0); the three `actionlog` covdata events (two `build-output`, one `build-fail`) removed; the two streams merged; `go run ./scripts/check_test_results` | exit 0; 70 packages pass; the skips are the PostgreSQL-only tests (migrations, `TestUpdateChat`, `TestStaffExclusivityTrigger`), no unexpected skip |
| The Phase 16 families through CI's gate: 88 new test names plus 9 edited ones in one `-run '^(...)$'` over `./fuku/modules ./fuku/i18n ./fuku/db/deletion ./fuku/db/cache ./fuku/db/greetings ./fuku/db/backup ./fuku/db/migrations`, `-json -race -count=1 -timeout 30m` saved, fed to `check_test_results` | exit 0, exit 0; 95 top-level tests pass, the one skip is `TestScheduledDeletionMigrationDefaults` (PostgreSQL only) |
| `make check-translations` | all 8 locales complete, exit 0 |
| `go vet -tags testtools ./...` | exit 0 |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings" |
| `gofmt -l` over every Go file changed since 8c0c827 | only `main.go`, which is not formatted at 8c0c827 either (21 changed lines since, none formatting) |
| Legacy names: `reflect.Ptr`, `ioutil.` and `//nolint` among added lines | none |
| `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 |
| `make -n test-postgres-integrity \| grep -c 'TestScheduleKeepsEarliest.*TestClaimBatchOneWinner.*TestScheduledDeletionMigrationDefaults'` | 1 |
| Requirement acceptance greps | the six `- [x]` lines 1 each, DEL-03 to DEL-05 `- [ ]` 1 each, PLAT-06 `- [x]` 1; only the six checkboxes and six rows changed in `git diff` |

**Not run:** `make lint` (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0; the fallback above ran), `make test-postgres-integrity`
itself and any PostgreSQL run (no PostgreSQL in this plan: the three PostgreSQL guarantees were proved on a throwaway PostgreSQL 16 cluster in plan 16-01, CI
runs them; the `fuku/modules` harness does not run on PostgreSQL, so the end-to-end test runs on SQLite), `node --test scripts/captcha-page/*.test.mjs` (the
page was not touched), gosec, the live checks of `16-SPIKE.md` (deferred by the owner).

## Excluded families: driven by a real fixture or by the structural proof

All four were driven by a real fixture, none by the structural proof alone: the Staff Group action card and summary by `newStaffActionEnv` and
`runStaffCommand("/mute 4242 flooding")` (two groups linked), the lockdown alert copy by `newLockdownEnv`, `linkLockdownStaffGroup`, `/lockdown` and the worker
cycle (the alert reached the Staff Group exactly once), the captcha challenge by `botSite` and `SendCaptcha` (a challenge message was sent), the log-channel post by
`botSite`, `setStaffLogChannel` and `actionlog.Admin` (one post reached the channel). Each case asserts its flow produced the message first, so it cannot pass by sending
nothing, and the structural grep (a non-test file of those families calling a scheduling helper) prints 0 as well.

## Requirements: what was proved and why each is complete

Ticked after the end-to-end test and the gate passed, through `requirements.mark-complete` with exactly the six IDs; the file was read back and only the six checkboxes and six
traceability rows changed.

- **DEL-01** (Service messages page: keep, now or after N; joined and left share `/cleanservice`): `TestSettingsDeletion*`, `TestServiceWatcher*`, `TestCleanService*`,
  `TestSettingsParityCleanService`, and `TestDeletionEndToEnd` (owner, joined and left subtests). "N seconds" is the owner's locked preset list (D-04: 10 s to 24 h).
- **DEL-02** (Bot messages page: five kinds off or after N minutes): `TestDeletionEndToEnd/bot_messages`, `TestBot*`, `TestScheduleBotMessage`, `TestSettingsDeletion*`.
- **DEL-06** (survive restarts, once across replicas, capped below 48 h, too old dropped quietly): `TestDeletionEndToEnd/restart,_replicas,_old_rows`,
  `TestDeletionWorkerResumesAfterRestart`, `TestDeletionWorkerOneDeleteAcrossReplicas`, `TestDeletionWorkerDropsOldRows`, `TestDeletionDelayCapped`, and the PostgreSQL
  guarantees `TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner` (proved on PostgreSQL 16 in 16-01, in CI's integrity list).
- **DEL-07** (nothing excluded is touched): `TestDeletionExcludedMessagesNeverScheduled`, `TestDeletionEndToEnd/Staff_Group`, `TestServiceWatcherSkipsStaffGroup`,
  `TestScheduleRefusesStaffGroup`, `TestDeletionWorkerSkipsStaffGroup`, `TestSettingsDeletionHidden`, and the structural grep.
- **DEL-08** (every deletion page says so when the bot cannot delete): `TestSettingsDeletionRightLine` (17 answers, 5 screens, both menus) and
  `TestDeletionEndToEnd/the_bot_cannot_delete`. Known nuance, flagged A16-A6 in `16-SPIKE.md` check 2: on Bot messages screens the line is conservative for a bot that is a plain member.
- **MREQ-15** (requirement notices deleted by the automatic-punishments timer, the mute notice held for its button): `TestMemberReqNoticeScheduled`,
  `TestMemberReqMuteNoticeAtCap`, `TestMemberReqFixedRescheduleMovesUp`, `TestDeletionEndToEnd/bot_messages`.
- Left Pending, as required: DEL-03, DEL-04, DEL-05 (Phase 17). **PLAT-05** and **PLAT-06** were already ticked and are cross-cutting: PLAT-05 is true now (57 keys in all 8
  locales, `make check-translations` clean, AGENTS.md says 312 `settings_` keys per locale); PLAT-06 holds (the fleet-wide deletion pacer, `RecoverFromPanic` on every
  goroutine, rows that resume from the database, a 3 s drain) with the documented caveat that the worst case of all drains running to their limit is 68 s against the 60 s budget
  (same shape as the member requirements' 65 s; no correctness cost). Both rows are unchanged.

## Deviations from Plan

1. **[Scope] AGENTS.md was not read whole in one pass.** The file is 3,211 lines and 367 KB, more than this run could take in as one read. Instead every paragraph a Phase 16 plan wrote
   was read against the code (Startup and shutdown including the registration order in `main.go`, the Handlers `-5` and ServiceDeletion bullets, the Callbacks Deletion page and Main grid
   bullets, Permissions, the Data bullets for `chat_delete_rules`, `scheduled_deletions`, `fuku:delete:*`, the Subsystem traps for the core, the worker and scheduling, the Testing counts),
   and a search for every stale figure (`65 s`, `68 s`, `group -5`, `no join handler`, `312`, `255 settings_`, `seven`) found nothing left stale except the phrase fixed below. The 65 s of the member
   requirements bullet is correctly scoped to "the drains up to and including it" and points at the 68 s. A reader who wants the whole-file pass should do it with the owner's usual review.
2. **[Docs] Corrections beyond the plan's list.** "Main lists the three pages after Antiflood" (stale since Phase 13; it lists five pages and the Access row) now reads "the pages after Antiflood".
3. **[Gate] `make test` could not report success by itself**, exactly the documented sandbox case: `go: no such tool "covdata"` for `fuku/utils/actionlog` makes `go test -coverpkg` emit a
   `build-fail`. Every test package passed in that very run (69 `ok`, no FAIL line); the JSON was piped away by the Makefile, so the fallback was run to produce the streams for `check_test_results`.
4. **[Plan text] `TestDeletionLogsCarryIDsOnly` panic shapes.** The plan's list names seams and "every worker failure shape (scripted on the fake bot)"; the test drives the three seams, the queue
   write, both hooked-send failures, and four worker shapes (a refused batch, a 500, a transport error, a 403 gone) and asserts each failing seam produced its Warn. The log line of a recovered
   panic is not asserted by this test; the worker's own `TestDeletionWorkerLogsNoMessageIDs` covers the worker's shapes.
5. **[Plan text] The Staff Group case of `TestSettingsStaffPrivacy`.** The Main entry list gains the Deletion entry; the "Staff Group Main with none" is proved live in `TestDeletionEndToEnd/Staff_Group` and
   `TestSettingsDeletionHidden` (Main has no Deletion cell, forged presses are invalid), not as a pure render, because hiding is done by `settingsMainEntries`, not by the renderer.
6. **[Observation, not changed] `settings_help_msg` still does not name the Deletion page** (A16-A11, recorded in `16-SPIKE.md` check 13): the help text is a locale key with a length test in all 8
   languages and the plan did not ask for it.
7. No plan gate text was broken; no production code changed; no fix commit was needed.

## Known Stubs

None. No file this plan created or changed holds a placeholder, an empty default flowing to the UI or a TODO.

## Threat Flags

None. The plan adds tests, docs and bookkeeping only; no endpoint, table, key or Telegram call.

## Notes for the orchestrator

- The phase is NOT marked Complete in ROADMAP.md and `phase.complete` was not run. The ROADMAP row and the plan checkbox of 16-09 were updated through `roadmap.update-plan-progress`.
- STATE.md reads `Plan: 9 of 9`; its position and status lines were edited by hand to say review, security and verification come next.
- Review points worth a second look: `deletionE2E` shares its fake with the settings environment (so `sendDate` is the deletion clock), and `TestDeletionEndToEnd` runs on SQLite (the
  `fuku/modules` harness does not run on PostgreSQL).

## Self-Check: PASSED

- Created files exist: `fuku/modules/deletion_e2e_test.go` and `16-SPIKE.md` FOUND (and this SUMMARY).
- Commits are ancestors of HEAD: `1ac05e3`, `63ca0f3`, `ecef438`, `0b5f694` FOUND.
- Requirements: the six are ticked in `.planning/REQUIREMENTS.md` (checkbox and traceability row); DEL-03 to DEL-05 stay Pending; PLAT-05 and PLAT-06 unchanged.
