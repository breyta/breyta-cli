# Breyta CLI (Go) – Agent Notes

This directory (`breyta-cli/`) is a standalone Go module that ships the `breyta` binary:
- `breyta` (no args): CLI help
- `breyta <subcommand>`: CLI API commands

## Key paths
- Cobra root: `internal/cli/root.go`
- Agent skill bundle (for external agents authoring flows): `skills/breyta/SKILL.md`

## Build & test
- Build: `go build ./...`
- Test: `go test ./...`

## Conventions
- Keep changes small and dependency-light (prefer stdlib).
- When changing API-facing commands, update `breyta docs` output and `docs/agentic-chat.md` if user-facing behavior changes.
