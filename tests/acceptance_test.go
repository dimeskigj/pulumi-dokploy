package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccMVP(t *testing.T) {
	timeout := 30 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	endpoint := os.Getenv("DOKPLOY_ENDPOINT")
	apiKey := os.Getenv("DOKPLOY_API_KEY")
	if endpoint == "" || apiKey == "" {
		t.Skip("live Dokploy credentials are not configured")
	}
	runMVP(t, ctx, liveConfig{Endpoint: endpoint, APIKey: apiKey, NameSuffix: uuid.NewString()})
}
