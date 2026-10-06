# Phase 4: Manual Lockdown - Pattern Map

**Mapped:** 2026-10-06
**Files analyzed:** 22 new or modified
**Analogs found:** 21 / 22 (all analog paths verified git-tracked; none are gitignored mirrors)

Excerpts are copied from the analog files read this session. Line numbers refer to those files as they are today. File names for new files follow 04-RESEARCH.md "Recommended Project Structure".

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `migrations/20261006120000_add_chat_lockdowns.sql` | migration | CRUD / state machine | `migrations/20261005120000_add_staff_actions.sql` (+ `20260730010000_enforce_channel_username_ownership.sql` for the partial unique index) | exact |
| `alita/db/models/lockdown.go` | model | CRUD | `alita/db/models/staff_action.go` | exact |
| `alita/db/lockdown/repository.go` | repository (fresh reads, conditional writes) | CRUD / state machine | `alita/db/staff/actions.go` (`ClaimUndo`, `ReleaseUndo`, `GetActionFresh`) | exact |
| `alita/db/lockdown/testmain_test.go` and repo tests | test | CRUD | `alita/db/staff/testmain_test.go` | exact |
| `alita/modules/lockdown.go` (commands, `LoadLockdown`, `init()` priority 238) | module / command | request-response | `alita/modules/staff.go` (`LoadStaff`) + `alita/modules/bans.go:1104-1138` (anon registration) | role-match |
| `alita/modules/lockdown_perms.go` (raw getChat / setChatPermissions) | utility | request-response | `alita/modules/staff_action_run.go` `fetchLiveMember` / `executeStaffCall` (paced, per-call timeout) | partial (raw JSON path is new) |
| `alita/modules/lockdown_guard.go` (group -7 join guard + pure `decideLockdownJoin`) | middleware / watcher | event-driven | `alita/modules/staff_watchers.go` (group -3 watchers) + `decideStaffAction` in `staff_action_decide.go` | role-match |
| `alita/modules/lockdown_worker.go` (Start/Stop worker, pacer, claims) | service (background) | batch / event-driven | `alita/modules/staff_sweeper.go` + `staffActionPacer` in `staff_action_run.go:169-184` | role-match |
| `alita/modules/chat_permissions.go` (modify `resolveUnmutePermissions`) | utility | transform | itself (lines 47-53) | exact |
| callers: `mute.go:228`, `bans.go:817`, `captcha.go:1396`, `staff_action_run.go:667` | modify | request-response | `staff_action_run.go:658-669` (already propagates errors) | exact |
| `alita/modules/staff_panel.go` (marker on row) | component (pure render) | transform | `renderStaffRow` lines 154-172, `buildStaffPanel` 449-464 | exact |
| `alita/modules/staff_action_decide.go` / `staff_action_run.go` (`LockdownBan` flag) | utility | transform | `staffTargetState` 104-125, `decideStaffBan` 207-224 | exact |
| `alita/modules/greetings.go` `joinRequestHandler` (D-25 accept alert) | handler | request-response | itself, lines 903-955 | exact |
| `alita/utils/chat_status/` exported anon-proof prompt | utility | request-response | `checkAnonAdmin` `chat_status.go:63-79` | exact |
| `main.go` (start + shutdown registration) | config | lifecycle | `main.go:206-220`, `main.go:433` | exact |
| `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go` (AutoMigrate lists) | test config | - | lines 360-363 / 79-80 | exact |
| `alita/modules/lockdown_fake_test.go` or extension of `staffActionFake` | test fake | request-response | `staff_action_fake_test.go` | exact |
| `alita/modules/lockdown_*_test.go` (integration tests) | test | request-response | `staff_action_fake_test.go` `newStaffActionEnv` 465-514 | exact |
| `locales/*.yml` (7) and `locales/config.yml` alt_names | config | - | existing `staff_*` keys | exact |
| `AGENTS.md` (group -7, Redis keys, fresh-read rule) | doc | - | existing Handlers / Data sections | exact |
| `Makefile` `test-postgres-integrity` `-run` list | config | - | `Makefile:43` | exact |
| callback namespace (none this phase) | - | - | - | n/a |

## Pattern Assignments

### `migrations/20261006120000_add_chat_lockdowns.sql` (migration)

**Analog:** `migrations/20261005120000_add_staff_actions.sql` (last existing is 20261005120000, so the new timestamp is greater).

