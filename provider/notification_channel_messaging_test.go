package dokploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationChannelBodiesMessaging(t *testing.T) {
	for _, kind := range []string{notificationTelegram, notificationGotify, notificationNtfy, notificationPushover} {
		a := normalizeNotificationArgs(notificationTestArgs(kind))
		a.Name = "requested-name"
		a.Events = &NotificationEvents{true, true, true, true, true, true, true, kind != notificationGotify && kind != notificationNtfy}
		var create, update any
		settings := map[string]any{}
		switch kind {
		case notificationTelegram:
			create, update = notificationTelegramCreateBody("requested-name", a), notificationTelegramUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings = map[string]any{"botToken": "placeholder", "chatId": "placeholder", "messageThreadId": ""}
		case notificationGotify:
			create, update = notificationGotifyCreateBody("requested-name", a), notificationGotifyUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings = map[string]any{"serverUrl": "https://example.com", "appToken": "placeholder", "priority": float64(5), "decoration": false}
		case notificationNtfy:
			create, update = notificationNtfyCreateBody("requested-name", a), notificationNtfyUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings = map[string]any{"serverUrl": "https://example.com", "topic": "placeholder", "accessToken": "", "priority": float64(3)}
		case notificationPushover:
			create, update = notificationPushoverCreateBody("requested-name", a), notificationPushoverUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings = map[string]any{"userKey": "placeholder", "apiToken": "placeholder", "priority": float64(0), "retry": nil, "expire": nil}
		}
		notificationBodyCase(t, kind, create, update, settings, kind+"Id", kind != notificationGotify && kind != notificationNtfy)
	}
}

func TestNotificationChannelPushoverEmergencySettings(t *testing.T) {
	a := notificationTestArgs(notificationPushover)
	a.Pushover.Priority, a.Pushover.Retry, a.Pushover.Expire = ptr(2), ptr(30), ptr(10800)
	for _, body := range []any{notificationPushoverCreateBody("requested-name", a), notificationPushoverUpdateBody("placeholder-notification", "placeholder-channel", a)} {
		fields := notificationBodyMap(t, body)
		require.Equal(t, float64(2), fields["priority"])
		require.Equal(t, float64(30), fields["retry"])
		require.Equal(t, float64(10800), fields["expire"])
	}
}
