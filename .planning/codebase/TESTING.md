---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# Testing Patterns

**Analysis Date:** 2026-10-04

## Test Framework

**Runner:**
- Go's built-in `testing` package (standard library)
- Test discovery: `*_test.go` files with `func Test<Name>(t *testing.T)` signatures
- Config: None required; standard Go testing conventions
- Race detection: Built-in via `-race` flag

**Assertion Libraries:**
- `testing.Errorf()` and `testing.Fatalf()` for standard assertions
- `github.com/stretchr/testify/assert` for softer assertions (don't stop test)
- `github.com/stretchr/testify/require` for hard assertions (stop test on failure)
- Example:

```go
require.NoError(t, err)                    // Hard assertion
assert.Equal(t, expected, got)             // Soft assertion
if err != nil {
    t.Fatalf("unexpected error: %v", err)  // Manual hard assertion
}
```

**Run Commands:**

```bash
make test                                  # Run all tests with coverage, race, and gates
make test-postgres-integrity               # PostgreSQL-specific tests (atomic operations)
go test -tags testtools -race -count=1 -run '^TestName$' ./alita/db/warns   # Single test
```

**Key Flags:**
- `-tags testtools`: Mandatory; excludes test-only code that would hide in production
- `-race`: Race condition detection (enabled by default in `make test`)
- `-count=1`: Disable result caching (ensures fresh runs)
- `-coverpkg=...`: Coverage calculation scoped to `alita/...` packages
- `-timeout 10m`: Test timeout (set in Makefile)
- `-json`: Machine-readable output (piped through `scripts/check_test_results`)

## Test File Organization

**Location:**
- Co-located with source files: `users.go` and `users_test.go` in same package
- Database tests: `*_test.go` files in their package (e.g., `alita/db/warns/repository_test.go`)
- Specialized tests: `*_pure_test.go` for isolated logic, `*_miniredis_test.go` for Redis mocks

**Naming:**
- Test functions: `Test<Feature><Scenario>` (e.g., `TestWarnUserCreatesMissingParentRows`)
- Benchmarks: `Benchmark<Name>` (e.g., `BenchmarkCacheLoader`)
- Setup helpers: `TestMain`, `skipIfNoDb`, `withChatSQLite`, `cleanupBackupChat`

**Structure:**

```
alita/db/<domain>/
├── repository.go        # Main implementation
├── repository_test.go   # Repository tests with TestMain
├── testmain_test.go     # Shared TestMain for the package
└── <feature>_test.go    # Specialized test files
```

## Test Structure

**TestMain Pattern:**

```go
func TestMain(m *testing.M) {
    // Setup: Initialize database/fixtures
    var dbFileName string
    if db.DB == nil {
        dbFile, err := os.CreateTemp("", "alita_test_*.db")
        // ... SQLite setup ...
    }
    
    // Run tests
    exitCode := m.Run()
    
    // Teardown: Close connections, clean files
    if dbFileName != "" {
        if rmErr := os.Remove(dbFileName); rmErr != nil {
            fmt.Printf("temp file remove failed: %v\n", rmErr)
        }
    }
    os.Exit(exitCode)
}
```

**Helper Functions:**

```go
func skipIfNoDb(t *testing.T) {
    t.Helper()
    if db.DB == nil {
        t.Fatal("test database was not initialized")
    }
}

func withChatSQLite(t *testing.T) {
    t.Helper()
    originalDB := db.DB
    testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:chats-%d?mode=memory", time.Now().UnixNano())), ...)
    db.DB = testDB
    t.Cleanup(func() {
        db.DB = originalDB
    })
}
```

**Test Patterns:**

### Table-Driven Tests

```go
func TestResolveBotAPIURL(t *testing.T) {
    tests := []struct {
        name   string
        input  string
        output string
    }{
        {name: "empty", output: gotgbot.DefaultAPIURL},
        {name: "default", input: gotgbot.DefaultAPIURL, output: gotgbot.DefaultAPIURL},
        {name: "path prefix", input: "https://bot-api.example/internal/", output: "https://bot-api.example/internal"},
    }
    
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            if got := resolveBotAPIURL(test.input); got != test.output {
                t.Fatalf("resolveBotAPIURL(%q) = %q, want %q", test.input, got, test.output)
            }
        })
    }
}
```

### Subtest Pattern

```go
func TestDemoteErrorHandling(t *testing.T) {
    t.Run("error takes precedence over nil member", func(t *testing.T) {
        userMember, err := simulateGetMemberResult(true)
        if err != nil {
            t.Logf("Error handled first: %v", err)
            return
        }
        if userMember == nil {
            t.Fatal("Should have returned on error")
        }
    })
    
    t.Run("nil error with nil member", func(t *testing.T) {
        userMember, err := simulateGetMemberResult(false)
        if err != nil {
            t.Fatalf("Unexpected error: %v", err)
        }
        if userMember == nil {
            t.Log("Nil member properly detected")
        }
    })
}
```

### Cleanup Pattern

```go
func TestWarnUser(t *testing.T) {
    skipIfNoDb(t)
    
    chatID := time.Now().UnixNano()
    userID := chatID + 1
    
    // Guarantee cleanup after test
    t.Cleanup(func() {
        _ = db.DB.Where("chat_id = ? AND user_id = ?", chatID, userID).Delete(&models.Warns{}).Error
        _ = db.DB.Where("chat_id = ?", chatID).Delete(&models.WarnSettings{}).Error
    })
    
    // Test logic
    numWarns, reasons, err := WarnUser(userID, chatID, "test reason")
    if err != nil {
        t.Fatalf("WarnUser() error = %v", err)
    }
    if numWarns != 1 {
        t.Fatalf("expected numWarns=1, got %d", numWarns)
    }
}
```

## Mocking

**Framework:** No mock libraries; use real fakes and test doubles

**Patterns:**

### BotClient Fake

```go
type moduleBotClient struct {
    calls     []mainBotCall
    responses map[string][]byte
}

func (c *moduleBotClient) RequestWithContext(ctx context.Context, _ string, method string, params map[string]any, _ *gotgbot.RequestOpts) (json.RawMessage, error) {
    c.calls = append(c.calls, mainBotCall{method: method, params: params})
    if resp, ok := c.responses[method]; ok {
        return resp, nil
    }
    return nil, gotgbot.ErrInvalidTokenFormat
}
```

### Test Database (SQLite in-memory)

```go
testDB, err := gorm.Open(
    sqlite.Open(":memory:"),
    &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
)
if err != nil {
    t.Fatalf("open SQLite: %v", err)
}
if err := testDB.AutoMigrate(&models.Chat{}); err != nil {
    t.Fatalf("AutoMigrate: %v", err)
}
```

### Miniredis for Redis

- Used in tests requiring Redis behavior (e.g., `antiraid_miniredis_test.go`)
- Provides real Redis-like responses without external service

**What to Mock:**
- External APIs: Telegram bot API (gotgbot.BotClient)
- I/O operations: File writes, network calls
- Expensive operations: Encryption, complex calculations

**What NOT to Mock:**
- Database queries: Use real SQLite test database
- Cache/Redis: Use miniredis for fidelity
- Logic within the codebase: Test actual implementations

## Fixtures and Factories

**Test Data:**

```go
buttons := models.ButtonArray{{Name: "docs", Url: "https://example.com", SameLine: true}}
require.NoError(t, db.DB.Create(&models.AdminSettings{ChatId: srcChat, AnonAdmin: true}).Error)
require.NoError(t, db.DB.Create(&models.ChatFilters{
    ChatId: srcChat, KeyWord: "hello", FilterReply: "world", MsgType: 2,
    FileID: "filter-file", NoNotif: true, Buttons: buttons,
}).Error)
```

**Location:**
- Inline in test files (no separate fixture files)
- Factory functions for complex objects (e.g., `newModuleTestBot`, `newModuleMessageContext`)

**Ensure Functions:**
- `chats.EnsureChatInDb(chatID, chatName)`: Create chat if needed
- `user.EnsureUserInDb(userID, username, name)`: Create user if needed
- These functions handle conflicts gracefully for idempotent test setup

## Coverage

**Requirements:**
- No explicit target enforced
- Generated by `make test` to `coverage.out` file
- Scope: `alita/...` packages only

**View Coverage:**

```bash
make test                           # Generates coverage.out
go tool cover -html=coverage.out   # View HTML coverage report
```

**Best Practices:**
- Test critical paths (database, state changes, error cases)
- Assert observable behavior: DB persistence, replies sent, cache invalidation
- Avoid asserting on internal implementation details

## Test Types

**Unit Tests:**
- Scope: Individual functions/methods in isolation
- Pattern: Test inputs → expected outputs
- Database: Use SQLite in-memory for data access tests
- Example: `TestCheckWarnSettings_Defaults` — tests default warn limit
- Location: Alongside source files or in dedicated `*_test.go` files

**Integration Tests:**
- Scope: Multiple components interacting
- Pattern: Real database, fake external APIs
- Example: `TestAllModulesRoundTripEveryMeaningfulField` — tests backup/restore flow
- Location: `alita/db/backup/roundtrip_test.go`

**Atomic Tests (Database Constraints):**
- Scope: Database-level guarantees (foreign keys, unique constraints, transactions)
- Run: `make test-postgres-integrity` (PostgreSQL only)
- GORM AutoMigrate: Configures models for testing; update when changing models
- Example: `TestWarnUserCreatesMissingParentRows` — verifies parent rows are auto-created
- Location: Listed in `make test-postgres-integrity` target

**E2E Tests:**
- Not used in this codebase
- Telegram integration is tested via fake BotClient

## Common Patterns

**Async Testing:**

```go
// Goroutines with recovery
go func() {
    defer error_handling.RecoverFromPanic(...)
    // Async work
}()

// Wait for completion
// (Uses WaitGroup drained by DrainUsersAsyncWrites in main)
```

**Error Testing:**

```go
func TestWarnUserReachesLimit(t *testing.T) {
    skipIfNoDb(t)
    // ... setup warns at limit ...
    numWarns, reasons, err := WarnUser(userID, chatID, "over limit")
    if err != nil {
        t.Fatalf("expected error, got nil")
    }
}

type testError struct {
    msg string
}

func (e *testError) Error() string {
    return e.msg
}

// Simulate API errors
simulateGetMemberResult := func(wantErr bool) (gotgbot.ChatMember, error) {
    if wantErr {
        return nil, &testError{msg: "API error"}
    }
    return nil, nil
}
```

**Skip Pattern:**

```go
if db.DB.Name() != "postgres" {
    t.Skip("PostgreSQL JSONB integration: run make test-postgres-integrity")
}
```

**Assertion Errors:**

```go
// Hard stop on failure (testify/require)
require.NoError(t, err)

// Continue on failure (testify/assert)
assert.Equal(t, expected, got)

// Manual hard stop
if numWarns != 1 {
    t.Fatalf("expected numWarns=1, got %d", numWarns)
}

// Manual soft stop
if numWarns != 1 {
    t.Errorf("expected numWarns=1, got %d", numWarns)
}
```

## Testing Gate

**Pre-commit:** golangci-lint and go fmt check commits

**CI:** 
- Full test run: `make test` (coverage, race, gates)
- Migration chain: Applied before tests to validate schema checksums
- Postgres integrity: `make test-postgres-integrity` on PostgreSQL instances

**Gates:**
- Dual applier agreement: `alita/db/migrations/runner.go` and `scripts/migrate_psql.sh` must be in sync
- Schema checksum validation: Prevents out-of-order migration runs
- Race detector: Catches concurrent access bugs

---

*Testing analysis: 2026-10-04*
