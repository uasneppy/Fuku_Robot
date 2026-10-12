---
phase: 03
review: 03-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: skipped
    title: "`Reached` is set for outcomes that provably made no write, so the claim is still burned for them"
  - id: WR-02
    severity: warning
    disposition: skipped
    title: "\"Undo changed nothing\" is shown for groups whose outcome is unknown or probably applied"
  - id: WR-03
    severity: warning
    disposition: open
    title: "A retry after an ambiguous `ReleaseUndo` error can run `FinalizeUndo` on an already released record"
  - id: WR-04
    severity: warning
    disposition: open
    title: "A history page press is answered before the data is read, so a database error is a silent no-op (carried forward; was WR-03)"
  - id: IN-01
    severity: info
    disposition: open
    title: "The original summary is marked \"see the reply\" even when the undo's own summary was never delivered"
  - id: IN-02
    severity: info
    disposition: skipped
    title: "The \"left behind\" predicate for an unmute accepts any non-muted state (carried forward; was WR-04, accepted by the owner at UAT)"
  - id: IN-03
    severity: info
    disposition: open
    title: "`ClaimUndo` is not bound to the Staff Group in SQL (carried forward; was IN-02)"
  - id: IN-04
    severity: info
    disposition: open
    title: "`RekeyChat` bumps `updated_at`, which now also flips dead undos back to \"running\" (carried forward; was IN-03)"
  - id: IN-05
    severity: info
    disposition: open
    title: "The history timestamp format is English-only in every locale (carried forward)"
  - id: IN-06
    severity: info
    disposition: open
    title: "The Prev offset ignores a page that was shrunk to fit the length cap (carried forward)"
  - id: IN-07
    severity: info
    disposition: open
    title: "`staffCallRestore` is defined by arithmetic outside the iota block (carried forward; was IN-01)"
open: 8
total: 11
recorded: 2026-10-06T11:51:17.012Z
---

# Phase 03: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | skipped | Owner kept the rule: any attempted write keeps the claim (03-UAT test 7, option a) |
| WR-02 | warning | skipped | Owner kept "changed nothing" for unconfirmed outcomes (03-UAT test 8, option a) |
| WR-03 | warning | open | - |
| WR-04 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | skipped | Owner accepted the broad unmute predicate (03-UAT test 3, option a); was WR-04 in the first review |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |
| IN-07 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
