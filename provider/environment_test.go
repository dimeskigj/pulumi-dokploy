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

func TestEnvironmentCheckRejectsProduction(t *testing.T) {
	r := Environment{}
	checked, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"projectId": property.New("p1"), "name": property.New("production"),
	})})
	require.NoError(t, err)
	require.Len(t, checked.Failures, 1)
	require.Contains(t, checked.Failures[0].Reason, "production")
}

func TestEnvironmentPreviewMakesNoRequest(t *testing.T) {
	_, err := (Environment{}).Create(t.Context(), infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: "p1", Name: "staging"}, DryRun: true})
	require.NoError(t, err)
}

func TestEnvironmentCreate(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/environment.create", `{"projectId":"p1","name":"staging","description":"demo"}`, `{"environmentId":"e1","projectId":"p1","name":"staging","isDefault":false}`))
	r := Environment{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: "p1", Name: "staging", Description: stringPtr("demo")}})
	require.NoError(t, err)
	require.Equal(t, "e1", got.ID)
}

func TestEnvironmentCreateDefaultRemovesCreatedEnvironmentBeforeReturningUnsupportedError(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/environment.create", `{"name":"production","projectId":"p1"}`, `{"environmentId":"e1","isDefault":true}`),
		expectPOST("/api/environment.remove", `{"environmentId":"e1"}`, `true`),
	)
	_, err := (Environment{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: "p1", Name: "production"}})
	require.EqualError(t, err, unsupportedDefaultEnvironment)
}

func TestEnvironmentCreateDefaultPreservesUnsupportedErrorWhenCleanupFails(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/environment.create", `{"name":"production","projectId":"p1"}`, `{"environmentId":"e1","isDefault":true}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/environment.remove", Body: json.RawMessage(`{"environmentId":"e1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"cleanup failed"}`)},
	)
	_, err := (Environment{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: "p1", Name: "production"}})
	require.EqualError(t, err, unsupportedDefaultEnvironment)
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

func TestEnvironmentDiffReportsOnlyChangedFields(t *testing.T) {
	r := Environment{}
	oldDescription := stringPtr("old")
	cases := []struct {
		name string
		old  EnvironmentArgs
		in   EnvironmentArgs
		want map[string]p.PropertyDiff
	}{
		{"unchanged", EnvironmentArgs{ProjectID: "p1", Name: "staging"}, EnvironmentArgs{ProjectID: "p1", Name: "staging"}, map[string]p.PropertyDiff{}},
		{"project", EnvironmentArgs{ProjectID: "p1", Name: "staging"}, EnvironmentArgs{ProjectID: "p2", Name: "staging"}, map[string]p.PropertyDiff{"projectId": {Kind: p.UpdateReplace}}},
		{"name", EnvironmentArgs{ProjectID: "p1", Name: "staging"}, EnvironmentArgs{ProjectID: "p1", Name: "new"}, map[string]p.PropertyDiff{"name": {Kind: p.Update}}},
		{"description", EnvironmentArgs{ProjectID: "p1", Name: "staging", Description: oldDescription}, EnvironmentArgs{ProjectID: "p1", Name: "staging", Description: stringPtr("new")}, map[string]p.PropertyDiff{"description": {Kind: p.Update}}},
		{"description-cleared", EnvironmentArgs{ProjectID: "p1", Name: "staging", Description: oldDescription}, EnvironmentArgs{ProjectID: "p1", Name: "staging"}, map[string]p.PropertyDiff{"description": {Kind: p.Update}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff, err := r.Diff(t.Context(), infer.DiffRequest[EnvironmentArgs, EnvironmentState]{State: EnvironmentState{EnvironmentArgs: tc.old}, Inputs: tc.in})
			require.NoError(t, err)
			require.Equal(t, len(tc.want) != 0, diff.HasChanges)
			require.Equal(t, tc.want, diff.DetailedDiff)
		})
	}
}

func TestEnvironmentUpdateClearsDescription(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/environment.update", `{"environmentId":"e1","name":"staging","description":null}`, `{}`))
	r := Environment{client: fixedClient(s.API())}
	_, err := r.Update(t.Context(), infer.UpdateRequest[EnvironmentArgs, EnvironmentState]{ID: "e1", Inputs: EnvironmentArgs{ProjectID: "p1", Name: "staging"}, State: EnvironmentState{EnvironmentID: "e1"}})
	require.NoError(t, err)
}

func TestEnvironmentLifecycleReadUpdateDeleteAndImport(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/environment.one", map[string][]string{"environmentId": {"e1"}}, http.StatusOK, `{"environmentId":"e1","projectId":"p1","name":"staging","description":"kept","isDefault":false}`),
		expectPOST("/api/environment.update", `{"environmentId":"e1","name":"renamed","description":"changed"}`, `{}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/environment.remove", Body: json.RawMessage(`{"environmentId":"e1"}`), Status: http.StatusOK, Response: []byte(`true`)},
	)
	r := Environment{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: "e1"})
	require.NoError(t, err)
	require.Equal(t, "e1", read.ID)
	require.Equal(t, EnvironmentArgs{ProjectID: "p1", Name: "staging", Description: stringPtr("kept")}, read.Inputs)
	_, err = r.Update(t.Context(), infer.UpdateRequest[EnvironmentArgs, EnvironmentState]{ID: "e1", Inputs: EnvironmentArgs{ProjectID: "p1", Name: "renamed", Description: stringPtr("changed")}, State: read.State})
	require.NoError(t, err)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[EnvironmentState]{ID: "e1"})
	require.NoError(t, err)
}

func TestEnvironmentReadAndDeleteNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/environment.one", map[string][]string{"environmentId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/environment.remove", Body: json.RawMessage(`{"environmentId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Environment{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[EnvironmentState]{ID: "missing"})
	require.NoError(t, err)
}

func TestEnvironmentReadRejectsDefaultEnvironment(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/environment.one", map[string][]string{"environmentId": {"e1"}}, http.StatusOK, `{"environmentId":"e1","projectId":"p1","name":"production","isDefault":true}`))
	r := Environment{client: fixedClient(s.API())}
	_, err := r.Read(t.Context(), infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: "e1"})
	require.EqualError(t, err, unsupportedDefaultEnvironment)
}

func inferStateEnvironment(projectID, name string) EnvironmentState {
	return EnvironmentState{EnvironmentArgs: EnvironmentArgs{ProjectID: projectID, Name: name}}
}
