---
phase: 15
review: 15-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: fixed
    title: "A durable AI hold is released, and \"I fixed it\" lifts the mute, with no judgement when Redis has lost the baseline and the marker"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "The baseline never refreshes, so any member silent for 30 days is re-baselined without judgement, whatever names they carry"
  - id: WR-03
    severity: Warning
    disposition: fixed
    title: "A Redis outage or read error is a plain unknown, so it suspends every weaker name punishment in groups whose AI action is stricter"
  - id: WR-04
    severity: Warning
    disposition: fixed
    title: "While the breaker is paused every message of every member with a join marker is a job and a paced live lookup"
  - id: IN-01
    severity: Info
    disposition: deferred
    title: "The verdict key and the baseline ignore case, but the text sent to the model keeps it"
  - id: IN-02
    severity: Info
    disposition: deferred
    title: "A refusal is never cached and counts as unknown, so it costs a call per message and defers weaker punishments"
  - id: IN-03
    severity: Info
    disposition: fixed
    title: "The spike's names CSV writes attacker-controlled names unescaped"
  - id: IN-04
    severity: Info
    disposition: deferred
    title: "One flight serves every chat, so one chat's breaker, budget or cap decides the answer of another chat's caller"
  - id: NEW-01
    severity: Info
    disposition: deferred
    title: "Fix pass: a member held only for a name rule is not judged for the AI rule while that hold lives, if the AI answer was a sticky unknown"
  - id: NEW-02
    severity: Info
    disposition: deferred
    title: "Fix pass: WR-01 costs one provider call per held bit-128 member after a Redis loss, and a member who renames during a breaker pause stays held until the pause ends"
  - id: NEW-03
    severity: Info
    disposition: deferred
    title: "Fix pass: WR-04 adds one Redis-only read per unsatisfied message while a pause is on and the group's AI rule is live"
open: 0
total: 11
---

# Phase 15: Code Review Disposition

Findings WR-01 to IN-04 come from `15-REVIEW.md` (deep review of the 24 production files the phase changed: 0 critical, 4 warnings,
4 info, each warning reproduced with a throwaway test). NEW-01 to NEW-03 are the concerns the fix pass reported
(`15-REVIEW-FIX.md`) and are dispositioned once here. The five fixes went through one single-writer fix pass, each as a RED test
commit followed by a GREEN fix, and the pass was proved through CI's own gate (`go test -json` into
`scripts/check_test_results`, exit 0, 3118 test events passed, none failed or skipped). The security audit (`15-SECURITY.md`) is
the next step and adds its own entries below if it finds any.

## Fixed

- **WR-01** (`89883df` RED, `122e732` GREEN). A hold that carries bit 128 is never released, and "I fixed it" never lifts its
  mute, by a baseline-only pass. `memberReqLookupMode` gains `IgnoreBaseline`, set by the held release and the button (`Joining`
  keeps meaning only the join entry's run), so with `nv`, `nm` and `nu` all lost the AI row answers from the verdict cache or one
  real call. The baseline-only step stays right for an established member with no hold.
- **WR-02** (`2c57ca9`, `0b1ac70`). The baseline `nm` lives 180 days (`memberReqAIBaselineTTL`); the marker `nu` stays at 30. The
  equal-baseline pass is still a no-write step. Accepted and documented (AGENTS.md, `15-SPIKE.md`): a member dormant for over 180
  days is first-sighted again and baselined with no call, the same D-02 limit.
- **WR-03** (`75894d0`, `1079516`). No Redis client, a failed state read, a failed in-flight cache re-read and a budget-counter
  fault are sticky unknowns (the D-22 mechanism, with the sticky bit carried from the prepare step in `memberReqAIPlan`), so a
  Redis outage defers no weaker name punishment. The verdict stays unknown for the held release and "I fixed it".
- **WR-04** (`e3adc29`, `03c10a3`). The watcher consults the in-memory `memberReqAIPaused`: when the AI part is the only reason a
  member is not satisfied and the breaker is paused for the chat, it starts no job and takes no clean path. This is a third
  outcome, not "satisfied", because "satisfied" would run `ClearHoldOnClean` and delete the hold of a member held for an AI breach
  (the orchestrator corrected the reviewer's suggestion). Any other open reason still hands a job.
- **IN-03** (`7930063`, `97496fa`). The spike's names CSV prefixes a single quote to a name cell that starts with `=`, `+`, `-`,
  `@`, tab or CR; the input file's parsing is unchanged.

## Deferred (accepted limits, none blocks the phase)

- **IN-01.** The verdict key and the baseline ignore case while the text sent keeps it. It needs the model to score two case
  variants of one name on opposite sides of 0.9, and a fix would bump `mrn1` and `mrv1` and invalidate every baseline and cached
  verdict.
- **IN-02.** A refusal is never cached. It is bounded by the per-account hourly budget (5), so it is waste and a short deferral of
  weaker punishments, not a bypass.
- **IN-04.** One flight serves every chat, so one chat's breaker or budget can decide another chat's caller for that moment. It is
  transient and needs the same canonical string in two chats at the same instant; the answer is a sticky unknown, never a breach.
- **NEW-01.** A member held only for a name rule is not judged for the AI rule while that hold lives, if the AI answer was a sticky
  unknown (the D-22 partial episode; WR-03 widens its causes to Redis faults). The member is judged when the hold clears.
- **NEW-02.** WR-01 costs one provider call per held bit-128 member after a Redis loss, bounded by the three budgets and cached
  after. A member who renames during a breaker pause stays held until the pause ends, at most 5 minutes per pause.
- **NEW-03.** The WR-04 check makes one extra Redis-only read per unsatisfied message, only while a pause is on and the group's AI
  rule is live; an unpaused group pays nothing extra.
