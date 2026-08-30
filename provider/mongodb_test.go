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

func TestMongoDBCheckDefaultsImage(t *testing.T) {
	r := MongoDB{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("cache"), "environmentId": property.New("env"), "databaseUser": property.New("user"), "databasePassword": property.New("pw"),
	})})
	require.NoError(t, err)
	require.Equal(t, "mongo:8", got.Inputs.DockerImage)
}

func TestMongoDBCheckAllowsComputedEnvironmentIdDuringPreview(t *testing.T) {
	r := MongoDB{}
	got, err := r.Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("cache"), "environmentId": property.New(property.Computed), "databaseUser": property.New("user"), "databasePassword": property.New("pw"),
	})})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}

func TestMongoDBProviderRegistrationAndStatusValidation(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	resource := spec.Resources["dokploy:index:MongoDB"]
	require.NotNil(t, resource)
	require.True(t, resource.InputProperties["databasePassword"].Secret)
	require.True(t, resource.InputProperties["environment"].Secret)
	require.EqualError(t, func() error { _, err := mongodbStatusValue(&generated.MongoDB{}); return err }(), "mongo.one returned mongo without a status")
}

func TestMongoDBDiffReplacesEnvironmentAndServerOnly(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	diff, err := (MongoDB{}).Diff(t.Context(), infer.DiffRequest[MongoDBArgs, MongoDBState]{Inputs: MongoDBArgs{EnvironmentID: "e2", ServerID: &newServer}, State: MongoDBState{MongoDBArgs: MongoDBArgs{EnvironmentID: "e1", ServerID: &oldServer}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
}

func TestMongoDBCreateConfiguresBeforeDeployAndPolls(t *testing.T) {
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	env, port := "MONGO_ENV", 6380
	s := newScriptedServer(t,
		expectPOST("/api/mongo.create", `{"databasePassword":"password","databaseUser":"user","description":null,"dockerImage":"mongo:8","environmentId":"env","name":"cache","serverId":null}`, `{"mongoId":"r1"}`),
		expectPOST("/api/mongo.saveEnvironment", `{"env":"MONGO_ENV","mongoId":"r1"}`, `true`),
		expectPOST("/api/mongo.saveExternalPort", `{"externalPort":6380,"mongoId":"r1"}`, `true`),
		expectPOST("/api/mongo.deploy", `{"mongoId":"r1"}`, `"running"`),
		expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","applicationStatus":"done"}`),
	)
	got, err := (MongoDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MongoDBArgs]{Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: "password", Environment: &env, ExternalPort: &port, DockerImage: "mongo:8"}})
	require.NoError(t, err)
	require.Equal(t, "r1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestMongoDBCreateSendsReplicaSetsFlag(t *testing.T) {
	replicaSets := true
	s := newScriptedServer(t,
		expectPOST("/api/mongo.create", `{"databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"mongo:8","environmentId":"env","name":"cache","replicaSets":true,"serverId":null}`, `{"mongoId":"r1"}`),
		expectPOST("/api/mongo.deploy", `{"mongoId":"r1"}`, `"running"`),
		expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","applicationStatus":"done"}`),
	)
	got, err := (MongoDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MongoDBArgs]{Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mongo:8", ReplicaSets: &replicaSets}})
	require.NoError(t, err)
	require.Equal(t, "r1", got.ID)
}

func TestMongoDBReadReconstructsReplicaSetsFlag(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","name":"cache","environmentId":"env","databaseUser":"user","replicaSets":true,"applicationStatus":"done"}`))
	got, err := (MongoDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "r1"})
	require.NoError(t, err)
	require.NotNil(t, got.Inputs.ReplicaSets)
	require.True(t, *got.Inputs.ReplicaSets)
}

func TestMongoDBMetadataUpdateDoesNotDeploy(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/mongo.update", `{"description":null,"name":"new","mongoId":"r1"}`, `{}`))
	_, err := (MongoDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MongoDBArgs, MongoDBState]{ID: "r1", Inputs: MongoDBArgs{Name: "new", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "mongo:8"}, State: MongoDBState{MongoDBArgs: MongoDBArgs{Name: "old", EnvironmentID: "env", DatabasePassword: "pw", DockerImage: "mongo:8"}}})
	require.NoError(t, err)
}

func TestMongoDBRuntimeUpdateClearsValuesAndDeploys(t *testing.T) {
	oldEnv, oldPort, oldPassword, newPassword := "OLD-ENV", 6379, "OLD-PASSWORD", "NEW-PASSWORD"
	oldImage := "mongo:7"
	old := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = old })
	s := newScriptedServer(t,
		expectPOST("/api/mongo.update", `{"databasePassword":"NEW-PASSWORD","databaseUser":"user","description":null,"dockerImage":"mongo:8","name":"cache","mongoId":"r1"}`, `{}`),
		expectPOST("/api/mongo.saveEnvironment", `{"env":null,"mongoId":"r1"}`, `true`),
		expectPOST("/api/mongo.saveExternalPort", `{"externalPort":null,"mongoId":"r1"}`, `true`),
		expectPOST("/api/mongo.deploy", `{"mongoId":"r1"}`, `"running"`),
		expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","applicationStatus":"done"}`),
	)
	_, err := (MongoDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MongoDBArgs, MongoDBState]{ID: "r1", Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: newPassword, DockerImage: "mongo:8"}, State: MongoDBState{MongoDBArgs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: oldPassword, DockerImage: oldImage, Environment: &oldEnv, ExternalPort: &oldPort}}})
	require.NoError(t, err)
}

