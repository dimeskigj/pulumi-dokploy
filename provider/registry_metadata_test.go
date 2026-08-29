package dokploy

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const releaseGoVersion = "1.25.13"

func readWorkflow(t *testing.T, name string) (map[string]any, string) {
	t.Helper()
	content, err := os.ReadFile("../.github/workflows/" + name)
	require.NoError(t, err)
	workflow := map[string]any{}
	require.NoError(t, yaml.Unmarshal(content, &workflow))
	return workflow, string(content)
}

func TestRegistryMetadata(t *testing.T) {
	spec := providerSchema(t)
	require.Equal(t, "https://github.com/gjorgjidimeski/pulumi-dokploy", spec.Repository)
	require.Equal(t, "Apache-2.0", spec.License)
	require.Equal(t, "gjorgjidimeski", spec.Publisher)
	require.NotEmpty(t, spec.Description)
	require.Equal(t, "@gjorgjidimeski/pulumi-dokploy", languageSetting(spec, "nodejs", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "moduleName"))
	require.Equal(t, "github.com/gjorgjidimeski/pulumi-dokploy/sdk/go/dokploy", languageSetting(spec, "go", "importBasePath"))
	require.Equal(t, "Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
	require.Equal(t, "dev.codechem.pulumi.dokploy", languageSetting(spec, "java", "packageName"))
	for token, resource := range spec.Resources {
		require.NotEmpty(t, resource.Description, token)
	}

	readme, err := os.ReadFile("../README.md")
	require.NoError(t, err)
	docs := string(readme)
	for _, section := range []string{
		"Node.js", "Python", "Go", ".NET", "Java", "YAML",
		"dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY",
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:Redis", "dokploy:index:Domain",
		"GitLab", "pulumi import dokploy:index:Project", "wait", "secret",
		"SSH", "volume", "partial state",
		"pulumi config set dokploy:endpoint https://dokploy.example.com",
		"pulumi config set --secret dokploy:apiKey \"$DOKPLOY_API_KEY\"",
		"Project owns the default environment", "Source type changes replace",
		"referenced GitLab integration is not managed", "SSH key references are likewise passed through",
		"deployment errors preserve partial state", "Compose volumes are preserved",
	} {
		require.True(t, strings.Contains(docs, section), "README missing %q", section)
	}

	workflowNames := []string{"build.yml", "command-dispatch.yml", "comment-on-stale-issues.yml", "community-moderation.yml", "export-repo-secrets.yml", "lint.yml", "prerelease.yml", "pull-request.yml", "release_command.yml", "release.yml", "run-acceptance-tests.yml", "weekly-pulumi-update.yml"}
	shaPattern := regexp.MustCompile(`uses:\s+[^@\s]+@([^\s#]+)`)
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	allWorkflowText := ""
	for _, workflow := range workflowNames {
		parsed, text := readWorkflow(t, workflow)
		allWorkflowText += text
		require.NotContains(t, text, "paths-ignore:\n    - \"**\"", workflow)
		require.NotContains(t, text, "@main", workflow)
		for _, match := range shaPattern.FindAllStringSubmatch(text, -1) {
			require.Regexp(t, sha, match[1], workflow)
		}
		if env, ok := parsed["env"].(map[string]any); ok {
			if version, ok := env["GOVERSION"]; ok {
				require.Equal(t, releaseGoVersion, version, workflow)
			}
		}
	}
	build, _ := readWorkflow(t, "build.yml")
	require.Contains(t, build["on"], "push")
	require.Contains(t, build["on"], "pull_request")
	for _, gate := range []string{"make lint", "make test_race", "make check_openapi", "make check_codegen", "make build_sdks", "make test_examples", "make govulncheck", "make license"} {
		require.Contains(t, allWorkflowText, gate)
	}
	release, releaseText := readWorkflow(t, "release.yml")
	prerelease, prereleaseText := readWorkflow(t, "prerelease.yml")
	require.Contains(t, release["on"], "push")
	require.Contains(t, prerelease["on"], "push")
	require.Contains(t, releaseText, "-f .goreleaser.yml")
	require.Contains(t, releaseText, "goreleaser/goreleaser-action@")
	require.NotContains(t, releaseText, "-f .goreleaser.prerelease.yml")
	require.Contains(t, prereleaseText, "-f .goreleaser.prerelease.yml")
	acceptance, acceptanceText := readWorkflow(t, "run-acceptance-tests.yml")
	require.Contains(t, acceptance["on"], "workflow_dispatch")
	require.NotContains(t, acceptanceText, "  pull_request:\n")
	require.Contains(t, acceptanceText, "environment: dokploy-acceptance")
	require.Contains(t, acceptanceText, "DOKPLOY_ENDPOINT: ${{ steps.esc-secrets.outputs.DOKPLOY_ENDPOINT }}")
	require.Contains(t, acceptanceText, "DOKPLOY_API_KEY: ${{ steps.esc-secrets.outputs.DOKPLOY_API_KEY }}")
	require.Contains(t, acceptanceText, `test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY"`)
	dispatch, dispatchText := readWorkflow(t, "command-dispatch.yml")
	require.NotNil(t, dispatch)
	require.Contains(t, dispatchText, "repository: gjorgjidimeski/pulumi-dokploy")
	for _, file := range []string{".mise.toml", "go.mod", "examples/go/go.mod"} {
		content, err := os.ReadFile("../" + file)
		require.NoError(t, err)
		require.Contains(t, string(content), releaseGoVersion, file)
		require.NotContains(t, string(content), "1.25.11", file)
	}
}
