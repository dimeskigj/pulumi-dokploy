# Registry Release Readiness Design

## Goal

Prepare Pulumi Dokploy for a technically publishable Pulumi Registry release while hardening release integrity and deferring decisions that require maintainer identity, visual assets, or broader product scope.

The current repository and publisher coordinates are final for this work:

- Repository: `dimeskigj/pulumi-dokploy`
- Provider name: `dokploy`
- Publisher: `dimeskigj`

## Scope

This work includes runtime identification, package download metadata, Registry documentation, nonvisual discovery metadata, licensing, schema compatibility enforcement, release SBOM/provenance, and clean-consumer release checks.

It does not submit the Registry pull request or publish a release.

## Runtime Identity

Dokploy API requests must send a provider-specific User-Agent containing the provider version:

```text
pulumi-dokploy/<version>
```

`provider.Version` is linker-injected for release builds. Empty or whitespace-only versions use a deterministic development fallback rather than producing an invalid or empty product version. The value is passed into the client constructor instead of introducing a provider package import into `internal/client`.

The request editor sets the User-Agent on every generated-client request. Tests verify release and development values and confirm that retry attempts preserve the same header.

## Package Metadata

Provider schema metadata will include:

- `pluginDownloadURL` targeting GitHub Releases for `dimeskigj/pulumi-dokploy`.
- Keywords: `category/infrastructure`, `kind/native`, `dokploy`, `deployment`, `self-hosted`, and `paas`.

The schema and all generated SDKs will be regenerated. Clean-cache tests must demonstrate that generated SDK metadata points Pulumi at the GitHub-hosted provider artifacts.

`logoUrl` is deferred because selecting or creating a visual asset requires maintainer input.

## Registry Documentation

Add `docs/_index.md` with valid YAML front matter and content suitable for Pulumi Registry ingestion. It will contain:

- Provider overview and community-maintained status.
- Installation using released packages and automatic plugin acquisition.
- Endpoint and secret API-key configuration.
- A minimal example.
- Links to repository support surfaces without inventing contact information.

Add `docs/installation-configuration.md` with installation commands for every supported language, YAML package usage, configuration options, environment variables, and secret-handling guidance.

The documents use current package coordinates and do not claim Dokploy or Pulumi ownership, endorsement, or official maintenance.

## Legal Baseline

Replace the abbreviated root `LICENSE` with the canonical complete Apache License 2.0 text. Preserve the existing Apache-2.0 package metadata.

No `NOTICE` file will be invented without identified attribution requirements.

## Schema Compatibility CI

Schema compatibility comparison must run in pull-request CI after schema generation. It must produce a useful report and fail the required check when incompatible changes are detected unless a future explicit major-version policy permits them.

Dead pull-request-only schema steps will be removed from stable and prerelease tag workflows because those workflows never receive pull-request events. Workflow contract tests will enforce that comparison remains in PR CI and absent from tag-only workflows.

## Release Integrity

Provider releases will retain SHA256 checksums and add:

- An SPDX or CycloneDX SBOM generated from release artifacts.
- GitHub artifact attestations using OIDC and least-privilege permissions.
- Verification that release archives use Pulumi's expected names.

The no-op Windows signing hook and misleading signing build identifier will be removed or renamed. This work will not introduce long-lived signing keys or claim Authenticode signing.

GoReleaser and release actions will remain version- or commit-pinned. Stable and prerelease paths must apply equivalent integrity controls.

## Consumer Smoke Verification

Add a post-release or manually dispatched workflow accepting an exact released version. It uses fresh temporary package and Pulumi plugin caches and verifies:

- Provider archive download and checksum validation.
- Provider plugin execution and schema loading.
- Exact npm, PyPI, NuGet, Maven Central, and Go module versions can be installed by clean consumer projects.
- Generated package metadata causes Pulumi to acquire the matching provider plugin automatically.

The workflow must not use local SDK paths, silently fall back to a preinstalled plugin, or publish artifacts. Ecosystem checks may be separate jobs so one package's failure is attributable.

## Error Handling And Security

- User-Agent construction must not contain API keys, endpoints, stack data, or user input.
- CI logs must not print package-registry credentials or Pulumi configuration secrets.
- Artifact attestation uses GitHub OIDC rather than stored signing credentials.
- Package smoke tests use public package registries and released artifacts only.

## Testing

Tests will cover:

- Exact User-Agent headers for release and development builds.
- Header preservation across retries.
- Required schema download and keyword metadata.
- Generated SDK plugin metadata.
- Registry documentation front matter, installation, configuration, examples, and community-maintained wording.
- Complete Apache license markers.
- PR schema compatibility workflow placement and failure behavior.
- Stable/prerelease checksum, SBOM, and attestation contracts.
- Absence of no-op signing claims.
- Clean consumer smoke workflow cache isolation and exact-version inputs.

Final verification includes provider tests, race tests, documentation checks, schema/SDK drift checks, workflow contract tests, and release-configuration validation where the pinned project tools are available.

## Deferred Items

The following remain explicit follow-up work:

- Logo asset and `logoUrl`.
- Security email or private reporting contact.
- Maintainer display names and Registry publisher-name mapping.
- CODEOWNERS and governance ownership assignments.
- Code of Conduct enforcement contact.
- Pulumi Registry pull request and release publication.
- Lookup functions.
- Broad Pulumi-engine acceptance expansion.
- Upstream Dokploy changes.

The readiness assessment will be updated to show completed and deferred items without marking the provider Registry-ready until required maintainer inputs and a corrected release exist.
