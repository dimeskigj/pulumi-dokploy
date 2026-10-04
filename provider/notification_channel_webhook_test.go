package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationChannelBodiesWebhook(t *testing.T) {
	for _, kind := range []string{notificationSlack, notificationDiscord, notificationMattermost, notificationCustom, notificationLark, notificationTeams} {
		a := normalizeNotificationArgs(notificationTestArgs(kind))
		a.Name = "requested-name"
		a.Events = &NotificationEvents{true, true, true, true, true, true, true, true}
		var create, update any
		settings := map[string]any{"webhookUrl": "https://example.com/hook"}
		switch kind {
		case notificationSlack:
			create, update = notificationSlackCreateBody("requested-name", a), notificationSlackUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings["channel"] = ""
		case notificationDiscord:
			create, update = notificationDiscordCreateBody("requested-name", a), notificationDiscordUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings["decoration"] = false
		case notificationMattermost:
			create, update = notificationMattermostCreateBody("requested-name", a), notificationMattermostUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings["channel"], settings["username"] = "", ""
		case notificationCustom:
			a.Custom.Headers = map[string]string{}
			create, update = notificationCustomCreateBody("requested-name", a), notificationCustomUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings = map[string]any{"endpoint": "https://example.com/hook", "headers": map[string]any{}}
		case notificationLark:
			create, update = notificationLarkCreateBody("requested-name", a), notificationLarkUpdateBody("placeholder-notification", "placeholder-channel", a)
		case notificationTeams:
			create, update = notificationTeamsCreateBody("requested-name", a), notificationTeamsUpdateBody("placeholder-notification", "placeholder-channel", a)
		}
		notificationBodyCase(t, kind, create, update, settings, kind+"Id", true)
	}
}

func TestNotificationChannelCustomHeaders(t *testing.T) {
	a := notificationTestArgs(notificationCustom)
	a.Custom.Headers = map[string]string{"X.A[B]": "token-like-placeholder"}
	for _, body := range []any{notificationCustomCreateBody("requested-name", a), notificationCustomUpdateBody("placeholder-notification", "placeholder-channel", a)} {
		fields := notificationBodyMap(t, body)
		require.Equal(t, map[string]any{"X.A[B]": "token-like-placeholder"}, fields["headers"])
	}
}
