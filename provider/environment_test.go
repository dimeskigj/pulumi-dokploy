package dokploy

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentCheckRejectsProduction(t *testing.T) {
	r := Environment{}
	checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"projectId": property.New("p1"), "name": property.New("production"),
	})})
	require.NoError(t, err)
	require.Len(t, checked.Failures, 1)
	require.Contains(t, checked.Failures[0].Reason, "production")
}

func TestEnvironmentProjectIDReplacement(t *testing.T) {
	r := Environment{}
	diff, err := r.Diff(t.Context(), infer.DiffRequest[EnvironmentArgs, EnvironmentState]{
		State: inferStateEnvironment("p1", "staging"), Inputs: EnvironmentArgs{ProjectID: "p2", Name: "staging"},
	})
	require.NoError(t, err)
	require.True(t, diff.HasChanges)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
}

func inferStateEnvironment(projectID, name string) EnvironmentState {
	return EnvironmentState{EnvironmentArgs: EnvironmentArgs{ProjectID: projectID, Name: name}}
}
