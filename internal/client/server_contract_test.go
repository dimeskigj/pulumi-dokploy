package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/require"
)

func TestServerClientContract(t *testing.T) {
	var readCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/server.create":
			require.Equal(t, http.MethodPost, r.Method)
			var payload map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.JSONEq(t, `{"name":"node","description":null,"ipAddress":"192.0.2.10","port":22,"username":"root","sshKeyId":null,"serverType":"deploy","enableDockerCleanup":false}`, string(mustMarshal(t, payload)))
			require.NotContains(t, payload, "command")
			_, _ = w.Write([]byte(`{"serverId":"server-1","unrelated":{"sshKey":{"privateKey":42},"monitoring":false}}`))
		case "/api/server.one":
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "server-1", r.URL.Query().Get("serverId"))
			readCount++
			description := `""`
			if readCount == 2 {
				description = "null"
			}
			_, _ = w.Write([]byte(`{"serverId":"server-1","name":"node","description":` + description + `,"ipAddress":"192.0.2.10","port":22,"username":"root","sshKeyId":null,"serverType":"deploy","enableDockerCleanup":false,"organizationId":"org-1","serverStatus":"active","sshKey":{"privateKey":"must-not-be-typed"},"monitoring":{"token":"must-not-be-typed"}}`))
		case "/api/server.update":
			require.Equal(t, http.MethodPost, r.Method)
			var payload map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.JSONEq(t, `{"serverId":"server-1","name":"node","description":null,"ipAddress":"192.0.2.10","port":22,"username":"root","sshKeyId":null,"serverType":"deploy","enableDockerCleanup":false}`, string(mustMarshal(t, payload)))
			require.NotContains(t, payload, "command")
			_, _ = w.Write([]byte(`{"serverId":"server-1","otherField":{"malformed":true}}`))
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "test-key")
	require.NoError(t, err)

	created, err := c.ServerCreateWithResponse(t.Context(), generated.ServerCreateRequest{
		Name: "node", Description: nullable.NewNullNullable[string](), IpAddress: "192.0.2.10", Port: 22,
		Username: "root", SshKeyId: nullable.NewNullNullable[string](), ServerType: generated.ServerCreateRequestServerTypeDeploy,
		EnableDockerCleanup: boolPointer(false),
	})
	require.NoError(t, err)
	require.NotNil(t, created.GetJSON200())
	require.Equal(t, "server-1", created.GetJSON200().ServerId)

	updated, err := c.ServerUpdateWithResponse(t.Context(), generated.ServerUpdateRequest{
		ServerId: "server-1", Name: "node", Description: nullable.NewNullNullable[string](), IpAddress: "192.0.2.10", Port: 22,
		Username: "root", SshKeyId: nullable.NewNullNullable[string](), ServerType: generated.ServerUpdateRequestServerTypeDeploy,
		EnableDockerCleanup: boolPointer(false),
	})
	require.NoError(t, err)
	require.Equal(t, "server-1", updated.GetJSON200().ServerId)

	first, err := c.ServerOneWithResponse(t.Context(), &generated.ServerOneParams{ServerId: "server-1"})
	require.NoError(t, err)
	read := first.GetJSON200()
	require.NotNil(t, read)
	require.Equal(t, "server-1", read.ServerId)
	require.Equal(t, false, *read.EnableDockerCleanup)
	require.Equal(t, "", read.Description.MustGet())
	second, err := c.ServerOneWithResponse(t.Context(), &generated.ServerOneParams{ServerId: "server-1"})
	require.NoError(t, err)
	require.True(t, second.GetJSON200().Description.IsNull())
	_, hasSSHKey := reflect.TypeOf(generated.Server{}).FieldByName("SshKey")
	_, hasMonitoring := reflect.TypeOf(generated.Server{}).FieldByName("Monitoring")
	require.False(t, hasSSHKey)
	require.False(t, hasMonitoring)
}

func boolPointer(value bool) *bool { return &value }

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return b
}
