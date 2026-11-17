# Audit findings, 2026-11-16 (input for quick tasks Q1 to Q5)

Two read-only audits found the defects below. Each quick task fixes one batch. Line numbers are
from commit b356e4b and may drift; re-locate by symbol. "Confirmed" was traced through code,
"reproduced" was run, "likely" depends on Telegram or PostgreSQL behaviour that no fake proves.

Rules for every batch (from AGENTS.md, which is binding):

- Start each fix with a failing test that goes through the REAL dispatcher where the defect is a
  routing one (the existing direct-call tests hid most of these). No mock libraries, no `t.Skip`,
  fresh chat and user IDs (`uniqueModuleChatID`, `uniqueLinkOwnerID`), cleanup in `t.Cleanup`.
- Keep new tests light: `fuku/modules` is near its CI time limit.
- Run only targeted tests: `go test -tags testtools -race -count=1 -run '^TestName$' ./pkg`.
  Never run `make test` (40 minutes). Finish with `CGO_ENABLED=0 go build ./...`,
  `go vet -tags testtools ./...` and, when locales changed, `make check-translations`.
- Never re-record or weaken a `TestLegacy*Requests` golden. When a fix changes a pinned byte on
  purpose, say so in the commit and change only the affected expectation.
- Locale keys go in all 8 files (en, es, fr, hi, id, pt, ru, uk); a new `settings_` key needs its
  row in `fuku/i18n/settings_locale_test.go`. The bot's name stays Xenobot-72.
- Command handlers are built only through `helpers.NewBotCommand` / `helpers.WrapCommand`
  (`TestNoRawNewCommand`). `NewBotCommand` returns a `handlers.Command`, so `.SetAllowChannel(true)`
  can be chained on it.
- Migrations are append-only, timestamp greater than every existing file, no top-level
  BEGIN/COMMIT. Both appliers must agree.
- Update AGENTS.md in the same commit as any change that invalidates a rule in it.
- Conventional Commits; user-visible fixes are `fix:`. One commit per defect or tight group.
- Do not push. Do not touch unrelated code.

## Q1. Log channel flows

1. `/setlog` cannot work (confirmed), three defects in `fuku/modules/logchannels.go`:
   - a. `:323` `helpers.NewBotCommand("setlog", ...)` has no `SetAllowChannel(true)`, so gotgbot's
     `Command.CheckUpdate` drops the channel post.
   - b. `:43` `requireUser` is nil for a channel post (no `from`), so the handler would stop.
   - c. Step 2 is shadowed: the forwarded channel post still reads `/setlog` with a `bot_command`
     entity, so in the group the group-0 `setlog` command matches first, replies
     `logs_need_channel` and returns `ext.EndGroups`; `captureSetLogForward` (group 11) never runs.
   Fix: allow channel posts; in the channel branch do not require a user (authority is step 2: the
   forwarder must be an admin of the group, already checked); in a group, when the message is a
   forward whose origin is a channel, do the bind in the command handler itself (share one bind
   function with `captureSetLogForward`) instead of replying and ending. Keep the group-11 watcher
   for forwards that carry no command text. A plain `/setlog` typed in a group keeps
   `logs_need_channel`. Test through a real dispatcher: channel post then forward binds the
   channel; a member's forward binds nothing; a forward with no pending marker binds nothing.
   The marker keys are fine (marshaler `Set` and raw `GetDel` address the same Redis key).
2. `/setfedlog <fed id>` channel branch unreachable (confirmed): `federations.go:1269` registration
   has no `SetAllowChannel`, `:951` `requireUser` is nil for a channel post, and the owner check
   needs a user. Fix it (owner decision: fix, do not drop) with the same two-step proof as
   `/setlog`: the channel post `/setfedlog <fed id>` stores a pending marker keyed by channel and
   message ID with the fed ID as value (1 hour, outside `fuku:cache:`); the federation OWNER then
   forwards that post to the bot's private chat or to a group, where the forwarder is checked as
   the fed owner and the log is set to the channel. Add the reply strings to all 8 locales and
   correct `federations_help_msg`, which lists `/setfedlog` under "Owner (in private)" although
   the handler refuses private chat (`:979`): describe the group use and the channel flow.
