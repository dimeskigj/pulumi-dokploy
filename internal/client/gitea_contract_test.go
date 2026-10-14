package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestGiteaGeneratedClientContract(t *testing.T) {
	var create, update map[string]any
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/gitea.create":
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&create)
			_, _ = w.Write([]byte(`{"giteaId":"g-1","clientId":null,"giteaUrl":"https://git.example"}`))
		case "/api/gitea.one":
			got = r.URL.Query().Get("giteaId")
			_, _ = w.Write([]byte(`{"giteaId":"opaque/id +?","gitProviderId":"p1","giteaUrl":"https://git.example","clientId":null,"clientSecret":"secret","redirectUri":"","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"p1","name":"p","providerType":"gitea","organizationId":"o1","userId":"u1"}}`))
		case "/api/gitea.update":
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&update)
			_, _ = w.Write([]byte(`{malformed`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "key")
	require.NoError(t, err)
	ack, err := c.GiteaCreateWithResponse(t.Context(), generated.GiteaCreateJSONRequestBody{GiteaUrl: "https://git.example", Name: "display"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, ack.HTTPResponse.StatusCode)
	require.Equal(t, "g-1", ack.JSON200.GiteaId)
	require.True(t, ack.JSON200.ClientId.IsNull())
	detail, err := c.GiteaOneWithResponse(t.Context(), &generated.GiteaOneParams{GiteaId: "opaque/id +?"})
	require.NoError(t, err)
	require.Equal(t, "opaque/id +?", got)
	require.True(t, detail.JSON200.ClientId.IsNull())
	require.True(t, detail.JSON200.GiteaInternalUrl.IsNull())
	require.True(t, detail.JSON200.OrganizationName.IsSpecified())
	require.Equal(t, "", detail.JSON200.OrganizationName.MustGet())
	require.Equal(t, "secret", detail.JSON200.ClientSecret.MustGet())
	resp, err := c.GiteaUpdateWithResponse(t.Context(), generated.GiteaUpdateJSONRequestBody{GiteaId: "g1", GitProviderId: "p1", GiteaUrl: "https://git.example", Name: "display"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.HTTPResponse.StatusCode)
	require.Equal(t, map[string]any{"giteaUrl": "https://git.example", "name": "display"}, create)
	require.Equal(t, map[string]any{"giteaId": "g1", "gitProviderId": "p1", "giteaUrl": "https://git.example", "name": "display"}, update)
	require.NotContains(t, create, "accessToken")
	require.NotContains(t, update, "scopes")
}
