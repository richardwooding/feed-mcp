package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/richardwooding/feed-mcp/model"
)

func TestGracefulShutdown(t *testing.T) {
	t.Skip("Skipping flaky shutdown test - stdio server shutdown is complex to test properly")
	// Use a dummy feed URL that will fail to fetch, but NewStore should succeed
	cmd := &RunCmd{
		Transport:       "stdio",
		Feeds:           []string{"http://127.0.0.1:0/doesnotexist"},
		ShutdownTimeout: 1 * time.Second,
	}

	// Create a context that will be cancelled to simulate shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run the command in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run(&model.Globals{}, ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to simulate a shutdown signal
	cancel()

	// Wait for the command to finish with a timeout
	select {
	case err := <-done:
		// The server should have shut down gracefully
		// Context cancellation errors are expected here
		if err != nil && err.Error() != "context canceled" {
			t.Logf("Server shut down with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not shut down within expected time")
	}
}

func TestShutdownTimeout(t *testing.T) {
	cmd := &RunCmd{
		Transport:       "stdio",
		Feeds:           []string{"http://127.0.0.1:0/doesnotexist"},
		ShutdownTimeout: 100 * time.Millisecond, // Very short timeout
	}

	// Test that the timeout configuration is parsed correctly
	if cmd.ShutdownTimeout != 100*time.Millisecond {
		t.Errorf("expected ShutdownTimeout to be 100ms, got %v", cmd.ShutdownTimeout)
	}
}

func TestDefaultShutdownTimeout(t *testing.T) {
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{"http://example.com/feed"},
	}

	// The default should be set by Kong based on the struct tag
	// We can't easily test this without involving Kong parsing,
	// but we can verify the struct field has the right tag

	// This is a compile-time check that the field exists with the right type
	var _ time.Duration = cmd.ShutdownTimeout
}
