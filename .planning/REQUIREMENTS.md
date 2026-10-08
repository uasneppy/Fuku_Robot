# Requirements: Fuku Robot

**Defined:** 2026-10-04
**Core Value:** My trusted staff can protect every one of my communities from one place. We act on a bad actor across all groups at once, and the bot never lets anyone act in a group where they aren't an admin.

## v1 Requirements

Requirements for this milestone. Each maps to one roadmap phase.

### Staff Group setup and links

- [x] **SETUP-01**: A group's owner can designate that Telegram group as a Staff Group. Channels and basic groups that can't be used are refused with a reason.
- [x] **SETUP-02**: Only the owner of a Staff Group can remove its Staff Group status, which unlinks all its groups.
- [x] **SETUP-03**: A user who owns both a group and a Staff Group can link that group to the Staff Group.
- [x] **SETUP-04**: Linking is refused if the user doesn't own both groups (checked live), or if the group is already linked to a Staff Group.
- [x] **SETUP-05**: The owner of both groups can unlink a group.
- [x] **SETUP-06**: A link is removed automatically as soon as the same person no longer owns both groups. Ownership is re-checked before every staff action, and ownership-change updates are also watched.
- [x] **SETUP-07**: Links, and Staff Group status, keep working after a group upgrades to a supergroup and its chat ID changes.
- [x] **SETUP-08**: In the Staff Group, `/staff` opens a panel with the Staff Group help text and every linked group with its status:
  - linked
  - bot is admin
  - bot can restrict members
  - owner still matches
  - currently in lockdown
- [x] **SETUP-09**: The `/staff` panel lists recent staff actions: who, what, target, when, reason, and per-group outcome.

### Staff actions

- [x] **STAFF-01**: Any member of the Staff Group can run `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` there, with optional timed variants for ban and mute, against an `@username` or a numeric user ID.
- [x] **STAFF-02**: A staff member can add an optional reason to any staff action.
- [x] **STAFF-03**: Before anything is applied, the bot shows a confirmation with:
  - the resolved target (name and ID)
  - the action
  - the duration
  - the reason
  - the number of linked groups

  Nothing happens until the issuer taps Confirm. Only the issuer can confirm or cancel.
- [x] **STAFF-04**: A confirmed action is applied to every linked group and never to the Staff Group itself.
- [x] **STAFF-05**: Before acting in each linked group, the bot checks live that the issuer is an admin there with the right to restrict members. It acts where they are and skips the groups where they aren't.
- [x] **STAFF-06**: Posts from anonymous admins in the Staff Group are refused with "post as yourself", because they can't be authorised per group.
- [x] **STAFF-07**: The bot never acts against a target who is an admin or the owner of a given group, or against the bot itself. Such groups are skipped with that reason. Staff Group membership alone does not protect anyone.
- [x] **STAFF-08**: The issuer sees one summary message, updated as groups complete. Each group is marked done, skipped (with reason) or failed (with reason, e.g. the bot lacks rights or is rate limited). No group is silently dropped.
- [x] **STAFF-09**: Each applied action and its reason are posted to the log channel of every group where it was applied.
- [x] **STAFF-10**: Every staff action is recorded with:
  - the issuer
  - the target
  - the action
  - the duration
  - the reason
  - the time
  - the per-group outcomes
- [x] **STAFF-11**: The summary has an "Undo everywhere" button. A Staff Group member who is an admin with restrict rights in a group can reverse the action there. The same per-group checks and the same summary apply.
- [x] **STAFF-12**: The fan-out stays within Telegram's rate limits, waits and retries when Telegram says to, and reports a group as failed instead of dropping it.
- [x] **STAFF-13**: The `/ban`, `/mute`, `/kick`, `/unban` and `/unmute` commands keep their existing per-group behaviour everywhere except inside a Staff Group.

### Lockdown

