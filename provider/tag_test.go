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

func TestTagAcceptsAndRoundTripsOpaqueColor(t *testing.T) {
	color := "not-a-css-color-or-hex"
	checked, err := (Tag{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("opaque"), "color": property.New(color),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)

	s := newScriptedServer(t,
		expectPOST("/api/tag.create", `{"color":"not-a-css-color-or-hex","name":"opaque"}`, `{"tagId":"t-opaque"}`),
		expectGET("/api/tag.one", map[string][]string{"tagId": {"t-opaque"}}, http.StatusOK, `{"tagId":"t-opaque","name":"opaque","color":"not-a-css-color-or-hex"}`),
	)
	created, err := (Tag{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: "opaque", Color: &color}})
	require.NoError(t, err)
	require.Equal(t, color, *created.Output.Color)
}

func TestTagCreateRejectsIncompleteResponse(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/tag.create", `{"color":null,"name":"one"}`, `{}`))
	_, err := (Tag{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: "one"}})
	require.EqualError(t, err, "tag.create returned incomplete tag")
}

func TestTagReadRejectsIncompleteResponse(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/tag.one", map[string][]string{"tagId": {"t1"}}, http.StatusOK, `{"name":"one"}`))
	_, err := (Tag{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[TagArgs, TagState]{ID: "t1"})
	require.EqualError(t, err, "tag.one returned incomplete tag")
}

func TestTagUpdateRejectsIncompleteResponse(t *testing.T) {
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/tag.update", Body: json.RawMessage(`{"color":null,"name":"one","tagId":"t1"}`), Status: http.StatusOK, Response: []byte(`{}`)})
	_, err := (Tag{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[TagArgs, TagState]{ID: "t1", Inputs: TagArgs{Name: "one"}})
	require.EqualError(t, err, "tag.update returned incomplete tag")
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
