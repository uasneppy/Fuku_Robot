---
gsd_state_version: "1.0"
current_phase: 9
current_phase_name: Settings Menu
status: completed
stopped_at: Phase 9 complete — all phases complete
last_updated: "2026-10-30T20:36:05.259Z"
last_activity: 2026-10-30
last_activity_desc: Phase 9 complete
state_head: d2b368f8b88c33f6827d54f003f58d781f56aec6
progress:
  total_phases: 9
  completed_phases: 9
  total_plans: 106
  completed_plans: 106
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-30)

**Core value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.
**Current focus:** All phases complete — follow-ups and deferred live checks only

## Current Position

Phase: 9 of 9 (Settings Menu) — complete
Plan: 14 of 14
Status: All phases complete
Last activity: 2026-10-30 — Completed quick task 261030-rig: Switch raid AI image checks from Gemini to Claude Haiku 5.5 through the Anthropic API

Progress: [██████████] 100%

## Performance Metrics

**Velocity:**
- Total plans completed: 106
- Average duration: -
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 10 | - | - |
| 02 | 10 | - | - |
| 03 | 12 | - | - |
| 04 | 8 | - | - |
| 5 | 14 | - | - |
| 6 | 13 | - | - |
| 7 | 14 | - | - |
| 8 | 11 | - | - |
| 9 | 14 | - | - |

**Recent Trend:**
- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 03 P01 | 15min | 3 tasks | 16 files |
| Phase 03 P02 | 8min | 2 tasks | 2 files |
| Phase 03 P03 | 11min | 2 tasks | 12 files |
| Phase 03 P04 | 12min | 2 tasks | 15 files |
| Phase 03 P05 | 14min | 2 tasks | 13 files |
| Phase 03 P06 | 24min | 2 tasks | 16 files |
| Phase 03 P07 | 11min | 2 tasks | 11 files |
| Phase 03 P08 | 13min | 2 tasks | 13 files |
| Phase 03 P09 | 20min | 2 tasks | 11 files |

## Accumulated Context

### Decisions

