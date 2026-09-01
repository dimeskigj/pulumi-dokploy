package dokploy

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	require.Equal(t, "@dimeskigj/pulumi-dokploy", languageSetting(spec, "nodejs", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "moduleName"))
	require.Equal(t, "Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
	require.Equal(t, "Pulumi", languageSetting(spec, "csharp", "rootNamespace"))
	require.Equal(t, "net.dimeski.pulumi", languageSetting(spec, "java", "basePackage"))
	require.Equal(t, "net.dimeski.pulumi.dokploy", languageSetting(spec, "java", "packageName"))
	require.NotNil(t, spec.Resources)
	require.ElementsMatch(t, []string{
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:MySQL", "dokploy:index:MariaDB",
		"dokploy:index:MongoDB", "dokploy:index:Redis", "dokploy:index:Domain",
		"dokploy:index:Destination", "dokploy:index:Backup", "dokploy:index:VolumeBackup",
		"dokploy:index:SSHKey", "dokploy:index:Registry", "dokploy:index:Tag", "dokploy:index:ProjectTag",
	}, resourceTokens(spec.Resources))
	for token, resource := range spec.Resources {
		require.NotEmpty(t, resource.Description, token)
		for property, spec := range resource.InputProperties {
			require.NotEmpty(t, spec.Description, token+"."+property)
		}
		for property, spec := range resource.Properties {
			require.NotEmpty(t, spec.Description, token+" output "+property)
		}
	}
	for property, variable := range spec.Config.Variables {
		require.NotEmpty(t, variable.Description, "config."+property)
	}
}

func TestSchemaSecretsAndDefaults(t *testing.T) {
	spec := providerSchema(t)
	require.True(t, spec.Config.Variables["apiKey"].Secret)
	require.Equal(t, "postgres:18", spec.Resources["dokploy:index:Postgres"].InputProperties["dockerImage"].Default)
	require.Equal(t, "mysql:8", spec.Resources["dokploy:index:MySQL"].InputProperties["dockerImage"].Default)
	require.Equal(t, "mariadb:11", spec.Resources["dokploy:index:MariaDB"].InputProperties["dockerImage"].Default)
	require.Equal(t, "mongo:8", spec.Resources["dokploy:index:MongoDB"].InputProperties["dockerImage"].Default)
	require.Equal(t, "redis:8", spec.Resources["dokploy:index:Redis"].InputProperties["dockerImage"].Default)
	require.Equal(t, "docker-compose", spec.Resources["dokploy:index:Compose"].InputProperties["composeType"].Default)
	require.Equal(t, true, spec.Resources["dokploy:index:Domain"].InputProperties["https"].Default)
	require.Equal(t, "letsencrypt", spec.Resources["dokploy:index:Domain"].InputProperties["certificateType"].Default)
	require.Equal(t, true, spec.Resources["dokploy:index:Domain"].InputProperties["enabled"].Default)

	secrets := []string{
		"dokploy:index:Application.environment", "dokploy:index:Application.buildArgs", "dokploy:index:Application.buildSecrets",
		"dokploy:index:Application.source.docker.password", "dokploy:index:Postgres.databasePassword",
		"dokploy:index:Postgres.environment", "dokploy:index:Redis.databasePassword", "dokploy:index:Redis.environment",
		"dokploy:index:MySQL.databasePassword", "dokploy:index:MySQL.databaseRootPassword", "dokploy:index:MySQL.environment",
		"dokploy:index:MariaDB.databasePassword", "dokploy:index:MariaDB.databaseRootPassword", "dokploy:index:MariaDB.environment",
		"dokploy:index:MongoDB.databasePassword", "dokploy:index:MongoDB.environment",
		"dokploy:index:Compose.environment",
		"dokploy:index:Destination.secretAccessKey",
		"dokploy:index:SSHKey.privateKey",
		"dokploy:index:Registry.password",
	}
	for _, path := range secrets {
		resource, property := splitSchemaPath(path)
		require.True(t, schemaProperty(spec, resource, property).Secret, path)
	}
	sshKeyProps := spec.Resources["dokploy:index:SSHKey"].InputProperties
	require.True(t, sshKeyProps["privateKey"].ReplaceOnChanges)
	require.True(t, sshKeyProps["publicKey"].ReplaceOnChanges)
}

