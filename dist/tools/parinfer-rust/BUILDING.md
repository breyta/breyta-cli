# Building `parinfer-rust` binaries (manual, via GitHub Actions)

We bundle a pinned `parinfer-rust` executable alongside `breyta` release artifacts so end users don’t need extra tooling installed.

This is intentionally **manual**: we build once, commit the binaries into this repo under `dist/tools/parinfer-rust/`, and then keep them stable.

## Build in GitHub Actions

1. Go to GitHub Actions → **Build parinfer-rust (manual)**.
2. Click **Run workflow**.
3. Provide `ref` (tag or commit SHA), e.g. `v0.4.3`.
4. Wait for the workflow to finish and download the artifacts:
   - `parinfer-rust-linux-<ref>`
   - `parinfer-rust-darwin-<ref>`
   - `parinfer-rust-windows-<ref>`

## Copy into the vendored layout

Unzip each artifact and copy the contained binaries into:

- `dist/tools/parinfer-rust/darwin/amd64/parinfer-rust`
- `dist/tools/parinfer-rust/darwin/arm64/parinfer-rust`
- `dist/tools/parinfer-rust/linux/amd64/parinfer-rust`
- `dist/tools/parinfer-rust/linux/arm64/parinfer-rust`
- `dist/tools/parinfer-rust/windows/amd64/parinfer-rust.exe`

Commit the binaries (and ideally also record the chosen ref somewhere, e.g. in a release note or PR description).

## Local dev note

If a developer builds `breyta` locally (without vendored binaries present), the CLI will:

1. prefer a sibling `parinfer-rust` next to `breyta` (release/brew)
2. fall back to `parinfer-rust` on `PATH` (e.g. `cargo install parinfer-rust`)
3. fall back to a built-in best-effort delimiter balancer

Override path:

- `BREYTA_PARINFER_RUST=/path/to/parinfer-rust`

