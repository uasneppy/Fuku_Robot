---
phase: 09-settings-menu
plan: 14
subsystem: docs
tags: [settings-menu, real-client-checklist, manual-verification]
requires:
  - phase: 09-settings-menu
    provides: the menu behaviour described in plans 09-01 to 09-12
provides:
  - "09-SPIKE.md, the owner's real-client checklist for the settings menu"
affects: [09-VALIDATION manual-only rows]
tech-stack:
  added: []
  patterns: ["checklist shape of 08-SPIKE.md"]
key-files:
  created:
    - .planning/phases/09-settings-menu/09-SPIKE.md
  modified: []
key-decisions:
  - "14 checks instead of the planned 12: one for the Captcha, Antiflood and Lockdown trigger pages against their commands (the pending items of 09-01, 09-08, 09-09, 09-10) and one for the native-speaker read of the new strings (VALIDATION manual row)"
requirements-completed: [MENU-01, MENU-02, MENU-03, MENU-04]
duration: 20 min
completed: 2026-10-30
status: complete
commits: 1
plan_head_before: 8fa613e06824bded1259955b60ba2d10389c9108
plan_head_after: 0f29c8c5fba46cf6602e9655d53e20306ed53e6d
actuals:
  tokens: 9000
  tasks: 1
  commits: 1
---

# Phase 9 Plan 14: Settings menu real-client checklist Summary

**A 14-check hand checklist (09-SPIKE.md) for everything the fakes cannot prove about the settings menu: deep link for a never-started admin, old private Close, alert rendering, hi/ru truncation, forwarded menu, forum topic, fast steppers and 429, slow Staff build, public lift notice, Link/Unlink privacy.**

## What was built

`09-SPIKE.md` follows `08-SPIKE.md`: why it exists, before you start, 14 numbered checks (each with steps, Pass looks like, If not, Change), what to read in the output, accepted limits (no auto-close, name not mention, 10 s cross-replica skew D-28, antiflood first-write limit D-27), decisions, constants and where they live, the live-only list mapped to `09-VALIDATION.md`, and what the checklist never does. Checks 1 to 12 are the planned ones in the planned order. Check 13 collects the pending human items of the page plans; check 14 is the translation read.

Every command, button label, alert text and constant named was grepped in `alita/` and `locales/en.yml`.

## Deviations from Plan

**1. [Rule 2 - Missing coverage] Two checks added.** The summaries of 09-01, 09-08, 09-09 and 09-10 and the Manual-Only table of 09-VALIDATION list human items (page values against commands, Turnstile unavailable mark, Off releasing a pending member, antiflood bot-admin notice, restrict alert on triggers; native-speaker read) that none of the 12 planned checks covered. Added as checks 13 and 14. Files modified: only `09-SPIKE.md`.

**2. [Process] Commit trailer.** The plan text prints a model name in the trailer; the harness trailer lines were used instead, as instructed. No model identifier appears in the doc.

## Known Stubs

None.

## Pending human verification

This whole plan is the human verification list; the owner runs it by hand.

## Self-Check: PASSED

- `.planning/phases/09-settings-menu/09-SPIKE.md` exists; 14 `### N.` headings, "Pass looks like" in all 14, one `## What the checklist never does`.
- Commit `0f29c8c` exists.
