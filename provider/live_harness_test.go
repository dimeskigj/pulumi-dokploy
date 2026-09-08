package dokploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/google/uuid"
	p "github.com/pulumi/pulumi-go-provider"
)

const liveCleanupTimeout = 5 * time.Minute

var liveCleanupPollInterval = 2 * time.Second

var liveHeavyStop atomic.Bool

var liveSecretRegistry struct {
	sync.RWMutex
	values map[string]int
}

const liveStopMarkerEnvironment = "DOKPLOY_ACCEPTANCE_STOP_FILE"

var liveHeavyOperation struct {
	sync.Mutex
	kind string
}

type liveHeavyOperationLease struct {
	kind     string
	released bool
}

func beginLiveHeavyOperation(t *testing.T, kind string) *liveHeavyOperationLease {
	t.Helper()
	if !heavyLiveTierAvailable() {
		t.Skip("live heavy tier stopped after cleanup failure")
	}
	liveHeavyOperation.Lock()
	defer liveHeavyOperation.Unlock()
	if liveHeavyOperation.kind != "" {
		t.Fatalf("heavy live operation %q is already active; cannot start %q", liveHeavyOperation.kind, kind)
	}
	liveHeavyOperation.kind = kind
	return &liveHeavyOperationLease{kind: kind}
}

func endLiveHeavyOperation(kind string) bool {
	liveHeavyOperation.Lock()
	defer liveHeavyOperation.Unlock()
	if liveHeavyOperation.kind != kind {
		return false
	}
	liveHeavyOperation.kind = ""
	return true
}

func (lease *liveHeavyOperationLease) release(t *testing.T) bool {
	t.Helper()
	if lease.released {
		return true
	}
	if !endLiveHeavyOperation(lease.kind) {
		t.Errorf("failed to release heavy live operation %q", lease.kind)
		return false
	}
	lease.released = true
	return true
}

func (lease *liveHeavyOperationLease) releaseIfNeeded(t *testing.T) {
	t.Helper()
	if !lease.released && !lease.release(t) {
		t.Errorf("heavy live operation %q remained active", lease.kind)
	}
}

// handleLiveHeavyCreateError closes the lease on every create error. A
// provider may return both an ID and an error; in that case cleanup happens
// synchronously before the test reports the fatal create failure.
func handleLiveHeavyCreateError(t *testing.T, lease *liveHeavyOperationLease, id string, createErr error, cleanup func()) {
	t.Helper()
	cleanupLiveHeavyCreateFailure(t, lease, id, createErr, cleanup)
	if createErr != nil {
		requireNoError(t, createErr)
	}
}

func cleanupLiveHeavyCreateFailure(t *testing.T, lease *liveHeavyOperationLease, id string, createErr error, cleanup func()) {
	t.Helper()
	if createErr == nil {
		return
	}
	if id != "" && cleanup != nil {
		cleanup()
	}
	lease.releaseIfNeeded(t)
}

var liveResultStore struct {
	sync.Mutex
	results []liveResult
}

type liveResult struct {
	kind         string
	diagnostic   string
	heavyFailure bool
}

func mountCaseNames() []string { return []string{mountTypeBind, mountTypeVolume, mountTypeFile} }

func liveResultKinds(results []liveResult) []string {
	kinds := make([]string, 0, len(results))
	for _, result := range results {
		kinds = append(kinds, result.kind)
	}
	return kinds
}

func recordLiveOutcome(kind, classification string) {
	recordLiveResult(kind, classification)
}

func classifyWorkloadCreateAttempt(operation string, status int, apiCode string, keys []string, targetPresent bool, targetReady bool) (string, error) {
	if operation != "domain" && operation != "mount" {
		return "", fmt.Errorf("unsupported workload operation")
	}
	statusClass := "transport"
	switch {
	case status >= 200 && status < 300:
		statusClass = "2xx"
	case status >= 400 && status < 500:
		statusClass = "4xx"
	case status >= 500 && status < 600:
		statusClass = "5xx"
	}
	if !isSafeWorkloadAPICode(apiCode) {
		apiCode = "unknown"
	}
	safeKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if !isSafeWorkloadRequestKey(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		safeKeys = append(safeKeys, key)
	}
	sort.Strings(safeKeys)
	keysLabel := "none"
	if len(safeKeys) > 0 {
		keysLabel = strings.Join(safeKeys, ",")
	}
	target := "missing"
	if targetPresent {
		target = "present-not-ready"
		if targetReady {
			target = "ready"
		}
	}
	return fmt.Sprintf("operation=%s;status=%s;code=%s;keys=%s;target=%s", operation, statusClass, apiCode, keysLabel, target), nil
}

