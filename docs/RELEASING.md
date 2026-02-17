# Releasing `breyta-cli`

This is the maintainer runbook for cutting a new `breyta-cli` release.

## Versioning

- Tag format is CalVer: `vYYYY.M.PATCH` (example: `v2026.2.2`).
- The release workflow only triggers for tags matching this pattern.

## Prerequisites

- You have push access to `breyta/breyta-cli`.
- You have a sibling checkout of `breyta/` next to `breyta-cli/` (required by integration tests).
- Your local branch is clean and up to date with `origin/main`.

## 1) Run release checks locally

From `breyta-cli/`:

```bash
make release-check
```

What this runs:

- `gofmt -w` on tracked Go files
- `go test ./...`
- CLI integration test harness via `../breyta/bases/flows-api/scripts/integration_tests.sh`

If the integration harness needs overrides, see:

- `breyta/bases/flows-api/docs/internal/INTEGRATION_TESTS.md`

## 2) Pick the next tag

List existing tags:

```bash
git tag --list 'v*' --sort=version:refname | tail -n 20
```

Pick the next `vYYYY.M.PATCH` value for the release.

## 3) Create and push the tag

Create an annotated tag on the release commit (normally `main`):

```bash
git tag -a vYYYY.M.PATCH -m "breyta-cli vYYYY.M.PATCH"
git push origin vYYYY.M.PATCH
```

## 4) Verify GitHub release automation

Pushing the tag triggers `.github/workflows/release.yml`, which runs GoReleaser.

Expected outputs:

- GitHub release with archives and checksums
- Homebrew tap update to `breyta/homebrew-tap` (Formula `breyta`)

Watch:

- Actions: <https://github.com/breyta/breyta-cli/actions/workflows/release.yml>
- Releases: <https://github.com/breyta/breyta-cli/releases>

## Troubleshooting

- If `make release-check` fails in integration setup, inspect:
  - `breyta/tmp/it/flows_api.log`
  - `breyta/tmp/flows-api.log`
- You can run the integration script directly for faster iteration:

```bash
cd ../breyta
./bases/flows-api/scripts/integration_tests.sh
```
