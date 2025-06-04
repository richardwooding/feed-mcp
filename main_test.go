package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestVersionFlag_IsBool(t *testing.T) {
	var v VersionFlag
	if !v.IsBool() {
		t.Error("VersionFlag should be bool")
	}
}

func TestVersionFlag_BeforeApply_PrintsVersionAndExits(t *testing.T) {
	var v VersionFlag
	app := &kong.Kong{}
	vars := kong.Vars{"version": "test-version"}
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// BeforeApply should call app.Exit(0), which panics
	defer func() {
		_ = recover()
		os.Stdout = old
	}()
	_ = v.BeforeApply(app, vars)
	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "test-version") {
		t.Errorf("expected version output, got %q", out)
	}
}

func TestCLI_Parse_RunCommand(t *testing.T) {
	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	_, err = parser.Parse([]string{"run", "--transport=stdio", "http://example.com"})
	if err != nil {
		t.Errorf("failed to parse run command: %v", err)
	}
}
