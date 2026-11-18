.PHONY: run tidy vendor build bump-version lint test test-rest test-shard test-shard-check coverage-merge test-postgres-integrity check-translations check-duplicates psql-migrate psql-status psql-reset generate-docs check-docs inventory docs-dev validate-db backup-db

GO_CMD = go
GORELEASER_CMD = goreleaser
GOLANGCI_LINT_CMD = golangci-lint

# PostgreSQL Migration Variables
PSQL_SCRIPT = scripts/migrate_psql.sh

run:
	$(GO_CMD) run main.go

tidy:
	$(GO_CMD) mod tidy

vendor:
	$(GO_CMD) mod vendor

build:
	$(GORELEASER_CMD) release --snapshot --skip=publish --clean --skip=sign

# Prepare version files on a PR branch; merge the reviewed PR before tagging.
bump-version:
	@if [ -z "$(TAG)" ]; then \
		echo "❌ Error: TAG is required, e.g. make bump-version TAG=v2.19.4"; \
		exit 1; \
	fi
	@bash scripts/bump_version.sh $(TAG)

lint:
	@which $(GOLANGCI_LINT_CMD) > /dev/null || (echo "golangci-lint not found, install it from https://golangci-lint.run/usage/install/" && exit 1)
	$(GOLANGCI_LINT_CMD) run

# The flags every full or sharded test run shares (race, coverage over all of fuku, no cache).
TEST_FLAGS = -tags testtools -json -race -coverpkg=$$(go list ./fuku/... | paste -sd, -) -count=1 -timeout 40m
# The package CI splits into shards (most of the suite's time), and how many.
SHARDED_PKG = ./fuku/modules
SHARDS ?= 4

test: SHELL := /bin/bash
test: .SHELLFLAGS := -o pipefail -c
test:
	$(GO_CMD) test $(TEST_FLAGS) -coverprofile=coverage.out ./... | $(GO_CMD) run ./scripts/check_test_results

# CI runs `make test` as test-rest plus test-shard SHARD=1..$(SHARDS) in parallel jobs, then
# coverage-merge. Together they run exactly the tests `make test` runs.
test-rest: SHELL := /bin/bash
test-rest: .SHELLFLAGS := -o pipefail -c
test-rest:
	$(GO_CMD) test $(TEST_FLAGS) -coverprofile=coverage-rest.out $$(go list ./... | grep -vx "$$(go list $(SHARDED_PKG))") | $(GO_CMD) run ./scripts/check_test_results

test-shard: SHELL := /bin/bash
test-shard: .SHELLFLAGS := -o pipefail -c
test-shard:
	@if [ -z "$(SHARD)" ]; then echo "SHARD is required, e.g. make test-shard SHARD=1"; exit 1; fi
	@pattern=$$(scripts/test_shard.sh $(SHARDED_PKG) $(SHARD) $(SHARDS)) || { echo "test_shard.sh failed for shard $(SHARD)"; exit 1; }; \
	if [ -z "$$pattern" ]; then echo "test_shard.sh printed no pattern for shard $(SHARD)"; exit 1; fi; \
	echo "$(GO_CMD) test $(TEST_FLAGS) -coverprofile=coverage-shard-$(SHARD).out -run <shard $(SHARD) of $(SHARDS)> $(SHARDED_PKG)"; \
	$(GO_CMD) test $(TEST_FLAGS) -coverprofile=coverage-shard-$(SHARD).out -run "$$pattern" $(SHARDED_PKG) | $(GO_CMD) run ./scripts/check_test_results

# Fails when the shard script's test list differs from what `go test -list` reports, so a test
# the script cannot see is never left out of every shard.
test-shard-check: SHELL := /bin/bash
test-shard-check: .SHELLFLAGS := -o pipefail -c
test-shard-check:
	diff <(scripts/test_shard.sh $(SHARDED_PKG) list) <($(GO_CMD) test -tags testtools -list '.*' $(SHARDED_PKG) | grep -E '^Test' | LC_ALL=C sort)

