package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// Moving registration after error handling leaks acknowledged partial creates.
// Reusing the lifecycle context prevents cleanup after a timeout.
func TestNotificationLiveFixtureCleanupOrdering(t *testing.T) {
	for _, phase := range []string{"success", "partial-create", "partial-rejected", "create-missing-state-id", "create-missing-channel", "create-missing-org", "create-wrong-type", "read-error", "read-wrong-id", "read-wrong-org", "read-wrong-channel", "read-enabled", "exercise-error", "delete-error", "absence-error", "uncertain", "missing-success-id", "rejected", "throttled"} {
		t.Run(phase, func(t *testing.T) {
			var events []string
			present := true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := NotificationArgs{Name: "offline", Events: &NotificationEvents{AppDeploy: true}, Slack: &NotificationSlackConfig{WebhookURL: "https://example.invalid/hook"}}
			state := NotificationState{NotificationArgs: normalizeNotificationArgs(args), NotificationID: "offline-notification", NotificationType: "slack", ChannelID: "offline-channel", OrganizationID: "offline-org"}
			state.Events = &NotificationEvents{}
			switch phase {
			case "create-missing-state-id":
				state.NotificationID = ""
			case "create-missing-channel":
				state.ChannelID = ""
			case "create-missing-org":
				state.OrganizationID = ""
			case "create-wrong-type":
				state.NotificationType = "teams"
			}
			ops := liveNotificationFixtureOps{
				create: func(_ context.Context, a NotificationArgs) (infer.CreateResponse[NotificationState], error) {
					require.Equal(t, &NotificationEvents{}, a.Events, "all subscriptions must be disabled even for unsafe caller inputs")
					events = append(events, "create")
					created := infer.CreateResponse[NotificationState]{ID: "offline-notification", Output: state}
					switch phase {
					case "partial-create":
						cancel()
						return created, errors.New("untrusted private response")
					case "partial-rejected":
						return created, &client.APIError{StatusCode: 400, Message: "untrusted private response"}
					case "uncertain":
						return infer.CreateResponse[NotificationState]{}, errors.New("untrusted private response")
					case "missing-success-id":
						return infer.CreateResponse[NotificationState]{}, nil
					case "rejected":
						return infer.CreateResponse[NotificationState]{}, &client.APIError{StatusCode: 400, Code: "BAD_REQUEST", Message: "untrusted private response"}
					case "throttled":
						return infer.CreateResponse[NotificationState]{}, &client.APIError{StatusCode: 429, Message: "untrusted private response"}
					}
					return created, nil
				},
				read: func(_ context.Context, id string, prior NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
					require.Equal(t, state, prior, "cleanup and stateful read must retain full identity")
					require.Equal(t, "offline-notification", id)
					if !present {
						if phase == "absence-error" && len(events) == 5 {
							events = append(events, "absence-error")
							return infer.ReadResponse[NotificationArgs, NotificationState]{}, errors.New("untrusted private response")
						}
						events = append(events, "absence")
						return infer.ReadResponse[NotificationArgs, NotificationState]{}, nil
					}
					events = append(events, "read")
					if phase == "read-error" {
						return infer.ReadResponse[NotificationArgs, NotificationState]{}, errors.New("untrusted private response")
					}
					read := infer.ReadResponse[NotificationArgs, NotificationState]{ID: id, Inputs: state.NotificationArgs, State: state}
					if phase == "read-wrong-id" {
						read.ID = "unrelated"
					}
					if phase == "read-wrong-org" {
						read.State.OrganizationID = "unrelated"
					}
					if phase == "read-wrong-channel" {
						read.State.ChannelID = "unrelated"
					}
					if phase == "read-enabled" {
						read.Inputs.Events = &NotificationEvents{DockerCleanup: true}
					}
					return read, nil
				},
				remove: func(c context.Context, id string, prior NotificationState) error {
					require.NoError(t, c.Err(), "cleanup must have a fresh context")
					_, bounded := c.Deadline()
					require.True(t, bounded)
					require.Equal(t, state, prior)
					require.Equal(t, "offline-notification", id)
					events = append(events, "delete")
					if phase == "delete-error" && present && len(events) == 5 {
						return errors.New("untrusted private response")
					}
					present = false
					return nil
				},
				exercise: func(context.Context, string, NotificationState) error {
					events = append(events, "exercise")
					cancel()
					if phase == "exercise-error" {
						return errors.New("untrusted private response")
					}
					return nil
				},
				uncertain: func() { events = append(events, "stop") },
			}
			t.Run("fixture", func(t *testing.T) {
				ops.register = func(id string, prior NotificationState) func() {
					events = append(events, "register")
					return registerLiveCleanup(t, "notification", id, func(c context.Context) error {
						return ops.remove(c, id, prior)
					}, func(c context.Context) (string, error) {
						read, err := ops.read(c, id, prior)
						return read.ID, err
					})
				}
				err := runNotificationLiveFixture(t, ctx, args, ops)
				if phase == "success" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.NotContains(t, err.Error(), "untrusted private response")
				}
			})
			want := map[string][]string{
				"success":                 {"create", "register", "read", "exercise", "delete", "absence"},
				"partial-create":          {"create", "register", "delete", "absence"},
				"partial-rejected":        {"create", "register", "delete", "absence"},
				"create-missing-state-id": {"create", "register", "delete", "absence"},
				"create-missing-channel":  {"create", "register", "delete", "absence"},
				"create-missing-org":      {"create", "register", "delete", "absence"},
				"create-wrong-type":       {"create", "register", "delete", "absence"},
				"read-error":              {"create", "register", "read", "delete", "absence"},
				"read-wrong-id":           {"create", "register", "read", "delete", "absence"},
				"read-wrong-org":          {"create", "register", "read", "delete", "absence"},
				"read-wrong-channel":      {"create", "register", "read", "delete", "absence"},
				"read-enabled":            {"create", "register", "read", "delete", "absence"},
				"exercise-error":          {"create", "register", "read", "exercise", "delete", "absence"},
				"delete-error":            {"create", "register", "read", "exercise", "delete", "delete", "absence"},
				"absence-error":           {"create", "register", "read", "exercise", "delete", "absence-error", "delete", "absence"},
				"uncertain":               {"create", "stop"},
				"missing-success-id":      {"create", "stop"},
				"rejected":                {"create"},
				"throttled":               {"create", "stop"},
			}
			require.Equal(t, want[phase], events)
		})
	}
}

