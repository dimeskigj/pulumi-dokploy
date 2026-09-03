package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

var _ infer.ExplicitDependencies[ProjectTagArgs, ProjectTagState] = ProjectTag{}

func TestProjectTagIDRoundTrip(t *testing.T) {
	id := formatProjectTagID("p1", "t1")
	require.Equal(t, "p1/t1", id)
	p, tag, err := parseProjectTagID(id)
	require.NoError(t, err)
	require.Equal(t, "p1", p)
	require.Equal(t, "t1", tag)
}

func TestProjectTagRejectsInvalidIDs(t *testing.T) {
	for _, id := range []string{"", "p1", "/t1", "p1/", "p1/t1/extra"} {
		_, _, err := parseProjectTagID(id)
		require.Error(t, err, id)
	}
}

func TestProjectTagDiffReplacesBothFields(t *testing.T) {
	diff, err := (ProjectTag{}).Diff(t.Context(), infer.DiffRequest[ProjectTagArgs, ProjectTagState]{
		Inputs: ProjectTagArgs{ProjectID: "p2", TagID: "t2"},
		State:  ProjectTagState{ProjectTagArgs: ProjectTagArgs{ProjectID: "p1", TagID: "t1"}},
	})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["tagId"].Kind)
}

func TestProjectTagCreateReadDeleteIsolatesAssociation(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/tag.assignToProject", Body: json.RawMessage(`{"projectId":"p1","tagId":"t1"}`), Status: http.StatusOK, Response: []byte(`{}`)},
		expectGET("/api/project.one", map[string][]string{"projectId": {"p1"}}, http.StatusOK, `{"projectId":"p1","tags":[{"tagId":"other"},{"tagId":"t1"}]}`),
		expectGET("/api/project.one", map[string][]string{"projectId": {"p1"}}, http.StatusOK, `{"projectId":"p1","tags":[{"tagId":"other"},{"tagId":"t1"}]}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/tag.removeFromProject", Body: json.RawMessage(`{"projectId":"p1","tagId":"t1"}`), Status: http.StatusOK, Response: []byte(`{}`)},
	)
	r := ProjectTag{client: fixedClient(s.API())}
	created, err := r.Create(t.Context(), infer.CreateRequest[ProjectTagArgs]{Inputs: ProjectTagArgs{ProjectID: "p1", TagID: "t1"}})
	require.NoError(t, err)
	require.Equal(t, "p1/t1", created.ID)
	read, err := r.Read(t.Context(), infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: "p1/t1"})
	require.NoError(t, err)
	require.Equal(t, "p1", read.Inputs.ProjectID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ProjectTagState]{ID: "p1/t1"})
	require.NoError(t, err)
}

func TestProjectTagReadMissingAssociationAndProject(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/project.one", map[string][]string{"projectId": {"p1"}}, http.StatusOK, `{"projectId":"p1","tags":[{"tagId":"other"}]}`),
		expectGET("/api/project.one", map[string][]string{"projectId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
	)
	r := ProjectTag{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: "p1/t1"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	read, err = r.Read(t.Context(), infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: "missing/t1"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
}
