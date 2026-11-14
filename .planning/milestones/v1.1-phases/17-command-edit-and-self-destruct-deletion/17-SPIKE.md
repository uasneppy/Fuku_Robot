# Phase 17: command, edit and self-destruct deletion, live checks and confirmations

This file collects the checks that need a live Telegram client, a real supergroup or your decision. It is for you, the
owner. No tool exists for them: you do them by hand on a test supergroup (and a second small group), with the bot as an
administrator and again as a plain member, and record each answer here.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 16 ("defer live checks,
keep going"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. Nothing here changes whether the code is safe: for every outcome (an unknown answer from Telegram, a
failed lookup of an admin or of the approvals, a message that cannot be deleted, a missing right, an unreachable Telegram,
a lost Redis, a full queue) the bot deletes nothing it should not, never fails a settings page and never disables a
button. The checks change wording, defaults and your confidence, not safety.

**This file records no result.** The code is written against Telegram's documented behaviour and the fakes of the tests;
what the real server does is yours to measure. An unknown answer is always "keep the message", never a guess.

## Before you rely on it in a real group

- **Every rule starts Off.** The migration creates no rule row (this phase adds no migration), so a group deletes nothing
  until an admin opens `/settings`, Deletion, Member messages and picks a value. Commands, Edited messages and
  Self-destruct are three separate rules.
- **A deleted message cannot be recovered.** Start in a test group with Commands only.
- **Self-destruct deletes members' messages in bulk.** It needs a second tap (the card) to turn on or to change the delay,
  and it only acts on messages sent after it was switched on: nothing already in the group is touched. Switching it off
  cancels the deletions still waiting (not a batch the worker already claimed, which finishes within seconds).
- **In a very busy group some messages are kept on purpose.** Self-destruct queues at most 20,000 messages per group
  (the ceiling) and a burst that cannot be written is shed toward keeping. The ceiling supports at most this many new
  messages a second at each delay before the newest are kept: 1 min 333, 5 min 66, 10 min 33, 1 h 5.5, 6 h 0.9, 24 h 0.23.
  A lively group with a 24 h delay therefore keeps messages once 20,000 are waiting. See the owner confirmation A17-6.
- **Admins, approved members and pinned messages stay**, and so does any member whose admin status or approval could not
  be read at that moment (an unknown answer is a keep). Exemptions are judged when the message is sent or edited, not when
  it is deleted: a member approved or promoted later still loses messages already queued.
- **A command is deleted after the bot answered it**, an admin's too, and the answer stays. Another bot's command and a
  word this bot does not know are never deleted.
- **Staff Group items, lockdown alerts, captcha challenges and log-channel posts are never deleted** and the Deletion page
  does not exist inside a Staff Group; none of the three rules ever acts in a Staff Group.
- **Rolling deploy.** A replica on the previous code has no `-8` handlers, no command wrapper and a 200-row worker: rules
  set on the new code are ignored by the old replica's messages until it is replaced, and rows written by a new replica
  wait for a new replica's worker (every row resumes from the database). Both are harmless.

## The checks

### 1. Edited-message noise

