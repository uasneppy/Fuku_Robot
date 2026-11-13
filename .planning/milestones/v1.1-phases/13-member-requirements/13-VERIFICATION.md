---
phase: 13-member-requirements
verified: 2026-11-08T02:10:00Z
status: human_needed
score: "17/17 must-haves verified (7/7 truths, the 4 ROADMAP success criteria plus 3 supporting truths; 10/10 requirements judged on Phase 13 scope, MREQ-01 on five of eight rules)"
covered_files:
  - ".github/workflows/ci.yml"
  - ".planning/phases/13-member-requirements/13-01-PLAN.md"
  - ".planning/phases/13-member-requirements/13-01-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-02-PLAN.md"
  - ".planning/phases/13-member-requirements/13-02-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-03-PLAN.md"
  - ".planning/phases/13-member-requirements/13-03-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-04-PLAN.md"
  - ".planning/phases/13-member-requirements/13-04-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-05-PLAN.md"
  - ".planning/phases/13-member-requirements/13-05-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-06-PLAN.md"
  - ".planning/phases/13-member-requirements/13-06-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-07-PLAN.md"
  - ".planning/phases/13-member-requirements/13-07-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-08-PLAN.md"
  - ".planning/phases/13-member-requirements/13-08-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-09-PLAN.md"
  - ".planning/phases/13-member-requirements/13-09-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-10-PLAN.md"
  - ".planning/phases/13-member-requirements/13-10-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-11-PLAN.md"
  - ".planning/phases/13-member-requirements/13-11-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-12-PLAN.md"
  - ".planning/phases/13-member-requirements/13-12-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-13-PLAN.md"
  - ".planning/phases/13-member-requirements/13-13-SUMMARY.md"
  - ".planning/phases/13-member-requirements/13-14-PLAN.md"
  - ".planning/phases/13-member-requirements/13-14-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "docs/src/content/docs/commands/settings/index.md"
  - "fuku/db/approvals/repository.go"
  - "fuku/db/cache/local.go"
  - "fuku/db/memberreq/holds.go"
  - "fuku/db/memberreq/settings.go"
  - "fuku/db/models/member_requirements.go"
  - "fuku/modules/bans.go"
  - "fuku/modules/captcha.go"
  - "fuku/modules/chat_permissions.go"
  - "fuku/modules/greetings.go"
  - "fuku/modules/memberreq.go"
  - "fuku/modules/memberreq_enforce.go"
  - "fuku/modules/memberreq_fixed.go"
  - "fuku/modules/memberreq_join.go"
  - "fuku/modules/memberreq_limits.go"
  - "fuku/modules/memberreq_notice.go"
  - "fuku/modules/memberreq_rules.go"
  - "fuku/modules/memberreq_settings.go"
  - "fuku/modules/memberreq_unmute.go"
  - "fuku/modules/memberreq_warned.go"
  - "fuku/modules/memberreq_watch.go"
  - "fuku/modules/membership.go"
  - "fuku/modules/settings_menu.go"
  - "fuku/modules/settings_mreq.go"
  - "fuku/modules/settings_pages.go"
  - "fuku/modules/shutdown_drains.go"
  - "fuku/modules/warns.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "main.go"
  - "migrations/20261106120000_add_member_requirements.sql"
covered_digest: "v4:sha256:398dd3515047a05960d606e94cf2c583fddcdc51c22e0b90b53e451084aa4d9c"
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "13-SPIKE.md checks 2, 3, 4, 5, 8 and 14 on a test group, BEFORE any group sets a rule to Mute. Check 2: place a requirement mute and read the member back. Checks 3 and 4: a muted member leaves and rejoins. Check 5: unmute during a lockdown. Check 8: how long a callback stays answerable. Check 14: a member lifted to a restrictive group's defaults reads as restricted but below nothing, so a Mute still applies, while an admin-restricted member is refused."
    expected: "The 330-day marker until_date reads back within 2 s. A muted member's leave and rejoin arrive as restricted-to-restricted updates and the rejoin reset lifts only the marker mute. A member unmuted in a lockdown still cannot send. The button answers inside 10 s. Check 14 behaves as memberReqRestrictedBelow assumes."
    why_human: "The tests prove every branch against the hand-written Bot fake (TestMemberReqFixedUnmutes, TestMemberReqRejoinResetsMuteHold, TestMemberReqFixedDuringLockdown, TestMemberReqMuteNeverReplacesRestriction). What Telegram really reports for a restricted member and how it echoes until_date is live Bot API behaviour. Warn, Kick and Ban do not depend on it."
  - test: "13-SPIKE.md check 1 (A1) and check 6: the group's message limit and the cap. Check 6: an account whose only username is collectible. Check 7: names are current right after a rename."
    expected: "The cap of 10 punishments a minute and the 10-minute pause fit the group's real limits. A collectible-username account reads as having a username only as the rule table expects. A renamed member is judged on the new name at the next message (the live getChatMember is the source)."
    why_human: "These depend on live Bot API data the Bot fake cannot produce."
  - test: "13-SPIKE.md check 9 (bidi isolates with an Arabic name) and check 10 (the seven cells and the choice rows in hi, ru and uk on phones)."
    expected: "A right-to-left name cannot reorder the notice line (memberReqStripBidi plus the U+2068/U+2069 isolates). No cell or choice row is cut off with an ellipsis on a phone, Hindi combining marks included."
    why_human: "TestMemberReqMention and TestSettingsMreqPageFitsEveryLocale hold the code point and byte limits. Bidi rendering and truncation at phone width are client behaviour."
  - test: "13-SPIKE.md check 11: a native speaker of each of es, fr, hi, id, pt, ru and uk reads the 25 mreq_ keys and the 35 settings_mreq / settings_btn_mreq / settings_page_mreq keys on the real page, notices and answers."
    expected: "Each string reads naturally. The sentences that say the name rules go by letters and can catch related languages are understood. 'Check on join', 'Delete messages' and 'I fixed it' are understood."
    why_human: "Translation quality is a human judgement. make check-translations and TestMemberReqScreensFitEveryLocale only prove presence and size."
  - test: "13-SPIKE.md check 12 (two live replicas over one PostgreSQL): a violator posts ten messages through two replicas; a muted member taps I fixed it on both."
    expected: "One punishment, one notice, one unmute, and the held welcome is sent once."
    why_human: "TestMemberReqOnePunishmentAcrossReplicas (three bot processes over one database and Redis) and TestMemberReqFixedTwoReplicas prove it on SQLite and miniredis here, and TestRequirementHoldClaimConcurrent proves the claim. Two real processes with a deployment config can only be checked live."
  - test: "13-SPIKE.md check 13 and CI on HEAD 2d85418: make lint with a golangci-lint built for Go 1.26, then make test and make test-postgres-integrity."
    expected: "make lint is clean for the new lines. make test is green. The integrity step runs TestChatRequirementsSettings, TestRequirementHoldClaimConcurrent and TestRequirementHolds for real on PostgreSQL (they are in the Makefile list)."
    why_human: "The verifier did not run make test (fuku/modules must not be run whole here; two full runs and a CI run passed on the code at HEAD minus docs, per the orchestrator) and did not run make lint or the PostgreSQL target (no PostgreSQL server here, and the sandbox golangci-lint is built for an older Go). The verifier ran the focused families instead."
  - test: "Owner review of the judgment-tier prohibitions of plans 13-01 to 13-14 (see 'Prohibitions' below) and of the follow-ups and deferred items: UI-SPEC A1, A4, A8, A14, A15, A17, A18, the deferred review items IN-01 to IN-06, NEW-02 to NEW-04 and the deferred security items SEC-NEW-03 to SEC-NEW-06 (AR-10, AR-15, AR-16, AR-17)."
    expected: "The owner accepts the verifier's non-authoritative verdict (every prohibition holds, each backed by a passing test or a direct code read) and decides which follow-ups become their own changes."
    why_human: "An autonomous verifier never passes a judgment-tier prohibition silently. The follow-ups are product decisions the phase left open on purpose."
