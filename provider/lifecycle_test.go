package dokploy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestFullLifecycleUsesProjectEnvironmentAcrossResources(t *testing.T) {
	expectedSecrets := []string{
		"provider-key", "ENV-SECRET", "ARGS-SECRET", "BUILD-SECRET", "APP-DOCKER-PASSWORD",
		"APP-UPDATE-PASSWORD", "COMPOSE-ENV-SECRET", "DB-PASSWORD", "DB-UPDATE-PASSWORD",
		"REDIS-PASSWORD", "REDIS-UPDATE-PASSWORD",
	}
	api := newLifecycleAPI(t, expectedSecrets)
	oldPoll := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldPoll })

	provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
		"endpoint": property.New(api.URL()), "apiKey": property.New("provider-key"),
	})}))

	project := lifecycleCreate(t, provider, "dokploy:index:Project", "project", map[string]property.Value{"name": property.New("demo")})
	environmentID := project.Properties.Get("defaultEnvironmentId").AsString()
	require.Equal(t, "e1", environmentID)
	environment := lifecycleCreate(t, provider, "dokploy:index:Environment", "environment", map[string]property.Value{
		"projectId": property.New(project.ID), "name": property.New("staging"),
	})
	require.Equal(t, project.ID, environment.Properties.Get("projectId").AsString())

	application := lifecycleCreate(t, provider, "dokploy:index:Application", "application", map[string]property.Value{
		"name": property.New("app"), "environmentId": property.New(environmentID),
		"source": property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("nginx"), "password": property.New("APP-DOCKER-PASSWORD")})}),
	})
	compose := lifecycleCreate(t, provider, "dokploy:index:Compose", "compose", map[string]property.Value{
		"name": property.New("stack"), "environmentId": property.New(environmentID),
		"environment": property.New("COMPOSE-ENV-SECRET"),
		"source":      property.New(property.NewMap(map[string]property.Value{"type": property.New("raw"), "raw": property.New(property.NewMap(map[string]property.Value{"composeFile": property.New("services: {}")}))})),
	})
	postgres := lifecycleCreate(t, provider, "dokploy:index:Postgres", "postgres", map[string]property.Value{
		"name": property.New("db"), "environmentId": property.New(environmentID), "databaseName": property.New("app"),
		"databaseUser": property.New("app"), "databasePassword": property.New("DB-PASSWORD"),
	})
	redis := lifecycleCreate(t, provider, "dokploy:index:Redis", "redis", map[string]property.Value{
		"name": property.New("cache"), "environmentId": property.New(environmentID), "databasePassword": property.New("REDIS-PASSWORD"),
	})
	applicationDomain := lifecycleCreate(t, provider, "dokploy:index:Domain", "application-domain", map[string]property.Value{
		"applicationId": property.New(application.ID), "host": property.New("app.example.com"),
	})
	composeDomain := lifecycleCreate(t, provider, "dokploy:index:Domain", "compose-domain", map[string]property.Value{
		"composeId": property.New(compose.ID), "serviceName": property.New("web"), "host": property.New("stack.example.com"),
	})

	resources := []struct {
		response p.CreateResponse
		urn      resource.URN
	}{{project, lifecycleURN("Project", "project")}, {environment, lifecycleURN("Environment", "environment")}, {application, lifecycleURN("Application", "application")}, {compose, lifecycleURN("Compose", "compose")}, {postgres, lifecycleURN("Postgres", "postgres")}, {redis, lifecycleURN("Redis", "redis")}, {applicationDomain, lifecycleURN("Domain", "application-domain")}, {composeDomain, lifecycleURN("Domain", "compose-domain")}}
	for _, r := range resources {
		_, err := provider.Read(p.ReadRequest{ID: r.response.ID, Urn: r.urn, Properties: r.response.Properties})
		require.NoError(t, err)
	}

	_, err = provider.Update(p.UpdateRequest{ID: project.ID, Urn: lifecycleURN("Project", "project"), State: project.Properties,
		OldInputs: project.Properties, Inputs: property.NewMap(map[string]property.Value{"name": property.New("renamed")})})
	require.NoError(t, err)
	_, err = provider.Update(p.UpdateRequest{ID: application.ID, Urn: lifecycleURN("Application", "application"), State: application.Properties,
		OldInputs: application.Properties, Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("app"), "environmentId": property.New(environmentID),
			"source": property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("alpine")})}),
		})})
	require.NoError(t, err)
	_, err = provider.Update(p.UpdateRequest{ID: application.ID, Urn: lifecycleURN("Application", "application"), State: application.Properties,
		OldInputs: application.Properties, Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("app"), "environmentId": property.New(environmentID),
			"environment": property.New("ENV-SECRET"), "buildArgs": property.New("ARGS-SECRET"), "buildSecrets": property.New("BUILD-SECRET"),
			"source": property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("broken"), "password": property.New("APP-UPDATE-PASSWORD")})}),
		})})
	require.Error(t, err)
	assertLifecycleSecretsRedacted(t, expectedSecrets, err)

	imported, err := provider.Read(p.ReadRequest{ID: postgres.ID, Urn: lifecycleURN("Postgres", "postgres")})
	require.NoError(t, err)
	require.Equal(t, postgres.ID, imported.ID)

	_, err = provider.Update(p.UpdateRequest{ID: postgres.ID, Urn: lifecycleURN("Postgres", "postgres"), State: postgres.Properties,
		OldInputs: postgres.Properties, Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("db"), "environmentId": property.New(environmentID), "databaseName": property.New("app-updated"),
			"databaseUser": property.New("app"), "databasePassword": property.New("DB-UPDATE-PASSWORD"),
		})})
	require.Error(t, err)
	assertLifecycleSecretsRedacted(t, expectedSecrets, err)
	_, err = provider.Update(p.UpdateRequest{ID: redis.ID, Urn: lifecycleURN("Redis", "redis"), State: redis.Properties,
		OldInputs: redis.Properties, Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("cache"), "environmentId": property.New(environmentID), "databasePassword": property.New("REDIS-UPDATE-PASSWORD"), "dockerImage": property.New("redis:9"),
		})})
	require.Error(t, err)
	assertLifecycleSecretsRedacted(t, expectedSecrets, err)

	for _, r := range []struct {
		response p.CreateResponse
		kind     string
	}{{composeDomain, "Domain"}, {applicationDomain, "Domain"}, {application, "Application"}, {compose, "Compose"}, {redis, "Redis"}, {postgres, "Postgres"}, {environment, "Environment"}, {project, "Project"}} {
		require.NoError(t, provider.Delete(p.DeleteRequest{ID: r.response.ID, Urn: lifecycleURN(r.kind, "delete"), Properties: r.response.Properties}))
	}

	api.AssertRequests(t)
}