- [x] **LOCK-01**: While a group is in lockdown, anyone who joins is removed immediately. They can rejoin after the lockdown lifts, and they're listed on the alert.
- [x] **LOCK-02**: While a group is in lockdown, everyone except its admins is muted.
- [x] **LOCK-03**: Approved users stay able to talk during a lockdown. If the Telegram test shows this isn't possible, the owner decides the fallback before lockdown is built.
- [x] **LOCK-04**: A lockdown affects only the group it was triggered in.
- [x] **LOCK-05**: A lockdown never lifts on its own. It survives bot restarts and Redis being cleared.
- [x] **LOCK-06**: Only an admin of the locked group can lift it, either with the alert's "Lift lockdown" button (wherever the alert was posted) or with `/unlockdown` in that group.
- [x] **LOCK-07**: Lifting a lockdown restores the group's permissions exactly as they were before it was locked.
- [x] **LOCK-08**: Unmuting someone during a lockdown, including by passing the captcha or `/unmute`, doesn't leave them muted after the lockdown lifts.
- [x] **LOCK-09**: An admin can start a lockdown manually with `/lockdown` and see the current lockdown status (active or not, since when, why).
- [ ] **LOCK-10**: A group can have at most one active lockdown. Several triggers firing at once create a single lockdown and a single alert.
- [ ] **LOCK-11**: Each lockdown posts one alert to the group's Staff Group (if linked) and to its log channel. The alert shows:
  - the trigger and the counts behind it
  - a sample of the accounts that joined during the burst
  - a "Lift lockdown" button
  - a "Ban N recent joiners" button
- [ ] **LOCK-12**: An admin of the locked group can press "Ban N recent joiners" to ban the accounts that joined during the burst. Admins and approved users are excluded, and the result is reported.
- [ ] **LOCK-13**: The alert shows which invite link the burst used, where Telegram reports it. It has a "Revoke link" button that only admins of that group can use.
- [ ] **LOCK-14**: While a group stays locked, the alert is re-posted periodically as a reminder until someone lifts it.
- [ ] **LOCK-15**: The old `/antiraid` mode is retired. Its commands point to `/lockdown`, the existing auto-threshold setting carries over to join-surge detection, and the temp-ban timings are dropped.

### Raid detection

- [ ] **RAID-01**: A burst of joins within a short window automatically triggers a lockdown.
- [ ] **RAID-02**: A burst of messages, photos, GIFs or stickers, mostly from recently joined members, automatically triggers a lockdown.
- [ ] **RAID-03**: A burst of AI spam verdicts within a short window automatically triggers a lockdown.
- [ ] **RAID-04**: Borderline text is classified with the existing TypeSafe AI check. Borderline photos, GIFs and stickers are classified with Gemini (`gemini-3.5-flash-lite`, owner-provided key).
- [ ] **RAID-05**: Rules, not AI, decide when to lock. AI verdicts only feed the counters in RAID-03.
- [ ] **RAID-06**: Automatic triggers are on by default in every group, with conservative thresholds. An admin can turn each one off or tune its threshold.
- [ ] **RAID-07**: Admins and approved users never count toward detection.

### Captcha

- [ ] **CAPT-01**: An admin can set a group's captcha mode to Turnstile. Math and text modes stay available as fallbacks.
- [ ] **CAPT-02**: In Turnstile mode, a new member is muted on join and gets a challenge message with a button that opens the bot's captcha page as a Telegram Mini App.
- [ ] **CAPT-03**: Only the new member the challenge was issued to can pass it. A challenge can't be reused or passed by anyone else.
- [ ] **CAPT-04**: Passing is decided only on the bot's server: Telegram's signed user data and Cloudflare's Turnstile verification. The page never reports a pass on its own.
- [ ] **CAPT-05**: A member who passes is unmuted. Their pending messages are replayed, the greeting runs, and the challenge is cleaned up, exactly as with the existing captcha.
- [ ] **CAPT-06**: A failed Turnstile check counts as failing the captcha (one solve only). The member is kicked and can rejoin to try again.
- [ ] **CAPT-07**: A member who doesn't pass within the group's time limit is kicked and can rejoin to try again.
- [ ] **CAPT-08**: The captcha page is shown in the user's language, using the bot's 7 languages.

