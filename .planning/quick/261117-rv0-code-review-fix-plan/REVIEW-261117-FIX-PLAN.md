# Xenobot-72 fix plan: code review of 2026-11-17

Repository `uasneppy/Xenobot-72`, base commit `0f9e587` (origin/main), branch `claude/jolly-mendel-fys9ve`.
Line numbers refer to `0f9e587` and drift as fixes land. Status: **V** verified in code by the orchestrator,
**R** reviewer read it end to end, **P** plausible. Every item is a hypothesis until a failing test proves it.

## 0. Ground rules

1. GSD: each wave is one quick task under `.planning/quick/261117-<id>-<slug>/` with a PLAN.md and a SUMMARY.md.
2. Follow `AGENTS.md` (grep it for the subsystem): `helpers.WrapCommand`; raw command handlers only through
   `helpers.NewBotCommand`; every write `cache.DeleteCache`s its keys; locale keys in all 8 files; every new `settings_`
   key needs a row in `fuku/i18n/settings_locale_test.go`, every `mreq_` key a row in `memberReqLocaleLimits`;
   fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic`; migrations append-only, timestamp after
   `20261116120000`, one transaction, accepted by both appliers; watchers return `ext.ContinueGroups` (`nil` ends the
   current group only). **Update `AGENTS.md` in the same commit** as any change that makes a sentence of it false.
3. Reproduce first: a test that fails on the current code, real fixtures only (`newSettingsEnv`, `newMemberReqEnv`,
   `newStaffActionEnv`, lockdown fakes, `internal/testdb`, miniredis), the real dispatcher where routing matters. If the
   test passes on current code, change nothing and record the item as "Not reproduced" with a one-line reason.
4. Never re-record or weaken a `TestLegacy*Requests` golden. A fix that would change pinned bytes is not shipped and is
   listed under "Owner decisions" with the exact difference.
5. No `t.Skip`; fresh IDs (`uniqueModuleChatID()`, `uniqueLinkOwnerID()`); clean rows and keys in `t.Cleanup`; keep tests
   light (`fuku/modules` is near its CI time limit).
6. One Conventional Commit per item (or tightly related pair): `fix(<area>):`, `docs:`, `ci:`/`build:`. No model names.
7. Per item: `go test -tags testtools -race -count=1 -run '^TestName$' ./<pkg>` and `CGO_ENABLED=0 go build ./...`.
   Per wave: `make lint`, `make check-translations`. At the end: `make check-docs`, `make test` (or `test-rest` plus
   the four shards), the captcha page node tests if the page changed, `make test-postgres-integrity` if PostgreSQL exists.
8. Done: branch pushed, draft PR listing fixed IDs, "Not reproduced", "Owner decisions", behaviour changes; CI green.

## 1. Owner decisions (do not implement; list in the PR)

- **R4** variation selectors in names (`namecanon/canon.go:134`): fixing changes the canonical form, forcing `mrn1`/`mrv1`
  bumps and a one-time re-judgement of every established member's names.
- **F-intent** `fbanChanged` skips the re-sweep on a repeated reason (`federations.go:762`), documented intent;
  `applyActiveUnfban` lifts any ban (accepted in a comment). No change.
- **S3-ban** the ban half of S3 (`/tban` on a member restricted for longer): document as an accepted limit in AGENTS.md.
- **I9** cross-replica stale cache write: only if a small Lua compare-and-set on a Redis-side generation fits; otherwise
  document as an accepted limit.
- Any fix blocked by a golden (rule 4).

## 2. Authorised user-visible behaviour changes

A1 automatic forwards never run a command; M1 `/dban` `/dmute` `/dwarn` refuse when the explicit target is not the
reply's sender; N6 `/logchannel` admin-only; N7 `/export` goes to the admin's private chat; N5 `/setrules` refuses a reply
with no text or caption; T4 `/anonadmin` on/off texts corrected; C7 captcha web links valid for the attempt's lifetime.

## Wave 1: command authority

- **A1 [High, V]** Automatic forwards from the linked channel (`from` 777000, `is_automatic_forward`) pass
  `RequireUserAdmin`, `IsUserConnected(chatAdmin)`, `chat_status.RequireUserAdmin` because `tgAdminList` holds 777000
  (`chat_status.go:37,235`). Fix in `helpers.NewBotCommand`: wrap the response so a message with `IsAutomaticForward` or
  `From.Id == 777000` returns `ext.ContinueGroups` at once, no reply, no I/O; library handler type unchanged
  (`TestNoRawNewCommand`, `TestBotCommandNameMatchesGotgbot` hold). Covers `setlog` (`logchannels.go:331-353`) and
  `setfedlog` (`federations.go:1028`). Check `CommandDeletionProcessor` skips automatic forwards
  (`raidFloodSenderCounts`). Leave `tgAdminList`; comment plus AGENTS.md sentence. Tests via the dispatcher: auto-forward
  `/approve <id>` makes no approval and no reply; `/setlog` auto-forward binds nothing; `/setfedlog` auto-forward gets
  nothing; manual forward by a real admin still binds (`TestSetLogFlowThroughDispatcher` unchanged).
- **A2 [Low, R]** `lockPerm`, `unlockPerm` (`locks.go:290,364`), `setWarnMode`, `setWarnLimit` (`warns.go:133,654`) call
  `RequireBotAdmin(b, ctx, nil)` after `IsUserConnected`; pass the connected chat. Goldens unchanged.
- **A3 [Low-Medium, R]** `/setlog` step 2 consumes the marker before `logchannels.Set`; on failure put it back
  (`storePendingMarker`) and reply `common_settings_save_failed`. `consumePendingMarker`'s no-Redis fallback
  (`logchannels.go:71`) lacks the lock `consumeOverwriteCache` has; reuse that helper or add the mutex.

## Wave 2: captcha

- **C1 [Medium, V]** `createCaptchaAttemptPreMessage` (`db/captcha/repository.go:173`) writes `ExpiresAt` with nanoseconds;
  PostgreSQL keeps microseconds; `scheduleCaptchaTimeout` (`captcha.go:1372,1416`) compares `.Equal` and treats the timer
  as stale. Write `time.Now().UTC().Add(...).Truncate(time.Microsecond)` at every `expires_at` write; compare truncated on
  both sides; grep other `.Equal(` on DB times (mirror `db/lockdown/repository.go:37`). Test: in-memory attempt with
  non-zero nanoseconds vs microsecond-truncated stored row applies the action; created `ExpiresAt.Nanosecond()%1000==0`.
- **C2 [Medium, R]** `handleCaptchaTimeout` (`captcha.go:1546-1554`): after a failed kick/ban/mute, read live
  `getChatMember`: kicked/left = applied; a definite 4xx other than 429 keeps release-and-unmute (WR-07); 429, 5xx,
  timeout, unreadable leaves the member muted, one Warn by IDs; never a member-removing unban after a ban attempt; keep
  `ActionResult` consistent; update WR-07 text in AGENTS.md.
- **C3 [Medium, P]** `unmuteExpiredCaptchaUsers` (`captcha.go:2300-2345`) restricts from a stale snapshot; read the live
  member first: restrict only member/restricted, delete the row for kicked/left.
- **C4 [Low, P]** `runOrphanedCaptchaRecovery` (`captcha.go:269-276`) releases every unexpired `message_id <= 0` attempt;
  only those whose `created_at` is older than 2 minutes.
- **C5 [Low, R]** `captchaweb/verify.go:611-614` with `captcha_web.go:424-431`: a verify with the attempt gone answers
  `expired` with the removal action even when the member was released; answer `expired`+action only when the live member
  is kicked/left, else neutral `invalid`; the replay marker stores the same.
- **C6 [Low, R]** `restoreCaptchaPermissions` (`captcha.go:1654`): a `DeleteMutedUser` error after a successful unmute is
  logged, not returned as an unmute failure.
- **C7 [Low, R]** initData older than 5 min is terminal `invalid` (`verify.go:387`, `initdata.go:93`) while attempts last up
  to 10 min; raise to max captcha time + 1 min (11 min); update page tests if they pin 5.
- **C8 [Low, R]** `startCaptchaLifecycleTask` (`captcha.go:197-200`): add `defer error_handling.RecoverFromPanic`.
- **C9 [Docs, R]** AGENTS.md says group -10 stores pending messages "for replay"; the pass only posts a summary. Fix.

## Wave 3: moderation commands and reply targets

- **M1 [Medium, V]** `/dban`, `/dmute`, `/dwarn` (`bans.go:307-330`, `mute.go` ~158, `warns.go:304-308,444`): explicit
  target beats reply so the wrong message is deleted; `/dwarn` deletes raw `ReplyToMessage`. Use `extraction.ReplyTarget`;
  when an explicit target differs from the reply's sender refuse with new key `common_reply_target_mismatch` (8 locales),
  nothing punished or deleted.
- **M2 [Medium, V]** `/ban` and `/unban` channel branch (`bans.go:191-196,379-384`) uses the reply's sender; ban/unban the
  extracted `t.userID`, `ReplyTarget` only for a display name when the reply's sender chat equals that ID; keep the hint.
- **M3 [Medium, R]** Report card Kick (`reports.go:576`), `/restrict` card Kick (`bans.go:612`) and Mute (`bans.go:622`),
  unrestrict card Unmute (`bans.go:808-826`), `/zombies` (`misc_extras.go:111-114`) act with no live status check; read the
  live member and act only on member/restricted (existing "not in group / already banned" text, else a key); `/zombies`
  also needs `CanUserRestrict`.
- **M4 [Medium, R]** `ExtractUserAndText` (`extraction.go:173-176`) splits `msg.Text` on a single space; use `GetText()`
  and split at the first run of Unicode white space, reason intact; explicit target beats reply there too. Table test of
  text, caption, newline, tab. Check `/warn`, `/mute`, `/approve` callers.
- **M5 [Medium, R]** `checkWarnSettingsContext` (`db/warns/repository.go:44-49,301`) turns a DB error into cached
  defaults; return the error, never cache a fallback; `applyWarn` fails closed (`common_settings_save_failed`, nothing
  punished). Golden unchanged.
- **M6 [Low-Medium, R]** `/report`, the group-8 `@admin` watcher (`reports.go:106-120,189`) and `/reports block|unblock`
  (`:398,:421`) read the raw reply; use `ReplyTarget`. The `@admin` watcher returns `ext.EndGroups` via `report()`; make it
  return `nil` after reporting so groups 9-12 run.
- **M7 [Low, R]** `ReplyTarget` in `/pin` (`pins.go:73,304,597`), `/unpin` (`:79`), `/permapin` via `GetPinType`
  (`:222,512`), `/tr` (`misc.go:378`).
- **M8 [Low-Medium, R]** unrestrict button (`bans.go:859-866`) re-inserts `EffectiveMessage.Text` unescaped into an HTML
  edit; rebuild from entities or escape; answer the callback even when the edit fails.
- **M9 [Low, R]** unescaped HTML: `/title` success (`admin.go:572`), `/tr` result (`misc.go:437`), connection titles
  (`connections.go:47,127`, `/reconnect`, `connectionButtons`), group `/notes` list (`notes.go:595`).
- **M10 [Low, R]** users tracker (`users.go:161,164`) records chat IDs as users; skip `sender_chat`, non-positive IDs,
  777000 and 1087968824.
- **M11 [Low, R]** `defer error_handling.RecoverFromPanic` in `spawnAsyncUpdate` (`users.go:27`) and purge workers
  (`purges.go:130`).
- **M12 [Low, R]** warn Rules button (`warns.go:348`) gated on `RulesSettings.HasContent()`.
- **M13 [Medium, V]** `applyActiveFban`/`applyActiveUnfban` (`federations.go:771-805`) loop unpaced and log at Debug; pace
  through `ratelimit.NewTelegramPacer` with keys `fuku:fed:pace:next`, `fuku:fed:pace:block`; log failures at Warn by IDs;
  append "N of M groups failed" to the federation log notice; `federations.Unfban`'s DB error (`:742`) answers
  `common_settings_save_failed`, not "not banned". Document the keys in AGENTS.md (Data).
- **M14 [Low-Medium, V]** `LOWER(username) = LOWER(?)` (`db/user/repository.go:155`) cannot use `idx_users_user_name`; add
  `migrations/20261117120000_users_username_lower_index.sql` with
  `CREATE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username));`; drop the username from the log at `:160`.

## Wave 4: notes, filters, greetings, rules, backup

- **N1 [Medium, V]** `notesWatcher` (`notes.go:879-980`, group 0) returns `ext.EndGroups` after answering; return `nil`
  after a note was sent or refused, keep `ContinueGroups` when none matched; `ReplyTarget` for `replyMsgId`. Dispatcher
  test: a `url` lock and note `rules`; a member's `#rules https://x.example` is deleted by the lock.
