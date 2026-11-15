# Phase 11: Message Editor - Pattern Map

**Mapped:** 2026-11-04
**Files analyzed:** 24 new/modified
**Analogs found:** 22 / 24 (every analog path was checked with `git ls-files`; all are tracked source)

11-RESEARCH.md (S-01..S-11) already fixes names, shapes and decisions. This map names the closest tracked analog per file and the excerpt to copy. Line numbers are from the working tree on 2026-11-04.

Read note: RESEARCH.md lines 1-449 and UI-SPEC lines 1-731 were read. Their tails (Code Examples, Validation Architecture, UI-SPEC locale rows 29+ and Notes for the planner) were not read; the planner should consult them for the exact Lua text (RESEARCH "Code Examples", line 475) and the locale key table (UI-SPEC lines 732+).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `fuku/modules/settings_edit.go` (part table, pure renderer, state read, register helper) | component (page) | request-response | `fuku/modules/settings_antiflood.go` (`renderSettingsAntiflood`, `init`) | exact |
| `fuku/modules/settings_edit_{welcome,goodbye,rules}.go` (thin `registerSettingsPage`) | component (page) | request-response | `fuku/modules/settings_antiflood.go` `init` (lines 43-57) and `settings_access.go` | exact |
| `fuku/modules/settings_edit_check.go` (pure validation + dry run) | utility | transform | `fuku/utils/content/content.go` `preFixes` (+ `raid_flood.go` slash predicate) | role-match |
| `fuku/modules/settings_edit_preview.go` (Preview, Full preview) | component (page actions) | request-response | `fuku/modules/greetings.go` join path (`SendWelcomeMessage`) + `media.SendGreeting` | role-match |
| `fuku/modules/settings_input.go` (wait store scripts, door, `/cancel`, `LoadSettingsInput`) | service + handler | event-driven (Redis CAS) | `fuku/utils/captchaweb/challenge.go` scripts (lines 238-311); `settings.go` `LoadSettings` (lines 230-238) for registration | role-match |
| `fuku/modules/settings_menu.go` (+ `settingsRightChangeInfo`, `Validate` hook line) | modify | request-response | self (`settingsRightRefusal` line 67+, `settingsCheckFields` ~line 225-262) | self |
| `fuku/modules/settings_pages.go` (+ `Validate` field on `settingsPageAction`) | modify | request-response | self (struct at ~line 366) | self |
| `fuku/modules/settings.go` (+ `LoadSettingsInput(dispatcher)` line) | modify | lifecycle | self (`LoadSettings`) | self |
| `fuku/modules/greetings.go` (apply functions, `renderGreeting`, commands call them) | modify | CRUD | `fuku/modules/captcha.go` `applyCaptchaTimeout` (797-831), `settings_access.go` `applyAdminsAllowed` (56-83) | exact |
| `fuku/modules/rules.go` (`applyRulesText/Clear`, `buildRulesText`) | modify | CRUD | same apply analogs; `rules.go:185-215` (`setRules`) | exact |
| `fuku/db/greetings/repository.go` (+ `LoadSettingsContext`, `Set{Welcome,Goodbye}{Text,Media,Buttons}Only`) | repository | CRUD | self: `upsertGreetingSettings` (160-180), `SetWelcomeText` (183-198) | exact |
| `fuku/db/rules/repository.go` (+ `LoadRulesContext`) | repository | CRUD read | self: `checkRulesSetting`/`SetChatRules` (30-52); `settingsFloodRead` seam pattern | role-match |
| `fuku/utils/content/content.go` (+ `ExtractMedia`, `TextLengthRefusal`, `ValidButtonURL`) | utility | transform | self (`extractMediaFromReply` 118-140, `preFixes` 200-239) | self |
| `fuku/utils/formatting/formatting.go` (+ `FillPlaceholders`, `VisibleLength`) | utility | transform | self (`FormattingReplacer` 218-320) | self |
| `locales/*.yml` (8 files, ~79 keys) | config | - | existing `settings_` keys in `locales/en.yml` | exact |
| `AGENTS.md`, generated docs | config | - | existing Handlers/Data bullets | exact |
| `fuku/modules/settings_edit_test.go`, `settings_input_test.go`, parity/gate tests | test | - | `settings_env_test.go` toolkit, `settings_gate_test.go` `TestSettingsGateAllDoors` | role-match |
| `fuku/modules/settings_env_test.go` (helpers for entities/media/albums in private) | test | setup | `sendIn`/`sendPrivate` (lines ~232-255) | self |
| `fuku/i18n/settings_locale_test.go` (rows for every new `settings_` key, alert keys) | test | - | self (`settingsLocaleLimits`, `settingsAlertKeys`) | self |
| `fuku/db/greetings/repository_test.go` (+ no-row default, column-targeted writes) | test | - | existing `repository_test.go:42-44` | self |
| `Makefile` / CI | config | - | none needed (no PostgreSQL guarantee, no migration) | n/a |
| Redis claim/finish/release/cancel Lua scripts | service | event-driven | `captchaweb/challenge.go` | role-match |
| Album gate (`SET NX EX 30`) | utility | event-driven | `raid_ai_image.go` album gate (`fuku:raid:{chat}:ag:`) per AGENTS | role-match (not read) |
| `/cancel` command | controller | request-response | `settings.go:233` `helpers.WrapCommand(dispatcher, settingsDesc, ...)` | role-match |
| Private-chat typed-reply door semantics | handler | event-driven | none for "private non-command consumer" | no analog |

