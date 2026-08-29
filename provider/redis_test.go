package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestRedisCheckDefaultsImage(t *testing.T) {
	r := Redis{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("cache"), "environmentId": property.New("env"), "databasePassword": property.New("pw"),
	})})
	require.NoError(t, err)
	require.Equal(t, "redis:8", got.Inputs.DockerImage)
}

func TestRedisProviderRegistrationAndStatusValidation(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	resource := spec.Resources["dokploy:index:Redis"]
	require.NotNil(t, resource)
	require.True(t, resource.InputProperties["databasePassword"].Secret)
	require.True(t, resource.InputProperties["environment"].Secret)
	require.EqualError(t, func() error { _, err := redisStatusValue(&generated.Redis{}); return err }(), "redis.one returned redis without a status")
}

func TestRedisDiffReplacesEnvironmentAndServerOnly(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	diff, err := (Redis{}).Diff(t.Context(), infer.DiffRequest[RedisArgs, RedisState]{Inputs: RedisArgs{EnvironmentID: "e2", ServerID: &newServer}, State: RedisState{RedisArgs: RedisArgs{EnvironmentID: "e1", ServerID: &oldServer}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
}

func TestRedisCreateConfiguresBeforeDeployAndPolls(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	env, port := "REDIS_ENV", 6380
	s := newScriptedServer(t,
		expectPOST("/api/redis.create", `{"databasePassword":"password","description":null,"dockerImage":"redis:8","environmentId":"env","name":"cache","serverId":null}`, `{"redisId":"r1"}`),
		expectPOST("/api/redis.saveEnvironment", `{"env":"REDIS_ENV","redisId":"r1"}`, `true`),
		expectPOST("/api/redis.saveExternalPort", `{"externalPort":6380,"redisId":"r1"}`, `true`),
		expectPOST("/api/redis.deploy", `{"redisId":"r1"}`, `"running"`),
		expectGET("/api/redis.one", map[string][]string{"redisId": {"r1"}}, http.StatusOK, `{"redisId":"r1","status":"done"}`),
	)
	got, err := (Redis{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[RedisArgs]{Inputs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: "password", Environment: &env, ExternalPort: &port, DockerImage: "redis:8"}})
	require.NoError(t, err)
	require.Equal(t, "r1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestRedisMetadataUpdateDoesNotDeploy(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/redis.update", `{"description":null,"name":"new","redisId":"r1"}`, `{}`))
	_, err := (Redis{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RedisArgs, RedisState]{ID: "r1", Inputs: RedisArgs{Name: "new", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "redis:8"}, State: RedisState{RedisArgs: RedisArgs{Name: "old", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "redis:8"}}})
	require.NoError(t, err)
}

func TestRedisRuntimeUpdateClearsValuesAndDeploys(t *testing.T) {
	oldEnv, oldPort, oldPassword, newPassword := "OLD-ENV", 6379, "OLD-PASSWORD", "NEW-PASSWORD"
	oldImage := "redis:7"
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/redis.update", `{"databasePassword":"NEW-PASSWORD","description":null,"dockerImage":"redis:8","name":"cache","redisId":"r1"}`, `{}`),
		expectPOST("/api/redis.saveEnvironment", `{"env":null,"redisId":"r1"}`, `true`),
		expectPOST("/api/redis.saveExternalPort", `{"externalPort":null,"redisId":"r1"}`, `true`),
		expectPOST("/api/redis.deploy", `{"redisId":"r1"}`, `"running"`),
		expectGET("/api/redis.one", map[string][]string{"redisId": {"r1"}}, http.StatusOK, `{"redisId":"r1","status":"done"}`),
	)
	_, err := (Redis{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RedisArgs, RedisState]{ID: "r1", Inputs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: newPassword, DockerImage: "redis:8"}, State: RedisState{RedisArgs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: oldPassword, DockerImage: oldImage, Environment: &oldEnv, ExternalPort: &oldPort}}})
	require.NoError(t, err)
}