**Why:** the bot keeps no copy of the original message, so any edit later than the grace counts (D-04, A17-3). How often
Telegram reports an edit nobody made (a link preview filling in, a caption touch, a reaction, a poll vote, a game or a
bot's inline keyboard update) is only known live.

**Steps:** in the test supergroup set Edited messages to 30 s. As a plain test member, send a message with a link and wait
2 minutes without touching it; send a photo with a caption; send a poll and vote on it; add a reaction to a message; edit a
message after 10 s and again after 45 s. Watch which messages are deleted. If you can, read the `edit_date` Telegram shows
for each (a bot's debug log or any client that shows "edited").

**Record:** untouched link / caption / poll / reaction message deleted: yes / no, each. Edit after 10 s kept, after 45 s
deleted: yes / no. Shortest grace you would keep on: _____

### 2. A bare `/word` in a multi-bot group

**Why:** the wrapper deletes a command only when the first word is one this bot registers (A17-1), but a bare `/word` may
also belong to another bot in the group. Deleting it could make the other bot's reply fail if that bot replies to the
command message, and what the Bot API does with a reply whose target is gone is the open question (D-01, D-03).

**Steps:** in a supergroup with this bot and a second bot that handles at least one command with the same name as one of
ours (`/help`, `/rules` or `/start` are likely), turn Commands on and send the bare `/help`. Then send `/help@otherbot` and
`/help@thisbot`. Look at which messages stay and whether the other bot's reply arrived, with or without a quoted original.

**Record:** other bot's reply to a bare shared command arrived: yes / no. Quoted original shown: yes / no. Bot API error
seen by the other bot ("message to be replied not found" or none): _____. `/help@otherbot` left alone: yes / no.

### 3. The anonymous admin's command, after the proof tap

**Why:** an anonymous admin's command runs only after the proof tap, and the wrapper deletes the command from the
re-entered update, in the order proof message deleted, the command's effect and reply, the command message deleted (D-02,
D-22). The fake proves the order; only a live client shows the real update.

**Steps:** in a group where the admin setting "Remain anonymous" is on for you and the group's AnonAdmin setting is **off**
(`/anonadmin off`), turn Commands on and send `/ban` (or any admin command) as the anonymous admin. Tap the proof button.
Then send another anonymous command and do not tap.

**Record:** after the tap the proof message is gone, the answer arrived and the command message is deleted: yes / no, each.
Without a tap the command stays: yes / no.

### 4. An anonymous admin's command in a group whose AnonAdmin setting is on (D-34)

**Why:** with the group's AnonAdmin setting on the command runs at the first update with `sender_chat` set, so the wrapper
skips it and that command message stays. This is the stated limit of D-34 and owner confirmation A17-2 as widened: the
wrapper cannot tell a fully handled anonymous command from a proof prompt. The question is whether you accept it.

**Steps:** set `/anonadmin on` in the test group, turn Commands on and send an admin command as the anonymous admin.

**Record:** the command ran at once: yes / no. The command message stayed: yes / no. Accept the limit: yes / want a fix
(a later phase would tag the update before the handlers run): _____

### 5. Pinned messages seen live

**Why:** Self-destruct never deletes a pinned message (D-11). The watcher learns of a pin from the `pinned_message` service
message; the bot's own `/pin` goes through a hook because a watcher without `SetAllowBot` never sees a bot-sent pin
service message (deviation note 3); a pin by another bot is a stated limit.

**Steps:** set Self-destruct to 1 min (confirm the card). As a plain member send three messages. (a) As an admin pin
the first one from the client within 20 seconds: after 2 minutes it must still be there and the other two gone. (b) Send a
fourth message and pin it with this bot's `/pin` as an admin: it must stay. (c) Send a fifth and pin it with **another
bot** that can pin: note whether it is deleted (expected: yes, the stated limit). (d) Pin an old message that was queued
and is already deleted: nothing happens. (e) Unpin the first message: it is not queued again.

**Record:** (a) stays: yes / no. (b) stays: yes / no. (c) deleted: yes / no. Message pinned in the first 300 ms after it
was sent deleted anyway (a race this suite cannot test live): _____

### 6. `deleteMessages` at busy-group volume

**Why:** the worker deletes up to 100 messages of a chat per `deleteMessages` call and claims 1000 rows a cycle (D-32);
the pacer's 100 ms interval is the only budget. How many calls Telegram allows before a 429, and the `retry_after` it
returns, are only known live (research S-09). Some IDs in a batch may already be gone.

**Steps:** in the test group set Self-destruct to 1 min. Have several accounts, or a script that is not part of the bot,
send a few hundred messages in a minute, delete about ten of them by hand while they wait, and watch the bot's debug log
(one line per chat batch, counts only) when the minute passes. Note the number of calls, how long the group took to
empty, and any 429.

**Record:** rows deleted per second: _____. A 429 seen: yes / no, its `retry_after`: _____. A batch holding an already
deleted ID failed whole / skipped it: _____. Did replies to commands slow down while the burst was deleted: yes / no.

### 7. A claimed batch at the moment of Off

**Why:** Off cancels the rows still waiting but leaves a batch the worker already claimed to finish (D-12, D-26,
UI-SPEC A17-A11), and the worker's own rule screen drops claimed rows after Off. The page says "still waiting" on purpose.

**Steps:** with a few hundred messages queued as in check 6 and due within the minute, press Off on Self-destruct while the
worker is deleting (watch the log). Look at how many messages are still deleted after the press and for how long.

**Record:** messages deleted after Off: _____, for _____ seconds. Wording of "still waiting" is clear: yes / no.

### 8. Phone-width cells in Hindi, Russian and Ukrainian

**Why:** every cell is at most 20 code points (checked by code points in a test), but Hindi combining marks look wider
than their count (A16-A9, A17-A13). Only a phone shows whether a cell wraps.

**Steps:** switch the language of your account or a test group to `hi`, `ru` and `uk`, and open Deletion, Member
messages and the Self-destruct screen on a phone in portrait. Look at: hi "एडिट किए संदेश" (Edited messages), "ऑटो-डिलीट" (Self-destruct)
and "सदस्य संदेश" (the section); ru "Отредактированные" and "Самоудаление"; uk "Відредаговані", "Самовидалення" and
"Відстрочка {delay}" (as shown with a delay); and the seven-button Self-destruct choice layout `[Off]`, `[1 min 5 min 10 min]`, `[1 h 6 h 24 h]`
(A17-A7).

**Record:** a cell that wraps or is cut: (language, cell). If one does, shorten the key in place in `locales/*.yml` and its
row in `fuku/i18n/settings_locale_test.go`; no key is added.

### 9. A native-speaker read of the 18 new strings and the changed intro

**Why:** en is the UI-SPEC's English; es, fr, hi, id, pt, ru and uk are the executor's translations. Keys: `settings_del_sec_member`,
`settings_del_list_mem`, `settings_del_mem_commands`, `_mem_edited`, `_mem_self`, `settings_del_desc_commands`, `_desc_edited`,
`_desc_self`, `settings_del_hint_commands`, `_hint_edited`, `_hint_self`, `_hint_grace`, `settings_del_state_on`,
`settings_del_state_grace`, `settings_del_delay_30s`, `_5m`, `_15m`, `settings_del_confirm_self`, plus the changed text of
`settings_del_intro` (it must name all three groups).

**Read these first, in order:** (1) the On and Off pairing: `settings_del_state_on` against `settings_state_off_plain`
(es `Encendido`/`Apagado`, id `Nyala`/`Mati`, hi `चालू`/`बंद`, ru `Вкл`/`Выкл`, uk `Увімк.`/`Вимк.`, fr and pt `On`/`Off`);
(2) the unit words and "grace": `settings_del_delay_30s`, `_5m`, `_15m` and `settings_del_state_grace` (es `Gracia`, fr
`Grâce`, pt `Tolerância`, id `Tenggang`, ru `Отсрочка`, uk `Відстрочка`, hi `ग्रेस`; "grace time" is the least certain term
in every language and `{delay}` must read as a phrase in each word order); (3) the phone-width cells of check 8; (4) the
section names shortened to 18 (es `De miembros`, pt `De membros`, ru `От участников`, uk `Від учасників`) and "approved
members" in each language's Approvals word (es aprobados, fr approuvés, hi स्वीकृत, id disetujui, pt aprovados, ru
одобренные, uk затверджені); (5) `settings_del_desc_commands` and `settings_del_hint_commands` ("my reply stays", "another
bot"), the three self-destruct texts (the pinned wording) and `settings_del_intro`.

**Record:** keys changed per language: _____

### 10. Two replicas with the batcher on both

**Why:** each replica has its own batcher; the queue is the database; the rules are read through a cache key in
`skipLocal`; a message edited or sent is handled by whichever replica receives it.

**Steps:** with two bot processes over one PostgreSQL and one Redis, set Self-destruct to 1 min, send messages so that both
replicas handle updates, and check the rows in `scheduled_deletions`: one row per message, deleted **once** (Telegram shows
one deletion, the two debug logs together show one successful call). Switch Self-destruct Off on one replica and send a
message: the other replica must write no row at the very next message.

**Record:** rule seen by the other replica at the next message: yes / no. Rows per message: _____. Any message deleted twice: _____

### 11. A restart with entries in the batcher and rows in the queue

**Why:** `StopDeletionWorker` flushes the batcher for at most 2 s on a context detached from the cancelled root; entries
that were never written are lost and their messages stay (a stated limit, no message is ever deleted wrongly). Queued rows
survive a restart, and a claim of a worker that died is released after 2 minutes without an attempt.

**Steps:** (a) send a burst and `SIGTERM` the bot within a second: are the burst's messages deleted at their time after the
restart? (b) Queue rows with a 5 min delay, `SIGTERM`, start again before they are due: deleted at their time. (c) `kill -9`
the bot during a cycle and start it again: after at most 2 minutes the rows a dead worker held are deleted. Note how many
messages from the last 300 ms before a `kill -9` were never queued.

**Record:** after a clean stop all deleted: yes / no. After a kill, within: _____ minutes. Messages lost (kept) by the kill: _____. Any deleted twice: _____

### 12. A plain-member bot: one check, then silence

**Why:** Commands, Edited messages and Self-destruct all need the right to delete other people's messages. The first inline
delete refused with a message-level answer triggers one live read of the bot, the shared `service` latch is set when it
cannot, and then no rule makes any further call for 10 minutes (D-24). A plain-member bot cannot delete members' messages.

**Steps:** make the bot a plain member of the test supergroup, turn Commands on, send a command: note the one refused call
and the log. Send ten more commands, edit a message after the grace and send a message with Self-destruct on: no further
call should appear for 10 minutes. Then make the bot an administrator with "Delete messages" and check the rules act again
after the latch expires (10 minutes) or after a restart.

**Record:** calls after the first refusal in 10 minutes: _____. Rules work again after the latch: yes / no. Time it took: _____

### 13. The delete-right line live on the Member messages screens

**Why:** DEL-08: every deletion screen says when the bot cannot delete. On member screens the shared line is true of all
three rules (A17-A5), softer than strictly needed ("some of these settings may not work"). The line costs one live
`getChatMember` of the bot per draw.

**Steps:** with the bot a plain member, then an administrator without "Delete messages", then with it, open Deletion, Member
messages, a row screen and the Self-destruct card: does the line show exactly in the first two cases? Does the line read
well for a rule that cannot work at all? In a basic group, does an administrator bot show no line?

**Record:** line shown correctly in all three: yes / no. Want a stronger member-section text (adds two keys, A17-A5): yes / no

### 14. A cold admin cache (D-33)

**Why:** an admin's edit or message is judged by the cached admin list when it is known, and otherwise by one bounded live
`getChatMember` (8 at a time, 3 s each); a failed read keeps the message. A group whose cache is cold (a restart, a long
quiet time, an admin change) therefore costs live reads for a short while, and an admin's message is never deleted because
a read failed.

**Steps:** restart the bot, then immediately, as an admin, edit an old message past the grace with Edited messages on, and send
a message with Self-destruct on; as a plain member do the same. Repeat while Telegram is slow or while the bot's network is
cut for a few seconds (for example block the Bot API briefly). Watch the number of `getChatMember` calls in the first minute
and how long the cache stays cold.

**Record:** the admin's edit and message are kept while the read fails: yes / no. Live reads in the first minute: _____. Cache
warm after: _____ seconds. A burst of edits past 8 at once kept (the semaphore): yes / no.

### 15. The owner confirmations A17-1 to A17-8, and the flagged UI points

Each was recommended at planning and is in the code. Confirm or change:

- **A17-1 (D-01)** Commands this bot does not register, and commands for another bot, are never deleted. _____
- **A17-2 (D-02, D-34)** An anonymous admin's command is deleted only after the proof tap ran it; in a group whose AnonAdmin
  setting is on there is no tap and that command stays (checks 3 and 4). _____
- **A17-3 (D-04)** An edit that changes nothing counts like any edit past the grace (check 1). _____
- **A17-4 (D-05, D-30)** Grace presets 30 s, 1 min, 5 min, 15 min and 1 h. There is NO stored or preselected default: Edited
  messages starts Off and the owner taps a preset, so the roadmap's "1 minute by default" holds only as 1 min being the second
  preset on the screen (the code that would have stored 60 s on switch-on was removed, D-30, because the page offers presets and
  has no switch-on-without-a-value caller). Decide: keep it (the owner always picks a value) or ask for a one-tap On that stores
  1 min (a small change in `settings_deletion.go`, no migration). _____
- **A17-5 (D-07, D-08)** Self-destruct rows in PostgreSQL with a bounded batcher that sheds toward keeping the message. _____
- **A17-6 (D-09, C-13)** Self-destruct presets 1 min to 24 h and a 20,000 row per-chat ceiling, with the rate table in the
  opening of this file (1 min 333/s, 5 min 66/s, 10 min 33/s, 1 h 5.5/s, 6 h 0.9/s, 24 h 0.23/s). Confirm the ceiling and the
  presets, or choose a different ceiling, or per-delay ceilings: _____
- **A17-7 (D-10, D-11)** Self-destruct leaves the bot's own messages, held captcha messages and a message named by a pin
  service message; unpinning does not re-queue. _____
- **A17-8 (D-15)** Only Self-destruct draws a confirm card; Commands and Edited messages write at once. _____
- **A17-A3** The card's No returns to the Deletion hub (the Phase 13 precedent), not the Self-destruct screen. _____
- **A17-A5** The shared delete-right line is reused on member screens (check 13). _____
- **A17-A6** `settings_del_intro` text changed to name members' messages (one locale edit in 8 files, no new key). _____
- **A17-A7** Self-destruct's seven choices are laid out `{1,3,3}`. _____
- **A17-A11** No notice after switching Self-destruct Off; the hint says "still waiting". _____
- **A17-A16** The hint keeps "In a very busy group some messages may be kept." _____

### 16. The amendments D-21 to D-34 and the three deviation notes

Each amends a decision you locked; they were accepted at planning and are in the code. Confirm or change:

- **D-21** `helpers.NewBotCommand` records every registered command name (131 module sites plus 3 helper sites replaced
  mechanically); a `go/parser` guard fails on any other construction, and the docs generator follows the shim.
- **D-22, D-23** The command wrapper sits outside the tracing processor, uses a fresh context, reads the post-run state and
  never deletes on a panic path; no reply path was edited, the delete follows the answer.
- **D-24** The delete-right latch and live right check now cover `member:*` kinds as well as `service:*` (a widening of its
  meaning; the stored value and the key are unchanged). A bot kind never needs the right.
- **D-25** Two delay validators: by value for a rule (`member:commands` 0, `member:edited` 30/60/300/900/3600,
  `member:selfdestruct` 60/300/600/3600/21600/86400) and for a queue entry; a hand-written other value reads as Off.
- **D-26** The batcher's flush and the worker both screen self-destruct rows against the current rule, so a row that was not yet
  claimed and screened is never deleted after Off; a batch the worker had already claimed and screened can finish within its
  cycle (up to the 90 s guard), like Phase 16's accepted NEW-01 (SEC-17-05).
- **D-27, D-31** The watcher only offers to the batcher; sheds toward keeping, counted per reason, one Warn per chat per hour.
- **D-28, D-29** One module, `MemberDeletion`, at handler group `-8` (free before), two handlers that never end an update.
- **D-30** There is no implicit 60 s default anywhere on the page.
- **D-32** The worker claims 1000 rows a cycle (was 200) and a chat costs one call per 100 rows; the only change to the
  claim size.
- **D-33** A failed approvals or admin read keeps the message (tri-state exemptions); a failed ceiling count sheds the chat's
  entries.
- **D-34** An anonymous admin's command with AnonAdmin on stays (check 4).
- **Deviation note 1 (17-01)** The Staff Group read of the batch flush lives inside `deletion.ScheduleBatch`.
- **Deviation note 2 (17-04)** A fifth latch site, the worker's pending-row drop after a `cannot` answer, covers the member
  prefix too.
- **Deviation note 3 (17-07)** The bot's own `/pin` drops the self-destruct row through a hook in `pins.go`; a pin by
  another bot is a stated limit (check 5).

**Record:** confirmed / changed, per item: _____

### 17. DEL-07 live with all three rules on

**Why:** the tests prove it with fakes (`TestMemberDeletionExcludedMessages`, `TestMemberDeletionStaffGroupNeverTouched`, the
structural file scan). A live run proves no real sender is missed.

**Steps:** in a test group linked to a Staff Group, with a log channel, a captcha and the lockdown trigger available, turn
on Commands, Edited messages at 30 s and Self-destruct at 1 min (confirm the card). Then: run a staff `/ban` from the Staff
Group (the card and the summary), start `/lockdown` (the alert copies in the Staff Group), join with a new account (the
captcha challenge) and do a moderation action that posts to the log channel. Edit a message in the Staff Group after a
minute. Wait two minutes.

**Record:** every one of those messages is still there: yes / no. Any message that vanished: _____

### 18. Accepted limits found while building

Read these as the known edges; none needs a decision unless you want it changed.

- **An anonymous admin's command with AnonAdmin on stays** (D-34, check 4).
- **A bare `/word` shared with another bot** may break that bot's reply (check 2); `/word@otherbot` and an unknown word are
  left alone.
- **No copy of the original message is stored**, so any edit past the grace counts (A17-3).
- **A pin that lands while the message is still in the batcher's buffer (about 300 ms), or after the worker claimed its row,
  still deletes it**; a pin by another bot is never seen; unpinning does not re-queue.
- **Exemptions are judged at send time**: a member approved or promoted later still loses messages already queued.
- **A command message deleted by command deletion leaves a self-destruct row** (if Self-destruct is also on) whose later
  delete is refused quietly with no message ID logged.
- **A held member's command, a fed-banned user's, and a command a handler already deleted (`/del`)** cost one refused call.
- **The reply guard** (`TestCommandRepliesStayInsideTheHandler`) sees calls by function name and the caller's own receiver
  only; a reply reached through another object's method would be invisible to it (reviewed in plan 17-11: no real case).
- **A cold admin cache costs bounded live reads** (check 14); a burst beyond the semaphore keeps its messages.
- **The shutdown arithmetic is unchanged**: 68 s of allowances against the enforced 60 s budget, `stop_grace_period: 75s`;
  the batcher stops inside `StopDeletionWorker`'s 3 s wait.
- **`settings_help_msg` is unchanged** (A17-A14); one line can be added later inside its length ceiling in all 8 languages.

### 19. CI: lint, the full test run and the PostgreSQL integrity target

**Why:** `make lint` cannot run in the build sandbox (the installed golangci-lint is built with Go 1.25) and the
`fuku/modules` harness does not run on PostgreSQL. CI covers both.

**Steps:** after the branch is pushed, read the CI run: `make lint` (including `dupl`, `gocyclo`, `godox` and govet's
`inline` analyzer), `make test`, and `make test-postgres-integrity` with its four Phase 17 names
(`TestScheduleBatchKeepsEarliest`, `TestScheduleBatchDuplicateKey`, `TestCountPending`, `TestDropPendingMessage`).

**Record:** green / findings: _____