func classifyWorkloadCreateError(operation string, err error, keys []string, targetPresent bool, targetReady bool) (string, error) {
	status, code := 0, ""
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		status, code = apiErr.StatusCode, apiErr.Code
	}
	return classifyWorkloadCreateAttempt(operation, status, code, keys, targetPresent, targetReady)
}

func requireWorkloadCreateNoError(t *testing.T, operation string, err error, keys []string, targetPresent bool, targetReady bool, outcome string) {
	t.Helper()
	if err == nil {
		return
	}
	classification, classificationErr := classifyWorkloadCreateError(operation, err, keys, targetPresent, targetReady)
	requireNoError(t, classificationErr)
	if outcome != "" {
		recordLiveOutcome(outcome, classification)
	}
	t.Fatalf("%s create failed: %s", operation, classification)
}

func classifyWorkloadLifecycleError(operation string, err error) (string, error) {
	if operation != "domain" && operation != "mount" {
		return "", fmt.Errorf("unsupported workload operation")
	}
	status, code := 0, ""
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		status, code = apiErr.StatusCode, apiErr.Code
	}
	statusClass := "transport"
	switch {
	case status >= 200 && status < 300:
		statusClass = "2xx"
	case status >= 400 && status < 500:
		statusClass = "4xx"
	case status >= 500 && status < 600:
		statusClass = "5xx"
	}
	if !isSafeWorkloadAPICode(code) {
		code = "unknown"
	}
	return fmt.Sprintf("operation=%s;status=%s;code=%s", operation, statusClass, code), nil
}

func requireWorkloadLifecycleNoError(t *testing.T, operation string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	classification, classificationErr := classifyWorkloadLifecycleError(operation, err)
	requireNoError(t, classificationErr)
	t.Fatalf("%s", classification)
}

func liveDiffKind(diff map[string]p.PropertyDiff, field string) (p.DiffKind, bool) {
	property, ok := diff[field]
	if !ok {
		return p.DiffKind(""), false
	}
	return property.Kind, true
}

func requireLiveDiffKind(t *testing.T, diff map[string]p.PropertyDiff, field string, want p.DiffKind) {
	t.Helper()
	kind, ok := liveDiffKind(diff, field)
	if !ok {
		t.Errorf("live diff missing field %s", field)
		return
	}
	if kind != want {
		t.Errorf("live diff field %s did not have expected kind", field)
	}
}

func deleteAndVerifyOnce(remove func() error, read func() (string, error), markUnowned func()) error {
	if err := remove(); err != nil && !client.IsNotFound(err) {
		return err
	}
	markUnowned()
	id, err := read()
	if err != nil {
		return err
	}
	if id != "" {
		return fmt.Errorf("resource remained after delete verification")
	}
	return nil
}

func cleanupAfterCreateErrorNeedsImmediateCleanup(id string, createErr error) bool {
	return id != "" && createErr != nil
}

func isSafeWorkloadAPICode(code string) bool {
	switch code {
	case "BAD_REQUEST", "NOT_FOUND", "VALIDATION_ERROR":
		return true
	default:
		return false
	}
}

func isSafeWorkloadRequestKey(key string) bool {
	switch key {
	case "host", "https", "stripPath", "certificateType", "path", "internalPath", "port", "serviceName", "customCertResolver", "applicationId", "domainType", "composeId", "mountPath", "serviceId", "serviceType", "type", "hostPath", "volumeName", "filePath", "content", "postgresId", "mysqlId", "mariadbId", "redisId":
		return true
	default:
		return false
	}
}

func classifyEnvironmentUpdateComparison(providerErr, directErr error) string {
	if providerErr == nil && directErr == nil {
		return "both-succeeded"
	}
	if providerErr != nil && directErr == nil {
		return "provider-failed-direct-succeeded"
	}
	if providerErr == nil && directErr != nil {
		return "provider-succeeded-direct-failed"
	}
	return "both-failed"
}

