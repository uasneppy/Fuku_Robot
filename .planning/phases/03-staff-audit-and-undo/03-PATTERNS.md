# Phase 3: Staff Audit and Undo - Pattern Map

**Mapped:** 2026-10-05
**Files analyzed:** 21 new/modified (plus 7 locale files, treated as one row)
**Analogs found:** 20 / 21 (every analog path verified with `git ls-files`; new files keep their intended path)

Line numbers refer to the files as read on 2026-10-05. The Phase 2 map (`../02-staff-actions-across-groups/02-PATTERNS.md`) still applies to the pieces Phase 3 does not touch (card Lua, pacer, callbackcodec use, fake client).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `migrations/20261005120000_add_staff_actions.sql` (new) | migration | CRUD | `migrations/20261004120000_add_staff_groups_and_links.sql` | exact |
| `alita/db/models/staff_action.go` (new) | model | CRUD | `alita/db/models/staff.go` | exact |
| `alita/db/staff/actions.go` (new) | repository | CRUD + conditional-claim | `alita/db/staff/repository.go` (`SetLinkHealth`, `DeleteLinkIfOwner`) | exact |
| `alita/db/staff/rekey.go` (modify) | repository | batch tx | itself, lines 33-53 | exact |
| `alita/modules/staff_action_decide.go` (modify: add `decideStaffUndo`) | utility | transform | itself, `decideStaffAction` lines 162-281 | exact (sibling) |
| `alita/modules/staff_action_record.go` (new) | service | CRUD (write-ahead) | `alita/modules/staff_action_run.go` per-group chain | role-match |
| `alita/modules/staff_action_run.go` (modify: generalize coordinator) | service | batch fan-out | itself, lines 186-401 | exact |
| `alita/modules/staff_action_summary.go` (modify: header param, markup edit/send, undo lines) | utility | transform + streaming edit | itself, lines 289-533 | exact |
| `alita/modules/staff_action_card.go` (modify: `undo_of`, header branch, Confirm hook) | store | event-driven | itself, lines 744-861 | exact |
| `alita/modules/staff_undo.go` (new: ask/confirm/original-edit) | controller | event-driven (callback) | `staffActionConfirm` in `staff_action_card.go` | role-match |
| `alita/modules/staff_history.go` (new: list, detail, paging) | controller | request-response | `alita/modules/staff_panel.go` (`renderStaffPanel`, `staffPanelRebuild`) | role-match |
| `alita/modules/staff_log.go` (new: log post, paced) | service | event-driven (fire per group) | `alita/utils/actionlog/actionlog.go` + `staffPaced` | role-match |
| `alita/utils/actionlog/actionlog.go` (modify: `Destination` seam) | utility | request-response | itself, lines 16-34 | exact |
| `alita/modules/staff.go` (modify: callback codes + switch) | controller | event-driven | itself, lines 44-55, 249-272 | exact |
| `alita/modules/staff_panel.go` (modify: "Recent actions" button) | component | request-response | itself, `staffRefreshButton` lines 355-368 | exact |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` (modify) | config | n/a | existing `staff_act_*` keys | exact |
| `alita/db/staff/testmain_test.go`, `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go` (modify AutoMigrate lists) | test | n/a | themselves | exact |
| `alita/db/staff/actions_test.go` (new) | test | CRUD | `alita/db/staff/rekey_test.go`, `repository_test.go` | role-match |
| `alita/modules/staff_undo_test.go`, `staff_history_test.go`, `staff_log_test.go`, `staff_action_record_test.go` (new) | test | n/a | `staff_action_progress_test.go` + `staff_action_fake_test.go` | role-match |
| `alita/modules/staff_action_fake_test.go`, `staff_helpers_test.go`, `staff_panel_render_test.go` (modify) | test | n/a | themselves | exact |
| `AGENTS.md` (modify) | doc | n/a | existing Staff bullets | exact |

## Pattern Assignments

### `migrations/20261005120000_add_staff_actions.sql` (migration)

**Analog:** `migrations/20261004120000_add_staff_groups_and_links.sql` (latest existing is `20261004130000_...`, so `20261005120000` sorts last).

**Style to copy** (lines 1-46): header comment explaining no FK on chat IDs, `CREATE TABLE IF NOT EXISTS`, `BIGSERIAL PRIMARY KEY`, `TIMESTAMP WITH TIME ZONE DEFAULT NOW()` for `created_at`/`updated_at`, constraints inline in `CREATE TABLE`, indexes after:
```sql
CREATE TABLE IF NOT EXISTS staff_group_links (
    id BIGSERIAL PRIMARY KEY,
    group_chat_id BIGINT NOT NULL,
    ...
    health VARCHAR(24) NOT NULL DEFAULT 'ok',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(group_chat_id),
    CONSTRAINT chk_staff_link_distinct CHECK (group_chat_id <> staff_chat_id),
    CONSTRAINT chk_staff_link_health CHECK (health IN ('ok', 'bot_missing', 'bot_not_admin', 'bot_cannot_restrict'))
);
CREATE INDEX IF NOT EXISTS idx_staff_group_links_staff_chat_id ON staff_group_links(staff_chat_id);
```
Column layout for `staff_actions` and `staff_action_groups`: copy the full skeleton from 03-RESEARCH.md Pattern 1 (it follows this style). No top-level `BEGIN`/`COMMIT`, no `CONCURRENTLY`. Do not CHECK the reason columns. Keep the parent-child FK SQL-only.

---

### `alita/db/models/staff_action.go` (model, CRUD)

**Analog:** `alita/db/models/staff.go`.

**Pattern** (lines 20-32, 37-51): GORM tags mirror the SQL, `check:` tags carried, `TableName()` returns the migration's table name, `time.Time` for `created_at`/`updated_at` (needed because `RekeyChat` writes `updated_at`):
```go
type StaffGroupLink struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	GroupChatID int64     `gorm:"column:group_chat_id;uniqueIndex;not null;check:chk_staff_link_distinct,group_chat_id <> staff_chat_id" json:"group_chat_id,omitempty"`
	StaffChatID int64     `gorm:"column:staff_chat_id;index:idx_staff_group_links_staff_chat_id;not null" json:"staff_chat_id,omitempty"`
	Health      string    `gorm:"column:health;size:24;not null;default:'ok';check:chk_staff_link_health,health IN (...)" json:"health,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}
func (StaffGroupLink) TableName() string { return "staff_group_links" }
```
Use `*time.Time` for nullable `finished_at`, `undo_started_at`, `undo_finished_at`, `applied_at`. Outcome string constants go at the top of the file next to the `StaffHealth*` constants (lines 5-15 style). Do not declare the FK in the model.

