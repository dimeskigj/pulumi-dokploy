package dokploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/google/uuid"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestLiveGateRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	require.False(t, liveAcceptanceEnabled())
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	require.True(t, liveAcceptanceEnabled())
}

func TestLiveGateRequiresBothCredentials(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	for _, name := range []string{"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
			t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
			t.Setenv(name, "")
			require.False(t, liveAcceptanceEnabled())
		})
	}
}

func TestClassifyWorkloadCreateAttempt(t *testing.T) {
	tests := []struct {
		name          string
		operation     string
		status        int
		code          string
		keys          []string
		targetPresent bool
		targetReady   bool
		want          string
	}{
		{
			name:      "ready domain with sorted keys",
			operation: "domain", status: 201, code: "VALIDATION_ERROR", keys: []string{"serviceName", "applicationId"},
			targetPresent: true, targetReady: true,
			want: "operation=domain;status=2xx;code=VALIDATION_ERROR;keys=applicationId,serviceName;target=ready",
		},
		{
			name:      "missing target is safe",
			operation: "mount", status: 404, code: "NOT_FOUND", keys: []string{"applicationId"},
			want: "operation=mount;status=4xx;code=NOT_FOUND;keys=applicationId;target=missing",
		},
		{
			name:      "unsafe metadata is omitted",
			operation: "mount", status: 503, code: "bad code; DROP TABLE secrets", keys: []string{"host.example/id", "apiKey", "composeId"},
			targetPresent: true,
			want:          "operation=mount;status=5xx;code=unknown;keys=composeId;target=present-not-ready",
		},
		{
			name:      "transport has no server metadata",
			operation: "domain", status: 0, code: "SAFECODE", keys: []string{"api-key-secret"},
			want: "operation=domain;status=transport;code=unknown;keys=none;target=missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classifyWorkloadCreateAttempt(tt.operation, tt.status, tt.code, tt.keys, tt.targetPresent, tt.targetReady)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			for _, sentinel := range []string{"application-id-sentinel", "host.example", "/id", "DROP TABLE", "SECRET"} {
				require.NotContains(t, got, sentinel)
			}
		})
	}
}

func TestClassifyWorkloadCreateAttemptRejectsUnknownOperationAndSentinels(t *testing.T) {
	_, err := classifyWorkloadCreateAttempt("application", 400, "SECRET123", []string{"applicationId", "abc123", "serviceId"}, true, false)
	require.Error(t, err)

	got, err := classifyWorkloadCreateAttempt("mount", 400, "SECRET123", []string{"mountPath", "SECRET123", "abc123", "serviceId"}, true, false)
	require.NoError(t, err)
	require.Equal(t, "operation=mount;status=4xx;code=unknown;keys=mountPath,serviceId;target=present-not-ready", got)
	for _, sentinel := range []string{"SECRET123", "abc123"} {
		require.NotContains(t, got, sentinel)
	}
}

func TestLiveRunNameUsesKindAndUUID(t *testing.T) {
	name := liveRunName("application")
	parts := strings.Split(name, "-")
	require.Equal(t, "pulumi-acceptance-application", strings.Join(parts[:3], "-"))
	_, err := uuid.Parse(strings.Join(parts[3:], "-"))
	require.NoError(t, err)
}

func TestCleanupContextHasFiniteFiveMinuteDeadline(t *testing.T) {
	ctx, cancel := cleanupContext()
	defer cancel()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), deadline, 2*time.Second)
}

func TestLiveCleanupPassesBoundedContext(t *testing.T) {
	var got context.Context
	liveCleanupVerified(t, "project", "id", func(ctx context.Context) error {
		got = ctx
		return nil
	}, func(context.Context) (string, error) { return "", nil })
	deadline, ok := got.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), deadline, 2*time.Second)
}

func TestLiveCleanupCancelsContextAfterDelete(t *testing.T) {
	var got context.Context
	liveCleanupVerified(t, "project", "id", func(ctx context.Context) error {
		got = ctx
		return nil
	}, func(context.Context) (string, error) { return "", nil })
	require.ErrorIs(t, got.Err(), context.Canceled)
}

func TestLiveResourceCleanupHasNoLegacyUnboundedPath(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "live_test.go"))
	require.NoError(t, err)
	require.NotContains(t, string(source), "boundedCleanupContext")
}

