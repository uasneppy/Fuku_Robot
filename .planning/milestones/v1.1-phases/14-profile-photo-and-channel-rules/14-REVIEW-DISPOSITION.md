---
phase: 14
review: 14-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: fixed
    title: "A group linked to a channel the bot cannot read costs a runner job and up to three paced calls per message of every non-exempt member"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "An unknown lookup lets a weaker punishment stand, and the hold then keeps the stricter rule from ever being judged"
  - id: WR-03
    severity: Warning
    disposition: fixed
    title: "The live record that gates a punishment is read before the lookup phase and acted on up to 20 s (job) or 4 s (join) later"
  - id: WR-04
    severity: Warning
    disposition: fixed
    title: "\"I fixed it\" reads the live marker, spends up to 5 s on fresh lookups, then lifts unconditionally"
  - id: WR-05
    severity: Warning
    disposition: fixed
    title: "A punishment can rest on a linked-channel fact up to 10 minutes old"
  - id: IN-01
    severity: Info
    disposition: deferred
    title: "An approved member with a lingering hold is no longer cleaned in a group whose lookup rule is on and the member has no cached positive"
  - id: IN-02
    severity: Info
    disposition: deferred
    title: "The held release (D-21) runs before the pause check, so a paused chat still makes lookups for held members"
  - id: IN-03
    severity: Info
    disposition: fixed
    title: "The linked-channel and bot-status flights do not re-read their cache inside the flight, unlike the photo and subscription flights"
  - id: IN-04
    severity: Info
    disposition: deferred
    title: "The claim has no remaining-budget guard, and the lookup phase makes a claimed-but-unpunishable hold likelier"
  - id: IN-05
    severity: Info
    disposition: deferred
    title: "Tests swap package globals that an abandoned flight goroutine may still read under -race"
  - id: NEW-01
    severity: Info
    disposition: deferred
    title: "Fix pass: a stricter lookup rule that stays unanswerable (a persistent 5xx or timeout on the channel) holds the weaker name punishment back for as long as it stays unanswerable"
  - id: NEW-02
    severity: Info
    disposition: deferred
    title: "Fix pass: each not-subscribed breach candidate costs one fresh getChat of the group, repeated per attempt for a member whose punishment is refused or backed off"
  - id: NEW-03
    severity: Info
    disposition: deferred
    title: "Fix pass: the lookup touch marker is set by any flight start or join, including a flight whose in-flight cache re-read hits, which only costs an extra safe live read"
  - id: SEC-01
    severity: Info
    disposition: deferred
    title: "Security audit: the linked channel's numeric ID is written to three Debug log lines (D-18 allows IDs; the log test forbids titles, names and texts, not the ID)"
  - id: SEC-02
    severity: Info
    disposition: fixed
    title: "Security audit: the approved and captcha-pending exemptions are read once before the lookup phase and not again before the claim"
  - id: SEC-03
    severity: Info
    disposition: fixed
    title: "Security audit: a demoted bot is still read as a channel administrator for up to 2 minutes through the ba positive cache"
  - id: SEC-04
    severity: Info
    disposition: deferred
    title: "Security audit: nothing limits a person's presses of the channel screen or I fixed it, and one 32-job runner serves every chat of a replica"
open: 0
total: 17
---

# Phase 14: Code Review Disposition

Findings WR-01 to IN-05 come from `14-REVIEW.md` (deep review of 11 production files: 0 critical, 5 warnings, 5 info). NEW-01
to NEW-03 are the concerns the fix pass reported (`14-REVIEW-FIX.md`) and are dispositioned once here. The six fixes went
through one single-writer fix pass, each as a RED test commit followed by a GREEN fix, and the pass was proved through CI's
own gate (`go test -json` into `scripts/check_test_results`, exit 0, no unexpected skip). The security audit
(`14-SECURITY.md`, 0 open threats) registered four more items (SEC-01 to SEC-04); the two that concern a destructive
punishment were fixed in a second small pass (`14-SECURITY-FIX.md`), and the verifier adds its own entries below if it
finds any.

## Fixed

- **WR-01** (`b3413c2` RED, `48d87cf` GREEN). The bot's definite not-administrator standing in a channel is cached as
  `fuku:memberreq:cna:<channel>` (60 s, keyed by the channel only, a fixed marker, never a user verdict and never a
  breach). A definite non-administrator status or a 400/403 writes it, a definite administrator answer deletes it, and a
  429, shed, 5xx, timeout or null answer writes nothing. It is read at the top of `memberReqChannelRule` (inert at once, no
  Telegram call, bypassed by Fresh) and in the watcher's pipelined read after the subscription key misses, so a group whose
  channel the bot cannot read costs one lookup per 60 s instead of one job per message. This amends D-24 for this one
  channel-level fact only (D-24's per-user rule stands). The per-shed Warn log line is Debug. The per-(chat, user) job gate
  and a reserved runner lane stay deferred.
- **WR-02** (`ba98914`, `812f945`). `memberReqUnknownDefers`: after the lookup phase, an unanswered rule whose punishment is
  STRICTER (by severity) than the strictest broken rule's makes the executor claim nothing and return none (one Info line,
  IDs only), so a stricter rule is never dropped behind a weaker punishment and its hold. An unknown rule that is equal or
  weaker does not defer, and an unknown with no established breach is simply not broken as before.
