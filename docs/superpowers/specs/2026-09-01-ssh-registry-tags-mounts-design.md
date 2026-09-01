# SSH, Registry, Tags, And Mounts Design

## Summary

Extend the Dokploy Pulumi provider with first-class management for SSH keys,
container registries, project tags, and workload mounts. The milestone adds five
resources:

- `SSHKey`
- `Registry`
- `Tag`
- `ProjectTag`
- `Mount`

It also connects managed SSH keys and registries to existing Application and
Compose resources. The design preserves the provider's existing inferred-schema,
normalized-OpenAPI, explicit-lifecycle, and runtime-deployment patterns.

## Goals

- Give each independently managed Dokploy object a stable Pulumi resource and
  import identity.
- Keep many-to-many project tag ownership explicit and conflict-free.
- Make managed SSH keys usable by generic Git Application and Compose sources.
- Make managed registries usable for Application image and build registry
  configuration.
- Make mount changes effective in the target runtime during the same
  `pulumi up`.
- Preserve private keys, passwords, and file contents as Pulumi secrets through
  CRUD, refresh, import, errors, generated SDKs, and documentation.
- Cover API behavior with scripted lifecycle tests and environment-gated live
  Dokploy tests.

## Non-Goals

- Generate SSH key pairs through Dokploy.
- Add organization resources or configurable organization selection.
- Add list functions or data sources for keys, registries, tags, or mounts.
- Add bulk project tag ownership.
- Add registry test resources or expose registry testing as an invoke.
- Add LibSQL mounts before the provider manages LibSQL.
- Add MongoDB mounts. They remain deferred because current upstream Dokploy
  mount path handling has regressed for MongoDB, and the provider should not
  expose a target until bind, volume, and file behavior can be verified safely.
- Expose Dokploy's raw `serviceType` and `serviceId` mount pair.

## Architecture

The provider adds one focused implementation file per primary resource area and
registers all five resources in `provider.Provider`. Shared mount target dispatch
belongs in a small common helper because target resolution and deployment are
used by Mount create, update, and delete.

The resource boundaries are:

- `SSHKey` owns one supplied key pair and its Dokploy metadata.
- `Registry` owns one cloud container registry and its credentials.
- `Tag` owns one reusable tag definition.
- `ProjectTag` owns exactly one project-to-tag association and no other project
  tags.
- `Mount` owns one bind, volume, or file mount attached to one workload.

Existing resources receive only narrow optional references. Generic Git
Application sources gain `sshKeyId`; generic Git Compose sources retain their
existing `sshKeyId`. Application gains `registryId` and `buildRegistryId`.
These additions do not change existing programs.

## OpenAPI Contract

Add only the operations needed by the resource lifecycles:

- `organization.active`
- `sshKey.create`, `sshKey.one`, `sshKey.update`, and `sshKey.remove`
- `registry.create`, `registry.one`, `registry.update`, `registry.remove`, and
  `registry.testRegistry`
- `tag.create`, `tag.one`, `tag.update`, `tag.remove`,
  `tag.assignToProject`, and `tag.removeFromProject`
- `mounts.create`, `mounts.one`, `mounts.update`, and `mounts.remove`

Existing `project.one` and workload deployment operations are reused. Do not
select list operations, SSH generation, bulk tag assignment, registry testing by
ID, or mount listing.

The upstream OpenAPI document declares empty successful response objects for
these operations even though the routers return created or observed records.
Checked-in corrections must describe response fields observed in the pinned
Dokploy source and confirmed by live tests. Create operations return the created
record, so each resource takes its stable ID directly from the corrected create
response rather than inferring it through list differences.

## SSHKey Resource

Inputs:

- `name`, required and non-empty
- `description`, optional
- `privateKey`, required, non-empty, and secret
- `publicKey`, required and non-empty

Outputs add:

- `sshKeyId`
- `organizationId`

Dokploy requires `organizationId` during creation. The provider calls
`organization.active` immediately before `sshKey.create` and uses the active
organization ID. This is deterministic because provider API keys are scoped to
an organization. The discovered ID is recorded as observed state, not accepted
as desired input. Read uses the organization attached to the SSH key and does
not introduce drift if the active-organization endpoint later returns a
different value.

