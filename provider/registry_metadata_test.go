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

func hasExactRunCommand(steps []workflowRunStep, expected string) bool {
	for _, step := range steps {
		for _, line := range strings.Split(step.run, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "set ") {
				continue
			}
			if line == expected {
				return true
			}
		}
	}
	return false
}

var workflowJobPolicy = map[string]map[string]bool{
	"build.yml":                   {"prerequisites": true, "build_sdks": true, "tag_release_if_labeled_needs_release": false, "test": true, "publish": true, "publish_sdk": true, "lint": true},
	"command-dispatch.yml":        {"command-dispatch-for-testing": false},
	"comment-on-stale-issues.yml": {"cleanup": false},
	"community-moderation.yml":    {"warn_codegen": false},
	"export-repo-secrets.yml":     {"export-to-esc": false},
	"lint.yml":                    {"lint": true},
	"pages.yml":                   {"build": false, "deploy": false},
	"prerelease.yml":              {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true},
	"pull-request.yml":            {"comment-on-pr": false},
	"release_command.yml":         {"should_release": false},
	"release.yml":                 {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true, "dispatch_docs_build": false},
	"run-acceptance-tests.yml":    {"comment-notification": false, "prerequisites": true, "build_sdks": true, "test": true, "sentinel": false, "lint": true},
	"weekly-pulumi-update.yml":    {"weekly-pulumi-update": true},
}

