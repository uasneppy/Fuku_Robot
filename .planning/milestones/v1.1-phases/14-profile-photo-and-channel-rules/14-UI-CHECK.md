# Phase 14 - UI Check (gsd-ui-checker)

**Contract:** 14-UI-SPEC.md (status left `draft`)  **Checked:** 2026-11-08  **Overall: BLOCKED (1 BLOCK, 9 FLAGs)**

## Verdict per dimension (Telegram-bot mapping)

| # | Dimension (bot reading) | Verdict | Note |
|---|-------------------------|---------|------|
| 1 | Copywriting (copy, remedies, D-16) | **BLOCK** | B1: the mute notice promises a remedy ("ask an admin to approve you") that "I fixed it" does not honour. FLAGs F1, F2 |
| 2 | Visuals (page, screens, focal points) | PASS | F9 (duplicated sentence) is a nit |
| 3 | Color (status icons, interaction states) | PASS | icons distinct; A4 fail-closed press is consistent with D-05 and MREQ-10 (see below) |
| 4 | Typography (lengths, limits, tests) | FLAG | F4, F5, F6 |
| 5 | Spacing (row budget, callback bytes) | PASS | recomputed, all hold |
| 6 | Registry safety / privacy | PASS | no registry; D-18 holds (no channel title/ID, no Staff Group, no counts) |
| 7 | Inventory provenance | PASS | Tool: none, no inventory |

## Independent counts (all recomputed)

- `grep -cE '^settings_' locales/en.yml` = 235; `^mreq_` = 25. +12 and +4 = 247 and 29, 16 new keys. Row tables hold 12 and 4 rows. CONFIRMED.
- Reuse: no new key duplicates an existing meaning. No existing key says "basic group" / "linked channel not admin". `mreq_fixed_check_failed` and `settings_mreq_here_nolink` reuse is sound.
- Grid: `settingsMreqRuleGrid` does `k.row(cells[i:min(i+2,len)])`, so 7 cells give widths 2,2,2,1. Page = 4 + switch + nav = 6 rows, widths {2,2,2,1,2,2}. Limits 8 private / 9 group. CONFIRMED. Phase 15's 8th cell makes the grid 4 rows of 2 (still 6 rows, fits the lone `[ch]` row's right half). CONFIRMED.
- Callback bytes (12-byte prefix): mo 38/34, st 46/42, my 53/49. Largest settings 53, "I fixed it" 52, all under 64. CONFIRMED.
- English code points: rule_photo 25 (<=32), rule_channel 29 (<=32), btn_photo 21 template / 15 rendered, btn_channel 15 / 9 (rendered <=20), desc_photo 183 (<=200), desc_channel 136 (<=160), hint_photo 121 (<=140), hint_lookup 202 (<=230), basic 92 (<=110), nolink 63 (<=90), notadmin 78 (<=100), unknown 64 (<=90); mreq: m_photo 24 (<=26), m_channel 29 (<=30), remedy 59 (<=62), still_broken_many 66 (<=80). All within limits.
- Notice arithmetic: mute 141+66+211+85 = 503; warn 497; kick 489; warn-limit mute 428; English mute 331. CONFIRMED. `mreq_warn_reason` 260 / 149 CONFIRMED. Still-broken "other" 193 CONFIRMED. Still-broken "EN at ceilings" is **155, not 153** (53+30+26+20+20+6) - still <=160.
- The 400 ceiling is real but test-only: `memberReqNoticeMax = 400` in `fuku/i18n/memberreq_locale_test.go` (comment "at most 400 code points", rendered with a 66-rune name and all phrases). Alert caps are real: `memberReqAnswerMax = 160` (templates), `memberReqStillBrokenEn/Other = 160/200` (rendered), Telegram alert limit 200. Raising 400 to 520 needs: the constant and its comment, `memberReqRulePhrases` to seven (the renderer joins all phrases), the tail rendering for the three notices, and an amendment to 13-UI-SPEC's "400". It is not a Telegram limit (4096).
- The existing still-broken test renders "five phrases"; with seven phrases five cannot fit 200, so the count variant is needed. Justified.

## Safety points (a) to (f)

