package dokploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
)

func TestApplicationGitSourceSavesAndClearsSSHKey(t *testing.T) {
	key := "key-1"
	for _, tc := range []struct {
		name   string
		source GitApplicationSource
		body   string
	}{
		{"set", GitApplicationSource{URL: "https://git.test/repo", Branch: "main", SSHKeyID: &key, Build: ApplicationBuild{Type: BuildNixpacks}}, `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitSSHKeyId":"key-1","customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
		{"cleared", GitApplicationSource{URL: "https://git.test/repo", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}, `{"applicationId":"a1","customGitBranch":"main","customGitBuildPath":"","customGitSSHKeyId":null,"customGitUrl":"https://git.test/repo","enableSubmodules":false,"watchPaths":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectPOST("/api/application.saveGitProvider", tc.body, `true`))
			err := configureApplicationSource(context.Background(), fixedClient(s.API())(context.Background()), "a1", ApplicationSource{Type: SourceGit, Git: &tc.source})
			require.NoError(t, err)
		})
	}
}

func TestApplicationDockerRegistrySourceRequestShape(t *testing.T) {
	password := "docker-password-sentinel"
	s := newScriptedServer(t, expectPOST("/api/application.saveDockerProvider", `{"applicationId":"application-sentinel","dockerImage":"registry.example/team/api:1","password":"docker-password-sentinel","registryUrl":"https://registry.example","username":"registry-user-sentinel"}`, `true`))
	err := configureApplicationSource(context.Background(), fixedClient(s.API())(context.Background()), "application-sentinel", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "registry.example/team/api:1", RegistryURL: stringPtr("https://registry.example"), Username: stringPtr("registry-user-sentinel"), Password: &password}})
	require.NoError(t, err)
}

