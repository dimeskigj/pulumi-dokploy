package dokploy

import (
	"fmt"
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

type workflowRunStep struct {
	run string
	env map[string]any
}

func workflowRunSteps(workflow map[string]any) []workflowRunStep {
	return workflowJobRunSteps(workflow, "")
}

func workflowJobRunSteps(workflow map[string]any, wantedJob string) []workflowRunStep {
	workflowEnv, _ := workflow["env"].(map[string]any)
	jobs, _ := workflow["jobs"].(map[string]any)
	var result []workflowRunStep
	for jobName, rawJob := range jobs {
		if wantedJob != "" && jobName != wantedJob {
			continue
		}
		job, ok := rawJob.(map[string]any)
		if !ok {
			continue
		}
		env := map[string]any{}
		for key, value := range workflowEnv {
			env[key] = value
		}
		if jobEnv, ok := job["env"].(map[string]any); ok {
			for key, value := range jobEnv {
				env[key] = value
			}
		}
		steps, _ := job["steps"].([]any)
		for _, rawStep := range steps {
			step, ok := rawStep.(map[string]any)
			if !ok {
				continue
			}
			run, ok := step["run"].(string)
			if !ok {
				continue
			}
			stepEnv := map[string]any{}
			for key, value := range env {
				stepEnv[key] = value
			}
			if values, ok := step["env"].(map[string]any); ok {
				for key, value := range values {
					stepEnv[key] = value
				}
			}
			result = append(result, workflowRunStep{run: run, env: stepEnv})
		}
	}
	return result
}

func hasRunStep(steps []workflowRunStep, fragments ...string) bool {
	for _, step := range steps {
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(step.run, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func isToolchainRun(run string) bool {
	for _, fragment := range []string{"go ", "make ", "golangci", "govuln", "license"} {
		if strings.Contains(run, fragment) {
			return true
		}
	}
	return false
}

func validateWorkflowSemantics(workflow map[string]any) error {
	steps := workflowRunSteps(workflow)
	for _, step := range steps {
		if isToolchainRun(step.run) && step.env["GOVERSION"] != releaseGoVersion {
			return fmt.Errorf("toolchain run %q does not resolve GOVERSION %s", step.run, releaseGoVersion)
		}
	}
	return nil
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
		require.NoError(t, validateWorkflowSemantics(parsed), workflow)
	}
	build, buildText := readWorkflow(t, "build.yml")
	on, ok := build["on"].(map[string]any)
	require.True(t, ok)
	push, ok := on["push"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"master", "main", "feature-**"}, push["branches"])
	require.Equal(t, []any{"v*", "sdk/*"}, push["tags-ignore"])
	require.NotContains(t, push, "paths-ignore")
	require.NotContains(t, push["tags-ignore"], "**")
	require.Equal(t, map[string]any{}, on["pull_request"])
	require.Equal(t, map[string]any{}, on["workflow_dispatch"])
	require.Equal(t, 1, strings.Count(buildText, "  push:\n"), "build has exactly one push trigger")
	require.Equal(t, 1, strings.Count(buildText, "  pull_request: {}\n"), "build has exactly one pull_request trigger")
	require.Equal(t, 0, strings.Count(allWorkflowText, "paths-ignore:"), "no workflow has paths-ignore")
	gateRuns := map[string][]string{
		"lint": {"make lint"}, "race": {"make test_race"},
		"openapi and codegen": {"make check_openapi", "make check_codegen"},
		"SDKs":                {"make build_sdks"}, "examples": {"make test_examples"},
		"vulnerability": {"make govulncheck"}, "license": {"make license"},
	}
	for name, fragments := range gateRuns {
		if name == "lint" {
			require.True(t, hasRunStep(workflowJobRunSteps(mustWorkflow(t, "lint.yml"), "lint"), fragments...), "missing complete %s gate run step", name)
		} else {
			require.True(t, hasRunStep(workflowJobRunSteps(build, "prerequisites"), fragments...), "missing complete %s gate run step", name)
		}
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

func mustWorkflow(t *testing.T, name string) map[string]any {
	t.Helper()
	workflow, _ := readWorkflow(t, name)
	return workflow
}

func TestWorkflowSemanticFixturesRejectMetadataAndWrongJobs(t *testing.T) {
	for name, fixture := range map[string]string{
		"comment and name": `name: make lint
# make test_race
jobs:
  check:
    steps:
      - name: make lint
        run: echo unrelated`,
		"wrong job": `jobs:
  unrelated:
    name: make lint
    steps:
      - run: make lint`,
		"missing GOVERSION": `jobs:
  build:
    steps:
      - run: make lint`,
	} {
		t.Run(name, func(t *testing.T) {
			workflow := map[string]any{}
			require.NoError(t, yaml.Unmarshal([]byte(fixture), &workflow))
			if name == "missing GOVERSION" {
				require.True(t, hasRunStep(workflowRunSteps(workflow), "make lint"))
				require.Error(t, validateWorkflowSemantics(workflow))
			} else if name == "wrong job" {
				require.True(t, hasRunStep(workflowRunSteps(workflow), "make lint"))
				require.False(t, hasRunStep(workflowJobRunSteps(workflow, "build"), "make lint"))
			} else {
				require.False(t, hasRunStep(workflowRunSteps(workflow), "make lint"))
			}
		})
	}
}
