package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
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

func TestLiveTargetReadinessUsesConcreteResourceReader(t *testing.T) {
	cases := []struct {
		name, path, queryKey, id, response string
		makeRead                           func(*client.Client, string) liveTargetReadiness
	}{
		{"application", "/api/application.one", "applicationId", "a1", `{"applicationId":"a1","applicationStatus":"done","type":"docker","image":"test/image"}`, applicationTargetReadiness},
		{"compose", "/api/compose.one", "composeId", "c1", `{"composeId":"c1","composeStatus":"done","type":"raw"}`, composeTargetReadiness},
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

func TestClassifyDomainComparison(t *testing.T) {
	keys := []string{"applicationId", "domainType", "host"}
	tests := []struct {
		name                string
		provider, generated liveDomainCreateResult
		want                string
	}{
		{"provider mismatch", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=2xx;code=unknown", keys: keys, created: true}, "provider-serialization-mismatch"},
		{"server rejection", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, "server-contract-rejection"},
		{"environment failure", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=transport;code=unknown", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, "environment-or-health-failure"},
		{"provider created only", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys, created: true}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, "provider-serialization-mismatch"},
		{"generated created only", liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys}, liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys, created: true}, "provider-serialization-mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyDomainComparison(tt.provider, tt.generated)
			require.Equal(t, tt.want, got)
			for _, sentinel := range []string{"secret-sentinel", "resource-id-sentinel", "private.example", "https://"} {
				require.NotContains(t, got, sentinel)
			}
		})
	}

	t.Run("sentinel-bearing metadata has fixed output", func(t *testing.T) {
		provider := liveDomainCreateResult{path: "secret-sentinel", target: "application", classification: "operation=domain;status=4xx;code=secret-sentinel", keys: keys}
		generated := liveDomainCreateResult{path: "https://private.example", target: "application", classification: provider.classification, keys: keys}
		got := classifyDomainComparison(provider, generated)
		require.Equal(t, "server-contract-rejection", got)
		for _, sentinel := range []string{"secret-sentinel", "resource-id-sentinel", "private.example", "https://"} {
			require.NotContains(t, got, sentinel)
		}
	})

	t.Run("sentinel target and key have fixed output", func(t *testing.T) {
		provider := liveDomainCreateResult{path: "provider", target: "resource-id-sentinel", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: append(keys, "secret-sentinel")}
		generated := liveDomainCreateResult{path: "generated", target: "application", classification: provider.classification, keys: keys}
		got := classifyDomainComparison(provider, generated)
		require.Equal(t, "provider-serialization-mismatch", got)
		for _, sentinel := range []string{"secret-sentinel", "resource-id-sentinel", "private.example", "https://"} {
			require.NotContains(t, got, sentinel)
		}
	})
}

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

func TestLiveDomainHostFixturesUseValidRFC1123Labels(t *testing.T) {
	for _, kind := range []string{"domain", "domain-direct", "updated-domain", "custom-domain", "focused-domain", "focused-domain-direct", "focused-domain-updated", "experiment-domain", "experiment-domain-direct", "experiment-domain-updated"} {
		host := liveDomainHost(kind)
		for _, label := range strings.Split(host, ".") {
			require.NotEmpty(t, label)
			require.LessOrEqual(t, len(label), 63)
			require.Regexp(t, `^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`, label)
		}
	}
}

func TestLiveDomainHostReplacesOverlongLegacyRunName(t *testing.T) {
	legacyLabel := strings.Split(liveRunName("focused-domain"), ".")[0]
	newLabel := strings.Split(liveDomainHost("focused-domain"), ".")[0]
	require.Greater(t, len(legacyLabel), 63)
	require.LessOrEqual(t, len(newLabel), 63)
}

func TestDomainComparisonEvidenceReportsSafeKeyShapeMismatch(t *testing.T) {
	providerKeys := []string{"applicationId", "domainType", "host"}
	generatedKeys := []string{"applicationId", "host"}
	got := formatDomainComparisonEvidence(
		liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: providerKeys},
		liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=2xx;code=unknown", keys: generatedKeys, created: true},
	)
	require.Contains(t, got, "target=application")
	require.Contains(t, got, "providerKeys=applicationId,domainType,host")
	require.Contains(t, got, "generatedKeys=applicationId,host")
	require.NotContains(t, got, "secret-sentinel")
	require.NotContains(t, got, "https://")
}

func TestDomainComparisonEvidenceRejectsMaliciousReasons(t *testing.T) {
	keys := []string{"applicationId", "certificateType", "domainType", "host", "https", "stripPath"}
	got := formatDomainComparisonEvidence(
		liveDomainCreateResult{path: "provider", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys, reason: "field=domainType;category=invalid-value;message=https://private.example secret-sentinel"},
		liveDomainCreateResult{path: "generated", target: "application", classification: "operation=domain;status=4xx;code=BAD_REQUEST", keys: keys, reason: "field=domainType;category=invalid-value"},
	)
	require.Equal(t, "invalid-evidence", got)
	require.NotContains(t, got, "private.example")
	require.NotContains(t, got, "secret-sentinel")
}

func TestDomainContractExperimentsChangeOneNamedField(t *testing.T) {
	applicationID := "application"
	cases := domainContractExperimentCases(DomainArgs{ApplicationID: &applicationID, Host: "example.invalid", Port: intPtr(80), CertificateType: CertificateNone}, false)
	got := make([]string, 0, len(cases))
	for _, experiment := range cases {
		got = append(got, experiment.field)
	}
	require.Equal(t, []string{"domainType", "port", "certificateType", "https", "stripPath"}, got)
}

