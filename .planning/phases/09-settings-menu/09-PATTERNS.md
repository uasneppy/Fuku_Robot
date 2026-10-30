# Phase 9: Settings Menu - Pattern Map

**Mapped:** 2026-10-30
**Files analyzed:** about 30 (new and modified)
**Analogs found:** all, mostly exact. All analog paths below are git-tracked under `alita/` (checked for the callback, panel and deep-link files).

All paths are relative to `/home/user/Fuku_Robot/`. Line numbers are as read on 2026-10-30.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| NEW `alita/modules/settings.go` (module, `/settings` cmd, `settings\|` callback entry, deep link) | module/handler | request-response + event | `alita/modules/lockdown.go` + `lockdown_alert_callbacks.go` | exact |
| NEW `settings_codec.go` (fields, action codes, parse helpers) | utility | transform | `lockdown_alert_callbacks.go:111-146` (parsers), `lockdown_alert.go:57` (namespace const) | exact |
| NEW `settings_render.go` (pure render from snapshot, page registry table) | render | transform | `staff_panel.go` `renderStaffPanel` / `composeStaffPanelText` | exact |
| NEW `settings_staff.go` | page | CRUD + live checks | `staff_panel.go`, `staff_link.go`, `staff_unlink.go` | exact |
| NEW `settings_lockdown.go` | page | CRUD | `lockdown_alert_callbacks.go` (Lift ask/confirm), `autolockdown.go` | exact |
| NEW `settings_captcha.go`, `settings_antiflood.go` | page | CRUD | `captcha.go`, `antiflood.go` (command bodies) | role-match |
| NEW `settings_apply.go` (apply fns, typed results) | service | CRUD | `autolockdown.go:136` `writeAutoLockdown` + `parseAutoLockdownArgs` | role-match |
| EDIT `captcha.go`, `antiflood.go`, `autolockdown.go` | handler refactor | CRUD | themselves | n/a |
| EDIT `staff_link.go`, `staff_panel.go`, `lockdown.go`, `lockdown_alert_callbacks.go` | seam splits | n/a | themselves | n/a |
| EDIT `alita/utils/chat_status/owner.go:47` | utility | transform | export `isBotMissingError` -> `IsBotMissingError` | exact |
| NEW `alita/modules/settings_env_test.go` + `settings_*_test.go` | test | fixtures | `lockdown_fake_test.go` (`lockdownEnv`), `lockdown_alert_test.go`, `staff_action_fake_test.go` | exact |
| EDIT `alita/db/antiflood/repository_test.go` | test | CRUD | same file (SQLite open at lines 17-30) | exact |
| NEW `alita/i18n/settings_locale_test.go` | test | transform | `alita/i18n/captcha_web_locale_test.go`, `lockdown_locale_test.go` | exact |
| EDIT `locales/{en,es,fr,hi,id,pt,ru}.yml` | config | n/a | `captcha_web_*` keys (e.g. `en.yml:1041`) | exact |
| EDIT `locales/config.yml` (`alt_names.Settings`) | config | n/a | `config.yml:33-34` (`Staff:`, `Lockdown:` rows) | exact |
| Docs `docs/src/content/docs/commands/settings/*` via `make generate-docs` | docs | generated | `docs/.../commands/lockdown/` | exact |
| NEW `.planning/phases/09-settings-menu/09-SPIKE.md` | docs | n/a | `08-SPIKE.md` (title, "Why this exists", "Before you start", numbered checks with Pass/If not/Change) | exact |

## Pattern Assignments

### `settings.go` - command descriptor and registration (analog `lockdown.go`)

Descriptor with live-authority checks (`lockdown.go:60-67`):
```go
lockdownDesc = helpers.CommandDescriptor{
    Name:           "lockdown",
    RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireLockdownAuthority(true)},
}
```
For `/settings` use `requireLockdownGroup(), requireLockdownAuthority(false)` (D-29). `requireLockdownAuthority` is at `lockdown.go:131-172`. It handles the anonymous admin proof, `sender_chat`, then `lockdownLiveMember`. With `needRestrict=false` it admits creator or administrator only. Do not set `Disableable`.

