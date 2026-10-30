# Phase 2: Staff Actions Across Groups - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-10-05
**Phase:** 02-staff-actions-across-groups
**Areas discussed:** Command syntax, Confirm card, Per-group rules, Progress summary

---

## Command syntax

**How should a staff member give a duration for a timed ban or mute in the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Optional duration (Recommended) | `/ban @x 2d spamming`: if the word after the target looks like 30m/2h/1d/1w, it is the duration. Otherwise the ban is permanent. `/tban` and `/tmute` also work, and they require a duration. If a reason happens to start with something like "2d", the confirm card shows the duration it read, so the mistake gets caught before anything happens. | ✓ |
| Only /tban and /tmute | Same as every other group today. `/ban` and `/mute` are always permanent, and timed actions need `/tban @x 2d` or `/tmute`. This is predictable, but staff have to remember two commands. |  |
| You decide | Leave the syntax to the planner, as long as the confirm card shows the duration it read. |  |

**User's choice:** Optional duration (Recommended)

**Elsewhere there are also /sban, /dban, /skick, /dkick, /smute and /dmute. They are silent or delete-the-message variants that act on the current group, and they make no sense in a staff room. What should they do inside a Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Refuse with a hint (Recommended) | Answer with something like "In the Staff Group use /ban, /mute, /kick, /unban or /unmute". They never act on the Staff Group itself, and they never fan out. Only the five commands, plus /tban and /tmute, mean "all linked groups". | ✓ |
| Treat as the plain command | /sban behaves like /ban: confirm card, then fan-out. The "silent" and "delete" parts are ignored. This is forgiving, but staff may expect a behaviour they don't get. |  |
| Ignore silently | No reply. The command does nothing in a Staff Group. |  |

**User's choice:** Refuse with a hint (Recommended)

**Today, replying to a message picks that message's sender as the target. In a Staff Group the sender is almost always a fellow staff member, or the staff member who forwarded the spam. Should replying to a message choose the target in the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| No, explicit target only (Recommended) | A reply alone gets "give an @username or user ID". Accepted targets are a numeric ID, an @username the bot has seen, or a mention picked from Telegram's list for people with no username (it carries the exact ID). Nobody gets banned everywhere by accident because of who sent a message. | ✓ |
| Allow reply to a plain message | Replying to a message that isn't forwarded targets its sender, the same as everywhere else. Replying to a forward is still refused, because that is out of scope. This is useful for removing a member who is in the Staff Group itself. |  |

**User's choice:** No, explicit target only (Recommended)

**Telegram usernames get recycled. The bot may have seen two different people use @foo at different times. When an @username matches more than one person the bot has seen, what should happen?**

| Option | Description | Selected |
|--------|-------------|----------|
| Refuse and list them (Recommended) | Reply with each match's name, ID and when the bot last saw them, and ask for the numeric ID. Nothing is guessed. Matching ignores upper and lower case, as Telegram does. | ✓ |
| Pick the most recent | Use whoever the bot saw with that username most recently. The confirm card shows their name and ID, so the issuer can still cancel. Quicker, but it relies on the issuer reading the card. |  |

**User's choice:** Refuse and list them (Recommended)

**Notes:** The owner picked the recommended option on every question in this area and added nothing in free text.

---

## Confirm card

**When the issuer taps Confirm, where does the progress and summary appear?**

| Option | Description | Selected |
|--------|-------------|----------|
| Card turns into the summary (Recommended) | The same message is edited: the buttons disappear, the target and action stay at the top, and the per-group lines fill in below. Each action is one message in the Staff Group, which keeps the room tidy and keeps the history easy to scroll. | ✓ |
| New summary message | The card is edited to "Confirmed by X" with no buttons, and the summary is posted as a separate reply. That is two messages per action, but the card stays as a record of what was approved. |  |

**User's choice:** Card turns into the summary (Recommended)

**How long should an unconfirmed card stay usable? Once it expires, its buttons answer "expired, run the command again".**