---

### `alita/db/staff/actions.go` (repository, CRUD + exactly-once claim)

**Analog:** `alita/db/staff/repository.go`.

**Imports/error style:** `log "github.com/sirupsen/logrus"`, `gorm.io/gorm`, `db`, `models`, `alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"`; every failure is `log.Errorf("[Staff] Fn: %v", err)` then `return ..., alitaerrors.Wrapf(err, "...")` (repository.go 401-402, 425-426).

**Exactly-once claim (copy for `ClaimUndo`)** (`SetLinkHealth`, lines 429-438, map update so zero values are never skipped, `RowsAffected == 1` is the claim):
```go
result := db.DB.Model(&models.StaffGroupLink{}).
	Where("id = ? AND health <> ?", id, health).
	Updates(map[string]any{"health": health, "updated_at": time.Now()})
if result.Error != nil {
	log.Errorf("[Staff] SetLinkHealth: %v", result.Error)
	return false, alitaerrors.Wrapf(result.Error, "set staff link %d health", id)
}
if result.RowsAffected != 1 {
	return false, nil
}
```
`ClaimUndo`: `Where("id = ? AND undo_started_at IS NULL", id)` with `undo_by`, `undo_by_name`, `undo_started_at`, `updated_at`.

**Transaction pattern (copy for `CreateAction`, `FinalizeAction`, `FinalizeUndo`)** (`DeleteLinkIfOwner`, lines 389-399): `db.DB.Transaction(func(tx *gorm.DB) error { ... })`, error wrapped after.

**Cache note:** the audit tables are never cached, so there is no `invalidateStaffKeys`/`cache.DeleteCache` call. Say so in the file comment so a reviewer does not look for one (`invalidateStaffKeys`, lines 29-34, is only for the group and link lookups). Reads use fresh queries (`...Fresh` naming, like `ListLinksByStaffFresh`). Use `Order("id DESC").Offset(o).Limit(10)` for history and one `GROUP BY action_id, outcome` query for tallies. Never use the DB context from `staffActionsCtx`.

---

### `alita/db/staff/rekey.go` (modify)

