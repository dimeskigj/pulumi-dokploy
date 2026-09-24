package dokploy

import (
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestProjectPreviewIDIsComputedOnCreateAndKnownAfterDescriptionUpdate(t *testing.T) {
	provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("Project", "preview-project")
	created, err := provider.Create(p.CreateRequest{Urn: urn, DryRun: true, Properties: property.NewMap(map[string]property.Value{
		"name": property.New("project"), "description": property.New("before"),
	})})
	require.NoError(t, err)
	require.True(t, created.Properties.Get("projectId").IsComputed())

	updated, err := provider.Update(p.UpdateRequest{
		ID: "stable-project-id", Urn: urn, State: property.NewMap(map[string]property.Value{
			"name": property.New("project"), "description": property.New("before"),
			"projectId": property.New("stable-project-id"), "defaultEnvironmentId": property.New("stable-environment-id"),
		}), OldInputs: property.NewMap(map[string]property.Value{
			"name": property.New("project"), "description": property.New("before"),
		}), Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("project"), "description": property.New("after"),
		}), DryRun: true,
	})
	require.NoError(t, err)
	require.False(t, updated.Properties.Get("projectId").IsComputed())
	require.Equal(t, "stable-project-id", updated.Properties.Get("projectId").AsString())
}

func TestTagPreviewIDIsComputedOnCreateAndKnownAfterColorUpdate(t *testing.T) {
	provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("Tag", "preview-tag")
	created, err := provider.Create(p.CreateRequest{Urn: urn, DryRun: true, Properties: property.NewMap(map[string]property.Value{
		"name": property.New("tag"), "color": property.New("before"),
	})})
	require.NoError(t, err)
	require.True(t, created.Properties.Get("tagId").IsComputed())

	updated, err := provider.Update(p.UpdateRequest{
		ID: "stable-tag-id", Urn: urn, State: property.NewMap(map[string]property.Value{
			"name": property.New("tag"), "color": property.New("before"), "tagId": property.New("stable-tag-id"),
		}), OldInputs: property.NewMap(map[string]property.Value{
			"name": property.New("tag"), "color": property.New("before"),
		}), Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("tag"), "color": property.New("after"),
		}), DryRun: true,
	})
	require.NoError(t, err)
	require.False(t, updated.Properties.Get("tagId").IsComputed())
	require.Equal(t, "stable-tag-id", updated.Properties.Get("tagId").AsString())
}

func TestEnvironmentDiffReplacesChangedProjectID(t *testing.T) {
	diff, err := (Environment{}).Diff(t.Context(), infer.DiffRequest[EnvironmentArgs, EnvironmentState]{
		Inputs: EnvironmentArgs{ProjectID: "new-project", Name: "staging"},
		State:  EnvironmentState{EnvironmentArgs: EnvironmentArgs{ProjectID: "old-project", Name: "staging"}},
	})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
}
