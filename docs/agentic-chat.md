## Agentic chat setup (Claude Code, Cursor, Codex, etc.)

The only hard requirements for an agentic tool are:
- it can execute a local CLI command
- the `breyta` binary is on `PATH`
- the environment variables are set (or the agent passes flags)

### Common environment (recommended)

```bash
export BREYTA_API_URL="http://localhost:8090"
export BREYTA_WORKSPACE="ws-acme"
export BREYTA_TOKEN="dev-user-123"
```

### Claude Code (Anthropic skills)

This repo includes a skill bundle at `breyta/skills/breyta-flows-cli/`.

Copy it to:
- `~/.claude/skills/user/breyta-flows-cli/`

Example:

```bash
mkdir -p ~/.claude/skills/user/breyta-flows-cli
rsync -a breyta/skills/breyta-flows-cli/ ~/.claude/skills/user/breyta-flows-cli/
```

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
- breyta CLI calls flows-api over HTTP:
  - BREYTA_API_URL=http://localhost:8090
  - BREYTA_WORKSPACE=ws-acme
  - BREYTA_TOKEN=dev-user-123

Use these commands to manage flows:
- breyta flows list --pretty
- breyta flows pull <slug> --out ./tmp/flows/<slug>.clj --pretty
- edit file
- breyta flows push --file ./tmp/flows/<slug>.clj --pretty
- breyta flows deploy <slug> --pretty

To run a flow and see output:
- breyta --dev runs start --flow run-hello --input '{"n":41}' --wait --pretty
- read output at: data.run.resultPreview.data.result
```

### Troubleshooting

- If `flows-api` port is busy, stop the server (Ctrl+C) and re-run.
- If a flow has drafts but no deployed version yet, `breyta flows show <slug>` may return `no_active_version`; use `--source latest`.
