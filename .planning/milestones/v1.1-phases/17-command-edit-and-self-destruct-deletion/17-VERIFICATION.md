---
phase: 17-command-edit-and-self-destruct-deletion
verified: 2026-11-12T15:00:00Z
status: human_needed
score: "10/10 truths verified in code and tests (4 ROADMAP success criteria plus 6 supporting truths); 1 warning (the roadmap's '1 minute by default' grace is not realised as a default, see truth 2 and human item 2); DEL-03, DEL-04, DEL-05 judged satisfied in code, so the ticks of plan 17-12 are true; DEL-07, DEL-08, PLAT-05 and PLAT-06 judged still satisfied"
covered_files:
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-01-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-01-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-02-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-02-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-03-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-03-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-04-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-04-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-05-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-05-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-06-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-06-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-07-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-07-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-08-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-08-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-09-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-09-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-10-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-10-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-11-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-11-SUMMARY.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-12-PLAN.md"
  - ".planning/phases/17-command-edit-and-self-destruct-deletion/17-12-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "fuku/db/deletion/kinds.go"
  - "fuku/db/deletion/queue.go"
  - "fuku/db/deletion/rules.go"
  - "fuku/modules/command_deletion.go"
  - "fuku/modules/deletion_apply.go"
  - "fuku/modules/deletion_kinds.go"
  - "fuku/modules/deletion_right.go"
  - "fuku/modules/deletion_schedule.go"
  - "fuku/modules/deletion_worker.go"
  - "fuku/modules/member_deletion.go"
  - "fuku/modules/member_deletion_batcher.go"
  - "fuku/modules/pins.go"
  - "fuku/modules/settings_deletion.go"
  - "fuku/utils/helpers/command_registry.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "main.go"
covered_digest: "v4:sha256:5ca5a549f70e7974d92bbc74d7e8e19aadd64dee812a7ea7f27f375a5badc868"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "SPIKE 1: edited-message noise. In a live group, edit a message past the grace with an unchanged text, a link preview refresh and a reaction."
    expected: "Only a real edit_date past the grace deletes the message; noise that Telegram does not report as an edit never does."
    why_human: "Needs a live group. The handler judges only edit_date against date (memberEditDue) and cannot tell what an edit changed (A17-3)."
  - test: "SPIKE 2 and 3 and 4: a bare /word in a multi-bot group; the anonymous admin's command after the proof tap; the same in a group whose AnonAdmin setting is on."
    expected: "A word another bot also handles may be deleted for this bot's registry (accepted race); an anonymous admin's command is deleted only after the tap ran it; with AnonAdmin on the command stays (D-34, A17-2)."
    why_human: "Needs a live multi-bot group and real anonymous-admin traffic. Proven against fakes by TestCommandDeletionAnonymousAdmin and TestBotCommandNameMatchesGotgbot (library's own CheckUpdate)."
  - test: "SPIKE 15 (A17-4) and the roadmap's '1 minute by default': decide what 'default' means for Edited messages."
    expected: "The owner either accepts 'presets only, nothing preselected' (D-30), or asks for the 1-minute preset to be marked or preselected on the page."
    why_human: "Owner decision. The code has no default anywhere (see truth 2). 17-SPIKE.md A17-4 and the 17-12 SUMMARY both say the 1-minute default 'exists in the apply function'; that sentence is not true of the code and should be corrected when the owner answers."
  - test: "SPIKE 5: pinned messages seen live (a pin by an admin, by /pin, and unpin)."
    expected: "A pinned message is never deleted by self-destruct; unpinning does not re-queue it."
    why_human: "Needs live pin service messages. Proven by TestMemberSelfDestructPinnedStays, TestMemberSelfDestructPinRace and the /pin hook at pins.go:338."
  - test: "SPIKE 6 and 7: deleteMessages at busy-group volume, and a claimed batch at the moment of Off."
    expected: "Telegram accepts 100-ID batches at the paced rate; a batch already claimed when the owner switches Off can finish within its 90 s cycle (accepted AR-9)."
    why_human: "Real rate limits and real timing need live Telegram. TestSelfDestructBusyGroup proves the call count and pacing over the real 100 ms pacer."
  - test: "SPIKE 8 and 9: phone-width cells in hi, ru, uk on the Member messages hub, list and row screens; native-speaker read of the 18 new strings and the changed intro."
    expected: "Nothing is cut off or reads wrongly."
    why_human: "Visual and language checks. Length and bytes are guarded by TestSettingsLabelLengths, the limits tables and TestSettingsDeletionPageFitsEveryLocale."
  - test: "SPIKE 10, 11, 12, 13, 14: two replicas with the batcher on both, a restart with entries in the batcher, a plain-member bot, the delete-right line live on the Member screens, a cold admin cache."
    expected: "One deletion per row across replicas; a clean stop resumes; a bot with no delete right costs one check then silence; the right line is correct; a cold cache warms once per chat."
    why_human: "Need two live processes, real signals and real Telegram answers."
  - test: "SPIKE 15 (rest) and 16: owner confirmations A17-1 to A17-8 and A17-A3/A5, amendments D-21 to D-34 and the three deviation notes."
    expected: "The owner confirms or changes each provisional decision."
    why_human: "Owner decisions; the standing choice is to defer them."
  - test: "SPIKE 17: DEL-07 live with all three rules on (staff card, lockdown alert, captcha challenge, log post)."
    expected: "None is ever deleted."
    why_human: "Live confirmation. Proven in TestMemberDeletionExcludedMessages through the four real producers."
  - test: "SPIKE 19: read the CI run of the pushed head (make lint, make test, make test-postgres-integrity)."
    expected: "All green, including the four names added to the Makefile list: TestScheduleBatchKeepsEarliest, TestScheduleBatchDuplicateKey, TestCountPending, TestDropPendingMessage."
    why_human: "make lint and the PostgreSQL run cannot run in this sandbox and CI results are not visible here. The last whole-package fuku/modules runs recorded by the executors were ok (1521 s, 1615 s, 1585 s)."
