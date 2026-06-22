package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildManifestUnix checks the manifest for a non-Windows target: a
// ${__dirname}-prefixed command, the "run" subcommand first in args, the
// multi-value feeds field last, and a platform token equal to the GOOS.
func TestBuildManifestUnix(t *testing.T) {
	m := buildManifest("darwin", "1.2.3")
	if err := m.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if m.Server.MCPConfig.Command != "${__dirname}/server/feed-mcp" {
		t.Errorf("command = %q, want ${__dirname}/server/feed-mcp", m.Server.MCPConfig.Command)
	}
	args := m.Server.MCPConfig.Args
	if len(args) == 0 || args[0] != "run" {
		t.Errorf("args[0] = %v, want \"run\"", args)
	}
	if got := args[len(args)-1]; got != "${user_config.feeds}" {
		t.Errorf("last arg = %q, want ${user_config.feeds} (multi-value must be last)", got)
	}
	feeds, ok := m.UserConfig["feeds"]
	if !ok || !feeds.Multiple {
		t.Errorf("feeds field = %+v, want present with Multiple=true", feeds)
	}
	if got := m.Compatibility.Platforms; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("platforms = %v, want [darwin]", got)
	}
	if m.Server.PlatformOverrides != nil {
		t.Errorf("unexpected platform_overrides on a unix target: %v", m.Server.PlatformOverrides)
	}
}

// TestBuildManifestWindows checks the Windows-specific bits: a .exe entry
// point, the win32 platform token, and a win32 override.
func TestBuildManifestWindows(t *testing.T) {
	m := buildManifest("windows", "v0.9.0")
	if m.Server.EntryPoint != "server/feed-mcp.exe" {
		t.Errorf("entry_point = %q, want server/feed-mcp.exe", m.Server.EntryPoint)
	}
	if got := m.Compatibility.Platforms; len(got) != 1 || got[0] != "win32" {
		t.Errorf("platforms = %v, want [win32]", got)
	}
	ov, ok := m.Server.PlatformOverrides["win32"]
	if !ok || ov.Command != "${__dirname}/server/feed-mcp.exe" {
		t.Errorf("win32 override = %+v, want command ${__dirname}/server/feed-mcp.exe", ov)
	}
}

// TestWriteBundle packs a fake binary and confirms the .mcpb zip holds a valid
// manifest.json at the root and the server binary under server/.
func TestWriteBundle(t *testing.T) {
	dir := t.TempDir()
	binSrc := filepath.Join(dir, "feed-mcp")
	if err := os.WriteFile(binSrc, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.mcpb")

	m := buildManifest("linux", "1.0.0")
	if err := writeBundle(out, &m, binSrc, binaryName("linux")); err != nil {
		t.Fatalf("writeBundle: %v", err)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer func() { _ = zr.Close() }()

	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				t.Fatal(err)
			}
			_ = rc.Close()
			var decoded Manifest
			if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
				t.Fatalf("manifest.json is not valid JSON: %v", err)
			}
		}
	}
	if !names["manifest.json"] {
		t.Error("bundle missing manifest.json at root")
	}
	if !names["server/feed-mcp"] {
		t.Error("bundle missing server/feed-mcp")
	}
}
