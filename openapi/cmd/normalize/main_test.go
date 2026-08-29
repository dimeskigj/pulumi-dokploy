package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var fixtureNames = splitLines("application.create\napplication.delete\napplication.deploy\napplication.one\napplication.reload\napplication.redeploy\napplication.saveBuildType\napplication.saveDockerProvider\napplication.saveEnvironment\napplication.saveGitProvider\napplication.saveGitlabProvider\napplication.update\ncompose.create\ncompose.delete\ncompose.deploy\ncompose.fetchSourceType\ncompose.one\ncompose.redeploy\ncompose.saveEnvironment\ncompose.update\ndomain.create\ndomain.delete\ndomain.one\ndomain.update\nenvironment.create\nenvironment.one\nenvironment.remove\nenvironment.update\npostgres.create\npostgres.deploy\npostgres.one\npostgres.remove\npostgres.saveEnvironment\npostgres.saveExternalPort\npostgres.update\nproject.create\nproject.one\nproject.remove\nproject.update\nredis.create\nredis.deploy\nredis.one\nredis.remove\nredis.saveEnvironment\nredis.saveExternalPort\nredis.update")

func normalizeFixture(t *testing.T) *Document {
	if t != nil {
		t.Helper()
	}
	d := &Document{OpenAPI: "3.1.0", Info: map[string]any{"title": "fixture"}, Paths: map[string]*PathItem{}, Components: &Components{Schemas: map[string]json.RawMessage{}}}
	for _, n := range fixtureNames {
		op := &Operation{OperationID: n, Raw: map[string]any{"responses": map[string]any{"200": map[string]any{}}}}
		p := &PathItem{Post: op, Methods: map[string]*Operation{"post": op}}
		if n == "application.one" || n == "application.reload" || n == "project.one" || n == "domain.one" || n == "environment.one" || n == "postgres.one" || n == "redis.one" || n == "compose.one" {
			p.Post = nil
			p.Get = op
			p.Methods = map[string]*Operation{"get": op}
		}
		d.Paths["/"+n] = p
	}
	d.Paths["/user.all"] = &PathItem{Get: &Operation{OperationID: "user-all", Raw: map[string]any{}}}
	return d
}
func contractWithout(names ...string) *Document {
	d := normalizeFixture(nil)
	for _, n := range names {
		if n[0] == '/' {
			delete(d.Paths, n)
		} else {
			delete(d.Paths, "/"+n)
		}
	}
	return d
}
func corrections() Corrections {
	return Corrections{Responses: map[string]string{"project.create": "CreateProjectResult"}, Schemas: map[string]json.RawMessage{"CreateProjectResult": json.RawMessage(`{"type":"object"}`)}}
}
func responseSchema(t *testing.T, d *Document, path, method, status string) struct {
	Ref string `json:"$ref"`
} {
	t.Helper()
	var x struct {
		Content map[string]struct {
			Schema struct {
				Ref string `json:"$ref"`
			} `json:"schema"`
		} `json:"content"`
	}
	op := d.Paths[path].Post
	if method == httpMethodGet {
		op = d.Paths[path].Get
	}
	b, _ := json.Marshal(op.Raw["responses"].(map[string]any)[status])
	require.NoError(t, json.Unmarshal(b, &x))
	return x.Content["application/json"].Schema
}

func TestNormalizeSelectsAndCorrectsContract(t *testing.T) {
	doc, err := normalize(normalizeFixture(t), fixtureNames, corrections())
	require.NoError(t, err)
	require.Len(t, doc.Paths, 46)
	require.Contains(t, doc.Paths, "/project.create")
	require.Contains(t, doc.Paths, "/application.reload")
	require.NotContains(t, doc.Paths, "/user.all")
	require.Equal(t, "#/components/schemas/CreateProjectResult", responseSchema(t, doc, "/project.create", "post", "200").Ref)
	require.Equal(t, "3.1.0", doc.OpenAPI)
}
func TestNormalizeRejectsMissingOperation(t *testing.T) {
	_, err := normalize(contractWithout("/domain.one"), []string{"domain.one"}, corrections())
	require.ErrorContains(t, err, "allowed operation domain.one is absent")
}
func TestNormalizeRejectsDuplicateOperationID(t *testing.T) {
	c := contractWithout()
	c.Paths["/domain.one"].Get.OperationID = c.Paths["/project.one"].Get.OperationID
	_, err := normalize(c, []string{"domain.one", "project.one"}, corrections())
	require.ErrorContains(t, err, "duplicate operation ID")
}

func TestNormalizePreservesPathFieldsAndMethods(t *testing.T) {
	c := contractWithout()
	c.Paths["/project.create"].Raw = map[string]json.RawMessage{
		"summary":    json.RawMessage(`"kept"`),
		"parameters": json.RawMessage(`[{"name":"shared","in":"query"}]`),
	}
	c.Paths["/project.create"].Methods["patch"] = &Operation{OperationID: "project-create-patch", Raw: map[string]any{"responses": map[string]any{}}}
	d, err := normalize(c, []string{"project.create"}, corrections())
	require.NoError(t, err)
	require.Contains(t, d.Paths["/project.create"].Methods, "patch")
	require.Equal(t, `"kept"`, string(d.Paths["/project.create"].Raw["summary"]))
}

func TestNormalizeRejectsDuplicateIDsAcrossMethods(t *testing.T) {
	c := contractWithout()
	c.Paths["/project.create"].Methods["patch"] = &Operation{OperationID: c.Paths["/project.create"].Post.OperationID, Raw: map[string]any{}}
	_, err := normalize(c, []string{"project.create"}, corrections())
	require.ErrorContains(t, err, "duplicate operation ID")
}

func TestNormalizeRetainsCorrectionTransitiveReferences(t *testing.T) {
	c := contractWithout()
	c.Components.Schemas["UpstreamLeaf"] = json.RawMessage(`{"type":"string"}`)
	corr := Corrections{
		Responses: map[string]string{"project.create": "Corrected"},
		Schemas: map[string]json.RawMessage{
			"Corrected": json.RawMessage(`{"type":"object","properties":{"value":{"$ref":"#/components/schemas/UpstreamLeaf"}}}`),
		},
	}
	d, err := normalize(c, []string{"project.create"}, corr)
	require.NoError(t, err)
	require.Contains(t, d.Components.Schemas, "Corrected")
	require.Contains(t, d.Components.Schemas, "UpstreamLeaf")
}

func TestNormalizeUsesDefinedSecurityScheme(t *testing.T) {
	c := normalizeFixture(t)
	c.Components.SecuritySchemes = map[string]json.RawMessage{"apiKey": json.RawMessage(`{"type":"apiKey","in":"header","name":"x-api-key"}`)}
	c.Paths["/project.create"].Post.Raw["security"] = []any{map[string]any{"Authorization": []any{}}}
	d, err := normalize(c, []string{"project.create"}, corrections())
	require.NoError(t, err)
	require.Contains(t, d.Components.SecuritySchemes, "apiKey")
	security := d.Paths["/project.create"].Post.Raw["security"].([]any)
	for _, entry := range security {
		for name := range entry.(map[string]any) {
			require.Equal(t, "apiKey", name)
		}
	}
}
