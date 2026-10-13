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
	"organization.active", "sshKey.create", "sshKey.all", "sshKey.one", "sshKey.update", "sshKey.remove",
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

func TestNotificationGotifyIntegerRequestContract(t *testing.T) {
	d := normalizeRealContract(t)
	for _, name := range []string{"notification.createGotify", "notification.updateGotify"} {
		properties := operationRequestSchema(t, d, name)["properties"].(map[string]any)
		priority := properties["priority"].(map[string]any)
		require.Equal(t, "integer", priority["type"])
		require.Equal(t, float64(1), priority["minimum"])
	}
	for _, name := range []string{"notification.createNtfy", "notification.updateNtfy", "notification.createPushover", "notification.updatePushover"} {
		properties := operationRequestSchema(t, d, name)["properties"].(map[string]any)
		require.Equal(t, "number", properties["priority"].(map[string]any)["type"])
	}
}

func TestApplicationSourceSaveOperationContracts(t *testing.T) {
	d := normalizeRealContract(t)
	for _, operation := range []struct {
		name       string
		properties []string
		required   []string
	}{
		{"application.saveGithubProvider", []string{"applicationId", "repository", "owner", "buildPath", "githubId", "branch", "triggerType", "enableSubmodules", "watchPaths"}, []string{"applicationId", "repository", "owner", "buildPath", "githubId", "branch", "triggerType"}},
		{"application.saveGiteaProvider", []string{"applicationId", "giteaBuildPath", "giteaOwner", "giteaRepository", "giteaId", "giteaBranch", "enableSubmodules", "watchPaths"}, []string{"applicationId", "giteaBuildPath", "giteaOwner", "giteaRepository", "giteaId", "giteaBranch"}},
		{"application.saveBitbucketProvider", []string{"applicationId", "bitbucketBuildPath", "bitbucketOwner", "bitbucketRepository", "bitbucketRepositorySlug", "bitbucketId", "bitbucketBranch", "enableSubmodules", "watchPaths"}, []string{"bitbucketBuildPath", "bitbucketOwner", "bitbucketRepository", "bitbucketRepositorySlug", "bitbucketId", "applicationId", "bitbucketBranch"}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			path := d.Paths["/"+operation.name]
			require.NotNil(t, path)
			require.NotNil(t, path.Post)
			require.Equal(t, strings.Replace(operation.name, ".", "-", 1), path.Post.OperationID)
			require.Equal(t, "boolean", responseSchemaType(t, d, "/"+operation.name, httpMethodPost, "200"))
			schema := operationRequestSchema(t, d, operation.name)
			properties := schema["properties"].(map[string]any)
			for _, name := range operation.properties {
				require.Contains(t, properties, name)
			}
			require.ElementsMatch(t, operation.required, schema["required"])
			for _, name := range operation.properties {
				if name == "applicationId" || name == "branch" || name == "giteaBranch" || name == "bitbucketBranch" || name == "triggerType" {
					continue
				}
				field := properties[name].(map[string]any)
				if name == "enableSubmodules" {
					require.Equal(t, "boolean", field["type"])
					continue
				}
				if name == "watchPaths" {
					requireNullableSchema(t, properties[name], "array")
					continue
				}
				requireNullableSchema(t, properties[name], "string")
			}
		})
	}
}

func requireNullableSchema(t *testing.T, raw any, valueType string) {
	t.Helper()
	property := raw.(map[string]any)
	if types, ok := property["type"].([]any); ok {
		require.ElementsMatch(t, []any{valueType, "null"}, types)
		return
	}
	anyOf, ok := property["anyOf"].([]any)
	require.True(t, ok, "property schema is not nullable")
	require.Len(t, anyOf, 2)
	types := []string{}
	for _, variant := range anyOf {
		types = append(types, variant.(map[string]any)["type"].(string))
	}
	require.ElementsMatch(t, []string{valueType, "null"}, types)
}

