---
last_mapped_commit: 4d3518f822a0014f925b59b6748acaee47241152
last_mapped_at: 2026-10-04
---
# Coding Conventions

**Analysis Date:** 2026-10-04

## Naming Patterns

**Files:**
- `*_test.go`: Test files (conventional Go test naming)
- `testmain_test.go`: Package initialization for tests with database setup
- `*_pure_test.go`: Pure/isolated logic tests (no database dependencies)
- `*.go`: Standard package files using lowercase with underscores

**Functions:**
- **Exported (public):** PascalCase (e.g., `WarnUser`, `GetWarnSetting`, `EnsureChatInDb`)
- **Unexported (private):** camelCase (e.g., `checkWarnSettings`, `checkWarns`)
- **Receiver methods:** Use value receivers on `moduleStruct`; follow `(m moduleStruct) methodName(...)` pattern
- **Test functions:** `Test<FunctionName>` with descriptive subtypes (e.g., `TestWarnUserCreatesMissingParentRows`)
- **Helper functions:** Private with clear purpose prefix (e.g., `skipIfNoDb`, `withChatSQLite`, `cleanupBackupChat`)

**Variables:**
- **Private:** camelCase (e.g., `numWarns`, `chatID`, `userID`)
- **Exported:** PascalCase (e.g., `ChatId`, `UserId`, `WarnLimit`)
- **Constants:** PascalCase or ALL_CAPS depending on usage context
- **Loop indices:** Single letters (e.g., `i`, `j`) in simple loops; meaningful names in range loops

**Types:**
- **Structs:** PascalCase (e.g., `WarnSettings`, `Warns`, `User`, `Chat`)
- **Interfaces:** PascalCase (e.g., `Repository`, `Handler`)
- **Receiver variable names:** Short abbreviations (e.g., `m` for module, `c` for context)

## Code Style

**Formatting:**
- Tool: `gofmt` (configured in pre-commit hooks)
- Automatic on commit via pre-commit hook: `go fmt`
- Max line length: Not explicitly enforced; follow reasonable Go conventions (~120 chars)
- Indentation: Tabs (Go standard)

**Linting:**
- Tool: `golangci-lint` v2 (configured in `.golangci.yml`)
- Run: `make lint` or via pre-commit hooks
- Enabled linters:
  - `godox`: Detects TODO, FIXME, XXX keywords
  - `dupl`: Detects duplicate code fragments (threshold: 100 lines)
  - `gocyclo`: Cyclomatic complexity (min-complexity: 20; CI reports > 15 for visibility)
- Exclusions: Test files, migrations, and generated docs exclude gocyclo/dupl checks
- Policy: `new: true` — only fails on new issues to allow gradual improvement

**Import Organization:**

```go
import (
    // Standard library
    "context"
    "fmt"
    "strings"
    
    // Third-party
    "github.com/PaulSonOfLars/gotgbot/v2"
    "gorm.io/gorm"
    log "github.com/sirupsen/logrus"
    
    // Internal
    "github.com/divkix/Alita_Robot/alita/config"
    "github.com/divkix/Alita_Robot/alita/db"
)
```

**Path Aliases:**
- No path aliases used in this codebase; full import paths are standard practice

## Error Handling

**Custom Error Wrapper:**
- Location: `alita/utils/errors/errors.go`
- Use `errors.Wrap(err, message)` to wrap errors with context (file, line, function)
- Use `errors.Wrapf(err, format, args...)` for formatted error messages
- Pattern: Return `nil` if err is nil; always wrap before returning
- Example:

```go
if err != nil {
    return errors.Wrap(err, "failed to fetch user")
}
```

**Error Propagation:**
- Never discard DB errors on state-changing paths (critical rule from AGENTS.md)
- Use `gorm.ErrRecordNotFound` to check for missing records
- Log errors with context: `log.Errorf("[Category][Function]: %v", err)`
- Return sentinel errors (e.g., `ext.EndGroups`, `ext.ContinueGroups`) from handlers

**Error Types:**
- Built-in `error` interface for most cases
- Sentinel errors (e.g., `gotgbot.TelegramError`, `gorm.ErrRecordNotFound`)
- Custom errors implement `Error()` string method

## Logging

