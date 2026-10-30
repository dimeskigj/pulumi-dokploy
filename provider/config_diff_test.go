package dokploy

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	rpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

const configDiffURN = "urn:pulumi:stack::project::pulumi:providers:dokploy::default"

func configDiffState(version, endpoint, apiKey string) resource.PropertyMap {
	return resource.PropertyMap{
		"__internal": resource.NewObjectProperty(resource.PropertyMap{
			"pluginDownloadURL": resource.NewStringProperty("github://api.github.com/dimeskigj/pulumi-dokploy"),
		}),
		"__pulumi-go-provider-infer":   resource.NewBoolProperty(true),
		"__pulumi-go-provider-version": resource.NewStringProperty("v1.6.0"),
		"endpoint":                     resource.NewStringProperty(endpoint),
		"apiKey":                       resource.MakeSecret(resource.NewStringProperty(apiKey)),
		"version":                      resource.NewStringProperty(version),
	}
}

func marshalConfigDiffState(t *testing.T, m resource.PropertyMap) *structpb.Struct {
	t.Helper()
	s, err := plugin.MarshalProperties(m, plugin.MarshalOptions{KeepSecrets: true})
	require.NoError(t, err)
	return s
}

// The engine calls the new provider's DiffConfig when a resource moves to a
// different default provider (for example after a plugin version bump). At that
// point the provider is configured and receives old inputs, including the
// framework's internal keys.
func diffConfig(t *testing.T, olds, news resource.PropertyMap) *rpc.DiffResponse {
	t.Helper()
	server, err := p.RawServer(Name, "0.4.2", Provider())(nil)
	require.NoError(t, err)
	_, err = server.Configure(t.Context(), &rpc.ConfigureRequest{
		Args:                   marshalConfigDiffState(t, news),
		SendsOldInputs:         true,
		SendsOldInputsToDelete: true,
	})
	require.NoError(t, err)
	resp, err := server.DiffConfig(t.Context(), &rpc.DiffRequest{
		Urn:       configDiffURN,
		Olds:      marshalConfigDiffState(t, olds),
		OldInputs: marshalConfigDiffState(t, olds),
		News:      marshalConfigDiffState(t, news),
	})
	require.NoError(t, err)
	return resp
}

func TestConfigDiffVersionBumpDoesNotReplace(t *testing.T) {
	resp := diffConfig(t,
		configDiffState("0.4.0", "https://dokploy.example", "token"),
		configDiffState("0.4.2", "https://dokploy.example", "token"))
	require.Empty(t, resp.GetReplaces())
}

func TestConfigDiffUnchanged(t *testing.T) {
	resp := diffConfig(t,
		configDiffState("0.4.2", "https://dokploy.example", "token"),
		configDiffState("0.4.2", "https://dokploy.example", "token"))
	require.Empty(t, resp.GetReplaces())
	require.Equal(t, rpc.DiffResponse_DIFF_NONE, resp.GetChanges())
}

func TestConfigDiffCredentialChangesUpdateInPlace(t *testing.T) {
	resp := diffConfig(t,
		configDiffState("0.4.2", "https://dokploy.example", "old-token"),
		configDiffState("0.4.2", "https://dokploy.example/", "new-token"))
	require.Empty(t, resp.GetReplaces())
	require.Equal(t, rpc.DiffResponse_DIFF_SOME, resp.GetChanges())
	require.ElementsMatch(t, []string{"endpoint", "apiKey"}, resp.GetDiffs())
}