Registration (`lockdown.go:550-581`):
```go
helpers.WrapCommand(dispatcher, lockdownDesc, pipelineHandler(lockdownModule.lockdown))
dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("lockdown|"), lockdownModule.lockdownAlertCallback).SetAllowChannel(true))
func init() {
    RegisterLegacyModule("Lockdown", 238, LoadLockdown)
    RegisterAnonymousAdminHandler("lockdown", anonPipelineHandler(lockdownDesc, lockdownModule.lockdown))
}
```
Use `callbackquery.Prefix("settings|")` without `SetAllowChannel` (menus are never on channel messages). Pick a unique priority and name `"Settings"`. Add `SetModuleEnabled(...)` at the top of `Load` as `LoadLockdown` does.

Refusals reply via `lockdownRefuse(c, key)` (`lockdown.go:75`), not `PermissionResponder`, because the anonymous-admin re-entry clears the callback from the update.

Live member lookup to reuse (`lockdown.go:108-122`): `lockdownLiveMember(b, chatID, userID)` with `lockdownLiveCheckTimeout`. Also `staffIssuerSkipReason(member)` returns `""`, `staffReasonSkipIssuerNoRight`, or other (`lockdown_alert_callbacks.go:165-179`).

Deep link (`deeplink_router.go:13-22`, handlers such as `rules.go:320`, `connections.go:362`):
```go
type DeepLinkHandler func(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error
RegisterDeepLinkHandler("settings_", settingsDeepLink)  // in init(); longest prefix wins
```
Chat ID parsing for `settings_<id>`: copy `parseStaffChatIDArg` (`staff_link.go:74-97`: optional minus, digits only, max 20 chars, negative result) and `parseStaffPickerPayload` (`staff_link.go:98`). Invalid payload: fall back to `sendDefaultHelp` (`deeplink_router.go:90`). Exact and prefix registries are in `HandleDeepLink` (`deeplink_router.go:27`). The group-chat variant `RegisterGroupDeepLinkHandler` (`deeplink_router.go:62`) is NOT needed.

### `settings.go` - callback entry and press pipeline (analog `lockdown_alert_callbacks.go:31-109`)

```go
func (m moduleStruct) lockdownAlertCallback(b *gotgbot.Bot, ctx *ext.Context) error {
    query, ok := callbackQueryFromContext(ctx)
    if !ok { return ext.EndGroups }
    decoded, ok := decodeCallbackData(query.Data, lockdownCallbackNamespace)
    if !ok || query.Message == nil { return answerInvalidCallback(b, ctx, query) }
    fields := decoded.Fields
    if staffServiceUserIDs[query.From.Id] {
        answerStaffCallback(b, query, lockdownText(fallback, "lockdown_alert_post_as_yourself"), true)
        return ext.EndGroups
    }
    ...
    switch fields["a"] { case lockdownActLiftAsk: ... }
    answerStaffCallback(b, query, lockdownText(tr, "lockdown_alert_expired"), false)
    return ext.EndGroups
}
```
Helpers (all in `package modules`): `callbackQueryFromContext` (`callback_context.go:8`), `decodeCallbackData` / `encodeCallbackData` (`callback_codec.go:15,35`; encode returns `""` on overflow, so a test must assert non-empty for every button), `answerInvalidCallback` (`callback_codec.go:51`, a toast only; D-30 wants a separate alert-sending helper), `answerStaffCallback(b, query, text, alert)` (`staff.go:301`), `editStaffCallbackMessage` (`staff.go:309`, plain HTML edit with keyboard removed; the menu needs its own edit-with-keyboard helper, treating "message is not modified" as success via `isMessageNotModified`, `staff_panel.go:506`).

