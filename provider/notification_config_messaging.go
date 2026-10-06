package dokploy

import "github.com/pulumi/pulumi-go-provider/infer"

type NotificationTelegramConfig struct {
	BotToken        string  `pulumi:"botToken" provider:"secret"`
	ChatID          string  `pulumi:"chatId"`
	MessageThreadID *string `pulumi:"messageThreadId,optional"`
}
type NotificationGotifyConfig struct {
	ServerURL  string `pulumi:"serverUrl" provider:"secret"`
	AppToken   string `pulumi:"appToken" provider:"secret"`
	Priority   *int   `pulumi:"priority,optional"`
	Decoration *bool  `pulumi:"decoration,optional"`
}
type NotificationNtfyConfig struct {
	ServerURL   string  `pulumi:"serverUrl" provider:"secret"`
	Topic       string  `pulumi:"topic"`
	AccessToken *string `pulumi:"accessToken,optional" provider:"secret"`
	Priority    *int    `pulumi:"priority,optional"`
}
type NotificationPushoverConfig struct {
	UserKey  string `pulumi:"userKey" provider:"secret"`
	APIToken string `pulumi:"apiToken" provider:"secret"`
	Priority *int   `pulumi:"priority,optional"`
	Retry    *int   `pulumi:"retry,optional"`
	Expire   *int   `pulumi:"expire,optional"`
}

func (a *NotificationTelegramConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.BotToken, "Secret Telegram bot token.")
	n.Describe(&a.ChatID, "Telegram chat ID.")
	n.Describe(&a.MessageThreadID, "Optional message thread ID; defaults to empty.")
	n.SetDefault(&a.MessageThreadID, "")
}
func (a *NotificationGotifyConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.ServerURL, "Secret Gotify server URL.")
	n.Describe(&a.AppToken, "Secret Gotify application token.")
	n.Describe(&a.Priority, "Message priority; defaults to 5 (minimum 1).")
	n.SetDefault(&a.Priority, 5)
	n.Describe(&a.Decoration, "Whether to decorate messages; defaults to false.")
	n.SetDefault(&a.Decoration, false)
}
func (a *NotificationNtfyConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.ServerURL, "Secret Ntfy server URL.")
	n.Describe(&a.Topic, "Ntfy topic.")
	n.Describe(&a.AccessToken, "Optional secret access token; defaults to empty.")
	n.SetDefault(&a.AccessToken, "")
	n.Describe(&a.Priority, "Message priority, 1 through 5; defaults to 3.")
	n.SetDefault(&a.Priority, 3)
}
func (a *NotificationPushoverConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.UserKey, "Secret Pushover user key.")
	n.Describe(&a.APIToken, "Secret Pushover API token.")
	n.Describe(&a.Priority, "Priority from -2 through 2; defaults to 0.")
	n.SetDefault(&a.Priority, 0)
	n.Describe(&a.Retry, "Emergency retry interval in seconds, at least 30; required for priority 2.")
	n.Describe(&a.Expire, "Emergency expiry in seconds, 1 through 10800; required for priority 2.")
}