**Header and idempotent style** (lines 1-27): explanatory comment block, no FK on chat IDs, SQL-only FK from child to parent, no top-level BEGIN/COMMIT, `IF NOT EXISTS` everywhere.
```sql
CREATE TABLE IF NOT EXISTS staff_actions (
    id BIGSERIAL PRIMARY KEY,
    ...
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_staff_action_kind CHECK (action IN ('ban', 'mute', 'kick', 'unban', 'unmute'))
);
CREATE INDEX IF NOT EXISTS idx_staff_actions_staff_chat_id_id ON staff_actions(staff_chat_id, id);
```
**Child with SQL-only FK** (lines 60-62, 85): `action_id BIGINT NOT NULL REFERENCES staff_actions(id) ON DELETE CASCADE` and `UNIQUE(action_id, group_chat_id)`. Copy as `lockdown_id ... REFERENCES chat_lockdowns(id) ON DELETE CASCADE`, `UNIQUE(lockdown_id, user_id)`.
**Free-text codes without CHECK** (line 69): "deliberately without a CHECK so a new reason code needs no migration" (use for `trigger_kind`).
**Partial unique index:** hand-write `CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_lockdowns_active ON chat_lockdowns (chat_id) WHERE state = 'active'`, following `migrations/20260730010000_enforce_channel_username_ownership.sql` (named in RESEARCH Pattern 2; read it before writing, not re-read here).
**Rules:** one transaction per file, no `CREATE INDEX CONCURRENTLY`. A new SQL file needs no change to `runner.go` / `scripts/migrate_psql.sh`.

---

### `alita/db/models/lockdown.go` (model)

**Analog:** `alita/db/models/staff_action.go`

**Constants + struct + TableName pattern** (lines 7-16, 23-56):
```go
const (
	StaffActionOutcomePending = "pending"
	...
)
type StaffAction struct {
	ID           uint   `gorm:"primaryKey;autoIncrement;..." json:"-"`
	StaffChatID  int64  `gorm:"column:staff_chat_id;...;not null" json:"staff_chat_id,omitempty"`
	Action       string `gorm:"column:action;size:8;not null;check:chk_staff_action_kind,action IN (...)" json:"action,omitempty"`
	UndoBy        *int64     `gorm:"column:undo_by" json:"undo_by,omitempty"`
	UndoStartedAt *time.Time `gorm:"column:undo_started_at" json:"undo_started_at,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at,omitempty"`
}
func (StaffAction) TableName() string { return "staff_actions" }
```
- Nullable timestamps are `*time.Time` (use for `LockedAt`, `LiftStartedAt`, `LiftedAt`).
- Child models declare **no relation** (see `StaffActionGroup`, lines 58-93). Same for `LockdownJoiner`.
- Partial unique tag form (RESEARCH Pattern 2, repo precedent `alita/db/models/channels.go:10`): `uniqueIndex:uk_chat_lockdowns_active,where:state = 'active'`.
- `TableName()` must return the migration's table names (`chat_lockdowns`, `chat_lockdown_joiners`).

---

### `alita/db/lockdown/repository.go` (repository, fresh reads, conditional state moves)

**Analog:** `alita/db/staff/actions.go`

**File header and imports** (lines 1-19): states "always read fresh ... never cached, so no cache key is read here and none needs invalidation"; writes use `db.DB` with no request context.
```go
import (
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)
```
**Fresh read returning (nil, nil) on absence** (lines 159-170):
```go
func GetActionFresh(id uint) (*models.StaffAction, error) {
	var row models.StaffAction
	err := db.DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.Errorf("[Staff] GetActionFresh: %v", err)
		return nil, alitaerrors.Wrapf(err, "get staff action %d", id)
	}
	return &row, nil
}
```
**Exactly-once conditional claim** (lines 280-298, use for `BeginLift`, joiner row claims, `ConfirmLocked`):
```go
now := time.Now().UTC().Truncate(time.Microsecond)
result := db.DB.Model(&models.StaffAction{}).
	Where("id = ? AND undo_started_at IS NULL", actionID).
	Updates(map[string]any{"undo_by": by, "undo_started_at": now, "updated_at": now})
if result.Error != nil { ...log + alitaerrors.Wrapf... }
if result.RowsAffected != 1 { return time.Time{}, false, nil }
```
Rules from this analog: updates go through `Updates(map[string]any{...})` so zero values are written; truncate to microseconds; an expected no-match is `(false, nil)`, never an error.
**Multi-statement write in a transaction** (lines 43-61 `CreateAction`, 308-339 `ReleaseUndo`): `db.DB.Transaction(func(tx *gorm.DB) error {...})`.
**Start (one active per chat):** RESEARCH Pattern 2 says `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)` and `RowsAffected == 1`. No analog in the repo for `clause.OnConflict` was read this session; assumption A9 (also treat a unique-violation as "already active") needs the Wave 0 SQLite test plus the Postgres test.
**Group-by tally for lift report** (lines 196-234 `TallyActionGroups`): copy the `Select(... COUNT(*) AS n).Group(...).Scan(&counts)` shape for joiner state counts and for `ListActiveByChatsFresh` (use `Where("chat_id IN ?", ids)`; return an empty result without a query for an empty list, as lines 197-199).

