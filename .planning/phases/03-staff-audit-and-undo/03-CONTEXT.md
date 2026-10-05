# Phase 3: Staff Audit and Undo - Context

**Gathered:** 2026-10-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Every staff action from Phase 2 (`/ban`, `/mute`, `/kick`, `/unban`, `/unmute`, plus `/tban` and `/tmute`) is:
- posted to the log channel of each linked group where it was applied;
- recorded durably with its issuer, target, action, duration, reason, time and per-group outcomes;
- listed in a recent-actions view reached from `/staff`;
- reversible through an "Undo everywhere" button on its summary.

Undo runs the same live per-group checks, goes through the same decision table and pacer, and reports in the same ✅/⏭/❌ summary format.

Requirements: STAFF-09, STAFF-10, STAFF-11, SETUP-09.

Not in this phase:
- Posting anything into linked groups' chats. Only their log channels are written to.
- Picking a subset of groups for an action or an undo. Out of scope by design (PROJECT.md).
- Re-applying recorded bans to a group linked later (FEATURES.md calls this a differentiator; it was not raised and stays unscheduled).

</domain>

<decisions>
## Implementation Decisions

### Undo semantics
- **D-01:** Undo mirrors each action:
  - ban → unban
  - mute → unmute
  - unban → re-ban
  - unmute → re-mute

  **Kick gets no Undo button**: the person can simply rejoin, so there is nothing to reverse. Timed variants (`/tban`, `/tmute`) undo like ban and mute.
- **D-02:** **Undo restores the prior state, and never just lifts.** For each group, the record keeps the target's status *before* the staff action: member/left, restricted with its end date, or kicked with its end date. Undo puts back exactly that. Example: a permanent staff ban that upgraded an existing 1-day ban (Phase 2 D-13) goes back to the 1-day ban with its original end date when that date is still in the future, or is lifted when the date has passed. Undo never removes a restriction staff didn't add, and never adds one that wasn't there. An undone unban or unmute re-applies the restriction that was lifted, with its original end date where one is known. — **Reversibility:** one-way — the pre-action state has to be captured at action time in the audit schema. Rows written without it can't be restored correctly later, so the column set is fixed by the first migration.
- **D-03:** **Undo touches only groups where the original action was applied (✅).** Groups that were skipped or failed in the original are left alone. They show in the undo summary as "skipped: not applied originally", or are listed by the planner's choice, but they are never acted on.
- **D-04:** **Skip a group if its status changed since.** Before reversing, compare the target's live status (the same live `getChatMember` read) with what the staff action left behind. On any difference, for example a group admin already unbanned them or banned them for longer, the group is skipped with "skipped: changed since". Undo never overrules a later decision. Every reverse call still goes through `decideStaffAction`, so nothing lifts a ban by accident.
- **D-05:** **The Undo button has no time limit.** It works as long as the record exists, and records are kept forever (D-16). D-04 and the Confirm step (D-07) are what protect against stale undos.

### Undo flow and summary
- **D-06:** **Any live member of the Staff Group may press Undo**, not only the issuer (STAFF-11). Each group is then checked live for *the presser's* rights: creator, or administrator with `can_restrict_members`, from the live `getChatMember`. The admin cache is never used. Groups where the presser lacks rights are skipped with the reason. Someone outside the Staff Group can't undo anything, and an anonymous admin gets "post as yourself" (STAFF-06). The link owner is rechecked through `recheckLink`, as in Phase 2.
- **D-07:** **Undo needs a Confirm tap.** Pressing Undo shows a card like "Undo ban of Name (123) in N groups?" with Confirm and Cancel, and only the person who pressed Undo can confirm it. It follows the Phase 2 card model: a short-lived Redis card behind a token, issuer-only Confirm, and Cancel and Expired states. N is the count of groups where the original was applied.
- **D-08:** **The undo result is a new message**, posted as a reply to the original summary, with its own ⏳/✅/⏭/❌ lines, tally and Phase 2 overflow rules (done lines collapse first, and skipped and failed lines are always listed). The original summary stays as history: its Undo button is removed and it gains a line like "↩ Undone by Name, see reply". If the original can't be edited (deleted, or too old), the undo still runs and its reply is sent anyway, as a plain message if the reply target is gone. ROADMAP criterion 4's "same done, skipped or failed summary" is read as *the same format and rules*, not the same message.
- **D-09:** **One undo per action.** The Undo button is removed when an undo is confirmed. A second press from a stale client or another replica answers "already undone by Name". The move to undone is a compare-and-set, the same as the card state moves in `staff_action_card.go`. Groups skipped during an undo stay as they are, and staff fix them by hand or re-run a command. There is no "retry undo" button, and an undo can't itself be undone (run the command again instead).

