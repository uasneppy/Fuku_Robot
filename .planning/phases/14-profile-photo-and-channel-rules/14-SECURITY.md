---
phase: "14"
slug: "profile-photo-and-channel-rules"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate, "high")
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-08"
---

# Phase 14 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `a770096` on branch `claude/ecstatic-goldberg-qsss05` (all 12 plans
> executed, plus the code review fix pass `b3413c2..5b87552` and its docs commits `cc5e7ce`, `c599e8c`, `a770096`). Every
> mitigation was read in the code at HEAD, not taken from a summary, and the security-relevant tests were run (62 top-level
> tests of `./fuku/modules` under `-race`, plus the database, locale, cache and backup guards). Evidence is `file:line` at
> `a770096`. Short names: `lk` = `fuku/modules/memberreq_lookup.go`, `rules` = `memberreq_rules.go`, `enf` =
> `memberreq_enforce.go`, `watch` = `memberreq_watch.go`, `held` = `memberreq_held.go`, `fixed` = `memberreq_fixed.go`,
> `notice` = `memberreq_notice.go` (all under `fuku/modules/`), `smreq` = `fuku/modules/settings_mreq.go`, `srepo` =
> `fuku/db/memberreq/settings.go`, `pacer` = `fuku/utils/ratelimit/telegram_pacer.go`, `mig` =
> `migrations/20261108120000_add_member_requirement_lookup_rules.sql`. Review items are written `RV-` (the review's own
> `NEW-01` to `NEW-03` would clash with this audit's findings, so this audit's findings are `SEC-`).

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | Any group member → message watcher (group -2, every supergroup message) | the sender's identity; names (attacker-chosen), nothing about a photo or a channel |
| 2 | Bot → Telegram lookups (`getUserProfilePhotos`, `getChat` of the group, `getChatMember` of a member and of the bot in a channel) | a user ID or a chat ID out; a verdict in, which can be wrong, late, missing or refused |
| 3 | Group owner → the linked channel (the owner chooses which channel the group is linked to) | the channel ID, taken only from `getChat(group).linked_chat_id`, never from an update or callback data |
| 4 | Channel owner → the bot's standing in the channel (rights can be removed at any time) | `getChatMember(channel, bot)` status |
| 5 | Replicas → Redis (`fuku:memberreq:photo|lc|sub|ba|cna:*`, the lookup pacer keys) | positives, the channel link, the bot's standing, pacing slots |
| 6 | Telegram user → `settings\|v1` callbacks for `ph` and `ch` (rule controls, `mo`, `my`, the channel screen) | page, control, action, value `v`, expiry `e`, group `c` (not authenticated by Telegram) |
| 7 | Held member → `mreq\|v1` "I fixed it" (now with fresh lookups) | hold ID `h`, member `u` |
| 8 | Requirements → every other restriction path (admin, staff, captcha, antiflood, lockdown, warn limit) | one member record, one `until_date` marker |
| 9 | Deploy → PostgreSQL (`AUTO_MIGRATE=true`) | migration 20261108120000, two `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` |
| 10 | Backup file → database (owner-only import) | modules a file may carry (neither requirement table is one) |
| 11 | Group menu and a non-creator's private menu → every reader | page text, buttons, alerts: no Staff Group, no channel title or ID |
| 12 | Process → shutdown | lookups and flights in flight (no new drain) |
| 13 | Process logs | IDs, codes and errors only; the bot token |

---

## Threat Register

49 distinct IDs: T-14-01 to T-14-48 and T-14-SC (60 rows in the 12 plans, because every plan registers T-14-SC). Every
disposition in the plans is `mitigate`; there is no `accept` and no `transfer` row, and no severity below `low`. Each ID is
registered once below, with the plan that introduced it (T-14-SC with `01-12`).

