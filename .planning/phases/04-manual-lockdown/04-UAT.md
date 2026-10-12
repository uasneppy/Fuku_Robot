---
status: complete
phase: 04-manual-lockdown
source: [04-VERIFICATION.md]
started: 2026-10-07T04:29:21Z
updated: 2026-10-08T13:35:24Z
---

## Current Test

[testing complete]

## Tests

### 1. Run the PostgreSQL 16 checks that the executors could not run: apply the whole migration chain, then TestStartLockdownOneActivePerChat against that database (the command is in 04-08-SUMMARY.md under 'Items for the orchestrator')
expected: '--- PASS: TestRepositoryMigrationChain', a 'lockdown repository backend: postgres' line, '--- PASS: TestStartLockdownOneActivePerChat', and no '--- SKIP'
why_human: Needs 'su postgres' for a throwaway cluster, which the sandbox refuses (it was refused for the executors and was not circumvented here). Every lockdown repository test so far ran on SQLite, so the uk_chat_lockdowns_active partial unique index, ON CONFLICT DO NOTHING without a target, the NOT EXISTS in FinishLift and the IN (SELECT ...) in ReleaseStaleClaims are unproven on the production database.
result: pass
evidence: "Run by Claude from the main checkout on a throwaway PostgreSQL 16 cluster at the owner's request: --- PASS TestRepositoryMigrationChain; lockdown repository backend: postgres; --- PASS TestStartLockdownOneActivePerChat (5 subtests incl. eight concurrent starts and a raw duplicate insert refused by the index); no SKIP."

### 2. Run make lint on a machine whose golangci-lint is built with Go 1.26
expected: Exit 0 with no new issues in the phase's files
why_human: The installed golangci-lint is built with go1.25 and refuses the go1.26.0 module, so the lint gate has never run on Phase 4 code.
result: pass

### 3. In a real supergroup set unusual default permissions (text on, photos off, reactions off, invite on), run /lockdown, compare the Permissions screen, run /unlockdown and compare again. Save the raw getChat permissions answers as a test fixture.
expected: The Permissions screen after /unlockdown is identical to before. Locked: members cannot send anything, admins still can.
why_human: The raw getChat permissions shape and the setChatPermissions replay (research A1) can only be confirmed against real Telegram. The phase's central promise (settings survive) rests on it.
result: pass
note: "Owner confirmed the Permissions screen matched after /unlockdown; no raw getChat fixture was supplied."

### 4. In a test supergroup run /lockdown, then join through (a) a normal invite link, (b) a join-request link and (c) an admin adding the user directly. Also repeat with a bot account being added by an admin.
expected: Each normal joiner is banned once with no welcome and no captcha, the join request is declined, the admin-added human stays, the bot is banned; all banned joiners can rejoin after /unlockdown.
why_human: Live delivery order and duplicate delivery of chat_member, new_chat_members and chat_join_request (D-03) cannot be reproduced by the fake dispatcher. ext.EndGroups on a real update is only proven in the fake.
result: pass

### 5. Leave a join request pending, run /lockdown, then approve that request from Telegram's own request list as an admin.
expected: The person gets in. If the accepted D-24 race bans them instead, /unlockdown unbans them.
why_human: Depends on Telegram's update order (research A3/A5); the residual race is accepted by the owner (D-24) but its real outcome was never seen.
result: pass

### 6. During a lockdown let a joiner be banned, read getChatMember for them and compare until_date with the stored ban_until (join row), then lift.
expected: until_date is within 2 s of the stored value, and the joiner is unbanned at the lift
why_human: Telegram's until_date echo and rounding (research A2) is assumed by isLockdownBan; if it is off, every lift would keep every raider banned.
result: pass

### 7. Lock a group that is linked to a Staff Group, open /staff in the Staff Group, then lift and press Refresh
expected: A '🔒 in lockdown since <date> UTC' line appears on that group's row only, with no reason and no name, and disappears after the lift and refresh
why_human: Live panel rendering in a real Staff Group (SETUP-08 deferred status).
result: pass

### 8. With auto-approve on and join requests required, run /lockdown and request to join from a second account; then with auto-approve off tap Accept on a posted approve card during a lockdown; run /unlockdown and try again
expected: The request is declined within seconds, requesting again is declined again, Accept shows the lockdown alert and approves nothing, and after the lift new requests behave normally
why_human: Whether a declined person can request again at once (research A11) and the Accept button alert in a real client are live behaviours.
result: pass

### 9. Lock two real groups, lift one
expected: The other keeps its restricted permissions, its removed joiners and its /staff marker
why_human: Cross-group isolation (LOCK-04) is proven in the fake; a live two-group run confirms nothing leaks in real Telegram.
result: pass

### 10. Lock a group, let a test account be banned by the lockdown, send /tban <id> 1d from the Staff Group, lift the lockdown
expected: The account is still banned in that group after the lift
why_human: The staff-ban-over-lockdown-ban branch (D-05) is proven with a fake; the real until_date semantics of banChatMember on a kicked user are assumed.
result: pass

### 11. Restart drills: (1) restart the bot with joiners pending and (2) restart in the middle of a lift with many banned joiners, then (3) flush Redis during a lockdown, ideally with two replicas running
expected: Pending joiners are banned after the restart, the lift resumes and posts one tally, a Redis flush changes nothing, and both replicas enforce the lockdown without banning anyone twice
why_human: Restart, Redis-loss and multi-replica behaviour are proven with an in-process worker and miniredis, not with real processes.
result: pass

### 12. As an anonymous admin run /lockdown, tap the proof button, then do the same for /unlockdown and /lockdownstatus, with the group's AnonAdmin mode on and off
expected: The proof button always appears, the tapper is the one named as locking or lifting, and a non-admin tapper is refused
why_human: The anonymous-admin flow is proven with the fake; a real tap on the real proof button is a live check.
result: pass

### 13. In a real group with a captcha pending, run /lockdown, let the user pass the captcha, run /unmute on another muted user, then /unlockdown
expected: Both users can talk after the lift (LOCK-08)
why_human: Real per-user restriction semantics (a user's own restriction copied from the snapshot while the default is locked) is what keeps them from being muted after the lift; the fake cannot prove how Telegram combines the two.
result: pass

## Summary

total: 13
passed: 13
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
