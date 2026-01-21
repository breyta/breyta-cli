## Agentic chat setup (Claude Code, Cursor, Codex, etc.)

The only hard requirements for an agentic tool are:
- it can execute a local CLI command
- the `breyta` binary is on `PATH`
- for local development: dev mode is enabled and the environment variables are set (or the agent passes flags)

By default, `breyta` targets the production API and does not expose `--api` / `--token` overrides unless dev mode is enabled.

### Common environment (recommended)

```bash
export BREYTA_DEV=1
export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
```

Sanity check:

```bash
breyta workspaces current --pretty
```

### Local API keys / secrets (for flows-api + OAuth)

Flows execute **inside `flows-api`**, so any API keys you want flows to use must be available to the server (not just in your shell).

- **Option A (recommended for per-user / production-like behavior)**: declare `:requires` slots and bind credentials via the UI or CLI bindings (creates a profile). Slot names must be non-namespaced keywords (e.g., `:api`, not `:ns/api`). You can also include activation-only inputs with `{:kind :form ...}` (available under `:activation` at run time).
- **Manual trigger fields / wait notify fields**: `:fields` items use non-namespaced keyword names (e.g., `{:name :user-id ...}`).
- Generate a template: `breyta flows bindings template <slug> --out profile.edn` (prefills current `:conn` bindings; use `--clean` for a blank template)
- Apply bindings with a profile file: `breyta flows bindings apply <slug> @profile.edn`
- Enable prod profile: `breyta flows activate <slug> --version latest`

- **Option B (local dev / server-global)**: create a local `secrets.edn` file (gitignored)
  - Copy template: `cp breyta/secrets.edn.example secrets.edn`
  - Fill in the keys you need (OpenAI/Anthropic, OAuth client IDs/secrets, etc.)
  - Restart `flows-api` after changing `secrets.edn`
  - Never commit `secrets.edn`

- **Option C (CLI/env only)**: set CLI env vars for talking to the server
  - `BREYTA_API_URL`, `BREYTA_WORKSPACE`, `BREYTA_TOKEN`
  - These are for authenticating the CLI to `flows-api`, **not** for providing third-party API keys to flow executions.

For HTTP integrations that require API keys, the intended path is to declare a connection slot in `:requires` and then enter the key / complete OAuth via the `flows-api` activation UI (it stores secrets server-side). Activation-only inputs live under `:activation`.

### Claude Code (Anthropic skills)

This repo includes a skill bundle at `breyta-cli/skills/breyta/`.

Copy it to:
- `~/.claude/skills/breyta/`

Example:

```bash
mkdir -p ~/.claude/skills/breyta
rsync -a breyta-cli/skills/breyta/ ~/.claude/skills/breyta/
```

You can also install it directly from the CLI/TUI:
- TUI: press `s` → pick an install target
- CLI: `breyta skills install --provider claude`

### Cursor / Codex / “generic agent”

Many tools don’t automatically ingest Anthropic skill bundles.

Use this approach instead:
1) ensure `breyta` is installed and on PATH (see `docs/install.md`)
2) ensure `flows-api` is running (see `docs/flows-api-local.md`)
3) paste the snippet below into whatever place your tool uses for project instructions / system prompt / agent guidelines.

Snippet to paste:

```text
This project has a local flow-authoring CLI.

- Start the server (from breyta/): ./scripts/start-flows-api.sh --emulator --auth-mock
- breyta CLI calls flows-api over HTTP (dev mode only):
  - BREYTA_DEV=1
  - BREYTA_API_URL=http://localhost:8090
  - BREYTA_WORKSPACE=ws-acme
  - BREYTA_TOKEN=dev-user-123

Use these commands to manage flows:
- breyta flows list
- breyta flows pull <slug> --out ./tmp/flows/<slug>.clj
- edit file
- breyta flows push --file ./tmp/flows/<slug>.clj
- breyta flows deploy <slug>

End-user installations (flows tagged `:end-user`):
- breyta flows installations create <slug> --name "My installation"
- breyta flows installations set-inputs <profile-id> --input '{"region":"EU"}'
- breyta flows installations enable <profile-id>   # activate
- breyta flows installations disable <profile-id>  # pause

To run a flow and see output:
- breyta runs start --flow run-hello --input '{"n":41}' --wait
- (as an installation) breyta runs start --flow <slug> --profile-id <profile-id> --input '{"x":1}' --wait
- read output at: data.run.resultPreview.data.result

Notes for agents:
- If a flow declares `:requires` slots, it needs bindings + activation (use `breyta flows bindings apply <slug> @profile.edn`, then `breyta flows activate <slug> --version latest`).
- Draft preview runs use draft bindings: `http://localhost:8090/<workspace>/flows/<slug>/draft-bindings` (or `breyta flows draft-bindings-url <slug>`), then run with `breyta runs start --flow <slug> --source draft`.
- Flow bodies are intentionally constrained (SCI sandbox / orchestration DSL). Put transformations into `:function` steps (`:code` alias).
- `--input` JSON keys arrive as strings, but runtime normalizes input so keyword lookups/destructuring work too.
```

### Troubleshooting

- If `flows-api` port is busy, stop the server (Ctrl+C) and re-run.
- If a flow has drafts but no deployed version yet, `breyta flows show <slug>` may return `no_active_version`; use `--source latest`.
