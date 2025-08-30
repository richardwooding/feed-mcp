package model

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExtractFeedURLsFromOPML(t *testing.T) {
	tests := []struct {
		name     string
		opml     string
		expected []string
		wantErr  bool
	}{
		{
			name: "simple OPML with feeds",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Test Feeds</title>
	</head>
	<body>
		<outline text="Tech News" title="Tech News" xmlUrl="https://techcrunch.com/feed/" />
		<outline text="Security" title="Security" xmlUrl="https://krebsonsecurity.com/feed/" />
	</body>
</opml>`,
			expected: []string{
				"https://techcrunch.com/feed/",
				"https://krebsonsecurity.com/feed/",
			},
			wantErr: false,
		},
		{
			name: "nested OPML with categories",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Categorized Feeds</title>
	</head>
	<body>
		<outline text="Technology" title="Technology">
			<outline text="TechCrunch" xmlUrl="https://techcrunch.com/feed/" />
			<outline text="The Verge" xmlUrl="https://www.theverge.com/rss/index.xml" />
		</outline>
		<outline text="Security" title="Security">
			<outline text="Krebs" xmlUrl="https://krebsonsecurity.com/feed/" />
		</outline>
	</body>
</opml>`,
			expected: []string{
				"https://techcrunch.com/feed/",
				"https://www.theverge.com/rss/index.xml",
				"https://krebsonsecurity.com/feed/",
			},
			wantErr: false,
		},
		{
			name: "OPML with mixed content (some feeds, some folders)",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<body>
		<outline text="Direct Feed" xmlUrl="https://example.com/feed.xml" />
		<outline text="News Category">
			<outline text="BBC" xmlUrl="https://feeds.bbci.co.uk/news/rss.xml" />
			<outline text="Empty Folder">
				<!-- No feeds in this folder -->
			</outline>
		</outline>
		<outline text="Another Direct Feed" xmlUrl="https://another.example.com/rss" />
	</body>
</opml>`,
			expected: []string{
				"https://example.com/feed.xml",
				"https://feeds.bbci.co.uk/news/rss.xml",
				"https://another.example.com/rss",
			},
			wantErr: false,
		},
		{
			name: "invalid XML",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Invalid XML</title>
	<body>
		<outline text="Missing closing tag" xmlUrl="https://example.com/feed.xml" />
	</body>
</opml>`,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "OPML with no feeds",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>No Feeds</title>
	</head>
	<body>
		<outline text="Empty Category">
			<outline text="Another Empty Category" />
		</outline>
	</body>
</opml>`,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "empty OPML",
			opml: `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Empty</title>
	</head>
	<body>
	</body>
</opml>`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls, err := ExtractFeedURLsFromOPML([]byte(tt.opml))
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFeedURLsFromOPML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(urls, tt.expected) {
				t.Errorf("ExtractFeedURLsFromOPML() = %v, want %v", urls, tt.expected)
			}
		})
	}
}

func TestLoadOPMLFromFile(t *testing.T) {
	// Create a temporary OPML file
	tmpDir := t.TempDir()
	opmlFile := filepath.Join(tmpDir, "test.opml")

	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Test Feeds</title>
	</head>
	<body>
		<outline text="Feed 1" xmlUrl="https://example.com/feed1.xml" />
		<outline text="Feed 2" xmlUrl="https://example.com/feed2.xml" />
	</body>
</opml>`

	err := os.WriteFile(opmlFile, []byte(opmlContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test OPML file: %v", err)
	}

	urls, err := LoadOPMLFromFile(opmlFile)
	if err != nil {
		t.Errorf("LoadOPMLFromFile() error = %v", err)
		return
	}

	expected := []string{
		"https://example.com/feed1.xml",
		"https://example.com/feed2.xml",
	}

	if !reflect.DeepEqual(urls, expected) {
		t.Errorf("LoadOPMLFromFile() = %v, want %v", urls, expected)
	}
}

func TestLoadOPMLFromFile_NonExistent(t *testing.T) {
	_, err := LoadOPMLFromFile("/nonexistent/file.opml")
	if err == nil {
		t.Error("LoadOPMLFromFile() should return error for non-existent file")
	}

	// Check that it's the right kind of error
	if !strings.Contains(err.Error(), "failed to open OPML file") {
		t.Errorf("Expected file error, got: %v", err)
	}
}

func TestLoadOPMLFromURL(t *testing.T) {
	// Create a test server
	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Remote Feeds</title>
	</head>
	<body>
		<outline text="Remote Feed 1" xmlUrl="https://remote1.example.com/feed.xml" />
		<outline text="Remote Feed 2" xmlUrl="https://remote2.example.com/feed.xml" />
	</body>
</opml>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(opmlContent))
	}))
	defer server.Close()

	urls, err := LoadOPMLFromURL(server.URL)
	if err != nil {
		t.Errorf("LoadOPMLFromURL() error = %v", err)
		return
	}

	expected := []string{
		"https://remote1.example.com/feed.xml",
		"https://remote2.example.com/feed.xml",
	}

	if !reflect.DeepEqual(urls, expected) {
		t.Errorf("LoadOPMLFromURL() = %v, want %v", urls, expected)
	}
}

