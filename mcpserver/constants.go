package mcpserver

// Shared string constants for the mcpserver package.
//
// These extract repeated string literals (JSON-schema keys, schema type
// values, schema documentation/format strings, tool names, and other
// recurring values) into named constants. Values are byte-for-byte identical
// to the literals they replace; changing any value would alter the server's
// JSON-schema, tool, prompt, or resource output.

// JSON/schema object keys.
const (
	keyDescription = "description"
	keyFormat      = "format"
	keyRequired    = "required"
	keyExample     = "example"
	keyTitle       = "title"
	keyUpdatedAt   = "updated_at"
	keyValues      = "values"
	keyRange       = "range"
	keyDefault     = "default"
	keyURI         = "uri"
	keyFeedID      = "feedId"
	keyFeedIDs     = "feedIds"
	keyID          = "ID"
	keyURL         = "URL"
	keyURLLower    = "url"
	keyItemIndex   = "itemIndex"
	keyTimeframe   = "timeframe"
)

// JSON-schema type values.
const (
	typeObject  = "object"
	typeString  = "string"
	typeInteger = "integer"
	typeBoolean = "boolean"
)

// Schema documentation/format value strings.
const (
	formatInteger      = "Integer"
	formatStringDoc    = "String"
	docTextString      = "Text string"
	docNonNegativeInts = "0 or positive integers"
)

// Tool names.
const (
	toolFetchLink               = "fetch_link"
	toolAllSyndicationFeeds     = "all_syndication_feeds"
	toolGetSyndicationFeedItems = "get_syndication_feed_items"
)

// Sentiment, sort, and format enum/value strings shared across resources,
// filters, and tool schemas.
const (
	sentimentPositive = "positive"
	sentimentNegative = "negative"
	sentimentNeutral  = "neutral"

	sortByDate       = "date"
	sortByRelevance  = "relevance"
	sortByPopularity = "popularity"
	valueSource      = "source"

	formatJSON     = "json"
	formatXML      = "xml"
	formatHTML     = "html"
	formatMarkdown = "markdown"
	formatCSV      = "csv"
	formatOPML     = "opml"
	formatRSS      = "rss"
	formatAtom     = "atom"
)

// Prompt-related values.
const (
	roleUser     = "user"
	timeframe24h = "24h"
	timeframe7d  = "7d"
)

// Miscellaneous shared strings.
const (
	serverName           = "RSS, Atom, and JSON Feed Server"
	fetchLinkDescription = "Fetch link URL"
	linkURLDescription   = "Link URL"
	nameAllFeeds         = "All Feeds"
)