| Threat ID | Plan | Cat | Sev | Disposition | Mitigation (evidence, and the test that proves it) | Status |
|---|---|---|---|---|---|---|
| T-14-01 | 01 | T | low | mitigate | `srepo:150-157` `SetRuleAction` refuses an unknown rule (`ruleColumns[rule]`, `:151-153`) and a value outside 0..4 (`:155-157`) before any write; `mig:23-24` two named CHECKs inline (`chk_chat_requirements_photo`, `chk_chat_requirements_channel`); the model carries the same checks (`fuku/db/models/member_requirements.go:36-37`). `TestChatRequirementsSettings` ("refused before any write", "the CHECKs") passes. The column name written is always a value of the constant map, never user text | CLOSED |
| T-14-02 | 01 | T | med | mitigate | `mig:23-24` is `ADD COLUMN IF NOT EXISTS ... CONSTRAINT ... CHECK` with the CHECK inline and no top-level `ADD CONSTRAINT`, so neither applier rewrites it (`fuku/db/migrations/runner.go:536` and `scripts/migrate_psql.sh:75` both match `ADD CONSTRAINT` only); `git diff cc19bba HEAD -- fuku/db/migrations/runner.go scripts/migrate_psql.sh` is empty; the file is the newest in `migrations/`. `TestMemberRequirementLookupMigrationStatements` passes (database-free). `TestMemberRequirementLookupMigrationDefaults` is PostgreSQL-only (skips locally, is in the `make test-postgres-integrity` list per `make -n`; plan 14-12 ran it, not repeated here) | CLOSED |
| T-14-03 | 01 | E | high | mitigate | `mig:23-24` `NOT NULL DEFAULT 0`; the model fields carry no `default:` tag (`models/member_requirements.go:36-37`); `srepo` `ActionOf` reads an unknown rule as Off; msgpack decodes a field the previous build never wrote as 0. `TestChatRequirementsSettings` ("an old row": a cached value written by a struct without the two fields) passes; the PostgreSQL defaults test is the migration half | CLOSED |
| T-14-04 | 02 | T | med | mitigate | `TestMemberReqLocaleKeys` and `TestSettingsLocaleKeys` (run, pass) fail any `mreq_`/`settings_mreq_` template with `<`, `>` or `&` and any placeholder set unlike English; `TestMemberReqNoProfileInReworkedKeys` passes in all 8 languages | CLOSED |
| T-14-05 | 02 | D | med | mitigate | rendered ceilings are binding in 8 languages: notices 520 (`fuku/i18n/memberreq_locale_test.go:92`), still-broken 160 and 200, alerts at most 200 (`TestMemberReqLookupAlertsFitEveryLocale`, `memberreq_lookup_budget_test.go:552`), cells at most 20 (`TestSettingsLabelLengths`). `TestMemberReqScreensFitEveryLocale`, `TestSettingsMreqPageFitsEveryLocale` pass | CLOSED |
| T-14-06 | 02 | I | high | mitigate | the new strings carry no placeholder for a title, ID, member or count (only `{rule}`, `{n}`, `{name}`, `{rules}`, `{state}`, `{limit}`); the settings lines are fixed words with `{rule}` the translated rule name (`smreq:259-280`); `TestMemberReqLookupAlertsFitEveryLocale` fails any chat or channel digits, "staff" or an unfilled token; `TestSettingsStaffPrivacy` passes | CLOSED |
| T-14-07 | 02 | R | med | mitigate | `notice:130-155` appends `mreq_remedy_photo` only to the plain warn, kick and mute notices and only while the photo bit (found through its table row) is broken, so a channel-only breaker is never told to fix a photo; the key exists once in each of the 8 locale files with the D-01 wording ("show it to everyone, or ask an admin to approve you"). `TestMemberReqNoProfileInReworkedKeys` passes | CLOSED |
| T-14-08 | 03 | T | high | mitigate | `lk:34` the verdict's zero value is unknown; `lk:304-307` every error is unknown; `lk:309-312` a null body (nil pointer) is unknown; `lk:313-320` a breach needs a non-nil answer with total 0 and no photos; `rules:200-206` a lookup adds a bit only on `memberReqBreach`; `lk:243-245,260-262` a caller whose context ended gets no value. `TestMemberReqVerdictZeroValue`, `TestMemberReqPhotoRule`, `TestMemberReqLookupUnanswerable` (28 shapes through the job and the join entry) pass | CLOSED |
| T-14-09 | 03 | D | high | mitigate | `lk:248` `defer error_handling.RecoverFromPanic` is the first deferred statement inside the `DoChan` function, so a leader panic is recovered before the library can re-raise it in a goroutine nothing can recover; `RecoverFromPanic` calls `recover()` itself (`fuku/utils/error_handling/error_handling.go:17`); the function then returns `(nil, nil)` and `lk:255-258` reads that as no value, i.e. unknown. `TestMemberReqLookupSingleflight` (panicking function) and the "a panic in a flight is recovered with one line that names no ID" subtest of `TestMemberReqLookupLogsCarryIDsOnly` pass | CLOSED |
| T-14-10 | 03 | D | high | mitigate | `lk:87-96` the lookup pacer has its own keys `fuku:memberreq:lkp:pace:next|block` and `MaxWait` 5 s; the requirement pacer's keys are `enf:33-34`; `pacer:172-174` and `:226-229` a slot more than `MaxWait` away is shed as `ErrRateLimited` with no Telegram request and no slot taken (also on the Redis-down local fallback). `TestMemberReqLookupPacer`, `TestMemberReqPacersAreIndependent` pass. Residual: the Telegram-side flood budget is not shared between pacers (AR-07) | CLOSED |
| T-14-11 | 03 | T | med | mitigate | `lk:313-316` only a pass is cached (30 min, `lk:59`); a breach and every unknown answer are never cached; `lk:288,295` a Fresh lookup bypasses the cache and `lk:317-319` drops a stale pass on a definite breach. `TestMemberReqPhotoRule` passes. Residual: a cached pass can outlive a removed photo by 30 min at the watcher (AR-05) | CLOSED |
| T-14-12 | 03 | I | med | mitigate | `lk:269-275` a lookup failure is logged by IDs and the code (`memberReqErrText`, `enf:144-150`) or the fixed text "rate limited", never Telegram's description; the photo answer is never stored or logged (`lk:309-316`). `TestMemberReqLookupLogsCarryIDsOnly` (run, passes) forbids a file ID, a channel title, first and last names, a username, a description, the bot credential and every notice or rule text in any captured line, over the job, the flights, "I fixed it", the held release, the page and the gate. The channel's numeric ID is logged at Debug by design (SEC-01, AR-10) | CLOSED |
| T-14-13 | 04 | T | high | mitigate | `lk:430-443` left, kicked and restricted-without-`is_member` are only a breach candidate; `lk:608-625` and `lk:638-646` the candidate becomes a breach only while `memberReqBotChannelStatus` proves the bot an administrator (a definite non-administrator is inert, anything unestablished is unknown); `lk:467-482` the bot's standing is classified by status and by Telegram code. `TestMemberReqChannelBreachNeedsBotAdmin` (every member answered `left`), `TestMemberReqChannelRule`, `TestMemberReqBotStatusClassifier` pass. Residual: a cached administrator standing (`lk:494-496`) is believed for up to 2 minutes after a demotion (SEC-03, AR-03) | CLOSED |
| T-14-14 | 04 | S | high | mitigate | the only non-test reader of `LinkedChatId` is `lk:408-416` inside `memberReqLinkedChannel` (`lk:380`), which asks `getChat(chatID)`; `chatID` is the update's chat (`enf` `c.Chat.Id`), the pressed message's chat (`fixed:126`) or the group the press pipeline verified (`smreq:216,608`); no callback field is read as a channel (grep of `settings_mreq.go` and `memberreq_fixed.go`). A group owner can still link a channel he administers, which only affects his own group. `TestMemberReqLinkedChannel` passes | CLOSED |
| T-14-15 | 04 | T | med | mitigate | `lk:467-482` classification by status and by `cache.IsBotGoneError` (`fuku/utils/cache/adminCache.go:83-89`, by code 400 or 403, never by wording), with `ErrRateLimited` excluded first (`lk:469`). `TestMemberReqBotStatusClassifier` (a 500 whose text says "chat not found" is unknown, a 400 with any text is not an administrator) passes. Any 400 or 403 reads as not-administrator, which is the inert direction only (AR-04) | CLOSED |
| T-14-16 | 04 | D | med | mitigate | `lk:309,404,586` a nil answer is unknown; `lk:441-442,481` an unrecognised status merges to the empty status and is unknown; `lk:509-511` the bot-status answer is merged only when non-nil. `TestMemberReqChannelRule`, `TestSettingsMreqChannelStatusScreen` ("an unknown status", null answers) pass | CLOSED |
| T-14-17 | 05 | D | high | mitigate | `rules:165` `memberReqBroken` skips a nil `Broken`; `rules:197` `memberReqLookupMasks` skips a nil `Lookup`; the table gives each row exactly one of the two (`rules:104-117`). `TestMemberReqBrokenIgnoresLookupRules`, `TestMemberReqRuleTable`, `TestMemberReqLookupBroken` pass | CLOSED |
| T-14-18 | 05 | T | med | mitigate | `rules:121-128` the code must be a table row; `smreq:65-66` `settingsMreqOpenFields` and `settingsMreqYesFields` (`:641-653`, exactly `t`, `v` in 2..4 and `e`) run in step 1 before any I/O; `smreq:50-55` rule controls accept `st` only with `settingsMreqActionValue` (`:432`, one digit 0..4); `registerSettingsPage` panics on a duplicate key or code. `TestSettingsControlActionAllowList`, `TestSettingsMreqForgedFields` pass | CLOSED |
| T-14-19 | 05 | D | med | mitigate | the page is six rows and the codes are two letters, so no callback grows past 53 bytes. `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqPageFitsEveryLocale` pass | CLOSED |
| T-14-20 | 05 | R | med | mitigate | `rules` tests assert seven rules and bits 32 and 64 (`memberreq_rules_test.go:299-348`); no test counts rules to five (the `!= 5` hits are callback field counts and a `sendMessage` count); the notice ceiling moved from 400 to 520 (`memberreq_locale_test.go:92`) | CLOSED |
| T-14-21 | 06 | T | high | mitigate | an unknown or inert lookup adds no bit (`rules:200-206`); the lookups run at `enf:287` and the claim only at `enf:632`, after them, so a lookup cut off by a crash or shutdown leaves no hold (`lk:249` derives from the root context, `watch:379-393` cancels it); `enf:259-316` returns `ok=false` for an unreadable record, setting or re-read. `TestMemberReqLookupUnanswerable` (job and join), `TestShutdownEndsMemberReqLookups` (`shutdown_drains_test.go:292`) pass | CLOSED |
| T-14-22 | 06 | T | high | mitigate | the claim is still insert-first (`enf:632` `memberReqTakeHold`, `ClaimHold`'s `INSERT ... ON CONFLICT DO NOTHING`, Phase 13). `TestMemberReqLookupOnePunishmentAcrossReplicas` (Mute and Kick) and Phase 13's `TestMemberReqOnePunishmentAcrossReplicas` pass on SQLite (the PostgreSQL run is plan 14-12's) | CLOSED |
| T-14-23 | 06 | E | high | mitigate | `enf:625` `memberReqMuteOverRestriction` still runs after the lookups (`enf:619`) and uses the record the WR-03 re-read returned; `enf:764-779`. `TestMemberReqLookupNeverReplacesRestriction`, `TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqLiveRecordAfterLookups` pass | CLOSED |
| T-14-24 | 06 | D | high | mitigate | every flight and lookup derives from `memberReqContext()` (`lk:249`); `git diff cc19bba HEAD -- main.go fuku/modules/shutdown_drains.go` is empty, so no drain was added; `lk` never touches PostgreSQL (no `db.DB` or `memberreq.` reference), only Redis and Telegram. `TestShutdownEndsMemberReqLookups`, `TestShutdownWaitsForMemberReqJobs` pass | CLOSED |
| T-14-25 | 06 | D | med | mitigate | `enf:53` the join entry waits 4 s (`memberReqLookupJoinBudget`), `enf:320-325`; the flight may outlive it by up to 15 s but holds no update goroutine. `TestMemberReqJoinLookups` passes | CLOSED |
| T-14-26 | 07 | D | high | mitigate | `watch:178-223` the cached read is one pipeline plus two `EXISTS`, all under one 250 ms context (`watch:184`, `raid_counters.go:38`); `watch:335-336` the result is a named value preset to `ext.ContinueGroups` with the recover deferred; `watch:284-294` `memberReqMissExempt` reads only the cached admin list and approvals, no Telegram call. `TestMemberReqWatcherLookupRules` (no request), `TestMemberReqWatcherLookupContinuesGroups`, `TestMemberReqWatcherContinuesGroups` pass | CLOSED |
| T-14-27 | 07 | D | med | mitigate | `watch:272-274` D-25 skip on a miss (a cached administrator, an approved member, an approvals read that failed: an unknown never starts a job and so never punishes); `watch:304-307` the runner cap of 32 sheds at Debug; `watch:214-219` a channel the bot cannot read costs one fact per minute, not a job per message (WR-01). `TestMemberReqWatcherLookupRules`, `TestMemberReqChannelNotAdminCache` pass. Redis down costs a job per message and never a punishment (AR-06) | CLOSED |
| T-14-28 | 07 | E | high | mitigate | `held:44-68` the held release makes only read-only lookups (`held:81`) and deletes the hold row through `ClearHoldOnClean` (`held:59`); it never claims, restricts, warns, posts or lifts a mute. `TestMemberReqHeldRelease` (no `restrictChatMember`, ban or unban in any branch) passes | CLOSED |
| T-14-29 | 07 | T | med | mitigate | `watch:178-223` returns `satisfied` and `passHit` apart; `watch:264-271` a pass hit alone never ends the check; the only things cached are a pass (`lk:314,518,592`) and the channel-level `cna` fact. `TestMemberReqPassCacheWithLookups` passes | CLOSED |
| T-14-30 | 08 | E | high | mitigate | `fixed:96` the presser must be `u`, before any read; `fixed:131-147` the hold matched by ID, chat and member, then the live marker `isMemberReqMute`, before approvals (`:152`) and the judge (`:162`); no lookup precedes them. `TestMemberReqFixedNeverLiftsWithLookupRules`, `TestMemberReqFixedNeverLifts`, `TestMemberReqFixedRefusals` pass | CLOSED |
| T-14-31 | 08 | T | high | mitigate | `fixed:244` the lookups are `Fresh: true`: they bypass the photo, subscription and bot-status caches (`lk:288,295,494,567,573`) and a definite breach deletes a stale positive (`lk:317-319,522,615-617`). `TestMemberReqFixedLookups` passes | CLOSED |
| T-14-32 | 08 | T | high | mitigate | `fixed:247-253` a broken rule is the answer even when another lookup is unknown, and an unknown with nothing broken is `mreq_fixed_check_failed` with nothing changed; only a definite non-administrator or a 400 or 403 makes the channel inert (`lk:620-622`). `TestMemberReqFixedLookups`, `TestMemberReqChannelInert` pass | CLOSED |
| T-14-33 | 08 | I | high | mitigate | the remedy and the phrases are fixed words (`notice:130-155`); `TestMemberReqScreensFitEveryLocale` (`assertMemberReqNoLeak`: chat ID, "staff", the admin's name) and `TestMemberReqLookupAlertsFitEveryLocale` pass in 8 languages | CLOSED |
| T-14-34 | 08 | D | med | mitigate | `notice:75-89` five or more broken rules answer with the count key, up to four are listed. `TestMemberReqStillBrokenText`, `TestMemberReqScreensFitEveryLocale` pass | CLOSED |
| T-14-35 | 09 | I | high | mitigate | `smreq:259-290` the lines are fixed words with `{rule}` the translated rule name; `settings_mreq.go` reads no `Title`, username or channel ID into any screen (the only `Title` hit is `TitleKey`, `:60`). `TestSettingsMreqChannelStatus` (a channel title and ID known to the fake never appear), `TestSettingsStaffPrivacy` (8 languages, group and private non-creator menus) pass | CLOSED |
| T-14-36 | 09 | D | med | mitigate | `smreq:200-232` a basic group, the photo and name screens, Main and a page with the channel rule Off make no call; otherwise at most two calls under one 3 s context (`smreq:191`), through the lookup pacer, which sheds a far slot. `TestSettingsMreqOpenCost` passes | CLOSED |
| T-14-37 | 09 | T | med | mitigate | the status is classified by `lk:467-482`; `smreq:223-229` maps only a definite non-administrator to the not-administrator line and everything else to the unknown mark, never an error text or the cannot-open edit. `TestSettingsMreqChannelStatus`, `TestSettingsMreqChannelStatusScreen` (429, shed, 5xx, null, timeout) pass | CLOSED |
| T-14-38 | 09 | R | low | mitigate | `smreq:237,247` `p.answer("", false)` runs once, before the status read, in both draws; the gate's alert is its own single answer. `TestSettingsMreqOpenCost`, `TestSettingsMreqRedrawKeepsStatus` pass | CLOSED |
| T-14-39 | 10 | E | high | mitigate | `smreq:599-631` the live link check (`getChat` of the group, Fresh, through the lookup pacer under 3 s) runs after the whole pipeline, the expiry check and the fresh same-value read, at `smreq:579-581` (a press) and `:699-701` (the card's Yes); a basic group or no linked channel answers `settings_mreq_here_nolink`, writes nothing and redraws the Off-only screen. The only write path of a rule action is `applyMemberReqAction` (`smreq:710`, grep of non-test code), so the gate has no bypass. `TestSettingsMreqChannelRefusal` (forged `t=ch&v=3`), `TestSettingsMreqForgedFields` pass | CLOSED |
| T-14-40 | 10 | T | med | mitigate | the gate runs before any empty answer and its alert is the press's one answer (`smreq:618`); an unreadable link answers `mreq_fixed_check_failed` and returns false with no edit (`:609-612`). `TestSettingsMreqChannelRefusal` counts the answers | CLOSED |
| T-14-41 | 10 | E | high | mitigate | `smreq:52,66` every rule control (`ph` and `ch` included) and `my` declare `settingsRightRestrict`, `mo` declares `settingsRightAdmin`; the press pipeline judges the right on a fresh live `getChatMember` before the control runs and the gate comes after it. `TestSettingsAuthorityMreq`, `TestSettingsMreqCard` over all seven rules pass | CLOSED |
| T-14-42 | 11 | D | high | mitigate | two pacers with different keys (`enf:33-34`, `lk:88-89`), each shedding on its own; the lookup pacer is the only one the lookups use. `TestMemberReqPacersAreIndependent` passes. Residual: aggregate Telegram flood is not shared between the two (AR-07) | CLOSED |
| T-14-43 | 11 | I | high | mitigate | `TestSettingsStaffPrivacy` renders every reachable screen, the channel's included, with a channel whose title and ID the fake knows; `TestMemberReqLookupAlertsFitEveryLocale` fails any chat or channel digits; `TestMemberReqLookupLogsCarryIDsOnly` holds the logs to IDs and codes. No screen, alert or notice names the channel or the Staff Group. The channel's numeric ID appears in three Debug log lines (SEC-01, AR-10) | CLOSED |
| T-14-44 | 11 | T | med | mitigate | `fuku/db/backup/types.go:115-137` `AllExportableModules` has neither table; `TestMemberRequirementsStayOutOfBackup` (export, import and reset, with `PhotoAction` and `ChannelAction` set, `memberreq_backup_test.go:37,79-80`) passes | CLOSED |
| T-14-45 | 11 | D | med | mitigate | `TestMemberReqLookupAlertsFitEveryLocale` measures the gate's alert (`settingsMreqNoLinkAlert`, `smreq:633-639`) and `mreq_fixed_check_failed` at most 200 code points in 8 languages | CLOSED |
| T-14-46 | 12 | R | med | mitigate | `AGENTS.md` carries the Phase 14 rules (the lookup keys, pacers, flights, the watcher read, the press pipeline, the channel gate); `make check-docs` run at HEAD: no drift, tree untouched; `.planning/REQUIREMENTS.md:154,163-165` scopes MREQ-01 to "seven of the eight" and MREQ-10 to Phase 14 and 15; `make -n test-postgres-integrity` lists the migration defaults test. No stale "five rules" remains in `AGENTS.md` (the hits say "five name rules") | CLOSED |
| T-14-47 | 12 | T | med | mitigate | `14-SPIKE.md:12-40` opens with the pre-enable section (Warn and Off need no spike; photo Mute, Kick or Ban needs checks 1 and 4; channel Mute, Kick or Ban needs 2, 3 and 5), and the code is safe under every outcome (an unknown never punishes, D-22, D-19). Live checks stay the owner's (AR-02) | CLOSED |
| T-14-48 | 12 | D | med | mitigate | `TestMemberReqLookupEndToEnd` (`memberreq_lookup_e2e_test.go:79`, photo and channel, through the real dispatcher) passes; the CI-style gate passed after the review fix pass (`14-REVIEW-FIX.md`, exit 0, no unexpected skip) | CLOSED |
| T-14-SC | 01-12 | T | high | mitigate | `git diff --exit-code 8c0c827 HEAD -- go.mod go.sum` exits 0 and `git diff --stat cc19bba HEAD -- go.mod go.sum` is empty; no package was installed | CLOSED |

**Prohibitions in the plans:** all hold in the code. No lookup runs before the presser check and the live marker in "I fixed
it" (`fixed:96-162`); the watcher makes no Telegram call and has no return other than `ext.ContinueGroups`
(`watch:178-223,335-366`); `lk` reads and writes Redis and Telegram only; no lookup verdict other than `memberReqBreach`
reaches a punishment (the three consumers of a verdict are `enf:287` the executor, `fixed:244` which can only withhold a lift,
and `held:81` which can only delete a hold row); the channel never comes from callback data or an update (T-14-14); the
migration is one new file with no top-level `ADD CONSTRAINT`, no `BEGIN`, `COMMIT` or `CONCURRENTLY`; `main.go`,
`shutdown_drains.go`, `fuku/config`, `go.mod` and `go.sum` are unchanged since the phase base `cc19bba`, so there is no new
drain, secret, config key or dependency.

---

## Code Review Findings Verified in the Code (14-REVIEW.md, fix pass `b3413c2..5b87552`)

The review has no critical finding. WR-01 to WR-05 were security-relevant (WR-03 to WR-05 widened a time-of-check window, WR-02
could leave a stricter rule unjudged, WR-01 was a cost lever). Each fix has a RED `test(14)` commit and a GREEN `fix(14)`
commit (`git log b3413c2^..5b87552`).

| Finding | Fix evidence at HEAD | Status |
|---|---|---|
| WR-01: a channel the bot cannot read cost a job and up to three paced calls per message | `lk:340-347` the channel-level key `fuku:memberreq:cna:<channel>`; `lk:516-526` the bot-status flight writes it on a definite not-administrator (a member, restricted, left or kicked status, or a 400 or 403 by code) and deletes it on an administrator, and a 429, shed, 5xx, timeout or null answer writes nothing; `lk:563-565` read at the top of `memberReqChannelRule` (inert at once, no request, never a breach, bypassed by Fresh); `watch:214-219` read in the watcher after the subscription key misses; `watch:306` the shed line is Debug. The key names the channel only, holds the fixed marker `1` and expires in 60 s, so it can only turn an unknown into an inert rule. `TestMemberReqChannelNotAdminCache` passes | FIXED, verified |
| WR-02: an unknown lookup left a weaker punishment standing and its hold kept the stricter rule from ever being judged | `rules:190-208` the unknown verdicts are a mask; `rules:217-228` `memberReqUnknownDefers`; `enf:287,310-314` the executor claims nothing when a rule that could not be answered is stricter (by `memberReqSeverity`, `rules:233-245`) than the strictest broken rule, with one ID-only Info line. `TestMemberReqPartialEpisode` (the review's scenario, then the ban lands once with both bits) passes. The deferral is unbounded (RV-NEW-01, AR-08) | FIXED, verified |
| WR-03: the live record was read before the lookup phase and acted on up to 20 s later | `lk:209-226,246` the touch marker is set by any flight start or join; `enf:282-306` when something is broken and the phase made a request or took over 1 s, the record is read again through `memberReqLiveMember`, eligibility, names, the mute-over-restriction check and the marker adoption use it, and a failed read punishes nobody. `TestMemberReqLiveRecordAfterLookups` (an administrator promoted during the phase is never actioned) passes. The approvals and captcha reads are not repeated (SEC-02, AR-09) | FIXED, verified |
| WR-04: "I fixed it" proved the marker, spent up to 5 s on lookups, then lifted unconditionally | `fixed:233-255` `memberReqFixedJudge` reports whether a lookup went beyond the cache; `fixed:181-190` the live record is read again right before the take and the press is `mreq_fixed_not_ours` when the marker is gone (an admin's `/mute` that landed meanwhile is never undone) or `mreq_fixed_check_failed` when the read fails, changing nothing; no extra request when step 5 made none. `TestMemberReqFixedMarkerRecheck` passes | FIXED, verified |
| WR-05: a punishment could rest on a linked-channel fact up to 10 minutes old | `lk:63-66,408-416` a positive link is cached 60 s and "none" 10 minutes (the inert direction); `lk:608-618,630-633` a not-subscribed candidate reads the link fresh through the lookup pacer first and a changed, removed or unreadable link is unknown, never a breach, while the fresh read refreshes `lc`. `TestMemberReqStaleLink` passes | FIXED, verified |
| IN-03: the link and bot-status flights did not re-read their cache inside the flight | `lk:389-393` and `lk:498-501`. `TestMemberReqFlightsReadCacheAgain` passes | FIXED, verified |

**Deferred review items** (`14-REVIEW-DISPOSITION.md`): none of IN-01, IN-02, IN-04, IN-05, the fix pass's NEW-01 to NEW-03 or
the two WR-01 companions deferred by decision leaves an open threat at or above the `high` gate, and none widens anyone's
authority. Each is an accepted risk below and none is dropped silently.

| Item | Threat touched | Why it is not an open security threat | Accepted as |
|---|---|---|---|
| RV-IN-01 an approved member with a lingering hold is not cleaned while a lookup rule is on and they have no cached positive | T-14-27 | the hold records a real current breach; the held release (`held:44-68`) ends it at the member's first message once the lookups pass, an approved member is exempt from every new punishment, and a hold ends at a leave or rejoin | AR-12 |
| RV-IN-02 the held release runs before the pause check, so a paused chat still spends lookups on held members | T-14-28 | it only deletes a hold row and never punishes; the lookups sit under the lookup pacer and a pause lasts 10 minutes at most | AR-11 |
| RV-IN-04 the claim has no remaining-budget guard | T-14-21 | a job that claims with seconds left and then fails leaves a pending hold that the 2-minute reclaim retries once; nobody is punished twice or wrongly | AR-13 |
| RV-IN-05 tests swap package globals an abandoned flight goroutine may still read under `-race` | none (test hygiene) | not a product behaviour; `-race` passed on every run here and in CI, and the remedy (pass the table as a parameter) is named | AR-14 |
| RV-NEW-01 a stricter lookup rule that stays unanswerable holds the weaker name punishment back | T-14-21 | nobody is punished on a guess; the cost is evasion of the weaker punishment while the stricter rule cannot be answered (an outage or a saturated lookup pacer); a bot that is no channel administrator is inert through `cna` after one message | AR-08 |
| RV-NEW-02 each not-subscribed breach candidate costs one fresh `getChat` | T-14-36 | cost only, under the lookup pacer and one `f:l:<chat>` flight per group; a subscribed member costs none | AR-15 |
| RV-NEW-03 the touch marker is set by any flight start or join | T-14-23 | the only effect is one extra paced live read, which is the safe direction | AR-16 |
| RV-WR-01 companions: the per-(chat, user) job gate and a reserved runner lane (deferred by the orchestrator's decision) | T-14-10, T-14-27, T-14-42 | availability only, no wrong punishment (SEC-04); the lockdown and the raid counters use other pacers and counters | AR-07 |

---

## New Threats Found by the Audit (unregistered)

The 12 plan summaries raise no unregistered threat flag (every `## Threat Flags` section is "None" or points at planned IDs).
The review's findings are covered above. The audit found four more items; none reaches the `high` gate, and none is a path to
a wrong punishment.

| ID | Severity | Finding and scenario | Disposition |
|---|---|---|---|
| SEC-01 | low | **The linked channel's numeric ID is written to three Debug log lines, which the log test does not forbid.** `memberReqLookupLogFailure` is called with the channel as its chat argument at `lk:513` (the bot's status) and `lk:583` (a subscription), and `lk:587` logs "in channel %d". `TestMemberReqLookupLogsCarryIDsOnly` forbids a channel title, file ID, names, description, credential and notice texts, not the ID. The production level is Info unless `DEBUG=true` (`fuku/config/config.go:659-661`), the ID is not a name or a secret, and `14-SPIKE.md` says the bot logs "by chat, channel and user IDs". The stricter property "no channel ID in any log line" does not hold literally | **Accepted** (AR-10). To make it hold, pass `0` or the group's ID in those three calls and add the channel ID to `mreqLogForbidden` |
| SEC-02 | low | **The approved and captcha-pending exemptions are read once, before the lookup phase, and not again before the claim.** `memberReqExempt` is called only at `enf:596`; the WR-03 re-read (`enf:294-306`) refreshes the live record only. An admin who approves a member, or a member whose captcha attempt starts, while that member's job is inside its lookup phase (at most 20 s for a job, 4 s for a join) does not stop the job from claiming and punishing. One punishment of an approved member; an admin can `/unmute` or `/unban`, and for a mute the approved member's "I fixed it" frees them at step 4b | **Accepted** (AR-09), the Phase 13 AR-11 class with the window widened from milliseconds to the lookup budget. Optional hardening: run `memberReqExempt` again inside the WR-03 re-read branch |
| SEC-03 | medium | **A demoted bot is still read as a channel administrator for up to 2 minutes.** `lk:494-496` returns administrator on a cached `ba` key and the D-22 gate (`lk:638-646`) trusts it. Scenario: the channel owner removes the bot's rights while a punishing channel value is set. If Telegram then answers `left` instead of an error to a non-administrator bot (spike S2, not yet checked live), members with no cached subscription positive are breaches for up to `memberReqBotAdminTTL` (`lk:69`), punished at most 10 per minute per chat by the cap. Warn removes nobody at once | **Accepted** (AR-03): the owner's pre-enable checks 2, 3 and 5 are the control. Optional hardening: read the bot's status Fresh on a breach candidate (one coalesced `f:b:` call, the shape of WR-05's link re-read) |
| SEC-04 | medium | **Nothing limits a person's presses of the channel screen or "I fixed it", and one 32-job runner serves every chat of a replica.** A page open or `mo` costs up to two lookup-pacer slots; "I fixed it" up to four fresh lookups plus up to three Phase 13 pacer calls; `watch:56` `memberReqMaxInFlight` is one counter for all chats (`watch:304`). Scenario: a busy group with the channel rule on, or alts in a group with a lookup rule on, produce about six uncached-member messages a second, or a muted alt presses the button in a loop; the lookup pacer sheds past 5 s (unknown), the runner reaches 32 and sheds other groups' jobs on that replica, so name violators elsewhere go unpunished until the load drops (their next message is checked again). Nobody is punished wrongly; lockdown triggers, the join and flood counters and the raid AI pool use other pacers and counters. Phase 13 AR-18, made easier by the lookup phase of up to 20 s | **Accepted** (AR-07): the per-chat gate and the reserved lane were deferred by decision. Recommended before turning a lookup rule on in a large group: a per-chat in-flight cap in `startBelow` |

---

## Accepted Risks Log

Phase 13's AR-01 to AR-18 (`13-SECURITY.md`) remain in force; the entries below are this phase's.

| Risk ID | Threat Ref | Rationale |
|---------|------------|-----------|
| AR-01 | T-14-08, T-14-11, MREQ-11 | A photo hidden by privacy settings is counted as missing (D-01, spike S1): a legitimate member can be warned, kicked, muted or banned, and Warn counts toward the group's warn limit, so repeated episodes (a leave and rejoin starts one) can reach its limit action. The member can fix it only in their own privacy settings. Controls: the rule's hint recommends Warn or Mute (`settings_mreq_hint_photo`), Kick and Ban need the two-tap card, the cap of 10 punishments a minute per chat, an approved member is exempt, the warn, kick and mute notices end with the remedy (`mreq_remedy_photo`), and an admin can approve or `/unban`. A Ban notice carries no remedy sentence (a banned member has nothing to tap) |
| AR-02 | T-14-08, T-14-13, T-14-14, T-14-47 | The live Telegram checks (spike checks 1 to 10: what a hidden photo returns, what a non-administrator bot is told, whether the linked channel reaches a plain member bot, the member statuses, two replicas, a raid with the pacer shedding, a restart, a relink) are the owner's and were deferred as in Phases 7 to 13. Mute, Kick and Ban on the photo or channel rule should not be enabled in a real group before the pre-enable checks of `14-SPIKE.md` are run; Warn and Off do not depend on them |
| AR-03 | T-14-13, SEC-03 | The bot's administrator standing is cached 2 minutes (`ba`) and its not-administrator fact 60 s (`cna`), so a demoted bot is still believed an administrator for up to 2 minutes, and an owner who fixes the rights is believed again within a minute |
| AR-04 | T-14-15 | `cache.IsBotGoneError` matches any 400 or 403 (spike R3), so a refusal for another reason reads as not-administrator. That is the inert direction only and never a punishment |
| AR-05 | T-14-11, T-14-27 | Only passes are cached (D-24): a photo or subscription pass lives 30 minutes, so a member who removes their photo or unsubscribes is not checked again at the watcher for up to 30 minutes ("I fixed it" is always fresh); a member who stays without either costs one paced lookup per message. A stale "no linked channel" lasts up to 10 minutes and only makes the rule inert |
| AR-06 | T-14-10, T-14-27 | Redis down: nothing is cached or reserved, every message of a non-exempt member in a group with a lookup rule on is a job (bounded by the 32-job runner), the lookup pacer falls back to one replica's interval and still sheds a far slot. Never a punishment; the cost is documented in `AGENTS.md` and `14-SPIKE.md` |
| AR-07 | T-14-10, T-14-36, T-14-42, SEC-04, RV-WR-01 companions | The runner is one 32-job counter per replica for all chats and the two pacers are separate budgets, so a lookup-heavy group can shed other groups' jobs, a Telegram 429 caused by lookups is seen by the lookup pacer only, and no per-user limit exists on presses (Telegram's own callback flood limits apply). Availability only, never a wrong punishment; extends Phase 13 AR-18 |
| AR-08 | RV-NEW-01, T-14-21 | A stricter lookup rule that stays unanswerable defers a weaker name punishment for as long as it does (an outage, or a saturated pacer a raider causes); nobody is punished on a guess and the member is judged again at every message |
| AR-09 | SEC-02, T-14-21 | The approved and captcha exemptions are not re-read after the lookup phase: a time-of-check to time-of-use window of up to 20 s (a job) or 4 s (a join), of the same class as Phase 13 AR-11 |
| AR-10 | T-14-12, T-14-43, SEC-01 | The linked channel's numeric ID (never its title or a name) appears in three Debug log lines, off in production; `memberReqErrText` returns a non-Telegram error as it is, which is safe because gotgbot's `BaseBotClient` replaces the bot token in the URL of a transport error (`request.go:153,258` of the pinned gotgbot, the only client `main.go:106-107` builds) and `logredact` scrubs the registered token from every line (`fuku/config/config.go:609-619`). The limits: an error from another HTTP client would not be scrubbed by gotgbot, and only an `*url.Error` is rewritten |
| AR-11 | RV-IN-02, T-14-28 | The held release makes its lookups before the pause check, so a paused chat spends lookups on held members; it only deletes a hold row |
| AR-12 | RV-IN-01, T-14-27 | An approved member's lingering hold is not cleaned by the watcher while a lookup rule is on and no positive is cached; it ends at the held release, a leave or a rejoin |
| AR-13 | RV-IN-04, T-14-21 | The claim has no remaining-budget guard; a job that claims late and fails leaves a pending hold the reclaim retries after `memberreq.StaleClaim` |
| AR-14 | RV-IN-05 | Tests that swap package globals while an abandoned flight runs could in principle race under `-race`; none did here or in CI |
| AR-15 | RV-NEW-02, T-14-36 | Each not-subscribed breach candidate costs one fresh `getChat` of the group through the lookup pacer, repeated per attempt for a member whose punishment is refused or backed off |
| AR-16 | RV-NEW-03 | The touch marker over-triggers on a flight whose in-flight cache re-read hits, costing one safe extra paced live read |
| AR-17 | T-14-03, T-14-18 | Rolling-deploy caveats (`14-SPIKE.md`): an old replica answers `t=ph` or `t=ch` as an invalid request, ignores the two columns and screens for neither, and the shared settings cache may serve an entry without them until it expires; the pass fingerprint changes once per member (one harmless miss); the migration runs before any replica serves |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open (blocking) | Open (non-blocking) | Run By |
|------------|---------------|--------|-----------------|---------------------|--------|
| 2026-11-08 | 49 distinct IDs (60 plan rows) | 49 (T-14-13 and T-14-43 closed with a documented residual: SEC-03 and SEC-01) | 0 | 0 (four new items SEC-01 to SEC-04, none at or above `high`, are accepted as AR-03, AR-07, AR-09 and AR-10) | gsd-security-auditor |

Tests run at `a770096`, all passing with no failure and no skip, under `-race -count=1`:
`./fuku/modules` 62 top-level tests: `TestMemberReqVerdictZeroValue`, `TestMemberReqPhotoRule`,
`TestMemberReqChannelBreachNeedsBotAdmin`, `TestMemberReqBotStatusClassifier`, `TestMemberReqLookupSingleflight`,
`TestMemberReqLookupPacer`, `TestMemberReqLookupUnanswerable`, `TestMemberReqStaleLink`, `TestMemberReqChannelNotAdminCache`,
`TestMemberReqPartialEpisode`, `TestMemberReqLiveRecordAfterLookups`, `TestMemberReqFixedMarkerRecheck`,
`TestSettingsMreqChannelRefusal`, `TestSettingsMreqForgedFields`, `TestMemberReqLookupLogsCarryIDsOnly`,
`TestShutdownEndsMemberReqLookups`, `TestMemberReqHeldRelease`, `TestMemberReqWatcherLookupRules`,
`TestMemberReqFixedLookups`, `TestMemberReqPacersAreIndependent`, `TestMemberReqLookupEndToEnd`,
`TestMemberReqLookupOnePunishmentAcrossReplicas`, `TestMemberReqLookupNeverReplacesRestriction`,
`TestMemberReqWatcherLookupContinuesGroups`, `TestMemberReqFixedNeverLiftsWithLookupRules`, `TestMemberReqChannelInert`,
`TestMemberReqPassCacheWithLookups`, `TestMemberReqLookupFlightsKeepModesApart`, `TestMemberReqFlightsReadCacheAgain`,
`TestMemberReqLinkedChannel`, `TestMemberReqChannelRule`, `TestSettingsMreqChannelStatus`, `TestSettingsMreqOpenCost`,
`TestMemberReqJoinLookups`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqCard`, `TestSettingsStaffPrivacy`,
`TestSettingsAuthorityMreq`, `TestSettingsDriftMreq`, `TestMemberReqLookupAlertsFitEveryLocale`,
`TestMemberReqScreensFitEveryLocale`, `TestSettingsControlActionAllowList`, `TestMemberReqRuleTable`,
`TestMemberReqBrokenIgnoresLookupRules`, `TestMemberReqEnforcementLive`, `TestMemberReqWatcherContinuesGroups`,
`TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqFixedNeverLifts`, `TestMemberReqFixedRefusals`,
`TestMemberReqOnePunishmentAcrossReplicas`, `TestMemberReqRunnerSheds`, `TestShutdownWaitsForMemberReqJobs`,
`TestSettingsMreqRowBudget`, `TestMemberReqStillBrokenText`, `TestSettingsMreqBasicGroup`,
`TestSettingsMreqRedrawKeepsStatus`, `TestMemberReqCapPausesPunishments`, `TestSettingsMreqPageFitsEveryLocale`,
`TestSettingsMreqChannelStatusScreen`, `TestMemberReqLookupBroken`, `TestMemberReqLookupCacheKeys`,
`TestMemberReqLookupCapAndPause`. Also `./fuku/db/memberreq` (`TestChatRequirementsSettings`, `TestRequirementHolds`,
`TestRequirementHoldClaimConcurrent`), `./fuku/db/migrations` (`TestMemberRequirementLookupMigrationStatements`),
`./fuku/db/backup` (`TestMemberRequirementsStayOutOfBackup`), `./fuku/db/cache` (`TestSkipLocalMemberRequirementsKey`) and
`./fuku/i18n` (`TestMemberReqLocaleKeys`, `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`,
`TestMemberReqNoProfileInReworkedKeys`). `make check-docs`: no drift. `git diff --exit-code 8c0c827 HEAD -- go.mod go.sum`:
exit 0. Read directly rather than taken from a summary: gotgbot's `sanitizeError` (`request.go:153,258`), the pacer's shed rule
(`pacer:140-186,215-231`), the production client (`main.go:106-107`) and the secret registration
(`fuku/config/config.go:609-619`). No source or test file was changed.

Not run: `make test`, `make lint`, `make check-translations` (its target runs `go mod tidy`, which could write files), the
PostgreSQL integrity target as a whole (`TestMemberRequirementLookupMigrationDefaults` skips locally and was proved on
PostgreSQL 16 by plan 14-12) and the live Telegram checks of `14-SPIKE.md`, which are the owner's.

**threats_open:** 0
