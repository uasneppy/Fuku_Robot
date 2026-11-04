---
phase: 10-pm-settings-hub-and-owner-gate
verified: 2026-11-04T15:30:00Z
status: human_needed
score: 5/5 must-haves verified
covered_files:
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-01-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-01-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-02-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-02-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-03-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-03-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-04-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-04-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-05-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-05-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-06-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-06-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-07-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-07-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-08-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-08-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-09-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-09-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-10-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-10-SUMMARY.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-11-PLAN.md"
  - ".planning/phases/10-pm-settings-hub-and-owner-gate/10-11-SUMMARY.md"
  - "fuku/db/adminindex/repository.go"
  - "fuku/db/menuaccess/repository.go"
  - "fuku/modules/admin_index_feed.go"
  - "fuku/modules/admin_index_sweeper.go"
  - "fuku/modules/settings.go"
  - "fuku/modules/settings_access.go"
  - "fuku/modules/settings_gate.go"
  - "fuku/modules/settings_hub.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_pages.go"
  - "migrations/20261104120000_add_settings_hub_tables.sql"
covered_digest: "v4:sha256:6245c0cce5f659da385e9134093e989aba32ba0981c0a18f214bfff1e6a10f39"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Owner's live check A1 (10-SPIKE.md): in a real group whose owner deleted their Telegram account, call getChatAdministrators as the bot and read the creator entry."
    expected: "The creator entry arrives as first name \"Deleted Account\" with no last name and no username, so settingsCreatorGone treats the group as having no live owner and lets live administrators in (D-15). If the shape differs, a group with a deleted owner stays owner-only for everyone (the safe direction) and the detection must change."
    why_human: "What Telegram returns for a deleted owner is a live API fact (planner assumption A1). The code and its tests only prove behaviour for the shape the fakes return."
  - test: "Owner review of the two orchestrator clarifications recorded 2026-11-04 (D-01 and D-20) and of the deferred WR-02."
    expected: "D-01: list buttons show the plain title, cut to 32 code points, not HTML-escaped (the code does this). D-20: the private group list has Close only, a private group Main has [Groups | Close], a group Main has [Close] alone, and every sub-page, Access included, has [Back | Close] (the code does this). WR-02: a hand-built callback with a forged `c` on a non-pick private action can still tell the sender whether the bot is in that chat (kept on purpose, Phase 9 D-11)."
    why_human: "These are owner decisions the orchestrator interpreted. The code honours them, but only the owner can accept them."
  - test: "Live Telegram pass over 10-SPIKE.md sections 3 and 4: private list and pick, Groups button, Access ON card and OFF tap, a gated admin's refusals (in-group /settings, deep link, an old menu), anonymous owner proof tap, and the backfill/hourly sweep with several groups and two replicas."
    expected: "As the criteria describe: a gated admin sees no group in the list and gets the short refusals; the owner sees the Access page; the next press on the other replica sees a flip; the grid reads as two columns with Close."
    why_human: "Needs a real Telegram client, real getChatAdministrators answers, and real pacing across replicas. The tests drive the same paths through hand-written fakes."
  - test: "CI-only items: `make lint` (golangci-lint on Go 1.26), `make test-postgres-integrity` (TestAdminIndexReplaceConcurrent, TestMenuAccessUpsert, and the migration list) and a native-speaker read of the 19 new strings."
    expected: "All green; strings read naturally in es, fr, hi, id, pt, ru, uk."
    why_human: "They need a PostgreSQL server, a lint toolchain or a native reader, none of which exist in this verification environment."
---

# Phase 10: PM Settings Hub and Owner Gate Verification Report

**Phase Goal:** As a group owner, I want to open my groups' settings from the bot's private chat and decide whether my admins may change them, so that I control who configures the bot.
**Verified:** 2026-11-04T15:30:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

All five ROADMAP success criteria are met in the code, and the focused tests pass. The status is `human_needed` only because of the live-Telegram and owner-review items listed above; there are no gaps.

## Goal Achievement

### Observable Truths