### Log-channel posts
- **D-10:** **Staff actions use the existing `admin` log category** (`logchannels.CategoryAdmin`). A group that has turned admin logs off gets no staff posts. No new category column and no migration on log settings.
- **D-11:** **A post names the issuer and says "via Staff Group", but does not name the Staff Group.** It has a staff-specific hashtag, the issuer's mention, the target's name and ID, the duration ("permanent" or the length), the reason, and a "via Staff Group" marker. The Staff Group's title and chat ID never appear in a linked group's log channel. Example shape: `#STAFF_BAN · Admin: Alice · User: Name (123) · 2 days · spamming · via Staff Group`. The reason is HTML-escaped and length-capped (Pitfall A).
- **D-12:** **Posts go out as each group succeeds.** A post is sent right after the action is applied (✅) in that group, and is skipped for groups that were skipped or failed. It goes through the shared pacer (`staffPaced`) like every other staff fan-out call. **A failed log post never changes the group's ✅ line** or counts as a failed action. At most it is logged at debug or warn level, matching `actionlog`'s rule that logging never blocks the action.
- **D-13:** **Undos are logged too, under the same rules**: admin category, posted as each group's undo succeeds, presser named, "via Staff Group". The post says what was undone. Example shape: `#STAFF_UNDO · unban · Admin: Bob · User: Name (123) · undoes ban by Alice · via Staff Group`.

### Recent-actions list (`/staff`)
- **D-14:** **The `/staff` panel gets a "Recent actions" button.** It switches the same message to a history view with Prev/Next paging and a "Back" button to the links panel. The links panel itself does not grow. Any Staff Group member can open it, and it shows only actions of *this* Staff Group.
- **D-15:** **Each entry is one compact line plus a detail button.** A line looks like `🔨 Ban · Name (123) · 2d · spamming · by Alice · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone`. A button per entry opens that action's full per-group list in the same ✅/⏭/❌ format as the summary, with the undo outcome if there was one. The detail view also offers Undo when the action can still be undone (D-01, D-09), through the same Confirm flow (D-07).
- **D-16:** **10 entries per page, newest first, paged back through everything stored. Records are kept forever**, with no pruning job. Staff tables stay out of backup, export, import and reset, as the existing AGENTS.md rule says. — **Reversibility:** reversible — adding a pruning job later is a local change. It would only take away undo on older records.

