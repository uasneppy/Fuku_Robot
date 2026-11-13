# Phase 13: Member Requirements - Pattern Map

**Mapped:** 2026-11-05
**Files analyzed:** 46 new/modified (code, tests, config, docs)
**Analogs found:** 43 / 46 (every analog path was checked with `git ls-files`; all are tracked source, no `.gsd` mirrors)

13-RESEARCH.md (S-01..S-12) fixes names, shapes and decisions; 13-UI-SPEC.md fixes the page contract. This map names the closest tracked analog per file and the excerpt to copy. Line numbers are from the working tree on 2026-11-05. Where RESEARCH gives the target code (watcher skeleton, hold claim, marker test, allow-list, classifier), the planner takes it from RESEARCH "Code Examples" and S-08; this file adds the real code to copy from. Read note: RESEARCH was read in full; UI-SPEC sections "Keys that change", "Rendered-size and content checks" and "Callback data" were read, the page mocks were not re-read (the planner reads them for row layouts and the key table).

## Wave order the planner should follow (RESEARCH decomposition, 10 plans, 4 waves)

1. **Wave 1 (parallel, no code dependency between them):** 13-01 Storage (migration, models, `fuku/db/memberreq`, `IsUserApprovedContext`, `skipLocal`, AutoMigrate lists, backup test, Makefile) · 13-02 Rules engine (pure, corpus test FIRST) · 13-03 Warn core (`applyWarn`; the existing `TestWarn*`/`TestBlacklist*` are the characterization gate, run them green BEFORE the edit) · 13-04 Locale keys and the limits tables (tests first).
2. **Wave 2:** 13-05 Enforcement core (needs 01-04) · 13-06 Settings page (needs 01, 04; page `m` and the harness AutoMigrate entry land in the SAME plan, see "Tests that break").
3. **Wave 3:** 13-07 Watcher · 13-08 Join hook (reset, welcome entry, captcha guard, leave hook) · 13-09 "I fixed it" (needs 05 and 08).
4. **Wave 4:** 13-10 Gate (three-replica test, AGENTS.md pass, generated docs, lint, owner live checks A1-A8).

Test-first order inside waves: the corpus (13-02) and the warn regression run (13-03) are written/run green before the code they guard. The fake profile fields (Wave 0 gap, `staff_action_fake_test.go`) land in 13-05 before any test that reads names.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `migrations/20261106120000_add_member_requirements.sql` | migration | batch | `migrations/20261105120000_add_rules_media_and_greet_once.sql`, `20261104120000_add_settings_hub_tables.sql` | exact |
| `fuku/db/migrations/member_requirements_migration_test.go` | test | batch | `fuku/db/migrations/rules_media_migration_test.go`, `settings_hub_migration_test.go`, `captcha_mode_test.go` | exact |
| `fuku/db/models/member_requirements.go` (`ChatRequirements`, `ChatRequirementHold`) | model | CRUD | `fuku/db/models/raid_detection.go`, `greeted_member.go` | exact |
| `fuku/db/memberreq/settings.go` (`GetSettings`, `GetSettingsFresh`, `Set*`) | repository | CRUD (cached read, column-targeted upsert) | `fuku/db/raiddetection/repository.go` (`loadSettings`, `upsertSettings`) | exact |
| `fuku/db/memberreq/holds.go` (claim, reclaim, take, restore, clean, clear) | repository | CRUD (insert-first latch) | `fuku/db/greetings/greeted.go` (`ClaimGreeting`); `fuku/db/lockdown/joiners.go` `RecordJoin` + reclaim | exact / role-match |
| `fuku/db/memberreq/testmain_test.go`, `*_test.go` | test | CRUD | `fuku/db/menuaccess/testmain_test.go`, `fuku/db/greetings/greeted_test.go` | exact |
| `fuku/db/approvals/repository.go` (+`IsUserApprovedContext`) | repository | request-response | self: `GetApprovedUsersContext` (46-62) | self |
| `fuku/db/cache/local.go` (+`memberRequirementsPrefix`) | config | - | self: `raidDetectionPrefix` (44-48, 67-70) | self |
| `fuku/modules/memberreq_rules.go` (+ `_test.go` corpus) | utility | transform | no analog for the classifier; test style: table tests in `fuku/modules/raid_flood_test.go`; sibling pure helper `raidFloodItemKey` (raid_flood.go:133-163) | no analog (partial) |
| `fuku/modules/warns.go` (`applyWarn` extraction) + `bans.go` (`kickMemberContext`) | service | request-response | self: `warnThisUser` (95-250), `kickMember` (bans.go:27-30) | self |
| `fuku/modules/memberreq_enforce.go` (executor, pacer, cap, runner, notices, drain) | service | event-driven | `raid_trigger.go` (`raidPostTracker` 72-134, `StopRaidPosts` 141), `lockdown_worker.go` (`lockdownPacer` 32-45), `raid_ai.go` (`raidAIEnqueue` 456-480) | role-match |
| `fuku/modules/shutdown_drains.go` (+`RegisterMemberReqDrain`) and `main.go` | config | event-driven | self: `RegisterRaidPostsDrain` (44-50); `main.go:209-225` | exact |
| `fuku/modules/memberreq_watch.go` (group -2 watcher, pass cache, clean path) | handler | event-driven | `raid_flood.go` `raidFloodCountUpdate` (270-330), `raid_ai.go` `raidAIWatcher` (183-215) | exact |
| `fuku/modules/memberreq.go` (module, `init`, `Load`) | module | - | `fuku/modules/raid_flood.go`/`lockdown.go:667` registration; `settings.go:235-237` | role-match |
| `fuku/modules/memberreq_join.go` (join entry, `Reset` dep, leave hook, captcha guard helper) | service | request-response | `greetings.go` `membershipDepsForJoin` (1051-1078), `leftMember` (988-1049), `membership.go` `ProcessSingleJoin` (71-102) | self |
| `fuku/modules/membership.go` (+`Reset` in `MembershipDeps`) | service | request-response | self (37-42) | self |
| `fuku/modules/captcha.go` (`unmuteCaptchaUser` guard, `finishCaptchaPass` last line) | service | request-response | self: `unmuteCaptchaUser` (1630-1641), `finishCaptchaPass` (1697-1745) | self |
| `fuku/modules/memberreq_fixed.go` ("I fixed it", `mreq|v1`) | handler | request-response | `lockdown_alert_callbacks.go` `lockdownAlertCallback` (31-105), `lockdown.go:667`, `lockdown_marker.go` `isLockdownBan` (20-29) | exact |
| `fuku/modules/chat_permissions.go` (comment of `resolveUnmutePermissions`) | docs-in-code | - | self (56-59) | self |
| `fuku/modules/settings_mreq.go` (page `m`, rule screen, card, apply fns) | component (page) | request-response | `settings_lockdown.go` (16-52, 213-260), `settings_access.go` (19-50, 150-200), `settings_captcha.go` (choice rows 165-180), `settings_card.go` | exact |
| `fuku/modules/settings_pages.go` (+`Actions []string` on `settingsControl`) and `settings_menu.go` (`settingsControlFieldsValid`) | component | request-response | self: `settingsControl` (376-386), `settingsCheckFields` (237-295), `settingsControlFieldsValid` (301-311) | self |
| `locales/{en,es,fr,hi,id,pt,ru,uk}.yml` (36 `settings_`, 25 `mreq_`, help text) | config | - | `settings_edit_*` / `settings_btn_edit_*` keys; `settings_help_msg` (en.yml:3203) | exact |
| `fuku/i18n/settings_locale_test.go` (limits rows, label lengths, help ceiling) | test | - | self: `settingsLocaleLimits` (125-142), `TestSettingsLabelLengths` (395-463), `TestSettingsHelpFitsOneMessage` (569-) | self |
| `fuku/i18n/memberreq_locale_test.go` (`TestMemberReqLocaleKeys`) | test | - | `fuku/i18n/captcha_web_locale_test.go` (`captchaWebLimits` 24-, `TestCaptchaWebLocaleKeys` 89-) | exact |
| `fuku/modules/staff_action_fake_test.go` (per-user profile) | test (fixture) | request-response | self: `staffFakeMemberJSON` (248-), `restrictChatMember` (346-372); `lockdown_fake_test.go` `setMemberRaw` (66), `getChatMember` (222) | self |
| `fuku/modules/memberreq_env_test.go` (`memberReqEnv`, `memberReqCleanup`) | test (fixture) | setup | `settings_env_test.go` (`newSettingsEnv` 39, `otherReplica` 103, `settingsCleanup` 192-218), `greet_once_test.go` `greetOnceCleanup` (44-58), `staff_action_target_test.go` `seedStaffUsers` (21-38) | exact |
| `fuku/modules/memberreq_*_test.go` (watcher, join, enforce, fixed, three-replica) | test | event-driven | `raid_flood_test.go`, `raid_trigger_test.go`, `lockdown_paths_test.go`, `greet_once_test.go`, `lockdown_unmute_test.go` | role-match |
| `fuku/modules/settings_mreq_*_test.go` | test | request-response | `settings_menu_test.go`, `settings_main_test.go`, `settings_staff_privacy_test.go`, `settings_edit_screens_test.go` | exact |
| `fuku/modules/test_harness_test.go` (AutoMigrate) and `settings_env_test.go` (`settingsCleanup`) | test (harness) | setup | self (329-378; 192-218) | self |
| `fuku/db/backup/testmain_test.go` + `memberreq_backup_test.go` | test | CRUD | `fuku/db/backup/greeted_backup_test.go` `TestGreetedMembersStayOutOfBackup` (109-152) | exact |
| `Makefile` (`test-postgres-integrity`) | config | - | self (39-44) | self |
| `AGENTS.md` | docs | - | existing bullets (S-12 list) | self |

