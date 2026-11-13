---
phase: 18-remaining-settings-pages
verified: 2026-11-13T04:20:00Z
status: gaps_found
score: "13/14 truths verified in code and tests (5 ROADMAP success criteria plus 9 supporting truths); 1 gap: the group branch of the change_language callback still writes through lang.ChangeGroupLanguage directly, not through applyGroupLanguage, so the Language page has a second write path beside its command (behaviour is identical and proven by the parity test, the structure the plan, the SUMMARY and AGENTS.md claim is not there). PAGE-01 to PAGE-06 and PAGE-10 and PLAT-05 are satisfied in behaviour; PAGE-10's 'one code path' wording is met for eight of nine settings"
covered_files:
  - ".planning/phases/18-remaining-settings-pages/18-01-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-01-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-02-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-02-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-03-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-03-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-04-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-04-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-05-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-05-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-06-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-06-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-07-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-07-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-08-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-08-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-09-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-09-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-10-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-10-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-11-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-11-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-12-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-12-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-13-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-13-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-14-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-14-SUMMARY.md"
  - ".planning/phases/18-remaining-settings-pages/18-15-PLAN.md"
  - ".planning/phases/18-remaining-settings-pages/18-15-SUMMARY.md"
  - "AGENTS.md"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "fuku/db/admin/repository.go"
  - "fuku/db/aispam/repository.go"
  - "fuku/db/lang/repository.go"
  - "fuku/db/logchannels/repository.go"
  - "fuku/db/reports/repository.go"
  - "fuku/db/warns/repository.go"
  - "fuku/modules/admin.go"
  - "fuku/modules/aispam.go"
  - "fuku/modules/greetings.go"
  - "fuku/modules/language.go"
  - "fuku/modules/locks.go"
  - "fuku/modules/logchannels.go"
  - "fuku/modules/reports.go"
  - "fuku/modules/settings_anonadmin.go"
  - "fuku/modules/settings_language.go"
  - "fuku/modules/settings_locks.go"
  - "fuku/modules/settings_logchannel.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_switches.go"
  - "fuku/modules/settings_toggle.go"
  - "fuku/modules/settings_warns.go"
  - "fuku/modules/warns.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
covered_digest: "v4:sha256:5e5036a1de938821cdbe290f7e79cf0f503143bda788c9d840a35c5267eb1ebb"
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "Every setting has one write path: its command and its menu page both call the setting's apply function (plan 18-07 truth 'the callback's group branch maps onto applyGroupLanguage'; AGENTS.md 'One setting, one code path'; ROADMAP cross-cutting constraint 'every menu control calls its command's apply function')"
    status: failed
    reason: "applyGroupLanguage (fuku/modules/language.go:61) is called only from the Language page (settings_language.go:171). The group branch of the change_language callback (langBtnHandler, language.go:192) still calls lang.ChangeGroupLanguage directly, and never touches the languageWrites seam, the groupLanguageCodes-based guard inside the apply function or the cancelled-context refusal. Commit cd87587's diff shows only the extraction and the groupLanguageCodes substitution in the callback's code check; the branch itself was never edited. The other eight apply functions (applyWarnLimit, applyWarnMode, applyReportsEnabled, applyAutoApprove, applyAISpamEnabled, applyLogCategory, applyAnonAdmin, applyLock) are each called by both the command and the page. Behaviour is identical (both reach lang.ChangeGroupLanguage, and TestSettingsParityLanguage drives the real callback and the page and compares the stored row, the dropped keys and the cached entry), so success criterion 5 holds in behaviour; what fails is the structural claim and three documents that state it"
    artifacts:
      - path: "fuku/modules/language.go"
        issue: "the else branch of langBtnHandler (around line 192) calls lang.ChangeGroupLanguage(chat.Id, language) and builds replyString itself; applyGroupLanguage has no caller but the page"
      - path: "AGENTS.md"
        issue: "line ~972 and the Language page bullet say applyGroupLanguage is the function the group branch of the change_language callback uses; that is not true of the code"
      - path: ".planning/phases/18-remaining-settings-pages/18-07-SUMMARY.md"
        issue: "says 'its group branch maps the result'; the code does not"
    missing:
      - "map the callback's group branch onto applyGroupLanguage(context.Background(), chat.Id, language): on langSaveFailed answer common_settings_save_failed as today, on langApplied use the result's Key and Params for replyString; keep the order (code check, admin check, apply) and every answer, so TestLegacyLanguageRequests passes unchanged"
      - "add a test that fails if the callback bypasses the function, for example by swapping the languageWrites seam to a recording write and pressing the real callback (TestApplyGroupLanguage tests the function alone and TestSettingsParityLanguage cannot tell the two paths apart)"
      - "or, if the owner prefers to keep the direct call, correct AGENTS.md and 18-07-SUMMARY.md and drop the claim that the callback shares the function"
