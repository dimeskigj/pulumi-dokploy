package dokploy

import (
	"context"
	"errors"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// The fixture's authenticated client is deliberately reused: it does not expose
// credentials for configuring another provider. infer.Function exercises the
// same typed invoke and property encoding as the registered provider functions.
// Offline tests additionally cover registration through Provider().
func runLiveLookup(t *testing.T, ctx context.Context, api *client.Client, family, id, expectedName string) {
	t.Helper()
	contract, ok := liveLookupContracts[family]
	if !ok || api == nil || id == "" {
		t.Fatal("live lookup fixture failed")
	}
	response, err := liveLookupFunction(api, family).Invoke(ctx, p.InvokeRequest{
		Token: tokens.Type("dokploy:index:get" + family),
		Args:  property.NewMap(map[string]property.Value{contract.id: property.New(id)}),
	})
	requireLiveLifecycleNoError(t, family, "lookup", err)
	if len(response.Failures) != 0 {
		t.Fatal("live lookup input failed")
	}
	requireLiveLifecycleNoError(t, family, "lookup contract", validateLiveLookup(family, response.Return, id, expectedName))
}

type liveLookupContract struct {
	id       string
	metadata map[string]string
}

func liveLookupMetadata(strings, numbers, booleans []string) map[string]string {
	fields := map[string]string{}
	for _, key := range strings {
		fields[key] = "string"
	}
	for _, key := range numbers {
		fields[key] = "number"
	}
	for _, key := range booleans {
		fields[key] = "bool"
	}
	return fields
}

var liveLookupContracts = map[string]liveLookupContract{
	"Project":     {"projectId", liveLookupMetadata([]string{"description", "defaultEnvironmentId"}, nil, nil)},
	"Environment": {"environmentId", liveLookupMetadata([]string{"description", "projectId"}, nil, []string{"isDefault"})},
	"Application": {"applicationId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "registryId", "buildRegistryId"}, nil, nil)},
	"Compose":     {"composeId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "composeType"}, nil, nil)},
	"Postgres":    {"postgresId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "dockerImage", "databaseName", "databaseUser"}, []string{"externalPort"}, nil)},
	"MySQL":       {"mysqlId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "dockerImage", "databaseName", "databaseUser"}, []string{"externalPort"}, nil)},
	"MariaDB":     {"mariadbId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "dockerImage", "databaseName", "databaseUser"}, []string{"externalPort"}, nil)},
	"MongoDB":     {"mongoId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "dockerImage", "databaseUser"}, []string{"externalPort"}, []string{"replicaSets"})},
	"Redis":       {"redisId", liveLookupMetadata([]string{"description", "appName", "environmentId", "serverId", "status", "dockerImage"}, []string{"externalPort"}, nil)},
	"Server":      {"serverId", liveLookupMetadata([]string{"description", "ipAddress", "username", "organizationId", "sshKeyId", "serverType", "status"}, []string{"port"}, nil)},
	"Registry":    {"registryId", liveLookupMetadata([]string{"url", "username", "imagePrefix", "serverId", "registryType"}, nil, nil)},
	"SSHKey":      {"sshKeyId", liveLookupMetadata([]string{"description", "publicKey", "organizationId"}, nil, nil)},
}

func validateLiveLookup(family string, output property.Map, id, expectedName string) error {
	contract, ok := liveLookupContracts[family]
	if !ok {
		return errors.New("lookup family is unsupported")
	}
	for key, want := range map[string]string{contract.id: id, "name": expectedName} {
		value, ok := output.GetOk(key)
		if !ok || !value.IsString() || value.Secret() || value.AsString() != want {
			return errors.New("lookup owned identity or name mismatch")
		}
	}
	for key, value := range output.All {
		if key == contract.id || key == "name" {
			continue
		}
		typeName, allowed := contract.metadata[key]
		if !allowed || value.Secret() ||
			(typeName == "string" && !value.IsString()) ||
			(typeName == "number" && !value.IsNumber()) ||
			(typeName == "bool" && !value.IsBool()) {
			return errors.New("lookup contains excluded or invalid metadata")
		}
	}
	return nil
}

func liveLookupFunction(api *client.Client, family string) infer.InferredFunction {
	factory := fixedClient(api)
	switch family {
	case "Project":
		return infer.Function(&GetProject{client: factory})
	case "Environment":
		return infer.Function(&GetEnvironment{client: factory})
	case "Application":
		return infer.Function(&GetApplication{client: factory})
	case "Compose":
		return infer.Function(&GetCompose{client: factory})
	case "Postgres":
		return infer.Function(&GetPostgres{client: factory})
	case "MySQL":
		return infer.Function(&GetMySQL{client: factory})
	case "MariaDB":
		return infer.Function(&GetMariaDB{client: factory})
	case "MongoDB":
		return infer.Function(&GetMongoDB{client: factory})
	case "Redis":
		return infer.Function(&GetRedis{client: factory})
	case "Server":
		return infer.Function(&GetServer{client: factory})
	case "Registry":
		return infer.Function(&GetRegistry{client: factory})
	case "SSHKey":
		return infer.Function(&GetSSHKey{client: factory})
	default:
		panic("unsupported lookup family")
	}
}
