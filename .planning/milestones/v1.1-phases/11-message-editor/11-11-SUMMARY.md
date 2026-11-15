---
phase: 11-message-editor
plan: 11
subsystem: modules
tags: [settings-menu, message-editor, typed-reply-door, handler-group-1, owner-gate, tdd]
requires:
  - phase: 11-message-editor
    provides: plan 11-07 wait store (settingsInputRead, Claim, Finish, Release, TakeMarker, Cancel); plan 11-08/11-09 editor pages, Set and Cancel actions; plan 11-10 settingsEditCheck; plan 11-05 apply functions
provides:
  - settingsInput, the typed-reply door at handler group 1 (settingsInputGroup), registered by LoadSettingsInput
  - the saved page as a new message in the private chat, the closed prompt, the refusal reply with Cancel, the wait-ended reply
  - the typed reply as the sixth door of TestSettingsGateAllDoors
affects: [11-12, 11-13, 11-14, settings-message-editor]

actuals:
  tokens: 17100
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 3c3c14607e408b1d709dff05a223f72cda1f9a05
plan_head_after: 89ddd0e03395ba4f9b70d87812ac683064e75e30

tech-stack:
  added: []
  patterns:
    - "The door judges a reply exactly like a press: live getChatMember, settingsGate on that record, settingsRightRefusalFor (plus IsBotAdmin on Rules), live getChat, and only then the pure check, the claim and the part's apply function"
    - "A loss of authority ends the wait (settingsInputCancel with the wait's own group, page and part); an answer that cannot be established keeps it"
    - "A refusal never claims or extends the wait; a write that fails after the claim releases it"

key-files:
  created:
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_input_door_test.go
  modified:
    - fuku/modules/settings_input.go
    - fuku/modules/settings_input_test.go
    - fuku/modules/settings_gate_test.go
    - AGENTS.md

key-decisions:
  - "The named-result panic guard is `ret = ext.EndGroups` set once the message is consumed (after the no-wait and marker steps), with `defer error_handling.RecoverFromPanic` directly: recover() only works when called by the deferred function itself, so wrapping RecoverFromPanic in another deferred func (as the plan words it) would not recover. A panic before consumption returns nil, which hands the message on."
  - "The Rules bot-admin right is checked with chat_status.IsBotAdmin on a synthetic supergroup chat of the wait's group ID, before getChat, in the plan's order; the press uses the same predicate after getChat."
  - "settingsInputValidPage was deleted (nothing called it once settingsInputValidPart looks the page and part up in the editor's page table)."

requirements-completed: [EDIT-02, EDIT-03]

duration: 75min
completed: 2026-11-04
status: complete

coverage:
  - deliverable: "A typed reply is saved exactly as the commands save it, on any replica, with the saved page as a new message and the prompt closed"
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditTypedTextSaved"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditTextParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditRulesParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditWaitAcrossReplicas"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditDoorStoresEveryPart"
        status: pass
    human_judgment: false
  - deliverable: "The door is silent for everyone else and judges the reply like a press (eleven worlds), and is the sixth door of the owner gate table"
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditDoorPassThrough"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditDoorAuthority"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_gate_test.go#TestSettingsGateAllDoors"
        status: pass
    human_judgment: false
  - deliverable: "A refusal keeps the wait (not claimed, not extended), a failed write releases it, a late reply is told once, a double delivery writes once, no log carries the message"
    verification:
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditRefusalKeepsWait"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditWriteFailureReleases"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditWaitExpiry"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditMarkerReplyOnce"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditDoubleDelivery"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_input_door_test.go#TestSettingsEditDoorLogsNoText"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 11: The typed-reply door Summary

**After Set, the owner's next private message becomes that part: the door at handler group 1 reads the sender's wait, judges them live like a press (member record, owner gate, right of the part, getChat), checks the message with `settingsEditCheck`, claims the wait by nonce and writes through the part's apply function, then sends the saved page as a new message and closes the prompt. Anyone with no wait and no marker is untouched.**

## Accomplishments

