---
phase: 13-member-requirements
plan: 02
subsystem: modules
tags: [member-requirements, unicode, rules-engine, pure-functions, tdd]
requires:
  - phase: 13-member-requirements
    provides: fuku/db/memberreq Rule identifiers and order, ActionOf, the RequirementAction constants (13-01)
provides:
  - memberReqRules, the rule table in rule order (ln un ru ar zh) with code, bit, stored rule and four locale keys
  - the name classifier (memberReqHasRussian, memberReqHasArabic, memberReqHasHan) and the invisible-last-name test (memberReqVisible, memberReqBlank)
  - memberReqBroken, memberReqSeverity, memberReqStrictest
  - memberReqPassValue, memberReqDisplayName, memberReqMention
affects: [13-05, 13-06, 13-07, 13-09, 13-11, 13-13, member-requirements]

actuals:
  tokens: 8000
  tasks: 3
  commits: 6

commits: 6
plan_head_before: 8e6c6323c8a352a1494341efc7729a88172875fe
plan_head_after: 518e5a1c9a1dae59a755e835b92bbafb60d0a4c3

tech-stack:
  added: []
  patterns:
    - "Rule table as data: code, bit, stored rule, locale keys and a Broken func per row, so later phases append rows"
    - "Severity table instead of the stored action numbers (warn 1, kick 2, ban 3, mute 4 are not in order of severity)"
    - "Names are spliced into a notice after translation, never through the translator"

key-files:
  created:
    - fuku/modules/memberreq_rules.go
    - fuku/modules/memberreq_rules_test.go
  modified: []

key-decisions:
  - "The classifier functions take one string; the rule table's Broken func applies them to the first name and the last name and never to the username"
  - "memberReqMention wraps the fallback text in the isolates too, so every link text has the same shape (at most 66 code points)"
  - "memberReqPassValue length-prefixes each name instead of joining with a separator, so a character moved between two names always changes the value"
  - "memberReqBroken and memberReqStrictest treat nil settings as every rule off, and memberReqPassValue fingerprints nil like all-off"

patterns-established:
  - "A corpus test (verdicts from the research probe) is committed before the classifier; its expected verdicts are never edited to pass"

requirements-completed: []

coverage:
  - id: D1
    description: "The 67-row Unicode corpus gets the research S-08 verdicts: 15 Ukrainian names never match the Russian rule, ы ъ э ё match after NFC (a decomposed ё too), Belarusian and Bulgarian names match only with those letters, Arabic letters match and Arabic-Indic digits and a lone mark do not, Han matches and kana and Hangul do not, and each rule table row's Broken agrees with the classifier"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestMemberReqRulesCorpus$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D2
    description: "A last name with no visible character (every invisible and whitespace-like code point of S-08, alone and in neighbouring pairs and all together) counts as none, U+0640 and U+FFFC count as visible, the username rule reads Username alone, and the name rules ignore the username"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqBlankLastName|TestMemberReqUsernameRule)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D3
    description: "The rule table has codes ln un ru ar zh in order, bits 1 2 4 8 16, the 13-01 stored rules and non-empty keys; the broken mask lists only rules that are on; the strictest of every pair and triple of broken rules is ban over kick over mute over warn, and none gives Off"
    requirement: "MREQ-13"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqRuleTable|TestMemberReqSeverity|TestMemberReqStrictest)$' ./fuku/modules"
        status: pass
    human_judgment: false
  - id: D4
    description: "The pass value is 16 lowercase hex characters, stable for equal inputs and different when any action, name or an empty-versus-space username changes; a member is named by visible first name, else @username, else the caller's text, cut to 64 runes, wrapped in U+2068/U+2069 and escaped"
    requirement: "MREQ-05"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestMemberReqPassValue|TestMemberReqMention)$' ./fuku/modules"
        status: pass
    human_judgment: false

status: complete
---

# Phase 13 Plan 02: Member requirements rules engine Summary

