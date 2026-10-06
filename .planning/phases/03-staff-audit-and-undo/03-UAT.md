---
status: testing
phase: 03-staff-audit-and-undo
source: [03-VERIFICATION.md]
started: 2026-10-05T20:35:00Z
updated: 2026-10-06T09:04:51Z
---

## Current Test

number: 3
name: DECISION: undo of an unmute and D-04 (code review WR-04)
expected: |
  Owner chooses (a) accept the documented broad "left behind" predicate for unmute (member, left, or restricted-but-able-to-send), or (b) open a gap-closure plan that stores what the unmute applied and requires equality. Today staffUndoLeftBehind (alita/modules/staff_action_decide.go:375-381) lets an undo of an unmute re-apply the old mute over a later partial restriction by another admin, or over a target who was kicked and rejoined.
awaiting: user response

## Tests

### 1. Live Telegram restore check (03-VALIDATION.md Manual-Only rows 1 and 2; 03-09 Task 2 human-check)
expected: Group A is restricted again with text allowed and media blocked, ending at the original time. Group B is banned again until the original end time. In /staff, Recent actions and the entry's detail, both actions show as undone and the detail lists each group's result and undo result. (Research A2, A3: fakes cannot prove live Telegram behaviour.)
result: pass

### 2. Log-channel posts end to end (03-VALIDATION.md Manual-Only row 3)
expected: Each linked group's log channel has a #STAFF_BAN or #STAFF_UNBAN post and a #STAFF_UNDO post naming the issuer and the presser, the target, the reason and "via Staff Group", and never the Staff Group's title or ID.
result: pass

### 3. DECISION: undo of an unmute and D-04 (code review WR-04)
expected: Owner chooses (a) accept the documented broad "left behind" predicate for unmute (member, left, or restricted-but-able-to-send), or (b) open a gap-closure plan that stores what the unmute applied and requires equality. Today staffUndoLeftBehind (alita/modules/staff_action_decide.go:375-381) lets an undo of an unmute re-apply the old mute over a later partial restriction by another admin, or over a target who was kicked and rejoined.
result: [pending]

### 4. DECISION: undo claim spent before any group is attempted (WR-01) and history says "undone" regardless of effect (WR-02)
expected: Owner chooses whether D-09 ("one undo per action") covers the zero-effect case (a Staff Group member who is an admin in no linked group presses Undo and Confirm, every group is skipped, and nobody can undo that action any more), or whether the claim should be released when no group reached a Telegram write, and whether the history line should say "undone" only when at least one group's undo succeeded (alita/modules/staff_undo.go:399-428, alita/modules/staff_history.go:69-80).
result: [pending]

### 5. make lint on a go1.26 toolchain (or CI)
expected: No new lint findings in the Phase 3 files. The installed golangci-lint was built with go1.25 and cannot run on this module (go 1.26.0).
result: [pending]

## Summary

total: 5
passed: 2
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
