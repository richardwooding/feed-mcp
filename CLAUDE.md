# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

An MCP server that fetches RSS/Atom/JSON feeds and serves them to AI assistants. It bridges web feeds and AI tools, exposing real-time syndicated content over the MCP protocol.

## Development Commands

This project uses a `Makefile` for all dev tasks — run `make help` for the full list.

```bash
make dev-setup     # Set up development environment
make dev           # Format, fix, and test (main dev loop)
make build         # Build all packages
make check         # Format, vet, lint, test
make test          # All tests (unit + BDD); also: test-race, test-coverage, test-coverage-html
make lint          # golangci-lint v2; also: lint-fix, fmt, vet, fix
make run            # Run with example tech feeds; also: run-security, run-reddit
```

Direct Go equivalents when needed:

```bash
go build ./...
go test ./...                                   # unit + BDD (model/features/*.feature via Godog)
go test -run TestName ./package                 # single test
go run main.go run https://techcrunch.com/feed/ # run the server locally
```

Docker images are built in CI via `ko` + `goreleaser` (no local Docker targets). Use `ghcr.io/richardwooding/feed-mcp:latest`.

## Architecture

CLI (`main.go`, Kong) → store init → MCP server → transport (stdio or Streamable HTTP).

- **`model/`** — domain types (`Feed`, `Item`, `Author`), transport enums, `FromGoFeed()` adapter, URL validation (`SanitizeFeedURLs`).
- **`store/`** — `Store` manages concurrent feed fetching, caching (gocache + ristretto), per-host rate limiting, circuit breakers, retries, and connection pooling. Implements `AllFeedsGetter` and `FeedAndItemsGetter`.
- **`mcpserver/`** — MCP protocol server (official Go SDK); tools, resources, prompts; session management.
- **`cmd/`** — `RunCmd` implements the `run` command: transport selection, server init, graceful shutdown.

Patterns: factory constructors (`NewStore`, `NewServer`), small segregated interfaces, adapter (`FromGoFeed`), early-return error handling with custom error types (e.g. `ErrInvalidTransport`), errors as the last return value. See **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** for the full breakdown.

## MCP Surface

Core tools: `all_syndication_feeds`, `get_syndication_feed_items` (paginated), `fetch_link`.
With `--allow-runtime-feeds`: `add_feed`, `remove_feed`, `list_managed_feeds`.
Resources: `feeds://all`, `feeds://feed/{id}`, `feeds://feed/{id}/items` (supports `since`/`until`/`limit`/`offset`/`category`/`author`/`search` filters), `feeds://feed/{id}/meta`.

`get_syndication_feed_items` uses **conservative defaults** to avoid blowing conversation context: metadata only (no content/images), `limit` 10 (max 20). Set `includeContent`/`includeImages`/`embedImages` only when needed, and keep `limit` low when you do. Full reference: README "How Claude Reads Feeds" and **[docs/RESOURCE_API.md](docs/RESOURCE_API.md)**.

## Resilience & Security (defaults; tune via CLI flags / `store.Config`)

- **Per-host rate limiting** — 2 req/s, burst 5, one limiter per hostname.
- **Circuit breaker** (sony/gobreaker) — enabled; opens after 3 consecutive failures, 30s timeout. Disable with `CircuitBreakerEnabled: &false`.
- **Retry** — exponential backoff + jitter, 3 attempts; retries 5xx/DNS/timeout, not 4xx.
- **Connection pooling** — tuned `http.Transport` (100 idle conns, 10/host).
- **URL security** — SSRF protection via `ssrfguard`: HTTP(S) only, private IPs blocked by default (`--allow-private-ips` to override). Enforced both up-front (`model.ValidateFeedURL`) and at dial time (the store's transport `Control` hook, which defeats DNS rebinding).
- **Graceful shutdown** — SIGINT/SIGTERM, context propagation, `--shutdown-timeout` (default 30s).

Full configuration, CLI flags, and metrics: **[docs/ADVANCED.md](docs/ADVANCED.md)** (Performance Tuning, Security) and **[docs/ENHANCED_ERRORS.md](docs/ENHANCED_ERRORS.md)**.

> **Rate-limit history (#114):** the limiter was once global, and every feed was eagerly pre-fetched at startup — a single 2 req/s bucket meant MCP `initialize` timed out (~80s for 164 feeds in a large OPML). The fix made fetches lazy (cache populated on demand) and split the limiter per host.

## Caching

In-memory only (resets on restart), keyed by feed URL, default expiration **1 hour** (`store.Config.ExpireAfter`).

## Conventions & Workflow

- **Branches + PRs always.** Branch protection requires PRs and passing tests — create a branch for any issue and open a PR when done. Never commit to `main`.
- **Use Context7, godoc, or GitHub** to get up-to-date library information; don't rely on memory for API details.
- **golangci-lint v2 only** — v2 is the installed version; this is not a config-file problem.
- Prefer a **non-cryptographic hash** (`hash/fnv`) where a hash is needed.
- Add docstring comments wherever needed.
- Fail fast with clear error messages, validate inputs at boundaries, return errors (don't panic), and keep serving other feeds if one fails.

## Documentation Map

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — architecture, design patterns, dev guide, testing, dependencies
- **[docs/ADVANCED.md](docs/ADVANCED.md)** — dynamic feeds, MCP resources, prompts, OPML, performance & security tuning
- **[docs/RESOURCE_API.md](docs/RESOURCE_API.md)** — MCP Resources API reference and URI filtering
- **[docs/ENHANCED_ERRORS.md](docs/ENHANCED_ERRORS.md)** — error context, debug logging (`DEBUG`, log levels, JSON logs)
- **[docs/PERFORMANCE.md](docs/PERFORMANCE.md)** — benchmark results
- **[README.md](README.md)** — user-facing setup, transports, troubleshooting
