# Phase 13: Member Requirements - Context

**Gathered:** 2026-11-05
**Status:** Ready for planning

<domain>
## Phase Boundary

A Member requirements page in the private settings menu gives each of five rules its own punishment (off, warn, kick, ban or mute), with "Check on join" and "Delete messages" switches, everything off by default. The rules of this phase are: must have a last name, must have a username, no Russian name, no Arabic name, no Chinese name (MREQ-01, MREQ-13). A member is checked right after the captcha (or on joining a group without one) and on every message; a violation is punished once, posts one notice and, for a mute, comes with an "I fixed it" button only that member can use (MREQ-02 to MREQ-09). Out of scope and owned by later phases: the profile-photo and channel-subscription rules and the page's "this rule cannot work here" lines (Phase 14, MREQ-10 to MREQ-12), the AI spam-name rule (Phase 15, MREQ-14), automatic deletion of the notices (Phase 16, MREQ-15).

</domain>

<decisions>
## Implementation Decisions

### Join, welcome and the "I fixed it" button (owner decisions from the roadmap)
- **D-01:** The captcha runs before the requirement check on join. The join check sits on the one shared pass path (`finishCaptchaPass` after `claimCaptchaPass`) and, in a group with no captcha, on the join itself before the welcome; both callers reach one shared entry point so the two paths cannot drift. The lockdown guard (group -7) still runs first, and a captcha-failed or lockdown-banned joiner never reaches the check. A member is never given a captcha challenge because of a requirement, and a member who passed the captcha is not sent through it again by "I fixed it".
- **D-02:** A member muted on join gets no welcome until "I fixed it" succeeds; the held welcome is then sent through `SendWelcomeMessage`, so Greet once (Phase 12) claims the member only when the welcome really goes out. A kicked or banned member gets no welcome. A warned member is welcomed as usual after the warn. A member muted later by the message check was already welcomed, so "I fixed it" sends no welcome for that hold.
- **D-03:** Only a mute gets the "I fixed it" button. A kick lets the member rejoin and be checked again, a ban waits for an admin, a warn only warns. The button is a callback in its own namespace through `callbackcodec`, at most 64 bytes, carrying numbers only, usable only by the held member.

### Several rules at once and the confirm tap (owner choices, 2026-11-05)
- **D-04:** A member who breaks several rules at once is one episode: the strictest punishment among the broken rules is applied once (ban over kick over mute over warn) and the one notice names every broken rule. Fixing one of several rules leaves the member held for the others: "I fixed it" re-checks every rule live and unmutes only when none is still broken.
- **D-05:** On the Member requirements page, choosing kick, ban or mute for a rule draws the shared two-tap Yes/No card (`settings_card.go`, lazy five-minute expiry, Yes through the whole press pipeline) and writes nothing until Yes; off and warn change at once. The card says the rule acts on every member at their next message.

### Storage and the one-punishment guarantee
- **D-06:** One wide row per chat, `chat_requirements`, with one small-integer action column per rule (0 off, 1 warn, 2 kick, 3 ban, 4 mute) under a `CHECK` 0 to 4, plus `check_on_join` and `delete_messages` booleans. The model has no `default:` tag on its booleans (the GORM zero-value trap), writes are column-targeted upserts that write a Go false, every write deletes the cache keys it affects, and a chat with no row means everything off. Phases 14 and 15 add their own rule columns by their own migrations. The row is read through `cache.GetFromCacheOrLoad` because the watcher reads it on every group message; whether its key goes into `skipLocal` is settled in research.
- **D-07:** One hold row per `(chat, user)` is both the episode latch and the mute record: the rules broken, the punishment applied, the mute's marker `until_date`, whether a welcome is held, the notice message ID. The row is inserted first with `INSERT ... ON CONFLICT DO NOTHING` and the one caller with `RowsAffected == 1` punishes and posts the notice, so ten messages on three replicas give one punishment and one notice. A message check that finds the member clean deletes a non-mute hold so a later break is a new episode; a rejoin or a leave deletes the hold. The hold is read fresh from the database, never cached, never part of backup, export, import or reset, and works without Redis.
- **D-08:** The requirement mute is owned by a marker end date, as the lockdown's ban is: `restrictChatMember` with `until_date` of now plus 330 days (a finite date Telegram keeps). "I fixed it" unmutes only when a live `getChatMember` shows the member restricted with an `until_date` matching the hold's marker within 2 seconds; a mute that an admin, a staff action, the captcha, antiflood or a lockdown placed replaces that end date, so the button refuses and changes nothing. No other mute path is edited. A captcha pass never unmutes a member who has an active hold.
- **D-09:** "I fixed it" during an active lockdown unmutes through `resolveUnmutePermissions`, which returns the stored pre-lockdown permissions, as MREQ-06 words it ("respecting an active lockdown"); an error there means nothing is changed and the member is told. The hold is deleted by a conditional delete before the unmute, so two taps on two replicas act once.

