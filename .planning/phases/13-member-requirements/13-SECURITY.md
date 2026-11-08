---
phase: "13"
slug: "member-requirements"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate, "high")
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-08"
---

# Phase 13 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `c048369` (all 14 plans executed, plus the code review fix pass
> `a1820fd..9663ac8` and its docs commits). Every mitigation was read in the code at HEAD, not taken from a summary, and
> the security-relevant tests were run, a throwaway PostgreSQL 16 cluster included (migration chain, the three database
> tests, the cross-replica and "I fixed it" tests). Evidence is `file:line` at `c048369`. Short names: `enf` =
> `fuku/modules/memberreq_enforce.go`, `watch` = `memberreq_watch.go`, `fixed` = `memberreq_fixed.go`, `join` =
> `memberreq_join.go`, `notice` = `memberreq_notice.go`, `lim` = `memberreq_limits.go`, `unm` = `memberreq_unmute.go`,
> `warned` = `memberreq_warned.go`, `rules` = `memberreq_rules.go`, `mod` = `memberreq.go` (all under `fuku/modules/`),
> `smreq` = `fuku/modules/settings_mreq.go`, `menu` = `fuku/modules/settings_menu.go`, `warns` = `fuku/modules/warns.go`,
> `holds` = `fuku/db/memberreq/holds.go`, `srepo` = `fuku/db/memberreq/settings.go`, `mig` =
> `migrations/20261106120000_add_member_requirements.sql`, `loc` = `fuku/i18n/memberreq_locale_test.go`.

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | Any group member → message watcher (group -2, every supergroup message) | first name, last name, username (attacker-chosen, change at any time) |
| 2 | Telegram user → `mreq\|v1` "I fixed it" callback | hold ID `h`, member ID `u` (callback data is not authenticated) |
| 3 | Telegram user → `settings\|v1` callbacks of page `m` (rule controls, `mo`, `my`, the two switches) | page, control, action, value `v`, expiry `e` |
| 4 | Member profile → name rules and the public notice | a name spliced into HTML after translation |
| 5 | Replicas → PostgreSQL (the hold latch) and Redis (cap, pause, pass cache, warned note) | `(chat_id, user_id)` claim, counters |
| 6 | Bot → Telegram | punishments, notices, deletes, unmutes (every one can be refused, rate limited or lost) |
| 7 | Requirements → every other restriction path (admin, staff, captcha, antiflood, warn limit, lockdown) | one member record, one `until_date` marker |
| 8 | Join and captcha paths → requirement entry, guard and reset | the member's hold, the welcome |
| 9 | Backup file → database (owner-only import) | modules a file may carry |
| 10 | Deploy → PostgreSQL (`AUTO_MIGRATE=true`) | migration 20261106120000 |
| 11 | Group menu and a non-creator's private menu → every reader | page text, buttons, answers |
| 12 | Process → shutdown | enforcement jobs in flight |
| 13 | Process logs | IDs, codes and errors only |

---

## Threat Register

59 distinct IDs: T-13-01 to T-13-03, T-13-05 to T-13-59 and T-13-SC (72 rows in the 14 plans, because every plan registers
T-13-SC). T-13-04 is not allocated in any plan (the numbering jumps from 03 to 05). T-13-56 to T-13-59 were added to plans
09, 10 and 11 after the first numbering. No plan register has a severity below `low`; every disposition is `mitigate`.

