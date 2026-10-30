---
phase: 01
review: 01-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: open
    title: "Staff Group \"owner changed\" / \"status removed\" notices are unbounded and can be lost after the data is already deleted"
  - id: WR-02
    severity: warning
    disposition: open
    title: "A link to a deleted or unreachable group can never be unlinked on its own"
  - id: WR-03
    severity: warning
    disposition: open
    title: "Callback queries are answered only after a full live panel rebuild (up to 45 s)"
  - id: WR-04
    severity: warning
    disposition: open
    title: "`/unsetstaff` Cancel button has no authority check"
  - id: WR-05
    severity: warning
    disposition: open
    title: "`/setstaff` on an existing Staff Group never refreshes the recorded owner"
  - id: WR-06
    severity: warning
    disposition: open
    title: "Two links resolved from a stale panel row can be unlinked without re-reading the link after the live checks"
  - id: IN-01
    severity: info
    disposition: open
    title: "Documentation page omits `/linkstaff` and `/unlinkstaff` from the command table and states the wrong permission"
  - id: IN-02
    severity: info
    disposition: open
    title: "Comments reference planning artifacts and plans, one of them inaccurate"
  - id: IN-03
    severity: info
    disposition: open
    title: "A single \"no creator listed\" observation removes every link of a Staff Group"
  - id: IN-04
    severity: info
    disposition: open
    title: "Minor duplication and unbounded Redis call in the sweeper"
open: 10
total: 10
recorded: 2026-10-05T00:15:45.664Z
---

# Phase 01: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| WR-03 | warning | open | - |
| WR-04 | warning | open | - |
| WR-05 | warning | open | - |
| WR-06 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
