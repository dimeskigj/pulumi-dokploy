package dokploy

import (
	"encoding/json"
	"fmt"
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
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","name":"demo","environmentId":"e1","status":"done","source":{"type":"docker","dockerImage":"nginx"}}`))
	secret := "keep"
	r := Application{client: fixedClient(s.API())}
	got, err := r.Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: ApplicationState{ApplicationArgs: ApplicationArgs{Environment: &secret, BuildArgs: &secret, BuildSecrets: &secret}}})
	require.NoError(t, err)
	require.Equal(t, &secret, got.Inputs.Environment)
	require.Equal(t, &secret, got.Inputs.BuildArgs)
	require.Equal(t, &secret, got.Inputs.BuildSecrets)
}

func TestApplicationMetadataUpdateDoesNotRedeploy(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/application.update", `{"applicationId":"a1","description":null,"name":"renamed"}`, `{}`))
	r := Application{client: fixedClient(s.API())}
	_, err := r.Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: ApplicationArgs{Name: "renamed", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}, State: ApplicationState{ApplicationArgs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}}})
	require.NoError(t, err)
}

func TestApplicationRuntimeSourceUpdateConfiguresProviderAndBuildBeforeEnvironment(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"src","customGitUrl":"https://example.test/repo","enableSubmodules":false,"watchPaths":null}`, `true`),
		expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"railpackVersion":null}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	buildPath := "src"
	newArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://example.test/repo", Branch: "main", BuildPath: &buildPath, Build: ApplicationBuild{Type: BuildNixpacks}}}}
	oldArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://old.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}}
	_, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: newArgs, State: ApplicationState{ApplicationArgs: oldArgs}})
	require.NoError(t, err)
}

func TestApplicationCreateGitLabOrdersProviderBuildEnvironmentAndDeploy(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		expectPOST("/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"app","gitlabId":"integration","gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabProjectId":42,"gitlabRepository":"repo","watchPaths":null}`, `true`),
		expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"dockerfile","dockerBuildStage":"production","dockerContextPath":".","dockerfile":"Dockerfile","herokuVersion":null,"railpackVersion":null}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
		expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	r := Application{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "integration", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "repo", Branch: "main", BuildPath: stringPtr("app"), Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Dockerfile"), DockerContextPath: stringPtr("."), DockerBuildStage: stringPtr("production")}}}}})
	require.NoError(t, err)
	require.Equal(t, "a1", got.ID)
}

func TestApplicationSourceTypeJSON(t *testing.T) {
	b, err := json.Marshal(ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}})
	require.NoError(t, err)
	require.NotEmpty(t, b)
}

func TestApplicationErrorsRedactAllSecrets(t *testing.T) {
	args := ApplicationArgs{Environment: stringPtr("ENV-SECRET"), BuildArgs: stringPtr("ARGS-SECRET"), BuildSecrets: stringPtr("BUILD-SECRET"), Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx", Password: stringPtr("PASSWORD-SECRET")}}}
	err := sanitizeApplicationError(fmt.Errorf("server echoed ENV-SECRET ARGS-SECRET BUILD-SECRET PASSWORD-SECRET"), args)
	require.NotContains(t, err.Error(), "SECRET")
}
