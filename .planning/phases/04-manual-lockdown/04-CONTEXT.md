# Phase 4: Manual Lockdown - Context

**Gathered:** 2026-10-06
**Status:** Ready for planning (one open item: the spike 1 result, see D-02)

<domain>
## Phase Boundary

An admin of a group runs `/lockdown` there. The lockdown then holds until an admin of that group runs `/unlockdown`:
- Every non-admin is muted through the group's default permissions.
- Anyone who joins is banned and recorded.
- The lift puts the group's permissions back exactly as they were and unbans the recorded joiners.

Lockdown state is stored in PostgreSQL. It survives restarts and a Redis flush, every replica enforces it, and it never lifts on its own. Admins can check the state with `/lockdownstatus`, and the `/staff` panel marks a linked group that is locked. Anyone unmuted during a lockdown can still talk after the lift.

Requirements: LOCK-01, LOCK-02, LOCK-03, LOCK-04, LOCK-05, LOCK-06, LOCK-07, LOCK-08, LOCK-09, plus the deferred in-lockdown status of SETUP-08 (Phase 1 D-18).

Not in this phase:
- Alerts to the Staff Group and the log channel, the alert's "Lift lockdown" button, the joiner sample on the alert, "Ban N recent joiners", "Revoke link" and reminders. These are Phase 5 (LOCK-11 to LOCK-14). In Phase 4, LOCK-06 is met by `/unlockdown` alone, and LOCK-01's "listed on the alert" is met by recording the joiners for Phase 5 to show.
- Automatic triggers, "one lockdown and one alert when several triggers fire" (LOCK-10) and retiring `/antiraid` (LOCK-15). These are Phase 6. Phase 4 still makes a second `/lockdown` a no-op (D-14), which LOCK-10 builds on.
- Locking other linked groups. A lockdown affects only the group it was started in (LOCK-04).

</domain>

<decisions>
## Implementation Decisions

### Approved users (LOCK-03)
- **D-01:** **Fallback if Telegram can't exempt approved users: they are muted like everyone else.** Only admins can talk during a lockdown. The lock notice (D-15) says that approved users are muted too. Telegram enforces it on its own, so it keeps working when the bot is slow, rate-limited or down.
- **D-02:** **Spike 1 is a 2-minute check the owner does in the Telegram app. Its result is still open.** The owner gives the planner the result before `/gsd-plan-phase 4`. The check: in a supergroup the owner owns, go to Permissions, turn OFF "Send messages" for all members, then add a user under Exceptions and try to switch "Send messages" back ON for just that user. The research and Telegram's "restricted for all members" UI both point to "can't".
  - **Branch "can't" (expected):** D-01 applies. Locking makes no per-user calls.
  - **Branch "works":** each approved user who is a plain member gets a per-user exception at lock time, and it is cleared at the lift so they become a plain member again. Someone muted on purpose is never given an exception. **Stop and raise this with the owner before planning on this branch:** if per-user permissions can override a locked default, then every member who already holds a per-user restriction that allows sending would also keep talking. That includes anyone ever unmuted or passed through captcha, because those paths write per-user permissions. Locking the defaults would then leak.
- **D-03:** **Spike 2 is designed away, not gated.** The join guard handles all three ways a join can arrive: the `chat_member` update, the `new_chat_members` service message, and `chat_join_request`. A join delivered twice is acted on once. The ban is never gated on a Redis claim (Pitfall 10: `claimRecentJoinProcessing` fails closed). An automated test proves the guard stops greetings and captcha for a banned joiner. Live join delivery and EndGroups behaviour are UAT items.

### Removing joiners (LOCK-01)
- **D-04:** **A joiner is banned until the lift, not kicked.** Every non-exempt joiner (D-07, D-08) is banned on joining and added to the lockdown's joiner list, so they can't rejoin while it lasts and each raider costs one call. The lift unbans them, paced, and reports any it couldn't unban. — **Reversibility:** costly — Phase 5's alert sample and "Ban N recent joiners" are built on the recorded joiner list and on the lift's unban step.
- **D-05:** **The lift never lifts a deliberate ban.** It unbans only joiners whose ban is still the lockdown's own. A joiner banned on purpose during the lockdown stays banned, whether by `/ban`, a staff `/ban`, or Phase 5's "Ban N recent joiners". This is the same never-lift-a-ban rule as `decideStaffAction` and undo.
- **D-06:** **Join requests that arrive during a lockdown are declined at once**, so the person can request again after the lift. Auto-approve (`greetings.pendingJoins`) never admits anyone while a group is locked.
- **D-07:** **Only a user added directly by a live admin of the group gets in.** The join's performer must be a creator or administrator at that moment, which includes an admin approving a pending request by hand. Everyone else is banned until the lift: people joining through any invite link (an admin's own link too), approved users who rejoin, and returning members.
- **D-08:** **Bots are banned until the lift whoever adds them**, an admin included. An admin can add the bot again after the lift.
- **D-09:** **Joiners still on their captcha when the lockdown starts keep their attempt.** If they pass, they stay muted by the lockdown until the lift (through D-23). If they fail or time out, the group's captcha action applies as usual.
- **D-10:** **A banned joiner gets no welcome, no captcha and no goodbye.** Their join service message is deleted when the bot has the delete right. If it doesn't, the message stays (D-20).

