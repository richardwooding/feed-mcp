# feed-mcp

[![Go Coverage](https://github.com/richardwooding/feed-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/richardwooding/feed-mcp/coverage.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/richardwooding/feed-mcp)](https://goreportcard.com/report/github.com/richardwooding/feed-mcp)

MCP Server for RSS, Atom, and JSON Feeds

## Libraries Used

This project makes use of the following open source libraries:

- [github.com/mmcdole/gofeed](https://github.com/mmcdole/gofeed) — RSS/Atom feed parser
- [github.com/alecthomas/kong](https://github.com/alecthomas/kong) — Command-line parser
- [github.com/eko/gocache](https://github.com/eko/gocache) — Caching library
- [github.com/dgraph-io/ristretto](https://github.com/dgraph-io/ristretto) — High performance cache
- [github.com/mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) — MCP protocol implementation

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
