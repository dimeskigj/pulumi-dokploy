package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

func TestBackupCleanupCreateResultOwnershipClassification(t *testing.T) {
	for _, operation := range []string{"backup.create", "volumeBackups.create"} {
		for _, tc := range []struct {
			name    string
			err     error
			pending bool
		}{
			{"create-400", &client.APIError{Operation: operation, StatusCode: 400}, false},
			{"create-404", &client.APIError{Operation: operation, StatusCode: 404}, false},
			{"create-422-wrapped", errors.Join(errors.New("private-response"), &client.APIError{Operation: operation, StatusCode: 422}), false},
			{"create-429", &client.APIError{Operation: operation, StatusCode: 429}, true},
			{"create-500", &client.APIError{Operation: operation, StatusCode: 500}, true},
			{"discovery-403", &client.APIError{Operation: "postgres.one", StatusCode: 403}, true},
			{"discovery-404", &client.APIError{Operation: "postgres.one", StatusCode: 404}, true},
			{"unknown-operation-400", &client.APIError{StatusCode: 400}, true},
			{"transport", errors.New("private-response"), true},
			{"canceled", context.Canceled, true},
			{"timeout", context.DeadlineExceeded, true},
			{"missing-success-identity", nil, true},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				saved := snapshotLiveHarnessState()
				t.Cleanup(func() { restoreLiveHarnessState(saved) })
				resetLiveHarnessState()
				marker := filepath.Join(t.TempDir(), "stop")
				t.Setenv(liveStopMarkerEnvironment, marker)
				state := &liveCleanupID{}
				require.False(t, state.pending())
				require.Error(t, noteLiveBackupCreateResult("backup", operation, state, tc.err))
				require.Equal(t, tc.pending, state.pending())
				require.Empty(t, state.current(), "uncertainty must not invent a cleanup ID")
				require.Equal(t, tc.pending, heavyLiveTierStopped())
				require.NoError(t, runLiveBackupCleanup(t.Context(), state, func(context.Context, string) error {
					t.Fatal("uncertain ownership must never delete by a guessed ID")
					return nil
				}, func(context.Context, string) (string, error) {
					t.Fatal("uncertain ownership must never read by a guessed ID")
					return "", nil
				}))
				require.Equal(t, tc.pending, state.pending(), "skipping unknown-ID cleanup must not resolve uncertainty")
				if tc.pending {
					content, err := os.ReadFile(marker)
					require.NoError(t, err)
					require.Equal(t, "stop\n", string(content))
				} else {
					_, err := os.Stat(marker)
					require.ErrorIs(t, err, os.ErrNotExist)
				}
			})
		}
	}
}

func TestBackupCleanupAcknowledgedCreateResultStaysPendingUntilAbsence(t *testing.T) {
	state := &liveCleanupID{id: "offline-backup"}
	require.NoError(t, noteLiveBackupCreateResult("backup", "backup.create", state, nil))
	require.True(t, state.pending())
	require.NoError(t, runLiveBackupCleanup(t.Context(), state, func(context.Context, string) error {
		return nil
	}, func(context.Context, string) (string, error) { return "", nil }))
	require.False(t, state.pending())
}

