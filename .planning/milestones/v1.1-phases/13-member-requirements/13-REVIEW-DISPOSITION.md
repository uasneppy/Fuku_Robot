---
phase: 13
review: 13-REVIEW.md
titles: json
findings:
  - id: CR-01
    severity: Critical
    disposition: fixed
    title: "The requirement Mute replaces an existing admin or staff restriction, and \"I fixed it\" / the rejoin reset then lift something that was not the requirements' own"
  - id: WR-01
    severity: Warning
    disposition: fixed
    title: "A recorded warn can be punished twice when the hold's punished mark fails"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "A job punishes from a stale settings snapshot, so a rule switched Off still punishes"
  - id: WR-03
    severity: Warning
    disposition: fixed
    title: "The rejoin reset deletes a pending mute hold without lifting a mute that may be live"
  - id: IN-01
    severity: Info
    disposition: deferred
    title: "The clean verdict is cached even when the predicate deleted nothing because of the 2 s margin"
  - id: IN-02
    severity: Info
    disposition: deferred
    title: "A mute whose notice could not be delivered still reports memberReqMuted, so the joiner is never welcomed"
  - id: IN-03
    severity: Info
    disposition: deferred
    title: "The join entry sends the welcome to a member a concurrent job is muting (D-02 race window)"
  - id: IN-04
    severity: Info
    disposition: deferred
    title: "Notice retry can duplicate a notice on an unknown outcome and pointlessly retries a 4xx"
  - id: IN-05
    severity: Info
    disposition: deferred
    title: "An admin whose name breaks a rule, with a cold admin cache, costs one paced getChatMember per message"
  - id: IN-06
    severity: Info
    disposition: deferred
    title: "The per-chat cap counts attempts, not distinct members, so one flooding account can spend the group's budget"
  - id: NEW-01
    severity: Info
    disposition: fixed
    title: "Fix pass: the rejoin once-tests dated the join with the current second, so a second boundary between the message's hold and the join made the reset delete the hold and warn twice (test only)"
  - id: NEW-02
    severity: Info
    disposition: deferred
    title: "Fix pass: a restricted member below the group's defaults who keeps breaking a Mute rule costs one paced getChatMember and one getChat per message, because no hold is created (the cost shape of IN-05)"
  - id: NEW-03
    severity: Info
    disposition: deferred
    title: "Fix pass: an admin who restricts a member between the live lookup and the restrictChatMember call can still be overwritten (time of check to time of use)"
  - id: NEW-04
    severity: Info
    disposition: deferred
    title: "Fix pass: ReclaimStaleHold clears notice_msg_id, so the hold of an adopted warn no longer records the first notice's message ID"
  - id: SEC-NEW-01
    severity: Info
    disposition: fixed
    title: "Security audit: a bidi control inside a name is not stripped, so a name can close its own isolate and reorder the notice line"
  - id: SEC-NEW-02
    severity: Info
    disposition: fixed
    title: "Security audit: the undelivered-notice lift does not re-check the marker, so an admin's restriction placed during the notice retry can be overwritten"
  - id: SEC-NEW-03
    severity: Info
    disposition: deferred
    title: "Security audit: a pending mute hold whose punished mark failed three times is not re-muted by the captcha guard until a reclaim"
  - id: SEC-NEW-04
    severity: Info
    disposition: deferred
    title: "Security audit: the warn module's limit action is neither paced nor bounded by the job context (the HTTP client's 30 s timeout and the per-chat cap bound it)"
  - id: SEC-NEW-05
    severity: Info
    disposition: deferred
    title: "Security audit: a raider with more than 10 violating accounts in a minute can pause punishments for 10 minutes (the cost of the brake)"
  - id: SEC-NEW-06
    severity: Info
    disposition: deferred
    title: "Security audit: held and paused deletes still read the watcher's settings snapshot"
open: 0
total: 20
---

# Phase 13: Code Review Disposition

Findings CR-01 to IN-06 come from `13-REVIEW.md` (deep review of 27 files: 1 critical, 3 warnings, 6 info). NEW-01 to
NEW-04 are the concerns the fix pass reported (`13-REVIEW-FIX.md`) and are dispositioned once here. The four fixes went
through one single-writer fix pass, each as a RED test commit followed by a GREEN fix. The security audit
(`13-SECURITY.md`, 0 open threats) registered six more items (SEC-NEW-01 to SEC-NEW-06); the two low ones were fixed in a
second small pass (`13-SECURITY-FIX.md`) and the verifier adds its own entries below if it finds any.

## Fixed

- **CR-01** (`a1820fd` RED, `339d63a` GREEN). `memberReqEnforce` refuses a Mute for a member who is restricted below the
  group's own default rights and carries no marker of this episode (`memberReqMuteOverRestriction`, after the live lookup
  and before the cap and the claim). The refusal places nothing: no hold, no notice, no delete and no Telegram call beyond
  one `getChat`; when the defaults cannot be read it refuses rather than guesses. Warn, Kick and Ban still act on such a
  member (a warn leaves the restriction alone, the other two remove the member). A stale hold whose own marker is live is
  still adopted. The rule is "below the group's defaults" and not the literal "restricted and no marker" because, in a group
  with restrictive defaults, the captcha pass, `/unmute` and every requirement lift all send the group's defaults and leave
  the member reading as `restricted` with no marker; the literal rule would stop muting them for good and broke three
  existing tests (`TestMemberReqMuteNoticeUndeliverable`, `TestMemberReqJoinAfterCaptcha`,
  `TestMemberReqRejoinResetsMuteHold`), none of which was edited. The orchestrator reviewed the deviation and accepts it:
  the comparison in `memberReqRestrictedBelow` is the check that tells a restriction someone placed from the group's own
  defaults. It rests on Telegram reading a restored member back with the group's rights, which live SPIKE check 14
  verifies.
