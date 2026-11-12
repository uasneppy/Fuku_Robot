# Phase 18: Remaining Settings Pages - Context

**Gathered:** 2026-11-12
**Status:** Ready for planning (decisions D-01 to D-14 are the recommended defaults; the owner has not confirmed them, see "Provisional status")

<domain>
## Phase Boundary

Phase 18 moves the remaining simple group settings onto the settings menu, one page per setting family, on the framework Phases 9 to 16 built (`registerSettingsPage`, the owner gate, the press pipeline, the shared card and stepper helpers).

- **Warnings page** (PAGE-01): the warn limit and the action at the limit, stored exactly as `/setwarnlimit` and `/setwarnmode` store them.
- **Switches page** (PAGE-02): reports (`/reports`), the AI spam filter (`/aispam`) and auto-approve of join requests (`/autoapprove`), each on or off.
- **Language page** (PAGE-03): the group's language (`/lang`); the menu redraws in the new language.
- **Log channel page** (PAGE-04): shows the group's log channel and turns each log category on or off (`/logcategories`, `/log`, `/nolog`); linking a new channel stays a command (`/setlog`).
- **Anonymous admins page** (PAGE-05): the anonymous-admin mode, with the same rights `/anonadmin` needs.
- **Locks page** (PAGE-06): each lock type on or off, paged (`/lock`, `/unlock`, `/locks`, `/locktypes`).
- **Parity** (PAGE-10): every setting changed on these pages is stored, cached and refused exactly as by its command, and the page and the command always show the same value (MENU-08 of v1.0).
- **PLAT-05 binds**: every new string exists in all 8 locales, with the limits tables and label-length tests.

Out of scope: the Blacklist, Filters and Notes pages and the typed-input flows they need (Phase 19), linking or unlinking a log channel from the menu, a typed number for any setting, and changing what any command accepts or replies.

</domain>

<provisional_status>
## Provisional status

As in Phase 17, the owner was not asked these grey areas in this window: an earlier question was interrupted and the run was asked to continue without further questions. Each decision below is the default recommended from the ROADMAP's closed text, the requirements and the existing design. None is recorded as confirmed. They are listed as owner confirmations A18-1 to A18-6 in the STATE deferred items, and every one is reversible without a migration (Go tables and apply functions only).

</provisional_status>

<decisions>
## Implementation Decisions

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

</decisions>

<code_context>
## Existing Code Insights

### Reusable assets
- The settings framework: `registerSettingsPage`, `settingsControl` and its rights (`settingsRightAdmin`, `settingsRightAdminBotAdmin`, `settingsRightAdminFloodBot`, `settingsRightRestrict`, `settingsRightChangeInfo`, `settingsRightOwner`), the shared card (`settings_card.go`), the stepper row, `settingsKeyboard.navRow`, the owner gate, `settingsPress.verify` and `answer`, and the seam pattern for reads (`settingsCaptchaRead`).
- The model pages: Captcha (`settings_captcha.go`, `applyCaptchaTimeout`), Antiflood (`settings_antiflood.go`, `applyFlood*`, the first-write rule D-27), the editor switches (`applyWelcomeEnabled`), Member requirements (`settings_mreq.go`, hub with row screens) and Deletion (`settings_deletion.go`, sections and tables).
- The commands: `/setwarnlimit` and `/setwarnmode` (`warns.go`, registered through a table with `CheckDisabled` and a handler-level check), `/reports` (`reports.go`), `/aispam` (`aispam.go`, `aispam.IsAISpamEnabled`), `/autoapprove` (`greetings.go`), `/lang` (`language.go`), `/setlog`, `/unsetlog`, `/logchannel`, `/logcategories`, `/log`, `/nolog` (`logchannels.go`, raw handlers), `/anonadmin` (`admin.go`, `WrapCommand` with `RequireGroup` and `RequireBotAdmin`), `/lock`, `/unlock`, `/locks`, `/locktypes` (`locks.go`, raw handlers).

### Integration points
- `AGENTS.md` (the settings menu bullets and the one-setting-one-code-path rule), the 8 locale files, `fuku/i18n/settings_locale_test.go`, `TestSettingsStaffPrivacy`, `TestSettingsGateAllDoors`, the Main grid tests (`TestSettingsMain*`), and the settings toolkit (`settings_env_test.go`).

</code_context>

<specifics>
## Specific Ideas

- Raising the warn limit from the menu leaves the stored row and cache exactly as `/setwarnlimit 5` does.
- Switching a language on the Language page redraws that page in the new language in a group menu.
- The Locks page opens at page 1 and pages with absolute `v` values so a double tap is harmless.

</specifics>

<deferred>
## Deferred Ideas

- The Blacklist, Filters and Notes pages (Phase 19) and the typed-input flows they need.
- Linking, unlinking or changing the log channel from the menu; a typed warn duration; per-lock exceptions or allowlists.
- Making the Anonymous admins page owner-only (needs the command stricter too).
- A second hub screen grouping the pages, unless the row budget forces it.

</deferred>

<owner_confirmations>
## Owner confirmations carried to the phase (not yet answered)

- **A18-1** (D-08) The Anonymous admins page uses the rights `/anonadmin` needs today, not owner-only.
- **A18-2** (D-04) A warn mode that needs a duration shows it read-only and is changed through the command.
- **A18-3** (D-07) The log channel's title and ID are never shown on the page; linking stays a command.
- **A18-4** (D-06) In a private menu the language stays the opener's own and the group's language is shown in it.
- **A18-5** (D-10) The six pages join the main grid in the two-column layout (a second hub only if forced).
- **A18-6** (D-11) The pages are shown in a Staff Group like the existing pages.

</owner_confirmations>
