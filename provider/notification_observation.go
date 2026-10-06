package dokploy

import (
	"context"
	"errors"
	"reflect"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
)

type notificationIdentity struct {
	NotificationID, NotificationType, ChannelID, OrganizationID string
}

var errNotificationObservation = errors.New("incomplete or inconsistent notification observation")

func notificationRequired[T any](v nullable.Nullable[T]) (T, error) {
	x, err := v.Get()
	if err != nil || reflect.ValueOf(x).IsZero() {
		return x, errNotificationObservation
	}
	return x, nil
}
func notificationOptional[T any](v nullable.Nullable[T], fallback T) T {
	x, err := v.Get()
	if err != nil {
		return fallback
	}
	return x
}
func notificationSecret(v nullable.Nullable[string], prior string) string {
	if !v.IsSpecified() {
		return prior
	}
	return notificationOptional(v, "")
}
func notificationPresent[T any](v nullable.Nullable[T]) bool { return v.IsSpecified() }

func notificationListIDs(v []generated.Notification) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(v))
	for _, item := range v {
		if item.NotificationId == "" {
			return nil, errNotificationObservation
		}
		if _, exists := ids[item.NotificationId]; exists {
			return nil, errNotificationObservation
		}
		ids[item.NotificationId] = struct{}{}
	}
	return ids, nil
}

