---
status: resolved
trigger: "UAT gap G-03-4 (phase 03, test 4): undo claim spent before any group is attempted (WR-01); history and original summary say 'undone' regardless of effect (WR-02); crashed undo shows pending forever"
created: 2026-10-06T00:00:00Z
updated: 2026-10-06T13:30:00Z
goal: find_root_cause_only
---

## Current Focus

hypothesis: CONFIRMED (all three parts reproduced with real fixtures, see Evidence T1-T4)
bug_class: Bohrbug (deterministic design gap; reproduces on every run with the given inputs)
test: done; scratch test alita/modules/zz_scratch_g034_test.go was run and deleted (worktree has no source changes)
expecting: n/a
next_action: none; resolved by 03-10..03-12 and confirmed in UAT test 6.

reasoning_checkpoint:
  hypothesis: |
    (1) staffUndoConfirm treats "claimed" as "undone": it takes the D-09 latch (ClaimUndo sets undo_started_at)
    and marks the original summary "Undone by X" before any group is visited, and no later step (Finish /
    FinalizeUndo) looks at the per-group results to release the latch or pick the label; the repository has no
    statement that can ever clear undo_started_at. (2) Every "undone" label is derived from UndoStartedAt != nil
    alone; the list path doesn't even load undo outcomes (TallyActionGroups counts only `outcome`). (3) The
    detail view's dead-run conversion is keyed on the action's FinishedAt and rewrites only the action's rows,
    so undo rows with undo_outcome '' render as pending (⏳) forever.
  confirming_evidence:
    - "T1: presser admin nowhere -> 0 writes, both rows skip_issuer_not_admin, undo_started_at kept; Carol (real admin) Ask answered 'staff_undo_already Bob'; staffHistoryState='staff_history_undone'; original edited with staff_undo_marker at call #19, before the first linked-group getChatMember at call #24"
    - "T2: Confirm after StopStaffActions -> 0 getChatMember and 0 unban calls in linked groups, every row fail_interrupted, claim kept, original marked undone"
    - "T3: StopStaffActions returned after 82us while staffUndoConfirm had already written the claim and was still inside editStaffUndoneOriginal (staffActionRunsWG.Add not reached yet)"
    - "T4: undo_started_at 2h old, undo_finished_at NULL, updated_at 2h old (> staffTargetLockTTL 30m): state 'undone', detail undo block = '⏳ Group A' / '⏳ Group B'"
  falsification_test: "A zero-write undo leaving undo_started_at NULL, or a 2h-dead undo rendering fail_interrupted lines, or the original summary edit arriving after the first group lookup. None observed."
  fix_rationale: "Move the latch/label decision to the point where per-group results are known (run Finish/Delivered), driven by an explicit per-group 'reached the write call' fact; derive labels from undo outcomes, not from the claim; apply a heartbeat-based dead-undo conversion like the action's."
  blind_spots: |
    - Live Telegram behaviour of an interrupted or timed-out write (did it land?) is not observable with fakes;
      the fix must treat any group that reached executeStaffUndoCall as "written" whatever the error.
    - The webhook-mode shutdown window (T3) was shown with the test dispatcher; in production it needs a Confirm
      handler that outlives httpServer.Stop's 10 s dispatch drain (polling mode's Dispatcher.Stop closes it).
  candidate_causes:
    - "code: claim taken before any effect and never released (staff_undo.go 394-416, actions.go ClaimUndo)"
    - "code: labels keyed on UndoStartedAt only; list tally lacks undo counts (staff_history.go 69-80, actions.go TallyActionGroups)"
    - "code: dead-run conversion only for the action, not the undo (staff_history.go 338-349, staff_action_record.go 208-226)"
    - "environment: shutdown ordering; claim-to-WG.Add window, and the shutdown manager's 10 s per-handler cap (alita/utils/shutdown/graceful.go:83) is shorter than StopStaffActions' 30 s wait, so a graceful shutdown can still close the DB before FinalizeUndo"
    - "data: record has no per-group 'write attempted' marker; fail_interrupted / fail_rate_limited / fail_group_not_found are written both before and after a write call, so the stored reason alone can't tell whether a write was reached"
  and_gate: |
    Partly yes. Symptom 1 (claim kept) needs only the code cause (unconditional latch). Symptom 3 (⏳ forever)
    needs the display cause AND an environment condition that leaves undo_finished_at NULL (hard crash, DB
    closed before FinalizeUndo by the 10 s handler cap, or the claim-to-WG.Add window). The environment conditions
    are triggers, not the defect: the record can always be left unfinished by a hard crash, so the display must
    handle it regardless.

