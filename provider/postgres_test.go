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
	require.EqualError(t, func() error { _, err := postgresStatusValue(&generated.Postgres{}); return err }(), "postgres.one returned postgres without a status")
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
