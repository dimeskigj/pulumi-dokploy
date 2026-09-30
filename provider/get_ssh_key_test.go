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

func TestLookupControlPlaneSSHKey(t *testing.T) {
	for _, tc := range []struct {
		title, body string
		status      int
		fail        bool
	}{
		{"complete", `{"sshKeyId":"id","name":"display","description":"","publicKey":"","organizationId":"","privateKey":"mock-secret","futureSecret":"mock-secret"}`, 200, false},
		{"minimal", `{"sshKeyId":"id","name":""}`, 200, false},
		{"null object", `null`, 200, true},
		{"missing id", `{"name":"x"}`, 200, true},
		{"null name", `{"sshKeyId":"id","name":null}`, 200, true},
		{"wrong optional type", `{"sshKeyId":"id","name":"x","publicKey":{"secret":"mock-secret"}}`, 200, true},
		{"missing name", `{"sshKeyId":"id"}`, 200, true},
		{"mismatched id", `{"sshKeyId":"other","name":"x"}`, 200, true},
		{"wrong type", `{"sshKeyId":"id","name":123}`, 200, true},
		{"invalid json", `{`, 200, true},
		{"not found", `{"message":"mock-secret"}`, 404, true},
		{"forbidden", `{"message":"mock-secret"}`, 403, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/sshKey.one", Query: url.Values{"sshKeyId": {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			r := GetSSHKey{client: fixedClient(s.API())}
			out, err := r.Invoke(t.Context(), infer.FunctionRequest[GetSSHKeyArgs]{Input: GetSSHKeyArgs{SSHKeyID: "id"}})
			if tc.fail {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "id", out.Output.SSHKeyID)
			if tc.title == "minimal" {
				require.Equal(t, "", out.Output.Name)
				require.Nil(t, out.Output.Description)
			} else {
				require.Equal(t, "display", out.Output.Name)
				require.Equal(t, "", *out.Output.Description)
				require.Equal(t, "", *out.Output.PublicKey)
				require.Equal(t, "", *out.Output.OrganizationID)
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		r := GetSSHKey{client: func(context.Context) *client.Client { called = true; return nil }}
		_, err := r.Invoke(t.Context(), infer.FunctionRequest[GetSSHKeyArgs]{Input: GetSSHKeyArgs{SSHKeyID: id}})
		require.Error(t, err)
		require.False(t, called)
	}
}
