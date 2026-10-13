package dokploy

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestApplicationBuildValidateAllTypes(t *testing.T) {
	for _, builder := range []BuildType{
		BuildDockerfile,
		BuildNixpacks,
		BuildHerokuBuildpacks,
		BuildPaketoBuildpacks,
		BuildStatic,
		BuildRailpack,
	} {
		t.Run(string(builder), func(t *testing.T) {
			build := ApplicationBuild{Type: builder}
			if builder == BuildDockerfile {
				build.Dockerfile = appStringPtr("Dockerfile")
			}
			require.NoError(t, validateBuild("source.git", build))
		})
	}

	for _, path := range []*string{nil, appStringPtr("")} {
		err := validateBuild("source.git", ApplicationBuild{Type: BuildDockerfile, Dockerfile: path})
		require.EqualError(t, err, "source.git.build.dockerfile is required for dockerfile builds")
	}
	require.EqualError(t, validateBuild("source.git", ApplicationBuild{Type: BuildType("unsupported")}), "source.git.build.type must be one of dockerfile, nixpacks, heroku_buildpacks, paketo_buildpacks, static, or railpack")
}

func TestApplicationBuildRejectsIncompatibleOptions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build ApplicationBuild
		want  string
	}{
		{"heroku version on paketo", ApplicationBuild{Type: BuildPaketoBuildpacks, HerokuVersion: appStringPtr("unrestricted")}, "source.git.build.herokuVersion is only valid for heroku_buildpacks builds"},
		{"heroku version on nixpacks", ApplicationBuild{Type: BuildNixpacks, HerokuVersion: appStringPtr("unrestricted")}, "source.git.build.herokuVersion is only valid for heroku_buildpacks builds"},
		{"railpack version on heroku", ApplicationBuild{Type: BuildHerokuBuildpacks, RailpackVersion: appStringPtr("unrestricted")}, "source.git.build.railpackVersion is only valid for railpack builds"},
		{"railpack version on static", ApplicationBuild{Type: BuildStatic, RailpackVersion: appStringPtr("unrestricted")}, "source.git.build.railpackVersion is only valid for railpack builds"},
		{"publish directory on static", ApplicationBuild{Type: BuildStatic, PublishDirectory: appStringPtr("dist")}, "source.git.build.publishDirectory is only valid for nixpacks builds"},
		{"publish directory on railpack", ApplicationBuild{Type: BuildRailpack, PublishDirectory: appStringPtr("dist")}, "source.git.build.publishDirectory is only valid for nixpacks builds"},
		{"spa on heroku", ApplicationBuild{Type: BuildHerokuBuildpacks, IsStaticSpa: appBoolPtr(false)}, "source.git.build.isStaticSpa is only valid for nixpacks or static builds"},
		{"spa on railpack", ApplicationBuild{Type: BuildRailpack, IsStaticSpa: appBoolPtr(false)}, "source.git.build.isStaticSpa is only valid for nixpacks or static builds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, validateBuild("source.git", tc.build), tc.want)
		})
	}
	for _, build := range []ApplicationBuild{
		{Type: BuildHerokuBuildpacks, HerokuVersion: appStringPtr("24")},
		{Type: BuildRailpack, RailpackVersion: appStringPtr("1")},
		{Type: BuildNixpacks, PublishDirectory: appStringPtr("dist"), IsStaticSpa: appBoolPtr(false)},
		{Type: BuildStatic, IsStaticSpa: appBoolPtr(false)},
	} {
		require.NoError(t, validateBuild("source.git", build))
	}
}

