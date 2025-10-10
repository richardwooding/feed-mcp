package model

import (
	"testing"
)

// FuzzExtractFeedURLsFromOPML tests OPML parsing with random inputs to discover
// XML parsing vulnerabilities, billion laughs attacks, and malformed XML handling
func FuzzExtractFeedURLsFromOPML(f *testing.F) {
	// Seed corpus with valid OPML examples

	// Minimal valid OPML
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline type="rss" xmlUrl="https://example.com/feed.xml" />
  </body>
</opml>`))

	// OPML with nested outlines
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline text="Category">
      <outline type="rss" xmlUrl="https://example.com/feed1.xml" />
      <outline type="rss" xmlUrl="https://example.com/feed2.xml" />
    </outline>
  </body>
</opml>`))

	// OPML with no feeds (should error)
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline text="Empty category" />
  </body>
</opml>`))

	// Empty OPML
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body></body>
</opml>`))

	// Malformed XML
	f.Add([]byte(`<opml><body><outline`))
	f.Add([]byte(`not-xml-at-all`))
	f.Add([]byte(``))

	// XML with special characters
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline type="rss" xmlUrl="https://example.com/feed?q=a&amp;b=c" />
  </body>
</opml>`))

	// XML with CDATA
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline type="rss" xmlUrl="https://example.com/feed.xml">
      <![CDATA[Some content]]>
    </outline>
  </body>
</opml>`))

	// Deeply nested structure (potential stack overflow)
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline text="1">
      <outline text="2">
        <outline text="3">
          <outline text="4">
            <outline text="5">
              <outline type="rss" xmlUrl="https://example.com/feed.xml" />
            </outline>
          </outline>
        </outline>
      </outline>
    </outline>
  </body>
</opml>`))

	// XML with DOCTYPE (potential XXE attack)
	f.Add([]byte(`<?xml version="1.0"?>
<!DOCTYPE opml [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<opml version="2.0">
  <body>
    <outline type="rss" xmlUrl="&xxe;" />
  </body>
</opml>`))

	// Large attribute values
	f.Add([]byte(`<?xml version="1.0"?>
<opml version="2.0">
  <body>
    <outline type="rss" xmlUrl="https://example.com/feed.xml" text="` +
		string(make([]byte, 10000)) + `" />
  </body>
</opml>`))

	f.Fuzz(func(t *testing.T, opmlContent []byte) {
		// The function should never panic, regardless of input
		// We're testing for robustness against malicious/malformed XML
		_, _ = ExtractFeedURLsFromOPML(opmlContent)
	})
}

// FuzzLoadFeedURLsFromOPML tests the OPML loader logic that determines
// whether to load from file or URL based on the input string
func FuzzLoadFeedURLsFromOPML(f *testing.F) {
	// Seed corpus with various input patterns
	f.Add("https://example.com/feeds.opml")
	f.Add("http://example.com/feeds.opml")
	f.Add("/path/to/feeds.opml")
	f.Add("feeds.opml")
	f.Add("")
	f.Add("file:///etc/passwd")
	f.Add("ftp://example.com/feeds.opml")

	f.Fuzz(func(t *testing.T, opmlSource string) {
		// The function should never panic
		// Note: This will attempt to read files/URLs, which may fail (expected)
		// We're only testing that it doesn't panic on unexpected input
		_, _ = LoadFeedURLsFromOPML(opmlSource)
	})
}