// Unsafe fixture addresses, enabled events, omitted channels, and an update
// that only renames the record must all fail before any live use.
func TestNotificationLiveFixtures(t *testing.T) {
	kinds := []string{"slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"}
	require.Equal(t, kinds, notificationChannels)
	names := map[string]bool{}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			name := liveRunName("notification-" + kind)
			require.False(t, names[name])
			names[name] = true
			initial := liveNotificationArgs(kind, name, false)
			updated := liveNotificationArgs(kind, name+"-updated", true)
			// Independent literals protect the defaults the live read must observe.
			normalized := normalizeNotificationArgs(initial)
			switch kind {
			case "slack":
				require.Equal(t, ptr(""), normalized.Slack.Channel)
			case "telegram":
				require.Equal(t, ptr(""), normalized.Telegram.MessageThreadID)
			case "discord":
				require.Equal(t, ptr(false), normalized.Discord.Decoration)
			case "gotify":
				require.Equal(t, ptr(5), normalized.Gotify.Priority)
				require.Equal(t, ptr(false), normalized.Gotify.Decoration)
			case "ntfy":
				require.Equal(t, ptr(3), normalized.Ntfy.Priority)
				require.Equal(t, ptr(""), normalized.Ntfy.AccessToken)
			case "mattermost":
				require.Equal(t, ptr(""), normalized.Mattermost.Channel)
				require.Equal(t, ptr(""), normalized.Mattermost.Username)
			case "custom":
				require.Equal(t, map[string]string{}, normalized.Custom.Headers)
			case "pushover":
				require.Equal(t, ptr(0), normalized.Pushover.Priority)
				require.Nil(t, normalized.Pushover.Retry)
				require.Nil(t, normalized.Pushover.Expire)
			}
			for _, args := range []NotificationArgs{initial, updated} {
				require.Empty(t, validateNotificationArgs(args))
				require.Equal(t, &NotificationEvents{}, args.Events)
				selected, err := notificationKind(args)
				require.NoError(t, err)
				require.Equal(t, kind, selected)
				block := reflect.ValueOf(notificationBlock(args, kind)).Elem()
				for i := 0; i < block.NumField(); i++ {
					value := block.Field(i)
					field := block.Type().Field(i).Name
					if value.Kind() != reflect.String {
						continue
					}
					s := value.String()
					switch field {
					case "WebhookURL", "ServerURL", "Endpoint":
						u, err := url.Parse(s)
						require.NoError(t, err)
						require.True(t, strings.HasSuffix(u.Hostname(), ".invalid"))
					case "SMTPServer", "FromAddress":
						require.True(t, strings.HasSuffix(s, ".invalid"))
					case "BotToken", "Password", "APIKey", "AppToken", "UserKey", "APIToken":
						require.True(t, strings.HasPrefix(s, "placeholder-"))
					}
				}
				if args.Email != nil {
					for _, recipient := range args.Email.ToAddresses {
						require.True(t, strings.HasSuffix(recipient, ".invalid"))
					}
				}
				if args.Resend != nil {
					for _, recipient := range args.Resend.ToAddresses {
						require.True(t, strings.HasSuffix(recipient, ".invalid"))
					}
				}
			}
			before := reflect.ValueOf(notificationBlock(normalizeNotificationArgs(initial), kind)).Elem()
			after := reflect.ValueOf(notificationBlock(normalizeNotificationArgs(updated), kind)).Elem()
			for i := 0; i < before.NumField(); i++ {
				require.NotEqual(t, before.Field(i).Interface(), after.Field(i).Interface(), "update must cover %s", before.Type().Field(i).Name)
			}
		})
	}
}