func TestRedisReadPreservesSecretsImportAndHandlesNotFound(t *testing.T) {
	pw, env := "PASSWORD", "ENVIRONMENT"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/redis.one", Query: map[string][]string{"redisId": {"r1"}}, Status: http.StatusOK, Response: []byte(`{"redisId":"r1","name":"cache","environmentId":"env","image":"redis:8","status":"running"}`)})
	got, err := (Redis{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[RedisArgs, RedisState]{ID: "r1", State: RedisState{RedisArgs: RedisArgs{DatabasePassword: pw, Environment: &env}}})
	require.NoError(t, err)
	require.Equal(t, pw, got.Inputs.DatabasePassword)
	require.Equal(t, env, *got.Inputs.Environment)
	nf := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/redis.one", Query: map[string][]string{"redisId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)}, scriptedRequest{Method: http.MethodPost, Path: "/api/redis.remove", Body: json.RawMessage(`{"redisId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)})
	r := Redis{client: fixedClient(nf.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[RedisArgs, RedisState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[RedisState]{ID: "missing"})
	require.NoError(t, err)
}

func TestRedisImportReconstructsObservablePassword(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/redis.one", map[string][]string{"redisId": {"r1"}}, http.StatusOK, `{"redisId":"r1","name":"cache","environmentId":"env","databasePassword":"observed-password","env":"observed-env","status":"done"}`))
	got, err := (Redis{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[RedisArgs, RedisState]{ID: "r1"})
	require.NoError(t, err)
	require.Equal(t, "observed-password", got.Inputs.DatabasePassword)
	require.Equal(t, "observed-env", *got.Inputs.Environment)
}

func TestRedisImportAllowsMissingPasswordWithoutInventingSecret(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/redis.one", map[string][]string{"redisId": {"r1"}}, http.StatusOK, `{"redisId":"r1","name":"cache","environmentId":"env","status":"done"}`))
	got, err := (Redis{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[RedisArgs, RedisState]{ID: "r1"})
	require.NoError(t, err)
	require.Empty(t, got.Inputs.DatabasePassword)
}

func TestRedisRefreshPreservesPriorPasswordWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/redis.one", map[string][]string{"redisId": {"r1"}}, http.StatusOK, `{"redisId":"r1","name":"cache","environmentId":"env","status":"done"}`))
	got, err := (Redis{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[RedisArgs, RedisState]{ID: "r1", State: RedisState{RedisArgs: RedisArgs{DatabasePassword: "prior"}}})
	require.NoError(t, err)
	require.Equal(t, "prior", got.Inputs.DatabasePassword)
}

func TestRedisPostImportPasswordIsRuntimeUpdate(t *testing.T) {
	diff, err := (Redis{}).Diff(t.Context(), infer.DiffRequest[RedisArgs, RedisState]{Inputs: RedisArgs{DatabasePassword: "supplied"}, State: RedisState{RedisArgs: RedisArgs{}}})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["databasePassword"].Kind)
}

func TestRedisPasswordErrorsRedactOldAndNewAcrossDeployAndPoll(t *testing.T) {
	oldPassword, newPassword := "OLD-PASSWORD", "NEW-PASSWORD"
	for _, stage := range []string{"deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{
				expectPOST("/api/redis.update", `{"databasePassword":"NEW-PASSWORD","description":null,"dockerImage":"redis:8","name":"cache","redisId":"r1"}`, `{}`),
				{Method: http.MethodPost, Path: "/api/redis.deploy", Body: json.RawMessage(`{"redisId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD"}`)},
			}
			if stage == "poll" {
				expectations[1] = expectPOST("/api/redis.deploy", `{"redisId":"r1"}`, `"running"`)
				expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/redis.one", Query: map[string][]string{"redisId": {"r1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD"}`)})
			}
			s := newScriptedServer(t, expectations...)
			_, err := (Redis{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RedisArgs, RedisState]{ID: "r1", Inputs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: newPassword, DockerImage: "redis:8"}, State: RedisState{RedisArgs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: oldPassword, DockerImage: "redis:8"}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldPassword)
			require.NotContains(t, err.Error(), newPassword)
		})
	}
}

func TestRedisDeploymentFailureReturnsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/redis.create", `{"databasePassword":"pw","description":null,"dockerImage":"redis:8","environmentId":"env","name":"cache","serverId":null}`, `{"redisId":"r1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/redis.deploy", Body: json.RawMessage(`{"redisId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"deploy failed"}`)},
	)
	got, err := (Redis{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[RedisArgs]{Inputs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "redis:8"}})
	require.Error(t, err)
	require.Equal(t, "r1", got.ID)
	require.Empty(t, got.Output.Status)
}

func TestRedisEnvironmentErrorsRedactOldAndNewAcrossSaveDeployAndPoll(t *testing.T) {
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	for _, stage := range []string{"save", "deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{expectPOST("/api/redis.saveEnvironment", `{"env":"NEW-ENV","redisId":"r1"}`, `true`)}
			if stage == "save" {
				expectations[0] = scriptedRequest{Method: http.MethodPost, Path: "/api/redis.saveEnvironment", Body: json.RawMessage(`{"env":"NEW-ENV","redisId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
			} else {
				expectations = append(expectations, expectPOST("/api/redis.deploy", `{"redisId":"r1"}`, `"running"`))
				if stage == "deploy" {
					expectations[1] = scriptedRequest{Method: http.MethodPost, Path: "/api/redis.deploy", Body: json.RawMessage(`{"redisId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
				} else {
					expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/redis.one", Query: map[string][]string{"redisId": {"r1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)})
				}
			}
			s := newScriptedServer(t, expectations...)
			_, err := (Redis{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[RedisArgs, RedisState]{ID: "r1", Inputs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "redis:8", Environment: &newEnv}, State: RedisState{RedisArgs: RedisArgs{Name: "cache", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "redis:8", Environment: &oldEnv}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldEnv)
			require.NotContains(t, err.Error(), newEnv)
		})
	}
}
