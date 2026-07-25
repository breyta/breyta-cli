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

When `functions.exec` is available, run independent tool calls concurrently
within one bounded stage. Prefer `await Promise.allSettled([...])` and inspect
every result. `Promise.all(...)` rejects early but does not cancel calls that
already started, so use it only when discarding other results is intentional.
Keep dependencies, waits/resumes, approvals, adaptive investigations,
conflicting mutations, and builds or mutations that write the same outputs
sequential. Do not split otherwise batchable inspections across outer calls.

Keep each nested call's output bounded. Prefer focused queries and per-call
output limits; broad outputs that can truncate task evidence are not a valid
efficiency gain. If a result is truncated, narrow or page only that result
instead of rerunning the whole batch.

## Minimal Working Rules

- Understand the task and trace the real flow first. Then stop at the first
  sufficient rung: skip speculative work, reuse repository code, use the
  standard library, use native platform features, use an installed dependency,
  and only then write the minimum new code.
- Fix root causes at the shared boundary after checking callers. Prefer
  deletion, boring code, few files, and the shortest correct diff.
- Do not add one-use abstractions, future scaffolding/config, or dependencies
  when existing code or a few direct lines suffice.
- Do not simplify away requested behavior, security, trust-boundary validation,
  data-loss/error handling, or accessibility.
- Leave the smallest runnable regression check for non-trivial logic. Mark a
  deliberate ceiling with a `ponytail:` comment and its upgrade trigger.
