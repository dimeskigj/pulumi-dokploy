# SSH, Registry, Tags, And Mounts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class SSH key, registry, tag, project-tag, and mount management to the Dokploy provider, including Application integrations, generated SDKs, documentation, and live verification.

**Architecture:** Add five inferred resources around a corrected, allowlisted Dokploy OpenAPI client. Keep global objects and project associations independent, extend Application with optional SSH and registry references, and dispatch Mount redeployments through one typed target helper. Preserve the existing explicit Check/Diff/CRUD, partial-state, polling, and secret-sanitization patterns.

**Tech Stack:** Go 1.25, `pulumi-go-provider/infer`, `oapi-codegen`, JSON OpenAPI normalization, scripted HTTP tests, Pulumi package generation, Astro/Starlight documentation, Python CI normalization scripts.

## Global Constraints

- Implement `SSHKey`, `Registry`, `Tag`, `ProjectTag`, and `Mount` as new `dokploy:index` resource tokens.
- SSH accepts supplied key pairs only; `organization.active` supplies the organization ID.
- `SSHKey.privateKey`, `Registry.password`, and `Mount.content` are always Pulumi secrets and must be sanitized from errors.
- Project tag ownership is one association per `ProjectTag`; never use bulk assignment.
- Mount targets are Application, Compose, Postgres, MySQL, MariaDB, and Redis only; MongoDB and LibSQL are excluded.
- Mount create, update, delete, and delete retry must redeploy the target and wait for `done`.
- Existing resource inputs are backward-compatible optional additions only.
- Use TDD for every task: add a focused failing test, run it, implement the smallest behavior, rerun the focused and package tests, then commit.
- Do not modify or commit the pre-existing untracked plans `docs/superpowers/plans/2026-08-27-dokploy-provider-mvp.md` and `docs/superpowers/plans/2026-08-30-live-dokploy-bug-reports.md`.

---

## File Structure

- `openapi/cmd/normalize/main.go`: select allowlisted operations and apply response plus request-body schema corrections.
- `openapi/corrections.json`: corrected Organization, SSHKey, Registry, Tag, Mount, Project tags, Application registry fields, and nullable registry update request schemas.
- `provider/ssh_key.go`: SSH key Check, Diff, CRUD, secret preservation, organization discovery, and dependency wiring.
- `provider/registry.go`: registry Check, Diff, preflight validation, CRUD, secret preservation, and dependency wiring.
- `provider/tag.go`: Tag lifecycle.
- `provider/project_tag.go`: one project/tag association with composite identity.
- `provider/mount_target.go`: typed target resolution, deploy dispatch, status reads, and confirmed-404 handling.
- `provider/mount.go`: mount validation, lifecycle, secret handling, partial state, and delete retry.
- Existing Application files: optional Git SSH and registry references plus redeployment.
- `provider/live_test.go`: gated end-to-end feature lifecycles.
- Generated schema, SDK, examples, and website files remain generator-owned where the repository already generates them.

---

### Task 1: Expand The Normalized OpenAPI Contract

**Files:**
- Modify: `openapi/operations.txt`
- Modify: `openapi/corrections.json`
- Modify: `openapi/cmd/normalize/main.go`
- Modify: `openapi/cmd/normalize/main_test.go`
- Regenerate: `openapi/dokploy.json`
- Regenerate: `internal/client/generated/generated.gen.go`

**Interfaces:**
- Produces generated request/response types for `OrganizationActive`, `SshKey*`, `Registry*`, `Tag*`, and `Mounts*` operations.
- Produces `Project.Tags`, `Application.RegistryId`, and `Application.BuildRegistryId` response fields.
- Produces nullable `RegistryUpdateJSONRequestBody.ServerId` so a configured server can be cleared.

- [ ] **Step 1: Write failing normalizer tests**

Add exact allowlist and correction assertions in `openapi/cmd/normalize/main_test.go`:

```go
func TestNormalizeSelectsNewResourceOperations(t *testing.T) {
	for _, operation := range []string{
		"organization.active", "sshKey.create", "sshKey.one", "sshKey.update", "sshKey.remove",
		"registry.create", "registry.one", "registry.update", "registry.remove", "registry.testRegistry",
		"tag.create", "tag.one", "tag.update", "tag.remove", "tag.assignToProject", "tag.removeFromProject",
		"mounts.create", "mounts.one", "mounts.update", "mounts.remove",
	} {
		require.Contains(t, normalizedOperationIDs(t, output), operation)
	}
}

func TestNormalizeCorrectsRegistryUpdateServerIDToNullable(t *testing.T) {
	schema := operationRequestSchema(t, output, "registry.update")
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, schema, "serverId"))
}
```

