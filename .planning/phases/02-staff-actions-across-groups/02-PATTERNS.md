# Phase 2: Staff Actions Across Groups - Pattern Map

**Mapped:** 2026-10-05
**Files analyzed:** 17 new/modified
**Analogs found:** 16 / 17 (all paths verified git-tracked with `git ls-files`)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `alita/modules/staff_action.go` (module reg, priority 65, interceptors, gate, anon/refusal) | handler/module | request-response | `alita/modules/staff.go` (LoadStaff, init) + `alita/modules/staff_watchers.go` (raw handlers, ContinueGroups) | role-match (deliberately NOT `WrapCommand`) |
| `alita/modules/staff_action_parse.go` (pure arg parser) | utility | transform | `alita/utils/extraction/extraction.go` `parseTemporaryDuration` + `alita/modules/moderation_input.go` `extractEntityText` | partial |
| `alita/modules/staff_action_card.go` (Redis card + Lua CAS, token, callbacks) | service/store | event-driven (callback) | `alita/modules/antiraid.go` lines 51-75 (`redis.NewScript`) + `staff_unlink.go` (issuer-bound confirm/cancel, `loadUnlinkTarget`) | role-match |
| `alita/modules/staff.go` (MODIFY: add `xc`/`xn` consts + switch cases in `staffCallback`) | controller | event-driven | itself, lines 44-53 and 247-266 | exact |
| `alita/modules/staff_action_decide.go` (pure decision table) | utility | transform | no code analog; use RESEARCH Q1 table | none |
| `alita/modules/staff_action_run.go` (coordinator, per-group worker, error classification, drain) | service | batch / fan-out | `staff_panel.go` `buildStaffPanelRows` (errgroup, panic-safe slots) + `staff_recheck.go` `recheckLink` + `staff_sweeper.go` (Start/Stop, WaitGroup) | role-match |
| `alita/modules/staff_action_summary.go` (renderer, batched editor, fallback) | utility/service | transform + streaming edit | `staff_panel.go` (`renderStaffPanel`, `staffPanelMaxUTF16`, `staffDisplayTitle`, `isMessageNotModified`) + `staff_notify.go` `sendStaffNotice` | role-match |
| `alita/utils/ratelimit/telegram_pacer.go` | utility | request-response (Redis) | `alita/utils/ratelimit/backup_ratelimit.go` + `antiraid.go` scripts | partial |
| `alita/db/user/repository.go` (MODIFY: `FindUsersByUsername`) | repository | CRUD read | `GetUserIdByUserName` lines 118-129 same file | exact |
| `alita/utils/extraction/extraction.go` (MODIFY: export `ParseDurationToken`) | utility | transform | `parseTemporaryDuration` lines 328-377 | exact |
| `alita/modules/main.go` (MODIFY `/home/user/Fuku_Robot/main.go`: shutdown drain for fan-out) | config | lifecycle | `main.go` lines 205-212 (`StopStaffSweeper`) | exact |
| `locales/{en,es,fr,hi,id,pt,ru}.yml` (`staff_act_*` keys) | config | n/a | existing `staff_*` keys, checked by `alita/i18n/staff_locale_test.go` | exact |
| `AGENTS.md` (MODIFY: document WrapCommand exception, staff action keys) | doc | n/a | existing Staff bullets | exact |
| `alita/modules/staff_action_fake_test.go` (stateful fake) | test | n/a | `alita/modules/staff_helpers_test.go` `staffBotClient` | exact (extend) |
| `alita/modules/staff_action_*_test.go` | test | n/a | `staff_unlink_test.go`, `staff_panel_test.go`, `antiraid_miniredis_test.go` `withMiniredis` | role-match |
| `alita/utils/ratelimit/telegram_pacer_test.go` | test | n/a | `backup_ratelimit_test.go` | role-match |
| `alita/db/user/repository_test.go` (MODIFY) | test | n/a | itself | exact |

## Pattern Assignments

### `alita/modules/staff_action.go` (handler, request-response)

**Analog:** `alita/modules/staff.go` (registration), `alita/modules/staff_watchers.go` (raw handlers). Deviation from AGENTS.md "commands go through WrapCommand" is intentional (RESEARCH Q4): WrapCommand calls `BuildCommandContext` first, which replies on sender-less updates, so it would double-reply in non-staff chats. Document in AGENTS.md.