---

### `alita/db/lockdown/testmain_test.go` (test)

**Analog:** `alita/db/staff/testmain_test.go` (lines 1-15)
```go
//go:build testtools

package staff

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m, &models.StaffGroup{}, ..., &models.StaffAction{}, &models.StaffActionGroup{}))
}
```
Use `testdb.Run(m, &models.ChatLockdown{}, &models.LockdownJoiner{})`. Add the models to the other two AutoMigrate lists: `alita/db/testmain_test.go:79-80` and `alita/modules/test_harness_test.go:362-363` (both list `&models.StaffAction{}, &models.StaffActionGroup{}` today). Add the PG test name to the `-run` list at `Makefile:43` area (`test-postgres-integrity`; also confirm `./alita/db/lockdown` joins the package list there).

---

### `alita/modules/lockdown.go` (module + commands)

**Analogs:** `alita/modules/staff.go` (registration), `alita/modules/bans.go:1104-1138` (anon registration), `alita/modules/moderation.go:337-360` (adapters).

**Descriptor + registration** (`staff.go:27-38`, `395-408`):
```go
setStaffDesc = helpers.CommandDescriptor{
	Name:           "setstaff",
	RequiredChecks: []helpers.CheckFunc{helpers.RejectAnonymousSender(), helpers.RequireGroup()},
}
func LoadStaff(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(staffModule.moduleName, true)
	helpers.WrapCommand(dispatcher, setStaffDesc, staffModule.setStaff)
	...
}
func init() { RegisterLegacyModule("Staff", 236, LoadStaff) }
```
Use priority 238 and a unique name `"Lockdown"`. Do not set `Disableable`. (Existing priorities in use: AntiRaid 230, Staff 236, StaffWatchers 237, StaffActions 65, Bans 70.)

**Anon-admin wiring** (`bans.go:1108`, `1131`; `moderation.go:340-360`):
```go
helpers.WrapCommand(dispatcher, banDesc, pipelineHandler(bansModule.ban))
func init() {
	RegisterAnonymousAdminHandler("ban", anonPipelineHandler(banDesc, bansModule.ban))
}
// anonPipelineHandler re-runs desc.RequiredChecks via helpers.RunChecks after the proof
```
For `lockdown` and `unlockdown` put `requireLockdownAuthority()` in `RequiredChecks`; `anonPipelineHandler` re-runs it after the proof, which is where the live check lives. Handlers use value receivers on `moduleStruct`; return `ext.EndGroups`.

**Live authority reuse:** `staffIssuerSkipReason(member) == ""` over `fetchLiveMember` (see next section) is the creator-or-restrict test (D-11). `chat_status.FetchBotMember(b, chatID)` (`owner.go:71`) gives the tri-state bot-rights probe (D-20). `staffChatTranslator(chatID)` (`staff_notify.go:24`) for text posted outside a command; `telegramErrorDetail(err)` (`staff_action_run.go:769`) for Telegram error text.

**User text spliced after translation** (`staff.go:360-364`):
```go
text, _ := tr.GetString("staff_unset_done", i18n.TranslationParams{"count": len(removed), "groups": staffGroupsToken})
text = strings.TrimSpace(strings.Replace(text, staffGroupsToken, renderUnlinkedGroups(removed), 1))
```
Use the same token trick for reason and names; HTML-escape at render; cap the reason at 300 runes.

---

### `alita/modules/chat_permissions.go` and `resolveUnmutePermissions` callers (D-23)