**Analog:** itself. Add one row to the `updates` table (lines 35-42); the loop already writes `updated_at`:
```go
updates := []struct {
	model  any
	column string
}{
	{&models.StaffGroup{}, "chat_id"},
	{&models.StaffGroupLink{}, "staff_chat_id"},
	{&models.StaffGroupLink{}, "group_chat_id"},
	// add: {&models.StaffAction{}, "staff_chat_id"},
}
```
Do NOT re-key `summary_chat_id` or `staff_action_groups.group_chat_id`. Update the doc comment (lines 14-18). Tests: `rekey_test.go` (`rekeySeed`, `TestRekeyChatMovesStaffGroupAndLinks` at line 51) extend with a `StaffAction` row. All three harness `AutoMigrate` lists must gain `&models.StaffAction{}, &models.StaffActionGroup{}` in the same task: `alita/db/staff/testmain_test.go` line 14 (`testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})`), `alita/modules/test_harness_test.go` (~361), `alita/db/testmain_test.go` (~78).

---

### `alita/modules/staff_action_decide.go` (modify: `decideStaffUndo`)

**Analog:** `decideStaffAction` in the same file (lines 162-281).

**Shape to copy:** admin/creator target check first, then a per-kind helper, verdicts built with `staffDo`/`staffSkip` (lines 152-160):
```go
func decideStaffAction(kind staffActionKind, st staffTargetState, newUntil int64) staffVerdict {
	if st.Status == gotgbot.ChatMemberStatusCreator || st.Status == gotgbot.ChatMemberStatusAdministrator {
		return staffSkip(staffReasonSkipTargetAdmin)
	}
	switch kind { ... }
	return staffSkip(staffReasonFailLookup)
}
```
**Add:**
- new `staffReason` constants (`undone_*`, `skip_not_applied`, `skip_changed_since`, `skip_restriction_ended`, `skip_no_prior_state`) in the const block at lines 39-67;
- a matching arm for EACH in `staffReasonOutcome` (lines 70-86; unlisted falls to `staffOutcomeFailed` at line 85, the Pitfall 7 trap);
- `staffCallRestore` appended after `staffCallUnmute` (lines 116-130);
- `staffUndoVerdict` embedding `staffVerdict` (do not add fields to `staffVerdict`: tests use unkeyed literals, e.g. `staff_action_decide_test.go:42`);
- `staffPriorState` and `staffPriorFromMember(m gotgbot.MergedChatMember)` beside `staffTargetStateFrom` (lines 99-111).

Full verdict matrix: 03-RESEARCH.md Pattern 3. Add `staffReasonText` arms in `staff_action_summary.go` (line 134). Test analog: `staff_action_decide_test.go` (table + invariants). Add a sibling `TestStaffUndoDecisionInvariants`; leave `TestStaffActionDecisionInvariants` untouched.

---

### `alita/modules/staff_action_run.go` (modify: generalize)

**Analog:** itself.

**Coordinator to extract into `startStaffRun(b, spec)`** (lines 186-290). Keep exactly: `staffActionRunsWG.Add(1)` before `staffActionsContext()`, `defer error_handling.RecoverFromPanic("staffActionRun", "StaffActions")`, `defer releaseStaffTargetLock(card.Target, card.Token)`, separate fan-out goroutine closing `fanOutDone`, edit ticker with `editHoldUntil`, lock-renew ticker, `sweepPending` with `staffReasonFailInterrupted` on `ctx.Err() != nil`, then `deliverStaffActionFinal`:
```go
staffActionRunsWG.Add(1)
ctx := staffActionsContext()
go func() {
	defer staffActionRunsWG.Done()
	defer error_handling.RecoverFromPanic("staffActionRun", "StaffActions")
	defer releaseStaffTargetLock(card.Target, card.Token)
	...
	results := progress.sweepPending(sweep)
	final, continuation := renderStaffActionSummaryFinal(tr, card, results)
	deliverStaffActionFinal(b, chatID, msgID, final, continuation, editHoldUntil)
```
Hooks to add: `afterGroup(i, res)` (DB `SaveGroupResult`, then log post only when `res.Outcome == staffOutcomeDone`) called right after `progress.set` in the worker closure (line 316); `onFinal(results)` called after `sweepPending` (line 276) and before delivery (`FinalizeAction`, undo button).

