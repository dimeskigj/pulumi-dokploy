package dokploy

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestLookupControlPlaneInvoke(t *testing.T) {
	for _, tc := range []struct {
		suffix, field, operation, payload string
		keys                              []string
	}{
		{"getProject", "projectId", "project.one", `{"projectId":"id","name":"","description":"","defaultEnvironmentId":null,"password":"mock-secret"}`, []string{"projectId", "name", "description"}},
		{"getEnvironment", "environmentId", "environment.one", `{"environmentId":"id","name":"","isDefault":false,"description":null,"password":"mock-secret"}`, []string{"environmentId", "name", "isDefault"}},
		{"getServer", "serverId", "server.one", `{"serverId":"id","name":"","port":0,"username":"","password":"mock-secret"}`, []string{"serverId", "name", "port", "username"}},
		{"getRegistry", "registryId", "registry.one", `{"registryId":"id","registryName":"","registryUrl":"https://user:secret@registry.example","imagePrefix":"","password":"mock-secret"}`, []string{"registryId", "name", "imagePrefix"}},
		{"getSSHKey", "sshKeyId", "sshKey.one", `{"sshKeyId":"id","name":"","publicKey":"","privateKey":"mock-secret"}`, []string{"sshKeyId", "name", "publicKey"}},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.field: {"id"}}, Status: 200, Response: []byte(tc.payload)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			invoke := func(args map[string]property.Value) (p.InvokeResponse, error) {
				return provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.suffix), Args: property.NewMap(args)})
			}
			out, err := invoke(map[string]property.Value{tc.field: property.New("id")})
			require.NoError(t, err)
			require.Empty(t, out.Failures)
			keys := make([]string, 0, out.Return.Len())
			for key := range out.Return.All {
				keys = append(keys, key)
			}
			require.ElementsMatch(t, tc.keys, keys)
			for _, key := range tc.keys {
				if key != tc.field {
					require.False(t, out.Return.Get(key).IsNull(), key)
				}
			}
			for _, bad := range []map[string]property.Value{{}, {tc.field: property.New(7.0)}, {tc.field: property.New("")}, {tc.field: property.New("  \t")}} {
				out, err := invoke(bad)
				require.True(t, err != nil || len(out.Failures) > 0)
				if err != nil {
					require.NotContains(t, err.Error(), "mock-api-key")
					require.NotContains(t, err.Error(), s.server.URL)
				}
			}
			_, err = provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:unknownLookup"), Args: property.NewMap(map[string]property.Value{tc.field: property.New("id")})})
			require.Error(t, err)
		})
	}
}
