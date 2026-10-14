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

func TestSchemaBitbucketIntegrationContract(t *testing.T) {
	spec := providerSchema(t)
	r, ok := spec.Resources["dokploy:index:BitbucketIntegration"]
	require.True(t, ok)
	require.ElementsMatch(t, []string{"name"}, r.RequiredInputs)
	require.ElementsMatch(t, []string{"name", "apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"}, objectKeys(r.InputProperties))
	require.ElementsMatch(t, []string{"name", "apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName", "bitbucketId", "gitProviderId", "organizationId"}, objectKeys(r.Properties))
	for _, field := range []string{"name", "apiToken", "appPassword", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"} {
		require.NotEmpty(t, r.InputProperties[field].Description, field)
		require.NotEmpty(t, r.Properties[field].Description, field)
		require.False(t, r.InputProperties[field].ReplaceOnChanges, field)
		require.False(t, r.Properties[field].ReplaceOnChanges, field)
	}
	for _, field := range []string{"apiToken", "appPassword"} {
		require.True(t, r.InputProperties[field].Secret, field)
		require.True(t, r.Properties[field].Secret, field)
	}
	for _, field := range []string{"name", "bitbucketEmail", "bitbucketUsername", "bitbucketWorkspaceName"} {
		require.False(t, r.InputProperties[field].Secret, field)
		require.False(t, r.Properties[field].Secret, field)
	}
	for _, field := range []string{"bitbucketId", "gitProviderId", "organizationId"} {
		require.NotEmpty(t, r.Properties[field].Description, field)
		require.False(t, r.Properties[field].Secret, field)
		require.NotContains(t, r.InputProperties, field)
	}
	for _, field := range []string{"ready", "readiness", "oauth", "session", "token", "authId", "owner", "isShared", "gitProviderId"} {
		require.NotContains(t, r.InputProperties, field)
	}
	for _, field := range []string{"ready", "readiness", "oauth", "session", "token", "authId", "owner", "isShared"} {
		require.NotContains(t, r.Properties, field)
	}
}

func TestBitbucketIntegrationPreviewIdentity(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "apiToken": property.New("api-secret").WithSecret(true),
		"bitbucketEmail": property.New("user@example.invalid"), "bitbucketWorkspaceName": property.New("workspace"),
	})
	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	created, err := server.Create(p.CreateRequest{Urn: lifecycleURN("BitbucketIntegration", "preview-bitbucket"), Properties: inputs, DryRun: true})
	require.NoError(t, err)
	for _, key := range []string{"bitbucketId", "gitProviderId", "organizationId"} {
		value := created.Properties.Get(key)
		require.True(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
	}
	require.True(t, created.Properties.Get("apiToken").Secret())
	require.False(t, created.Properties.Get("name").Secret())

	state := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "apiToken": property.New("api-secret").WithSecret(true),
		"bitbucketEmail": property.New("user@example.invalid"), "bitbucketWorkspaceName": property.New("workspace"),
		"bitbucketId": property.New("relation"), "gitProviderId": property.New("parent"), "organizationId": property.New("org"),
	})
	updatedInputs := property.NewMap(map[string]property.Value{
		"name": property.New("renamed"), "appPassword": property.New("app-secret").WithSecret(true),
		"bitbucketUsername": property.New("legacy-user"), "bitbucketWorkspaceName": property.New(property.Computed),
	})
	updated, err := server.Update(p.UpdateRequest{
		ID: "relation", Urn: lifecycleURN("BitbucketIntegration", "preview-bitbucket"), State: state,
		OldInputs: inputs, Inputs: updatedInputs, DryRun: true,
	})
	require.NoError(t, err)
	for key, want := range map[string]string{"bitbucketId": "relation", "gitProviderId": "parent", "organizationId": "org"} {
		value := updated.Properties.Get(key)
		require.False(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
		require.Equal(t, want, value.AsString(), key)
	}
	require.Equal(t, "renamed", updated.Properties.Get("name").AsString())
	require.False(t, updated.Properties.Get("name").Secret(), "ordinary known values must not inherit secret dependencies")
	require.True(t, updated.Properties.Get("bitbucketWorkspaceName").IsComputed(), "computed optional input remains computed")
	require.False(t, updated.Properties.Get("bitbucketWorkspaceName").Secret())
	require.True(t, updated.Properties.Get("appPassword").Secret())
	require.False(t, updated.Properties.Get("bitbucketUsername").Secret())
}

