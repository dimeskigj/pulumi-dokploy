package dokploy

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func serverArgs() ServerArgs {
	return ServerArgs{Name: "node", IPAddress: "192.0.2.10", Port: 22, Username: "root", ServerType: "deploy"}
}
func TestServerCheckDefaultsAndValidation(t *testing.T) {
	r := Server{}
	for _, tc := range []struct {
		name    string
		values  map[string]property.Value
		invalid string
	}{
		{"defaults", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("host.example")}, ""},
		{"empty name", map[string]property.Value{"name": property.New("  "), "ipAddress": property.New("::1")}, "name"},
		{"empty address", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("  ")}, "ipAddress"},
		{"bad port", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "port": property.New(65536.0)}, "port"},
		{"zero port", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "port": property.New(0.0)}, "port"},
		{"minimum port", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "port": property.New(1.0)}, ""},
		{"maximum port", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "port": property.New(65535.0)}, ""},
		{"ipv4 explicit false", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "enableDockerCleanup": property.New(false)}, ""},
		{"ipv6", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("2001:db8::10")}, ""},
		{"explicit true", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("host.example"), "enableDockerCleanup": property.New(true)}, ""},
		{"empty username", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "username": property.New("  ")}, "username"},
		{"empty key", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "sshKeyId": property.New(" ")}, "sshKeyId"},
		{"empty type", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "serverType": property.New("")}, "serverType"},
		{"invalid type", map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("::1"), "serverType": property.New("other")}, "serverType"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(tc.values)})
			require.NoError(t, err)
			if tc.invalid != "" {
				require.Contains(t, func() []string {
					var fields []string
					for _, f := range got.Failures {
						fields = append(fields, f.Property)
					}
					return fields
				}(), tc.invalid)
				return
			}
			require.Empty(t, got.Failures)
			port := 22
			if v, ok := tc.values["port"]; ok {
				port = int(v.AsNumber())
			}
			require.Equal(t, port, got.Inputs.Port)
			require.Equal(t, "root", got.Inputs.Username)
			require.Equal(t, "deploy", got.Inputs.ServerType)
			wantCleanup := false
			if v, ok := tc.values["enableDockerCleanup"]; ok {
				wantCleanup = v.AsBool()
			}
			require.Equal(t, wantCleanup, got.Inputs.EnableDockerCleanup)
		})
	}
}

func TestServerCheckDefersComputedInputs(t *testing.T) {
	values := map[string]property.Value{}
	for _, key := range []string{"name", "description", "ipAddress", "port", "username", "sshKeyId", "serverType", "enableDockerCleanup"} {
		values[key] = property.New(property.Computed)
	}
	got, err := (Server{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(values)})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}
