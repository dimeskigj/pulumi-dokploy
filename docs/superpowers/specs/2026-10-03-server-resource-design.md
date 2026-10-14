# Configuration-only Dokploy Server resource

## Intent and approved scope

Add `dokploy:index:Server` so Pulumi programs can declaratively register remote
deployment and build servers in Dokploy, reference managed SSH keys, and pass a
managed server ID to existing workloads. The resource owns the Dokploy server
record, not the remote machine or its readiness for deployment.

The user approved:

- Configuration-only CRUD and import by server ID.
- Inputs for name, description, address, SSH connection settings, server type,
  and Docker-cleanup configuration.
- Defaults of SSH port `22`, username `root`, server type `deploy`, and Docker
  cleanup disabled.
- In-place configuration updates and flat metadata outputs.
- Mocked lifecycle/error coverage, generated API client and all five SDKs,
  examples, and reference documentation.
- Offline verification only; no live acceptance tests during this work.

Success means the resource is available in the provider schema and Go, Node.js,
Python, .NET, and Java SDKs, can refresh/import a server record, and preserves
acknowledged identity through partial initialization failures. Existing resource
contracts remain unchanged.

## Architecture and alternatives

Implement one inferred resource in `provider/server.go`, registered through the
existing client factory in `provider/provider.go`. Use typed generated requests
and explicit response projection, following the provider's existing resource
patterns. Keep validation, request construction, response projection, and error
handling locally understandable; reuse existing helpers where appropriate and
avoid unrelated refactoring.

Chosen approach: one CRUD resource using `server.create`, `server.one`,
`server.update`, and `server.remove`.

Alternatives not selected:

1. Broader configuration coverage including `buildsConcurrency`. This requires a
   separate mutation and more partial-update handling; defer it from this release.
2. Bootstrap or operational resources for setup, security, monitoring, or cluster
   actions. These exceed the approved configuration-only scope.

No `command`, connection-validation operation, deployment wait, monitoring
configuration, raw response object, or lookup function is included. The existing
read-only lookup design/plan, including `getServer`, is separate work and must
remain untouched.

## Public resource contract

| Input | Type | Requirement/default | Behavior |
| --- | --- | --- | --- |
| `name` | string | Required | Nonempty after whitespace trimming |
| `description` | string | Optional | Omission clears the stored value to null |
| `ipAddress` | string | Required | Nonempty address; no IP-only restriction |
| `port` | integer | Optional, `22` | SSH port in `1..65535` |
| `username` | string | Optional, `root` | Nonempty after whitespace trimming |
| `sshKeyId` | string | Optional | Reference to a Dokploy SSH key; omission clears to null |
| `serverType` | string | Optional, `deploy` | `deploy` or `build` |
| `enableDockerCleanup` | boolean | Optional, `false` | Always sent explicitly, including false |

If provided, `sshKeyId` must be nonempty after whitespace trimming. Validation
does not rewrite values. Empty description is distinct from omission. Validation
must tolerate computed Pulumi inputs during preview and must not contact the API
or test SSH connectivity. Explicit empty values are not silently replaced with
defaults. Neither omission nor a zero Go value may erase an explicit boolean
choice or computed input.

All configuration fields are mutable in place; none causes replacement. Optional
string comparisons preserve the difference between absent and empty values.

Outputs contain the managed configuration and:

| Output | Type | Mapping |
| --- | --- | --- |
| `serverId` | string | Dokploy `serverId`; also the Pulumi resource ID |
| `organizationId` | string | Owning organization from the response |
| `status` | string | Dokploy `serverStatus`; descriptive, not readiness |

Organization selection follows the authenticated Dokploy session. There is no
organization input, status input, or create-time ID override. Status must not
gate resource creation or trigger host setup. SSH key material is managed by the
existing `SSHKey` resource and never appears in Server state.

## Lifecycle and data flow

### Create

1. Dry-run returns projected configuration without constructing/calling a client.
2. Send one `server.create` request containing effective defaults, explicit
   nullable description/SSH key fields, and an explicit cleanup boolean.
3. Require an acknowledged nonempty `serverId` in the successful response. Do not
   list servers, discover by name, retry the mutation, or adopt an unverified ID.
4. Read the record with `server.one` and return its projected configuration and
   metadata. Do not wait for activation, contact the host, or call setup.
5. If the follow-up read fails, is incomplete, has mismatched identity, or reports
   absence, return the acknowledged ID, best available safe state, and the
   provider's partial-initialization error. Never discard known identity.

Fallback create state starts with the submitted effective configuration and
acknowledged ID. Populate organization/status and persisted configuration only
from present, correctly typed fields in a matching acknowledged response; missing
metadata remains unknown rather than being invented.

If creation fails before identity is acknowledged, report the failure without
inventing identity or retrying creation. A successful but identity-free response
is a contract error, not proof that nothing was created.

### Read, refresh, and import

Use `server.one` with the requested ID. A classified not-found response returns
an empty resource ID. Other failures remain errors. A successful response must
contain a nonempty ID matching the requested ID and the required persisted flat
fields: name, address, port, username, server type, cleanup boolean, organization
ID, and status. Missing/null required fields must not silently become empty
strings, false, zero, or provider creation defaults. Optional description and SSH
key values map null/absence to omission; preserve explicit empty descriptions.

