# Dokploy Port resource

## Intent and approved decisions

Expose application port mappings as independently managed declarative child resources, rather than adding port-list ownership to `Application`. Pulumi users must be able to create, update, refresh, import, and delete individual mappings using typed SDKs.

The user approved:

- A standalone `dokploy:index:Port` backed by the dedicated port API.
- Application-only targets, with no Compose or database targets.
- Configuration-only CRUD, including deletion: never deploy or redeploy an application.
- Required application ID and published/target ports; TCP and ingress defaults.
- Integer ports from 1 through 65535; no automatic/random port allocation.
- Replacement when the application changes; in-place updates for the remaining settings.
- Exact-ID read-back, conservative absence handling, safe diagnostics, and acknowledged-create partial state.
- Provider, OpenAPI, generated schema/SDK, example, and documentation changes with offline regression coverage.

Success means the resource is available in all five generated SDKs, owns only its individual mapping, and accurately reflects stored configuration without implying that a workload has adopted the change. Documentation updates are required deliverables, not follow-up work.

## Architecture and alternatives

Implement a typed inferred resource in `provider/port.go`, registered with `configuredClient` in `provider/provider.go`. Use the existing authentication, cancellation, timeout, and bounded GET retry behavior. Keep conversion, validation, and lifecycle helpers local to the Port implementation unless an existing helper already fits.

Alternatives rejected:

1. An application-owned list of ports. This couples independent mappings to application lifecycle and risks ownership conflicts with individual resources.
2. A CLI wrapper. This adds a runtime dependency and weakens typed request/response handling despite the dedicated HTTP API.

“Child resource” describes the required Dokploy application relationship. Do not automatically set Pulumi's `parent` option or change the `Application` contract. Passing an application's output as `applicationId` establishes the usual Pulumi dependency.

No new lookup function, application port collection, deployment toggle, firewall management, availability probing, or unrelated refactoring is in scope.

## Public resource contract

| Property | Schema type | Required/default | Change behavior |
| --- | --- | --- | --- |
| `applicationId` | string | Required | Replacement |
| `publishedPort` | integer | Required | In-place update |
| `targetPort` | integer | Required | In-place update |
| `protocol` | enum: `tcp`, `udp` | Optional; `tcp` | In-place update |
| `publishMode` | enum: `ingress`, `host` | Optional; `ingress` | In-place update |
| `portId` | string output | Server-assigned | Stable resource identity |

State contains the five observed input properties and `portId`. No deployment-status, application relation object, or raw response fields are exposed. These fields are not inherently secrets; normal Pulumi secret propagation still applies.

Validation requirements:

- Reject missing, null, empty, or whitespace-only application IDs. Treat nonempty IDs as opaque and send them unchanged; do not include them in diagnostics.
- Accept only integral port values in the inclusive range 1..65535. Reject fractional values, zero, negative values, and values above the upper bound before networking.
- Apply defaults only to omitted enum inputs. Explicit empty or unsupported values are errors, not requests for a default.
- Validate each known field independently; defer checks only for the corresponding computed input during preview.
- Validate concrete create/update arguments defensively before issuing mutations. Update must not silently retarget an existing ID to a different application; that change belongs to replacement.

`Diff` reports replacement only for `applicationId`, updates for the four settings, and no changes for identical desired and observed values. Use the inference framework's normal replacement ordering; no special delete-before-replace behavior is needed for stored mappings on different applications.

Preview performs no networking or client construction. A create preview leaves `portId` computed; an update preview retains the known ID. Configure dependency wiring accordingly and cover the behavior through provider-level tests.

## Verified upstream contract and OpenAPI changes

The repository pins Dokploy commit `cebd3808565ea9bed0791961bc25c60513d94c5a` in `openapi/source.json`. Its upstream OpenAPI includes all four operations but describes their successful responses as empty objects. Source inspection establishes the actual response shape:

