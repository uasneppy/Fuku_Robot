---
phase: 01-staff-group-links
verified: 2026-10-05T00:25:00Z
status: human_needed
score: 5/5 must-haves verified
covered_files:
  - ".planning/phases/01-staff-group-links/01-01-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-01-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-02-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-02-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-03-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-03-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-04-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-04-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-05-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-05-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-06-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-06-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-07-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-07-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-08-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-08-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-09-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-09-SUMMARY.md"
  - ".planning/phases/01-staff-group-links/01-10-PLAN.md"
  - ".planning/phases/01-staff-group-links/01-10-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "alita/db/cache/local.go"
  - "alita/db/cache/local_test.go"
  - "alita/db/models/staff.go"
  - "alita/db/staff/exclusivity_postgres_test.go"
  - "alita/db/staff/rekey.go"
  - "alita/db/staff/rekey_test.go"
  - "alita/db/staff/repository.go"
  - "alita/db/staff/repository_test.go"
  - "alita/db/staff/testmain_test.go"
  - "alita/db/testmain_test.go"
  - "alita/i18n/staff_locale_test.go"
  - "alita/modules/deeplink_router.go"
  - "alita/modules/help.go"
  - "alita/modules/staff.go"
  - "alita/modules/staff_health_test.go"
  - "alita/modules/staff_helpers_test.go"
  - "alita/modules/staff_link.go"
  - "alita/modules/staff_link_refusals_test.go"
  - "alita/modules/staff_link_test.go"
  - "alita/modules/staff_migrate_test.go"
  - "alita/modules/staff_notify.go"
  - "alita/modules/staff_ownership_test.go"
  - "alita/modules/staff_panel.go"
  - "alita/modules/staff_panel_render_test.go"
  - "alita/modules/staff_panel_test.go"
  - "alita/modules/staff_picker_test.go"
  - "alita/modules/staff_recheck.go"
  - "alita/modules/staff_setstaff_test.go"
  - "alita/modules/staff_sweeper.go"
  - "alita/modules/staff_sweeper_test.go"
  - "alita/modules/staff_test.go"
  - "alita/modules/staff_unlink.go"
  - "alita/modules/staff_unlink_button_test.go"
  - "alita/modules/staff_unlink_test.go"
  - "alita/modules/staff_unset_test.go"
  - "alita/modules/staff_watchers.go"
  - "alita/modules/test_harness_test.go"
  - "alita/utils/chat_status/owner.go"
  - "alita/utils/chat_status/owner_test.go"
  - "alita/utils/helpers/command_pipeline.go"
  - "alita/utils/helpers/command_pipeline_test.go"
  - "docs/src/content/docs/commands/staff/index.md"
  - "locales/config.yml"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "main.go"
  - "main_test.go"
  - "migrations/20261004120000_add_staff_groups_and_links.sql"
  - "migrations/20261004130000_add_staff_role_exclusivity_trigger.sql"
  - "scripts/check_test_results/main.go"
  - "scripts/check_test_results/main_test.go"
