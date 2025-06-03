# feed-mcp
## Coverage
```
<!---go-badges-coverage-->
```
## Version
```
<!---go-badges-version-->
```
## Report Card
```
<!---go-badges-report-card-->
```


MCP Server for RSS, Atom, and JSON Feeds

## Installation

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
      "command": "/path/to/binary/feed-mcp",
      "args": [
        "run",
        "https://feeds.capi24.com/v1/Search/articles/news24/TopStories/rss",
        "https://rss.dw.com/rdf/rss-en-all",
        "https://www.france24.com/en/rss"
      ]
    }
  }
}
```
