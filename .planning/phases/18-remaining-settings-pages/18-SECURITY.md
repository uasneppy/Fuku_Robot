---
phase: "18"
slug: "remaining-settings-pages"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate, "high")
threats_open: 0
threats_total: 51
threats_closed: 51
findings_open_nonblocking: 0
findings_fixed: 2
findings_accepted: 4
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-13"
---

# Phase 18 - Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `2dff096` (diff `29aca33..2dff096`, all 15 plans; `7a18e10` that followed added only `18-VERIFICATION.md`). Every mitigation was read in the code, not taken from a plan or summary, and the security-relevant tests were run (see the audit trail). Depth is ASVS L1 (`security_block_on: high`) with L2-depth traces on authority, the anonymous-admin toggle, cross-chat replay, the readers and the audit post. As for Phases 15 to 17 the mandate was wider than "verify the register": it also hunted for what a group member, a forged or replayed callback, a failing dependency or a rolling deploy can actually cause. This audit's own findings are `SEC-18-nn`. Paths are under `fuku/modules/` unless a path is given.
>
> **Register:** 51 threats: the 50 numbered ones (T-18-01 to T-18-50: 48 `mitigate`, 2 `accept`) plus T-18-SC, the supply-chain row repeated in all 15 plans.
>
> The auditor role is read-only: this file was persisted by the orchestrator from the auditor's report, with the evidence column shortened and the three items the orchestrator fixed afterwards marked (T-18-49, SEC-18-01, SEC-18-02, all in `99f796e`).
>
> Evidence limits: no live Telegram run (`18-SPIKE.md` holds those checks). `make lint` and `make test-postgres-integrity` were not run by the auditor (the sandbox golangci-lint is built with Go 1.25, and a PostgreSQL cluster cannot live in its scratchpad); the closing plan 18-15 ran the full gate with a throwaway PostgreSQL (`18-15-SUMMARY.md`). The phase adds no PostgreSQL guarantee (no migration, statement, index or CHECK).

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | `settings\|v1` callback to the press pipeline (`settingsPress.verify`) | fields `a p t v c u` are attacker-controlled. Checks in order: strict decode, service identity, opener, live `getChatMember`, owner gate, right |
| 2 | composite rights to the bot predicates and the press's own member record | the bot part (`IsUserAdmin(bot)` or `IsBotAdmin`, from the cached admin list) is judged first; the member part comes from the live record of step 4 |
| 3 | page reads to the DB and cache through six new non-creating readers plus two existing ones | a read error must become the cannot-open edit or the unknown mark, never a default; opening a page must create no row |
| 4 | command and apply function | one write path per setting; goldens recorded before each extraction |
| 5 | group-menu text to every member of the group | must never name the log channel's title or ID, the Staff Group, a member or a count |
| 6 | bot configuration (TypeSafe value, `ENABLE_AISPAM`) to the AI line of the Switches page | must name no cause, key, variable or provider |
| 7 | Log channel page to the log channel | the `#SETTINGS` audit post, sent only after a stored change |
| 8 | Locks page right line to Telegram `getChatMember(bot)` | goes through the fleet-wide deletion pacer, 3 s bound |
| 9 | anonymous-admin mode to every anonymous sender of the group | lowers the proof check; only the live creator may switch it, and only with the bot an administrator |
| 10 | the `change_language` callback's private branch (the user's own language) | must not be reachable from the page |

---

## Threat Register