- **(a) Fail-closed `ch` press: PASS.** Refusing a punishing value on a definite no-link or unreadable link follows D-05 and does not contradict MREQ-10, because refusing a write punishes nobody. Off never makes the link check, so an admin is never stranded; retry works. `[Off]` alone is exactly D-05. Only wording nit: unknown state draws both rows and then refuses; acceptable, the `❔` line explains it.
- **(b) A9: mitigation inadequate (F3).** Under P14-A5 a bot that was removed from the channel gets an error, which is "unknown", never "definite non-admin". The suggested mitigation therefore does not cover the most likely case, and the member stays muted (up to 330 days) behind "try again in a moment" forever. This does not contradict a D (D-17 is honoured) but leaves roadmap criterion 3/4 ("nothing stuck") unresolved. A decision must be recorded now, not left as "unresolved".
- **(c) Photo remedy: B1 BLOCK and F2.** See below.
- **(d) Budget and privacy: PASS.** Page: 0 calls with the channel rule Off, basic group, or photo; at most 2 (cache miss) otherwise; channel screen at most 2; all through the lookup pacer, 3 s bound, answered empty first. Matches D-08 and D-15. Status lines are words only; the notice names no channel. Note: a group menu is readable by every member and the `notadmin` vs `nolink` lines reveal that a channel is linked and the bot's rights there; D-18 only forbids title/ID, so accepted.
- **(e) A8: FLAG, not acceptable as "no action" (F1).**
- **(f) Spikes.** S1 (photo shape): safe either way, wording only; contract states the rewrite. S2: safe (all unknown). S3: does not change D-05/D-06 safety (the failure is inert), but it makes "definite no linked channel" not definite for a non-admin bot, so the `[Off]`-only screen and the refusal alert would say a false "no linked channel". It changes wording only, but S3 is a third spike beyond D-19's two and must be added to 14-SPIKE.md (F7).

## BLOCK

**B1 (Dimension 1). A remedy the button does not honour.** Required property: every sentence in a notice that tells the member how to get out is true for the path the notice offers. Evidence: `mreq_notice_mute` plus `mreq_remedy_photo` says "ask an admin to approve you". An approved member is exempt for new punishments (D-04, `memberReqExempt`), but a muted member can post nothing, and section 7's "I fixed it" step 5 judges the photo with a fresh lookup and has no approval step (and `memberReqFixedRun` today has none), so after approval the member taps the button and is told "Not fixed yet: no visible profile photo". The contract's own claim "which is why asking an admin to approve them is a real remedy" is false for mute (true for warn and kick).
Exact fix (either, one is enough): (1) add to section 7 / the press order a step: before judging the photo and channel rules, read approvals fresh (`approvals.IsUserApprovedContext`; an error is `mreq_fixed_check_failed`, nothing changed) and treat an approved member as not broken for every rule, then say so in check 11 and the Phase 13 "I fixed it" order note; or (2) drop "or ask an admin to approve you" from the mute notice only (append a differently worded sentence there). D-16 and D-04 fix the wording for warn and kick, so (1) is the smaller change.

## FLAGs (recommendations)

- **F1 A8 (Dimension 1).** A channel-only breach gets "Fix your Telegram profile, then tap I fixed it", "your profile breaks this group's rules" and "fixed their profile": wrong instruction on the main MREQ-12 path. Fix: neutralise "profile" in `mreq_notice_warn`, `_kick`, `_mute`, `_fixed`, `_fixed_lockdown` (and `settings_mreq_hint_mute`) or give the channel rule its own sentence; if declined, record the owner's acceptance in Assumptions.
- **F2 D-16 mapping.** D-16 says "the photo rule's phrase names the real remedy"; the contract's phrase is "no visible profile photo" and the remedy is a separate sentence only on warn, kick and mute. Justified, but the "Decisions applied" row for D-16 must state this deviation instead of implying the phrase carries it.
- **F3 A9.** Record a decision, not "unresolved". Suggested: for the channel rule, when the member lookup is unanswerable, make one paced `getChatMember(channel, bot)`; a definite non-admin, or the bot-gone errors Bot API reports for "bot was kicked / chat not found", counts as inert (not broken). Also record that the member sees `mreq_fixed_check_failed` otherwise and an admin's `/unmute` frees them.
- **F4 Amendments.** The 400 to 520 notice cap changes a Phase 13 check; add it as amendment M10 in the table (it is only in P14-A7 now).
- **F5 Tests list.** "Keys that change" omits `memberReqAnswerKeys` (+`mreq_fixed_still_broken_many`) and the notice render of the tail; add them. Header of section 3 says "Exactly three conditions" but lists four rows (unknown); say "three conditions plus unknown".
- **F6 Arithmetic.** Still-broken "EN at its ceilings" is 155, not 153.
- **F7 S3.** Add S3 to the `14-SPIKE.md` list (D-19 names two) and say that with S3 negative the nolink wording must change to ask for bot admin.
- **F8 Alert caps.** State the real caps as in the repo: templates 160 (`memberReqAnswerMax`), rendered 160 EN / 200 other, Telegram 200.
- **F9 Duplication.** `settings_mreq_hint_lookup` repeats the "hidden photo counts as missing" sentence of `settings_mreq_desc_photo`; drop it from one (saves page length, keeps the 3800 margin).

