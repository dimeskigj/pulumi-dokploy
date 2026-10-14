package dokploy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestSchemaGiteaIntegrationContract(t *testing.T) {
	spec := providerSchema(t)
	r, ok := spec.Resources["dokploy:index:GiteaIntegration"]
	require.True(t, ok)
	require.ElementsMatch(t, []string{"name", "giteaUrl", "clientId", "clientSecret", "redirectUri"}, r.RequiredInputs)
	require.ElementsMatch(t, []string{"name", "giteaUrl", "clientId", "clientSecret", "redirectUri", "giteaInternalUrl", "organizationName"}, objectKeys(r.InputProperties))
	require.ElementsMatch(t, []string{"name", "giteaUrl", "clientId", "clientSecret", "redirectUri", "giteaInternalUrl", "organizationName", "giteaId", "gitProviderId", "organizationId"}, objectKeys(r.Properties))
	for _, field := range []string{"name", "giteaUrl", "clientId", "clientSecret", "redirectUri", "giteaInternalUrl", "organizationName"} {
		require.NotEmpty(t, r.InputProperties[field].Description, field)
		require.NotEmpty(t, r.Properties[field].Description, field)
		require.False(t, r.InputProperties[field].ReplaceOnChanges, field)
		require.False(t, r.Properties[field].ReplaceOnChanges, field)
	}
	for _, field := range []string{"clientSecret", "giteaInternalUrl"} {
		require.True(t, r.InputProperties[field].Secret, field)
		require.True(t, r.Properties[field].Secret, field)
	}
	for _, field := range []string{"name", "giteaUrl", "clientId", "redirectUri", "organizationName"} {
		require.False(t, r.InputProperties[field].Secret, field)
		require.False(t, r.Properties[field].Secret, field)
	}
	for _, field := range []string{"giteaId", "gitProviderId", "organizationId"} {
		require.NotEmpty(t, r.Properties[field].Description, field)
		require.False(t, r.Properties[field].Secret, field)
		require.NotContains(t, r.InputProperties, field)
	}
	for _, field := range []string{"ready", "readiness", "token", "accessToken", "refreshToken", "expiresAt", "username", "scopes", "isShared", "gitProviderId"} {
		require.NotContains(t, r.InputProperties, field)
	}
	for _, field := range []string{"ready", "readiness", "token", "accessToken", "refreshToken", "expiresAt", "username", "scopes", "isShared", "parentId"} {
		require.NotContains(t, r.Properties, field)
	}
}

func TestGiteaIntegrationPreviewIdentity(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "giteaUrl": property.New("https://gitea.example"),
		"clientId": property.New("client"), "clientSecret": property.New("secret").WithSecret(true),
		"redirectUri":      property.New("https://dokploy.example/callback"),
		"giteaInternalUrl": property.New("https://internal.example").WithSecret(true),
		"organizationName": property.New("team"),
	})
	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	created, err := server.Create(p.CreateRequest{Urn: lifecycleURN("GiteaIntegration", "preview-gitea"), Properties: inputs, DryRun: true})
	require.NoError(t, err)
	for _, key := range []string{"giteaId", "gitProviderId", "organizationId"} {
		value := created.Properties.Get(key)
		require.True(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
	}
	for _, key := range []string{"clientSecret", "giteaInternalUrl"} {
		require.True(t, created.Properties.Get(key).Secret(), key)
	}
	for _, key := range []string{"name", "giteaUrl", "clientId", "redirectUri", "organizationName"} {
		require.False(t, created.Properties.Get(key).Secret(), key)
	}

	state := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "giteaUrl": property.New("https://gitea.example"),
		"clientId": property.New("client"), "clientSecret": property.New("secret").WithSecret(true),
		"redirectUri":      property.New("https://dokploy.example/callback"),
		"giteaInternalUrl": property.New("https://internal.example").WithSecret(true),
		"organizationName": property.New("team"), "giteaId": property.New("42"),
		"gitProviderId": property.New("parent"), "organizationId": property.New("org"),
	})
	updatedInputs := property.NewMap(map[string]property.Value{
		"name": property.New("renamed"), "giteaUrl": property.New("https://gitea.example/new"),
		"clientId": property.New(property.Computed), "clientSecret": property.New("rotated").WithSecret(true),
		"redirectUri":      property.New("https://dokploy.example/new-callback"),
		"giteaInternalUrl": property.New("https://internal-new.example").WithSecret(true),
		"organizationName": property.New(property.Computed),
	})
	updated, err := server.Update(p.UpdateRequest{
		ID: "42", Urn: lifecycleURN("GiteaIntegration", "preview-gitea"), State: state,
		OldInputs: inputs, Inputs: updatedInputs, DryRun: true,
	})
	require.NoError(t, err)
	for key, want := range map[string]string{"giteaId": "42", "gitProviderId": "parent", "organizationId": "org"} {
		value := updated.Properties.Get(key)
		require.False(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
		require.Equal(t, want, value.AsString(), key)
	}
	require.Equal(t, "renamed", updated.Properties.Get("name").AsString())
	require.Equal(t, "https://gitea.example/new", updated.Properties.Get("giteaUrl").AsString())
	require.True(t, updated.Properties.Get("clientSecret").Secret())
	require.True(t, updated.Properties.Get("giteaInternalUrl").Secret())
	for _, key := range []string{"name", "giteaUrl", "clientId", "redirectUri", "organizationName"} {
		require.False(t, updated.Properties.Get(key).Secret(), key)
	}
}

