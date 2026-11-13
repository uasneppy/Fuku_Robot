# Phase 14: Profile Photo and Channel Rules - Context

**Gathered:** 2026-11-08
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 14 adds the two member requirements that need a Telegram lookup, on top of Phase 13's rules engine, punisher and page:

- **Profile photo** (MREQ-11): a member with no photo the bot can see is punished. A photo hidden from the bot counts as missing, and the page says so.
- **Channel subscription** (MREQ-12): offered only in a group linked to a channel; a member not subscribed to that channel is punished.
- **Unanswerable checks** (MREQ-10): a Telegram error, a timeout, a shed lookup, a crash or shutdown, or the bot not being an admin of the linked channel never punishes anyone. The page shows when a rule cannot work in that group (no linked channel, the bot not admin there, a basic group). The "AI not configured" line arrives with the AI rule in Phase 15.
- **PLAT-06** for this phase's new background work: the lookups share one Telegram rate budget across replicas, never hold up the rest of the bot, and end within the existing shutdown budget.

The five name rules, the hold and its one-punishment guarantee, the marker mute, "I fixed it", the watcher at group -2, the cap and pause, and the shutdown drain are Phase 13's and are extended, not rebuilt. The AI spam-name rule is Phase 15; deleting requirement notices on a timer is Phase 16.

</domain>

<decisions>
## Implementation Decisions

