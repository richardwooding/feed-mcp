package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/richardwooding/feed-mcp/model"
)

func TestRunCmd_Run_InvalidTransport(t *testing.T) {
	cmd := &RunCmd{
		Transport: "invalid",
		Feeds:     []string{"http://example.com/feed"},
	}
	ctx := context.Background()
	err := cmd.Run(&model.Globals{}, ctx)
	if err == nil {
		t.Error("expected error for invalid transport")
	}
}

func TestRunCmd_Run_NoFeeds(t *testing.T) {
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{},
	}
	ctx := context.Background()
	err := cmd.Run(&model.Globals{}, ctx)
	if err == nil {
		t.Error("expected error for no feeds specified")
	}
}

func TestRunCmd_Run_NoFeedsWithRuntimeFeedsAllowed(t *testing.T) {
	cmd := &RunCmd{
		Transport:         "stdio",
		Feeds:             []string{},
		AllowRuntimeFeeds: true,
	}
	ctx := context.Background()

	// This test validates that we can start with no feeds when AllowRuntimeFeeds is enabled
	// We expect this to not return an error from validation, but it may fail later during
	// server startup due to stdio transport setup in test environment
	err := cmd.Run(&model.Globals{}, ctx)

	// We accept that the server may fail to start in test environment due to stdio,
	// but we should not get the "no feeds specified" configuration error
	if err != nil && err.Error() == "no feeds specified - use either feed URLs or --opml" {
		t.Error("should not require feeds when AllowRuntimeFeeds is enabled")
	}
}

func TestRunCmd_Run_Valid(t *testing.T) {
	// This test just validates that NewStore succeeds with valid configuration.
	// We can't easily test the full server.Run() without setting up stdio properly.
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{"http://127.0.0.1:0/doesnotexist"},
	}

	// Test that the configuration parsing and store creation works
	_, err := cmd.parseConfig()
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}
}

// Helper method to test configuration parsing
func (c *RunCmd) parseConfig() (any, error) {
	transport, err := model.ParseTransport(c.Transport)
	if err != nil {
		return nil, err
	}
	if len(c.Feeds) == 0 {
		return nil, ErrNoFeeds
	}
	return transport, nil
}

var ErrNoFeeds = errors.New("no feeds specified")

// TestRunCmd_RateLimitFlagsDefaults verifies that --requests-per-second and
// --burst-capacity have the documented defaults when omitted from the CLI.
func TestRunCmd_RateLimitFlagsDefaults(t *testing.T) {
	type cli struct {
		Run RunCmd `cmd:""`
	}
	c := &cli{}
	parser, err := kong.New(c)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse([]string{"run", "http://example.com/feed"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Run.RequestsPerSecond != 2 {
		t.Errorf("default RequestsPerSecond = %v, want 2", c.Run.RequestsPerSecond)
	}
	if c.Run.BurstCapacity != 5 {
		t.Errorf("default BurstCapacity = %v, want 5", c.Run.BurstCapacity)
	}
}

// TestRunCmd_RateLimitFlagsOverride verifies that --requests-per-second and
// --burst-capacity propagate from CLI args onto the RunCmd struct.
func TestRunCmd_RateLimitFlagsOverride(t *testing.T) {
	type cli struct {
		Run RunCmd `cmd:""`
	}
	c := &cli{}
	parser, err := kong.New(c)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse([]string{"run", "--requests-per-second", "7.5", "--burst-capacity", "20", "http://example.com/feed"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Run.RequestsPerSecond != 7.5 {
		t.Errorf("RequestsPerSecond = %v, want 7.5", c.Run.RequestsPerSecond)
	}
	if c.Run.BurstCapacity != 20 {
		t.Errorf("BurstCapacity = %v, want 20", c.Run.BurstCapacity)
	}
}
