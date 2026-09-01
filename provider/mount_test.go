package dokploy

import (
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestMountCheckRequiresValidTypeAndTarget(t *testing.T) {
	checked, err := (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("bind"), "mountPath": property.New("/data"), "hostPath": property.New("/host"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.Equal(t, "bind", checked.Inputs.Type)
	checked, err = (Mount{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"type": property.New("unknown"), "mountPath": property.New("/data"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	require.NotEmpty(t, checked.Failures)
}