## Pattern Assignments

### Page registration: `settings_edit_welcome.go` / `_goodbye.go` / `_rules.go`
**Analog:** `fuku/modules/settings_antiflood.go:43-57`
```go
func init() {
	registerSettingsPage(settingsPage{
		Key:       settingsPageAntiflood,
		Order:     40,
		Icon:      "🌊",
		TitleKey:  "settings_page_antiflood",
		MainState: settingsFloodMainState,
		Open:      settingsFloodOpen,
		Controls: map[string]settingsControl{
			settingsFloodLimitKey:  {Right: settingsRightAdminFloodBot, Set: settingsFloodSetLimit},
		},
	})
}
```
Copy for keys `w` (order 50), `g` (60), `r` (70), with `Actions: map[string]settingsPageAction{"ws": {Fields: []string{"t"}, Right: settingsRightChangeInfo, Run: ...}, ...}` for `ws wr wv wf wc` (and twins). `Controls` carries `e` (switch, `settingsRightAdmin`) and `d` (delete last, `settingsRightChangeInfo`) on Welcome and Goodbye. `registerSettingsPage` panics at init on duplicate key or action code (`settings_pages.go:~400-420`), so each page needs its own codes.

**Seam for reads** (copy, `settings_antiflood.go:41-42`): `var settingsFloodRead = antiflood.GetAntifloodSettingsCachedContext` with comment "a variable only so a test can make the read fail". Make `var settingsEditRead = greetings.LoadSettingsContext` and `var settingsEditRulesRead = rules.LoadRulesContext`.

**MainState** (copy shape, `settings_antiflood.go:60-71`): `context.WithTimeout(context.Background(), settingsReadTimeout)`, read, `if err != nil { return "", false }`, otherwise `settingsText(tr, "settings_state_...")`. Must not call `GetGreetingSettings` or `GetChatRulesInfo` (they create rows, RESEARCH S-05).

**Pure renderer + thin Open** (copy `renderSettingsAntiflood`, `settings_antiflood.go:121-154`):
```go
func renderSettingsAntiflood(v settingsView, s *models.AntifloodSettings, notice string) settingsScreen {
	var body []string
	body = append(body, settingsText(v.Tr, "settings_antiflood_limit", i18n.TranslationParams{"n": s.Limit}))
	state := settingsStateWord(v.Tr, s.DeleteAntifloodMessage)
	k := newSettingsKeyboard(v)
	k.row(settingsToggleButton(k, settingsPageAntiflood, settingsFloodDeleteKey, toggleLabel, s.DeleteAntifloodMessage)...)
	k.navRow(false)
	return k.screen(settingsCompose(v, "settings_page_antiflood", body, notice, false))
}
```
`renderSettingsEditor(v, st, notice)` is the same shape and takes no `settingsPress`, so the door can draw the saved page. Toggle buttons: `settingsToggleButton` (`settings_pages.go:~176`) sends the absolute target (`"0"`/`"1"`). Part buttons use `k.button(label, map[string]string{"a": "ws", "t": "x"})`; the owner field is added by the keyboard. A group menu adds a `URL` button built from `settingsPrivateURL(botUsername, chatID)` (`settings.go:48-50`). Header/group line comes from `settingsCompose` (`settings_pages.go:131-170`), which escapes the title via `staffDisplayTitle`.

