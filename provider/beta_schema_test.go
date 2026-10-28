package dokploy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Beta labels are documentation only: existing tokens and capabilities stay intact.
func TestSchemaBetaAdditions(t *testing.T) {
	spec := providerSchema(t)
	wantBeta := []string{
		"dokploy:index:GitLabIntegration", "dokploy:index:GitHubIntegration",
		"dokploy:index:GiteaIntegration", "dokploy:index:BitbucketIntegration",
	}
	var betaResources []string
	for token, resource := range spec.Resources {
		if strings.HasPrefix(resource.Description, "Beta. ") {
			betaResources = append(betaResources, token)
		}
	}
	require.ElementsMatch(t, wantBeta, betaResources)
	app := spec.Resources["dokploy:index:Application"]
	for _, field := range []string{app.InputProperties["autoDeploy"].Description, app.Properties["autoDeploy"].Description} {
		require.True(t, strings.HasPrefix(field, "Beta. "), field)
	}
	source := spec.Types["dokploy:index:ApplicationSource"]
	for _, name := range []string{"github", "gitea", "bitbucket"} {
		require.True(t, strings.HasPrefix(source.Properties[name].Description, "Beta. "), name)
	}
	for _, name := range []string{"docker", "git", "gitlab"} {
		require.NotContains(t, source.Properties[name].Description, "Beta", name)
	}
	for _, name := range []string{"GitHubAppSource", "GiteaAppSource", "BitbucketAppSource"} {
		require.True(t, strings.HasPrefix(spec.Types["dokploy:index:"+name].Description, "Beta. "), name)
	}
	build := spec.Types["dokploy:index:ApplicationBuild"]
	for _, name := range []string{"herokuVersion", "railpackVersion", "publishDirectory", "isStaticSpa"} {
		require.True(t, strings.HasPrefix(build.Properties[name].Description, "Beta. "), name)
	}
	for _, name := range []string{"dockerfile", "dockerContextPath", "dockerBuildStage"} {
		require.NotContains(t, build.Properties[name].Description, "Beta", name)
	}
	for _, builder := range []string{"heroku_buildpacks", "paketo_buildpacks", "static", "railpack"} {
		require.Contains(t, build.Properties["type"].Description, builder)
	}
	require.Contains(t, build.Properties["type"].Description, "Beta")
}
