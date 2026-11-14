---
phase: 17-command-edit-and-self-destruct-deletion
plan: 02
subsystem: modules
tags: [command-registry, gotgbot, go-parser-guard, docs-generator, mechanical-rewrite]

requires:
  - phase: 17-command-edit-and-self-destruct-deletion
    provides: plan 17-01 (member kinds and batch queue); this plan does not depend on its code
provides:
  - helpers.NewBotCommand (the recording shim) and helpers.IsBotCommand (case-insensitive registry lookup)
  - every command handler of the bot (131 module sites, WrapCommand's register, MultiCommand) built through the shim
  - TestNoRawNewCommand, a go/parser guard over fuku/ and main.go
  - the docs generator following the shim with no change to any generated page
affects: [17-03, 17-12]

plan_head_before: 6501229edbebb9dde42ea48834577f5bc6c1bb13
plan_head_after: e3830e0c2add3127d550e982200c5a200b547a88

actuals:
  tokens: 6700    # chars/4 over the added lines of 6501229..HEAD (26644 chars), tests and docs included
  tasks: 3
  commits: 4      # MEASURED: git rev-list --count 6501229..HEAD (before this SUMMARY commit)

tech-stack:
  added: []
  patterns:
    - "one constructor for every command handler: a mutex-guarded set of lower-cased names filled at registration, returning the library's own handler unchanged"
    - "go/parser guard test (ast.Inspect over selector and composite-literal forms, aliased imports included) instead of a source grep"

key-files:
  created:
    - fuku/utils/helpers/command_registry.go
    - fuku/utils/helpers/command_registry_test.go
    - fuku/modules/command_registry_test.go
  modified:
    - fuku/utils/helpers/command_pipeline.go
    - fuku/utils/helpers/decorators.go
    - fuku/modules/admin.go
    - fuku/modules/antiflood.go
    - fuku/modules/approvals.go
    - fuku/modules/backup.go
    - fuku/modules/blacklists.go
    - fuku/modules/connections.go
    - fuku/modules/devs.go
    - fuku/modules/disabling.go
    - fuku/modules/federations.go
    - fuku/modules/filters.go
    - fuku/modules/help.go
    - fuku/modules/language.go
    - fuku/modules/locks.go
    - fuku/modules/logchannels.go
    - fuku/modules/misc.go
    - fuku/modules/misc_extras.go
    - fuku/modules/notes.go
    - fuku/modules/pins.go
    - fuku/modules/reactions.go
    - fuku/modules/reports.go
    - fuku/modules/rules.go
    - fuku/modules/settings_input.go
    - fuku/modules/staff_action.go
    - scripts/generate_docs/parsers.go
    - scripts/generate_docs/parsers_test.go
    - scripts/generate_docs/main.go
    - docs/src/content/docs/contributing/index.mdx
    - docs/src/content/docs/architecture/request-flow.mdx
    - docs/src/content/docs/architecture/module-pattern.md
    - README.md
    - AGENTS.md

key-decisions:
  - "The guard flags any selector use of the library's NewCommand (call or function value) and any composite literal of its Command type, under every import name a file uses, so an aliased import or a stored function value cannot escape; a type reference such as the settingsCancelPassOn field `command handlers.Command` is allowed."
  - "TestRawCommandUsesFindsEveryForm proves the guard detects all four forms, so TestNoRawNewCommand cannot pass by looking at nothing; it also fails if fewer than 100 files were parsed."
  - "The docs generator regex matches only the shim's literal-name calls, exactly as before, so the table-driven StaffActions loop and the settings cancel descriptor stay unlisted; the old `handlers.NewCommand` form is no longer listed (the guard rejects it)."

patterns-established:
  - "Every command handler, wrapped or raw, is built by helpers.NewBotCommand; command deletion (plan 17-03) reads helpers.IsBotCommand."

requirements-completed: []

coverage:
  - id: D1
    description: "NewBotCommand matches like the library handler, records lower-cased names, IsBotCommand ignores case, 50 goroutines register and read under -race"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^TestBotCommandRegistry$' -v ./fuku/utils/helpers"
        status: pass
    human_judgment: false
  - id: D2
    description: "No construction of the library's command handler outside the shim in non-test Go code; every module path (WrapCommand, table loop, settings cancel pair, raw sites, MultiCommand) records its names; routing unchanged"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: unit
        ref: "go test -tags testtools -race -count=1 -run '^(TestNoRawNewCommand|TestRawCommandUsesFindsEveryForm|TestBotCommandRegistry)$' ./fuku/utils/helpers; go test -tags testtools -race -count=1 -run '^TestModulesRecordTheirCommands$' ./fuku/modules; command family run of the plan"
        status: pass
    human_judgment: false
  - id: D3
    description: "The docs generator finds commands through the shim; make generate-docs changes no generated page; make check-docs reports no drift"
    requirement: "DEL-03 (contribution only)"
    verification:
      - kind: integration
        ref: "make generate-docs then git status --porcelain -- docs/src/content/docs; make check-docs"
        status: pass
    human_judgment: false

duration: 40min
completed: 2026-11-12
status: complete
---

# Phase 17 Plan 02: The command registry shim Summary

Every command handler of the bot is now built by `helpers.NewBotCommand`, which records the lower-cased name and returns the library's own handler unchanged, so `helpers.IsBotCommand` answers "does this bot register that command" for command deletion in plan 17-03; a go/parser guard makes any other construction fail the build.

## What was built

- `fuku/utils/helpers/command_registry.go`: `NewBotCommand(name, resp)` (records `strings.ToLower(name)` under a `sync.RWMutex`, returns `handlers.NewCommand(name, resp)`, the only call of the library constructor in the repository) and `IsBotCommand(name)` (read lock, lower-cased).
- `command_pipeline.go` (`register`, both branches) and `decorators.go` (`MultiCommand`) go through the shim (3 sites).
- 131 module sites in 23 files rewritten by a scripted token substitution (`handlers.NewCommand(` to `helpers.NewBotCommand(`) plus a deterministic import fix (add the helpers import in sorted position of the internal group, remove the `ext/handlers` import only where no other use remained). The script lives in the scratchpad and is not committed. The diff outside the call token is exactly 6 added helpers imports and 5 removed handlers imports. `help.go` was not reformatted. The StaffActions table loop and `settingsCancelPassOn` keep their behavior and stay documented raw exceptions to the WrapCommand rule.
- Tests: `TestBotCommandRegistry` (matches `/probe17`, `/Probe17@<bot>` and a foreign `@OtherBot` suffix exactly as the library's own handler does, case-insensitive registry, 50-goroutine race), `TestNoRawNewCommand` plus `TestRawCommandUsesFindsEveryForm` (helpers), `TestModulesRecordTheirCommands` (modules: LoadRules answers `/rules` through a real dispatcher; LoadBans, LoadStaffActions, LoadSettings, LoadFederations and LoadNotes record sampled names; an unregistered name is false), and a shim case in the generator's tests (`TestParseCommands_BotCommandShim`).
- Generator: the command regex in `scripts/generate_docs/parsers.go` matches `helpers.NewBotCommand("name", module.handler)` and the `func` form with the same capture groups; the inventory label in `main.go` reads `NewBotCommand`.
- Docs: the three hand-written pages (`contributing/index.mdx`, `architecture/request-flow.mdx`, `architecture/module-pattern.md`) show the shim (their example import blocks follow), README.md's "Handler groups" clause names it, and AGENTS.md states the rule, the guard, `IsBotCommand`, the StaffActions and `settingsCancelPassOn` wording and the generator's shim-call rule.

## Verification results (real)

- `go test -tags testtools -race -count=1 -run '^(TestNoRawNewCommand|TestRawCommandUsesFindsEveryForm|TestBotCommandRegistry)$' -v ./fuku/utils/helpers`: all three PASS, no SKIP. Whole `./fuku/utils/helpers` under `-race`: `ok`.
- `go test -tags testtools -race -count=1 -run '^TestModulesRecordTheirCommands$' -v ./fuku/modules`: 6 subtests PASS (rules module, WrapCommand, table loop, settings and cancel, federations, notes).
- Task 2 command family: `go test -tags testtools -race -count=1 -timeout 30m -run '^(TestModulesRecordTheirCommands|Test.*Command.*|TestAnonymousAdminBanAfterProof|TestSettingsCancel.*|TestStaffAction.*)$' ./fuku/modules`: `ok ... 100.849s`, exit 0.
- Counts: `handlers.NewCommand(` outside tests and the shim: 0 (baseline 134); `helpers.NewBotCommand(` in `fuku/modules` non-test: 131; `NewBotCommand(` in `command_pipeline.go` plus `decorators.go`: 3.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules ./fuku/utils/helpers` clean.
- `gofmt -l` over the new and generator files prints nothing; over the 23 rewritten module files it prints exactly `fuku/modules/help.go` (unformatted at 42c48c4, not reformatted).
- `cd scripts/generate_docs && go test -count=1 ./...`: ok. `make generate-docs` then `git status --porcelain -- docs/src/content/docs` lists only the three hand-edited pages (no generated page changed); `make check-docs`: "No drift detected". `make check-translations`: all translations present.
- `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` exit 0 (read directly): "no dupl, gocyclo or godox findings on lines added since 8c0c827".
- `git diff --exit-code 8c0c827 -- go.mod go.sum` exit 0. No `reflect.Ptr` or `ioutil.` in the new files.
- Not run: `make lint` (cannot run here, Go 1.25 golangci-lint), `make test` (not this plan's task), the full settings family (not asked at this plan's end; the settings tests that touch `cancel` and `/settings` registration are in the command family run above), the CI-only govet `inline` analyzer.

## Deviations from Plan

None in code or behavior. Process notes:

- The first task's red commit (d8c1cd2) does not compile alone (it names the shim), as the plan prescribes; the green commit follows.
- The plan-commit ledger file was written after the red commit (the base `6501229` is the parent of that commit, so `commits: 4` counts all four commits of the plan).
- The guard went beyond the plan's minimum in two ways: it flags a function value use (`x = handlers.NewCommand`) as well as a call, and it handles aliased imports; `TestRawCommandUsesFindsEveryForm` proves it. A `handlers.Command` type reference (the `settingsCancelPassOn` field) is correctly allowed.
- Two README/docs follow-ups not named in the plan: the three docs examples' import blocks were adjusted so a contributor copying them gets compiling imports (the contributing template swaps the `ext/handlers` import for the helpers one, module-pattern adds the helpers import and drops `handlers` from the welcome example where it became unused).

## Requirements

DEL-03 is contributed to (the registry that decides "a command this bot registers") and stays Pending: `requirements-completed: []`. `requirements.mark-complete` was not run; only plan 17-12 completes requirements.

## Known Stubs

None. `IsBotCommand` has no production reader yet; plan 17-03 (command deletion) is its only user and this plan's tests use it.

## Threat Flags

None. No new network endpoint, auth path or file access; T-17-06 to T-17-08 and T-17-SC are mitigated as the plan states (registry filled only at registration, guard test, generator regex, go.mod clean).

## Self-Check: PASSED

- FOUND: fuku/utils/helpers/command_registry.go, fuku/utils/helpers/command_registry_test.go, fuku/modules/command_registry_test.go
- FOUND commits: d8c1cd2, ed78b0b, ac6a1f0, e3830e0 (all ancestors of HEAD)
