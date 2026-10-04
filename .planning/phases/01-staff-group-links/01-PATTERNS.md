# Phase 1: Staff Group Links - Pattern Map

**Mapped:** 2026-10-04
**Files analyzed:** 21 new/modified
**Analogs found:** 19 / 21

All analog paths below were checked as git-tracked source. Locale files are `en es fr hi id pt ru` (research correction C4), not the list in CLAUDE.md.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `migrations/2026100xxxxxxx_add_staff_groups_and_links.sql` | migration | CRUD | `migrations/20260826000000_add_federations_and_log_channels.sql` | exact |
| `alita/db/models/staff.go` | model | CRUD | `alita/db/models/antiflood.go` (check tags), federation models | exact |
| `alita/db/staff/repository.go` | repository | CRUD + tx | `alita/db/federations/repository.go` | exact |
| `alita/db/staff/testmain_test.go` | test | CRUD | `alita/db/federations/testmain_test.go` (or `internal/testdb.Run`) | exact |
| `alita/db/staff/repository_test.go` | test | CRUD | `alita/db/federations/repository_test.go` | exact |
| `alita/db/cache/local.go` (modify `skipLocal`) | config | cache | itself (`captchaPendingPrefix`) | exact |
| `alita/modules/staff.go` | module/controller | request-response | `alita/modules/federations.go` (joinFed/leaveFed, fedCallback, Load) + `admin.go` (WrapCommand) | role-match |
| `alita/modules/staff_panel.go` | component | transform + request-response | `federations.go` `fedCallback` (edit-in-place) | partial |
| `alita/modules/staff_ownership.go` | watcher + worker | event-driven, batch | `alita/modules/bot_updates.go` (watchers), `antiraid.go` (poller lifecycle) | role-match |
| `alita/modules/deeplink_router.go` (modify) | utility | request-response | itself | exact |
| `alita/modules/help.go` `start` (modify) | controller | request-response | itself (lines ~360-381) | exact |
| `alita/utils/chat_status/owner.go` | utility | request-response | `chat_status/access.go` `RequireUserOwner` | role-match |
| `alita/utils/helpers/command_pipeline.go` (add `RejectAnonymousSender`) | middleware | request-response | existing `CheckFunc` constructors in same file | role-match |
| `alita/modules/staff_*_test.go` | test | request-response | `alita/modules/admin_test.go` + `test_harness_test.go` | exact |
| `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go` (modify AutoMigrate) | test config | - | themselves | exact |
| `main.go` (start sweeper in `postInit`, shutdown handler) | config | lifecycle | `main.go` lines 190-206, 410-417 | exact |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` | config | i18n | existing `feds_*` keys | exact |
| `locales/config.yml` | config | help alt names | existing entries | exact |
| `AGENTS.md` (group `-3`, `alita:staff:*`) | docs | - | its Handlers/Data sections | exact |
| Chat-migration handler (in `staff_ownership.go`) | watcher | event-driven | `bot_updates.go` | no true analog |
| Hourly sweeper w/ Redis lock | worker | batch | `antiraid.go` poller + `federations.go:1024` SetNX | role-match |

## Pattern Assignments

### `migrations/<ts>_add_staff_groups_and_links.sql` (migration)

**Analog:** `migrations/20260826000000_add_federations_and_log_channels.sql` (lines 1-40). Latest existing is `20261001120000_...`, so the new timestamp must be greater.

```sql
CREATE TABLE IF NOT EXISTS federation_chats (
    id BIGSERIAL PRIMARY KEY,
    fed_id VARCHAR(36) NOT NULL,
    chat_id BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_chats_chat_id ON federation_chats (chat_id);
CREATE INDEX IF NOT EXISTS idx_federation_chats_fed_id ON federation_chats (fed_id);
```

Rules: no top-level BEGIN/COMMIT, no CONCURRENTLY, no FK on chat IDs (federation_chats precedent). Add CHECKs and `UNIQUE(group_chat_id)` per RESEARCH "Data model". For the D-10 PG triggers use dollar-quoted `DO $$`/`CREATE FUNCTION`; precedent `migrations/20260919120000_add_ai_spam_settings.sql`. Runner splits statements via `splitSQLStatements` (`alita/db/migrations/runner.go`). Keep `scripts/migrate_psql.sh` in agreement if the runner changes (it should not).

### `alita/db/models/staff.go` (model)

**Analog:** `alita/db/models/antiflood.go` (lines 1-17).

```go
type AntifloodSettings struct {
	ID     uint  `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatId int64 `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id,omitempty"`
	Limit  int   `gorm:"column:flood_limit;default:5;check:chk_antiflood_limit,flood_limit >= 0" json:"limit,omitempty"`
	Action string `gorm:"column:action;default:'mute';check:chk_antiflood_action,action IN ('mute','ban',...)" json:"action,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}
func (AntifloodSettings) TableName() string { return "antiflood_settings" }
```

Use `check:chk_staff_link_distinct,group_chat_id <> staff_chat_id` and the health IN-list check. `TableName()` must match the migration.

### `alita/db/staff/repository.go` (repository, CRUD + tx)

**Analog:** `alita/db/federations/repository.go`.

**Imports** (lines 1-20):
```go
import (
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
)
```

**Cache key + invalidation** (lines 21-60):
```go
cachePrefixFedChat = "fed_chat"
func invalidateChat(chatID int64) {
	cache.DeleteCache(cache.CacheKey(cachePrefixFedChat, chatID))
}
```
Staff version: `invalidateStaff(chatIDs ...int64)` that deletes `staff_group:`, `staff_links:`, `staff_link_of:` for every ID passed. On RekeyChat pass both old and new IDs, after commit.

**Cached gate read with negative sentinel** (lines 223-243, `GetChatFedContext`):
```go
result, err := cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cachePrefixFedChat, chatID), cache.CacheTTLFederation, func(ctx context.Context) (models.FederationChat, error) {
	var row models.FederationChat
	err := db.GetRecordContext(ctx, &row, models.FederationChat{ChatID: chatID})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.FederationChat{}, nil
	}
	...
})
if result.ChatID == 0 { return nil }
```
Use this for "is this chat a Staff Group" only. Authority reads (`...Fresh`) call `db.DB` directly and bypass the cache.

**Write pattern** (JoinFed, lines 245-267, and `RenameFederation` lines 98-119): `db.CreateRecord`, log `[Staff] Fn: %v`, return the error, then invalidate. Use `db.UpdateRecordWithZeroValues(..., map[string]any{...})` for health/owner (never `Updates(struct)`). For the race-free link insert, use `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(...)` and check `RowsAffected` (precedent `alita/db/chats/repository.go:38`, `warns/repository.go:37`). Winner-decides deletes: `db.DB.Where("id = ? AND owner_user_id = ?", ...).Delete(...)` and test `RowsAffected == 1`.

### `alita/db/staff/testmain_test.go` and `repository_test.go` (test)

**Analog:** `alita/db/federations/testmain_test.go` (manual temp SQLite + AutoMigrate list including `&models.User{}`, `&models.Chat{}`). `internal/testdb.Run(m, tables...)` (`internal/testdb/database.go:17`) is the shorter form and prepends User and Chat; prefer it as RESEARCH says. Tests follow `federations/repository_test.go`: `ownerID := time.Now().UnixNano()`, `t.Cleanup` deleting rows, real DB, no mocks. The D-10 triggers are PG-only, so gate that test on `ALITA_TEST_DATABASE`; the CHECK and UNIQUE constraints run in SQLite.

### `alita/db/cache/local.go` (modify)

**Analog:** itself (lines 44-56).
```go
captchaPendingPrefix = CacheKey("captcha_pending") + ":"
func skipLocal(key string) bool {
	return strings.HasPrefix(key, captchaPendingPrefix)
}
```
Add `staff_group:`, `staff_links:`, `staff_link_of:` prefixes built the same way, joined with `||`.

### `alita/modules/staff.go` (module, request-response)

**Analogs:** `alita/modules/federations.go` for the owner-check and refusal flow and callbacks; `alita/modules/admin.go` for `WrapCommand` registration (federations still uses raw `handlers.NewCommand`, which AGENTS.md says not to copy).

**Descriptor + registration** (`admin.go:699-708, 800`):
```go
adminlistDesc = helpers.CommandDescriptor{
	Name: "adminlist", Disableable: true,
	RequiredChecks: []helpers.CheckFunc{
		helpers.CheckDisabled("adminlist"), helpers.RequireBotAdmin(), helpers.RequireGroup(),
	},
}
helpers.WrapCommand(dispatcher, adminlistDesc, adminModule.adminlist)
```
Name descriptors `setStaffDesc`, etc. The docs generator regex requires `helpers.WrapCommand(dispatcher, xDesc, mod.handler)`. Put `helpers.RejectAnonymousSender()` first in `RequiredChecks`. `RequireGroup()` lets channels through (`access.go:201`), so `/setstaff` must itself require Type in {group, supergroup}.

**Refusal flow to copy** (`federations.go:181-226`, joinFed):
```go
tr := ctxTr(ctx)
if !chat_status.RequireUserOwner(b, ctx, chat, from.Id) {
	text, _ := tr.GetString("feds_owner_only_join")
	replyHTML(b, msg, text)
	return ext.EndGroups
}
...
text, _ := tr.GetString("feds_joined", i18n.TranslationParams{"name": html.EscapeString(fed.Name), "id": fed.FedID})
```
Differences: use the tri-state helper instead of `RequireUserOwner`; use literal `GetString("...")` calls only (no dynamic keys); `html.EscapeString` every title; refusals for a non-owned Staff Group go only to the issuing chat as a self-deleting reply (`time.AfterFunc(30*time.Second, ...)` precedent at `captcha.go:1329`).

**Callbacks** (`federations.go:1144-1174`, `fedCallback`; registration line 1273):
```go
dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix(fedCallbackNamespace+"|"), federationsModule.fedCallback))
query, ok := callbackQueryFromContext(ctx)
decoded, ok := decodeCallbackData(query.Data, fedCallbackNamespace)
action := decoded.Fields["a"]
_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: trS(tr, "feds_callback_denied"), ShowAlert: true})
_, _, _ = query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: text, ParseMode: formatting.HTML})
```
Encode with `encodeCallbackData("staff", map[string]string{"a": "ul", "l": "<linkID>"})` (`alita/modules/callback_codec.go:15`); it returns `""` on overflow. Register `callbackquery.Prefix("staff|")`. Always `query.Answer`.

**Module registration** (`federations.go:1282`): `RegisterLegacyModule("Staff", 236, LoadStaff)`; Load starts with `SetModuleEnabled(...)`.

### `alita/modules/deeplink_router.go` and `help.go` `start` (modify)

**Analog:** `deeplink_router.go` lines 13-47: `deepLinkRegistry` map, `RegisterDeepLinkHandler`, longest-prefix match in `HandleDeepLink`. Add a parallel `groupDeepLinkRegistry` and `RegisterGroupDeepLinkHandler(prefix, handler)` plus `HandleGroupDeepLink`, same longest-prefix loop, returning false when nothing matches.

`help.go` non-private branch (~373-381) to extend:
```go
} else {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("help_pm_questions")
	_, err := msg.Reply(b, text, formatting.Shtml())
```
Insert before this: if `len(args) == 2` and a group handler matches, call it and return its result; otherwise keep the reply. `TestStartCommandRepliesInPrivateAndGroup` pins the no-payload group behaviour and must still pass. The payload is `stf_<digits>` parsed with `strconv.ParseInt`.

### `alita/modules/staff_ownership.go` (watchers + sweeper)

**Watcher registration analog:** `bot_updates.go:LoadBotUpdates` (lines ~225-250):
```go
dispatcher.AddHandlerToGroup(handlers.NewMyChatMember(
	chat_status.ExtractAdminUpdateStatusChange, adminCacheAutoUpdate), -2)
dispatcher.AddHandler(handlers.NewChatMember(chat_status.ExtractAdminUpdateStatusChange, adminCacheAutoUpdate))
```
New: `AddHandlerToGroup(handlers.NewMessage(func(m) bool {...ChatOwnerChanged||ChatOwnerLeft}, h).SetAllowBot(true), -3)`, the same for `message.Migrate`, `NewChatMember`, and `NewMyChatMember`. Watchers return `ext.ContinueGroups`. `botJoinedGroup` (`bot_updates.go:34-66`) returns `ext.EndGroups` after leaving basic groups at group `-1`, so group `-3` runs before it. Group `-3` is new, so update the AGENTS.md group list.

**Sweeper lifecycle analog:** `antiraid.go:84-121` (Start/Stop):
```go
antiRaidCtx, antiRaidCancel = context.WithCancel(context.Background())
antiRaidPollerWG.Add(1)
go func(ctx context.Context) {
	defer antiRaidPollerWG.Done()
	defer error_handling.RecoverFromPanic("antiRaidExpiryPoller", "antiraid")
	antiRaidModule.expiryPoller(ctx)
}(antiRaidCtx)
// Stop: cancel(); WG.Wait() without holding the mutex
```
The ticker loop is at `antiraid.go:257-270`. The staff version takes `*gotgbot.Bot`, so it is started from `postInit` like `modules.StartCaptchaLifecycle(b)` (`main.go:415`). Redis lock precedent: `client.SetNX(cache.Context, "alita:fed_export:"+fedID, "1", fedExportCooldown)` (`federations.go:1024`). Run unguarded when `cache.IsRedisAvailable()` is false, because Pattern 3 makes the work idempotent. Pace links with `select { case <-ctx.Done(): ...; case <-time.After(250ms): }`.

**Chat migration:** no codebase analog (nothing handles `migrate_to_chat_id` today). Use RESEARCH Pattern 8 and the gotgbot `message.Migrate` filter.

### `alita/utils/chat_status/owner.go` (utility)

**Analog:** `access.go:182-193` (`RequireUserOwner`).
```go
func RequireUserOwner(b *gotgbot.Bot, ctx *ext.Context, chat *gotgbot.Chat, userId int64) bool {
	chat = extractChatFromContext(ctx, chat)
	mem, err := chat.GetMember(b, userId, nil)
	if err != nil || mem == nil { return false }
	return mem.GetStatus() == "creator"
}
```
It returns a bare bool, so an API error looks like "not owner". New `FetchOwnerID(b, chatID) (int64, error)` calls `b.GetChatAdministratorsWithContext` and returns the user with `GetStatus() == "creator"`. Callers get match / mismatch / unknown, and only mismatch may delete. Add a bot-status helper using `b.GetChatMember(chatID, b.Id, nil)` with `MergeChatMember()`.

### `alita/utils/helpers/command_pipeline.go` (add `RejectAnonymousSender`)

**Analog:** the `CheckFunc` type and `WrapCommand` (lines 43-68): `RequiredChecks` run in order and return `ext.EndGroups` on the first false. Copy an existing constructor in the same file (e.g. `RequireGroup()`) for shape. The check tests `ctx.EffectiveSender.IsAnonymousAdmin()` and User.Id in `{1087968824, 777000}`, and sends the "post as yourself" reply through the pipeline's responder.

### Tests in `alita/modules/`

**Analog:** `admin_test.go:61-85` with `test_harness_test.go` (`newModuleBotClient`, `client.responses["getChatAdministrators"] = ...`, `newModuleTestBot`, `newModuleMessageContext`, `uniqueModuleChatID`, `client.callsFor("sendMessage")`).
```go
client := newModuleBotClient()
client.responses["getChatAdministrators"] = []byte(`[{"status":"creator","user":{"id":4242,...}}]`)
bot := newModuleTestBot(client)
ctx := newModuleMessageContext(bot, chat, user, "/adminlist")
cmdCtx, _ := helpers.BuildCommandContext(bot, ctx)
if err := mod.handler(cmdCtx); err != ext.EndGroups { ... }
```
Add `getChatMember` and `getChatAdministrators` canned responses, plus error responses for the "unknown leaves link intact" test. Sweeper lock test uses `withMiniredis(t)` (`antiraid_miniredis_test.go:22`). Add `&models.StaffGroup{}, &models.StaffGroupLink{}` to the AutoMigrate list (`test_harness_test.go` near line 323-360 and `alita/db/testmain_test.go`).

### `main.go` (modify)

**Analog:** shutdown handlers at lines 190-206:
```go
shutdownManager.RegisterHandler(func() error {
	log.Info("[Shutdown] Stopping anti-raid expiry poller...")
	modules.StopAntiRaidExpiryPoller()
	return nil
})
```
and `postInit` (line ~413): `if err := modules.StartCaptchaLifecycle(b); err != nil { log.Fatalf(...) }`. Add `modules.StartStaffSweeper(b)` after it and a handler `modules.StopStaffSweeper()` registered after the DB-close handler so LIFO runs it first.

## Shared Patterns

### Cache invalidation on every write
**Source:** `alita/db/federations/repository.go:21-60`. **Apply to:** every function in `alita/db/staff/repository.go`. Invalidate after commit, both old and new IDs on re-key.

### Live authority, fail closed
**Source:** new `chat_status` tri-state helper (basis `access.go:182-193`). **Apply to:** `linkGroup`, `recheckLink`, panel callbacks, `/setstaff`, `/unsetstaff`. Only a successful response with a different creator is a mismatch.

### Rows-affected decides the single notice
**Source:** RESEARCH Pattern 3 using GORM `RowsAffected`. **Apply to:** auto-unlink notices, health transitions (D-13, D-14).

### i18n
**Source:** `federations.go` `tr.GetString("feds_...")` with `i18n.TranslationParams`. **Apply to:** all modules. Literal keys only; per-chat language for background notices via `lang.GetLanguage(&ext.Context{EffectiveChat: &gotgbot.Chat{Id: staffChatID}})` (precedent `captcha.go:1321`). Add every key to all 7 files and run `make check-translations`.

### Panic-safe goroutines
**Source:** `antiraid.go:100-105`. **Apply to:** sweeper and any `go func`.

### Logging
`log.Errorf("[Staff] Fn: %v", err)` style, as in `[Federations] JoinFed: %v`.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| Chat-migration handler / `RekeyChat` | watcher + repo tx | event-driven | Nothing handles `migrate_to_chat_id` today; use RESEARCH Pattern 8 |
| Group `/start@bot <payload>` route | controller | request-response | Only private-chat deep links exist; extend the registry as above |
| `staff_panel.go` live builder with `errgroup` | component | transform | No existing multi-call live status panel; `golang.org/x/sync` is already required (`errgroup` subpackage not yet used, `singleflight` is) |

## Metadata

**Analog search scope:** `alita/db/*`, `alita/modules`, `alita/utils/chat_status`, `alita/utils/helpers`, `migrations`, `main.go`, `internal/testdb`
**Files scanned:** about 25 (targeted reads)
**Pattern extraction date:** 2026-10-04
**Not read:** RESEARCH.md lines 389-662 (Pitfalls 10+, assumptions log, validation) were truncated in my read, so the planner should read them directly.
