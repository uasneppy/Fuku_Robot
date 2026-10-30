# Phase 3: Staff Audit and Undo - Research

**Researched:** 2026-10-05
**Domain:** Go / gotgbot / GORM audit schema, Redis card reuse, Telegram restore-prior semantics, log-channel fan-out
**Confidence:** HIGH on repo integration points (all read this session), MEDIUM on live Telegram behaviour (core.telegram.org is blocked in this sandbox; the Bot API docs could only be read through gotgbot's vendored comments)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Undo semantics
- **D-01:** Undo mirrors each action:
  - ban → unban
  - mute → unmute
  - unban → re-ban
  - unmute → re-mute

  **Kick gets no Undo button**: the person can simply rejoin, so there is nothing to reverse. Timed variants (`/tban`, `/tmute`) undo like ban and mute.
- **D-02:** **Undo restores the prior state, and never just lifts.** For each group, the record keeps the target's status *before* the staff action: member/left, restricted with its end date, or kicked with its end date. Undo puts back exactly that. Example: a permanent staff ban that upgraded an existing 1-day ban (Phase 2 D-13) goes back to the 1-day ban with its original end date when that date is still in the future, or is lifted when the date has passed. Undo never removes a restriction staff didn't add, and never adds one that wasn't there. An undone unban or unmute re-applies the restriction that was lifted, with its original end date where one is known. — **Reversibility:** one-way — the pre-action state has to be captured at action time in the audit schema. Rows written without it can't be restored correctly later, so the column set is fixed by the first migration.
- **D-03:** **Undo touches only groups where the original action was applied (✅).** Groups that were skipped or failed in the original are left alone. They show in the undo summary as "skipped: not applied originally", or are listed by the planner's choice, but they are never acted on.
- **D-04:** **Skip a group if its status changed since.** Before reversing, compare the target's live status (the same live `getChatMember` read) with what the staff action left behind. On any difference, for example a group admin already unbanned them or banned them for longer, the group is skipped with "skipped: changed since". Undo never overrules a later decision. Every reverse call still goes through `decideStaffAction`, so nothing lifts a ban by accident.
- **D-05:** **The Undo button has no time limit.** It works as long as the record exists, and records are kept forever (D-16). D-04 and the Confirm step (D-07) are what protect against stale undos.

#### Undo flow and summary
- **D-06:** **Any live member of the Staff Group may press Undo**, not only the issuer (STAFF-11). Each group is then checked live for *the presser's* rights: creator, or administrator with `can_restrict_members`, from the live `getChatMember`. The admin cache is never used. Groups where the presser lacks rights are skipped with the reason. Someone outside the Staff Group can't undo anything, and an anonymous admin gets "post as yourself" (STAFF-06). The link owner is rechecked through `recheckLink`, as in Phase 2.
- **D-07:** **Undo needs a Confirm tap.** Pressing Undo shows a card like "Undo ban of Name (123) in N groups?" with Confirm and Cancel, and only the person who pressed Undo can confirm it. It follows the Phase 2 card model: a short-lived Redis card behind a token, issuer-only Confirm, and Cancel and Expired states. N is the count of groups where the original was applied.
- **D-08:** **The undo result is a new message**, posted as a reply to the original summary, with its own ⏳/✅/⏭/❌ lines, tally and Phase 2 overflow rules (done lines collapse first, and skipped and failed lines are always listed). The original summary stays as history: its Undo button is removed and it gains a line like "↩ Undone by Name, see reply". If the original can't be edited (deleted, or too old), the undo still runs and its reply is sent anyway, as a plain message if the reply target is gone. ROADMAP criterion 4's "same done, skipped or failed summary" is read as *the same format and rules*, not the same message.
- **D-09:** **One undo per action.** The Undo button is removed when an undo is confirmed. A second press from a stale client or another replica answers "already undone by Name". The move to undone is a compare-and-set, the same as the card state moves in `staff_action_card.go`. Groups skipped during an undo stay as they are, and staff fix them by hand or re-run a command. There is no "retry undo" button, and an undo can't itself be undone (run the command again instead).

#### Log-channel posts
- **D-10:** **Staff actions use the existing `admin` log category** (`logchannels.CategoryAdmin`). A group that has turned admin logs off gets no staff posts. No new category column and no migration on log settings.
- **D-11:** **A post names the issuer and says "via Staff Group", but does not name the Staff Group.** It has a staff-specific hashtag, the issuer's mention, the target's name and ID, the duration ("permanent" or the length), the reason, and a "via Staff Group" marker. The Staff Group's title and chat ID never appear in a linked group's log channel. Example shape: `#STAFF_BAN · Admin: Alice · User: Name (123) · 2 days · spamming · via Staff Group`. The reason is HTML-escaped and length-capped (Pitfall A).
- **D-12:** **Posts go out as each group succeeds.** A post is sent right after the action is applied (✅) in that group, and is skipped for groups that were skipped or failed. It goes through the shared pacer (`staffPaced`) like every other staff fan-out call. **A failed log post never changes the group's ✅ line** or counts as a failed action. At most it is logged at debug or warn level, matching `actionlog`'s rule that logging never blocks the action.
- **D-13:** **Undos are logged too, under the same rules**: admin category, posted as each group's undo succeeds, presser named, "via Staff Group". The post says what was undone. Example shape: `#STAFF_UNDO · unban · Admin: Bob · User: Name (123) · undoes ban by Alice · via Staff Group`.

#### Recent-actions list (`/staff`)
- **D-14:** **The `/staff` panel gets a "Recent actions" button.** It switches the same message to a history view with Prev/Next paging and a "Back" button to the links panel. The links panel itself does not grow. Any Staff Group member can open it, and it shows only actions of *this* Staff Group.
- **D-15:** **Each entry is one compact line plus a detail button.** A line looks like `🔨 Ban · Name (123) · 2d · spamming · by Alice · 5 Oct 12:04 · ✅4 ⏭1 ❌1 · ↩ undone`. A button per entry opens that action's full per-group list in the same ✅/⏭/❌ format as the summary, with the undo outcome if there was one. The detail view also offers Undo when the action can still be undone (D-01, D-09), through the same Confirm flow (D-07).
- **D-16:** **10 entries per page, newest first, paged back through everything stored. Records are kept forever**, with no pruning job. Staff tables stay out of backup, export, import and reset, as the existing AGENTS.md rule says. — **Reversibility:** reversible — adding a pruning job later is a local change. It would only take away undo on older records.

### Claude's Discretion
- The audit schema: table and column layout, a parent action row with per-group outcome rows or another shape, and how undo results are stored against the original. It must survive restarts (STAFF-10), hold the pre-action state per group (D-02), and record the undo with who, when and per-group outcomes. Migration rules in AGENTS.md apply.
- When the record is written: at Confirm, then updated as groups finish, or written once at the end. A restart mid-run must still leave a truthful record. Phase 2's `StopStaffActions` marks unfinished groups "interrupted by restart", and those lines are not undoable.
- How the record links to the Telegram message (Staff chat ID + summary message ID) for D-08's reply and edit, and how this survives a Staff Group chat migration (`staff.RekeyChat`).
- How callback data names a record. It needs a short ID or token through `callbackcodec` (64-byte cap), and never user text.
- How unlinked or renamed groups show in history and detail: probably the stored title at action time, marked as no longer linked. Undo should skip groups that are no longer linked, with a reason.
- Undo on actions that ran before Phase 3 shipped: they have no record, so there is no button.
- Whether pressing Undo while the original run is still going is impossible because the button appears only on the final summary, or is refused.
- How undo interacts with the per-target fan-out lock (`alita:staff:lock:target:<id>`). It should take the same lock, so an undo and a new action on the same person never race.
- The exact locale wording for every new string, in all 7 locale files.
- The reason length cap for log posts (Pitfall A suggests about 300 characters) and how history lines truncate long names and reasons.

### Deferred Ideas (OUT OF SCOPE)
None: the discussion stayed within phase scope.

Not in this phase (from `<domain>`):
- Posting anything into linked groups' chats. Only their log channels are written to.
- Picking a subset of groups for an action or an undo. Out of scope by design (PROJECT.md).
- Re-applying recorded bans to a group linked later.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description (REQUIREMENTS.md, read this session) | Research Support |
|----|--------------------------------------------------|------------------|
| STAFF-09 | Each applied action and its reason are posted to the log channel of every group where it was applied. | "Log-channel posts" section: `actionlog` seam split, `staffPaced` send, worker-side hook after `progress.set`, D-11 template, escape and cap. |
| STAFF-10 | Every staff action is recorded with issuer, target, action, duration, reason, time and per-group outcomes. | "Audit schema" and "Record lifecycle": two tables, create at Confirm, write-ahead prior state, per-group results, authoritative finalize, shutdown/crash truthfulness. |
| STAFF-11 | The summary has an "Undo everywhere" button. A Staff Group member who is an admin with restrict rights in a group can reverse the action there. The same per-group checks and the same summary apply. | "Undo decision table", "Undo flow", "What to generalize": `decideStaffUndo`, undo card in the existing Redis hash, DB claim, generalized run coordinator. |
| SETUP-09 | The `/staff` panel lists recent staff actions: who, what, target, when, reason, and per-group outcome. | "History view": offset paging, one GROUP BY tally query, UTF-16 caps through `fitStaffSummaryLines`, detail view, Back via `staffRerenderPanel`. |
</phase_requirements>

## Project Constraints (from CLAUDE.md / AGENTS.md)

Read this session: `/home/user/Fuku_Robot/.claude/CLAUDE.md` (includes `AGENTS.md`). Treated as locked.

- Go 1.26.0, gotgbot v2, GORM on PostgreSQL (SQLite in tests), Redis. No new stack; no new packages.
- Migrations are append-only, timestamped greater than every existing file, one transaction per file, no top-level `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`. Never edit an applied file.
- Every write `cache.DeleteCache`s the keys it affects. The audit tables are authority/accountability reads and are **never cached**, so there is nothing to delete. State that in the repository file comment so a reviewer does not look for it.
- Commands go through `helpers.WrapCommand`. Phase 3 adds **no new command**: only callbacks under the existing `staff` namespace and a button on the `/staff` panel.
- Callback data only through `alita/utils/callbackcodec` (`encodeCallbackData`, `<ns>|v1|<url-encoded>`, 64-byte cap). `encodeCallbackData` returns `""` on overflow: never ship a button without checking.
- Locale keys go in all 7 files (`en es fr hi id pt ru`); copy key names from `locales/en.yml`; every `tr.GetString` call must use a literal key so `make check-translations` sees it.
- Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`.
- `decideStaffAction` is the only place staff calls are chosen; every staff fan-out Telegram call goes through `staffPaced`; per-group authority is only the live `getChatMember`.
- Tests: real fixtures only (`internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes), `-tags testtools`, `make test`. No mock libraries; assert observable behaviour.
- Conventional Commits; update `AGENTS.md` in the same commit as any change that invalidates a rule in it (list below in "AGENTS.md edits").
- Staff tables are never part of backup, export, import or reset. Verified: no code enumerates tables by name outside migrations (grep for `information_schema|pg_tables|GetTables|sqlite_master` in non-test Go finds only `alita/db/migrations/runner.go:852`), so new tables are excluded by default.
- Ignore `/home/user/Fuku_Robot/.claude/worktrees/agent-aae3302fd6b0eafae/`: a stale worktree copy of Phase 1 code, not part of the build.

