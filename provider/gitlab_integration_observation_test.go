package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestGitLabIntegrationReadAndImport(t *testing.T) {
	c, paths := gitLabRouter(t, gitLabRecord, nil)
	r := GitLabIntegration{client: func(context.Context) *client.Client { return c }}
	read, err := r.Read(t.Context(), infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", State: gitLabPrior()})
	require.NoError(t, err)
	require.Equal(t, "relation", read.ID)
	require.Equal(t, "prior-secret", read.State.ApplicationSecret)
	require.False(t, read.State.IsConfigured)
	require.Equal(t, "", read.State.GroupName)
	require.Nil(t, read.State.GitlabInternalURL)
	encoded, err := json.Marshal(read)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "accessToken")
	require.NotContains(t, string(encoded), "refreshToken")
	require.NotContains(t, string(encoded), "access-token-sentinel")
	require.Equal(t, []string{"/api/organization.active", "/api/gitlab.one"}, *paths)
	imported, err := r.Read(t.Context(), infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation"})
	require.NoError(t, err)
	require.Equal(t, "relation", imported.State.GitLabID)
	require.Empty(t, imported.State.ApplicationSecret)
}

func TestGitLabIntegrationConfigurationComplete(t *testing.T) {
	for _, tt := range []struct {
		name, record string
		complete     bool
	}{
		{"observable", strings.Replace(gitLabRecord, `"secret":null`, `"secret":"prior-secret"`, 1), true},
		{"null secret", gitLabRecord, false},
		{"missing group", strings.Replace(strings.Replace(gitLabRecord, `"secret":null`, `"secret":"prior-secret"`, 1), `,"groupName":null`, "", 1), false},
		{"missing internal", strings.Replace(strings.Replace(gitLabRecord, `"secret":null`, `"secret":"prior-secret"`, 1), `,"gitlabInternalUrl":null`, "", 1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gitLabRouter(t, tt.record, nil)
			observed, err := readGitLabIntegration(t.Context(), c, "relation", gitLabPrior(), "org")
			require.NoError(t, err)
			require.Equal(t, tt.complete, observed.ConfigurationComplete)
		})
	}
}

func TestGitLabIntegrationObservationValidation(t *testing.T) {
	for name, change := range map[string]func(string) string{
		"relation": func(s string) string { return strings.Replace(s, `"gitlabId":"relation"`, `"gitlabId":"wrong"`, 1) },
		"parent": func(s string) string {
			return strings.Replace(s, `"gitProviderId":"parent","name"`, `"gitProviderId":"wrong","name"`, 1)
		},
		"type":          func(s string) string { return strings.Replace(s, `"type":"gitlab"`, `"type":"github"`, 1) },
		"missing token": func(s string) string { return strings.Replace(s, `,"accessToken":null`, "", 1) },
		"organization": func(s string) string {
			return strings.Replace(s, `"organizationId":"org"`, `"organizationId":"wrong"`, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := gitLabRouter(t, change(gitLabRecord), nil)
			_, err := readGitLabIntegration(t.Context(), c, "relation", gitLabPrior(), "org")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "access-token-sentinel")
		})
	}
}

func TestGitLabIntegrationDiagnostics(t *testing.T) {
	for _, err := range []error{errors.New("access-token-sentinel private.example"), fmt.Errorf("wrapped: %w", context.Canceled), context.DeadlineExceeded} {
		safe := sanitizeGitLabIntegrationError(err)
		require.NotContains(t, safe.Error(), "access-token-sentinel")
		require.NotContains(t, safe.Error(), "private.example")
		if errors.Is(err, context.Canceled) {
			require.ErrorIs(t, safe, context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			require.ErrorIs(t, safe, context.DeadlineExceeded)
		}
	}
}

func TestGitLabIntegrationReadMalformedAndMissing(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		absent     bool
	}{
		{"malformed success", `{access-token-sentinel`, 200, false},
		{"unexpected secret-bearing prose", `{"message":"access-token-sentinel"}`, 403, false},
		{"not found", `{"message":"access-token-sentinel"}`, 404, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/organization.active" {
					fmt.Fprint(w, `{"id":"org"}`)
					return
				}
				require.Equal(t, "/api/gitlab.one", r.URL.Path)
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})
			read, err := (GitLabIntegration{client: func(context.Context) *client.Client { return c }}).Read(t.Context(), infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]{ID: "relation", State: gitLabPrior()})
			if tt.absent {
				require.NoError(t, err)
				require.Empty(t, read.ID)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "access-token-sentinel")
			}
		})
	}
}

func TestGitLabIntegrationProviderListValidation(t *testing.T) {
	valid := `[{"gitProviderId":"parent","name":"old","type":"gitlab","organizationId":"org","userId":"user","gitlab":{"gitlabId":"relation","gitlabUrl":"https://gitlab.example","isConfigured":false}},{"gitProviderId":"other","name":"github","type":"github","organizationId":"org","userId":"user","gitlab":null}]`
	for _, tt := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", valid, true},
		{"duplicate parent", strings.Replace(valid, `"gitProviderId":"other"`, `"gitProviderId":"parent"`, 1), false},
		{"duplicate relation", strings.Replace(strings.Replace(valid, `"type":"github"`, `"type":"gitlab"`, 1), `"gitlab":null`, `"gitlab":{"gitlabId":"relation","gitlabUrl":"https://gitlab.example","isConfigured":false}`, 1), false},
		{"missing owner", strings.Replace(valid, `"userId":"user"`, `"userId":""`, 1), false},
		{"wrong organization", strings.Replace(valid, `"organizationId":"org"`, `"organizationId":"other"`, 1), false},
		{"wrong type", strings.Replace(valid, `"type":"gitlab"`, `"type":"github"`, 1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := gitLabFixture(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/gitProvider.getAll", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tt.body)
			})
			list, err := readGitLabProviderList(t.Context(), c, "org")
			if tt.valid {
				require.NoError(t, err)
				require.Len(t, list, 2)
			} else {
				require.Error(t, err)
			}
		})
	}
}
