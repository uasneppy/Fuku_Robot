---
phase: 13-member-requirements
reviewed: 2026-11-08T00:00:00Z
depth: deep
files_reviewed: 27
files_reviewed_list:
  - migrations/20261106120000_add_member_requirements.sql
  - fuku/db/models/member_requirements.go
  - fuku/db/memberreq/holds.go
  - fuku/db/memberreq/settings.go
  - fuku/db/approvals/repository.go
  - fuku/db/cache/local.go
  - fuku/modules/memberreq.go
  - fuku/modules/memberreq_rules.go
  - fuku/modules/memberreq_settings.go
  - fuku/modules/memberreq_watch.go
  - fuku/modules/memberreq_enforce.go
  - fuku/modules/memberreq_limits.go
  - fuku/modules/memberreq_notice.go
  - fuku/modules/memberreq_unmute.go
  - fuku/modules/memberreq_join.go
  - fuku/modules/memberreq_fixed.go
  - fuku/modules/settings_mreq.go
  - fuku/modules/settings_pages.go
  - fuku/modules/settings_menu.go
  - fuku/modules/chat_permissions.go
  - fuku/modules/captcha.go
  - fuku/modules/greetings.go
  - fuku/modules/membership.go
  - fuku/modules/warns.go
  - fuku/modules/bans.go
  - fuku/modules/shutdown_drains.go
  - main.go
findings:
  critical: 1
  warning: 3
  info: 6
  total: 10
status: issues_found
---

# Phase 13: Code Review Report

**Reviewed:** 2026-11-08
**Depth:** deep
**Files Reviewed:** 27
**Status:** issues_found

## Summary

All 27 files were read in full or, for the shared files, as the phase diff against
`1915dccb`. `CGO_ENABLED=0 go build ./...` is clean. The core design holds up. The
insert-first claim, the 90 s job against the 2 minute stale claim, the tri-state
exemptions, the fixed-order "I fixed it" handler, the Redis brake, the shutdown
ordering and the settings page field validation are sound. There is one real
authorization problem, in the marker mute. The punish-once guarantee has one hole on
the warn path. A switched-off rule keeps punishing from an already-queued job. There
is also one place where a member's mute can be stranded. Everything else is low
severity.

## Critical Issues

### CR-01: The requirement Mute replaces an existing admin/staff restriction, and "I fixed it" / the rejoin reset then lift something that was not the requirements' own

**Files:** `fuku/modules/memberreq_enforce.go:162-173` (`memberReqLiveEligible`),
`:394-404` (mute apply), `:462-505` (`memberReqEnforce`); entry points
`fuku/modules/memberreq_join.go:71-121` and `fuku/modules/memberreq_watch.go:191-231`.

**Issue.** `memberReqLiveEligible` deliberately accepts `restricted` with `IsMember`.
Nothing after it checks whether that restriction is already someone else's.
`memberReqApplyStep` then sends `restrictChatMember(MutedPermissions, until=marker)`.
That call replaces the member's whole restriction, as AGENTS.md says of
`restrictChatMember`. `isMemberReqMute` only protects the lift side (a mute an admin
placed after ours replaces the marker). It does nothing for the reverse: the
requirement mute replacing a restriction that was already there.

Two concrete paths:

1. **Join entry, admin-muted member.** An admin permanently mutes a spammer (status
   `restricted`, `until_date` 0). The spammer leaves and rejoins. The restricted-to-restricted
   rejoin reaches only the `new_chat_members` service message, then
   `ProcessSingleJoin` → `memberReqJoinThenWelcome` with Check on join on and a
   Mute rule. Live status is `restricted` and `IsMember`, so the member is eligible. A
   marker mute replaces the admin's permanent mute and a notice with an "I fixed
   it" button is posted. The member fixes their name and taps it. The marker matches,
   so `memberReqLiftMute` restores the group defaults. The admin's mute is gone and
   the member freed it themselves.
2. **Watcher, partially restricted member.** An admin restricts a member's media or
   stickers only (`can_send_messages` stays true). That member posts text with a name
   that breaks a Mute rule. The mute replaces the partial restriction. After "I fixed
   it" they get the group defaults, which silently drops the admin's restriction.

The same applies to `memberReqUndeliveredMute` (`memberreq_notice.go:183-201`),
which lifts to defaults without a marker check.

**Fix.** Do not mute a restricted member who does not carry this hold's own marker.
Add the check in `memberReqEnforce`, after the live lookup and before the cap and
claim:

