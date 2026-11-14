# Phase 12: Rules Media, Media Under Text and Greet Once - Pattern Map

**Mapped:** 2026-11-05
**Files analyzed:** 41 new/modified (code, tests, config, docs)
**Analogs found:** 38 / 41 (every analog path was checked with `git ls-files`; all are tracked source, no `.gsd` mirrors)

12-RESEARCH.md (S-01..S-07) already fixes names, shapes and decisions; 12-UI-SPEC.md fixes the page contract. This map names the closest tracked analog per file and the excerpt to copy. Line numbers are from the working tree on 2026-11-05. Where RESEARCH gives the full target code (sender skeleton, extraction, classifier, claim), the planner takes it from RESEARCH "Code Examples" and Pattern 1; this file adds the real code to copy from.

Read note: RESEARCH.md was read in full. UI-SPEC lines 740-819 (assumptions, decisions applied, planner notes) were read; the page mocks (lines 1-739) were not re-read, so the planner reads them for row layouts and the locale key table.

## Wave order the planner should follow

1. **Wave 0, plan 1: characterization test first** (`fuku/modules/rules_legacy_request_test.go`, written and committed green against UNTOUCHED code, before any edit to `rules.go`, `sender.go`, `greetings.go`).
2. Migration + models + repositories + sender + extraction (leaf packages, each with its own tests).
3. Rules builder/sender refactor (the characterization test must stay green).
4. Greet-once claim, then editor pages, grid, controls, checks, previews.
5. Locales, locale tests, backup, Makefile, AGENTS.md, generated docs.
6. Last, droppable: `last_msg_id` swap (S-02).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match |
|---|---|---|---|---|
| `fuku/modules/rules_legacy_request_test.go` (NEW, FIRST) | test (characterization) | request-response | `fuku/modules/rules_command_test.go` `TestSendRulesUsesPrivateButtonWhenEnabled` (62-93) | exact |
| `migrations/20261105120000_add_rules_media_and_greet_once.sql` | migration | batch | `migrations/20261029120000_add_raid_detection_ai.sql` (ADD COLUMN IF NOT EXISTS), `20261104120000_add_settings_hub_tables.sql` (CREATE TABLE IF NOT EXISTS) | exact |
| `fuku/db/migrations/rules_media_migration_test.go` | test | batch | `fuku/db/migrations/settings_hub_migration_test.go`; PostgreSQL half: `captcha_mode_test.go` | exact |
| `fuku/db/db.go` (`ANIMATION int = 9`) | config | - | self (lines 34-41 constants) | self |
| `fuku/db/models/greetings.go` (+`MediaBelow`, `Once`) | model | CRUD | self; `models/chat_settings_access.go` (no `default:` tag comment) | self |
| `fuku/db/models/rules.go` (+4 columns, `HasContent`) | model | CRUD | self; `models.ButtonArray` in `models/types.go` | self |
| `fuku/db/models/greeted_member.go` (NEW) | model | CRUD | `fuku/db/models/chat_settings_access.go` (TableName + comment style) | role-match |
| `fuku/db/greetings/greeted.go` (NEW: `ClaimGreeting`, `ReleaseGreeting`) | repository | CRUD (conditional insert) | `fuku/db/lockdown/joiners.go` `RecordJoin` (43-65) | exact |
| `fuku/db/greetings/repository.go` (+`SetWelcomeMediaBelow`, `SetGoodbyeMediaBelow`, `SetWelcomeOnce`, optional swap) | repository | CRUD | self: `SetWelcomeMediaOnly` (279-290) via `upsertGreetingSettings` (218-238) | exact |
| `fuku/db/rules/repository.go` (+`LoadRulesSettingsContext`, `SetRulesMediaOnly`, `SetRulesButtonsOnly`, `SetRulesMediaBelow`, `ClearRules`) | repository | CRUD | self: `LoadRulesContext` (45-58), `SetChatRules` (60-69) | exact |
| `fuku/db/lang/repository.go` (+`GetGroupLanguage`) | repository | request-response | self: `getGroupLanguageContext` (lines 53-97) | self |
| `fuku/utils/content/content.go` (+`extractGreetingMedia`) | utility | transform | self: `extractMediaFromReply` (118-140), `ExtractMedia` (234-236) | self |
| `fuku/utils/media/sender.go` (+`ANIMATION` case, `sendAnimation`, `CaptionAbove`, `CaptionAboveEligible`, `GreetingMessage`, `SendGreetingMessage`) | utility | request-response | self: `sendPhoto` (155-171), `SendGreeting` (329-346) | exact |
| `fuku/utils/formatting/formatting.go` (`{rules}` gate via `HasContent`) | utility | transform | self (318-321) | self |
| `fuku/modules/rules.go` (`buildRulesMessage`, `sendRulesMessage`, apply fns, `rulesWrites`, deep link) | handler/service | CRUD + request-response | self: `buildRulesText` (33-41), `sendRules` (156-218), `applyRulesText` (58-76) | self |
| `fuku/modules/greetings.go` (claim in `SendWelcomeMessage`, `welcomeDefinitelyNotSent`, apply fns, flag at 4 send sites) | handler | event-driven | self: `SendWelcomeMessage` (787-828), `applyWelcomeEnabled` (238-240), `greetingWrites` (165-178) | self |
| `fuku/modules/settings_edit.go` (parts, state, grid, controls, Main state) | component (page) | request-response | self: `settingsEditSwitchRows` (356-369), `settingsEditMainState` (404-) | self |
| `fuku/modules/settings_edit_check.go` (Rules arm removal, caption rule) | utility | transform | self (116-117 Rules arm; `settingsEditCheckRulesText`, `settingsEditCheckMedia`, `settingsEditCheckButtons`) | self |
| `fuku/modules/settings_edit_actions.go` (`settingsEditPromptMax`, switch handlers) | handler | request-response | self (56-64; `settingsEditApplyRemove` 196-226) | self |
| `fuku/modules/settings_input_door.go` (`apply` part switch, 442-444) | handler | event-driven | self | self |
| `fuku/modules/settings_edit_preview.go` (flag at preview sites, Rules builder, group language) | handler | request-response | self (80-91, `settingsEditPreviewRules`) | self |
| `fuku/modules/settings_menu.go` (`settingsCheckFields` control check, `settingsControl.Validate`) | middleware | request-response | self (262-266 `settingsActToggle, settingsActSet`) and `settingsPageAction.Validate` in `settings_pages.go` | self |
| `fuku/modules/settings_pages.go` (`settingsStateIcon`) | utility | transform | self: `settingsStateWord` (101-106), `settingsToggleButton` (327-338) | self |
| `locales/{en,es,fr,hi,id,pt,ru,uk}.yml` | config | - | existing `settings_edit_*`, `settings_btn_edit_*` keys in `locales/en.yml` | exact |
| `fuku/i18n/settings_locale_test.go` | test | - | self: `settingsLocaleLimits`, `TestSettingsLabelLengths` (384-463) | self |
| `fuku/utils/media/sender_test.go` | test | request-response | self (368 `SendGreeting` test) | exact |
| `fuku/utils/content/{content,editor_helpers}_test.go` | test | transform | self (`TestExtractMedia` 142-176) | self |
| `fuku/db/greetings/{greeted_test.go,testmain_test.go}` | test | CRUD | `fuku/db/greetings/testmain_test.go`; `fuku/db/lockdown` `TestStartLockdownOneActivePerChat` | exact |
| `fuku/db/rules/*_test.go`, `fuku/db/lang/repository_test.go`, `fuku/db/models` `TestHasContent` | test | CRUD | `fuku/db/rules/editor_repository_test.go` | exact |
| `fuku/db/backup/{testmain_test.go,roundtrip_test.go}` | test | CRUD | self (`TestAllModulesRoundTripEveryMeaningfulField`, lines 81, 121 literals) | self |
| `fuku/modules/test_harness_test.go` | test (harness) | setup | self (AutoMigrate list 337, 343) | self |
| `fuku/modules/settings_edit_env_test.go` (`editCleanup`) | test (harness) | setup | self (33-40) | self |
| `fuku/modules/*_test.go` listed in RESEARCH S-06 (edit, not new) | test | - | self | self |
| `fuku/modules/greet_once_test.go` (NEW) | test | event-driven | `fuku/modules/greetings_command_test.go` `TestMemberJoinAndLeaveSendConfiguredGreetings` (391-) | exact |
| `Makefile` (`test-postgres-integrity`) | config | - | self (39-44) | self |
| `AGENTS.md` | docs | - | existing bullets (S-07 lines) | self |