3. Log categories `automated` and `other` never receive a post (confirmed: nothing calls
   `actionlog.Log` with `CategoryAutomated` or `CategoryOther`). Owner decision: remove the two
   dead toggles from what users can see and set, without a migration: drop them from the Log
   channel settings page table (`settings_logchannel.go` `settingsLogCategories`), from
   `logchannels.AllCategories`/`IsValidCategory` as accepted names for `/log` and `/nolog`, and
   from `logs_categories` / help texts in all 8 locales. Leave the DB columns. This changes the
   `TestLegacyLogCategoryRequests` golden only where it names those two categories, and the page
   tests' shapes (`{2,2}` grid, four lines); update those expectations and the AGENTS.md bullets
   on the Log channel page. If removing them from `AllCategories` breaks backup import of old
   exports, keep import tolerant (ignore the two names).

## Q2. Targets and moderation safety

4. `/unban` on a member who is not banned kicks them (confirmed): `bans.go:400`
   `c.Chat.UnbanMember(c.Bot, t.userID, nil)`. Pass
   `&gotgbot.UnbanChatMemberOpts{OnlyIfBanned: true}`. The golden for `/unban`, if one pins the
   request, changes by that one parameter on purpose.
5. An explicit target loses to the reply (confirmed): `fuku/utils/extraction/extraction.go:241-246`
   (`ExtractUserAndText`). `/ban @alice` sent as a reply to Bob acts on Bob, because an
   `@username` or `text_mention` sets `userId` but not `isId`, and the next branch replaces it with
   `IdFromReply`. Fix: fall back to the reply only when the argument resolved to no user. Affects
   every command that resolves targets this way. Test at the extraction level plus one command.
6. Forum topics: an implicit reply is trusted (likely). In a forum supergroup Telegram sets
   `reply_to_message` to the topic's `forum_topic_created` service message on every topic message
   that is not an explicit reply. `/warn`, `/ban`, `/mute`, `/id` with no target act on the topic
   creator, and `/purge` with no reply deletes across topics. Fix: one shared helper (in
   `extraction` or `helpers`) that returns the replied message or nil when
   `ReplyToMessage.ForumTopicCreated != nil` (or its ID equals `msg.MessageThreadId` for a topic
   message); use it at `extraction.IdFromReply` (`extraction.go:305`),
   `moderation.extractFromReply` (`moderation.go:143`), `purges.go:169` and `:230`,
   `misc.go:128`.
7. `@username` lookups are case-sensitive and can match a stale owner (confirmed):
   `fuku/db/user/repository.go:119` `GetUserIdByUserName` (`WHERE username = ?`, first row).
   Fix: match `LOWER(username) = LOWER(?)`, newest `last_activity` first; when more than one user
   holds the name, return not found (refuse an ambiguous name) as `FindUsersByUsername` (staff
   code) does. Keep the cache key behaviour correct.
8. Ban, kick and mute replies put the reason into HTML unescaped and through `Sprintf` as a format
   string (confirmed): `bans.go:57-66`, `:120-127`, `:147-153`; `mute.go:29-36`, `:108-113`;
   names in `/id` (`misc.go:143-145`, `:186-189`). A reason with `<` makes Telegram reject the
   reply after the ban happened; one with `%` prints `%!d(MISSING)`. Fix: `html.EscapeString` the
   reason and names, and never pass user text through `Sprintf` as the format.
