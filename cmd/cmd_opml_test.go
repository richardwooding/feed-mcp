package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/richardwooding/feed-mcp/model"
)

func TestRunCmd_OPML(t *testing.T) {
	// Create a temporary OPML file for testing
	tmpDir := t.TempDir()
	opmlFile := filepath.Join(tmpDir, "test.opml")

	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Test Feeds</title>
	</head>
	<body>
		<outline text="Tech News" xmlUrl="https://techcrunch.com/feed/" />
		<outline text="Security" xmlUrl="https://krebsonsecurity.com/feed/" />
		<outline text="Technology Category">
			<outline text="The Verge" xmlUrl="https://www.theverge.com/rss/index.xml" />
		</outline>
	</body>
</opml>`

	err := os.WriteFile(opmlFile, []byte(opmlContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test OPML file: %v", err)
	}

	tests := []struct {
		name        string
		cmd         RunCmd
		wantErr     bool
		errContains string
	}{
		{
			name: "valid OPML file",
			cmd: RunCmd{
				Transport: "stdio",
				OPML:      opmlFile,
			},
			wantErr: false, // Will fail with store creation but OPML parsing should succeed
		},
		{
			name: "both OPML and feeds specified",
			cmd: RunCmd{
				Transport: "stdio",
				OPML:      opmlFile,
				Feeds:     []string{"https://example.com/feed.xml"},
			},
			wantErr:     true,
			errContains: "cannot specify both --opml and feed URLs",
		},
		{
			name: "no feeds or OPML specified",
			cmd: RunCmd{
				Transport: "stdio",
			},
			wantErr:     true,
			errContains: "no feeds specified - use either feed URLs or --opml",
		},
		{
			name: "non-existent OPML file",
			cmd: RunCmd{
				Transport: "stdio",
				OPML:      "/nonexistent/file.opml",
			},
			wantErr:     true,
			errContains: "failed to open OPML file",
		},
		{
			name: "empty OPML path",
			cmd: RunCmd{
				Transport: "stdio",
				OPML:      "",
				Feeds:     []string{}, // Empty feeds
			},
			wantErr:     true,
			errContains: "no feeds specified - use either feed URLs or --opml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals := &model.Globals{}
			ctx := context.Background()

			err := tt.cmd.Run(globals, ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RunCmd.Run() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RunCmd.Run() error = %v, should contain %q", err, tt.errContains)
				}
			} else {
				// For the success case, we expect it to fail later in the process
				// (e.g., when trying to create the store with real feed URLs)
				// but the OPML parsing itself should work
				if err != nil && strings.Contains(err.Error(), "failed to parse OPML") {
					t.Errorf("RunCmd.Run() failed at OPML parsing: %v", err)
				}
			}
		})
	}
}

func TestRunCmd_OPML_URL(t *testing.T) {
	t.Run("valid OPML URL", func(t *testing.T) {
		opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<body>
		<outline text="Remote Feed 1" xmlUrl="https://example.com/feed1.xml" />
	</body>
</opml>`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(opmlContent))
		}))
		defer server.Close()

		cmd := RunCmd{Transport: "stdio", OPML: server.URL}
		err := cmd.Run(&model.Globals{}, context.Background())

		// Should not fail with OPML parsing error
		if err != nil && strings.Contains(err.Error(), "failed to parse OPML") {
			t.Errorf("RunCmd.Run() failed at OPML parsing: %v", err)
		}
	})

	t.Run("HTTP 404 from OPML URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cmd := RunCmd{Transport: "stdio", OPML: server.URL}
		err := cmd.Run(&model.Globals{}, context.Background())

		if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("RunCmd.Run() expected HTTP 404 error, got: %v", err)
		}
	})

	t.Run("invalid OPML content from URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not valid xml"))
		}))
		defer server.Close()

		cmd := RunCmd{Transport: "stdio", OPML: server.URL}
		err := cmd.Run(&model.Globals{}, context.Background())

		if err == nil || !strings.Contains(err.Error(), "failed to parse OPML") {
			t.Errorf("RunCmd.Run() expected OPML parsing error, got: %v", err)
		}
	})
}

