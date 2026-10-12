package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestHeavyCreateUnconfirmedIdentityStopsUnlessDefinitelyRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		stop bool
	}{
		{"bad request", &client.APIError{StatusCode: 400}, false},
		{"unauthorized", &client.APIError{StatusCode: 401}, false},
		{"not found", &client.APIError{StatusCode: 404}, false},
		{"wrapped validation", errors.Join(errors.New("private-message"), &client.APIError{StatusCode: 422}), false},
		{"rate limited", &client.APIError{StatusCode: 429}, true},
		{"server error", &client.APIError{StatusCode: 500}, true},
		{"canceled", context.Canceled, true},
		{"timeout", context.DeadlineExceeded, true},
		{"decode", &json.SyntaxError{}, true},
		{"success without ID", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetLiveHarnessState()
			t.Cleanup(resetLiveHarnessState)
			t.Setenv("DOKPLOY_ACCEPTANCE", "0")
			marker := filepath.Join(t.TempDir(), "stop")
			t.Setenv(liveStopMarkerEnvironment, marker)
			lease := beginLiveHeavyOperation(t, "postgres")
			err := processLiveHeavyCreateError(t, lease, "", tc.err, func() {
				t.Fatal("unconfirmed identity must not be deleted or discovered")
			})
			require.Error(t, err)
			require.True(t, lease.released, "failed create must release its heavy lease")
			require.Equal(t, tc.stop, heavyLiveTierStopped())
			ran := false
			runLiveCase(t, "independent operation", func(*testing.T) { ran = true })
			require.Equal(t, !tc.stop, ran)
			if tc.stop {
				contents, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Equal(t, "stop\n", string(contents))
				liveHeavyStop.Store(false)
				require.True(t, heavyLiveTierStopped(), "later processes/reruns must honor the marker")
				require.True(t, snapshotLiveHarnessState().results[0].heavyFailure)
			} else {
				_, err := os.Stat(marker)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestHeavyCreateAcknowledgedIdentityWithoutCleanupStops(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	t.Setenv(liveStopMarkerEnvironment, filepath.Join(t.TempDir(), "stop"))
	lease := beginLiveHeavyOperation(t, "application")
	require.ErrorIs(t, processLiveHeavyCreateError(t, lease, "owned-id", context.Canceled, nil), context.Canceled)
	require.True(t, lease.released)
	require.True(t, heavyLiveTierStopped())
}

func TestHeavyCreateAcknowledgedIdentityVerifiedCleanupRemainsOrdinary(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	lease := beginLiveHeavyOperation(t, "application")
	var events []string
	owner := registerLiveCleanupOwner(t, "application", "owned-id", func(context.Context) error {
		events = append(events, "delete")
		return nil
	}, func(context.Context) (string, error) {
		events = append(events, "verified absent")
		return "", nil
	})
	require.ErrorIs(t, processLiveHeavyCreateError(t, lease, "owned-id", context.Canceled, owner.cleanupOnce), context.Canceled)
	require.True(t, lease.released)
	require.Equal(t, []string{"delete", "verified absent"}, events)
	owner.cleanupOnce()
	require.Len(t, events, 2, "fallback cleanup must not delete twice")
	require.False(t, heavyLiveTierStopped())
	ran := false
	runLiveCase(t, "independent operation", func(*testing.T) { ran = true })
	require.True(t, ran)
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLiveProjectAcknowledgedFailureHelperProcess(t *testing.T) {
	if os.Getenv("LIVE_PROJECT_ACKNOWLEDGED_HELPER") == "1" {
		api, err := client.New(os.Getenv("LIVE_PROJECT_ENDPOINT"), "offline-key")
		require.NoError(t, err)
		// Exercise the same finalizer with an acknowledged partial-create result;
		// Project.Create currently cannot produce an ID alongside an error.
		finalizeLiveProjectCreate(t, api, infer.CreateResponse[ProjectState]{ID: "owned-project", Output: ProjectState{DefaultEnvironmentID: "owned-env"}}, context.Canceled)
		t.Fatal("failed create must never return a fixture")
	}
	for _, cleanupFails := range []bool{false, true} {
		name := "verified partial cleanup"
		if cleanupFails {
			name = "failed partial cleanup retains fallback ownership"
		}
		t.Run(name, func(t *testing.T) {
			requests := []scriptedRequest{}
			if cleanupFails {
				requests = append(requests, scriptedRequest{Method: http.MethodPost, Path: "/api/project.remove", Body: json.RawMessage(`{"projectId":"owned-project"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"BAD_REQUEST","message":"private-message"}`)})
			}
			requests = append(requests,
				scriptedRequest{Method: http.MethodPost, Path: "/api/project.remove", Body: json.RawMessage(`{"projectId":"owned-project"}`), Status: http.StatusOK, Response: []byte(`{}`)},
				scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: map[string][]string{"projectId": {"owned-project"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
			)
			server := newScriptedServer(t, requests...)
			marker := filepath.Join(t.TempDir(), "stop")
			t.Setenv(liveStopMarkerEnvironment, marker)
			cmd := exec.Command(os.Args[0], "-test.run=^TestLiveProjectAcknowledgedFailureHelperProcess$") //nolint:gosec // Trusted test binary.
			cmd.Env = append(os.Environ(), "DOKPLOY_ACCEPTANCE=0", "LIVE_PROJECT_ACKNOWLEDGED_HELPER=1", "LIVE_PROJECT_ENDPOINT="+server.server.URL)
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			require.NotContains(t, string(output), "private-message")
			require.NotContains(t, string(output), "owned-project")
			require.NotContains(t, string(output), "failed create must never return a fixture")
			_, markerErr := os.Stat(marker)
			if cleanupFails {
				require.NoError(t, markerErr)
			} else {
				require.ErrorIs(t, markerErr, os.ErrNotExist)
			}
		})
	}
}

// The child calls the real fixture, including Fatal and registered fallback
// cleanup. Only HTTP is faked, so no acceptance opt-in or live server is needed.
func TestLiveProjectOwnershipHelperProcess(t *testing.T) {
	if os.Getenv("LIVE_PROJECT_OWNERSHIP_HELPER") == "1" {
		api, err := client.New(os.Getenv("LIVE_PROJECT_ENDPOINT"), "offline-key")
		require.NoError(t, err)
		id, env, _ := liveProject(t, t.Context(), api)
		require.NoError(t, os.WriteFile(os.Getenv("LIVE_PROJECT_RETURNED"), []byte("returned\n"), 0600))
		require.Equal(t, "owned-project", id)
		require.Equal(t, "owned-env", env)
		return
	}
	for _, tc := range []struct {
		name, response string
		status         int
		stop, cleanup  bool
		success        bool
	}{
		{"missing ID success", `{"project":{"projectId":""},"environment":{"environmentId":"owned-env"}}`, 200, true, false, false},
		{"empty response", `{}`, 200, true, false, false},
		{"decode failure", `{`, 200, true, false, false},
		{"server error", `{"code":"INTERNAL_ERROR"}`, 500, true, false, false},
		{"rate limited", `{"code":"BAD_REQUEST"}`, 429, true, false, false},
		{"definite rejection", `{"code":"BAD_REQUEST"}`, 400, false, false, false},
		{"missing environment acknowledged project", `{"project":{"projectId":"owned-project"},"environment":{"environmentId":""}}`, 200, false, true, false},
		{"incomplete response loses identity", `{"project":{"projectId":"owned-project"},"environment":{}}`, 200, true, false, false},
		{"confirmed fixture", `{"project":{"projectId":"owned-project"},"environment":{"environmentId":"owned-env"}}`, 200, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/project.create":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.response))
				case "/api/project.remove":
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]string{"projectId": "owned-project"}, body)
					_, _ = w.Write([]byte(`{}`))
				case "/api/project.one":
					require.Equal(t, "owned-project", r.URL.Query().Get("projectId"))
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"code":"NOT_FOUND"}`))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			marker := filepath.Join(t.TempDir(), "stop")
			returned := filepath.Join(filepath.Dir(marker), "returned")
			t.Setenv(liveStopMarkerEnvironment, marker)
			cmd := exec.Command(os.Args[0], "-test.run=^TestLiveProjectOwnershipHelperProcess$") //nolint:gosec // Trusted test binary.
			cmd.Env = append(os.Environ(), "DOKPLOY_ACCEPTANCE=0", "LIVE_PROJECT_OWNERSHIP_HELPER=1", "LIVE_PROJECT_ENDPOINT="+server.URL, "LIVE_PROJECT_RETURNED="+returned)
			output, err := cmd.CombinedOutput()
			if tc.success {
				require.NoError(t, err, "%s", output)
			} else {
				require.Error(t, err, "fixture must fail rather than return an unconfirmed identity")
				_, returnedErr := os.Stat(returned)
				require.ErrorIs(t, returnedErr, os.ErrNotExist, "fixture must fail before returning")
			}
			_, markerErr := os.Stat(marker)
			if tc.stop {
				require.NoError(t, markerErr, "%s", output)
			} else {
				require.ErrorIs(t, markerErr, os.ErrNotExist)
			}
			mu.Lock()
			defer mu.Unlock()
			want := []string{"/api/project.create"}
			if tc.cleanup {
				want = append(want, "/api/project.remove", "/api/project.one")
			}
			require.Equal(t, want, paths, "no retries, discovery, or guessed-ID cleanup")
		})
	}
}
