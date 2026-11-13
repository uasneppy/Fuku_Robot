---
phase: 18-remaining-settings-pages
plan: 08
subsystem: settings-menu
tags: [telegram, log-channel, golden, apply-function, reader, docs]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-07 (the Language page and the reader-and-apply pattern this plan follows)"
provides:
  - "TestLegacyLogCategoryRequests: 25 cases pinning /log and /nolog (a private chat, a non-admin, no linked channel, no argument, unknown categories alone and next to a known one, one, several and repeated categories, mixed case, one of several failing to save, every save failing, /nolog settings) with every sendMessage request, the #SETTINGS audit post to the log channel and the stored flags, committed alone before any production edit"
  - "applyLogCategory, logCategoryWrites, logApplyResult and the outcomes logApplied (zero value), logUnknownCategory, logNotSet, logSaveFailed in logchannels.go; /log and /nolog keep their loop, pre-checks, quirk and audit post and call it once per category"
  - "logchannels.LoadContext: the log_channel:<chat> key GetContext reads (one shared loader), (nil, nil) for a chat with no linked channel, the error for a failed read, nothing created and nothing cached for a miss"
affects: [18-09, 18-15]

plan_head_before: ac274a233ffb4d647f95aae5c6677ec73d518472
plan_head_after: f19ffbaf748e3a50f4d5c5b22fd525d40df4fdc2

actuals:
  tokens: 10500   # chars/4 over the added non-planning lines of ac274a2..f19ffba (41925 chars), tests and docs included
  tasks: 2
  commits: 3   # MEASURED: git rev-list --count ac274a2..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/logchannels_legacy_request_test.go
    - fuku/modules/logchannels_apply_test.go
    - fuku/db/logchannels/load_context_test.go
  modified:
    - fuku/modules/logchannels.go
    - fuku/db/logchannels/repository.go
    - AGENTS.md

key-decisions:
  - "The commands' quirk is kept, not fixed (C-06): a save error other than 'not linked' is logged and the loop goes on, so a total failure replies logs_enabled (or logs_disabled) with an empty list and posts the audit line, whose text is the raw key plus the empty list, to the log channel. The golden pins it."
  - "applyLogCategory logs a failed save itself (log.Errorf with the chat ID and the error, as the command's log.Error did) and the command's loop has an empty logSaveFailed case, so the one place a save error is logged is the apply function."
  - "No cancelled-context pre-check in applyLogCategory (unlike applyGroupLanguage): logchannels.SetCategory takes no context, and the commands pass the live update context, so a refusal there would only add a way for the commands to behave differently. A failed read-back is reported in the result (Row nil, Err set) and is still an applied write."
  - "GetContext and LoadContext share one unexported loadRow, so the key, TTL and loader cannot drift and the reader caches exactly what the commands' loader caches (nothing for a miss)."

patterns-established:
  - "A multi-argument command keeps its own loop and calls a one-item apply function; the apply function validates the item with the repository's own predicate so the command and a page treat a name identically."

requirements-completed: []

coverage:
  - id: D1
    description: "/log and /nolog send, post and store exactly what they did before the extraction, including the continue-on-failed-save quirk and its empty-list reply and audit post"
    requirement: "PAGE-04 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyLogCategoryRequests|TestNologTogglesCategory)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "applyLogCategory changes exactly the one category it is given, drops log_channel:<chat>, treats a mixed-case name as valid, and returns typed results for an unknown name, a chat with no channel, a failing write and a failed read-back"
    requirement: "PAGE-04, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestApplyLogCategory$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "logchannels.LoadContext tells a chat with no linked channel from a failed read, creates nothing and leaves no cache entry for a miss"
    requirement: "PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^TestLoadLogChannelContext$' ./fuku/db/logchannels"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 08: /log and /nolog behind applyLogCategory Summary

Each log category is now written through one apply function that `/log` and `/nolog` call once per category they are given, behind a golden recorded first, and a reader separates "no log channel" from "could not read it" for the Log channel page of 18-09.

## What changed