**Analog:** itself, lines 47-53:
```go
func resolveUnmutePermissions(chatInfo *gotgbot.ChatFullInfo) gotgbot.ChatPermissions {
	if chatInfo != nil && chatInfo.Permissions != nil {
		return *chatInfo.Permissions
	}
	return defaultUnmutePermissions()
}
```
Change to `(gotgbot.ChatPermissions, error)`: when `chatInfo != nil && chatInfo.Id != 0`, read the active lockdown fresh and return the unmarshalled `PrePermissions`; keep the `Id != 0` guard so the parallel pure tests in `chat_permissions_test.go` (Id 0) never touch the DB. Do not reuse `MutedPermissions`/`defaultUnmutePermissions` for the snapshot (hand-rolled structs, lossy).
**Call-site template** (`staff_action_run.go:658-669`, already inside a `paced` closure that returns errors):
```go
var info *gotgbot.ChatFullInfo
if err := paced(func(callCtx context.Context) error { var err error; info, err = b.GetChatWithContext(callCtx, groupID, nil); return err }); err != nil { return err }
return paced(func(callCtx context.Context) error {
	_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, resolveUnmutePermissions(info), nil)
	return err
})
```
Update this and `mute.go:228`, `bans.go:817`, `captcha.go:1396` to take `(perms, err)` and propagate the error before any success reply.

---

### `alita/modules/lockdown_perms.go` (raw permission helpers)

**Analog (partial):** `staff_action_run.go` `fetchLiveMember` (572-591) and the `paced` wrapper (623-629).
```go
err := staffPaced(ctx, func(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
	defer cancel()
	member, err := b.GetChatMemberWithContext(callCtx, chatID, userID, nil)
	...
})
```
Copy the per-call timeout + paced-closure shape, but send `bot.RequestWithContext(ctx, "getChat", ...)` / `"setChatPermissions"` with a `json.RawMessage` `permissions` value and `use_independent_chat_permissions=true` (RESEARCH Pattern 1, gotgbot `request.go` default branch marshals verbatim). Constants for the locked set and `canonicalPermissions` are in RESEARCH "Code Examples". No existing repo code does a raw request, so this part is NEW.

---

### `alita/modules/lockdown_guard.go` (group -7 guard, pure decision)

**Analog:** `alita/modules/staff_watchers.go`

**Module struct with handler group, watcher contract** (lines 18-25):
```go
var staffWatchersModule = moduleStruct{moduleName: "StaffWatchers", handlerGroup: -3}
```
Watchers `defer error_handling.RecoverFromPanic("<fn>", "<Module>")` first, and return `ext.ContinueGroups` unless they handled the update.
**Registration for the three paths** (lines 169-189):
```go
dispatcher.AddHandlerToGroup(
	handlers.NewMessage(func(m *gotgbot.Message) bool { ... }, handler).SetAllowBot(true),
	staffWatchersModule.handlerGroup,
)
dispatcher.AddHandlerToGroup(handlers.NewChatMember(filterFn, handler), group)
```
Join filter (copy from `greetings.go:1105-1109`): `wasMember, isMember := chat_status.ExtractJoinLeftStatusChange(u); return !wasMember && isMember`. Join request: `handlers.NewChatJoinRequest(chatjoinrequest.All, fn)` (`greetings.go:1099-1101`). Leave filter: `wasMember && !isMember` (`greetings.go:1116-1118`). Service message filter: `msg.NewChatMembers != nil` (`greetings.go:1127`, `antiraid.go:932`); add `.SetAllowBot(true)`.
**Group number:** -7 (free; -10, -6, -5, -3, -2, -1 are taken, see AGENTS.md). Update the AGENTS.md group list in the same commit. Do not share -5 with `antiRaidModule.onJoin` (`antiraid.go:929-937`).
**Pure decision function:** mirror `decideStaffAction` (`staff_action_decide.go:176-203`): pure, no I/O, table-tested, inputs include tri-state live performer status. The never-lift-by-accident comment style at lines 181-185 is the tone to copy.
**Do not gate on** `claimRecentJoinProcessing` (`greetings.go:84-101` fails closed on a Redis error). Dedupe by the joiner row insert.

---

### `alita/modules/lockdown_worker.go` (background worker, pacer, lifecycle)

**Analog:** `alita/modules/staff_sweeper.go`

