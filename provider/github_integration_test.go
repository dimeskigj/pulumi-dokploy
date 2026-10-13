package dokploy

import (
	"context"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntegrationCheckKnownAndComputed(t *testing.T) {
	r := GitHubIntegration{}
	for _, tc := range []struct {
		name     string
		inputs   map[string]any
		property string
	}{
		{"empty name with computed app", map[string]any{"name": "", "githubAppName": nil}, "name"},
		{"empty app with computed name", map[string]any{"name": nil, "githubAppName": ""}, "githubAppName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]property.Value{}
			for key, value := range tc.inputs {
				if value == nil {
					values[key] = property.New(property.Computed)
				} else {
					values[key] = property.New(value.(string))
				}
			}
			resp, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(values)})
			require.NoError(t, err)
			require.Contains(t, []string{resp.Failures[0].Property}, tc.property)
		})
	}
}

func TestGitHubIntegrationDiff(t *testing.T) {
	got, err := (GitHubIntegration{}).Diff(t.Context(), infer.DiffRequest[GitHubIntegrationArgs, GitHubIntegrationState]{Inputs: GitHubIntegrationArgs{Name: "new", GitHubAppName: "new-app"}, State: GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "old-app"}}})
	require.NoError(t, err)
	require.True(t, got.HasChanges)
	require.Equal(t, p.Update, got.DetailedDiff["name"].Kind)
	require.Equal(t, p.Update, got.DetailedDiff["githubAppName"].Kind)
}

func TestGitHubIntegrationCreateRejectedNoAPI(t *testing.T) {
	calls := 0
	_, err := (GitHubIntegration{client: func(context.Context) *client.Client { calls++; return nil }}).Create(t.Context(), infer.CreateRequest[GitHubIntegrationArgs]{Inputs: GitHubIntegrationArgs{Name: "n", GitHubAppName: "a"}})
	require.EqualError(t, err, "GitHubIntegration cannot create an integration; complete GitHub App setup and authorization in Dokploy, then import the GitHub relation ID")
	require.Zero(t, calls)
}

func TestGitHubIntegrationCreatePreviewHasUnresolvedIdentity(t *testing.T) {
	created, err := (GitHubIntegration{}).Create(t.Context(), infer.CreateRequest[GitHubIntegrationArgs]{Inputs: GitHubIntegrationArgs{Name: "n", GitHubAppName: "a"}, DryRun: true})
	require.NoError(t, err)
	require.Empty(t, created.ID)
	require.Empty(t, created.Output.GitHubID)
	require.Empty(t, created.Output.GitProviderID)
	require.Empty(t, created.Output.OrganizationID)
}

func TestGitHubIntegrationPreviewState(t *testing.T) {
	prior := GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "old-app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}
	got := gitHubIntegrationPreviewState(GitHubIntegrationArgs{Name: "new", GitHubAppName: "new-app"}, "relation", prior)
	require.Equal(t, "new", got.Name)
	require.Equal(t, "new-app", got.GitHubAppName)
	require.Equal(t, "relation", got.GitHubID)
	require.Equal(t, "parent", got.GitProviderID)
	require.Equal(t, "org", got.OrganizationID)
}