Decisions are logged in the PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [Roadmap]: Image classification uses Gemini `gemini-3.5-flash-lite` (owner's decision). This replaces the research STACK's Claude Haiku / `anthropic-sdk-go` recommendation, so Phase 7 research must target Gemini.
- [Roadmap]: Raid auto-triggers are on by default with conservative thresholds (owner's decision), not "off until configured" as the research suggested.
- [Roadmap]: Multiple replicas are the deployment target, so fan-out pacing and detection counters go through Redis, not an in-process limiter. This closes research gap 5.
- [Roadmap]: PLAT-03 is mapped to Phase 1, PLAT-01 to Phase 2 and PLAT-02 to Phase 7, where each first matters. All three still apply to every later phase (see ROADMAP Cross-Cutting Constraints).
- [Roadmap]: LOCK-10 (one lockdown and one alert per raid) is in Phase 6, where concurrent auto-triggers first exist.
- [Phase 1]: Role exclusivity is enforced by a PostgreSQL trigger with per-chat advisory locks (owner's pick), on top of in-transaction app checks.
- [Phase 1]: Only a definite owner mismatch removes a link; Telegram errors are unknown and change nothing. Phase 2's pre-action owner recheck should reuse `staff.*Fresh` + `chat_status.CheckOwner` the same way.
- [Phase 1]: Exactly-once Staff Group notices come from conditional writes (RowsAffected == 1 posts), not locks.
- [Phase 2]: One decision table (`decideStaffAction`) picks each group's Telegram call from the target's live status, so no staff action lifts a ban. Phase 3's "Undo everywhere" must reuse it rather than calling Telegram directly.
- [Phase 2]: Staff fan-out calls go through the fleet-wide Redis pacer (`staffPaced`); a slot more than 60 s away fails at once as "rate limited". One run per target is held by a Redis lock renewed every 10 minutes.
- [Phase 3]: Undo decisions live in `decideStaffUndo`, beside `decideStaffAction`. One undo per action is the conditional `ClaimUndo`; `ReleaseUndo` gives it back only when no group reached its Telegram write (owner's UAT choice).
- [Phase 3]: Undo state everywhere (Recent actions, detail view, Ask/Confirm, the original summary) comes from stored per-group undo outcomes through `staffUndoStateOf`, never from the claim alone.
- [Phase 3]: The staff audit record (`staff_actions`, `staff_action_groups`) is read fresh, never cached, kept forever and outside backup/export/import/reset. Every staff fan-out, action or undo, goes through `startStaffRun` with a `staffRunSpec`; later fan-outs (Phase 5's "Ban N recent joiners") should reuse it.
- [Phase 4]: Lockdown state is only in PostgreSQL (`chat_lockdowns`, `chat_lockdown_joiners`, read fresh). The join guard (group -7) records and a DB-driven worker on every replica makes every Telegram write through the fleet-wide `lockdownPacer`. Phase 5's alert buttons and "Ban N recent joiners" should read the joiner rows and lift through `lockdownBeginLift` only.
- [Phase 4]: The lift restores the raw `getChat` snapshot first, then records it with the conditional `BeginLift`; it unbans only joiners whose live ban carries the row's 330-day `ban_until` marker. Authority is a live `getChatMember` (`requireLockdownAuthority`), never the admin cache.
- [Phase 4]: Every unmute goes through `resolveUnmutePermissions`, which returns the stored pre-lockdown permissions during a lockdown (D-23).
- [Phase 5]: Alerts are planned once by the conditional `lockdown.BeginAlert` and posted, refreshed, reminded and closed by the lockdown worker from `chat_lockdown_alerts` rows. Phase 6's automatic triggers should call the same alert path with their own `trigger_kind` and counts, and may reuse the `alita:lockdown:recent:<chat>` 10-minute join tracker.
- [Phase 5]: Alert buttons (`lockdown|v1`) are stateless and judge every press by a live `getChatMember` of the locked chat; Lift goes through `liftLockdownRow`, shared with `/unlockdown`.
- [Phase 5]: "Ban N recent joiners" bans are permanent and resumable (`chat_lockdown_ban_runs`, result delivery since migration 20261011160000); the lift keeps them.
- [Phase 6]: Automatic triggers call `startLockdownRow`, the start path `/lockdown` uses; one lockdown and one alert come from the PostgreSQL index and conditional writes, never from the Redis latch. Settings live in `chat_raid_detection` (`raiddetection.GetSettings`, listed in `skipLocal`). Phase 7's AI-verdict trigger should add its own counter and setting beside the join and flood ones, and fire through `raidFire`.
- [Phase 6]: The flood counter is two watchers (group `-4` and group `12`) sharing `raidFloodCountUpdate`; group `12` must stay after every other handler group. The old `/antiraid` mode is a pointer command and `alita:antiraid:*` keys are swept at startup.
- [Phase 7]: AI verdicts only add to a Redis count (`raidAIRecordVerdict`, items `:av`, accounts `:aa`); the rule (threshold from at least 3 accounts in 120 s) fires `raidFire` with `ai_burst`, off the worker on its own goroutine. Settings are the `ai_enabled`/`ai_threshold` columns of `chat_raid_detection`. The candidate watcher is handler group 2; the raid AI pool, its budgets, breakers and the `StopRaidAI` drain are separate from `/aispam`. Phase 8 must register the Turnstile secret with `logredact.RegisterSecret` (the Turnstile half of PLAT-02) and keep new handler groups below 12.
- [Phase 7]: The Gemini client (`alita/utils/gemini`) is stdlib-only so tools can import it without the pre-main database connection; the key travels only in the `x-goog-api-key` header. The spike tool (`scripts/gemini-spike/`) is the owner's way to measure accuracy, latency and cost.
- [Phase 8]: A Turnstile attempt is an ordinary `captcha_attempts` row with a reserved `ts1:` answer; the Redis token (`alita:captcha:ch:<token>`) is only a handle, consumed by a user-compared Lua claim that is released on any error. Pass, failed solve and timeout are decided only by the conditional delete of the attempt row (`claimCaptchaPass`/`finishCaptchaPass` are shared with the button), so any replica can act. Both button callbacks refuse a `ts1:` attempt.
- [Phase 8]: The page decides nothing: a pass needs signed `initData` (HMAC, 5 min, user must match) and a `siteverify` success echoing hostname, action `captcha_join` and `cdata`. Only provable failures kick (D-28); Cloudflare, network and setup faults retry with the token re-armed; five failed solves in a row with no pass are held back as a suspected key mismatch (`alita:captcha:streak`). The verify route has Redis rate windows, a readiness gate, a decided-answer replay marker and shares the bot's port: the owner's proxy must forward only `/captcha*`.
- [Phase 9]: The settings menu (`settings|v1`) is stateless: no migration, model, Redis key, shutdown drain or handler group. Every press runs one pipeline (decode and allow-list, service identity, opener, live `getChatMember` of the presser in the configured group, the control's `settingsRight`), is answered exactly once and edits the one message. Every setting has one apply function beside its command, so menu and command cannot drift (parity, drift and authority-parity tests per setting). A page is one `registerSettingsPage` call from its own file.
- [Phase 9]: Lift and Unlink cards carry the object they were drawn for (`r`, `l`) and a lazy expiry, so a stale Yes cannot hit a newer lockdown or link (D-33). A group menu, and a private menu of an admin who is not the group's creator, never name the Staff Group (D-21, D-32); `settingsStaffViewFor` is the one place that decides. A Lift from a group menu only wakes the worker (its redrawn menu is the notice); from a private menu it also posts the public notice (D-26).

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 7]: The owner accepted the seven live checks without running them ("All good, continue"): the Gemini spike with a paid-tier key (the 0.85 image bar, the GIF/video thumbnail choice, the model id, latency, cost, safety-block rate), a real AI burst, two replicas, `make test-postgres-integrity` for the new `ai_threshold` CHECK, a native-speaker read of the translations (the es, pt and id help paragraphs were shortened; pt has 26 units of headroom against Telegram's 4096), a live log grep for the Gemini key and bot token, and `make lint` on Go 1.26. Code review WR-01..WR-05 and IN-01..IN-09 were all fixed (`07-REVIEW-FIX.md`).
- [Phase 7]: The security audit closed 48/48 threats (`07-SECURITY.md`, `threats_open: 0`); validation is compliant (`07-VALIDATION.md`, no gaps). Open non-blocking notes: O-1 (worth a follow-up) the raid-only TypeSafe text path has no hourly volume cap or verdict cache like the Gemini 20/150/500 guards, so a sustained link flood from fresh accounts could spend roughly 14,000 TypeSafe calls an hour (bounded only by the 4-worker pool, the 128-job queue and the candidate rule; D-20 names Gemini only), and a per-account and per-chat hourly counter for text jobs would close it; O-2 `aispamCarvedOut` on the group 2 path uses the bot client's default timeout; O-3 `ValidModel` accepts `.` and `..`; O-8 five crafted unusable Gemini answers can pause one chat's image trigger for 5 minutes (join and flood triggers are unaffected). Accepted risks AR-07-01..04 and transfer TR-07-01 (images and texts go to Google and TypeSafe by the owner's choice).
- [Phase 7]: Accepted limit: a follower of a shared image verdict inherits the leader's refusal only within one chat; overlap across replicas is covered only once a verdict is cached. A rolling deploy can make an `ai off` setting read as on for up to 30 minutes (documented in AGENTS.md).
- [Phase 8]: The owner accepted the live checks without running them ("All good, continue"): the Direct Link Mini App delivering the tapper's `initData` on Android, iOS, Desktop and Web, the Turnstile widget inside each WebView, the real `siteverify` hostname, the CSP and `frame-ancestors` list against a live widget, BotFather accepting the short name, a solve after the signed data is more than 5 minutes old (it shows the "invalid" copy today), a native-speaker read of the new strings in six languages, and `TestCaptchaModeTurnstileConstraint` in CI through `make test-postgres-integrity`. Steps are in `08-SPIKE.md`; the owner needs a public HTTPS hostname, a BotFather Mini App and Turnstile keys (one widget's site key and secret).
- [Phase 8]: The security audit closed 49/49 threats (`08-SECURITY.md`, `threats_open: 0`); validation is compliant (`08-VALIDATION.md`, no gaps, 12 review-fix rows added). Open non-blocking notes: U-1 (low) the key-mismatch breaker is fleet-wide state that a hostile member can trip with 6 failed solves, which only delays the immediate kick (the attempt's own timeout still applies the failure action; per-chat tripping or counting only unforgeable codes would harden it); O-1 no test cancels the request mid-verify (the `WithoutCancel` path and the durable unmute row are tested); O-2 `TestPageHeadersAndNonce` compares the CSP header with `CSPFor` itself, so a weakened directive would not fail it; an old token after a re-join answers `expired` with the removal wording although the new attempt is untouched.
- [Phase 8]: Code review found 0 critical and 7 warnings and 6 info notes; all 13 were fixed (`08-REVIEW-FIX.md`). Accepted limits: if Redis is also unreachable when a failed consume is released, the claim stays until the challenge expires and that member times out; a replica that crashes between a consume and a re-arm loses that member's challenge until the timeout; the rolling-deploy window before every replica has the `ts1:` guard.
- [Phase 9]: The owner accepted the 14 live checks without running them ("Accept, test live later"): edit-in-place, Back and Close on every client, the Open in private deep link for an admin who never started the bot, a demoted opener, another member pressing the opener's menu and the anonymous-admin proof flow, fast stepper taps with a 429 and Close on a private menu older than 48 hours, the Staff Group page after a slow build, the picker with 9 or more groups, Link and Unlink from a group menu and a Lift from both menus, button truncation in hi and ru on a phone, a native-speaker read of the 83 `settings_*` strings in six languages, and the Captcha, Antiflood and Lockdown pages against the commands in a real group (including the 10 s local-cache skew between replicas). Steps are in `09-SPIKE.md`.
- [Phase 9]: Code review found 0 critical, 5 warnings and 5 info notes; all 10 were fixed (`09-REVIEW-FIX.md`), with amendments D-32, D-33 and D-34 recorded in `09-CONTEXT.md`. Known and accepted: the antiflood first-write side effect (D-27) stays in the repository, so a first Ban, Kick or delete-switch press on a chat with no antiflood row stores limit 5 and turns antiflood on; the menu now says so (`settings_antiflood_now_on`). A repository fix (insert with limit 0) would also change `/setfloodmode` and `/delflood`, so it is a separate follow-up that must update `TestSetFloodModeOnFreshChatStoredLimit`, `TestSetFloodMsgDelOnFreshChatStoredLimit` and the AGENTS.md trap bullet. The `captcha_settings` and `antiflood` keys are not in `skipLocal`, so another replica can show an old value for up to `CACHE_LOCAL_TTL`, exactly as the commands can.
- [Phase 9]: The security audit closed 52/52 threats (`09-SECURITY.md`, `threats_open: 0`; 25 mitigations were mutation-checked); validation is compliant (`09-VALIDATION.md`, no gaps, 10 review-fix rows added). Open non-blocking notes: U-1 (low) the private press still answers "bot not in chat X" differently from "not an admin" (the deep link was fixed by IN-03; the press path needs a modified client and has no write or data impact); U-2 a modified-client admin can aim a settings callback at any bot message in a chat they administer; U-3 opening the Staff page runs `recheckLink`/`applyLinkHealth` for any live admin of a linked group, the same as `/staff`; U-4 hardening: the private path checks authority for `c` but writes to the ID `getChat` returned; O-6 no behavioural test asserts that the Lockdown page never starts a lockdown (it is held by the absence of code). Accepted risks AR-09-01..09 (group menus show the masked "Linked" line, cross-replica cache skew up to 10 s, the antiflood first write, menus never auto-close, no deep-link cooldown).
- [Phase 9]: Remaining items that need the owner's environment, across all phases: `make test-postgres-integrity` in CI (including `TestCaptchaModeTurnstileConstraint`), `make lint` on a Go 1.26 golangci-lint, the Gemini spike with a paid-tier key, the Turnstile real-device checklist (`08-SPIKE.md`) and a deploy with the public `/captcha*` proxy rules.
- [Quick 261030-rig]: Raid AI image checks now use Claude Haiku 5.5 (`claude-haiku-5-5`) through the Anthropic Messages API (`alita/utils/claude`, `ANTHROPIC_API_KEY`, optional `ANTHROPIC_MODEL`); `GEMINI_API_KEY` and `GEMINI_MODEL` are no longer read, so a deployment that set them must set `ANTHROPIC_API_KEY` to keep image checks (breaking). The request and response shapes (`anthropic-version 2026-10-21`, a forced one-field tool call, no `temperature`, `thinking` disabled, refusal and error fields) come from the claude-api reference notes and are verified only against `httptest` doubles; the owner's run of `go run ./scripts/claude-spike -probe` with a real key is the live check, and it also measures accuracy, latency and cost (the 0.85 bar and the 20/150/500 hourly budgets are one-line constants to retune for Haiku pricing). A few Gemini-era verdicts may be served from the 1 h verdict cache right after deploy. Closes Phase 7 note O-3 (`ValidModel` rejects `.` and `..`). Help text in `lockdown_help_msg` grew 3 units per language (pt now has 23 spare).
- [Phase 1]: The live ownership-transfer check (does `chat_owner_changed`/`chat_member` arrive, with the bot as admin and as a member) was never run; UAT tests 3-4 were deferred. The hourly sweep removes stale links whatever Telegram sends.
- [Phase 1]: 10 of 11 live-Telegram UAT checks were deferred without being run (`phases/01-staff-group-links/01-UAT.md` Deferred Follow-Ups). CI had not run on PR #1 at phase close; the PostgreSQL trigger test passed locally against PostgreSQL 16.
- [Phase 1]: Low, non-blocking security flag TF-01-06: a stranger's `/unlinkstaff` reply reveals whether the group is linked (`01-SECURITY.md`).
- [Phase 2]: Open code-review finding WR-03 (warning): at a `retry_after` of exactly 60 s, all but the first concurrent paced call fail as "rate limited" instead of waiting. Those groups are still reported. Nine info notes are open too (`02-REVIEW-DISPOSITION.md`).
- [Phase 3]: Open code-review warnings WR-03 (a retry after an ambiguous `ReleaseUndo` error can run `FinalizeUndo` on a released record) and WR-04 (a history page press is answered before the data is read, so a DB error is a silent no-op), plus 6 info notes (`03-REVIEW-DISPOSITION.md`). The owner accepted WR-01, WR-02 and IN-02 in UAT.
- [Phase 4]: Spike 1 answered by the owner: Telegram greys out a per-user "Send messages" exception once the default is locked, so approved users are muted during a lockdown like everyone else (LOCK-03 fallback, 04-CONTEXT D-01/D-02). Spike 2 (join paths) is designed away, with live join delivery checked in UAT.
- [Phase 4]: Code review info notes IN-01..IN-04 are open (`04-REVIEW-DISPOSITION.md`); CR-01, WR-01 and WR-02 were fixed before verification. UI review copy fixes landed in quick task 261008-j3d.
- [Cross-phase]: Some requirements are only partly checkable in their own phase, and a later phase finishes them. The `/staff` in-lockdown status (SETUP-08) shipped in Phase 4; the alert's Lift button (LOCK-06) and listing removed joiners on the alert (LOCK-01) shipped in Phase 5. The AI-trigger toggle (RAID-06) lands in Phase 7.
- [Phase 5]: Live Telegram UAT (Lift from both copies, real Ban N run, revoke refusal texts, hourly reminders) and `make test-postgres-integrity` in CI were accepted by the owner without being run here. Code review WR-01..WR-03 and IN-01..IN-09 were all fixed (`05-REVIEW-FIX.md`). By decision D-04/D-20 the alert text is not refreshed and a log channel with the admin category off gets no copy.
- [Quick 261008-j3b]: Shutdown follow-ups not fixed: `DrainAISpamChecks` waits up to 30 s but keeps the 10 s allowance; in polling mode the 60 s budget can still cut the staff drain short; `render.yaml`, `app.json`/Procfile and `railway.toml` keep their platforms' 30 s stop limits.
- [Quick 261008-j3a]: Follow-up candidate: an anonymous admin's ban-family command also gets a stray `chat_status_restrict_cmd_error` reply before the proof tap (pre-existing).
- [Phase 6]: The owner accepted the seven live checks without running them ("All good, continue"): a real join surge, a real flood, two replicas racing, `/autolockdown` with anonymous-admin proof, the retired `/antiraid`, and `TestRaidDetectionCarryOver` / `TestRaidDetectionSettings` through `make test-postgres-integrity` in CI. Code review WR-01..WR-03 and IN-01..IN-05 were all fixed (`06-REVIEW-FIX.md`).
- [Phase 6]: The six non-English translations of the new lockdown and antiraid texts have not had a native-speaker review. `lockdown_help_msg` is near Telegram's 4096-character limit in es (margin 44) and pt (margin 31); `TestLockdownHelpFitsOneMessage` guards it.
- [Phase 6]: The security audit closed 39/39 threats (`06-SECURITY.md`, `threats_open: 0`). Open non-blocking notes: O-1 the per-message settings read is bounded at 2 s (not 250 ms) and bypasses the in-process cache, so a Redis brownout can hold a message in group -4 for up to 2 s; O-4 the `lockdown_help_msg` sentence "only the owner or a restricting admin can use these commands" is stricter than the code for `/lockdownstatus` and for viewing `/autolockdown` (any live admin), in all 7 locales; O-5 a stale flooder snapshot can reach a second flood alert if its write fails inside the 15-minute TTL. Accepted risk AR-06-02: someone can lock a group on purpose (15 fresh accounts joining in 60 s, or 3 accounts that joined in the last 24 h sending 30 items in 30 s); the cost is one `/unlockdown`.
- [Tooling]: `make lint` needs a golangci-lint built with Go 1.26; the sandbox's is built with Go 1.25, so the quick batch was linted only with `go vet` and `gofmt`.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 261004-w9v | Load AGENTS.md from .claude/CLAUDE.md and correct the locale list in CLAUDE.md and the codebase map | 2026-10-04 | c85d648 | [261004-w9v-load-agents-md-from-claude-claude-md-and](./quick/261004-w9v-load-agents-md-from-claude-claude-md-and/) |
| 261008-j3a | Fix anonymous-admin commands doing nothing after the proof tap | 2026-10-08 | 686f960 | .planning/quick/261008-j3a-fix-the-pre-existing-anonymous-admin-bug-after-the-anonymous |
| 261008-j3b | Give the staff and lockdown shutdown drains their full wait | 2026-10-08 | 6f7da31 | .planning/quick/261008-j3b-close-the-shutdown-timing-gap-the-shutdown-manager-gives-eac |
| 261008-j3c | Make TestStaffActionNonStaffUnchanged robust to second boundaries | 2026-10-08 | 077ee2e | .planning/quick/261008-j3c-make-the-flaky-test-teststaffactionnonstaffunchanged-tmute-a |
| 261008-j3d | Lockdown copy polish: no raw Telegram errors, warning markers, code-formatted help | 2026-10-08 | 924b5e6 | .planning/quick/261008-j3d-lockdown-copy-polish-from-planning-phases-04-manual-lockdown |
| 261030-rig | Switch raid AI image checks from Gemini to Claude Haiku 5.5 through the Anthropic API | 2026-10-30 | d2b368f | [261030-rig-change-gemini-to-claude-haiku-5-5-via-th](./quick/261030-rig-change-gemini-to-claude-haiku-5-5-via-th/) |

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| *(none)* | | | | |

## Session Continuity

Last session: 2026-10-30T13:30:00.000Z
Stopped at: Phase 9 complete — all phases complete
Resume file: None
