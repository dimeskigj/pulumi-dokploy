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

func TestProjectDiffReportsOnlyChangedFields(t *testing.T) {
	r := Project{}
	cases := []struct {
		name string
		old  ProjectArgs
		in   ProjectArgs
		want map[string]p.PropertyDiff
	}{
		{"unchanged", ProjectArgs{Name: "demo"}, ProjectArgs{Name: "demo"}, map[string]p.PropertyDiff{}},
		{"name", ProjectArgs{Name: "old"}, ProjectArgs{Name: "new"}, map[string]p.PropertyDiff{"name": {Kind: p.Update}}},
		{"description", ProjectArgs{Name: "demo", Description: stringPtr("old")}, ProjectArgs{Name: "demo", Description: stringPtr("new")}, map[string]p.PropertyDiff{"description": {Kind: p.Update}}},
		{"description-cleared", ProjectArgs{Name: "demo", Description: stringPtr("old")}, ProjectArgs{Name: "demo"}, map[string]p.PropertyDiff{"description": {Kind: p.Update}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff, err := r.Diff(t.Context(), infer.DiffRequest[ProjectArgs, ProjectState]{State: ProjectState{ProjectArgs: tc.old}, Inputs: tc.in})
			require.NoError(t, err)
			require.Equal(t, len(tc.want) != 0, diff.HasChanges)
			require.Equal(t, tc.want, diff.DetailedDiff)
		})
	}
}

func TestProjectUpdateClearsDescription(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/project.update", `{"projectId":"p1","name":"demo","description":null}`, `{}`))
	r := Project{client: fixedClient(s.API())}
	_, err := r.Update(t.Context(), infer.UpdateRequest[ProjectArgs, ProjectState]{ID: "p1", Inputs: ProjectArgs{Name: "demo"}, State: ProjectState{ProjectID: "p1"}})
	require.NoError(t, err)
}

func TestProjectLifecycleReadUpdateDeleteAndImport(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/project.one", map[string][]string{"projectId": {"p1"}}, http.StatusOK, `{"projectId":"p1","name":"demo","description":"kept","defaultEnvironmentId":"e1"}`),
		expectPOST("/api/project.update", `{"projectId":"p1","name":"renamed","description":"changed"}`, `{}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/project.remove", Body: json.RawMessage(`{"projectId":"p1"}`), Status: http.StatusOK, Response: []byte(`true`)},
	)
	r := Project{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[ProjectArgs, ProjectState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "p1", read.ID)
	require.Equal(t, "demo", read.Inputs.Name)
	require.Equal(t, "e1", read.State.DefaultEnvironmentID)
	_, err = r.Update(t.Context(), infer.UpdateRequest[ProjectArgs, ProjectState]{ID: "p1", Inputs: ProjectArgs{Name: "renamed", Description: stringPtr("changed")}, State: read.State})
	require.NoError(t, err)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ProjectState]{ID: "p1"})
	require.NoError(t, err)
}

func TestProjectReadAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/project.one", map[string][]string{"projectId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/project.remove", Body: json.RawMessage(`{"projectId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Project{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[ProjectArgs, ProjectState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ProjectState]{ID: "missing"})
	require.NoError(t, err)
}

func stringPtr(value string) *string { return &value }
