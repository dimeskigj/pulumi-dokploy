package dokploy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

// Only the network is scripted; the lifecycle and resource methods remain real.
type liveRecordHTTPFixture struct {
	mu                        sync.Mutex
	t                         *testing.T
	api                       *client.Client
	phase                     string
	rejectRoute               string
	rejectStatus              int
	portBadRequestAbsence     bool
	routes                    []string
	counts                    map[string]int
	names                     []string
	hosts                     []string
	server, application, port map[string]any
}

func newLiveRecordHTTPFixture(t *testing.T, phase string) *liveRecordHTTPFixture {
	t.Helper()
	f := &liveRecordHTTPFixture{t: t, phase: phase, counts: map[string]int{}}
	s := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "offline-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	f.api = api
	return f
}

func (f *liveRecordHTTPFixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	route := strings.TrimPrefix(r.URL.Path, "/api/")
	f.routes = append(f.routes, route)
	f.counts[route]++
	w.Header().Set("Content-Type", "application/json")
	b := map[string]any{}
	if r.Method == http.MethodPost {
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&b))
	} else {
		require.Equal(f.t, http.MethodGet, r.Method)
	}
	write := func(v any) { require.NoError(f.t, json.NewEncoder(w).Encode(v)) }
	fail := func() { w.WriteHeader(403); write(map[string]any{"message": "private-upstream-detail"}) }
	missing := func() { w.WriteHeader(404); write(map[string]any{"code": "NOT_FOUND"}) }
	if route == f.rejectRoute {
		w.WriteHeader(f.rejectStatus)
		write(map[string]any{"message": "private-upstream-detail"})
		return
	}
	if f.phase == "update-error" && (route == "server.update" || route == "port.update") {
		fail()
		return
	}
	if f.phase == "update-read-error" && (route == "server.one" || route == "port.one") && f.counts[route] == 3 {
		fail()
		return
	}
	if f.phase == "absence-read-error" && ((route == "server.one" && f.counts["server.remove"] == 1) || (route == "port.one" && f.counts["port.delete"] == 1)) {
		fail()
		return
	}
	switch route {
	case "server.create", "server.update":
		require.Equal(f.t, http.MethodPost, r.Method)
		require.Equal(f.t, false, b["enableDockerCleanup"])
		require.Nil(f.t, b["sshKeyId"])
		host, _ := b["ipAddress"].(string)
		require.True(f.t, strings.HasSuffix(host, ".invalid"))
		if route == "server.create" {
			require.Equal(f.t, float64(22), b["port"])
			require.Equal(f.t, "root", b["username"])
			require.Equal(f.t, "deploy", b["serverType"])
			f.names = append(f.names, b["name"].(string))
			f.hosts = append(f.hosts, host)
		} else {
			require.Equal(f.t, "offline-server", b["serverId"])
			require.Equal(f.t, float64(2222), b["port"])
			require.Equal(f.t, "acceptance", b["username"])
			require.Equal(f.t, "build", b["serverType"])
		}
		b["serverId"], b["organizationId"], b["serverStatus"] = "offline-server", "offline-org", "inactive"
		f.server = b
		if f.phase == "uncertain-create" && route == "server.create" {
			write(map[string]any{})
			return
		}
		write(map[string]any{"serverId": "offline-server"})
	case "server.one":
		require.Equal(f.t, "offline-server", r.URL.Query().Get("serverId"))
		if f.phase == "partial-create" && f.counts[route] == 1 {
			fail()
			return
		}
		if f.server == nil {
			missing()
			return
		}
		if f.phase == "read-mismatch" && f.counts[route] == 2 {
			wrong := map[string]any{}
			for key, value := range f.server {
				wrong[key] = value
			}
			wrong["name"] = "unexpected-name"
			write(wrong)
			return
		}
		write(f.server)
	case "server.remove":
		require.Equal(f.t, map[string]any{"serverId": "offline-server"}, b)
		if f.phase == "delete-error" && f.counts[route] == 1 {
			fail()
			return
		}
		if f.server == nil {
			missing()
			return
		}
		f.server = nil
		write(map[string]any{})
	case "application.create":
		require.Equal(f.t, http.MethodPost, r.Method)
		require.Equal(f.t, "offline-environment", b["environmentId"])
		require.Len(f.t, b, 2, "bare fixture must not configure a source, server, or deployment")
		f.names = append(f.names, b["name"].(string))
		f.application = map[string]any{"applicationId": fmt.Sprintf("offline-app-%d", len(f.names)), "name": b["name"], "environmentId": b["environmentId"]}
		if f.phase == "uncertain-application" {
			write(map[string]any{})
			return
		}
		write(f.application)
	case "application.delete":
		require.NotNil(f.t, f.application)
		require.Equal(f.t, map[string]any{"applicationId": f.application["applicationId"]}, b)
		require.Nil(f.t, f.port, "mapping absence must precede owner deletion")
		if f.phase == "fixture-delete-error" {
			fail()
			return
		}
		f.application = nil
		write(map[string]any{})
	case "application.one":
		if f.portBadRequestAbsence && f.application != nil {
			require.Equal(f.t, f.application["applicationId"], r.URL.Query().Get("applicationId"))
			application := map[string]any{}
			for key, value := range f.application {
				application[key] = value
			}
			ports := []any{}
			if f.port != nil {
				ports = append(ports, f.port)
			}
			application["ports"] = ports
			write(application)
			return
		}
		require.Nil(f.t, f.application, "bare application is read only for verified deletion")
		missing()
	case "port.create", "port.update":
		require.Equal(f.t, http.MethodPost, r.Method)
		require.NotNil(f.t, f.application)
		require.Len(f.t, b, 5)
		if route == "port.create" {
			require.Equal(f.t, f.application["applicationId"], b["applicationId"])
			require.Equal(f.t, float64(18080), b["publishedPort"])
			require.Equal(f.t, float64(8080), b["targetPort"])
			require.Equal(f.t, "tcp", b["protocol"])
			require.Equal(f.t, "ingress", b["publishMode"])
		} else {
			require.Equal(f.t, "offline-port", b["portId"])
			require.Equal(f.t, float64(18081), b["publishedPort"])
			require.Equal(f.t, float64(8081), b["targetPort"])
			require.Equal(f.t, "udp", b["protocol"])
			require.Equal(f.t, "host", b["publishMode"])
		}
		b["portId"], b["applicationId"] = "offline-port", f.application["applicationId"]
		f.port = b
		if f.phase == "uncertain-create" && route == "port.create" {
			write(map[string]any{})
			return
		}
		write(map[string]any{"portId": "offline-port"})
	case "port.one":
		require.Equal(f.t, "offline-port", r.URL.Query().Get("portId"))
		if f.phase == "partial-create" && f.counts[route] == 1 {
			fail()
			return
		}
		if f.port == nil {
			if f.portBadRequestAbsence {
				w.WriteHeader(http.StatusBadRequest)
				write(map[string]any{"code": "BAD_REQUEST", "message": "Port not found"})
				return
			}
			missing()
			return
		}
		if f.phase == "read-mismatch" && f.counts[route] == 2 {
			wrong := map[string]any{}
			for key, value := range f.port {
				wrong[key] = value
			}
			wrong["targetPort"] = 9999
			write(wrong)
			return
		}
		write(f.port)
	case "port.delete":
		require.Equal(f.t, map[string]any{"portId": "offline-port"}, b)
		if f.phase == "persistent-delete-error" {
			fail()
			return
		}
		if f.phase == "delete-error" && f.counts[route] == 1 {
			fail()
			return
		}
		if f.port == nil {
			if f.portBadRequestAbsence {
				w.WriteHeader(http.StatusBadRequest)
				write(map[string]any{"code": "BAD_REQUEST", "message": "Port not found"})
				return
			}
			missing()
			return
		}
		f.port = nil
		write(map[string]any{})
	default:
		f.t.Errorf("unsafe or unexpected lifecycle route: %s", route)
		w.WriteHeader(403)
	}
}