func TestApplicationBuildRequestShapes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build ApplicationBuild
		body  string
	}{
		{"dockerfile", ApplicationBuild{Type: BuildDockerfile, Dockerfile: appStringPtr("Dockerfile")}, `{"applicationId":"a1","buildType":"dockerfile","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":"Dockerfile","herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`},
		{"nixpacks", ApplicationBuild{Type: BuildNixpacks}, `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`},
		{"heroku", ApplicationBuild{Type: BuildHerokuBuildpacks, HerokuVersion: appStringPtr("24")}, `{"applicationId":"a1","buildType":"heroku_buildpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":"24","isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`},
		{"paketo", ApplicationBuild{Type: BuildPaketoBuildpacks}, `{"applicationId":"a1","buildType":"paketo_buildpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":null}`},
		{"nixpacks options", ApplicationBuild{Type: BuildNixpacks, PublishDirectory: appStringPtr("dist"), IsStaticSpa: appBoolPtr(false)}, `{"applicationId":"a1","buildType":"nixpacks","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":false,"publishDirectory":"dist","railpackVersion":null}`},
		{"static", ApplicationBuild{Type: BuildStatic, IsStaticSpa: appBoolPtr(true)}, `{"applicationId":"a1","buildType":"static","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":true,"publishDirectory":null,"railpackVersion":null}`},
		{"railpack", ApplicationBuild{Type: BuildRailpack, RailpackVersion: appStringPtr("0.2")}, `{"applicationId":"a1","buildType":"railpack","dockerBuildStage":null,"dockerContextPath":null,"dockerfile":null,"herokuVersion":null,"isStaticSpa":null,"publishDirectory":null,"railpackVersion":"0.2"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectPOST("/api/application.saveBuildType", tc.body, `true`))
			source := ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{Build: tc.build}}
			require.NoError(t, configureApplicationBuild(context.Background(), fixedClient(s.API())(context.Background()), "a1", source))
		})
	}
}

func TestApplicationBuildReadSelectedFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   string
		prior ApplicationBuild
		want  ApplicationBuild
	}{
		{"id only defaults to nixpacks", `{}`, ApplicationBuild{}, ApplicationBuild{Type: BuildNixpacks}},
		{"heroku selected field", `{"buildType":"heroku_buildpacks","herokuVersion":"24","railpackVersion":"stale","publishDirectory":"stale","isStaticSpa":true}`, ApplicationBuild{}, ApplicationBuild{Type: BuildHerokuBuildpacks, HerokuVersion: appStringPtr("24")}},
		{"railpack selected field", `{"buildType":"railpack","railpackVersion":"1","herokuVersion":"stale"}`, ApplicationBuild{}, ApplicationBuild{Type: BuildRailpack, RailpackVersion: appStringPtr("1")}},
		{"nixpacks preserves explicit false", `{"buildType":"nixpacks","isStaticSpa":false,"publishDirectory":"dist"}`, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false)}, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false), PublishDirectory: appStringPtr("dist")}},
		{"nixpacks default false omitted", `{"buildType":"nixpacks","isStaticSpa":false}`, ApplicationBuild{}, ApplicationBuild{Type: BuildNixpacks}},
		{"default false on changed builder omitted", `{"buildType":"static","isStaticSpa":false}`, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false)}, ApplicationBuild{Type: BuildStatic}},
		{"null clears prior", `{"buildType":"nixpacks","isStaticSpa":null,"publishDirectory":null}`, ApplicationBuild{IsStaticSpa: appBoolPtr(false), PublishDirectory: appStringPtr("old")}, ApplicationBuild{Type: BuildNixpacks}},
		{"static false preserved", `{"buildType":"static","isStaticSpa":false}`, ApplicationBuild{Type: BuildStatic, IsStaticSpa: appBoolPtr(false)}, ApplicationBuild{Type: BuildStatic, IsStaticSpa: appBoolPtr(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var values map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &values))
			require.Equal(t, tc.want, decodeBuild(values, tc.prior))
		})
	}
}

func TestApplicationBuildReadSelectedFieldsThroughApplicationRead(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       ApplicationSourceType
		sourceData string
		buildData  string
		prior      ApplicationBuild
		want       ApplicationBuild
	}{
		{"git id-only import defaults to nixpacks", SourceGit, `"customGitUrl":"repo","customGitBranch":"main"`, "", ApplicationBuild{}, ApplicationBuild{Type: BuildNixpacks}},
		{"gitlab id-only import defaults to nixpacks", SourceGitLab, `"gitlabId":"integration","gitlabProjectId":1,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main"`, "", ApplicationBuild{}, ApplicationBuild{Type: BuildNixpacks}},
		{"git prior explicit false survives server default", SourceGit, `"customGitUrl":"repo","customGitBranch":"main"`, `,"buildType":"nixpacks","isStaticSpa":false,"publishDirectory":"dist","herokuVersion":"stale","railpackVersion":"stale"`, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false)}, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false), PublishDirectory: appStringPtr("dist")}},
		{"gitlab prior explicit false survives server default", SourceGitLab, `"gitlabId":"integration","gitlabProjectId":1,"gitlabOwner":"owner","gitlabPathNamespace":"namespace","gitlabRepository":"repo","gitlabBranch":"main"`, `,"buildType":"nixpacks","isStaticSpa":false,"publishDirectory":"dist","herokuVersion":"stale","railpackVersion":"stale"`, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false)}, ApplicationBuild{Type: BuildNixpacks, IsStaticSpa: appBoolPtr(false), PublishDirectory: appStringPtr("dist")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"applicationId":"a1","type":"` + string(tc.kind) + `",` + tc.sourceData + tc.buildData + `}`
			s := newScriptedServer(t, expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, body))
			priorSource := ApplicationSource{Type: tc.kind}
			if tc.kind == SourceGit {
				priorSource.Git = &GitApplicationSource{Build: tc.prior}
			} else {
				priorSource.GitLab = &GitLabAppSource{Build: tc.prior}
			}
			prior := ApplicationState{ApplicationArgs: ApplicationArgs{Source: priorSource}}
			got, err := (Application{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: "a1", State: prior})
			require.NoError(t, err)
			build, ok := got.Inputs.Source.applicationBuild()
			require.True(t, ok)
			require.Equal(t, tc.want, build)
		})
	}
}

func TestApplicationBuildSelectionIsNilSafe(t *testing.T) {
	for _, source := range []ApplicationSource{
		{Type: SourceDocker, Docker: &DockerSource{Image: "nginx"}},
		{Type: SourceGit},
		{Type: SourceGitLab},
		{Type: SourceGit, Git: &GitApplicationSource{Build: ApplicationBuild{Type: BuildNixpacks}}},
	} {
		build, ok := source.applicationBuild()
		if source.Git == nil && source.GitLab == nil {
			require.False(t, ok)
			require.Equal(t, ApplicationBuild{}, build)
		} else {
			require.True(t, ok)
		}
	}
	for _, source := range []ApplicationSource{
		{Type: SourceGit},
		{Type: SourceGitLab},
		{Type: SourceGit, GitLab: &GitLabAppSource{}},
		{Type: ApplicationSourceType("unsupported"), Git: &GitApplicationSource{}},
	} {
		err := configureApplicationBuild(context.Background(), nil, "a1", source)
		require.ErrorContains(t, err, "source variant")
	}
	require.NoError(t, configureApplicationBuild(context.Background(), nil, "a1", ApplicationSource{Type: SourceDocker}))
}

func TestApplicationBuildOptionChangesProduceRuntimeDiff(t *testing.T) {
	oldBuild := ApplicationBuild{Type: BuildNixpacks, PublishDirectory: appStringPtr("dist")}
	newBuild := ApplicationBuild{Type: BuildNixpacks, PublishDirectory: appStringPtr("public")}
	diff, err := (Application{}).Diff(t.Context(), infer.DiffRequest[ApplicationArgs, ApplicationState]{
		Inputs: ApplicationArgs{EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "repo", Branch: "main", Build: newBuild}}},
		State:  ApplicationState{ApplicationArgs: ApplicationArgs{EnvironmentID: "e1", Source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "repo", Branch: "main", Build: oldBuild}}}},
	})
	require.NoError(t, err)
	require.Equal(t, p.Update, diff.DetailedDiff["source"].Kind)
}

func appBoolPtr(value bool) *bool { return &value }
