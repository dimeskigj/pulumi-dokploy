# Complete Verification Repairs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Diagnose and repair the Domain, PostgreSQL mount dispatch, Pulumi smoke, strict YAML, and gRPC vulnerability failures found by full verification.

**Architecture:** Treat each failure as an independent evidence track with a focused red-green cycle and reviewer gate. Retain only sanitized structural diagnostics, change generated contracts only when live evidence proves they are stale, and verify live fixes individually with fresh stop markers before rerunning broad tiers.

**Tech Stack:** Go 1.26.6, Pulumi CLI 3.259.0, Pulumi Automation API, pulumi-go-provider `infer`, oapi-codegen client, `testing`, `testify/require`, mise, golangci-lint, govulncheck.

## Global Constraints

- Preserve the existing uncommitted deterministic cancellation changes in `provider/backup_test.go` and `provider/testserver_test.go`.
- Never print credentials, endpoint values, private URLs, resource IDs, hostnames, payload values, response bodies, or unsanitized errors.
- Diagnostics may contain only operation, lifecycle phase, status class/code, sorted request-key names, sanitized tool stage, cleanup state, health state, and marker state.
- Do not add alternate Domain payload retries, blind retries, or broad compatibility fallbacks.
- Preserve existing incident markers; every live verification uses a fresh absent `DOKPLOY_ACCEPTANCE_STOP_FILE` and `-parallel=1`.
- Stop later heavy live tests when a fresh marker appears until cleanup and health are independently verified.
- Raise `google.golang.org/grpc` to at least `v1.83.2` in the root, SDK Go, and example Go modules without unrelated upgrades unless required by module resolution.
- Optional live cases may skip only for their documented prerequisites; all enabled cases must pass and clean up owned resources.

---

### Task 1: Diagnose and Repair Domain Creation

**Files:**
- Modify: `provider/live_harness_test.go:286-466`
- Modify: `provider/live_harness_unit_test.go:140-245`
- Modify: `provider/live_workloads_test.go:220-275, 750-820`
- Modify if evidence requires: `provider/domain.go:161-270`
- Modify if contract evidence requires: `openapi/upstream.json`, `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`
- Test: `provider/domain_test.go`
- Test: `provider/live_harness_unit_test.go`

**Interfaces:**
- Consumes: `domainCreateBody(DomainArgs) generated.DomainCreateJSONRequestBody`, `domainCreateRequestKeysFromBody`, `liveDomainCreateResult`, `classifyDomainComparison`.
- Produces: a proven Domain create request contract shared by provider and generated-client paths, plus sanitized comparison evidence that never includes field values.

- [ ] **Step 1: Add a failing structural diagnostic test**

Add a unit test that requires Domain comparison output to include target, provider status/code, generated status/code, and equal sorted key sets while rejecting sentinel values:

```go
func TestDomainComparisonEvidenceIsStructuralAndSanitized(t *testing.T) {
    keys := []string{"applicationId", "certificateType", "domainType", "host", "https", "stripPath"}
    got := formatDomainComparisonEvidence(
        liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys},
        liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys},
    )
    require.Contains(t, got, "target=application")
    require.Contains(t, got, "provider=4xx/BAD_REQUEST")
    require.Contains(t, got, "generated=4xx/BAD_REQUEST")
    require.Contains(t, got, "keys=applicationId,certificateType,domainType,host,https,stripPath")
    require.NotContains(t, got, "secret-sentinel")
    require.NotContains(t, got, "https://")
}
```