func isolateLiveRecordHarness(t *testing.T) string {
	t.Helper()
	snapshot := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(snapshot) })
	resetLiveHarnessState()
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	return marker
}

func TestServerLiveLifecycleOfflineRepeatable(t *testing.T) {
	marker := isolateLiveRecordHarness(t)
	f := newLiveRecordHTTPFixture(t, "success")
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) { require.NoError(t, runLiveServerLifecycle(t, t.Context(), f.api)) })
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.names, 2)
	require.NotEqual(t, f.names[0], f.names[1])
	require.NotEqual(t, f.hosts[0], f.hosts[1])
	require.Nil(t, f.server)
	require.Equal(t, 4, f.counts["server.update"], "metadata update and description clearing each run")
	require.Equal(t, 4, f.counts["server.remove"], "repeat delete must be idempotent")
	wantRun := []string{"server.create", "server.one", "server.one", "server.update", "server.one", "server.one", "server.update", "server.one", "server.one", "server.remove", "server.one", "server.remove", "server.one", "server.one"}
	require.Equal(t, append(append([]string{}, wantRun...), wantRun...), f.routes)
	require.False(t, heavyLiveTierStopped())
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err))
}

func TestServerLiveLookupRunsBeforeOwnedRecordCleanup(t *testing.T) {
	isolateLiveRecordHarness(t)
	f := newLiveRecordHTTPFixture(t, "success")
	lookups := 0
	err := runLiveServerLifecycle(t, t.Context(), f.api, func(id, name string) {
		lookups++
		t.Run("lookup", func(t *testing.T) { runLiveLookup(t, t.Context(), f.api, "Server", id, name) })
	})
	require.NoError(t, err)
	require.Equal(t, 1, lookups)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Nil(t, f.server, "lookup must not prevent verified cleanup")
}

