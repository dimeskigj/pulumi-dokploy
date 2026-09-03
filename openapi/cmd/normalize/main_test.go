package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	return Corrections{Responses: map[string]string{"project.create": "CreateProjectResult"}, Schemas: map[string]any{"CreateProjectResult": map[string]any{"type": "object"}}}
}

var newResourceOperations = []string{
	"organization.active", "sshKey.create", "sshKey.one", "sshKey.update", "sshKey.remove",
	"registry.create", "registry.one", "registry.update", "registry.remove", "registry.testRegistry",
	"tag.create", "tag.one", "tag.update", "tag.remove", "tag.assignToProject", "tag.removeFromProject",
	"mounts.create", "mounts.one", "mounts.update", "mounts.remove",
}

func newResourceFixture(t *testing.T) *Document {
	t.Helper()
	d := normalizeFixture(t)
	for _, n := range newResourceOperations {
		op := &Operation{OperationID: strings.Replace(n, ".", "-", 1), Raw: map[string]any{"responses": map[string]any{"200": map[string]any{}}}}
		if n == "registry.update" {
			op.Raw["requestBody"] = map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}}
		}
		d.Paths["/"+n] = &PathItem{Post: op, Methods: map[string]*Operation{"post": op}}
	}
	return d
}

func repositoryOpenAPIPath(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not locate test source")
	return filepath.Join(filepath.Dir(filename), "..", "..", name)
}

func normalizeRealContract(t *testing.T) *Document {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(repositoryOpenAPIPath(t, name))
		require.NoError(t, err, "read openapi/%s", name)
		return b
	}
	var input Document
	require.NoError(t, json.Unmarshal(read("upstream.json"), &input))
	var allow []string
	for _, line := range splitLines(string(read("operations.txt"))) {
		if line != "" {
			allow = append(allow, line)
		}
	}
	var corrections Corrections
	require.NoError(t, json.Unmarshal(read("corrections.json"), &corrections))
	output, err := normalize(&input, allow, corrections)
	require.NoError(t, err)
	return output
}

func normalizedOperationIDs(t *testing.T, d *Document) []string {
	t.Helper()
	ids := make([]string, 0, len(d.Paths))
	for _, path := range d.Paths {
		for _, operation := range allOperations(path) {
			ids = append(ids, operation.OperationID)
		}
	}
	return ids
}

func operationRequestSchema(t *testing.T, d *Document, operationID string) map[string]any {
	t.Helper()
	for _, path := range d.Paths {
		for _, operation := range allOperations(path) {
			if operation.OperationID != strings.Replace(operationID, ".", "-", 1) {
				continue
			}
			requestBody, ok := operation.Raw["requestBody"].(map[string]any)
			require.True(t, ok, "operation %s request body is not an object", operationID)
			content, ok := requestBody["content"].(map[string]any)
			require.True(t, ok, "operation %s request body content is not an object", operationID)
			applicationJSON, ok := content["application/json"].(map[string]any)
			require.True(t, ok, "operation %s has no application/json content", operationID)
			schema, ok := applicationJSON["schema"].(map[string]any)
			require.True(t, ok, "operation %s request schema is not an object", operationID)
			if ref, ok := schema["$ref"].(string); ok {
				name := strings.TrimPrefix(ref, "#/components/schemas/")
				var resolved map[string]any
				require.Contains(t, d.Components.Schemas, name, "operation %s references missing schema", operationID)
				require.NoError(t, json.Unmarshal(d.Components.Schemas[name], &resolved), "decode request schema %s", name)
				return resolved
			}
			return schema
		}
	}
	t.Fatalf("operation %s not found", operationID)
	return nil
}

func schemaPropertyTypes(t *testing.T, schema map[string]any, property string) []any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "schema properties are not an object")
	propertySchema, ok := properties[property].(map[string]any)
	require.True(t, ok, "schema property %s is not an object", property)
	types, ok := propertySchema["type"].([]any)
	require.True(t, ok, "schema property %s types are not an array", property)
	return types
}

func componentSchema(t *testing.T, d *Document, name string) map[string]any {
	t.Helper()
	raw, ok := d.Components.Schemas[name]
	require.True(t, ok, "missing component schema %s", name)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(raw, &schema), "decode component schema %s", name)
	return schema
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
	pathItem, ok := d.Paths[path]
	require.True(t, ok, "missing path %s", path)
	op := pathItem.Post
	if method == httpMethodGet {
		op = pathItem.Get
	}
	require.NotNil(t, op, "missing %s operation for path %s", method, path)
	responses, ok := op.Raw["responses"].(map[string]any)
	require.True(t, ok, "responses for %s are not an object", path)
	response, ok := responses[status]
	require.True(t, ok, "missing %s response for %s", status, path)
	b, err := json.Marshal(response)
	require.NoError(t, err)
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
		Schemas: map[string]any{
			"Corrected": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"$ref": "#/components/schemas/UpstreamLeaf"}}},
		},
	}
	d, err := normalize(c, []string{"project.create"}, corr)
	require.NoError(t, err)
	require.Contains(t, d.Components.Schemas, "Corrected")
	require.Contains(t, d.Components.Schemas, "UpstreamLeaf")
}

