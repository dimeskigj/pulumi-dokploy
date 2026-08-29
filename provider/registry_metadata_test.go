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
	require.Equal(t, "@gjorgjidimeski/pulumi-dokploy", languageSetting(spec, "nodejs", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "moduleName"))
	require.Equal(t, "github.com/gjorgjidimeski/pulumi-dokploy/sdk/go/dokploy", languageSetting(spec, "go", "importBasePath"))
	require.Equal(t, "Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
	require.Equal(t, "dev.codechem.pulumi.dokploy", languageSetting(spec, "java", "packageName"))
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
		"pulumi config set dokploy:endpoint https://dokploy.example.com",
		"pulumi config set --secret dokploy:apiKey \"$DOKPLOY_API_KEY\"",
		"Project owns the default environment", "Source type changes replace",
		"referenced GitLab integration is not managed", "SSH key references are likewise passed through",
		"deployment errors preserve partial state", "Compose volumes are preserved",
	} {
		require.True(t, strings.Contains(docs, section), "README missing %q", section)
	}

	for _, workflow := range []string{"build.yml", "release.yml", "prerelease.yml", "run-acceptance-tests.yml"} {
		// #nosec G304 -- workflow names come from the fixed table above.
		content, err := os.ReadFile("../.github/workflows/" + workflow)
		require.NoError(t, err)
		text := string(content)
		require.NotContains(t, text, "paths-ignore:\n    - \"**\"", workflow)
		require.NotContains(t, text, "pulumi/action-release-by-pr-label@main", workflow)
	}
	release, err := os.ReadFile("../.github/workflows/release.yml")
	require.NoError(t, err)
	releaseText := string(release)
	require.Contains(t, releaseText, "-f .goreleaser.yml")
	require.Contains(t, releaseText, "goreleaser/goreleaser-action@")
	require.NotContains(t, releaseText, "-f .goreleaser.prerelease.yml")
	acceptance, err := os.ReadFile("../.github/workflows/run-acceptance-tests.yml")
	require.NoError(t, err)
	acceptanceText := string(acceptance)
	require.NotContains(t, acceptanceText, "  pull_request:\n")
	require.Contains(t, acceptanceText, "environment: dokploy-acceptance")
	require.Contains(t, acceptanceText, "DOKPLOY_ENDPOINT")
	require.Contains(t, acceptanceText, "DOKPLOY_API_KEY")
}
