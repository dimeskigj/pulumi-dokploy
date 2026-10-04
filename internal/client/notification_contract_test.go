package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
)

func TestNotificationGeneratedDecoding(t *testing.T) {
	fixture := json.RawMessage(`{"notificationId":"opaque","organizationId":"org","appDeploy":false,"appBuildError":false,"databaseBackup":false,"volumeBackup":false,"dokployBackup":false,"dokployRestart":false,"dockerCleanup":false,"serverThreshold":false,"slack":{"slackId":"s","webhookUrl":null},"telegram":{"telegramId":"t","botToken":"token","chatId":"chat","messageThreadId":null},"discord":{"discordId":"d","webhookUrl":"url","decoration":false},"email":{"emailId":"e","smtpServer":"smtp","smtpPort":25,"username":"u","password":null,"fromAddress":"from","toAddresses":["to"]},"resend":{"resendId":"r","apiKey":"key","fromAddress":"from","toAddresses":["to"]},"gotify":{"gotifyId":"g","serverUrl":"url","appToken":"token","priority":5,"decoration":false},"ntfy":{"ntfyId":"n","serverUrl":"url","topic":"topic","accessToken":null,"priority":3},"mattermost":{"mattermostId":"m","webhookUrl":"url","channel":null,"username":"user"},"custom":{"customId":"c","endpoint":"url","headers":{"X-Test":"value"}},"lark":{"larkId":"l","webhookUrl":"url"},"teams":{"teamsId":"x","webhookUrl":"url"},"pushover":{"pushoverId":"p","userKey":"key","apiToken":"token","priority":0,"retry":null,"expire":null},"unknownFutureField":{"ignored":true}}`)
	var one generated.Notification
	require.NoError(t, json.Unmarshal(fixture, &one))
	require.False(t, *one.AppDeploy)
	encoded, err := json.Marshal(one)
	require.NoError(t, err)
	var output map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &output))
	var custom map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(output["custom"], &custom))
	require.JSONEq(t, `{"X-Test":"value"}`, string(custom["headers"]))
	require.Equal(t, json.RawMessage("null"), func() json.RawMessage { var x map[string]json.RawMessage; _ = json.Unmarshal(output["pushover"], &x); return x["retry"] }())
	require.NotContains(t, string(encoded), "unknownFutureField", "unknown response fields are ignored")
	var list generated.NotificationList
	require.NoError(t, json.Unmarshal(append(append([]byte("["), fixture...), ']'), &list))
	require.Len(t, list, 1)
	require.NotNil(t, list[0].Slack)
	require.NotContains(t, output, "slackId", "omitted inactive foreign key stays absent")
	var inactive generated.Notification
	require.NoError(t, json.Unmarshal([]byte(`{"notificationId":"opaque","organizationId":"org","slack":null,"telegram":null,"discord":null,"email":null,"resend":null,"gotify":null,"ntfy":null,"mattermost":null,"custom":null,"lark":null,"teams":null,"pushover":null}`), &inactive))
	inactiveJSON, err := json.Marshal(inactive)
	require.NoError(t, err)
	var relations map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(inactiveJSON, &relations))
	for _, field := range []string{"slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"} {
		require.Equal(t, json.RawMessage("null"), relations[field], "%s null relation must remain null", field)
	}
}
