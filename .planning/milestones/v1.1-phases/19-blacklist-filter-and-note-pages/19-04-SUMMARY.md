---
phase: 19-blacklist-filter-and-note-pages
plan: 04
subsystem: settings-menu-typed-reply-door
tags: [telegram, settings-menu, typed-reply-door, list-engine, blacklist, wait-store, redis-lua]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-01's applyBlacklistAdd and fresh readers, 19-02's ninth right and its bot halves, 19-03's list engine and Blacklist page
provides:
  - the door's list branch (runList, authorize(right), botCheck with live classification), editor path unchanged
  - the Blacklist typed add (Add words bn, Cancel bc, settingsBlacklistTyped) stored through applyBlacklistAdd
  - 16 settings_ strings in 8 locales (420 settings_ keys)
  - the wait store's w and q fields and the settingsInputAdvance / settingsInputSetPrompt scripts (M41), proved at the store level
affects: [19-05, 19-06, 19-07, 19-08, 19-09, 19-10, 19-11, 19-12]

plan_head_before: 3f7de39949a9443c41eca0cffa986771d72a6ecc
plan_head_after: b9286cc384d51536d13f58158340542725267650

actuals:
  tokens: 23700   # chars/4 over the added lines of 3f7de39..b9286cc outside .planning (94667 chars, locales and AGENTS.md included)
  tasks: 4
  commits: 4      # MEASURED: git rev-list --count 3f7de39..b9286cc (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "a wait whose page is a registered list page is run by the list engine (runList); the editor path through the door is untouched"
    - "authorize(right) runs settingsRightBotFirst, the member part, settingsRightBotAfter(typed=true) exactly as a press does, with each bot refusal classified by one live getChatMember of the bot"
    - "the wait hash gains one bounded user-text field w only on a step-2 list part, never logged"

key-files:
  created:
    - fuku/modules/settings_list_input.go
    - fuku/modules/settings_list_door_test.go
  modified:
    - fuku/modules/settings_input.go
    - fuku/modules/settings_input_door.go
    - fuku/modules/settings_input_test.go
    - fuku/modules/settings_list.go
    - fuku/modules/settings_blacklist.go
    - fuku/modules/settings_blacklist_test.go
    - fuku/modules/settings_list_test.go
    - fuku/modules/settings_list_locale_test.go
    - fuku/i18n/settings_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - AGENTS.md

key-decisions:
  - "The Add button is bn and Cancel is bc (the pager is bp): bg and ba belong to the Deletion page's Bot goodbye and Bot auto rows."
  - "settingsInputAdvance also compares the wait's c and p in the script (two extra ARGV), so an advance can only move a wait of the same group and page; the plan listed only the nonce and the claim."
  - "A typed reply for the authority-parity test writes its wait directly (settingsInputSet), because the Add press is itself refused whenever the records refuse; the door then reads the same records live."

patterns-established:
  - "settingsListDoor* helpers (settingsListDoorWait, settingsListDoorSay, settingsListDoorReply, settingsListDoorClearAdminCache) in settings_list_door_test.go"
  - "scripted getChatMember 429 sequences for the ninth right: the person's lookup, the cache-miss load and its retry, then the door's live bot check"

requirements-completed: []

coverage:
  - id: D1
    description: "the owner presses Add words, types words in the bot's private chat, and every addable word is stored through applyBlacklistAdd exactly as /addblacklist stores it; the saved page arrives as a new message with counts only"
    requirement: "PAGE-07 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorOneStep, TestSettingsParityBlacklistAdd"
        status: pass
    human_judgment: false
  - id: D2
    description: "a typed reply is judged like a press (live member, owner gate, the ninth right with live bot classification, live getChat); refusals keep the wait, losses end it, the album, Cancel, /cancel, expiry and no Redis behave as for the editor"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: unit
        ref: "TestSettingsListDoorRefusals, TestSettingsListDoorAuthority, TestSettingsListDoorReplaced, TestSettingsListDoorExpired, TestSettingsListDoorCancel, TestSettingsListDoorAlbum, TestSettingsListDoorNoRedis, TestSettingsAuthorityParityBlacklist (typed column)"
        status: pass
    human_judgment: false
  - id: D3
    description: "the editor's path through the door is byte-identical: every Phase 11 and 12 door test passes unedited"
    verification:
      - kind: unit
        ref: "git diff --exit-code d93ec1d -- settings_input_door_test.go settings_input_media_test.go settings_input_rules_test.go exits 0; TestSettingsEditTyped*, TestSettingsEditDoor*, TestSettingsEditAlbum*, TestSettingsEditRulesTyped*, TestSettingsInput*"
        status: pass
    human_judgment: false
  - id: D4
    description: "the wait store keeps a bounded step-1 keyword (w) and moves a wait to step 2 atomically (advance, set-prompt), proved at the store level"
    requirement: "PAGE-08 / PAGE-09 (groundwork only)"
    verification:
      - kind: unit
        ref: "TestSettingsInputAdvance, TestSettingsInputSetPrompt, TestSettingsInputListParts, TestSettingsInputStore, TestSettingsInputRead, TestSettingsInputCancelOutcomes"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 04: the typed-reply door's list branch and the Blacklist typed add Summary

