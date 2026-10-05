package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/require"
)

func TestGitLabGeneratedClientContract(t *testing.T) {
	var createBody, updateBody, removeBody map[string]any
	var requestedGitlabID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/gitlab.create":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createBody))
			// Successful mutations may acknowledge without a response body.
		case "/api/gitlab.update":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updateBody))
			_, _ = w.Write([]byte(`{not-json`))
		case "/api/gitProvider.remove":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&removeBody))
			_, _ = w.Write([]byte(`{}`))
		case "/api/gitlab.one":
			requestedGitlabID = r.URL.Query().Get("gitlabId")
			if requestedGitlabID == "omitted-token" {
				_, _ = w.Write([]byte(`{"gitlabId":"omitted-token","gitProviderId":"provider-1","gitlabUrl":"https://gitlab.example","gitProvider":{"gitProviderId":"provider-1"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"gitlabId":"opaque/id +?","gitProviderId":"provider-1","gitlabUrl":"https://gitlab.example","gitProvider":{"gitProviderId":"provider-1"},"gitlabInternalUrl":null,"accessToken":null,"refreshToken":"refresh-value","privateEmail":"ignored"}`))
		case "/api/gitProvider.getAll":
			_, _ = w.Write([]byte(`[{"gitProviderId":"provider-1","gitlab":{"gitlabId":"integration-1","applicationId":null,"gitlabUrl":"https://gitlab.example","isConfigured":true,"applicationSecret":"ignore","github":{"token":"ignore"}}},{"gitProviderId":"provider-2","gitlab":null,"otherProvider":{"accessToken":"ignore"}},{"gitProviderId":"provider-3"}]`))
		case "/api/user.get":
			_, _ = w.Write([]byte(`{"userId":"member-1","organizationId":"org-1","user":{"id":"opaque-user","email":"private@example.test"},"name":"private"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "test-key")
	require.NoError(t, err)

	groupName := ""
	secret := "application-secret"
	_, err = c.GitlabCreate(t.Context(), generated.GitlabCreateJSONRequestBody{
		AuthId: "auth-1", Name: "integration", GitlabUrl: "https://gitlab.example",
		Secret: &secret, GroupName: &groupName,
	})
	require.NoError(t, err)
	require.Equal(t, "auth-1", createBody["authId"])
	require.Equal(t, "application-secret", createBody["secret"], "application secret maps to upstream secret field")
	require.Equal(t, "", createBody["groupName"])

	name := "updated"
	_, err = c.GitlabUpdate(t.Context(), generated.GitlabUpdateJSONRequestBody{
		GitlabId: "integration-1", GitProviderId: "provider-1", Name: name,
		GitlabUrl: "https://gitlab.example", GitlabInternalUrl: nullable.NewNullNullable[string](),
	})
	require.NoError(t, err)
	require.Equal(t, "integration-1", updateBody["gitlabId"])
	require.Equal(t, "provider-1", updateBody["gitProviderId"])
	require.Contains(t, updateBody, "gitlabInternalUrl")
	require.Nil(t, updateBody["gitlabInternalUrl"])

	one, err := c.GitlabOneWithResponse(t.Context(), &generated.GitlabOneParams{GitlabId: "opaque/id +?"})
	require.NoError(t, err)
	require.Equal(t, "opaque/id +?", requestedGitlabID)
	require.NotNil(t, one.JSON200)
	require.Equal(t, "provider-1", one.JSON200.GitProvider.GitProviderId)
	require.True(t, one.JSON200.GitlabInternalUrl.IsNull())
	require.True(t, one.JSON200.AccessToken.IsNull())
	require.False(t, one.JSON200.RefreshToken.IsNull())
	require.True(t, one.JSON200.RefreshToken.IsSpecified())
	require.Equal(t, "refresh-value", one.JSON200.RefreshToken.MustGet())
	require.Equal(t, "ignored", one.JSON200.AdditionalProperties["privateEmail"])
	omitted, err := c.GitlabOneWithResponse(t.Context(), &generated.GitlabOneParams{GitlabId: "omitted-token"})
	require.NoError(t, err)
	require.False(t, omitted.JSON200.AccessToken.IsSpecified())

	list, err := c.GitProviderGetAllWithResponse(t.Context())
	require.NoError(t, err)
	require.NotNil(t, list.JSON200)
	require.Len(t, *list.JSON200, 3, "incomplete parent provider entries remain visible")
	require.Equal(t, "provider-1", (*list.JSON200)[0].GitProviderId)
	summary := (*list.JSON200)[0].Gitlab.MustGet()
	require.True(t, summary.ApplicationId.IsNull())
	require.Equal(t, "ignore", summary.AdditionalProperties["applicationSecret"])
	require.Equal(t, "ignore", summary.AdditionalProperties["github"].(map[string]any)["token"])
	require.True(t, (*list.JSON200)[1].Gitlab.IsNull())
	require.Equal(t, "ignore", (*list.JSON200)[1].AdditionalProperties["otherProvider"].(map[string]any)["accessToken"])
	require.False(t, (*list.JSON200)[2].Gitlab.IsSpecified())
	_, err = c.GitProviderRemove(t.Context(), generated.GitProviderRemoveJSONRequestBody{GitProviderId: "provider-1"})
	require.NoError(t, err)
	require.Equal(t, "provider-1", removeBody["gitProviderId"])

	member, err := c.UserGetWithResponse(t.Context())
	require.NoError(t, err)
	require.NotNil(t, member.JSON200)
	require.Equal(t, "opaque-user", member.JSON200.User.Id)
	require.Equal(t, "private", member.JSON200.AdditionalProperties["name"])
	require.Equal(t, "private@example.test", member.JSON200.User.AdditionalProperties["email"])
}
