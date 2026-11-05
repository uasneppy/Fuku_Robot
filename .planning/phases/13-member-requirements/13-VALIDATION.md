---
phase: "13"
slug: "member-requirements"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-11-05"
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `13-RESEARCH.md` § Validation Architecture; the test names are the plans' contract (13-01 to 13-14, revised 2026-11-05), and the executors set the status.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, testify assertions, miniredis, hand-written `gotgbot.BotClient` fakes (`staffActionFake`/`lockdownFake` with per-user profiles from 13-07), SQLite via `internal/testdb.Run`, no mock libraries |
| Config file | none; `Makefile` targets `test` (15 min per package) and `test-postgres-integrity` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestMemberReq\|TestSettingsMreq\|TestSettingsAuthorityMreq\|TestSettingsDriftMreq\|TestSettingsControlActionAllowList)' ./fuku/modules ./fuku/i18n` plus `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/approvals ./fuku/db/cache ./fuku/db/backup` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |
| Enforcement switch | `memberReqEnforcementLive` is false from 13-09 to 13-12 and true from 13-13 on; tests that drive the watcher or the join entry turn it on with `withMemberReqLive(t, true)` (`newMemberReqEnv` does it) and the off tests set it false |

---

## Sampling Rate

- **Per task commit:** the focused `-run '^(...)$'` of the task's tests (seconds each), plus the package run of any `fuku/db/*` package the task touched.
- **Per plan end:** the plan's family run in its `<verification>` block (the settings family `^(TestSettings|TestLoadSettings)` over `./fuku/modules ./fuku/i18n` takes about 4 minutes and runs only at the end of 13-05 and 13-06; the `TestMemberReq` family at the end of 13-07 to 13-13).
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/...` (about 11 minutes for `fuku/modules` here) plus the lint stand-in `python3 .planning/phases/11-message-editor/handoff/lint-local.py 8c0c827` (an invariant gate: it exits 0 at HEAD and fails only on lines the phase added).
- **Phase gate:** `make test` (or the documented sandbox fallback of 12-16-SUMMARY), `make lint` (or its fallback), `make check-translations`, `make generate-docs` then `make check-docs`, `git diff --exit-code 8c0c827 -- go.mod go.sum`, CI `make test-postgres-integrity`, and the owner's live checklist `13-SPIKE.md` (deferred like Phases 7 to 12 unless the owner says otherwise).

---

## Per-Requirement Verification Map

Every command below is `go test -tags testtools -race -count=1` with the given `-run` and package. Test function names are the plans' contract. "Completes" names the one plan that completes the requirement.

| Requirement | Behavior | Plan | Test Type | Automated Command | File Exists | Status |
|---|---|---|---|---|---|---|
| MREQ-13 (completes: 13-09) | Unicode corpus (Ukrainian pass; ы ъ э ё after NFC; Belarusian and Bulgarian mixed; Arabic letters; Han; zero-width inside); invisible last name; username rule; rule table | 13-02 | unit (pure) | `-run '^(TestMemberReqRulesCorpus\|TestMemberReqBlankLastName\|TestMemberReqUsernameRule\|TestMemberReqRuleTable)$' ./fuku/modules` | ❌ W0 (corpus first) | ⬜ pending |
| MREQ-13 | The letters literal and the related languages named in the page hints in 8 locales | 13-04 | locale | `-run '^TestMemberReqLocaleKeys$' ./fuku/i18n` | ❌ W0 | ⬜ pending |
| MREQ-13 | Every member's first and last name checked at their message (behind the switch) | 13-09 | integration | `-run '^(TestMemberReqWatcherPunishesOnce\|TestMemberReqPassCache)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-01 (completes: 13-06, five of the eight rules) | Storage: CHECK 0..4, column-targeted upserts, a Go 0 and false written, no row means all off, fresh read | 13-01 | db (+ PostgreSQL) | `-run '^TestChatRequirementsSettings$' ./fuku/db/memberreq` (CI also runs it on PostgreSQL) | ❌ W0 | ⬜ pending |
| MREQ-01 | Page `m` on Main (`r\|m` row, Off / `{n} on` / unknown), the page body and hints, the switches, no row created, privacy of the page | 13-05 | integration | `-run '^(TestSettingsMreqOpensFromMain\|TestSettingsMreqMainState\|TestSettingsMreqSwitches\|TestSettingsStaffPrivacy)$' ./fuku/modules` | ❌ / ✅ extended | ⬜ pending |
| MREQ-01 | Rule screens, Off and Warn at once, the card for Kick, Ban and Mute (four exits, expiry, demotion, same value read fresh), `st` only on rule controls | 13-06 | integration | `-run '^(TestSettingsMreqRuleScreen\|TestSettingsMreqCard\|TestSettingsControlActionAllowList)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-01 | Rights, drift, bytes, row budget (5/3/2), 8-language fit, privacy of rule screens and cards | 13-05, 13-06 | integration | `-run '^(TestSettingsAuthorityMreq\|TestSettingsDriftMreq\|TestSettingsMreqCallbackBytes\|TestSettingsMreqRowBudget\|TestSettingsMreqPageFitsEveryLocale\|TestSettingsStaffPrivacy)$' ./fuku/modules` | ❌ / ✅ extended | ⬜ pending |
| MREQ-02 (completes: 13-11) | Check on join switch; the message check; the join check without a captcha and after both captcha passes; nothing while the switch is off; no getChatMember when Check on join is off | 13-05, 13-09, 13-11 | integration | `-run '^(TestSettingsMreqSwitches\|TestMemberReqWatcherPunishesOnce\|TestMemberReqWatcherOffUntilLive\|TestMemberReqJoinNoCaptcha\|TestMemberReqJoinOffUntilLive\|TestMemberReqJoinAfterCaptcha)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-03 (completes: 13-10) | Delete messages: the violating message, held members' messages, after a refused punishment, during a pause, a failed delete only logged; the cap and pause, with and without Redis | 13-10 | integration | `-run '^(TestMemberReqDeleteMessages\|TestMemberReqDeleteDuringPause\|TestMemberReqCapPausesPunishments\|TestMemberReqCapWithoutRedis)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-04 (completes: 13-12) | Insert-first claim: 16 concurrent claims one winner; stale reclaim one winner; take, restore, clean, clear, revert | 13-01 | db (+ PostgreSQL) | `-run '^(TestRequirementHoldClaimConcurrent\|TestRequirementHolds)$' ./fuku/db/memberreq` (CI also runs both on PostgreSQL) | ❌ W0 | ⬜ pending |
| MREQ-04 | One punishment per episode; the warn and the hold mark inseparable; held path; backoff; ten messages on three replicas; clean ends a warn hold; join plus message once; rejoin resets | 13-07, 13-08, 13-09, 13-12 | integration | `-run '^(TestMemberReqEnforceWarnOnce\|TestMemberReqWarnMarkInsideLock\|TestMemberReqPunishRefused\|TestMemberReqOnePunishmentAcrossReplicas\|TestMemberReqCleanEndsWarnHold\|TestMemberReqJoinAndMessageOnce\|TestMemberReqRejoinResetsMuteHold)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-05 (completes: 13-11) | One notice per episode, the strictest punishment, every broken rule not Off in rule order, warn count and limit, the limit variants, the group's language, the mention escaped and isolated, every notice within 400 code points | 13-07 | integration | `-run '^(TestMemberReqEnforcePunishments\|TestMemberReqEnforceStrictest\|TestMemberReqNoticeWarnLimit\|TestMemberReqNoticeLanguage\|TestMemberReqScreensFitEveryLocale)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-05 | A notice is retried, never dropped by pacing; the join path posts its notice | 13-08, 13-11 | integration | `-run '^(TestMemberReqMuteNoticeRetry\|TestMemberReqJoinNoCaptcha)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-05 | Warn through the warn module's own path; every warn command byte-identical (characterization run of the 32 named warn and blacklist tests, before and after) | 13-03 | integration + characterization | `-run '^(TestApplyWarn\|TestKickMemberContext)$' ./fuku/modules`; the anchored 32-name run of 13-03 Task 1 | ✅ characterization / ❌ new | ⬜ pending |
| MREQ-05 | Pure strictest choice and mention | 13-02 | unit (pure) | `-run '^(TestMemberReqStrictest\|TestMemberReqPassValue\|TestMemberReqMention)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-06 (completes: 13-13) | A mute that cannot carry its button is lifted (lockdown-aware) and left as a backoff | 13-08 | integration | `-run '^(TestMemberReqMuteNoticeUndeliverable\|TestResolveUnmutePermissionsWithLockdown)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-06 | I fixed it: presser only, matched by hold, chat and member (also before the notice ID is stored), still broken naming the rules, restore with the same ID, two taps on two replicas, lockdown permissions, the held welcome once and Greet once, group language, bytes; enforcement on with the handler registered | 13-13 | integration | `-run '^(TestMemberReqFixedUnmutes\|TestMemberReqFixedBeforeNoticeStored\|TestMemberReqFixedRefusals\|TestMemberReqFixedDuringLockdown\|TestMemberReqFixedRestoreOnFailure\|TestMemberReqFixedTwoReplicas\|TestMemberReqFixedHeldWelcome\|TestMemberReqFixedCallbackBytes\|TestMemberReqEnforcementLive)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-07 (completes: 13-13) | Never lifts an admin, /tmute, staff, captcha, antiflood or warn-limit mute, nor a member no longer restricted; a captcha pass re-applies the marker; an unknown mute answer decided by the marker | 13-13, 13-11, 13-08 | integration | `-run '^(TestMemberReqFixedNeverLifts\|TestMemberReqCaptchaGuard\|TestMemberReqPunishUnknown)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-08 (completes: 13-12) | Kick is the member-removing unban; the leave clears the hold; the rejoin is checked again; a ban stays | 13-03, 13-12 | integration | `-run '^(TestKickMemberContext\|TestMemberReqKickBan)$' ./fuku/modules` | ❌ | ⬜ pending |
| MREQ-09 (completes: 13-11) | Admins (cached and live), approved (tri-state reader), bots, the bot, service accounts, sender_chat (channel, anonymous admin), automatic forwards, pending captcha, edited messages, private chats, basic groups, service messages; unknown states never punish; ContinueGroups on every path | 13-07, 13-09, 13-11 | integration | `-run '^(TestMemberReqEnforceExemptions\|TestMemberReqWatcherExemptions\|TestMemberReqWatcherContinuesGroups\|TestMemberReqJoinExemptions)$' ./fuku/modules`; `-run '^TestIsUserApprovedContext$' ./fuku/db/approvals` | ❌ | ⬜ pending |
| D-17 | Migration statements (db-free); both tables out of backup, export, import and reset | 13-01, 13-14 | unit + db | `-run '^TestMemberRequirementsMigrationStatements$' ./fuku/db/migrations`; `-run '^TestMemberRequirementsStayOutOfBackup$' ./fuku/db/backup` | ❌ | ⬜ pending |
| D-20 | The settings key in `skipLocal`; the runner sheds when full (logged); the drain cancels and waits at most 2 s inside a documented budget | 13-09 | unit + integration | `-run '^(TestSkipLocalMemberRequirementsKey\|TestSkipLocalMatchesFreshnessCriticalKeys)$' ./fuku/db/cache`; `-run '^(TestMemberReqRunnerSheds\|TestShutdownWaitsForMemberReqJobs)$' ./fuku/modules` | ❌ | ⬜ pending |
| UI / locale | 36 `settings_` and 25 `mreq_` keys in 8 locales, label ceilings (grid 20, cells 20, choice cells 13/15), help fits one message | 13-04 | locale | `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsHelpFitsOneMessage\|TestMemberReqLocaleKeys)$' ./fuku/i18n` | ✅ extended / ❌ new | ⬜ pending |
| Privacy | No Phase 13 screen names the Staff Group (group menu, non-creator private menu, 8 languages, a private run); answers within 200 code points | 13-05, 13-06, 13-14 | integration | `-run '^(TestSettingsStaffPrivacy\|TestMemberReqScreensFitEveryLocale)$' ./fuku/modules` | ✅ extended / ❌ new | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `fuku/db/memberreq/testmain_test.go` with `&models.ChatRequirements{}` and `&models.ChatRequirementHold{}`; `TestChatRequirementsSettings` and the hold tests written red first (13-01).
- [ ] Makefile: `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent`, `TestRequirementHolds` in the anchored `-run` list and `./fuku/db/memberreq` in the package list of `test-postgres-integrity` (13-01).
- [ ] The Unicode corpus `TestMemberReqRulesCorpus` committed red before the classifier (13-02).
- [ ] The 32 named warn and blacklist tests run green and recorded before `warns.go` changes (13-03, the characterization run).
- [ ] `fuku/i18n/memberreq_locale_test.go` (`TestMemberReqLocaleKeys`) written before the keys (13-04).
- [ ] `fuku/modules/test_harness_test.go` AutoMigrate gains both models and `settingsCleanup` both tables and the `member_requirements` key, in the same plan as page `m`; the hand-built `mainEntries` of `TestSettingsStaffPrivacy` gain page `m` (13-05).
- [ ] `fuku/modules/staff_action_fake_test.go`: a per-user profile (first name, last name, username) reported in every `getChatMember` answer, kept across `restrictChatMember` (13-07).
- [ ] `fuku/modules/memberreq_env_test.go`: `memberReqEnv`, `mreqMember` (fresh `uniqueLinkOwnerID()` members), `memberReqCleanup` (both tables, warns, warn settings, approvals, users rows and `user` cache keys of every member it minted, the `member_requirements` and `approvals` keys, the `fuku:memberreq:*` keys, `ResetLocalForTest`) (13-07), extended by 13-09 (`withMemberReqLive`, `mreqSend`, `mreqSendAt`, `mreqReplica`) and 13-13 (`mreqPressFixed`).
- [ ] `fuku/db/backup/testmain_test.go` gains both models for `TestMemberRequirementsStayOutOfBackup` (13-14).
- [ ] AGENTS.md lines in the same commit as each rule change (13-01, 13-05 to 13-13), finished in 13-14.