func TestNotificationMessagingServerThresholdContract(t *testing.T) {
	d := normalizeRealContract(t)
	for _, channel := range []string{"Gotify", "Ntfy"} {
		for _, action := range []string{"create", "update"} {
			name := "notification." + action + channel
			t.Run(name, func(t *testing.T) {
				schema := operationRequestSchema(t, d, name)
				properties := schema["properties"].(map[string]any)
				require.Contains(t, properties, "serverThreshold")
				require.Equal(t, "boolean", properties["serverThreshold"].(map[string]any)["type"])
				if action == "create" {
					require.Contains(t, schema["required"], "serverThreshold")
				} else {
					require.NotContains(t, schema["required"], "serverThreshold")
				}
				require.Contains(t, properties, "priority", "retain other upstream fields")
			})
		}
	}
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

func TestRealContractIncludesScheduleCRUD(t *testing.T) {
	d := normalizeRealContract(t)
	for _, operation := range []struct{ path, method string }{
		{"/schedule.create", "post"}, {"/schedule.update", "post"},
		{"/schedule.delete", "post"}, {"/schedule.one", "get"},
	} {
		p, ok := d.Paths[operation.path]
		require.True(t, ok, "missing path %s", operation.path)
		require.NotNil(t, p.Methods[operation.method], "missing %s method at %s", operation.method, operation.path)
	}
	require.Equal(t, "#/components/schemas/Schedule", responseSchema(t, d, "/schedule.one", "get", "200").Ref)
	schedule := componentSchema(t, d, "Schedule")
	require.Equal(t, []any{"scheduleId"}, schedule["required"])
	require.Equal(t, true, schedule["additionalProperties"])
	for _, operation := range []string{"schedule.create", "schedule.update", "schedule.delete"} {
		responses := d.Paths["/"+operation].Post.Raw["responses"].(map[string]any)
		ack := responses["200"].(map[string]any)
		require.NotContains(t, ack, "content", "%s acknowledgment must be bodyless", operation)
	}
	for _, operation := range []string{"schedule.create", "schedule.update"} {
		schema := operationRequestSchema(t, d, operation)
		required := []any{"name", "cronExpression", "command"}
		if operation == "schedule.update" {
			required = append(required, "scheduleId")
		}
		require.ElementsMatch(t, required, schema["required"])
		for _, field := range []string{"scheduleId", "name", "cronExpression", "command", "enabled"} {
			require.Contains(t, schema["properties"], field, "%s request lacks %s", operation, field)
		}
	}
	require.NotContains(t, d.Paths, "/schedule.list")
	require.NotContains(t, d.Paths, "/schedule.runManually")
}

func TestPortOpenAPIContract(t *testing.T) {
	d := normalizeRealContract(t)
	for _, operation := range []struct{ path, method string }{
		{"/port.one", "get"}, {"/port.create", "post"}, {"/port.update", "post"}, {"/port.delete", "post"},
	} {
		p, ok := d.Paths[operation.path]
		require.True(t, ok, "missing path %s", operation.path)
		require.NotNil(t, p.Methods[operation.method], "missing %s method", operation.path)
		require.Len(t, p.Methods, 1, "unexpected methods on %s", operation.path)
		require.Equal(t, "#/components/schemas/Port", responseSchema(t, d, operation.path, operation.method, "200").Ref)
	}
	s := componentSchema(t, d, "Port")
	require.Equal(t, []any{"portId"}, s["required"])
	require.Equal(t, true, s["additionalProperties"])
	properties := s["properties"].(map[string]any)
	require.Equal(t, "string", properties["portId"].(map[string]any)["type"])
	for _, field := range []string{"applicationId", "protocol", "publishMode"} {
		require.Equal(t, "string", properties[field].(map[string]any)["type"])
	}
	for _, field := range []string{"publishedPort", "targetPort"} {
		require.Equal(t, "integer", properties[field].(map[string]any)["type"])
	}
	create := operationRequestSchema(t, d, "port.create")
	require.ElementsMatch(t, []any{"applicationId", "publishedPort", "targetPort", "protocol", "publishMode"}, create["required"])
	update := operationRequestSchema(t, d, "port.update")
	require.NotContains(t, update["properties"], "applicationId")
	require.Contains(t, update["properties"], "portId")
	require.ElementsMatch(t, []any{"portId", "publishedPort", "targetPort", "protocol", "publishMode"}, update["required"])

	ids := normalizedOperationIDs(t, d)
	operations, err := os.ReadFile(repositoryOpenAPIPath(t, "operations.txt"))
	require.NoError(t, err)
	var expected []string
	for _, operation := range splitLines(string(operations)) {
		if operation != "" {
			expected = append(expected, strings.Replace(operation, ".", "-", 1))
		}
	}
	require.ElementsMatch(t, expected, ids, "normalized operation set must exactly match operations.txt")
}

func TestRealContractIncludesNotificationCRUD(t *testing.T) {
	doc := normalizeRealContract(t)
	channels := []string{"Slack", "Telegram", "Discord", "Email", "Resend", "Gotify", "Ntfy", "Mattermost", "Custom", "Lark", "Teams", "Pushover"}
	want := []string{"notification.one", "notification.all", "notification.remove"}
	for _, channel := range channels {
		want = append(want, "notification.create"+channel, "notification.update"+channel)
	}
	ids := normalizedOperationIDs(t, doc)
	for _, id := range want {
		require.Contains(t, ids, strings.Replace(id, ".", "-", 1))
	}
	for _, channel := range channels {
		field := strings.ToLower(channel[:1]) + channel[1:] + "Id"
		require.Contains(t, operationRequestSchema(t, doc, "notification.update"+channel)["required"], field, "update%s must require its relation ID", channel)
	}
	require.Len(t, want, 27)
	var notificationIDs []string
	for _, id := range ids {
		if strings.HasPrefix(id, "notification-") {
			notificationIDs = append(notificationIDs, id)
		}
	}
	require.Len(t, notificationIDs, 27)
	for _, id := range want {
		path, method := "/"+id, "post"
		if id == "notification.one" || id == "notification.all" {
			method = "get"
		}
		require.NotNil(t, doc.Paths[path].Methods[method], "%s must use %s", id, method)
	}
	require.Contains(t, doc.Paths, "/notification.one")
	require.NotNil(t, doc.Paths["/notification.one"].Get)
	require.Equal(t, "#/components/schemas/Notification", responseSchema(t, doc, "/notification.one", "get", "200").Ref)
	require.Equal(t, "#/components/schemas/NotificationList", responseSchema(t, doc, "/notification.all", "get", "200").Ref)
	require.NotContains(t, doc.Paths, "/notification.testSlackConnection")
	require.Equal(t, []any{"notificationId"}, operationRequestSchema(t, doc, "notification.remove")["required"])
	for _, id := range want {
		if strings.HasPrefix(id, "notification.create") || strings.HasPrefix(id, "notification.update") || id == "notification.remove" {
			ack := doc.Paths["/"+id].Post.Raw["responses"].(map[string]any)["200"].(map[string]any)
			require.NotContains(t, ack, "content", "%s must be bodyless", id)
		}
	}
	require.Contains(t, ids, "organization-active")
	require.Contains(t, ids, "schedule-one")
}

func TestRealContractIncludesServerCRUD(t *testing.T) {
	d := normalizeRealContract(t)
	require.Contains(t, d.Paths, "/server.one")
	require.NotNil(t, d.Paths["/server.one"].Get)
	for _, operation := range []string{"server.create", "server.update", "server.remove"} {
		require.Contains(t, d.Paths, "/"+operation)
		require.NotNil(t, d.Paths["/"+operation].Post)
	}
	require.Equal(t, "#/components/schemas/Server", responseSchema(t, d, "/server.one", "get", "200").Ref)
	require.Equal(t, "#/components/schemas/ServerMutationResult", responseSchema(t, d, "/server.create", "post", "200").Ref)
	require.Equal(t, "#/components/schemas/ServerMutationResult", responseSchema(t, d, "/server.update", "post", "200").Ref)
	removeResponse := d.Paths["/server.remove"].Post.Raw["responses"].(map[string]any)["200"].(map[string]any)
	require.NotContains(t, removeResponse, "content")
	for _, operation := range []string{"server.setup", "server.validate", "server.security", "server.setupMonitoring", "server.updateBuildsConcurrency"} {
		require.NotContains(t, d.Paths, "/"+operation)
	}
	server := componentSchema(t, d, "Server")
	require.ElementsMatch(t, []string{"serverId", "name", "description", "ipAddress", "port", "username", "sshKeyId", "serverType", "enableDockerCleanup", "organizationId", "serverStatus"}, mapKeys(server["properties"].(map[string]any)))
	require.Equal(t, []any{"serverId"}, server["required"])
	require.Equal(t, false, server["additionalProperties"])
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, server, "description"))
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, server, "sshKeyId"))
	require.Equal(t, "integer", componentSchemaPropertyType(t, server, "port"))
	require.Equal(t, []string{"string", "string"}, []string{componentSchemaPropertyType(t, server, "serverStatus"), componentSchemaPropertyType(t, server, "serverType")})
	ack := componentSchema(t, d, "ServerMutationResult")
	require.Equal(t, []any{"serverId"}, ack["required"])
	require.Equal(t, false, ack["additionalProperties"])
	for _, operation := range []string{"server.create", "server.update"} {
		req := operationRequestSchema(t, d, operation)
		require.Equal(t, "integer", componentSchemaPropertyType(t, req, "port"))
		require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, req, "description"))
		require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, req, "sshKeyId"))
		require.Equal(t, []any{"deploy", "build"}, req["properties"].(map[string]any)["serverType"].(map[string]any)["enum"])
		require.Equal(t, true, req["properties"].(map[string]any)["enableDockerCleanup"].(map[string]any)["default"])
		required := req["required"].([]any)
		require.Contains(t, required, "description")
		require.Contains(t, required, "sshKeyId")
	}
	require.Contains(t, operationRequestSchema(t, d, "server.update")["properties"], "command")
	for _, operation := range []string{"server.one", "server.update", "server.remove"} {
		rawOp := d.Paths["/"+operation]
		var parameters []any
		if operation == "server.one" {
			parameters = rawOp.Get.Raw["parameters"].([]any)
		} else if operation == "server.update" || operation == "server.remove" {
			req := operationRequestSchema(t, d, operation)
			require.Contains(t, req["properties"], "serverId")
			require.Contains(t, req["required"], "serverId")
			continue
		}
		require.Len(t, parameters, 1)
		require.Equal(t, "serverId", parameters[0].(map[string]any)["name"])
		require.Equal(t, true, parameters[0].(map[string]any)["required"])
	}
	for _, operation := range []string{"application.create", "project.one", "schedule.one", "sshKey.one"} {
		require.Contains(t, d.Paths, "/"+operation)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func componentSchemaPropertyType(t *testing.T, schema map[string]any, property string) string {
	t.Helper()
	propertySchema, ok := schema["properties"].(map[string]any)[property].(map[string]any)
	require.True(t, ok, "schema property %s is missing", property)
	typ, ok := propertySchema["type"].(string)
	require.True(t, ok, "schema property %s type is not a string", property)
	return typ
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

func TestNormalizeEnvironmentUpdateDescriptionIsOptionalString(t *testing.T) {
	output := normalizeRealContract(t)
	schema := operationRequestSchema(t, output, "environment.update")
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "environment.update request properties are not an object")
	description, ok := properties["description"].(map[string]any)
	require.True(t, ok, "environment.update request has no description property")
	require.Equal(t, "string", description["type"])
	required, ok := schema["required"].([]any)
	require.True(t, ok, "environment.update request required is not an array")
	require.NotContains(t, required, "description")
}

func TestGitLabIntegrationOpenAPIContract(t *testing.T) {
	d := normalizeRealContract(t)
	for _, operation := range []struct{ path, method string }{
		{"/gitlab.one", httpMethodGet}, {"/gitProvider.getAll", httpMethodGet},
		{"/user.get", httpMethodGet}, {"/gitlab.create", httpMethodPost},
		{"/gitlab.update", httpMethodPost}, {"/gitProvider.remove", httpMethodPost},
	} {
		path, ok := d.Paths[operation.path]
		require.True(t, ok, "missing path %s", operation.path)
		require.NotNil(t, path.Methods[operation.method], "missing %s operation at %s", operation.method, operation.path)
	}
	require.Len(t, d.Paths, 147, "the 139 baseline operations, six GitLab operations, and two GitHub operations are the complete inventory")
	require.Equal(t, "#/components/schemas/GitLabIntegrationRecord", responseSchema(t, d, "/gitlab.one", httpMethodGet, "200").Ref)
	require.Equal(t, "#/components/schemas/GitProviderList", responseSchema(t, d, "/gitProvider.getAll", httpMethodGet, "200").Ref)
	require.Equal(t, "#/components/schemas/AuthenticatedGitProviderMember", responseSchema(t, d, "/user.get", httpMethodGet, "200").Ref)

	create := operationRequestSchema(t, d, "gitlab.create")
	require.Contains(t, create["required"], "authId")
	for _, field := range []string{"applicationId", "secret", "redirectUri", "gitlabUrl", "gitlabInternalUrl", "groupName"} {
		require.Contains(t, create["properties"], field)
	}
	update := operationRequestSchema(t, d, "gitlab.update")
	require.Contains(t, update["required"], "gitlabId")
	require.Contains(t, update["required"], "gitProviderId")
	require.NotContains(t, update["properties"], "accessToken")
	remove := operationRequestSchema(t, d, "gitProvider.remove")
	require.Contains(t, remove["required"], "gitProviderId")
	record := componentSchema(t, d, "GitLabIntegrationRecord")
	require.ElementsMatch(t, []any{"gitlabId", "gitProviderId", "gitlabUrl", "gitProvider"}, record["required"])
	require.Equal(t, true, record["additionalProperties"])
	for _, field := range []string{"gitlabInternalUrl", "secret", "accessToken", "refreshToken"} {
		require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, record, field), "nullable %s", field)
	}
	identity := componentSchema(t, d, "GitLabProviderIdentity")
	require.Equal(t, []any{"gitProviderId"}, identity["required"])
	identityProperties := identity["properties"].(map[string]any)
	require.Equal(t, float64(1), identityProperties["gitProviderId"].(map[string]any)["minLength"])
	entry := componentSchema(t, d, "GitProviderListEntry")
	entryProperties := entry["properties"].(map[string]any)
	gitlabSummary := entryProperties["gitlab"].(map[string]any)
	require.Contains(t, gitlabSummary, "anyOf", "GitLab summary must allow explicit null")
	member := componentSchema(t, d, "AuthenticatedGitProviderMember")
	require.ElementsMatch(t, []any{"userId", "organizationId", "user"}, member["required"])
	for _, operation := range []string{"gitlab.create", "gitlab.update", "gitProvider.remove"} {
		responses := d.Paths["/"+operation].Post.Raw["responses"].(map[string]any)
		ack := responses["200"].(map[string]any)
		require.NotContains(t, ack, "content", "%s acknowledgment must be bodyless", operation)
	}
	for _, path := range []string{"/gitlab.gitlabProviders", "/gitlab.testConnection", "/gitlab.callback"} {
		require.NotContains(t, d.Paths, path)
	}
}