### Settings menu

- [ ] **MENU-01**: `/settings` in a group asks the admin whether to open the menu in the group or in their private chat with the bot.
- [ ] **MENU-02**: A menu opened in the group can only be used by the admin who opened it. A menu opened in private manages the group it was opened from.
- [ ] **MENU-03**: Every button press re-checks live that the presser is still an admin of the group being configured.
- [ ] **MENU-04**: The menu edits one message in place, with Back and Close buttons.
- [ ] **MENU-05**: The menu has a Staff Group page showing the `/staff` panel information. Linking and unlinking stay limited to the owner of both groups.
- [ ] **MENU-06**: The menu has a Lockdown page showing the current state, a Lift button, and on/off switches and thresholds for each automatic trigger.
- [ ] **MENU-07**: The menu has a Captcha page (mode, time limit) and an Antiflood page.
- [ ] **MENU-08**: A setting changed in the menu is identical to the same setting changed by command.

### Platform

- [x] **PLAT-01**: Everything works with several bot replicas running at once. Fan-out pacing, detection counters, captcha challenges and lockdown state are shared, not per process.
- [ ] **PLAT-02**: New secrets (Turnstile secret key, Gemini API key) never appear in logs.
- [x] **PLAT-03**: Every new message and button label exists in all 7 languages.
- [ ] **PLAT-04**: The deployment steps for the captcha page are documented: public HTTPS hostname, BotFather Mini App registration, Turnstile keys.

## v2 Requirements

Deferred. Tracked but not in this roadmap.

### Staff Group

- **STAFF2-01**: Silent mode, deleting the staff command message after acting
- **STAFF2-02**: Re-applying active staff bans to a newly linked group

### Raid

- **RAID2-01**: Alert-only mode for automatic triggers, for tuning thresholds without locking
- **RAID2-02**: Suspicious-account scoring (new account, no photo, no username, look-alike names) to sharpen "Ban recent joiners"
- **RAID2-03**: External reputation lookup on join (e.g. CAS)

### Captcha

- **CAPT2-01**: Captcha metrics (pass rate, solve time, timeouts) on the metrics endpoint
- **CAPT2-02**: Join-request captcha that verifies people before they're admitted

### Settings menu

- **MENU2-01**: Settings changes posted to the group's log channel
- **MENU2-02**: Free-text editing flows (welcome text, rules) inside the menu

## Out of Scope

| Feature | Reason |
|---------|--------|
| Public, multi-tenant bot | Runs for the owner's own communities only |
| A Telegram channel as the Staff Group | Must be a group so staff can talk and run commands |
| Merging with or changing federations | Separate feature; `/fban` stays as it is |
| Choosing a subset of groups for one staff action | Every action hits all linked groups, by design |
| One group linked to several Staff Groups | One Staff Group per group keeps authority unambiguous |
| Linking by group admins or by two different owners | Only a person who owns both groups can link them |
| Protecting Staff Group members from staff actions | Owner's decision: only each group's admins and owner (and the bot) are protected |
| Applying staff actions to the Staff Group itself | It's a control room, not a target |
| Reply-to-forward targeting or report buttons in the Staff Group | `@username` or ID is enough for v1 |
| Deleting the target's messages across groups | Not needed for v1 |
| Staff changing settings or promoting/demoting in linked groups | Authority creep; staff commands stay restrict-type only |
| Auto-lifting lockdowns | An admin decides when it's safe |
| Locking every linked group when one is raided | One attacked group shouldn't silence the rest |
| Sending every message to AI | Too slow and costly; AI only assists rule-based detection |
| Gemini for text | Owner's decision: TypeSafe stays for text, Gemini for images only |
| Google reCAPTCHA | Turnstile was chosen as more private |
| Retrying inside the captcha page | Owner's decision: one solve only |
| A web admin dashboard | Settings stay in Telegram; smaller attack surface |