Use existing fixture helpers where available; if missing, add small JSON-map helpers in the test file rather than production code.

- [ ] **Step 2: Run tests and confirm failure**

Run: `mise exec -- go test ./openapi/cmd/normalize -count=1`

Expected: FAIL because operations and request correction support are absent.

- [ ] **Step 3: Add request correction support**

Extend the corrections model and normalization pass:

```go
type Corrections struct {
	Responses map[string]string `json:"responses"`
	Requests  map[string]string `json:"requests"`
	Schemas   map[string]any    `json:"schemas"`
}
```

For each `Requests[operationID]`, replace the operation's `requestBody.content.application/json.schema` with `{"$ref":"#/components/schemas/<name>"}` after selected components are copied. Return a descriptive error when the operation, request body, or named correction schema is missing.

- [ ] **Step 4: Add operations and schemas**

Append the 20 operation IDs listed in the test. Add compact correction schemas with these required stable fields:

```json
{
  "Organization": {"type":"object","properties":{"organizationId":{"type":"string"}},"required":["organizationId"],"additionalProperties":true},
  "SSHKey": {"type":"object","properties":{"sshKeyId":{"type":"string"},"name":{"type":"string"},"description":{"type":["string","null"]},"privateKey":{"type":"string"},"publicKey":{"type":"string"},"organizationId":{"type":"string"}},"required":["sshKeyId"],"additionalProperties":true},
  "Registry": {"type":"object","properties":{"registryId":{"type":"string"},"registryName":{"type":"string"},"username":{"type":"string"},"password":{"type":"string"},"registryUrl":{"type":"string"},"registryType":{"type":"string"},"imagePrefix":{"type":["string","null"]},"serverId":{"type":["string","null"]}},"required":["registryId"],"additionalProperties":true},
  "Tag": {"type":"object","properties":{"tagId":{"type":"string"},"name":{"type":"string"},"color":{"type":["string","null"]}},"required":["tagId"],"additionalProperties":true},
  "Mount": {"type":"object","properties":{"mountId":{"type":"string"},"type":{"type":"string"},"hostPath":{"type":["string","null"]},"volumeName":{"type":["string","null"]},"filePath":{"type":["string","null"]},"content":{"type":["string","null"]},"mountPath":{"type":"string"},"serviceType":{"type":"string"},"applicationId":{"type":["string","null"]},"composeId":{"type":["string","null"]},"postgresId":{"type":["string","null"]},"mysqlId":{"type":["string","null"]},"mariadbId":{"type":["string","null"]},"redisId":{"type":["string","null"]}},"required":["mountId"],"additionalProperties":true}
}
```

Map create/one/update responses to their corresponding model and `organization.active` to `Organization`. Extend the existing corrected `Project` schema with `tags: [{tagId: string}]` and corrected `Application` with nullable `registryId` and `buildRegistryId`. Add `RegistryUpdateRequest` with all upstream update fields but nullable `serverId`, then map `registry.update` in `requests`.

- [ ] **Step 5: Regenerate and test the client**

Run:

```bash
mise exec -- make generate_openapi
mise exec -- go test ./openapi/cmd/normalize ./internal/client -count=1
mise exec -- make check_openapi
```

Expected: all commands PASS and generated types expose the interfaces above.

- [ ] **Step 6: Commit**

```bash
git add openapi/operations.txt openapi/corrections.json openapi/cmd/normalize/main.go openapi/cmd/normalize/main_test.go openapi/dokploy.json internal/client/generated/generated.gen.go
git commit -m "feat: add SSH registry tag and mount API contracts"
```

---

### Task 2: Add The SSHKey Resource

**Files:**
- Create: `provider/ssh_key.go`
- Create: `provider/ssh_key_test.go`
- Modify: `provider/provider.go`
- Modify: `provider/provider_test.go`
- Modify: `provider/schema_test.go`

