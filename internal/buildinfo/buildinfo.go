package buildinfo

import (
	"runtime/debug"
	"strings"
)

// These values are set at build time (typically via -ldflags -X).
// Defaults keep local `go run` / `go test` usable.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// DisplayVersion returns a user-facing version string.
//
// Conventions:
// - "dev" stays "dev"
// - numeric versions get a "v" prefix (e.g. "2026.1.1" -> "v2026.1.1")
// - values already starting with "v" are left as-is
//
// When Version is unset/dev, it falls back to Go build info (useful for
// `go install ...@vX.Y.Z` builds where the module version is embedded).
func DisplayVersion() string {
	v := strings.TrimSpace(Version)

	if v == "" || v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			mv := strings.TrimSpace(bi.Main.Version)
			if mv != "" && mv != "(devel)" {
				v = mv
			}
		}
	}

	v = strings.TrimSpace(v)
	if v == "" || v == "dev" || v == "(devel)" {
		return "dev"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	if v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}
