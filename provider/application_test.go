package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestApplicationPreviewMakesNoRequest(t *testing.T) {
	r := Application{}
	got, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{DryRun: true, Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.NoError(t, err)
	require.Empty(t, got.ID)
}

func TestApplicationDeclaresSecretDependenciesForTopLevelAndNestedOutputs(t *testing.T) {
	var _ infer.ExplicitDependencies[ApplicationArgs, ApplicationState] = Application{}
	// Provider schema generation exercises WireDependencies and must retain the
	// nested Docker password's secret marker as well as top-level secrets.
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.True(t, schemaProperty(spec, "dokploy:index:Application", "source.docker.password").Secret)
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
	)
	r := Application{client: fixedClient(s.API())}
	got, err := r.Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}}})
	require.NoError(t, err)
	require.Equal(t, "a1", got.ID)
	require.Equal(t, "done", got.Output.Status)
}

func TestApplicationCheckAllowsComputedEnvironmentIdDuringPreview(t *testing.T) {
	checked, err := (Application{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name":          property.New("demo"),
		"environmentId": property.New(property.Computed),
		"source":        property.New(map[string]property.Value{"type": property.New("docker"), "docker": property.New(map[string]property.Value{"image": property.New("nginx")})}),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)
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
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","name":"demo","environmentId":"e1","applicationStatus":"done","type":"docker","dockerImage":"nginx"}`))
	secret := "keep"
	r := Application{client: fixedClient(s.API())}
	got, err := r.Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: ApplicationState{ApplicationArgs: ApplicationArgs{Environment: &secret, BuildArgs: &secret, BuildSecrets: &secret}}})
	require.NoError(t, err)
	require.Equal(t, &secret, got.Inputs.Environment)
	require.Equal(t, &secret, got.Inputs.BuildArgs)
	require.Equal(t, &secret, got.Inputs.BuildSecrets)
	require.Equal(t, "nginx", got.Inputs.Source.Docker.Image)
}

func TestApplicationReadReconstructsObservableFieldsAndPreservesSecrets(t *testing.T) {
	password := "prior-password"
	tests := []struct {
		name, source string
		check        func(*testing.T, ApplicationSource)
	}{
		{"docker", `{"type":"docker","dockerImage":"nginx","registryUrl":"https://registry.test","username":"alice"}`, func(t *testing.T, source ApplicationSource) {
			require.Equal(t, "nginx", source.Docker.Image)
			require.Equal(t, "https://registry.test", *source.Docker.RegistryURL)
			require.Equal(t, "alice", *source.Docker.Username)
			require.Equal(t, "prior-password", *source.Docker.Password)
		}},
		{"git", `{"type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"main","customGitBuildPath":"src","watchPaths":["src/**","README.md"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Containerfile","dockerContextPath":"app","dockerBuildStage":"release"}`, func(t *testing.T, source ApplicationSource) {
			require.Equal(t, "https://git.test/repo", source.Git.URL)
			require.Equal(t, "main", source.Git.Branch)
			require.Equal(t, "src", *source.Git.BuildPath)
			require.Equal(t, []string{"src/**", "README.md"}, source.Git.WatchPaths)
			require.True(t, source.Git.EnableSubmodules)
			require.Equal(t, BuildDockerfile, source.Git.Build.Type)
			require.Equal(t, "Containerfile", *source.Git.Build.Dockerfile)
			require.Equal(t, "app", *source.Git.Build.DockerContextPath)
			require.Equal(t, "release", *source.Git.Build.DockerBuildStage)
		}},
		{"gitlab", `{"type":"gitlab","gitlabId":"integration","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main","gitlabBuildPath":"app","watchPaths":["app/**"],"enableSubmodules":true,"buildType":"nixpacks"}`, func(t *testing.T, source ApplicationSource) {
			require.Equal(t, "integration", source.GitLab.IntegrationID)
			require.Equal(t, 42, source.GitLab.ProjectID)
			require.Equal(t, "owner", source.GitLab.Owner)
			require.Equal(t, "namespace", source.GitLab.Namespace)
			require.Equal(t, "repo", source.GitLab.Repository)
			require.Equal(t, "main", source.GitLab.Branch)
			require.Equal(t, "app", *source.GitLab.BuildPath)
			require.Equal(t, []string{"app/**"}, source.GitLab.WatchPaths)
			require.True(t, source.GitLab.EnableSubmodules)
			require.Equal(t, BuildNixpacks, source.GitLab.Build.Type)
			require.Nil(t, source.GitLab.Build.Dockerfile)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flattened := strings.TrimSuffix(strings.TrimPrefix(tc.source, "{"), "}")
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","name":"demo","appName":"app","description":"description","environmentId":"e1","serverId":"s1","createEnvFile":true,"applicationStatus":"done",`+flattened+`}`))
			got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: ApplicationState{ApplicationArgs: ApplicationArgs{Environment: &password, BuildArgs: &password, BuildSecrets: &password, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Password: &password}}}}})
			require.NoError(t, err)
			require.Equal(t, "app", *got.Inputs.AppName)
			require.Equal(t, "description", *got.Inputs.Description)
			require.Equal(t, "s1", *got.Inputs.ServerID)
			require.Equal(t, "prior-password", *got.Inputs.Environment)
			require.Equal(t, "prior-password", *got.Inputs.BuildArgs)
			require.Equal(t, "prior-password", *got.Inputs.BuildSecrets)
			require.True(t, got.Inputs.CreateEnvFile)
			tc.check(t, got.Inputs.Source)
		})
	}
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
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
		{"git", `{"applicationId":"a1","name":"demo","environmentId":"e1","type":"git","customGitUrl":"https://example.test/repo","customGitBranch":"main","customGitBuildPath":"app","watchPaths":["src/**"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Dockerfile","dockerContextPath":".","dockerBuildStage":"prod"}`, func(t *testing.T, s ApplicationSource) {
			require.Equal(t, "https://example.test/repo", s.Git.URL)
			require.Equal(t, "Dockerfile", *s.Git.Build.Dockerfile)
			require.True(t, s.Git.EnableSubmodules)
		}},
		{"gitlab", `{"applicationId":"a1","name":"demo","environmentId":"e1","type":"gitlab","gitlabId":"i1","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main","gitlabBuildPath":"app","watchPaths":["src/**"],"enableSubmodules":true,"buildType":"nixpacks"}`, func(t *testing.T, s ApplicationSource) {
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
			got, err := decodeApplicationSource(tc.raw, ApplicationSource{})
			require.NoError(t, err)
			require.Equal(t, tc.kind, got.Type)
		})
	}
}