- **Golden (commit d6471a0, alone, before any production edit).** `logchannels_legacy_request_test.go`: `TestLegacyLogCategoryRequests`, 25 cases from `-1009918001601` (case i uses the chat `-1009918001600 - 2i - 1` and the log channel one below it) through `enableLog` and `disableLog` with the real locales. It records every `sendMessage` request in order (the reply in the group and the `#SETTINGS` audit post to the log channel, with the chat ID and channel ID blanked) and the six stored flags. Cases: a private chat (both commands), a non-admin (both), no channel linked (both), no argument (both), only unknown categories, an unknown next to a known one, one category off and on, a mixed-case name, several on and off, a repeated name, a category already in that state, `/nolog settings` (the audit post is never sent because the settings category is off by then), one of several failing to save (`cat_user` only, through a real GORM callback on the update chain of `log_channels` that matches the column in the update map), every save failing (reply and audit post carry an empty list) and an unknown plus failing known. It passed unchanged after the extraction and was never re-recorded.
- **Extraction and reader (commit 9c3bd47).** `logchannels.go`: `logCategoryWrites` (default `logchannels.SetCategory`), `logApplyResult` (outcome, reply key, the row read back, the error) and `applyLogCategory(ctx, chatID, name, on)`; `setLogCategories` calls it once per argument and maps the outcomes onto its old behaviour (unknown collected, not-set replies and stops, a save failure skipped, applied collected) and keeps its pre-checks, replies and audit post. `fuku/db/logchannels/repository.go`: `LoadContext` and the shared `loadRow`. `TestLoadLogChannelContext` (four subtests over a miniredis-backed cache stack, as the warn reader's test): a chat with no row is `(nil, nil)`, creates no row and leaves no `log_channel` key, also after the commands' `Get`; a linked chat reads back and leaves exactly the commands' key; a `SetCategory` and an `Unset` are seen at once by both readers; a cancelled context returns an error and no row. AGENTS.md: the "One setting, one code path" bullet gains the log-category paragraph (the apply function, its outcomes, the commands' loop, quirk and audit post, `LoadContext`, and that `log_channel` is not in `skipLocal`).
- **Apply test (commit f19ffba).** `logchannels_apply_test.go`: `TestApplyLogCategory` (six subtests). Each of the six categories off and on over a baseline with two already off changes that flag only in the table and in the read-back and drops a stale marker primed under `log_channel:<chat>`; `Settings` and `  ADMIN ` are valid and turn those flags; `bogus`, the empty string and `admin,user` are `logUnknownCategory` with the write seam called zero times; a chat with no channel is `logNotSet` and creates no row; a failing seam is `logSaveFailed` carrying the error with the flag unchanged; a cancelled context is an applied write with no row and the read-back error. A mutation check commenting out the `invalidate` call in `SetCategory` made the first subtest fail ("left the log_channel key cached"); restored (no diff left).

## Verification (real results)

- Task 1 verify: `go test -tags testtools -race -count=1 -run '^(TestLegacyLogCategoryRequests|TestNologTogglesCategory)$' -v ./fuku/modules`: `--- PASS` for the golden (25 subtests) and `TestNologTogglesCategory`; `-run '^TestLoadLogChannelContext$' ./fuku/db/logchannels`: four subtests `--- PASS`, `ok`. The golden also passed with `-count=2`. `git log --reverse --format=%s 29aca33..HEAD -- fuku/modules/logchannels_legacy_request_test.go fuku/modules/logchannels.go | head -n 1` prints `test(18-08): pin /log and /nolog requests before the extraction`. The four acceptance greps (`TestLegacyLogCategoryRequests`, `TestLoadLogChannelContext`, `applyLogCategory`, `LoadContext`) each print `1`.
- Task 2 verify: `-run '^(TestApplyLogCategory|TestLegacyLogCategoryRequests)$'`: both `--- PASS`; `grep -c 'func TestApplyLogCategory' fuku/modules/logchannels_apply_test.go` prints 1.
- Focused run of the new tests plus the neighbouring goldens: `-run '^(TestLegacy.*Requests|TestSettingsMainGrid|TestNologTogglesCategory|TestApplyLogCategory)$' ./fuku/modules`: `ok ... 10.561s`.
- Whole `fuku/modules` package, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log `modules-18-08.log` in the scratchpad): `ok github.com/uasneppy/fuku_robot/fuku/modules 1633.821s`, `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1714 top-level `--- PASS` lines (298 of them `TestSettings*`, so the settings family ran inside it), and the golden and the apply test both `--- PASS` in it. `go test -tags testtools -race -count=1 ./fuku/db/logchannels`: `ok 1.114s`.
- `go vet -tags testtools ./fuku/modules ./fuku/db/logchannels` clean; `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` on the five plan Go files prints nothing; `make check-docs`: "No drift detected"; `make check-translations`: all translations present; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; no `t.Skip` in the new tests.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Deviations from Plan

### Auto-fixed Issues

None needing a production fix. Notes:

1. **[Test design] The cache-drop proof judges only the stale marker.** The read-back inside `applyLogCategory` refills `log_channel:<chat>` with the real row right after the write drops it, so a probe expecting a cache miss would fail for the wrong reason. The test primes a stale marker row (after deleting whatever the key holds), calls the function, and asserts the marker is gone; the mutation check above shows it fails when the drop is removed.
2. **[Test scope] No "the fake records no call" assertion.** `applyLogCategory` takes no bot and has no access to one, so it cannot send; an assertion on a fake the function cannot reach would prove nothing. The audit post stays in the commands and is pinned by the golden.
3. **[Test fixture] The write failure for one category matches the column in the update map**, not the chat ID, because the callback sees the `log_channels` table and the `Updates` map; each case registers and removes its own callback and the goldens never run in parallel, so no other case is touched.
4. **Earlier-plan tests edited:** none; no golden was re-recorded.

**Total deviations:** 0 auto-fixed, 4 notes. **Impact:** none on the contract.

## Known Stubs

None. The apply function writes through the repository and the reader reads the real table.

## Requirements

`requirements-completed: []`. PAGE-04 and PAGE-10 stay Pending; only plan 18-15 completes them.

## Self-Check: PASSED

- Created files exist on disk (the three created files above) and the commits are ancestors of HEAD: d6471a0, 9c3bd47, f19ffba.
- Every task's acceptance criteria were re-run and print the required values (see Verification).
