package dokploy

import (
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"testing"
)

func TestLookupWorkloadsCompose(t *testing.T) {
	for _, tc := range []struct {
		body string
		fail bool
	}{
		{`{"composeId":"id","name":"n","composeStatus":"future","composeType":"future","composeFile":"secret","command":"secret","env":"secret"}`, false},
		{`{"composeId":"id","name":"n","composeStatus":""}`, false},
		{`{"composeId":"id","name":"n"}`, false},
		{`{"composeId":"id","name":"n","composeStatus":4}`, true},
		{`{"composeId":"other","name":"n"}`, true},
		{`null`, true},
	} {
		s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/compose.one", Query: url.Values{"composeId": {"id"}}, Status: 200, Response: []byte(tc.body)})
		out, err := (GetCompose{client: fixedClient(s.API())}).Invoke(t.Context(), infer.FunctionRequest[GetComposeArgs]{Input: GetComposeArgs{ComposeID: "id"}})
		if tc.fail {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, "id", out.Output.ComposeID)
	}
}
