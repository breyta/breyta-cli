# Vendored `parinfer-rust` binaries

Maintainer note: this directory holds the prebuilt `parinfer-rust` binaries that
ship alongside `breyta` in release archives and Homebrew installs.

End users normally do not need to manage these files directly. They are bundled
so `breyta` can offer delimiter repair out of the box without requiring a
separate `parinfer-rust` install.

Upstream project: <https://github.com/eraserhd/parinfer-rust>
Upstream license: ISC

## Layout

Place binaries here (per target):

- `tools/parinfer-rust/darwin/amd64/parinfer-rust`
- `tools/parinfer-rust/darwin/arm64/parinfer-rust`
- `tools/parinfer-rust/linux/amd64/parinfer-rust`
- `tools/parinfer-rust/linux/arm64/parinfer-rust`
- `tools/parinfer-rust/windows/amd64/parinfer-rust.exe`

The release config (`.goreleaser.yaml`) includes the matching file into the archive as `parinfer-rust` next to the `breyta` binary, and Homebrew installs it into `bin/`.

## How It Is Used

The CLI prefers the bundled binary (sibling `parinfer-rust` next to `breyta`), then falls back to `PATH` (for local dev via `cargo install parinfer-rust`), and finally falls back to a built-in best-effort delimiter balancer.

Env override:

- `BREYTA_PARINFER_RUST=/path/to/parinfer-rust`