- **N2 [Medium, V]** connected `/get` (`notes.go:992`, `media.SendNote` `sender.go:271`), `/welcome`, `/goodbye`
  (`greetings.go:517,531`) post into the group; send to `msg.Chat.Id` replying to the command (as `sendNoFormatNote`).
- **N3 [Medium, V]** `importGreetings` (`db/backup/backup.go:694`) keeps `welcome_last_msg_id`/`goodbye_last_msg_id`; zero
  them on import and leave them out of the export.
- **N4 [Medium-Low, R]** `importLogChannels` (`backup.go:1111`) imports `log_channel_id`; never import it; apply category
  flags only when the target already has a log channel, else skip and say so. Imported filters, blacklist words, note
  names skip `keywordTooBroad`, `noteNameInvalid`, button-URL validation, lower-casing; validate, skip invalid, report a count.
- **N5 [Medium, R]** `/setrules` (`rules.go:406`) reads the raw reply and only `.Text`; use `ReplyTarget` and text or caption
  with formatting; refuse a reply with neither (new key, 8 locales).
- **N6 [Low, V]** `/logchannel` (`logchannels.go:164-184`) needs `RequireUserAdmin` with the usual responder.
- **N7 [Low-Medium, R]** `/export` (`backup.go:262`) sends to the group; send to the admin's private chat; on 403 reply in the
  group asking them to start the bot (new key, 8 locales), send nothing to the group.