## Pattern Assignments

### 1. `migrations/20261106120000_add_member_requirements.sql` + `fuku/db/migrations/member_requirements_migration_test.go`

**Analog:** `migrations/20261105120000_add_rules_media_and_greet_once.sql` (header comment style, inline `PRIMARY KEY`, `CREATE TABLE IF NOT EXISTS`), `20261104120000_add_settings_hub_tables.sql` / `20261018130000_add_chat_raid_detection.sql` (inline named `CONSTRAINT ... CHECK`, `uk_` unique).

```sql
CREATE TABLE IF NOT EXISTS greeted_members (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);
```
Use RESEARCH S-05/S-06 columns: `chat_requirements` (`CONSTRAINT uk_chat_requirements_chat UNIQUE (chat_id)`, five SMALLINT action columns each with its own named `CHECK (... BETWEEN 0 AND 4)`, two booleans, `created_at`/`updated_at`) and `chat_requirement_holds` (`CONSTRAINT uk_chat_requirement_holds_chat_user UNIQUE (chat_id, user_id)`, `rules INTEGER CHECK (rules > 0)`, `action SMALLINT CHECK (action BETWEEN 1 AND 4)`, `mute_until BIGINT`, `welcome_held`, `notice_msg_id`, `created_at`, `punished_at NULL`). Header comment names both tables as outside backup, export, import and reset.
**Traps:** no `BEGIN`/`COMMIT`, no top-level `ADD CONSTRAINT` (both appliers swallow `duplicate_object`, so a late add would silently do nothing), no `CONCURRENTLY`; name > `20261105120000`; neither `fuku/db/migrations/runner.go` nor `scripts/migrate_psql.sh` needs an edit (say so in the plan; Phase 12 commit `4553cb1` is the precedent). Re-check the highest filename before planning.
**Test analog** (`settings_hub_migration_test.go:1-40` shape, copied in `rules_media_migration_test.go`): statements via `runner.splitSQLStatements(cleanSupabaseSQL(...))`, skip `isTransactionControlStatement`; `TestMemberRequirementsMigrationStatements` (database-free: count, `IF NOT EXISTS`, no `ADD CONSTRAINT`, sorts last, statements run twice on SQLite or by inspection). Optional PostgreSQL half (`TestMemberRequirementsMigration`) copies `captcha_mode_test.go` shape (rolled-back tx, skips locally); add to Makefile only if written.

---

### 2. `fuku/db/models/member_requirements.go` (model)

**Analog:** `fuku/db/models/raid_detection.go` (whole file) for GORM-built CHECKs and the no-`default:` comment; `fuku/db/models/greeted_member.go` for a table with no foreign key.

