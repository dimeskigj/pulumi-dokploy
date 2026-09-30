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

func TestLookupWorkloadsCompose(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       GetComposeResult
		category   string
	}{
		{"complete", `{"composeId":"id","name":"display","description":"description","appName":"app","environmentId":"env-id","serverId":"server-id","composeStatus":"future-status","composeType":"future-type","composeFile":"mock-secret","command":"mock-secret","env":"mock-secret","nested":{"password":"mock-secret"}}`, 200,
			GetComposeResult{ComposeID: "id", Name: "display", Description: stringPtr("description"), AppName: stringPtr("app"), EnvironmentID: stringPtr("env-id"), ServerID: stringPtr("server-id"), Status: stringPtr("future-status"), ComposeType: stringPtr("future-type")}, ""},
		{"empty optionals", `{"composeId":"id","name":"","description":"","appName":"","environmentId":"","serverId":"","composeStatus":"","composeType":""}`, 200,
			GetComposeResult{ComposeID: "id", Name: "", Description: stringPtr(""), AppName: stringPtr(""), EnvironmentID: stringPtr(""), ServerID: stringPtr(""), Status: stringPtr(""), ComposeType: stringPtr("")}, ""},
		{"null optionals", `{"composeId":"id","name":"n","description":null,"appName":null,"environmentId":null,"serverId":null,"composeStatus":null,"composeType":null}`, 200, GetComposeResult{ComposeID: "id", Name: "n"}, ""},
		{"minimal", `{"composeId":"id","name":"n"}`, 200, GetComposeResult{ComposeID: "id", Name: "n"}, ""},
		{"null object", `null`, 200, GetComposeResult{}, "response contract"},
		{"missing id", `{"name":"n"}`, 200, GetComposeResult{}, "response contract"},
		{"empty id", `{"composeId":"","name":"n"}`, 200, GetComposeResult{}, "response contract"},
		{"mismatched id", `{"composeId":"mock-other-id","name":"n"}`, 200, GetComposeResult{}, "response contract"},
		{"missing name", `{"composeId":"id"}`, 200, GetComposeResult{}, "response contract"},
		{"null name", `{"composeId":"id","name":null}`, 200, GetComposeResult{}, "response contract"},
		{"invalid id type", `{"composeId":17,"name":"n"}`, 200, GetComposeResult{}, "response contract"},
		{"invalid name type", `{"composeId":"id","name":false}`, 200, GetComposeResult{}, "response contract"},
		{"invalid optional type", `{"composeId":"id","name":"n","description":{"password":"mock-secret"}}`, 200, GetComposeResult{}, "response contract"},
		{"invalid status type", `{"composeId":"id","name":"n","composeStatus":4}`, 200, GetComposeResult{}, "response contract"},
		{"invalid compose type", `{"composeId":"id","name":"n","composeType":[]}`, 200, GetComposeResult{}, "response contract"},
		{"invalid json", `{`, 200, GetComposeResult{}, "response contract"},
		{"unauthorized", `{"message":"mock-secret"}`, 401, GetComposeResult{}, "authentication"},
		{"forbidden", `{"message":"mock-secret"}`, 403, GetComposeResult{}, "authorization"},
		{"not found", `{"message":"mock-secret"}`, 404, GetComposeResult{}, "not found"},
		{"API not found", `{"code":"NOT_FOUND","message":"mock-secret"}`, 400, GetComposeResult{}, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/compose.one", Query: url.Values{"composeId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			out, err := (GetCompose{client: fixedClient(s.API())}).Invoke(t.Context(), infer.FunctionRequest[GetComposeArgs]{Input: GetComposeArgs{ComposeID: "id"}})
			if tc.category != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "compose.one")
				require.Contains(t, err.Error(), tc.category)
				for _, sensitive := range []string{"mock-secret", "mock-other-id", "private.example", s.server.URL} {
					require.NotContains(t, err.Error(), sensitive)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, out.Output)
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		lookup := GetCompose{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := lookup.Invoke(t.Context(), infer.FunctionRequest[GetComposeArgs]{Input: GetComposeArgs{ComposeID: id}})
		require.ErrorContains(t, err, "composeId")
		require.False(t, called)
	}
}
