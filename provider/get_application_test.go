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

func TestLookupWorkloadsApplication(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       GetApplicationResult
		category   string
	}{
		{"complete", `{"applicationId":"id","name":"display","description":"description","appName":"app","environmentId":"env-id","serverId":"server-id","applicationStatus":"future-status","registryId":"registry-id","buildRegistryId":"build-id","env":"mock-secret","buildArgs":"mock-secret","buildSecrets":"mock-secret","source":{"token":"mock-secret"}}`, 200,
			GetApplicationResult{ApplicationID: "id", Name: "display", Description: stringPtr("description"), AppName: stringPtr("app"), EnvironmentID: stringPtr("env-id"), ServerID: stringPtr("server-id"), Status: stringPtr("future-status"), RegistryID: stringPtr("registry-id"), BuildRegistryID: stringPtr("build-id")}, ""},
		{"empty optionals", `{"applicationId":"id","name":"","description":"","appName":"","environmentId":"","serverId":"","applicationStatus":"","registryId":"","buildRegistryId":""}`, 200,
			GetApplicationResult{ApplicationID: "id", Name: "", Description: stringPtr(""), AppName: stringPtr(""), EnvironmentID: stringPtr(""), ServerID: stringPtr(""), Status: stringPtr(""), RegistryID: stringPtr(""), BuildRegistryID: stringPtr("")}, ""},
		{"null optionals", `{"applicationId":"id","name":"n","description":null,"appName":null,"environmentId":null,"serverId":null,"applicationStatus":null,"registryId":null,"buildRegistryId":null}`, 200, GetApplicationResult{ApplicationID: "id", Name: "n"}, ""},
		{"minimal", `{"applicationId":"id","name":"n"}`, 200, GetApplicationResult{ApplicationID: "id", Name: "n"}, ""},
		{"null object", `null`, 200, GetApplicationResult{}, "response contract"},
		{"missing id", `{"name":"n"}`, 200, GetApplicationResult{}, "response contract"},
		{"empty id", `{"applicationId":"","name":"n"}`, 200, GetApplicationResult{}, "response contract"},
		{"mismatched id", `{"applicationId":"mock-other-id","name":"n"}`, 200, GetApplicationResult{}, "response contract"},
		{"missing name", `{"applicationId":"id"}`, 200, GetApplicationResult{}, "response contract"},
		{"null name", `{"applicationId":"id","name":null}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid id type", `{"applicationId":17,"name":"n"}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid name type", `{"applicationId":"id","name":false}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid optional type", `{"applicationId":"id","name":"n","description":{"password":"mock-secret"}}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid status type", `{"applicationId":"id","name":"n","applicationStatus":false}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid registry type", `{"applicationId":"id","name":"n","registryId":42}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid build registry type", `{"applicationId":"id","name":"n","buildRegistryId":[]}`, 200, GetApplicationResult{}, "response contract"},
		{"invalid json", `{`, 200, GetApplicationResult{}, "response contract"},
		{"unauthorized", `{"message":"mock-secret"}`, 401, GetApplicationResult{}, "authentication"},
		{"forbidden", `{"message":"mock-secret"}`, 403, GetApplicationResult{}, "authorization"},
		{"not found", `{"message":"mock-secret"}`, 404, GetApplicationResult{}, "not found"},
		{"API not found", `{"code":"NOT_FOUND","message":"mock-secret"}`, 400, GetApplicationResult{}, "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/application.one", Query: url.Values{"applicationId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			out, err := (GetApplication{client: fixedClient(s.API())}).Invoke(t.Context(), infer.FunctionRequest[GetApplicationArgs]{Input: GetApplicationArgs{ApplicationID: "id"}})
			if tc.category != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "application.one")
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
		lookup := GetApplication{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := lookup.Invoke(t.Context(), infer.FunctionRequest[GetApplicationArgs]{Input: GetApplicationArgs{ApplicationID: id}})
		require.ErrorContains(t, err, "applicationId")
		require.False(t, called)
	}
}