| Threat ID | Category | Component | Sev | Disposition | Status | Key evidence (read in code; test that holds it) |
|-----------|----------|-----------|-----|-------------|--------|-----------------------------|
| T-18-01 | Tampering | forged limit press | med | mitigate | CLOSED | `settings_warns.go` control `l`: `Actions{st}` and `Validate` = `settingsParseNumber(v,3)`, enforced in step 1 by `settings_menu.go:340` `settingsControlFieldsValid`; `warns.go:95` `applyWarnLimit` refuses values outside 1..100 with no write. `TestSettingsWarnsForged`, `TestSettingsControlActionAllowList` |
| T-18-02 | EoP | an admin the command refuses writes the limit | med | mitigate | CLOSED | `settingsRightAdminFloodBot`: live member check plus `IsUserAdmin(bot)`. `TestSettingsAuthorityParityWarns`. See SEC-18-03 |
| T-18-03 | Tampering | the extraction changes `/setwarnlimit` and `/setwarnmode` | med | mitigate | CLOSED | golden `871d39c` precedes refactor `07ea430` and was never edited afterwards; `TestLegacyWarnSettingsRequests` green |
| T-18-04 | InfoDisc/Tamp | a reader creates a row or caches a default | low | mitigate | CLOSED | `fuku/db/warns/repository.go:316`: `First` with no insert; `ErrRecordNotFound` returns {3, mute} uncached. Same key and type as `GetWarnSettingContext`, so the shared loader cannot hit a type-assertion panic. `TestLoadWarnSettingsContext` |
| T-18-05 | InfoDisc | a string names configuration, the log channel, a member or the Staff Group | med | mitigate | CLOSED | all 55 new keys read in 8 locales: none names a provider, key, variable, ID or Staff Group. `TestSettingsReusedKeyPlaceholders`, `TestSettingsStaffPrivacy` |
| T-18-06 | DoS | an overlong label or a missing key | low | mitigate | CLOSED | `make check-translations`; `TestSettingsLabelLengths` and the limits table (`fuku/i18n`) |
| T-18-07 | Tampering | a forged warn mode (`tban`, `tmute`, empty) | med | mitigate | CLOSED | `Validate` = `settingsFloodModeWord`; `warns.go:108` allow-list is exactly ban, kick, mute. `TestSettingsWarnsForged` |
| T-18-08 | EoP | a presser the warn commands refuse | med | mitigate | CLOSED | the right is judged after step 4 (live record) and step 4b (owner gate). `TestSettingsAuthorityParityWarns` |
| T-18-09 | Repud/Tamp | the page shows a value the warn module does not use | low | mitigate | CLOSED | page and `applyWarn` (`warns.go:198`, `GetWarnSettingContext`) share the `warn_settings` key. `TestSettingsDriftWarns` |
| T-18-10 | DoS | an inert stored mode silently disables the limit | low | mitigate | CLOSED | `renderSettingsWarns` adds `settings_warns_action_inert` and marks no choice; the `/reset warnings` defect is recorded in SPIKE check 3 |
| T-18-11 | EoP | a non-owner changes anonymous-admin mode | high | mitigate | CLOSED | `settings_menu.go:110` `settingsRightOwnerBotAdmin` requires `Status==creator` on the live `p.member`. `TestSettingsRights` "mirrors /anonadmin" |
| T-18-12 | EoP | the menu writes auto-approve where `/autoapprove` refuses | med | mitigate | CLOSED | `settings_menu.go:160` checks the bot part first (`IsUserAdmin(bot)`, as `IsUserConnected(true,true)` does), then `can_change_info` on the live record. `TestSettingsRights` "mirrors /autoapprove" |
| T-18-13 | Tampering | the extraction changes `/reports` or `/autoapprove` | med | mitigate | CLOSED | goldens `fbf6241` (private branch included) and `0ab2728` precede their refactors; `fe84b3e` edited only a fixture's error checks, no expectation |
| T-18-14 | Tampering | the reports reader creates a row | low | mitigate | CLOSED | `fuku/db/reports/repository.go:42`: plain `Select("status").Take`. `TestLoadChatStatusContext` |
| T-18-15 | Tampering | the page enables AI where `/aispam on` refuses | med | mitigate | CLOSED | `aispam.go:833` `aispamRefusalKey` keeps the order of the removed `aispamEnableRefusal`; `applyAISpamEnabled` refuses On before any write. `TestAISpamConfiguredMatchesCommand` |
| T-18-16 | InfoDisc | the provider value logged or named | med | mitigate | CLOSED | no new log line or identifier carries `TypeSafeAPIKey`; the test constant is `aispamTestProviderValue` |
| T-18-17 | Tampering | the extraction changes the status text or drops the error | low | mitigate | CLOSED | golden `45d7082` first; the command still returns `result.Err` after replying |
| T-18-18 | Tampering | a forged or stale On press enables AI | med | mitigate | CLOSED | `settings_switches.go:223`: the `on && !aispamConfigured()` alert fires before the apply call; Off-only form (`renderSettingsSwitches`). `TestSettingsSwitchesAIGate` |
| T-18-19 | EoP | auto-approve written without change-info or bot admin | med | mitigate | CLOSED | control `j` = `settingsRightChangeInfoBotAdmin`. `TestSettingsAuthorityParitySwitches`, `TestSettingsParityAutoApprove` |
| T-18-20 | InfoDisc | the AI line names its cause | med | mitigate | CLOSED | `settingsSwAIBlock` shows a code-side 🚫 plus `settings_sw_ai_unavailable` (no cause in any of the 8 locales). `TestSettingsSwitchesAIStatus` |
| T-18-21 | Tampering | `st` on a toggle, or a non-binary `v` | low | mitigate | CLOSED | `Actions{tg}` and `settingsEditSwitchValue` (exactly `"0"` or `"1"`). `TestSettingsSwitchesForged` |
| T-18-22 | Tampering | the page writes the presser's own language | med | mitigate | CLOSED | `settings_language.go:171` calls only `applyGroupLanguage` on `p.group.Id` (a verified group); no `ChangeUserLanguage` in any `settings_*.go`. `TestSettingsLanguagePrivate` |
| T-18-23 | Tampering | a forged language code | low | mitigate | CLOSED | `settingsLangCodeShape` (two lower-case letters) in step 1, then the 8-code list in `language.go:61`. `TestSettingsLanguageForged`, `TestApplyGroupLanguage` |
| T-18-24 | EoP | a non-admin changes the language from a replayed button | low | mitigate | CLOSED | step 4 live record and step 4b owner gate; the group is the message chat or a verified `c`. `TestSettingsAuthorityParityLanguage` |
| T-18-25 | Tampering | the extraction changes the callback | low | mitigate | CLOSED | golden `8d8aa49` first. The callback was in fact not refactored at 2dff096 (SEC-18-01, fixed in `99f796e`). `85ca132` pins `ValidLangCodes` for the two `/lang` display cases only; no security effect |
| T-18-26 | Repudiation | a category change with no audit line | low | mitigate | CLOSED | commands keep their post (`TestLegacyLogCategoryRequests`); the page posts the same entry. The residual gap (SEC-18-02) is fixed in `99f796e` |
| T-18-27 | Tampering | the extraction changes the multi-category behaviour | med | mitigate | CLOSED | golden `d6471a0` first; the loop and its quirk are preserved in `setLogCategories` |
| T-18-28 | InfoDisc | a failed read shown as "not linked" | low | mitigate | CLOSED | `fuku/db/logchannels/repository.go:64` `LoadContext` returns the error; a failed page read is the cannot-open edit. `TestLoadLogChannelContext`. See SEC-18-04 |
| T-18-29 | InfoDisc | the log channel's title or ID in a group menu | high | mitigate | CLOSED | `settings_logchannel.go:139` renders from `settingsLogState` (a linked flag and six booleans) and never receives the row; the notices are `logs_not_set` and `common_settings_save_failed` only. `TestSettingsLogChannelPrivacy` |
| T-18-30 | Repudiation | a menu change with no audit | low | mitigate | CLOSED | `settings_logchannel.go` posts `actionlog.Settings` only when `auditKey` was set by `logApplied`, and since `99f796e` whenever it was set. `TestSettingsParityLogCategory`, `TestSettingsLogChannelAuditSurvivesFailedReadBack` |
| T-18-31 | Tampering | a forged category | low | mitigate | CLOSED | controls exist only for the six table codes, with `Actions{tg}` and `Validate`. `TestSettingsLogChannelForged` |
| T-18-32 | DoS (log gap) | the admin category off silences staff and lockdown copies | low | accept | ACCEPTED | AR-1 (parity with `/nolog admin`; SPIKE check 9) |
| T-18-33 | EoP | a non-owner turns anonymous-admin mode on | high | mitigate | CLOSED | `settings_anonadmin.go:41-46` toggle with the right `settingsRightOwnerBotAdmin`, judged after step 4 and step 4b. `TestSettingsAnonAdminRights`, `TestSettingsAuthorityParityAnonAdmin` |
| T-18-34 | Spoofing | the toggle replayed from another chat | low | mitigate | CLOSED | `verify`: a group menu uses `p.group` = the message chat and requires `u == presser`; a private menu requires a re-verified `c` (live member, gate, `getChat` type group or supergroup) |
| T-18-35 | Tampering | the extraction changes `/anonadmin` | med | mitigate | CLOSED | golden `20e87c9` first (the created row pinned); `RequireUserOwner` and `GetAdminSettings` unchanged in the command |
| T-18-36 | Tampering | a forged `v` or `st` on the toggle | low | mitigate | CLOSED | `Actions{tg}` and `settingsEditSwitchValue`. `TestSettingsAnonAdminForged` (7 shapes) |
| T-18-37 | Tampering | multi-name `/lock` behaviour changed | med | mitigate | CLOSED | golden `1593804` first; the all-or-nothing pre-validation stays in `lockPerm` before `applyLockList` |
| T-18-38 | Tampering | a forged lock kind from the page | low | mitigate | CLOSED | `locks.go:220` refuses kinds outside `getLockMapAsArray()`; controls exist only for `settingsLockRows` codes. `TestApplyLock` |
| T-18-39 | Tampering | the table drifts from `lockMap`/`restrMap` | low | mitigate | CLOSED | `TestSettingsLocksTableMatchesCommand` (set equality both ways) |
| T-18-40 | EoP | a presser `/lock` refuses | med | mitigate | CLOSED | `settingsRightAdminFloodBot` on every lock control. `TestSettingsAuthorityParityLocks`. See SEC-18-03 |
| T-18-41 | Tampering | a forged code, value or page | low | mitigate | CLOSED | `settings_locks.go:70` `kg` takes exactly `v` (1 or 2 digits, extra keys refused), clamped in `settingsLocksRunPage`. `TestSettingsLocksForged` |
| T-18-42 | DoS (members) | a stray tap on `all` or `messages` | med | accept | ACCEPTED | AR-2 (parity with `/lock all`, one tap undoes it, A18-8 in SPIKE check 21) |
| T-18-43 | DoS (budget) | the right-line read on every draw | low | mitigate | CLOSED | `settingsDelReadRight`: one bot read through the deletion pacer under 3 s; forged and alert-refused presses read nothing. A notice refusal costs one read (SPIKE check 24). `TestSettingsLocksCost` |
| T-18-44 | InfoDisc | a Phase 18 screen names the Staff Group, the log channel, a member or a count | high | mitigate | CLOSED | no Phase 18 page reads staff tables or member names. `TestSettingsStaffPrivacy` with `settingsPrivacyPhase18Run` (8 languages, both menus) |
| T-18-45 | Tampering | a code collision | med | mitigate | CLOSED | `settings_pages.go:429` panics on a duplicate page key, a duplicate action code or a core code. `TestSettingsPhase18Codes` |
| T-18-46 | DoS/Tamp | opening Main or a page creates rows or spends calls | low | mitigate | CLOSED | no Phase 18 `MainState` calls Telegram. `TestSettingsPhase18ReadsCreateNothing`, `TestSettingsPhase18Cost`, `TestSettingsMainPhase18` |
| T-18-47 | Tamp/Repud | the page stores the right row but the bot acts on another value | med | mitigate | CLOSED | `TestSettingsPhase18EndToEnd` (real dispatcher, every page) |
| T-18-48 | EoP | anonymous-admin mode turned on by someone the command refuses | high | mitigate | CLOSED | e2e subtest "anonymous admins": a non-owner's press leaves the mode off and gets the owner-only alert; the creator's press removes the proof prompt |
| T-18-49 | Repudiation | docs claim behaviour the code lacks | med | mitigate | CLOSED (was OPEN at `2dff096`, fixed in `99f796e`) | AGENTS.md, the doc comment of `applyGroupLanguage` and `18-07-SUMMARY` said the callback writes through `applyGroupLanguage`; at `2dff096` `language.go:192` called `lang.ChangeGroupLanguage` directly (SEC-18-01). Now the callback calls the function and `TestLanguageCallbackWritesThroughApplyGroupLanguage` holds it |
| T-18-50 | Repudiation | a requirement ticked before the proof | med | mitigate | CLOSED | `58aa643` ticks exactly PAGE-01 to PAGE-06 and PAGE-10, after the e2e commits `f1d22d4`, `78bdc1c` and `c901995` |
| T-18-SC | Tampering | `go.mod` and `go.sum` | high | mitigate | CLOSED | `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; `go.mod`, `go.sum`, `migrations`, `main.go`, `fuku/db/models`, `fuku/db/cache` and `fuku/utils` are unchanged since `29aca33` |

### Open (blocking: severity at or above high)

None.

### Open (non-blocking)

None. T-18-49 was open at `2dff096` and is closed by `99f796e` (see below).

*Only blocking-open threats count toward `threats_open` in the frontmatter.*

### Unregistered flags

None. Every SUMMARY's `## Threat Flags` maps to registered IDs or says "None".