func TestLiveCleanupToleratesAlreadyAbsentResource(t *testing.T) {
	liveCleanupVerified(t, "project", "id", func(context.Context) error {
		return &client.APIError{StatusCode: 404, Message: "already absent"}
	}, func(context.Context) (string, error) { return "", nil })
}

func TestVerifiedCleanupPollsDelayedAbsenceAndCancelsTimer(t *testing.T) {
	oldInterval := liveCleanupPollInterval
	liveCleanupPollInterval = time.Millisecond
	t.Cleanup(func() { liveCleanupPollInterval = oldInterval })
	reads := 0
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := verifyLiveCleanup(ctx, func(context.Context) error { return nil }, func(context.Context) (string, error) {
		reads++
		if reads < 3 {
			return "still-present", nil
		}
		return "", nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, reads)
}

func TestVerifiedCleanupTimeoutRecordsStopPropagationForPartialCreate(t *testing.T) {
	oldInterval := liveCleanupPollInterval
	liveCleanupPollInterval = time.Millisecond
	t.Cleanup(func() { liveCleanupPollInterval = oldInterval })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Millisecond)
	defer cancel()
	err := verifyLiveCleanup(ctx, func(context.Context) error { return nil }, func(context.Context) (string, error) {
		return "partial-resource", nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	recordLiveCleanupFailure("partial-resource", "partial-id", err)
	require.True(t, heavyLiveTierStopped())
}

func TestSanitizeLiveDiagnostic(t *testing.T) {
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	t.Setenv("DOKPLOY_ENDPOINT", "https://endpoint-sentinel.invalid")
	t.Setenv("DOKPLOY_REGISTRY_USERNAME", "registry-user-sentinel")
	t.Setenv("DOKPLOY_REGISTRY_PASSWORD", "registry-password-sentinel")
	t.Setenv("DOKPLOY_GITLAB_USERNAME", "gitlab-user-sentinel")
	t.Setenv("DOKPLOY_GITLAB_TOKEN", "gitlab-token-sentinel")
	resetLiveSecretRegistry()
	t.Cleanup(resetLiveSecretRegistry)
	unregister := registerLiveSecrets("ssh-private-sentinel", "ssh-public-sentinel", "mount-file-sentinel", "db-password-sentinel")
	t.Cleanup(unregister)
	message := strings.Join([]string{"failed", "secret-sentinel", "https://endpoint-sentinel.invalid", "registry-user-sentinel", "registry-password-sentinel", "gitlab-user-sentinel", "gitlab-token-sentinel", "ssh-private-sentinel", "ssh-public-sentinel", "mount-file-sentinel", "db-password-sentinel"}, ":")
	got := sanitizeLiveDiagnostic(message)
	for _, secret := range strings.Split(message, ":") {
		if secret != "failed" {
			require.NotContains(t, got, secret)
		}
	}
}

func TestLiveSecretRegistryIsSafeForConcurrentRegistrationAndSnapshots(t *testing.T) {
	resetLiveSecretRegistry()
	t.Cleanup(resetLiveSecretRegistry)
	const workers = 20
	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			unregister := registerLiveSecrets("concurrent-secret-" + strconv.Itoa(i))
			defer unregister()
			_ = sanitizeLiveDiagnostic("concurrent-secret-" + strconv.Itoa(i))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < workers; i++ {
		<-done
	}
}

func TestCleanupHelperProcessRunsRegisteredCleanupAfterFailNow(t *testing.T) {
	if os.Getenv("LIVE_CLEANUP_HELPER") == "1" {
		t.Helper()
		marker := os.Getenv("LIVE_CLEANUP_MARKER")
		register := func(kind string) {
			registerLiveCleanup(t, kind, kind+"-id", func(ctx context.Context) error {
				deadline, bounded := ctx.Deadline()
				file, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				defer file.Close()
				_, err = file.WriteString(kind + " delete bounded=" + strconv.FormatBool(bounded && time.Until(deadline) > 0) + "\n")
				return err
			}, func(context.Context) (string, error) {
				file, err := os.OpenFile(marker, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return "", err
				}
				defer file.Close()
				_, err = file.WriteString(kind + " absence checked\n")
				return "", err
			})
		}
		register("domain")
		register("mount")
		t.FailNow()
	}

	marker := filepath.Join(t.TempDir(), "cleanup-marker")
	// The test binary is the fixed, trusted helper executable for this process.
	cmd := exec.Command(os.Args[0], "-test.run=^TestCleanupHelperProcessRunsRegisteredCleanupAfterFailNow$") //nolint:gosec
	cmd.Env = append(os.Environ(), "LIVE_CLEANUP_HELPER=1", "LIVE_CLEANUP_MARKER="+marker)
	require.Error(t, cmd.Run())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	for _, kind := range []string{"domain", "mount"} {
		require.Contains(t, string(contents), kind+" delete bounded=true")
		require.Contains(t, string(contents), kind+" absence checked")
	}
}

func TestSanitizedLiveErrorKeepsCallerContext(t *testing.T) {
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	diagnostic := sanitizedLiveError(&client.APIError{StatusCode: 500, Message: "secret-sentinel"}, "creating project", "request id")
	require.Contains(t, diagnostic, "creating project")
	require.Contains(t, diagnostic, "request id")
	require.NotContains(t, diagnostic, "secret-sentinel")
}

func TestRedactedLiveMismatchExcludesSecretSentinel(t *testing.T) {
	message := redactedLiveMismatch("application.buildSecrets")
	require.NotContains(t, message, "secret-sentinel")
	require.NotContains(t, message, "private-key-sentinel")
	require.Contains(t, message, "application.buildSecrets")
}

func TestMountDiffCoversTypeTargetCartesianMatrix(t *testing.T) {
	types := []string{mountTypeBind, mountTypeVolume, mountTypeFile}
	targets := []string{"applicationId", "composeId", "postgresId", "mysqlId", "mariadbId", "redisId"}
	for _, mountType := range types {
		for _, target := range targets {
			inputs := MountArgs{Type: mountType, MountPath: "/mnt/test"}
			state := MountState{MountArgs: inputs}
			setMountTarget(&inputs, target, "new-target")
			response, err := (Mount{}).Diff(t.Context(), infer.DiffRequest[MountArgs, MountState]{Inputs: inputs, State: state})
			require.NoError(t, err)
			require.Equal(t, p.UpdateReplace, response.DetailedDiff[target].Kind, "%s/%s", mountType, target)
		}
	}
	// Live redeployment of this Cartesian product is intentionally excluded:
	// every mount causes a workload redeploy and the acceptance server is
	// explicitly low-power. The live tier still dispatches all six targets and
	// fully exercises bind/volume/file representatives.
}

func TestBackupDestinationSkipRequiresTypedConnectivityValidation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"connection validation", &client.APIError{Operation: "destination.create", StatusCode: 400, Code: "DESTINATION_CONNECTION_FAILED"}, true},
		{"connection message", &client.APIError{Operation: "destination.create", StatusCode: 422, Code: "BAD_REQUEST", Message: "failed to connect to object storage"}, true},
		{"provider error", &client.APIError{Operation: "destination.create", StatusCode: 400, Code: "VALIDATION_ERROR", Message: "invalid provider"}, false},
		{"other endpoint", &client.APIError{Operation: "backup.create", StatusCode: 400, Code: "DESTINATION_CONNECTION_FAILED"}, false},
		{"decode error", context.Canceled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isExternalDestinationConnectivityValidation(tt.err))
		})
	}
}

