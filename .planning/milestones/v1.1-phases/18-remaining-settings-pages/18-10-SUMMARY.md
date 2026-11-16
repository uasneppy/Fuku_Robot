---
phase: 18-remaining-settings-pages
plan: 10
subsystem: settings-menu
tags: [telegram, settings-menu, anonymous-admin, owner-right, apply-function, golden]
status: complete

requires:
  - phase: 18-remaining-settings-pages
    provides: "plan 18-04 (settingsRightOwnerBotAdmin), plan 18-06 (settingsToggleSet and the Switches page pattern) and plan 18-09 (the latest page pattern)"
provides:
  - "TestLegacyAnonAdminRequests: /anonadmin pinned before the extraction (23 cases: the status for the mode off, off with no row and on, also for a member; on and off by the creator in each spelling; already on and already off; an invalid word; a non-owner refused after the command's own read had created the admin row; a failing save), the title escaped, never re-recorded"
  - "applyAnonAdmin(ctx, chatID, current, want) in admin.go with the anonAdminWrites seam and the typed result (applied, already, save failed, the locale key, the mode to draw); /anonadmin keeps its read, owner check and every reply byte and maps its two write branches onto it"
  - "admin.LoadAnonAdminContext: one plain SELECT that returns its error, creates no admin row and reads a missing row as off"
  - "The Anonymous admins page (page y, Order 89, icon 🎭, not owner-only) in settings_anonadmin.go: the reused mode sentence with the group title escaped and capped, the owner hint, a full-width toggle whose control declares settingsRightOwnerBotAdmin and writes only through applyAnonAdmin"
  - "Eleven new tests across five files (apply, reader, opens, parity, rights, forged, drift, authority parity, reads create nothing, callback bytes, page fits every locale)"
affects: [18-11, 18-12, 18-13, 18-15]

plan_head_before: c33dd14539db43e2850e29ad4ce2e71a0f13f541
plan_head_after: 1090c57

actuals:
  tokens: 18000   # chars/4 over the added non-planning lines of c33dd14..1090c57 (72139 chars), tests and docs included
  tasks: 4
  commits: 6   # MEASURED: git rev-list --count c33dd14..HEAD (before the SUMMARY and state commits)

key-files:
  created:
    - fuku/modules/admin_legacy_request_test.go
    - fuku/modules/admin_apply_test.go
    - fuku/db/admin/load_anon_test.go
    - fuku/modules/settings_anonadmin.go
    - fuku/modules/settings_anonadmin_test.go
    - fuku/modules/settings_anonadmin_screens_test.go
  modified:
    - fuku/modules/admin.go
    - fuku/db/admin/repository.go
    - fuku/modules/settings_main_test.go
    - AGENTS.md
    - README.md
    - docs/src/content/docs/api-reference/callbacks.md
    - docs/src/content/docs/api-reference/permissions.md
    - .planning/phases/18-remaining-settings-pages/18-SPIKE.md

key-decisions:
  - "The apply result carries the full locale key (admin_anon_admin_enabled_now and so on) instead of a suffix: the command's module name lower-cased is admin, so the key is the same string, and the page needs no prefix logic."
  - "/anonadmin's reply for a successful or unchanged write is built in a small plain function (anonAdminChange) that calls applyAnonAdmin and splices the escaped title; a failed save is answered there with the plain error text exactly as before, so every reply byte stays what the golden pinned."
  - "Opening the page needs settingsRightAdmin only (D-02), so it opens while the bot is not an administrator, looser than the command's status view (RequireBotAdmin in its descriptor): stated in AGENTS.md and 18-SPIKE.md, and asserted as exactly that difference by TestSettingsAuthorityParityAnonAdmin."
  - "The page reads the mode only through admin.LoadAnonAdminContext, so opening Main or the page creates no row, while /anonadmin keeps its creating read (Pitfall 3) pinned by the golden."

patterns-established:
  - "A control whose right mixes a person and the bot declares the composite right of 18-04 and is proved by an authority-parity matrix over the actors and the bot's role, with the viewing looseness asserted as the stated difference."

requirements-completed: []