## Symptoms

expected: |
  1. If no linked group reached a Telegram write during an undo run (every group skipped, or every group failed
     before any write: shutdown, rate limit, Redis or Telegram outage, Confirm after StopStaffActions), the
     one-undo-per-action claim (staff_actions.undo_started_at, set by ClaimUndo) is released.
  2. The /staff history list line, the detail view and the original summary's "Undone by ..." edit say "undone"
     only when at least one group's undo succeeded; otherwise they say the undo changed nothing.
  3. An undo that crashed partway (undo_started_at set, undo_finished_at NULL) shows unfinished groups as
     interrupted, not pending forever.
actual: |
  The claim is consumed at Confirm before any group is attempted and kept whatever the run does; history shows
  "undone" for any claimed undo; the original summary is edited to "Undone by X, see the reply" before the first
  group runs; a crashed undo renders pending lines forever.
errors: none reported; found by code review 03-REVIEW.md (WR-01, WR-02), confirmed as requirement by owner in UAT test 4 (option b).
reproduction: Test 4 in .planning/phases/03-staff-audit-and-undo/03-UAT.md
started: discovered during UAT, after phase 3 execution (design gap, present since the undo flow was written)

## Eliminated

- hypothesis: The claim survives only because FinalizeUndo fails (e.g. DB closed at shutdown).
  evidence: T1 and T2 finalized normally (undo_finished_at set) and the claim was still kept; nothing in the
    code path can clear it. The latch is unconditional by design.
  timestamp: 2026-10-06T09:18:00Z

- hypothesis: The claim-to-run window (claim written before staffActionRunsWG.Add) is hit on every graceful shutdown.
  evidence: |
    Partly eliminated. Polling mode: shutdown LIFO runs updater.Stop first, and gotgbot's Updater.Stop calls
    Dispatcher.Stop, which waits on the dispatcher waitGroup for in-flight handlers (ext/dispatcher.go:213-214)
    before StopStaffActions runs. Webhook mode: httpServer.Stop (alita/utils/httpserver/server.go:419-452)
    closes the dispatch queue and waits for dispatchWG within a 10 s budget. So the window (T3) is reachable
    only when a Confirm handler outlives those drains, or on a hard crash. It is a contributing trigger for the
    "crashed undo" state, not the main path for symptom 1.
  timestamp: 2026-10-06T09:25:00Z

## Evidence

- timestamp: 2026-10-06T00:05:00Z
  checked: knowledge base (.planning/debug/knowledge-base.md) and MemPalace
  found: No .planning/debug directory existed before this session, so there is no knowledge base and no prior match.
  implication: No known-pattern candidate; investigate from scratch.

- timestamp: 2026-10-06T00:10:00Z
  checked: alita/modules/staff_undo.go staffUndoConfirm (lines 394-430)
  found: |
    Order at Confirm: claim via staffClaimUndo -> staff.ClaimUndo (line 399) -> editStaffUndoneOriginal (line 416,
    a Telegram edit of the original summary to "Undone by X, see the reply" and removal of its Undo button) ->
    edit undo card into the pending summary (line 424) -> runStarted=true -> startStaffUndoRun (line 428).
    No group has been visited when the claim is taken or the original is marked.
  implication: The claim and the "Undone" marker are both committed before any evidence of effect exists.

- timestamp: 2026-10-06T00:12:00Z
  checked: alita/db/staff/actions.go ClaimUndo (270-285) and FinalizeUndo (329-358); grep for any writer that
    clears undo_started_at
  found: |
    ClaimUndo: UPDATE staff_actions SET undo_by, undo_by_name, undo_started_at=now WHERE id=? AND
    undo_started_at IS NULL. FinalizeUndo writes per-group undo_outcome/reason/detail, marks not-applied rows
    skip_not_applied, sets undo_finished_at. No function anywhere sets undo_started_at back to NULL.
  implication: The claim is a one-way latch; nothing in the repository can release it, whatever the results.

