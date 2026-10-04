package dokploy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/stretchr/testify/require"
)

func notificationTestResource(t *testing.T, handler http.HandlerFunc) Notification {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "placeholder-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return Notification{client: fixedClient(api)}
}

func notificationTestArgs(kind string) NotificationArgs {
	a := NotificationArgs{Name: "example"}
	switch kind {
	case "slack":
		a.Slack = &NotificationSlackConfig{WebhookURL: "https://example.com/hook"}
	case "telegram":
		a.Telegram = &NotificationTelegramConfig{BotToken: "placeholder", ChatID: "placeholder"}
	case "discord":
		a.Discord = &NotificationDiscordConfig{WebhookURL: "https://example.com/hook"}
	case "email":
		a.Email = &NotificationEmailConfig{SMTPServer: "example.com", SMTPPort: 25, Username: "placeholder", Password: "placeholder", FromAddress: "sender@example.com", ToAddresses: []string{"recipient@example.com"}}
	case "resend":
		a.Resend = &NotificationResendConfig{APIKey: "placeholder", FromAddress: "sender@example.com", ToAddresses: []string{"recipient@example.com"}}
	case "gotify":
		a.Gotify = &NotificationGotifyConfig{ServerURL: "https://example.com", AppToken: "placeholder"}
	case "ntfy":
		a.Ntfy = &NotificationNtfyConfig{ServerURL: "https://example.com", Topic: "placeholder"}
	case "mattermost":
		a.Mattermost = &NotificationMattermostConfig{WebhookURL: "https://example.com/hook"}
	case "custom":
		a.Custom = &NotificationCustomConfig{Endpoint: "https://example.com/hook"}
	case "lark":
		a.Lark = &NotificationLarkConfig{WebhookURL: "https://example.com/hook"}
	case "teams":
		a.Teams = &NotificationTeamsConfig{WebhookURL: "https://example.com/hook"}
	case "pushover":
		a.Pushover = &NotificationPushoverConfig{UserKey: "placeholder", APIToken: "placeholder"}
	default:
		panic("unknown test channel")
	}
	return a
}