- [Port router](https://github.com/Dokploy/dokploy/blob/cebd3808565ea9bed0791961bc25c60513d94c5a/apps/dokploy/server/api/routers/port.ts)
- [Port service](https://github.com/Dokploy/dokploy/blob/cebd3808565ea9bed0791961bc25c60513d94c5a/packages/server/src/services/port.ts)
- [Port database/API schema](https://github.com/Dokploy/dokploy/blob/cebd3808565ea9bed0791961bc25c60513d94c5a/packages/server/src/db/schema/port.ts)

| Operation | Method | Request | Successful response |
| --- | --- | --- | --- |
| `port.create` | POST | Application ID and four port settings | Created Port row |
| `port.one` | GET | `portId` query parameter | Port row with an application relation |
| `port.update` | POST | Port ID and all four port settings | Updated Port row |
| `port.delete` | POST | Port ID | Deleted Port row |

The service persists database rows only; none of these operations deploys the application. Update does not accept `applicationId`. The database application foreign key cascades deletion of port rows when the application is deleted.

The API schema specifies ingress as the default even though the database column defaults to host. The provider explicitly supplies ingress unless configured otherwise and does not rely on the database default.

Add the four operations to `openapi/operations.txt` and response corrections to `openapi/corrections.json`, all referencing a flat `Port` model. Model the ID as required and the other response fields as optional, non-null integer/string pointers so the provider can reject absent or null settings instead of accepting zero-value defaults. Ignore additional fields and the nested application relation.

Keep the pinned upstream file and source hash unchanged. Preserve upstream request shapes, including their numeric wire types; convert validated provider integers at the request boundary. Regenerate `openapi/dokploy.json` and `internal/client/generated/generated.gen.go` with the existing Makefile workflow. Do not hand-edit generated output.

## Lifecycle and data flow

### Create

1. Return preview output without networking when dry-run is set.
2. Validate the concrete inputs and send one `port.create` request with all settings explicit.
3. Extract a nonempty server-issued `portId` from the successful response before decoding or validating unrelated row fields. Use the generated raw request method if needed to avoid losing an acknowledged ID when full response decoding fails.
4. Read that exact ID through `port.one`; require the returned ID to match and the application to equal the requested owner. Validate all observed settings, then return canonical state.
5. If creation returned an ID but read-back fails or is invalid, return that ID with attempted input state and `ResourceInitFailed` so Pulumi retains the partial resource. Never adopt the identity from a mismatching read response.

A successful response without a recoverable ID is an uncertain creation outcome. Return a safe error directing the operator to inspect Dokploy before retrying. Do not retry create, scan ports, match settings, guess an ID, or automatically delete a possible resource. Apply the same no-retry rule to transport failures whose mutation outcome is unknown.

### Read, refresh, and import

Call `port.one` with the requested ID. Require a matching, nonempty ID, a nonempty application ID, both valid integer ports, and supported protocol/publish-mode values. Missing/null fields and invalid configuration are errors; never fill them from prior state or creation defaults.

Project the complete observed row into both refreshed inputs and state. Import requires only the ID and works with empty prior inputs/state. A confirmed missing resource returns an empty Pulumi resource ID; malformed or ambiguous responses leave state intact by returning an error.

### Update

Return preview output without networking when dry-run is set, preserving `portId`. Validate inputs and the unchanged application relationship. Send the requested ID and all four settings to `port.update`; never include an application ID in the update body.

Read back the exact ID after acknowledgment, validate ownership and configuration, and return canonical state. If acknowledgment succeeds but read-back fails, return an error with the existing ID and attempted configuration rather than claiming success or returning an unrelated object. If the mutation fails before acknowledgment, retain prior state rather than claiming the requested values were observed. Do not retry the mutation or roll it back automatically.

### Delete

Call `port.delete` directly with the resource ID, without a preliminary `port.one` read. Treat confirmed absence as success; report all other failures. A successful delete does not depend on decoding the deleted row and never invokes application deployment. Retrying a later Pulumi destroy after a failure uses the same identity, not discovery.

## Absence, safety, and diagnostics

Only an explicit HTTP 404 or structured `NOT_FOUND` code counts as absence, using the existing client classifier before sanitizing the error. Do not modify global absence handling for this resource.

The pinned `port.one` router catches both lookup failures and application-permission failures and returns `BAD_REQUEST` with “Port not found.” Therefore HTTP 400 or that message alone cannot prove deletion. Refresh/import fail conservatively on this response. Out-of-band application deletion can consequently produce a refresh error instead of automatic state removal on this version. Document this as an upstream compatibility limitation; do not claim offline tests establish deployed-server behavior.

Preserve cancellation/deadline classification and safe operation/status diagnostics. Do not forward arbitrary server messages, response bodies, IDs, or transport URLs. Classify confirmed absence before sanitization removes server codes. Close response bodies and use bounded response handling where the implementation bypasses generated response parsing.

No lifecycle operation calls deployment, redeployment, application-update, registry-test, or list/discovery endpoints. Successful CRUD means stored configuration has changed, not that traffic routing has changed. Deleting a mapping also requires a subsequent application deployment to change the running workload. Port management does not open a firewall or establish host-port availability.

## Documentation and examples — required acceptance criteria

- Update provider metadata and `README.md` resource/import documentation to include Port. Keep resource listings accurate without dropping existing Schedule support; avoid hard-coded stale counts.
- Add `website/src/content/docs/guides/ports.mdx` explaining application-only mappings, property defaults, ingress versus host publishing, integer validation, configuration-only CRUD, and explicit application deployment after create/update/delete.
- Update the applications guide with a link to the Port guide; update the imports and troubleshooting guides with the ID-only import contract and ambiguous HTTP 400 limitation. Include `pulumi import dokploy:index:Port mapping <port-id>` using a placeholder only.
- Generate `website/src/content/docs/reference/port.mdx` from the schema. Update reference-generation inventories/navigation if the existing generator requires explicit resource registration. Never hand-edit generated references.
- Add an application Port to the canonical `examples/yaml/Pulumi.yaml` with a comment that it saves configuration only. Use a nonprivileged example published port and target port 80; do not claim automatic activation.
- Regenerate the Node.js, Python, Go, .NET, and Java examples through the repository workflow, and regenerate website example pages from their sources. Update the canonical example README so the copied language READMEs carry the same deployment warning.
- Document that Compose ports remain in Compose configuration and database exposure is not managed by Port. Explain that the Dokploy application relationship is separate from Pulumi's optional parent setting.

Review user-facing wording for accidental claims that every resource create/update waits for deployment. Clarify such wording where necessary for Port, without changing existing resource behavior.

## Regression coverage

Use the existing scripted HTTP server and inferred-provider integration tests. Cover:

1. Required fields, enum defaults, explicit empty enums, boundary ports (1 and 65535), out-of-range/fractional/null inputs, and whitespace-only IDs.
2. Computed inputs and independent validation of other known invalid fields.
3. No-change diff, every mutable field, owner replacement, output-only ID, enum schema types/defaults, integer schema types, and replacement metadata.
4. Exact create/update request bodies, ID query encoding, canonical read-back, complete refresh, import with empty prior state, and direct deletion.
5. Missing/null/incomplete records, unsupported settings, mismatched identities, wrong ownership after mutation, and invalid numeric responses.
6. Acknowledged create identity retained after failed read-back or invalid unrelated create-response fields; missing create identity and uncertain transport outcomes do not trigger retry, discovery, or cleanup.
7. Update failures before/after acknowledgment, cancellation/timeouts, HTTP 404 and structured `NOT_FOUND`, ambiguous 400, authentication/permission errors, and safe diagnostics.
8. Create/update previews perform no client/network access; create ID is computed and update ID remains known.
9. Strict endpoint sequences ensure no deployment or discovery calls on any success, failure, preview, or deletion path.
10. Normalization includes the four operations and actual response corrections while preserving the existing contract. Schema/SDK/example/reference generation remains deterministic and discoverable.

## Validation and delivery

Use versions pinned in `.mise.toml` and Makefile targets. Before regeneration, inspect uncommitted changes; preserve unrelated work. Planned checks:

- `mise exec -- go test ./openapi/cmd/normalize -count=1`
- `make lint`, `make test_provider`, `make test_race`, `make provider`
- `make check_openapi`, `make check_codegen`
- `make build_sdks`, `make test_examples`
- `make docs_check`
- `git diff --check`, final status and focused diff review

Generation-drift checks compare against Git, so newly generated files must first be included in the implementation commits before requiring a clean drift result. Run targeted regression tests before the broad checks. Report unavailable or unrun checks explicitly.

No live acceptance execution, release, or publication is authorized by this plan. Live response-contract and deployed compatibility verification remain a separately opted-in activity on a dedicated server under `tests/README.md`. Do not add an automatically run live fixture as part of this scope.

Delivery includes provider and test sources, OpenAPI corrections and generated client, generated schema and all five SDKs, canonical and generated language examples, required prose docs, and generated website references/examples. The next stage is a written implementation plan after the user reviews this spec; product implementation is not authorized yet.
