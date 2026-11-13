# Phase 18: Remaining Settings Pages - Research

**Researched:** 2026-11-12
**Domain:** Go Telegram bot (gotgbot v2.0.0-rc.36, GORM on PostgreSQL / SQLite tests, Redis): six more pages on the settings menu framework of Phases 9 to 17 (Warnings, Switches, Language, Log channel, Anonymous admins, Locks), each writing through one apply function shared with its command
**Confidence:** HIGH for every code fact (each discrete value below was read this session and is quoted with file and line), MEDIUM for label widths on real phones and for the first-write side effects on PostgreSQL (listed as live checks)

Provenance tags: `[VERIFIED: file:line]` = file opened with Read (or printed whole) this session, value quoted verbatim beside the claim; `[VERIFIED: run]` = I ran it here; `[CITED: file]` = project planning text; `[ASSUMED]` = reasoning or estimate, listed in the Assumptions Log.

<user_constraints>
## User Constraints (from CONTEXT.md)

The decisions D-01 to D-14 are the owner's provisional defaults (CONTEXT "Provisional status": none confirmed, owner confirmations A18-1 to A18-6). Where the code disagrees with one, this file says so as a numbered correction C-nn with a recommended amendment and does not deviate silently.

### Locked Decisions

### One setting, one code path (area 1, PAGE-10)
- **D-01:** Every setting a page changes gets one apply function beside its command, which holds the validation, the refusals, the repository write and its `cache.DeleteCache`, and returns a typed result (outcome, the locale key the command replies with, the stored row read back, the repository error), as `applyCaptchaTimeout` set up in Phase 9: `applyWarnLimit`, `applyWarnMode`, `applyReportsEnabled`, `applyAISpamEnabled`, `applyAutoApprove`, `applyGroupLanguage`, `applyLogCategory`, `applyAnonAdmin` and `applyLock`. Each command is refactored to map the result to its unchanged reply, and a characterization test recorded BEFORE each extraction proves the command's requests and replies are byte-identical afterwards (never re-recorded). Pages read through the getter their command reads; a getter that hides read errors is never used by a page (the `settingsCaptchaRead` seam shape), and a failed read is the cannot-open edit.
- **D-02:** Each control declares the one `settingsRight` the matching command asks for, found by research per command (`/setwarnlimit` and `/setwarnmode` through `WrapCommand` with a handler-level check, `/reports`, `/aispam`, `/autoapprove`, `/lang`, the raw `/lock`, `/unlock`, `/log`, `/nolog`, `/logcategories` handlers with their own checks, and `/anonadmin`), and the menu refuses what the command refuses (an authority-parity test per setting, with the documented stricter-than-the-command carve-out of the live check). Opening a page needs `settingsRightAdmin` only. No new right is invented.
- **D-03:** Every page registers from its own file through `registerSettingsPage` (so `settings_pages.go` is not edited, apart from a framework hook only if research proves one is unavoidable), reads and writes through the owner gate and the shared press pipeline unchanged, uses absolute target values (`v`), and carries a drift test (the page reads what the command reads), an authority test and, where a command exists, a parity test (the menu press and the command leave the same stored row and the same cache), a worst-case callback test (64 bytes) and a length test in all 8 languages.