func TestSanitizeLiveDiagnosticRedactsBackupDestinationCredentials(t *testing.T) {
	message := "access=AKIALIVETEST secret=live-test-secret"
	got := sanitizeLiveDiagnostic(message)
	require.NotContains(t, got, "AKIALIVETEST")
	require.NotContains(t, got, "live-test-secret")
}

func TestLiveCleanupFailureRecordsSanitizedDiagnosticAndStopsTier(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	diagnostic := recordLiveCleanupFailure("project", "id", &client.APIError{StatusCode: 500, Message: "secret-sentinel"})
	require.True(t, heavyLiveTierStopped())
	liveResultStore.Lock()
	defer liveResultStore.Unlock()
	require.Len(t, liveResultStore.results, 1)
	require.NotContains(t, liveResultStore.results[0].diagnostic, "secret-sentinel")
	require.NotContains(t, sanitizeLiveDiagnostic(diagnostic), "secret-sentinel")
}

func TestCleanupFailureStopsLaterHeavyTier(t *testing.T) {
	state := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(state) })
	recordCleanupResult("project", "cleanup failed")
	require.True(t, heavyLiveTierStopped())
	require.False(t, heavyLiveTierAvailable())
}

func TestPartialCleanupFailureRecordsStopAndBlocksHeavyAcquisition(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	recordLiveCleanupFailure("partial-resource", "partial-id", &client.APIError{StatusCode: 500, Message: "cleanup failed"})
	require.True(t, heavyLiveTierStopped())
	require.False(t, heavyLiveTierAvailable())
	liveResultStore.Lock()
	defer liveResultStore.Unlock()
	require.Len(t, liveResultStore.results, 1)
	require.True(t, liveResultStore.results[0].heavyFailure)
}

