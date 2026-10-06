package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
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

func TestNotificationSensitiveMutationAndReadbackErrors(t *testing.T) {
	const sensitive = "fixture-sensitive-credential"
	for _, phase := range []string{"update mutation", "update read", "delete mutation", "delete read", "create final read"} {
		t.Run(phase, func(t *testing.T) {
			v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/`+sensitive+`","channel":""}`)
			v.NotificationId = "placeholder-notification"
			state, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			updated := state.NotificationArgs
			updated.Name = "updated"
			var marker string
			mutations, reads := 0, 0
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fail := func() {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"code":"BAD_REQUEST","message":"` + sensitive + `"}`))
				}
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.all":
					if marker == "" {
						_, _ = w.Write([]byte(`[]`))
					} else {
						_ = json.NewEncoder(w).Encode([]generated.Notification{v})
					}
				case "/api/notification.createSlack":
					var body struct {
						Name string `json:"name"`
					}
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					marker = body.Name
					v.Name = &marker
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.one":
					reads++
					if phase == "update read" && mutations > 0 || phase == "delete read" && mutations > 0 || phase == "create final read" && mutations > 0 {
						fail()
					} else {
						_ = json.NewEncoder(w).Encode(v)
					}
				case "/api/notification.updateSlack":
					mutations++
					if phase == "update mutation" {
						fail()
					} else {
						if phase == "create final read" {
							v.Name = ptr("example")
						} else {
							v.Name = ptr(updated.Name)
						}
						w.WriteHeader(http.StatusNoContent)
					}
				case "/api/notification.remove":
					mutations++
					if phase == "delete mutation" {
						fail()
					} else {
						w.WriteHeader(http.StatusNoContent)
					}
				default:
					t.Errorf("unexpected endpoint")
				}
			})
			if phase == "create final read" {
				createArgs := notificationTestArgs("slack")
				createArgs.Slack.WebhookURL = "https://example.com/" + sensitive
				_, err = r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: createArgs})
			} else if strings.HasPrefix(phase, "update") {
				_, err = r.Update(t.Context(), infer.UpdateRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state, Inputs: updated})
			} else {
				_, err = r.Delete(t.Context(), infer.DeleteRequest[NotificationState]{ID: state.NotificationID, State: state})
			}
			require.Error(t, err)
			require.NotContains(t, err.Error(), sensitive)
			require.Equal(t, 1, mutations)
			require.GreaterOrEqual(t, reads, 1)
		})
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
