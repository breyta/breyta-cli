# Breyta CLI (Go) – Agent Notes

This directory (`breyta-cli/`) is a standalone Go module that ships the `breyta` binary:
- `breyta` (no args): CLI help
- `breyta <subcommand>`: CLI API commands

## Key paths
- Cobra root: `internal/cli/root.go`
- Docs + skill bundle source of truth: served by `flows-api` (`/api/docs/...`) in the main `breyta` repo

## Build & test
- Build: `go build ./...`
- Test: `go test ./...`
- Release runbook: `docs/RELEASING.md`

## Conventions
- Keep changes small and dependency-light (prefer stdlib).
- TUI: prefer modal-based interactions; keep keyboard hints in the header.
- When changing API-facing commands, update the docs pages served by `flows-api` (for example the public docs pages and the `breyta` skill page in `bases/flows-api/resources/public/docs/`).
- Keep Breyta flow-authoring guidance aligned across the three user-facing surfaces in this repo:
  - installed skill override text in `internal/skilldocs/overrides.go`
  - generated `breyta init` workspace guidance in `internal/cli/init.go`
  - repo-facing docs in `README.md`
