# Phase 22: PM Copies - Context

**Gathered:** 2026-11-19
**Status:** Ready for planning. The decisions marked "owner" were settled before this phase (milestone decisions of 2026-11-18 and the
phase brief) and are not reopened. The decisions marked "default" are the planner's recommended defaults, taken from the documents and the
code without further questions to the owner; they are provisional and reversible, and the ones an owner would notice are listed at the end.

<domain>
## Phase Boundary

Phase 22 adds an optional private copy of an event message, sent to the punished person in the bot's private chat, as an engine feature
that every later event reuses:

- **The PM destination** (PMSG-01): each event that has a private copy gets its own text, one media item, URL buttons and an on/off
  switch, off by default, edited on the Messages page with the v1.1 editor exactly as the group message is.
- **Best effort, its own pacing** (PMSG-02, PMSG-03): the copy is sent after the punishment and after the group message, on its own
  fleet-wide pacer; a person who never started the bot or blocked it changes nothing else.
- **The Details button** (PMSG-04): when the copy was not delivered and the group's option is on, the group message gets a button that
  opens the bot's private chat and shows that person, and only that person, their message.
- **Never for raid-scale removals or Staff Group actions; a cap for automatic events** (PMSG-05).
- **No Staff Group and no other group named** (PMSG-06).
- **The log line** (PMSG-07): with the group's option on, a copy that was not delivered leaves one line in the group's log channel.

In this phase the seven manual events of Phase 21 (ban, tban, kick, mute, tmute, unban, unmute) get their private copy. The destination,
the switch, the pacing, the caps, the Details button and the log line are built as engine features driven by a per-event spec table, so
Phases 23 and 24 add their events' private copies by registering one spec row and adding strings, with no engine change.

Out of scope here: the automatic, captcha, requirement and lockdown events (Phases 23 to 25; lockdown messages send no private copy at
all), a private copy of a Staff Group action or of a raid-scale removal (never), the appeal button on the private copy (Phase 27), plain
tone (Phase 29), copying settings between groups (Phase 29), and private copies or the two options in backups (Phase 30, MSG-10).

</domain>

<decisions>
## Implementation Decisions

### Scope rules (owner)
- **D-01 (owner):** Everything is set group by group in that group's settings in the bot's private chat. Nothing at Staff Group level,
  nothing inherited, no command sets a private copy (the Messages page is the only door).
- **D-02 (owner):** A custom text is one text for every language. With no text stored, the bot's default private text is used.
- **D-03 (owner):** A private copy never delays, undoes or skips a punishment or the group message. It is sent after both, best effort,
  and it is off by default.
- **D-04 (owner):** No private copy is ever sent for a lockdown's joiner bans, for "Ban N recent joiners" or for a Staff Group action.
- **D-05 (owner):** Phase 22 covers only the events that exist after Phase 21. The PM destination, the switch, the pacing, the caps,
  the Details button and the log line are engine features, not per-event code: an event has a private copy when a spec row is
  registered for it (`registerEventPM(event, eventPMSpec{DefaultKey, Automatic})`, panicking at init on an unknown event, a duplicate or
  an empty key), and the page and the send path read that table.

### Events and destination (default)
- **D-06:** All seven manual events get a private copy, unban and unmute included: an unbanned person cannot read the group, so a
  private "you can join again" is the one way to tell them, and the switch is off until the owner turns it on. `/dban`, `/dkick` and
  `/dmute` use their parent event's copy (they share the event, as in Phase 21). The silent variants `/sban`, `/skick` and `/smute` send
  no private copy: they never reach `replyEvent`, and EVT-01 keeps their behaviour as it is (they leave no trace anywhere). The channel
  `/ban` and `/unban` act on a channel, are not events and send nothing.
- **D-07:** The destination is `eventmsg.DestPM = "pm"`, appended to `eventmsg.Dests` (no migration: the destination is validated in Go).
  An event's private copy is a second message editor definition whose key (the wait's `p`) is `h` + the event code + `p` (`hbnp`), beside
  the group's `hbng`; its buttons carry the event in `k` and the destination in the new field `d` (only the value `p`; no `d` means the
  group, so every Phase 21 button keeps its bytes).

