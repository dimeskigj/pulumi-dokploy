package dokploy

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestNotificationDiffContract(t *testing.T) {
	old := normalizeNotificationArgs(notificationTestArgs("slack"))
	check := func(a NotificationArgs) infer.DiffResponse {
		t.Helper()
		d, err := (Notification{}).Diff(t.Context(), infer.DiffRequest[NotificationArgs, NotificationState]{Inputs: a, State: NotificationState{NotificationArgs: old}})
		require.NoError(t, err)
		return d
	}
	require.False(t, check(notificationTestArgs("slack")).HasChanges)
	changed := notificationTestArgs("slack")
	changed.Slack.WebhookURL = "https://example.com/rotated"
	changed.Name = "renamed"
	changed.Events = &NotificationEvents{AppDeploy: true}
	d := check(changed)
	for _, path := range []string{"name", "slack.webhookUrl", "events.appDeploy"} {
		require.Equal(t, p.Update, d.DetailedDiff[path].Kind, path)
	}
	replacement := check(notificationTestArgs("teams"))
	require.True(t, replacement.HasChanges)
	require.Equal(t, p.DeleteReplace, replacement.DetailedDiff["slack"].Kind)
	require.Equal(t, p.AddReplace, replacement.DetailedDiff["teams"].Kind)
}

func TestNotificationHeaderDiffEscapesKeys(t *testing.T) {
	old := normalizeNotificationArgs(notificationTestArgs("custom"))
	old.Custom.Headers = map[string]string{"X.Version": "placeholder", "X[Mode]": "placeholder", "Authorization": "placeholder"}
	next := normalizeNotificationArgs(old)
	next.Custom.Headers["X.Version"] = "different-placeholder"
	d, err := (Notification{}).Diff(t.Context(), infer.DiffRequest[NotificationArgs, NotificationState]{Inputs: next, State: NotificationState{NotificationArgs: old}})
	require.NoError(t, err)
	require.True(t, d.HasChanges)
	require.Contains(t, d.DetailedDiff, "custom.headers")
	require.Equal(t, p.Update, d.DetailedDiff["custom.headers"].Kind)
}
