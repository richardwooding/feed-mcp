# Copilot Instructions for MCP Feed Server
## Codebase Structure & Architecture

- **Entry Point**: `main.go` sets up the CLI and launches the MCP server.
- **CLI**: Uses the [`kong`](https://github.com/alecthomas/kong) library for command parsing (`Run` command is the main entry).
- **Feed Logic**: Handled in `model` (data structures, conversion) and `store` (fetching, caching, error handling) packages.
- **Caching**: Uses [`gocache`](https://github.com/eko/gocache) and [`ristretto`](https://github.com/dgraph-io/ristretto) for in-memory feed caching.
- **Feed Parsing**: [`gofeed`](https://github.com/mmcdole/gofeed`) parses RSS/Atom/JSON feeds.
- **Protocol Layer**: `mcpserver` handles the MCP protocol with [`mcp-go`](https://github.com/mark3labs/mcp-go).
- **Tests**: All core logic is tested, with test files named `*_test.go` (see `main_test.go`, `store/store_test.go`, etc).

## Go Conventions & Patterns

- **Package Layout**
    - `model`: Data structures (`Feed`, `Globals`, transports) and adapters (e.g., `FromGoFeed()`).
    - `store`: Feed fetching, caching, management (`Store` type, `NewStore` constructor, error handling).
- **Error Handling**
    - Returns `error` as the last return in all constructors and core methods.
    - Uses Go idioms: if error, return early (`if err != nil { return ... }`).
    - Custom errors for validation (e.g., `ErrInvalidTransport`).
- **Concurrency**
    - Uses goroutines and `sync.WaitGroup` for parallel feed fetching at startup.
- **Config Struct Pattern**
    - `store.NewStore(Config)` accepts a config struct, validates, and applies sensible defaults (`Timeout`, `ExpireAfter`).
- **Test Patterns**
    - Use Go’s `testing` package.
    - Test helpers (e.g., `mockFeedServer`) for HTTP mocking.
    - `t.Fatalf`/`t.Errorf` for error assertions and value checks.

## Design Decisions

- **Extensibility**: Adding new transports, feed sources, or output formats requires minimal changes—extend the relevant package.
- **Caching Layer**: Abstracted for efficiency and to support pluggable backends.
- **Decoupling**: Strong separation between CLI, business logic, and protocol/server code.
- **Strong Typing**: All major entities (feeds, transports, config) are typed for clarity and type safety.

## Notable Patterns

- **Factory Functions**: E.g., `NewStore()` for creating configured stores.
- **Adapters**: E.g., `FromGoFeed()` converts third-party types into internal models.
- **Constants & Enums**: Transports use `iota`-based enums for type safety.