---
phase: 03-staff-audit-and-undo
verified: 2026-10-05T20:30:00Z
status: human_needed
score: 4/6 must-haves verified
covered_files:
  - .planning/phases/03-staff-audit-and-undo/03-01-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-01-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-02-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-02-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-03-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-03-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-04-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-04-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-05-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-05-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-06-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-06-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-07-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-07-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-08-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-08-SUMMARY.md
  - .planning/phases/03-staff-audit-and-undo/03-09-PLAN.md
  - .planning/phases/03-staff-audit-and-undo/03-09-SUMMARY.md
  - alita/db/models/staff_action.go
  - alita/db/staff/actions.go
  - alita/db/staff/rekey.go
  - alita/modules/staff.go
  - alita/modules/staff_action_card.go
  - alita/modules/staff_action_decide.go
  - alita/modules/staff_action_record.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_action_summary.go
  - alita/modules/staff_history.go
  - alita/modules/staff_log.go
  - alita/modules/staff_panel.go
  - alita/modules/staff_undo.go
  - alita/utils/actionlog/actionlog.go
  - migrations/20261005120000_add_staff_actions.sql
covered_digest: "v2:sha256:afc0ed90ded1873bda4be2d54d65fa8ff2dffbafa605b351efc5fa75903814ee"
behavior_unverified: 1
overrides_applied: 0
behavior_unverified_items:
  - truth: "Undo puts the exact prior state back on live Telegram: restrictChatMember with use_independent_chat_permissions on a restricted prior, a restore or re-ban sent to a kicked or left target (research A2, A3)"
    test: "Link two test supergroups, each with a log channel, to a test Staff Group. In group A restrict a test account to text-only for 1 day, run a staff /ban, Confirm, then Undo everywhere and Confirm. In group B ban the account for 2 hours with the per-group /ban, run a staff /unban, then Undo it."
    expected: "Group A: restricted again, text allowed and media blocked, ending at the original time. Group B: banned again until the original end time."
    why_human: "Telegram's server-side semantics for restrict/ban on kicked or left users and for independent chat permissions exist only in the hand-written fake. Presence and wiring are proven, the live effect is not."
human_verification:
  - test: "Live Telegram restore check (03-VALIDATION.md Manual-Only rows 1 and 2; 03-09 Task 2 human-check)"
    expected: "See behavior_unverified_items above. Also open /staff, Recent actions and the entry's detail: both actions show as undone and the detail lists each group's result and undo result."
    why_human: "Fakes cannot prove live Telegram behaviour (research A2, A3)."
  - test: "Log-channel posts end to end (03-VALIDATION.md Manual-Only row 3)"
    expected: "Each linked group's log channel has a #STAFF_BAN or #STAFF_UNBAN post and a #STAFF_UNDO post naming the issuer and the presser, the target, the reason and 'via Staff Group', and never the Staff Group's title or ID."
    why_human: "Needs real channels. The fake proves the text and the routing, not delivery into a real channel."
  - test: "DECISION: undo of an unmute and D-04 (code review WR-04)"
    expected: "Owner chooses: (a) accept the documented broad 'left behind' predicate for unmute (member, left, or restricted-but-able-to-send), or (b) open a gap-closure plan that stores what the unmute applied and requires equality."
    why_human: "staffUndoLeftBehind (staff_action_decide.go:375-381) lets an undo of an unmute re-apply the old mute over a later partial restriction by another admin, or over a target who was kicked and rejoined. D-04 says 'on any difference ... skipped'. Research A5 and plan 03-02 chose the broad predicate on purpose because Telegram may report member, left or restricted for a target restricted to the group's default permissions. This is a locked-decision trade-off, so it is the owner's call."
  - test: "DECISION: the undo claim is spent before any group is attempted (code review WR-01) and history says 'undone' regardless of effect (WR-02)"
    expected: "Owner chooses whether D-09 ('one undo per action ... staff fix them by hand') covers the zero-effect case (a Staff Group member who is an admin in no linked group presses Undo and Confirm, every group is skipped, and nobody can undo that action any more), or whether the claim should be released when no group reached a Telegram write, and whether the history line should say 'undone' only when at least one group's undo succeeded."
    why_human: "Both are real in the code (staff_undo.go:399-428, staff_history.go:69-80). D-09 states the one-shot rule without exempting a no-op undo, so this is a judgment call, not a clear violation."
  - test: "make lint on a go1.26 toolchain (or CI)"
    expected: "No new lint findings in the Phase 3 files."
    why_human: "The installed golangci-lint was built with go1.25 and cannot run on this module (go 1.26.0). 03-VALIDATION.md and AGENTS.md require it at the end of the phase."
