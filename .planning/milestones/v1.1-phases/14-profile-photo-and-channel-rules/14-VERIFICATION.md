---
phase: 14-profile-photo-and-channel-rules
verified: 2026-11-08T14:30:00Z
status: human_needed
score: "16/16 must-haves verified (4 ROADMAP success criteria plus 12 supporting truths); MREQ-11, MREQ-12 and PLAT-06 judged satisfied on Phase 14 scope, MREQ-10 on all but its Phase 15 AI line, MREQ-01 on seven of eight rules"
covered_files:
  - ".planning/phases/14-profile-photo-and-channel-rules/14-01-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-01-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-02-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-02-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-03-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-03-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-04-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-04-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-05-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-05-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-06-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-06-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-07-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-07-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-08-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-08-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-09-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-09-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-10-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-10-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-11-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-11-SUMMARY.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-12-PLAN.md"
  - ".planning/phases/14-profile-photo-and-channel-rules/14-12-SUMMARY.md"
  - "AGENTS.md"
  - "Makefile"
  - "README.md"
  - "docs/src/content/docs/api-reference/callbacks.md"
  - "docs/src/content/docs/api-reference/permissions.md"
  - "fuku/db/memberreq/settings.go"
  - "fuku/db/models/member_requirements.go"
  - "fuku/modules/memberreq_enforce.go"
  - "fuku/modules/memberreq_fixed.go"
  - "fuku/modules/memberreq_held.go"
  - "fuku/modules/memberreq_lookup.go"
  - "fuku/modules/memberreq_notice.go"
  - "fuku/modules/memberreq_rules.go"
  - "fuku/modules/memberreq_watch.go"
  - "fuku/modules/settings_mreq.go"
  - "locales/en.yml"
  - "locales/es.yml"
  - "locales/fr.yml"
  - "locales/hi.yml"
  - "locales/id.yml"
  - "locales/pt.yml"
  - "locales/ru.yml"
  - "locales/uk.yml"
  - "migrations/20261108120000_add_member_requirement_lookup_rules.sql"
covered_digest: "v4:sha256:0d706d3d556891511e15b5564fc31ec7b81941d1974e686e455f6f1b425190bc"
behavior_unverified: 0
overrides_applied: 0
unverified_prohibitions: 12
human_verification:
  - test: "14-SPIKE.md S1, BEFORE any group sets the photo rule to Mute, Kick or Ban: in a test group, check what getUserProfilePhotos returns for a member who hides their photo from the bot (a total of 0, or an error). Also the public-photo fallback check (14-SPIKE.md section 4)."
    expected: "A hidden photo reads as a total of 0 (a breach, as the page text 'A photo hidden by privacy settings counts as missing' says), or as an error (unknown, nobody punished). A member who shows their photo is never punished."
    why_human: "The tests drive every shape (total 0, total above 0, list, 400, 403, 429, 500, timeout, shed, null) through a hand-written Bot fake. What Telegram really returns for a hidden photo is live Bot API behaviour. The code is safe under both outcomes, so this changes wording only, and the owner deferred it."
  - test: "14-SPIKE.md S2, S3 and sections 5 and 10, BEFORE enabling a punishment for the channel rule: (S2) what getChatMember(channel, member) answers a bot that is NOT the channel's administrator; (S3) whether getChat of the group returns linked_chat_id to a bot that is only a plain member of the group; the member statuses of a channel; a channel linked and unlinked while a rule is set."
    expected: "A non-administrator bot gets an error, or a status the D-22 gate reads as inert (nobody punished, the page says the bot is not an administrator). getChat returns linked_chat_id to a plain-member bot (otherwise the rule is inert there). left, kicked and restricted answers read as the code expects."
    why_human: "memberReqChannelOutcome, memberReqGateBreach and memberReqClassifyBot are proved against the fake in TestMemberReqChannelBreachNeedsBotAdmin, TestMemberReqDemotedBotNeverBreaches and TestMemberReqBotStatusClassifier, but only a real channel shows what Telegram answers. By construction (D-22, SEC-03) no outcome punishes anyone."
  - test: "14-SPIKE.md section 6: the two new cells (Profile photo, Channel) and the Off-only channel screen in hi, ru and uk on a phone."
    expected: "No cell is cut off with an ellipsis (20-code-point cell limit, Hindi combining marks included) and the Off-only screen reads clearly."
    why_human: "TestSettingsLabelLengths, TestSettingsMreqPageFitsEveryLocale and TestSettingsMreqCallbackBytes hold the code point and byte limits. Truncation at phone width is client rendering."
  - test: "14-SPIKE.md section 11: a native speaker of each of es, fr, hi, id, pt, ru and uk reads the 16 new and 7 reworded keys (mreq_rule_photo, mreq_rule_channel, mreq_remedy_photo, mreq_fixed_still_broken_many, settings_mreq_here_*, settings_mreq_desc_photo, settings_mreq_desc_channel, settings_mreq_hint_photo, settings_mreq_hint_lookup, the rule and cell keys, and the seven reworded M12 keys)."
    expected: "Each string reads naturally. The sentences that say a hidden photo counts as missing and that the bot must be a channel administrator are understood."
    why_human: "Translation quality is a human judgement. make check-translations, TestMemberReqLocaleKeys and the fit tests prove only presence, placeholders and size."
  - test: "14-SPIKE.md sections 7, 8 and 9: two live replicas over one PostgreSQL and one Redis share the lookup budget; a raid with the lookup pacer shedding; a restart during a burst of lookups."
    expected: "One punishment, one notice across replicas. During a raid the lookup pacer sheds lookups and an admin's page open without delaying any notice or removal. A restart mid-burst leaves no hold and punishes nobody; the next message starts again."
    why_human: "TestMemberReqLookupPacer, TestMemberReqPacersAreIndependent, TestMemberReqLookupOnePunishmentAcrossReplicas and TestShutdownEndsMemberReqLookups prove it with two pacers over one miniredis and a cancelled root context. Real processes with a real deployment config and real Telegram flood limits can only be checked live."
  - test: "14-SPIKE.md section 12 and CI on HEAD aa68b34: make lint with a golangci-lint built for Go 1.26, make test, and make test-postgres-integrity."
    expected: "make lint clean for the new lines, make test green, and the integrity step runs TestMemberRequirementLookupMigrationDefaults (./fuku/db/migrations) and the extended TestChatRequirementsSettings (./fuku/db/memberreq) for real on PostgreSQL: the migration's statements run twice, both named constraints exist, 5 is refused with SQLSTATE 23514."
    why_human: "The verifier did not run make test (20 minutes, CI covers it), make lint (the sandbox golangci-lint is built for an older Go) or the PostgreSQL target (no PostgreSQL server here). TestMemberRequirementLookupMigrationDefaults skips without PostgreSQL by design (scripts/check_test_results allows it). The verifier ran the cheap checks and 23 named tests instead (see Behavioral Spot-Checks)."
  - test: "Owner review of the 12 judgment-tier prohibitions of plans 14-01 to 14-12 (see 'Prohibitions' below), the two small deviations from plan wording (the channel-level cna fact; WR-02's deferral), the PLAT-06 tick (see Requirements Coverage), and the deferred review and security items IN-01, IN-02, IN-04, IN-05, NEW-01 to NEW-03, SEC-01 and SEC-04."
    expected: "The owner accepts the verifier's non-authoritative verdict (every prohibition holds, each backed by a passing test or a direct code read), decides whether PLAT-06 stays ticked now or only when Phases 15 to 17 add their work, and chooses which follow-ups become their own changes."
    why_human: "An autonomous verifier never passes a judgment-tier prohibition silently. The follow-ups are product decisions the phase left open on purpose."
