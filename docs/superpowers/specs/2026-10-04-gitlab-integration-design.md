# Dokploy GitLabIntegration resource

## Intent and approved scope

Manage the Dokploy GitLab integration's configuration and lifecycle in Pulumi,
so existing Application and Compose GitLab sources can reference its output ID
instead of requiring an externally created integration.

The user approved configuration/lifecycle management with manual OAuth
authorization, the public contract below, and guarded creation/discovery and
deletion. Success includes create, read, update, delete, import, safe preview,
generated SDKs, offline regression coverage, examples, and user documentation.

This spec does not authorize execution. Written-spec review precedes an
implementation plan; the user then selects its execution method.

Out of scope: GitHub/Gitea/Bitbucket integrations, GitLab-side OAuth application
creation, unattended authorization, token management, organization sharing,
repository discovery, automatic connection tests, and live acceptance execution.
Existing externally supplied integration IDs remain supported.

## Architecture and alternatives

Register `dokploy:index:GitLabIntegration` as an inferred resource in
`provider/provider.go`, using `configuredClient` and the generated API client.
Keep resource arguments/validation/diff separate from lifecycle and response
verification if necessary to keep files focused. Reuse existing partial-state,
annotation, secret, preview, and HTTP transport conventions without refactoring
unrelated resources.

Alternatives considered:

1. Full configuration/lifecycle management with manual OAuth: selected; meets
   the current API's capabilities and immediately feeds existing workload inputs.
2. Import-only management: safer initial implementation but does not provision
   the integration, so leaves the principal configuration gap open.
3. Unattended OAuth provisioning: unavailable in the pinned create/update APIs;
   requires upstream support and is not approximated through private endpoints.

## Public contract

| Input | Requirement and meaning |
| --- | --- |
| `name` | Required, nonempty display name. |
| `applicationId` | Required, nonempty GitLab OAuth application/client ID, not a Dokploy Application ID. |
| `applicationSecret` | Required, nonempty GitLab OAuth client secret; secret in input and output schema. Maps to API `secret`. |
| `redirectUri` | Required absolute HTTP(S) OAuth callback URL configured on the GitLab OAuth application, normally the Dokploy `/api/providers/gitlab/callback` URL without an integration query parameter. |
| `gitlabUrl` | Optional absolute HTTP(S) GitLab base URL; default `https://gitlab.com`. |
| `gitlabInternalUrl` | Optional absolute HTTP(S) internal GitLab base URL. Absence means null/no override. Mark secret because Dokploy supports embedded basic-auth credentials here. |
| `groupName` | Optional comma-separated GitLab group filter; omission canonicalizes to an empty string. No custom group parser. |

Validate known values without echoing URLs, IDs, credentials, or other values.
Allow base URL paths for self-hosted GitLab; do not silently rewrite supplied
URLs. Reject query/fragment components on base URLs. Public GitLab URL and
callback URL must not contain userinfo; the internal URL may contain it, matching
Dokploy's supported token-exchange configuration. No public `authId`,
`gitProviderId`, or organization selector inputs.

Outputs include observed configuration plus:

- `gitlabId`: stable GitLab relation ID; also the Pulumi resource identity.
- `gitProviderId`: stable parent Git-provider ID used by update and removal.
- `organizationId`: owning Dokploy organization.
- `isConfigured`: true when Dokploy's observed access and refresh tokens are
  both nonempty. This is not a connectivity, token-validity, or deployment guarantee.

The schema does not expose access tokens, refresh tokens, token expiration, user
records, or unrelated response properties. Ordinary identity outputs are not made
secret merely because configuration has secret inputs.

Import uses `dokploy:index:GitLabIntegration` and the GitLab relation ID, not the
parent Git-provider ID. Existing Application and Compose inputs remain
`source.gitlab.integrationId`; consumers assign the integration's `gitlabId`.

### Diff, checks, and preview

All managed configuration changes update in place, including client-secret
rotation and URL changes. Such changes may require manual OAuth reauthorization;
the provider must not clear existing tokens or claim they are still valid.
`isConfigured` is an observed output, never an input or deployment gate.

Check respects computed inputs and never rejects an unresolved preview value as
empty. Apply defaults only where inputs are known absent. Create/update dry-runs
perform no API calls. New identity/readiness outputs remain computed on creation;
update preview retains established IDs and does not invent readiness after an
OAuth configuration change. Configure dependency wiring so normal IDs stay usable
as ordinary workload references while secret fields stay secret.

## Pinned API evidence and OpenAPI changes

The existing OpenAPI pin is Dokploy commit
`cebd3808565ea9bed0791961bc25c60513d94c5a` in `openapi/source.json`. Reviewed source:

- `apps/dokploy/server/api/routers/gitlab.ts`: create returns the service result;
  one returns the GitLab record with its parent; update changes the parent name
  and GitLab settings separately and returns no body.
- `packages/server/src/services/gitlab.ts`: create inserts parent and relation
  in a transaction but returns no transaction identity. The relation ID is
  generated internally. Session organization/user, not supplied `authId`, own it.