func TestSchemaReplacementFlags(t *testing.T) {
	spec := providerSchema(t)
	for _, property := range []string{"projectId", "tagId"} {
		require.True(t, spec.Resources["dokploy:index:ProjectTag"].InputProperties[property].ReplaceOnChanges, property)
	}
	for _, resource := range []string{"Application", "Compose", "Postgres", "MySQL", "MariaDB", "MongoDB", "Redis"} {
		props := spec.Resources["dokploy:index:"+resource].InputProperties
		for _, property := range []string{"environmentId", "serverId"} {
			require.True(t, props[property].ReplaceOnChanges, resource+"."+property)
		}
	}
	props := spec.Resources["dokploy:index:Domain"].InputProperties
	require.True(t, props["applicationId"].ReplaceOnChanges)
	require.True(t, props["composeId"].ReplaceOnChanges)
	require.True(t, props["serviceName"].ReplaceOnChanges)

	volumeBackupProps := spec.Resources["dokploy:index:VolumeBackup"].InputProperties
	require.True(t, volumeBackupProps["applicationId"].ReplaceOnChanges)
	require.True(t, volumeBackupProps["composeId"].ReplaceOnChanges)
	require.True(t, volumeBackupProps["serviceName"].ReplaceOnChanges)

	backupProps := spec.Resources["dokploy:index:Backup"].InputProperties
	for _, property := range []string{"postgresId", "mysqlId", "mariadbId", "mongoId"} {
		require.True(t, backupProps[property].ReplaceOnChanges, property)
	}
}

func TestGeneratedDotnetAndJavaPackagesDoNotDuplicateProviderSuffix(t *testing.T) {
	dotnetProject := readGenerated(t, "sdk", "dotnet", "Pulumi.Dokploy.csproj")
	require.NotContains(t, dotnetProject, "Pulumi.Dokploy.Dokploy")
	dotnetProvider := readGenerated(t, "sdk", "dotnet", "Provider.cs")
	require.Contains(t, dotnetProvider, "namespace Pulumi.Dokploy\n")
	require.NotContains(t, dotnetProvider, "namespace Pulumi.Dokploy.Dokploy")

	javaProvider := readGenerated(t, "sdk", "java", "src", "main", "java", "net", "dimeski", "pulumi", "dokploy", "Provider.java")
	require.Contains(t, javaProvider, "package net.dimeski.pulumi.dokploy;")
	require.NotContains(t, javaProvider, "package net.dimeski.pulumi.dokploy.dokploy;")
	_, err := os.Stat(filepath.Join("..", "sdk", "java", "src", "main", "java", "net", "dimeski", "pulumi", "dokploy", "Provider.java"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join("..", "sdk", "java", "src", "main", "java", "net", "dimeski", "pulumi", "dokploy", "dokploy"))
	require.Error(t, err)
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
	if strings.HasPrefix(property, "source.git.") {
		source := spec.Types[trimTypeRef(resource.InputProperties["source"].Ref)]
		git := spec.Types[trimTypeRef(source.Properties["git"].Ref)]
		return git.Properties[strings.TrimPrefix(property, "source.git.")]
	}
	return resource.InputProperties[property]
}

func trimTypeRef(ref string) string { return strings.TrimPrefix(ref, "#/types/") }

func readGenerated(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{".."}, parts...)...))
	require.NoError(t, err)
	return string(data)
}

func languageSetting(spec schema.PackageSpec, language, key string) any {
	settings := map[string]any{}
	if err := json.Unmarshal(spec.Language[language], &settings); err != nil {
		return nil
	}
	return settings[key]
}
