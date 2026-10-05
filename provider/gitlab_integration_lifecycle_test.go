package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

const gitLabRecord = `{"gitlabId":"relation","gitProviderId":"parent","gitlabUrl":"https://gitlab.example","gitProvider":{"gitProviderId":"parent","name":"old","type":"gitlab","organizationId":"org","userId":"user"},"applicationId":"app","secret":null,"redirectUri":"https://callback.example","groupName":null,"gitlabInternalUrl":null,"accessToken":null,"refreshToken":null,"other":{"accessToken":"access-token-sentinel"}}`

func gitLabFixture(t *testing.T, handle func(http.ResponseWriter, *http.Request)) *client.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(handle))
	t.Cleanup(s.Close)
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return c
}

func gitLabRouter(t *testing.T, record string, extra func(http.ResponseWriter, *http.Request)) (*client.Client, *[]string) {
	t.Helper()
	paths := &[]string{}
	c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			require.Equal(t, "relation", r.URL.Query().Get("gitlabId"))
			fmt.Fprint(w, record)
		default:
			if extra != nil {
				extra(w, r)
			} else {
				t.Errorf("unexpected request %s", r.URL.Path)
			}
		}
	})
	return c, paths
}

func gitLabPrior() GitLabIntegrationState {
	return GitLabIntegrationState{GitLabIntegrationArgs: GitLabIntegrationArgs{Name: "old", ApplicationID: "app", ApplicationSecret: "prior-secret", RedirectURI: "https://callback.example", GitlabURL: "https://gitlab.example"}, GitLabID: "relation", GitProviderID: "parent", OrganizationID: "org", IsConfigured: true}
}

func TestGitLabIntegrationUpdateVerified(t *testing.T) {
	var body map[string]any
	calls := 0
	observations := 0
	c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			observations++
			record := gitLabRecord
			if observations > 1 {
				record = strings.Replace(record, `"name":"old"`, `"name":"changed"`, 1)
			}
			fmt.Fprint(w, record)
		case "/api/gitlab.update":
			calls++
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			fmt.Fprint(w, `{"untrusted":"access-token-sentinel"}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	in := gitLabPrior().GitLabIntegrationArgs
	in.Name = "changed"
	result, err := r.Update(t.Context(), infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", Inputs: in, State: gitLabPrior()})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 2, observations)
	require.Equal(t, "changed", result.Output.Name)
	require.Equal(t, map[string]any{
		"gitlabId": "relation", "gitProviderId": "parent", "name": "changed",
		"applicationId": "app", "secret": "prior-secret", "redirectUri": "https://callback.example",
		"gitlabUrl": "https://gitlab.example", "groupName": "", "gitlabInternalUrl": nil,
	}, body)
}

func TestGitLabIntegrationUpdatePartialState(t *testing.T) {
	calls := 0
	c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			calls++
			record := gitLabRecord
			if calls > 1 {
				record = strings.Replace(record, `"name":"old"`, `"name":"partially-renamed"`, 1)
			}
			fmt.Fprint(w, record)
		case "/api/gitlab.update":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"access-token-sentinel"}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	in := gitLabPrior().GitLabIntegrationArgs
	in.Name = "requested-name"
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	result, err := r.Update(t.Context(), infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", State: gitLabPrior(), Inputs: in})
	require.Error(t, err)
	var partial infer.ResourceInitFailedError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, "partially-renamed", result.Output.Name)
	require.Equal(t, "prior-secret", result.Output.ApplicationSecret)
	require.Equal(t, 2, calls)
	require.NotContains(t, err.Error(), "access-token-sentinel")
}

func TestGitLabIntegrationDeleteVerified(t *testing.T) {
	present := true
	var removed map[string]any
	c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			if !present {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{}`)
			} else {
				fmt.Fprint(w, gitLabRecord)
			}
		case "/api/gitProvider.remove":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&removed))
			present = false
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	req := infer.DeleteRequest[GitLabIntegrationState]{ID: "relation", State: gitLabPrior()}
	_, err := r.Delete(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"gitProviderId": "parent"}, removed)
	_, err = r.Delete(t.Context(), req)
	require.NoError(t, err)
}

