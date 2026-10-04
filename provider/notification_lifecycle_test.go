package dokploy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestNotificationCreateAllChannels(t *testing.T) {
	for kind, tc := range notificationProjectionCases() {
		t.Run(kind, func(t *testing.T) {
			args := tc.want
			args.Name = "requested"
			args.Events = &NotificationEvents{AppDeploy: true, DockerCleanup: true}
			v := observedNotification(t, kind, tc.settings)
			v.NotificationId = "placeholder-notification"
			var marker string
			var calls []string
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				calls = append(calls, req.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.all":
					if marker == "" {
						_, _ = w.Write([]byte(`[]`))
					} else {
						_ = json.NewEncoder(w).Encode([]generated.Notification{v})
					}
				case "/api/notification.create" + strings.ToUpper(kind[:1]) + kind[1:]:
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					marker, _ = body["name"].(string)
					require.True(t, strings.HasPrefix(marker, "pulumi-notification-"))
					_, err := uuid.Parse(strings.TrimPrefix(marker, "pulumi-notification-"))
					require.NoError(t, err)
					require.Equal(t, false, body["appDeploy"])
					v.Name = &marker
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.one":
					require.Equal(t, v.NotificationId, req.URL.Query().Get("notificationId"))
					_ = json.NewEncoder(w).Encode(v)
				case "/api/notification.update" + strings.ToUpper(kind[:1]) + kind[1:]:
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.Equal(t, v.NotificationId, body["notificationId"])
					require.Equal(t, "channel", body[kind+"Id"])
					require.Equal(t, true, body["appDeploy"])
					v.Name = ptr("requested")
					v.AppDeploy = ptr(true)
					v.DockerCleanup = ptr(true)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected notification endpoint")
				}
			})
			got, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: args})
			require.NoError(t, err)
			require.Equal(t, v.NotificationId, got.ID)
			require.Equal(t, "requested", got.Output.Name)
			require.True(t, got.Output.Events.AppDeploy)
			require.Equal(t, []string{"/api/organization.active", "/api/notification.all", "/api/notification.create" + strings.ToUpper(kind[:1]) + kind[1:], "/api/notification.all", "/api/notification.one", "/api/notification.update" + strings.ToUpper(kind[:1]) + kind[1:], "/api/notification.one"}, calls)
		})
	}
}

func TestNotificationCreateDryRunNoCalls(t *testing.T) {
	r := Notification{client: func(_ context.Context) *client.Client { t.Fatal("preview invoked client factory"); return nil }}
	got, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: notificationTestArgs("slack"), DryRun: true})
	require.NoError(t, err)
	require.Empty(t, got.ID)
}

