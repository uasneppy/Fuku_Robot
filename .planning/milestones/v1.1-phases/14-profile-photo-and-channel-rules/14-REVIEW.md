---
phase: 14-profile-photo-and-channel-rules
reviewed: 2026-11-08T00:00:00Z
depth: deep
files_reviewed: 11
files_reviewed_list:
  - migrations/20261108120000_add_member_requirement_lookup_rules.sql
  - fuku/db/models/member_requirements.go
  - fuku/db/memberreq/settings.go
  - fuku/modules/memberreq_lookup.go
  - fuku/modules/memberreq_rules.go
  - fuku/modules/memberreq_enforce.go
  - fuku/modules/memberreq_watch.go
  - fuku/modules/memberreq_held.go
  - fuku/modules/memberreq_notice.go
  - fuku/modules/memberreq_fixed.go
  - fuku/modules/settings_mreq.go
findings:
  critical: 0
  warning: 5
  info: 5
  total: 10
status: issues_found
---

# Phase 14: Code Review Report

**Reviewed:** 2026-11-08
**Depth:** deep
**Files Reviewed:** 11
**Status:** issues_found

## Summary

All 11 files were read in full (the shared files also as the phase diff against `cc19bba`).
`CGO_ENABLED=0 go build ./...` is clean. No test run was made. Cross-file chains were
traced: lookup layer to the executor, the watcher, the held release, "I fixed it", the
notice and the settings page. The AGENTS.md rules were checked against the code: locale
keys, secrets, panic recovery and the migration appliers.

The central safety property holds. I found no path from an error, shed, timeout,
cancelled context, cache read error, null body or unrecognised status to a punishment.
`memberReqVerdict`'s zero value is unknown. `memberReqLookupBroken` sets a bit only on
`memberReqBreach`. The only producers of `memberReqBreach` are `memberReqPhotoRule`
(a definite non-nil answer with total 0 and no photos) and `memberReqChannelRule` (a
definite left, kicked or restricted-without-`is_member` answer that then passes the D-22
gate with the bot proven an administrator). The claim sits after every lookup. The
singleflight recovers its own leader panic. The watcher keeps its preset
`ext.ContinueGroups` and makes no Telegram call. The "I fixed it" handler can never lift a
mute without the live marker and never uses `MutedPermissions`.

There are no Critical findings. The real problems are about cost and about stale state:

1. **WR-01.** The default failure mode of the new feature turns every message of every
   non-exempt member into a runner job plus 2 to 3 paced Telegram calls. That shared
   runner and the fleet-wide pacer are also Phase 13's punishment path.
2. **WR-02.** An unknown lookup can leave a weaker punishment standing, and the hold then
   keeps the stricter rule from ever being judged.
3. **WR-03 and WR-04.** The live member record that gates a punishment or a lift is now
   up to 20 s old (job), 4 s old (join) or 5 s old ("I fixed it") when it is acted on.
4. **WR-05.** A punishment can rest on a linked-channel fact up to 10 minutes old.

## Warnings

### WR-01: A group linked to a channel the bot cannot read costs a runner job and up to three paced calls per message of every non-exempt member, and it shares both budgets with Phase 13's punishment path

**Files:** `fuku/modules/memberreq_watch.go:253-270` (`memberReqWatchClean`),
`:176-217` (`memberReqLookupCached`), `:297-314` (`memberReqRunAsync`, shed log at `:299`);
`fuku/modules/memberreq_enforce.go:252-281`, `:557-609`;
`fuku/modules/memberreq_lookup.go:478-529` (`memberReqChannelRule`);
`fuku/modules/settings_mreq.go:599-629` (`settingsMreqChannelGate`).

**Issue.** The cost is not exotic. It is what happens when an owner forgets to make the bot
an administrator of the channel.

- `settingsMreqChannelGate` lets a punishing channel value through for any linked channel,
  "whatever the bot's standing there" (`:614-616`). The page shows the not-administrator
  line only if someone opens the screen.
- For such a channel the member lookup answers 400 or 403, which is an unknown.
  `memberReqChannelRule` caches only a pass, so nothing is ever cached for that group.
- The watcher's `memberReqLookupCached` therefore returns `satisfied == false` for every
  message of every member. `memberReqMissExempt` spares only a cached-admin or approved
  member. Everyone else gets `memberReqStartJob`.

