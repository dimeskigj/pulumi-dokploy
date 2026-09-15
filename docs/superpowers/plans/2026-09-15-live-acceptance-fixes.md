# Live Acceptance Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct database mount-dispatch readiness and add safe evidence that determines whether Domain create failures belong to provider serialization or the deployed Dokploy contract.

**Architecture:** Dispatch fixtures own concrete readiness callbacks, so the generic mount lifecycle never guesses a target API from pointer presence. Domain live tests compare provider and generated-client attempts through a small structural result type that stores only target kind, status class, allowlisted code, request keys, and target readiness; production Domain behavior changes only in a later evidence-gated task.

**Tech Stack:** Go 1.26.6, `testing`, `testify/require`, Pulumi Go Provider `infer`, generated oapi-codegen client, mise, golangci-lint.

## Global Constraints

- Do not add speculative Domain payload fallbacks.
- Do not weaken cleanup verification or convert failures into skips.
- A stop marker is created only for failed verified cleanup or independently confirmed server-health failure.
- Never emit credentials, endpoint values, resource IDs, private hosts, payload values, or response bodies.
- Run live tests serially with `-parallel=1` and a fresh absent `DOKPLOY_ACCEPTANCE_STOP_FILE`.
- Do not run a later heavy test after the stop marker exists.
- Do not modify or stage the unrelated untracked report `docs/bugs/2026-09-14-live-acceptance-run.md` unless the implementation task explicitly updates it after verification.

---

### Task 1: Resource-Specific Dispatch Readiness

**Files:**
- Modify: `provider/live_workloads_test.go:389-477, 669-683, 897-1109`
- Test: `provider/live_harness_unit_test.go`
- Test: `provider/mount_lifecycle_matrix_test.go:236-248`

**Interfaces:**
- Consumes: existing resource `Read` methods for `Application`, `Compose`, `Postgres`, `MySQL`, `MariaDB`, and `Redis`; existing `statusDone` constant.
- Produces: `type liveTargetReadiness func(context.Context) (present bool, ready bool, err error)`; `liveDispatchFixture.readiness liveTargetReadiness`; `runLiveMountLifecycle(..., readiness liveTargetReadiness)`.

- [ ] **Step 1: Write the failing target-reader matrix test**

Add to `provider/live_harness_unit_test.go`. The exact response fields match each resource's existing `Read` implementation:

```go
func TestLiveTargetReadinessUsesConcreteResourceReader(t *testing.T) {
	cases := []struct {
		name, path, queryKey, id, response string
		makeRead                            func(*client.Client, string) liveTargetReadiness
	}{
		{"application", "/api/application.one", "applicationId", "a1", `{"applicationId":"a1","applicationStatus":"done"}`, applicationTargetReadiness},
		{"compose", "/api/compose.one", "composeId", "c1", `{"composeId":"c1","composeStatus":"done"}`, composeTargetReadiness},
		{"postgres", "/api/postgres.one", "postgresId", "p1", `{"postgresId":"p1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","applicationStatus":"done"}`, postgresTargetReadiness},
		{"mysql", "/api/mysql.one", "mysqlId", "m1", `{"mysqlId":"m1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","applicationStatus":"done"}`, mysqlTargetReadiness},
		{"mariadb", "/api/mariadb.one", "mariadbId", "md1", `{"mariadbId":"md1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","applicationStatus":"done"}`, mariadbTargetReadiness},
		{"redis", "/api/redis.one", "redisId", "r1", `{"redisId":"r1","name":"db","environmentId":"e1","applicationStatus":"done"}`, redisTargetReadiness},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET(tc.path, map[string][]string{tc.queryKey: {tc.id}}, http.StatusOK, tc.response))
			present, ready, err := tc.makeRead(s.API(), tc.id)(t.Context())
			require.NoError(t, err)
			require.True(t, present)
			require.True(t, ready)
		})
	}
}
```

