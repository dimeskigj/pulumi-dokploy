package dokploy

import (
	"context"
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
	require.Equal(t, "nginx", got.Inputs.Source.Docker.Image)
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

func TestApplicationRuntimeDockerImageUpdateRedeploysAfterEnvironment(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"redis","password":"","registryUrl":"","username":""}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	oldArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}
	newArgs := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "redis"}}}
	_, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: newArgs, State: ApplicationState{ApplicationArgs: oldArgs}})
	require.NoError(t, err)
}

func TestApplicationRuntimeGitLabUpdateOrdersProviderBuildEnvironmentAndRedeploy(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":true,"gitlabBranch":"main","gitlabBuildPath":"app","gitlabId":"integration","gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabProjectId":42,"gitlabRepository":"repo","watchPaths":["app/**"]}`, `true`),
		expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"dockerfile","dockerBuildStage":"prod","dockerContextPath":".","dockerfile":"Containerfile","herokuVersion":null,"railpackVersion":null}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":true,"env":null}`, `true`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", CreateEnvFile: true, Source: ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "integration", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "repo", Branch: "main", BuildPath: stringPtr("app"), WatchPaths: []string{"app/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Containerfile"), DockerContextPath: stringPtr("."), DockerBuildStage: stringPtr("prod")}}}}
	_, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: args, State: ApplicationState{ApplicationArgs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "integration", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "old", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}}}})
	require.NoError(t, err)
}

func TestApplicationReadReconstructsGitAndGitLabSources(t *testing.T) {
	tests := []struct {
		name, response string
		check          func(*testing.T, ApplicationSource)
	}{
		{"git", `{"applicationId":"a1","name":"demo","environmentId":"e1","source":{"type":"git","customGitUrl":"https://example.test/repo","customGitBranch":"main","customGitBuildPath":"app","watchPaths":["src/**"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Dockerfile","dockerContextPath":".","dockerBuildStage":"prod"}}`, func(t *testing.T, s ApplicationSource) {
			require.Equal(t, "https://example.test/repo", s.Git.URL)
			require.Equal(t, "Dockerfile", *s.Git.Build.Dockerfile)
			require.True(t, s.Git.EnableSubmodules)
		}},
		{"gitlab", `{"applicationId":"a1","name":"demo","environmentId":"e1","source":{"type":"gitlab","gitlabId":"i1","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main","gitlabBuildPath":"app","watchPaths":["src/**"],"enableSubmodules":true,"buildType":"nixpacks"}}`, func(t *testing.T, s ApplicationSource) {
			require.Equal(t, 42, s.GitLab.ProjectID)
			require.Equal(t, "namespace", s.GitLab.Namespace)
			require.Equal(t, BuildNixpacks, s.GitLab.Build.Type)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, tc.response))
			prior := ApplicationState{ApplicationArgs: ApplicationArgs{Environment: stringPtr("env-secret"), BuildArgs: stringPtr("args-secret"), BuildSecrets: stringPtr("build-secret")}}
			got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: prior})
			require.NoError(t, err)
			tc.check(t, got.Inputs.Source)
			require.Equal(t, "env-secret", *got.Inputs.Environment)
			require.Equal(t, "args-secret", *got.Inputs.BuildArgs)
			require.Equal(t, "build-secret", *got.Inputs.BuildSecrets)
		})
	}
}

func TestApplicationImportReconstructsAllSourceVariants(t *testing.T) {
	for _, tc := range []struct {
		kind ApplicationSourceType
		raw  map[string]interface{}
	}{
		{SourceDocker, map[string]interface{}{"type": "docker", "dockerImage": "nginx"}},
		{SourceGit, map[string]interface{}{"type": "git", "customGitUrl": "u", "customGitBranch": "main"}},
		{SourceGitLab, map[string]interface{}{"type": "gitlab", "gitlabId": "i", "gitlabProjectId": float64(1), "gitlabOwner": "o", "gitlabPathNamespace": "n", "gitlabRepository": "r", "gitlabBranch": "main"}},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			got, err := decodeApplicationSource(&tc.raw, ApplicationSource{})
			require.NoError(t, err)
			require.Equal(t, tc.kind, got.Type)
		})
	}
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

func TestApplicationCreateGitOrdersProviderBuildEnvironmentAndDeploy(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		expectPOST("/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"app","customGitUrl":"https://example.test/repo","enableSubmodules":true,"watchPaths":["src/**"]}`, `true`),
		expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"railpackVersion":null}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":true,"env":null}`, `true`),
		expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","status":"done"}`),
	)
	buildPath := "app"
	args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", CreateEnvFile: true, Source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://example.test/repo", Branch: "main", BuildPath: &buildPath, WatchPaths: []string{"src/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks}}}}
	got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: args})
	require.NoError(t, err)
	require.Equal(t, "a1", got.ID)
}

func TestApplicationReadAndDeleteNotFoundAreIdempotent(t *testing.T) {
	s := newScriptedServer(t,
		scriptedRequest{Method: http.MethodGet, Path: "/api/application.one", Query: map[string][]string{"applicationId": {"missing"}}, Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.delete", Body: json.RawMessage(`{"applicationId":"missing"}`), Status: http.StatusNotFound, Response: []byte(`{"code":"NOT_FOUND"}`)},
	)
	r := Application{client: fixedClient(s.API())}
	read, err := r.Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "missing"})
	require.NoError(t, err)
	require.Empty(t, read.ID)
	_, err = r.Delete(t.Context(), infer.DeleteRequest[ApplicationState]{ID: "missing"})
	require.NoError(t, err)
}

func TestApplicationCreateSetupFailureCleansUpAndReturnsPartialState(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.saveDockerProvider", Body: json.RawMessage(`{"applicationId":"a1","dockerImage":"nginx","password":"","registryUrl":"","username":""}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"bad setup"}`)},
		expectPOST("/api/application.delete", `{"applicationId":"a1"}`, `true`),
	)
	got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.Error(t, err)
	require.Equal(t, "a1", got.ID)
}

func TestApplicationCreateSetupCleanupFailureKeepsOriginalError(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.saveDockerProvider", Body: json.RawMessage(`{"applicationId":"a1","dockerImage":"nginx","password":"","registryUrl":"","username":""}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"bad setup"}`)},
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.delete", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusInternalServerError, Response: []byte(`{"code":"CLEANUP_FAILED","message":"cleanup failed"}`)},
	)
	_, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.Contains(t, err.Error(), "SETUP_FAILED: bad setup")
	require.NotContains(t, err.Error(), "CLEANUP_FAILED")
}

func TestApplicationDiffMarksServerReplacement(t *testing.T) {
	oldServer, newServer := "s1", "s2"
	diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: ApplicationArgs{EnvironmentID: "e1", ServerID: &newServer, Source: ApplicationSource{Type: SourceDocker}}, State: ApplicationState{ApplicationArgs: ApplicationArgs{EnvironmentID: "e1", ServerID: &oldServer, Source: ApplicationSource{Type: SourceDocker}}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
}

func TestApplicationPerVariantDiffClassifiesReplacementAndRuntimeChanges(t *testing.T) {
	for _, tc := range []struct {
		name        string
		old, in     ApplicationSource
		replacement bool
	}{
		{"git-to-gitlab", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, true},
		{"gitlab-runtime", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "old", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "new", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: ApplicationArgs{EnvironmentID: "e1", Source: tc.in}, State: ApplicationState{ApplicationArgs: ApplicationArgs{EnvironmentID: "e1", Source: tc.old}}})
			require.NoError(t, err)
			key := "source"
			if tc.replacement {
				key = "source.type"
				require.Equal(t, p.UpdateReplace, diff.DetailedDiff[key].Kind)
			} else {
				require.Equal(t, p.Update, diff.DetailedDiff[key].Kind)
			}
		})
	}
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

func TestApplicationCreateDeployErrorRedactsEchoedSecrets(t *testing.T) {
	password, env, buildArgs, buildSecrets := "PASSWORD-SECRET", "ENV-SECRET", "ARGS-SECRET", "BUILD-SECRET"
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"nginx","password":"PASSWORD-SECRET","registryUrl":"","username":""}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":"ARGS-SECRET","buildSecrets":"BUILD-SECRET","createEnvFile":false,"env":"ENV-SECRET"}`, `true`),
		expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `{"code":"DEPLOY_FAILED","message":"PASSWORD-SECRET ENV-SECRET"}`),
	)
	got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: &env, BuildArgs: &buildArgs, BuildSecrets: &buildSecrets, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx", Password: &password}}}})
	require.Error(t, err)
	require.Equal(t, "a1", got.ID)
	require.NotContains(t, err.Error(), password)
	require.NotContains(t, err.Error(), env)
	require.NotContains(t, err.Error(), buildArgs)
	require.NotContains(t, err.Error(), buildSecrets)
}

func TestApplicationCreatePollErrorRedactsEchoedSecrets(t *testing.T) {
	password, env, buildArgs, buildSecrets := "PASSWORD-SECRET", "ENV-SECRET", "ARGS-SECRET", "BUILD-SECRET"
	s := newScriptedServer(t,
		expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
		expectPOST("/api/application.saveDockerProvider", `{"applicationId":"a1","dockerImage":"nginx","password":"PASSWORD-SECRET","registryUrl":"","username":""}`, `true`),
		expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":"ARGS-SECRET","buildSecrets":"BUILD-SECRET","createEnvFile":false,"env":"ENV-SECRET"}`, `true`),
		expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusBadRequest, `{"code":"POLL_FAILED","message":"PASSWORD-SECRET ENV-SECRET"}`),
	)
	_, err := (Application{client: fixedClient(s.API())}).Create(context.Background(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: &env, BuildArgs: &buildArgs, BuildSecrets: &buildSecrets, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx", Password: &password}}}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)
	require.NotContains(t, err.Error(), env)
	require.NotContains(t, err.Error(), buildArgs)
	require.NotContains(t, err.Error(), buildSecrets)
}