**Registration pattern** (`staff.go` 363-377):
```go
func LoadStaff(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(staffModule.moduleName, true)
	helpers.WrapCommand(dispatcher, setStaffDesc, staffModule.setStaff)
	...
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("staff|"), staffModule.staffCallback))
}
func init() {
	RegisterLegacyModule("Staff", 236, LoadStaff)
```
Use `RegisterLegacyModule("StaffActions", 65, LoadStaffActions)`. Bans=70 (`bans.go:1129`), Mutes=80 (`mute.go:302`); 65 is unused. Register 13 raw `handlers.NewCommand(name, h)` at group 0 (default group; `dispatcher.AddHandler`).

**Pass-through gate** (copy shape from RESEARCH Pattern 1; zero side effects for non-staff chats):
```go
chat := ctx.EffectiveChat
if chat == nil || (chat.Type != "group" && chat.Type != "supergroup") || staff.GetStaffGroup(chat.Id) == nil {
	return ext.ContinueGroups
}
defer error_handling.RecoverFromPanic("staffEntry", "StaffActions")
```
Inside a Staff Group confirm with `staff.GetStaffGroupFresh`, always return `ext.EndGroups`.

**Anonymous refusal** (`helpers/command_pipeline.go` 289-313; reuse `helpers.IsAnonymousSender(ctx, user)` and the same responder call):
```go
if helpers.IsAnonymousSender(ctx, ctx.EffectiveSender.User) {
	chat_status.NewPermissionResponder(b).Respond(ctx, "staff_post_as_yourself", "", chat_status.WithReply())
	return ext.EndGroups
}
```

**Reply helper** (copy `replyStaff` from `staff.go` 61-66, but over `*gotgbot.Message`; or `replySelfDeleting` in `staff_notify.go` 32-46 for hints):
```go
if _, err := c.Msg.Reply(c.Bot, text, formatting.Shtml()); err != nil {
	log.Errorf("[Staff] reply: %v", err)
}
```

**Redis-required refusal:** guard `cache.GetRedisClient() == nil` (as `backup_ratelimit.go` 57) and reply with a translated message (RESEARCH Open Question 3).

---

### `alita/modules/staff.go` MODIFY (callback dispatch)

**Constants** (lines 44-53): add `staffActActConfirm = "xc"`, `staffActActCancel = "xn"` to the existing block. **Switch** (lines 247-266): add two cases before `default`, delegating to methods in `staff_action_card.go`:
```go
case staffActUnlinkCancel:
	return m.staffUnlinkCancel(b, query, tr, decoded.Fields)
// add: case staffActActConfirm: return m.staffActionConfirm(b, query, tr, decoded.Fields)
```
Every branch must answer the callback exactly once (`answerStaffCallback`, lines 269-274). Use `editStaffCallbackMessage` (278-285) for plain edits; the summary editor needs its own edit helper with `ReplyMarkup` omitted (removes the keyboard).

---

### `alita/modules/staff_action_card.go` (store + callbacks, event-driven)

