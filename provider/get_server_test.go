package dokploy

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestLookupControlPlaneServer(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		status      int
		fail        bool
	}{
		{"complete", `{"serverId":"id","name":"display","description":"","ipAddress":"127.0.0.1","port":0,"username":"","organizationId":"org","sshKeyId":"","serverType":"","serverStatus":"","futureSecret":"mock-secret"}`, 200, false},
		{"minimal", `{"serverId":"id","name":""}`, 200, false},
		{"null object", `null`, 200, true},
		{"missing id", `{"name":"x"}`, 200, true},
		{"null name", `{"serverId":"id","name":null}`, 200, true},
		{"wrong optional type", `{"serverId":"id","name":"x","port":{"secret":"mock-secret"}}`, 200, true},
		{"missing name", `{"serverId":"id"}`, 200, true},
		{"mismatched id", `{"serverId":"other","name":"x"}`, 200, true},
		{"wrong type", `{"serverId":"id","name":123}`, 200, true},
		{"invalid json", `{`, 200, true},
		{"not found", `{"message":"mock-secret"}`, 404, true},
		{"forbidden", `{"message":"mock-secret"}`, 403, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/server.one", Query: url.Values{"serverId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			r := GetServer{client: fixedClient(s.API())}
			out, err := r.Invoke(t.Context(), infer.FunctionRequest[GetServerArgs]{Input: GetServerArgs{ServerID: "id"}})
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "id", out.Output.ServerID)
			if tc.title == "minimal" {
				require.Equal(t, "", out.Output.Name)
				require.Nil(t, out.Output.Port)
				require.Nil(t, out.Output.Status)
			} else {
				require.Equal(t, "display", out.Output.Name)
				require.Equal(t, 0, *out.Output.Port)
				require.Equal(t, "", *out.Output.Status)
				require.Equal(t, "127.0.0.1", *out.Output.IPAddress)
				require.Equal(t, "org", *out.Output.OrganizationID)
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		r := GetServer{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := r.Invoke(t.Context(), infer.FunctionRequest[GetServerArgs]{Input: GetServerArgs{ServerID: id}})
		require.Error(t, err)
		require.False(t, called)
	}
}