func TestNotificationCreateFailureStates(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                                                                     string
		listDelay                                                                                                                                int
		duplicate, wrongConfig, malformed, listError, createError, readError, wrongReadID, transportRead, finalError, finalMismatch, renameError bool
		wantID                                                                                                                                   bool
	}{
		{name: "zero candidates"},
		{name: "second list visibility", listDelay: 1, wantID: true},
		{name: "duplicate marker", duplicate: true},
		{name: "wrong configuration", wrongConfig: true},
		{name: "malformed snapshot", malformed: true},
		{name: "list error", listError: true},
		{name: "uncertain zero", createError: true},
		{name: "uncertain one", createError: true, wantID: true, listDelay: 1},
		{name: "verified read 404", wantID: true, readError: true},
		{name: "verified read wrong ID", wantID: true, wrongReadID: true},
		{name: "verified read transport", wantID: true, transportRead: true},
		{name: "final rename rejection", wantID: true, renameError: true},
		{name: "final read failure", wantID: true, finalError: true},
		{name: "final read mismatch", wantID: true, finalMismatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := notificationTestArgs("slack")
			args.Events = &NotificationEvents{AppDeploy: true}
			v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
			v.NotificationId = "placeholder-notification"
			var marker string
			createPosts, deletePosts, afterLists, updates := 0, 0, 0, 0
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.all":
					if marker == "" {
						if tc.malformed {
							_, _ = w.Write([]byte(`[{"notificationId":""}]`))
						} else {
							_, _ = w.Write([]byte(`[]`))
						}
						return
					}
					afterLists++
					if tc.listError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					if tc.name == "zero candidates" || tc.name == "uncertain zero" || afterLists <= tc.listDelay {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					x := v
					if tc.wrongConfig {
						x.SlackId.Set("wrong")
					}
					if tc.duplicate {
						_ = json.NewEncoder(w).Encode([]generated.Notification{x, x})
						return
					}
					_ = json.NewEncoder(w).Encode([]generated.Notification{x})
				case "/api/notification.createSlack":
					createPosts++
					var body struct {
						Name string `json:"name"`
					}
					_ = json.NewDecoder(req.Body).Decode(&body)
					marker = body.Name
					v.Name = &marker
					if tc.createError {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.one":
					if tc.readError || tc.finalError && updates > 0 {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					if tc.transportRead {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					if tc.wrongReadID {
						x := v
						x.NotificationId = "other"
						_ = json.NewEncoder(w).Encode(x)
						return
					}
					if tc.finalMismatch && updates > 0 {
						v.AppDeploy = ptr(false)
					}
					_ = json.NewEncoder(w).Encode(v)
				case "/api/notification.updateSlack":
					updates++
					if tc.renameError {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					v.Name = ptr(args.Name)
					v.AppDeploy = ptr(true)
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.remove":
					deletePosts++
				default:
					t.Errorf("unexpected endpoint")
				}
			})
			got, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: args})
			if tc.name == "second list visibility" {
				require.NoError(t, err)
				require.Equal(t, 2, afterLists)
			} else {
				require.Error(t, err)
			}
			if tc.wantID {
				require.Equal(t, v.NotificationId, got.ID)
				if err != nil {
					var partial infer.ResourceInitFailedError
					require.ErrorAs(t, err, &partial)
					require.Equal(t, marker, got.Output.Name)
					require.False(t, got.Output.Events.AppDeploy)
				}
			} else {
				require.Empty(t, got.ID)
			}
			if tc.malformed {
				require.Zero(t, createPosts)
			} else {
				require.Equal(t, 1, createPosts)
			}
			require.Zero(t, deletePosts)
			if tc.name == "zero candidates" || tc.name == "uncertain zero" {
				require.Equal(t, 3, afterLists)
			}
			if tc.createError {
				require.Zero(t, updates)
			}
		})
	}
}

func TestNotificationCreateUnreadableAckBody(t *testing.T) {
	// Raw mutation must ignore the body; discovery still runs after 2xx headers.
	args := notificationTestArgs("slack")
	var marker string
	listed := 0
	v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
	r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/organization.active":
			_, _ = w.Write([]byte(`{"id":"org"}`))
		case "/api/notification.all":
			if marker == "" {
				_, _ = w.Write([]byte(`[]`))
			} else {
				listed++
				_ = json.NewEncoder(w).Encode([]generated.Notification{v})
			}
		case "/api/notification.createSlack":
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			marker = body.Name
			v.Name = &marker
			w.Header().Set("Content-Length", "80")
			_, _ = w.Write([]byte("short"))
		case "/api/notification.one":
			_ = json.NewEncoder(w).Encode(v)
		case "/api/notification.updateSlack":
			v.Name = ptr(args.Name)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected endpoint")
		}
	})
	got, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: args})
	require.NoError(t, err)
	require.NotEmpty(t, got.ID)
	require.Equal(t, 1, listed)
}

