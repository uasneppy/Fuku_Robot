---
phase: 16
review: 16-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: fixed
    title: "A flood-control wait longer than 15 s silently loses inline \"delete now\" messages instead of queueing them"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "Counted retries have no backoff, so a few seconds of Telegram 5xx or timeouts permanently drops rows"
  - id: WR-03
    severity: Warning
    disposition: fixed
    title: "Switching a rule to Keep or Off (or /cleanservice off) does not stop messages already queued"
  - id: IN-01
    severity: Info
    disposition: fixed
    title: "The /ban and /unban \"reply to a message\" hint is queued as a manual-punishment success reply"
  - id: IN-02
    severity: Info
    disposition: fixed
    title: "/cleanservice no longer deletes join and leave messages in a Staff Group (documented)"
  - id: IN-03
    severity: Info
    disposition: deferred
    title: "A failure between the claim UPDATE and the read-back in ClaimDue strands the claimed rows for two minutes"
  - id: IN-04
    severity: Info
    disposition: fixed
    title: "Test-only helpers in production code"
  - id: IN-05
    severity: Info
    disposition: fixed
    title: "deletion.Schedule does not reject non-group chat IDs"
  - id: NEW-01
    severity: Info
    disposition: deferred
    title: "Fix pass: a send site that read the rule just before a switch-off can still write one row after the drop, and rows queued before the fix deploys are not dropped"
  - id: NEW-02
    severity: Info
    disposition: deferred
    title: "Fix pass: switching the automatic-punishments kind Off drops a mute notice's 24 hour row, so the notice and its button stay in the group"
  - id: NEW-03
    severity: Info
    disposition: deferred
    title: "Fix pass: an inline delete that hits its 10 s call timeout or is cut off by shutdown is still lost, and a deadline hit after the request left may queue a row for a message Telegram already deleted"
  - id: NEW-04
    severity: Info
    disposition: deferred
    title: "Fix pass: the new queue SQL was run on SQLite only"
  - id: NEW-05
    severity: Info
    disposition: deferred
    title: "Fix pass: commit a129472 leaves three tests red until 3555ee5 and 24c766c (bisect note)"
open: 0
total: 13
---

# Phase 16: Code Review Disposition

Findings WR-01 to IN-05 come from `16-REVIEW.md` (deep review of the 33 production files the phase changed: 0 critical, 3
warnings, 5 info, each warning reproduced with a throwaway test, and no breach of any DEL-07 exclusion). NEW-01 to NEW-05 are
the concerns the fix pass reported (`16-REVIEW-FIX.md`) and are dispositioned once here. The seven fixes went through one
single-writer fix pass, each behaviour change as a RED test commit followed by a GREEN fix, and the pass was proved through
CI's own gate (`go test -json` into `scripts/check_test_results`, exit 0: 128 top-level `fuku/modules` tests and 96 `fuku/db`
tests passed, none failed, only the PostgreSQL-only migration tests skipped) plus the whole settings family (474 s, ok). The
orchestrator re-ran build, vet, `go.mod`/`go.sum`, the skip gate, gofmt, the lint stand-in, `make check-docs`,
`make check-translations`, the repository packages and the Phase 16 module families itself before pushing. The security audit
(`16-SECURITY.md`) is the next step and adds its own entries below if it finds any.

## Fixed

- **WR-01** (`490a450` seam, `18e6895` RED, `95badfc` GREEN). `deletionInlineSettle` queues a row due now when the inline
  context's own deadline ran out while the deletion root is still live (a flood-control wait longer than the inline timeout), like
  `ErrRateLimited`; a cancelled root (shutdown) still writes nothing. `deletionInlineTimeout` is a variable so the test can
  shrink it.
- **WR-02** (`a33e6c7` RED, `9b93c18` GREEN). `deletion.Release` takes a retry time: a counted release (5xx, transport error,
  per-call timeout) adds an attempt and moves `delete_at` to `deletionNow() + deletionRetryBackoff` (60 s), so the three tries are
  spaced and a Bot API blip of a few seconds no longer drops rows; an uncounted release (a 429, a shutdown) leaves `delete_at`
  alone. The time is computed in Go, so SQLite and PostgreSQL run the same statement. `TestDeletionWorkerClassifiesErrors` and
  the repository test "the third counted release drops the row" legitimately changed (the retries follow the backoff on the test
  clock).
- **WR-03** (`b798c81` RED, `a129472` GREEN, `3555ee5` and `24c766c` test follow-ups). Removing a rule, Keep on joined or left and
  `/cleanservice off` drop the chat's pending queue rows of the affected kind(s) in the same transaction as the rule change, by an
  exact kind match (`kind IN ?`, never a prefix), never an `acting` row, never another chat or kind; a delay change and Now or
  After N drop nothing. Two bot-message tests and one mute-notice test expected the old (surviving rows) behaviour and now expect
  the rows to go with the rule.
- **IN-01** (`210dd86` RED, `08d309a` GREEN). The two `/ban` and `/unban` reply-to-a-message hints go through `replyHint`, the
  same request as `replySuccess` without scheduling (D-13: validation replies are not covered).
- **IN-05** (`940e2b5` RED, `f985759` GREEN). `Schedule` refuses a non-negative chat ID with `ErrInvalidEntry` before any read or
  write, so "never a private chat" no longer rests only on the callers.
- **IN-04** (`b925737`). `deletionRowOf`, `deletionRowOfKind` and `deletionFindRow` moved to a `_test.go` file, `deletion.Kinds`
  and `deletion.IsBotKind` to the `testtools`-tagged `hook_testtools.go`; none had a production caller.
- **IN-02** (`d4fa04c`, docs only). AGENTS.md says join and leave cleaning does nothing inside a Staff Group (D-18). It joins the
  owner-confirmation list of `16-SPIKE.md` as a behaviour change of `/cleanservice`.

## Deferred (accepted limits, none blocks the phase)

- **IN-03.** A failed read-back right after the claim `UPDATE` (a cancel at shutdown, a dropped connection) strands the claimed
  rows `acting` until `ReleaseStale` returns them after 2 minutes. It is bounded, self-healing, and already written down in
  AGENTS.md ("claimed, until the 2 minute stale release").
- **NEW-01.** A send site that read the rule a moment before an owner switched it off can still write one row after the drop, so
  one message may be deleted after the switch-off; rows queued by a build that predates the fix are not dropped retroactively.
  The window is milliseconds and the effect is one deletion the owner just declined, of a message the bot itself sent.
- **NEW-02.** Switching the automatic-punishments kind to Off now drops a mute notice's 24 hour row, so that notice and its "I
  fixed it" button stay in the group for good. Off means "never delete", which is what the kind did before this phase; the button
  keeps working.
- **NEW-03.** An inline delete that hits its own 10 s call timeout, or is cut off by the shutdown, is still lost (one message stays
  in the chat), and a deadline hit after the request left may queue a row for a message Telegram already deleted, which the worker
  drops quietly as refused. The durable path handles every other shape.
- **NEW-04.** The new queue SQL (an `IN` delete and a `delete_at` update with a Go-computed time) was run on SQLite only. It uses
  no construct the two databases treat differently, and `make test-postgres-integrity` holds the guarantees that need a
  PostgreSQL proof (the partial indexes, the CHECKs, the conditional claims). The two repository tests the fix report names can
  join that list if the owner wants a PostgreSQL run.
- **NEW-05.** Commit `a129472` (the WR-03 fix) leaves three tests red until `3555ee5` and `24c766c` update their expectations; a
  bisect landing between them will see those failures. History is not rewritten; the head is green.