```go
if action == models.RequirementActionMute &&
    member.Status == gotgbot.ChatMemberStatusRestricted &&
    !memberReqLiveMarkerPresent(stale, member) { // marker of the stale hold being adopted, else false
    log.Infof("[MemberReq] user %d in chat %d is already restricted by someone else, so no mute is placed", c.User.Id, c.Chat.Id)
    memberReqDeleteMessage(ctx, b, c) // live lookup showed a non-admin violator
    return memberReqResult{}
}
```

Keep `restricted` eligible for the adoption case (`memberReqLiveMarker`, which
already requires the marker) and decide separately whether warn, kick and ban may
still act on a restricted member. Add a test: a restricted member with `until_date` 0
and a Mute rule gets no `restrictChatMember` call, no hold and no notice.

## Warnings

### WR-01: A recorded warn can be punished twice when the hold's punished mark fails

**Files:** `fuku/modules/memberreq_enforce.go:293-299` (`memberReqMarkPunished`,
failure only logged), `:304-319` (`memberReqWarnStep`), `fuku/modules/warns.go:123-127`
(`afterRecord`).

**Issue.** The warn is stored first and the hold is marked punished inside the warn
lock by `afterRecord`. If `MarkHoldPunished` fails (5 s record timeout, DB blip), the
failure is only logged. The warn stays counted and the notice is posted. The hold
stays pending. Two minutes later `ReclaimStaleHold` hands it to the member's next
violating message, which warns and posts a second notice. Kick and ban recover
because the leave update clears the hold. Mute recovers because the stale hold adopts
the live marker (`memberReqLiveMarker`). The warn path has no such recovery, so one
episode produces two warns, and two warns can reach the warn limit.

The error branch (`:310-314`) has a related problem. When `applyWarn` returns an
error with `Recorded` true (the limit action ran but `ResetUserWarns` failed), it
returns `memberReqNotApplied`. No notice is posted for a member who was just
warn-kicked or warn-banned. `res.Recorded` is logged but never acted on.

**Fix.** Retry `MarkHoldPunished` a few times (bounded, same detached context)
before giving up. On a final failure, write a short-lived Redis marker
(`fuku:memberreq:warned:<chat>:<user>`, TTL above `StaleClaim`) and have the reclaim
path in `memberReqEnforce` check it for a stale warn hold. Alternatively, on a stale
hold with `Action == Warn`, mark it punished instead of warning again. Distinguish
that from the "warn never recorded" case: use `res.Recorded` to skip the backoff and
still announce.

### WR-02: A job punishes from a stale settings snapshot, so a rule switched Off still punishes

**Files:** `fuku/modules/memberreq_watch.go:221-229` (snapshot taken at message
time), `fuku/modules/memberreq_enforce.go:244` and `:491`
(`memberReqBroken`/`memberReqStrictest` on `c.Settings`).

**Issue.** D-20 and AGENTS.md say a rule switched off must stop punishing on every
replica at the next message. The watcher reads the cached settings, hands a copy to a
goroutine, and `memberReqEnforce` never re-reads them. A job can wait behind
`memberReqPaced` (100 ms spacing across the fleet, 32 jobs, up to 60 s per slot and a
90 s job budget), then run the live lookup, the cap and the claim. A rule an admin set
to Off, or Delete messages turned off, in that window is still enforced. The
punishment is then recorded with a hold and posted with a notice. A reclaim also
reuses the old snapshot only through a new message, which re-reads settings, so the
exposure is the single in-flight job.

**Fix.** After the live lookup and before the cap and claim, re-read
`memberReqSettingsLookup(ctx, c.Chat.Id)`. On error return `memberReqNone`. Then
recompute `broken` and `action` from the fresh row, and use its `DeleteMessages` for
`memberReqDeletes`.

### WR-03: The rejoin reset deletes a pending mute hold without lifting a mute that may be live

**Files:** `fuku/modules/memberreq_join.go:42-44` (`memberReqPunishedMute` requires
`PunishedAt != nil`), `:200-203` (reset loop), `fuku/db/memberreq/holds.go:213-226`.

**Issue.** `TakeHoldsBefore` deletes every older hold, pending ones included. The
reset then lifts only holds for which `memberReqPunishedMute` is true. A pending mute
hold can still carry a live marker: the restrict was applied but
`MarkHoldPunished` failed or the process crashed between the two. `memberReqLiveMarker`
(`memberreq_enforce.go:573-581`) handles exactly that state at reclaim. If the member
leaves and rejoins before a reclaim, the reset removes the only record of the marker.
The member stays muted for 330 days with no hold and no button, and only an admin's
`/unmute` ends it.

