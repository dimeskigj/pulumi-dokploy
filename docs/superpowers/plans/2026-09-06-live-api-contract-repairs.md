# Live API Contract Repairs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the provider/client contract mismatches demonstrated by the 2026-09-05 live acceptance run for update responses, Environment updates, ProjectTag reads, and active-organization discovery.

**Architecture:** Treat `openapi/corrections.json` as the source of truth and regenerate normalized OpenAPI and client code rather than editing generated files. Keep provider changes limited to request construction and response-field selection, with deterministic HTTP fixtures matching observed or source-backed Dokploy shapes before focused live verification.

**Tech Stack:** Go 1.26.6, `testing`, `testify/require`, oapi-codegen v2.8.0, Pulumi Go Provider infer resources, generated Dokploy HTTP client.

## Global Constraints

- Never edit `openapi/upstream.json` or `internal/client/generated/generated.gen.go` by hand.
- Never log request bodies, IDs, API keys, SSH key material, database passwords, or environment values during live verification.
- Use deterministic tests before implementation and run only the smallest focused live subtest after all non-live checks pass.
- Keep the pinned upstream contract intact; all deployed-Dokploy corrections belong in `openapi/corrections.json`.
- Do not add generic recursive JSON field discovery or compatibility behavior without an observed response shape.

---

### Task 1: Boolean Update Responses

**Files:**
- Modify: `openapi/corrections.json:14,45,52,59,66,77`
- Modify: `openapi/cmd/normalize/main_test.go:290-335`
- Regenerate: `openapi/dokploy.json`
- Regenerate: `internal/client/generated/generated.gen.go`
- Modify: `internal/client/client_test.go`
- Modify: `provider/application_test.go`
- Modify: `provider/postgres_test.go`
- Modify: `provider/mysql_test.go`
- Modify: `provider/mariadb_test.go`
- Modify: `provider/mongodb_test.go`
- Modify: `provider/redis_test.go`

**Interfaces:**
- Consumes: operation-response corrections applied by `openapi/cmd/normalize`.
- Produces: generated update response types with `JSON200 *bool` for Application and all five database resources.

- [ ] **Step 1: Add a failing normalized-contract test**

Add a focused helper alongside `responseSchema` so primitive response types can
be asserted without changing existing reference assertions:

```go
func responseSchemaType(t *testing.T, d *Document, path, method, status string) string {
	t.Helper()
	pathItem, ok := d.Paths[path]
	require.True(t, ok, "missing path %s", path)
	op := pathItem.Post
	if method == httpMethodGet {
		op = pathItem.Get
	}
	require.NotNil(t, op, "missing %s operation for path %s", method, path)
	responses, ok := op.Raw["responses"].(map[string]any)
	require.True(t, ok, "responses for %s are not an object", path)
	response, ok := responses[status]
	require.True(t, ok, "missing %s response for %s", status, path)
	b, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Content map[string]struct {
			Schema struct {
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(b, &decoded))
	return decoded.Content["application/json"].Schema.Type
}
```

Then add a table to `TestNormalizeUsesProductionOperationsAndCorrections` that checks the concrete schema type instead of an object reference:

```go
for _, operation := range []string{
	"application.update", "postgres.update", "mysql.update",
	"mariadb.update", "mongo.update", "redis.update",
} {
	require.Equal(t, "boolean", responseSchemaType(t, output, "/"+operation, httpMethodPost, "200"), "response correction for %s", operation)
	require.Empty(t, responseSchema(t, output, "/"+operation, httpMethodPost, "200").Ref, "response correction for %s", operation)
}
```

- [ ] **Step 2: Run the normalizer test and verify RED**

Run: `go test ./openapi/cmd/normalize -run TestNormalizeUsesProductionOperationsAndCorrections -count=1`

Expected: FAIL because the six schemas reference resource objects instead of declaring `type: boolean`.

- [ ] **Step 3: Correct all six response mappings**

Change the values for these keys in `openapi/corrections.json` to `"boolean"`:

```json
"application.update": "boolean",
"mariadb.update": "boolean",
"mongo.update": "boolean",
"mysql.update": "boolean",
"postgres.update": "boolean",
"redis.update": "boolean"
```

- [ ] **Step 4: Regenerate the normalized contract and client**

Run: `make generate_openapi`

Expected: `openapi/dokploy.json` contains boolean HTTP 200 schemas and each generated `Parse*UpdateResponse` decodes into `bool`.

- [ ] **Step 5: Add generated-client boundary coverage**

In `internal/client/client_test.go`, table-test the six update endpoints with status 200, content type `application/json`, and body `true`. Invoke the corresponding `*UpdateWithResponse` method and assert no error and a non-nil true `JSON200`. Keep each request body minimal but valid for its generated request type.

- [ ] **Step 6: Make provider fixtures reproduce the live shape**

Replace successful response fixtures of `{}` with `true` for the six affected `*.update` operations. Preserve assertions proving metadata-only updates do not deploy and runtime updates continue through save/deploy/poll.

- [ ] **Step 7: Verify GREEN**

