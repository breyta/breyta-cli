## Onboarding (agent-first)

If you’re using Breyta with a coding agent (Codex, Claude Code, Cursor, etc.), the intended setup is:

1) Install the `breyta` CLI
2) Install the Breyta agent skill bundle for your agent tool
3) Authenticate the CLI to your Breyta account
4) Let your agent author flows by calling `breyta` commands

### Recommended: `breyta init`

`breyta init` is a single command that:
- installs the skill bundle for your agent tool
- creates a local workspace directory with an `AGENTS.md` file and a suggested folder layout

Example:

```bash
breyta init --provider codex
breyta auth login
breyta workspaces list
breyta flows list
```

### Copy/paste snippet (for your agent)

Paste this into your agent tool as the “task” / “instructions” and let the agent guide you:

```text
You are my Breyta onboarding assistant.

Goal: install the Breyta CLI, install the Breyta agent skill bundle, authenticate, then help me create and run my first flow.

Please:
1) Check whether `breyta` is installed by running: `breyta --version`.
2) If it isn’t installed, help me install it:
   - macOS: `brew tap breyta/tap && brew install breyta`
   - otherwise: download the latest release from https://github.com/breyta/breyta-cli/releases and put `breyta` on my PATH
   - (alternative, if Go is installed): `go install github.com/breyta/breyta-cli/cmd/breyta@latest`
3) Ask which agent tool I’m using (Codex, Cursor, or Claude Code) and run:
   - `breyta init --provider <codex|cursor|claude>`
4) Authenticate (and create an account if needed): `breyta auth login`
5) Verify the setup:
   - `breyta workspaces list`
   - `breyta flows list`
6) Help me build my first flow using the CLI loop:
   - `breyta flows pull <slug> --out ./flows/<slug>.clj`
   - edit `./flows/<slug>.clj`
   - `breyta flows push --file ./flows/<slug>.clj`
   - `breyta flows validate <slug>`
   - `breyta flows deploy <slug>`
   - `breyta runs start --flow <slug> --source latest --input '{"n":41}' --wait`
7) Use `breyta docs <command...>` whenever you’re unsure about a command or flags.

Prefer small, incremental changes and verify each step before moving on.
```

### Notes

- `breyta init` creates `./breyta-workspace/` by default. You can change that with `--dir`.
- Many agent tools only read `AGENTS.md` when the folder is opened as the active project/workspace (or when a new agent session starts in that directory).
- If your agent supports “skills”, make it reliable by explicitly referencing the Breyta skill file path in your project/root instructions (for example in `AGENTS.md`) so the agent always knows to read it when working with Breyta.
- If you only want the skill bundle, you can run `breyta skills install --provider <codex|cursor|claude>`.