- [ ] **Step 2: Run the diagnostic test and confirm RED**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainComparisonEvidenceIsStructuralAndSanitized$' -count=1 -v
```

Expected: FAIL because `formatDomainComparisonEvidence` does not exist.

- [ ] **Step 3: Implement the minimal sanitized formatter**

Add a formatter that parses only the fixed structural classification and sorted key names. It must return a fixed invalid-evidence classification if either result contains an unknown path, target, classification shape, or unsafe key.

```go
func formatDomainComparisonEvidence(provider, generated liveDomainCreateResult) string {
    providerStatus, providerCode := domainResultStatus(provider.classification)
    generatedStatus, generatedCode := domainResultStatus(generated.classification)
    keys := append([]string(nil), provider.keys...)
    sort.Strings(keys)
    return fmt.Sprintf(
        "target=%s;provider=%s/%s;generated=%s/%s;keys=%s",
        provider.target, providerStatus, providerCode,
        generatedStatus, generatedCode, strings.Join(keys, ","),
    )
}
```

Use allowlists already enforced by `classifyDomainComparison`; do not interpolate raw errors or values.

- [ ] **Step 4: Run focused unit tests and confirm GREEN**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'DomainComparison|DomainCreateRequestKeys|DomainCreateBody' -count=1 -v
```

Expected: PASS with no sensitive output.

- [ ] **Step 5: Run focused live comparison with a fresh marker**

Run application and Compose separately:

```bash
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-application-diagnostic.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/application$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-compose-diagnostic.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/compose$' -parallel=1 -count=1 -v
```

Expected before the fix: FAIL with sanitized structural evidence sufficient to identify the incompatible field shape; neither command leaves an owned resource or new stop marker unless cleanup/health independently fails.

- [ ] **Step 6: Write the failing request-contract regression**

Based on the live evidence, update the exact request matrix in `provider/domain_test.go`. For example, if the deployed contract rejects `domainType`, encode that proven contract explicitly:

```go
func TestDomainCreateBodyUsesDeployedTargetContract(t *testing.T) {
    applicationID := "a1"
    body := domainCreateBody(DomainArgs{
        ApplicationID: &applicationID,
        Host: "app.example.invalid",
        HTTPS: true,
        CertificateType: CertificateLetsencrypt,
    })
    encoded, err := json.Marshal(body)
    require.NoError(t, err)
    require.JSONEq(t, `{
        "applicationId":"a1",
        "certificateType":"letsencrypt",
        "host":"app.example.invalid",
        "https":true,
        "stripPath":false
    }`, string(encoded))
}
```

Use the actual proven contract from Step 5, not this example if evidence identifies a different field.

- [ ] **Step 7: Run the contract test and confirm RED**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainCreateBodyUsesDeployedTargetContract$' -count=1 -v
```

Expected: FAIL with the exact extra, missing, nullable, or enum field identified by live evidence.

- [ ] **Step 8: Implement the smallest causal contract fix**

Change only the earliest incorrect layer. If generated types are wrong, update `openapi/upstream.json`, rerun normalization/generation, and assert the generated request. If only provider mapping is wrong, change `domainCreateBody` without editing OpenAPI. Do not add a second create attempt.

- [ ] **Step 9: Verify focused local and live behavior**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider ./tests -run 'Domain|LiveDiagnosticSourceContract' -count=1
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-application-fixed.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/application$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-compose-fixed.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/compose$' -parallel=1 -count=1 -v
```

Expected: all PASS; create/read/update/import/delete succeed and fresh markers remain absent.

- [ ] **Step 10: Commit the Domain repair**

```bash
git add provider/domain.go provider/domain_test.go provider/live_harness_test.go provider/live_harness_unit_test.go provider/live_workloads_test.go tests/acceptance_test.go openapi/upstream.json openapi/dokploy.json internal/client/generated/generated.gen.go
git commit -m "fix: align domain create contract"
```

Stage only files actually changed.

---

### Task 2: Diagnose and Repair PostgreSQL Mount Dispatch

**Files:**
- Modify: `provider/live_workloads_test.go:979-1218`
- Modify: `provider/live_harness_test.go:100-205, 606-715`
- Modify: `provider/live_harness_unit_test.go`
- Modify if evidence requires: `provider/mount.go`
- Test if production changes: `provider/mount_test.go`