### Claude's Discretion
- The audit schema: table and column layout, a parent action row with per-group outcome rows or another shape, and how undo results are stored against the original. It must survive restarts (STAFF-10), hold the pre-action state per group (D-02), and record the undo with who, when and per-group outcomes. Migration rules in AGENTS.md apply.
- When the record is written: at Confirm, then updated as groups finish, or written once at the end. A restart mid-run must still leave a truthful record. Phase 2's `StopStaffActions` marks unfinished groups "interrupted by restart", and those lines are not undoable.
- How the record links to the Telegram message (Staff chat ID + summary message ID) for D-08's reply and edit, and how this survives a Staff Group chat migration (`staff.RekeyChat`).
- How callback data names a record. It needs a short ID or token through `callbackcodec` (64-byte cap), and never user text.
- How unlinked or renamed groups show in history and detail: probably the stored title at action time, marked as no longer linked. Undo should skip groups that are no longer linked, with a reason.
- Undo on actions that ran before Phase 3 shipped: they have no record, so there is no button.
- Whether pressing Undo while the original run is still going is impossible because the button appears only on the final summary, or is refused.
- How undo interacts with the per-target fan-out lock (`alita:staff:lock:target:<id>`). It should take the same lock, so an undo and a new action on the same person never race.
- The exact locale wording for every new string, in all 7 locale files.
- The reason length cap for log posts (Pitfall A suggests about 300 characters) and how history lines truncate long names and reasons.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project scope and requirements
- `.planning/ROADMAP.md` § "Phase 3: Staff Audit and Undo": goal, requirements, and 4 success criteria.
- `.planning/REQUIREMENTS.md`: STAFF-09, STAFF-10, STAFF-11 and SETUP-09 (full text).
- `.planning/PROJECT.md`: the Key Decisions table (undo button and recent-actions list in v1, backed by an audit record; Confirm tap; every action hits all linked groups; shared multi-replica state) and Out of Scope.
- `.planning/STATE.md` § Decisions: Phase 3's "Undo everywhere" must reuse `decideStaffAction` rather than call Telegram directly. Fan-out goes through `staffPaced`. One run per target is held by the Redis lock.

### Prior phase context
- `.planning/phases/02-staff-actions-across-groups/02-CONTEXT.md`: card, summary, per-group rules, and the never-shorten and upgrade rules (D-10..D-19) that undo's restore-prior rule (D-02) builds on.
- `.planning/phases/02-staff-actions-across-groups/02-VERIFICATION.md`, `02-REVIEW-DISPOSITION.md`, `02-SECURITY.md`: what Phase 2 guarantees, open finding WR-03 (`retry_after` = 60 s edge case), and the flaky `tmute_as_a_reply` test.
- `.planning/phases/01-staff-group-links/01-CONTEXT.md`: link authority, no posts into linked groups (D-06), and Staff Group migration re-keying.

### Repo rules
- `AGENTS.md`:
  - migrations: append-only, timestamped, one transaction per file;
  - `cache.DeleteCache` on every write;
  - `callbackcodec` with its 64-byte cap and `""` on overflow;
  - the `alita:staff:*` operational keys, the staff action card and the per-target lock;
  - the StaffActions bullets (`decideStaffAction` is the only place calls are chosen; live `getChatMember` authority);
  - staff tables never in backup or export;
  - 7 locales;
  - real-fixture tests only.

  New Redis keys or rules update `AGENTS.md` in the same commit.

### Research
- `.planning/research/PITFALLS.md`: Pitfall A (escape and cap the reason, a log-channel failure is never a failed action, paced and sent after the action), Pitfall 1 (the admin cache is not a gate), Pitfall 19 (64-byte callback data), Pitfall D (behavioural tests with the hand-written bot fake).
- `.planning/research/FEATURES.md`: the audit table, undo button and "recent actions" rows, and the dependency graph (the audit table enables undo and history).
- `.planning/research/ARCHITECTURE.md`: staff fan-out design.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `alita/modules/staff_action_decide.go`: `decideStaffAction(kind, staffTargetState, newUntil)`, `staffTargetStateFrom(MergedChatMember)` and the `staffReason` and outcome types. Undo's reverse calls must be chosen here. D-02's restore-prior and D-04's "changed since" likely need new kinds or reasons added inside this table, not bypassing it.
- `alita/modules/staff_action_run.go`:
  - `startStaffActionRun`, `runStaffActionFanOut` and `runStaffActionInGroup`: the coordinator, progress and per-group pipeline that undo should run through.
  - `fetchLiveMember`, `staffIssuerSkipReason`, `executeStaffCall`, `classifyStaffFailure`.
  - `staffPaced`: the fleet-wide pacer for every call, log posts included (D-12).
  - `StopStaffActions`: restart handling.