---

# Phase 3: Staff Audit and Undo Verification Report

**Phase Goal:** As a staff member, I want to log, list and undo every staff action, so that mistakes can be reversed and we stay accountable.
**Verified:** 2026-10-05T20:30:00Z
**Status:** human_needed
**Re-verification:** No, initial verification

## Goal Achievement

The code delivers all four roadmap criteria. I found no missing, stubbed or unwired artifact and no blocker. The status is `human_needed` because live-Telegram behaviour is modeled only in the fake, and because three deviations from locked decisions or from the audit's honesty need an owner decision (WR-04, WR-01, WR-02). None of them blocks a criterion as the roadmap words it.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: every group where a staff action was applied gets a log-channel post with action, target, issuer and reason | ✓ VERIFIED | `startStaffActionRun` AfterGroup (`staff_action_run.go:353-359`) saves the group result, then calls `postStaffActionLog` only when `res.Outcome == staffOutcomeDone`. `composeStaffActionLog` (`staff_log.go:62-77`) builds hashtag, issuer mention, target, duration (ban/mute), reason (escaped, capped at 300 runes) and "via Staff Group", and never reads `card.StaffChat`. `sendStaffLogPost` goes through `actionlog.Destination` (admin category, no post if the group has no channel or the category is off) and `staffPaced`, and a failed post is only logged, so it cannot change the result. Tests `TestStaffActionLogPosts`, `...CategoryOff`, `...FailureKeepsDone`, `...RateLimitedKeepsDone`, `...Paced`, `...ReasonCapped`, `...NoneWhenNothingApplied`, `...SharedChannel` pass (my own run). Residual: a shutdown skips remaining log posts (`ctx.Err()` guard) and AGENTS.md documents it. |
| 2 | SC2: every staff action is recorded with issuer, target, action, duration, reason, time and per-group outcomes; the record survives restarts | ✓ VERIFIED | Migration `20261005120000_add_staff_actions.sql` creates `staff_actions` (issuer, target, action with CHECK incl. kick, reason, duration_sec/amount/unit, over_limit, until_date, created_at, finished_at, undo_*) and `staff_action_groups` (outcome, reason, detail, prior_*, applied_at, undo_*). Models match column for column and `TableName()` returns the migration names. `staffActionConfirm` (`staff_action_card.go:903-911`) creates the record before the run and aborts the card when the create fails. Each group's prior state is written before its Telegram write and a failed save fails that group closed (`staff_action_run.go:527-534`). Results are saved per group and finalized in one transaction through `db.DB`, never the run context (`staff/actions.go:135-155`). `StopStaffActions` is registered in `main.go:218` before DB close in LIFO order. PostgreSQL chain and integrity checks passed per the orchestrator. Tests `TestStaffActionRecordCreate/RoundTrip/PerGroup/Finalize/EveryKind/FailureFailsClosed/NoRecordOnAbort`, `TestStaffActionRecordsPriorState`, `TestStopStaffActionsRecordsInterrupted` pass. |
| 3 | SC3: `/staff` lists recent staff actions with who, what, target, when, reason and the outcome in each group | ✓ VERIFIED (WR-02 caveat) | The panel keyboard carries the Recent actions button (`staff_panel.go:317`), routed in `staffCallback` (`staff.go:282-289`). `staffHistoryLine` renders icon and action, target name and ID, duration, reason, issuer, time and the ✅/⏭/❌ tally; `renderStaffHistoryDetail` lists every group in the summary's line format, marks unlinked groups and shows the undo's per-group results. Access needs a live Staff Group member (`staffHistoryGate`), reads are scoped by `staff_chat_id`, and 10 entries per page, newest first, with offset paging. Tests `TestStaffHistory*` (list, detail, access, length cap, callback budget, unlinked, unfinished, same-second, shrunk page) pass. Caveat: the list line and detail header say "undone" for any claimed undo, even one where every group was skipped or failed (WR-02, see decision item). The per-group undo lines stay truthful. |
| 4 | SC4: "Undo everywhere" reverses the action in each group where the presser is a Staff Group member and an admin with restrict rights; other groups are skipped with the reason; the result is in the same done/skipped/failed summary; an outsider can't undo anything | ✓ VERIFIED against the fake (see Truth 6) | Undo button exists only on the final summary of a finalized, non-kick record with at least one applied group (`staff_action_run.go:363-376`, `staffActionUndoable`). `staffUndoAsk` and `staffUndoConfirm` both re-check live Staff Group membership, the record's `staff_chat_id` against the pressed chat, and refuse service identities; only the presser can confirm (compare-and-set on the card issuer). Per group, `staffGroupPrechecks` runs with the presser as actor: link owner recheck, live `getChatMember` creator or admin with `can_restrict_members` (never the admin cache), bot and service guards, live target. Only `done` groups are visited. `decideStaffUndo` is the only place undo calls are chosen and `executeStaffUndoCall` switches only on the verdict. The undo takes the same per-target lock as an action. `ClaimUndo` is one conditional update, so an undo runs once. Results use the same ✅/⏭/❌ renderer, overflow rules and `staffPaced`. Note: D-08 (locked) delivers the result as a reply to the original summary, which is edited with "Undone by", not inside the same message. The roadmap's "same summary" is read there as the same format and rules, which the code does. Tests `TestStaffUndoAccess/Outsider/PresserRights/PresserNeedsTheRestrictRight/OnlyApplied/NotAppliedOriginally/LinkRemoved/Once/OnceAcrossReplicas/TargetLock/Order/AfterRekey/ButtonRules`, `TestStopStaffActionsUndo` pass. |
| 5 | D-02/D-04 (plan 03-02 prohibition): undo restores exactly the recorded prior state and never overrules a later decision | ? UNCERTAIN (owner decision, WR-04) | Verified for ban, mute and unban: `staffUndoLeftBehind` compares status and end date (with the 30-second permanent-ban rule), and restore uses the stored permission set and original `until`. `decideStaffUndo` skips target admins, missing prior state, ended restrictions (120 s margin) and any mismatch (`skip_changed_since`). For undo of an unmute the predicate accepts any `member`, any `left` and any `restricted` target that is not fully muted (`staff_action_decide.go:375-381`), so a later partial restriction by another admin, or a kick and rejoin, would be overwritten by the old mute. I confirmed WR-04 in the code. It is a deliberate choice recorded in research A5 and in plan 03-02's truth ("an unmute left them a member, left, or restricted but able to send"), but it contradicts the prohibition's "any difference ... skips the group", which the plan marked `resolved`. |
| 6 | Live Telegram semantics of restore and re-ban (research A2, A3) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Artifacts present and wired (`staffCallRestore` with `UseIndependentChatPermissions`, re-ban with `UntilDate`). No test can exercise Telegram's own behaviour; the fake models it. See human verification. |

