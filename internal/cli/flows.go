package cli

// NOTE:
// This file intentionally exists as a no-op.
//
// Some editors/tools have been observed to recreate `internal/cli/flows.go` as an
// empty file, which breaks `go test` / `go install` with:
//   expected 'package', found 'EOF'
//
// Keeping this file valid makes the build resilient.