- `alita/modules/staff_action_summary.go`: `composeStaffActionSummary` / `renderStaffActionSummaryFinal` (overflow and continuation rules), `staffResultLine`, `staffSummaryTally*`, `editStaffActionMessage`, `staffRetryAfter*`. To be reused for the undo reply (D-08) and the history detail view (D-15).
- `alita/modules/staff_action_card.go`: the card model to copy for the undo confirm card (D-07):
  - the Redis hash at `alita:staff:act:<token>`;
  - Lua create and compare-and-set transition scripts;
  - states pending, running, done, cancelled, expired, aborted;
  - `staffActionCardDoneTTL` = 1 h, which is why undo needs a DB record;
  - the per-target lock helpers.
- `alita/utils/actionlog/actionlog.go`: `Log(b, chat, category, html)` checks `logchannels.CategoryEnabled` and adds the group header. `Admin(...)` is the per-group format. Staff posts can reuse `Log` with a staff-specific body (D-11), but the send has to go through `staffPaced` (D-12), and `Log` currently calls `SendMessageWithErrorHandling` directly.
- `alita/db/logchannels/repository.go`: `CategoryAdmin` and `CategoryEnabled`. The categories are columns, so D-10 avoids a migration.
- `alita/modules/staff_panel.go`: `renderStaffPanel`, `staffPagingRow`, `staffRefreshButton`, `staffPanelTextFor` (UTF-16 length cap) and `staffPanelRebuild`. The history view (D-14) mirrors this paging and length-cap approach.
- `alita/modules/staff.go`: the `staff` callbackcodec namespace and action constants, and `staffCallback` routing. New "recent", "detail", "undo" and "undo-confirm" actions go here.
- `alita/db/staff/repository.go`: `ListLinksByStaffFresh` (fan-out order), `GetStaffGroupFresh` (Staff Group authority) and `RekeyChat` (migration). The audit repository joins this domain or sits beside it.

### Established Patterns
- Authority reads are fresh and uncached. Exactly-once effects come from conditional writes (RowsAffected == 1) or Redis compare-and-set. This applies to "undone once" (D-09).
- One coordinator goroutine is the only writer to a summary message. Edits are throttled to `staffActionEditEvery` (2.5 s) and wait out 429 `retry_after`. Each delivery part has its own budget.
- Staff commands are raw group-0 interceptors. Callbacks route through `staffCallback`. Anonymous senders are refused.

### Integration Points
- The Phase 2 Confirm path (`staffActionConfirm` → `startStaffActionRun`) is where the audit record is created and filled as groups finish, and where the pre-action status is captured from the existing live `getChatMember(group, target)` read.
- The final summary render gets an Undo button, except for kick, actions with no ✅ groups, and already-undone actions.
- The `/staff` panel keyboard gets a "Recent actions" button.
- Model added to the `AutoMigrate` list in the staff `testmain_test.go` / `internal/testdb.Run` (AGENTS.md step 4).

</code_context>

<specifics>
## Specific Ideas

- History line format the owner picked: `🔨 Ban · Name (123) · 2d · spamming · by Alice · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone`
- Log post shape: `#STAFF_BAN · Admin: Alice · User: Name (123) · 2 days · spamming · via Staff Group`, and for undo `#STAFF_UNDO · unban · Admin: Bob · User: Name (123) · undoes ban by Alice · via Staff Group`.
- The original summary is never overwritten by an undo. It gains a "↩ Undone by Name, see reply" line, and the undo result is its own reply message.

</specifics>

<deferred>
## Deferred Ideas

None: the discussion stayed within phase scope.

</deferred>

---

*Phase: 03-staff-audit-and-undo*
*Context gathered: 2026-10-05*