D-30 order for settings: decode, structural check (`c` only if `query.Message.GetChat().Type == "private"`, `u` only if non-private), `staffServiceUserIDs`, live check (`lockdownLiveMember`) against the real chat or the verified `c`, then per-setting authority. Answer the callback exactly once. Per D-31 answer empty first, then work.

Bot-missing detection: `chat_status/owner.go:41-57` (`botMissingMarkers`, `isBotMissingError`); export it as `IsBotMissingError` (one-line rename plus update callers).

### Two-tap confirm with asker and expiry (Lift, Unlink) - analog `lockdown_alert_callbacks.go:214-321`

Ask builds the data (lines 229-236):
```go
asker := strconv.FormatInt(query.From.Id, 10)
expiry := strconv.FormatInt(time.Now().Add(lockdownCardLifetime).Unix()/60, 10)
confirmData := encodeCallbackData(lockdownCallbackNamespace, map[string]string{
    "a": lockdownActLiftConfirm, "r": copyID, "u": asker, "e": expiry,
})
if confirmData == "" || cancelData == "" { answerStaffCallback(..."check_failed", true); return ext.EndGroups }
```
Confirm checks (lines 264-320): `lockdownParseCardFields` (u and e positive 63-bit), `asker != query.From.Id` gives `lockdown_alert_not_yours`, lazy expiry `expiryMinute < time.Now().Unix()/60` edits to `lockdown_alert_card_expired`, re-reads state, re-runs `lockdownButtonAuthority`, then `liftLockdownRow(context.Background(), b, ld, query.From.Id, name)` with a switch on `res.Outcome` (`lockdownLiftRestoreFailed`, `lockdownLiftRecordFailed`, `lockdownLiftLost`). It ends with:
```go
lockdownPostLiftNotice(b, ld, lockdownLiftedText(staffChatTranslator(ld.ChatID), query.From.Id, name, res))
wakeLockdownWorker()
```
For the menu (D-26): the menu edits its own message in place instead of posting a card (the card here is a reply message). The group-menu variant already carries `u`; the private variant has no opener field, so `u` in the confirm data is still the presser (private chat user). Split the post-lift tail (`lockdownLiftedText` + `lockdownPostLiftNotice` + `wakeLockdownWorker`) into one shared function so the alert and the menu cannot drift (the "lift seam"). `lockdownPostLiftNotice` is at lines 704-720 (paced send, error only logged).

Authority per setting: `lockdownButtonAuthority(b, chatID, userID) (ok, refusalKey)` (`lockdown_alert_callbacks.go:165`) for Lift and trigger switches. Owner checks for Link and Unlink: `chat_status.CheckOwner(b, chatID, userID)` returning `OwnerMatch`, `OwnerUnknown`, etc. (use as in `staff_link.go:173-205`).

### `settings_staff.go` (analogs `staff_panel.go`, `staff_link.go`)

Split `renderStaffRow` (`staff_panel.go:162-195`) so the title and ID line is optional (D-21 masks it). Keep status/reason/lockdown lines:
```go
status, _ := tr.GetString("staff_panel_row_status", i18n.TranslationParams{
    "bot": staffStatusIcon(bot), "restrict": staffStatusIcon(restrict), "owner": staffStatusIcon(owner)})
sb.WriteString(staffDisplayTitle(row.Link.GroupTitle)); sb.WriteString(" <code>"); ... 
if !row.LockedSince.IsZero() { locked, _ := tr.GetString("staff_panel_row_lockdown", ...) }
```
`staffPanelTextFor(tr, v)` (`staff_panel.go:271-291`) measures UTF-16 against `staffPanelMaxUTF16`; add a `limit` parameter (D-29) and pass the constant from the existing callers. `renderStaffPanel` (297) comments say it is pure and reusable by Phase 9. Loaders: `buildStaffPanelRows` (419), `markLockedRows` (464, uses one `lockdown.ListActiveByChatsFresh`), `buildStaffPanel` (485). Use `staff.*Fresh` reads only.