func notificationStateFrom(v *generated.Notification, prior *NotificationState, expected notificationIdentity) (NotificationState, error) {
	var s NotificationState
	if v == nil || v.NotificationId == "" || v.OrganizationId == "" || expected.NotificationID == "" || expected.OrganizationID == "" || v.NotificationId != expected.NotificationID || v.OrganizationId != expected.OrganizationID || v.Name == nil || *v.Name == "" {
		return s, errNotificationObservation
	}
	fields := []*bool{v.AppDeploy, v.AppBuildError, v.DatabaseBackup, v.VolumeBackup, v.DokployBackup, v.DokployRestart, v.DockerCleanup, v.ServerThreshold}
	for _, field := range fields {
		if field == nil {
			return s, errNotificationObservation
		}
	}
	s.Name, s.NotificationID, s.OrganizationID = *v.Name, v.NotificationId, v.OrganizationId
	var kind, channelID string
	// A foreign key without its relation (or vice versa) is never a trustworthy identity.
	check := func(k string, key nullable.Nullable[string], active bool, id string) error {
		if key.IsSpecified() && !key.IsNull() || active {
			fk, err := notificationRequired(key)
			if err != nil || !active || id == "" || fk != id || kind != "" {
				return errNotificationObservation
			}
			kind, channelID = k, fk
		}
		return nil
	}
	if err := check(notificationSlack, v.SlackId, v.Slack.IsSpecified() && !v.Slack.IsNull(), func() string {
		if x, e := v.Slack.Get(); e == nil {
			return x.SlackId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationTelegram, v.TelegramId, v.Telegram.IsSpecified() && !v.Telegram.IsNull(), func() string {
		if x, e := v.Telegram.Get(); e == nil {
			return x.TelegramId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationDiscord, v.DiscordId, v.Discord.IsSpecified() && !v.Discord.IsNull(), func() string {
		if x, e := v.Discord.Get(); e == nil {
			return x.DiscordId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationEmail, v.EmailId, v.Email.IsSpecified() && !v.Email.IsNull(), func() string {
		if x, e := v.Email.Get(); e == nil {
			return x.EmailId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationResend, v.ResendId, v.Resend.IsSpecified() && !v.Resend.IsNull(), func() string {
		if x, e := v.Resend.Get(); e == nil {
			return x.ResendId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationGotify, v.GotifyId, v.Gotify.IsSpecified() && !v.Gotify.IsNull(), func() string {
		if x, e := v.Gotify.Get(); e == nil {
			return x.GotifyId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationNtfy, v.NtfyId, v.Ntfy.IsSpecified() && !v.Ntfy.IsNull(), func() string {
		if x, e := v.Ntfy.Get(); e == nil {
			return x.NtfyId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationMattermost, v.MattermostId, v.Mattermost.IsSpecified() && !v.Mattermost.IsNull(), func() string {
		if x, e := v.Mattermost.Get(); e == nil {
			return x.MattermostId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationCustom, v.CustomId, v.Custom.IsSpecified() && !v.Custom.IsNull(), func() string {
		if x, e := v.Custom.Get(); e == nil {
			return x.CustomId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationLark, v.LarkId, v.Lark.IsSpecified() && !v.Lark.IsNull(), func() string {
		if x, e := v.Lark.Get(); e == nil {
			return x.LarkId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationTeams, v.TeamsId, v.Teams.IsSpecified() && !v.Teams.IsNull(), func() string {
		if x, e := v.Teams.Get(); e == nil {
			return x.TeamsId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if err := check(notificationPushover, v.PushoverId, v.Pushover.IsSpecified() && !v.Pushover.IsNull(), func() string {
		if x, e := v.Pushover.Get(); e == nil {
			return x.PushoverId
		}
		return ""
	}()); err != nil {
		return s, err
	}
	if kind == "" || expected.NotificationType != "" && expected.NotificationType != kind || expected.ChannelID != "" && expected.ChannelID != channelID {
		return s, errNotificationObservation
	}
	s.NotificationType, s.ChannelID = kind, channelID
	var old NotificationArgs
	if prior != nil {
		old = prior.NotificationArgs
	}
	var err error
	s.NotificationArgs, err = notificationProject(v, old, kind)
	if err != nil {
		return NotificationState{}, err
	}
	s.Events = &NotificationEvents{*fields[0], *fields[1], *fields[2], *fields[3], *fields[4], *fields[5], *fields[6], *fields[7]}
	s.Name = *v.Name
	return s, nil
}

func notificationProject(v *generated.Notification, old NotificationArgs, kind string) (NotificationArgs, error) {
	var a NotificationArgs
	var err error
	switch kind {
	case notificationSlack:
		x, _ := v.Slack.Get()
		a.Slack, err = notificationSlackArgsFrom(&x, old.Slack)
	case notificationTelegram:
		x, _ := v.Telegram.Get()
		a.Telegram, err = notificationTelegramArgsFrom(&x, old.Telegram)
	case notificationDiscord:
		x, _ := v.Discord.Get()
		a.Discord, err = notificationDiscordArgsFrom(&x, old.Discord)
	case notificationEmail:
		x, _ := v.Email.Get()
		a.Email, err = notificationEmailArgsFrom(&x, old.Email)
	case notificationResend:
		x, _ := v.Resend.Get()
		a.Resend, err = notificationResendArgsFrom(&x, old.Resend)
	case notificationGotify:
		x, _ := v.Gotify.Get()
		a.Gotify, err = notificationGotifyArgsFrom(&x, old.Gotify)
	case notificationNtfy:
		x, _ := v.Ntfy.Get()
		a.Ntfy, err = notificationNtfyArgsFrom(&x, old.Ntfy)
	case notificationMattermost:
		x, _ := v.Mattermost.Get()
		a.Mattermost, err = notificationMattermostArgsFrom(&x, old.Mattermost)
	case notificationCustom:
		x, _ := v.Custom.Get()
		a.Custom, err = notificationCustomArgsFrom(&x, old.Custom)
	case notificationLark:
		x, _ := v.Lark.Get()
		a.Lark, err = notificationLarkArgsFrom(&x, old.Lark)
	case notificationTeams:
		x, _ := v.Teams.Get()
		a.Teams, err = notificationTeamsArgsFrom(&x, old.Teams)
	case notificationPushover:
		x, _ := v.Pushover.Get()
		a.Pushover, err = notificationPushoverArgsFrom(&x, old.Pushover)
	default:
		return a, errNotificationObservation
	}
	return a, err
}

func notificationCandidate(v *generated.Notification, marker, orgID string, desired NotificationArgs, before map[string]struct{}) (NotificationState, bool, error) {
	var zero NotificationState
	if v == nil || v.Name == nil || *v.Name != marker {
		return zero, false, nil
	}
	if marker == "" || orgID == "" || v.NotificationId == "" {
		return zero, false, errNotificationObservation
	}
	if _, exists := before[v.NotificationId]; exists {
		return zero, false, errNotificationObservation
	}
	kind, err := notificationKind(desired)
	if err != nil {
		return zero, false, err
	}
	s, err := notificationStateFrom(v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: orgID, NotificationType: kind})
	if err != nil {
		return zero, false, err
	}
	if *s.Events != (NotificationEvents{}) || !notificationComplete(v, kind) {
		return zero, false, errNotificationObservation
	}
	want := normalizeNotificationArgs(desired)
	if !reflect.DeepEqual(notificationBlock(s.NotificationArgs, kind), notificationBlock(want, kind)) {
		return zero, false, errNotificationObservation
	}
	return s, true, nil
}

func notificationComplete(v *generated.Notification, kind string) bool {
	switch kind {
	case notificationSlack:
		x, _ := v.Slack.Get()
		return notificationPresent(x.WebhookUrl) && notificationPresent(x.Channel)
	case notificationTelegram:
		x, _ := v.Telegram.Get()
		return notificationPresent(x.BotToken) && notificationPresent(x.ChatId) && notificationPresent(x.MessageThreadId)
	case notificationDiscord:
		x, _ := v.Discord.Get()
		return notificationPresent(x.WebhookUrl) && notificationPresent(x.Decoration)
	case notificationEmail:
		x, _ := v.Email.Get()
		return notificationPresent(x.SmtpServer) && notificationPresent(x.SmtpPort) && notificationPresent(x.Username) && notificationPresent(x.Password) && notificationPresent(x.FromAddress) && notificationPresent(x.ToAddresses)
	case notificationResend:
		x, _ := v.Resend.Get()
		return notificationPresent(x.ApiKey) && notificationPresent(x.FromAddress) && notificationPresent(x.ToAddresses)
	case notificationGotify:
		x, _ := v.Gotify.Get()
		return notificationPresent(x.ServerUrl) && notificationPresent(x.AppToken) && notificationPresent(x.Priority) && notificationPresent(x.Decoration)
	case notificationNtfy:
		x, _ := v.Ntfy.Get()
		return notificationPresent(x.ServerUrl) && notificationPresent(x.Topic) && notificationPresent(x.AccessToken) && notificationPresent(x.Priority)
	case notificationMattermost:
		x, _ := v.Mattermost.Get()
		return notificationPresent(x.WebhookUrl) && notificationPresent(x.Channel) && notificationPresent(x.Username)
	case notificationCustom:
		x, _ := v.Custom.Get()
		return notificationPresent(x.Endpoint) && notificationPresent(x.Headers)
	case notificationLark:
		x, _ := v.Lark.Get()
		return notificationPresent(x.WebhookUrl)
	case notificationTeams:
		x, _ := v.Teams.Get()
		return notificationPresent(x.WebhookUrl)
	case notificationPushover:
		x, _ := v.Pushover.Get()
		return notificationPresent(x.UserKey) && notificationPresent(x.ApiToken) && notificationPresent(x.Priority)
	}
	return false
}

func notificationActiveOrganizationID(ctx context.Context, api *client.Client) (string, error) {
	r, err := api.OrganizationActiveWithResponse(ctx)
	if err != nil {
		return "", err
	}
	if r == nil || r.JSON200 == nil {
		return "", errNotificationObservation
	}
	id := activeOrganizationID(*r.JSON200)
	if id == "" {
		return "", errNotificationObservation
	}
	return id, nil
}
func readNotification(ctx context.Context, api *client.Client, expected notificationIdentity, prior *NotificationState) (NotificationState, error) {
	if expected.NotificationID == "" || expected.OrganizationID == "" {
		return NotificationState{}, errNotificationObservation
	}
	r, err := api.NotificationOneWithResponse(ctx, &generated.NotificationOneParams{NotificationId: expected.NotificationID})
	if err != nil {
		return NotificationState{}, err
	}
	if r == nil || r.JSON200 == nil {
		return NotificationState{}, errNotificationObservation
	}
	return notificationStateFrom(r.JSON200, prior, expected)
}
