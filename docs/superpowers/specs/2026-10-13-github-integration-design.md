# Import-only Dokploy GitHubIntegration

## Intent and approved conversational scope

Manage an existing Dokploy GitHub integration with Pulumi after its GitHub App
setup and authorization have been completed outside Pulumi. The user selected
import-only management and approved the public contract below. This is the GitHub
phase of the requested Git-provider integration rollout, not a GitHub App creator.

This document requires written-spec review before an implementation plan. It does
not authorize product implementation on its own. The selected execution method
is SDD with Luna implementer/reviewer subagents, parallel where file ownership and
dependencies permit.

## Public contract

Register `dokploy:index:GitHubIntegration` using existing inferred-resource and
configured generated-client conventions.

| Input | Meaning |
| --- | --- |
| `name` | Required, nonempty Dokploy display name. |
| `githubAppName` | Required, nonempty app-name value stored by Dokploy. Updating it changes Dokploy configuration, not the GitHub-side App registration. |

Outputs are observed managed configuration plus:

- `githubId`: stable Dokploy GitHub relation ID and Pulumi identity.
- `gitProviderId`: stable parent Git-provider ID used for update/removal.
- `organizationId`: owning Dokploy organization.

Import uses the relation ID, not a GitHub App ID, installation ID, or parent
Git-provider ID. No app private key, OAuth client secret, access token, refresh
token, installation secret, or upstream user record is a resource input/output.
There is no readiness output or connection guarantee.

Existing Application/Compose source variants are unchanged. This phase does not
add GitHub workload-source configuration or change deployment behavior. The IDs
remain ordinary outputs, not secret merely because the underlying integration
contains credentials not represented in Pulumi.

## API boundary and alternatives

The pinned `openapi/upstream.json` exposes `github.one`, `github.update`, and
`gitProvider.remove`, but no `github.create`. The update request requires
`githubId`, `gitProviderId`, `name`, and `githubAppName`. Reuse the existing active
organization operation and parent removal operation; select only the required
GitHub read/update operations. Preserve the pinned upstream commit and all
existing resource/API selections.

The chosen lifecycle is import-only management. A read-only lookup would not
manage changes or destruction; fabricating creation using private UI/callback
routes would exceed the supported public API. Neither is substituted silently.

Response corrections must be based on the pinned Dokploy router/service/schema
sources. Model only relation/parent identities, parent name/type/organization,
and observed app-name configuration needed by this contract. The record's parent
relation must be checked, not inferred from its list position or display name.
Ignore extra response fields and never project GitHub credentials into state.

## Lifecycle

### Create and preview

Real Create always fails with a fixed diagnostic explaining that the integration
must be set up/authorized in Dokploy and imported. It performs no API request and
returns no fabricated identity. No private endpoint, browser automation, token
exchange, or adoption by name is attempted.

Create preview makes no API calls and leaves identity outputs computed. Update
preview preserves established IDs, echoes managed inputs, and performs no API
calls. Check validates each known input while accepting unresolved preview inputs;
one computed field must not hide another known invalid field.

### Read and import

Fetch by the requested GitHub relation ID. Require the returned relation ID to
match the request, a nonempty parent ID matching its nested parent identity,
GitHub provider type, and an owning organization consistent with the active
organization. A previously recorded parent or organization may not silently
change. Import establishes those identities from the verified record.

Reconstruct only the whitelisted public inputs and outputs. An explicitly
incomplete app-name configuration can remain readable; document that the user
must supply a nonempty app-name setting before an update. Missing required
identity/configuration-presence data is malformed, not proof of absence. Do not
invent settings or placeholders. Canonical response handling must distinguish an
explicit incomplete value from an unexpectedly omitted modeled field.

Only structured genuine not-found responses remove the resource from state.
Authorization, transport, response-decoding, and identity failures remain errors.
Sharing does not authorize importing/mutating an integration owned by a different
active organization.

### Update

Both managed configuration fields update in place. Validate resolved inputs,
read and verify the existing relation/parent/organization before mutation, and
send only the two managed settings and the verified relation/parent IDs. Do not
send sharing, ownership, credentials, installation settings, or unrelated fields.

Use existing raw generated mutation/transport conventions when acknowledgment
does not carry resource identity; close bodies without letting irrelevant success
JSON parsing invalidate an acknowledged mutation. Never retry a mutation.

Read back after acknowledgment and require stable identity and matching managed
settings. If parent/relation changes are not atomic, preserve verified actual or
last-observed state on failure; do not claim desired settings were applied. When
context allows, a safe identity-verified readback can improve partial failure
state. An unconfirmed mutation is an error, not silent success.

### Delete

Read and verify relation, parent, type, and active/previous organization before
removing only the verified parent ID. A pre-read proving relation absence makes
deletion idempotent without a removal request. A removal error is considered
success only after a verified read proves absence; generic BAD_REQUEST is not
proof of deletion. Never delete by a requested parent ID without relation
verification, list position, matching display name, or unrelated fallback search.

This removes the Dokploy integration record. It does not remove the GitHub App,
uninstall it at GitHub, revoke upstream authorization, or delete workloads.
Document destruction risk for unmanaged workloads that reference the integration.

## Diagnostics and security

Resource-scoped errors contain only safe operation/status/context classification.
Do not forward arbitrary response bodies, parser fragments, URLs, resource IDs,
private keys, or upstream error prose. Preserve cancellation/deadline and
structured not-found classification before discarding unsafe text. Import must
remain safe without prior secrets in state: unrelated credential-bearing response
fields and errors are never included in diagnostics or state.

## Tests and generated deliverables

Offline tests cover known/computed input validation, in-place diffs, no-API
Create/rejection and preview, import/read reconstruction and incomplete settings,
relation/parent/type/organization mismatches, exact update payload and sequence,
split-update/readback partial failures, deletion idempotence/error verification,
mutation non-retry, cancellation, ignored credentials, and safe diagnostics.

Schema tests assert exactly two required inputs and the three identity outputs,
no credential/readiness/sharing fields, no replacement flags, and ordinary stable
IDs in preview. OpenAPI/client tests verify selected operations, corrected
whitelisted response shapes, required update IDs/settings, and bodyless mutation
handling against pinned source behavior.

Generate provider schema and Go/Node.js/Python/.NET/Java SDKs from source. Update
inventories additively, preserving GitLab and all existing resources/invokes.
Document UI setup, relation-ID import, subsequent managed use, limitations,
incomplete configuration repair, and removal semantics. Canonical examples must
not create a GitHub integration by default; show an explicitly imported existing
integration using reserved placeholders and Pulumi import options. Regenerate
language examples/reference docs from their canonical sources.

Run applicable pinned-tool offline checks through the verified SAFE launcher,
including normalize/provider tests, lint/race/build, OpenAPI/codegen drift, SDKs,
examples, and docs. No protected `.env` access, live acceptance, release,
publication, push, or shared-branch merge is authorized. Report unavailable checks
as unavailable. Inspect status and diff checks before finishing.

## Self-review and document retention

The public contract matches the user's import-only choice; no unsupported create
or readiness promise remains. Lifecycle identities are distinct, managed settings
are narrow, and testing/SDK/docs requirements cover the exposed surface. Exact
API response corrections belong to the source-grounded contract task, not an
invented response shape. No unresolved placeholders are required to understand
the intended resource behavior.

Keep this spec and its future plan until the user explicitly approves cleanup.
Preserve unrelated documentation and pre-existing user work.
