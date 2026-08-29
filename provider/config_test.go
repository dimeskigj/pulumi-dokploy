package dokploy

import (
	"testing"

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
}
