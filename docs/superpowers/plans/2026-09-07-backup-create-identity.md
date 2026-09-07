# Backup Create Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Discover a newly created Dokploy backup through bounded exact-match polling without adopting unrelated or ambiguous backups.

**Architecture:** Parse each target database's nested backups into an internal observation type, match only observable fields against the create request, and poll until exactly one new matching record is visible. The Pulumi operation context bounds discovery; ambiguous candidates fail safely and no process-local lock is introduced.

**Tech Stack:** Go 1.26.6, `pulumi-go-provider/infer`, generated oapi-codegen client, `httptest` scripted server, Testify.

## Global Constraints

- Do not change the Pulumi schema or the `dokploy:index:Backup` token.
- Do not add process-local locking.
- Do not modify generated OpenAPI code manually.
- Prefer false negatives over adopting an unrelated backup.
- Respect the Pulumi operation context and preserve `errors.Is` for cancellation and deadline errors.
- Do not log backup response payloads or user-supplied values in discovery errors.
- Existing user changes and unrelated files must remain untouched.

---

## File Structure

- Modify `provider/backup.go`: define backup observations, parse target collections, match create inputs, and poll for one unique candidate.
- Modify `provider/backup_test.go`: specify matching, delayed visibility, concurrency, ambiguity, cancellation, and target-dispatch behavior.
- Modify `website/src/content/docs/reference/backup.mdx`: document the residual orphan risk when Dokploy cannot expose one unique created backup.

### Task 1: Parse and Match Backup Observations

**Files:**
- Modify: `provider/backup.go:156-218`
- Test: `provider/backup_test.go:78-137`

**Interfaces:**
- Produces: `type backupObservation struct`, `func backupObservationsForTarget(context.Context, *client.Client, string, string) (map[string]backupObservation, error)`, and `func (backupObservation) matchesCreate(string, string, BackupArgs) bool`.
- Consumes: `BackupArgs`, target-specific generated `*.one` responses, and nested `AdditionalProperties["backups"]`.

- [ ] **Step 1: Add failing pure matching tests**

Add table-driven tests that construct observations directly and prove exact matching, unrelated schedule/destination/database/prefix rejection, target-type rejection, optional enabled handling, and nullable retention handling. Use this shape:

```go
func TestBackupObservationMatchesCreate(t *testing.T) {
	trueValue := true
	three := 3
	args := BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "p-",
		DestinationID: "d1", Database: "app", KeepLatestCount: &three,
		PostgresID: stringPtr("pg1"),
	}
	base := backupObservation{
		ID: "b1", Schedule: "0 0 * * *", Enabled: &trueValue,
		Prefix: "p-", DestinationID: "d1", Database: "app",
		DatabaseType: backupDatabaseTypePostgres, TargetID: "pg1",
		KeepLatestCount: &three,
	}

	tests := []struct {
		name string
		mutate func(*backupObservation)
		want bool
	}{
		{name: "exact", want: true},
		{name: "other schedule", mutate: func(v *backupObservation) { v.Schedule = "0 1 * * *" }},
		{name: "other destination", mutate: func(v *backupObservation) { v.DestinationID = "d2" }},
		{name: "other database", mutate: func(v *backupObservation) { v.Database = "other" }},
		{name: "other prefix", mutate: func(v *backupObservation) { v.Prefix = "other-" }},
		{name: "other type", mutate: func(v *backupObservation) { v.DatabaseType = backupDatabaseTypeMySQL }},
		{name: "other target", mutate: func(v *backupObservation) { v.TargetID = "pg2" }},
		{name: "other enabled", mutate: func(v *backupObservation) { disabled := false; v.Enabled = &disabled }},
		{name: "other retention", mutate: func(v *backupObservation) { four := 4; v.KeepLatestCount = &four }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := base
			if test.mutate != nil { test.mutate(&got) }
			require.Equal(t, test.want, got.matchesCreate(backupDatabaseTypePostgres, "pg1", args))
		})
	}
}
```

