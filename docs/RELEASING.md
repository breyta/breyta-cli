# Releasing the canonical Breyta CLI

## Current status: publication disabled

The `open-source/engine` branch must not publish CLI binaries, GitHub releases,
Homebrew updates, or Go-module compatibility tags yet. Existing releases and
`@latest` belong to the retiring hosted product.

There is deliberately no active release workflow under `.github/workflows`.
The reviewed release draft is stored at
`release/release.workflow.yml.disabled`, outside GitHub's workflow discovery
path and with a non-YAML extension. Branch pushes run no workflow; pull
requests run test/build validation only. The Parinfer rebuild workflow is
manual and uploads workflow artifacts, not releases. Pull requests also build
all five CLI archives as snapshots, derive target-specific notices and SPDX
SBOMs from those final artifacts, add the verified locked Cargo graph that
cannot be inferred from the opaque helper binary, verify bundled Parinfer
hashes and provenance, and retain only compliance evidence as a workflow
artifact.

Do not move the disabled workflow into `.github/workflows`, create a release
tag, update Homebrew, or create compatibility tags as part of ordinary engine
development.

## Release-enablement gate

Publication requires a separate reviewed change that records all of these
decisions:

- the approved source-available license and trademark policy;
- release owner and signing/provenance owner;
- canonical version and tag namespace, distinct from retired hosted releases;
- approved GitHub and package-manager distribution targets;
- branch/ref restrictions proving only an approved `open-source/engine`
  release commit can publish;
- required reviewers or protected environment for the publishing job;
- final archive tests, SBOMs, notices, checksums and Parinfer provenance;
- rollback and revocation procedure.

That change may use `release/release.workflow.yml.disabled` as a reviewed
starting point, but must define an explicit intentional trigger. Restoring the
old tag-on-push behavior is not sufficient.

## Local artifact validation

Maintainers can validate the packaging configuration without publishing:

```bash
go test ./...
go build ./...
go vet ./...
./scripts/verify-parinfer-provenance.sh
./scripts/prepare-release-compliance.sh
go run github.com/goreleaser/goreleaser/v2@v2.17.1 check
```

The full cross-platform release-candidate gate runs in pull-request CI because
it pins both GoReleaser and Syft. It invokes GoReleaser with
`--snapshot --skip=publish`, then runs
`./scripts/verify-release-artifacts.sh`. Neither the local commands nor the CI
gate creates a GitHub release or updates an external distribution channel.

CI also checks out upstream `parinfer-rust` by its `v0.4.3` tag, proves that it
resolves to the pinned commit, compares the retained `Cargo.toml`, `Cargo.lock`,
and ISC license byte-for-byte, and checks every locked Cargo package against
the committed license inventory. The separate manual rebuild workflow uses the
same immutable commit and Rust toolchain and only uploads workflow artifacts.