**Interfaces:**
- Consumes: `liveDispatchFixture`, `createDispatchDatabase`, `runLiveMountLifecycle`, `processLiveHeavyOperationError`, `finishDatabaseCleanup`.
- Produces: deterministic phase evidence and cleanup ordering in which mount absence is verified before PostgreSQL deletion and ordinary target errors do not create markers.

- [ ] **Step 1: Write a failing phase-classification test**

Add a pure classifier that accepts only known phases and structural errors:

```go
func TestClassifyMountDispatchPhase(t *testing.T) {
    require.Equal(t, "operation=mount-dispatch;phase=target-read;status=transport;code=unknown",
        classifyMountDispatchPhase("target-read", errors.New("transport")))
    require.Equal(t, "operation=mount-dispatch;phase=cleanup;status=failed;code=unknown",
        classifyMountDispatchPhase("cleanup", errLiveCleanup))
}
```

Use typed API/context errors in the final test table and assert secret sentinels are absent.

- [ ] **Step 2: Run the classifier test and confirm RED**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestClassifyMountDispatchPhase$' -count=1 -v
```

Expected: FAIL because the classifier is missing.

- [ ] **Step 3: Implement phase classification and wire it to the focused test only**

Allow only `fixture-create`, `target-read`, `mount-create`, `mount-update`, `mount-delete`, `fixture-delete`, and `health-probe`. Return fixed status/code values from typed errors; never format raw errors.

- [ ] **Step 4: Run focused local tests**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'MountDispatch|ValidateLiveTarget|HeavyOperation|Cleanup' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Reproduce the focused live failure with a fresh marker**

Run:

```bash
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/mount-postgres-diagnostic.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
```

Expected before the fix: FAIL with a single sanitized phase classification. Verify whether the marker came from cleanup or an independently failed health probe.

- [ ] **Step 6: Write the smallest failing regression for the proven cause**

If the failure is cleanup ordering, use a deterministic call-order test:

```go
func TestPostgresMountDispatchCleansMountBeforeFixture(t *testing.T) {
    var calls []string
    owner := newLiveCleanupOwner(func() { calls = append(calls, "fixture") })
    cleanupMountThenFixture(func() { calls = append(calls, "mount") }, owner.cleanupOnce)
    require.Equal(t, []string{"mount", "fixture"}, calls)
}
```

If evidence points to readiness or request routing, write the corresponding typed unit test against `newPostgresDispatchFixture` or `mountCreateBody`. Do not encode a guessed cause.

- [ ] **Step 7: Run the regression and confirm RED**

Run the exact new test with `-count=1 -v`. Expected: FAIL for the live-proven cause.

- [ ] **Step 8: Implement the minimal repair**

Change only fixture ownership, readiness routing, mount request mapping, or timeout classification proven by Step 5. Keep the database heavy-operation lease until dependent mount absence is verified.

- [ ] **Step 9: Verify repeatedly under local race and live execution**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -race ./provider -run 'MountDispatch|PostgresTargetReadiness|Cleanup' -count=20
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/mount-postgres-fixed-1.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/mount-postgres-fixed-2.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
```

Expected: PASS twice live, no fresh marker, and verified absence of both resources.

- [ ] **Step 10: Commit the mount dispatch repair**

```bash
git add provider/live_workloads_test.go provider/live_harness_test.go provider/live_harness_unit_test.go provider/mount.go provider/mount_test.go
git commit -m "fix: stabilize postgres mount dispatch"
```

Stage only files actually changed.

---

### Task 3: Diagnose and Repair Pulumi Automation Smoke

**Files:**
- Modify: `tests/acceptance_program_test.go:34-125`
- Modify: `tests/acceptance_test.go:245-327`
- Modify if plugin discovery requires: `Makefile:84-128`
- Test: `tests/acceptance_test.go`