**Interfaces:**
- Produces `SSHKeyArgs`, `SSHKeyState`, and inferred resource token `dokploy:index:SSHKey`.
- Consumes generated `OrganizationActive`, `SshKeyCreate`, `SshKeyOne`, `SshKeyUpdate`, and `SshKeyRemove` methods.

- [ ] **Step 1: Write failing validation, diff, lifecycle, and secret tests**

Cover empty fields, computed preview inputs, name/description updates, key replacement, organization lookup before create, 404 refresh/delete, omitted private-key preservation, import, and current/prior secret redaction. Assert request order:

```text
GET /api/organization.active
POST /api/sshKey.create
GET /api/sshKey.one?sshKeyId=k1
POST /api/sshKey.update
POST /api/sshKey.remove
```

Assert Diff returns `p.UpdateReplace` for `privateKey` and `publicKey`, and `p.Update` for metadata.

- [ ] **Step 2: Verify focused tests fail**

Run: `mise exec -- go test ./provider -run 'TestSSHKey|TestSchema' -count=1`

Expected: FAIL because `SSHKey` is not implemented or registered.

- [ ] **Step 3: Implement the resource**

Use these public shapes:

```go
type SSHKeyArgs struct {
	Name        string  `pulumi:"name"`
	Description *string `pulumi:"description,optional"`
	PrivateKey  string  `pulumi:"privateKey" provider:"secret"`
	PublicKey   string  `pulumi:"publicKey"`
}

type SSHKeyState struct {
	SSHKeyArgs
	SSHKeyID      string `pulumi:"sshKeyId"`
	OrganizationID string `pulumi:"organizationId"`
}
```

Implement `Annotate`, `Check`, explicit `Diff`, `Create`, `Read`, `Update`, `Delete`, and `WireDependencies`. Create must resolve `organization.active`, reject an incomplete organization, create the key, and use `initFailed` if a post-create read fails. Read preserves `req.State.PrivateKey` when the response omits it. Sanitize all API errors against current and prior private key values.

- [ ] **Step 4: Register and assert the schema**

Add `infer.Resource(&SSHKey{client: configuredClient})`, update package metadata, add the resource to exact registration assertions, and verify `privateKey` is secret while both key fields replace.

- [ ] **Step 5: Run provider tests**

Run:

```bash
mise exec -- go test ./provider -run 'TestSSHKey|TestSchema|TestProvider' -count=1
mise exec -- go test ./provider -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add provider/ssh_key.go provider/ssh_key_test.go provider/provider.go provider/provider_test.go provider/schema_test.go
git commit -m "feat: add SSH key resource"
```

---

### Task 3: Add The Registry Resource

**Files:**
- Create: `provider/registry.go`
- Create: `provider/registry_test.go`
- Modify: `provider/provider.go`
- Modify: `provider/provider_test.go`
- Modify: `provider/schema_test.go`

**Interfaces:**
- Produces `RegistryArgs`, `RegistryState`, and `dokploy:index:Registry`.
- Consumes nullable `RegistryUpdateJSONRequestBody.ServerId` from Task 1.

- [ ] **Step 1: Write failing tests**

Test required non-empty `name`, `username`, `password`, and `url`; all-update diffs; credential test before create; test-before-update for username/password/url/imagePrefix/serverId; no test for name-only update; clearing `imagePrefix` and `serverId`; 404 handling; redacted password preservation; import; and current/prior password sanitization.

- [ ] **Step 2: Verify focused tests fail**

Run: `mise exec -- go test ./provider -run 'TestRegistry|TestSchema' -count=1`

Expected: FAIL because the resource is absent.

- [ ] **Step 3: Implement Registry**

Use:

```go
type RegistryArgs struct {
	Name        string  `pulumi:"name"`
	Username    string  `pulumi:"username"`
	Password    string  `pulumi:"password" provider:"secret"`
	URL         string  `pulumi:"url"`
	ImagePrefix *string `pulumi:"imagePrefix,optional"`
	ServerID    *string `pulumi:"serverId,optional"`
}

type RegistryState struct {
	RegistryArgs
	RegistryID string `pulumi:"registryId"`
}
```

Send `registryType: "cloud"`. Factor a private `testRegistry(ctx, api, args)` used before create and relevant updates. Update uses nullable wrappers for `imagePrefix` and `serverId`. Read preserves prior password when absent. Wire `RegistryID` to all non-secret inputs and wire output password to the secret input.

- [ ] **Step 4: Register, test, and commit**