func TestMountLifecycleOrderingAndImport(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mounts.create", `{"content":null,"filePath":null,"hostPath":"/host","mountPath":"/data","serviceId":"a1","serviceType":"application","type":"bind","volumeName":null}`, `{"mountId":"m1"}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","hostPath":"/host","type":"bind","serviceType":"application","applicationId":"a1"}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","hostPath":"/host","type":"bind","serviceType":"application","applicationId":"a1"}`),
	)
	provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
	require.NoError(t, err)
	require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
		"endpoint": property.New(s.server.URL), "apiKey": property.New("test-api-key"),
	})}))
	created, err := provider.Create(p.CreateRequest{Urn: lifecycleURN("Mount", "mount"), Properties: property.NewMap(map[string]property.Value{
		"type": property.New("bind"), "mountPath": property.New("/data"), "hostPath": property.New("/host"), "applicationId": property.New("a1"),
	})})
	require.NoError(t, err)
	imported, err := provider.Read(p.ReadRequest{ID: created.ID, Urn: lifecycleURN("Mount", "mount")})
	require.NoError(t, err)
	require.Equal(t, created.ID, imported.ID)
}

func assertLifecycleSecretsRedacted(t *testing.T, expectedSecrets []string, err error) {
	t.Helper()
	for _, secret := range expectedSecrets {
		require.NotContains(t, err.Error(), secret)
	}
}

