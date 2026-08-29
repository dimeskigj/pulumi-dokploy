package dokploy

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

var (
	_ infer.ExplicitDependencies[ProjectArgs, ProjectState]         = Project{}
	_ infer.ExplicitDependencies[EnvironmentArgs, EnvironmentState] = Environment{}
	_ infer.ExplicitDependencies[ApplicationArgs, ApplicationState] = Application{}
	_ infer.ExplicitDependencies[PostgresArgs, PostgresState]       = Postgres{}
	_ infer.ExplicitDependencies[RedisArgs, RedisState]             = Redis{}
)

func TestProviderSchema(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.NotEmpty(t, spec.Config)
	require.Equal(t, "dokploy", Name)
	require.True(t, spec.Config.Variables["apiKey"].Secret)
	require.Equal(t, []string{"DOKPLOY_ENDPOINT"}, spec.Config.Variables["endpoint"].DefaultInfo.Environment)
	require.Equal(t, []string{"DOKPLOY_API_KEY"}, spec.Config.Variables["apiKey"].DefaultInfo.Environment)
}

func TestProviderRegistersProjectAndEnvironmentResources(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	var tokens []string
	for token := range spec.Resources {
		tokens = append(tokens, token)
	}
	require.Contains(t, tokens, "dokploy:index:Project")
	require.Contains(t, tokens, "dokploy:index:Environment")
}

func TestProviderHasNoSampleFunctionsOrComponents(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Empty(t, spec.Functions)
	for token, resource := range spec.Resources {
		require.False(t, resource.IsComponent, token)
	}
}
