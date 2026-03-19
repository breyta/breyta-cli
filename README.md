# Breyta CLI (`breyta`)

`breyta` is the command-line interface for working with Breyta flows from coding
agents and terminals.

This repository is public so you can inspect what runs on your machine, build
the CLI yourself, and understand how it talks to the Breyta API. The code is
available under the MIT License. In practice, this repo is the client for
Breyta, so most usage happens inside a Breyta workspace rather than as a
standalone general-purpose tool.

## Install

Choose one:

- Homebrew (macOS):
  - `brew tap breyta/tap`
  - `brew install breyta`
- Prebuilt binaries:
  - https://github.com/breyta/breyta-cli/releases
- Go install:
  - `go install github.com/breyta/breyta-cli/cmd/breyta@latest`
- From source checkout:
  - `go install ./cmd/breyta`

Verify the install:

```bash
breyta version
```

## Getting started

The usual path is:

1. Install the CLI
2. Bootstrap a local agent workspace
3. Authenticate
4. Pick a workspace
5. Start working with flows

```bash
breyta init --provider <codex|cursor|claude|gemini>
breyta auth login
breyta workspaces list
breyta workspaces use <workspace-id>
breyta flows list
```

`breyta init` installs the Breyta skill bundle for your agent tool and creates a
local `breyta-workspace/` directory with an `AGENTS.md` file and flow folders.

If you only want the skill bundle and not the workspace files:

```bash
breyta skills install --provider <codex|cursor|claude|gemini>
```

## First Workflow

Pull a flow into a local workspace, edit it, push it back to draft, and run it:

```bash
breyta flows pull <slug> --out ./flows/<slug>.clj
breyta flows push --file ./flows/<slug>.clj
breyta flows configure check <slug>
breyta flows run <slug> --wait
```

When the draft behavior is correct, release the latest pushed version once:

```bash
breyta flows release <slug>
breyta flows show <slug> --target live
breyta flows run <slug> --target live --wait
```

Inspect runs with the same structured filter syntax as the web runs list:

```bash
breyta runs list --query 'status:failed flow:<slug>'
breyta runs list --installation-id <profile-id> --version 7
```

## Provenance

If a new flow was derived from existing flows, keep that lineage in flow metadata
instead of overloading `created-by`.

- `breyta flows show <slug>` and `breyta flows pull <slug>` record consulted flows
  inside an initialized agent workspace.
- Persist curated source refs with:
  - `breyta flows provenance set <slug> --from-consulted`
  - `breyta flows provenance set <slug> --source <workspace-id>/<flow-slug>`
  - `breyta flows provenance set <slug> --template <template-slug>`
- Remove provenance intentionally with:
  - `breyta flows provenance set <slug> --clear`

## Docs And Help

- Product docs:
  - https://flows.breyta.ai/docs
  - `breyta docs`
  - `breyta docs find "<query>"`
  - `breyta docs show <slug>`
- Command usage:
  - `breyta help <command...>`

## Recovery URLs

When Breyta already knows the manual fix page, open the exact URL from the CLI
response instead of reconstructing it:

- Failures: `error.actions[].url` first, then `meta.webUrl`
- Successful reads/runs: `meta.webUrl` / `data.*.webUrl`
- Only derive canonical URLs when the required ids are already known: billing,
  activate, draft-bindings, installation, or connection edit

## Updates

Check for a newer release:

```bash
breyta upgrade
```

Apply an update automatically when supported:

```bash
breyta upgrade --apply
```

Open the latest release page:

```bash
breyta upgrade --open
```

## Repository Policy

This repo is public for transparency and inspectability. We are not accepting
pull requests or code contributions at this time.

- General help, bugs, and feedback: `hello@breyta.ai`
- In-product feedback: `breyta feedback send`
- Security reporting: see `SECURITY.md`

## Maintainer Docs

These docs stay in the repo for Breyta maintainers, but they are not part of
the main user path:

- Release runbook: `docs/RELEASING.md`
- Bundled `parinfer-rust` notes: `tools/parinfer-rust/README.md`
- Bundled `parinfer-rust` build steps: `tools/parinfer-rust/BUILDING.md`

## License

- Repo license: `LICENSE`
- Third-party notices: `THIRD_PARTY_NOTICES.md`