func TestGitLabIntegrationMutationBodyIgnored(t *testing.T) {
	var calls []string
	c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			fmt.Fprint(w, gitLabRecord)
		case "/api/gitlab.update":
			fmt.Fprint(w, `{not-json access-token-sentinel`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	_, err := r.Update(t.Context(), infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", State: gitLabPrior(), Inputs: gitLabPrior().GitLabIntegrationArgs})
	require.NoError(t, err)
	require.Equal(t, []string{"/api/organization.active", "/api/gitlab.one", "/api/gitlab.update", "/api/gitlab.one"}, calls)
}

type gitLabCloseErrorBody struct{ io.ReadCloser }

func (body gitLabCloseErrorBody) Close() error {
	_ = body.ReadCloser.Close()
	return fmt.Errorf("access-token-sentinel")
}

type gitLabCloseErrorTransport struct{ http.RoundTripper }

func (tr gitLabCloseErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := tr.RoundTripper.RoundTrip(req)
	if err == nil && req.URL.Path == "/api/gitlab.update" {
		resp.Body = gitLabCloseErrorBody{resp.Body}
	}
	return resp, err
}

func TestGitLabIntegrationMutationCloseErrorAcknowledged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitlab.one":
			fmt.Fprint(w, gitLabRecord)
		case "/api/gitlab.update":
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: gitLabCloseErrorTransport{http.DefaultTransport}}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	res, err := (GitLabIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", State: gitLabPrior(), Inputs: gitLabPrior().GitLabIntegrationArgs})
	require.NoError(t, err)
	require.Equal(t, "relation", res.Output.GitLabID)
}

func TestGitLabIntegrationCancellation(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			c := gitLabFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("canceled request must not reach server") })
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := (GitLabIntegration{client: func(context.Context) *client.Client { return c }}).Read(ctx, infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation"})
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, sanitizeGitLabIntegrationError(fmt.Errorf("token-sentinel: %w", cause)), cause)
		})
	}
}

func TestGitLabIntegrationUpdateUnconfirmedRotation(t *testing.T) {
	c, _ := gitLabRouter(t, gitLabRecord, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/gitlab.update", r.URL.Path)
		fmt.Fprint(w, `{}`)
	})
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	in := gitLabPrior().GitLabIntegrationArgs
	in.ApplicationSecret = "rotated-secret"
	res, err := r.Update(t.Context(), infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", Inputs: in, State: gitLabPrior()})
	var partial infer.ResourceInitFailedError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, "prior-secret", res.Output.ApplicationSecret)
	require.NotContains(t, err.Error(), "rotated-secret")
}

func TestGitLabIntegrationDeleteAmbiguousFailure(t *testing.T) {
	c, _ := gitLabRouter(t, gitLabRecord, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/gitProvider.remove", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"message":"access-token-sentinel"}`)
	})
	_, err := (GitLabIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GitLabIntegrationState]{ID: "relation", State: gitLabPrior()})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "access-token-sentinel")
}

func TestGitLabIntegrationDeleteFailedRemoveReadback(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		absent    bool
		uncertain bool
	}{
		{"BAD_REQUEST then absent", http.StatusBadRequest, true, false},
		{"forbidden still present", http.StatusForbidden, false, false},
		{"transient still present", http.StatusServiceUnavailable, false, false},
		{"transient uncertain readback", http.StatusServiceUnavailable, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oneCalls, removeCalls := 0, 0
			c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/gitlab.one":
					oneCalls++
					if oneCalls > 1 && tt.uncertain {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"message":"access-token-sentinel"}`)
					} else if oneCalls > 1 && tt.absent {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{}`)
					} else {
						fmt.Fprint(w, gitLabRecord)
					}
				case "/api/gitProvider.remove":
					removeCalls++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]any{"gitProviderId": "parent"}, body)
					w.WriteHeader(tt.status)
					fmt.Fprint(w, `{"message":"access-token-sentinel"}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			})
			_, err := (GitLabIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GitLabIntegrationState]{ID: "relation", State: gitLabPrior()})
			if tt.absent {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "access-token-sentinel")
				if tt.uncertain {
					require.Contains(t, err.Error(), "absence unconfirmed")
				}
			}
			require.Equal(t, 1, removeCalls, "never retry remove")
			require.Equal(t, 2, oneCalls, "pre-read and one readback only")
		})
	}
}