### Lock & lift commands (LOCK-06, LOCK-09)
- **D-11:** **`/lockdown` and `/unlockdown` need the group's owner, or an administrator with `can_restrict_members`, checked live with `getChatMember`.** The admin cache is never used. Everyone else is refused through the command pipeline's standard reply.
- **D-12:** **Anonymous admins go through the existing "prove you're admin" button** (`RegisterAnonymousAdminHandler` + `anonPipelineHandler`). Whoever taps it is checked live for D-11's right, runs the command, and is named as the one who locked or lifted. The group's AnonAdmin mode never skips this live check.
- **D-13:** **Both commands act at once, with no Confirm tap.** `/lockdown [reason]` takes an optional reason.
- **D-14:** **`/lockdown` during an active lockdown starts nothing.** It replies with the existing one: since when, who started it, and the reason (ROADMAP criterion 3).
- **D-15:** **The group sees a public notice on lock and on lift.**
  - Lock: the group is in lockdown, only admins can talk, and new members are removed until it lifts. It also says who locked it, gives the reason if there is one, and says that approved users are muted too when D-01 applies.
  - Lift: "Lockdown lifted by Name", how many joiners were unbanned, and any that couldn't be.
- **D-16:** **`/lockdownstatus` is a new read-only command for any admin of the group.** It shows whether the group is locked, since when, who locked it, the reason, how many joiners have been removed so far, and the D-19 warning when permissions were changed by hand. It never changes anything. The name can't be confused with `/lock`, `/locks` or `/locktypes`.
- **D-17:** **`/staff` marks a locked group with one marker on its row**, like "🔒 in lockdown since 5 Oct 12:04". The reason and who locked it are not shown there. They stay in `/lockdownstatus`, and Phase 5's alert brings them to the Staff Group.

### Hand edits & restore (LOCK-05, LOCK-07, LOCK-08)
- **D-18:** **The lift always restores the exact pre-lockdown permissions.** If the live permissions are no longer the ones the lockdown set, the lift reply says a manual change was replaced. There's no prompt or choice.
- **D-19:** **The bot never fights a manual unlock.** If an admin reopens the group by hand in Telegram, the lockdown stays active until `/unlockdown`, and joiners keep being banned. The bot doesn't re-lock. `/lockdownstatus` and the lift reply point out the manual change.
- **D-20:** **`/lockdown` is refused unless the bot can lock.** The bot must be an administrator with `can_restrict_members`, and the group's permissions must be readable and settable. On refusal the reason is given and nothing is recorded. A missing "Delete messages" right doesn't block the lockdown: join messages just stay, and the reply says so.
- **D-21:** **A failed restore leaves the group locked.** If restoring the permissions fails (a Telegram error, or the bot lost its rights), the lockdown stays active and nobody is unbanned. The admin is told why and to fix it and run `/unlockdown` again. The bot never reports "lifted" until Telegram confirms the restore. Joiner unbans start only after that confirmation, and any that fail are listed.
- **D-22:** **The bot being removed from a locked group doesn't end the lockdown.** It stays recorded as active, because it never lifts on its own. `/staff` shows the group as locked and the bot as missing. Re-adding the bot, then `/unlockdown`, ends it cleanly. (Claude's default, accepted by the owner.)
- **D-23:** **Anyone unmuted during a lockdown gets the pre-lockdown permissions, not the locked set (LOCK-08).** This covers `/unmute`, a captcha pass, staff `/unmute` and the unban restore. `resolveUnmutePermissions` is the single choke point for all of them, and it must return the active lockdown's stored snapshot instead of the live (locked) defaults. Otherwise those users stay muted after the lift (Pitfall 7b).

