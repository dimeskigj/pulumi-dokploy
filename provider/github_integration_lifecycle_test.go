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
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

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
