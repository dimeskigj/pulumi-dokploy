package dokploy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationDocumentationCoverage(t *testing.T) {
	guide := readProjectFile(t, "../website/src/content/docs/guides/notifications.mdx")
	imports := readProjectFile(t, "../website/src/content/docs/guides/imports.mdx")
	secrets := readProjectFile(t, "../website/src/content/docs/concepts/secrets.mdx")
	readme := readProjectFile(t, "../README.md")
	for _, marker := range []string{"dokploy:index:Notification", "../../reference/notification/", "../../reference/types/", "temporary", "refresh", "orphan", "all events", "false", "Gotify", "Ntfy", "serverThreshold", "priority", "retry", "expire", "read/list/create/update", "names alone"} {
		require.Contains(t, guide, marker)
	}
	for _, text := range []string{imports, readme} {
		require.Contains(t, text, "dokploy:index:Notification")
		require.Contains(t, text, "<notification-id>")
	}
	require.Contains(t, secrets, "Notification")
	require.Contains(t, secrets, "pulumi config set --secret")
	for _, text := range []string{guide, imports, secrets, readme} {
		require.NotContains(t, text, "testConnection")
		require.NotContains(t, text, "DOKPLOY_ACCEPTANCE=1")
		require.NotContains(t, text, "https://hooks.slack.com/services/")
		require.NotContains(t, strings.ToLower(text), "send a test message")
	}
}
