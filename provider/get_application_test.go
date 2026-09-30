package dokploy

import (
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"testing"
)

func TestLookupWorkloadsApplication(t *testing.T) {
	for _, tc := range []struct {
		body string
		fail bool
	}{
		{`{"applicationId":"id","name":"n","applicationStatus":"future","registryId":"r","buildRegistryId":"b","env":"secret","buildArgs":"secret","source":{"token":"secret"}}`, false},
		{`{"applicationId":"id","name":"n","applicationStatus":"","registryId":null}`, false},
		{`{"applicationId":"id","name":"n"}`, false},
		{`{"applicationId":"id","name":"n","applicationStatus":false}`, true},
		{`{"applicationId":"other","name":"n"}`, true},
		{`null`, true},
	} {
		s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/application.one", Query: url.Values{"applicationId": {"id"}}, Status: 200, Response: []byte(tc.body)})
		out, err := (GetApplication{client: fixedClient(s.API())}).Invoke(t.Context(), infer.FunctionRequest[GetApplicationArgs]{Input: GetApplicationArgs{ApplicationID: "id"}})
		if tc.fail {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, "id", out.Output.ApplicationID)
		if out.Output.Status != nil {
			require.NotEqual(t, "secret", *out.Output.Status)
		}
	}
}