**Framework:** logrus (`github.com/sirupsen/logrus`)

**Patterns:**
- Categorized messages: `log.Infof("[Category][Subcategory]: message")`
- Categories observed: `[Database]`, `[ActionLog]`, `[Shutdown]`, `[Media]`
- Levels used:
  - `log.Debugf()`: Detailed diagnostics
  - `log.Infof()`: General informational messages
  - `log.Warnf()`: Warning conditions
  - `log.Errorf()`: Error conditions
- Never log errors from control-flow mechanisms (e.g., `ext.EndGroups`) as errors
- Register secrets with `logredact.RegisterSecret(secret)` (≥6 chars) to prevent logging
- Example:

```go
log.Infof("[Shutdown] Starting graceful shutdown...")
log.Errorf("[Database][checkWarns]: %d - %v", chatID, err)
log.Debugf("[Database][checkWarnSettings]: Chat %d doesn't exist", chatID)
```

## Comments

**When to Comment:**
- Comment exported functions/types: Required GoDoc style
- Comment non-obvious logic: Complex algorithms, workarounds, important invariants
- Skip obvious comments: Don't comment `count++` or `if x != nil`
- Mark frozen content: `<!-- MANUALLY MAINTAINED: do not regenerate -->` in docs

**GoDoc Style:**
- Start with the function/type name: "Comment describes what the function does"
- Single-line comments above exported identifiers
- Multi-line comments for complex behavior
- Example:

```go
// WarnUser increments warn count and returns the warning data for a user in a chat.
func WarnUser(userID, chatID int64, reason string) (int, []string, error) {
    // ...
}

// A locale holding two keys that differ only in case gets no index: lowering the tables
// would merge the pair and make the exact-case key unreachable, while lookupSegment
// matches exact case first.
func buildLookupIndex(data map[string]any) *lookupIndex {
    // ...
}
```

## Function Design

**Size:** Functions should be reasonably sized; gocyclo limit of 20 enforces moderate complexity

**Parameters:**
- Use context.Context as first parameter for database-accessing functions
- Combine related parameters into structs for clarity (e.g., `*helpers.CommandContext`)
- Avoid more than 4-5 parameters; use struct receivers for methods
- Example:

```go
func checkWarnSettingsContext(ctx context.Context, chatID int64) *models.WarnSettings
func (m moduleStruct) adminlist(c *helpers.CommandContext) error
```

**Return Values:**
- Use explicit types (not bare returns after named parameters)
- Error as last return value in multi-return functions
- Nil checks: `if errors.Is(err, gorm.ErrRecordNotFound)` not equality
- Return sentinel values from handlers: `ext.EndGroups`, `ext.ContinueGroups`, `nil`
- Example:

```go
numWarns, reasons, err := WarnUser(userID, chatID, reason)
if err != nil {
    return err
}
```

## Module Design

**Exports:**
- Public functions use PascalCase
- Unexported helpers use camelCase
- Avoid public global variables; use functions to access singletons
- Example: `db.DB` is public singleton; `GetWarnSetting` is exported function

**Barrel Files:**
- No barrel/index files pattern used; each package imports specifically needed files

**Package Structure:**
- Domain-focused packages: `alita/db/<domain>/` for data access
- Module packages: `alita/modules/<feature>.go` for feature implementations
- Utility packages: `alita/utils/<concern>/` for cross-cutting functionality
- Config packages: `alita/config/` for environment and startup configuration

## Receiver Functions

- **Always use value receivers** on `moduleStruct` (not pointer receivers)
- Pattern:

```go
var adminModule = moduleStruct{moduleName: "Admin"}

func (m moduleStruct) adminlist(c *helpers.CommandContext) error {
    // ...
    return ext.EndGroups
}
```

## Database Conventions

- **Repository pattern:** Read via `cache.GetFromCacheOrLoad`, write with `cache.DeleteCache` to invalidate
- **Models:** Define `TableName()` method returning the migration table name
- **Queries:** Use GORM's Where/First/Create patterns
- **Caching:** Two-layer: in-process (`CACHE_LOCAL_TTL` default 10s) + Redis
- **Migrations:** SQL files in `migrations/` are schema source of truth; never edit applied files

---

*Convention analysis: 2026-10-04*
