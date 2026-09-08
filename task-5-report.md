# Task 5 report

## Status

Implemented release SHA256 checksums, archive SBOM generation, and pinned GitHub
OIDC build-provenance attestations for stable and prerelease provider releases.
Windows remains in the build matrix under the neutral `build-provider-windows`
ID, with signing hooks and the no-op Make target removed.

## Verification

- `go test ./provider -run 'Release|Workflow' -count=1` — PASS
- `go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.yml` — PASS
- `go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.prerelease.yml` — PASS
- `git diff --check` — PASS

The GoReleaser v2 validation required the current `version: 2` schema,
`changelog.disable`, and `snapshot.version_template`; those schema updates are
included in both configurations.

## Security notes

The attestation action is pinned to commit
`62fc1d596301d0ab9914e1fec14dc5c8d93f65cd` (v3.2.0). Only the provider
`publish` jobs receive `id-token: write` and `attestations: write`; SDK
publishing credentials are not passed to the attestation step.

## Concern

The repository does not have a `goreleaser` or `mise` executable available
locally, so validation used the Go toolchain's automatic Go 1.27.1 toolchain
download with GoReleaser v2.18.1.
