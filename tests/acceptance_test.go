package tests

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	configValues := map[string]string{
		"databasePassword":        "mock-database-password",
		"redisPassword":           "mock-redis-password",
		"applicationEnvironment":  "mock-application-environment",
		"applicationBuildArgs":    "mock-application-build-args",
		"applicationBuildSecrets": "mock-application-build-secrets",
		"composeEnvironment":      "mock-compose-environment",
	}
	namespacedConfig := map[string]string{}
	for key, value := range configValues {
		namespacedConfig["mvp:"+key] = value
	}
	configJSON, err := json.Marshal(namespacedConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(pulumi.EnvConfig, string(configJSON))
	secretKeys, err := json.Marshal([]string{"mvp:databasePassword", "mvp:redisPassword", "mvp:applicationEnvironment", "mvp:applicationBuildArgs", "mvp:applicationBuildSecrets", "mvp:composeEnvironment"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(pulumi.EnvConfigSecretKeys, string(secretKeys))

	variants := []struct {
		name  string
		image string
	}{
		{name: "first image", image: "nginx:1.27"},
		{name: "second image", image: "nginx:1.28"},
	}
	observed := make([]map[string]string, 0, len(variants))
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			mocks := &captureMVPResources{}
			err := pulumi.RunErr(mvpProgram(liveConfig{NameSuffix: "mock"}, variant.image), pulumi.WithMocks("mvp", variant.name, mocks))
			if err != nil {
				t.Fatal(err)
			}
			assertSecretInput(t, mocks, "dokploy:index:Postgres", "databasePassword", configValues["databasePassword"])
			assertSecretInput(t, mocks, "dokploy:index:Redis", "databasePassword", configValues["redisPassword"])
			assertSecretInput(t, mocks, "dokploy:index:Application", "environment", configValues["applicationEnvironment"])
			assertSecretInput(t, mocks, "dokploy:index:Application", "buildArgs", configValues["applicationBuildArgs"])
			assertSecretInput(t, mocks, "dokploy:index:Application", "buildSecrets", configValues["applicationBuildSecrets"])
			assertSecretInput(t, mocks, "dokploy:index:Compose", "environment", configValues["composeEnvironment"])
			inputs := map[string]string{}
			for _, field := range []struct {
				token string
				key   string
				want  string
			}{
				{"dokploy:index:Postgres", "databasePassword", configValues["databasePassword"]},
				{"dokploy:index:Redis", "databasePassword", configValues["redisPassword"]},
				{"dokploy:index:Application", "environment", configValues["applicationEnvironment"]},
				{"dokploy:index:Application", "buildArgs", configValues["applicationBuildArgs"]},
				{"dokploy:index:Application", "buildSecrets", configValues["applicationBuildSecrets"]},
				{"dokploy:index:Compose", "environment", configValues["composeEnvironment"]},
			} {
				inputs[field.token+"."+field.key] = secretInputValue(t, mocks, field.token, field.key, field.want)
			}
			application := mocks.resource("dokploy:index:Application")
			docker := application[resource.PropertyKey("source")].ObjectValue()[resource.PropertyKey("docker")].ObjectValue()
			inputs["dokploy:index:Application.source.docker.image"] = docker[resource.PropertyKey("image")].StringValue()
			if got := inputs["dokploy:index:Application.source.docker.image"]; got != variant.image {
				t.Errorf("application image = %q, want %q", got, variant.image)
			}
			observed = append(observed, inputs)
		})
	}
	for key, first := range observed[0] {
		if key == "dokploy:index:Application.source.docker.image" {
			continue
		}
		if got := observed[1][key]; got != first {
			t.Errorf("%s differs between image variants: %q != %q", key, first, got)
		}
	}
	if observed[0]["dokploy:index:Application.source.docker.image"] == observed[1]["dokploy:index:Application.source.docker.image"] {
		t.Fatal("image variants did not differ")
	}
}

type captureMVPResources struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
}

func (m *captureMVPResources) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func (m *captureMVPResources) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.resources == nil {
		m.resources = map[string]resource.PropertyMap{}
	}
	m.resources[args.TypeToken] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *captureMVPResources) resource(token string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resources[token]
}

func assertSecretInput(t *testing.T, mocks *captureMVPResources, token, key, want string) {
	t.Helper()
	secretInputValue(t, mocks, token, key, want)
}

func secretInputValue(t *testing.T, mocks *captureMVPResources, token, key, want string) string {
	t.Helper()
	value := mocks.resource(token)[resource.PropertyKey(key)]
	if !value.IsSecret() && (!value.IsOutput() || !value.OutputValue().Secret) {
		t.Fatalf("%s.%s did not retain secret propagation: %#v", token, key, value)
	}
	if value.IsSecret() {
		value = value.SecretValue().Element
	} else {
		value = value.OutputValue().Element
	}
	if got := value.StringValue(); got != want {
		t.Errorf("%s.%s = %q, want %q", token, key, got, want)
	}
	return value.StringValue()
}