**Score:** 4/6 truths verified (1 present, behavior-unverified; 1 uncertain, owner decision)

### Judgment on the orchestrator's questions

- **WR-04 against D-04:** the unmute column is a real, narrow departure from D-04's absolute wording. It does not remove or add any restriction beyond what staff itself had placed and then lifted, it needs another admin to act between the unmute and the undo, and the presser still needs live restrict rights in that group. I classify it as an accepted-limitation candidate, not a blocker, and ask the owner to confirm. The fix, if wanted, is to persist the permission set the unmute applied and compare it.
- **WR-01:** real, but D-09 ("one undo per action ... groups skipped during an undo stay as they are ... no retry undo button") covers the partial-undo case by design. The zero-effect case (no rights anywhere, transient outage, shutdown before `staffActionRunsWG.Add`) is the part D-09 did not consider. Owner decision.
- **WR-02:** real. It weakens the audit's honesty for the all-skipped and crashed-undo cases but does not touch criteria 3 or 4 as worded, because the detail view shows each group's own undo result. Owner decision. I did not count it as a gap.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `migrations/20261005120000_add_staff_actions.sql` | Audit tables, append-only, timestamp above all existing | ✓ VERIFIED | Highest filename, no `BEGIN/COMMIT`, idempotent statements. PostgreSQL chain passed (orchestrator). |
| `alita/db/models/staff_action.go` | Models match migration | ✓ VERIFIED | Columns and CHECK constraints match. |
| `alita/db/staff/actions.go` | Fresh reads, conditional claim, per-group and finalize writes | ✓ VERIFIED | Never cached, so no `DeleteCache` applies. All writes use `db.DB`. |
| `alita/db/staff/rekey.go` | Re-key `staff_actions.staff_chat_id` only | ✓ VERIFIED | Line 48-53. IN-03 (bumps `updated_at`) is noted below. |
| `alita/modules/staff_action_record.go` | Prior-state capture, record build and map-back | ✓ VERIFIED | |
| `alita/modules/staff_log.go` | Action and undo log posts | ✓ VERIFIED | |
| `alita/modules/staff_undo.go` | Ask, Confirm, run, restore call | ✓ VERIFIED | |
| `alita/modules/staff_history.go` | List and detail views | ✓ VERIFIED | |
| `alita/modules/staff_action_decide.go` | `decideStaffUndo` | ✓ VERIFIED | See Truth 5 for the unmute predicate. |
| `locales/*.yml` (7) | New keys in every locale | ✓ VERIFIED | 32 matching keys in each of the seven files; `make check-translations` passes. |
| `docs/.../commands/staff/index.md`, `AGENTS.md` | Rules and docs updated | ✓ VERIFIED | Undo, history and audit rules are present. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `staffActionConfirm` | `staff.CreateAction` | `staffCreateActionRecord` before `startStaffActionRun` | WIRED | Fails closed. |
| `runStaffActionInGroup` | `staff.SavePrior` | write-ahead before `executeStaffCall` | WIRED | |
| `startStaffActionRun` AfterGroup | `postStaffActionLog` | only for done groups, after the result is stored | WIRED | |
| `startStaffActionRun` Finish | `FinalizeAction` and `staffUndoKeyboard` | button only when finalized and ≥1 done group | WIRED | |
| `staffCallback` | `staffHistoryList/Detail`, `staffUndoAsk/Confirm`, shared Cancel | action codes `rc`, `dt`, `ya`, `yc`, `yn` | WIRED | |
| `staffUndoConfirm` | `ClaimUndo`, target lock, `startStaffUndoRun` | conditional update then shared engine | WIRED | |
| `runStaffUndoInGroup` | `staffGroupPrechecks`, `decideStaffUndo`, `executeStaffUndoCall` | presser as actor | WIRED | |
| Panel keyboard | `staffRecentButton` | Recent actions button | WIRED | `staff_panel.go:317` |
| `main.go` shutdown | `StopStaffActions` | LIFO, after DB-close registration | WIRED | Undo runs join the same wait group via `startStaffRun`. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| History list | `rows`, `tallies` | `ListActionsFresh`, `TallyActionGroups` (SQL) | Yes | ✓ FLOWING |
| History detail | `a`, `groups`, `linked` | `GetActionFresh`, `ListActionGroupsFresh`, `ListLinksByStaffFresh` | Yes | ✓ FLOWING |
| Undo run | `prior`, `applied`, `live` | stored group row, `a.UntilDate`, live `getChatMember` | Yes | ✓ FLOWING |
| Log post | issuer, target, reason | `staffActionCard` filled at Confirm from the live press | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Phase 3 test families (undo, history, log, record, stop) | `go test -tags testtools -count=1 -run '^TestStaff(Undo\|History\|ActionLog\|ActionRecord)\|^TestStopStaffActions' ./alita/modules ./alita/db/staff` | `ok` for both packages, no FAIL or SKIP lines, 70 named tests | ✓ PASS |
| Production build | `CGO_ENABLED=0 go build ./...` | exit 0 | ✓ PASS |
| Locale parity | `make check-translations` | "All translations are present" | ✓ PASS |
| Behavior-dependent invariants (exactly-once undo, shutdown finalizes undo, interrupted record) | the named tests above (`TestStaffUndoOnce`, `TestStaffUndoOnceAcrossReplicas`, `TestStopStaffActionsUndo`, `TestStopStaffActionsRecordsInterrupted`) | pass | ✓ PASS |