- timestamp: 2026-10-06T00:14:00Z
  checked: staff_undo.go startStaffUndoRun Finish (540-559)
  found: Finish converts results to rows and calls FinalizeUndo (one retry); it never inspects the results to
    decide whether anything was written, and it returns an empty keyboard. Nothing edits the original summary at
    the end of the run (there is no Delivered hook on the undo spec).
  implication: The only hook that sees the final per-group results makes no claim or label decision.

- timestamp: 2026-10-06T00:16:00Z
  checked: staffActionUndoable (staff_undo.go 67-82), staffUndoAsk (206-209), staffUndoConfirm (380-383)
  found: All three refuse once UndoStartedAt != nil ("Already undone by <name>." from staffUndoAlreadyText).
  implication: A kept claim blocks every later presser, including one with real rights, permanently (D-05 no
    time limit, D-16 records kept forever). The Ask/Confirm "Already undone by" text is a fourth surface that
    says "undone" regardless of effect.

- timestamp: 2026-10-06T00:18:00Z
  checked: staff_history.go staffHistoryState (69-80), staffHistoryLine (125-128), renderStaffHistory, and
    staff.TallyActionGroups / ActionTally (actions.go 172-228)
  found: |
    staffHistoryState's first case is `a.UndoStartedAt != nil` -> "staff_history_undone"; it receives only the
    record and now, no group data. ActionTally counts only the `outcome` column (Done/Skipped/Failed/Pending);
    TallyActionGroups never reads undo_outcome. So the list path has no data from which to know whether any
    group's undo succeeded, or whether the undo is still running or dead.
  implication: The list line cannot be made accurate by changing staffHistoryState alone; the tally query must
    also carry undo counts (or a per-record undo summary).

- timestamp: 2026-10-06T00:20:00Z
  checked: staff_history.go renderStaffHistoryDetail (338-380) and staff_action_record.go
    staffUndoResultsFromRecord (208-226)
  found: |
    Dead-run conversion (pending -> failed fail_interrupted) is gated on `a.FinishedAt == nil` and only rewrites
    `results` (the action's own rows). An undo is only possible on a finished record (FinishedAt != nil), so this
    branch never applies to an undo, and `undoResults` are never converted. staffUndoResultsFromRecord maps
    undo_outcome '' (no result written) to staffOutcomePending -> rendered ⏳. The header gets
    staffHistoryState ("undone") plus "staff_history_undone_by".
  implication: A crashed undo (undo_started_at set, undo_finished_at NULL, rows with undo_outcome '') renders as
    "undone" with ⏳ lines forever.

- timestamp: 2026-10-06T00:22:00Z
  checked: staff_action_run.go startStaffRun (226-333), StopStaffActions (70-90), staffGroupPrechecks (431-495),
    runStaffUndoInGroup (staff_undo.go 570-615), classifyStaffFailure / classifyStaffLookupFailure
  found: |
    - staffActionRunsWG.Add(1) happens inside startStaffRun, i.e. after the claim and after two Telegram edits in
      staffUndoConfirm. StopStaffActions cancels the ctx then Waits on the WG; a Confirm that has claimed but not
      yet reached Add is invisible to it.
    - staffActionsContext() is never checked before the claim; a cancelled ctx makes every group return
      fail_interrupted from staffGroupPrechecks' first line with no Telegram call at all.
    - Reasons that mean "no write call was made": every skip_* reason (incl. skip_issuer_not_admin,
      skip_issuer_no_right, skip_changed_since, skip_link_removed, skip_no_prior_state, skip_restriction_ended),
      fail_owner_unknown, fail_lookup. Reasons that are AMBIGUOUS in the stored record: fail_interrupted,
      fail_rate_limited and fail_group_not_found are produced both before the write (prechecks / lookup
      classification) and by the write itself (classifyStaffFailure). fail_telegram, fail_bot_not_admin,
      fail_bot_no_rights come only from a write.
    - No code writes undo_outcome='pending' (grep), although the CHECK constraint allows it, so the record holds
      no per-group "write attempted" marker.
  implication: Whether a group "reached a Telegram write" cannot be derived reliably from (outcome, reason) as
    stored today; the run must carry that fact explicitly (in-memory per result, and/or a persisted marker).

- timestamp: 2026-10-06T00:24:00Z
  checked: existing tests TestStaffUndoAllSkipped (staff_undo_test.go 453-488), TestStopStaffActionsUndo
    (staff_undo_lifecycle_test.go 555-614), TestStaffHistoryDetailUndoOutcome (staff_history_test.go 731-794),
    TestStaffUndoTracer (107-206), TestStaffUndoOriginalEditFails (378-402), TestStaffUndoOnce (347-406)
  found: |
    AllSkipped asserts the skip rows and undo_finished_at but nothing about undo_started_at or a later presser.
    StopStaffActionsUndo asserts fail_interrupted rows and undo_finished_at, not the claim. DetailUndoOutcome
    asserts "⏳ Group B" for an unfinished undo but never ages updated_at. Tracer asserts the original gains
    staff_undo_marker (true in its all-done case). OriginalEditFails scripts "the first edit after the claim is
    the original summary's", which depends on the edit happening at Confirm. UndoOnce (Bob's undo wrote in
    every group, Carol refused) stays valid under owner decision b.
  implication: No existing test pins the zero-effect claim, the label accuracy or the dead-undo display; the
    gap went through because the behaviour was never specified (owner decision came at UAT). AllSkipped,
    StopStaffActionsUndo, OriginalEditFails and Tracer need their assertions revised by the fix.

