package dokploy

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func readProjectFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func TestRegistryDocumentation(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	installation := readProjectFile(t, "../docs/installation-configuration.md")

	require.True(t, strings.HasPrefix(index, "---\n"))
	for _, marker := range []string{"title: Dokploy", "Pulumi", "community-maintained", "## Installation", "## Configuration", "## Example"} {
		require.Contains(t, index, marker)
	}
	for _, marker := range []string{"@dimeskigj/pulumi-dokploy", "pulumi_dokploy", "Dimeskigj.Pulumi.Dokploy", "net.dimeski.pulumi:dokploy", "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy", "dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY"} {
		require.Contains(t, installation, marker)
	}
	require.NotContains(t, index+installation, "official Dokploy")
	require.NotContains(t, index+installation, "official Pulumi")
}

func TestRegistryFrontMatterAndSupportLinks(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	installation := readProjectFile(t, "../docs/installation-configuration.md")
	for name, content := range map[string]string{"index": index, "installation": installation} {
		t.Run(name, func(t *testing.T) {
			parts := strings.SplitN(content, "---\n", 3)
			require.Len(t, parts, 3)
			frontMatter := map[string]string{}
			require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &frontMatter))
			require.Equal(t, "package", frontMatter["layout"])
			require.NotEmpty(t, frontMatter["meta_desc"])
			require.NotEmpty(t, frontMatter["title"])
		})
	}
	for _, link := range []string{
		"https://github.com/dimeskigj/pulumi-dokploy",
		"https://github.com/dimeskigj/pulumi-dokploy/issues",
		"https://github.com/dimeskigj/pulumi-dokploy/blob/main/CONTRIBUTING.md",
	} {
		require.Contains(t, index, link)
	}
}

func TestRegistryInstallationDetails(t *testing.T) {
	installation := readProjectFile(t, "../docs/installation-configuration.md")
	require.Regexp(t, regexp.MustCompile(`(?s)<groupId>net\.dimeski\.pulumi</groupId>\s*<artifactId>dokploy</artifactId>\s*<version>\$\{DOKPLOY_VERSION\}</version>`), installation)
	require.Contains(t, installation, "pluginDownloadURL")
	require.Contains(t, installation, "github://api.github.com/dimeskigj/pulumi-dokploy")
}

func TestRegistryExampleDoesNotExposeSecrets(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	start := strings.Index(index, "```typescript")
	end := strings.Index(index[start+len("```typescript"):], "```")
	require.NotEqual(t, -1, start)
	require.NotEqual(t, -1, end)
	example := index[start : start+len("```typescript")+end]
	require.NotContains(t, example, "apiKey")
	require.NotContains(t, example, "DOKPLOY_API_KEY")
}

func TestRegistryLicense(t *testing.T) {
	license := readProjectFile(t, "../LICENSE")
	require.Contains(t, license, "TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION")
	for _, section := range []string{"1. Definitions.", "2. Grant of Copyright License.", "3. Grant of Patent License.", "4. Redistribution.", "5. Submission of Contributions.", "6. Trademarks.", "7. Disclaimer of Warranty.", "8. Limitation of Liability.", "9. Accepting Warranty or Additional Liability."} {
		require.Contains(t, license, section)
	}
	require.Contains(t, license, "APPENDIX: How to apply the Apache License to your work")
}
