---
phase: "3"
slug: "staff-audit-and-undo"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-06"
---

# Phase 3 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| All replicas to Telegram's flood limits | Posts add one call per applied group to the shared budget |
| Any Telegram user to staff callbacks | Callback data can be forged and replayed; only the message's chat is trusted |
| Any Telegram user to staff callbacks | Record IDs in callback data are guessable and can be forged or replayed from another chat |
| Any Telegram user to the Undo and undo Confirm buttons | Buttons can be pressed by anyone who sees them, replayed from stale clients, or forged with guessable record IDs |
| Any Telegram user to undo buttons | Presses can come from non-members, service identities, other members, or stale clients |
| Audit record to staff readers | Recent actions, the detail view and the Undo answers are the accountability trail; they must report only what the stored outcomes support |
| Bot process to PostgreSQL | Writes can fail or be cut off by a shutdown or crash mid-run |
| History view to the undo flow | A second entry point to the same dangerous calls |
| Recorded prior state to live Telegram state | The record is old by the time undo runs; group admins may have changed the target's status since |
| Recorded prior state to live Telegram state | The restore call replaces the target's whole permission set server-side |
| Replicas sharing Redis and PostgreSQL | Concurrent Confirms on two replicas; shutdown of one replica mid-run |
| Replicas sharing Redis and PostgreSQL | Two replicas can handle two undo cards for one record at once; a rolling deploy runs old and new code together |
| Staff Group chat migration | A basic group upgraded to a supergroup gets a new chat ID; stored message IDs belong to the old chat |
| Staff Group chat migration | Stored summary message IDs belong to the old chat |
| Staff Group member to the undo Confirm | Any live member may press Undo (D-06); the claim decides whether a dangerous fan-out may run again |
| Staff Group membership to per-group authority | Being in the Staff Group grants nothing in a linked group |
| Staff Group to linked groups' log channels | Linked groups' admins read undo posts |
| Staff Group to linked groups' log channels | Linked groups' admins, who may not be staff, read the post |
| Staff member input (reason, names) to HTML posts | Free text from staff and stored names is rendered as Telegram HTML |
| Staff member to the staff action pipeline | The issuer's Confirm starts writes in other groups; the record must exist before any of them |
| Stored names and reasons to HTML text | Names and reasons are user-controlled text rendered as Telegram HTML |
| Stored names to Telegram HTML | Presser names stored in the record are spliced into HTML messages and plain callback answers |
| Stored titles, names and reasons to HTML | User-controlled text rendered as Telegram HTML |
| Undo Confirm to the shutdown drain | A claim written outside the drain can outlive the process and leave a record no run will finish |
| Undo run to the audit record | The record is the accountability trail; its undo columns and the original summary must say only what the run did |

---

## Threat Register