func TestOrdinaryLiveResultDoesNotStopHeavyTier(t *testing.T) {
	state := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(state) })
	recordLiveResult("project", "ordinary failure")
	require.False(t, heavyLiveTierStopped())
}

func TestServerHealthFailureStopsHeavyTier(t *testing.T) {
	state := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(state) })
	recordServerHealthFailure("server", "unhealthy")
	require.True(t, heavyLiveTierStopped())
}

func TestCleanupFailureCreatesNonSecretStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop-marker")
	t.Setenv("DOKPLOY_ACCEPTANCE_STOP_FILE", marker)
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")

	recordCleanupResult("project", "cleanup failed: secret-sentinel")

	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
	require.NotContains(t, string(contents), "secret-sentinel")
}

func TestServerHealthFailureCreatesNonSecretStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop-marker")
	t.Setenv("DOKPLOY_ACCEPTANCE_STOP_FILE", marker)

	recordServerHealthFailure("server", "unhealthy")

	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestOrdinaryLiveResultDoesNotCreateStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop-marker")
	t.Setenv("DOKPLOY_ACCEPTANCE_STOP_FILE", marker)

	recordLiveResult("project", "ordinary failure")

	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStopMarkerWriteFailureIsRecordedAsSafeFailure(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "missing", "stop-marker")
	t.Setenv("DOKPLOY_ACCEPTANCE_STOP_FILE", marker)

	diagnostic := recordCleanupResult("project", "cleanup failed")

	require.True(t, heavyLiveTierStopped())
	require.Contains(t, diagnostic, "stop marker propagation failed")
	require.Contains(t, diagnostic, "open stop marker")
	liveResultStore.Lock()
	defer liveResultStore.Unlock()
	require.Contains(t, liveResultStore.results[0].diagnostic, "stop marker propagation failed")
}

func TestHarnessStateSurvivesTestCleanup(t *testing.T) {
	original := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(original) })
	resetLiveHarnessState()
	recordLiveResult("prior", "prior result")
	state := snapshotLiveHarnessState()
	t.Run("isolated", func(t *testing.T) {
		inner := snapshotLiveHarnessState()
		t.Cleanup(func() { restoreLiveHarnessState(inner) })
		recordCleanupResult("inner", "stop")
	})
	require.Equal(t, state.results, snapshotLiveHarnessState().results)
	restoreLiveHarnessState(state)
}

func TestHeavyOperationReleaseReportsOwnershipMismatch(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	beginLiveHeavyOperation(t, "postgres")
	require.False(t, endLiveHeavyOperation("mysql"))
	require.True(t, endLiveHeavyOperation("postgres"))
	require.False(t, endLiveHeavyOperation("postgres"))
}

func TestResetLiveHarnessStateClearsHeavyOperation(t *testing.T) {
	resetLiveHarnessState()
	beginLiveHeavyOperation(t, "postgres")
	resetLiveHarnessState()
	beginLiveHeavyOperation(t, "redis")
	require.True(t, endLiveHeavyOperation("redis"))
}

func TestHeavyOperationCreateFailureReleasesAndCleansOwnership(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	lease := beginLiveHeavyOperation(t, "postgres")
	cleanupLiveHeavyCreateFailure(t, lease, "", context.Canceled, nil)
	second := beginLiveHeavyOperation(t, "redis")
	require.True(t, second.release(t))

	lease = beginLiveHeavyOperation(t, "postgres")
	cleaned := false
	cleanupLiveHeavyCreateFailure(t, lease, "partial-id", context.Canceled, func() { cleaned = true })
	require.True(t, cleaned)
	second = beginLiveHeavyOperation(t, "redis")
	require.True(t, second.release(t))
}