covered_digest: "v2:sha256:b8c929181ca4fe8f8eaceffbea3c0c16e8ce71e9c5d7035c9a71e8aed82b1e81"
behavior_unverified: 0
overrides_applied: 0
gaps: []
deferred: []
human_verification:
  - test: "Anonymous-admin delivery (plan 01-02 / 01-05 human-check, research A4). In a throwaway supergroup where the bot is admin, turn on 'Remain anonymous' for the owner and send /setstaff, /linkstaff and the picker's /start payload; then turn it off and repeat."
    expected: "Anonymous posts get the 'post as yourself' reply (self-deleting in the issuing group for link attempts) and nothing is created; the non-anonymous posts succeed."
    why_human: "Anonymous-admin message shape (SenderChat == chat, From 1087968824) comes from real Telegram clients; the tests build that shape by hand."
  - test: "Add group picker (plan 01-05 human-check, research A3/A12). In /staff press 'Add group' and pick (a) a supergroup where the bot is already a member and admin, (b) one where it is not a member, (c) once with 'Remain anonymous' on."
    expected: "Picker accepts (a) and (b); the bot's existing admin rights are not reduced (restrict + delete are combined with them); /start@bot stf_... arrives with the owner as sender, is deleted, and the confirmation appears only in the Staff Group. /linkstaff is the fallback if the picker misbehaves."
    why_human: "The group picker and the admin-rights dialog exist only in real Telegram clients."
  - test: "Ownership transfer with bot as admin (plan 01-07 human-check, research A1/A5). In a throwaway supergroup linked to a Staff Group, transfer ownership to another account while logging raw updates. Repeat for the Staff Group itself."
    expected: "A chat_owner_changed service message arrives (record whether a creator chat_member update also arrives); the link disappears within seconds; the Staff Group gets exactly one 'owner changed' notice; nothing appears in the linked group. For a Staff Group transfer, the old owner's links are removed and the Staff Group keeps its status."
    why_human: "Whether Telegram sends these updates on a real transfer is undocumented. If it does not, the hourly sweeper is the only remover, so the observed delay decides whether the 'without anyone running a command' wording is met within seconds or within about an hour."
  - test: "Ownership transfer with bot as plain member (plan 01-10 human-check, research A2/A5). Demote the bot to member in a linked throwaway supergroup, transfer ownership, then wait for (or trigger by restarting the bot) the next sweep (first run 1-5 minutes after start)."
    expected: "Record whether chat_owner_changed still reached the bot. Within one sweep the link is removed and the Staff Group gets exactly one 'owner changed' notice; getChatAdministrators succeeded for the non-admin bot (no 'unknown' log for that group). If the admin list is unreadable for a non-admin bot, such links stay unknown and are never removed until the bot is admin (documented A2 fallback)."
    why_human: "Telegram server behaviour for a non-admin bot; the test fake scripts both answers."
  - test: "Bot removal, demotion and re-add (plan 01-08 human-check). In a linked throwaway supergroup remove the bot, add it back as admin with the ban right, then take the ban right away once."
    expected: "The Staff Group gets one heads-up per change and one 'healthy again'; nothing appears in the linked group; the link stays listed throughout."
    why_human: "Only a live run shows the real my_chat_member update shapes (including skipped or merged updates)."
  - test: "/staff panel appearance and Refresh (plan 01-09 human-check). In a real Staff Group with at least two linked groups (bot demoted to member in one), send /staff, press Refresh, then promote the bot back and press Refresh again."
    expected: "One message shows each group with readable status icons and a reason line for the broken one; Refresh edits the same message and the 'Updated' time changes; after promoting the bot the group shows healthy and the Staff Group got exactly one 'healthy again' heads-up. Also page past 8 groups if practical."
    why_human: "Visual layout and readability in real Telegram clients."
  - test: "Native-speaker read of non-English text (plans 01-08 and 01-09). Have speakers of es, fr, hi, id, pt, ru skim the 63 staff_* strings, especially the four health heads-up strings and the 14 panel strings."
    expected: "Wording reads naturally and keeps every placeholder; panel fits one message in each language."
    why_human: "Translation quality cannot be asserted by tests; parity of keys and placeholders is already machine-checked (63 keys, identical placeholders in all 7 locales)."
  - test: "Unlink leaves no trace in the linked group (plan 01-06 coverage item D8). In a real client run /unlinkstaff in a linked group and press Unlink in the Staff Group panel."
    expected: "/unlinkstaff is deleted; refusals disappear after about 30 seconds; Unlink buttons appear only in the Staff Group's /staff reply; members of the linked group see nothing."
    why_human: "What a client shows (command deletion rights, anonymous posting, inline keyboards in a live group) cannot be reproduced by the fake."
  - test: "Real basic-group to supergroup upgrade (plan 01-03 coverage item D4). Upgrade a Staff Group, and separately a linked group, from a basic group to a supergroup (or otherwise change its chat ID)."
    expected: "/staff keeps working under the new ID and the links follow. (Dispatcher wiring of the migrate watcher was checked by the verifier with a throwaway test, see Behavioral Spot-Checks; only the live Telegram upgrade remains.)"
    why_human: "Live Telegram upgrade behaviour. Note a basic-group Staff Group is practically unreachable because the bot leaves basic groups, so the supergroup chat-ID change case matters most."
  - test: "PostgreSQL migration and trigger on the deploy path (plan 01-01 D5, plan 01-04). Deploy with AUTO_MIGRATE=true against a real PostgreSQL and run ALITA_TEST_DATABASE=true make test-postgres-integrity in CI."
    expected: "Both migrations apply once, checksums recorded; TestStaffExclusivityTrigger passes (5 subtests including the 20-round concurrency race) and is not skipped."
    why_human: "No PostgreSQL server in the verification sandbox. Plan 01-04's summary reports a pass on PostgreSQL 16; the verifier could not reproduce it. All other tests run on SQLite."
  - test: "Concurrent /unsetstaff Confirm on two replicas (plan 01-02 coverage items D9, D10, verification: backstop). Press Confirm twice at the same time against two running replicas."
    expected: "Status is removed once, exactly one 'removed' notice, and either all links go or none do."
    why_human: "Cross-replica concurrency and mid-transaction failure are not simulated by any test. The code claims the row first (DELETE staff_groups, RowsAffected == 1) inside one transaction, so the design supports it; the sequential double-press is tested."
