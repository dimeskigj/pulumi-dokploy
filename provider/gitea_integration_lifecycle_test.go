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

func giteaArgs() GiteaIntegrationArgs {
	return GiteaIntegrationArgs{Name: "display", GiteaURL: "https://gitea.example", ClientID: "client", ClientSecret: "secret", RedirectURI: "https://dokploy.example/callback"}
}

func TestGiteaIntegrationCreateAcknowledged(t *testing.T) {
	var payload map[string]any
	calls := []string{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitea.create":
			_ = json.NewDecoder(r.Body).Decode(&payload)
			fmt.Fprint(w, `{"giteaId":"42","clientId":"client","giteaUrl":"https://gitea.example"}`)
		case "/api/gitea.one":
			fmt.Fprint(w, `{"giteaId":"42","gitProviderId":"parent","giteaUrl":"https://gitea.example","clientId":"client","clientSecret":"secret","redirectUri":"https://dokploy.example/callback","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"parent","name":"display","providerType":"gitea","organizationId":"org","userId":"owner"}}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	out, err := (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Create(t.Context(), infer.CreateRequest[GiteaIntegrationArgs]{Inputs: giteaArgs()})
	require.NoError(t, err)
	require.Equal(t, "42", out.ID)
	require.Equal(t, "parent", out.Output.GitProviderID)
	require.Equal(t, []string{"/api/organization.active", "/api/gitea.create", "/api/gitea.one"}, calls)
	require.Equal(t, map[string]any{"name": "display", "giteaUrl": "https://gitea.example", "clientId": "client", "clientSecret": "secret", "redirectUri": "https://dokploy.example/callback", "organizationName": "", "giteaInternalUrl": nil}, payload)
}

func TestGiteaIntegrationCreatePartialState(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitea.create":
			fmt.Fprint(w, `{"giteaId":"42","clientId":"client","giteaUrl":"https://gitea.example"}`)
		case "/api/gitea.one":
			w.WriteHeader(500)
			fmt.Fprint(w, `{"message":"unsafe"}`)
		}
	}))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	out, err := (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Create(t.Context(), infer.CreateRequest[GiteaIntegrationArgs]{Inputs: giteaArgs()})
	require.Error(t, err)
	require.Equal(t, "42", out.ID)
	require.Equal(t, "42", out.Output.GiteaID)
	require.Empty(t, out.Output.GitProviderID)
	require.Empty(t, out.Output.OrganizationID)
	require.NotContains(t, err.Error(), "unsafe")
}

func TestGiteaIntegrationUpdateVerifiedAndDeleteVerified(t *testing.T) {
	reads := 0
	removed := false
	var update map[string]any
	var remove map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitea.one":
			if removed {
				w.WriteHeader(404)
				fmt.Fprint(w, `{}`)
				return
			}
			reads++
			record := `{"giteaId":"42","gitProviderId":"parent","giteaUrl":"https://gitea.example","clientId":"client","clientSecret":"secret","redirectUri":"https://dokploy.example/callback","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"parent","name":"display","providerType":"gitea","organizationId":"org","userId":"owner"}}`
			if reads > 1 {
				record = `{"giteaId":"42","gitProviderId":"parent","giteaUrl":"https://gitea.example/new","clientId":"new-client","clientSecret":"new-secret","redirectUri":"https://dokploy.example/new","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"parent","name":"new-name","providerType":"gitea","organizationId":"org","userId":"owner"}}`
			}
			fmt.Fprint(w, record)
		case "/api/gitea.update":
			_ = json.NewDecoder(r.Body).Decode(&update)
			fmt.Fprint(w, `not-json`)
		case "/api/gitProvider.remove":
			_ = json.NewDecoder(r.Body).Decode(&remove)
			removed = true
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	state := GiteaIntegrationState{GiteaIntegrationArgs: giteaArgs(), GiteaID: "42", GitProviderID: "parent", OrganizationID: "org"}
	desired := giteaArgs()
	desired.Name = "new-name"
	desired.GiteaURL = "https://gitea.example/new"
	desired.ClientID = "new-client"
	desired.ClientSecret = "new-secret"
	desired.RedirectURI = "https://dokploy.example/new"
	out, err := (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "42", State: state, Inputs: desired})
	require.NoError(t, err)
	require.Equal(t, "new-name", out.Output.Name)
	require.Equal(t, map[string]any{"giteaId": "42", "gitProviderId": "parent", "name": "new-name", "giteaUrl": "https://gitea.example/new", "clientId": "new-client", "clientSecret": "new-secret", "redirectUri": "https://dokploy.example/new", "organizationName": "", "giteaInternalUrl": nil}, update)
	_, err = (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Delete(t.Context(), infer.DeleteRequest[GiteaIntegrationState]{ID: "42", State: out.Output})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"gitProviderId": "parent"}, remove)
}

func TestGiteaIntegrationUpdateRepairsIncompleteImport(t *testing.T) {
	reads := 0
	mutations := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/organization.active":
			fmt.Fprint(w, `{"id":"org"}`)
		case "/api/gitea.one":
			reads++
			secret := `,"clientSecret":"rotated-secret"`
			if reads == 1 {
				secret = ""
			}
			fmt.Fprintf(w, `{"giteaId":"42","gitProviderId":"parent","giteaUrl":"https://gitea.example","clientId":"client","redirectUri":"https://dokploy.example/callback","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"parent","name":"display","providerType":"gitea","organizationId":"org","userId":"owner"}%s}`, secret)
		case "/api/gitea.update":
			mutations++
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "rotated-secret", payload["clientSecret"])
			fmt.Fprint(w, `not-json`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	prior := GiteaIntegrationState{GiteaIntegrationArgs: GiteaIntegrationArgs{Name: "display", GiteaURL: "https://gitea.example", ClientID: "client", RedirectURI: "https://dokploy.example/callback"}, GiteaID: "42", GitProviderID: "parent", OrganizationID: "org"}
	desired := prior.GiteaIntegrationArgs
	desired.ClientSecret = "rotated-secret"
	out, err := (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "42", State: prior, Inputs: desired})
	require.NoError(t, err)
	require.Equal(t, "rotated-secret", out.Output.ClientSecret)
	require.Equal(t, 1, mutations)
	require.Equal(t, 2, reads)
}

func TestGiteaIntegrationUpdateRejectsIncompleteDesiredInputsBeforeMutation(t *testing.T) {
	requests := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer s.Close()
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	desired := giteaArgs()
	desired.ClientSecret = ""
	_, err = (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Update(t.Context(), infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "42", Inputs: desired})
	require.Error(t, err)
	require.Zero(t, requests)
}