- **N8 [Low, R]** filter `noformat` path (`filters.go:857`): for a non-admin ignore the suffix and answer normally; never end
  the update from the watcher.
- **N9 [Low, R]** `content.go:85,199` `strings.Fields(msg.Text)[1:]` panics for a captioned command; `GetText()` and guard.
- **N10 [Low, R]** `/privnote` (`notes.go:487,493`) replies `common_settings_save_failed` on a write error.
- **N11 [Low, R]** `/save` (`notes.go:304`), `/filter` (`filters.go:349`) use `ReplyTarget`.
- **N12 [Low, R]** `hi.yml:668-670` blacklist and forward-block mute/kick/ban templates put the reason first; use explicit
  indexes `%[2]s ... %[1]s`; check other locales.

## Wave 5: lockdown

- **L1 [Medium, V]** `startLockdownRow` (`lockdown_start.go:163-174`) ignores `ConfirmLocked`'s `won`; when false re-read the
  row; lifting/lifted replays `pre_permissions` and returns a distinct outcome mapped to the "already lifted" text. Test
  with a fake whose `setChatPermissions` lifts the row before answering.
- **L2 [Medium, P]** `lockdownBanOne` (`lockdown_worker.go:437-443,510,516`) treats 5xx/timeouts as not banned; at the lift
  route `ban_failed` rows and `pending` rows with `attempts > 0` to the live-checked unban (state `banned`). AGENTS.md.