## Traceability

Which phases cover which requirements. Each v1 requirement maps to exactly one phase.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SETUP-01 | Phase 1 | Complete |
| SETUP-02 | Phase 1 | Complete |
| SETUP-03 | Phase 1 | Complete |
| SETUP-04 | Phase 1 | Complete |
| SETUP-05 | Phase 1 | Complete |
| SETUP-06 | Phase 1 | Complete |
| SETUP-07 | Phase 1 | Complete |
| SETUP-08 | Phase 1 | Complete |
| SETUP-09 | Phase 3 | Complete |
| STAFF-01 | Phase 2 | Complete |
| STAFF-02 | Phase 2 | Complete |
| STAFF-03 | Phase 2 | Complete |
| STAFF-04 | Phase 2 | Complete |
| STAFF-05 | Phase 2 | Complete |
| STAFF-06 | Phase 2 | Complete |
| STAFF-07 | Phase 2 | Complete |
| STAFF-08 | Phase 2 | Complete |
| STAFF-09 | Phase 3 | Complete |
| STAFF-10 | Phase 3 | Complete |
| STAFF-11 | Phase 3 | Complete |
| STAFF-12 | Phase 2 | Complete |
| STAFF-13 | Phase 2 | Complete |
| LOCK-01 | Phase 4 | Complete |
| LOCK-02 | Phase 4 | Complete |
| LOCK-03 | Phase 4 | Complete |
| LOCK-04 | Phase 4 | Complete |
| LOCK-05 | Phase 4 | Complete |
| LOCK-06 | Phase 4 | Complete |
| LOCK-07 | Phase 4 | Complete |
| LOCK-08 | Phase 4 | Complete |
| LOCK-09 | Phase 4 | Complete |
| LOCK-10 | Phase 6 | Pending |
| LOCK-11 | Phase 5 | Pending |
| LOCK-12 | Phase 5 | Pending |
| LOCK-13 | Phase 5 | Pending |
| LOCK-14 | Phase 5 | Pending |
| LOCK-15 | Phase 6 | Pending |
| RAID-01 | Phase 6 | Pending |
| RAID-02 | Phase 6 | Pending |
| RAID-03 | Phase 7 | Pending |
| RAID-04 | Phase 7 | Pending |
| RAID-05 | Phase 7 | Pending |
| RAID-06 | Phase 6 | Pending |
| RAID-07 | Phase 6 | Pending |
| CAPT-01 | Phase 8 | Pending |
| CAPT-02 | Phase 8 | Pending |
| CAPT-03 | Phase 8 | Pending |
| CAPT-04 | Phase 8 | Pending |
| CAPT-05 | Phase 8 | Pending |
| CAPT-06 | Phase 8 | Pending |
| CAPT-07 | Phase 8 | Pending |
| CAPT-08 | Phase 8 | Pending |
| MENU-01 | Phase 9 | Pending |
| MENU-02 | Phase 9 | Pending |
| MENU-03 | Phase 9 | Pending |
| MENU-04 | Phase 9 | Pending |
| MENU-05 | Phase 9 | Pending |
| MENU-06 | Phase 9 | Pending |
| MENU-07 | Phase 9 | Pending |
| MENU-08 | Phase 9 | Pending |
| PLAT-01 | Phase 2 | Complete |
| PLAT-02 | Phase 7 | Pending |
| PLAT-03 | Phase 1 | Complete |
| PLAT-04 | Phase 8 | Pending |

**Coverage:**
- v1 requirements: 64 total
- Mapped to phases: 64
- Unmapped: 0 ✓

---
*Requirements defined: 2026-10-04*
*Last updated: 2026-10-04 after roadmap creation (traceability filled)*
