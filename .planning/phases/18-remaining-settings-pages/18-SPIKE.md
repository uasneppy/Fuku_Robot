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