**The owner presses Add words on the private Blacklist page and types words in the bot's private chat; the door judges the message like a press and stores each addable word through the command's own apply function, answering with counts only. The wait store is ready for the two-step adds of Filters and Notes.**

## What was built

- **Task 1, the tracer (`dfa54ef`, by the previous executor).** `settings_list_input.go`: the static part table `settingsListParts` (`(b,w) (i,k) (i,y) (e,n) (e,c)`), `settingsListPartByCode`, the Add and Cancel actions of the engine, `renderSettingsListPrompt`, `renderSettingsListRefusal`, `runList`. `settings_input_door.go`: the list branch at the top of `run` (the editor lookup below it unchanged), `authorize(right)` (bot first half, member part, bot second half with `typed` true, then the live `getChat`), `botCheck(key)` generalising the old bot-admin classification, `renderSettingsInputRefusalCode`. `settings_blacklist.go`: codes `bn bc`, `settingsBlacklistTyped` (words from `strings.Fields`, lower-cased, de-duplicated, pre-checked against a fresh read, claim, `applyBlacklistAdd`, finish, the saved page with counts only, the prompt closed). Nine strings.
- **Task 2 (`0c9317f`, this executor adopted the previous executor's uncommitted work).** Seven strings (`settings_entries_wait_ended_note`, the five `settings_entries_part_*` words, `settings_bl_refuse_none`) with their limit rows, and the seven door behaviors in `settings_list_door_test.go`. The production code for the refusal, failure and expiry branches was already in Task 1's commit; the tests exposed three faults in the tests themselves, fixed here: the admin-cache helper dropped the wrong key (it now calls `cache.InvalidateAdminCache`), the scripted `getChatMember` 429 sequences did not account for the cache-miss load, its retry and then the door's live bot check (`nil, 429, 429, 429` for the refused 429 case and `nil, 429, 429, nil` for the stale-predicate case), and `TestSettingsListDoorReplaced` pressed Welcome Set as an administrator without `can_change_info` (it now uses `newOwner`, and opens the second Blacklist menu as the same owner).
- **Task 3, the ship point (`a0db8ee`).** `TestSettingsParityBlacklistAdd` (typed `Spam scam` and `a spam` against `/addblacklist` on twin chats: same rows with action and reason, shared action `ban` per A19-3, the one-character word refused by both per A19-2, the cache key dropped on both) and the typed column of `TestSettingsAuthorityParityBlacklist` (the bot a member, the bot without restrict answering `chat_status_bot_restrict_group_error`, a member without restrict, a plain member, the owner-gate carve-out, and the all-held case that writes). AGENTS.md: 420 keys and the door test families.
- **Task 4 (`b9286cc`).** `settings_input.go`: `settingsInputWait.Pending` (`w`) and `StepMsgID` (`q`), `settingsInputPendingValid` (valid UTF-8, not blank, at most 100 code points), `settingsInputWaitFromFields` reads them only for a step-2 list part and refuses such a wait without a valid `w` and a positive `q`, `settingsInputAdvanceScript` / `settingsInputAdvance`, `settingsInputSetPromptScript` / `settingsInputSetPrompt` (nine Lua scripts in all). AGENTS.md: the wait's Data bullet names `w`, `q`, both scripts and the provisional confirmation A19-6. **`settingsInputAdvance` and `settingsInputSetPrompt` have no production caller yet: plan 19-06 is the first.** Their only users now are the store-level tests of this plan, and the task was kept last so the typed blacklist add shipped first.

## Verification (real results)

- Task gates, bare and under the CI environment (`cienv.sh`): `TestSettingsListDoor*` (8 families), `TestSettingsListKeysExist`, `TestSettingsParityBlacklistAdd`, `TestSettingsAuthorityParityBlacklist`, the Phase 11 and 12 door families (`TestSettingsEditTyped*`, `TestSettingsEditDoor*`, `TestSettingsEditAlbum*`, `TestSettingsEditRulesTyped*`, `TestSettingsEditRefusalKeepsWait`, `TestSettingsEditWriteFailureReleases`, `TestSettingsEditWaitAcrossReplicas`, `TestSettingsEditWaitExpiry`, `TestSettingsEditMarkerReplyOnce`, `TestSettingsEditDoubleDelivery`, `TestSettingsEditWholeMessageParity`, `TestSettingsEditCancelCommand*`, `TestSettingsCancelInGroupPassesOn`) and `TestLegacyBlacklistRequests`: all `ok` (the door family run took 25 s). Store families `TestSettingsInputAdvance`, `TestSettingsInputSetPrompt`, `TestSettingsInputListParts`, `TestSettingsInputStore`, `TestSettingsInputSetRefusesInvalidWait`, `TestSettingsInputCancelOutcomes`, `TestSettingsInputRead`, `TestSettingsInputAlbumAndMarker`, `TestSettingsInputWithoutRedis`: all PASS. `./fuku/i18n` `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`: ok. `make check-translations`: "All translations are present". Each locale has 420 `settings_` lines.
- Invariants: `git diff --exit-code d93ec1d -- fuku/modules/settings_input_door_test.go fuku/modules/settings_input_media_test.go fuku/modules/settings_input_rules_test.go` exits 0 (no Phase 11 or 12 door test edited); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `grep -c 'redis.NewScript' fuku/modules/settings_input.go` prints 9; `grep -cE 'log\.[A-Za-z]+\(.*Pending' fuku/modules/settings_input.go` prints 0; 8 `TestSettingsListDoor*` functions; no `t.Skip` in the new tests.
- Gates: `go vet -tags testtools ./fuku/modules ./fuku/i18n` clean; `gofmt -l` over the files this plan created or edited prints nothing (it lists pre-existing unformatted files this plan never touched: `blacklists_command_test.go`, `callback_codec.go`, `deeplink_router.go`, `error_logging.go`, `formatting.go`, `help.go`, `overwrite.go`); `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827"); `make lint`, `gosec` and `make test` were not run (the sandbox cannot run them, see the brief).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-04.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1827.198s`, last line `exit 0`, 1795 top-level PASS (19-03's 1783 plus this plan's 12), 0 FAIL, 0 SKIP. `--- PASS: TestSettingsListDoorOneStep (0.35s)` and `--- PASS: TestSettingsEditTypedTextSaved (0.38s)` are in it.
- Settings family under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`): `./fuku/i18n` `ok 7.565s`: `ok fuku/modules 732.772s`, `ok fuku/i18n 7.546s`, `exit 0` (log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/family-19-04.log`).

