package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func notificationBodyMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(b, &result))
	return result
}

func notificationBodyCase(t *testing.T, kind string, create, update any, settings map[string]any, idKey string, hasThreshold bool) {
	t.Helper()
	for _, tc := range []struct {
		name   string
		body   any
		events bool
	}{{"create", create, false}, {"update", update, true}} {
		t.Run(kind+"/"+tc.name, func(t *testing.T) {
			body := notificationBodyMap(t, tc.body)
			want := map[string]any{"name": "requested-name"}
			for key, value := range settings {
				want[key] = value
			}
			for _, key := range []string{"appDeploy", "appBuildError", "databaseBackup", "volumeBackup", "dokployBackup", "dokployRestart", "dockerCleanup"} {
				want[key] = tc.events
			}
			if hasThreshold {
				want["serverThreshold"] = tc.events
			}
			if tc.name == "update" {
				want["notificationId"] = "placeholder-notification"
				want[idKey] = "placeholder-channel"
			}
			require.Equal(t, want, body, "unexpected fields in %s body", kind)
			require.NotContains(t, body, "organizationId")
		})
	}
}

func TestNotificationChannelEndpoints(t *testing.T) {
	for _, kind := range notificationChannels {
		t.Run(kind, func(t *testing.T) {
			for _, operation := range []string{"create", "update"} {
				t.Run(operation, func(t *testing.T) {
					calls := 0
					r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
						calls++
						require.Equal(t, http.MethodPost, req.Method)
						require.Equal(t, "/api/notification."+operation+strings.ToUpper(kind[:1])+kind[1:], req.URL.Path)
						body := map[string]any{}
						require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
						require.NotContains(t, body, "organizationId")
						if operation == "create" {
							require.Equal(t, "temporary-name", body["name"])
							require.Equal(t, false, body["appDeploy"])
						} else {
							require.Equal(t, "placeholder-notification", body["notificationId"])
							require.Equal(t, "placeholder-channel", body[kind+"Id"])
							require.Equal(t, true, body["appDeploy"])
						}
						if kind == notificationCustom {
							require.Equal(t, map[string]any{"X.A[B]": "token-like-placeholder"}, body["headers"])
						}
						if operation == "create" {
							w.WriteHeader(http.StatusNoContent)
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte("unrelated success body"))
						}
					})
					a := notificationTestArgs(kind)
					a.Events = &NotificationEvents{AppDeploy: true}
					if kind == notificationCustom {
						a.Custom.Headers = map[string]string{"X.A[B]": "token-like-placeholder"}
					}
					api := r.client(context.Background())
					var err error
					if operation == "create" {
						err = createNotificationChannel(context.Background(), api, "temporary-name", a)
					} else {
						err = updateNotificationChannel(context.Background(), api, "placeholder-notification", "placeholder-channel", a)
					}
					require.NoError(t, err)
					require.Equal(t, 1, calls)
				})
			}
		})
	}
}

type notificationCloseBody struct{ closed bool }

func (b *notificationCloseBody) Read([]byte) (int, error) { return 0, errors.New("irrelevant body") }
func (b *notificationCloseBody) Close() error             { b.closed = true; return errors.New("irrelevant close") }

func TestNotificationChannelMutationResult(t *testing.T) {
	for _, status := range []int{200, 204, 400} {
		body := &notificationCloseBody{}
		err := notificationMutationResult(&http.Response{StatusCode: status, Body: body}, nil)
		require.True(t, body.closed)
		if status < 300 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.Error(t, notificationMutationResult(nil, errors.New("transport failed")))
	require.Error(t, notificationMutationResult(nil, nil))
}
