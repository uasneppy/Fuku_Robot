---
name: gsd-new-project
description: Initialize a new project with deep context gathering and PROJECT.md
argument-hint: "[--auto]"
allowed-tools:
  - Read
  - Bash
  - Grep
  - Write
  - Agent
  - AskUserQuestion
requires: [config, phase, plan-phase]
---


<context>
**Flags:**
- `--auto` — Automatic mode. After config questions, runs research → requirements → roadmap without further interaction. Expects idea document via @ reference.
</context>

<objective>
Initialize a new project through unified flow: questioning → research (optional) → requirements → roadmap.

**Creates:**
- `.planning/PROJECT.md` — project context
- `.planning/config.json` — workflow preferences
- `.planning/research/` — domain research (optional)
- `.planning/REQUIREMENTS.md` — scoped requirements
- `.planning/ROADMAP.md` — phase structure
- `.planning/STATE.md` — project memory

**After this command:** Run `/gsd-plan-phase 1` to start execution.
</objective>

<execution_context>
@/home/user/Fuku_Robot/.claude/gsd-core/workflows/new-project.md
@/home/user/Fuku_Robot/.claude/gsd-core/references/questioning.md
@/home/user/Fuku_Robot/.claude/gsd-core/references/ui-brand.md
@/home/user/Fuku_Robot/.claude/gsd-core/templates/project.md
@/home/user/Fuku_Robot/.claude/gsd-core/templates/requirements.md
</execution_context>

<process>
Execute end-to-end.
Preserve all workflow gates (validation, approvals, commits, routing).
</process>
