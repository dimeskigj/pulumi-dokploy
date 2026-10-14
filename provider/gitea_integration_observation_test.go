package dokploy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

const giteaRecord = `{"giteaId":"relation","gitProviderId":"parent","giteaUrl":"https://gitea.example","clientId":"client","clientSecret":"secret","redirectUri":"https://dokploy.example/callback","giteaInternalUrl":null,"organizationName":"","gitProvider":{"gitProviderId":"parent","name":"display","providerType":"gitea","organizationId":"org","userId":"owner"}}`

func giteaFixture(t *testing.T, record string, status int) *client.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/organization.active" {
			_, _ = fmt.Fprint(w, `{"id":"org"}`)
			return
		}
		if r.URL.Path == "/api/gitea.one" {
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, record)
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	t.Cleanup(s.Close)
	c, e := client.New(s.URL, "fixture", client.WithRetryPolicy(client.RetryPolicy{Attempts: 1}))
	require.NoError(t, e)
	return c
}

func TestGiteaIntegrationReadAndImport(t *testing.T) {
	c := giteaFixture(t, giteaRecord, http.StatusOK)
	read, err := (GiteaIntegration{client: func(context.Context) *client.Client { return c }}).Read(t.Context(), infer.ReadRequest[GiteaIntegrationArgs, GiteaIntegrationState]{ID: "relation"})
	require.NoError(t, err)
	require.Equal(t, "relation", read.State.GiteaID)
	require.Equal(t, "parent", read.State.GitProviderID)
	require.Equal(t, "org", read.State.OrganizationID)
	require.Equal(t, "display", read.State.Name)
	require.Equal(t, "secret", read.State.ClientSecret)
	require.Nil(t, read.State.GiteaInternalURL)
}

func TestGiteaIntegrationObservationValidation(t *testing.T) {
	for _, tc := range []struct{ name, record string }{{"wrong relation", strings.Replace(giteaRecord, `"giteaId":"relation"`, `"giteaId":"other"`, 1)}, {"wrong type", strings.Replace(giteaRecord, `"providerType":"gitea"`, `"providerType":"github"`, 1)}, {"wrong organization", strings.Replace(giteaRecord, `"organizationId":"org"`, `"organizationId":"other"`, 1)}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readGiteaIntegration(t.Context(), giteaFixture(t, tc.record, 200), "relation", GiteaIntegrationState{}, "org")
			require.Error(t, err)
		})
	}
}

func TestGiteaIntegrationDiagnostics(t *testing.T) {
	err := sanitizeGiteaIntegrationError(fmt.Errorf("credential-sentinel https://private.example/id"))
	require.NotContains(t, err.Error(), "credential-sentinel")
	require.NotContains(t, err.Error(), "private.example")
	require.ErrorIs(t, sanitizeGiteaIntegrationError(context.Canceled), context.Canceled)
}

func TestGiteaIntegrationIncompleteImportPreservesOnlyOmittedSecret(t *testing.T) {
	record := strings.Replace(giteaRecord, `"clientSecret":"secret",`, "", 1)
	api := giteaFixture(t, record, http.StatusOK)
	prior := GiteaIntegrationState{GiteaIntegrationArgs: GiteaIntegrationArgs{ClientSecret: "prior"}}
	observed, err := readGiteaIntegration(t.Context(), api, "relation", prior, "org")
	require.NoError(t, err)
	require.Equal(t, "prior", observed.State.ClientSecret)
	require.False(t, observed.ConfigurationComplete)
	require.False(t, observed.SecretObserved)
	require.Empty(t, observed.State.OrganizationName)
	require.Nil(t, observed.State.GiteaInternalURL)
}