coverage:
  - id: D1
    description: "/anonadmin writes the mode through applyAnonAdmin with every request and stored row unchanged, including the admin row the command's own read creates for a non-owner it then refuses"
    requirement: "PAGE-05, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestLegacyAnonAdminRequests|TestApplyAnonAdmin|TestAnonAdminOwnerTogglesSetting)$' ./fuku/modules; go test -tags testtools -race -count=1 -run '^TestLoadAnonAdminContext$' ./fuku/db/admin"
        status: pass
    human_judgment: false
  - id: D2
    description: "The live creator with the bot an administrator turns the mode on and off from the page and /anonadmin and the anonymous-admin check then read the same; a press of the stored value writes nothing and shows no already text"
    requirement: "PAGE-05, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsAnonAdminOpens|TestSettingsParityAnonAdmin|TestSettingsDriftAnonAdmin)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "A non-owner administrator gets the owner alert, the bot not being an administrator gives the bot notice (first when both refuse), a plain member is refused before any right, forged presses are refused before any lookup, each with nothing written or edited"
    requirement: "PAGE-05, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsAnonAdminRights|TestSettingsAnonAdminForged|TestSettingsAuthorityParityAnonAdmin)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "Reads create nothing, a failed read is the cannot-open edit and the unknown mark, buttons fit 64 bytes (45 private, 41 group), the page fits 3800 UTF-16 units in 8 languages with 64-rune titles and the title is escaped"
    requirement: "PAGE-05, PAGE-10 (contribution only)"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -count=1 -run '^(TestSettingsAnonAdminReadsCreateNothing|TestSettingsAnonAdminCallbackBytes|TestSettingsAnonAdminPageFitsEveryLocale)$' ./fuku/modules"
        status: pass
    human_judgment: false
---

# Phase 18 Plan 10: the Anonymous admins page Summary

The group's owner can now switch anonymous-admin mode from an Anonymous admins page with exactly the rights `/anonadmin` needs (the live creator, and the bot an administrator), every administrator can read the mode in `/anonadmin`'s own words, and the command's write and the page's press are one function.

## What changed

- **Task 1, tracer (commits 20e87c9, b2fe8c0).** The golden `TestLegacyAnonAdminRequests` (`admin_legacy_request_test.go`, chats from `-1009918001701`, title `Anon <Golden> & Co`, a real GORM update callback for the failing save) was committed alone and run green on the unchanged code in both environments. Then `admin.go` gained `anonAdminWrites`, `anonAdminApplyResult`, `applyAnonAdmin` and the plain helper `anonAdminChange`; `anonAdmin` keeps its `admin.GetAdminSettings` read, its `RequireUserOwner` check and its replies. `fuku/db/admin/repository.go` gained `LoadAnonAdminContext` (and its imports were regrouped as the project does, the one gofmt hunk the plan names). `TestApplyAnonAdmin` and `TestLoadAnonAdminContext` are new. AGENTS.md: the "One setting, one code path" bullet.
- **Task 2 (commit ab6bc91).** `settings_anonadmin.go`: page `y`, `Order` 89, icon 🎭, seam `settingsAnonRead`, control `m` (`tg` only, `settingsEditSwitchValue`, `settingsRightOwnerBotAdmin`), `settingsAnonMainState`, `settingsAnonOpen`, the pure `renderSettingsAnonAdmin` (the reused sentence formatted with `staffDisplayTitle`, the hint, the full-width toggle, Back and Close: rows `{1,2}`) and `settingsAnonSet` over `settingsToggleSet` and `applyAnonAdmin`. `settings_main_test.go`: the Main-shape constants become `.../ o|y / a / ...`, still nine rows. AGENTS.md: the callbacks page list, the Main order sentence and the new page bullet. `TestSettingsAnonAdminOpens` and `TestSettingsParityAnonAdmin`.
- **Task 3 (commits 765c6d2, 81722d0).** `TestSettingsAnonAdminRights` and `TestSettingsAnonAdminForged` (7 forged shapes). Docs: AGENTS.md Permissions bullet, README, `callbacks.md`, `permissions.md` and `18-SPIKE.md` checks 14 to 16.
- **Task 4 (commit 1090c57).** `TestSettingsDriftAnonAdmin`, `TestSettingsAuthorityParityAnonAdmin` (six actors, the bot an administrator or not, plus two viewing subtests), `TestSettingsAnonAdminReadsCreateNothing`, and in `settings_anonadmin_screens_test.go` `TestSettingsAnonAdminCallbackBytes` and `TestSettingsAnonAdminPageFitsEveryLocale` (4 titles including `<` and `&` runs, 8 languages, both menu kinds, both modes, four notices).

## Verification (real results)

