package dokploy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestApplicationRepositorySourceValidation(t *testing.T) {
	valid := repositorySourcesForTest()
	for _, source := range valid {
		require.NoError(t, source.validate())
	}

	for _, tc := range []struct {
		name   string
		source ApplicationSource
		want   string
	}{
		{"missing github", ApplicationSource{Type: SourceGitHub}, "source.github is required when source.type is github"},
		{"missing gitea", ApplicationSource{Type: SourceGitea}, "source.gitea is required when source.type is gitea"},
		{"missing bitbucket", ApplicationSource{Type: SourceBitbucket}, "source.bitbucket is required when source.type is bitbucket"},
		{"github empty integration", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{Owner: "o", Repository: "r", Branch: "b", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github.integrationId must not be empty"},
		{"github empty owner", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Repository: "r", Branch: "b", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github.owner must not be empty"},
		{"github empty repository", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Branch: "b", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github.repository must not be empty"},
		{"github empty branch", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github.branch must not be empty"},
		{"bitbucket empty slug", ApplicationSource{Type: SourceBitbucket, Bitbucket: &BitbucketAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "b", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.bitbucket.repositorySlug must not be empty"},
		{"mismatched github", ApplicationSource{Type: SourceGitea, GitHub: &GitHubAppSource{}, Gitea: &GiteaAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "b", Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github must be omitted when source.type is gitea"},
		{"invalid trigger", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "b", TriggerType: githubTriggerPtr(GitHubTriggerType("schedule")), Build: ApplicationBuild{Type: BuildNixpacks}}}, "source.github.triggerType must be one of push or tag"},
		{"invalid builder", ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "b", Build: ApplicationBuild{Type: BuildType("unknown")}}}, "source.github.build.type must be one of dockerfile, nixpacks, heroku_buildpacks, paketo_buildpacks, static, or railpack"},
	} {
		t.Run(tc.name, func(t *testing.T) { require.EqualError(t, tc.source.validate(), tc.want) })
	}
}

func TestApplicationRepositorySourceRequests(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		source           ApplicationSource
	}{
		{"github push", "/api/application.saveGithubProvider", `{"applicationId":"a1","branch":"main","buildPath":"services/api","enableSubmodules":true,"githubId":"github-integration","owner":"acme","repository":"api","triggerType":"push","watchPaths":["services/**"]}`, ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "github-integration", Owner: "acme", Repository: "api", Branch: "main", BuildPath: stringPtr("services/api"), WatchPaths: []string{"services/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks}}}},
		{"github tag", "/api/application.saveGithubProvider", `{"applicationId":"a1","branch":"main","buildPath":"","enableSubmodules":false,"githubId":"i","owner":"o","repository":"r","triggerType":"tag","watchPaths":null}`, ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", TriggerType: githubTriggerPtr(GitHubTriggerTag), Build: ApplicationBuild{Type: BuildNixpacks}}}},
		{"gitea", "/api/application.saveGiteaProvider", `{"applicationId":"a1","enableSubmodules":true,"giteaBranch":"main","giteaBuildPath":"","giteaId":"gitea-integration","giteaOwner":"acme","giteaRepository":"api","watchPaths":["src/**"]}`, ApplicationSource{Type: SourceGitea, Gitea: &GiteaAppSource{IntegrationID: "gitea-integration", Owner: "acme", Repository: "api", Branch: "main", WatchPaths: []string{"src/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildNixpacks}}}},
		{"bitbucket", "/api/application.saveBitbucketProvider", `{"applicationId":"a1","bitbucketBranch":"main","bitbucketBuildPath":"app","bitbucketId":"bitbucket-integration","bitbucketOwner":"acme","bitbucketRepository":"api","bitbucketRepositorySlug":"acme/api","enableSubmodules":false,"watchPaths":null}`, ApplicationSource{Type: SourceBitbucket, Bitbucket: &BitbucketAppSource{IntegrationID: "bitbucket-integration", Owner: "acme", Repository: "api", RepositorySlug: "acme/api", Branch: "main", BuildPath: stringPtr("app"), Build: ApplicationBuild{Type: BuildNixpacks}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectPOST(tc.path, tc.body, `true`))
			err := configureApplicationSource(context.Background(), fixedClient(s.API())(context.Background()), "a1", tc.source)
			require.NoError(t, err)
		})
	}
	for _, source := range repositorySourcesForTest() {
		_, ok := source.applicationBuild()
		require.True(t, ok, "source type %s must select a build", source.Type)
	}
	err := configureApplicationBuild(context.Background(), &client.Client{}, "a1", ApplicationSource{Type: SourceGitHub})
	require.Error(t, err)
}

func TestApplicationRepositorySourceReadAndImport(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		want           ApplicationSource
	}{
		{"github", `{"applicationId":"a1","type":"github","githubId":"i","owner":"o","repository":"r","branch":"main","buildPath":"app","triggerType":"tag","watchPaths":["app/**"],"enableSubmodules":true,"buildType":"railpack","railpackVersion":"stable"}`, ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", BuildPath: stringPtr("app"), TriggerType: githubTriggerPtr(GitHubTriggerTag), WatchPaths: []string{"app/**"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildRailpack, RailpackVersion: stringPtr("stable")}}}},
		{"gitea", `{"applicationId":"a1","type":"gitea","giteaId":"i","giteaOwner":"o","giteaRepository":"r","giteaBranch":"main","giteaBuildPath":"","watchPaths":null,"enableSubmodules":false,"buildType":"static","isStaticSpa":true}`, ApplicationSource{Type: SourceGitea, Gitea: &GiteaAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", BuildPath: nil, Build: ApplicationBuild{Type: BuildStatic, IsStaticSpa: appBoolPtr(true)}}}},
		{"bitbucket", `{"applicationId":"a1","type":"bitbucket","bitbucketId":"i","bitbucketOwner":"o","bitbucketRepository":"r","bitbucketRepositorySlug":"o/r","bitbucketBranch":"main","bitbucketBuildPath":".","watchPaths":["src"],"enableSubmodules":true,"buildType":"heroku_buildpacks","herokuVersion":"24"}`, ApplicationSource{Type: SourceBitbucket, Bitbucket: &BitbucketAppSource{IntegrationID: "i", Owner: "o", Repository: "r", RepositorySlug: "o/r", Branch: "main", BuildPath: nil, WatchPaths: []string{"src"}, EnableSubmodules: true, Build: ApplicationBuild{Type: BuildHerokuBuildpacks, HerokuVersion: stringPtr("24")}}}},
		{"github implicit push", `{"applicationId":"a1","type":"github","githubId":"i","owner":"o","repository":"r","branch":"main","buildType":"nixpacks"}`, ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.response), &raw))
			got, err := decodeApplicationSource(raw, ApplicationSource{})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, tc.response))
			read, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1"})
			require.NoError(t, err)
			require.Equal(t, tc.want, read.Inputs.Source)
		})
	}
	for _, tc := range []struct {
		kind   string
		fields map[string]interface{}
	}{
		{"github", map[string]interface{}{"githubId": "i", "owner": "o", "repository": "r", "branch": "main"}},
		{"gitea", map[string]interface{}{"giteaId": "i", "giteaOwner": "o", "giteaRepository": "r", "giteaBranch": "main"}},
		{"bitbucket", map[string]interface{}{"bitbucketId": "i", "bitbucketOwner": "o", "bitbucketRepository": "r", "bitbucketRepositorySlug": "o/r", "bitbucketBranch": "main"}},
	} {
		fields := tc.fields
		fields["type"] = tc.kind
		got, err := decodeApplicationSource(fields, ApplicationSource{})
		require.NoError(t, err)
		require.Equal(t, ApplicationSourceType(tc.kind), got.Type)
		delete(fields, "branch")
		delete(fields, "githubBranch")
		delete(fields, "giteaBranch")
		delete(fields, "bitbucketBranch")
		_, err = decodeApplicationSource(fields, ApplicationSource{})
		require.Error(t, err)
	}
}

func TestApplicationRepositorySourceBuilderMatrix(t *testing.T) {
	builders := []BuildType{BuildDockerfile, BuildNixpacks, BuildHerokuBuildpacks, BuildPaketoBuildpacks, BuildStatic, BuildRailpack}
	for _, source := range repositorySourcesForTest() {
		for _, builder := range builders {
			build := ApplicationBuild{Type: builder}
			if builder == BuildDockerfile {
				build.Dockerfile = stringPtr("Dockerfile")
			}
			source = repositorySourceWithBuild(source.Type, build)
			selected, ok := source.applicationBuild()
			require.True(t, ok)
			require.Equal(t, build, selected)
			require.NoError(t, source.validate())
		}
	}
}

func TestApplicationRepositorySourceDefaultsRespectPriorAndObservedChanges(t *testing.T) {
	for _, tc := range []struct {
		name         string
		path         string
		prior        *string
		want         *string
		priorTrigger *GitHubTriggerType
		wantTrigger  *GitHubTriggerType
	}{
		{"implicit root", `"buildPath":"/"`, nil, nil, nil, nil},
		{"configured root retained", `"buildPath":"/"`, stringPtr("."), stringPtr("."), githubTriggerPtr(GitHubTriggerPush), githubTriggerPtr(GitHubTriggerPush)},
		{"observed root replaces non-default", `"buildPath":"/"`, stringPtr("src"), stringPtr("/"), githubTriggerPtr(GitHubTriggerTag), nil},
		{"empty default normalized", `"buildPath":""`, nil, nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]interface{}{"type": "github", "githubId": "i", "owner": "o", "repository": "r", "branch": "main", "buildType": "nixpacks"}
			var pathFields map[string]interface{}
			if tc.path != "" {
				require.NoError(t, json.Unmarshal([]byte("{"+tc.path+"}"), &pathFields))
				for k, v := range pathFields {
					raw[k] = v
				}
			}
			prior := ApplicationSource{Type: SourceGitHub, GitHub: &GitHubAppSource{BuildPath: tc.prior, TriggerType: tc.priorTrigger, Build: ApplicationBuild{Type: BuildNixpacks}}}
			got, err := decodeApplicationSource(raw, prior)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.GitHub.BuildPath)
			require.Equal(t, tc.wantTrigger, got.GitHub.TriggerType)
		})
	}
}

func TestApplicationRepositorySourceAcceptsSourceTypeDiscriminator(t *testing.T) {
	got, err := decodeApplicationSource(map[string]interface{}{
		"sourceType": "bitbucket", "bitbucketId": "i", "bitbucketOwner": "o", "bitbucketRepository": "r",
		"bitbucketRepositorySlug": "o/r", "bitbucketBranch": "main", "watchPaths": []interface{}{},
	}, ApplicationSource{})
	require.NoError(t, err)
	require.Equal(t, SourceBitbucket, got.Type)
	require.Equal(t, []string{}, got.Bitbucket.WatchPaths)
}

func TestApplicationRepositorySourceCreateOrdering(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	for _, tc := range repositoryRequestFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t,
				expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
				expectPOST(tc.path, tc.body, `true`),
				expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`, `true`),
				expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
				expectPOST("/api/application.deploy", `{"applicationId":"a1"}`, `"running"`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			)
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: tc.source}})
			require.NoError(t, err)
			require.Equal(t, "a1", got.ID)
		})
	}
}

func TestApplicationRepositorySourceUpdateRedeploys(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	for _, tc := range repositoryRequestFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t,
				scriptedRequest{Method: http.MethodPost, Path: tc.path, Body: json.RawMessage(tc.body), Status: http.StatusOK, Response: []byte(`true`)},
				expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`, `true`),
				expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":"BUILD-SECRET","createEnvFile":false,"env":"ENV-SECRET"}`, `true`),
				expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			)
			args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: stringPtr("ENV-SECRET"), BuildSecrets: stringPtr("BUILD-SECRET"), Source: tc.source}
			prior := args
			prior.Source = repositorySourceWithBuild(tc.source.Type, ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Dockerfile")})
			_, err := (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: args, State: ApplicationState{ApplicationArgs: prior}})
			require.NoError(t, err)
		})
	}
}

func TestApplicationRepositorySourceMetadataUpdateRedeploys(t *testing.T) {
	oldInterval := waitPollInterval
	waitPollInterval = 0
	t.Cleanup(func() { waitPollInterval = oldInterval })
	for _, tc := range repositoryRequestFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			updatedSource := repositorySourceWithRepository(tc.source, "r-updated")
			field := ""
			switch tc.source.Type {
			case SourceGitHub:
				field = `"repository":"r"`
			case SourceGitea:
				field = `"giteaRepository":"r"`
			case SourceBitbucket:
				field = `"bitbucketRepository":"r"`
			}
			updatedBody := strings.Replace(tc.body, field, strings.Replace(field, `"r"`, `"r-updated"`, 1), 1)
			require.NotEqual(t, tc.body, updatedBody)
			args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: updatedSource}
			prior := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Source: tc.source}
			diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: args, State: ApplicationState{ApplicationArgs: prior}})
			require.NoError(t, err)
			require.Equal(t, p.Update, diff.DetailedDiff["source"].Kind)
			require.NotContains(t, diff.DetailedDiff, "source.type")

			s := newScriptedServer(t,
				expectPOST(tc.path, updatedBody, `true`),
				expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`, `true`),
				expectPOST("/api/application.saveEnvironment", `{"applicationId":"a1","buildArgs":null,"buildSecrets":null,"createEnvFile":false,"env":null}`, `true`),
				expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `"running"`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			)
			_, err = (Application{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: "a1", Inputs: args, State: ApplicationState{ApplicationArgs: prior}})
			require.NoError(t, err)
		})
	}
}

func TestApplicationRepositorySourceSetupFailureRedactsEchoedSecrets(t *testing.T) {
	for _, tc := range repositoryRequestFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t,
				expectPOST("/api/application.create", `{"name":"demo","environmentId":"e1"}`, `{"applicationId":"a1"}`),
				scriptedRequest{Method: http.MethodPost, Path: tc.path, Body: json.RawMessage(tc.body), Status: http.StatusBadRequest, Response: []byte(`{"code":"SETUP_FAILED","message":"echo ENV-SECRET BUILD-SECRET"}`)},
				expectPOST("/api/application.delete", `{"applicationId":"a1"}`, `true`),
			)
			args := ApplicationArgs{Name: "demo", EnvironmentID: "e1", Environment: stringPtr("ENV-SECRET"), BuildSecrets: stringPtr("BUILD-SECRET"), Source: tc.source}
			got, err := (Application{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[ApplicationArgs]{Inputs: args})
			require.Error(t, err)
			require.Equal(t, "a1", got.ID)
			require.NotContains(t, err.Error(), "ENV-SECRET")
			require.NotContains(t, err.Error(), "BUILD-SECRET")
		})
	}
}

func TestApplicationRepositorySourceTypeChangesReplace(t *testing.T) {
	old := repositorySourceWithBuild(SourceGitHub, ApplicationBuild{Type: BuildNixpacks})
	in := repositorySourceWithBuild(SourceGitea, ApplicationBuild{Type: BuildNixpacks})
	diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: ApplicationArgs{EnvironmentID: "e1", Source: in}, State: ApplicationState{ApplicationArgs: ApplicationArgs{EnvironmentID: "e1", Source: old}}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
}

func TestApplicationRepositorySourceBuildOptionsAreNullWhenUnset(t *testing.T) {
	for _, source := range repositorySourcesForTest()[2:] {
		s := newScriptedServer(t, expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`, `true`))
		require.NoError(t, configureApplicationBuild(context.Background(), fixedClient(s.API())(context.Background()), "a1", source))
	}
}

func TestApplicationRepositorySourceSchema(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	for _, path := range []string{"source.github.integrationId", "source.github.triggerType", "source.gitea.integrationId", "source.bitbucket.repositorySlug"} {
		require.NotEmpty(t, schemaProperty(spec, "dokploy:index:Application", path).Type, path)
	}
}

func repositorySourcesForTest() []ApplicationSource {
	return []ApplicationSource{
		{Type: SourceGit, Git: &GitApplicationSource{URL: "https://example.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
		{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
		{Type: SourceGitHub, GitHub: &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
		{Type: SourceGitea, Gitea: &GiteaAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
		{Type: SourceBitbucket, Bitbucket: &BitbucketAppSource{IntegrationID: "i", Owner: "o", Repository: "r", RepositorySlug: "o/r", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}},
	}
}

func repositoryRequestFixtures() []struct {
	name, path, body string
	source           ApplicationSource
} {
	return []struct {
		name, path, body string
		source           ApplicationSource
	}{
		{"github", "/api/application.saveGithubProvider", `{"applicationId":"a1","branch":"main","buildPath":"","enableSubmodules":false,"githubId":"i","owner":"o","repository":"r","triggerType":"push","watchPaths":null}`, repositorySourceWithBuild(SourceGitHub, ApplicationBuild{Type: BuildNixpacks})},
		{"gitea", "/api/application.saveGiteaProvider", `{"applicationId":"a1","enableSubmodules":false,"giteaBranch":"main","giteaBuildPath":"","giteaId":"i","giteaOwner":"o","giteaRepository":"r","watchPaths":null}`, repositorySourceWithBuild(SourceGitea, ApplicationBuild{Type: BuildNixpacks})},
		{"bitbucket", "/api/application.saveBitbucketProvider", `{"applicationId":"a1","bitbucketBranch":"main","bitbucketBuildPath":"","bitbucketId":"i","bitbucketOwner":"o","bitbucketRepository":"r","bitbucketRepositorySlug":"o/r","enableSubmodules":false,"watchPaths":null}`, repositorySourceWithBuild(SourceBitbucket, ApplicationBuild{Type: BuildNixpacks})},
	}
}

func repositorySourceWithRepository(source ApplicationSource, repository string) ApplicationSource {
	switch source.Type {
	case SourceGitHub:
		copy := *source.GitHub
		copy.Repository = repository
		source.GitHub = &copy
	case SourceGitea:
		copy := *source.Gitea
		copy.Repository = repository
		source.Gitea = &copy
	case SourceBitbucket:
		copy := *source.Bitbucket
		copy.Repository = repository
		source.Bitbucket = &copy
	}
	return source
}

func repositorySourceWithBuild(kind ApplicationSourceType, build ApplicationBuild) ApplicationSource {
	source := ApplicationSource{Type: kind}
	switch kind {
	case SourceGit:
		source.Git = &GitApplicationSource{URL: "https://example.test/repo", Branch: "main", Build: build}
	case SourceGitLab:
		source.GitLab = &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: build}
	case SourceGitHub:
		source.GitHub = &GitHubAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", Build: build}
	case SourceGitea:
		source.Gitea = &GiteaAppSource{IntegrationID: "i", Owner: "o", Repository: "r", Branch: "main", Build: build}
	case SourceBitbucket:
		source.Bitbucket = &BitbucketAppSource{IntegrationID: "i", Owner: "o", Repository: "r", RepositorySlug: "o/r", Branch: "main", Build: build}
	}
	return source
}

func githubTriggerPtr(v GitHubTriggerType) *GitHubTriggerType { return &v }
