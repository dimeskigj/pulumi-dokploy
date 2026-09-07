# Pulumi Registry Readiness Assessment

Date: 2026-09-07

## Summary

Pulumi Dokploy is a strong community provider with 18 resources, generated SDKs for five languages, CI and release automation, secret-aware schema metadata, and substantial unit and live API coverage. It is not yet ready for publication in the public Pulumi Registry.

This document distinguishes a community package listed in the Pulumi Registry from a Pulumi-maintained official provider. Registry publication is achievable through repository work and a Registry pull request. Pulumi-maintained status requires direct coordination with Pulumi and likely Dokploy through Pulumi's partner process.

## Registry Blockers

### Required Registry overview

The repository needs `docs/_index.md` with YAML front matter. Pulumi's Registry onboarding instructions identify this as the only required handwritten page. The existing Astro site does not replace it.

Add an overview that includes:

- What the provider manages.
- Installation instructions that download a released plugin.
- Endpoint and API-key configuration.
- A minimal example.
- Support and community-maintenance status.

Pulumi also recommends `docs/installation-configuration.md`.

### Provider-specific User-Agent

`internal/client/client.go` sends the API key and `Accept` headers but does not identify the provider. Pulumi requires a provider-specific User-Agent so vendor traffic can be attributed correctly.

Send a stable value such as `pulumi-dokploy/<version>`, with a development fallback, and verify that retries preserve it.

### Plugin download metadata

The provider schema lacks `pluginDownloadURL`. Generated SDKs therefore cannot reliably install the provider automatically, and the README requires a separate manual plugin installation.

Add the canonical GitHub plugin source, regenerate the schema and all SDKs, and test installation from clean Pulumi plugin caches.

### Public Registry registration

The provider is not in `pulumi/registry`'s community package list. After publishing a corrected release, submit an entry with:

```json
{
  "repoSlug": "dimeskigj/pulumi-dokploy",
  "schemaFile": "provider/cmd/pulumi-resource-dokploy/schema.json"
}
```

Because this is a new publisher, the Registry pull request may also need a publisher display-name entry.

## Production Correctness

### Backup create identity

`backup.create` returns no backup ID. The provider currently compares target backup IDs immediately before and after creation. Concurrent creation or delayed visibility can make that comparison empty or ambiguous after the remote create has already succeeded.

The approved remediation is documented in `docs/superpowers/specs/2026-09-07-backup-create-identity-design.md`.

### Engine-level acceptance breadth

Direct provider and live API tests are extensive, but the Pulumi engine lifecycle smoke test covers only Project, Environment, Tag, and ProjectTag. Add engine-level coverage for representative workloads, databases, domains, credentials, mounts, and backups, including import, refresh, replacement, secrets, and interrupted operations.

## Recommended Improvements

### Lookup functions

The provider exposes resources but no functions. Lookup functions are not a Registry requirement, but they improve adoption of existing Dokploy resources. Initial candidates are project, environment, application, compose, database, destination, registry, SSH key, and tag lookups.

### Schema discovery metadata

Add a web-accessible `logoUrl` and keywords including:

- `category/infrastructure`
- `kind/native`
- `dokploy`
- `deployment`
- `self-hosted`
- `paas`

### Schema compatibility enforcement

Pull-request CI installs `schema-tools`, but schema comparison currently appears in tag-oriented release workflows. Run compatibility comparison in pull-request CI so incompatible changes cannot silently ship in minor or patch releases.

### Release integrity

Release archives have broad platform coverage and checksums, but signing, provenance, and SBOM generation are absent. The Windows signing hook is currently a no-op. Add artifact attestations or Sigstore signing and an SPDX or CycloneDX SBOM, or remove misleading signing terminology.

### Repository policy metadata

- Replace the abbreviated `LICENSE` with the complete Apache License 2.0 text.
- Add an actionable private vulnerability-reporting route to `SECURITY.md`.
- Add ownership and support policy files such as `CODEOWNERS`, `MAINTAINERS.md`, and `SUPPORT.md`.
- Use a project-controlled Code of Conduct enforcement contact.

### Published-package smoke tests

Local examples use local SDK paths. Add post-release tests that install exact published npm, PyPI, NuGet, Maven Central, and Go module versions with an empty plugin cache and verify automatic plugin acquisition.

## Verification Evidence

The following checks passed on 2026-09-07:

- `make test_provider`
- `go test -race ./provider/... ./internal/...`
- `make docs_check`
- `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`

The vulnerability scan found no reachable vulnerabilities. Website installation reported three high-severity dependency advisories that require separate `npm audit` review, while all website checks and builds passed.

The following checks could not run locally because their tools were unavailable:

- `make lint`: `golangci-lint` was missing.
- `make check_openapi`: `mise` was missing.

Live Dokploy acceptance tests were not run during this assessment because explicit acceptance credentials and opt-in were not enabled.

## Recommended Sequence

1. Correct backup creation identity handling.
2. Add the User-Agent.
3. Add plugin download, logo, and keyword metadata and regenerate SDKs.
4. Add `docs/_index.md` and publish a corrected release.
5. Verify clean-cache installation for every SDK.
6. Submit the Pulumi Registry pull request.
7. Expand engine-level acceptance coverage and add lookup functions.
8. Harden release integrity and repository policy metadata.
