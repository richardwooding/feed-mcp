# feed-mcp

[![Go Coverage](https://github.com/richardwooding/feed-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/richardwooding/feed-mcp/coverage.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/richardwooding/feed-mcp)](https://goreportcard.com/report/github.com/richardwooding/feed-mcp)
[![MCP Badge](https://lobehub.com/badge/mcp/richardwooding-feed-mcp)](https://lobehub.com/mcp/richardwooding-feed-mcp)

**Bring RSS feeds to Claude Desktop** — Read news, blogs, and updates directly in your AI conversations.

## What is feed-mcp?

feed-mcp is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server that lets Claude Desktop read RSS, Atom, and JSON feeds. Think of it as a bridge that connects your favorite websites' RSS feeds to Claude, so you can ask questions about the latest articles, get summaries, and stay updated—all from within your Claude chat.

### Why use it?

- 📰 **Stay informed** — Read the latest news and blog posts without leaving Claude
- 🎯 **Get summaries** — Ask Claude to summarize multiple articles across different feeds
- 🔍 **Deep dive** — Research topics by querying specific feeds or articles
- ⚡ **Save time** — No need to open multiple websites to stay current

## Quick Start

### Step 1: Add to Claude Desktop

Open your Claude Desktop configuration file and add feed-mcp with your favorite feeds:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "feed-mcp": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://techcrunch.com/feed/",
        "https://www.theverge.com/rss/index.xml"
      ]
    }
  }
}
```

### Step 2: Restart Claude Desktop

Restart Claude Desktop to load the new configuration.

### Step 3: Start chatting!

Try asking Claude:
- "What are the latest tech news headlines?"
- "Summarize the top 5 articles from my feeds"
- "Are there any articles about AI today?"

## Popular Feed Collections

### Technology News
```json
{
  "mcpServers": {
    "tech-news": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://techcrunch.com/feed/",
        "https://www.theverge.com/rss/index.xml",
        "https://www.wired.com/feed/rss",
        "https://feeds.arstechnica.com/arstechnica/index"
      ]
    }
  }
}
```

### Security & Privacy
```json
{
  "mcpServers": {
    "security-news": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://krebsonsecurity.com/feed/",
        "https://www.schneier.com/blog/atom.xml",
        "https://www.bleepingcomputer.com/feed/"
      ]
    }
  }
}
```

### Web Development
```json
{
  "mcpServers": {
    "webdev-news": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://css-tricks.com/feed/",
        "https://www.smashingmagazine.com/feed/",
        "https://hacks.mozilla.org/feed/"
      ]
    }
  }
}
```

### Podcasts
```json
{
  "mcpServers": {
    "podcasts": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://feeds.npr.org/510282/podcast.xml",
        "https://feeds.npr.org/381444908/podcast.xml"
      ]
    }
  }
}
```

**Try asking Claude:**
- "What are the latest podcast episodes?"
- "Summarize the most recent episode from NPR Politics"
- "Are there any episodes about climate change this week?"

## Using Your RSS Reader Feeds

Already have feeds in Feedly, Inoreader, or another RSS reader? Export them as OPML and use with feed-mcp:

```json
{
  "mcpServers": {
    "my-feeds": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/feeds.opml:/feeds.opml:ro",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run", "--opml", "/feeds.opml"
      ]
    }
  }
}
```

**How to export OPML:**
- **Feedly**: Settings → OPML → Export
- **Inoreader**: Preferences → Folders and Tags → Export OPML
- **NewsBlur**: Account → Import/Export → Export Stories
- **The Old Reader**: Settings → Import/Export → Export

## How Claude Reads Feeds

When you ask Claude about your feeds, here's what happens:

1. **Browse first** — Claude gets a list of article titles and metadata
2. **Read selectively** — Claude only fetches full content for articles you ask about
3. **Smart caching** — Articles are cached to avoid re-fetching

This two-pass approach keeps responses fast and prevents overwhelming your conversation.

### Example Usage

**You**: "What's new in tech today?"

Claude will:
1. Browse your tech feed titles
2. Summarize the latest headlines
3. Ask if you want details on specific articles

**You**: "Tell me more about the first article"

Claude will:
1. Fetch the full content of that article
2. Provide a detailed summary or answer your questions

## Features

- 🌐 **Multiple formats** — RSS, Atom, and JSON feeds
- 📱 **Import from readers** — OPML support for easy migration
- 💾 **Smart caching** — Efficient feed retrieval with automatic updates
- ⚡ **Fast & reliable** — Built-in rate limiting and error handling
- 🔒 **Secure** — URL validation and private IP blocking
- 🐳 **Easy deployment** — Docker and Podman support

## Advanced Features

For power users, feed-mcp includes:

- **Dynamic feed management** — Add/remove feeds at runtime
- **MCP Resources** — Advanced filtering and real-time subscriptions
- **Intelligent prompts** — Analyze trends, monitor keywords, generate reports
- **Circuit breakers** — Automatic handling of failing feeds
- **Custom configuration** — Rate limiting, retries, connection pooling

See [docs/ADVANCED.md](docs/ADVANCED.md) for details.

## Alternative Installation Methods

### MCP Bundle (Claude Desktop, one-click)

Every [release](https://github.com/richardwooding/feed-mcp/releases) attaches **MCP Bundles**
(`.mcpb`) — one per platform. Download the bundle matching your OS and architecture (e.g.
`feed-mcp_<version>_darwin_arm64.mcpb`) and open it with Claude Desktop to install — no Docker
or Go toolchain required. The bundle's settings screen lets you set feed URLs, the per-feed
request timeout, and the cache expiration. Feeds are optional at install time: runtime feed
management is enabled, so you can also add feeds later with the `add_feed` tool.

### Homebrew

```bash
brew install richardwooding/tap/feed-mcp
```

Then configure Claude Desktop:

```json
{
  "mcpServers": {
    "feed-mcp": {
      "command": "feed-mcp",
      "args": ["run", "https://techcrunch.com/feed/"]
    }
  }
}
```

### Go Install

If you have Go installed:

```bash
go install github.com/richardwooding/feed-mcp@latest
feed-mcp run https://techcrunch.com/feed/
```

Then configure Claude Desktop:

```json
{
  "mcpServers": {
    "feed-mcp": {
      "command": "feed-mcp",
      "args": ["run", "https://techcrunch.com/feed/"]
    }
  }
}
```

### Podman

Prefer Podman over Docker? Just replace `docker` with `podman`:

```json
{
  "mcpServers": {
    "feed-mcp": {
      "command": "podman",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://techcrunch.com/feed/"
      ]
    }
  }
}
```

## Transport Options

feed-mcp supports multiple transport protocols for different deployment scenarios:

| Transport | Description | Use Case |
|-----------|-------------|----------|
| `stdio` | Standard input/output (default) | Claude Desktop, local CLI |
| `streamable-http` | HTTP server with Streamable HTTP protocol | Web deployments, remote access |
| `http-with-sse` | Deprecated, maps to `streamable-http` | Backwards compatibility only |

### stdio (Default)

Standard input/output transport for local usage. This is the default and what Claude Desktop uses:

```bash
feed-mcp run https://techcrunch.com/feed/
```

### Streamable HTTP

HTTP-based transport for web deployments and remote access:

```bash
# Start HTTP server on default port 8080
feed-mcp run --transport=streamable-http https://techcrunch.com/feed/