Run:

```bash
mise exec -- go test ./provider -run 'TestRegistry|TestSchema|TestProvider' -count=1
mise exec -- go test ./provider -count=1
```

Then:

```bash
git add provider/registry.go provider/registry_test.go provider/provider.go provider/provider_test.go provider/schema_test.go
git commit -m "feat: add container registry resource"
```

---

### Task 4: Add Tag And ProjectTag Resources

**Files:**
- Create: `provider/tag.go`
- Create: `provider/tag_test.go`
- Create: `provider/project_tag.go`
- Create: `provider/project_tag_test.go`
- Modify: `provider/provider.go`
- Modify: `provider/provider_test.go`
- Modify: `provider/schema_test.go`

**Interfaces:**
- Produces `TagArgs`, `TagState`, and `dokploy:index:Tag`.
- Produces `ProjectTagArgs`, `ProjectTagState`, `formatProjectTagID`, `parseProjectTagID`, and `dokploy:index:ProjectTag`.

- [ ] **Step 1: Write failing Tag tests**

Test non-empty name, opaque color, update diffs, CRUD, import, response validation, and 404 refresh/delete.

- [ ] **Step 2: Write failing ProjectTag tests**

Test composite identity and association isolation:

```go
func TestProjectTagIDRoundTrip(t *testing.T) {
	id := formatProjectTagID("p1", "t1")
	require.Equal(t, "p1/t1", id)
	p, tag, err := parseProjectTagID(id)
	require.NoError(t, err)
	require.Equal(t, "p1", p)
	require.Equal(t, "t1", tag)
}
```

Also test invalid import IDs, both fields replacing, assign/remove request bodies, project reads with target plus unrelated tags, association drift, and project 404.

- [ ] **Step 3: Verify focused tests fail**

Run: `mise exec -- go test ./provider -run 'TestTag|TestProjectTag|TestSchema' -count=1`

Expected: FAIL because resources are absent.

- [ ] **Step 4: Implement Tag and ProjectTag**

Use:

```go
type TagArgs struct {
	Name  string  `pulumi:"name"`
	Color *string `pulumi:"color,optional"`
}

type ProjectTagArgs struct {
	ProjectID string `pulumi:"projectId" provider:"replaceOnChanges"`
	TagID     string `pulumi:"tagId" provider:"replaceOnChanges"`
}
```

`ProjectTag.Read` must inspect corrected `Project.Tags` and return an empty ID when the association is absent. It must not call bulk assignment or remove unrelated tags. Parse imports with exactly two non-empty slash-separated segments.

- [ ] **Step 5: Register, test, and commit**

Run:

```bash
mise exec -- go test ./provider -run 'TestTag|TestProjectTag|TestSchema|TestProvider' -count=1
mise exec -- go test ./provider -count=1
```

Then:

```bash
git add provider/tag.go provider/tag_test.go provider/project_tag.go provider/project_tag_test.go provider/provider.go provider/provider_test.go provider/schema_test.go
git commit -m "feat: add project tag resources"
```

---

### Task 5: Integrate SSH Keys And Registries With Application

**Files:**
- Modify: `provider/application_source.go`
- Modify: `provider/application.go`
- Modify: `provider/application_source_test.go`
- Modify: `provider/application_test.go`
- Modify: `provider/compose_source_test.go`
- Modify: `provider/compose_test.go`
- Modify: `provider/lifecycle_test.go`
- Modify: `provider/schema_test.go`

**Interfaces:**
- Extends `GitApplicationSource` with `SSHKeyID *string` at Pulumi name `sshKeyId`.
- Extends `ApplicationArgs` with `RegistryID *string` and `BuildRegistryID *string`.
- Preserves the existing Compose Git `SSHKeyID` implementation.

- [ ] **Step 1: Write failing source and lifecycle tests**

Add Git Application request assertions for a set key and explicit null when cleared. Add Application read/import assertions for all three fields. Add registry update request plus redeploy/poll tests. Add one Compose regression proving the existing `sshKeyId` accepts an output-shaped reference without schema changes.

- [ ] **Step 2: Verify focused tests fail**

Run: `mise exec -- go test ./provider -run 'TestApplication|TestCompose|TestLifecycle|TestSchema' -count=1`

Expected: FAIL on missing Application fields or request values.

- [ ] **Step 3: Add Application SSH support**