func TestBitbucketIntegrationInferPartialState(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run("create/"+cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			fixture := newBitbucketCreateFixture(t)
			fixture.onRename = func() { ctx.cancel(cause) }
			api := fixture.api()
			resource := infer.Resource(&BitbucketIntegration{client: func(context.Context) *client.Client { return api }})
			inputs := property.NewMap(map[string]property.Value{
				"name": property.New("requested"), "apiToken": property.New("api-secret").WithSecret(true),
				"bitbucketEmail": property.New("user@example.invalid"), "bitbucketUsername": property.New("user"),
				"bitbucketWorkspaceName": property.New("workspace"),
			})
			resp, err := resource.Create(ctx, p.CreateRequest{Urn: lifecycleURN("BitbucketIntegration", "infer-create-partial"), Properties: inputs})
			require.ErrorIs(t, err, cause)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.NotNil(t, resp.PartialState)
			require.Equal(t, "relation", resp.ID)
			require.Equal(t, "relation", resp.Properties.Get("bitbucketId").AsString())
			require.Equal(t, "parent", resp.Properties.Get("gitProviderId").AsString())
			require.Equal(t, "org", resp.Properties.Get("organizationId").AsString())
			require.False(t, resp.Properties.Get("bitbucketId").Secret())
			require.True(t, resp.Properties.Get("apiToken").Secret())
			require.Equal(t, 1, fixture.postLists, "cancellation after rename must not restart discovery")
		})

		t.Run("update/"+cause.Error(), func(t *testing.T) {
			ctx := newLifecycleCancelContext()
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/organization.active":
					fmt.Fprint(w, `{"id":"org"}`)
				case "/api/bitbucket.one":
					fmt.Fprint(w, bitbucketRecord)
				case "/api/bitbucket.update":
					fmt.Fprint(w, `{}`)
				}
			}))
			t.Cleanup(server.Close)
			transport := lifecycleRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.URL.Path == "/api/bitbucket.update" {
					ctx.cancel(cause)
				}
				return resp, err
			})
			api, err := client.New(server.URL, "fixture", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
			require.NoError(t, err)
			resource := infer.Resource(&BitbucketIntegration{client: func(context.Context) *client.Client { return api }})
			oldInputs := property.NewMap(map[string]property.Value{
				"name": property.New("old"), "apiToken": property.New("prior-api").WithSecret(true),
				"bitbucketEmail": property.New("user@example.invalid"), "bitbucketWorkspaceName": property.New("old-workspace"),
			})
			state := oldInputs.Set("bitbucketId", property.New("relation")).Set("gitProviderId", property.New("parent")).Set("organizationId", property.New("org"))
			newInputs := oldInputs.Set("name", property.New("new"))
			resp, err := resource.Update(ctx, p.UpdateRequest{ID: "relation", Urn: lifecycleURN("BitbucketIntegration", "infer-update-partial"), State: state, OldInputs: oldInputs, Inputs: newInputs})
			require.ErrorIs(t, err, cause)
			var initFailed infer.ResourceInitFailedError
			require.ErrorAs(t, err, &initFailed)
			require.NotNil(t, resp.PartialState)
			for key, want := range map[string]string{"name": "old", "bitbucketId": "relation", "gitProviderId": "parent", "organizationId": "org"} {
				require.Equal(t, want, resp.Properties.Get(key).AsString(), key)
			}
			require.False(t, resp.Properties.Get("bitbucketId").IsComputed())
			require.False(t, resp.Properties.Get("bitbucketId").Secret())
			require.True(t, resp.Properties.Get("apiToken").Secret())
			require.Equal(t, []string{"/api/organization.active", "/api/bitbucket.one", "/api/bitbucket.update"}, paths)
		})
	}
}