- timestamp: 2026-10-06T09:18:00Z
  checked: Scratch test (alita/modules/zz_scratch_g034_test.go, real fixtures newStaffActionEnv: SQLite +
    miniredis + fake BotClient), `go test -tags testtools -race -count=1 -run TestScratchG034 ./alita/modules/`
  found: |
    All four PASS (= bug observed):
    T1 ZeroEffectUndoKeepsClaim: Bob plain member in both groups. 0 unban calls. Rows done/skipped
       skip_issuer_not_admin. undo_started_at and undo_finished_at both set. Carol (admin + restrict everywhere)
       Ask -> "@@staff-undo-already@@ Bob" alert. staffHistoryState -> "@@staff-history-undone@@". Original
       summary text now "@@staff-undo-marker@@ Bob" with no button. Call log: original marker edit at index 19,
       first getChatMember of Bob in a linked group at index 24.
    T2 ConfirmAfterStop: 3 groups, StopStaffActions() then Confirm. 0 presser lookups and 0 unbans in linked
       groups; all 3 rows failed/fail_interrupted; claim kept; original marked undone.
    T3 ShutdownWindow: editMessageText delayed 500 ms in the Staff chat to hold Confirm inside
       editStaffUndoneOriginal; once undo_started_at was written, StopStaffActions() returned after 82.5 us with
       the Confirm handler still running. The run then started on a cancelled ctx: both rows fail_interrupted,
       0 unbans, claim kept. (In production the DB-close handler follows StopStaffActions.)
    T4 CrashedUndoPendingForever: undo_started_at=now-2h, undo_finished_at NULL, updated_at=now-2h (TTL 30m),
       FinishedAt set. State "undone"; detail undo block "⏳ Group A" / "⏳ Group B"; no fail_interrupted.
  implication: All three expected behaviours are violated exactly as reported; root cause confirmed by direct
    observation. Scratch file deleted afterwards; `git status` shows only .planning/debug/.

