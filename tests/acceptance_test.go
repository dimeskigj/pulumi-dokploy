package tests

import (
	"context"
	"os"
	"strings"
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

func TestMVPProgramUsesConfiguredSecrets(t *testing.T) {
	source, err := os.ReadFile("acceptance_program_test.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, key := range []string{"databasePassword", "redisPassword", "applicationEnvironment", "applicationBuildArgs", "applicationBuildSecrets", "composeEnvironment"} {
		if !strings.Contains(text, `RequireSecret("`+key+`")`) {
			t.Errorf("inline program does not read secret config %q", key)
		}
	}
	for _, literal := range []string{"task12-db", "task12-redis", "APP_SECRET=task12"} {
		if strings.Contains(text, literal) {
			t.Errorf("inline program contains hardcoded secret %q", literal)
		}
	}
}
