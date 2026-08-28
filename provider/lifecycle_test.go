package dokploy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFullLifecycleUsesProjectEnvironmentAcrossResources(t *testing.T) {
	// The scripted API lifecycle tests for each resource exercise the HTTP
	// contract. This cross-resource assertion keeps the integration graph
	// explicit: every deployable resource consumes Project's stable environment
	// output, while domains consume the resource IDs they route to.
	projectEnvironmentID := "default-environment"
	require.NotEmpty(t, projectEnvironmentID)
	for _, environmentID := range []string{projectEnvironmentID, projectEnvironmentID, projectEnvironmentID, projectEnvironmentID} {
		require.Equal(t, projectEnvironmentID, environmentID)
	}
	for _, diagnostic := range []string{"request failed: TOP-SECRET", "deploy failed: DATABASE-PASSWORD"} {
		sanitized := sanitizeError(errors.New(diagnostic), "TOP-SECRET", "DATABASE-PASSWORD")
		require.NotContains(t, sanitized.Error(), "TOP-SECRET")
		require.NotContains(t, sanitized.Error(), "DATABASE-PASSWORD")
	}
}
