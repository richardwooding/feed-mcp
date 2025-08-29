package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/richardwooding/feed-mcp/model"
)

//nolint:gocognit // Test function complexity is acceptable for thorough validation
func TestRunCmd_URLValidation(t *testing.T) {
	tests := []struct {
		name            string
		errorContains   string
		feeds           []string
		allowPrivateIPs bool
		expectError     bool
	}{
		{
			name:            "valid URLs",
			feeds:           []string{"https://example.com/feed.xml", "http://feeds.example.org/rss"},
			allowPrivateIPs: false,
			expectError:     false,
		},
		{
			name:            "invalid scheme rejected",
			feeds:           []string{"file:///etc/passwd"},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "unsupported URL scheme",
		},
		{
			name:            "localhost blocked by default",
			feeds:           []string{"http://localhost/feed"},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "private IP addresses and localhost are blocked",
		},
		{
			name:            "private IP blocked by default",
			feeds:           []string{"http://192.168.1.1/feed"},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "private IP addresses and localhost are blocked",
		},
		{
			name:            "localhost allowed with flag",
			feeds:           []string{"http://localhost/feed"},
			allowPrivateIPs: true,
			expectError:     false,
		},
		{
			name:            "private IP allowed with flag",
			feeds:           []string{"http://192.168.1.1/feed"},
			allowPrivateIPs: true,
			expectError:     false,
		},
		{
			name:            "mixed valid and invalid URLs",
			feeds:           []string{"https://example.com/feed", "ftp://evil.com/file", "http://localhost/feed"},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "invalid feed URLs",
		},
		{
			name:            "malformed URL rejected",
			feeds:           []string{"not-a-url-at-all"},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "unsupported URL scheme",
		},
		{
			name:            "empty URL rejected",
			feeds:           []string{""},
			allowPrivateIPs: false,
			expectError:     true,
			errorContains:   "URL cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &RunCmd{
				Transport:       "stdio",
				Feeds:           tt.feeds,
				AllowPrivateIPs: tt.allowPrivateIPs,
			}

			err := cmd.Run(&model.Globals{}, context.Background())

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for feeds %v, but got none", tt.feeds)
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %v", tt.errorContains, err)
				}
			} else {
				// For valid URLs, we expect the error to be related to server setup, not URL validation
				if err != nil {
					// Check that it's not a URL validation error
					if strings.Contains(err.Error(), "invalid feed URLs") ||
						strings.Contains(err.Error(), "unsupported URL scheme") ||
						strings.Contains(err.Error(), "private IP addresses") ||
						strings.Contains(err.Error(), "URL cannot be empty") {
						t.Errorf("unexpected URL validation error for valid URLs %v: %v", tt.feeds, err)
					}
					// Other errors (like server setup failures) are expected since we're not providing a full environment
				}
			}
		})
	}
}

func TestRunCmd_SecurityDefaults(t *testing.T) {
	// Test that security is enabled by default
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{"http://localhost/feed"},
		// AllowPrivateIPs not set, should default to false
	}

	err := cmd.Run(&model.Globals{}, context.Background())
	if err == nil {
		t.Error("expected localhost to be blocked by default")
	}
	if !strings.Contains(err.Error(), "private IP addresses") {
		t.Errorf("expected private IP blocking error, got: %v", err)
	}
}

func TestRunCmd_NoFeeds(t *testing.T) {
	// Test that the "no feeds" error comes before URL validation
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{}, // empty feeds
	}

	err := cmd.Run(&model.Globals{}, context.Background())
	if err == nil {
		t.Error("expected error for no feeds")
	}
	if !strings.Contains(err.Error(), "no feeds specified") {
		t.Errorf("expected 'no feeds specified' error, got: %v", err)
	}
}

// Test that validates URL validation happens before expensive operations
func TestRunCmd_ValidationOrder(t *testing.T) {
	// This test ensures URL validation fails fast before trying to create stores or servers
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{"javascript:alert('xss')"}, // Clearly malicious URL
	}

	err := cmd.Run(&model.Globals{}, context.Background())
	if err == nil {
		t.Error("expected malicious URL to be rejected")
	}

	// Should get URL validation error, not a server setup error
	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("expected URL validation error, got: %v", err)
	}
}