Extend the source shape:

```go
type GitApplicationSource struct {
	URL              string            `pulumi:"url"`
	Branch           string            `pulumi:"branch"`
	BuildPath        *string           `pulumi:"buildPath,optional"`
	SSHKeyID         *string           `pulumi:"sshKeyId,optional"`
	WatchPaths       []string          `pulumi:"watchPaths,optional"`
	EnableSubmodules bool              `pulumi:"enableSubmodules,optional"`
	Build            *ApplicationBuild `pulumi:"build,optional"`
}
```

Encode `CustomGitSSHKeyId` as a nullable value and decode it from reads. Existing source equality must classify it as runtime-changing.

- [ ] **Step 4: Add Application registry support**

Add optional `registryId` and `buildRegistryId`, annotate them, include them in Diff and dependency wiring, decode them on Read, and include them as explicit nullable fields in `application.update`. Any change sets `runtimeChanged` and triggers the existing redeploy/poll path.

- [ ] **Step 5: Preserve old and new Application secrets**

Where this task touches Application update error handling, sanitize both `req.Inputs` and `req.State.ApplicationArgs` environment/build/source secrets so old credentials cannot leak from API errors.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
mise exec -- go test ./provider -run 'TestApplication|TestCompose|TestLifecycle|TestSchema' -count=1
mise exec -- go test ./provider -count=1
```

Then:

```bash
git add provider/application_source.go provider/application.go provider/application_source_test.go provider/application_test.go provider/compose_source_test.go provider/compose_test.go provider/lifecycle_test.go provider/schema_test.go
git commit -m "feat: connect applications to SSH keys and registries"
```

---

### Task 6: Add Typed Mount Management And Redeployment

**Files:**
- Create: `provider/mount_target.go`
- Create: `provider/mount_target_test.go`
- Create: `provider/mount.go`
- Create: `provider/mount_test.go`
- Modify: `provider/provider.go`
- Modify: `provider/provider_test.go`
- Modify: `provider/schema_test.go`
- Modify: `provider/lifecycle_test.go`

**Interfaces:**
- Produces `MountArgs`, `MountState`, and `dokploy:index:Mount`.
- Produces internal `mountTarget` with `serviceType`, `serviceID`, `deploy(context.Context,*client.Client) error`, and `status(context.Context,*client.Client) (string,error)`.
- Produces `deployMountTarget(ctx, api, target) (exists bool, err error)` for normal and delete-retry paths.

- [ ] **Step 1: Write failing target dispatch tests**

Table-test all six targets and expected operation bodies. Confirm a target read 404 returns `exists=false`, while malformed and transient responses return errors. Confirm each successful deployment polls the matching status helper.

- [ ] **Step 2: Write failing Mount Check and Diff tests**

Use this shape:

```go
type MountArgs struct {
	Type          string  `pulumi:"type" provider:"replaceOnChanges"`
	MountPath     string  `pulumi:"mountPath"`
	HostPath      *string `pulumi:"hostPath,optional"`
	VolumeName    *string `pulumi:"volumeName,optional"`
	FilePath      *string `pulumi:"filePath,optional"`
	Content       *string `pulumi:"content,optional" provider:"secret"`
	ApplicationID *string `pulumi:"applicationId,optional" provider:"replaceOnChanges"`
	ComposeID     *string `pulumi:"composeId,optional" provider:"replaceOnChanges"`
	PostgresID    *string `pulumi:"postgresId,optional" provider:"replaceOnChanges"`
	MySQLID       *string `pulumi:"mysqlId,optional" provider:"replaceOnChanges"`
	MariaDBID     *string `pulumi:"mariadbId,optional" provider:"replaceOnChanges"`
	RedisID       *string `pulumi:"redisId,optional" provider:"replaceOnChanges"`
}
```

Test exactly one target, computed target deferral, type-specific presence/absence, empty file content accepted only when explicitly supplied, update diffs, target/type replacement, and `DeleteBeforeReplace=true` for every replacement.

- [ ] **Step 3: Write failing lifecycle tests**

Cover create/read/update/delete for bind, volume, and file; every target dispatch; explicit nulls; secret preservation; import; 404 refresh; mutation-success/deploy-failure partial state; delete failure retaining state; and retry where mount is already absent but target redeploy still occurs.

- [ ] **Step 4: Verify focused tests fail**

Run: `mise exec -- go test ./provider -run 'TestMount|TestSchema|TestLifecycle' -count=1`

Expected: FAIL because Mount is absent.

- [ ] **Step 5: Implement target dispatch**

Resolve exactly one typed ID to Dokploy's service type and use existing `applicationStatus`, `composeStatus`, `postgresStatus`, `mysqlStatus`, `mariadbStatus`, and `redisStatus` functions. Deploy through the matching generated redeploy/deploy method, then call `waitForDone`.

- [ ] **Step 6: Implement Mount lifecycle**

Create sends derived `serviceType/serviceId`, captures `mountId`, reads back, redeploys, and returns `initFailed` with state if post-create work fails. Update sends all type-specific values using nullable wrappers, reads back, redeploys, and preserves partial state. Delete removes the mount if present, then always deploys from retained state; if the mount is already absent, skip removal but not deployment. Skip deployment only when the target read is a confirmed 404.

- [ ] **Step 7: Register, test, and commit**

Run:

```bash
mise exec -- go test ./provider -run 'TestMount|TestSchema|TestProvider|TestLifecycle' -count=1
mise exec -- go test ./provider -count=1
```

Then:

```bash
git add provider/mount_target.go provider/mount_target_test.go provider/mount.go provider/mount_test.go provider/provider.go provider/provider_test.go provider/schema_test.go provider/lifecycle_test.go
git commit -m "feat: add workload mount resource"
```

---

### Task 7: Add Gated Live Coverage And Registry Prerequisites

**Files:**
- Modify: `provider/live_test.go`
- Modify: `.ci-mgmt.yaml`
- Modify: `scripts/normalize_ci.py`
- Modify: `scripts/test_normalize_ci.py`
- Regenerate: `.github/workflows/run-acceptance-tests.yml`
- Regenerate as required: other `.github/workflows/*.yml`

**Interfaces:**
- Consumes `DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY`.
- Adds `DOKPLOY_REGISTRY_URL`, `DOKPLOY_REGISTRY_USERNAME`, `DOKPLOY_REGISTRY_PASSWORD`, and optional `DOKPLOY_REGISTRY_IMAGE_PREFIX` for full live Registry tests.

- [ ] **Step 1: Write failing CI-normalization tests**

Assert generated live-test workflow jobs forward the four registry variables, password comes from secrets, and ordinary non-live jobs do not receive credentials.

- [ ] **Step 2: Verify CI tests fail**

Run: `python3 -m unittest scripts/test_normalize_ci.py -v`

Expected: FAIL because registry variables are not wired.

- [ ] **Step 3: Add live lifecycle tests**

Add dependency-ordered cleanup and subtests for:

```text
SSHKey: create -> read -> metadata update -> import/read -> delete
Registry: credential test -> create -> read -> name update -> import/read -> delete
Tag/ProjectTag: create tag -> assign -> observe -> remove association -> delete tag
Mount: create/update/delete bind, volume, and file mounts on a disposable Application
```

Generate a disposable Ed25519 pair in test code using `crypto/ed25519`; never load a checked-in private key. Require all registry variables when the live feature suite is selected and report their names without values.

- [ ] **Step 4: Wire registry prerequisites and regenerate workflows**

Extend `.ci-mgmt.yaml` and `scripts/normalize_ci.py` following existing endpoint/API-key handling. Regenerate workflows through the repository's CI management target or normalization script; do not hand-edit generated workflow differences.

- [ ] **Step 5: Run non-live verification**

Run:

```bash
python3 -m unittest scripts/test_normalize_ci.py -v
mise exec -- go test ./provider -run TestLive -count=1
```

Expected: Python tests PASS; Go live tests cleanly skip only when the overall live gate is absent.

- [ ] **Step 6: Run live verification when credentials are present**

Run:

```bash
DOKPLOY_ENDPOINT="$DOKPLOY_ENDPOINT" DOKPLOY_API_KEY="$DOKPLOY_API_KEY" DOKPLOY_REGISTRY_URL="$DOKPLOY_REGISTRY_URL" DOKPLOY_REGISTRY_USERNAME="$DOKPLOY_REGISTRY_USERNAME" DOKPLOY_REGISTRY_PASSWORD="$DOKPLOY_REGISTRY_PASSWORD" DOKPLOY_REGISTRY_IMAGE_PREFIX="$DOKPLOY_REGISTRY_IMAGE_PREFIX" mise exec -- go test ./provider -run TestLive -v -count=1
```

Expected: PASS. If the environment is unavailable to the agent, record this exact command as unrun; do not claim live success.

- [ ] **Step 7: Commit**

```bash
git add provider/live_test.go .ci-mgmt.yaml scripts/normalize_ci.py scripts/test_normalize_ci.py .github/workflows
git commit -m "test: cover SSH registry tags and mounts live"
```

---

### Task 8: Regenerate SDKs, Examples, And Documentation

**Files:**
- Modify: `README.md`
- Modify: `examples/yaml/Pulumi.yaml`
- Modify: `examples/yaml/README.md`
- Modify: `examples/yaml_test.go`
- Modify: `provider/registry_metadata_test.go`
- Modify: `website/scripts/reference-model.mjs`
- Modify: `website/tests/reference-model.test.mjs`
- Modify: `website/astro.config.mjs`
- Modify: `website/tests/content.test.mjs`
- Modify curated guides under: `website/src/content/docs/`
- Regenerate: `provider/cmd/pulumi-resource-dokploy/schema.json`
- Regenerate: `sdk/go/dokploy/`, `sdk/nodejs/`, `sdk/python/`, `sdk/dotnet/`, `sdk/java/`
- Regenerate: language directories under `examples/`
- Regenerate: `website/src/content/docs/reference/` and `website/src/content/docs/examples/complete.mdx`

**Interfaces:**
- Publishes all five resources and new Application input fields in every SDK and documentation surface.

- [ ] **Step 1: Write failing metadata, example, and website tests**

Update exact resource expectations from 13 to 18 and require `SSHKey`, `Registry`, `Tag`, `ProjectTag`, and `Mount`. Assert secret and replacement metadata, sidebar entries, reference routes, import syntax, and documented MongoDB/LibSQL exclusions.

- [ ] **Step 2: Verify tests fail**

Run:

```bash
mise exec -- go test ./provider -run 'TestRegistryMetadata|TestSchema' -count=1
mise exec -- go test ./examples -count=1
npm test --prefix website
```

Expected: FAIL because generated artifacts and documentation are stale.

- [ ] **Step 3: Update canonical examples and curated docs**

Add examples that consume configuration secrets rather than literals:

```yaml
configuration:
  dokploy:registryPassword:
    type: string
    secret: true
  dokploy:sshPrivateKey:
    type: string
    secret: true
```

Show a supplied `SSHKey`, generic Git Application `sshKeyId`, Registry plus Application registry references, Tag plus ProjectTag, and bind/volume/file Mount examples. Document imports, automatic mount redeployment, registry testing, secret behavior, and target exclusions.

- [ ] **Step 4: Regenerate all artifacts**

Run:

```bash
mise exec -- make codegen
mise exec -- make gen_examples
mise exec -- make docs_generate
```

- [ ] **Step 5: Run full verification**

Run:

```bash
mise exec -- go test ./openapi/cmd/normalize -count=1
mise exec -- make check_openapi
mise exec -- go test -short -v -count=1 ./provider/... ./internal/...
mise exec -- make test_race
mise exec -- make check_codegen
mise exec -- make build_sdks
mise exec -- make test_examples
mise exec -- make docs_check
mise exec -- make docs_build
python3 -m unittest scripts/test_normalize_ci.py -v
mise exec -- make lint
git diff --check
```

Expected: every command PASS. Do not regenerate unrelated files beyond generator output required by the changed schema and canonical examples.

- [ ] **Step 6: Commit**

```bash
git add README.md provider/cmd/pulumi-resource-dokploy/schema.json provider/registry_metadata_test.go sdk examples website
git commit -m "docs: publish SSH registry tag and mount resources"
```

---

## Final SDD Review

- [ ] Run a Luna reviewer across the complete diff from `5526f99` through the final task commit, checking the design document requirement by requirement.
- [ ] Dispatch a fresh Luna implementer for every reviewer finding; do not let the reviewer edit code.
- [ ] Rerun the focused tests for each fix and the full Task 8 verification suite.
- [ ] Run the environment-gated live command if credentials are available and report it separately from non-live verification.
- [ ] Confirm `git status --short` contains only the two pre-existing untracked plan files, unless the user has changed the worktree concurrently.