## Pattern Assignments

### 1. `fuku/modules/rules_legacy_request_test.go` (test, characterization, WRITE FIRST)

**Analog:** `fuku/modules/rules_command_test.go:62-93` (`TestSendRulesUsesPrivateButtonWhenEnabled`) for the fixtures; `fuku/modules/greetings_command_test.go:391-` for the join/leave greeting recording.

**Fixture + call pattern** (rules_command_test.go 62-80):
```go
client := newModuleBotClient()
bot := newModuleTestBot(client)
chatID := uniqueModuleChatID()
rules.SetChatRules(chatID, "Read before posting")
rules.SetPrivateRules(chatID, true)
rules.SetChatRulesButton(chatID, "Read rules")

chat := gotgbot.Chat{Id: chatID, Type: "supergroup", Title: "Rules Chat"}
user := gotgbot.User{Id: 42, FirstName: "Member"}
ctx := newModuleMessageContext(bot, chat, user, "/rules")
if err := rulesModule.sendRules(bot, ctx); err != ext.EndGroups { ... }
calls := client.callsFor("sendMessage")
replyMarkup, ok := calls[0].Params["reply_markup"].(gotgbot.InlineKeyboardMarkup)
```
**Record, per RESEARCH S-03a/Wave 0:** for an old row (text only, no media columns) assert the full `sendMessage` `Params` of (a) `/rules` with private rules off, (b) `/rules` with private rules on (click-to-read message and button), (c) the deep link (`rulesDeepLinkHandler`, `rules.go:369-429`, `msg.Reply` with `formatting.Shtml()`), (d) a legacy welcome and goodbye send with the flag off (no `show_caption_above_media` key in params, previews disabled, no `reply_parameters`), (e) an old document-GIF row sends `sendDocument`. Assert the observable keys that matter: `text`, `parse_mode`, `reply_parameters` (message_id, `allow_sending_without_reply` true for `/rules`, false for deep link), absence of `link_preview_options` (previews on) for `/rules`, `reply_markup` present only with buttons. Assert behavior (request parameters), not source text. Commit green before any refactor.