### Pipeline hook: `settings_menu.go` / `settings_pages.go`
**Analog:** self. Current required-fields logic (`settings_menu.go:~232-262`):
```go
default:
	entry, ok := settingsActions[action]
	if !ok { return false }
	for _, key := range entry.action.Fields { allowed[key] = true }
```
Add one line after the loop: `if entry.action.Validate != nil && !entry.action.Validate(fields) { return false }` and the field `Validate func(fields map[string]string) bool` on `settingsPageAction` (`settings_pages.go:~366`, currently `Fields, Right, Run`). Rights: add `settingsRightChangeInfo` to the const block (after `settingsRightOwner`, `settings_menu.go:~60`) and a case in `settingsRightRefusal` modeled on `settingsRightRestrict`:
```go
case settingsRightRestrict:
	switch staffIssuerSkipReason(p.member) {
	case "": return "", false
	case staffReasonSkipIssuerNoRight: return "settings_no_restrict", true
	}
```
ChangeInfo: creator passes; administrator passes when `p.member.CanChangeInfo`; else `("settings_no_change_info", true)`. Factor a pure `settingsRightRefusalFor(member, right)` so the door shares it (RESEARCH S-06).

### Apply functions: `greetings.go`, `rules.go`
**Analog:** `fuku/modules/captcha.go:797-831` and `settings_access.go:56-83`.
```go
type captchaApplyResult struct { Outcome captchaApplyOutcome; Key string; Settings *models.CaptchaSettings; Err error }

func applyCaptchaTimeout(ctx context.Context, chatID int64, minutes int64) captchaApplyResult {
	if minutes < 1 || minutes > 10 { return captchaApplyResult{Outcome: captchaInvalid, Key: "captcha_timeout_invalid"} }
	if err := captcha.SetCaptchaTimeout(chatID, int(minutes)); err != nil { ... return captchaApplyResult{Outcome: captchaSaveFailed, Key: "...", Err: err} }
	result := captchaApplyResult{Outcome: captchaApplied}
	settings, err := captcha.GetCaptchaSettingsContext(ctx, chatID)
	if err != nil { log.Warnf("[Captcha] read back ..."); return result }
	result.Settings = settings
	return result
}
```
Create `applyWelcomeEnabled`, `applyGoodbyeEnabled`, `applyCleanWelcome`, `applyCleanGoodbye`, `applyGreetingText/Media/Buttons(isWelcome, ...)`, `applyRulesText`, and the Remove variants. Move the commands' repository calls into them (commands keep their replies). Existing command registration to leave unchanged: `greetings.go:1155-1172` (table loop through `helpers.WrapCommand(... CheckDisabled(cfg.name) ...)`). Rules commands: `setRules` at `rules.go:185-215` stores `rules.SetChatRules(chat.Id, tgmd2html.MD2HTMLV2(text))`.

### Repository: `fuku/db/greetings/repository.go`
**Analog:** self, `upsertGreetingSettings` (160-180) and `SetWelcomeText` (183-198).
```go
func upsertGreetingSettings(chatID int64, updates map[string]any) error {
	if !db.ChatExists(chatID) { if err := chats.EnsureChatInDb(chatID, ""); err != nil { return fukuerrors.Wrapf(...) } }
	updates["updated_at"] = time.Now()
	... FirstOrCreate(&settings) ... Updates(updates) ...
	cache.DeleteCache(cache.CacheKey("greetings", chatID))
	return nil
}
func SetWelcomeText(chatID int64, welcometxt, fileId string, buttons []models.Button, welcType int) error {
	updates := map[string]any{"welcome_text": welcometxt, "welcome_btns": models.ButtonArray(buttons), "welcome_type": welcType, "welcome_file_id": fileId}
	err := upsertGreetingSettings(chatID, updates)
	if err != nil { log.Errorf("[Database][SetWelcomeText]: %v", err); return err }
	return nil
}
```
Each new `Set*Only` is the same function with a one-column (or two-column for media: `welcome_file_id` + `welcome_type`) map, so `cache.DeleteCache` is inherited. `LoadSettingsContext(ctx, chatID)`: `db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(...)`, `errors.Is(err, gorm.ErrRecordNotFound)` returns runtime defaults without `Create` (copy the default-filling from `checkGreetingSettings`, lines 95-119). For `LoadRulesContext` copy the not-found branch of `fuku/db/rules/repository.go:~15-35` but without `EnsureChatInDb`/`CreateRecord`. Goodbye defaults to ON for an existing chat (RESEARCH S-05 and Pitfall 1); the UI-SPEC's "Goodbye Off" is wrong.

