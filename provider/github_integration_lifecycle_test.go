package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

type lifecycleCancelContext struct {
	context.Context
	done chan struct{}
	err  error
}

func newLifecycleCancelContext() *lifecycleCancelContext {
	return &lifecycleCancelContext{Context: context.Background(), done: make(chan struct{})}
}
func (c *lifecycleCancelContext) Done() <-chan struct{} { return c.done }
func (c *lifecycleCancelContext) Err() error            { return c.err }
func (c *lifecycleCancelContext) cancel(err error)      { c.err = err; close(c.done) }

type lifecycleRoundTripFunc func(*http.Request) (*http.Response, error)

func (f lifecycleRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func lifecycleState() GitHubIntegrationState {
	return GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
}

func TestGitHubIntegrationUpdateCancellationStopsReadback(t *testing.T) {
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
				case "/api/github.one":
					fmt.Fprint(w, githubRecord)
				case "/api/github.update":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/github.update" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			state := lifecycleState()
			in := state.GitHubIntegrationArgs
			in.Name = "desired"
			in.GitHubAppName = "desired-app"
			out, err := (GitHubIntegration{client: func(context.Context) *client.Client { return api }}).Update(ctx, infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
			require.ErrorIs(t, err, cause)
			require.Equal(t, state, out.Output)
			require.Equal(t, []string{"/api/organization.active", "/api/github.one", "/api/github.update"}, paths)
		})
	}
}

func TestGitHubIntegrationInferUpdateCancellationReturnsPartialState(t *testing.T) {
	ctx := newLifecycleCancelContext()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			fmt.Fprint(w, githubRecord)
		case "/api/github.update":
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil && req.URL.Path == "/api/github.update" {
			ctx.cancel(context.Canceled)
		}
		return resp, err
	})
	api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	resource := infer.Resource(&GitHubIntegration{client: func(context.Context) *client.Client { return api }})
	state := property.NewMap(map[string]property.Value{"name": property.New("old"), "githubAppName": property.New("app"), "githubId": property.New("relation"), "gitProviderId": property.New("parent"), "organizationId": property.New("org")})
	inputs := property.NewMap(map[string]property.Value{"name": property.New("desired"), "githubAppName": property.New("desired-app")})
	resp, err := resource.Update(ctx, p.UpdateRequest{ID: "relation", Urn: lifecycleURN("GitHubIntegration", "infer-partial-state"), State: state, OldInputs: property.NewMap(map[string]property.Value{"name": property.New("old"), "githubAppName": property.New("app")}), Inputs: inputs})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, resp.PartialState)
	for key, value := range map[string]string{"name": "old", "githubAppName": "app", "githubId": "relation", "gitProviderId": "parent", "organizationId": "org"} {
		require.Equal(t, value, resp.Properties.Get(key).AsString(), key)
	}
}

func TestGitHubIntegrationInferUpdateReadbackCancellationReturnsActualPartialState(t *testing.T) {
	ctx := newLifecycleCancelContext()
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			reads++
			record := githubRecord
			if reads > 1 {
				record = strings.Replace(record, `"name":"old"`, `"name":"observed"`, 1)
				record = strings.Replace(record, `"githubAppName":"app"`, `"githubAppName":"observed-app"`, 1)
			}
			fmt.Fprint(w, record)
		case "/api/github.update":
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil && req.URL.Path == "/api/github.one" && reads == 2 {
			ctx.cancel(context.DeadlineExceeded)
		}
		return resp, err
	})
	api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	resource := infer.Resource(&GitHubIntegration{client: func(context.Context) *client.Client { return api }})
	state := property.NewMap(map[string]property.Value{"name": property.New("old"), "githubAppName": property.New("app"), "githubId": property.New("relation"), "gitProviderId": property.New("parent"), "organizationId": property.New("org")})
	inputs := property.NewMap(map[string]property.Value{"name": property.New("desired"), "githubAppName": property.New("desired-app")})
	resp, err := resource.Update(ctx, p.UpdateRequest{ID: "relation", Urn: lifecycleURN("GitHubIntegration", "infer-readback-partial-state"), State: state, OldInputs: property.NewMap(map[string]property.Value{"name": property.New("old"), "githubAppName": property.New("app")}), Inputs: inputs})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, resp.PartialState)
	for key, value := range map[string]string{"name": "observed", "githubAppName": "observed-app", "githubId": "relation", "gitProviderId": "parent", "organizationId": "org"} {
		require.Equal(t, value, resp.Properties.Get(key).AsString(), key)
	}
}