- **L3 [Medium-Low, V]** forged Cancel/Confirm (`lockdown_alert_callbacks.go:733-747`, `:289-291`) edit the alert copy;
  require the pressed message to be a reply to the copy row's message ID and not the copy itself, else "no longer valid".
- **L4 [Medium-Low, P]** `lockdownUnbanOne` (`lockdown_worker.go:558-589`): live read and unban in one paced unit.
- **L5 [Low-Medium, P]** `decideLockdownJoin` (`lockdown_guard.go:74-76,99-103`): exempt only a human creator/administrator
  (performer not `is_bot`) or the anonymous admin.
- **L6 [Low, R]** `ListRemindableFresh(lockdownWorkerBatch)` (`lockdown_alert.go:393`) only covers the 50 lowest IDs; page
  through all due lockdowns across cycles.
- **L7 [Low, R]** inline lift (`lockdown.go:613-626`): when `liftLockdownRow` wins `FinishLift` itself, post the same tally.
- **L8 [Low, P]** `lockdownStaleClaim` 2 min is shorter than a paced call (~280 s); raise above the worst case (5 min) or
  renew; fix comment and AGENTS.md.

## Wave 6: raid and AI

- **R1 [Medium, V]** `DrainAISpamChecks` (`aispam.go:469-488`, `main.go:259-263`, `raid_ai_count.go:143-150`) waits 30 s under
  a 10 s handler, checks run on `context.Background()`, the deferred `aispamDraining.Store(false)` reopens the queue,
  `raidAIFireAsync` ignores draining. Cancellable root context cancelled first; wait at most 9 s; never reset the flag;
  `raidAIFireAsync` refuses while draining. Keep AGENTS.md's 10 s true.