- `packages/server/src/db/schema/gitlab.ts`: create requires a nonempty `authId`,
  but neither create nor update accepts access/refresh tokens. Internal URL is
  nullable; optional client settings are non-nullable in update requests.
- `apps/dokploy/server/api/routers/git-provider.ts`: getAll includes incomplete
  integrations with safe GitLab summaries; remove targets the parent ID and
  cascades its relation. The `gitlabProviders` endpoint filters incomplete
  integrations, so cannot be used for creation discovery.
- `packages/server/src/db/schema/git-provider.ts`: provider type, name,
  organization, user, parent ID, and relation identity establish ownership.
- `apps/dokploy/server/api/routers/user.ts` and
  `packages/server/src/db/schema/account.ts`: protected `user.get` returns a
  member with `userId`, `organizationId`, and nested `user.id`.
- `apps/dokploy/pages/api/providers/gitlab/callback.ts` and
  `packages/server/src/utils/providers/gitlab.ts`: OAuth callback creates the
  tokens; later operations refresh them independently of Pulumi.

Source links use `https://github.com/Dokploy/dokploy/blob/` followed by the pin and
these paths. Do not change the upstream pin or fabricate ID-returning create.

Add `gitlab.create`, `gitlab.one`, `gitlab.update`, `gitProvider.getAll`,
`gitProvider.remove`, and `user.get` to `openapi/operations.txt`. Reuse existing
`organization.active`. Do not select connection tests or OAuth callback routes.

Define source response corrections for:

- An authenticated member: user and organization identity only, including nested
  user ID; ignore PII and API-key metadata.
- A Git-provider list: parent ID, name, type, organization/user identity, and
  nullable GitLab summary containing GitLab ID, client ID, URL, and isConfigured.
  Other providers may appear, but their settings are neither modeled nor exposed.
- A GitLab record: relation ID, parent ID, URL/internal URL, client ID, secret,
  redirect URI, group filter, access/refresh tokens, and nested parent identity.
  Tokens are internal response fields only, used to calculate a boolean and
  discarded before building state. Require explicit nullable token fields when
  calculating readiness; malformed/missing fields must not manufacture false.

Create/update are bodyless acknowledgments at this pin. Use raw generated
mutation calls, close bodies, and ignore irrelevant success bodies, so JSON
parsing cannot lose an acknowledged mutation. Removal also needs no response
projection. Existing transport supplies non-2xx errors and bounded GET retries;
mutations must never be retried automatically.

