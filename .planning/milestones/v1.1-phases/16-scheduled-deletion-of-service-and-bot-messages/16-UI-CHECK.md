# Phase 16 - UI-SPEC Review

Reviewed: 2026-11-10. Contract: 16-UI-SPEC.md. Status: **APPROVED** (0 BLOCK, 4 FLAG).
Dimensions are applied in their Telegram-menu form, as in 14-UI-CHECK.md and 15-UI-CHECK.md.

| Dimension | Verdict |
|-----------|---------|
| 1 Copywriting | PASS |
| 2 Visuals | PASS |
| 3 Color (status icons) | PASS |
| 4 Typography (text styling) | PASS |
| 5 Spacing (layout, rows, callbacks) | PASS |
| 6 Registry Safety | PASS |
| 7 Inventory Provenance | PASS |

## Checks against the code

1. **Collisions: none.** Page keys registered today are `s l c f w g r m a`, so `d` is free. Orders in use are 10, 20, 30, 40, 50, 60, 70, 80 and 90, so 85 is free. Existing action codes (settings_menu.go, lockdown, mreq, access, editor) are `oh op bk cl tg st no pk ls ly mo my ay ws wr wc wv wf gs gr gc gv gf rs rr rc rv rf` plus Staff `lk ua uy pg`. `dl` and `dr` are free. Row codes are controls scoped by `p=d`, and `t` is never an action, so reusing letters such as `ch` or `ba` on other pages cannot clash. `registerSettingsPage` also panics on a duplicate at init.
2. **Callback bytes recomputed** (prefix `settings|v1|` is 12 bytes). `a=st&c=-1001234567890&p=d&t=sj&v=86400` is 38 + 12 = **50** private and 46 group. The `dl` cell is 37, `dr` is 38, `v=n` is 46, and `a=op` is 37. All match the spec. The maximum is 50 against the 64 cap.
3. **Rows.** Main is 4 existing grid rows + the `d` cell (5) + Access + nav = 7, within 8 and 9. Service list is 6 + 1 = 7, Bot list 4, hub 2, service row 4, bot row 3. All hold. Hub with five sections is 3 + 1 = 4.
4. **Keys.** `grep -c '^settings_' locales/en.yml` gives 255. The spec's table groups 5 + 5 + 2 + 11 + 24 + 10 = 57, so the total is 312. Cell arithmetic holds. `Delete now` is 10 + `✅ ` = 12 against 13. Names are at most 18, so with an icon and a space they fit 20. `Manual punishments` is exactly 18, which is the tightest. Every key has a limits row proposal. `settings_main_state_line` and `settings_state_off_plain` exist in en.yml.
5. **M24 `Hidden`.** It is the minimum change. `settingsMainEntries` is the only place Main entries are built (settings.go:219 and settings_menu.go:232 both call it), so one skip there plus one guard in `openPage` covers every door. The signature `func(group *gotgbot.Chat) bool` fits the existing `MainState` shape. `staff.GetStaffGroupFresh` returns `(nil, nil)` for no row and an error otherwise, so "true only on a definite row, fail closed on error" is implementable. The A16-A2 fallback is sound but does not satisfy D-18's "not drawn". It is correctly labelled as owner-choosable.
6. **M28 no card and icons.** Justified: no member is acted on, and the change is reversible with one tap. This is the Phase 13 Off/Warn precedent. `❌` would misread, as the spec says. `🧹 🔔 🤖 ➖ 🗑 ⏳` are unused by other pages.
7. **Right line.** The classifier shape matches `memberReqClassifyBot`. One `getChatMember` per draw after the answer is the existing pattern. Main makes none. The line never disables a button. (A bot that is a plain member can delete its own messages, so the Bot screens are conservative. See F2.)
8. **Privacy.** No key or callback carries a Staff Group, title, ID, member or count other than `{n}` rules. The group line is the group's own title, as on every page. The page is hidden in a Staff Group, and a forged press gets `common_callback_invalid_request`, which names nothing.
9. **Navigation.** Row to list (`a=dl&t=<section>`), list to hub (`a=op&p=d`) and hub to Main (`bk`) follow the Phase 13 "Back to the page" precedent. Close is unchanged.
10. **Completeness.** DEL-01: 12 service rows with Keep, Now and delays. DEL-02: five bot kinds with Off and the D-04 presets. DEL-08: the line is on every screen. MREQ-15: the Auto punishments hint (row 10) says a mute notice stays up to 24 h, matching D-14. D-04 presets match exactly. D-15 to D-18 are honoured, with M24 as a justified amendment.

## FLAGs (non-blocking, fixable by the spec author now)

- **F1 (Dim 5/M24).** The spec says a failed Staff Group read makes "Main draw the entry with the unknown mark". `Hidden` returns false on error, and that does not change `MainState`. Say explicitly that `MainState` also returns ok=false when its Staff Group read fails, or drop the sentence. Opening is already the cannot-open edit, so safety is unaffected.
- **F2 (Dim 1, A16-A6).** On the Bot messages screens, "these settings do nothing yet" is false for a plain-member bot, which can delete its own messages. The spec accepts this as conservative, and D-17 fixes one rule for every screen. Prefer a softer `settings_del_no_right` wording that stays true for both lists, for example "I can't delete messages here, so some of these settings may not work." Owner decision.
- **F3 (Dim 1).** `mreq_btn_fixed` is `✅ I fixed it`. The hint quotes “I fixed it”. Confirm the quote check in `fuku/i18n/memberreq_locale_test.go` compares the words and not the `✅ ` prefix, or the new key will fail it in all languages.
- **F4 (Dim 3, A16-A3 / A16-A4).** The new icons and the absence of a card for `Delete now` are the owner's call. Defaults are sound.

## Owner-confirmation assumptions (A16-*)

A16-A2 (Hidden vs fallback), A16-A3 (icons), A16-A4 (no card), A16-A6 (right line on Bot screens), A16-A8 (57 keys; six covers droppable), plus the planner/live items A16-A9, A16-A12 and A16-A15 (phone width, native read, spike).

Nothing here is a BLOCK. F1 and F3 are fixable by the spec author now.