### Claude's Discretion
- **Schema.** The lockdown row and the joiner list. The pre-lockdown permissions are stored exactly as Telegram returned them, with every field round-tripped (Pitfall 8), taken from a fresh `getChat` before locking and never re-read later. The locked set the bot applied is stored too, for D-18's and D-19's "changed by hand" check. There is one active lockdown per group, enforced by a conditional insert or a partial unique index (groundwork for LOCK-10), and a trigger-kind field so Phase 6 can add automatic triggers. Migration rules in AGENTS.md apply.
- **Recognising "the lockdown's own ban" (D-05).** For example, every ban path marks the joiner's row, or the lockdown bans with a distinctive end date that the lift compares against the live status. Any approach works as long as no deliberate ban is lifted and no lockdown ban is left in place.
- **Never ban a joiner without recording them first.** An unrecorded ban would never be unbanned. If the record write fails, skip the ban: the joiner is still muted by the locked defaults. No cap on the joiner list may leave anyone banned forever.
- **A restart in the middle of a lift** must not leave joiners banned forever. Remaining unbans resume, or are retried, after a restart. Background work joins the shutdown drain (AGENTS.md: drains are registered after DB-close).
- **Pacing.** Joiner bans, lift unbans and declines respect Telegram rate limits across replicas, through a fleet-wide budget like `staffPaced`, not a per-replica limiter. A manual lockdown still works with Redis down (the state is in PostgreSQL). Only pacing degrades.
- **Handler group for the join guard.** It must run before greetings, captcha and auto-approve (group 0), and must not share `-5` with antiraid's `onJoin` (gotgbot stops a group at the first matching handler). Update AGENTS.md's group list in the same commit. Until Phase 6, `/antiraid` keeps working as it does now, but while a lockdown is active each joiner is handled once, by the lockdown.
- **Cross-path dedupe of joins (D-03)**, and suppressing goodbye messages when the bot itself did the ban.
- **How lockdown state is read** on each join: fresh, or through `GetFromCacheOrLoad` with the key added to `skipLocal`, so that every replica enforces a new lockdown at once.
- **Basic groups.** Whether `/lockdown` works there or asks the admin to upgrade first. Restricting and unmuting individual users needs a supergroup.
- **`/lockdown` inside a Staff Group** is allowed whenever the D-11 and D-20 checks pass.
- **Wording and formats.** The exact text in all 7 locale files, the reason length cap and HTML escaping (about 300 characters, as Phase 3 suggested for log-post reasons), the time format, help text, and docs (`make generate-docs`). Also what `/unmute` replies during a lockdown, for example "they can talk once the lockdown lifts" under D-01.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Scope and requirements
- `.planning/ROADMAP.md` § "Phase 4: Manual Lockdown": goal, spike gate and the 5 success criteria.
- `.planning/REQUIREMENTS.md` § "Lockdown": LOCK-01 to LOCK-09 are this phase. LOCK-10 to LOCK-15 mark the boundary with Phases 5 and 6.
- `.planning/PROJECT.md` § "Key Decisions": the lockdown row (kick joiners, mute all except admins and approved users, admin-only manual lift), "Auto-lifting lockdowns" out of scope.
- `.planning/phases/01-staff-group-links/01-CONTEXT.md` D-18: the `/staff` row statuses that D-17 extends.

### Research (read the named sections)
- `.planning/research/ARCHITECTURE.md`:
  - Pattern 4, "Persisted state machine with atomic transitions (lockdown)".
  - Flow C, "Raid detection to lockdown to lift" (the manual half).
  - The handler-group table, plus the integration rows for antiraid, captcha, greetings and approvals.
  - Anti-Pattern 3, "Redis-only lockdown state".
  - Open Question 1, about approved-user exceptions.
- `.planning/research/PITFALLS.md`:
  - Pitfall 7: approved users, and the `resolveUnmutePermissions` contamination.
  - Pitfall 8: snapshot fidelity, double trigger and edits made by hand.
  - Pitfall 9: Redis-only state, and the `antiraid` name and namespace trap.
  - Pitfall 10: the three join paths, double counting, fail-closed dedupe, join requests and kick loops.
  - Pitfall 11: announce only what took effect.
- `.planning/research/FEATURES.md` Area 2: raid lockdown table stakes.