---

## New Threats Found by the Audit (unregistered, none blocking)

- **SEC-18-01 (low; Tampering/Repudiation, the "one setting, one code path" integrity). Status: FIXED in `99f796e`. The group branch of the `change_language` callback bypassed `applyGroupLanguage`.**
  - *Evidence at `2dff096`:* `language.go:192` (`langBtnHandler`) called `lang.ChangeGroupLanguage(chat.Id, language)` itself; `applyGroupLanguage` had one production caller, the page. Commit `cd87587` only added the function and swapped `groupLanguageCodes` into the callback's code check. The independent verification (`18-VERIFICATION.md`) reached the same finding.
  - *Behaviour then:* identical. Both paths validated the same 8 codes and reached the same repository function. `TestSettingsParityLanguage` proved it; neither `TestLegacyLanguageRequests` nor `TestApplyGroupLanguage` could detect the bypass.
  - *Failure scenario:* a later guard added to `applyGroupLanguage` (refusing locales disabled by `ENABLED_LOCALES`, or the `ctx.Err()` refusal it already has) would reach the page and not `/lang`'s buttons; page and command would refuse different things, breaking PAGE-10, while the docs kept claiming parity.
  - *Fix:* the else branch now calls `applyGroupLanguage(context.Background(), chat.Id, language)`, maps `langSaveFailed` to the unchanged `common_settings_save_failed` answer and builds the reply from `result.Key` and `result.Params`, keeping the order (code check, admin check, apply). `TestLegacyLanguageRequests` passes unchanged; `TestLanguageCallbackWritesThroughApplyGroupLanguage` swaps the `languageWrites` seam for a recorder and presses the real callback (it failed on the old code: "the press wrote []").

