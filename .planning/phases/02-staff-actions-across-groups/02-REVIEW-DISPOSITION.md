---
phase: 02
review: 02-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: warning
    disposition: open
    title: "Final summary can be lost under flood control; the delivery budget is shorter than the waits it tries to honor"
  - id: WR-02
    severity: warning
    disposition: open
    title: "A 429 block longer than `MaxWait` makes every other paced call sleep through the whole block, which can outlive the target lock"
  - id: IN-01
    severity: info
    disposition: open
    title: "The \"shared\" duration grammar is not shared, so the two parsers can drift"
  - id: IN-02
    severity: info
    disposition: open
    title: "`FindUsersByUsername` ordering is not deterministic and can put NULL activity first"
  - id: IN-03
    severity: info
    disposition: open
    title: "`staff_act_hint_bad_target` says only a numeric ID is accepted"
  - id: IN-04
    severity: info
    disposition: open
    title: "A write cut off by shutdown is reported as \"interrupted\" although Telegram may have applied it"
  - id: IN-05
    severity: info
    disposition: open
    title: "Pacer marks Redis as down when its own context is cancelled"
  - id: IN-06
    severity: info
    disposition: open
    title: "Any Staff Group member's tap on a card whose Redis hash is gone rewrites the message"
open: 8
total: 8
recorded: 2026-10-05T07:58:01.947Z
---

# Phase 02: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | warning | open | - |
| WR-02 | warning | open | - |
| IN-01 | info | open | - |
| IN-02 | info | open | - |
| IN-03 | info | open | - |
| IN-04 | info | open | - |
| IN-05 | info | open | - |
| IN-06 | info | open | - |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
Re-running the gate keeps every row it can. A row the current review no longer reports is kept and its Source cell flagged, so a finding does not leave this record silently. ONE exception: when a finding id is REUSED by a different finding, the earlier decision cannot keep a row — the id is taken — and it is dropped. A RECORDED decision (anything but `open`) is named on the console when that happens; a row still at `open` is replaced silently, because `open` records no decision to lose.
