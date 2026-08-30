package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestMariaDBCheckDefaultsImage(t *testing.T) {
	r := MariaDB{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("db"), "environmentId": property.New("env"),
		"databaseName": property.New("app"), "databaseUser": property.New("user"), "databasePassword": property.New("secret"),
	})})
	require.NoError(t, err)
	require.Equal(t, "mariadb:11", got.Inputs.DockerImage)
}

func TestMariaDBCheckAllowsComputedEnvironmentIdDuringPreview(t *testing.T) {
	r := MariaDB{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("db"), "environmentId": property.New(property.Computed),
		"databaseName": property.New("app"), "databaseUser": property.New("user"), "databasePassword": property.New("secret"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}

func TestMariaDBProviderRegistrationAndStatusValidation(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, spec.Resources, "dokploy:index:MariaDB")
	resource := spec.Resources["dokploy:index:MariaDB"]
	require.True(t, resource.InputProperties["databasePassword"].Secret)
	require.True(t, resource.InputProperties["databaseRootPassword"].Secret)
	require.True(t, resource.InputProperties["environment"].Secret)
	require.EqualError(t, func() error { _, err := mariadbStatusValue(&generated.MariaDB{}); return err }(), "mariadb.one returned mariadb without a status")
}

func TestMariaDBDiffReplacesEnvironmentAndServerOnly(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	diff, err := (MariaDB{}).Diff(t.Context(), infer.DiffRequest[MariaDBArgs, MariaDBState]{Inputs: MariaDBArgs{EnvironmentID: "e2", ServerID: &newServer}, State: MariaDBState{MariaDBArgs: MariaDBArgs{EnvironmentID: "e1", ServerID: &oldServer}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
}

func TestMariaDBStatusPreservesValuesAndRejectsUnknown(t *testing.T) {
	for _, status := range []string{"running", "done", "error", "paused"} {
		v := &generated.MariaDB{AdditionalProperties: map[string]interface{}{"applicationStatus": status}}
		got, err := mariadbStatusValue(v)
		require.NoError(t, err)
		require.Equal(t, status, got)
	}
	v := &generated.MariaDB{AdditionalProperties: map[string]interface{}{"applicationStatus": 42}}
	_, err := mariadbStatusValue(v)
	require.EqualError(t, err, "mariadb.one returned invalid status 42")
}

func TestMariaDBCreateOrdersOptionalConfigurationBeforeDeploy(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	env, port := "MARIADB_ENV", 5433
	s := newScriptedServer(t,
		expectPOST("/api/mariadb.create", `{"databaseName":"app","databasePassword":"password","databaseUser":"user","description":null,"dockerImage":"mariadb:11","environmentId":"env","name":"db","serverId":null}`, `{"mariadbId":"p1"}`),
		expectPOST("/api/mariadb.saveEnvironment", `{"env":"MARIADB_ENV","mariadbId":"p1"}`, `true`),
		expectPOST("/api/mariadb.saveExternalPort", `{"externalPort":5433,"mariadbId":"p1"}`, `true`),
		expectPOST("/api/mariadb.deploy", `{"mariadbId":"p1"}`, `"running"`),
		expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","applicationStatus":"done"}`),
	)
	got, err := (MariaDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "app", DatabaseUser: "user", DatabasePassword: "password", Environment: &env, ExternalPort: &port, DockerImage: "mariadb:11"}})
	require.NoError(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestMariaDBCreateSendsRootPasswordAndRedactsItOnFailure(t *testing.T) {
	rootPassword := "ROOT-SECRET"
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.create", Body: json.RawMessage(`{"databaseName":"app","databasePassword":"password","databaseRootPassword":"ROOT-SECRET","databaseUser":"user","description":null,"dockerImage":"mariadb:11","environmentId":"env","name":"db","serverId":null}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"failed ROOT-SECRET"}`)},
	)
	_, err := (MariaDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "app", DatabaseUser: "user", DatabasePassword: "password", DatabaseRootPassword: &rootPassword, DockerImage: "mariadb:11"}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), rootPassword)
}

func TestMariaDBReadReconstructsObservedRootPassword(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","databaseRootPassword":"observed-root","applicationStatus":"done"}`))
	got, err := (MariaDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "observed-root", *got.Inputs.DatabaseRootPassword)
}

func TestMariaDBMetadataUpdateDoesNotDeploy(t *testing.T) {
	newName, oldName := "new", "old"
	s := newScriptedServer(t, expectPOST("/api/mariadb.update", `{"description":null,"name":"new","mariadbId":"p1"}`, `{}`))
	_, err := (MariaDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MariaDBArgs, MariaDBState]{ID: "p1", Inputs: MariaDBArgs{Name: newName, EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw"}, State: MariaDBState{MariaDBArgs: MariaDBArgs{Name: oldName, EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw"}}})
	require.NoError(t, err)
}

func TestMariaDBRuntimeUpdateClearsOptionalValuesAndDeploys(t *testing.T) {
	oldEnv, oldPort, pw := "SECRET", 5432, "pw"
	s := newScriptedServer(t,
		expectPOST("/api/mariadb.update", `{"databaseName":"newdb","databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"mariadb:11","name":"db","mariadbId":"p1"}`, `{}`),
		expectPOST("/api/mariadb.saveEnvironment", `{"env":null,"mariadbId":"p1"}`, `true`),
		expectPOST("/api/mariadb.saveExternalPort", `{"externalPort":null,"mariadbId":"p1"}`, `true`),
		expectPOST("/api/mariadb.deploy", `{"mariadbId":"p1"}`, `"running"`),
		expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","applicationStatus":"done"}`),
	)
	_, err := (MariaDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MariaDBArgs, MariaDBState]{ID: "p1", Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "newdb", DatabaseUser: "user", DatabasePassword: pw, DockerImage: "mariadb:11"}, State: MariaDBState{MariaDBArgs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: pw, DockerImage: "mariadb:11", Environment: &oldEnv, ExternalPort: &oldPort}}})
	require.NoError(t, err)
}

