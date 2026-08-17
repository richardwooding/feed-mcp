package model

import (
	"io"
	"net/http"
	"os"
	"strings"
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

// fuzzRoundTripper serves a canned OPML response for any request, so
// fuzzing the URL branch never leaves the process.
type fuzzRoundTripper struct{}

func (fuzzRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `<?xml version="1.0"?><opml version="2.0"><body><outline type="rss" xmlUrl="https://example.com/feed.xml"/></body></opml>`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// FuzzLoadFeedURLsFromOPML tests the OPML loader logic that determines
// whether to load from file or URL based on the input string.
//
// Real I/O is stubbed out: fuzzer-generated strings routinely look like
// URLs ("http://8.AC" — the input behind the 2026-08-09 CI failure) or
// like files the runner can actually open (/dev/zero), so hitting the
// real network/filesystem makes runs hang, OOM, or fail on DNS luck.
// The seams keep the routing + error-wrapping logic under test while
// every probe stays in-memory and deterministic.
func FuzzLoadFeedURLsFromOPML(f *testing.F) {
	prevClient, prevOpen := opmlHTTPClient, opmlOpenFile
	opmlHTTPClient = &http.Client{Transport: fuzzRoundTripper{}}
	opmlOpenFile = func(path string) (io.ReadCloser, error) {
		if strings.HasSuffix(path, ".opml") {
			return io.NopCloser(strings.NewReader(`<opml version="2.0"><body><outline xmlUrl="https://example.com/a.xml"/></body></opml>`)), nil
		}
		return nil, os.ErrNotExist
	}
	f.Cleanup(func() {
		opmlHTTPClient, opmlOpenFile = prevClient, prevOpen
	})

	// Seed corpus with various input patterns
	f.Add("/path/to/feeds.opml")
	f.Add("feeds.opml")
	f.Add("")
	f.Add("file:///etc/passwd")
	f.Add("ftp://example.com/feeds.opml")
	f.Add("http://8.AC") // regression: URL-shaped input must not touch the network

	f.Fuzz(func(t *testing.T, opmlSource string) {
		// The function should never panic on unexpected input.
		_, _ = LoadFeedURLsFromOPML(opmlSource)
	})
}
