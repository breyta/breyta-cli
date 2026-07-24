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

## Code Mode Tool Batching

In Code Mode, within each bounded stage, run independent, functions.exec-available tool calls concurrently in one functions.exec call. Use await Promise.allSettled([...]) when partial results are useful, and inspect every result; use await Promise.all([...]) only when any failure should abort the batch. Keep dependencies, waits/resumes, approvals, conflicting or interdependent mutations, and adaptive investigations where each result may change the next step sequential. Do not split otherwise batchable inspections across outer tool calls.

Keep each nested call's output bounded. Prefer focused queries and per-call
output limits; broad outputs that can truncate task evidence are not a valid
efficiency gain. If a result is truncated, narrow or page only that result
instead of rerunning the whole batch.
