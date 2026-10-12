package dokploy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blang/semver"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestLiveLookupHelperExercisesOwnedIdentity(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/project.one", r.URL.Path)
		require.Equal(t, url.Values{"projectId": {"owned-id"}}, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"projectId":"owned-id","name":"owned-name","description":"metadata","password":{"malformed":"private"}}`)
	}))
	defer server.Close()
	api, err := client.New(server.URL, "offline-key")
	require.NoError(t, err)
	runLiveLookup(t, t.Context(), api, "Project", "owned-id", "owned-name")
	require.Equal(t, int32(1), calls.Load(), "helper must issue the lookup")
}

func TestLiveLookupInvokesAllFamiliesRepeatAfterFailure(t *testing.T) {
	for _, tc := range []struct {
		family, token, idField, operation, nameField, metadata string
		wantMetadata                                           map[string]property.Value
	}{
		{"Project", "getProject", "projectId", "project.one", "name", `"description":"safe"`, map[string]property.Value{"description": property.New("safe")}},
		{"Environment", "getEnvironment", "environmentId", "environment.one", "name", `"isDefault":false`, map[string]property.Value{"isDefault": property.New(false)}},
		{"Application", "getApplication", "applicationId", "application.one", "name", `"applicationStatus":"future"`, map[string]property.Value{"status": property.New("future")}},
		{"Compose", "getCompose", "composeId", "compose.one", "name", `"composeStatus":"future"`, map[string]property.Value{"status": property.New("future")}},
		{"Postgres", "getPostgres", "postgresId", "postgres.one", "name", `"externalPort":0`, map[string]property.Value{"externalPort": property.New(float64(0))}},
		{"MySQL", "getMySQL", "mysqlId", "mysql.one", "name", `"externalPort":0`, map[string]property.Value{"externalPort": property.New(float64(0))}},
		{"MariaDB", "getMariaDB", "mariadbId", "mariadb.one", "name", `"externalPort":0`, map[string]property.Value{"externalPort": property.New(float64(0))}},
		{"MongoDB", "getMongoDB", "mongoId", "mongo.one", "name", `"replicaSets":false`, map[string]property.Value{"replicaSets": property.New(false)}},
		{"Redis", "getRedis", "redisId", "redis.one", "name", `"externalPort":0`, map[string]property.Value{"externalPort": property.New(float64(0))}},
		{"Server", "getServer", "serverId", "server.one", "name", `"port":0`, map[string]property.Value{"port": property.New(float64(0))}},
		{"Registry", "getRegistry", "registryId", "registry.one", "registryName", `"imagePrefix":"safe","registryUrl":"https://user:private@registry.invalid"`, map[string]property.Value{"imagePrefix": property.New("safe")}},
		{"SSHKey", "getSSHKey", "sshKeyId", "sshKey.one", "name", `"publicKey":"safe"`, map[string]property.Value{"publicKey": property.New("safe")}},
	} {
		t.Run(tc.family, func(t *testing.T) {
			const id, name = "owned-offline-id", "owned-offline-name"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/"+tc.operation, r.URL.Path)
				require.Equal(t, url.Values{tc.idField: {id}}, r.URL.Query())
				require.Equal(t, "offline-key", r.Header.Get("x-api-key"))
				w.Header().Set("Content-Type", "application/json")
				if call == 1 || call == 6 {
					status := http.StatusUnauthorized
					if call == 6 {
						status = http.StatusNotFound
					}
					w.WriteHeader(status)
					_, _ = fmt.Fprint(w, `{"message":"private-response-sentinel"}`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"%s":%q,"%s":%q,%s,"env":42,"buildArgs":false,"buildSecrets":[7],"source":17,"composeFile":false,"command":true,"environment":true,"databasePassword":{"private":"sentinel"},"password":{"token":"private-response-sentinel"},"privateKey":3,"unknown":{"nested":true}}`, tc.idField, id, tc.nameField, name, tc.metadata)
			}))
			defer server.Close()
			srv, err := integration.NewServer(t.Context(), Name, semver.Version{}, integration.WithProvider(Provider()))
			require.NoError(t, err)
			require.NoError(t, srv.Configure(p.ConfigureRequest{Args: property.NewMap(map[string]property.Value{
				"endpoint": property.New(server.URL), "apiKey": property.New("offline-key"),
			})}))
			invoke := func() (p.InvokeResponse, error) {
				return srv.Invoke(p.InvokeRequest{Token: tokens.Type("dokploy:index:" + tc.token), Args: property.NewMap(map[string]property.Value{tc.idField: property.New(id)})})
			}
			_, err = invoke()
			require.ErrorContains(t, err, "authentication")
			for _, sentinel := range []string{id, name, "offline-key", server.URL, "private-response-sentinel"} {
				require.NotContains(t, err.Error(), sentinel)
			}
			for range 2 {
				out, err := invoke()
				require.NoError(t, err, "preceding independent failure must not poison invoke")
				require.Empty(t, out.Failures)
				require.NoError(t, validateLiveLookup(tc.family, out.Return, id, name))
				want := map[string]property.Value{tc.idField: property.New(id), "name": property.New(name)}
				for key, value := range tc.wantMetadata {
					want[key] = value
				}
				require.Equal(t, property.NewMap(want), out.Return)
			}
			api, err := client.New(server.URL, "offline-key")
			require.NoError(t, err)
			for range 2 {
				runLiveLookup(t, t.Context(), api, tc.family, id, name)
			}
			_, err = invoke()
			require.ErrorContains(t, err, "not found")
			require.NotContains(t, err.Error(), "private-response-sentinel")
			require.Equal(t, int32(6), calls.Load(), "lookup must not retry or discover identities")
		})
	}
}

func TestLiveLookupContractRejectsIdentityMismatchAndExcludedFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output map[string]property.Value
	}{
		{"wrong identity", map[string]property.Value{"projectId": property.New("private-id"), "name": property.New("owned")}},
		{"wrong name", map[string]property.Value{"projectId": property.New("owned-id"), "name": property.New("private-name")}},
		{"excluded secret", map[string]property.Value{"projectId": property.New("owned-id"), "name": property.New("owned"), "password": property.New("private-password")}},
		{"unknown nested", map[string]property.Value{"projectId": property.New("owned-id"), "name": property.New("owned"), "unknown": property.New(property.NewMap(map[string]property.Value{"private": property.New("private-value")}))}},
		{"malformed metadata", map[string]property.Value{"projectId": property.New("owned-id"), "name": property.New("owned"), "description": property.New(false)}},
		{"secret metadata", map[string]property.Value{"projectId": property.New("owned-id"), "name": property.New("owned"), "description": property.New("private-description").WithSecret(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLiveLookup("Project", property.NewMap(tc.output), "owned-id", "owned")
			require.Error(t, err)
			require.False(t, strings.Contains(err.Error(), "private-"), "contract failure must not leak values")
		})
	}
}
