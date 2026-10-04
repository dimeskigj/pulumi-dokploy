package dokploy

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestNotificationSafeErrorCategories(t *testing.T) {
	const sensitive = "fixture-sensitive-value"
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"allowlisted", &client.APIError{StatusCode: 404, Code: "NOT_FOUND", Message: sensitive, Operation: sensitive}, 404, "NOT_FOUND"},
		{"untrusted code", &client.APIError{StatusCode: 400, Code: sensitive, Message: sensitive}, 400, ""},
		{"transport", errors.New("https://example.com/" + sensitive), 0, ""},
		{"decoder", errors.New("json: " + sensitive), 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeNotificationError(tc.err)
			require.Error(t, got)
			require.False(t, strings.Contains(got.Error(), sensitive))
			var api *client.APIError
			if tc.status != 0 {
				require.ErrorAs(t, got, &api)
				require.Equal(t, tc.status, api.StatusCode)
				require.Equal(t, tc.code, api.Code)
			}
		})
	}
	for _, original := range []error{context.Canceled, context.DeadlineExceeded} {
		require.ErrorIs(t, sanitizeNotificationError(original), original)
	}
}

func TestNotificationSafeErrors(t *testing.T) {
	const sensitive = "fixture-sensitive-credential"
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server prose", 400, `{"code":"` + sensitive + `","message":"` + sensitive + `"}`},
		{"server transformed secret", 503, `{"code":"BAD_REQUEST","message":"unexpected-` + sensitive + `"}`},
		{"malformed decoder", 200, `{"id":"` + sensitive + `"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			args := notificationTestArgs("slack")
			args.Slack.WebhookURL = "https://example.com/" + sensitive
			_, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: args})
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), "notification.create"))
			require.False(t, strings.Contains(err.Error(), sensitive))
		})
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		require.ErrorIs(t, notificationFailure("read", err), err)
	}
	r := Notification{client: func(context.Context) *client.Client { t.Fatal("validation touched HTTP"); return nil }}
	invalid := notificationTestArgs("slack")
	invalid.Slack.WebhookURL = "https://example.com/" + sensitive
	invalid.Name = ""
	_, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: invalid})
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), sensitive))
}