func TestServerDiff(t *testing.T) {
	a := serverArgs()
	unchanged, err := (Server{}).Diff(t.Context(), infer.DiffRequest[ServerArgs, ServerState]{Inputs: a, State: ServerState{ServerArgs: a}})
	require.NoError(t, err)
	require.False(t, unchanged.HasChanges)
	b := a
	b.IPAddress = "example.test"
	diff, err := (Server{}).Diff(t.Context(), infer.DiffRequest[ServerArgs, ServerState]{Inputs: b, State: ServerState{ServerArgs: a}})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["ipAddress"].Kind)
	for _, tc := range []struct {
		name   string
		change func(*ServerArgs)
	}{
		{"name", func(a *ServerArgs) { a.Name = "after" }},
		{"description", func(a *ServerArgs) { a.Description = ptr("") }},
		{"ipAddress", func(a *ServerArgs) { a.IPAddress = "2001:db8::10" }},
		{"port", func(a *ServerArgs) { a.Port = 65535 }},
		{"username", func(a *ServerArgs) { a.Username = "admin" }},
		{"sshKeyId", func(a *ServerArgs) { a.SSHKeyID = ptr("placeholder-key") }},
		{"serverType", func(a *ServerArgs) { a.ServerType = "build" }},
		{"enableDockerCleanup", func(a *ServerArgs) { a.EnableDockerCleanup = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := a
			tc.change(&changed)
			got, err := (Server{}).Diff(t.Context(), infer.DiffRequest[ServerArgs, ServerState]{Inputs: changed, State: ServerState{ServerArgs: a}})
			require.NoError(t, err)
			require.Equal(t, map[string]p.PropertyDiff{tc.name: {Kind: p.Update}}, got.DetailedDiff)
		})
	}
	for _, tc := range []struct {
		name string
		old  *string
		new  *string
		key  string
	}{
		{"description absent to empty", nil, ptr(""), "description"},
		{"description empty to absent", ptr(""), nil, "description"},
		{"ssh key absent to present", nil, ptr("placeholder-key"), "sshKeyId"},
		{"ssh key present to absent", ptr("placeholder-key"), nil, "sshKeyId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, next := serverArgs(), serverArgs()
			if tc.key == "description" {
				old.Description, next.Description = tc.old, tc.new
			} else {
				old.SSHKeyID, next.SSHKeyID = tc.old, tc.new
			}
			got, err := (Server{}).Diff(t.Context(), infer.DiffRequest[ServerArgs, ServerState]{Inputs: next, State: ServerState{ServerArgs: old}})
			require.NoError(t, err)
			require.Equal(t, map[string]p.PropertyDiff{tc.key: {Kind: p.Update}}, got.DetailedDiff)
		})
	}
}
func TestServerDryRun(t *testing.T) {
	r := Server{client: func(context.Context) *client.Client { t.Fatal("preview constructed client"); return nil }}
	a := serverArgs()
	created, err := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: a, DryRun: true})
	require.NoError(t, err)
	require.Empty(t, created.ID)
	updated, err := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: "placeholder-server", Inputs: a, State: ServerState{OrganizationID: ptr("placeholder-org"), Status: ptr("active")}, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "placeholder-server", updated.Output.ServerID)
	require.Equal(t, ptr("placeholder-org"), updated.Output.OrganizationID)
}
func TestServerLifecycleAndImport(t *testing.T) {
	a := serverArgs()
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/server.create", Body: map[string]any{"name": "node", "ipAddress": "192.0.2.10", "port": 22, "username": "root", "serverType": "deploy", "description": nil, "sshKeyId": nil, "enableDockerCleanup": false}, Status: 200, Response: []byte(`{"serverId":"placeholder-server"}`)},
		scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: serverRecord("node")},
		scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: serverRecord("node")},
	)
	r := Server{client: fixedClient(s.API())}
	created, err := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: a})
	require.NoError(t, err)
	require.Equal(t, "placeholder-server", created.ID)
	require.Equal(t, "placeholder-server", created.Output.ServerID)
	read, err := r.Read(t.Context(), infer.ReadRequest[ServerArgs, ServerState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, a, read.Inputs)
}
func serverRecord(name string) []byte {
	return []byte(`{"serverId":"placeholder-server","name":"` + name + `","ipAddress":"192.0.2.10","port":22,"username":"root","serverType":"deploy","description":null,"sshKeyId":null,"enableDockerCleanup":false,"organizationId":"placeholder-org","serverStatus":"active","sshKey":{"privateKey":"fixture-private-key"}}`)
}