---

### 2. `migrations/20261105120000_add_rules_media_and_greet_once.sql` (migration)

**Analog:** `migrations/20261029120000_add_raid_detection_ai.sql:24` and `migrations/20261104120000_add_settings_hub_tables.sql`.

Use the RESEARCH S-04 SQL verbatim (seven `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ... NOT NULL DEFAULT`, one `CREATE TABLE IF NOT EXISTS greeted_members` with inline `PRIMARY KEY (chat_id, user_id)`). Rules: no `BEGIN`/`COMMIT`, no `ADD CONSTRAINT` (both appliers swallow `duplicate_object`), no `CONCURRENTLY`. Neither `fuku/db/migrations/runner.go` (reads `*.sql` generically, 153-158) nor `scripts/migrate_psql.sh` needs an edit; say so in the plan. `20261104120000` is the current highest; check again before planning if another phase merged.

**Test analog** (`fuku/db/migrations/settings_hub_migration_test.go:1-40`): copy `settingsHubStatements` as `rulesMediaStatements`:
```go
runner := &MigrationRunner{}
var run []string
for _, stmt := range runner.splitSQLStatements(cleanSupabaseSQL(string(content))) {
	if strings.TrimSpace(stmt) == "" || isTransactionControlStatement(stmt) {
		continue
	}
	run = append(run, stmt)
}
```
`TestRulesMediaMigrationStatements` asserts statement count, idempotence markers, no `ADD CONSTRAINT`, no transaction control, and the file name sorts last. `TestRulesMediaMigrationDefaults` (PostgreSQL only, rolled-back transaction) copies the shape of `TestCaptchaModeTurnstileConstraint` in `captcha_mode_test.go` (`tx := database.Begin()` ... rollback), after the chain step.

---

### 3. `fuku/db/models/{greetings,rules,greeted_member}.go` (models)

**Analog:** `fuku/db/models/rules.go` (whole file, 15 lines):
```go
type RulesSettings struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	ChatId    int64     `gorm:"column:chat_id;uniqueIndex;not null" json:"chat_id,omitempty"`
	Rules     string    `gorm:"column:rules;type:text" json:"rules,omitempty"`
	RulesBtn  string    `gorm:"column:rules_btn" json:"rules_btn,omitempty"`
	Private   bool      `gorm:"column:private;default:false" json:"private,omitempty"`
	...
}
func (RulesSettings) TableName() string { return "rules" }
```
Add (RESEARCH S-04, no `default:` tag on any new boolean): `FileID string gorm:"column:file_id" json:"file_id,omitempty"`, `MediaType int gorm:"column:media_type" json:"media_type,omitempty"`, `Buttons ButtonArray gorm:"column:btns;type:jsonb" json:"btns,omitempty"`, `MediaBelow bool gorm:"column:media_below" json:"media_below"`, and `func (r RulesSettings) HasContent() bool` = trimmed text non-empty, or `FileID != ""`, or `len(Buttons) > 0`. Note the existing `Private` has a `default:false` tag (a pre-existing wart); do not copy it.

`WelcomeSettings` / `GoodbyeSettings` in `models/greetings.go`: embedded with `embeddedPrefix:welcome_` / `goodbye_`; add `MediaBelow` and (welcome) `Once` with `gorm:"column:media_below"` / `"column:once"`. `GreetedMember` copies the `chat_settings_access.go` comment style plus `TableName() "greeted_members"`, `ChatID`/`UserID` with `gorm:"column:...;primaryKey;autoIncrement:false"`, `CreatedAt`.

---

### 4. `fuku/db/greetings/greeted.go` (repository, conditional insert)

**Analog:** `fuku/db/lockdown/joiners.go:43-65` (`RecordJoin`):
```go
result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
```
Copy for `ClaimGreeting(chatID, userID int64) (claimed bool, err error)`: build `models.GreetedMember{ChatID, UserID, CreatedAt: now}`, `result := db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)`, return `result.Error` wrapped with `fukuerrors.Wrapf` and `claimed = result.RowsAffected == 1`. `ReleaseGreeting` = `db.DB.Where("chat_id = ? AND user_id = ?", ...).Delete(&models.GreetedMember{})` (absent row is not an error). No cache, so no `cache.DeleteCache` (say so in the doc comment: read fresh, never cached).