---

## Manual-Only Verifications

Covered by the owner's live checklist `13-SPIKE.md` (written by plan 13-14; deferred like Phases 7 to 12 unless the owner says otherwise). None blocks the phase.

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Telegram's per-group message limit (about 20 a minute) and the first 429's retry_after; the cap of 10 punishments a minute leaves room | MREQ-05 | Live API rate limits (core.telegram.org unreachable during research, A1) | Send 25 notices to a test group within a minute and record the first 429 |
| A 330-day restriction reads back with an until_date within 2 s of the value sent | MREQ-06, MREQ-07 | Live API behaviour (A2) | Mute a test account for 330 days and read getChatMember |
| A restriction survives a leave (restricted, is_member false); the leave and rejoin are restricted-to-restricted chat_member updates and the rejoin sends a new_chat_members service message; the join reset lifts the mute | MREQ-04, MREQ-08 | Live API behaviour (A3, A4) | Mute, leave, read getChatMember, rejoin, read again, log the updates |
| During a lockdown a member unmuted with all rights still cannot send | MREQ-06 | Live API behaviour (A5) | Lock a test group, press I fixed it as a fixed member, try to send |
| A user with only a collectible username has User.username set | MREQ-01 | Needs a Fragment username account (A6) | Join with such an account under the username rule |
| getChatMember returns the current names right after a rename; a callback stays answerable for the flow's 10 s | MREQ-06 | Live API behaviour (A7, A8) | Rename a test account and press I fixed it at once; delay an answer in a test bot |
| An Arabic name in a notice does not reorder the sentence; the cells and choice rows in hi, ru and uk are not ellipsized | MREQ-05, MREQ-01 | Real-client rendering | Post a notice for an Arabic-named account and open the page on Android, iOS and Desktop |
| Two live replicas over one PostgreSQL: ten messages give one punishment; a join and a first message give one | MREQ-04 | Two real replicas | Run two bot instances against one database and post from a violating account |
| Native-speaker read of the 61 new strings and the changed help in es, fr, hi, id, pt, ru, uk | MREQ-01, MREQ-05, MREQ-06, MREQ-13 | Wording quality is human | Read each new `settings_mreq_*`, `settings_btn_mreq_*` and `mreq_*` key |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency: a focused `-run` under 60 s; the settings family (about 4 minutes) and the `fuku/modules` package (about 11 minutes) only at plan end or wave merge
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