Link flow split (D-29): `resolveOwnStaffGroup` (`staff_link.go:173-205`) lists candidates via `staff.ListStaffGroupsByOwner`, verifies each with `chat_status.CheckOwner` (`OwnerMatch` / `OwnerUnknown`, unknown fails closed). Extract `listVerifiedOwnStaffGroups` (returns matched list plus an unknown flag), `linkToStaffGroup` (the part of `runLinkGroup`, `staff_link.go:273-310`, after the staff group is resolved: `checkLinkTarget`, `FetchBotMember`, `storeLink`) and `announceStaffLink` (the tail of `deliverLinkOutcome`, 381). Refusal enum values: `staffRefusalNone`, `staffRefusalNoStaffGroup`, `staffRefusalNeedStaffID`, `staffRefusalCheckFailedInPlace`, `staffRefusalAnonymous`; refusal text via `staffLinkRefusalText(tr, r, groupTitle)` (328). Reuse these keys (D-23). Unlink: see `staff_unlink.go` and `staffUnlinkButton` (`staff_panel.go:392`); view it before planning, since it was not read in full here.

### `settings_lockdown.go` (analogs `autolockdown.go`, `lockdown.go`)

`autolockdown.go` pieces to reuse: `parseAutoLockdownArgs` (78), `writeAutoLockdown(chatID, change)` (136, which `autolockdownWrite` wraps), `autolockdownStatusText` (171), `autolockdownReadSettings` (256, runs under `raidSettingsTimeout`). The apply core of `autolockdownApply` (195-254) is the one to split: validation (supergroup check gives `lockdown_basic_group`, range gives `lockdown_auto_range` with `raiddetection.MinThreshold/MaxThreshold`, `errors.Is(writeErr, raiddetection.ErrThresholdOutOfRange)`, save failure gives `lockdown_auto_save_failed`) and write, returning a typed result that the command renders as `reply(...)`. The six `raiddetection.Set*` functions do their own `cache.DeleteCache`; key `raid_detection:*` is already in `skipLocal`.

Lockdown state page: fresh `lockdown.GetFresh` / an active-by-chat fresh read (see `lockdown.go` `lockdownStatus` ~line 294+ and `lockdownReasonLine`, `lockdownAlreadyActiveText` at 258-290; `lockdownTime` at 205). Reuse `lockdown_reason_line` (D-29), and the text helpers `lockdownText(tr, key, params...)` (249) and `lockdownSplice` (211) for post-translation escaping.

### `settings_captcha.go` and the `captcha.go` refactor

Gate (`captcha.go:445-462`): `captchaAdminGate(bot, ctx, requireBotAdmin)` does RequireUser, RequireGroup, RequireUserAdmin (cached), optional RequireBotAdmin, and replies through `PermissionResponder`. It cannot be reused by the menu (it reads `ctx`). Keep it for the commands; the menu's live equivalent is `lockdownLiveMember` plus bot-admin check (D-25: `/captcha` needs user and bot admin, mode/time/action need user only).

Time apply (`captcha.go:683-727`): `strconv.ParseInt(args[0],10,64)`, range 1 to 10, `captcha.SetCaptchaTimeout(chat.Id, timeout)`, `errors.Is(err, captcha.ErrInvalidTimeout)` gives `captcha_timeout_range_error`, else `captcha_timeout_failed`; success `captcha_timeout_set_success` with `{"d": timeout}`. Extract `applyCaptchaTimeout(chatID, n) (result)` and keep these keys. Mode: `captchaModeCommand` (587) and `captchaWebModeRefusal(bot, chat) string` (658, already pure on `bot, chat`, returns refusal text or `""`, in the three-step order). Mode write is `captcha.SetCaptchaMode` (accepts math, text, turnstile; invalidates `captcha_settings`). Look at `captchaCommand` (465) and `captchaActionCommand` (728) before planning (not read in full).