Name and description update in place. Private or public key changes replace the
resource because `sshKey.update` cannot rotate key material. Read preserves the
prior private key when Dokploy omits or redacts it. Private key values from
imports remain secret if Dokploy returns them; if Dokploy does not return key
material, import records the observable fields and a subsequent program must
supply key material before it can manage replacement.

Generic Git Application sources gain optional `sshKeyId`. Changing it uses the
existing source update and redeployment lifecycle. Compose already exposes the
same reference and needs no schema migration.

## Registry Resource

Inputs:

- `name`, required and non-empty
- `username`, required and non-empty
- `password`, required, non-empty, and secret
- `url`, required and non-empty
- `imagePrefix`, optional
- `serverId`, optional

Outputs add `registryId`. Dokploy currently supports only the `cloud` registry
type, so the provider sends it as an internal constant rather than exposing a
single-value input.

Create calls `registry.testRegistry` before `registry.create`, preventing an
unusable credential record from being persisted. Update tests before mutation
when username, password, URL, image prefix, or server changes. A name-only
update skips credential testing. All fields update in place. Read preserves the
prior password when Dokploy omits or redacts it.

Application gains optional `registryId` and `buildRegistryId` inputs mapped to
the existing `application.update` fields. Changes update in place and trigger
redeployment because they affect image retrieval or build publication. The
provider does not expose `rollbackRegistryId` in this milestone.

## Tag And ProjectTag Resources

`Tag` inputs are required non-empty `name` and optional `color`; output adds
`tagId`. Both inputs update in place. Color remains an opaque string because
Dokploy's API defines no color format constraint.

`ProjectTag` inputs are required non-empty `projectId` and `tagId`. Both are
replacement-only. Its Pulumi ID and import syntax are the deterministic
composite `<projectId>/<tagId>`.

Create calls `tag.assignToProject`; delete calls `tag.removeFromProject`. Read
uses `project.one` and checks the project's observed nested tags for the target
tag ID. The Project OpenAPI response correction is extended only enough to
decode tag IDs. A missing association removes `ProjectTag` from state; a missing
project or tag has the same result. The resource never uses bulk assignment and
therefore never removes associations owned elsewhere.

## Mount Resource

Inputs:

- `type`, required: `bind`, `volume`, or `file`
- `mountPath`, required and non-empty
- exactly one target ID: `applicationId`, `composeId`, `postgresId`, `mysqlId`,
  `mariadbId`, or `redisId`
- `hostPath`, required and non-empty only for `bind`
- `volumeName`, required and non-empty only for `volume`
- `filePath`, required and non-empty only for `file`
- `content`, required only for `file` and always secret; an explicitly supplied
  empty string is valid

Output adds `mountId`. The provider derives Dokploy `serviceType` and
`serviceId` from the selected typed target. Validation rejects missing or
multiple targets, unsupported targets, fields belonging to another mount type,
and missing type-specific values. Validation defers checks for computed Pulumi
inputs until values are known.

Target and type changes replace the resource. Replacement deletes before
creating to ensure file artifacts are removed and duplicate mount paths are not
temporarily attached. Mount path and type-specific value changes update in
place. Update sends explicit nulls for irrelevant fields so stale bind, volume,
or file values do not survive transitions supported within a type.

Create, update, and delete redeploy the target after the mount mutation:

- Application uses `application.redeploy`.
- Compose uses `compose.redeploy`.
- Postgres, MySQL, MariaDB, and Redis use their deploy operations.

The provider waits for the target's existing deployment status to reach `done`.
If delete finds that the mount is already absent, it still redeploys the target
from the target ID retained in Pulumi state. This lets a retry finish a
redeployment that failed after a prior delete removed the mount. If the target
has also disappeared, deletion succeeds without redeployment.