func TestMongoDBReadPreservesSecretsImportAndHandlesNotFound(t *testing.T) {
	pw, env := "PASSWORD", "ENVIRONMENT"
	s := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/mongo.one", Query: map[string][]string{"mongoId": {"r1"}}, Status: http.StatusOK, Response: []byte(`{"mongoId":"r1","name":"cache","environmentId":"env","databaseUser":"user","image":"mongo:8","applicationStatus":"running"}`)})
	got, err := (MongoDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "r1", State: MongoDBState{MongoDBArgs: MongoDBArgs{DatabasePassword: pw, Environment: &env}}})
	require.NoError(t, err)
	require.Equal(t, pw, got.Inputs.DatabasePassword)
	require.Equal(t, env, *got.Inputs.Environment)
	nf := newScriptedServer(t, scriptedRequest{Method: http.MethodGet, Path: "/api/mongo.one", Query: map[string][]string{"mongoId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)}, scriptedRequest{Method: http.MethodPost, Path: "/api/mongo.remove", Body: json.RawMessage(`{"mongoId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)})
	r := MongoDB{client: fixedClient(nf.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[MongoDBState]{ID: "missing"})
	require.NoError(t, err)
}

func TestMongoDBImportReconstructsObservablePassword(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","name":"cache","environmentId":"env","databaseUser":"user","databasePassword":"observed-password","env":"observed-env","applicationStatus":"done"}`))
	got, err := (MongoDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "r1"})
	require.NoError(t, err)
	require.Equal(t, "observed-password", got.Inputs.DatabasePassword)
	require.Equal(t, "observed-env", *got.Inputs.Environment)
}

func TestMongoDBImportAllowsMissingPasswordWithoutInventingSecret(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","name":"cache","environmentId":"env","databaseUser":"user","applicationStatus":"done"}`))
	got, err := (MongoDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "r1"})
	require.NoError(t, err)
	require.Empty(t, got.Inputs.DatabasePassword)
}

func TestMongoDBRefreshPreservesPriorPasswordWhenAPIOmitsIt(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mongo.one", map[string][]string{"mongoId": {"r1"}}, http.StatusOK, `{"mongoId":"r1","name":"cache","environmentId":"env","databaseUser":"user","applicationStatus":"done"}`))
	got, err := (MongoDB{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: "r1", State: MongoDBState{MongoDBArgs: MongoDBArgs{DatabasePassword: "prior"}}})
	require.NoError(t, err)
	require.Equal(t, "prior", got.Inputs.DatabasePassword)
}