- **SEC-18-02 (low; Repudiation). Status: FIXED in `99f796e`. The Log channel page could store a category change without its `#SETTINGS` audit line.**
  - *Evidence at `2dff096`:* `settings_logchannel.go` returned before `actionlog.Settings` when `screen == nil`: the apply function's read-back and the helper's second read both failed, so the page drew the cannot-open edit and the control returned `nil`. The commands always post after their loop.
  - *Failure scenario:* during a short database or cache fault right after the write, an administrator turns a category off and no audit entry records it.
  - *Fix:* the control posts whenever the apply function reported a stored change (`auditKey != ""`); only the answer and the redraw are skipped when the page could not be drawn. `TestSettingsLogChannelAuditSurvivesFailedReadBack` fails exactly two reads after the write (a GORM query callback) and requires the one `#SETTINGS` post, the stored flag and one answer; it failed on the old code ("posted []").

- **SEC-18-03 (info; EoP parity). Status: ACCEPTED. `settingsRightAdminFloodBot` checks one bot predicate where four commands check two.**
  - *Evidence:* `/setwarnlimit`, `/setwarnmode`, `/reports` and `/lock`/`/unlock` call `IsUserConnected(true,true)` (which uses `IsUserAdmin(bot)`) and also `RequireBotAdmin` (`IsBotAdmin`). The right, built for `/setflood` (which has only the first), checks only `IsUserAdmin(bot)`.
  - *Impact:* both predicates read the same admin cache and fall back to live lookups, so they agree except in a transient cache divergence. The member part is live in both cases. Worst case: a setting is stored while the bot cannot enforce it. A plain member or a demoted administrator can never write (step 4 refuses them).
  - *Disposition:* accepted; a later change may add `IsBotAdmin` to these controls.

