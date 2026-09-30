package dokploy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/blang/semver"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestLookupAllFamiliesInvalidIDsAndOpaqueRoundTrip(t *testing.T) {
	for _, tc := range []struct{ token, field, operation, nameField string }{
		{"getProject", "projectId", "project.one", "name"},
		{"getEnvironment", "environmentId", "environment.one", "name"},
		{"getApplication", "applicationId", "application.one", "name"},
		{"getCompose", "composeId", "compose.one", "name"},
		{"getPostgres", "postgresId", "postgres.one", "name"},
		{"getMySQL", "mysqlId", "mysql.one", "name"},
		{"getMariaDB", "mariadbId", "mariadb.one", "name"},
		{"getMongoDB", "mongoId", "mongo.one", "name"},
		{"getRedis", "redisId", "redis.one", "name"},
		{"getServer", "serverId", "server.one", "name"},
		{"getRegistry", "registryId", "registry.one", "registryName"},
		{"getSSHKey", "sshKeyId", "sshKey.one", "name"},
	} {
		t.Run(tc.token, func(t *testing.T) {
			const id = " opaque+/percent% "
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/"+tc.operation, r.URL.Path)
				require.Equal(t, url.Values{tc.field: {id}}, r.URL.Query())
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"%s":%q,"%s":""}`, tc.field, id, tc.nameField)
			}))
			defer s.Close()
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.URL), "apiKey": property.New("mock-api-key")})}))
			for _, args := range []map[string]property.Value{{}, {tc.field: property.New(float64(7))}, {tc.field: property.New("")}, {tc.field: property.New(" \t")}} {
				out, err := provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(args)})
				require.True(t, err != nil || len(out.Failures) != 0)
				require.Zero(t, calls, "invalid input must not make HTTP requests")
			}
			out, err := provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(map[string]property.Value{tc.field: property.New(id)})})
			require.NoError(t, err)
			require.Empty(t, out.Failures)
			require.Equal(t, id, out.Return.Get(tc.field).AsString())
			require.Equal(t, 1, calls)
		})
	}
}

func TestLookupDatabaseInvoke(t *testing.T) {
	for _, tc := range []struct {
		token, operation, field string
		optional                []string
	}{
		{"getPostgres", "postgres.one", "postgresId", []string{"databaseName", "databaseUser"}},
		{"getMySQL", "mysql.one", "mysqlId", []string{"databaseName", "databaseUser"}},
		{"getMariaDB", "mariadb.one", "mariadbId", []string{"databaseName", "databaseUser"}},
		{"getMongoDB", "mongo.one", "mongoId", []string{"databaseUser", "replicaSets"}},
		{"getRedis", "redis.one", "redisId", nil},
	} {
		for _, variant := range []struct {
			name, image string
			wantImage   *string
		}{
			{"canonical empty", `"dockerImage":"","image":"legacy",`, stringPtr("")},
			{"legacy null", `"dockerImage":null,"image":"legacy",`, stringPtr("legacy")},
			{"no image", ``, nil},
		} {
			t.Run(tc.token+"/"+variant.name, func(t *testing.T) {
				body := fmt.Sprintf(`{"%s":"id","name":"","description":"","appName":"","environmentId":"","serverId":"",%s"applicationStatus":"future","externalPort":0,"databaseName":"","databaseUser":"","replicaSets":false,"databasePassword":"mock-secret","env":"mock-secret","source":{"password":"mock-secret"}}`, tc.field, variant.image)
				s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.field: {"id"}}, Status: 200, Response: []byte(body)})
				provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
				require.NoError(t, err)
				require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
				response, err := provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(map[string]property.Value{tc.field: property.New("id")})})
				require.NoError(t, err)
				require.Empty(t, response.Failures)
				keys := []string{tc.field, "name", "description", "appName", "environmentId", "serverId", "status", "externalPort"}
				keys = append(keys, tc.optional...)
				if variant.wantImage != nil {
					keys = append(keys, "dockerImage")
				}
				var actual []string
				for key := range response.Return.All {
					actual = append(actual, key)
				}
				require.ElementsMatch(t, keys, actual)
				require.Equal(t, "future", response.Return.Get("status").AsString())
				require.Equal(t, float64(0), response.Return.Get("externalPort").AsNumber())
				if variant.wantImage != nil {
					require.Equal(t, *variant.wantImage, response.Return.Get("dockerImage").AsString())
				}
				if tc.token == "getMongoDB" {
					require.False(t, response.Return.Get("replicaSets").AsBool())
					require.Equal(t, float64(0), response.Return.Get("externalPort").AsNumber())
					_, hasPassword := response.Return.GetOk("databasePassword")
					require.False(t, hasPassword)
				}
			})
		}
	}
}

func TestLookupControlPlaneInvoke(t *testing.T) {
	for _, tc := range []struct {
		suffix, field, operation, payload string
		keys                              []string
	}{
		{"getProject", "projectId", "project.one", `{"projectId":"id","name":"","description":"","defaultEnvironmentId":null,"password":"mock-secret"}`, []string{"projectId", "name", "description"}},
		{"getEnvironment", "environmentId", "environment.one", `{"environmentId":"id","name":"","isDefault":false,"description":null,"password":"mock-secret"}`, []string{"environmentId", "name", "isDefault"}},
		{"getServer", "serverId", "server.one", `{"serverId":"id","name":"","port":0,"username":"","password":"mock-secret"}`, []string{"serverId", "name", "port", "username"}},
		{"getRegistry", "registryId", "registry.one", `{"registryId":"id","registryName":"","registryUrl":"https://user:secret@registry.example","imagePrefix":"","password":"mock-secret"}`, []string{"registryId", "name", "imagePrefix"}},
		{"getSSHKey", "sshKeyId", "sshKey.one", `{"sshKeyId":"id","name":"","publicKey":"","privateKey":"mock-secret"}`, []string{"sshKeyId", "name", "publicKey"}},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.field: {"id"}}, Status: 200, Response: []byte(tc.payload)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			invoke := func(args map[string]property.Value) (p.InvokeResponse, error) {
				return provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.suffix), Args: property.NewMap(args)})
			}
			out, err := invoke(map[string]property.Value{tc.field: property.New("id")})
			require.NoError(t, err)
			require.Empty(t, out.Failures)
			keys := make([]string, 0, out.Return.Len())
			for key := range out.Return.All {
				keys = append(keys, key)
			}
			require.ElementsMatch(t, tc.keys, keys)
			for _, key := range tc.keys {
				if key != tc.field {
					require.False(t, out.Return.Get(key).IsNull(), key)
				}
			}
			for _, bad := range []map[string]property.Value{{}, {tc.field: property.New(7.0)}, {tc.field: property.New("")}, {tc.field: property.New("  \t")}} {
				out, err := invoke(bad)
				require.True(t, err != nil || len(out.Failures) > 0)
				if err != nil {
					require.NotContains(t, err.Error(), "mock-api-key")
					require.NotContains(t, err.Error(), s.server.URL)
				}
			}
			_, err = provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:unknownLookup"), Args: property.NewMap(map[string]property.Value{tc.field: property.New("id")})})
			require.Error(t, err)
		})
	}
}

func TestLookupWorkloadInvoke(t *testing.T) {
	for _, tc := range []struct {
		name, field, operation, payload string
		want                            map[string]string
	}{
		{"getApplication", "applicationId", "application.one", `{"applicationId":"id","name":"","description":"desc","appName":"app","environmentId":"env","serverId":"server","applicationStatus":"future","registryId":"registry","buildRegistryId":"build","env":"mock-secret","source":{"token":"mock-secret"}}`, map[string]string{"applicationId": "id", "name": "", "description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "status": "future", "registryId": "registry", "buildRegistryId": "build"}},
		{"getApplication/empty", "applicationId", "application.one", `{"applicationId":"id","name":"","applicationStatus":"","registryId":"","buildRegistryId":null,"env":"mock-secret"}`, map[string]string{"applicationId": "id", "name": "", "status": "", "registryId": ""}},
		{"getApplication/minimal", "applicationId", "application.one", `{"applicationId":"id","name":"","applicationStatus":null}`, map[string]string{"applicationId": "id", "name": ""}},
		{"getCompose", "composeId", "compose.one", `{"composeId":"id","name":"","description":"desc","appName":"app","environmentId":"env","serverId":"server","composeStatus":"future","composeType":"future-type","composeFile":"mock-secret","command":"mock-secret","source":{"password":"mock-secret"}}`, map[string]string{"composeId": "id", "name": "", "description": "desc", "appName": "app", "environmentId": "env", "serverId": "server", "status": "future", "composeType": "future-type"}},
		{"getCompose/empty", "composeId", "compose.one", `{"composeId":"id","name":"","composeStatus":"","composeType":"","composeFile":"mock-secret"}`, map[string]string{"composeId": "id", "name": "", "status": "", "composeType": ""}},
		{"getCompose/minimal", "composeId", "compose.one", `{"composeId":"id","name":"","composeStatus":null,"composeType":null}`, map[string]string{"composeId": "id", "name": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + tc.operation, Query: url.Values{tc.field: {"id"}}, Status: 200, Response: []byte(tc.payload)})
			provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key")})}))
			token := "getCompose"
			if tc.field == "applicationId" {
				token = "getApplication"
			}
			response, err := provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + token), Args: property.NewMap(map[string]property.Value{tc.field: property.New("id")})})
			require.NoError(t, err)
			require.Empty(t, response.Failures)
			require.Equal(t, len(tc.want), response.Return.Len())
			for key, value := range tc.want {
				got, ok := response.Return.GetOk(key)
				require.True(t, ok, key)
				require.True(t, got.IsString(), key)
				require.Equal(t, value, got.AsString(), key)
			}
			for _, key := range []string{"environment", "env", "source", "composeFile", "command", "password", "buildArgs", "token", "buildRegistryId", "status", "composeType"} {
				if _, expected := tc.want[key]; !expected {
					_, present := response.Return.GetOk(key)
					require.False(t, present, key)
				}
			}
		})
	}
}

func TestLookupControlPlaneHTTPErrorCategories(t *testing.T) {
	for _, lookup := range []struct{ token, field, operation string }{
		{"getProject", "projectId", "project.one"},
		{"getEnvironment", "environmentId", "environment.one"},
		{"getServer", "serverId", "server.one"},
		{"getRegistry", "registryId", "registry.one"},
		{"getSSHKey", "sshKeyId", "sshKey.one"},
	} {
		for _, failure := range []struct {
			name, category, code string
			status               int
		}{
			{"unauthorized", "authentication", "UNAUTHORIZED", http.StatusUnauthorized},
			{"forbidden", "authorization", "FORBIDDEN", http.StatusForbidden},
			{"not found", "not found", "NOT_FOUND", http.StatusNotFound},
			{"API not found", "not found", "NOT_FOUND", http.StatusBadRequest},
		} {
			t.Run(lookup.token+"/"+failure.name, func(t *testing.T) {
				const requestedID = "mock-resource-id"
				const responseSecret = "mock-private-key-and-token"
				body := mustJSON(map[string]string{"code": failure.code, "message": responseSecret + " https://private.example/hidden"})
				s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/" + lookup.operation,
					Query: url.Values{lookup.field: {requestedID}}, Status: failure.status, Response: body})
				provider, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
				require.NoError(t, err)
				require.NoError(t, provider.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
					"endpoint": property.New(s.server.URL), "apiKey": property.New("mock-api-key"),
				})}))
				_, err = provider.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + lookup.token),
					Args: property.NewMap(map[string]property.Value{lookup.field: property.New(requestedID)})})
				require.Error(t, err)
				require.Contains(t, err.Error(), lookup.operation)
				require.Contains(t, err.Error(), failure.category)
				for _, unsafe := range []string{requestedID, responseSecret, "private.example", "mock-api-key", s.server.URL} {
					require.NotContains(t, err.Error(), unsafe)
				}
			})
		}
	}
}
