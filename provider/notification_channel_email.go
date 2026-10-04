package dokploy

import "github.com/dimeskigj/pulumi-dokploy/internal/client/generated"

func notificationEmailCreateBody(name string, a NotificationArgs) generated.NotificationCreateEmailJSONRequestBody {
	v := a.Email
	return generated.NotificationCreateEmailJSONRequestBody{Name: name, SmtpServer: v.SMTPServer, SmtpPort: float32(v.SMTPPort), Username: v.Username, Password: v.Password, FromAddress: v.FromAddress, ToAddresses: v.ToAddresses}
}
func notificationEmailUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateEmailJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Email
	return generated.NotificationUpdateEmailJSONRequestBody{
		NotificationId: id, EmailId: channelID, Name: ptr(a.Name), SmtpServer: ptr(v.SMTPServer), SmtpPort: ptr(float32(v.SMTPPort)),
		Username: ptr(v.Username), Password: ptr(v.Password), FromAddress: ptr(v.FromAddress), ToAddresses: &v.ToAddresses,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}

func notificationResendCreateBody(name string, a NotificationArgs) generated.NotificationCreateResendJSONRequestBody {
	v := a.Resend
	return generated.NotificationCreateResendJSONRequestBody{Name: name, ApiKey: v.APIKey, FromAddress: v.FromAddress, ToAddresses: v.ToAddresses}
}
func notificationResendUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateResendJSONRequestBody {
	a = normalizeNotificationArgs(a)
	e, v := a.Events, a.Resend
	return generated.NotificationUpdateResendJSONRequestBody{
		NotificationId: id, ResendId: channelID, Name: ptr(a.Name), ApiKey: ptr(v.APIKey), FromAddress: ptr(v.FromAddress), ToAddresses: &v.ToAddresses,
		AppDeploy: ptr(e.AppDeploy), AppBuildError: ptr(e.AppBuildError), DatabaseBackup: ptr(e.DatabaseBackup), VolumeBackup: ptr(e.VolumeBackup),
		DokployBackup: ptr(e.DokployBackup), DokployRestart: ptr(e.DokployRestart), DockerCleanup: ptr(e.DockerCleanup), ServerThreshold: ptr(e.ServerThreshold),
	}
}