**Interfaces:**
- Consumes: `runLifecycleSmoke`, `acceptanceFailure`, `auto.NewStackInlineSource`, local `bin/pulumi-resource-dokploy`.
- Produces: `acceptanceStageError(stage string, err error) error` whose text contains only an allowlisted stage and sanitized category.

- [ ] **Step 1: Write failing sanitized stage tests**

```go
func TestAcceptanceStageErrorIsSanitized(t *testing.T) {
    err := acceptanceStageError("stack-create", errors.New("secret-sentinel https://private.example resource-id-sentinel"))
    require.EqualError(t, err, "Pulumi acceptance failed at stage stack-create: category=process")
}

func TestAcceptanceStageErrorRejectsUnknownStage(t *testing.T) {
    err := acceptanceStageError("secret-sentinel", errors.New("boom"))
    require.EqualError(t, err, "Pulumi acceptance failed at stage unknown: category=process")
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./tests -run '^TestAcceptanceStageError' -count=1 -v
```

Expected: FAIL because `acceptanceStageError` is missing.

- [ ] **Step 3: Implement the stage classifier**

Allow only `workspace`, `plugin-discovery`, `stack-create`, `configure-endpoint`, `configure-api-key`, `preview-1`, `up-1`, `refresh-1`, `preview-2`, `up-2`, `refresh-2`, `destroy`, `export`, `remove-stack`, and `list-stacks`. Categories are `timeout`, `canceled`, `process`, and `unknown`, derived without returning raw error text.

- [ ] **Step 4: Wire every smoke failure to the stage classifier**

Replace calls such as:

```go
t.Fatal(acceptanceFailure("stack creation"))
```

with:

```go
t.Fatal(acceptanceStageError("stack-create", err))
```

Retain the existing field-only output assertions and source-contract checks.

- [ ] **Step 5: Run local smoke-contract tests**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./tests -run 'AcceptanceStage|AcceptanceFailure|LifecycleSmoke|LiveDiagnosticSourceContract' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Reproduce stack creation with sanitized stage evidence**

Build the provider and run:

```bash
make provider
PATH="$PWD/bin:$PATH" DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/pulumi-smoke-diagnostic.stop mise exec -- go test ./tests -run '^TestAccLifecycleSmoke$' -parallel=1 -count=1 -v
```

Expected before the fix: FAIL with stage/category only. Separately inspect non-secret process facts: executable lookup result, Pulumi version, whether the local binary exists and is executable, and whether the temporary file backend directory is writable.

- [ ] **Step 7: Write the failing regression for the proven setup defect**

For example, if plugin discovery is the cause, extract and test a deterministic setup function:

```go
func TestPrepareAcceptanceEnvironmentIncludesProviderDirectory(t *testing.T) {
    env, err := prepareAcceptanceEnvironment("/repo/bin", "/usr/bin")
    require.NoError(t, err)
    require.Equal(t, "/repo/bin"+string(os.PathListSeparator)+"/usr/bin", env["PATH"])
}
```

If the failure is backend or stack naming, write the exact corresponding unit test instead. Do not implement this example unless live evidence confirms plugin discovery.

- [ ] **Step 8: Run the regression and confirm RED**

Run the exact new test. Expected: FAIL for the proven setup defect.

- [ ] **Step 9: Implement and verify the minimal setup repair**