9. Staff `/tban` / `/tmute` with a short duration can become permanent (likely, high impact):
   `staff_action_card.go:886-898` fixes `until_date` once at Confirm; the paced fan-out
   (`staff_action_run.go:647-659`) can send it to late groups, or after a 429 wait, less than
   30 s before it ends or after it, which Telegram treats as forever; Undo then refuses
   ("restriction ended"). Fix: right before each ban or restrict write, when the fixed end date
   is less than 35 s away (or past), do NOT send the call: fail that group with a clear reason
   ("the restriction's time had already run out", new locale key in 8 files) so nobody is
   restricted for good. Keep one `until_date` for all groups (AGENTS.md says so); update the
   AGENTS.md "Staff durations" bullet.
10. Federation names are cut by bytes (likely): `fuku/db/federations/repository.go:65-71`
    `trimFedName` uses `name[:64]`. Truncate by runes.
11. `/unfban` only deletes the row (likely): `federations.go:738-753`. After the unfban, unban the
    user in every chat of the federation with `OnlyIfBanned: true`, the way `applyActiveFban`
    loops over the fed's chats (same pacing/goroutine pattern, with
    `defer error_handling.RecoverFromPanic`). Minor: `/fban` on an already fbanned user should
    not re-run the notification and the active-ban loop unless the reason changed (only if it is
    a small change; otherwise leave).

## Q3. Anonymous admins, locks, connections

12. Anonymous-admin proof cache cannot be decoded for a command that replies to a forwarded or
    pin-service message (reproduced): `fuku/utils/chat_status/chat_status.go:527-536`
    (`setAnonAdminCache`) stores the whole `gotgbot.Message` through the msgpack marshaler;
    `fuku/modules/bot_updates.go:253-268` reads it back and msgpack panics on interface-typed
    fields (`ForwardOrigin`, `PinnedMessage`). The panic is swallowed. Fix: store JSON bytes
    (`json.Marshal` / `json.Unmarshal`, gotgbot has custom unmarshalers) and decode on read. A
    value in the old format must read as "expired", never panic (rolling deploy).
13. Commands that prompt for anonymous proof but have no re-entry (confirmed):
    `anonymous_admin_router.go:21-26`, `bot_updates.go:236-252`. After the tap the prompt is
    deleted, nothing runs and the callback is never answered. Owner decision: register `/kick`,
    `/dkick` (`bans.go:1145-1152`), `/purgefrom`, `/purgeto` (`purges.go:516-517`) with
    `RegisterAnonymousAdminHandler` + `anonPipelineHandler` like their siblings; for every other
    command name not in the registry, answer the tap with an alert that says the command cannot
    be used anonymously and to post as yourself (new locale key, 8 files), keep the prompt
    deleted, and answer the callback exactly once on every path. Also parse the command name
    the way gotgbot does: first field up to any whitespace (` \t\n\v\f\r`), lower-cased, `@bot`
    suffix stripped, so `/Ban` and `/ban\nreason` find the registry.
14. Double reply at the proof prompt (confirmed, minor): `access.go:26-28` `hasUserPermission`
    sends the proof keyboard and returns false, so the caller's `PermissionResponder` also sends
    "you lack the right". Suppress the second reply when the proof prompt was sent (for example a
    per-update marker in `ctx.Data` that the responder checks). Do it only if it stays small and
    safe; otherwise leave and note it.
15. `/lock` and `/unlock` always refuse anonymous admins (confirmed): `locks.go:278`, `:353` call
    `RequireUserAdmin(b, ctx, nil, sender.Id())`; for an anonymous admin `sender.Id()` is the
    group's own negative ID and `IsUserAdmin` is false for non-positive IDs. Fix: treat a sender
    that `IsAnonymousAdmin()` as an admin when the chat's AnonAdmin setting allows, else use the
    anonymous proof path the other admin commands use; a real user is checked by `From.Id`.
16. Lock watchers treat anonymous admins and the linked channel as ordinary members (confirmed):
    `locks.go:447-452` (`restHandler`), `:498-503` (`permHandler`). With `/lock all`, `url`,
    `forward` and so on, an anonymous admin's messages and every automatic forward from the
    linked channel are deleted. Fix: exempt `IsAnonymousAdmin()` and `IsLinkedChannel()` senders
    (and automatic forwards) before the lock loop, except for the two locks that are about them
    on purpose (`anonchannel`, `comments`).
