package dokploy

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
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
		{"app missing", portInputs(nil), "applicationId"},
		{"app null", portInputs(map[string]property.Value{"applicationId": property.New(property.Null)}), "applicationId"},
		{"app empty", portInputs(map[string]property.Value{"applicationId": property.New("")}), "applicationId"},
		{"app whitespace", portInputs(map[string]property.Value{"applicationId": property.New(" \t ")}), "applicationId"},
		{"published missing", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"published null", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(property.Null), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"published zero", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(0)), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"published negative", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(-1)), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"published too high", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(65536)), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"published fractional", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(80.5), "targetPort": property.New(float64(80))}), "publishedPort"},
		{"target missing", property.NewMap(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80))}), "targetPort"},
		{"target null", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(property.Null)}), "targetPort"},
		{"target zero", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(0))}), "targetPort"},
		{"target negative", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(-1))}), "targetPort"},
		{"target too high", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(float64(65536))}), "targetPort"},
		{"target fractional", portInputs(map[string]property.Value{"applicationId": property.New("a"), "publishedPort": property.New(float64(80)), "targetPort": property.New(80.5)}), "targetPort"},
		{"protocol null", portInputs(map[string]property.Value{"protocol": property.New(property.Null)}), "protocol"},
		{"protocol empty", portInputs(map[string]property.Value{"protocol": property.New("")}), "protocol"},
		{"protocol invalid", portInputs(map[string]property.Value{"protocol": property.New("sctp")}), "protocol"},
		{"publish mode null", portInputs(map[string]property.Value{"publishMode": property.New(property.Null)}), "publishMode"},
		{"publish mode empty", portInputs(map[string]property.Value{"publishMode": property.New("")}), "publishMode"},
		{"publish mode invalid", portInputs(map[string]property.Value{"publishMode": property.New("direct")}), "publishMode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: tc.input})
			require.NoError(t, err)
			require.Contains(t, portFailureProperties(got.Failures), tc.field)
		})
	}
}
func portInputs(overrides map[string]property.Value) property.Map {
	values := map[string]property.Value{"applicationId": property.New("app"), "publishedPort": property.New(float64(8081)), "targetPort": property.New(float64(80))}
	for k, v := range overrides {
		values[k] = v
	}
	if overrides == nil {
		delete(values, "applicationId")
		delete(values, "publishedPort")
		delete(values, "targetPort")
	}
	return property.NewMap(values)
}
func TestPortCheckAcceptsPortBoundariesEnumsAndOpaqueID(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, mode string
		port                 float64
	}{
		{"lower tcp ingress", "tcp", "ingress", 1}, {"upper udp host", "udp", "host", 65535},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
				"applicationId": property.New("  opaque id  "), "publishedPort": property.New(tc.port), "targetPort": property.New(tc.port), "protocol": property.New(tc.protocol), "publishMode": property.New(tc.mode),
			})})
			require.NoError(t, err)
			require.Empty(t, got.Failures)
			require.Equal(t, "  opaque id  ", got.Inputs.ApplicationID)
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
	require.Contains(t, d.DetailedDiff, "publishedPort")
	replaceOnly := old
	replaceOnly.ApplicationID = "new"
	replaceDiff, err := (Port{}).Diff(t.Context(), infer.DiffRequest[PortArgs, PortState]{Inputs: replaceOnly, State: PortState{PortArgs: old}})
	require.NoError(t, err)
	require.Equal(t, map[string]p.PropertyDiff{"applicationId": {Kind: p.UpdateReplace}}, replaceDiff.DetailedDiff)
	for name, change := range map[string]func(*PortArgs){"publishedPort": func(a *PortArgs) { a.PublishedPort++ }, "targetPort": func(a *PortArgs) { a.TargetPort++ }, "protocol": func(a *PortArgs) { a.Protocol = PortProtocolUDP }, "publishMode": func(a *PortArgs) { a.PublishMode = PortPublishModeHost }} {
		t.Run(name, func(t *testing.T) {
			changed := old
			change(&changed)
			got, err := (Port{}).Diff(t.Context(), infer.DiffRequest[PortArgs, PortState]{Inputs: changed, State: PortState{PortArgs: old}})
			require.NoError(t, err)
			require.Equal(t, p.Update, got.DetailedDiff[name].Kind)
			require.False(t, got.DeleteBeforeReplace)
		})
	}
	unchanged, err := (Port{}).Diff(t.Context(), infer.DiffRequest[PortArgs, PortState]{Inputs: old, State: PortState{PortArgs: old}})
	require.NoError(t, err)
	require.False(t, unchanged.HasChanges)
	require.Empty(t, unchanged.DetailedDiff)
}
func TestValidatePortArgs(t *testing.T) {
	a := PortArgs{ApplicationID: "opaque", PublishedPort: 65535, TargetPort: 1, Protocol: PortProtocolUDP, PublishMode: PortPublishModeHost}
	require.NoError(t, validatePortArgs(a))
	a.TargetPort = 0
	require.Error(t, validatePortArgs(a))
	for _, mutate := range []func(*PortArgs){func(v *PortArgs) { v.ApplicationID = "" }, func(v *PortArgs) { v.PublishedPort = 0 }, func(v *PortArgs) { v.PublishedPort = -1 }, func(v *PortArgs) { v.PublishedPort = 65536 }, func(v *PortArgs) { v.TargetPort = 0 }, func(v *PortArgs) { v.TargetPort = -1 }, func(v *PortArgs) { v.TargetPort = 65536 }, func(v *PortArgs) { v.Protocol = "" }, func(v *PortArgs) { v.Protocol = "sctp" }, func(v *PortArgs) { v.PublishMode = "" }, func(v *PortArgs) { v.PublishMode = "direct" }} {
		bad := PortArgs{ApplicationID: "x", PublishedPort: 1, TargetPort: 65535, Protocol: PortProtocolTCP, PublishMode: PortPublishModeIngress}
		mutate(&bad)
		require.Error(t, validatePortArgs(bad))
	}
}
