# Workload API Compatibility Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Determine why the deployed Dokploy rejects Domain and Application/Compose Mount creates without making speculative provider request changes.

**Architecture:** First prove provider-to-generated-client serialization against the checked-in OpenAPI for every failing branch. Then add secret-safe live structural diagnostics and compare the deployed Dokploy revision, handlers, and migrations with the pinned API source before deciding whether the fix belongs in the provider or Dokploy.

**Tech Stack:** Go 1.26.6, `testing`, `testify/require`, kin-openapi, generated Dokploy HTTP client, serial live acceptance harness.

## Global Constraints

- Do not change Domain or Mount production request construction until one request field is proven incompatible with the deployed server.
- Never log workload IDs, hostnames, host paths, file contents, raw SQL errors, credentials, or request/response bodies.
- Keep `.example.invalid` hosts, certificate type `none`, and serial execution.
- Treat PostgreSQL/MySQL/MariaDB/Redis Mount success as the control group for generic mount dispatch.

---

### Task 1: Deterministic Request Contract Matrix

**Files:**
- Modify: `provider/domain_test.go`
- Modify: `provider/mount_lifecycle_matrix_test.go`
- Modify: `internal/client/client_test.go`

**Interfaces:**
- Consumes: `domainCreateBody`, `mountCreateBody`, `generated.NewDomainCreateRequest`, and `generated.NewMountsCreateRequest`.
- Produces: exhaustive serialized-request evidence for two Domain targets and eighteen Mount target/type combinations.

- [ ] **Step 1: Add Domain create matrix tests**

Table-test Application and Compose inputs using port 80, HTTPS false, certificate `none`, and strip-path false. Assert Application emits `applicationId` plus `domainType:"application"`; Compose emits `composeId`, `serviceName:"web"`, and `domainType:"compose"`; neither emits the other target ID.

- [ ] **Step 2: Add Mount Cartesian tests**

Table-test target mappings `application`, `compose`, `postgres`, `mysql`, `mariadb`, and `redis` crossed with `bind`, `volume`, and `file`. For each case assert exactly one typed Pulumi target becomes the documented generic `serviceId` and `serviceType`, and only the selected mount-type value is non-null.

- [ ] **Step 3: Add generated request serialization tests**

Use a recording `http.Client` transport to build requests with `generated.NewDomainCreateRequest` and `generated.NewMountsCreateRequest`. Assert method POST, paths `/domain.create` and `/mounts.create`, JSON content type, and JSON-semantic equality with the provider-body matrix.

- [ ] **Step 4: Run deterministic verification**

Run: `go test ./provider ./internal/client -run 'TestDomain.*Create|TestMount.*Create|TestGenerated.*Create' -count=1`

Expected: PASS without production changes. Any failure here supersedes the server hypothesis and must be fixed test-first before Task 2.

- [ ] **Step 5: Commit**

```bash
git add provider/domain_test.go provider/mount_lifecycle_matrix_test.go internal/client/client_test.go
git commit -m "test: lock workload create request contracts"
```

### Task 2: Secret-Safe Live Diagnostics

**Files:**
- Modify: `provider/live_harness_test.go`
- Modify: `provider/live_harness_unit_test.go`
- Modify: `provider/live_workloads_test.go`

**Interfaces:**
- Produces: `classifyWorkloadCreateAttempt(operation string, status int, apiCode string, keys []string, targetPresent bool, targetReady bool) string` returning labels only.
- Consumes: sanitized error metadata and request-key names; never request values.

- [ ] **Step 1: Write redaction and classification tests**

Assert classifications include only operation, status class, safe API code, sorted key names, target-present boolean, and target-ready boolean. Seed IDs, hostnames, paths, content, API keys, and SQL text as sentinels and assert none appears in output.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./provider -run 'TestClassifyWorkloadCreateAttempt' -count=1`

Expected: compilation failure because the classifier does not exist.

- [ ] **Step 3: Implement the pure classifier**

Return a fixed-format label assembled only from an allowlist:

```text
operation=<domain|mount>;status=<2xx|4xx|5xx|transport>;code=<sanitized-code>;keys=<sorted-safe-keys>;target=<missing|present-not-ready|ready>
```

Reject unknown operation names and replace non-alphanumeric API codes with `unknown`.

- [ ] **Step 4: Refresh target state immediately before create**

In each Domain and Application/Compose Mount live case, call the target resource `Read` immediately before the dependent create. Record only whether it exists and whether status is `done`; fail before create when absent or not ready.

- [ ] **Step 5: Record structural failure evidence**

On create failure, record the classifier using only generated request key names and typed error status/code. Preserve the existing sanitized provider error as the test failure and never emit raw server details.

- [ ] **Step 6: Verify non-live safety**

Run: `go test ./provider -run 'TestClassifyWorkloadCreateAttempt|TestTask9|TestLiveHarness' -count=1`

Expected: PASS, including sentinel redaction.

- [ ] **Step 7: Commit**

```bash
git add provider/live_harness_test.go provider/live_harness_unit_test.go provider/live_workloads_test.go
git commit -m "test: add safe workload compatibility diagnostics"
```

### Task 3: Deployed Dokploy Compatibility Decision

**Files:**
- Modify: `docs/bugs/2026-09-05-live-acceptance-run.md`
- Create conditionally: `docs/bugs/2026-09-06-domain-create-compatibility.md`
- Create conditionally: `docs/bugs/2026-09-06-mount-create-compatibility.md`

**Interfaces:**
- Consumes: pinned source commit from `openapi/source.json`, deployed Dokploy revision, deterministic request matrix, and focused live classifications.
- Produces: an evidence-backed owner and exact follow-up patch boundary.

- [ ] **Step 1: Record deployed revision safely**

Determine the deployed Dokploy version or commit from server administration metadata without recording endpoint or credentials. Compare it with the commit in `openapi/source.json`.

- [ ] **Step 2: Run focused cases once**

Run serially with the acceptance stop-file contract:

```bash
go test ./provider -run '^TestLiveTier2Workloads/Domain/(application|compose)$' -parallel=1 -count=1 -v
go test ./provider -run '^TestLiveTier2Workloads/Mount/(application|compose)$' -parallel=1 -count=1 -v
```

Expected: either successful lifecycle/cleanup or sanitized structural classifications sufficient to compare target branches.

- [ ] **Step 3: Inspect matching server handlers and migrations**

For `/domain.create`, compare application/Compose target lookup, `domainType`, `serviceName`, and domain foreign-key columns. For `/mounts.create`, compare application/Compose switch branches and foreign-key columns against the four working database branches. Verify all required migrations are applied on the deployed instance.

- [ ] **Step 4: Apply the evidence decision**

Use this fixed decision table:

| Evidence | Action |
| --- | --- |
| Deployed handler expects fields different from pinned OpenAPI | Correct `openapi/corrections.json`, regenerate, add the failing request fixture, then minimally update provider mapping. |
| Handler matches request but Application/Compose FK or migration fails | Patch Dokploy/migrations; make no provider production change. |
| Deployed version predates the pinned contract | Document a minimum compatible Dokploy version and fail with a targeted compatibility diagnostic. |
| Irrelevant null omission alone changes outcome | Omit irrelevant fields on create only; retain explicit-null clearing on update. |

- [ ] **Step 5: Update sanitized reports**

Record version comparison, classification, ownership, workaround, and cleanup status. Create a provider bug report only if a provider request mismatch is reproduced; otherwise retain the finding as server/environment compatibility evidence in the run summary.

- [ ] **Step 6: Commit evidence**

```bash
git add docs/bugs
git commit -m "docs: classify workload API compatibility failures"
```
