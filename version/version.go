// Package version provides version information for the feed-mcp server.
package version

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// Constants for repeated string values
const (
	unknownValue = "unknown"
	devVersion   = "dev"
)

// These variables can be set at build time using -ldflags
var (
	// Version is the version of the binary, set at build time
	Version = "dev"
	// GitCommit is the git commit hash, set at build time
	GitCommit = unknownValue
	// BuildDate is the build date, set at build time
	BuildDate = unknownValue
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
	if info.Version == devVersion {
		updateBuildInfo(&info)
	}

	// Clean version string (remove 'v' prefix if present)
	info.Version = strings.TrimPrefix(info.Version, "v")

	return info
}

// updateBuildInfo updates version info from build metadata
func updateBuildInfo(info *Info) {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// Look for version in VCS info
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			updateGitCommit(info, setting.Value)
		case "vcs.time":
			updateBuildDate(info, setting.Value)
		}
	}
}

// updateGitCommit updates git commit if not already set
func updateGitCommit(info *Info, value string) {
	if info.GitCommit != unknownValue {
		return
	}
	// Use short commit hash
	if len(value) > 7 {
		info.GitCommit = value[:7]
	} else {
		info.GitCommit = value
	}
}

// updateBuildDate updates build date if not already set
func updateBuildDate(info *Info, value string) {
	if info.BuildDate == unknownValue {
		info.BuildDate = value
	}
}

// GetVersion returns just the version string
func GetVersion() string {
	return Get().Version
}

// GetFullVersion returns a full version string with commit info
func GetFullVersion() string {
	info := Get()
	if info.GitCommit != unknownValue {
		return info.Version + "-" + info.GitCommit
	}
	return info.Version
}
