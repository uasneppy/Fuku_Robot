---
phase: 19-blacklist-filter-and-note-pages
plan: 11
subsystem: settings-menu-list-pages-e2e
tags: [telegram, settings-menu, list-engine, end-to-end, tests]
status: complete

requires:
  - phase: 19-blacklist-filter-and-note-pages
    provides: 19-03 to 19-10, the list engine, the Blacklist, Filters and Notes pages, their apply functions, the typed-reply door and the cross-page sweeps
provides:
  - TestSettingsPhase19EndToEnd (subtests blacklist, filters, notes, staff group, no redis, two replicas) and its toolkit phase19E2E
affects: [19-12]

plan_head_before: 6bb4c55094ee713808ff6e49e8b94f4d822c767e
plan_head_after: 9217d79

actuals:
  tokens: 9300    # chars/4 over the added lines of 6bb4c55..9217d79 outside .planning (about 37000 chars)
  tasks: 3
  commits: 3      # MEASURED: git rev-list --count 6bb4c55..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "an end-to-end subtest presses only buttons the bot drew and sends only messages through the real dispatcher"

key-files:
  created:
    - fuku/modules/settings_phase19_e2e_test.go
  modified:
    - AGENTS.md

key-decisions:
  - "Test-only plan: no production file changed; no subtest exposed a defect."
  - "One owner (a live creator, settingsFiltersOwner) works all three pages: the creator passes every right (restrict, change info) without a per-page fixture, and env.admin stays the exempt administrator of the blacklist subtest."

patterns-established:
  - "phase19E2E: the list settings environment plus Filters, Notes and Warns on its dispatcher; member, approvedMember, openList, tap (by marker), deleteEntry, typeInPrivate, post, deleted, warned, sentTo"

requirements-completed: []

coverage:
  - id: E1
    description: "a word added from the Blacklist page (Add, typed in private) is deleted and warned once at a member's next message in the group, an admin's and an approved member's messages stay, Delete only on the action screen deletes with no new warn, and a page delete stops the deletion"
    requirement: "PAGE-07 (evidence only; 19-12 completes it)"
    verification:
      - kind: e2e
        ref: "TestSettingsPhase19EndToEnd/blacklist (mutation check: removing the blacklist cache drop in RemoveBlacklistByID and SetBlacklistActionContext fails it on the extra warn and the message deleted after the word's delete)"
        status: pass
    human_judgment: false
  - id: E2
    description: "a filter added by two typed messages answers a member's message with the stored reply and its Shop button and stops after the page deletes it"
    requirement: "PAGE-08 (evidence only; 19-12 completes it)"
    verification:
      - kind: e2e
        ref: "TestSettingsPhase19EndToEnd/filters"
        status: pass
    human_judgment: false
  - id: E3
    description: "a note added from the Notes page is sent by #name and by /get name, and after the page deletes it /get answers notes_does_not_exist and #name sends nothing"
    requirement: "PAGE-09 (evidence only; 19-12 completes it)"
    verification:
      - kind: e2e
        ref: "TestSettingsPhase19EndToEnd/notes"
        status: pass
    human_judgment: false
  - id: E4
    description: "the pages open and write for a group that is a Staff Group without naming it, work without Redis (Add is the alert, list, action and delete Yes work and the watcher follows) and across replicas (a wait made on one is completed on another, the word acts on the second once its local layer expired)"
    requirement: "PAGE-10 (evidence only; the Staff Group privacy and no-Redis behaviour)"
    verification:
      - kind: e2e
        ref: "TestSettingsPhase19EndToEnd/staff group, /no redis, /two replicas"
        status: pass
    human_judgment: false
---

# Phase 19 Plan 11: the end-to-end proof Summary

**Test-only plan. Through the real dispatcher with only Telegram faked, each list page is proved to change what the bot does in the group: the blacklisted word is acted on (and an admin's and an approved member's messages stay), the filter answers, the note is fetched by `#name` and `/get`, each stops after a page delete, and the same holds for a Staff Group, without Redis and across two replicas. No production code changed.**

## What was built

- **Task 1, the tracer (`1e4d8a0`).** `settings_phase19_e2e_test.go` with `TestSettingsPhase19EndToEnd` and the toolkit `phase19E2E` (the list settings environment, the Filters, Notes and Warns modules on its dispatcher, a live creator who works the private menu; `member`, `approvedMember`, `openList`, `tap` by locale marker, `deleteEntry`, `typeInPrivate`, `post`, `deleted`, `warned`, `sentTo`) and the `blacklist` subtest. The owner presses Add on the Blacklist page and types `spam`; the stored word is on warn (A19-3 on an empty list). A member's `buy SPAMMER now` is deleted once and the member has one warn; an admin's and an approved member's `spam` messages are not deleted and nobody of them is warned. On a fresh menu the owner presses Set action and Delete only: the word is stored on `none`, the member's next message is deleted and the warn count stays 1. The owner deletes the word through `🗑 1` and the card's Yes: nothing is stored and the member's next message stays.
- **Task 2 (`4ca77b9`).** Subtests `filters` and `notes`. Filters: Add, the keyword `price`, then a text reply with a markdown button; a member's `what are the Prices?` gets exactly one reply carrying the stored text and exactly one button, `Shop`; after the page deletes the filter, `prices?` gets no reply. Notes: Add, the name `rules`, then the text; `#rules` and `/get rules` each send the note; after the page deletes it `/get rules` is answered `notes_does_not_exist` once and `#rules` sends nothing.
- **Task 3 (`9217d79`).** Subtests `staff group`, `no redis` and `two replicas`, and the AGENTS.md Testing bullet. Staff group: the env chat becomes a Staff Group (title `OpsCrewZeta`); its private menu opens the three list pages, none of their texts nor the saved page contains "staff" or the title, and a word added there is deleted and warned in that group. No Redis: Add answers the alert `settings_entries_add_unavailable`, edits nothing and leaves no wait key; the list draws, the action screen sets `none`, a member's message with the word is deleted (the watcher works without Redis), the card's Yes deletes the word and the next message stays. Two replicas: Add is pressed on replica A, the typed word goes to replica B (`otherReplica`, its own dispatcher and fake with the owner as creator and the bot an administrator), the word is stored, and after `dbcache.ResetLocalForTest()` a member's message on B is deleted by B and not by A.