17. `/lock anonchannel` also deletes the linked channel's own posts (likely unintended):
    `locks.go:81-84` filter is `IsAnonymousChannel() || IsLinkedChannel()`. Drop
    `IsLinkedChannel()`.
18. Admin commands sent in PM through a connection refuse a real admin (confirmed): after
    `IsUserConnected(..., true, ...)` these call `RequireUserAdmin(b, ctx, nil, ...)`, and a nil
    chat resolves to the private chat: `locks.go:278`, `:353`; `warns.go:137`, `:658`;
    `greetings.go:546`; `notes.go:532`, `:1094`. Pass the connected chat.
19. Overwrite Yes/No card can never succeed for `/save` or `/filter` used through a PM connection
    (confirmed): the card stores the group's chat ID but is posted in the private chat; the
    callback (`notes.go:688`, `:738-741`; `filters.go:665`, `:720-721`) checks admin rights and
    chat equality against the callback's chat. Fix: when the callback arrives in a private chat,
    take the target chat from the cached card and check the presser's admin rights there; keep
    the equality test for group callbacks. Do the checks before the token is consumed where
    possible.

## Q4. Strings, links and small commands

20. Messages print raw `%s` / `%d` (confirmed). `interpolateParams`
    (`fuku/i18n/translator.go:90-99`) fills verbs only from keys "0".."9" or its `commonKeys`.
    - `backup.go:443` `backup_import_confirm`, `:523` `backup_reset_confirm`, `:613`
      `backup_import_success`, `:669` `backup_reset_success`, `:688` `backup_export_success`:
      params `modules`, `list`, `chat`, `time`.
    - `captcha.go:1714` `captcha_stored_messages_summary`: params `user`, `count`, `types`.
    - `captcha.go:904`, `:926` `captcha_max_attempts_current`, `captcha_max_attempts_set`: param
      `attempts`.
    Fix: change those strings to `{name}` placeholders in all 8 locales, matching the Go param
    names. Add one test in `fuku/i18n` or `fuku/modules` that renders each of these keys through
    the real locale files with the real param names and asserts no `%` verb and no `{` remains.
21. Two locale keys missing in all 8 files (confirmed, low reach): `greetings.go:68`, `:78`
    `greetings_welcome_not_configured`, `greetings_goodbye_not_configured`. Add them.
22. `aispam_help_msg` mixes HTML tags with Markdown (`*...*`, backticks), so the Markdown is shown
    raw (confirmed, cosmetic). Convert the string to HTML in all 8 locales where it has that shape.
23. `backup_help_msg` lists `/export` under user commands, but it needs an admin
    (`backup.go:201`). Move it in all 8 locales.
24. `captcha_action_muted` says "muted permanently" while the mute failure action lasts 24 hours
    (`captcha.go:1620`). Correct the wording in 8 locales.
25. Private-note deep links put the raw note name in `?start=` (likely): `notes.go:552`, `:948`,
    `:1060`, `:1221` build `note_<chatid>_<name>`; parser `:1234-1258`. Telegram start
    parameters allow only `A-Za-z0-9_-` and 64 characters, so Cyrillic and long names fail. Fix:
    build `noteid_<chatid>_<row id>` (new deep-link prefix through `RegisterDeepLinkHandler`,
    looked up by `(id, chat_id)`), keep the old `note_` parser for buttons already posted, and
    HTML-escape note names where they are spliced into HTML (`:552`, `:1221`).
26. `/wiki` always answers "not found" (confirmed live): `misc_extras.go:23-37` `fetchJSON` sends
    no User-Agent and Wikimedia answers 403. Set a descriptive `User-Agent`
    (`Xenobot-72/<version> (Telegram bot)`) on every `fetchJSON` request.