func classifyOrganizationActiveShape(body []byte) string {
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil {
		return "invalid-json"
	}
	classify := func(raw json.RawMessage) (valid bool, label string) {
		var id string
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false, "null-id"
		}
		if json.Unmarshal(raw, &id) != nil {
			return false, "wrong-type-id"
		}
		if id == "" {
			return false, "empty-id"
		}
		return true, ""
	}
	if raw, ok := payload["id"]; ok {
		if valid, _ := classify(raw); valid {
			return "flat-id"
		}
	}
	if raw, ok := payload["organizationId"]; ok {
		if valid, _ := classify(raw); valid {
			return "flat-organization-id"
		}
	}
	if raw, ok := payload["id"]; ok {
		_, label := classify(raw)
		return label
	}
	if raw, ok := payload["organizationId"]; ok {
		_, label := classify(raw)
		return label
	}
	if raw, ok := payload["organization"]; ok {
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil {
			return "nested-organization-object"
		}
	}
	return "missing-id"
}

func pollProjectTagAssociation(ctx context.Context, read func(context.Context) (bool, error)) (bool, string) {
	deadline := time.Now().Add(2 * time.Second)
	seen := false
	for {
		appeared, err := read(ctx)
		if err != nil {
			return false, "read-error"
		}
		if appeared {
			if seen {
				return true, "appeared-after-delay"
			}
			return true, "appeared-immediately"
		}
		seen = true
		if time.Now().After(deadline) {
			return false, "never-visible"
		}
		select {
		case <-ctx.Done():
			return false, "read-error"
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// liveAcceptanceEnabled is deliberately pure: an explicit opt-in is required
// in addition to both endpoint credentials before any live operation can start.
func liveAcceptanceEnabled() bool {
	return os.Getenv("DOKPLOY_ACCEPTANCE") == "1" &&
		os.Getenv("DOKPLOY_ENDPOINT") != "" && os.Getenv("DOKPLOY_API_KEY") != ""
}

func requireLiveAcceptance(t *testing.T) {
	t.Helper()
	if !liveAcceptanceEnabled() {
		t.Skip("live Dokploy acceptance is not enabled (set DOKPLOY_ACCEPTANCE=1, DOKPLOY_ENDPOINT, and DOKPLOY_API_KEY)")
	}
	if heavyLiveTierStopped() {
		t.Skip("live acceptance stopped after a cleanup or server-health failure")
	}
}

func liveRunName(kind string) string {
	return "pulumi-acceptance-" + kind + "-" + uuid.NewString()
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), liveCleanupTimeout)
}

func liveCleanupVerified(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	ctx, cancel := cleanupContext()
	defer cancel()
	if err := verifyLiveCleanup(ctx, remove, read); err != nil {
		reportLiveCleanup(t, kind, id, err)
	}
}

func registerLiveCleanup(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	if id == "" {
		return
	}
	t.Cleanup(func() { liveCleanupVerified(t, kind, id, remove, read) })
}

func verifyLiveCleanup(ctx context.Context, remove func(context.Context) error, read func(context.Context) (string, error)) error {
	if err := remove(ctx); err != nil && !client.IsNotFound(err) {
		return err
	}
	return waitForDatabaseAbsence(ctx, read)
}

func sanitizedLiveError(err error, msgAndArgs ...interface{}) string {
	context := fmt.Sprint(msgAndArgs...)
	if context != "" {
		context += ": "
	}
	return sanitizeLiveDiagnostic(context + fmt.Sprint(err))
}

func reportLiveCleanup(t *testing.T, kind, id string, err error) {
	if client.IsNotFound(err) {
		return
	}
	diagnostic := recordLiveCleanupFailure(kind, id, err)
	t.Errorf("%s", sanitizeLiveDiagnostic(diagnostic))
}

func recordLiveCleanupFailure(kind, id string, err error) string {
	diagnostic := fmt.Sprintf("cleanup %s %s: %v", kind, id, err)
	return recordCleanupResult(kind, diagnostic)
}

// recordLiveResult stores only sanitized diagnostics. heavyFailure must be true
// only for cleanup or server-health failures; ordinary test failures must not
// stop later tiers.
func recordLiveResult(kind string, diagnostic interface{}) {
	recordResult(liveResult{kind: kind, diagnostic: sanitizeLiveDiagnostic(toDiagnostic(diagnostic))})
}
func recordCleanupResult(kind string, diagnostic interface{}) string {
	liveHeavyStop.Store(true)
	return recordHeavyFailure(kind, diagnostic)
}
func recordServerHealthFailure(kind string, diagnostic interface{}) string {
	liveHeavyStop.Store(true)
	return recordHeavyFailure(kind, diagnostic)
}

// createLiveStopMarker publishes only a fixed, non-secret sentinel. O_EXCL
// makes the first writer win across test processes without exposing diagnostics
// or credentials to the next workflow step.
func recordHeavyFailure(kind string, diagnostic interface{}) string {
	message := toDiagnostic(diagnostic)
	if err := createLiveStopMarker(); err != nil {
		message += "; stop marker propagation failed: " + err.Error()
	}
	sanitized := sanitizeLiveDiagnostic(message)
	recordResult(liveResult{kind: kind, diagnostic: sanitized, heavyFailure: true})
	return sanitized
}

func createLiveStopMarker() error {
	path := strings.TrimSpace(os.Getenv(liveStopMarkerEnvironment))
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("open stop marker: %w", err)
	}
	if _, err := file.WriteString("stop\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write stop marker: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close stop marker: %w", err)
	}
	return nil
}
func recordResult(result liveResult) {
	liveResultStore.Lock()
	liveResultStore.results = append(liveResultStore.results, result)
	liveResultStore.Unlock()
}