## Time added to the whole-package run

About 4.5 s: the eight `TestSettingsListDoor*` tests 0.30 to 0.50 s each (3.0 s in all, each mostly environment setup), `TestSettingsParityBlacklistAdd` 0.45 s, the typed column added about 0.04 s to `TestSettingsAuthorityParityBlacklist` (0.53 s), `TestSettingsInputAdvance` 0.35 s, `TestSettingsInputSetPrompt` 0.31 s, `TestSettingsInputListParts` under 0.01 s. No new test is over 1 s.

## Deviations from Plan

### Execution notes

**1. [Process] The whole `fuku/modules` package ran once, after Task 4, not at Task 3's ship point.** The plan asks for it at Task 3. A container restart had already cost an executor, so every task was committed first and the single 30 minute run covers the final tree (all four tasks), which subsumes a Task 3 run. The Task 3 gate (the focused door, parity and editor families) ran at Task 3.

**2. [Rule 1 - Bug] Three test faults of the previous executor's uncommitted Task 2 tests**, found by running them (see Task 2 above): the admin-cache helper, the scripted 429 sequences and the Replaced test's actor. No production change was needed.

**3. [Design, within the task's intent] `settingsInputAdvance` compares `c` and `p` in the script** (two extra ARGV beyond the plan's list), so an advance can never move a wait of another group or page; `TestSettingsInputAdvance` covers it ("another group changes nothing").

**4. [Test design] The typed column of the authority-parity test writes its wait directly** with `settingsInputSet` and not through the Add press, because a press is refused whenever the records refuse, so no wait would exist to test. The door then reads the same member and bot records live.

None of these changes behaviour the plan describes. Auth gates: none.

## Known Stubs

None. `settingsInputAdvance` and `settingsInputSetPrompt` are complete and tested; plan 19-06 is their first production caller (the two-step Filters and Notes adds).

## Threat Flags

None new. T-19-16 (typed reply after losing a right), T-19-17 (no word in a log or callback), T-19-18 (`w` bounded, only on step 2, deleted with the hash), T-19-19 (nonce-checked claim and advance, album gate) and T-19-21 (editor path unedited tests) are covered as the plan's register says; `grep` finds no log line naming `Pending`.

## Requirements

None completed: `requirements-completed: []`. PAGE-07 (Blacklist add by typing is now done, but Filters and Notes are later plans), PAGE-10 and PLAT-05 remain Pending and are not ticked.

## Self-Check: PASSED

- Files exist: `fuku/modules/settings_list_input.go`, `fuku/modules/settings_list_door_test.go`.
- Commits reachable from HEAD: `dfa54ef`, `0c9317f`, `a0db8ee`, `b9286cc`.