---

# Phase 1: Staff Group Links Verification Report

**Phase Goal:** As a community owner, I want to make one group my Staff Group and link my groups to it, so that staff have one control room.
**Verified:** 2026-10-05T00:25:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

All five roadmap success criteria are met in the code, with passing tests and a verifier-run dispatcher check. No criterion failed, so there are no gaps. The status is `human_needed` only because the phase's end-of-phase human checks (real Telegram client behaviour, native-speaker wording, a real PostgreSQL run) have not been done. Six open review warnings are real defects at the edges. None makes a success criterion false, but WR-02 and WR-05 should be triaged before relying on the feature.

## Goal Achievement

### Observable Truths

| # | Truth (ROADMAP success criterion) | Status | Evidence |
|---|-----------------------------------|--------|----------|
| 1 | A group's owner can make it the Staff Group. A channel or unusable group is refused with a reason. Only the Staff Group's owner can remove the status, and removal unlinks all its groups. | VERIFIED | `staff.go` `setStaff`: rejects channels (`staff_refuse_not_group`), `requireLiveCreator` (live `chat_status.CheckOwner`, fail-closed on `OwnerUnknown`), `requireBotAdministrator` (D-08), then `staff.CreateStaffGroup` maps `ErrRoleConflict` to `staff_refuse_group_is_linked`. Anonymous senders are stopped by `helpers.RejectAnonymousSender()`. `unsetStaff` and `staffUnsetConfirm` re-check the live creator at the Confirm press (the recorded owner is never the authority) and call `staff.DeleteStaffGroupWithLinks`, one transaction that claims the staff row first, then deletes its links, then invalidates cache keys for the Staff Group and every removed link. Tests pass: `TestSetStaff*` (anonymous, channel, bot not admin, unknown, linked group, idempotent, refusal order), `TestUnsetStaff*` (confirm removes status and links, non-creator alert, failing owner check changes nothing, double confirm once, recorded owner not creator refused). |
| 2 | A user who owns both a group and the Staff Group can link and unlink it. Anyone who doesn't own both (checked live) is refused with a reason, as is an already-linked group. | VERIFIED | `staff_link.go` `runLinkGroup`: anonymous refusal, then `resolveLinkStaffGroup` (live creator of the Staff Group, uniform refusal for strangers), then `checkLinkTarget` (supergroup only, not a Staff Group, live creator of the target), then `staff.CreateLink`. The unique `group_chat_id` index plus `ON CONFLICT DO NOTHING` gives `ErrAlreadyLinked` (`staff_link_refuse_already_linked`). Both `/linkstaff [id]` and the picker's `/start@bot stf_<id>` use this one flow, and the payload never grants authority. Unlink: `staff_unlink.go` `checkUnlinkAuthority` requires the live creator of the Staff Group and then of the linked group, at the command and at both the Ask and Confirm button presses. Only the caller whose DELETE removed the row posts the notice, and only to the Staff Group. Tests pass: `TestLinkStaff*`, `TestStaffPicker*`, `TestStaffPickerAndTypedCommandMakeTheSameLink`, `TestUnlinkStaff*`, `TestStaffUnlinkButton*` (stranger, only creator of both, owner changed between presses, wrong chat, double confirm, check failure). |
| 3 | When ownership of either group passes to someone else, the link disappears without anyone running a command. | VERIFIED (real-Telegram delivery is a human item) | `staff_watchers.go` registers four group -3 watchers returning `ext.ContinueGroups`: migrate, `chat_owner_changed`/`chat_owner_left`, creator `chat_member` transition, and `my_chat_member`. Each is only a hint; `recheckLink` and `recheckStaffGroup` ask Telegram live. Only `OwnerMismatch` calls `staff.DeleteLinkIfOwner` (single conditional DELETE with RETURNING, so exactly one caller posts the one Staff Group notice). `OwnerUnknown` never deletes. `staff_sweeper.go` is the fallback that needs no update delivery: it rechecks every Staff Group and link hourly (first run 1-5 min after start) behind `SETNX alita:staff:sweep:lock`, started in `main.go` `postInit` and stopped by a shutdown handler registered after the DB-close handler. `config.AppConfig.AllowedUpdates` includes `chat_member` and `my_chat_member`. Tests pass: `TestStaffOwnership*` (payload is only a hint, staff-group-owner change, owner left, unknown keeps link, concurrent rechecks post once, chat_member transition), `TestStaffSweep*` (lock, errors never delete, rekey, orphans, lifecycle, panic recovery). Verifier dispatcher check: a `chat_owner_changed` update pushed through a real `ext.Dispatcher` with the staff modules loaded removed the link and posted the notice. |
| 4 | A link and the Staff Group status keep working after a group upgrades to a supergroup and gets a new chat ID. | VERIFIED | `db/staff/rekey.go` `RekeyChat` updates `staff_groups.chat_id` and both link columns in one transaction, is idempotent for the two migrate messages in either order, and invalidates old and new cache keys. It is called from the migrate watcher (`MigrateToChatId` and `MigrateFromChatId`), from `recheckStaffGroup` and from the sweeper via `rekeyFromTelegramError` (400 `migrate_to_chat_id`). Tests pass: `TestRekeyChat*`, `TestStaffMigrate*` (either order, unrelated chat, from Telegram error). Verifier dispatcher check: a migrate message through a real dispatcher re-keyed the Staff Group and its link, and `/staff` then replied under the new ID. |
| 5 | `/staff` in the Staff Group shows the help text and each linked group's status (linked, bot is admin, bot can restrict, owner still matches). All its text exists in all 7 languages. | VERIFIED | `staff_panel.go`: `staffPanel` is silent outside a Staff Group. `buildStaffPanel` loads links fresh, then for each link (4 in flight, 45 s cap) runs `recheckLink` (owner) and `FetchBotMember` + `applyLinkHealth` (bot admin, can restrict), and renders the help text, chat ID, one row per group with icons, a reason line, a legend and the check time. It pages at 8 groups and caps text at 3800 UTF-16 units, with Refresh and Prev/Next in place. Locale check: 63 `staff_*` keys in each of en, es, fr, hi, id, pt, ru; zero missing, placeholders identical, only `staff_panel_page` is identical to English in fr (a legitimate cognate). Every `GetString("staff_...")` literal in Go resolves to a defined key and no defined key is unused. `make check-translations` and `make check-docs` pass. Tests pass: `TestStaffPanel*` (live statuses, refresh in place, non-member refused, call budget, concurrency limit, fits in every locale, paging), `TestStaffLocaleKeys`. The in-lockdown status is correctly absent (Phase 4). |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `migrations/20261004120000_add_staff_groups_and_links.sql` | Tables, UNIQUE chat IDs, health CHECK, self-link CHECK | VERIFIED | Timestamp greater than every earlier file, idempotent, no top-level transaction control. |
| `migrations/20261004130000_add_staff_role_exclusivity_trigger.sql` | D-10 role exclusivity with advisory lock | VERIFIED (read) | Fires before insert/update on both tables; locks both chat IDs in ascending order. Not executed here (no PostgreSQL); see human items. |
| `alita/db/models/staff.go` | `StaffGroup`, `StaffGroupLink` with `TableName()` | VERIFIED | Table names match the migration; health constants match the CHECK list. |
| `alita/db/staff/repository.go`, `rekey.go` | Cached gates plus `*Fresh` authority reads; every write invalidates | VERIFIED | Each write calls `invalidateStaffKeys` after commit; `staff_group` and `staff_link_of` keys are on the `skipLocal` list in `alita/db/cache/local.go`. |
| `alita/utils/chat_status/owner.go` | Tri-state live `CheckOwner` and `FetchBotMember` | VERIFIED | Any API error is `Unknown`, never `Mismatch`; 8 s timeout; no cache. |
| `alita/modules/staff*.go` (setstaff, unsetstaff, linkstaff, unlinkstaff, panel, watchers, recheck, sweeper, notify) | Commands, callbacks, watchers, sweeper | VERIFIED | Substantive, wired through `RegisterLegacyModule("Staff", 236)` and `("StaffWatchers", 237)`; no stubs. |
| `alita/modules/deeplink_router.go`, `help.go` | Group `/start` payload route | VERIFIED | `HandleGroupDeepLink` called from the group branch of `/start`; `stf_` registered; non-matching payloads fall back to the normal reply. |
| `locales/*.yml` | 63 keys x 7 locales | VERIFIED | See Truth 5. |
| `AGENTS.md` | Group -3, `alita:staff:*` keys, staff rules | VERIFIED | Present and matches the code. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `main.go` `postInit` | `StartStaffSweeper` | direct call after `alita.LoadModules` | WIRED | Line 425; a shutdown handler calls `StopStaffSweeper` and is registered after the DB-close handler (LIFO, so it runs first). |
| Dispatcher | Staff commands and watchers | `LoadAllModules` over the registry | WIRED | Verifier throwaway test loaded `LoadStaff` and `LoadStaffWatchers` into a real dispatcher and drove migrate, owner-changed and `/staff` updates (then deleted the test file). |
| `/start@bot stf_<id>` in a group | `runLinkGroup` | `HandleGroupDeepLink` to `staffPickerDeepLink` | WIRED | Same flow as `/linkstaff`. |
| Watchers, panel, sweeper | `recheckLink` / `recheckStaffGroup` / `applyLinkHealth` | single authority path | WIRED | Only `DeleteLinkIfOwner` removes on Mismatch; only `SetLinkHealth` changes health. |
| Staff callback buttons | `staffCallback` | `callbackquery.Prefix("staff\|")`, `callbackcodec` | WIRED | Chat comes from the message, never the payload; button data that overflows 64 bytes is omitted and logged rather than shipped dead. |
| Every staff write | cache invalidation | `invalidateStaffKeys` | WIRED | Covers create, delete, unset, rekey, orphan cleanup, health and owner update. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `/staff` panel rows | `rows` | `staff.ListLinksByStaffFresh` (DB) plus live `getChatAdministrators` / `getChatMember` | Yes | FLOWING |
| Link authority | `OwnerResult` | live Telegram call, no cache | Yes | FLOWING |
| Unset notice | `removed` | rows read inside the delete transaction | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Build | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Repo, owner, i18n, cache, helpers tests | `go test -tags testtools -race -count=1 ./alita/db/staff ./alita/utils/chat_status ./alita/i18n ./alita/db/cache ./alita/utils/helpers` | all `ok` | PASS |
| Staff module tests under race | `go test -tags testtools -race -count=1 -run 'Staff\|Unset\|Setstaff\|SetStaff\|LinkStaff\|UnlinkStaff\|StartGroupPayload' ./alita/modules` | `ok` (33.8 s) | PASS |
| Locale parity | `make check-translations` | "All translations are present" | PASS |
| Generated docs drift | `make check-docs` | "No drift detected" | PASS |
| Real-dispatcher wiring (throwaway test, deleted after run) | dispatched chat_owner_changed, migrate and `/staff` updates | link removed + notice sent; Staff Group and link re-keyed; `/staff` replied | PASS |
| Full `make test` | not re-run | Orchestrator reports 60 packages ok after each wave | SKIP (reported, not reproduced) |