- Task 1 verify: `TestLegacyAnonAdminRequests` (23 subtests), `TestApplyAnonAdmin`, `TestAnonAdminOwnerTogglesSetting` and `TestAnonAdminHandlesNoopAndInvalidOptions` pass; `TestLoadAnonAdminContext` passes in `fuku/db/admin`. The golden also passed in a bare run (BOT_TOKEN, OWNER_ID, MESSAGE_DUMP, REDIS_ADDRESS and DATABASE_URL unset) and in the CI environment, before the extraction and after it, unchanged.
- Tasks 2 to 4 verify blocks: every named test `--- PASS`, plus `TestSettingsMainGrid`, `TestSettingsDeletionOpensFromMain`, `TestSettingsDeletionRowBudget`, `TestSettingsEditRegistry`, `TestSettingsMreqOpensFromMain`, `TestSettingsControlActionAllowList` and `TestSettingsRights`; `make check-docs` "No drift detected".
- Focused run, CI environment (`cienv.sh`), of the new tests plus `^(TestLegacy|TestSettingsMain|TestSettingsLabelLengths)` on `./fuku/modules ./fuku/i18n ./fuku/db/admin`: `ok` (modules 24.3 s); the same set bare: `ok` (24.5 s).
- Whole `fuku/modules` package, CI environment, `go test -tags testtools -race -count=1 -timeout 30m -v ./fuku/modules` (background, log `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/modules-18-10.log`): `ok github.com/uasneppy/fuku_robot/fuku/modules 1605.359s`, last line `exit 0`, `grep -cE -- '--- (FAIL|SKIP)|^FAIL'` = 0, 1736 top-level `--- PASS` lines, and all eleven new tests `--- PASS` in it. Whole `./fuku/db/admin` and `./fuku/i18n`: `ok`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/db/admin ./fuku/i18n` clean; `gofmt -l` on every touched Go file prints nothing; `make check-translations` all present; `make check-docs` no drift; `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 ("no dupl, gocyclo or godox findings"), read directly; `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0; `git diff --exit-code 29aca33 -- fuku/modules/settings_pages.go migrations fuku/db/models main.go` exit 0; no `t.Skip`, `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (the installed golangci-lint is built with Go 1.25, a documented sandbox fact), gosec, `make test`.

## Deviations from Plan

### Auto-fixed Issues

None needing a production fix. Notes:

1. **[Plan text] No locale keys added.** `settings_page_anon`, `settings_anon_hint` and `settings_btn_anon_toggle` were added by plan 18-02 in all eight files with their `settingsLocaleLimits` rows and the label-length coverage (`TestSettingsLabelLengths`), so this plan adds none.
2. **[Design] The result carries the whole locale key** (`admin_anon_admin_*`), not a suffix, and `anonAdminChange` is a plain function rather than a method (it would not use its receiver).
3. **[Test design] Page-open helper presses by hand.** The settings env's `openMenu` only opens `env.chat`; the page tests hand-build the `op` and `bk` presses on any chat, as the Switches tests do.
4. **[Plan text] The failed-write redraw is proved inside `TestSettingsParityAnonAdmin`**, not repeated in `TestSettingsAnonAdminRights`; the same assertion (stored off sentence plus `admin_anon_admin_db_error`, nothing stored).
5. **[Docs] Refusal words.** The plan and UI-SPEC mock name `chat_status_owner_cmd_error` for a non-owner, which 18-04 changed to `chat_status_owner_button_error` for a button (the command's own responder choice); docs and the live-check list quote that key's real text, "You need to be the group creator to do this!", and `I'm not admin!` for the bot notice.
6. **Earlier-plan tests edited:** only the three Main-shape constants in `settings_main_test.go`, as the plan says; no golden was re-recorded.

**Total deviations:** 0 auto-fixed, 6 notes. **Impact:** none on the contract.

## Known Stubs

None. The page writes through the repository and reads the real table.

## Threat Flags

None. The page adds no endpoint, no auth path and no schema. T-18-33 to T-18-36 are mitigated as the plan says: the owner right on the press's own member record (`TestSettingsAnonAdminRights`, `TestSettingsAuthorityParityAnonAdmin`), the verified group of the press, the golden recorded first, and `Actions` plus `Validate` in step 1 (`TestSettingsAnonAdminForged`).

## Requirements

`requirements-completed: []`. PAGE-05 and PAGE-10 stay Pending; only plan 18-15 completes them.

## Self-Check: PASSED

- Created files exist on disk (`admin_legacy_request_test.go`, `admin_apply_test.go`, `load_anon_test.go`, `settings_anonadmin.go`, `settings_anonadmin_test.go`, `settings_anonadmin_screens_test.go`) and the commits are ancestors of HEAD: 20e87c9, b2fe8c0, ab6bc91, 765c6d2, 81722d0, 1090c57.
- Acceptance greps re-run: the first commit touching `admin_legacy_request_test.go` or `admin.go` starts with `test(18-10):`; each required test function prints `1`; `func applyAnonAdmin(` and `func LoadAnonAdminContext(` once each; `gofmt -l fuku/db/admin/repository.go` prints nothing; `registerSettingsPage` once and `settingsRightOwnerBotAdmin` at least once in `settings_anonadmin.go`; no `OwnerOnly: true` there; `/anonadmin` appears in `permissions.md`.
