# Phase 14: Profile photo and channel rules, real-device and live checklist

This file collects the checks that need a live Telegram client or a real group. It is for you, the owner. There is no tool
to run: you work through the numbered checks below on a test supergroup and a test channel and record each answer here.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 13 ("Plan and build now,
check later"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. Nothing here changes whether the code is safe: for every outcome of every check below, a lookup that
cannot be answered, or that is answered in a way the code does not trust, punishes nobody (D-19, D-22). The checks change
wording and your confidence, not safety.

## Before you enable Mute, Kick or Ban for the photo or channel rule in a real group

Enforcement is ON in code. Since plan 13-13 the kill switch `memberReqEnforcementLive` is true by default, so the watcher,
the join check, the "I fixed it" button and the captcha guard run in every group, and Phase 14 added the photo and channel
rules to all of them. A group still screens nobody until its owner sets a rule, because **every rule defaults to Off**
(a migration adds the two new columns with the value 0, so every existing group reads both rules Off). Nothing changes
anywhere until someone opens Requirements and picks a punishment.

**Both new rules rest on live Telegram behaviour that no fake in the test suite can prove**: what the Bot API answers for a
member who hides their profile photo, what it answers a bot that is not the channel's administrator, and whether a bot that
is only a plain member of a group is told which channel the group is linked to. The tests prove the server logic against
a hand-written Telegram; they cannot prove Telegram. Before you set a punishing value in a real group:

- **Warn and Off need no spike.** Warn goes through the warn module and counts toward the group's warn limit; Off is the
  default. You can use Warn on both rules at once.
- **Photo rule with Mute, Kick or Ban:** run checks **1** (S1, a hidden photo) and **4** (the public-photo fallback). A member
  who hides their photo from the bot can fix it only in their own privacy settings, so the page recommends Warn or Mute for
  this rule, the group notice ends with the remedy ("show it to everyone, or ask an admin to approve you") and an admin can
  approve the member. If check 1 shows an error rather than zero photos, a hidden photo is simply not checked and the page's
  sentence "counts as missing" must be reworded (see check 1).
- **Channel rule with Mute, Kick or Ban:** run checks **2** (S2, what a non-administrator bot is told), **3** (S3, whether
  the group's linked channel reaches the bot) and **5** (the member statuses). The rule works only in a **supergroup**
  linked to a channel, and only while the bot is an **administrator of that channel**.
- **Before Mute on either rule:** also run the Phase 13 checks A2 to A5 and A8 of `13-SPIKE.md`. The "I fixed it" button and
  the mute marker rest on them exactly as for the name rules.

**A basic group is not screened.** Mute and kick are supergroup methods, so the watcher and the join check act on
supergroups only. The Requirements page says so with its basic-group line while any rule is not Off, and the channel rule's
screen offers Off alone in a basic group (it cannot be linked to a channel). This is a stated limit, not a silent one.

## What the automatic tests prove, and what they cannot

The test suite fakes Telegram with a hand-written bot client and runs on SQLite and miniredis (PostgreSQL in CI). It proves
the server logic of the phase: that a rule judges only what it is meant to (a photo total of zero, a subscription status),
that a lookup that cannot be answered, was shed, timed out or was cut off by a shutdown punishes nobody and leaves no hold,
that one punishment happens across replicas, that lookups share one rate budget and never delay a notice, who may press
what, that the "I fixed it" button re-checks every enabled rule with fresh lookups, and that every screen and answer fits
3800 UTF-16 units and 64-byte buttons in 8 languages with no channel title, member name, count or Staff Group in any of
them. `TestMemberReqLookupEndToEnd` drives the whole slice for both rules through the real dispatcher. It cannot prove how
Telegram itself answers (the three spikes below), how a screen renders on a phone, how the lookup pacer behaves against
Telegram's real rate limits across two processes, or whether a native speaker finds a translation natural.

## Before you start

1. **A test supergroup** with the bot as an administrator (Restrict members and Delete messages rights), and a second test
   group that is a **basic group** (for the basic-group line).
2. **A test channel** linked to the supergroup as its discussion group. For some checks you also need a second channel that
   is not linked, and the ability to make the bot an administrator of the linked channel, remove it, and add it as a plain
   subscriber (a bot can only be an administrator of a channel, so "plain member" means removed).
3. **Accounts besides the bot**: the creator of the supergroup (the owner), one administrator, and several plain test
   accounts: one with a profile photo shown to everyone, one with no photo at all, one that **hides its photo** from
   non-contacts (Settings, Privacy, Profile Photo, My contacts), one that hides its main photo but has a public fallback
   photo, one subscribed to the test channel, one that left the channel, one banned from the channel, and a channel
   administrator.
4. **Four kinds of client**: Android, iOS, Telegram Desktop and Telegram Web.
5. **Phones set to Hindi, Russian and Ukrainian** (or test accounts with those languages) for the truncation and grammar
   checks.
6. **The bot's log open.** It logs refusals and failed lookups by chat, channel and user IDs and a Telegram error by its
   code only; it never logs a name, a file ID, a channel title or a notice text.
7. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a
   screenshot.
8. **Restore anything you change** in a borrowed group (the rules, the switches, the bot's rights in the channel).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it
passes or when you have recorded what you changed.

### 1. S1: what a member who hides their photo looks like to the bot

- [ ] done

Steps: from the Bot API (a scratch script or `curl` with the bot's own token, kept out of this file) call
`getUserProfilePhotos(user_id, limit=1)` for four accounts: one with a shown photo, one with no photo, one that hides its
photo from non-contacts, and (check 4) one with a public fallback. Record `total_count`, whether `photos` is empty and
whether the call is an error, with its code and description. Then, in the test supergroup, set Profile photo to Warn, and
have the hiding account send a message.

Pass looks like: the hiding account returns `total_count` 0 with an empty list (not an error), exactly as an account with no
photo does, and the bot warns it. This is what "a hidden photo counts as missing" in `settings_mreq_desc_photo` says.

If not: if the hiding account returns an **error**, the rule can never punish a hidden-photo member (an error is unknown, so
safe). Reword in place, adding no key, `settings_mreq_desc_photo` (the sentence "A photo hidden by privacy settings counts
as missing"), `mreq_remedy_photo` and `settings_mreq_hint_photo` in all 8 locales so they say a hidden photo is not
checked. If it returns a **non-empty list with total 0**, or `null`, the code already handles both (a breach needs a total
of zero and an empty list; null is unknown). Record the exact answer.

### 2. S2: what the bot is told about a channel it does not administer

- [ ] done

Steps: with the bot **not** an administrator of the linked channel (remove it, or use a channel it was never added to),
call `getChatMember(channel_id, bot_id)` and `getChatMember(channel_id, subscriber_id)` and `getChatMember(channel_id,
non_subscriber_id)`. Repeat just after removing the bot from a channel it did administer. Record, for each call, the HTTP
status, the `error_code` and the exact `description`, or the `status` field if an answer comes back. In the test supergroup
set Channel to Warn and have the non-subscriber send a message; open the Channel rule screen and the Requirements page.

Pass looks like: every call for a non-administrator bot is an **error** (expected 400 or 403, "chat not found", "bot is not a
member of the channel chat" or "CHAT_ADMIN_REQUIRED"), never a status. The page shows "I am not an administrator of the
linked channel, so nobody is checked" and the non-subscriber is **not** warned. Nobody is punished while the bot cannot see
the channel.

If not: if a **status** comes back to a non-administrator bot (a plain `member`, or `left` about everyone), the code is still
safe: the breach gate (D-22) asks for the bot's own status in the channel and counts a candidate as a breach only while the
bot is a definite administrator, so a non-administrator bot's `left` answers are inert (the page shows the same
not-administrator line). Record what came back. Also record whether the codes are always 400 or 403: the classifier
`cache.IsBotGoneError` matches **any** 400 or 403 (R3, and a 403 for another reason would read as "not an administrator",
which is inert, never a punishment); if the real answers are narrower, the match can be narrowed to those descriptions.

### 3. S3: does the group's linked channel reach a bot that is only a member of the group

- [ ] done

Steps: put the bot in a supergroup that is linked to a channel as a **plain member** (not an administrator), then call
`getChat(group_id)` and read `linked_chat_id`. Repeat with the bot as an administrator of the group.

Pass looks like: `linked_chat_id` is returned in both cases, as the code assumes.

If not: if `linked_chat_id` appears only for administrators, a linked group whose bot is not an administrator reads as
"no linked channel found", `lc` caches 0 for 10 minutes and the channel rule is inert there, which cannot punish there
anyway (a bot that is not an administrator of the group cannot restrict, kick or ban in it). The page says "I found no
linked channel for this group", the text chosen to say what the bot found and not that none exists. In that case reword
`settings_mreq_here_nolink` in place to ask for administrator rights in the group. No key is added and safety is unchanged.

### 4. The public-photo fallback

- [ ] done

Steps: take an account whose main photo is hidden from non-contacts but which set a public fallback photo (Settings, Edit
Profile, Set Public Photo). Call `getUserProfilePhotos(user_id, 1)` for it.

Pass looks like: `total_count` 1 (the bot sees the fallback), so the account passes the photo rule. Either answer is fine
for safety; this only decides what the copy may promise.

If not: if the fallback is not shown, the account counts as having no visible photo, which the page already says
("a photo that I can see"). Record it; no change is needed.

### 5. The member statuses of a channel

- [ ] done

Steps: with the bot an administrator of the test channel, call `getChatMember(channel_id, user_id)` for a subscriber, an
account that left, an account banned from the channel, a channel administrator and the channel creator. Then have each of
them post in the supergroup with Channel set to Warn.

Pass looks like: a subscriber reads `member`, one who left reads `left`, one banned reads `kicked`, administrators read
`administrator` or `creator`; subscribers and administrators (a channel administrator who posts as themselves counts as
subscribed) are not warned, and the accounts that left or were banned are warned once.

If not: record the status Telegram gave. An unrecognised status is unknown and never punishes. A `restricted` answer is a
subscriber only with `is_member` true.

### 6. The two new cells and the Off-only screen in hi, ru and uk

- [ ] done

Steps: on a phone set to Hindi, then Russian, then Ukrainian, open `/settings`, the Requirements page and the **Profile
photo** and **Channel** cells, then the Channel rule screen in a group with no linked channel (the Off-only screen) and in
the linked group.

Pass looks like: both cells (an icon and a name of at most 18 code points) fit two to a row without truncation, the photo
cell sits beside the Chinese cell's row and the Channel cell is alone on the last row, the status block lines read as
sentences and the Off-only screen shows `[Off]` alone.

If not: shorten the `settings_btn_mreq_photo` or `settings_btn_mreq_channel` translation in that language; the grid limit
is 20 code points (`TestSettingsLabelLengths`). Record the language and what was cut.

### 7. Two live replicas over one PostgreSQL and one Redis

- [ ] done

Steps: run two bot processes over the same database and Redis (a staging stack). In a test supergroup set Profile photo to
Mute. Have one photo-less account send ten messages in quick succession so that the replicas both receive them. Watch the
lookup rate in the logs.

Pass looks like: one punishment (one mute, one notice), at most one lookup per message across both processes, and the
lookups together are paced at about 10 a second (`fuku:memberreq:lkp:pace:*` is shared, Interval 100 ms), never above it.

If not: if Telegram answers 429, the shared block absorbs it (`retry_after` kept in full); if it still hits the limit raise
the interval of `memberReqLookupPacerOptions` (research Open Question 5: 100 ms against 200 ms) and record the first 429.

### 8. A raid with the lookup pacer shedding

- [ ] done

Steps: in a test supergroup with Profile photo on Mute and Channel on Warn, have 30 photo-less accounts join and post within
a few seconds (or run a scratch burst). While it runs, open the Requirements page as an administrator and press a rule.

Pass looks like: the lookups beyond the pacer's five-second horizon are shed and count as unknown, so nobody extra is
punished; the page draws the unknown mark for the channel status at that moment and a refresh draws the real one; no mute
notice is delayed, because notices and punishments use their own pacer (`fuku:memberreq:pace:*`) and never the lookup one.

If not: if a notice is late or the page errors, record it. The two budgets are separate on purpose (`TestMemberReqPacersAreIndependent`).

### 9. A restart during a burst of lookups

- [ ] done

Steps: while the burst of check 8 is running, send SIGTERM to the bot (a container stop) and restart it. Read the holds
table (`chat_requirement_holds`) and the group.

Pass looks like: the process exits inside its grace period (the lookup flights end with the member requirements drain, 3 s
in all, no new drain), no hold is left by a cut-off lookup, nobody is punished because of it, and the next message of a
photo-less account starts the check again and punishes it once.

If not: if a hold row or a half-sent notice is left, record the group and member IDs; a pending hold older than two minutes
is reclaimed by the next message, so this should only ever delay a punishment.

### 10. A channel linked and unlinked while a rule is set

- [ ] done

Steps: set Channel to Warn in a linked group, then unlink the channel in the group's Telegram settings and open the
Requirements page at once, again after a minute, and again after eleven minutes. Link it again and repeat. Open the Channel
rule screen each time.

Pass looks like: the page may still say the channel is linked for up to 60 seconds (a positive `lc` is cached for 60 seconds, a none for 10 minutes), the
Channel rule screen read is always fresh and shows the Off-only form once the link is gone, a stored Warn stays visible and
inert, and no member is warned while there is no link.

If not: if the status line is stale for longer than a minute after a relink (10 minutes after a link appears where none was cached), record the replica count; the `lc` key expires on its own.

### 11. A native-speaker read of the new and reworded keys

- [ ] done

The 16 new keys and 7 reworded ones were translated without a native read. Please read them in the language of the group
that will see them. Highest risk first:

- **hi (Hindi), every key below, above all** `mreq_rule_photo` ("कोई दिखती प्रोफ़ाइल फ़ोटो नहीं", a shortened noun phrase
  that was cut to the 32-code-point ceiling), `mreq_rule_channel` ("चैनल सब्सक्रिप्शन नहीं"), `mreq_notice_kick` and
  `mreq_notice_mute` ("जो सूचीबद्ध है ..."), `settings_mreq_desc_photo`, `settings_mreq_hint_mute`.
- **ru and uk**: the phrases that fill the `{rules}` slot of a sentence (`mreq_rule_photo`, `mreq_rule_channel`), the grammar
  of the count in `mreq_fixed_still_broken_many` ("нарушено правил — {n}" and "порушено правил — {n}"), and the kick
  notice ("когда указанное будет исправлено").
- **id**: `mreq_rule_channel` ("belum berlangganan kanal", Telegram's Indonesian word for a channel) and
  `settings_mreq_hint_photo` (it keeps the English action names Kick and Ban, as the page's existing words do).
- **fr**: `mreq_notice_kick` ("une fois ce qui est indiqué corrigé") and the `settings_mreq_here_*` lines, which have the
  space before the colon.
- **es and pt**: `mreq_rule_channel` ("sin suscripción al canal", "sem inscrição no canal") and the neutral "lo indicado" /
  "o que foi listado" wording of the notices.

The 16 new keys: `mreq_rule_photo`, `mreq_rule_channel`, `mreq_fixed_still_broken_many`, `mreq_remedy_photo`,
`settings_mreq_rule_photo`, `settings_mreq_rule_channel`, `settings_mreq_hint_photo`, `settings_mreq_hint_lookup`,
`settings_btn_mreq_photo`, `settings_btn_mreq_channel`, `settings_mreq_desc_photo`, `settings_mreq_desc_channel`,
`settings_mreq_here_basic`, `settings_mreq_here_nolink`, `settings_mreq_here_notadmin`, `settings_mreq_here_unknown`. The 7
reworded keys (they no longer say "profile", so a channel breach is never called a profile problem, amendment M12):
`mreq_notice_warn`, `mreq_notice_kick`, `mreq_notice_mute`, `mreq_notice_fixed`, `mreq_notice_fixed_lockdown`,
`mreq_fixed_still_broken`, `settings_mreq_hint_mute`.

Pass looks like: each reads as natural, correct language, the `{rules}` slot phrases work after "you break this group's
rules:" in the language's grammar, and the photo remedy sentence is clear.

If not: fix the wording in `locales/<lang>.yml` in place. The checks that guard it are `TestMemberReqLocaleKeys`,
`TestMemberReqNoProfileInReworkedKeys`, `TestMemberReqScreensFitEveryLocale` (a notice is at most 520 code points) and the
settings limits table.

### 12. CI: lint, the full test run and the PostgreSQL integrity target

- [ ] done

Steps: run `make lint` with a golangci-lint built with Go 1.26, then `make test` and `make test-postgres-integrity` in CI
(or locally against a PostgreSQL), and `node --test scripts/captcha-page/*.test.mjs` is unchanged by this phase.

Pass looks like: all three pass. `make test-postgres-integrity` runs `TestChatRequirementsSettings` (seven columns, the
CHECKs refusing 5 and -1) and `TestMemberRequirementLookupMigrationDefaults` (the migration's statements run twice, a row
with only `chat_id` reads both new columns 0, both named constraints exist), which skip locally.

If not: the sandbox this phase was built in could not run `make lint` (its golangci-lint is built with Go 1.25) or
PostgreSQL-only tests through CI; read the failure and fix it, never loosen a test.

## Follow-ups: decisions for the owner

These are choices the planner made as recommended defaults and the owner still confirms. None blocks the phase.

- **P14-A3: no positive "I am an administrator of the linked channel" line.** The channel screen shows a status line only
  when the rule cannot work; when all is well it shows nothing. Recommended default: keep it (the absence of a problem line
  is the signal). Alternative: one more key (`settings_mreq_here_ok`, a green line) so you see confirmation.
- **P14-A4: a punishing channel press is refused when the link cannot be read.** Fail closed: the alert says "I couldn't check
  right now", nothing is written and nothing is edited. Recommended default: keep it, so the rule is offered only in a group
  that can be linked. Alternative: allow the write (it is inert without a link).
- **P14-A7 and M10: the notice cap rises from 400 to 520 code points.** Computed at the translations' ceilings, the worst
  mute notice is 503, English as written is at most 328; the cap binds only a degenerate translation. Recommended default:
  520. Alternative: keep 400 and drop the photo remedy tail from the notice (it then lives only in the phrase and the page).
- **M11: the "I fixed it" press reads approvals at step 4b.** An approved member is freed at the tap, once the live marker
  proves the mute is the requirements', with no settings read and no lookup, so "ask an admin to approve you" is true for a
  mute. The approvals are read through the cached list (a lag of up to `CACHE_LOCAL_TTL`, 10 s, on another replica is a
  retry). Recommended default: keep it. Alternative: a fresh approvals read.
- **M12 and P14-A8: seven keys were re-translated** in all 8 languages so no notice calls a channel breach a profile
  problem (49 texts beyond the 16 new keys). Recommended default: keep, and read them (check 11).
- **M13 and P14-A5, P14-A9: one classifier** reads the bot's own status in the channel for the page, the screens, the held
  release and "I fixed it": a definite non-administrator status or a 400 or 403 is "not an administrator" (inert), and
  everything else is unknown. Recommended default: keep. If check 2 shows other codes, adjust `cache.IsBotGoneError` or the
  classifier's mapping; a mismatch can only make the rule unknown, never punish.
- **Research Open Question 5: the lookup pacer's interval.** 100 ms (10 a second) against 200 ms. Recommended default: 100 ms;
  raise it if check 7 shows 429s.
- **MREQ-10 and MREQ-01 complete in Phase 15.** MREQ-10's text ends with "AI not configured", which is Phase 15's
  `settings_mreq_here_ai` line, so MREQ-10 stays Pending until then. MREQ-01 is **seven of the eight rules** after this phase
  (Phase 13: last name, username, Russian, Arabic, Chinese; Phase 14: profile photo and channel); the AI spam-name rule is
  MREQ-14 in Phase 15, and MREQ-01 stays Pending until it exists.

## Accepted limits

The code and the rulebook state these; none blocks the phase.

- **A channel the bot cannot read costs one channel-level fact per minute (bounded since the Phase 14 review fix, WR-01).** A
  verdict about a member that is inert or unknown is still never cached (D-24; only a pass is), but the bot's own definite
  not-administrator standing in the channel is cached for the channel only, as `fuku:memberreq:cna:<channel>` (60 s, fixed
  marker, never a user verdict). While it stands the channel rule is inert at the top of the executor with no Telegram call and
  the watcher starts no job, so after the first message such a group costs one `getChatMember` of the bot per channel per
  minute. A standing that cannot be established (a 429, a 5xx, a timeout, a shed) writes nothing, so the per-message job (one
  runner job and the paced lookups, capped at 32 jobs in flight and bounded by the lookup pacer) stays until it can. Nobody is
  punished either way. The same per-message cost applies to a member who stays without a photo or a subscription, and to every
  member while Redis is down. The shed log line is Debug.
- **A member can be stuck behind "I fixed it" only while a channel lookup for them stays unanswerable AND the bot is neither a
  definite non-administrator nor bot-gone.** Then the classifier keeps the rule unknown and the press answers "I couldn't
  check right now". An administrator's `/unmute` is the fallback and the 330-day marker mute is the worst case.
- **The photo and subscription positives are cached for 30 minutes** at the watcher, so a member who removes their photo or
  unsubscribes is not checked again for up to 30 minutes (the "I fixed it" press is always fresh).
- **A partial episode is fixed where it did harm (Phase 14 review fix, WR-02):** an unknown lookup never blocks an established
  name breach when the rule it could not answer is equal or weaker; when that rule is stricter (a Ban channel rule over a Warn
  last-name rule, say) nothing is claimed and the member is judged again at their next message, so a stricter rule is never
  dropped behind a weaker punishment and its hold. A stricter rule that stays unanswerable holds the weaker punishment back
  for as long as it does (nobody is punished on a guess). The member is punished for the names, with a hold and notice that
  name only the rules established, when the unknown rule is equal or weaker.
- **A stale link is fixed (Phase 14 review fix, WR-05):** a positive `lc` is cached for 60 s (none stays 10 minutes), and a
  not-subscribed candidate reads the link fresh once before the gate, so a punishment never rests on a link older than one
  request. The page's own linked-channel read can still be up to 60 s old (a link) or 10 minutes (none); opening the Channel
  rule screen reads fresh.
- **`cache.IsBotGoneError` matches any 400 or 403** (R3), so a 403 for another reason reads as "not an administrator". That is
  inert and never a punishment.
- **The log of a non-gotgbot client's transport errors is not scrubbed.** `TestMemberReqLookupLogsCarryIDsOnly` relies on
  gotgbot's own scrubbing of the bot credential from a transport error's URL; a different HTTP client would need its own.
- **A basic group is not screened** (mute and kick are supergroup methods); the page says so with its basic-group line.
- **Redis down costs a job per message** of a member in a group with a lookup rule on, and never a punishment.
- **The rolling-deploy caveats.** An old replica answers a `t=ph` or `t=ch` press as an invalid request, ignores the two
  columns and does not screen for them, and the shared settings cache may serve an entry without them until it expires or
  the next write; the pass fingerprint changes once per member (one harmless pass-cache miss). The migration runs before any
  replica serves.
- **The Phase 13 limits that still hold:** a Mute never replaces a restriction someone else placed (CR-01); an unknown-status
  violator's message is kept during a pause; two Telegram refusals in a row leave a mute with no button (an admin's
  `/unmute` ends it); a crash between the rejoin reset's delete and its lift can strand a mute with no hold; the SQLite
  "database is locked" artifact in the warn variant of the replica test; a Latin ë evades the Russian rule and the name
  rules can catch related languages; the Unicode 15 tables; the goodbye after a requirement kick or ban is kept; the runner
  sheds under overload (32 jobs).

None of the checks or limits above blocks the phase.