### Redis wait store: `settings_input.go`
**Analog:** `fuku/utils/captchaweb/challenge.go:238-311`.
```go
var finishScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'k') == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`)
var releaseScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'k') == ARGV[1] then return redis.call('HDEL', KEYS[1], 'k') end
return 0
`)
func (s *Store) settleClaim(ctx context.Context, script *redis.Script, key, nonce string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), RedisTimeout)
	defer cancel()
	_ = script.Run(ctx, s.client, []string{key}, nonce).Err()
}
```
Copy the claim-by-field / finish-with-nonce / release-with-nonce shape. The exact `setScript`, `claimScript`, `finishScript` (deletes wait and marker, two keys), `releaseScript`, `cancelScript` are given in RESEARCH "Code Examples" (line 475) and S-03. Use `cache.GetRedisClient()` (nil means "unavailable": Set answers `settings_edit_unavailable`, the door returns `ext.ContinueGroups`) and `cache.ContextWithTimeout()`. Keys: `fuku:settings:input:<user>` (hash, 10 min), `fuku:settings:inputdone:<user>` (15 min), `fuku:settings:inputalbum:<user>:<gid>` (30 s). Invariant: every deliberate end deletes both wait and marker.

### Door and registration: `settings_input.go`, `settings.go`
**Registration analog:** `fuku/modules/settings.go:230-238`
```go
func LoadSettings(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(settingsModule.moduleName, true)
	helpers.WrapCommand(dispatcher, settingsDesc, pipelineHandler(settingsModule.settings))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("settings|"), settingsModule.settingsCallback))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("shub|"), settingsModule.settingsHubCallback))
}
```
Add `LoadSettingsInput(dispatcher)` here so `settingsEnv.otherReplica()` (which calls `LoadSettings`, `settings_env_test.go:~128`) gets the door with no toolkit edit. `LoadSettingsInput` adds `dispatcher.AddHandlerToGroup(handlers.NewMessage(isPrivate, door), 1)` and `/cancel` via `helpers.WrapCommand`. Check `AddHandlerToGroup` usage in a sibling module (for example `LoadLockdown`'s group `-4`/`-7` registrations) for the exact call.

**`/cancel` analog:** `settings.go:40-43`
```go
var settingsDesc = helpers.CommandDescriptor{
	Name:           "settings",
	RequiredChecks: []helpers.CheckFunc{requireSettingsAccess()},
}
```
`/cancel` descriptor: no `Disableable`, one `RequiredChecks` entry that returns `false` silently outside a private chat (a CheckFunc that replies is wrong here; follow `requireSettingsAccess` for how a check is built).

**Slash predicate analog:** `raid_flood.go:165-177` (`raidFloodIsSlashText`). It reads `msg.Text` only; the door must read `msg.GetText()` and `msg.GetEntities()` because gotgbot's command handler treats a caption as a command (RESEARCH S-02, Pitfall 6).

**Live member:** `lockdown.go:114-126`
```go
func lockdownLiveMember(b *gotgbot.Bot, chatID, userID int64) (gotgbot.MergedChatMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lockdownLiveCheckTimeout)
	defer cancel()
	member, err := b.GetChatMemberWithContext(ctx, chatID, userID, nil)
	...
	return member.MergeChatMember(), nil
}
```
**Gate:** `settings_gate.go` `settingsGate(b, chatID, presser, member)` returns `settingsGateIn / settingsGateOwnerOnly / settingsGateUndecided`. 4xx classification: `isTelegramClientError(err)` (`settings_gate.go:256`). Service identities: `staffServiceUserIDs` (`staff_action_decide.go:513`). Panics: `defer error_handling.RecoverFromPanic("settingsInput", "Settings")`.

Return rules: no wait, no marker, or not a candidate returns `ext.ContinueGroups` (no reply/write/Telegram call); consumed (accepted, refused, ended, silenced album sibling) returns `ext.EndGroups`. Order of the 10 door steps is RESEARCH S-10.

### Dry run / validation: `settings_edit_check.go`, `content.go`, `formatting.go`
**Analog:** `fuku/utils/content/content.go` (`preFixes`, lines 200-239) and `FormattingReplacer` (`formatting.go:218-320`).
Stored form to reuse (content.go:160-172, 255-265):
```go
result.Text, _buttons = tgmd2html.MD2HTMLButtonsV2(rawText)   // Welcome/Goodbye: rawText = msg.OriginalMDV2()
tgmd2html.MD2HTMLV2(text)                                      // Rules (rules.go:~205)
```
Media: `extractMediaFromReply` (content.go:118-140; GIF stored as `db.DOCUMENT`); expose as `content.ExtractMedia`. Limits: `TextLengthRefusal` from the two `preFixes` checks (4096 text / 1024 caption on `utf8.RuneCountInString` of the stored HTML, keys `content_text_too_long`, `content_caption_too_long`). URL predicate: `ValidButtonURL` = `buttonUrlFixer` predicate (`err == nil && (scheme == "http" || "https") && host != ""`, content.go:220-229). `FillPlaceholders` is extracted from `FormattingReplacer` so `{count}` makes no Telegram call and `{rules}` no DB read (RESEARCH S-04, Pitfall 4). `VisibleLength` = `len(utf16.Encode([]rune(tgmd2html.StripHTMLV2(html))))`.

### Preview: `settings_edit_preview.go`, `greetings.go` `renderGreeting`
**Analog:** `media.SendGreeting` (`fuku/utils/media/sender.go:329-346`), which takes the target chat as an argument:
```go
func SendGreeting(b *gotgbot.Bot, chatID int64, text, fileID string, msgType int, keyboard *gotgbot.InlineKeyboardMarkup, threadID int64) (*gotgbot.Message, error)
```
Preview calls `SendGreeting(bot, presser.Id, ...)`. Do not call `SendWelcomeMessage` (it deletes the previous welcome and rewrites `LastMsgId`, `greetings.go:619-665`). Extract `renderGreeting(bot, group, user, text, buttons)` from the three-line sequence (`FormattingReplacer` + `keyboard.BuildKeyboard`) at `greetings.go:226-228`, `642-646`, `721-722` and make join, leave, `/welcome` and preview use it. Rules: extract `buildRulesText(tr, title, raw)` from `rules.go:129-142`. 403 classification: `errors.As(err, &tgErr)` with `Code == 403` gives `settings_edit_preview_need_start`; anything else `settings_edit_preview_failed` with the cause in the log only.

### Card/Remove: no confirmation card
Remove is one tap (D-09), so `settings_card.go` is NOT used. If the planner wants a card later, `settingsCardRows(k, yes, noPage)` is at `settings_card.go:66`.

### Tests
- **Toolkit (copy, do not edit page tests into it):** `settings_env_test.go` `sendIn` builds `gotgbot.Message{MessageId, Date, Chat, From, Text}` and runs `e.dispatcher.ProcessUpdate(e.bot, update, nil)` (lines ~232-255). It cannot send entities, media, captions or albums. Add new helpers in a new file (`settings_edit_env_test.go`): `sendPrivateEntities`, `sendPrivateMedia`, `sendPrivateAlbum` building the same `gotgbot.Update` with `Entities`, `Photo`, `MediaGroupId`. Per the AGENTS note the page tests call the toolkit by name and do not edit it, so prefer a new file; RESEARCH S-11 lists the gaps.
- **Two replicas:** `settingsEnv.otherReplica()` (`settings_env_test.go:~103-134`) builds a second fake, bot and dispatcher over the same miniredis and calls `LoadSettings`; with the door registered inside `LoadSettings` the Set-on-A / reply-on-B test needs no toolkit change.
- **Gate table:** add a "typed reply" door to `TestSettingsGateAllDoors` (`settings_gate_test.go:~998-1004`); `settingsGateClassify` (`~846-862`) already knows the `owner_only` and `check_failed` markers.
- **Redis TTL:** `env.redis.FastForward(10*time.Minute + time.Second)`; `env.redis.Exists`, `HGet`, `TTL` assert state.
- **Fakes:** `moduleBotClient` answers only `sendMessage/sendDocument/sendPhoto` with a Message JSON; add `fake.responses["sendSticker"|"sendVideo"|"sendAudio"|"sendVoice"|"sendAnimation"]` (Pitfall 14). Scripted failures: `setFailure(method, chatID, err)` (`staff_helpers_test.go:~100-104`); an administrator with `can_change_info`: add `settingsEditAdminJSON` beside `lockdown_fake_test.go:68-83`.
- **Parity tests:** call the real commands (`greetingsModule.setWelcome`, `rulesModule.setRules`) on chat A with `"/setwelcome " + text` and entity offsets shifted by the UTF-16 length of the command prefix, the editor on chat B with the unshifted message, compare stored `welcome_text/_type/_file_id/_btns`.
- **Locale tests:** `fuku/i18n/settings_locale_test.go` needs one limits row per new `settings_` key, alert keys appended to `settingsAlertKeys`, and label length rows for the three `icon + name` Main labels. Existing loops over `settingsPagesInOrder()` (`settings_main_test.go:218-229, 315`) now see three more pages; update `TestSettingsMainGrid` expectations (`[Welcome | Goodbye]`, `[Rules]` alone, `[Access]` last).
- **Test commands:** always `-tags testtools -race -count=1`; settings subset `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n`.

## Shared Patterns

### Callback data
**Source:** `fuku/utils/callbackcodec` via `encodeCallbackData` / `decodeCallbackData`, built by `settingsKeyboard.button`. **Apply to:** every new button. Worst case 45 bytes (the switches). A test renders every screen with `c=-1001234567890`, `u=1234567890` and asserts `settingsScreen.Omitted == 0` (`TestSettings*CallbackBytes`).

### Answer exactly once
**Source:** `settingsPress.answer(text, alert)` (`settings_menu.go:129`). No settings file calls `query.Answer`. **Apply to:** every action Run; the door is not a callback and never answers a query.

### Live authority, never cache
**Source:** `lockdownLiveMember` + `settingsGate`. **Apply to:** the door and every page press. The door re-checks the person live for each typed reply.

### Error handling and logging
**Source:** `fukuerrors.Wrapf(err, ...)` in repositories; `log.Warnf("[Category] ...")`. Never log message text, button names, URLs or file ids; log IDs, part and outcome only.

### i18n
**Source:** `settingsText(tr, "key", i18n.TranslationParams{...})` (`settings_pages.go:71`), invisible to `make check-translations`; `TestSettingsLocaleKeys` is the guard. All 8 locale files (`en es fr hi id pt ru uk`); AGENTS.md says "7" in two places and should be corrected. Unknown `{tokens}` such as `{first}` stay literal when params are passed.

### Privacy (D-21 / D-32)
**Source:** `TestSettingsStaffPrivacy`. **Apply to:** every new screen, prompt, refusal, alert and preview header, in a group menu and a private menu of a non-creator admin. The editor pages never read the Staff Group row.

### Goroutines
**Source:** `defer error_handling.RecoverFromPanic(name, cat)`. The door, `/cancel` and every press handler recover; the prompt-closing edit is synchronous (no goroutine).

### Docs to update in the same commit
AGENTS.md: Handlers (group `1`, the door, `/cancel`), Callbacks (new codes `ws wr wv wf wc gs gr gv gf gc rs rr rv rf rc`, new `Validate` hook, `settingsRightChangeInfo`), Data (three `fuku:settings:*` key families, uncached greetings/rules reads, column-targeted writes), Testing. Then `make generate-docs`, `make check-docs`, `make check-translations`, `make test`, `make lint`.

## No Analog Found

| File | Role | Data Flow | Reason |
|---|---|---|---|
| Typed-reply door (`settings_input.go` handler body) | handler | event-driven | No existing group-1 or "private non-command message consumer"; assemble from the pieces above (`captcha.go:2144-2146` private-chat pass-through and `raid_flood.go` watcher shape show the `ext.ContinueGroups` conventions) |
| Album first-wins gate | utility | event-driven | The raid AI album gate (`fuku:raid:{<chat>}:ag:<group id>`, `SET NX EX 120`, in `raid_ai_image.go`) is the closest analog; not read in this pass, planner should read it before writing the gate |

## Metadata

**Analog search scope:** `fuku/modules/{settings*,greetings,rules,captcha,lockdown,raid_flood}.go`, `fuku/utils/{captchaweb,content,formatting,media}`, `fuku/db/{greetings,rules}`, phase 10 PATTERNS.
**Files read this pass:** settings_antiflood.go (partial), settings_access.go (partial), settings_pages.go (partial), settings_menu.go (partial), settings_gate.go (partial), captchaweb/challenge.go (partial), captcha.go (partial), content.go (partial), greetings.go (partial), rules.go (partial), db/greetings/repository.go (partial), db/rules/repository.go (partial), media/sender.go (partial), settings_env_test.go (partial), lockdown.go (partial), raid_flood.go (partial).
**Pattern extraction date:** 2026-11-04