// Delete errors, read errors, and a still-present resource must all keep the
// acknowledged identity armed; only verified absence permits exactly-once exit.
func TestBackupCleanupRetainsOwnerUntilVerifiedAbsence(t *testing.T) {
	for _, phase := range []string{"read-error", "still-present", "delete-error", "delete-not-found-read-error"} {
		t.Run(phase, func(t *testing.T) {
			state := &liveCleanupID{id: "offline-backup"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deletes, reads := 0, 0
			err := runLiveBackupCleanup(ctx, state, func(c context.Context, id string) error {
				require.Same(t, ctx, c)
				require.Equal(t, "offline-backup", id)
				deletes++
				if phase == "delete-error" {
					return errors.New("private-delete-response")
				}
				if phase == "delete-not-found-read-error" {
					return &client.APIError{StatusCode: 404}
				}
				return nil
			}, func(c context.Context, id string) (string, error) {
				require.Same(t, ctx, c)
				require.Equal(t, "offline-backup", id)
				require.Equal(t, id, state.current(), "owner must remain armed during verification")
				reads++
				if phase == "still-present" {
					cancel()
					return id, nil
				}
				return "", errors.New("private-read-response")
			})
			require.Error(t, err)
			require.Equal(t, "offline-backup", state.current())
			require.Equal(t, 1, deletes)
			if phase == "delete-error" {
				require.Zero(t, reads)
			} else {
				require.Equal(t, 1, reads)
			}
			fresh, freshCancel := cleanupContext()
			defer freshCancel()
			remove := func(c context.Context, id string) error {
				require.NoError(t, c.Err())
				require.Equal(t, "offline-backup", id)
				deletes++
				return nil
			}
			read := func(c context.Context, id string) (string, error) {
				require.NoError(t, c.Err())
				require.Equal(t, "offline-backup", id)
				require.Equal(t, id, state.current())
				reads++
				return "", nil
			}
			require.NoError(t, runLiveBackupCleanup(fresh, state, remove, read))
			require.Empty(t, state.current())
			verifiedReads := reads
			require.NoError(t, runLiveBackupCleanup(fresh, state, remove, read))
			require.Equal(t, 2, deletes, "verified cleanup must never delete again")
			require.Equal(t, verifiedReads, reads, "verified cleanup must never read again")
		})
	}
}

func TestBackupCleanupPollsPresentStateBeforeDisarming(t *testing.T) {
	oldInterval := liveCleanupPollInterval
	liveCleanupPollInterval = time.Millisecond
	t.Cleanup(func() { liveCleanupPollInterval = oldInterval })
	state := &liveCleanupID{id: "offline-backup"}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	reads := 0
	require.NoError(t, runLiveBackupCleanup(ctx, state, func(context.Context, string) error {
		return &client.APIError{StatusCode: 404}
	}, func(_ context.Context, id string) (string, error) {
		require.Equal(t, id, state.current())
		reads++
		if reads < 3 {
			return id, nil
		}
		return "", &client.APIError{StatusCode: 404}
	}))
	require.Equal(t, 3, reads)
	require.Empty(t, state.current())
}

func TestBackupCleanupNeverUsesUnacknowledgedIdentity(t *testing.T) {
	state := &liveCleanupID{}
	require.NoError(t, runLiveBackupCleanup(t.Context(), state, func(context.Context, string) error {
		t.Fatal("must not delete an unacknowledged identity")
		return nil
	}, func(context.Context, string) (string, error) {
		t.Fatal("must not read an unacknowledged identity")
		return "", nil
	}))
}

func TestBackupCleanupFallbackUsesFreshBoundedContext(t *testing.T) {
	state := &liveCleanupID{id: "offline-backup"}
	var contexts []context.Context
	reads := 0
	t.Run("fixture", func(t *testing.T) {
		cleanup := registerLiveBackupCleanup(t, "backup", state, func(c context.Context, id string) error {
			require.Equal(t, "offline-backup", id)
			require.NoError(t, c.Err())
			_, bounded := c.Deadline()
			require.True(t, bounded)
			contexts = append(contexts, c)
			return nil
		}, func(c context.Context, id string) (string, error) {
			require.Equal(t, "offline-backup", id)
			require.NoError(t, c.Err())
			reads++
			if reads == 1 {
				return "", errors.New("private-read-response")
			}
			return "", nil
		})
		require.Error(t, cleanup())
		require.Equal(t, "offline-backup", state.current())
		require.ErrorIs(t, contexts[0].Err(), context.Canceled)
	})
	require.Empty(t, state.current())
	require.Len(t, contexts, 2)
	require.NotSame(t, contexts[0], contexts[1])
	require.ErrorIs(t, contexts[1].Err(), context.Canceled)
}

func TestBackupCleanupVerifiedExplicitDeleteDisarmsFallback(t *testing.T) {
	state := &liveCleanupID{id: "offline-backup"}
	deletes, reads := 0, 0
	t.Run("fixture", func(t *testing.T) {
		cleanup := registerLiveBackupCleanup(t, "backup", state, func(_ context.Context, id string) error {
			require.Equal(t, "offline-backup", id)
			deletes++
			return nil
		}, func(_ context.Context, id string) (string, error) {
			require.Equal(t, "offline-backup", id)
			require.Equal(t, id, state.current())
			reads++
			return "", nil
		})
		require.NoError(t, cleanup())
		require.Empty(t, state.current())
	})
	require.Equal(t, 1, deletes)
	require.Equal(t, 1, reads)
}

// Clearing the owner after remove but before its read loses the fallback delete.
// Exercise the actual Backup lifecycle in a child so its expected failure does
// not fail this test, and replace only the HTTP boundary with an offline server.
func TestBackupCleanupLifecycleRetriesFailedAbsenceRead(t *testing.T) {
	testBackupCleanupLifecycle(t, false)
}

func TestBackupCleanupLifecycleUnresolvedAbsenceStopsAndPreservesTarget(t *testing.T) {
	testBackupCleanupLifecycle(t, true)
}

func TestBackupCleanupNoIdentityCreateClassification(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		status, discoveryStatus int
		stop                    bool
		workload                bool
	}{
		{"post-create-discovery-400", http.StatusOK, http.StatusBadRequest, true, false},
		{"post-create-discovery-403", http.StatusOK, http.StatusForbidden, true, false},
		{"post-create-discovery-404", http.StatusOK, http.StatusNotFound, true, false},
		{"definite-rejection", http.StatusBadRequest, 0, false, false},
		{"rate-limited", http.StatusTooManyRequests, 0, true, false},
		{"workload/discovery-403", http.StatusOK, http.StatusForbidden, true, true},
		{"workload/definite-rejection", http.StatusBadRequest, 0, false, true},
		{"workload/rate-limited", http.StatusTooManyRequests, 0, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			creates, discoveries, targetDeletes := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/postgres.one":
					discoveries++
					if discoveries > 1 {
						w.WriteHeader(tc.discoveryStatus)
						_, _ = w.Write([]byte(`{"message":"private-discovery-response"}`))
						return
					}
					_, _ = w.Write([]byte(`{"postgresId":"offline-target","backups":[]}`))
				case "/api/backup.create":
					creates++
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{}`))
				case "/offline-target.remove":
					targetDeletes++
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Error("unacknowledged backup must never be read, removed, or discovered again")
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			t.Cleanup(server.Close)
			marker := filepath.Join(t.TempDir(), "stop")
			fixture := "^TestBackupCleanupOfflineLifecycleFixture$"
			if tc.workload {
				fixture = "^TestBackupCleanupOfflineWorkloadGuardFixture$"
			}
			require.Error(t, runOfflineLiveEntry(t, server.URL, fixture, marker, 1))
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, 1, creates)
			if tc.status == http.StatusOK {
				require.Equal(t, 2, discoveries)
			} else {
				require.Equal(t, 1, discoveries)
			}
			if tc.stop {
				require.Zero(t, targetDeletes, "uncertain dependent ownership must preserve the acknowledged target")
				content, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Equal(t, "stop\n", string(content))
			} else {
				require.Equal(t, 1, targetDeletes, "definite rejection must permit owned target cleanup")
				_, err := os.Stat(marker)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestBackupCleanupSupportingFixtureNoIdentityClassification(t *testing.T) {
	for _, kind := range []string{"destination", "application", "compose"} {
		for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusTooManyRequests} {
			name := kind + "/incomplete-success"
			switch status {
			case http.StatusBadRequest:
				name = kind + "/definite-rejection"
				if kind == "destination" {
					name = kind + "/unverifiable-rejection"
				}
			case http.StatusTooManyRequests:
				name = kind + "/rate-limited"
			}
			t.Run(name, func(t *testing.T) {
				var mu sync.Mutex
				creates := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					if r.URL.Path != "/api/"+kind+".create" {
						t.Error("unacknowledged fixture must never be read, deleted, or used as a backup target")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					creates++
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{}`))
				}))
				t.Cleanup(server.Close)
				marker := filepath.Join(t.TempDir(), "stop")
				require.Error(t, runOfflineLiveEntry(t, server.URL, "^TestBackupCleanupOfflineUnacknowledgedFixture$/^"+kind+"$", marker, 1))
				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, 1, creates)
				_, err := os.Stat(marker)
				// Destination sanitization currently erases APIError metadata.
				// An opaque error cannot prove that creation was rejected; never
				// reconstruct that proof by parsing untrusted error text.
				if status == http.StatusBadRequest && kind != "destination" {
					require.ErrorIs(t, err, os.ErrNotExist)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestBackupCleanupOfflineUnacknowledgedFixture(t *testing.T) {
	api := offlineBackupCleanupClient(t)
	t.Run("destination", func(t *testing.T) { liveBackupDestination(t, t.Context(), api) })
	t.Run("application", func(t *testing.T) {
		liveVolumeBackupForApplication(t, t.Context(), api, "offline-environment", "offline-destination")
	})
	t.Run("compose", func(t *testing.T) {
		liveVolumeBackupForCompose(t, t.Context(), api, "offline-environment", "offline-destination")
	})
}

func offlineBackupCleanupClient(t *testing.T) *client.Client {
	t.Helper()
	if os.Getenv("DOKPLOY_ACCEPTANCE") != "1" {
		t.Skip("offline subprocess fixture")
	}
	u, err := url.Parse(os.Getenv("DOKPLOY_ENDPOINT"))
	require.NoError(t, err)
	if u.Hostname() != "127.0.0.1" {
		t.Skip("offline fixture requires loopback HTTP boundary")
	}
	return liveClient(t)
}

func testBackupCleanupLifecycle(t *testing.T, unresolved bool) {
	t.Helper()
	var mu sync.Mutex
	var backup map[string]any
	deletes, absenceReads, targetDeletes := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{}
		if r.Method == http.MethodPost && r.URL.Path != "/offline-target.remove" {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		}
		respond := func(v any) { require.NoError(t, json.NewEncoder(w).Encode(v)) }
		switch r.URL.Path {
		case "/api/postgres.one":
			backups := []map[string]any{}
			if backup != nil {
				backups = append(backups, backup)
			}
			respond(map[string]any{"postgresId": "offline-target", "backups": backups})
		case "/api/backup.create":
			backup = body
			backup["backupId"] = "offline-backup"
			respond(map[string]any{})
		case "/api/backup.update":
			for k, v := range body {
				backup[k] = v
			}
			respond(map[string]any{})
		case "/api/backup.remove":
			require.Equal(t, "offline-backup", body["backupId"])
			deletes++
			backup = nil
			respond(map[string]any{})
		case "/api/backup.one":
			if backup != nil {
				respond(backup)
				return
			}
			absenceReads++
			if absenceReads == 1 || unresolved {
				w.WriteHeader(http.StatusBadRequest)
				respond(map[string]string{"message": "private-response-sentinel"})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			respond(map[string]string{"message": "absent"})
		case "/offline-target.remove":
			targetDeletes++
			respond(map[string]any{})
		default:
			t.Error("unexpected offline backup route")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), "stop")
	require.Error(t, runOfflineLiveEntry(t, server.URL, "^TestBackupCleanupOfflineLifecycleFixture$", marker, 1))
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, deletes, "failed explicit verification must retain fallback ownership")
	require.Equal(t, 2, absenceReads, "fallback must verify absence before disarming")
	require.Nil(t, backup)
	if unresolved {
		require.Zero(t, targetDeletes, "unresolved backup ownership must prevent cascading target deletion")
		content, err := os.ReadFile(marker)
		require.NoError(t, err)
		require.Equal(t, "stop\n", string(content))
		return
	}
	require.Equal(t, 1, targetDeletes, "verified fallback must permit target cleanup")
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "ordinary failure resolved by fallback must not stop later tiers")
}

