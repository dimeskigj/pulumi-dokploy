# Changelog

## Unreleased

## 0.4.1 — 2026-10-28

### Fixed

- Fix `Schedule` creation to adopt Dokploy's server-assigned `scheduleId`, instead of a discarded client-generated UUID that left schedules partially created and subsequent updates failing with HTTP 404. Existing affected resources still require manual state repair or re-import using the actual Dokploy schedule ID.

### Added — Beta

- GitLab, GitHub (import-only), Gitea, and Bitbucket integration resources.
- GitHub, Gitea, and Bitbucket Application sources.
- Heroku Buildpacks, Paketo Buildpacks, Static, and Railpack builders.
- Application build options `herokuVersion`, `railpackVersion`, `publishDirectory`, and `isStaticSpa`.
- Application `autoDeploy` configuration.

## Earlier entries

- Update the pinned Go toolchain to 1.26.9 and golang.org/x/net to v0.60.0 to address disclosed vulnerabilities.
- Document twelve read-only ID lookup functions and their non-secret metadata results.
- Initial Pulumi Dokploy provider MVP and Registry release setup.
- Document the Notification resource, all twelve channels, secure examples, import and recovery guidance.