| Option | Description | Selected |
|--------|-------------|----------|
| 5 minutes (Recommended) | Long enough to double-check the target. Short enough that an old card can't be tapped by accident an hour later, after the situation has changed. | ✓ |
| 1 minute | Tight. This suits a fast incident response, but a slow reader may have to run the command again. |  |
| Never expires | The card works until someone taps Confirm or Cancel. That is simple, but a stale card can be confirmed much later. |  |

**User's choice:** 5 minutes (Recommended)

**Should the card check your admin rights in each linked group before you confirm, or only show how many groups are linked?**

| Option | Description | Selected |
|--------|-------------|----------|
| Just the count (Recommended) | "Applies to 6 linked groups". All checks run live after Confirm anyway, and those results are the ones that count. The card appears instantly and needs no Telegram calls. | ✓ |
| Pre-check and preview | "You can act in 4 of 6 groups" with the skipped ones named. This costs about one Telegram call per group before the card shows. It is only a preview, because everything is checked again after Confirm. |  |

**User's choice:** Just the count (Recommended)

**What should Cancel, or an expired card, leave behind in the Staff Group?**

| Option | Description | Selected |
|--------|-------------|----------|
| Edit to "Cancelled" (Recommended) | The card stays with its buttons removed and a line such as "Cancelled by X" or "Expired". The other staff can see that an action was considered and dropped. | ✓ |
| Delete the card | The card disappears, plus the command message if the bot can delete it. A cleaner room, but no trace that someone nearly banned a user everywhere. |  |

**User's choice:** Edit to "Cancelled" (Recommended)

**Notes:** The owner picked the recommended option on every question in this area and added nothing in free text.

---

## Per-group rules

**A staff /ban reaches a linked group the target has never joined, or has already left. What should happen there?**

| Option | Description | Selected |
|--------|-------------|----------|
| Ban anyway (Recommended) | Telegram lets the bot ban someone who isn't in the group, so the target can never join it. This stops a raider moving on to the next group. The line reads "banned (not in group)". | ✓ |
| Skip non-members | Ban only in groups where the target is currently a member, and mark the rest "skipped: not in group". The ban list stays shorter, but the target can still join those groups. |  |

**User's choice:** Ban anyway (Recommended)

**A staff /mute reaches a group where the target isn't a member. Kicking a non-member makes no sense, so /kick will skip them. What should mute do?**

| Option | Description | Selected |
|--------|-------------|----------|
| Skip: not in group (Recommended) | Mute only current members, and mark the others "skipped: not in group". This is predictable, and it avoids anything Telegram might do to the record of someone who isn't in the group. | ✓ |
| Pre-mute if safe | Mute them in advance if research shows Telegram keeps the restriction when they join later and never turns a ban into a mute. If research can't confirm that, fall back to skipping. |  |

**User's choice:** Skip: not in group (Recommended)

**The target is already banned in a group, for example permanently, and a staff member sends `/ban @x 1d`. Sending the new ban as-is would shorten it to one day. What should happen?**

| Option | Description | Selected |
|--------|-------------|----------|
| Never shorten (Recommended) | If they are already banned, keep the existing ban and mark the group "skipped: already banned". Mutes work the same way: an existing mute is never shortened. A staff action only ever makes things stricter, unless it is an unban or unmute. | ✓ |
| New action replaces it | Always apply the duration that was asked for, even if that shortens an existing ban or mute. It does exactly what the command says, but a quick `/ban 1d` can cut short someone's permanent ban. |  |
| Lengthen only | Apply the new ban or mute only when it ends later than the existing one, and skip it otherwise. This needs the existing end date from Telegram for each group. |  |

**User's choice:** Never shorten (Recommended)

**One edge of "never shorten": the target already has a 1-day ban in a group, and a staff member sends a permanent `/ban`. The new ban is stricter. What should happen?**

