---
phase: 13-member-requirements
fixed_at: 2026-11-08T00:00:00Z
review_path: .planning/phases/13-member-requirements/13-REVIEW.md
base_head: 5e9fb2b
status: all_fixed
findings_in_scope: 4
fixed: 4
skipped: 0
---

# Phase 13: Code Review Fix Report

Scope: CR-01, WR-01, WR-02, WR-03. IN-01 to IN-06 were left to the orchestrator's disposition and no code was
changed for them. Each fix was test-first: a RED commit whose new assertions fail for the stated reason, then the GREEN
fix. AGENTS.md was updated in the same commit as each fix; the SPIKE rewrite is a separate docs commit.

## CR-01 (Critical): a requirement Mute replaced a restriction someone else placed

- **RED:** `a1820fd` test(13): a requirement Mute must not replace a restriction someone else placed
- **GREEN:** `339d63a` fix(13): never mute a member over a restriction someone else placed
- **Files:** `fuku/modules/memberreq_enforce.go`, `fuku/modules/memberreq_enforce_test.go`,
  `fuku/modules/memberreq_join_test.go`, `AGENTS.md`
- **What changed:** after the live lookup and before the cap and the claim, `memberReqEnforce` calls
  `memberReqMuteOverRestriction`. When the strictest action is Mute, the live record is `restricted` and no marker of this
  episode is live (the stale hold's marker that `memberReqLiveMarker` adopts keeps working), it refuses: `memberReqNone`,
  no hold, no notice, no delete, no further Telegram call beyond one `getChat`, and one Info line with chat and user IDs.
  Warn, Kick and Ban are untouched.
- **Decision beyond the brief (please review):** the literal rule "restricted and no marker" broke three existing tests
  (`TestMemberReqMuteNoticeUndeliverable` first-message-after-StaleClaim, `TestMemberReqJoinAfterCaptcha` button pass,
  `TestMemberReqRejoinResetsMuteHold` leave and rejoin). The cause is real, not a fixture quirk: in a group whose default
  permissions are restrictive, `restrictChatMember` with the group's defaults (what the captcha pass, `/unmute` and every
  requirement lift send) leaves the member `restricted` with the group's rights, so the literal rule would never mute
  that member again, and the captcha-then-Mute-on-join flow would silently never mute in those groups. So the refusal
  applies only when the restriction takes a right away from the group's defaults: the executor reads the defaults with one
  paced `getChat` through `memberReqUnmutePermissions` (the same read the lifts use, lockdown snapshot included) and
  `memberReqRestrictedBelow` compares the 13 plain rights. A `getChat` failure refuses the mute (fail closed). An admin's
  permanent mute, a timed mute and a partial media restriction are all below the defaults and are refused. No existing test
  was edited. This rests on Telegram reading a member restored to the group's defaults back with the group's rights, which
  is new SPIKE check 14.
- **Reachability of the lifts:** `memberReqUndeliveredMute` and the "I fixed it" lift are reached only for a mute this code
  placed on a plain member, a member at the defaults, or an adopted marker, so no test was added for a remaining path.
- **Tests:** `TestMemberReqMuteNeverReplacesRestriction` (no mute over a partial restriction with no end date or a timed
  one: outcome none, no hold, no write, delete or message, restriction untouched, exactly one Info line with IDs and no
  name; Warn, Kick and Ban still act on a restricted member; a member restricted only to the group's defaults is muted;
  a restriction below the defaults is refused; unreadable defaults refuse; the stale live marker is still adopted),
  `TestMemberReqJoinNeverReplacesRestriction` (a restricted rejoiner on the join path: no mute, no notice, no hold,
  restriction untouched).

## WR-01 (Warning): a recorded warn could be punished twice

- **RED:** `7bdc7c9` test(13): a recorded warn must not be punished twice (its only non-test change is the
  `memberReqMarkHold` and `memberReqMarkRetryWait` seams, with no behaviour change)
- **GREEN:** `9663ac8` fix(13): never punish a recorded warn twice
- **Files:** `fuku/modules/memberreq_enforce.go`, new `fuku/modules/memberreq_warned.go`,
  `fuku/modules/memberreq_enforce_test.go`, `AGENTS.md`
- **What changed:** (a) `memberReqMarkPunished` tries 3 times (100 ms apart) on one detached 5 s record context and reports
  whether it landed. The warn path writes a Redis note `fuku:memberreq:warned:<chat>:<user>` (value the hold ID, TTL
  `StaleClaim` plus 10 minutes) inside `afterRecord` before the mark and deletes it once the mark landed. The reclaim of a
  stale pending Warn hold for a Warn action (`memberReqAdoptRecordedWarn`, after the claim was won) finds the note naming
  that hold, marks it punished, warns and announces nothing and returns `memberReqHeld`. No migration. No Redis or a
  Redis error means "no note" and the reclaim warns again (accepted, documented in AGENTS.md next to the key). The ID in the
  value stops a note an earlier episode left from matching a later hold. (b) An `applyWarn` error with `Recorded` true marks
  the hold, sets `LimitApplied` from `LimitReached` with no `LimitErr` (the reset failed after the action ran), posts the
  notice and returns Warned or Removed; `Recorded` false keeps the backoff.
