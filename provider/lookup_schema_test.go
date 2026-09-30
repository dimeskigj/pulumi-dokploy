package dokploy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func databaseRequest[T any](input T) infer.FunctionRequest[T] {
	return infer.FunctionRequest[T]{Input: input}
}

func testDatabaseLookup(t *testing.T, operation, field string, invoke func(clientFactory, string) (any, error)) {
	t.Helper()
	for _, tc := range []struct {
		name, body, category string
		status               int
		expected             map[string]any
	}{
		{"complete", fmt.Sprintf(`{"%s":"id","name":"display","description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"canonical","image":"legacy","applicationStatus":"future","externalPort":123,"databaseName":"db","databaseUser":"user","replicaSets":true,"databasePassword":"mock-secret","env":"mock-secret","source":{"token":"mock-secret"}}`, field), "", 200, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "canonical", "status": "future", "externalPort": 123, "databaseName": "db", "databaseUser": "user", "replicaSets": true}},
		{"empty and zero", fmt.Sprintf(`{"%s":"id","name":"","description":"","appName":"","environmentId":"","serverId":"","dockerImage":"","image":"legacy","applicationStatus":"","externalPort":0,"databaseName":"","databaseUser":"","replicaSets":false}`, field), "", 200, map[string]any{"description": "", "appName": "", "environmentId": "", "serverId": "", "dockerImage": "", "status": "", "externalPort": 0, "databaseName": "", "databaseUser": "", "replicaSets": false}},
		{"legacy absent", fmt.Sprintf(`{"%s":"id","name":"n","image":"legacy"}`, field), "", 200, map[string]any{"dockerImage": "legacy"}},
		{"legacy null", fmt.Sprintf(`{"%s":"id","name":"n","dockerImage":null,"image":"legacy"}`, field), "", 200, map[string]any{"dockerImage": "legacy"}},
		{"minimal", fmt.Sprintf(`{"%s":"id","name":"n"}`, field), "", 200, nil},
		{"null optionals", fmt.Sprintf(`{"%s":"id","name":"n","description":null,"appName":null,"dockerImage":null,"image":null,"applicationStatus":null,"externalPort":null,"replicaSets":null}`, field), "", 200, nil},
		{"null object", `null`, "response contract", 200, nil},
		{"missing id", `{"name":"n"}`, "response contract", 200, nil},
		{"empty id", fmt.Sprintf(`{"%s":"","name":"n"}`, field), "response contract", 200, nil},
		{"mismatched id", fmt.Sprintf(`{"%s":"other","name":"n"}`, field), "response contract", 200, nil},
		{"missing name", fmt.Sprintf(`{"%s":"id"}`, field), "response contract", 200, nil},
		{"null name", fmt.Sprintf(`{"%s":"id","name":null}`, field), "response contract", 200, nil},
		{"wrong name", fmt.Sprintf(`{"%s":"id","name":false}`, field), "response contract", 200, nil},
		{"wrong id", fmt.Sprintf(`{"%s":17,"name":"n"}`, field), "response contract", 200, nil},
		{"wrong description", fmt.Sprintf(`{"%s":"id","name":"n","description":{"password":"mock-secret"}}`, field), "response contract", 200, nil},
		{"wrong legacy image", fmt.Sprintf(`{"%s":"id","name":"n","image":42}`, field), "response contract", 200, nil},
		{"wrong username", fmt.Sprintf(`{"%s":"id","name":"n","databaseUser":true}`, field), "response contract", 200, nil},
		{"wrong status", fmt.Sprintf(`{"%s":"id","name":"n","applicationStatus":false}`, field), "response contract", 200, nil},
		{"wrong port", fmt.Sprintf(`{"%s":"id","name":"n","externalPort":"bad"}`, field), "response contract", 200, nil},
		{"wrong image", fmt.Sprintf(`{"%s":"id","name":"n","dockerImage":false}`, field), "response contract", 200, nil},
		{"wrong replica", fmt.Sprintf(`{"%s":"id","name":"n","replicaSets":"bad"}`, field), "response contract", 200, nil},
		{"invalid json", `{`, "response contract", 200, nil},
		{"unauthorized", `{"message":"mock-secret"}`, "authentication", 401, nil},
		{"forbidden", `{"message":"mock-secret"}`, "authorization", 403, nil},
		{"not found", `{"message":"mock-secret"}`, "not found", 404, nil},
		{"API not found", `{"code":"NOT_FOUND","message":"mock-secret"}`, "not found", 400, nil},
	} {
		if operation != "mongo" && tc.name == "wrong replica" || operation == "redis" && tc.name == "wrong username" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + operation + ".one", Query: url.Values{field: {"id"}}, Status: tc.status, Response: []byte(tc.body)})
			result, err := invoke(fixedClient(s.API()), "id")
			if tc.category != "" {
				require.ErrorContains(t, err, operation+".one")
				require.ErrorContains(t, err, tc.category)
				require.NotContains(t, err.Error(), "mock-secret")
				require.NotContains(t, err.Error(), "other")
				return
			}
			require.NoError(t, err)
			output := reflect.ValueOf(result).FieldByName("Output")
			require.Equal(t, "id", output.FieldByName(databaseIDField(field)).String())
			name := "n"
			if tc.name == "complete" {
				name = "display"
			}
			if tc.name == "empty and zero" {
				name = ""
			}
			require.Equal(t, name, output.FieldByName("Name").String())
			for jsonField, expected := range tc.expected {
				if operation != "mongo" && jsonField == "replicaSets" {
					continue
				}
				if operation == "redis" && (jsonField == "databaseName" || jsonField == "databaseUser") {
					continue
				}
				if operation == "mongo" && jsonField == "databaseName" {
					continue
				}
				actual := output.FieldByName(databaseIDField(jsonField))
				require.True(t, actual.IsValid(), jsonField)
				require.False(t, actual.IsNil(), jsonField)
				require.Equal(t, expected, actual.Elem().Interface(), jsonField)
			}
			if tc.expected == nil {
				for i := 0; i < output.NumField(); i++ {
					value := output.Field(i)
					if value.Kind() == reflect.Pointer {
						require.True(t, value.IsNil(), output.Type().Field(i).Name)
					}
				}
			}
		})
	}
	for _, id := range []string{"", " \t"} {
		called := false
		_, err := invoke(func(context.Context) *client.Client { called = true; return nil }, id)
		require.ErrorContains(t, err, field)
		require.False(t, called)
	}
}

