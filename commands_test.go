package main

import (
	"testing"
)

type dummyGlobals struct{}

func TestRunCmd_Run_InvalidTransport(t *testing.T) {
	cmd := &RunCmd{
		Transport: "invalid",
		Feeds:     []string{"http://example.com/feed"},
	}
	err := cmd.Run(&Globals{})
	if err == nil {
		t.Error("expected error for invalid transport")
	}
}

func TestRunCmd_Run_NoFeeds(t *testing.T) {
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{},
	}
	err := cmd.Run(&Globals{})
	if err == nil {
		t.Error("expected error for no feeds specified")
	}
}

func TestRunCmd_Run_Valid(t *testing.T) {
	// Use a dummy feed URL that will fail to fetch, but NewStore should succeed
	cmd := &RunCmd{
		Transport: "stdio",
		Feeds:     []string{"http://127.0.0.1:0/doesnotexist"},
	}
	_ = cmd.Run(&Globals{})
}