func TestGitHubIntegrationOpenAPIContract(t *testing.T) {
	d := normalizeRealContract(t)
	baseCount := 145 // Captured from the task's base operations inventory (including all six GitLab selections).
	for _, operation := range []struct{ path, method string }{
		{"/github.one", httpMethodGet}, {"/github.update", httpMethodPost},
		{"/organization.active", httpMethodGet}, {"/gitProvider.remove", httpMethodPost},
	} {
		path, ok := d.Paths[operation.path]
		require.True(t, ok, "missing path %s", operation.path)
		require.NotNil(t, path.Methods[operation.method], "missing %s operation at %s", operation.method, operation.path)
	}
	require.Equal(t, baseCount+2, len(d.Paths), "GitHub adds only github.one and github.update")
	require.Equal(t, "#/components/schemas/GitHubIntegrationRecord", responseSchema(t, d, "/github.one", httpMethodGet, "200").Ref)
	update := operationRequestSchema(t, d, "github.update")
	for _, field := range []string{"githubId", "gitProviderId", "name", "githubAppName"} {
		require.Contains(t, update["required"], field)
	}
	remove := operationRequestSchema(t, d, "gitProvider.remove")
	require.Contains(t, remove["required"], "gitProviderId")
	record := componentSchema(t, d, "GitHubIntegrationRecord")
	require.ElementsMatch(t, []any{"githubId", "gitProviderId", "githubAppName", "gitProvider"}, record["required"])
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, record, "githubAppName"))
	identity := componentSchema(t, d, "GitHubProviderIdentity")
	require.ElementsMatch(t, []any{"gitProviderId", "name", "providerType", "organizationId"}, identity["required"])
	for _, operation := range []string{"github.update", "gitProvider.remove"} {
		responses := d.Paths["/"+operation].Post.Raw["responses"].(map[string]any)
		require.NotContains(t, responses["200"].(map[string]any), "content")
	}
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
		"sshKey.all":          {schema: "SSHKeyList", method: httpMethodGet},
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
	for _, operation := range []string{
		"application.update", "postgres.update", "mysql.update",
		"mariadb.update", "mongo.update", "redis.update",
	} {
		require.Equal(t, "boolean", responseSchemaType(t, output, "/"+operation, httpMethodPost, "200"), "response correction for %s", operation)
		require.Empty(t, responseSchema(t, output, "/"+operation, httpMethodPost, "200").Ref, "response correction for %s", operation)
	}
	for _, operation := range []string{"sshKey.create", "sshKey.remove", "registry.remove", "registry.testRegistry", "tag.remove", "tag.assignToProject", "tag.removeFromProject", "mounts.remove"} {
		require.Empty(t, responseSchema(t, output, "/"+operation, httpMethodPost, "200").Ref, "response correction for %s should be empty", operation)
	}

	project := componentSchema(t, output, "Project")
	projectProperties, ok := project["properties"].(map[string]any)
	require.True(t, ok, "Project properties are not an object")
	tags, ok := projectProperties["projectTags"].(map[string]any)
	require.True(t, ok, "Project.projectTags is not an object")
	require.Equal(t, "array", tags["type"])
	tagItems, ok := tags["items"].(map[string]any)
	require.True(t, ok, "Project.projectTags items are not an object")
	tagItemProperties, ok := tagItems["properties"].(map[string]any)
	require.True(t, ok, "Project.projectTags item properties are not an object")
	require.Contains(t, tagItemProperties, "tagId")
	require.Equal(t, []any{"tagId"}, tagItems["required"])

	application := componentSchema(t, output, "Application")
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, application, "registryId"))
	require.Equal(t, []any{"string", "null"}, schemaPropertyTypes(t, application, "buildRegistryId"))
	organization := componentSchema(t, output, "Organization")
	organizationProperties, ok := organization["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "string"}, organizationProperties["id"])
	require.Equal(t, map[string]any{"type": "string"}, organizationProperties["organizationId"])
	require.NotContains(t, organization, "required")
}