func TestNotificationUpdateDryRunNoCalls(t *testing.T) {
	r := Notification{client: func(context.Context) *client.Client { t.Fatal("preview invoked client factory"); return nil }}
	state := NotificationState{NotificationArgs: notificationTestArgs("slack"), NotificationID: "placeholder-id", ChannelID: "placeholder-channel", OrganizationID: "placeholder-org", NotificationType: "slack"}
	got, err := r.Update(t.Context(), infer.UpdateRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state, Inputs: notificationTestArgs("slack"), DryRun: true})
	require.NoError(t, err)
	require.Equal(t, state.NotificationID, got.Output.NotificationID)
	require.Equal(t, state.ChannelID, got.Output.ChannelID)
	require.Equal(t, state.OrganizationID, got.Output.OrganizationID)
}

func TestNotificationReadUpdateDelete(t *testing.T) {
	for kind, tc := range notificationProjectionCases() {
		t.Run("import "+kind, func(t *testing.T) {
			v := observedNotification(t, kind, tc.settings)
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.one":
					_ = json.NewEncoder(w).Encode(v)
				default:
					t.Errorf("unexpected endpoint")
				}
			})
			got, err := r.Read(t.Context(), infer.ReadRequest[NotificationArgs, NotificationState]{ID: v.NotificationId})
			require.NoError(t, err)
			require.Equal(t, v.NotificationId, got.ID)
			require.Equal(t, kind, got.State.NotificationType)
			require.Equal(t, "channel", got.State.ChannelID)
			require.Equal(t, "org", got.State.OrganizationID)
			require.Equal(t, notificationBlock(tc.want, kind), notificationBlock(got.Inputs, kind))
		})
	}
	for _, tc := range []struct {
		name                                              string
		readStatus, removeStatus                          int
		wrongOrg, wrongChannel, mismatch, keepAfterDelete bool
		wantUpdates, wantRemoves                          int
		wantErr                                           bool
	}{
		{name: "update and delete", wantUpdates: 1, wantRemoves: 1},
		{name: "substituted channel", wrongChannel: true, wantErr: true},
		{name: "organization changed", wrongOrg: true, wantErr: true},
		{name: "update mismatch", mismatch: true, wantUpdates: 1, wantErr: true},
		{name: "read not found", readStatus: 404},
		{name: "BAD_REQUEST present", removeStatus: 400, keepAfterDelete: true, wantRemoves: 1, wantErr: true},
		{name: "BAD_REQUEST absent", removeStatus: 400, wantRemoves: 1},
		{name: "acknowledged delete still present", keepAfterDelete: true, wantRemoves: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := observedNotification(t, "slack", `{"slackId":"channel","webhookUrl":"https://example.com/hook","channel":""}`)
			v.NotificationId = "placeholder-notification"
			state, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			updated := state.NotificationArgs
			updated.Name = "updated"
			updated.Events = &NotificationEvents{DatabaseBackup: true}
			updated.Slack = &NotificationSlackConfig{WebhookURL: "https://example.com/rotated", Channel: ptr("alerts")}
			updates, removes, reads := 0, 0, 0
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					if tc.wrongOrg {
						_, _ = w.Write([]byte(`{"id":"other"}`))
					} else {
						_, _ = w.Write([]byte(`{"id":"org"}`))
					}
				case "/api/notification.one":
					reads++
					if tc.readStatus != 0 || removes > 0 && !tc.keepAfterDelete {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					x := v
					if tc.wrongChannel {
						x.SlackId.Set("other")
					}
					_ = json.NewEncoder(w).Encode(x)
				case "/api/notification.updateSlack":
					updates++
					var body map[string]any
					_ = json.NewDecoder(req.Body).Decode(&body)
					require.Equal(t, state.NotificationID, body["notificationId"])
					require.Equal(t, state.ChannelID, body["slackId"])
					require.Equal(t, true, body["databaseBackup"])
					require.Equal(t, false, body["appDeploy"])
					if !tc.mismatch {
						v.Name = ptr(updated.Name)
						v.DatabaseBackup = ptr(true)
						x, _ := v.Slack.Get()
						x.WebhookUrl.Set(updated.Slack.WebhookURL)
						x.Channel.Set(*updated.Slack.Channel)
						v.Slack.Set(x)
					}
					w.WriteHeader(http.StatusNoContent)
				case "/api/notification.remove":
					removes++
					if tc.removeStatus != 0 {
						w.WriteHeader(tc.removeStatus)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected endpoint")
				}
			})
			if tc.name == "update and delete" || tc.name == "update mismatch" || tc.wrongOrg || tc.wrongChannel {
				got, updateErr := r.Update(t.Context(), infer.UpdateRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state, Inputs: updated})
				if tc.wantErr {
					require.Error(t, updateErr)
					require.NotEqual(t, updated.Name, got.Output.Name)
				} else {
					require.NoError(t, updateErr)
					require.Equal(t, updated.Name, got.Output.Name)
					require.Equal(t, updated.Slack.WebhookURL, got.Output.Slack.WebhookURL)
					_, deleteErr := r.Delete(t.Context(), infer.DeleteRequest[NotificationState]{ID: state.NotificationID, State: got.Output})
					require.NoError(t, deleteErr)
				}
			} else if tc.name == "read not found" {
				got, readErr := r.Read(t.Context(), infer.ReadRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state})
				require.NoError(t, readErr)
				require.Empty(t, got.ID)
			} else {
				_, deleteErr := r.Delete(t.Context(), infer.DeleteRequest[NotificationState]{ID: state.NotificationID, State: state})
				if tc.wantErr {
					require.Error(t, deleteErr)
				} else {
					require.NoError(t, deleteErr)
				}
			}
			require.Equal(t, tc.wantUpdates, updates)
			require.Equal(t, tc.wantRemoves, removes)
			if tc.wrongOrg {
				require.Zero(t, reads)
			}
		})
	}
}

