# Requirements: Xenobot-72, milestone v1.2 Custom Messages and Tuning

**Defined:** 2026-11-18
**Core Value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.

v1.0 and v1.1 requirements are archived in `milestones/v1.0-REQUIREMENTS.md` and `milestones/v1.1-REQUIREMENTS.md`.

Scope rule for the whole milestone (owner decision, 2026-11-18): every message and every tuning value is set group by group, in that group's own settings in the bot's private chat. Nothing is set at Staff Group level and nothing is inherited between groups.

## v1.2 Requirements

### Messages engine and page

- [ ] **MSG-01**: For each event and destination (group or PM) a group can store one custom message: text, at most one media item and URL buttons. A group with nothing stored for an event sends exactly what it sends today, in the group's language.
- [ ] **MSG-02**: A custom text supports the Welcome placeholders (`{first}`, `{mention}` and the rest) plus `{reason}`, `{admin}`, `{duration}`, `{until}`, `{action}`, `{count}`, `{limit}` and `{window}`. A placeholder that has no value for an event is replaced by nothing or by a neutral word, never shown raw. Every value that comes from a user is escaped.
- [ ] **MSG-03**: `{admin}` shows "an admin" (translated) for an anonymous admin and for an automatic action, and no placeholder ever names the Staff Group.
- [ ] **MSG-04**: When Telegram refuses a custom message (too long, bad file, bad markup), the bot sends the default text instead, and the punishment or lockdown itself is never delayed, undone or skipped because of the message.
- [ ] **MSG-05**: A private-only Messages page in `/settings` lists the events in families (manual, automatic, lockdown, requirements) with each event's state. In a group menu the page shows only the "open in private" note.
- [ ] **MSG-06**: Each event opens the v1.1 message editor: Set and Remove for text, media and buttons by sending messages in private, with the editor's own limits and refusals (4096 characters, 1024 as a caption, http or https buttons only), Preview of each part and a Full preview through the real send path.
- [ ] **MSG-07**: Each event has a Silent switch for the group message. Silent means the bot acts and posts nothing in the group for that event; the default is not silent.
- [ ] **MSG-08**: Viewing the Messages page needs the live admin check and the owner gate like every settings page; every change needs `can_change_info`, judged live on every press and every typed message.
- [ ] **MSG-09**: The existing "Bot messages" deletion timers apply to custom messages exactly as to the default ones.
- [ ] **MSG-10**: Custom messages are part of `/export` and `/import`, with the same link and length checks the editor applies; an old export without them imports as before.
- [ ] **MSG-11**: Every existing reply and notice is byte-identical for a group with no custom message, proved by characterization tests recorded before the send sites are changed.

### Events

- [ ] **EVT-01**: Manual punishments have their own messages: ban, timed ban, kick, mute, timed mute, unban and unmute. The silent and delete-first command variants keep their current behaviour.
- [ ] **EVT-02**: Warn and "warn limit reached" have their own messages; the warn message keeps its Remove warn button.
- [ ] **EVT-03**: The antiflood action, the blacklist action and the forward blocklist action each have their own message.
- [ ] **EVT-04**: A failed captcha has its own message, for the timeout and for a failed web check alike.
- [ ] **EVT-05**: The AI spam filter can post a notice when it deletes a message. It is off by default, and when on it posts at most one notice per person per few minutes and a capped number per group per minute, so a spam wave never floods the chat.
- [ ] **EVT-06**: A lockdown started by hand has its own message, and the technical notes (approved users are muted too, the bot cannot delete or manage requests here) are still appended after it.
- [ ] **EVT-07**: An automatic lockdown has its own message per trigger (join surge, flood, AI burst) with `{count}`, `{window}` and the trigger's other numbers, and the same technical notes appended.
- [ ] **EVT-08**: Lifting a lockdown has its own message, whoever lifted it (command, alert button or settings menu).
- [ ] **EVT-09**: Member requirement notices (warn, kick, ban, mute) take a custom text, and always keep the list of broken rules and, for a mute, the "I fixed it" button.
- [ ] **EVT-10**: Staff Group actions, lockdown alerts, raid-scale removals, log channel posts and captcha challenges are not events: their texts and behaviour are unchanged.

