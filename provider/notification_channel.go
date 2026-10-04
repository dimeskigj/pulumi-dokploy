package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
)

// The raw generated methods are used because mutation success is determined by
// HTTP status, not by decoding a response body (which may be empty or unrelated).
func createNotificationChannel(ctx context.Context, api *client.Client, name string, a NotificationArgs) error {
	kind, err := notificationKind(a)
	if err != nil {
		return err
	}
	var resp *http.Response
	switch kind {
	case notificationSlack:
		resp, err = api.NotificationCreateSlack(ctx, notificationSlackCreateBody(name, a))
	case notificationTelegram:
		resp, err = api.NotificationCreateTelegram(ctx, notificationTelegramCreateBody(name, a))
	case notificationDiscord:
		resp, err = api.NotificationCreateDiscord(ctx, notificationDiscordCreateBody(name, a))
	case notificationEmail:
		resp, err = api.NotificationCreateEmail(ctx, notificationEmailCreateBody(name, a))
	case notificationResend:
		resp, err = api.NotificationCreateResend(ctx, notificationResendCreateBody(name, a))
	case notificationGotify:
		resp, err = api.NotificationCreateGotify(ctx, notificationGotifyCreateBody(name, a))
	case notificationNtfy:
		resp, err = api.NotificationCreateNtfy(ctx, notificationNtfyCreateBody(name, a))
	case notificationMattermost:
		resp, err = api.NotificationCreateMattermost(ctx, notificationMattermostCreateBody(name, a))
	case notificationCustom:
		resp, err = api.NotificationCreateCustom(ctx, notificationCustomCreateBody(name, a))
	case notificationLark:
		resp, err = api.NotificationCreateLark(ctx, notificationLarkCreateBody(name, a))
	case notificationTeams:
		resp, err = api.NotificationCreateTeams(ctx, notificationTeamsCreateBody(name, a))
	case notificationPushover:
		resp, err = api.NotificationCreatePushover(ctx, notificationPushoverCreateBody(name, a))
	default:
		return errors.New("unsupported notification channel")
	}
	return notificationMutationResult(resp, err)
}

func updateNotificationChannel(ctx context.Context, api *client.Client, id, channelID string, a NotificationArgs) error {
	kind, err := notificationKind(a)
	if err != nil {
		return err
	}
	var resp *http.Response
	switch kind {
	case notificationSlack:
		resp, err = api.NotificationUpdateSlack(ctx, notificationSlackUpdateBody(id, channelID, a))
	case notificationTelegram:
		resp, err = api.NotificationUpdateTelegram(ctx, notificationTelegramUpdateBody(id, channelID, a))
	case notificationDiscord:
		resp, err = api.NotificationUpdateDiscord(ctx, notificationDiscordUpdateBody(id, channelID, a))
	case notificationEmail:
		resp, err = api.NotificationUpdateEmail(ctx, notificationEmailUpdateBody(id, channelID, a))
	case notificationResend:
		resp, err = api.NotificationUpdateResend(ctx, notificationResendUpdateBody(id, channelID, a))
	case notificationGotify:
		resp, err = api.NotificationUpdateGotify(ctx, notificationGotifyUpdateBody(id, channelID, a))
	case notificationNtfy:
		resp, err = api.NotificationUpdateNtfy(ctx, notificationNtfyUpdateBody(id, channelID, a))
	case notificationMattermost:
		resp, err = api.NotificationUpdateMattermost(ctx, notificationMattermostUpdateBody(id, channelID, a))
	case notificationCustom:
		resp, err = api.NotificationUpdateCustom(ctx, notificationCustomUpdateBody(id, channelID, a))
	case notificationLark:
		resp, err = api.NotificationUpdateLark(ctx, notificationLarkUpdateBody(id, channelID, a))
	case notificationTeams:
		resp, err = api.NotificationUpdateTeams(ctx, notificationTeamsUpdateBody(id, channelID, a))
	case notificationPushover:
		resp, err = api.NotificationUpdatePushover(ctx, notificationPushoverUpdateBody(id, channelID, a))
	default:
		return errors.New("unsupported notification channel")
	}
	return notificationMutationResult(resp, err)
}

func notificationMutationResult(resp *http.Response, err error) error {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("notification mutation returned no response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification mutation returned HTTP %d", resp.StatusCode)
	}
	return nil
}