If mount mutation succeeds but readback or redeployment fails, create and update
return partial state containing the mount ID and requested configuration. A
later refresh can recover the observed mount. When post-delete redeployment
fails, Delete returns the error so Pulumi retains the resource state; a retry
observes the missing mount, redeploys from the retained target ID, and then
completes deletion without recreating the mount.

## Lifecycle And State

Preview performs defaults, conditional validation, and detailed diffing without
network mutations. Active-organization discovery, registry testing, resource
mutation, and deployment occur only outside preview.

Every normal resource uses its Dokploy ID as its Pulumi ID. `ProjectTag` is the
only composite-identity resource. Read returns an empty ID on a confirmed 404.
Authorization, malformed responses, and transient failures remain errors rather
than being interpreted as deletion.

Reads normalize API nulls and omitted values consistently with existing
resources. Volatile timestamps and last-used fields are not desired state and
do not participate in diffs. Stable observed identifiers are outputs.

## Secrets And Error Handling

The following fields are always secret:

- `SSHKey.privateKey`
- `Registry.password`
- `Mount.content`

Provider dependency wiring marks these values secret in state and generated
SDKs. Read preserves prior secret values when the API omits or redacts them.
Import marks any returned values secret and never copies them into diagnostics.

Errors from create, read, update, validation prerequisites, and deployment are
sanitized against current and prior secret values. Registry testing and SSH key
creation receive the same sanitization as CRUD calls. Request bodies containing
these values are never logged by provider code or test failures.

## Testing

### OpenAPI And Schema

- Assert the exact selected operation set and corrected successful responses.
- Assert generated request nullability supports clearing optional values.
- Assert all five resource tokens, descriptions, fields, defaults, secret
  markers, and replacement markers.
- Assert Application and Compose integration fields in generated schema.

### Resource Tests

- Exercise Check and Diff for valid, invalid, computed, update, and replacement
  cases.
- Exercise create, read, update, delete, import, refresh, and 404 behavior for
  all resources.
- Verify SSH active-organization lookup ordering and key replacement.
- Verify registry preflight ordering, name-only test skipping, redacted password
  preservation, and sanitized failures.
- Verify ProjectTag reads and deletes one association without changing unrelated
  tags.
- Cover every supported mount target and all three mount types with scripted
  tests.
- Verify explicit mount field clearing, delete-before-replace behavior, target
  deployment dispatch, polling, target disappearance, and partial failures.
- Verify Application SSH and registry changes and Compose SSH references use the
  expected API request and redeployment sequence.

### Live Tests

Environment-gated live tests create, update, refresh, import, and delete
representative resources for SSH, registries, tags, tag assignments, and mounts.
Mount live coverage exercises bind, volume, and file mounts on a disposable
Application; scripted tests cover the complete target matrix.

Registry live tests use dedicated registry URL, username, password, and optional
prefix environment variables. When the full live suite is explicitly enabled,
missing registry prerequisites fail with a clear message rather than silently
skipping the feature area. Live tests must clean up resources in dependency
order even after an assertion failure.

## Documentation And Generated Artifacts

After schema changes:

- regenerate the provider schema and Go, Node.js, Python, .NET, and Java SDKs;
- regenerate website resource and type reference pages;
- update provider metadata and README capability lists;
- add examples showing SSH-backed generic Git, registry references, project tag
  assignment, and each mount type without committing real credentials; and
- document registry live-test prerequisites and the deliberate MongoDB and
  LibSQL mount exclusions.

## Delivery Order

1. Expand the normalized OpenAPI subset, corrections, generated client, and
   contract tests.
2. Add `SSHKey`, `Registry`, `Tag`, and `ProjectTag` with scripted tests.
3. Add Application and Compose SSH and registry integrations.
4. Add shared mount target dispatch and `Mount` with scripted tests.
5. Add environment-gated live coverage.
6. Regenerate schema, SDKs, examples, and website reference documentation.
7. Run formatting, unit, lifecycle, code-generation consistency, example,
   website, and live verification commands.

## Compatibility

The milestone requires no migration or backward-compatibility adapter. Existing
resources only gain optional fields, Compose retains its existing SSH key field,
and all five managed types are new resource tokens.
