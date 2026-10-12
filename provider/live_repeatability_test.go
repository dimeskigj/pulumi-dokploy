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
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

// Execute the actual live entry point in a subprocess, replacing only its HTTP
// boundary. A real acceptance endpoint/key can never enter this child process.
func runOfflineLiveEntry(t *testing.T, endpoint, pattern, marker string, count int) error {
	t.Helper()
	ctx := t.Context()
	// Only this test binary and caller-owned literal filters are executed.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+pattern, "-test.count="+strconv.Itoa(count), "-test.timeout=30s") //nolint:gosec
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "DOKPLOY_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "DOKPLOY_ACCEPTANCE=1", "DOKPLOY_ENDPOINT="+endpoint, "DOKPLOY_API_KEY=offline-key", "DOKPLOY_ACCEPTANCE_STOP_FILE="+marker)
	// Do not echo child output, which may include provider diagnostics.
	return cmd.Run()
}

func TestAcceptanceProjectTagRepeatedRunsLeaveNoOwnedFixtures(t *testing.T) {
	var mu sync.Mutex
	projects, tags := map[string]bool{}, map[string]bool{}
	names := map[string]bool{}
	var association bool
	var duplicateName bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{}
		if r.Method == http.MethodPost {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		}
		respond := func(v any) { require.NoError(t, json.NewEncoder(w).Encode(v)) }
		switch r.URL.Path {
		case "/api/project.create":
			name := body["name"].(string)
			duplicateName = duplicateName || names[name]
			names[name], projects["offline-project"] = true, true
			respond(map[string]any{"project": map[string]string{"projectId": "offline-project"}, "environment": map[string]string{"environmentId": "offline-env"}})
		case "/api/project.one":
			if !projects["offline-project"] {
				w.WriteHeader(http.StatusNotFound)
				respond(map[string]string{"message": "absent"})
				return
			}
			projectTags := []map[string]string{}
			if association {
				projectTags = append(projectTags, map[string]string{"tagId": "offline-tag"})
			}
			respond(map[string]any{"projectId": "offline-project", "name": "offline-project", "projectTags": projectTags})
		case "/api/project.remove":
			delete(projects, "offline-project")
			respond(map[string]bool{"success": true})
		case "/api/tag.create":
			name := body["name"].(string)
			duplicateName = duplicateName || names[name]
			names[name], tags["offline-tag"] = true, true
			respond(map[string]string{"tagId": "offline-tag"})
		case "/api/tag.one":
			if !tags["offline-tag"] {
				w.WriteHeader(http.StatusNotFound)
				respond(map[string]string{"message": "absent"})
				return
			}
			respond(map[string]string{"tagId": "offline-tag", "name": "offline-tag", "color": "#123456"})
		case "/api/tag.assignToProject":
			association = true
			respond(map[string]bool{"success": true})
		case "/api/tag.removeFromProject":
			association = false
			respond(map[string]bool{"success": true})
		case "/api/tag.remove":
			delete(tags, "offline-tag")
			respond(map[string]bool{"success": true})
		default:
			t.Error("unexpected offline acceptance route")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), "stop")
	require.NoError(t, runOfflineLiveEntry(t, server.URL, "^TestLiveTier1ControlPlane$/^ProjectTag$", marker, 3))
	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, projects, "project fixture must be removed on every run")
	require.Empty(t, tags, "association tag must be removed, not just disarmed")
	require.False(t, association)
	require.False(t, duplicateName)
	require.Len(t, names, 6)
}

func TestAcceptanceGateRespectsStopFromPreviousProcess(t *testing.T) {
	resetLiveHarnessState()
	t.Cleanup(resetLiveHarnessState)
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	require.NoError(t, os.WriteFile(marker, []byte("stop\n"), 0600))
	require.True(t, heavyLiveTierStopped(), "persistent safety stops survive new test processes")
}

