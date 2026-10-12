# Phase 5: Lockdown Alerts and Response - Pattern Map

**Mapped:** 2026-10-11
**Files analyzed:** 17 new/modified (plus locale files and tests)
**Analogs found:** 16 / 17 (one partial: Redis zset tracker)

All analog paths below were checked as git-tracked source (no `.gsd` mirror paths). Line numbers are from the current tree.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `migrations/20261011120000_add_lockdown_alerts.sql` | migration | CRUD | `migrations/20261006120000_add_chat_lockdowns.sql` | exact |
| `alita/db/models/lockdown_alert.go` | model | CRUD | `alita/db/models/lockdown.go` | exact |
| `alita/db/lockdown/alerts.go` | repository | CRUD, conditional claim | `alita/db/lockdown/repository.go` (ConfirmLocked/BeginLift) + `joiners.go` (RecordJoin/ClaimJoiner) | exact |
| `alita/db/lockdown/burst.go` | repository | CRUD, batch | `alita/db/lockdown/joiners.go` | exact |
| `alita/modules/lockdown_alert.go` | service/worker | event-driven (DB-driven worker) | `alita/modules/lockdown_worker.go` + `staff_log.go` | role-match |
| `alita/modules/lockdown_alert_callbacks.go` | controller (callback) | request-response | `alita/modules/staff.go` `staffCallback` + `lockdown.go` `requireLockdownAuthority` | role-match |
| `alita/modules/lockdown_response.go` | service/worker | batch, event-driven | `lockdown_worker.go` `lockdownBanOne` + `decideLockdownJoin` | exact |
| `alita/modules/lockdown_recent.go` | utility (Redis tracker) | streaming (write per join) | `alita/modules/antiraid.go` `trackJoin` | partial |
| `alita/modules/lockdown.go` (modify) | controller | request-response | itself, `unlockdown` lines 424-516 | exact |
| `alita/modules/lockdown_worker.go` (modify) | worker | event-driven | itself, `runLockdownCycle` 262-272 | exact |
| `alita/modules/lockdown_guard.go` (modify) | middleware/watcher | event-driven | itself, `lockdownOnJoinMember` 100-160 | exact |
| `alita/modules/lockdown_status.go` (modify) | controller | request-response | itself, lines 75-76 | exact |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` | config | transform | existing `lockdown_*` keys | exact |
| `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`, `alita/db/lockdown/testmain_test.go` (modify) | test | CRUD | themselves (AutoMigrate lists) | exact |
| `alita/modules/lockdown_fake_test.go` (modify) | test | request-response | itself (`lockdownFake`, `lockdownEnv`, `pressJoinRequest`, `ageJoiner`, `lockdownCleanup`) | exact |
| `alita/modules/lockdown_alert_*_test.go` (new) | test | request-response | `lockdown_lift_test.go`, `lockdown_requests_test.go`, `lockdown_staff_test.go` | role-match |
| `alita/db/lockdown/alerts_test.go`, `burst_test.go` (new) | test | CRUD | `alita/db/lockdown/joiners_test.go` | exact |
| `AGENTS.md` (modify) | docs | n/a | existing Lockdown bullets | exact |

## Pattern Assignments

### `migrations/20261011120000_add_lockdown_alerts.sql` (migration)

**Analog:** `migrations/20261006120000_add_chat_lockdowns.sql`

Name sorts after `20261006120000` (latest). Header comment style, lines 1-22: explain no FK on chat IDs, never cached, never in backup/export, "Every statement is idempotent; the Go runner applies each file once inside a transaction". No `BEGIN`/`COMMIT`, no `CONCURRENTLY`.

**Table pattern** (lines 26-58 and 60-95):
```sql
CREATE TABLE IF NOT EXISTS chat_lockdown_joiners (
    id BIGSERIAL PRIMARY KEY,
    lockdown_id BIGINT NOT NULL REFERENCES chat_lockdowns(id) ON DELETE CASCADE,
    ...
    claimed_at TIMESTAMP WITH TIME ZONE,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_chat_lockdown_joiners_user UNIQUE (lockdown_id, user_id),
    CONSTRAINT chk_chat_lockdown_joiner_state CHECK (state IN ('pending', ...))
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_lockdowns_active ON chat_lockdowns (chat_id) WHERE state = 'active';
CREATE INDEX IF NOT EXISTS idx_chat_lockdown_joiners_lockdown_state ON chat_lockdown_joiners (lockdown_id, state);
```
Use for: `chat_lockdown_alerts` (`UNIQUE (lockdown_id, kind, seq, dest)`), `chat_lockdown_burst` (`UNIQUE (lockdown_id, user_id)`), `chat_lockdown_ban_runs` (partial unique `(lockdown_id) WHERE state = 'running'`, same idiom as `uk_chat_lockdowns_active`). `ALTER TABLE chat_lockdowns ADD COLUMN IF NOT EXISTS ...` for `alert_started_at`, `chat_title`, `link_revoked_*`. Do not touch the joiner state CHECK (research confirms no new state is needed; `kept` + detail `response_ban`). Keep `scripts/migrate_psql.sh` and `runner.go` unchanged unless the runner changes.

---

### `alita/db/models/lockdown_alert.go` (model)

**Analog:** `alita/db/models/lockdown.go`

**Model pattern** (lines 39-70, 62-63, 79-86): gorm tags carry column, size, index and `check:` mirroring SQL; `TableName()` returns the migration table; state constants in a `const (...)` block with doc comments; parent link is SQL-only (no Go relation); `ChatLockdown` gains the new columns as `*time.Time` for nullable stamps.
```go
// TableName returns the migration's table name for ChatLockdown.
func (ChatLockdown) TableName() string {
	return "chat_lockdowns"
}
```
Add `ChatLockdownAlert`, `ChatLockdownBurst`, `ChatLockdownBanRun` the same way with `uniqueIndex:uk_...,priority:N` tags matching the SQL names. Add constants such as `JoinerDetailResponseBan = "response_ban"` next to `JoinerDetailBanExpired` (lockdown.go ~line 52).

---

### `alita/db/lockdown/alerts.go` (repository, conditional claims)

**Analog:** `alita/db/lockdown/repository.go` and `joiners.go`

**Package doc + imports** (repository.go lines 1-24): header comment says rows are read fresh, never cached, so no `DeleteCache` applies; writes use `db.DB` with no request context.
```go
import (
	"errors"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	alitaerrors "github.com/divkix/Alita_Robot/alita/utils/errors"
)
```

**Exactly-once gate** (copy `ConfirmLocked`, repository.go 193-203, for `BeginAlert`; wrap in `db.DB.Transaction` to also insert slots):
```go
func ConfirmLocked(id uint) (bool, error) {
	stamp := now()
	result := db.DB.Model(&models.ChatLockdown{}).
		Where("id = ? AND state = ? AND locked_at IS NULL", id, models.LockdownStateActive).
		Updates(map[string]any{"locked_at": stamp, "updated_at": stamp})
	if result.Error != nil {
		log.Errorf("[Lockdown] ConfirmLocked: %v", result.Error)
		return false, alitaerrors.Wrapf(result.Error, "confirm lock of lockdown %d", id)
	}
	return result.RowsAffected == 1, nil
}
```
`BeginAlert` condition: `state='active' AND locked_at IS NOT NULL AND alert_started_at IS NULL`.

**Insert-once slot** (copy `RecordJoin`, joiners.go 42-83): `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)` then `RowsAffected == 1` means the caller owns the slot; otherwise re-read the existing row. Use for reminder slots `(lockdown_id, kind, seq, dest)`.

**Claim/move** (copy `ClaimJoiner` 85-98 and `MoveJoiner` 103-126): conditional update on `state = from`, stamp `claimed_at`, release with `claimed_at: nil`, `attempts + 1` via `gorm.Expr`. Mirror for slots (`pending -> posting -> posted`, third failed attempt final) and close claims (`close_claimed_at`).

**Stale release** (copy `ReleaseStaleClaims`, joiners.go 300): add slot and burst releases; a duplicate alert is accepted over a lost one.

**Link revoke record once:** `UPDATE chat_lockdowns SET link_revoked_at=... WHERE id=? AND link_revoked_at IS NULL`, `RowsAffected == 1`.

---

### `alita/db/lockdown/burst.go` (repository, batch)

**Analog:** `alita/db/lockdown/joiners.go`

Use `ListJoinersInState` (line 188) for the list shape and `CountJoinersWithDetail` (174) for counts. Candidate query for "Ban N" is `INSERT ... SELECT FROM chat_lockdown_joiners ... ON CONFLICT DO NOTHING` into the burst table for states `pending, acting, banned, ban_failed, declined` (D-21), never `exempt/cancelled/unbanned`. Top-link query: `SELECT invite_link, COUNT(*) ... GROUP BY invite_link ORDER BY 2 DESC LIMIT 2` through `db.DB.Model(...).Select(...).Scan(&rows)` as in `ListActiveByChatsFresh` (repository.go 95-118):
```go
var rows []struct {
	ChatID   int64
	LockedAt time.Time
}
err := db.DB.Model(&models.ChatLockdown{}).
	Select("chat_id, locked_at").
	Where("chat_id IN ? AND state = ? AND locked_at IS NOT NULL", chatIDs, models.LockdownStateActive).
	Scan(&rows).Error
