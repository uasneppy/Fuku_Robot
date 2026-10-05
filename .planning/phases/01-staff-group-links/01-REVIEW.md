---
phase: 01-staff-group-links
reviewed: 2026-10-05T00:00:00Z
depth: standard
files_reviewed: 59
files_reviewed_list:
  - .claude/CLAUDE.md
  - AGENTS.md
  - Makefile
  - alita/db/cache/local.go
  - alita/db/cache/local_test.go
  - alita/db/models/staff.go
  - alita/db/staff/exclusivity_postgres_test.go
  - alita/db/staff/rekey.go
  - alita/db/staff/rekey_test.go
  - alita/db/staff/repository.go
  - alita/db/staff/repository_test.go
  - alita/db/staff/testmain_test.go
  - alita/db/testmain_test.go
  - alita/i18n/staff_locale_test.go
  - alita/modules/deeplink_router.go
  - alita/modules/help.go
  - alita/modules/staff.go
  - alita/modules/staff_health_test.go
  - alita/modules/staff_helpers_test.go
  - alita/modules/staff_link.go
  - alita/modules/staff_link_refusals_test.go
  - alita/modules/staff_link_test.go
  - alita/modules/staff_migrate_test.go
  - alita/modules/staff_notify.go
  - alita/modules/staff_ownership_test.go
  - alita/modules/staff_panel.go
  - alita/modules/staff_panel_render_test.go
  - alita/modules/staff_panel_test.go
  - alita/modules/staff_picker_test.go
  - alita/modules/staff_recheck.go
  - alita/modules/staff_setstaff_test.go
  - alita/modules/staff_sweeper.go
  - alita/modules/staff_sweeper_test.go
  - alita/modules/staff_test.go
  - alita/modules/staff_unlink.go
  - alita/modules/staff_unlink_button_test.go
  - alita/modules/staff_unlink_test.go
  - alita/modules/staff_unset_test.go
  - alita/modules/staff_watchers.go
  - alita/modules/test_harness_test.go
  - alita/utils/chat_status/owner.go
  - alita/utils/chat_status/owner_test.go
  - alita/utils/helpers/command_pipeline.go
  - alita/utils/helpers/command_pipeline_test.go
  - docs/src/content/docs/commands/staff/index.md
  - locales/config.yml
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - main.go
  - main_test.go
  - migrations/20261004120000_add_staff_groups_and_links.sql
  - migrations/20261004130000_add_staff_role_exclusivity_trigger.sql
  - scripts/check_test_results/main.go
  - scripts/check_test_results/main_test.go
findings:
  critical: 0
  warning: 6
  info: 4
  total: 10
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-10-05
**Depth:** standard
**Files Reviewed:** 59
**Status:** issues_found

## Summary

Reviewed the Staff Group / link feature: migrations (including the Postgres exclusivity trigger), `alita/db/staff`,
the `chat_status` live-owner helpers, all `staff*.go` modules, the group deep-link route, locales and the CI skip
gate. The authority model is sound: every authority read goes through uncached `*Fresh` reads plus live
`CheckOwner`, only `OwnerMismatch` removes state, anonymous senders are rejected, and removals/health changes are
single conditional statements. I traced the trigger's lock ordering (no deadlock cycle), the cache invalidation of
every write (all covered, including re-key and orphan cleanup), the callback codec limits, locale key parity (63
`staff_*` keys, identical placeholders in all 7 locales) and `AGENTS.md` handler/priority rules. `go build ./...` and
the staff tests (db/staff, chat_status, i18n, cache, and the module staff tests under `-race`) pass.

No security vulnerability or data-loss bug was found. The warnings are behavioural defects at the edges: unbounded
notice length after a destructive write, a link that can never be removed individually, callback answers delayed
behind a long live rebuild, an unauthenticated Cancel button, and a stale recorded owner on `/setstaff`.

## Warnings

### WR-01: Staff Group "owner changed" / "status removed" notices are unbounded and can be lost after the data is already deleted

**File:** `alita/modules/staff_recheck.go:106-120`, `alita/modules/staff.go:329-334`, `alita/modules/staff.go:347-361`
**Issue:** `postStaffOwnerChangedNotice` joins every removed title into one message, and `staffUnsetConfirm`
renders every removed link with `renderUnlinkedGroups`, with no length cap. Each row is up to 64 title runes (more
once HTML-escaped) plus `<code>id</code>`, so roughly 40-60 links exceed Telegram's 4096-character limit. Both
paths run only after `DeleteLinkIfOwner` / `DeleteStaffGroupWithLinks` have committed. The send/edit then fails with
MESSAGE_TOO_LONG, which is only logged (`sendStaffNotice` warn, `editStaffCallbackMessage` error). The result: the
links are gone and the Staff Group is told nothing; after `/unsetstaff` Confirm the prompt keeps its Confirm button
and the next press says "already removed". The panel already solves this problem (`staffPanelMaxUTF16` and
paging), but these two notices do not. This contradicts the project rule "report partial failures, not drop them".
**Fix:** Chunk the list into messages under ~3800 UTF-16 units (reuse the `fits` logic from `staffPanelTextFor`),
or cap the list and append an "and N more" line. For unset, edit the prompt with the count and send the list in
follow-up messages.