func TestMariaDBReadPreservesWriteOnlyFieldsAndHandlesNotFound(t *testing.T) {
	pw, env := "PASSWORD", "ENVIRONMENT"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/mariadb.one", Query: map[string][]string{"mariadbId": {"p1"}}, Status: http.StatusOK, Response: []byte(`{"mariadbId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","image":"mariadb:11","applicationStatus":"running"}`)})
	got, err := (MariaDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "p1", State: MariaDBState{MariaDBArgs: MariaDBArgs{DatabasePassword: pw, Environment: &env}}})
	require.NoError(t, err)
	require.Equal(t, pw, got.Inputs.DatabasePassword)
	require.Equal(t, env, *got.Inputs.Environment)
	require.Equal(t, "running", got.State.Status)
	nf := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/mariadb.one", Query: map[string][]string{"mariadbId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)}, scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.remove", Body: json.RawMessage(`{"mariadbId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)})
	r := MariaDB{client: fixedClient(nf.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[MariaDBState]{ID: "missing"})
	require.NoError(t, err)
}

func TestMariaDBImportReconstructsObservableSecrets(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","databasePassword":"observed-password","env":"observed-env","applicationStatus":"done"}`))
	got, err := (MariaDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "p1"})
	require.NoError(t, err)
	require.Equal(t, "observed-password", got.Inputs.DatabasePassword)
	require.Equal(t, "observed-env", *got.Inputs.Environment)
}

func TestMariaDBImportAllowsMissingPasswordWithoutInventingSecret(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","applicationStatus":"done"}`))
	got, err := (MariaDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "p1"})
	require.NoError(t, err)
	require.Empty(t, got.Inputs.DatabasePassword)
}

func TestMariaDBRefreshPreservesPriorPasswordWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mariadb.one", map[string][]string{"mariadbId": {"p1"}}, http.StatusOK, `{"mariadbId":"p1","name":"db","environmentId":"env","databaseName":"app","databaseUser":"user","applicationStatus":"done"}`))
	got, err := (MariaDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: "p1", State: MariaDBState{MariaDBArgs: MariaDBArgs{DatabasePassword: "prior"}}})
	require.NoError(t, err)
	require.Equal(t, "prior", got.Inputs.DatabasePassword)
}

func TestMariaDBPostImportPasswordIsRuntimeUpdate(t *testing.T) {
	diff, err := (MariaDB{}).Diff(t.Context(), infer.DiffRequest[MariaDBArgs, MariaDBState]{Inputs: MariaDBArgs{DatabasePassword: "supplied"}, State: MariaDBState{MariaDBArgs: MariaDBArgs{}}})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["databasePassword"].Kind)
}

func TestMariaDBUpdateErrorsRedactOldAndNewSecrets(t *testing.T) {
	oldPassword, newPassword := "OLD-PASSWORD", "NEW-PASSWORD"
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.update", Body: json.RawMessage(`{"databaseName":"db","databasePassword":"NEW-PASSWORD","databaseUser":"user","description":null,"dockerImage":"mariadb:11","name":"db","mariadbId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD OLD-ENV NEW-ENV"}`)})
	_, err := (MariaDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MariaDBArgs, MariaDBState]{ID: "p1", Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: newPassword, DockerImage: "mariadb:11", Environment: &newEnv}, State: MariaDBState{MariaDBArgs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: oldPassword, DockerImage: "old", Environment: &oldEnv}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), oldPassword)
	require.NotContains(t, err.Error(), newPassword)
	require.NotContains(t, err.Error(), oldEnv)
	require.NotContains(t, err.Error(), newEnv)
}

func TestMariaDBEnvironmentDeployAndPollErrorsRedactOldAndNewSecrets(t *testing.T) {
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	for _, stage := range []string{"save", "deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{expectPOST("/api/mariadb.saveEnvironment", `{"env":"NEW-ENV","mariadbId":"p1"}`, `true`)}
			if stage == "save" {
				expectations[0] = scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.saveEnvironment", Body: json.RawMessage(`{"env":"NEW-ENV","mariadbId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
			} else {
				expectations = append(expectations, expectPOST("/api/mariadb.deploy", `{"mariadbId":"p1"}`, `"running"`))
				if stage == "deploy" {
					expectations[1] = scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.deploy", Body: json.RawMessage(`{"mariadbId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
				} else {
					expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/mariadb.one", Query: map[string][]string{"mariadbId": {"p1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)})
				}
			}
			s := newScriptedServer(t, expectations...)
			_, err := (MariaDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MariaDBArgs, MariaDBState]{ID: "p1", Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mariadb:11", Environment: &newEnv}, State: MariaDBState{MariaDBArgs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "db", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mariadb:11", Environment: &oldEnv}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldEnv)
			require.NotContains(t, err.Error(), newEnv)
		})
	}
}

func TestMariaDBDeploymentFailureReturnsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mariadb.create", `{"databaseName":"app","databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"mariadb:11","environmentId":"env","name":"db","serverId":null}`, `{"mariadbId":"p1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/mariadb.deploy", Body: json.RawMessage(`{"mariadbId":"p1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"deploy failed"}`)},
	)
	got, err := (MariaDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: "db", EnvironmentID: "env", DatabaseName: "app", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mariadb:11"}})
	require.Error(t, err)
	require.Equal(t, "p1", got.ID)
	require.Equal(t, "", got.Output.Status)
}