func TestMongoDBPostImportPasswordIsRuntimeUpdate(t *testing.T) {
	diff, err := (MongoDB{}).Diff(t.Context(), infer.DiffRequest[MongoDBArgs, MongoDBState]{Inputs: MongoDBArgs{DatabasePassword: "supplied"}, State: MongoDBState{MongoDBArgs: MongoDBArgs{}}})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["databasePassword"].Kind)
}

func TestMongoDBPasswordErrorsRedactOldAndNewAcrossDeployAndPoll(t *testing.T) {
	oldPassword, newPassword := "OLD-PASSWORD", "NEW-PASSWORD"
	for _, stage := range []string{"deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{
				expectPOST("/api/mongo.update", `{"databasePassword":"NEW-PASSWORD","databaseUser":"user","description":null,"dockerImage":"mongo:8","name":"cache","mongoId":"r1"}`, `{}`),
				{Method: http.MethodPost, Path: "/api/mongo.deploy", Body: json.RawMessage(`{"mongoId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD"}`)},
			}
			if stage == "poll" {
				expectations[1] = expectPOST("/api/mongo.deploy", `{"mongoId":"r1"}`, `"running"`)
				expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/mongo.one", Query: map[string][]string{"mongoId": {"r1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-PASSWORD NEW-PASSWORD"}`)})
			}
			s := newScriptedServer(t, expectations...)
			_, err := (MongoDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MongoDBArgs, MongoDBState]{ID: "r1", Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: newPassword, DockerImage: "mongo:8"}, State: MongoDBState{MongoDBArgs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: oldPassword, DockerImage: "mongo:8"}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldPassword)
			require.NotContains(t, err.Error(), newPassword)
		})
	}
}

func TestMongoDBDeploymentFailureReturnsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mongo.create", `{"databasePassword":"pw","databaseUser":"user","description":null,"dockerImage":"mongo:8","environmentId":"env","name":"cache","serverId":null}`, `{"mongoId":"r1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/mongo.deploy", Body: json.RawMessage(`{"mongoId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"deploy failed"}`)},
	)
	got, err := (MongoDB{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MongoDBArgs]{Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mongo:8"}})
	require.Error(t, err)
	require.Equal(t, "r1", got.ID)
	require.Empty(t, got.Output.Status)
}

func TestMongoDBEnvironmentErrorsRedactOldAndNewAcrossSaveDeployAndPoll(t *testing.T) {
	oldEnv, newEnv := "OLD-ENV", "NEW-ENV"
	for _, stage := range []string{"save", "deploy", "poll"} {
		t.Run(stage, func(t *testing.T) {
			old := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = old })
			expectations := []scriptedRequest{expectPOST("/api/mongo.saveEnvironment", `{"env":"NEW-ENV","mongoId":"r1"}`, `true`)}
			if stage == "save" {
				expectations[0] = scriptedRequest{Method: http.MethodPost, Path: "/api/mongo.saveEnvironment", Body: json.RawMessage(`{"env":"NEW-ENV","mongoId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
			} else {
				expectations = append(expectations, expectPOST("/api/mongo.deploy", `{"mongoId":"r1"}`, `"running"`))
				if stage == "deploy" {
					expectations[1] = scriptedRequest{Method: http.MethodPost, Path: "/api/mongo.deploy", Body: json.RawMessage(`{"mongoId":"r1"}`), Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)}
				} else {
					expectations = append(expectations, scriptedRequest{Method: http.MethodGet, Path: "/api/mongo.one", Query: map[string][]string{"mongoId": {"r1"}}, Status: http.StatusBadRequest, Response: []byte(`{"message":"OLD-ENV NEW-ENV"}`)})
				}
			}
			s := newScriptedServer(t, expectations...)
			_, err := (MongoDB{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MongoDBArgs, MongoDBState]{ID: "r1", Inputs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mongo:8", Environment: &newEnv}, State: MongoDBState{MongoDBArgs: MongoDBArgs{Name: "cache", EnvironmentID: "env", DatabaseUser: "user", DatabasePassword: "pw", DockerImage: "mongo:8", Environment: &oldEnv}}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), oldEnv)
			require.NotContains(t, err.Error(), newEnv)
		})
	}
}
