# Phase 10: PM Settings Hub and Owner Gate - Pattern Map

**Mapped:** 2026-11-04
**Files analyzed:** 30 new/modified
**Analogs found:** 28 / 30 (all analog paths verified git-tracked)

Research (10-RESEARCH.md R-01..R-12) already fixes names and shapes; this map gives the closest tracked analog and the excerpt to copy for each.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `migrations/20261104120000_add_settings_hub_tables.sql` | migration | schema | `migrations/20261018130000_add_chat_raid_detection.sql` | exact (inline CREATE TABLE constraints) |
| `fuku/db/models/chat_admin_index.go` | model | CRUD | `fuku/db/models/raid_detection.go` | exact |
| `fuku/db/models/chat_settings_access.go` | model | CRUD | `fuku/db/models/raid_detection.go` (no `default:` tag on bool) | exact |
| `fuku/db/menuaccess/*.go` | repository | CRUD, fresh read | `fuku/db/raiddetection/repository.go` `upsertSettings` (minus cache) + `fuku/db/lockdown` fresh reads | role-match |
| `fuku/db/adminindex/*.go` | repository | batch replace, fresh read | `fuku/db/staff/repository.go`, `fuku/db/lockdown/burst.go` | role-match |
| `*/testmain_test.go` (adminindex, menuaccess) | test | setup | `fuku/db/raiddetection/testmain_test.go` | exact |
| `fuku/utils/cache/adminCache.go` (observer) | utility | event-driven | itself (`loadAdminCacheFromTelegram`, singleflight, generation guard) | self |
| `fuku/modules/admin_index_feed.go` | service | event-driven | `fuku/modules/bot_updates.go` `adminCacheAutoUpdate` | role-match |
| `fuku/modules/admin_index_sweeper.go` | service | batch/background | `fuku/modules/staff_sweeper.go` | exact |
| `fuku/modules/settings_gate.go` | utility | request-response | `fuku/utils/chat_status/owner.go` (`CheckOwner`), `lockdown.go` `lockdownLiveMember` | role-match |
| `fuku/modules/settings_access.go` | component (page) | CRUD | `fuku/modules/settings_captcha.go` + `settings_card.go` | exact |
| `fuku/modules/settings_hub.go` | handler | request-response | `fuku/modules/settings_staff.go` (picker), `settings_menu.go` | role-match |
| `fuku/modules/settings_menu.go`, `settings_pages.go`, `settings.go`, `lockdown.go` | modify | request-response | self | self |
| `main.go` | config | lifecycle | lines ~203-208 and 483 (staff sweeper) | exact |
| `locales/*.yml` (8), `AGENTS.md`, docs | config | - | existing `settings_` keys | exact |
| `fuku/modules/test_harness_test.go`, `settings_env_test.go`, `staff_helpers_test.go` | test | setup | self | self |
| `fuku/modules/settings_gate_test.go`, `settings_access_test.go`, `settings_hub_test.go`, `admin_index_sweeper_test.go` | test | - | `staff_action_lifecycle_test.go` (`otherReplica`), staff sweeper test | role-match |
| `fuku/i18n/settings_locale_test.go` | test | - | self (`settingsLocaleLimits`, `settingsAlertKeys`) | self |
| `Makefile` (postgres-integrity) | config | - | line 41 `-run` list | self |

## Pattern Assignments

### Migration (schema)
**Analog:** `migrations/20261018130000_add_chat_raid_detection.sql`. Header comment explaining table, "never cached / never backup", no FK on chat_id; `CREATE TABLE IF NOT EXISTS`, `id BIGSERIAL PRIMARY KEY`, `created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()`; constraints and CHECK **inline** (never bare `ALTER TABLE ADD CONSTRAINT`, appliers swallow duplicate_object); `CREATE INDEX IF NOT EXISTS`; no BEGIN/COMMIT. Latest existing is `20261101120000_default_language_ukrainian.sql`; re-check max at execution. No change to `runner.go` / `migrate_psql.sh` (plain additive file).