func TestDomainContractExperimentsSerializeExactlyOneFieldDifference(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    DomainArgs
		compose bool
	}{
		{name: "application", args: DomainArgs{ApplicationID: stringPtr("application"), Host: "example.invalid", Port: intPtr(80), CertificateType: CertificateNone}},
		{name: "compose", args: DomainArgs{ComposeID: stringPtr("compose"), ServiceName: stringPtr("web"), Host: "example.invalid", Port: intPtr(80), CertificateType: CertificateNone}, compose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline := jsonObject(t, domainCreateBody(tc.args))
			for _, experiment := range domainContractExperimentCases(tc.args, tc.compose) {
				variant := jsonObject(t, experiment.body)
				changed := serializedDomainFieldDiff(baseline, variant)
				require.Equal(t, []string{experiment.field}, changed)
			}
		})
	}
}

func TestDomainContractExperimentFailsClosedWithoutID(t *testing.T) {
	category, continueExperiments := classifyDomainExperimentResult("2xx", false)
	require.Equal(t, "cleanup-failure", category)
	require.False(t, continueExperiments)
}

func TestDomainContractExperimentSequenceStopsAfterBaselineFailure(t *testing.T) {
	calls := []string{}
	runDomainContractExperimentSequence(t, generated.DomainCreateJSONRequestBody{}, []domainContractExperiment{{field: "domainType"}}, func(field string, _ generated.DomainCreateJSONRequestBody) domainExperimentResult {
		calls = append(calls, field)
		return domainExperimentResult{field: field, category: "cleanup-failure"}
	})
	require.Equal(t, []string{"baseline"}, calls)
	calls = nil
	runDomainContractExperimentSequence(t, generated.DomainCreateJSONRequestBody{}, []domainContractExperiment{{field: "domainType"}, {field: "port"}}, func(field string, _ generated.DomainCreateJSONRequestBody) domainExperimentResult {
		calls = append(calls, field)
		if field == "domainType" {
			return domainExperimentResult{field: field, category: "cleanup-failure"}
		}
		return domainExperimentResult{field: field, category: "rejected"}
	})
	require.Equal(t, []string{"baseline", "domainType"}, calls)
}

func TestDomainContractExperimentTargetsStopWhenMarkerIsSet(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	calls := []string{}
	runDomainContractExperimentTargets(t, []string{"application", "compose"}, func(name string) {
		calls = append(calls, name)
		liveHeavyStop.Store(true)
	})
	require.Equal(t, []string{"application"}, calls)
}

func TestDomainExperimentTransportDoesNotCreateHealthStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "domain.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)

	recorded := recordDomainExperimentHealthFailure(t.Context(), func(context.Context) error {
		return nil
	})

	require.False(t, recorded)
	require.False(t, heavyLiveTierStopped())
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestDomainExperimentUsesIndependentHealthContext(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "domain.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	operationCtx, cancel := context.WithCancel(t.Context())
	cancel()
	probeCalled := false

	recorded := recordDomainExperimentHealthFailure(operationCtx, func(ctx context.Context) error {
		probeCalled = true
		require.NoError(t, ctx.Err())
		return nil
	})

	require.True(t, probeCalled)
	require.False(t, recorded)
	require.False(t, heavyLiveTierStopped())
}

func TestDomainExperimentFailedHealthProbeCreatesStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "domain.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)

	recorded := recordDomainExperimentHealthFailure(t.Context(), func(context.Context) error {
		return &client.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE"}
	})

	require.True(t, recorded)
	require.True(t, heavyLiveTierStopped())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestDomainCreateComparisonRunsOnlyAfterPartialCleanup(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	var events []string
	runDomainComparisonAfterCleanup(func() {
		events = append(events, "cleanup")
	}, func() bool { return heavyLiveTierStopped() }, func() {
		events = append(events, "generated-create")
	})
	require.Equal(t, []string{"cleanup", "generated-create"}, events)

	events = nil
	runDomainComparisonAfterCleanup(func() {
		events = append(events, "cleanup")
		liveHeavyStop.Store(true)
	}, func() bool { return heavyLiveTierStopped() }, func() {
		events = append(events, "generated-create")
	})
	require.Equal(t, []string{"cleanup"}, events)
}

func TestSanitizeDomainValidationReasonAllowListsFieldAndCategory(t *testing.T) {
	reason := sanitizeDomainValidationReason(&client.APIError{Message: "domainType has an invalid value secret-sentinel"})
	require.Equal(t, "field=domainType;category=invalid-value", reason)
	require.NotContains(t, reason, "secret-sentinel")
}

func TestRunFocusedDomainTargetOwnsAndCleansTarget(t *testing.T) {
	var events []string
	t.Run("focused target", func(t *testing.T) {
		runFocusedDomainTarget(t, func() (focusedDomainTarget, func()) {
			events = append(events, "create")
			return focusedDomainTarget{id: "target", name: "application"}, func() {
				events = append(events, "cleanup")
			}
		}, func(target focusedDomainTarget) {
			events = append(events, "run:"+target.name+":"+target.id)
		})
		require.Equal(t, []string{"create", "run:application:target"}, events)
	})
	require.Equal(t, []string{"create", "run:application:target", "cleanup"}, events)
}

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

