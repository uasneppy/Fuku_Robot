---
phase: 10-pm-settings-hub-and-owner-gate
reviewed: 2026-11-04T00:00:00Z
depth: standard
files_reviewed: 20
files_reviewed_list:
  - fuku/modules/settings_gate.go
  - fuku/modules/settings_access.go
  - fuku/modules/settings_hub.go
  - fuku/modules/settings_menu.go
  - fuku/modules/settings_pages.go
  - fuku/modules/settings.go
  - fuku/modules/lockdown.go
  - fuku/modules/admin_index_feed.go
  - fuku/modules/admin_index_sweeper.go
  - fuku/modules/bot_updates.go
  - fuku/utils/cache/adminCache.go
  - fuku/db/adminindex/repository.go
  - fuku/db/menuaccess/repository.go
  - fuku/db/models/chat_admin_index.go
  - fuku/db/models/chat_settings_access.go
  - migrations/20261104120000_add_settings_hub_tables.sql
  - main.go
  - Makefile
  - locales/en.yml
  - locales/uk.yml
findings:
  critical: 0
  warning: 3
  info: 4
  total: 7
status: issues_found
---

# Phase 10: Code Review Report

**Reviewed:** 2026-11-04
**Depth:** standard
**Files Reviewed:** 20
**Status:** issues_found

## Summary

The phase delivers the owner gate, the Access page, the private group list, the roster feed and the hourly sweeper. I traced every door (group `/settings` and its anonymous-admin re-entry, the `/start settings_<id>` deep link, every `settings|` press including the private pick, the `shub|` list buttons) and all of them call `settingsGate` after their own live `getChatMember`. I found no way for a non-owner administrator to get in while the switch is off, and no way for a non-creator to change the switch:

- Access controls and the `ay` action carry `settingsRightOwner`, and `settingsAccessApply` and `openPage` check the creator status again behind it. A forged `tg`, `st`, `ay` or `op&p=a` from an admin is refused.
- Every unreadable switch, every Telegram error or timeout and every unusable owner list ends `undecided` and refuses.
- Only "an owner exists" is memoised, and that can only keep an administrator out.

`go build` and `go vet` (with `-tags testtools`) on the touched packages are clean. `make check-translations` passes, and the `{placeholders}` of the new keys match across all 8 locale files. `lockdownAuthorityMember` preserves the previous behavior of `requireLockdownAuthority` exactly. The cache observer only adds latency and a recovered panic path to the existing admin-cache load and does not change what it returns or stores.

The remaining problems are one authorization heuristic that can fail open, one place where the phase's "a forged `c` must not reveal whether the bot is in a chat" rule is only half applied, and a client-error classifier that is too broad for a destructive roster clear.

## Warnings

### WR-01: A live owner whose first name is "Deleted Account" (no last name, no username) is treated as "no owner", which lets every admin in

**File:** `fuku/modules/settings_gate.go:241-243` (also `fuku/modules/admin_index_feed.go:124`, `fuku/modules/misc_extras.go:40-43`)
**Issue:** `settingsCreatorGone` is `isDeletedAccount(first_name) && last_name == "" && username == ""`. The comment and AGENTS.md claim that a live owner who names themselves "Deleted Account" "keeps one of the two" (a username or a last name) and so still counts. That is not a guarantee. Many accounts have no username and no last name, and Telegram lets a user set the first name to "Deleted Account". `isDeletedAccount` is also `EqualFold` and true for an empty name. Failing scenario: the owner of a group has first name "Deleted Account" and no username. The group has the switch off. Any administrator opens `/settings`. `settingsEvaluateOwner` sees a creator entry that `settingsCreatorGone` calls gone, so `liveOwner` stays false, the result is `settingsOwnerNone`, and `settingsGate` returns `settingsGateIn`. The owner-only default is silently void for that group. `adminIndexEntries` makes the same call and drops the creator row, so `ListForUserFresh` lists the group for every admin as well. The failure is limited to owners with that exact display name, and the owner is not harmed in the sense that they still pass. It is still an authorization decision resting on a display string, which is the one place this design is not fail-closed.
**Fix:** Corroborate before opening the group. For example, treat the creator as gone only when the name matches and a live `getChatMember(chatID, creator.Id)` also shows no usable account (an error that names a missing user, or a status that is not creator). Otherwise answer `settingsOwnerExists`. If no second signal exists, keep the name test but treat a match as `settingsOwnerUnknown` (refuse) unless an explicit, documented override exists. Correct the comment and AGENTS.md sentence either way.

### WR-02: A forged `c` on a non-pick private press still tells the sender whether the bot is in that chat

**File:** `fuku/modules/settings_menu.go:340-345` and `fuku/modules/settings_menu.go:383-386` (the `IsBotMissingError` to `cannotOpen` branches), and `fuku/modules/settings_menu.go:352` (`settings_not_admin`)
**Issue:** The phase makes the pick answer one neutral `settings_pick_unavailable` for every 4xx and every non-admin status, and the deep link now does the same, precisely because `c` is attacker-controlled callback data. The other private actions (`oh` excluded, but `op`, `bk`, `cl`, `tg`, `st`, `no` and every page action, all of which are allowed with `c` by `settingsCheckFields`) still use the old mapping. Failing scenario: a user sends a hand-built callback `settings|v1|a=op&p=l&c=-100<X>` from their own private chat (callback data is not authenticated). If the bot is not in chat X, the answer is the `settings_cannot_open_alert` alert plus an edit to `settings_cannot_open`. If the bot is in X and the user is not an administrator, the answer is `settings_not_admin`. A user can enumerate which chat IDs the bot is a member of, exactly the oracle D-11/R3 was meant to close for the pick and the deep link.
**Fix:** Apply the pick's mapping to every `c` action in step 4: for a private press, answer any 4xx at `getChatMember` or `getChat` and any non-admin status with one neutral refusal (for example `settings_pick_unavailable`, or `settings_not_admin` for both). Keep the "menu closes itself" behavior only when the presser is already proven an administrator of that chat (that is, after the member check passed), which tells nobody anything new. Update AGENTS.md D-11 accordingly.