**Lifecycle** (lines 79-123): package-level `mu`, `cancel`, `WaitGroup`; start is idempotent; stop cancels then waits without holding the mutex.
```go
func StartStaffSweeper(b *gotgbot.Bot) {
	staffSweepMu.Lock(); defer staffSweepMu.Unlock()
	if staffSweepCancel != nil { return }
	ctx, cancel := context.WithCancel(context.Background())
	staffSweepCancel = cancel
	staffSweepWG.Add(1)
	go func() {
		defer staffSweepWG.Done()
		defer error_handling.RecoverFromPanic("staffSweeper", "Staff")
		staffSweepLoop(ctx, b)
	}()
}
```
**Per-cycle recovery + test hook** (lines 143-149): a nested `defer error_handling.RecoverFromPanic("staffSweepCycle", "Staff")` so a panicking cycle does not stop the loop; tunables (`staffSweepInterval`, `staffSweepPace`) are package vars so tests shorten them.
**Cancel-aware pause** (lines 51-66 `staffSweepPaceFunc`): select on `ctx.Done()` and a timer.
**Optional Redis guard with fail-open** (lines 161-177): `cache.GetRedisClient() == nil` returns true; on a Redis error warn and proceed. Lockdown needs no lock (per-row conditional claims) but copy this degrade-on-Redis-down stance for the pacer.

**Pacer analog:** `staff_action_run.go:169-184`
```go
var staffActionPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey: "alita:staff:pace:next", BlockKey: "alita:staff:pace:block",
	Interval: 100 * time.Millisecond, MaxRetries: 3, MaxWait: 60 * time.Second,
})
func staffPaced(ctx context.Context, call func(context.Context) error) error { return staffActionPacer.Do(ctx, call) }
```
Create a separate `lockdownPacer` with keys `alita:lockdown:pace:next` / `alita:lockdown:pace:block` as a package var. A test helper like `withFastStaffPacer` (`staff_action_fake_test.go:451-463`, `RetryAfterUnit: 5 * time.Millisecond`) installs a fast one.

**Shutdown registration** (`main.go:206-220`, DB-close is registered first at 160-163 so LIFO runs these before it):
```go
shutdownManager.RegisterHandler(func() error {
	log.Info("[Shutdown] Stopping staff sweeper...")
	modules.StopStaffSweeper()
	return nil
})
```
Add `StopLockdownWorker()` the same way after the DB-close handler, and `modules.StartStaffSweeper(b)` at `main.go:433` gets a sibling `StartLockdownWorker(b)`. Per-handler budget is 10 s, so cancel and wait about 5 s; the work is resumable.

---

### `alita/modules/staff_panel.go` (D-17 marker)

**Analog:** itself.
**Pure row render** (lines 154-172): builds title, `<code>` ID, status line from `tr.GetString("staff_panel_row_status", ...)`, then optional reason line.
```go
sb.WriteString(status)
if reason := staffRowReason(tr, row); reason != "" {
	sb.WriteString("\n")
	sb.WriteString(reason)
}
```
Add a `LockedSince time.Time` field to `staffLinkRow` and, after the status line, a line from new key `staff_panel_row_lockdown` when non-zero. Time format `"2 Jan 15:04"` UTC (as `staff_history.go:136`, per RESEARCH). Load data in `buildStaffPanel` (449-464) with one `ListActiveByChatsFresh` call after `buildStaffPanelRows`, keeping `renderStaffPanel` pure. Test with pure render tests next to `staff_panel_render_test.go`.

---

### `staffTargetState` / `decideStaffBan` (Pitfall 4, D-05)

**Analog:** `staff_action_decide.go:104-111` and `207-213`:
```go
type staffTargetState struct { Status string; IsMember bool; Muted bool; Until int64 }
case gotgbot.ChatMemberStatusKicked:
	if staffEndsLater(newUntil, st.Until) { return staffVerdict{Call: staffCallBan, Reason: staffReasonBanned} }
	return staffVerdict{Call: staffCallNone, Reason: staffReasonSkipAlreadyBanned}
```
Append `LockdownBan bool` as the LAST field (keeps any unkeyed literals compiling; check first). In the `Kicked` case return the ban verdict when `st.LockdownBan`. Set it in `runStaffActionInGroup` after `fetchLiveMember`. Add a row to the decision table test (`staff_action_decide_test.go`).

---

### `alita/modules/greetings.go` `joinRequestHandler` (D-25)

**Analog:** itself, lines 903-955. After the existing `decodeCallbackData(query.Data, "join_request")` and admin checks, when `response == "accept"` do one fresh active-lockdown read and answer an alert (no approve). Reuse `chat_status.NewPermissionResponder(b).Respond(...)` / `answerInvalidCallback` style for the alert; do not touch `ban`/`decline`.

---

### `alita/utils/chat_status/` exported anon-proof prompt

**Analog:** `chat_status.go:63-79` `checkAnonAdmin`:
```go
if admin.GetAdminSettings(chat.Id).AnonAdmin { return true, true }   // the shortcut D-12 forbids
setAnonAdminCache(chat.Id, msg)
_, err := sendAnonAdminKeyboard(b, msg, chat)
```
Add one exported function that does the last two statements unconditionally (RESEARCH Pattern 4).