Regenerate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`,
the provider schema, and Go/Node.js/Python/.NET/Java SDKs from their sources.
Do not hand-edit generated output or add dependencies solely for this resource.

## Lifecycle and identity safety

### Create

1. Validate resolved settings. Read active organization and authenticated member;
   require a consistent nonempty organization and user identity. Use the member's
   user ID for the required API `authId`; it is not a resource input.
2. Snapshot the complete accessible provider list. Require nonempty unique parent
   IDs, valid common identity fields, and consistent GitLab summaries for GitLab
   entries. Never infer identity from list position or requested display name.
3. Generate an unpredictable temporary name with existing UUID support, such as
   `pulumi-gitlab-<uuid>`, and verify its absence from the snapshot. POST create
   exactly once using that marker and requested OAuth settings. Do not supply
   a caller-chosen parent/relation ID.
4. After acknowledged success or an uncertain transport/server failure, discover
   via a bounded post-create getAll read. Permit at most three list attempts;
   only an empty candidate set permits another attempt. Each attempt remains
   subject to context cancellation and existing bounded transport retries.
   A proven pre-mutation authentication/permission rejection does not trigger
   discovery. Do not treat generic BAD_REQUEST as proven absence: the create
   router can wrap an error after its database transaction committed.
5. Require exactly one new parent ID, absent from the snapshot, with the marker,
   GitLab type, expected organization/user, and complete matching summary. Fetch
   `gitlab.one` and require matching relation/parent IDs, consistent nested parent,
   marker/type/organization/user, and matching complete requested configuration.
   No omitted credential may be treated as an ownership match. Ambiguity,
   malformed data, or mismatches fail immediately rather than retrying.
6. Only that complete verification establishes an owned identity. Rename using
   `gitlab.update` with verified IDs and requested settings; read back again and
   require stable identity. Return actual observed state, including readiness.

Do not automatically authorize OAuth, retry create, or delete any failed or
uncertain candidate. Once identity is verified, subsequent failures return its
IDs and last verified state via `ResourceInitFailedError`. If initial creation
was uncertain but an owned identity is verified, return partial state with the
original failure and do not perform the rename. Without verified identity, return
no resource ID and a sanitized operator-action diagnostic; do not adopt a marker
candidate on weaker evidence. Record only verified observed fields as actual state.

### Read and import

Read by the Pulumi/GitLab ID. Validate the returned relation ID equals the request,
parent ID equals the nested parent, provider type is GitLab, and organization is
consistent with active organization and any previously recorded identity. A
known parent/organization identity changing fails closed, not silent adoption.
An ID-only import establishes those identities from the validated record.

Reconstruct observed configuration, canonicalizing null groupName to an empty
string and null internal URL to absent. Preserve a prior application secret if
the response omits/nulls it; never substitute a placeholder. For an import with
no observable secret or other required OAuth settings, document that the user
must supply those settings before an update can succeed. Read itself need not
reject an otherwise valid incomplete integration solely for missing OAuth config.
Project only whitelisted resource fields; tokens and unrelated response fields
must never enter inputs/state or diagnostics.

A genuine not-found response removes the resource from state. Authorization,
transport, malformed-response, or identity failures are errors, not absence.

### Update

Validate inputs, read/verify existing identity and active organization before
mutation, then send a complete managed configuration with both verified IDs.
Send groupName as an explicit string, including empty to clear; send explicit
null to clear gitlabInternalUrl. Map applicationSecret to secret and never include
tokens. Do not toggle sharing or overwrite unrelated parent fields.

Read back after acknowledgement. Because parent and relation updates are not
atomic, an update failure must not claim requested settings were applied. Attempt
a safe verified readback on failure when the context permits and retain the
actual observed/last verified state under the provider's partial-failure contract.
Never retry the mutation. Successful updates also verify all managed fields
against the desired canonical configuration; mismatches are partial failures.

### Delete

Read/verify the relation, parent, type, and organization first. Remove only the
verified parent ID; its cascade removes the GitLab relation. If pre-read proves
the relation already absent, succeed without a remove call. A remove error is
treated as success only when a subsequent verified read proves absence; the
router wraps removal failures as BAD_REQUEST, so do not classify every such
failure as not found. Otherwise report a safe error and preserve identity.

Do not delete workloads, revoke GitLab-side OAuth grants, or change permissions.
Pulumi references provide dependent-workload destruction ordering. Warn that
unmanaged workloads may still reference an integration being destroyed.

## Secrets and diagnostics

Protect applicationSecret and internal URL with schema and dependency annotations.
Do not print settings, private URLs, IDs, temporary markers, tokens, request bodies,
or full upstream errors. The global text redactor does not cover every OAuth
field name; use resource-scoped diagnostics with operation and safe failure
classification/status, not arbitrary server text. Classify not-found before
discarding unsafe text. Test credential/token-bearing malformed responses and
error messages, including secrets observed on import without prior state.

Secret omission during refresh must not drop a known Pulumi secret. Token refresh
outside Pulumi changes only the observed boolean as appropriate, not managed
inputs or a credential-rotation diff.

## Documentation and examples

Update provider metadata and README resource/limitation descriptions. Extend
Application/Compose source guidance, imports, lifecycle/state, and troubleshooting
with integration usage, ID distinctions, required permissions, manual OAuth,
reauthorization, secret handling, and uncertain-create recovery.

Explain the two-stage workflow explicitly: create the integration, authorize it
in Dokploy's UI, then deploy dependent workloads. An initial single Pulumi update
creating both integration and workload cannot guarantee deployment success just
because there is an ID dependency. isConfigured is not an automatic waiter.

Include a focused example of configuration/secret inputs and a workload reference,
with documentation separating the authorization stage. Follow the repository's
canonical YAML and generated language-example conventions. Regenerate website
reference docs from schema; do not hand-edit generated reference pages.

Preserve the existing uncommitted changes in README and the lifecycle/state,
applications, imports, and troubleshooting guides. Inspect and layer focused
changes rather than replacing those files or resetting user work.

## Regression coverage and validation

Offline tests cover known/computed input validation, defaults, in-place diffs,
secret metadata, computed create outputs, stable preview IDs, and no preview API
calls. Scripted API tests assert exact request fields and ordering for CRUD/import,
manual-authorization readiness, omitted secrets, optional clears, token exclusion,
and existing Application/Compose references including computed integration outputs.

Creation safety cases include missing/duplicate identities, wrong organization,
wrong user/type, prior IDs, summary/detail disagreement, matching display names,
multiple marker candidates, missing credentials, unknown-create recovery,
discovery timeout, rename failure, and readback failure. Update/delete tests cover
identity mismatches, split-update failures, missing resources, error classification,
and no retries or unrelated mutations. OpenAPI tests assert selected operations,
corrected fields/nullable shapes, and bodyless acknowledgments.

Use the pinned tools and applicable Makefile targets: `make lint`,
`make test_provider`, `make test_race`, `make provider`, `make check_openapi`,
`make check_codegen`, `make build_sdks`, `make test_examples`, and `make docs_check`.
Check generated changes, run `git diff --check`, and distinguish feature changes
from pre-existing user edits. Report checks that are unavailable rather than
claiming success. No release/publication or live acceptance workflows are run.

Important validation safety: `.mise.toml` loads `.env` and sets
`DOKPLOY_ACCEPTANCE=1`. Do not invoke offline tests in that inherited environment.
The implementation plan must establish a verified sanitized tooling configuration
that does not load protected credentials and forces live opt-in off for every
test command, including nested Makefile `mise exec` calls. Do not read or modify
the protected `.env` file.

When implementation is finished, ask whether to retain or delete this task's
spec/plan documents. Delete only explicitly approved related documents; preserve
unrelated specs/plans and user changes.