**Analog 1 (Lua):** `antiraid.go` 51-75:
```go
deleteRaidStateScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		redis.call("DEL", KEYS[1], KEYS[2])
		return 1
	end
	return 0
`)
```
Copy the `redis.NewScript` package-var style for the `pending -> running|cancelled|expired` CAS (issuer and expiry compared inside the script). Keys: `alita:staff:act:<token>` (outside `alita:cache:`). Redis client via `cache.GetRedisClient()` and `cache.Context`.

**Analog 2 (issuer-bound callback, chat check)** `staff_unlink.go` 281-312: load state from the token alone, then compare `query.Message.GetChat().Id` with the stored staff chat:
```go
if query.Message.GetChat().Id != link.StaffChatID {
	text, _ := tr.GetString("staff_cb_denied")
	answerStaffCallback(b, query, text, true)
	return nil
}
```
Add `query.From.Id == card.issuer` inside the Lua script.

**Button construction** (`staff_unlink.go` 317-336; MUST refuse to send if `""`):
```go
confirmData := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUnlinkConfirm, "l": id})
if confirmData == "" || cancelData == "" { return "", keyboard, false }
confirmLabel, _ := tr.GetString("staff_btn_confirm")
cancelLabel, _ := tr.GetString("staff_btn_cancel")
keyboard.InlineKeyboard = [][]gotgbot.InlineKeyboardButton{{
	{Text: confirmLabel, CallbackData: confirmData},
	{Text: cancelLabel, CallbackData: cancelData},
}}
```
Reuse `staff_btn_confirm` / `staff_btn_cancel`. Data = `{"a":"xc","t":<16 hex from crypto/rand>}` (32 bytes).

**Expiry timer** (`staff_notify.go` 41-45):
```go
time.AfterFunc(staffSelfDeleteAfter, func() {
	defer error_handling.RecoverFromPanic("replySelfDeleting", "Staff")
	...
})
```
Use the same shape for the 5-minute `pending -> expired` CAS plus edit; a lazy path on tap covers restarts. Make the duration a package `var` (like `staffSelfDeleteAfter`) so tests can shorten it.

**Issuer still a staff member at Confirm:** `chat_status.IsUserInChatWithError(b, chat, userId)` (`chat_status.go:348`), then `staff.GetStaffGroupFresh(chatID)` (as `staffUnsetConfirm`, `staff.go` 289-300).

---

### `alita/modules/staff_action_run.go` (service, fan-out batch)

**Analog:** `staff_panel.go` `buildStaffPanelRows` (401-441). Copy errgroup + panic-safe slots:
```go
var group errgroup.Group
group.SetLimit(4)
for i, link := range links {
	group.Go(func() error {
		defer error_handling.RecoverFromPanic("buildStaffPanelRows", "Staff")
		...
		slots[i] = &row
		return nil
	})
}
_ = group.Wait()
```
Differences required by STAFF-08: pre-fill every result slot as `pending`; after `Wait()` convert any still-pending slot to `failed: internal error` (a nil slot must never mean "dropped"). Fan-out list: `staff.ListLinksByStaffFresh(staffChatID)` (`id ASC`, `staff_panel.go` 452).

**Per-group owner recheck** (`staff_recheck.go` 134): `recheckLink(b, link, pass)` with one shared `newStaffOwnerPass()`; map results: `staffRecheckRemoved`/`staffRecheckGone` -> skipped "link removed", `staffRecheckUnknown` -> failed, only `OwnerMismatch` removes (Phase 1 rule). Pacer seam: recheckLink calls `chat_status.CheckOwner` through `pass.check`; add a pacing hook (a func field on `staffOwnerPass` or a `pace` parameter) without changing existing callers.

**Live bot probe for classification** (`owner.go` 71-92): `chat_status.FetchBotMember(b, chatID)` returns `(MergedChatMember, BotMemberResult, error)`; Missing -> "group not found"; status != administrator -> "bot isn't an admin"; `!CanRestrictMembers` -> "bot lacks ban rights"; otherwise "Telegram error: <description>". Never string-match error text.

**Live per-group issuer check:** do NOT use `chat_status.IsUserAdmin`/`CanUserRestrict`. Use `b.GetChatMemberWithContext(ctx, groupID, issuerID, nil)` then `member.MergeChatMember()` (shape as `owner.go` 75-86); accept only `creator` or `administrator && CanRestrictMembers`. Bound each call with a context timeout like `liveCheckTimeout` (`owner.go` 39).

**Telegram write calls:** `b.BanChatMember`, `b.RestrictChatMember(chatID, userID, perms, &gotgbot.RestrictChatMemberOpts{UntilDate: ...})`, `b.UnbanChatMember(..., &gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true})`. Mute permissions: `MutedPermissions` (`chat_permissions.go` 10-25). Unmute: `resolveUnmutePermissions(chatInfo)` from `b.GetChat(groupID, nil)` (`chat_permissions.go` 47-53). Compute `until_date` once at Confirm time.

**Shutdown/drain** (`staff_sweeper.go` 83-109 pattern): package `sync.WaitGroup` + context + `StopStaffActions()`; register in `main.go` next to the sweeper, after the DB-close handler:
```go
shutdownManager.RegisterHandler(func() error {
	log.Info("[Shutdown] Stopping staff sweeper...")
	modules.StopStaffSweeper()
	return nil
})
```
Every goroutine starts with `defer error_handling.RecoverFromPanic("<fn>", "StaffActions")` (`error_handling.go:16`).

**Target lock:** `client.SetNX(cache.Context, key, owner, ttl)` as in `staff_sweeper.go` 171 (`alita:staff:lock:target:<id>`), with compare-and-delete Lua on release (same shape as `deleteRaidStateScript`).

---

### `alita/modules/staff_action_summary.go` (renderer + editor)

**Analog:** `staff_panel.go`.
- Length cap and UTF-16 fit (lines 34-37, 257): `fits := func(text string) bool { return len(utf16.Encode([]rune(text))) <= staffPanelMaxUTF16 }`; reuse `staffPanelMaxUTF16 = 3800`.
- Escaped titles: `staffDisplayTitle(title)` (line 78).
- Not-modified: `isMessageNotModified(err)` (line 466) treat as success.
- Fallback new message: `sendStaffNotice(b, staffChatID, text)` (`staff_notify.go` 51-60), returns error so callers can fall back.
- Splice user text after translation (Pitfall 7), as in `staff_recheck.go` 93-99:
```go
text, _ := tr.GetString("staff_notice_unlinked_group_owner_changed", i18n.TranslationParams{"group": staffGroupTitleToken})
text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
```
Use the Staff Group's translator: `staffChatTranslator(chatID)` (`staff_notify.go` 24-26) for any post-callback text; `ctxTr(ctx)` inside handlers.
Edit call shape (`staff.go` 279-282): `query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: text, ParseMode: formatting.HTML})`; for the coordinator edit by chat/message id with no `ReplyMarkup` so the keyboard is removed (verify in test, A11). Single coordinator goroutine; 2.5 s batching.

---

### `alita/utils/ratelimit/telegram_pacer.go` (utility, Redis)

**Analog:** `backup_ratelimit.go` (package, Redis acquisition, imports):
```go
import (
	"context"
	...
	log "github.com/sirupsen/logrus"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)
