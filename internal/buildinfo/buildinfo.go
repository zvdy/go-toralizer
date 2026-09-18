// Package buildinfo exposes the version details stamped into the binary.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Set at build time with -ldflags, for example:
//
//	-X github.com/zvdy/go-toralizer/internal/buildinfo.version=v1.2.3
var (
	version = ""
	commit  = ""
	date    = ""
)

// Info describes the running binary.
type Info struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	Platform  string
}

// Get returns the build details, falling back to the metadata the Go toolchain
// embeds when the binary was not stamped with -ldflags.
func Get() Info {
	info := Info{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}

		// A stamped commit is left exactly as the build passed it; only a
		// revision recovered from the embedded VCS data gets the dirty marker.
		fromVCS := info.Commit == ""
		dirty := false

		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = setting.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = setting.Value
				}
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}

		if fromVCS && dirty && info.Commit != "" {
			info.Commit += "-dirty"
		}
	}

	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}

// Short returns just the version, suitable for `--version`.
func (i Info) Short() string { return i.Version }

// String returns a multi-line summary of the build.
func (i Info) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version:    %s\n", i.Version)
	if i.Commit != "" {
		fmt.Fprintf(&b, "commit:     %s\n", i.Commit)
	}
	if i.Date != "" {
		fmt.Fprintf(&b, "built:      %s\n", i.Date)
	}
	fmt.Fprintf(&b, "go version: %s\n", i.GoVersion)
	fmt.Fprintf(&b, "platform:   %s", i.Platform)
	return b.String()
}
