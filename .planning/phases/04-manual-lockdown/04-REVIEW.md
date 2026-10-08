---
phase: 04-manual-lockdown
reviewed: 2026-10-07T04:10:00Z
depth: standard
files_reviewed: 60
files_reviewed_list:
  - AGENTS.md
  - Makefile
  - alita/db/lockdown/joiners.go
  - alita/db/lockdown/joiners_lift_test.go
  - alita/db/lockdown/joiners_test.go
  - alita/db/lockdown/repository.go
  - alita/db/lockdown/repository_test.go
  - alita/db/lockdown/testmain_test.go
  - alita/db/models/lockdown.go
  - alita/db/testmain_test.go
  - alita/i18n/lockdown_locale_test.go
  - alita/modules/antiraid.go
  - alita/modules/bans.go
  - alita/modules/captcha.go
  - alita/modules/chat_permissions.go
  - alita/modules/chat_permissions_test.go
  - alita/modules/greetings.go
  - alita/modules/lockdown.go
  - alita/modules/lockdown_anon_test.go
  - alita/modules/lockdown_decide_test.go
  - alita/modules/lockdown_durability_test.go
  - alita/modules/lockdown_fake_test.go
  - alita/modules/lockdown_guard.go
  - alita/modules/lockdown_guard_policy_test.go
  - alita/modules/lockdown_guard_test.go
  - alita/modules/lockdown_isolation_test.go
  - alita/modules/lockdown_lift_joiners_test.go
  - alita/modules/lockdown_lift_test.go
  - alita/modules/lockdown_marker.go
  - alita/modules/lockdown_paths_test.go
  - alita/modules/lockdown_perms.go
  - alita/modules/lockdown_perms_test.go
  - alita/modules/lockdown_refusals_test.go
  - alita/modules/lockdown_requests_test.go
  - alita/modules/lockdown_staff_test.go
  - alita/modules/lockdown_status.go
  - alita/modules/lockdown_status_test.go
  - alita/modules/lockdown_test.go
  - alita/modules/lockdown_unmute_test.go
  - alita/modules/lockdown_worker.go
  - alita/modules/mute.go
  - alita/modules/staff_action_decide.go
  - alita/modules/staff_action_decide_test.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_panel.go
  - alita/modules/staff_panel_render_test.go
  - alita/modules/test_harness_test.go
  - alita/utils/chat_status/chat_status.go
  - docs/src/content/docs/commands/lockdown/index.md
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
  - migrations/20261006120000_add_chat_lockdowns.sql
findings:
  critical: 1
  warning: 2
  info: 4
  total: 7
status: issues_found
---

# Phase 04: Code Review Report

**Reviewed:** 2026-10-07T04:10:00Z
**Depth:** standard
**Files Reviewed:** 60
**Status:** issues_found

## Summary

Reviewed the Manual Lockdown phase: the `chat_lockdowns` / `chat_lockdown_joiners` schema and repositories, the group `-7` join guard, the DB-driven worker, `/lockdown`, `/unlockdown` and `/lockdownstatus`, the `resolveUnmutePermissions` change, the greetings Accept-button refusal, the antiraid step-aside, the `/staff` marker and the staff-ban-over-lockdown-ban branch. For the pre-existing files only the phase diff was reviewed.

The core design holds up. Every state move is one conditional update. The restore-before-record order in `/unlockdown` is correct. Names, reasons and Telegram error text are escaped and spliced in after translation. Live `getChatMember` authority is used throughout. The gotgbot error path strips the bot token before it can reach `telegramErrorDetail`. All 7 locales carry every key and every placeholder matches. `make check-translations` and `make check-docs` pass here, and the `alita/db/lockdown` and `alita/i18n` tests pass.

Gaps found:
- One path contradicts owner decision D-05: a deliberate ban can be turned into a lockdown ban and then lifted.
- One failure path destroys the only copy of the pre-lockdown permissions.
- A lockdown's bans silently expire after 330 days.

Known accepted items (the D-24 race, `make lint` not runnable, the anonymous-admin RequireGroup issue) are not re-reported.

## Critical Issues

### CR-01: The worker's ban overwrites a deliberate ban, and the lift then removes it (violates D-05)