### WR-03: The roster clear fires on any Telegram 4xx of the bot's own `getChatMember`, including 401 and 403 causes unrelated to the chat

**File:** `fuku/modules/admin_index_feed.go:79-84`, `fuku/utils/cache/adminCache.go:149-151` (`isBotLookupClientError`), `fuku/modules/settings_gate.go:249-255` (`isTelegramClientError`)
**Issue:** "Definitive fact: the bot cannot see the chat" is derived from `Code in 400..499 && Code != 429`. That includes 401 (invalid or revoked token, or a Telegram-side auth fault), 409 and any 400 whose text is not about this chat. In the sweeper this is applied to every candidate chat in a row. Failing scenario: the sweep (or an admin-cache load) runs while Telegram answers 401 for the token, for example just after a token rotation where an old replica is still running. For every chat `refreshChatAdminIndex` calls `adminindex.ClearChat`, so `chat_admin_index` is emptied for the whole fleet, and the private `/settings` list is empty ("no groups to show") for every user until the next sweep an hour later or until each group's admin cache reloads. The roster is a hint, so this is not an authorization failure, but it is the destructive branch of a "changes rows only on definitive facts" function, and the classification is broader than the fact.
**Fix:** Clear only on answers that mean the bot is not in the chat: 403 and 400 with a description recognised by `chat_status.IsBotMissingError` ("bot was kicked", "bot is not a member", "chat not found"), or a status other than administrator or creator. Treat 401 and every other 4xx as "unchanged" and log it. Optionally stop a sweep after N consecutive clears (or after a 401) so one fleet-wide fault cannot wipe the table.

## Info

### IN-01: A group's "Admins can change settings" switch is lost silently when a basic group migrates to a supergroup

**File:** `migrations/20261104120000_add_settings_hub_tables.sql:19-27`, `fuku/db/menuaccess/repository.go:50-63`
**Issue:** `chat_settings_access` is keyed by the chat ID and is not re-keyed on `migrate_to_chat_id` (AGENTS.md says only the Staff Group rows are). The new supergroup has no row, so it is owner-only again and the old row stays orphaned. This fails in the safe direction, but the owner is not told that an ON setting was reset.
**Fix:** Either re-key `chat_settings_access` in the same place `staff.RekeyChat` runs on migration, or document the reset in the Access page text and AGENTS.md.

### IN-02: The private list can show a group title to someone the roster has not yet noticed was demoted

**File:** `fuku/db/adminindex/repository.go:145-218`, `fuku/modules/settings_hub.go:179-191`
**Issue:** The list is built from the stored roster only. A demoted or removed administrator stays listed (with the group's current `chats.chat_name` as the button label) until a `chat_member` update, an admin-cache reload, `/settings` in the group or the hourly sweep refreshes the rows. The pick refuses them live, so nothing can be changed, but the title is shown for up to that window. The phase treats the roster as a hint, so this is accepted, but it is worth knowing that the title is the one thing the stale hint discloses.
**Fix:** None required. If the exposure matters, drop a roster row at the moment of the demotion (the `chat_member` handler already reloads the cache and can remove that user's row) instead of waiting for the next full replace.

### IN-03: `main.go` comment says the admin index sweeper stop "returns at once", but it waits for the sweep goroutine

**File:** `main.go:209-218`, `fuku/modules/admin_index_sweeper.go:99-113`
**Issue:** `StopAdminIndexSweeper` calls `adminIndexSweepWG.Wait()`. It returns promptly in practice because every Telegram call and the roster write derive from the cancelled sweep context, and the lock release is bounded to 2 s, so it fits the 10 s default handler timeout and the drain ordering (LIFO: it runs after the staff, lockdown and raid drains and before the staff sweeper stop and the database close) is correct. The comment overstates it, and a database call that ignores cancellation would hold the drain until its 10 s timeout.
**Fix:** Reword the comment to "returns once the in-flight call, bounded by the cancelled sweep context, ends".

### IN-04: Every admin-cache load in a group where the bot is not an administrator now does a database transaction

**File:** `fuku/utils/cache/adminCache.go:159-163`, `fuku/modules/admin_index_feed.go:45-46`
**Issue:** `AdminListBotNotAdmin` calls `adminindex.ClearChat`, which opens a transaction (and on PostgreSQL takes an advisory lock) to delete rows that are usually already absent, synchronously under the chat's admin-cache singleflight with a 3 s timeout. The cache result is unchanged, but while the database is slow every such load, and every caller waiting on its singleflight, is delayed by up to 3 s. This is the accepted R-05.1 trade-off, and performance is out of v1 scope, so it is informational only. A cheap improvement is to skip the write when a read shows the chat has no rows.
**Fix:** Optional: in `ClearChat`, run a non-locking existence check first, or have the observer ignore `AdminListBotNotAdmin` for a chat it has no rows for.

---

_Reviewed: 2026-11-04_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