**Fix.** In the reset loop test `hold.Action == models.RequirementActionMute`
regardless of `PunishedAt`. `memberReqLiftOldMute` already checks the live marker
before lifting, so a pending hold with no live marker is a no-op.

## Info

### IN-01: The clean verdict is cached even when the predicate deleted nothing because of the 2 s margin

**File:** `fuku/modules/memberreq_watch.go:127-146`, `fuku/db/memberreq/holds.go:179-194`.

`ClearHoldOnClean` skips a hold created within `cleanMargin` of the message. If a clean message
arrives that quickly, the verdict is still stored for 30 minutes
(`memberReqPassHit` then skips the predicate). A hold that should have been cleared
survives until the TTL ends or the names or settings change. Meanwhile a member who
breaks the rule again is "held" and not punished. This needs a very fast rename, so
it is rare. A related flap: clean at t0, break at t1, revert at t2 inside 30 minutes
hits the old pass and also leaves the hold in place.
Fix: store the pass only when `ClearHoldOnClean` deleted something or no hold exists
(for example a cheap `GetHoldFresh` before `Set`), or include a hold-exists bit in the
fingerprint.

### IN-02: A mute whose notice could not be delivered still reports `memberReqMuted`, so the joiner is never welcomed

**Files:** `fuku/modules/memberreq_enforce.go:433-441`, `fuku/modules/memberreq_notice.go:183-201`,
`fuku/modules/memberreq_join.go:111-113`.

After `memberReqUndeliveredMute` lifts the mute and reverts the hold to pending, the
outcome is still `memberReqMuted`. The join entry returns without a welcome and the
member is unmuted and unwelcomed. A later reclaim also rewrites `welcome_held`
from the reclaiming source. `ReclaimStaleHold` overwrites it
(`holds.go:68-75`), so a join mute whose answer was lost and which a message later
reclaims will never send the held welcome after "I fixed it".
Fix: return a distinct outcome (or `memberReqNotApplied`) from `memberReqPunish` when
the mute was lifted again, so the join entry welcomes. Have `ReclaimStaleHold` keep
`welcome_held` when the fresh row's source is not a join.

### IN-03: The join entry sends the welcome to a member a concurrent job is muting (D-02 race window)

**File:** `fuku/modules/memberreq_join.go:96-99` and `:111-120`.

`memberReqPunishedMute` needs `PunishedAt`. In the window between the watcher's claim
and `MarkHoldPunished`, a join entry sees a pending mute hold: `memberReqHeld` with
that hold, or `memberReqHeld` with `Hold == nil` when its own claim lost
(`memberreq_enforce.go:498-500`). Both fall through to `welcome()`. The member is
welcomed and Greet once claims them while the mute lands, which is the opposite of D-02.
Fix: in the join entry, treat a hold whose `Action == Mute` (pending too) as holding
the welcome, and have the claim-lost branch return the hold it saw.

### IN-04: Notice retry can duplicate a notice on an unknown outcome and pointlessly retries a 4xx

**File:** `fuku/modules/memberreq_notice.go:136-156`.

A `SendMessage` that timed out but was delivered is sent again (`default: retried = true`),
posting two notices. A definite 4xx (bot cannot post) is also retried once.
Fix: retry only unknown outcomes (not `memberReqRefused`). A duplicate mute notice is
harmless because both buttons carry the same hold ID.

### IN-05: An admin whose name breaks a rule, with a cold admin cache, costs one paced `getChatMember` per message

**Files:** `fuku/modules/memberreq_watch.go:218-229`, `fuku/modules/memberreq_enforce.go:483`.

AGENTS.md documents that an unknown cache goes to the job. There is no negative
verdict cache, so every message from such an admin consumes a slot of the fleet-wide
100 ms pacer shared with real punishments and notices (a mute notice waits instead of
dropping, but punishments can be shed as `ErrRateLimited`). Consider a short per-member
"exempt" Redis key after a live admin verdict.

### IN-06: Punishment is skipped for the whole group when one account floods before its claim lands

**File:** `fuku/modules/memberreq_limits.go:53-76`.

The cap counts attempts, not distinct members, and counts before the claim. A single
violator whose first messages each start a job before the first claim lands (several
quick messages while the pacer is backed up) adds several counts. With
`memberReqCapPerMinute` at 10 this needs a slow claim, so it is unlikely. It is
documented as an accepted trade ("lost still counts"). Keeping a per-member in-flight
latch (Redis `SET NX` for a few seconds before the live lookup) would stop one member
spending the budget.