Run: `go test ./openapi/cmd/normalize ./internal/client ./provider -run 'TestNormalizeUsesProductionOperationsAndCorrections|Test.*Update' -count=1`

Expected: PASS, including the exact boolean response shape that failed live.

- [ ] **Step 8: Commit**

```bash
git add openapi/corrections.json openapi/dokploy.json openapi/cmd/normalize/main_test.go internal/client/generated/generated.gen.go internal/client/client_test.go provider/*_test.go
git commit -m "fix: decode Dokploy boolean update responses"
```

### Task 2: Environment Update Contract

**Files:**
- Modify: `provider/environment.go:3-14,125-146`
- Modify: `provider/environment_test.go:90-112`
- Modify: `openapi/cmd/normalize/main_test.go`

**Interfaces:**
- Consumes: `generated.EnvironmentUpdateJSONRequestBody` with optional non-nullable `Description *string`.
- Produces: Environment updates that omit an unchanged absent description and reject unsupported description removal without sending invalid JSON null.

- [ ] **Step 1: Replace the invalid-null test with three failing request tests**

Add cases proving:

```go
// nil -> nil: {"environmentId":"e1","name":"renamed"}
// nil -> value: {"environmentId":"e1","name":"renamed","description":"changed"}
// value -> nil: no HTTP request; error contains "Dokploy does not support clearing an environment description"
```

The third case must initialize `req.State.Description` with a non-empty string and `req.Inputs.Description` as nil.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./provider -run 'TestEnvironmentUpdate' -count=1`

Expected: FAIL because current code serializes nil as `description:null` and attempts the HTTP request.

- [ ] **Step 3: Use the generated request and guard unsupported removal**

Remove the `bytes`, `encoding/json`, and `nullable` imports from `provider/environment.go`. Implement the non-dry-run path as:

```go
if req.State.Description != nil && req.Inputs.Description == nil {
	return infer.UpdateResponse[EnvironmentState]{}, errors.New("Dokploy does not support clearing an environment description")
}
body := generated.EnvironmentUpdateJSONRequestBody{
	EnvironmentId: req.ID,
	Name:          &req.Inputs.Name,
	Description:   req.Inputs.Description,
}
if _, err := r.client(ctx).EnvironmentUpdateWithResponse(ctx, body); err != nil {
	return infer.UpdateResponse[EnvironmentState]{}, err
}
```

Do not add `projectId`: both checked-in schemas mark it optional, and the live comparison did not isolate it as causal.

- [ ] **Step 4: Lock the normalized request contract**

Add a normalizer assertion that `/environment.update` has optional `description` with `type: string` and no nullable union. This protects the generated request shape used by the provider.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./provider -run 'TestEnvironment' -count=1`

Run: `go test ./openapi/cmd/normalize -count=1`

Expected: PASS; no test expects an explicit null Environment description.

- [ ] **Step 6: Commit**

```bash
git add provider/environment.go provider/environment_test.go openapi/cmd/normalize/main_test.go
git commit -m "fix: honor environment update request contract"
```

### Task 3: ProjectTag Response Shape

**Files:**
- Modify: `openapi/corrections.json:108`
- Modify: `openapi/cmd/normalize/main_test.go:325-335`
- Regenerate: `openapi/dokploy.json`
- Regenerate: `internal/client/generated/generated.gen.go`
- Modify: `provider/project_tag.go:85-110`
- Modify: `provider/project_tag_test.go:41-71`

**Interfaces:**
- Consumes: Dokploy `project.one` responses with `projectTags` association rows.
- Produces: `ProjectTag.Read` association lookup by direct `projectTags[].tagId`.

- [ ] **Step 1: Add failing real-shape provider fixtures**

Change the create/read fixture to:

```json
{"projectId":"p1","projectTags":[{"tagId":"other","tag":{"tagId":"other"}},{"tagId":"t1","tag":{"tagId":"t1"}}]}
```

Change the missing-association fixture to `projectTags:[{"tagId":"other"}]`. Assert create/read finds only `t1`, missing association returns an empty ID, and delete still calls only `tag.removeFromProject`.

- [ ] **Step 2: Run the ProjectTag tests and verify RED**

Run: `go test ./provider -run TestProjectTag -count=1`

Expected: FAIL because generated `Project` exposes `Tags`, not `ProjectTags`.

- [ ] **Step 3: Correct the Project schema**

Replace the unsupported `tags` property in the `Project` correction with:

```json
"projectTags": {
  "type": "array",
  "items": {
    "type": "object",
    "required": ["tagId"],
    "properties": {
      "tagId": {"type": "string"}
    },
    "additionalProperties": true
  }
}
```

Update the normalizer test to require `Project.projectTags[].tagId`, then run `make generate_openapi`.

- [ ] **Step 4: Read the corrected generated field**