27. `/reports on|off` in PM requires a group connection, and a connected admin can then never
    change the group's setting from PM (likely): `reports.go:304-309`, private branch ~`:333`.
    Fix: in a private chat with no connection, the command sets the user's own preference
    without needing a connection; with a connection it sets the connected group's switch. The
    `TestLegacyReportsRequests` / `TestLegacyReportsPrivateRequests` goldens pin today's
    behaviour: change only the expectations this fix alters and say so.
28. `/disable` accepts names whose handlers never check them (confirmed, low): `blocklists`
    (`blacklists.go:811`; handler checks only `blacklists` at `:445`), `antiraid`, `raidtime`,
    `raidactiontime` (`antiraid.go:43-47`), `export`, `import` (`backup.go:768-769`). Make
    `blocklists` honour its own name (check the name that was used); drop `Disableable` from the
    retired raid pointer and from `export`/`import`.

## Q5. Captcha, raid, database, typed replies

29. `/reset warnings` and a backup import with an empty warn mode fail on a chain-built
    PostgreSQL (likely): `fuku/db/backup/backup.go:1072` writes `WarnSettings{WarnLimit: 3}`
    (mode `''`), which the legacy `chk_warns_mode` of `migrations/20250805204145` rejects;
    `20260412000000` drops only the singular `chk_warn_mode`. Fix: a new migration that drops
    `chk_warns_mode` (`ALTER TABLE warns_settings DROP CONSTRAINT IF EXISTS chk_warns_mode;`),
    following the pattern of `20261113120000` for the blacklist check, with its database-free
    statements test in `make test` and a PostgreSQL integrity test added to the
    `make test-postgres-integrity` list (see how `TestBlacklistActionValuesOnPostgres` is wired).
    Also make the reset write mode `mute` and the import replace an empty mode with `mute`
    (AGENTS.md notes the empty mode as inert); update that AGENTS.md note.
30. Captcha group -10 can swallow the joiner's own join service message (likely):
    `captcha.go:2378` `NewMessage(nil, handlePendingCaptchaMessage)`, body `:2140-2233`. When
    the `chat_member` update created the attempt first, the `new_chat_members` service message
    (sender is the joiner) is stored as "[Unsupported message type]", deleted, and the update
    ends, so the lockdown guard (-7), fed-ban (-6), service deletion (-5) and `cleanService`
    never see it. Fix: return `ext.ContinueGroups` early for service messages
    (`NewChatMembers`, `LeftChatMember` and the other service types; reuse
    `deletionServiceKindOf` or `memberContentMessage` if it fits). Test through the dispatcher.
31. A captcha refresh leaves no timer (confirmed): `captcha.go:2100-2138` increments
    `refresh_count`, the original timer then sees itself as stale (`:1421-1425`), and nothing
    schedules a new one, so the failure action waits for the 5-minute cleanup tick. Call
    `scheduleCaptchaTimeout` with the updated attempt after a successful refresh.
32. With the join and flood triggers off, the AI trigger's own checks never run (confirmed):
    `raid_join.go:155` `raidJoinSettings` returns early when `!JoinEnabled && !FloodEnabled`, so
    nothing is written to `fuku:raid:{chat}:joined`, which both raid AI paths use to decide "new
    member". Include `AIEnabled` in that gate (the join-surge trigger itself must still not fire
    when it is off).
33. The settings typed-reply door silently ignores a text that starts with `/` and a non-space
    (minor): `settings_input_door.go` `settingsInputLooksLikeCommand`. When the sender has a live
    wait and the first word is NOT a command the bot registers (`!helpers.IsBotCommand(name)`),
    refuse with a reply that says a text starting with a slash cannot be saved here, keep the
    wait, and show the Cancel button (use the door's existing refusal path; new `settings_` key
    in 8 locales with its limits-table row). A registered command still passes through
    untouched. Update the AGENTS.md door bullet.