### Probe Execution

Step 7c: SKIPPED. No probes are declared by any plan or summary, and the repo has no `scripts/*/tests/probe-*.sh` for this phase.

### Requirements Coverage

Requirement IDs declared across the ten PLAN frontmatters, cross-referenced against `.planning/REQUIREMENTS.md`. Every ID is accounted for, and no Phase 1 ID is orphaned (SETUP-09 is mapped to Phase 3).

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| SETUP-01 | 01-01, 01-02 | Owner designates a Staff Group; unusable chats refused with a reason | SATISFIED | Truth 1 |
| SETUP-02 | 01-02 | Only the owner removes status; all links go | SATISFIED | Truth 1 |
| SETUP-03 | 01-05 | Owner of both can link | SATISFIED | Truth 2 |
| SETUP-04 | 01-04, 01-05 | Linking refused unless owner of both (live) or if already linked | SATISFIED | Truth 2; the DB-level role exclusivity trigger awaits a real-PostgreSQL human check |
| SETUP-05 | 01-06 | Owner of both can unlink | SATISFIED | Truth 2 (see WR-02 for the deleted-group edge) |
| SETUP-06 | 01-07, 01-08, 01-09, 01-10 | Automatic removal when owner changes; watched plus rechecked | SATISFIED in code | Truth 3. The "re-checked before every staff action" half is Phase 2's caller of `recheckLink`, which is ready. Real-Telegram delivery is a human item. |
| SETUP-07 | 01-03, 01-10 | Links and status survive a chat-ID change | SATISFIED | Truth 4 |
| SETUP-08 | 01-01, 01-08, 01-09 | `/staff` panel with help text and per-group status | SATISFIED | Truth 5 |
| PLAT-03 | 01-01, 01-02, 01-05, 01-06, 01-07, 01-08, 01-09 | All new text in all 7 languages | SATISFIED | Truth 5 locale checks |