```go
JoinThreshold  int `gorm:"column:join_threshold;not null;check:chk_chat_raid_detection_join,join_threshold BETWEEN 3 AND 500" json:"join_threshold"`
...
func (ChatRaidDetection) TableName() string { return "chat_raid_detection" }
```
Copy: five `int` action fields with `type:smallint;not null;check:chk_chat_requirements_last_name,last_name_action BETWEEN 0 AND 4` (constraint names must equal the migration's), `CheckOnJoin`/`DeleteMessages` bool with NO `default:` tag, `ChatID` `uniqueIndex:uk_chat_requirements_chat;not null`, `TableName()` returns `"chat_requirements"`. Hold model: `ID` is the button's `h`, so `uint`/`int64` PK autoIncrement; `PunishedAt *time.Time`; `TableName()` `"chat_requirement_holds"`.
**Traps:** a `default:` tag on any boolean makes GORM `Create` drop a Go `false` (AGENTS Data); CHECKs apply on SQLite too via the tag, so test fixtures that create rows must satisfy them; `json:"-"` is irrelevant because the tables are never exported, but do not put them in `AllExportableModules`.

---

### 3. `fuku/db/memberreq/settings.go` (repository, cached read + column-targeted upsert)

**Analog:** `fuku/db/raiddetection/repository.go` (`loadSettings` ~79-97, column sets and `upsertSettings` ~108-140).

**Read** (repository.go ~80-97):
```go
return cache.GetFromCacheOrLoad(ctx, cache.CacheKey(cacheModule, chatID), cache.DefaultCacheTTL,
	func(ctx context.Context) (*models.ChatRaidDetection, error) {
		var row models.ChatRaidDetection
		err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) { return Defaults(chatID), nil }
		if err != nil { return nil, fukuerrors.Wrapf(err, "read raid detection settings of chat %d", chatID) }
		return &row, nil
	})
```
**Write** (~120-140):
```go
err := db.DB.Clauses(clause.OnConflict{
	Columns:   []clause.Column{{Name: "chat_id"}},
	DoUpdates: clause.AssignmentColumns(columns),
}).Create(row).Error
...
cache.DeleteCache(cache.CacheKey(cacheModule, row.ChatID))
```
Per-column sets: `[]string{"last_name_action","updated_at"}` etc. (one `SetRuleAction(ctx, chat, rule, action)` that picks the column set from a rule table), `SetCheckOnJoin`, `SetDeleteMessages`; a value outside 0..4 returns an error before any write (mirror `thresholdInRange` / `ErrThresholdOutOfRange`). `GetSettingsFresh` = the same query without the cache (the card's same-value check).
**Menuaccess variant for the fresh read:** `fuku/db/menuaccess/repository.go:25-45` (`AdminsAllowedFresh`: nil-DB guard, `ErrRecordNotFound` maps to the zero value, every other error returned wrapped, never guessed).
**Traps:** the loader returns a zero row with the chat ID for no row (all off) and creates NO row (page and Main call it); `skipLocal` must get `memberRequirementsPrefix` (item 5) or another replica serves a switched-off rule for `CACHE_LOCAL_TTL`; first write inserts zeros for the other columns, which is "off", correct; a decoded cache entry written by a later phase's extra column decodes unknown as zero (safe).

### 4. `fuku/db/memberreq/holds.go` (insert-first latch, stale reclaim, conditional delete)

**Analog (claim):** `fuku/db/greetings/greeted.go:19-32`:
```go
row := &models.GreetedMember{ChatID: chatID, UserID: userID, CreatedAt: time.Now()}
result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
if result.Error != nil { log.Errorf("[Database][ClaimGreeting]: chat %d user %d: %v", ...); return false, fukuerrors.Wrapf(...) }
return result.RowsAffected == 1, nil
```
Copy with `Columns: chat_id,user_id` and context (`db.DB.WithContext(ctx)`); `row.ID` after a won create is the button's `h`. `ReleaseGreeting` (greeted.go:41-48, `Where(...).Delete`) is the shape of `ClearHolds`.
**Analog (stale reclaim, conditional update):** `fuku/db/lockdown/joiners.go` `RecordJoin` (43-65 per the Phase 12 map) and AGENTS "lockdown stale claim" (`lockdownStaleClaim = 2 * time.Minute`, `lockdown_worker.go` ~60): one conditional `UPDATE ... WHERE ... punished_at IS NULL AND created_at < ?`, winner is `RowsAffected == 1`. `memberReqStaleClaim` = 2 minutes, a package variable only so tests can shorten it.
**No analog:** `DELETE ... RETURNING` (`TakeHoldByID`) and `RestoreHold` (insert with explicit id, `ON CONFLICT DO NOTHING` with no target). SQLite 3.35+ supports RETURNING and GORM's `clause.Returning{}`; if a SQLite build rejects it, fall back to read then `DELETE ... WHERE id = ?` and check `RowsAffected` (RESEARCH S-05).
**Traps:** no `cache.DeleteCache` anywhere in this file (fresh reads, never cached, D-07); never use `db.DB.Create` on a conflict path without `OnConflict` (a unique violation is an error, not a lost claim); the clean predicate is the exact SQL in RESEARCH S-05 (never delete a mute hold on every clean message, never delete on a message dated before the hold); `ReclaimStaleHold` must refresh `created_at` so a refused punishment is also the 2-minute backoff; store `MuteUntil` on the hold BEFORE the Telegram call.

### 5. `fuku/db/cache/local.go`, `fuku/db/approvals/repository.go`

**`local.go`:** add beside line 44-48 `memberRequirementsPrefix = CacheKey("member_requirements") + ":"` and one `strings.HasPrefix` line in the return of `skipLocal` (67-70), with a comment in the file's style: "a rule switched off must stop punishing on every replica at the next message".
**`approvals/repository.go`:** `IsUserApprovedContext(ctx, chat, user) (bool, error)` over the SAME key and loader as `GetApprovedUsersContext` (46-62):
```go
cacheKey := cache.CacheKey("approvals", chatID)
result, err := cache.GetFromCacheOrLoad(ctx, cacheKey, cache.CacheTTLApprovals, func(ctx context.Context) ([]*models.ApprovedUsers, error) {...})
if err != nil { return nil }   // <-- the existing function hides the error here
```
**Trap:** the existing function returns nil on error, which reads as "not approved"; the new one must return the error (so the job never punishes on an unknown). No new invalidation duty because the key is shared.

---

### 6. `fuku/modules/memberreq_rules.go` (pure rules engine) and its corpus test

**Analog:** none for the classifier (no Unicode script code exists in the repo). Shape and test style: `raidFloodItemKey` (raid_flood.go:133-163) is the pure, table-testable per-message function; `fuku/modules/raid_flood_test.go` for table tests. `golang.org/x/text` is already direct (`go.mod:27`).
Take the classifier verbatim from RESEARCH S-08 (`hasRussian` with `norm.NFC`, `hasArabic` = `unicode.Arabic` and `IsLetter`, `hasHan` = `unicode.Han`, `visible`/`blank` with `Other_Default_Ignorable_Code_Point` and U+2800). Rule table (code, bit, column, notice word) is the one place that Phases 14 and 15 extend; the bitmask bits are 1 last name, 2 username, 4 Russian, 8 Arabic, 16 Chinese.
**Traps:** severity is a separate table (ban > kick > mute > warn), never the raw stored number (1 warn, 2 kick, 3 ban, 4 mute are not in severity order); `strings.TrimSpace` and `unicode.IsSpace` MISS Cf characters, Hangul fillers (Lo) and U+2800 (So); a category-only test misses fillers; U+FFFC is visible (renders a box); Latin ë evades Russian (accepted); NFC not NFKC; Unicode 15 tables (document in the test the CJK Ext I and Arabic 16.0 limits). Write the corpus test FIRST from the RESEARCH S-08 table: Ukrainian names assert ru=false (the corrected A11), Belarusian and Bulgarian mixed, decomposed Пётр true, zero-width inside (`Пе<ZWSP>тр` false, `Э<ZWSP>` true), username rule goes by `User.Username == ""` only. The notice name helper: cap 64 runes, then U+2068/U+2069 isolates, fallback `@username` then `user {id}`, escaped through `formatting.MentionHtml` and spliced after translation.

---

### 7. `fuku/modules/warns.go` (`applyWarn`) and `bans.go` (`kickMemberContext`)

**Analog:** self. `warnThisUser` lines 95-250; the block to move is 121-187.
```go
mu := warnPunishLock(chat.Id, userId)   // :121
mu.Lock()
defer mu.Unlock()
warnrc := warns.GetWarnSettingContext(tracing.UpdateContext(ctx), chat.Id)
numWarns, reasons, err := warns.WarnUser(userId, chat.Id, reason)
if numWarns >= warnrc.WarnLimit {
	switch warnrc.WarnMode {
	case "kick": err = kickMember(b, chat.Id, userId) ... if err != nil { return err }
	case "mute": _, err = chat.RestrictMember(b, userId, MutedPermissions, nil) ...
	case "ban":  _, err = chat.BanMember(b, userId, nil) ...
	}
	if punished { if _, resetErr := warns.ResetUserWarns(userId, chat.Id); resetErr != nil { return resetErr } }
```
Target signature and result struct: RESEARCH S-04 (`applyWarn(ctx, b, chat, userID, reason, afterRecord func()) (warnApplyResult, error)`). `warnThisUser` keeps 95-120 (admin refusal reply, live `GetMember`, translator) and 188-249 (reply building) and maps the result back. **Note a latent quirk to preserve byte-for-byte:** `WarnUser`'s `err` at line 125 is overwritten and unchecked before `numWarns >= WarnLimit`; do not "fix" it inside the extraction (the characterization tests pin today's behaviour), and give the requirement path a checked error.
`kickMember` (bans.go:27-30) becomes a thin caller of `kickMemberContext(ctx, b, chatID, userID)` using `UnbanChatMemberWithContext` and `OnlyIfBanned: false`.
**Traps:** the dwarn/swarn deletion (`:135-144`) must run INSIDE the lock right after the warn is stored (the `afterRecord` hook), keeping today's order; replies at `:110`, `:128`, `:231` stay in `warnThisUser` only; `blacklists.go:572` also calls `warnThisUser` (run `TestBlacklist*`); the limit action inside `applyWarn` stays unpaced (D open question 14); the requirement path posts its own notice (never `warns_user_warning`, never the Remove warn button).
**Regression gate (run before and after):** `go test -tags testtools -race -count=1 -run '^(TestWarn|TestSilentWarn|TestDeleteWarn|TestRemoveWarn|TestRmWarn|TestResetAllWarns|TestLoadWarns|TestBlacklist)' ./fuku/modules` (files `warns_command_test.go`, blacklist tests). Add `TestApplyWarn*` (below limit, at limit per mode, limit action error leaves the warn counted and nothing reset).

---

### 8. `fuku/modules/memberreq_enforce.go` (executor, pacer, cap and pause, runner, notices) and the drain

**Analog (runner):** `raid_trigger.go:72-134` (`raidPostTracker`, `raidPostAsync`) and `raid_ai.go:456-480` (`raidAIEnqueue`: counter taken first, shed when full, `Debug` log with IDs only).
```go
func raidPostAsync(name string, post func()) {
	raidPosts.start()
	go func() {
		defer raidPosts.finish()
		defer error_handling.RecoverFromPanic(name, "Lockdown")
		post()
	}()
}
```
Copy as `memberReqJobs raidPostTracker` plus `memberReqRunAsync(job)`: `start()` BEFORE reading a draining flag, then a buffered/semaphore cap of 32 (`memberReqMaxInFlight`, variable for tests), over the cap `finish()` and drop (the next message retries); the goroutine starts with `defer error_handling.RecoverFromPanic("memberReqJob", "MemberRequirements")`. Do NOT use `sync.WaitGroup` (the comment at raid_trigger.go:67-71 explains why).
**Analog (pacer):** `lockdown_worker.go:32-45`:
```go
var lockdownPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey: "fuku:lockdown:pace:next", BlockKey: "fuku:lockdown:pace:block",
	Interval: 100 * time.Millisecond, MaxRetries: 3, MaxWait: 60 * time.Second,
})
func lockdownPaced(ctx context.Context, call func(context.Context) error) error { return lockdownPacer.Do(ctx, call) }
```
`memberReqPacer` with `fuku:memberreq:pace:next` / `:pace:block`. Tests install a fast one the way `withFastLockdownPacer(t)` does (used in `newLockdownEnv`, lockdown_fake_test.go:339-). `ratelimit.ErrRateLimited` (telegram_pacer.go) means no request was made.
**Analog (cap and pause, Redis):** raid counters in `raid_flood.go` (`raidFloodScript` 100-, `raidKey(chatID, part)` hash-tag keys `fuku:raid:{<chat>}:...`, `raidRedisTimeout`, `raidRedis()` nil without Redis). Use `fuku:memberreq:{<chat>}:cap:<unix minute>` (`INCR` + `EXPIRE 120` pipeline) and `fuku:memberreq:{<chat>}:paused` (`SET NX EX 600`; only the caller whose `SET NX` succeeded posts `mreq_notice_paused`). Pass cache key `fuku:memberreq:pass:<chat>:<user>`. Verdict cache analog for a positive-only Redis value with a TTL: `fuku:raid:aiv:<file_unique_id>` (raid_ai_image.go, `raidAIVerdictCacheTTL`).
**Analog (classification of Telegram errors):** `isTelegramClientError` (settings_gate.go:256-262): 4xx other than 429 = definitely not applied; 5xx/timeout = unknown.
**Drain:** `shutdown_drains.go:44-50` (`RegisterRaidPostsDrain`) and `StopRaidPosts` (raid_trigger.go:141-149):
```go
func RegisterRaidPostsDrain(m *shutdown.Manager) {
	m.RegisterHandlerWithTimeout(func() error { log.Info("[Shutdown] Draining raid detection posts..."); StopRaidPosts(); return nil },
		raidPostStopWait+shutdownDrainGrace)
}
```
`RegisterMemberReqDrain` (wait 2 s + grace) cancels the runner context first, then waits on the tracker; register in `main.go` immediately before `modules.RegisterStaffActionsDrain(shutdownManager)` (main.go ~225; see the comment block at 209-224 for the LIFO rationale). Add a test beside `shutdown_drains_test.go`.
**Traps:** Redis down means no cap, no pass cache, no pause (correct, slower; nothing for correctness lives only in Redis, D-15); every Telegram call of the engine (live `getChatMember`, punishment, notice, delete) through `memberReqPaced`; a mute notice is never dropped by pacing (wait out the block, bounded by the job context) while one Telegram-refusal retry is separate; no names, text or message content in logs (IDs only); the executor reads the live record's names after `getChatMember` (rename race) and treats `left`/`kicked`/restricted-not-member as exempt; tri-state at every step (an unknown never punishes): approved via `IsUserApprovedContext`, pending captcha via `captcha.HasPendingCaptchaAttemptsContext` then `captcha.GetCaptchaAttempt` (NOT `aispamCarvedOut`, aispam.go:238-248, which treats an error as "not carved out"), admin via the cached list then live (NOT `IsUserAdmin`, chat_status.go:217-226 returns false on failure). Marker: `lockdownBanSpan` (lockdown_marker.go:35) now plus 330 days, `RestrictChatMemberWithContext(..., MutedPermissions, &gotgbot.RestrictChatMemberOpts{UntilDate: marker})`; ban is `BanChatMemberWithContext` with nil options; kick is `kickMemberContext`.

---

### 9. `fuku/modules/memberreq_watch.go` + `memberreq.go` (group -2 watcher, module registration)

**Analog:** `raid_flood.go:247-330` (`raidFloodWatcher` and `raidFloodCountUpdate`) and `raid_ai.go:183-215` (`raidAIWatcher`/`raidAIWatch`).
```go
func (m moduleStruct) raidFloodWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("raidFloodWatcher", "Lockdown")
	return raidFloodCountUpdate(b, ctx, false)
}
...
userID, ok := raidFloodSenderCounts(b, ctx)         // raid_flood.go:199-213
itemKey, slash, ok := raidFloodItemKey(ctx.EffectiveMessage)   // :133-163
...
if chat_status.IsApprovedForUpdate(b, ctx, chat.Id, userID) { return ext.ContinueGroups }
```
Use the RESEARCH "Code Examples: watcher's shape" as the skeleton; reuse `raidFloodItemKey`'s `ok` and `raidFloodSenderCounts` unchanged; `cachedAdminStatus` (antiflood.go:106-121) for the synchronous cached-admin check; hand the case to `memberReqRunAsync`. Registration: `dispatcher.AddHandlerToGroup(handlers.NewMessage(func(m *gotgbot.Message) bool { return m.Chat.Type == "supergroup" }, memberReqModule.memberReqWatcher), memberReqWatchGroup)` with `memberReqWatchGroup = -2`, no `SetAllow*`; `dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("mreq|"), ...))` (as `settings.go:235-237`, no `SetAllowChannel`, unlike `lockdown.go:667`); `RegisterLegacyModule("MemberRequirements", 241, LoadMemberRequirements)` (priority 241: free, Settings is 239; confirm with `grep RegisterLegacyModule` before planning, AGENTS says a duplicate NAME is dropped silently).
**Traps:** `ext.ContinueGroups` on EVERY path (a `nil` here is harmless today only because the group's other handler is `NewMyChatMember` at `bot_updates.go:273-275`); the watcher makes no Telegram call and no database write for a clean member beyond the pass-cache-guarded clean `DELETE`; settings read error means return (never punish); the pass value is the first 16 hex chars of SHA-256 of rule signature + names (never the names in a key); write the pass value only after a clean verdict AND a successful clean predicate; a violation is never cached; basic groups out of scope (supergroup filter); do not add another message handler to group -2 later (AGENTS line). Add the group to `RegisterLegacyModule` ordering knowledge: `TestRegisteredModulesRespectGotgbotDefaultGroupSemantics` (`registry_test.go:124-153`) pins nil vs ContinueGroups semantics; no edit needed, but add `TestMemberReqWatcherContinuesGroups` (every path returns `ext.ContinueGroups`, commands still run).

---

### 10. `fuku/modules/memberreq_join.go`, `membership.go`, `greetings.go` hooks, `captcha.go` guard

**Join entry (shared by both paths).** Analogs, both self:
`greetings.go:1051-1078` (`membershipDepsForJoin`), final closure:
```go
Welcome: func(user gotgbot.User) error {
	ctxCopy := ext.Context{EffectiveChat: chat}
	if threadID != 0 { ctxCopy.EffectiveMessage = &gotgbot.Message{Chat: *chat, MessageThreadId: threadID} }
	return SendWelcomeMessage(bot, &ctxCopy, user.Id, user.FirstName)
},
```
becomes a call to `memberReqJoinThenWelcome(bot, &ctxCopy, user, src)`; `captcha.go:1742` (`SendWelcomeMessage(bot, in.Ctx, in.UserID, in.FirstName)` at the end of `finishCaptchaPass`, 1697-1745) becomes the same call with `gotgbot.User{Id: in.UserID, FirstName: in.FirstName}`. One entry means the two paths cannot drift (D-01). With Check on join off or no rule on it sends the welcome and returns; otherwise one live `getChatMember` under a 15 s context, `memberReqEnforce` synchronous with `src=join`, welcome only when not removed and not held; refused or unanswerable check sends the welcome.
**Reset dep.** `membership.go:37-42` `MembershipDeps` gains `Reset func(user gotgbot.User)`; `ProcessSingleJoin` (membership.go:71-102) calls it after a won `Claim` (line ~79) and before `DecideJoin`; `membershipDepsForJoin` fills it with the join's own date (`ctx.ChatMember.Date`, or `msg.Date` for the service message; both paths already reach `processSingleNewMember` at greetings.go ~1080). `TakeHoldsBefore(chat, user, joinDate)` deletes holds created before the join; a deleted punished mute hold whose live status is still restricted with the marker is lifted through the SAME unmute core as "I fixed it".
**Leave hook.** `greetings.go:988-1000`: add `memberreq.ClearHolds(chat.Id, leftMember.Id)` right after `clearRecentJoinProcessing(chat.Id, leftMember.Id)` (line 997); log, do not abort the goodbye on error.
**Captcha guard.** `captcha.go:1630-1641`:
```go
func unmuteCaptchaUser(bot *gotgbot.Bot, chatID, userID int64) error {
	chat, err := bot.GetChat(chatID, nil)
	...
	perms, err := resolveUnmutePermissions(chat)
	...
	_, err = bot.RestrictChatMember(chatID, userID, perms, nil)
	return err
}
```
Add at the top: read the hold fresh; on a read error return the error (every caller schedules a retry, see `restoreCaptchaPermissions` 1643-1650); when a punished mute hold exists, re-apply `MutedPermissions` with `UntilDate: hold.MuteUntil` and return nil. Re-applying the same absolute marker keeps "I fixed it" valid.
**Traps:** ProcessSingleJoin already dedupes the two deliveries via `claimRecentJoinProcessing` (greetings.go:87-124); put the reset and the check AFTER the won claim, never before. A muted member's leave/rejoin is INVISIBLE to `leftMember`/`newMember` (`chat_status.go:584-586`: old and new status both `restricted`); only the `new_chat_members` service message (`cleanService`, greetings.go:1092-1138) sees the rejoin, so a test that sends a restricted-to-restricted `chat_member` update and expects `leftMember` is wrong (Pitfall 5). The welcome is held by a conditional `SetHoldWelcomeHeld` on the existing hold (D-02); greet-once claims only inside `SendWelcomeMessage` (greetings.go ~924-929), so never call it for a kicked, banned or held member. The lockdown guard (-7) and fed-ban (-6) end the update first (D-01 holds without code); a warn sends its notice first, then the welcome. A hold created between the join and the attempt row is the rare D-08 race the captcha guard covers. All existing join/captcha tests must pass unchanged: with no `chat_requirements` row the entry is one cached read plus `SendWelcomeMessage` (`greet_once_test.go`, `captcha_*_test.go`, `lockdown_paths_test.go`, `lockdown_unmute_test.go:194,275`, `membership_test.go` which builds `MembershipDeps{}` literals, so the new field must be nil-safe like `Claim`).

---

### 11. `fuku/modules/memberreq_fixed.go` ("I fixed it", `mreq|v1`)

**Analog:** `lockdown_alert_callbacks.go:31-105` (`lockdownAlertCallback`) for the registration and the ordered refusals; `lockdown_marker.go:11-29` (`isLockdownBan`) for the marker test; `settings_card.go:27-41` (`settingsCardNumber`) for strict digit parsing; `callback_codec.go:15-45` for the codec.
```go
query, ok := callbackQueryFromContext(ctx)
if !ok { return ext.EndGroups }
decoded, ok := decodeCallbackData(query.Data, lockdownCallbackNamespace)
if !ok || query.Message == nil { return answerInvalidCallback(b, ctx, query) }
fields := decoded.Fields
if staffServiceUserIDs[query.From.Id] { answerStaffCallback(b, query, ..., true); return ext.EndGroups }
```
Marker test (RESEARCH "Marker ownership", extends lockdown_marker.go:20-29): status restricted, `IsMember`, `muteUntil != 0`, `|UntilDate - muteUntil| <= lockdownBanUntilTolerance` (2). Encode the button with `encodeCallbackData("mreq", map[string]string{"a": "f", "h": id, "u": user})` (returns `""` on overflow: assert `Omitted`-style zero in a bytes test; worst case 52 bytes). Steps 1-10 are RESEARCH S-09 in order; the unmute permissions are ONLY `resolveUnmutePermissions(chatInfo)` (chat_permissions.go:66-84; an error means nothing changed); the conditional `TakeHoldByID` runs BEFORE the restrict, `RestoreHold` with the same row on failure.
**Traps:** answer exactly once on every path (defer an empty answer after the recover, as `settingsCallback` does); `ctxTr(ctx)` in a group callback is the GROUP's language (`lang/repository.go:42-53`), not the presser's: use it, and drop the UI-SPEC two-language sub-test in favour of "answers and edit are in the group's language"; the hold must match chat, user, `action = 4`, `punished_at` set and `notice_msg_id` = the pressed message, else `mreq_fixed_stale`; the press is presser-only (`query.From.Id == u`), no admin override; step 4's live `getChatMember` needs the bot to be an administrator (a bot that lost rights answers check-failed); bound the whole flow to about 10 s; the held welcome goes through `SendWelcomeMessage` only when `welcome_held`, once; a message-check hold sends no welcome (D-02). Also update the `resolveUnmutePermissions` comment (chat_permissions.go:56-59) to list "I fixed it" and the join reset.

---

### 12. `fuku/modules/settings_mreq.go` (page `m`, rule screen, card, apply functions) and the allow-list edit

**Analogs:** page registration `settings_lockdown.go:33-52` and `settings_access.go:31-50`; two-tap card `settings_access.go:155-200` (`settingsAccessSet` draws the card for the risky value, writes the safe one at once; `settingsAccessYes` does `settingsCardExpired` then applies) plus `settings_card.go:66-84` (`settingsCardRows`); choice rows `settings_captcha.go:165-180` via `settings_pages.go:314-332` (`settingsChoiceRow`) and toggles `:336-348` (`settingsToggleButton`); apply function with typed result `applyAdminsAllowed` (settings_access.go:73-); controls driven through an apply function `settingsLockdownSetTrigger` (settings_lockdown.go:223-258).
```go
registerSettingsPage(settingsPage{
	Key: settingsPageAccess, Order: 90, Icon: "🔑", TitleKey: "settings_page_access",
	MainState: settingsAccessMainState, Open: settingsAccessOpen,
	Controls: map[string]settingsControl{ settingsAccessControlSwitch: {Right: settingsRightOwner, Set: settingsAccessSet} },
	Actions:  map[string]settingsPageAction{ settingsActAccessYes: {Fields: []string{"e"}, Right: settingsRightOwner, Run: settingsAccessYes} },
})
```
Page `m`, `Order` 80, icon 🛂, controls `ln un ru ar zh` (`Right: settingsRightRestrict`, `Actions: []string{"st"}`, `Validate` one digit 0..4) and `j d` (`Validate` "0"/"1", both `tg` and `st`), actions `mo` (`Fields: []string{"t"}`, right `settingsRightAdmin`, `Validate` exactly `t` a rule code) and `my` (`Fields: t,v,e`, right `settingsRightRestrict`, `Validate`: `t` rule code, `v` in 2..4, `e` via `settingsCardNumber`). Choices 2/3/4 draw the card with `settingsCardRows(k, map[string]string{"a":"my","t":rule,"v":v}, "m")`; 0/1 call `applyMemberReqAction` at once; a same-value press (judged by `memberreq.GetSettingsFresh`) writes nothing. The rule screen's Back is `k.button` with `{"a":"op","p":"m"}` plus Close, NOT `navRow` (UI amendment M5).
**Allow-list edit** (the one edit of the shared files): `settings_pages.go:376-386` add `Actions []string` to `settingsControl`; `settings_menu.go:301-311` `settingsControlFieldsValid(action string, fields map[string]string)` with `len(control.Actions) == 0 || slices.Contains(control.Actions, action)` before `Validate`, and the one call site (`:265-269`) passes `action`. Nil `Actions` keeps today's behaviour for every existing page.
**Traps:** the page reads through a side-effect-free loader that creates no row (`GetSettings`), and a failed read is the cannot-open edit and the unknown mark on Main; no screen, label, notice, alert or callback names the Staff Group (D-21/D-32; add the page to `TestSettingsStaffPrivacy`); callback data codes and numbers only, at most 64 bytes (card Yes is 53 private); a private page has at most 8 rows and a group-menu page 9 (this page is 5); the card's Yes goes through the whole press pipeline and a presser demoted between taps is refused at Yes with `settings_no_restrict` and nothing written; `registerSettingsPage` panics at init on a duplicate page key or action code (free: page `m`, codes `mo`, `my`; taken page keys `s l c f w g r a`, taken action codes `lk ua uy pg ls ly ay ws wr wc gs gr gc rs rr rc wv wf gv gf rv rf`); one apply function per setting with a typed result (`captchaApplyResult` precedent, `applyCaptchaTimeout` in captcha.go), validation, write and `cache.DeleteCache` inside it; there is no command, so the tests are drift and authority tests, not a parity test.

---

### 13. Locale files and locale tests

**Analog:** `settings_edit_*` / `settings_btn_edit_*` keys in `locales/en.yml`; the limits table `fuku/i18n/settings_locale_test.go:125-142` (`"settings_help_msg": {1100, 1540}`), `TestSettingsLabelLengths` (395-463: `gridMax = 20`, `pages` slice of `{icon, key}` at ~410, `options` list at ~423-426, `editorCells` ~437-441), `TestSettingsHelpFitsOneMessage` (569-610).
Add: 36 `settings_` keys (each needs a row in the limits table, the test fails when the table does not cover EXACTLY the `settings_` keys: 199 before, 235 after); 25 `mreq_` keys (new `TestMemberReqLocaleKeys` in `fuku/i18n/memberreq_locale_test.go`, copy `captchaWebLimits` + `TestCaptchaWebLocaleKeys` from `captcha_web_locale_test.go:24-130`: table covers exactly the locale's `mreq_` keys, present and non-empty in all 8 locales, within ceiling, no markup, placeholders equal English, quote-consistency and rendered-notice checks per UI-SPEC check 11). `TestSettingsLabelLengths` gains `{"🛂", "settings_page_mreq"}`, the seven cells rendered with their icons (≤ 20 code points), and `settings_state_off_plain`, `settings_opt_warn` in the choice-row list (13 EN, 15 others). Letters ы ъ э ё must appear literally in `settings_mreq_hint_letters` and `settings_mreq_desc_russian` in all 8 locales.
**Help text trap:** `settings_help_msg` ceiling is 1100 runes in English (current en length is about 961, headroom about 139) and 1540 elsewhere, and it must stay one Telegram message with the module help header. UI-SPEC changes it in all 8 locales (adds "and member requirements" and one sentence "Member requirements have no command: the menu is the only way to set them"). The `settingsHelpCommands` list (591-595) needs no change (no new command). Copy key names from `locales/en.yml`; `make check-translations` only sees literal `GetString` keys, so the `mreq_` keys built by templates need the new test. Run `make generate-docs` and `make check-docs` after the help text changes (the docs generator reads en.yml).

---

### 14. Test fixtures and hygiene (the part that bit Phase 12)

**`memberReqEnv`** (`memberreq_env_test.go`): build on `newSettingsEnv(t)` (settings_env_test.go:39) which wraps `newLockdownEnv` (lockdown_fake_test.go:339: miniredis via `withMiniredisServer`, `withStaffLocale`, `withFastLockdownPacer`, `lockdownFake` over `staffActionFake`); for the three-replica test use `env.otherReplica()` (settings_env_test.go:103: a second fake, bot and dispatcher over the SAME database and miniredis, update IDs from 50000). Load the Greetings, Captcha, Warns and the new module on the env's dispatcher the way `newSettingsEnvWith` loads its modules (settings_env_test.go:46-100). Helpers: `sendAs(user, text)`, `joinAs(user)`, `pressFixed(user, notice)`, `holdOf(user)`, `noticesTo(chat)`, `jobs.wait()` (`memberReqJobs.wait`, like `raidPosts.wait`, registered with `t.Cleanup` as at settings_env_test.go ~190).
**Names in `getChatMember`:** `staffFakeMemberJSON` (staff_action_fake_test.go:248) hard-codes `{"id":%d,"is_bot":false,"first_name":"User%d"}` and `restrictChatMember` (346-372) replaces the member record. Two options for the planner: (a) add a per-user profile map to `staffActionFake` consulted only by the JSON builder (zero value keeps today's answer; research Wave 0), or (b) use `lockdownFake.setMemberRaw` (lockdown_fake_test.go:66; `getChatMember` consults `memberRaw` at ~222-260) which already supplies raw answers, but check whether `memberRaw` survives a later `restrictChatMember` before relying on it for the "marker present" assertions (the marker needs the restricted record's `until_date`, which only the staff record carries). Prefer (a).
**Cleanup pattern to copy:** `greetOnceCleanup` (greet_once_test.go:44-58):
```go
t.Cleanup(func() {
	db.DB.Where("chat_id = ?", chatID).Delete(&models.GreetedMember{})
	db.DB.Where("user_id IN ?", greetOnceMemberIDs).Delete(&models.User{})
	for _, id := range greetOnceMemberIDs { dbcache.DeleteCache(dbcache.CacheKey("user", id)) }
	dbcache.ResetLocalForTest()
})
```
and `seedStaffUsers` (staff_action_target_test.go:21-38: delete before AND after, `ResetLocalForTest`). `memberReqCleanup(t, chats, users)` deletes both new tables' rows for the chats, the `models.User` rows and `user` cache keys of EVERY member ID the test used (`warns.WarnUser` creates a users row through `EnsureUserInDb`, repository.go:92-97, and the captcha path stores one from the fake), the warn rows (`models.Warns`, `models.WarnSettings`), `chat_requirements` cache key `member_requirements`, the approvals key, `dbcache.ResetLocalForTest()`, and the Redis keys of its chats (`fuku:memberreq:*`; `otherReplica` shares the miniredis). Extend `settingsCleanup` (settings_env_test.go:192-218) with both tables and the cache key so every settings test also clears them.
**Traps:** NEVER use small fixed user IDs (`greetOnceMemberIDs` = 4242, 4343, 777000 is exactly the Phase 12 CI failure: a leaked users row for 4242 changed a later test's "unknown user" expectation, commit `7030575`); mint IDs with `uniqueLinkOwnerID()` (staff_link_test.go:21) and chat IDs with `uniqueModuleChatID()` (rules_command_test.go:14); CI runs all of `fuku/modules` in one PostgreSQL database, so a test passing on SQLite can still leak; assert hold rows by chat+user, never by count of the table.

**AutoMigrate lists to extend** (add `&models.ChatRequirements{}, &models.ChatRequirementHold{}`): `fuku/modules/test_harness_test.go` (list 329-378, after `&models.GreetedMember{}`), a NEW `fuku/db/memberreq/testmain_test.go` modelled on `fuku/db/menuaccess/testmain_test.go` (`os.Exit(testdb.Run(m, &models.ChatSettingsAccess{}))`), `fuku/db/backup/testmain_test.go` (list near 41-48, beside `&models.GreetedMember{}`). **Trap:** the harness entry must land in the SAME plan as page `m` (item 15, "every page ends with Back and Close" iterates `settingsPagesInOrder()`).

**Backup exclusion test:** copy `TestGreetedMembersStayOutOfBackup` (greeted_backup_test.go:109-152) as `TestMemberRequirementsStayOutOfBackup` with both tables (export holds only backup modules and `ExportChatData(..., []string{"chat_requirements"})` is an error, import creates no row in the destination, reset leaves the source rows). Use fresh `time.Now().UnixNano()` chat IDs and `cleanupBackupChat` plus deletes of both tables.

**Makefile `test-postgres-integrity`** (Makefile:39-44): append `|TestChatRequirementsSettings|TestRequirementHoldClaimConcurrent` (and `TestMemberRequirementsMigration` only if the PostgreSQL half is written) INSIDE the anchored `-run` group before `)$$`, and `./fuku/db/memberreq` to the package list (`./fuku/db/greetings` is currently last). The pattern is anchored with `$`, so each name must be the real top-level function name. Analog tests for the concurrency shape: `TestGreetedMemberClaimConcurrent` (greetings, already in the list) and `TestStartLockdownOneActivePerChat` (lockdown).

---

### 15. AGENTS.md sections to update (same commit as the behaviour change)

Handlers (group list: `-2` becomes "admin-cache (my_chat_member) and the member requirements watcher (messages only, the only message handler of the group, `ext.ContinueGroups` on every path); never add a message handler to -2 ahead of it"; the module list for the new module, priority 241); Callbacks (new `mreq|v1` namespace; page `m` of `settings|v1` with codes `mo`, `my`, controls `ln un ru ar zh j d`, fields, the strict validation, and one clause on the new `settingsControl.Actions` allow-list, since "a new page never edits `settings_pages.go`" stays true for registering pages); Permissions (`settingsRightRestrict` for page `m`, "I fixed it" presser-only with no admin override, the marker, `resolveUnmutePermissions` callers now include "I fixed it" and the join reset); Data (`chat_requirements`, `chat_requirement_holds`, every `fuku:memberreq:*` key, the `skipLocal` entry, never part of backup, export, import or reset, fresh reads); Startup and shutdown (new drain `RegisterMemberReqDrain`, its place before `RegisterStaffActionsDrain`, 2 s plus grace); Subsystem traps (the marker end date, the captcha choke-point guard in `unmuteCaptchaUser`, the join reset and the invisible muted leave, the `applyWarn` core, the pass cache meaning "skips the clean DELETE"); Testing (AutoMigrate lists, the PostgreSQL list, the fake's profile fields, the cleanup rule and unique-ID rule, the new test families). The existing sentences "`-2` admin-cache" and "A new page is one `registerSettingsPage` call ... and never edits `settings_pages.go`" become false or incomplete.

## Shared Patterns

### Fresh read, no cache (holds)
**Source:** `fuku/db/menuaccess/repository.go:25-45` and `fuku/db/greetings/greeted.go`. **Apply to:** `holds.go`, every hold read. nil-DB guard, `ErrRecordNotFound` is not an error, every other failure wrapped and returned; no `DeleteCache`.

### Column-targeted upsert + cache invalidation
**Source:** `fuku/db/raiddetection/repository.go` (`upsertSettings`). **Apply to:** every `Set*` of `settings.go`; a Go `false`/`0` is always written; each write ends with `cache.DeleteCache(cache.CacheKey("member_requirements", chat))`.

### Watchers never end an update
**Source:** `raid_flood.go:247-257`, `raid_ai.go:183-190`. **Apply to:** `memberReqWatcher`: `defer error_handling.RecoverFromPanic(...)` first line, `ext.ContinueGroups` on every path.

### Fire-and-forget goroutine
**Source:** `raid_trigger.go:127-134`. **Apply to:** the runner and any timer: counter started before the goroutine, `defer finish()`, `defer error_handling.RecoverFromPanic(name, module)`.

### Paced Telegram call + definitive-vs-unknown classification
**Source:** `lockdown_worker.go:32-45`, `settings_gate.go:256-262`. **Apply to:** every engine call; 4xx other than 429 and `ErrRateLimited` mean "nothing happened", 5xx/timeout mean unknown (verify the marker with one live `getChatMember`).

### Settings press pipeline and two-tap card
**Source:** `settings_card.go`, `settings_access.go:155-200`, `settings_lockdown.go:223-258`. **Apply to:** `settings_mreq.go` only; the page adds no new pipeline step.

### Logging
Logs carry chat and user IDs only, never names, rule text, notice text or message content (AGENTS "Go rules" and the editor's no-content rule). Notice names are escaped (`formatting.MentionHtml`) and spliced AFTER translation; templates hold no markup.

## Tests that break (or must change on purpose) — schedule their edits

| Existing test | File:line | Why it breaks | Fix in plan |
|---|---|---|---|
| owner's group Main shape `"s|l / c|f / w|g / r / a / close"` | `fuku/modules/settings_edit_test.go:990` | page `m` (Order 80) joins the grid | becomes `"s|l / c|f / w|g / r|m / a / close"` (13-06) |
| hand-built `mainEntries` | `fuku/modules/settings_staff_privacy_test.go:113-123` | privacy render lacks the `m` entry | add `{Key: "m", Icon: "🛂", Title: ..., State: ..., Known: true}`; add page `m`, rule screens and cards to the rendered set (13-06) |
| every page ends with Back and Close (group and private) | `fuku/modules/settings_menu_test.go:470-512` | iterates `settingsPagesInOrder()`; page `m` must open in the env, so `ChatRequirements` must be in the harness AutoMigrate list in the same plan | harness entry in 13-06; the rule screen is a documented exception (Back carries `a=op&p=m`) so keep the assertion on the PAGE |
| Main grid end to end (odd/even page counts, rows) | `fuku/modules/settings_main_test.go:215-260`, `settings_test.go:165-176` | dynamic from the registry; the grid row count and the `MainState` call need the new table | verify only, no literal edit expected |
| worst-case Main / screens | `fuku/modules/settings_edit_screens_test.go:106-118` | `settingsMainWorstState` is applied to every page incl. `m`; new screens need adding to `settingsEditScreens`-style fit tests | add `TestSettingsMreqPageFitsEveryLocale`; Main stays dynamic |
| `TestSettingsLocaleKeys` (exact key coverage) | `fuku/i18n/settings_locale_test.go` (~290) | 36 new `settings_` keys without limit rows fail it | add the 36 rows (13-04) |
| `TestSettingsLabelLengths` | `settings_locale_test.go:395-463` | new labels and choice cells | add grid page, seven cells, `settings_state_off_plain`, `settings_opt_warn` |
| `TestSettingsHelpFitsOneMessage` and `settings_help_msg` ceiling | `settings_locale_test.go:138, 569-` | changed help text in all 8 locales; en ceiling 1100 (now about 961) | keep each locale within ceiling and one message; shorten translations that do not fit; only widen the ceiling with a recorded reason |
| `TestLoadSettingsRegistersHelp`, settings help test | `fuku/modules/settings_help_test.go` | help text changed | verify only |
| warn commands and blacklist warns | `fuku/modules/warns_command_test.go`, blacklist tests (`blacklists.go:572` calls `warnThisUser`) | `applyWarn` extraction must be byte-identical | must pass UNCHANGED; run before and after |
| captcha unmute and rollback paths | `fuku/modules/lockdown_unmute_test.go:194,275` and `captcha_*_test.go` | `unmuteCaptchaUser` gets the hold guard | pass unchanged (no hold, one extra fresh read: the test DB needs `chat_requirement_holds` in the harness list in 13-08 at the latest) |
| join and welcome tests | `greet_once_test.go`, `greetings_command_test.go` (`TestMemberJoinAndLeaveSendConfiguredGreetings`), `membership_test.go` (`MembershipDeps{}` literals), `lockdown_paths_test.go` | `Welcome` closure and `finishCaptchaPass` call the join entry; `MembershipDeps` gets `Reset` | nil-safe `Reset`; unchanged behaviour with no `chat_requirements` row |
| module registry / group semantics | `fuku/modules/registry_test.go:105-153` | a new handler at group -2 | no edit expected; add the watcher test |
| `/help` generated docs | `make check-docs` | en.yml `settings_help_msg` changed | `make generate-docs` in 13-10 |
| backup/export module lists | `fuku/db/backup/*_test.go` | none if the tables stay out of `AllExportableModules` | `TestMemberRequirementsStayOutOfBackup` proves it |

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `fuku/modules/memberreq_rules.go` classifier (Russian letters, Arabic and Han scripts, invisible-name test, NFC) | utility | transform | no Unicode script or normalisation code exists in the repo; use RESEARCH S-08 (probe-verified) and `golang.org/x/text/unicode/norm` |
| `memberreq.TakeHoldByID` (`DELETE ... RETURNING`) and `RestoreHold` (explicit-ID insert) | repository | CRUD | no repository uses `RETURNING` or re-inserts a fixed primary key; RESEARCH S-05 gives the shape and a read-then-delete fallback |
| Redis positive-only pass cache keyed by a hash of names | utility | cache | nearest is the verdict cache `fuku:raid:aiv:<file_unique_id>` (raid_ai_image.go) and the limiter windows `fuku:captcha:rl:*` (captchaweb/limiter.go); neither hashes input fields, copy only the `SET EX` and error-is-miss behaviour |

## Metadata

**Analog search scope:** `fuku/modules`, `fuku/db/{greetings,raiddetection,menuaccess,approvals,cache,lockdown,backup,migrations}`, `fuku/i18n`, `fuku/utils/ratelimit`, `migrations/`, `main.go`, `Makefile`, `locales/en.yml`.
**Files scanned:** about 70 (read in full or by range; `git ls-files` run on every cited path).
**Pattern extraction date:** 2026-11-05
**Open points for the planner:** (1) fake-profile option (a) vs (b) in item 14; (2) confirm module priority 241 is free; (3) the pre-existing unchecked `err` at `warns.go:125` must be preserved inside `applyWarn` and checked only on the requirement path; (4) A1-A8 live checks are owner steps in 13-10, not CI.