Add explicit cases proving an omitted `enabled` field is unknown and accepted, while an omitted `keepLatestCount` matches only a request with no retention value.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./provider -run '^TestBackupObservationMatchesCreate$' -count=1`

Expected: compilation fails because `backupObservation` and `matchesCreate` do not exist.

- [ ] **Step 3: Implement the observation and pure matcher**

Add an internal type with fields needed by the design:

```go
type backupObservation struct {
	ID              string
	Schedule        string
	Enabled         *bool
	Prefix          string
	DestinationID   string
	Database        string
	DatabaseType    string
	TargetID        string
	KeepLatestCount *int
}
```

Implement `matchesCreate` with direct comparisons. Required fields must equal the request. Compare `Enabled` only when observed. Require both retention pointers to be nil or both concrete and equal.

- [ ] **Step 4: Replace ID-only target parsing with observations**

Rename `backupIDsForTarget` to `backupObservationsForTarget`. Preserve target dispatch and errors, but parse each nested object into `backupObservation`. Convert JSON numeric retention safely only when it is integral and within `int` range; skip malformed records instead of matching them. Derive `TargetID` from the target-specific property and fall back to the known target ID only when Dokploy omits that property.

- [ ] **Step 5: Run matching and existing backup tests**

Run: `go test ./provider -run '^TestBackup' -count=1`

Expected: matching tests pass; create tests may still fail until Task 2 updates discovery.

- [ ] **Step 6: Commit the parsing and matching unit**

```bash
git add provider/backup.go provider/backup_test.go
git commit -m "refactor: model observed Dokploy backups"
```

### Task 2: Add Bounded Unique Candidate Discovery

**Files:**
- Modify: `provider/backup.go:242-282`
- Test: `provider/backup_test.go:78-137`

**Interfaces:**
- Consumes: `backupObservationsForTarget` and `backupObservation.matchesCreate` from Task 1.
- Produces: `var backupCreatePollInterval time.Duration` and `func waitForCreatedBackup(context.Context, *client.Client, string, string, map[string]backupObservation, BackupArgs) (string, error)`.

- [ ] **Step 1: Rewrite create tests as full observable records**

Update nested backup JSON fixtures to contain matching fields, for example:

```json
{"backupId":"new1","schedule":"0 0 * * *","enabled":true,"prefix":"p-","destinationId":"d1","database":"app","databaseType":"postgres","postgresId":"pg1","keepLatestCount":null}
```

This prevents ID-only fixtures from accidentally being accepted.

- [ ] **Step 2: Add a failing delayed-visibility test**

Set `backupCreatePollInterval` to one millisecond with `t.Cleanup` restoring it. Script pre-create read, create, two unchanged reads, then one exact matching backup. Assert `Create` returns `new1`.

Run: `go test ./provider -run '^TestBackupCreateWaitsForDelayedVisibility$' -count=1`

Expected: FAIL because current creation performs only one post-create read.

- [ ] **Step 3: Add failing concurrent-candidate tests**

Add these scripted scenarios:

- One unrelated new backup appears first, then the exact match appears: return the exact match.
- One exact match and one unrelated new backup appear together: return the exact match.
- Two exact matches appear together: return an error containing `2 matching backups` and an empty response ID.

Run: `go test ./provider -run '^TestBackupCreate_(IgnoresUnrelatedCandidate|SelectsUniqueMatchAmongNewBackups|RejectsMultipleExactMatches)$' -count=1`

Expected: FAIL because current code counts IDs without matching fields.

- [ ] **Step 4: Add failing cancellation and deadline tests**

Use a short context and repeated unchanged target responses. Assert the error wraps `context.DeadlineExceeded`. Add a canceled context case and assert `errors.Is(err, context.Canceled)`. Both messages must contain `backup.create succeeded but no unique matching backup became visible`.

Run: `go test ./provider -run '^TestBackupCreate_(Deadline|Cancellation)' -count=1`

Expected: FAIL because current code returns immediately and does not wrap context errors.

- [ ] **Step 5: Implement bounded discovery**

Add:

```go
var backupCreatePollInterval = 2 * time.Second
```

Implement `waitForCreatedBackup` as a cancellation-aware loop. Build candidates from IDs absent in the pre-create map and observations satisfying `matchesCreate`. Return one ID, return an ambiguity error for more than one candidate, or wait with `time.NewTimer(backupCreatePollInterval)` when none match. Stop and drain timers consistently with `waitForDone`.

When context ends, wrap it:

```go
return "", fmt.Errorf(
	"backup.create succeeded but no unique matching backup became visible on %s %s: %w",
	databaseType, targetID, ctx.Err(),
)
```

- [ ] **Step 6: Integrate discovery into `Backup.Create`**

Capture pre-create observations, submit `BackupCreateWithResponse`, call `waitForCreatedBackup`, and return its unique ID. Preserve dry-run and create-request failure behavior.

- [ ] **Step 7: Run focused and full provider tests**

Run: `go test ./provider -run '^TestBackup' -count=1`

Expected: PASS.

Run: `go test -short ./provider/... ./internal/... -count=1`

Expected: PASS with live tests skipped by the existing opt-in guard.

- [ ] **Step 8: Run the race suite**

Run: `go test -race ./provider/... ./internal/...`

Expected: PASS.

- [ ] **Step 9: Commit unique discovery**

```bash
git add provider/backup.go provider/backup_test.go
git commit -m "fix: safely discover created backup identity"
```

### Task 3: Document Residual Recovery Behavior

**Files:**
- Modify: `website/src/content/docs/reference/backup.mdx`
- Test: `website/tests/content.test.mjs`

**Interfaces:**
- Consumes: final error and recovery semantics from Task 2.
- Produces: user guidance for ambiguous or timed-out creates.

- [ ] **Step 1: Add a failing content assertion**

Extend the backup documentation test to require language stating that Dokploy may successfully create a schedule before identity discovery times out, and users must inspect Dokploy before retrying to avoid duplicates.

- [ ] **Step 2: Run the focused website test and verify RED**

Run: `node --test website/tests/content.test.mjs`

Expected: FAIL because the recovery guidance is absent.

- [ ] **Step 3: Add concise recovery guidance**

Add a lifecycle note to `website/src/content/docs/reference/backup.mdx` explaining:

```markdown
Dokploy does not return the new backup ID from `backup.create`. The provider waits for one uniquely matching schedule to appear on the target database. If creation reports an ambiguity or visibility timeout, inspect the target in Dokploy before retrying because the schedule may already exist.
```

- [ ] **Step 4: Verify documentation and provider suites**

Run: `make docs_check`

Expected: PASS.

Run: `go test -short ./provider/... ./internal/... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit documentation**

