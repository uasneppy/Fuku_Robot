---
status: complete
phase: 01-staff-group-links
source: [01-VERIFICATION.md]
started: 2026-10-05T00:22:22Z
updated: 2026-10-05T00:30:33Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

[testing complete]

## Tests

### 1. Anonymous-admin delivery (plan 01-02 / 01-05 human-check, research A4). In a throwaway supergroup where the bot is admin, turn on 'Remain anonymous' for the owner and send /setstaff, /linkstaff and the picker's /start payload; then turn it off and repeat.
expected: Anonymous posts get the 'post as yourself' reply (self-deleting in the issuing group for link attempts) and nothing is created; the non-anonymous posts succeed.
result: pass

### 2. Add group picker (plan 01-05 human-check, research A3/A12). In /staff press 'Add group' and pick (a) a supergroup where the bot is already a member and admin, (b) one where it is not a member, (c) once with 'Remain anonymous' on.
expected: Picker accepts (a) and (b); the bot's existing admin rights are not reduced (restrict + delete are combined with them); /start@bot stf_... arrives with the owner as sender, is deleted, and the confirmation appears only in the Staff Group. /linkstaff is the fallback if the picker misbehaves.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 3. Ownership transfer with bot as admin (plan 01-07 human-check, research A1/A5). In a throwaway supergroup linked to a Staff Group, transfer ownership to another account while logging raw updates. Repeat for the Staff Group itself.
expected: A chat_owner_changed service message arrives (record whether a creator chat_member update also arrives); the link disappears within seconds; the Staff Group gets exactly one 'owner changed' notice; nothing appears in the linked group. For a Staff Group transfer, the old owner's links are removed and the Staff Group keeps its status.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 4. Ownership transfer with bot as plain member (plan 01-10 human-check, research A2/A5). Demote the bot to member in a linked throwaway supergroup, transfer ownership, then wait for (or trigger by restarting the bot) the next sweep (first run 1-5 minutes after start).
expected: Record whether chat_owner_changed still reached the bot. Within one sweep the link is removed and the Staff Group gets exactly one 'owner changed' notice; getChatAdministrators succeeded for the non-admin bot (no 'unknown' log for that group). If the admin list is unreadable for a non-admin bot, such links stay unknown and are never removed until the bot is admin (documented A2 fallback).
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 5. Bot removal, demotion and re-add (plan 01-08 human-check). In a linked throwaway supergroup remove the bot, add it back as admin with the ban right, then take the ban right away once.
expected: The Staff Group gets one heads-up per change and one 'healthy again'; nothing appears in the linked group; the link stays listed throughout.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 6. /staff panel appearance and Refresh (plan 01-09 human-check). In a real Staff Group with at least two linked groups (bot demoted to member in one), send /staff, press Refresh, then promote the bot back and press Refresh again.
expected: One message shows each group with readable status icons and a reason line for the broken one; Refresh edits the same message and the 'Updated' time changes; after promoting the bot the group shows healthy and the Staff Group got exactly one 'healthy again' heads-up. Also page past 8 groups if practical.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 7. Native-speaker read of non-English text (plans 01-08 and 01-09). Have speakers of es, fr, hi, id, pt, ru skim the 63 staff_* strings, especially the four health heads-up strings and the 14 panel strings.
expected: Wording reads naturally and keeps every placeholder; panel fits one message in each language.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 8. Unlink leaves no trace in the linked group (plan 01-06 coverage item D8). In a real client run /unlinkstaff in a linked group and press Unlink in the Staff Group panel.
expected: /unlinkstaff is deleted; refusals disappear after about 30 seconds; Unlink buttons appear only in the Staff Group's /staff reply; members of the linked group see nothing.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 9. Real basic-group to supergroup upgrade (plan 01-03 coverage item D4). Upgrade a Staff Group, and separately a linked group, from a basic group to a supergroup (or otherwise change its chat ID).
expected: /staff keeps working under the new ID and the links follow. (Dispatcher wiring of the migrate watcher was checked by the verifier with a throwaway test, see Behavioral Spot-Checks; only the live Telegram upgrade remains.)
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 10. PostgreSQL migration and trigger on the deploy path (plan 01-01 D5, plan 01-04). Deploy with AUTO_MIGRATE=true against a real PostgreSQL and run ALITA_TEST_DATABASE=true make test-postgres-integrity in CI.
expected: Both migrations apply once, checksums recorded; TestStaffExclusivityTrigger passes (5 subtests including the 20-round concurrency race) and is not skipped.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

### 11. Concurrent /unsetstaff Confirm on two replicas (plan 01-02 coverage items D9, D10, verification: backstop). Press Confirm twice at the same time against two running replicas.
expected: Status is removed once, exactly one 'removed' notice, and either all links go or none do.
result: skipped
reason: "Deferred follow-up: Just mark all tests complete for now"

## Summary

total: 11
passed: 1
issues: 0
pending: 0
skipped: 10
blocked: 0

## Gaps

## Deferred Follow-Ups

- test: 2
  name: "Add group picker"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 3
  name: "Ownership transfer with bot as admin"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 4
  name: "Ownership transfer with bot as plain member"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 5
  name: "Bot removal, demotion and re-add"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 6
  name: "/staff panel appearance and Refresh"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 7
  name: "Native-speaker read of non-English text"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 8
  name: "Unlink leaves no trace in the linked group"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 9
  name: "Real basic-group to supergroup upgrade"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 10
  name: "PostgreSQL migration and trigger on the deploy path"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
- test: 11
  name: "Concurrent /unsetstaff Confirm on two replicas"
  idea: "Just mark all tests complete for now"
  deferred_at: 2026-10-05