Built from the `<threat_model>` blocks of plans 03-01 to 03-12. The supply-chain row (T-03-SC) appears in every plan and is listed once.

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-03-01 | Repudiation | staffActionConfirm record create | high | mitigate | Record created before the run; a failed create aborts the card with no Telegram write (TestStaffActionRecordFailureFailsClosed "record create fails") | closed |
| T-03-02 | Tampering | prior state lost before a write (an unrestorable action) | high | mitigate | staffSavePrior commits before executeStaffCall; failure returns fail_internal with no write (TestStaffActionRecordsPriorState, "prior write fails in one group") | closed |
| T-03-03 | Repudiation | shutdown or crash mid-run leaving a misleading record | medium | mitigate | Coordinator finalizes after sweepPending with fail_interrupted; record writes avoid the cancelled run context; SaveGroupResult bumps updated_at as a crash heartbeat (TestStopStaffActionsRecordsInterrupted) | closed |
| T-03-04 | Information disclosure | audit rows (names, IDs, reasons) kept forever | low | mitigate | Never cached, never in backup/export/import/reset; AGENTS.md rule; only fresh queries scoped by staff_chat_id | closed |
| T-03-SC | Tampering | npm/pip/cargo installs | low | accept | No package installs; go.mod and go.sum are unchanged | closed |
| T-03-05 | Tampering | undo lifting a ban or restriction staff did not place, or overruling a later admin decision | high | mitigate | decideStaffUndo skips on any left-behind mismatch (skip_changed_since); invariants (a), (b), (c) over every combination (TestStaffUndoDecisionInvariants) | closed |
| T-03-06 | Tampering | a restore turning permanent through Telegram's 30-second rule under pacing or retries | medium | mitigate | staffUndoMinRemaining = 120 s; invariant (d); boundary rows at now+120 and now+121 | closed |
| T-03-07 | Information disclosure | Staff Group identity leaking into linked groups' log channels | medium | mitigate | composeStaffActionLog never reads card.StaffChat; TestStaffActionLogPosts scans every post for the Staff Group title and ID | closed |
| T-03-08 | Tampering | reason or names injecting HTML or overflowing a post | medium | mitigate | Reason cut to 300 runes then html.EscapeString; target via staffTargetDisplay (escaped, capped); issuer via MentionHtml (TestStaffActionLogReasonCapped) | closed |
| T-03-09 | Denial of service | log posts exhausting the shared flood budget or stalling a run | medium | mitigate | Every post through staffPaced; MaxWait refusal fails fast; failures logged only (TestStaffActionLogPaced, TestStaffActionLogRateLimitedKeepsDone) | closed |
| T-03-10 | Repudiation | a failed post hiding or falsifying an applied action | low | mitigate | Post sent after progress.set and the record write; its failure never changes the result (TestStaffActionLogFailureKeepsDone) | closed |
| T-03-11 | Information disclosure | another Staff Group's history, or history shown to non-members | medium | mitigate | Chat from query.Message only; GetStaffGroupFresh plus live IsUserInChatWithError; queries scoped by staff_chat_id (TestStaffHistoryAccess, TestStaffHistoryList) | closed |
| T-03-12 | Tampering | forged or oversized offset in callback data | low | mitigate | Strict integer parse, 0..1,000,000, else expired answer; no user text in callback data (TestStaffHistoryAccess "bad offset", TestStaffHistoryCallbackBudget) | closed |
| T-03-13 | Denial of service | HTML injection or oversize text from names and reasons breaking the message | low | mitigate | Cut then escape every field; fitStaffSummaryLines against 3800 UTF-16 units (TestStaffHistoryLengthCap) | closed |
| T-03-14 | Information disclosure | forged dt callback naming another Staff Group's record | medium | mitigate | Record loaded fresh; refused unless a.StaffChatID equals query.Message's chat; live membership check first (TestStaffHistoryDetailAccess) | closed |
| T-03-15 | Denial of service | oversize or HTML-heavy detail text | low | mitigate | Escaped, capped titles; done-line collapse then explicit "N more" note within 3800 UTF-16 units (TestStaffHistoryDetailLong) | closed |
| T-03-16 | Elevation of privilege | undo acting in a group where the presser is not an admin with restrict rights | high | mitigate | staffGroupPrechecks reads the presser's live getChatMember per group through staffIssuerSkipReason; no admin cache (TestStaffUndoAllSkipped; grep gate on admin-cache calls) | closed |
| T-03-17 | Elevation of privilege | outsider or non-presser starting or confirming an undo | high | mitigate | Ask requires live Staff Group membership and a record bound to the message's chat; Confirm uses the Lua issuer check (issuer = presser) and rechecks membership | closed |
| T-03-18 | Tampering | undo lifting a ban it did not place | high | mitigate | Every call chosen by decideStaffUndo (left-behind check) and made by executeStaffUndoCall, which switches only on the verdict | closed |
| T-03-19 | Tampering | old replica running an undo card as a ban during a rolling deploy | medium | mitigate | action=undo plus new codes ya/yc/yn; new code refuses an undo card at xc and an action card at yc (TestStaffUndoCardsDoNotCross) | closed |
| T-03-20 | Tampering | editing an unrelated message after a Staff Group migration | medium | mitigate | Original edited and replied to only when summary_chat_id equals the pressing chat (TestStaffUndoAfterRekey) | closed |
| T-03-21 | Elevation of privilege | non-member, service identity or non-presser driving an undo | high | mitigate | Live membership at Ask and Confirm; staffServiceUserIDs refusal; Lua issuer (presser) check on Confirm and Cancel (TestStaffUndoOutsider, TestStaffUndoCardPresserOnly) | closed |
| T-03-22 | Tampering | double undo from a second card, stale client or replica | high | mitigate | ClaimUndo conditional update as the last step before the run; Ask refuses claimed records (TestStaffUndoOnce, TestStaffUndoOnceAcrossReplicas) | closed |
| T-03-23 | Tampering | undo racing a staff action on the same person | medium | mitigate | Same alita:staff:lock:target:<id> lock taken at undo Confirm (TestStaffUndoTargetLock) | closed |
| T-03-24 | Repudiation | shutdown leaving a half-finished undo unrecorded | medium | mitigate | Sweep with fail_interrupted, FinalizeUndo off the cancelled context, StopStaffActions drain (TestStopStaffActionsUndo) | closed |
| T-03-25 | Tampering | restore widening or narrowing permissions through Telegram's implied-permission rule | medium | mitigate | Restore sends use_independent_chat_permissions with the recorded struct; the fake applies the implication rule without the flag, so TestStaffUndoRestores "ban over a partial restriction" fails if the flag is dropped | closed |
| T-03-26 | Elevation of privilege | undo acting where the presser lacks rights, or where the original was not applied, or in an unlinked group | high | mitigate | Per-group presser check, not-applied filter before any read, link lookup (TestStaffUndoPresserRights, TestStaffUndoNotAppliedOriginally, TestStaffUndoLinkRemoved) | closed |
| T-03-27 | Information disclosure | Staff Group identity in undo log posts | medium | mitigate | composeStaffUndoLog never reads the Staff Group chat; TestStaffUndoLogPosts scans posts for its title and ID | closed |
| T-03-28 | Elevation of privilege | detail-view Undo bypassing the undo checks | high | mitigate | The button carries only a=ya&r=<id> and is handled by staffUndoAsk, with live membership, record chat binding, undoable check, presser-only Confirm and per-group presser rights (TestStaffHistoryDetailUndo) | closed |
| T-03-29 | Tampering | Undo offered on an action that must not be undone | medium | mitigate | staffActionUndoable gates the button and is re-checked at Ask and Confirm (TestStaffHistoryDetailUndoRules) | closed |
| T-03-30 | Elevation of privilege | Releasing a claim after a write lets an action be undone twice | high | mitigate | Release only when no result has Reached; Reached is set once executeStaffUndoCall returned whatever it returned, and sweepPending marks every swept group reached (TestStaffUndoNothingChanged, TestStaffUndoPanicKeepsClaim, TestStopStaffActionsUndo "a write reached") | closed |
| T-03-31 | Tampering | ReleaseUndo clearing a claim it does not own | high | mitigate | One conditional update on id, undo_by, the exact undo_started_at and undo_finished_at IS NULL, inside the run that holds the target lock; group columns are cleared only in the same transaction after RowsAffected == 1 (TestStaffActionReleaseUndo) | closed |
| T-03-32 | Repudiation | The original summary claims "Undone" when nothing changed | medium | mitigate | The marker is written at the end of the run from the final results: "Undone by" only with at least one done group, "changed nothing" otherwise, no edit when the claim was given back (TestStaffUndoTracer, TestStaffUndoNothingChanged, TestStaffUndoClaimReleased) | closed |
| T-03-33 | Repudiation | A claim taken in the shutdown window is never finished or given back | medium | mitigate | The Confirm joins staffActionRunsWG before the claim, so StopStaffActions waits for its run, which releases or finalizes the record through db.DB (TestStaffUndoShutdownWindow); a Confirm after the cancel claims nothing (TestStaffUndoConfirmAfterStop) | closed |
| T-03-34 | Denial of service | A wait-group registration that is never given back holds every shutdown for the full 30 s | medium | mitigate | One deferred Done for every path that does not start the run, gated on joined and not runStarted; the started run owns the other Done; every undo test ends with waitRuns | closed |
| T-03-35 | Repudiation | History reports "undone" for an undo that changed nothing, or ⏳ for a dead one | medium | mitigate | One state function (staffUndoStateOf) over the stored undo outcomes and the heartbeat drives the list, the detail and the Ask and Confirm answers (TestStaffHistoryUndoStates, TestStaffHistoryDetailUndoOutcome, TestStaffUndoClaimedTexts) | closed |
| T-03-36 | Tampering | The detail view's dead-undo conversion writes to the record | medium | mitigate | The conversion changes only the rendered results; TestStaffHistoryDetailUndoOutcome re-reads the row and requires its empty undo_outcome unchanged | closed |
| T-03-37 | Tampering | HTML injection through a stored presser name in the new texts | medium | mitigate | Names are cut and escaped (staffHistoryCut, html.EscapeString(staffPlainName)) and spliced after translation through staffUserToken, as the existing undo texts do; callback answers use plain text | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-03-01 | T-03-SC | No packages were installed: `go.mod` and `go.sum` are unchanged from f06bb21 (phase 3 start) to the phase head | Plan threat models 03-01 to 03-12 | 2026-10-06 |