### Storage (default)
- **D-08:** The per-event on/off switch is the `silent` column of the `pm` row, with a destination default: `eventmsg.DefaultSilent(dest)`
  is false for the group and true for `pm`. A part writer (`SetText`, `SetMedia`, `SetButtons`) that inserts a new `pm` row writes
  `silent = true`; only `SetSilent` changes it; the conflict update never touches it. `(*Messages).Resolve(event, dest)` returns the stored
  message or, with no row, the empty message with `Silent = DefaultSilent(dest)`. So "Silent" means "nothing is sent to that destination"
  for both destinations, no row means off for a private copy, and storing a text alone never turns the copy on. Reason: the existing row
  carries the switch with no new column (the brief prefers that), and the meaning is the same word for both destinations. Rejected: an
  `enabled` column (a migration for a value the row can already carry) and "a row exists means on" (setting a text would turn it on).
- **D-09:** The two per-group options (Details button, log line) live in one new table `chat_event_message_options` (one row per chat:
  `chat_id` primary key, `pm_details`, `pm_log`, `updated_by`, `updated_at`, every column `NOT NULL` with a zero default, inline primary
  key, no foreign key, no `default:` tag on the model's booleans), added by migration `20261120120000_add_event_message_options.sql` (one
  idempotent `CREATE TABLE IF NOT EXISTS`, one transaction, both appliers unchanged). It is read by `eventmsg.GetFresh` into
  `Messages.Options` (`eventmsg.Options{PMDetails, PMLog bool}`), so the send path gets the per-event rows and the options in its one
  cached read under `event_messages:<chat>` (already in `skipLocal`), and `SetPMDetails` and `SetPMLog` are column-targeted upserts that
  drop that key. A failed read of either table is the read's error, never a default. Rejected: pseudo event rows (an event key that is not
  an event) and option columns on an unrelated table (`chat_settings_access` is the owner switch, read uncached).
- **D-10:** Neither the `pm` rows nor the options table is part of backup, export, import or reset in this phase; Phase 30 decides for
  both together with MSG-10. `TestEventMessagesStayOutOfBackup` is extended to the options table.

### Sending (default)
- **D-11:** Order inside `replyEvent`: the one read; the group step exactly as Phase 21 (its requests pinned by `TestLegacyModerationRequests`
  and unchanged); then the private copy is offered, whatever the group step did (posted the default or a custom message, Silent, a refused
  custom message replaced by the default, or a send error). A failed read sends the default group reply and no private copy (the switch is
  unknown, and unknown never sends). The offer is non-blocking: the handler returns as before, and the copy is sent from a job.
- **D-12:** Content. With text, media or buttons stored on the `pm` row, the copy is composed by `composeEventPM`, which uses the same
  renderer as the group message (`renderEventText`: the Welcome placeholders for the punished person and the group, then the event
  placeholders, every user value escaped, `{admin}` the acting admin's escaped name or the translated `eventmsg_admin_anon` for an
  anonymous admin) and a keyboard of the custom buttons and the `{rules}` button only, never the bot's own Unban or Unmute row (those are
  group callbacks). With no stored text, the text is the event's default private text (`eventPMDefaultText`: the spec's `DefaultKey`
  with `{group}` the group's escaped title and, for a timed event, `{until}` in the group's zone, then `eventmsg_pm_reason` when there is a
  reason); the default never names the acting admin. An `/unmute` during a lockdown carries the private wording of the lockdown note
  (`eventmsg_pm_unmute_lockdown`, passed by the site in the new `eventReply.PMNote`), after a custom text or the default. The copy is in
  the group's language (the moderation context's translator), not the person's: the person may never have set a language, a preview can
  only show one, and the group message is in that language too.
- **D-13:** The job pool is bounded in-flight goroutines, not a queue: `pmCopyJobs` (a `raidPostTracker`) with `startBelow(pmCopyMaxInFlight)`
  (16 per replica), each starting with `defer error_handling.RecoverFromPanic`, each under `pmCopyJobTimeout` (60 s) from
  `context.Background()`. There is no shutdown drain and no flag: a restart cuts off the copies in flight (accepted, like the federation
  fan-out), so the shutdown worst case (68 s) and `stop_grace_period: 75s` are unchanged and `main.go` is not edited. A job reads no
  PostgreSQL except the log destination (D-21), which fails quietly once the database is closed.