func TestGeneratedDomainCreateResultRegistersPartialIDBeforeErrorClassification(t *testing.T) {
	keys := []string{"applicationId", "domainType", "host"}
	id := "partial-domain-id"
	response := &generated.DomainCreateResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
		JSON200:      &generated.Domain{DomainId: &id},
	}
	registered := ""
	result := finalizeGeneratedDomainCreateAttempt(t, "application", keys, response, &client.APIError{StatusCode: http.StatusBadRequest, Code: "BAD_REQUEST"}, func(got string) {
		registered = got
	})
	require.NotEmpty(t, registered)
	require.True(t, result.created)
	require.Contains(t, result.classification, "status=4xx")
}

func TestGeneratedDomainCreateResultUsesTypedErrorStatusAndRunsVerifiedCleanup(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/domain.delete", Body: json.RawMessage(`{"domainId":"partial-domain-id"}`), Status: http.StatusOK, Response: []byte(`{}`)},
		scriptedRequest{Method: http.MethodGet, Path: "/api/domain.one", Query: map[string][]string{"domainId": {"partial-domain-id"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	keys := []string{"applicationId", "domainType", "host"}
	id := "partial-domain-id"
	response := &generated.DomainCreateResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
		JSON200:      &generated.Domain{DomainId: &id},
	}
	result := finalizeGeneratedDomainCreateAttempt(t, "application", keys, response, &client.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE"}, func(id string) {
		registerGeneratedDomainCleanup(t, s.API(), id)
	})
	require.True(t, result.created)
	require.Contains(t, result.classification, "status=4xx")

	transportResult := finalizeGeneratedDomainCreateAttempt(t, "application", keys, nil, &client.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE"}, func(string) {})
	require.Contains(t, transportResult.classification, "status=5xx")
}

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

func TestTargetReadErrorWithHealthyProbeRemainsOrdinary(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	readErr := errors.New("read sentinel")
	_, err := validateLiveTargetWithHealthProbe(t.Context(), "mount", []string{"postgresId"}, func(context.Context) (bool, bool, error) {
		return false, false, readErr
	}, func(context.Context) error {
		return nil
	})
	require.ErrorIs(t, err, readErr)
	require.False(t, heavyLiveTierStopped())
}

func TestTargetReadErrorStopsOnlyAfterFailedHealthProbe(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	readErr := errors.New("read sentinel")
	probeErr := &client.APIError{StatusCode: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE"}
	_, err := validateLiveTargetWithHealthProbe(t.Context(), "mount", []string{"postgresId"}, func(context.Context) (bool, bool, error) {
		return false, false, readErr
	}, func(context.Context) error {
		return probeErr
	})
	require.ErrorIs(t, err, readErr)
	require.True(t, heavyLiveTierStopped())
	_, err = os.Stat(marker)
	require.NoError(t, err)
}

func TestLiveRunNameUsesKindAndUUID(t *testing.T) {
	name := liveRunName("application")
	parts := strings.Split(name, "-")
	require.Equal(t, "pulumi-acceptance-application", strings.Join(parts[:3], "-"))
	_, err := uuid.Parse(strings.Join(parts[3:], "-"))
	require.NoError(t, err)
}

func TestClassifyWorkloadCreateErrorPropagatesUnknownOperation(t *testing.T) {
	classification, err := classifyWorkloadCreateError("unknown-operation", nil, nil, false, false)
	require.Error(t, err)
	require.Empty(t, classification)
}

func TestClassifyMountDispatchPhase(t *testing.T) {
	secret := "mount-dispatch-secret-sentinel"
	for _, test := range []struct {
		name  string
		phase string
		err   error
		want  string
	}{
		{name: "transport", phase: "target-read", err: errors.New("transport"), want: "operation=mount-dispatch;phase=target-read;status=transport;code=unknown"},
		{name: "fixture-delete", phase: "fixture-delete", err: errLiveCleanup, want: "operation=mount-dispatch;phase=fixture-delete;status=failed;code=unknown"},
		{name: "api", phase: "mount-create", err: &client.APIError{StatusCode: 400, Code: "BAD_REQUEST", Message: secret}, want: "operation=mount-dispatch;phase=mount-create;status=4xx;code=BAD_REQUEST"},
		{name: "timeout", phase: "health-probe", err: context.DeadlineExceeded, want: "operation=mount-dispatch;phase=health-probe;status=timeout;code=unknown"},
		{name: "unknown-phase", phase: "unexpected", err: errors.New(secret), want: "operation=mount-dispatch;phase=unknown;status=transport;code=unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := classifyMountDispatchPhase(test.phase, test.err)
			require.Equal(t, test.want, got)
			require.NotContains(t, got, secret)
		})
	}
}

func TestClassifyMountDispatchPhaseIncludesSafeMountUpdateStep(t *testing.T) {
	secret := "private-host-payload-response-sentinel"
	for _, tc := range []struct {
		name          string
		phase, status string
		cause         error
		want          string
	}{
		{"transport", "update", mountUpdateStatusFailed, errors.New(secret), "operation=mount-dispatch;phase=mount-update;status=transport;code=unknown;step=update;stepStatus=failed"},
		{"HTTP status preserved", "readback", mountUpdateStatusFailed, &client.APIError{StatusCode: http.StatusBadRequest, Code: "BAD_REQUEST", Message: secret}, "operation=mount-dispatch;phase=mount-update;status=4xx;code=BAD_REQUEST;step=readback;stepStatus=failed"},
		{"timeout preserved", "redeploy", mountUpdateStatusTimeout, fmt.Errorf("wrapped: %w", syntheticDispatchTimeout{}), "operation=mount-dispatch;phase=mount-update;status=timeout;code=unknown;step=redeploy;stepStatus=timeout"},
		{"unallowlisted step omitted", secret, mountUpdateStatusFailed, errors.New(secret), "operation=mount-dispatch;phase=mount-update;status=transport;code=unknown"},
		{"unallowlisted status omitted", "target", secret, errors.New(secret), "operation=mount-dispatch;phase=mount-update;status=transport;code=unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyMountDispatchPhase("mount-update", &mountUpdateFailure{phase: tc.phase, status: tc.status, cause: tc.cause})
			require.Equal(t, tc.want, got)
			require.NotContains(t, got, secret)
		})
	}
}

type syntheticDispatchTimeout struct{}

func (syntheticDispatchTimeout) Error() string   { return "timeout host sentinel" }
func (syntheticDispatchTimeout) Timeout() bool   { return true }
func (syntheticDispatchTimeout) Temporary() bool { return true }

func TestMountDispatchHealthProbeEvidence(t *testing.T) {
	require.Equal(t, "operation=mount-dispatch;phase=health-probe;status=timeout;code=unknown",
		classifyMountDispatchHealthProbe(context.DeadlineExceeded))
}

func TestRecordMountDispatchHealthFailureStoresSanitizedMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "mount-dispatch.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	raw := "https://private.example/secret-id?token=secret-sentinel"
	err := &mountDispatchHealthProbeError{err: &client.APIError{StatusCode: 503, Code: "SERVER_UNHEALTHY", Message: raw}}

	diagnostic := recordMountDispatchHealthFailure(err)
	state := snapshotLiveHarnessState()
	require.True(t, state.stopped)
	require.Len(t, state.results, 1)
	require.Equal(t, "mount-dispatch", state.results[0].kind)
	require.Equal(t, diagnostic, state.results[0].diagnostic)
	require.Equal(t, "operation=mount-dispatch;phase=health-probe;status=5xx;code=SERVER_UNHEALTHY", diagnostic)
	require.NotContains(t, diagnostic, raw)
	require.NotContains(t, diagnostic, "secret-sentinel")
	contents, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	require.Equal(t, "stop\n", string(contents))
}

