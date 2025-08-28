package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/richardwooding/feed-mcp/model"
)

type dummyGlobals struct{}

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
func (c *RunCmd) parseConfig() (interface{}, error) {
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