## Requirement evidence (none ticked)

`requirements-completed: []`, nothing ticked in REQUIREMENTS.md; 19-12 is the only plan that ticks PAGE-07 to PAGE-09. Evidence: `blacklist` for PAGE-07, `filters` for PAGE-08, `notes` for PAGE-09, and `staff group`, `no redis` and `two replicas` for the cross-cutting privacy and availability behaviour of PAGE-10 and all three pages.

## Verification (real results)

- Focused, bare (no `BOT_TOKEN`) and under `cienv.sh`: `go test -tags testtools -race -count=1 -run '^TestSettingsPhase19EndToEnd$' ./fuku/modules` ok each time at every commit (task 1: the `blacklist` subtest; task 2: all four subtests; task 3: all six). Verbose: `--- PASS: TestSettingsPhase19EndToEnd (2.48s)`, subtests 0.37 to 0.45 s each, no `--- FAIL`, no `--- SKIP`.
- Mutation check: removing the `cache.DeleteCache` of `blacklists.RemoveBlacklistByID` and `SetBlacklistActionContext` made `TestSettingsPhase19EndToEnd/blacklist` fail with "Delete only added a warn: the member has 2, want 1" and "a message after the word was deleted: the message was deleted 1 times, want 0"; the file was restored with `git checkout -- fuku/db/blacklists/repository.go` (`git status` clean for it).
- Whole `fuku/modules` package under the CI environment (`go test -tags testtools -race -count=1 -timeout 40m -v ./fuku/modules`, background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-19-11.log`): `ok  github.com/uasneppy/fuku_robot/fuku/modules  1971.340s`, last line `exit 0`, 1841 top-level PASS (one more than the 1840 of 19-10: exactly the new test, so the settings family ran inside it), 0 `--- FAIL`, 0 `--- SKIP`, 0 `^FAIL`; `--- PASS: TestSettingsPhase19EndToEnd (2.22s)` appears once; `TestSettingsGateAllDoors (16.78s)` and `TestSettingsStaffPrivacy (25.25s)` are unchanged within noise.
- Gates: `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` on the new file prints nothing; `lint-local.py 8c0c827` exits 0 ("no dupl, gocyclo or godox findings on lines added since 8c0c827", read from the exit code); `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no `t.Skip`, `reflect.Ptr` or `ioutil` in the new file. Grep acceptance counts: `t.Run("(filters|notes)"` 2, `t.Run("(staff group|no redis|two replicas)"` 3, `TestSettingsPhase19EndToEnd` in AGENTS.md 1.
- Not run: `make lint`, gosec and `make test` (the sandbox cannot run lint; `make test` is 19-12's). The `fuku/modules` harness does not run on PostgreSQL here, so every subtest ran on SQLite.

## Time added to the whole-package run

About 2.3 s (`TestSettingsPhase19EndToEnd` 2.22 s in the whole-package log, 2.48 s focused; the plan estimated 4 s and 2 s per subtest at most; each subtest is at most 0.45 s). The whole-package run measured 1971 s against 1989 s for 19-10, which is machine noise. Phase 19's own added time stays inside D-12's 40 s budget as far as the per-plan reports go (19-10: about 6 s, 19-11: 2.3 s).

## Deviations from Plan

**1. [Facts newer than the plan text] Registered codes and names.** The Blacklist Add and pager codes are `bn` and `bp`, the pages' keys `b`, `i`, `e`; the test takes every button from the drawn keyboard by its locale marker or its exact `🗑 N` label, so no code is written in the test. The plan's helper `typeInPrivate` takes text (the step-2 markdown button is part of the text).
**2. [Fixture] The page owner is a live creator** (`settingsFiltersOwner`), not `env.admin`, because the administrator lacks `can_change_info` (Filters and Notes rights) and is the exempt administrator the Blacklist subtest needs.
**3. [Structure] The three tasks were committed from staged versions of one file** (blacklist only, then plus filters and notes, then the whole file with AGENTS.md); a container restart could not lose a task.

No production defect was exposed (none), no auth gates.

## Known Stubs

None.
