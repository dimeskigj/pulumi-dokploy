package dokploy

import (
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"
)

func providerSchema(t *testing.T) schema.PackageSpec {
	t.Helper()
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	return spec
}

func TestSchemaHasExactlyTheMVPResources(t *testing.T) {
	spec := providerSchema(t)
	require.Empty(t, spec.Functions)
	require.NotNil(t, spec.Resources)
	require.ElementsMatch(t, []string{
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:Redis", "dokploy:index:Domain",
	}, resourceTokens(spec.Resources))
	for token, resource := range spec.Resources {
		require.NotEmpty(t, resource.Description, token)
		for property, spec := range resource.InputProperties {
			require.NotEmpty(t, spec.Description, token+"."+property)
		}
	}
}

func TestSchemaSecretsAndDefaults(t *testing.T) {
	spec := providerSchema(t)
	require.True(t, spec.Config.Variables["apiKey"].Secret)
	require.Equal(t, "postgres:18", spec.Resources["dokploy:index:Postgres"].InputProperties["dockerImage"].Default)
	require.Equal(t, "redis:8", spec.Resources["dokploy:index:Redis"].InputProperties["dockerImage"].Default)
	require.Equal(t, "docker-compose", spec.Resources["dokploy:index:Compose"].InputProperties["composeType"].Default)
	require.Equal(t, true, spec.Resources["dokploy:index:Domain"].InputProperties["https"].Default)
	require.Equal(t, "letsencrypt", spec.Resources["dokploy:index:Domain"].InputProperties["certificateType"].Default)
	require.Equal(t, true, spec.Resources["dokploy:index:Domain"].InputProperties["enabled"].Default)

	secrets := []string{
		"dokploy:index:Application.environment", "dokploy:index:Application.buildArgs", "dokploy:index:Application.buildSecrets",
		"dokploy:index:Application.source.docker.password", "dokploy:index:Postgres.databasePassword",
		"dokploy:index:Postgres.environment", "dokploy:index:Redis.databasePassword", "dokploy:index:Redis.environment",
		"dokploy:index:Compose.environment",
	}
	for _, path := range secrets {
		resource, property := splitSchemaPath(path)
		require.True(t, schemaProperty(spec, resource, property).Secret, path)
	}
}

func TestSchemaReplacementFlags(t *testing.T) {
	spec := providerSchema(t)
	for _, resource := range []string{"Application", "Compose", "Postgres", "Redis"} {
		props := spec.Resources["dokploy:index:"+resource].InputProperties
		for _, property := range []string{"environmentId", "serverId"} {
			require.True(t, props[property].ReplaceOnChanges, resource+"."+property)
		}
	}
	props := spec.Resources["dokploy:index:Domain"].InputProperties
	require.True(t, props["applicationId"].ReplaceOnChanges)
	require.True(t, props["composeId"].ReplaceOnChanges)
	require.True(t, props["serviceName"].ReplaceOnChanges)
}

func resourceTokens(resources map[string]schema.ResourceSpec) []string {
	result := make([]string, 0, len(resources))
	for token := range resources {
		result = append(result, token)
	}
	return result
}

func splitSchemaPath(path string) (string, string) {
	if strings.HasSuffix(path, ".source.docker.password") {
		return strings.TrimSuffix(path, ".source.docker.password"), "source.docker.password"
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[:i], path[i+1:]
		}
	}
	return path, ""
}

func schemaProperty(spec schema.PackageSpec, resourceToken, property string) schema.PropertySpec {
	resource := spec.Resources[resourceToken]
	if property == "source.docker.password" {
		source := spec.Types[trimTypeRef(resource.InputProperties["source"].Ref)]
		docker := spec.Types[trimTypeRef(source.Properties["docker"].Ref)]
		return docker.Properties["password"]
	}
	return resource.InputProperties[property]
}

func trimTypeRef(ref string) string { return strings.TrimPrefix(ref, "#/types/") }