# Joins coverage-rest.out and coverage-shard-*.out into coverage.out. `go tool cover` adds up the
# counts of a block listed more than once, which `go test` already relies on for -coverpkg.
coverage-merge:
	@ls coverage-rest.out coverage-shard-*.out > /dev/null
	{ echo "mode: atomic"; for f in coverage-rest.out coverage-shard-*.out; do tail -n +2 "$$f"; done; } > coverage.out

test-postgres-integrity:
	$(GO_CMD) test -tags testtools -v -race -p 1 -count=1 -timeout 10m \
		-run '^(TestAllModulesRoundTripEveryMeaningfulField|TestImportChatDataRollsBackEarlierModules|TestImportWarnsCreatesMissingParents|TestLegacyBackupPreservesFieldsThatVersionDidNotExport|TestDeleteCaptchaAttemptByIDAtomicSingleClaim|TestCreateMutedUserUpdatesExistingSchedule|TestCreateCaptchaAttemptReplacesExistingChallenge|TestCreateCaptchaAttemptIfEnabledRejectsDisabledChat|TestCaptchaAttemptClaimsSchedulePermissionRestore|TestDeleteMutedUserIfUnchangedPreservesNewerSchedule|TestIncrementCaptchaAttemptsRejectsRefreshedChallenge|TestUpdateChannelClearsAndReassignsNormalizedUsername|TestConnectChat|TestAddAndGetFiltersList|TestAddNotePreservesExistingUntilExplicitUpdate|TestReportSettingsCRUD|TestWarnUserCreatesMissingParentRows|TestConcurrentWarnAndRemovePreserveCount|TestUpdateChat|TestStaffExclusivityTrigger|TestStartLockdownOneActivePerChat|TestBeginAlert|TestBanRunStart|TestUnqueueBanRun|TestBanRunReport|TestAlertClose|TestRecordLinkRevoked|TestAlertSync|TestAlertSyncKeepsAMarkSetDuringTheEdit|TestReminderSlots|TestRaidDetectionSettings|TestRaidDetectionCarryOver|TestCaptchaModeTurnstileConstraint|TestAdminIndexReplaceConcurrent|TestMenuAccessUpsert|TestGreetedMemberClaimConcurrent|TestRulesMediaMigrationDefaults|TestImportGreetingsOmittedBlocks|TestRequirementHolds|TestChatRequirementsSettings|TestRequirementHoldClaimConcurrent|TestMemberRequirementLookupMigrationDefaults|TestMemberRequirementAIMigrationDefaults|TestScheduleKeepsEarliest|TestClaimBatchOneWinner|TestReleaseStaleClaims|TestDeleteRules|TestApplyJoinLeftRule|TestScheduledDeletionMigrationDefaults|TestScheduleBatchKeepsEarliest|TestScheduleBatchDuplicateKey|TestCountPending|TestDropPendingMessage|TestBlacklistActionValuesOnPostgres|TestWarnModeCheckOnPostgres|TestStaffForwardBlockConstraints|TestStaffForwardBlockMigrationDefaults|TestEventMessagesUpsert|TestChatEventMessagesMigrationDefaults)$$' \
		./fuku/db/backup ./fuku/db/captcha ./fuku/db/channels ./fuku/db/connections \
		./fuku/db/filters ./fuku/db/notes ./fuku/db/reports ./fuku/db/warns ./fuku/db/chats ./fuku/db/staff \
		./fuku/db/lockdown ./fuku/db/raiddetection ./fuku/db/migrations ./fuku/db/adminindex ./fuku/db/menuaccess ./fuku/db/greetings ./fuku/db/memberreq ./fuku/db/deletion ./fuku/db/eventmsg

check-translations:
	@echo "🔍 Checking for missing translations..."
	@cd scripts/check_translations && $(GO_CMD) mod tidy && $(GO_CMD) run main.go

check-duplicates:
	@echo "🔍 Checking for duplicate code..."
	@which $(GOLANGCI_LINT_CMD) > /dev/null || (echo "golangci-lint not found, install it from https://golangci-lint.run/usage/install/" && exit 1)
	$(GOLANGCI_LINT_CMD) run --enable dupl

