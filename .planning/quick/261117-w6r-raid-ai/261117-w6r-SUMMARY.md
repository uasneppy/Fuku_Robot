# Quick task 261117-w6r: raid and AI review fixes (Wave 6), summary

Branch `wave6-raid`, base `86fafe0`. Every fix started with a test that failed on the base code.

## Fixed

| Item | Commit | What changed | Test |
|------|--------|--------------|------|
| R7 | `8a8ceb8` | The TypeSafe client has `CheckRedirect` returning `http.ErrUseLastResponse`: a 3xx is a non-2xx answer, no verdict, and the bearer key never reaches another host. | `TestAISpamJevDecideRefusesRedirect` |
| R3 | `f83d9bb` | `raidFloodItemKey` counts a rich message, a checklist and a game (the fields exist in the vendored gotgbot: `Message.RichMessage`, `Checklist`, `Game`). | `TestRaidFloodItemKeyCountsRichKinds` |
| R6 | `2b6fcf5` | `raidFloodIsSlashText` judges the caption (`GetText`, `GetEntities`) and a rich message's first paragraph the way gotgbot's command handler does, so a command in a caption is counted only by the late watcher. | `TestRaidFloodCaptionCommandIsSlash` |
| R5 | `8434203` | Both breaker maps record `lastFailure`; when full they prune breakers that are not paused and whose last failure is older than the cooldown (the image map also keeps pruning idle ones). | `TestAISpamBreakerPrunesStale`, `TestRaidAIImageBreakerPrunesStale` |
| R1 | `c0febad` | `/aispam` checks derive from a root context (`aispamContext`) that `DrainAISpamChecks` cancels first; a cut-off or dropped check is shed, never a breaker failure; the wait is 9 s (under the 10 s handler default, so AGENTS.md's "AI spam 10 s" stays true); `aispamDraining` is never reset; `raidAIFireAsync` counts the start and refuses it while `raidAIDraining` is set. | `TestShutdownDrainsAISpamChecks` |
| R2 | `c8ff233` | `raidCountMessageJoins` skips self-performed joins; they count from the `chat_member` update only. | `TestRaidJoinApprovedRequestServiceMessage` |
| R8 | `1b3d408` | `lockdown_help_msg` in all 8 locales: video thumbnails go to Anthropic too, and with `/aispam` on every message it deletes counts, from any member. `TestLockdownHelpFitsOneMessage` passes; docs regenerated. | `TestLockdownHelpFitsOneMessage` |

R2 evidence: the Bot API server (`tdlib/telegram-bot-api`, `Client.cpp`) builds the service message of
`messageChatJoinByRequest` exactly like `messageChatJoinByLink`: `new_chat_members` is `[sender_user_id]` and `from` is the
message sender, the joiner. The `chat_member` update carries `from` = the actor (the approver) and `via_join_request`
(`gotgbot.ChatMemberUpdated.ViaJoinRequest`). So the service message cannot tell an approved request from a raider's join
by link, and the fix takes the plan's fallback: self-joins count from `chat_member` only. The guard
(`lockdownOnJoinMessage`, `decideLockdownJoin`) is unchanged.

## Not reproduced

None.

## Owner decisions

- R4 (variation selectors in names, `namecanon`): not touched, per the plan.
- R8 trade-off: to keep the help inside one Telegram message, the sentence "These settings are not saved in backups."
  was removed from `lockdown_help_msg` in all 8 locales, and "AI never deletes or bans" was dropped (false once `/aispam`
  deletions feed the trigger). The owner may prefer another cut.
- R2 trade-off: in a group where the bot is not an administrator (no `chat_member` updates), self-joins are no longer
  counted at all; a lockdown cannot be started there anyway, but the once-a-day "cannot lock" warning of a join surge is
  no longer sent for such a group.

## Existing tests changed

- `TestRaidFloodItemKey`: the case "photo with a command as its caption" moved from the counted list to
  `TestRaidFloodCaptionCommandIsSlash` (it encoded the R6 bug).
- `TestRaidAIImageBreakerCap` ("a full map of failing chats warns once"): its failing breakers now carry
  `lastFailure: raidNow()`, so they are failing now and not stale.
- `aispam_test.go`, `raid_ai_burst_test.go`, `raid_ai_pool_test.go`, `deletion_bot_auto_test.go`: calls of
  `DrainAISpamChecks` used as a wait became `waitAISpamChecks` (new helper), because the drain now closes the queue for good.
- `TestRaidJoinDoubleDelivery`: the 15th account seen only as a service message no longer locks; its `chat_member` update does.
- No `TestLegacy*Requests` golden was touched.

## User-visible behaviour changes

- A TypeSafe redirect is a failed check (no verdict, no deletion) instead of being followed.
- Rich messages, checklists and games count toward the flood trigger; the member requirements watcher, which shares
  `raidFloodItemKey`, now screens them too.
- A command in a media caption or a rich message no longer raises the flood count when a handler takes it.
- A full breaker map now makes room for a newly failing chat instead of leaving it unpausable.
- At shutdown `/aispam` checks in flight are cut off at once (no deletion, no breaker count) and nothing is queued
  after the drain; no AI burst lockdown starts after the raid AI drain began.
- Join requests approved by an administrator never count toward the join surge and are not recorded as new members for
  the flood and AI paths.
- The `/lockdown` help text changed as described under R8.

## Verification

`go test -tags testtools -race -count=1` over `TestAISpam*`, `TestCheckAISpam*`, `TestRaidAI*`, `TestShutdown*`,
`TestBotAutoPunishmentNotices`, `TestRaid*`, `TestMemberReqJoin*`, `TestMemberReqWatcher*`, `TestLockdown*` (fuku/modules)
and `./fuku/i18n`: all pass. `CGO_ENABLED=0 go build ./...`, `make check-translations`, `make check-docs` pass.
golangci-lint v2.5.0 built with go1.26.0: 0 issues (the binary at `scratchpad/bin/golangci-lint` is actually a go1.24
build and refuses the module; a go1.26 build was made in `scratchpad/bin6`).