The orchestrator's own evidence (full `make test`, PostgreSQL migration chain, integrity check, `go vet`, `make check-docs`) is accepted as supplementary. I did not rerun the full suite, per the single-run rule.

### Probe Execution

Step 7c: SKIPPED. The phase declares no `probe-*.sh` scripts and none exist for it.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| STAFF-09 | 03-03, 03-08 | Each applied action and its reason posted to the log channel of every group where it was applied | ✓ SATISFIED (live channel check pending) | Truth 1 |
| STAFF-10 | 03-01 | Every staff action recorded with issuer, target, action, duration, reason, time, per-group outcomes | ✓ SATISFIED | Truth 2 |
| STAFF-11 | 03-02, 03-06, 03-07, 03-08, 03-09 | "Undo everywhere" with per-group live checks and the same summary | ✓ SATISFIED in code, live semantics pending | Truths 4, 5, 6 |
| SETUP-09 | 03-04, 03-05, 03-09 | `/staff` lists recent actions with who, what, target, when, reason, per-group outcome | ✓ SATISFIED | Truth 3 |

All four IDs appear in plan frontmatter and in REQUIREMENTS.md. Nothing in REQUIREMENTS.md maps to Phase 3 without a plan, so there are no orphans. REQUIREMENTS.md still shows the four boxes unchecked and the traceability rows "Pending". Update them when the phase closes.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (Phase 3 files) | - | `TBD`, `FIXME`, `XXX` markers | none found | grep over the Phase 3 source and migration returned nothing |
| `alita/modules/staff_history.go` | 295-310 | Press answered with an empty toast before the data is read (WR-03) | ⚠️ Warning | A database error on a history page press is silent. The detail handler does it correctly. |
| `alita/modules/staff_history.go` | 124, 359, 376 | `Format("2 Jan 15:04")` is English-only (IN-05) | ℹ️ Info | Month names are not localized. |
| `alita/db/staff/rekey.go` | 48-53 | Re-key bumps `updated_at` on every history row (IN-03) | ℹ️ Info | After a migration, old crashed records read "running" for up to 30 minutes. |
| `alita/modules/staff_history.go` | 197 | Prev ignores a shrunk page (IN-06) | ℹ️ Info | Entries can appear on two pages. None are hidden. |
| `alita/modules/staff_action_decide.go` | 300 | `staffCallRestore` defined by arithmetic outside the iota block (IN-01) | ℹ️ Info | |
| `alita/db/staff/actions.go` | 270-273 | `ClaimUndo` not bound to the Staff Group in SQL (IN-02) | ℹ️ Info | Callers bind it today. |
| Tests | - | Two tests sleep a fixed 50 ms (IN-04) | ℹ️ Info | Possible flake under `-race` load. |

