package dokploy

import "testing"

func TestNotificationChannelBodiesEmail(t *testing.T) {
	for _, kind := range []string{notificationEmail, notificationResend} {
		a := normalizeNotificationArgs(notificationTestArgs(kind))
		a.Name = "requested-name"
		a.Events = &NotificationEvents{true, true, true, true, true, true, true, true}
		var create, update any
		settings := map[string]any{"fromAddress": "sender@example.com", "toAddresses": []any{"recipient@example.com"}}
		switch kind {
		case notificationEmail:
			create, update = notificationEmailCreateBody("requested-name", a), notificationEmailUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings["smtpServer"], settings["smtpPort"], settings["username"], settings["password"] = "example.com", float64(25), "placeholder", "placeholder"
		case notificationResend:
			create, update = notificationResendCreateBody("requested-name", a), notificationResendUpdateBody("placeholder-notification", "placeholder-channel", a)
			settings["apiKey"] = "placeholder"
		}
		notificationBodyCase(t, kind, create, update, settings, kind+"Id", true)
	}
}
