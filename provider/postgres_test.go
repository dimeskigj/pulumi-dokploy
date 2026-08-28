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

func TestPostgresCheckDefaultsImage(t *testing.T) {
	r := Postgres{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("db"), "environmentId": property.New("env"),
		"databaseName": property.New("app"), "databaseUser": property.New("user"), "databasePassword": property.New("secret"),
	})})
	require.NoError(t, err)
	require.Equal(t, "postgres:18", got.Inputs.DockerImage)
}

func TestPostgresProviderRegistrationAndStatusValidation(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:Postgres")
	resource := spec.Resources["dokploy:index:Postgres"]
	require.True(t, resource.InputProperties["databasePassword"].Secret)
	require.True(t, resource.InputProperties["environment"].Secret)
	require.EqualError(t, func() error { _, err := postgresStatusValue(&generated.Postgres{}); return err }(), "postgres.one returned postgres without a status")
}

func TestPostgresDiffReplacesEnvironmentAndServerOnly(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	diff, err := (Postgres{}).Diff(t.Context(), infer.DiffRequest[PostgresArgs, PostgresState]{Inputs: PostgresArgs{EnvironmentID: "e2", ServerID: &newServer}, State: PostgresState{PostgresArgs: PostgresArgs{EnvironmentID: "e1", ServerID: &oldServer}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
}

func TestPostgresStatusPreservesValuesAndRejectsUnknown(t *testing.T) {
	for _, status := range []string{"running", "done", "error", "paused"} {
		v := &generated.Postgres{AdditionalProperties: map[string]interface{}{"status": status}}
		got, err := postgresStatusValue(v)
		require.NoError(t, err)
		require.Equal(t, status, got)
	}
	v := &generated.Postgres{AdditionalProperties: map[string]interface{}{"status": 42}}
	_, err := postgresStatusValue(v)
	require.EqualError(t, err, "postgres.one returned invalid status 42")
}

func TestPostgresCreateOrdersOptionalConfigurationBeforeDeploy(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	env, port := "POSTGRES_ENV", 5433
	s := newScriptedServer(t,
		expectPOST("/api/postgres.create", `{"databaseName":"app","databasePassword":"password","databaseUser":"user","description":null,"dockerImage":"postgres:18","environmentId":"env","name":"db","serverId":null}`, `{"postgresId":"p1"}`),
		expectPOST("/api/postgres.saveEnvironment", `{"env":"POSTGRES_ENV","postgresId":"p1"}`, `true`),
		expectPOST("/api/postgres.saveExternalPort", `{"externalPort":5433,"postgresId":"p1"}`, `true`),
		expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `"running"`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, `{"postgresId":"p1","status":"done"}`),
	)
	got, err := (Postgres{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "app", DatabaseUser: "user", DatabasePassword: "password", Environment: &env, ExternalPort: &port, DockerImage: "postgres:18"}})
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestPostgresMetadataUpdateDoesNotDeploy(t *testing.T) {
	newName, oldName := "new", "old"
	s := newScriptedServer(t, expectPOST("/api/postgres.update", `{"description":null,"name":"new","postgresId":"p1"}`, `{}`))
	_, err := (Postgres{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[PostgresArgs, PostgresState]{ID: "p1", Inputs: PostgresArgs{Name: newName, EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw"}, State: PostgresState{PostgresArgs: PostgresArgs{Name: oldName, EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw"}}})
	require.NoError(t, err)
}

func TestPostgresRuntimeUpdateClearsOptionalValuesAndDeploys(t *testing.T) {
	oldEnv, oldPort, pw := "SECRET", 5432, "pw"
	s := newScriptedServer(t,
		expectPOST("/api/postgres.update", `{"databaseName":"newdb","databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"postgres:18","name":"db","postgresId":"p1"}`, `{}`),
		expectPOST("/api/postgres.saveEnvironment", `{"env":null,"postgresId":"p1"}`, `true`),
		expectPOST("/api/postgres.saveExternalPort", `{"externalPort":null,"postgresId":"p1"}`, `true`),
		expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `"running"`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, `{"postgresId":"p1","status":"done"}`),
	)
	_, err := (Postgres{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[PostgresArgs, PostgresState]{ID: "p1", Inputs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "newdb", DatabaseUser: "user", DatabasePassword: pw, DockerImage: "postgres:18"}, State: PostgresState{PostgresArgs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: pw, DockerImage: "postgres:18", Environment: &oldEnv, ExternalPort: &oldPort}}})
	require.NoError(t, err)
}

