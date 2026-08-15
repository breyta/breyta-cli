# Building `parinfer-rust` binaries (manual, via GitHub Actions)

Maintainer-only notes for updating the vendored `parinfer-rust` binaries.

We bundle a pinned `parinfer-rust` executable alongside `breyta` release
artifacts so end users do not need extra tooling installed.

This is intentionally **manual**: build once, commit the binaries into this repo
under `tools/parinfer-rust/`, and keep them stable until the next planned
refresh.

## Build in GitHub Actions

1. Update `SOURCE.lock`, the retained Cargo inputs, notices, and the workflow's
   immutable commit/toolchain together in a reviewed pull request.
2. Go to GitHub Actions → **Build pinned parinfer-rust** and click **Run
   workflow**. The workflow accepts no source-ref input.
3. Wait for the workflow to finish and download the commit-named artifacts.

## Copy into the vendored layout

Unzip each artifact and copy the contained binaries into:

- `tools/parinfer-rust/darwin/amd64/parinfer-rust`
- `tools/parinfer-rust/darwin/arm64/parinfer-rust`
- `tools/parinfer-rust/linux/amd64/parinfer-rust`
- `tools/parinfer-rust/linux/arm64/parinfer-rust`
- `tools/parinfer-rust/windows/amd64/parinfer-rust.exe`

Commit the binaries and record the chosen upstream ref in a release note or
maintainer note.

## Local Dev Note

If a developer builds `breyta` locally (without vendored binaries present), the CLI will:

1. prefer a sibling `parinfer-rust` next to `breyta` (release/brew)
2. fall back to `parinfer-rust` on `PATH` (e.g. `cargo install parinfer-rust`)
3. fall back to a built-in best-effort delimiter balancer

Override path:

- `BREYTA_PARINFER_RUST=/path/to/parinfer-rust`
