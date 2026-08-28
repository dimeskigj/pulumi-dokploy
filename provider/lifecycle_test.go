package dokploy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	api := newLifecycleAPI(t)
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
		"source": property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("nginx")})}),
	})
	compose := lifecycleCreate(t, provider, "dokploy:index:Compose", "compose", map[string]property.Value{
		"name": property.New("stack"), "environmentId": property.New(environmentID),
		"source": property.New(property.NewMap(map[string]property.Value{"type": property.New("raw"), "raw": property.New(property.NewMap(map[string]property.Value{"composeFile": property.New("services: {}")}))})),
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
	api.FailNextRedeploy()
	_, err = provider.Update(p.UpdateRequest{ID: application.ID, Urn: lifecycleURN("Application", "application"), State: application.Properties,
		OldInputs: application.Properties, Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("app"), "environmentId": property.New(environmentID),
			"environment": property.New("ENV-SECRET"), "buildArgs": property.New("ARGS-SECRET"), "buildSecrets": property.New("BUILD-SECRET"),
			"source": property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("broken")})}),
		})})
	require.Error(t, err)
	for _, secret := range []string{"ENV-SECRET", "ARGS-SECRET", "BUILD-SECRET"} {
		require.NotContains(t, err.Error(), secret)
	}

	imported, err := provider.Read(p.ReadRequest{ID: postgres.ID, Urn: lifecycleURN("Postgres", "postgres")})
	require.NoError(t, err)
	require.Equal(t, postgres.ID, imported.ID)

	for _, r := range []struct {
		response p.CreateResponse
		kind     string
	}{{composeDomain, "Domain"}, {applicationDomain, "Domain"}, {application, "Application"}, {compose, "Compose"}, {redis, "Redis"}, {postgres, "Postgres"}, {environment, "Environment"}, {project, "Project"}} {
		require.NoError(t, provider.Delete(p.DeleteRequest{ID: r.response.ID, Urn: lifecycleURN(r.kind, "delete"), Properties: r.response.Properties}))
	}

	api.AssertRequests(t)
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
	server           *httptest.Server
	mu               sync.Mutex
	requests         []string
	bodies           []string
	failNextRedeploy bool
}

func newLifecycleAPI(t *testing.T) *lifecycleAPI {
	api := &lifecycleAPI{}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.mu.Lock()
		api.requests = append(api.requests, r.Method+" "+r.URL.Path)
		api.bodies = append(api.bodies, string(body))
		api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ".redeploy") && api.consumeRedeployFailure() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"redeploy failed ENV-SECRET ARGS-SECRET BUILD-SECRET"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(lifecycleResponse(r.URL.Path)))
	}))
	t.Cleanup(api.server.Close)
	return api
}

func (s *lifecycleAPI) URL() string { return s.server.URL }

func (s *lifecycleAPI) FailNextRedeploy() {
	s.mu.Lock()
	s.failNextRedeploy = true
	s.mu.Unlock()
}

func (s *lifecycleAPI) consumeRedeployFailure() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := s.failNextRedeploy
	s.failNextRedeploy = false
	return fail
}