**Test analog:** `fuku/db/greetings/testmain_test.go` is just
```go
os.Exit(testdb.Run(m, &models.GreetingSettings{}))
```
Change to add `&models.GreetedMember{}`. `TestGreetedMemberClaim` (SQLite: 8 goroutines, one winner, release then claim again, other chat independent) and `TestGreetedMemberClaimConcurrent` (PostgreSQL integrity list; same body). Per RESEARCH the SQLite scratch run proved `winners=1`.

---

### 5. `fuku/db/greetings/repository.go` (column-targeted writers)

**Analog:** `fuku/db/greetings/repository.go:279-290` (`SetWelcomeMediaOnly`) and `upsertGreetingSettings` (218-238):
```go
func SetWelcomeMediaOnly(chatID int64, fileID string, dataType int) error {
	updates := map[string]any{
		"welcome_file_id": fileID,
		"welcome_type":    dataType,
	}
	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetWelcomeMediaOnly]: %v", err)
		return err
	}
	...
```
Copy for `SetWelcomeMediaBelow(chatID, on)` -> `{"welcome_media_below": on}`, `SetGoodbyeMediaBelow` -> `goodbye_media_below`, `SetWelcomeOnce` -> `welcome_once`. `upsertGreetingSettings` already runs `cache.DeleteCache(cache.CacheKey("greetings", chatID))` (line 236); `Updates(map)` writes a Go `false`. Test `TestSetMediaBelowColumnTargeted`: other columns unchanged, `false` written. Optional last plan: `SwapCleanWelcomeMsgID` as a transaction with `clause.Locking{Strength: "UPDATE"}` (RESEARCH S-02; not a single `UPDATE ... RETURNING`).

---

### 6. `fuku/db/rules/repository.go` (loader + writers)

**Analog:** same file. Loader (45-58):
```go
func LoadRulesContext(ctx context.Context, chatID int64) (string, error) {
	rulesrc := &models.RulesSettings{}
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(rulesrc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fukuerrors.Wrapf(err, "load rules for chat %d", chatID)
	}
	return rulesrc.Rules, nil
}
```
Copy to `LoadRulesSettingsContext(ctx, chatID) (*models.RulesSettings, error)` returning a zero row on not-found; turn `LoadRulesContext` into a wrapper returning `.Rules` (about ten call sites keep compiling). Writer (60-69):
```go
func SetChatRules(chatId int64, rules string) error {
	if _, err := checkRulesSetting(chatId); err != nil { return err }
	err := db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatId}, map[string]any{"rules": rules})
	if err != nil { log.Errorf("[Database] SetChatRules: %v - %d", err, chatId) }
	return err
}
```
Copy for `SetRulesMediaOnly` (`file_id`, `media_type`), `SetRulesButtonsOnly` (`btns`: `models.ButtonArray(buttons)`), `SetRulesMediaBelow` (`media_below`), `ClearRules` (`rules ""`, `file_id ""`, `media_type 0`, `btns []`). Rules reads are uncached, so there is no key to delete; state it in each doc comment.

---

### 7. `fuku/utils/media/sender.go` (sender)

**Analog:** same file. `sendPhoto` (155-171) is the template for `sendAnimation` (RESEARCH "Sender additions" skeleton is correct as written). Add to `Content` (44-49) `CaptionAbove bool`; in `sendPhoto` and `sendVideo` add `ShowCaptionAboveMedia: content.CaptionAbove` (a false flag adds no parameter, `addIfValueNotZero`); never in `sendDocument`, `sendAudio`, `sendVoice`, `sendSticker`, `sendVideoNote` (no such opts field). Add `case db.ANIMATION: return sendAnimation(...)` to the switch (82-102) before `default`.

**Wrapper pattern** (`SendGreeting`, 329-346): keep as is, built on the new struct-based function:
```go
func SendGreeting(b *gotgbot.Bot, chatID int64, text, fileID string, msgType int, keyboard *gotgbot.InlineKeyboardMarkup, threadID int64) (*gotgbot.Message, error) {
	return Send(b, Content{Text: text, FileID: fileID, MsgType: msgType, Name: "greeting"},
		Options{ChatID: chatID, ReplyMsgID: 0, ThreadID: threadID, Keyboard: keyboard,
			WebPreview: false, AllowWithoutReply: true})
}
```
`SendGreetingMessage(b, chatID, GreetingMessage{Text, FileID, MsgType, CaptionAbove, Keyboard, ThreadID})` does the same with `CaptionAbove: msg.CaptionAbove && CaptionAboveEligible(msg.MsgType)`; `SendGreeting` calls it with false. `CaptionAboveEligible` in this package, the one predicate for page, sender and stale-press check. Tests (`TestSendGreetingMessageCaptionAbove`, `TestCaptionAboveEligible`, `TestSendAnimationUsesSendAnimation`) copy the existing `SendGreeting` test at `sender_test.go:368` (hand-written `gotgbot.BotClient` fake).

