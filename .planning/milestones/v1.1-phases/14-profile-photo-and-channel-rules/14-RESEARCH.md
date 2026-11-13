# Phase 14: Profile Photo and Channel Rules - Research

**Researched:** 2026-11-08
**Domain:** Telegram bot (Go 1.26, gotgbot v2 rc.36, GORM PostgreSQL/SQLite, Redis): two Telegram-lookup rules (profile photo, channel subscription) added to Phase 13's member requirements engine (rule table, watcher at group -2, executor, join entry, "I fixed it", settings page), with a fleet-wide lookup pacer, singleflight, positive-only caches and the M13 inert classifier
**Confidence:** HIGH for everything grounded in repo code, the gotgbot module source and the `golang.org/x/sync` source read in this session; MEDIUM-LOW for the three Telegram-side behaviours (S1, S2, S3) that only a live client can settle (core.telegram.org does not resolve from this session: `getaddrinfo ENOTFOUND core.telegram.org`; two WebSearch queries found no authoritative statement; see the Assumptions Log). Every code path below is safe under every outcome of the three spikes.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Profile-photo rule (accepted, area 1)
- **D-01:** A member with no photo the bot can see is in breach, and a hidden photo counts as missing. `getUserProfilePhotos(user, limit 1)` with a total of zero is the breach; any error, timeout or shed lookup is unknown and never a breach (MREQ-10, research Pitfall 20). The rule's page text says that a photo hidden by privacy settings counts as missing and what the member can do (show the photo to everyone, or ask an admin to approve them).
- **D-02:** The photo is looked up only in the enforcement job (and in the join entry's synchronous call), never in the watcher's pure path: after the exemptions and the live `getChatMember` and before the cap and the claim, only while the photo rule's action is not Off, so a group with the rule Off makes no call.
- **D-03:** Only a found photo is cached: `fuku:memberreq:photo:<user>` for 30 minutes (a photo is the user's, not the group's, so the key has no chat). A missing photo and an unknown answer are never cached, so a member who adds a photo is noticed at their next message and nobody is held on an old answer.
- **D-04:** Off by default like every rule. The rule screen's hint recommends Warn or Mute and not Kick or Ban, because a member cannot always fix a hidden photo from the group; all five punishments stay selectable behind the same two-tap card for Kick, Ban and Mute (Phase 13 D-05). Approved users, admins, bots, service accounts and members with a pending captcha are exempt exactly as in Phase 13 (D-11 there).

#### Channel-subscription rule (accepted, area 2)
- **D-05:** The rule is offered only in a group linked to a channel. Its cell is always drawn on the page; in a group with no linked channel its screen says so and offers Off only, so a stored non-Off value after the channel is unlinked is inert (the check cannot run, nothing is punished) and can be turned Off.
- **D-06:** The linked channel is `getChat(group).LinkedChatId`, resolved once per chat and cached about 10 minutes in Redis (`fuku:memberreq:lc:<chat>`, positive and "none" alike) for the job; opening the rule screen reads it fresh and refreshes the cache. It is never taken from callback data or from the update.
- **D-07:** Subscribed means `getChatMember(channel, user)` answers member, administrator, creator, or restricted with `is_member`. Left and kicked are not subscribed and are the breach. Every error, timeout and shed lookup, and the answer a bot that is not the channel's admin gets, is unknown and never punishes. `chat_status.IsBotMissingError` reads "chat not found" as the bot being missing, which is wrong for a channel lookup, so the channel path classifies by answer and never by that helper (research flag; settled by the live spike).
- **D-08:** Opening the channel rule's screen makes at most one bounded live `getChatMember(channel, bot)` and shows a status line when the rule cannot work (the bot is not an administrator of the linked channel). The punishment can still be set; the check simply fails open. A status that cannot be determined shows the page's unknown mark, never an error and never a refusal.

#### Lookup budget, pacing and shutdown (accepted, area 3)
- **D-09:** Photo and channel lookups (including the linked-channel `getChat`) share one new fleet-wide pacer with its own Redis keys (`fuku:memberreq:lkp:pace:next` and `:block`), built like `memberReqPacer`/`staffPaced` (`ratelimit.NewTelegramPacer`, a slot reservation plus a shared `retry_after` block). A call whose slot is far away is shed as unknown with no Telegram request. Punishments and notices keep Phase 13's own pacer, so a burst of lookups can never delay a mute notice. No per-replica limiter holds a shared budget.
- **D-10:** Lookups run inside the existing enforcement job, never on a new goroutine pool, with a singleflight per lookup key so 100 messages from one user make one lookup. The watcher stays cheap: for a member whose photo and subscription verdicts are both cached positive (and whose names are clean) the message is clean as before; a miss hands one job to the existing runner (still capped at `memberReqMaxInFlight` 32 and shed on overflow). A held member's messages take the held path before any lookup.
- **D-11:** No new shutdown drain and no change to the drain budget. Every lookup derives from `memberReqContext()`, which `StopMemberReqJobs` cancels (2 s wait plus 1 s grace). The claim of the hold happens only after every lookup answered, so a lookup cut off by a crash or a shutdown leaves no hold, no notice and punishes nobody; the member's next message starts again.
- **D-12:** Caches are positive only: photo `fuku:memberreq:photo:<user>` and subscription `fuku:memberreq:sub:<channel>:<user>`, 30 minutes each, both in the `fuku:memberreq:*` family outside the `fuku:cache:` prefix. "I fixed it" always does a fresh lookup that bypasses both caches; a lookup it finds positive may refresh them. Without Redis every lookup is a miss and the pacer falls back to one replica's interval (correct, slower, documented).

#### Page, notices and "I fixed it" (accepted, area 4)
- **D-13:** The two rules are appended to the rule table (the photo rule is bit 32, the channel rule bit 64, short codes `ph` and `ch`, `memberreq.RulePhoto` and `RuleChannel`) with two new `SMALLINT` columns `photo_action` and `channel_action` (named CHECKs 0..4, default 0) in one new migration with a timestamp above `20261106120000`, one transaction, both appliers agreeing. The model, the repository's column-targeted writes (`SetRuleAction`), the harness AutoMigrate lists and the Phase 13 page, rule screen, cards and controls extend from the table; `settings_pages.go` is not edited.
- **D-14:** The rule grid grows to seven cells (the odd last cell alone on its row); the row budget test and the worst-case callback test are updated for it. The rule screen, the five punishments, the two-tap card and the rights (`settingsRightAdmin` to open, `settingsRightRestrict` to choose) are Phase 13's, unchanged.
- **D-15:** The page shows a rule that cannot work here as one "cannot work here" line, on the page body and on that rule's screen: the channel rule with no linked channel, the channel rule with the bot not an administrator of the linked channel, and, for every rule, a basic group (Phase 13 left that line for this phase: only supergroups are screened). The page open stays fast: the extra bounded Telegram calls (the cached linked channel and, only while the channel rule is not Off, one live bot-admin read) are made at most once per open, and a failed one shows the unknown mark. The "AI not configured" line is Phase 15's.
- **D-16:** The notice names every broken rule as in Phase 13 (D-04 there). The photo rule's phrase names the real remedy (show the profile photo to everyone, or ask an admin to approve you); the channel rule's phrase says to subscribe to the linked channel. A mute still carries the one "I fixed it" button. New strings go to all 8 locale files; the exact keys, wording and lengths are fixed in the UI contract.
- **D-17:** "I fixed it" re-checks every enabled rule live: the name rules on the live names as before, the photo and channel rules with fresh lookups through the lookup pacer under the press's 10 s budget. A member who still lacks the photo or the subscription stays held and is told which rules remain (`mreq_fixed_still_broken`). A lookup that cannot be answered at that moment is `mreq_fixed_check_failed` and changes nothing, exactly like an unreadable setting.
- **D-18:** Privacy and logs: the photo lookup sends Telegram only the user ID, the channel lookup the channel and user IDs. Log lines carry chat, channel, user IDs and outcomes only, never a name or a notice text. No screen names the Staff Group or shows a count of held or punished members (Phase 13 D-16 stands), and a group menu never reveals the linked channel's title or ID (the status lines are words, not identifiers).

#### Verification, spikes and tests
- **D-19:** Two live spikes are the owner's, deferred like the earlier phases' (the owner's standing choice): does `getUserProfilePhotos` return zero or an error for a user who hides their photo, and what exact error does `getChatMember(channel, user)` give when the bot is not the channel's admin. They go into `14-SPIKE.md` as non-blocking live checks, with the Phase 13 live checks' warning that nothing punishes on an unknown. The code treats both shapes safely either way (zero photos is a breach and an error is unknown for the photo; any error is unknown for the channel), so the spikes change wording, not safety.
- **D-20:** The tests cover: both rules punishing a violator and sparing a member who has a photo or a subscription; every unanswerable shape (Telegram error, timeout, shed by the pacer, the context cancelled by `StopMemberReqJobs`) punishing nobody and leaving no hold; the singleflight (100 messages from one user, one lookup); positive-only caching (a missing photo is looked up again at the next message); the lookup pacer shared across two replicas over one miniredis, with Phase 13's pacer untouched by it; the linked-channel cache and the page's three statuses (no linked channel, bot not admin, basic group); "I fixed it" with fresh lookups and the remaining-rules answer; the migration's CHECKs on PostgreSQL (`TestChatRequirementsSettings` extended, in the integrity list) and its zero defaults for an existing row; the page grid, row budget, callback bytes and 8-language fit tests; and `TestSettingsStaffPrivacy` over the new screens.

### Claude's Discretion
- Exact locale keys, wording, string lengths and icons (fixed in the UI contract within the 20-code-point cell limit and the 3800-unit page limit), the short codes if `ph` or `ch` clash, the lookup pacer's interval, retry count and wait (the Phase 13 pacer is the model), the linked-channel cache TTL within about 10 minutes, the order of the lookups inside the job (photo before channel or the reverse, each only when its rule is on), how the watcher's clean verdict fingerprint is extended to the two lookup verdicts, and the migration's file name.

### Deferred Ideas (OUT OF SCOPE)
- The AI spam-name rule and its "AI not configured" status line (Phase 15, MREQ-14).
- Deleting requirement notices automatically on the group's timer (Phase 16, MREQ-15).
- Screening in a basic group: only supergroups are screened (mute and kick are supergroup methods); this phase's page says so, a later phase may decide otherwise.
- A short cache for a "missing" photo or "not subscribed" answer (60 s), judged unnecessary here because a miss is looked up once per message through the pacer.
- A separate lookup worker pool and a dedicated shutdown drain, judged unnecessary because lookups run inside the existing jobs and the existing drain.
</user_constraints>

**Binding amendments of the approved UI contract (14-UI-SPEC.md, not reopened here):** M6 (nine icon-only cells), M7 (the channel screen is `[Off]` alone while it cannot be linked), M8, M9 (the page makes bounded paced calls only while the channel rule is not Off and the group is a supergroup), M10 (notice cap 400 to 520), M11 (new step 4b: approvals before any rule is judged at "I fixed it"), M12 (seven existing keys reworded, no "profile" for a channel breach), M13 (the inert classifier), and the planner notes R1 to R4 (R2: the classifier never runs in the watcher, an inert verdict is never cached as a pass, `ClearHoldOnClean` never lifts a mute; R3: `cache.IsBotGoneError` matches any 400 or 403; R4: four new keys contain "profile": `mreq_rule_photo`, `settings_mreq_rule_photo`, `settings_btn_mreq_photo`, `settings_mreq_desc_photo`).

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MREQ-10 | A check that cannot be answered (a Telegram error, a lookup that times out, the bot not being an admin of the linked channel) never punishes anyone. The page shows when a rule cannot work in that group (no linked channel, the bot not admin there; "AI not configured" is Phase 15) | S-01 (unknown is the zero value of the verdict; unknown and inert are both "not broken"), S-02 (where the lookups sit, decoupled from established name breaches), S-06 (classifier), S-09 (status block), S-12 (the unanswerable-shape matrix) |
| MREQ-11 | The profile-photo rule treats a photo the bot cannot see as missing, and the page says so | S-04 (photo verdict and positive-only cache), S-02, S-07 (fresh at the tap), S-09 (photo screen), S-11 (spike S1) |
| MREQ-12 | The channel rule is offered only in a group linked to a channel, and checks that the member is subscribed to that channel | S-04 (linked channel resolution, subscription verdict), S-09 (Off-only screen, refused punishing press), S-11 (spikes S2, S3) |
| PLAT-06 | New background work shares Telegram rate budgets across replicas, recovers from crashes, and is drained within the existing shutdown budget (this phase: the requirement lookups) | S-04 (lookup pacer, flights recover internally), S-05 (no new drain, the budget recomputed), S-12 (pacer shared across two replicas) |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

- Stay on Go 1.26.0, gotgbot v2, GORM (PostgreSQL production, SQLite tests), Redis. **No new dependency**: `golang.org/x/sync v0.23.0` is already a direct requirement [VERIFIED: go.mod:26 `golang.org/x/sync v0.23.0`]; `singleflight` is already imported in `users.go:14`, `raid_ai_image.go:14`, `settings_gate.go:12`, `fuku/utils/cache/adminCache.go:13`.
- Migrations append-only, timestamped above every existing file (latest `20261106120000_add_member_requirements.sql`), one transaction, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`; `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` must agree.
- Every write `cache.DeleteCache`s the keys it affects (the new columns ride the existing `member_requirements:<chat>` key, already in `skipLocal`, `fuku/db/cache/local.go:49,77`). The new Redis keys are positive-only verdict caches outside `fuku:cache:`: no `DeleteCache` duty, no `skipLocal` entry (they are not read through `GetFromCacheOrLoad`).
- Watchers return `ext.ContinueGroups`; the member requirements watcher is the only message handler of group -2 (`memberreq.go:50-53`); no new handler group is added by this phase.
- Locale keys in all 8 files (en, es, fr, hi, id, pt, ru, uk); every `settings_` key has one row in `fuku/i18n/settings_locale_test.go`, every `mreq_` key one row in `memberReqLocaleLimits`.
- Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`; a singleflight `DoChan` leader that panics **crashes the process** (S-04), so its function recovers internally.
- Tests: `-tags testtools`, real fixtures (no mock libraries, hand-written `gotgbot.BotClient` fakes), `make test`; PostgreSQL-only guarantees join `make test-postgres-integrity` (`Makefile:39-44`, an anchored `-run` list plus a package list).
- Security: callback data is never authority; logs carry IDs and outcomes only (never a name, a notice text, a photo file ID, a link or a Telegram description: `memberReqErrText` prints a Telegram error by its code, `memberreq_enforce.go:128-134`); no screen names the Staff Group or the linked channel's title or ID (D-18).
- AGENTS.md is updated in the same commit as any change that invalidates a rule in it (S-10 lists them). GSD workflow: all edits through `/gsd-execute-phase`.
- Nothing in this research edited source, tests, locales or other planning files; nothing was committed.

## Summary

Phase 14 is small in new concepts and wide in touch points: two rule-table rows, one new file of lookups, one pacer, a few new caches, and an extension of the five places that already read the rule set (executor, watcher, join entry via the executor, "I fixed it", page). The central design fact is that **Phase 13 already puts the right seams in the right order**. `memberReqEnforce` reads the hold (`memberreq_enforce.go:523-531`) and decides the held path **before** the live `getChatMember` (`:540`), the settings re-read and the names (`memberReqLiveViolation`, `:239-260`), and well before the cap (`:549`) and the claim (`:553`). The two lookups therefore slot into `memberReqLiveViolation` after the names, with no reordering, and the join entry (`memberreq_join.go:105`) reaches them through the same executor with no code change. The rule table needs one new field (`Lookup`, a function) beside the existing `Broken` (nil for the two new rows); `memberReqBroken` keeps judging only the five name rules, so their behaviour cannot change; a second function folds the lookup verdicts into the same bit mask, and `memberReqStrictest`, `memberReqRulesText`, the notice, the hold's `rules` column and `memberReqPassValue` already iterate the table or the `memberreq.Rules` enum and pick the new rows up.

Three findings change the plan relative to the CONTEXT and UI contract (details in S-03, S-02, S-04): (1) **a literal D-10 deadlocks the release of a Warn hold.** If a held member's messages never trigger a lookup, a Warn hold with a lookup bit is never cleared after the member fixes the photo, the positive cache is never written for them, and the watcher hands a job to the runner on every one of their messages for as long as the hold lives. UI-SPEC R2 already assumes the job "re-judges" a held member, so this research keeps D-10's intent (no lookup before the *punishment* path; the hold is read first) and adds a **held-release re-check** in the job, limited to the lookup bits of the hold, run only when the names are clean, never punishing, ending in `ClearHoldOnClean`. (2) **A singleflight `DoChan` leader that panics crashes the whole process** (`go panic(e)` in `golang.org/x/sync@v0.23.0/singleflight/singleflight.go` `doCall`), and a plain `Do` follower ignores its own context. The lookups therefore use `DoChan` with a function that recovers internally, run under a context derived from `memberReqContext()` (so `StopMemberReqJobs` ends them, D-11) and bounded on their own, while every caller waits with `select` on its own deadline. (3) **`memberReqLookupPaced` must be a separate pacer from `memberReqPacer`**, with `MaxRetries: 1` and a short `MaxWait` (5 s), so a Telegram flood block longer than 5 s sheds every lookup at once (no Telegram request, no waiting) instead of piling jobs behind it.

The shutdown budget is unchanged and **no new drain is needed**: every lookup call derives from `memberReqContext()`, which `StopMemberReqJobs` cancels first (`memberreq_watch.go:244-258`); a cancelled paced call aborts at once (`TelegramPacer.wait` selects on `ctx.Done()`, `telegram_pacer.go:164-188`), jobs end in milliseconds inside the 2 s wait, and lookups read and write only Redis and Telegram, never PostgreSQL, so nothing can outlive the database close. The claim stays after every lookup, so a lookup cut off leaves no hold (D-11); the existing worst case (65 s of drains against the 60 s budget with `stop_grace_period: 75s`, AGENTS.md lines 47-57) does not move by a second.

**Primary recommendation:** add `fuku/modules/memberreq_lookup.go` (verdict type with unknown as the zero value, the lookup pacer, the flights, the three positive-only caches, the M13 classifier), give the rule table a `Lookup` field, fold lookup verdicts into the broken mask inside `memberReqLiveViolation` (photo, then channel, each only while its action is not Off, under a 20 s job budget and a 4 s join budget), extend the watcher with one pipelined read of cached positives before it hands a job, add the held-release re-check, and add the approvals step 4b plus fresh lookups to "I fixed it".

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Photo and channel rule settings (`photo_action`, `channel_action`) | Database (`chat_requirements`, two new columns) | Cache (`member_requirements` key, already `skipLocal`) | Same row, same read path and invalidation as the five name rules (D-13) |
| Lookup verdicts (photo present, channel subscribed) | Bot process, Telegram Bot API (paced) | Redis (positive caches, 30 min) | Telegram is the only source; Redis only saves calls and is never needed for correctness |
| Lookup rate budget | Redis (`fuku:memberreq:lkp:pace:next`, `:block`) | in-process fallback (the pacer's local interval) | One fleet-wide budget separate from the punishment pacer (D-09) |
| Linked channel of a group | Telegram (`getChat`) | Redis (`fuku:memberreq:lc:<chat>`, 10 min, positive and "none") | Never from callback data or the update (D-06) |
| Episode latch, hold, one punishment | Database (`chat_requirement_holds`, unchanged) | none | Phase 13's guarantee; lookups happen before the claim |
| Watcher's cheap path | Bot process (group -2) + Redis reads | none | No Telegram call, no database write for a clean member (D-10) |
| "Cannot work here" status | Settings page (bounded, paced reads) | Redis (`lc` cache) | Words only, never identifiers (D-15, D-18) |
| Unmute at "I fixed it" | Callback handler (`mreq\|v1`) | Telegram (fresh lookups), Database (approvals, hold) | Presser-only, conditional delete, `resolveUnmutePermissions` (unchanged) |

## Standard Stack

### Core (all existing, no new dependency)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36.0.20260919140833-240296efadb4 [VERIFIED: module cache path read this session] | `GetUserProfilePhotosWithContext`, `GetChatWithContext` (`ChatFullInfo.LinkedChatId`), `GetChatMemberWithContext`, `MergeChatMember` | The bot's client |
| `golang.org/x/sync/singleflight` | v0.23.0 [VERIFIED: go.mod:26] | one lookup per key however many callers | Already used 4 times in the repo |
| `github.com/redis/go-redis/v9` | v9.22.0 (CLAUDE.md stack) | positive caches, the pacer's slot and block keys | Existing client via `memberReqRedisClient` |
| `fuku/utils/ratelimit` `NewTelegramPacer` | in repo | the lookup pacer | The fleet-wide pacer every fan-out uses |
| `github.com/alicebob/miniredis/v2` | v2.39.0 (CLAUDE.md stack) | Redis in tests, two replicas over one server | every module test |

### gotgbot API shapes (read from the module cache this session)
| Call | Shape | Verified text |
|------|-------|---------------|
| photo | `func (bot *Bot) GetUserProfilePhotosWithContext(ctx context.Context, userId int64, opts *GetUserProfilePhotosOpts) (*UserProfilePhotos, error)`; `GetUserProfilePhotosOpts{Offset int64; Limit int64; RequestOpts *RequestOpts}`; `UserProfilePhotos{TotalCount int64 \`json:"total_count"\`; Photos [][]PhotoSize \`json:"photos,omitempty"\`}` | `gen_methods.go:3809-3849`, `gen_types.go:15547-15552`. The result is `json.Unmarshal` into a `*UserProfilePhotos`, so a body of `null` gives a **nil pointer with a nil error**: treat nil as unknown, never dereference |
| linked channel | `func (bot *Bot) GetChatWithContext(ctx context.Context, chatId int64, opts *GetChatOpts) (*ChatFullInfo, error)`; `LinkedChatId int64 \`json:"linked_chat_id,omitempty"\`` ("Unique identifier for the linked chat, i.e. the discussion group identifier for a channel and vice versa") | `gen_methods.go:2822-2838`, `gen_types.go:1814-1815`. Same nil-on-`null` trap. Absent means 0 |
| member | `func (bot *Bot) GetChatMemberWithContext(ctx context.Context, chatId int64, userId int64, opts *GetChatMemberOpts) (ChatMember, error)`; doc: "The method is only guaranteed to work for other users if the bot is an administrator in the chat" | `gen_methods.go:2950-2977`. An unrecognised status comes back as `ChatMemberUnknown{RawStatus, Data}` with **no error** (`gen_types.go:2191-2250`, final `return ChatMemberUnknown{RawStatus: t.Status, Data: d}, nil`), and an empty body as `(nil, nil)`: both must be classified unknown. Status constants: `ChatMemberStatusCreator = "creator"`, `...Administrator = "administrator"`, `...Member = "member"`, `...Restricted = "restricted"`, `...Left = "left"`, `...Kicked = "kicked"` (`gen_consts.go:222-227`) |
| administrator right | `CanManageChat`: "True, if the administrator can access the chat event log, get boost list, see hidden supergroup and channel members, ..." | `gen_types.go:2267` (the `ChatMemberAdministrator` field comment): consistent with "the bot must be an administrator of the channel to read members". `ChatMemberUnknown.MergeChatMember()` returns an empty `MergedChatMember` (`gen_types.go:2154-2156`), so an unknown status merges to `Status == ""` |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `DoChan` + select | plain `singleflight.Do` | `Do` is simpler and a leader panic is recoverable by the caller, but a follower waits with no regard to its own deadline (a page open bounded to 3 s would wait for a job's 15 s flight). `DoChan` needs the internal recover (below) |
| a new pacer | `memberReqPacer` | Sharing would let a lookup burst delay a mute notice (D-09 forbids it) |
| `Do` leaders on the caller's context | flight on `memberReqContext()` | A short caller deadline (a 3 s page open) would cancel a lookup other callers share; deriving from the root also gives D-11 for every entry (job, join, press) |
| negative cache (60 s) | none | Deferred idea (D-03, CONTEXT "Deferred"); PITFALLS Pitfall 5 suggested it, CONTEXT wins |

**Installation:** none.

**Version verification:** no package is added; `go.mod` and `go.sum` are untouched (the T-13-SC check `git diff --exit-code <base> -- go.mod go.sum` applies again).

## Package Legitimacy Audit

No external package is installed by this phase. `golang.org/x/sync` is already pinned in `go.mod:26`; the legitimacy seam has nothing to check.

| Package | Registry | Verdict | Disposition |
|---------|----------|---------|-------------|
| (none new) | - | - | - |

**Packages removed due to [SLOP] verdict:** none. **Packages flagged as suspicious [SUS]:** none.

## Settled Findings (what the planner builds on)

### S-01. The rule table, the verdict type and the broken mask (question 1)

**What exists** [VERIFIED: `memberreq_rules.go:34-43`]:

```go
type memberReqRule struct {
	Code      string
	Bit       int
	Store     memberreq.Rule
	LineKey   string
	CellKey   string
	DescKey   string
	PhraseKey string
	Broken    func(memberReqNames) bool
}
```

`memberReqRules` holds five rows with `Bit` 1, 2, 4, 8, 16 and the comment "A later phase appends rows (the next free bit is 32)" (`:90-93`). `memberReqBroken` calls `rule.Broken(names)` for every rule whose action is not Off (`:141-155`), so a row with a nil `Broken` **would panic there**; `TestMemberReqRuleTable` fails a nil `Broken` today (`memberreq_rules_test.go:328-329`, "has no Broken test") and asserts `rule.Bit != 1<<i` fails (`:312`), which 32 and 64 satisfy as rows 6 and 7.

**Recommendation (minimal, five name rules untouched):**

1. Add one field to the row type: `Lookup func(ctx context.Context, b *gotgbot.Bot, chatID, userID int64, mode memberReqLookupMode) memberReqVerdict`. A name rule has `Broken` set and `Lookup` nil; a lookup rule the reverse. `TestMemberReqRuleTable` changes from "every rule has Broken" to "exactly one of `Broken` and `Lookup` is set".
2. Append two rows (keys fixed by the UI contract: `settings_mreq_rule_photo`, `settings_btn_mreq_photo`, `settings_mreq_desc_photo`, `mreq_rule_photo`; `settings_mreq_rule_channel`, `settings_btn_mreq_channel`, `settings_mreq_desc_channel`, `mreq_rule_channel`): `{Code: "ph", Bit: 32, Store: memberreq.RulePhoto, ... Lookup: memberReqPhotoRule}` and `{Code: "ch", Bit: 64, Store: memberreq.RuleChannel, ... Lookup: memberReqChannelRule}`. Table order is photo then channel, which is also the order the lookups run (the order is Claude's discretion; photo first because its verdict is per user across all groups and needs no chat state, while the channel needs the link resolved first and costs up to three calls).
3. `memberReqBroken` gains one line, `if rule.Broken == nil { continue }`, and keeps returning **only name bits**. Its tests (`TestMemberReqStrictest` at `memberreq_rules_test.go:376-396`, which at `:391` compares `memberReqBroken(requirementsOf(1, 1, 1, 1, 1), everything)` with `allBits` computed over **every** table row) must compute `allBits` over rows with a non-nil `Broken`, or they fail with 31 against 127.
4. Add `memberReqLookupBroken(ctx, b, settings, chatID, userID, mode) (mask int, unknown bool)`: loop the table in order, skip rows with nil `Lookup` and rows whose action is Off, call `Lookup`; `memberReqBreach` sets `rule.Bit`; `memberReqUnknown` sets `unknown`; `memberReqPass` and `memberReqInert` add nothing. The caller ORs the result into the name mask.
5. **The verdict type has unknown as its zero value** (`memberReqUnknown = iota`, then `memberReqPass`, `memberReqBreach`, `memberReqInert`), so any path that forgets to set a verdict cannot punish (MREQ-10 by construction, not by discipline). Inert exists only for the channel rule and means "cannot work here"; it counts as not broken everywhere, exactly like unknown, and differs only where a *hold* is re-judged (S-02 held release, S-07).

**What the existing generic code does with the new bits, unchanged** [VERIFIED: files read]: `memberReqStrictest` (`:177-192`) iterates the table by bit and reads `memberreq.ActionOf(settings, rule.Store)`, so ban over kick over mute over warn (`memberReqSeverity`, `:160-172`) covers the new rows once `ActionOf` knows them; `memberReqRulesText` (`memberreq_notice.go:60-68`) lists `rule.PhraseKey` for every set bit in table order, which gives the seven-phrase list with no change; the hold's `rules` is `INTEGER NOT NULL` with `CHECK (rules > 0)` (`migrations/20261106120000_add_member_requirements.sql`, holds table), so 32 and 64 fit; `memberReqPassValue` (`:200-216`) writes `strconv.Itoa(memberreq.ActionOf(settings, rule))` for every `memberreq.Rules` entry, so adding the two enum values changes the fingerprint of every member once (a one-time pass-cache miss after the deploy, one `ClearHoldOnClean` statement per member, harmless; `TestMemberReqPassValue` gains "a change of the photo or channel action changes the value").

**The notice's remedy tail** (UI-SPEC section 7, `mreq_remedy_photo`): the single place is `memberReqNoticeText` (`memberreq_notice.go:108-112`). After `memberReqNoticeKey(action, warn)` picks the template, append `" " + memberReqText(tr, "mreq_remedy_photo")` to the translated template **before** the name splice, only when the key is one of `mreq_notice_warn`, `mreq_notice_kick`, `mreq_notice_mute` and `broken&photoBit != 0` (find the bit through the table row with `Code == "ph"`, never a literal 32). The warn-limit variants and `mreq_notice_ban` get no tail. The tail is part of the text the 520 cap test renders (M10).

**The still-broken answer** (UI-SPEC section 7): count the broken rules whose action is not Off; one to four phrases use `mreq_fixed_still_broken` with `{rules}`, five or more use `mreq_fixed_still_broken_many` with `{n}`. One small function beside `memberReqRulesText` (`memberReqStillBrokenText(tr, broken)`) keeps this out of `memberReqFixedRun`.

### S-02. Where the lookups sit in the executor and the join (question 1)

**Order of `memberReqEnforce` today** [VERIFIED: `memberreq_enforce.go:519-568`], with the one new step in bold:

| # | Step | Line | Telegram call |
|---|------|------|---------------|
| 1 | bot itself, bots, service accounts; approved or pending captcha (tri-state, unknown = exempt) | `:520` | none (cached reads) |
| 2 | hold read fresh; a non-stale hold is the **held path** | `:523-531` | none |
| 3 | paused chat: no punishment, no lookup | `:532-539` | none |
| 4 | one live `getChatMember` (admin check and names) through `memberReqPaced` | `memberReqLiveViolation` `:240-247` | 1, Phase 13 pacer |
| 5 | settings read again; unreadable punishes nobody | `:248-257` | none |
| 6 | the name rules on the live names: `memberReqBroken` | `:258` | none |
| **6b** | **the lookup rules (photo, then channel), each only while its action is not Off, folded into `broken`** | new, inside `memberReqLiveViolation` after `:258` | 0 to 4, lookup pacer |
| 7 | mute over a restriction someone else placed | `:546` | 0-1 |
| 8 | cap (counted before the claim) | `:549` | none |
| 9 | claim | `:553` | none |
| 10 | punishment, notice, delete | `:556-566` | Phase 13 pacer |

`memberReqLiveViolation(ctx, b, c *memberReqCase)` keeps its signature `(member gotgbot.MergedChatMember, broken int, ok bool)`: after `broken = memberReqBroken(...)` it does

```go
// proposed
lookupCtx, cancel := memberReqLookupPhase(ctx, c.Source) // 20 s for a message job, 4 s for a join
defer cancel()
lookupBroken, _ := memberReqLookupBroken(lookupCtx, b, c.Settings, c.Chat.Id, c.User.Id, memberReqLookupMode{})
broken |= lookupBroken
return member, broken, broken != 0
```

D-02 is met in letter: after the exemptions (step 1), after the live `getChatMember` (step 4), only while the action is not Off (a group with the rule Off makes no call: the loop skips it), before the cap (step 8) and the claim (step 9). A paused chat (step 3) makes no lookup either.

**Decision: an unknown lookup never blocks an established name breach.** If the names break a rule and the photo lookup is unanswerable, the episode proceeds on the name mask, and the notice names only the rules that were established. The alternative (abort the whole episode on any unknown) would couple Phase 13's name rules to the availability of the lookup pacer: during a raid the pacer sheds, and every name violator would escape for as long as it does. The cost is a partial episode (a Ban chosen for the photo rule is not applied to a member who was Warned for a name rule while the photo lookup was shed); the hold's mask lacks the photo bit, and the member is re-judged for it only after the hold clears. This is the same "unknown does nothing" rule, applied per rule. [ASSUMED: owner-acceptable; flagged as Open Question 4]

**Budgets.** `memberReqLookupPhase` returns `context.WithTimeout(ctx, memberReqLookupJobBudget)` with `memberReqLookupJobBudget = 20 * time.Second` for a message job and `memberReqLookupJoinBudget = 4 * time.Second` for `memberReqFromJoin` (variables so tests can shorten them). Why: the job context is 90 s (`memberReqJobTimeout`, `memberreq_watch.go:36`) and must end 30 s before `memberreq.StaleClaim` (T-13-57, asserted by `TestMemberReqRunnerSheds`); the lookups run before the claim, so they cannot break that margin, but without a phase budget a worst case of three paced calls at 5 s of pacer wait plus 10 s of call each could eat 45 s of the 90 s and leave the notice of a mute short of time. The join path runs on a dispatcher goroutine with a 15 s context (`memberReqJoinTimeout`, `memberreq_join.go:26`); a 4 s phase budget keeps a raid of joins from parking 200 goroutines for 15 s each, and a lookup that did not finish only means the member is welcomed and caught at their first message by the watcher (every member is checked at their message, Phase 13 MREQ-02).

**The join entry needs no code change.** `memberReqJoinThenWelcome` reads `AnyRuleOn(settings)` (`memberreq_join.go:88`), which iterates `memberreq.Rules`, so a group with only the photo rule on is screened at join with Check on join; it then calls `memberReqEnforce` with `Source: memberReqFromJoin` (`:105-110`), which reaches step 6b. A kick or ban removes (no welcome), a mute holds the welcome, a warn welcomes: all Phase 13. A lookup that is unknown at join welcomes the member. [VERIFIED: `memberreq_join.go:71-121`]

**The held branch gets one addition (the "held release", Conflict C1).** Today the held path is `memberReqDeleteHeld` then return (`:528-531`). With lookup rules a Warn hold (or an admin-unmuted mute hold) whose member later fixes the photo is never released: nobody looks up the member (so no positive is cached), `ClearHoldOnClean` runs only in the watcher's clean path (S-03), and the watcher hands a job to the runner for **every** later message of that member for as long as the hold lives (a hold ends only by leave, rejoin, `ClearHoldOnClean`, or "I fixed it", which exists for mutes only). The fix keeps D-10's intent (the hold is read before any lookup; nothing on this path punishes or claims) and adds, in the held branch **before** `memberReqDeleteHeld`:

```go
// proposed
if memberReqHeldReleased(ctx, b, c, held) { // names clean, every lookup bit of the hold now pass or inert
	return memberReqResult{}
}
```

`memberReqHeldReleased`: (a) only for a message case with a message ID (a join has no message date: `ClearHoldOnClean` with date 0 would never match anyway); (b) only when `held.Rules` has a lookup bit (any table row with a non-nil `Lookup`); (c) reads the settings fresh (`memberReqSettingsLookup`, an error means not released); (d) the names of `c.User` must be clean (`memberReqBroken(settings, names) == 0`: a violation job never spends a lookup on a member whose names still break); (e) for each lookup bit in `held.Rules` whose rule is still on, runs `Lookup` with `memberReqLookupMode{Classify: true}`, and any answer but `Pass` or `Inert` keeps the hold; (f) then `memberreq.ClearHoldOnClean(ctx, chat, user, time.Unix(c.MessageDate, 0))` and reports whether it deleted a row. It **only deletes the hold row; it never lifts a mute** (R2): for a punished mute the SQL deletes the row only when `punished_at` is more than 2 s before the message (`holds.go:179-193`), and a member the bot muted cannot send, so that message proves an admin lifted it. Only the lookup bits of the hold are re-judged: a member whose hold was for the photo and who now also lacks the channel subscription is a **new** episode for the channel at their next message after the hold clears, as in Phase 13 a name rule broken after a fix is.

Cost, stated honestly: a held member who still lacks the photo costs one paced `getUserProfilePhotos` per message (a missing photo is never cached, D-03, D-20), through the singleflight, the pacer and the 32-job runner; the delete of the message follows as before. If the planner rejects the release, the consequence is the perpetual-job pattern above for every released-but-held member; there is no safety consequence.

### S-03. The watcher (question 2)

**Today** [VERIFIED: `memberreq_watch.go:191-231`]: filters (supergroup, item, sender shape, settings with `AnyRuleOn`), then `memberReqBroken(settings, names) == 0` runs `memberReqCleanMember` (pass-key GET, else the clean predicate and a pass-key SET), else a known admin returns and `memberReqRunAsync` starts a job. It makes no Telegram call and returns `ext.ContinueGroups` on every path through a preset named result.

**Extension (still no Telegram call, D-10):**

```
names broken                      -> unchanged (known admin returns, else one job)
names clean, no lookup rule on    -> unchanged (memberReqCleanMember)
names clean, a lookup rule on     -> memberReqLookupCached: are all lookup verdicts cached positive?
     all satisfied                -> memberReqCleanMember (pass hit returns, else clean predicate + pass SET)
     not satisfied                -> known admin returns; approved (or unknown approvals) returns; else ONE job
```

`memberReqLookupCached(ctx, settings, chatID, userID)` is one pipelined round of single-key `GET`s: the pass key (so the common path stays cheap), `fuku:memberreq:photo:<user>` when the photo rule is on, `fuku:memberreq:lc:<chat>` when the channel rule is on; and, only when `lc` holds a channel ID, a second `GET fuku:memberreq:sub:<channel>:<user>`. Photo satisfied is "the key exists"; channel satisfied is "`lc` is `0` (no linked channel: nothing to look up, the rule is inert here) or the `sub` key exists". **Any** Redis error or a missing client is "not satisfied", so Redis down hands a job per message to the runner (documented cost; shed over 32; the pacer falls back to one replica's interval, D-12). Two round trips at most (one with only the photo rule), against one today.

**Proof obligations (tests, S-12):** (1) a clean member with cached positives costs zero jobs (`memberReqJobs` never started) and zero Telegram calls; (2) a miss starts exactly one job; (3) a held member's job reads the hold before any lookup: with a hold whose `rules` has only name bits there is **no** `getUserProfilePhotos` call, with a lookup bit and clean names exactly the release re-check's lookups, with broken names none.

**R2 and the pass cache.** The pass key keeps its Phase 13 meaning: "names and the rule actions are clean and the clean predicate ran" (value `memberReqPassValue`, 30 min, written only for a message not older than 2 minutes, `memberreq_watch.go:42-45,127-146`). It is **never consulted without the lookup positives** (they are checked in the same pipeline and both must hold) and **never stands in for a lookup verdict**, so a pass hit cannot hide a missing photo. The watcher writes nothing about inert or unknown anywhere: it reads `lc == 0` as "nothing to verify" but writes no verdict of any kind; the classifier (S-06) never runs here. A member held only for the channel rule whose lookup is inert is re-judged by the job at their next message (S-02), exactly as R2 says.

**Research addition (not in D-10's text; small; Open Question 3):** on the **not satisfied** path the watcher checks, before it starts a job, `cachedAdminStatus` (existing, `antiflood.go:106-121`) and the approvals tri-state through the existing seam `memberReqApprovedLookup` (`memberreq_enforce.go:111`, a cached read: Redis plus the in-process layer). Without it, an administrator or an approved member without a photo (exempt inside the job, so no positive is ever cached for them) would cost a job per message forever. An approvals error or an approved member returns without a job; nothing is ever punished on that path.

### S-04. The lookup layer: pacer, flights, caches (questions 3 and 4)

New file `fuku/modules/memberreq_lookup.go` (proposed names; the planner may rename).

**Pacer** [model: `memberReqPacer`, `memberreq_enforce.go:32-38`, quoted: `NextKey: "fuku:memberreq:pace:next"`, `BlockKey: "fuku:memberreq:pace:block"`, `Interval: 100 * time.Millisecond`, `MaxRetries: 3`, `MaxWait: 60 * time.Second`]:

```go
// proposed: the options are a package variable so a test can build a second pacer over the same keys
var memberReqLookupPacerOptions = ratelimit.TelegramPacerOptions{
	NextKey:    "fuku:memberreq:lkp:pace:next",
	BlockKey:   "fuku:memberreq:lkp:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 1,
	MaxWait:    5 * time.Second,
}
var memberReqLookupPacer = ratelimit.NewTelegramPacer(memberReqLookupPacerOptions)
```

- **Interval 100 ms** (10 lookups a second across the fleet): the Phase 13 model; the pacers are independent, so staff, lockdown, requirement and lookup bursts add up and could exceed Telegram's overall guidance [ASSUMED: about 30 requests a second per bot; the 20 messages a minute per group limit applies to sends, not to reads]. Alternative (Open Question 5): 200 ms, which halves the worst case at the price of more shed lookups in a raid.
- **MaxRetries 1.** `NewTelegramPacer` replaces a value `<= 0` with 3 (`telegram_pacer.go:106-110`), so 1 is the smallest. After a 429 the pacer records the **full** `retry_after` as the shared block (`telegram_pacer.go:140-160`, "The full retry_after is recorded on purpose") and, for a `retry_after` over `MaxWait`, returns `ErrRateLimited` at once; a block longer than 5 s therefore sheds every lookup of every replica with **no Telegram request and no waiting**, which is what "never hold up the rest of the bot" needs. A slot more than `MaxWait` away is shed the same way without taking the slot (`reserveSlotScript`, `telegram_pacer.go:84-96`).
- **MaxWait 5 s**: at 100 ms a queue of 50 reservations; the 32-job runner plus a few interactive callers can fill about 40, so only a block or a flood sheds.
- `memberReqLookupPaced(ctx, call)` mirrors `memberReqPaced` (`memberreq_enforce.go:51-57`): `memberReqLookupPacer.Do(ctx, func(callCtx) error { bounded, cancel := context.WithTimeout(callCtx, memberReqCallTimeout); defer cancel(); return call(bounded) })`, reusing the 10 s constant `memberReqCallTimeout` (`:41`).
- **Never** route a lookup through `memberReqPaced`, and never a notice, punishment, delete or the live `getChatMember` of the group through the lookup pacer (D-09). `memberReqLiveMember` stays on Phase 13's pacer; a channel needs its own helper that takes a chat ID and goes through the lookup pacer.
- Error shapes: `ratelimit.ErrRateLimited` wraps its cause with `%v`, not `%w` (`fmt.Errorf("%w: %v", ErrRateLimited, err)`, `telegram_pacer.go:157`), so a shed 429 never satisfies `cache.IsBotGoneError`'s `errors.As(*gotgbot.TelegramError)`: a shed is unknown, never inert. [VERIFIED: pacer source and `adminCache.go:83-89`]
- A test installs a fast pacer over the same keys (`withFastLookupPacer(t)`, the `withFastMemberReqPacer` shape, `memberreq_env_test.go:36-50`, `Interval: time.Millisecond`, `RetryAfterUnit: 5 * time.Millisecond`).

**Caches** (all outside the `fuku:cache:` prefix, so `CLEAR_CACHE_ON_STARTUP` keeps them [VERIFIED: AGENTS.md:710 "Operational Redis keys (... `fuku:memberreq:*`) sit outside the `fuku:cache:` prefix"]; all through `memberReqRedisClient()` with `raidRedisTimeout` 250 ms, the pass cache's helper shape, `memberreq_watch.go:111-120`):

| Key | Value | TTL | Written when | Read by |
|-----|-------|-----|--------------|---------|
| `fuku:memberreq:photo:<user>` | `1` | 30 min | a photo lookup found a photo (inside the flight, before it returns) | watcher, job, release |
| `fuku:memberreq:sub:<channel>:<user>` | `1` | 30 min | a member lookup answered subscribed | watcher, job, release |
| `fuku:memberreq:lc:<chat>` | decimal channel ID, or `0` for none | 10 min (inside "about 10 minutes", D-06) | `getChat` answered (a linked ID, or no `linked_chat_id`) | watcher, job, page; **not** written on any error |
| `fuku:memberreq:lkp:pace:next`, `:block` | the pacer's | the pacer's | by `ratelimit` | `ratelimit` |

Positive only: a missing photo, a not-subscribed answer and every unknown are **never** written (D-03, D-12). `lc` caches a definite `getChat` answer both ways (D-06): a `getChat` **error** is not cached. A channel ID is accepted only when it is negative; a non-negative `linked_chat_id` is treated as none.

**"I fixed it" and the screens bypass the read caches and may refresh positives.** `memberReqLookupMode{Fresh: true}` skips the cache `GET`s, makes the live call, writes the positive on a positive answer, and on a **definite negative** deletes the stale positive key (`DEL`), so a member who removed the photo within 30 minutes is not shown as fixed and is not waved through by the watcher afterwards. The channel screen's fresh `getChat` also refreshes `lc`.

**Redis down.** No client or an error is a miss on every read and a silent no-op on every write; the pacer falls back to one replica's interval after a Redis failure (`pacerRedisBackoff`, `telegram_pacer.go:30`). Every message of a member in a group with a lookup rule on then starts a job (S-03), each making its lookups, bounded by the runner's 32, the pacer, and shedding beyond. Correct and slower, as D-12 says; nothing the requirements need for correctness lives only in Redis.

**Flights** (one `singleflight.Group`, `memberReqLookupFlight`): keys `p:<user>`, `s:<channel>:<user>`, `l:<chat>`, `b:<channel>` (the bot's own status). The cache read happens **outside** the flight (cheap, and a `Fresh` caller never takes a cached value); the flight wraps only the paced live call **and the cache write**, so by the time the flight deregisters the positive key exists and a job that starts a moment later hits the cache (this is what makes "100 messages from one user, one lookup" deterministic for a user who has a photo; for a user without one the concurrent jobs share the flight, and later sequential ones look up again by design).

```go
// proposed shape; every identifier new
func memberReqFlight(ctx context.Context, key string, fn func(ctx context.Context) memberReqVerdict) memberReqVerdict {
	ch := memberReqLookupFlight.DoChan(key, func() (any, error) {
		defer func() { _ = recover() }() // a DoChan leader that panics crashes the process; see below
		flightCtx, cancel := context.WithTimeout(memberReqContext(), memberReqLookupFlightTimeout) // 15 s, ended at once by StopMemberReqJobs
		defer cancel()
		return fn(flightCtx), nil
	})
	select {
	case res := <-ch:
		if v, ok := res.Val.(memberReqVerdict); ok {
			return v
		}
		return memberReqUnknown
	case <-ctx.Done(): // the caller's own deadline (a 3 s page open, the 4 s join budget, the job)
		return memberReqUnknown
	}
}
```

Notes the planner must keep: (1) the recover must also log one line with the key family and no ID beyond chat, channel and user (use `error_handling.RecoverFromPanic`'s message style, `AGENTS.md` Go rules); the named-result trick is not needed because `fn`'s zero value is `memberReqUnknown`. (2) [VERIFIED: `golang.org/x/sync@v0.23.0/singleflight/singleflight.go` `doCall`] with a `DoChan` caller present a panic in `fn` is re-raised as `go panic(e); select {}`, which "cannot be recovered" and kills the process; with `Do` it is re-raised in each caller. (3) Deriving the flight context from `memberReqContext()` and not from the caller is what lets a short-lived page-open deadline coexist with job followers, and what delivers D-11 for the join entry and the press (which are not jobs). (4) A flight goroutine ends within `memberReqLookupFlightTimeout` or at once on `StopMemberReqJobs`; it is one goroutine per **distinct key in flight**, not a pool (D-10), bounded by the runner's 32 jobs plus the dispatcher's callers.

**Concurrency inside the runner.** No new pool: a job calls the lookups sequentially (photo, then channel), each lookup blocking that job on the pacer or the call. The runner's cap (`memberReqMaxInFlight = 32`, `memberreq_watch.go:54`) therefore bounds in-flight lookups to at most 32 flights plus interactive callers, and a full runner sheds the job with one log line (`:165-168`), the member's next message retrying.

### S-05. Shutdown, recomputed (question 3)

**No new drain; the budget does not move.** Facts [VERIFIED: `memberreq_watch.go:233-258`, `main.go:221-226`, AGENTS.md:47-57]:

- `StopMemberReqJobs` sets `memberReqDraining`, calls `memberReqContext()`, cancels the **root** context, then waits at most `memberReqStopWait = 2 * time.Second` for `memberReqJobs`. It is registered with `memberReqStopWait+shutdownDrainGrace` (3 s) after the DB close and right before the staff drain.
- A job's context is `context.WithTimeout(memberReqContext(), memberReqJobTimeout)` (`:176`), a child of the root, so the cancel reaches every wait of a job at once: the pacer's `wait` returns `ctx.Err()` from its `select` (`telegram_pacer.go:177-187`), a paced call's HTTP request is aborted through its context, and `select` on the caller's deadline in `memberReqFlight` returns unknown. A job in a lookup therefore ends in milliseconds, inside the 2 s wait. The flight goroutine derives from the same root (S-04), so it ends too.
- Lookups read Telegram and Redis only, never PostgreSQL. The one database statement near a lookup is the held release's `ClearHoldOnClean`, which runs on the job's context (cancelled at shutdown: it returns an error, logged, nothing deleted) inside the existing `memberReqJobs` count, so the DB close (registered earlier, so it runs later in LIFO) can never overtake it.
- The webhook-mode worst case is unchanged: HTTP stop 10, AI spam 10, raid AI 2, raid posts 3, lockdown 6, staff 31, member requirements 3 = 65 s against the 60 s budget, `stop_grace_period: 75s` before SIGKILL (AGENTS.md:50-54). Nothing in this phase adds a drain, a wait or a second.

**What a lookup cut off leaves behind: nothing.** The claim comes after every lookup (S-02 step order); a lookup that did not finish returns unknown, which is "not broken"; no hold, no notice, no write. If the cancel lands after the lookups but before the claim, `memberReqTakeHold` runs on the cancelled context and fails (`memberreq_enforce.go:291-295`), leaving nothing; if it lands after the claim the Phase 13 recovery applies unchanged (a pending hold the two-minute reclaim retries, T-13-57). The join and "I fixed it" run on update goroutines that the HTTP and dispatcher stop already drain; their lookups end at the root cancel too.

### S-06. The M13 inert classifier (question 5)

One function maps the bot's own `getChatMember(channel, bot)` answer to {administrator, not administrator, unknown}:

| Answer | Class | Why |
|--------|-------|-----|
| status `administrator` or `creator` | administrator | the bot can read members |
| status `member`, `restricted`, `left`, `kicked` (a definite non-administrator status) | not administrator | cannot read members (a bot cannot be a plain channel subscriber [ASSUMED]; a status other than administrator is a definite "no") |
| an error for which `cache.IsBotGoneError(err)` holds | not administrator | `return tgErr.Code == 400 \|\| tgErr.Code == 403` [VERIFIED: `adminCache.go:83-89`]: **any** 400 or 403 (R3), which errs only toward inert |
| 429 (also the pacer's shed `ErrRateLimited`), any 5xx, a transport error, a timeout, a cancelled context, a nil answer, an unrecognised status (`ChatMemberUnknown`, merged `Status == ""`) | unknown | says nothing about the channel |

It is **by answer and by code, never by `chat_status.IsBotMissingError`** (D-07: its markers are `"bot was kicked", "bot is not a member", "chat not found"`, `owner.go:43`, wording-based, and it reads "chat not found" as the bot being missing, wrong for a channel).

**Where it runs and where it does not:**

| Place | Runs? | Used for |
|-------|-------|----------|
| the page and the channel rule screen (S-09) | yes, one paced call per draw | the `notadmin` line, or `unknown` |
| "I fixed it" step 5 (S-07) | yes, only when the member's own channel lookup was unanswerable | not administrator makes the channel rule **inert** (not broken, no answer of its own); administrator or unknown makes `mreq_fixed_check_failed` |
| the job's held release (S-02) | yes, only when the member's channel lookup failed with an error (`Classify: true`) | not administrator is inert: a member held only for the channel rule is released instead of kept by a channel the bot can no longer read |
| the job's **punishing** path (step 6b) | **no** | an unknown and an inert member lookup are both "not broken" there, so the extra call would change nothing; saving it also avoids a second paced call per shed lookup |
| the join entry's call | no (same reason as the job's punishing path) | |
| the watcher | **never** (R2, D-10) | |

**The guarantee, in one sentence:** the classifier's only outputs are verdicts that count as *not broken* (inert) or *do not decide* (unknown); no code path can turn any classifier result into a breach, so it can only turn a rule into not-broken, never into a punishment. An inert verdict is never written to any cache; the only `lc` write is the definite `getChat` answer, and `lc == 0` is read by the watcher as "nothing to verify", which writes nothing.

### S-07. "I fixed it": step 4b and the fresh lookups (question 5)

**Today** [VERIFIED: `memberreq_fixed.go:119-190`]: `memberReqFixedRun` runs: hold by ID (`:125-132`), live `getChatMember` that must carry the marker (`:134-141`), settings read and `memberReqBroken` on the live names (`:145-155`), `memberReqUnmutePermissions` (`:157-161`), `memberReqFixedTake` (`:163-170`), `memberReqLiftMute` (`:172-176`), the notice edit and the held welcome (`:178-189`). The whole press runs under `memberReqFixedTimeout = 10 * time.Second` (`:29`).

**New order (between lines 141 and 143, nothing else reordered):**

1. **Step 4b, approvals.** `approved, err := memberReqApprovedLookup(ctx, chat.Id, userID)` (the existing seam, `approvals.IsUserApprovedContext`, `memberreq_enforce.go:111`). An error is `mreq_fixed_check_failed`, nothing changed. **Approved: the member is judged clean for every rule, name rules included; no settings read, no lookup; continue at the permissions step.** Not approved: step 5. No new key (M11).
2. **Step 5, judge every enabled rule.** Read the settings (an error is `mreq_fixed_check_failed`); `memberReqBroken` on the live names as before; then `memberReqLookupBroken(lookupCtx, b, settings, chat.Id, userID, memberReqLookupMode{Fresh: true, Classify: true})` under `lookupCtx` = `context.WithTimeout(ctx, memberReqFixedLookupBudget)` with `memberReqFixedLookupBudget = 5 * time.Second` (a variable). `Fresh` bypasses the photo, `sub` and `lc` read caches (D-12 names the first two; reading `lc` fresh costs at most one `getChat` and is right for a rare press), writes a positive, and deletes a stale positive on a definite negative (S-04). `Classify` runs the M13 read only when the member's channel lookup was unanswerable.
3. **Which answer wins.** Recommended: any broken rule (names or lookups) is the definite answer `mreq_fixed_still_broken` (one to four phrases) or `mreq_fixed_still_broken_many` (five or more); only with nothing broken does an unknown lookup become `mreq_fixed_check_failed`. Both change nothing; the first tells the member something true and actionable even when another lookup was shed. (UI-SPEC lists the unknown case first without ordering them; Open Question 6.)
4. A channel rule that is on with no linked channel found, or inert by the classifier, is not broken and has no answer of its own.

**Budget, counted.** Worst case of one press: Phase 13's own calls are the live `getChatMember` (step 4, `memberReqPaced`), `getChat` for the permissions (`memberReqUnmutePermissions`), `restrictChatMember` (the lift) and the notice edit, 4 calls. Phase 14 adds, inside the 5 s lookup sub-budget: photo 1, linked channel `getChat` 1 (fresh), member `getChatMember(channel, user)` 1, and only when that was unanswerable the bot's own `getChatMember(channel, bot)` 1, so **at most 4 paced lookup calls**, sequential. Typical latency is under a second at a 100 ms pacer interval; the sub-budget leaves at least 5 s of the 10 s for steps 6 to 10, and a lookup that exhausts its 5 s is unknown (answer `mreq_fixed_check_failed`, nothing changed, the hold untouched). The planner may run photo and channel in parallel inside the press; if it does, each goroutine starts with `defer error_handling.RecoverFromPanic(...)` and the answer is still given once (the callback already defers an empty answer after its recover, `memberreq_fixed.go:60-76`).

**The inert case and the stuck member.** Documented fallback (P14-A9): a requirement mute placed while the bot could read the channel, with every later lookup unknown and no admin acting, keeps the member under the 330-day marker mute until an admin frees them (`/unmute`) or they rejoin (the rejoin reset). An inert channel (no linked channel found, bot not administrator or gone) is not broken, so such a member **is** freed at the tap.

**Language.** Unchanged from Phase 13: `ctxTr` in a group callback is the group's language (Phase 13 research finding 1); the answers and the notice edit are in the group's language.

### S-08. Data (question 6)

**Migration** `migrations/20261108120000_add_member_requirement_lookup_rules.sql` (any name above `20261106120000` sorts after it; applying the `20261108...` prefix assumes no other pending migration; read `ls migrations | tail` when the plan starts). Precedent, quoted verbatim [VERIFIED: `migrations/20261029120000*.sql` last line]:

```sql
ALTER TABLE chat_raid_detection ADD COLUMN IF NOT EXISTS ai_threshold INTEGER NOT NULL DEFAULT 5 CONSTRAINT chk_chat_raid_detection_ai CHECK (ai_threshold BETWEEN 3 AND 500);
```

Proposed file body (comment header in the Phase 12/13 style):

```sql
ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS photo_action SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_photo CHECK (photo_action BETWEEN 0 AND 4);
ALTER TABLE chat_requirements ADD COLUMN IF NOT EXISTS channel_action SMALLINT NOT NULL DEFAULT 0 CONSTRAINT chk_chat_requirements_channel CHECK (channel_action BETWEEN 0 AND 4);
```

- **Inline constraint, not `NOT VALID`.** The column is new and every existing row takes the constant default 0, which satisfies the CHECK; the table has one row per configured chat (small). Phase 8's `NOT VALID` existed because it swapped a constraint on populated data (migration `20261030120000`); Phase 13's tables and Phase 6's `ai_threshold` use inline constraints, which is the precedent that fits. `ADD COLUMN IF NOT EXISTS` skips the whole column clause, constraint included, when the column exists, which is what makes the statement idempotent (PostgreSQL has no `ADD CONSTRAINT IF NOT EXISTS`).
- **Both appliers agree with no applier change.** The Go runner rewrites only a statement matching `alter table X add constraint NAME ...` (`runner.go:536`: `(?is)alter\s+table\s+(?:only\s+)?(...)\s+add\s+constraint\s+...`), and `scripts/migrate_psql.sh:75` rewrites only `^ALTER\s+TABLE\s+(\S+)\s+ADD\s+CONSTRAINT\s+(\S+)`; `ADD COLUMN ... CONSTRAINT` matches neither, so both run the file unchanged (as 20261029120000 did). No top-level `BEGIN`/`COMMIT` (`isTransactionControlStatement`, `runner.go:361`), one transaction per file.
- A row written before the migration reads `photo_action = 0` and `channel_action = 0` (off): the safe direction. A cached `member_requirements` value written by the previous build decodes the two fields as zero (msgpack ignores nothing missing), also off. An old-build replica that reads a new row ignores the columns it does not know and does not screen for the new rules (rolling-deploy caveat, accepted like the others in AGENTS.md).

**Model** (`fuku/db/models/member_requirements.go`, existing style quoted, `:28-31`): `RussianAction  int \`gorm:"column:russian_action;type:smallint;not null;check:chk_chat_requirements_russian,russian_action BETWEEN 0 AND 4" json:"russian_action"\``. Add `PhotoAction` (`column:photo_action`, check name `chk_chat_requirements_photo`) and `ChannelAction` (`column:channel_action`, `chk_chat_requirements_channel`), no `default:` tag, after `ChineseAction`. GORM builds the CHECK on SQLite from the tag, so the SQLite tests see it too (AGENTS.md Testing).

**Repository** (`fuku/db/memberreq/settings.go`): add `RulePhoto Rule = "photo"` and `RuleChannel Rule = "channel"` to the const block (`:21-29`), append both to `Rules` (`:32`, which is "every rule in the order the settings page shows them" and must equal the table order: `TestMemberReqRuleTable` asserts `rule.Store == memberreq.Rules[i]`, `:320`), add the two `ruleColumns` entries (`:55-61`: `"photo_action"`, `"channel_action"`) and two cases to `actionField` (`:65-81`). `SetRuleAction` (`:139-150`), `AnyRuleOn`, `GetSettings` and `GetSettingsFresh` are generic over those and need no change; `upsertSettings` still writes one column plus `updated_at` and calls `cache.DeleteCache`. Column naming is uniform (`<rule>_action`), so the `the CHECKs` subtest of `TestChatRequirementsSettings`, which executes `UPDATE chat_requirements SET %s_action = ?` for every `Rules` entry (`settings_test.go:198`), covers both new columns the moment the enum has them; its `[5]int` helpers (`actionsOf` at `:59`, the `want [5]int` literals at `:80,104,141,171,257`) become `[7]int`.

**Harness and backup.** `models.ChatRequirements` is already in every AutoMigrate list that needs it (`fuku/modules/test_harness_test.go:378`, `fuku/db/memberreq/testmain_test.go`, the backup testmain), and the new columns arrive from the model tags, so **no list changes** (the brief's "harness AutoMigrate lists" resolves to "no edit; verified by reading them"). Backup exclusion is unchanged: `AllExportableModules` is an explicit list without the table, proven by `TestMemberRequirementsStayOutOfBackup` (`fuku/db/backup/memberreq_backup_test.go`), which stays as it is.

**PostgreSQL integrity list.** `TestChatRequirementsSettings` and `./fuku/db/memberreq` are already in `Makefile:39-44`; extend the test (the two CHECKs reject 5 and -1 with SQLSTATE 23514, a column-targeted upsert of photo and channel at once never clobbers each other or the five, a Go 0 is written, and a row inserted with only `chat_id` reads both columns 0). Add `TestMemberRequirementLookupMigrationDefaults` (PostgreSQL only, the `TestRulesMediaMigrationDefaults` shape: the statements run twice without error inside a rolled-back transaction, an `INSERT INTO chat_requirements (chat_id)` reads both columns 0, both named constraints exist in `pg_constraint`, a direct insert of 5 fails with the named check) in `fuku/db/migrations` (package already in the list) and append its name to the anchored `-run` regex. Its database-free half, `TestMemberRequirementLookupMigrationStatements` (two statements, `ADD COLUMN IF NOT EXISTS`, the two named constraints inline, no `BEGIN`/`COMMIT`/`CONCURRENTLY`, a name that sorts after `memberRequirementsMigration`), runs in `make test` beside the existing `TestMemberRequirementsMigrationStatements` (`member_requirements_migration_test.go:42`).

### S-09. The settings page (question 7)

**Everything the table derives** [VERIFIED: `settings_mreq.go`]: the rule controls are registered by `for _, rule := range memberReqRules` (`:46-52`: `Right: settingsRightRestrict, Actions: []string{settingsActSet}, Validate: settingsMreqActionValue`), `mo` validates `t` through `memberReqRuleByCode` (`:276-279`), the grid is `settingsMreqRuleGrid` (`:229-242`, `cells[i:min(i+2, len(cells))]`, which yields `[ln,un]`, `[ru,ar]`, `[zh,ph]`, `[ch]` with no change), the body loops the table (`:206`), the rule screen reads `rule.LineKey` and `rule.DescKey` (`:291`), `settingsMreqMainState` counts table rows on (`:128-132`, `n` up to 7). **`ph` and `ch` clash with nothing**: page `m` controls are `ln un ru ar zh j d`, page actions `mo my`, core actions `settingsCoreActions` (`oh op bk cl tg st no pk`); `registerSettingsPage` panics on a duplicate. `settings_pages.go` is not edited.

**New code, minimal:**

1. **A status value** `settingsMreqStatus{Basic bool; Channel settingsMreqChannelState}` with states `unread` (the zero value: no line, the Phase 13 shape), `ok`, `notadmin`, `nolink`, `unknown`. Keep `renderSettingsMreqPage(v, row, notice)` and `renderSettingsMreqRuleScreen(v, rule, row, notice)` as thin wrappers over new `...Status(v, ..., status)` renderers, so the many direct callers in tests (`settings_mreq_test.go:675,682,770,813,1163,1178`, `settings_staff_privacy_test.go:201-205`) keep compiling and keep drawing the Phase 13 shape for the zero status.
2. **The page body's cannot-work block** is a `[]string` of lines (UI-SPEC "Notes for the planner"), preceded by one blank line only when non-empty, between the switch lines and the hints (`settingsMreqBody`, `:213-224`), so Phase 15 appends `settings_mreq_here_ai` with no renderer change. Lines: `🚫 ` + `settings_mreq_here_basic` while `Basic` and any rule is on; else, in a supergroup while the channel rule is on, `🚫 `/`❔ ` + the `nolink`, `notadmin` or `unknown` key with `{rule}` = `settings_mreq_rule_channel` (icons are code-side prefixes, never in a template). New hint `settings_mreq_hint_lookup` after the blank-name hint.
3. **The rule screen's status slot** (one line between the description and the blank before "Punishment:"): the basic line on every rule; on `ch` the line for its state; `ph` needs no Telegram call. **M7:** for `ch` with `Basic` or `nolink`, the choice rows are `[Off]` alone and the two hints (`settings_mreq_choose_hint`, `settings_mreq_actions_hint`) are omitted (two rows, not three); `settingsChoiceRow` (`settings_pages.go:314-332`) already builds a row from a slice of options, so this is a one-option slice. `settings_mreq_hint_photo` follows the photo description (the blank-name hint's slot, `:292-294`, generalised to "a rule may carry a hint key").
4. **One status reader** `settingsMreqReadStatus(p *settingsPress, row, forChannelScreen bool) settingsMreqStatus`: `Basic` from `p.group.Type == "group"` (type is available on both menus: the message's chat for a group menu, `full.ToChat()` of the live `getChat` for a private one, `settings_menu.go:433-447`); returns with **zero calls** for a basic group, for the channel rule Off on the page, and for the photo screen. Otherwise, under `context.WithTimeout(context.Background(), settingsMreqHereTimeout)` (`3 * time.Second`, the owner-state lookup's bound, `settings_gate.go:78`) and **only through the lookup pacer**: the channel screen makes one fresh `getChat(group)` (refreshing `lc`), the page reads `lc` and calls `getChat` only on a miss; then, only when a channel is linked, one `getChatMember(channel, bot)` classified by S-06. At most 2 calls per draw (the table in UI-SPEC section 2). A private press already makes one `getChat` whose `LinkedChatId` `ToChat()` drops; reusing it would need a new field on `settingsPress` and an edit of `settings_menu.go`. **Recommendation: do not reuse** (one code path for both menus, the press pipeline untouched); the UI contract says the planner "may".
5. **Every draw reads the status.** The page is drawn by `settingsMreqOpen` (`:141-147`) and by `settingsMreqSetSwitch` (`:173-185`, after a switch press); the rule screen by `settingsMreqOpenRule` (`:327-341`) and `settingsMreqRedrawRule` (`:352-355`, reached from `settingsMreqChoose`, `settingsMreqYes` and `settingsMreqApplyRule`). Route all four through one helper so a redraw never drops the line. Order in each: read the database first (an error is the cannot-open edit), then `p.answer("", false)` (the Staff Group page's order, `settings_staff.go:294`: `p.answer` is idempotent through `p.answered`), then the status read, then the edit. `settingsMreqMainState` stays database-only.
6. **The refused punishing press** (UI-SPEC section 5) is one gate function `settingsMreqChannelGate(p, rule, action) (screen *settingsScreen, proceed bool)` called by `settingsMreqChoose` after the fresh same-value read (`:364-376`) and before the card or the write (`:377-382`), and by `settingsMreqYes` after the expiry check (`:433-441`) and before `settingsMreqApplyRule` (`:442`). It is a no-op for `ph`, for `v == 0` and for a press of the stored value. For `ch` with `v` 1..4: one fresh `getChat(group)` through the lookup pacer bounded by `settingsMreqHereTimeout`, **before** the callback is answered (the alert must be the one answer): a basic group or a `getChat` that carries no linked channel answers the alert `settings_mreq_here_nolink` (the status line's own key, `{rule}` filled, no icon), writes nothing, draws no card, and redraws the Off-only channel screen; an unreadable link (error, timeout, shed) answers `mreq_fixed_check_failed`, writes nothing and edits nothing (P14-A4: fail closed); linked proceeds as Phase 13. The bot not being an administrator of the channel **never refuses** a press (D-08).
7. **Callback bytes do not change** (`ph` and `ch` are two letters like `ln`): settings buttons stay at most 53 bytes, "I fixed it" 52 (`mreq|v1|a=f&h=<hold>&u=<user>`).

**Call-count assertions** the tests make (a counting fake, S-12): Main 0; photo screen 0; page with the channel rule Off 0; basic group 0; page with `lc` cached `0` 0; page with `lc` cached linked 1; page on a cache miss at most 2; channel screen at most 2 and the cache refreshed; none through `memberReqPacer`; a pacer that refuses draws `❔` and nothing else; a draw never exceeds 3 s.

### S-10. What else changes: locale keys, i18n tests, existing tests, docs (question 8)

**Locale keys.** 16 new keys (12 `settings_`, 4 `mreq_`; totals 235 to 247 and 25 to 29) and 7 reworded keys, all in 8 files; the exact keys, English, ceilings and the native-read list are in 14-UI-SPEC.md "Copywriting Contract" and are not repeated. Counts re-verified this session: `grep -cE '^settings_' locales/en.yml` = 235 and `grep -cE '^mreq_' locales/en.yml` = 25; the seven M12 keys currently say "profile" in `en.yml` lines 3209, 3213, 3215, 3217, 3218, 3221, 3244.

**Tests in `fuku/i18n` that must change (by name):**

| Test or constant | File | Change |
|------------------|------|--------|
| `memberReqLocaleLimits` | `memberreq_locale_test.go:27-53` | +4 rows (`mreq_rule_photo` {26, 32}, `mreq_rule_channel` {30, 38}, `mreq_remedy_photo` {62, 84}, `mreq_fixed_still_broken_many` {80, 112}); still covers exactly the `mreq_` keys |
| `memberReqRulePhrases` | `:55-58` | seven phrases in rule order |
| `memberReqAnswerKeys` | `:62-65` | + `mreq_fixed_still_broken_many` (template at most 160) |
| `memberReqNoticeMax` and the comment above `memberReqNoticeKeys` ("at most 400 code points") | `:68,79` | 400 to 520 (M10); the failure message that prints the ceiling follows; grep `400` in `memberreq_locale_test.go`, `memberreq_*_test.go` and `TestMemberReqScreensFitEveryLocale` |
| `memberReqRender`, `memberReqRenderedFailures` | `:100-120`, `:201-241` | join all seven phrases; the still-broken render uses the **four longest** phrases (160 EN, 200 other, `memberReqStillBrokenEn/Other`, `:77-78`); a `_many` render at `n` 99 (cap 160); the three notices that carry the tail (`mreq_notice_warn`, `_kick`, `_mute`) append `mreq_remedy_photo` after one space |
| `TestSettingsLocaleKeys` limits table, `TestSettingsLabelLengths` (`mreqRuleCells`, `gridMax = 20`), `TestSettingsHelpFitsOneMessage` | `settings_locale_test.go` | +12 rows covering exactly the 247 `settings_` keys; seven cells each rendered with ❌ ❗ 👢 ⛔ 🔇; help text unchanged |
| the quote-consistency check | `memberReqCrossFamilyFailures` (`memberreq_locale_test.go`) | unchanged: the button words inside the quotes of `mreq_notice_mute` and `settings_mreq_hint_mute` do not change |
| **new** `TestMemberReqRuleKeysExist` (recommended) | `fuku/modules` or `fuku/i18n` | every `LineKey`, `CellKey`, `DescKey`, `PhraseKey` of the table exists in `en.yml` and is non-empty: `make check-translations` only sees literal `GetString` arguments, and the table keys are not literals (a typo ships an empty message) |

**Existing `fuku/modules` and `fuku/db` tests that change on purpose, or break if forgotten** (none re-recorded; each is edited to the new truth):

| Test | File | Why |
|------|------|-----|
| `TestMemberReqRuleTable` | `memberreq_rules_test.go:295-325` | 7 codes `ln un ru ar zh ph ch`, `Bit == 1<<i` holds, exactly one of `Broken`/`Lookup` |
| `TestMemberReqStrictest` | `:376-396` | `allBits` must be computed over name rules for the `memberReqBroken` comparison; add lookup-bit cases to `memberReqStrictest` |
| `requirementsOf(actions ...int)` | `:346-354` | five actions; extend to seven (or add `requirementsOf7`) |
| `TestMemberReqPassValue` | `:468` | a photo or channel action change changes the value |
| `TestMemberReqScreensFitEveryLocale` | `memberreq_screens_test.go:37,107,114` | loops `memberReqRules`; line 107 builds `mreq_fixed_still_broken` with **all** rules, which with seven phrases exceeds 200 in some languages: it must use the real count-aware builder |
| `settingsMreqWantSwitchRow`, `settingsMreqWantGrid`, `settingsMreqRuleCodes`, `settingsMreqCellKeys`, `settingsMreqRuleKeys` | `settings_mreq_test.go:29-33,131-176,179-200` | 6 rows, switch row index 4, navigation index 5, grid widths `{2, 2, 2, 1}`, seven codes and keys |
| `TestSettingsMreqRowBudget`, `TestSettingsMreqCallbackBytes` | `:774,784,811,1164` | 6 rows; `InlineKeyboard[:4]` for the cells; `InlineKeyboard[4]` for the switches; `want 5` becomes `want 6` |
| `[5]int` helpers (`settingsMreqSeed`, `settingsMreqStoredActions`, call sites) | `settings_mreq_test.go:36-44,493,503,960,1029`, `settings_mreq_card_test.go:70-75,174` | seven actions |
| `TestSettingsStaffPrivacy` (page `m` part) | `settings_staff_privacy_test.go:192-215,274-284` | `everyPunishment` gets the two new actions; add the channel statuses and `ph`/`ch` presses |
| `TestSettingsControlActionAllowList`, `TestSettingsMreqRuleScreen`, `TestSettingsMreqCard`, `TestSettingsDriftMreq`, `TestSettingsAuthorityMreq` | `settings_mreq_test.go`, `settings_mreq_card_test.go` | iterate the rule codes: extend to `ph`, `ch` |
| `TestMemberReqFixedUnmutes`, `TestMemberReqFixedRefusals` | `memberreq_fixed_test.go` | UI-SPEC says "use a member who is not approved"; they approve nobody today, so likely no edit (verify) |
| `TestChatRequirementsSettings` | `fuku/db/memberreq/settings_test.go:67` | seven columns |
| `TestMemberRequirementsMigrationStatements` | `fuku/db/migrations/member_requirements_migration_test.go:42` | unchanged (it pins the old file); the new file gets its own test |

**Docs and AGENTS.md that become false or incomplete** (updated in the same commits as the code):

- `AGENTS.md`: Startup and shutdown (the member-requirements drain bullet, lines 44-57: add that lookups derive from `memberReqContext()`, no new drain, the 65 s worst case unchanged); Handlers (the watcher bullet, line 137: "the pure name rules" gains the cached-positive read and the not-satisfied job); Callbacks (the `mreq|v1` bullet, lines 311-340: step 4b and the wider step 5; the page `m` bullet, lines 485-523: seven cells, six rows, `ph` and `ch`, the status block, the Off-only channel screen, the refused press order); Permissions (line 641 "I fixed it" bullet: approved members are freed; line 694: rights of the new screens); Data (line 710 family list unchanged; new bullets for `fuku:memberreq:photo:*`, `:sub:*`, `:lc:*`, `:lkp:pace:*`; the `chat_requirements` bullet line 1112: "five `SMALLINT` action columns" becomes seven, migration name, and "Phases 14 and 15 add their own rule columns" now true for 14); Subsystem traps (the executor bullet, line 1217: step 6b, the decoupling decision and the held release; the notice tail and the count answer; the classifier); Testing (the fake wrapper, `withFastLookupPacer`, the new families, the PostgreSQL list and the locale counts 247 and 29).
- `docs/src/content/docs/api-reference/callbacks.md`: the `mreq` section (lines 194-206: step 4b, fresh lookups) and the page `m` table (lines 275-285: the rule codes `ph` and `ch`, the refused press); `permissions.md` lines 439-441 (rule screens, "I fixed it" freeing an approved member); `README.md:386` ("five rules", "Supergroups only": add the two lookup rules and what they need); `docs/.../commands/settings/index.md:13` needs no edit. Run `make generate-docs`, `make check-docs`, `make check-translations`.

### S-11. The three spikes and what the code does under each outcome (question 9)

core.telegram.org is unreachable from this session (the same `ENOTFOUND` as Phase 13); two WebSearch queries returned nothing authoritative ("closest related material is in the Business Account documentation ... a main profile photo can be hidden by a business account's privacy settings"; "the likely explanation for CHAT_ADMIN_REQUIRED is that the bot isn't an administrator", an inference). The gotgbot doc strings, which are generated from the Bot API text, are the only primary text and say only "The method is only guaranteed to work for other users if the bot is an administrator in the chat". All three are therefore **deferred live checks for `14-SPIKE.md`** (owner's standing choice), each non-blocking, each with Phase 13's warning that nothing punishes on an unknown.

| Spike | Question | Outcome A | Outcome B | What the code does, and why it stays safe |
|-------|----------|-----------|-----------|--------------------------------------------|
| **S1** (MEDIUM, community reports only) | `getUserProfilePhotos(user, 1)` for a user whose privacy hides the photo from the bot | `total_count` 0 | an error (400 or other) | A: breach, which is the intended rule (D-01) and what the page says ("counts as missing"). B: unknown, the member is never punished; then `settings_mreq_desc_photo` and `mreq_remedy_photo` must be reworded in place to say a hidden photo is not checked (no key added, P14-A14 as corrected by R1). Zero photos with a non-empty `Photos`, or a nil answer, are handled: breach needs `TotalCount == 0 && len(Photos) == 0`; nil is unknown |
| **S2** (LOW-MEDIUM) | `getChatMember(channel, user)` and `getChatMember(channel, bot)` when the bot is not the channel's administrator, or was removed | an error, expected 400 or 403 (a bot can only be a channel administrator, so "not an administrator" means "not in the channel") | a **status** answer (a plain member or `left` for the bot) | A: the member lookup is an error, so unknown, never a breach; the bot lookup is `IsBotGoneError`, so not administrator, so the page shows `notadmin` and "I fixed it" treats the channel as inert. B (not expected): the bot's own status is `member` or `left`, a definite non-administrator, so inert; but a status answer to a **member** lookup from a non-administrator bot would be trusted as a breach if it said `left`. That is the one place a false breach could arise; the hardening (Open Question 2) is a `memberReqBotChannelStatus == administrator` precondition on a `left`/`kicked` verdict, one extra paced call per channel breach (not per message), and it can only turn a breach into not-broken, so it is consistent with M13. Recommendation: do not add it now; add it only if S2 shows a status answer to a non-administrator bot |
| **S3** (MEDIUM) | does `getChat(group).linked_chat_id` come back for a supergroup where the bot is a plain member | yes | only for administrators | A: as designed. B: the group reads "no linked channel", `lc` caches `0`, the channel rule is inert in that group (never punishes), the page says "I found no linked channel for this group" (the key's own wording, chosen for exactly this, P14-A15) and a punishing press is refused (S-09 item 6). This only affects groups where the bot is not even an administrator of the **group**, which cannot restrict, kick or ban there anyway (Phase 13), so the cost is nil where punishment is possible. If B, reword `settings_mreq_here_nolink` in place to ask for administrator rights in the group; no key added |

Additional live checks the plans should list in `14-SPIKE.md` (beyond D-19's two and the UI contract's S3): (4) a user with a "public photo" fallback while the main photo is hidden: does `getUserProfilePhotos` show it (LOW; affects only wording); (5) the exact wording and code of the S2 errors, to decide whether `cache.IsBotGoneError` (any 400/403, R3) can be narrowed; (6) a subscribed member reads `member`, a member who left the channel reads `left`, one banned from the channel reads `kicked`; (7) the phone-width read of the two new cells in hi, ru, uk.

### S-12. The Phase 13 guarantees that must keep holding, and how Phase 14 keeps them (question 10)

| Phase 13 guarantee (source) | Why Phase 14 keeps it | Re-asserted by (new test names) |
|-----------------------------|-----------------------|---------------------------------|
| One punishment per episode across replicas (T-13-01, T-13-21) | lookups run before the claim; the claim is still `ClaimHold` insert-first; a lookup is a read | `TestMemberReqLookupOnePunishmentAcrossReplicas` (Mute or Kick, **not Warn**: on SQLite concurrent warn writers fail with "database is locked", the lesson of `TestMemberReqOnePunishmentAcrossReplicas`) |
| Unknown never punishes (MREQ-10, T-13-19/20) | the verdict's zero value is unknown; unknown and inert are not broken; the live admin check precedes the lookups | `TestMemberReqLookupUnanswerable` matrix (below) |
| Never mute over someone else's restriction (CR-01) | `memberReqMuteOverRestriction` still runs after step 6b | `TestMemberReqLookupNeverReplacesRestriction` (photo rule Mute on a member restricted below the defaults) |
| Cap and pause (T-13-35) | the cap is still counted after the lookups and before the claim; a pause (step 3) precedes the lookups | `TestMemberReqLookupCapAndPause` (a pause makes no lookup call) |
| Marker ownership, "I fixed it" never lifts another mute (T-13-47) | step 4 (the live marker) precedes 4b and 5 | `TestMemberReqFixedNeverLifts` re-run with the photo rule broken; `TestMemberReqFixedApproved` |
| Held member takes the held path (T-13-21) | the hold is read before `memberReqLiveViolation`; the release re-check is lookup-bit-only, names-clean-only | `TestMemberReqLookupHeldPath`, `TestMemberReqHeldRelease` |
| Watcher returns `ext.ContinueGroups` on every path, no Telegram call for a clean member (T-13-30) | the new reads are Redis only | `TestMemberReqWatcherContinuesGroups` re-run with both rules on; `TestMemberReqWatcherLookupRules` |
| No drain growth, jobs end at shutdown (T-13-34, T-13-57) | S-05 | `TestShutdownEndsMemberReqLookups`, `TestMemberReqRunnerSheds` unchanged |
| Settings page privacy and fit (T-13-52) | status lines are words | `TestSettingsStaffPrivacy` over every new screen and every channel status |
| Names never reach logs; Telegram errors by code only (T-13-23) | every new log line carries chat, channel, user IDs and `memberReqErrText` | `TestMemberReqLookupLogsCarryIDsOnly` |

## Conflicts between CONTEXT, UI-SPEC and the code that the planner must resolve

| # | Conflict | Evidence | Recommendation |
|---|----------|----------|----------------|
| **C1** | **D-10 "a held member's messages take the held path before any lookup" vs. releasing a hold.** Taken literally, a Warn hold (or an admin-unmuted mute hold) with a lookup bit is never released after the member fixes the photo, no positive is ever cached for them, and the watcher starts a job for every message they send for as long as the hold lives. UI-SPEC R2 says the opposite assumption: "a member held only for the channel rule whose lookup is inert is re-judged by the job at their next message" | `memberreq_enforce.go:528-531` (held branch has no lookup), `holds.go:172-194` (`ClearHoldOnClean` runs only from the watcher's clean path), `memberreq_watch.go:149-155` | The **held release** of S-02: the hold is still read first and nothing on the path punishes; a lookup-bit-only, names-clean-only re-check may end in `ClearHoldOnClean`. Needs the planner's explicit "yes" because it adds lookups on a held path |
| **C2** | UI-SPEC M13 / P14-A5 say the one classifier is used by "the executor's inert decision, the clean predicate"; R2 says the clean predicate (watcher) never classifies | UI-SPEC "Notes for the planner", R2 | The punishing path never needs it (unknown and inert are both "not broken"); it runs in the held release, "I fixed it" and the screens only (S-06 table) |
| **C3** | UI-SPEC step 4b says "read approvals **fresh**" and names `approvals.IsUserApprovedContext`, which reads the **cached** list (`memberReqApprovedLookup`, `memberreq_enforce.go:111`; `approvals` is not among the `skipLocal` prefixes, `fuku/db/cache/local.go:73-79`) | `fuku/db/approvals/repository.go:71-82` | Use the existing seam. The Redis layer is invalidated on approve; only another replica's in-process layer (`CACHE_LOCAL_TTL`, 10 s) can lag, and a member told "still broken" for 10 s taps again. If the owner wants truly fresh, `approvals.ListApprovedIDsFresh(chatID)` (`repository.go:89`, no context, returns an error) is the alternative |
| **C4** | UI-SPEC section 2 counts "at most 2 calls" per page **open**, but the page and the channel screen are also **redrawn** after every press (switch toggle, punishment, card Yes, expired Yes), each of which would read the status again | `settings_mreq.go:173-185,327-355` | Route every draw through one helper; each redraw costs at most 2 calls (answer first, bounded 3 s). The private press's own `getChat` is not reused (S-09 item 4) |
| **C5** | UI-SPEC step 5 lists "a photo lookup that cannot be answered" as `mreq_fixed_check_failed` and "rules still broken" as `still_broken` without an order | UI-SPEC section 7 | Broken first, unknown second (S-07 item 3) |
| **C6** | PITFALLS Pitfall 5 recommends a short **negative** cache (30 to 60 s); D-03/D-12 and the CONTEXT's Deferred list reject it | `PITFALLS.md:64-75`, CONTEXT "Deferred" | CONTEXT wins. Consequence stated in S-02 and Pitfall 8: one lookup per message for a member who is still missing |
| **C7** | UI-SPEC "Keys that change" says the Phase 13 tests `TestMemberReqFixedUnmutes` and `TestMemberReqFixedRefusals` use "a member who is **not** approved" | `memberreq_fixed_test.go` approves nobody | Verify while editing; likely no change to those two |
| **C8** | UI-SPEC P14-A14 names `settings_mreq_hint_lookup` as holding the hidden-photo sentence; the keys table puts it only in `settings_mreq_desc_photo` | UI-SPEC R1 | R1 is binding: read A14 as `settings_mreq_desc_photo` |
| **C9** | D-17 gives the press "the press's 10 s budget" for fresh lookups, but Phase 13's steps (live member, `getChat`, permissions, lift, edit) already share that 10 s | `memberreq_fixed.go:29` | A 5 s lookup sub-budget (S-07), leaving at least 5 s for the unmute |
| **C10** | CONTEXT "Claude's Discretion" leaves the order of the lookups open; the PITFALLS "cheapest first" principle favours photo | PITFALLS Pitfall 5 | Photo, then channel (S-01) |

## Architecture Patterns

### System Architecture Diagram

```
 supergroup message ──► group -2 watcher (no Telegram call, always ContinueGroups)
        │                  filters ► settings (cached) ► names (pure)
        │                         names broken ───────────────────────────────┐
        │                         names clean, lookup rule on:                 │
        │                           one pipelined Redis read: pass, photo:<u>, lc:<chat> ► sub:<ch>:<u>
        │                           all positive (or lc = 0) ─► clean path (pass hit | ClearHoldOnClean + SET)
        │                           else: known admin / approved ─► stop;  else ───┤
        ▼                                                                         ▼
   join / captcha pass ──► memberReqJoinThenWelcome ─(sync, 4 s lookup budget)─► memberReqRunAsync (32 jobs, 90 s)
                                                                                  │ ctx derived from memberReqContext()
                                                                                  ▼
                                      memberReqEnforce:  exempt? ► HOLD READ ──held──► held release (lookup bits, names clean)
                                                                  │                        ► ClearHoldOnClean | held path delete
                                                                  ▼ no hold / stale
                                                         paused? ► live getChatMember (Phase 13 pacer) ► settings again ► names
                                                                  ▼
                                    ┌──────────── lookup rules, table order: photo, channel (each only if action ≠ Off) ───────────┐
                                    │ cache GET (skipped when Fresh) ► DoChan flight (key p:/s:/l:/b:) ► lookup pacer ► Telegram      │
                                    │   photo   : getUserProfilePhotos(user, limit 1)   0 = breach, error/nil = unknown               │
                                    │   channel : lc (getChat, 10 min) ► getChatMember(channel, user)  left/kicked = breach            │
                                    │   positive ► SET photo:/sub: EX 30 min (inside the flight);  unknown ► nothing written          │
                                    └──────────────────────────────────────────────┬───────────────────────────────────────────────┘
                                                     broken mask = names | lookups  ▼
                                    mute-over-restriction ► cap ► CLAIM (insert-first) ► punish ► notice (+ photo remedy tail) ► delete
  "I fixed it": 4 live marker ► 4b approvals ► 5 names + Fresh/Classify lookups (5 s) ► permissions ► take hold ► lift ► edit
  Settings page: answer empty ► status (≤ 2 paced calls, 3 s) ► edit;  punishing `ch` press: fresh getChat gate before the card
  StopMemberReqJobs ► cancels memberReqContext() ► every wait, flight and call ends; nothing claimed, nothing written
```

### Recommended file structure

```
fuku/modules/
├── memberreq_lookup.go        # NEW: verdict type, lookup pacer + options, flights, caches, photo/channel/linked/bot-status lookups, memberReqLookupBroken, the classifier
├── memberreq_rules.go         # + Lookup field, two rows, Broken-nil skip in memberReqBroken
├── memberreq_enforce.go       # memberReqLiveViolation step 6b, held-release call in the held branch
├── memberreq_held.go          # NEW (or in memberreq_enforce.go): memberReqHeldReleased
├── memberreq_watch.go         # memberReqLookupCached read, not-satisfied path (admin/approved skip, one job)
├── memberreq_notice.go        # remedy tail, memberReqStillBrokenText
├── memberreq_fixed.go         # step 4b, step 5 with Fresh/Classify lookups under a 5 s sub-budget
├── settings_mreq.go           # status value, wrapper renderers, status reader, channel gate, cannot-work block
fuku/db/memberreq/settings.go  # RulePhoto, RuleChannel, Rules, ruleColumns, actionField
fuku/db/models/member_requirements.go   # PhotoAction, ChannelAction
migrations/20261108120000_add_member_requirement_lookup_rules.sql
locales/{en,es,fr,hi,id,pt,ru,uk}.yml   # 16 new, 7 reworded
```

### Pattern 1: A verdict whose zero value is "unknown"
**What:** `memberReqUnknown` is `iota` 0; every lookup returns a verdict; the only way to a breach is an explicit `memberReqBreach` from a definite Telegram answer. **When:** every Telegram-derived judgement in this phase. A forgotten branch, a recovered panic and a zero-valued flight result all read as "does nothing" (MREQ-10 by construction).

### Pattern 2: Cache outside the flight, call and cache write inside it
**What:** `GET` positive cache (skipped when `Fresh`), then `DoChan` wrapping the paced call and the cache `SET`; callers wait with `select` on their own deadline; the flight context derives from the shutdown root. **When:** every live lookup. Gives one call for 100 concurrent messages, a deterministic cache by the time the flight ends, per-caller deadlines and D-11 for non-job callers.

### Pattern 3: Fold into the mask, never branch on the rule
**What:** the executor, the notice and the hold work on the bit mask and the rule table; the two new rules add rows and a lookup function, and no `if rule == photo` appears outside the lookup file and the page's status code. **When:** adding Phase 15's AI rule (a third `Lookup` row) needs no edit of the executor.

### Anti-Patterns to Avoid
- **A lookup in the watcher** (D-10): the watcher reads Redis only.
- **Calling `rule.Broken` on a lookup row**: nil function, a panic in the hottest path.
- **`singleflight.Do` with the caller's context, or a `DoChan` function that can panic**: a follower ignoring its deadline, or a process crash.
- **Routing a lookup through `memberReqPaced`, or a notice through the lookup pacer**: D-09.
- **`chat_status.IsBotMissingError` on a channel path**: D-07.
- **Caching anything negative, inert or unknown**: D-03, D-12, R2.
- **Reading `LinkedChatId` from callback data or the update**: D-06.
- **Dereferencing the result of `GetUserProfilePhotos` or `GetChat` without a nil check**: both can be a nil pointer with a nil error.
- **Logging the photo response, a file ID, a channel title or the Telegram description**: IDs and `memberReqErrText` only.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Fleet-wide Telegram rate budget | a per-replica token bucket or a `time.Sleep` | `ratelimit.NewTelegramPacer` with its own keys | slot reservation, shared `retry_after` block, shed on a far slot, Redis-down fallback already exist and are tested |
| One call for concurrent callers | a mutex map of in-flight requests | `golang.org/x/sync/singleflight` (`DoChan`) | already a dependency; handles waiters, panics and cleanup |
| Telegram error class | string matching on descriptions | `*gotgbot.TelegramError` `Code` via `errors.As` (`isTelegramClientError`, `cache.IsBotGoneError`) | wording is not a contract; a wrapped shed must not match |
| The clean predicate | a second hold-deleting statement | `memberreq.ClearHoldOnClean` | the cutoff and mute rules are Phase 13's tested guarantee |
| Punishments, notice, unmute | a second copy | the existing `memberReqApplyStep`, `memberReqPostNotice`, `memberReqUnmutePermissions`, `memberReqLiftMute` | ROADMAP: "Every punishment and every unmute goes through an existing executor" |
| The settings page's rule screens and card | page-specific renderers | the table-driven renderers plus the status value | S-09 |

**Key insight:** every hard part (rate budget, one-punishment latch, marker ownership, unmute under a lockdown, 64-byte callbacks) already exists. The phase is an extension at five seams plus one small lookup file; a custom solution at any of them re-opens a Phase 13 guarantee.

## Common Pitfalls

### Pitfall 1: A hidden photo is a breach the member cannot fix from the group
**What goes wrong:** `getUserProfilePhotos` returns zero for a member whose privacy hides the photo from the bot (S1, MEDIUM), so a legitimate member is held and "I fixed it" cannot help. **Avoid:** the page and the notice say so (UI-SPEC); the photo screen recommends Warn or Mute; approved users are exempt and (M11) an approved member is freed at the tap; a Ban chosen for the photo rule sits behind the two-tap card and the per-chat cap of 10 punishments a minute (`memberReqCapPerMinute`, `memberreq_limits.go:27`) is the safety net against a mass punishment when the rule is switched on. **Warning sign:** many holds with `rules = 32` right after a rule was switched on.

### Pitfall 2: A stale positive hides a removed photo or an unsubscribe for 30 minutes
**Avoid:** accepted (D-12). "I fixed it" is `Fresh` and deletes a stale positive on a definite negative; the watcher serves positives only from Redis, so the worst case is 30 minutes of a member who was clean at the last lookup.

### Pitfall 3: The linked channel changes while a hold exists
`sub:<channel>:<user>` is keyed by channel, so a new link starts from no positives; `lc` can lag the real link by up to 10 minutes (D-06), so a member may be judged against the old channel for that long; a hold whose rules include the channel bit is re-judged by the held release (`Classify`): an unlinked group (`lc = 0`) is inert, so the hold is released; "I fixed it" reads `lc` fresh. A stored non-Off channel value after an unlink stays inert and the Off-only screen clears it.

### Pitfall 4: Messages that must never be screened
A linked channel's automatic forward has `sender_chat` set and `IsAutomaticForward`; an anonymous admin and a channel post as the channel have `sender_chat`; `raidFloodSenderCounts` rejects all of them and the bot and `staffServiceUserIDs` [VERIFIED: `raid_flood.go:196-213`], and the watcher calls it before anything else (`memberreq_watch.go:204-207`), so no lookup is ever made for them. A channel administrator who posts as themselves in the group is a subscriber by definition (`administrator` or `creator` counts as subscribed).

### Pitfall 5: A channel the bot left, or was never an administrator of
The member lookup is an error (unknown, never a breach), `lc` still names the channel, `sub:` positives still serve for 30 minutes, the page shows `notadmin` (S-06: any 400 or 403), "I fixed it" treats the channel as inert (not broken). Nothing punishes. **Warning sign:** a group with the channel rule on and zero holds for the channel bit and `notadmin` on the page.

### Pitfall 6: Deleted accounts, bots, Premium and private-profile accounts
Bots and the bot itself are never checked (`memberReqNeverChecked`, `memberreq_enforce.go:176-178`; gotgbot drops bot senders at the handler, Phase 13 S-01). A deleted account sends no messages. A Premium or private-profile account that hides its photo or its channel membership from non-contacts reads as a missing photo (S1: the intended rule) or, for a channel, as the same `member`/`left` answer an administrator bot always gets (privacy settings do not hide channel membership from the channel's administrators [ASSUMED]). A "public photo" fallback visible to everyone while the main photo is hidden may or may not appear in `getUserProfilePhotos` (additional spike 4, LOW; it changes wording only).

### Pitfall 7: Process crash from a `DoChan` leader's panic
See S-04: recover inside the function; the recover must be the **first** deferred statement of the closure.

### Pitfall 8: A job per message
**Causes and bounds:** (a) a member who is exempt inside the job (administrator, approved) and has no photo: bounded by the watcher's admin/approved skip (S-03 addition); (b) a member whose lookups are persistently unknown (a Telegram-side answer for that user): one paced call per message, shed by the pacer and the runner, accepted (D-03); (c) a held member still missing the photo: one paced call per message (S-02, C1); (d) Redis down: a job per message for every member of a group with a lookup rule on, bounded by 32 jobs, the pacer's local interval and shed, accepted (D-12).

### Pitfall 9: The page open becomes a rate-limit victim
During a raid the pacer sheds an admin's page open: the screen draws `❔` and the next refresh the truth (P14-A16, harmless, UI-SPEC).

### Pitfall 10: A callback answered twice, or not at all
The gate's alert must be the press's one answer, so it runs **before** any `p.answer("", false)`; every other path answers empty first and the callback already defers an empty answer after its recover. A test counts `answerCallbackQuery` calls per press (`env.mreqAnswers()`, `memberreq_env_test.go`).

### Pitfall 11: Test hygiene (CI runs all of `fuku/modules` in one PostgreSQL database in source order)
- Every member from `uniqueLinkOwnerID()`, every chat and channel from `uniqueModuleChatID()`; `memberReqCleanup(t, chatIDs, userIDs)` deletes the `fuku:memberreq:*` Redis keys that **contain a chat ID** (`memberReqDeleteRedisKeys`, `memberreq_env_test.go:195`): register the **channel ID as a chat ID** so `sub:<channel>:<user>` and `lc:<chat>` go; `photo:<user>` contains only a user ID, so the new fake file adds its own cleanup of `fuku:memberreq:photo:<user>` (fresh user IDs make a leak harmless, but delete anyway).
- `popScripted`/`setDelay` in the staff fake are keyed by `chat_id`; `getUserProfilePhotos` has none, so its scripted key is chat 0 (`staffParamInt(params, "chat_id")` is 0). The wrapper fake owns its own per-user state instead of the scripts.
- `restoreMemberReqRunner(t)` after any test that calls `StopMemberReqJobs`; `t.Cleanup(memberReqJobs.wait)` is already in `newMemberReqEnvWith`.
- The Phase 13 second-boundary flake: the join reset deletes holds created **before the join's date**, which has one-second resolution (13-REVIEW-FIX "Gate": `TestMemberReqJoinAndMessageOnce` failed once of 30 runs). Any Phase 14 test that sends a message and then a join within the same second must date them at least 2 s apart (`mreqSendAt`).
- SQLite "database is locked" with concurrent warn writers: concurrency tests that race several jobs use Mute or Kick, never Warn.
- Singleflight and the lookup pacer are package globals: a test that holds a flight open (a hang) must release it in `t.Cleanup`, and the fast pacer is restored in cleanup, as `withFastMemberReqPacer` does.

## Code Examples

Identifiers marked "proposed" are new; every repo value is quoted in the sections above.

### The photo lookup
```go
// proposed; API shape verified in gen_methods.go:3809-3849 and gen_types.go:15547-15552
func memberReqPhotoRule(ctx context.Context, b *gotgbot.Bot, _, userID int64, mode memberReqLookupMode) memberReqVerdict {
	if !mode.Fresh && memberReqCacheHas(ctx, memberReqPhotoKey(userID)) {
		return memberReqPass
	}
	return memberReqFlight(ctx, "p:"+strconv.FormatInt(userID, 10), func(flightCtx context.Context) memberReqVerdict {
		var photos *gotgbot.UserProfilePhotos
		err := memberReqLookupPaced(flightCtx, func(callCtx context.Context) error {
			var getErr error
			photos, getErr = b.GetUserProfilePhotosWithContext(callCtx, userID, &gotgbot.GetUserProfilePhotosOpts{Limit: 1})
			return getErr
		})
		switch {
		case err != nil || photos == nil:
			return memberReqUnknown // error, timeout, shed, cancelled, null body: never a breach
		case photos.TotalCount > 0 || len(photos.Photos) > 0:
			memberReqCacheSet(flightCtx, memberReqPhotoKey(userID), memberReqPositiveTTL) // 30 min, positive only
			return memberReqPass
		}
		if mode.Fresh {
			memberReqCacheDel(flightCtx, memberReqPhotoKey(userID)) // a stale positive must not outlive a definite "no photo"
		}
		return memberReqBreach
	})
}
```

### The member lookup for the channel
```go
// proposed; statuses verified in gen_consts.go:222-227, IsMember in gen_types.go:2093
func memberReqSubscribed(m gotgbot.MergedChatMember) memberReqVerdict {
	switch m.Status {
	case gotgbot.ChatMemberStatusCreator, gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember:
		return memberReqPass
	case gotgbot.ChatMemberStatusRestricted:
		if m.IsMember {
			return memberReqPass
		}
		return memberReqBreach
	case gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return memberReqBreach
	}
	return memberReqUnknown // "" (ChatMemberUnknown or a nil answer) and anything unrecognised
}
```
Every **error** of this call is unknown at the call site (never inspected for a breach); only the classifier of S-06 reads an error, and only the bot's own.

### The classifier (S-06)
```go
// proposed
func memberReqClassifyBot(m gotgbot.MergedChatMember, err error) memberReqBotStatus {
	switch {
	case err != nil && cache.IsBotGoneError(err):
		return memberReqBotNotAdmin // any 400 or 403 (R3)
	case err != nil:
		return memberReqBotUnknown // 429, shed, 5xx, transport, timeout, cancelled
	}
	switch m.Status {
	case gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusCreator:
		return memberReqBotAdmin
	case gotgbot.ChatMemberStatusMember, gotgbot.ChatMemberStatusRestricted, gotgbot.ChatMemberStatusLeft, gotgbot.ChatMemberStatusKicked:
		return memberReqBotNotAdmin
	}
	return memberReqBotUnknown
}
```

### The watcher's cached read (S-03)
```go
// proposed: one pipelined round of single-key GETs, a second only for sub:
pipe := client.Pipeline()
passGet := pipe.Get(ctx, memberReqPassKey(chatID, userID))
photoGet := pipe.Get(ctx, memberReqPhotoKey(userID))   // only when the photo rule is on
lcGet := pipe.Get(ctx, memberReqLinkedKey(chatID))     // only when the channel rule is on
_, _ = pipe.Exec(ctx) // redis.Nil is a miss; any other error is "not satisfied"
```

### A wrapper fake (tests)
```go
// proposed, in memberreq_lookup_fake_test.go: wraps *lockdownFake (settingsEnv.fake) and is installed with e.bot.BotClient = wrapper
// getUserProfilePhotos: per-user total, scripted error, optional hang that honours ctx.Done()
// getChat: delegates, then adds "linked_chat_id" for a configured group
// getChatMember for a channel ID: per (channel, user) status JSON, the bot's own status, scripted errors (400/403/429/5xx)
// every call is recorded through f.record(method, params), so env.fake.callsFor("getUserProfilePhotos") counts it
```
The existing `lockdownFake.RequestWithContext` answers `getChat` with no `linked_chat_id` and does not know `getUserProfilePhotos` (`lockdown_fake_test.go:204-222`), so a wrapper (not an edit of the shared fake) adds them; `otherReplica()` builds a second fake that must be given the same channel and photo state.

### The refused punishing press (S-09 item 6)
```go
// proposed shape inside settingsMreqChoose, after the fresh same-value read and before the card
if rule.Store == memberreq.RuleChannel && action >= models.RequirementActionWarn {
	if screen, proceed := settingsMreqChannelGate(p, rule); !proceed {
		return screen // nil when the alert already answered and the Off-only screen was redrawn by the gate
	}
}
```

## State of the Art

| Old Approach (Phase 13) | Current Approach (Phase 14) | Impact |
|-------------------------|-----------------------------|--------|
| Pure name rules, no Telegram call | two lookup rules through a paced, coalesced, positive-cached layer | the watcher's hot path grows by one Redis round trip only for groups with a lookup rule on |
| One pacer for every requirement call | a second, shorter pacer for reads | a lookup burst cannot delay a mute notice |
| "Unknown" as an error return | a verdict type whose zero value is unknown | no branch can punish by forgetting a case |

**Deprecated/outdated:** nothing is removed. `chat_status.IsBotMissingError` stays for the other callers and is never used on a channel path.

## Assumptions Log

| # | Claim | Section | Confidence | Risk if wrong |
|---|-------|---------|------------|---------------|
| A1 | `getUserProfilePhotos(user, limit 1)` returns `total_count` 0 (not an error) for a user whose privacy hides the photo from the bot (S1) | S-11, Pitfall 1 | MEDIUM-LOW (community reports only; the Bot API text is silent) | If an error: the rule never punishes a hidden-photo member (safe); the page's privacy sentence and `mreq_remedy_photo` must be reworded in place (no key added) |
| A2 | A bot can only be an **administrator** of a channel, so for a bot that is not one `getChatMember(channel, user)` is an error (expected 400 or 403), never a status (S2) | S-06, S-11 | LOW-MEDIUM | If a status (`left`) came back to a non-administrator bot, subscribed members could be punished as not subscribed; hardening: require the bot's own status to be administrator before a `left`/`kicked` breach (Open Question 2) |
| A3 | `getChat(group).linked_chat_id` is returned to a bot that is a plain member of the supergroup (S3) | S-04, S-11 | MEDIUM | If not: the channel rule is inert in groups where the bot is not an administrator, which cannot punish there anyway; reword `settings_mreq_here_nolink` |
| A4 | Telegram's soft cap of about 30 requests a second per bot; the 20-messages-a-minute limit applies to group **sends**, not reads; a 100 ms pacer interval for lookups is safe next to the three other pacers | S-04 | LOW (not checked: `core.telegram.org` unreachable) | Too many 429s: the shared block sheds lookups (unknown), nothing punishes; raise the interval to 200 ms |
| A5 | Telegram answers `getChat` with no `linked_chat_id` for an unlinked group, which gotgbot decodes as 0 (`int64` with `omitempty`) | S-04 | HIGH for the decode (VERIFIED `gen_types.go:1814-1815`), MEDIUM for the Telegram side | A wrong "none" is cached for 10 minutes and is inert (nothing punishes) |
| A6 | A privacy setting does not hide a member's channel membership from the channel's own administrators | Pitfall 6 | MEDIUM | A subscribed member reads `left`: a false breach; same hardening as A2 |
| A7 | `singleflight.DoChan` re-raises a leader panic as an unrecoverable crash | S-04 | HIGH (VERIFIED in `golang.org/x/sync@v0.23.0/singleflight/singleflight.go` `doCall`) | n/a: the function recovers internally either way |
| A8 | The 4 s join and 20 s job lookup budgets, the 5 s press sub-budget and the 5 s pacer `MaxWait` are good defaults | S-02, S-04, S-07 | MEDIUM (reasoned from the 90 s job, 15 s join and 10 s press contexts, not measured) | Too tight: more unknowns (never a punishment); too loose: slower joins in a raid |
| A9 | `cache.IsBotGoneError` (any 400 or 403) never wrongly **punishes**, because its only use turns a rule inert | S-06 | HIGH (VERIFIED: the function and every use proposed here) | n/a; it may mark a rule inert for a non-channel cause (a user-specific 400), which loses a punishment, never causes one |
| A10 | Redis TTLs: a positive cached for 30 minutes after an unsubscribe or a removed photo is an accepted escape window (D-12) | Pitfall 2 | HIGH (a decision) | none beyond the window |
| A11 | Roughly a few percent of accounts hide their photo from non-contacts; the cap of 10 punishments a minute bounds the blast radius of a mis-set Ban | Pitfall 1 | LOW (training knowledge) | A louder first day after a rule is switched on; the card and the cap are the safeguards |
| A12 | The callback stays answerable long enough for the 3 s status read before an answer, and the 10 s press (Phase 13 spike check 8, deferred) | S-09 | MEDIUM | A `query is too old` on a slow press; the existing press already makes up to three Telegram calls before answering |

## Open Questions

1. **C1: the held release.** What we know: without it a Warn or admin-unmuted hold with a lookup bit is never released and costs a job per message. What is unclear: whether the owner accepts lookups on the held path (one paced call per message of a held member who is still missing). Recommendation: accept (S-02). If rejected, add the perpetual-job pattern to the plans' known limits and drop `memberReqHeldReleased`.
2. **The "breach gate"** (a `left`/`kicked` verdict counts only while the bot's own channel status reads administrator, one extra paced call per channel breach episode). What we know: it can only turn a breach into not-broken, so it is consistent with M13. What is unclear: whether S2 will ever show a status answer to a non-administrator bot. Recommendation: do **not** add it now (A2 is LOW-MEDIUM but bots cannot be plain channel members); add it if S2 shows a status.
3. **The watcher's admin and approvals skip on the not-satisfied path** (S-03 addition). Recommendation: add (one cached read, only on a miss). Without it an exempt member without a photo costs a job per message.
4. **A partial episode** (an unknown lookup does not block an established name breach, S-02). Recommendation: accept; the alternative couples Phase 13's name rules to the lookup pacer's health.
5. **The pacer interval** (100 ms vs 200 ms) and **MaxWait** (5 s). Recommendation: 100 ms, `MaxRetries` 1, `MaxWait` 5 s; revisit after the first live raid.
6. **Which answer wins at the tap when something is broken and another lookup is unknown.** Recommendation: broken first (S-07).
7. **Whether the page reuses the private press's `getChat`** (saves one call). Recommendation: no (S-09 item 4).
8. **Owner confirmations still open in the UI contract** (P14-A3, A4, A7 and the other listed there) are unchanged by this research; A4 (a punishing channel press is refused when the link cannot be read) is implemented as written.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build, tests | yes | go1.26.0 linux/amd64 | none needed |
| `make`, `golangci-lint` | `make test`, `make lint` | yes | not queried; the Phase 13 fix report notes the local golangci-lint is built with Go 1.25 (13-REVIEW-FIX "Not run") | the repo's `handoff/lint-local.py` stand-in, as Phase 13 did |
| miniredis (module) | every module test | yes (module cache) | v2.39.0 | none needed |
| PostgreSQL server | `make test-postgres-integrity`, `TestMemberRequirementLookupMigrationDefaults` | **no** (`pg_isready`: no response; `psql` client is installed) | - | the SQLite run proves the CHECKs through the model tags; the PostgreSQL run is a CI step, as in Phases 12 and 13 (the security audit used a throwaway cluster) |
| Node 22 | `node --test scripts/captcha-page/*.test.mjs` | yes (v22.23.4) | not touched by this phase | n/a |
| `core.telegram.org` | the three spikes | **no** (`ENOTFOUND`) | - | live checks in `14-SPIKE.md`, the owner's |
| A live Telegram test group (a hidden-photo account, a channel with and without the bot as administrator) | S1, S2, S3 | owner only | - | deferred, non-blocking (D-19) |

**Missing with no fallback:** none for code. **Missing with fallback:** PostgreSQL (CI), live Telegram (spikes).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` with `-tags testtools -race`; hand-written `gotgbot.BotClient` fakes; miniredis; SQLite (PostgreSQL in CI for the integrity list); testify only in `fuku/db` tests |
| Config file | none (build tag `testtools`); `Makefile:33-37` `test`, `:39-44` `test-postgres-integrity` |
| Quick run, lookup layer and executor | `go test -tags testtools -race -count=1 -run '^(TestMemberReq)' ./fuku/modules` |
| Quick run, settings page | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` |
| Quick run, data and locales | `go test -tags testtools -race -count=1 ./fuku/db/memberreq ./fuku/db/migrations ./fuku/db/backup ./fuku/i18n` |
| Full suite | `make test` (the only valid full run; `fuku/modules` takes 9 to 15 minutes) |
| Phase gate | `make test`, `make lint`, `make check-translations`, `make check-docs`, `make test-postgres-integrity` (CI); the captcha page's `node --test` is not touched by this phase |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MREQ-11 | a member without a photo is punished by the job and by the join entry; a member with a photo is spared; positive cached 30 min, negative never; fresh bypass and refresh | integration (fake bot, miniredis) | `-run '^(TestMemberReqPhotoRule\|TestMemberReqLookupPunishes\|TestMemberReqLookupCache)$'` | Wave 0 (new) |
| MREQ-12 | linked channel resolved once and cached (positive and none), subscribed statuses spare, left and kicked punish, restricted by `is_member`; no link means inert | integration | `-run '^(TestMemberReqLinkedChannel\|TestMemberReqChannelRule\|TestMemberReqChannelInert)$'` | Wave 0 |
| MREQ-10 | every unanswerable shape (500, 400, 403, 429, timeout, shed, cancelled, nil body, unknown status) punishes nobody, leaves no hold, notice or write | integration, table of shapes | `-run '^TestMemberReqLookupUnanswerable$'` | Wave 0 |
| MREQ-10 | the page shows `nolink`, `notadmin`, `basic`, `unknown`; Off-only channel screen; refused punishing press | integration | `-run '^(TestSettingsMreqChannelStatus\|TestSettingsMreqChannelRefusal\|TestSettingsMreqOpenCost\|TestSettingsMreqBasicGroup)$'` | Wave 0 |
| PLAT-06 | one lookup for 100 messages; the pacer shared across two replicas over one miniredis, Phase 13's pacer untouched; far slot shed with no request; jobs and flights end at `StopMemberReqJobs` | integration, concurrency | `-run '^(TestMemberReqLookupSingleflight\|TestMemberReqLookupPacer\|TestShutdownEndsMemberReqLookups)$'` | Wave 0 |
| MREQ-11/12 | "I fixed it": step 4b, fresh lookups, still-broken (1 to 4 phrases) and `_many`, `check_failed`, inert | integration | `-run '^(TestMemberReqFixedApproved\|TestMemberReqFixedLookups\|TestMemberReqChannelInert)$'` | Wave 0 |
| MREQ-01 (7 rules) | CHECKs 0..4 on both columns, zero defaults for an existing row, no clobbering | repository (SQLite and PostgreSQL) | `go test -tags testtools -race -count=1 -run '^TestChatRequirementsSettings$' ./fuku/db/memberreq` | exists (extend) |
| MREQ-01 | migration statements, idempotence, named constraints | unit (statements), PostgreSQL (defaults) | `-run '^(TestMemberRequirementLookupMigrationStatements\|TestMemberRequirementLookupMigrationDefaults)$' ./fuku/db/migrations` | Wave 0 |

### Test names the plans add (families)
- **Rule table and pure functions:** `TestMemberReqRuleTable` (changed), `TestMemberReqBrokenIgnoresLookupRules`, `TestMemberReqStrictestLookupBits`, `TestMemberReqPassValue` (extended), `TestMemberReqRuleKeysExist`, `TestMemberReqNoticePhotoRemedy` (the three notices end with the tail only when the photo bit is set; warn-limit and ban never), `TestMemberReqStillBrokenText` (1 to 4 phrases, 5 or more `_many`).
- **Lookup layer** (`memberreq_lookup_test.go`): `TestMemberReqPhotoRule` (zero is breach; a count above zero or a non-empty `Photos` is pass; error, 400, 403, 429, timeout, shed, cancelled and a `null` body are unknown; a positive is cached 30 min, a missing photo and an unknown never; `Fresh` bypasses and refreshes and deletes a stale positive on a definite negative), `TestMemberReqChannelRule` (member, administrator, creator and restricted with `is_member` pass; restricted without it, left and kicked breach; every error and an unrecognised or nil answer unknown; a positive cached under `sub:<channel>:<user>`), `TestMemberReqLinkedChannel` (a linked ID cached 10 min, none cached as `0`, an error never cached, a non-negative ID read as none, `Fresh` refreshes), `TestMemberReqBotStatusClassifier` (the S-06 table, 400 and 403 inert, 429 and a shed and a 5xx unknown, never `IsBotMissingError`), `TestMemberReqLookupSingleflight` (100 concurrent callers of one key, one call, for each key family; a follower honours its own deadline; a panic in the flight function is recovered and returns unknown; a caller's cancellation does not cancel the shared flight), `TestMemberReqLookupPacer` (keys `fuku:memberreq:lkp:pace:next` and `:block`; two pacers built from `memberReqLookupPacerOptions` over one miniredis space calls at least the interval apart and a 429 on one blocks the other; a far slot is shed with no Telegram request; `fuku:memberreq:pace:next` is never written by a lookup), `TestMemberReqLookupRedisDown` (every read a miss, writes a no-op, the pacer's local fallback).
- **Executor and join** (`memberreq_lookup_enforce_test.go`): `TestMemberReqLookupPunishes` (each rule and each of the five punishments, via `mreqEnforce` and via the real dispatcher), `TestMemberReqLookupSpares`, `TestMemberReqLookupOrder` (photo before channel; a rule Off makes no call; no call in a paused chat; no call before the exemptions; the cap and the claim after the lookups), `TestMemberReqLookupUnanswerable` (the shape matrix above, run for the job and for the join entry: no hold row, no notice, no `banChatMember`/`restrictChatMember`/`unbanChatMember`/warn row, the member welcomed at join), `TestMemberReqLookupDoesNotBlockNameBreach` (S-02 decision), `TestMemberReqLookupHeldPath` (a hold without a lookup bit makes zero lookup calls), `TestMemberReqHeldRelease` (a Warn hold whose photo is fixed is released and its positive cached; still missing keeps it and deletes the message; broken names skip the lookup; an inert channel releases a channel-only hold; unknown keeps it; a mute hold is never lifted), `TestMemberReqLookupOnePunishmentAcrossReplicas` (Mute or Kick), `TestMemberReqLookupNeverReplacesRestriction`, `TestMemberReqLookupCapAndPause`, `TestMemberReqJoinLookups` (the 4 s join budget: a hung lookup welcomes the member and the first message is caught).
- **Watcher** (`memberreq_watch_lookup_test.go`): `TestMemberReqWatcherLookupRules` (clean member with cached positives: no job, no Telegram call, `memberReqJobs` never started; a miss starts exactly one job; names broken unchanged; `lc = 0` reads as nothing to verify; a known admin and an approved member start no job; Redis down starts a job), `TestMemberReqWatcherLookupSingleflight` (100 messages from one user, one `getUserProfilePhotos` call), `TestMemberReqPassCacheWithLookups` (the pass key never replaces a lookup positive; no inert or unknown value is written anywhere), `TestMemberReqWatcherContinuesGroups` (re-run with both rules on).
- **"I fixed it"** (`memberreq_fixed_lookup_test.go`): `TestMemberReqFixedApproved` (UI-SPEC check 11: approved member freed for a name, photo and channel mute with no settings read and no lookup; approvals error answers `check_failed`, no restrict, hold untouched; the approvals read precedes the settings read and every lookup), `TestMemberReqFixedLookups` (a stale positive does not hide a removed photo; a positive refreshes the cache; still-broken names exactly the broken rules; five or more `_many` with the right `{n}`; the whole press stays inside 10 s with a slow pacer; the answer language is the group's), `TestMemberReqChannelInert` (UI-SPEC: member lookup unanswerable plus a definite non-administrator or a 400/403 is inert and frees the member; 429, 5xx, transport, timeout and a shed stay `check_failed`; the bot-status read goes through the lookup pacer), `TestMemberReqFixedNeverLifts` (re-run with the photo rule).
- **Shutdown:** `TestShutdownEndsMemberReqLookups` (`shutdown_drains_test.go` style: a hung lookup, `StopMemberReqJobs` returns true within `memberReqStopWait`, no hold, no write, `restoreMemberReqRunner`), `TestShutdownWaitsForMemberReqJobs` unchanged.
- **Settings page** (`settings_mreq_lookup_test.go` plus the changed Phase 13 tests of S-10): `TestSettingsMreqChannelStatus` (every status: linked and administrator no line and 3 rows; not administrator, 400 and 403 included; no link `[Off]` alone, stored Ban still shown, no hints; basic group; error, timeout and shed all `❔`, never an error text, never the cannot-open edit), `TestSettingsMreqOpenCost` (the call counts of S-09, all through the lookup pacer and none through `memberReqPacer`, under 3 s), `TestSettingsMreqChannelRefusal` (UI-SPEC check 8 for `st` 1 to 4 and `my`, answered exactly once), `TestSettingsMreqBasicGroup`, `TestSettingsMreqPhotoScreen`, `TestSettingsMreqRowBudget` (widths `{2, 2, 2, 1, 2, 2}` private and group), `TestSettingsMreqCallbackBytes` (53 and 52), `TestSettingsMreqPageFitsEveryLocale`, `TestSettingsStaffPrivacy` (the new screens, every channel status, a channel whose title and ID the fake knows).

### Sampling Rate
- **Per task commit:** the quick command of the plan's own family (above), `-race -count=1`.
- **Per wave merge:** `go test -tags testtools -race -count=1 -run '^(TestMemberReq|TestSettings|TestLoadSettings|TestChatRequirements|TestMemberRequirement|TestShutdown)' ./fuku/modules ./fuku/db/memberreq ./fuku/db/migrations ./fuku/i18n`.
- **Phase gate:** `make test` green, `make lint`, `make check-translations`, `make check-docs`, and the PostgreSQL integrity run in CI before `/gsd-verify-work`.
- **Flake discipline:** run each new concurrency test `-race -count=20` once; the join-date test with dates at least 2 s apart; no Warn in a multi-goroutine test.

### Wave 0 Gaps
- [ ] `fuku/modules/memberreq_lookup_fake_test.go`: the wrapper over `*lockdownFake` (`getUserProfilePhotos` per user with hang and scripted errors that honour `ctx`, `getChat` with `linked_chat_id`, channel `getChatMember`, recording through `f.record`), plus `withFastLookupPacer(t)` (the `withFastMemberReqPacer` shape, `memberreq_env_test.go:36-50`), `mreqLookupCleanup`, and a `mreqChannel` builder that also configures a second replica's fake.
- [ ] test seams in production code: `memberReqLookupPacerOptions`/`memberReqLookupPacer`, the three phase budgets and `memberReqLookupFlightTimeout` as variables, `memberReqRedisClient` (exists) for Redis-down.
- [ ] Framework install: none.

## Security Domain

### Applicable ASVS Categories (level 1)
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication / V3 Session | no | none new: Telegram authenticates senders; callback data is never authority |
| V4 Access Control | yes | the per-group live admin check and the owner gate on every press (unchanged); `settingsRightRestrict` for choices and Yes; the live link check before a punishing `ch` press; presser-only "I fixed it"; lookups only after the live admin check, so an administrator is never looked up for punishment |
| V5 Input Validation | yes | strict callback decoding (`t` from the table, `v` one digit 0..4, `e` via `settingsCardNumber`); nil-checks on `GetChat` and `GetUserProfilePhotos` bodies; a negative channel ID only; `rules` mask from the table |
| V6 Cryptography | no | no new secret, token or key (nothing for `logredact`) |
| V7 Logging | yes | chat, channel, user IDs and `memberReqErrText` only; never a photo file ID, a title, a name, a notice text or a Telegram description |
| V8 Data Protection | yes | the photo lookup sends only the user ID; the response's file IDs are never stored or logged; caches hold the string `1` (and a channel ID in `lc`), no identity beyond the IDs in the key |
| V11 Business Logic | yes | unknown never punishes (verdict zero value); one punishment per episode (claim after lookups); the per-chat cap and pause; the two-tap card for Kick, Ban, Mute |
| V13 API and rate limiting | yes | the fleet-wide lookup pacer, shed on a far slot, the runner's 32-job cap, singleflight |

### Known Threat Patterns for this stack
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Mass punishment of members who hide their photo when a Ban rule is switched on | Denial of service / Tampering | card, the 10-a-minute cap and pause, the Warn/Mute recommendation, approvals exempt |
| A shed or failed lookup punishing a legitimate member | Tampering | verdict zero value is unknown; unknown and inert are not broken; the claim follows every lookup |
| False breach if a non-administrator bot received a status answer (A2) | Tampering | S2 spike; optional breach gate (Open Question 2) |
| Forged `t=ch&v=3` for a group with no linked channel, or a stale card | Tampering / Elevation | the live fresh `getChat` gate before the card and before the write; the right is judged on a fresh member record |
| Process crash from a panicking `DoChan` leader | Denial of service | internal recover in the flight function; the first deferred statement |
| Lookup flood from many new accounts | Denial of service | pacer shed, runner cap, positive caches, singleflight, no new goroutine pool |
| The page or notice naming the linked channel or the Staff Group | Information disclosure | status lines are words; `TestSettingsStaffPrivacy`; `assertMemberReqNoLeak` |
| Cache poisoning of a positive for another member | Spoofing | keys built from Telegram-authenticated IDs of the update's sender; values carry no input |
| Telegram file URL or bot token in an error | Information disclosure | the photo lookup uses `GetUserProfilePhotos` (no file download); errors by code only |

## Recommended decomposition into plans

Eight plans; the unit that cannot be split is "a table row, its lookup, its executor use and its tests"; locale files and AGENTS.md are single-owner per wave to avoid merge conflicts (`use_worktrees` is false).

| Plan | Wave | Content | Files owned |
|------|------|---------|-------------|
| 14-01 data | 1 | migration, model, `memberreq` enum/columns/`actionField`, repository and migration tests, `TestChatRequirementsSettings` extension, Makefile list | `migrations/`, `fuku/db/models/member_requirements.go`, `fuku/db/memberreq/settings*.go`, `fuku/db/migrations/*_test.go`, `Makefile` |
| 14-02 lookup layer | 1 | `memberreq_lookup.go` (verdict, pacer, flights, caches, photo/channel/linked/bot-status lookups, classifier, `memberReqLookupBroken`), the wrapper fake, helpers, the lookup-layer tests | `memberreq_lookup.go`, `memberreq_lookup_fake_test.go`, `memberreq_lookup_test.go` |
| 14-03 table and executor | 2 | `Lookup` field and rows, `memberReqBroken` skip, `memberReqLiveViolation` step 6b, phase budgets, held release, rule-table test changes, join tests | `memberreq_rules.go`, `memberreq_enforce.go`, `memberreq_held.go`, their tests |
| 14-04 watcher | 3 | `memberReqLookupCached`, the not-satisfied path, watcher tests | `memberreq_watch.go`, `memberreq_watch*_test.go` |
| 14-05 notice and "I fixed it" | 3 | remedy tail, `memberReqStillBrokenText`, step 4b, step 5 with the sub-budget, tests | `memberreq_notice.go`, `memberreq_fixed.go`, their tests |
| 14-06 settings page | 3 | status value, wrapper renderers, status reader, channel gate, screens, the changed Phase 13 settings tests | `settings_mreq.go`, `settings_mreq*_test.go`, `settings_staff_privacy_test.go` |
| 14-07 locales | 4 | 16 new and 7 reworded keys in 8 files, every i18n test change, the native-read list | `locales/*.yml`, `fuku/i18n/*_test.go` |
| 14-08 closing | 5 | `TestShutdownEndsMemberReqLookups`, `14-SPIKE.md`, AGENTS.md (or per plan), docs, README, `make generate-docs`, the gates | `shutdown_drains_test.go`, `14-SPIKE.md`, `AGENTS.md`, `docs/`, `README.md` |

Plans 14-03 to 14-06 use the marker locale in their tests, so they do not wait for 14-07; `make check-translations` runs in 14-07 and 14-08 only (literal `GetString` keys must exist then).

## Sources

### Primary (HIGH confidence)
- The repository files named with line ranges above, read this session: `fuku/modules/memberreq_rules.go`, `memberreq_enforce.go`, `memberreq_watch.go`, `memberreq_join.go`, `memberreq_fixed.go`, `memberreq_notice.go`, `memberreq_settings.go`, `memberreq_limits.go`, `memberreq.go`, `settings_mreq.go`, `settings_menu.go`, `settings_pages.go`, `settings_staff.go`, `settings_gate.go`, `raid_flood.go`, `raid_ai_image.go`, `fuku/db/memberreq/settings.go`, `holds.go`, `fuku/db/models/member_requirements.go`, `fuku/db/approvals/repository.go`, `fuku/db/cache/local.go`, `fuku/utils/ratelimit/telegram_pacer.go`, `fuku/utils/cache/adminCache.go`, `fuku/utils/chat_status/owner.go`, `fuku/db/migrations/runner.go`, `scripts/migrate_psql.sh`, `migrations/20261029120000*.sql`, `20261105120000*.sql`, `20261106120000_add_member_requirements.sql`, `main.go`, `fuku/modules/shutdown_drains.go`, the test files named in S-10 and S-12, `locales/en.yml`, `AGENTS.md`, `Makefile`.
- gotgbot v2.0.0-rc.36.0.20260919140833-240296efadb4 in the Go module cache: `gen_methods.go` (`GetUserProfilePhotos`, `GetChat`, `GetChatMember`), `gen_types.go` (`UserProfilePhotos`, `ChatFullInfo.LinkedChatId`, `unmarshalChatMember`, `ChatMemberUnknown`, `MergedChatMember.IsMember`), `gen_consts.go` (status constants).
- `golang.org/x/sync@v0.23.0/singleflight/singleflight.go` (`Do`, `DoChan`, `doCall`).
- 14-CONTEXT.md, 14-UI-SPEC.md (with amendments M6 to M13 and R1 to R4), 14-UI-CHECK.md, 13-CONTEXT.md, 13-RESEARCH.md, 13-VERIFICATION.md, 13-REVIEW-FIX.md, 13-SECURITY.md, `.planning/research/PITFALLS.md` (Pitfalls 5, 17 to 21, 30), `STACK.md`, `FEATURES.md`, `ROADMAP.md` Phase 14, `REQUIREMENTS.md` (MREQ-10 to 12, PLAT-06).

### Secondary (MEDIUM confidence)
- `STACK.md` rows 39 to 41 and 263 to 264 (the Bot API behaviour of the three calls, themselves marked LOW-MEDIUM and MEDIUM there).

### Tertiary (LOW confidence)
- Two WebSearch queries (hidden profile photo; `getChatMember` on a channel for a non-administrator bot): no authoritative statement found; results were generic library pages and an inference. Nothing in this document rests on them. `core.telegram.org` could not be fetched (`ENOTFOUND`); it has to be allowed in the session's network settings before the spikes' wording can be checked against the Bot API text.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - no new dependency; every API shape read from the module cache.
- Architecture (seams, order, budget, shutdown): HIGH - read from the code at HEAD; the held-release decision (C1) is a recommendation that needs the planner's explicit yes.
- Telegram-side behaviour (S1, S2, S3, the pacer interval): MEDIUM-LOW - deferred live checks; every outcome is safe by construction.
- Pitfalls: HIGH for the code-derived ones, MEDIUM for the Telegram-privacy ones.

**Research date:** 2026-11-08
**Valid until:** 2026-12-08 (stable repo and library versions; re-read `ls migrations | tail`, the locale counts, and `memberreq_enforce.go` if Phase 15 or 16 lands first)

