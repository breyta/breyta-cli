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
4. Verify your account and workspace summary
5. Discover approved examples to copy from
6. Pick a workspace later when you are ready to adopt or build

```bash
breyta init --provider <codex|cursor|claude|gemini>
cd breyta-workspace
breyta auth login
breyta auth whoami
breyta flows search "<idea>"
```

To browse public installable end-user flows for your current workspace instead,
use:

```bash
breyta flows discover list
breyta flows discover search "<idea>"
```

`breyta init` installs the Breyta skill bundle for your agent tool and creates a
local `breyta-workspace/` directory with an `AGENTS.md` file and flow folders.

If `breyta auth whoami` shows multiple workspaces or no default workspace
selected, use:

```bash
breyta workspaces list
breyta workspaces use <workspace-id>
```

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

When the draft behavior is correct, inspect draft-vs-live changes and release once with a markdown note:

```bash
breyta flows diff <slug>
breyta flows release <slug> --release-note-file ./release-note.md
breyta flows show <slug> --target live
breyta flows run <slug> --target live --wait
```

If the flow should appear in public discover/install surfaces, make that explicit:

```bash
# Tag it as end-user installable
breyta flows update <slug> --tags 'end-user,...'

# Either author this in the flow file:
# :discover {:public true}
#
# Or set it explicitly after push/release:
breyta flows discover update <slug> --public=true
```

Public discover visibility is stored flow metadata. A released version and the
`end-user` tag are both required before the flow can be exposed in discover.
This discover catalog is separate from `breyta flows search`, which is only for
approved example flows to inspect and copy from.

Archive or remove flows intentionally:

```bash
# Hide a flow from normal active use while preserving versions and metadata
breyta flows archive <slug>

# Permanently remove a flow definition
breyta flows delete <slug> --yes

# If the flow still has runs/installations to clean up, force the delete
breyta flows delete <slug> --yes --force
```

Inspect runs with the same structured filter syntax as the web runs list:

```bash
breyta runs list --query 'status:failed flow:<slug>'
breyta runs list --installation-id <profile-id> --version 7
```

If you need to revise the note later:

```bash
breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md
```

## Docs And Help

- Product docs:
  - https://flows.breyta.ai/docs
  - `breyta docs`
  - `breyta docs find "<query>"`
  - `breyta docs show <slug>`
- Command usage:
  - `breyta help <command...>`

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
