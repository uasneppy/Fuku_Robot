---
phase: 05-lockdown-alerts-and-response
plan: 04
subsystem: lockdown
tags: [lockdown, alerts, staff-group, callbacks, authority, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: alert copy rows (05-01), the lockdown| callbacks and liftLockdownRow (05-02), the stale-button path (05-03)
provides:
  - service-identity refusal on every lockdown| button before any read or lookup
  - copy rows honoured only on the chat they were posted in
  - separate refusal for an administrator without can_restrict_members
  - lazy expiry and strict field parsing for the Lift confirm card
  - authority, card lifecycle and callback size tests, including a no-Redis run
affects: [05-06, 05-13]
tech-stack:
  added: []
  patterns:
    - "refusals happen in the dispatcher before the first database read or Telegram lookup, in the order identity, field shape, chat, lockdown, authority"
    - "confirm cards keep no state: asker u and expiry minute e travel in the callback data"
key-files:
  created:
    - alita/modules/lockdown_alert_auth_test.go
  modified:
    - alita/modules/lockdown_alert_callbacks.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "Cancel requires only a valid asker u (its data carries no e); Confirm requires both u and e."
  - "An expired Confirm answers once with an empty toast and puts the explanation on the card (lockdown_alert_card_expired)."
  - "The stale-lockdown path still runs before field and expiry checks, so a Confirm on an expired card of an already lifted lockdown says 'already lifted'."
requirements-completed: [LOCK-11, LOCK-06]
duration: 35m
completed: 2026-10-11
commits: 4
plan_head_before: 589d484fca1c028b44fc729820cada51f917cc7c
plan_head_after: 3540528e50a7022f2aaa5101454503c4c57a5fa7
actuals:
  tokens: 9000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 04: Harden the alert's Lift button Summary

**Only a live creator or administrator with can_restrict_members of the locked group can reach the confirm card or the lift; service identities, forged chats, Staff Group members, demoted admins and failed lookups are refused with nothing changed, and the confirm card is asker-only, expires after five minutes, lifts once and works without Redis.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows it, so `plan_head_after` is the last code commit.

## What was built

- **Dispatcher order (`lockdown_alert_callbacks.go`)**: decode, then service identity (`staffServiceUserIDs[query.From.Id]`, "post as yourself", alert), then `lockdownParseCopyID` (1 to the largest 63-bit value, else "no longer valid"), then the copy row, then the pressed-chat check (`slot.DestChatID` must equal the pressed message's chat, else "no longer valid" with no lockdown read and no lookup), then the lockdown and the stale path from 05-03, then the action.
- **Authority**: `lockdownButtonAuthority` still looks the presser up live in the lockdown row's chat and now returns `lockdown_alert_no_restrict` for an administrator without the restrict right, `lockdown_alert_not_admin` for any other non-admin, and `lockdown_alert_check_failed` for a failed lookup. It runs at Ask and again at Confirm; a refusal at Confirm edits the card and lifts nothing.
- **Card fields**: `lockdownParseAsker`, `lockdownParseCardFields` (u and e positive 63-bit integers) and `lockdownParseCopyID`. A Confirm or Cancel with a bad field is answered "no longer valid" before any lookup; another user's press is answered "not yours" and edits nothing; a Confirm whose expiry minute is before the current unix minute answers once, edits the card to `lockdown_alert_card_expired` and lifts nothing. No timer and no Redis key.
- **Locale keys** in all 7 files: `lockdown_alert_post_as_yourself`, `lockdown_alert_no_restrict`, `lockdown_alert_card_expired`.

## Tests (`alita/modules/lockdown_alert_auth_test.go`)

- `TestLockdownAlertLiftAuthority`: staff member who is not an admin, admin without restrict, failed lookup, the three service identities (no `getChatMember` call names them), forged chat (the env chat and a random chat, no lookup of the presser, nothing posted in the forged chat), creator allowed, admin demoted before Confirm. Every refusal case asserts one answer, no `setChatPermissions`, no new message in the Staff Group and the lockdown still active.
- `TestLockdownAlertConfirmCard`: expired card (then a fresh card lifts), double confirm (one `setChatPermissions`, second card text "already lifted"), two askers (one lift, one "done" card, one "already lifted" card), seven bad-field variants (no `getChatMember`, no write), and a full Lift, Confirm and next cycle with `cache.DisableRedisForTest()` that lifts the lockdown and closes the alert. Each press is asserted to give exactly one `answerCallbackQuery`.
- `TestLockdownCallbackData`: Ask, Confirm (16-digit user ID) and Cancel data encode within `callbackcodec.MaxCallbackDataLen` and decode to the same fields; an unknown action is answered once as expired; undecodable data gets the common invalid-request text.

## Task commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 (RED) | 54fdbf3 | test(05-04): show the alert's Lift button trusts service identities and forged chats |
| 1 (GREEN) | 44e9ecf | fix(05-04): refuse service identities, forged chats and admins without the restrict right on the Lift button |
| 2 (RED) | 5255b44 | test(05-04): show a lockdown confirm card never expires and accepts loose fields |
| 2 (GREEN) | 3540528 | fix(05-04): expire lockdown confirm cards and parse their fields strictly |

## Deviations from Plan

**1. [Task 1 - RED shape] Four of seven subtests already passed at RED.** "staff member, not an admin", "lookup fails", "creator allowed" and "demoted before Confirm" were already enforced by 05-02's authority path. Only "admin without restrict", "service identities" and "forged chat" failed at RED, as expected.

**2. [Task 2 - RED shape] `TestLockdownCallbackData` and the "double confirm", "two askers" and "no Redis" subtests already passed at RED.** The 05-02/05-03 code already lifted once, used conditional updates and kept state in the callback data. Only "expired" and "bad fields" failed at RED (they lifted the lockdown or answered with the wrong text).

**3. [Test data] The "r 20 digits" bad-field case uses `u=7`.** A 20-digit `r` next to a real 12-digit user ID and an 8-digit `e` exceeds 64 bytes and does not encode; the dispatcher rejects `r` before it reads `u`, so the case still proves the 63-bit bound.

**4. [Cancel] The plan says Confirm and Cancel both refuse "fields not ok".** Cancel's data carries no `e`, so Cancel validates only `r` (dispatcher) and `u`.

Otherwise the plan was executed as written.

## Known limitations

- AGENTS.md was not touched (not in this plan's file list, and plan 05-05 runs beside it); the new refusals and the card expiry are not yet described there.
- Tests run on SQLite with a fake Telegram client; no PostgreSQL server or real Staff Group was available in the sandbox.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertLiftAuthority$|^TestLockdownAlertConfirmCard$|^TestLockdownCallbackData$' ./alita/modules`: pass (all subtests).
- `go test -tags testtools -race -count=1 -run 'Lockdown|Unlockdown' ./alita/modules`: pass (40 s).
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n` and `make check-translations`: pass.
- `CGO_ENABLED=0 go build ./...` and `go vet -tags testtools ./alita/modules`: clean. `gofmt -l` on the two touched Go files prints nothing. The package-wide `gofmt -l alita/modules` reports `greetings_command_test.go`, which this plan does not touch (pre-existing). golangci-lint was not run: the installed one is built with Go 1.25 and cannot lint Go 1.26 code.
- `grep` for `utils/cache|db/cache` in `lockdown_alert_callbacks.go`: no matches.
- Not run: the whole `./alita/modules` package under `-race`, any PostgreSQL check, and a manual UAT press from a real Staff Group.

## Threat surface

No surface beyond the plan's threat model. T-05-10 (another user's card) is covered by "two askers" and the 05-02 "not yours" tests; T-05-11 (service identities) by "service identities"; T-05-12 (no restrict right, failed lookup, demotion) by "admin without restrict", "lookup fails" and "demoted before Confirm"; T-05-13 (forged or replayed data, overflow) by "forged chat", "bad fields", "expired" and `TestLockdownCallbackData`.

## Known Stubs

None.

## Self-Check: PASSED

`alita/modules/lockdown_alert_auth_test.go` exists, and commits 54fdbf3, 44e9ecf, 5255b44 and 3540528 are on the branch.