```go
// sketch: split escaped lines into chunks that fit, send each with sendStaffNotice
for _, chunk := range chunkByUTF16(lines, staffPanelMaxUTF16) {
    _ = sendStaffNotice(b, staffChatID, header+chunk)
}
```

### WR-02: A link to a deleted or unreachable group can never be unlinked on its own

**File:** `alita/modules/staff_unlink.go:52-72`, `alita/utils/chat_status/owner.go:101-107`
**Issue:** `checkUnlinkAuthority` requires a live `OwnerMatch` for the linked group. When the group is deleted,
or the bot was kicked, `getChatAdministrators` fails and `CheckOwner` returns `OwnerUnknown` by design (so it never
auto-removes). `recheckLink` and the sweeper therefore never remove the link either (they only act on
`OwnerMismatch`), and the sweeper just marks it `bot_missing`. Both `/unlinkstaff` (which has to be run in the
group, which no longer exists) and the panel's Unlink button answer "could not verify" forever. The only escape is
`/unsetstaff`, which unlinks every other group too. Removing a link only lowers authority, so allowing the already
verified Staff Group creator to drop a link whose group is provably gone does not weaken the security model.
**Fix:** In `checkUnlinkAuthority`, once the Staff Group owner is verified, treat a group-side error that matches
`isBotMissingError` (kicked / not a member / chat not found) as sufficient. Expose a small predicate from
`chat_status` rather than string-matching in the module. Add a test for the panel Unlink button on a
`chat not found` group.

### WR-03: Callback queries are answered only after a full live panel rebuild (up to 45 s)

