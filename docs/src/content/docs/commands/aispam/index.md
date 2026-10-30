---
title: Aispam Commands
description: Complete guide to the Aispam module — the per-chat AI spam filter
---
<!-- MANUALLY MAINTAINED: do not regenerate -->

# 🤖 Aispam Commands

The Aispam module is an opt-in, per-chat AI spam filter. When it is on, every
message in the chat is judged by TypeSafe's Jev decision model against the
chat's own title, description and rules, and messages the model is confident
are spam are deleted.

It is the only module that deletes on a judgement rather than a rule, and it is
off in every chat until an admin turns it on.

## Commands

| Command | Description | Disableable |
|---------|-------------|-------------|
| `/aispam` | Show whether the filter is on, and exactly what is sent to the model | ✅ |
| `/aispam on` | Turn the filter on for this chat | ✅ |
| `/aispam off` | Turn the filter off for this chat | ✅ |

`on`/`off` also accept `yes`/`no`. A bare `/aispam` reports status and lists the
data sent to the model.

## What Happens To A Flagged Message

Only one action exists: the message is deleted. No warning, no mute, no
restriction, no reply, and no notice to the sender. Every deletion is mirrored
to `MESSAGE_DUMP` with the probability, category, model version and the message
text, which is the audit trail and the only feedback loop for tuning.

## Thresholds

| The model reads the message as | Minimum probability to delete |
|-------------------------------|------------------------------|
| English | 0.80 |
| Anything else, including too short or mixed to tell | 0.90 |

Jev is English-first, so only a message it reads as English takes the lower bar.
The message's own language decides, not the chat's configured language: an
English chat still receives posts written in other languages, and those are held
to the higher bar. If the language answer is missing or unreadable, the higher
bar applies.

## What Is Skipped

These messages are never sent to the model:

- Anything from an admin or an approved user.
- Members still solving a captcha challenge.
- This bot's own commands: a bare `/command`, or `/command@thisbot`. A slash
  message aimed at another bot is judged like any other text.
- Messages with no text or caption, and messages from bots, channels, anonymous
  admins or linked channels.

## What The Model Receives

For each checked message:

- The chat's title, description, rules and language.
- The sender's profile: days since the bot first saw them, messages in the last
  hour, warn count.
- The message text or caption, the domains it links to, and whether it is a
  forward or a reply.
- The sender's few most recent messages in this chat, and how many messages they
  posted in the last hour.

The sender window is kept in Redis for one hour, keyed per chat and user, and
expires on its own. Nothing else about a message is stored. A message joins the
window even when the check that follows it fails, and losing Redis costs the
window rather than the check: the message is still judged, with no history.

No names, usernames, user IDs, phone numbers, or messages from other chats are
sent. `/aispam` prints this list in the chat.

## AI Spam And Raid Protection

Automatic raid protection has an AI burst trigger (see `/autolockdown`). It is
separate from the opt-in filter above, and the two work together:

- **Deleted messages feed the trigger.** Every message `/aispam` deletes also
  counts as one AI spam verdict for the group's automatic AI spam trigger,
  whoever sent it. A message the filter keeps never counts.
- **The AI never locks a group on its own.** A lockdown starts only when at
  least 5 spam verdicts (the group's threshold, 3 to 500) from at least 3
  different accounts arrive within 120 seconds. One verdict never does. The
  lockdown is the same one `/lockdown` starts, and nothing is deleted, banned or
  restricted by the count itself.
- **Raid protection also checks new members' posts, with or without `/aispam`.**
  For members who joined the group in the last 24 hours, a text with a link, a
  forwarded text, or a text they just repeated is checked with TypeSafe. The check
  sends much less than the list above: the chat's title and language, the
  sender's days since the bot first saw them and a messages-last-hour number
  (always 1 here), and the text with the domains it links to and whether it is a
  forward. It never sends the chat's description or rules, the sender's warn
  count, whether the message is a reply, or any earlier message. When `/aispam` is on and will judge the text itself, the raid check
  sends nothing, so a message is never judged twice. These checks only report a
  verdict: they delete nothing.
- **Images go to Google Gemini.** When the bot owner has set `GEMINI_API_KEY`,
  a new member's suspicious photo, GIF, sticker or video is judged by Google
  Gemini. Only the image itself (for a GIF or video, Telegram's preview picture)
  and a fixed instruction are sent: no caption, name, username, user ID or chat
  information. A verdict counts only at 0.85 or higher, and the checks are
  limited to 20 per member, 150 per group and 500 for all groups per hour.
- **Nothing is sent without the keys.** Without `GEMINI_API_KEY` no image
  leaves the server. Without `TYPESAFE_API_KEY` no text is sent for the raid
  checks. With `ENABLE_AISPAM` off nothing is sent at all, no text and no image. Without Redis the trigger does nothing.
- **Use a paid-tier Gemini key.** On Google's free tier the images you send may
  be used to improve Google's products, so a paid-tier key is recommended before
  the owner turns the image checks on.

An admin can turn the trigger off for a group with `/autolockdown ai off`, or
change its threshold with `/autolockdown ai <N>`. This does not change what
`/aispam` itself deletes.

## Failure Behaviour

The filter fails open: a timeout, rate limit, server error or unparseable
answer produces no verdict and no action. `429` and `5xx` get exactly one retry,
honouring `Retry-After`. After five consecutive failures in a chat, checks for
that chat pause for five minutes and the chat gets one notice; the stored
setting is untouched, so protection resumes on its own.

Checks run on a bounded worker queue. When the queue is full — a raid, or a
provider brownout — checks are shed rather than queued unbounded. A shed check
means "not checked", and antiflood still owns floods.

## Required Permissions

| Command | Permissions |
|---------|-------------|
| `/aispam`, `/aispam on`, `/aispam off` | Admin only — requires `IsUserAdmin` |

Enabling is refused, with a message explaining why, when the bot operator has
not configured an API key or has switched the filter off globally — an admin is
never left believing their chat is protected when it is not.

## Operator Setup

| Variable | Meaning |
|----------|---------|
| `TYPESAFE_API_KEY` | TypeSafe API key used for every check. One credential for the whole bot; chats cannot bring their own. Empty disables the feature. |
| `ENABLE_AISPAM` | Global kill switch for every AI check, `/aispam` and the raid checks, text and images. Default `true`; `/aispam` still stays off in chats that never enabled it. Set to `false`, nothing goes to TypeSafe or Google even with the keys set. |
| `GEMINI_API_KEY` | Google Gemini key for the raid protection's image checks. Empty means no image is ever sent. A paid-tier key is recommended. |

The key is registered with `logredact`, so it never appears in logs.
