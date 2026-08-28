package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestFullLifecycleUsesProjectEnvironmentAcrossResources(t *testing.T) {
	scripted := newScriptedServer(t,
		expectPOST("/api/project.create", `{"name":"demo"}`, `{"project":{"projectId":"p1","name":"demo"},"environment":{"environmentId":"e1","name":"production","isDefault":true}}`),
		expectPOST("/api/environment.create", `{"projectId":"p1","name":"staging"}`, `{"environmentId":"e2","projectId":"p1","name":"staging","isDefault":false}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/environment.remove", Body: json.RawMessage(`{"environmentId":"e2"}`), Status: http.StatusOK, Response: []byte(`true`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/project.remove", Body: json.RawMessage(`{"projectId":"p1"}`), Status: http.StatusOK, Response: []byte(`true`)},
	)

	server, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, server.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
		"endpoint": property.New(scripted.server.URL),
		"apiKey":   property.New("integration-api-key"),
	})}))

	projectURN := resource.NewURN("stack", "project", "", "dokploy:index:Project", "project")
	project, err := server.Create(p.CreateRequest{
		Urn:        projectURN,
		Properties: property.NewMap(map[string]property.Value{"name": property.New("demo")}),
	})
	require.NoError(t, err)
	require.Equal(t, "p1", project.ID)
	require.Equal(t, "e1", project.Properties.Get("defaultEnvironmentId").AsString())

	environmentURN := resource.NewURN("stack", "project", "", "dokploy:index:Environment", "staging")
	environment, err := server.Create(p.CreateRequest{
		Urn: environmentURN,
		Properties: property.NewMap(map[string]property.Value{
			"projectId": property.New(project.ID),
			"name":      property.New("staging"),
		}),
	})
	require.NoError(t, err)
	require.Equal(t, "e2", environment.ID)
	require.Equal(t, project.ID, environment.Properties.Get("projectId").AsString())

	require.NoError(t, server.Delete(p.DeleteRequest{ID: environment.ID, Urn: environmentURN, Properties: environment.Properties}))
	require.NoError(t, server.Delete(p.DeleteRequest{ID: project.ID, Urn: projectURN, Properties: project.Properties}))
}
