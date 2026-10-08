---
phase: 04
review: 04-REVIEW.md
titles: json
findings:
  - id: CR-01
    severity: critical
    disposition: fixed
    title: "The worker's ban overwrites a deliberate ban, and the lift then removes it (violates D-05)"
  - id: WR-01
    severity: warning
    disposition: fixed
    title: "An ambiguous lock failure deletes the only copy of the pre-lockdown permissions and says \"nothing changed\""
  - id: WR-02
    severity: warning
    disposition: fixed
    title: "Joiner bans expire after 330 days, so a lockdown can outlive its own enforcement"
  - id: IN-01
    severity: info
    disposition: open
    title: "Duplicated join-policy conditions and an unreachable verdict"
  - id: IN-02
    severity: info
    disposition: open
    title: "AGENTS.md says the snapshot never goes through the typed `ChatPermissions`, but `resolveUnmutePermissions` does that"
  - id: IN-03
    severity: info
    disposition: open
    title: "A fourth copy of the Group Anonymous Bot ID"
  - id: IN-04
    severity: info
    disposition: open
    title: "Fed-ban enforcement is skipped for banned joiners of a locked group, and a \"request gone\" is recorded as declined"
open: 4
total: 7
recorded: 2026-10-07T04:19:19.425Z
---

# Phase 04: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| CR-01 | critical | fixed | 04-REVIEW-FIX.md |
| WR-01 | warning | fixed | 04-REVIEW-FIX.md |
| WR-02 | warning | fixed | 04-REVIEW-FIX.md |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