- **D-14:** Pacing: one fleet-wide pacer `pmCopyPacer` on its own keys `fuku:pmcopy:pace:next` and `fuku:pmcopy:pace:block`
  (`pmCopyPacerOptions`: Interval 200 ms, MaxRetries 1, MaxWait 15 s), so private copies take at most five sends a second of Telegram's
  global budget and a burst never delays a ban, a mute, a group reply or any other pacer's work. Every Telegram call of a job (the copy,
  its fallback, the Details edit, the log post) goes through `pmCopyPaced`, bounded by `pmCopyCallTimeout` (10 s), and nothing else uses
  those keys. A slot more than 15 s away is "busy" with no request.
- **D-15:** Classification, by Telegram's error code only (never a description): no error is delivered; a 403 is "blocked" (the person
  never started the bot, blocked it, or the account is gone: the normal "not delivered" answer, logged at Debug, never at Warn); any other
  4xx except 429, and `media.ErrNoPermission`, is "refused"; a 429 or `ratelimit.ErrRateLimited` is "busy"; a 5xx, a transport error or a
  timeout is "unknown". A refused custom copy is replaced by the default private text, sent once (as MSG-04 does for the group); a blocked
  copy is never retried. Not delivered means any final outcome but delivered of a job that ran.
- **D-16:** Targets never offered: a non-positive ID, a Telegram service identity (`staffServiceUserIDs`) and the bot itself.
- **D-17:** The per-group cap for automatic events (PMSG-05) is an engine feature now, used by the events Phases 23 and 24 register with
  `Automatic: true`: one counter `fuku:pmcopy:{<chat>}:cap:<unix minute>` (`INCR` and `EXPIRE 120` in one transaction, the minute from
  `pmCopyNow`), at most `pmCopyAutoCapPerMinute` (10) copies a group a minute, checked as the job's first step so the handler never makes a
  Redis call for a copy. Over the cap, with no Redis or on a Redis error, the copy is
  not sent (fail closed: the copy is optional and a raid must not become a stream of private messages). Manual events have no cap: they come
  at a human rate, and the in-flight bound and the pacer bound them. The seven manual specs are `Automatic: false`.
- **D-18:** A copy shed by a full pool or refused by the cap leaves nothing: no Details button, no log line, one Debug line with the chat
  and user IDs. Only a job that tried to send and did not deliver can add the Details button and the log line.

### Details button (default)
- **D-19:** When a job's copy was not delivered, the Details option is on and the group message exists, the job stores a record and adds
  the button. The record is `fuku:pmcopy:det:<token>` (the token 16 bytes from `crypto/rand` in unpadded base64url, 22 characters), TTL 48 h
  (`pmCopyDetailsTTL`, longer than the 24 h Bot messages cap, so the record outlives every group message that can carry its button), value a
  JSON of the punished user's ID, the chat ID, the event and the composed copy (text, file ID, media type, URL buttons). Then
  `editMessageReplyMarkup` of the group message with the rows it was sent with plus one row with one URL button `eventmsg_btn_details`
  (`https://t.me/<bot username>?start=pmcopy_<token>`), through the PM pacer. The button is added after the group message was posted,
  because the copy is tried after it (D-11). No button when Redis is unavailable (no record can exist), the bot has no username, the group
  event was Silent or its send failed (no message to carry it), or one more button would pass Telegram's 100. A banned person cannot read
  the group, so for a ban the button mostly serves a public group's visitors who are the punished person: a stated limit, not a reason to
  drop it. Accepted race: an admin who presses the Unban or Unmute button in the moment between the group message and the edit can see that
  row drawn again by the edit; pressing it again finds the member already unbanned or unmuted.
