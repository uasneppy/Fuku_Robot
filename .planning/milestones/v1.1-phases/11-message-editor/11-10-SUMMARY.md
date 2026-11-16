---
phase: 11-message-editor
plan: 10
subsystem: modules
tags: [settings-menu, message-editor, check, dry-run, tdd, parity]
requires:
  - phase: 11-message-editor
    provides: plan 11-02 content.TextLengthRefusal, ValidButtonURL, ExtractMedia and formatting.FillPlaceholders, VisibleLength; plan 11-05 normalizeRulesForHTML and buildRulesText; plan 11-08 settingsEditPageDef and settingsEditState
provides:
  - settingsEditCheck, the pure check and dry run of a message typed for one part (text, media or buttons) of a Welcome, Goodbye or Rules message
  - settingsEditVerdict with the stored values (Text, FileID, DataType, Buttons, CaptionIgnored) or the refusal (a locale key with its parameters, or the commands' own finished text)
affects: [11-11, 11-13, settings-message-editor]

actuals:
  tokens: 14000
  tasks: 3
  commits: 5

commits: 5
plan_head_before: 22258b63ccf2291dbb0f47ddac234082f7323e75
plan_head_after: 9cbd7152da338dfc574dcdd472e68b7a25a1f1af

tech-stack:
  added: []
  patterns:
    - "The commands' own converters and helpers are called, never re-implemented (MD2HTMLButtonsV2, MD2HTMLV2, TextLengthRefusal, ValidButtonURL, ExtractMedia, ConvertButtonV2ToDbButton, keyboard.BuildKeyboard), so the editor stores what the commands store"
    - "A dry run fills the stored message for the owner, a worst-case member and a minimal member with the pure FillPlaceholders and holds the longest visible UTF-16 length to Telegram's limit"
    - "Every conversion runs under settingsEditConvert, a recover that turns a converter panic into a need_text refusal and logs no text"

key-files:
  created:
    - fuku/modules/settings_edit_check.go
    - fuku/modules/settings_edit_check_test.go
  modified: []

key-decisions:
  - "settingsEditButtonRows takes the stored text instead of a bool: it finds the {rules}, {rules:same} or {rules:up} token itself and builds the full button list the join builds, then counts rows through keyboard.BuildKeyboard, so {rules:same} widens the last row and {rules:up} goes first exactly as at a join."
  - "The commands count the stored HTML before they trim it (preFixes), so the over-length refusal is made on the untrimmed HTML for parity; the trim and the dry run follow."
  - "A blank text is refused as need_text whether or not media is stored; only the 'filled text is empty for the minimal member' refusal is limited to a text-type message, as the plan says."
  - "Rules page with a media or buttons part is refused as need_text (Rules has a text part only)."

requirements-completed: []

duration: 25min
completed: 2026-11-04
status: complete

coverage:
  - deliverable: "A typed Welcome or Goodbye text is converted as /setwelcome and /setgoodbye convert it; a Rules text as /setrules converts it"
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditCheckTextParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditCheckRulesParity"
        status: pass
    human_judgment: false
  - deliverable: "Over-limit text refused with the commands' words; dry run over three fills refuses what Telegram would refuse; empty filled text and button lines in a text refused; Rules heading budget"
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditTextLimits"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditFilledLength"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditTextButtonLines"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditCheckRules"
        status: pass
    human_judgment: false
  - deliverable: "Media accepts one supported kind, refuses the rest and a stored text that cannot be its caption; buttons use the bot's syntax with the link, count and row refusals; malformed entities are recovered"
    verification:
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditMediaKinds"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditMediaTextLong"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditButtonRefusals"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditCheckButtonsParity"
        status: pass
      - kind: test
        ref: "fuku/modules/settings_edit_check_test.go#TestSettingsEditCheckMalformedEntities"
        status: pass
    human_judgment: false
---

# Phase 11 Plan 10: The pure check and dry run of a typed part Summary

**`settingsEditCheck` judges a message the owner typed for a part before anything is saved: it converts exactly as the commands convert, and refuses with the reason whatever Telegram would refuse at a join (over-length filled text, empty text, unsafe or too many buttons, unsupported media). It is pure: no Telegram call, no database read, no write.**

## Accomplishments

- Task 1 (tracer): Welcome and Goodbye texts go through `tgmd2html.MD2HTMLButtonsV2(msg.OriginalMDV2())` and the preFixes trim, Rules texts through `tgmd2html.MD2HTMLV2`. `settingsEditConvert` recovers a converter panic (an entity offset past the end of the text) and the check answers `settings_edit_refuse_need_text`, logging no text. Parity tests compare with `content.ExtractWelcome` (bold plus link plus emoji, spoiler, a right-to-left word, bold and italic on the same words, a `{first}` placeholder, a typed `*x*` with no entity) and with the `/setrules` conversion.
- Task 2 (TDD): the commands' own over-length reply (`content.TextLengthRefusal`, returned as `RefusalText`, 4096 code points, 1024 when the group already has media), button lines in a text (`settings_edit_refuse_text_buttons`), a blank text or a text empty for the minimal member (`need_text`), and the dry run (`settingsEditFilledLongest`: the owner, `settingsEditWorstUser` with 64-letter names and a 13-digit ID, `settingsEditMinimalUser`, `{count}` as nine digits) holding the filled visible length to 4096 or 1024 with `settings_edit_refuse_too_long_filled` and the worst `{n}`. The Rules text is checked against the longest `/rules` message over the 8 languages (`settingsEditRulesHeadings`: both the `rules_for_chat_header` composition and the `rules_for_chat` template) with `normalizeRulesForHTML` applied as `/rules` applies it; placeholders are not filled in rules.
- Task 3 (TDD): media accepts exactly the kinds `content.ExtractMedia` stores (a GIF as a document) and refuses a video note, contact, location, venue, poll, dice, story, plain text and an empty message with `need_media`; the stored text must fit as a caption by the raw count and by the worst-case fill (`settings_edit_refuse_media_text_long`); a caption on the media message is ignored and `CaptionIgnored` says so. Buttons: no readable line is `buttons_syntax`, an empty name becomes `Greeting`, a link that is not http(s) with a host is `button_url` naming the button's number only, and more than 100 buttons or 8 in a row (counting a `{rules}` token as the join places it) is `buttons_limit`.

## Task Commits

1. Task 1 (tracer): `ef84d4f` feat(11-10): convert a typed welcome, goodbye or rules text as the commands do
2. Task 2 RED: `d0fe378` test(11-10): add failing tests for the text limits and the dry run
3. Task 2 GREEN: `c687adc` feat(11-10): refuse a typed text Telegram would refuse at a join
4. Task 3 RED: `39890fe` test(11-10): add failing tests for media, buttons and malformed entities
5. Task 3 GREEN: `9cbd715` feat(11-10): check typed media and buttons before they are saved

## TDD Gate Compliance

Tasks 2 and 3 are the plan's `tdd="true"` tasks (the plan is `type: execute`, so no plan-level gate applies; the `gsd_run check tdd-red-evidence` classifier was not run and no evidence record was persisted).

- RED, Task 2: the real `go test` run showed `TestSettingsEditTextLimits`, `TestSettingsEditFilledLength`, `TestSettingsEditTextButtonLines` and `TestSettingsEditCheckRules` failing on their assertions (for example "a 4097 character text was accepted", `verdict OK=true key="", want settings_edit_refuse_too_long_filled`), the package compiling. Subtests of behaviour that already held (4096 accepted, `Hi {last}` accepted) passed, as expected. Semantic assessment: the targets executed and failed on the planned assertions for the intended reason.
- RED, Task 3: `TestSettingsEditMediaKinds`, `TestSettingsEditMediaTextLong`, `TestSettingsEditCheckButtonsParity` and `TestSettingsEditButtonRefusals` failed on their assertions (`a photo was refused: "settings_edit_refuse_need_text"`, `valid button lines were refused`). `TestSettingsEditCheckMalformedEntities` already passed at RED, because Task 1's recover covers the text parts and the stub buttons part answered `need_text` for everything; it is a regression guard for the buttons branch, not a RED. The RED commit also adds the four verdict fields (`FileID`, `DataType`, `Buttons`, `CaptionIgnored`) to the production struct so the tests compile (a build failure is not a valid RED); the GREEN commit fills them.
- GREEN: both GREEN commits make every target pass; REFACTOR: none needed. The Task 1 tracer was committed as one `feat` commit with its tests, as the plan says for a tracer.

## Verification (real results)

- `go test -tags testtools -race -count=1 -run '^(TestSettingsEditCheckTextParity|TestSettingsEditCheckRulesParity)$' ./fuku/modules`: ok (Task 1, run before its commit).
- `go test -tags testtools -race -count=1 -run '^(TestSettingsEditTextLimits|TestSettingsEditFilledLength|TestSettingsEditTextButtonLines|TestSettingsEditCheckRules|TestSettingsEditCheckTextParity|TestSettingsEditCheckRulesParity)$' ./fuku/modules`: ok.
- `go test -tags testtools -race -count=1 -run '^(TestSettingsEditMediaKinds|TestSettingsEditMediaTextLong|TestSettingsEditButtonRefusals|TestSettingsEditCheckButtonsParity|TestSettingsEditCheckMalformedEntities)$' ./fuku/modules`: ok.
- `go test -tags testtools -race -count=1 -run '^TestSettingsEdit' ./fuku/modules` (the focused command of the plan, covering every earlier editor test too): `ok ... 62.9s`.
- Acceptance greps on `settings_edit_check.go`: `func settingsEditCheck(` 1; `content.ExtractMedia(` plus `content.ValidButtonURL(` 2; `content.TextLengthRefusal(` 2 at the end (1 after Task 2, the second is the media caption check of Task 3); `formatting.FillPlaceholders(` 1; `FormattingReplacer` 0; the log grep for Text, Caption, Url, FileId or Name 0; the purity grep (db.DB, SendMessage, GetChatMember, greetings and rules reads, comments excluded) 0.
- `CGO_ENABLED=0 go build ./...`: exit 0. `go vet -tags testtools ./fuku/modules` and `gofmt -l` on both files: clean. `git diff --exit-code 72a692b -- go.mod go.sum`: exit 0.
- `python3 .../lint-local.py 72a692b`: "no dupl, gocyclo or godox findings on lines added since 72a692b" (run after the last code commit; the staticcheck and errcheck parts print nothing).
- Not run: `make lint` (installed golangci-lint is built with Go 1.25, a sandbox fact; the stand-in ran instead); `make test` (not asked by this plan); PostgreSQL (nothing here needs it).

## Requirements

Contributes to EDIT-03, EDIT-04, EDIT-05 and EDIT-11 (what is accepted, how it is stored and what is refused). It completes none of them: the door saves a checked message in plan 11-11 (completes EDIT-03) and plan 11-13 (completes EDIT-04, EDIT-05, EDIT-11). `requirements-completed` is empty, `requirements.mark-complete` was not run and all four stay Pending in REQUIREMENTS.md.

## Deviations from Plan

**1. [Interface] `settingsEditButtonRows` takes the stored text, not a bool**
- The plan's signature is `settingsEditButtonRows(buttons []db.Button, rulesToken bool)`. A bool cannot tell `{rules}` from `{rules:same}` (which widens the last row) or `{rules:up}` (which goes first), so the function takes the stored text, finds the token with the join fill's own pattern and counts rows through `keyboard.BuildKeyboard`. No test or caller depends on the bool form; behaviour for a plain `{rules}` is as the plan says (one more button).

**2. [Test] The "overlapping bold and italic" parity case uses nested entities**
- A true partial overlap (bold 0..9, italic 5..13) makes the gotgbot converter panic. The check recovers it as `need_text` (this is the malformed-entity behaviour the plan wants), so the parity case uses bold and italic on the same words, which the converter handles.

**3. [Process] The RED commit of Task 3 adds four struct fields to the production verdict type** (see TDD Gate Compliance). Tasks 2 and 3 helpers (`settingsEditWorstUser`, `settingsEditMinimalUser`, `settingsEditRulesHeadings`, `settingsEditFilledLongest`) are used by `settingsEditCheck` itself.

**Total deviations:** 1 interface adaptation, 1 test-case adjustment, 1 process note. **Impact:** none on the contract the later plans use (`settingsEditCheck`, its input and verdict types).

## Known Stubs

None. Nothing calls `settingsEditCheck` in production yet: plans 11-11 and 11-13 are its only callers by design (the door).

## Threat Flags

None. The check is pure (no endpoint, auth path, file access or schema change). T-11-34 (dry run, empty-text check, button limits), T-11-35 (`ValidButtonURL`, refusal names the number only), T-11-36 (recover), T-11-37 (no log carries text, names, links or file IDs; grep gate 0) and T-11-38 (the commands' converters are called; parity tests) are mitigated and tested; no package was installed (T-11-SC).

## Issues Encountered

- The first Rules test run failed because the helper read the locale before `withRealLocales` installed it; the helper now installs it itself.
- The accepted limitation A11 (64 astral characters count as two UTF-16 units each) stands: the worst-case member uses BMP letters.

## Next Phase Readiness

Plans 11-11 and 11-13 can call `settingsEditCheck(settingsEditCheckInput{...})` between their authority checks and the claim and store `Text`, `FileID` and `DataType`, or `Buttons`, from an OK verdict; a refusal is `RefusalText` when not empty (already translated) and otherwise `RefusalKey` with `RefusalParams`.

## Self-Check: PASSED

- Created files exist: `fuku/modules/settings_edit_check.go`, `fuku/modules/settings_edit_check_test.go` (found on disk).
- Commits `ef84d4f`, `d0fe378`, `c687adc`, `39890fe` and `9cbd715` are ancestors of HEAD; `git rev-list --count 22258b6..HEAD` was 5 before this SUMMARY commit.
- Every acceptance criterion was re-run after the last code commit and passes (results above).
