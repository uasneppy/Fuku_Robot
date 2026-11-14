---
phase: "17"
slug: "command-edit-and-self-destruct-deletion"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate, "high")
threats_open: 0
threats_total: 48
threats_closed: 48
findings_open_nonblocking: 0
findings_fixed: 5
findings_accepted: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-12"
---

# Phase 17 - Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `fd40ac2` (diff `42c48c4..fd40ac2`, all 12 plans). Every mitigation was read
> in the code, not taken from a plan or summary, and the security-relevant tests were run (68 named Phase 17 top-level tests in
> `fuku/modules` and `fuku/i18n` with `-race`, the `fuku/db/deletion` and `fuku/utils/helpers` packages, `make check-translations`,
> `make check-docs`, `go build`, `go vet`). Depth is ASVS L1 (`security_block_on: high`) with L2-depth traces on the DEL-07,
> authority, fail-safe and cross-chat paths. As for Phases 15 and 16 the mandate was wider than "verify the register": it also hunted
> for what a group member, a forged callback, a failing dependency or a rolling deploy can actually cause. This audit's own findings
> are `SEC-17-nn`. Paths are under `fuku/modules/` unless a path is given; `dl` = `fuku/db/deletion`.
>
> The auditor role is read-only: this file was persisted by the orchestrator from the auditor's report, with the evidence column
> shortened. Evidence limits: no live Telegram run (`17-SPIKE.md` holds those checks); `make test-postgres-integrity` and `make lint`
> were not run by the auditor (a PostgreSQL cluster cannot live in its scratchpad). The PostgreSQL-only SQL was read: the `LEAST`
> upsert, the multi-row `ON CONFLICT` and the bounded count mirror Phase 16's `Schedule`, and the four new names
> (`TestScheduleBatchKeepsEarliest`, `TestScheduleBatchDuplicateKey`, `TestCountPending`, `TestDropPendingMessage`) are in the
> Makefile integrity list, which CI ran green at `dd2a28b` and later.

---

## Trust Boundaries

| # | Boundary | Data crossing |
|---|----------|---------------|
| 1 | Member message -> `CommandDeletionProcessor` post-step (after all handler groups) | the message, its sender, command text, chat; only this bot's registered commands may be deleted |
| 2 | Anonymous-admin proof tap -> re-entered update | cached command message, the tapper, callback chat vs cached chat |
| 3 | `edited_message` update -> edit handler (group -8) | sender, date, edit_date, content: attacker-controlled |
| 4 | Member message -> self-destruct watcher (group -8) -> bounded batcher -> `scheduled_deletions` | entries (chat, message, kind, sent time, delay); never text or user IDs |
| 5 | Exemption lookups (approvals, admin cache and live `getChatMember`, captcha) -> the decision | a failed read must never mean "not exempt" |
| 6 | Pin service message / `/pin` -> queue | chat from the service message, message ID named by it |
| 7 | Settings menu (`dl`, `dr`, `st`, `dy`) -> rules | callback data (attacker-controlled), live member record, owner gate, card expiry |
| 8 | Bot -> Telegram `deleteMessage(s)` through the deletion pacer; worker -> queue rows | per-chat chunks of 100, fleet-wide pacer, claim tokens |
| 9 | Shutdown -> batcher and worker | one 3 s wait inside the 60 s budget |
| 10 | Staff Group, lockdown, captcha, log and bot messages -> the deletion code (DEL-07) | none may cross: `deletionAct`, `Schedule`/`ScheduleBatch`, worker, page `Hidden` |

---

## Threat Register

48 threats: 47 `mitigate` plus T-17-SC, all CLOSED; T-17-22 is the one `accept` (AR-1).