func TestMountDispatchPhaseAllowlistIsExact(t *testing.T) {
	require.Equal(t, map[string]struct{}{
		"fixture-create": {},
		"target-read":    {},
		"mount-create":   {},
		"mount-read":     {},
		"mount-update":   {},
		"mount-delete":   {},
		"fixture-delete": {},
		"health-probe":   {},
	}, mountDispatchPhases)
}

func TestMountDispatchPhaseEvidenceIsSanitized(t *testing.T) {
	secret := "mount-dispatch-secret-sentinel"
	got := classifyMountDispatchPhase("mount-create", &client.APIError{StatusCode: 500, Code: "INTERNAL_ERROR", Message: secret})
	require.Equal(t, "operation=mount-dispatch;phase=mount-create;status=5xx;code=unknown", got)
	require.NotContains(t, got, secret)
}

func TestDispatchFixtureLeaseTransitionsAroundDependentMount(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	fixtureLease := beginLiveHeavyOperation(t, "mount-dispatch-fixture")
	fixture := &liveDispatchFixture{lease: fixtureLease}
	fixture.releaseForDependentMount(t)

	mountLease := beginLiveHeavyOperation(t, "mount-create")
	order := []string{}
	mountAbsent := false
	mountOwner := newLiveCleanupOwner(func() {
		order = append(order, "mount-delete")
		mountAbsent = true
		require.True(t, mountLease.release(t))
	})
	fixtureOwner := newLiveCleanupOwner(func() {
		require.True(t, mountAbsent)
		cleanupLease := beginLiveHeavyOperation(t, "mount-dispatch-cleanup")
		require.True(t, cleanupLease.release(t))
		order = append(order, "fixture-delete")
	})
	mountOwner.cleanupOnce()
	fixtureOwner.cleanupOnce()
	require.Equal(t, []string{"mount-delete", "fixture-delete"}, order)
}

func TestDispatchMountFallbackCleanupHoldsLeaseThroughVerifiedAbsence(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	var events []string
	owner := registerDispatchMountCleanupOwner(t, nil, "mount-id", func(context.Context) error {
		liveHeavyOperation.Lock()
		kind := liveHeavyOperation.kind
		liveHeavyOperation.Unlock()
		require.Equal(t, "mount-dispatch-fallback-cleanup", kind)
		events = append(events, "delete")
		return nil
	}, func(context.Context) (string, error) {
		liveHeavyOperation.Lock()
		kind := liveHeavyOperation.kind
		liveHeavyOperation.Unlock()
		require.Equal(t, "mount-dispatch-fallback-cleanup", kind)
		events = append(events, "verify-absent")
		return "", nil
	})
	owner.cleanupOnce()
	require.Equal(t, []string{"delete", "verify-absent"}, events)
	lease := beginLiveHeavyOperation(t, "fixture-cleanup-after-mount")
	lease.releaseIfNeeded(t)
}

