---
phase: "17"
slug: "command-edit-and-self-destruct-deletion"
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-12"
---

# Phase 17 - Validation Strategy

> Reconstructed (State B) from plans 17-01 to 17-12, their summaries, `17-CONTEXT.md` (D-01 to D-34), `17-RESEARCH.md`, `17-12-PLAN.md`'s decision table, the review fix and disposition reports and the test files. Every claim below was checked by reading the named test, and the new tests were run and mutation-checked. Format and depth follow `16-VALIDATION.md`.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, tag `testtools`, hand-written fakes (`lockdownFake`, `deletionScriptClient`), miniredis, SQLite via the harness, no mock libraries |
| Quick run, Phase 17 families | `go test -tags testtools -race -count=1 -run '^(TestMember\|TestSelfDestruct\|TestDeletion\|TestCommand\|TestShutdown\|TestValidation)' ./fuku/modules` (about 9 minutes) |
| Quick run, audit tests | `go test -tags testtools -race -count=1 -run '^TestValidation(CommandAlso\|EditOfSelf\|SelfDestruct)' ./fuku/modules` (about 6 s) |
| Quick run, repository | `go test -tags testtools -race -count=1 ./fuku/db/deletion` |
| Full suite | `make test`; CI `make test-postgres-integrity` |

## Per-Requirement Verification Map

Status columns: before = classification found at the start of this audit, after = after the tests added here.