---

### 8. `fuku/utils/content/content.go` (extraction)

**Analog:** same file, `extractMediaFromReply` (118-140; `Document` branch at 124-125 comes before `Animation` at 134-135, which is why GIFs are documents) and `ExtractMedia` (234-236). Use the RESEARCH `extractGreetingMedia` (Animation first, else delegate to the unchanged legacy function); make `ExtractMedia` and `ExtractWelcome` (174) call it; leave `ExtractNoteAndFilter` (105) and `pins.go:595-620` alone. Test edits per RESEARCH S-06: `TestExtractMedia` animation cases flip to `db.ANIMATION` and the loop `ExtractMedia == extractMediaFromReply` excludes animation; `TestExtractMediaFromReply` stays; add `TestExtractGreetingMediaAnimation` and `TestExtractNoteAndFilterGifStaysDocument`.

---

### 9. `fuku/modules/rules.go` (builder, sender, apply functions)

**Analog:** same file.

**Builder to keep** (33-41): `buildRulesText` stays unchanged and is called by `buildRulesMessage(tr, title, row, form)`:
```go
func buildRulesText(tr *i18n.Translator, title, rawRules string) string {
	normalized := normalizeRulesForHTML(rawRules)
	if normalized == "" {
		text, _ := tr.GetString("rules_no_rules_set")
		return text
	}
	header, _ := tr.GetString("rules_for_chat_header")
	return fmt.Sprintf(header, html.EscapeString(title)) + "\n\n" + normalized
}
```
Two forms: group form (above, `rules_for_chat_header`) and private form (deep link, `rules_for_chat` via `tr.GetString("rules_for_chat", i18n.TranslationParams{"first": html.EscapeString(chatinfo.Title), "second": normalizedRules})`, `rules.go` ~369-429, with `rules_not_set` when empty). Heading-only caption (trailing newlines trimmed) when text empty but media or buttons exist.

**Legacy send to keep literally for `FileID == ""`** (202-211):
```go
_, err = msg.Reply(bot, Text,
	&gotgbot.SendMessageOpts{
		ReplyMarkup: rulesKb,
		ReplyParameters: &gotgbot.ReplyParameters{MessageId: replyMsgId, AllowSendingWithoutReply: true},
		ParseMode: formatting.HTML,
	},
)
```
(no `LinkPreviewOptions`, so previews stay on; do not route through `SendGreeting`, which turns them off). With media: `media.Send(Content{Text, FileID, MsgType, CaptionAbove}, Options{ChatID, ReplyMsgID, AllowWithoutReply, Keyboard})`. Keep the private-rules click-to-read branch (188-200) producing no media and no buttons. `/rules` uses `AllowSendingWithoutReply: true`; the deep link keeps false.

**Apply-function pattern** (50-76):
```go
var rulesWrites = struct {
	SetChatRules func(chatID int64, rules string) error
}{rules.SetChatRules}

func applyRulesText(ctx context.Context, chatID int64, html string) rulesApplyResult {
	return finishRulesApply(ctx, chatID, "set", rulesWrites.SetChatRules(chatID, html))
}
func finishRulesApply(ctx context.Context, chatID int64, what string, writeErr error) rulesApplyResult {
	stored, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) { log.Errorf("[Rules] Failed to %s rules for chat %d: %v", what, chatID, e) },
		func(e error) { log.Warnf("[Rules] read back rules of chat %d after %s: %v", chatID, what, e) },
		func(c context.Context) (string, error) { return rules.LoadRulesContext(c, chatID) })
	return rulesApplyResult{Outcome: outcome, Rules: stored, Err: err}
}
```
Extend `rulesWrites` with the four new writers; add `applyRulesMedia`, `applyRulesButtons`, `applyRulesMediaBelow`; give `rulesApplyResult` a `Row *models.RulesSettings` (keep `Rules`); read back through `LoadRulesSettingsContext`. `applyRulesClear` now means all three parts (`ClearRules`), used by `/clearrules` (`clearRules`, 79-104); Remove text must call `applyRulesText(ctx, id, "")` (text only).

---

### 10. `fuku/modules/greetings.go` (greet-once claim, apply functions, flag)

**Analog:** same file, `SendWelcomeMessage` (787-828). Insert the claim between the `ShouldWelcome` check (801) and `renderGreeting` (809); use RESEARCH Pattern 1 verbatim for the claim/release block and `welcomeDefinitelyNotSent` (it reuses `isTelegramClientError`, `settings_gate.go:256-262`, plus `media.ErrNoPermission` and code 429). Replace the send at 815:
```go
sent, err := media.SendGreeting(bot, chat.Id, res, greetPrefs.WelcomeSettings.FileID, greetPrefs.WelcomeSettings.WelcomeType, kb, threadID)
```
with `media.SendGreetingMessage(...)` carrying `CaptionAbove: WelcomeSettings.MediaBelow`; same at the leave handler (~889) and display sites (~404, ~411). The `CleanWelcome` block (820-825) stays after a successful send only.

