// Package version provides version information for the feed-mcp server.
package version

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// These variables can be set at build time using -ldflags
var (
	// Version is the version of the binary, set at build time
	Version = "dev"
	// GitCommit is the git commit hash, set at build time  
	GitCommit = "unknown"
	// BuildDate is the build date, set at build time
	BuildDate = "unknown"
)

// Info contains version information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
	GoVersion string
}

// Get returns version information
func Get() Info {
	info := Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}

	// Try to get version from build info if not set at build time
	if info.Version == "dev" {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			// Look for version in VCS info
			for _, setting := range buildInfo.Settings {
				switch setting.Key {
				case "vcs.revision":
					if info.GitCommit == "unknown" {
						// Use short commit hash
						if len(setting.Value) > 7 {
							info.GitCommit = setting.Value[:7]
						} else {
							info.GitCommit = setting.Value
						}
					}
				case "vcs.time":
					if info.BuildDate == "unknown" {
						info.BuildDate = setting.Value
					}
				}
			}
		}
	}

	// Clean version string (remove 'v' prefix if present)
	if strings.HasPrefix(info.Version, "v") {
		info.Version = strings.TrimPrefix(info.Version, "v")
	}

	return info
}

// GetVersion returns just the version string
func GetVersion() string {
	return Get().Version
}

// GetFullVersion returns a full version string with commit info
func GetFullVersion() string {
	info := Get()
	if info.GitCommit != "unknown" {
		return info.Version + "-" + info.GitCommit
	}
	return info.Version
}