func TestApplicationGitLabSourceRequestShape(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/application.saveGitlabProvider", `{"applicationId":"application-sentinel","enableSubmodules":true,"gitlabBranch":"release","gitlabBuildPath":"services/api","gitlabId":"integration-sentinel","gitlabOwner":"owner-sentinel","gitlabPathNamespace":"platform/api","gitlabProjectId":42,"gitlabRepository":"service-sentinel","watchPaths":["services/**"]}`, `true`))
	err := configureApplicationSource(context.Background(), fixedClient(s.API())(context.Background()), "application-sentinel", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "integration-sentinel", ProjectID: 42, Owner: "owner-sentinel", Namespace: "platform/api", Repository: "service-sentinel", Branch: "release", BuildPath: stringPtr("services/api"), WatchPaths: []string{"services/**"}, EnableSubmodules: true}})
	require.NoError(t, err)
}

// TestApplicationRailpackBuildDecodeToEncodeRoundTrip decodes a Railpack build as
// application.one reports it and re-encodes it through the same saveBuildType call
// Create and Update use, so a read followed by a write preserves railpackVersion,
// isStaticSpa, and publishDirectory instead of dropping them.
func TestApplicationRailpackBuildDecodeToEncodeRoundTrip(t *testing.T) {
	const response = `{"type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"main","buildType":"railpack","railpackVersion":"0.4.2","isStaticSpa":true,"publishDirectory":"dist"}`
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response), &raw))

	decoded, err := decodeApplicationSource(raw, ApplicationSource{})
	require.NoError(t, err)
	require.Equal(t, ApplicationBuild{Type: BuildRailpack, RailpackVersion: stringPtr("0.4.2"), IsStaticSpa: true, PublishDirectory: stringPtr("dist")}, decoded.Git.Build)

	s := newScriptedServer(t, expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"railpack","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":true,"publishDirectory":"dist","railpackVersion":"0.4.2"}`, `true`))
	require.NoError(t, configureApplicationBuild(context.Background(), fixedClient(s.API())(context.Background()), "a1", decoded))
}

// TestApplicationRailpackBuildSendsUnsetOptionalFieldsAsNull pins the clearing
// path: publishDirectory is written on every railpack save, null when unset, so
// removing it from a program actually clears it server-side.
func TestApplicationRailpackBuildSendsUnsetOptionalFieldsAsNull(t *testing.T) {
	s := newScriptedServer(t, expectPOST("/api/application.saveBuildType", `{"applicationId":"a1","buildType":"railpack","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":false,"publishDirectory":null,"railpackVersion":null}`, `true`))
	source := ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildRailpack}}}
	require.NoError(t, configureApplicationBuild(context.Background(), fixedClient(s.API())(context.Background()), "a1", source))
}

// TestApplicationBuildDecodeKeepsUnmodeledBuildTypes covers the defaulting
// correction without trading it for a worse failure: a build type Dokploy reports
// but the provider does not model is kept verbatim instead of being rewritten to
// nixpacks, and it must not error — Read runs during refresh and import, where one
// failing application would abort the whole operation.
func TestApplicationBuildDecodeKeepsUnmodeledBuildTypes(t *testing.T) {
	for _, buildType := range []string{"static", "heroku_buildpacks", "paketo_buildpacks", "a-build-type-from-a-later-dokploy"} {
		t.Run(buildType, func(t *testing.T) {
			got, err := decodeBuild(map[string]interface{}{"buildType": buildType, "railpackVersion": "0.4.2", "isStaticSpa": true, "publishDirectory": "dist"})
			require.NoError(t, err)
			require.Equal(t, BuildType(buildType), got.Type)
			require.Nil(t, got.RailpackVersion)
			require.False(t, got.IsStaticSpa)
			require.Nil(t, got.PublishDirectory)
		})
	}
}

func TestApplicationSourceDecodeKeepsUnmodeledBuildType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   map[string]interface{}
		build func(ApplicationSource) ApplicationBuild
	}{
		{"git", map[string]interface{}{"type": "git", "customGitUrl": "u", "customGitBranch": "main", "buildType": "static"}, func(s ApplicationSource) ApplicationBuild { return s.Git.Build }},
		{"gitlab", map[string]interface{}{"type": "gitlab", "gitlabId": "i", "gitlabProjectId": float64(1), "gitlabOwner": "o", "gitlabPathNamespace": "n", "gitlabRepository": "r", "gitlabBranch": "main", "buildType": "static"}, func(s ApplicationSource) ApplicationBuild { return s.GitLab.Build }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := decodeApplicationSource(tc.raw, ApplicationSource{})
			require.NoError(t, err)
			require.Equal(t, BuildType("static"), tc.build(decoded).Type)
		})
	}
}

// TestApplicationUnmodeledBuildTypeIsStillRejectedAsInput keeps Check strict: a
// program may only declare a build type the provider actually writes.
func TestApplicationUnmodeledBuildTypeIsStillRejectedAsInput(t *testing.T) {
	for _, buildType := range []string{"static", "heroku_buildpacks", "paketo_buildpacks"} {
		t.Run(buildType, func(t *testing.T) {
			err := ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: ApplicationBuild{Type: BuildType(buildType)}}}.validate()
			require.EqualError(t, err, "source.git.build.type must be one of nixpacks, dockerfile, or railpack")
		})
	}
}

// TestApplicationBuildDecodeDefaultsEmptyBuildTypeToNixpacks pins the existing
// compatibility behavior: an absent or empty buildType still means nixpacks.
func TestApplicationBuildDecodeDefaultsEmptyBuildTypeToNixpacks(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  map[string]interface{}
	}{
		{"absent", map[string]interface{}{}},
		{"empty", map[string]interface{}{"buildType": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeBuild(tc.raw)
			require.NoError(t, err)
			require.Equal(t, BuildNixpacks, got.Type)
		})
	}
}

func TestApplicationRailpackBuildDoesNotLeakFieldsIntoOtherBuildTypes(t *testing.T) {
	for _, buildType := range []string{"nixpacks", "dockerfile"} {
		t.Run(buildType, func(t *testing.T) {
			got, err := decodeBuild(map[string]interface{}{"buildType": buildType, "dockerfile": "Dockerfile", "railpackVersion": "0.4.2", "isStaticSpa": true, "publishDirectory": "dist"})
			require.NoError(t, err)
			require.Nil(t, got.RailpackVersion)
			require.False(t, got.IsStaticSpa)
			require.Nil(t, got.PublishDirectory)
		})
	}
}

// TestApplicationRailpackImportedStateMatchesProgramInputs walks the import path
// as it is really walked: a program through Check on one side, application.one
// through Read on the other. Diffing a read against itself compares a value with
// itself and would pass even if the decoder dropped every railpack field.
func TestApplicationRailpackImportedStateMatchesProgramInputs(t *testing.T) {
	const response = `{"applicationId":"a1","name":"demo","environmentId":"e1","applicationStatus":"done","type":"git","customGitUrl":"https://git.test/repo","customGitBranch":"main","watchPaths":["apps/web/**"],"buildType":"railpack","railpackVersion":"0.4.2","isStaticSpa":true,"publishDirectory":"dist"}`
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, response))
	read, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1"})
	require.NoError(t, err)
	require.Equal(t, BuildRailpack, read.Inputs.Source.Git.Build.Type)

	checked, err := (Application{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
		"name": property.New("demo"), "environmentId": property.New("e1"),
		"source": property.New(map[string]property.Value{
			"type": property.New("git"),
			"git": property.New(map[string]property.Value{
				"url": property.New("https://git.test/repo"), "branch": property.New("main"),
				"watchPaths": property.New([]property.Value{property.New("apps/web/**")}),
				"build": property.New(map[string]property.Value{
					"type": property.New("railpack"), "railpackVersion": property.New("0.4.2"),
					"isStaticSpa": property.New(true), "publishDirectory": property.New("dist"),
				}),
			}),
		}),
	})})
	require.NoError(t, err)
	require.Empty(t, checked.Failures)

	diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: checked.Inputs, State: read.State})
	require.NoError(t, err)
	require.False(t, diff.HasChanges, "imported state must match the program that declared it")
	require.Empty(t, diff.DetailedDiff)
}

func TestApplicationRailpackBuildValidate(t *testing.T) {
	railpack := func(mutate func(*ApplicationBuild)) ApplicationSource {
		b := ApplicationBuild{Type: BuildRailpack}
		mutate(&b)
		return ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: b}}
	}
	for _, tc := range []struct {
		name   string
		source ApplicationSource
		want   string
	}{
		{"bare", railpack(func(*ApplicationBuild) {}), ""},
		{"all fields", railpack(func(b *ApplicationBuild) {
			b.RailpackVersion, b.IsStaticSpa, b.PublishDirectory = appStringPtr("0.4.2"), true, appStringPtr("dist")
		}), ""},
		{"rejects dockerfile", railpack(func(b *ApplicationBuild) { b.Dockerfile = appStringPtr("Dockerfile") }), "source.git.build.dockerfile must be omitted for railpack builds"},
		{"rejects dockerContextPath", railpack(func(b *ApplicationBuild) { b.DockerContextPath = appStringPtr(".") }), "source.git.build.dockerContextPath must be omitted for railpack builds"},
		{"rejects dockerBuildStage", railpack(func(b *ApplicationBuild) { b.DockerBuildStage = appStringPtr("prod") }), "source.git.build.dockerBuildStage must be omitted for railpack builds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.source.validate()
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestApplicationNonRailpackBuildsRejectRailpackFields(t *testing.T) {
	build := func(kind BuildType, mutate func(*ApplicationBuild)) ApplicationSource {
		b := ApplicationBuild{Type: kind}
		if kind == BuildDockerfile {
			b.Dockerfile = appStringPtr("Dockerfile")
		}
		mutate(&b)
		return ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "u", Branch: "main", Build: b}}
	}
	for _, kind := range []BuildType{BuildNixpacks, BuildDockerfile} {
		t.Run(string(kind), func(t *testing.T) {
			require.EqualError(t, build(kind, func(b *ApplicationBuild) { b.RailpackVersion = appStringPtr("0.4.2") }).validate(),
				fmt.Sprintf("source.git.build.railpackVersion must be omitted for %s builds", kind))
			require.EqualError(t, build(kind, func(b *ApplicationBuild) { b.IsStaticSpa = true }).validate(),
				fmt.Sprintf("source.git.build.isStaticSpa must be omitted for %s builds", kind))
			require.EqualError(t, build(kind, func(b *ApplicationBuild) { b.PublishDirectory = appStringPtr("dist") }).validate(),
				fmt.Sprintf("source.git.build.publishDirectory must be omitted for %s builds", kind))
		})
	}
}

func TestApplicationRailpackBuildSchemaExposesFields(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	build := spec.Types["dokploy:index:ApplicationBuild"]
	require.Equal(t, "string", build.Properties["railpackVersion"].Type)
	require.Equal(t, "boolean", build.Properties["isStaticSpa"].Type)
	require.Equal(t, "string", build.Properties["publishDirectory"].Type)
}

func TestApplicationSourceIDOnlyReadReconstructsAllFields(t *testing.T) {
	tests := []struct {
		name            string
		response        string
		want            ApplicationSource
		registryID      *string
		buildRegistryID *string
	}{
		{
			name:     "git",
			response: `{"applicationId":"a1","type":"git","registryId":"registry-git","buildRegistryId":"build-registry-git","customGitUrl":"https://git.test/repo","customGitBranch":"release","customGitBuildPath":"services/api","customGitSSHKeyId":"ssh-1","watchPaths":["services/**","README.md"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Containerfile","dockerContextPath":"services/api","dockerBuildStage":"production"}`,
			want: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{
				URL: "https://git.test/repo", Branch: "release", BuildPath: stringPtr("services/api"), SSHKeyID: stringPtr("ssh-1"),
				WatchPaths: []string{"services/**", "README.md"}, EnableSubmodules: true,
				Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Containerfile"), DockerContextPath: stringPtr("services/api"), DockerBuildStage: stringPtr("production")},
			}},
			registryID:      stringPtr("registry-git"),
			buildRegistryID: stringPtr("build-registry-git"),
		},
		{
			name:            "docker",
			response:        `{"applicationId":"a1","type":"docker","registryId":"registry-docker","buildRegistryId":"build-registry-docker","dockerImage":"registry.test/team/api:1","registryUrl":"https://registry.test","username":"alice"}`,
			want:            ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "registry.test/team/api:1", RegistryURL: stringPtr("https://registry.test"), Username: stringPtr("alice")}},
			registryID:      stringPtr("registry-docker"),
			buildRegistryID: stringPtr("build-registry-docker"),
		},
		{
			name:     "gitlab",
			response: `{"applicationId":"a1","type":"gitlab","registryId":"registry-gitlab","buildRegistryId":"build-registry-gitlab","gitlabId":"integration-1","gitlabProjectId":42,"gitlabOwner":"owner","gitlabPathNamespace":"platform/api","gitlabRepository":"service","gitlabBranch":"main","gitlabBuildPath":".","watchPaths":["cmd/**"],"enableSubmodules":true,"buildType":"dockerfile","dockerfile":"Dockerfile","dockerContextPath":".","dockerBuildStage":"release"}`,
			want: ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{
				IntegrationID: "integration-1", ProjectID: 42, Owner: "owner", Namespace: "platform/api", Repository: "service", Branch: "main", BuildPath: stringPtr("."),
				WatchPaths: []string{"cmd/**"}, EnableSubmodules: true,
				Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: stringPtr("Dockerfile"), DockerContextPath: stringPtr("."), DockerBuildStage: stringPtr("release")},
			}},
			registryID:      stringPtr("registry-gitlab"),
			buildRegistryID: stringPtr("build-registry-gitlab"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, tc.response))
			got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1"})
			require.NoError(t, err)
			assertApplicationSourceFields(t, tc.want, got.Inputs.Source)
			requireLiveEqual(t, "application.registryId", tc.registryID, got.Inputs.RegistryID)
			requireLiveEqual(t, "application.buildRegistryId", tc.buildRegistryID, got.Inputs.BuildRegistryID)
		})
	}
}

func TestApplicationSourceWriteOnlySecretsArePreserved(t *testing.T) {
	password := "write-only-password"
	s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","type":"docker","dockerImage":"nginx:1.27","registryUrl":"https://registry.test","username":"alice"}`))
	prior := ApplicationState{ApplicationArgs: ApplicationArgs{Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Password: &password}}}}
	got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: prior})
	require.NoError(t, err)
	requireLiveEqual(t, "application.source.docker.password", &password, got.Inputs.Source.Docker.Password)
}

func assertApplicationSourceFields(t *testing.T, want, got ApplicationSource) {
	t.Helper()
	require.Equal(t, want.Type, got.Type)
	switch want.Type {
	case SourceGit:
		requireLiveEqual(t, "application.source.git", true, got.Git != nil)
		if got.Git == nil {
			return
		}
		requireLiveEqual(t, "application.source.git.url", want.Git.URL, got.Git.URL)
		requireLiveEqual(t, "application.source.git.branch", want.Git.Branch, got.Git.Branch)
		requireLiveEqual(t, "application.source.git.buildPath", want.Git.BuildPath, got.Git.BuildPath)
		requireLiveEqual(t, "application.source.git.sshKeyId", want.Git.SSHKeyID, got.Git.SSHKeyID)
		requireLiveEqual(t, "application.source.git.watchPaths", want.Git.WatchPaths, got.Git.WatchPaths)
		requireLiveEqual(t, "application.source.git.enableSubmodules", want.Git.EnableSubmodules, got.Git.EnableSubmodules)
		assertApplicationBuildFields(t, "application.source.git.build", want.Git.Build, got.Git.Build)
	case SourceDocker:
		requireLiveEqual(t, "application.source.docker", true, got.Docker != nil)
		if got.Docker == nil {
			return
		}
		requireLiveEqual(t, "application.source.docker.image", want.Docker.Image, got.Docker.Image)
		requireLiveEqual(t, "application.source.docker.registryUrl", want.Docker.RegistryURL, got.Docker.RegistryURL)
		requireLiveEqual(t, "application.source.docker.username", want.Docker.Username, got.Docker.Username)
	case SourceGitLab:
		requireLiveEqual(t, "application.source.gitlab", true, got.GitLab != nil)
		if got.GitLab == nil {
			return
		}
		requireLiveEqual(t, "application.source.gitlab.integrationId", want.GitLab.IntegrationID, got.GitLab.IntegrationID)
		requireLiveEqual(t, "application.source.gitlab.projectId", want.GitLab.ProjectID, got.GitLab.ProjectID)
		requireLiveEqual(t, "application.source.gitlab.owner", want.GitLab.Owner, got.GitLab.Owner)
		requireLiveEqual(t, "application.source.gitlab.namespace", want.GitLab.Namespace, got.GitLab.Namespace)
		requireLiveEqual(t, "application.source.gitlab.repository", want.GitLab.Repository, got.GitLab.Repository)
		requireLiveEqual(t, "application.source.gitlab.branch", want.GitLab.Branch, got.GitLab.Branch)
		requireLiveEqual(t, "application.source.gitlab.buildPath", want.GitLab.BuildPath, got.GitLab.BuildPath)
		requireLiveEqual(t, "application.source.gitlab.watchPaths", want.GitLab.WatchPaths, got.GitLab.WatchPaths)
		requireLiveEqual(t, "application.source.gitlab.enableSubmodules", want.GitLab.EnableSubmodules, got.GitLab.EnableSubmodules)
		assertApplicationBuildFields(t, "application.source.gitlab.build", want.GitLab.Build, got.GitLab.Build)
	}
}

func assertApplicationBuildFields(t *testing.T, field string, want, got ApplicationBuild) {
	t.Helper()
	requireLiveEqual(t, field+".type", want.Type, got.Type)
	requireLiveEqual(t, field+".dockerfile", want.Dockerfile, got.Dockerfile)
	requireLiveEqual(t, field+".dockerContextPath", want.DockerContextPath, got.DockerContextPath)
	requireLiveEqual(t, field+".dockerBuildStage", want.DockerBuildStage, got.DockerBuildStage)
	requireLiveEqual(t, field+".railpackVersion", want.RailpackVersion, got.RailpackVersion)
	requireLiveEqual(t, field+".isStaticSpa", want.IsStaticSpa, got.IsStaticSpa)
	requireLiveEqual(t, field+".publishDirectory", want.PublishDirectory, got.PublishDirectory)
}

func assertLiveApplicationSource(t *testing.T, label string, want, got ApplicationSource) {
	t.Helper()
	if want.Type != got.Type {
		t.Fatalf("live Application %s source type did not match", label)
	}
	assertApplicationSourceFields(t, want, got)
}

func TestApplicationGitSourceSchemaIncludesSSHKeyID(t *testing.T) {
	spec, err := p.GetSchema(t.Context(), Name, Version, Provider())
	require.NoError(t, err)
	require.Contains(t, schemaProperty(spec, "dokploy:index:Application", "source.git.sshKeyId").Type, "string")
}

func TestApplicationSourceValidate(t *testing.T) {
	tests := []struct {
		name   string
		source ApplicationSource
		want   string
	}{
		{"docker missing", ApplicationSource{Type: SourceDocker}, "source.docker is required when source.type is docker"},
		{"docker extra", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "x"}, Git: &GitApplicationSource{URL: "x", Branch: "main"}}, "source.git must be omitted when source.type is docker"},
		{"git mismatch", ApplicationSource{Type: SourceGit, Docker: &DockerSource{Image: "x"}}, "source.git is required when source.type is git"},
		{"git empty url", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{Branch: "main"}}, "source.git.url must not be empty"},
		{"git empty branch", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://example.test/repo"}}, "source.git.branch must not be empty"},
		{"git nixpacks docker fields", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks, Dockerfile: appStringPtr("Dockerfile")}}}, "source.git.build.dockerfile must be omitted for nixpacks builds"},
		{"git dockerfile missing", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildDockerfile}}}, "source.git.build.dockerfile is required for dockerfile builds"},
		{"valid docker", ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}}, ""},
		{"valid git", ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "x", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ""},
		{"valid gitlab", ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: "i", ProjectID: 1, Owner: "o", Namespace: "n", Repository: "r", Branch: "main", Build: ApplicationBuild{Type: BuildDockerfile, Dockerfile: appStringPtr("Dockerfile")}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.validate()
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.want)
			}
		})
	}
}

func appStringPtr(s string) *string { return &s }
