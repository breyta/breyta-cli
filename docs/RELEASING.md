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
manual and uploads workflow artifacts, not releases.

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
go run github.com/goreleaser/goreleaser/v2@v2.17.1 check
```

These commands produce no GitHub release and update no external distribution
channel.
