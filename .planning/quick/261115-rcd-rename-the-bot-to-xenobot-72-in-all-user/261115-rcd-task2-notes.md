# 261115-rcd Task 2 notes (untracked, never committed)

Commit: 98d363d "fix: rename the image labels and GitHub files to Xenobot-72 and drop the original developer's metadata" (on top of a721b5a). Branch claude/ecstatic-goldberg-qsss05.

## Rescan result (Task 2 area)

PAT-DEV hits before: CODEOWNERS 1, FUNDING.yml 2, issue template config 2, dependabot 3, .goreleaser.yaml 2, docker/alpine, alpine.debug, pr-build 3 each. All gone (git grep over the verify path set is empty).
Name-word hits before: .claude/CLAUDE.md 2, CODEOWNERS 1, ci.yml 1, .golangci.yml 1, .goreleaser.yaml 1, AGENTS.md 1, docker-compose.yml 1, docker/* 2 each, scripts/generate_docs/main.go 2, scripts/migrate_psql.sh 1. All gone.

## Classification (Task 2 area)

| File | Class | Decision |
|---|---|---|
| .goreleaser.yaml | b + c | title label is the literal Xenobot-72 (not .ProjectName); description "Xenobot-72 Docker Image"; vendor and authors labels removed; source label (this repo URL), project name, image name and binary name kept (a) |
| docker/alpine, alpine.debug, pr-build | b + c | authors, url, vendor labels removed; title and description say Xenobot-72 (PR build: "Xenobot-72 - PR Build"); source label, ENTRYPOINT and COPY paths, users kept (a) |
| .github/workflows/ci.yml | b | only the dev-build image title label changed ("Xenobot-72 Development Build") |
| .github/FUNDING.yml, .github/CODEOWNERS | c | git rm (decision 2) |
| .github/dependabot.yml | c | three reviewers blocks removed, rest untouched, YAML parses |
| .github/ISSUE_TEMPLATE/config.yml | c | contact_links removed, `blank_issues_enabled: false` kept |
| .github/SECURITY.md, pull_request_template.md, release.yml, bug_report.yml, feature_request.md | none | scanned: no original-developer text, no old name (release.yml "authors:" is the changelog category key) |
| docker-compose.yml | b | header comment now "Production Docker Compose for Xenobot-72"; service key `fuku`, image `ghcr.io/uasneppy/fuku_robot`, container names, credentials untouched (a) |
| debug.docker-compose.yml | b | comment said "fuku-robot"; reworded to "the bot" (names no old name and no identifier) |
| .golangci.yml | d | line 1 comment names the old project; it is human-readable text in a config file, so it takes the new name ("golangci-lint configuration for Xenobot-72"); not in the plan's file list, added to the commit explicitly |
| scripts/generate_docs/main.go | b | package comment and startup banner say Xenobot-72; generate_docs tests pass (banner not pinned); flag defaults and paths untouched |
| scripts/migrate_psql.sh | b | header comment only; bash -n ok |
| app.json | a | not edited (decision 11, binding clarification 2); byte-identical to base |
| render.yaml, Procfile, nixpacks.toml, railway.toml, sample.env | a | only identifiers (service/db names fuku-robot, fuku-db, fuku-redis, fuku_robot, bin/Fuku_Robot path, binary name); no old-name prose, no developer text; untouched |
| AGENTS.md | b | line 3 now "Xenobot-72: Telegram group-management bot. Go module ..."; no other sentence mentions FUNDING, CODEOWNERS, reviewers, OCI labels, the fork claim or the generate_docs banner (grep), so no other rule was invalidated |
| .claude/CLAUDE.md | b + c | title "**Xenobot-72**"; first sentence "Xenobot-72 is a Go Telegram group-management bot." fork clause gone; rest of paragraph unchanged |
| .planning/PROJECT.md | b + c | see "Left for the orchestrator" |

## Decisions (class d), Task 2

- .golangci.yml comment: renamed (see table).
- debug.docker-compose.yml comment: neutral wording ("the bot") instead of the new name, since the old text was a service-name-like identifier.
- PROJECT.md "Context" bullet "Fork of Fuku Robot." (line 115) also states the fork claim: retitled "Identifiers keep the earlier name." keeping the rest of the sentence (module path still the old one, AGENTS.md is authoritative). Line 126 "Fuku keeps its own pages and labels" became "Xenobot-72 keeps ...". Line 30 comment "existing Fuku Robot codebase" became "existing Xenobot-72 codebase". CLAUDE.md does not contain these bullets (the generated section holds only the Project block), so CLAUDE.md needed no further edit.

## Checks run

- YAML parse (PyYAML) of dependabot.yml, issue template config.yml, .goreleaser.yaml, ci.yml, docker-compose.yml, debug.docker-compose.yml, render.yaml, .golangci.yml: all ok. json.load of app.json: ok. bash -n scripts/migrate_psql.sh: ok.
- goreleaser is not installed (not run); .goreleaser.yaml validated by parsing as YAML and by listing its opencontainers labels (created, title, revision, version, source, description: no authors, vendor, url, no stray indentation). docker is installed: `docker compose -f docker-compose.yml config -q` exits 0 (only unset-variable warnings).
- go test -tags testtools ./scripts/generate_docs/...: ok. No Go files edited besides the comment/log string in scripts/generate_docs/main.go, so the local lint stand-in was not needed; golangci-lint cannot run here (built with go1.25, repo targets 1.26.0).
- PAT-DEV and PAT-NAME-WORD greps over the plan's verify path set (plus .golangci.yml): empty. FUNDING.yml and CODEOWNERS absent. Identifier greps (ghcr.io/uasneppy/fuku_robot, `^  fuku:`, fuku_robot in .goreleaser.yaml) pass.
- git diff against base a8e2541 for README.md README.uk.md LICENSE migrations go.mod go.sum app.json: identical.
- make check-docs: no drift. git status after commit: only .planning/PROJECT.md modified (plus untracked quick-task files).
- make test not run (Task 3).

## Left for the orchestrator

.planning/PROJECT.md is edited (title, "What This Is" first sentence, the "Fork of ..." context bullet, one comment line, one "Fuku keeps" sentence) and left UNCOMMITTED on purpose; commit it with the quick task's other planning artifacts. It is the only tracked file left modified.

## For Task 3 / owner

- Docs and README task items unchanged from the plan. Remaining old-name hits outside docs and tests are class (a) identifiers only (checked by grep).
