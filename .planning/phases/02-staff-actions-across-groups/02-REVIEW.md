---
phase: 02-staff-actions-across-groups
reviewed: 2026-10-05T08:00:00Z
depth: standard
files_reviewed: 38
files_reviewed_list:
  - AGENTS.md
  - alita/db/user/find_by_username_test.go
  - alita/db/user/repository.go
  - alita/modules/staff.go
  - alita/modules/staff_action.go
  - alita/modules/staff_action_actions_test.go
  - alita/modules/staff_action_card.go
  - alita/modules/staff_action_decide.go
  - alita/modules/staff_action_decide_test.go
  - alita/modules/staff_action_dispatch_test.go
  - alita/modules/staff_action_fake_test.go
  - alita/modules/staff_action_gates_test.go
  - alita/modules/staff_action_lifecycle_test.go
  - alita/modules/staff_action_parse.go
  - alita/modules/staff_action_parse_test.go
  - alita/modules/staff_action_progress_test.go
  - alita/modules/staff_action_run.go
  - alita/modules/staff_action_scale_test.go
  - alita/modules/staff_action_summary.go
  - alita/modules/staff_action_summary_test.go
  - alita/modules/staff_action_target_test.go
  - alita/modules/staff_action_test.go
  - alita/modules/staff_action_timed_test.go
  - alita/modules/staff_recheck.go
  - alita/modules/users_command_test.go
  - alita/utils/extraction/duration_token_test.go
  - alita/utils/extraction/extraction.go
  - alita/utils/ratelimit/telegram_pacer.go
  - alita/utils/ratelimit/telegram_pacer_test.go
  - docs/src/content/docs/commands/staff/index.md
  - locales/en.yml
  - locales/es.yml
  - locales/fr.yml
  - locales/hi.yml
  - locales/id.yml
  - locales/pt.yml
  - locales/ru.yml
  - main.go
findings:
  critical: 0
  warning: 2
  info: 6
  total: 8
status: issues_found
---

# Phase 2: Code Review Report

**Reviewed:** 2026-10-05T08:00:00Z
**Depth:** standard
**Files Reviewed:** 38
**Status:** issues_found

## Summary

Reviewed the Staff Actions fan-out end to end: the command interceptors, the parser, the Redis confirm card (Lua compare-and-set, target lock), the per-group check chain and decision table, the summary renderer, the Redis-backed Telegram pacer, the username resolver, the shutdown hook, the locales and the docs.

The safety-critical parts hold up under adversarial reading:

- The per-group authority is the live `getChatMember` of the issuer (creator, or administrator with `can_restrict_members`) and runs before the target is looked up. The owner recheck runs through `recheckLink`. The Staff Group is never written to.
- The decision table never sends a restrict or a member-removing unban to a banned or departed target.
- The card is issuer-only through an atomic Lua transition, and Confirm re-verifies the Staff Group, the issuer's membership and the linked-group signature.
- Workers write only their own slot, a panicked worker is swept into a failed line, and the summary never summarizes a skipped or failed group.
- Anonymous senders are refused before any parsing or Telegram lookup.

Verification run: `go test -tags testtools -race` for `alita/utils/ratelimit`, `alita/utils/extraction`, `alita/db/user` and the `Staff|staff` tests in `alita/modules` all pass. `make check-translations` and `make check-docs` are clean. All `staff_act_*` keys used from Go exist in all 7 locales with matching placeholders. `golangci-lint` could not run in this environment (built with Go 1.25, repo targets 1.26.0).

No security vulnerabilities or data-loss paths were found. Two robustness defects can make the final report or the per-target exclusivity guarantee unreliable under Telegram flood control, plus several smaller quality items.

## Warnings

### WR-01: Final summary can be lost under flood control; the delivery budget is shorter than the waits it tries to honor

**File:** `alita/modules/staff_action_run.go:245` (budget), `alita/modules/staff_action_summary.go:421-465` (retry loops), `alita/modules/staff_action_run.go:222-231` (progress edits)
**Issue:** `deliverStaffActionFinal` runs on a 15 s context (`staffActionDeliverTimeout`). `staffRetryAfterWait` allows a single wait of up to 60 s (`staffActionRetryAfterCap`). Both `editStaffActionFinal` and `sendStaffSummaryPart` give up when `sleepStaffRetry` returns false, which happens as soon as the 15 s context ends.

