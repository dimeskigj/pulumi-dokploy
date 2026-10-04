package dokploy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestServerFrameworkPreviewAndPartialState(t *testing.T) {
	srv, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("Server", "preview-server")
	inputs := property.NewMap(map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "sshKeyId": property.New(property.Computed)})
	created, err := srv.Create(p.CreateRequest{Urn: urn, DryRun: true, Properties: inputs})
	require.NoError(t, err)
	require.True(t, created.Properties.Get("serverId").IsComputed())
	require.True(t, created.Properties.Get("sshKeyId").IsComputed())
	state := property.NewMap(map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "serverId": property.New("placeholder-server"), "organizationId": property.New("placeholder-org"), "status": property.New("active")})
	updated, err := srv.Update(p.UpdateRequest{ID: "placeholder-server", Urn: urn, DryRun: true, State: state, OldInputs: inputs, Inputs: inputs})
	require.NoError(t, err)
	require.Equal(t, "placeholder-server", updated.Properties.Get("serverId").AsString())
	require.Equal(t, "placeholder-org", updated.Properties.Get("organizationId").AsString())
	require.Equal(t, "active", updated.Properties.Get("status").AsString())
	require.True(t, updated.Properties.Get("sshKeyId").IsComputed())
	for _, field := range []string{"serverId", "organizationId", "status"} {
		require.False(t, updated.Properties.Get(field).IsComputed(), field)
		require.False(t, updated.Properties.Get(field).Secret(), field)
	}
	secretInput := property.NewMap(map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10"), "sshKeyId": property.New("placeholder-key").WithSecret(true)})
	secretUpdate, err := srv.Update(p.UpdateRequest{ID: "placeholder-server", Urn: urn, DryRun: true, State: state, OldInputs: inputs, Inputs: secretInput})
	require.NoError(t, err)
	for _, field := range []string{"serverId", "organizationId", "status"} {
		require.False(t, secretUpdate.Properties.Get(field).Secret(), field)
		require.False(t, secretUpdate.Properties.Get(field).IsComputed(), field)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/server.create":
			_, _ = w.Write([]byte(`{"serverId":"placeholder-server","name":"persisted-create","description":"","enableDockerCleanup":false,"organizationId":"placeholder-new-org","serverStatus":"inactive","sshKey":{"privateKey":"fixture-private-key"}}`))
		case "/api/server.update":
			_, _ = w.Write([]byte(`{"serverId":"placeholder-server","name":"persisted-update","description":null,"ipAddress":"2001:db8::10","port":65535,"organizationId":"placeholder-updated-org","serverStatus":"unknown","sshKey":{"privateKey":"fixture-private-key"},"monitoring":{"token":"fixture-monitoring-token"}}`))
		case "/api/server.one":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	t.Cleanup(api.Close)
	require.NoError(t, srv.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(api.URL), "apiKey": property.New("placeholder-key")})}))
	plain := property.NewMap(map[string]property.Value{"name": property.New("node"), "ipAddress": property.New("192.0.2.10")})
	partialCreate, err := srv.Create(p.CreateRequest{Urn: urn, Properties: plain})
	require.Error(t, err)
	require.NotNil(t, partialCreate.PartialState)
	require.Equal(t, "placeholder-server", partialCreate.ID)
	require.Equal(t, "placeholder-server", partialCreate.Properties.Get("serverId").AsString())
	require.Equal(t, "persisted-create", partialCreate.Properties.Get("name").AsString())
	require.Equal(t, "", partialCreate.Properties.Get("description").AsString())
	require.Equal(t, "placeholder-new-org", partialCreate.Properties.Get("organizationId").AsString())
	require.Equal(t, "inactive", partialCreate.Properties.Get("status").AsString())
	require.False(t, partialCreate.Properties.Get("enableDockerCleanup").AsBool())
	_, hasSSHKey := partialCreate.Properties.GetOk("sshKey")
	require.False(t, hasSSHKey)
	changed := property.NewMap(map[string]property.Value{"name": property.New("after"), "ipAddress": property.New("192.0.2.10")})
	partialUpdate, err := srv.Update(p.UpdateRequest{ID: "placeholder-server", Urn: urn, State: state, OldInputs: plain, Inputs: changed})
	require.Error(t, err)
	require.NotNil(t, partialUpdate.PartialState)
	require.Equal(t, "persisted-update", partialUpdate.Properties.Get("name").AsString())
	require.Equal(t, "2001:db8::10", partialUpdate.Properties.Get("ipAddress").AsString())
	require.Equal(t, 65535.0, partialUpdate.Properties.Get("port").AsNumber())
	require.True(t, partialUpdate.Properties.Get("description").IsNull())
	require.Equal(t, "placeholder-updated-org", partialUpdate.Properties.Get("organizationId").AsString())
	require.Equal(t, "unknown", partialUpdate.Properties.Get("status").AsString())
	require.Equal(t, "placeholder-server", partialUpdate.Properties.Get("serverId").AsString())
	_, hasSSHKey = partialUpdate.Properties.GetOk("sshKey")
	require.False(t, hasSSHKey)
	_, hasMonitoring := partialUpdate.Properties.GetOk("monitoring")
	require.False(t, hasMonitoring)
	for _, marker := range []string{"fixture-private-key", "fixture-monitoring-token"} {
		require.NotContains(t, err.Error(), marker)
	}
}
