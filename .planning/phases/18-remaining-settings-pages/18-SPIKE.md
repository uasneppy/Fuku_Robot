# Phase 18: the remaining settings pages, live checks

This file collects the checks that need a live Telegram client, a real supergroup or your decision. It is for you, the
owner. No tool exists for them: you do them by hand on a test supergroup (and a second small group), with the bot as an
administrator and again as a plain member, and record each answer here. It grows with the phase: the Warnings page's
checks come first, and each later page adds its own below.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 17 ("defer live checks,
keep going"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. Nothing here changes whether the code is safe: for every outcome (an unreadable setting, a refused
press, a bot that is not an administrator, an unreachable Telegram, a lost Redis) a page shows the cannot-open edit or a
plain notice, a refused press writes nothing and no button is ever disabled. The checks change wording, defaults and your
confidence, not safety.

**This file records no result.** The code is written against Telegram's documented behaviour and the fakes of the tests;
what the real server does is yours to measure.

## Before you rely on it in a real group

- **The pages change the same settings as the commands.** A value set on a page is the value `/setwarnlimit` and
  `/setwarnmode` read, and the reverse; there is no second copy and no migration.
- **A Warnings change applies at the next warn.** Warns a member already has are kept; the new limit and action are used
  the next time that member is warned. Nothing is applied or cleared when you change the page.
- **The bot must be an administrator to change the warn settings from the page**, as for the commands; the page says so
  with the bot's "I'm not an admin in this chat" notice and writes nothing.

## The checks

### 1. The warn limit and each action reached in a real supergroup

**Why:** the page stores exactly what the commands store, but only a live group shows the action being applied to a real
member and the warns being cleared afterwards.

**Steps:** in the test supergroup open `/settings`, Warnings. Set the limit to 2 and the action to Mute. As a plain test
member, get warned twice (an admin replies `/warn` twice). Check the member is muted and `/warns` shows none left. Repeat
with Ban (the member is banned; unban them) and with Kick (the member is removed and can rejoin). Then set the limit to 1
and 100 and confirm the stepper shows the toast at each end and edits nothing.

**Record:** Mute applied at the limit and warns cleared: yes / no. Ban: yes / no. Kick: yes / no. `/warnings` shows the
same limit and action as the page after each change: yes / no.

### 2. A stored `tban` shows the inert line and does nothing at the limit

**Why:** the table's CHECK allows `tban`, `tmute` and an empty action, which the warn module does not act on. They cannot
be set by any command or button, only by a database edit or a backup import, so the page says plainly that the action is
not acted on (the inert line) and offers Mute, Ban and Kick to replace it.

**Steps:** in a test group set the warn mode by hand in the database (`UPDATE warns_settings SET warn_mode = 'tban'
WHERE chat_id = <group>;`), then open Warnings. Check the page shows the stored word, the inert line and no marked choice.
Warn a test member up to the limit and confirm nothing is applied to them. Press Mute and confirm the line is gone and
the next warn at the limit mutes.

**Record:** inert line shown: yes / no. Nothing happened at the limit: yes / no. Mute replaced it: yes / no.

### 3. `/reset warnings` stores an empty action

**Why:** the reset module writes an empty warn mode (`backup.go`), which the warn module does not act on, so a group reset
this way stops applying an action at the limit until someone picks one. The page shows that state as Not set with the inert
line. Phase 18 does not change the reset (the decision deferred it): the follow-up is for you to schedule.

**Steps:** in a test group run `/reset warnings` as the creator, open Warnings and check the action reads Not set with the
inert line. Pick Mute and confirm it is stored.

**Follow-up to decide:** store `mute` in the reset instead of the empty string (one line in `backup.go` and a backup test),
outside this phase. **Record:** your decision: _____

### 4. The AI line of the Switches page in each configuration, and a stale On press

**Why:** the page decides whether the AI spam filter can run from the bot's configuration alone (a provider value and the
global switch), never from a call. Only a live bot shows the three states in a real group and what a stale button does.

**Steps:** (a) with `TYPESAFE_API_KEY` set and `ENABLE_AISPAM=true`, open `/settings`, Switches in a test group. Check the AI
block shows the state sentence followed by the data notice and the AI toggle is drawn. Turn it on and off with the button and
compare `/aispam` after each. (b) Restart the bot with `TYPESAFE_API_KEY` empty. Open Switches again: the AI block must show
the state sentence and a line starting with a stop sign saying the filter is not available on this bot, with no data notice
and no name of a variable or provider; with the filter stored off the AI toggle must be gone. (c) With the filter stored on,
the toggle must still be there and Off must work. (d) Repeat (b) with the key set and `ENABLE_AISPAM=false`. (e) Keep an
older Switches message open from step (a), restart as in (b), and press its On button: the bot must answer an alert saying
the filter is not available and the message must redraw without the AI toggle, with nothing stored.

**Record:** (a) the notice and toggle shown: yes / no. (b) the unavailable line and no notice, toggle gone: yes / no.
(c) Off worked with the toggle shown: yes / no. (d) same as (b): yes / no. (e) alert, redraw, nothing stored: yes / no.

### 5. Auto-approve's first press on a group with no greetings row, on PostgreSQL

