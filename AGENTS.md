# Breyta CLI (Go) – Agent Notes

This directory (`breyta-cli/`) is a standalone Go module that ships the `breyta` binary:
- `breyta` (no args): Bubble Tea TUI
- `breyta <subcommand>`: CLI API commands

## Key paths
- TUI entry: `internal/tui/home.go`
- Cobra root: `internal/cli/root.go`
- Agent skill bundle (for external agents authoring flows): `skills/breyta-flows-cli/SKILL.md`

## Build & test
- Build: `go build ./...`
- Test: `go test ./...`

## Conventions
- Keep changes small and dependency-light (prefer stdlib).
- TUI: prefer modal-based interactions; keep keyboard hints in the header.
- When changing API-facing commands, update `breyta docs` output and `docs/agentic-chat.md` if user-facing behavior changes.