func TestLookupOpenAPIContracts(t *testing.T) {
	doc := normalizeRealContract(t)
	require.Contains(t, doc.Paths, "/server.one")
	require.NotNil(t, doc.Paths["/server.one"].Get)
	require.Nil(t, doc.Paths["/server.one"].Post)
	require.Equal(t, "#/components/schemas/Server", responseSchema(t, doc, "/server.one", "get", "200").Ref)
	serverParams := doc.Paths["/server.one"].Get.Raw["parameters"].([]any)
	require.Len(t, serverParams, 1)
	parameter := serverParams[0].(map[string]any)
	require.Equal(t, "serverId", parameter["name"])
	require.Equal(t, true, parameter["required"])
	for _, name := range []string{"environment.one", "compose.one", "postgres.one", "mysql.one", "mariadb.one", "mongo.one", "redis.one"} {
		require.Contains(t, normalizedOperationIDs(t, doc), strings.Replace(name, ".", "-", 1))
	}
	assertProperty := func(schemaName, property string, expected any) {
		schema := componentSchema(t, doc, schemaName)
		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "%s properties", schemaName)
		require.Equal(t, expected, properties[property], "%s.%s", schemaName, property)
	}
	assertProperty("Environment", "isDefault", map[string]any{"type": "boolean"})
	for _, n := range []string{"Postgres", "MySQL", "MariaDB", "MongoDB", "Redis"} {
		assertProperty(n, "dockerImage", map[string]any{"type": []any{"string", "null"}})
		assertProperty(n, "image", map[string]any{"type": "string"})
	}
	require.Equal(t, map[string]any{"type": "string"}, componentSchema(t, doc, "Server")["properties"].(map[string]any)["serverId"])
}

func responseSchemaType(t *testing.T, d *Document, path, method, status string) string {
	t.Helper()
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
	var decoded struct {
		Content map[string]struct {
			Schema struct {
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(b, &decoded))
	return decoded.Content["application/json"].Schema.Type
}
