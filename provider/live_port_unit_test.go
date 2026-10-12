package dokploy

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPortLiveLifecycleOfflineIndependentRepeatable(t *testing.T) {
	isolateLiveRecordHarness(t)
	f := newLiveRecordHTTPFixture(t, "success")
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			guard, err := runLivePortLifecycle(t, t.Context(), f.api, "offline-environment")
			require.NoError(t, err)
			retained := false
			t.Cleanup(func() { guard.retainProjectIfNeeded(func() { retained = true }); require.False(t, retained) })
		})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.names, 2)
	require.NotEqual(t, f.names[0], f.names[1])
	require.Nil(t, f.port)
	require.Nil(t, f.application)
	require.Equal(t, 2, f.counts["application.create"])
	require.Equal(t, 2, f.counts["application.delete"])
	require.Equal(t, 2, f.counts["port.update"])
	require.Equal(t, 4, f.counts["port.delete"])
	wantRun := []string{"application.create", "port.create", "port.one", "port.one", "port.update", "port.one", "port.one", "port.one", "port.delete", "port.one", "port.delete", "port.one", "application.delete", "application.one"}
	require.Equal(t, append(append([]string{}, wantRun...), wantRun...), f.routes)
	require.False(t, heavyLiveTierStopped())
}

func TestPortLiveCleanupRetainsStateForAmbiguousAbsence(t *testing.T) {
	for _, phase := range []string{"success", "partial-create", "update-error"} {
		t.Run(phase, func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveRecordHTTPFixture(t, phase)
			f.portBadRequestAbsence = true
			for i := 0; i < 2; i++ {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					var guard *livePortCleanupGuard
					// Evaluate retention after the nested fixture's cleanup callbacks.
					t.Cleanup(func() {
						retained := false
						guard.retainProjectIfNeeded(func() { retained = true })
						require.False(t, retained)
					})
					t.Run("port", func(t *testing.T) {
						var err error
						guard, err = runLivePortLifecycle(t, t.Context(), f.api, "offline-environment")
						// The partial-create fixture injects its read-back fault once;
						// its second independent lifecycle must complete normally.
						if phase == "success" || phase == "partial-create" && i > 0 {
							require.NoError(t, err)
						} else {
							require.Error(t, err)
						}
					})
				})
				f.mu.Lock()
				require.Nil(t, f.port)
				require.Nil(t, f.application, "known owner is removed after confirmed port absence")
				f.mu.Unlock()
				require.False(t, heavyLiveTierStopped())
				_, err := os.Stat(marker)
				require.True(t, os.IsNotExist(err))
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Equal(t, 2, f.counts["application.delete"])
			require.Greater(t, f.counts["application.one"], 2, "ambiguity must consult the saved owner, not ignore the 400")
		})
	}
}

