---
quick_id: 261101-h7w
status: complete
---

# Summary: Ukrainian translation tool design spec

Wrote `docs/superpowers/specs/2026-11-01-uk-translation-tool-design.md` (commit e6a67d8) on branch
`claude/charming-babbage-356bx3`. Design only; no tool code, locale file or registration change.

Findings that shaped the spec:
- The `/lang` picker is driven by `ENABLED_LOCALES` (default `en`) and the callback by an allow-list in
  `language.go`, so a draft `uk.yml` in `locales/` is not user-reachable until both are changed.
- The locale loader and `scripts/check_translations` read every file in `locales/`, so the progress file lives
  under `scripts/translate_uk/`.
- Scripts in the main module must not import `alita/config` or `alita/db` (pre-main DB connection).

Refinement of the approved design: progress is stored as `key + hash of English text`, so a later English edit
re-opens the key.

Deviation from the quick workflow: no planner/executor subagents were spawned; the plan and the spec were written
inline because the task was a single document and the full design context was already in the session.
Local project GSD install (`.claude/gsd-core`) is missing `bin/lib/vendor/re2js.cjs`; the global install was used.

Next: user reviews the spec, then superpowers:writing-plans.