**Apply pattern** (165-178, 226-240): add `WelcomeMediaBelow`, `GoodbyeMediaBelow`, `WelcomeOnce` to `greetingWrites` (the test seam) and
```go
func applyWelcomeEnabled(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetWelcomeToggle", greetingWrites.WelcomeToggle(chatID, on))
}
```
as the template for `applyWelcomeMediaBelow`, `applyGoodbyeMediaBelow`, `applyWelcomeOnce`. `applyGreetingReset` and Remove leave all three switches untouched (UI-SPEC A3).

**Join test template:** `greetings_command_test.go:391-420` (`newChatMemberContext`, `clearRecentJoinProcessing`, `greetingsModule.newMember`). `greet_once_test.go` copies it for `TestGreetOnceFirstJoinOnly`, `...WelcomeOffNoClaim`, `...OffNoRecord`, `...CaptchaPathAndFailure` (drive `finishCaptchaPass`), `...LockdownBanNoRecord`, `...WithoutRedis`, `...ConcurrentOneWelcome`, `...DefiniteRefusalReleases`, `...UnknownOutcomeKeeps`, `...ClaimErrorFailsClosed`. Never use `claimRecentJoinProcessing` for the record (Pitfall 10).

---

### 11. `fuku/modules/settings_edit.go` (parts, state, switch grid, controls)

**Analog:** same file.

**Switch block to replace** (356-369):
```go
func settingsEditSwitchRows(k *settingsKeyboard, def settingsEditPageDef, st settingsEditState) {
	if !def.isGreeting() { return }
	tr := k.view.Tr
	k.row(settingsToggleButton(k, def.Key, settingsEditControlEnabled,
		settingsText(tr, def.ToggleLabelKey, i18n.TranslationParams{"state": settingsStateWord(tr, st.Enabled)}),
		st.Enabled)...)
	k.row(settingsToggleButton(k, def.Key, settingsEditControlClean, ...)...)
}
```
Build a cell list (Welcome `e d u? o`, Goodbye `e d u?`, Rules `u?`), two cells per row left to right via `k.row`; `settingsToggleButton` (`settings_pages.go:327-338`) returns a one-button slice, so collect the buttons and pair them. Label `settingsStateIcon(on) + " " + name`; the body line carries `settingsStateWord`. Rules `Parts` becomes `{settingsEditRulesTextPart, settingsEditMediaPart, settingsEditButtonsPart}` (entries at lines 95-106 exist); `init()` (508-527) registers `Controls` per page (4/3/1); `settingsEditLoad` Rules arm (200-206) uses `LoadRulesSettingsContext`; `settingsEditMediaKey` (219-236) gets `case db.ANIMATION: return "settings_edit_media_animation"`; `settingsEditMainState` (404-423) Rules uses `HasContent()`. Row budget: 8 private, 9 group (worst case test `TestSettingsEditSwitchGrid`).

**Switch handler pattern:** `settingsEditApplyRemove` (`settings_edit_actions.go:196-226`) returns outcome plus `*settingsEditState`; generalise `settingsEditApplySwitch` (443-457) the same way so the Rules `u` fits. Stale `u` press writes nothing and redraws `settingsNoticeWarn(settingsText(tr, "settings_edit_below_unavailable"))`.

**Control validation hook** (`settings_menu.go:262-266`): in `settingsCheckFields`, for `settingsActToggle, settingsActSet`, additionally refuse when the page is registered and `Controls[fields["t"]]` is absent, and when a control's optional `Validate(v)` is false (editor switches: "0" or "1"). Mirror `settingsPageAction.Validate` (`settings_pages.go`, declared at registration). Tests: `TestSettingsControlValidate`, update `TestSettingsEditRegistry` (settings_edit_test.go:866) to 4/3/1.

---

### 12. `fuku/modules/settings_edit_check.go`, `settings_edit_actions.go`, `settings_input_door.go`

