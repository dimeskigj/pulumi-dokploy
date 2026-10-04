package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// Use the real generated response decoder and the scripted HTTP server; an
// incomplete successful response must not be patched up from prior state.
func TestServerReadRequiredFieldPresenceMatrix(t *testing.T) {
	fields := []string{"serverId", "name", "ipAddress", "port", "username", "serverType", "enableDockerCleanup", "organizationId", "serverStatus"}
	for _, field := range fields {
		for _, form := range []string{"missing", "null", "malformed"} {
			t.Run(field+"/"+form, func(t *testing.T) {
				var record map[string]any
				require.NoError(t, json.Unmarshal(serverRecord("node"), &record))
				switch form {
				case "missing":
					delete(record, field)
				case "null":
					record[field] = nil
				case "malformed":
					// Each required flat field is a scalar; an object is invalid
					// for strings, integers, and booleans alike.
					record[field] = map[string]any{"privateKey": "fixture-private-key", "token": "fixture-monitoring-token"}
				}
				s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: mustJSON(record)})
				prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server", OrganizationID: ptr("placeholder-org"), Status: ptr("active")}
				got, err := (Server{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: prior.ServerArgs, State: prior})
				require.Error(t, err)
				require.Empty(t, got.ID)
				require.NotContains(t, err.Error(), "fixture-private-key")
				require.NotContains(t, err.Error(), "fixture-monitoring-token")
				require.NotContains(t, err.Error(), "placeholder-server")
			})
		}
	}
}

func TestServerReadDriftAndNullableFields(t *testing.T) {
	prior := ServerState{ServerArgs: ServerArgs{Name: "old", Description: ptr("old description"), IPAddress: "old.example", Port: 65535, Username: "old", SSHKeyID: ptr("old-key"), ServerType: "build", EnableDockerCleanup: true}, ServerID: "placeholder-server", OrganizationID: ptr("old-org"), Status: ptr("old-status")}
	for _, tc := range []struct {
		name        string
		description any
		sshKey      any
		wantDesc    *string
		wantKey     *string
	}{
		{"empty description", "", nil, ptr(""), nil},
		{"null fields", nil, nil, nil, nil},
		{"omitted fields", nil, nil, nil, nil},
		{"key present", "updated", "placeholder-key", ptr("updated"), ptr("placeholder-key")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var record map[string]any
			require.NoError(t, json.Unmarshal(serverRecord("node"), &record))
			record["serverType"] = "future-type"
			record["serverStatus"] = "future-status"
			record["description"], record["sshKeyId"] = tc.description, tc.sshKey
			if tc.name == "omitted fields" {
				delete(record, "description")
				delete(record, "sshKeyId")
			}
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: mustJSON(record)})
			got, err := (Server{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: prior.ServerArgs, State: prior})
			require.NoError(t, err)
			require.Equal(t, "placeholder-server", got.ID)
			require.Equal(t, "node", got.Inputs.Name)
			require.Equal(t, "192.0.2.10", got.Inputs.IPAddress)
			require.Equal(t, 22, got.Inputs.Port)
			require.Equal(t, "root", got.Inputs.Username)
			require.False(t, got.Inputs.EnableDockerCleanup)
			require.Equal(t, "future-type", got.Inputs.ServerType)
			require.Equal(t, tc.wantDesc, got.Inputs.Description)
			require.Equal(t, tc.wantKey, got.Inputs.SSHKeyID)
			require.Equal(t, ptr("placeholder-org"), got.State.OrganizationID)
			require.Equal(t, ptr("future-status"), got.State.Status)
			require.Equal(t, got.Inputs, got.State.ServerArgs)
			encoded, err := json.Marshal(got.State)
			require.NoError(t, err)
			for _, unmanaged := range []string{"fixture-private-key", "monitoring", "command", "sshKey\""} {
				require.NotContains(t, string(encoded), unmanaged)
			}
		})
	}
}

func serverMutationBody(name string, description, sshKey any, cleanup bool) map[string]any {
	return map[string]any{"serverId": "placeholder-server", "name": name, "ipAddress": "192.0.2.10", "port": 22, "username": "root", "serverType": "deploy", "description": description, "sshKeyId": sshKey, "enableDockerCleanup": cleanup}
}

func TestServerUpdateNullableClearAndExplicitFalse(t *testing.T) {
	priorArgs := serverArgs()
	priorArgs.Description = ptr("old description")
	priorArgs.SSHKeyID = ptr("placeholder-key")
	priorArgs.EnableDockerCleanup = true
	prior := ServerState{ServerArgs: priorArgs, ServerID: "placeholder-server", OrganizationID: ptr("placeholder-org"), Status: ptr("active")}
	next := serverArgs()
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: serverMutationBody("node", nil, nil, false), Status: 200, Response: []byte(`{"serverId":"placeholder-server"}`)},
		scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: serverRecord("node")},
	)
	got, err := (Server{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, State: prior, Inputs: next})
	require.NoError(t, err)
	require.Equal(t, next, got.Output.ServerArgs)
	require.Equal(t, prior.OrganizationID, got.Output.OrganizationID)
	require.False(t, got.Output.EnableDockerCleanup)
}