Bookkeeping to update after sign-off: `REQUIREMENTS.md` still shows SETUP-01 to SETUP-07 as unchecked and "Pending" (only SETUP-08 and PLAT-03 are marked Complete), and `01-VALIDATION.md` is still `status: draft`, `nyquist_compliant: false`.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (phase files) | n/a | `TBD`, `FIXME`, `XXX`, `TODO`, `HACK`, `PLACEHOLDER`: none found in the staff modules, repository, model, owner helper or migrations | none | Debt-marker gate clear |
| `staff_sweeper.go:99`, `staff_notify.go:42` | n/a | Goroutine / `AfterFunc` | none | Both start with `defer error_handling.RecoverFromPanic(...)` |
| (phase files) | n/a | Discarded DB errors on state-changing paths | none found | Every write returns or logs its error; sweeper and watchers never delete on an error |

### Code Review Findings: Assessment Against the Success Criteria

`01-REVIEW.md` found 0 critical, 6 warnings, 4 info; all are `open` in `01-REVIEW-DISPOSITION.md`. I read the cited code for each. None turns a success criterion false. The first six are real defects worth triaging.

| ID | Real? | Does it break a must-have? | Assessment |
|----|-------|----------------------------|------------|
| WR-02 | Yes (confirmed in `checkUnlinkAuthority`) | Edge of SC2 / SETUP-05 | A link to a deleted or bot-kicked group can never be unlinked on its own: the live owner check returns `Unknown`, the sweeper only marks `bot_missing`, and the command must be run inside the dead group. The only way out is `/unsetstaff`, which drops every link. The criterion "owner of both can unlink" holds for every reachable group. Recommend fixing: let the verified Staff Group creator drop a link whose group is provably gone. |
| WR-05 | Yes (confirmed in `createStaffGroup`) | Edge of SC1 / SC3 | After a Staff Group ownership transfer, the new creator's `/setstaff` says "already a Staff Group" and does not refresh the recorded owner. `/linkstaff` without an argument then fails for the real owner until a watcher update or the hourly sweep corrects it; `/linkstaff <id>` works at once. Self-healing, low risk. One-line fix with `staff.UpdateStaffGroupOwner`. |
| WR-01 | Yes | No | The Staff Group notice for owner-change removal and for `/unsetstaff` Confirm is unbounded; with roughly 40-60 links it can exceed 4096 characters and fail after the data is already gone. The links are still removed as required. Unlikely at this deployment's scale. Fix before Phase 2 reuses the pattern for fan-out reporting. |
| WR-03 | Yes | No | Confirm, Cancel and expired-press answer the callback only after a full live panel rebuild, which can take up to 45 s, so the query can go stale. The state change has already happened. Fix: answer first, as the Refresh path already does. |
| WR-04 | Yes | No | `/unsetstaff` Cancel has no authority check, so any member can dismiss the creator's prompt. Confirm is correctly gated, so status cannot be removed by a bystander. |
| WR-06 | Yes | No | `runUnlinkGroup` deletes by row ID after the live proof; a concurrent re-key could race. Very low probability and cannot delete a different group. |
| IN-01 | Yes (confirmed) | No | The docs page table omits `/linkstaff` and `/unlinkstaff` and says commands are open to all users. The intro text is correct. |
| IN-02 | Yes | No | Source comments cite planning IDs, one inaccurately; the panel does not re-key a migrated linked group, so it shows "unknown" until the next sweep. |
| IN-03 | Yes | No | A single "no creator listed" answer removes every link of a Staff Group. It follows the documented mismatch rule and is covered by a test, but it is a bulk, unrecoverable action on one response. Consider treating "no creator" as unknown for the Staff-Group-side sweep. |
| IN-04 | Yes | No | Duplicate 64-rune cap constants; stored titles never refresh after a rename; the sweep lock `SetNX` has no timeout and could delay shutdown inside the 60 s budget if Redis hangs. |