**Analog:** self; RESEARCH S-03 "The caption rule, one predicate" lists each edit exactly:
- Remove the arm `case in.Def.Kind == settingsEditRules: // Rules has a text part only.` (settings_edit_check.go:116-117).
- `settingsEditCheckRulesText`: with stored media, limit 1024 -> `settings_edit_refuse_text_rules_long`; no media > 4096 keeps `settingsEditTooLong` (old key).
- `settingsEditCheckMedia` for Rules: max over `settingsEditRulesHeadings(title)` (244-257, already measures both forms in all 8 languages) of `formatting.VisibleLength(heading + normalizeRulesForHTML(stored))` against 1024 -> `settings_edit_refuse_media_rules_long`; video note still `settings_edit_refuse_need_media`.
- `settingsEditCheckButtons` (307): pass `""` as the stored text for Rules.
- `settingsEditPromptMax` (`settings_edit_actions.go:56-64`): media set gives 1024 for Rules too, else 3800.
- Door `apply` (`settings_input_door.go:442-444`): replace `if r.def.Kind == settingsEditRules { return applyRulesText(...) }` with a part switch (`applyRulesText`, `applyRulesMedia`, `applyRulesButtons`). The line-269 bot-admin check is already generic.
Parity test `TestSettingsRulesCaptionParity` runs Welcome and Rules over every stored kind.

---

### 13. `fuku/modules/settings_edit_preview.go`

**Analog:** self (80-91, `settingsEditPreviewRules`). Only Full preview passes `CaptionAbove`; Rules Full preview and Preview use `buildRulesMessage` (group form) with `i18n.MustNewTranslator(lang.GetGroupLanguage(ctx, group.Id))`, keeping `p.tr` for `settings_edit_preview_header_rules`. Previews never call `SendWelcomeMessage` or `ClaimGreeting` (`TestSettingsEditPreviewNeverClaims`: assert no `greeted_members` row). Update the stale comment above `settingsEditPreviewRules`.

### 14. `fuku/db/lang/repository.go` (`GetGroupLanguage`)

**Analog:** same file, `getGroupLanguageContext` (called at line 53 as `return getGroupLanguageContext(tracing.UpdateContext(ctx), chat.Id)`; cached read at 61 `getCachedLanguage(ctx, cache.CacheKey("chat_lang", groupID), &models.Chat{}, "chat_id", groupID)`, `"uk"` fallback 80-97). Export `GetGroupLanguage(ctx, groupID) string` as exactly that call. Test `TestGetGroupLanguage` (fallback `uk`, creates no row) beside `repository_test.go`.

### 15. `fuku/utils/formatting/formatting.go` (`{rules}` gate)

**Analog:** self, 318-321: `rulesDb := rules.GetChatRulesInfo(chat.Id)` / `if rulesDb.Rules == "" { return res, btns }`. Change the test to `!rulesDb.HasContent()`. Test `TestFormattingRulesButtonMediaOnly`.

### 16. Locales and `fuku/i18n/settings_locale_test.go`

**Analog:** existing keys in `locales/en.yml` (`rules_for_chat` 1067, `rules_for_chat_header` 1271, `settings_edit_*`, `settings_btn_edit_*`) and the limits table in `fuku/i18n/settings_locale_test.go` (144-223 rows, 312 `utf8.RuneCountInString(text)` ceiling check, 384-463 `TestSettingsLabelLengths`).

Concrete edits (RESEARCH S-05 locales): 13 new keys, 4 renames (`settings_btn_edit_welcome_toggle` -> `settings_edit_line_welcome`, `..._goodbye_toggle` -> `settings_edit_line_goodbye`, `..._clean_welcome` -> `settings_edit_line_clean_welcome`, `..._clean_goodbye` -> `settings_edit_line_clean_goodbye`) in all 8 files plus their users (`settings_edit.go` 2 lines, `settings_edit_test.go` 10, `settings_edit_actions_test.go` 2, `settings_locale_test.go` 10), change `settings_edit_hint_rules`. Limits rows `{26, 26}` for the five switch keys (template as written). Replace the old four-key `editorSwitches` loop (lines 420-425, 454-463), which substitutes `icon + word`, by a rendered check that replaces `{state}` with a single icon (`✅`, then `❌`, no word) and fails above 20 code points. Extend `settingsEditLocaleTokens` for `{state}`, `{n}`/`{max}` and `<code>/rules</code>`. Total `settings_` keys must be 199 and the table must cover exactly that set. Run `make check-translations` before and after.

### 17. Backup (`fuku/db/backup`)

**Analog:** `types.go:181,196` (`GreetingsBackup = SettingBackup[models.GreetingSettings]`, `RulesBackup = SettingBackup[models.RulesSettings]`) and `backup.go:209-238` `replaceChatSetting` (builds the column map from `stmt.Schema.Fields`). No production edit: the new columns travel through the JSON tags on the models. `AllExportableModules` (`types.go:119-139`) lists names explicitly, so `greeted_members` stays out by construction. Edits are test-only: `testmain_test.go` AutoMigrate gains `&models.GreetedMember{}` (it uses `db.DB.AutoMigrate(` at line 41); `roundtrip_test.go` literals at lines 81 and 121 gain the new fields; new `TestOldGreetingsExportDecodesToZeros`, `TestGreetedMembersStayOutOfBackup` (export, import, reset leave rows alone).

### 18. Harness AutoMigrate lists