audit_acknowledged:
  milestone: v1.1
  at: 2026-11-13
  status: human_needed
---

# Phase 13: Member Requirements Verification Report

**Phase Goal:** As a group owner, I want members with no last name or username, or with names in a script I block, warned, removed or muted automatically, and a muted member able to free themselves once they fix it, so that the group screens people when no admin is online.
**Mode:** mvp (the user story format is valid: "As a group owner, I want ..., so that ...")
**Verified:** 2026-11-08T02:10:00Z at HEAD 2d85418 (branch claude/ecstatic-goldberg-qsss05, clean tree)
**Status:** human_needed
**Re-verification:** No, initial verification

All four ROADMAP success criteria and all ten requirements are delivered and wired end to end in the code: migration, models, repository, message watcher, join entry, captcha guard, rejoin reset, executor, "I fixed it", settings page, shutdown drain, eight locale files and docs. Every behaviour-dependent truth has a passing test that drives the real handlers through the hand-written Bot fake, miniredis and SQLite, and the verifier ran those tests itself (see Behavioral Spot-Checks). There are no gaps. The status is `human_needed` only because the live checks the owner chose to defer (Phases 10 to 13: "Defer live checks, keep going") remain, CI has not run on HEAD, and the owner still has to review the judgment-tier prohibitions and the deferred follow-ups.

MREQ-01 is deliberately delivered as five of the eight rules in this phase (last name, username, Russian, Arabic, Chinese). It is judged here on those five and it **stays Pending** in the tracker (REQUIREMENTS.md lines 38 and 154) until MREQ-11, MREQ-12 and MREQ-14 deliver the profile-photo, channel-subscription and AI spam-name rules in Phases 14 and 15.

## User Flow Coverage