| Threat ID | Category | Component | Sev | Status | Key evidence (read in code) |
|-----------|----------|-----------|-----|--------|-----------------------------|
| T-17-01 | Tampering | delay outside a kind's presets | med | CLOSED | `dl/kinds.go` `ValidateRuleDelay` by value, before any write in `dl/rules.go`; readers treat an unaccepted stored value as Off |
| T-17-02 | Tampering | member entry with a wrong delay | low | CLOSED | `ValidateEntryDelay`; `validateEntry` over all entries before any write |
| T-17-03 | Tampering / InfoDisc | Staff Group through the batch writer | high | CLOSED | `dl/queue.go` one fresh Staff Group read per distinct chat, a failed read drops that chat's entries |
| T-17-04 | DoS | duplicate key failing a PostgreSQL batch | med | CLOSED | `mergeBatch` dedupes in Go, sorts, chunks of 100 |
| T-17-05 | DoS | ceiling count scanning a big table | low | CLOSED | bounded `LIMIT` subselect, parameterised |
| T-17-06 | Tampering | hand-kept command list drifts | med | CLOSED | `helpers/command_registry.go` is the only recording site |
| T-17-07 | Tampering | command built outside the shim | low | CLOSED | `TestNoRawNewCommand` (go/parser; aliases and literals covered) |
| T-17-08 | Repudiation | generated docs drop commands | low | CLOSED | `scripts/generate_docs/parsers.go`; `make check-docs` no drift |
| T-17-09 | Tampering | another bot's or an unknown command deleted | med | CLOSED | `botCommandName` mirrors the library's matcher; `helpers.IsBotCommand`; 30 messages x 8 names differential test. Bare-name race: AR-2 |
| T-17-10 | Spoofing | `sender_chat`, bot, forward, service ID | med | CLOSED | `command_deletion.go` sender filter incl. `staffServiceUserIDs` |
| T-17-11 | DoS | post-step blocks or fails under a cancelled context | med | CLOSED | own recover, fresh context, delay 0 through `deletionAct` (64 inline goroutines, durable fallback) |
| T-17-12 | Tampering | command deleted before the reply | med | CLOSED | delete follows `Inner.ProcessUpdate`; ordering tests |
| T-17-13 | Spoofing / Repudiation | anonymous-admin proof replay | med | CLOSED | `bot_updates.go` chat triple, proof button deleted at the first tap, `SenderChat` cleared, tapper as sender. AnonAdmin-on limit: AR-3 |
| T-17-14 | Tampering / InfoDisc | command deleted in a Staff Group | high | CLOSED | `deletionAct` refuses a Staff Group (a read error counts as one) |
| T-17-15 | DoS | rightless group hammered | med | CLOSED | `NeedsDeleteRight` at every latch site; `TestMemberKindsNeedDeleteRight` |
| T-17-16 | Tampering | late reply after the delete | low | CLOSED | reply-guard test, empty allow-list. Name-based limit: AR-7 |
| T-17-17 | InfoDisc | message ID in a log | low | CLOSED | no log call in `command_deletion.go`; refused delete dropped quietly |
| T-17-18 | Spoofing | via_bot, channel, forward, service ID on edits | med | CLOSED | `raidFloodSenderCounts`, `IsBot`, `ViaBot` before any I/O |
| T-17-19 | Tampering / EoP | admin or approved message deleted when a read fails | high | CLOSED | tri-state approvals, tri-state admin status, captcha error = pending; none of `IsUserAdmin`, `IsApprovedForUpdate`, `userIsFloodExempt`, `aispamCarvedOut` is called in the new files. Residual: AR-4 |
| T-17-20 | Tampering / InfoDisc | edit deleted in a Staff Group | high | CLOSED | edit handler goes through `deletionAct` |
| T-17-21 | DoS | handler ends updates, panics into nil, or live read blocks | med | CLOSED | named result preset to `ContinueGroups` plus recover; semaphore of 8 that never waits; 3 s timeout |
| T-17-22 | Tampering | no-op edit deleted | low | CLOSED (accepted) | AR-1 |
| T-17-23 | DoS | flood grows the queue table | med | CLOSED | 4096 buffer, 20,000 exact ceiling, shed latest first |
| T-17-24 | DoS | dispatcher blocked by queue writes | high | CLOSED | watcher does reads only; non-blocking offer to the batcher |
| T-17-25 | Tampering | deleted after the owner turned it off | med | CLOSED | flush-time rule check, worker-time rule screen, same-transaction drop in `RemoveRule`. Residual: AR-9 |
| T-17-26 | DoS | stop overruns the 60 s budget | med | CLOSED | worker and batcher under one 3 s timer, final flush 2 s on a detached context; main.go diff is the Processor line only |
| T-17-27 | InfoDisc | shed log with IDs or one line per shed | low | CLOSED | one Warn per chat per hour, chat ID and reason only, bounded map |
| T-17-28 | Spoofing | channel, anonymous admin, forward, bot, service ID queued | med | CLOSED | library drops bot senders without `SetAllowBot`; `raidFloodSenderCounts`; `memberContentMessage` |
| T-17-29 | Tampering / EoP | admin or approved message self-destructed | high | CLOSED | shared exemption incl. failing-read rows. Residual: AR-4 |
| T-17-30 | Tampering | pin used to drop another chat's row, or a pinned message deleted | med | CLOSED | chat from the service message; `DropPendingMessage` matches chat, message, kind and pending state; hook only after `pinChatMessage` succeeded. Races: AR-5 |
| T-17-31 | DoS | busy group starves replies or other kinds | med | CLOSED | own pacer keys, batcher touches the database only, 1000-row claims, chunks of 100 |
| T-17-32 | InfoDisc / Tampering | any rule in a Staff Group | high | CLOSED | `deletionAct`, `ScheduleBatch`, worker screens; all three rules tested |
| T-17-33 | Tampering | translation with markup or a mismatched placeholder | low | CLOSED | locale key and placeholder tests |
| T-17-34 | Repudiation | copy overstates or names the Staff Group | med | CLOSED | `TestSettingsStaffPrivacy` (8 languages, both menus). README nit: SEC-17-03 |
| T-17-35 | Tampering | forged `dy`, `st` or `e` | med | CLOSED | `settingsDelYesFields`, preset validator, canonical digits, `ValidateRuleDelay` |
| T-17-36 | EoP | Yes after demotion or expiry | med | CLOSED | `dy` right `settingsRightChangeInfo` on a fresh member record; `settingsCardExpired` before the write |
| T-17-37 | Tampering | renderer change alters a Phase 16 screen | low | CLOSED | `TestSettingsDeletionPhase16ScreensUnchanged` |
| T-17-38 | DoS | dead button over 64 bytes or an overlong screen | low | CLOSED | callback-bytes and fit-every-locale tests (57 of 64) |
| T-17-39 | EoP | administrator without change-info switches self-destruct on | high | CLOSED | every row control and `dy` need `settingsRightChangeInfo`, Off included |
| T-17-40 | InfoDisc | Staff Group named on a member screen | med | CLOSED | `Hidden`, `settingsDelStaffGate` at the top of `dl`, `dr`, every control and `dy` |
| T-17-41 | Repudiation | page claims what producers do not do | med | CLOSED | one cached `delete_rules` key in `skipLocal` read by page and producers; drift tests |
| T-17-42 | DoS | draw makes more than one Telegram call | low | CLOSED | `TestSettingsDeletionCost` |
| T-17-43 | Tampering | toggle or forged action slips past step 1 | low | CLOSED | action allow-list and gate-door tests; controls declare `st` only |
| T-17-44 | Tampering / InfoDisc | member rule reaches an excluded message, or a log leaks data | high | CLOSED | exclusion and log-sweep tests; the only callers of the deletion core added here are the command wrapper, the edit handler and the batcher |
| T-17-45 | Tampering | an exemption read that fails deletes in the full slice | high | CLOSED | "unknown never deletes" subtests of the end-to-end test |
| T-17-46 | Repudiation | docs claim behaviour the code lacks | med | CLOSED | AGENTS.md read against the code; `make check-docs`. Nits: SEC-17-03, SEC-17-05 |
| T-17-47 | Repudiation | requirement ticked before proof | med | CLOSED | REQUIREMENTS.md diff flips only DEL-03 to DEL-05, after the end-to-end commits |
| T-17-SC | Tampering | `go.mod` and `go.sum` | high | CLOSED | `git diff --exit-code 8c0c827 -- go.mod go.sum` exits 0; no migration, env var, secret, exec or HTTP call added |

