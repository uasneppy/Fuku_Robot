# Quick task 261117-w7q: member requirements review fixes (Wave 7) — summary

Branch `wave7-memberreq`, based on `90f7cb4`. Every fix was written test first and the test failed on the old code.

## Fixed

| Item | Commit | What changed | Test |
|------|--------|--------------|------|
| Q1 | c5278d6 | During a cap pause the executor deletes a message only when the case's own names break a name rule (`memberReqNameBreach`, the watcher's check); a lookup-miss case is never deleted. | `TestMemberReqDeleteDuringPause/a lookup miss with clean names is not deleted (Q1)` |
| Q2 | 6b41af4 | The per-chat cap counts members, not jobs: one Lua script (`memberReqCapScript`) `SADD`s the user to `fuku:memberreq:{<chat>}:capm:<minute>` and `INCR`s `:cap:<minute>` only for a new member, both with `EXPIRE 120`. `memberReqCapAllow` takes the user ID. AGENTS.md Data bullet documents the new key. | `TestMemberReqCapCountsMembers` |
| Q3 | f02b003 | The watcher never counts the AI part satisfied while the member holds the join marker `nu`, so a joiner whose names have a cached pass gets one job that records the baseline and deletes the marker (no provider call); the next message needs no job. | `TestMemberReqWatcherAI` (new job case, new follow-through subtest, pure table rows) |
| Q4 | 4087c25 | `memberReqUndeliveredMute` reports whether it lifted the mute; the executor then answers `memberReqNotApplied`, so the join entry welcomes the joiner. | `TestMemberReqJoinNoCaptcha/a join mute whose notice was undelivered and lifted is welcomed (Q4)` |
| Q5 | a122035 | The captcha guard also keeps a pending mute hold's mute when the live record carries its marker (`memberReqPendingMuteLive`, one paced `getChatMember`); a read failure fails closed like an unreadable hold. | `TestMemberReqCaptchaGuard/a pending mute hold keeps its live marker and unmutes without one (Q5)` |
| Q6 | 3ee42a6 | The recorded-warn note lives `StaleClaim` plus 7 days (was plus 10 minutes). | `TestMemberReqWarnRecovery/the note outlives a reclaim that comes hours later (Q6)` |
| Q7 | 0e0fb28 | Comments only: the AI rule's order (baseline before the verdict cache), the budget fault is a sticky unknown, the punishing path asks the bot's channel standing. | none (docs) |

## Not reproduced

None.

## Owner decisions

None new. Q2 changes what the cap measures (members per minute instead of punishment attempts per minute); a member counted
within the cap stays allowed for that minute, and repeated refused attempts for one member no longer reach the pause on
their own. This is the plan's stated fix.

## Existing tests changed

- `TestMemberReqCapPausesPunishments` ("a new minute counts afresh", "two replicas tipping the cap at once post one notice"):
  only the call sites of `memberReqCapAllow`, which now takes a user ID; each call passes a distinct user, so the asserted
  behaviour is unchanged.
- `TestMemberReqWatcherAI` pure-decision table: the rows "below the bar" and "just below the bar" asserted that a cached
  pass satisfies a marker holder (the Q3 bug). They now run without a marker (still satisfied) and two new rows assert that
  the same verdicts with a marker are not satisfied.

No `TestLegacy*Requests` golden was touched. No locale key was added.

## User-visible behaviour changes

- During a cap pause, a message of a member handed to a job only for a photo, channel or AI lookup miss is no longer deleted.
- The pause starts after more than 10 different members in a minute, not after more than 10 punishment attempts.
- A joiner whose names have a cached AI pass gets their baseline recorded at their first message (one job, no provider call).
- A joiner whose mute notice could not be delivered (and whose mute was lifted again) now gets the welcome.
- A captcha pass no longer frees a member under a pending requirement mute whose marker is live.
- A member whose recorded warn could not be marked is not warned twice when their next violation comes hours or days later.

## Checks

- Per item: the named tests and nearby families; at the end `go test -tags testtools -race -count=1 -run '^TestMemberReq' ./fuku/modules` passed (543 s).
- `CGO_ENABLED=0 go build ./...`, `make check-translations` (all present), golangci-lint v2.5.0 built with Go 1.26: 0 issues.

## Left unfinished

Nothing.
