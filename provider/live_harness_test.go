package dokploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
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
	kind             string
	released         bool
	holdForFollowUp  bool
	releaseRequested bool
}

type liveCleanupOwner struct {
	mu      sync.Mutex
	owned   bool
	cleanup func()
}

func newLiveCleanupOwner(cleanup func()) *liveCleanupOwner {
	return &liveCleanupOwner{owned: true, cleanup: cleanup}
}

func releaseAfterVerifiedCleanup(owner *liveCleanupOwner, verify func() error) error {
	err := verify()
	if err == nil {
		owner.release()
	}
	return err
}

func (o *liveCleanupOwner) release() {
	o.mu.Lock()
	o.owned = false
	o.mu.Unlock()
}

func (o *liveCleanupOwner) cleanupOnce() {
	o.mu.Lock()
	if !o.owned {
		o.mu.Unlock()
		return
	}
	o.owned = false
	cleanup := o.cleanup
	o.mu.Unlock()
	cleanup()
}

func deleteAndVerifyLiveOwned(ctx context.Context, remove func() error, read func() (string, error), release func()) error {
	if err := remove(); err != nil && !client.IsNotFound(err) {
		return err
	}
	err := waitForDatabaseAbsence(ctx, func(context.Context) (string, error) { return read() })
	if err == nil {
		release()
	}
	return err
}

func beginLiveHeavyOperation(t *testing.T, kind string, probes ...func(context.Context) error) *liveHeavyOperationLease {
	t.Helper()
	if !heavyLiveTierAvailable() {
		t.Skip("live heavy tier stopped after cleanup failure")
	}
	var probe func(context.Context) error
	if len(probes) > 0 {
		probe = probes[0]
	}
	lease, err := acquireLiveHeavyOperation(t.Context(), kind, probe)
	if err != nil {
		recordServerHealthFailure(kind, err)
		t.Skip("live acceptance stopped after server health failure")
	}
	return lease
}

