// Command mcpb packs the feed-mcp binary into an Anthropic MCP Bundle (.mcpb) —
// a zip archive containing a manifest.json at the root and the server binary
// under server/. It is a small, dependency-free (stdlib-only) alternative to
// the Node @anthropic-ai/mcpb CLI, so a bundle can be built on any OS with just
// the Go toolchain.
//
// Usage:
//
//	go run ./tools/mcpb pack [-binary path] [-os GOOS] [-arch GOARCH] [-version v] [-out file.mcpb]
//
// One .mcpb targets a single OS+arch (the manifest format selects a binary by OS
// only, never architecture), so a release emits one bundle per platform.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	projectName = "feed-mcp"
	displayName = "Feed MCP"
	authorName  = "Richard Wooding"
	description = "MCP server that fetches RSS/Atom/JSON feeds and serves them to AI assistants"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "pack" {
		fmt.Fprintln(os.Stderr, "usage: mcpb pack [-binary path] [-os GOOS] [-arch GOARCH] [-version v] [-out file.mcpb]")
		os.Exit(2)
	}
	if err := pack(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "mcpb: "+err.Error())
		os.Exit(1)
	}
}

func pack(argv []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	defaultBinary := "./" + projectName
	if runtime.GOOS == "windows" {
		defaultBinary += ".exe"
	}
	var (
		binary  = fs.String("binary", defaultBinary, "path to the compiled server binary to bundle")
		goos    = fs.String("os", runtime.GOOS, "target OS (GOOS): linux, darwin or windows")
		goarch  = fs.String("arch", runtime.GOARCH, "target architecture (GOARCH); used only in the output filename")
		version = fs.String("version", "dev", "bundle version (a leading \"v\" is stripped)")
		out     = fs.String("out", "", "output .mcpb path (default dist/<name>_<version>_<os>_<arch>.mcpb)")
	)
	if err := fs.Parse(argv); err != nil {
		return err
	}

	ver := strings.TrimPrefix(*version, "v")
	outPath := *out
	if outPath == "" {
		outPath = filepath.Join("dist", fmt.Sprintf("%s_%s_%s_%s.mcpb", projectName, ver, *goos, *goarch))
	}

	m := buildManifest(*goos, ver)
	if err := m.validate(); err != nil {
		return err
	}
	if err := writeBundle(outPath, m, *binary, binaryName(*goos)); err != nil {
		return err
	}
	fmt.Printf("mcpb: wrote %s\n", outPath)
	return nil
}

// binaryName is the server binary's name inside the bundle for the target OS.
func binaryName(goos string) string {
	if goos == "windows" {
		return projectName + ".exe"
	}
	return projectName
}

// manifestPlatform maps a Go GOOS to the manifest platform token (Node's
// process.platform): windows -> win32, others unchanged.
func manifestPlatform(goos string) string {
	if goos == "windows" {
		return "win32"
	}
	return goos
}

func buildManifest(goos, version string) Manifest {
	binName := binaryName(goos)
	entry := "server/" + binName
	// The command must be an absolute path; ${__dirname} expands to the
	// installed bundle directory at runtime. A relative command (e.g.
	// "server/...") is not resolved against the bundle dir and fails to spawn.
	cmd := "${__dirname}/" + entry

	m := Manifest{
		ManifestVersion: "0.3",
		Name:            projectName,
		DisplayName:     displayName,
		Version:         version,
		Description:     description,
		Author:          Author{Name: authorName},
		Server: Server{
			Type:       "binary",
			EntryPoint: entry,
			MCPConfig: MCPConfig{
				Command: cmd,
				// feed-mcp's "run" requires feeds, --opml, or
				// --allow-runtime-feeds to start. Enabling runtime feeds lets
				// the bundle start with zero configured feeds (users add them
				// in chat). ${user_config.feeds} is a multi-value field, so it
				// must come last — it expands to zero or more positional args.
				Args: []string{
					"run",
					"--transport", "stdio",
					"--allow-runtime-feeds",
					"--timeout", "${user_config.timeout}",
					"--expire-after", "${user_config.expire_after}",
					"${user_config.feeds}",
				},
			},
		},
		UserConfig: map[string]UserConfigField{
			"feeds": {
				Type:        "string",
				Title:       "Feed URLs",
				Description: "RSS/Atom/JSON feed URLs to load at startup (you can also add feeds at runtime)",
				Multiple:    true,
				Required:    false,
			},
			"timeout": {
				Type:        "string",
				Title:       "Request timeout",
				Description: "Per-feed fetch timeout (Go duration, e.g. 30s)",
				Default:     "30s",
			},
			"expire_after": {
				Type:        "string",
				Title:       "Cache expiration",
				Description: "How long feeds stay cached (Go duration, e.g. 1h)",
				Default:     "1h",
			},
		},
		Compatibility: &Compatibility{Platforms: []string{manifestPlatform(goos)}},
	}
	// On Windows the whole bundle already targets .exe; a win32 override makes
	// the intent explicit and matches the manifest spec's binary example.
	if goos == "windows" {
		m.Server.PlatformOverrides = map[string]PlatformOverride{
			"win32": {Command: cmd},
		}
	}
	return m
}

// writeBundle creates the .mcpb zip: manifest.json at the root and the binary at
// server/<binName> with an executable mode bit.
func writeBundle(outPath string, m Manifest, binarySrc, binName string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)

	mw, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	if err := m.writeJSON(mw); err != nil {
		return err
	}

	// server/<binName> — forward slashes are required by archive/zip on every
	// host OS, which keeps Windows-built bundles valid. Mode 0755 so the
	// extracted binary stays executable on macOS/Linux.
	hdr := &zip.FileHeader{Name: "server/" + binName, Method: zip.Deflate}
	hdr.SetMode(0o755)
	bw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	src, err := os.Open(binarySrc)
	if err != nil {
		return fmt.Errorf("open binary: %w", err)
	}
	defer func() { _ = src.Close() }()
	if _, err := io.Copy(bw, src); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}
