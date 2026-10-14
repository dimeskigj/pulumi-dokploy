package dokploy

import (
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntegrationPreviewMethodsDoNotCallClient(t *testing.T) {
	r := GitHubIntegration{client: func(_ context.Context) *client.Client { t.Fatal("preview called client"); return nil }}
	inputs := GitHubIntegrationArgs{Name: "example", GitHubAppName: "example-app"}
	created, err := r.Create(t.Context(), infer.CreateRequest[GitHubIntegrationArgs]{Inputs: inputs, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, inputs, created.Output.GitHubIntegrationArgs)
	for _, id := range []string{created.Output.GitHubID, created.Output.GitProviderID, created.Output.OrganizationID} {
		require.Empty(t, id)
	}
	prior := GitHubIntegrationState{GitHubIntegrationArgs: inputs, GitHubID: "relation-1", GitProviderID: "parent-1", OrganizationID: "org-1"}
	updatedArgs := GitHubIntegrationArgs{Name: "renamed", GitHubAppName: "renamed-app"}
	updated, err := r.Update(t.Context(), infer.UpdateRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation-1", Inputs: updatedArgs, State: prior, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, updatedArgs, updated.Output.GitHubIntegrationArgs)
	require.Equal(t, prior.GitHubID, updated.Output.GitHubID)
	require.Equal(t, prior.GitProviderID, updated.Output.GitProviderID)
	require.Equal(t, prior.OrganizationID, updated.Output.OrganizationID)
}

func TestGitHubIntegrationPreviewIdentity(t *testing.T) {
	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("GitHubIntegration", "preview-github")
	inputs := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "githubAppName": property.New("example-app"),
	})
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: inputs, DryRun: true})
	require.NoError(t, err)
	for _, key := range []string{"githubId", "gitProviderId", "organizationId"} {
		value := created.Properties.Get(key)
		require.True(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
	}

	state := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "githubAppName": property.New("example-app"),
		"githubId": property.New("stable-relation"), "gitProviderId": property.New("stable-parent"),
		"organizationId": property.New("stable-org"),
	})
	newInputs := property.NewMap(map[string]property.Value{
		"name": property.New("renamed"), "githubAppName": property.New("renamed-app"),
	})
	updated, err := server.Update(p.UpdateRequest{
		ID: "stable-relation", Urn: urn, State: state, OldInputs: inputs, Inputs: newInputs, DryRun: true,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Properties.Get("name").AsString())
	require.Equal(t, "renamed-app", updated.Properties.Get("githubAppName").AsString())
	for key, want := range map[string]string{
		"githubId": "stable-relation", "gitProviderId": "stable-parent", "organizationId": "stable-org",
	} {
		value := updated.Properties.Get(key)
		require.False(t, value.IsComputed(), "%s: %#v", key, value)
		require.False(t, value.Secret(), key)
		require.Equal(t, want, value.AsString())
	}
}
