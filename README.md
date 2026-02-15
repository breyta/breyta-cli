# Breyta CLI (`breyta`)

This repo contains the `breyta` command-line interface (CLI) for working with Breyta workflows.

The CLI is **agent-first**: it’s designed to be called by tools like **Codex**, **Claude Code**, and **Cursor** (and also works great for humans in a terminal).

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
- `docs/flows-api-local.md`
- `skills/breyta/references/authoring-reference.md`

## Agent-first design

- **Scriptable outputs:** CLI commands return stable JSON, which makes it easy for agents to parse and act on results.
- **On-demand docs:** `breyta docs` provides Markdown command docs that agent tools can ingest directly.
- **Agent tooling:** this repo includes a skill bundle at `skills/breyta/SKILL.md` (install instructions in `docs/agentic-chat.md`).

### Recommended: set up your agent via CLI

Use the CLI setup flow to:

- **Log in** (so the CLI can authenticate to the API)
- **Install the Breyta skill** to your local agent tool (Codex / Cursor / Claude Code)
- **Set your default workspace** for subsequent commands

Run:

```bash
breyta auth login
breyta skills install --provider <codex|cursor|claude>
breyta workspaces list
breyta workspaces use <workspace-id>
```

## Install

Choose one:

- **Homebrew (macOS):**
  - `brew tap breyta/tap`
  - `brew install breyta`
- **Prebuilt binaries (no Go required):** https://github.com/breyta/breyta-cli/releases
- **From source (Go required):** `go install ./cmd/breyta` (from this repo)

Verify install:

```bash
breyta --version
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

After installing `breyta`, install the agent skill bundle (recommended for Codex/Cursor/Claude Code):

```bash
breyta init --provider <codex|cursor|claude>
```

This installs the Breyta skill bundle for your agent tool and creates a local `breyta-workspace/` directory with an `AGENTS.md` file.

If you only want the skill bundle (no workspace files), use:

```bash
breyta skills install --provider <codex|cursor|claude>
```

Examples (skill-only):

```bash
# Codex
breyta skills install --provider codex

# Cursor
breyta skills install --provider cursor

# Claude Code
breyta skills install --provider claude
```

More details (including troubleshooting): `docs/install.md`.

## First 2 Minutes

Hosted Breyta:

```bash
breyta init --provider <codex|cursor|claude>
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
breyta flows show <slug> --source latest
breyta runs start --flow <slug> --source latest --input '{"n":41}' --wait
```

Environment/setup details: `docs/agentic-chat.md`.

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