func TestNormalizeEmptyStringCorrectionRemovesResponseContent(t *testing.T) {
	c := contractWithout()
	c.Paths["/application.deploy"].Post.Raw["responses"] = map[string]any{"200": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "string"}}}}}
	corr := Corrections{Responses: map[string]string{"application.deploy": ""}}
	d, err := normalize(c, []string{"application.deploy"}, corr)
	require.NoError(t, err)
	responses := d.Paths["/application.deploy"].Post.Raw["responses"].(map[string]any)
	response := responses["200"].(map[string]any)
	require.NotContains(t, response, "content")
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

func TestNormalizeSelectsNewResourceOperations(t *testing.T) {
	output, err := normalize(newResourceFixture(t), newResourceOperations, Corrections{})
	require.NoError(t, err)
	for _, operation := range newResourceOperations {
		require.Contains(t, normalizedOperationIDs(t, output), strings.Replace(operation, ".", "-", 1), operation)
	}
}

func TestNormalizeCorrectsRegistryUpdateServerIDToNullable(t *testing.T) {
	var c Corrections
	require.NoError(t, json.Unmarshal([]byte(`{"requests":{"registry.update":"RegistryUpdateRequest"},"schemas":{"RegistryUpdateRequest":{"type":"object","properties":{"serverId":{"type":["string","null"]}}}}}`), &c))
	output, err := normalize(newResourceFixture(t), newResourceOperations, c)
	require.NoError(t, err)
	schema := operationRequestSchema(t, output, "registry.update")
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, schema, "serverId"))
}

func TestNormalizeUsesProductionOperationsAndCorrections(t *testing.T) {
	output := normalizeRealContract(t)
	ids := normalizedOperationIDs(t, output)
	operations, err := os.ReadFile(repositoryOpenAPIPath(t, "operations.txt"))
	require.NoError(t, err)
	var expected []string
	for _, operation := range splitLines(string(operations)) {
		if operation != "" {
			expected = append(expected, strings.Replace(operation, ".", "-", 1))
		}
	}
	require.ElementsMatch(t, expected, ids, "normalized operation set must exactly match operations.txt")
	for _, operation := range newResourceOperations {
		require.Contains(t, ids, strings.Replace(operation, ".", "-", 1), "missing normalized operation %s", operation)
	}

	responses := map[string]struct {
		schema string
		method string
	}{
		"organization.active": {schema: "Organization", method: httpMethodGet},
		"sshKey.create":       {schema: "SSHKey", method: httpMethodPost},
		"sshKey.one":          {schema: "SSHKey", method: httpMethodGet},
		"sshKey.update":       {schema: "SSHKey", method: httpMethodPost},
		"registry.create":     {schema: "Registry", method: httpMethodPost},
		"registry.one":        {schema: "Registry", method: httpMethodGet},
		"registry.update":     {schema: "Registry", method: httpMethodPost},
		"tag.create":          {schema: "Tag", method: httpMethodPost},
		"tag.one":             {schema: "Tag", method: httpMethodGet},
		"tag.update":          {schema: "Tag", method: httpMethodPost},
		"mounts.create":       {schema: "Mount", method: httpMethodPost},
		"mounts.one":          {schema: "Mount", method: httpMethodGet},
		"mounts.update":       {schema: "Mount", method: httpMethodPost},
	}
	for operation, assertion := range responses {
		require.Equal(t, "#/components/schemas/"+assertion.schema, responseSchema(t, output, "/"+operation, assertion.method, "200").Ref, "response correction for %s", operation)
	}
	for _, operation := range []string{"sshKey.remove", "registry.remove", "registry.testRegistry", "tag.remove", "tag.assignToProject", "tag.removeFromProject", "mounts.remove"} {
		require.Empty(t, responseSchema(t, output, "/"+operation, httpMethodPost, "200").Ref, "response correction for %s should be empty", operation)
	}

	project := componentSchema(t, output, "Project")
	projectProperties, ok := project["properties"].(map[string]any)
	require.True(t, ok, "Project properties are not an object")
	tags, ok := projectProperties["tags"].(map[string]any)
	require.True(t, ok, "Project.tags is not an object")
	require.Equal(t, "array", tags["type"])
	tagItems, ok := tags["items"].(map[string]any)
	require.True(t, ok, "Project.tags items are not an object")
	tagItemProperties, ok := tagItems["properties"].(map[string]any)
	require.True(t, ok, "Project.tags item properties are not an object")
	require.Contains(t, tagItemProperties, "tagId")

	application := componentSchema(t, output, "Application")
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, application, "registryId"))
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, application, "buildRegistryId"))
}