`threats_open: 0`. No unregistered flags in the SUMMARY files.

---

## New Threats Found by the Audit (unregistered, none blocking)

All are below `high`, so none counts toward `threats_open`. Dispositions below are the fix pass's (see the Audit Trail).

- **SEC-17-01 (low, DoS by log volume): the failure paths of the member rules log one Warn per message or per cycle, with no throttle.**
  `member_deletion.go` (semaphore full, admin status unknown, approvals unknown, captcha unknown) logs per message with user and chat
  IDs, and `deletion_worker.go` `screenSelfDestruct` (rules unreadable) logs per chat per cycle. A Telegram incident, a cold or
  negative admin cache or a Redis or DB fault in a busy group with Self-destruct on writes a Warn per message and can bury real
  alerts. Nothing leaks beyond IDs other bot logs already carry. Recommendation: route through the existing hourly gate or Debug.
- **SEC-17-02 (low, DoS): the self-destruct watcher, the batcher and `ScheduleBatch` never read the delete-right latch.** In a group
  where the bot cannot delete, every member message still costs the rule, approval and admin reads plus a queue insert, a claim and a
  drop until the 20,000 ceiling, and a cold admin cache adds a live `getChatMember` per message (at most 8 in flight). No Telegram
  delete is made. Recommendation: one `deletionLatched` read per chat per flush in `memberDeleteScreenChat`. (Same as review WR-02.)
