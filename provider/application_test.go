package dokploy

import (
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestApplicationPreviewMakesNoRequest(t *testing.T) {
	r := Application{}
	got, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{DryRun: true, Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.NoError(t, err)
	require.Empty(t, got.ID)
}

func TestApplicationCreateDockerOrdersConfigurationBeforeDeployment(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"nginx","password":"","registryUrl":"","username":""}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
		expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	r := Application{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.NoError(t, err)
	require.Equal(t, "a1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestApplicationDiffUsesReplacementForParentAndSourceType(t *testing.T) {
	diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{
		Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e2", Source: ApplicationSource{Type: SourceGit}},
		State:  ApplicationState{ApplicationArgs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker}}},
	})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
}

func TestApplicationReadPreservesWriteOnlySecrets(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","name":"demo","environmentId":"e1","status":"done"}`))
	secret := "keep"
	r := Application{client: fixedClient(s.API())}
	got, err := r.Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: ApplicationState{ApplicationArgs: ApplicationArgs{Environment: &secret, BuildArgs: &secret, BuildSecrets: &secret}}})
	require.NoError(t, err)
	require.Equal(t, &secret, got.Inputs.Environment)
	require.Equal(t, &secret, got.Inputs.BuildArgs)
	require.Equal(t, &secret, got.Inputs.BuildSecrets)
}

func TestApplicationMetadataUpdateDoesNotRedeploy(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/application.update", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"description":null,"env":null,"environmentId":"e1","name":"renamed","sourceType":"docker"}`, `{}`))
	r := Application{client: fixedClient(s.API())}
	_, err := r.Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: ApplicationArgs{Name: "renamed", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}, State: ApplicationState{ApplicationArgs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}}})
	require.NoError(t, err)
}

func TestApplicationSourceTypeJSON(t *testing.T) {
	b, err := json.Marshal(ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}})
	require.NoError(t, err)
	require.NotEmpty(t, b)
}