func TestDispatchMountFallbackCleanupRunsDespiteExistingStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	marker := filepath.Join(t.TempDir(), "incident.stop")
	require.NoError(t, os.WriteFile(marker, []byte("stop\n"), 0600))
	t.Setenv(liveStopMarkerEnvironment, marker)
	liveHeavyStop.Store(true)
	deleted, verified := false, false
	t.Run("cleanup owner runs", func(t *testing.T) {
		owner := registerDispatchMountCleanupOwner(t, nil, "mount-id", func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline)
			require.NoError(t, ctx.Err())
			deleted = true
			return nil
		}, func(ctx context.Context) (string, error) {
			require.NoError(t, ctx.Err())
			verified = true
			return "", nil
		})
		owner.cleanupOnce()
	})
	require.True(t, deleted)
	require.True(t, verified)
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestDispatchMountFallbackCleanupRunsAfterHealthProbeFailure(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	marker := filepath.Join(t.TempDir(), "incident.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	requests := make([]scriptedRequest, 5)
	for i := range requests {
		requests[i] = scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: map[string][]string{"projectId": {""}}, Status: http.StatusServiceUnavailable, Response: []byte(`{"code":"SERVICE_UNAVAILABLE"}`)}
	}
	server := newScriptedServer(t, requests...)
	deleted, verified := false, false
	t.Run("cleanup owner runs", func(t *testing.T) {
		owner := registerDispatchMountCleanupOwner(t, server.API(), "mount-id", func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline)
			require.NoError(t, ctx.Err())
			deleted = true
			return nil
		}, func(ctx context.Context) (string, error) {
			require.NoError(t, ctx.Err())
			verified = true
			return "", nil
		})
		owner.cleanupOnce()
	})
	require.True(t, deleted)
	require.True(t, verified)
	require.True(t, heavyLiveTierStopped())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestMountDispatchPhaseFailureReleasesLeaseBeforeClassification(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	lease := beginLiveHeavyOperation(t, "mount-dispatch-read")
	releaseMountDispatchLease(t, lease)
	diagnostic := classifyMountDispatchPhase("mount-read", errors.New("phase failure"))
	require.Contains(t, diagnostic, "phase=mount-read")
	lease = beginLiveHeavyOperation(t, "cleanup-after-phase-failure")
	lease.releaseIfNeeded(t)
}

func TestDispatchFixtureCleanupRunsDespiteExistingStopMarker(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	marker := filepath.Join(t.TempDir(), "incident.stop")
	require.NoError(t, os.WriteFile(marker, []byte("stop\n"), 0600))
	t.Setenv(liveStopMarkerEnvironment, marker)
	liveHeavyStop.Store(true)
	deleted, verified := false, false
	fixture := &liveDispatchFixture{
		id: "fixture-id", lease: &liveHeavyOperationLease{kind: "fixture-create", released: true},
		remove: func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline)
			deleted = true
			return nil
		},
		readID: func(ctx context.Context) (string, error) {
			require.NoError(t, ctx.Err())
			verified = true
			return "", nil
		},
	}

	t.Run("cleanup", func(t *testing.T) { fixture.cleanup(t) })

	require.True(t, deleted)
	require.True(t, verified)
	require.True(t, fixture.cleaned)
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
	lease, err := acquireLiveHeavyOperation(t.Context(), "after-fixture-cleanup", nil)
	require.NoError(t, err)
	require.True(t, lease.release(t))
}

func TestDispatchFixtureCleanupContinuesAfterHealthProbeFailure(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	marker := filepath.Join(t.TempDir(), "incident.stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	requests := make([]scriptedRequest, 5)
	for i := range requests {
		requests[i] = scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: map[string][]string{"projectId": {""}}, Status: http.StatusServiceUnavailable, Response: []byte(`{"code":"SERVICE_UNAVAILABLE"}`)}
	}
	server := newScriptedServer(t, requests...)
	deleted, verified := false, false
	fixture := &liveDispatchFixture{
		id: "fixture-id", api: server.API(), lease: &liveHeavyOperationLease{kind: "fixture-create", released: true},
		remove: func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline)
			deleted = true
			return nil
		},
		readID: func(ctx context.Context) (string, error) {
			require.NoError(t, ctx.Err())
			verified = true
			return "", nil
		},
	}

	t.Run("cleanup", func(t *testing.T) { fixture.cleanup(t) })

	require.True(t, deleted)
	require.True(t, verified)
	require.True(t, fixture.cleaned)
	require.True(t, heavyLiveTierStopped())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
	lease, err := acquireLiveHeavyOperation(t.Context(), "after-fixture-cleanup", nil)
	require.NoError(t, err)
	require.True(t, lease.release(t))
}

func TestClassifyWorkloadCreateErrorNeverIncludesRawServerDetails(t *testing.T) {
	err := &client.APIError{StatusCode: 400, Code: "BAD_REQUEST", Message: `insert into mount values ('mount-id-sentinel', 'target-id-sentinel')`}
	for _, test := range []struct {
		name      string
		operation string
	}{
		{name: "Domain/application", operation: "domain"},
		{name: "Domain/compose", operation: "domain"},
		{name: "Mounts/application", operation: "mount"},
		{name: "Mounts/compose", operation: "mount"},
		{name: "MountDispatch/compose", operation: "mount"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, classifyErr := classifyWorkloadCreateError(test.operation, err, []string{"mountPath", "composeId", "target-id-sentinel"}, true, true)
			require.NoError(t, classifyErr)
			require.Equal(t, "operation="+test.operation+";status=4xx;code=BAD_REQUEST;keys=composeId,mountPath;target=ready", got)
			require.NotContains(t, got, "insert into")
			require.NotContains(t, got, "mount-id-sentinel")
			require.NotContains(t, got, "target-id-sentinel")
		})
	}
}

