package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestGitHubGeneratedClientContract(t *testing.T) {
	var updateBody, removeBody map[string]any
	var requestedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/github.one":
			requestedID = r.URL.Query().Get("githubId")
			if requestedID == "omitted" {
				_, _ = w.Write([]byte(`{"githubId":"omitted","gitProviderId":"parent-1","gitProvider":{"gitProviderId":"parent-1","name":"parent","providerType":"github","organizationId":"org-1"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"githubId":"opaque/id +?","gitProviderId":"parent-1","githubAppName":null,"gitProvider":{"gitProviderId":"parent-1","name":"parent","providerType":"github","organizationId":"org-1"},"privateKey":"ignore"}`))
		case "/api/github.update":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updateBody))
			_, _ = w.Write([]byte(`{"irrelevant":"success"}`))
		case "/api/gitProvider.remove":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&removeBody))
			_, _ = w.Write([]byte(`{not-json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "test-key")
	require.NoError(t, err)

	one, err := c.GithubOneWithResponse(t.Context(), &generated.GithubOneParams{GithubId: "opaque/id +?"})
	require.NoError(t, err)
	require.Equal(t, "opaque/id +?", requestedID)
	require.NotNil(t, one.JSON200)
	require.True(t, one.JSON200.GithubAppName.IsNull())
	require.Contains(t, one.JSON200.AdditionalProperties, "privateKey", "unmodeled response fields remain available for provider-side whitelisting")
	require.Equal(t, "parent-1", one.JSON200.GitProvider.GitProviderId)
	omitted, err := c.GithubOneWithResponse(t.Context(), &generated.GithubOneParams{GithubId: "omitted"})
	require.NoError(t, err)
	require.False(t, omitted.JSON200.GithubAppName.IsSpecified(), "missing is distinguishable from explicit null")

	_, err = c.GithubUpdate(t.Context(), generated.GithubUpdateJSONRequestBody{GithubId: "relation-1", GitProviderId: "parent-1", Name: "display", GithubAppName: "App"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"githubId": "relation-1", "gitProviderId": "parent-1", "name": "display", "githubAppName": "App"}, updateBody)
	_, err = c.GitProviderRemove(t.Context(), generated.GitProviderRemoveJSONRequestBody{GitProviderId: "parent-1"})
	require.NoError(t, err)
	require.Equal(t, "parent-1", removeBody["gitProviderId"])
}
