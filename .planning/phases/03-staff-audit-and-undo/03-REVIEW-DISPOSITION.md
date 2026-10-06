---
phase: 03
review: 03-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: open
    title: "The one-shot undo claim is consumed even when no group was (or could be) undone"
  - id: WR-02
    severity: warning
    disposition: open
    title: "History and the original summary say \"undone\" regardless of what the undo did; a crashed undo reads \"undone\" with ⏳ lines forever"
  - id: WR-03
    severity: warning
    disposition: open
    title: "A history page press is answered before the data is read, so a database error is a silent no-op"
  - id: WR-04
    severity: warning
    disposition: skipped
    title: "The \"left behind\" predicate for an unmute accepts any non-muted state, so undo can overwrite another admin's newer restriction"
  - id: IN-01
    severity: info
    disposition: open
    title: "`staffCallRestore` is defined by arithmetic outside the iota block"
  - id: IN-02
    severity: info
    disposition: open
    title: "`ClaimUndo` is not bound to the Staff Group in SQL"
  - id: IN-03
    severity: info
    disposition: open
    title: "`RekeyChat` bumps `updated_at` on every history row, which can flip dead runs back to \"running\""
  - id: IN-04
    severity: info
    disposition: open
    title: "Two tests synchronise with a fixed `time.Sleep(50ms)`"
  - id: IN-05
    severity: info
    disposition: open
    title: "The history timestamp format is English-only in every locale"
  - id: IN-06
    severity: info
    disposition: open
    title: "The Prev offset ignores a page that was shrunk to fit the length cap"
open: 9
total: 10
recorded: 2026-10-05T20:12:26.912Z
---

# Phase 03: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| WR-03 | warning | open | - |
| WR-04 | warning | skipped | Owner accepted the broad unmute predicate (03-UAT test 3, option a) |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