**A pure rules engine in `fuku/modules/memberreq_rules.go`: a rule table (ln un ru ar zh), letter-based name classifiers with NFC, a visible-character test for the last name, a severity-ordered strictest choice, a clean-member fingerprint and a safe, escaped, isolate-wrapped member mention, all pinned by a 67-row Unicode corpus.**

## Performance

- **Tasks:** 3 of 3 (task 1 tracer, tasks 2 and 3 TDD)
- **Commits:** 6 (three RED `test(13-02)`, three GREEN `feat(13-02)`)
- No Telegram call, database access, handler, locale key or kill-switch change; nothing here reaches a real group.

## Accomplishments

- `memberReqRules`: five rows in rule order with code, bit (1 2 4 8 16), stored `memberreq.Rule`, and `LineKey`, `CellKey`, `DescKey`, `PhraseKey` (`settings_mreq_rule_*`, `settings_btn_mreq_*`, `settings_mreq_desc_*`, `mreq_rule_*`). `memberReqRuleByCode` finds a row by code.
- Classifiers: Russian is the letters ы ъ э ё in either case after `norm.NFC` (a decomposed ё is caught, NFKC adds nothing), Arabic is `unicode.Is(unicode.Arabic, r) && unicode.IsLetter(r)`, Chinese is any `unicode.Han` code point. Every name rule judges the first and the last name and never the username.
- `memberReqVisible` and `memberReqBlank`: a letter, number, punctuation or symbol that is not `Other_Default_Ignorable_Code_Point` and not U+2800. This counts whitespace, every format character, marks, the Hangul fillers and the Braille blank as no last name, and U+FFFC and tatweel as visible.
- `memberReqBroken` (mask of rules that are on and broken), `memberReqSeverity` (ban 4, kick 3, mute 2, warn 1, off 0) and `memberReqStrictest`, never comparing the stored numbers.
- `memberReqPassValue` (16 hex characters of SHA-256 over the five actions and the length-prefixed names), `memberReqDisplayName` and `memberReqMention` (cap 64 runes, U+2068/U+2069, `formatting.MentionHtml`).

## TDD Gate Compliance

| Gate | Commit | Evidence |
|------|--------|----------|
| RED task 1 | f9d6895 `test(13-02): pin the name rules on a Unicode corpus` | with a compile-only classifier stub returning false, `TestMemberReqRulesCorpus` failed in 34 subtests on assertions (for example the Russian, Arabic and Han rows); semantic check: the test ran and failed on the planned verdicts, not on a build error |
| GREEN task 1 | 0859a0b `feat(13-02): judge first and last names by letters` | corpus passes |
| RED task 2 | 70d3ca1 `test(13-02): add failing tests for invisible last names and the strictest punishment` | `TestMemberReqBlankLastName`, `TestMemberReqUsernameRule`, `TestMemberReqSeverity` and `TestMemberReqStrictest` failed on assertions against stubs (`memberReqBlank("") = false, want true` and so on); `TestMemberReqRuleTable` passed at once because the table came with the tracer (a guard, not a RED test) |
| GREEN task 2 | 129912f `feat(13-02): pick the strictest punishment among broken rules` | all pass |
| RED task 3 | 3942522 `test(13-02): add failing tests for the pass value and the member mention` | `pass value "" is not 16 lowercase hex characters`, and `mention "" is not a link` and four more in the mention subtests |
| GREEN task 3 | 518e5a1 `feat(13-02): fingerprint a clean member and name a member safely` | all pass |

`gsd_run check tdd-red-evidence` was not run (the plan is `type: execute`); the evidence is the real test output.

## Task Commits