User story: «As a group owner, I want members with no last name or username, or with names in a script I block, warned, removed or muted automatically, and a muted member able to free themselves once they fix it, so that the group screens people when no admin is online.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Owner opens Member requirements from /settings Main | Page `m` is a Main grid entry between Rules and Access, everything Off on a chat with no row | `registerSettingsPage` page `m`, `Order` 80 (`settings_mreq.go:53-55`); read through `memberReqSettingsLookup` = `memberreq.GetSettings` (`memberreq_settings.go:14`), which creates no row (`settings.go:106-119`). Tests: `TestSettingsMreqOpensFromMain`, `TestSettingsMreqMainState` | VERIFIED |
| Owner picks a punishment per rule | Off and Warn write at once; Kick, Ban and Mute draw a Yes/No card and write nothing until Yes | `settingsMreqChoose`, `renderSettingsMreqCard`, `settingsMreqYes` (`settings_mreq.go:361-447`), `applyMemberReqAction` (`memberreq_settings.go:88-104`). Tests: `TestSettingsMreqCard`, `TestSettingsMreqRuleScreen`, `TestApplyMemberReqAction` | VERIFIED |
| Owner turns on Check on join and Delete messages | Two switches, column-targeted writes, a Go `false` written | controls `j` and `d` (`settings_mreq.go:43-44`), `memberreq.SetCheckOnJoin` / `SetDeleteMessages` (`settings.go:156-163`). Test: `TestSettingsMreqSwitches` | VERIFIED |
| A violating member posts a message | Punished once, one notice naming them and the rule, their message deleted when Delete messages is on | watcher `memberreq_watch.go:191-231`, executor `memberreq_enforce.go:519-568`. Tests: `TestMemberReqWatcherPunishesOnce`, `TestMemberReqEnforceStrictest`, `TestMemberReqDeleteMessages` | VERIFIED |
| The same member posts nine more messages, through three replicas | No more warns, notices or punishments | insert-first hold `memberreq.ClaimHold` (`holds.go:38`) behind `uk_chat_requirement_holds_chat_user`. Tests: `TestMemberReqOnePunishmentAcrossReplicas`, `TestRequirementHoldClaimConcurrent` | VERIFIED |
| A muted member fixes their profile and taps I fixed it | Re-checked live, unmuted (lockdown respected), held welcome sent, or told what is missing | `memberReqFixedRun` (`memberreq_fixed.go:119-190`). Tests: `TestMemberReqFixedUnmutes`, `TestMemberReqFixedRefusals`, `TestMemberReqFixedDuringLockdown`, `TestMemberReqFixedHeldWelcome` | VERIFIED |
| Outcome: the group screens people with no admin online | Watcher, join entry and button run in every group with a rule on; every rule defaults Off | `memberReqEnforcementLive` true by default (`memberreq.go:57-61`), all rule columns `DEFAULT 0` (migration lines 23-29) | VERIFIED (live Telegram behaviour deferred, see Human Verification) |

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | **SC1 / MREQ-01 (five rules), MREQ-13:** A Member requirements page gives each rule its own punishment (off, warn, kick, ban or mute), with "Check on join" and "Delete messages" switches, everything off by default. The rules are last name, username, no Russian, no Arabic, no Chinese name. The page says the name rules go by letters and can catch related languages, and an invisible or whitespace-only last name does not count | VERIFIED | **Storage:** `chat_requirements` has five `SMALLINT` action columns, each `DEFAULT 0` with its own named CHECK 0..4, plus `check_on_join` and `delete_messages` `DEFAULT FALSE` (migration lines 20-38). Model has no `default:` tag on its booleans (`models/member_requirements.go:28-41`). A chat with no row reads every rule off (`settings.go:112-114`). **Rule table:** `memberReqRules` holds exactly `ln un ru ar zh` (`memberreq_rules.go:93-104`). Last name is `memberReqBlank(n.Last)`, true unless some rune is a visible letter, number, punctuation or symbol that is not default-ignorable and not U+2800 (`:122-136`). Russian is ы ъ э ё either case after NFC (`:48-66`), Arabic is an Arabic-script letter (`:70-77`), Chinese is any Han code point (`:81-88`), all on first and last name and never the username. **Writes:** `SetRuleAction` refuses an unknown rule or a value outside 0..4 before any write, writes only its own column plus `updated_at` through `ON CONFLICT (chat_id) DO UPDATE`, then `cache.DeleteCache` (`settings.go:121-153`). The key `member_requirements:*` is in `skipLocal` (`fuku/db/cache/local.go:77`). **Page:** `settings_mreq.go`, registered from its own file so `settings_pages.go` is not edited. Locale text: `settings_mreq_hint_letters` ("go by letters ... can also catch related languages, such as Belarusian, Bulgarian, Persian or Japanese names") and `settings_mreq_hint_blank` ("A last name made only of invisible or blank characters counts as no last name"), `locales/en.yml:3240-3241`, present in all 8 files. Tests pass: `TestMemberReqRulesCorpus` (table-driven: Ukrainian, Belarusian, Bulgarian, Russian, mixed script, zero-width, kana, Hangul, Han extension B, accepted Latin-ë evasion), `TestMemberReqBlankLastName` (every invisible code point of the research table), `TestMemberReqUsernameRule`, `TestMemberReqRuleTable`, `TestSettingsMreqOpensFromMain`, `TestSettingsMreqMainState`, `TestSettingsMreqRuleScreen`, `TestSettingsMreqCard`, `TestSettingsMreqSwitches`, `TestApplyMemberReqAction`, `TestChatRequirementsSettings`. |
| 2 | **SC2 / MREQ-02, MREQ-09:** With Check on join on, a new member is checked right after the captcha, or on joining when the group has none, and every member is checked when they send a message. Admins, approved users, bots, Telegram service accounts and messages sent as a channel or by an anonymous admin are never checked | VERIFIED | **Messages:** `memberReqWatcher` at handler group -2, the only message handler of that group, supergroup-only filter (`memberreq.go:49-52`). Order cheapest first: switch, supergroup, item (`raidFloodItemKey`), sender shape (`raidFloodSenderCounts`: no `sender_chat`, automatic forward, service account or the bot), cached settings, pure name rules, cached admin list (`memberreq_watch.go:191-231`). Returns `ext.ContinueGroups` on every path, named result preset so a recovered panic cannot return nil (`:191-193`). **Join:** `memberReqJoinThenWelcome` is the one entry of both welcome paths (`memberreq_join.go:71-121`): the `chat_member` join and the join service message through `membershipDepsForJoin` (`greetings.go:1089`) and the captcha pass through `finishCaptchaPass` (`captcha.go:1747`), so the captcha always runs first. With Check on join off the member is welcomed with no further read (`:100-102`). **Exemptions:** `memberReqNeverChecked` (bot itself, bots, `staffServiceUserIDs`, non-positive ID, `memberreq_enforce.go:176-178`), `memberReqExempt` (approved through the tri-state `approvals.IsUserApprovedContext`, pending captcha; an unreadable state is exempt, `:201-216`), and the live `getChatMember` that rejects creator, administrator, left, kicked, non-member restricted and bots (`memberReqLiveEligible`, `:162-173`). Tests pass: `TestMemberReqWatcherExemptions`, `TestMemberReqEnforceExemptions`, `TestMemberReqJoinNoCaptcha`, `TestMemberReqJoinAfterCaptcha`, `TestMemberReqJoinExemptions`, `TestMemberReqJoinAndMessageOnce`, `TestMemberReqWatcherContinuesGroups`. |
| 3 | **SC3 / MREQ-03, MREQ-04, MREQ-05, MREQ-08:** A violation is punished once and posts one notice naming the member and the rule. Further messages add no warns, notices or punishments until they fix it or rejoin, across replicas. A warn counts toward the group's warn limit. Delete messages removes the violator's messages. A kicked member can rejoin and is checked again, a banned member stays banned | VERIFIED | **Once across replicas:** `memberReqEnforce` claims the hold before any punishment, with one `INSERT ... ON CONFLICT DO NOTHING`, and only the caller whose insert landed acts (`memberreq_enforce.go:266-297,553-563`, `holds.go:38`); a hold that is not a stale pending one is the held path (`:221-230,528-531`). The clean predicate ends a fixed member's old episode (`memberReqClean`, `memberreq_watch.go:127-146`). **One notice, strictest wins, every broken rule named:** `memberReqStrictest` ranks ban over kick over mute over warn by `memberReqSeverity`, not by the stored values (`memberreq_rules.go:160-192`); `memberReqNoticeText` names the member by a mention spliced in after translation and every broken rule in rule order (`memberreq_notice.go:60-112`). **Warn limit:** a warn goes through the warn module's own `applyWarn` with the hold marked punished in its `afterRecord` hook inside the warn lock, so it counts toward the limit and its action (`memberReqWarnStep`, `memberreq_enforce.go:342-370`; a warn whose limit action kicked or banned is a removal). **Delete messages:** `memberReqDeleteMessage` after the punishment whatever became of it, `memberReqDeleteHeld` for a held member, deleted only when the sender is established as a non-admin (`memberReqSenderNonAdmin`, `:575-630`). **Kick, ban:** kick is `kickMemberContext` (member-removing unban), ban a plain `banChatMember` with no end date (`:437-444`); `leftMember` calls `memberreq.ClearHolds` (`greetings.go:1005`), so a kicked member who rejoins is checked again; a ban is never lifted by any requirement code path (grep: no `UnbanChatMember` outside `kickMemberContext`). **Brakes:** per-chat cap of 10 punishments a minute counted before the claim, then a 10-minute pause with one notice (`memberreq_limits.go`, `memberReqCapAllow` at `memberreq_enforce.go:549`), at most 32 jobs in flight (`memberReqRunAsync`, `memberreq_watch.go:164-181`). Tests pass: `TestMemberReqWatcherPunishesOnce`, `TestMemberReqOnePunishmentAcrossReplicas` (three replicas, ten messages), `TestMemberReqEnforceWarnOnce`, `TestMemberReqWarnMarkInsideLock`, `TestMemberReqEnforceStrictest`, `TestMemberReqNoticeWarnLimit`, `TestMemberReqDeleteMessages`, `TestMemberReqKickBan`, `TestMemberReqCapPausesPunishments`, `TestMemberReqCleanEndsWarnHold`, `TestMemberReqRunnerSheds`, `TestRequirementHolds`, `TestRequirementHoldClaimConcurrent`. |
| 4 | **SC4 / MREQ-06, MREQ-07:** A muted member gets an "I fixed it" button only they can use. The bot re-checks them live. If fixed, they are unmuted (respecting an active lockdown) and get the welcome held back. If not, they are told what is still missing. The button never lifts a mute placed by an admin, a staff action, the captcha, antiflood or a lockdown | VERIFIED (live marker behaviour deferred) | **Button:** only a mute notice carries it (`memberReqNoticeKeyboard`), data `mreq|v1|a=f&h=<hold>&u=<member>`, at most 52 bytes (`memberReqParseFixed`, `memberreq_fixed.go:42-55`, `TestMemberReqFixedCallbackBytes`, `TestMemberReqFixedButtonBytes`). **Pipeline in the contract's order** (`memberreq_fixed.go:59-190`): strict decode; the presser must be `u`, before any read, an admin is refused too (`:90-93`); the hold read fresh and matched by ID, chat and member (`:125-132`); one live `getChatMember` that must show the requirement marker `isMemberReqMute` (`:134-141`, `memberreq_enforce.go:721-723`: restricted, still a member, `until_date` within 2 s of the hold's `mute_until`, so an admin's, staff, captcha, antiflood or warn-limit mute, which write an end date of 0 or one a person chose, is never ours); the names judged again on the live record against the current settings, naming what is still broken (`:145-155`, `mreq_fixed_still_broken`); permissions only from `resolveUnmutePermissionsWithLockdown` (`memberReqUnmutePermissions`, `:157-161`, so an active lockdown's stored permissions apply and an error changes nothing); the conditional delete `TakeHoldByID` so one of two taps wins (`:163-170`); the lift with those permissions and `RestoreHold` with the same ID on failure (`:172-176`); the notice edited with no keyboard (`:184`); the held welcome sent once through `SendWelcomeMessage` only for a join hold, after the query was answered (`:185-188,99-101`), so Greet once claims the member only then. The callback recovers from a panic and answers exactly once (`:60,76`). **Held welcome (D-02):** a joiner whose Mute took effect is not welcomed and the hold stores `welcome_held` (`memberreq_enforce.go:273`, `memberreq_join.go:111-118`). **Captcha never frees a held member (D-08):** `memberReqCaptchaGuard` is the first statement of `unmuteCaptchaUser` (`captcha.go:1633`, `memberreq_join.go:138-158`), re-applies the hold's own marker, and a hold-read error fails closed. **Rejoin:** `memberReqJoinReset` takes only older holds and lifts only while the live record carries the marker (`memberreq_join.go:193-238`, `MembershipDeps.Reset` at `greetings.go:1092`). **A Mute never replaces a restriction someone else placed (CR-01):** `memberReqMuteOverRestriction` (`memberreq_enforce.go:685-700`). Tests pass: `TestMemberReqFixedUnmutes`, `TestMemberReqFixedRefusals`, `TestMemberReqFixedNeverLifts`, `TestMemberReqFixedDuringLockdown`, `TestMemberReqFixedRestoreOnFailure`, `TestMemberReqFixedTwoReplicas`, `TestMemberReqFixedHeldWelcome`, `TestMemberReqCaptchaGuard`, `TestMemberReqCaptchaGuardOffUntilLive`, `TestMemberReqRejoinResetsMuteHold`, `TestMemberReqMuteNeverReplacesRestriction`, `TestResolveUnmutePermissionsWithLockdown`. |
| 5 | **Supporting: fail safe.** A check that cannot be answered punishes nobody (an unreadable setting, approvals, captcha state, hold or live record), and a punishment Telegram refuses or leaves unknown never posts a notice for something that did not happen | VERIFIED | Every unreadable state returns `memberReqNone`: approvals and captcha (`memberreq_enforce.go:201-216`), hold (`:523-527`), live lookup (`:240-244`), settings read again after the live lookup (`:248-253`, WR-02). A definite refusal (4xx other than 429 by code, or `ErrRateLimited`) backs the hold off with no notice; an unknown answer of a mute is checked against the marker; a kick or ban with an unknown answer stays pending with no notice (`memberReqSendFailed`, `:398-416`). A mute whose notice cannot be delivered is lifted again, only while the marker still holds (`memberReqUndeliveredMute`). The watcher never caches a violation (`memberreq_watch.go:221-229`). Tests pass: `TestMemberReqPunishRefused`, `TestMemberReqPunishUnknown`, `TestMemberReqEnforceReadsSettingsAfterLookup`, `TestMemberReqWarnRecovery`, `TestMemberReqMuteNoticeRetry`, `TestMemberReqMuteNoticeUndeliverable`, `TestMemberReqUndeliveredMuteChecksMarker`, `TestMemberReqCapWithoutRedis` (all run by the verifier, second batch below). |
| 6 | **Supporting: wiring and lifecycle.** Migration, models, repository, module registration, shutdown drain and the cache and backup rules are in place and agree | VERIFIED | Migration `20261106120000` is later than `20261105120000`, idempotent, no BEGIN, COMMIT or CONCURRENTLY, no foreign keys (`TestMemberRequirementsMigrationStatements`). Models carry `TableName()` (`member_requirements.go:43,65`). Module `MemberRequirements` priority 241 registered in `init()` (`memberreq.go:57-62`); `LoadMemberRequirements` adds the watcher at group -2 and the `mreq|` callback (`:49-53`). `RegisterMemberReqDrain` is registered in `main.go:226` right before the staff drain, so LIFO runs it after the staff drain and before the admin index sweeper stop and the DB close, with `memberReqStopWait` (2 s) plus grace (`shutdown_drains.go:73-79`), cancelling the jobs' root context first (`memberreq_watch.go:244-258`). The fire-and-forget job goroutine starts with `error_handling.RecoverFromPanic` and is counted before the draining flag is read (`:173-180`). Every write of settings drops the cache key (`settings.go:136`); holds are fresh reads, never cached. Neither table is in backup, export, import or reset (`TestMemberRequirementsStayOutOfBackup`), and both models are in the harness AutoMigrate lists. The three Phase 13 PostgreSQL guarantees are in the Makefile integrity list (`Makefile:41`: `TestRequirementHolds`, `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent`, package `./fuku/db/memberreq` at `:44`). No new dependency (`git diff 72a692b -- go.mod go.sum` empty). Tests pass: `TestShutdownWaitsForMemberReqJobs`, `TestMemberReqEnforcementLive`, `TestMemberRequirementsStayOutOfBackup`, `TestMemberRequirementsMigrationStatements`. |
| 7 | **Supporting: locale, size limits, privacy, docs.** Every string exists in all 8 locales, every screen fits 3800 UTF-16 units and every button 64 bytes, no screen names the Staff Group, a member or a held count, and the docs and rulebook say what the code does | VERIFIED | `mreq_` has 25 keys and `settings_mreq` / `settings_btn_mreq` / `settings_page_mreq` 35 keys in each of en, es, fr, hi, id, pt, ru, uk. Every `memberReqText` key, every `PhraseKey` and every `settings_`, `common_` and `chat_status_` key named in the Phase 13 production files exists in all 8 files (script check, 0 missing; `mreq_notice_warn_limit_` is a prefix completed by `kick`, `ban`, `mute`, all three present). `make check-translations` passes. Tests pass: `TestMemberReqScreensFitEveryLocale`, `TestSettingsMreqPageFitsEveryLocale`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqRowBudget`, and the full `./fuku/i18n` package (includes `memberreq_locale_test.go`, label lengths, the settings key table). Docs: `callbacks.md:57,194-196,275`, `permissions.md:439-441`, `README.md:386`, `docs/src/content/docs/commands/settings/index.md`; `make check-docs` reports no drift. AGENTS.md carries the module, watcher group, drain, Redis key families, tables and traps (`StopMemberReqJobs`, `fuku:memberreq:*`, `chat_requirements`, `chat_requirement_holds`). |

**Score:** 7/7 truths verified (0 present but behavior-unverified). Requirements: 10/10 satisfied on Phase 13's scope (MREQ-01 on five of eight rules, stays Pending). Total 17/17.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261106120000_add_member_requirements.sql` | two tables, named CHECKs, unique keys | VERIFIED | Read in full (55 lines) |
| `fuku/db/models/member_requirements.go` | `ChatRequirements`, `ChatRequirementHold`, action constants, no `default:` on booleans | VERIFIED | Read in full |
| `fuku/db/memberreq/settings.go`, `holds.go` | cached settings, column-targeted writes, 14 hold operations | VERIFIED | Every exported function has a production caller (grep: ClaimHold 1, ReclaimStaleHold 1, MarkHoldPunished 1, SetHoldNotice 1, SetHoldWelcomeHeld 1, BackoffHold 1, RevertHoldToPending 1, TakeHoldByID 1, RestoreHold 3, ClearHoldOnClean 1, ClearHolds 1, TakeHoldsBefore 2, GetHoldFresh 2, GetHoldByIDFresh 1, GetSettings 1, GetSettingsFresh 2, SetRuleAction 1, SetCheckOnJoin 1, SetDeleteMessages 1) |
| `fuku/db/approvals/repository.go` | tri-state `IsUserApprovedContext` | VERIFIED | Same cached list as the old reader, error returned |
| `fuku/db/cache/local.go` | `member_requirements:` in `skipLocal` | VERIFIED | `:49,77` |
| `fuku/modules/memberreq.go` | module, registration, switch | VERIFIED | Read in full |
| `fuku/modules/memberreq_rules.go` | rule table, name classifiers, severity, mention | VERIFIED | Read in full |
| `fuku/modules/memberreq_watch.go` | watcher, runner, pass cache, drain | VERIFIED | Read in full |
| `fuku/modules/memberreq_enforce.go` | executor, hold claim, punishments, marker, CR-01 guard | VERIFIED | Read in full |
| `fuku/modules/memberreq_join.go` | join entry, captcha guard, rejoin reset | VERIFIED | Read in full |
| `fuku/modules/memberreq_fixed.go` | "I fixed it" | VERIFIED | Read in full |
| `fuku/modules/memberreq_notice.go`, `memberreq_limits.go`, `memberreq_warned.go`, `memberreq_unmute.go` | notices, cap and pause, recorded-warn note, lift | VERIFIED | Notice file read to line 130 and by the tests that drive it; the others by grep and tests |
| `fuku/modules/memberreq_settings.go`, `settings_mreq.go` | apply functions, page, rule screen, card | VERIFIED | `memberreq_settings.go` read in full; page by structure and tests |
| `fuku/modules/shutdown_drains.go`, `main.go` | `RegisterMemberReqDrain` and its call | VERIFIED | `shutdown_drains.go:73-79`, `main.go:226` |
| `fuku/modules/greetings.go`, `captcha.go`, `membership.go`, `warns.go`, `bans.go`, `chat_permissions.go` | integration points and the warn core extracted for the executor | VERIFIED | Call sites listed in Key Links; `warns.go` refactor covered by `TestWarn*` (green) |
| Tests named in the plans | 31 new test files | VERIFIED | The focused families below pass |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `LoadMemberRequirements` | the watcher at group -2 and the `mreq|` callback | `AddHandlerToGroup`, `AddHandler` | WIRED | `memberreq.go:49-53` |
| `memberReqWatcher` | `memberReqEnforce` | `memberReqRunAsync` job, capped and recovered | WIRED | `memberreq_watch.go:222,164-181` |
| `membershipDepsForJoin` `Welcome` | `memberReqJoinThenWelcome` | the one welcome entry | WIRED | `greetings.go:1089` |
| `membershipDepsForJoin` `Reset` | `memberReqJoinReset` | after the dedupe claim, before the join is decided | WIRED | `greetings.go:1092` |
| `finishCaptchaPass` | `memberReqJoinThenWelcome` | last step, button and web pass alike | WIRED | `captcha.go:1747` |
| `unmuteCaptchaUser` | `memberReqCaptchaGuard` | first statement, the one captcha unmute choke point | WIRED | `captcha.go:1633` |
| `leftMember` | `memberreq.ClearHolds` | right after the recent-join clear | WIRED | `greetings.go:1005` |
| `memberReqEnforce` | `memberreq.ClaimHold` / `ReclaimStaleHold` | insert-first latch | WIRED | `memberreq_enforce.go:266-297` |
| `memberReqWarnStep` | `applyWarn` with `afterRecord` | warn module's own path | WIRED | `:346` (`memberReqApplyWarn = applyWarn`, `:109`) |
| `memberReqEnforce` Kick | `kickMemberContext` | shared with `/kick` and the captcha | WIRED | `:438`, `bans.go` |
| `memberReqFixedCallback` | `memberReqUnmutePermissions` -> `resolveUnmutePermissionsWithLockdown` | lockdown-aware unmute | WIRED | `memberreq_fixed.go:157` |
| `memberReqFixedRun` | `SendWelcomeMessage` | held welcome, Greet once claims then | WIRED | `:107-112` |
| `registerSettingsPage` page `m` | `memberReqSettingsLookup`, `applyMemberReq*` | page, rule screen, card, switches | WIRED | `settings_mreq.go:41-66` |
| `memberreq` writes | `cache.DeleteCache("member_requirements")` | every settings write | WIRED | `settings.go:136` |
| `main.go` | `RegisterMemberReqDrain` | before `RegisterStaffActionsDrain` | WIRED | `main.go:226` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Member requirements page and Main line | `models.ChatRequirements` | `memberreq.GetSettings` (cached read of `chat_requirements`, no row created) | Yes | FLOWING |
| Watcher verdict | `settings`, `memberReqNamesOf(*msg.From)` | cached settings plus the update's own sender | Yes | FLOWING |
| Executor verdict | live member record and fresh settings | `getChatMember` through the pacer, `memberReqSettingsLookup` again | Yes | FLOWING |
| One punishment | hold | `chat_requirement_holds` conditional insert | Yes | FLOWING |
| I fixed it | hold, live record, permissions | `GetHoldByIDFresh`, `getChatMember`, `getChat` plus the lockdown row | Yes | FLOWING |
| Notice text | rules and mention | rule table phrase keys, the live user, spliced after translation | Yes | FLOWING |