## Summary

Phase 3 is almost entirely **integration work on Phase 2 machinery**, plus one new durable schema. There is no new library. The three load-bearing design facts are: (1) the record has to be created at Confirm and filled per group, with the pre-action state written **before** each Telegram write (write-ahead), because D-02 is a one-way door and a hard crash can otherwise lose it; (2) undo is a second *run* through the same coordinator, pacer, target lock and summary renderer, so `startStaffActionRun`, `composeStaffActionSummary` and the per-group check chain have to be parameterized rather than copied; (3) D-02's "restore exactly" cannot be met with `until_date` alone: `restrictChatMember` replaces the whole permission set, so the record must also keep the prior **permission bits** of a restricted target, and the restore call must send `UseIndependentChatPermissions: true`.

The decision logic for undo must live in `staff_action_decide.go` as a pure `decideStaffUndo`, next to `decideStaffAction`. It takes the original kind, the recorded prior state, the "left behind" state derived from the original verdict, the live state and `now`. Its one deliberate exception to the Phase 2 invariants is a restore call sent to a **kicked** target (restoring a prior restriction over the staff's own ban), and that is allowed only when the live state equals what the staff action left behind (D-04). A skeleton and a full verdict matrix are below. Existing unkeyed `staffVerdict{...}` test literals mean the verdict type must not gain fields; use an embedding wrapper.

Record linkage has two traps. `RekeyChat` re-keys `staff_chat_id` on actions (so history follows a Staff Group migration) but a message ID only means something in the chat it was sent to, so the record must also store an immutable `summary_chat_id` and every edit or reply must compare it with the current chat before use, or an undo after a migration could edit an unrelated message. And `RekeyChat` will now touch the new table, so **every** test harness that calls it needs the new models in its `AutoMigrate` list or the existing migration tests fail with "no such table".

**Primary recommendation:** two tables (`staff_actions` parent with undo-claim columns, `staff_action_groups` child with prior state, outcome and per-group undo outcome); create both at Confirm and abort the card if that write fails; write each group's prior state before its Telegram call and fail that group closed (`fail_internal`, no write) if the write fails; flush all results in one transaction at the end; attach the Undo button in the final edit; run undo as a generalized fan-out whose claim is a conditional `UPDATE ... WHERE undo_started_at IS NULL`.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Audit record (action, per-group outcome, prior state, undo) | Database / Storage (PostgreSQL via GORM) | API / Backend (module writes it) | Must survive restarts and Redis loss; Redis card lives 1 h, undo has no time limit (D-05). |
| One-undo-per-action claim | Database / Storage (conditional UPDATE) | — | The Redis card hash expires; the claim must outlive it. Same pattern as `SetLinkHealth` / `DeleteLinkIfOwner`. |
| Undo confirm card (pending state, issuer-only Confirm) | Redis (existing `alita:staff:act:<token>` hash + Lua CAS) | API / Backend | Short-lived interaction state shared by replicas. |
| Per-target mutual exclusion | Redis (existing `alita:staff:lock:target:<id>`) | — | Undo and new actions on one person must not race. |
| Pre-action state capture | API / Backend (worker reads live `getChatMember`, writes DB) | Database | The only moment the prior state is knowable. |
| Restore / reverse decision | API / Backend (pure `decideStaffUndo`) | — | Single decision table, no I/O, exhaustively testable. |
| Fleet-wide pacing of every call incl. log posts | Redis (`staffActionPacer`) | API / Backend | Existing shared budget; per-replica limiters are forbidden by AGENTS.md. |
| Log-channel destination lookup | Database (cached `logchannels.Get`) | API / Backend | Non-authority read, existing cache is fine. |
| Summary and history rendering | API / Backend (pure render functions) | Telegram (message edit) | Pure functions with UTF-16 caps, reused from Phase 2. |
| Authority (membership, per-group rights, link owner) | Telegram live reads | — | Never cached (Pitfall 1). |

## Standard Stack

### Core
No new libraries. Everything below is already in `go.mod` and was read this session.

| Library | Version (go.mod via CLAUDE.md) | Purpose | Why Standard |
|---------|-------------------------------|---------|--------------|
| `gorm.io/gorm` | v1.31.2 | Audit models and repository, conditional UPDATE claims | Repo pattern for every table. [VERIFIED: CLAUDE.md Technology Stack] |
| `github.com/PaulSonOfLars/gotgbot/v2` | v2.0.0-rc.36 | `RestrictChatMemberOpts.UseIndependentChatPermissions`, `ReplyParameters`, `EditMessageTextOpts.ReplyMarkup` | Telegram client. [VERIFIED: CLAUDE.md; vendored source below] |
| `github.com/redis/go-redis/v9` | v9.22.0 | Undo card and lock reuse (existing Lua scripts) | Existing card model. [VERIFIED: CLAUDE.md] |
| `github.com/alicebob/miniredis/v2` | v2.39.0 | Test Redis | Existing fixture. [VERIFIED: CLAUDE.md] |
| `alita/utils/callbackcodec` | in-repo | 64-byte callback data | Mandated by AGENTS.md. |

### Supporting
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `encoding/json` (stdlib) | Persist the prior `gotgbot.ChatPermissions` as TEXT | Prior-permission snapshot column. |
| `html`, `unicode/utf16` (stdlib) | Escape and UTF-16 length of log posts and history lines | Already used in `staff_action_summary.go`. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| TEXT JSON of `ChatPermissions` | A bitmask BIGINT | A bitmask freezes bit positions forever (one-way door) and drops unknown future permissions; JSON is self-describing and SQLite/PostgreSQL portable. Do not use JSONB: AutoMigrate on SQLite and the migration splitter add risk for no query benefit. |
| Two tables + undo columns | A third `staff_action_undos` table | The UNIQUE(action_id) insert would be a neat claim, but a third table and model buys nothing: D-09 allows exactly one undo per action, so the columns fit on the parent. |
| New Redis key family for undo cards | Reuse `alita:staff:act:<token>` | Reuse keeps the Lua scripts and the expiry/lazy-check paths. Use `action="undo"` plus new callback action codes so a replica still running the old code answers "expired" and can never run a ban from an undo card (see Pitfall 9). |

**Installation:** none.

**Version verification:** not applicable (no packages added). gotgbot behaviour was read from the vendored module:
`/root/go/pkg/mod/github.com/!paul!son!of!lars/gotgbot/v2@v2.0.0-rc.36.0.20260919140833-240296efadb4/gen_methods.go`.

## Package Legitimacy Audit

No external packages are installed in this phase. Not applicable. **Packages removed due to [SLOP] verdict:** none. **Packages flagged as suspicious [SUS]:** none.

## Verified in-repo values (quoted verbatim)

Every discrete value used in the code skeletons below appears here. Anything not here is `[ASSUMED]` or a new proposed identifier (marked "new").

| Fact | Source | Verbatim |
|------|--------|----------|
| Kinds | `alita/modules/staff_action_decide.go:8-19` | `staffKindBan staffActionKind = "ban"`, `staffKindMute ... = "mute"`, `staffKindKick ... = "kick"`, `staffKindUnban ... = "unban"`, `staffKindUnmute ... = "unmute"` |
| Outcomes | `staff_action_decide.go:24-33` | `staffOutcomePending staffOutcome = iota`, `staffOutcomeDone`, `staffOutcomeSkipped`, `staffOutcomeFailed` |
| Reason codes | `staff_action_decide.go:40-66` | `staffReasonBanned = "banned"`, `staffReasonBannedNotInGroup = "banned_not_in_group"`, `staffReasonMuted = "muted"`, `staffReasonKicked = "kicked"`, `staffReasonUnbanned = "unbanned"`, `staffReasonUnmuted = "unmuted"`, `staffReasonSkipIssuerNotAdmin = "skip_issuer_not_admin"`, `staffReasonSkipIssuerNoRight = "skip_issuer_no_right"`, `staffReasonSkipTargetAdmin = "skip_target_admin"`, `staffReasonSkipTargetBot = "skip_target_bot"`, `staffReasonSkipTargetService = "skip_target_service"`, `staffReasonSkipNotInGroup = "skip_not_in_group"`, `staffReasonSkipLinkRemoved = "skip_link_removed"`, `staffReasonFailLookup = "fail_lookup"`, `staffReasonFailInternal = "fail_internal"`, `staffReasonFailInterrupted = "fail_interrupted"` |
| `staffReasonOutcome` default | `staff_action_decide.go:85` | `return staffOutcomeFailed` (an unlisted reason reads as **failed**) |
| Target state | `staff_action_decide.go:90-97` | `type staffTargetState struct { Status string; IsMember bool; Muted bool; Until int64 }` |
| Calls | `staff_action_decide.go:116-130` | `staffCallNone staffAPICall = iota`, `staffCallBan`, `staffCallMute`, `staffCallKick`, `staffCallUnban`, `staffCallUnmute` |
| Verdict | `staff_action_decide.go:134-137` | `type staffVerdict struct { Call staffAPICall; Reason staffReason }` |
| Unkeyed verdict literals in tests | `staff_action_decide_test.go:42` | `staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}` (adding a field breaks compilation) |
| Callback namespace and codes | `alita/modules/staff.go:42,45-55` | `staffCallbackNamespace = "staff"`; `staffActUnsetConfirm = "uy"`, `"un"`, `staffActUnlinkAsk = "ul"`, `staffActUnlinkConfirm = "uc"`, `staffActUnlinkCancel = "ux"`, `staffActRefresh = "rf"`, `staffActPage = "pg"`, `staffActRunConfirm = "xc"`, `staffActRunCancel = "xn"` |
| Card key / lock key | `staff_action_card.go:32,83` | `staffActionCardPrefix = "alita:staff:act:"`, `staffTargetLockPrefix = "alita:staff:lock:target:"` |
| Card states | `staff_action_card.go:217-224` | `staffCardPending = "pending"`, `"running"`, `"done"`, `"cancelled"`, `"expired"`, `"aborted"` |
| Card TTLs | `staff_action_card.go:211-213` | `staffActionCardGrace = time.Minute`, `staffActionCardDoneTTL = time.Hour` |
| Lock TTL / renew | `staff_action_card.go:86,91` | `staffTargetLockTTL = 30 * time.Minute`, `staffTargetLockRenewEvery = 10 * time.Minute` |
| Pacer | `staff_action_run.go:162-174` | `NextKey: "alita:staff:pace:next"`, `BlockKey: "alita:staff:pace:block"`, `Interval: 100 * time.Millisecond`, `MaxRetries: 3`, `MaxWait: 60 * time.Second`; `func staffPaced(ctx context.Context, call func(context.Context) error) error` |
| Per-call timeout | `staff_action_run.go:33` | `staffActionCallTimeout = 8 * time.Second` |
| Workers | `staff_action_run.go:157` | `const staffActionWorkers = 4` |
| WaitGroup | `staff_action_run.go:35` | `staffActionRunsWG sync.WaitGroup` |
| Panel caps | `alita/modules/staff_panel.go:26,27,33,37` | `maxStaffDisplayTitleRunes = 64`, `maxStaffButtonTitleRunes  = 20`, `staffPanelPageSize = 8`, `staffPanelMaxUTF16 = 3800` |
| Reason cap (Phase 2) | `alita/modules/staff_action_parse.go:18` | `const staffReasonMaxRunes = 200` (the card already caps the reason at 200 runes plus `…`) |
| Telegram until rule | `extraction.go:39-40` | `minTemporaryDurationSeconds int64 = 30`, `maxTemporaryDurationSeconds int64 = 366 * 24 * 60 * 60` |
| Telegram until rule (vendored docs) | gotgbot `gen_methods.go:420,4635` | "If user is banned for more than 366 days or less than 30 seconds from the current time they are considered to be banned forever." and, for restrict, "...restricted forever." |
| Independent permissions flag | gotgbot `gen_methods.go:4632-4634` | `UseIndependentChatPermissions bool`: "Pass True if chat permissions are set independently. Otherwise, the can_send_other_messages and can_add_web_page_previews permissions will imply the can_send_messages, ..." |
| Reply params | gotgbot `gen_types.go:10654-10662` | `ReplyParameters{ MessageId int64; ChatId int64; AllowSendingWithoutReply bool ...}`; `SendMessageOpts.ReplyParameters *ReplyParameters` |
| Edit markup type | gotgbot `gen_methods.go:2352` | `EditMessageTextOpts.ReplyMarkup InlineKeyboardMarkup` (value; zero value removes the keyboard) |
| `ChatPermissions` fields | gotgbot `gen_types.go:2719-2752` | `CanSendMessages, CanSendAudios, CanSendDocuments, CanSendPhotos, CanSendVideos, CanSendVideoNotes, CanSendVoiceNotes, CanSendPolls, CanSendOtherMessages, CanAddWebPagePreviews bool`; `CanReactToMessages *bool`, `CanEditTag *bool`, `CanChangeInfo, CanInviteUsers, CanPinMessages bool`, `CanManageTopics *bool` |
| `MergedChatMember` carries all of them | gotgbot `gen_types.go:2091-2117` | `UntilDate int64`, `IsMember bool`, `CanSendMessages ... CanReactToMessages bool`, `CanEditTag bool` |
| Table names | `alita/db/models/staff.go` | `TableName() ... return "staff_groups"` and `"staff_group_links"` |
| Latest migration | `ls migrations` | `20261004130000_add_staff_role_exclusivity_trigger.sql` |
| `RekeyChat` updates | `alita/db/staff/rekey.go:35-42` | `{&models.StaffGroup{}, "chat_id"}, {&models.StaffGroupLink{}, "staff_chat_id"}, {&models.StaffGroupLink{}, "group_chat_id"}` with `Updates(map[string]any{u.column: newChatID, "updated_at": now})` (so every re-keyed model needs an `updated_at` column) |
| Panel render test hard-fails on unknown actions | `staff_panel_render_test.go:87` | `t.Errorf("button %q carries unexpected action %q", ...)` |
| Harness model lists | `alita/modules/test_harness_test.go:361`, `alita/db/testmain_test.go:78`, `alita/db/staff/testmain_test.go:14` | each lists `&models.StaffGroupLink{}` / `testdb.Run(m, &models.StaffGroup{}, &models.StaffGroupLink{})` |
| Admin category | `alita/db/logchannels/repository.go:21-28` | `CategoryAdmin     = "admin"` |
| Phase 2 delivery fallback | `staff_action_summary.go:509-533` | `deliverStaffActionFinal(b, chatID, msgID, text, continuation, notBefore)` edits the card, else `sendStaffSummaryPart`, then each continuation; `editStaffActionMessage` sends no `ReplyMarkup` |

## Architecture Patterns

### System Architecture Diagram

```
 Staff member ──/ban x──▶ handleStaffAction ──▶ Redis card (pending)           [Phase 2, unchanged]
                                   │ Confirm (issuer only, Lua CAS, target lock)
                                   ▼
                 staffActionConfirm  ── all live checks pass ──┐
                                                               ▼
   NEW ① staff.CreateAction(parent + N pending group rows)  ──fail──▶ abort card "could not check"
                                                               │ ok
                                                               ▼
        startStaffRun (generalized coordinator: lock renew, throttled edits, sweep, deliver)
          │
          ├─ worker × 4 ─▶ per-group chain: link recheck → presser/issuer live rights → target live read
          │                    │ verdict.Call != none
          │                    ▼
          │       NEW ② SavePrior(action, group, prior state)   ──fail──▶ result fail_internal, NO Telegram write
          │                    ▼
          │                executeStaffCall  (staffPaced)
          │                    ▼
          │       progress.set(i, result) ──▶ NEW ③ SaveGroupResult (best effort, bumps parent updated_at)
          │                    ▼ result is ✅
          │       NEW ④ staff log post to that group's admin log channel (staffPaced, never changes the line)
          ▼
   sweepPending ─▶ NEW ⑤ FinalizeAction (one tx: all results + finished_at)
          ▼
   final summary text + NEW Undo button ──edit──▶ card message (else new message, SetSummaryMessage)
                                                          │
 Staff member presses [↩ Undo everywhere] (summary or history detail)
          ▼
   staffUndoAsk: live Staff Group membership, record.staff_chat == this chat, kind != kick,
                 ≥1 done group, not undone ─▶ NEW undo card (Redis, action="undo", undo_of=<id>)
                                              sent as a NEW message replying to the original summary
          ▼ Confirm (presser only)  ── target lock ──▶ card CAS ──▶ live checks
   NEW ⑥ staff.ClaimUndo (UPDATE ... WHERE undo_started_at IS NULL, RowsAffected==1) ─no▶ "already undone by X"
          ▼
   original summary edited: buttons removed + "↩ Undone by Name, see reply"   (only if summary_chat_id == staff chat)
          ▼
   startStaffRun again, per-group = undo chain:
        recheckLink → presser live rights → target live read → decideStaffUndo(orig, prior, leftBehind, live, now)
        → executeStaffUndoCall (staffPaced) → SaveUndoResult → undo log post → FinalizeUndo
```

### Recommended Project Structure
```
migrations/20261005120000_add_staff_actions.sql      # new (timestamp > 20261004130000)
alita/db/models/staff_action.go                      # StaffAction, StaffActionGroup, outcome constants
alita/db/staff/actions.go                            # repository (same package: RekeyChat needs it)
alita/db/staff/rekey.go                              # + staff_actions.staff_chat_id
alita/modules/staff_action_decide.go                 # + prior/left-behind types, decideStaffUndo, new reasons
alita/modules/staff_action_record.go                 # new: prior capture, record writes, card<->record mapping
alita/modules/staff_action_run.go                    # generalized run + shared gate helper
alita/modules/staff_action_summary.go                # header param, markup-capable edit/send, undo lines
alita/modules/staff_undo.go                          # new: ask / confirm / cancel glue / undo chain / original-edit
alita/modules/staff_history.go                       # new: recent-actions list, detail, paging
alita/modules/staff_log.go                           # new: log post composition + paced send
alita/utils/actionlog/actionlog.go                   # + Destination seam (Log keeps its behaviour)
alita/modules/staff.go                               # + callback actions
alita/modules/staff_panel.go                         # + "Recent actions" button
locales/*.yml (7)                                    # new keys + staff_help_msg line
```

### Pattern 1: Audit schema (two tables)

**What:** parent `staff_actions` plus child `staff_action_groups`, created in one transaction at Confirm.

**Migration skeleton** (constraints inline in `CREATE TABLE`: the runner rewrites `ALTER TABLE ... ADD CONSTRAINT` into DO blocks, `runner.go:~540-590`; follow the Phase 1 migration's style):

```sql
-- Staff action audit record: one row per confirmed staff action, one row per linked group.
-- No foreign keys on chat IDs (federation_chats / staff_group_links precedent). The child
-- references its parent with a real FK because both rows are written by the same transaction.
CREATE TABLE IF NOT EXISTS staff_actions (
    id BIGSERIAL PRIMARY KEY,
    staff_chat_id BIGINT NOT NULL,          -- re-keyed by staff.RekeyChat
    issuer_user_id BIGINT NOT NULL,
    issuer_name TEXT NOT NULL DEFAULT '',
    target_user_id BIGINT NOT NULL,
    target_name TEXT NOT NULL DEFAULT '',
    action VARCHAR(8) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    duration_sec BIGINT NOT NULL DEFAULT 0,     -- as typed, 0 = permanent
    duration_amount BIGINT NOT NULL DEFAULT 0,
    duration_unit VARCHAR(1) NOT NULL DEFAULT '',
    over_limit BOOLEAN NOT NULL DEFAULT FALSE,
    until_date BIGINT NOT NULL DEFAULT 0,       -- the one value sent to every group (0 = permanent)
    group_count INTEGER NOT NULL DEFAULT 0,
    summary_chat_id BIGINT NOT NULL DEFAULT 0,  -- NEVER re-keyed: where summary_msg_id lives
    summary_msg_id BIGINT NOT NULL DEFAULT 0,
    finished_at TIMESTAMP WITH TIME ZONE,
    undo_by BIGINT,
    undo_by_name TEXT NOT NULL DEFAULT '',
    undo_started_at TIMESTAMP WITH TIME ZONE,
    undo_finished_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT chk_staff_action_kind CHECK (action IN ('ban', 'mute', 'kick', 'unban', 'unmute'))
);
CREATE INDEX IF NOT EXISTS idx_staff_actions_staff_chat_id_id ON staff_actions(staff_chat_id, id);

CREATE TABLE IF NOT EXISTS staff_action_groups (
    id BIGSERIAL PRIMARY KEY,
    action_id BIGINT NOT NULL REFERENCES staff_actions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,                       -- link order at Confirm
    group_chat_id BIGINT NOT NULL,              -- NOT re-keyed (the restriction lives in that chat)
    group_title TEXT NOT NULL DEFAULT '',       -- title at action time
    outcome VARCHAR(8) NOT NULL DEFAULT 'pending',
    reason VARCHAR(40) NOT NULL DEFAULT '',     -- staffReason code, deliberately NOT a CHECK list
    detail TEXT NOT NULL DEFAULT '',            -- already HTML-escaped Telegram text, <= 120 runes
    -- Pre-action state (D-02, written BEFORE the Telegram write):
    prior_status VARCHAR(16) NOT NULL DEFAULT '',   -- member|left|restricted|kicked, '' = not captured
    prior_is_member BOOLEAN NOT NULL DEFAULT FALSE,
    prior_until BIGINT NOT NULL DEFAULT 0,
    prior_permissions TEXT NOT NULL DEFAULT '',     -- JSON of gotgbot.ChatPermissions, '' unless restricted
    applied_at TIMESTAMP WITH TIME ZONE,            -- set when outcome = done
    -- Per-group undo result (same row, D-09 allows one undo per action):
    undo_outcome VARCHAR(8) NOT NULL DEFAULT '',
    undo_reason VARCHAR(40) NOT NULL DEFAULT '',
    undo_detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(action_id, group_chat_id),
    CONSTRAINT chk_staff_action_group_outcome CHECK (outcome IN ('pending', 'done', 'skipped', 'failed')),
    CONSTRAINT chk_staff_action_group_undo_outcome CHECK (undo_outcome IN ('', 'pending', 'done', 'skipped', 'failed'))
);
```

Notes for the planner:
- **Why per-group rows, not JSONB:** results are written as groups finish (restart truthfulness), counted in SQL for the history tally, and SQLite tests run the same shape through `AutoMigrate`.
- **Do not declare the FK in the GORM model.** Keep it SQL-only (as the chat-ID columns are), so SQLite `AutoMigrate` needs no relation setup. No repository code relies on the cascade (nothing deletes audit rows).
- **GORM tags must mirror the SQL** (`TableName()` returns `"staff_actions"` / `"staff_action_groups"`), including `check:` tags like `StaffGroupLink` carries.
- **Do not CHECK the reason column.** New reason codes must not need a migration; only `action` and `outcome` are stable small enums.
- **`prior_status` values:** store gotgbot's own status strings (`member`, `left`, `restricted`, `kicked`); `''` means "not captured" and is **never undoable**.
- Prior write is per group; the parent holds the single `until_date` (`card` stores duration, not an end date; `until_date` is computed once at Confirm, `staff_action_card.go:842-854`), so there is nothing per group to store for "what was applied" except `applied_at`.
- Times are `TIMESTAMP WITH TIME ZONE`, `*time.Time` in the models for nullable ones.
- **No cache keys** are read or written for these tables.

### Pattern 2: Record lifecycle hooks (exact seams)

| Step | Where (verified this session) | Change |
|------|-------------------------------|--------|
| Create parent + N pending rows | `staffActionConfirm`, right before the `editStaffActionMessage(... pendingResults(links))` at `staff_action_card.go:856`. Everything that can still abort (`GetStaffGroupFresh`, membership, links, signature, `TemporaryUntilDate`) has passed by then. | `staff.CreateAction(...)`; on error call `abortStaffActionCard(... "staff_act_abort_check_failed")` so **no action is applied without a record** (STAFF-10). Put the new `ActionID` on the in-memory `card` (not in Redis). Issuer name is `staffFullName(&query.From)` (Confirm is issuer-only). `summary_chat_id = staffChat.Id`, `summary_msg_id = msgID`. |
| Capture prior state | `runStaffActionInGroup` between the `verdict := decideStaffAction(...)` at `staff_action_run.go:389` and `executeStaffCall` at `:396`, only when `verdict.Call != staffCallNone`. The live `target` (`gotgbot.MergedChatMember`) is already in hand. | `prior := staffPriorFromMember(target)`; `staff.SavePrior(...)`; on error return `result(staffReasonFailInternal, "")` **without** the write (fail closed). |
| Per-group result | The worker closure in `runStaffActionFanOut` (`staff_action_run.go:315-318`: `progress.set(i, runStaffActionInGroup(...))`). | After `progress.set`, `staff.SaveGroupResult(...)` (best effort: log at Error, keep going; the final flush is authoritative). Set `applied_at` when the outcome is done. Bump the parent's `updated_at` in the same transaction (it is the heartbeat). |
| Log post | Same closure, **after** `progress.set` so the ✅ line is visible before the post's pacing wait. | `if res.Outcome == staffOutcomeDone { postStaffActionLog(...) }`; never alters `res`. |
| Authoritative finalize | The coordinator after `progress.sweepPending(sweep)` at `staff_action_run.go:276`, **before** `deliverStaffActionFinal`. | `staff.FinalizeAction(actionID, results)`: one transaction updating every child (outcome, reason, detail) and setting `finished_at`. Idempotent. One retry on error, then log at Error. |
| Undo button + summary message ID | `deliverStaffActionFinal` (`staff_action_summary.go:509`). | Add a markup parameter; the first message keeps the button (see Pattern 6). If the final edit fell back to a new message, `staff.SetSummaryMessage(actionID, chatID, newMsgID)`. |
| Restart truthfulness | `StopStaffActions` needs no change: the coordinator still runs `sweepPending` → finalize → deliver (it cancels `staffActionsCtx`; the DB writes must **not** use that context). `FinalizeAction` runs while the DB is still open (Staff drain is registered after DB-close, `main.go:216-219`). | A hard crash leaves `finished_at IS NULL`; render such a record as running while `updated_at` is under 30 min old (`staffTargetLockTTL`), else "interrupted, no final result recorded". Such a record gets **no Undo button**; its already-✅ groups stay in the DB. |

**Why write-ahead for the prior state:** the only moment the prior state exists is between the live read and the write call. A crash after the call but before a post-call DB write would apply a ban that can never be restored correctly. Writing the prior first costs one indexed UPDATE per applied group; the cost of the alternative is an unrestorable action.

**Known residual hole (accept and document):** a crash between the Telegram write and `SaveGroupResult` leaves the child at `pending` with prior set. It renders as interrupted and is not undoable; staff fix it by hand. (Matches CONTEXT: interrupted lines are not undoable.)

### Pattern 3: Prior state, left-behind state and the undo decision table

**Capture type (new, in `staff_action_decide.go`):**

```go
// staffPriorState is the target's state in one group before a staff action, as one
// live getChatMember read reported it. It is what undo puts back (D-02).
type staffPriorState struct {
	Status   string                 // gotgbot status string; "" = not captured
	IsMember bool
	Until    int64                  // end of the ban or restriction, 0 = permanent
	Perms    *gotgbot.ChatPermissions // only for Status == restricted
}
```

`staffPriorFromMember(m gotgbot.MergedChatMember)` builds it from the same `MergedChatMember` that `staffTargetStateFrom` reads. Convert the three pointer fields (`CanReactToMessages`, `CanEditTag`, `CanManageTopics`) with `helpers.Ptr` (AGENTS.md: use `helpers.Ptr[T]` for option pointers).

**Left-behind state** (what the staff action must have left, derived deterministically from `kind`, the parent's `until_date` and the child's `applied_at`; no extra Telegram call):

| Original ✅ reasons | Live must match (else `skip_changed_since`) |
|---|---|
| `ban` (`banned`, `banned_not_in_group`) | `Status == kicked` and `Until` equals the record's `until_date`; **also accept** live `Until == 0` when `until_date != 0` and `until_date - applied_at < 30 s` (Telegram stores a sub-30-second ban as permanent). |
| `mute` (`muted`) | `Status == restricted`, `IsMember`, `Muted`, and `Until` rule as for ban. |
| `unban` (`unbanned`) | `Status == left`. A target who has rejoined (`member`) differs and is skipped. |
| `unmute` (`unmuted`) | not `Muted`, not `kicked`, not admin: `member`, `left`, or `restricted` with `!Muted`. (After a restrict with default permissions Telegram may report any of these.) |
| `kick` | no undo. |

**Verdict matrix for `decideStaffUndo(orig, prior, appliedUntil, appliedAt, live, now)`** (creator/administrator live status first: always `skip_target_admin`, as in `decideStaffAction`):

| Original | Prior (recorded) | Live matches left-behind? | Verdict |
|---|---|---|---|
| any | `Status == ""` | any | skip `skip_no_prior_state` |
| any | any | no | skip `skip_changed_since` |
| ban | member, left | yes | `staffCallUnban` → `undone_unbanned` |
| ban | restricted, ended (`Until != 0 && Until < now+margin`) | yes | `staffCallUnban` → `undone_unbanned` (lifted: the restriction had already run out) |
| ban | restricted, still on | yes | restore: restrict with `Perms`, `Until` → `undone_restriction_restored` |
| ban | kicked (shorter ban D-13), ended | yes | `staffCallUnban` → `undone_unbanned` |
| ban | kicked, still on | yes | `staffCallBan` with `Until = prior.Until` → `undone_ban_restored` |
| mute | member | yes | `staffCallUnmute` → `undone_unmuted` |
| mute | restricted, ended | yes | `staffCallUnmute` → `undone_unmuted` |
| mute | restricted, still on | yes | restore: restrict with `Perms`, `Until` → `undone_restriction_restored` |
| unban | kicked, still on or permanent | yes | `staffCallBan` with `Until = prior.Until` → `undone_ban_restored` |
| unban | kicked, ended | yes | skip `skip_restriction_ended` |
| unmute | restricted, still on or permanent | yes | restore: restrict with `Perms`, `Until` → `undone_restriction_restored` |
| unmute | restricted, ended | yes | skip `skip_restriction_ended` |
| not applied (child outcome != done) | n/a | n/a | skip `skip_not_applied` (decided before any Telegram read) |

`margin` is `staffUndoMinRemaining = 120 * time.Second`: a prior end date closer than that is treated as ended, never re-applied (Pitfall 3).

**Skeleton:**

```go
// staffCallRestore is restrictChatMember with a recorded permission set and end date.
// Only decideStaffUndo ever returns it. (new)
const staffCallRestore staffAPICall = staffCallUnmute + 1

// staffUndoVerdict wraps staffVerdict instead of adding fields to it: the Phase 2
// tests build staffVerdict with unkeyed literals, so that type must not change. (new)
type staffUndoVerdict struct {
	staffVerdict
	Until int64                    // ban or restore end date
	Perms gotgbot.ChatPermissions // restore only
}

func decideStaffUndo(orig staffActionKind, prior staffPriorState, live staffTargetState,
	applied staffAppliedState, now int64) staffUndoVerdict { /* matrix above */ }
```

**The one sanctioned exception to the Phase 2 invariants:** `staffCallRestore` and `staffCallBan` may be sent to a **kicked or left** target. Rule: only when `live` matches the left-behind state (the staff's own action), never otherwise. `TestStaffActionDecisionInvariants` loops `decideStaffAction` only and stays valid unchanged; add a sibling `TestStaffUndoDecisionInvariants` that proves: (a) a kicked live target gets a call only when it equals the left-behind state; (b) no call to an administrator or creator; (c) `staffCallUnban` only to a kicked target; (d) a restore never carries an `Until` inside the margin; (e) done reasons stand exactly for made calls (invariant 6 of the Phase 2 test).

**Execution:** `executeStaffUndoCall` delegates `staffCallBan`, `staffCallUnban`, `staffCallUnmute` to the existing `executeStaffCall(ctx, b, group, target, verdict.staffVerdict, until)` (its `newUntil` parameter already carries the ban end date) and handles `staffCallRestore` itself:

```go
_, err := b.RestrictChatMemberWithContext(callCtx, groupID, targetID, v.Perms,
	&gotgbot.RestrictChatMemberOpts{UseIndependentChatPermissions: true, UntilDate: v.Until})
```

Wrap in the same `paced` closure so a retry carries identical parameters.

### Pattern 4: Undo flow (reusing the Phase 2 card)

1. **Ask** (`a=ya&r=<recordID>`): refuse unless the presser is a **live** member of the Staff Group (`chat_status.IsUserInChatWithError`, as `staffPanelRebuild`/`staffActionConfirm` do). Load the record with `staff.GetActionFresh`; require `record.StaffChatID == query.Message chat id` (the chat comes from the message, never from the data), `action != kick`, `finished_at != NULL`, at least one `done` child, `undo_started_at IS NULL` (else answer "already undone by Name"). Create the card with `saveStaffActionCard`: `action="undo"`, plus two new hash fields `undo_of` (record ID) and `undo_kind` (original kind); `issuer` = the presser, so the existing Lua `transitionStaffCardScript` already enforces "only the person who pressed Undo can confirm". `group_count` = number of `done` children (N). Send it as a **new message** replying to the original summary (`SendMessageOpts.ReplyParameters{MessageId: record.SummaryMsgID, AllowSendingWithoutReply: true}`, **only** when `record.SummaryChatID == staffChat`; otherwise send without `ReplyParameters`). Call `scheduleStaffActionExpiry` for it.
2. **Confirm/Cancel buttons** use **new** action codes (suggest `yc`, `yn`; the codes `uy un ul uc ux rf pg xc xn` are taken). New codes mean a replica still running the old code answers `staff_cb_expired` for an undo button and can never mistake an undo card for a Phase 2 card. `staffActionCancel` can be reused as is once `staffActionHeader` knows how to render an undo card (branch on `card.UndoOf != 0`); expiry, "already handled" and issuer-only answers also come for free.
3. **Confirm** (`staffUndoConfirm`): same order as `staffActionConfirm`: load card; `takeStaffTargetLock(b, query, tr, card)` (same lock, keyed by target); `transitionStaffActionCard(... staffCardRunning, true)`; `GetStaffGroupFresh`; presser live in Staff Group; fresh record re-check; **then the DB claim as the last step before the run starts** so nothing can fail between claim and run: `staff.ClaimUndo(recordID, presserID, presserName)` is `UPDATE staff_actions SET undo_by=?, undo_by_name=?, undo_started_at=now(), updated_at=now() WHERE id=? AND undo_started_at IS NULL`, success iff `RowsAffected == 1`. On a lost claim abort the card with "already undone by Name" (read the winner from the row). Do **not** compare `links_sig` (the undo set is the record's done groups, and every group is rechecked live anyway).
4. **Original summary:** after the claim, edit the original (only if `summary_chat_id == staff chat`) with the re-rendered final text plus the "↩ Undone by Name, see reply" line and **no** keyboard; a failure is logged and the undo proceeds (D-08).
5. **Run:** the generalized run (below) over the record's groups. Show only the **originally ✅ groups** as lines; put "N other groups were not changed by the original action" in the **header** instead of N skipped lines (D-03 allows "listed by the planner's choice"; this keeps the 97-groups-skipped case readable and drops nothing silently).
6. **Finalize:** `staff.FinalizeUndo(recordID, results)` writes each child's `undo_outcome/reason/detail` and `undo_finished_at`. No Undo button on the undo summary (an undo cannot be undone).

**Button visibility rules** for the summary and the detail view: kind is not kick; `finished_at` set; at least one `done` child; `undo_started_at` NULL. The summary button only exists on the final text, so "Undo while the original run is still going" is impossible by construction; the detail view must test `finished_at` for the same reason.

**Anonymous admin note (D-06):** a callback query always carries the real pressing user in `query.From`, so the "post as yourself" refusal that guards the **command** cannot be reached from a button `[ASSUMED]`. Add a cheap defensive check anyway: reject `staffServiceUserIDs[query.From.Id]` with the existing `staff_post_as_yourself` text.

### Pattern 5: What to generalize versus duplicate

| Piece | Verdict | How |
|---|---|---|
| Coordinator in `startStaffActionRun` (lock renewal, throttled edits, hold on 429, sweep, final deliver, `defer releaseStaffTargetLock`) | **Generalize.** Copying ~100 lines of delicate timing logic would drift. | Extract `startStaffRun(b, spec)`. `spec` carries: the card (token, target, staff chat for the lock and translator), the links/groups, a per-group function `func(ctx, i int) staffGroupResult`, a render function `(results, final) (text, continuation)`, the message to edit, an `onFinal(results)` hook (DB finalize, Undo button), and an `afterGroup(i, res)` hook (DB result + log post). Phase 2's call becomes one caller. The Phase 2 progress/429/shutdown tests are the regression net. |
| Per-group check chain prefix (`interrupted`, staff-group guard, `recheckLink`, live actor rights, bot/service target guards, live target read) | **Generalize.** | Extract the chain up to the verdict into one helper taking the *actor* (issuer for runs, presser for undo). The order is a safety contract (comment at `staff_action_run.go:323-325`). |
| `composeStaffActionSummary` | **Generalize minimally.** It builds `header := staffActionHeader(tr, card)` internally. | Add `composeStaffSummary(tr, header string, results, final)` and make the old function a wrapper. All overflow rules (collapse done lines, never drop skipped/failed, continuation messages) are then shared by the original summary, the undo summary and the detail view. |
| `staffActionHeader` | **Branch.** | `if card.UndoOf != 0 { return staffUndoHeader(...) }` so Cancel/Expired/Abort edits work for undo cards untouched. |
| `editStaffActionMessage`, `deliverStaffActionFinal`, `sendStaffNotice` | **Add markup-capable variants.** Today `editStaffActionMessage` never sets `ReplyMarkup` (so every edit strips buttons) and `sendStaffNotice` discards the sent message. | New `editStaffActionMessageWithMarkup`; `sendStaffNotice` variant returning `*gotgbot.Message`. `deliverStaffActionFinal` gains `markup` and returns the message ID where the first final text landed. |
| Card Redis model and Lua scripts | **Reuse unchanged**, add two optional hash fields. | `saveStaffActionCard` writes `undo_of`/`undo_kind`; `loadStaffActionCard` reads them as optional (like the duration fields at `staff_action_card.go:425-445`). |
| Target lock helpers, `takeStaffTargetLock`, pacer, `fetchLiveMember`, `staffIssuerSkipReason`, `classifyStaffFailure` | **Reuse unchanged.** | — |
| `decideStaffAction` | **Do not change.** | Add `decideStaffUndo` beside it. |

### Pattern 6: Final-summary edit with the Undo button

```go
// skeleton: new variant, same options as editStaffActionMessage plus the keyboard.
_, _, err := b.EditMessageTextWithContext(ctx, &gotgbot.EditMessageTextOpts{
	ChatId: chatID, MessageId: msgID, Text: text,
	ParseMode:          formatting.HTML,
	LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
	ReplyMarkup:        markup, // zero value = no buttons
})
```

Build the button with `encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActUndoAsk, "r": strconv.FormatUint(uint64(recordID), 10)})` and check for `""` (leave the button out and log, never ship it dead). Typical size: `staff|v1|a=ya&r=1234567890` is 26 bytes.

### Pattern 7: Log-channel posts

- **Seam in `actionlog`:** `Log` mixes destination lookup and send, and sends with `helpers.SendMessageWithErrorHandling` directly. Split it: `actionlog.Destination(chat *gotgbot.Chat, category string) (channelID int64, header string, ok bool)` (the existing `chat.Type == "channel"`, `logchannels.Get` and `CategoryEnabled` logic plus the `<b>title</b> (<code>id</code>)` header), and keep `Log` calling it so every current caller is byte-identical. The staff module builds a `*gotgbot.Chat{Id: link.GroupChatID, Title: link.GroupTitle, Type: "supergroup"}` and sends **inside `staffPaced`**:

```go
err := staffPaced(ctx, func(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, staffActionCallTimeout)
	defer cancel()
	_, err := b.SendMessageWithContext(callCtx, channelID, header+body, &gotgbot.SendMessageOpts{ParseMode: formatting.HTML})
	return err
})
```

  `helpers.SendMessageWithErrorHandling` also swallows permission errors and marks a chat restricted; if that behaviour is wanted, call it **inside** the paced closure. Its `errors.Wrapf` wrapper has `Unwrap` (`alita/utils/errors/errors.go:21`), so the pacer still sees a wrapped 429.
- **Language:** the post is read by the linked group's own admins: use `staffChatTranslator(link.GroupChatID)` (same helper Phase 2 uses for the Staff Group, `staff_notify.go:24`).
- **Body (D-11):** hashtag (not localized; `#STAFF_BAN`, `#STAFF_MUTE`, `#STAFF_KICK`, `#STAFF_UNBAN`, `#STAFF_UNMUTE`, `#STAFF_UNDO`), `Admin: ` + `formatting.MentionHtml(issuerID, issuerName)`, `User: ` + the existing `staffTargetDisplay` shape (`<b>name</b> (<code>id</code>)`), duration via `staffDurationLabel` (ban and mute only), the reason, and the localized "via Staff Group" marker. Build by concatenation with user text spliced in after translation (the translator runs a printf-style pass; see `staffGroupsToken` comments). The Staff Group's title and ID must not appear anywhere in it: **assert that in a test** by scanning the sent text for the Staff Group's chat ID string and title.
- **Reason (Pitfall A):** cap **before** escaping so an entity is never cut: `html.EscapeString(capRunes(reason, 300))`. Phase 2 already caps the stored reason at 200 runes (`staffReasonMaxRunes`), so 300 is a defensive second cap for rows read back from the database. An empty reason uses the existing `staff_act_no_reason` text.
- **Where:** in the worker closure after `progress.set`, so the ✅ line is already visible before the post waits for its pacing slot. A post's failure (including `ErrRateLimited` from `MaxWait`) is `log.Warnf` and nothing else; it never touches `res`.
- **Shutdown:** a run cancelled by `StopStaffActions` skips remaining posts (`ctx.Err() != nil`), so a group applied just before shutdown may have no post. Accept and document (see Open Questions).
- **Undo posts (D-13):** same function, undo variant: `#STAFF_UNDO · <reverse action name> · Admin: <presser> · User: ... · undoes <original action name> by <issuer> · via Staff Group`. The reverse names reuse `staffActionName` through a pure `staffReverseKind` (ban↔unban, mute↔unmute) so no new action-name keys are needed.

### Pattern 8: History view (`/staff` → Recent actions)

- **Callbacks** under the existing namespace, all with short numeric fields: `a=rc&o=<offset>` (list), `a=dt&r=<id>&o=<offset>` (detail, `o` is where Back returns to), `a=ya&r=<id>` (undo ask). "Back to links" reuses `a=rf&p=0` (existing Refresh handler rebuilds the panel). Worst-case size is still far under 64 bytes (`staff|v1|a=dt&r=9999999999&o=999999` = 35 bytes). Add a test that builds every history keyboard with the largest IDs and asserts every `CallbackData` is 1..64 bytes and never `""` (Pitfall 19).
- **Authority:** `GetStaffGroupFresh(chat.Id)` for the message's chat, then a **live** membership check of the presser, exactly as `staffPanelRebuild` does (`staff_panel.go:498-541`). Query only `staff_chat_id = this chat`. The history view makes **no** Telegram calls to linked groups (unlike the links panel, which does live health checks); it is one membership call plus DB reads.
- **Paging by offset, not page number:** the page holds 10 entries, but a page whose text exceeds the cap must shrink. If the page number were the cursor, a shrunken page would hide entries. With an `offset` cursor, Next is `offset + shown`. Truncate hard first so 10 normally fit: name 16 runes, reason 24 runes, issuer 16 runes (`staffDisplayTitle` caps at 64 runes, too long for ten lines), and use `fitStaffSummaryLines(head, lines, tail, atLeastOne)` (`staff_action_summary.go:254`) as the safety net, measured in UTF-16 on the escaped HTML against `staffPanelMaxUTF16` (3800). Worst case with escape-only characters (`"`, `&` become 5-6 units) is about 350 units per line, which is why the loop is still needed.
- **Tally per line without N+1:** one query `SELECT action_id, outcome, COUNT(*) FROM staff_action_groups WHERE action_id IN ? GROUP BY action_id, outcome`.
- **Detail view:** header (`composeStaffSummary` with the original header) plus every group line in `seq` order, undo results below under an "Undone by Name, <date>" line. A group whose `group_chat_id` is not in `ListLinksByStaffFresh(staffChat)` is marked "no longer linked" using the stored title. The detail message is one edited message: use the non-final compose path (collapses done lines, keeps every skipped and failed line) and, if it still does not fit, an explicit "N more groups not shown" note (new key). The Undo button follows the visibility rules above.
- **Date format:** `5 Oct 12:04` in UTC like the panel's `Updated {time} UTC`; use the record's `created_at`.
- **Panel button:** `renderStaffPanel` gets one extra row (`a=rc&o=0`, label key new). `renderStaffPanel` is pure and the Phase 9 menu reuses it, so keep it I/O-free. **Update `panelSplitKeyboard`** in `staff_panel_render_test.go:61-90`: its `default:` branch calls `t.Errorf` on any unknown action, so the new button fails every render test until the helper learns it.

### Anti-Patterns to Avoid
- **Calling Telegram from the undo path outside `decideStaffUndo`/`executeStaffUndoCall`.** The invariant "the decision lives in one table" is what Phase 2's security story rests on.
- **Using `cache.GetAdminCacheUser`, `IsUserAdmin` or any `CheckFunc` for the presser.** Live `getChatMember` only (Pitfall 1).
- **Treating `staffReasonOutcome`'s default as harmless.** An unlisted reason maps to **failed** (`staff_action_decide.go:85`). Every new reason must be added to `staffReasonOutcome` and to `staffReasonText` (which returns `""` for unknown reasons, giving a blank line).
- **Editing an undo into the original summary.** D-08: the original stays as history; the result is a new message.
- **Storing user text in callback data or the Redis undo card beyond what Phase 2 already stores.** Callbacks carry only a numeric record ID or a token.
- **Writing audit rows with `context.WithCancel` contexts derived from `staffActionsCtx`.** On shutdown that context is cancelled first; the finalize write must still succeed.
- **Re-keying `summary_msg_id` (or editing by it) without checking `summary_chat_id`** (Pitfall 5).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| Exactly-once undo | A Redis flag or an in-process mutex | Conditional `UPDATE ... WHERE undo_started_at IS NULL`, `RowsAffected == 1` | Survives card expiry and replicas; the repo already does this (`SetLinkHealth`, `DeleteLinkIfOwner`). |
| Confirm/Cancel/expiry for undo | A second card implementation | `saveStaffActionCard` + `transitionStaffActionCard` + `scheduleStaffActionExpiry` | The Lua scripts already give issuer-only, expiry and exactly-once. |
| Fan-out pacing for log posts | A per-replica `time.Ticker` | `staffPaced` | AGENTS.md: per-replica limiters must not be used for shared budgets. |
| Summary overflow | New collapsing rules | `composeStaffActionSummary` rules via the header-parameterized wrapper | "Skipped and failed lines are always listed" is a tested guarantee. |
| UTF-16 length and fit | `len(string)` | `staffSummaryLen`, `fitStaffSummaryLines` | Telegram's cap is in UTF-16 units on the escaped HTML. |
| Callback encoding | `fmt.Sprintf("...%d")` / `strings.Split` | `encodeCallbackData` / `decodeCallbackData` | 64-byte cap, `""` on overflow. |
| Duration labels | New strings | `staffDurationLabel`, `staffTargetDisplay` | Existing localized keys, already escaped. |
| Per-group authority | Admin cache | `fetchLiveMember` + `staffIssuerSkipReason` | Pitfall 1. |
| Chat-migration handling | A new watcher | Add one row to `RekeyChat`'s `updates` table | Already called from both migrate service messages and the 400 parameter. |

**Key insight:** the risk in this phase is not novel algorithms; it is that undo is a second path to the same dangerous Telegram calls. Every shortcut that skips the decision table, the live rights read or the pacer re-opens a hole Phase 2 closed.

## Runtime State Inventory

Not a rename/refactor/migration phase. The only state-shape change is additive: two new tables and two optional fields on the Redis card hash. Omitted per the template. (One related fact: pending Phase 2 cards in Redis at deploy time have no `undo_of` field and load as normal cards, because the reader treats it as optional.)

## Common Pitfalls

### Pitfall 1: `restrictChatMember` replaces the entire permission set
**What goes wrong:** restoring "restricted until T" with only an end date, or with `MutedPermissions`, silently changes what the person may do. A prior "can send text but no media" restriction becomes "muted", or a prior mute becomes the group default.
**Why:** the Bot API restrict call sets all permissions at once; `ChatMemberRestricted` exposes every `can_*` flag, and gotgbot's `MergedChatMember` carries them all (`gen_types.go:2095-2117`).
**How to avoid:** capture `prior_permissions` (JSON of `ChatPermissions`) with the prior state; restore with `UseIndependentChatPermissions: true`, otherwise `can_send_other_messages`/`can_add_web_page_previews` imply the send permissions (`gen_methods.go:4633`).
**Warning signs:** a restore test that only asserts `until_date`; a prior-state column set with no permissions.

### Pitfall 2: Restoring "member" is impossible
**What goes wrong:** D-02 says undo puts "member" back. A banned member who was in the group is no longer in it; the only restoring call is `unbanChatMember(only_if_banned=true)` which leaves them `left`, and they must rejoin. `[ASSUMED]` (matches the stateful fake: `only_if_banned` on kicked → `left`).
**How to avoid:** treat `member` and `left` priors identically (unban), and word the done line "unbanned (can rejoin)". Do not promise re-adding. Same for a restricted prior member after a ban: the restriction is restored on the kicked user (they come back restricted when they rejoin).

### Pitfall 3: Telegram's 30-second rule and retries make a restore permanent
**What goes wrong:** `until_date` within 30 s of the call is "forever" (`gen_methods.go:420,4635`). Re-banning or restoring with a prior end date that is 40 s away becomes permanent if the call is delayed by pacing or a 429 wait (up to 60 s) and the closure is retried with the same `until`.
**How to avoid:** `staffUndoMinRemaining = 120 s`; anything closer is treated as ended and lifted or skipped. Test the boundary on both sides. **Related Phase 2 latent issue (not this phase's to fix, flag to the owner):** `/ban 1m` computes `until = now+60` at Confirm; a group reached more than 30 s later becomes a permanent ban at Telegram. The undo comparison must therefore accept live `Until == 0` where the applied end date was under 30 s after `applied_at`.

### Pitfall 4: The undo lifts the staff's own ban, which Phase 2 forbids everywhere else
**What goes wrong:** restoring a prior restriction sends `restrictChatMember` to a kicked target, the exact combination the Phase 2 invariants forbid. A naive "allow it for undo" widens the hole.
**How to avoid:** allowed only when the live state equals the left-behind state (D-04), expressed once in `decideStaffUndo` and covered by its own invariant test (Pattern 3). Never reuse `decideStaffAction` to do it.

### Pitfall 5: Message IDs are only meaningful in the chat they were sent to
**What goes wrong:** `staff.RekeyChat` re-keys the Staff Group's chat ID. The stored `summary_msg_id` then names a message in the old chat; after migration the new supergroup has its own ID sequence `[ASSUMED]`, so the same number can be an unrelated bot message in the new chat. D-08's "edit the original" or `ReplyParameters` would hit the wrong message.
**How to avoid:** store immutable `summary_chat_id`; `RekeyChat` updates `staff_actions.staff_chat_id` only; edit and reply only when `summary_chat_id == the staff chat of this callback`, otherwise treat the target as gone and send a plain message (D-08 already specifies that fallback). Test: re-key then undo.

### Pitfall 6: `RekeyChat` now touches a new table
**What goes wrong:** adding `{&models.StaffAction{}, "staff_chat_id"}` to `RekeyChat` makes every existing re-key test (`alita/db/staff/rekey_test.go`, `alita/modules/staff_migrate_test.go`) fail with "no such table" in any harness that did not `AutoMigrate` the new models.
**How to avoid:** add `&models.StaffAction{}, &models.StaffActionGroup{}` to all three lists in the same task as the `rekey.go` change: `alita/db/staff/testmain_test.go:14`, `alita/modules/test_harness_test.go:361`, `alita/db/testmain_test.go:78` (AGENTS.md "Adding a module" step 4). The group rows are **not** re-keyed (`group_chat_id` names the chat the restriction lives in; a migrated linked group no longer matches any link, so undo skips it with `skip_link_removed`).

### Pitfall 7: Unlisted reasons read as failures and blank lines
`staffReasonOutcome` defaults to failed and `staffReasonText` returns `""` for unknown codes. Every new reason (`undone_*`, `skip_not_applied`, `skip_changed_since`, `skip_restriction_ended`, `skip_no_prior_state`) needs both switch arms and a locale key in all 7 files. Add a test that iterates a list of **all** reason constants and asserts a non-empty text for every non-done-plain reason.

### Pitfall 8: Adding a field to `staffVerdict` breaks compilation
Phase 2 tests use unkeyed `staffVerdict{staffCallNone, staffReasonSkipTargetAdmin}` (`staff_action_decide_test.go:42` and ~45 more rows). Use the embedding wrapper `staffUndoVerdict` (Pattern 3).

### Pitfall 9: Rolling deploy across replicas
Replicas share Redis and the DB. A replica on old code that loads an undo card (same `alita:staff:act:` hash) and sees `action="ban"` would run a real ban. Mitigations built into the design: undo cards use `action="undo"` (old code's `decideStaffAction` default returns `staffReasonFailLookup`, so every group fails harmlessly) **and** new callback action codes (old code answers `staff_cb_expired`). The new tables only matter to new code. Note it in the plan; the deploy manifests already run `AUTO_MIGRATE=true`.

### Pitfall 10: The final-edit fallback moves the summary
`deliverStaffActionFinal` falls back to a **new** message when the card edit fails. The Undo button must sit on whichever message holds the first final text, and `summary_msg_id` must follow it (`SetSummaryMessage`). Otherwise D-08's "edit the original" targets a message that has no button and "↩ Undone" is never shown on the live one.

### Pitfall 11: Test-only traps
- `staffCleanup` (`staff_helpers_test.go:263`) deletes only staff groups and links: extend it to delete `staff_actions` (and children) for the test's chat IDs, or history/paging tests see other tests' rows.
- The stateful fake stores `CanSendMessages` only for restricted members and emits only `can_send_messages` in `getChatMember` JSON (`staff_action_fake_test.go:215-235`): extend `staffFakeMember` with the full permission set (and emit/store it) or partial-permission restore cannot be tested.
- `restrictChatMember` params carry `permissions` as a `gotgbot.ChatPermissions` **struct value**, not JSON (the fake type-asserts it, `staff_action_fake_test.go:301`); assert on that.
- `TestStaffActionNonStaffUnchanged/tmute_as_a_reply` is flaky across a wall-clock second (STATE.md blocker); do not copy its time handling for undo timing tests, use an injected clock or generous margins.
- `StopStaffActions` cancels the package context; any test that calls it must restore `staffActionsCtx` in `t.Cleanup` as `TestStopStaffActionsFinalizes` does (`staff_action_progress_test.go:423-427`).

## Code Examples

### Prior capture inside the group chain
```go
// Source: skeleton against staff_action_run.go:383-399 (verified this session).
target, err := fetchLiveMember(ctx, b, link.GroupChatID, card.Target)
if err != nil { /* unchanged */ }
verdict := decideStaffAction(card.Kind, staffTargetStateFrom(target), newUntil)
if verdict.Call == staffCallNone {
	return result(verdict.Reason, "")
}
prior := staffPriorFromMember(target)
if err := staff.SavePrior(card.ActionID, link.GroupChatID, prior.toRow()); err != nil {
	log.Errorf("[StaffActions] save prior state in group %d: %v", link.GroupChatID, err)
	return result(staffReasonFailInternal, "") // fail closed: no write that could not be undone
}
if interrupted() { /* unchanged */ }
// executeStaffCall(...) unchanged
```

### Claim (exactly once)
```go
// Source: pattern of staff.SetLinkHealth (repository.go:419-441) and DeleteLinkIfOwner.
res := db.DB.Model(&models.StaffAction{}).
	Where("id = ? AND undo_started_at IS NULL", id).
	Updates(map[string]any{"undo_by": by, "undo_by_name": name,
		"undo_started_at": time.Now(), "updated_at": time.Now()})
if res.Error != nil { /* wrap, return */ }
claimed := res.RowsAffected == 1
```

### Test-side history of a restart (STAFF-10)
Run an action to completion, then read through the repository on a fresh query (no in-memory state) and assert every column; for the mid-run case follow `TestStopStaffActionsFinalizes` and assert the child rows read `fail_interrupted` and the parent has `finished_at`.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|---|---|---|---|
| Summary kept only in the Redis card (1 h) | Durable DB record plus the card for the interaction | This phase | Undo has no time limit (D-05) |
| Every edit strips buttons (`editStaffActionMessage`) | Final edit carries the Undo keyboard | This phase | New markup-capable edit/send variants |
| One decision table for forward actions | A second pure table for undo beside it | This phase | Phase 2 invariants remain intact |

**Deprecated/outdated:** none relevant.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `getChatMember` returns `until_date` exactly as sent (only the 30 s / 366 d normalization applies). | Pattern 3 left-behind rules | Spurious `skip_changed_since` on healthy groups; mitigated by the sub-30 s tolerance, would need a read-back (one extra paced call per group) instead. |
| A2 | `restrictChatMember` on a `kicked` user replaces the ban with a restriction (`is_member=false`), and on a `left` user pre-restricts. Phase 2 documents the replace rule for kicked; the `left` case is unverified live. | Pattern 3 (restore, unmute-undo) | A restore fails (reported as ❌ with Telegram's text, no data hazard) or does not restore. Needs the live UAT item below. |
| A3 | `unbanChatMember(only_if_banned=true)` on a kicked member leaves them `left` (they must rejoin). | Pitfall 2 | Wording of the done line only. |
| A4 | A callback query's `from` is always the real pressing user, also for anonymous admins. | Undo flow, D-06 | If wrong, an anonymous presser would pass as a service ID; the defensive `staffServiceUserIDs` check covers it. |
| A5 | After a restrict with the group's default permissions, Telegram may report `member`, `left` or `restricted` with `!Muted`. | Left-behind for unmute | The predicate is deliberately broad; a stricter value would cause false `changed_since`. |
| A6 | A 120 s remaining-time margin is the right trade (lift slightly early rather than risk a permanent restriction). | Pitfall 3 | Owner may prefer a smaller margin; one constant. |
| A7 | After a basic-group to supergroup migration the new chat has its own message-ID sequence. | Pitfall 5 | The `summary_chat_id` guard is safe either way. |
| A8 | A hard crash between a Telegram write and its DB result write is rare enough to accept as "not undoable, fix by hand". | Pattern 2 residual hole | Owner may want a reconcile job; out of scope. |
| A9 | Forwarded copies of a summary do not carry working callback buttons; the chat-binding check covers it regardless. | Security | None (defence in depth exists). |

## Open Questions

1. **Fail closed when the audit record cannot be written?**
   - Known: STAFF-10 says "every action is recorded"; D-02 says prior state must exist before the write.
   - Unclear: the owner never said what happens during a database outage.
   - Recommendation: **fail closed**. Abort the card at Confirm (no record, no action) and fail the group `fail_internal` if its prior-state write fails. The summary already reports such groups, so nothing is dropped silently.

2. **History detail when a record has hundreds of groups.**
   - Recommendation: one edited message, collapse done lines, always list skipped and failed, and add an explicit "N more not shown" note. Continuation messages are for run summaries, not for browsing. Planner may instead choose continuation messages; both fit the helper.

3. **Reconcile job for hard-crashed runs.**
   - Recommendation: derive "interrupted" at render time from `finished_at IS NULL AND updated_at < now - 30 min`; no job. The hourly sweeper could mark them later if the owner wants.

4. **Log posts for groups applied just before a shutdown.** `StopStaffActions` skips remaining posts. Accept (30 s drain budget), note in AGENTS.md next to the existing "a delivery cut off with the process" sentence, or record a per-group `log_status` later (additive column).

5. **Phase 2 latent issue:** `/ban 1m` can become permanent in groups reached after 30 s. Surface to the owner; not part of Phase 3 scope (a possible fix is a 5-minute minimum in `staff` durations only, a Phase 2 decision).

6. **Live UAT (cannot be automated):** the restore calls against real Telegram (restrict on a kicked user, restrict on a user who left, `UseIndependentChatPermissions` behaviour). Put these in the phase's manual verification list; STATE.md shows live checks were deferred in earlier phases.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|---|---|---|---|---|
| Go | build, tests | yes | go1.26.0 linux/amd64 | none needed |
| gcc (CGO for go-sqlite3) | `make test` | yes | `/usr/bin/gcc` | none needed |
| `go test -tags testtools` for the modules package | all phase tests | yes | baseline run passed (`ok ... 1.180s`, first compile 77 s) | none |
| PostgreSQL 16 binaries | `TestRepositoryMigrationChain`, PG integrity tests | binaries present (`/usr/lib/postgresql/16`, `/usr/bin/psql`); **server not running** (`pg_isready`: no response) | 16 | start a throwaway cluster, or rely on CI (`ci.yml:327-330` runs the chain test); SQLite covers all unit tests |
| redis-server, docker | optional live runs | binaries present | n/a | miniredis in tests (always) |
| core.telegram.org | Bot API doc verification | **blocked by the sandbox egress proxy** | n/a | gotgbot vendored comments (used); live UAT for the unverified behaviours |
| Node/npm | docs generation (`make generate-docs`) | not checked (docs generator is Go, per Makefile) | n/a | verify when running `make check-docs` |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** a running PostgreSQL for the migration-chain test (start a cluster or use CI).

## Validation Architecture

(`workflow.nyquist_validation` is `true` in `/home/user/Fuku_Robot/.planning/config.json`.)

### Test Framework
| Property | Value |
|----------|-------|
| Framework | `go test` with stdlib `testing` plus testify assertions; real fixtures: SQLite via `internal/testdb.Run`, miniredis, hand-written `gotgbot.BotClient` fakes (`staffActionFake`, `staffBotClient`) |
| Config file | none; every test file begins `//go:build testtools` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^TestStaff(Undo\|History\|Audit\|ActionRecord\|ActionLog)' ./alita/modules ./alita/db/staff` |
| Full suite command | `make test` (measured Phase 2: about 180 s; `alita/modules` about 120 s) |
| Other gates | `make lint`, `make check-translations`, `make generate-docs && make check-docs` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| STAFF-10 | Parent + pending rows created at Confirm; each column round-trips (issuer, target, action, duration, reason, time, until_date) | repo (SQLite) | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecord(Create\|RoundTrip)' ./alita/db/staff` | ❌ Wave 0 (`alita/db/staff/actions_test.go`) |
| STAFF-10 | Prior state written before the call; per-group outcomes written; finalize flush; survives a "restart" (fresh read) | module + SQLite | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordsPriorState$\|^TestStaffActionRecordPerGroup$\|^TestStaffActionRecordFinalize$' ./alita/modules` | ❌ Wave 0 |
| STAFF-10 | Mid-run shutdown leaves a truthful record (`fail_interrupted`, `finished_at` set) | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsRecordsInterrupted$' ./alita/modules` | ❌ Wave 0 |
| STAFF-10 | Audit write failure at Confirm aborts the card with no Telegram write; prior-write failure fails the group closed | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionRecordFailureFailsClosed$' ./alita/modules` | ❌ Wave 0 |
| STAFF-10 | `RekeyChat` moves `staff_actions.staff_chat_id`, leaves `summary_chat_id` and group rows | repo | `go test -tags testtools -race -count=1 -run '^TestRekeyChat' ./alita/db/staff` | ✅ extend `rekey_test.go` |
| STAFF-09 | Post only to ✅ groups, admin category only, via the pacer (429 retried), failure never changes ✅, no Staff Group title or ID in the text, reason escaped and capped | module | `go test -tags testtools -race -count=1 -run '^TestStaffActionLog' ./alita/modules` | ❌ Wave 0 (`staff_log_test.go`) |
| STAFF-09 | `actionlog.Log` unchanged for existing callers after the `Destination` split | unit | `go test -tags testtools -race -count=1 ./alita/utils/actionlog` | check (existing tests, if any) |
| STAFF-11 | Decision matrix, every row of the table in Pattern 3 | table (pure) | `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecisionTable$' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Invariants: kicked target only when live == left-behind; no call to admins; unban only to kicked; restore `Until` outside the margin | property (pure) | `go test -tags testtools -race -count=1 -run '^TestStaffUndoDecisionInvariants$' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Restore-prior end to end for ban, mute, unban, unmute over member / left / restricted (partial permissions) / kicked-shorter priors, with the stateful fake | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoRestores' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Only ✅ groups touched; skipped/failed originals never acted on; changed-since skipped; no-longer-linked skipped | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(OnlyApplied\|ChangedSince\|LinkRemoved)$' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Presser rights live per group (creator, admin with restrict, admin without, member, ex-admin); outsider refused; service ID refused | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(PresserRights\|Outsider)$' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Confirm card: presser-only, expiry, cancel; same target lock (`target busy` against a running action) | module + miniredis | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Card\|TargetLock)' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | One undo per action across two replicas (exactly one claim, other answers "already undone by") | module, concurrent | `go test -tags testtools -race -count=1 -run '^TestStaffUndoOnce' ./alita/modules` | ❌ Wave 0 (use `otherReplica()`) |
| STAFF-11 | Result is a new reply message; original edited without keyboard plus the undone line; foreign `summary_chat_id` (after re-key) gets a plain message; edit failure does not stop the undo | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndo(Reply\|OriginalEdit\|AfterRekey)' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | No Undo button for kick, no ✅ groups, undone, or unfinished records; button present on the final edit and on the fallback message | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoButton' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Undo logs (`#STAFF_UNDO`) per succeeded group, same rules | module | `go test -tags testtools -race -count=1 -run '^TestStaffUndoLog' ./alita/modules` | ❌ Wave 0 |
| STAFF-11 | Shutdown during an undo finalizes it (`fail_interrupted`, `undo_finished_at`) | module | `go test -tags testtools -race -count=1 -run '^TestStopStaffActionsUndo' ./alita/modules` | ❌ Wave 0 |
| SETUP-09 | History list: newest first, 10 per page, offset paging, only this Staff Group, line contents, tally, undone tag | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryList' ./alita/modules` | ❌ Wave 0 |
| SETUP-09 | Detail view: per-group lines, undo outcome, "no longer linked" marker, Undo button rules | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryDetail' ./alita/modules` | ❌ Wave 0 |
| SETUP-09 | Members only; non-member and wrong-chat refused; Back returns to the panel | module | `go test -tags testtools -race -count=1 -run '^TestStaffHistoryAccess' ./alita/modules` | ❌ Wave 0 |
| SETUP-09 | Length cap on adversarial names and reasons; every callback datum 1..64 bytes with the largest IDs | pure | `go test -tags testtools -race -count=1 -run '^TestStaffHistory(LengthCap\|CallbackBudget)' ./alita/modules` | ❌ Wave 0 |
| SETUP-09 | Panel renders the new button; existing render tests still pass | pure | `go test -tags testtools -race -count=1 -run '^TestStaffPanelRender' ./alita/modules` | ✅ update `panelSplitKeyboard` |
| All | Locale parity and every reason has text | unit | `make check-translations` and `go test -tags testtools -race -count=1 -run '^TestStaffReasonText' ./alita/modules` | ✅ / ❌ |
| All | Phase 2 regression net after the run/summary generalization | module | `go test -tags testtools -race -count=1 -run '^TestStaffAction\|^TestStopStaffActions' ./alita/modules` | ✅ |
| All | New migration applies on PostgreSQL and its checksum is recorded | integration (CI-style) | `ALITA_TEST_MIGRATION_CHAIN=true DATABASE_URL=<empty pg> go test -tags testtools -v -count=1 -run '^TestRepositoryMigrationChain$' ./alita/db/migrations` | ✅ (needs a PG server) |
| STAFF-11 | Claim concurrency on real PostgreSQL | integration | add the claim test name to the `-run` list of `make test-postgres-integrity` (`Makefile:41`) | ❌ Wave 0 (optional) |

### Sampling Rate
- **Per task commit:** the narrow `-run` for the touched area (each command above is under 60 s once the package is compiled; the first compile of `alita/modules` takes about 77 s).
- **Per wave merge:** `go test -tags testtools -race -count=1 ./alita/modules ./alita/db/staff ./alita/utils/actionlog ./alita/utils/ratelimit`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs && make check-docs` green before `/gsd-verify-work`.

### Wave 0 Gaps
- [ ] `alita/db/staff/actions_test.go`: repository tests (create, prior, result, finalize, claim concurrency, list/paging/tally, rekey).
- [ ] Extend the three `AutoMigrate` lists with `&models.StaffAction{}, &models.StaffActionGroup{}` (`alita/db/staff/testmain_test.go`, `alita/modules/test_harness_test.go`, `alita/db/testmain_test.go`).
- [ ] Extend `staffActionFake` (`staff_action_fake_test.go`): full permission set on `staffFakeMember`, emitted in `getChatMember` JSON and stored by `restrictChatMember`; honour `use_independent_chat_permissions`; keep struct-valued `permissions` params.
- [ ] `staffCleanup` (`staff_helpers_test.go:263`): delete audit rows for the test's chat IDs.
- [ ] Test helpers on `staffActionEnv`: `undoCard()` (find the undo confirm token like `card()` does for `staffActRunConfirm`), `setLogChannel(group, channelID)` using `logchannels.Set` (the `LogChannel` model is already migrated in the modules harness, `test_harness_test.go`), `recordOf(token)`.
- [ ] Update `panelSplitKeyboard` for the new action code.
- [ ] Framework install: none.

## Security Domain

`security_enforcement` is enabled (absent = enabled; `security_asvs_level: 1`, `security_block_on: high`).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (Telegram identity only) | n/a |
| V3 Session Management | partly: Redis card token | 16-hex `crypto/rand` token, server-side state, issuer compared inside Lua CAS (existing) |
| V4 Access Control | **yes, central** | Live Staff Group membership; per-group live `getChatMember` of the **presser**; record bound to the message's chat; link owner recheck; no admin cache |
| V5 Input Validation | yes | `html.EscapeString` after capping runes; callback data only numeric IDs or tokens, parsed strictly; record ID loaded fresh, never trusted |
| V6 Cryptography | no new use | `crypto/rand` token (existing) |
| V7 Error handling and logging | yes | Never log reasons or secrets verbatim at high volume; log-post failures at warn; no Staff Group identity in linked-group logs |
| V8 Data Protection | yes | The audit table stores names, user IDs and free-text reasons forever (D-16). Excluded from backup/export/import/reset; never cached |
| V13 API / web service | yes (Telegram callbacks) | Chat from `query.Message`, never from callback data |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| A non-staff or former member presses an old Undo button | Elevation of privilege | Live membership check at Ask and again at Confirm; record's `staff_chat_id` must equal the message's chat |
| Undo used to act in groups where the presser has no rights | Elevation of privilege | `fetchLiveMember(group, presser)` + `staffIssuerSkipReason` per group, before the target is read |
| Undo overrules a later group-admin decision | Tampering | `decideStaffUndo` compares live state to the left-behind state; any difference is `skip_changed_since` |
| Undo lifts a ban it did not place | Tampering | Restore only when live equals left-behind; `staffCallUnban` only to a kicked target; invariant test |
| Double undo across replicas or a stale client | Tampering / Repudiation | DB conditional UPDATE claim, `RowsAffected == 1` |
| Concurrent undo and new action on one person | Tampering | Same `alita:staff:lock:target:<id>` lock, token-based CAS release |
| Forged callback naming another Staff Group's record | Information disclosure / EoP | Record loaded fresh and rejected unless `staff_chat_id` equals the message's chat; record IDs are guessable, so this check is mandatory |
| Reason injects HTML or overflows a log message | Tampering / DoS | Cap runes then escape; staff names via `MentionHtml`/`staffDisplayTitle` |
| Staff Group identity leaks to linked groups' admins | Information disclosure | Post body built without the Staff Group title or ID; asserted by test |
| Log-post amplification exhausts flood budget | DoS | Every post through `staffPaced`; `MaxWait` refusal fails fast (post lost, action unaffected) |
| Audit write skipped so an action is unaccountable | Repudiation | Fail closed at Confirm and per group (Open Question 1) |
| Edit of a wrong message after chat migration | Tampering | `summary_chat_id` guard (Pitfall 5) |
| Old replica runs an undo card as a ban | Tampering | `action="undo"` plus new callback codes (Pitfall 9) |

### AGENTS.md edits required in the same commits
- **Startup and shutdown / `StopStaffActions` bullet:** undo runs join the same drain; the final audit flush runs inside the 30 s.
- **Data:** `alita:staff:act:<token>` also holds undo cards (`action=undo`, `undo_of`, `undo_kind`); the audit tables are never cached, never in backup/export/import/reset; no `DeleteCache` is needed for them.
- **StaffActions bullets:** `decideStaffUndo` sits beside `decideStaffAction`; the restore exception (a restrict or ban to a kicked or left target only when live equals left-behind); prior state is written before each write; `executeStaffUndoCall` switches only on its verdict.
- **Callbacks:** the new action codes and that history paging uses an offset cursor.
- **Subsystem traps:** `RekeyChat` re-keys `staff_actions.staff_chat_id` only; `summary_chat_id` and group rows are not re-keyed; `summary_msg_id` is only valid with `summary_chat_id`.
- **Testing:** the three harness lists now include the audit models.

## Sources

### Primary (HIGH confidence): read this session
- `/home/user/Fuku_Robot/.planning/phases/03-staff-audit-and-undo/03-CONTEXT.md`, `.planning/REQUIREMENTS.md`, `.planning/STATE.md`, `.planning/ROADMAP.md`, `.planning/config.json`
- `/home/user/Fuku_Robot/.claude/CLAUDE.md` and `AGENTS.md`
- `alita/modules/staff_action_decide.go`, `staff_action_run.go`, `staff_action_card.go`, `staff_action_summary.go`, `staff_action.go`, `staff.go`, `staff_panel.go`, `staff_notify.go`, `staff_recheck.go`, `staff_unlink.go` (panel rerender), `staff_action_parse.go` (reason cap)
- `alita/db/staff/repository.go`, `rekey.go`, `testmain_test.go`; `alita/db/models/staff.go`; `alita/db/logchannels/repository.go`; `alita/utils/actionlog/actionlog.go`; `alita/utils/ratelimit/telegram_pacer.go`; `alita/utils/callbackcodec/callbackcodec.go`; `alita/modules/callback_codec.go`; `alita/utils/errors/errors.go`; `alita/utils/extraction/extraction.go`; `alita/utils/helpers/telegram_helpers.go`
- `migrations/20261004120000_add_staff_groups_and_links.sql`, `20261004130000_add_staff_role_exclusivity_trigger.sql`; `alita/db/migrations/runner.go` (statement splitting, constraint rewriting, checksum)
- Tests and harness: `staff_action_fake_test.go`, `staff_helpers_test.go`, `staff_action_decide_test.go`, `staff_action_progress_test.go`, `staff_action_lifecycle_test.go`, `staff_panel_render_test.go`, `test_harness_test.go`, `alita/db/testmain_test.go`, `internal/testdb/database.go`
- gotgbot v2.0.0-rc.36 vendored source: `gen_methods.go` (`BanChatMemberOpts`, `RestrictChatMemberOpts`, `UnbanChatMemberOpts`, `EditMessageTextOpts`, `SendMessageOpts`), `gen_types.go` (`MergedChatMember`, `ChatPermissions`, `ReplyParameters`)
- `.planning/research/PITFALLS.md` (Pitfalls 1, 19, A, D), `FEATURES.md`; `.planning/phases/02-staff-actions-across-groups/02-SECURITY.md`, `02-REVIEW-DISPOSITION.md`, `02-VALIDATION.md`, `02-UAT.md`
- Baseline run: `go test -tags testtools -race -count=1 -run '^TestStaffActionDecision|^TestStaffActionEndsLater$' ./alita/modules` → `ok`

### Secondary (MEDIUM confidence)
- None used.

### Tertiary (LOW confidence)
- `https://core.telegram.org/bots/api` could not be fetched (egress blocked); all Bot API semantics beyond gotgbot's vendored doc comments are in the Assumptions Log.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, nothing new is introduced.
- Architecture: HIGH for hook points and reuse (all code read); MEDIUM for restore semantics against live Telegram (A1-A3, A5).
- Pitfalls: HIGH for repo-derived ones (3, 5-11); MEDIUM for Telegram-derived ones (1-4).

**Research date:** 2026-10-05
**Valid until:** 2026-11-04 (stable internal codebase; re-check if Phase 2 files change before planning)
