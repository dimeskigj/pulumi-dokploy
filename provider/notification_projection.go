package dokploy

import "github.com/dimeskigj/pulumi-dokploy/internal/client/generated"

func notificationSlackArgsFrom(v *generated.NotificationSlack, prior *NotificationSlackConfig) (*NotificationSlackConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.WebhookURL
	}
	return &NotificationSlackConfig{WebhookURL: notificationSecret(v.WebhookUrl, old), Channel: ptr(notificationOptional(v.Channel, ""))}, nil
}
func notificationDiscordArgsFrom(v *generated.NotificationDiscord, prior *NotificationDiscordConfig) (*NotificationDiscordConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.WebhookURL
	}
	return &NotificationDiscordConfig{WebhookURL: notificationSecret(v.WebhookUrl, old), Decoration: ptr(notificationOptional(v.Decoration, false))}, nil
}
func notificationMattermostArgsFrom(v *generated.NotificationMattermost, prior *NotificationMattermostConfig) (*NotificationMattermostConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.WebhookURL
	}
	return &NotificationMattermostConfig{WebhookURL: notificationSecret(v.WebhookUrl, old), Channel: ptr(notificationOptional(v.Channel, "")), Username: ptr(notificationOptional(v.Username, ""))}, nil
}
func notificationCustomArgsFrom(v *generated.NotificationCustom, prior *NotificationCustomConfig) (*NotificationCustomConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.Endpoint
	}
	h := notificationOptional(v.Headers, map[string]string{})
	if h == nil {
		h = map[string]string{}
	}
	return &NotificationCustomConfig{Endpoint: notificationSecret(v.Endpoint, old), Headers: h}, nil
}
func notificationLarkArgsFrom(v *generated.NotificationLark, prior *NotificationLarkConfig) (*NotificationLarkConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.WebhookURL
	}
	return &NotificationLarkConfig{WebhookURL: notificationSecret(v.WebhookUrl, old)}, nil
}
func notificationTeamsArgsFrom(v *generated.NotificationTeams, prior *NotificationTeamsConfig) (*NotificationTeamsConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	old := ""
	if prior != nil {
		old = prior.WebhookURL
	}
	return &NotificationTeamsConfig{WebhookURL: notificationSecret(v.WebhookUrl, old)}, nil
}
func notificationTelegramArgsFrom(v *generated.NotificationTelegram, prior *NotificationTelegramConfig) (*NotificationTelegramConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	chat, err := notificationRequired(v.ChatId)
	if err != nil {
		return nil, err
	}
	old := ""
	if prior != nil {
		old = prior.BotToken
	}
	return &NotificationTelegramConfig{BotToken: notificationSecret(v.BotToken, old), ChatID: chat, MessageThreadID: ptr(notificationOptional(v.MessageThreadId, ""))}, nil
}
func notificationGotifyArgsFrom(v *generated.NotificationGotify, prior *NotificationGotifyConfig) (*NotificationGotifyConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	oldURL, oldToken := "", ""
	if prior != nil {
		oldURL, oldToken = prior.ServerURL, prior.AppToken
	}
	return &NotificationGotifyConfig{ServerURL: notificationSecret(v.ServerUrl, oldURL), AppToken: notificationSecret(v.AppToken, oldToken), Priority: ptr(notificationOptional(v.Priority, 5)), Decoration: ptr(notificationOptional(v.Decoration, false))}, nil
}
func notificationNtfyArgsFrom(v *generated.NotificationNtfy, prior *NotificationNtfyConfig) (*NotificationNtfyConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	topic, err := notificationRequired(v.Topic)
	if err != nil {
		return nil, err
	}
	old := ""
	if prior != nil {
		old = prior.ServerURL
	}
	return &NotificationNtfyConfig{ServerURL: notificationSecret(v.ServerUrl, old), Topic: topic, AccessToken: ptr(notificationOptional(v.AccessToken, "")), Priority: ptr(notificationOptional(v.Priority, 3))}, nil
}
func notificationPushoverArgsFrom(v *generated.NotificationPushover, prior *NotificationPushoverConfig) (*NotificationPushoverConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	user, token := "", ""
	if prior != nil {
		user, token = prior.UserKey, prior.APIToken
	}
	a := &NotificationPushoverConfig{UserKey: notificationSecret(v.UserKey, user), APIToken: notificationSecret(v.ApiToken, token), Priority: ptr(notificationOptional(v.Priority, 0))}
	if x, e := v.Retry.Get(); e == nil {
		a.Retry = ptr(x)
	}
	if x, e := v.Expire.Get(); e == nil {
		a.Expire = ptr(x)
	}
	return a, nil
}
func notificationEmailArgsFrom(v *generated.NotificationEmail, prior *NotificationEmailConfig) (*NotificationEmailConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	server, e := notificationRequired(v.SmtpServer)
	if e != nil {
		return nil, e
	}
	port, e := notificationRequired(v.SmtpPort)
	if e != nil {
		return nil, e
	}
	if port < 1 || port > 65535 {
		return nil, errNotificationObservation
	}
	user, e := notificationRequired(v.Username)
	if e != nil {
		return nil, e
	}
	from, e := notificationRequired(v.FromAddress)
	if e != nil {
		return nil, e
	}
	to, e := notificationRequired(v.ToAddresses)
	if e != nil {
		return nil, e
	}
	if len(to) == 0 {
		return nil, errNotificationObservation
	}
	for _, recipient := range to {
		if recipient == "" {
			return nil, errNotificationObservation
		}
	}
	old := ""
	if prior != nil {
		old = prior.Password
	}
	return &NotificationEmailConfig{SMTPServer: server, SMTPPort: port, Username: user, Password: notificationSecret(v.Password, old), FromAddress: from, ToAddresses: to}, nil
}
func notificationResendArgsFrom(v *generated.NotificationResend, prior *NotificationResendConfig) (*NotificationResendConfig, error) {
	if v == nil {
		return nil, errNotificationObservation
	}
	from, e := notificationRequired(v.FromAddress)
	if e != nil {
		return nil, e
	}
	to, e := notificationRequired(v.ToAddresses)
	if e != nil {
		return nil, e
	}
	if len(to) == 0 {
		return nil, errNotificationObservation
	}
	for _, recipient := range to {
		if recipient == "" {
			return nil, errNotificationObservation
		}
	}
	old := ""
	if prior != nil {
		old = prior.APIKey
	}
	return &NotificationResendConfig{APIKey: notificationSecret(v.ApiKey, old), FromAddress: from, ToAddresses: to}, nil
}
