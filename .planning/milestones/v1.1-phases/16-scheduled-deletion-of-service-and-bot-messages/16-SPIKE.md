# Phase 16: scheduled deletion, live checks and confirmations

This file collects the checks that need a live Telegram client, a real supergroup or your decision. It is for you, the
owner. No tool exists for them: you do them by hand on a test supergroup (and a second small group), with the bot as an
administrator and again as a plain member, and record each answer here.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 15 ("defer live checks,
keep going"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. Nothing here changes whether the code is safe: for every outcome (an unknown answer from Telegram, a
message that cannot be deleted, a missing right, an unreachable Telegram, a lost Redis) the bot deletes nothing it should
not, never fails a settings page and never disables a button. The checks change wording, defaults and your confidence,
not safety.

**This file records no result.** The code is written against Telegram's documented behaviour and the fakes of the tests;
what the real server does is yours to measure. An unknown answer is always "do not delete, try again later or drop the
row quietly", never a guess.

## Before you rely on it in a real group

- **Every rule is Keep or Off by default.** The migration creates no rule row, so a group deletes nothing and schedules
  nothing until an admin opens `/settings`, Deletion, and picks a delay. "Member joined" and "Member left" keep reading
  `/cleanservice`: a group that already turned it on keeps deleting those two at once, exactly as before.
- **A deleted message cannot be recovered.** Delete now removes the notice in the same moment it arrives. Start with a
  test group and one row (Pinned message at 10 minutes is a good first one).
- **The mute notice stays up to a day on purpose.** A requirement mute notice carries the "I fixed it" button, the member's
  only way out, so with any Auto punishments timer it is queued at 24 hours, whatever the timer says. After the member taps
  the button the confirmation follows the timer from the tap. With Auto punishments Off nothing is queued.
- **The time counts from when the message was sent**, never from when the rule was set. A rule set now never deletes an
  old message. A message older than 47 hours is dropped quietly (Telegram only deletes messages younger than 48 hours).
- **Staff Group items, lockdown alerts, captcha challenges and log-channel posts are never deleted** and the Deletion page
  does not exist inside a Staff Group.
- **Rolling deploy.** A replica on the previous code has no worker and no group `-5` watcher: rows written by a new replica
  wait until a new replica's worker runs (every row resumes from the database), and a rule set on the new code is ignored
  by the old replica's messages until it is replaced. Both are harmless.
- **A supergroup migration is not followed.** The rules and the queued rows of a basic group that becomes a supergroup are
  not moved: the new supergroup starts at Keep and Off, and the old group's pending rows are dropped when Telegram answers
  "upgraded". Set the rules again on the new group.

## The checks

### 1. The batch delete: one ID in `deleteMessages` that cannot be deleted

**Why:** the worker deletes up to 100 messages of a chat in one `deleteMessages` call. The pinned docs only say a message
that "can't be found" is skipped (research A4). Whether an ID that exists but cannot be deleted (a forum topic's creation
message, a message of a bot that left, a message older than 48 hours) fails the whole batch or only that ID decides how
many calls a mixed batch costs. The code falls back to one `deleteMessage` per ID after a refused batch and is safe both
ways.

**Steps:** in the test supergroup, with the bot an administrator that can delete messages, set Pinned message and Chat title
to 10 seconds. Pin three messages in a row so three notices queue within a second, and also change the title once. While
they are queued, delete one of the three notices by hand. When the worker runs, read the bot's log at debug level (one line
per chat batch, counts only) and Telegram's behaviour: were the other two deleted?