Update `ProjectTag.Read` to return absent when `ProjectTags == nil` and iterate `*resp.JSON200.ProjectTags`, comparing each direct `TagId` with the imported tag ID. Do not inspect arbitrary `AdditionalProperties`.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./openapi/cmd/normalize ./provider -run 'TestNormalizeUsesProductionOperationsAndCorrections|TestProjectTag' -count=1`

Expected: PASS with source-backed `projectTags` fixtures.

- [ ] **Step 6: Commit**

```bash
git add openapi/corrections.json openapi/dokploy.json openapi/cmd/normalize/main_test.go internal/client/generated/generated.gen.go provider/project_tag.go provider/project_tag_test.go
git commit -m "fix: read project tag associations from projectTags"
```

### Task 4: Active Organization Identifier

**Files:**
- Modify: `openapi/corrections.json:121`
- Modify: `openapi/cmd/normalize/main_test.go`
- Regenerate: `openapi/dokploy.json`
- Regenerate: `internal/client/generated/generated.gen.go`
- Modify: `provider/ssh_key.go:84-99`
- Modify: `provider/ssh_key_test.go:41-80`
- Modify: `provider/live_harness_test.go:146-164`
- Modify: `provider/task9_regressions_test.go:201-214`

**Interfaces:**
- Consumes: `organization.active` current response field `id` and an optional legacy `organizationId` field.
- Produces: `activeOrganizationID(generated.Organization) string` with explicit precedence `id`, then `organizationId`.

- [ ] **Step 1: Add failing current-shape tests**

Use `{"id":"org1"}` for the primary SSH create fixture and assert the subsequent `sshKey.create` request still sends `"organizationId":"org1"`. Retain one compatibility case for `{"organizationId":"legacy-org"}` and incomplete-object rejection.

Extend structural-classification tests with exact labels for flat `id`, flat `organizationId`, nested IDs, null, missing, invalid JSON, empty string, and wrong type. Labels must never include the identifier value.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./provider -run 'TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1`

Expected: FAIL because the generated model recognizes only `organizationId`.

- [ ] **Step 3: Correct and regenerate the Organization schema**

Define both optional fields in `openapi/corrections.json`:

```json
"Organization": {
  "type": "object",
  "properties": {
    "id": {"type": "string"},
    "organizationId": {"type": "string"}
  },
  "additionalProperties": true
}
```

Do not mark either field required because supported response variants use different names. Update normalizer assertions and run `make generate_openapi`.

- [ ] **Step 4: Add explicit identifier selection**

Add a helper that returns non-empty `Id`, then non-empty `OrganizationId`, then `""`. Use its result for both `SSHKeyState.OrganizationID` and `SshKeyCreateJSONRequestBody.OrganizationId`; preserve the existing incomplete-organization error when neither exists.

- [ ] **Step 5: Update safe live shape classification**

Make `classifyOrganizationActiveShape` recognize flat `id` and flat `organizationId` separately. Change the SSH live prerequisite to permit only those two flat non-empty shapes; nested IDs remain diagnostic skips rather than recursive decoding.

- [ ] **Step 6: Verify GREEN**

Run: `go test ./openapi/cmd/normalize ./provider -run 'TestNormalizeUsesProductionOperationsAndCorrections|TestSSHKey|TestTask9OrganizationActiveShapeClassification' -count=1`

Expected: PASS without exposing key material or identifier values.

- [ ] **Step 7: Commit**

```bash
git add openapi/corrections.json openapi/dokploy.json openapi/cmd/normalize/main_test.go internal/client/generated/generated.gen.go provider/ssh_key.go provider/ssh_key_test.go provider/live_harness_test.go provider/task9_regressions_test.go
git commit -m "fix: accept active organization primary IDs"
```

### Task 5: Static And Focused Live Verification

**Files:**
- Modify: `docs/bugs/2026-09-05-live-acceptance-run.md`
- Create conditionally: focused sanitized defect reports under `docs/bugs/`

**Interfaces:**
- Consumes: Tasks 1-4 and configured protected acceptance credentials.
- Produces: deterministic verification plus sanitized resolution evidence.

- [ ] **Step 1: Run complete static verification**

Run: `make check_openapi`

Run: `go test -short -count=1 ./provider/... ./internal/... ./tests/...`

Run: `go test -race ./provider/... ./internal/...`

Expected: PASS; live tests skip when opt-in is absent.

- [ ] **Step 2: Run focused live checks serially**

After sourcing credentials without printing values and confirming the stop marker is absent, run:

```bash
go test ./provider -run '^TestLiveTier1ControlPlane/(Environment|ProjectTag|SSHKey)$' -parallel=1 -count=1 -v
go test ./provider -run '^TestLiveTier2Workloads/Application$' -parallel=1 -count=1 -v
go test ./provider -run '^TestLiveTier3Databases/PostgreSQL$' -parallel=1 -count=1 -v
```

Expected: the focused resources complete lifecycle and cleanup. Stop immediately if cleanup or server health sets the stop marker.

- [ ] **Step 3: Record sanitized outcomes**

Append the provider revision, focused command, pass/fail classification, duration, and cleanup status to the run summary. Never include endpoint, IDs, request/response payloads, credentials, SSH material, or database values.

- [ ] **Step 4: Commit verification evidence**

```bash
git add docs/bugs
git commit -m "docs: record live API contract verification"
```