Reconstruct inputs and state from the returned record, including on import when
there is no prior state. Status and type strings from reads are preserved rather
than restricted to a closed response enum; input validation still limits newly
requested server types. Ignore relations and fields outside the public contract.

### Update

Dry-run performs no API calls and retains existing identity/metadata while
projecting new configuration. Actual updates send one `server.update` request
with the resource ID and all managed configuration, including explicit nullable
clears and cleanup false. Never send `command` or other unmanaged properties.

After success, read back with `server.one`. Require response identity to match
when the update response supplies it. Preserve identity and best available safe
state if the write succeeded but readback fails; surface a partial-state error
using the inference framework's supported update semantics. A rejected write
must remain a failure and must not claim new configuration was persisted.

Fallback state after an acknowledged update starts with the requested effective
configuration and existing identity/metadata, overlaid only by safe present fields
from a matching response. If the mutation itself fails, retain prior state.

Dokploy can return `NOT_FOUND` from `server.update` for an inactive server that
still exists. Report this as an update failure, not successful removal or a cue
to recreate the record. Do not retry mutations automatically.

### Delete

Use `server.remove` without force flags, service deletion, remote teardown, or
setup/cleanup commands. Dokploy refuses removal while associated workloads exist;
propagate that refusal. An API-confirmed absent record is already deleted. If a
remove error is classified as not-found, confirm absence using `server.one`
before treating deletion as successful; an existing record or non-not-found read
failure must not be reported as successful deletion.

Document that removal deletes the Dokploy record and associated deployment
history/logs according to Dokploy behavior; it does not destroy the remote VM or
uninstall its software. Applications and other workloads can depend on
`server.serverId` for normal Pulumi dependency ordering.

## Upstream behavior and OpenAPI changes

Source verification used Dokploy commit
`48504fde4eb210056f7d9f80406f9692a1a7ea8a`:

- `apps/dokploy/server/api/routers/server.ts`
- `packages/server/src/db/schema/server.ts`
- `packages/server/src/services/server.ts`
- `packages/server/src/services/deployment.ts`
- `apps/dokploy/server/utils/docker-cleanup.ts`

Create/update return the persisted record; one additionally loads relations;
remove returns the removed record. The checked-in upstream OpenAPI snapshot has
empty success schemas, so add the four operations to `openapi/operations.txt` and
flat Server response corrections to `openapi/corrections.json`. Correct SSH port
to an integer in the normalized request contract without changing unrelated
operations or pretending the vendored upstream snapshot has been refreshed.
Regenerate `openapi/dokploy.json` and the generated Go client using the repository
targets; do not hand-edit generated output.

Docker cleanup is an important exception to the phrase configuration-only:
Dokploy create/update apply a recurring cleanup schedule when the boolean is
true. The provider defaults it to false and always sends it explicitly to avoid
the API's true default. Document that enabling this setting authorizes Dokploy's
scheduled cleanup behavior. The provider itself never runs cleanup commands.

## Security and failure handling

Only project the documented flat fields into resource state and diagnostics.
Never expose nested SSH keys, deployments, monitoring tokens/configuration,
commands, or additional upstream fields through generic maps or generated SDK
outputs. Server inputs contain no private key/password material.

Do not log raw request/response bodies, private addresses/URLs, or resource IDs.
Use operation-specific safe errors that preserve useful failure classification
without echoing arbitrary upstream text containing nested credentials or private
connection details. Keep cancellation/timeouts and existing bounded GET retry
behavior. Do not broaden retries to mutations.

## Tests and documentation

Use the existing scripted HTTP test server to verify:

- Defaults, explicit false, omitted/empty fields, computed inputs, and validation
  failures without network access.
- In-place diffs for every managed field and no diff for unchanged configuration.
- Create/read/update/delete request methods, paths, query parameters, and exact
  payloads, including nullable clears and integer ports.
- Refresh and import reconstruction, persisted defaults, drift, missing records,
  mismatched identities, incomplete/malformed responses, and unknown status/type
  strings on read.
- Acknowledged create identity and update state retention on readback failures;
  missing create identity must not trigger retry/discovery.
- Inactive-server update failure, deletion refusal for workloads, and confirmed
  absence versus other deletion errors.
- Dry-run network silence, cancellation, no mutation retries, and exclusion of
  nested credentials/unmanaged fields from state and diagnostics.
- Schema registration, property documentation/defaults, mutable fields, flat
  outputs, and all generated SDKs.

Update provider/README resource lists, import guidance, and website reference
documentation. Add illustrative server configuration examples using reserved
addresses and an `SSHKey` output; clearly separate registration from bootstrap
and explain cleanup opt-in and destroy behavior. Follow existing example source
and generation conventions rather than hand-editing generated examples.

## Verification and safety

Use pinned tools from `.mise.toml` and repository targets. Relevant checks are
`make lint`, `make test_provider`, `make test_race`, `make provider`,
`make check_openapi`, `make check_codegen`, `make build_sdks`,
`make test_examples`, and `make docs_check`. Inspect generated diffs and run
`git diff --check` and `git status --short` before completion. Report unavailable
checks explicitly; offline tests do not establish deployed-version compatibility.

Do not read `.env`, use live credentials, run live acceptance tests, or execute
release/publication workflows. Preserve pre-existing design/plan documents and
user changes. At implementation completion, ask whether to retain or remove only
the task-specific specification and implementation plan; do not delete them
without approval.
