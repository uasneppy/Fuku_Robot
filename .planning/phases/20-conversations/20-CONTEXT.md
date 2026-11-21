# Phase 20: Conversations - Context

**Milestone:** v1.2 Conversations
**Imported from:** `.planning/research/xenobot-chat/PLAN.md` (the specification; read it whole). Supporting files in the same folder: `LORE.md` (source of record for every name, date and number), `COMIC-DIGEST.md` (chapter-by-chapter event order; unreliable on names, its Episode 1 section is wrong), `GEMINI-API.md` (Gemini facts read 2026-11-19, with unverified points).

## Owner's hard rules

1. The bot never converses in private chats.
2. Only a group's owner can enable conversations, from the bot's private chat.

## Decisions

Section 12 of `PLAN.md` lists the decisions made for the owner; build to them. The owner chose `claude-haiku-5-5` as the default Anthropic conversation model (2026-11-19). If the code proves a decision impossible, stop and report rather than choosing differently.

## Plans

The eight plans follow `PLAN.md` section 11 in order. Plan 20-01 ends with an owner review of `persona.md` and `archive.md` before any later plan starts.

## Constraints that bite

`AGENTS.md` rules apply in full. No test calls a real provider. `fuku/modules` tests stay light. Live checks with real keys are the owner's manual step (`PLAN.md` 7.5).
