package dokploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

type lookupFaultBody struct {
	closed bool
	err    error
}

func (b *lookupFaultBody) Read([]byte) (int, error) { return 0, b.err }
func (b *lookupFaultBody) Close() error             { b.closed = true; return nil }

func TestLookupDecodeBodyAndResponseSafety(t *testing.T) {
	_, err := lookupDecode[lookupProject]("project.one", nil, nil)
	require.ErrorContains(t, err, "invalid response contract")
	_, err = lookupDecode[lookupProject]("project.one", &http.Response{StatusCode: 200}, nil)
	require.ErrorContains(t, err, "invalid response contract")
	for _, tc := range []struct {
		status     int
		requestErr error
		category   string
	}{
		{200, nil, "request failed"},
		{404, nil, "not found"},
		{200, errors.New("mock-secret"), "request failed"},
	} {
		body := &lookupFaultBody{err: errors.New("mock-secret read failure")}
		_, err := lookupDecode[lookupProject]("project.one", &http.Response{StatusCode: tc.status, Body: body}, tc.requestErr)
		require.ErrorContains(t, err, tc.category)
		require.NotContains(t, err.Error(), "mock-secret")
		require.True(t, body.closed)
	}
	body := io.NopCloser(strings.NewReader(`{"projectId":"id","name":"ok"}`))
	obj, err := lookupDecode[lookupProject]("project.one", &http.Response{StatusCode: 200, Body: body}, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", *obj.Name)
}

func TestLookupBodyReadContextCategories(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cause    error
		category string
	}{
		{"canceled", context.Canceled, "request canceled"},
		{"deadline", context.DeadlineExceeded, "request deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &lookupFaultBody{err: fmt.Errorf("mock-secret reader detail: %w", tc.cause)}
			_, err := lookupDecode[lookupProject]("project.one", &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
			require.ErrorIs(t, err, tc.cause)
			require.ErrorContains(t, err, tc.category)
			require.NotContains(t, err.Error(), "mock-secret")
			require.True(t, body.closed)
		})
	}
}

func TestLookupBodyReadCancellationAfterHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cause    error
		category string
	}{
		{"canceled", context.Canceled, "request canceled"},
		{"deadline", context.DeadlineExceeded, "request deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flushed := make(chan struct{})
			release := make(chan struct{})
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/project.one" || r.URL.Query().Get("projectId") != "id" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"projectId":"id","name":"`)
				w.(http.Flusher).Flush()
				close(flushed)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer s.Close()
			defer close(release)
			api, err := client.New(s.URL, "mock-api-key")
			require.NoError(t, err)
			var ctx context.Context
			var cancel context.CancelFunc
			if tc.cause == context.Canceled {
				ctx, cancel = context.WithCancel(t.Context())
			} else {
				ctx, cancel = context.WithTimeout(t.Context(), time.Second)
			}
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, invokeErr := (GetProject{client: fixedClient(api)}).Invoke(ctx, infer.FunctionRequest[GetProjectArgs]{Input: GetProjectArgs{ProjectID: "id"}})
				result <- invokeErr
			}()
			select {
			case <-flushed:
			case <-time.After(3 * time.Second):
				t.Fatal("response headers were not flushed")
			}
			if tc.cause == context.Canceled {
				cancel()
			}
			select {
			case invokeErr := <-result:
				require.ErrorIs(t, invokeErr, tc.cause)
				require.ErrorContains(t, invokeErr, tc.category)
				require.NotContains(t, invokeErr.Error(), s.URL)
				require.NotContains(t, invokeErr.Error(), "mock-api-key")
			case <-time.After(3 * time.Second):
				t.Fatal("body read did not terminate")
			}
			require.Equal(t, int32(1), calls.Load(), "must not retry or discover")
		})
	}
}

func TestLookupExcludedFieldsDoNotBlockInvokes(t *testing.T) {
	for _, tc := range []struct {
		token, id, operation, name string
		metadata                   string
		want                       map[string]any
	}{
		{"getProject", "projectId", "project.one", "name", `"description":"desc","defaultEnvironmentId":"env"`, map[string]any{"description": "desc", "defaultEnvironmentId": "env"}},
		{"getEnvironment", "environmentId", "environment.one", "name", `"description":"desc","projectId":"project","isDefault":false`, map[string]any{"description": "desc", "projectId": "project", "isDefault": false}},
		{"getApplication", "applicationId", "application.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","applicationStatus":"future","registryId":"registry","buildRegistryId":"build"`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "status": "future", "registryId": "registry", "buildRegistryId": "build"}},
		{"getCompose", "composeId", "compose.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","composeStatus":"future","composeType":"future-type"`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "status": "future", "composeType": "future-type"}},
		{"getPostgres", "postgresId", "postgres.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"","image":"legacy","applicationStatus":"future","externalPort":0,"databaseName":"db","databaseUser":"user"`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "", "status": "future", "externalPort": float64(0), "databaseName": "db", "databaseUser": "user"}},
		{"getMySQL", "mysqlId", "mysql.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"","image":"legacy","applicationStatus":"future","externalPort":0,"databaseName":"db","databaseUser":"user"`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "", "status": "future", "externalPort": float64(0), "databaseName": "db", "databaseUser": "user"}},
		{"getMariaDB", "mariadbId", "mariadb.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"","image":"legacy","applicationStatus":"future","externalPort":0,"databaseName":"db","databaseUser":"user"`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "", "status": "future", "externalPort": float64(0), "databaseName": "db", "databaseUser": "user"}},
		{"getMongoDB", "mongoId", "mongo.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"","image":"legacy","applicationStatus":"future","externalPort":0,"databaseUser":"user","replicaSets":false`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "", "status": "future", "externalPort": float64(0), "databaseUser": "user", "replicaSets": false}},
		{"getRedis", "redisId", "redis.one", "name", `"description":"desc","appName":"app","environmentId":"env","serverId":"server","dockerImage":"","image":"legacy","applicationStatus":"future","externalPort":0`, map[string]any{"description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "dockerImage": "", "status": "future", "externalPort": float64(0)}},
		{"getServer", "serverId", "server.one", "name", `"description":"desc","ipAddress":"127.0.0.1","port":0,"username":"user","organizationId":"org","sshKeyId":"ssh","serverType":"future","serverStatus":"future"`, map[string]any{"description": "desc", "ipAddress": "127.0.0.1", "port": float64(0), "username": "user", "organizationId": "org", "sshKeyId": "ssh", "serverType": "future", "status": "future"}},
		{"getRegistry", "registryId", "registry.one", "registryName", `"registryUrl":"registry.example","username":"user","imagePrefix":"prefix","serverId":"server","registryType":"future"`, map[string]any{"url": "registry.example", "username": "user", "imagePrefix": "prefix", "serverId": "server", "registryType": "future"}},
		{"getSSHKey", "sshKeyId", "sshKey.one", "name", `"description":"desc","publicKey":"public","organizationId":"org"`, map[string]any{"description": "desc", "publicKey": "public", "organizationId": "org"}},
	} {
		t.Run(tc.token, func(t *testing.T) {
			body := fmt.Sprintf(`{"%s":"id","%s":"",%s,"env":42,"buildArgs":false,"buildSecrets":[7],"createEnvFile":{"password":"mock-secret"},"source":17,"composeFile":false,"environment":true,"password":{"token":"mock-secret"},"privateKey":3,"unknown":{"nested":true}}`, tc.id, tc.name, tc.metadata)
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.id: {"id"}}, Status: 200, Response: []byte(body)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			out, err := provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(map[string]property.Value{tc.id: property.New("id")})})
			require.NoError(t, err)
			require.Empty(t, out.Failures)
			want := map[string]any{tc.id: "id", "name": ""}
			for k, v := range tc.want {
				want[k] = v
			}
			require.Equal(t, len(want), out.Return.Len())
			for k, v := range want {
				got := out.Return.Get(k)
				switch value := v.(type) {
				case string:
					require.Equal(t, value, got.AsString())
				case bool:
					require.Equal(t, value, got.AsBool())
				case float64:
					require.Equal(t, value, got.AsNumber())
				}
			}
		})
	}
}