func (s *lifecycleAPI) AssertRequests(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotEmpty(t, s.requests)
	createOrder := []string{"POST /api/project.create", "POST /api/environment.create", "POST /api/application.create", "POST /api/compose.create", "POST /api/postgres.create", "POST /api/redis.create", "POST /api/domain.create", "POST /api/domain.create"}
	last := -1
	for _, expected := range createOrder {
		at := requestIndexAfter(s.requests, expected, last+1)
		require.Greater(t, at, last, expected)
		last = at
	}
	for _, expected := range []string{"POST /api/application.deploy", "POST /api/application.redeploy", "POST /api/compose.deploy", "POST /api/postgres.deploy", "POST /api/redis.deploy"} {
		require.Contains(t, s.requests, expected)
	}
	deleteOrder := []string{"POST /api/domain.delete", "POST /api/domain.delete", "POST /api/application.delete", "POST /api/compose.delete", "POST /api/redis.remove", "POST /api/postgres.remove", "POST /api/environment.remove", "POST /api/project.remove"}
	deleteStart := len(s.requests) - len(deleteOrder)
	for i, expected := range deleteOrder {
		require.Equal(t, expected, s.requests[deleteStart+i])
	}
	requireBody(t, s.bodies[requestIndex(s.requests, "POST /api/project.create")], map[string]any{"name": "demo"})
	requireBody(t, s.bodies[requestIndex(s.requests, "POST /api/environment.create")], map[string]any{"projectId": "p1", "name": "staging"})
	require.Contains(t, strings.Join(s.bodies, "\n"), "DB-PASSWORD")
	require.Contains(t, strings.Join(s.bodies, "\n"), "REDIS-PASSWORD")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(s.bodies[0]), &decoded))
	require.Equal(t, "demo", decoded["name"])
}

func requestIndex(requests []string, expected string) int {
	return requestIndexAfter(requests, expected, 0)
}

func requestIndexAfter(requests []string, expected string, start int) int {
	for i := start; i < len(requests); i++ {
		request := requests[i]
		if request == expected {
			return i
		}
	}
	return -1
}

func requireBody(t *testing.T, raw string, expected map[string]any) {
	t.Helper()
	var actual map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &actual))
	require.Equal(t, expected, actual)
}

func lifecycleResponse(path string) string {
	switch {
	case strings.HasSuffix(path, ".deploy"), strings.HasSuffix(path, ".redeploy"):
		return `"running"`
	case strings.HasSuffix(path, ".update"):
		return `{}`
	case strings.Contains(path, "save"), strings.HasSuffix(path, ".remove"), strings.HasSuffix(path, ".delete"):
		return `true`
	case strings.HasSuffix(path, "project.create"):
		return `{"project":{"projectId":"p1","name":"demo"},"environment":{"environmentId":"e1","name":"production","isDefault":true}}`
	case strings.HasSuffix(path, "environment.create"):
		return `{"environmentId":"e2","projectId":"p1","name":"staging","isDefault":false}`
	case strings.HasSuffix(path, "application.create"):
		return `{"applicationId":"a1"}`
	case strings.HasSuffix(path, "compose.create"):
		return `{"composeId":"c1"}`
	case strings.HasSuffix(path, "postgres.create"):
		return `{"postgresId":"pg1"}`
	case strings.HasSuffix(path, "redis.create"):
		return `{"redisId":"r1"}`
	case strings.HasSuffix(path, "domain.create"):
		return `{"domainId":"d1"}`
	case strings.HasSuffix(path, "project.one"):
		return `{"projectId":"p1","name":"demo","defaultEnvironmentId":"e1"}`
	case strings.HasSuffix(path, "environment.one"):
		return `{"environmentId":"e2","projectId":"p1","name":"staging","isDefault":false}`
	case strings.HasSuffix(path, "application.one"):
		return `{"applicationId":"a1","name":"app","environmentId":"e1","status":"done","source":{"type":"docker","image":"nginx"}}`
	case strings.HasSuffix(path, "compose.one"):
		return `{"composeId":"c1","name":"stack","environmentId":"e1","composeType":"raw","source":{"type":"raw","composeFile":"services: {}"},"status":"done"}`
	case strings.HasSuffix(path, "postgres.one"):
		return `{"postgresId":"pg1","name":"db","environmentId":"e1","databaseName":"app","databaseUser":"app","databasePassword":"DB-PASSWORD","status":"done"}`
	case strings.HasSuffix(path, "redis.one"):
		return `{"redisId":"r1","name":"cache","environmentId":"e1","databasePassword":"REDIS-PASSWORD","status":"done"}`
	case strings.HasSuffix(path, "domain.one"):
		return `{"domainId":"d1","host":"app.example.com","applicationId":"a1","enabled":true,"https":true,"certificateType":"letsencrypt"}`
	default:
		return `{}`
	}
}