audit_acknowledged:
  milestone: v1.1
  at: 2026-11-13
  status: human_needed
---

# Phase 14: Profile Photo and Channel Rules Verification Report

**Phase Goal:** As a group owner, I want to require a profile photo and, in a channel's comment group, a subscription to that channel, so that throwaway accounts and outsiders are screened too, without anyone punished because a lookup failed.
**Mode:** mvp (the user story format is valid: `user-story.validate --pick valid` printed `true`)
**Verified:** 2026-11-08T14:30:00Z at HEAD aa68b34 (branch claude/ecstatic-goldberg-qsss05, clean working tree)
**Status:** human_needed
**Re-verification:** No, initial verification

All four ROADMAP success criteria and every must-have of the twelve plans are delivered and wired in the code, and each behaviour-dependent truth has a passing test that drives the real handlers through the hand-written Bot fake, miniredis and SQLite. The verifier read the production code and ran the tests itself (see Behavioral Spot-Checks). No gap was found. I tried to find a path where an unanswerable lookup punishes, where a punishment rests on a stale link or a demoted bot, and where AGENTS.md and the code disagree. The first two did not exist (details under Safety invariants). The third found two wording slips in AGENTS.md (Info, below), not behavioural contradictions.

The status is `human_needed` and not `passed` only because the owner deferred the live Telegram checks (14-SPIKE.md S1 to S3 and the other sections), the native-speaker read of the new locale keys, a two-replica live test and CI on HEAD, and because the judgment-tier prohibitions need the owner's eyes. The code is safe under every outcome those checks can have (D-19, D-22, SEC-03).

## User Flow Coverage

User story: «As a group owner, I want to require a profile photo and, in a channel's comment group, a subscription to that channel, so that throwaway accounts and outsiders are screened too, without anyone punished because a lookup failed.»