### PM copies

- [ ] **PMSG-01**: Each punishment event can also send a message to the punished person in private, with its own text, media and buttons and its own on/off switch, off by default.
- [ ] **PMSG-02**: A PM copy is best effort: when the person never started the bot or blocked it, nothing else changes, the punishment stands and the group message is sent as usual.
- [ ] **PMSG-03**: PM copies are sent on their own fleet-wide pacing budget and never before or instead of the punishment, so a burst of PMs never delays a ban, a mute or a notice.
- [ ] **PMSG-04**: When the PM copy cannot be delivered and the owner turned the option on, the group message carries a Details button that opens the bot's private chat and shows that person their message there. Only the punished person gets the content.
- [ ] **PMSG-05**: No PM copy is ever sent for a lockdown's joiner bans, for "Ban N recent joiners" or for a Staff Group action, and a per-group cap bounds PM copies per minute for automatic events.
- [ ] **PMSG-06**: A PM copy never names the Staff Group or another group, and names the acting admin only through `{admin}` under MSG-03.
- [ ] **PMSG-07**: With the log option on, a PM copy that could not be delivered leaves one line in the group's log channel.

### Tuning

- [ ] **TUNE-01**: Warn limit, blacklist and antiflood can apply a timed ban or a timed mute with a duration the owner chooses, set from their settings pages and by their commands alike.
- [ ] **TUNE-02**: An escalation ladder: the owner defines what the first, second and later automatic offences of one member get (for example mute 1 hour, mute 24 hours, ban), per group, off by default.
- [ ] **TUNE-03**: Warns can expire: a warn older than N days no longer counts toward the limit. Off by default.
- [ ] **TUNE-04**: Manual commands have per-group defaults: a default duration for `/mute` and `/ban` given without one, and a "reason required" switch that refuses a punishment with no reason.
- [ ] **TUNE-05**: Reason shortcuts: the owner saves short words that expand to a full reason text when used as the reason of a manual punishment.
- [ ] **TUNE-06**: An appeal button: the owner sets one link or contact per group, and it is added to ban messages and their PM copies when set.
- [ ] **TUNE-07**: An Advanced page exposes per-group thresholds that are code constants today, each inside safe bounds and with a reset to default: the raid windows, the new-member share and age, the AI spam bars, the member requirement cap and pause, the lockdown reminder interval and a captcha timeout above 10 minutes.
- [ ] **TUNE-08**: A plain-tone switch per group replaces the bot's themed default texts with neutral ones for the events of this milestone, in all 8 languages.
- [ ] **TUNE-09**: Copy settings: from a group's settings the owner copies the custom messages and tuning values of another group they own, after a confirmation that says what will be overwritten. The person must be allowed to change settings in both groups, checked live.

## Future Requirements

- **MSG-F1**: A custom message per language.
- **EVT-F1**: Custom texts for Staff Group cards and lockdown alerts.
- **TUNE-F1**: Different escalation ladders per kind of offence.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Staff Group-level defaults or inheritance of messages and tuning | Owner decision: everything is set group by group in the bot's private chat |
| Automatic lockdown lift after N hours | Only a confirmed lift by an admin ends a lockdown |
| PM copies for raid-scale removals | Would hit Telegram's limits and tell attackers what happened |
| Posting a custom message into linked groups for a Staff Group action | Staff actions post nothing in a linked group's chat, by design |
| Editing messages from a menu opened in the group | The editor works in the bot's private chat only, as in v1.1 |

## Traceability

Filled in by the roadmap.

| Requirement | Phase | Status |
|-------------|-------|--------|

**Coverage:**
- v1.2 requirements: 37 total
- Mapped to phases: 0
- Unmapped: 37

---
*Requirements defined: 2026-11-18*
*Last updated: 2026-11-18 after initial definition*
