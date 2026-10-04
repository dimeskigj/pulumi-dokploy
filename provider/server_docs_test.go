package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerDocumentation(t *testing.T) {
	readme := readProjectFile(t, "../README.md")
	imports := readProjectFile(t, "../website/src/content/docs/guides/imports.mdx")
	guide := readProjectFile(t, "../website/src/content/docs/guides/servers.mdx")
	for _, text := range []string{readme, imports, guide} {
		require.Contains(t, text, "pulumi import dokploy:index:Server")
	}
	for _, marker := range []string{"enableDockerCleanup", "defaults to `false`", "bootstrap", "SSH", "server.serverId", "status", "in place", "workloads", "deployment history", "partial", "inactive", "does not destroy the VM"} {
		require.Contains(t, guide, marker)
	}
}