## Approval

NOT approved: B1 is open. `14-UI-SPEC.md` was not edited (status stays `draft`, no Assumptions entry written). Re-run after B1 and the FLAG decisions are recorded.

---

# Re-check (after the researcher's revision for B1 and F1 to F9)

**Overall: APPROVED (0 BLOCK, 4 FLAGs).** `14-UI-SPEC.md` frontmatter set to `status: approved`; accepted FLAGs recorded in its Assumptions section.

| # | Dimension | Verdict | Note |
|---|-----------|---------|------|
| 1 | Copywriting | PASS | B1 closed by M11; F1 closed by M12; F2 stated in section 7 |
| 2 | Visuals | PASS | F9 closed (privacy sentence only on the photo screen) |
| 3 | Color / status / interaction | PASS | M13 classifier consistent with MREQ-10 |
| 4 | Typography / lengths | PASS | F4, F5, F6, F8 closed |
| 5 | Spacing / rows / callbacks | PASS | unchanged, still holds |
| 6 | Registry safety / privacy | PASS | no new identifier on any screen |
| 7 | Inventory provenance | PASS | n/a |

## Independent re-verification

- Counts: `^settings_` 235, `^mreq_` 25; 12 + 4 new keys listed, totals 247 and 29; 7 changed existing keys add no key. CONFIRMED.
- Old English of the 7 changed keys matches `locales/en.yml`. New English lengths computed: warn 72 (<=110), kick 80 (<=100), mute 76 (<=110), fixed 62 (<=70), fixed_lockdown 120 (<=130), still_broken 47 (<=60), settings_mreq_hint_mute 199 (<=220). The quoted button words are unchanged. All CONFIRMED.
- Changed new keys: hint_lookup 148 (<=170), nolink 71 (<=90). Page-fit estimate 2.8k vs 3800: consistent (hint ceiling 238 vs 322 before).
- Notice maxima: English as written kick 328, mute 324, warn 314 recomputed; ceilings 503/497/489/428 unchanged (ceilings did not move). Still-broken English 132 as written, 155 at ceilings, 193 other. Fixed notices 122/180 as written, 158/242 at ceilings. Alert caps stated as in the repo (160 / 160 EN-200 other / 200 Telegram). CONFIRMED.
- Rows and callbacks: grid 2,2,2,1, page 6 rows; 38/46/53 bytes. Unchanged, CONFIRMED.

## M11 and M13 against D-01..D-20 and Phase 13

- M11 (approvals at step 4b): runs after step 4's live `isMemberReqMute` marker check, so it can only free a mute the requirements placed; the presser must still be the held member; unknown approvals refuse with `mreq_fixed_check_failed`. It opens no way to punish an exempt member (it only removes a restriction for an exempt one) and none to free a mute the requirements did not place. It extends Phase 13's order, declared as M11; D-04 and D-17 are consistent (D-17 "re-check every enabled rule" is read as "for a member the rules apply to"). PASS.
- M13 (inert classifier): only ever turns a rule into "not broken / not punished", never into a punishment; step 4 still guards the unmute. D-07 forbids `IsBotMissingError`; the contract uses `IsBotGoneError`, a different helper, with a stated classification by answer and code. It contradicts no D. Extension to the screen line is consistent with D-08 (a determinable status), and the hidden-photo sentence living only on the photo screen still satisfies D-01 ("the rule's page text"). PASS with FLAGs R2, R3.

## Remaining FLAGs (accepted, non-blocking)

- **R1** A14 still names `settings_mreq_hint_lookup` as carrying the hidden-photo sentence. Fix: say `settings_mreq_desc_photo`.
- **R2** The inert classification is said to apply to "the clean predicate at the member's next message", which is the watcher, not the executor. Fix: state that the watcher makes no Telegram call for it (D-10), the classifier runs only in the job and at "I fixed it", an inert verdict is never cached as a pass, and `ClearHoldOnClean` never lifts a mute (it only deletes the hold row).
- **R3** `IsBotGoneError` is any 400 or 403, wider than the four wordings listed. Fix: say "any 400 or 403"; it errs only toward inert; S2 may narrow it.
- **R4** The "grep profile" planner check expects two photo keys; four new photo keys contain the word. Fix: list the four.
