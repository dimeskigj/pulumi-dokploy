package dokploy

import "github.com/pulumi/pulumi-go-provider/infer"

type NotificationSlackConfig struct {
	WebhookURL string  `pulumi:"webhookUrl" provider:"secret"`
	Channel    *string `pulumi:"channel,optional"`
}
type NotificationDiscordConfig struct {
	WebhookURL string `pulumi:"webhookUrl" provider:"secret"`
	Decoration *bool  `pulumi:"decoration,optional"`
}
type NotificationMattermostConfig struct {
	WebhookURL string  `pulumi:"webhookUrl" provider:"secret"`
	Channel    *string `pulumi:"channel,optional"`
	Username   *string `pulumi:"username,optional"`
}
type NotificationCustomConfig struct {
	Endpoint string            `pulumi:"endpoint" provider:"secret"`
	Headers  map[string]string `pulumi:"headers,optional" provider:"secret"`
}
type NotificationLarkConfig struct {
	WebhookURL string `pulumi:"webhookUrl" provider:"secret"`
}
type NotificationTeamsConfig struct {
	WebhookURL string `pulumi:"webhookUrl" provider:"secret"`
}

func (a *NotificationSlackConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.WebhookURL, "Secret Slack webhook URL.")
	n.Describe(&a.Channel, "Optional routing channel; defaults to empty.")
	n.SetDefault(&a.Channel, "")
}
func (a *NotificationDiscordConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.WebhookURL, "Secret Discord webhook URL.")
	n.Describe(&a.Decoration, "Whether to decorate messages; defaults to false.")
	n.SetDefault(&a.Decoration, false)
}
func (a *NotificationMattermostConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.WebhookURL, "Secret Mattermost webhook URL.")
	n.Describe(&a.Channel, "Optional routing channel; defaults to empty.")
	n.SetDefault(&a.Channel, "")
	n.Describe(&a.Username, "Optional display username; defaults to empty.")
	n.SetDefault(&a.Username, "")
}
func (a *NotificationCustomConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.Endpoint, "Secret HTTP(S) destination URL.")
	n.Describe(&a.Headers, "Secret custom HTTP headers; defaults to an empty map.")
	// Pulumi schema rejects constant defaults for maps. Check normalizes omitted
	// headers to an empty map; keep the runtime default documented here.
}
func (a *NotificationLarkConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.WebhookURL, "Secret Lark webhook URL.")
}
func (a *NotificationTeamsConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.WebhookURL, "Secret Teams webhook URL.")
}