- **SEC-17-03 (info, Repudiation): README overstates the exemptions.** It says admins, approved members and pinned messages are
  exempt from the member rules, but Commands deletes an admin's command by design. Recommendation: scope the sentence to Edited
  messages and Self-destruct.
- **SEC-17-04 (info): no rolling-deploy caveat is recorded for the member kinds.** During a rollout an old-code worker claims
  `member:*` rows without the self-destruct Off screen and with a service-only latch. Transient and additive; document one sentence
  in AGENTS.md.
- **SEC-17-05 (info): "nothing is deleted after Off except a call already in flight" is slightly too strong.** A cycle that claimed
  and screened its rows before the Off press keeps deleting its remaining chunks (up to the 90 s cycle guard). Accept (AR-9) and
  reword the AGENTS.md sentence.

---

## Accepted Risks Log

| # | Risk | Why accepted |
|---|------|--------------|
| AR-1 | T-17-22 / A17-3: any edit past the grace counts, a link-preview or caption touch included (no stored copy of the original) | D-04 and the deferred "compare with the original" idea; bounded by the grace; stated on the page, in AGENTS.md and README; live check 1 in `17-SPIKE.md` |
| AR-2 | A17-1: a bare `/word` another bot also handles may be deleted after this bot answers and break that bot's reply | the matcher cannot tell which bot a bare word was for; `@otherbot` and unregistered words are never touched; live check 2 |
| AR-3 | D-34 / A17-2: with the AnonAdmin setting on, an anonymous admin's command runs at its first update (`sender_chat` set) and stays; with it off it is deleted only after the proof tap | the wrapper cannot tell a fully handled anonymous command from a proof prompt; fails toward keeping; live checks 3 and 4 |
| AR-4 | exemptions are judged at send time (self-destruct) or edit time: a member approved or promoted later still loses queued messages; a stale admin cache can read an admin as a member | rows hold no user ID by design (no migration, no PII); promotion invalidates the admin cache; an unknown always keeps the message |
| AR-5 | pin races: a pin landing while the message is in the batcher buffer (about 300 ms) or after the worker claimed its row still deletes it; a pin by another bot is never seen; unpinning does not re-queue | bounded and documented (A17-7, live check 5); the bot's own `/pin` goes through the hook |
| AR-6 | the per-chat ceiling: past 20,000 queued rows new messages are kept | D-31, A17-6; sheds toward keeping, the table stays bounded |
| AR-7 | the reply guard resolves calls by function name and the caller's receiver only | reviewed in plan 17-11 (no real case, empty allow-list); a late reply to a deleted command only fails to send |
| AR-8 | a member flooding registered commands with Delete commands on costs at most one paced call or one durable row per command, from the fleet-wide deletion pacer | inherits Phase 16 AR-1; bot replies use other pacers; the worker batches 100 IDs a call |
| AR-9 | a claimed and screened batch can finish after Off (up to one cycle, 90 s guard) | same class as Phase 16 AR-3; bounded (SEC-17-05) |
| carried | Phase 16 AR-1 to AR-7 stand | unchanged |

The decisions D-01 to D-34 and owner confirmations A17-1 to A17-8 are provisional (the owner has not confirmed them). A change of any
of them can reopen the matching threat above.

---

## Security Audit Trail

| Date | Event | Result |
|------|-------|--------|
| 2026-11-12 | `gsd-security-auditor` at `fd40ac2`, ASVS L1 with L2 traces, wider mandate | 48/48 threats CLOSED, `threats_open: 0`; SEC-17-01 to SEC-17-05 (low or info), none blocking; no unregistered flags in the SUMMARY files |
| 2026-11-12 | evidence run | `fuku/db/deletion`, `fuku/utils/helpers`, 68 Phase 17 top-level tests in `fuku/modules` and `fuku/i18n` (`-race -count=1`), `TestDeletionTablesStayOutOfBackup`, `make check-translations`, `make check-docs`, build and vet all pass; PostgreSQL integrity, lint, the full suite and live Telegram not run by the auditor (CI and `17-SPIKE.md`) |

| 2026-11-12 | fix pass (`17-REVIEW-FIX.md`, `17-REVIEW-DISPOSITION.md`) | SEC-17-01 to SEC-17-05 fixed (b53cd8b, 1ceae78, b8864e0, ccd711d, 7bdffab, 4f297a9, 62ad877); whole `fuku/modules` package, `fuku/db/deletion`, lint stand-in, docs and translations gates pass; CI result recorded below once green |

| Threats found | Closed | Open |
|---------------|--------|------|
| 48 | 48 | 0 |