human_verification:
  - test: "Warn limit and each action in a real supergroup (18-SPIKE.md checks 1 to 3)"
    expected: "Mute, Ban and Kick are applied at the limit and warns are cleared; /warnings shows what the page shows; a stored tban shows the inert line; /reset warnings' empty action is scheduled for a follow-up fix"
    why_human: "the page stores what the commands store, only a live group shows the real action on a real member"
  - test: "Switches page in each configuration (checks 4 to 6)"
    expected: "the AI line in every provider configuration and a stale On press refused with the alert; Auto-approve's first press on a group with no greetings row on PostgreSQL; reports on and off from the page change what /report does"
    why_human: "needs real provider configuration, PostgreSQL and a live /report"
  - test: "Language page on a live bot (checks 7 and 8)"
    expected: "a group menu redraws in the new language, another replica shows it within CACHE_LOCAL_TTL, a private menu keeps the opener's language and never writes users.language"
    why_human: "two replicas and a real client"
  - test: "Log channel and Anonymous admins pages with a real channel and creator (checks 9 to 12)"
    expected: "categories stop and resume posts, the #SETTINGS line appears, no screen names the channel, the creator switches anonymous-admin mode, a non-owner administrator and a demoted bot are refused, and the page opens while the bot is not an administrator (accepted looseness)"
    why_human: "a real log channel, a real anonymous admin and bot demotion"
  - test: "Locks page in a real group (checks 13 to 15)"
    expected: "all, messages, url, bots and previews act at once; comments, anonchannel, rtl and forward need particular live messages; the right line shows when the bot cannot delete"
    why_human: "needs live messages of those kinds"
  - test: "Phones and wording (checks 16 to 19)"
    expected: "22 lock cells, eight language cells and ten Main rows fit on a phone, the flag of Bahasa Indonesia has the width the code assumes, the 55 new strings read well to native speakers"
    why_human: "visual and linguistic"
  - test: "Stale /settings help and the owner confirmations (checks 20 to 25)"
    expected: "decide whether settings_help_msg (and the generated docs page) should name the Deletion page and the six new pages; confirm or change A18-1 to A18-9, the flagged UI points and the four deviation notes; confirm CI lint and the full run"
    why_human: "owner decisions and a CI-only linter"
---

# Phase 18: Remaining Settings Pages Verification Report

