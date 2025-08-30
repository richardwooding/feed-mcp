// Package model provides OPML parsing functionality for the feed-mcp server.
package model

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OPMLOutline represents an outline element in OPML
type OPMLOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string        `xml:"htmlUrl,attr,omitempty"`
	Outlines []OPMLOutline `xml:"outline,omitempty"`
}

// OPMLBody represents the body section of OPML
type OPMLBody struct {
	Outlines []OPMLOutline `xml:"outline"`
}

// OPMLHead represents the head section of OPML
type OPMLHead struct {
	Title       string `xml:"title,omitempty"`
	DateCreated string `xml:"dateCreated,omitempty"`
	OwnerName   string `xml:"ownerName,omitempty"`
	OwnerEmail  string `xml:"ownerEmail,omitempty"`
}

// OPML represents an OPML document
type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    OPMLHead `xml:"head"`
	Body    OPMLBody `xml:"body"`
}

// ExtractFeedURLsFromOPML parses OPML content and extracts all feed URLs
func ExtractFeedURLsFromOPML(opmlContent []byte) ([]string, error) {
	var opml OPML
	if err := xml.Unmarshal(opmlContent, &opml); err != nil {
		return nil, NewFeedErrorWithCause(ErrorTypeParsing, "failed to parse OPML content", err).
			WithOperation("extract_feed_urls").
			WithComponent("opml_parser")
	}

	var urls []string
	extractURLsFromOutlines(opml.Body.Outlines, &urls)

	if len(urls) == 0 {
		return nil, NewFeedError(ErrorTypeConfiguration, "no feed URLs found in OPML").
			WithOperation("extract_feed_urls").
			WithComponent("opml_parser")
	}

	return urls, nil
}

// extractURLsFromOutlines recursively extracts feed URLs from OPML outlines
func extractURLsFromOutlines(outlines []OPMLOutline, urls *[]string) {
	for _, outline := range outlines {
		// If this outline has an xmlUrl, it's a feed
		if outline.XMLURL != "" {
			*urls = append(*urls, outline.XMLURL)
		}
		// Recursively check nested outlines
		if len(outline.Outlines) > 0 {
			extractURLsFromOutlines(outline.Outlines, urls)
		}
	}
}

// LoadOPMLFromFile loads and parses an OPML file from the local filesystem
func LoadOPMLFromFile(path string) ([]string, error) {
	file, err := os.Open(path) // #nosec G304 -- path is user-provided CLI argument, this is expected behavior
	if err != nil {
		return nil, NewFeedErrorWithCause(ErrorTypeSystem, fmt.Sprintf("failed to open OPML file: %s", path), err).
			WithOperation("load_opml_file").
			WithComponent("opml_loader")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Note: In a production application, this would be logged
			// For now, we silently ignore close errors to avoid overriding main errors
			_ = closeErr
		}
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, NewFeedErrorWithCause(ErrorTypeSystem, fmt.Sprintf("failed to read OPML file: %s", path), err).
			WithOperation("load_opml_file").
			WithComponent("opml_loader")
	}

	return ExtractFeedURLsFromOPML(content)
}

// LoadOPMLFromURL loads and parses an OPML file from a remote URL
func LoadOPMLFromURL(url string) ([]string, error) {
	// Use a reasonable timeout for OPML fetching
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, NewFeedErrorWithCause(ErrorTypeNetwork, fmt.Sprintf("failed to fetch OPML from URL: %s", url), err).
			WithOperation("load_opml_url").
			WithComponent("opml_loader")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Note: In a production application, this would be logged
			// For now, we silently ignore close errors to avoid overriding main errors
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, NewFeedError(ErrorTypeHTTP, fmt.Sprintf("HTTP %d when fetching OPML from: %s", resp.StatusCode, url)).
			WithOperation("load_opml_url").
			WithComponent("opml_loader").
			WithHTTP(resp.StatusCode, resp.Header)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewFeedErrorWithCause(ErrorTypeNetwork, fmt.Sprintf("failed to read OPML response from: %s", url), err).
			WithOperation("load_opml_url").
			WithComponent("opml_loader")
	}

	return ExtractFeedURLsFromOPML(content)
}

// LoadFeedURLsFromOPML loads feed URLs from either a local file or remote URL
func LoadFeedURLsFromOPML(opmlSource string) ([]string, error) {
	if opmlSource == "" {
		return nil, NewFeedError(ErrorTypeConfiguration, "OPML source cannot be empty").
			WithOperation("load_feeds_from_opml").
			WithComponent("opml_loader")
	}

	// Determine if it's a URL or file path
	if strings.HasPrefix(opmlSource, "http://") || strings.HasPrefix(opmlSource, "https://") {
		return LoadOPMLFromURL(opmlSource)
	}

	return LoadOPMLFromFile(opmlSource)
}