| #   | Truth (ROADMAP success criterion) | Status | Evidence |
| --- | --------------------------------- | ------ | -------- |
| 1 | `/settings` in the private chat lists the user's groups, paged; a pick opens that group's main menu after a live check; the list comes from the bot's own roster, refreshed on admin-list loads and promotion/demotion, cleared when the bot leaves; an empty list says to run `/admincache` or `/settings` in the missing group | VERIFIED | `settingsDesc.RequiredChecks` is `requireSettingsAccess()`, which lets a private chat pass at once (`settings_gate.go:270-274`); `settings()` sends `settingsPrivateList` in private (`settings.go:113`), which reads `settingsHubLookup` = `adminindex.ListForUserFresh` from the sender's own ID and makes no Telegram call (`settings.go:86`, `settings_hub.go:48,179`). 8 groups a page, sorted by lower-cased title, a one-group list is still a list (`settings_hub.go:33,85,143-157`). The pick `pk` runs the whole press pipeline: live `getChatMember`, gate, then `getChat` (`settings_menu.go:329-400`). Roster is written from the admin-list loader through `cache.SetAdminListObserver` (`admin_index_feed.go:27,44`), `adminCacheAutoUpdate` reloads on promotion/demotion and clears on left/kicked (`admin.go`, `bot_updates.go:108`), `/admincache` and group `/settings` refresh (`settings.go:122,131`), plus a startup backfill and hourly sweep (`admin_index_sweeper.go`, `main.go:496`, stop at `main.go:218`). Empty text and hint `settings_list_empty` and `settings_list_hint` exist in all 8 locales. Tests pass: `TestSettingsHub*`, `TestAdminIndex*`, `TestSettingsPickSameAnswer`, `TestSettingsEmptyListSameForAll`. |
| 2 | By default only the live owner gets in through the private list, in-group `/settings`, the deep link and a button press; a regular admin does not see the group in their list; in-group `/settings` and the deep link refuse with a short reply that never names the Staff Group; in-group setting commands keep working for admins | VERIFIED | One decision, `settingsGate` (`settings_gate.go:89-125`): a live creator is in, any status other than administrator is undecided, an administrator needs the fresh switch or a group with no live owner. It is called at all four doors: in-group `/settings` via `requireSettingsAccess` (`:279`), the deep link (`settings.go:185`, before `getChat`), every press and the pick (`settings_menu.go:360`). Refusals are plain: `settings_refuse_owner_only`, `settings_start_owner_only`, `settings_gate_owner_only`, none naming the switch or Staff Group (present in 8 locales). The list hides a gated admin's group in SQL (`ListForUserFresh`: creator row, or administrator row with the switch on or no creator row, `repository.go:137-190`). `/setrules`, `/captchamode` and the rest were not touched: `TestSettingsCommandsUngated` passes. `TestSettingsGateAllDoors` proves all doors decide the same over four worlds and passes. |
| 3 | Only the live owner sees and can press "Admins can change settings"; while on, live admins get the full menu; a change takes effect on every replica at the next press; an anonymous owner is identified by the proof tap | VERIFIED | Access page is `OwnerOnly` (`settings_access.go:37`): `settingsMainEntries` skips it for a non-owner without calling it, `openPage` refuses a non-creator, control and `ay` action declare `settingsRightOwner`, and `applyAdminsAllowed` refuses a non-creator before writing. ON is a two-tap card (`settingsAccessSet` `"1"` draws the card and writes nothing), OFF is one tap (`:169-179`). The switch is a fresh DB read with no cache (`menuaccess.AdminsAllowedFresh`, the only reader, `settings_gate.go:61`), and the gate reads it before the owner memo, so a flip is seen at the next press. `TestSettingsAccessAcrossReplicas` flips on replica A and checks B's very next press in both directions, including past a memoised owner. Anonymous: `/settings` re-enters through `RegisterAnonymousAdminHandler` with `anonPipelineHandler` re-running `RequiredChecks`; `TestSettingsAnonymousOwnerGate` passes. |
| 4 | The main menu shows every page of the group in a two-column grid with Close, in the opener's language, and every page has Back | VERIFIED | `renderSettingsMain` fills pages two per row in `Order`, an odd last one alone, owner-only pages on their own full-width row, a status block in the body, and `navRow(true)` (`settings_pages.go:479-527`). `navRow(false)` gives every sub-page `[Back | Close]` (`:223-242`); a private Main ends `[Groups | Close]`, a group Main `[Close]`. `TestSettingsMainGrid`, `TestSettingsNavigation` and the per-page `PageFitsEveryLocale` tests pass. |
| 5 | Every new message and button label exists in all 8 locales, and `AGENTS.md` describes the 8-locale set | VERIFIED | `make check-translations` is clean; spot-checked keys (`settings_refuse_owner_only`, `settings_start_owner_only`, `settings_gate_owner_only`, `settings_access_owner_only`, `settings_list_empty`, `settings_list_hint`, `settings_pick_unavailable`, `settings_btn_access_toggle`, `settings_main_state_line`, `settings_btn_groups`) are present in all 8 files; `TestLocalesAreTheEight` and `TestSettingsLocaleKeys` pass; `make check-docs` shows no drift; a grep for "7 locale/language/files" in AGENTS.md, README.md, PROJECT.md and CLAUDE.md finds nothing. |

