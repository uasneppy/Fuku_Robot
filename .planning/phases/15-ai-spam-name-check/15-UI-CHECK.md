# Phase 15 - UI Check (gsd-ui-checker)

**Contract:** 15-UI-SPEC.md  **Checked:** 2026-11-08  **Overall: APPROVED WITH FLAGS** (first pass BLOCKED with 1 BLOCK and 7 FLAGs; second pass 0 BLOCK, 2 FLAGs; the contract is now `status: approved`)

## Verdict per dimension (Telegram-bot mapping, as in 14-UI-CHECK.md)

| # | Dimension (bot reading) | First pass | Second pass | Note |
|---|-------------------------|------------|-------------|------|
| 1 | Copywriting (copy, remedies, confirmation truthfulness) | **BLOCK** | PASS | B1: the three AI card keys promised a check "when they join" that does not happen with Check on join off. Closed by the orchestrator's decision UD-1 (a member recorded as joining is new and judged at first sight; the card keys stay true) |
| 2 | Visuals (page, screens, focal points) | FLAG F2 | PASS | the hint's Warn or Mute advice on the Off-only form is stated as intentionally not actionable (D-20) |
| 3 | Color (status icons, states) | PASS | PASS | the AI rule never draws the unknown mark; the composition table is exhaustive |
| 4 | Typography (lengths, ceilings, tests) | FLAG F3, F4 | PASS | stale test comments and the two-tail render helper are in "Tests that change" |
| 5 | Spacing (row budget, callbacks, cost) | FLAG F5, F6, F7 | PASS | "the status read makes none; the pipeline's own calls are unchanged"; "I fixed it" is 52 typical, 55 at the 19+19 digit limit |
| 6 | Registry safety / privacy | PASS | PASS | no registry; no new screen, alert or notice names the Staff Group, the channel, a member or a count; the AI status is `config.AppConfig` only |
| 7 | Inventory provenance | PASS | PASS | Tool: none |

## Independent counts (recomputed by the checker over `locales/*.yml` and the code)

- Key counts before the phase: `^settings_` 247 and `^mreq_` 29 in all eight files. +8 and +2 give 255 and 31 (10 keys, 80 texts).
- Callback bytes with the 12-byte prefix: `mo` 38 private / 34 group, `st` 46 / 42, `my` 53 / 49, single `[Off]` 46 / 42; "I fixed it" 52 typical and 55 at the parser's 19-digit limit. All under 64; no earlier size changes.
- Rule grid: eight cells make four rows of two; the page stays at six rows (`{2,2,2,2,2,2}`); the AI Off-only rule screen is two rows, the confirm card two rows.
- English lengths as written: `mreq_rule_ai` 16 (<=22), `mreq_remedy_ai` 67 (<=72), `settings_mreq_desc_ai` 255 (<=280), hint 163 (<=190), `settings_mreq_here_ai` 67 (alert rendered 84, at ceilings 164, inside 200), card keys 171, 162 and 183 (<=190, 180, 200).
- Notice worst case with the seven existing phrases, the new phrase and both remedies at their ceilings (real translations of the two new keys do not exist yet, so the ceilings are the bound): fr mute 506, id mute 483, uk kick 480, pt mute 472, ru kick 469, es kick 462, hi kick 459, en kick 425, all inside 520 (14 to spare in fr). At every ceiling the arithmetic is 636, so the binding gate is the rendered test over the real translations (M18).
- Still-broken alert with the four longest of eight phrases: es 159, uk 156, id 155, fr 153, hi 151, ru 151, pt 150, en 133, all inside 200.

## Amendments M14 to M23

All ten are real gaps or contradictions between the locked decisions and the code and none reopens D-01..D-18. Resolutions: M19 (the AI rule's own three card keys, required), M20 (one remedy sentence per remedy-bearing broken rule, in rule order), M21 and M22 accepted as written, M23 settled by D-19.

## Remaining FLAGs (non-blocking, both accepted)

- **F-a.** The three card keys say "when they join"; under D-19 a new member with Check on join off is judged at their first message. Mild looseness, not a false promise. Optional later: add "or at their first message" and raise the ceilings by about 26 characters.
- **F-b.** UD-1 (and research D-21) relies on the per-chat `nu` marker, which is Redis best effort and is written by the join entry only: with no Redis or a missed join a real new member is baselined and gets a free pass until they rename. This is the safe direction. The rulebook and `15-SPIKE.md` must say that "new" means "recorded as joined in the last 24 hours".
