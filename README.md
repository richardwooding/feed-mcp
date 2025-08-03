# feed-mcp

[![Go Coverage](https://github.com/richardwooding/feed-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/richardwooding/feed-mcp/coverage.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/richardwooding/feed-mcp)](https://goreportcard.com/report/github.com/richardwooding/feed-mcp)

MCP Server for RSS, Atom, and JSON Feeds

## Features

- Serves RSS, Atom, and JSON feeds via the MCP protocol
- Supports Docker and Podman for easy deployment
- CLI installable via `go install`
- Compatible with Claude Desktop as an MCP server
- Caching for efficient feed retrieval
- Supports multiple feeds simultaneously
- Extensible and configurable

## Architecture

The core of `feed-mcp` is a Go server that fetches, parses, and serves RSS/Atom/JSON feeds over the [MCP protocol](https://spec.modelcontextprotocol.io/specification/). The main architectural components are:

- **Command-line Interface (CLI):** Uses [kong](https://github.com/alecthomas/kong) for parsing commands and flags. The `run` command is the entry point for starting the server.
- **Feed Fetching & Parsing:** Feeds are fetched and parsed using [gofeed](https://github.com/mmcdole/gofeed). The server supports multiple feeds, which are periodically refreshed and cached.
- **Caching Layer:** Feed data is cached using [gocache](https://github.com/eko/gocache) and [ristretto](https://github.com/dgraph-io/ristretto) for efficient retrieval and reduced network usage.
- **MCP Protocol Server:** Implements the MCP protocol using the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk), allowing integration with clients like Claude Desktop.
- **Transport Options:** Supports different transports (e.g., stdio, HTTP with SSE) for communication with MCP clients.
- **Docker/Podman Support:** The server can be run in containers for easy deployment and integration.

### How it Works

1. **Startup:** The CLI parses arguments and starts the server with the specified feeds and transport.
2. **Feed Management:** The server fetches and parses the configured feeds, storing results in the cache.
3. **Serving Requests:** When an MCP client connects, the server responds to requests for feed data using the cached content, updating as needed.
4. **Extensibility:** The architecture allows for adding new transports, feed sources, or output formats with minimal changes.

For contributors:  
- The main entry point is `main.go`, which wires up the CLI and server.
- Feed logic is in the `model` and `store` packages.
- MCP protocol handling is in the `mcpserver` package.
- Tests are provided for core logic; see `*_test.go` files for examples.

## Running via docker

```sh
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run \
  https://www.reddit.com/r/golang/.rss \
  https://www.reddit.com/r/mcp/.rss
```

## Running via podman

```sh
podman run -i --rm ghcr.io/richardwooding/feed-mcp:latest run \
  https://www.reddit.com/r/golang/.rss \
  https://www.reddit.com/r/mcp/.rss
```

## Installing using Go install

You can install the CLI using:

```sh
go install github.com/richardwooding/feed-mcp@latest
```

## Add to Claude Desktop

In your Claude Desktop configuration file, add the following configuration to the `mcpServers` section:

### Docker

```json
{
  "mcpServers": {
    "feed": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://www.reddit.com/r/golang/.rss",
        "https://www.reddit.com/r/mcp/.rss"
      ]
    }
  }
}
```

### Podman

```json
{
  "mcpServers": {
    "feed": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://www.reddit.com/r/golang/.rss",
        "https://www.reddit.com/r/mcp/.rss"
      ]
    }
  }
}
```

## Dependencies

This project makes use of the following open source libraries:

- [gofeed](https://github.com/mmcdole/gofeed) — RSS/Atom feed parser
- [kong](https://github.com/alecthomas/kong) — Command-line parser
- [gocache](https://github.com/eko/gocache) — Caching library
- [ristretto](https://github.com/dgraph-io/ristretto) — High performance cache
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) — Official MCP protocol implementation

## License

This project is licensed under the [MIT License](LICENSE).