func TestTier2WorkloadLifecycleDeletesTargetsOnlyAfterDependents(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "live_workloads_test.go"))
	require.NoError(t, err)
	text := string(source)
	ordered := []string{
		"workloadTargets :=",
		"t.Run(\"Domain/\"+target.name",
		"t.Run(\"Mounts/\"+target.name",
		"t.Run(\"MountDispatch/\"+target",
		"t.Run(\"SourceVariants\"",
		"deleteAndReadApplication(t, ctx",
		"deleteAndReadCompose(t, ctx",
	}
	previous := -1
	for _, marker := range ordered {
		position := strings.Index(text, marker)
		require.GreaterOrEqual(t, position, 0, "missing lifecycle marker %q", marker)
		require.Greater(t, position, previous, "lifecycle marker %q is out of order", marker)
		previous = position
	}
	require.Equal(t, 1, strings.Count(text, "deleteAndReadApplication(t, ctx"))
	require.Equal(t, 1, strings.Count(text, "deleteAndReadCompose(t, ctx"))
}

func TestSuccessfulCreateRegistersCleanupAndExplicitDeleteReleasesIt(t *testing.T) {
	deletes := 0
	owner := newLiveCleanupOwner(func() { deletes++ })
	owner.release()
	owner.cleanupOnce()
	require.Equal(t, 0, deletes)

	owner = newLiveCleanupOwner(func() { deletes++ })
	owner.cleanupOnce()
	owner.cleanupOnce()
	require.Equal(t, 1, deletes)
}

func TestExplicitDeleteRetainsOwnershipWhenAbsenceVerificationFails(t *testing.T) {
	fallbackCalls := 0
	owner := newLiveCleanupOwner(func() { fallbackCalls++ })
	deleteCalls := 0
	err := deleteAndVerifyLiveOwned(t.Context(), func() error {
		deleteCalls++
		return nil
	}, func() (string, error) { return "present", errors.New("verification sentinel") }, owner.release)
	require.Error(t, err)
	require.Equal(t, 1, deleteCalls)
	owner.cleanupOnce()
	require.Equal(t, 1, fallbackCalls)
}

func TestVerifiedProjectCleanupDisarmsFallbackOnlyAfterSuccess(t *testing.T) {
	deletes := 0
	owner := newLiveCleanupOwner(func() { deletes++ })
	require.Error(t, releaseAfterVerifiedCleanup(owner, func() error { return errors.New("absence not verified") }))
	owner.cleanupOnce()
	require.Equal(t, 1, deletes)
	owner = newLiveCleanupOwner(func() { deletes++ })
	require.NoError(t, releaseAfterVerifiedCleanup(owner, func() error { return nil }))
	owner.cleanupOnce()
	require.Equal(t, 1, deletes)
}

func TestWorkloadLifecycleDiagnosticsExcludeUnstructuredFailureDetails(t *testing.T) {
	err := &client.APIError{StatusCode: 500, Code: "INTERNAL_ERROR", Message: `update failed: sql=insert into mount values ('id-sentinel', '/host/path', 'secret-sentinel')`}
	for _, operation := range []string{"domain", "mount"} {
		for _, phase := range []string{"create", "read", "update", "delete"} {
			t.Run(operation+"/"+phase, func(t *testing.T) {
				got, classifyErr := classifyWorkloadLifecycleError(operation, err)
				require.NoError(t, classifyErr)
				require.Equal(t, "operation="+operation+";status=5xx;code=unknown", got)
				for _, sentinel := range []string{"insert into", "id-sentinel", "/host/path", "secret-sentinel", "INTERNAL_ERROR"} {
					require.NotContains(t, got, sentinel)
				}
			})
		}
	}
}

func TestWorkloadCallPathsEmitOnlyStructuralDiagnostics(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/domain.create", Body: json.RawMessage(`{"applicationId":"app","certificateType":"letsencrypt","domainType":"application","host":"example.test","https":false,"stripPath":false}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"BAD_REQUEST","message":"sql=insert domain-id-sentinel /raw/path"}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/mounts.create", Body: json.RawMessage(`{"content":null,"filePath":null,"hostPath":"/host","mountPath":"/mnt","serviceId":"app","serviceType":"application","type":"bind","volumeName":null}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"BAD_REQUEST","message":"content=mount-content-sentinel path=/raw/path"}`)},
	)
	domainErr := mustDomainCreateError(t, Domain{client: fixedClient(s.API())})
	mountErr := mustMountCreateError(t, Mount{client: fixedClient(s.API())})
	for _, test := range []struct {
		operation string
		err       error
	}{
		{"domain", domainErr}, {"mount", mountErr},
	} {
		got, err := classifyWorkloadLifecycleError(test.operation, test.err)
		require.NoError(t, err)
		require.Equal(t, "operation="+test.operation+";status=4xx;code=BAD_REQUEST", got)
		for _, sentinel := range []string{"domain-id-sentinel", "/raw/path", "mount-content-sentinel", "insert domain"} {
			require.NotContains(t, got, sentinel)
		}
	}
}

func mustDomainCreateError(t *testing.T, r Domain) error {
	t.Helper()
	_, err := r.Create(t.Context(), infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{ApplicationID: stringPtr("app"), Host: "example.test"}})
	require.Error(t, err)
	return err
}

func mustMountCreateError(t *testing.T, r Mount) error {
	t.Helper()
	_, err := r.Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: MountArgs{Type: mountTypeBind, MountPath: "/mnt", HostPath: stringPtr("/host"), ApplicationID: stringPtr("app")}})
	require.Error(t, err)
	return err
}

