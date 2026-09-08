package dokploy

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestRegistryLicense(t *testing.T) {
	license := readProjectFile(t, "../LICENSE")
	require.Contains(t, license, "TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION")
	for _, section := range []string{"1. Definitions.", "2. Grant of Copyright License.", "3. Grant of Patent License.", "4. Redistribution.", "5. Submission of Contributions.", "6. Trademarks.", "7. Disclaimer of Warranty.", "8. Limitation of Liability.", "9. Accepting Warranty or Additional Liability."} {
		require.Contains(t, license, section)
	}
	require.Contains(t, license, "APPENDIX: How to apply the Apache License to your work")
}
