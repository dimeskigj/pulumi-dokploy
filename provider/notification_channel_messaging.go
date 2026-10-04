package dokploy

import (
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
)

func notificationTelegramCreateBody(name string, a NotificationArgs) generated.NotificationCreateTelegramJSONRequestBody {
	a = normalizeNotificationArgs(a)
	v := a.Telegram
	return generated.NotificationCreateTelegramJSONRequestBody{Name: name, BotToken: v.BotToken, ChatId: v.ChatID, MessageThreadId: *v.MessageThreadID}
}
func notificationTelegramUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateTelegramJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Telegram
	return generated.NotificationUpdateTelegramJSONRequestBody{
		NotificationId: id, TelegramId: channelID, Name: ptr(a.Name), BotToken: ptr(v.BotToken), ChatId: ptr(v.ChatID), MessageThreadId: v.MessageThreadID,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationGotifyCreateBody(name string, a NotificationArgs) generated.NotificationCreateGotifyJSONRequestBody {
	a = normalizeNotificationArgs(a)
	v := a.Gotify
	return generated.NotificationCreateGotifyJSONRequestBody{Name: name, ServerUrl: v.ServerURL, AppToken: v.AppToken, Priority: float32(*v.Priority), Decoration: *v.Decoration}
}
func notificationGotifyUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateGotifyJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Gotify
	return generated.NotificationUpdateGotifyJSONRequestBody{
		NotificationId: id, GotifyId: channelID, Name: ptr(a.Name), ServerUrl: ptr(v.ServerURL), AppToken: ptr(v.AppToken), Priority: ptr(float32(*v.Priority)), Decoration: v.Decoration,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup),
	}
}

func notificationNtfyCreateBody(name string, a NotificationArgs) generated.NotificationCreateNtfyJSONRequestBody {
	a = normalizeNotificationArgs(a)
	v := a.Ntfy
	return generated.NotificationCreateNtfyJSONRequestBody{Name: name, ServerUrl: v.ServerURL, Topic: v.Topic, AccessToken: *v.AccessToken, Priority: float32(*v.Priority)}
}
func notificationNtfyUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateNtfyJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Ntfy
	return generated.NotificationUpdateNtfyJSONRequestBody{
		NotificationId: id, NtfyId: channelID, Name: ptr(a.Name), ServerUrl: ptr(v.ServerURL), Topic: ptr(v.Topic), AccessToken: v.AccessToken, Priority: ptr(float32(*v.Priority)),
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup),
	}
}

func notificationOptionalNumber(v *int) nullable.Nullable[float32] {
	if v == nil {
		return nullable.NewNullNullable[float32]()
	}
	return nullable.NewNullableWithValue(float32(*v))
}

func notificationCreateOptionalNumber(v *int) nullable.Nullable[float32] {
	if v == nil {
		return nil
	}
	return nullable.NewNullableWithValue(float32(*v))
}

func notificationPushoverCreateBody(name string, a NotificationArgs) generated.NotificationCreatePushoverJSONRequestBody {
	a = normalizeNotificationArgs(a)
	v := a.Pushover
	e := NotificationEvents{}
	return generated.NotificationCreatePushoverJSONRequestBody{
		Name: name, UserKey: v.UserKey, ApiToken: v.APIToken, Priority: ptr(float32(*v.Priority)), Retry: notificationCreateOptionalNumber(v.Retry), Expire: notificationCreateOptionalNumber(v.Expire),
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}
func notificationPushoverUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdatePushoverJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Pushover
	return generated.NotificationUpdatePushoverJSONRequestBody{
		NotificationId: id, PushoverId: channelID, Name: ptr(a.Name), UserKey: ptr(v.UserKey), ApiToken: ptr(v.APIToken), Priority: ptr(float32(*v.Priority)),
		Retry: notificationOptionalNumber(v.Retry), Expire: notificationOptionalNumber(v.Expire),
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}