---

### Test fakes and env

**Analog:** `alita/modules/staff_action_fake_test.go`
- `staffActionFake` (lines 53-80, 296-414): records every call; per-method scripted errors (`script`, `popScripted`); `banChatMember`/`unbanChatMember`/`restrictChatMember` mutate member records; `getChat` currently marshals typed `chatPerms` (383-395). Extend with `setChatPermissions` (store raw JSON; honour `use_independent_chat_permissions` via `staffParamBool`), `getChat` returning raw stored permissions, `declineChatJoinRequest`, `deleteMessage`, and kicked `getChatMember` echoing `until_date` (`staffFakeMemberJSON`).
- Env builder (lines 465-514): `withMiniredis(t)`, `withStaffLocale(t)`, `withFastStaffPacer(t)`, `newModuleTestBot(...)`, `bot.BotClient = fake`, `ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})`, then `LoadX(dispatcher)`. Clone as `lockdownEnv` loading Lockdown + Greetings + Captcha + AntiRaid.
- Cleanup via `staffCleanup(t, ids...)` style helper for lockdown rows. Assertions on observable calls (`callsTo`, `writes`, `memberLookups`).

## Shared Patterns

### Live authority (never the admin cache)
**Source:** `staff_action_run.go:572-607` (`fetchLiveMember`, `staffIssuerSkipReason`). **Apply to:** `requireLockdownAuthority`, performer exemption in the guard. Never `chat_status.IsUserAdmin` (cache plus `tgAdminList`).

### Panic recovery on every goroutine and handler
**Source:** `staff_watchers.go:33` and `staff_sweeper.go:101`: `defer error_handling.RecoverFromPanic("<name>", "<Module>")`. **Apply to:** guard handlers, worker goroutine, each worker cycle, any fire-and-forget.

### Conditional write = exactly once
**Source:** `alita/db/staff/actions.go:280-298`. **Apply to:** start, confirm lock, begin lift, joiner claims, final `lifting -> lifted` tally post.

### Fresh reads, no cache
**Source:** `alita/db/staff/actions.go:1-8`. **Apply to:** all lockdown reads; state it in AGENTS.md so nobody adds a cache without `skipLocal` and `DeleteCache`.

### Error wrapping and logging
`alitaerrors.Wrapf(err, "...")` plus `log.Errorf("[Lockdown] fn: %v", err)`; never discard a DB error on a state-changing path.

### Fleet-wide pacing
**Source:** `staff_action_run.go:169-184`. Separate pacer keys for lockdown; never a per-replica limiter.

### Locale keys
Add every new key to all 7 files in `locales/` (copy names from `locales/en.yml`), add `Lockdown` alt names to `locales/config.yml`, then `make check-translations`, `make generate-docs`, `make check-docs`.

## No Analog Found

| File / part | Role | Data Flow | Reason |
|-------------|------|-----------|--------|
| raw `getChat`/`setChatPermissions` JSON calls in `lockdown_perms.go` | utility | request-response | No code path sends a `json.RawMessage` through `bot.RequestWithContext`; follow RESEARCH Pattern 1 and the gotgbot `request.go` default branch |
| `clause.OnConflict{DoNothing: true}` start insert | repository | CRUD | No in-repo usage read this session; RESEARCH A9 needs SQLite and Postgres tests |
| DB-queue worker that claims rows (`pending -> banning -> banned`) | service | batch | Staff code has a sweeper and per-run workers, but no row-claim queue; compose from `ClaimUndo` conditional updates and the sweeper lifecycle |
| ChatMember-kicked suppression of `greetings.leftMember` | watcher | event-driven | New behaviour; only the join/leave filters at `greetings.go:1105-1121` are reusable |

## Metadata

**Analog search scope:** `alita/db/staff`, `alita/db/models`, `alita/modules` (staff*, greetings, antiraid, bans, moderation, chat_permissions, fake and harness tests), `alita/utils/chat_status`, `migrations`, `main.go`, `Makefile`.
**Files read:** about 20. Tracked check: `git ls-files` confirmed tracked status for the six core analog files probed (`staff_action_fake_test.go`, `staff_sweeper.go`, `db/staff/actions.go`, the staff actions migration, `telegram_pacer.go`, `main.go`); the rest are ordinary source under tracked directories, none under `.gsd/capabilities`.
**Pattern extraction date:** 2026-10-06