func TestNotificationUnsupportedThresholdGuard(t *testing.T) {
	for _, kind := range []string{"gotify", "ntfy"} {
		t.Run(kind, func(t *testing.T) {
			tc := notificationProjectionCases()[kind]
			v := observedNotification(t, kind, tc.settings)
			v.ServerThreshold = ptr(true)
			state, err := notificationStateFrom(&v, nil, notificationIdentity{NotificationID: v.NotificationId, OrganizationID: "org"})
			require.NoError(t, err)
			posts := 0
			r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/api/organization.active":
					_, _ = w.Write([]byte(`{"id":"org"}`))
				case "/api/notification.one":
					_ = json.NewEncoder(w).Encode(v)
				default:
					posts++
				}
			})
			got, err := r.Update(t.Context(), infer.UpdateRequest[NotificationArgs, NotificationState]{ID: state.NotificationID, State: state, Inputs: notificationTestArgs(kind)})
			require.Error(t, err)
			require.True(t, got.Output.Events.ServerThreshold)
			require.Zero(t, posts)
		})
	}
}

func TestNotificationCreateCanceledDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	posts, lists := 0, 0
	r := notificationTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/organization.active":
			_, _ = w.Write([]byte(`{"id":"org"}`))
		case "/api/notification.all":
			lists++
			_, _ = w.Write([]byte(`[]`))
		case "/api/notification.createSlack":
			posts++
			w.WriteHeader(http.StatusNoContent)
			cancel()
		default:
			t.Errorf("unexpected endpoint")
		}
	})
	got, err := r.Create(ctx, infer.CreateRequest[NotificationArgs]{Inputs: notificationTestArgs("slack")})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, got.ID)
	require.Equal(t, 1, posts)
	require.Equal(t, 1, lists)
}

func TestNotificationReadIncompletePriorIdentity(t *testing.T) {
	r := Notification{client: func(context.Context) *client.Client { t.Fatal("incomplete identity touched HTTP"); return nil }}
	_, err := r.Read(t.Context(), infer.ReadRequest[NotificationArgs, NotificationState]{ID: "placeholder-id", State: NotificationState{NotificationID: "placeholder-id", OrganizationID: "org"}})
	require.Error(t, err)
}