func TestServerLiveLifecycleOfflineFailureCleanup(t *testing.T) {
	for _, phase := range []string{"partial-create", "read-mismatch", "update-error", "update-read-error", "delete-error", "absence-read-error", "uncertain-create"} {
		t.Run(phase, func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveRecordHTTPFixture(t, phase)
			t.Run("lifecycle", func(t *testing.T) {
				err := runLiveServerLifecycle(t, t.Context(), f.api)
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-upstream-detail")
				require.NotContains(t, err.Error(), "offline-server")
			})
			f.mu.Lock()
			defer f.mu.Unlock()
			if phase == "uncertain-create" {
				require.Zero(t, f.counts["server.remove"])
				require.True(t, heavyLiveTierStopped())
				contents, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Equal(t, "stop\n", string(contents))
			} else {
				require.Nil(t, f.server)
				require.False(t, heavyLiveTierStopped(), "ordinary failure with verified cleanup must not stop independent coverage")
				// An unrelated fixture still runs after this ordinary failure.
				next := newLiveRecordHTTPFixture(t, "success")
				t.Run("independent-next", func(t *testing.T) { require.NoError(t, runLiveServerLifecycle(t, t.Context(), next.api)) })
				next.mu.Lock()
				defer next.mu.Unlock()
				require.NotEqual(t, f.names[0], next.names[0])
				require.Nil(t, next.server)
			}
		})
	}
}

func TestServerLiveLifecycleOfflineCreateRejection(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422, 499, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveRecordHTTPFixture(t, "success")
			f.mu.Lock()
			f.rejectRoute, f.rejectStatus = "server.create", status
			f.mu.Unlock()
			t.Run("lifecycle", func(t *testing.T) {
				err := runLiveServerLifecycle(t, t.Context(), f.api)
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-upstream-detail")
			})
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Equal(t, []string{"server.create"}, f.routes, "unacknowledged identities must never be discovered or deleted")
			require.Nil(t, f.server)
			uncertain := status == 429 || status >= 500
			require.Equal(t, uncertain, heavyLiveTierStopped())
			_, err := os.Stat(marker)
			if uncertain {
				require.NoError(t, err)
			} else {
				require.True(t, os.IsNotExist(err), "definite rejection must not stop independent coverage")
			}
		})
	}
}