- **D-20:** The deep link is `RegisterDeepLinkHandler("pmcopy_", pmCopyDetailsDeepLink)` (private chats only, as every deep link). The
  payload is attacker-controlled: it is parsed strictly (`pmcopy_` then exactly 22 characters of `A-Za-z0-9_-`), and a malformed payload, an
  unknown or expired token, a Redis error and an opener whose user ID is not the record's all get the same neutral reply
  (`eventmsg_pm_details_none`, in the opener's language), which names no group, event or person. The opener the record names gets the stored
  copy sent to their private chat (no header, the same message the copy would have been), and may open it again until the record expires.
  The token is never logged.

### Log line (default)
- **D-21:** With the log option on, a job that did not deliver posts one line in the group's log channel under the admin category
  (`actionlog.Destination(chat, logchannels.CategoryAdmin)`, so no channel or the category off posts nothing): the usual header, `#PM_COPY`
  (not translated, like `#LOCKDOWN_BAN`) and `eventmsg_pm_log` in the group's language naming the member (mention and code ID), the event's
  name (the page's name key) and why (`eventmsg_pm_why_blocked` for blocked, `eventmsg_pm_why_failed` otherwise), sent through the PM pacer.
  It names no Staff Group, carries no message content, and is never posted for a shed copy (D-18).

### Messages page (default)
- **D-22:** The event screen gains the line `settings_msg_line_pm` (Private copy: On or Off) after the Silent line and, after the Silent
  toggle, one full-width row `settings_btn_msg_pm_open` that opens the private copy's screen (`a=he&k=<code>&d=p`). Worst case eight rows,
  `[2 2 2 2 2 1 1 2]`, the private content-page budget.
- **D-23:** The private copy's screen is the event's PM editor definition drawn by its own Draw hook (`renderSettingsMsgPM`): the event's icon
  and name, `settings_msg_pm_intro`, the part lines, the no-caption note, the switch line, the family's hint and `settings_msg_hint_pm`;
  keyboard: the part rows (Set, Remove), the preview rows, the full-width switch `hp` (`settings_btn_msg_pm`, the absolute target), then Back
  to the event screen (`a=he&k=<code>`) and Close; worst seven rows. Set, Cancel and the typed reply go through the editor and the typed-reply
  door unchanged (the door finds `hbnp` through `settingsEditPageByKey`); the prompt's header is `settings_msg_pm_title`, so the owner sees the
  prompt is for the private copy.
- **D-24:** The hub gains the private copies block: after the families a blank line, `📩 <b>settings_msg_pm_title</b>`, one line per option of
  the table `settingsMsgPMOptions` (Details, then Log) and the options hint, and one full-width toggle per option (`ho`, `t` = `d` or `l`), so
  the hub rows are `[1 1 1 2]`. An event line gets the code-side suffix ` · 📩` while its private copy is on. Main's count counts an event once
  when its group message is not Default or its private copy is on.
- **D-25:** Rights: opening the page, a family, an event screen, the private copy's screen and the previews need `settingsRightAdmin` (the
  live admin check and the owner gate of every press); Set, Remove, the switch `hp` and the options `ho` need `settingsRightChangeInfo`, judged
  live on every press and on every typed message. There is no command, so each control carries a drift test and an authority test in place of
  a parity test.
- **D-26:** Callback data: the new field `d` on `he hs hr hc hv hf` (only `p`; any other value is an invalid request in step 1), the new page
  actions `hp` (exactly `k` and `v`) and `ho` (exactly `t` and `v`), every one with the private owner field `c`. Neither code collides with a
  registered code (`hl he hs hr hc hv hf hq`, the Deletion row codes, the list codes, the core codes). Worst sizes at a 14-character chat ID:
  `hs hr hc hv` with `d` 46 bytes, `he` and `hf` with `d` 42, `hp` 42, `ho` 41, all under 64.
- **D-27:** Previews of the private copy go through `composeEventPM` (the send path's own composer) with the presser as the member and the
  admin, the sample reason and, for a timed event, the sample duration and end, in the group's language; sent to the presser's private chat
  only, with the header `settings_msg_preview_header_pm`; they ignore the switch, write nothing and schedule nothing.
- **D-28:** The page strings that describe the page (`settings_msg_intro`, `settings_msg_fam_manual_intro`) are updated in all 8 languages
  to mention the private copy and that the silent variants send none.

### Security (default)
- **D-29:** Only the punished person ever gets the content: the copy's chat is the punished user's ID taken from the moderation pipeline's
  resolved target, the Details record is shown only to the user whose ID equals the record's, and a preview goes only to the presser.
- **D-30:** A private copy, a Details view and a log line never name the Staff Group or another group: they are built only from the event's
  own chat (whose events never come from a Staff Group, where the StaffActions interceptors take every punishment command and the page is
  Hidden), the default names the group's own title, and the acting admin is named only through `{admin}` under MSG-03.
- **D-31:** No log line of the private copy paths carries a stored or typed text, a caption, a file ID, a button, a reason, a name or a
  Details token: chat ID, user ID, event, outcome class and Telegram error code only. The job builds no `ReplyParameters` and calls no method
  named `Reply`, so `TestCommandRepliesStayInsideTheHandler` stays green with its empty allow-list.

### Tests (default)
- **D-32:** No new golden. The only send path this phase touches is `replyEvent` and `replySuccess`, whose requests
  `TestLegacyModerationRequests` (one commit, 31e7e18) pins; every plan that edits `event_message.go`, `moderation.go`, `bans.go` or `mute.go`
  runs it and checks the file is unchanged since its own commit. The Details edit is a new request after the group message, only with the
  copy on, the option on and the copy not delivered.
- **D-33:** PMSG-05 is held by a structural guard, `TestPMCopyCallers` (a `go/parser` walk of the non-test files of `fuku/modules`): calls of
  `offerEventPM` are listed (only `event_message.go`), calls of `registerEventPM` are listed (only `event_message_pm.go`), and no file whose name
  starts with `staff`, `lockdown` or `raid` references `offerEventPM`, `registerEventPM`, `replyEvent` or an identifier starting with
  `pmCopy`; plus `TestEventPMSpecs`, which holds the spec table to exactly the seven manual events (later phases add their rows there). The
  end-to-end test proves a Staff Group action sends no copy.
- **D-34:** `fuku/modules` is near its CI time limit (D-26 of Phase 21): worst cases, not cross products; real locales only where a default
  text is compared; one environment per subtest; fresh chat and user IDs; cleanup in `t.Cleanup` (rows, cache keys and the `fuku:pmcopy:*`
  keys of the test's chats and tokens); tests wait for jobs with a helper over `pmCopyJobs`; no `t.Skip`, no `t.Parallel`, no mock library.
- **D-35:** PostgreSQL: `TestEventMessageOptionsMigrationDefaults` joins `make test-postgres-integrity`; `TestEventMessagesUpsert`, already in its
  list, gains the `pm` default and the options upserts.

### Repo rules that bind
- **D-36:** New locale keys in all 8 files. `settings_` keys get rows in `settingsLocaleLimits` (and `TestSettingsLabelLengths` groups for
  labels); the `eventmsg_` keys get a new limits table `eventMsgLocaleLimits` in `fuku/i18n/eventmsg_locale_test.go` (`TestEventMsgLocaleKeys`),
  which covers every `eventmsg_` key, `eventmsg_admin_anon` included. Ukrainian and English follow `locale-style.md` (`TestLocaleStyle`), and
  placeholders match across languages. HTML only.
- **D-37:** AGENTS.md is updated in the commit that changes the rule (the destination and its default, the options table, the pacer and cap
  keys, the Details record and deep link, the page's codes and rights, the "no drain" shutdown note, the test families).
- **D-38:** Every new production identifier has a production caller in the plan that adds it, or a dated `//nolint:unused // first
  production caller arrives in plan 22-NN` that the named plan removes. No dependency is added (`go.mod`, `go.sum`, `main.go` unchanged).

### Planner's and Executor's Discretion
Function and test names within the prefixes above, the exact strings (inside their limits and the style guide), the order of lines inside a
screen body, and how the job's steps are split into functions.

</decisions>

<open_questions_resolved>
## Roadmap Planning Notes and Open Questions, Resolved

| Question | Resolution |
|----------|------------|
| A 403 is the normal "not delivered" answer, not an error | D-15: by code; logged at Debug; never retried; it drives the Details button and the log line |
| Own Redis pacing keys and a per-group per-minute cap counter, outside the cache prefix | D-14 (`fuku:pmcopy:pace:*`), D-17 (`fuku:pmcopy:{<chat>}:cap:<minute>`); both outside `fuku:cache:`, so `CLEAR_CACHE_ON_STARTUP` keeps them |
| Bounded pool with no new drain, or recompute the shutdown budget | D-13: bounded in-flight goroutines, no drain, budget unchanged |
| The Details record and who may read it | D-19, D-20: a Redis token behind `/start pmcopy_<token>`, 48 h, shown only to the punished user's ID, one neutral reply otherwise |
| Never PM for raid-scale removals or Staff Group actions; a guard lists every caller | D-04, D-33 |
| Each event screen gains a PM copy; the two options sit on the Messages page | D-22 to D-24 |
| Goldens first for any send site Phase 21 did not touch | D-32: none is touched |
| Where the per-event switch lives | D-08: the `pm` row's `silent` with a destination default, no new column |
| Where the Details and log options live | D-09: one new table read with the messages under the same cache key |
| Which language the private copy uses | D-12: the group's |
| Unban and unmute copies; the silent variants | D-06: unban and unmute get one; the silent variants send none |
| A copy when the group message is Silent | D-11: yes (Silent is the group destination's switch); no Details button then |
| A cap for manual events | D-17: none; the pool and the pacer bound them |

</open_questions_resolved>

<screens_and_codes>
## Screens and Callback Codes

All screens are in the bot's private chat (the page is `PrivateOnly` and `Hidden` in a Staff Group, as in Phase 21).

- **Hub** (`a=op&p=h`): intro, the families with their event lines (suffix ` · 📩` while a copy is on), then the private copies block
  (title, `settings_msg_line_pm_details`, `settings_msg_line_pm_log`, hint). Rows `[1 1 1 2]`: the family cell, the Details toggle
  (`a=ho&t=d&v=<target>`), the Log toggle (`a=ho&t=l&v=<target>`), Back and Close.
- **Family list** (`a=hl&t=m`): unchanged rows `[2 2 2 1 2]`, the event lines carry the suffix.
- **Event screen** (`a=he&k=<code>`): Phase 21's screen plus the line `settings_msg_line_pm` and the row "📩 Private copy"
  (`a=he&k=<code>&d=p`) between the Silent toggle and Back. Worst `[2 2 2 2 2 1 1 2]`.
- **Private copy screen** (`a=he&k=<code>&d=p`): intro, part lines, switch line, hints; part rows (`hs`/`hr` with `t`, `k`, `d=p`), preview
  rows (`hv` with `t`, `k`, `d=p`; `hf` with `k`, `d=p`), the switch (`a=hp&k=<code>&v=<target>`), Back (`a=he&k=<code>`) and Close. Worst
  `[2 2 2 2 2 1 2]`.
- **Prompts and refusals** of the private copy: the editor's, with Cancel `a=hc&t=<part>&k=<code>&d=p`.

| Code | Fields besides `a` and `c` | Right | Since |
|------|----------------------------|-------|-------|
| `he` | `k`, optional `d` | admin | 20 (`d` new) |
| `hs` `hr` | `t`, `k`, optional `d` | change info | 20 (`d` new) |
| `hc` `hv` | `t`, `k`, optional `d` | admin | 20 (`d` new) |
| `hf` | `k`, optional `d` | admin | 20 (`d` new) |
| `hq` | `k`, `v` | change info | 20 (group Silent, unchanged) |
| `hp` | `k`, `v` | change info | 21 (private copy switch) |
| `ho` | `t` (`d` or `l`), `v` | change info | 21 (group options) |

Deep link: `/start pmcopy_<22 characters>` (29 characters, under Telegram's 64), a URL button, so no callback byte limit applies.

</screens_and_codes>

<redis_keys>
## Redis Keys (all outside the `fuku:cache:` prefix)

| Key | Value and life | Written by | Read by |
|-----|----------------|------------|---------|
| `fuku:pmcopy:pace:next`, `fuku:pmcopy:pace:block` | the pacer's slot and shared `retry_after` block | `pmCopyPacer` | `pmCopyPacer` |
| `fuku:pmcopy:{<chat>}:cap:<unix minute>` | a counter, `EXPIRE 120` | the offer of an automatic event | the same |
| `fuku:pmcopy:det:<token>` | JSON of the record (D-19), 48 h | a job that did not deliver with Details on | the deep link handler |

The cached read `event_messages:<chat>` (in `skipLocal`) now also carries the `pm` rows and the options.

</redis_keys>

<failure_classes>
## Failure Classification of a Private Copy

| Telegram answer | Class | Retry | Details and log (when the option is on) |
|-----------------|-------|-------|-----------------------------------------|
| success | delivered | - | none |
| 403 | blocked | never | yes, log says "has not started the bot or blocked it" |
| 4xx other than 429, `media.ErrNoPermission` | refused | a custom copy: the default once | yes after the final outcome, log says "could not be delivered" |
| 429, `ratelimit.ErrRateLimited` | busy | no (the pacer already retried) | yes |
| 5xx, transport error, timeout | unknown | no | yes (the copy may have arrived; a second view is harmless) |
| shed (a full pool) or refused by the automatic cap (over the cap, no Redis, a Redis error) | not sent | - | none (D-18) |

</failure_classes>

<canonical_refs>
## Canonical References

- `AGENTS.md`: Data (`chat_event_messages`), Subsystem traps (Event messages, Scheduling the bot's own messages), Callbacks (the Messages page,
  the message editor), Permissions, Startup and shutdown (the drains and the 68 s worst case), Testing.
- `.planning/phases/21-message-engine-messages-page-and-manual-punishments/`: `21-CONTEXT.md` and the eleven summaries (identifiers below).
- `fuku/db/eventmsg/` (`events.go`, `repository.go`), `migrations/20261120110000_add_chat_event_messages.sql`.
- `fuku/modules/event_message.go` (`replyEvent`, `composeEventMessage`, `renderEventText`, `eventPlaceholderValues`, `actorIsAnonymous`),
  `event_message_apply.go` (`eventMsgWrites`, `applyEvent*`), `settings_messages.go` (the page, `settingsMsgEditDef`, `settingsMsgDefByFields`,
  `settingsMsgPreview`), `settings_edit.go` (`registerSettingsEditDef`, `settingsEditHooks`), `settings_edit_actions.go`
  (`settingsEditResolvedActions`), `settings_input_door.go`.
- `fuku/modules/moderation.go` (`replySuccess`), `mute.go` (the unmute lockdown note), `bot_updates.go` (`markAnonymousActor`).
- Pacers: `fuku/utils/ratelimit/telegram_pacer.go`, `deletionPacer` (`deletion_schedule.go`), `memberReqLookupPacerOptions` (a test swaps the
  options), `fedPaced`; the tracker: `raidPostTracker` (`raid_trigger.go`, `startBelow` in `memberreq_watch.go`).
- Deep links: `deeplink_router.go` (`RegisterDeepLinkHandler`), `noteIDDeepLinkHandler` (`notes.go`, strict parse), `settingsStartDeepLink`.
- Log posts: `fuku/utils/actionlog` (`Destination`), `sendStaffLogPost` (`staff_log.go`, a paced log post).
- Guards: `command_replies_guard_test.go` (`TestCommandRepliesStayInsideTheHandler`), `TestNoRawNewCommand` (the `go/parser` pattern).
- `.planning/ROADMAP.md` Phase 22 planning notes and success criteria; `.planning/REQUIREMENTS.md` PMSG-01 to PMSG-07.

</canonical_refs>

<owner_visible>
## Behaviour an Owner Would Notice

Nothing changes for a group that turns nothing on: every private copy is off, both options are off, and every group reply is byte-identical.
When the owner turns a copy on:
- the punished person gets a private message after the punishment and after the group message, in the group's language, only if they ever
  started the bot (D-12, D-15);
- with the Details option on and the copy not delivered, the group message gains a "📩 Details" button a moment after it was posted (D-19);
- with the log option on, an undelivered copy leaves a `#PM_COPY` line in the log channel's admin category (D-21);
- `/sban`, `/skick` and `/smute` never send a copy (D-06); unban and unmute can (D-06).
The Messages page grows a private copy row on every event screen, a private copy screen and an options block on its hub (D-22 to D-24), and its
intro texts mention the private copy (D-28).

</owner_visible>

<deferred>
## Deferred Ideas

The private copy in the person's own language (would need a per-user language read and a preview that cannot show it); a per-event Details or
log option; private copies of automatic, captcha and requirement events (Phases 23 and 24 register them); the appeal button on the private copy
(Phase 27); both options and the `pm` rows in backups (Phase 30); a custom message per language (MSG-F1).

</deferred>

---
*Phase: 21-pm-copies*
*Context gathered: 2026-11-19*