// Cleanup correctly reports a failing test for unresolved absence. A child
// process lets this offline test verify that failure and its safety guard
// without suppressing the real cleanup reporter or marking our suite failed.
func TestPortLiveLifecycleOfflinePersistentCleanupGuard(t *testing.T) {
	phase := os.Getenv("PULUMI_DOKPLOY_OFFLINE_PORT_CLEANUP_CHILD")
	if phase == "" {
		for _, phase := range []string{"persistent-delete-error", "fixture-delete-error"} {
			t.Run(phase, func(t *testing.T) {
				// Execute only this test binary with a fixed offline helper filter.
				cmd := exec.Command(os.Args[0], "-test.run=^TestPortLiveLifecycleOfflinePersistentCleanupGuard$", "-test.v") //nolint:gosec
				cmd.Env = append(os.Environ(), "DOKPLOY_ACCEPTANCE=0", "PULUMI_DOKPLOY_OFFLINE_PORT_CLEANUP_CHILD="+phase)
				out, err := cmd.CombinedOutput()
				require.Error(t, err, "unresolved cleanup must fail the child test")
				require.Contains(t, string(out), "offline persistent guard verified")
				require.Contains(t, string(out), "operation=cleanup")
				for _, private := range []string{"offline-app", "offline-port", "private-upstream-detail"} {
					require.NotContains(t, string(out), private)
				}
			})
		}
		return
	}
	marker := isolateLiveRecordHarness(t)
	f := newLiveRecordHTTPFixture(t, phase)
	retained := false
	t.Run("parent", func(t *testing.T) {
		var guard *livePortCleanupGuard
		t.Cleanup(func() { guard.retainProjectIfNeeded(func() { retained = true }) })
		t.Run("port", func(t *testing.T) {
			var err error
			guard, err = runLivePortLifecycle(t, t.Context(), f.api, "offline-environment")
			require.Error(t, err)
		})
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	require.True(t, retained)
	require.NotNil(t, f.application)
	require.True(t, heavyLiveTierStopped())
	contents, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(contents))
	if phase == "persistent-delete-error" {
		require.NotNil(t, f.port)
		require.Equal(t, 2, f.counts["port.delete"], "explicit and fresh-context fallback attempts")
		require.Zero(t, f.counts["application.delete"])
	} else {
		require.Nil(t, f.port)
		require.Equal(t, 2, f.counts["application.delete"])
	}
	t.Log("offline persistent guard verified")
}

func TestPortLiveLifecycleOfflineFailureIsolation(t *testing.T) {
	for _, phase := range []string{"partial-create", "read-mismatch", "update-error", "update-read-error", "delete-error", "absence-read-error", "uncertain-create", "uncertain-application"} {
		t.Run(phase, func(t *testing.T) {
			marker := isolateLiveRecordHarness(t)
			f := newLiveRecordHTTPFixture(t, phase)
			retained := false
			t.Run("parent", func(t *testing.T) {
				var guard *livePortCleanupGuard
				// Parent cleanup runs after the nested fixture's fallbacks.
				t.Cleanup(func() { guard.retainProjectIfNeeded(func() { retained = true }) })
				t.Run("port", func(t *testing.T) {
					var err error
					guard, err = runLivePortLifecycle(t, t.Context(), f.api, "offline-environment")
					require.Error(t, err)
					require.NotContains(t, err.Error(), "private-upstream-detail")
					require.NotContains(t, err.Error(), "offline-app")
					require.NotContains(t, err.Error(), "offline-port")
				})
			})
			f.mu.Lock()
			defer f.mu.Unlock()
			if phase == "uncertain-create" || phase == "uncertain-application" {
				require.True(t, retained, "parent project must not cascade into unresolved ownership")
				require.Zero(t, f.counts["application.delete"])
				require.Zero(t, f.counts["port.delete"])
				require.True(t, heavyLiveTierStopped())
				contents, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Equal(t, "stop\n", string(contents))
			} else {
				require.False(t, retained)
				require.Nil(t, f.application)
				require.Nil(t, f.port)
				require.False(t, heavyLiveTierStopped())
				next := newLiveRecordHTTPFixture(t, "success")
				t.Run("independent-next", func(t *testing.T) {
					_, err := runLivePortLifecycle(t, t.Context(), next.api, "offline-environment")
					require.NoError(t, err)
				})
				next.mu.Lock()
				defer next.mu.Unlock()
				require.NotEqual(t, f.names[0], next.names[0])
				require.Nil(t, next.application)
				require.Nil(t, next.port)
			}
		})
	}
}

func TestPortLiveLifecycleOfflineCreateRejection(t *testing.T) {
	for _, route := range []string{"application.create", "port.create"} {
		for _, status := range []int{400, 401, 403, 404, 422, 499, 429, 500, 503} {
			t.Run(fmt.Sprintf("%s/%d", route, status), func(t *testing.T) {
				marker := isolateLiveRecordHarness(t)
				f := newLiveRecordHTTPFixture(t, "success")
				f.mu.Lock()
				f.rejectRoute, f.rejectStatus = route, status
				f.mu.Unlock()
				retained := false
				t.Run("parent", func(t *testing.T) {
					var guard *livePortCleanupGuard
					t.Cleanup(func() { guard.retainProjectIfNeeded(func() { retained = true }) })
					t.Run("port", func(t *testing.T) {
						var err error
						guard, err = runLivePortLifecycle(t, t.Context(), f.api, "offline-environment")
						require.Error(t, err)
						require.NotContains(t, err.Error(), "private-upstream-detail")
					})
				})
				f.mu.Lock()
				defer f.mu.Unlock()
				uncertain := status == 429 || status >= 500
				require.Equal(t, uncertain, retained)
				require.Equal(t, uncertain, heavyLiveTierStopped())
				require.Zero(t, f.counts["port.delete"], "no mapping identity was acknowledged")
				if route == "port.create" && !uncertain {
					require.Equal(t, []string{"application.create", "port.create", "application.delete", "application.one"}, f.routes)
					require.Nil(t, f.application, "definite mapping rejection permits owned bare fixture cleanup")
				} else {
					require.Zero(t, f.counts["application.delete"])
				}
				_, err := os.Stat(marker)
				if uncertain {
					require.NoError(t, err)
				} else {
					require.True(t, os.IsNotExist(err))
				}
			})
		}
	}
}