- **SEC-18-04 (info; Information disclosure, related to T-18-28). Status: ACCEPTED (parity). A press can be refused with "No log channel is set" when the real cause is a failed read.**
  - *Evidence:* `logchannels.SetCategory` calls `Get(chatID)`, which hides read errors as nil, and returns `ErrRecordNotFound`; `applyLogCategory` maps that to `logNotSet`.
  - *Impact:* it fails closed: nothing is written and the drawn state is correct; only the notice is wrong. `/log` behaves the same way, because its `GetContext` pre-check also hides the error.

- **SEC-18-05 (info; Repudiation). Status: ACCEPTED (parity, extends AR-1). Turning the Settings category off silences its own audit.**
  - *Evidence:* turning the Settings category off posts no `#SETTINGS` line for that very change, because `Destination` is read after the write.
  - *Failure scenario:* any administrator the owner gate admits can switch off Settings and then Admin actions, leaving no trace in the log channel. Identical to `/nolog settings`; SPIKE check 9 records it.

- **SEC-18-06 (info; Tampering). Status: ACCEPTED (parity with the callback, D-21). The Language page accepts locales that `ENABLED_LOCALES` disabled.**
  - *Evidence:* a forged press can store any of the 8 hard-coded codes, including one hidden from the keyboard.
  - *Impact:* harmless. The i18n manager falls back silently, and the existing callback accepts the same 8 codes.

