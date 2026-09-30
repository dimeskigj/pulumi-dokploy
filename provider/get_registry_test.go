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

func TestLookupControlPlaneRegistry(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		status      int
		fail        bool
	}{
		{"complete", `{"registryId":"id","registryName":"display","registryUrl":"registry.example:5000","username":"","imagePrefix":"","serverId":"","registryType":"cloud","password":"mock-secret","futureSecret":"mock-secret"}`, 200, false},
		{"minimal", `{"registryId":"id","registryName":""}`, 200, false},
		{"unsafe URL", `{"registryId":"id","registryName":"display","registryUrl":"https://user:secret@registry.example"}`, 200, false},
		{"null object", `null`, 200, true},
		{"missing id", `{"registryName":"x"}`, 200, true},
		{"null name", `{"registryId":"id","registryName":null}`, 200, true},
		{"wrong optional type", `{"registryId":"id","registryName":"x","username":{"secret":"mock-secret"}}`, 200, true},
		{"missing name", `{"registryId":"id"}`, 200, true},
		{"mismatched id", `{"registryId":"other","name":"x","registryName":"x"}`, 200, true},
		{"wrong type", `{"registryId":"id","name":123,"registryName":123}`, 200, true},
		{"invalid json", `{`, 200, true},
		{"not found", `{"message":"mock-secret"}`, 404, true},
		{"forbidden", `{"message":"mock-secret"}`, 403, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/registry.one", Query: url.Values{"registryId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			r := GetRegistry{client: fixedClient(s.API())}
			out, err := r.Invoke(t.Context(), infer.FunctionRequest[GetRegistryArgs]{Input: GetRegistryArgs{RegistryID: "id"}})
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "id", out.Output.RegistryID)
			if tc.title == "minimal" {
				require.Equal(t, "", out.Output.Name)
				require.Nil(t, out.Output.URL)
				require.Nil(t, out.Output.ImagePrefix)
			} else {
				require.Equal(t, "display", out.Output.Name)
			}
			if tc.title == "unsafe URL" {
				require.Nil(t, out.Output.URL)
			}
			if tc.title == "complete" {
				require.Equal(t, "registry.example:5000", *out.Output.URL)
				require.Equal(t, "", *out.Output.ImagePrefix)
				require.Equal(t, "", *out.Output.ServerID)
				require.Equal(t, "cloud", *out.Output.RegistryType)
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		r := GetRegistry{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := r.Invoke(t.Context(), infer.FunctionRequest[GetRegistryArgs]{Input: GetRegistryArgs{RegistryID: id}})
		require.Error(t, err)
		require.False(t, called)
	}
}