### Warnings, switches, language (area 2, PAGE-01 to PAGE-03)
- **D-04:** The Warnings page sets the warn limit with the shared stepper (bounds exactly as `/setwarnlimit` accepts, absolute `v`) and the action at the limit from the choices `/setwarnmode` accepts. A mode that needs a duration beyond a preset is shown read-only with its stored duration and changed only through the command (no typed input in this phase). Research settles the exact modes, bounds and how the warn settings row is created on a first write (`UpdateRecord` skips zero values: use the repository's zero-value-safe write).
- **D-05:** The Switches page has three controls: Reports, AI spam (`/aispam`) and Auto-approve join requests. AI spam carries the Phase 15 style status line when it cannot work (`ENABLE_AISPAM` off or no provider key: the page says so in words and offers Off only, never an error text, a key or a provider), read from `config.AppConfig` with no network call. Each control reads what its command reads.
- **D-06:** The Language page lists the 8 languages by their own name and sets the group's language through the same repository function as `/lang`. "The menu redraws in the new language" is judged for the menu opened in the group, which uses the group's language (the press's translator is rebuilt from the stored language after the write); a menu opened in the bot's private chat keeps the opener's own language and shows the group's current language in it. Research confirms how `ctxTr` and the private-chat translator choose their language and whether a language change needs any cache key dropped besides the language row.

### Log channel, anonymous admins, locks (area 3, PAGE-04 to PAGE-06)
- **D-07:** The Log channel page shows whether the group has a log channel and each category's on/off state, and turns a category on or off through `applyLogCategory` (the same write as `/log` and `/nolog`). It never links, unlinks or changes the channel (that stays `/setlog` and `/unsetlog`) and says so in one hint line. Privacy: the channel's title and ID are never shown in a group menu (it is readable by every member); the page shows a linked or not-linked state, and a private menu may show nothing more than that either (A18-3). With no channel linked the category switches are drawn and refused with a plain notice, as `/log` refuses.
- **D-08:** The Anonymous admins page sets the anonymous-admin mode through `applyAnonAdmin` with the rights `/anonadmin` needs today (PAGE-05 says "the same rights"); the research suggestion that it should be the owner only is NOT applied here and is recorded as owner confirmation A18-1, because making the menu stricter than the command would break "refused exactly as by its command" unless the command is changed too. The page explains in one line what the mode does.
- **D-09:** The Locks page lists the lock types from one table (code, label key, the lock kind the command uses) so a later lock is one row, paged at a fixed page size chosen against the 64-byte callback cap (Pitfall 28: a button carries the table code and the absolute value, never the lock's name) and the label limits in the longer languages (Pitfall 29). Each press is `applyLock` (the write `/lock` and `/unlock` make); the page shows the value `/locks` shows. The set of lock types is read from the code (`/locktypes`) by research, and a lock the command refuses in this chat (a type that needs a bot right, say) is drawn with its notice as the command words it.

### Menu shape and rights (area 4)
- **D-10:** The Main grid grows by six pages. Main keeps its two-column grid and status block (state lines from each page's `MainState`); research and the UI-SPEC settle the order (`Order`) of the new pages between Antiflood and Welcome or after Deletion, the keyboard row budget (a Main with sixteen cells is eight grid rows plus the owner row and the navigation row) and the 3800-unit text limit, and may group the six pages under a second hub screen only if the row budget cannot be met any other way.
- **D-11:** Pages are shown in a Staff Group exactly as the existing pages are (no `Hidden`), because these settings are meaningful for any group the bot administers, and `settingsGate` and the press pipeline apply unchanged; no screen names a Staff Group (D-21 of Phase 10).
- **D-12:** Strings: every new string exists in all 8 locales with a row in the limits table of `fuku/i18n/settings_locale_test.go`, labels held to the 20-code-point cell and 64-byte callback limits (`TestSettingsLabelLengths`), `TestSettingsStaffPrivacy` renders the new screens in all 8 languages, and `make check-translations` passes. The existing command strings are reused wherever a page states what a command states.

### Process (planning level)
- **D-13:** Plans are sequential and small (a tracer slice first: one apply function extracted with its characterization golden and one page press that writes through it, then the rest). Order: the characterization goldens and apply-function extractions first (one plan per module: warns, reports, aispam, autoapprove, language, log channels, anonadmin, locks, grouped sensibly), then the pages and their strings, then the Main grid and privacy sweeps, then the closing plan (docs, AGENTS.md, README, `18-SPIKE.md`, the gate, PAGE-01 to PAGE-06 and PAGE-10 ticked). Whole-package runs are required at each plan boundary that changes a module (the lesson of Phase 17).
- **D-14:** No migration, no new Redis key family, no new handler group and no shutdown change are expected; a command's cache keys are the ones its apply function already drops. `AGENTS.md`, the README, the generated docs and `18-SPIKE.md` are updated in the same commits as the code.

### Claude's Discretion
- The page keys, `Order` values, icons and labels, the lock page size, the exact warn modes shown, the order of the apply-function extraction plans, locale key names, and whether Switches and the Warnings page share a screen helper.

### Deferred Ideas (OUT OF SCOPE)
- The Blacklist, Filters and Notes pages (Phase 19) and the typed-input flows they need.
- Linking, unlinking or changing the log channel from the menu; a typed warn duration; per-lock exceptions or allowlists.
- Making the Anonymous admins page owner-only (needs the command stricter too).
- A second hub screen grouping the pages, unless the row budget forces it.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PAGE-01 | Warnings page: warn limit and action at the limit, stored as the warn commands store them | S-01 (command facts), S-02 (right), C-03 (modes: no duration exists), C-05 (reader), Plans 18-01, 18-02 |
| PAGE-02 | Switches page: reports, AI spam, auto-approve | S-01, S-06, C-02 (new right for auto-approve), C-08, C-09, C-10, Plans 18-03 to 18-05 |
| PAGE-03 | Language page, redraws in the new language | S-04, C-07, Plan 18-06 |
| PAGE-04 | Log channel page: categories on or off, linking stays a command | S-05, C-06, Plans 18-07, 18-08 |
| PAGE-05 | Anonymous admins page with the same rights as `/anonadmin` | S-02, C-01, C-02, Plan 18-09 |
| PAGE-06 | Locks page, paged | S-03, C-04, Plans 18-10, 18-11 |
| PAGE-10 | Every setting stored, cached and refused exactly as by its command | S-01 apply designs, S-08 parity/drift/authority tests, C-05, C-12 |
| PLAT-05 (bind) | New strings in 8 locales with limits tables | S-07 (tests that change), S-03 (22 lock labels), C-12 |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM, Redis; no new dependency; no mock library; tests use real fixtures (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes), always `-tags testtools`, `make test` is the only full run, no `t.Skip` to dodge a case. [VERIFIED: AGENTS.md "Testing", "Commands"]
- Every write `cache.DeleteCache`s the keys it affects; migrations append-only (none expected here); locale keys in all 8 files; commands through `helpers.WrapCommand` (the commands touched here keep their registration style, including the raw `helpers.NewBotCommand` ones); fire-and-forget goroutines recover from panics (none expected). [VERIFIED: AGENTS.md "Data", "Migrations", "Go rules"]
- One setting, one code path: one apply function per setting beside its command; menu rights mirror the command; the user check is live (stricter than the commands' cached one) and the bot check uses the commands' own predicate; the menu keeps no state (no Redis key, no migration, no drain). [VERIFIED: AGENTS.md "Subsystem traps" "One setting, one code path", "Permissions"]
- A menu opened in the group never names the Staff Group, and its text is readable by every member of the group. [VERIFIED: AGENTS.md "Subsystem traps"]
- Lint traps seen this milestone: gocyclo minimum 20 (`.golangci.yml`), dupl threshold 100, gosec G101 on identifiers containing token, secret or pass; `make lint` cannot run in this sandbox (golangci-lint 2.5.0 built with go1.25.1) so a stand-in (vet, gofmt, a manual dupl and gocyclo read) is used and `gosec` at `~/go/bin/gosec` can run. [VERIFIED: run, `golangci-lint --version`; .golangci.yml lines 12-21; 17-11-SUMMARY.md]

## Summary

Phase 18 needs **no new package, no new dependency, no migration, no new Redis key family, no new handler group and no shutdown change**. It is nine apply functions extracted from eight command files, six page files that register through `registerSettingsPage` exactly as Captcha and Antiflood do, new error-returning read helpers, two new `settingsRight` values and about 80 to 100 new `settings_` locale keys. `settings_pages.go` is not edited: the language redraw only needs `p.tr` to be replaced for a group menu, which a control's `Set` can do because `settingsPress` is in the same package. The Main grid with six more pages is 15 cells, 8 grid rows, the owner row and the navigation row (10 rows, 18 buttons) and about 1.2 to 1.4 thousand UTF-16 units against the 3800 limit, so no second hub screen is needed (C-11).

The decisions hold with **fourteen corrections** (C-01 to C-14). The ones that change the design: (1) `/anonadmin` is **already owner-only for every change** (`RequireUserOwner`), so A18-1's premise is inverted and the page needs the owner right plus the bot-administrator check, which no existing right combines (C-01, C-02); (2) "no new right is invented" cannot hold: `/autoapprove` needs change-info plus bot-admin and `/anonadmin` needs owner plus bot-admin (C-02); (3) the warn "duration" does not exist: the modes are exactly `ban`, `kick`, `mute`, a stored `tban`, `tmute` or empty value is possible (and `/reset warnings` stores the empty one) and then the limit does nothing (C-03); (4) there is no per-lock refusal and the 22 lock names fit a callback as they are (55 bytes worst case), so the table code is optional (C-04); (5) every getter the commands use either hides read errors or creates a row on read, so six new readers are needed that share the commands' cache keys (C-05); (6) the group language write lives in the **callback** `change_language`, not in `/lang`, and the same callback in a private chat writes the *user's* language, which the page must never reach (C-07).

**Primary recommendation:** build the tracer first (golden for `/setwarnlimit` and `/setwarnmode`, `applyWarnLimit`/`applyWarnMode`, `warns.LoadWarnSettingsContext`, and the Warnings page limit stepper writing through it with `settingsRightAdminFloodBot`), then Warnings complete, then Switches (with the two new rights), Language, Log channel, Anonymous admins, Locks, then the Main grid and privacy sweeps and the closing plan. Record each golden in its own commit before the extraction, use the lock's own name as the `t` value, and run the settings family plus the whole `./fuku/modules` package at every plan that registers a page.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Apply functions (validate, refuse, write, invalidate) | API / Backend (`fuku/modules` beside each command) | Database repositories (`fuku/db/<domain>`) | One write path for the command and the page (PAGE-10) |
| Error-returning readers | Database / Storage (`fuku/db/<domain>`) | Cache (`cache.GetFromCacheOrLoad`, same keys as the commands) | A page must not hide a read error or create a row |
| Pages, rights, press pipeline | API / Backend (settings framework) | Telegram client (inline keyboard edits) | Existing pipeline, unchanged except two rights |
| Language of the menu | API / Backend (`ctxTr`, `lang.GetLanguage`) | Cache (`chat_lang`, `user_lang`) | Group menu uses the group's language, private menu the presser's |
| "AI not configured" line | API / Backend (config read) | none | `config.AppConfig` only, no network |
| Locks enforcement | API / Backend (existing handlers at groups 5 and 6) | Cache (`locks_map`) | Unchanged; the page writes rows only |

## Standard Stack

### Core (all existing; no new dependency)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 | callbacks, `EditText`, `getChatMember` | the bot's client |
| `gorm.io/gorm` + postgres/sqlite drivers | v1.31.2 / v1.6.3 / v1.6.0 | reads and the existing upserts | the repositories already written |
| `github.com/redis/go-redis/v9`, `eko/gocache` | v9.22.0, v4.4.0 | the cached reads (`warn_settings`, `ai_spam_settings`, `locks_map`, `log_channel`, `chat_lang`) | existing |
| `fuku/utils/callbackcodec` | in repo | `settings|v1|...` data, 64-byte cap | `settingsKeyboard.button` |

**Installation:** none. **Package Legitimacy Audit:** no external package is added; packages removed (SLOP): none; flagged (SUS): none. The planner needs no `checkpoint:human-verify` install task.

## Corrections to CONTEXT.md (the planner must carry these)

| # | Where | Problem (evidence) | Recommended amendment |
|---|-------|--------------------|-----------------------|
| C-01 | D-08, A18-1, ROADMAP flag "the research suggests the owner", Pitfall 26 | The premise "making the menu stricter than the command" is false: `/anonadmin` is already owner-only for changes. `anonAdmin` does `if !chat_status.RequireUserOwner(c.Bot, c.Ctx, nil, user.Id) {` before any `on`/`off` write [VERIFIED: admin.go:603], and `RequireUserOwner` is a **live** `mem.GetStatus() == "creator"` check [VERIFIED: access.go:182-193]. Only the read-only status (no argument) is open to any group member. `anonAdminDesc` carries `helpers.RequireGroup(), helpers.RequireBotAdmin(),` and no user check [VERIFIED: admin.go:754-760]. | "The same rights `/anonadmin` needs" IS the owner right. The control declares a new composite right `settingsRightOwnerBotAdmin` (live creator on the press's own member record, then the commands' cached `IsBotAdmin`). The page opens for any admin who passed the gate (as the command's status view is open) and is **not** `OwnerOnly`. A18-1 becomes: "the page writes for the creator only, exactly as the command" (confirm, no conflict). Refusal alert for a non-owner: the command's own `chat_status_owner_button_error` ("You need to be the group creator to do this!", locales/en.yml:1574) |
| C-02 | D-02 "No new right is invented" | Two commands need a bot check **and** a user right that no existing value combines. `/autoapprove` runs `chat_status.IsUserConnected(bot, ctx, true, cfg.connectBotAdmin)` with `connectBotAdmin: true` [VERIFIED: greetings.go:826, 815] and then `chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id)` [VERIFIED: greetings.go:838]; `IsUserConnected(.., true, true)` refuses with `connections_is_user_connected_bot_not_admin` when `!IsUserAdmin(b, chat.Id, b.Id)` [VERIFIED: chat_status.go:486-490]. The Deletion page set the precedent of dropping the bot check for `/cleanservice` (`settingsRightChangeInfo` alone), but that leaves the menu writing where the command refuses, which PAGE-10 forbids here. | Append two values after `settingsRightChangeInfo` in `settings_menu.go` (zero value stays `settingsRightAdmin`): `settingsRightChangeInfoBotAdmin` (member part = `settingsRightChangeInfo`'s, bot part = `chat_status.IsUserAdmin(b, group, b.Id)` with the notice `connections_is_user_connected_bot_not_admin`, as `settingsRightAdminFloodBot`) and `settingsRightOwnerBotAdmin` (C-01; bot part = `chat_status.IsBotAdmin`, notice `chat_status_bot_not_admin`, as `settingsRightAdminBotAdmin`). Evaluate the bot part **first** for both (the command order: WrapCommand/IsUserConnected bot checks precede the user right) and pin both refusal orders in a test. `settingsRightRefusalFor` (pure, shared with the typed-reply door) gains the two member parts; `TestSettingsRightRefusalForParity` and `TestSettingsRights` gain the cases. All other controls reuse existing rights (table in S-02) |
| C-03 | D-04, A18-2 "a mode that needs a duration" | There is no duration anywhere. `/setwarnmode` accepts exactly `ban`, `kick`, `mute` (`modeKeys := map[string]string{"ban": ..., "kick": ..., "mute": ...}` [VERIFIED: warns.go:68-72]) and `WarnSettings` has only `warn_limit` and `warn_mode` [VERIFIED: models/warns.go:8-9]. The CHECK is wider than the code: `warn_mode = '' OR warn_mode IN ('ban','kick','mute','tban','tmute')` [VERIFIED: models/warns.go:9], and `applyWarnLimitAction` handles only `kick`, `mute`, `ban`; every other value hits `log.Warnf("[Warns] Unknown warn mode: %s", mode)` and `return false, nil`, so **the limit applies nothing and the warns are not reset** [VERIFIED: warns.go:153-179]. The empty value is reachable: `/reset warnings` stores `&models.WarnSettings{ChatId: chatID, WarnLimit: 3}` (mode empty) [VERIFIED: backup.go:1072] (a pre-existing defect: out of scope, recorded in Open Questions). | Replace the duration wording by: the page offers `[Ban | Kick | Mute]` (the three words `/setwarnmode` accepts); a stored `tban`, `tmute` or empty value is shown read-only with a note that no action is applied at the limit and no choice is marked, exactly as the Antiflood page does for a stored `warn`, `tban` or `tmute` (`settingsFloodSettable`, `settings_antiflood_action_note` [VERIFIED: settings_antiflood.go:109-117, 131-133]). A18-2 is moot; reword it as "a stored mode other than ban, kick or mute is shown read-only" |
| C-04 | D-09 "a lock the command refuses in this chat is drawn with its notice"; "a button carries the table code ... never the lock's name" | (a) Neither `/lock` nor `/unlock` refuses any single type: the only refusals are the global ones (bot administrator, user administrator) and an unknown name, which refuses the **whole** call before any write (`if !slices.Contains(m.getLockMapAsArray(), perm) {` then reply and `return ext.EndGroups`, nothing locked yet) [VERIFIED: locks.go:220-234]. The `bots` lock's bot-right need (`CanBotRestrict`) is checked when a bot joins, not when the lock is written [VERIFIED: locks.go:496-499]. (b) There are **22 types**: `lockMap` holds `sticker, audio, voice, document, video, videonote, contact, photo, gif, url, bots, forward, game, location, rtl, anonchannel` (16) and `restrMap` holds `messages, comments, media, other, previews, all` (6) [VERIFIED: locks.go:59-92]; `/locktypes` prints their sorted union [VERIFIED: locks.go:98-118]. (c) The longest name is `anonchannel` (11): the worst-case toggle `settings|v1|a=tg&c=-1001234567890&p=k&t=anonchannel&v=1` is 12 + 43 = 55 bytes, under 64 [VERIFIED: callbackcodec.go:34-48 format `"%s|%s|%s"`, sorted `url.Values.Encode`; arithmetic mine]. | Drop the per-type notice (the page's refusals are the control's right, as for every other page). The control key `t` is the lock's own name, registered from one table (one control per row, as the Deletion page registers one per row); a second code table adds a second identifier to keep in sync and buys nothing under the cap. The table is checked against the command's source of truth by `TestSettingsLocksTableMatchesCommand` (set equality with `locksModule.getLockMapAsArray()`), so a lock added to `lockMap` or `restrMap` without a row fails the build. A two-letter code is an acceptable alternative if the UI-SPEC wants it |
| C-05 | D-01 "Pages read through the getter their command reads" and D-05 "Each control reads what its command reads" | Every getter a command uses hides read errors or creates a row, so none can serve a page: `warns.GetWarnSettingContext` falls back to `&models.WarnSettings{ChatId: chatId, WarnLimit: 3, WarnMode: "mute"}` on error and its loader inserts a row (`checkWarnSettingsContext` ... `.Create(warnrc)`) [VERIFIED: warns/repository.go:24-52, 296-309]; `reports.GetChatReportSettings` calls `chats.EnsureChatInDb` and `db.CreateRecord` and returns defaults on error [VERIFIED: reports/repository.go:17-36]; `admin.GetAdminSettings` creates a row and returns `AnonAdmin: false` on error [VERIFIED: admin/repository.go:16-31]; `aispam.IsAISpamEnabled` logs and returns `false` [VERIFIED: aispam/repository.go:22-29]; `logchannels.GetContext` returns `nil` for both "not linked" and "read failed" [VERIFIED: logchannels/repository.go:47-63]; `lang.getCachedLanguage` returns `"uk"` on error [VERIFIED: lang/repository.go:80-102]. Exceptions that already return errors: `locks.GetChatLocksCachedContext(ctx, chatID) (map[string]bool, error)` [VERIFIED: locks/optimized.go:51-63] and `greetings.LoadSettingsContext` (fresh, creates nothing, the Welcome and Rules pages' reader) [VERIFIED: greetings/repository.go:149-175]. | Add six readers, each sharing the command's cache key and TTL (so page and command always agree, and a write's existing `DeleteCache` covers them), creating nothing and returning the error: `warns.LoadWarnSettingsContext`, `reports.LoadChatStatusContext` (uncached today: a plain `SELECT`, no row means the model default `true`), `aispam.LoadEnabledContext`, `admin.LoadAnonAdminContext` (uncached), `logchannels.LoadContext(ctx, chat) (*LogChannel, error)` (nil row with a nil error is "not linked"), `lang.LoadGroupLanguageContext`. Each is exposed to the page as a package variable seam (`settingsCaptchaRead` shape) so a test can make it fail. The page for auto-approve and the Locks page use the two that exist. Each reader gets a repository test (no row is created, an error is returned, a second read is served from cache after a write drops it) |
| C-06 | D-01 `applyLogCategory`, D-07 | `/log` and `/nolog` take **several** categories per call, validate each, treat a write error `gorm.ErrRecordNotFound` as "not linked" (reply and stop) but **any other write error is only logged and the loop continues** (`log.Error(err); continue`), so a total failure replies `logs_enabled` with an empty list [VERIFIED: logchannels.go:177-205]; before the loop the command refuses with `logs_not_set` when `logchannels.GetContext(...) == nil` and with `logs_usage` for no argument [VERIFIED: logchannels.go:164-173]; after the reply it posts an audit line `actionlog.Settings(b, chat, from, key+" "+strings.Join(changed, ", "))` into the log channel [VERIFIED: logchannels.go:207]. `logchannels.SetCategory` is already the single write: it validates the name, returns `gorm.ErrRecordNotFound` when `Get(chatID) == nil`, writes with `db.UpdateRecordWithZeroValues` and drops `log_channel:<chat>` [VERIFIED: logchannels/repository.go:137-156]. | `applyLogCategory(ctx, chatID, name string, on bool) logApplyResult` handles ONE category (outcomes `Applied`, `UnknownCategory`, `NotSet`, `SaveFailed`) and the command keeps its loop, its pre-checks, its "continue on SaveFailed" quirk (pinned by the golden, not fixed) and its audit post. The page writes through the same function. Whether the page also posts the `#SETTINGS` audit line is the one real design choice: recommended **yes**, best effort after the redraw through the same `actionlog.Settings` call (it is the only audit trail of a settings change that exists; `Destination` already gates it on the category) - Open Question Q1 |
| C-07 | D-06 | (a) The group-language write is **not in `/lang`**: `changeLanguage` only shows the keyboard; the write is `lang.ChangeGroupLanguage(chat.Id, language)` in the `change_language` callback `langBtnHandler` [VERIFIED: language.go:135], and the same callback in a private chat writes the user's own language through `lang.ChangeUserLanguage(user.Id, language)` [VERIFIED: language.go:127]. The apply function is extracted from the group branch only and the page must never reach the user branch. (b) The supported set is hard-coded in the callback as `[]string{"uk", "en", "es", "fr", "hi", "ru", "pt", "id"}` [VERIFIED: language.go:108] while the keyboard offers `config.AppConfig.ValidLangCodes`, which is the `ENABLED_LOCALES` setting with the default `[]string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"}` [VERIFIED: config.go:496-498, keyboard.go:37-56]. (c) The callback checks the code **before** the admin check, so a forged code from a non-admin gets `language_invalid_selection` first [VERIFIED: language.go:108-122]. (d) `ChangeGroupLanguage` already drops `chat_lang`, `chat_settings` and `chat` [VERIFIED: lang/repository.go:135-167]; none of them is in `skipLocal` [VERIFIED: cache/local.go:58-87], so another replica can serve the old language for up to `CACHE_LOCAL_TTL` (10 s), as `/lang` can. | `applyGroupLanguage(ctx, chatID, code) langApplyResult` (outcomes `Applied`, `Unsupported`, `SaveFailed`) holds the 8-code list as a package variable; the callback keeps its own order (validate, admin, apply). The page lists `config.AppConfig.ValidLangCodes` filtered like `MakeLanguageKeyboard` (non-empty `language_name`/`language_flag`), validates `v` against the 8 in step 1 and lets the apply function refuse the rest. After a write in a **group** menu the control replaces `p.tr` with `i18n.MustNewTranslator(code)` built from the code just written (never re-read: another replica's local cache can still hold the old one); in a private menu `p.tr` is untouched. No cache key beyond the three `ChangeGroupLanguage` drops, no `skipLocal` entry |
| C-08 | D-05 "no provider key" | The key `/aispam` checks is `config.AppConfig.TypeSafeAPIKey` (TypeSafe), not the Anthropic key: `aispamEnableRefusal` returns `aispam_no_key` when `config.AppConfig.TypeSafeAPIKey == ""`, then `aispam_globally_disabled` when `!config.AppConfig.EnableAISpam` [VERIFIED: aispam.go:865-875]; the status text uses the same two tests for its warning lines [VERIFIED: aispam.go:852-857]. Off is never refused. The enable refusal is evaluated **before** the write and a failed write replies `common_settings_save_failed` then **returns the error** (`return err`, not `ext.EndGroups`) [VERIFIED: aispam.go:821-834]. | Extract `aispamConfigured() bool` (the two tests, pure) and `applyAISpamEnabled(ctx, chatID, on) aispamApplyResult` (outcomes `Applied`, `NoKey`, `GloballyDisabled`, `SaveFailed`); the command maps `NoKey` to `aispam_no_key`, `GloballyDisabled` to `aispam_globally_disabled` and keeps its `return err`. The page draws one status line (new `settings_` key, a plain "AI spam checks cannot run on this bot" with no cause, key or provider) and Off alone while `!aispamConfigured()`; a stale or forged On press is answered with the command's own refusal text as the press's one answer and writes nothing (the `settingsMreqAIGate` shape). Reading `config.AppConfig` costs no network call. The status line reaches every member of a group menu: accepted, the same as Phase 15's AI line |
| C-09 | D-05 "Auto-approve", D-01 `applyAutoApprove` | (a) The command accepts only `on`/`yes`/`off`/`no` (`case "off", "no":` and `case "on", "yes":` [VERIFIED: greetings.go:864-874]), not `parseToggleArg`'s wider set. (b) The write is `greetings.SetShouldAutoApprove` through `upsertGreetingSettings`, which creates the whole greetings row with `defaultGreetingSettingsAttrs` when the chat has none and then updates only `auto_approve` and `updated_at`, then drops `greetings:<chat>` [VERIFIED: greetings/repository.go:234-254, 453-465]. So a first press on a chat with no greetings row creates the Welcome and Goodbye defaults too (the same side effect `/autoapprove on` has and the Welcome page's first-Set note describes). (c) The command reads `greetings.GetGreetingSettings(chatID).ShouldAutoApprove` (cached, may create the row) [VERIFIED: greetings.go:811]. | `greetings.go` already has the pattern: `greetingWrites` (a struct of write seams), `finishGreetingApply` and `greetingApplyResult` [VERIFIED: greetings.go:159-199, 234-242]. Add `AutoApprove func(chatID int64, on bool) error` to `greetingWrites` and `applyAutoApprove(ctx, chatID, on) greetingApplyResult`; `autoApproveToggleConfig.setPref` becomes `applyAutoApprove(ctx, chatID, v).Err` exactly as `cleanWelcomeToggleConfig` does [VERIFIED: greetings.go:751-755]. The page reads `greetings.LoadSettingsContext` (fresh, no row created). The golden and the parity test compare the **whole** greetings row snapshot (command vs press on a chat with no row), so the first-write side effect is pinned and equal; if it turns out to flip a visible Welcome or Goodbye setting the page adds a notice as the Antiflood page did (`settings_antiflood_now_on`) - SPIKE check 6 |
| C-10 | D-01 `applyReportsEnabled` | The group branch of `/reports` is `reports.SetChatReportStatus(chat.Id, true|false)` chosen by `msg.Chat.Type == "private"` (then `SetUserReportSettings(user.Id, ...)`, the user's own preference, even when the private chat is connected to a group) [VERIFIED: reports.go:290-304]. The repository writes **both** `enabled` and `status` with `db.UpdateRecordWithZeroValues(..., map[string]any{"enabled": pref, "status": pref})` after `GetChatReportSettings(chatID)` created the row [VERIFIED: reports/repository.go:38-48]; the report path reads `Status` [VERIFIED: reports.go:407-411]. No cache is involved (direct `db.GetRecord`). | `applyReportsEnabled(ctx, chatID, on) reportsApplyResult` (outcomes `Applied`, `SaveFailed`) wraps only the group branch; the private branch stays untouched in the command. The page reads `Status` through `reports.LoadChatStatusContext` (C-05). There is no cache key to drop and none to add |
| C-11 | D-10 | The grid fits. Today 9 non-owner pages (`s l c f w g r m d`, orders 10 to 85) plus the owner-only Access page (90) give 5 grid rows [VERIFIED: settings_*.go `Order:` lines]; six more pages make 15 cells = 8 grid rows (7 pairs and one single), plus the owner row and the navigation row = 10 rows, 18 buttons (Telegram's limit is 100 buttons) [arithmetic mine]. Worst-case text per language with 16 state lines at the current per-language maxima is 1093 (en) to 1201 (pt) UTF-16 units; allowing 46 units per line (22-unit state) it stays near 1.4 thousand against 3800 [VERIFIED: run, Python over locales/*.yml with the group title and opener name at 64 astral characters; new strings `[ASSUMED]`]. | No second hub. Recommended `Order` (UI-SPEC may change it): Warnings 42, Locks 44, Switches 46 (after Antiflood 40, before Welcome 50); Language 86, Log channel 87, Anonymous admins 88 (after Deletion 85, before the owner-only Access 90). Tests that change legitimately (none re-recorded to pass): the registry-driven shape tests adapt (`TestSettingsMainGrid` end-to-end subtest, settings_test.go gridRows); the hand-built "nine grid cells" subtest stays; `settingsMainWorstState` gains the new state strings; `TestSettingsLabelLengths` gains the six page titles and the new cells; the `settingsLocaleLimits` coverage test fails until every key has a row; `TestSettingsControlActionAllowList` subtest "every control of the other pages still accepts tg and st alike" iterates every control of every other page with `v=1` and requires `tg` and `st` to agree [VERIFIED: settings_mreq_test.go:1440-1450], so a new control either declares no `Actions` allow-list or its `Validate` must reject `v=1` |
| C-12 | D-12 "existing command strings are reused" | Reuse is right for refusals and outcomes, but: (a) the page is HTML while `/setwarnlimit` replies with `formatting.Smarkdown()` [VERIFIED: warns.go:605, 614]; the reused texts are plain (`warns_limit_range_error`, `warns_mode_updated_ban` ...) so they are safe, but any reused text with `<` or Markdown must be checked; (b) `warns_limit_updated`, `warns_invalid_number`, `locks_lock_failed` and others are printf templates (`%d`, `%s`) and are formatted with `fmt.Sprintf` and an **escaped** argument, as the Antiflood page does for `antiflood_setfloodmode_unknown_type` [VERIFIED: settings_antiflood.go:219-222]; (c) `TestSettingsLocaleKeys` and `settingsLocaleLimits` cover only the `settings_` prefix [VERIFIED: settings_locale_test.go:19-21, 443-460], so a reused non-`settings_` text is never length-checked as a notice. | New page strings are `settings_`-prefixed (limits rows, placeholder and tag checks apply). Reused command texts go through `settingsNoticeWarn`/`settingsNoticeOK` and a rendered-length test over the real locales (each notice plus the page body at most 3800 units, each alert at most 200 code points, `settingsAlertMaxRunes`). Notices never carry the group title unescaped |
| C-13 | D-13 order | "Goldens and extractions first, then pages" contradicts "a tracer slice first: one extraction and one page press". Reconcile: the tracer (18-01) is warn limit end to end (golden, extraction, reader, a one-control page); every later module is golden, extraction and page in two or three small plans (S-10). The goldens are separate commits recorded before the extraction in the same plan | Adopt the decomposition in S-10 |
| C-14 | D-14 "a command's cache keys are the ones its apply function already drops" | Confirmed, with the stated limits. Keys written and dropped by the repositories: `warn_settings:<chat>` [VERIFIED: warns/repository.go:274, 288], `ai_spam_settings:<chat>` [VERIFIED: aispam/repository.go:45], `locks_map:<chat>` (TTL 1 h) [VERIFIED: locks/repository.go:37, 41-43; optimized.go:51-63], `log_channel:<chat>` [VERIFIED: logchannels/repository.go:39-41], `chat_lang`, `chat_settings`, `chat` (above), `greetings:<chat>` [VERIFIED: greetings/repository.go:252]; `reports` and `admin` have no cache. None of the new keys is in `skipLocal`, which lists only captcha pending, staff, raid detection, member requirements and delete rules [VERIFIED: cache/local.go:58-87]. | No `skipLocal` entry, no new key. State in AGENTS.md: a lock, a log category, a warn setting or a language changed from the menu reaches another replica after at most `CACHE_LOCAL_TTL` (10 s default), exactly as when the command is used (accepted, as for Captcha and Antiflood) |

## Settled Findings

### S-01. Per-setting command facts and apply designs (question 1; PAGE-10)

All commands act on a group; none changes a Telegram permission or posts to the group (locks, language and the rest are database rows only). "First write" = what the repository does on a chat with no row.

| Setting | Command (registration) | Accepts / replies (keys, in order of the checks) | Write (zero-value safe?) | First write on a chat with no row | Cache keys dropped |
|---|---|---|---|---|---|
| Warn limit | `/setwarnlimit` via `WrapCommand(CheckDisabled)` + handler checks [VERIFIED: warns.go:915-924, 565-621] | gates: `IsUserConnected(true,true)` (replies `connections_is_user_connected_bot_not_admin` / `_user_not_admin`), `RequireUser`, `RequireBotAdmin` (`chat_status_bot_not_admin`), `RequireUserAdmin` (`chat_status_user_admin_cmd_error`/`_button_error`); then no arg `warns_limit_set_help`, `Atoi` error `warns_invalid_number`(arg), outside 1..100 `warns_limit_range_error` [VERIFIED: warns.go:599 `if num < 1 || num > 100 {`], save error `common_settings_save_failed`, else `warns_limit_updated`(n). All `formatting.Smarkdown()` | `warns.SetWarnLimit`: single `INSERT ... ON CONFLICT (chat_id) DO UPDATE SET warn_limit` of one column; Go values are real (limit >= 1) [VERIFIED: warns/repository.go:264-276] | inserts `WarnLimit: n, WarnMode: "mute"`; the `mute` equals the runtime default, so no visible change. A chat missing from `chats` fails the FK and the command answers `common_settings_save_failed` (page: same) | `warn_settings:<chat>` |
| Warn mode | `/setwarnmode` (same registration) [VERIFIED: warns.go:40-95] | same gates; no arg `warns_specify_action`; `strings.ToLower(args[0])` in `ban`/`kick`/`mute` -> write -> `warns_mode_updated_<mode>`; else `warns_mode_unknown`(original arg); save error `common_settings_save_failed`. `formatting.Shtml()` | `warns.SetWarnMode`: upsert of `warn_mode` only [VERIFIED: warns/repository.go:278-290] | inserts `WarnLimit: 3, WarnMode: mode` (default limit) | `warn_settings:<chat>` |
| Reports | `/reports` raw `helpers.NewBotCommand` [VERIFIED: reports.go:573] | gates: `IsUserConnected(true,true)`, `RequireUserAdmin`, `RequireBotAdmin`; `on/yes/true`, `off/no/false`: group `reports_turned_on_group` / `reports_turned_off_group`, save error `common_settings_save_failed`; no arg: `reports_status_enabled_group`/`_disabled_group` + `reports_change_settings_hint`. `Shtml` | `SetChatReportStatus`: `UpdateRecordWithZeroValues` of `enabled` and `status` (zero-safe) [VERIFIED: reports/repository.go:38-48] | creates the chat (`EnsureChatInDb`) and the row (`Enabled: true, Status: true`) first | none (uncached) |
| AI spam | `/aispam` via `WrapCommand` with `CheckDisabled("aispam"), RequireGroup(), RequireUserAdmin()` [VERIFIED: aispam.go:100-109] | no arg status text; `parseToggleArg` words `on yes true enable` / `off no false disable` [VERIFIED: federations.go:45-54], else `aispam_invalid_option`; enable refusals `aispam_no_key`, `aispam_globally_disabled`; save error reply then `return err`; success `aispam_enabled_success` / `aispam_disabled_success` | `aispam.SetAISpamEnabled`: upsert of `enabled`, `updated_at`; the `default:false` tag omits a Go `false` from the INSERT and the column default is `false`, so the update-on-conflict still writes false (coupling: never change that default) [VERIFIED: aispam/repository.go:32-47, models/aispam.go:10] | inserts the row | `ai_spam_settings:<chat>` |
| Auto-approve | `/autoapprove` via `WrapCommand(CheckDisabled)` + `greetingToggle` [VERIFIED: greetings.go:1503-1509, 824-893] | gates above (C-02); no arg `greetings_auto_approve_enabled` (Smarkdown) / `_disabled`; `off`/`no` -> `greetings_auto_approve_disable`; `on`/`yes` -> `greetings_auto_approve_enable`; else `greetings_auto_approve_invalid_option`; save error `common_settings_save_failed` | `upsertGreetingSettings` column update of `auto_approve` (zero-safe through a map) | creates the full greetings row (C-09) | `greetings:<chat>` |
| Group language | callback `change_language` (`langBtnHandler`), the keyboard from `/lang` [VERIFIED: language.go:39-168] | `language_invalid_selection` for a code outside the 8; group: `RequireUserAdmin` (responder `chat_status_user_admin_cmd_error`/`_button_error`); write; `query.Answer`; edit to `language_changed_group` | `lang.ChangeGroupLanguage`: `Create` of a `Chat` when none, else `UpdateRecord` of `language` (a non-empty string, so zero-safe); equal language is a no-op returning nil | creates a `chats` row with the language | `chat_lang`, `chat_settings`, `chat` |
| Log categories | `/log`, `/nolog` raw (`logCategories` is the read-only list `logs_categories`) [VERIFIED: logchannels.go:265-270] | C-06; gates: `chat.Type != "private"` (`logs_group_only`), `RequireUser`, `RequireUserAdmin` | `logchannels.SetCategory` (zero-safe, needs a row) | none: no row means `logs_not_set` | `log_channel:<chat>` |
| Anonymous admins | `/anonadmin` via `WrapCommand(RequireGroup, RequireBotAdmin)` | no argument: `admin_anon_admin_enabled`/`_disabled` (title spliced with `fmt.Sprintf`); an argument: owner check, then `on/true/yes`: already -> `admin_anon_admin_already_enabled`, else write -> `admin_anon_admin_enabled_now`; `off/no/false` mirrored; save error `admin_anon_admin_db_error` (reply, `EndGroups`); other `admin_anon_admin_invalid_arg` [VERIFIED: admin.go:583-648; keys locales/en.yml:2-76] | `admin.SetAnonAdminMode`: `UpdateRecordWithZeroValues` of `anon_admin` after `checkAdminSetting` created the row [VERIFIED: admin/repository.go:33-45] | creates the row (`AnonAdmin: false`) on any read, **before** the owner check | none (uncached) |
| Locks | `/lock`, `/unlock` raw; `/locks`, `/locktypes` raw and disableable [VERIFIED: locks.go:521-526] | `/lock`: gates `IsUserConnected(true,true)`, `RequireBotAdmin`, `RequireUserAdmin(sender.Id())`; no arg `locks_what_to_lock`; unknown name `locks_invalid_lock_type`(name) refuses all; failures `locks_lock_failed`(list), else `locks_locked_successfully`("\n - " list). `/unlock` mirrored (`locks_what_to_unlock`, `locks_unlock_failed`, `locks_unlocked_successfully`). `/locks` prints `locks_current_locks_header` then `"\n - %s = %v"` for the **stored** rows, sorted | `locks.UpdateLock`: upsert on `(chat_id, lock_type)` of `locked` | inserts the row; FK to `chats` | `locks_map:<chat>` |

**Apply functions** (each beside its command; one write path; the repository's own `DeleteCache` stays the only invalidation). A write seam per module (a `var xWrites = struct{...}` as `greetingWrites`) lets a test make the write fail. The result type per module follows `floodApplyResult` [VERIFIED: antiflood.go:434-457]: an outcome enum whose zero value is `Applied`, the stored row read back (nil when the write or the read-back failed, so the caller reloads) and the error.

| Function | Signature (recommended) | Outcomes | Command keeps |
|---|---|---|---|
| `applyWarnLimit` | `(ctx, chatID int64, limit int) warnApplySettingsResult` | `Applied`, `OutOfRange` (not 1..100, nothing written), `SaveFailed` | argument parsing, `warns_invalid_number`, all gates, Smarkdown replies |
| `applyWarnMode` | `(ctx, chatID int64, mode string)` | `Applied`, `UnknownMode` (not `ban`/`kick`/`mute`, as spelled; the caller lower-cases), `SaveFailed` | `strings.ToLower`, the `warns_mode_unknown` text with the original argument, Shtml |
| `applyReportsEnabled` | `(ctx, chatID int64, on bool)` | `Applied`, `SaveFailed` | the private-chat branch, argument words |
| `applyAISpamEnabled` | `(ctx, chatID int64, on bool)` | `Applied`, `NoKey`, `GloballyDisabled`, `SaveFailed` | `parseToggleArg`, status text, `return err` on save failure |
| `applyAutoApprove` | `(ctx, chatID int64, on bool) greetingApplyResult` (existing type) | `greetingApplied`, `greetingSaveFailed` | argument words, Smarkdown on the enabled status |
| `applyGroupLanguage` | `(ctx, chatID int64, code string)` | `Applied`, `Unsupported`, `SaveFailed` | the callback's validate-then-admin order, the answer and the edit |
| `applyLogCategory` | `(ctx, chatID int64, name string, on bool)` | `Applied`, `UnknownCategory`, `NotSet`, `SaveFailed` | the multi-argument loop, `logs_not_set`/`logs_usage`/`logs_unknown_category`, the "continue on SaveFailed" quirk, the audit post |
| `applyAnonAdmin` | `(ctx, chatID int64, current, want bool)` | `Applied`, `Already` (no write), `SaveFailed` | the initial `GetAdminSettings` read (it creates the row even for a refused non-owner, so keep it), the owner check, `strings.ToLower(m.moduleName)` key building |
| `applyLock` | `(ctx, chatID int64, lockType string, on bool)` | `Applied`, `UnknownType`, `SaveFailed` | the all-or-nothing pre-validation loop, `locks_lock_failed`/`locks_locked_successfully` list building |

**Characterization goldens** (separate commit, recorded before each extraction, never re-recorded): follow `rules_legacy_request_test.go` - a fixed chat ID per case (a `const` block such as `legacyRulesChat int64 = -1009912001201`), `legacyRequestJSON(t, call)` (the `json.Marshal` of the call's params, map keys sorted) and `moduleBotClient`/`newModuleMessageContext` fakes [VERIFIED: rules_legacy_request_test.go:19-48]. One case per branch of S-01 per command (every key above, the refusal paths and the save-failure paths), asserting method, `chat_id`, `text`, `parse_mode` and the stored row, so both the request and the storage side effect are pinned. Existing assertion-style tests (`TestWarnSettingsCommandsUpdateAndDisplay`, `TestAnonAdminOwnerTogglesSetting`, `TestLockAndUnlockCommandsPersistSettings`, `TestAISpamCommandTogglesChatSetting`, `TestNologTogglesCategory`, `TestLanguageCallback*`) stay and are not goldens.

### S-02. Rights per command (question 2)

| Control (page) | Command's checks today | Right to declare | Refusal the menu gives |
|---|---|---|---|
| Warn limit, Warn mode | `IsUserConnected(true,true)` (bot via cached `IsUserAdmin(b, chat, b.Id)`, user via cached `IsUserAdmin`), then `RequireBotAdmin` (cached `IsBotAdmin`), `RequireUserAdmin` | `settingsRightAdminFloodBot` (refuses with `connections_is_user_connected_bot_not_admin`, the first-firing bot check, exactly as `/setflood` does) [VERIFIED: settings_menu.go:129-133; chat_status.go:486-497] | notice `connections_is_user_connected_bot_not_admin` |
| Reports | `IsUserConnected(true,true)`, `RequireUserAdmin`, `RequireBotAdmin` | `settingsRightAdminFloodBot` | same notice |
| AI spam | `CheckDisabled`, `RequireGroup`, `RequireUserAdmin` only (no bot check) | `settingsRightAdmin` | none beyond the press pipeline; configuration refusal is the page's gate (C-08) |
| Auto-approve | `IsUserConnected(true,true)`, `CanUserChangeInfo` | new `settingsRightChangeInfoBotAdmin` (C-02) | bot notice `connections_is_user_connected_bot_not_admin`; member alert `settings_no_change_info` |
| Language (group) | `RequireUserAdmin` only | `settingsRightAdmin` | none |
| Log categories | `RequireUserAdmin` only; plus the linked-channel precondition | `settingsRightAdmin` | `logs_not_set` as a notice when not linked |
| Anonymous admins | `RequireGroup`, `RequireBotAdmin`; changes also `RequireUserOwner` (live creator) | new `settingsRightOwnerBotAdmin` (C-01, C-02) | bot notice `chat_status_bot_not_admin`; owner alert `chat_status_owner_button_error` |
| Locks | `IsUserConnected(true,true)`, `RequireBotAdmin`, `RequireUserAdmin` | `settingsRightAdminFloodBot` | notice `connections_is_user_connected_bot_not_admin` |

Opening any page needs `settingsRightAdmin` (the live administrator the press pipeline already proved). Documented carve-outs the framework already states and the authority-parity tests carry: the user check is live (stricter than the commands' cached `IsUserAdmin`, which can lag a demotion by the admin-cache TTL), the bot check is the commands' own cached predicate, and `settingsGate` (the owner gate) can make the menu stricter for a non-owner administrator with the switch off [VERIFIED: AGENTS.md "Permissions"; settings_menu.go:115-135]. Two further small carve-outs to document: for `/setwarnlimit`, `/setwarnmode`, `/reports` and `/lock` the command's second bot check (`IsBotAdmin`, status `administrator`) can in principle disagree with the first (`IsUserAdmin` over the admin list) and the menu makes only the first; a presser who lacks a member right while the bot is not an administrator gets the command's order (bot first) for the two new composite rights. Commands disabled with `/disable` do not bind administrators (`CheckDisabledCmd` returns false for an admin [VERIFIED: chat_status.go:164-166]), so the menu needs no disable check. Authority-parity test pattern: "mirrors /command" with the settings fake's `setBotRole` and a real command run in the same chat, as `TestSettingsRights` does for `/captcha` and `/setflood` [VERIFIED: settings_menu_test.go:320-388].

### S-03. Locks (question 3; PAGE-06; C-04)

- **Types:** 22 = 16 "permission locks" (`lockMap`, enforced by `permHandler` at group 5) + 6 "restriction locks" (`restrMap`, `restHandler` at group 6) [VERIFIED: locks.go:28-32, 59-92]. `permHandler` skips the `bots` lock (it is enforced by `botLockHandler` on a bot joining) [VERIFIED: locks.go:454-456, 465-516]. The page's two sections can reuse the command docs' vocabulary ("Permission Locks", "Restriction Locks" in `locks_types_docs`, locales/en.yml:1896-1921, not read by Go code).
- **What `/locks` prints:** only the **stored** rows (`GetChatLocks`), `"\n - %s = %v"`, sorted [VERIFIED: locks.go:120-139]; a type never written is absent and enforced as unlocked (`chatLocks[key]` false). The page shows the same value: stored `true` is locked, everything else unlocked.
- **No Telegram write:** a lock is a row; enforcement is deletion by the watchers for non-administrators and non-approved senders [VERIFIED: locks.go:387-396, 438-447]. So a lock write needs no permission change and no bot right at write time.
- **Write and cache:** `locks.UpdateLock` then `InvalidateLockCache` (`locks_map:<chat>`, TTL 1 hour) [VERIFIED: locks/repository.go:21-43]; read through `GetChatLocksCachedContext`, which returns the error (the page's reader).
- **Page size and rows:** recommended **8 locks a page** in a two-column grid (3 pages: 8, 8, 6; 4 grid rows), then one pager row, then Back and Close: 6 rows. A pager action `kp` with `v` (the target page, `settingsParseNumber`, clamped like `settingsStaffPage` [VERIFIED: settings_staff.go:316-329]) leaves the toggles' `a=tg&p=<page key>&t=<lock>&v=<0|1>`; after a toggle the redraw stays on the lock's own page because the page index is the lock's position in the table (no extra callback field). The value button of a stepper-like pager and Refresh (`op`) open page 1 (CONTEXT specific: "opens at page 1"). The lock icon pair (locked / unlocked) is the state, the body carries one line per page, not the whole list.
- **Labels:** no per-type label key exists today, so 22 new `settings_` keys x 8 locales. A cell is the state icon, a space and the label: the label must be at most 18 code points in every language (the 20-code-point cell, `gridMax = 20` [VERIFIED: settings_locale_test.go:581-588]). Risk names to shorten in ru, uk, hi, es, pt, fr: `anonchannel`, `videonote`, `previews`, `comments`, `location`, `forward`. Add the 22 keys to a `lockLabels` group in `TestSettingsLabelLengths` rendered with both icons.
- **Paging within the callback cap:** toggles at most 55 bytes with the longest name (C-04); pager `a=kp&c=-1001234567890&v=3` 12 + 25 = 37 bytes. The worst-case test (`TestSettingsLocksCallbackBytes`) renders every page in both menu kinds with a 14-character group ID and a 10-digit user ID.
- **Impact of `all` and `messages`:** `/lock all` makes the watchers delete every non-admin, non-approved message at once. The command has no confirmation, the lock is one tap to undo, and PAGE-10 is "refused exactly as by the command", so no card is recommended (Open Question Q2); the page carries one hint line that admins and approved users are exempt.

### S-04. Language (question 4; PAGE-03; C-07)

- **Selection of the menu's own language:** `newSettingsPress` sets `tr: ctxTr(ctx)` and `ctxTr` is `i18n.MustNewTranslator(lang.GetLanguage(ctx))` [VERIFIED: settings_menu.go:319-321; i18n_helpers.go:24-26]. `GetLanguage` reads the update's chat: private chat -> `getUserLanguageContext(userID)` (key `user_lang`), anything else -> `getGroupLanguageContext(chatID)` (key `chat_lang`); default `"uk"` [VERIFIED: lang/repository.go:31-58]. A callback update's chat is the menu message's chat, so a group menu draws in the group's language and a private menu in the presser's own.
- **Storage and cache:** the group's language is `chats.language`, read by `getCachedLanguage` (`cache.GetFromCacheOrLoad`, `CacheTTLLanguage`), a missing row or empty value reads `"uk"`; the user's is `users.language`. A group write drops `chat_lang`, `chat_settings`, `chat` (C-07 d); a user write (not used here) drops `user_lang`, `user`. No further key is needed. Skew: another replica serves the old language for up to `CACHE_LOCAL_TTL`; so does `/lang`.
- **"Redraws in the new language":** group menu: after `applyGroupLanguage` the control sets `p.tr` to the translator of the code written and returns the redrawn page (the Language page, then every later press builds `ctxTr` from the stored value). Private menu: the page keeps the opener's translator and shows the group's current language by its own name in it (D-06, A18-4 unchanged). A test per menu kind: group, the page text after the press is the new language's; private, it stays the opener's.
- **The 8 languages and their own names:** `uk` Українська, `en` English, `es` Español, `fr` Français, `hi` हिन्दी, `id` Bahasa Indonesia, `pt` Português, `ru` Русский, each `language_name` plus a `language_flag` in its own locale [VERIFIED: locales/*.yml `language_name`/`language_flag` lines]; `keyboard.GetLangFormat(code)` is `name + " " + flag` [VERIFIED: keyboard.go:16-21]. These are not new keys. Cell width: flag, space, name is at most 19 code points ("Bahasa Indonesia" is 16 + 1 + a 2-code-point flag); the selected cell replaces the flag by the check mark (`settingsSelectedPrefix`, 2 code points) which is also at most 19; two per row, four rows.
- **Stored code outside the 8** (an import or an old row): show the code escaped, mark nothing, offer all choices.

### S-05. Log channel (question 5; PAGE-04; C-06)

- **Model:** `models.LogChannel` has `chat_id`, `log_channel_id` and six booleans `cat_settings`, `cat_admin`, `cat_user`, `cat_automated`, `cat_reports`, `cat_other` (all `default:true`) [VERIFIED: models/logchannels.go:5-17]; `logchannels.AllCategories` is `settings, admin, user, automated, reports, other` [VERIFIED: logchannels/repository.go:30-37]. `Set` (used by the forward-binding flow, not the page) inserts all six `true` and on conflict updates only `log_channel_id` and `updated_at`; `Unset` deletes the row.
- **Shown as "linked or not" without naming the channel:** the page uses `logchannels.LoadContext` (C-05): a nil row with a nil error is "not linked", an error is the cannot-open edit. It never prints `log_channel_id`, a title or `logs_current` (the command prints the channel ID in `logs_current`, which the group menu must not [VERIFIED: logchannels.go:121-124]). Main state: reuse `settings_state_linked` / `settings_state_not_linked` [VERIFIED: locales/en.yml:3077, 3081] or a count of enabled categories (new key). Category cells: six, two per row, labelled by new `settings_` keys, the state as the check mark or cross icon.
- **What `/log`, `/nolog` write and refuse:** S-01 and C-06. The page with no channel linked draws the six cells and refuses a press with the notice `logs_not_set` (reused text), writing nothing.
- **Relation to the Staff Group paths (no behaviour change):** `actionlog.Destination` reads `logchannels.Get` (cached) and `logchannels.CategoryEnabled`, and the staff log posts, lockdown alert copies and `#STAFF_UNDO` posts use it with the `admin` category [VERIFIED: actionlog.go:21-33; AGENTS.md "Subsystem traps" staff and lockdown bullets]. A category toggled from the page changes exactly what the command changes, through `log_channel:<chat>`; nothing in staff or lockdown code is edited. One hint line says linking stays `/setlog` and `/unsetlog`.
- **Privacy:** the group menu is readable by every member; "linked or not" and the six on/off flags are shown there (accepted by A18-3); the channel's title and ID never are.

### S-06. Switches (question 6; PAGE-02; C-08, C-09, C-10)

- **Reports:** group status = `ReportChatSettings.Status` (default `true`), uncached, written together with `enabled`. **AI spam:** `AISpamSettings.Enabled` (no row = off), cached under `ai_spam_settings:<chat>`. **Auto-approve:** `GreetingSettings.ShouldAutoApprove`, from the greetings row.
- **"AI not configured" status line:** `config.AppConfig.TypeSafeAPIKey == ""` or `!config.AppConfig.EnableAISpam` [VERIFIED: aispam.go:852-857, 865-875] - config reads only, no network. While it holds the AI cell offers Off alone (as Phase 15's Off-only form), a stored On stays visible and inert, and the page never prints the cause (D-05). Test helpers: a `withAISpamConfigured(t)` / `withAISpamNotConfigured(t, how)` pair that sets and restores `config.AppConfig` in `t.Cleanup`, as `withAISpamTestConfig(t, apiKey, globallyEnabled, dump)` does [VERIFIED: aispam_test.go:26].
- **Main state:** `{n} on` for the number of the three that are on (reads three rows; a failed read is the unknown mark).
- **Cost of Main:** with six more pages a Main draw makes about six more cheap reads (three cached, three direct `SELECT`s); accepted, no Telegram call is made by any `MainState` [VERIFIED: settings_pages.go:359-362 contract].

### S-07. The settings framework as it stands (question 7)

**Registration fields a new page needs:** `settingsPage{Key, Order, Icon, TitleKey, MainState, Open, Controls, Actions}` plus optional `OwnerOnly` and `Hidden` [VERIFIED: settings_pages.go:350-376]; `Open(p, notice) (settingsScreen, error)`; `settingsControl{Right, Set, Validate, Actions}`; `settingsPageAction{Fields, Right, Validate, Run}`; `registerSettingsPage` panics at init on a duplicate page key, a duplicate page action code or a page action code equal to a core code [VERIFIED: settings_pages.go:429-446]. Helpers: `settingsStepperRow(k, page, control, current, steps, clamp, valueLabel)` (absolute target `clamp(current+step)`, a target equal to the current value is labelled with the bound mark and keeps valid data) [VERIFIED: settings_pages.go:274-305], `settingsChoiceRow`, `settingsToggleButton` (sends `"1"` when off and `"0"` when on) [VERIFIED: settings_pages.go:314-346], `settingsCardRows` with `settingsCardExpired` (two-tap card; none needed here) [VERIFIED: settings_card.go:46-83], `settingsCompose`, `settingsNoticeWarn`/`settingsNoticeOK`, `settingsStateWord`, `settingsStateIcon`, `settingsParseNumber`. The press pipeline: decode and field allow-list, service identity, opener (group menu), live `getChatMember`, owner gate, right, `Set`/`Run`; the callback is answered once through `p.answer` and the menu edited in place [VERIFIED: settings_menu.go:335-460, 540-594]. The model pages to copy: Antiflood (`settings_antiflood.go`: read seam, `Redraw` from the apply read-back, first-write notice), Captcha (`settingsRightAdminBotAdmin` toggle), Deletion (one control per table row registered in `init`).

**Registered code table (page keys, core codes, page action codes and control `t` values; the UI-SPEC must avoid all of them)** [VERIFIED: grep of `registerSettingsPage`, the `settingsPage*`, `settingsAct*` constants and `Controls` maps this session]:

| Page | Key `p` | Order | Controls `t` | Page action codes `a` |
|---|---|---|---|---|
| Staff Group | `s` | 10 | none | `lk` `ua` `uy` `pg` |
| Lockdown | `l` | 20 | `j` `f` `a` | `ls` `ly` |
| Captcha | `c` | 30 | `t` `e` `m` `f` | none |
| Antiflood | `f` | 40 | `l` `m` `d` | none |
| Welcome | `w` | 50 | `e` `d` `o` `u` | `ws` `wr` `wc` `wv` `wf` |
| Goodbye | `g` | 60 | `e` `d` `u` | `gs` `gr` `gc` `gv` `gf` |
| Rules | `r` | 70 | `u` | `rs` `rr` `rc` `rv` `rf` |
| Requirements | `m` | 80 | `ln` `un` `ru` `ar` `zh` `ph` `ch` `ai` `j` `d` | `mo` `my` |
| Deletion | `d` | 85 | `sj` `sl` `sp` `sh` `sn` `sv` `sf` `sb` `sa` `sk` `sg` `so` `bw` `bg` `br` `ba` `bm` `mc` `me` `ms` | `dl` `dr` `dy` |
| Access (owner only) | `a` | 90 | `s` | `ay` |
| Core actions (reserved) | | | | `oh` `op` `bk` `cl` `tg` `st` `no` `pk` |

The private hub list uses its own namespace `shub|v1` with `lp` and `cx` (no collision with the settings namespace) [VERIFIED: AGENTS.md "Callbacks"]. Control `t` values are scoped to their page, so only page keys and page action codes can collide. **Free page keys:** `b e h i j k n o q t u v x y z` (avoid `p`, the field name). Suggested: Warnings `n`, Locks `k`, Switches `x`, Language `i`, Log channel `o`, Anonymous admins `z`. **Free action codes:** `kp` (the locks pager) is free; no other page action is needed (Warnings, Switches, Language, Log channel, Anonymous admins use controls only). Controls: Warnings `l` (limit) and `m` (mode); Switches `r`, `s`, `a`; Language `l`; Log channel the six category names (at most 9 characters, "automated"); Anonymous admins `m`; Locks the 22 lock names.

**Alignment with `18-UI-SPEC.md` (present in the phase directory when this file was finished):** the UI-SPEC's page keys (`n` Warnings, `t` Switches, `u` Language, `o` Log channel, `y` Anonymous admins, `k` Locks), pager code `kg` and `Order` values (42, 44, 86, 87, 88, 89) are all inside the free sets above and take precedence over the suggestions in this file (they are Claude's-discretion items). Its M35 agrees with C-02 on the two composite rights and its rights table agrees with S-02. One difference to settle in planning: for the owner refusal of Anonymous admins it names `chat_status_owner_cmd_error` while this file suggests `chat_status_owner_button_error`; both keys exist in all locales (en.yml:1574-1575) and a press is a callback, for which the commands' own responder picks the `_button_error` variant, so prefer that one unless the UI-SPEC's wording is chosen on purpose.

**Main grid:** see C-11. **Tests pinning the grid, and what changes legitimately:** `TestSettingsMainGrid` (hand-built entries stay; end-to-end subtest is registry-driven and adapts), `TestSettingsMainPageFitsEveryLocale` (iterates the registry; `settingsMainWorstState` needs the new state strings; `len(entries) < 5` guard), `settings_menu_test.go:478-510` (every page ends with Back and Close in a group and in a private menu, and Back draws Main: **every new page must end with the nav row and its pager row must come before it**), `settings_test.go:167` (rows computed from the registry), `TestSettingsControlActionAllowList` (C-11), `TestSettingsLabelLengths` page list, the `settingsLocaleLimits` coverage test, `TestSettingsStaffPrivacy` (add `settingsPrivacyPhase18Run`), `TestSettingsHelpFitsOneMessage` if `settings_help_msg` is edited (S-08). Every test that opens every page in the default environment requires each new page's `Open` to work with the default fake and no Telegram call beyond `getChatMember`/`getChat` (our pages make none).

### S-08. Test and CI strategy (question 8)

- **Per setting (AGENTS.md demands):** parity (`TestSettingsParityWarnings`: a menu press and the command leave the same stored row and the same cache state, compared on the whole row), drift (`TestSettingsDriftWarnings`: the page shows what the command's read shows, including the `/warnings` text), authority parity (`TestSettingsAuthorityParityWarnings` and the right mirrors above), worst-case callback bytes (`TestSettingsWarningsCallbackBytes`, 64 bytes with a 14-character chat ID and a 10-digit user ID), page length (`TestSettingsWarningsPageFitsEveryLocale`, 3800 UTF-16 units and the label limits, 64-rune titles in the three title shapes of `TestSettingsMainPageFitsEveryLocale`). The same family for Switches (three controls, one parity test each), Language, Log channel, Anonymous admins, Locks. Where no command exists to compare (none here: every control has a command) a drift and an authority test replace parity.
- **Characterization goldens:** S-01, `TestLegacyWarnSettingsRequests`, `TestLegacyReportsRequests`, `TestLegacyAISpamRequests`, `TestLegacyAutoApproveRequests`, `TestLegacyLanguageRequests`, `TestLegacyLogCategoryRequests`, `TestLegacyAnonAdminRequests`, `TestLegacyLockRequests`.
- **Harness:** no new model, so no `AutoMigrate` change: `WarnSettings`, `ReportChatSettings`, `AISpamSettings`, `AdminSettings`, `LockSettings`, `LogChannel`, `GreetingSettings` are already in `fuku/modules/test_harness_test.go` [VERIFIED: test_harness_test.go:334-365]. `settingsCleanup` (settings_env_test.go:192-221) does not delete these rows or the keys `warn_settings`, `ai_spam_settings`, `locks_map`, `log_channel`, `greetings`: AGENTS.md says page tests do not edit the toolkit, so each page's tests add a page-prefixed cleanup in their own file (as `settingsMreq*` do) deleting its rows and keys and calling `dbcache.ResetLocalForTest()`.
- **Hygiene:** fresh IDs from `uniqueLinkOwnerID()` and `uniqueModuleChatID()`; a test that changes `config.AppConfig` restores it in `t.Cleanup`; no `t.Skip`; no `t.Parallel` with config swaps; whole-package `./fuku/modules` run (9 to 15 minutes) at plan boundaries that register a page or change a shared file (the lesson of Phase 17: cross-plan interactions - registry-iterating tests, locale tables - only fail in the whole package or the settings family), plus `./fuku/i18n` and `./fuku/db/...` for the readers.
- **PostgreSQL integrity:** no new SQL statement (the readers are `SELECT`s, the writers exist), so nothing joins `make test-postgres-integrity`; `./fuku/db/reports` and `./fuku/db/warns` are already in its package list [VERIFIED: Makefile:39-44]. If a reader's repository test needs a PostgreSQL guarantee (a first write failing the `chats` foreign key) it is already covered by `TestWarnUserCreatesMissingParentRows` and `TestReportSettingsCRUD`.
- **Lint traps:** gocyclo 20 (`.golangci.yml:17-21`): the Locks renderer, the Switches renderer and `setLogCategories` (already long) must be split into build and draw steps; dupl 100: three toggle pages (Switches, Log channel, Locks) share one generic "press -> read before -> apply -> redraw" helper (the `settingsMreqSetSwitch` shape [VERIFIED: settings_mreq.go:361-386]) rather than three copies; G101: no identifier containing `token`, `secret`, `pass`, `pw`, `cred` or `apiKey` assigned a string literal (the TypeSafe key is read from config, never named by a constant); `make lint` itself cannot run here (golangci-lint built with go1.25.1), so use `go vet`, `gofmt -l`, `gosec` and a manual dupl and gocyclo read, and record that in each SUMMARY.

### S-09. Threats (question 9; STRIDE)

| Pattern | STRIDE | Mitigation (existing unless noted) |
|---|---|---|
| Forged callback: new page keys, `t` that is not a table row, `v` outside the allowed set, extra keys, `c` and `u` together | Tampering | `settingsCheckFields` allow-list and `settingsControlFieldsValid` run in step 1 before any I/O; each control's `Validate` (`v` in `0`/`1`, the 8 language codes, 1 to 3 digits for the limit and the pager); the lock and category controls exist only for table rows [VERIFIED: settings_menu.go:238-316]. New: a test that forges every `t` and `v` shape per page |
| A presser without the command's right | Elevation | the press pipeline's live `getChatMember`, `settingsGate`, then the control's right; the two new composite rights (C-02); owner-only anon-admin writes judged on the press's own live record |
| `/anonadmin` mode widened by a non-owner | Elevation | owner right (C-01): the mode makes `checkAnonAdmin` return admin for every anonymous sender (`if admin.GetAdminSettings(chat.Id).AnonAdmin { return true, true }` [VERIFIED: chat_status.go:68-70]), so it must never be easier than the command |
| Stale menu (another admin changed the value; a demoted presser) | Tampering | absolute `v`, redraw from the read-back, rights judged on a fresh member record on every press, a double tap writes the same row |
| Cross-chat replay (`c` of a private menu, `u` of a group menu) | Spoofing | the group is always `p.group` (the message's chat or the verified `c`), never a field of the page; a private `c` is re-verified by live `getChatMember` and `getChat` |
| Language change reaches the user's own language | Tampering | the page calls only `applyGroupLanguage`; the callback's private branch is never called by it (test: a private-menu language press leaves `users.language` untouched) |
| Information disclosure in a group menu: log channel title or ID, Staff Group, operator configuration | Information disclosure | the page shows linked or not and the six flags only; `TestSettingsStaffPrivacy` renders every new screen in 8 languages in a group menu and in a private menu of a non-creator administrator, asserting no channel ID, title or Staff Group text; the AI line carries no key, provider or cause |
| An apply function changes a command's behaviour | Tampering | the goldens (never re-recorded) and the full-row parity tests; the command keeps its own gates, parsing and replies |
| Page and command disagree (PAGE-10) | Tampering | one reader per setting sharing the command's cache key and TTL; the same `DeleteCache` on every write; drift tests including a two-replica case where useful (`otherReplica`) |
| `/lock all` or `messages` locks everyone out by a stray tap | Denial of service (to members) | stated, not mitigated (parity, one tap undoes it, admins and approved exempt): Open Question Q2 |

ASVS: V4 Access Control and V5 Input Validation apply (strict `Validate`, table-bound controls); V2, V3 do not (Telegram authenticates senders); V7 logging: log lines carry chat and user IDs and error classes only, never a lock list or a channel ID beyond what the commands already log. `security_enforcement` is not disabled in `.planning/config.json` (key absent), so this section applies.

### S-10. Recommended plan decomposition (question 10)

Sequential, small (about 3 tasks each); `use_worktrees` is false so `locales/*.yml`, `settings_menu.go` and `AGENTS.md` are single-owner per wave (AGENTS.md only in 18-13, staged notes in the earlier summaries).

| Plan | Content | Depends on |
|---|---|---|
| 18-01 tracer (warn limit) | (1) golden commit for `/setwarnlimit` and `/setwarnmode`; (2) `applyWarnLimit`/`applyWarnMode`, `warns.LoadWarnSettingsContext` + repository test, command mapped, goldens unchanged; (3) the Warnings page registered with the limit stepper only, one parity test and a minimal set of keys (a press writes through the apply function) | none |
| 18-02 Warnings complete | mode choice row, the read-only note for a stored `tban`/`tmute`/empty value, Main state, the bound toasts (`settings_min_value`/`settings_max_value`), drift, authority parity, bytes, fit tests, all locale keys of the page | 18-01 |
| 18-03 rights and auto-approve + reports | `settingsRightChangeInfoBotAdmin` and `settingsRightOwnerBotAdmin` in `settings_menu.go` with tests (both refusal orders); goldens and extractions for `/autoapprove` and `/reports`; readers `reports.LoadChatStatusContext` | 18-02 |
| 18-04 AI spam | golden, `aispamConfigured`, `applyAISpamEnabled`, reader `aispam.LoadEnabledContext` | 18-03 |
| 18-05 Switches page | three controls through the shared toggle helper, the AI status line and Off-only form, Main state, parity per control, config helpers, locale keys | 18-04 |
| 18-06 Language | golden of the callback (group and private), `applyGroupLanguage`, reader, the page, the `p.tr` swap, group and private tests, locale keys | 18-05 |
| 18-07 log channels extraction | golden of `/log`, `/nolog`, `/logchannel`, `/logcategories`; `applyLogCategory`; `logchannels.LoadContext` | 18-06 |
| 18-08 Log channel page | six category controls, the not-linked refusal, the hint, the audit post decision, privacy tests, locale keys | 18-07 |
| 18-09 Anonymous admins | golden, `applyAnonAdmin`, reader, the page with `settingsRightOwnerBotAdmin`, tests (creator vs administrator vs bot-not-admin), keys | 18-08 |
| 18-10 locks extraction | golden of `/lock`, `/unlock`, `/locks`, `/locktypes`; `applyLock`; the lock table and `TestSettingsLocksTableMatchesCommand` | 18-09 |
| 18-11 Locks page | 22 controls from the table, paging (`kp`), 22 labels x 8 locales, callback bytes, fit tests | 18-10 |
| 18-12 Main grid and sweeps | `Order` values, `settingsMainWorstState`, `TestSettingsLabelLengths` groups, `TestSettingsStaffPrivacy` Phase 18 run in group and private menus, `TestSettingsControlActionAllowList` check, help text if edited | 18-11 |
| 18-13 closing | AGENTS.md (settings bullets, rights, Data keys, tests, the 10 s skew), README, `make generate-docs`/`check-docs`, `18-SPIKE.md`, the gate (`make test`, `make check-translations`, `make check-docs`, `make test-postgres-integrity`, lint stand-in), PAGE-01 to PAGE-06 and PAGE-10 ticked | 18-12 |

## Architecture Patterns

### Diagram (one press)

```
Telegram callback settings|v1|a=st&p=<page>&t=<control>&v=<value>&(c|u)=...
   -> settingsCallback -> settingsPress.verify
        decode + allow-list + Validate (no I/O) -> service id -> opener
        -> live getChatMember (presser) -> owner gate -> [private: getChat]
   -> runControl: checkRight(control.Right)     (new composite rights: bot part first)
   -> control.Set(p, v):
        read before  (error-returning reader, same cache key as the command)
        apply<Setting>(ctx, p.group.Id, v)       (validation, refusal, write, DeleteCache)
        redraw from the read-back / notice from the outcome
   -> p.edit(screen)    (Language: p.tr replaced first, group menu only)
```

### Recommended project structure (additions only)

```
fuku/modules/settings_warnings.go    settings_switches.go   settings_language.go
fuku/modules/settings_logchannel.go  settings_anonadmin.go  settings_locks.go
fuku/modules/{warns,reports,aispam,greetings,language,logchannels,admin,locks}.go   # apply functions beside the commands
fuku/modules/settings_menu.go        # two rights only
fuku/db/{warns,reports,aispam,admin,logchannels,lang}/repository.go               # six readers
```

### Pattern: a toggle control (shape of `settingsMreqSetSwitch`, S-08)

```go
// Source: fuku/modules/settings_antiflood.go (read seam, redraw), settings_mreq.go:361-386 (switch helper)
var settingsXRead = pkg.LoadXContext            // seam: a test makes the read fail
func settingsXSet(p *settingsPress, value string) *settingsScreen {
	if value != "0" && value != "1" { /* common_callback_invalid_request alert */ return nil }
	ctx, cancel := context.WithTimeout(context.Background(), settingsReadTimeout); defer cancel()
	before, ok := settingsXReadBefore(ctx, p)                // cannot-open edit on error
	if !ok { return nil }
	return settingsXRedraw(ctx, p, applyX(ctx, p.group.Id, value == "1"), before)
}
```

### Anti-patterns

- Reading through `GetWarnSettingContext`, `GetChatReportSettings`, `GetAdminSettings`, `IsAISpamEnabled`, `logchannels.GetContext` or `getCachedLanguage` from a page (C-05).
- Calling `lang.ChangeUserLanguage` from the Language page (C-07) or writing the user's language in a private menu.
- A second write path, or an apply function that sends a reply (the command words its reply from the result).
- A `tg`-only or `st`-only control that accepts `v=1` (breaks `TestSettingsControlActionAllowList`).
- Printing the channel ID, title or Staff Group in any group-menu text; reusing `logs_current`.
- Copying `settingsFloodLimitRow` for the warn stepper (use `settingsStepperRow`; the limit has no Off).
- Re-recording a golden or a pinned test to make a change pass.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| Typed apply result and read-back | a new result style | `floodApplyResult` / `greetingApplyResult` shapes, `applyReadBack` | `applyReadBack` already logs and handles a failed read-back without undoing the write |
| Toggle page plumbing | per-page copies | one generic helper over the `settingsMreqSetSwitch` shape | dupl and drift |
| Stepper | a custom row | `settingsStepperRow` | bound marks and absolute targets |
| Page of controls from a table | per-control branches | one control per table row registered in `init` (`settings_deletion.go:86-128`) | a later lock is one row |
| Language names | new keys | `keyboard.GetLangFormat` / `language_name`, `language_flag` | already in all 8 locales |
| Rights | ad hoc checks inside `Set` | `settingsRight` values and `settingsRightRefusalFor` | the typed-reply door shares the pure part |
| Command-parity proof | an assertion on a literal | goldens (`legacyRequestJSON`) + full-row snapshot comparison | observable behaviour only |

## Common Pitfalls

1. **A reader that creates a row or hides an error** (C-05): the page would write a row by being opened, or show "off" for a failed read. Avoid: the six readers, each tested for "no row created".
2. **Golden recorded after the extraction** proves nothing: record in its own commit first, with the storage side effect, never re-record.
3. **`/anonadmin` read-before-owner-check creates a row** for a refused non-owner: keep the command's initial read; `applyAnonAdmin` takes the current value from the caller.
4. **Language swap in a private menu** would draw the page in the group's language for a presser who reads another one: swap only when `!p.private`; and build the translator from the written code.
5. **A control that accepts `v=1` for `tg` but not `st`** fails `TestSettingsControlActionAllowList` for every other page: declare no allow-list or reject `v=1`.
6. **Pager row after the nav row**: `settings_menu_test.go:478-510` requires the last row to be Back and Close.
7. **Locks with a localized label over 18 code points** truncates on phones; the 22 x 8 label check (S-03) is the guard.
8. **`UpdateLock` relies on the `locked` default being false** for unlock; never change the model tag or the column default.
9. **Warn mode empty after `/reset warnings`**: the page must show "no action at the limit" (C-03), not a mark on a choice.
10. **Skew**: 10 s on another replica for `locks_map`, `warn_settings`, `chat_lang` and the rest; state it, do not fix it with `skipLocal`.
11. **Main fit tests use the registry**: a new state string longer than the current worst one needs `settingsMainWorstState` updated or the fit test under-reports.
12. **Lint**: a `settingsLocks*` renderer over 19 branches, three copied toggle pages, an identifier with `pass` in it.

## Code Examples

### Apply function shape (follows `applyFloodMode`, antiflood.go:493-498)

```go
// Source: fuku/modules/antiflood.go (applyFloodMode) - the shape to copy
var warnModes = []string{"ban", "kick", "mute"}

func applyWarnMode(ctx context.Context, chatID int64, mode string) warnSettingsResult {
	if !slices.Contains(warnModes, mode) {
		return warnSettingsResult{Outcome: warnUnknownMode}
	}
	return warnSettingsSaved(ctx, chatID, warns.SetWarnMode(chatID, mode)) // read back, never assume
}
```

### Characterization record (follows rules_legacy_request_test.go)

```go
// Source: fuku/modules/rules_legacy_request_test.go:19-48
raw, _ := json.Marshal(call.Params) // encoding/json sorts map keys: stable string to compare
```

## State of the Art

| Old approach | Current approach | Impact |
|---|---|---|
| Command code writes and replies inline | apply function + typed result, the command words its own reply | page and command cannot diverge (PAGE-10) |
| Page reads the command's getter | page reads an error-returning reader on the same cache key | no hidden defaults, no row created by reading |
| Fixed right enum of five | two composite rights appended | the menu refuses exactly what these two commands refuse |

No deprecated library is involved.

## Runtime State Inventory

Not a rename or migration phase; omitted. (Existing state touched without change: the rows of `warns_settings`, `report_chat_settings`, `ai_spam_settings`, `greeting_settings`, `admin`, `locks`, `log_channels` and `chats.language`, written only through the existing repository functions.)

## Assumptions Log

| # | Claim | Section | Risk if wrong |
|---|-------|---------|---------------|
| A1 | The new strings number about 80 to 100 `settings_` keys (330 today), 22 of them the lock labels | Summary, S-03 | only the size of the locale and limits-table work; the plan counts the real number |
| A2 | The first greetings write on a chat with no row does or does not flip a visible Welcome or Goodbye setting (AGENTS.md says a first Set turns the goodbye on) | C-09 | a notice may be needed on the Switches page; SPIKE check 6 and the golden decide |
| A3 | New Main state strings keep a state line near 46 UTF-16 units, so Main stays near 1.4 thousand units | C-11 | the fit test measures it; the limit is 3800 |
| A4 | Labels of at most 18 code points exist for all 22 locks in hi, ru, uk, es, pt, fr | S-03 | a longer name is shortened (never the cap raised); native-speaker read |
| A5 | Phone clients show a 20-code-point two-column cell untruncated (the Phase 9 to 17 assumption) | S-03, S-04 | live check on phone widths |
| A6 | The command's second bot check (`IsBotAdmin`) and the first (`IsUserAdmin` over the list) rarely disagree | S-02 | the menu is a hair more permissive in a rare cache state; documented carve-out |
| A7 | GORM omits a zero Go `false` with a `default:false` tag from the INSERT, so `ON CONFLICT ... locked = excluded.locked` writes the column default `false` | S-01 | the existing unlock tests would fail first; unchanged code |

## Open Questions

1. **Does the Log channel page post the `#SETTINGS` audit line like `/log` does? (Q1)** - genuinely open, owner decides; recommendation: yes, best effort after the redraw through `actionlog.Settings`, gated by `Destination` as today, because it is the only audit trail of a settings change and a page toggle would otherwise be invisible to the log channel the toggle controls. Alternative: no post (settings pages write no audit elsewhere).
2. **Confirmation card for `all` and `messages` locks? (Q2)** - RESOLVED: none, for parity ("refused exactly as by its command") and because one tap undoes it; one hint line says administrators and approved users are exempt. The owner may add a card later (a Go table field, no migration).
3. **Warn mode empty after `/reset warnings` (Q3)** - RESOLVED for this phase: the page shows it read-only (C-03); the defect itself (`clearWarns` writes no mode, backup.go:1072, so the limit silently does nothing) is out of scope and is recorded as a follow-up for the owner (store `mute` in the reset, one line plus a backup test).
4. **Page keys, `Order` values, icons and the lock page size (Q4)** - RESOLVED as recommendations in S-07, C-11 and S-03; the UI-SPEC may change them within the free codes.
5. **Anonymous admins page visibility (Q5)** - RESOLVED: not `OwnerOnly` (the command's status view is open to everyone); only writes need the creator (C-01).
6. **Owner confirmations:** A18-1 becomes "writes need the creator, as the command" (no conflict); A18-2 is reworded (C-03); A18-3 stands; A18-4 stands (S-04); A18-5 stands (no second hub, C-11); A18-6 stands. New for the owner: A18-7 the audit post (Q1) and A18-8 no card for `all` (Q2).
7. **`ENABLED_LOCALES` with fewer than 8 languages (Q6)** - RESOLVED: the page lists what `/lang` lists (`ValidLangCodes`), the apply function accepts the 8 like the callback.

## Environment Availability

| Dependency | Required by | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build and tests | yes | go1.26.0 linux/amd64 | none needed |
| gcc (cgo for go-sqlite3) | tests | yes | /usr/bin/gcc | none needed |
| PostgreSQL 16 binaries | optional integrity run | yes | `/usr/lib/postgresql/16` | CI runs it; SQLite covers the logic |
| golangci-lint | `make lint` | no (2.5.0 built with go1.25.1, cannot lint a Go 1.26 module) | - | `go vet`, `gofmt -l`, manual dupl and gocyclo read |
| gosec | G101 and friends | yes | `~/go/bin/gosec` | - |
| Node 22 | captcha page tests only | yes | v22.23.4 | not touched by this phase |
| Bot API site | live-behaviour citations | not needed | - | live checks below |

**Missing dependencies with no fallback:** none.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing`, `-tags testtools -race`, testify assertions only, `internal/testdb.Run` (SQLite), miniredis, hand-written `gotgbot.BotClient` fakes |
| Config file | none (Makefile targets) |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestLegacy\|TestApply\|TestSettings(Warnings\|Switches\|Language\|LogChannel\|AnonAdmin\|Locks\|Parity\|Drift))' ./fuku/modules` (the `\|` is the table escape; type a plain pipe) |
| Settings family | `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (about 4.5 minutes) |
| Full suite command | `make test` (the only valid full run), plus `make test-postgres-integrity`, `make check-translations`, `make check-docs`; lint stand-in |

### Phase Requirements to Test Map

| Req | Behaviour | Test type | Automated command | File exists? |
|-----|-----------|-----------|-------------------|--------------|
| PAGE-01 | limit stepper and mode choices write exactly as `/setwarnlimit` and `/setwarnmode`; empty or `tban` mode shown read-only | parity, drift, authority | `-run '^(TestSettingsParityWarnings\|TestSettingsDriftWarnings\|TestSettingsAuthorityParityWarnings\|TestSettingsWarnings)'` | Wave 0 (18-01, 18-02) |
| PAGE-02 | three switches; AI Off-only and refusal when not configured; auto-approve right incl. bot admin | parity per control, authority, gate | `-run '^(TestSettingsParity(Reports\|AISpam\|AutoApprove)\|TestSettingsSwitches\|TestSettingsAISpamGate)'` | Wave 0 (18-03 to 18-05) |
| PAGE-03 | language written through `applyGroupLanguage`; group menu redraws in the new language, private menu does not and never writes the user language | integration | `-run '^TestSettingsLanguage'` | Wave 0 (18-06) |
| PAGE-04 | category toggles, not-linked refusal, no channel title or ID anywhere | parity, privacy | `-run '^(TestSettingsLogChannel\|TestSettingsParityLogCategory)'` | Wave 0 (18-07, 18-08) |
| PAGE-05 | creator writes, administrator refused, bot-not-admin refused, order of refusals; same as `/anonadmin` | authority parity | `-run '^(TestSettingsAnonAdmin\|TestSettingsAuthorityParityAnonAdmin\|TestSettingsRights)'` | Wave 0 (18-03, 18-09) |
| PAGE-06 | 22 locks from one table equal to the command's set; paging; absolute toggles; bytes | table, parity, bytes | `-run '^(TestSettingsLocks\|TestSettingsParityLocks)'` | Wave 0 (18-10, 18-11) |
| PAGE-10 | goldens identical after extraction; readers share the commands' keys; whole-row parity | golden, repository | `-run '^TestLegacy'` ; `go test -tags testtools -race -count=1 ./fuku/db/warns ./fuku/db/reports ./fuku/db/aispam ./fuku/db/admin ./fuku/db/logchannels ./fuku/db/lang` | Wave 0 |
| PLAT-05 | every key in 8 locales, limits rows, label lengths, fit | i18n | `make check-translations` ; `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths)' ./fuku/i18n` | exists, tables extended |

### Sampling Rate

- **Per task commit:** the quick run command for the files touched.
- **Per plan merge:** the settings family; at 18-03, 18-05, 18-11, 18-12 and 18-13 also the whole `./fuku/modules` package (`go test -tags testtools -race -count=1 ./fuku/modules`, 9 to 15 minutes) and `./fuku/i18n`.
- **Phase gate:** `make test`, `make test-postgres-integrity`, `make check-translations`, `make check-docs` and the lint stand-in green before `/gsd-verify-work`.

### Wave 0 Gaps

- [ ] `fuku/modules/*_legacy_request_test.go` for the eight commands (goldens, committed first).
- [ ] Six reader tests in `fuku/db/{warns,reports,aispam,admin,logchannels,lang}`.
- [ ] Page-prefixed cleanup helpers and `withAISpamConfigured`/`withAISpamNotConfigured`.
- [ ] Rows in `settingsLocaleLimits` and the new groups in `TestSettingsLabelLengths`.
- Framework install: none.

## Security Domain

See S-09 for the STRIDE table and the ASVS notes. Secrets: none new; the TypeSafe key is read from `config.AppConfig` and never logged or shown (it is already registered with `logredact` where it is configured).

## Live checks for `18-SPIKE.md` (owner steps, deferred under the standing choice)

1. **Language redraw on two replicas:** change the group language from a group menu and press again on the other replica inside 10 seconds (old language possible, as `/lang`); and a private menu opened for the group keeps the opener's language.
2. **Phone widths:** the 22 lock cells and the 8 language cells in hi, ru, uk, es, pt, fr on Android, iOS and Desktop; the six-page Main grid on a phone.
3. **`/anonadmin` from the menu:** creator turns it on and off; an administrator who is not the creator gets the owner alert; the bot demoted gets the bot notice; an anonymous admin's commands then run without the proof tap.
4. **Log channel:** with a real linked channel, toggle each category and confirm which posts stop (admin: staff and lockdown copies; settings: the `#SETTINGS` audit line); the page in a group menu shows no channel title or ID.
5. **Locks:** toggle `all`, `messages`, `url`, `bots` from the menu in a real group; non-administrators are deleted, administrators and approved users are not; the `bots` lock with a bot that cannot restrict.
6. **First writes on PostgreSQL:** a group with no greetings row presses Auto-approve (does Welcome or Goodbye change?), no `warns_settings`, `report_chat_settings`, `admin` or `locks` row (FK and defaults), warn mode after `/reset warnings`.
7. **AI spam line:** with and without `TYPESAFE_API_KEY`, and with `ENABLE_AISPAM=false`: the status line and the Off-only form; a stale On press.
8. **Native-speaker read** of every new string, the 22 lock labels first.

## Sources

### Primary (HIGH confidence)
- Repository files read this session: `fuku/modules/{warns.go, reports.go, aispam.go, greetings.go (125-250, 740-910, 1395-1516), language.go, logchannels.go, admin.go (575-660, 745-815), locks.go, antiflood.go (430-540), settings_pages.go, settings_menu.go, settings_card.go, settings_antiflood.go, settings_captcha.go (1-60), settings_lockdown.go (14-60), settings_deletion.go (86-135), settings_staff.go (28-75, 316-329), settings_access.go, i18n_helpers.go}`, `fuku/db/{warns,reports,aispam,admin,logchannels,locks,lang,greetings,cache}/*.go`, `fuku/db/models/{warns,aispam,locks,logchannels,reports,admin}.go`, `fuku/utils/chat_status/{access.go, chat_status.go}`, `fuku/utils/helpers/command_pipeline.go`, `fuku/utils/actionlog/actionlog.go`, `fuku/utils/callbackcodec/callbackcodec.go`, `fuku/utils/keyboard/keyboard.go`, `fuku/config/config.go (496-502)`, `fuku/i18n/settings_locale_test.go`, tests `settings_main_test.go`, `settings_menu_test.go (296-420, 470-520)`, `settings_edit_hooks_test.go (130-250)`, `settings_mreq_test.go (1425-1470)`, `rules_legacy_request_test.go`, `settings_env_test.go`, `Makefile`, `.golangci.yml`, `locales/en.yml` (keys quoted above), AGENTS.md, the Phase 16 and 17 research and review files.
- A computation over `locales/*.yml` (Python, UTF-16 units of the Main page in the worst case).

### Secondary / Tertiary
- Phone rendering, first writes on PostgreSQL and the Telegram behaviour of `all` locks are `[ASSUMED]` and listed as live checks.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new.
- Architecture: HIGH, each seam read in the repository.
- Pitfalls: HIGH for code-derived ones (C-01 to C-14), MEDIUM for client rendering and first-write side effects.

**Research date:** 2026-11-12
**Valid until:** 2026-12-12 (stable; revisit if a page, a right or a command file changes before planning)