func databaseIDField(field string) string {
	switch field {
	case "postgresId":
		return "PostgresID"
	case "mysqlId":
		return "MySQLID"
	case "mariadbId":
		return "MariaDBID"
	case "mongoId":
		return "MongoID"
	case "redisId":
		return "RedisID"
	case "appName":
		return "AppName"
	case "environmentId":
		return "EnvironmentID"
	case "serverId":
		return "ServerID"
	case "dockerImage":
		return "DockerImage"
	case "externalPort":
		return "ExternalPort"
	case "databaseName":
		return "DatabaseName"
	case "databaseUser":
		return "DatabaseUser"
	case "replicaSets":
		return "ReplicaSets"
	default:
		return strings.ToUpper(field[:1]) + field[1:]
	}
}

func TestLookupSchemaContract(t *testing.T) {
	spec := providerSchema(t)
	commonDB := []string{"description", "appName", "environmentId", "serverId", "dockerImage", "status", "externalPort"}
	contracts := []struct {
		token, id string
		optional  []string
	}{
		{"getProject", "projectId", []string{"description", "defaultEnvironmentId"}},
		{"getEnvironment", "environmentId", []string{"description", "projectId", "isDefault"}},
		{"getApplication", "applicationId", []string{"description", "appName", "environmentId", "serverId", "status", "registryId", "buildRegistryId"}},
		{"getCompose", "composeId", []string{"description", "appName", "environmentId", "serverId", "status", "composeType"}},
		{"getPostgres", "postgresId", append(append([]string{}, commonDB...), "databaseName", "databaseUser")},
		{"getMySQL", "mysqlId", append(append([]string{}, commonDB...), "databaseName", "databaseUser")},
		{"getMariaDB", "mariadbId", append(append([]string{}, commonDB...), "databaseName", "databaseUser")},
		{"getMongoDB", "mongoId", append(append([]string{}, commonDB...), "databaseUser", "replicaSets")},
		{"getRedis", "redisId", commonDB},
		{"getServer", "serverId", []string{"description", "ipAddress", "port", "username", "organizationId", "sshKeyId", "serverType", "status"}},
		{"getRegistry", "registryId", []string{"url", "username", "imagePrefix", "serverId", "registryType"}},
		{"getSSHKey", "sshKeyId", []string{"description", "publicKey", "organizationId"}},
	}
	var tokens []string
	for _, contract := range contracts {
		token := "dokploy:index:" + contract.token
		tokens = append(tokens, token)
		fn, ok := spec.Functions[token]
		require.True(t, ok, token)
		require.NotEmpty(t, fn.Description)
		require.Equal(t, []string{contract.id}, fn.Inputs.Required)
		require.Equal(t, []string{contract.id}, objectKeys(fn.Inputs.Properties))
		input := fn.Inputs.Properties[contract.id]
		require.Equal(t, "string", input.Type)
		require.NotEmpty(t, input.Description)
		require.Nil(t, input.Default)
		output := fn.ReturnType.ObjectTypeSpec
		require.NotNil(t, output)
		require.ElementsMatch(t, []string{contract.id, "name"}, output.Required)
		require.ElementsMatch(t, append([]string{contract.id, "name"}, contract.optional...), objectKeys(output.Properties))
		for field, property := range output.Properties {
			typeName := "string"
			if field == "isDefault" || field == "replicaSets" {
				typeName = "boolean"
			}
			if field == "port" || field == "externalPort" {
				typeName = "integer"
			}
			require.Equal(t, typeName, property.Type, token+"."+field)
			require.NotEmpty(t, property.Description, token+"."+field)
			require.Nil(t, property.Default)
			require.False(t, property.Secret)
		}
	}
	require.ElementsMatch(t, tokens, functionTokens(spec.Functions))
	require.ElementsMatch(t, []string{
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application", "dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:MySQL", "dokploy:index:MariaDB", "dokploy:index:MongoDB", "dokploy:index:Redis", "dokploy:index:Domain", "dokploy:index:Destination", "dokploy:index:Backup", "dokploy:index:VolumeBackup", "dokploy:index:SSHKey", "dokploy:index:Registry", "dokploy:index:Tag", "dokploy:index:ProjectTag", "dokploy:index:Mount",
	}, resourceTokens(spec.Resources))
	for token, resource := range spec.Resources {
		require.False(t, resource.IsComponent, token)
	}
}
