package dokploy

import (
	"context"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestGiteaIntegrationPreviewMethodsDoNotCallClient(t *testing.T) {
	r := GiteaIntegration{client: func(context.Context) *client.Client {
		t.Fatal("preview called API client")
		return nil
	}}
	inputs := giteaArgs()
	created, err := r.Create(t.Context(), infer.CreateRequest[GiteaIntegrationArgs]{Inputs: inputs, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, inputs, created.Output.GiteaIntegrationArgs)
	for _, id := range []string{created.Output.GiteaID, created.Output.GitProviderID, created.Output.OrganizationID} {
		require.Empty(t, id)
	}
	prior := GiteaIntegrationState{GiteaIntegrationArgs: inputs, GiteaID: "42", GitProviderID: "parent", OrganizationID: "org"}
	updatedInputs := inputs
	updatedInputs.Name = "renamed"
	updated, err := r.Update(t.Context(), infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "42", State: prior, Inputs: updatedInputs, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "42", updated.Output.GiteaID)
	require.Equal(t, "parent", updated.Output.GitProviderID)
	require.Equal(t, "org", updated.Output.OrganizationID)
	require.Equal(t, updatedInputs, updated.Output.GiteaIntegrationArgs)
}

func TestGiteaIntegrationPreviewState(t *testing.T) {
	inputs := giteaArgs()
	created, err := (GiteaIntegration{}).Create(t.Context(), infer.CreateRequest[GiteaIntegrationArgs]{Inputs: inputs, DryRun: true})
	require.NoError(t, err)
	require.Empty(t, created.ID)
	require.Empty(t, created.Output.GiteaID)
	require.Empty(t, created.Output.GitProviderID)
	require.Empty(t, created.Output.OrganizationID)
	prior := GiteaIntegrationState{GiteaIntegrationArgs: inputs, GiteaID: "42", GitProviderID: "parent", OrganizationID: "org"}
	updated, err := (GiteaIntegration{}).Update(context.Background(), infer.UpdateRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "42", State: prior, Inputs: inputs, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "42", updated.Output.GiteaID)
	require.Equal(t, "parent", updated.Output.GitProviderID)
	require.Equal(t, "org", updated.Output.OrganizationID)
	require.Equal(t, inputs, updated.Output.GiteaIntegrationArgs)
}