- timestamp: 2026-10-06T09:25:00Z
  checked: main.go shutdown registration (158-293), alita/utils/shutdown/graceful.go (67-109), gotgbot
    ext/updater.go Stop (244-263) and ext/dispatcher.go Stop (213-217), alita/utils/httpserver/server.go Stop
  found: |
    LIFO order: updater.Stop (polling) or httpServer.Stop (webhook) -> DrainAISpamChecks -> StopStaffActions ->
    StopStaffSweeper -> ... -> closeDBConnections. Polling: Dispatcher.Stop waits for in-flight handlers.
    Webhook: dispatchWG drained within a 10 s budget. The shutdown manager gives EVERY handler a 10 s timeout
    (graceful.go:83, "Handler %d timeout, skipping"), shorter than StopStaffActions' staffActionStopWait (30 s),
    so a run still winding down after 10 s keeps going while the manager proceeds towards closing the DB.
  implication: A graceful shutdown can still leave a claimed undo with undo_finished_at NULL (FinalizeUndo fails
    against a closed DB), which is the crashed-undo state of symptom 3. Contributing trigger; the 10 s vs 30 s
    mismatch is outside this gap (it also affects action runs) and is worth its own note.

## Resolution

root_cause: |
  The undo flow conflates "an undo was claimed" with "the action was undone", in three places.
  (1) Claim: staffUndoConfirm (alita/modules/staff_undo.go:394-416) takes the D-09 latch through
  staff.ClaimUndo (alita/db/staff/actions.go:270-285) and edits the original summary to "Undone by X" BEFORE any
  group is visited. The run's Finish (staff_undo.go:540-559) only calls FinalizeUndo and never looks at the
  per-group results. No repository function can clear undo_started_at, so the latch is permanent whatever the
  run did, even with zero Telegram writes. The run also doesn't record whether a group reached its write call,
  and the stored reasons are ambiguous (fail_interrupted, fail_rate_limited and fail_group_not_found occur both
  before and after a write). Contributing: the claim is taken without checking staffActionsContext().Err(), and
  outside staffActionRunsWG (Add happens later, in startStaffRun).
  (2) Labels: staffHistoryState (staff_history.go:69-80) returns "undone" whenever UndoStartedAt != nil. The list
  path can't do better because staff.TallyActionGroups/ActionTally (actions.go:172-228) count only `outcome`,
  never `undo_outcome`. The detail header (staff_history.go:360-378), the original-summary marker
  (editStaffUndoneOriginal, staff_undo.go:437-455, called at Confirm) and the Ask/Confirm "Already undone by"
  text (staffUndoAlreadyText) are all keyed off the claim, not off any group's undo outcome.
  (3) Crashed undo: renderStaffHistoryDetail's dead-run conversion (staff_history.go:338-349) is gated on the
  action's FinishedAt == nil and rewrites only the action's own rows. An undo always has FinishedAt set, so undo
  rows with undo_outcome '' are mapped to pending by staffUndoResultsFromRecord (staff_action_record.go:208-226)
  and rendered ⏳ forever.
fix: |
  Applied through phase 03 gap-closure plans 03-10, 03-11 and 03-12 (gap G-03-4), per the user's UAT decision (test 4 "b"):
  (1) staffGroupResult.Reached marks each group whose undo Telegram call returned; staff.ReleaseUndo (one conditional
  update scoped to the claim's undo_by and undo_started_at) gives the claim back when no group reached its write, and the
  claim is kept once any group did. The undo Confirm joins staffActionRunsWG (joinStaffRuns) before it claims and claims
  nothing once StopStaffActions has cancelled the run context (staff_undo_abort_restarting).
  (2) TallyActionGroups counts undo outcomes (UndoDone); staffUndoStateOf derives running / interrupted / undone /
  changed nothing from the stored undo outcomes, used by the Recent actions line, the detail view and Ask/Confirm.
  The original summary is edited at the end of the run: "Undone by" only when at least one group was undone,
  "changed nothing" when writes were tried and none succeeded, left alone with its Undo button when the claim was given back.
  (3) An unfinished undo whose heartbeat is older than staffTargetLockTTL shows as interrupted, its pending groups
  rendered as interrupted without writing anything.
verification: |
  Unit and lifecycle tests in staff_undo_claim_test.go, staff_undo_lifecycle_test.go, staff_undo_shutdown_test.go,
  staff_history_undo_test.go and actions_test.go (make test green after each plan). Confirmed live by the user in
  03-UAT.md test 6 (re-run of test 4 after gap closure), passed 2026-10-06.
files_changed:
  - alita/db/staff/actions.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_history.go
  - alita/modules/staff_undo.go
  - locales/*.yml (all 7)