**Score:** 5/5 truths verified (0 present but behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261104120000_add_settings_hub_tables.sql` | `chat_settings_access`, `chat_admin_index`, constraints inline, idempotent, no BEGIN | VERIFIED | Read in full; unique and CHECK constraints are inline; `TestSettingsHubMigrationStatements` passes |
| `fuku/db/menuaccess/repository.go` | Fresh switch read, column-targeted upsert | VERIFIED | `AdminsAllowedFresh`, `SetAdminsAllowed`; package tests pass |
| `fuku/db/adminindex/repository.go` | Replace/clear per chat under a lock, list for user | VERIFIED | `ReplaceChat` with advisory lock on PostgreSQL, upsert then prune; `TestAccessTablesStayOutOfBackup` passes |
| `fuku/modules/settings_gate.go` | Owner gate, owner-state read, memo | VERIFIED | Read in full; wired at all four doors |
| `fuku/modules/settings_hub.go` | Private list and `shub|` callback | VERIFIED | Registered in `LoadSettings` (`settings.go:237`) |
| `fuku/modules/settings_access.go` | Access page, single apply path | VERIFIED | Registered through `registerSettingsPage` |
| `fuku/modules/admin_index_feed.go`, `admin_index_sweeper.go` | Roster feed, backfill and hourly sweep | VERIFIED | Started in `postInit` (`main.go:496`), stopped by a shutdown handler registered after DB-close (`main.go:218`) |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `settingsDesc` | `settingsGate` | `requireSettingsAccess` | WIRED | `settings.go:42` |
| deep link | `settingsGate` | member record of its own `getChatMember` | WIRED | `settings.go:185` |
| `settingsPress.verify` | `settingsGate` | step 4b | WIRED | `settings_menu.go:360` |
| admin-cache loader | `chat_admin_index` | `cache.SetAdminListObserver` and `adminIndexObserve` | WIRED | `admin_index_feed.go:27,44` |
| bot left/kicked | roster clear | `adminCacheAutoUpdate` | WIRED | `bot_updates.go:108` |
| list pick | Main page | core action `pk` and the full press pipeline | WIRED | `settings_hub.go:153`, `settings_menu.go:430` |
| Access write | gate read | same `settingsAccessLookup` seam | WIRED | no cache layer in between |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| private list | `groups` | `chat_admin_index` via `ListForUserFresh`, filled by the loader feed, `/admincache`, `/settings` and the sweep | Yes | FLOWING |
| Access page and gate | `allowed` | `chat_settings_access` fresh query, written by `SetAdminsAllowed` | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Settings, hub, gate, access, roster tests | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings\|TestAdminIndex)' ./fuku/modules ./fuku/i18n` | ok (modules 137.6 s, i18n 5.1 s) | PASS |
| Repositories and cache observer | `go test -tags testtools -race -count=1 ./fuku/db/adminindex ./fuku/db/menuaccess ./fuku/utils/cache` | ok | PASS |
| Migration statements, locale set, locale keys, backup exclusion | `go test -tags testtools -count=1 -run 'TestSettingsHubMigrationStatements\|TestLocalesAreTheEight\|TestSettingsLocaleKeys\|TestAccessTablesStayOutOfBackup' ...` | ok | PASS |
| Named gate/replica/grid/hide/ungated/anon tests, verbose | `-run '^(TestSettingsAccessAcrossReplicas\|TestSettingsGateAllDoors\|TestSettingsMainGrid\|TestSettingsListHidesGated\|TestSettingsCommandsUngated\|TestSettingsAnonymousOwnerGate)$' -v` | all six PASS | PASS |
| Translations, generated docs, compile | `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...` | clean | PASS |

The invariant-style truths (every door gated the same, a flip seen on the next press on another replica, a gated admin's menu refused on the next press, one answer per press) are covered by tests that drive the real handlers through fake Bot clients, so none is left presence-only.

### Probe Execution

Step 7c: SKIPPED. No probe scripts are declared by the phase.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| MENU-09 | 10-05/09/10 | Private `/settings` list, paged, pick opens Main | SATISFIED | Truth 1 |
| MENU-10 | 10-01/07/08 | Roster built from admin-list loads, promotion/demotion, cleared on leave; empty-list hint | SATISFIED | Truth 1 |
| MENU-11 | 10-05/06/11 | Owner-only by default at every door | SATISFIED | Truth 2 |
| MENU-12 | 10-06 | Owner-only switch on the Access page | SATISFIED | Truth 3 |
| MENU-13 | 10-05/11 | Gated admin hidden in the list and refused in-group and at the deep link; commands unchanged | SATISFIED | Truth 2 |
| MENU-14 | 10-05/06 | Change seen on every replica; anonymous owner by proof tap | SATISFIED | Truth 3 |
| MENU-15 | 10-10 | Two-column Main with Close, Back everywhere | SATISFIED | Truth 4 |
| PLAT-05 | 10-02/03 | 8 locales and AGENTS.md says 8 | SATISFIED | Truth 5 |

All eight IDs appear in the plans' `requirements` frontmatter and are marked Complete in REQUIREMENTS.md. No orphaned requirement maps to Phase 10.

### Anti-Patterns Found

None blocking. No `TBD`, `FIXME` or `XXX` in the phase's changed source files. No stubs: every artifact is read and wired.

### Prior Audits (used as evidence, spot-checked, not redone)

- Code review: 0 critical; WR-03 (roster clear only on Bot API 400 or 403) and IN-03 are fixed, and the fix is visible in `cache.IsBotGoneError` (`adminCache.go:74-83,155`). WR-01 (name-based deleted-owner detection), WR-02 (forged `c` on a non-pick private press), IN-01, IN-02 and IN-04 are deferred with recorded reasons, and none contradicts a success criterion.
- Security: 51 threats, 0 open, 2 accepted. Validation: `validated`.

### Human Verification Required

See the frontmatter list. In short: the live deleted-owner check (A1), the owner's review of the D-01 and D-20 clarifications and of deferred WR-02, a live Telegram pass over the hub, and the CI-only items (`make lint`, `make test-postgres-integrity`, native-speaker read of the new strings).

### Gaps Summary

No gaps. Every ROADMAP success criterion and every requirement ID is satisfied in the code, with passing tests behind the behaviour-dependent parts. The D-01 and D-20 clarifications are honoured by the code (plain-text button labels cut to 32 code points; the list has Close only and every group sub-page has Back). The remaining items need a real Telegram client, CI or the owner's decision.

---

_Verified: 2026-11-04T15:30:00Z_
_Verifier: Claude (gsd-verifier)_