- **R2 [Medium, P]** `raidJoinCounts` via `raidCountMessageJoins` (`raid_join.go:47`): approved join requests count toward the
  surge. Check the Bot API service message for an approved request; if indistinguishable count only from the `chat_member`
  path for self-performed joins. Guard unchanged unless proven. AGENTS.md.
- **R3 [Low-Medium, P]** `raidFloodItemKey` (`raid_flood.go:144-151`): count `RichMessage`, `Checklist`, `Game` if the fields exist.
- **R5 [Low, R]** breaker maps (`aispam.go:520-535`, `raid_ai_breaker.go:55-60`) never shed stale entries; prune entries
  whose last failure is older than the window/cooldown.
- **R6 [Low, R]** `raid_flood.go:152`: a slash command in a caption counts as flood at -4; detect it like text.
- **R7 [Low, P]** TypeSafe client (`aispam_jev.go:41-47`): `CheckRedirect` returning `http.ErrUseLastResponse`.
- **R8 [Low, R]** `lockdown_help_msg` (8 locales): video thumbnails also go to Anthropic; with `/aispam` on any member's
  delete verdict counts. `TestLockdownHelpFitsOneMessage` must pass.

## Wave 7: member requirements

- **Q1 [Medium, V]** during a cap pause the executor deletes the message of every case (`memberreq_enforce.go:640-646`,
  `memberreq_watch.go:399-405`), lookup-miss cases included. Add a case field "watcher found a name-rule breach"; delete
  only for those during a pause.
- **Q2 [Medium, P]** the cap counts jobs (`memberreq_enforce.go:657`, `memberreq_limits.go:53-76`); count distinct members
  per window: one Lua `SADD` of the user to `fuku:memberreq:{<chat>}:capm:<minute>` with EXPIRE and `INCR` only when
  `SADD` returned 1. AGENTS.md.
- **Q3 [Low, P]** a joiner with a cached AI pass never gets a baseline (`memberreq_watch.go:240-245`, `memberreq_join.go:105-110`,
  `memberreq_ai.go:353-354`); when the AI part is satisfied by a cached verdict while `nu` is present, hand exactly one job.
- **Q4 [Low, R]** an undeliverable join-mute notice means no welcome (`memberreq_join.go:119-121`, `memberreq_notice.go:259-279`,
  `memberreq_enforce.go:584-585`); return an outcome that makes the join entry welcome.
- **Q5 [Low, P]** captcha guard misses a pending mute hold (`memberreq_join.go:157,99`); read the live record and keep the
  marker mute when it carries the hold's marker.
- **Q6 [Low, P]** warned note TTL (`memberreq_warned.go:44`) raise to 7 days.
- **Q7 [Docs, R]** comments: `memberreq_ai.go:26-41` baseline step first; `memberreq_ai_budget.go:71-73` budget fault is
  sticky unknown; `memberreq_lookup.go:49-52` the punishing path asks the bot's standing.

## Wave 8: Staff Group

- **S1 [Medium, P]** kick/mute decide from a stale state (`staff_action_run.go:583-605,682-697`); re-read live inside the
  paced closure and re-decide with `decideStaffAction`.
