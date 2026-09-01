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

var _ infer.ExplicitDependencies[TagArgs, TagState] = Tag{}

func TestTagCheckRequiresName(t *testing.T) {
	checked, err := (Tag{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{"name": property.New("")})})
	require.NoError(t, err)
	require.NotEmpty(t, checked.Failures)
}

func TestTagDiffUpdatesNameAndColor(t *testing.T) {
	diff, err := (Tag{}).Diff(t.Context(), infer.DiffRequest[TagArgs, TagState]{Inputs: TagArgs{Name: "new", Color: stringPtr("blue")}, State: TagState{TagArgs: TagArgs{Name: "old", Color: stringPtr("red")}}})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["name"].Kind)
	require.Equal(t, p.Update, diff.DetailedDiff["color"].Kind)
}

func TestTagCRUDImportAndNotFound(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/tag.create", `{"color":"blue","name":"one"}`, `{"tagId":"t1"}`),
		expectGET("/api/tag.one", map[string][]string{"tagId": {"t1"}}, http.StatusOK, `{"tagId":"t1","name":"one","color":"blue"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/tag.update", Body: json.RawMessage(`{"color":"green","name":"two","tagId":"t1"}`), Status: http.StatusOK, Response: []byte(`{"tagId":"t1"}`)},
		expectGET("/api/tag.one", map[string][]string{"tagId": {"t1"}}, http.StatusOK, `{"tagId":"t1","name":"one","color":"blue"}`),
		expectGET("/api/tag.one", map[string][]string{"tagId": {"missing"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/tag.remove", Body: json.RawMessage(`{"tagId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Tag{client: fixedClient(s.API())}
	created, err := r.Create(t.Context(), infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: "one", Color: stringPtr("blue")}})
	require.NoError(t, err)
	require.Equal(t, "t1", created.ID)
	_, err = r.Update(t.Context(), infer.UpdateRequest[TagArgs, TagState]{ID: "t1", Inputs: TagArgs{Name: "two", Color: stringPtr("green")}})
	require.NoError(t, err)
	read, err := r.Read(t.Context(), infer.ReadRequest[TagArgs, TagState]{ID: "t1"})
	require.NoError(t, err)
	require.Equal(t, "one", read.Inputs.Name)
	read, err = r.Read(t.Context(), infer.ReadRequest[TagArgs, TagState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[TagState]{ID: "missing"})
	require.NoError(t, err)
}