### Behavioral Spot-Checks

All were run by the verifier at HEAD 2d85418 in this sandbox.

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Compile | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Vet | `go vet -tags testtools ./fuku/modules ./fuku/db/memberreq ./fuku/db/models ./fuku/i18n ./fuku/db/backup` | exit 0 | PASS |
| Format | `gofmt -l` on the 16 Go files of the phase (the `.sql` file is not Go) | none | PASS |
| Translations | `make check-translations` | "All translations are present!" (exit 0) | PASS |
| Generated docs | `make check-docs` | "No drift detected" (exit 0) | PASS |
| Dependencies untouched | `git diff 72a692b -- go.mod go.sum` | no diff | PASS |
| Repositories, backup, models, migrations | `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/backup ./fuku/db/models ./fuku/db/migrations` | all 4 ok (includes `TestChatRequirementsSettings`, `TestRequirementHolds`, `TestRequirementHoldClaimConcurrent`, `TestMemberRequirementsStayOutOfBackup`, `TestMemberRequirementsMigrationStatements`) | PASS |
| Locale package | `go test -tags testtools -race -count=1 ./fuku/i18n` | ok (15.9 s) | PASS |
| Phase 13 families, verbose | `go test -tags testtools -race -count=1 -v -run '^(TestMemberReqRulesCorpus\|TestMemberReqBlankLastName\|TestMemberReqUsernameRule\|TestMemberReqRuleTable\|TestMemberReqSeverity\|TestMemberReqStrictest\|TestMemberReqMention\|TestMemberReqEnforcementLive\|TestMemberReqWatcherPunishesOnce\|TestMemberReqWatcherContinuesGroups\|TestMemberReqWatcherExemptions\|TestMemberReqOnePunishmentAcrossReplicas\|TestMemberReqMuteNeverReplacesRestriction\|TestMemberReqFixedUnmutes\|TestMemberReqFixedRefusals\|TestMemberReqFixedNeverLifts\|TestMemberReqFixedDuringLockdown\|TestMemberReqFixedTwoReplicas\|TestMemberReqFixedHeldWelcome\|TestMemberReqFixedRestoreOnFailure\|TestMemberReqKickBan\|TestMemberReqJoinNoCaptcha\|TestMemberReqJoinAfterCaptcha\|TestMemberReqCaptchaGuard\|TestMemberReqRejoinResetsMuteHold\|TestMemberReqDeleteMessages\|TestMemberReqEnforceStrictest\|TestMemberReqEnforceWarnOnce\|TestMemberReqNoticeWarnLimit\|TestMemberReqEnforceExemptions\|TestMemberReqCapPausesPunishments\|TestShutdownWaitsForMemberReqJobs\|TestSettingsMreq\|TestSettingsDriftMreq\|TestSettingsAuthorityMreq\|TestApplyMemberReqAction\|TestSettingsControlActionAllowList\|TestMemberReqScreensFitEveryLocale\|TestMemberReqJoinAndMessageOnce)' ./fuku/modules` | ok (36.6 s), 47 top-level tests PASS, 0 FAIL, 0 SKIP | PASS |
| Phase 13 families, second batch (fail safe, join, pass cache, notices, bytes, privacy) | `go test -tags testtools -race -count=1 -v -run '^(TestMemberReqPunishRefused\|TestMemberReqPunishUnknown\|TestMemberReqEnforceReadsSettingsAfterLookup\|TestMemberReqWarnRecovery\|TestMemberReqMuteNoticeRetry\|TestMemberReqMuteNoticeUndeliverable\|TestMemberReqUndeliveredMuteChecksMarker\|TestMemberReqCapWithoutRedis\|TestMemberReqJoinExemptions\|TestMemberReqWarnMarkInsideLock\|TestMemberReqCleanEndsWarnHold\|TestMemberReqRunnerSheds\|TestMemberReqFixedCallbackBytes\|TestMemberReqFixedButtonBytes\|TestResolveUnmutePermissionsWithLockdown\|TestSettingsStaffPrivacy\|TestMemberReqJoinReadsSettingsAgain\|TestMemberReqJoinNeverReplacesRestriction\|TestMemberReqRejoinResetsPendingMuteHold\|TestMemberReqJoinResetOrder\|TestMemberReqWatcherOffUntilLive\|TestMemberReqJoinOffUntilLive\|TestMemberReqPassCache\|TestMemberReqPassValue\|TestMemberReqNoticeLanguage\|TestMemberReqDeleteDuringPause\|TestMemberReqEnforcePunishments\|TestMemberReqFixedBeforeNoticeStored)$' ./fuku/modules` | ok (48.7 s), 28 PASS, 0 FAIL | PASS |
| Warn, ban, captcha, unmute regressions (the shared code `warns.go`, `bans.go`, `captcha.go`, `chat_permissions.go` changed) | `-run '^(TestWarn\|TestApplyWarn\|TestKick\|TestBan\|TestCaptcha\|TestResolveUnmute)' ./fuku/modules` | ok (16.1 s) | PASS |