The failure sequence on a 429 with `retry_after` above about 15 s:

1. The final edit gets a 429.
2. `sleepStaffRetry(ctx, wait)` returns false at the 15 s deadline and the edit gives up.
3. The fallback `sendStaffSummaryPart` reuses the same, now expired, context. It makes one `SendMessage` call (which ignores the context). That call hits the same chat-level flood.
4. `sleepStaffRetry` returns false immediately, so there is no retry.
5. Every continuation part fails the same way.

The run then logs "final summary could not be delivered" and `setStaffActionCardState(..., done)` still runs. The card in the Staff Group keeps the last progress snapshot, with groups still showing the hourglass, and the bans have been applied. This violates the intent of STAFF-08/STAFF-12 (every group reported, the final edit retried after `retry_after`).

The progress ticker makes this more likely. It edits the same group message every 2.5 s (about 24 edits per minute) and ignores 429s, so it keeps the chat under flood pressure while the run is going.

**Fix:** Give delivery a budget that matches the cap, and use a separate deadline per message instead of one shared 15 s budget. Make progress edits back off on a 429.

```go
// staff_action_run.go
const staffActionDeliverTimeout = staffActionRetryAfterCap*staffActionFinalAttempts + 10*time.Second

// deliverStaffActionFinal: derive a fresh context per message so one slow
// retry cannot starve the fallback and the continuation parts.
func deliverStaffActionFinal(parent context.Context, b *gotgbot.Bot, chatID, msgID int64, text string, continuation []string) {
	step := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), staffActionRetryAfterCap+staffActionEditTimeout)
	}
	ctx, cancel := step()
	ok := editStaffActionFinal(ctx, b, chatID, msgID, text)
	cancel()
	// ...same for sendStaffSummaryPart and each continuation part
}

// progress loop: on a 429 from the progress edit, skip ticks until retry_after has passed.
if wait, limited := staffRetryAfterWait(err); limited {
	nextEditAt = time.Now().Add(wait)
}
```

### WR-02: A 429 block longer than `MaxWait` makes every other paced call sleep through the whole block, which can outlive the target lock

**File:** `alita/utils/ratelimit/telegram_pacer.go:128-167` (`Do` and `wait`), `alita/modules/staff_action_card.go:86` (`staffTargetLockTTL`), `alita/modules/staff_action_run.go:375-379` (`fetchLiveMember`)
**Issue:** The `MaxWait` doc says a `retry_after` above the cap "fails the call at once". That only holds for the caller that received the 429. `Do` calls `p.block(d)` before it checks `d > MaxWait`, so the shared Redis block key is set to the full `retry_after` (which can be thousands of seconds). Every other paced call, on this replica and on every other one, then reserves `slot = now + blk` and `wait` sleeps for the full delay. That sleep is not capped by `MaxWait`, and it is not covered by `staffActionCallTimeout` either, because that timeout is created inside the closure and so only starts after the wait.

Consequences:

- The remaining workers of the run, and every other run, stall silently for the whole block. The card shows the hourglass on those groups until a restart cancels the context.
- The run can outlast `staffTargetLockTTL` (30 min). The target lock then expires while the run is still going, and a second Confirm on the same target can start. That breaks the documented "two staff actions on one person never run at once" guarantee and the restrict-replaces-ban protection it exists for.

**Fix:** Cap the delay a caller will accept in `wait`, and fail with `ErrRateLimited` when the reservation is longer than `MaxWait`. Do not block the whole fleet for a duration you refuse to wait out yourself.

```go
func (p *TelegramPacer) wait(ctx context.Context) error {
	// ...
	delay, ok := p.reserve(ctx)
	if !ok {
		delay = p.reserveLocal()
	}
	if delay > p.opts.MaxWait {
		return fmt.Errorf("%w: slot is %s away", ErrRateLimited, delay)
	}
	// ...
}
```

Optionally also clamp the block set in `Do` to `MaxWait` when the call itself is being failed, and renew the target lock TTL while a run is alive (a heartbeat in the progress loop).

## Info

### IN-01: The "shared" duration grammar is not shared, so the two parsers can drift