- **Tests:** `TestMemberReqWarnRecovery`: a mark that fails once is retried; a mark that fails three times leaves the hold
  pending, the reclaim adopts it with one warn and one notice in all and the note is dropped; a note naming another hold ID
  does not match; without Redis the reclaim warns again; a recorded-then-failed warn below the limit and with the limit
  action run posts the notice, stores its ID, marks the hold and returns the right outcome. The existing "a warn that cannot
  be recorded backs off" tests still cover `Recorded` false.

## WR-02 (Warning): a switched-off rule still punished from an in-flight job

- **RED:** `6b4636d` test(13): a job must honour settings changed after its message
- **GREEN:** `59cbf9a` fix(13): honour a rule switched off while an enforcement job waited
- **Files:** `fuku/modules/memberreq_enforce.go`, `fuku/modules/memberreq_enforce_test.go`,
  `fuku/modules/memberreq_join_test.go`, `AGENTS.md`
- **What changed:** `memberReqLiveViolation` takes `*memberReqCase`; after the live lookup it re-reads
  `memberReqSettingsLookup`, returns no violation on an error (an unreadable setting punishes nobody) and stores the fresh
  row in `c.Settings`. Broken rules, the strictest action and the Delete messages switch of everything after it come from
  that row. The held and paused paths before the live lookup still use the snapshot for their deletes (as the review
  scoped it). The join entry's synchronous call goes through the same function.
- **Tests:** `TestMemberReqEnforceReadsSettingsAfterLookup` (a rule switched Off, Delete messages switched off, a rule
  changed Mute to Warn punishing with Warn, Delete messages switched on deleting, an unreadable setting punishing nobody)
  and `TestMemberReqJoinReadsSettingsAgain` (the join entry's read then the executor's: a rule that went Off in between
  punishes nobody and the member is welcomed).

## WR-03 (Warning): the rejoin reset skipped a pending mute hold's live marker

- **RED:** `c09480d` test(13): the rejoin reset must lift a pending mute hold's live marker
- **GREEN:** `cdf0b7c` fix(13): lift a pending mute hold's live marker at the rejoin
- **Files:** `fuku/modules/memberreq_join.go`, `fuku/modules/memberreq_rejoin_test.go`, `AGENTS.md`
- **What changed:** `memberReqJoinReset` tests `Action == models.RequirementActionMute` instead of
  `memberReqPunishedMute`; `memberReqLiftOldMute` still lifts only while the live record carries the hold's marker and the
  hold is restored with its ID on a failed lift.
- **Tests:** `TestMemberReqRejoinResetsPendingMuteHold`: a pending hold with a live marker is lifted; one without a marker
  makes no Telegram write; a failed lift restores the pending hold with its ID.

## Docs

- `781db20` docs(13): the SPIKE's restricted-member accepted limit rewritten for CR-01 (a partially restricted member who
  breaks a Mute rule is not muted, so the owner should use Warn, Kick or Ban there), the "A4 for restricted members" and
  check 4 lines adjusted, and new live check 14 (a restricted member is not muted, a member lifted to the defaults of a
  restrictive group still is). AGENTS.md: the executor bullet (fresh settings, the Mute refusal, the warn recovery), the
  Data bullets (the new `fuku:memberreq:warned:*` key, the rejoin reset), the join bullet and the testing families. The
  member requirements bullets were re-read at the end against the code.

## Gate (real results)

| Command | Result |
| --- | --- |
| `gofmt -l` on the touched files (`memberreq_enforce.go`, `memberreq_warned.go`, `memberreq_join.go` and the four test files) | no output |
| `CGO_ENABLED=0 go build ./...` | ok |
| `go vet -tags testtools ./fuku/modules ./fuku/db/memberreq` | no output (clean) |
| `python3 .../handoff/lint-local.py 8c0c827`, exit code read directly | exit 0: "no dupl, gocyclo or godox findings on lines added since 8c0c827" |
| `-race -count=20` of `TestMemberReqWarnRecovery`, `TestMemberReqMuteNeverReplacesRestriction`, `TestMemberReqJoinNeverReplacesRestriction`, `TestMemberReqEnforceReadsSettingsAfterLookup`, `TestMemberReqJoinReadsSettingsAgain`, `TestMemberReqRejoinResetsPendingMuteHold` | ok |
| `go test -tags testtools -race -count=1 -timeout 30m ./fuku/modules` (background) | first run: one failure, `TestMemberReqJoinAndMessageOnce/a_message_and_then_the_join_give_one_warn_and_one_notice` (two warns); second run on the same HEAD: `ok ... 708.348s`, exit 0 |
| `-race -count=30` of `TestMemberReqJoinAndMessageOnce` | ok (30 of 30) |
| `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/i18n` | ok, ok |
| `make check-translations` | all translations present |
| `make check-docs` | no drift |

The one failure of the first whole-package run is a second-boundary flake that is not caused by this pass. The join
reset deletes holds created before the join's date, which has one-second resolution (`memberReqJoinDate`), and that test
sends a message and then a join within milliseconds: when the second ticks over between the hold's claim (before the
warn is stored) and the join's date, the message's hold counts as older than the join, is deleted and a second warn
follows. It passed 30 of 30 alone and in the clean whole-package run. It was not fixed (out of scope, pre-existing).

## Edited existing tests

None. Every existing test passes unchanged.

## Not run

`make test` and `make lint` (the golangci-lint here is built with Go 1.25; the stand-in stood in), the PostgreSQL
integrity target and the live Telegram SPIKE checks (including the new check 14).