## Checked and found clean

- **Migration and data.** One transaction, idempotent `CREATE TABLE IF NOT EXISTS`,
  named CHECKs and unique keys (`uk_chat_requirement_holds_chat_user`,
  `uk_chat_requirements_chat`), no top-level BEGIN/COMMIT, no semicolons in comments,
  the timestamp is greater than every existing file. Model tags match the migration
  names. Booleans carry no `default:` tag. Writes are column-targeted upserts that
  call `cache.DeleteCache` after the write. `member_requirements:` is in `skipLocal`.
  No non-test code outside the memberreq packages references the two tables, so they
  stay out of backup, export, import and reset.
- **Hold statements.** `ClaimHold`, `ReclaimStaleHold` (`UPDATE ... RETURNING`),
  `TakeHoldByID` (`DELETE ... RETURNING`) and `RestoreHold` (`ON CONFLICT DO NOTHING`
  with no target) are valid on both PostgreSQL and SQLite. The `OR` in
  `ClearHoldOnClean` is parenthesized by GORM when combined. `RowsAffected` use is
  correct.
- **One punishment.** The 90 s job budget ends before the 2 minute stale claim for
  every path except the detached 30 s undelivered-mute lift, which stays inside the
  window. Backoff and revert refresh `created_at`. A lost claim, a lost reclaim and a
  second same-replica job each yield exactly one winner. Mute adopts a live marker.
  Kick and ban hold clears on the detected leave. The only hole is WR-01.
- **Exemptions.** The bot, bots, `staffServiceUserIDs`, `sender_chat` and automatic
  forwards, approvals (tri-state, an error is exempt), pending captcha (both reads, an
  error is exempt), creator and administrator by live record, left, kicked and
  restricted non-members. An unreadable setting, hold or lookup punishes nobody.
- **Watcher.** The named result is preset to `ext.ContinueGroups`, the deferred recover
  keeps it, and every path returns it. It is the only message handler of group -2.
  Counting before the draining read, the 32 cap with `startBelow`, a recovered
  goroutine and a root context cancelled by the drain are correct. No Telegram call for
  a clean member.
- **"I fixed it".** The strict parse and the presser check run before any read. The
  order stale / not-ours / still-broken / permissions / take / lift / restore matches
  the contract. Permissions come only from `resolveUnmutePermissionsWithLockdown`
  (`MutedPermissions` is only used by the punishment and the guard's re-mute). The
  hold is restored with its ID on a failed lift. The query is answered exactly once
  with a recovered panic path. The held welcome is sent after the answer through
  `SendWelcomeMessage`.
- **Captcha guard and join.** The guard is the first statement of the one choke point,
  inert while the switch is off, a read error returns a wrapped sentinel that
  `isPermanentUnmuteError` explicitly maps to false, and the re-mute reuses the hold's
  marker. `ProcessSingleJoin` runs `Reset` once per won claim. `leftMember` clears
  holds. `TakeHoldsBefore` plus `RestoreHold` on a failed lift works as described
  (apart from WR-03).
- **Name rules.** NFC for Russian only, no unbounded work, invisible-only last names
  are blank (default-ignorable, Braille blank, Hangul filler), digits and marks do not
  count as Arabic. The mention is escaped and never passes through the translator.
- **Settings page.** The `Actions` allow-list runs in step 1 before any I/O, `st` only
  on the rule controls, `v` is one digit 0..4, a forged `my` with v 0 or 1 is refused
  twice (Validate and handler), `e` goes through `settingsCardExpired`, "already
  stored" uses a fresh read, rights are `settingsRightRestrict` for every punishment
  choice and both switches. No screen names the Staff Group.
- **Shutdown and switch.** `RegisterMemberReqDrain` is registered after DB-close and
  right before the staff drain, so LIFO runs it after the staff drain and before the
  sweeper stops and the database close. The draining flag, root cancel and 2 s wait
  are correct. `memberReqEnforcementLive` has one reader and one writer (`init()`).
  Nothing punishes while every rule is Off (the watcher, the join entry and
  `memberReqBroken` all stop on Off).
- **Locales and rules.** Every `mreq_*`, `settings_mreq_*`, `settings_btn_mreq_*` and
  `settings_state_mreq_on` key used in code exists in all 8 language files (a loop over
  the keys found none missing apart from `config.yml`, which is not a language). Spliced
  parameters match the en templates. No new command, so `WrapCommand` is not in play.
  The fire-and-forget goroutine recovers, log lines carry IDs and error codes only.

---

_Reviewed: 2026-11-08_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