func TestGiteaIntegrationInferPartialState(t *testing.T) {
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
				case "/api/gitea.create":
					fmt.Fprint(w, `{"giteaId":"42","clientId":"client","giteaUrl":"https://gitea.example"}`)
				}
			}))
			defer server.Close()
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/gitea.create" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			resource := infer.Resource(&GiteaIntegration{client: func(context.Context) *client.Client { return api }})
			inputs := property.NewMap(map[string]property.Value{
				"name": property.New("display"), "giteaUrl": property.New("https://gitea.example"),
				"clientId": property.New("client"), "clientSecret": property.New("secret").WithSecret(true),
				"redirectUri": property.New("https://dokploy.example/callback"),
			})
			resp, err := resource.Create(ctx, p.CreateRequest{Urn: lifecycleURN("GiteaIntegration", "infer-partial-gitea"), Properties: inputs})
			require.ErrorIs(t, err, cause)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.NotNil(t, resp.PartialState)
			require.Equal(t, "42", resp.ID)
			require.Equal(t, "42", resp.Properties.Get("giteaId").AsString())
			require.False(t, resp.Properties.Get("giteaId").IsComputed())
			require.False(t, resp.Properties.Get("giteaId").Secret())
			require.Equal(t, "https://gitea.example", resp.Properties.Get("giteaUrl").AsString())
			require.Equal(t, "client", resp.Properties.Get("clientId").AsString())
			require.Empty(t, resp.Properties.Get("gitProviderId").AsString())
			require.Empty(t, resp.Properties.Get("organizationId").AsString())
			require.Equal(t, []string{"/api/organization.active", "/api/gitea.create"}, paths)
		})
	}
}

func TestGiteaIntegrationInferUpdateMutationCancellationReturnsPartialState(t *testing.T) {
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
			resource := infer.Resource(&GiteaIntegration{client: func(context.Context) *client.Client { return api }})
			oldInputs := property.NewMap(map[string]property.Value{
				"name": property.New("display"), "giteaUrl": property.New("https://gitea.example"),
				"clientId": property.New("client"), "clientSecret": property.New("secret").WithSecret(true),
				"redirectUri": property.New("https://dokploy.example/callback"), "organizationName": property.New(""),
			})
			state := property.NewMap(map[string]property.Value{
				"name": property.New("display"), "giteaUrl": property.New("https://gitea.example"),
				"clientId": property.New("client"), "clientSecret": property.New("secret").WithSecret(true),
				"redirectUri": property.New("https://dokploy.example/callback"), "organizationName": property.New(""),
				"giteaId": property.New("relation"), "gitProviderId": property.New("parent"), "organizationId": property.New("org"),
			})
			newInputs := oldInputs.Set("name", property.New("requested"))
			resp, err := resource.Update(ctx, p.UpdateRequest{
				ID: "relation", Urn: lifecycleURN("GiteaIntegration", "infer-update-partial-gitea"),
				State: state, OldInputs: oldInputs, Inputs: newInputs,
			})
			require.ErrorIs(t, err, cause)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.NotNil(t, resp.PartialState)
			for key, value := range map[string]string{"name": "display", "giteaId": "relation", "gitProviderId": "parent", "organizationId": "org"} {
				require.Equal(t, value, resp.Properties.Get(key).AsString(), key)
			}
			require.False(t, resp.Properties.Get("giteaId").IsComputed())
			require.False(t, resp.Properties.Get("giteaId").Secret())
			require.True(t, resp.Properties.Get("clientSecret").Secret())
			require.Equal(t, []string{"/api/organization.active", "/api/gitea.one", "/api/gitea.update"}, paths)
		})
	}
}
