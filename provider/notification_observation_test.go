package dokploy

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func observedNotification(t *testing.T, kind string, settings string) generated.Notification {
	t.Helper()
	var v generated.Notification
	fixture := `{"notificationId":"opaque+/% space","organizationId":"org","name":"marker","appDeploy":false,"appBuildError":false,"databaseBackup":false,"volumeBackup":false,"dokployBackup":false,"dokployRestart":false,"dockerCleanup":false,"serverThreshold":false,"unknownFutureField":{"ignored":true},"` + kind + `Id":"channel","` + kind + `":` + settings + `}`
	require.NoError(t, json.Unmarshal([]byte(fixture), &v))
	return v
}

func TestNotificationExactReadAndOrganization(t *testing.T) {
	v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook"}`)
	count := 0
	r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/organization.active":
			_, _ = w.Write([]byte(`{"id":"org"}`))
		case "/api/notification.one":
			count++
			if req.URL.Query().Get("notificationId") != v.NotificationId {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(v)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	api := r.client(context.Background())
	org, err := notificationActiveOrganizationID(context.Background(), api)
	require.NoError(t, err)
	require.Equal(t, "org", org)
	s, err := readNotification(context.Background(), api, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: org}, nil)
	require.NoError(t, err)
	require.Equal(t, v.NotificationId, s.NotificationID)
	require.Equal(t, 1, count)
	_, err = readNotification(context.Background(), api, notificationIdentity{}, nil)
	require.Error(t, err)
	require.Equal(t, 1, count)
}

type notificationProjectionCase struct {
	settings, omit string
	want           NotificationArgs
}

func notificationProjectionCases() map[string]notificationProjectionCase {
	return map[string]notificationProjectionCase{
		"slack":      {`{"slackId":"channel","webhookUrl":"https://example.com/slack","channel":"alerts"}`, "channel", NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "https://example.com/slack", Channel: ptr("alerts")}}},
		"telegram":   {`{"telegramId":"channel","botToken":"token","chatId":"chat","messageThreadId":"thread"}`, "messageThreadId", NotificationArgs{Telegram: &NotificationTelegramConfig{BotToken: "token", ChatID: "chat", MessageThreadID: ptr("thread")}}},
		"discord":    {`{"discordId":"channel","webhookUrl":"https://example.com/discord","decoration":true}`, "decoration", NotificationArgs{Discord: &NotificationDiscordConfig{WebhookURL: "https://example.com/discord", Decoration: ptr(true)}}},
		"email":      {`{"emailId":"channel","smtpServer":"smtp.example.com","smtpPort":2525,"username":"user","password":"password","fromAddress":"sender@example.com","toAddresses":["one@example.com","two@example.com"]}`, "smtpPort", NotificationArgs{Email: &NotificationEmailConfig{SMTPServer: "smtp.example.com", SMTPPort: 2525, Username: "user", Password: "password", FromAddress: "sender@example.com", ToAddresses: []string{"one@example.com", "two@example.com"}}}},
		"resend":     {`{"resendId":"channel","apiKey":"key","fromAddress":"sender@example.com","toAddresses":["one@example.com","two@example.com"]}`, "fromAddress", NotificationArgs{Resend: &NotificationResendConfig{APIKey: "key", FromAddress: "sender@example.com", ToAddresses: []string{"one@example.com", "two@example.com"}}}},
		"gotify":     {`{"gotifyId":"channel","serverUrl":"https://example.com/gotify","appToken":"token","priority":7,"decoration":true}`, "priority", NotificationArgs{Gotify: &NotificationGotifyConfig{ServerURL: "https://example.com/gotify", AppToken: "token", Priority: ptr(7), Decoration: ptr(true)}}},
		"ntfy":       {`{"ntfyId":"channel","serverUrl":"https://example.com/ntfy","topic":"topic","accessToken":"token","priority":4}`, "priority", NotificationArgs{Ntfy: &NotificationNtfyConfig{ServerURL: "https://example.com/ntfy", Topic: "topic", AccessToken: ptr("token"), Priority: ptr(4)}}},
		"mattermost": {`{"mattermostId":"channel","webhookUrl":"https://example.com/mattermost","channel":"alerts","username":"display"}`, "channel", NotificationArgs{Mattermost: &NotificationMattermostConfig{WebhookURL: "https://example.com/mattermost", Channel: ptr("alerts"), Username: ptr("display")}}},
		"custom":     {`{"customId":"channel","endpoint":"https://example.com/custom","headers":{"X-Test":"value"}}`, "headers", NotificationArgs{Custom: &NotificationCustomConfig{Endpoint: "https://example.com/custom", Headers: map[string]string{"X-Test": "value"}}}},
		"lark":       {`{"larkId":"channel","webhookUrl":"https://example.com/lark"}`, "webhookUrl", NotificationArgs{Lark: &NotificationLarkConfig{WebhookURL: "https://example.com/lark"}}},
		"teams":      {`{"teamsId":"channel","webhookUrl":"https://example.com/teams"}`, "webhookUrl", NotificationArgs{Teams: &NotificationTeamsConfig{WebhookURL: "https://example.com/teams"}}},
		"pushover":   {`{"pushoverId":"channel","userKey":"user-key","apiToken":"api-token","priority":2,"retry":30,"expire":120}`, "priority", NotificationArgs{Pushover: &NotificationPushoverConfig{UserKey: "user-key", APIToken: "api-token", Priority: ptr(2), Retry: ptr(30), Expire: ptr(120)}}},
	}
}

