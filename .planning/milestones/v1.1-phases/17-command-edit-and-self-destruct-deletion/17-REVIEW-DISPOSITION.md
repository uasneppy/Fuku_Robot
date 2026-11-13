# Phase 17 - Review and security disposition

> Written by the orchestrator after the fix pass (`17-REVIEW-FIX.md`). Every finding of `17-REVIEW.md` and every non-blocking finding of
> `17-SECURITY.md` has a disposition. The decisions D-01 to D-34 and the owner confirmations A17-1 to A17-8 are provisional (the owner
> has not confirmed them); nothing here changes them.

| Finding | Severity | Disposition | Commit | Note |
|---------|----------|-------------|--------|------|
| WR-01 admin status costs a live `getChatMember` per message on a cold cache | medium | fixed | b53cd8b, 1ceae78 | the cache is warmed once through `cache.LoadAdminCache` (singleflight) behind the same semaphore and 3 s timeout, the live per-user read is the fallback, unknown still keeps the message; a 30 s per-chat backoff after a load that stores nothing. Behaviour change: the first non-exempt message after the admin cache expires costs one `getChatAdministrators` instead of one `getChatMember` per message |
| WR-02 batcher never reads the delete-right latch | low-medium | fixed | ccd711d | one latch read per chat per flush; a failed read writes as before. Same as SEC-17-02 |
| WR-03 pin drop is one-shot and misses the buffer window | low | fixed | c8c7022 | bounded in-process set of recent pins (10 s, 4096 keys) checked right before the write, plus one retry of the drop. Closes the window on the pinning replica only; a pin handled by another replica stays AR-5 |
| IN-01 worker logs per chat per cycle when rules are unreadable | info | fixed | b8864e0 | once per chat per hour, release semantics unchanged. Same class as SEC-17-01 |
| IN-02 one 5 s context for screening and write | info | fixed | fc42977 | the write has its own bounded detached context; the stop's final flush keeps its 2 s |
| IN-03 every pin in every chat does a synchronous DB write | info | fixed | afcba8c | the cached rule is read first; no database call without the rule; an unreadable rule keeps the old behaviour |
| SEC-17-01 failure-path Warns without a throttle | low | fixed | b53cd8b, 1ceae78, b8864e0 | one gate per line kind, hourly per chat, IDs only |
| SEC-17-02 watcher and batcher ignore the latch | low | fixed | ccd711d | see WR-02 |
| SEC-17-03 README overstates the exemptions | info | fixed | 7bdffab | scoped to Edited messages and Self-destruct; `docs/` had no copy |
| SEC-17-04 no rolling-deploy caveat for member kinds | info | fixed | 4f297a9 | one sentence in AGENTS.md |
| SEC-17-05 "nothing is deleted after Off except a call in flight" too strong | info | fixed (accepted as AR-9) | 62ad877 | AGENTS.md, the worker comment and `17-SPIKE.md` say a claimed and screened batch can finish within the cycle (90 s guard); the planning history files record the decision as made and are unchanged |

## Process notes

- The whole-package run of the fix pass caught two failures its focused runs missed (`TestMemberDeletionLogsCarryIDsOnly` and the
  cold-cache 429 case of `TestMemberDeletionEndToEnd`); `1ceae78` fixed them by giving each log line its own gate and keeping the
  admin cache cold through the `memberDeletionAdminWarm` seam. No assertion was weakened; the warmable case has its own test
  (`TestMemberDeletionAdminWarm`).
- Earlier in the phase CI's whole-package run caught one cross-plan interaction (plan 17-04's `self-destruct rows` subtest queued rows
  with no rule, which plan 17-06's worker screen drops); `4582725` stores the rule in that subtest. Lesson kept: run the whole package
  before a plan boundary that follows a worker or rule-screen change.