// Exercise the actual provider/client path with an in-process API. The server
// stores submitted values instead of returning the fixture's desired output.
// Any connection-test/send endpoint or reused identity is rejected.
func TestNotificationLiveAllChannelsOfflineLifecycle(t *testing.T) {
	state := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(state) })
	resetLiveHarnessState()
	records := map[string]map[string]any{}
	var creates, updates, removes []string
	finalNames := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		operation := strings.TrimPrefix(req.URL.Path, "/api/notification.")
		switch {
		case req.URL.Path == "/api/organization.active":
			_, _ = w.Write([]byte(`{"id":"offline-org"}`))
		case operation == "all":
			list := []map[string]any{}
			for _, record := range records {
				list = append(list, record)
			}
			_ = json.NewEncoder(w).Encode(list)
		case operation == "one":
			record, ok := records[req.URL.Query().Get("notificationId")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(record)
		case strings.HasPrefix(operation, "create"), strings.HasPrefix(operation, "update"):
			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			creating := strings.HasPrefix(operation, "create")
			kind := strings.ToLower(operation[6:7]) + operation[7:]
			id, _ := body["notificationId"].(string)
			if creating {
				require.Empty(t, records, "each channel must finish cleanup before the next create")
				id = "offline-" + kind
				creates = append(creates, kind)
				records[id] = map[string]any{"notificationId": id, "organizationId": "offline-org", kind + "Id": "offline-channel-" + kind}
			} else {
				updates = append(updates, kind)
				require.NotNil(t, records[id])
				require.Equal(t, "offline-channel-"+kind, body[kind+"Id"])
				name := body["name"].(string)
				if strings.HasPrefix(name, "pulumi-acceptance-") && !strings.HasSuffix(name, "-updated") {
					require.False(t, finalNames[name], "fixtures need unique names")
					finalNames[name] = true
				}
			}
			record := records[id]
			record["name"] = body["name"]
			for _, event := range []string{"appDeploy", "appBuildError", "databaseBackup", "volumeBackup", "dokployBackup", "dokployRestart", "dockerCleanup", "serverThreshold"} {
				if value, ok := body[event]; ok {
					require.Equal(t, false, value, "no mutation may subscribe to events")
				} else {
					require.True(t, creating || event == "serverThreshold" && (kind == "gotify" || kind == "ntfy"), "update must explicitly disable supported events")
				}
				record[event] = false
				delete(body, event)
			}
			delete(body, "notificationId")
			delete(body, "name")
			body[kind+"Id"] = "offline-channel-" + kind
			record[kind] = body
			w.WriteHeader(http.StatusNoContent)
		case operation == "remove":
			var body struct {
				ID string `json:"notificationId"`
			}
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.NotNil(t, records[body.ID])
			removes = append(removes, strings.TrimPrefix(body.ID, "offline-"))
			delete(records, body.ID)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Error("unexpected endpoint: notification fixture must never send or test connectivity")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	api, err := client.New(server.URL, "placeholder-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	runLiveNotificationCases(t, context.Background(), api)
	want := []string{"slack", "telegram", "discord", "email", "resend", "gotify", "ntfy", "mattermost", "custom", "lark", "teams", "pushover"}
	require.Equal(t, want, creates)
	require.Equal(t, want, removes)
	require.Len(t, updates, 24, "finalization plus explicit update per channel")
	require.Len(t, finalNames, 12)
	require.Empty(t, records)
	require.False(t, heavyLiveTierStopped())
}

func TestNotificationLiveRepeatedIndependentFixtures(t *testing.T) {
	var cleaned []string
	for index, partial := range []bool{false, true, false, true, false} {
		id := fmt.Sprintf("offline-%d", index)
		kind := notificationChannels[index]
		t.Run(id, func(t *testing.T) {
			args := liveNotificationArgs(kind, id, false)
			state := NotificationState{NotificationArgs: normalizeNotificationArgs(args), NotificationID: id, NotificationType: kind, ChannelID: id + "-channel", OrganizationID: "offline-org"}
			present := true
			ops := liveNotificationFixtureOps{
				create: func(context.Context, NotificationArgs) (infer.CreateResponse[NotificationState], error) {
					var err error
					if partial {
						err = errors.New("untrusted response")
					}
					return infer.CreateResponse[NotificationState]{ID: id, Output: state}, err
				},
				read: func(context.Context, string, NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
					if !present {
						return infer.ReadResponse[NotificationArgs, NotificationState]{}, nil
					}
					return infer.ReadResponse[NotificationArgs, NotificationState]{ID: id, Inputs: state.NotificationArgs, State: state}, nil
				},
				remove: func(_ context.Context, gotID string, gotState NotificationState) error {
					require.Equal(t, id, gotID)
					require.Equal(t, state, gotState)
					present = false
					cleaned = append(cleaned, id)
					return nil
				},
				uncertain: func() { t.Fatal("acknowledged partial failure must not stop independent channels") },
			}
			ops.register = func(id string, state NotificationState) func() {
				return registerLiveCleanup(t, "notification", id, func(c context.Context) error { return ops.remove(c, id, state) }, func(c context.Context) (string, error) { v, e := ops.read(c, id, state); return v.ID, e })
			}
			err := runNotificationLiveFixture(t, context.Background(), args, ops)
			if partial {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
		require.Len(t, cleaned, index+1, "cleanup must complete before next independent fixture")
	}
	require.Equal(t, []string{"offline-0", "offline-1", "offline-2", "offline-3", "offline-4"}, cleaned)
}

func TestNotificationLiveUncertaintyStopsWithoutGuessingIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		stop bool
	}{
		{"transport", errors.New("private response"), true},
		{"opaque-4xx", errors.New("notification request failed; response details withheld"), true},
		{"missing-success-id", nil, true},
		{"validation", fmt.Errorf("wrapped: %w", &client.APIError{StatusCode: 400, Code: "BAD_REQUEST", Message: "private response"}), false},
		{"discovery-4xx-after-mutation", notificationFailure("create discovery; creation may have occurred; inspect disabled records", &client.APIError{StatusCode: 400, Code: "BAD_REQUEST"}), true},
		{"partial-error-without-id", initFailed(notificationFailure("create finalization; refresh before repair", &client.APIError{StatusCode: 400, Code: "BAD_REQUEST"})), true},
		{"throttled", &client.APIError{StatusCode: 429}, true},
		{"server-error", &client.APIError{StatusCode: 500}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saved := snapshotLiveHarnessState()
			t.Cleanup(func() { restoreLiveHarnessState(saved) })
			resetLiveHarnessState()
			marker := filepath.Join(t.TempDir(), "stop")
			t.Setenv(liveStopMarkerEnvironment, marker)
			calls := 0
			ops := liveNotificationFixtureOps{
				create: func(context.Context, NotificationArgs) (infer.CreateResponse[NotificationState], error) {
					calls++
					return infer.CreateResponse[NotificationState]{}, tc.err
				},
				register: func(string, NotificationState) func() { t.Fatal("unacknowledged identity registered"); return nil },
				read: func(context.Context, string, NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
					t.Fatal("uncertain identity probed")
					return infer.ReadResponse[NotificationArgs, NotificationState]{}, nil
				},
				remove: func(context.Context, string, NotificationState) error {
					t.Fatal("uncertain identity deleted")
					return nil
				},
				uncertain: func() { recordStructuralCleanup("notification", errors.New("create ownership uncertain")) },
			}
			err := runNotificationLiveFixture(t, context.Background(), liveNotificationArgs("slack", "offline", false), ops)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private response")
			require.Equal(t, 1, calls, "never retry uncertain creation")
			require.Equal(t, tc.stop, heavyLiveTierStopped())
			_, markerErr := os.Stat(marker)
			if tc.stop {
				require.NoError(t, markerErr)
				// A nil client proves the serial runner gates before another API call.
				runLiveNotificationCases(t, context.Background(), nil)
				require.Len(t, snapshotLiveHarnessState().results, 1)
			} else {
				require.True(t, os.IsNotExist(markerErr))
			}
		})
	}
}

func TestNotificationLiveUnresolvedCleanupStopsAfterFallback(t *testing.T) {
	saved := snapshotLiveHarnessState()
	t.Cleanup(func() { restoreLiveHarnessState(saved) })
	resetLiveHarnessState()
	marker := filepath.Join(t.TempDir(), "stop")
	t.Setenv(liveStopMarkerEnvironment, marker)
	args := liveNotificationArgs("slack", "offline", false)
	state := NotificationState{NotificationArgs: normalizeNotificationArgs(args), NotificationID: "offline", NotificationType: "slack", ChannelID: "offline-channel", OrganizationID: "offline-org"}
	deletes := 0
	ops := liveNotificationFixtureOps{
		create: func(context.Context, NotificationArgs) (infer.CreateResponse[NotificationState], error) {
			return infer.CreateResponse[NotificationState]{ID: state.NotificationID, Output: state}, nil
		},
		read: func(context.Context, string, NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
			return infer.ReadResponse[NotificationArgs, NotificationState]{ID: state.NotificationID, Inputs: state.NotificationArgs, State: state}, nil
		},
		remove: func(c context.Context, id string, prior NotificationState) error {
			require.NoError(t, c.Err())
			require.Equal(t, state.NotificationID, id)
			require.Equal(t, state, prior)
			deletes++
			return errors.New("private response")
		},
		uncertain: func() { t.Fatal("acknowledged ownership is certain") },
	}
	t.Run("fixture", func(t *testing.T) {
		ops.register = func(id string, state NotificationState) func() {
			owner := newLiveCleanupOwner(func() {
				c, cancel := cleanupContext()
				defer cancel()
				if err := verifyLiveCleanup(c, func(c context.Context) error { return ops.remove(c, id, state) }, func(c context.Context) (string, error) { read, err := ops.read(c, id, state); return read.ID, err }); err != nil {
					recordStructuralCleanup("notification", err)
				}
			})
			t.Cleanup(owner.cleanupOnce)
			return owner.release
		}
		require.Error(t, runNotificationLiveFixture(t, context.Background(), args, ops))
	})
	require.Equal(t, 2, deletes, "failed explicit cleanup must retain fallback ownership")
	require.True(t, heavyLiveTierStopped())
	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "stop\n", string(content))
	runLiveNotificationCases(t, context.Background(), nil)
}

func TestNotificationLiveProviderPartialCreateFallback(t *testing.T) {
	for _, phase := range []string{"verification-error", "finalization-error"} {
		t.Run(phase, func(t *testing.T) {
			var calls []string
			present, failRead := false, phase == "verification-error"
			args := liveNotificationArgs("slack", liveRunName("notification-slack"), false)
			v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://notification.example.invalid/slack/initial","channel":""}`)
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.all":
					list := []generated.Notification{}
					if present {
						list = append(list, v)
					}
					_ = json.NewEncoder(w).Encode(list)
				case "/api/notification.createSlack":
					calls = append(calls, "create")
					var body struct {
						Name string `json:"name"`
					}
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					v.Name, present = &body.Name, true
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.updateSlack":
					calls = append(calls, "finalization-error")
					w.WriteHeader(http.StatusInternalServerError)
				case "/api/notification.one":
					require.Equal(t, v.NotificationId, req.URL.Query().Get("notificationId"))
					if failRead {
						calls = append(calls, "verification-error")
						failRead = false
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					if !present {
						calls = append(calls, "absence")
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(v)
				case "/api/notification.remove":
					var body struct {
						ID string `json:"notificationId"`
					}
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.Equal(t, v.NotificationId, body.ID)
					calls = append(calls, "delete")
					present = false
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Error("unexpected endpoint in partial-create cleanup")
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			ops := liveNotificationFixtureOps{
				create: func(c context.Context, a NotificationArgs) (infer.CreateResponse[NotificationState], error) {
					return r.Create(c, infer.CreateRequest[NotificationArgs]{Inputs: a})
				},
				read: func(c context.Context, id string, state NotificationState) (infer.ReadResponse[NotificationArgs, NotificationState], error) {
					return r.Read(c, infer.ReadRequest[NotificationArgs, NotificationState]{ID: id, State: state})
				},
				remove: func(c context.Context, id string, state NotificationState) error {
					_, err := r.Delete(c, infer.DeleteRequest[NotificationState]{ID: id, State: state})
					return err
				},
				uncertain: func() { t.Fatal("provider returned an acknowledged partial identity") },
			}
			t.Run("fixture", func(t *testing.T) {
				ops.register = func(id string, state NotificationState) func() {
					calls = append(calls, "register")
					require.Equal(t, id, state.NotificationID)
					require.Equal(t, "channel", state.ChannelID)
					require.Equal(t, "org", state.OrganizationID)
					require.Equal(t, "slack", state.NotificationType)
					return registerLiveCleanup(t, "notification", id, func(c context.Context) error { return ops.remove(c, id, state) }, func(c context.Context) (string, error) { v, err := ops.read(c, id, state); return v.ID, err })
				}
				require.Error(t, runNotificationLiveFixture(t, context.Background(), args, ops))
			})
			require.False(t, present)
			require.Equal(t, []string{"create", phase, "register", "delete", "absence", "absence"}, calls)
		})
	}
}
