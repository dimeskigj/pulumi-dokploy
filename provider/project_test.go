package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func expectPOST(path, body, response string) scriptedRequest {
	return scriptedRequest{Method: http.MethodPost, Path: path, Body: json.RawMessage(body), Status: http.StatusOK, Response: []byte(response)}
}

func expectGET(path string, query map[string][]string, status int, response string) scriptedRequest {
	return scriptedRequest{Method: http.MethodGet, Path: path, Query: query, Status: status, Response: []byte(response)}
}

func TestProjectPreviewMakesNoRequest(t *testing.T) {
	r := Project{}
	_, err := r.Create(t.Context(), infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: "demo"}, DryRun: true})
	require.NoError(t, err)
}

func TestProjectCreate(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/project.create", `{"name":"demo"}`, `{"project":{"projectId":"p1","name":"demo"},"environment":{"environmentId":"e1","name":"production","isDefault":true}}`))
	r := Project{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: "demo"}})
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "e1", got.Output.DefaultEnvironmentID)
}

func TestProjectCheckAndDiff(t *testing.T) {
	r := Project{}
	checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("")})})
	require.NoError(t, err)
	require.Len(t, checked.Failures, 1)
	diff, err := r.Diff(t.Context(), infer.DiffRequest[ProjectArgs, ProjectState]{State: ProjectState{ProjectArgs: ProjectArgs{Name: "old"}}, Inputs: ProjectArgs{Name: "new"}})
	require.NoError(t, err)
	require.True(t, diff.HasChanges)
	require.Equal(t, p.Update, diff.DetailedDiff["name"].Kind)
}