func TestServerReadContract(t *testing.T) {
	for _, tc := range []struct {
		name, record    string
		absent, invalid bool
	}{
		{"import", string(serverRecord("node")), false, false},
		{"unknown type and status", strings.ReplaceAll(strings.ReplaceAll(string(serverRecord("node")), `"serverType":"deploy"`, `"serverType":"future"`), `"serverStatus":"active"`, `"serverStatus":"future"`), false, false},
		{"missing name", `{"serverId":"placeholder-server"}`, false, true},
		{"null port", strings.Replace(string(serverRecord("node")), `"port":22`, `"port":null`, 1), false, true},
		{"mismatch", strings.Replace(string(serverRecord("node")), `"serverId":"placeholder-server"`, `"serverId":"other"`, 1), false, true},
		{"absent", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := 200
			if tc.absent {
				status = 404
			}
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: status, Response: []byte(tc.record)})
			read, err := (Server{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ServerArgs, ServerState]{ID: "placeholder-server"})
			if tc.invalid {
				require.Error(t, err)
				require.Empty(t, read.ID)
			} else {
				require.NoError(t, err)
				if tc.absent {
					require.Empty(t, read.ID)
				} else {
					require.Equal(t, "placeholder-server", read.ID)
					require.Equal(t, read.Inputs, read.State.ServerArgs)
				}
			}
		})
	}
}
func TestServerPartialFailures(t *testing.T) {
	a := serverArgs()
	for _, tc := range []struct {
		name       string
		create     bool
		readStatus int
		readBody   []byte
	}{
		{"create read missing", true, 404, []byte(`{}`)},
		{"create read mismatch", true, 200, []byte(`{"serverId":"other"}`)},
		{"update read missing", false, 404, []byte(`{}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verb := "/api/server.update"
			if tc.create {
				verb = "/api/server.create"
			}
			body := map[string]any{"name": "after", "ipAddress": "192.0.2.10", "port": 22, "username": "root", "serverType": "deploy", "description": nil, "sshKeyId": nil, "enableDockerCleanup": false, "serverId": "placeholder-server"}
			if tc.create {
				body["name"] = "node"
				delete(body, "serverId")
			}
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: verb, Body: body, Status: 200, Response: []byte(`{"serverId":"placeholder-server"}`)}, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: tc.readStatus, Response: tc.readBody})
			r := Server{client: fixedClient(s.API())}
			if tc.create {
				got, err := r.Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: a})
				var partial infer.ResourceInitFailedError
				require.ErrorAs(t, err, &partial)
				require.Equal(t, "placeholder-server", got.ID)
				require.Equal(t, a, got.Output.ServerArgs)
			} else {
				a.Name = "after"
				got, err := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: "placeholder-server", Inputs: a, State: ServerState{ServerID: "placeholder-server", OrganizationID: ptr("placeholder-org")}})
				var partial infer.ResourceInitFailedError
				require.ErrorAs(t, err, &partial)
				require.Equal(t, "after", got.Output.Name)
				require.Equal(t, ptr("placeholder-org"), got.Output.OrganizationID)
			}
		})
	}
}
func TestServerMutationErrors(t *testing.T) {
	for _, status := range []int{400, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: map[string]any{"name": "after", "ipAddress": "192.0.2.10", "port": 22, "username": "root", "serverType": "deploy", "description": nil, "sshKeyId": nil, "enableDockerCleanup": false, "serverId": "placeholder-server"}, Status: status, Response: []byte(`{"message":"fixture-private-key monitoring-token 192.0.2.10 placeholder-server"}`)})
			prior := ServerState{ServerArgs: serverArgs(), ServerID: "placeholder-server"}
			a := serverArgs()
			a.Name = "after"
			got, err := (Server{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: a, State: prior})
			require.Error(t, err)
			require.Equal(t, prior, got.Output)
			for _, secret := range []string{"fixture-private-key", "monitoring-token", "192.0.2.10", "placeholder-server"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}
func TestServerDiagnostics(t *testing.T) {
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.remove", Body: map[string]any{"serverId": "placeholder-server"}, Status: 404, Response: []byte(`{"message":"fixture-private-key"}`)}, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 404, Response: []byte(`{}`)})
	_, err := (Server{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[ServerState]{ID: "placeholder-server"})
	require.NoError(t, err)
}

func TestServerUpdateAndDelete(t *testing.T) {
	a := serverArgs()
	a.Name = "after"
	a.Description = ptr("")
	a.SSHKeyID = ptr("placeholder-key")
	a.EnableDockerCleanup = true
	updateBody := map[string]any{"serverId": "placeholder-server", "name": "after", "description": "", "ipAddress": "192.0.2.10", "port": 22, "username": "root", "sshKeyId": "placeholder-key", "serverType": "deploy", "enableDockerCleanup": true}
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: updateBody, Status: 200, Response: []byte(`{"serverId":"placeholder-server","unrelated":{"privateKey":42}}`)},
		scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"placeholder-server"}}, Status: 200, Response: []byte(`{"serverId":"placeholder-server","name":"persisted","description":"","ipAddress":"192.0.2.10","port":22,"username":"root","sshKeyId":"placeholder-key","serverType":"deploy","enableDockerCleanup":true,"organizationId":"placeholder-org","serverStatus":"inactive","monitoring":{"token":"fixture-token"}}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/server.remove", Body: map[string]any{"serverId": "placeholder-server"}, Status: 200, Response: []byte(`{"serverId":"placeholder-server"}`)})
	r := Server{client: fixedClient(s.API())}
	got, err := r.Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: "placeholder-server", Inputs: a, State: ServerState{ServerID: "placeholder-server"}})
	require.NoError(t, err)
	require.Equal(t, "persisted", got.Output.Name)
	require.Equal(t, ptr(""), got.Output.Description)
	require.Equal(t, ptr("inactive"), got.Output.Status)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ServerState]{ID: "placeholder-server", State: got.Output})
	require.NoError(t, err)
}

func TestServerRejectsUnconfirmedMutations(t *testing.T) {
	a := serverArgs()
	createBody := map[string]any{"name": "node", "description": nil, "ipAddress": "192.0.2.10", "port": 22, "username": "root", "sshKeyId": nil, "serverType": "deploy", "enableDockerCleanup": false}
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.create", Body: createBody, Status: 200, Response: []byte(`{"unrelated":{"privateKey":"fixture-private-key"}}`)})
	got, err := (Server{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ServerArgs]{Inputs: a})
	require.Error(t, err)
	require.Empty(t, got.ID)
	require.NotContains(t, err.Error(), "fixture-private-key")
	updateBody := map[string]any{"serverId": "placeholder-server", "name": "node", "description": nil, "ipAddress": "192.0.2.10", "port": 22, "username": "root", "sshKeyId": nil, "serverType": "deploy", "enableDockerCleanup": false}
	s2 := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/server.update", Body: updateBody, Status: 200, Response: []byte(`{"serverId":"other"}`)})
	prior := ServerState{ServerArgs: a, ServerID: "placeholder-server"}
	up, err := (Server{client: fixedClient(s2.API())}).Update(t.Context(), infer.UpdateRequest[ServerArgs, ServerState]{ID: prior.ServerID, Inputs: a, State: prior})
	require.Error(t, err)
	require.Equal(t, prior, up.Output)
}