**File:** `alita/modules/staff_unlink.go:231-247`, `alita/modules/staff_unlink.go:249-258`, `alita/modules/staff_unlink.go:411-418`
**Issue:** `staffPanelRebuild` deliberately answers the callback before the live checks ("they can outlast the
callback's validity"). `finishStaffPanelPress` (Confirm and Cancel) and `staffUnlinkExpired` do the opposite: they
call `staffRerenderPanel` -> `buildStaffPanel`, which performs up to 2N+1 sequential Telegram calls (8 s timeout
each, 4 in flight, 45 s overall), and only then call `answerStaffCallback`. For a Staff Group with a handful of
slow/unknown links the answer arrives after Telegram has invalidated the query: the button spinner hangs and the
answer fails with "query is too old". The link has already been deleted at that point, so the user sees no
confirmation. It also pins a dispatcher goroutine for the duration.
**Fix:** Answer first, then rebuild, as `staffPanelRebuild` does:

```go
func finishStaffPanelPress(b *gotgbot.Bot, query *gotgbot.CallbackQuery, tr *i18n.Translator) {
    answerStaffCallback(b, query, "", false)
    if err := staffRerenderPanel(b, query.Message, tr); err != nil && !errors.Is(err, errStaffPanelGone) {
        log.Warnf("[Staff] re-render after unlink: %v", err)
    }
}
```

For `staffUnlinkExpired`, answer with the expired toast first and refresh afterwards.

### WR-04: `/unsetstaff` Cancel button has no authority check

**File:** `alita/modules/staff.go:340-345`
**Issue:** `staffUnlinkCancel` was explicitly hardened ("so a bystander cannot dismiss the creator's prompt"), but
`staffUnsetCancel` edits the message and answers with no check at all. Any member of the Staff Group (or any chat
where such a message exists) can press Cancel on the creator's confirmation and replace it with "Cancelled. This
group is still a Staff Group." That text is also asserted unconditionally, even if the group was already unset in
the meantime. This is inconsistent with the sibling buttons and with the module's own stated rule.
**Fix:** Require the live creator before cancelling (reuse `chat_status.CheckOwner` as in `staffUnsetConfirm`), or
at least verify `GetStaffGroupFresh` before claiming the group is still a Staff Group.

### WR-05: `/setstaff` on an existing Staff Group never refreshes the recorded owner

**File:** `alita/modules/staff.go:130-151`, `alita/db/staff/repository.go:69-102`
**Issue:** `CreateStaffGroup` is deliberately idempotent and leaves the existing row's `owner_user_id` untouched.
After ownership of a Staff Group is transferred, the new live creator who runs `/setstaff` passes the live creator
check, is told "already a Staff Group", and the recorded owner stays the old user until a watcher update or the
hourly sweep (the watcher depends on `chat_owner_changed` / `chat_member` updates that the code itself calls
undocumented and bot-admin dependent). Until then `/linkstaff` without an argument
(`resolveOwnStaffGroup`, which narrows candidates by `owner_user_id`) answers "You do not own a Staff Group yet" to
the real owner. The repository already has `UpdateStaffGroupOwner` for exactly this.
**Fix:** In `createStaffGroup`, when `!created`, call `staff.UpdateStaffGroupOwner(c.Chat.Id, c.User.Id)` (the live
creator has just been verified) before replying, and log the error without refusing.

### WR-06: Two links resolved from a stale panel row can be unlinked without re-reading the link after the live checks

**File:** `alita/modules/staff_unlink.go:78-95`
**Issue:** `runUnlinkGroup` receives a `models.StaffGroupLink` value loaded before two sequential live API calls
(up to 16 s) and finally deletes with `staff.DeleteLink(link.ID)`, an unconditional delete by ID. Everywhere else
the module uses a conditional claim (`DeleteLinkIfOwner`). The row ID is never reused so this cannot delete the
wrong group, but the code deletes whatever the row currently is: if the link's group was re-keyed
(supergroup migration) while the checks ran, the ownership proof was made against the old `GroupChatID` and the
delete still proceeds. This is low probability, but the proof and the write are not tied together.
**Fix:** Make the delete conditional on the row still having the proven `(group_chat_id, staff_chat_id)`, for example
`DeleteLinkIfUnchanged(id, groupChatID, staffChatID)`, and map "no row" to `staffUnlinkGone`.

## Info

### IN-01: Documentation page omits `/linkstaff` and `/unlinkstaff` from the command table and states the wrong permission

**File:** `docs/src/content/docs/commands/staff/index.md:27-47`
**Issue:** "Available Commands" and "Usage Examples" list only `/setstaff`, `/staff`, `/unsetstaff`; the two link
commands are missing even though the aliases list names them. "Required Permissions" says commands are available
to all users, but setstaff/unsetstaff/linkstaff/unlinkstaff are creator-only. The page is generated from
`locales/en.yml`, so this is a generator limitation (it reads command registrations/`help` text), but the output is
misleading as shipped.
**Fix:** Mark the page `<!-- MANUALLY MAINTAINED: do not regenerate -->` and correct it, or teach the generator
about the module's commands.

### IN-02: Comments reference planning artifacts and plans, one of them inaccurate

**File:** `alita/modules/staff_watchers.go:154-155`, `alita/modules/staff_panel.go:406`, `alita/modules/staff_sweeper.go:226`
**Issue:** Source comments cite planning IDs (`T-01-29`, `P4`, `D-10`, `D-15.2`, "Plans 01-09 (panel) and 01-10
(sweeper)"). These are meaningless once `.planning/` is archived. The `rekeyFromTelegramError` comment is also wrong:
the panel (01-09) never calls it; only the sweeper and `recheckStaffGroup` do. Consequently a linked group that
migrated while the bot was down shows as "unknown" in the panel until the next hourly sweep.
**Fix:** Replace plan/decision IDs with the actual rule in prose, and either call `rekeyFromTelegramError` from
`buildStaffPanelRows` when `FetchBotMember` is `Unknown`, or fix the comment.

### IN-03: A single "no creator listed" observation removes every link of a Staff Group

**File:** `alita/modules/staff_recheck.go:262-299`, `alita/utils/chat_status/owner.go:119`
**Issue:** `CheckOwner` returns `(OwnerMismatch, 0, nil)` when the admin list has no creator. `recheckStaffGroup`
then proceeds with `liveOwner == 0`, and `link.OwnerUserID != 0` is true for every link, so all links are deleted in
one pass. That follows the documented rule, but it is a bulk, unrecoverable action based on one response with no
second look, and the notice lists the groups only. Consider requiring `liveOwner != 0` before the bulk removal (treat
"no creator" as Unknown for the Staff-Group-side sweep) or confirming on a second pass.
**Fix:** `if liveOwner == 0 { summary.Unknown = true; return summary }` before listing links, with a test.

### IN-04: Minor duplication and unbounded Redis call in the sweeper

**File:** `alita/db/staff/repository.go:24`, `alita/modules/staff_panel.go:26`, `alita/modules/staff_sweeper.go:171`
**Issue:** The 64-rune title cap exists twice (`maxStaffTitleRunes`/`trimStaffTitle` and
`maxStaffDisplayTitleRunes`/`staffDisplayTitle`); stored titles are never refreshed after a group rename, so panels
show stale names. `acquireStaffSweepLock` uses `cache.Context` with no timeout, so a hung Redis blocks the sweeper
goroutine and, through `staffSweepWG.Wait()`, delays shutdown inside the 60 s budget.
**Fix:** Export one cap constant; refresh `group_title` in `applyLinkHealth`/sweep when it differs; wrap the
`SetNX` in `context.WithTimeout(cache.Context, 5*time.Second)`.

---

_Reviewed: 2026-10-05_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
