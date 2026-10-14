package dokploy

import (
	"context"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntegrationPreviewIdentity(t *testing.T) {
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