**File:** `alita/modules/lockdown_worker.go:366-390` (also `alita/db/lockdown/joiners.go:128-142`, `alita/modules/lockdown_marker.go:20-29`)

**Issue:** `lockdownBanOne` calls `banChatMember(chat, user, until_date=row.BanUntil)` for a pending row without first looking at the user's live status. The guard records the joiner as `pending` the moment they join, and the worker bans later. Under a raid that gap can be long. The pacer spaces calls 100 ms apart, `lockdownWorkerBatch` is 50 and a slot up to 60 s away is still accepted, so hundreds of rows can queue.

During that gap an admin, or a staff `/ban` fan-out, may ban the joiner deliberately:
- Typically that ban is permanent, or a `/tban`.
- The worker's later `banChatMember` then changes the already-kicked user's end date to the lockdown marker (`now+330d`). The repo already relies on this behaviour, because `decideStaffBan` sends a ban to a kicked target to change its end date.
- At the lift, `isLockdownBan` sees `kicked` with `until_date == ban_until` and unbans the person.

The deliberate ban is lifted without anyone asking, and the tally reports the person as "unbanned". D-05 says the lift never lifts a deliberate ban. The staff-ban-over-lockdown-ban branch added in this phase covers only the opposite order (lockdown ban first, staff ban second). This order is unprotected.

**Fix:** Make the ban conditional on the live state, the way `/unban` and staff `decideStaffAction` already are. Check `getChatMember` inside the same paced unit. If the member is already `kicked`, move the row to `kept` and make no call:

```go
member, err := b.GetChatMemberWithContext(call, row.ChatID, row.UserID, nil)
if err == nil && member.MergeChatMember().Status == gotgbot.ChatMemberStatusKicked {
    moveLockdownJoiner(row.ID, models.JoinerStateActing, models.JoinerStateKept, "", false)
    return true
}
// otherwise ban with UntilDate: row.BanUntil
```

Add a test that bans the user directly between `RecordJoin` and the worker cycle, then asserts that the user is still banned after `/unlockdown`.

## Warnings

### WR-01: An ambiguous lock failure deletes the only copy of the pre-lockdown permissions and says "nothing changed"

**File:** `alita/modules/lockdown.go:375-381`

**Issue:** When `setLockdownPermissions` returns any error, `/lockdown` calls `DeleteUnconfirmed(row.ID)` and replies "Telegram refused the lock, so nothing changed". `setLockdownPermissions` can fail without Telegram having refused anything:
- the 10 s `lockdownCallTimeout` expires after the request was applied;
- the connection drops;
- the answer cannot be decoded.

If the lock did take effect, the group is locked on Telegram, no lockdown row exists, and `pre_permissions` is gone with the row.
- `/unlockdown` answers "not in lockdown".
- The join guard does nothing.
- The admin has to rebuild the group's permission set by hand from memory.

The repo already has the right mechanism for exactly this uncertainty: `settleUnconfirmedLockdown` decides from the live permissions after the grace period. It is not used on this path. This is data loss for the data the phase's central safety property is built to protect.

**Fix:** Delete the row immediately only for a definitive refusal, a `*gotgbot.TelegramError`. For any other error, leave the row unconfirmed so the worker settles it from the live permissions, and tell the admin the result is unknown:

```go
var tgErr *gotgbot.TelegramError
if errors.As(err, &tgErr) {
    // definitive refusal: delete the unconfirmed row as today
} else {
    // ambiguous: keep the row; settleUnconfirmedLockdown confirms or deletes it
    return refuse("lockdown_lock_unknown", telegramErrorDetail(err))
}
```

Add a new `lockdown_lock_unknown` key to all 7 locale files, and a test whose fake applies the permissions and then times out.

### WR-02: Joiner bans expire after 330 days, so a lockdown can outlive its own enforcement

**File:** `alita/modules/lockdown_marker.go:31-40`

**Issue:** The "banned until the lift" mechanism (D-04) is implemented as a Telegram ban with `until_date = join time + 330 days`. The phase also states that a lockdown never ends on its own and has no TTL. An abandoned or forgotten lockdown, or one on a group whose admins left, therefore silently stops banning after 330 days:
- Each raider's ban lapses 330 days after their own join.
- They can rejoin a group that is still "locked".
- At the lift the live member is `left`, not `kicked`, so the row is counted as `kept` and reported as "someone had already unbanned them or banned them on purpose". That is untrue.