| Step | Expected | Evidence | Status |
| ---- | -------- | -------- | ------ |
| Owner opens the Member requirements page and sees a Profile photo cell and a Channel cell | Seven cells in rule order, two per row, odd last alone | `settings_mreq.go:384-397` `settingsMreqRuleGrid` ranges `memberReqRules`; `memberreq_rules.go:114-117` the two rows (`ph` bit 32, `ch` bit 64); `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes`, `TestSettingsMreqPageFitsEveryLocale` | VERIFIED |
| Owner opens the photo rule and learns a hidden photo counts as missing | The description says it and the hint recommends Warn or Mute | `locales/en.yml:3267` `settings_mreq_desc_photo`, `:3248` `settings_mreq_hint_photo`; rendered by `settings_mreq.go:477-522` through the rule's `HintKey` | VERIFIED |
| Owner sets Warn, or Kick, Ban or Mute behind the two-tap card | Written through `applyMemberReqAction`; for the channel rule only with a live linked channel | `settings_mreq.go:563-588`, `:599-629` `settingsMreqChannelGate`; `TestSettingsMreqChannelRefusal` | VERIFIED |
| A member with clean names and no photo posts | One lookup, one hold, one notice naming the rule with the photo remedy, a mute carries "I fixed it" | `memberreq_watch.go:259-276` hands a miss to a job; `memberreq_enforce.go:258-309` lookups; `memberreq_notice.go:134-155` remedy; `TestMemberReqLookupEndToEnd` (passed) | VERIFIED |
| A member outside the linked channel posts, the bot administers it | Punished by the channel rule once | `memberreq_lookup.go:553-652` (candidate, fresh link re-read, fresh bot standing); `TestMemberReqChannelBreachNeedsBotAdmin` (passed) | VERIFIED |
| The bot is not a channel administrator, or any lookup fails | Nobody punished, the page says so | `memberreq_lookup.go:608-625`; `settings_mreq.go:259-297`; `TestMemberReqLookupUnanswerable` (28 shapes, job and join entry), `TestSettingsMreqChannelStatus` (passed) | VERIFIED |
| The muted member adds a photo or subscribes and taps "I fixed it" | Fresh lookups judge every enabled rule, the member is unmuted, still-broken rules are named | `memberreq_fixed.go:125-255`; `TestMemberReqFixedLookups`, `TestMemberReqChannelInert` (passed) | VERIFIED |

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | ROADMAP SC1: the profile-photo rule punishes a member who has no photo the bot can see; a hidden photo counts as missing and the page says so | VERIFIED | `memberreq_lookup.go:286-322` `memberReqPhotoRule`: total 0 and no photos is `memberReqBreach` (`:313-320`), a total above 0 or a list is a pass cached 30 min (`:313-316`). Page text `locales/en.yml:3267`. Executor folds the bit in at `memberreq_enforce.go:287-288`. `TestMemberReqPhotoRule`, `TestMemberReqLookupPunishes`, `TestMemberReqLookupEndToEnd` pass. The real Telegram behaviour for a hidden photo is spike S1 (human item 1) |
| 2 | ROADMAP SC2: the channel rule is offered only in a group linked to a channel and punishes a member who is not subscribed | VERIFIED | Offered: the cell is always drawn (D-05) but `settingsMreqChannelGate` (`settings_mreq.go:599-629`) refuses any non-Off value in a basic group or a group whose live `getChat` has no linked channel, and the screen becomes Off-only (`:462-467`, `:498-500`). Punishes: `memberReqChannelRule` (`memberreq_lookup.go:553-597`) with `memberReqSubscribed` (`:430-443`) and the D-22 gate. `TestSettingsMreqChannelRefusal`, `TestMemberReqChannelRule`, `TestMemberReqLookupEndToEnd` pass |
| 3 | ROADMAP SC3: a check that cannot be answered (Telegram error, timeout, the bot not a channel admin) never punishes anyone; the page shows when a rule cannot work (no linked channel, bot not admin); the AI line arrives in Phase 15 | VERIFIED | Verdict zero value is unknown (`memberreq_lookup.go:34`); a lookup adds a bit only on `memberReqBreach` (`memberreq_rules.go:200-206`); every error, null body, shed, cancelled context is unknown (`:304-312`, `:582-589`); the claim follows every lookup (`memberreq_enforce.go:633-646`). Page lines `settings_mreq.go:259-297`, locale `settings_mreq_here_basic/_nolink/_notadmin/_unknown`. `TestMemberReqLookupUnanswerable` (28 shapes x job and join, passed), `TestMemberReqBotStatusClassifier` (passed). No AI line exists in `settings_mreq.go` (correct, it is Phase 15) |
| 4 | ROADMAP SC4: lookups share one Telegram rate budget across replicas and never hold up the rest of the bot; a lookup cut off by a crash or shutdown punishes nobody and leaves nothing stuck; lookups in flight end within the existing shutdown budget | VERIFIED | Own pacer keys `fuku:memberreq:lkp:pace:next/block`, Interval 100 ms, MaxRetries 1, MaxWait 5 s (`memberreq_lookup.go:87-101`), separate from `memberReqPacer` (`memberreq_enforce.go:32-38`); flights derive from `memberReqContext()` (`memberreq_lookup.go:249`), cancelled by `StopMemberReqJobs` (`memberreq_watch.go:379-393`); no drain added (see truth 10). `TestMemberReqLookupPacer`, `TestMemberReqPacersAreIndependent`, `TestShutdownEndsMemberReqLookups` pass |
| 5 | Unknown never punishes: a failed, shed, timed out, cancelled or null lookup adds no bit, makes no hold, no notice, no warn, no Telegram write | VERIFIED | `memberreq_rules.go:190-208` (only `case memberReqBreach` sets a bit); `memberreq_enforce.go:258-309` returns `ok=false` for an unreadable record, setting or re-read. `mreqWantNothing` (`memberreq_lookup_enforce_test.go:224-243`) asserts outcome None, no hold, no warns row, zero ban/restrict/unban calls, no notice text, for 28 shapes through the job and the join entry (join: also exactly one welcome) |
| 6 | A channel breach needs a FRESH, definite administrator answer for the bot; a candidate whose link changed or cannot be read is unknown; no linked channel is inert | VERIFIED | `memberreq_lookup.go:608-625` `memberReqChannelOutcome`: breach candidate -> `memberReqLinkUnchanged` (fresh `getChat`, `:630-633`) -> `memberReqGateBreach` forces `mode.Fresh = true` (`:643-652`) so the cached `ba` positive is never believed; a definite non-administrator (status or 400/403 by code, `:467-482`) is inert; 429, shed, 5xx, timeout, null, unrecognised status are unknown. No linked channel: `channel == 0` returns inert (`:558-560`). `TestMemberReqDemotedBotNeverBreaches` (left, kicked, restricted-not-member with a cached administrator standing; passed), `TestMemberReqStaleLink` (passed) |
| 7 | A punishing value of the channel rule (Warn, Kick, Ban, Mute, and the card's Yes) is refused without a live linked channel; the link comes only from a live getChat | VERIFIED | `settings_mreq.go:599-629` runs after the whole press pipeline, after the expiry check and the fresh same-value read (`:566-581`, `:688-702`), uses `memberReqLinkedChannel(... Fresh: true)` (`:608`), refuses basic groups with no call, answers `mreq_fixed_check_failed` for an unreadable link and edits nothing. The only non-test reader of `LinkedChatId` is `memberreq_lookup.go:408-416`. `TestSettingsMreqChannelRefusal` (Warn, Kick, Ban, Mute, the card's Yes after losing the link or a shed pacer; both menu kinds; passed) |
| 8 | Only positives are cached; a missing photo, a missing subscription and every unknown answer are never cached; the `cna` negative fact is channel-level and never a verdict about a member | VERIFIED | Photo: `memberreq_lookup.go:313-320`. Subscription: `:590-594` (`judged == memberReqPass` only). Link: positive 60 s, none 10 min, error/nil/positive-ID not cached (`:402-416`). `ba`: administrator only (`:516-519`). `cna`: written only on a definite not-administrator, keyed by channel, fixed marker `1`, 60 s; it can only turn an unanswerable member lookup into inert, never a breach (`:340-347`, `:520-524`, `:561-565`). Amends plan 14-03's "no verdict other than a pass is cached" for this one channel-level fact (WR-01, documented in AGENTS.md Data and Subsystem traps). `TestMemberReqPhotoRule`, `TestMemberReqChannelRule`, `memberreq_cna_test.go` pass |
| 9 | Lookups use their own pacer keys, separate from the punishment pacer, in both directions | VERIFIED | `memberReqLookupPaced` (`memberreq_lookup.go:123-129`) vs `memberReqPaced` (`memberreq_enforce.go:67-73`). Notices, punishments, deletes and the live `getChatMember` use `memberReqPaced`; photo, link, subscription and bot status use `memberReqLookupPaced`. `TestMemberReqPacersAreIndependent` (a 30 s block on one pacer's keys leaves the other untouched at the job, join entry, "I fixed it" and page; passed) |
| 10 | No new shutdown drain and no change to the 65 s worst case; flights end with `StopMemberReqJobs` | VERIFIED | `git log -- main.go fuku/modules/shutdown_drains.go` last touched by `f6407cf feat(13-09)`; `git diff e96091e HEAD --stat` over `main.go`, `shutdown_drains.go`, `runner.go`, `migrate_psql.sh`, `settings_pages.go`, `go.mod`, `go.sum` is empty. Flight context `memberreq_lookup.go:249` derives from `memberReqContext()`, caller waits on its own `ctx.Done()` (`:253-262`). `lk` never touches PostgreSQL. `TestShutdownEndsMemberReqLookups` passes |
| 11 | The watcher stays cheap and returns `ext.ContinueGroups` on every path; approved and admin members are exempt | VERIFIED | `memberreq_watch.go:334-366` named result preset to `ext.ContinueGroups`, deferred recover; `memberReqLookupCached` (`:178-223`) is Redis only (one pipelined round, `cna` only when `sub` misses); a cache miss starts a job unless `memberReqMissExempt` (cached admin, approved, or an unreadable approvals read, `:284-294`); executor exemptions `memberreq_enforce.go:610-612`, live eligibility `:175-189`, and the SEC-02 re-judge after a slow lookup phase `:316-330`. `TestMemberReqWatcherLookupRules`, `TestMemberReqWatcherLookupContinuesGroups`, `TestMemberReqExemptionsReadAfterLookups` (approved, captcha pending and an approvals read error flipping during the lookup phase, all four punishments; passed) |
| 12 | "I fixed it" re-proves the live marker and never lifts another mute, now with fresh lookups | VERIFIED | `memberreq_fixed.go:140-147` marker check first; approvals step 4b after it (`:152-156`); fresh lookups under the 5 s sub-budget with `Fresh: true, Classify: true` (`:244`); marker proved again right before the take when a lookup made a request (`:181-190`, WR-04); permissions only from `memberReqUnmutePermissions` (`:167`). `TestMemberReqFixedLookups`, `TestMemberReqChannelInert`, `TestMemberReqFixedNeverLiftsWithLookupRules` and the Phase 13 `TestMemberReqFixedNeverLifts` pass |
| 13 | Logs carry IDs and error codes only | VERIFIED | `memberReqLookupLogFailure` (`memberreq_lookup.go:269-275`) logs "rate limited" or `memberReqErrText` (a Telegram error by code); every other lookup log line uses IDs only (grep of `memberreq_lookup.go`); gotgbot scrubs the bot token out of transport errors (`request.go:153` `sanitizeError`). `TestMemberReqLookupLogsCarryIDsOnly` (hook at debug level over every failure shape plus a real `BaseBotClient`; 30.6 s; passed). The channel's numeric ID is logged at Debug by design (SEC-01, accepted) |
| 14 | Migration, model and repository for the two columns: idempotent, named CHECKs, zero defaults, no applier change | VERIFIED | `migrations/20261108120000_add_member_requirement_lookup_rules.sql:23-24` two `ADD COLUMN IF NOT EXISTS ... SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_photo/_channel CHECK (... BETWEEN 0 AND 4)`; no `BEGIN`, no `ADD CONSTRAINT`; it is the newest file in `migrations/` (sorts after `20261106120000`). Model `member_requirements.go:36-37` carries the same check tags and no `default:` tag. `settings.go:42-95,150-157` `RulePhoto`/`RuleChannel` appended to `Rules`, `ruleColumns`, `actionField`; unknown rule and out-of-range action refused before a write. `TestChatRequirementsSettings` and `TestMemberRequirementLookupMigrationStatements` pass; `TestMemberRequirementLookupMigrationDefaults` is PostgreSQL-only (human item 6) |
| 15 | New strings exist in all 8 locale files; no Go reference to a missing key | VERIFIED | Script over `locales/*.yml`: 77 `mreq_`/`settings_mreq_`/`settings_btn_mreq_` keys in `en.yml`, none missing or empty in the other 7 files, placeholder sets identical to English, no `<`, `>` or `&`; the 16 new keys each present in all 8; counts 29 `mreq_` and 247 `settings_` match AGENTS.md. Script over non-test Go: 77 referenced key names, the only one absent from `en.yml` is the prefix `mreq_notice_warn_limit_` (built dynamically from `kick`/`ban`/`mute`, all three exist). `make check-translations` exit 0, `TestMemberReqLocaleKeys`, `TestSettingsLocaleKeys`, `TestMemberReqNoProfileInReworkedKeys` pass |
| 16 | The rulebook (AGENTS.md) describes the shipped behaviour | VERIFIED (2 wording slips, Info) | Every Phase 14 claim spot-checked against the code: pacer options, TTLs (60 s link, 10 min none, 30 min positives, 2 min `ba`, 60 s `cna`), budgets (20 s job, 4 s join, 5 s fixed, 1 s stale-after, 3 s page), order of the executor steps, held release before the pause check, watcher read order, 65 s worst case, 7 cells and 6 rows, 29 and 247 key counts. All 179 `Test*` names AGENTS.md cites resolve (the 28 that are prefixes of families end in `*` in the text). Two slips: see Anti-Patterns Found |

**Score:** 16/16 truths verified (0 present, behavior-unverified)

### Prohibitions (judgment tier, non-authoritative verdicts)

`unverified-prohibition: human review recommended` for each. None is passed silently; each is backed by a code read or a passing test, and the owner should confirm.

| Plan | Prohibition (short) | Verdict | Evidence |
| ---- | ------------------- | ------- | -------- |
| 14-01 | No gorm default tag, no `ADD CONSTRAINT` or `NOT VALID`, no change to `runner.go` or `migrate_psql.sh` | holds | `member_requirements.go:36-37`; migration lines 23-24; empty `git diff e96091e HEAD` over both appliers |
| 14-02 | No new string carries a title, name, chat ID, count, channel title or ID, or the Staff Group; icons are code-side | holds | placeholder sets of all 77 keys are `action id limit minutes n name rule rules state` (`id` is the pre-existing Phase 13 `mreq_name_unknown` user-ID fallback); `settings_mreq.go:259-277` icon prefixes in Go; `TestMemberReqLookupAlertsFitEveryLocale` |
| 14-03 | A lookup never uses the requirement pacer and vice versa; only a pass is cached; no pool or drain | holds, with one documented amendment | see truth 8 and 9 (`cna` is a channel-level fact, WR-01; `lc` and `ba` are facts about the link and the bot, not verdicts about a member) |
| 14-04 | Channel path never classifies by description and never uses `IsBotMissingError` | holds | `memberReqClassifyBot` (`memberreq_lookup.go:467-482`) uses status and `cache.IsBotGoneError` (code); the only `IsBotMissingError` mention in `fuku/modules/memberreq*.go` or `settings_mreq*.go` is a comment (`:465`) |
| 14-05 | No renderer, control, grid or callback branches on a rule's identity except `HintKey`; `settings_pages.go` untouched | holds for the plan's scope | the page and grid range the table. Plans 14-09 and 14-10 later branch on the channel rule on purpose (`settingsMreqRuleSlot`, `settingsMreqOffOnly`, `settingsMreqChannelGate`, D-05 and D-15), and `memberReqNoticeHasRemedy` finds the photo row by its code. `settings_pages.go` is untouched since Phase 13 |
| 14-06 | No lookup in the watcher, before exemptions, in a paused chat, for an Off rule or after the claim; claim never moved | holds | `memberreq_enforce.go:610-646` order; `memberReqLookupMasks` skips Off rules (`memberreq_rules.go:197`) |
| 14-07 | Watcher makes no Telegram call, writes no inert or unknown verdict; held release never punishes, claims or lifts a mute | holds | `memberReqLookupCached` reads only Redis; `memberReqHeldReleased` (`memberreq_held.go:44-68`) ends only in `ClearHoldOnClean` |
| 14-08 | Remedy only on warn, kick, mute notices; no new key; approvals through the cached seam; unmute permissions only from the lockdown-aware resolver | holds | `memberreq_notice.go:147-155`; `memberreq_fixed.go:152,167` |
| 14-09 | No screen names the channel title or ID, the Staff Group or a member; page makes no call for a channel rule that is Off or in a basic group; Main makes no call | holds | `settings_mreq.go:200-231` returns early for Off, basic and Basic scope; `settingsMreqMainState` reads the database only; `TestSettingsMreqOpenCost`, `TestSettingsStaffPrivacy` |
| 14-10 | The gate never reads the link from callback data, never checks for Off, a stored value or an expired Yes, never refuses on the bot's own admin status, never writes before it passed | holds | `settings_mreq.go:566-581`, `:599-629`, `:681-703` |
| 14-11 | No Phase 14 screen, alert, notice or log names the Staff Group, the channel title or ID, a file ID, Telegram's description or a count | holds (one accepted exception) | `TestMemberReqLookupLogsCarryIDsOnly`, `TestSettingsStaffPrivacy`; the channel's numeric ID in three Debug lines is SEC-01, accepted |
| 14-12 | The gate plan changes no production code unless its own end-to-end test proves it necessary | holds | the plan's commits (`098e0f3`, `e15a8dd`, `5c4634f`, `73aba64`, `1690613`) are a test and docs only |

### Deferred Items

None. Everything Phase 14 owns is delivered. The "AI not configured" status line and the AI spam-name rule belong to Phase 15 (ROADMAP Phase 15 success criterion 3 says "the page says AI is not configured"; MREQ-14), so they are not gaps here.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `migrations/20261108120000_add_member_requirement_lookup_rules.sql` | two idempotent columns with named CHECKs | VERIFIED | 24 lines, two statements, wired by the model and `ruleColumns` |
| `fuku/db/models/member_requirements.go` | `PhotoAction`, `ChannelAction` | VERIFIED | check tags equal the migration's names |
| `fuku/db/memberreq/settings.go` | `RulePhoto`, `RuleChannel`, columns | VERIFIED | in `Rules`, `ruleColumns`, `actionField`; `AnyRuleOn` and `ActionOf` pick them up |
| `fuku/modules/memberreq_lookup.go` (652 lines) | verdict type, pacer, flights, positive caches, photo, link, subscription, bot status, classifier, gate | VERIFIED | substantive, wired from rules table, executor, watcher, held release, fixed and page |
| `fuku/modules/memberreq_held.go` (88 lines) | held release | VERIFIED | called at `memberreq_enforce.go:619` before the held delete |
| `fuku/modules/memberreq_rules.go` | seven rows, `Lookup` field, `memberReqLookupMasks`, `memberReqUnknownDefers` | VERIFIED | one of `Broken`/`Lookup` per row (`TestMemberReqRuleTable`) |
| `fuku/modules/memberreq_enforce.go`, `memberreq_watch.go`, `memberreq_notice.go`, `memberreq_fixed.go` | lookups at step 6b, cached read, remedy tail, fresh-lookup press | VERIFIED | read in full above |
| `fuku/modules/settings_mreq.go` (715 lines) | seven-cell grid, status block, Off-only screen, link gate | VERIFIED | `settings_pages.go` not edited |
| `locales/*.yml` | 16 new and 7 reworded keys in 8 locales | VERIFIED | see truth 15 |
| `14-SPIKE.md` | owner's live checks with a pre-enable warning | VERIFIED | section "Before you enable Mute, Kick or Ban for the photo or channel rule in a real group" at line 12 |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `memberReqLiveViolation` | `memberReqLookupMasks` | OR-ed into the name mask before `memberReqStrictest` | WIRED | `memberreq_enforce.go:287-288`, `:637` |
| `memberReqFlightValue` | `memberReqContext()` | flight context derives from the shutdown root | WIRED | `memberreq_lookup.go:249` |
| `memberReqLookupPaced` | `ratelimit.NewTelegramPacer` | own `NextKey`/`BlockKey` | WIRED | `memberreq_lookup.go:87-101` |
| `memberReqEnforce` held branch | `memberReqHeldReleased` | before `memberReqDeleteHeld` | WIRED | `memberreq_enforce.go:618-623` |
| `memberReqWatchClean` | `memberReqLookupCached` | pipelined Redis read | WIRED | `memberreq_watch.go:259-276` |
| `memberReqFixedRun` | `memberReqLookupBroken` (Fresh, Classify) | step 5 under 5 s | WIRED | `memberreq_fixed.go:233-255` |
| `settingsMreqChoose`, `settingsMreqYes` | `settingsMreqChannelGate` | after same-value read and expiry check, before card or write | WIRED | `settings_mreq.go:579`, `:699` |
| `settingsMreqReadStatus` | `memberReqLinkedChannel`, `memberReqBotChannelStatus` | page reads the executor's facts through the lookup pacer | WIRED | `settings_mreq.go:216-223` |
| Makefile `test-postgres-integrity` | `TestMemberRequirementLookupMigrationDefaults`, `TestChatRequirementsSettings` | name in the `$$`-anchored `-run` list and `./fuku/db/memberreq` package | WIRED | `git diff 8c0c827 HEAD -- Makefile` |
| `cache.skipLocal` | `member_requirements:*` | settings key never served from process memory | WIRED | `fuku/db/cache/local.go` (Phase 13) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `memberReqPhotoRule` | `photos` | `b.GetUserProfilePhotosWithContext` through the lookup pacer | Yes (Telegram answer; the fake in tests) | FLOWING |
| `memberReqLinkedChannel` | `info.LinkedChatId` | `b.GetChatWithContext(chatID)` only | Yes | FLOWING |
| `memberReqChannelRule` | `answer` | `b.GetChatMemberWithContext(channel, user)` where `channel` came from the live link | Yes | FLOWING |
| `renderSettingsMreqPageStatus` | `status` | `settingsMreqReadStatus` (link and bot status via the caches or live) | Yes | FLOWING |
| `memberReqNoticeText` | `broken` mask | hold's `Rules`, set from `memberReqLookupMasks` | Yes | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Production code compiles | `CGO_ENABLED=0 go build ./...` | exit 0 | PASS |
| Vet incl. test files | `go vet -tags testtools ./fuku/modules ./fuku/db/memberreq` | exit 0 | PASS |
| Dependencies unchanged since the base | `git diff --exit-code 8c0c827 -- go.mod go.sum` | exit 0 | PASS |
| No new `t.Skip` in the phase's tests | `grep -rnE 't\.Skip(f\|Now)?\(' fuku/modules/memberreq*_test.go fuku/modules/settings_mreq*_test.go` | exit 1 (no match) | PASS |
| Only skips are the PostgreSQL-only migration test | `grep -rn t.Skip fuku/db/memberreq fuku/db/migrations/member_requirement_lookup_migration_test.go` | 2 matches in `TestMemberRequirementLookupMigrationDefaults`, allowed by `scripts/check_test_results/main.go:53-56` for the migrations package | PASS |
| Translations present | `make check-translations` | exit 0 ("All translations are present!") | PASS |
| Generated docs have no drift | `make check-docs` | exit 0 ("No drift detected") | PASS |
| Lint-local (dupl, gocyclo, godox) on added lines | `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` | exit 0 | PASS |
| Debt markers in production files | `grep -n "TODO\|FIXME\|XXX\|HACK\|TBD"` over the phase's production files | no match | PASS |
| Requested focused sample | `go test -tags testtools -race -count=1 -run '^(TestMemberReqLookupEndToEnd\|TestMemberReqDemotedBotNeverBreaches\|TestMemberReqExemptionsReadAfterLookups\|TestMemberReqLookupUnanswerable\|TestSettingsMreqChannelRefusal)$' ./fuku/modules` | `ok ... 53.667s`, exit 0 (read directly, not through a pipe) | PASS |
| Wider sample of 18 more named tests (shutdown, pacers, logs, classifier, singleflight, held release, fixed lookups, page status and cost, partial episode, stale link, live record, watcher) | `go test -tags testtools -race -count=1 -run '^(TestShutdownEndsMemberReqLookups\|TestMemberReqPacersAreIndependent\|...)$' ./fuku/modules -v` | 18 `--- PASS`, `ok ... 107.879s`, exit 0 | PASS |
| Package tests | `go test -tags testtools -race -count=1 ./fuku/i18n ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup` | all `ok`, exit 0 | PASS |
| `user-story.validate` of the ROADMAP goal | `gsd-tools query user-story.validate --pick valid` | `true` | PASS |

The full `make test` was not run (20 minutes, covered by CI), nor `make lint` or the PostgreSQL target (no server, older `golangci-lint`).

### Probe Execution

SKIPPED: the phase declares no `probe-*.sh` (no `scripts/*/tests/probe-*.sh` is referenced by any PLAN or SUMMARY of Phase 14).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| MREQ-01 | 14-05, 14-12 | A page gives each of eight rules its own punishment | Correctly PENDING | Seven of the eight rules exist and are settable on page `m` (truths 1, 2, 14; `memberreq_rules.go:103-118`). The eighth, the AI spam-name rule, is MREQ-14 in Phase 15. The tracker note (REQUIREMENTS.md line 154) says "seven of the eight" and stays Pending, as it should |
| MREQ-10 | 14-02..14-11 | An unanswerable check never punishes; the page shows when a rule cannot work (no linked channel, bot not admin, AI not configured) | Correctly PENDING | Delivered except the "AI not configured" line, which needs the AI rule (Phase 15, MREQ-14): no AI line exists in `settings_mreq.go`. Everything else is truths 3, 5, 6 and the status lines `settings_mreq_here_*`. Tracker note says "AI line through MREQ-14 in Phase 15" |
| MREQ-11 | 14-01..14-08, 14-12 | The photo rule treats a photo the bot cannot see as missing, and the page says so | SATISFIED (tick justified) | truth 1; `locales/en.yml:3267` |
| MREQ-12 | 14-01..14-12 | The channel rule is offered only in a group linked to a channel and checks subscription | SATISFIED (tick justified) | truths 2, 6, 7 |
| PLAT-06 | 14-03, 14-06, 14-07, 14-11 | New background work (deletion queue, name checks, requirement lookups) shares Telegram rate budgets across replicas, recovers from crashes, is drained within the shutdown budget | SATISFIED for the work that exists; tick is a bookkeeping judgment | The requirement lookups meet all three clauses (truths 4, 9, 10; panic recovery `memberreq_lookup.go:248`). The literal text also names the deletion queue (Phases 16 and 17) and name checks (Phase 15), which do not exist yet. ROADMAP maps PLAT-06 to Phase 14 and REQUIREMENTS.md line 190 declares it cross-cutting ("mapped to the first phase where they matter and bind every later phase"), the same convention under which PLAT-05 was ticked at Phase 10. The tick is consistent with that convention; the owner may prefer to hold it until Phase 17 (human item 7) |
| MREQ-02 | Phase 13 | check on join and on every message | SATISFIED (tick justified) | `memberReqJoinThenWelcome` reached from `greetings.go:1089` and `captcha.go:1747` (after the captcha pass); watcher group -2 |
| MREQ-03 | Phase 13 | Delete messages | SATISFIED | `memberReqDeleteMessage`, `memberReqDeleteHeld` (`memberreq_enforce.go:668-723`) |
| MREQ-04 | Phase 13 | punished once across replicas | SATISFIED | insert-first `ClaimHold` (`memberreq_enforce.go:345-376`); `TestMemberReqOnePunishmentAcrossReplicas`, `TestMemberReqLookupOnePunishmentAcrossReplicas` |
| MREQ-05 | Phase 13 | one notice naming the rule, warn counts toward the limit | SATISFIED | `memberreq_notice.go:134-142`, `memberReqWarnStep` through `applyWarn` |
| MREQ-06 | Phase 13 | "I fixed it" for the muted member | SATISFIED | `memberreq_fixed.go` |
| MREQ-07 | Phase 13 | only lifts a requirement mute | SATISFIED | `isMemberReqMute` (`memberreq_enforce.go:814-816`) checked first (`memberreq_fixed.go:145`) |
| MREQ-08 | Phase 13 | kicked rejoins and is checked again, banned stays banned | SATISFIED | `ClearHolds` on leave (`greetings.go:1005`); nothing unbans |
| MREQ-09 | Phase 13 | admins, approved, bots, service accounts, channel and anonymous senders never checked | SATISFIED | `raidFloodSenderCounts` (`raid_flood.go:199-213`), `memberReqNeverChecked`, `memberReqExempt`, `memberReqLiveEligible` |
| MREQ-13 | Phase 13 | name rules by letters, page says so | SATISFIED | `memberreq_rules.go:54-97`; `locales/en.yml:3246` |

Ticks of MREQ-02..09 and MREQ-13 date from Phase 13 and were re-confirmed here by reading the code paths above; the Phase 13 report holds the full evidence. No requirement mapped to Phase 14 (MREQ-10, MREQ-11, MREQ-12, PLAT-06) is orphaned: all four IDs appear in plan frontmatter. MREQ-01, which no plan completes alone, is listed in plans 14-05 and 14-12.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| `AGENTS.md` | 1456 | "its one production caller is `memberReqLiveViolation`" said of `memberReqLookupBroken` | Info | Inaccurate. `memberReqLiveViolation` calls its sibling `memberReqLookupMasks` (`memberreq_enforce.go:287`); the only production caller of `memberReqLookupBroken` is `memberReqFixedJudge` (`memberreq_fixed.go:244`). No behavioural effect. Fix: say the executor calls `memberReqLookupMasks` and "I fixed it" calls `memberReqLookupBroken` |
| `AGENTS.md` | 1267-1271 | `ba` bullet opens "The breach gate below and the settings page's status line read it", then says the gate never reads it | Info | Self-contradicting wording. The code is the second half: `memberReqGateBreach` forces `mode.Fresh = true` (`memberreq_lookup.go:643-646`), so it writes `ba` but never reads it. Fix: drop "the breach gate" from the first sentence |

No TODO, FIXME, XXX, HACK or TBD in any production file of the phase. No stub, empty handler or hard-coded empty data was found in the phase's production code.

### Human Verification Required

See the `human_verification` frontmatter (7 items). In short: spike S1 to S3 and the related live checks (before enabling a punishment), the phone-width read of the new cells, the native-speaker read of 16 new and 7 reworded keys, two live replicas, a raid and a restart, CI on HEAD (lint, `make test`, the PostgreSQL integrity target), and the owner's review of the judgment-tier prohibitions and deferred items.

### Gaps Summary

There are no gaps: every ROADMAP success criterion and plan must-have is delivered, wired and proved by tests that assert observable behaviour (stored rows, Telegram calls made or not made, notices sent, caches written or absent).

Things the owner should know, none of them blocking:

- **Two intentional deviations from plan wording, both documented in AGENTS.md and the review disposition.** (1) Plan 14-03 said "no verdict other than a pass is ever cached"; WR-01 added the channel-level `cna` fact (the bot is not an administrator of this channel, 60 s, keyed by channel only). It can only turn an unanswerable lookup into inert, never into a breach. (2) Plan 14-06 said an unknown lookup "never blocks an established name breach"; WR-02 added `memberReqUnknownDefers`: when a rule that could not be answered has a stricter punishment than the broken rules, nothing is claimed and the member is judged again at their next message. This errs toward punishing less, never more. The accepted cost is NEW-01 (a persistently unanswerable stricter rule holds the weaker punishment back).
- **Adversarial results.** Unanswerable path: 28 failure shapes through the job and the join entry leave no hold, warn, write or notice, and the join entry still welcomes. Stale link: a breach candidate re-reads the link fresh and then the bot's standing fresh (a cached administrator standing is never believed, SEC-03), and an exemption that appears during a slow lookup phase is honoured (SEC-02). Rulebook: every cited test exists and every behavioural claim spot-checked matches the code, except the two wording slips above.
- **Accepted limits stay accepted** (review disposition): IN-01, IN-02, IN-04, IN-05, NEW-01 to NEW-03, SEC-01 and SEC-04 are deferred, none blocks the phase, and SEC-04 (no per-person limit on presses of the channel screen or "I fixed it", one 32-job runner per replica) is the one the owner may want to schedule.

---

_Verified: 2026-11-08T14:30:00Z_
_Verifier: Claude (gsd-verifier)_