...
if client := cache.GetRedisClient(); client != nil {
	acquired, err := client.SetNX(cache.Context, cacheKey, time.Now().Unix(), cooldown).Result()
```
Script from RESEARCH Code Examples (reserve-slot Lua with `TIME`/`PTTL`) declared as `redis.NewScript` package var (antiraid style). `retryAfter(err)` uses `errors.As(err, &tgErr *gotgbot.TelegramError)`, `Code == 429`, `ResponseParams.RetryAfter`. Keys `alita:staff:pace:next`, `alita:staff:pace:block`. On Redis error mid-run fall back to a local interval and log at warn. Error type `ErrRateLimited` maps to the "rate limited, gave up after retries" line. Note `ratelimit` imports `alita/utils/cache`; keep it free of `alita/modules` imports.

---

### `alita/db/user/repository.go` MODIFY (repository, CRUD read)

**Analog** lines 118-129 (same file):
```go
func GetUserIdByUserName(username string) int64 {
	var userId int64
	err := db.DB.Model(&models.User{}).Select("user_id").Where("username = ?", username).Scan(&userId).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return 0 } else if err != nil {
		log.Errorf("[Database] GetUserIdByUserName: %v - %s", err, username)
		return 0
	}
```
New `FindUsersByUsername(username string, limit int) ([]models.User, error)`: `Where("LOWER(username) = LOWER(?)", username)`, `Order("last_activity DESC")`. Unlike the analog, RETURN the error (a lookup failure must not read as "never seen"). Model fields: `models.User{UserId, UserName, Name, LastActivity}` (`db/models/user.go` 8-11). Read path, no cache, so no `DeleteCache`. Tests go in `repository_test.go` using the package `testmain_test.go` SQLite fixture.

---

### `alita/utils/extraction/extraction.go` MODIFY (utility, transform)

**Analog** `parseTemporaryDuration` (328-377): grammar `m/h/d/w`, `TemporaryUntilDate(now, secs)` window 30 s..366 d (lines 38-50). Add exported pure `ParseDurationToken(token string, now int64) (until int64, label string, ok bool, overLimit bool)` (no replies; reuses the same switch). Staff parser maps `errTimeLimitExceeded` to permanent with label "permanent (longer than 366 days)" (Pitfall 10). Do not change `ExtractTime` behavior (STAFF-13). Refactor `parseTemporaryDuration` to call shared code only if existing tests stay green.

---

### Target parsing in `staff_action_parse.go`

**Entity text:** `extractEntityText(source, offset, length)` (`moderation_input.go` 89) is UTF-16 aware; match both `msg.Entities` and `msg.CaptionEntities`; get text with `msg.GetText()`. Do NOT call `extraction.GetUserId`/`ExtractUserAndText` (channel + live-Telegram fallbacks, exact-case). `text_mention` entity: `ent.User.Id` direct. Validate username `^[A-Za-z0-9_]{5,32}$`; numeric IDs > 0 (never chat IDs).

---

### Tests

**Fake client:** extend `staffBotClient` (`staff_helpers_test.go` 41-196). Existing shape: per `(chat,user)` status string in `members map[[2]int64]string`, scripted `failures map["method:chatID"]`, `record(method, params)`, only `getChatAdministrators`/`getChatMember` handled, all else delegates to `moduleBotClient`. `staffMemberJSON` (88-102) emits no `until_date`/`can_send_messages`/`is_member=false`: Phase 2 needs a new stateful fake (new file) that models `banChatMember`/`restrictChatMember`/`unbanChatMember` as status REPLACEMENT (research flag), per-call scripted 429 sequences (`gotgbot.TelegramError{Code:429, ResponseParams:&gotgbot.ResponseParameters{RetryAfter:N}}`), and `editMessageText` failure injection. Keep `//go:build testtools` and no mock libs.

**Locale in tests:** `withStaffLocale(t)` (`staff_helpers_test.go` 209+) swaps strings for `staffMarker(key)`; assert keys, not literals.

**Redis:** `withMiniredis(t)` (`antiraid_miniredis_test.go` 22-42) installs a miniredis-backed client via `cache.SetCacheState`; `cache.SetRedisClientForTest(t, client)` (`utils/cache/test_setup_testtools.go` 144) is the lighter form.

**Dispatcher harness for STAFF-13:** load `LoadBans`, `LoadMutes`, `LoadStaffActions` in registry order (`registry.go` sorts by priority), send the same `/ban` update to a normal and a Staff Group; precedent in `bans_command_test.go` / `mutes_command_test.go`.

**i18n:** `alita/i18n/staff_locale_test.go` already covers all `staff_`-prefixed keys across the 7 locales (`en es fr hi id pt ru`; `config.yml` is not a language). Name new keys `staff_act_*` and use literal `tr.GetString("staff_act_...")` so `make check-translations` sees them.

## Shared Patterns

### Fail-closed live authority
**Source:** `alita/utils/chat_status/owner.go` 94-120 (tri-state `CheckOwner`), 61-92 (`FetchBotMember`).
**Apply to:** every per-group check in `staff_action_run.go`. Unknown answer = failed/skipped line, never an action; only `OwnerMismatch` deletes a link.

### Callback encode/decode
**Source:** `alita/modules/callback_codec.go` (`encodeCallbackData` returns `""` on overflow), `staff.go` 236-246 (`decodeCallbackData`, `callbackQueryFromContext`, `answerInvalidCallback`).
**Apply to:** card buttons. Never `strings.Split`; never user text in callback data.

### Panic recovery on goroutines
**Source:** `staff_panel.go` 410, `staff_notify.go` 43: `defer error_handling.RecoverFromPanic("<fn>", "<Module>")`.
**Apply to:** coordinator, workers, expiry timer, pacer helpers.

### Translator + placeholders
**Source:** `staff_recheck.go` 93-99 and `staff.go` 55-59 (`staffGroupsToken`). Splice user text (reason, names, titles) after `GetString`, always `html.EscapeString`/`staffDisplayTitle`.
**Apply to:** card, summary, hints.

### Operational Redis keys
Keys under `alita:staff:*` (outside `alita:cache:`, per AGENTS.md): `alita:staff:act:<token>`, `alita:staff:pace:*`, `alita:staff:lock:target:<id>`. No DB writes in this phase, so no `cache.DeleteCache` is required beyond what `recheckLink` already does.

### Logging
`log.Warnf("[Staff] ...: %v", err)` / `log.Errorf("[Staff] ...")`; never log `ext.EndGroups` as an error.

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| `alita/modules/staff_action_decide.go` | utility | transform | Pure status-first decision table is new; use RESEARCH Q1 table and the invariants (restrict only to `member`/`restricted`; `unban(false)` only for kick; unban always `OnlyIfBanned: true`) |
| Slot-reservation Lua in `telegram_pacer.go` | utility | request-response | Only backup limiter exists (SETNX cooldown); use RESEARCH Code Examples script (miniredis-probed). `antiraid.go` supplies only the `redis.NewScript` style |

## Metadata

**Analog search scope:** `alita/modules`, `alita/utils/{chat_status,extraction,helpers,ratelimit,cache,error_handling}`, `alita/db/{user,staff,models}`, `main.go`, `locales`.
**Files scanned:** ~25 read (targeted ranges), all cited paths confirmed in `git ls-files`.
**Pattern extraction date:** 2026-10-05
