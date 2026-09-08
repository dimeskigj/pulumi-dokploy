package dokploy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestConfigConfigure(t *testing.T) {
	t.Run("requires endpoint", func(t *testing.T) {
		c := Config{APIKey: "token"}
		require.EqualError(t, c.Configure(t.Context()), "endpoint is required")
	})
	t.Run("requires api key", func(t *testing.T) {
		c := Config{Endpoint: "https://dokploy.example"}
		require.EqualError(t, c.Configure(t.Context()), "apiKey is required")
	})
	t.Run("creates client", func(t *testing.T) {
		c := Config{Endpoint: "https://dokploy.example/", APIKey: "token"}
		require.NoError(t, c.Configure(t.Context()))
		require.NotNil(t, c.client)
	})
	t.Run("configures provider user agent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "pulumi-dokploy/1.2.3", r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"id":"p1"}`)
		}))
		t.Cleanup(server.Close)

		originalVersion := Version
		Version = "v1.2.3"
		t.Cleanup(func() { Version = originalVersion })
		c := Config{Endpoint: server.URL, APIKey: "token"}
		require.NoError(t, c.Configure(t.Context()))
		_, err := c.client.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
		require.NoError(t, err)
	})
}