Make only the proven environment, backend, project-name, or Automation API option change. Then run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./tests -count=1
PATH="$PWD/bin:$PATH" DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/pulumi-smoke-fixed.stop mise exec -- go test ./tests -run '^TestAccLifecycleSmoke$' -parallel=1 -count=1 -v
```

Expected: PASS through stack removal, with no fresh stop marker.

- [ ] **Step 10: Commit the smoke repair**

```bash
git add tests/acceptance_program_test.go tests/acceptance_test.go Makefile
git commit -m "fix: restore pulumi lifecycle smoke"
```

Stage only files actually changed.

---

### Task 4: Replace the Invalid Strict YAML Assumption

**Files:**
- Modify: `examples/yaml_test.go:114-133`
- Modify: `examples/base_test.go:21-41`

**Interfaces:**
- Consumes: `providerSchema`, `schemaResources`, canonical `examples/yaml/Pulumi.yaml`.
- Produces: `validateYAMLResourceProperties(document map[string]any, resources map[string]any) error`, validating provider inputs directly against generated schema.

- [ ] **Step 1: Write the failing schema-validation tests**

```go
func TestValidateYAMLResourcePropertiesRejectsUnknownProperty(t *testing.T) {
    document := map[string]any{"resources": map[string]any{
        "project": map[string]any{
            "type": "dokploy:index:Project",
            "properties": map[string]any{"name": "ok", "invalidProperty": true},
        },
    }}
    err := validateYAMLResourceProperties(document, schemaResources(t))
    if err == nil || !strings.Contains(err.Error(), "invalidProperty") {
        t.Fatalf("validation error = %v, want invalidProperty", err)
    }
}

