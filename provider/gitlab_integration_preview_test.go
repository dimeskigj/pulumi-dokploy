package dokploy

import (
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestGitLabIntegrationPreviewIdentity(t *testing.T) {
	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("GitLabIntegration", "preview-gitlab")
	inputs := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "applicationId": property.New("client"),
		"applicationSecret": property.New("secret").WithSecret(true),
		"redirectUri":       property.New("https://example.test/callback"),
		"gitlabUrl":         property.New("https://gitlab.com"),
	})
	created, err := server.Create(p.CreateRequest{Urn: urn, Properties: inputs, DryRun: true})
	require.NoError(t, err)
	for _, key := range []string{"gitlabId", "gitProviderId", "organizationId", "isConfigured"} {
		value := created.Properties.Get(key)
		require.True(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
	}
	state := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "applicationId": property.New("client"),
		"applicationSecret": property.New("secret").WithSecret(true),
		"redirectUri":       property.New("https://example.test/callback"), "gitlabUrl": property.New("https://gitlab.com"),
		"gitlabId": property.New("stable-relation"), "gitProviderId": property.New("stable-parent"),
		"organizationId": property.New("stable-org"), "isConfigured": property.New(false),
	})
	newInputs := property.NewMap(map[string]property.Value{
		"name": property.New("example"), "applicationId": property.New("client"),
		"applicationSecret": property.New("rotated").WithSecret(true),
		"redirectUri":       property.New("https://example.test/callback"), "gitlabUrl": property.New("https://gitlab.com"),
	})
	updated, err := server.Update(p.UpdateRequest{ID: "stable-relation", Urn: urn, State: state, OldInputs: inputs, Inputs: newInputs, DryRun: true})
	require.NoError(t, err)
	for key, want := range map[string]string{"gitlabId": "stable-relation", "gitProviderId": "stable-parent", "organizationId": "stable-org"} {
		value := updated.Properties.Get(key)
		require.False(t, value.IsComputed(), key)
		require.False(t, value.Secret(), key)
		require.Equal(t, want, value.AsString())
	}
	readiness := updated.Properties.Get("isConfigured")
	require.True(t, readiness.IsComputed())
	require.False(t, readiness.Secret())
	nameChanged := property.NewMap(map[string]property.Value{
		"name": property.New("renamed"), "applicationId": property.New("client"),
		"applicationSecret": property.New("secret").WithSecret(true),
		"redirectUri":       property.New("https://example.test/callback"), "gitlabUrl": property.New("https://gitlab.com"),
	})
	renamed, err := server.Update(p.UpdateRequest{ID: "stable-relation", Urn: urn, State: state, OldInputs: inputs, Inputs: nameChanged, DryRun: true})
	require.NoError(t, err)
	for _, key := range []string{"gitlabId", "gitProviderId", "organizationId"} {
		require.False(t, renamed.Properties.Get(key).IsComputed(), key)
		require.False(t, renamed.Properties.Get(key).Secret(), key)
	}
	require.True(t, renamed.Properties.Get("isConfigured").IsComputed())
}