```bash
git add website/src/content/docs/reference/backup.mdx website/tests/content.test.mjs
git commit -m "docs: explain backup create recovery"
```

### Task 4: Final Verification

**Files:**
- Verify only; do not modify generated SDKs or schema.

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: release-ready verification evidence.

- [ ] **Step 1: Run formatting and inspect the diff**

Run: `gofmt -w provider/backup.go provider/backup_test.go`

Run: `git diff --check`

Expected: no whitespace errors.

- [ ] **Step 2: Run all offline checks available in the repository toolchain**

Run: `make test_provider`

Expected: PASS.

Run: `make test_race`

Expected: PASS.

Run: `make lint`

Expected: PASS.

Run: `make docs_check`

Expected: PASS.

- [ ] **Step 3: Run live backup acceptance when credentials are available**

Run: `DOKPLOY_ACCEPTANCE=1 go test ./provider -run '^TestLiveTier4Backups$' -parallel=1 -count=1 -v`

Expected: all configured database and volume backup lifecycle cases pass and cleanup verifies absence. If credentials are unavailable, record the test as not run rather than claiming live verification.

- [ ] **Step 4: Review repository status**

Run: `git status --short`

Expected: only intended implementation files are modified or the tree is clean after commits; pre-existing user changes remain untouched.