1. Task 1 (tracer): f9d6895 (RED), 0859a0b (GREEN)
2. Task 2: 70d3ca1 (RED), 129912f (GREEN)
3. Task 3: 3942522 (RED), 518e5a1 (GREEN, also the `U+200B` escapes in the corpus test)

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^TestMemberReq' -v ./fuku/modules`: PASS for `TestMemberReqRulesCorpus`, `TestMemberReqBlankLastName`, `TestMemberReqUsernameRule`, `TestMemberReqRuleTable`, `TestMemberReqSeverity`, `TestMemberReqStrictest`, `TestMemberReqPassValue`, `TestMemberReqMention`; `ok`.
- `CGO_ENABLED=0 go build ./...`: ok. `go vet -tags testtools ./fuku/modules`: clean. `gofmt -l fuku/modules/memberreq_rules*.go`: empty (it lists seven older files of the package that are not mine).
- `git diff --exit-code 8c0c827 -- go.mod go.sum`: exit 0 (`golang.org/x/text v0.42.0` is already a direct requirement).
- Lint stand-in `lint-local.py 8c0c827`: exit 0, run before the last code commit and again before this SUMMARY. It first reported three `ST1018` findings (a raw zero-width character in three corpus literals); I rewrote them as `U+200B` escapes in the last commit, with no change to any verdict. `make lint` itself cannot run here (golangci-lint built with Go 1.25).
- `make generate-docs` left no diff, `make check-docs`: "No drift detected", `make check-translations`: "All translations are present".
- Acceptance greps: `norm.NFC` present, at least three of the Ukrainian names present in the test, `Other_Default_Ignorable_Code_Point` present, one `func memberReqStrictest(`, one `func memberReqMention(` and `u2068` present.

## Deviations from Plan

None - plan executed as written. Notes:

- The tracer commit's last-name and username rows had a placeholder `Broken` (always false) until task 2, as the plan allowed; task 2 replaced them with the real rules.
- The Task 2 tests include a `TestMemberReqSeverity` beyond the named list, so that `memberReqSeverity` is called directly.
- Test hygiene: these tests are pure, touch no users, chats, Redis or cache, and so need no cleanup.

## Known Stubs

None. (The RED commits carried compile-only stubs; all are replaced.)

## Threat Flags

None. No new network endpoint, auth path or file access. T-13-05 (evasion by zero-width, decomposed or invisible characters) is covered by the NFC scan, the visible-character test and the corpus and blank tests; T-13-06 (HTML or bidi injection through a name) by the escaping, 64-rune cap and isolates in `TestMemberReqMention`; T-13-07 (a Ukrainian member caught by the Russian rule) by the 15 Ukrainian rows that must pass; T-13-SC by the unchanged go.mod and go.sum.

## Notes for later plans

- Accepted limits (research S-08), documented in the corpus comment: a Latin ë in place of ё evades the Russian rule (the corpus row `Пëтр` pins it), and characters added after Unicode 15.0 (CJK Extension I, Arabic letters of Unicode 16.0) are not detected until Go updates its tables.
- Every function here has no production caller yet; each is called by a test of this plan. First production callers: the rule table and `memberReqRuleByCode` by 13-05 and 13-06 (page and rule screen), `memberReqBroken` and `memberReqStrictest` by 13-07 (the punisher), `memberReqPassValue` and the classifier by 13-09 (the watcher), `memberReqMention` and `memberReqDisplayName` by 13-11 (notices), and `memberReqBroken` again by 13-13 ("I fixed it"). `memberReqMention` takes the "no visible name and no username" text as a parameter (`mreq_name_unknown` rendered by the caller with the ID), because the locale key lands in 13-04.
- `memberReqRussianLetters`, `memberReqVisible` and `memberReqNameCap` are helpers of the functions above and are used by them.
- Requirements: this plan completes none. MREQ-13 stays Pending until 13-09 (members are checked at their message and the page says the rules are letter-based) and MREQ-05 until 13-11 (every violation path posts its notice).
- AGENTS.md is not changed by this plan (the plan names no AGENTS.md edit).

## Self-Check: PASSED

- Files exist: `fuku/modules/memberreq_rules.go`, `fuku/modules/memberreq_rules_test.go`.
- Commits are ancestors of HEAD: f9d6895, 0859a0b, 70d3ca1, 129912f, 3942522, 518e5a1.
- `git rev-list --count 8e6c632..HEAD` was 6 before this SUMMARY commit.