- **WR-01** (`7bdc7c9`, `9663ac8`). Marking a hold punished is tried three times on one detached record context. A Redis
  note `fuku:memberreq:warned:<chat>:<user>` (value the hold ID, TTL `memberreq.StaleClaim` plus 10 minutes, no migration)
  is written inside `afterRecord` before the mark and deleted once the mark lands; the reclaim of a stale Warn hold finds
  it, marks the hold punished and returns `memberReqHeld` with no second warn or notice. An `applyWarn` error with
  `Recorded` true now marks the hold and posts the notice, and a warn-kick or warn-ban still reports as a removal.
  Accepted: without Redis the reclaim warns again.
- **WR-02** (`6b4636d`, `59cbf9a`). `memberReqLiveViolation` re-reads `memberReqSettingsLookup` after the live lookup; an
  unreadable setting punishes nobody, and the broken rules, the strictest action and the Delete messages switch come from
  the fresh row. The join entry goes through the same function.
- **WR-03** (`c09480d`, `cdf0b7c`). `memberReqJoinReset` lifts for every older MUTE hold, pending or punished, and
  `memberReqLiftOldMute` still lifts only while the live record carries the hold's marker (a failed lift restores the
  hold with its ID).
- **NEW-01** (`96b2f73`, test only). The rejoin once-tests now date their join before the message is sent, as Telegram
  dates a join the bot processes late; `-race -count=40` passes. Production code is unchanged: there a join dated after a
  member's first message really is the earlier-membership case the reset exists for.

## Deferred (accepted limits, none blocks the phase)

- **IN-01.** The clean verdict is cached for 30 minutes even when `ClearHoldOnClean` skipped a hold inside the 2 s margin,
  so a hold can outlive a very fast rename. Needs a rename within seconds of the punishment; a rule broken again meanwhile
  is the held path and is not punished again until the verdict expires or the names or settings change. Revisit with a
  hold-exists bit in the fingerprint if the live checks show it.
- **IN-02.** A mute whose notice could not be delivered is lifted again but still reports `memberReqMuted`, so a joiner is
  not welcomed, and a later reclaim rewrites `welcome_held`. Needs the notice to fail twice in a row (the
  `memberReqUndeliveredMute` path); the member is unmuted and an admin can greet them.
- **IN-03.** In the window between a watcher's claim and the punished mark, a join entry sees a pending mute hold and
  welcomes the member (Greet once claims them) while the mute lands. A narrow race of the join with a first message; the
  welcome is a cosmetic loss, never a missed punishment. Candidate for a later hardening phase (treat a pending mute hold
  as holding the welcome, and return the hold on a lost claim).
- **IN-04.** The notice retry can post a second notice when a send timed out after it was delivered, and retries a definite
  4xx once. A duplicate mute notice is harmless (both buttons carry the same hold ID).
- **IN-05 and NEW-02.** Each message of an admin with a cold admin cache whose name breaks a rule, or of a restricted
  violator below the defaults, costs one paced `getChatMember` (plus a `getChat` for the latter), because no hold or
  exempt verdict is stored. It shares the fleet-wide 100 ms pacer with punishments and notices. Revisit with a short
  per-member exempt key if the live checks show pressure.
- **IN-06.** The per-chat cap counts attempts before the claim, so one account whose first messages each start a job before
  the first claim lands can spend several counts. Documented in AGENTS.md as the price of counting before the claim.
- **NEW-03.** A narrow time-of-check to time-of-use gap: an admin who restricts a member between the live lookup and the
  `restrictChatMember` call is overwritten. The window is the milliseconds between two calls of one job.
- **NEW-04.** `ReclaimStaleHold` clears `notice_msg_id`, so an adopted warn's hold no longer records the first notice's
  message ID. The "I fixed it" button edits the message it sits on and never reads that column, so nothing depends on it.

## Security audit items (`13-SECURITY.md`)

- **SEC-NEW-01** (`ac75ae7` RED, `44a20b2` GREEN, fixed). `memberReqStripBidi` removes U+061C, U+200E, U+200F, U+202A to
  U+202E and U+2066 to U+2069 from a name before it is cut to 64 runes and wrapped in the U+2068/U+2069 isolate, so a name
  can no longer close its own isolate; an all-control name uses the fallback. The name rules still judge the original name.
- **SEC-NEW-02** (same commits, fixed). `memberReqUndeliveredMute` makes one paced `getChatMember` and lifts only while the
  live record carries the hold's marker; a restriction someone else placed during the notice retry is left alone (the hold
  is still reverted to pending). When the read itself fails it still lifts (a double fault; a button-less 330-day mute is
  the worse outcome), documented in the function comment and AGENTS.md.
- **SEC-NEW-03** (AR-15). A pending mute hold whose mark failed three times is not re-muted by the captcha guard; the reclaim
  after `StaleClaim` mutes again. A triple database failure plus a captcha pass inside two minutes.
- **SEC-NEW-04** (AR-16). The warn module's limit action (kick or ban at the limit) is not paced through `memberReqPaced` and
  not bounded by the job context; it is bounded by the HTTP client's 30 s timeout and the per-chat cap. The AGENTS.md
  wording ("every Telegram call is paced") is imprecise for this one call and is left for a docs pass.
- **SEC-NEW-05** (AR-10). A raider with more than 10 violating accounts a minute can trip the cap and pause punishments for
  10 minutes. This is the brake working as designed: the held members' messages are still deleted.
- **SEC-NEW-06** (AR-17). Held and paused deletes read the watcher's settings snapshot (WR-02 fixed the punishing path).