Related owner decisions recorded at UAT (code-review findings, not register threats): the broad "left behind" check for undoing an unmute (03-UAT test 3), keeping the undo claim whenever a write call was attempted (test 7), and keeping "changed nothing" for unconfirmed outcomes (test 8). See `03-REVIEW-DISPOSITION.md`.

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-06 | 38 | 38 | 0 | /gsd-secure-phase (orchestrator, L1 grep depth; the auditor was skipped under the threats_open 0 / plan-time register / ASVS 1 short-circuit) |

## Security Audit 2026-10-06

| Metric | Count |
|--------|-------|
| Threats found | 38 |
| Closed | 38 |
| Open | 0 |

Evidence at L1 depth:
- Every test a mitigation names exists and passed under `-race` on the phase head: 306 top-level `TestStaff*`, `TestStop*` and `TestRekeyChat*` tests in `alita/modules` and `alita/db/staff`, plus `TestActionLog*` and `TestStaffTablesStayOutOfBackup`.
- The six mitigations that name no test were checked in code:
  - T-03-04: `alita/db/staff/actions.go` uses no cache, and `TestStaffTablesStayOutOfBackup` covers backup exclusion.
  - T-03-06: `staffUndoMinRemaining = 120` in `staff_action_decide.go`, with boundary tests in `staff_action_undo_decide_test.go`.
  - T-03-17: the undo Confirm checks `card.Issuer != query.From.Id`.
  - T-03-18: `decideStaffUndo` and `staffUndoLeftBehind`.
  - T-03-34: `joinStaffRuns` with a deferred `Done` when the run never starts (`staff_undo.go:459-463`).
  - T-03-37: `html.EscapeString` on every name spliced into the new texts.
- No SUMMARY of plans 03-01 to 03-12 raised a threat flag.
- The live UAT (03-UAT tests 1, 2 and 6) passed against a real bot: restore, log posts and the undo claim behaviour.

Out of L1 scope and tracked separately: the shutdown manager's 10 s per-handler cap versus `StopStaffActions`' 30 s wait (`STATE.md` concerns).

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-06
