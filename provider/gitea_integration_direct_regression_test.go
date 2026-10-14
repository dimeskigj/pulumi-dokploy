package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestGiteaIntegrationUpdateMutationCancellationDoesNotRetryOrReadBack(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/gitea.one":
					fmt.Fprint(w, giteaRecord)
				case "/api/gitea.update":
					fmt.Fprint(w, `{}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/gitea.update" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			state := GiteaIntegrationState{GiteaIntegrationArgs: giteaArgs(), GiteaID: "relation", GitProviderID: "parent", OrganizationID: "org"}
			inputs := state.GiteaIntegrationArgs
			inputs.Name = "changed"
			result, err := (GiteaIntegration{client: func(context.Context) *client.Client { return api }}).Update(ctx, infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "relation", State: state, Inputs: inputs})
			require.ErrorIs(t, err, cause)
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.Equal(t, state, result.Output)
			require.Equal(t, []string{"/api/organization.active", "/api/gitea.one", "/api/gitea.update"}, paths)
		})
	}
}

func TestGiteaIntegrationDeleteBranches(t *testing.T) {
	for _, tc := range []struct {
		name            string
		removeStatus    int
		readbackStatus  int
		wantError       bool
		wantAbsenceHint bool
	}{
		{name: "pre-read absence skips removal", removeStatus: 200, readbackStatus: http.StatusNotFound},
		{name: "remove error followed by verified absence succeeds", removeStatus: http.StatusBadRequest, readbackStatus: http.StatusNotFound},
		{name: "generic bad request while relation remains fails", removeStatus: http.StatusBadRequest, readbackStatus: http.StatusOK, wantError: true},
		{name: "ambiguous absence verification fails safely", removeStatus: http.StatusBadRequest, readbackStatus: http.StatusServiceUnavailable, wantError: true, wantAbsenceHint: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			present := tc.name != "pre-read absence skips removal"
			var paths []string
			removeCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/gitea.one":
					if !present {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{}`)
						return
					}
					if removeCalls > 0 && tc.readbackStatus != http.StatusOK {
						w.WriteHeader(tc.readbackStatus)
						fmt.Fprint(w, `{"message":"unsafe-sentinel"}`)
						return
					}
					fmt.Fprint(w, giteaRecord)
				case "/api/gitProvider.remove":
					removeCalls++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]any{"gitProviderId": "parent"}, body)
					w.WriteHeader(tc.removeStatus)
					fmt.Fprint(w, `{"message":"unsafe-sentinel"}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer server.Close()
			api, err := client.New(server.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			_, err = (GiteaIntegration{client: func(context.Context) *client.Client { return api }}).Delete(t.Context(), infer.DeleteRequest[GiteaIntegrationState]{ID: "relation", State: GiteaIntegrationState{GiteaID: "relation"}})
			if tc.wantError {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "unsafe-sentinel")
			} else {
				require.NoError(t, err)
			}
			if tc.wantAbsenceHint {
				require.Contains(t, err.Error(), "absence unconfirmed")
			}
			if tc.name == "pre-read absence skips removal" {
				require.Zero(t, removeCalls)
				require.Equal(t, []string{"/api/organization.active", "/api/gitea.one"}, paths)
				return
			}
			require.Equal(t, 1, removeCalls, "remove is never retried")
			require.Equal(t, []string{"/api/organization.active", "/api/gitea.one", "/api/gitProvider.remove", "/api/gitea.one"}, paths)
		})
	}
}
