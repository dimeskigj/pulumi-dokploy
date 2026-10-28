# Changelog

## Unreleased

- Fix `Schedule` create: adopt the `scheduleId` Dokploy assigns instead of a client-generated UUID that Dokploy discards, which left every new schedule partially created and failing `schedule.update` with HTTP 404.
- Update the pinned Go toolchain to 1.26.9 and golang.org/x/net to v0.60.0 to address disclosed vulnerabilities.
- Document twelve read-only ID lookup functions and their non-secret metadata results.
- Initial Pulumi Dokploy provider MVP and Registry release setup.
- Document the Notification resource, all twelve channels, secure examples, import and recovery guidance.
