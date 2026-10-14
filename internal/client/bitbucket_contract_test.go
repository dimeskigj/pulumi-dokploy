package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestBitbucketGeneratedClientContract(t *testing.T) {
	var create, update map[string]any
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/bitbucket.create":
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&create)
			_, _ = w.Write([]byte(`{not-json`))
		case "/api/bitbucket.one":
			got = r.URL.Query().Get("bitbucketId")
			_, _ = w.Write([]byte(`{"bitbucketId":"opaque/id +?","gitProviderId":"p1","bitbucketUsername":null,"bitbucketEmail":"","bitbucketWorkspaceName":"","apiToken":"token","appPassword":null,"gitProvider":{"gitProviderId":"p1","name":"p","providerType":"bitbucket","organizationId":"o1","userId":"u1","sharing":true}}`))
		case "/api/bitbucket.update":
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&update)
			_, _ = w.Write([]byte(`{irrelevant`))
		case "/api/gitProvider.getAll":
			_, _ = w.Write([]byte(`[{"gitProviderId":"p1","bitbucket":{"bitbucketId":"b1","bitbucketUsername":null,"isConfigured":false,"isDeprecated":false},"gitlab":null,"sharing":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "key")
	require.NoError(t, err)
	_, err = c.BitbucketCreate(t.Context(), generated.BitbucketCreateJSONRequestBody{AuthId: "u1", Name: "display"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"authId": "u1", "name": "display"}, create)
	detail, err := c.BitbucketOneWithResponse(t.Context(), &generated.BitbucketOneParams{BitbucketId: "opaque/id +?"})
	require.NoError(t, err)
	require.Equal(t, "opaque/id +?", got)
	require.True(t, detail.JSON200.BitbucketUsername.IsNull())
	require.True(t, detail.JSON200.ApiToken.IsSpecified())
	require.Equal(t, "token", detail.JSON200.ApiToken.MustGet())
	resp, err := c.BitbucketUpdateWithResponse(t.Context(), generated.BitbucketUpdateJSONRequestBody{BitbucketId: "b1", GitProviderId: "p1", Name: "display"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.HTTPResponse.StatusCode)
	require.NotContains(t, update, "apiToken")
	require.NotContains(t, update, "organizationId")
	list, err := c.GitProviderGetAllWithResponse(t.Context())
	require.NoError(t, err)
	require.Len(t, *list.JSON200, 1)
	require.True(t, (*list.JSON200)[0].Bitbucket.IsSpecified())
	require.False(t, (*list.JSON200)[0].Bitbucket.MustGet().IsConfigured)
	require.True(t, (*list.JSON200)[0].Bitbucket.MustGet().BitbucketUsername.IsNull())
	require.True(t, (*list.JSON200)[0].Gitlab.IsNull())
	require.Contains(t, (*list.JSON200)[0].AdditionalProperties, "sharing")
}