func TestServerInactiveUpdateNotFoundPreservesPriorState(t *testing.T) {
	prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server", OrganizationID: ptr("placeholder-org"), Status: ptr("inactive")}
	next := serverArgs()
	next.Name = "after"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: serverMutationBody("after", nil, nil, false), Status: 400, Response: []byte(`{"code":"NOT_FOUND","message":"fixture-monitoring-token placeholder-server"}`)})
	got, err := (Server{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, State: prior, Inputs: next})
	require.Error(t, err)
	require.Equal(t, prior, got.Output)
	require.NotContains(t, err.Error(), "placeholder-server")
	require.NotContains(t, err.Error(), "fixture-monitoring-token")
}

func TestServerDeleteRefusalAndNotFoundVerification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		removeCode int
		readCode   int
		readBody   []byte
		wantError  bool
	}{
		{"workloads refusal", 400, 0, nil, true},
		{"not found and absent", 404, 404, []byte(`{}`), false},
		{"not found but present", 404, 200, serverRecord("node"), true},
		{"not found but read failed", 404, 403, []byte(`{"message":"fixture-private-key fixture-monitoring-token"}`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := []scriptedRequest{{Method: http.MethodPost, Path: "/api/server.remove", Body: map[string]any{"serverId": "placeholder-server"}, Status: tc.removeCode, Response: []byte(`{"message":"fixture-private-key fixture-monitoring-token 192.0.2.10 placeholder-server"}`)}}
			if tc.readCode != 0 {
				requests = append(requests, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: tc.readCode, Response: tc.readBody})
			}
			s := newScriptedServer(t, requests...)
			_, err := (Server{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[ServerState]{ID: "placeholder-server"})
			if tc.wantError {
				require.Error(t, err)
				for _, marker := range []string{"fixture-private-key", "fixture-monitoring-token", "192.0.2.10", "placeholder-server"} {
					require.NotContains(t, err.Error(), marker)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestServerMutationAcknowledgments(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing", `{}`}, {"empty", `{"serverId":""}`}, {"null", `{"serverId":null}`},
		{"malformed", `{"serverId":{"privateKey":"fixture-private-key"}}`},
		{"broken json", `{"serverId":`},
	} {
		t.Run("create/"+tc.name, func(t *testing.T) {
			body := serverMutationBody("node", nil, nil, false)
			delete(body, "serverId")
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.create", Body: body, Status: 200, Response: []byte(tc.body)})
			got, err := (Server{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: serverArgs()})
			require.Error(t, err)
			require.Empty(t, got.ID)
			require.NotContains(t, err.Error(), "fixture-private-key")
		})
		t.Run("update/"+tc.name, func(t *testing.T) {
			prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server"}
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: serverMutationBody("node", nil, nil, false), Status: 200, Response: []byte(tc.body)})
			got, err := (Server{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: prior.ServerArgs, State: prior})
			require.Error(t, err)
			require.Equal(t, prior, got.Output)
			require.NotContains(t, err.Error(), "fixture-private-key")
		})
	}
	for _, tc := range []struct {
		name   string
		create bool
		body   string
	}{
		{"create valid id malformed unrelated", true, `{"serverId":"placeholder-server","monitoring":{"token":42},"sshKey":{"privateKey":false}}`},
		{"update valid id malformed unrelated", false, `{"serverId":"placeholder-server","monitoring":{"token":42},"sshKey":{"privateKey":false}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := serverMutationBody("node", nil, nil, false)
			endpoint := "/api/server.update"
			if tc.create {
				endpoint = "/api/server.create"
				delete(body, "serverId")
			}
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: endpoint, Body: body, Status: 200, Response: []byte(tc.body)}, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 404, Response: []byte(`{}`)})
			r := Server{client: fixedClient(s.API())}
			var err error
			if tc.create {
				got, e := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: serverArgs()})
				err = e
				require.Equal(t, "placeholder-server", got.ID)
				require.Equal(t, "placeholder-server", got.Output.ServerID)
			} else {
				got, e := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: "placeholder-server", Inputs: serverArgs(), State: ServerState{ServerID: "placeholder-server"}})
				err = e
				require.Equal(t, "placeholder-server", got.Output.ServerID)
			}
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
		})
	}
}

func TestServerAcknowledgedFallbackProjectsOnlySafeMatchingFields(t *testing.T) {
	for _, tc := range []struct {
		name, acknowledgment string
		create               bool
		want                 ServerState
	}{
		{
			name: "create persisted configuration and metadata", create: true,
			acknowledgment: `{"serverId":"placeholder-server","name":"persisted","description":"","ipAddress":"2001:db8::10","port":65535,"username":"operator","sshKeyId":null,"serverType":"build","enableDockerCleanup":false,"organizationId":"placeholder-new-org","serverStatus":"inactive","sshKey":{"privateKey":"fixture-private-key"},"monitoring":{"token":"fixture-monitoring-token"},"command":"fixture-command"}`,
			want:           ServerState{ServerArgs: ServerArgs{Name: "persisted", Description: ptr(""), IPAddress: "2001:db8::10", Port: 65535, Username: "operator", ServerType: "build", EnableDockerCleanup: false}, ServerID: "placeholder-server", OrganizationID: ptr("placeholder-new-org"), Status: ptr("inactive")},
		},
		{
			name: "create independently skips invalid and absent fields", create: true,
			acknowledgment: `{"serverId":"placeholder-server","name":{"privateKey":"fixture-private-key"},"description":42,"ipAddress":"persisted.example","port":22.5,"username":null,"sshKeyId":false,"serverType":{},"enableDockerCleanup":0,"organizationId":[],"serverStatus":"observed","sshKey":{"privateKey":"fixture-private-key"}}`,
			want:           ServerState{ServerArgs: ServerArgs{Name: "node", Description: ptr("before"), IPAddress: "persisted.example", Port: 22, Username: "root", SSHKeyID: ptr("placeholder-key"), ServerType: "deploy", EnableDockerCleanup: true}, ServerID: "placeholder-server", Status: ptr("observed")},
		},
		{
			name: "update persisted configuration and metadata", create: false,
			acknowledgment: `{"serverId":"placeholder-server","name":"persisted","description":null,"ipAddress":"2001:db8::10","port":1,"username":"operator","sshKeyId":"placeholder-new-key","serverType":"build","enableDockerCleanup":false,"organizationId":"placeholder-new-org","serverStatus":"inactive","sshKey":{"privateKey":"fixture-private-key"}}`,
			want:           ServerState{ServerArgs: ServerArgs{Name: "persisted", IPAddress: "2001:db8::10", Port: 1, Username: "operator", SSHKeyID: ptr("placeholder-new-key"), ServerType: "build", EnableDockerCleanup: false}, ServerID: "placeholder-server", OrganizationID: ptr("placeholder-new-org"), Status: ptr("inactive")},
		},
		{
			name: "update invalid fields skipped and absent metadata retained", create: false,
			acknowledgment: `{"serverId":"placeholder-server","name":"persisted","description":null,"ipAddress":false,"port":65536,"username":[],"sshKeyId":{},"serverType":null,"enableDockerCleanup":"false","organizationId":{"privateKey":"fixture-private-key"},"sshKey":{"privateKey":"fixture-private-key"}}`,
			want:           ServerState{ServerArgs: ServerArgs{Name: "persisted", IPAddress: "192.0.2.10", Port: 22, Username: "root", SSHKeyID: ptr("placeholder-key"), ServerType: "deploy", EnableDockerCleanup: true}, ServerID: "placeholder-server", OrganizationID: ptr("placeholder-old-org"), Status: ptr("active")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := serverArgs()
			a.Description = ptr("before")
			a.SSHKeyID = ptr("placeholder-key")
			a.EnableDockerCleanup = true
			prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server", OrganizationID: ptr("placeholder-old-org"), Status: ptr("active")}
			body := serverMutationBody("node", "before", "placeholder-key", true)
			endpoint := "/api/server.update"
			if tc.create {
				endpoint = "/api/server.create"
				delete(body, "serverId")
			}
			s := newScriptedServer(t,
				scriptedRequest{Method: http.MethodPost, Path: endpoint, Body: body, Status: 200, Response: []byte(tc.acknowledgment)},
				scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 404, Response: []byte(`{}`)},
			)
			r := Server{client: fixedClient(s.API())}
			var got ServerState
			var err error
			if tc.create {
				created, createErr := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: a})
				err, got = createErr, created.Output
				require.Equal(t, "placeholder-server", created.ID)
			} else {
				updated, updateErr := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, State: prior, Inputs: a})
				err, got = updateErr, updated.Output
			}
			var partial infer.ResourceInitFailedError
			require.ErrorAs(t, err, &partial)
			require.Equal(t, tc.want, got)
			encoded := fmt.Sprintf("%+v", got)
			for _, marker := range []string{"fixture-private-key", "fixture-monitoring-token", "fixture-command"} {
				require.NotContains(t, encoded, marker)
				require.NotContains(t, err.Error(), marker)
			}
		})
	}
}

func TestServerMismatchedUpdateAcknowledgmentCannotOverlay(t *testing.T) {
	prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server", OrganizationID: ptr("placeholder-old-org"), Status: ptr("active")}
	inputs := serverArgs()
	inputs.Name = "after"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: serverMutationBody("after", nil, nil, false), Status: 200, Response: []byte(`{"serverId":"other-server","name":"unrelated","organizationId":"unrelated-org","sshKey":{"privateKey":"fixture-private-key"}}`)})
	got, err := (Server{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: inputs, State: prior})
	require.Error(t, err)
	require.Equal(t, prior, got.Output)
	require.NotContains(t, err.Error(), "fixture-private-key")
}

type serverRoundTripFunc func(*http.Request) (*http.Response, error)

const serverTestOperationUpdate = "update"

func (f serverRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestServerMutationTransportAndCancellation(t *testing.T) {
	for _, operation := range []string{"create", serverTestOperationUpdate, "delete"} {
		for _, mode := range []string{"transport", "canceled"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				endpoint := operation
				if endpoint == "delete" {
					endpoint = "remove"
				}
				transport := serverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodPost, req.Method)
					require.Equal(t, "/api/server."+endpoint, req.URL.Path)
					calls.Add(1)
					if mode == "canceled" {
						cancel()
						return nil, context.Canceled
					}
					return nil, errors.New("fixture-private-key fixture-monitoring-token 192.0.2.10 placeholder-server")
				})
				api, err := client.New("http://example.test", "placeholder-key", client.WithHTTPClient(&http.Client{Transport: transport}), client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
				require.NoError(t, err)
				r := Server{client: fixedClient(api)}
				prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server"}
				switch operation {
				case "create":
					got, e := r.Create(ctx, infer.CreateRequest[ServerArgs]{Inputs: serverArgs()})
					err = e
					require.Empty(t, got.ID)
				case serverTestOperationUpdate:
					got, e := r.Update(ctx, infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, State: prior, Inputs: serverArgs()})
					err = e
					require.Equal(t, prior, got.Output)
				case "delete":
					_, err = r.Delete(ctx, infer.DeleteRequest[ServerState]{ID: prior.ServerID})
				}
				require.Error(t, err)
				require.EqualValues(t, 1, calls.Load())
				if mode == "canceled" {
					require.ErrorIs(t, err, context.Canceled)
				}
				for _, marker := range []string{"fixture-private-key", "fixture-monitoring-token", "192.0.2.10", "placeholder-server"} {
					require.NotContains(t, err.Error(), marker)
				}
			})
		}
	}
}

func TestServerMutationHTTPFailuresPerOperation(t *testing.T) {
	for _, operation := range []string{"create", serverTestOperationUpdate, "delete"} {
		for _, status := range []int{400, 403, 500} {
			t.Run(fmt.Sprintf("%s/%d", operation, status), func(t *testing.T) {
				prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server", OrganizationID: ptr("placeholder-org")}
				endpoint, body := "/api/server."+operation, serverMutationBody("node", nil, nil, false)
				if operation == "create" {
					delete(body, "serverId")
				}
				if operation == "delete" {
					endpoint = "/api/server.remove"
					body = map[string]any{"serverId": "placeholder-server"}
				}
				s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: endpoint, Body: body, Status: status, Response: []byte(`{"message":"fixture-private-key fixture-monitoring-token 192.0.2.10 placeholder-server"}`)})
				r := Server{client: fixedClient(s.API())}
				var err error
				switch operation {
				case "create":
					got, e := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: serverArgs()})
					err = e
					require.Empty(t, got.ID)
				case serverTestOperationUpdate:
					got, e := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, State: prior, Inputs: serverArgs()})
					err = e
					require.Equal(t, prior, got.Output)
				case "delete":
					_, err = r.Delete(t.Context(), infer.DeleteRequest[ServerState]{ID: prior.ServerID})
				}
				require.Error(t, err)
				for _, marker := range []string{"fixture-private-key", "fixture-monitoring-token", "192.0.2.10", "placeholder-server"} {
					require.NotContains(t, err.Error(), marker)
				}
			})
		}
	}
}

// Ensure the HTTP server is the only possible upstream and reject operational endpoints.
func TestServerMutationNeverCallsOperationalEndpoints(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/api/server.create" && r.URL.Path != "/api/server.one" {
			t.Errorf("unexpected operational endpoint: %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/server.create" {
			_, _ = w.Write([]byte(`{"serverId":"placeholder-server"}`))
			return
		}
		_, _ = w.Write(serverRecord("node"))
	}))
	t.Cleanup(s.Close)
	api, err := client.New(s.URL, "placeholder-key", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	_, err = (Server{client: fixedClient(api)}).Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: serverArgs()})
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"/api/server.create", "/api/server.one"}, paths)
}
