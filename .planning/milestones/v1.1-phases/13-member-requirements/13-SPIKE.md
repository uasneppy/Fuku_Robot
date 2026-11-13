# Phase 13: Member requirements, real-device and live checklist

This file collects the checks that need a live Telegram client or a real group. It is for you, the owner. There is no tool
to run: you work through the numbered checks below on a test supergroup and record each answer here.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 12 ("Plan and build now,
check later"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards.

## Before you enable Mute in a real group

Enforcement is ON in code. Since plan 13-13 the kill switch `memberReqEnforcementLive` is true by default (stored once in
`memberreq.go`'s `init()`), so the watcher, the join check, the "I fixed it" button and the captcha guard all run in every
group. A group still screens nobody until its owner sets a rule, because every rule defaults to Off, so nothing changes
anywhere until someone opens Requirements and picks a punishment.

Before you set any rule to **Mute** in a real group, run these live checks on a test group first:

- **A2** (check 2): the marker `until_date` of a 330-day restriction reads back within 2 seconds. The button recognises its
  own mute only through that marker.
- **A3** (check 3) and **A4** (check 4): what Telegram reports when a muted member leaves and rejoins. The rejoin reset
  depends on it.
- **A4 for restricted members** (check 4, second half, and check 14): how a restricted member who is still in the group is
  read, because a Mute is refused for a member restricted below the group's defaults (see "Accepted limits" below) and a
  member lifted to the defaults of a restrictive group must read as the same.
- **A8** (check 8): how long a callback stays answerable. The button's flow is bounded to 10 seconds.
- **A5** (check 5): the lockdown unmute path, a member unmuted during a lockdown must still be unable to send.

**Warn, Kick and Ban do not depend on the marker.** A warn goes through the warn module, a kick is the member-removing
unban and a ban is a plain `banChatMember`; none of them reads `until_date` back. You can use those three punishments
before the Mute checks are done. Only Mute (and so the "I fixed it" button, which exists only on a mute notice) rests on
A2, A3, A4, A5 and A8.

## What the automatic tests prove, and what they cannot

The test suite fakes Telegram with a hand-written bot client and runs on SQLite and miniredis (PostgreSQL in CI). So it
proves the server logic: who is checked, what is punished once across replicas, what is written and sent, who may press
what, that every screen and answer fits 3800 UTF-16 units and 64-byte buttons in 8 languages, and that nothing names the
Staff Group. It cannot prove how Telegram itself behaves (rate limits, what a restriction reads back as, leave and rejoin
updates) or how a screen renders on a phone. Those are the checks below.

## Before you start

1. **A test supergroup** with the bot as an administrator (Restrict members and Delete messages rights), and a second test
   group. Use groups that are not your real communities.
2. **Accounts besides the bot**: the creator of the test supergroup (the owner), one administrator, and several plain test
   accounts that can join and leave. One account with no last name, one with no username, one with an Arabic name, one with
   a Russian name using ы, ъ, э or ё, one with a Chinese name.
3. **Four kinds of client**: Android, iOS, Telegram Desktop and Telegram Web.
4. **Phones set to Hindi, Russian and Ukrainian** (or test accounts with those languages) for the truncation check.
5. **The bot's log open.** It logs refusals and failed sends by chat, user and hold IDs only; it never logs a name, a rule
   text or a notice text.
6. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a
   screenshot.
7. **Restore anything you change** in a borrowed group (the rules, the switches, restrictions).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it
passes or when you have recorded what you changed.

### 1. A1: the group's message limit and the cap

- [ ] done

Steps: in the test supergroup set Last name to Mute (Yes on the card). Then have 25 different accounts without a last name
send one message each within a minute (or send 25 notices from a scratch bot command). Record the number of notices
delivered, the first 429 and its `retry_after`, and whether the pause notice (`mreq_notice_paused`) appeared once.

Pass looks like: the bot posts at most 10 notices in the minute, then one pause notice, then nothing for 10 minutes; no 429
appears, or the one that appears is absorbed by the shared pacer (100 ms spacing, 3 retries).

If not: if Telegram rate-limits the group below 10 a minute, lower `memberReqCapPerMinute` (a variable) or the pacer's
interval; if there is plenty of room, the cap may be raised. Record the first 429's `retry_after`.

### 2. A2: a 330-day restriction reads back with its marker

- [ ] done

Steps: mute a test account through the requirements (Last name on Mute, the account has no last name, send a message). Then
call `getChatMember` for that account (or read the bot's debug log) and compare `until_date` with the hold's `mute_until`
(visible in the `chat_requirement_holds` row).

Pass looks like: `status` is `restricted` and `until_date` is within 2 seconds of `mute_until` (now plus 330 days).

If not: the button never recognises its own mute (`mreq_fixed_not_ours` on every press). Record the difference and widen the
tolerance in `isMemberReqMute`, or stop using Mute.

### 3. A3: a muted member leaves

- [ ] done

Steps: with the account from check 2 still muted, leave the group from that account, then read `getChatMember` for it.

Pass looks like: `status` is `restricted` and `is_member` is false.

If not: if Telegram drops the restriction on leave, the rejoin reset's lift is a harmless no-op and the member is simply
checked again on rejoin. Record what status Telegram reports.

### 4. A4: the leave and the rejoin arrive as restricted-to-restricted updates

- [ ] done

Steps: continue from check 3. Watch the bot's log, rejoin with the account, and note (a) the `chat_member` updates the bot
receives (old and new status), (b) whether a `new_chat_members` service message follows the rejoin, (c) whether the join
reset lifted the mute (the member can chat again, or is checked again with a fresh notice). Then restrict a second test
account by hand as an administrator (for example mute it for a day with `/mute`) while it still breaks a rule, and have it
send a message.

Pass looks like: both updates are restricted to restricted (so `leftMember` and `newMember` do not run), the rejoin still
sends a service message, and the reset lifts only a mute that carries the hold's marker. For the hand-restricted account a Mute rule
places no mute (it never replaces the administrator's restriction, check 14); Warn, Kick and Ban still act.

If not: if no service message follows a muted member's rejoin, the reset never runs for them and the member stays muted until
"I fixed it" or an admin's `/unmute`. Record which groups behave so.

### 5. A5: during a lockdown a member unmuted with all rights still cannot send

- [ ] done

Steps: lock the test group with `/lockdown`, have a muted test account press "I fixed it" after fixing its profile, then try
to send a message from it. Lift the lockdown and try again.

Pass looks like: the member is told the group is locked right now (`mreq_fixed_done_lockdown`), cannot send during the
lockdown and can send after the lift.

If not: if the member can talk in a locked group, the unmute during a lockdown needs a refusal instead (D-09). Record it.

### 6. A6: an account whose only username is collectible

- [ ] done

Steps: with Username on Warn, join with an account whose only username is a Fragment collectible username, and send a
message.

Pass looks like: `User.username` is filled and the account is not warned.

If not: such an account is wrongly caught by "must have a username"; record it and, if it matters, accept it or ask for a
follow-up. Skip the check if no such account is available.

### 7. A7: names are current right after a rename

- [ ] done

Steps: mute an account for a missing last name, rename the account (add a last name), and press "I fixed it" at once.

Pass looks like: the live `getChatMember` shows the new name and the member is unmuted. If it still shows the old name, the
member is told "Not fixed yet" and a second tap a moment later works.

If not: record how long the old name stays; the button already answers `mreq_fixed_still_broken` and keeps the hold.

### 8. A8: how long a callback stays answerable

- [ ] done

Steps: press "I fixed it" normally and note the time to the answer; then, with a slow Telegram connection or a deliberately
delayed answer in a test bot, find when `answerCallbackQuery` starts to fail.

Pass looks like: the flow's own bound of 10 seconds (`memberReqFixedTimeout`) is inside the window, so the member sees an
answer, not a spinner.

If not: shorten `memberReqFixedTimeout`.

### 9. The bidi isolates with an Arabic name

- [ ] done

Steps: on Android, iOS and Telegram Desktop, trigger a notice for an account with an Arabic (or Hebrew) first name and read
the whole sentence, in English and in Russian.

Pass looks like: the sentence is not reordered: the name sits inside its own isolates (U+2068, U+2069) and the words around
it keep their order, the mention is tappable.

If not: record the client and version and a screenshot (without real names). The isolates are added in `memberReqMention`.

### 10. The seven cells and the choice rows in hi, ru and uk

- [ ] done

Steps: with phones set to Hindi, Russian and Ukrainian, open `/settings`, the Requirements page, then a rule screen. Look at
the rule cells (two per row, the last alone), the Check on join and Delete messages switches and the three-cell choice row
`[Kick | Ban | Mute]`.

Pass looks like: no label is cut off with an ellipsis; the Hindi cells with combining marks are readable.

If not: shorten the label of the cut cell in that language (the code-point tests bound them, the phone is the real test).

### 11. A native-speaker read

- [ ] done

Steps: have a native reader go through every new `settings_mreq_*`, `settings_btn_mreq_*` and `mreq_*` key and the changed
help in es, fr, hi, id, pt, ru, uk (the 25 `mreq_` keys and the 36 settings keys of the page, plus the changed help). Pay
attention to the sentence that the name rules go by letters and can catch related languages.

Pass looks like: the wording reads naturally and says what the English says; no fact is added or lost.

If not: correct the locale file in all affected languages and run `make check-translations`.

### 12. Two live replicas over one PostgreSQL

- [ ] done

Steps: run two bot instances against one PostgreSQL and one Redis. In a group with Last name on Mute, send ten messages in a
row from one violating account, then, with a second violating account, join and send a first message at once.

Pass looks like: one punishment and one notice for the first account, one for the second, however the messages fall on the
two replicas. No second warn, notice or mute follows.

If not: record the logs of both replicas (chat and user IDs only). The latch is `memberreq.ClaimHold`, a conditional insert.

### 13. CI: lint, the full test run and the PostgreSQL integrity target

- [ ] done

Steps: on a golangci-lint built with Go 1.26 run `make lint`; let CI run `make test` and `make test-postgres-integrity`
(which now includes `TestChatRequirementsSettings`, `TestRequirementHoldClaimConcurrent` and `TestRequirementHolds` over
`./fuku/db/memberreq`).

Pass looks like: all three green. The local sandbox could not run `make lint` (golangci-lint is built with Go 1.25) and had no
PostgreSQL for the integrity target, so CI is the authority for both.

If not: fix what the log names; a `dupl`, `gocyclo` or `godox` finding is on lines the phase added.

### 14. CR-01: a restricted member is not muted, and a member lifted to the defaults still is

- [ ] done

Steps: in the test supergroup set Last name to Mute and make the group's default permissions restrictive (for example no
media, links or polls, messages allowed). (a) As an administrator restrict a test account with no last name by hand (`/mute`
for a day, then separately restrict only its media) and have it send a message while it is restricted. (b) Take a second test
account with no last name through the captcha (or `/unmute` it after a mute) so it is restored to the group's defaults, then
have it send a message and, separately, rejoin.

Pass looks like: (a) the account stays restricted exactly as the administrator left it, no new restriction, no notice and no
hold; the log has one Info line `already restricted by someone else` with chat and user IDs only. (b) The account is muted
with the requirement mute and gets the notice with "I fixed it": Telegram reads a member restored to the group's defaults
back as having the group's rights, so the bot does not take it for someone else's restriction.

If not: if (b) is not muted, Telegram reads a restored member with fewer rights than `getChat` reports (compare the
`getChatMember` flags with the group's `permissions` and tell the owner), and `memberReqRestrictedBelow` needs a looser
comparison. If (a) is muted, record the member's `getChatMember` answer: it did not read as restricted below the defaults.

## Follow-ups: decisions for the owner

These are not checks. They are the UI contract's open confirmations (13-UI-SPEC "Assumptions and open questions"), each with
the default the code uses now. None blocks the phase.

- **UI-SPEC A1: the rule screen as the control shape.** A rule cell opens a rule screen with `[Off | Warn]` and
  `[Kick | Ban | Mute]`. The alternative, five buttons per rule on the page, was rejected for size.
- **UI-SPEC A4: one right for the whole page.** Every punishment choice, the card's Yes and both switches need the creator or
  an administrator with `can_restrict_members`. Delete messages could instead need `can_delete_messages`; one right was kept.
- **UI-SPEC A8: a mention versus a plain name.** A notice names the member with a mention link, which notifies them and puts
  "I fixed it" in front of them. The alternative is a plain escaped name that notifies nobody.
- **UI-SPEC A14: no "cannot work here" line until Phase 14.** The page does not say when the bot cannot restrict or delete or
  when the group is a basic group; Phase 14 (MREQ-10) adds that line.
- **UI-SPEC A15: the cap numbers.** Ten punishments a minute per chat, then a ten-minute pause with one pause notice.
- **UI-SPEC A17: no log-channel post.** Requirement punishments are not posted to the group's log channel in this phase.
- **UI-SPEC A18: Check on join is Off by default,** as everything is, and the page shows it Off on a chat with no row.

## Accepted limits

The code and the rulebook state these; none blocks the phase.

- **A basic group is never screened in this phase (D-20).** Mute and kick are supergroup methods, the watcher's filter is
  supergroups only and the join entry sends the plain welcome for any other chat. Phase 14's "cannot work here" line will say
  so on the page.
- **MREQ-01 is five of the eight rules.** Phase 13 delivers last name, username, Russian, Arabic and Chinese. The profile
  photo, channel subscription and AI spam-name rules arrive through MREQ-11, MREQ-12 and MREQ-14 of Phases 14 and 15.
- **A Mute never replaces a restriction someone else placed (CR-01, fixed in the review pass).** `memberReqLiveEligible`
  still lets a restricted member who is still in the group through, but when the strictest broken rule is a Mute and the
  live record is `restricted` with no marker of this episode, `memberReqMuteOverRestriction` reads the group's defaults (one
  paced `getChat`) and refuses the mute when the member lacks a right the group grants: no mute, no hold, no notice, no
  delete, one Info line with IDs. `restrictChatMember` replaces a whole restriction, so a mute there would have overwritten
  an administrator's `/mute` or partial media restriction, and "I fixed it" would then have freed the member of it. The
  member's restriction stays. A stale hold whose own marker is live is still adopted. Warn, Kick and Ban still act on a
  restricted member (a warn leaves the restriction alone, the other two remove the member). So a partially restricted member
  who breaks a Mute rule is not muted, and the owner should use Warn, Kick or Ban in groups where administrators restrict
  members by hand. A member restricted only to the group's own defaults (what the captcha, `/unmute` and the requirement
  lifts leave behind) lacks nothing and is muted as before; that comparison rests on Telegram reading such a member back
  with the group's rights, which check 14 confirms live. A `getChat` that fails refuses the mute (nothing is replaced on a guess).
- **An unknown-status violator's message is kept during a pause.** When the cached admin list cannot say whether the sender
  is an administrator and a pause forbids the live lookup, the message is not deleted.
- **Two Telegram refusals in a row leave a mute with no button.** When the notice could not be delivered and the lift of the
  mute was refused too, the mute stays and the hold stays punished (one Warn in the log); an admin's `/unmute` or the member's
  rejoin ends it.
- **A crash between the rejoin reset's delete and its lift can strand a mute with no hold.** An admin's `/unmute` ends it.
- **The SQLite "database is locked" artifact.** On the SQLite of the test suite a concurrent writer makes the warn module's
  read-then-write transaction fail where PostgreSQL waits, so the warn variant of the three-replica test judges its first
  message alone. It is a test-environment artifact, not behaviour of the bot.
- **A Latin ë evades the Russian rule,** which looks for the letters ы, ъ, э and ё. The rules go by letters and can also catch
  related languages (Belarusian, Bulgarian); the page says so.
- **Unicode 15 tables.** The Arabic and Han script tests use the Unicode version of the Go standard library; a newer block is
  not seen until the toolchain is upgraded.
- **The goodbye after a requirement kick or ban is kept,** as after `/kick`, the captcha kick and the warn limit.
- **The runner sheds under overload.** At most 32 enforcement jobs run at once; a full runner drops a job with one log line
  and the violator's next message is checked again, so a message can stay undeleted until then.
- **The rolling-deploy caveats of research Pitfall 12.** An old replica has no watcher (fewer punishments, never wrong), no
  `mreq|` handler (a press there gets no answer; the member taps again), no page `m`, no captcha guard (it can unmute a held
  member on the rare race) and no join reset (a muted rejoiner on it stays muted until the button is pressed on a new
  replica). The migration runs before any replica serves.

None of the checks or limits above blocks the phase.