Not run here: `make test` as a whole (fuku/modules must not be run whole here; the orchestrator reports two full runs and a CI run green on the code at HEAD minus docs, which this verifier did not reproduce), `make lint` (the sandbox golangci-lint is built for an older Go than the 1.26.0 target) and `make test-postgres-integrity` (no PostgreSQL server). These are human item 6.

### Probe Execution

Step 7c: SKIPPED. The phase declares no probe scripts.

### Test Quality Audit

| Test File | Linked Req | Active | Skipped | Circular | Assertion Level | Verdict |
|-----------|-----------|--------|---------|----------|-----------------|---------|
| `memberreq_rules_test.go` | MREQ-01, MREQ-13 | corpus plus 6 | 0 | no: a literal table of names, each row asserting the classifier and the rule table separately | Value | VALID |
| `memberreq_watch_test.go`, `memberreq_enforce_test.go`, `memberreq_delete_test.go`, `memberreq_limits_test.go` | MREQ-02..05, 09 | all | 0 | no | Behavioral (Telegram calls counted on the fake, rows counted, three replicas over one database and Redis) | VALID |
| `memberreq_fixed_test.go` | MREQ-06, MREQ-07 | all | 0 | no | Behavioral (restrict calls, edits, answers, held welcome sends) | VALID |
| `memberreq_join_test.go`, `memberreq_rejoin_test.go` | MREQ-02, MREQ-08 | all | 0 | no | Behavioral | VALID |
| `memberreq_failure_test.go`, `memberreq_screens_test.go` | fail safe, locale | all | 0 | no | Behavioral and size | VALID |
| `settings_mreq_test.go`, `settings_mreq_card_test.go` | MREQ-01 | all | 0 | no | Behavioral (press, stored row, redraw, forged fields) | VALID |
| `fuku/db/memberreq/*_test.go`, `member_requirements_migration_test.go`, `memberreq_backup_test.go` | storage, D-17 | all | 0 | no | Value and concurrent claims | VALID |