**Per-group chain (lines 326-401)** is a safety-ordered contract: interrupted -> staff-group guard -> `recheckLink` -> live actor rights (`fetchLiveMember` + `staffIssuerSkipReason`) -> bot/service target guards -> live target read -> decision -> write. Extract the prefix into one helper that takes the actor ID (issuer for runs, presser for undo). Insert the write-ahead prior capture between `decideStaffAction` (line 389) and `executeStaffCall` (line 396), only when `verdict.Call != staffCallNone`:
```go
prior := staffPriorFromMember(target)
if err := staff.SavePrior(card.ActionID, link.GroupChatID, prior.toRow()); err != nil {
	log.Errorf("[StaffActions] save prior state in group %d: %v", link.GroupChatID, err)
	return result(staffReasonFailInternal, "") // fail closed
}
```
Undo calls: `executeStaffUndoCall` delegates ban/unban/unmute to `executeStaffCall` and handles `staffCallRestore` through `staffPaced` (pacer closure at lines 170-174; 8 s timeout `staffActionCallTimeout`; restrict with `UseIndependentChatPermissions: true`). Regression net: `staff_action_progress_test.go`, `staff_action_lifecycle_test.go`.

---

### `alita/modules/staff_action_summary.go` (modify)

**Analog:** itself.

- `composeStaffActionSummary` (lines 289-362) builds `header := staffActionHeader(tr, card)` at line 295. Add `composeStaffSummary(tr, header string, results, final)` holding lines 296-361 and make the old function a wrapper, so the undo summary and the history detail share the overflow rules (done lines collapse, skipped/failed never dropped, continuation messages).
- `editStaffActionMessage` (lines 385-397) never sets `ReplyMarkup`, which removes buttons. Add `editStaffActionMessageWithMarkup` with the same options plus `ReplyMarkup: markup`:
```go
_, _, err := b.EditMessageTextWithContext(ctx, &gotgbot.EditMessageTextOpts{
	ChatId: chatID, MessageId: msgID, Text: text,
	ParseMode:          formatting.HTML,
	LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
})
```
- `deliverStaffActionFinal` (lines 509-533): add a `markup` parameter used on the first final message, and return the message ID where it landed (the fallback new message at lines 518-520 must call `staff.SetSummaryMessage`, Pitfall 10). Each part keeps its own `newStaffDeliverContext()` budget.
- `fitStaffSummaryLines(head, lines, tail, atLeastOne)` (line 254) and `staffSummaryLen` (line 212, UTF-16 on escaped HTML) are the length-cap tools for history lines too.

---

### `alita/modules/staff_action_card.go` (modify)

**Analog:** itself.

- `saveStaffActionCard` (line 327) / `loadStaffActionCard` (line 381): add optional hash fields `undo_of`, `undo_kind`, read as optional (like the duration fields ~425-445) so Phase 2 cards still load.
- `staffActionHeader` (summary.go line 120): branch on `card.UndoOf != 0` so `staffActionCancel`, expiry and `abortStaffActionCard` (line 610) work for undo cards untouched.
- Confirm hook: create the audit record right before the pending-summary edit at line 856, abort when it fails:
```go
if err := editStaffActionMessage(b, staffChat.Id, msgID, renderStaffActionSummary(staffTr, card, pendingResults(links))); err != nil {
	log.Warnf("[StaffActions] edit card %s into summary: %v", card.Token, err)
}
runStarted = true
startStaffActionRun(b, card, links, newUntil, staffChat.Id, msgID)
```
Abort shape (lines 795-799): `text, _ := staffTr.GetString("staff_act_abort_check_failed"); abortStaffActionCard(b, staffTr, card, staffChat.Id, msgID, text); return ext.EndGroups`. The new `ActionID` lives on the in-memory card only, not in Redis.

---

### `alita/modules/staff_undo.go` (new: `staffUndoAsk`, `staffUndoConfirm`, original edit)

**Analog:** `staffActionConfirm` (`staff_action_card.go` 744-861). Copy the order exactly:

1. `loadStaffCardForTap` -> issuer-only answer (`staff_act_card_issuer_only`, lines 760-764) -> `takeStaffTargetLock` (line 765) with the `lockHeld`/`runStarted` deferred release (lines 758-776):
```go
defer func() {
	if lockHeld && !runStarted {
		releaseStaffTargetLock(card.Target, card.Token)
	}
}()
```
2. `transitionStaffActionCard(card.Token, query.From.Id, staffCardRunning, true)` and `answerStaffCardClaim` on a lost claim (lines 778-789); `answerStaffCallback(b, query, "", false)` exactly once.
3. `staff.GetStaffGroupFresh(staffChat.Id)` and live `chat_status.IsUserInChatWithError(b, &staffChat, presser)` (lines 795-818), each failure through `abortStaffActionCard`.
4. Then `staff.ClaimUndo` as the last step before the run starts; no `links_sig` comparison.