func validateWorkflowSemantics(workflow map[string]any, name string) error {
	policy, ok := workflowJobPolicy[name]
	if !ok {
		return fmt.Errorf("workflow %q is not classified", name)
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok || len(jobs) != len(policy) {
		return fmt.Errorf("workflow %q job set is not exhaustively classified", name)
	}
	workflowEnv, _ := workflow["env"].(map[string]any)
	for jobName, executesGo := range policy {
		rawJob, exists := jobs[jobName]
		if !exists {
			return fmt.Errorf("workflow %q job %q is not classified", name, jobName)
		}
		job, _ := rawJob.(map[string]any)
		env := map[string]any{}
		for key, value := range workflowEnv {
			env[key] = value
		}
		if jobEnv, ok := job["env"].(map[string]any); ok {
			for key, value := range jobEnv {
				env[key] = value
			}
		}
		if executesGo && env["GOVERSION"] != releaseGoVersion {
			return fmt.Errorf("Go workflow job %s/%s does not resolve GOVERSION %s", name, jobName, releaseGoVersion)
		}
		if !executesGo {
			if steps, ok := job["steps"].([]any); ok {
				for _, rawStep := range steps {
					step, _ := rawStep.(map[string]any)
					if uses, _ := step["uses"].(string); strings.Contains(uses, "setup-go") || strings.Contains(uses, "setup-tools") {
						return fmt.Errorf("non-Go workflow job %s/%s invokes Go/project setup", name, jobName)
					}
				}
			}
			for _, step := range workflowJobRunSteps(workflow, jobName) {
				if strings.Contains(step.run, "make ") || strings.Contains(step.run, "go ") || strings.Contains(step.run, "setup-go") || strings.Contains(step.run, "setup-tools") {
					return fmt.Errorf("non-Go workflow job %s/%s invokes Go tooling", name, jobName)
				}
			}
		}
	}
	return nil
}

func TestRegistryMetadata(t *testing.T) {
	spec := providerSchema(t)
	require.Equal(t, "https://github.com/dimeskigj/pulumi-dokploy", spec.Repository)
	require.Equal(t, "Apache-2.0", spec.License)
	require.Equal(t, "dimeskigj", spec.Publisher)
	require.NotEmpty(t, spec.Description)
	require.Equal(t, "@dimeskigj/pulumi-dokploy", languageSetting(spec, "nodejs", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "packageName"))
	require.Equal(t, "pulumi_dokploy", languageSetting(spec, "python", "moduleName"))
	require.Equal(t, "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy", languageSetting(spec, "go", "importBasePath"))
	require.Equal(t, "Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
	require.Equal(t, "net.dimeski.pulumi.dokploy", languageSetting(spec, "java", "packageName"))
	for token, resource := range spec.Resources {
		require.NotEmpty(t, resource.Description, token)
	}
	require.Len(t, spec.Resources, 18)
	for _, token := range []string{"dokploy:index:SSHKey", "dokploy:index:Registry", "dokploy:index:Tag", "dokploy:index:ProjectTag", "dokploy:index:Mount"} {
		require.Contains(t, spec.Resources, token)
	}

	readme, err := os.ReadFile("../README.md")
	require.NoError(t, err)
	docs := string(readme)
	for _, section := range []string{
		"Node.js", "Python", "Go", ".NET", "Java", "YAML",
		"dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY",
		"dokploy:index:Project", "dokploy:index:Environment", "dokploy:index:Application",
		"dokploy:index:Compose", "dokploy:index:Postgres", "dokploy:index:MySQL", "dokploy:index:MariaDB",
		"dokploy:index:MongoDB", "dokploy:index:Redis", "dokploy:index:Domain",
		"dokploy:index:Destination", "dokploy:index:Backup", "dokploy:index:VolumeBackup",
		"GitLab", "pulumi import dokploy:index:Project", "wait", "secret",
		"SSH", "volume", "partial state",
		"pulumi config set dokploy:endpoint https://dokploy.example.com",
		"pulumi config set --secret dokploy:apiKey \"$DOKPLOY_API_KEY\"",
		"Project owns the default environment", "Source type changes replace",
		"referenced GitLab integration is not managed", "SSH key references are likewise passed through",
		"deployment errors preserve partial state", "Compose volumes are preserved",
		"SSHKey", "Registry", "Tag", "ProjectTag", "Mount", "registryPassword", "sshPrivateKey",
		"pulumi import dokploy:index:SSHKey", "pulumi import dokploy:index:Registry", "pulumi import dokploy:index:Tag",
		"pulumi import dokploy:index:ProjectTag", "pulumi import dokploy:index:Mount",
		"MongoDB", "LibSQL", "automatic mount redeployment", "testRegistry",
	} {
		require.True(t, strings.Contains(docs, section), "README missing %q", section)
	}

	workflowNames := make([]string, 0, len(workflowJobPolicy))
	for name := range workflowJobPolicy {
		workflowNames = append(workflowNames, name)
	}
	allWorkflowFiles, err := os.ReadDir("../.github/workflows")
	require.NoError(t, err)
	actualWorkflowNames := make([]string, 0, len(allWorkflowFiles))
	for _, file := range allWorkflowFiles {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yml") {
			actualWorkflowNames = append(actualWorkflowNames, file.Name())
		}
	}
	require.ElementsMatch(t, workflowNames, actualWorkflowNames)
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
		require.NoError(t, validateWorkflowSemantics(parsed, workflow), workflow)
	}
	require.NotContains(t, allWorkflowText, "1.21.x", "stale workflow Go pin")
	require.NotContains(t, allWorkflowText, "1.25.11", "stale workflow Go pin")
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
	gateRuns := map[string]string{
		"lint": "make lint", "race": "make test_race",
		"openapi and codegen": "make check_openapi && make check_codegen",
		"SDKs":                "make build_sdks", "examples": "make test_examples",
		"vulnerability": "make govulncheck", "license": "make license",
	}
	for name, command := range gateRuns {
		if name == "lint" {
			require.True(t, hasExactRunCommand(workflowJobRunSteps(mustWorkflow(t, "lint.yml"), "lint"), command), "missing complete %s gate run step", name)
		} else {
			require.True(t, hasExactRunCommand(workflowJobRunSteps(build, "prerequisites"), command), "missing complete %s gate run step", name)
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
	require.Contains(t, dispatchText, "repository: dimeskigj/pulumi-dokploy")
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
			switch name {
			case "missing GOVERSION":
				require.True(t, hasExactRunCommand(workflowRunSteps(workflow), "make lint"))
				require.Error(t, validateWorkflowSemantics(workflow, "lint.yml"))
			case "wrong job":
				require.True(t, hasExactRunCommand(workflowRunSteps(workflow), "make lint"))
				require.False(t, hasExactRunCommand(workflowJobRunSteps(workflow, "build"), "make lint"))
			default:
				require.False(t, hasExactRunCommand(workflowRunSteps(workflow), "make lint"))
			}
		})
	}
}