# PostgreSQL Migration Targets
psql-migrate:
	@echo "🚀 Applying PostgreSQL migrations..."
	@bash $(PSQL_SCRIPT)

psql-status:
	@echo "📊 PostgreSQL Migration Status"
	@if [ -z "$${PSQL_DB_HOST:-}" ] || [ -z "$${PSQL_DB_NAME:-}" ] || [ -z "$${PSQL_DB_USER:-}" ]; then \
		echo "❌ Error: Required environment variables not set"; \
		echo "   Please set: PSQL_DB_HOST, PSQL_DB_NAME, PSQL_DB_USER"; \
		exit 1; \
	fi
	@echo "🔍 Checking migration status..."
	@PGPASSWORD="$${PSQL_DB_PASSWORD:-}" PGSSLMODE="$${PSQL_DB_SSLMODE:-prefer}" psql -h "$$PSQL_DB_HOST" -p "$${PSQL_DB_PORT:-5432}" -U "$$PSQL_DB_USER" -d "$$PSQL_DB_NAME" -c \
		"SELECT version, executed_at FROM schema_migrations ORDER BY executed_at DESC;"

psql-reset:
	@if [ -z "$${PSQL_DB_HOST:-}" ] || [ -z "$${PSQL_DB_NAME:-}" ] || [ -z "$${PSQL_DB_USER:-}" ]; then \
		echo "❌ Error: Required environment variables not set"; \
		echo "   Please set: PSQL_DB_HOST, PSQL_DB_NAME, PSQL_DB_USER"; \
		exit 1; \
	fi
	@echo "🔥 WARNING: This will DROP ALL TABLES in the database!"
	@echo "   Database: $$PSQL_DB_NAME on $$PSQL_DB_HOST"
	@echo "   Type 'yes' to confirm: " && read confirm && [ "$$confirm" = "yes" ] || (echo "Cancelled" && exit 1)
	@echo "💣 Resetting database..."
	@PGPASSWORD="$${PSQL_DB_PASSWORD:-}" PGSSLMODE="$${PSQL_DB_SSLMODE:-prefer}" psql -h "$$PSQL_DB_HOST" -p "$${PSQL_DB_PORT:-5432}" -U "$$PSQL_DB_USER" -d "$$PSQL_DB_NAME" -c \
		"DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	@echo "✅ Database reset complete"

# Documentation Targets
generate-docs:
	@echo "📚 Generating documentation..."
	@cd scripts/generate_docs && $(GO_CMD) run .
	@echo "✅ Documentation generated in docs/src/content/docs/"

check-docs:
	@echo "🔍 Checking docs generation for drift..."
	@set -e; \
	TMP=$$(mktemp -d /tmp/fuku-docs-check.XXXXXX); \
	trap 'rm -rf "$$TMP"' EXIT; \
	ROOT=$$(pwd); \
	cp -R docs/src/content/docs/. "$$TMP"/; \
	echo "  Generating docs to temp directory..."; \
	cd scripts/generate_docs && $(GO_CMD) run . -output "$$TMP"; \
	cd "$$ROOT"; \
	echo "  Comparing generated docs against current docs..."; \
	if diff -rq "$$TMP" docs/src/content/docs/ > /dev/null 2>&1; then \
		echo "✅ No drift detected — generated docs match current docs."; \
	else \
		echo "❌ Drift detected! Generated docs differ from current docs:"; \
		diff -r "$$TMP" docs/src/content/docs/ || true; \
		echo ""; \
		echo "Run 'make generate-docs' to sync."; \
		exit 1; \
	fi

inventory:
	@echo "Generating canonical command inventory..."
	@cd scripts/generate_docs && $(GO_CMD) run . -inventory
	@echo "Inventory written to .planning/INVENTORY.json and .planning/INVENTORY.md"

docs-dev:
	@echo "🚀 Starting Blume dev server..."
	@cd docs && bun run dev

# Database validation and backup
validate-db:
	@echo "🔍 Validating database for orphaned records..."
	@go run scripts/validate_orphaned_data.go

backup-db:
	@echo "💾 Creating database backup..."
	@./scripts/backup_database.sh
