package dokploy

import (
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestPortPreviewIdentity(t *testing.T) {
	r := Port{client: func(context.Context) *client.Client { t.Fatal("preview accessed client"); return nil }}
	a := portArgs()
	created, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: a, DryRun: true})
	require.NoError(t, err)
	require.Empty(t, created.ID)
	updated, err := r.Update(t.Context(), infer.UpdateRequest[PortArgs, PortState]{ID: "p1", Inputs: a, State: PortState{PortArgs: a, PortID: "p1"}, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "p1", updated.Output.PortID)
	provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	urn := lifecycleURN("Port", "preview-port")
	inputs := property.NewMap(map[string]property.Value{"applicationId": property.New("a1"), "publishedPort": property.New(float64(8080)), "targetPort": property.New(float64(80)), "protocol": property.New("tcp"), "publishMode": property.New("ingress")})
	preview, err := provider.Create(p.CreateRequest{Urn: urn, DryRun: true, Properties: inputs})
	require.NoError(t, err)
	require.True(t, preview.Properties.Get("portId").IsComputed())
	state := inputs.Set("portId", property.New("p1"))
	newInputs := inputs.Set("publishedPort", property.New(float64(8081)))
	change, err := provider.Update(p.UpdateRequest{ID: "p1", Urn: urn, State: state, OldInputs: inputs, Inputs: newInputs, DryRun: true})
	require.NoError(t, err)
	require.Equal(t, "p1", change.Properties.Get("portId").AsString())
}

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