| Option | Description | Selected |
|--------|-------------|----------|
| Upgrade it (Recommended) | Apply the permanent ban, because it is stricter. A ban or mute is applied only when it ends later than the current one (permanent counts as the latest). Otherwise the group is "skipped: already banned/muted". The bot reads the current end date from Telegram, using the same call it already makes for that group. | ✓ |
| Skip any existing ban | If they are banned at all, leave it alone and skip, even when the existing ban is shorter. Simpler, but the 1-day ban expires and they can come back. |  |

**User's choice:** Upgrade it (Recommended)

**Notes:** The owner picked the recommended option on every question in this area and added nothing in free text.

---

## Progress summary

**How often should the summary message update while groups are being processed?**

| Option | Description | Selected |
|--------|-------------|----------|
| Batched, about every 2-3 s (Recommended) | Collect finished groups and edit at most every few seconds, with a final edit when everything is done. This stays clear of Telegram's limit of about one edit per second per chat, so the final summary never gets rate limited. | ✓ |
| After every group | Edit as soon as each group finishes. It feels the most live with fewer than 10 groups, but a burst of fast groups can hit the edit limit and delay the final result. |  |

**User's choice:** Batched, about every 2-3 s (Recommended)

**How should the per-group lines be laid out?**

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed order, icons flip (Recommended) | Every linked group is listed from the start as "⏳ Group name" and flips to ✅ / ⏭ skipped: reason / ❌ failed: reason as it finishes. Lines never jump around, and a tally line (✅ 4 · ⏭ 1 · ❌ 1) sits under the header. | ✓ |
| Problems first when done | Same while running, but the final edit re-sorts: failed, then skipped, then done. Problems are easier to spot, but the lines move at the end. |  |

**User's choice:** Fixed order, icons flip (Recommended)

**With many linked groups the summary could outgrow Telegram's message size limit. How should it fit?**

| Option | Description | Selected |
|--------|-------------|----------|
| Collapse done lines (Recommended) | Telegram caps a message at 4096 characters. If the full list wouldn't fit, the done groups collapse into one line such as "✅ 37 groups done". Skipped and failed groups are always listed by name with their reason, so no problem is ever hidden. With fewer than about 40 groups every line is shown. | ✓ |
| Spill into extra messages | Keep every group on its own line and continue in a second or third message when the first one fills up. Everything stays visible by name, but one action can produce several messages in the Staff Group. |  |

**User's choice:** Collapse done lines (Recommended)

**When a group fails, what should its line say?**

| Option | Description | Selected |
|--------|-------------|----------|
| Plain reason, short detail (Recommended) | Known cases get a clear translated reason: "bot isn't an admin", "bot lacks ban rights", "rate limited, gave up after retries", "group not found". Anything unexpected reads "failed: Telegram error" followed by Telegram's short error text, so staff can still report it. | ✓ |
| Plain reason only | Unexpected errors read just "failed: Telegram error". Tidier, but staff can't tell two different unexpected errors apart without checking the bot's logs. |  |
| Telegram's raw text | Always show Telegram's own error message, such as "Bad Request: not enough rights to restrict/unrestrict chat member". Precise, but it's English-only and harder to read. |  |

**User's choice:** Plain reason, short detail (Recommended)

**Notes:** The owner picked the recommended option on every question in this area and added nothing in free text.

---

## Claude's Discretion

- How the final summary still reaches the issuer if the card is deleted or can't be edited mid-run.
- Whether two staff members acting on the same target at once need anything beyond the never-shorten and upgrade rules.
- The hint wording for refused variants, bare replies, unknown usernames and ambiguous usernames.
- A reason length cap, and how a long reason is shortened.
- The cross-action rules: ban over mute, mute over ban, unmute on a banned user. Mute and unmute must never lift a ban.
- Durations over 366 days, which Telegram treats as permanent.
- The group order in the summary (suggested: the `/staff` panel order).
- The unban and unmute "nothing to do" defaults (D-14). They were proposed in the per-group rules wrap-up, and the owner accepted them by choosing "Next area".

## Deferred Ideas

None. The discussion stayed within phase scope.
