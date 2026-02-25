# Breyta CLI (`breyta`)

This repo contains the `breyta` command-line interface (CLI) for working with Breyta workflows.

The CLI is **agent-first**: it’s designed to be called by tools like **Codex**, **Claude Code**, **Cursor**, and **Gemini CLI** (and also works great for humans in a terminal).

- `breyta` shows help.
- `breyta <command>` runs a scriptable CLI command (JSON).

## What is Breyta?

Breyta is a workflow platform with a deterministic runtime for building and operating reliable backend automations ("flows") with your coding agent.

With Breyta, you can:

- Build multi-step flows with triggers, waits, external API calls, and human approvals
- Version, deploy, and safely roll forward workflow changes
- Run flows from apps, webhooks, and schedules with clear run history and outputs
- Give AI agents a deterministic, scriptable way to create and operate workflows through the CLI

This CLI is the main interface for flow authoring and operations:

- Browse workflows ("flows"), versions, and runs
- Start runs and inspect results
- Cancel active runs when needed with `breyta runs cancel <workflow-id> --reason "..."`
- Fetch run artifacts via a unified "resources" interface

Typical flow example: Stripe webhook -> normalize payload -> approval step -> external action -> artifact/audit output.

When Breyta fits: multi-step backend workflows with APIs, state, approvals, and operational visibility.
Less ideal: one-step automations where you do not need versioning and runtime controls.

Determinism and orchestration constraints are documented in:
- `breyta docs` (product docs served from the Breyta API)

Concurrency policy quick rule:
- Reconciler, sweeper, and scheduled cleanup flows: `:on-new-version :supersede`
- Use `:on-new-version :drain` only when in-flight runs must finish on the old version

## Agent-first design

- **Scriptable outputs:** CLI commands return stable JSON, which makes it easy for agents to parse and act on results.
- **Docs from the API:** `breyta docs` searches and prints product docs from the Breyta API (`docs find` / `docs show`).
- **Command truth:** use `breyta help <command...>` for flags and usage.
- **Agent tooling:** `breyta skills install` downloads the Breyta skill bundle from the docs API and installs it for Codex/Cursor/Claude Code/Gemini CLI.

### Recommended: set up your agent via CLI

Use the CLI setup flow to:

- **Log in** (so the CLI can authenticate to the API)
- **Install the Breyta skill** to your local agent tool (Codex / Cursor / Claude Code / Gemini CLI)
- **Set your default workspace** for subsequent commands

Run:

```bash
breyta auth login
breyta skills install --provider <codex|cursor|claude|gemini>
breyta workspaces list
breyta workspaces use <workspace-id>
```

## Install

Choose one:

- **Homebrew (macOS):**
  - `brew tap breyta/tap`
  - `brew install breyta`
- **Prebuilt binaries (no Go required):** https://github.com/breyta/breyta-cli/releases
- **Go install latest release (Go required):** `go install github.com/breyta/breyta-cli/cmd/breyta@latest`
- **From source checkout (Go required):** `go install ./cmd/breyta` (from this repo)

Verify install:

```bash
breyta version
```

Check for updates:

```bash
breyta upgrade
```

Upgrade in one command (Homebrew installs):

```bash
breyta upgrade --apply
```

Or open the latest release page:

```bash
breyta upgrade --open
```

After installing `breyta`, install the agent skill bundle (recommended for Codex/Cursor/Claude Code/Gemini CLI):

```bash
breyta init --provider <codex|cursor|claude|gemini>
```

This installs the Breyta skill bundle for your agent tool and creates a local `breyta-workspace/` directory with an `AGENTS.md` file.

If you only want the skill bundle (no workspace files), use:

```bash
breyta skills install --provider <codex|cursor|claude|gemini>
```

Examples (skill-only):

```bash
# Codex
breyta skills install --provider codex

# Cursor
breyta skills install --provider cursor

# Claude Code
breyta skills install --provider claude

# Gemini CLI
breyta skills install --provider gemini
```

You can also do this from the TUI: `breyta` then press `s` (Agent skills).

More details: `breyta docs find "install"` (then `breyta docs show <slug>`).

## First 2 Minutes

Hosted Breyta:

```bash
breyta init --provider <codex|cursor|claude|gemini>
breyta auth login
breyta flows list
```

Local development (`flows-api` running locally):

```bash
export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
breyta --dev workspaces current --pretty
breyta --dev flows list
```

Run an existing flow and wait for output:

```bash
breyta flows show <slug>
breyta flows run <slug> --input '{"n":41}' --wait
```

Environment/setup details: `breyta docs find "agent"` (and `breyta docs show <slug>`).

Docs/help:

- Product docs: `breyta docs` / `breyta docs find "<query>"` / `breyta docs show <slug>`
- Command flags: `breyta help <command...>`

## Development

This repo also includes local-development tooling and docs:

- Build: `go build ./...`
- Test: `go test ./...`
