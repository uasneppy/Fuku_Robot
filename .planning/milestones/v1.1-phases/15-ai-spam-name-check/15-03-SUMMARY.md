---
phase: 15-ai-spam-name-check
plan: 03
subsystem: i18n
tags: [locale, i18n, member-requirements, ai-spam-name, limits-tables]

requires:
  - phase: 15-ai-spam-name-check
    provides: plans 15-01 (ai_action, memberreq.RuleAI) and 15-02 (names request, namecanon); neither is read by this plan
provides:
  - ten new locale keys (8 settings_, 2 mreq_) in all 8 locale files, inside their UI-SPEC ceilings
  - memberReqLocaleLimits (31 rows) and settingsLocaleLimits (255 rows), extended before the keys
  - rendered checks for eight phrases, three remedy tail combinations, the warn reason (300), the AI alert with the longest rule name (200) and the eighth cell
affects: [15-04, 15-05, 15-08, 15-10, 15-11]

plan_head_before: e21d23735812523a4e8f580469d61fe2562765c5
plan_head_after: f10efdf0df0db2d4f798500f8de152be5f17ead4
actuals:
  tokens: 36000
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Limits rows first (red commit), keys second (green commit): a table that forgets a key fails"
    - "memberReqRender and memberReqSplice take a slice of tails, each appended after one space in rule order"

key-files:
  created: []
  modified:
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - fuku/i18n/memberreq_locale_test.go
    - fuku/i18n/settings_locale_test.go
    - AGENTS.md

key-decisions:
  - "The rule name and cell texts follow the UI-SPEC's non-binding hints (es/fr/pt cell 'Spam IA', id 'Spam AI', ru 'ИИ-спам', uk 'ШІ-спам', hi 'AI स्पैम नाम'); all other texts are translated from the English source in the register of the existing mreq_ and settings_mreq_ keys"
  - "The AI alert check renders settings_mreq_here_ai with the longest settings_mreq_rule_* name of the language (the plan says the longest rule name), as a separate helper called from TestSettingsLocaleKeys"
  - "AGENTS.md says mreq_remedy_ai is read by the notice builder through a key held in a variable, not through a RemedyKey table field, because no such field exists at HEAD (the photo remedy is a literal in memberreq_notice.go)"

requirements-completed: []

coverage:
  - id: D1
    description: "every word the AI rule shows or sends to a group exists in 8 languages inside its limits, before any code reads it"
    requirement: "MREQ-14, MREQ-10 (contribution only; completes none)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 ./fuku/i18n"
        status: pass
    human_judgment: true
    note: "the seven non-English texts need the owner's native read (see For the owner)"

duration: 30min
completed: 2026-11-08
status: complete
---

# Phase 15 Plan 03: AI rule locale keys Summary

**The ten locale keys of the AI spam-name rule (`mreq_rule_ai`, `mreq_remedy_ai` and eight `settings_` keys) exist in all 8 locale files within their UI-SPEC ceilings, the two limits tables were extended first and cover exactly 255 `settings_` and 31 `mreq_` keys, and the rendered checks hold a notice with all eight phrases and both remedy tails under 520 code points in every language.**

## Accomplishments

- `memberreq_locale_test.go` (red commit first): two new rows (`mreq_rule_ai` {22, 30}, `mreq_remedy_ai` {72, 100}), `memberReqRulePhrases` has eight entries in rule order, `memberReqRender` and `memberReqSplice` take a slice of tails (each after one space), the three remedy notices are rendered with the photo tail, the AI tail and both, the still-broken alert uses the four longest of eight, `mreq_warn_reason` with eight phrases is checked at 300 (new constant `memberReqWarnReasonMax`), and the stale "seven" and "ceilings make it satisfiable" comments are reworded.
- `settings_locale_test.go` (second red commit): eight new rows (255), `settings_mreq_here_ai` in `settingsAlertKeys` and rendered with the longest rule name (`settingsAIAlertFailures`), `settings_btn_mreq_ai` as the eighth cell rendered with the five punishment icons, `settings_mreq_confirm_ai_mute` in `memberReqQuotingKeys` (the words of `mreq_btn_fixed` appear verbatim in it in every language).
- Locale files: the ten keys beside the channel rule's keys in en, es, fr, hi, id, pt, ru and uk. English is the UI-SPEC text verbatim. No existing key changed text; `git diff` of the locale commits is insertions only (2 lines per file in the first, 8 in the second).
- `AGENTS.md`: the Testing sentence states 255 and 31, the ten keys and that the 520 cap is now a rendered check over the real translations.
- Real margins (computed over the real files): the worst notice (warn, kick or mute with eight phrases, the 66-code-point name and both tails) is 414 en, 427 es, 477 fr (the worst), 405 hi, 437 id, 431 pt, 424 ru, 434 uk, against 520. `mreq_warn_reason` with eight phrases is 153 to 185, against 300. The AI cell rendered is at most 14 code points (hi), against 20.

## Task Commits

1. `9c273f8` test(15-03): the limits and the renders know the AI phrase and its remedy (red: 31-row table, keys missing)
2. `f50a257` feat(15-03): the AI phrase and its remedy sentence in eight languages (green)
3. `c2510a1` test(15-03): the limits know the AI rule's page keys (red: 255-row table, keys missing)
4. `d0f9ba7` feat(15-03): the AI rule's page, status, hint and card texts in eight languages (green)
5. `f10efdf` docs(15-03): the new locale counts and the owner's reading list

