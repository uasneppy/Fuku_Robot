# Quick task 261117-w2c: captcha review fixes (Wave 2), summary

Branch `wave2-captcha` from `6f2acd4`, not pushed. Every fix has a test that failed on the code before it.

## Fixed

| Item | Commit | What changed | Test |
|------|--------|--------------|------|
| C1 | e8a576f | Attempts are created with a microsecond `expires_at` (UTC); the timer compares fresh and captured expiry truncated to the microsecond. On PostgreSQL the timer had treated every attempt as stale, leaving the action to the 5-minute cleanup tick. | `TestCaptchaTimerIgnoresMicrosecondRounding`, `TestCreateCaptchaAttemptExpiresAtMicroseconds` |
| C2 | 5311275 | `settleFailedCaptchaAction`: a failure action answered with an error is judged by one live `getChatMember`. Left or kicked (kick, ban) and restricted-without-send (mute) are applied; a definite 4xx other than 429 (or a mute whose unmute could not be stored) releases and unmutes (WR-07); a 429, 5xx, timeout or unreadable record leaves the member muted with the 24 h fallback unmute row and one Warn by IDs. The blind second member-removing unban after a failed kick is gone. AGENTS.md WR-07 text updated. | `TestCaptchaTimeoutFailedActionReadsLiveMember` (8 cases) |
| C3 | 6f6ee3e | `unmuteExpiredCaptchaUsers` reads the member live first: only member or restricted is unmuted, any other status drops the schedule, an unreadable record retries next tick. | `TestUnmuteExpiredCaptchaUsersReadsLiveMember` |
| C4 | b05bcae | Startup recovery releases an unsent (`message_id <= 0`) unexpired attempt only when it is older than 2 minutes (`captchaUnsentAttemptGrace`); the cleanup tick releases it at expiry otherwise. | `TestRunOrphanedCaptchaRecoverySparesYoungUnsentAttempt` |
| C5 | c6c18df | New `captchaweb.ActionSpared`. `captchaWebBackend.Expire`, when the row is gone or another caller won the claim, reads the member live (`decidedElsewhere`, 3 s): expired + action only while the record shows that action; otherwise neutral `invalid`, stored the same in the replay marker, no error counted. An unreadable record or setting keeps the old answer. AGENTS.md updated. | `TestCaptchaWebDecidedAttemptAnswersLiveMember`, `TestVerifyRouteActionSpared` |
| C6 | 86bc9cc | `restoreCaptchaPermissions` logs a failed schedule delete after a successful unmute instead of returning it. | `TestRestoreCaptchaPermissionsIgnoresScheduleDeleteError` |
| C7 | 09ea48c | `InitDataMaxAge` 5 min -> 11 min (longest attempt 10 min + 1). The page and its node tests do not pin the age, so they did not change. | `TestInitDataRejects` (new 10-minute case, 11 min 1 s refusal) |
| C8 | 9f73875 | `startCaptchaLifecycleTask`'s goroutine starts with `defer error_handling.RecoverFromPanic` (inside the wait group's `Done`). | `TestCaptchaLifecycleTaskRecoversFromPanic` |
| C9 | 5821451 | AGENTS.md: group -10 records and deletes a pending member's messages; nothing is replayed, a pass posts a summary. | none (docs) |

Existing tests changed to match the intended behaviour (none is a `TestLegacy*Requests` golden):
- `TestHandleCaptchaTimeoutRestoresPermissionsWhenActionFails` and `TestCaptchaWebFailedActionNotAppliedIsNeutral`: the
  injected error is now a real `*gotgbot.TelegramError` 400 (they describe a refusal; a plain error now reads as unknown).
- `TestRunOrphanedCaptchaRecoveryPersistsFailedUnmuteWithoutFailingStartup`: the unsent attempt is 3 minutes old (C4).
- `TestCaptchaWebRejoinInvalidatesOldChallenge`: the old token is answered `invalid` with no action (C5).
- `outcomeRace` (two race tests): the fakes answer the racer's live record as `left`, because the fake does not track
  status and the expired branch now reads it (C5).
- Stale initData fixtures use `InitDataMaxAge + 1 min` (C7).

## Not reproduced

None.

## Owner decisions

None raised by this wave. No golden was touched.

## User-visible behaviour changes

- C1: on PostgreSQL the captcha's failure action now runs when the attempt ends instead of up to 5 minutes later.
- C2: a kick, ban or mute that Telegram answered with a 429, 5xx or timeout no longer unmutes the member; they stay muted
  and are unmuted by the worker after 24 hours if still in the group. A failed kick is not retried.
- C3: the 24-hour fallback unmute never touches a member who was banned or left meanwhile.
- C4: a joiner whose challenge another replica is still sending is not unmuted by a restart.
- C5: the web page no longer tells a member who was released (or is on a newer attempt) that they were kicked; it shows
  the neutral "no longer valid" answer. An old token after a re-join now answers `invalid`, not `expired`.
- C7 (authorised): a web captcha link opened at the start of an attempt works for the attempt's whole lifetime.

## Verification

- `go test -tags testtools -race -count=1` for every captcha test of `fuku/modules`
  (`^(TestCaptcha|TestHandleCaptcha|TestRunOrphanedCaptcha|TestUnmute|TestRestoreCaptcha|TestRunCaptcha|TestSendCaptcha|TestCleanupExpiredCaptcha|TestMemberReqCaptcha|TestGreetOnceCaptcha)`)
  plus greet-once, member requirement join and the deletion exclusion tests: pass.
- All of `./fuku/utils/captchaweb` and `./fuku/db/captcha`: pass.
- `CGO_ENABLED=0 go build ./...`: pass. `make check-translations`: pass (no locale key added).
- `make lint`: the installed golangci-lint 2.5.0 (built with go1.25) refuses the Go 1.26 target; golangci-lint 2.15.0
  built with go1.26.0 reports 0 issues with the repository's config.
- Node page tests not run: the page and its scripts did not change.

## Left unfinished

Nothing. `make test` was not run (wave instruction).
