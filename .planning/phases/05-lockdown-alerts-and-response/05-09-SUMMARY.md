---
phase: 05-lockdown-alerts-and-response
plan: 09
subsystem: lockdown
tags: [lockdown, alerts, invite-links, i18n]
status: complete
requires:
  - phase: 05-lockdown-alerts-and-response
    provides: burst rows and the alert poster (05-07), RecordAlertPosted with its link argument (05-01..05-06)
provides:
  - lockdown.TopInviteLinksFresh and lockdown.InviteLinkCount
  - the invite-link line on the lockdown alert (top link, count, other links, cut-short flag)
  - the shown link stored on each alert copy for plan 05-12's Revoke button
affects: [05-12]
tech-stack:
  added: []
  patterns:
    - "user text (an invite link) built outside the translator and spliced in after it through a token (lockdownLinkToken)"
key-files:
  created:
    - alita/db/lockdown/links.go
    - alita/db/lockdown/links_test.go
    - alita/modules/lockdown_alert_link_test.go
  modified:
    - alita/modules/lockdown_alert.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
key-decisions:
  - "Link counts come from the pre-lockdown burst rows plus the joiner rows of users with no pre row, grouped by invite_link, most used first and ties by link; join requests count like joins. The query is in its own file so it runs beside plan 05-08 without touching burst.go."
  - "A link is revocable when it is non-empty and holds neither '...' nor the single-character ellipsis (D-18)."
  - "lockdownRenderAlert now returns (text, link): the link shown is the one stored on the copy, so the Revoke button of plan 05-12 revokes exactly what the copy shows."
requirements-completed: [LOCK-13]
duration: 25min
completed: 2026-10-11
commits: 4
plan_head_before: b0770888b90f47bedbdabc79ac1c8021291c422e
plan_head_after: d9b2e27ddf269767dfc042a766719fcfca277bb1
actuals:
  tokens: 9000
  tasks: 2
  commits: 4
---

# Phase 5 Plan 09: Invite link on the lockdown alert Summary

**The lockdown alert now shows how the raiders got in: the invite link most of the burst used with how many joined through it and "+N other links", a link another admin made (reported cut short with "...") flagged with where to revoke it in Telegram, and the shown link stored on the alert copy.**

Note: `commits: 4` counts the two RED test commits and the two implementation commits made before this SUMMARY; the SUMMARY commit follows, so `plan_head_after` is the last code commit.

## What was built

- **`alita/db/lockdown/links.go`**: `InviteLinkCount{Link, Count}` and `TopInviteLinksFresh(lockdownID)`. Two grouped reads (pre-lockdown burst rows; joiner rows of users with no pre row), merged in Go and sorted by count descending then link ascending. Empty links are ignored; a user with a pre row and a later joiner row counts once, by the pre row. Always read fresh, never cached, so no cache key applies.
- **`alita/modules/lockdown_alert.go`**: `lockdownLinkRevocable` (non-empty, no "..." and no "…"), `lockdownRenderLinkLine(tr, links) (text, top)` (link escaped in code tags and spliced in after translation through `lockdownLinkToken`; the foreign-link pointer after the link line; the "+N other links" line when more than one link was used) and `lockdownRenderAlert` now returns `(text, link)`. `lockdownPostAlertSlot` passes the shown link to `RecordAlertPosted`. A failed link read leaves the line out (warn log) and the alert still posts.
- **Locale keys in all 7 files**: `lockdown_alert_link_line`, `lockdown_alert_link_others`, `lockdown_alert_link_foreign`.

## Tests

- `TestTopInviteLinks` (repository): merge of pre and joiner rows with order (count, then link), empty links ignored, a user with both rows counted once by the pre row, no links, another lockdown's links not counted.
- `TestLockdownAlertInviteLink` (module): top link (3 vs 1, count 3, "1 other", stored link), joiner links count (2 raiders after /lockdown, no others line), foreign link (cut-short link flagged, stored), revocable (full, "...", "…", empty), escaped (`<`, `>`, `&`, `"` and a splice-token look-alike escaped inside code tags, no raw text), no link (no line, empty stored link).
- Mutation check: removing `html.EscapeString` on the link makes the "escaped" case fail.

## Deviations from Plan

None - plan executed as written. `lockdownLinkRevocable` was added in Task 2 together with the foreign flag (the plan lists it under both tasks; its behaviour is only observable with the flag).

### Process notes

- The two RED commits do not compile (the symbols they test do not exist yet), which is their failing state, as in plans 05-05 to 05-07.

## Verification

- `go test -tags testtools -race -count=1 -run '^TestLockdownAlertInviteLink$' -v ./alita/modules`: pass (6 subtests).
- `go test -tags testtools -race -count=1 -run '^TestTopInviteLinks$' -v ./alita/db/lockdown`: pass (4 subtests).
- `go test -tags testtools -race -count=1 -run 'Lockdown' ./alita/modules ./alita/db/lockdown`: pass (49 s).
- `go test -tags testtools -count=1 -run '^TestLockdownLocaleKeys$' ./alita/i18n`, `make check-translations`, `make check-docs`, `go vet -tags testtools ./alita/modules ./alita/db/lockdown`: clean. `gofmt -l` on the four touched Go files prints nothing. `git diff --exit-code go.mod go.sum` exits 0.
- `golangci-lint` was not run: the installed build targets Go 1.25 and cannot lint Go 1.26 code (go vet and gofmt used instead).
- Not run: any PostgreSQL check (SQLite only) and a real Telegram post. Assumption A7 (a bot-created link arrives in full in chat_member updates) stays for UAT; if Telegram cuts it too, the link is simply flagged as not revocable.

## Threat surface

No surface beyond the plan's threat model. T-05-25 is covered by the "escaped" case (the link is escaped inside code tags and spliced in after the translator). T-05-26 is accepted as planned. No package installs.

## Known Stubs

None.

## Self-Check: PASSED

`alita/db/lockdown/links.go`, `alita/db/lockdown/links_test.go` and `alita/modules/lockdown_alert_link_test.go` exist; commits 1645a19, 92b41f0, c2b50cb and d9b2e27 are on the branch.
