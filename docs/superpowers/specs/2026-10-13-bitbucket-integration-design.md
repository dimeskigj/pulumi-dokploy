# Dokploy BitbucketIntegration resource

## Intent, approval, and scope

Manage a Dokploy Bitbucket integration's configuration and lifecycle as a Pulumi
resource, including the two credential modes accepted by the user and guarded
creation despite Dokploy's bodyless create acknowledgment. The user approved the
public contract and create strategy described here. This written spec does not
authorize implementation: obtain written-spec approval before preparing an
implementation plan; plan approval and execution method are separate decisions.

Out of scope: OAuth/session/readiness state, `authId` as public configuration,
owner or sharing changes, connection tests, repository discovery, workload-source
expansion, upstream credential revocation, live acceptance, and release or
publication. Existing workload source contracts remain unchanged; this resource
exports the Bitbucket relation ID for any separately supported consumer.

## Public contract

Register `dokploy:index:BitbucketIntegration` using inferred-resource,
configured-client, generated-client and existing secret/partial-state conventions.

| Input | Meaning |
| --- | --- |
| `name` | Required nonempty Dokploy display name. |
| `apiToken` | Optional secret credential for API-token mode. Exactly one of `apiToken` and `appPassword` must be a nonempty concrete value. |
| `appPassword` | Optional secret credential for legacy app-password mode. Exactly one credential mode must be selected. |
| `bitbucketEmail` | Optional identity. Required and valid email when `apiToken` mode is selected. |
| `bitbucketUsername` | Optional identity. Required nonempty string when `appPassword` mode is selected. |
| `bitbucketWorkspaceName` | Optional workspace configuration; omission preserves it, explicit empty string requests empty workspace. |

Exactly one credential must be nonempty; neither, both, unresolved selection on
an actual apply, or a selected mode missing its required identity fails validation.
Check validates every known field independently: an unknown credential cannot
hide a known invalid email or other known error. Preview may retain unknown
inputs without treating them as empty. Do not infer or silently switch modes.
Email, whenever supplied, must be syntactically valid even if it is not the active
mode's identity. No public organization, owner, sharing, OAuth, session, readiness,
`authId`, `gitProviderId`, or connection-test input.

Outputs include the public arguments (credentials remain secret), observed
`bitbucketId` as the Pulumi identity and import ID, `gitProviderId`, and
`organizationId`. No readiness, OAuth, auth/session, provider-owner, or unrelated
upstream fields. Import uses `bitbucketId`, never parent ID. Identity outputs are
ordinary nonsecret values.

## API evidence and OpenAPI/client scope

The upstream pin is `cebd3808565ea9bed0791961bc25c60513d94c5a` in
`openapi/source.json`. Source-grounded follow-up research and verification are
`.superpowers/sdd/2026-10-04-gitlab-integration/followup-api-research.md` and
`followup-api-verification.md`; do not re-research externally or change the pin.
`bitbucket.create` acknowledges with no identity/body; its service inserts parent
and relation. `bitbucket.one` returns the full Bitbucket relation and nested
parent. `bitbucket.update` has separate parent/relation updates. `gitProvider.getAll`
returns accessible parent identities and Bitbucket summaries, including incomplete
records with `isConfigured:false`; `gitProvider.remove` deletes parent with a
verified cascading relation delete. `user.get` supplies authenticated member
identity; use that member's ID for required create `authId`, never a public input.

Critical request constraint: create/update credentials, username, and workspace
are non-nullable strings; email is also non-nullable and valid-email. Never send
null for these fields. Empty strings are accepted (no minimum length); use empty
string explicitly to deactivate an unused credential. Observed null and empty
credential values normalize to absent. Active credential must remain nonempty.
Omitted optional inactive identity and workspace preserve server values; do not
invent clears or manage omitted fields. Email cannot be null/empty-cleared. An
inactive identifier can persist outside the selected mode and is reflected in
observation; omission must not cause a diff. Explicit empty workspace may clear.