### Models
**Analog:** `fuku/db/models/raid_detection.go`
```go
type ChatRaidDetection struct {
	ID     uint  `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatID int64 `gorm:"column:chat_id;uniqueIndex:uk_chat_raid_detection_chat;not null" json:"chat_id,omitempty"`
	JoinEnabled bool `gorm:"column:join_enabled;not null" json:"join_enabled"` // NO default: tag
	... check:chk_chat_raid_detection_ai,ai_threshold BETWEEN 3 AND 500
}
func (ChatRaidDetection) TableName() string { return "chat_raid_detection" }
```
Copy for `ChatAdminIndex` (`uniqueIndex:uk_...` composite on chat_id,user_id; `check:` on role IN creator/administrator; index on user_id) and `ChatSettingsAccess` (`AdminsAllowed bool` with no default tag, `uk_chat_settings_access_chat`). The GORM tags must match the migration because SQLite tests build the table from them.

### `fuku/db/menuaccess` (fresh read + column-targeted upsert)
**Analog:** `fuku/db/raiddetection/repository.go` (imports `db`, `models`, `fukuerrors`, `gorm`, `clause`). Copy:
```go
if db.DB == nil { return fukuerrors.Wrap(errors.New("database not initialized"), "...") }
err := db.DB.Clauses(clause.OnConflict{
	Columns:   []clause.Column{{Name: "chat_id"}},
	DoUpdates: clause.AssignmentColumns(columns),
}).Create(row).Error
if err != nil { return fukuerrors.Wrapf(err, "write ... of chat %d", row.ChatID) }
```
Differences: use `db.DB.WithContext(ctx)`, columns `admins_allowed, updated_by, updated_at`, and **omit `cache.DeleteCache`** (fresh-read table; AGENTS sentence to add). Read: `First` with `errors.Is(err, gorm.ErrRecordNotFound)` -> `false, nil` (loader shape in `loadSettings`, lines ~74-90), other errors wrapped and returned.

### `fuku/db/adminindex`
**Analog:** `fuku/db/staff/repository.go` and `fuku/db/lockdown/*.go` for `*Fresh` naming and plain `db.DB` queries. `ReplaceChat` = one `db.DB.Transaction`: upsert (`ON CONFLICT (chat_id,user_id) DO UPDATE role, updated_at`) then `DELETE ... NOT IN`; not delete-then-insert. Sort/page in Go (R-01).

### testmain for new db packages
**Analog:** `fuku/db/raiddetection/testmain_test.go`
```go
//go:build testtools
func TestMain(m *testing.M) { os.Exit(testdb.Run(m, &models.ChatRaidDetection{})) }
```
Also add both models to the AutoMigrate list in `fuku/modules/test_harness_test.go` (list ends near line 375 `&models.ChatRaidDetection{},`, call at 329), `fuku/db/devs/testmain_test.go` is NOT needed (stats untouched). Add a `TestStaffTablesStayOutOfBackup` twin (`fuku/db/staff/repository_test.go` ~964-975).

### `admin_index_sweeper.go`
**Analog:** `fuku/modules/staff_sweeper.go` (copy whole skeleton):
- lock key const outside cache prefix + `uuid.NewString()` owner (lines 19-24); pace/interval vars (27-33); `...PaceFunc(ctx)` timer-select (45-58); `FirstDelay` var with `rand.IntN` jitter (66-69; use 30-90 s); `CycleHook` var (73-75).
- `Start`/`Stop` with `Mu`, `Cancel`, `WG`; goroutine: `defer WG.Done(); defer error_handling.RecoverFromPanic("name","Cat")` (lines 87-100); Stop cancels, waits **without** holding mutex, then clears cancel (103-123).
- Loop/cycle (126-153): timer, `Cycle` with own `RecoverFromPanic`.
- Lock (155-185): `cache.GetRedisClient()` nil -> true; `SetNX(cache.Context, key, owner, ttl)`; error -> warn and run unguarded. Improvement per R-06: compare-and-delete lock on cancel.
- Never delete rows on an error (staff sweep P4 rule, ~226-230). Candidates: `SELECT chat_id FROM chats WHERE chat_id < 0`.

### `main.go`
**Analog:** lines 200-208 and 483.
```go
// Registered after the DB-close handler, so LIFO stops the sweeper before the database closes.
shutdownManager.RegisterHandler(func() error {
	log.Info("[Shutdown] Stopping staff sweeper...")
	modules.StopStaffSweeper()
	return nil
})
...
modules.StartStaffSweeper(b)   // postInit, line 483
```
Add the admin-index pair right next to each.

### `settings_access.go` (Access page, two-tap ON card)
**Analog:** `settings_captcha.go` registration (lines 43-56) and `settings_card.go`.
```go
registerSettingsPage(settingsPage{
	Key: settingsPageCaptcha, Order: 30, Icon: "🧩", TitleKey: "settings_page_captcha",
	MainState: settingsCaptchaMainState, Open: settingsCaptchaOpen,
	Controls: map[string]settingsControl{ t: {Right: settingsRightAdmin, Set: fn} },
	Actions:  map[string]settingsPageAction{ code: {Fields: []string{"e"}, Right: ..., Run: fn} },
})
```
Add `OwnerOnly bool` to `settingsPage` (struct at `settings_pages.go:318`), `settingsRightOwner` to `settingsRightRefusal` (`settings_menu.go:67`; alert-style return `("settings_access_owner_only", true)`). Card: `settingsCardRows(k, yesFields, noPage)`; Yes parses `settingsCardExpired(fields)` (not ok -> invalid request) before running; `settingsCardExpiry`/`settingsCardLifetime` var for tests. Apply function pattern: `applyCaptchaTimeout` in `captcha.go` (typed result with outcome, reply key, stored row, err). Lift card in `settings_lockdown.go` is the closest Yes-with-object-ID example (`settingsCardObjectID`). `registerSettingsPage` panics on dup key/action code and core codes (`settingsCoreActions` at pages.go:368: add `pk`).

### `settings_gate.go`
**Analog:** `fuku/utils/chat_status/owner.go` `CheckOwner` (8 s `liveCheckTimeout`, scan of `getChatAdministrators` for the creator, lines ~73-122) and `adminCache.go` singleflight (`golang.org/x/sync/singleflight`). Test seams as package vars like `lockdownCurrentLookup`. Use `lockdownLiveMember` for the live member (already used by verify/deep link). 4xx classification: `errors.As(err, &tgErr)` with `tgErr.Code` 400-499 except 429 (see `lockdown.go:397-402`; do not reuse `lockdownDefinitiveRefusal`).

### `lockdown.go` extraction
Split `requireLockdownAuthority` (lines ~156-176) into `lockdownAuthorityMember(c, needRestrict) (gotgbot.MergedChatMember, bool)` plus wrapper; keep `PromptAnonAdminProof` branch; reply via `lockdownRefuse` (lines 73-88). Anonymous re-entry needs no change: `RegisterAnonymousAdminHandler("settings", anonPipelineHandler(settingsDesc, ...))` (moderation.go:349-360) re-runs `RequiredChecks`.

### `settings.go` / `settings_menu.go` modifications
- Callback registration (settings.go:173): `dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))`; add a second `callbackquery.Prefix("shub|")` handler; no new handler group.
- `settingsPress.verify` step order (menu.go ~315-343): insert step 4b after the member check and before the private `getChat`; `pk` special mapping only for `pk`.
- `settingsCheckFields` (menu.go:194-205) exactly-one-of `c`/`u`; `pk` allows exactly `a`,`c`.
- Deep link `settingsStartDeepLink` (settings.go:102-164): keep strict `parseStaffChatIDArg`; replace IsBotMissingError mapping with class helper + gate; set `Owner` on the hand-built view.
- Answer exactly once only via `settingsPress.answer`; the hub handler builds a `settingsPress` with no group.

### `admin_index_feed.go` and `cache/adminCache.go`
**Analog:** `bot_updates.go:93-105` (`adminCacheAutoUpdate`: `cache.InvalidateAdminCache(chat.Id); cache.LoadAdminCache(b, chat.Id)`); add `adminindex.ClearChat` when new bot status is left/kicked. Observer: `atomic.Pointer` setter in `utils/cache` (must not import `fuku/db`), fired inside `loadAdminCacheFromTelegram` only after `storeResult` and only when `generation == adminCacheGeneration.Load()`; never on the retry-failure path. Install in `LoadSettings` (pointer set only, no DB access, per AGENTS pre-main rule). Wrapper: 3 s context, `recover`, synchronous.

### Tests
- Two-replica: copy `staffActionEnv.otherReplica` (`staff_action_lifecycle_test.go:90-115`): new `newStaffActionFake`, `newModuleTestBot(fake.moduleBotClient)`, `bot.BotClient = fake`, `fake.setCreator`, `fake.setMember(..., staffFakeMember{Status: ...})`, fresh `ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})`, `LoadSettings(...)`. Build `settingsEnv.otherReplica()` the same way.
- `newSettingsEnvWith` (settings_env_test.go ~43-67): after `fake.setCreator(env.chat.Id, env.admin.Id)` seed `menuaccess.SetAdminsAllowed(..., true)` and cleanup in `settingsCleanup`; extend `staffBotClient.adminList` (staff_helpers_test.go:112-120) to list administrator records.
- Sweeper test: mirror the staff sweeper tests using `...CycleHook`, shortened pace/first delay vars.
- Locale tests: add 19 rows to `settingsLocaleLimits`, 3 alert keys to `settingsAlertKeys`, help limits to `{1100,1540}` in `fuku/i18n/settings_locale_test.go`; rewrite `TestSettingsLabelLengths`. Add the `staffLocales` == `locales/*.yml` guard.
- PostgreSQL: add `TestAdminIndexReplaceConcurrent`, `TestMenuAccessUpsert` and packages to `Makefile` line ~41.

## Shared Patterns

- **Live authority, never cache:** `lockdownLiveMember` / `getChatMember` per press (AGENTS Permissions).
- **Fresh-read tables:** no `cache.DeleteCache`; state this in AGENTS Data section.
- **Callbacks:** `encodeCallbackData` / `decodeCallbackData` only; worst-case 64-byte test per page (`TestSettings*CallbackBytes`).
- **Goroutines:** `defer error_handling.RecoverFromPanic(name, cat)`.
- **Errors:** `fukuerrors.Wrapf(err, ...)` (nil-safe); never discard DB errors on write paths.
- **i18n:** `settingsText(tr, "key")` (invisible to `check-translations`; `TestSettingsLocaleKeys` is the guard); 8 locales including `uk.yml`; titles escaped and spliced in after translation (`staffDisplayTitle`).
- **Privacy (D-21/D-32):** use `settingsView.Owner` (new) for Access; keep `Creator` for the Staff title only.

## No Analog Found

| File | Reason |
|---|---|
| Paged private group list render (`settings_hub.go`, `shub` namespace) | No existing paged list in the settings menu; use `settings_staff.go` picker (max 8, row-ID buttons) and the read-only staff panel paging (`pg` action, `settingsParseNumber` clamp) as partial analogs, plus UI-SPEC |
| Deleted-account creator heuristic (`settingsCreatorGone`) | New; Telegram behavior unverified (RESEARCH A1), needs human-verify checkpoint |

## Metadata

**Analog search scope:** fuku/db/{raiddetection,staff,lockdown,models}, fuku/modules/{staff_sweeper,settings_*,staff_action_lifecycle_test,bot_updates,lockdown}, migrations, main.go
**Note:** only RESEARCH.md lines 1-398 were read; the Validation Architecture tail was not.
**Pattern extraction date:** 2026-11-04