- Task 1 (tracer): `settings_input_door.go` with `settingsInputGroup = 1`, `settingsInputIsPrivate`, the candidate and command predicates (`bot_command` at offset 0 or a slash plus a non-space, in text or caption; service identities, bots, rich messages and non-self senders never), the door `settingsModule.settingsInput` and a `settingsInputReply` that carries one reply. `LoadSettingsInput` registers it. `settingsInputValidPart` now looks the page and part up in the editor's page table (`settingsEditPageByKey`, `settingsEditPartByCode`), so a wait can only name what the editor registers.
- Task 2 (TDD): the wait-ended reply (the marker taken with `GETDEL`, one plain `settings_edit_wait_ended_note` naming the part, two deliveries answer once) and the failed-write answer (`common_settings_save_failed`, still waiting and Cancel, the claim released). Eleven tests cover parity (Welcome, Goodbye, Rules), pass-through, authority in eleven worlds, refusals keeping the wait, a failed write, two replicas, expiry, the marker, double delivery and logs.
- Task 3: `settingsGateDoorTypedReply` is the sixth door of `TestSettingsGateAllDoors` (the saved-page marker is an "in" in `settingsGateClassify`); AGENTS.md lists handler group `1`, describes the door and names it among the gate's doors, and no longer says the Settings module adds no handler group.
- An extra guard test, `TestSettingsEditDoorStoresEveryPart`, proves a typed photo and a typed button line are stored as media and buttons with everything else kept, and that 1025 characters over stored media are refused as a caption with the wait kept. It is why EDIT-02 can be ticked: the door's media and buttons paths ran end to end here, ahead of plan 11-13's album and notice work.

## Task Commits

1. Task 1 (tracer): `18b4dc3` feat(11-11): save the owner's next private message as the part they set
2. Task 2 RED: `9f7c756` test(11-11): add failing tests for the typed-reply door's refusals, expiry and failed write
3. Task 2 GREEN: `0b7b884` feat(11-11): tell a late reply the wait ended and answer a failed save
4. Task 3: `28c1236` test(11-11): add the typed reply as the sixth door of the owner gate table (also carries the AGENTS.md edits)
5. Guard: `89ddd0e` test(11-11): prove the door stores media, buttons and refuses a long caption

## TDD Gate Compliance

Task 2 is the plan's `tdd="true"` task (the plan is `type: execute`, so no plan-level gate applies; `gsd_run check tdd-red-evidence` was not run and no evidence record was persisted).

- RED (`9f7c756`): the real `go test` run (package compiled) failed exactly `TestSettingsEditWriteFailureReleases` ("the bot sent 0 messages, want exactly 1"), `TestSettingsEditWaitExpiry` (same) and `TestSettingsEditMarkerReplyOnce/two_deliveries_answer_once` (same), the three behaviours the tracer left out. Semantic assessment: the targets ran and failed on the planned assertion for the intended reason (no reply was sent). The other tests of the commit passed already, because Task 1 built the whole authority, refusal and claim flow of the plan's action text; they are regression guards for that behaviour, as in plan 11-10. An earlier RED run also failed the parity and pass-through tests on a test mistake (the command owner was replaced between cases, so the cached administrator list refused the second command, and an unrelated `fuku:cache:antiflood` key was counted); those were fixed in the test before the RED commit and are not the door's.
- GREEN (`0b7b884`): the three targets and every other test of the task pass (also `-count=4` for the concurrent ones). REFACTOR: none.
- Task 1 is a tracer, committed as one `feat` commit with its test, as the plan says.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestSettingsEditTypedTextSaved$' ./fuku/modules`: ok (`--- PASS`, run with `-v`).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsEditTextParity|TestSettingsEditRulesParity|TestSettingsEditDoorPassThrough|TestSettingsEditDoorAuthority|TestSettingsEditRefusalKeepsWait|TestSettingsEditWriteFailureReleases|TestSettingsEditWaitAcrossReplicas|TestSettingsEditWaitExpiry|TestSettingsEditMarkerReplyOnce|TestSettingsEditDoubleDelivery|TestSettingsEditDoorLogsNoText)$' ./fuku/modules`: ok 10.3 s. The focused command of this plan, the one the orchestrator should run.
- `go test -tags testtools -race -count=1 -v -run '^TestSettingsGateAllDoors$' ./fuku/modules`: `--- PASS` for all five fixture subtests (the typed reply decides "in", "owner_only" and "check_failed" as the other five doors do).
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (after the last code and test change except the final guard test, which was run alone): both ok (201 s and 7 s).
- `go test -tags testtools -race -count=1 -run '^(TestGreeting|TestWelcome|TestSetGreeting|TestMemberJoin|TestLeftMember|TestSetRules|TestSendRules|TestNotes)' ./fuku/modules`: ok.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l` on the touched files: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings on lines added since 72a692b" (run after the last code commit and again after the last test commit).
- Acceptance greps: `settingsInputGroup = 1` in the door file 1; `settingsInputGroup)` in `settings_input.go` 1; `settingsEditPageByKey` in `settings_input.go` 1; `settingsInputClaim(` in the door file 1, at line 140 after `settingsEditCheck(` at line 126; the live-checks-only grep (`GetGreetingSettings|GetChatRulesInfo|IsUserAdmin(`, comments excluded) 0; `"typed reply"` in the gate test 1; `typed-reply door` in AGENTS.md 4 (baseline 0); group `1` line present in the Handlers list.
- Not run: `make lint` (installed golangci-lint is built with Go 1.25, a sandbox fact; the stand-in ran instead); `make test` (not asked by this plan); PostgreSQL (nothing here needs it); `node --test` (no page change).