audit_acknowledged:
  milestone: v1.1
  at: 2026-11-13
  status: human_needed
---

# Phase 17: Command, Edit and Self-Destruct Deletion Verification Report

**Phase Goal:** As a group owner, I want members' commands, late edits and, when I choose, every message removed automatically, so that the group stays clean and an edited message cannot slip spam in later.
**Verified:** 2026-11-12T15:00:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

The phase is MVP mode. The outcome clause ("the group stays clean and an edited message cannot slip spam in later") is traced below as three flows (commands, edits, self-destruct) from the owner's page press to the delete, plus the never-delete list and the pace under load.

## Goal Achievement

### User Flow Coverage

| Step of the story | Expected | Evidence in the code | Status |
| ----------------- | -------- | -------------------- | ------ |
| Owner turns a rule on | Deletion page, section `m`, rows `mc me ms` write `member:*` rules; Self-destruct asks for a card Yes | `fuku/modules/deletion_kinds.go:210-233` (`deletionMemberRows`), `fuku/modules/settings_deletion.go:82-83` (section `m`), `settings_pages.go`, `settings_menu.go`, `settings_card.go` unchanged since before the phase | VERIFIED |
| A member sends a command | The bot answers, then the command is deleted | `main.go:431` wraps `tracing.TracingProcessor` in `modules.CommandDeletionProcessor`; `command_deletion.go:40-86` runs `Inner` first, then `deletionAct` delay 0 | VERIFIED |
| A member edits a message late | The edit handler deletes it, admins and approved users are kept | `member_deletion.go:345-376`, registered at group -8 by `LoadMemberDeletion` (module priority 243, `member_deletion.go:564`) | VERIFIED |
| A member sends any message | The watcher offers it to the batcher, the batcher writes the queue rows, the worker deletes at date + N | `member_deletion.go:522-550`, `member_deletion_batcher.go`, `deletion_worker.go` (`memberDeleteStart` at line 80, 1000-row cycles, 100-ID chunks) | VERIFIED |
| Something must never be deleted | Staff Group, lockdown alerts, captcha challenges, log posts, the bot's own messages | `deletionAct` refusal, batcher latch screen, worker Staff Group drop, bot senders excluded; `TestMemberDeletionExcludedMessages` | VERIFIED |

