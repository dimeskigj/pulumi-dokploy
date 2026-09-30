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

func TestLookupControlPlaneEnvironment(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		status      int
		fail        bool
	}{
		{"complete", `{"environmentId":"id","name":"display","description":"","projectId":"p","isDefault":false,"futureSecret":"mock-secret"}`, 200, false},
		{"default", `{"environmentId":"id","name":"display","isDefault":true}`, 200, false},
		{"minimal", `{"environmentId":"id","name":""}`, 200, false},
		{"null object", `null`, 200, true},
		{"missing id", `{"name":"x"}`, 200, true},
		{"null name", `{"environmentId":"id","name":null}`, 200, true},
		{"wrong optional type", `{"environmentId":"id","name":"x","isDefault":{"secret":"mock-secret"}}`, 200, true},
		{"missing name", `{"environmentId":"id"}`, 200, true},
		{"mismatched id", `{"environmentId":"other","name":"x"}`, 200, true},
		{"wrong type", `{"environmentId":"id","name":123}`, 200, true},
		{"invalid json", `{`, 200, true},
		{"not found", `{"message":"mock-secret"}`, 404, true},
		{"forbidden", `{"message":"mock-secret"}`, 403, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/environment.one", Query: url.Values{"environmentId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			r := GetEnvironment{client: fixedClient(s.API())}
			out, err := r.Invoke(t.Context(), infer.FunctionRequest[GetEnvironmentArgs]{Input: GetEnvironmentArgs{EnvironmentID: "id"}})
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "id", out.Output.EnvironmentID)
			if tc.title == "minimal" {
				require.Equal(t, "", out.Output.Name)
				require.Nil(t, out.Output.IsDefault)
			} else {
				require.Equal(t, "display", out.Output.Name)
			}
			if tc.title == "default" {
				require.Equal(t, true, *out.Output.IsDefault)
			}
			if tc.title == "complete" {
				require.Equal(t, false, *out.Output.IsDefault)
				require.Equal(t, "p", *out.Output.ProjectID)
				require.Equal(t, "", *out.Output.Description)
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		r := GetEnvironment{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := r.Invoke(t.Context(), infer.FunctionRequest[GetEnvironmentArgs]{Input: GetEnvironmentArgs{EnvironmentID: id}})
		require.Error(t, err)
		require.False(t, called)
	}
}