**Disabled tests on requirements:** 0 in the families the verifier ran. **Circular patterns detected:** 0. **Insufficient assertions:** 0.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| MREQ-01 | 13-01..13-14 (five of eight rules) | A Member requirements page gives each rule its own punishment, all rules off by default | SATISFIED for Phase 13's five rules (last name, username, Russian, Arabic, Chinese); stays **Pending** in the tracker until MREQ-11, MREQ-12 and MREQ-14 add the profile-photo, channel and AI spam-name rules in Phases 14 and 15 | Truth 1. REQUIREMENTS.md lines 38 and 154 already read Pending and are correct. Not marked complete here |
| MREQ-02 | 13-05, 13-06, 13-07, 13-09 | Check on join (after the captcha) and a check on every message | SATISFIED | Truth 2 |
| MREQ-03 | 13-07, 13-10 | Delete messages removes a violator's message | SATISFIED | Truth 3 |
| MREQ-04 | 13-02, 13-06, 13-07 | Punished once, across replicas | SATISFIED | Truth 3 |
| MREQ-05 | 13-06, 13-07 | One notice naming the member and the rule; a warn counts toward the limit | SATISFIED | Truth 3 |
| MREQ-06 | 13-08, 13-11 | "I fixed it" re-checks live, unmutes respecting a lockdown, sends the held welcome | SATISFIED (live marker behaviour deferred) | Truth 4 |
| MREQ-07 | 13-08, 13-11 | Only lifts a mute a requirement placed | SATISFIED (live marker behaviour deferred) | Truth 4 |
| MREQ-08 | 13-06, 13-09 | A kicked member can rejoin and is checked again; a ban stays | SATISFIED | Truth 3 |
| MREQ-09 | 13-05, 13-06, 13-09 | Admins, approved users, bots, service accounts, channel and anonymous-admin senders never checked | SATISFIED | Truth 2 |
| MREQ-13 | 13-04 | Russian, Arabic and Chinese name rules judge first and last name; the page says they are letter-based; invisible last names do not count | SATISFIED | Truth 1 |