Chat comes from `query.Message.GetChat()` (line 792), never from callback data; additionally require `record.StaffChatID == chat.Id`. Reject `staffServiceUserIDs[query.From.Id]` with `staff_post_as_yourself`. Ask: new message with `ReplyParameters{MessageId: record.SummaryMsgID, AllowSendingWithoutReply: true}` only when `record.SummaryChatID == staffChat.Id`; then `scheduleStaffActionExpiry(b, token, chatID, msgID)` (line 47). Callback codes `yc`/`yn` (confirm/cancel) are new; `ya` asks.

**Button construction (copy, check for `""`)**, from `staff_panel.go` 357-367:
```go
data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRefresh, "p": strconv.Itoa(page)})
if data == "" {
	log.Warnf("[Staff] refresh button for page %d skipped: callback data does not fit", page)
	return gotgbot.InlineKeyboardButton{}, false
}
```
Reuse `staff_btn_confirm` / `staff_btn_cancel` labels.

---

### `alita/modules/staff_history.go` (new: list, detail, paging)

**Analog:** `alita/modules/staff_panel.go`.

**Access check (copy)** `staffPanelRebuild` lines 504-531: `GetStaffGroupFresh(chat.Id)`; nil -> `staff_cb_expired`; `IsUserInChatWithError`; `!member` -> `staff_cb_members_only`; answer once BEFORE the heavy work; then `editStaffMessage(b, query.Message, text, keyboard)` (line 539):
```go
chat := query.Message.GetChat()
group, err := staff.GetStaffGroupFresh(chat.Id)
...
member, err := chat_status.IsUserInChatWithError(b, &chat, query.From.Id)
...
answerStaffCallback(b, query, "", false)
```
**Paging:** copy the `staffPagingRow` shape (lines 331-353; `add` closure, skip and log when `encodeCallbackData` returns `""`) but with an offset cursor (`a=rc&o=<offset>`), because a page that shrinks to fit the cap must not hide rows. Page size 10 (the panel's `staffPanelPageSize` is 8, do not reuse it). Length: `staffPanelMaxUTF16` (3800) and `staffDisplayTitle` (line 78, 64 runes) plus tighter per-field caps; `fitStaffSummaryLines` as the safety net. "Back" reuses `a=rf&p=0`.

---

### `alita/modules/staff_log.go` (new: log post)

**Analogs:** `alita/utils/actionlog/actionlog.go` (destination + header) and `staffPaced` (`staff_action_run.go` 170-174) for the send.

**Destination logic to extract (actionlog.go 16-34):**
```go
if b == nil || chat == nil || htmlText == "" { return }
if chat.Type == "channel" { return }
settings := logchannels.Get(chat.Id)
if settings == nil || !logchannels.CategoryEnabled(settings, category) { return }
title := html.EscapeString(chat.Title)
header := fmt.Sprintf("<b>%s</b> (<code>%d</code>)\n", title, chat.Id)
```
Split into `actionlog.Destination(chat, category) (channelID int64, header string, ok bool)`; `Log` calls it and sends exactly as before (other callers byte-identical). Category constant: `logchannels.CategoryAdmin` (`alita/db/logchannels/repository.go` line 22). The staff module builds `*gotgbot.Chat{Id: link.GroupChatID, Title: link.GroupTitle, Type: "supergroup"}`, then sends inside `staffPaced` with the closure shape from `fetchLiveMember` (run.go 407-426): `callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)`. Mention: `formatting.MentionHtml(id, name)` (actionlog.go 43). Failures are `log.Warnf`/`Debugf` only and never touch the result (matches `Log`'s "failures are swallowed" comment). Translator for the linked group: `staffChatTranslator(link.GroupChatID)` (`staff_notify.go` 24-26). Never include the Staff Group title or ID in the body (assert in test). Cap reason runes before `html.EscapeString`.

---

### `alita/modules/staff.go` (modify: callback routing)

**Analog:** itself. Add codes to the const block (lines 44-55) next to `staffActRunConfirm = "xc"` / `staffActRunCancel = "xn"`: new `ya` (undo ask), `yc`/`yn` (undo confirm/cancel), `rc` (recent list), `dt` (detail). Taken codes: `uy un ul uc ux rf pg xc xn`. Add switch cases before `default` (lines 264-268):
```go
case staffActRunConfirm:
	return m.staffActionConfirm(b, query, tr, decoded.Fields)
case staffActRunCancel:
	return m.staffActionCancel(b, query, tr, decoded.Fields)
default:
	text, _ := tr.GetString("staff_cb_expired")
	answerStaffCallback(b, query, text, false)
	return ext.EndGroups
```
Every branch answers the query exactly once.

---

### `alita/modules/staff_panel.go` (modify: "Recent actions" button)

**Analog:** `staffRefreshButton` (lines 355-368) for the button, `renderStaffPanel` (lines 307-324) for where the row goes. Add one row built the same way (`a=rc&o=0`, new label key) before the unlink rows; keep `renderStaffPanel` I/O-free. Update `panelSplitKeyboard` in `staff_panel_render_test.go` (~61-90), whose `default:` branch calls `t.Errorf` on an unknown action.

---

### Tests

**Fixtures to extend (all exist, tracked):** `staff_action_fake_test.go` (stateful fake, add full permission set on `staffFakeMember`, honour `use_independent_chat_permissions`, keep struct-valued `permissions` params), `staff_helpers_test.go` (`staffCleanup` ~263 must delete audit rows; `withStaffLocale(t)` swaps strings for `staffMarker(key)`, assert keys not literals), `withMiniredis(t)` in `antiraid_miniredis_test.go`. Repo tests: model on `alita/db/staff/rekey_test.go` (`rekeySeed`, `cleanupStaffRows`, `//go:build testtools`, package `staff`). Shutdown tests: copy `TestStopStaffActionsFinalizes` in `staff_action_progress_test.go` (restore `staffActionsCtx` in `t.Cleanup`). Use an injected clock or generous margins for undo timing (flaky `tmute_as_a_reply` is a known blocker). No mock libraries.

---

## Shared Patterns

### Fail-closed live authority
**Source:** `staff_action_run.go` 363-371, `fetchLiveMember` 407-426, `staffIssuerSkipReason` 431+.
**Apply to:** the undo per-group chain (actor = the presser). Admin cache and `IsUserAdmin` are never used.

### Exactly-once effects
**Source:** `alita/db/staff/repository.go` 411-441 (`SetLinkHealth`) and 388-409 (`DeleteLinkIfOwner`); card Lua CAS in `staff_action_card.go` 469.
**Apply to:** `ClaimUndo` (D-09), per-group result writes, finalize.

### Panic recovery on goroutines
`defer error_handling.RecoverFromPanic("<fn>", "StaffActions")` as at `staff_action_run.go` 201, 213, 315. **Apply to:** every new goroutine and timer; audit writes must not use the cancelled `staffActionsCtx`.

### Callback data
`encodeCallbackData(staffCallbackNamespace, map[string]string{...})`, refuse to ship a button when it returns `""` (`staff_panel.go` 334-341, 358-365). Only numeric IDs or tokens, never user text; add a test building every history keyboard with the largest IDs and asserting 1..64 bytes.

### Translator and user text
Translate first, splice user text after, always escaped (`staff_recheck.go` 93-99 per Phase 2 map); use literal `tr.GetString("staff_act_...")` keys; new keys in all 7 locale files (reasons need both `staffReasonOutcome` and `staffReasonText` arms).

### Logging
`log.Warnf("[StaffActions] ...: %v", err)` / `log.Errorf("[Staff] ...")`; log-post failures at warn or debug; never log `ext.EndGroups`.

### Operational keys and AGENTS.md
No new Redis key family (undo cards reuse `alita:staff:act:<token>` with `action=undo`). Update AGENTS.md in the same commits (list in 03-RESEARCH.md "AGENTS.md edits required").

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `decideStaffUndo` (inside `staff_action_decide.go`) | utility | transform | No restore-prior table exists; use 03-RESEARCH.md Pattern 3 matrix. It is a sibling of `decideStaffAction`, not a copy. |

(Also no code analog for the write-ahead prior-state capture and the `summary_chat_id` guard; both are specified in 03-RESEARCH.md Patterns 2 and Pitfall 5.)

## Metadata

**Analog search scope:** `alita/modules/staff*.go`, `alita/db/staff`, `alita/db/models/staff.go`, `alita/utils/actionlog`, `alita/db/logchannels`, `migrations/`.
**Files read:** ~14 (targeted ranges), 19 cited paths confirmed in `git ls-files`.
**Pattern extraction date:** 2026-10-05
