# Breyta CLI (`breyta`)

This repo contains the `breyta` command-line interface (CLI) and terminal UI (TUI) for working with Breyta workflows.

The CLI is **agent-first**: it’s designed to be called by tools like **Codex**, **Claude Code**, and **Cursor** (and also works great for humans in a terminal).

- `breyta` opens the interactive TUI.
- `breyta <command>` runs a scriptable CLI command (JSON).

## Agent-first design

- **Scriptable outputs:** CLI commands return stable JSON, which makes it easy for agents to parse and act on results.
- **On-demand docs:** `breyta docs` provides Markdown command docs that agent tools can ingest directly.
- **Agent tooling:** this repo includes a skill bundle at `skills/breyta/SKILL.md` (install instructions in `docs/agentic-chat.md`).

### Recommended: set up your agent via the TUI

The easiest setup flow is to use the TUI to:

- **Log in** (so the CLI can authenticate to the API)
- **Install the Breyta skill** to your local agent tool (Codex / Cursor / Claude Code)
- **Browse flows and runs** (the TUI is a great “truth surface” for what your agent created)

Start the TUI:

```bash
breyta
```

Then:
Then use:

- **Auth** (press `a`) to log in
- **Skills** (press `s`) to install the skill bundle

## What is Breyta?

Breyta is a workflow platform. The CLI/TUI helps you:

- Browse workflows (“flows”) and their versions
- Start runs and inspect results
  - `breyta runs cancel <workflow-id> --reason "..."` (use `--force` to terminate)
  - `workflow-id` is the primary identifier
- Fetch artifacts via a unified “resources” interface

## Install

Choose one:

- **Homebrew (macOS):**
  - `brew tap breyta/tap`
  - `brew install breyta`
- **Prebuilt binaries (no Go required):** https://github.com/breyta/breyta-cli/releases
- **From source (Go required):** `go install ./cmd/breyta` (from this repo)

More details (including troubleshooting): `docs/install.md`.

## Quick start

```bash
breyta --help
breyta docs
breyta
```

If you’re using the hosted Breyta API, authenticate with:

```bash
breyta auth login
breyta flows list
```

## Docs

- Docs index: `docs/index.md`
- Install: `docs/install.md`
- Agentic chat setup (Claude Code, Cursor, Codex, etc.): `docs/agentic-chat.md`
- Cursor IDE sandbox/network troubleshooting: `docs/agentic-chat.md#cursor-ide-sandboxing`
- Distribution / releases: `docs/distribution.md`

## Development

This repo also includes local-development tooling and docs:

- Build: `go build ./...`
- Test: `go test ./...`
- Local `flows-api` (dev): `docs/flows-api-local.md`
- CLI development notes (mock mode, demos): `docs/development.md`