Add only `bitbucket.create`, `bitbucket.one`, `bitbucket.update`,
`gitProvider.getAll`, `gitProvider.remove`, `user.get`, and existing active
organization operation to `openapi/operations.txt`. Correct source response
shapes from pinned router/service/schema evidence: authenticated member identity;
provider-list parent identity plus nullable Bitbucket summary with relation ID,
username, configured/deprecated flags; detail relation ID, parent ID, username,
email, workspace, both credentials and nested parent identity (name, type, org,
user, sharing metadata, created time). Keep response-only credential data
internal and project only whitelisted fields. Model field presence/nullability
explicitly so missing modeled fields are not confused with explicit null/empty.
Mutation responses are bodyless acknowledgments: consume/close raw mutation bodies
without relying on JSON decoding for success, and never retry mutations.

Regenerate OpenAPI output, generated client, provider schema, and Go, Node.js,
Python, .NET, and Java SDKs only from their canonical sources. Do not hand-edit
generated outputs or add dependencies solely for this resource.

## Credential and optional-field semantics

Read/import can represent incomplete legacy records, including neither or both
credentials, without exposing secrets as ordinary values. A mutation requires a
valid selected mode. For a previously managed credential, refresh omission/null
preserves the prior known Pulumi secret; do not replace it with a placeholder.
For a new rotation, omission alone is not evidence that the server retained or
accepted the new value. Updates send the complete desired managed settings, using
an explicit empty string for the unused credential and nonempty selected
credential. A mode switch succeeds only after readback confirms the old mode's
credential is actually cleared; if upstream has partial/normalized state, report
that actual state, not desired fiction. Optional inactive email/username/workspace
are sent only when supplied; omitted values preserve server state and remain
represented observationally without perpetual diffs. Active mode's identity must
be supplied and valid on apply. No implicit email clear is supported.

Legacy app-password support is present in Dokploy's schema, but external Bitbucket
acceptance is not established. Do not claim legacy auth compatibility with current
Bitbucket or live connectivity.

## Lifecycle and safety

### Preview and create

Create preview makes no calls; identity outputs stay computed. Check validates
known fields and handles computed inputs as above. Real create validates selected
mode, reads active organization and authenticated member, and verifies consistent
nonempty identity. Snapshot the full accessible provider list, validating unique
nonempty parent IDs, relation IDs, and summary consistency. Follow the approved
GitLab guarded-create pattern in `provider/gitlab_integration_create.go`:
create a cryptographically unpredictable temporary parent name; use no caller-
chosen ID; issue exactly one create; then discover from a bounded maximum of
three `getAll` attempts, cancellation-aware. Retry discovery only when there is
no candidate. Require exactly one new marker-matched parent absent from snapshot,
correct Bitbucket type, active org and authenticated user, and a new relation ID.
Fetch `bitbucket.one` and verify relation/parent identities, nested parent,
marker/type/org/user, and complete requested configuration including credential
mode and supplied optional values. Summary and detail must agree. Matching name
alone is never adoption; ambiguity, malformed/mismatching detail fails closed.

Rename the verified parent to requested name via update, then verify by readback.
Never retry create or perform cleanup/delete of an uncertain candidate. Proven
pre-mutation auth/permission rejection does not trigger discovery; generic bad
request is not proof create did not commit. If create outcome is uncertain but a
complete owned identity is verified, return partial state with that identity and
the original failure, without rename. Without verified identity, return no ID and
a sanitized error. If rename or post-rename verification fails after identity was
verified, return only verified actual state and IDs via partial-state error.

### Read and import

Read by relation ID. Verify returned relation ID, parent ID and nested parent,
Bitbucket provider type, active/recorded organization, and stable previously known
identity. Import establishes IDs from verified response. Preserve prior known
secrets on response omission/null; no fake placeholders. Expose observed
incomplete/both-credential records and configuration without treating them as
mutation-valid. Only structured genuine not-found removes state; authorization,
transport, malformed response, and identity errors do not.

`getAll` summaries include incomplete Bitbucket entries as
`{id, username, isConfigured:false, isDeprecated}` without credentials; detail
contains credentials and all configuration plus parent. Use summary only for
candidate discovery, then verify detail. Do not leak credentials into outputs
other than the declared secret args/state.