```

---

### `alita/modules/lockdown_alert.go` (worker steps: post, remind, close)

**Analogs:** `alita/modules/lockdown_worker.go` (loop/claim/classify) and `alita/modules/staff_log.go` (destination + translator)

**Imports** (lockdown_worker.go 3-20): stdlib, then gotgbot + logrus, then internal `lockdown`, `models`, `i18n`, `error_handling`, `formatting`, `ratelimit`.

**Pacing** (lockdown_worker.go 40-46): every Telegram call goes through `lockdownPaced`, never `staffPaced`:
```go
func lockdownPaced(ctx context.Context, call func(context.Context) error) error {
	return lockdownPacer.Do(ctx, call)
}
```

**Per-row claim, call, classify** (lockdown_worker.go 334-362, `lockdownDeclineOne`):
```go
err := lockdownPaced(ctx, func(callCtx context.Context) error {
	call, cancel := context.WithTimeout(callCtx, lockdownCallTimeout)
	defer cancel()
	_, declineErr := b.DeclineChatJoinRequestWithContext(call, row.ChatID, row.UserID, nil)
	return declineErr
})
switch lockdownClassify(ctx, err) {
case lockdownCallDone:
	...
case lockdownCallRetryFree:   // rate limited or shutdown: back to pending, no attempt counted
	...
}
// third failure final: row.Attempts+1 >= lockdownMaxAttempts
```
Post the alert with `b.SendMessageWithContext(call, destChat, text, &gotgbot.SendMessageOpts{ParseMode: formatting.HTML, ReplyMarkup: kb, LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true}})` and record `sent.MessageId` (the staff analog discards it, which is why `sendStaffLogPost` cannot be reused).

**Destination resolution** (staff_log.go 91-96):
```go
chat := &gotgbot.Chat{Id: link.GroupChatID, Title: link.GroupTitle, Type: "supergroup"}
channelID, header, ok := actionlog.Destination(chat, logchannels.CategoryAdmin)
if !ok { return }
text := header + body(staffChatTranslator(link.GroupChatID))
```
Staff Group destination: `staff.GetLinkOfGroupFresh(chatID)` (db/staff/repository.go 325, uncached) -> `link.StaffChatID`, always sent (D-20). Log channel respects the `admin` category (D-20). Render the Staff Group copy with `staffChatTranslator(staffChatID)` and the log copy with `staffChatTranslator(lockedChatID)` (staff_notify.go 24).

**Text assembly** (lockdown.go 203-257): user text goes in after translation:
```go
func lockdownSplice(text string, pairs ...string) string { ... }
func lockdownMention(userID int64, name string) string { ... }
func lockdownText(tr *i18n.Translator, key string, params ...i18n.TranslationParams) string { ... }
func lockdownReasonLine(tr *i18n.Translator, reason string) string { ... }
func lockdownTime(t time.Time) string { ... }          // "2 Jan 15:04 UTC"
func lockdownCapRunes(s string, n int) string { ... }   // lockdownNameMaxRunes = 64
```
Reuse these; never `fmt.Sprintf` user text into a translated string.

**Closing edit** : treat `isMessageNotModified(err)` (staff_panel.go 506) as success. Bound each cycle (about 20 edits).

**Cycle hook** (lockdown_worker.go 262-272): add steps and OR into the return:
```go
func runLockdownCycle(ctx context.Context, b *gotgbot.Bot) bool {
	defer error_handling.RecoverFromPanic("lockdownCycle", "Lockdown")
	lockdownReleaseStale()
	settled := lockdownSettleUnconfirmed(ctx, b)
	banned := lockdownBanPending(ctx, b)
	deleted := lockdownDeleteJoinMessages(ctx, b)
	lifted := lockdownProcessLifts(ctx, b)
	return settled || banned || deleted || lifted
}
```
Add `lockdownPlanAlerts`, post slots, reminders, close copies, ban runs; each stops on `ctx.Err()`. Reminder interval as a package var like `lockdownStaleClaim` (line 69): `var lockdownReminderEvery = time.Hour`.

---

### `alita/modules/lockdown_alert_callbacks.go` (callbacks, `lockdown|v1`)

**Analog:** `alita/modules/staff.go` lines 249-316 (dispatch), `lockdown.go` 101-164 (authority)

**Dispatcher shape** (staff.go 252-298): `callbackQueryFromContext`, `decodeCallbackData(query.Data, ns)`, `answerInvalidCallback` on bad data, switch on `decoded.Fields["a"]`, default answers "expired". Answer every press exactly once with `answerStaffCallback(b, query, text, alert)`; edit cards with `editStaffCallbackMessage`.
```go
query, ok := callbackQueryFromContext(ctx)
decoded, ok := decodeCallbackData(query.Data, staffCallbackNamespace)
if !ok || query.Message == nil { return answerInvalidCallback(b, ctx, query) }
```
Registration (staff.go 403): `dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("lockdown|"), lockdownModule.lockdownAlertCallback))` inside `LoadLockdown` (lockdown.go 520-527). Do not start the namespace with `antiraid`.

**Encoding** (callback_codec.go 15-25): `encodeCallbackData` returns `""` on overflow; builders must skip a dead button. Fields: `a` action, `r` slot row ID, `u` asker ID, `e` expiry minutes. Worst case 58 bytes; add a 16-digit-ID size test.

**Authority (live, locked group's chat, not the pressed message's chat)** : copy the checks from `requireLockdownAuthority` (lockdown.go 144-163) but refactor out a bool/reason function, because that CheckFunc replies via `c.Msg` and is valid only inside `WrapCommand`:
```go
member, err := lockdownLiveMember(c.Bot, c.Chat.Id, c.User.Id)   // live getChatMember
if err != nil { /* refuse: lockdown_check_failed */ }
switch staffIssuerSkipReason(member) {          // creator, or admin with can_restrict_members
case "": return true
case staffReasonSkipIssuerNoRight: /* chat_status_restrict_cmd_error */
}
```
For Revoke: creator or administrator with `member.CanInviteUsers`. Refuse `staffServiceUserIDs[query.From.Id]` first (staff_action_decide.go 511). Chat comes from `lockdown.GetFresh(slot.LockdownID).ChatID`, never from `query.Message`.

**Stale button / already lifted** : load the lockdown by the slot's `lockdown_id`; not `active` -> answer "Already lifted by X at T", edit that message (lifted state), change nothing.

---

### `alita/modules/lockdown.go` (modify: shared lift function + alert hook)

**Analog:** itself.

**Hook** after `ConfirmLocked` (lines 387-391): call the alert gate right after, success or not (worker plan step also covers crashes).

**Extract `liftLockdownRow(ctx, b, row, actorID, actorName)`** from `unlockdown` lines 463-515. Order is the safety property: `getChat` compare -> `setLockdownPermissions(bg, b, chat.Id, row.PrePermissions)` -> `lockdownBeginLift(row.ID, actor.Id, name, manualChange)` -> `lockdown.FinishLift`:
```go
if err := setLockdownPermissions(bg, b, chat.Id, row.PrePermissions); err != nil { ... }
won, err := lockdownBeginLift(row.ID, actor.Id, name, manualChange)
...
finished, err := lockdown.FinishLift(row.ID)
if !finished { wakeLockdownWorker() ... }
```
The command replies from the outcome enum; the button Confirm calls the same function and sends the public "Lockdown lifted by X" notice with `SendMessage` through `lockdownPaced` (D-19). `lockdownBeginLift` stays a test seam variable (line 99). Update the AGENTS.md sentence naming it.

---

### `alita/modules/lockdown_response.go` ("Ban N recent joiners")

**Analog:** `lockdown_worker.go` `lockdownBanOne` (364-414) and `decideLockdownJoin` / `isLockdownBan` (`lockdown_marker.go` 21)

**Claim, paced live check, ban, move** (lockdown_worker.go 373-414): same shape, with `BanChatMemberOpts` carrying no `UntilDate` (permanent). Claim the joiner row first (`ClaimJoiner(id, pending|banned, acting)`), then after a successful ban `MoveJoiner(acting -> kept, "response_ban")`:
```go
won, err := lockdown.ClaimJoiner(row.ID, models.JoinerStatePending, models.JoinerStateActing)
if err != nil || !won { continue }
...
live, lookupErr := b.GetChatMemberWithContext(call, row.ChatID, row.UserID, nil)
if member := live.MergeChatMember(); member.Status == gotgbot.ChatMemberStatusKicked {
	deliberate = !isLockdownBan(member, row.BanUntil)
```
Pure decision table `decideLockdownResponseBan(m, banUntil) (ban bool, skip string)` as in research Code Examples, kept beside `decideLockdownJoin`. Skip order: `approvals.IsUserApproved` (db/approvals/repository.go 31), bot self, then the table. Re-read the lockdown fresh before each account and stop when not `active`. Result card edit and one `#LOCKDOWN_BAN` post through `actionlog.Destination` with `sendStaffLogPost`-style body (`staff_log.go` 87-109 for the shape; hashtag helper like `staffLogHashtag`, line 26).

---

### `alita/modules/lockdown_recent.go` (Redis tracker, D-17)

**Analog (partial):** `alita/modules/antiraid.go` lines 124-139 `trackJoin`

```go
if !cache.IsRedisAvailable() { return 0, fmt.Errorf("cache not initialized") }
rdb := cache.GetRedisClient()
member := fmt.Sprintf("%d:%d:%d", userID, now, time.Now().UnixNano())
```
Copy only the availability guard and client access. Differences: key `alita:lockdown:recent:<chat>` (outside `alita:cache:`), one pipelined `ZADD` + `ZREMRANGEBYSCORE` + `ZREMRANGEBYRANK 0 -501` + `EXPIRE 700`, member is compact JSON keyed by user ID so both join paths dedupe, short (about 250 ms) timeout instead of `cache.ContextWithTimeout()` (5 s, `utils/cache/cache.go` 32). Silent no-op without Redis. Called from `lockdownOnJoinMember`/`lockdownOnJoinMessage` before the "no lockdown" early return (lockdown_guard.go ~128-130, 196-198); keep `defer error_handling.RecoverFromPanic(...)` and never change the returned `ext.ContinueGroups` behaviour.

---

### `alita/modules/lockdown_status.go` (modify)

Lines 75-76 use `tally[models.JoinerStateBanned]` only. Change to `banned + kept rows with detail response_ban` (via `lockdown.CountJoinersWithDetail`, joiners.go 174), and use the same helper in reminders.

---

### Locale files and tests

- Locales: copy key names from `locales/en.yml` `lockdown_*` keys (e.g. `lockdown_started_by`, `lockdown_lifted`, `lockdown_already_lifted`, `lockdown_lift_in_progress`, `lockdown_lift_tally_kept`); add every new key to all 7 files; `make check-translations`, and `make generate-docs` if `lockdown_help_msg` changes.
- Test harness: add the three new models to the `AutoMigrate` lists in `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`, `alita/db/lockdown/testmain_test.go`; extend `lockdownCleanup` (lockdown_fake_test.go 223-232), deleting children first:
```go
db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.LockdownJoiner{})
db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.ChatLockdown{})
```
- Fake: extend `lockdownFake` (embeds `*staffActionFake`) with `editMessageReplyMarkup`, `revokeChatInviteLink`, an `onBan` hook, per-user `getChatMember`. Press helper modelled on `pressJoinRequest` (lockdown_fake_test.go ~462-483); `lastAnswer()` and `ageJoiner()` (~486-510) are the patterns for `ageAlert`. Tests assert through `withStaffLocale` markers, not literals; no mock libraries; always `-tags testtools`.
- DB repo tests: copy `alita/db/lockdown/joiners_test.go` helpers (`newJoinRecord`, `mustRecord`, `seedClaimedJoiner`, `TestReleaseStaleClaims`).

## Shared Patterns

### Live authority, never cache
**Source:** `alita/modules/lockdown.go` 101-164. Apply to all three button handlers and the ban-run executor. `lockdownLiveMember` + `staffIssuerSkipReason`; a failed lookup refuses.

### Conditional update, `RowsAffected == 1` wins
**Source:** `alita/db/lockdown/repository.go` 193-203, 255-272; `joiners.go` 85-126. Apply to the alert gate, slot claims, reminder slots, close claims, link revoke record, ban run state.

### Paced Telegram calls and outcome classification
**Source:** `lockdown_worker.go` 40-46 and 287-296 (`lockdownClassify`). Apply to every worker send/edit/ban. A rate-limited or shutdown result puts the row back without costing an attempt.

### Panic recovery on goroutines/handlers
**Source:** `lockdown_worker.go` 200-204, 263; `lockdown_guard.go` 107.
```go
defer error_handling.RecoverFromPanic("lockdownCycle", "Lockdown")
```

### HTML with user text
**Source:** `lockdown.go` 203-257 (`lockdownSplice`, `lockdownMention`, `html.EscapeString`). Apply to alert, reminder, cards, results.

### Fresh reads, no cache
**Source:** `repository.go` header 1-8. New tables are never cached, so no `DeleteCache`; never in backup/export/import/reset.

### Callback encoding
**Source:** `alita/modules/callback_codec.go` 15-25, `alita/utils/callbackcodec`. Only IDs/tokens in data; handle `""` as dead button.

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| Stateless confirm card (inside `lockdown_alert_callbacks.go`) | controller | request-response | Staff cards are Redis hashes (`staff_action_card.go`, `errStaffCardNoRedis`), which is the wrong model; asker ID and expiry go in callback data. Use the staff card only for terminal-edit conventions (edit to Cancelled/Expired, never delete). |
| Sorted-set tracker pipeline | utility | streaming | `trackJoin` uses a Lua script with a 60 s window; only the Redis-availability guard carries over. |

## Metadata

**Analog search scope:** `alita/modules/lockdown*.go`, `staff*.go`, `antiraid.go`, `alita/db/lockdown`, `alita/db/models`, `alita/db/staff`, `alita/utils/actionlog`, `migrations/`
**Files scanned:** about 25 read or grepped
**Pattern extraction date:** 2026-10-11
