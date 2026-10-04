package dokploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

const registryLogoURL = "https://raw.githubusercontent.com/dimeskigj/pulumi-dokploy/main/website/public/logo.svg"

func providerSchema(t *testing.T) schema.PackageSpec {
	t.Helper()
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	return spec
}

func TestSchemaHasExactlyTheMVPResources(t *testing.T) {
	spec := providerSchema(t)
	require.ElementsMatch(t, []string{"dokploy:index:getProject", "dokploy:index:getEnvironment", "dokploy:index:getApplication", "dokploy:index:getCompose", "dokploy:index:getPostgres", "dokploy:index:getMySQL", "dokploy:index:getMariaDB", "dokploy:index:getMongoDB", "dokploy:index:getRedis", "dokploy:index:getServer", "dokploy:index:getRegistry", "dokploy:index:getSSHKey"}, functionTokens(spec.Functions))
	for token, function := range spec.Functions {
		require.NotEmpty(t, function.Description, token)
		require.Len(t, function.Inputs.Required, 1)
		for key, field := range function.Inputs.Properties {
			require.NotEmpty(t, field.Description, token+" input "+key)
			require.Nil(t, field.Default)
		}
		require.NotNil(t, function.ReturnType)
		require.NotNil(t, function.ReturnType.ObjectTypeSpec)
		for key, field := range function.ReturnType.ObjectTypeSpec.Properties {
			require.NotEmpty(t, field.Description, token+" output "+key)
			require.False(t, field.Secret)
		}
	}
	for _, contract := range []struct {
		token, id string
		fields    []string
	}{
		{"getProject", "projectId", []string{"description", "defaultEnvironmentId"}},
		{"getEnvironment", "environmentId", []string{"description", "projectId", "isDefault"}},
		{"getApplication", "applicationId", []string{"description", "appName", "environmentId", "serverId", "status", "registryId", "buildRegistryId"}},
		{"getCompose", "composeId", []string{"description", "appName", "environmentId", "serverId", "status", "composeType"}},
		{"getPostgres", "postgresId", []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort", "databaseName", "databaseUser"}},
		{"getMySQL", "mysqlId", []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort", "databaseName", "databaseUser"}},
		{"getMariaDB", "mariadbId", []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort", "databaseName", "databaseUser"}},
		{"getMongoDB", "mongoId", []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort", "databaseUser", "replicaSets"}},
		{"getRedis", "redisId", []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort"}},
		{"getServer", "serverId", []string{"description", "ipAddress", "port", "username", "organizationId", "sshKeyId", "serverType", "status"}},
		{"getRegistry", "registryId", []string{"url", "username", "imagePrefix", "serverId", "registryType"}},
		{"getSSHKey", "sshKeyId", []string{"description", "publicKey", "organizationId"}},
	} {
		fn := spec.Functions["dokploy:index:"+contract.token]
		require.Equal(t, []string{contract.id}, fn.Inputs.Required)
		require.Equal(t, []string{contract.id}, objectKeys(fn.Inputs.Properties))
		output := fn.ReturnType.ObjectTypeSpec
		require.ElementsMatch(t, []string{contract.id, "name"}, output.Required)
		require.ElementsMatch(t, append([]string{contract.id, "name"}, contract.fields...), objectKeys(output.Properties))
		for _, field := range contract.fields {
			kind := "string"
			if field == "isDefault" || field == "replicaSets" {
				kind = "boolean"
			}
			if field == "port" || field == "externalPort" {
				kind = "integer"
			}
			require.Equal(t, kind, output.Properties[field].Type, contract.token+"."+field)
		}
	}
	require.Equal(t, "@dimeskigj/pulumi-dokploy", languageSetting(spec, "nodejs", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "moduleName"))
	require.Equal(t, "Dimeskigj.Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
	require.Equal(t, "Dimeskigj.Pulumi", languageSetting(spec, "csharp", "rootNamespace"))
	require.Equal(t, "net.dimeski.pulumi", languageSetting(spec, "java", "basePackage"))
	require.Equal(t, "net.dimeski.pulumi.dokploy", languageSetting(spec, "java", "packageName"))
	require.NotNil(t, spec.Resources)
	require.ElementsMatch(t, []string{
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:MySQL", "dokploy:index:MariaDB",
		"dokploy:index:MongoDB", "dokploy:index:Redis", "dokploy:index:Domain",
		"dokploy:index:Destination", "dokploy:index:Backup", "dokploy:index:VolumeBackup",
		"dokploy:index:SSHKey", "dokploy:index:Registry", "dokploy:index:Tag", "dokploy:index:ProjectTag", "dokploy:index:Mount",
		"dokploy:index:Schedule", "dokploy:index:Port",
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
	for _, term := range []string{"SSH keys", "registries", "tags", "project-tag associations", "mounts"} {
		require.Contains(t, spec.Description, term)
	}
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
	require.True(t, spec.Resources["dokploy:index:Registry"].InputProperties["password"].Secret)
	require.True(t, spec.Resources["dokploy:index:Mount"].InputProperties["content"].Secret)
}

func TestSchemaScheduleContract(t *testing.T) {
	spec := providerSchema(t)
	require.Contains(t, spec.Description, "schedules")
	r, ok := spec.Resources["dokploy:index:Schedule"]
	require.True(t, ok)
	require.Equal(t, false, r.InputProperties["enabled"].Default)
	require.Equal(t, "boolean", r.InputProperties["enabled"].Type)
	require.Equal(t, "boolean", r.Properties["enabled"].Type)
	for _, k := range []string{"command", "script"} {
		require.True(t, r.InputProperties[k].Secret, k)
		require.True(t, r.Properties[k].Secret, k)
	}
	for _, k := range []string{"scheduleType", "applicationId", "composeId", "serverId", "appName", "serviceName"} {
		require.True(t, r.InputProperties[k].ReplaceOnChanges, k)
	}
	for _, k := range []string{"name", "cronExpression", "command", "script", "timezone", "shellType", "description", "enabled"} {
		require.False(t, r.InputProperties[k].ReplaceOnChanges, k)
	}
	require.Contains(t, r.Properties, "scheduleId")
	require.NotContains(t, r.InputProperties, "scheduleId")
	require.NotContains(t, r.Properties, "createdAt")
}

func TestSchemaPortContract(t *testing.T) {
	spec := providerSchema(t)
	r, ok := spec.Resources["dokploy:index:Port"]
	require.True(t, ok)
	require.Contains(t, spec.Description, "application ports")
	require.ElementsMatch(t, []string{"applicationId", "publishedPort", "targetPort"}, r.RequiredInputs)
	require.Contains(t, r.Properties, "portId")
	require.NotContains(t, r.InputProperties, "portId")
	for _, key := range []string{"applicationId", "publishedPort", "targetPort", "protocol", "publishMode"} {
		require.NotEmpty(t, r.InputProperties[key].Description, key)
		require.False(t, r.InputProperties[key].Secret, key)
		require.False(t, r.Properties[key].Secret, key)
		require.Equal(t, key == "applicationId", r.InputProperties[key].ReplaceOnChanges, key)
	}
	for _, key := range []string{"publishedPort", "targetPort"} {
		require.Equal(t, "integer", r.InputProperties[key].Type)
		require.Equal(t, "integer", r.Properties[key].Type)
	}
	for key, tc := range map[string]struct {
		def    string
		values []any
	}{"protocol": {"tcp", []any{"tcp", "udp"}}, "publishMode": {"ingress", []any{"ingress", "host"}}} {
		require.Equal(t, tc.def, r.InputProperties[key].Default)
		typ := spec.Types[trimTypeRef(r.InputProperties[key].Ref)]
		require.NotEmpty(t, typ.Description)
		require.Contains(t, typ.Description, tc.values[0])
		require.Contains(t, typ.Description, tc.values[1])
		values := []any{}
		for _, v := range typ.Enum {
			values = append(values, v.Value)
			require.NotEmpty(t, v.Description)
		}
		require.ElementsMatch(t, tc.values, values)
	}
}

func TestSchemaPortSDKEnumDefaultNormalization(t *testing.T) {
	spec := providerSchema(t)
	r := spec.Resources["dokploy:index:Port"]
	for key, expected := range map[string]string{"protocol": "tcp", "publishMode": "ingress"} {
		require.Equal(t, expected, r.InputProperties[key].Default)
		require.NotContains(t, r.RequiredInputs, key)
	}
	// These assertions document generated SDK behavior rather than changing it:
	// Python None and Node.js null/undefined are treated as omission before Check.
	python := readGenerated(t, "sdk", "python", "pulumi_dokploy", "port.py")
	for _, fragment := range []string{"if protocol is None:\n            protocol = 'tcp'", "if publish_mode is None:\n            publish_mode = 'ingress'", "if protocol is not None:\n            pulumi.set(__self__, \"protocol\", protocol)", "if publish_mode is not None:\n            pulumi.set(__self__, \"publish_mode\", publish_mode)"} {
		require.Contains(t, python, fragment)
	}
	node := readGenerated(t, "sdk", "nodejs", "port.ts")
	require.Contains(t, node, `resourceInputs["protocol"] = (args?.protocol) ?? "tcp";`)
	require.Contains(t, node, `resourceInputs["publishMode"] = (args?.publishMode) ?? "ingress";`)
	require.NotContains(t, node, `(args?.protocol) || "tcp"`)
	require.NotContains(t, node, `(args?.publishMode) || "ingress"`)
	// Explicit empty strings remain values for the provider's Check to reject.
	for _, key := range []string{"protocol", "publishMode"} {
		props := map[string]property.Value{"applicationId": property.New("a1"), "publishedPort": property.New(float64(8080)), "targetPort": property.New(float64(80)), key: property.New("")}
		checked, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(props)})
		require.NoError(t, err)
		require.Contains(t, portFailureProperties(checked.Failures), key)
		props[key] = property.New(property.Null)
		checked, err = (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(props)})
		require.NoError(t, err)
		require.Contains(t, portFailureProperties(checked.Failures), key)
	}
}

func TestSchemaPublishingMetadata(t *testing.T) {
	spec := providerSchema(t)
	require.Equal(t, "github://api.github.com/dimeskigj/pulumi-dokploy", spec.PluginDownloadURL)
	require.ElementsMatch(t, []string{
		"category/infrastructure", "kind/native", "dokploy",
		"deployment", "self-hosted", "paas",
	}, spec.Keywords)
	require.Equal(t, registryLogoURL, spec.LogoURL)
}

func TestGeneratedPublishingMetadata(t *testing.T) {
	generatedSchema := readGenerated(t, "provider", "cmd", "pulumi-resource-dokploy", "schema.json")
	var schemaMetadata struct {
		PluginDownloadURL string   `json:"pluginDownloadURL"`
		Keywords          []string `json:"keywords"`
		LogoURL           string   `json:"logoUrl"`
	}
	require.NoError(t, json.Unmarshal([]byte(generatedSchema), &schemaMetadata))
	require.Equal(t, "github://api.github.com/dimeskigj/pulumi-dokploy", schemaMetadata.PluginDownloadURL)
	require.ElementsMatch(t, []string{
		"category/infrastructure", "kind/native", "dokploy",
		"deployment", "self-hosted", "paas",
	}, schemaMetadata.Keywords)
	require.Equal(t, registryLogoURL, schemaMetadata.LogoURL)

	for _, parts := range [][]string{
		{"sdk", "go", "dokploy", "pulumi-plugin.json"},
		{"sdk", "python", "pulumi_dokploy", "pulumi-plugin.json"},
		{"sdk", "dotnet", "pulumi-plugin.json"},
	} {
		var pluginMetadata map[string]any
		require.NoError(t, json.Unmarshal([]byte(readGenerated(t, parts...)), &pluginMetadata))
		require.Equal(t, true, pluginMetadata["resource"])
		require.Equal(t, "dokploy", pluginMetadata["name"])
		require.Equal(t, "github://api.github.com/dimeskigj/pulumi-dokploy", pluginMetadata["server"])
	}

	pythonUtilities := readGenerated(t, "sdk", "python", "pulumi_dokploy", "_utilities.py")
	require.Contains(t, pythonUtilities, "return \"github://api.github.com/dimeskigj/pulumi-dokploy\"")
	nodeUtilities := readGenerated(t, "sdk", "nodejs", "utilities.ts")
	require.Contains(t, nodeUtilities, "pluginDownloadURL: \"github://api.github.com/dimeskigj/pulumi-dokploy\"")
	goUtilities := readGenerated(t, "sdk", "go", "dokploy", "internal", "pulumiUtilities.go")
	require.Contains(t, goUtilities, "pulumi.PluginDownloadURL(\"github://api.github.com/dimeskigj/pulumi-dokploy\")")
	dotnetUtilities := readGenerated(t, "sdk", "dotnet", "Utilities.cs")
	require.Contains(t, dotnetUtilities, "PluginDownloadURL ?? \"github://api.github.com/dimeskigj/pulumi-dokploy\"")
	javaProvider := readGenerated(t, "sdk", "java", "src", "main", "java", "net", "dimeski", "pulumi", "dokploy", "Provider.java")
	require.Contains(t, javaProvider, ".pluginDownloadURL(\"github://api.github.com/dimeskigj/pulumi-dokploy\")")
}

func TestCommittedSchemaDoesNotClaimDevelopmentVersion(t *testing.T) {
	var committed map[string]json.RawMessage
	command := exec.Command("git", "show", ":provider/cmd/pulumi-resource-dokploy/schema.json")
	command.Dir = ".."
	data, err := command.Output()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &committed))
	require.NotContains(t, committed, "version")
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
	for _, property := range []string{"type", "applicationId", "composeId", "postgresId", "mysqlId", "mariadbId", "redisId"} {
		require.True(t, spec.Resources["dokploy:index:Mount"].InputProperties[property].ReplaceOnChanges, property)
	}
	applicationProps := spec.Resources["dokploy:index:Application"].InputProperties
	require.Contains(t, applicationProps, "registryId")
	require.Contains(t, applicationProps, "buildRegistryId")
	gitProps := spec.Types["dokploy:index:GitApplicationSource"].Properties
	require.Contains(t, gitProps, "sshKeyId")
}

func TestGeneratedDotnetAndJavaPackagesDoNotDuplicateProviderSuffix(t *testing.T) {
	dotnetProject := readGenerated(t, "sdk", "dotnet", "Dimeskigj.Pulumi.Dokploy.csproj")
	require.NotContains(t, dotnetProject, "Dimeskigj.Pulumi.Dokploy.Dokploy")
	dotnetProvider := readGenerated(t, "sdk", "dotnet", "Dokploy", "Provider.cs")
	require.Contains(t, dotnetProvider, "namespace Dimeskigj.Pulumi.Dokploy\n")
	require.NotContains(t, dotnetProvider, "namespace Dimeskigj.Pulumi.Dokploy.Dokploy")

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

func functionTokens(functions map[string]schema.FunctionSpec) []string {
	result := make([]string, 0, len(functions))
	for token := range functions {
		result = append(result, token)
	}
	return result
}

func objectKeys(properties map[string]schema.PropertySpec) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	return keys
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