func TestPostgresReadPreservesWriteOnlyFieldsAndHandlesNotFound(t *testing.T) {
	pw, env := "PASSWORD", "ENVIRONMENT"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/postgres.one", Query: map[string][]string{"postgresId": {"p1"}}, Status: http.StatusOK, Response: []byte(`{"postgresId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","image":"postgres:18","status":"running"}`)})
	got, err := (Postgres{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[PostgresArgs, PostgresState]{ID: "p1", State: PostgresState{PostgresArgs: PostgresArgs{DatabasePassword: pw, Environment: &env}}})
	require.NoError(t, err)
	require.Equal(t, pw, got.Inputs.DatabasePassword)
	require.Equal(t, env, *got.Inputs.Environment)
	require.Equal(t, "running", got.State.Status)
	nf := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/postgres.one", Query: map[string][]string{"postgresId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)}, scriptedRequest{Method: http.MethodPost, Path: "/api/postgres.remove", Body: json.RawMessage(`{"postgresId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)})
	r := Postgres{client: fixedClient(nf.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[PostgresArgs, PostgresState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[PostgresState]{ID: "missing"})
	require.NoError(t, err)
}

func TestPostgresImportReconstructsObservableSecrets(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, `{"postgresId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","databasePassword":"observed-password","env":"observed-env","status":"done"}`))
	got, err := (Postgres{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[PostgresArgs, PostgresState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "observed-password", got.Inputs.DatabasePassword)
	require.Equal(t, "observed-env", *got.Inputs.Environment)
}

func TestPostgresImportRejectsMissingPassword(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, `{"postgresId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","status":"done"}`))
	_, err := (Postgres{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[PostgresArgs, PostgresState]{ID: "p1"})
	require.EqualError(t, err, "postgres.one omitted required databasePassword; import requires an observable password or prior state")
}

func TestPostgresUpdateErrorsRedactOldAndNewSecrets(t *testing.T) {
	oldPassword, newPassword := "OLD-PASSWORD", "NEW-PASSWORD"
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/postgres.update", Body: json.RawMessage(`{"databaseName":"db","databasePassword":"NEW-PASSWORD","databaseUser":"user","description":null,"dockerImage":"postgres:18","name":"db","postgresId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD OLD-ENV NEW-ENV"}`)})
	_, err := (Postgres{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[PostgresArgs, PostgresState]{ID: "p1", Inputs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: newPassword, DockerImage: "postgres:18", Environment: &newEnv}, State: PostgresState{PostgresArgs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: oldPassword, DockerImage: "old", Environment: &oldEnv}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), oldPassword)
	require.NotContains(t, err.Error(), newPassword)
	require.NotContains(t, err.Error(), oldEnv)
	require.NotContains(t, err.Error(), newEnv)
}

func TestPostgresEnvironmentDeployAndPollErrorsRedactOldAndNewSecrets(t *testing.T) {
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	for _, stage := range []string{"save", "deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{expectPOST("/api/postgres.saveEnvironment", `{"env":"NEW-ENV","postgresId":"p1"}`, `true`)}
			if stage == "save" {
				expectations[0] = scriptedRequest{Method: http.MethodPost, Path: "/api/postgres.saveEnvironment", Body: json.RawMessage(`{"env":"NEW-ENV","postgresId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
			} else {
				expectations = append(expectations, expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `"running"`))
				if stage == "deploy" {
					expectations[1] = scriptedRequest{Method: http.MethodPost, Path: "/api/postgres.deploy", Body: json.RawMessage(`{"postgresId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
				} else {
					expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/postgres.one", Query: map[string][]string{"postgresId": {"p1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)})
				}
			}
			s := newScriptedServer(t, expectations...)
			_, err := (Postgres{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[PostgresArgs, PostgresState]{ID: "p1", Inputs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "postgres:18", Environment: &newEnv}, State: PostgresState{PostgresArgs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "postgres:18", Environment: &oldEnv}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldEnv)
			require.NotContains(t, err.Error(), newEnv)
		})
	}
}

func TestPostgresDeploymentFailureReturnsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/postgres.create", `{"databaseName":"app","databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"postgres:18","environmentId":"env","name":"db","serverId":null}`, `{"postgresId":"p1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/postgres.deploy", Body: json.RawMessage(`{"postgresId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"deploy failed"}`)},
	)
	got, err := (Postgres{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{Name: "db", EnvironmentID: "env", DatabaseName: "app", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "postgres:18"}})
	require.Error(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "", got.Output.Status)
}