REQUIREMENTS.md marks MREQ-02..09 and MREQ-13 `[x]` and Complete and MREQ-01 Pending, which matches the code. MREQ-10, MREQ-11, MREQ-12, MREQ-14 and MREQ-15 map to Phases 14, 15 and 16 and are not Phase 13's. No orphaned requirement maps to Phase 13: the ROADMAP's list (MREQ-01..09, MREQ-13) equals the plans' union.

### Decisions D-01..D-23 against the code

| Decision | Holds | Evidence |
| -------- | ----- | -------- |
| D-01 captcha before the requirement check; one shared entry | Yes | `memberReqJoinThenWelcome` called from `membershipDepsForJoin` and `finishCaptchaPass` only (grep) |
| D-02 a join mute holds the welcome until "I fixed it" | Yes | `WelcomeHeld` at the claim, `SendWelcomeMessage` from the callback (`memberreq_fixed.go:185-188`) |
| D-03 only a mute gets the button | Yes | `memberReqNoticeKeyboard(tr, hold.Action, ...)` |
| D-04 strictest wins, one notice names every rule | Yes | `memberReqStrictest`, `memberReqRulesText` |
| D-05 Kick, Ban and Mute go through the two-tap card | Yes | `settingsMreqChoose`, `TestSettingsMreqCard` |
| D-06 one wide row per chat | Yes | migration lines 20-38 |
| D-07 one hold row is latch and mute record | Yes | `uk_chat_requirement_holds_chat_user`, `ClaimHold` |
| D-08 marker `until_date` ownership; captcha never frees a held member | Yes | `isMemberReqMute`, `memberReqCaptchaGuard` |
| D-09 "I fixed it" in a lockdown through `resolveUnmutePermissions` | Yes | `memberReqUnmutePermissions`, `TestMemberReqFixedDuringLockdown` |
| D-10 names judge first and last name, never the username | Yes | rule table, `TestMemberReqRulesCorpus` |
| D-11 exemptions, unknown is exempt | Yes | `memberReqNeverChecked`, `memberReqExempt` |
| D-12, D-19 watcher at group -2, `ext.ContinueGroups` on every path | Yes | `memberreq_watch.go:191-231`, `TestMemberReqWatcherContinuesGroups` |
| D-13 punishments reuse existing paths | Yes | `applyWarn`, `kickMemberContext`, `banChatMember`, `restrictChatMember` |
| D-14 Delete messages incl. held members; a failed delete only logged | Yes | `memberReqDeleteHeld`, `memberReqDeleteMessage` |
| D-15 `fuku:memberreq:*` outside the cache prefix | Yes | pass, pace, cap, paused, warned keys |
| D-16 page from its own file, side-effect-free loader | Yes | `settings_mreq.go`, `GetSettings` |
| D-17 one migration, models, harness lists, out of backup | Yes | Truth 6 |
| D-18 test coverage list | Yes | corpus, three replicas, strictest, card, never-lifts families all pass |
| D-20 supergroups only, clean message costs no Telegram call, no DB write beyond the guarded predicate | Yes | watcher filter and order; `skipLocal` for the settings key |
| D-21 pacer, cap 10, pause 10 minutes, one pause notice, mute notice never dropped by pacing | Yes | `memberReqPacer`, `memberReqCapAllow`, `memberReqPostNotice` |
| D-22 a mute hold survives a leave and is settled at the rejoin | Yes | `memberReqJoinReset`, `TestMemberReqRejoinResetsMuteHold` |
| D-23 answers and the notice edit in the group's language | Yes | `ctxTr` of the group callback, `memberReqTranslator` |

### Prohibitions (judgment tier, non-authoritative verdict, flagged for owner review)

The plans' prohibitions are `verification: judgment`. The verdicts below are the verifier's, non-authoritative, and listed among the human items.