func TestAcceptanceOrdinarySubtestFailureDoesNotBlockSibling(t *testing.T) {
	var mu sync.Mutex
	var tag map[string]any
	names := map[string]bool{}
	creates, deletes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{}
		if r.Method == http.MethodPost {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		}
		respond := func(v any) { require.NoError(t, json.NewEncoder(w).Encode(v)) }
		switch r.URL.Path {
		case "/api/project.create":
			w.WriteHeader(http.StatusBadRequest)
			respond(map[string]string{"message": "ordinary rejection"})
		case "/api/tag.create":
			creates++
			names[body["name"].(string)] = true
			tag = body
			tag["tagId"] = "offline-tag"
			respond(tag)
		case "/api/tag.one":
			if tag == nil {
				w.WriteHeader(http.StatusNotFound)
				respond(map[string]string{"message": "absent"})
			} else {
				respond(tag)
			}
		case "/api/tag.update":
			tag = body
			respond(tag)
		case "/api/tag.remove":
			deletes++
			tag = nil
			respond(map[string]bool{"success": true})
		default:
			t.Error("unexpected offline acceptance route")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), "stop")
	require.Error(t, runOfflineLiveEntry(t, server.URL, "^TestLiveTier1ControlPlane$/^(Project|Tag)$", marker, 3))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 3, creates, "a failed Project must not prevent the independent Tag segment")
	require.Equal(t, 3, deletes)
	require.Nil(t, tag)
	require.Len(t, names, 3, "every repeat must use a fresh owned name")
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "ordinary failure must not become a safety stop")
}

func TestAcceptanceUncertainCreateStopsLaterControlPlaneMutations(t *testing.T) {
	var mu sync.Mutex
	projectCreates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/project.create" {
			projectCreates++
			w.WriteHeader(http.StatusBadRequest)
		} else if r.URL.Path != "/api/server.create" {
			t.Error("unexpected offline acceptance route")
			w.WriteHeader(http.StatusBadRequest)
		}
		// A successful write without its identity cannot be cleaned safely.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{}))
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), "stop")
	require.Error(t, runOfflineLiveEntry(t, server.URL, "^TestLiveTier1ControlPlane$/^(Server|Project)$", marker, 1))
	mu.Lock()
	defer mu.Unlock()
	require.Zero(t, projectCreates, "safety stops must gate sibling mutations, not just later tiers")
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestAcceptanceUnacknowledgedWorkloadRequiresSafetyStop(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		stop bool
	}{
		{"definite rejection", &client.APIError{StatusCode: 400}, false},
		{"timed out", context.DeadlineExceeded, true},
		{"missing successful identity", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateLiveRecordHarness(t)
			cleanupAfterCreateError(t, "application", "", tc.err, func(context.Context) error {
				t.Fatal("must not delete an unconfirmed identity")
				return nil
			}, func(context.Context) (string, error) {
				t.Fatal("must not discover an unconfirmed identity")
				return "", nil
			})
			require.Equal(t, tc.stop, heavyLiveTierStopped())
		})
	}
}

func TestAcceptanceUnacknowledgedProjectStopsIndependentSibling(t *testing.T) {
	var mu sync.Mutex
	tagCreates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tag.create" {
			tagCreates++
			w.WriteHeader(http.StatusBadRequest)
		} else if r.URL.Path != "/api/project.create" {
			t.Error("unexpected offline acceptance route")
			w.WriteHeader(http.StatusBadRequest)
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{}))
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), "stop")
	require.Error(t, runOfflineLiveEntry(t, server.URL, "^TestLiveTier1ControlPlane$/^(Project|Tag)$", marker, 1))
	mu.Lock()
	defer mu.Unlock()
	require.Zero(t, tagCreates, "an unconfirmed project mutation must stop later siblings")
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
}

func TestControlPlaneCreateIdentityClassifiesOwnershipNotOrdinaryFailure(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		err      error
		stop     bool
	}{
		{"rejected", "", &client.APIError{StatusCode: 400}, false},
		{"acknowledged error", "offline-id", errors.New("read failed"), false},
		{"acknowledged success", "offline-id", nil, false},
		{"unacknowledged timeout", "", context.DeadlineExceeded, true},
		{"missing successful identity", "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateLiveRecordHarness(t)
			err := confirmLiveControlPlaneCreate("tag", tc.id, tc.err)
			if tc.id == "" || tc.err != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.stop, heavyLiveTierStopped())
		})
	}
}
