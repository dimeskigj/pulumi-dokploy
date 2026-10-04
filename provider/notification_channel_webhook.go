package dokploy

import "github.com/dimeskigj/pulumi-dokploy/internal/client/generated"

// Create requests deliberately leave every event disabled until observation has
// verified the newly created notification and its channel relation.
func notificationSlackCreateBody(name string, a NotificationArgs) generated.NotificationCreateSlackJSONRequestBody {
	a = normalizeNotificationArgs(a)
	return generated.NotificationCreateSlackJSONRequestBody{Name: name, WebhookUrl: a.Slack.WebhookURL, Channel: *a.Slack.Channel}
}
func notificationSlackUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateSlackJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateSlackJSONRequestBody{
		NotificationId: id, SlackId: channelID, Name: ptr(a.Name), WebhookUrl: ptr(a.Slack.WebhookURL), Channel: a.Slack.Channel,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationDiscordCreateBody(name string, a NotificationArgs) generated.NotificationCreateDiscordJSONRequestBody {
	a = normalizeNotificationArgs(a)
	return generated.NotificationCreateDiscordJSONRequestBody{Name: name, WebhookUrl: a.Discord.WebhookURL, Decoration: *a.Discord.Decoration}
}
func notificationDiscordUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateDiscordJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateDiscordJSONRequestBody{
		NotificationId: id, DiscordId: channelID, Name: ptr(a.Name), WebhookUrl: ptr(a.Discord.WebhookURL), Decoration: a.Discord.Decoration,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationMattermostCreateBody(name string, a NotificationArgs) generated.NotificationCreateMattermostJSONRequestBody {
	a = normalizeNotificationArgs(a)
	return generated.NotificationCreateMattermostJSONRequestBody{Name: name, WebhookUrl: a.Mattermost.WebhookURL, Channel: a.Mattermost.Channel, Username: a.Mattermost.Username}
}
func notificationMattermostUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateMattermostJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateMattermostJSONRequestBody{
		NotificationId: id, MattermostId: channelID, Name: ptr(a.Name), WebhookUrl: ptr(a.Mattermost.WebhookURL), Channel: a.Mattermost.Channel, Username: a.Mattermost.Username,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationCustomCreateBody(name string, a NotificationArgs) generated.NotificationCreateCustomJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := NotificationEvents{}
	return generated.NotificationCreateCustomJSONRequestBody{
		Name: name, Endpoint: a.Custom.Endpoint, Headers: &a.Custom.Headers,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}
func notificationCustomUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateCustomJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateCustomJSONRequestBody{
		NotificationId: id, CustomId: channelID, Name: ptr(a.Name), Endpoint: ptr(a.Custom.Endpoint), Headers: &a.Custom.Headers,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationLarkCreateBody(name string, a NotificationArgs) generated.NotificationCreateLarkJSONRequestBody {
	return generated.NotificationCreateLarkJSONRequestBody{Name: name, WebhookUrl: a.Lark.WebhookURL}
}
func notificationLarkUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateLarkJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateLarkJSONRequestBody{
		NotificationId: id, LarkId: channelID, Name: ptr(a.Name), WebhookUrl: ptr(a.Lark.WebhookURL),
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationTeamsCreateBody(name string, a NotificationArgs) generated.NotificationCreateTeamsJSONRequestBody {
	return generated.NotificationCreateTeamsJSONRequestBody{Name: name, WebhookUrl: a.Teams.WebhookURL}
}
func notificationTeamsUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateTeamsJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e := a.Events
	return generated.NotificationUpdateTeamsJSONRequestBody{
		NotificationId: id, TeamsId: channelID, Name: ptr(a.Name), WebhookUrl: ptr(a.Teams.WebhookURL),
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}