func TestBackupCleanupOfflineLifecycleFixture(t *testing.T) {
	runBackupCleanupOfflineLifecycleFixture(t, false)
}

func TestBackupCleanupOfflineWorkloadGuardFixture(t *testing.T) {
	runBackupCleanupOfflineLifecycleFixture(t, true)
}

func runBackupCleanupOfflineLifecycleFixture(t *testing.T, workload bool) {
	t.Helper()
	api := offlineBackupCleanupClient(t)
	u, err := url.Parse(os.Getenv("DOKPLOY_ENDPOINT"))
	require.NoError(t, err)
	target := liveBackupTarget{
		id: "offline-target", resource: "postgres", lease: &liveHeavyOperationLease{released: true},
		remove: func(c context.Context) error {
			req, e := http.NewRequestWithContext(c, http.MethodPost, u.String()+"/offline-target.remove", http.NoBody)
			if e != nil {
				return e
			}
			resp, e := http.DefaultClient.Do(req)
			if e == nil {
				require.NoError(t, resp.Body.Close())
			}
			return e
		},
		readID: func(context.Context) (string, error) { return "", nil },
	}
	if workload {
		t.Cleanup(func() {
			cleanupLiveResource(t, "application", target.id, target.dependent, target.remove, target.readID)
		})
	} else {
		t.Cleanup(func() { target.cleanup(t) })
	}
	liveBackupCRUD(t, context.Background(), Backup{client: fixedClient(api)}, BackupArgs{
		Schedule: "0 0 * * *", Enabled: false, Prefix: "live-test-", DestinationID: "offline-destination",
		Database: "app", PostgresID: stringPtr("offline-target"),
	}, "offline-target", "postgresId", &target)
}