### The rules (MREQ-13)
- **D-10:** The name rules judge the first and last name, never the username. Russian: the letters ы, ъ, э, ё (either case) in either name. Arabic: any letter of the Arabic script. Chinese: any Han ideograph. The page says these are letter-based and can also catch related languages (Ukrainian, Belarusian and Bulgarian share some letters). A last name made only of invisible or whitespace-like characters (zero-width, joiners, the Braille blank, the Hangul filler and similar) counts as no last name. "Must have a username" goes by the `username` field. A table-driven corpus pins the Unicode behaviour (Ukrainian, Belarusian, Bulgarian, Russian, mixed script, zero-width).
- **D-11:** Never checked: admins, approved users, bots, Telegram service accounts (`staffServiceUserIDs`), messages with `sender_chat` set (a channel or an anonymous admin), the bot itself, and a member with a pending captcha. A check that cannot be answered (an unreadable setting or admin state) never punishes anyone.

### The watcher, punishments and notices
- **D-12:** One message watcher in its own handler group before the commands (group 0), after the raid flood counter (-4) so raid detection still counts everything, settled to an exact free slot in research. It returns `ext.ContinueGroups` on every path and never ends an update, so the commands, aispam, antiflood, locks and blacklists still see the message. Checks run cheapest first: chat type, sender exemptions, the cached settings (every rule off ends it), a positive-only Redis pass cache keyed by a hash of the names, the pure name rules, then the hold insert and the punishment. The rules of this phase need no Telegram call.
- **D-13:** Punishments reuse the existing paths: a warn goes through the warn module's own path and counts toward the group's warn limit and its warn action; a kick is the member-removing unban and a ban a `banChatMember`, as the commands send them; a mute is `restrictChatMember` with the D-08 marker. Each violation posts one notice in the group naming the member (escaped, never trusted) and the rules broken (MREQ-05). The group message rate limit during a raid is a research flag: notices are paced or capped per chat, and a per-chat cap on punishments per minute is considered, so a mis-set rule cannot mass-punish without a visible stop.
- **D-14:** With "Delete messages" on, every message of a member who breaks a rule is deleted, including further messages of a member who is already held (they add no warns, notices or punishments). A failed delete is only logged. A kicked member can rejoin and is checked again; a banned member stays banned until an admin unbans them (MREQ-08).
- **D-15:** The Redis key family is `fuku:memberreq:*`, outside the `fuku:cache:` prefix so `CLEAR_CACHE_ON_STARTUP` keeps it. Nothing the requirements need for correctness lives only in Redis.