WR-01, WR-02 and WR-04 are listed under Human Verification as decisions. All ten review findings are still `open` in `03-REVIEW-DISPOSITION.md`.

### Human Verification Required

See the `human_verification` frontmatter. In short:

1. **Live restore test** (Telegram A2 and A3) from 03-09 Task 2 and 03-VALIDATION.md.
2. **Log-channel inspection** in two real channels, checking that the Staff Group's title and ID never appear.
3. **Decision on WR-04:** accept the broad unmute "left behind" predicate, or plan a fix.
4. **Decision on WR-01 and WR-02:** accept D-09 as written for the zero-effect undo and the "undone" label, or plan a fix.
5. **`make lint`** on a go1.26 toolchain or in CI.

### Gaps Summary

There are no gaps that block the phase goal. All four roadmap criteria are met in the code and by passing behavioral tests, and every requirement ID is accounted for. What remains is evidence the codebase cannot give: live Telegram behaviour, real log channels and a lint run. Also open are three owner decisions where the implementation is weaker than the locked wording (D-04 for unmute undo, D-09 for a no-op undo, honest "undone" status). If the owner declines to accept any of them, run `/gsd-plan-phase --gaps` for that item. WR-03 and the info items can be folded into that plan or deferred.

---

_Verified: 2026-10-05T20:30:00Z_
_Verifier: Claude (gsd-verifier)_