This is a limit of the marker design. Nothing documents it for the owner or states what happens, and AGENTS.md says "no expired ban lifts one" about the lockdown itself, which does not cover the bans.

**Fix:** Pick one of these and record it in AGENTS.md and `/lockdownstatus`:
- (a) Document the 330-day ceiling as an accepted limit.
- (b) Have the worker re-ban rows of an active lockdown whose `ban_until` is nearing expiry, with a fresh end date written to the row first.
- (c) Count a `left` member whose `ban_until` is in the past as "expired" in the tally instead of `kept`.

## Info

### IN-01: Duplicated join-policy conditions and an unreachable verdict

**File:** `alita/modules/lockdown_guard.go:60-90`

**Issue:** `lockdownNeedsPerformerLookup` re-states the first six conditions of `decideLockdownJoin`. If one is changed without the other, the performer lookup is skipped while the decision depends on it, or the reverse. A mistake there fails closed, as a ban, but it would still be a silent behaviour drift. `lockdownJoinDecline` is returned by `decideLockdownJoin` for `JoinPathRequest`, but no production caller passes that path. `lockdownOnJoinRequest` records the row directly, and `recordLockdownJoin` has a `default:` branch only for verdicts that never occur.

**Fix:** Derive the lookup need from the decision. For example, run `decideLockdownJoin` once with a sentinel status and check whether the result depends on it. Alternatively, delete the dead `Decline` path or have `lockdownOnJoinRequest` call it.

### IN-02: AGENTS.md says the snapshot never goes through the typed `ChatPermissions`, but `resolveUnmutePermissions` does that

**File:** `alita/modules/chat_permissions.go:66-80` (rule stated in `AGENTS.md`, Data section, `chat_lockdowns` bullet)

**Issue:** The rule is that `pre_permissions` is "never passed through the typed `gotgbot.ChatPermissions`". `resolveUnmutePermissions` unmarshals `PrePermissions` into `gotgbot.ChatPermissions`, whose `omitempty` bools and `*bool` fields lose "explicitly off". The code is defensible here. It is a per-user restriction in which an omitted false means restricted, and it matches what the live-defaults path did before. As written, the rule and the code disagree, and the next reader will either "fix" the code or stop trusting the rule.

**Fix:** Narrow the AGENTS.md sentence to "the group's lock and restore never pass it through the typed struct; the per-user unmute reads it only to build a user restriction".

### IN-03: A fourth copy of the Group Anonymous Bot ID

**File:** `alita/modules/lockdown_guard.go:19-21`

**Issue:** `1087968824` already exists as `chat_status.groupAnonymousBot`, `helpers.groupAnonymousBotID` and `staffServiceUserIDs`. This phase adds `lockdownGroupAnonymousBot`. A shared exported constant would remove the drift risk.

**Fix:** Export one constant from `chat_status` or `helpers` and use it here.

### IN-04: Fed-ban enforcement is skipped for banned joiners of a locked group, and a "request gone" is recorded as declined

**File:** `alita/modules/lockdown_guard.go:266-278` and `alita/modules/lockdown_worker.go:342-345`

**Issue:** The guard ends the update with `ext.EndGroups` at `-7` for a banned joiner, so fed-ban (`-6`, `enforceFedBan` on the join service message) never runs for them.
- A federation-banned user who joins a locked group is banned with the lockdown marker only.
- The lift unbans them.
- Fed-ban catches them again only on their next join.

This is a consequence of D-10 and D-03, not a defect in the code. It deserves a line in AGENTS.md and in the docs.

Separately, `lockdownDeclineOne` treats `USER_ALREADY_PARTICIPANT` as "declined". That is the case where an admin approved the request first, so `/lockdownstatus` "Join requests declined" can count people who were let in.

**Fix:** Document the fed-ban interaction. For the second point, move the row to `cancelled` instead of `declined` when the error is `USER_ALREADY_PARTICIPANT`.

---

_Reviewed: 2026-10-07T04:10:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