func TestRunCmd_OPML_SecurityValidation(t *testing.T) {
	// Create OPML with private IP feed URLs
	tmpDir := t.TempDir()
	opmlFile := filepath.Join(tmpDir, "private-feeds.opml")

	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<body>
		<outline text="Local Feed" xmlUrl="http://192.168.1.100/feed.xml" />
		<outline text="Localhost Feed" xmlUrl="http://localhost/feed.xml" />
	</body>
</opml>`

	err := os.WriteFile(opmlFile, []byte(opmlContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test OPML file: %v", err)
	}

	tests := []struct {
		name            string
		allowPrivateIPs bool
		wantErr         bool
		errContains     string
	}{
		{
			name:            "private IPs blocked by default",
			allowPrivateIPs: false,
			wantErr:         true,
			errContains:     "private IP",
		},
		{
			name:            "private IPs allowed with flag",
			allowPrivateIPs: true,
			wantErr:         false, // Will fail later but not with IP validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := RunCmd{
				Transport:       "stdio",
				OPML:            opmlFile,
				AllowPrivateIPs: tt.allowPrivateIPs,
			}

			globals := &model.Globals{}
			ctx := context.Background()

			err := cmd.Run(globals, ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RunCmd.Run() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RunCmd.Run() error = %v, should contain %q", err, tt.errContains)
				}
			} else {
				// Should not fail with private IP validation when allowed
				if err != nil && strings.Contains(err.Error(), "private IP") {
					t.Errorf("RunCmd.Run() failed with private IP error when it should be allowed: %v", err)
				}
			}
		})
	}
}

func TestRunCmd_OPML_BackwardsCompatibility(t *testing.T) {
	// Test that existing feed URL functionality still works
	tests := []struct {
		name        string
		cmd         RunCmd
		wantErr     bool
		errContains string
	}{
		{
			name: "traditional feed URLs still work",
			cmd: RunCmd{
				Transport: "stdio",
				Feeds:     []string{"https://techcrunch.com/feed/", "https://example.com/feed.xml"},
			},
			wantErr: false, // Will fail with store creation but feed validation should succeed
		},
		{
			name: "empty feeds array still gives appropriate error",
			cmd: RunCmd{
				Transport: "stdio",
				Feeds:     []string{},
			},
			wantErr:     true,
			errContains: "no feeds specified - use either feed URLs or --opml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals := &model.Globals{}
			ctx := context.Background()

			err := tt.cmd.Run(globals, ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RunCmd.Run() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RunCmd.Run() error = %v, should contain %q", err, tt.errContains)
				}
			} else {
				// Should not fail with feed validation for valid public URLs
				if err != nil && (strings.Contains(err.Error(), "no feeds specified") || strings.Contains(err.Error(), "invalid URL")) {
					t.Errorf("RunCmd.Run() failed at feed validation: %v", err)
				}
			}
		})
	}
}

// BenchmarkRunCmd_OPML_Parsing benchmarks OPML parsing performance
func BenchmarkRunCmd_OPML_Parsing(b *testing.B) {
	// Create a large OPML file for benchmarking
	tmpDir, _ := os.MkdirTemp("", "opml_bench")
	defer os.RemoveAll(tmpDir)

	opmlFile := filepath.Join(tmpDir, "large.opml")

	// Generate OPML with many feeds
	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
	<head>
		<title>Large Feed Collection</title>
	</head>
	<body>`

	for i := 0; i < 100; i++ {
		opmlContent += fmt.Sprintf(`
		<outline text="Feed %d" xmlUrl="https://example%d.com/feed.xml" />`, i, i)
	}

	opmlContent += `
	</body>
</opml>`

	os.WriteFile(opmlFile, []byte(opmlContent), 0o644)

	cmd := RunCmd{
		Transport: "stdio",
		OPML:      opmlFile,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		globals := &model.Globals{}
		ctx := context.Background()

		// We expect this to fail at store creation, but OPML parsing should be fast
		cmd.Run(globals, ctx)
	}
}