**Phase Goal:** As a group owner, I want warnings, reports, language, the log channel, anonymous-admin mode and locks on menu pages, so that I can configure them without commands.
**Verified:** 2026-11-13T04:20:00Z
**Status:** gaps_found (one structural gap; every success criterion holds in behaviour; the owner's live checks are still deferred)
**Re-verification:** No, initial verification

The expected status was `human_needed`. I found one real gap in code (one of nine write paths is not shared) and report `gaps_found` as asked. It is small, has no user-visible effect today and is a few lines to close. With it closed the status would be `human_needed` for the deferred live checks of `18-SPIKE.md`.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1: a Warnings page sets the warn limit and the action; a switches page turns reports, `/aispam` and auto-approve on or off | VERIFIED | `settings_warns.go` registers page `n` (Order 42) with controls `l` and `m`, both `st` only, right `settingsRightAdminFloodBot`; they call `applyWarnLimit` (line 228) and `applyWarnMode` (line 246). `settings_switches.go` registers page `t` (Order 86) with toggles `r`, `a`, `j` calling `applyReportsEnabled`, `applyAISpamEnabled`, `applyAutoApprove` (lines 211, 229, 245). `TestSettingsPhase18EndToEnd` subtests "warnings", "ai spam", "reports", "auto approve" drive the real dispatcher: the page stores limit 2 and ban, two `/warn` commands then produce exactly one `banChatMember`, warns cleared; Mute gives one restrict with no rights, Kick one member-removing unban; a stored `tban` applies nothing |
| 2 | SC2: a Language page sets the group's language and the menu redraws in it | VERIFIED | `settings_language.go` page `u` (Order 87), control `l` `st` only, right `settingsRightAdmin`; `settingsLangSet` calls `applyGroupLanguage` (line 171) and in a group menu swaps `p.tr` to `i18n.MustNewTranslator(code)`. E2E "language": `/rules` answers in uk before, in es after the page press, stored language es, the owner's own `users.language` untouched |
| 3 | SC3: a Log channel page shows the channel and switches each category (linking stays a command); an Anonymous admins page with `/anonadmin`'s rights | VERIFIED | `settings_logchannel.go` page `o` (Order 88), six categories from one table, `tg` only, right `settingsRightAdmin`, writes through `applyLogCategory` (line 174), reads through `logchannels.LoadContext`, renders `settingsLogState` only (no row, no channel title or ID). `settings_anonadmin.go` page `y` (Order 89), control `m` declares `settingsRightOwnerBotAdmin` (the only control that does), writes through `applyAnonAdmin` (line 107). `/anonadmin`'s own checks (admin.go ~675: `RequireUserOwner`; descriptor `RequireBotAdmin`) match. E2E "log channel" and "anonymous admins" pass |
| 4 | SC4: a Locks page turns each lock type on or off, paged | VERIFIED | `settings_locks.go` page `k` (Order 44), table `settingsLockRows` of 22 rows (12 on page 1, 10 on page 2, two-letter codes), pager action `kg` with field `v`, right `settingsRightAdminFloodBot` (matches `/lock`'s `IsUserConnected(true,true)` plus `RequireBotAdmin` plus `RequireUserAdmin`), writes through `applyLock` (line 207). `/lock` and `/unlock` call `applyLockList` which calls `applyLock` per name (locks.go 310, 385). `TestSettingsLocksTableMatchesCommand` and `TestSettingsLocksKeysExist` ran green; E2E "locks" passes |
| 5 | SC5: every setting changed on these pages is stored, cached and refused exactly as by its command, and page and command always show the same value | VERIFIED (behaviour) | Parity, drift and authority tests exist and pass for each setting (`TestSettingsParity*`, `TestSettingsDrift*`, `TestSettingsAuthorityParity*`). `TestSettingsParityLanguage` presses the real callback and the page on two chats and compares the stored row, the dropped keys and the cached entry. Goldens pin every command's requests (truth 10). See truth 7 for the structural caveat |
| 6 | The six pages are registered with keys `n t u o y k`, Orders 42, 86, 87, 88, 89, 44, and Main lists them in the final grid | VERIFIED | `registerSettingsPage` in each of the six files (no `Hidden`, no `OwnerOnly`). `settings_main_test.go:36-38`: group shape `s|l / c|f / n|k / w|g / r|m / d|t / u|o / y / a / close`, private shape ends `groups|close`, `settingsMainOwnerRows = 10`. `TestSettingsMainGrid`, `TestSettingsPhase18Codes` (page keys unique, action codes belong to one page, `kg` is not a core code) pass |
| 7 | Each setting is written only through its apply function, called by both the command and the page | FAILED (BLOCKER for the structural claim, no behavioural effect) | Eight of nine hold: `applyWarnLimit`/`applyWarnMode` (called from `warns.go:673` and `:146` and the page), `applyReportsEnabled` (`reports.go:87`), `applyAutoApprove` (`greetings.go:826`), `applyAISpamEnabled` (`aispam.go:901`), `applyLogCategory` (`logchannels.go:240`), `applyAnonAdmin` (`admin.go:646`), `applyLock` via `applyLockList`. A grep for the repository writers outside `fuku/db` finds each only behind its seam, except `language.go:192`, where the callback calls `lang.ChangeGroupLanguage` directly. `applyGroupLanguage` has one production caller (the page). See Gaps |
| 8 | Each page's control declares the right its command asks for | VERIFIED | Warnings and Locks and Reports `settingsRightAdminFloodBot`; AI spam `settingsRightAdmin` (`/aispam`: `RequireUserAdmin` only); Auto-approve `settingsRightChangeInfoBotAdmin` (`/autoapprove`); Language and Log channel `settingsRightAdmin`; Anonymous admins `settingsRightOwnerBotAdmin`. The two composite rights are the only framework edit, in `settings_menu.go`; `settings_pages.go` is untouched. `TestSettingsRights`, `TestSettingsRightRefusalForParity` and the authority-parity tests ran green |
| 9 | The page readers create nothing and return their errors | VERIFIED | Read `LoadWarnSettingsContext`, `LoadChatStatusContext`, `LoadEnabledContext` (cached getter, miss returns an unsaved default), `LoadSettingsContext`, `LoadGroupLanguageContext`, `logchannels.LoadContext` (nil for no channel), `LoadAnonAdminContext`: none inserts. `TestSettingsPhase18ReadsCreateNothing` and the per-page `...ReadsCreateNothing` tests ran green |
| 10 | A characterization golden exists for each extraction | VERIFIED | `warns_legacy_request_test.go`, `reports_legacy_request_test.go` (two tests), `autoapprove_`, `aispam_`, `language_`, `logchannels_`, `admin_`, `locks_legacy_request_test.go`: `TestLegacy{WarnSettings,Reports,ReportsPrivate,AutoApprove,AISpam,Language,LogCategory,AnonAdmin,Lock}Requests` all ran green. The language golden would pass with either callback path, which is why it did not catch the gap |
| 11 | All new locale keys exist in all 8 locale files (PLAT-05) | VERIFIED | `make check-translations`: "All translations are present". My own diff of the key sets: en, es, fr, hi, id, pt, ru, uk all have 385 `settings_` and 31 `mreq_` keys, with no key missing or extra against en. Every literal key in the six page files exists in en. Table-field keys: `TestSettingsLocksKeysExist` ran green; the limits-table guard `TestSettingsLabelLengths` lives in `fuku/i18n` and was not matched by the requested `-run` pattern (see below) |
| 12 | No migration, model, Redis family, handler group or shutdown drain was added | VERIFIED | `git diff --stat 29aca33..2dff096 -- migrations fuku/db/models main.go fuku/modules/settings_pages.go` is empty. A grep over added lines for `go func`, `RegisterHandler`, `AddHandlerToGroup` finds nothing |
| 13 | Docs describe the pages (README, `callbacks.md`, `permissions.md`, AGENTS.md) | VERIFIED with one inaccuracy | `make check-docs`: "No drift detected". README has paragraphs for the new pages, `callbacks.md` and `permissions.md` carry the keys, codes, rights. AGENTS.md has a bullet per page and the nine-apply-function paragraph. Inaccuracy: AGENTS.md (~line 972 and the Language bullet) and `18-07-SUMMARY.md` claim the callback uses `applyGroupLanguage` (gap above). `settings_help_msg` does not name the new pages (known, 18-SPIKE.md check 20) |
| 14 | `TestSettingsPhase18EndToEnd` proves behaviour through the dispatcher | VERIFIED | 576-line test, eight subtests (warnings, locks, ai spam, anonymous admins, language, log channel, reports, auto approve), each presses the page through the real dispatcher and then runs the real command or message path and asserts what the bot does. Ran green with `-race` |

**Score:** 13/14 truths verified (0 present, behaviour-unverified in code terms; the live Telegram behaviour is listed for the owner below).

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|----------------|-------------|--------|----------|
| PAGE-01 | 18-01, 18-03, 18-14 | Warnings page: limit and action at the limit | SATISFIED | truth 1, E2E "warnings", `TestSettingsParityWarnLimit`, `TestSettingsParityWarnMode` |
| PAGE-02 | 18-04 to 18-06, 18-14 | Reports, AI spam filter, auto-approve switches | SATISFIED | truth 1, E2E "reports", "ai spam", "auto approve", `TestSettingsParity{Reports,AISpam,AutoApprove}` |
| PAGE-03 | 18-07, 18-14 | Language page, menu redraws in the new language | SATISFIED | truth 2, E2E "language", `TestSettingsLanguage*` |
| PAGE-04 | 18-08, 18-09, 18-14 | Log channel shown, categories switched, linking stays a command | SATISFIED | truth 3, `TestSettingsLogChannel*`, `TestSettingsParityLogCategory` |
| PAGE-05 | 18-04, 18-10, 18-14 | Anonymous-admin mode with `/anonadmin`'s rights | SATISFIED | truth 3, `TestSettingsAnonAdmin*`, `TestSettingsAuthorityParityAnonAdmin` |
| PAGE-06 | 18-11, 18-12, 18-14 | Each lock on or off, paged | SATISFIED | truth 4, `TestSettingsLocks*`, E2E "locks" |
| PAGE-10 | 18-01 to 18-14 | Every setting stored, cached and refused exactly as by its command | SATISFIED in behaviour, not in the "one code path" wording for the language callback | truth 5 and truth 7. The tick in REQUIREMENTS.md is true as to observable behaviour; the owner may prefer to hold it until the gap is closed |
| PLAT-05 | 18-02, 18-11, 18-13 | New messages and button labels in all 8 locales | SATISFIED | truth 11 |

No orphaned requirement: REQUIREMENTS.md maps PAGE-01 to PAGE-06 and PAGE-10 to Phase 18 and every one appears in a plan.

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `/setwarnlimit`, `/setwarnmode` | `applyWarnLimit`, `applyWarnMode` | `warns.go:673`, `:146` | WIRED | |
| Warnings page `l`, `m` | the same two | `settings_warns.go:228`, `:246` | WIRED | |
| `/reports` group branch, Switches `r` | `applyReportsEnabled` | `reports.go:87`, `settings_switches.go:211` | WIRED | |
| `/aispam`, Switches `a` | `applyAISpamEnabled` | `aispam.go:901`, `settings_switches.go:229` | WIRED | |
| `/autoapprove`, Switches `j` | `applyAutoApprove` | `greetings.go:826`, `settings_switches.go:245` | WIRED | |
| `change_language` callback (group branch) | `applyGroupLanguage` | `language.go` else branch | NOT_WIRED | calls `lang.ChangeGroupLanguage` directly (line 192) |
| Language page `l` | `applyGroupLanguage` | `settings_language.go:171` | WIRED | |
| `/log`, `/nolog`, Log channel page | `applyLogCategory` | `logchannels.go:240`, `settings_logchannel.go:174` | WIRED | |
| `/anonadmin`, Anonymous admins page | `applyAnonAdmin` | `admin.go:646`, `settings_anonadmin.go:107` | WIRED | |
| `/lock`, `/unlock`, Locks page | `applyLock` | `locks.go:250` via `applyLockList`, `settings_locks.go:207` | WIRED | |
| Six pages | Main grid | `registerSettingsPage` in each `init`, `settingsMainEntries` | WIRED | `settings_pages.go` unchanged |

### Data-Flow Trace (Level 4)

| Artifact | Data variable | Source | Real data | Status |
|----------|---------------|--------|-----------|--------|
| Warnings page | limit, mode | `warns.LoadWarnSettingsContext` (the key the commands read) | yes | FLOWING |
| Switches page | reports, AI, auto-approve | `LoadChatStatusContext`, `LoadEnabledContext`, `greetings.LoadSettingsContext` | yes | FLOWING |
| Language page | stored code | `lang.LoadGroupLanguageContext` (`chat_lang`) | yes | FLOWING |
| Log channel page | six flags, linked | `logchannels.LoadContext` | yes | FLOWING |
| Anonymous admins page | mode | `admin.LoadAnonAdminContext` | yes | FLOWING |
| Locks page | lock map | `locks.GetChatLocksCachedContext` | yes | FLOWING |

### Behavioral Spot-Checks and Commands Run

All commands ran in `/home/user/Fuku_Robot` at HEAD `2dff096` with the CI environment (`source .../scratchpad/cienv.sh`).

| Command | Result |
|---------|--------|
| `CGO_ENABLED=0 go build ./...` | exit 0 |
| `go vet -tags testtools ./fuku/modules` | exit 0 |
| `make check-translations` | "All translations are present" |
| `make check-docs` | "No drift detected" |
| `go test -tags testtools -race -count=1 -timeout 40m -run '^(TestSettingsPhase18\|TestSettingsMain\|TestLegacy\|TestSettingsLocks\|TestSettingsWarns\|TestSettingsSwitches\|TestSettingsLanguage\|TestSettingsLogChannel\|TestSettingsAnonAdmin\|TestApply)' ./fuku/modules ./fuku/i18n` | `ok fuku/modules 119.9 s`, `ok fuku/i18n [no tests to run]` (no `fuku/i18n` test name starts with those prefixes; `TestSettingsLabelLengths` and the limits table were not exercised by me, the 18-15 gate ran them) |
| `git diff --stat 29aca33..2dff096 -- migrations fuku/db/models main.go fuku/modules/settings_pages.go` | empty |
| key-set diff of the eight `locales/*.yml` against en (own script) | 0 missing, 0 extra in every file; 385 `settings_` and 31 `mreq_` keys each |

I did not run `make lint` (golangci-lint cannot run on Go 1.26.0 here, per 18-15), the whole `./fuku/modules` package (25 minutes, 18-15 reports `ok 1731.7 s`) or `make test`. I took those from `18-15-SUMMARY.md` as reported, not as verified by me; CI is green on the earlier boundary heads.

### Probe Execution

Step 7c: SKIPPED (the phase declares no probe scripts).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `fuku/modules/language.go` | ~192 | second write path (direct `lang.ChangeGroupLanguage` beside `applyGroupLanguage`) | Blocker for the structural truth, no behavioural effect | the callback bypasses the apply function's guards and its test seam; two paths can drift |
| (scan) | - | TODO, FIXME, XXX, HACK, TBD, "not implemented" in the six page files and `settings_toggle.go` | none found | clear |
| (scan) | - | `t.Skip` added since `29aca33` | none found | clear |
| (scan) | - | hard-coded English user text in the six page files | none found (a grep for capitalised multi-word literals outside log calls is empty; every literal key exists in en) | clear |
| (scan) | - | page readers that create rows, discarded errors on state-changing paths | none found in the apply functions (each returns the write error in its typed result) | clear |
| `fuku/modules/language.go` | ~165-200 | `query.Answer` results discarded in the callback | Info | pre-existing, not a state change |

Debt-marker gate: no `TBD`, `FIXME` or `XXX` in any file this phase modified.

### Known limits the plans accepted (not gaps)

- The Anonymous admins page opens, and shows the mode, while the bot is not an administrator; `/anonadmin`'s status view refuses then. Viewing writes nothing and the change itself is refused exactly as the command refuses it (documented in AGENTS.md and `permissions.md`, check 12).
- `settings_help_msg` and the generated `docs/.../commands/settings/index.md` do not name the Deletion page or the six new pages (18-SPIKE.md check 20, A18-A15). A text change in eight locale files under a one-message ceiling, left to the owner.
- The pre-existing `/reset warnings` stores an empty warn mode (`backup.go`), which the Warnings page shows as Not set with the inert line; the fix is one line plus a backup test (check 3).
- None of the cache keys the pages read (`warn_settings`, `ai_spam_settings`, `greetings`, `chat_lang`, `chat_settings`, `chat`, `log_channel`, `locks_map`) is in `skipLocal`: another replica can show an old value for up to `CACHE_LOCAL_TTL`, exactly as after the command. Reports and the `admin` row are uncached.
- Auto-approve's first press on a chat with no greetings row writes the whole greetings row, as `/autoapprove on` does; for a chat missing from the chats table the Goodbye page reads off before and on after (a state no served group is in).

### Human Verification Required

The owner has deferred the live Telegram checks by standing choice (Phases 7 to 17). `18-SPIKE.md` lists 25 numbered checks; **none blocks the phase**, and the code is safe under every outcome (a refused or failed press writes nothing, the AI line names no cause, the log channel is never named). Grouped:

1. **Warnings (checks 1 to 3).** Limit and each action (Mute, Ban, Kick) applied to a real member and warns cleared; a stored `tban` shows the inert line; schedule the `/reset warnings` empty-mode fix.
2. **Switches (checks 4 to 6).** The AI line in every configuration and a stale On press; Auto-approve's first press on PostgreSQL; reports on and off against a live `/report`.
3. **Language (checks 7 and 8).** Group menu redraw on two replicas within `CACHE_LOCAL_TTL`; a private menu keeps the opener's language and never writes `users.language`.
4. **Log channel and Anonymous admins (checks 9 to 12).** Real channel: which posts stop, the `#SETTINGS` line, no channel title or ID in a group menu; the creator switches anonymous-admin mode; a non-owner administrator and a demoted bot are refused; the page opening while the bot is not an administrator.
5. **Locks (checks 13 to 15).** The broad locks (`all`, `messages`, `url`, `bots`, `previews`) act at once; `comments`, `anonchannel`, `rtl`, `forward` need particular live messages; the right line when the bot cannot delete.
6. **Phones and wording (checks 16 to 19).** The 22 lock cells, eight language cells and six new Main cells on narrow phones in hi, ru, uk, id; the flag width of `Bahasa Indonesia`; Main at ten rows; a native-speaker read of the 55 new strings.
7. **Owner decisions and CI (checks 20 to 25).** The stale `/settings` help text; confirmations A18-1 to A18-9 (unconfirmed defaults), the flagged UI points, the four deviation notes, the accepted limits; CI lint and the full test run.

### Gaps Summary

One gap, one root cause. Plan 18-07 extracted `applyGroupLanguage` and said the `change_language` callback's group branch maps onto it. Commit `cd87587` added the function and substituted `groupLanguageCodes` into the callback's code check, but left the group branch calling `lang.ChangeGroupLanguage` itself. The Language page is therefore the function's only caller, and AGENTS.md, `18-07-SUMMARY.md` and the function's own doc comment describe a sharing that does not exist. Nothing a user can see differs: both paths call the same repository function, which drops `chat_lang`, `chat_settings` and `chat`, and `TestSettingsParityLanguage` proves the stored row, dropped keys and cached entry match through the real callback. The golden `TestLegacyLanguageRequests` and `TestApplyGroupLanguage` both pass under either wiring, so no test catches it. The cost is the cross-cutting rule "one setting, one code path": a later change to the apply function (a new guard, a new code) would not reach the command. Closing it is a few lines in `langBtnHandler` plus one test through the `languageWrites` seam; the existing golden pins every answer, so the change is safe to make. The remaining eight settings, the six pages, the rights, the readers, the goldens, the locale keys, the end-to-end proof and the docs checked out.

---

_Verified: 2026-11-13T04:20:00Z_
_Verifier: Claude (gsd-verifier)_
