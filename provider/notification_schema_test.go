package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaNotificationContract(t *testing.T) {
	spec := providerSchema(t)
	r, ok := spec.Resources["dokploy:index:Notification"]
	require.True(t, ok, "Notification token must be registered")
	require.Contains(t, spec.Description, "notifications")
	require.Equal(t, []string{"name"}, r.RequiredInputs)
	require.ElementsMatch(t, []string{"name", "events", "slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"}, keysOfNotificationInputs(r.InputProperties))
	for _, field := range []string{"notificationId", "notificationType", "channelId", "organizationId"} {
		require.NotContains(t, r.InputProperties, field)
		require.Contains(t, r.Required, field)
		require.NotEmpty(t, r.Properties[field].Description)
	}
	for _, field := range []string{"notificationType", "enabled"} {
		require.NotContains(t, r.InputProperties, field)
	}
	for _, field := range []string{"events", "slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"} {
		input := r.InputProperties[field]
		require.NotEmpty(t, input.Description, field)
		require.False(t, input.ReplaceOnChanges, field)
		require.NotEmpty(t, input.Ref, field)
		nested := spec.Types[trimTypeRef(input.Ref)]
		require.NotEmpty(t, nested.Properties, field)
		for name, prop := range nested.Properties {
			require.NotEmpty(t, prop.Description, field+"."+name)
		}
	}
	events := spec.Types[trimTypeRef(r.InputProperties["events"].Ref)]
	require.ElementsMatch(t, []string{"appDeploy", "appBuildError", "databaseBackup", "volumeBackup", "dokployBackup", "dokployRestart", "dockerCleanup", "serverThreshold"}, keysOfNotificationInputs(events.Properties))
	for _, event := range events.Properties {
		require.Equal(t, "boolean", event.Type)
		require.Equal(t, false, event.Default)
	}
	fields := map[string][]string{
		"slack":      {"webhookUrl", "channel"},
		"telegram":   {"botToken", "chatId", "messageThreadId"},
		"discord":    {"webhookUrl", "decoration"},
		"email":      {"smtpServer", "smtpPort", "username", "password", "fromAddress", "toAddresses"},
		"resend":     {"apiKey", "fromAddress", "toAddresses"},
		"gotify":     {"serverUrl", "appToken", "priority", "decoration"},
		"ntfy":       {"serverUrl", "topic", "accessToken", "priority"},
		"mattermost": {"webhookUrl", "channel", "username"},
		"custom":     {"endpoint", "headers"},
		"lark":       {"webhookUrl"}, "teams": {"webhookUrl"},
		"pushover": {"userKey", "apiToken", "priority", "retry", "expire"},
	}
	for channel, want := range fields {
		typeSpec := spec.Types[trimTypeRef(r.InputProperties[channel].Ref)]
		require.ElementsMatch(t, want, keysOfNotificationInputs(typeSpec.Properties), channel)
	}
	defaults := map[string]map[string]any{
		"slack": {"channel": ""}, "telegram": {"messageThreadId": ""},
		"discord":    {"decoration": false},
		"gotify":     {"priority": float64(5), "decoration": false},
		"ntfy":       {"accessToken": "", "priority": float64(3)},
		"mattermost": {"channel": "", "username": ""},
		"pushover":   {"priority": float64(0)},
	}
	require.Nil(t, spec.Types[trimTypeRef(r.InputProperties["custom"].Ref)].Properties["headers"].Default,
		"Pulumi schema does not support map constant defaults; Check supplies an empty map")
	require.Contains(t, spec.Types[trimTypeRef(r.InputProperties["custom"].Ref)].Properties["headers"].Description, "empty map")
	for channel := range fields {
		want := defaults[channel]
		props := spec.Types[trimTypeRef(r.InputProperties[channel].Ref)].Properties
		for field, value := range want {
			require.Equal(t, value, props[field].Default, channel+"."+field)
		}
		for field, prop := range props {
			if _, ok := want[field]; !ok {
				require.Nil(t, prop.Default, channel+"."+field)
			}
		}
	}
	for channel, fields := range map[string][]string{
		"slack": {"webhookUrl"}, "telegram": {"botToken"}, "discord": {"webhookUrl"},
		"email": {"password"}, "resend": {"apiKey"}, "gotify": {"serverUrl", "appToken"},
		"ntfy": {"serverUrl", "accessToken"}, "mattermost": {"webhookUrl"},
		"custom": {"endpoint", "headers"}, "lark": {"webhookUrl"}, "teams": {"webhookUrl"},
		"pushover": {"userKey", "apiToken"},
	} {
		typeSpec := spec.Types[trimTypeRef(r.InputProperties[channel].Ref)]
		for _, field := range fields {
			require.True(t, typeSpec.Properties[field].Secret, channel+"."+field)
		}
	}
	for channel, field := range map[string]string{"gotify": "priority", "ntfy": "priority", "pushover": "priority", "custom": "headers"} {
		require.Contains(t, spec.Types[trimTypeRef(r.InputProperties[channel].Ref)].Properties, field)
	}
	for _, field := range []string{"retry", "expire"} {
		require.NotContains(t, spec.Types[trimTypeRef(r.InputProperties["pushover"].Ref)].Required, field)
	}
}

func keysOfNotificationInputs[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
