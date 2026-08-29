package dokploy

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryMetadata(t *testing.T) {
	spec := providerSchema(t)
	require.Equal(t, "https://github.com/gjorgjidimeski/pulumi-dokploy", spec.Repository)
	require.Equal(t, "Apache-2.0", spec.License)
	require.Equal(t, "gjorgjidimeski", spec.Publisher)
	require.NotEmpty(t, spec.Description)
	for token, resource := range spec.Resources {
		require.NotEmpty(t, resource.Description, token)
	}

	readme, err := os.ReadFile("../README.md")
	require.NoError(t, err)
	docs := string(readme)
	for _, section := range []string{
		"Node.js", "Python", "Go", ".NET", "Java", "YAML",
		"dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY",
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:Redis", "dokploy:index:Domain",
		"GitLab", "pulumi import dokploy:index:Project", "wait", "secret",
		"SSH", "volume", "partial state",
	} {
		require.True(t, strings.Contains(docs, section), "README missing %q", section)
	}
}
