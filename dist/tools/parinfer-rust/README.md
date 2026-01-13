# Vendored `parinfer-rust` binaries

This directory is for **prebuilt `parinfer-rust` CLI binaries** that we bundle alongside `breyta` in release archives and Homebrew installs.

Why: end users typically only install `breyta`, but we want SOTA delimiter repair (Parinfer) available without extra installs.

## Layout

Place binaries here (per target):

- `dist/tools/parinfer-rust/darwin/amd64/parinfer-rust`
- `dist/tools/parinfer-rust/darwin/arm64/parinfer-rust`
- `dist/tools/parinfer-rust/linux/amd64/parinfer-rust`
- `dist/tools/parinfer-rust/linux/arm64/parinfer-rust`
- `dist/tools/parinfer-rust/windows/amd64/parinfer-rust.exe`

The release config (`.goreleaser.yaml`) includes the matching file into the archive as `parinfer-rust` next to the `breyta` binary, and Homebrew installs it into `bin/`.

## How it’s used

The CLI prefers the bundled binary (sibling `parinfer-rust` next to `breyta`), then falls back to `PATH` (for local dev via `cargo install parinfer-rust`), and finally falls back to a built-in best-effort delimiter balancer.

Env override:

- `BREYTA_PARINFER_RUST=/path/to/parinfer-rust`

