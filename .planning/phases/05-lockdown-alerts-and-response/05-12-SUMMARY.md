---
phase: 05-lockdown-alerts-and-response
plan: 12
subsystem: lockdown
tags: [lockdown, alerts, invite-links, revoke, callbacks, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: the shown link stored on each alert copy (05-09), alert copies, close claims and the confirm-card pattern (05-03, 05-10, 05-11)
provides:
  - migration 20261011150000 (chat_lockdowns.link_revoked_at, link_revoked_by, link_revoked_by_name, link_revoked_link)
  - lockdown.RecordLinkRevoked, ListAlertSlotsToSyncFresh, ClaimAlertSync, RecordAlertSynced
  - the Revoke link button (a=rk), its confirm card (a=rc) and lockdownInviteAuthority
  - lockdownSyncAlertCopies, a generic worker step that refreshes copies marked edit_pending (plan 05-13 reuses it)
  - lockdownRevokeRefusalKey, plain-word mapping of Telegram's refusals
affects: [05-13]
tech-stack:
  added: []
  patterns:
    - "Telegram first, record second: the revoke is recorded only after revokeChatInviteLink succeeded, so a refusal leaves no trace"
    - "a sync claim reuses the close claim's claimed_at, so a copy is never refreshed and closed at once, and the stale-claim release covers it with no new code"
key-files:
  created:
    - migrations/20261011150000_add_lockdown_link_revoke.sql
    - alita/modules/lockdown_revoke_test.go
  modified:
    - alita/db/models/lockdown.go
    - alita/db/lockdown/alerts.go
    - alita/db/lockdown/alerts_test.go
    - alita/modules/lockdown_alert.go
    - alita/modules/lockdown_alert_callbacks.go
    - alita/modules/lockdown_worker.go
    - alita/modules/lockdown_fake_test.go
    - AGENTS.md
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "Revoke authority is the invite right: a live getChatMember of the lockdown row's chat, creator or administrator with can_invite_users, at Ask and again at Confirm; the restrict right alone is refused and a failed lookup refuses."
  - "The link revoked is always slot.Link of the pressed copy, loaded fresh by the dispatcher; callback data carries only the copy row ID, the asker and the expiry."
  - "RecordLinkRevoked is one transaction: the conditional update (link_revoked_at IS NULL) and, for the winner only, edit_pending on the lockdown's posted, open copies with live buttons."
  - "lockdownSyncAlertCopies edits to stored body plus footers with the keyboard lockdownAlertKeyboard gives the copy now; the close step appends the same footers before the lifted footer, so a revoked-then-lifted alert reads revoked, then lifted."
  - "A refresh that fails three times is given up (RecordAlertSynced, warn log) rather than looped; a message that is gone closes the copy."
patterns-established:
  - "Telegram refusals reach staff as plain keys; the raw text goes to the warn log only."
requirements-completed: [LOCK-13]
duration: 80min
completed: 2026-10-11
commits: 4
plan_head_before: 694e9745aead362376904eeda0d15fef0670a599
plan_head_after: 46134e5382e3a9fb3265ead0a4f07159e16781fe
actuals:
  tokens: 18000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 12: Revoke the raid's invite link from the alert Summary

**A link the bot created (reported in full) gets a "Revoke link" button on every live alert copy; a live creator or an administrator with `can_invite_users` of the locked group revokes it behind a confirm card only they can confirm, the revoke is recorded once after Telegram confirmed it, and every live copy is refreshed to say who revoked it and drops the Revoke button while Lift and Ban stay; every refusal, by authority or by Telegram, is answered in plain words with nothing recorded.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit. `actuals.tokens` is chars/4 over the realized diff.

## What was built

- **Migration `20261011150000_add_lockdown_link_revoke.sql`** and the four fields on `models.ChatLockdown` (`LinkRevokedAt`, `LinkRevokedBy` as pointers, `LinkRevokedByName`, `LinkRevokedLink`). Idempotent `ADD COLUMN IF NOT EXISTS`, no top-level transaction statements, no `CONCURRENTLY`.
- **Repository (`alerts.go`)**:
  - `RecordLinkRevoked(lockdownID, by, byName, link)`: one transaction; the conditional update `WHERE id = ? AND link_revoked_at IS NULL` and, for the winner only, `edit_pending = true` on the lockdown's posted copies with `closed_at IS NULL` and `buttons_live`. A second call, or an unknown lockdown, returns `false, nil` and writes nothing.
  - `ListAlertSlotsToSyncFresh`, `ClaimAlertSync`, `RecordAlertSynced`: posted, open, unclaimed, `edit_pending` copies of active lockdowns, oldest first; the claim is a conditional update that reuses `claimed_at`, so `ReleaseAlertClose` and `ReleaseStaleAlertClaims` already cover it.
- **Alert (`lockdown_alert.go`)**: `lockdownActRevokeAsk` ("rk") and `lockdownActRevokeConfirm` ("rc"); `lockdownAlertKeyboard` adds a third row with the Revoke button while `lockdownLinkRevocable(slot.Link)` and no revoke is recorded; `lockdownPostAlertSlot` now sets `slot.Link` before building the keyboard (the link is stored only after the send, so without this the first post would have had no Revoke button). `lockdownAlertFooters` / `lockdownCopyText` build stored body plus footers; `lockdownCloseClaimed` uses them so the lifted footer comes after the revoked footer. `lockdownSyncAlertCopies` / `lockdownSyncClaimed` refresh the marked copies through `lockdownPaced`.
- **Callbacks (`lockdown_alert_callbacks.go`)**: the dispatcher routes `rk` and `rc`. `lockdownInviteAuthority` (creator, or administrator with `CanInviteUsers`, live; failed lookup refuses). `lockdownRevokeAsk` (expired for a non-revocable stored link, "already revoked by X" toast for a revoked lockdown, authority, then a card replying to the pressed copy with the escaped link in code tags and the group). `lockdownRevokeConfirm` (asker, lazy expiry, active, not revoked, revocable, authority again, one `lockdownPaced` `RevokeChatInviteLinkWithContext(call, ld.ChatID, slot.Link, nil)`, then `RecordLinkRevoked` with the presser's capped full name, card edited to `lockdown_revoke_done`, `wakeLockdownWorker()`). A lost race on the record shows the winner's name from a fresh read. `lockdownRevokeRefusalKey` maps rate limits to busy, code 403 or "not enough rights" / `CHAT_ADMIN_REQUIRED` to no-rights, `INVITE_HASH` / "not found" / "expired" to not-found, anything else to the generic line; the raw error is logged at warn only.
- **Worker (`lockdown_worker.go`)**: `runLockdownCycle` runs `synced := lockdownSyncAlertCopies(ctx, b)` before the close step and ORs it into its progress result.
- **Locale keys in all 7 files (10)**: `lockdown_alert_btn_revoke`, `lockdown_revoke_ask`, `lockdown_revoke_not_admin`, `lockdown_revoke_done`, `lockdown_alert_link_revoked`, `lockdown_revoke_already`, `lockdown_revoke_failed`, `lockdown_revoke_no_rights`, `lockdown_revoke_not_found`, `lockdown_revoke_busy`.
- **AGENTS.md**: the `lockdown|v1` bullet gains `rk` / `rc`, the invite-right authority and "the link is the stored one, never callback data"; Data gains the four `link_revoked_*` columns and the `edit_pending` sync step.
- **Test fake**: `revokeChatInviteLink` answer (scripted errors first, then records and answers a revoked `ChatInviteLink`), `setMemberRaw` (consulted for non-bot users before the staff fake, honours scripted errors) and `lockdownAdminJSON`.

## Tests

- `TestLockdownRevokeLink` ("full link": both copies have Lift, Ban and a third Revoke row; a can_invite_users-only administrator gets a card with the link, Confirm for them and Cancel; Confirm sends exactly one `revokeChatInviteLink` with the chat and link and records by, name and link; the next cycle edits each copy once with body plus the revoked footer and the escaped name, Lift and Ban kept, no Revoke; a further cycle edits nothing; an old Revoke press answers "already" with no second call; `/unlockdown` closes both copies with the revoked footer before the lifted footer and no keyboard. "cut short link": no Revoke button on either copy).
- `TestLockdownRevokeLinkRefusals`: restrict right only, plain member, service identity, failed lookup (no card, no call, nothing recorded, no copy edit), authority lost between Ask and Confirm, "no rights" (400 `CHAT_ADMIN_REQUIRED`: one call, the card shows the plain key and not the raw text), "not the bot's" (400 `INVITE_HASH_EXPIRED`), "server error" (500), "rate limited" (four 429s), and "retry after a refusal" (a later Ask and Confirm records once, two calls in all).
- `TestRecordLinkRevoked` (once; marks only posted, open, live copies; a lost call marks nothing; unknown lockdown) and `TestAlertSync` (listing, claim won once, release lists it again, `RecordAlertSynced` clears the mark, claim and attempts; lifted lockdown and closed copy not listed).
- Mutation checks: making `RecordLinkRevoked` set `edit_pending` false makes "full link" fail ("staff copy was edited 0 times, want once"); replacing `CanInviteUsers` with `CanRestrictMembers` in `lockdownInviteAuthority` makes `TestLockdownRevokeLinkRefusals` fail.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The first post of an alert had no Revoke button**
- **Found during:** Task 1 (reading `lockdownPostAlertSlot`)
- **Issue:** the keyboard is built before `RecordAlertPosted` stores the link, so `slot.Link` was still empty and the plan's "Revoke when `lockdownLinkRevocable(slot.Link)`" would never have shown it on the first post, only after a refresh.
- **Fix:** `lockdownPostAlertSlot` sets `slot.Link = link` (the link the rendered text shows, the same value it stores) before `lockdownAlertKeyboard`.
- **Files modified:** `alita/modules/lockdown_alert.go`
- **Commit:** c2817d0

### Process notes

- The two RED commits: the first does not compile (the symbols it tests do not exist yet), which is its failing state, as in plans 05-05 to 05-11. In the second, `no rights`, `not the bot's` and `rate limited` failed (the card read the generic line); the authority cases, `confirm checks authority again`, `server error` and `retry after a refusal` already passed against Task 1's code and are kept as pinned tests.
- The plan's Task 2 step 3 lists the three Task 2 locale keys; they were added in the Task 2 commit, the seven Task 1 keys in the Task 1 commit.
- The AGENTS.md "Subsystem traps" note on the sync step is written in the Data section next to the other alert-copy bullets, where the neighbouring alert rules already live.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownRevokeLink$|^TestLockdownAlertInviteLink$' -v ./alita/modules`: pass.
- `go test -tags testtools -race -count=1 -run '^TestLockdownRevokeLinkRefusals$|^TestLockdownRevokeLink$' -v ./alita/modules`: pass (10 refusal subtests).
- `go test -tags testtools -race -count=1 -run '^TestRecordLinkRevoked$|^TestAlertSync$|^TestAlertClose$' -v ./alita/db/lockdown`: pass.
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules ./alita/db/lockdown`: pass (59 s).
- Full `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/lockdown ./alita/db/models ./alita/i18n`: pass (modules 213 s); no intermittent failure seen in this run (the 05-08 setup flake recorded in `deferred-items.md` did not appear).
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown ./alita/db/models`: clean. `gofmt -l` on every touched Go file prints nothing. `git diff --exit-code go.mod go.sum` exits 0.
- Acceptance greps pass: latest migration file, 0 transaction statements or `CONCURRENTLY`, `RevokeChatInviteLinkWithContext(` with `slot.Link`, `synced := lockdownSyncAlertCopies(ctx, b)`, `CanInviteUsers`, `can_invite_users` in AGENTS.md, `func lockdownRevokeRefusalKey(`, `lockdown_revoke_busy` once in each of the 7 locale files.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: applying the migration on PostgreSQL 16 (the plan's human check; SQLite only here) and a real Telegram revoke.

## Threat surface

No surface beyond the plan's threat model. T-05-33 is covered by `TestLockdownRevokeLinkRefusals` (restrict-only, member, service identity, failed lookup, authority lost before Confirm) and the `CanRestrictMembers` mutation; T-05-34 by the `slot.Link` acceptance grep and the fact that callback data holds no link; T-05-35 by the "no rights", "not the bot's", "server error" and "rate limited" cases; T-05-36 by `TestRecordLinkRevoked` / `TestAlertSync`. No package installs.

## Known Stubs

None.

## Known limits

- Telegram's exact error text for revoking a link the bot did not create is unconfirmed (assumption A4); an unmapped answer shows the generic "refused" line. Confirm in UAT, and add the real wording to `lockdownRevokeRefusalKey` if it differs.
- If Telegram confirms the revoke but the record write fails, the card still says "Link revoked", the copies keep their Revoke button and a later press revokes an already revoked link (which Telegram will refuse); the error is logged. The window is a failed database write between two statements.
- A sync that started before a second `edit_pending` mark (plan 05-13's reminders reuse the step) could clear that mark when it records; the next lifecycle change re-marks. Not reachable with one revoke per lockdown.

## Self-Check: PASSED

`migrations/20261011150000_add_lockdown_link_revoke.sql` and `alita/modules/lockdown_revoke_test.go` exist; commits 9bec408, c2817d0, 0eedc54 and 46134e5 are on the branch.
