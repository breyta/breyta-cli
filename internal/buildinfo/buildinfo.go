package buildinfo

// These values are set at build time (typically via -ldflags -X).
// Defaults keep local `go run` / `go test` usable.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