**Why:** the first write of Auto-approve creates the whole greetings row (Welcome and Goodbye on), exactly as
`/autoapprove on` does. The Welcome and Goodbye pages read a missing row as those same defaults for a group the bot knows,
which the tests prove on SQLite. A group the bot has not met reads Goodbye as off before the first write and on after it,
for any first greetings write; every group the bot serves is known, so only a fresh database shows it.

**Steps:** on a PostgreSQL deployment, pick a group that has no `greetings` row (`SELECT 1 FROM greetings WHERE chat_id =
<group>;` returns nothing). Open Welcome and Goodbye and note the Welcome, Goodbye and Delete last lines. Open Switches and
press Auto-approve. Open Welcome and Goodbye again.

**Record:** the Welcome and Goodbye lines unchanged by the press: yes / no. The row now exists with `auto_approve` true:
yes / no.

### 6. Reports on and off from the page and the `/report` path

**Why:** the page writes `enabled` and `status` through `/reports`' own function, and the report path reads `status`.

**Steps:** in a test group with an admin and a plain member, open Switches and turn Reports off. As the member reply to a
message with `/report` and check no admin is notified. Turn Reports on from the page and repeat: the admins must be
notified. Compare `/reports` after each change.

**Record:** off: nobody notified: yes / no. On: admins notified: yes / no. `/reports` agrees with the page: yes / no.

### 7. The Language page redraws a group menu on two replicas within 10 s

**Why:** a group menu redraws in the new language from the code just written, but `chat_lang`, `chat_settings` and `chat` are
not in `skipLocal`, so another replica can answer in the old language for up to `CACHE_LOCAL_TTL` (10 s by default), exactly
as after `/lang`. Only a live deployment with two replicas shows how long the old language lasts.

**Steps:** with two replicas behind one bot, open `/settings` in a test group, Language, and pick another language. Send
`/lang` and any command that replies, repeatedly, for about 15 seconds, and note which language each reply is in and when it
changes.

**Record:** the page, its notice and its Back and Close redrawn in the new language at once: yes / no. The longest time a reply
came in the old language: ___ s (accepted up to 10 s).

### 8. A private menu keeps the opener's language and never writes it

**Why:** the page's private menu names the group's language and writes it, while the opener's own language (`/lang` in
private) is a different setting that the page must never change.

**Steps:** with your own language set to English (`/lang` in a private chat with the bot), open `/settings` in private, pick a
group that answers in another language, Language, and pick a third language. Then send `/lang` in the private chat.

**Record:** the page stayed in English and named the group's new language: yes / no. `/lang` still says your own language is
English: yes / no.

### 9. The eight language cells on a narrow phone in Hindi, Russian and Ukrainian

**Why:** each cell is a flag, a space and the language's own name (at most 20 code points, `Bahasa Indonesia` the longest) and
the stored one shows a check mark in place of its flag. Telegram clips a button label to the screen width, which only a real
phone shows.

**Steps:** set the group to Hindi, then Russian, then Ukrainian and open the Language page on a narrow phone (about 320 dp wide)
in a group menu and a private menu.

**Record:** every cell readable without clipping, two to a row: yes / no, per language. The check mark replaces the flag on the
stored one: yes / no.

### 10. The flag width of `Bahasa Indonesia`

**Why:** the Indonesian flag is two regional-indicator code points and renders as one wide glyph on some clients and as the two
letters `ID` on others; the cell length limit counts code points, not glyphs.

**Steps:** open the Language page on Android, iOS and Telegram Desktop and look at the Bahasa Indonesia cell, unmarked and then
marked.

**Record:** the cell fits on one line on each client: yes / no. The marked cell (`✅ Bahasa Indonesia`) fits: yes / no.

### 11. Which posts stop when each log category is off

**Why:** the page flips one category at a time exactly as `/nolog` does, and only a real log channel shows that the bot's own
posts (admin actions, user actions, automated, reports, other) stop for the category turned off.

**Steps:** link a real log channel with `/setlog`, open `/settings`, Log channel, and turn off one category at a time. After
each, cause an event of that category (a `/ban`, a member joining, a blacklist hit, a `/report`, a lockdown) and look at the
channel; turn the category back on and repeat.

**Record:** for each of the six categories, nothing is posted while it is off and the post returns when it is on: yes / no. Accepted (T-18-32): with the Admin actions category off the Staff Group and lockdown copies of the log channel are silent too, exactly as after `/nolog admin`; the page's hint says what it controls.

### 12. The `#SETTINGS` line from the page

**Why:** a change made on the page posts the same audit line as `/log` and `/nolog`, with the presser as the admin; only a real
channel shows the line as an administrator reads it.

**Steps:** with the Settings category on, turn the Admin actions category off on the page, then on. Compare with the lines
`/nolog admin` and `/log admin` post.

**Record:** the two lines from the page match the commands' (same words, the presser named): yes / no. Turning the Settings
category off posts no line: yes / no.

### 13. No channel title or ID in a group menu

**Why:** a group menu is read by every member while the log channel is private to the administrators, so the page must never
show which channel the log goes to.

**Steps:** with a log channel linked, open the Log channel page in the group as an administrator and as the owner, in a group menu
and in a private menu, and press a category. Read every line, button and toast. Then unlink with `/unsetlog` and press a cell.

**Record:** no line, button or toast names the channel or its ID: yes / no. The page says Linked or Not linked and a press without
a channel is refused with the plain notice: yes / no.