---

## Accepted Risks Log

| # | Risk | Why accepted |
|---|------|--------------|
| AR-1 | T-18-32 and SEC-18-05: an administrator can turn the admin or settings log category off from the page, silencing staff and lockdown copies and the `#SETTINGS` line for that change | parity with `/nolog`; the page hint says what it controls; SPIKE checks 9 and 24 |
| AR-2 | T-18-42: one tap on `all` or `messages` deletes every non-exempt member's messages; there is no card | parity with `/lock all`; one tap undoes it; administrators and approved members are exempt; A18-8 is an open owner decision (SPIKE check 21) |
| AR-3 | the Anonymous admins page opens, and shows the mode, while the bot is not an administrator (looser than `/anonadmin`'s status view) | viewing writes nothing; the change keeps the bot-first refusal; SPIKE check 12 |
| AR-4 | no card for turning Anonymous admins On | the creator only; Off is one tap; A18-8 and A18-A12 open for the owner |
| AR-5 | a Locks redraw after a notice refusal makes one more bot read; the right line takes slots from the fleet-wide deletion pacer | bounded at 3 s per draw; same design as the Phase 16 Deletion page; SPIKE check 24 |
| AR-6 | another replica can show an old value for up to `CACHE_LOCAL_TTL` (`warn_settings`, `ai_spam_settings`, `chat_lang`, `log_channel`, `locks_map`) | exactly as after the commands; `reports` and `admin` are uncached |
| AR-7 | SEC-18-03, SEC-18-04, SEC-18-06 | as described above |
| carried | Phase 16 AR-1 to AR-7 and Phase 17 AR-1 to AR-9 stand | unchanged |

Decisions D-01 to D-24 and owner confirmations A18-1 to A18-9 are provisional (unconfirmed defaults). Changing any of them can reopen the matching threat.

---

## Security Audit Trail

| Date | Event | Result |
|------|-------|--------|
| 2026-11-13 | `gsd-security-auditor` at `2dff096` (HEAD `7a18e10`, docs only), ASVS L1 with L2 traces | 50 of 51 threats closed (47 mitigated, 2 accepted, and T-18-SC); T-18-49 open (medium, non-blocking); SEC-18-01 to SEC-18-06 (2 low open, 4 info accepted); `threats_open: 0` |
| 2026-11-13 | evidence run by the auditor | `CGO_ENABLED=0 go build ./...` ok. `go vet -tags testtools ./fuku/modules` clean. 148 top-level tests of `fuku/modules` (`TestSettingsAuthority*`, `TestSettingsRights`, `TestSettingsRightRefusalForParity`, `TestSettingsPhase18*`, `TestSettingsLogChannel*`, `TestSettingsAnonAdmin*`, `TestSettingsLocks*`, `TestSettingsStaffPrivacy`, `TestSettingsGateAllDoors`, `TestLegacy*`, `TestSettingsParity*`, `TestSettingsDrift*`, `TestSettingsWarns*`, `TestSettingsSwitches*`, `TestSettingsLanguage*`, `TestSettingsControlActionAllowList`, `TestSettingsMain*`, `TestApply*`, `TestAISpamConfiguredMatchesCommand`) with `-race -count=1` under the CI environment: ok (248 s). `fuku/db/{admin,aispam,lang,logchannels,reports,warns}` ok. `fuku/i18n` `^TestSettings` ok. `make check-translations` and `make check-docs` pass. Golden commit order verified for all 8 goldens |
| 2026-11-13 | orchestrator, `99f796e` | SEC-18-01 and SEC-18-02 fixed with a test each, run red on the unfixed code and green on the fixed code, in the CI environment and bare; the language and log goldens pass unchanged. The fixes were NOT re-audited by the auditor: the language change is a rewire of one branch onto an existing function whose behaviour a golden pins, and the audit change moves one early return. T-18-49 closed with them |

| Threats found | Closed | Open (blocking) | Open (non-blocking) |
|---------------|--------|-----------------|---------------------|
| 51 | 51 | 0 | 0 |

### Notes for the orchestrator (from the auditor)

- **Authority:** no gap found. Every new control's right matches its command, judged on the live record after step 4 and step 4b. The composite rights check the bot first.
- **Anonymous-admin toggle:** it cannot be turned on by a non-owner, by a forged `v` (exactly 0 or 1, and On still needs the creator), by a replayed or forwarded button (authority is re-checked live in the chat the write targets), by an anonymous non-creator admin (callbacks carry the real user), or by anyone while the bot is not an administrator (`IsBotAdmin` is judged first).
- **Unchanged framework:** `settings_pages.go`, `settings_gate.go` and every `settings_input*` file are untouched; the typed-reply door does not use the new rights.
- **Readers:** none of the six creates a row. Each returns its error. Each shares its command's key with the same value type, so the shared cache loader cannot hit a type-assertion panic.
- **Callbacks and logs:** no new goroutine; every button goes through `callbackcodec` (`settingsKeyboard.button`); new log lines carry IDs and error text only.
- **`bc424c4` (Makefile `-timeout 40m`):** no security relevance. The job ceiling (45 min) and the skip gate stand.
- **`85ca132` (golden language pin):** no security relevance. It pins process state for the two `/lang` display cases and leaves the callback expectations as recorded.
