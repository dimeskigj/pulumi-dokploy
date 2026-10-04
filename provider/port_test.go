package dokploy

import (
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPortCheckDefaults(t *testing.T) {
	checked, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"applicationId": property.New("a1"), "publishedPort": property.New(float64(8081)), "targetPort": property.New(float64(80))})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
	require.Equal(t, PortProtocolTCP, checked.Inputs.Protocol)
	require.Equal(t, PortPublishModeIngress, checked.Inputs.PublishMode)
}
func TestPortCheckRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input property.Map
		field string
	}{
		{"app missing", property.NewMap(map[string]property.Value{}), "applicationId"},
		{"app null", property.NewMap(map[string]property.Value{"applicationId": property.New(property.Null)}), "applicationId"},
		{"app empty", property.NewMap(map[string]property.Value{"applicationId": property.New(" ")}), "applicationId"},
		{"fractional", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(80.5), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"out of range", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(65536)), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"zero", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(0))}), "targetPort"},
		{"enum invalid", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(80)), "protocol": property.New("sctp")}), "protocol"},
		{"enum empty", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(80)), "protocol": property.New("")}), "protocol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: tc.input})
			require.NoError(t, err)
			require.Contains(t, portFailureProperties(got.Failures), tc.field)
		})
	}
}
func portFailureProperties(fs []p.CheckFailure) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Property)
	}
	return out
}
func TestPortCheckDefersOnlyComputedFields(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"applicationId": property.New(property.Computed), "publishedPort": property.New(property.Computed), "targetPort": property.New(float64(0)), "protocol": property.New("bogus")})
	got, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Contains(t, portFailureProperties(got.Failures), "targetPort")
	require.Contains(t, portFailureProperties(got.Failures), "protocol")
	require.NotContains(t, portFailureProperties(got.Failures), "applicationId")
	require.NotContains(t, portFailureProperties(got.Failures), "publishedPort")
}
func TestPortDiff(t *testing.T) {
	old := PortArgs{ApplicationID: "a", PublishedPort: 80, TargetPort: 80, Protocol: PortProtocolTCP, PublishMode: PortPublishModeIngress}
	next := old
	next.PublishedPort = 81
	d, err := (Port{}).Diff(t.Context(), infer.DiffRequest[PortArgs, PortState]{Inputs: next, State: PortState{PortArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.Update, d.DetailedDiff["publishedPort"].Kind)
	next.ApplicationID = "b"
	d, err = (Port{}).Diff(t.Context(), infer.DiffRequest[PortArgs, PortState]{Inputs: next, State: PortState{PortArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, d.DetailedDiff["applicationId"].Kind)
	require.False(t, d.DeleteBeforeReplace)
}
func TestValidatePortArgs(t *testing.T) {
	a := PortArgs{ApplicationID: "opaque", PublishedPort: 65535, TargetPort: 1, Protocol: PortProtocolUDP, PublishMode: PortPublishModeHost}
	require.NoError(t, validatePortArgs(a))
	a.TargetPort = 0
	require.Error(t, validatePortArgs(a))
}