| Requirement / criterion | Claim | Tests | Before | After |
|---|---|---|---|---|
| DEL-03, criterion 1 | a command of any member, admins included, is deleted after the bot answered it; refused, failed and disabled commands too; another bot's command, an unknown word, edited commands, private chats, bots, sender chats, service accounts and a pending captcha member are kept | `TestCommandDeletionAfterAnswer` (reply index before delete index), `TestCommandDeletionKeeps`, `TestCommandDeletionSenders`, `TestCommandDeletionRefusedAndFailed`, `TestCommandDeletionHeldAndRepeated`, `TestCommandDeletionRawCommand`, `TestCommandDeletionPollingContext`, `TestBotCommandNameMatchesGotgbot` | COVERED | COVERED |
| DEL-03 | anonymous admin: deleted after the proof tap; AnonAdmin on stays (D-34 stated limit) | `TestCommandDeletionAnonymousAdmin`, `TestMemberDeletionEndToEnd` | COVERED | COVERED |
| DEL-03 | a stored value other than 0, or no row, is Off at the command producer | `TestCommandDeletionKeeps` ("a stored value of 60", "no row") | COVERED | COVERED |
| DEL-03 x DEL-05 | a command that is also queued for self-destruct: deleted at once after the answer, the row later completes quietly | none | MISSING | COVERED: `TestValidationCommandAlsoSelfDestructQueued` |
| DEL-04, criterion 2 | an edit later than the grace is deleted at once; at 20 s, exactly at the grace, 48 h old, admin and approved edits kept; no rule keeps; grace presets and the 47 h boundary | `TestMemberEditedDeletesLateEdit`, `TestMemberEditedGrace` (grace, grace+1, 47 h, 47 h+1 min, five presets), `TestMemberEditedKeeps` (includes a stored 45 as Off), `TestMemberDeletionEndToEnd` ("edits") | COVERED | COVERED |
| DEL-04 | the handler never ends an update, also on a panic; a new message never reaches it | `TestMemberEditedNeverEndsUpdate` | COVERED | COVERED |
| DEL-04 | tri-state exemptions: a failed approvals or admin read keeps the message (D-33) | `TestMemberDeletionExemptTriState`, `TestMemberDeletionAdminWarm`, `TestMemberEditedKeeps`, e2e "unknown never deletes" | COVERED | COVERED |
| DEL-04 x DEL-05 | a late edit of a message already queued for self-destruct: deleted at once, row completes quietly | none | MISSING | COVERED: `TestValidationEditOfSelfDestructQueuedMessage` |
| DEL-05, criterion 3 | every member message is queued at date + delay; admins, approved and pinned stay; a pin drops the row (service message, bot's `/pin`, exactness, a race, a failed drop) | `TestMemberSelfDestructQueuesMessages`, `TestMemberSelfDestructKeeps` (stored 45 Off, failing reads), `TestMemberSelfDestructPinnedStays`, `TestMemberSelfDestructPinRace`, `TestMemberDeletePinnedNeedsRule`, `TestDropPendingMessage`, `TestMemberContentMessageExcludesServiceFields` | COVERED | COVERED |
| DEL-05 | the shortest and longest preset fire exactly at date + delay (60 s and 86400 s), not a second earlier | `TestMemberDeletionEndToEnd` used 600 s only | PARTIAL | COVERED: `TestValidationSelfDestructPresetEdges` |
| DEL-05 | Off cancels waiting rows, later messages are not queued, an in-flight flush honours Off (flush-time and worker-time screens) | `TestMemberSelfDestructOffCancels`, `TestMemberSelfDestructBatcherOffRace`, `TestDeletionWorkerScreensSelfDestructOff`, `TestSettingsDeletionMemberWrites`, e2e "self-destruct" | COVERED | COVERED |
| DEL-05 | Off transitions and unaccepted stored values read as Off at all three producers, page and producers agree, written by the page and read back | `TestSettingsDriftDeletionMember` | COVERED | COVERED |
| DEL-05, criterion 4 | busy group: bursts fit, replies are not slowed by a blocked pacer, a 429 delays only deletions, shed is counted and those messages are never deleted | `TestSelfDestructBusyGroup`, `TestDeletionWorkerManyChats`, `TestMemberSelfDestructBatcherSheds` | COVERED | COVERED |
| DEL-05, criterion 4 | the 20,000 ceiling at its real value (19,998 / 19,999 / 20,000 / 20,001 queued) writes exactly the room left and never goes over | tests swapped the ceiling to 5 and 50 | PARTIAL | COVERED: `TestValidationSelfDestructRealCeiling` |
| DEL-05 | restart: entries in the batcher are written by the final flush, rows resume after a restart and two replicas delete each once | `TestMemberDeletionEndToEnd` ("restart and replicas"), `TestShutdownEndsMemberDeletionBatcher` | COVERED | COVERED |
| DEL-07, criterion 4 | nothing on the never-delete list is touched: Staff Group card and summary, lockdown alert copy, captcha challenge, log post; the bot's own messages; no producer reachable from those files; Staff Group refused at the batcher, the queue, the watchers and the page | `TestMemberDeletionExcludedMessages`, `TestMemberDeletionStaffGroupNeverTouched`, `TestMemberEditedStaffGroupNeverTouched`, `TestCommandDeletionStaffGroupNeverTouched`, `TestScheduleBatchRefusesStaffGroup`, `TestSettingsDeletionMemberStaffGroup` | COVERED | COVERED |
| DEL-08 | the right line and `NeedsDeleteRight` at every latch site; a plain-member bot: one check then silence | `TestMemberEditedNeedsDeleteRight`, `TestMemberKindsNeedDeleteRight`, `TestMemberSelfDestructBatcherLatch`, `TestMemberDeletionEndToEnd` ("the bot cannot delete") | COVERED | COVERED |
| criterion 2, grace adjustable on the page | Member messages section, rows `mc`, `me`, `ms`, the Self-destruct two-tap card, rights, staleness, Hidden in a Staff Group | `TestSettingsDeletionMemberOpens`, `...Writes`, `...Card`, `...CodesUnique`, `...Navigation`, `...Stale`, `TestSettingsAuthorityDeletionMember`, `TestSettingsDriftDeletionMember` | COVERED | COVERED |
| PLAT-05 | strings in 8 locales, ceilings, privacy | `TestDeletionKindKeysExist`, `TestSettingsStaffPrivacy`, `./fuku/i18n` | COVERED | COVERED |
| PLAT-06 | recovery, pacing, shutdown: panic in the watchers, flush with its own context, drained inside `StopDeletionWorker`'s 3 s, logs carry IDs only | `TestMemberDeletionLogsCarryIDsOnly`, `TestMemberDeleteFlushWriteOwnContext`, `TestShutdownEndsMemberDeletionBatcher`, `TestShutdownEndsDeletionWorker` | COVERED | COVERED |
| Repository | batch write keeps the earliest time, duplicate keys, validation, Staff Group refusal, `CountPending`, per-kind delay validators by value (-1, 0, 30, 45, 60 ... 86401) | `TestScheduleBatchKeepsEarliest`, `TestScheduleBatchDuplicateKey`, `TestScheduleBatchValidates`, `TestScheduleBatchRefusesStaffGroup`, `TestCountPending`, `TestDropPendingMessage`, `TestMemberKindDelays` | COVERED | COVERED |
| D-01 to D-34 | each decision maps to a test (decision table in `17-12-PLAN.md`); this audit re-read the tests of D-02, D-04, D-05, D-08, D-09, D-10, D-11, D-12, D-26, D-31, D-33 and D-34 for observable assertions | as above | COVERED (D-09's presets and D-31's ceiling PARTIAL) | COVERED |

Totals: 24 claims checked, 21 COVERED, 3 PARTIAL/MISSING at start (2 MISSING cross-rule interactions, the preset ends, the real ceiling: 4 items), all 4 resolved, 0 escalated.

## Tests Added (`fuku/modules/member_deletion_validation_test.go`)

1. `TestValidationCommandAlsoSelfDestructQueued` - Commands plus Self-destruct on: the command is deleted exactly once, after the answer; its queue row (date + 600 s) is written; when due and Telegram answers "message to delete not found" the row completes, no latch key is set and no warn line names the chat.
2. `TestValidationEditOfSelfDestructQueuedMessage` - a message queued at date + 3600 s, then edited 90 s after sending with a 60 s grace: deleted once at once, the row stays for the worker and later completes quietly with no latch and no log line.
3. `TestValidationSelfDestructPresetEdges` - presets 60 s and 86400 s: the row is due at date + delay, nothing is deleted one second before, exactly one delete at the due second.
4. `TestValidationSelfDestructRealCeiling` - the real ceiling (asserted equal to 20000) with 19,998, 19,999, 20,000 and 20,001 rows seeded: an offer of 3 writes 2, 1, 0 and 0, sheds the rest (counted) and the queue never exceeds the ceiling.

Mutation check: changing the 20,000-queued expectation from 0 to 1 failed the test (`memberDeleteFlush() = 0, want 1`).

## Manual-Only Verifications

All live-Telegram checks are the owner's checklist `17-SPIKE.md`: edit noise (link previews, reactions arriving as edits), a bare `/word` in a multi-bot group, the anonymous admin's command live and with AnonAdmin on (D-34), pins seen live, `deleteMessages` at busy-group volume, a claimed batch at the moment of Off, phone-width cells, the native read, two replicas with live batchers, a restart with queued rows, a plain-member bot, the cold admin cache (D-33) and the owner confirmations A17-1 to A17-8. "Commands off stops at the next message across replicas" depends on `delete_rules:*` being in `skipLocal`, proven by `TestSkipLocalMatchesFreshnessCriticalKeys` (`./fuku/db/cache`); two live replicas are 17-SPIKE.md check 10.

## Production bugs found

None. All four new tests passed on the first run against the unchanged implementation.

## Verification output

- `CGO_ENABLED=0 go build ./...`: ok.
- `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on the new file: nothing (other listed files are pre-existing and untouched).
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0 ("no dupl, gocyclo or godox findings").
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- `go test -tags testtools -race -count=1 -run '^TestValidation(CommandAlso|EditOfSelf|SelfDestruct)' ./fuku/modules`: 4 top-level tests (10 with subtests) PASS.
- Family run, `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestMember|TestSelfDestruct|TestDeletion|TestCommand|TestShutdown|TestValidation)' ./fuku/modules`: `ok  github.com/uasneppy/fuku_robot/fuku/modules  546.956s`, exit 0.

## Validation Sign-Off

- [x] Every requirement and criterion has an automated verification
- [x] The four gaps are filled and run
- [x] No production code, go.mod or go.sum touched
- [x] No `t.Skip`, no mock library, fresh IDs, cleanups through the existing toolkit

**Approval:** validated 2026-11-12
