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
	fixture := `{"notificationId":"opaque+/% space","organizationId":"org","name":"marker","appDeploy":false,"appBuildError":false,"databaseBackup":false,"volumeBackup":false,"dokployBackup":false,"dokployRestart":false,"dockerCleanup":false,"serverThreshold":false,"` + kind + `Id":"channel","` + kind + `":` + settings + `}`
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

func TestNotificationObservedStateAllChannels(t *testing.T) {
	cases := map[string]string{
		"slack":      `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":null}`,
		"telegram":   `{"telegramId":"channel","botToken":"placeholder","chatId":"chat","messageThreadId":null}`,
		"discord":    `{"discordId":"channel","webhookUrl":"https://example.com/hook","decoration":false}`,
		"email":      `{"emailId":"channel","smtpServer":"example.com","smtpPort":25,"username":"user","password":"placeholder","fromAddress":"sender@example.com","toAddresses":["recipient@example.com"]}`,
		"resend":     `{"resendId":"channel","apiKey":"placeholder","fromAddress":"sender@example.com","toAddresses":["recipient@example.com"]}`,
		"gotify":     `{"gotifyId":"channel","serverUrl":"https://example.com","appToken":"placeholder","priority":5,"decoration":false}`,
		"ntfy":       `{"ntfyId":"channel","serverUrl":"https://example.com","topic":"topic","accessToken":null,"priority":3}`,
		"mattermost": `{"mattermostId":"channel","webhookUrl":"https://example.com/hook","channel":null,"username":null}`,
		"custom":     `{"customId":"channel","endpoint":"https://example.com/hook","headers":null}`,
		"lark":       `{"larkId":"channel","webhookUrl":"https://example.com/hook"}`,
		"teams":      `{"teamsId":"channel","webhookUrl":"https://example.com/hook"}`,
		"pushover":   `{"pushoverId":"channel","userKey":"placeholder","apiToken":"placeholder","priority":0,"retry":null,"expire":null}`,
	}
	for kind, settings := range cases {
		t.Run(kind, func(t *testing.T) {
			v := observedNotification(t, kind, settings)
			state, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			require.Equal(t, kind, state.NotificationType)
			require.Equal(t, "channel", state.ChannelID)
			require.Equal(t, "org", state.OrganizationID)
			require.Equal(t, "marker", state.Name)
			require.Equal(t, &NotificationEvents{}, state.Events)
			require.NotNil(t, notificationBlock(state.NotificationArgs, kind))
			for _, other := range notificationChannels {
				if other != kind {
					require.False(t, notificationSelected(state.NotificationArgs, other))
				}
			}
		})
	}
}

func TestNotificationIdentityValidation(t *testing.T) {
	v := observedNotification(t, "slack", `{"slackId":"different","webhookUrl":"https://example.com/hook"}`)
	_, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
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
	t.Run("required nonsecret", func(t *testing.T) {
		x := observedNotification(t, "telegram", `{"telegramId":"channel","botToken":"token","chatId":null}`)
		_, err := notificationStateFrom(&x, nil, identity)
		require.Error(t, err)
	})
	s, err := notificationStateFrom(&good, nil, identity)
	require.NoError(t, err)
	require.Equal(t, good.NotificationId, s.NotificationID)
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