type liveHarnessState struct {
	stopped bool
	results []liveResult
}

func snapshotLiveHarnessState() liveHarnessState {
	liveResultStore.Lock()
	defer liveResultStore.Unlock()
	return liveHarnessState{stopped: liveHeavyStop.Load(), results: append([]liveResult(nil), liveResultStore.results...)}
}
func restoreLiveHarnessState(state liveHarnessState) {
	liveHeavyStop.Store(state.stopped)
	liveResultStore.Lock()
	liveResultStore.results = append([]liveResult(nil), state.results...)
	liveResultStore.Unlock()
}

func heavyLiveTierStopped() bool { return liveHeavyStop.Load() }

func heavyLiveTierAvailable() bool { return !heavyLiveTierStopped() }

func resetLiveHarnessState() {
	liveHeavyStop.Store(false)
	liveHeavyOperation.Lock()
	liveHeavyOperation.kind = ""
	liveHeavyOperation.Unlock()
	liveResultStore.Lock()
	liveResultStore.results = nil
	liveResultStore.Unlock()
}

func sanitizeLiveDiagnostic(diagnostic string) string {
	secrets := configuredLiveSecrets()
	liveSecretRegistry.RLock()
	for secret := range liveSecretRegistry.values {
		secrets = append(secrets, secret)
	}
	liveSecretRegistry.RUnlock()
	for _, secret := range secrets {
		if secret != "" {
			diagnostic = strings.ReplaceAll(diagnostic, secret, "[REDACTED]")
		}
	}
	return diagnostic
}

func configuredLiveSecrets() []string {
	secrets := make([]string, 0, 16)
	for _, env := range os.Environ() {
		name, value, ok := strings.Cut(env, "=")
		if !ok || value == "" || !strings.HasPrefix(name, "DOKPLOY_") {
			continue
		}
		if name != "DOKPLOY_ACCEPTANCE" && name != liveStopMarkerEnvironment {
			secrets = append(secrets, value)
		}
	}
	secrets = append(secrets, "AKIALIVETEST", "live-test-secret", "live-test-password", "live-test-root-password")
	return secrets
}

func registerLiveSecrets(values ...string) func() {
	liveSecretRegistry.Lock()
	if liveSecretRegistry.values == nil {
		liveSecretRegistry.values = map[string]int{}
	}
	added := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			liveSecretRegistry.values[value]++
			added = append(added, value)
		}
	}
	liveSecretRegistry.Unlock()
	return func() {
		liveSecretRegistry.Lock()
		for _, value := range added {
			if liveSecretRegistry.values[value] <= 1 {
				delete(liveSecretRegistry.values, value)
			} else {
				liveSecretRegistry.values[value]--
			}
		}
		liveSecretRegistry.Unlock()
	}
}

func resetLiveSecretRegistry() {
	liveSecretRegistry.Lock()
	liveSecretRegistry.values = nil
	liveSecretRegistry.Unlock()
}

func toDiagnostic(value interface{}) string {
	if value == nil {
		return ""
	}
	if err, ok := value.(error); ok {
		return err.Error()
	}
	return strings.TrimSpace(toString(value))
}

func toString(value interface{}) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(stringify(value)), "\n", " "))
}

// Kept local so callers can pass either errors or already formatted text
// without requiring a logging dependency in the live-test harness.
func stringify(value interface{}) string {
	return fmt.Sprint(value)
}

// requireLiveEqual deliberately never formats either operand. Live assertions
// frequently compare credentials or file contents, and testify's normal
// equality diagnostics would put those values in CI logs.
func requireLiveEqual(t *testing.T, field string, want, got interface{}) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("live field %s did not match", field)
	}
}

func redactedLiveMismatch(field string) string {
	return fmt.Sprintf("live field %s did not match", field)
}