**Analog:** `fuku/modules/test_harness_test.go:337,343` (`&models.RulesSettings{}`, `&models.GreetingSettings{}`): add `&models.GreetedMember{}`. `fuku/db/greetings/testmain_test.go` as in section 4. `editCleanup` (`settings_edit_env_test.go:33-40`) also deletes `greeted_members` rows of its chats. Column additions need no list edits (AutoMigrate adds them).

### 19. `Makefile` `test-postgres-integrity`

**Analog:** same target (lines 39-44). The `-run` alternation is `$$`-anchored: append the real function names `TestGreetedMemberClaimConcurrent|TestRulesMediaMigrationDefaults` (and `TestSwapCleanMsgIDConcurrent` only if the swap plan is kept) inside the existing `^(...)$$`, and add `./fuku/db/greetings` to the package list (absent today; `./fuku/db/migrations` already there). Update the AGENTS.md Testing bullet list in the same commit.

### 20. `AGENTS.md`

**Analog:** the existing Phase 11 bullets (Callbacks `settings|v1` editor bullet, Data `fuku:settings:*`, Subsystem traps, Testing). Rewrite the sentences RESEARCH S-07 lists (GIF stored as a document for new GIFs; "Rules has `x` only"; the Rules Full preview heading in the presser's language) and add the new controls `u`/`o`, Rules parts, the new columns and `greeted_members` (outside backup, export, import, reset; read fresh; claim/release rule; chat-migration orphaning accepted), `ANIMATION`, the shared Rules builder, `lang.GetGroupLanguage`, the PostgreSQL list and harness lists. Same commit as the code.

## Shared Patterns

### Apply function, one setting one code path
**Source:** `fuku/modules/greetings.go:226-251` and `fuku/modules/rules.go:58-76`. **Apply to:** every new switch and Rules part (`applyWelcomeOnce`, `applyXMediaBelow`, `applyRulesMedia`, `applyRulesButtons`): write through a seam-struct writer, read back through the uncached loader, return a typed result. Menu, typed reply and any command call the same function.

### Column-targeted writes that store a Go zero value
**Source:** `greetings.upsertGreetingSettings` (`Updates(map)`) and `rules.SetChatRules` (`db.UpdateRecordWithZeroValues`). **Apply to:** all new writers. Models carry no `default:` tag on new booleans.

### Conditional insert for exactly-once
**Source:** `fuku/db/lockdown/joiners.go:65` `db.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)`. **Apply to:** `ClaimGreeting`; also the model for PostgreSQL-only CI proof (`TestStartLockdownOneActivePerChat` family in the Makefile list).

### Refusal classification by code, never wording
**Source:** `isTelegramClientError` (`fuku/modules/settings_gate.go:256-262`: `tgErr.Code >= 400 && tgErr.Code <= 499 && tgErr.Code != 429`, unwraps with `errors.As`) and `media.ErrNoPermission` (`sender.go:42`). **Apply to:** `welcomeDefinitelyNotSent` and the member-path `/rules` degrade to text-only on a definite 4xx (not in Full preview).

### Callback data
**Source:** `encodeCallbackData` / `fuku/utils/callbackcodec`, 64-byte cap, worst-case test `TestSettingsEditCallbackBytes` (largest 45 bytes, `Omitted` 0). **Apply to:** `t=u`, `t=o`, Rules `t=m`/`t=b`.

### Logging
Log IDs, part and outcome only; never text, captions, button names, links or file IDs (the `[Greetings]`, `[Rules]`, `[Media]` prefixes as in the excerpts).

### Test fixtures
Real fixtures only: `newModuleBotClient`, `newModuleTestBot`, `uniqueModuleChatID`, `newModuleMessageContext`, `newChatMemberContext`, the settings toolkit (`newSettingsEditEnv`, `editCleanup`, `openEditPage`, `press`), miniredis, hand-written `gotgbot.BotClient` fakes (`settingsEditFakeSends` already answers `sendAnimation`). Always `-tags testtools`; full run only through `make test`.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| (none fully without analog) | | | Every piece reuses a Phase 11 or earlier repo pattern |
| `settingsControl.Validate` hook | middleware | request-response | Partial: `settingsPageAction.Validate` exists, the control-level twin does not; copy that field's declaration and call site |
| Two-column switch cell grid | component | request-response | Partial: `settingsKeyboard.navRow` and the Main page two-column grid (`renderSettingsMain`) pair buttons, but no editor block does |
| Live-client spike `12-SPIKE.md` (owner's checklist) | doc | - | Telegram rendering needs a real client; RESEARCH S-01 table lists the items |

## Metadata

**Analog search scope:** `fuku/modules`, `fuku/db/{greetings,rules,lang,lockdown,backup,migrations,models}`, `fuku/utils/{media,content,formatting}`, `fuku/i18n`, `migrations/`, `Makefile`, `locales/`.
**Files scanned:** about 45 (read in part: `rules.go`, `sender.go`, `greetings.go`, repositories, models, test mains, Makefile target; the rest via RESEARCH.md line references).
**Pattern extraction date:** 2026-11-05