func TestDeleteAndVerifyOnceRetainsOwnershipUntilVerification(t *testing.T) {
	oldInterval := liveCleanupPollInterval
	liveCleanupPollInterval = time.Millisecond
	t.Cleanup(func() { liveCleanupPollInterval = oldInterval })
	deleteCalls, readCalls := 0, 0
	owned := true
	err := deleteAndVerifyOnce(func() error {
		deleteCalls++
		return nil
	}, func() (string, error) {
		readCalls++
		require.True(t, owned)
		return "still-present", errors.New("verification sentinel")
	}, func() { owned = false })
	require.Error(t, err)
	require.Equal(t, 1, deleteCalls)
	require.Equal(t, 1, readCalls)
	require.True(t, owned)
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

func TestRequireNoErrorUsesStructuralDiagnosticOnly(t *testing.T) {
	t.Setenv("DOKPLOY_API_KEY", "raw-api-key")
	got := structuralLiveError("Destination", "update", &client.APIError{StatusCode: 500, Message: "raw response body with resource-id"})
	require.Equal(t, "Destination update failed", got)
	require.NotContains(t, got, "raw response")
	require.NotContains(t, got, "resource-id")
}

func TestRedactedLiveMismatchExcludesSecretSentinel(t *testing.T) {
	message := redactedLiveMismatch("application.buildSecrets")
	require.NotContains(t, message, "secret-sentinel")
	require.NotContains(t, message, "private-key-sentinel")
	require.Contains(t, message, "application.buildSecrets")
}

func TestLiveLifecycleDiagnosticContainsOnlyStructure(t *testing.T) {
	message := liveLifecycleDiagnostic("Mount", "Diff", "mountPath", "alphanumeric-id SQL /etc/config secret-content")
	require.Equal(t, "Mount Diff failed for field mountPath", message)
	for _, sensitive := range []string{"alphanumeric-id", "SQL", "/etc/config", "secret-content"} {
		require.NotContains(t, message, sensitive)
	}
}

func TestLiveTargetReadMustPrecedeCreate(t *testing.T) {
	var order []string
	read := func() (string, error) {
		order = append(order, "read")
		return statusDone, nil
	}
	create := func() { order = append(order, "create") }
	require.NoError(t, readReadyMountTarget(read, create))
	require.Equal(t, []string{"read", "create"}, order)
}

func TestLiveDiffKindMissingFieldIsReportedWithoutPanic(t *testing.T) {
	kind, ok := liveDiffKind(map[string]p.PropertyDiff{}, "missing")
	require.False(t, ok)
	require.Zero(t, kind)
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

func TestClassifyLiveServerHealthFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"502", &client.APIError{StatusCode: 502, Code: "BAD_GATEWAY"}, true},
		{"503 safe code", &client.APIError{StatusCode: 503, Code: "SERVICE_UNAVAILABLE"}, true},
		{"504", &client.APIError{StatusCode: 504, Code: "TIMEOUT"}, true},
		{"capacity code", &client.APIError{StatusCode: 400, Code: "CAPACITY_EXHAUSTED"}, true},
		{"validation", &client.APIError{StatusCode: 400, Code: "VALIDATION_ERROR"}, false},
		{"not found", &client.APIError{StatusCode: 404, Code: "NOT_FOUND"}, false},
		{"decode", errors.New("invalid character in response"), false},
		{"ordinary timeout", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyLiveServerHealthFailure(tt.err))
		})
	}
}

func TestVerifyLiveServerHealthUsesBoundedContext(t *testing.T) {
	var got context.Context
	err := verifyLiveServerHealth(t.Context(), func(ctx context.Context) error {
		got = ctx
		return nil
	})
	require.NoError(t, err)
	_, ok := got.Deadline()
	require.True(t, ok)
}

func TestVerifyLiveServerHealthReturnsSanitizedFailure(t *testing.T) {
	err := verifyLiveServerHealth(t.Context(), func(context.Context) error {
		return &client.APIError{StatusCode: 503, Code: "SERVICE_UNAVAILABLE", Message: "raw-body-sentinel"}
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "raw-body-sentinel")
}

func TestHeavyOperationProbeRunsInsideSerializationBoundary(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	started := make(chan struct{})
	releaseProbe := make(chan struct{})
	firstDone := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		defer close(firstDone)
		lease, err := acquireLiveHeavyOperation(t.Context(), "first", func(context.Context) error {
			close(started)
			<-releaseProbe
			return nil
		})
		if err == nil && !lease.release(t) {
			err = errors.New("first lease release failed")
		}
		firstResult <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(releaseProbe)
		t.Fatal("first health probe did not start")
	}
	secondAttempting := make(chan struct{})
	secondProbeStarted := make(chan struct{})
	secondDone := make(chan struct{})
	secondResult := make(chan error, 1)
	go func() {
		defer close(secondDone)
		close(secondAttempting)
		lease, err := acquireLiveHeavyOperation(t.Context(), "second", func(context.Context) error {
			close(secondProbeStarted)
			return nil
		})
		if err == nil {
			if !lease.release(t) {
				err = errors.New("second lease release failed")
			}
		}
		secondResult <- err
	}()
	<-secondAttempting
	var overlapped bool
	select {
	case <-secondProbeStarted:
		overlapped = true
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseProbe)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first operation did not finish after releasing its probe")
	}
	require.NoError(t, <-firstResult)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second acquisition did not finish after first operation released")
	}
	secondErr := <-secondResult
	if secondErr != nil {
		require.ErrorContains(t, secondErr, `heavy live operation "first" is already active`)
		lease, err := acquireLiveHeavyOperation(t.Context(), "after-first-release", func(context.Context) error {
			close(secondProbeStarted)
			return nil
		})
		require.NoError(t, err)
		require.True(t, lease.release(t))
	}
	require.False(t, overlapped, "second health probe overlapped the first probe")
	select {
	case <-secondProbeStarted:
	default:
		t.Fatal("second health probe did not run after the first lease was released")
	}
}