### `settings_antiflood.go` and the `antiflood.go` refactor

`setFlood` (`antiflood.go:421-473`): authority `chat_status.IsUserConnected(b, ctx, true, true)` (user admin and bot admin, refusal `connections_is_user_connected_bot_not_admin`); `off/no/false/0` gives `SetFloodContext(ctx, chat.Id, 0)` and `<module>_setflood_disabled`; integers outside 3..100 give `_errors_set_in_limit`; non-ints `_errors_invalid_int`; save errors give `common_settings_save_failed`; success `_setflood_success` formatted with `fmt.Sprintf(temp, num)`. Note keys are built as `strings.ToLower(m.moduleName)+"_..."`, so the apply result should carry the key name, not the module name. Others: `flood` (475), `setFloodMode` (514), `setFloodDeleter` (557). Page reads must use `GetAntifloodSettingsCachedContext` (D-30), same cache entry as the command; the apply functions read back the uncached row (D-27). Add a characterization test for the first-write side effect in `alita/db/antiflood/repository_test.go` (see the SQLite open with `gorm.Open(sqlite.Open(":memory:"))` + `utilsCache.SetupTestMemoryMarshaler(t)` pattern at lines 17-30).

### `settings_codec.go` (callback fields and byte table)

Namespace constant analog `lockdown_alert.go:57` (`lockdownCallbackNamespace = "lockdown"`). Action codes are 2-letter constants (`lockdownActLiftAsk` etc.; AGENTS.md lists `a=lk`, `bn`, `rk`, `lc`, `x`). Parsers to mirror: `lockdownParseCopyID`, `lockdownParseAsker`, `lockdownParseCardFields` (`lockdown_alert_callbacks.go:111-146`): `strconv.ParseInt(...,10,64)`, positive-only. For `c` (negative chat ID) write a separate parser with the same strict bounds as `parseStaffChatIDArg`. Format is `<ns>|v1|<url-encoded>` via `alita/utils/callbackcodec` (`Encode` at `callbackcodec/...:28`); never `strings.Split`. Add a table test that encodes the largest field set of every button (14-char chat ID, 19-digit values) and asserts `<= 64` bytes and non-empty.

### Tests (`settings_env_test.go`, `settings_*_test.go`)

Header pattern: `//go:build testtools`, `package modules`. Fixture: `lockdownEnv` (`lockdown_fake_test.go:273-286`: `t, fake *lockdownFake, bot, dispatcher, chat, admin, redis *miniredis.Miniredis`) with the hand-written `lockdownFake` BotClient (`RequestWithContext` at 173; helpers `setMemberRaw`, `lockdownAdminJSON(userID, canRestrict, canInvite)`, `lockdownBotAdminJSON`, `setChatType`, `setOnGetMember`). Build a `settingsEnv` that wraps or copies this; do not create a mock library. Cleanups: `lockdownCleanup(t, chatIDs...)` (290), `staffCleanup(t, ...)`, `raidCleanup`. Staff fixtures: `newLockdownStaffGroup(env)`, `linkLockdownStaffGroup(env)`, `linkLockdownGroupTo` (`lockdown_alert_test.go:21-60`). Press/refusal helpers to copy: `pressedLookups`, `snapshotAlert`, `wantRefused`, `lastCardEdit` (`lockdown_alert_auth_test.go:30-248`). The harness `AutoMigrate` lists are in `alita/modules/test_harness_test.go` (D-31 says no change needed). Assert observable behaviour only (rows, replies, cache), not literals.

Parity tests (D-19): drive the command through the dispatcher, then a menu press, and compare the stored rows and the `GetCaptchaSettings` / `GetAntifloodSettingsCachedContext` values; call `cache.ResetLocalForTest()` after direct writes.

### `alita/i18n/settings_locale_test.go`