| Area | Prohibition | Verifier verdict | Evidence |
| ---- | ----------- | ---------------- | -------- |
| Watcher | Never ends or skips an update, never makes a Telegram call or DB write for a clean member beyond the guarded predicate, no second message handler in group -2 | holds | `ext.ContinueGroups` on every path; only handler of the group; `TestMemberReqWatcherContinuesGroups` |
| Executor | Never punishes on an unknown state; never uses `IsUserAdmin`, `IsUserApproved` or `aispamCarvedOut` | holds | Tri-state reads; grep shows none of the three in the executor |
| Mute | Never replaces a restriction someone else placed; never lifts a mute without the live marker | holds | `memberReqMuteOverRestriction`, `isMemberReqMute`, `TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqFixedNeverLifts` |
| "I fixed it" | No admin override; lifts only with permissions from `resolveUnmutePermissionsWithLockdown`, never `MutedPermissions` | holds | `memberreq_fixed.go:90-93,157,172` |
| Ban | A ban is never lifted by any requirement path | holds | No unban call outside `kickMemberContext` |
| Logs and strings | No name, rule text or notice text in a log line; no Staff Group, member or held count on any screen | holds | `memberReqErrText` carries codes only; `TestSettingsStaffPrivacy` renders the page in 8 languages |
| Storage | Neither table in backup, export, import or reset; no `default:` tag on a new boolean | holds | `TestMemberRequirementsStayOutOfBackup`, model read |
| Captcha | The captcha guard changes nothing while the switch is off and fails closed on an unreadable hold | holds | `TestMemberReqCaptchaGuardOffUntilLive`, `errMemberReqHoldUnreadable` |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (phase files, production) | - | No `TODO`, `FIXME`, `XXX`, `TBD`, `HACK`, placeholder or "not yet implemented" in the 15 new or changed production Go files, the migration or `settings_mreq.go` | - | none |
| (phase files, production) | - | The only discarded value is the `_` of `known, _ := cachedAdminStatus(...)` (`memberreq_enforce.go:625`), where only the "known" flag is wanted; `tr.GetString` errors go through `memberReqText`, which logs them. No DB error is discarded on a state-changing path | - | none |
| `fuku/modules/memberreq_enforce.go` | 21-23 | The file comment still says "nothing here is reached by a handler yet", which is stale: the watcher, the join entry and the button reach it | Info | Comment only |
| `fuku/modules/memberreq_watch.go` | 183-187 | Without Redis the pass cache never hits, so every clean message from a member of a group with a rule on runs the clean predicate (one DB statement) | Info | Documented as "correct, slower" (AGENTS.md pass bullet). Only groups with a rule on |
| `fuku/modules/memberreq_enforce.go` | 540-548 | IN-05 and NEW-02: an admin with a cold admin cache whose name breaks a rule, or a restricted violator below the defaults, costs one paced `getChatMember` per message, because no hold or exempt verdict is stored | Info | Deferred and accepted in 13-REVIEW-DISPOSITION.md; shares the fleet pacer |
| `AGENTS.md` | pacing bullet | "every Telegram call of `memberReqEnforce` goes through it" is imprecise for the warn module's limit action (SEC-NEW-04, AR-16) | Info | Documentation only, recorded in the disposition |
| exported API | - | None unused: every exported function of `fuku/db/memberreq` has a production caller (table above) | - | none |

### Prior Audits (used as evidence, spot-checked, not redone)

- **Code review (`13-REVIEW.md`, `13-REVIEW-FIX.md`, `13-REVIEW-DISPOSITION.md`):** 1 critical, 3 warnings, 6 info. CR-01, WR-01, WR-02 and WR-03 are fixed, and the verifier saw each fix in the code: `memberReqMuteOverRestriction` (`memberreq_enforce.go:685`), the 3-attempt `memberReqMarkPunished` plus the `fuku:memberreq:warned` note (`:319-333`, `memberreq_warned.go`), the settings re-read in `memberReqLiveViolation` (`:248-258`), and the pending-hold lift in `memberReqJoinReset` (`memberreq_join.go:201-211`). The tests named for each pass here (`TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqWarnRecovery`, `TestMemberReqEnforceReadsSettingsAfterLookup`, `TestMemberReqRejoinResetsMuteHold`). IN-01..IN-06 and NEW-02..NEW-04 are deferred, accepted and listed for the owner. `open: 0`.
- **Security (`13-SECURITY.md`, `13-SECURITY-FIX.md`):** status verified, `threats_open: 0`. SEC-NEW-01 (bidi strip) is in the code (`memberReqStripBidi`, `memberreq_rules.go:239-247`, `TestMemberReqMention` passes) and SEC-NEW-02 (marker re-check before the undelivered-notice lift) is covered by `TestMemberReqUndeliveredMuteChecksMarker`, which passes here. SEC-NEW-03..06 are accepted risks.
- **Validation (`13-VALIDATION.md`):** `validated`, `nyquist_compliant: true`.

### Human Verification Required

See the frontmatter list. In short:

- 13-SPIKE.md live checks 2 to 5, 8 and 14 (the 330-day marker readback, a muted member's leave and rejoin, the lockdown unmute, how long a callback stays answerable, restricted-member readback), which must be done before any group sets a rule to Mute. Warn, Kick and Ban do not depend on them.
- Checks 1, 6 and 7 (the group's message limit and the cap, a collectible-username account, names current after a rename), 9 and 10 (bidi with an Arabic name, the seven cells and choice rows on hi, ru and uk phones), 11 (native-speaker read of es, fr, hi, id, pt, ru, uk), 12 (two live replicas over one PostgreSQL).
- CI on HEAD (check 13): `make lint` with a Go 1.26 golangci-lint, `make test`, and `make test-postgres-integrity` running the three Phase 13 database tests for real.
- The owner's review of the judgment-tier prohibitions and of the deferred follow-ups: UI-SPEC A1, A4, A8, A14, A15, A17, A18, IN-01 to IN-06, NEW-02 to NEW-04 and SEC-NEW-03 to SEC-NEW-06.

### Gaps Summary

No gaps. Every ROADMAP success criterion, every requirement in scope, every decision D-01 to D-23 and the wiring from the migration to the shutdown drain is delivered and present in the code:

- Five rules with per-rule punishments, two switches and everything Off by default are stored in one migration and one wide row, read through the cached, side-effect-free loader, and written column by column with the cache key dropped.
- The watcher at group -2 and the one join entry (captcha first) feed one executor that claims an insert-first hold, so a violation is punished once across replicas. It punishes through the existing warn, kick, ban and restrict paths, posts one notice and deletes the violator's messages only when the sender is established as a non-admin.
- "I fixed it" lifts only a mute whose live marker proves the requirements placed it, through the lockdown-aware permission resolver, and sends the held welcome once. The captcha guard and the rejoin reset keep a held member held and never lift anything that is not theirs.
- A basic group is not screened (documented limit D-20), and an unanswerable check punishes nobody.

MREQ-01 is delivered on five of eight rules by design and stays Pending until Phases 14 and 15. The remaining open work is the live checks the owner deferred (as in Phases 10 to 12), the first CI run on HEAD, and the owner's review of the judgment-tier items.

---

_Verified: 2026-11-08T02:10:00Z_
_Verifier: Claude (gsd-verifier)_