func TestFailedHeavyOperationProbeReleasesOwnership(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	_, err := acquireLiveHeavyOperation(t.Context(), "failed", func(context.Context) error {
		return errLiveServerHealthProbe
	})
	require.Error(t, err)
	lease, err := acquireLiveHeavyOperation(t.Context(), "next", nil)
	require.NoError(t, err)
	require.True(t, lease.release(t))
}

func TestDisabledAcceptanceDoesNotInvokeHealthProbe(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	t.Setenv("DOKPLOY_ENDPOINT", "")
	t.Setenv("DOKPLOY_API_KEY", "")
	called := false
	require.NoError(t, maybeVerifyLiveServerHealth(t.Context(), func(context.Context) error {
		called = true
		return errLiveServerHealthProbe
	}))
	require.False(t, called)
}

func TestCreateTimeoutFollowUpProbeStaysUnderLease(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	lease := beginLiveHeavyOperation(t, "timeout")
	probeCalled := false
	cleanupCalled := false
	err := processLiveHeavyCreateError(t, lease, "partial", context.DeadlineExceeded, func() { cleanupCalled = true }, func(context.Context) error {
		probeCalled = true
		_, err := acquireLiveHeavyOperation(t.Context(), "overlap", nil)
		require.Error(t, err)
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, cleanupCalled)
	require.True(t, probeCalled)
	require.True(t, lease.release(t))
}

func TestCreateTimeoutHandlerSkipsProbeWhenAcceptanceDisabled(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	t.Setenv("DOKPLOY_ENDPOINT", "")
	t.Setenv("DOKPLOY_API_KEY", "")
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	lease := beginLiveHeavyOperation(t, "timeout")
	called := false
	err := processLiveHeavyCreateError(t, lease, "partial", context.DeadlineExceeded, func() {}, func(context.Context) error {
		called = true
		return errLiveServerHealthProbe
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
	require.True(t, lease.release(t))
}

func TestSuccessfulCreateRetainsHeavyLeaseUntilNormalRelease(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	lease := beginLiveHeavyOperation(t, "successful-create")
	require.NoError(t, processLiveHeavyCreateError(t, lease, "created-id", nil, func() {
		t.Fatal("successful create must not invoke error cleanup")
	}))
	_, err := acquireLiveHeavyOperation(t.Context(), "overlap", nil)
	require.Error(t, err, "successful create must retain its lease")
	require.True(t, lease.release(t))
	next, err := acquireLiveHeavyOperation(t.Context(), "next", nil)
	require.NoError(t, err)
	require.True(t, next.release(t))
}

func TestHeavyOperationUpdateClassificationStopsOnlyConfirmedHealthFailures(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	for _, test := range []struct {
		name string
		err  error
		stop bool
	}{
		{"validation", &client.APIError{StatusCode: 400, Code: "VALIDATION_ERROR"}, false},
		{"not-found", &client.APIError{StatusCode: 404, Code: "NOT_FOUND"}, false},
		{"unavailable", &client.APIError{StatusCode: 503, Code: "SERVICE_UNAVAILABLE"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetLiveHarnessState()
			lease := beginLiveHeavyOperation(t, "update")
			err := processLiveHeavyOperationError(t, lease, test.err, nil)
			require.Error(t, err)
			require.Equal(t, test.stop, heavyLiveTierStopped())
		})
	}
}

func TestHeavyOperationUpdateTimeoutProbesBeforeRelease(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "test-key")
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	lease := beginLiveHeavyOperation(t, "redeploy")
	probed := false
	err := processLiveHeavyOperationError(t, lease, context.DeadlineExceeded, nil, func(context.Context) error {
		probed = true
		_, overlapErr := acquireLiveHeavyOperation(t.Context(), "overlap", nil)
		require.Error(t, overlapErr)
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, probed)
	require.False(t, heavyLiveTierStopped())
	next, nextErr := acquireLiveHeavyOperation(t.Context(), "next", nil)
	require.NoError(t, nextErr)
	require.True(t, next.release(t))
}

func TestHeavyOperationCreateClassificationStopsOnlyHealthFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		stop bool
	}{
		{"validation", &client.APIError{StatusCode: 400, Code: "VALIDATION_ERROR"}, false},
		{"decode", errors.New("decode failure"), false},
		{"capacity", &client.APIError{StatusCode: 503, Code: "CAPACITY_EXHAUSTED"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetLiveHarnessState()
			lease := beginLiveHeavyOperation(t, "create")
			cleanupCalled := false
			err := processLiveHeavyCreateError(t, lease, "partial", test.err, func() { cleanupCalled = true })
			require.Error(t, err)
			require.True(t, cleanupCalled)
			require.Equal(t, test.stop, heavyLiveTierStopped())
		})
	}
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
	require.Contains(t, diagnostic, "operation=cleanup")
	require.Contains(t, diagnostic, "marker=write-failed")
	liveResultStore.Lock()
	defer liveResultStore.Unlock()
	require.Contains(t, liveResultStore.results[0].diagnostic, "marker=write-failed")
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
	require.True(t, lease.release(t))
	second := beginLiveHeavyOperation(t, "redis")
	require.True(t, second.release(t))

	lease = beginLiveHeavyOperation(t, "postgres")
	cleaned := false
	cleanupLiveHeavyCreateFailure(t, lease, "partial-id", context.Canceled, func() { cleaned = true })
	require.True(t, cleaned)
	require.True(t, lease.release(t))
	second = beginLiveHeavyOperation(t, "redis")
	require.True(t, second.release(t))
}
