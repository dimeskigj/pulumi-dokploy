package dokploy

import (
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestNotificationResolvedDefaultsAndValidation(t *testing.T) {
	for _, kind := range []string{"slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"} {
		a := normalizeNotificationArgs(notificationTestArgs(kind))
		require.Empty(t, validateNotificationArgs(a), kind)
	}
	a := normalizeNotificationArgs(notificationTestArgs("gotify"))
	require.Equal(t, 5, *a.Gotify.Priority)
	require.False(t, a.Events.ServerThreshold)
	a.Events.ServerThreshold = true
	require.NotEmpty(t, validateNotificationArgs(a))
	for _, tc := range []struct {
		name string
		args NotificationArgs
		path string
	}{
		{"none", NotificationArgs{Name: "example"}, "slack"},
		{"two", func() NotificationArgs {
			a := notificationTestArgs("slack")
			a.Teams = notificationTestArgs("teams").Teams
			return a
		}(), "teams"},
		{"name", NotificationArgs{Slack: notificationTestArgs("slack").Slack}, "name"},
		{"url", func() NotificationArgs { a := notificationTestArgs("slack"); a.Slack.WebhookURL = "relative"; return a }(), "slack.webhookUrl"},
		{"recipients", func() NotificationArgs {
			a := notificationTestArgs("email")
			a.Email.ToAddresses = []string{""}
			return a
		}(), "email.toAddresses[0]"},
		{"smtp", func() NotificationArgs { a := notificationTestArgs("email"); a.Email.SMTPPort = 65536; return a }(), "email.smtpPort"},
		{"priority", func() NotificationArgs { a := notificationTestArgs("ntfy"); a.Ntfy.Priority = ptr(6); return a }(), "ntfy.priority"},
		{"emergency", func() NotificationArgs { a := notificationTestArgs("pushover"); a.Pushover.Priority = ptr(2); return a }(), "pushover.retry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := validateNotificationArgs(normalizeNotificationArgs(tc.args))
			require.NotEmpty(t, failures)
			require.Contains(t, failurePaths(failures), tc.path)
		})
	}
}

func failurePaths(f []p.CheckFailure) []string {
	paths := make([]string, 0, len(f))
	for _, v := range f {
		paths = append(paths, v.Property)
	}
	return paths
}

func TestNotificationCheckNestedUnknowns(t *testing.T) {
	r := notificationTestResource(t, func(http.ResponseWriter, *http.Request) { t.Error("check must not call API") })
	require.NotNil(t, r.client)
	for _, inputs := range []map[string]property.Value{
		{"name": property.New("example"), "gotify": property.New(property.Computed)},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New(property.Computed)}), "events": property.New(map[string]property.Value{"serverThreshold": property.New(property.Computed)})},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder")}), "ntfy": property.New(property.Computed)},
		{"name": property.New("example"), "gotify": property.New(map[string]property.Value{"serverUrl": property.New("https://example.com"), "appToken": property.New("placeholder"), "priority": property.New(property.Computed)})},
	} {
		got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(inputs)})
		require.NoError(t, err)
		require.Empty(t, got.Failures)
		if block := inputs["gotify"]; block.IsMap() && block.AsMap().Get("priority").IsComputed() {
			require.Nil(t, got.Inputs.Gotify.Priority)
		}
	}
	two, err := (Notification{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("example"), "slack": property.New(map[string]property.Value{"webhookUrl": property.New("https://example.com")}), "teams": property.New(map[string]property.Value{"webhookUrl": property.New("https://example.com")})})})
	require.NoError(t, err)
	require.NotEmpty(t, two.Failures)
}