// acquireLiveHeavyOperation keeps the serialization mutex held during the
// health probe. A failed probe clears the reservation before returning, so a
// later tier can never inherit a stale heavy-operation owner.
func acquireLiveHeavyOperation(ctx context.Context, kind string, probe func(context.Context) error) (*liveHeavyOperationLease, error) {
	liveHeavyOperation.Lock()
	defer liveHeavyOperation.Unlock()
	if liveHeavyOperation.kind != "" {
		return nil, fmt.Errorf("heavy live operation %q is already active", liveHeavyOperation.kind)
	}
	liveHeavyOperation.kind = kind
	if err := maybeVerifyLiveServerHealth(ctx, probe); err != nil {
		liveHeavyOperation.kind = ""
		return nil, err
	}
	return &liveHeavyOperationLease{kind: kind}, nil
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
	if lease.holdForFollowUp {
		lease.releaseRequested = true
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
func handleLiveHeavyCreateError(t *testing.T, lease *liveHeavyOperationLease, id string, createErr error, cleanup func(), probes ...func(context.Context) error) {
	t.Helper()
	if err := processLiveHeavyCreateError(t, lease, id, createErr, cleanup, probes...); err != nil {
		requireNoError(t, err)
	}
}

func processLiveHeavyCreateError(t *testing.T, lease *liveHeavyOperationLease, id string, createErr error, cleanup func(), probes ...func(context.Context) error) error {
	return processLiveHeavyOperationError(t, lease, createErr, func() {
		cleanupLiveHeavyCreateFailure(t, lease, id, createErr, cleanup)
	}, probes...)
}

// processLiveHeavyOperationError keeps the lease through partial cleanup and
// any gated timeout probe. It is also used for updates/redeploys, where there
// is no partial resource cleanup callback.
func processLiveHeavyOperationError(t *testing.T, lease *liveHeavyOperationLease, operationErr error, cleanup func(), probes ...func(context.Context) error) error {
	t.Helper()
	if operationErr == nil {
		return nil
	}
	lease.holdForFollowUp = true
	if cleanup != nil {
		cleanup()
	}
	if classifyLiveServerHealthFailure(operationErr) {
		recordServerHealthFailure(lease.kind, errLiveServerHealthProbe)
	} else if errors.Is(operationErr, context.DeadlineExceeded) {
		for _, probe := range probes {
			if probeErr := maybeVerifyLiveServerHealth(t.Context(), probe); probeErr != nil {
				recordServerHealthFailure(lease.kind, probeErr)
				break
			}
		}
	}
	lease.holdForFollowUp = false
	lease.releaseIfNeeded(t)
	return operationErr
}

func cleanupLiveHeavyCreateFailure(t *testing.T, lease *liveHeavyOperationLease, id string, createErr error, cleanup func()) {
	t.Helper()
	if createErr == nil {
		return
	}
	if id != "" && cleanup != nil {
		cleanup()
	}
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

type liveDomainCreateResult struct {
	path           string
	target         string
	classification string
	keys           []string
	reason         string
	created        bool
}

type focusedDomainTarget struct {
	id      string
	name    string
	compose bool
}

func runFocusedDomainTarget(t *testing.T, create func() (focusedDomainTarget, func()), run func(focusedDomainTarget)) {
	t.Helper()
	target, cleanup := create()
	t.Cleanup(cleanup)
	run(target)
}

func classifyDomainComparison(provider, generated liveDomainCreateResult) string {
	if provider.target != generated.target || provider.target == "" || !slices.Equal(provider.keys, generated.keys) {
		return "provider-serialization-mismatch"
	}
	if strings.Contains(provider.classification, "status=transport") || strings.Contains(provider.classification, "status=5xx") || strings.Contains(generated.classification, "status=transport") || strings.Contains(generated.classification, "status=5xx") {
		return "environment-or-health-failure"
	}
	if provider.created != generated.created {
		return "provider-serialization-mismatch"
	}
	if provider.classification == generated.classification {
		return "server-contract-rejection"
	}
	return "provider-serialization-mismatch"
}

func sanitizeDomainValidationReason(err error) string {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.Message == "" {
		return "category=unknown"
	}
	fields := []string{"host", "path", "port", "customEntrypoint", "https", "applicationId", "certificateType", "customCertResolver", "composeId", "serviceName", "domainType", "previewDeploymentId", "internalPath", "stripPath", "middlewares", "forwardAuthEnabled"}
	field := ""
	message := strings.ToLower(apiErr.Message)
	for _, candidate := range fields {
		if strings.Contains(message, strings.ToLower(candidate)) {
			field = candidate
			break
		}
	}
	category := "validation"
	switch {
	case strings.Contains(message, "missing") || strings.Contains(message, "required"):
		category = "missing-field"
	case strings.Contains(message, "invalid"):
		category = "invalid-value"
	case strings.Contains(message, "unsupported") || strings.Contains(message, "unknown"):
		category = "unsupported"
	}
	if field == "" && category == "validation" {
		return "category=unknown"
	}
	if field == "" {
		return "category=" + category
	}
	return "field=" + field + ";category=" + category
}

func allowlistedDomainReason(reason string) (string, bool) {
	if reason == "" {
		return "", true
	}
	parts := strings.Split(reason, ";")
	if len(parts) == 1 && strings.HasPrefix(parts[0], "category=") {
		if isSafeDomainReasonCategory(strings.TrimPrefix(parts[0], "category=")) {
			return parts[0], true
		}
		return "", false
	}
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "field=") || !strings.HasPrefix(parts[1], "category=") {
		return "", false
	}
	if !isSafeDomainReasonField(strings.TrimPrefix(parts[0], "field=")) || !isSafeDomainReasonCategory(strings.TrimPrefix(parts[1], "category=")) {
		return "", false
	}
	return parts[0] + ";" + parts[1], true
}

func isSafeDomainReasonField(field string) bool {
	switch field {
	case "host", "path", "port", "customEntrypoint", "https", "applicationId", "certificateType", "customCertResolver", "composeId", "serviceName", "domainType", "previewDeploymentId", "internalPath", "stripPath", "middlewares", "forwardAuthEnabled":
		return true
	default:
		return false
	}
}

func isSafeDomainReasonCategory(category string) bool {
	switch category {
	case "unknown", "missing-field", "invalid-value", "unsupported", "validation":
		return true
	default:
		return false
	}
}

func serializedDomainFieldDiff(baseline, variant map[string]any) []string {
	seen := make(map[string]struct{}, len(baseline)+len(variant))
	for key := range baseline {
		seen[key] = struct{}{}
	}
	for key := range variant {
		seen[key] = struct{}{}
	}
	changed := make([]string, 0, len(seen))
	for key := range seen {
		baselineValue, _ := json.Marshal(baseline[key])
		variantValue, _ := json.Marshal(variant[key])
		if !bytes.Equal(baselineValue, variantValue) {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}

func domainResultStatus(classification string) (string, string) {
	parts := strings.Split(classification, ";")
	if (len(parts) != 3 && len(parts) != 5) || parts[0] != "operation=domain" || !strings.HasPrefix(parts[1], "status=") || !strings.HasPrefix(parts[2], "code=") {
		return "invalid", "invalid"
	}
	if len(parts) == 5 && (!strings.HasPrefix(parts[3], "keys=") || !strings.HasPrefix(parts[4], "target=")) {
		return "invalid", "invalid"
	}
	status := strings.TrimPrefix(parts[1], "status=")
	code := strings.TrimPrefix(parts[2], "code=")
	if status != "transport" && status != "2xx" && status != "4xx" && status != "5xx" {
		return "invalid", "invalid"
	}
	if !isSafeWorkloadAPICode(code) && code != "unknown" {
		return "invalid", "invalid"
	}
	return status, code
}

func formatDomainComparisonEvidence(provider, generated liveDomainCreateResult) string {
	if provider.path != "provider" || generated.path != "generated" {
		return "invalid-evidence"
	}
	if provider.target != generated.target || (provider.target != "application" && provider.target != "compose") {
		return "invalid-evidence"
	}
	providerStatus, providerCode := domainResultStatus(provider.classification)
	generatedStatus, generatedCode := domainResultStatus(generated.classification)
	if providerStatus == "invalid" || generatedStatus == "invalid" {
		return "invalid-evidence"
	}
	keys := append([]string(nil), provider.keys...)
	generatedKeys := append([]string(nil), generated.keys...)
	sort.Strings(keys)
	sort.Strings(generatedKeys)
	for i, key := range keys {
		if !isSafeWorkloadRequestKey(key) || (i > 0 && keys[i-1] == key) {
			return "invalid-evidence"
		}
	}
	for i, key := range generatedKeys {
		if !isSafeWorkloadRequestKey(key) || (i > 0 && generatedKeys[i-1] == key) {
			return "invalid-evidence"
		}
	}
	if !slices.Equal(keys, generatedKeys) {
		return fmt.Sprintf(
			"target=%s;provider=%s/%s;generated=%s/%s;providerKeys=%s;generatedKeys=%s",
			provider.target, providerStatus, providerCode,
			generatedStatus, generatedCode,
			strings.Join(keys, ","), strings.Join(generatedKeys, ","),
		)
	}
	providerReason, providerReasonOK := allowlistedDomainReason(provider.reason)
	generatedReason, generatedReasonOK := allowlistedDomainReason(generated.reason)
	if !providerReasonOK || !generatedReasonOK {
		return "invalid-evidence"
	}
	if providerReason != generatedReason {
		return fmt.Sprintf(
			"target=%s;provider=%s/%s;generated=%s/%s;keys=%s;providerReason=%s;generatedReason=%s",
			provider.target, providerStatus, providerCode,
			generatedStatus, generatedCode, strings.Join(keys, ","), providerReason, generatedReason,
		)
	}
	return fmt.Sprintf(
		"target=%s;provider=%s/%s;generated=%s/%s;keys=%s;providerReason=%s;generatedReason=%s",
		provider.target, providerStatus, providerCode,
		generatedStatus, generatedCode, strings.Join(keys, ","), providerReason, generatedReason,
	)
}

func compareDomainAttempts(providerAttempt, generatedAttempt func() liveDomainCreateResult) string {
	providerResult := providerAttempt()
	generatedResult := generatedAttempt()
	return classifyDomainComparison(providerResult, generatedResult)
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
	ctx, cancel := cleanupContext()
	defer cancel()
	if err := waitForDatabaseAbsence(ctx, func(context.Context) (string, error) { return read() }); err != nil {
		return err
	}
	markUnowned()
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

func registerLiveCleanup(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) func() {
	return registerLiveCleanupOwner(t, kind, id, remove, read).release
}

func registerLiveCleanupOwner(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) *liveCleanupOwner {
	t.Helper()
	if id == "" {
		return newLiveCleanupOwner(func() {})
	}
	owner := newLiveCleanupOwner(func() { liveCleanupVerified(t, kind, id, remove, read) })
	t.Cleanup(owner.cleanupOnce)
	return owner
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
	return recordStructuralCleanup(kind, err)
}

// recordLiveResult stores only sanitized diagnostics. heavyFailure must be true
// only for cleanup or server-health failures; ordinary test failures must not
// stop later tiers.
func recordLiveResult(kind string, diagnostic interface{}) {
	recordResult(liveResult{kind: kind, diagnostic: sanitizeLiveDiagnostic(toDiagnostic(diagnostic))})
}
func recordCleanupResult(kind string, diagnostic interface{}) string {
	var err error
	if candidate, ok := diagnostic.(error); ok {
		err = candidate
	}
	return recordStructuralCleanup(kind, err)
}

func recordStructuralCleanup(kind string, err error) string {
	liveHeavyStop.Store(true)
	message := structuralLiveDiagnostic("cleanup", kind, err)
	if markerErr := createLiveStopMarker(); markerErr != nil {
		message += "; marker=write-failed"
	}
	sanitized := sanitizeLiveDiagnostic(message)
	recordResult(liveResult{kind: kind, diagnostic: sanitized, heavyFailure: true})
	return sanitized
}
func recordServerHealthFailure(kind string, diagnostic interface{}) string {
	liveHeavyStop.Store(true)
	return recordHeavyFailure(kind, diagnostic)
}

var errLiveServerHealthProbe = errors.New("live server health probe failed")

func classifyLiveServerHealthFailure(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == 502 || apiErr.StatusCode == 503 || apiErr.StatusCode == 504 {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(apiErr.Code)) {
	case "SERVICE_UNAVAILABLE", "SERVER_UNHEALTHY", "CAPACITY_EXHAUSTED":
		return true
	default:
		return false
	}
}

// verifyLiveServerHealth deliberately returns a fixed diagnostic. Probe
// implementations must not expose response bodies, endpoint values, or IDs.
func verifyLiveServerHealth(parent context.Context, probe func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if err := probe(ctx); err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) {
			// A request reaching the API and being rejected for its deliberately
			// invalid probe input proves that the server is responsive.
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && !classifyLiveServerHealthFailure(err) {
				return nil
			}
		} else if isLiveDecodeError(err) {
			return nil
		}
		return fmt.Errorf("%w: %s", errLiveServerHealthProbe, healthProbeFailureClass(err))
	}
	return nil
}

func maybeVerifyLiveServerHealth(ctx context.Context, probe func(context.Context) error) error {
	if probe == nil || !liveAcceptanceEnabled() {
		return nil
	}
	return verifyLiveServerHealth(ctx, probe)
}

func isLiveDecodeError(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var invalidErr *json.InvalidUnmarshalError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || errors.As(err, &invalidErr)
}

func healthProbeFailureClass(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if classifyLiveServerHealthFailure(err) {
		return "unavailable"
	}
	return "transport"
}

func liveServerHealthProbe(api *client.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		response, err := api.ProjectOneWithResponse(ctx, &generated.ProjectOneParams{ProjectId: ""})
		if err != nil {
			return err
		}
		if response == nil || response.HTTPResponse == nil {
			return errLiveServerHealthProbe
		}
		status := response.HTTPResponse.StatusCode
		if status >= 200 && status < 500 {
			return nil
		}
		return &client.APIError{StatusCode: status, Code: "SERVER_UNHEALTHY"}
	}
}

// createLiveStopMarker publishes only a fixed, non-secret sentinel. O_EXCL
// makes the first writer win across test processes without exposing diagnostics
// or credentials to the next workflow step.
func recordHeavyFailure(kind string, diagnostic interface{}) string {
	var err error
	if candidate, ok := diagnostic.(error); ok {
		err = candidate
	}
	message := structuralLiveDiagnostic("heavy", kind, err)
	if err := createLiveStopMarker(); err != nil {
		message += "; marker=write-failed"
	}
	sanitized := sanitizeLiveDiagnostic(message)
	recordResult(liveResult{kind: kind, diagnostic: sanitized, heavyFailure: true})
	return sanitized
}

func structuralLiveDiagnostic(operation, kind string, err error) string {
	resource := "unknown"
	switch kind {
	case "application", "compose", "domain", "mount", "project", "environment", "destination", "ssh-key", "registry", "tag", "project-tag", "postgres", "mysql", "mariadb", "mongodb", "redis", "backup", "backup-postgres", "backup-mysql", "backup-mariadb", "backup-mongodb":
		resource = kind
	}
	statusClass, code := "transport", "unknown"
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode >= 200 && apiErr.StatusCode < 300:
			statusClass = "2xx"
		case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500:
			statusClass = "4xx"
		case apiErr.StatusCode >= 500 && apiErr.StatusCode < 600:
			statusClass = "5xx"
		}
		if isSafeWorkloadAPICode(apiErr.Code) || isSafeHealthCode(apiErr.Code) {
			code = apiErr.Code
		}
	} else if errors.Is(err, context.DeadlineExceeded) {
		statusClass = "timeout"
	}
	return fmt.Sprintf("operation=%s;resource=%s;status=%s;code=%s", operation, resource, statusClass, code)
}

func isSafeHealthCode(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "SERVICE_UNAVAILABLE", "SERVER_UNHEALTHY", "CAPACITY_EXHAUSTED":
		return true
	default:
		return false
	}
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

func requireLiveContains(t *testing.T, field string, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("live field %s did not contain expected content", field)
	}
}

func redactedLiveMismatch(field string) string {
	return fmt.Sprintf("live field %s did not match", field)
}