func TestLoadOPMLFromURL_HTTPError(t *testing.T) {
	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	_, err := LoadOPMLFromURL(server.URL)
	if err == nil {
		t.Error("LoadOPMLFromURL() should return error for HTTP 404")
	}

	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("Expected HTTP error, got: %v", err)
	}
}

func TestLoadOPMLFromURL_InvalidURL(t *testing.T) {
	_, err := LoadOPMLFromURL("not-a-valid-url")
	if err == nil {
		t.Error("LoadOPMLFromURL() should return error for invalid URL")
	}
}

func TestLoadFeedURLsFromOPML(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		setup    func() (string, func())
		expected []string
		wantErr  bool
	}{
		{
			name:   "empty source",
			source: "",
			setup: func() (string, func()) {
				return "", func() {}
			},
			expected: nil,
			wantErr:  true,
		},
		{
			name:   "file path",
			source: "test.opml", // Will be replaced in setup
			setup: func() (string, func()) {
				tmpDir, _ := os.MkdirTemp("", "opml_test")
				opmlFile := filepath.Join(tmpDir, "test.opml")
				opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<body>
		<outline text="Test Feed" xmlUrl="https://example.com/feed.xml" />
	</body>
</opml>`
				os.WriteFile(opmlFile, []byte(opmlContent), 0o644)
				return opmlFile, func() { os.RemoveAll(tmpDir) }
			},
			expected: []string{"https://example.com/feed.xml"},
			wantErr:  false,
		},
		{
			name:   "HTTP URL",
			source: "http://example.com/feeds.opml", // Will be replaced in setup
			setup: func() (string, func()) {
				opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<body>
		<outline text="Remote Feed" xmlUrl="https://remote.example.com/feed.xml" />
	</body>
</opml>`
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(opmlContent))
				}))
				return server.URL, func() { server.Close() }
			},
			expected: []string{"https://remote.example.com/feed.xml"},
			wantErr:  false,
		},
		// Skip HTTPS test for now since it requires TLS certificate handling
		// This would be better tested in integration tests with proper certificates
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, cleanup := tt.setup()
			defer cleanup()

			urls, err := LoadFeedURLsFromOPML(source)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFeedURLsFromOPML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(urls, tt.expected) {
				t.Errorf("LoadFeedURLsFromOPML() = %v, want %v", urls, tt.expected)
			}
		})
	}
}

// BenchmarkExtractFeedURLsFromOPML tests performance of OPML parsing
func BenchmarkExtractFeedURLsFromOPML(b *testing.B) {
	// Create a larger OPML with multiple nested categories
	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Large Feed Collection</title>
	</head>
	<body>`

	// Add multiple categories with feeds
	for i := 0; i < 10; i++ {
		opmlContent += `
		<outline text="Category ` + string(rune('A'+i)) + `">`
		for j := 0; j < 10; j++ {
			opmlContent += `
			<outline text="Feed ` + strconv.Itoa(j) + `" xmlUrl="https://example` + strconv.Itoa(j) + `.com/feed.xml" />`
		}
		opmlContent += `
		</outline>`
	}

	opmlContent += `
	</body>
</opml>`

	opmlBytes := []byte(opmlContent)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ExtractFeedURLsFromOPML(opmlBytes)
		if err != nil {
			b.Errorf("Benchmark failed: %v", err)
		}
	}
}
