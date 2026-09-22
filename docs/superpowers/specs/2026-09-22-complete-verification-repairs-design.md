# Complete Verification Repairs Design

## Goal

Diagnose and repair all five failures discovered during full local and live
verification:

1. Application and Compose Domain creation returns `400 BAD_REQUEST`.
2. PostgreSQL mount dispatch ends in a transport or server-health failure.
3. The Pulumi Automation API smoke test fails during stack creation.
4. The tagged example test expects Pulumi YAML strict conversion to reject an
   unknown provider property, but Pulumi 3.259.0 accepts it.
5. `govulncheck` reports reachable GO-2026-6443 through
   `google.golang.org/grpc` 1.83.1.

The repair must preserve the existing uncommitted deterministic cancellation
test changes and must not weaken live cleanup or stop-marker safety.

## Approach

Use evidence-first, issue-by-issue repair. Each failure is reproduced in
isolation, classified at the narrowest relevant boundary, covered by a focused
regression test, and fixed with the smallest causal change. Generated artifacts
are updated only when evidence proves their source contract is stale. The
implementation must not introduce speculative alternate payloads, blind
retries, or broad compatibility fallbacks.

## Diagnostic Safety

Diagnostics may record only:

- operation and lifecycle phase;
- HTTP status class and structured API code;
- request field names, never values;
- process or tool stage and sanitized exit classification;
- cleanup, health-probe, and stop-marker state.

Diagnostics must never print credentials, endpoint values, private URLs,
resource IDs, hostnames, payload values, response bodies, or unsanitized error
objects. Temporary diagnostics are removed before completion. Generally useful
structural diagnostics may remain when covered by source-contract tests.

## Domain Creation

The provider and generated-client create paths are compared serially against
equivalent disposable Application and Compose targets. Evidence includes only
status/code and sorted request-key names. The investigation determines whether
the deployed contract differs in field presence, nullability, enum value, or
target-specific shape.

The repair belongs at the earliest incorrect layer:

- `provider/domain.go` when provider argument-to-request mapping is wrong;
- normalized OpenAPI and generated client output when the source contract is
  wrong;
- the live fixture when the request is valid but its target precondition is
  not.

Provider and generated-client behavior must converge. A successful create must
register cleanup immediately, and an error containing a partial ID must still
trigger verified cleanup. No retry with alternate field names is allowed.

Success requires focused Application and Compose Domain live lifecycles to
create, read, update, import-read, delete, and verify absence.

## PostgreSQL Mount Dispatch

The mount flow is divided into explicit phases: database fixture create,
readiness, mount create, mount read/update, mount delete, fixture delete, and
health verification. Sanitized phase evidence identifies whether the failure
comes from target readiness, target routing, request serialization, operation
timeout, or cleanup ownership.

The fix must preserve lease ordering: the dependent mount is removed and its
absence verified before the PostgreSQL fixture is removed. A target read or
ordinary operation error must not create a stop marker unless an independent
health probe fails or verified cleanup fails.

Success requires the focused PostgreSQL dispatch lifecycle to pass repeatedly
with no stop marker and with both resources confirmed absent afterward.

## Pulumi Automation Smoke

The smoke test reports a sanitized stage classification for workspace setup,
plugin discovery, stack creation, configuration, preview, update, refresh,
destroy, and stack removal. The underlying error text remains hidden unless it
passes existing secret and identifier sanitization rules.

Investigation checks the local provider binary, PATH/plugin discovery,
workspace options, project and stack naming, backend initialization, and
Pulumi CLI version. The repair changes provider packaging or acceptance setup
only where the failed stage proves it necessary. It must not silently skip the
smoke test or convert a failure into a pass.

Success requires the focused smoke lifecycle to create its stack, configure,
preview, update, refresh, destroy, remove the stack, and leave no stop marker.

## Strict YAML Contract

The pinned Pulumi 3.259.0 behavior is treated as the external contract. The
test first distinguishes YAML syntax strictness from provider-schema property
validation. Because this project requires examples to use only declared
provider inputs, unknown-property rejection is tested directly against the
generated provider schema rather than inferred from `pulumi convert --strict`.
The Pulumi conversion test remains responsible for proving that canonical YAML
binds successfully.

The chosen test must fail for a deliberate violation and pass for canonical
YAML. It must not merely remove negative coverage.

## Vulnerability Repair

Raise `google.golang.org/grpc` to at least 1.83.2 in every checked Go module:
the root module, generated Go SDK module, and generated Go example module.
Dependency updates are made through Go module tooling so `go.mod` and `go.sum`
remain consistent. No unrelated dependency upgrades are included unless the Go
resolver requires them.

Success requires `govulncheck ./...` to report no reachable vulnerabilities.

## Testing Strategy

Every behavioral repair follows red-green-refactor:

1. Add or tighten one focused deterministic regression test.
2. Run it and confirm failure for the expected reason.
3. Make the smallest causal change.
4. Run the focused test and adjacent package tests.
5. Run race tests where synchronization or shared state is involved.

Live verification uses fresh absent marker paths and serial execution. Focused
Domain, PostgreSQL mount dispatch, and Pulumi smoke tests run before broader
tiers. An existing incident marker is preserved. If a fresh marker appears,
later heavy tests stop until health and cleanup are independently verified.

Final verification includes:

- all root Go packages;
- provider, internal, and acceptance race tests;
- tagged example tests;
- generated Go, Node.js, Python, .NET, and Java SDK checks;
- website checks, build, and built-output tests;
- `mise exec -- golangci-lint run`;
- generated OpenAPI/schema/SDK checks when their sources change;
- `govulncheck ./...`;
- focused live tests and then all enabled live tiers;
- `git diff --check` and an inspection for unintended generated artifacts.

Optional live cases may skip only for their documented explicit prerequisites.
All enabled cases must pass, all owned resources must be absent after cleanup,
and no new stop marker may remain.

## Scope Boundaries

This work does not add new provider features, redesign resource APIs, delete
unrelated resources, or weaken sanitization and cleanup guarantees. Existing
uncommitted cancellation-test repairs remain intact and are included in final
verification, but they are not redesigned as part of these five repairs.