func TestValidateYAMLResourcePropertiesAcceptsCanonicalYAML(t *testing.T) {
    document := loadCanonicalYAML(t)
    if err := validateYAMLResourceProperties(document, schemaResources(t)); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go test ./examples -tags=all -run '^TestValidateYAMLResourceProperties' -count=1 -v
```

Expected: FAIL because the validator and loader do not exist.

- [ ] **Step 3: Implement direct schema property validation**

Decode each resource token, read that resource schema's `inputProperties`, and reject every YAML property absent from the schema. Sort resource and property names before validation so the first error is deterministic. Return errors containing only resource logical name, token, and property name.

- [ ] **Step 4: Remove the invalid Pulumi strict-conversion negative test**

Delete `TestCanonicalYAMLRejectsUnknownProperty`. Keep `TestCanonicalYAMLActuallyBindsWithPulumi` unchanged so Pulumi conversion remains tested separately.

- [ ] **Step 5: Run all tagged examples and confirm GREEN**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go test ./examples -tags=all -count=1
```

Expected: PASS, including canonical conversion and direct unknown-property rejection.

- [ ] **Step 6: Commit the YAML contract repair**

```bash
git add examples/base_test.go examples/yaml_test.go
git commit -m "test: validate yaml properties against schema"
```

---

### Task 5: Upgrade the Vulnerable gRPC Dependency

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `sdk/go/dokploy/go.mod`, `sdk/go/dokploy/go.sum`
- Modify: `examples/go/go.mod`, `examples/go/go.sum`

**Interfaces:**
- Consumes: existing Pulumi dependency graph.
- Produces: all checked modules resolving `google.golang.org/grpc` at `v1.83.2` or newer without unrelated direct-dependency changes.

- [ ] **Step 1: Capture the failing vulnerability evidence**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Expected: FAIL with reachable `GO-2026-6443` in `google.golang.org/grpc@v1.83.1`.

- [ ] **Step 2: Upgrade each module minimally**

Run from each module root:

```bash
go get google.golang.org/grpc@v1.83.2
```

Module roots are repository root, `sdk/go/dokploy`, and `examples/go`.

- [ ] **Step 3: Verify module resolution**

Run `go list -m google.golang.org/grpc` in all three module roots.

Expected: `google.golang.org/grpc v1.83.2` or newer in every module.

- [ ] **Step 4: Run compatibility and vulnerability checks**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -count=1 ./...
go test -count=1 ./...
```

The second command runs in `sdk/go/dokploy`, then in `examples/go`.

Run root vulnerability scanning again:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Expected: no reachable vulnerabilities.

- [ ] **Step 5: Commit the dependency repair**

```bash
git add go.mod go.sum sdk/go/dokploy/go.mod sdk/go/dokploy/go.sum examples/go/go.mod examples/go/go.sum
git commit -m "fix: upgrade grpc security patch"
```

---

### Task 6: Integrated Local and Live Verification

**Files:**
- Modify only if results require factual updates: `docs/bugs/2026-09-15-live-acceptance-fix-verification.md`

**Interfaces:**
- Consumes: all repairs from Tasks 1-5 and the existing cancellation-test changes.
- Produces: complete fresh evidence that all enabled local and live checks pass and cleanup is verified.

- [ ] **Step 1: Run formatting and static checks**

Run:

```bash
gofmt -w provider/domain.go provider/domain_test.go provider/live_harness_test.go provider/live_harness_unit_test.go provider/live_workloads_test.go provider/mount.go provider/mount_test.go tests/acceptance_program_test.go tests/acceptance_test.go examples/base_test.go examples/yaml_test.go provider/backup_test.go provider/testserver_test.go
```

Expected: lint reports `0 issues`; diff check exits zero.

- [ ] **Step 2: Run all root Go and race tests without live credentials**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -count=1 ./...
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -race -count=1 ./provider/... ./internal/... ./tests/...
```

Expected: PASS.

- [ ] **Step 3: Run tagged examples and generated SDK checks**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go test ./examples -tags=all -count=1
```

Then run:

```bash
go test -count=1 ./...
```

in `sdk/go/dokploy` and `examples/go`; compile Python examples/SDK; build Node.js, .NET with `make build_dotnet VERSION_GENERIC=0.0.1-alpha.0+dev`, and Java with the pinned Gradle command.

Expected: PASS. Existing compiler deprecation or nullable warnings may remain only if they do not fail the documented build.

- [ ] **Step 4: Run website checks**

Run:

```bash
npm ci --prefix website
npm --prefix website run check
npm --prefix website run build
npm --prefix website run test:built
```

Expected: PASS.

- [ ] **Step 5: Run generated artifact checks when applicable**

If OpenAPI changed:

```bash
make check_openapi
```

If provider schema or SDK source changed:

```bash
make check_codegen
```

Expected: generated output is reproducible and `git diff` contains only intended changes.

- [ ] **Step 6: Run vulnerability scan**

Run:

```bash
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Expected: no reachable vulnerabilities.

- [ ] **Step 7: Run focused live repairs serially**

Use a unique fresh marker for every command, verify it remains absent afterward, and preserve all prior markers:

```bash
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/application$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/compose$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
PATH="$PWD/bin:$PATH" mise exec -- go test ./tests -run '^TestAccLifecycleSmoke$' -parallel=1 -count=1 -v
```

Expected: PASS, verified cleanup, no fresh marker.

- [ ] **Step 8: Run all live tiers serially**

Build the provider first, then run each command with a unique fresh marker and inspect it before continuing:

```bash
make provider
mise exec -- go test ./provider -run '^TestLiveTier1ControlPlane$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier2Workloads$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier3Databases$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier4Backups$' -parallel=1 -count=1 -v
PATH="$PWD/bin:$PATH" mise exec -- go test ./tests -run '^TestAccLifecycleSmoke$' -parallel=1 -count=1 -v
```

Expected: every enabled test passes; only documented optional prerequisites skip; owned resources are absent; no fresh marker remains.

- [ ] **Step 9: Record sanitized factual verification**

Update the verification report only with revision, exact safe commands, duration, pass/fail/skip totals, structural classifications, and marker/cleanup state. Do not record endpoints, IDs, hosts, payloads, bodies, or credentials.

- [ ] **Step 10: Final diff and commit**

Run:

```bash
git status --short
```

Inspect generated and ignored artifacts. Commit only the intended report or final integration changes:

```bash
git add docs/bugs/2026-09-15-live-acceptance-fix-verification.md provider/backup_test.go provider/testserver_test.go
git commit -m "test: complete verification repairs"
```

Stage only files actually intended for this commit.