### Repo rules
- `AGENTS.md`: the Handlers group list, Data (cache and `skipLocal`), Permissions, Migrations, "Adding a module", Go rules (panic recovery, secrets, i18n keys) and Testing. Update it in the same commit as any new handler group or Redis key family.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `alita/modules/chat_permissions.go`:
  - `resolveUnmutePermissions` is the D-23 choke point. Its callers are `mute.go:228`, `bans.go:817`, `captcha.go:1396` (`unmuteCaptchaUser`) and `staff_action_run.go:667` (staff unmute).
  - `MutedPermissions` and `defaultUnmutePermissions` are hand-rolled structs. Don't use them for the snapshot (Pitfall 8).
- `alita/modules/greetings.go` handlers, all in group 0:
  - `newMember`: the `chat_member` join.
  - `leftMember`: goodbye messages.
  - `cleanService`: service messages, which calls `ProcessJoins` in `alita/modules/membership.go`.
  - `pendingJoins`: join requests and auto-approve.

  `claimRecentJoinProcessing` here must not gate the lockdown ban (D-03).
- `alita/modules/antiraid.go`:
  - `onJoin` sits at group `-5`, on `NewChatMembers` only.
  - It has its own `antiraid` callback namespace. A new namespace must not start with `antiraid`, because prefix routing would swallow it.
  - `trackJoinScript` is for Phase 6.
- `alita/modules/captcha.go`: `SendCaptcha` and `unmuteCaptchaUser`. D-09 leaves pending attempts in place.
- `alita/db/approvals/repository.go`: `IsUserApproved`, `GetApprovedUsersContext`. Needed only on D-02's "works" branch, and to word the D-15 notice.
- `alita/modules/anonymous_admin_router.go` (`RegisterAnonymousAdminHandler`) and `alita/modules/moderation.go:349` (`anonPipelineHandler`): for D-12.
- `alita/modules/staff_panel.go`: `renderStaffRow`, `staffRowStatuses` and `buildStaffPanelRows`, where the D-17 marker goes.
- `alita/modules/staff_action_run.go:182` `staffPaced`: the fleet-wide Redis pacer pattern for bans, unbans and declines.
- `alita/db/cache/local.go` `skipLocal`: the freshness-critical key list.
- `alita/config/config.go:339`: `AllowedUpdates` already includes `chat_member`, `my_chat_member` and `chat_join_request`.

### Established Patterns
- Authority comes from the live `getChatMember` answer, never the admin cache (Phases 2 and 3).
- Exactly-once state moves come from conditional writes (RowsAffected == 1). That makes the second `/lockdown` a no-op, and a double `/unlockdown` lifts once.
- Record before acting, and fail closed when the record can't be written (Phase 3). Here that means no ban without a joiner row.
- Never lift a ban by accident (`decideStaffAction`, `decideStaffUndo`). D-05 applies the same rule to the lift.
- Announce only what Telegram confirmed (Pitfall 11, D-20, D-21).
- Commands go through `helpers.WrapCommand`. Fire-and-forget goroutines start with `defer error_handling.RecoverFromPanic(...)`. Every string goes in all 7 locale files.

### Integration Points
- A new join-guard handler group runs before group 0 and apart from `-5`. It returns `ext.EndGroups` for a banned joiner and `ext.ContinueGroups` otherwise.
- `resolveUnmutePermissions` reads the active lockdown's snapshot (D-23).
- The `/staff` row rendering adds the 🔒 marker (D-17).
- Shutdown: any lift or unban worker registers its drain after DB-close.
- New models go into the `AutoMigrate` lists of the test harnesses that touch them (AGENTS.md step 4).

</code_context>

<specifics>
## Specific Ideas

- The lock notice says: the group is in lockdown, only admins can talk, and new members are removed until it lifts. It names who locked it and gives the reason, and adds "approved users are muted too" when D-01 applies.
- The lift notice reads "Lockdown lifted by Name". It gives the number of joiners unbanned and lists any that couldn't be, and notes when a manual permission change was replaced (D-18).
- The `/staff` row marker reads "🔒 in lockdown since 5 Oct 12:04", in the same time style as Phase 3's history lines.
- The owner's spike 1 check is in D-02. It needs only the Telegram app, not the bot.

</specifics>

<deferred>
## Deferred Ideas

None came up. The discussion stayed within the phase. Notes for later phases:
- Phase 5: "Ban N recent joiners" must mark those joiners so the lift keeps them banned (D-05). The alert's sample reads the D-04 joiner list.
- Phase 6: LOCK-10 builds on the one-active-lockdown rule (D-14), and LOCK-15 retires `/antiraid`, which coexists with lockdown until then.

</deferred>

---

*Phase: 04-manual-lockdown*
*Context gathered: 2026-10-06*