**Record:** batch with a missing ID: skipped / failed whole. Batch with a message that cannot be deleted: skipped / failed
whole. Cost in calls (the log's put-back and deleted counts): _____

### 2. The delete-right line: a plain-member bot, and a basic group's administrator

**Why:** the line on every deletion screen says the bot cannot delete when its own `getChatMember` shows no right
(A16-A6, research A2). Two facts are only known live: a bot that is just a member can still delete **its own** messages in a
group, so on the Bot messages screens the line is conservative; and a basic group's administrator may report
`can_delete_messages` false although an administrator of a basic group can delete any message (the page trusts the flag only
outside basic groups).

**Steps:** (a) Make the bot a plain member of the test supergroup, set Welcome to 1 minute, add a member and wait: is the
welcome deleted after a minute? Open `/settings`, Deletion, Bot messages: does the screen show the warning line (it should)?
Does the wording of `settings_del_no_right` read as "these settings do nothing yet" while your result says the welcome was
deleted? (b) Make the bot an administrator **without** "Delete messages": does the line show, and does Pinned message at 10
seconds fail to delete a pin notice? Does a refused service message set the 10-minute service latch (the bot's log says
nothing about it; Pinned notices stop being attempted for 10 minutes)? (c) In a **basic group** (not a supergroup), make the
bot an administrator: does `getChatMember` report `can_delete_messages` true? Does the page draw no line?

**Record:** plain-member bot deletes its own messages: yes / no. Line shown for it: yes / no. Basic-group administrator flag:
true / false. Wording change wanted for `settings_del_no_right`: _____

### 3. Phone-width cells in Hindi, Russian and Ukrainian

**Why:** every Service list cell is at most 20 code points and every choice at most 15 (checked by code points in a test),
but Hindi combining marks look wider than their count (A16-A9). Only a phone shows whether a cell wraps.

**Steps:** switch the bot's language for your account or a test group to `hi`, `ru` and `uk` and open Deletion, Service
messages and Bot messages on a phone in portrait, then a row screen (Pinned message, Member joined, Automatic punishments).
Look at these cells: hi "ऑटो-डिलीट टाइमर", "ग्रुप बैकग्राउंड", "पिन किया संदेश", "सदस्य ने छोड़ा"; ru "Участник вступил",
"Розыгрыш, подарки"; uk "Учасник долучився", "Розіграш, дарунки"; and the three-choice row at 15 code points: fr
"✅ Immédiatement", ru "✅ Удалить сразу", hi "✅ 10 सेकंड".

**Record:** a cell that wraps or is cut: (language, cell). If one does, shorten the key in place in `locales/*.yml` and its
row in `fuku/i18n/settings_locale_test.go`; no key is added.

### 4. Two replicas

**Why:** the queue is the database, every replica runs a worker, and a rule is read through a cache key in `skipLocal`.

**Steps:** with two bot processes over one PostgreSQL and one Redis: set a rule on one (Pinned message at 1 minute), then
send a pin through the group so the other replica handles the update; check the row exists (`scheduled_deletions`) and the
message is deleted **once** (Telegram shows one deletion; the bots' debug logs together show one successful call). Switch
the rule to Keep on one replica and pin again: the very next message on the other replica must write no row.

**Record:** rule seen by the other replica at the next message: yes / no. Deletions of one message: _____

### 5. A restart with queued rows

**Why:** queued rows survive a restart and a claim held by a worker that stopped is released after 2 minutes without
counting an attempt.

**Steps:** queue several pin notices at 10 minutes, stop the bot (`SIGTERM`) with rows pending, start it again before the due
time: the rows are deleted at their time. Then queue rows, `kill -9` the bot while a cycle is running (use a delay that comes
due during the kill), start it again: after at most 2 minutes the rows a dead worker held are claimed again and deleted.

**Record:** rows resumed after a clean stop: yes / no. Rows resumed after a kill, within: _____ minutes. Any message deleted twice: _____

### 6. A native-speaker read of the 57 new strings

**Why:** en is the UI-SPEC's English; es, fr, hi, id, pt, ru and uk are the executor's translations. A native reader should
confirm the words a member or admin sees. Keys: `settings_page_del`, `settings_state_del_on`, `settings_del_intro`,
`settings_del_sec_service`, `settings_del_sec_bot`, `settings_del_list_svc`, `settings_del_list_bot`,
`settings_del_hint_time`, `settings_del_hint_cleanservice`, `settings_del_hint_punish_auto`, `settings_del_no_right`,
`settings_del_right_unknown`, `settings_del_state_keep`, `settings_del_state_now`, `settings_del_state_after`,
`settings_del_delay_10s`, `_1m`, `_10m`, `_1h`, `_6h`, `_24h`, `settings_del_delay_custom`, `settings_del_currently`, the
twelve `settings_del_svc_*` and their `settings_del_desc_*` (`joined left photo title pinned video forum boost timer
background gifts other`), and the five `settings_del_bot_*` with their `settings_del_desc_*` (`welcome goodbye rules auto
manual`).

**Read these first:** the delay labels and `settings_del_state_after` in hi ("10 सेकंड", "1 मिनट", "1 घंटा", "6 घंटे",
"{delay} बाद"), ru ("10 с", "1 мин", "1 ч", "Через {delay}"), uk ("10 с", "1 хв", "1 год", "Через {delay}") and id ("dtk",
"mnt", "jam"); `settings_del_state_now` (es "Borrar ya", fr "Immédiatement", hi "तुरंत हटाएँ", id "Hapus segera", pt "Apagar
já", ru "Удалить сразу", uk "Негайно"); the **quoted right name** in `settings_del_no_right` and `settings_del_right_unknown`,
which must be what the Telegram client shows for "Delete messages" (es "Eliminar mensajes", fr "Supprimer des messages", hi
"संदेश हटाएँ", id "Hapus pesan", pt "Apagar mensagens", ru "Удаление сообщений", uk "Видалення повідомлень"); the Telegram
terms in the `settings_del_desc_*` covers (es "fijado", "videochat", "tema del foro", "impulsó"; fr "épinglé", "chat vidéo",
"sujet", "boosté"; pt "fixada", "chat de vídeo", "tópico", "impulsionou"; ru "закреплённое", "видеочат", "забустил"; uk
"закріплене", "відеочат", "забустив"; id "disematkan", "obrolan video", "mem-boost"); `settings_del_hint_punish_auto` (it
quotes the "I fixed it" button in each language's own quote marks and must equal the button's words); fr "Sanctions auto"
and "Sanctions admin" (admin stands for manual because "Sanctions manuelles" is 19 code points); es "Castigos auto", pt
"Punições auto", ru "Ручные наказания", uk "Ручні покарання" (the Member requirements page's word for punishment);
`settings_state_del_on` and `settings_del_currently` (es "{n} activas", fr "{n} actives", pt "{n} ativas", ru "включено:
{n}", uk "увімк.: {n}", hi "{n} चालू").

**Borderline lengths (one character from the ceiling):** `settings_del_sec_service` es "Avisos de servicio" 18/18;
`settings_del_sec_bot` uk "Повідомлення бота" 17/18; `settings_del_state_now` fr "Immédiatement" and ru "Удалить сразу"
13/13 (15/15 in the cell with the check mark); en "Manual punishments" 18/18. A translation that has to change must stay
inside the ceiling in `fuku/i18n/settings_locale_test.go`.

Hindi sentences are worded without a gendered first person on purpose; a native reader may prefer "मैं ... सकता/सकती हूँ".

**Record:** keys changed per language: _____

### 7. Assumptions the owner confirms: A16-A2, A16-A3, A16-A4, A16-A6, A16-A8

- **A16-A2 (hiding the page in a Staff Group).** The registry had no way to hide a page, so `settings_pages.go` gained one
  optional field, `Hidden func(group *gotgbot.Chat) bool` (6 lines, plus a 5-line guard in `openPage` in `settings_menu.go`),
  the Phase 13 precedent for one field added once. Fallback if you do not want those two files touched: draw the page
  everywhere and refuse every press in a Staff Group (the screens would still not name the Staff Group). Confirm or choose:
  _____
- **A16-A3 (state icons).** The page uses new icons `➖` (none), `🗑` (Delete now) and `⏳` (a delay) beside the word,
  because `✅`/`❌` misreads ("❌ Member joined: Keep") and `✅` cannot tell Delete now from After N. Fallback:
  `settingsStateIcon` with `✅` for any rule and `❌` for none; no key changes. Confirm or choose: _____
- **A16-A4 (no confirmation card).** Every choice writes at once, with no card and no `e` field, because a delay change is
  reversible with one tap and acts on no member. Alternative: a two-tap card for Delete now only (it removes a notice in the
  same moment). Confirm or choose: _____
- **A16-A6 (the delete-right line on Bot messages screens).** One rule everywhere: the line shows when the bot is not an
  administrator with the delete right, even though a plain-member bot can delete its own messages (check 2). Confirm or
  choose a different rule for the Bot messages screens: _____
- **A16-A8 (the key budget).** 57 keys (against 8 and 10 in the last two phases): 17 rows, each with a name and a one-line
  cover (34 keys). Six of the covers repeat their name and can be dropped one by one with no code change (rows 36 to 39, 43
  and 46 of the UI-SPEC's key table), taking the total to 306; the Pinned message and Other covers must stay (they say
  "the notice, not the pinned message" and "creation and migration notices stay"). Keep all or drop some: _____

### 8. The amendments D-20 to D-28

Each of these amends a decision you locked; they were accepted at planning and are in the code. Confirm or change:

- **D-20** `scheduled_deletions` has a `sent_at` column (the message's own send time) and a row more than 47 hours past it
  is dropped quietly with no call and no log line.
- **D-21** The Forum topics row never includes topic creation (Telegram refuses to delete that message), and the Giveaways
  row does not include the giveaway post itself (only the notices around it). See check 9.
- **D-22** The "no rights" latch is scoped: `gone` (the bot is not in the chat, was kicked, or the chat is gone or upgraded)
  stops every kind, `service` is set only after one live read of the bot shows it cannot delete and stops only service
  kinds; a refusal of one message never latches anything.
- **D-23** `StopDeletionWorker` waits at most 3 s, so the shutdown allowances add up to 68 s against the enforced 60 s
  budget in webhook mode (and the process exits with code 1 inside `stop_grace_period: 75s` when every drain runs to its
  limit). No correctness cost: every row resumes from the database. Keep `stop_grace_period` above 60 s.
- **D-24** `/cleanservice` now also covers a join or leave a **bot** caused (the new watcher handles those; before this
  phase `cleanService` never accepted a bot sender, so they were never deleted). A group that had `/cleanservice on` starts
  deleting those too.
- **D-25** With Auto punishments Off a mute notice writes no row; with any timer it is queued at 24 hours; the tap moves it
  to the timer counted from the tap.
- **D-26** `/cleanservice on` and `/cleanservice off` both remove the delay rows of joined and left, so a stale "After N"
  never comes back after the command.
- **D-27** The AI spam breaker's pause notice counts as an automatic-punishment message (`bot:punish_auto`), because it is
  aispam's only group message. You can narrow it: say so and that one call site is removed.
- **D-28** The one optional `Hidden` field of `settings_pages.go` and its guard in `settings_menu.go` (check 7, A16-A2).

**Record:** confirmed / changed, per decision: _____

### 9. Forum topics and Giveaways, live

**Why:** two rows rest on what Telegram refuses (D-21). A topic's creation message cannot be deleted, so the row covers
edited, closed, reopened and the General topic hidden or shown only; the giveaway post is content, so the row covers the
notices around it (the giveaway ended, the winners announced).

**Steps:** in a forum supergroup set Forum topics to Delete now and create a topic (the "topic created" service message must
stay), then close and reopen it (those notices go). Set Giveaways and gifts to Delete now and run a giveaway: the giveaway
post itself stays, the notices go.

**Record:** topic creation stays: yes / no. Close and reopen notices deleted: yes / no. Giveaway post stays: yes / no. Other
service types that Telegram refused (log lines of refused rows are quiet; look at the message list): _____

### 10. The joined and left coupling through one flag

**Why:** Member joined and Member left share the `/cleanservice` flag (D-02). Keep on either row, and `/cleanservice off`,
turn the flag off and remove **both** delay rows; Delete now or After N on either row turns the flag on, so the **other**
row then reads its own stored delay or Delete now. The planner's refinement of D-02 is that Keep clears both rows (so a
stale delay never comes back). It is documented in AGENTS.md ("Joined and left share one flag") and is surprising once.

**Steps:** in the test group, set Member joined to After 10 minutes: open Member left (it reads Delete now at once). Press
Keep on Member left: Member joined reads Keep too. Run `/cleanservice on` and look at both; run `/cleanservice off`. Also add
a member with a bot's help (an administrator bot adds a member) with the flag on: the join service message is deleted.

**Record:** coupling acceptable: yes / change to separate switches (a later phase would need its own flag column): _____

### 11. The real error strings of a refused delete, and a 429 on `deleteMessages`

**Why:** the classifier reads lower-cased fragments and codes (research A1, A7): "message can't be deleted", "message to
delete not found", "chat not found", "bot was kicked", "upgraded to a supergroup", and a 429's `retry_after`. A string it does
not know falls to "a 4xx other than 429 drops the row" or "retry, and the third failure drops", so a wrong guess costs a few
extra calls, never a wrong deletion.

**Steps:** with the bot's debug logging on, provoke each answer: delete a queued message by hand (not found); queue a message
older than 48 hours by editing a row's `sent_at` in a test database (too old, dropped before any call); remove the bot from
the group with rows queued (chat not found or kicked); upgrade a basic group to a supergroup with rows queued; and send enough
deletes to meet a 429 (set 100 rows due at once in a busy chat). Note Telegram's exact description for each from the Bot API
response (the bot never logs it).

**Record:** descriptions seen: _____. Strings that fell to the retry path (the third failure dropped the row): _____. A 429's
`retry_after` present on `deleteMessages`: yes / no.

### 12. DEL-07 live: nothing excluded is ever deleted

**Why:** the tests prove it with fakes (`TestDeletionExcludedMessagesNeverScheduled`, the grep gate). A live run proves no
real sender is missed.

**Steps:** in a test group linked to a Staff Group, with a log channel, a captcha and the lockdown trigger available, set
every Service row to Delete now and every Bot row to 1 minute. Then: run a staff `/ban` from the Staff Group (the card and
the summary), start `/lockdown` (the alert copies in the Staff Group), join with a new account (the captcha challenge) and do
a moderation action that posts to the log channel. Wait two minutes.

**Record:** every one of those messages is still there: yes / no. Any message that vanished: _____

### 13. Accepted limits found while building

Read these as the known edges; none needs a decision unless you want it changed.

- **A 429 on the live read of the bot costs 4 requests** (the pacer's three retries); at the production unit a `retry_after`
  of 1 s or more runs into the page's 3 s bound and ends as the unknown line. Never an error.
- **The page's right line is conservative on Bot messages screens** (check 2) and costs one `getChatMember` per draw and per
  redraw after a press.
- **The worker does not trust the administrator flag**, so in a supergroup whose administrator lacks the delete right it
  costs one refused call per service notice (bounded by the pacer) before the live check latches the chat for service
  kinds. A basic group's administrator is never latched.
- **A delete-now that meets a retry-class error loses that one message quietly** (D-08, accepted); one that the pacer sheds,
  or that comes over the inline cap of 64, becomes a queue row due now instead.
- **Delete last and the timer apply together** (A16-A10): the queue keeps the earlier time, the page says nothing about it;
  and the known Delete-last race of two welcomes on two replicas (Phase 12, Pitfall 22) is unchanged.
- **A mute notice stays up to a day** even with a short Auto punishments timer; the other automatic notices follow the
  timer directly.
- **`settings_help_msg` is unchanged** (A16-A11): it does not name the Deletion page; one line can be added later inside the
  length ceiling, in all 8 languages.
- **A supergroup migration is not followed** (see the opening) and **a rolling deploy** leaves the old replica ignoring the
  rules (see the opening).
- **A message of a bot that left, or one Telegram refuses, is dropped with no log line**; the worker logs counts per chat
  and cycle at debug level, never a message ID.

### 14. CI: lint, the full test run and the PostgreSQL integrity target

**Why:** `make lint` cannot run in the build sandbox (the installed golangci-lint is built with Go 1.25) and the
`fuku/modules` harness does not run on PostgreSQL. CI covers both.

**Steps:** after the branch is pushed, read the CI run: `make lint` (including `dupl`, `gocyclo`, `godox` and govet's
`inline` analyzer), `make test`, and `make test-postgres-integrity` with its three Phase 16 names
(`TestScheduleKeepsEarliest`, `TestClaimBatchOneWinner`, `TestScheduledDeletionMigrationDefaults`).

**Record:** green / findings: _____