| Threat ID | Plan | Cat | Sev | Disposition | Mitigation (evidence, and the test that proves it) | Status |
|---|---|---|---|---|---|---|
| T-13-01 | 01 | T | high | mitigate | `holds:38-51` `ClaimHold` is `INSERT ... ON CONFLICT (chat_id, user_id) DO NOTHING` and returns `RowsAffected == 1` (`:50`), behind `mig:51` `uk_chat_requirement_holds_chat_user`. `TestRequirementHoldClaimConcurrent` passes on SQLite and on PostgreSQL 16 (migration-chain schema); `TestMemberReqOnePunishmentAcrossReplicas` passes on both; the three database tests and `./fuku/db/memberreq` are in the `Makefile` integrity list | CLOSED |
| T-13-02 | 01 | T | med | mitigate | `holds:170` `cleanMargin` 2 s; `holds:186-189` `ClearHoldOnClean` deletes only holds created before `msgDate - 2 s`, a mute only when confirmed earlier or a pending one older than `StaleClaim`. `TestRequirementHolds` (SQLite, PostgreSQL 16) | CLOSED |
| T-13-03 | 01 | T | low | mitigate | `srepo:142-148` refuses an unknown rule and a value outside 0..4 before any write; `mig:33-37,52-53` seven named CHECKs, confirmed in `pg_constraint` on PostgreSQL 16, where a direct insert of action 5, hold rules 0 and hold action 0 each failed with its named check. `TestChatRequirementsSettings`, `TestApplyMemberReqAction` | CLOSED |
| T-13-05 | 02 | T | med | mitigate | `rules:59-66` NFC then a rune scan; `rules:122-136` the visible set (letters, numbers, punctuation, symbols, minus Other_Default_Ignorable and U+2800), `:129` the blank test. `TestMemberReqRulesCorpus`, `TestMemberReqBlankLastName` | CLOSED |
| T-13-06 | 02 | T | high | mitigate | `rules:241-250`: name cut to 64 runes (`:246`), wrapped in U+2068 and U+2069 (`:249`), `formatting.MentionHtml` escapes with `html.EscapeString` (`formatting.go:161-167`); the name is spliced after translation (`notice:108-112`, `lockdownSplice` `lockdown.go:229`), so it never reaches the translator. `TestMemberReqMention`, `TestMemberReqNoticeLanguage` (a name `{rules}<b>` arrives literal and escaped through the real send path). HTML and markup injection is closed. A bidi control embedded in the name is not stripped (NEW-01, low) | CLOSED |
| T-13-07 | 02 | D | high | mitigate | `rules:48-54` the rule is the eight letters ы ъ э ё Ы Ъ Э Ё only; `rules_test.go:37-51` 15 Ukrainian rows must pass. `TestMemberReqRulesCorpus` | CLOSED |
| T-13-08 | 03 | T | med | mitigate | `warns:118-148` `applyWarn`: warn lock, `afterRecord` inside it right after the store (`:130`), then the limit action, then the reset; the warn commands and blacklist warns call it. `TestApplyWarn`; no existing warn test was edited (`git diff 8c0c827..HEAD` adds only `warns_apply_test.go` under the warn tests) | CLOSED |
| T-13-09 | 03 | R | low | mitigate | `warns:145` `LimitApplied` is set only after the action and the reset succeeded; `notice:97` picks a removal text only when `LimitReached && LimitApplied`; `enf:364` sets it from `LimitErr == nil` only for a recorded warn whose reset failed. `TestApplyWarn` ("limit action refused", `warns_apply_test.go:217`), `TestMemberReqNoticeWarnLimit` | CLOSED |
| T-13-10 | 04 | T | med | mitigate | `loc:161` fails any `mreq_` template with `<`, `>` or `&`; placeholders equal English. `TestMemberReqLocaleKeys`, `TestSettingsLocaleKeys`; `make check-translations` clean | CLOSED |
| T-13-11 | 04 | D | med | mitigate | ceilings and rendered worst cases (`loc:153-225`: notices 400, answers 200, still-broken 160/200). `TestMemberReqLocaleKeys`, `TestMemberReqScreensFitEveryLocale` | CLOSED |
| T-13-12 | 05 | E | high | mitigate | `smreq:43-44` both switches carry `settingsRightRestrict`; the press pipeline makes the live `getChatMember` (`menu:382-388`), the owner gate (`:419`) and then `checkRight` (`:568`) before the control's `Set`. `TestSettingsAuthorityMreq` | CLOSED |
| T-13-13 | 05 | T | med | mitigate | `smreq:43-44` `Validate: settingsEditSwitchValue`; `menu:296-317` `settingsControlFieldsValid` (the control must exist on the page, `Validate` judges `v`) in step 1, before any I/O. `TestSettingsMreqSwitches` ("a value that is not 0 or 1 is refused before any lookup") | CLOSED |
| T-13-14 | 05 | I | med | mitigate | `smreq:104-117` the page reads only `chat_requirements` through `memberReqSettingsLookup`; the file never reads or names the Staff Group. `TestSettingsStaffPrivacy` renders page `m` in 8 languages and a private non-creator run | CLOSED |
| T-13-15 | 06 | D | high | mitigate | `smreq:361-383` `settingsMreqChoose`: a value of Kick or above draws the card and writes nothing (`:377-381`); the card says it acts on every member (`:402-420`, `settings_mreq_confirm_*`). `TestSettingsMreqCard` ("asks first and Yes writes", `settings_mreq_card_test.go:94`) | CLOSED |
| T-13-16 | 06 | E | high | mitigate | `smreq:62-63` `my` declares `settingsRightRestrict` and `Validate` `settingsMreqYesFields` (`:387-396`: `t` a rule, `v` one of 2..4, `e` a valid minute), so a forged `my` with `v` 0 or 1 is refused in step 1 and again in `settingsMreqYes` (`:426-443`); the right is judged at step 5 on a fresh live member record, so a presser demoted since is refused. `TestSettingsMreqCard` ("a presser demoted between the two taps", "a forged my is refused before any lookup"), `TestSettingsMreqCallbackBytes` | CLOSED |
| T-13-17 | 06 | T | med | mitigate | `smreq:48-51` `Actions: []string{settingsActSet}` on the rule controls; `menu:296-317` refuses any other code in step 1. `TestSettingsControlActionAllowList` | CLOSED |
| T-13-18 | 06 | T | low | mitigate | `smreq:37-39,364` the same-value check reads `memberreq.GetSettingsFresh` (database, no cache); an unreadable fresh row writes nothing and draws no card. `TestSettingsMreqCard` ("same value is judged by a fresh read, never the cache", "an unreadable fresh row draws no card and writes nothing") | CLOSED |
| T-13-19 | 07 | E | high | mitigate | `enf:143-173` one live `getChatMember` through the pacer is the admin check, and `memberReqLiveEligible` lets only a plain member or an in-group restricted non-bot through; `enf:240-247` any lookup error is no violation; the watcher's cached list only ever skips (`watch:218`); no `memberreq*.go` references `IsUserAdmin`. `TestMemberReqEnforceExemptions`, `TestMemberReqJoinExemptions` | CLOSED |
| T-13-20 | 07 | D | high | mitigate | `enf:201-216` approvals through `IsUserApprovedContext` (`fuku/db/approvals/repository.go:71`: a load error is an error, not "nobody approved") and the two captcha reads; every error returns exempt. A grep of `memberreq*.go` finds no `IsUserApproved(`, `IsUserAdmin` or `aispamCarvedOut`. `TestMemberReqEnforceExemptions` | CLOSED |
| T-13-21 | 07 | T | high | mitigate | `enf:553-566` the claim comes before any punishment (`holds:38-51`); a stale hold is reclaimed by one conditional update (`holds:59-83`); warn: the hold is marked punished inside the warn lock (`enf:346-352`, `warns:130`). Fix WR-01: three bounded mark attempts (`enf:319-333`), a Redis note of the stored warn (`warned:36-47`) that the reclaim adopts (`warned:80-93`, `enf:556`), and an `applyWarn` error with the warn recorded still marks and announces (`enf:353-365`). `TestMemberReqEnforceWarnOnce`, `TestMemberReqWarnMarkInsideLock`, `TestMemberReqWarnRecovery` (SQLite and PostgreSQL 16) | CLOSED |
| T-13-22 | 07 | T | high | mitigate | as T-13-06: `notice:108-112` and `rules:241-250`; `notice:123-157` sends HTML with link previews off. `TestMemberReqNoticeLanguage` | CLOSED |
| T-13-23 | 07 | I | med | mitigate | `TestMemberReqScreensFitEveryLocale` renders every notice and answer in 8 languages and fails on a chat ID, "staff" or an unfilled token (`assertMemberReqNoLeak`); every `log.*` line of `memberreq*.go` carries chat, user, hold or message IDs and `memberReqErrText` (`enf:128-134`: a Telegram error by its code, never its description); no name, rule text or notice text reaches a log (grep) | CLOSED |
| T-13-24 | 07 | D | med | mitigate | the same test holds every notice to 400 visible code points with a 64-rune name and five rules; `TestMemberReqLocaleKeys` | CLOSED |
| T-13-25 | 08 | D | med | mitigate | `enf:383-389` `memberReqBackoff` refreshes `created_at`, so the next messages are the held path with no Telegram call; `enf:376-378,398-416` a refusal is a 4xx other than 429 by code or the pacer's `ErrRateLimited`. `TestMemberReqPunishRefused` | CLOSED |
| T-13-26 | 08 | D | high | mitigate | `notice:123-157` a mute notice is retried (after `ErrRateLimited` until the 5 minute budget, else once); `notice:183-201` `memberReqUndeliveredMute` lifts through the shared unmute code and reverts the hold to pending (`enf:468-471`). `TestMemberReqMuteNoticeUndeliverable`, `TestMemberReqMuteNoticeRetry`. Residuals: two refusals in a row leave the mute (AR-03); the lift does not re-check the marker (NEW-02) | CLOSED |
| T-13-27 | 08 | E | high | mitigate | every lift takes its permissions only from `unm:18-29` → `resolveUnmutePermissionsWithLockdown` (`fixed:157`, `join:233`, `notice:186`); `MutedPermissions` appears only in the punishment (`enf:452`) and the captcha guard's re-mute (`join:153`). `TestMemberReqFixedDuringLockdown`, the lockdown case of the rejoin tests. Live check A5 is the owner's (13-SPIKE, deferred, AR-02) | CLOSED (code) |
| T-13-28 | 08 | T | med | mitigate | `enf:398-416` an unknown answer of a mute is checked against the stored marker (`isMemberReqMute`, `enf:721-723`) before it counts as applied or backs off; `enf:545,636-644` a reclaimed pending mute whose marker is live is adopted, never sent twice. `TestMemberReqPunishUnknown` | CLOSED |
| T-13-29 | 09 | D | high | mitigate | `mod:50-59` the button handler (`:53`) and the switch's default true (`:59`) are in the same module and `TestMemberReqEnforcementLive` asserts them together; the plan's intermediate state (switch false until 13-13) no longer exists by design, which is T-13-51; `TestMemberReqWatcherOffUntilLive` still proves the switch turns the watcher, the join entry and the guard off | CLOSED |
| T-13-56 | 09 | T | low | mitigate | `mod:36-42` `atomic.Bool`, one reader `memberReqLive`, one production writer (`init`, `:59`); tests write it only through `withMemberReqLive`; the whole family ran under `-race` | CLOSED |
| T-13-57 | 09 | T | med | mitigate | `watch:36` job budget 90 s against `holds:24` `StaleClaim` 2 min; `TestMemberReqRunnerSheds` (`memberreq_watch_test.go:668-672`) asserts the 30 s margin. The detached record writes (5 s) and the 30 s undelivered lift stay inside the remaining window | CLOSED |
| T-13-30 | 09 | D | high | mitigate | `watch:191-231` the named result is preset to `ext.ContinueGroups` (`:192`), the recover is deferred (`:193`), no path returns anything else; the only message handler of group -2 (`mod:50-53`; the other -2 handler, `bot_updates.go:275`, is `my_chat_member`). `TestMemberReqWatcherContinuesGroups`. A clean member costs no Telegram call and at most one pass-guarded statement bounded to 3 s (`watch:127-146`) | CLOSED |
| T-13-31 | 09 | D | high | mitigate | `watch:164-181` the job runs on its own goroutine under `startBelow(32)` (`:88-99`, check and increment in one step), sheds with one ID-only line (`:166`) and recovers from panics (`:175`); every Telegram call of the executor sits inside a `memberReqPaced` closure (`enf:147,442,452,580`, `fixed:225`, `join:153`, `notice:140`, `unm:22,35`). `TestMemberReqRunnerSheds`. The warn path's limit action is the warn module's own call (NEW-04, AR-16) | CLOSED |
| T-13-32 | 09 | T | high | mitigate | `holds:38-51`; `fuku/db/cache/local.go:49,77` puts `member_requirements:` in `skipLocal`; fix WR-02: the executor re-reads the settings after its live lookup (`enf:248-258`), so a rule switched Off is honoured by a job already in flight. `TestMemberReqOnePunishmentAcrossReplicas` (SQLite and PostgreSQL 16), `TestSkipLocalMemberRequirementsKey`, `TestMemberReqEnforceReadsSettingsAfterLookup` | CLOSED |
| T-13-33 | 09 | S | med | mitigate | `watch:201-206` + `raidFloodSenderCounts` (`raid_flood.go:199-213`: no `sender_chat`, automatic forward, non-positive or service ID, or the bot); the handler sets no `SetAllowChannel` or `SetAllowEdited`; the live record's `IsBot` is checked again (`enf:163`). `TestMemberReqWatcherExemptions` | CLOSED |
| T-13-34 | 09 | D | low | mitigate | `main.go:226` + `shutdown_drains.go:67-79` the drain waits `memberReqStopWait` (2 s) plus the 1 s grace, registered after DB-close and right before the staff drain; `watch:244-258` sets the draining flag, cancels the root context and waits. The 65 s worst case is in AGENTS.md; a cut costs no correctness (holds live in PostgreSQL). `TestShutdownWaitsForMemberReqJobs` | CLOSED |
| T-13-35 | 10 | D | high | mitigate | `lim:53-76` `memberReqCapAllow` (10 attempts per fixed minute per chat, counted before the claim at `enf:549`); `lim:81-105` the 10 minute latch and one notice. `TestMemberReqCapPausesPunishments`, `TestMemberReqCapWithoutRedis`. Accepted: no Redis means no brake (AR-04); the brake is also a lever for a raider (NEW-05, AR-10) | CLOSED |
| T-13-36 | 10 | T | med | mitigate | `enf:532-539` a paused chat still deletes the message of a sender the cached list knows as a non-admin and makes no Telegram call otherwise. `TestMemberReqDeleteDuringPause`. Accepted: an unknown-status sender's message is kept during a pause (AR-05) | CLOSED |
| T-13-59 | 10 | E | high | mitigate | `enf:590-630` `memberReqDeletes` and `memberReqSenderNonAdmin` (a known list without the sender, else one live lookup outside a pause that must read member or restricted, else no delete); the punishing path deletes only after the job's own live lookup (`enf:566`); a promotion invalidates the list (`adminCacheAutoUpdate`, `bot_updates.go:275`). `TestMemberReqDeleteMessages`, `TestMemberReqDeleteDuringPause` | CLOSED |
| T-13-37 | 10 | R | low | mitigate | `lim:88-95` only the caller whose `SET NX` won posts the notice. The two-replica subtest of `TestMemberReqCapPausesPunishments` | CLOSED |
| T-13-38 | 10 | I | low | mitigate | `enf:584` the delete-failure line holds the message, user and chat IDs and the error code only; asserted in `TestMemberReqDeleteMessages` | CLOSED |
| T-13-39 | 11 | E | high | mitigate | `join:138-158` `memberReqCaptchaGuard` is the first statement of `unmuteCaptchaUser` (`captcha.go:1633`, the one choke point: the other `RestrictChatMember` calls of `captcha.go` mute); a punished requirement mute is re-applied with the hold's own marker; a hold-read error returns `errMemberReqHoldUnreadable` (`join:34,147`), which `isPermanentUnmuteError` maps to false (`captcha.go:227-232`). `TestMemberReqCaptchaGuard` (SQLite and PostgreSQL 16). Residual: a pending mute hold is not guarded (NEW-03, AR-15) | CLOSED |
| T-13-58 | 11 | D | med | mitigate | `join:139-141` the guard returns before any read while `memberReqLive()` is false; afterwards a read error keeps the member muted until the `captcha_muted_users` retry (AR-06). `TestMemberReqCaptchaGuardOffUntilLive` | CLOSED |
| T-13-40 | 11 | T | med | mitigate | `join:71-121` the entry's only welcome is `SendWelcomeMessage`, and only for a clean, exempt, unanswerable or warned member; a kick, ban or mute returns nil and a punished mute hold records `welcome_held` (`:96-99`). `TestMemberReqJoinNoCaptcha`, `TestMemberReqJoinAndMessageOnce`. Residual races: AR-07 | CLOSED |
| T-13-41 | 11 | D | med | mitigate | `captcha.go:1747` the entry is the last step of `finishCaptchaPass` and `greetings.go:1089` of the join welcome, both after the claim; `join:73` reads the switch first. `TestMemberReqJoinAfterCaptcha`, `TestMemberReqJoinOffUntilLive` | CLOSED |
| T-13-42 | 11 | E | med | mitigate | `join:77` bots, the bot and service IDs are never checked; the executor's tri-state approval and live admin check (`enf:201-216,240-247`). `TestMemberReqJoinExemptions` | CLOSED |
| T-13-43 | 12 | E | high | mitigate | `join:225-238` `memberReqLiftOldMute` lifts only while `isMemberReqMute(live, hold.MuteUntil)` holds, and only with `memberReqUnmutePermissions`; fix WR-03: the reset now handles a pending mute hold too (`join:205`). `TestMemberReqRejoinResetsMuteHold`, `TestMemberReqRejoinResetsPendingMuteHold` (an admin's mute is never lifted) | CLOSED |
| T-13-44 | 12 | D | med | mitigate | `join:193-220` the reset runs at the rejoin's join (`membership.go:79-84` after the claim, `greetings.go:1092`) and puts the hold back with its ID on a failed lift; `leftMember` clears holds (`greetings.go:1005`). `TestMemberReqRejoinResetsMuteHold`. Live checks A3 and A4 are the owner's (AR-02) | CLOSED |
| T-13-45 | 12 | T | high | mitigate | `join:57-121` and `holds:213-226`: the reset deletes only holds created before the join date, and the entry and the watcher share one hold latch. `TestMemberReqJoinAndMessageOnce`, `TestMemberReqJoinResetOrder` | CLOSED |
| T-13-46 | 13 | S | high | mitigate | `fixed:90` `query.From.Id != userID` is the first check after the strict parse (`:83-88`), before the hold read (`:125`) and any Telegram call; an admin is refused too. `TestMemberReqFixedRefusals` ("another member and an admin are refused before any read": no `getChatMember`, with the hold removed so a read would have answered stale) | CLOSED |
| T-13-47 | 13 | E | high | mitigate | `fixed:134-141` the live record must satisfy `isMemberReqMute` (restricted, in the group, `until_date` within 2 s of the hold's marker, `enf:704-723`) before any write; a hold with no marker never matches (`enf:705`). `TestMemberReqFixedNeverLifts` (admin, staff, captcha, antiflood and warn-limit mutes) | CLOSED |
| T-13-48 | 13 | E | high | mitigate | `fixed:157-161` permissions come from `memberReqUnmutePermissions` before the hold is touched, and an error changes nothing; `:181-183` the lockdown wording. `TestMemberReqFixedDuringLockdown`. Live check A5 is the owner's (AR-02) | CLOSED (code) |
| T-13-49 | 13 | T | med | mitigate | `fixed:42-55` strict decode (exactly `a`, `h`, `u`; positive whole numbers of at most 19 digits through `settingsCardNumber`, `settings_card.go:27-41`; the message present and in a group or supergroup); `fixed:130` the hold is matched on its ID, the pressed message's chat and the member, then the live marker; a hold of another chat, of another member and a missing one all answer `mreq_fixed_stale`, so there is no oracle. `TestMemberReqFixedRefusals` | CLOSED |
| T-13-50 | 13 | T | med | mitigate | `fixed:192-203` `TakeHoldByID` is one `DELETE ... RETURNING` on ID, chat and user (`holds:133-149`) before the lift; `fixed:172-176` and `holds:155-164` `RestoreHold` puts it back with the same ID on a failed lift. `TestMemberReqFixedTwoReplicas`, `TestMemberReqFixedRestoreOnFailure` (both also on PostgreSQL 16) | CLOSED |
| T-13-51 | 13 | D | high | mitigate | as T-13-29: `mod:50-59` registration and default together; `TestMemberReqEnforcementLive` | CLOSED |
| T-13-52 | 14 | I | high | mitigate | `TestSettingsStaffPrivacy` renders every Phase 13 screen and a private non-creator run (passes); `TestMemberReqScreensFitEveryLocale` (`assertMemberReqNoLeak`: chat ID, "staff", the admin's name); `smreq` never reads the Staff Group | CLOSED |
| T-13-53 | 14 | T | med | mitigate | `fuku/db/backup/types.go:115` `AllExportableModules` has neither table and `ImportChatData` walks only valid modules (`backup.go:121-125`), so a planted key is ignored; nothing outside the `memberreq` packages and the module files references the two tables (grep). `TestMemberRequirementsStayOutOfBackup` (export, import and reset) | CLOSED |
| T-13-54 | 14 | D | med | mitigate | every answer measured at 200 code points in 8 languages (`assertMemberReqFixedAnswers` in `memberreq_screens_test.go`, `loc:184`) | CLOSED |
| T-13-55 | 14 | R | med | mitigate | AGENTS.md carries the Phase 13 bullets and was updated again in the fix pass; the `Makefile` integrity list holds `TestRequirementHolds`, `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent` and `./fuku/db/memberreq`; `.planning/REQUIREMENTS.md:154` says MREQ-01 is "five of the eight rules ... Pending". `make check-docs` and `make check-translations` clean at HEAD | CLOSED |
| T-13-SC | 01-14 | T | high | mitigate | `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0 at HEAD; no package was installed | CLOSED |

**Prohibitions in the plans:** all hold in the code. `memberreq*.go` references no `IsUserAdmin`, `IsUserApproved(` or
`aispamCarvedOut` (13-07); `MutedPermissions` is used only to punish and to re-mute in the captcha guard, never to lift
(13-08, 13-13); the watcher has no return other than `ext.ContinueGroups` and is alone in group -2 (13-09); the guard reads
nothing while the switch is off (13-11); no lift runs without the live marker except the undelivered-notice lift (NEW-02);
no migration edit (one new file, no `BEGIN`, `COMMIT`, `ADD CONSTRAINT` or `CONCURRENTLY`, a timestamp above every
existing file); no secret, config key or dependency was added (`fuku/config` untouched, `go.mod` and `go.sum` unchanged).

---

## Code Review Findings Verified in the Code (13-REVIEW.md, fix pass `a1820fd..9663ac8`)

| Finding | Fix evidence at HEAD | Status |
|---|---|---|
| CR-01 (critical): a requirement Mute replaced a restriction someone else placed | `enf:546` calls `memberReqMuteOverRestriction` after the live lookup and before the cap and the claim; `enf:685-700`: for a Mute on a `restricted` member with no live marker of this episode (`keepUntil == 0`) it reads the group defaults through `memberReqUnmutePermissions` (lockdown snapshot included), refuses when `memberReqRestrictedBelow` (`enf:652-673`, 13 rights) finds a right the group grants that the member lacks, and refuses when the defaults cannot be read (fail closed); a stale hold's own live marker is adopted (`enf:636-644`); Warn, Kick and Ban are untouched. The refusal creates no hold, no notice, no delete. `TestMemberReqMuteNeverReplacesRestriction` and `TestMemberReqJoinNeverReplacesRestriction` pass (the first also on PostgreSQL 16). The "below the defaults" rule instead of the literal "restricted and no marker" was reviewed: the literal rule would have stopped muting any member lifted to the defaults of a restrictive group; it rests on Telegram reading such a member back with the group's rights, which is live check 14 (AR-02) | FIXED, verified |
| WR-01: a recorded warn punished twice | `enf:319-333` three mark attempts on one detached 5 s context; `warned:36-93` the Redis note the reclaim adopts; `enf:353-365` a recorded-then-failed warn still marks and announces. `TestMemberReqWarnRecovery` (SQLite, PostgreSQL 16). Without Redis the reclaim warns again (AR-04) | FIXED, verified |
| WR-02: a job punished from a stale settings snapshot | `enf:248-258` the executor re-reads `memberReqSettingsLookup` after the live lookup, returns no violation on an error and stores the fresh row in `c.Settings`; the join entry goes through the same function. `TestMemberReqEnforceReadsSettingsAfterLookup`, `TestMemberReqJoinReadsSettingsAgain`. The held and paused delete paths still use the snapshot (NEW-06, AR-17) | FIXED, verified |
| WR-03: the rejoin reset skipped a pending mute hold's live marker | `join:205` tests `Action == Mute` instead of a punished mute; `memberReqLiftOldMute` still lifts only on the live marker. `TestMemberReqRejoinResetsPendingMuteHold` | FIXED, verified |

**Deferred review items** (`13-REVIEW-DISPOSITION.md`): none of IN-01 to IN-06 or the fix pass's NEW-01 to NEW-04 leaves an
open threat at or above the `high` gate, and none widens anyone's authority. Each is an accepted risk below and none is
dropped silently. Naming: `RV-` marks an item of the review or its disposition; plain `NEW-` is this audit's own finding.

| Item | Threat touched | Why it is not an open security threat | Accepted as |
|---|---|---|---|
| RV-IN-01 clean verdict cached though the 2 s margin skipped a hold | T-13-02 | needs a rename within seconds of the punishment; a kick or ban hold ends at the leave, a mute hold keeps the member muted, so the worst case is one skipped repeat Warn inside 30 minutes | AR-01 |
| RV-IN-02 undelivered mute still reports Muted, so the joiner is not welcomed | T-13-26, T-13-40 | needs the notice to fail twice; the member is unmuted, nobody is punished wrongly, a welcome is missed | AR-07 |
| RV-IN-03 join entry welcomes a member a concurrent job is muting | T-13-40 | a narrow join-versus-first-message race; the effect is a welcome (Greet once claim) sent while the mute lands, never a missed punishment | AR-07 |
| RV-IN-04 notice retry can duplicate a notice or retry a 4xx once | T-13-24, T-13-26 | a duplicate mute notice carries the same hold ID, and the button is idempotent (`TakeHoldByID`) | AR-08 |
| RV-IN-05 and RV-NEW-02 a cold-cache admin or a restricted violator costs one paced lookup per message | T-13-31 | cost only, shared with the fleet pacer; no punishment follows (an admin is exempt by the live record) | AR-09 |
| RV-IN-06 the cap counts attempts, not members | T-13-35 | the cap may trip early under a slow claim; it never lets more punishments through | AR-10 |
| RV-NEW-03 time of check to time of use between the live lookup and the restrict call | T-13-47 | a window of milliseconds inside one job; an admin's restriction placed in it is overwritten and the admin can re-apply it | AR-11 |
| RV-NEW-04 `ReclaimStaleHold` clears `notice_msg_id` | none | "I fixed it" edits the message the button sits on and never reads the column | AR-12 |

---

## New Threats Found by the Audit (unregistered)

The 14 plan summaries raise no unregistered threat flag (every `## Threat Flags` section is "None" or points at a planned
ID; the `mreq\|` callback surface named in 13-13 is T-13-46 to T-13-51). The review's findings are covered above. The audit
found six more items; none reaches the `high` gate.

| ID | Severity | Finding and scenario | Disposition |
|---|---|---|---|
| NEW-01 | low | **A bidi control inside a name is not stripped, so a name can close its own isolate.** `memberReqMention` (`rules:241-250`) wraps the name in U+2068 and U+2069 but keeps every character of it. A name such as `x` U+2069 U+202E `evil` ends the isolate early and opens a right-to-left override that, where the client honours it, reorders the rest of the notice line (the fixed words after the name). No markup, link or other member's data is involved (the HTML is escaped and the name is capped at 64 runes), so the effect is a garbled or misleading public notice about that member only | **Open, non-blocking** (below `high`). Suggested fix: drop U+061C, U+200E, U+200F, U+202A to U+202E and U+2066 to U+2069 from the name before it is wrapped, with a case in `TestMemberReqMention` |
| NEW-02 | low | **The undelivered-notice lift does not re-check the marker.** `memberReqUndeliveredMute` (`notice:183-201`) lifts the mute it just placed through `memberReqLiftMute` with no `isMemberReqMute` read. A mute notice that is rate limited waits up to 5 minutes (`notice:41,147-150`); an admin who restricts that member in the window has their restriction replaced by the lift. Needs a 429 streak and an admin acting on an already muted member; the admin can re-apply. Same class as RV-NEW-03 with a wider window | **Open, non-blocking** (below `high`). Suggested fix: one paced `getChatMember` and `isMemberReqMute` before the lift (as `memberReqLiftOldMute` does) |
| NEW-03 | info | **A pending mute hold is not guarded against a captcha unmute.** `memberReqPunishedMute` (`join:42-44`) needs `PunishedAt`, so when a mute took effect but the three mark attempts failed (`enf:319-333`) the guard does not re-apply it and a captcha pass can free the member once. The next violating message after `StaleClaim` reclaims the hold and mutes again; the member can only use the window while a database fault, a captcha attempt and the mute coincide | **Accepted** (AR-15) |
| NEW-04 | info | **The warn path's limit action is not paced or bounded by the job context.** `applyWarnLimitAction` (`warns:152-180`) calls `chat.RestrictMember` and `chat.BanMember` directly (and `kickMemberContext` with the job context, unpaced); AGENTS.md says every executor call goes through `memberReqPaced`, which is imprecise for this one. The HTTP client's 30 s timeout bounds each call and the per-chat cap bounds them to 10 a minute | **Accepted** (AR-16); the AGENTS.md sentence can say "except the warn limit action" |
| NEW-05 | info | **The brake is also a lever.** A raider with more than 10 violating accounts in one minute starts the 10 minute pause (`lim:71-76`), after which only the deletes of known non-admins go on (`enf:532-539`). Lockdown, antiflood and the raid triggers are untouched. This is the cost of bounding a mis-set rule (T-13-35) and the notice says it | **Accepted** (AR-10) |
| NEW-06 | info | **Held and paused deletes read the watcher's snapshot.** WR-02 re-reads settings only after the live lookup, so a Delete messages switch turned off while a job waits can still delete one more message of a held member from that job (at most 90 s). The sender is still established as a non-admin (T-13-59) | **Accepted** (AR-17) |

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale |
|---------|------------|-----------|
| AR-01 | T-13-02, RV-IN-01 | A clean verdict cached for 30 minutes can outlive a hold the 2 s margin skipped; the worst case is one skipped repeat Warn |
| AR-02 | T-13-27, T-13-44, T-13-48, CR-01 | The live Telegram checks (A2 marker read-back, A3 and A4 leave and rejoin, A5 lockdown unmute, A8 callback lifetime, 14 restricted members) are the owner's and were deferred as in Phases 7 to 12 (13-SPIKE.md); Mute should not be enabled in a real group before A2 to A5 and 14 are run. Warn, Kick and Ban do not depend on them |
| AR-03 | T-13-26, T-13-44 | Two Telegram refusals in a row leave a mute with no button (an admin's `/unmute` or the rejoin ends it); a crash between the rejoin reset's delete and its lift can strand a mute with no hold (13-SPIKE accepted limits) |
| AR-04 | T-13-21, T-13-35 | The cap, the pause latch and the warned note are Redis only: with no Redis or a Redis error the punishment is allowed, nothing is paused and a reclaimed warn hold whose mark failed warns again (AGENTS.md, like the raid counters) |
| AR-05 | T-13-36 | An unknown-status violator's message is kept during a pause: the cached list cannot say and a pause forbids the live lookup |
| AR-06 | T-13-58 | A database fault delays a captcha unmute until the `captcha_muted_users` retry that can read, rather than freeing a member the requirements muted |
| AR-07 | T-13-40, RV-IN-02, RV-IN-03 | Two narrow welcome races (an undeliverable mute, a join against a first message); a welcome is sent or missed, nobody is wrongly punished or freed |
| AR-08 | T-13-24, T-13-26, RV-IN-04 | A notice may be posted twice after a timed-out send and a 4xx is retried once; both buttons carry one hold ID and the press is idempotent |
| AR-09 | T-13-31, RV-IN-05, RV-NEW-02 | A cold-cache admin or a restricted violator costs one paced lookup per message until a verdict is cached; shed first under load, never punishes |
| AR-10 | T-13-35, RV-IN-06, NEW-05 | The cap counts attempts before the claim and a raider can trip the pause; the price of bounding a mis-set rule |
| AR-11 | T-13-47, RV-NEW-03 | A time-of-check to time-of-use window of milliseconds between a job's live lookup and its restrict call |
| AR-12 | RV-NEW-04 | A reclaimed hold no longer records the first notice's message ID; nothing reads it |
| AR-13 | T-13-05, T-13-07 | The rules are letter based: a Latin ë evades the Russian rule, Belarusian, Bulgarian, Persian, Urdu and Japanese kanji names can be caught, and Unicode newer than the toolchain's tables is not seen; the page says so |
| AR-14 | T-13-39, T-13-43 | During a rolling deploy an old replica has no watcher, no button handler, no captcha guard and no join reset (an old replica can unmute a held member on the rare race, and a muted rejoiner on it stays muted until the button is pressed on a new replica); the migration runs before any replica serves |
| AR-15 | NEW-03 | A pending mute hold with a live marker is not re-applied by the captcha guard; the reclaim mutes again |
| AR-16 | NEW-04 | The warn limit action is the warn module's own call, bounded by the client timeout and the cap |
| AR-17 | NEW-06 | Held and paused deletes read the watcher's settings snapshot |
| AR-18 | T-13-31 | At most 32 enforcement jobs run per replica; a full runner sheds a job with one log line and the violator's next message is checked again (13-SPIKE accepted limit) |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open (blocking) | Open (non-blocking) | Run By |
|------------|---------------|--------|-----------------|---------------------|--------|
| 2026-11-08 | 59 distinct IDs (72 plan rows) | 59 (T-13-27 and T-13-48 closed in code, with live check A5 the owner's) | 0 | 2 (NEW-01 and NEW-02, both low, both unregistered) | gsd-security-auditor |

Tests run at `c048369`, all passing with no failure and no skip: the whole `^(TestMemberReq|TestSettingsMreq|TestApplyMemberReq|
TestSettingsDriftMreq|TestSettingsAuthorityMreq|TestSettingsControlActionAllowList|TestSettingsStaffPrivacy|
TestShutdownWaitsForMemberReqJobs|TestApplyWarn)` family in `./fuku/modules` with `-race` (74 top-level tests, 79 s);
`./fuku/db/memberreq` (`TestRequirementHolds`, `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent`),
`./fuku/db/backup` (`TestMemberRequirementsStayOutOfBackup`), `./fuku/db/cache` (`TestSkipLocalMemberRequirementsKey`),
`./fuku/i18n` (`TestMemberReqLocaleKeys`, `TestSettingsLocaleKeys`, `TestSettingsLabelLengths`) and `./fuku/db/migrations`
(`TestMemberRequirementsMigrationStatements`). On a throwaway PostgreSQL 16 cluster (removed afterwards):
`TestRepositoryMigrationChain` (59 migrations applied, the Phase 13 file included), the three `./fuku/db/memberreq`
tests, and in `./fuku/modules` `TestMemberReqOnePunishmentAcrossReplicas`, `TestMemberReqFixedTwoReplicas`,
`TestMemberReqFixedRestoreOnFailure`, `TestMemberReqFixedRefusals`, `TestMemberReqCaptchaGuard`,
`TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqWarnRecovery`, `TestMemberReqEnforceWarnOnce`; the named
constraints of both tables were read from `pg_constraint` and three out-of-range inserts were refused. `make check-docs`
(no drift), `make check-translations` (all present), `CGO_ENABLED=0 go build ./...` (ok), and a standalone timing probe of
the name rules on 8 MB hostile names (combining marks, zero-width characters, bidi controls, decomposed ё, invalid UTF-8):
each ran in 0.2 to 0.4 s, linear (no regular expression is used; Telegram caps a name at 64 characters). No source or
test file was changed. Not run: `make test`, `make lint`, the PostgreSQL integrity target as a whole and the live Telegram
checks (13-SPIKE.md), which are the owner's.

**threats_open:** 0