# Custom port
feed-mcp run --transport=streamable-http --http-port=3000 https://techcrunch.com/feed/

# Stateless mode (for load-balanced deployments)
feed-mcp run --transport=streamable-http --http-stateless https://techcrunch.com/feed/
```

**Docker with HTTP transport:**

```bash
docker run -p 8080:8080 ghcr.io/richardwooding/feed-mcp:latest \
  run --transport=streamable-http https://techcrunch.com/feed/
```

**HTTP transport options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--http-port` | `8080` | Port for HTTP server (also reads `PORT` env var) |
| `--http-stateless` | `false` | Run in stateless mode (no session tracking) |
| `--http-session-timeout` | `30m` | Timeout for idle HTTP sessions |

## Troubleshooting

### "Claude hit the maximum length for this conversation"

If you see this error:
- You're fetching too many large articles at once
- Try asking Claude to browse titles first, then read specific articles
- The server automatically limits content to prevent this

### Feed not updating

Feeds are cached for 10 minutes by default. If you need fresh data:
- Wait a few minutes and try again
- Restart Claude Desktop to clear the cache

### Private/localhost feeds

By default, localhost and private IP feeds are blocked for security. To enable:

```json
{
  "mcpServers": {
    "feed-mcp": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run", "--allow-private-ips",
        "http://localhost:8080/feed.xml"
      ]
    }
  }
}
```

## Documentation

- **[ADVANCED.md](docs/ADVANCED.md)** — Dynamic feed management, MCP Resources, intelligent prompts
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** — Technical details, architecture, development guide
- **[CLAUDE.md](CLAUDE.md)** — Instructions for Claude Code when working with this codebase

## Releasing

Pushing a `vX.Y.Z` tag triggers the release workflow, which uses GoReleaser to:

- build binaries for linux/darwin/windows (amd64 + arm64),
- pack each binary into an **MCP Bundle** (`.mcpb`) and attach all six to the release,
- build and push a multi-arch OCI image to GHCR with ko,
- publish a Homebrew cask to [richardwooding/homebrew-tap](https://github.com/richardwooding/homebrew-tap).

The Homebrew step needs a `HOMEBREW_TAP_GITHUB_TOKEN` repo secret with write access to the tap.

### MCP Bundles

`tools/mcpb` is a small, dependency-free (Go stdlib) packer that zips the server binary and a
generated `manifest.json` into a `.mcpb`. GoReleaser invokes it per build target, but you can
build a bundle for your current platform locally (no Node required):

```bash
go build -o feed-mcp .
go run ./tools/mcpb pack -version dev   # writes dist/feed-mcp_dev_<os>_<arch>.mcpb
```

A full local dry run of the release pipeline (skips publishing):

```bash
goreleaser release --snapshot --clean
```

## Contributing

Contributions are welcome! See the [architecture docs](docs/ARCHITECTURE.md) for technical details and development guidelines.

## License

MIT License — See [LICENSE](LICENSE) for details.
