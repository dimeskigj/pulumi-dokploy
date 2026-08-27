package provider

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/stretchr/testify/require"
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