## Requirements

Completes EDIT-02 (after Set the next private message is stored as that part, for text, media and buttons; Cancel, `/cancel`, the 10-minute expiry and any replica all tested) and EDIT-03 (formatting and placeholders kept, parity with the three commands; 4096 and 1024 limits refused with the reason and the wait kept). Both are ticked in REQUIREMENTS.md (checkbox and traceability row). EDIT-04 (album handling and the album notice), EDIT-05 and EDIT-11 stay Pending: plan 11-13 proves them.

## Deviations from Plan

**1. [Rule 1 - Bug in the plan] The panic guard cannot be a named result set in a deferred func around `RecoverFromPanic`**
- `recover()` only recovers when the deferred function itself calls it; `RecoverFromPanic` called from inside another deferred func returns nil. The door uses `defer error_handling.RecoverFromPanic("settingsInput", "Settings")` directly and sets the named result `ret = ext.EndGroups` once the message is consumed, so a recovered panic after that point still ends the update and a panic before it hands the message on.

**2. [Test] The expectation of plan 11-07's test changed** (`fuku/modules/settings_input_test.go`, outside the plan's file list)
- `TestSettingsInputRead` seeded a Rules wait for part `b` and the marker parse test accepted pages and parts from the literal allow-list. With the page-table lookup the plan asks for, Rules has a text part only, so the seed became Goodbye part `b`, and `-100|r|m` was added to the marker's refused examples. This is the change the plan itself asks for.

**3. [Process] The tracer built the whole authority and refusal flow, so most Task 2 tests were green at RED** (see TDD Gate Compliance).

**4. [Extra] `TestSettingsEditDoorStoresEveryPart`** is not in the plan's test list; it backs the claim that EDIT-02 is complete for the media and buttons parts.

**Total deviations:** 1 plan-text bug, 1 test expectation, 1 process note, 1 extra test. **Impact:** none on the contract plans 11-12 to 11-14 use.

## Known Stubs

None. The album gate and the album-cut notice are plan 11-13's by design (until then a refused album answers once per item and an accepted one still writes once, by the claim).

## Threat Flags

None beyond the plan's register. T-11-39 (live member record, owner gate and right on every reply; `TestSettingsEditDoorAuthority`, sixth door of `TestSettingsGateAllDoors`), T-11-40 (the wait key is the sender's own ID, the sender must be the private chat, service identities and bots never candidates), T-11-41 (the group comes from the wait; getChat must show a group or supergroup), T-11-42 (claim after the check, release on a failed write; `TestSettingsEditDoubleDelivery`, `TestSettingsEditRefusalKeepsWait`, `TestSettingsEditWriteFailureReleases`), T-11-43 (access_lost is one text for every 4xx and non-admin status and names no group), T-11-44 (cheap checks first, one Redis round trip, no Telegram call without a wait; `TestSettingsEditDoorPassThrough`), T-11-45 (logs carry IDs, part and outcome only; `TestSettingsEditDoorLogsNoText`), T-11-46 (group 1 after the group-0 commands, slash texts and captions are not candidates) are mitigated and tested; no package was installed (T-11-SC).

## Issues Encountered

- `staffActionFake` has no persistent failure for `getChat`/`getChatMember` per call: scripted one-shot errors (`fake.script`) are used for those, and `setFailure` for `getChatAdministrators`.
- The cached admin list of a chat names the first owner only, so a parity test uses one command owner per chat.

## Next Phase Readiness

Plan 11-13 adds the media and buttons proofs of the door's apply branches, the album gate (`settingsInputAlbumFirst`, which the door does not call yet) and the album-cut notice. Plan 11-12 and later plans need nothing else from this door.

## Self-Check: PASSED

- Created files exist: `fuku/modules/settings_input_door.go`, `fuku/modules/settings_input_door_test.go` (found on disk).
- Commits `18b4dc3`, `9f7c756`, `0b7b884`, `28c1236` and `89ddd0e` are ancestors of HEAD; `git rev-list --count 3c3c146..HEAD` was 5 before this SUMMARY commit.
- Every acceptance criterion was re-run after the last code commit and passes (results above).
