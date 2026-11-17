---
phase: quick-261116-wh0
plan: 261116-wh0
subsystem: moderation, extraction, users, staff actions, federations
tags: [moderation, targets, forum-topics, html-escaping, staff, federations]
status: complete
requires: []
provides:
  - extraction.ReplyTarget (forum-safe reply reader)
  - explicit target beats the reply in ExtractUserAndText
  - case-insensitive, ambiguity-refusing GetUserIdByUserName
  - moderationReasonSuffix (escaped reason line)
  - staff end-date guard (staffUntilTooClose, errStaffTimeRanOut, fail_time_ran_out)
  - applyActiveUnfban and fbanChanged
affects: [every command that resolves a target through ExtractUserAndText, purges, /id, staff fan-out, federations]
tech-stack:
  added: []
  patterns: [one reply reader for targets and delete ranges, user text never a format string]
key-files:
  created:
    - fuku/modules/target_resolution_test.go
    - fuku/modules/moderation_reason_test.go
    - fuku/modules/federations_unfban_test.go
  modified:
    - fuku/utils/extraction/extraction.go
    - fuku/utils/extraction/extraction_test.go
    - fuku/db/user/repository.go
    - fuku/db/user/find_by_username_test.go
    - fuku/modules/moderation.go
    - fuku/modules/bans.go
    - fuku/modules/mute.go
    - fuku/modules/purges.go
    - fuku/modules/misc.go
    - fuku/modules/bans_command_test.go
    - fuku/modules/staff_action_run.go
    - fuku/modules/staff_action_decide.go
    - fuku/modules/staff_action_summary.go
    - fuku/modules/staff_action_timed_test.go
    - fuku/modules/staff_history_test.go
    - fuku/db/federations/repository.go
    - fuku/db/federations/repository_test.go
    - fuku/modules/federations.go
    - locales/{en,es,fr,hi,id,pt,ru,uk}.yml
    - AGENTS.md
decisions:
  - "An explicit target (text_mention, @username, numeric ID) always beats the reply; /id shows reply details only when the resolved user is the reply's sender"
  - "Targets and delete ranges are read only through extraction.ReplyTarget, which ignores a forum topic's creation message and a topic message's own thread root"
  - "GetUserIdByUserName goes through FindUsersByUsername and refuses a name two accounts hold (owner decision: refuse, do not guess)"
  - "A staff ban or restrict whose fixed end date is under 35 s away at write time is not sent; the group fails with fail_time_ran_out; one until_date for all groups kept"
  - "/unfban unbans with only_if_banned in the fed's chats that know the user, unless a subscribed federation still bans them"
metrics:
  duration: "about 40 minutes"
  completed: 2026-11-17
actuals:
  tokens: 17500
  tasks: 3
  commits: 8
plan_head_before: 68414edbc7043ae081dd3363894a320183fe0c1d
plan_head_after: 5cd047ace00a450c51ccc981ba2d38ab1ce73188
---

# Quick 261116-wh0: Target resolution and moderation safety Summary

