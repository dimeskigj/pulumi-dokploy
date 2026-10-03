package dokploy

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"
)

func TestLookupGeneratedSchema(t *testing.T) {
	data, err := os.ReadFile("cmd/pulumi-resource-dokploy/schema.json")
	require.NoError(t, err)
	var committed schema.PackageSpec
	require.NoError(t, json.Unmarshal(data, &committed))
	source := providerSchema(t)
	require.Len(t, committed.Functions, 12)
	require.ElementsMatch(t, []string{"projectId"}, committed.Functions["dokploy:index:getProject"].Inputs.Required)
	require.ElementsMatch(t, []string{"projectId", "name"}, committed.Functions["dokploy:index:getProject"].ReturnType.ObjectTypeSpec.Required)
	require.NotContains(t, committed.Functions, "dokploy:index:getDatabase")
	require.Equal(t, source.Functions, committed.Functions, "generated function fields, optionality, types and descriptions must match source")
	require.Equal(t, source.PluginDownloadURL, committed.PluginDownloadURL)
	require.Equal(t, source.LogoURL, committed.LogoURL)
	require.ElementsMatch(t, source.Keywords, committed.Keywords)
	for _, language := range []string{"go", "nodejs", "python", "csharp", "java"} {
		require.JSONEq(t, string(source.Language[language]), string(committed.Language[language]), language)
	}
}