## Verification (real results)

- Red states observed: after commit 1, `TestMemberReqLocaleKeys` failed in all 8 languages with "missing key mreq_rule_ai" and "missing key mreq_remedy_ai"; after commit 3 the three tests reported 110 failure lines before the keys existed.
- `go test -tags testtools -race -count=1 -run '^(TestMemberReqLocaleKeys|TestMemberReqNoProfileInReworkedKeys)$' ./fuku/i18n`: ok (after commit 2).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsLocaleKeys|TestSettingsLabelLengths|TestSettingsHelpFitsOneMessage|TestMemberReqLocaleKeys|TestMemberReqNoProfileInReworkedKeys)$' ./fuku/i18n`: ok (after commit 4).
- `go test -tags testtools -race -count=1 ./fuku/i18n` (the whole package): ok, exit 0.
- `make check-translations`: exit 0 ("All translations are present!"). `make generate-docs`: exit 0. `make check-docs`: exit 0 ("No drift detected"); the generated pages did not change, nothing to commit.
- `go test -tags testtools -race -count=1 -run '^(TestMemberReqRuleKeysExist|TestMemberReqScreensFitEveryLocale)$' ./fuku/modules`: ok, exit 0 (SQLite).
- `grep -cE '^settings_'` and `grep -cE '^mreq_'` over the eight locale files: 255 and 31 in every file. `grep -c 255 AGENTS.md` prints 1, `grep -c 'ten keys' AGENTS.md` prints 1.
- `CGO_ENABLED=0 go build ./...`: exit 0. `gofmt -l fuku/i18n`: nothing. `go vet -tags testtools ./fuku/i18n`: exit 0.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827`: exit 0, read directly ("no dupl, gocyclo or godox findings on lines added since 8c0c827").
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0.
- Not run: `make lint` and gosec (cannot run in this sandbox), `make test` (this plan does not require it), the settings test family in `fuku/modules` (the page does not read these keys yet).

## Deviations from Plan

None - plan executed as written. Two small judgement points, neither a behaviour change: (1) the alert render check is a separate helper `settingsAIAlertFailures`, called from `TestSettingsLocaleKeys`, to keep that test's complexity down; (2) AGENTS.md does not mention a `RemedyKey` field (the plan's sentence names one), because none exists at HEAD; it says the notice builder reads `mreq_remedy_ai` through a variable key.

## For the owner (native-speaker read, for the closing plan's 15-SPIKE.md)

The seven non-English texts were written by me from the English source and the register of the existing keys, not by a native speaker. Please have a native reader check these before relying on them (es, fr, hi, id, pt, ru, uk):

| Key | Why it needs a native reader |
|-----|------------------------------|
| `mreq_rule_ai` | fills a slot after a colon and joins other fragments with a comma; in ru ("имя, похожее на спам"), uk ("ім'я, схоже на спам") and hi ("स्पैम जैसा नाम") it must stay a noun phrase that does not agree with the addressee; it must not read as an accusation of the member in a public message. The grammar-sensitive one |
| `mreq_remedy_ai` | a second-person sentence appended to a notice, sometimes after the photo remedy; must name changing the name or username and the admin's approval (ru, uk use the plural-polite imperative "смените ... либо попросите", "змініть ... чи попросіть") |
| `settings_mreq_desc_ai`, `settings_mreq_hint_ai` | facts the owner will repeat to members: only the first name, last name and username go to Anthropic's Claude, never messages; members already here are checked only after a name change; new members when they join or at their first message |
| `settings_mreq_here_ai` | must match the wording of `lockdown_auto_ai_unavailable` and use the language's usual word for AI (IA in es, fr, pt; ИИ in ru; ШІ in uk; AI in hi, id) |
| `settings_mreq_confirm_ai_kick`, `_ban`, `_mute` | the consequence texts the owner reads at the destructive confirmation; the mute text quotes the `mreq_btn_fixed` words verbatim (the test enforces it) |
| `settings_btn_mreq_ai`, `settings_mreq_rule_ai` | cell name at most 18 code points; A15-A2 phone check of the Hindi ("AI स्पैम नाम", 14 code points rendered), Russian ("ИИ-спам") and Ukrainian ("ШІ-спам") cells, because Hindi combining marks can look wider than their count |

## Requirements

`requirements-completed: []`. MREQ-14 and MREQ-10 get their texts here and stay Pending, as does MREQ-01; they complete in plan 15-11. REQUIREMENTS.md is unchanged.

## Known Stubs

None. The ten keys are not read by any code yet by design (plans 15-04 to 15-08 read them); each has a row in a limits table and is covered by `TestMemberReqLocaleKeys` or `TestSettingsLocaleKeys`, so no unused identifier is left for CI.

## Threat Flags

None. No endpoint, auth path or file access; T-15-08 (length overflow) and T-15-09 (promising more than the code does: the English text is the approved UI-SPEC text and the quote check ties the mute card to the button) are mitigated as planned; T-15-SC: `go.mod` and `go.sum` unchanged.