Moderation commands now act on exactly the person named (explicit target beats the reply, a forum topic's implicit reply is ignored, `@Name` is case-insensitive and ambiguous names are refused), their replies survive any reason or name, `/unban` never kicks, staff timed actions can no longer turn permanent through fan-out timing, federation names are rune-safe and `/unfban` really unbans.

## Audit items (section Q2)

| # | Item | Status | Commit | Tests that failed before and pass after |
|---|------|--------|--------|------------------------------------------|
| 4 | `/unban` kicked a member who was not banned | fixed | 7c4cd20 | `TestUnbanCommandUnbansUser` (extended: only_if_banned must be true) |
| 5 | Reply beat an explicit target | fixed | bd4c763 | `TestExtractUserAndTextExplicitTargetBeatsReply` (extraction), `TestBanExplicitTargetBeatsReply`, `TestGetIdExplicitTargetBeatsReply` (modules) |
| 6 | Forum topic implicit reply trusted | fixed | 64915b4 | `TestReplyTarget`, `TestExtractUserAndTextForumImplicitReply` (extraction), `TestForumTopicImplicitReplyIsNoTarget` (modules: /ban, /mute, /dkick, /purge, /del, /purgefrom, /id; real replies in the topic still act) |
| 7 | `@username` case-sensitive, stale owner | fixed | b2f927c | `TestGetUserIdByUserNameCaseInsensitive`, `TestGetUserIdByUserNameRefusesAmbiguous` |
| 8 | Reasons unescaped and used as format strings; names in `/id` unescaped | fixed | bf6c516 | `TestModerationRepliesEscapeReason` (kick, ban, tban, mute, tmute), `TestGetIdEscapesNames` |
| 9 | Staff tban/tmute could become permanent | fixed | 383ba31 | `TestStaffActionTimeRanOutSkipsWrite` (tmute 1m, tban 1m skipped; tmute 30m applied), `TestStaffActionTimeRanOutAfter429`, `TestStaffReasonText` (new reason in the worded list) |
| 10 | Federation names cut by bytes | fixed | 107ac1b | `TestFederationNameTruncatesByRunes` |
| 11 | `/unfban` only deleted the row; repeated `/fban` re-announced | fixed (both parts) | 5cd047a | `TestUnfbanUnbansTheUserInEveryFedChat`, `TestApplyActiveUnfbanKeepsBansFromSubscribedFeds`, `TestFbanChangedOnlyWhenNewOrReasonChanged` |

Red was observed for every new test before its fix (for items 5 and 6 at module level by temporarily removing the production change; for items 9 and 11 by landing the seam or a stub first).

## Pinned expectations changed on purpose

- `TestGetUserIdByUserNameUnchanged` (`fuku/db/user/find_by_username_test.go`) pinned the old exact-case behaviour; replaced by `TestGetUserIdByUserNameCaseInsensitive` and `TestGetUserIdByUserNameRefusesAmbiguous` (stated in commit b2f927c).
- `/unban` request gains `only_if_banned=true` (one parameter, stated in commit 7c4cd20). No `TestLegacy*Requests` golden pins `/unban`; no `TestLegacy*` file changed, and the `TestLegacy` goldens pass.

## Deviations from Plan

1. **Sites beyond the audit's list converted to `extraction.ReplyTarget`** (planned as siblings): `/purgefrom`, `/purgeto`, `/dkick` delete, `/dban` validate and delete, `/dmute` validate and delete. `/echo`, `/tr`, `/dwarn`, pins and the sender-chat branches of `/ban` and `/unban` left untouched as the plan said.
2. **Item 9 commit includes the reason constant, the summary text and locale keys** together with the guard (one commit per defect). `TestStaffReasonText` was extended in the same commit; it would have failed before the text existed but was not run red separately.
3. **Test file name**: the federation tests went into a new `fuku/modules/federations_unfban_test.go` instead of `federations_command_test.go`, to keep the fixture helpers (`fedChatWithUsers`, `unbansOf`) local.
4. **Process slip (no effect on the result)**: while proving item 6's module test red, a `git checkout` of `extraction_test.go` dropped the not-yet-committed `TestReplyTarget` and `TestExtractUserAndTextForumImplicitReply`; both were re-added verbatim before the item 6 commit and pass.

## Notes and residuals

- After item 5, `/dban @alice` sent as a reply to Bob bans Alice and still deletes the replied (Bob's) message, because `/dban` deletes the replied message by design. Before the fix it banned Bob. Not changed here; worth a look if `/d*` with an explicit target should refuse.
- Item 7 accepted consequence: while an old holder of a name still has a stale `username` row, the name reads as ambiguous (not found) until that account is seen again.
- Item 11: `applyActiveUnfban` reads the fed tree through `FindBanInFedTree`, whose `GetFedBan` hides a read error as "no ban", so on a database fault it would proceed to unban (only_if_banned). Accepted residual; an error-returning reader would be needed to fail closed. Also accepted by the owner: a ban a chat admin placed separately in a federation chat is lifted too.
- Item 9: the guard applies only to the action run (`executeStaffCall`); the undo path is untouched, as planned.
- Locale wording of the ban reason label (and missing newline in es/fr/id `bans_ban_ban_reason`) left to the strings batch, as planned.

## Verification

- `CGO_ENABLED=0 go build ./...` OK; `go vet -tags testtools ./...` clean; gofmt clean on changed files; `make check-translations` OK; `make check-docs` no drift.
- Targeted runs green: extraction package, `fuku/db/user`, `fuku/db/federations`, and in `fuku/modules` the ban, kick, mute, purge, `/id`, `/info`, warn, approval, staff (`TestStaffAction*`, `TestStaffHistory*`, `TestStaffReason*`, `TestStaffUndo*`), federation and `TestLegacy*` families; `fuku/i18n` whole package.

## Threat Flags

None: no new endpoint, auth path or schema change.

## Self-Check: PASSED

- Files exist: `fuku/modules/target_resolution_test.go`, `fuku/modules/moderation_reason_test.go`, `fuku/modules/federations_unfban_test.go`.
- Commits on HEAD: bd4c763, 64915b4, b2f927c, 7c4cd20, bf6c516, 383ba31, 107ac1b, 5cd047a (8 = `git rev-list --count 68414ed..HEAD`).