func TestNotificationObservedStateAllChannels(t *testing.T) {
	for kind, tc := range notificationProjectionCases() {
		t.Run(kind, func(t *testing.T) {
			v := observedNotification(t, kind, tc.settings)
			v.AppDeploy, v.DatabaseBackup, v.DokployRestart, v.ServerThreshold = ptr(true), ptr(true), ptr(true), ptr(true)
			state, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			want := NotificationState{NotificationArgs: tc.want, NotificationID: v.NotificationId, NotificationType: kind, ChannelID: "channel", OrganizationID: "org"}
			want.Name = "marker"
			want.Events = &NotificationEvents{AppDeploy: true, DatabaseBackup: true, DokployRestart: true, ServerThreshold: true}
			require.Equal(t, want, state, "entire state must match; inactive blocks and extra fields must be absent")
		})
	}
}

func TestNotificationIdentityValidation(t *testing.T) {
	// Complete inline response: the selected relation ID must agree with its foreign key.
	var v generated.Notification
	require.NoError(t, json.Unmarshal([]byte(`{"notificationId":"placeholder-notification","organizationId":"placeholder-org","name":"example","appDeploy":false,"appBuildError":false,"databaseBackup":false,"volumeBackup":false,"dokployBackup":false,"dokployRestart":false,"dockerCleanup":false,"serverThreshold":false,"slackId":"foreign","slack":{"slackId":"nested","webhookUrl":"https://example.com/hook","channel":""}}`), &v))
	_, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: "placeholder-notification", OrganizationID: "placeholder-org"})
	require.Error(t, err)
	good := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook"}`)
	identity := notificationIdentity{NotificationID: good.NotificationId, OrganizationID: "org"}
	for name, expected := range map[string]notificationIdentity{
		"notification": {NotificationID: "different", OrganizationID: "org"},
		"organization": {NotificationID: good.NotificationId, OrganizationID: "other"},
		"type":         {NotificationID: good.NotificationId, OrganizationID: "org", NotificationType: "teams"},
		"channel":      {NotificationID: good.NotificationId, OrganizationID: "org", ChannelID: "other"},
	} {
		t.Run(name, func(t *testing.T) { _, err := notificationStateFrom(&good, nil, expected); require.Error(t, err) })
	}
	t.Run("missing foreign key", func(t *testing.T) {
		x := good
		x.SlackId.SetUnspecified()
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("multiple relations", func(t *testing.T) {
		x := good
		require.NoError(t, json.Unmarshal([]byte(`{"teamsId":"other","teams":{"teamsId":"other","webhookUrl":"https://example.com/hook"}}`), &x))
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("missing event", func(t *testing.T) {
		x := good
		x.AppDeploy = nil
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("decoded null event", func(t *testing.T) {
		var x generated.Notification
		fixture, marshalErr := json.Marshal(good)
		require.NoError(t, marshalErr)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(fixture, &fields))
		fields["appBuildError"] = json.RawMessage("null")
		fixture, marshalErr = json.Marshal(fields)
		require.NoError(t, marshalErr)
		require.NoError(t, json.Unmarshal(fixture, &x))
		require.Nil(t, x.AppBuildError)
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("unsupported type", func(t *testing.T) {
		var x generated.Notification
		require.NoError(t, json.Unmarshal([]byte(`{"notificationId":"opaque+/% space","organizationId":"org","name":"unknown","appDeploy":false,"appBuildError":false,"databaseBackup":false,"volumeBackup":false,"dokployBackup":false,"dokployRestart":false,"dockerCleanup":false,"serverThreshold":false,"futureId":"channel","future":{"futureId":"channel"}}`), &x))
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("negative SMTP port", func(t *testing.T) {
		x := observedNotification(t, "email", `{"emailId":"channel","smtpServer":"smtp.example.com","smtpPort":-1,"username":"user","password":"password","fromAddress":"sender@example.com","toAddresses":["recipient@example.com"]}`)
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	t.Run("required nonsecret", func(t *testing.T) {
		x := observedNotification(t, "telegram", `{"telegramId":"channel","botToken":"token","chatId":null}`)
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	s, err := notificationStateFrom(&good, nil, identity)
	require.NoError(t, err)
	require.Equal(t, good.NotificationId, s.NotificationID)
}

func TestNotificationDiscoveryAllChannels(t *testing.T) {
	for kind, tc := range notificationProjectionCases() {
		t.Run(kind, func(t *testing.T) {
			v := observedNotification(t, kind, tc.settings)
			state, matched, err := notificationCandidate(&v, "marker", "org", tc.want, map[string]struct{}{})
			require.NoError(t, err)
			require.True(t, matched)
			require.Equal(t, notificationBlock(tc.want, kind), notificationBlock(state.NotificationArgs, kind), "candidate must preserve every canonical channel setting")
			require.Equal(t, &NotificationEvents{}, state.Events)
			require.Equal(t, v.NotificationId, state.NotificationID)

			// Delete one observed setting to ensure defaults never conceal missing list data.
			raw, err := json.Marshal(v)
			require.NoError(t, err)
			var record map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &record))
			var relation map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(record[kind], &relation))
			delete(relation, tc.omit)
			record[kind], err = json.Marshal(relation)
			require.NoError(t, err)
			raw, err = json.Marshal(record)
			require.NoError(t, err)
			var incomplete generated.Notification
			require.NoError(t, json.Unmarshal(raw, &incomplete))
			_, matched, err = notificationCandidate(&incomplete, "marker", "org", tc.want, nil)
			require.Error(t, err)
			require.False(t, matched)
		})
	}
}

func TestNotificationDiscoveryCandidates(t *testing.T) {
	v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
	state, match, err := notificationCandidate(&v, "marker", "org", NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "https://example.com/hook"}}, map[string]struct{}{})
	require.NoError(t, err)
	require.True(t, match)
	require.Equal(t, v.NotificationId, state.NotificationID)
}

func TestNotificationMarkerConflictIsNotEmpty(t *testing.T) {
	v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
	_, match, err := notificationCandidate(&v, "marker", "org", NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "https://example.com/different"}}, map[string]struct{}{})
	require.Error(t, err)
	require.False(t, match)
}

func TestNotificationSecretPresence(t *testing.T) {
	identity := notificationIdentity{NotificationID: "opaque+/% space", OrganizationID: "org"}
	for _, tc := range []struct{ name, settings, want string }{
		{"omitted", `{"slackId":"channel"}`, "previous"},
		{"null", `{"slackId":"channel","webhookUrl":null}`, ""},
		{"empty", `{"slackId":"channel","webhookUrl":""}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := observedNotification(t, "slack", tc.settings)
			s, err := notificationStateFrom(&v, &NotificationState{NotificationArgs: NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "previous"}}}, identity)
			require.NoError(t, err)
			require.Equal(t, tc.want, s.Slack.WebhookURL)
			if tc.name == "omitted" {
				s, err = notificationStateFrom(&v, nil, identity)
				require.NoError(t, err)
				require.Empty(t, s.Slack.WebhookURL)
			}
		})
	}
	ntfy := observedNotification(t, "ntfy", `{"ntfyId":"channel","serverUrl":"https://example.com","topic":"topic"}`)
	s, err := notificationStateFrom(&ntfy, nil, identity)
	require.NoError(t, err)
	require.Equal(t, "", *s.Ntfy.AccessToken)
}

func TestNotificationNullableRoutingAndHeaders(t *testing.T) {
	for _, tc := range []struct {
		kind, settings string
		want           NotificationArgs
	}{
		{"slack", `{"slackId":"channel","webhookUrl":"https://example.com/slack","channel":null}`, NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "https://example.com/slack", Channel: ptr("")}}},
		{"telegram", `{"telegramId":"channel","botToken":"token","chatId":"chat","messageThreadId":null}`, NotificationArgs{Telegram: &NotificationTelegramConfig{BotToken: "token", ChatID: "chat", MessageThreadID: ptr("")}}},
		{"mattermost", `{"mattermostId":"channel","webhookUrl":"https://example.com/mattermost","channel":null,"username":null}`, NotificationArgs{Mattermost: &NotificationMattermostConfig{WebhookURL: "https://example.com/mattermost", Channel: ptr(""), Username: ptr("")}}},
		{"custom", `{"customId":"channel","endpoint":"https://example.com/custom","headers":null}`, NotificationArgs{Custom: &NotificationCustomConfig{Endpoint: "https://example.com/custom", Headers: map[string]string{}}}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			v := observedNotification(t, tc.kind, tc.settings)
			s, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			require.Equal(t, notificationBlock(tc.want, tc.kind), notificationBlock(s.NotificationArgs, tc.kind))
			candidate, matched, err := notificationCandidate(&v, "marker", "org", tc.want, nil)
			require.NoError(t, err)
			require.True(t, matched)
			require.Equal(t, notificationBlock(tc.want, tc.kind), notificationBlock(candidate.NotificationArgs, tc.kind))
		})
	}
}

func TestNotificationCandidateSafety(t *testing.T) {
	v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
	desired := NotificationArgs{Slack: &NotificationSlackConfig{WebhookURL: "https://example.com/hook"}}
	for name, mutate := range map[string]func(*generated.Notification){
		"old ID":             func(x *generated.Notification) {},
		"wrong org":          func(x *generated.Notification) { x.OrganizationId = "other" },
		"nonfalse event":     func(x *generated.Notification) { x.AppDeploy = ptr(true) },
		"missing optional":   func(x *generated.Notification) { r, _ := x.Slack.Get(); r.Channel.SetUnspecified(); x.Slack.Set(r) },
		"missing credential": func(x *generated.Notification) { r, _ := x.Slack.Get(); r.WebhookUrl.SetUnspecified(); x.Slack.Set(r) },
		"wrong relation":     func(x *generated.Notification) { x.SlackId.Set("other") },
	} {
		t.Run(name, func(t *testing.T) {
			x := v
			mutate(&x)
			before := map[string]struct{}{}
			if name == "old ID" {
				before[x.NotificationId] = struct{}{}
			}
			_, ok, err := notificationCandidate(&x, "marker", "org", desired, before)
			require.Error(t, err)
			require.False(t, ok)
		})
	}
	_, ok, err := notificationCandidate(&v, "unrelated", "org", desired, nil)
	require.NoError(t, err)
	require.False(t, ok)
	_, err = notificationListIDs([]generated.Notification{v, v})
	require.Error(t, err)
	v.NotificationId = ""
	_, err = notificationListIDs([]generated.Notification{v})
	require.Error(t, err)
}