Per message the job does the following, in `memberReqEnforce`:

1. The approvals read.
2. The captcha-pending read.
3. `GetHoldFresh`, a fresh database query.
4. A Redis pause read.
5. One live `getChatMember` on the shared `memberReqPacer`.
6. The settings read.
7. The channel member lookup on the lookup pacer.

That is 2 Telegram calls and about 1 fresh query per message. With the photo rule on and
the photo cached, the photo lookup costs nothing.

Concrete failure, a busy supergroup at about 20 messages a second:

- `memberReqPacer` is one fleet-wide Redis slot at 100 ms, so about 10 live lookups a
  second serve all groups on all replicas. 20 jobs a second cannot be served.
- The runner is capped at 32 jobs per replica (`memberReqMaxInFlight`, one counter shared
  by all chats). Excess messages are shed, each with its own `log.Warnf` at `:299`. That is
  about 10 Warn lines a second for as long as the group is busy.
- The jobs queued on `memberReqPacer` hold its slots, so a real name violator's job
  (same runner, same pacer) is shed about half the time, in every group on that replica.
  The punishment and its mute notice (`memberReqPostNotice`, same pacer, "never dropped by
  pacing") queue behind up to 32 reserved slots, roughly 3 s.
- The lockdown is not affected, because it has its own pacer keys.

Two multipliers are not exotic either:

- **Redis down.** Nothing is cached or reserved. Every message of every non-exempt member
  in any group with a lookup rule on is a job with unpaced local fallback, and nothing is
  cached by the lookups. AGENTS.md accepts this as slower. It is bounded by 32 jobs, but
  it is also the situation in which Phase 13 violators lose their punishment.
- **Bursts from one member.** Nothing de-duplicates jobs per `(chat, user)`. The lookups
  are flight-coalesced but the live `getChatMember`, the hold read and the exemption reads
  are not. A new member who posts 10 messages in 2 s, before the first lookup caches,
  costs 10 live lookups. In Phase 13 only a violator reached the job, and the claim made
  later jobs take the held path with no Telegram call. A member with no hold gets no such
  shortcut.
- **Cold admin cache.** Phase 13 IN-05 is widened: an administrator with no profile photo
  and a cold admin cache now costs a job per message, not just a violator's.

**Fix.** The mitigation the brief asks about is safe and narrow. It is a channel-level
negative cache, keyed by channel only, never by user, so it is never a verdict about a
member:

```go
// fuku:memberreq:cna:<channel>  "1"  EX 60  -- "the bot is not an administrator of C"
// written in memberReqChannelRule on an unknown member lookup (one flight-deduped
// memberReqBotChannelStatus per channel per 60 s, which the Classify=false path does not
// make today) when the bot is memberReqBotNotAdmin; also written by the page's status read.
// read (1) in memberReqLookupCached, as a fourth key of the same pipeline: set means the
//         channel part counts as satisfied, so the watcher starts no job for it;
//     (2) at the top of memberReqChannelRule after the link is known: set means
//         memberReqInert with no member lookup and no gate call.
```

It can only turn an unknown into an inert, never into a breach, and it expires in 60 s.
It does not conflict with D-24, which rejected a user-verdict negative cache. Cheap
companions:

- A `SET NX EX 5` job gate per `(chat, user)` in `memberReqWatchClean`, on the lookup-miss
  hand-off only. Violators are not gated.
- A reserved lane in `startBelow` for the name-violator path, for example 8 of the 32, so
  lookup-miss jobs can never use more than 24.
- Demote the shed line at `:299` to Debug, or rate-limit it once a minute per replica.

### WR-02: An unknown lookup lets a weaker punishment stand, and the hold then keeps the stricter rule from ever being judged

**Files:** `fuku/modules/memberreq_enforce.go:276-280` (the unknown flag is discarded:
`lookupMask, _ :=`), `:585`, `:594-601`; `fuku/modules/memberreq_held.go:44-68`.

**Issue.** D-02 says an unknown must never block an established breach. The implementation
goes one step further and also ignores which rules were unknown when it picks the
punishment and claims the hold.

Concrete failure:

- A group sets the last-name rule to Warn and the channel rule to Ban.
- A member breaks both. The channel lookup is shed by the lookup pacer, which happens
  exactly during a raid, or it times out.
- `broken` is the last-name bit only. `memberReqStrictest` is Warn. The hold is claimed
  with `Rules = 1`, the member is warned, and the notice names one rule.
- The next messages take the held path (`memberReqEnforce:566-572`).
  `memberReqHeldReleased` returns false while the name is still broken, and the held path
  makes no lookup. The channel rule is never judged while the hold lives.
- A member who keeps the broken name keeps the hold, so the Ban never lands. A hold ends
  only by a clean predicate, a leave or a rejoin, or "I fixed it".

With Delete messages off, a Warn hold has no effect at all after the first notice.

**Fix.** Choose one of these:

- When `unknown` is true and a skipped lookup rule has a stricter configured action than
  the one about to be claimed, do not claim this time. Return `memberReqNone` and let the
  next message retry. Bound the deferral so a permanently unknown rule cannot block a name
  breach for ever, for example the per-member `SET NX EX 5` gate of WR-01 with a small
  counter.
- Alternatively, record that the episode was partial. The held path can re-run the
  lookups for the lookup rules that are on, are not in `hold.Rules`, and are stricter than
  `hold.Action`.

### WR-03: The live record that gates a punishment is read before the lookup phase and acted on up to 20 s (job) or 4 s (join) later

**Files:** `fuku/modules/memberreq_enforce.go:252-281`, `:581-594`, `:726-741`
(`memberReqMuteOverRestriction`).

**Issue.** Phase 13 read the live `getChatMember` and claimed within milliseconds. Phase 14
inserts the lookup phase between them:

- up to `memberReqLookupJobBudget` (20 s) for a job;
- up to `memberReqLookupJoinBudget` (4 s) for the join entry.

Everything decided from that record is now stale by that much:

- the admin and eligibility check;
- the CR-01 decision in `memberReqMuteOverRestriction`;
- `memberReqLiveMarker`.

The exemptions (approved, pending captcha) were read earlier still.

Concrete failures:

1. A member is promoted to administrator, or an admin approves them, within the window. The
   job still kicks, bans or mutes them.
2. An admin restricts the member (`/mute`, a partial restriction) within the window. Phase
   13's CR-01 guarantee, "never replace a restriction the requirements did not place", no
   longer holds there. The marker mute replaces it, and "I fixed it" later frees them.

Both need a specific concurrent action. The window is now the length of a lookup under a
loaded pacer, not a database round trip.

**Fix.** After the lookup phase, and only when it made a Telegram call or took longer than
about 1 s, re-read the member once through `memberReqLiveMember` and re-run
`memberReqLiveEligible`. Feed that fresh record to `memberReqMuteOverRestriction` and
`memberReqLiveMarker`. It costs one paced call, and only on the breach path.

### WR-04: "I fixed it" reads the live marker, spends up to 5 s on fresh lookups, then lifts unconditionally

**Files:** `fuku/modules/memberreq_fixed.go:140-147` (marker check), `:160-163` and `:210-229`
(`memberReqFixedJudge`, up to `memberReqFixedLookupBudget` of 5 s), `:165-184`
(permissions, take, `memberReqLiftMute`).

**Issue.** `isMemberReqMute` is proven at step 4. Step 5 may then wait up to 5 s on four
paced lookup calls. `memberReqLiftMute` is an unconditional `restrictChatMember` with
default permissions.

Concrete failure: an admin runs `/mute` on the member inside that gap. The admin's mute
replaces the marker. The lift then removes the admin's mute, and the presser has freed
themselves of a mute the requirements did not place. This is MREQ-07's "never", made
probabilistic. Phase 13's gap between marker check and lift was a few milliseconds.

The press stays within its 10 s budget. The worst case is about 8 paced requests, 4 of
them lookups, and the lookups hold a 5 s cap. A slow step after `memberReqFixedTake` is
safe: the hold is restored on a detached context.

**Fix.** Re-check the marker right before the take. Do one more paced `memberReqLiveMember`
and `isMemberReqMute`, after `memberReqUnmutePermissions`. Or run the fresh lookups first,
which need no live record, and read the live record and marker after them.

### WR-05: A punishment can rest on a linked-channel fact up to 10 minutes old

**Files:** `fuku/modules/memberreq_lookup.go:60` (`memberReqLinkedTTL`), `:326-360`
(`memberReqLinkedChannel`), `:478-529`.

**Issue.** The channel the rule judges comes from `fuku:memberreq:lc:<chat>`, cached for 10
minutes. A positive link is the dangerous direction: a stale "none" only errs toward inert,
but a stale channel ID leads to punishment.

Concrete failure: an owner unlinks the group from channel A, or relinks it to B.

- For up to 10 minutes, every member who is not in A is judged against A.
- The bot is still an administrator of A, so the D-22 gate passes. The `ba:` cache lives
  for 2 minutes.
- Those members are kicked, banned or muted for not subscribing to a channel that is no
  longer linked.
- A Ban chosen for the rule is permanent.

The settings page's fresh read refreshes `lc`, but only when an admin happens to open the
screen.

**Fix.** Choose one of these:

- Cache a positive link for 60 to 120 s and keep 10 minutes for "none". The cost is one
  `getChat` per chat per minute at most.
- Re-read the link fresh once on the breach path, before claiming a hold whose mask
  includes the channel bit, and drop the bit if the channel differs. The `f:l:<chat>`
  flight coalesces it.

## Info

### IN-01: An approved member with a lingering hold is no longer cleaned in a group whose lookup rule is on and the member has no cached positive

**File:** `fuku/modules/memberreq_watch.go:253-270`, `:278-288`.

**Issue.** In Phase 13 the watcher's clean path ran `ClearHoldOnClean` for any
clean-named member. Now, with a lookup rule on, an approved member who lacks a photo or
subscription hits `memberReqMissExempt`, which returns before any predicate. The member
never reaches the job either, because `memberReqExempt` ends it first. The hold lingers
until a leave or a rejoin. If approval is removed later, the member is "held" by a stale
episode and their messages are deleted with Delete messages on. It is narrow and
low-impact.

**Fix.** In the approved branch, call `memberReqClean` (or `ClearHoldOnClean` directly)
without writing the pass key, or have `memberReqExempt` clear the hold.

### IN-02: The held release (D-21) runs before the pause check, so a paused chat still makes lookups for held members

**File:** `fuku/modules/memberreq_enforce.go:566-580`; `fuku/modules/memberreq_held.go:74-88`.

**Issue.** AGENTS.md says that while the pause latch stands "the live `getChatMember` is
not made". The held release makes up to two paced lookups per message of a held member
during a pause. The release only deletes a row, so there is no punishment. It is a cost
and documentation nuance, not a safety problem.

**Fix.** Either note it in AGENTS.md, or read `memberReqPaused` first and skip the lookups
during a pause.

### IN-03: The linked-channel and bot-status flights do not re-read their cache inside the flight, unlike the photo and subscription flights

**File:** `fuku/modules/memberreq_lookup.go:335-355`, `:432-457`.

**Issue.** `memberReqPhotoRule` and `memberReqChannelRule` re-check the cache inside the
flight so that messages arriving together make one request. `memberReqLinkedChannel` and
`memberReqBotChannelStatus` do not, so a caller that missed the cache just before the
previous flight wrote it starts a second `getChat` or `getChatMember`. The cost is one
extra paced call per such race.

**Fix.** Re-read the `lc:` and `ba:` keys at the top of their flight functions.

### IN-04: The claim has no remaining-budget guard, and the lookup phase makes a claimed-but-unpunishable hold likelier

**File:** `fuku/modules/memberreq_enforce.go:594`; `fuku/modules/memberreq_watch.go:309`.

**Issue.** The job context is 90 s. A saturated `memberReqPacer` can make the live lookup
wait up to 60 s, and the lookups add up to 20 s. A job can then claim the hold with a few
seconds left, fail the punishment on the deadline, and leave a pending hold for the
2-minute reclaim. This is safe, because the reclaim retries once. It is more likely than
in Phase 13.

**Fix.** Skip the claim, or shorten the lookup budget, when `ctx.Deadline()` leaves less
than about 15 s.

### IN-05: Tests swap package globals that an abandoned flight goroutine may still read under `-race`

**Files:** `fuku/modules/memberreq_lookup_fake_test.go:449-453` (`memberReqLookupPacer`),
`fuku/modules/memberreq_rules_test.go:391-405` (`memberReqRules`), and the many
`mreqSwap(t, &memberReqLookupFlightTimeout|memberReqRedisClient, ...)` sites.

**Issue.** By design a flight outlives a caller whose deadline ended, for up to 15 s.
Cut-off tests (`memberreq_lookup_enforce_test.go:420-422`,
`memberreq_fixed_lookup_test.go:486-487`, `memberreq_held_test.go:395`) abandon flights.
After the call returns, a flight reads `memberReqRedisClient` (the cache write) and
`memberReqPositiveTTL`. If `t.Cleanup` restores the global at that moment, `-race` may
flag it. I could not confirm this without running the suite. The pacer tests assert only
lower bounds on timing (25 ms, 150 ms, 360 ms), so they are not tolerance-flaky.

**Fix.** Have each cut-off test wait until its flight has ended before cleanup, for
example by polling the fake's call count and `memberReqLookupFlight` idleness.

## Checked and found clean

- **No punishment on an unknown.**
  - `memberReqLookupBroken` (`memberreq_rules.go:180-198`) sets a bit only on
    `memberReqBreach`. Unknown sets the flag, and pass and inert add nothing.
  - `memberReqFlightValue` returns `(zero, false)` for a caller whose context ended and for
    a recovered panic. The zero verdict is unknown.
  - `memberReqPhotoRule` treats `photos == nil` (a `null` body) as unknown. Breach
    requires `TotalCount == 0` and no photos, and anything above zero is a pass.
  - `memberReqSubscribed` treats the empty status as unknown. An unanswered, nil or
    unrecognised answer is unknown and is never inspected for a breach.
  - `memberReqClassifyBot` guards `ErrRateLimited` explicitly before `IsBotGoneError`.
    Only a 400 or 403 or a definite non-admin status is not-administrator. A 429, shed,
    5xx, transport error, timeout or cancelled context is unknown. It never produces a
    breach.
  - `memberReqGateBreach` returns breach only for a definite administrator, inert for a
    definite non-administrator, and unknown otherwise.
  - A cache read error, missing Redis, `Exists` error or cancelled context is a miss, never
    a verdict.
- **The claim boundary.**
  - Lookups run in `memberReqLiveViolation` after the live record, the settings re-read
    and the names, and before the mute-over-restriction check, the cap and the claim.
  - A lookup cut off by shutdown or by its deadline is an unknown and leaves no hold, no
    notice and no write.
  - A paused chat returns before the live lookup, so it makes no lookup. The held release
    is the one exception, IN-02.
  - An unknown never hides an established name breach, because the names are folded
    first. This is the intent, with the cost noted in WR-02.
- **The singleflight.**
  - The leader recovers its own panic before the library can re-raise it. The flight
    context derives from `memberReqContext()` and is bounded by 15 s. A caller's deadline
    only ends that caller's wait.
  - Fresh and non-Fresh callers never share a flight (`f:` prefix). The `p:`, `l:`, `s:`
    and `b:` key families cannot collide.
  - The flight map entry is removed when the flight ends, and the flight is bounded by
    `memberReqCallTimeout` and `memberReqLookupFlightTimeout`. The pacer sheds a slot more
    than 5 s away, so pending flights are bounded under a raid.
  - A Fresh breach drops a stale pass for the photo and the subscription, and a Fresh
    not-administrator drops a stale `ba:` positive.
- **The watcher.**
  - `memberReqWatcher` still returns `ext.ContinueGroups` on every path through its
    preset named result and its deferred recover.
  - It makes no Telegram call. `memberReqLookupCached` is one pipeline of GETs plus one
    `EXISTS`, and any Redis error means not satisfied.
  - `memberReqRunAsync` counts the job before reading `memberReqDraining`, sheds at 32,
    and recovers panics.
  - The pass key cannot stand in for a lookup positive: `satisfied` and `passHit` are
    separate.
  - An approvals read error is exempt, so it starts no job and punishes nobody.
- **The held release (D-21).**
  - It never punishes, claims, warns, posts or restricts. It runs only the hold's own
    lookup bits, treats a rule now Off as released, and keeps the hold on any unknown,
    breach or cut-off lookup.
  - A mute hold is deleted only through `ClearHoldOnClean`'s existing 2 s margin and
    punished-at rule, so a requirement mute is freed only by "I fixed it", an admin, or
    the rejoin reset.
  - A name-only hold is released only when the names are clean against the current
    settings.
- **"I fixed it".**
  - The ten steps run in order. The presser-only rule is checked before any read. The
    live marker is checked before approvals (step 4b, tri-state: approved is clean, an
    error is `mreq_fixed_check_failed`) and before the judge (step 5).
  - The judge reports a broken rule first, and an unknown alone changes nothing.
  - It uses `memberReqUnmutePermissions` and never `MutedPermissions`. It answers exactly
    once, including after a recovered panic.
  - The failure handling after `memberReqFixedTake` restores the hold on a detached
    context.
  - The M13 inert classification is the documented intent. It frees a member only when the
    bot is a definite non-administrator, or the channel is unlinked on a fresh read. An
    admin loses nothing but enforcement, and `/unmute` and approval are unaffected.
  - The worst case is about 8 paced requests. Only the lookup phase is sub-budgeted. A
    deadline after the take is safe.
- **The notice.**
  - The photo remedy is appended only for the plain warn, kick and mute keys, and only
    when the photo bit (found through the table row) is among the broken rules. It is added
    after translation and before the name splice.
  - `memberReqMention` still strips the bidirectional controls and caps the name at 64
    runes. No title, channel ID or user text is spliced without escaping.
  - `memberReqStillBrokenText` names up to 4 rules and counts above that.
- **The settings page.**
  - `settingsMreqChannelGate` is called for a channel value other than Off. It refuses for
    a basic group or no linked channel, answers `mreq_fixed_check_failed` for an unreadable
    link, and writes nothing in those cases.
  - It runs in the Yes path after the pipeline and the expiry check. The link comes only
    from a live read, never from callback data.
  - `settingsMreqReadStatus` makes 0 calls for a basic group, an Off channel rule, the
    photo and name screens, and Main, and at most 2 bounded paced calls otherwise. The
    press is answered empty first.
  - Callback data for `ph` and `ch` stays under 64 bytes (46 and 57 by hand count for the
    private worst case).
  - No screen names the linked channel or the Staff Group. A forged `t` is refused by
    `settingsMreqOpenFields` and `settingsMreqYesFields` before any I/O.
- **Data.**
  - The migration uses `ADD COLUMN IF NOT EXISTS ... NOT NULL DEFAULT 0` with each named
    CHECK inline. It is idempotent, an old row reads both rules Off, and the file name is
    the newest.
  - Neither applier's `ADD CONSTRAINT` rewrite matches an `ADD COLUMN` statement.
  - The model fields carry no `default:` tag. `memberreq.Rules`, `ruleColumns` and
    `actionField` cover both rules.
  - Every settings write goes through `upsertSettings`, which calls `cache.DeleteCache`.
  - The pass fingerprint covers seven actions. A one-time miss at deploy costs one
    `ClearHoldOnClean` per active member.
  - A cached entry from an older build decodes both new fields as 0, which is Off, the
    safe direction.
  - The `rules > 0` CHECK on holds is unaffected by bits 32 and 64.
- **AGENTS.md rules.**
  - The one new goroutine surface, the flight leader, recovers panics. The job goroutine
    still does.
  - Logs carry IDs, outcomes and Telegram error codes only. `memberReqErrText` prints the
    code alone, and the bot token is registered with `logredact`.
  - Locale keys: every `settings_mreq_*`, `settings_btn_mreq_*`, `mreq_*`,
    `settings_state_mreq_on` and `settings_page_mreq` key used in non-test Go code exists in
    all 8 locale files. The `mreq_notice_warn_limit_` prefix resolves to the kick, ban and
    mute keys, 3 per locale.
  - No `t.Skip` was added to a test file of `fuku/modules`, `fuku/db/memberreq` or
    `fuku/i18n`. The two skips in the new migration test sit in `fuku/db/migrations`,
    which `scripts/check_test_results` already exempts. The test is in the PostgreSQL
    integrity list.
- **Documented and accepted, not defects.**
  - The photo rule counts a photo hidden by privacy as missing (D-01, spike S1).
  - A Ban on that rule is guarded only by the two-tap card and the cap of 10 punishments a
    minute.
  - Only positive lookups are cached (D-24).
  - The join entry's 4 s lookup budget welcomes a joiner whose lookup did not finish.

---

_Reviewed: 2026-11-08_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
