# 20-05 Summary: settings page

- Config: `ENABLE_CHAT` (default false), `GEMINI_API_KEY` (logredact), `CHAT_ANTHROPIC_MODEL`, `CHAT_GEMINI_MODEL`.
- Page `p` (`settings_chat.go`): owner-only and private-only; toggle with a confirm card, provider, audience.
  Group-menu presses are refused, so no group door can write. 16 locale keys in all 8 files.
- Tests: owner flow, Main and group menu, authority (non-owner admin, gated admin, group menu), not configured,
  drift, callback bytes, every-locale fit; privacy sweep renders the page.
- Deviation: the page does not join `TestSettingsGateAllDoors` (that test needs every door to decide alike; this page
  admits only the creator). Its authority is covered by its own tests.
- Verified: all `TestSettings*` of `fuku/modules` pass, i18n and config tests pass, lint clean, translations and docs checks clean.