- [ ] **Step 2: Run the reader test to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestLiveTargetReadinessUsesConcreteResourceReader$' -count=1 -v
```

Expected: build failure because `liveTargetReadiness` and the six constructor functions do not exist.

- [ ] **Step 3: Add readiness-state and error tests**

Add table tests to `provider/live_harness_unit_test.go` using the same constructor signatures. Cover a successful concrete read whose status is `running` and a `404` response:

```go
func TestPostgresTargetReadinessReportsNotReadyAndMissing(t *testing.T) {
	t.Run("not ready", func(t *testing.T) {
		s := newScriptedServer(t, expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, `{"postgresId":"p1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","applicationStatus":"running"}`))
		present, ready, err := postgresTargetReadiness(s.API(), "p1")(t.Context())
		require.NoError(t, err)
		require.True(t, present)
		require.False(t, ready)
	})
	t.Run("missing", func(t *testing.T) {
		s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/postgres.one", Query: map[string][]string{"postgresId": {"p1"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)})
		present, ready, err := postgresTargetReadiness(s.API(), "p1")(t.Context())
		require.NoError(t, err)
		require.False(t, present)
		require.False(t, ready)
	})
}
```

- [ ] **Step 4: Implement concrete readiness constructors**

In `provider/live_workloads_test.go`, replace `readLiveWorkloadTarget` with explicit callbacks. Keep this test-only code local to the live harness:

```go
type liveTargetReadiness func(context.Context) (present bool, ready bool, err error)

func applicationTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Application{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func composeTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Compose{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func postgresTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Postgres{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func mysqlTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (MySQL{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func mariadbTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (MariaDB{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func redisTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Redis{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}
```

- [ ] **Step 5: Run the readiness tests to verify GREEN**

Run:

```bash
mise exec -- go test ./provider -run '^(TestLiveTargetReadinessUsesConcreteResourceReader|TestPostgresTargetReadinessReportsNotReadyAndMissing)$' -count=1 -v
```

Expected: both tests pass and every scripted server verifies the expected concrete endpoint.

- [ ] **Step 6: Write the failing fixture wiring test**

Extend `TestPostgresMountCleanupOwnershipRunsMountBeforeFixture` or add the following in `provider/mount_lifecycle_matrix_test.go`:

```go
func TestDispatchFixtureCarriesConcreteReadiness(t *testing.T) {
	called := false
	fixture := liveDispatchFixture{readiness: func(context.Context) (bool, bool, error) {
		called = true
		return true, true, nil
	}}
	present, ready, err := fixture.readiness(t.Context())
	require.NoError(t, err)
	require.True(t, present)
	require.True(t, ready)
	require.True(t, called)
}
```

- [ ] **Step 7: Run the wiring test to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestDispatchFixtureCarriesConcreteReadiness$' -count=1 -v
```

Expected: build failure because `liveDispatchFixture` has no `readiness` field.

- [ ] **Step 8: Wire readiness through fixture creation and mount lifecycle**

In `provider/live_workloads_test.go`:

1. Add `readiness liveTargetReadiness` to `liveDispatchFixture`.
2. Add a final `readiness liveTargetReadiness` parameter to `runLiveMountLifecycle`.
3. At the top of `runLiveMountLifecycle`, replace the inferred read with `targetPresent, targetReady, err := readiness(ctx)`.
4. Pass `applicationTargetReadiness(api, target.id)` and `composeTargetReadiness(api, target.id)` from ordinary Application/Compose mount cases.
5. Set each database fixture's callback during creation:

```go
fixture := &liveDispatchFixture{
	id: created.ID, api: api, lease: lease, remove: remove,
	readID: func(c context.Context) (string, error) {
		v, e := (Postgres{client: fixedClient(api)}).Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: created.ID})
		return v.ID, e
	},
	readiness: postgresTargetReadiness(api, created.ID),
}
```

Use the corresponding MySQL, MariaDB, and Redis constructors in those branches. Pass `fixture.readiness` from `MountDispatch/postgres` and each database dispatch case. Pass `composeTargetReadiness(api, composeID)` for `MountDispatch/compose`.

- [ ] **Step 9: Verify wiring and cleanup tests**

Run:

```bash
mise exec -- go test ./provider -run '^(TestLiveTargetReadiness|TestPostgresTargetReadiness|TestDispatchFixture|TestPostgresMountCleanupOwnership)' -count=1 -v
```

Expected: all selected tests pass; no scripted request is made to `/api/application.one` for a database ID.

- [ ] **Step 10: Commit Task 1**

```bash
git add provider/live_workloads_test.go provider/live_harness_unit_test.go provider/mount_lifecycle_matrix_test.go
git commit -m "test: fix mount dispatch readiness"
```

---

### Task 2: Correct Stop Classification For Missing Targets

**Files:**
- Modify: `provider/live_workloads_test.go:897-934, 1021-1109`
- Modify: `provider/live_harness_test.go:285-338, 579-664`
- Test: `provider/live_harness_unit_test.go`

**Interfaces:**
- Consumes: `liveTargetReadiness` from Task 1; `classifyWorkloadCreateAttempt`; `maybeVerifyLiveServerHealth`; existing cleanup marker functions.
- Produces: `validateLiveTarget(ctx context.Context, operation string, keys []string, readiness liveTargetReadiness) (classification string, err error)` where missing/not-ready is ordinary and only read errors enter health probing.

- [ ] **Step 1: Write failing target-validation tests**

Add to `provider/live_harness_unit_test.go`:

```go
func TestValidateLiveTargetClassifiesMissingWithoutStopping(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	classification, err := validateLiveTarget(t.Context(), "mount", []string{"postgresId", "mountPath"}, func(context.Context) (bool, bool, error) {
		return false, false, nil
	})
	require.NoError(t, err)
	require.Equal(t, "operation=mount;status=transport;code=unknown;keys=mountPath,postgresId;target=missing", classification)
	require.False(t, heavyLiveTierStopped())
}

func TestValidateLiveTargetPropagatesReadErrorWithoutInventingHealthFailure(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	readErr := errors.New("read sentinel")
	_, err := validateLiveTarget(t.Context(), "mount", []string{"postgresId"}, func(context.Context) (bool, bool, error) {
		return false, false, readErr
	})
	require.ErrorIs(t, err, readErr)
	require.False(t, heavyLiveTierStopped())
}
```

Use the existing `resetLiveHarnessState()` helper and register it with `t.Cleanup`; it clears `liveHeavyStop` and stored results. Set marker paths only through `t.Setenv` to temporary test paths.

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestValidateLiveTarget' -count=1 -v
```

Expected: build failure because `validateLiveTarget` does not exist.

- [ ] **Step 3: Implement ordinary target validation**

Add to `provider/live_workloads_test.go` near the safe classifiers:

```go
func validateLiveTarget(ctx context.Context, operation string, keys []string, readiness liveTargetReadiness) (string, error) {
	present, ready, err := readiness(ctx)
	if err != nil {
		return "", err
	}
	if present && ready {
		return "", nil
	}
	return classifyWorkloadCreateAttempt(operation, 0, "", keys, present, ready)
}
```

Use it in `runLiveMountLifecycle`. A non-empty classification calls `t.Fatalf("workload target unavailable: %s", classification)` only after fixture cleanup ownership has been registered. Do not call `recordServerHealthFailure` for this ordinary target state. Existing cleanup failure reporting remains unchanged and may still create the marker.

- [ ] **Step 4: Add independent health-failure coverage**

Add a test showing a genuine `503` from `maybeVerifyLiveServerHealth` still creates a stop marker through `recordServerHealthFailure`:

```go
func TestTargetReadErrorStopsOnlyAfterFailedHealthProbe(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	probeErr := &client.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE"}
	require.Error(t, maybeVerifyLiveServerHealth(t.Context(), func(context.Context) error { return probeErr }))
	recordServerHealthFailure("mount-target-read", probeErr)
	require.True(t, heavyLiveTierStopped())
	_, err := os.Stat(marker)
	require.NoError(t, err)
}
```

- [ ] **Step 5: Run stop-classification tests to verify GREEN**

Run:

```bash
mise exec -- go test ./provider -run '^(TestValidateLiveTarget|TestTargetReadErrorStopsOnlyAfterFailedHealthProbe)' -count=1 -v
```

Expected: missing/not-ready target tests pass with no marker; explicit failed health probe test passes with a marker.

- [ ] **Step 6: Run existing harness safety tests**

Run:

```bash
mise exec -- go test ./provider -run '^(TestLive|TestClassifyWorkload|TestHeavy|TestServerHealth|TestCleanup)' -count=1
```

Expected: PASS. Inspect failures for changed stop semantics; do not update expectations that protect cleanup or confirmed health stops.

- [ ] **Step 7: Commit Task 2**

```bash
git add provider/live_workloads_test.go provider/live_harness_test.go provider/live_harness_unit_test.go
git commit -m "test: classify dispatch target failures safely"
```

---

### Task 3: Sanitized Domain Comparison Model

**Files:**
- Modify: `provider/live_harness_test.go:242-338, 378-410`
- Test: `provider/live_harness_unit_test.go:45-138`
- Test: `provider/domain_test.go:142-171, 247-276`

**Interfaces:**
- Consumes: `domainCreateBody(DomainArgs)`, `classifyWorkloadCreateAttempt`, `client.APIError`, generated `DomainCreateJSONRequestBody`.
- Produces: `type liveDomainCreateResult struct { path string; target string; classification string; keys []string; created bool }`; `classifyDomainComparison(provider, generated liveDomainCreateResult) string`; `domainCreateRequestKeysFromBody(generated.DomainCreateJSONRequestBody) ([]string, error)`.

- [ ] **Step 1: Write failing request-key parity tests**

Add to `provider/domain_test.go`:

```go
func TestDomainCreateRequestKeysFromBody(t *testing.T) {
	cases := []struct {
		name string
		args DomainArgs
		want []string
	}{
		{"application", DomainArgs{ApplicationID: stringPtr("a1"), Host: "app.example.invalid", Port: intPtr(80), CertificateType: CertificateNone}, []string{"applicationId", "certificateType", "domainType", "host", "https", "port", "stripPath"}},
		{"compose", DomainArgs{ComposeID: stringPtr("c1"), ServiceName: stringPtr("web"), Host: "compose.example.invalid", Port: intPtr(80), CertificateType: CertificateNone}, []string{"certificateType", "composeId", "domainType", "host", "https", "port", "serviceName", "stripPath"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domainCreateRequestKeysFromBody(domainCreateBody(tc.args))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
```

- [ ] **Step 2: Run key tests to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestDomainCreateRequestKeysFromBody$' -count=1 -v
```

Expected: build failure because `domainCreateRequestKeysFromBody` does not exist.

- [ ] **Step 3: Implement structural request-key extraction**

Add near `domainCreateRequestKeys` in `provider/live_harness_test.go`:

```go
func domainCreateRequestKeysFromBody(body generated.DomainCreateJSONRequestBody) ([]string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("domain request key encoding failed")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("domain request key decoding failed")
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if isSafeWorkloadRequestKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
```

This function returns names only and never includes values in errors.

- [ ] **Step 4: Write failing Domain comparison classifier tests**

Add to `provider/live_harness_unit_test.go`:

```go
func TestClassifyDomainComparison(t *testing.T) {
	keys := []string{"applicationId", "domainType", "host"}
	tests := []struct {
		name string
		provider, generated liveDomainCreateResult
		want string
	}{
		{"provider mismatch", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=2xx;code=unknown", keys: keys, created: true}, "provider-serialization-mismatch"},
		{"server rejection", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, "server-contract-rejection"},
		{"environment failure", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=transport;code=unknown", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, "environment-or-health-failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyDomainComparison(tt.provider, tt.generated))
		})
	}
}
```

- [ ] **Step 5: Run classifier tests to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestClassifyDomainComparison$' -count=1 -v
```

Expected: build failure because the result type and classifier do not exist.

- [ ] **Step 6: Implement the minimal structural result and classifier**

Add to `provider/live_harness_test.go`:

```go
type liveDomainCreateResult struct {
	path           string
	target         string
	classification string
	keys           []string
	created        bool
}

func classifyDomainComparison(provider, generated liveDomainCreateResult) string {
	if provider.target != generated.target || provider.target == "" || !slices.Equal(provider.keys, generated.keys) {
		return "provider-serialization-mismatch"
	}
	if strings.Contains(provider.classification, "status=transport") || strings.Contains(provider.classification, "status=5xx") || strings.Contains(generated.classification, "status=transport") || strings.Contains(generated.classification, "status=5xx") {
		return "environment-or-health-failure"
	}
	if !provider.created && generated.created {
		return "provider-serialization-mismatch"
	}
	if provider.classification == generated.classification {
		return "server-contract-rejection"
	}
	return "provider-serialization-mismatch"
}
```

Add `slices` to imports. Keep `path` for report construction but never permit arbitrary path values into output; live call sites set only `provider` or `generated`.

- [ ] **Step 7: Add secrecy assertions**

Extend the classifier test with sentinel checks:

```go
for _, sentinel := range []string{"secret-sentinel", "resource-id-sentinel", "private.example", "https://"} {
	require.NotContains(t, classifyDomainComparison(tt.provider, tt.generated), sentinel)
}
```

Also run `domainCreateRequestKeysFromBody` on a body containing sentinel values and assert only key names are returned.

- [ ] **Step 8: Run Task 3 tests to verify GREEN**

Run:

```bash
mise exec -- go test ./provider -run '^(TestDomainCreateRequestKeysFromBody|TestClassifyDomainComparison|TestClassifyWorkloadCreate)' -count=1 -v
```

Expected: PASS with no sentinel values in output.

- [ ] **Step 9: Commit Task 3**

```bash
git add provider/live_harness_test.go provider/live_harness_unit_test.go provider/domain_test.go
git commit -m "test: classify domain compatibility safely"
```

---

### Task 4: Live Domain Provider-Versus-Generated Comparison

**Files:**
- Modify: `provider/live_workloads_test.go:204-304`
- Modify: `provider/live_harness_test.go`
- Test: `provider/live_harness_unit_test.go`

**Interfaces:**
- Consumes: `liveDomainCreateResult`, `classifyDomainComparison`, `domainCreateRequestKeysFromBody`, `domainCreateBody`, `cleanupAfterCreateError`, `registerLiveCleanup`, generated Domain methods.
- Produces: `runLiveDomainCreateAttempt(...) liveDomainCreateResult`; `runGeneratedDomainCreateAttempt(...) liveDomainCreateResult`; `compareDomainAttempts(providerAttempt, generatedAttempt func() liveDomainCreateResult) string`.

- [ ] **Step 1: Write failing deterministic comparison-runner test**

Add a test using two scripted servers or injected attempt functions, avoiding live credentials:

```go
func TestCompareLiveDomainCreateRunsSerialAttemptsAndCleansCreatedResult(t *testing.T) {
	order := []string{}
	cleaned := false
	classification := compareDomainAttempts(
		func() liveDomainCreateResult {
			order = append(order, "provider")
			return liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: []string{"applicationId", "domainType", "host"}}
		},
		func() liveDomainCreateResult {
			order = append(order, "generated")
			cleaned = true
			return liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=2xx;code=unknown", keys: []string{"applicationId", "domainType", "host"}, created: true}
		},
	)
	require.Equal(t, []string{"provider", "generated"}, order)
	require.True(t, cleaned)
	require.Equal(t, "provider-serialization-mismatch", classification)
}
```

- [ ] **Step 2: Run runner test to verify RED**

Run:

```bash
mise exec -- go test ./provider -run '^TestCompareLiveDomainCreateRunsSerialAttemptsAndCleansCreatedResult$' -count=1 -v
```

Expected: build failure because `compareDomainAttempts` does not exist.

- [ ] **Step 3: Implement serial comparison orchestration**

Add to `provider/live_harness_test.go`:

```go
func compareDomainAttempts(providerAttempt, generatedAttempt func() liveDomainCreateResult) string {
	providerResult := providerAttempt()
	generatedResult := generatedAttempt()
	return classifyDomainComparison(providerResult, generatedResult)
}
```

Keep actual API execution in `provider/live_workloads_test.go`, where `testing.T` cleanup can be registered immediately.

- [ ] **Step 4: Implement provider attempt helper**

Add a helper in `provider/live_workloads_test.go` that:

1. Calculates sorted keys from `domainCreateBody(args)`.
2. Calls `Domain.Create` once.
3. Immediately registers cleanup when an ID is returned.
4. Converts errors through `classifyWorkloadCreateError("domain", err, keys, true, true)`.
5. Returns `created: true` only when create succeeds with a non-empty ID.

Use this exact signature:

```go
func runLiveDomainCreateAttempt(t *testing.T, ctx context.Context, api *client.Client, target string, args DomainArgs) liveDomainCreateResult
```

On success, classification must be generated by
`classifyWorkloadCreateAttempt("domain", http.StatusOK, "", keys, true, true)`;
do not include the ID in the result.

- [ ] **Step 5: Implement generated-client attempt helper**

Add:

```go
func runGeneratedDomainCreateAttempt(t *testing.T, ctx context.Context, api *client.Client, target string, args DomainArgs) liveDomainCreateResult
```

It calls `api.DomainCreateWithResponse(ctx, domainCreateBody(args))` exactly once. Derive status from `response.HTTPResponse.StatusCode`; derive an allowlisted API code only through the typed `client.APIError` returned by the wrapped client. If `JSON200.DomainId` exists, register `Domain.Delete` plus `Domain.Read` absence verification immediately and set `created: true`. Never store or print the ID.

- [ ] **Step 6: Integrate comparison after ordinary Domain failure**

In each `Domain/application` and `Domain/compose` case, retain the ordinary lifecycle as the primary test. When provider create fails:

1. Ensure any partial provider result is cleaned.
2. Construct a second `DomainArgs` using a distinct `liveRunName("domain-direct")` host and the same ready target.
3. Run the generated attempt once.
4. Record only `domain-comparison=<classification>` through `recordLiveOutcome`.
5. Fail the primary case with its existing sanitized classification.

Do not retry alternate payload fields. Do not run comparison when target readiness failed or when the stop marker already exists.

- [ ] **Step 7: Add source-contract secrecy coverage**

Extend `TestLiveDiagnosticSourceContract` expectations in `tests/acceptance_test.go` only if its AST rules reject the new helpers. Ensure all live execution helpers use structural classifiers and never call `err.Error`, `%#v`, or ordinary `require.Contains`/`require.Equal` on sensitive values.

Run:

```bash
mise exec -- go test ./tests -run '^TestLiveDiagnosticSourceContract$' -count=1 -v
```

Expected: PASS without weakening prohibited-pattern checks.

- [ ] **Step 8: Run deterministic Domain comparison tests**

Run:

```bash
mise exec -- go test ./provider ./tests -run 'DomainComparison|DomainCreateRequestKeys|LiveDiagnosticSourceContract' -count=1 -v
```

Expected: PASS.

- [ ] **Step 9: Commit Task 4**

```bash
git add provider/live_workloads_test.go provider/live_harness_test.go provider/live_harness_unit_test.go tests/acceptance_test.go
git commit -m "test: compare live domain create paths"
```

---

### Task 5: Static Verification And Focused Live Evidence

**Files:**
- Modify after live evidence: `docs/bugs/2026-09-14-live-acceptance-run.md`
- No production file changes unless Task 4 proves `provider-serialization-mismatch`.

**Interfaces:**
- Consumes: all Task 1-4 tests and live helpers.
- Produces: verified dispatch result and a sanitized Domain ownership classification.

- [ ] **Step 1: Run focused deterministic tests**

```bash
mise exec -- go test ./provider ./tests -run 'LiveTargetReadiness|ValidateLiveTarget|DispatchFixture|DomainComparison|DomainCreateRequestKeys|LiveDiagnosticSourceContract' -count=1 -v
```

Expected: PASS.

- [ ] **Step 2: Run the short provider/internal/tests suite**

```bash
mise exec -- go test -short -count=1 ./provider/... ./internal/... ./tests/...
```

Expected: PASS; live tests skip when protected variables are deliberately omitted from this command environment.

- [ ] **Step 3: Run the race suite**

```bash
mise exec -- go test -race ./provider/... ./internal/...
```

Expected: PASS with no race reports.

- [ ] **Step 4: Run lint and whitespace verification**

```bash
mise exec -- golangci-lint run
git diff --check
```

Expected: both exit 0.

- [ ] **Step 5: Prepare live safety state**

Set a new path without printing its value, verify `DOKPLOY_ACCEPTANCE`, endpoint, and API key are set without printing values, remove only the new marker, and verify authenticated control-plane health returns HTTP 200. Preserve every prior incident marker.

- [ ] **Step 6: Run focused PostgreSQL dispatch**

```bash
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
```

Expected: PASS; PostgreSQL readiness uses `/api/postgres.one`; mount and fixture cleanup complete; the new marker remains absent. If it fails or the marker exists, stop and do not run Domain diagnostics.

- [ ] **Step 7: Run focused Domain diagnostics serially**

```bash
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/application$' -parallel=1 -count=1 -v
mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/compose$' -parallel=1 -count=1 -v
```

Expected: each primary lifecycle either passes or fails with the existing sanitized classification and records exactly one of `provider-serialization-mismatch`, `server-contract-rejection`, or `environment-or-health-failure`. Check the marker after each command; stop immediately if it exists.

- [ ] **Step 8: Update the sanitized report**

Append a section to `docs/bugs/2026-09-14-live-acceptance-run.md` containing revision, exact safe commands, durations, pass/fail status, comparison classification, and cleanup/marker state. Do not include endpoint hostname, IDs, hosts, payloads, response bodies, or credentials.

- [ ] **Step 9: Commit verified diagnostics and report**

```bash
git add docs/bugs/2026-09-14-live-acceptance-run.md
git commit -m "docs: record acceptance fix verification"
```

---

### Task 6: Evidence-Gated Domain Production Correction

**Condition:** Execute this task only if Task 5 records `provider-serialization-mismatch`. If it records `server-contract-rejection`, cancel this task and retain production `provider/domain.go` unchanged. If it records `environment-or-health-failure`, block this task pending new evidence.

**Files:**
- Modify: `provider/domain.go:173-270`
- Test: `provider/domain_test.go`
- Regenerate only if the proved mismatch is in the pinned schema: `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`

**Interfaces:**
- Consumes: the exact structural difference recorded by Task 5.
- Produces: one corrected Domain request transformation with no compatibility retry.

- [ ] **Step 1: Write one failing regression test for the proved difference**

Add a table case to `TestDomainCreateBodyMatrix` or a focused scripted-server test. The expected JSON must encode exactly the key/type difference proved in Task 5 and preserve all unaffected fields. Do not invent an alternate request. Name the test after the concrete contract, for example `TestDomainCreateOmitsUnsupportedDomainType` only if evidence proves that omission.

- [ ] **Step 2: Run the exact regression test to verify RED**

```bash
mise exec -- go test ./provider -run '^TestDomainCreate<ConcreteContractName>$' -count=1 -v
```

Expected: FAIL showing only the proved structural mismatch.

- [ ] **Step 3: Implement the smallest production correction**

Change only `domainCreateBody` or the relevant normalized OpenAPI correction. Keep a single request path. Do not add `BAD_REQUEST` fallback retries or server-version branching.

- [ ] **Step 4: Regenerate only when schema correction is required**

```bash
mise exec -- make generate_openapi
mise exec -- make check_openapi
```

Expected: generated output matches the corrected normalized schema. If the fix is provider mapping only, skip regeneration and verify generated files are unchanged.

- [ ] **Step 5: Run Domain and safety tests**

```bash
mise exec -- go test ./provider ./tests -run 'Domain|ClassifyWorkload|LiveDiagnosticSourceContract' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Repeat full static and focused live verification**

Repeat Task 5 Steps 2-7 under a new absent marker. Expected: focused Domain Application and Compose pass, or produce new evidence requiring investigation; no marker exists.

- [ ] **Step 7: Commit the evidence-backed correction**

```bash
git add provider/domain.go provider/domain_test.go openapi/dokploy.json internal/client/generated/generated.gen.go docs/bugs/2026-09-14-live-acceptance-run.md
git commit -m "fix: align domain create contract"
```

Stage only files actually changed; omit generated files when no schema change occurred.