func TestApplicationImportReconstructsObservableFieldsWithoutInventingSecrets(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want ApplicationSource
	}{
		{"docker", `{"type":"docker","dockerImage":"nginx","registryUrl":"https://registry.test","username":"alice"}`, ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx", RegistryURL: stringPtr("https://registry.test"), Username: stringPtr("alice")}}},
		{"git", `{"type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"main","customGitBuildPath":"src","watchPaths":["src/**"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Containerfile","dockerContextPath":"app","dockerBuildStage":"release"}`, ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", BuildPath: stringPtr("src"), WatchPaths: []string{"src/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Containerfile"), DockerContextPath: stringPtr("app"), DockerBuildStage: stringPtr("release")}}}},
		{"gitlab", `{"type":"gitlab","gitlabId":"integration","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main","gitlabBuildPath":"app","watchPaths":["app/**"],"enableSubmodules":true,"buildType":"nixpacks"}`, ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "integration", ProjectID: 42, Owner: "owner", Namespace: "namespace", Repository: "repo", Branch: "main", BuildPath: stringPtr("app"), WatchPaths: []string{"app/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &raw))
			got, err := decodeApplicationSource(raw, ApplicationSource{})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			if got.Docker != nil {
				require.Nil(t, got.Docker.Password)
			}
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
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
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
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

func TestApplicationCreateGitAndGitLabSetupFailuresCleanUp(t *testing.T) {
	tests := []struct {
		name         string
		source       ApplicationSource
		stage        string
		provider     string
		providerBody string
	}{
		{"git-source", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source", "/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"git-build", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "build", "/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"git-environment", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "environment", "/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"gitlab-source", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 42, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source", "/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"","gitlabId":"i","gitlabOwner":"o","gitlabPathNamespace":"n","gitlabProjectId":42,"gitlabRepository":"r","watchPaths":null}`},
		{"gitlab-build", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 42, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "build", "/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"","gitlabId":"i","gitlabOwner":"o","gitlabPathNamespace":"n","gitlabProjectId":42,"gitlabRepository":"r","watchPaths":null}`},
		{"gitlab-environment", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 42, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "environment", "/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"","gitlabId":"i","gitlabOwner":"o","gitlabPathNamespace":"n","gitlabProjectId":42,"gitlabRepository":"r","watchPaths":null}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const buildBody = `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"railpackVersion":null}`
			const environmentBody = `{"applicationId":"a1","buildArgs":"ARGS-SECRET","buildSecrets":"BUILD-SECRET","createEnvFile":false,"env":"ENV-SECRET"}`
			expectations := []scriptedRequest{
				expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
				{Method: http.MethodPost, Path: tc.provider, Body: json.RawMessage(tc.providerBody), Status: http.StatusOK, Response: []byte(`true`)},
			}
			if tc.stage != "source" {
				expectations = append(expectations, expectPOST("/api/application.saveBuildType", buildBody, `true`))
			}
			switch tc.stage {
			case "environment":
				expectations = append(expectations, scriptedRequest{Method: http.MethodPost, Path: "/api/application.saveEnvironment", Body: json.RawMessage(environmentBody), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"setup failed ENV-SECRET ARGS-SECRET BUILD-SECRET"}`)})
			case "build":
				expectations[len(expectations)-1] = scriptedRequest{Method: http.MethodPost, Path: "/api/application.saveBuildType", Body: json.RawMessage(buildBody), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"setup failed ENV-SECRET ARGS-SECRET BUILD-SECRET"}`)}
			default:
				expectations[1] = scriptedRequest{Method: http.MethodPost, Path: tc.provider, Body: json.RawMessage(tc.providerBody), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"setup failed ENV-SECRET ARGS-SECRET BUILD-SECRET"}`)}
			}
			expectations = append(expectations, expectPOST("/api/application.delete", `{"applicationId":"a1"}`, `true`))
			s := newScriptedServer(t, expectations...)
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: stringPtr("ENV-SECRET"), BuildArgs: stringPtr("ARGS-SECRET"), BuildSecrets: stringPtr("BUILD-SECRET"), Source: tc.source}})
			require.Error(t, err)
			require.Equal(t, "a1", got.ID)
			require.Contains(t, err.Error(), "SETUP_FAILED: setup failed")
			require.NotContains(t, err.Error(), "CLEANUP_FAILED")
			for _, secret := range []string{"ENV-SECRET", "ARGS-SECRET", "BUILD-SECRET"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestApplicationCreateGitAndGitLabDeployFailuresReturnPartialStateWithoutSecrets(t *testing.T) {
	secret := "TOP-SECRET"
	tests := []struct {
		name   string
		source ApplicationSource
		path   string
		body   string
	}{
		{"git", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"gitlab", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 42, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"","gitlabId":"i","gitlabOwner":"o","gitlabPathNamespace":"n","gitlabProjectId":42,"gitlabRepository":"r","watchPaths":null}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldInterval := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = oldInterval })
			s := newScriptedServer(t, expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`), expectPOST(tc.path, tc.body, `true`), expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"railpackVersion":null}`, `true`), expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":"TOP-SECRET","buildSecrets":"TOP-SECRET","createEnvFile":false,"env":"TOP-SECRET"}`, `true`), scriptedRequest{Method: http.MethodPost, Path: "/api/application.deploy", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"DEPLOY_FAILED","message":"TOP-SECRET"}`)})
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: &secret, BuildArgs: &secret, BuildSecrets: &secret, Source: tc.source}})
			require.Error(t, err)
			require.Equal(t, "a1", got.ID)
			require.NotContains(t, err.Error(), secret)
		})
	}
}

func TestApplicationCreateGitAndGitLabPollFailuresRedactEnvironmentAndBuildValues(t *testing.T) {
	secret := "TOP-SECRET"
	for _, tc := range []struct {
		name   string
		source ApplicationSource
		path   string
		body   string
	}{
		{"git", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "/api/application.saveGitProvider", `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"gitlab", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 42, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, "/api/application.saveGitlabProvider", `{"applicationId":"a1","enableSubmodules":false,"gitlabBranch":"main","gitlabBuildPath":"","gitlabId":"i","gitlabOwner":"o","gitlabPathNamespace":"n","gitlabProjectId":42,"gitlabRepository":"r","watchPaths":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldInterval := waitPollInterval
			waitPollInterval = 0
			t.Cleanup(func() { waitPollInterval = oldInterval })
			s := newScriptedServer(t, expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`), expectPOST(tc.path, tc.body, `true`), expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"railpackVersion":null}`, `true`), expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":"TOP-SECRET","buildSecrets":"TOP-SECRET","createEnvFile":false,"env":"TOP-SECRET"}`, `true`), expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`), scriptedRequest{Method: http.MethodGet, Path: "/api/application.one", Query: map[string][]string{"applicationId": {"a1"}}, Status: http.StatusBadRequest, Response: []byte(`{"code":"POLL_FAILED","message":"TOP-SECRET"}`)})
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: &secret, BuildArgs: &secret, BuildSecrets: &secret, Source: tc.source}})
			require.Error(t, err)
			require.Equal(t, "a1", got.ID)
			require.NotContains(t, err.Error(), secret)
		})
	}
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
		{"docker-to-git", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}, ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, true},
		{"git-to-gitlab", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, true},
		{"gitlab-to-docker", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}, true},
		{"docker-runtime", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "old"}}, ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "new"}}, false},
		{"git-runtime-url-branch-build", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://old.test/repo", Branch: "main", BuildPath: stringPtr("old"), Build: ApplicationBuild{Type: BuildNixpacks}}}, ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://new.test/repo", Branch: "release", BuildPath: stringPtr("new"), Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Containerfile"), DockerContextPath: stringPtr("app"), DockerBuildStage: stringPtr("prod")}}}, false},
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

func TestApplicationDiffMarksEnvironmentAndServerReplacementForEverySourceType(t *testing.T) {
	for _, source := range []ApplicationSource{
		{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}},
		{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
		{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
	} {
		oldServer, newServer := "old", "new"
		diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: ApplicationArgs{EnvironmentID: "new-env", ServerID: &newServer, Source: source}, State: ApplicationState{ApplicationArgs: ApplicationArgs{EnvironmentID: "old-env", ServerID: &oldServer, Source: source}}})
		require.NoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["environmentId"].Kind)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["serverId"].Kind)
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
		scriptedRequest{Method: http.MethodPost, Path: "/api/application.deploy", Body: json.RawMessage(`{"applicationId":"a1"}`), Status: http.StatusBadRequest, Response: []byte(`{"code":"DEPLOY_FAILED","message":"PASSWORD-SECRET ENV-SECRET"}`)},
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