- **WR-03** (`0bece09`, `79eeb53`). When something is broken and the lookup phase made or waited for a request, or took
  over 1 s, the live member record is read once more and eligibility, names, mute-over-restriction and marker adoption use
  that record (a failed re-read punishes nobody); an all-cache phase costs no extra call.
- **WR-04** (`497d779`, `71a472a`). When step 5 ran any lookup, "I fixed it" re-proves the live marker with one paced live
  read right before the conditional take; a gone marker answers `mreq_fixed_not_ours`, a failed read
  `mreq_fixed_check_failed`, and nothing changes either way.
- **WR-05** (`9cf8e19`, `5b87552`). A positive linked-channel cache lives 60 s (none keeps 10 minutes: that direction only
  makes the rule inert), and a not-subscribed breach candidate re-reads the link fresh through the lookup pacer before the
  gate; a changed, removed or unreadable link is unknown, never a breach.
- **IN-03** (same commits). The linked-channel and bot-status flights re-read their cache inside the flight.

## Deferred (accepted limits, none blocks the phase)

- **IN-01.** An approved member with a lingering hold is not cleaned by the watcher while a lookup rule is on and they have no
  cached positive; the hold ends at the member's leave, a rejoin or "I fixed it", and an approved member is exempt from
  every new punishment anyway.
- **IN-02.** The held release makes its lookups before the pause check, so a paused chat still spends lookups on held
  members; they are bounded by the lookup pacer and the pause lasts ten minutes at most.
- **IN-04.** The claim has no remaining-budget guard; the lookup phase adds up to 20 s to a 90 s job, which can make a
  claimed-but-unpunishable hold likelier. A reclaim after `memberreq.StaleClaim` retries it, as for any cut-off job.
- **IN-05.** Tests swap package globals (`memberReqRules` in `TestMemberReqLookupBroken`, the lookup pacer options, the
  phase budgets) that an abandoned flight goroutine could in principle still read under `-race`. Every CI run since plan
  14-05 passed `-race` on them; if a race is ever reported, pass the table as a parameter instead of swapping it.
- **NEW-01.** A stricter lookup rule that stays unanswerable holds the weaker name punishment back for as long as it stays
  unanswerable (the unbounded deferral is the orchestrator's decision under WR-02). For a bot that is no administrator the
  `cna` cache makes the rule inert after the first message, so only a persistent 5xx or timeout on the channel (a Telegram
  outage) can keep a name violator unpunished, and the member is judged again at every message.
- **NEW-02.** Each not-subscribed breach candidate costs one fresh `getChat` (lookup pacer, bounded); a member whose
  punishment is refused or backed off repeats it per attempt. A subscribed member costs none.
- **NEW-03.** The touch marker is set by any flight start or join, so the live record may be read once more when it was not
  strictly needed; that only costs a safe extra paced call.

## Security audit items (`14-SECURITY.md`)

- **SEC-02** (`ec6f839` RED, `7f7b0c7` GREEN, fixed). After a slow lookup phase (one that made or waited for a request, or took
  over 1 s) the executor reads the approvals and pending-captcha exemptions again before the claim
  (`memberReqRereadAfterLookups`); an exempt result or a read error punishes nobody, and an all-cache phase costs no extra read.
- **SEC-03** (same commits, fixed). A left, kicked or restricted-not-member candidate becomes a channel breach only on a FRESH
  definite administrator answer of the bot's standing (the coalesced `f:b:<channel>` flight behind the lookup pacer); a
  non-administrator or bot-gone answer makes it inert, writes `cna` and drops the stale `ba`; an unanswerable answer is
  unknown. Only breach candidates pay the extra call.
- **SEC-01** (accepted, AR-10). The linked channel's numeric ID appears in three Debug log lines. D-18 allows IDs in logs
  (chat, channel and user IDs and outcomes only, never a name, title or text); production runs at Info unless `DEBUG=true`.
  To make the stricter 'no channel ID in any log' property hold, pass the group's ID in those calls and add the channel ID
  to the log test's forbidden list.
- **SEC-04** (accepted). The pressing of the channel screen or "I fixed it" is not rate limited beyond the lookup pacer and
  the rights each press needs, and one 32-job runner serves every chat of a replica. Nobody is punished wrongly under load
  (a shed lookup is unknown); the cost is that a sustained storm sheds other groups' jobs on that replica until it drops,
  and every shed message is judged again at its sender's next message. Phase 13 documented the same shared runner.