- **S2 [Medium, V]** fan-out and undo never re-read links (`staff_action_run.go:502`, `staff_recheck.go:162-202`); before each
  write read `staff.GetLinkOfGroupFresh`, require the same link ID and Staff Group, else skip "no longer linked" (8
  locales, summary line); `recheckLink` returns `staffRecheckGone` when the row is gone.
- **S3 [Medium, R]** staff mute replaces a longer restriction (`staff_action_decide.go:250-257`); skip "already restricted
  longer" when restricted with a later or permanent end. Ban half: owner decision.
- **S4 [Low-Medium, R]** `RekeyChat` (`db/staff/rekey.go:71-73`) also drops `staff_link_of:<group>` of every linked group.
- **S5 [Low, P]** undo restore (`staff_undo.go:797-804`): same `staffUntilTooClose`/`errStaffTimeRanOut` guard; fix AGENTS.md.
- **S6 [Low, P]** undo restoring an old lockdown marker ban (`staff_action_decide.go:488-492`): when that lockdown is not
  active, unban `only_if_banned` instead.
- **S7 [Low, R]** owner-change notices per link, unpaced (`staff_recheck.go:184-188`): one combined notice through `staffPaced`.
- **S8 [Low, V]** `/unsetstaff` Cancel (`staff.go:371-376`) needs the same authority as Confirm.
- **S9 [Low, P]** answer the callback before the panel rebuild (`staff_unlink.go:253-257,411-417`).

## Wave 9: deletion and infrastructure

- **I1 [Medium, V]** `i18n/loader.go:33` `filepath.Join` on embed paths; use `path.Join`; grep others.
- **I2 [Medium, R]** a pin drops only the `member:selfdestruct` row (`member_deletion.go:494-502`, `pins.go:338`); drop the
  pending row of any kind; gate the fast path on "any stored delete rule". AGENTS.md D-11.
- **I3 [Medium-Low, R]** webhook pre-fills `ContextDataKey` (`tracing/processor.go:51`, `httpserver/server.go:370`); detect
  nesting by a dedicated marker; adjust `TestTracingProcessorProcessUpdateCountsTopLevelUpdatesOnly` and add a webhook case.
- **I4 [Low-Medium, R]** `translator.go:90-94` verb check after `{name}` substitution; Sprintf from the raw template before
  filling user values; `TestRawPlaceholderKeysRender` passes; test with reason `100%sure %d`.
- **I5 [Low-Medium, V]** `migrations/runner.go:79` skips checksums when nothing is pending; always verify, report a row with no
  file; keep `scripts/migrate_psql.sh` consistent.
- **I6 [Low, R]** `errors.Wrapf` (`utils/errors/errors.go:60`) reports its own location; shared helper with the right skip.
- **I7 [Low, R]** `/debug/pprof/` behind `requireMetricsAuth` when a token is set (`server.go:201-202`).
- **I8 [Low, P]** `/health` (`server.go:105`): `PING` instead of the shared key; with `DISABLE_CACHE=true` and no Redis do
  not fail.
- **I10 [Low, R]** `i18n/table.go` `StringTable` must fail on a missing/empty key in any language; fix `trS` comment.
- **I11 [Low, V]** `Makefile:56` `make test-shard` runs everything on an empty pattern; fail when the script fails or prints nothing.
- **I12 [Low, V]** CI `Test Suite` job skipped when a shard fails (`ci.yml:413-417`); `if: always()` plus a first step that
  fails unless both needs succeeded.

## Wave 10: settings menu and small fixes

- **T1 [Low, R]** hidden page forged write press gets the owner-only alert (`settings_menu.go:689`, `settings_list.go:602`);
  run the `Hidden` guard before the right check.
- **T2 [Low, P]** claim script (`settings_input.go:201`) should extend a nearly-expired wait to at least 60 s, marker in step.
- **T3 [Low, P]** `listStepOne` (`settings_list_input.go:397-421`): own bound for the send; record `m` on a detached bounded context.
- **T4 [Low, V]** `admin_anon_admin_enabled_now`/`_disabled_now` texts are backwards in en, uk, ru, fr (check all 8);
  fix unless a golden pins them.

## Wave 11: final verification and PR

Full checks, push, draft PR, subscribe, fix CI until green, report.