### Human Verification Required

Harvested from every `<verify><human-check>` block in the plans (01-02, 01-05, 01-07, 01-09, 01-10), every summary's pending human item, and the VALIDATION manual-only table. They are listed in the frontmatter `human_verification` section and summarised here:

1. **Anonymous-admin posting** (setstaff, linkstaff, picker): confirm the refusal and that the non-anonymous path works.
2. **Add group picker** with the bot already in the group, not in it, and anonymous: confirm rights are kept and the payload arrives as designed.
3. **Ownership transfer, bot admin** (linked group, then Staff Group): record which updates arrive and how fast the link disappears. This decides whether the hourly sweeper is the only remover.
4. **Ownership transfer, bot plain member**: confirm a non-admin bot can read the admin list and the sweep removes the link.
5. **Bot removal, demotion and re-add**: one heads-up per change, nothing in the linked group.
6. **`/staff` panel appearance and Refresh**, including paging past 8 groups.
7. **Native-speaker read** of the 63 `staff_*` strings in es, fr, hi, id, pt, ru.
8. **Unlink leaves no trace** in the linked group.
9. **Real chat-ID change** (supergroup upgrade) of a Staff Group and of a linked group.
10. **Real PostgreSQL**: migrations via `AUTO_MIGRATE=true` and `make test-postgres-integrity` for the role exclusivity trigger. This is the only coverage of the D-10 cross-replica guarantee, and it could not be reproduced here.
11. **Two-replica concurrent `/unsetstaff` Confirm** (plan-level backstop truth; design supports it, no test simulates it).

### Gaps Summary

No gaps. Every roadmap success criterion is implemented, wired from the dispatcher and `main.go` down to the database, and exercised by passing tests. The verifier additionally pushed migrate, owner-changed and `/staff` updates through a real dispatcher, which no committed test does. The phase is not `passed` only because the eleven human checks above are outstanding, most importantly the real-Telegram ownership-transfer behaviour (SC3) and the PostgreSQL trigger run (SETUP-04's database-level guarantee). Recommended before Phase 2: fix WR-02 and WR-05, and answer-first for WR-03; carry WR-01's chunking into Phase 2's fan-out reporting.

---

_Verified: 2026-10-05T00:25:00Z_
_Verifier: Claude (gsd-verifier)_