### Profile-photo rule (accepted, area 1)
- **D-01:** A member with no photo the bot can see is in breach, and a hidden photo counts as missing. `getUserProfilePhotos(user, limit 1)` with a total of zero is the breach; any error, timeout or shed lookup is unknown and never a breach (MREQ-10, research Pitfall 20). The rule's page text says that a photo hidden by privacy settings counts as missing and what the member can do (show the photo to everyone, or ask an admin to approve them).
- **D-02:** The photo is looked up only in the enforcement job (and in the join entry's synchronous call), never in the watcher's pure path: after the exemptions and the live `getChatMember` and before the cap and the claim, only while the photo rule's action is not Off, so a group with the rule Off makes no call.
- **D-03:** Only a found photo is cached: `fuku:memberreq:photo:<user>` for 30 minutes (a photo is the user's, not the group's, so the key has no chat). A missing photo and an unknown answer are never cached, so a member who adds a photo is noticed at their next message and nobody is held on an old answer.
- **D-04:** Off by default like every rule. The rule screen's hint recommends Warn or Mute and not Kick or Ban, because a member cannot always fix a hidden photo from the group; all five punishments stay selectable behind the same two-tap card for Kick, Ban and Mute (Phase 13 D-05). Approved users, admins, bots, service accounts and members with a pending captcha are exempt exactly as in Phase 13 (D-11 there).

### Channel-subscription rule (accepted, area 2)
- **D-05:** The rule is offered only in a group linked to a channel. Its cell is always drawn on the page; in a group with no linked channel its screen says so and offers Off only, so a stored non-Off value after the channel is unlinked is inert (the check cannot run, nothing is punished) and can be turned Off.
- **D-06:** The linked channel is `getChat(group).LinkedChatId`, resolved once per chat and cached about 10 minutes in Redis (`fuku:memberreq:lc:<chat>`, positive and "none" alike) for the job; opening the rule screen reads it fresh and refreshes the cache. It is never taken from callback data or from the update.
- **D-07:** Subscribed means `getChatMember(channel, user)` answers member, administrator, creator, or restricted with `is_member`. Left and kicked are not subscribed and are the breach. Every error, timeout and shed lookup, and the answer a bot that is not the channel's admin gets, is unknown and never punishes. `chat_status.IsBotMissingError` reads "chat not found" as the bot being missing, which is wrong for a channel lookup, so the channel path classifies by answer and never by that helper (research flag; settled by the live spike).
- **D-08:** Opening the channel rule's screen makes at most one bounded live `getChatMember(channel, bot)` and shows a status line when the rule cannot work (the bot is not an administrator of the linked channel). The punishment can still be set; the check simply fails open. A status that cannot be determined shows the page's unknown mark, never an error and never a refusal.

### Lookup budget, pacing and shutdown (accepted, area 3)
- **D-09:** Photo and channel lookups (including the linked-channel `getChat`) share one new fleet-wide pacer with its own Redis keys (`fuku:memberreq:lkp:pace:next` and `:block`), built like `memberReqPacer`/`staffPaced` (`ratelimit.NewTelegramPacer`, a slot reservation plus a shared `retry_after` block). A call whose slot is far away is shed as unknown with no Telegram request. Punishments and notices keep Phase 13's own pacer, so a burst of lookups can never delay a mute notice. No per-replica limiter holds a shared budget.
- **D-10:** Lookups run inside the existing enforcement job, never on a new goroutine pool, with a singleflight per lookup key so 100 messages from one user make one lookup. The watcher stays cheap: for a member whose photo and subscription verdicts are both cached positive (and whose names are clean) the message is clean as before; a miss hands one job to the existing runner (still capped at `memberReqMaxInFlight` 32 and shed on overflow). A held member's messages take the held path before any lookup.
- **D-11:** No new shutdown drain and no change to the drain budget. Every lookup derives from `memberReqContext()`, which `StopMemberReqJobs` cancels (2 s wait plus 1 s grace). The claim of the hold happens only after every lookup answered, so a lookup cut off by a crash or a shutdown leaves no hold, no notice and punishes nobody; the member's next message starts again.
- **D-12:** Caches are positive only: photo `fuku:memberreq:photo:<user>` and subscription `fuku:memberreq:sub:<channel>:<user>`, 30 minutes each, both in the `fuku:memberreq:*` family outside the `fuku:cache:` prefix. "I fixed it" always does a fresh lookup that bypasses both caches; a lookup it finds positive may refresh them. Without Redis every lookup is a miss and the pacer falls back to one replica's interval (correct, slower, documented).

### Page, notices and "I fixed it" (accepted, area 4)
- **D-13:** The two rules are appended to the rule table (the photo rule is bit 32, the channel rule bit 64, short codes `ph` and `ch`, `memberreq.RulePhoto` and `RuleChannel`) with two new `SMALLINT` columns `photo_action` and `channel_action` (named CHECKs 0..4, default 0) in one new migration with a timestamp above `20261106120000`, one transaction, both appliers agreeing. The model, the repository's column-targeted writes (`SetRuleAction`), the harness AutoMigrate lists and the Phase 13 page, rule screen, cards and controls extend from the table; `settings_pages.go` is not edited.
- **D-14:** The rule grid grows to seven cells (the odd last cell alone on its row); the row budget test and the worst-case callback test are updated for it. The rule screen, the five punishments, the two-tap card and the rights (`settingsRightAdmin` to open, `settingsRightRestrict` to choose) are Phase 13's, unchanged.
- **D-15:** The page shows a rule that cannot work here as one "cannot work here" line, on the page body and on that rule's screen: the channel rule with no linked channel, the channel rule with the bot not an administrator of the linked channel, and, for every rule, a basic group (Phase 13 left that line for this phase: only supergroups are screened). The page open stays fast: the extra bounded Telegram calls (the cached linked channel and, only while the channel rule is not Off, one live bot-admin read) are made at most once per open, and a failed one shows the unknown mark. The "AI not configured" line is Phase 15's.
- **D-16:** The notice names every broken rule as in Phase 13 (D-04 there). The photo rule's phrase names the real remedy (show the profile photo to everyone, or ask an admin to approve you); the channel rule's phrase says to subscribe to the linked channel. A mute still carries the one "I fixed it" button. New strings go to all 8 locale files; the exact keys, wording and lengths are fixed in the UI contract.
- **D-17:** "I fixed it" re-checks every enabled rule live: the name rules on the live names as before, the photo and channel rules with fresh lookups through the lookup pacer under the press's 10 s budget. A member who still lacks the photo or the subscription stays held and is told which rules remain (`mreq_fixed_still_broken`). A lookup that cannot be answered at that moment is `mreq_fixed_check_failed` and changes nothing, exactly like an unreadable setting.
- **D-18:** Privacy and logs: the photo lookup sends Telegram only the user ID, the channel lookup the channel and user IDs. Log lines carry chat, channel, user IDs and outcomes only, never a name or a notice text. No screen names the Staff Group or shows a count of held or punished members (Phase 13 D-16 stands), and a group menu never reveals the linked channel's title or ID (the status lines are words, not identifiers).

### Verification, spikes and tests
- **D-19:** Two live spikes are the owner's, deferred like the earlier phases' (the owner's standing choice): does `getUserProfilePhotos` return zero or an error for a user who hides their photo, and what exact error does `getChatMember(channel, user)` give when the bot is not the channel's admin. They go into `14-SPIKE.md` as non-blocking live checks, with the Phase 13 live checks' warning that nothing punishes on an unknown. The code treats both shapes safely either way (zero photos is a breach and an error is unknown for the photo; any error is unknown for the channel), so the spikes change wording, not safety.
- **D-20:** The tests cover: both rules punishing a violator and sparing a member who has a photo or a subscription; every unanswerable shape (Telegram error, timeout, shed by the pacer, the context cancelled by `StopMemberReqJobs`) punishing nobody and leaving no hold; the singleflight (100 messages from one user, one lookup); positive-only caching (a missing photo is looked up again at the next message); the lookup pacer shared across two replicas over one miniredis, with Phase 13's pacer untouched by it; the linked-channel cache and the page's three statuses (no linked channel, bot not admin, basic group); "I fixed it" with fresh lookups and the remaining-rules answer; the migration's CHECKs on PostgreSQL (`TestChatRequirementsSettings` extended, in the integrity list) and its zero defaults for an existing row; the page grid, row budget, callback bytes and 8-language fit tests; and `TestSettingsStaffPrivacy` over the new screens.

### Decisions settled after research (orchestrator, binding on the plans)
- **D-21 (research C1):** D-10's "a held member takes the held path before any lookup" is amended in one case: a member whose hold carries a lookup rule bit (photo or channel), whose names are clean now, and who is not exempt, gets one held-release re-check in the enforcement job. It runs only the hold's lookup bits, never punishes, never lifts a mute, and ends in `ClearHoldOnClean` when every bit is now clean (the hold row is deleted; a mute hold is only ever freed by "I fixed it"). The cost is one paced lookup per message of a held member who is still missing the photo or the subscription, bounded by the lookup pacer and the runner. The watcher itself still makes no Telegram call; it only decides whether to start the job.
- **D-22 (research Open Question 2, makes spike S2 safe by construction):** a `left` or `kicked` answer for the member counts as a channel breach only when the bot is established as a definite administrator of the linked channel (the `b:<channel>` lookup, cached positive for a short time in the lookup cache family). Anything else (the bot not an administrator, an error, an unknown) leaves the channel rule inert for that check: nobody is punished and the page's "bot is not an admin" line shows. So even if a non-admin bot's `getChatMember` returned a plausible-looking `left` for subscribed members, nobody would be punished on it.
- **D-23 (research C3, C4, C5):** "I fixed it" step 4b reads approvals through the existing `memberReqApprovedLookup` seam (the cached approvals list; another replica's in-process layer can lag by up to 10 s, accepted); a definite "still broken" rule beats an unknown lookup in the answer (broken-first), an unknown lookup alone is `mreq_fixed_check_failed`. Every redraw of the page or the channel screen reads the status through one helper that is bounded and paced, so the "at most 2 calls" budget is per draw, and the linked channel and the bot's admin status are read from short positive caches wherever a draw would otherwise repeat the call (the planner picks the key names inside the `fuku:memberreq:*` family; no migration).
- **D-24 (research C6):** the research's suggested short negative cache (PITFALLS) is rejected; D-03 and D-12 stand: only positive results are cached, and the cost (one paced lookup per message of a member who stays without the photo or subscription) is documented in AGENTS.md.
- **D-25 (research Open Question 3):** on a lookup cache miss the watcher does not start a job for a member the cached admin list knows as an administrator or for an approved member, so exempt members without a photo cost nothing.

### Claude's Discretion
- Exact locale keys, wording, string lengths and icons (fixed in the UI contract within the 20-code-point cell limit and the 3800-unit page limit), the short codes if `ph` or `ch` clash, the lookup pacer's interval, retry count and wait (the Phase 13 pacer is the model), the linked-channel cache TTL within about 10 minutes, the order of the lookups inside the job (photo before channel or the reverse, each only when its rule is on), how the watcher's clean verdict fingerprint is extended to the two lookup verdicts, and the migration's file name.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `fuku/modules/memberreq_rules.go`: `memberReqRules` is the rule table (a row per rule: `Code`, `Bit`, `Store`, `LineKey`, `CellKey`, `DescKey`, `PhraseKey`, `Broken`); the comment says the next free bit is 32. `memberReqBroken`, `memberReqStrictest`, `memberReqPassValue` and `memberReqNamesOf` judge the pure name rules; the two new rules need a lookup and so cannot be a pure `Broken` func.
- `fuku/db/memberreq/settings.go` (`Rule` enum, `GetSettings`, `GetSettingsFresh`, `SetRuleAction`, `AnyRuleOn`) and `fuku/db/models/member_requirements.go` (`ChatRequirements`, five action columns with named CHECKs) are where a rule's column lives; `migrations/20261106120000_add_member_requirements.sql` is the pattern for the next migration.
- `fuku/modules/memberreq_enforce.go`: `memberReqPacer` (`ratelimit.NewTelegramPacer`), `memberReqPaced`, `memberReqLiveViolation` (the live lookup and the fresh settings re-read, where lookups slot in), `memberReqLiveMember`, `memberReqUnmutePermissions`; `memberreq_watch.go`: `memberReqContext`, `memberReqPassHit`, `memberReqRunAsync` (32 jobs, 90 s), `StopMemberReqJobs`; `memberreq_fixed.go`: the "I fixed it" order whose step 5 judges the rules again; `memberreq_notice.go`: `memberReqRulesText`, `memberReqPostNotice`.
- `fuku/modules/settings_mreq.go` renders the page, the rule screens and the cards from `memberReqRules`; `settings_card.go` is the shared two-tap card; `fuku/utils/ratelimit` holds the Telegram pacer; `raidAIImageFlight` (singleflight) and `staffPaced`/`lockdownPaced` are the precedent for coalescing and fleet-wide pacing.
- `gotgbot` `Bot.GetUserProfilePhotos(userId, &GetUserProfilePhotosOpts{Limit: 1})` (`TotalCount`), `Bot.GetChat` (`ChatFullInfo.LinkedChatId`) and `Bot.GetChatMember(channelId, userId)` (`MergeChatMember`), per `.planning/research/STACK.md`.

### Established Patterns
- One setting, one code path: a rule's choice is written by `applyMemberReqAction` and read by the page through `memberReqSettingsLookup`; reads create no row; every write drops the `member_requirements` cache key (in `skipLocal`).
- Anything that cannot be answered punishes nobody and creates no hold; the hold is claimed only after the punishment is decided (Phase 13).
- New background Telegram work is paced fleet-wide in a `fuku:*` Redis family outside the cache prefix, bounded by a context, shed on a far slot, with panics recovered; a drain is added only when work outlives the existing one (none here).
- Tests use the `memberReqEnv` toolkit (`memberreq_env_test.go`), fresh IDs and cleanups, the hand-written bot fake and miniredis; PostgreSQL-only guarantees join `make test-postgres-integrity`.

### Integration Points
- The rule table, the settings model and migration, the page and rule screen (`settings_mreq.go`), the executor's live step (`memberReqLiveViolation`), the watcher's clean verdict and pass cache, the join entry (`memberReqJoinThenWelcome`) and "I fixed it" (step 5) all read the same rule set, so each must learn the two lookup rules.
- `main.go` and the shutdown drains stay as they are; AGENTS.md's handler, callback, data and subsystem bullets and the locale files (8) are extended.

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond the accepted answers. Treat a photo hidden by privacy as missing, say so on the page, and recommend Warn or Mute for it; a lookup that cannot be answered must never be the reason anyone is punished.

</specifics>

<deferred>
## Deferred Ideas

- The AI spam-name rule and its "AI not configured" status line (Phase 15, MREQ-14).
- Deleting requirement notices automatically on the group's timer (Phase 16, MREQ-15).
- Screening in a basic group: only supergroups are screened (mute and kick are supergroup methods); this phase's page says so, a later phase may decide otherwise.
- A short cache for a "missing" photo or "not subscribed" answer (60 s), judged unnecessary here because a miss is looked up once per message through the pacer.
- A separate lookup worker pool and a dedicated shutdown drain, judged unnecessary because lookups run inside the existing jobs and the existing drain.

</deferred>