### The page and compatibility
- **D-16:** The Member requirements page is registered from its own file through `registerSettingsPage` and joins the Main grid by its `Order`, with its state line on Main. It reads through a side-effect-free loader that creates no row. The right of a punishment control is settled in the UI contract (the proposal is the creator or an administrator with `can_restrict_members`, as the lockdown triggers). The page row limits (a private page at most 8 rows, a group-menu page 9), the 3800 UTF-16 and 64-byte limits in all 8 languages and the rule that no screen names the Staff Group still hold; every new string goes into all 8 locale files.
- **D-17:** One migration with a timestamp greater than `20261105120000`, one transaction, no top-level BEGIN or COMMIT, `fuku/db/migrations/runner.go` and `scripts/migrate_psql.sh` agreeing, models with `TableName()`, the new tables added to the AutoMigrate lists of the test harnesses and listed in AGENTS.md as outside backup, export, import and reset. Each setting has one apply function beside where its command would be (there is no command: the page's apply functions carry drift and authority tests instead of a parity test).
- **D-18:** The tests cover: the Unicode corpus; the one-punishment guarantee across three replicas (`otherReplica`) and ten messages; the strictest-wins rule and the single notice; the confirm card for kick, ban and mute; "I fixed it" refusing to lift an admin, staff, captcha, antiflood or lockdown mute and unmuting through `resolveUnmutePermissions` during a lockdown; the held welcome and Greet once; the exemptions; the page's size, row and callback limits in 8 languages; and test hygiene (every test deletes the users, cache, hold and settings rows it writes, with fresh chat IDs, because CI runs all of `fuku/modules` in one PostgreSQL database).

### Claude's Discretion
- The table and column names within D-06 and D-07, the exact handler group, the callback namespace name, the locale keys and the page's key letter, `Order` and layout within the row limits, the pass-cache lifetime, the notice pacing and the punishment cap numbers, the order of the migration statements.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- Join and welcome: `processSingleNewMember` and `SendWelcomeMessage` (`fuku/modules/greetings.go`, with the Phase 12 greet-once claim), `claimCaptchaPass` and `finishCaptchaPass` (`captcha.go`), the lockdown guard (`lockdown_guard.go`, group -7), `claimRecentJoinProcessing`.
- Punishments: the warn module (`warns.go`, `warnThisUser`, `fuku/db/warns.WarnUser`, `warnPunishLock`), the ban and mute paths of `bans.go` and `mutes.go`, `resolveUnmutePermissions` (`chat_permissions.go`, the one choke point of every unmute), `isLockdownBan` for the marker idea.
- Exemptions: `IsApprovedForUpdate`, `staffServiceUserIDs`, `aispamCarvedOut` (pending captcha, cached admin, approved), `antifloodModule.userIsFloodExempt`, `raidFloodSenderCounts`.
- Settings pages: `registerSettingsPage`, the press pipeline and rights, the two-tap card (`settings_card.go`), the switch grid and steppers, `settingsKeyboard`, the apply-function pattern (`applyCaptchaTimeout`), the side-effect-free loaders, `callbackcodec`.
- Conditional-insert exactly-once patterns: `greeted_members` (Phase 12), `lockdown.BeginAlert`, the staff conditional writes.

### Established Patterns
- Models carry no `default:` tag on booleans; migrations are append-only; every write deletes its cache keys; tables outside backup are listed in AGENTS.md; harness AutoMigrate lists are updated with each model; locale keys in all 8 files with a limits row in `fuku/i18n/settings_locale_test.go`.
- A watcher returns `ext.ContinueGroups`; gotgbot stops a group at the first matching handler and `nil` leaves the group, so a watcher must never return `nil` on a path that should continue.

### Integration Points
- Handler groups (AGENTS.md "Handlers"): -4 raid flood counter, -3 staff watchers, -2 admin cache, -1 users tracker, 0 commands; the new watcher's slot is chosen in research. The Main grid and its status block (`renderSettingsMain`), `fuku/db/backup` (types and the not-in-backup test), `migrations/`, `fuku/db/migrations/runner.go`, `scripts/migrate_psql.sh`, the test harness AutoMigrate lists.

</code_context>

<specifics>
## Specific Ideas

- The owner's roadmap decisions: the captcha runs before the requirement check on join; a member muted on join gets no welcome until "I fixed it" succeeds; only a mute gets the "I fixed it" button.
- The owner's choices of this discussion (2026-11-05): several broken rules are one punishment (the strictest) and one notice naming every rule; kick, ban and mute need a confirm tap on the page.

</specifics>

<deferred>
## Deferred Ideas

- The profile-photo rule, the channel-subscription rule and the page's lines that say a rule cannot work in a group: Phase 14. The AI spam-name rule: Phase 15. Automatic deletion of requirement notices: Phase 16.
- A command to set the rules (the page is the only way); grace periods for existing members when a rule is switched on (the requirement says every member is checked at their next message, and the confirm card is the safeguard).

</deferred>
