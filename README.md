# feed-mcp

[![Go Coverage](https://github.com/richardwooding/feed-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/richardwooding/feed-mcp/coverage.html)

MCP Server for RSS, Atom, and JSON Feeds

## Running via docker

```sh
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run https://www.reddit.com/r/capetown/.rss
```

## Installing using Go install

You can install the CLI using:

```sh
go install github.com/richardwooding/feed-mcp@latest
```

## Add to Claude Desktop

Locate the binary in your system, by using

```sh
which feed-mcp
```

In your Claude Desktop configuration file, add the following configuration to the `mcpServers` section:

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
        "https://www.reddit.com/r/capetown/.rss"
      ]
    }
  }
}
```