**File:** `alita/modules/staff_action_parse.go:187-192`, `alita/utils/extraction/extraction.go:52-118` and `:396-440`, `AGENTS.md` (Staff durations bullet)
**Issue:** The comments and AGENTS.md say `extraction.ParseDurationToken` is "the grammar shared with the per-group /tban and /tmute". It is called only from the staff parser. The per-group commands still use `parseTemporaryDuration`, which uses `strconv.ParseInt` and so accepts a sign (`+5d`). The staff grammar treats `+5d` as plain reason text. `TestParseDurationTokenAgreesWithExistingGrammar` covers in-range tokens only. The claim is false and the duplicate logic will drift.
**Fix:** Either make `parseTemporaryDuration` call `ParseDurationToken`, or correct the three comments and the AGENTS.md line to say the two parsers are kept in step by a test.

### IN-02: `FindUsersByUsername` ordering is not deterministic and can put NULL activity first

**File:** `alita/db/user/repository.go:146-150`
**Issue:** `ORDER BY last_activity DESC` has no tiebreaker, and in PostgreSQL `DESC` sorts NULLs first. `users.last_activity` is a nullable column (migration `20250809000000`). A row with a NULL or equal timestamp can therefore be listed ahead of a more recently active user in the "several users have used @name" refusal list, and equal timestamps return in a nondeterministic order.
**Fix:** `Order("last_activity DESC NULLS LAST, user_id DESC")`. This is PostgreSQL syntax; SQLite 3.30+ also accepts it, so the SQLite test fixtures still work.

### IN-03: `staff_act_hint_bad_target` says only a numeric ID is accepted

**File:** `locales/en.yml` (key `staff_act_hint_bad_target`), same key in the other six locale files; reached from `alita/modules/staff_action.go:169-170`
**Issue:** The text reads "The user must come first, as a numeric user ID." The parser also accepts an `@username` and a text mention. A user who typed `/ban spammer` is told only about numeric IDs, while `staff_act_hint_need_target` correctly lists all three forms.
**Fix:** Reword the key in all 7 locales to name the three accepted forms, e.g. "The user must come first: a numeric ID, an @username or a mention. Nothing was done."

### IN-04: A write cut off by shutdown is reported as "interrupted" although Telegram may have applied it

**File:** `alita/modules/staff_action_run.go:362-365`, `:522-526`
**Issue:** When the run's context is cancelled while a ban or mute request is in flight, `classifyStaffFailure` returns `staffReasonFailInterrupted` for any error. The request may already have been processed by Telegram, so the line reads "failed: interrupted by restart" for a group that was in fact acted on. Re-running is safe because the decision table is idempotent, so this is only a misleading status line.
**Fix:** Word the interrupted reason as "interrupted by restart, result unknown; check the group or run the command again". Update `staff_act_fail_interrupted` in all 7 locales.

### IN-05: Pacer marks Redis as down when its own context is cancelled

**File:** `alita/utils/ratelimit/telegram_pacer.go:180-188`
**Issue:** If the caller's context is cancelled while `reserveSlotScript.Run` is in flight (shutdown), `reserve` treats the context error as a Redis failure. It calls `warnRedisDown`, which logs a warning and switches the pacer to local pacing for 5 s (`pacerRedisBackoff`) even though Redis is healthy.
**Fix:** In `reserve`, return `(0, false)` without calling `warnRedisDown` when `errors.Is(err, context.Canceled)` or `ctx.Err() != nil`.

### IN-06: Any Staff Group member's tap on a card whose Redis hash is gone rewrites the message

**File:** `alita/modules/staff_action_card.go:518-526` and `:577-606`
**Issue:** `loadStaffCardForTap` calls `answerStaffCardExpiredMissing` when the hash no longer exists. That function edits the message to a bare "Expired. Nothing was done." with no issuer check. If an earlier edit that removes the buttons failed, a tap after the hash's 1 h terminal TTL can overwrite a message that carried a real summary. The header is also dropped. This is a narrow edge case, and the final summary normally removes the keyboard.
**Fix:** Edit only when the message text still comes from a pending card (e.g. keep the header with `staffActionHeader`), or answer the toast only and leave the message alone when the hash is missing.

---

_Reviewed: 2026-10-05T08:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