func lifecycleCreate(t *testing.T, provider integration.Server, kind, name string, inputs map[string]property.Value) p.CreateResponse {
	t.Helper()
	urn := lifecycleURN(strings.TrimPrefix(kind, "dokploy:index:"), name)
	created, err := provider.Create(p.CreateRequest{Urn: urn, Properties: property.NewMap(inputs)})
	require.NoError(t, err)
	return created
}

func lifecycleURN(kind, name string) resource.URN {
	return resource.NewURN("stack", "project", "", tokens.Type("dokploy:index:"+kind), name)
}

type lifecycleAPI struct {
	server          *httptest.Server
	mu              sync.Mutex
	expected        []lifecycleRequest
	seen            int
	secrets         map[string]struct{}
	expectedSecrets []string
}

type lifecycleRequest struct {
	method       string
	path         string
	query        url.Values
	body         json.RawMessage
	status       int
	response     string
	responseFunc func([]string) string
}

func newLifecycleAPI(t *testing.T, expectedSecrets []string) *lifecycleAPI {
	api := &lifecycleAPI{secrets: map[string]struct{}{}, expectedSecrets: expectedSecrets}
	api.expected = lifecycleTranscript()
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.mu.Lock()
		index := api.seen
		api.seen++
		if index >= len(api.expected) {
			api.mu.Unlock()
			t.Errorf("unexpected request %d: %s %s", index, r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		expected := api.expected[index]
		api.mu.Unlock()
		queriesMatch := len(r.URL.Query()) == 0 && len(expected.query) == 0 || reflect.DeepEqual(r.URL.Query(), expected.query)
		if r.Method != expected.method || r.URL.Path != expected.path || !queriesMatch {
			t.Errorf("request %d mismatch: got %s %s?%s, want %s %s?%s", index, r.Method, r.URL.Path, r.URL.Query().Encode(), expected.method, expected.path, expected.query.Encode())
		}
		assertSemanticJSON(t, body, expected.body, index)
		api.mu.Lock()
		api.captureSecrets(body)
		api.captureSecret(r.Header.Get("x-api-key"))
		api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(expected.status)
		response := expected.response
		if expected.responseFunc != nil {
			response = expected.responseFunc(api.expectedSecrets)
		}
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(api.server.Close)
	t.Cleanup(func() {
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.seen != len(api.expected) {
			t.Errorf("request transcript incomplete: got %d requests, want %d", api.seen, len(api.expected))
		}
	})
	return api
}

func (s *lifecycleAPI) captureSecrets(body []byte) {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return
	}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case string:
			for _, expected := range s.expectedSecrets {
				if value == expected {
					s.secrets[value] = struct{}{}
				}
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		case map[string]any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(value)
}

func (s *lifecycleAPI) captureSecret(secret string) {
	for _, expected := range s.expectedSecrets {
		if secret == expected {
			s.secrets[secret] = struct{}{}
		}
	}
}

func (s *lifecycleAPI) missingExpectedSecretsLocked() []string {
	missing := []string{}
	for _, expected := range s.expectedSecrets {
		if _, ok := s.secrets[expected]; !ok {
			missing = append(missing, expected)
		}
	}
	return missing
}

func (s *lifecycleAPI) URL() string { return s.server.URL }

func (s *lifecycleAPI) AssertRequests(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Equal(t, len(s.expected), s.seen)
	require.Empty(t, s.missingExpectedSecretsLocked())
}

func assertSemanticJSON(t *testing.T, actual, expected []byte, index int) {
	t.Helper()
	if len(expected) == 0 {
		require.Empty(t, strings.TrimSpace(string(actual)), "request %d body", index)
		return
	}
	var got, want any
	require.NoError(t, json.Unmarshal(actual, &got), "request %d body", index)
	require.NoError(t, json.Unmarshal(expected, &want), "expected request %d body", index)
	require.Equal(t, want, got, "request %d semantic body", index)
}

func lifecycleTranscript() []lifecycleRequest {
	q := func(values ...string) url.Values {
		if len(values) == 0 {
			return nil
		}
		return url.Values{values[0]: {values[1]}}
	}
	r := func(method, path string, query url.Values, body, response string) lifecycleRequest {
		return lifecycleRequest{method: method, path: path, query: query, body: json.RawMessage(body), status: http.StatusOK, response: response}
	}
	get := func(path, key, id, response string) lifecycleRequest {
		return r(http.MethodGet, path, q(key, id), "", response)
	}
	trueResponse := `true`
	secretEcho := func(secrets []string) string {
		return `{"message":"redeploy failed ` + strings.Join(secrets, " ") + `"}`
	}
	appOne := `{"applicationId":"a1","name":"app","environmentId":"e1","applicationStatus":"done","type":"docker","image":"nginx"}`
	composeOne := `{"composeId":"c1","name":"stack","environmentId":"e1","composeType":"raw","type":"raw","composeFile":"services: {}","composeStatus":"done"}`
	postgresOne := `{"postgresId":"pg1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","databasePassword":"DB-PASSWORD","applicationStatus":"done"}`
	redisOne := `{"redisId":"r1","name":"cache","environmentId":"e1","databasePassword":"REDIS-PASSWORD","applicationStatus":"done"}`
	return []lifecycleRequest{
		r(http.MethodPost, "/api/project.create", nil, `{"name":"demo"}`, `{"project":{"projectId":"p1","name":"demo"},"environment":{"environmentId":"e1","name":"production","isDefault":true}}`),
		r(http.MethodPost, "/api/environment.create", nil, `{"name":"staging","projectId":"p1"}`, `{"environmentId":"e2","projectId":"p1","name":"staging","isDefault":false}`),
		r(http.MethodPost, "/api/application.create", nil, `{"environmentId":"e1","name":"app"}`, `{"applicationId":"a1"}`),
		r(http.MethodPost, "/api/application.saveDockerProvider", nil, `{"applicationId":"a1","dockerImage":"nginx","password":"APP-DOCKER-PASSWORD","registryUrl":"","username":""}`, trueResponse),
		r(http.MethodPost, "/api/application.saveEnvironment", nil, `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, trueResponse),
		r(http.MethodPost, "/api/application.deploy", nil, `{"applicationId":"a1"}`, `"running"`),
		get("/api/application.one", "applicationId", "a1", appOne),
		r(http.MethodPost, "/api/compose.create", nil, `{"composeFile":"services: {}","composeType":"docker-compose","environmentId":"e1","name":"stack"}`, `{"composeId":"c1"}`),
		r(http.MethodPost, "/api/compose.update", nil, `{"composeId":"c1","composeFile":"services: {}","sourceType":"raw"}`, `{}`),
		r(http.MethodPost, "/api/compose.saveEnvironment", nil, `{"composeId":"c1","env":"COMPOSE-ENV-SECRET"}`, trueResponse),
		r(http.MethodPost, "/api/compose.deploy", nil, `{"composeId":"c1"}`, `"running"`),
		get("/api/compose.one", "composeId", "c1", composeOne),
		r(http.MethodPost, "/api/postgres.create", nil, `{"databaseName":"app","databasePassword":"DB-PASSWORD","databaseUser":"app","description":null,"dockerImage":"","environmentId":"e1","name":"db","serverId":null}`, `{"postgresId":"pg1"}`),
		r(http.MethodPost, "/api/postgres.deploy", nil, `{"postgresId":"pg1"}`, `"running"`),
		get("/api/postgres.one", "postgresId", "pg1", postgresOne),
		r(http.MethodPost, "/api/redis.create", nil, `{"databasePassword":"REDIS-PASSWORD","description":null,"dockerImage":"","environmentId":"e1","name":"cache","serverId":null}`, `{"redisId":"r1"}`),
		r(http.MethodPost, "/api/redis.deploy", nil, `{"redisId":"r1"}`, `"running"`),
		get("/api/redis.one", "redisId", "r1", redisOne),
		r(http.MethodPost, "/api/domain.create", nil, `{"applicationId":"a1","certificateType":"letsencrypt","domainType":"application","host":"app.example.com","https":false,"stripPath":false}`, `{"domainId":"d-app"}`),
		r(http.MethodPost, "/api/domain.update", nil, `{"domainId":"d-app","enabled":false}`, trueResponse),
		r(http.MethodPost, "/api/domain.create", nil, `{"certificateType":"letsencrypt","composeId":"c1","domainType":"compose","host":"stack.example.com","https":false,"serviceName":"web","stripPath":false}`, `{"domainId":"d-compose"}`),
		r(http.MethodPost, "/api/domain.update", nil, `{"domainId":"d-compose","enabled":false}`, trueResponse),
		get("/api/project.one", "projectId", "p1", `{"projectId":"p1","name":"demo","defaultEnvironmentId":"e1"}`),
		get("/api/environment.one", "environmentId", "e2", `{"environmentId":"e2","projectId":"p1","name":"staging","isDefault":false}`),
		get("/api/application.one", "applicationId", "a1", appOne), get("/api/compose.one", "composeId", "c1", composeOne), get("/api/postgres.one", "postgresId", "pg1", postgresOne), get("/api/redis.one", "redisId", "r1", redisOne),
		get("/api/domain.one", "domainId", "d-app", `{"domainId":"d-app","host":"app.example.com","applicationId":"a1","enabled":false,"https":false,"certificateType":"letsencrypt"}`),
		get("/api/domain.one", "domainId", "d-compose", `{"domainId":"d-compose","host":"stack.example.com","composeId":"c1","serviceName":"web","enabled":false,"https":false,"certificateType":"letsencrypt"}`),
		r(http.MethodPost, "/api/project.update", nil, `{"projectId":"p1","name":"renamed","description":null}`, `{}`),
		r(http.MethodPost, "/api/application.saveDockerProvider", nil, `{"applicationId":"a1","dockerImage":"alpine","password":"","registryUrl":"","username":""}`, trueResponse),
		r(http.MethodPost, "/api/application.saveEnvironment", nil, `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, trueResponse),
		r(http.MethodPost, "/api/application.redeploy", nil, `{"applicationId":"a1"}`, `"running"`), get("/api/application.one", "applicationId", "a1", appOne),
		r(http.MethodPost, "/api/application.saveDockerProvider", nil, `{"applicationId":"a1","dockerImage":"broken","password":"APP-UPDATE-PASSWORD","registryUrl":"","username":""}`, trueResponse),
		r(http.MethodPost, "/api/application.saveEnvironment", nil, `{"applicationId":"a1","buildArgs":"ARGS-SECRET","buildSecrets":"BUILD-SECRET","createEnvFile":false,"env":"ENV-SECRET"}`, trueResponse),
		{method: http.MethodPost, path: "/api/application.redeploy", body: json.RawMessage(`{"applicationId":"a1"}`), status: http.StatusBadRequest, responseFunc: secretEcho},
		get("/api/postgres.one", "postgresId", "pg1", postgresOne),
		{method: http.MethodPost, path: "/api/postgres.update", body: json.RawMessage(`{"databaseName":"app-updated","databasePassword":"DB-UPDATE-PASSWORD","databaseUser":"app","description":null,"dockerImage":"","name":"db","postgresId":"pg1"}`), status: http.StatusBadRequest, responseFunc: secretEcho},
		{method: http.MethodPost, path: "/api/redis.update", body: json.RawMessage(`{"databasePassword":"REDIS-UPDATE-PASSWORD","description":null,"dockerImage":"redis:9","name":"cache","redisId":"r1"}`), status: http.StatusBadRequest, responseFunc: secretEcho},
		r(http.MethodPost, "/api/domain.delete", nil, `{"domainId":"d-compose"}`, trueResponse), r(http.MethodPost, "/api/domain.delete", nil, `{"domainId":"d-app"}`, trueResponse),
		r(http.MethodPost, "/api/application.delete", nil, `{"applicationId":"a1"}`, trueResponse), r(http.MethodPost, "/api/compose.delete", nil, `{"composeId":"c1","deleteVolumes":false}`, trueResponse),
		r(http.MethodPost, "/api/redis.remove", nil, `{"redisId":"r1"}`, trueResponse), r(http.MethodPost, "/api/postgres.remove", nil, `{"postgresId":"pg1"}`, trueResponse),
		r(http.MethodPost, "/api/environment.remove", nil, `{"environmentId":"e2"}`, trueResponse), r(http.MethodPost, "/api/project.remove", nil, `{"projectId":"p1"}`, trueResponse),
	}
}