### Observable Truths

| #  | Truth | Status | Evidence |
| -- | ----- | ------ | -------- |
| 1 | SC1: With Delete commands on, every member's command, admins' included, is deleted after the bot answered it, and the answer still arrives | VERIFIED | `commandDeletionAfter` (`command_deletion.go:56-86`) runs only after `inner.ProcessUpdate` returned, so every handler group and every reply has finished. Command test is `botCommandName` mirroring gotgbot's matcher plus `helpers.IsBotCommand` (registry fed by `helpers.NewBotCommand`, `command_registry.go:28`; no other `handlers.NewCommand(` in production code, grep clean). Sender must be a user account (`commandDeletionSenderIsMember`), admins are not excluded. Rule read is `member:commands` stored as 0 (`commandDeletionRuleOn`). Delete is `deletionAct` delay 0. Tests passed this run: `TestCommandDeletionAfterAnswer` (reply index < delete index, under the base and the tracing processor, banChatMember still sent, bot's own messages kept, no queue rows), `TestCommandDeletionRawCommand`, `TestCommandDeletionKeeps`, `TestCommandDeletionSenders`, `TestCommandDeletionRefusedAndFailed`, `TestBotCommandNameMatchesGotgbot` (against the library's own `CheckUpdate`), `TestCommandRepliesStayInsideTheHandler`, `TestMemberDeletionEndToEnd/commands` |
| 2 | SC2: With Delete edited messages on, a member's message edited after the grace is deleted; edits by admins and approved users are kept; the grace is adjustable on the page (1 minute by default) | VERIFIED for behaviour, WARNING for the word "default" | Behaviour: `memberEditedHandler` (`member_deletion.go:329-376`): raw handler matching `ctx.EditedMessage != nil` only, result preset to `ext.ContinueGroups` with recover, order chat type, `raidFloodSenderCounts`, bot and `via_bot`, pure `memberEditDue` (gap equal to the grace is kept, older than 47 h never due), stored grace from `deletionRulesLookup` (only the five presets count, a hand-written 45 is Off), then `memberDeletionExempt` (approvals tri-state, cached admin list, warm once via singleflight `cache.LoadAdminCache`, one live `getChatMember` fallback, unknown is exempt) and the pending captcha, then `deletionAct`. Grace is set on the page by row `me` with presets 30 s, 1, 5, 15 min, 1 h (`deletion_kinds.go:219-225`). Tests passed: `TestMemberEditedDeletesLateEdit`, `TestMemberEditedGrace`, `TestMemberEditedKeeps`, `TestMemberDeletionExemptTriState`, `TestMemberDeletionAdminWarm`, `TestMemberEditedNeverEndsUpdate`, `TestMemberEditedStaffGroupNeverTouched`, `TestMemberEditedNeedsDeleteRight`, and the end-to-end case (a 20 s edit kept, a 90 s edit deleted with a 60 s grace). Warning: nothing in the code or on the page makes 1 minute a default (no grep hit for any default grace; D-30 removed it and the page preselects nothing). 17-SPIKE.md A17-4 and the 17-12 SUMMARY claim the default "exists in the apply function"; it does not. See human item 3 |
| 3 | SC3: With Self-destruct on, every member message is deleted N minutes after it was sent; admins, approved users and pinned messages stay | VERIFIED | Watcher `memberSelfDestructWatch` (`member_deletion.go:522-550`) at group -8 with `message.All` and no setter: `pinned_message` service message drops that row first (`memberDeletePinned`), then sender shape and not a bot, `memberContentMessage` (no service, creation or migration messages), stored delay (six presets only), `memberDeletionExempt` (approved, admin or unknown all exempt), then `memberDeleteOffer` with `SentAt` the message's own date. `/pin` also drops the row (`pins.go:338`). The batcher screens the rule, the delete-right latch, the 20,000-row ceiling and pins again right before writing (`member_deletion_batcher.go:241-337`). The worker re-screens self-destruct rows against the current rule (`deletion_worker.go:370-399`). Tests passed: `TestMemberSelfDestructQueuesMessages`, `TestMemberSelfDestructKeeps`, `TestMemberSelfDestructPinnedStays`, `TestMemberSelfDestructPinRace`, `TestMemberSelfDestructOffCancels`, `TestMemberSelfDestructBatcher{Queues,Earliest,Latch,OffRace,Sheds}`, `TestDeletionWorkerScreensSelfDestructOff`, `TestMemberDeletionEndToEnd` (queued at date + 600 s, nothing at +599 s, one delete at +601 s, no rows left) |
| 4 | SC4: In a busy group with self-destruct on, deletions keep pace without hitting Telegram's limits or slowing the bot's replies, and nothing on the never-delete list is touched | VERIFIED | Pace: offers are one non-blocking channel send (4096 buffer), the batcher writes in statements of at most 100 rows every 300 ms or at 100 buffered, the worker claims 1000 rows a cycle and makes one `deleteMessages` call per 100 IDs on the deletion pacer's own keys (`deletion_worker.go:28-30`, `:437`). `TestSelfDestructBusyGroup` (passed) sends 3000 messages through the real dispatcher: every update only offered and wrote nothing, flush wrote all 3000, the worker made exactly 30 calls of 100 IDs and no single deletes, at the real 100 ms pacer, a block of the deletion budget did not slow a reply, a 429 only delayed deletions, the ceiling shed the excess. Never-delete list: `TestMemberDeletionExcludedMessages` (passed) drives a real Staff Group card and summary, a real lockdown alert copy, a real captcha challenge and a real log post at the shortest preset, advances a day and a half and asserts no row and no delete request; a source scan asserts no file of those families calls a deletion producer; `TestMemberDeletionStaffGroupNeverTouched`, `TestMemberEditedStaffGroupNeverTouched`, `TestCommandDeletionStaffGroupNeverTouched` (passed) |
| 5 | The Member messages section is wired into the Deletion page by table entries only, reachable from Main, with the one card for Self-destruct, rights and the delete-right line | VERIFIED | `deletionMemberRows` and section `m` (icon 💬) registered in the page tables; `git diff` of `settings_pages.go`, `settings_menu.go`, `settings_card.go` against the commit before Phase 17 is empty. Phase 16 screens are byte-identical (`TestSettingsDeletionPhase16ScreensUnchanged`, golden digests, passed). Tests passed: `TestSettingsDeletionMemberOpens`, `...Writes`, `...Card`, `...Navigation`, `...Stale`, `...StaffGroup`, `...CodesUnique`, `TestSettingsAuthorityDeletionMember` (change-info right on every choice and on the card's Yes), `TestSettingsDriftDeletionMember` (a stored 45 s reads Off), `TestMemberKindsNeedDeleteRight` |
| 6 | DEL-07 for the member rules: unknown never deletes, a Staff Group and every non-group chat are never touched | VERIFIED | All three paths end in `deletionAct` (command, edit) or in the batcher screen plus the worker's Staff Group drop (self-destruct); `deletionAct` refuses a zero or positive chat ID, a latched chat and a Staff Group or an unreadable Staff Group row. Approval, admin and captcha reads that fail keep the message (tri-state). Tests above plus `TestMemberDeletionLogsCarryIDsOnly` (no text, name or message ID in any line) |
| 7 | PLAT-06: the batcher and the member kinds add no shutdown drain, share the paced deletion budget, recover from panics, and stop inside the existing 3 s wait | VERIFIED | `StartDeletionWorker` starts the batcher (`deletion_worker.go:80`); `StopDeletionWorker` sets the draining flag, asks for one final flush bounded to 2 s on a detached context and waits for worker and batcher under the one `deletionWorkerStopWait` timer (`:94-129`); `main.go:230` registration unchanged, so the 68 s arithmetic and `stop_grace_period: 75s` stand. Every goroutine starts with `RecoverFromPanic` (`member_deletion_batcher.go:377,387,419`, `member_deletion.go:157`). `TestShutdownEndsMemberDeletionBatcher` passed |
| 8 | PLAT-05: every new string exists in all 8 locales and AGENTS.md reflects it | VERIFIED | 330 `settings_` keys in each of en, es, fr, hi, id, pt, ru, uk; `make check-translations` exit 0 ("All translations are present"); `./fuku/i18n` passed (limits tables, `TestDeletionKindKeysExist`); AGENTS.md documents command deletion, `MemberDeletion`, the batcher, the shutdown addition, the -8 group and the Deletion page section |
| 9 | No migration and no dependency change; the DEL requirements ticks are only the three this phase completes; ROADMAP is not marked Complete | VERIFIED | Newest migration is still `20261110120000_add_scheduled_deletion.sql` (Phase 16); `git diff --exit-code fe84974 -- go.mod go.sum` exit 0 (fe84974 is the commit before the first Phase 17 commit; the instructed `8c0c827` base also exits 0); the REQUIREMENTS.md diff against fe84974 changes exactly DEL-03, DEL-04, DEL-05 (checkbox and traceability row); ROADMAP line 79 is `[ ]` and the progress row reads "12/12 In Progress" |
| 10 | The Makefile PostgreSQL integrity list names the four new repository tests and the new packages | VERIFIED | `Makefile` list gains `TestScheduleBatchKeepsEarliest`, `TestScheduleBatchDuplicateKey`, `TestCountPending`, `TestDropPendingMessage`; each exists as a top-level test in `fuku/db/deletion/queue_batch_test.go` (lines 43, 157, 365, 403); `./fuku/db/memberreq ./fuku/db/deletion` added to the package list; `./fuku/db/deletion` passed on SQLite |

**Score:** 10/10 truths verified (0 present, behavior-unverified). The two behavior-dependent truths (the post-answer ordering of truth 1 and the off-switch/pin races of truth 3) each have a passing behavioral test. Truth 2 carries one warning on the word "default" (human item 3). What needs live Telegram or two processes is listed below.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `fuku/modules/command_deletion.go` | Processor wrapper, matcher mirror | VERIFIED | 193 lines, wired in `main.go:431` |
| `fuku/utils/helpers/command_registry.go` | `NewBotCommand`, `IsBotCommand` | VERIFIED | the one construction site (grep of `handlers.NewCommand(` in non-test code finds only line 28); `TestNoRawNewCommand` in `fuku/utils/helpers` passed |
| `fuku/modules/member_deletion.go` | edit handler, self-destruct watcher, exemptions, pin drop | VERIFIED | 566 lines, registered `RegisterLegacyModule("MemberDeletion", 243, ...)` |
| `fuku/modules/member_deletion_batcher.go` | bounded batcher | VERIFIED | 459 lines, started and stopped with the worker |
| `fuku/db/deletion/{kinds,queue,rules}.go` | member kinds, `ValidateRuleDelay`, `ScheduleBatch`, `CountPending`, `DropPendingMessage` | VERIFIED | present, tested (`./fuku/db/deletion` ok) |
| `fuku/modules/deletion_kinds.go`, `settings_deletion.go` | Member messages rows and section | VERIFIED | rows `mc me ms`, section `m` |
| `fuku/modules/pins.go` | `/pin` hook | VERIFIED | `memberDeletePinned` at line 338 |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `main.go` dispatcher | `CommandDeletionProcessor` | `ext.DispatcherOpts.Processor`, outside `tracing.TracingProcessor` | WIRED | `main.go:431`; polling-context test `TestCommandDeletionPollingContext` passed |
| `RegisterLegacyModule` | `LoadMemberDeletion` | priority 243 | WIRED | `member_deletion.go:564`; both handlers added at group -8 |
| `StartDeletionWorker` | `memberDeleteStart` | direct call | WIRED | `deletion_worker.go:80`, called from `main.go:516` |
| `StopDeletionWorker` | `memberDeleteStop` | before the wait | WIRED | `deletion_worker.go:96`, registered at `main.go:230` |
| watcher | batcher | `memberDeleteOffer` | WIRED | never calls `deletionAct`, `deletionSchedule` or the database |
| batcher | queue | `deletion.ScheduleBatch` via `memberDeleteBatchWrite` | WIRED | Staff Group refusal inside the writer, latch and ceiling before it |
| `/pin` and `pinned_message` | queue | `memberDeletePinned` -> `deletion.DropPendingMessage` | WIRED | pins.go:338, member_deletion.go:531 |
| Deletion page | rules repository and cache | `applyDeleteRule`, `deletion.SetRule` with `cache.DeleteCache` | WIRED | `delete_rules:*` in `skipLocal` from Phase 16 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| command wrapper | rule on | `deletionRulesLookup` (cached `chat_delete_rules`), fresh context | yes | FLOWING |
| edit handler | grace | same read, presets only | yes | FLOWING |
| watcher and batcher | delay, exemptions, ceiling | rules read, approvals (`IsUserApprovedContext`), admin cache and live `getChatMember`, `CountPending` | yes | FLOWING |
| worker | due rows | `ClaimDue` (1000 per cycle) | yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Production build | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Phase 17 module tests (command, edit, self-destruct, batcher, busy group, DEL-07, shutdown, page member rows, golden screens, validation edges, worker screens) | `go test -tags testtools -race -count=1 -run '^(TestBotCommandNameMatchesGotgbot\|TestCommandDeletion.*\|TestCommandRepliesStayInsideTheHandler\|TestMember(Edited\|Deletion\|SelfDestruct\|Kinds\|Content\|Delete).*\|TestSelfDestructBusyGroup\|TestShutdownEndsMemberDeletionBatcher\|TestSettings(Authority\|Drift)?DeletionMember.*\|TestSettingsDeletionMember.*\|TestSettingsDeletionPhase16ScreensUnchanged\|TestValidation(Command\|Edit\|SelfDestruct).*\|TestDeletionWorker(ManyChats\|ScreensSelfDestructOff\|ScreenLogThrottled))$' ./fuku/modules` | `ok ... 78.722s` | PASS |
| Repositories, caches, backup, migrations, locales, helpers | `go test -tags testtools -race -count=1 ./fuku/db/deletion ./fuku/db/cache ./fuku/db/backup ./fuku/db/migrations ./fuku/i18n ./fuku/utils/helpers` | all `ok` | PASS |
| Vet | `go vet -tags testtools ./fuku/modules` | no output | PASS |
| Docs drift | `make check-docs` | "No drift detected" | PASS |
| Translations | `make check-translations` | "All translations are present" | PASS |
| Dependencies untouched | `git diff --exit-code fe84974 -- go.mod go.sum` and `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 both | PASS |
| No migration added | `ls migrations \| tail -1` | `20261110120000_add_scheduled_deletion.sql` (Phase 16's) | PASS |
| Debt markers | grep `TBD\|FIXME\|XXX\|TODO\|HACK` in the Phase 17 production files | none | PASS |

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh` scripts.

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
| ----------- | ------------ | ----------- | ------ | -------- |
| DEL-03 | 17-01 to 17-04, 17-11 | Delete commands, admins' included, after the answer | SATISFIED | Truth 1; the tick in REQUIREMENTS.md is true |
| DEL-04 | 17-05, 17-11 | Delete edited messages after the grace, admins and approved exempt | SATISFIED in behaviour; wording "1 minute by default" is only a preset (truth 2, human item 3) | Truth 2; the tick is true for the behaviour, owner to confirm the default wording |
| DEL-05 | 17-06, 17-07, 17-11 | Self-destruct after N minutes, admins, approved and pinned exempt | SATISFIED | Truths 3 and 4; the tick is true |
| DEL-07 (binding) | 17-07, 17-11 | Never touches staff items, lockdown alerts, captcha challenges, log posts, a Staff Group | SATISFIED | Truth 6 |
| DEL-08 (binding) | 17-04, 17-09, 17-10 | Right-to-delete line on deletion pages | SATISFIED | Truth 5 (`TestMemberKindsNeedDeleteRight`, the shared line on member screens); live check is SPIKE 13 |
| PLAT-05 (binding) | 17-08 | 8 locales and AGENTS.md | SATISFIED | Truth 8 |
| PLAT-06 (binding) | 17-06 | Shared budget, crash recovery, drained inside the existing budget | SATISFIED | Truth 7 |

No orphaned requirement: REQUIREMENTS.md maps exactly DEL-03, DEL-04 and DEL-05 to Phase 17.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `17-SPIKE.md` (A17-4) and `17-12-SUMMARY.md` | 240, 104 | Claim that the 1-minute default "exists in the apply function" | Warning (documentation, not code) | No default exists in any Go file (grep finds none) and D-30 says "no implicit 60 s default anywhere on the page". The owner reading A17-4 would confirm something the code does not do. Correct the sentence when the owner answers |

Accepted limits recorded in `17-REVIEW-DISPOSITION.md` and AGENTS.md (each fails toward keeping the message): a claimed and screened batch can finish within the 90 s cycle after Off (AR-9), a pin handled by another replica can still miss the in-process set (AR-5), an anonymous admin's command stays in a group whose AnonAdmin setting is on (D-34), the bare-name race with another bot, one baseline-only admin warm per chat per cold cache. All review findings (WR-01 to WR-03, IN-01 to IN-03) and security findings (SEC-17-01 to 05) are marked fixed in the disposition, and the fix commits are in the history.

### Human Verification Required

Everything automatable passed. The items below are the live checks and owner decisions the standing policy defers; each is in `17-SPIKE.md` under the number shown. None changes what the code does when Telegram answers unexpectedly (it keeps the message).

1. **Edit noise** (SPIKE 1, A17-3): what Telegram reports as an edit in a live group.
2. **Bare `/word` in a multi-bot group, and the anonymous-admin flows** (SPIKE 2, 3, 4; A17-1, A17-2, D-34).
3. **The "1 minute by default" grace** (SPIKE 15, A17-4): owner decision on whether the 1-minute preset must be marked or preselected, plus a correction of the SPIKE and SUMMARY sentence that says an apply-function default exists.
4. **Pins seen live** (SPIKE 5).
5. **`deleteMessages` at volume and a claimed batch at Off** (SPIKE 6, 7).
6. **Phone-width cells in hi, ru, uk; native-speaker read of the 18 new strings** (SPIKE 8, 9).
7. **Two replicas, a restart, a plain-member bot, the right line live, a cold admin cache** (SPIKE 10 to 14).
8. **Owner confirmations A17-1 to A17-8, A17-A3, A17-A5 and amendments D-21 to D-34** (SPIKE 15, 16).
9. **DEL-07 live with all three rules on** (SPIKE 17).
10. **CI lint, full test run and the PostgreSQL integrity target on the pushed head** (SPIKE 19).

### Gaps Summary

No gap blocks the goal. The one finding worth the owner's attention is the roadmap's "1 minute by default" for the grace: the code offers 1 minute as a preset and preselects nothing (a documented decision, D-30), while the SPIKE and the 17-12 summary describe a default in the apply function that is not there. It is routed to human verification, not marked failed, because the owner decision A17-4 is still open and the adjustable grace itself works.

### What was not run

- `make test` and the whole `fuku/modules` package, by instruction. The executors' whole-package runs were ok at 1521 s, 1615 s and 1585 s; this verification ran the Phase 17 families (78.7 s) and the repository, cache, backup, migration, locale and helper packages.
- `make lint`, `make test-postgres-integrity` and the Turnstile page script test: not runnable here or not touched by this phase.

---

_Verified: 2026-11-12T15:00:00Z_
_Verifier: Claude (gsd-verifier)_