### Update

Validate full desired mode and required identity; pre-read and verify relation,
parent, type, org and stable IDs. Send complete managed desired settings plus
known IDs; never send null. Send empty string for the unused credential. Send
optional email/username/workspace only if supplied; omission preserves upstream
configuration, explicit empty workspace means clear. Do not modify owner/sharing.
Read back after acknowledgment and verify identity and managed fields; compare
inactive optional fields only when supplied, and ensure mode-switch credential
clear is observed. Parent and relation updates may not be atomic: on errors or
mismatch, attempt safe verified readback when context allows and return actual
observed/last-known state as partial failure. Never retry mutation.

### Delete

Pre-read and verify relation, type, active organization, stable parent identity;
remove only verified parent ID using `gitProvider.remove`. Parent delete cascades
relation. Pre-read proving absence succeeds idempotently. Removal error succeeds
only if subsequent verified read proves absence; generic `BAD_REQUEST` is not
not-found. No workload deletion, upstream revocation, or external credential
rotation.

## Secrets, diagnostics, and documentation

Both credentials are secret inputs/outputs and must receive secret annotations
and dependency propagation through every generated SDK. Keep credentials out of
logs, diagnostics, and public response projection. Resource-scoped diagnostics
include only operation, safe status/classification, and safe context; never include
raw upstream prose/bodies, IDs, private URLs, or credential values. Classify
structured not-found before discarding unsafe text. Test malformed and error
responses containing secret-like material, including imports without prior state.

Document two credential modes, conditional email/username requirements, mutually
exclusive credentials, secret configuration via canonical Pulumi secret config,
manual limits, import identity, lifecycle/uncertain-create behavior, omission vs
explicit-empty workspace, mode-switch clear verification, and destruction scope.
Update provider metadata, README, canonical examples, workload-source guidance
only where relevant, and generated language/reference docs. Examples use reserved
placeholders and canonical secret-config references; never include live credential
values or suggest ordinary plaintext credentials. Clearly state legacy
app-password acceptance and connectivity are not guaranteed. Preserve unrelated
and pre-existing documentation.

## Regression coverage and validation

Offline scripted tests cover per-field known/computed validation; exclusive modes,
conditional identities and email validation; secret metadata and dependency
annotations; incomplete/both-secret read/import; null/omission secret preservation;
request exactness and no null; optional omission-preserve / explicit-empty workspace;
mode switch clear and mismatched partial state; create snapshots, duplicate/missing
IDs, marker collisions, wrong user/org/type, prior IDs, summary/detail mismatch,
ambiguity, bodyless acknowledgments, exactly bounded empty-only retries,
cancellation, uncertain create partial recovery without rename, no mutation retries,
rename/readback failures, updates split failures, delete cascade verification,
not-found classification, safe diagnostics, previews with no calls, and SDK/schema
contract. No live credential placeholders or live acceptance tests.

Run applicable checks through a verified SAFE launcher with actual temporary
`TMPDIR=/tmp/opencode`, protected `.env` neither read nor modified, and live
acceptance forcibly disabled including nested Makefile `mise exec`. Keep generator
temporary outputs portable; do not hardcode `/tmp/opencode` in product or
canonical generator configuration. After approval/implementation, applicable
checks include `make lint`, `make test_provider`, `make test_race`, `make provider`,
`make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`,
and `make docs_check`. Do not run live acceptance, release, publication, push, or
other-worktree operations. Inspect status and `git diff --check`; report unavailable
checks, and never claim unrun checks passed.

## Self-review

The spec distinguishes relation and parent identity, bodyless create reconciliation,
readable incomplete records, strict mutation validity, inactive-field observation
from active-mode requirements, and omitted versus explicit-empty workspace. It
makes no null-clearing promise for nonnullable upstream requests, no legacy
Bitbucket compatibility claim, and no OAuth/readiness/workload expansion. Generated
surface, offline coverage, and safe validation are explicit; no placeholders or
execution authority are implied. Retain this spec and any future plan unless the
user explicitly approves cleanup; preserve unrelated documentation.
