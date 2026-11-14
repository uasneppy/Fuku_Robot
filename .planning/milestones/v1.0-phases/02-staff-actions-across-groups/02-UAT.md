---
status: complete
phase: 02-staff-actions-across-groups
source: [02-VERIFICATION.md]
started: 2026-10-05T11:28:45Z
updated: 2026-10-05T11:43:14Z
---

## Current Test

[testing complete]

## Tests

### 1. Live run on a real bot with several linked groups (research flag A3, end-to-end run)
expected: /ban @user 1d spam from the Staff Group shows the card; only the issuer's Confirm starts it; the card turns into a summary that fills in; groups where the issuer is not an admin with restrict rights, where the target is an admin, or where the bot lacks rights show skipped or failed with the reason; the Staff Group itself is never touched
result: pass

### 2. Flood control with many linked groups and two bot replicas
expected: Staff Group edits and moderation calls stay inside Telegram limits, a real 429 is waited out, the final summary still arrives, and a retry_after of exactly 60 shows which groups (if any) read "rate limited" (WR-03)
result: pass

### 3. Second Confirm on the same target while a first run is still going
expected: Answered "target busy" for as long as the first run lasts, also past 30 minutes (lock renewal)
result: pass

### 4. Anonymous admin posts /ban in the Staff Group
expected: Reply asking to post as yourself, no card, no action
result: pass

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