func TestLookupIsolatedDecodeContracts(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `"secret"`, `{"projectId":"id","name":"ok","description":7}`, `{"projectId":"id","name":"ok"} {"token":"mock-secret"}`, `{"projectId":"id","name":"ok","description":"mock-secret"`} {
		t.Run(body, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/project.one", Query: url.Values{"projectId": {"id"}}, Status: 200, Response: []byte(body)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			_, err = provider.Invoke(p.InvokeRequest{Token: "dokploy:index:getProject", Args: property.NewMap(map[string]property.Value{"projectId": property.New("id")})})
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid response contract")
			require.False(t, strings.Contains(err.Error(), "mock-secret"))
		})
	}
}

func TestLookupConsumedFieldTypesAreStrict(t *testing.T) {
	for _, tc := range []struct{ token, id, operation, name, field string }{
		{"getProject", "projectId", "project.one", "name", "defaultEnvironmentId"},
		{"getEnvironment", "environmentId", "environment.one", "name", "isDefault"},
		{"getApplication", "applicationId", "application.one", "name", "registryId"},
		{"getCompose", "composeId", "compose.one", "name", "composeStatus"},
		{"getPostgres", "postgresId", "postgres.one", "name", "dockerImage"},
		{"getMySQL", "mysqlId", "mysql.one", "name", "externalPort"},
		{"getMariaDB", "mariadbId", "mariadb.one", "name", "applicationStatus"},
		{"getMongoDB", "mongoId", "mongo.one", "name", "replicaSets"},
		{"getRedis", "redisId", "redis.one", "name", "image"},
		{"getServer", "serverId", "server.one", "name", "port"},
		{"getRegistry", "registryId", "registry.one", "registryName", "imagePrefix"},
		{"getSSHKey", "sshKeyId", "sshKey.one", "name", "publicKey"},
	} {
		t.Run(tc.token, func(t *testing.T) {
			body := fmt.Sprintf(`{"%s":"id","%s":"ok","%s":{"token":"mock-secret"}}`, tc.id, tc.name, tc.field)
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.id: {"id"}}, Status: 200, Response: []byte(body)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			_, err = provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(map[string]property.Value{tc.id: property.New("id")})})
			require.ErrorContains(t, err, "invalid response contract")
			require.NotContains(t, err.Error(), "mock-secret")
		})
	}
}