func TestGitHubIntegrationUpdateCancellationDuringReadback(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			reads := 0
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/github.one":
					reads++
					record := githubRecord
					if reads > 1 {
						record = strings.Replace(record, `"name":"old"`, `"name":"desired"`, 1)
						record = strings.Replace(record, `"githubAppName":"app"`, `"githubAppName":"desired-app"`, 1)
					}
					fmt.Fprint(w, record)
				case "/api/github.update":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/github.one" && reads == 2 {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			state := lifecycleState()
			in := state.GitHubIntegrationArgs
			in.Name = "desired"
			in.GitHubAppName = "desired-app"
			out, err := (GitHubIntegration{client: func(context.Context) *client.Client { return api }}).Update(ctx, infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
			require.ErrorIs(t, err, cause)
			require.Equal(t, "desired", out.Output.Name)
			require.Equal(t, "desired-app", out.Output.GitHubAppName)
			require.Equal(t, []string{"/api/organization.active", "/api/github.one", "/api/github.update", "/api/github.one"}, paths)
		})
	}
}

func TestGitHubIntegrationDeleteCancellationStopsVerification(t *testing.T) {
	ctx := newLifecycleCancelContext()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			fmt.Fprint(w, githubRecord)
		case "/api/gitProvider.remove":
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil && req.URL.Path == "/api/gitProvider.remove" {
			ctx.cancel(context.DeadlineExceeded)
		}
		return resp, err
	})
	api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	_, err = (GitHubIntegration{client: func(context.Context) *client.Client { return api }}).Delete(ctx, infer.DeleteRequest[GitHubIntegrationState]{ID: "relation", State: lifecycleState()})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, []string{"/api/organization.active", "/api/github.one", "/api/gitProvider.remove"}, paths)
}

func TestGitHubIntegrationUpdateRejectsChangedReadbackIdentity(t *testing.T) {
	for _, tc := range []struct{ name, field, from, to string }{{"relation", "githubId", "relation", "other"}, {"parent", "gitProviderId", "parent", "other"}, {"organization", "organizationId", "org", "other"}} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/github.one":
					reads++
					record := githubRecord
					if reads > 1 {
						record = strings.Replace(record, `"name":"old"`, `"name":"desired"`, 1)
						record = strings.Replace(record, `"githubAppName":"app"`, `"githubAppName":"desired-app"`, 1)
						switch tc.field {
						case "githubId":
							record = strings.Replace(record, `"githubId":"`+tc.from+`"`, `"githubId":"`+tc.to+`"`, 1)
						case "gitProviderId":
							record = strings.Replace(record, `"gitProviderId":"parent"`, `"gitProviderId":"other"`, 2)
						case "organizationId":
							record = strings.Replace(record, `"organizationId":"org"`, `"organizationId":"other"`, 1)
						}
					}
					fmt.Fprint(w, record)
				case "/api/github.update":
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			api, e := client.New(server.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, e)
			state := lifecycleState()
			in := state.GitHubIntegrationArgs
			in.Name = "desired"
			in.GitHubAppName = "desired-app"
			out, e := (GitHubIntegration{client: func(context.Context) *client.Client { return api }}).Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
			require.Error(t, e)
			require.Equal(t, state, out.Output)
			require.Equal(t, 2, reads)
		})
	}
}