Analog `captcha_web_locale_test.go:1-45`: a `map[string]captchaWebLimit{max, other}` table of per-key rune ceilings ("fails when it does not cover exactly" the key set), checks present, not empty, no markup, in all 7 locales. Also copy from `lockdown_locale_test.go`: `TestLockdownLocaleKeys` (45), `TestLockdownStringsHaveBalancedTags` (132), `TestLockdownRepliesCarryNoTelegramDetail` (174). Add a `TestSettingsHelpFitsOneMessage`-style test for `settings_help_msg` (analog `TestLockdownHelpFitsOneMessage`). `make check-translations` finds keys missing across locales.

### Locale and docs

Add `settings_*` keys to all of `locales/{en,es,fr,hi,id,pt,ru}.yml` (flat keys, double-quoted values as at `en.yml:1041`). Reuse existing keys wherever a command already has the text (D-23): `lockdown_*`, `captcha_*`, `antiflood_*`, `staff_*`, `connections_is_user_connected_bot_not_admin`. `locales/config.yml` `alt_names:` block, lines 6-34, one row per module: add `Settings: [settings]` beside `Staff:` and `Lockdown:` (the module name equals `RegisterLegacyModule` name). Generated docs live in `docs/src/content/docs/commands/<module>/`; run `make generate-docs` and `make check-docs`. The docs generator reads only `locales/en.yml`, and registered commands via `WrapCommand` (raw handlers are skipped).

### AGENTS.md

Add bullets under Callbacks (the `settings|v1` namespace, `c` vs `u` rule, authority per setting, two-tap with `e`), Handlers (`/settings` and the anonymous-admin re-entry, deep-link `settings_`), and Data/Permissions (apply functions are the one write path), in the same style as the existing lockdown bullets. Update in the same commit.

## Shared Patterns

### Escape after translate
`staffDisplayTitle` / `staffButtonTitle` (`staff_panel.go:83,92`) and `lockdownSplice` (`lockdown.go:211`): translate first, then splice HTML-escaped user strings. Never put a title through `tr.GetString` params.

### Missing i18n key
`tr.GetString` returns `""` for a typo'd key. Copy key names from `locales/en.yml`; the new locale test must iterate every key referenced.

### Live check, fail closed
`lockdownLiveMember` (`lockdown.go:108`) with a timeout; error -> refuse with a check_failed text (`lockdown_alert_check_failed` pattern); never the admin cache.

### Cache writes
Each setting write goes through its repository (`captcha.Set*`, `antiflood.Set*`, `raiddetection.Set*`) which already calls `cache.DeleteCache`; the menu adds no direct DB writes. `captcha_settings` and `antiflood` keys are not in `skipLocal` (D-28, accepted).

### Goroutines
Any async post after a press starts with `defer error_handling.RecoverFromPanic(...)`. Prefer to keep the menu synchronous (D-31 no new drains).

## No Analog Found

| File | Role | Reason |
|---|---|---|
| Stateless stepper buttons (absolute target value in `v`) | control | No existing stepper; design in `settings_codec.go` from the data-field pattern above |
| Edit-in-place menu with Back/Close, `deleteMessage` with edit fallback | UI | Existing cards are reply messages edited to a final text and never deleted (`editStaffCallbackMessage`); the closest edit-with-keyboard helper is `staffPanelRefresh` / `staffPanelRebuild` (`staff_panel.go:513-581`); read before planning |
| Page registry table (key, title key, render, handle) | registry | No such table; the nearest is the `RegisterLegacyModule` table and the staff command table of `StaffActions` |

## Metadata

**Analog search scope:** `alita/modules/`, `alita/utils/chat_status`, `alita/i18n`, `alita/db/antiflood`, `locales/`, `docs/`, `.planning/phases/08-*`
**Not fully read (read before planning):** `staff_unlink.go`, `captchaCommand`, `captchaModeCommand`, `captchaActionCommand`, `setFloodMode`, `setFloodDeleter`, `flood`, `lockdownStatus`, `help.go` registry entries (`help_system.go`)
**Pattern extraction date:** 2026-10-30
