package dokploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

const githubRecord = `{"githubId":"relation","gitProviderId":"parent","githubAppName":"app","gitProvider":{"gitProviderId":"parent","name":"old","providerType":"github","organizationId":"org"}}`

func githubFixture(t *testing.T, record string, status int) *client.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/organization.active" {
			fmt.Fprint(w, `{"id":"org"}`)
			return
		}
		w.WriteHeader(status)
		fmt.Fprint(w, record)
	}))
	t.Cleanup(s.Close)
	c, err := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, err)
	return c
}

func TestGitHubIntegrationReadAbsenceClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		clears bool
	}{{http.StatusNotFound, true}, {http.StatusBadRequest, false}, {http.StatusForbidden, false}, {http.StatusInternalServerError, false}} {
		_, err := (GitHubIntegration{client: func(context.Context) *client.Client {
			return githubFixture(t, `{"message":"credential-sentinel"}`, tc.status)
		}}).Read(t.Context(), infer.ReadRequest[GitHubIntegrationArgs, GitHubIntegrationState]{ID: "relation"})
		if tc.clears {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.NotContains(t, err.Error(), "credential-sentinel")
		}
	}
}

func TestGitHubIntegrationObservationValidation(t *testing.T) {
	valid, err := readGitHubIntegration(t.Context(), githubFixture(t, githubRecord, 200), "relation", GitHubIntegrationState{}, "org")
	require.NoError(t, err, "valid response is a positive control for the rejection cases")
	require.Equal(t, GitHubIntegrationState{GitHubIntegrationArgs: GitHubIntegrationArgs{Name: "old", GitHubAppName: "app"}, GitHubID: "relation", GitProviderID: "parent", OrganizationID: "org"}, valid.State)
	for name, record := range map[string]string{
		"relation":            strings.Replace(githubRecord, `"githubId":"relation"`, `"githubId":"other"`, 1),
		"parent":              strings.Replace(githubRecord, `"gitProviderId":"parent","name"`, `"gitProviderId":"other","name"`, 1),
		"provider type":       strings.Replace(githubRecord, `"providerType":"github"`, `"providerType":"gitlab"`, 1),
		"organization":        strings.Replace(githubRecord, `"organizationId":"org"`, `"organizationId":"other"`, 1),
		"omitted app":         strings.Replace(githubRecord, `,"githubAppName":"app"`, "", 1),
		"omitted parent":      strings.Replace(githubRecord, `,"gitProvider":{"gitProviderId":"parent","name":"old","providerType":"github","organizationId":"org"}`, `,"gitProvider":{}`, 1),
		"missing parent name": strings.Replace(githubRecord, `"name":"old",`, ``, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readGitHubIntegration(t.Context(), githubFixture(t, record, 200), "relation", GitHubIntegrationState{}, "org")
			require.Error(t, err)
			require.EqualError(t, err, "github.one: incomplete or inconsistent response")
		})
	}
}

func TestGitHubIntegrationObservationPriorIdentity(t *testing.T) {
	for _, prior := range []GitHubIntegrationState{{GitHubID: "other"}, {GitProviderID: "other"}, {OrganizationID: "other"}} {
		_, err := readGitHubIntegration(t.Context(), githubFixture(t, githubRecord, 200), "relation", prior, "org")
		require.Error(t, err)
		require.EqualError(t, err, "github.one: incomplete or inconsistent response")
	}
}

func TestGitHubIntegrationIncompleteConfiguration(t *testing.T) {
	for _, value := range []string{`null`, `""`} {
		record := strings.Replace(githubRecord, `"githubAppName":"app"`, `"githubAppName":`+value, 1)
		obs, err := readGitHubIntegration(t.Context(), githubFixture(t, record, 200), "relation", GitHubIntegrationState{}, "org")
		require.NoError(t, err)
		require.False(t, obs.ConfigurationComplete)
		require.Empty(t, obs.State.GitHubAppName)
	}
}

func TestGitHubIntegrationDiagnostics(t *testing.T) {
	for _, err := range []error{fmt.Errorf("credential private.example: %w", context.Canceled), fmt.Errorf("credential private.example: %w", context.DeadlineExceeded), fmt.Errorf("credential private.example")} {
		safe := sanitizeGitHubIntegrationError(err)
		require.NotContains(t, safe.Error(), "credential")
		require.NotContains(t, safe.Error(), "private.example")
		if errors.Is(err, context.Canceled) {
			require.ErrorIs(t, safe, context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			require.ErrorIs(t, safe, context.DeadlineExceeded)
		}
	}
}