func TestGitHubIntegrationUpdateAcknowledgedReadbackFailureKeepsPriorState(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			reads++
			if reads == 1 {
				fmt.Fprint(w, githubRecord)
			} else {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"message":"credential-sentinel"}`)
			}
		case "/api/github.update":
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	api, e := client.New(server.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	state := lifecycleState()
	in := state.GitHubIntegrationArgs
	in.Name = "desired"
	out, e := (GitHubIntegration{client: func(context.Context) *client.Client { return api }}).Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
	require.Error(t, e)
	require.NotContains(t, e.Error(), "credential-sentinel")
	require.Equal(t, state, out.Output)
	require.Equal(t, 2, reads)
}

func TestGitHubIntegrationReadAndImport(t *testing.T) {
	c := githubFixture(t, githubRecord, 200)
	read, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Read(t.Context(), infer.ReadRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation"})
	require.NoError(t, err)
	require.Equal(t, "relation", read.State.GitHubID)
	require.Equal(t, "app", read.State.GitHubAppName)
}

func TestGitHubIntegrationUpdateVerified(t *testing.T) {
	reads := 0
	var payload map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			reads++
			record := githubRecord
			if reads > 1 {
				record = strings.Replace(record, `"name":"old"`, `"name":"new"`, 1)
				record = strings.Replace(record, `"githubAppName":"app"`, `"githubAppName":"new-app"`, 1)
			}
			fmt.Fprint(w, record)
		case "/api/github.update":
			_ = json.NewDecoder(r.Body).Decode(&payload)
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	in := state.GitHubIntegrationArgs
	in.Name = "new"
	in.GitHubAppName = "new-app"
	out, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
	require.NoError(t, err)
	require.Equal(t, "new", out.Output.Name)
	require.Equal(t, 2, reads)
	require.Equal(t, "relation", payload["githubId"])
	require.Equal(t, "parent", payload["gitProviderId"])
	require.Equal(t, "new", payload["name"])
	require.Equal(t, "new-app", payload["githubAppName"])
	require.Len(t, payload, 4)
}

func TestGitHubIntegrationUpdatePartialState(t *testing.T) {
	// A rejected mutation still reads back a verified actual value, never the requested value.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			fmt.Fprint(w, githubRecord)
		case "/api/github.update":
			w.WriteHeader(400)
			fmt.Fprint(w, `{"message":"unsafe"}`)
		}
	}))
	defer s.Close()
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	in := state.GitHubIntegrationArgs
	in.Name = "desired"
	out, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
	require.Error(t, err)
	require.Equal(t, "old", out.Output.Name)
	require.NotContains(t, err.Error(), "unsafe")
}

func TestGitHubIntegrationUpdateAcknowledgedSplitState(t *testing.T) {
	reads := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			reads++
			record := githubRecord
			if reads > 1 {
				record = strings.Replace(record, `"name":"old"`, `"name":"new"`, 1)
			}
			fmt.Fprint(w, record)
		case "/api/github.update":
			fmt.Fprint(w, `{}`)
		}
	}))
	defer s.Close()
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	in := state.GitHubIntegrationArgs
	in.Name = "new"
	in.GitHubAppName = "new-app"
	out, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation", State: state, Inputs: in})
	require.Error(t, err)
	require.Equal(t, "new", out.Output.Name)
	require.Equal(t, "app", out.Output.GitHubAppName)
	require.Equal(t, 2, reads)
}

func TestGitHubIntegrationDeleteVerified(t *testing.T) {
	present := true
	var removed map[string]any
	var calls []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			if present {
				fmt.Fprint(w, githubRecord)
			} else {
				w.WriteHeader(404)
				fmt.Fprint(w, `{}`)
			}
		case "/api/gitProvider.remove":
			_ = json.NewDecoder(r.Body).Decode(&removed)
			present = false
			fmt.Fprint(w, `{}`)
		}
	}))
	defer s.Close()
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	_, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GitHubIntegrationState]{ID: "relation", State: state})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"gitProviderId": "parent"}, removed)
	require.Equal(t, []string{"/api/organization.active", "/api/github.one", "/api/gitProvider.remove", "/api/github.one"}, calls)
}

func TestGitHubIntegrationDeletePreReadAbsenceSkipsRemoval(t *testing.T) {
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/organization.active" {
			fmt.Fprint(w, `{"id":"org"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{}`)
	}))
	defer s.Close()
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	_, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GitHubIntegrationState]{ID: "relation"})
	require.NoError(t, err)
	require.Equal(t, []string{"/api/organization.active", "/api/github.one"}, paths)
}

func TestGitHubIntegrationDeleteAmbiguousFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/github.one":
			fmt.Fprint(w, githubRecord)
		case "/api/gitProvider.remove":
			w.WriteHeader(400)
			fmt.Fprint(w, `{"message":"unsafe"}`)
		}
	}))
	defer s.Close()
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	state := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	_, err := (GitHubIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GitHubIntegrationState]{ID: "relation", State: state})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "unsafe")
}
