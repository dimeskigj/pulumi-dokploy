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

const releaseGoVersion = "1.26.0"

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
	"build.yml":                {"prerequisites": true, "build_sdks": true, "test": true, "lint": true},
	"lint.yml":                 {"lint": true},
	"pages.yml":                {"build": false, "deploy": false},
	"prerelease.yml":           {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true},
	"release.yml":              {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true, "dispatch_docs_build": false},
	"run-acceptance-tests.yml": {"prerequisites": true, "build_sdks": true, "test": true, "lint": true},
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
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".yml") || strings.HasSuffix(file.Name(), ".yaml")) {
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
		assertDokploySecretsAreAcceptanceOnly(t, workflow, parsed)
		if workflow == "release.yml" || workflow == "prerelease.yml" {
			assertCheckoutJobsHaveReadContents(t, workflow, parsed, "prerequisites", "build_sdks")
		}
		if workflow == "run-acceptance-tests.yml" {
			require.NoError(t, validateAcceptanceWorkflowContracts(parsed))
		}
		require.NoError(t, validateWorkflowSemantics(parsed, workflow), workflow)
	}
	require.NotContains(t, allWorkflowText, "1.21.x", "stale workflow Go pin")
	require.NotContains(t, allWorkflowText, "1.25.11", "stale workflow Go pin")
	for _, forbidden := range []string{
		"pulumi-ubuntu-8core", "pulumi-gen-",
		"pulumi/esc-action@", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AWS_UPLOAD_ROLE_ARN", "blobs:",
	} {
		require.NotContains(t, allWorkflowText, forbidden, "owned workflows must not contain %q", forbidden)
	}
	build, buildText := readWorkflow(t, "build.yml")
	require.NotContains(t, buildText, "  publish:\n")
	require.NotContains(t, buildText, "  publish_sdk:\n")
	require.Contains(t, build["on"], "pull_request")
	require.Contains(t, build["on"], "push")
	require.Contains(t, buildText, "make docs_check")
	require.Contains(t, buildText, "make check_openapi && make check_codegen")
	require.Contains(t, buildText, "make test_race")
	require.Contains(t, buildText, "make test_examples")
	require.NotContains(t, buildText, "Configure AWS Credentials")
	on, ok := build["on"].(map[string]any)
	require.True(t, ok)
	push, ok := on["push"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"main", "feat/**", "fix/**"}, push["branches"])
	require.NotContains(t, push, "tags-ignore")
	require.NotContains(t, push, "paths-ignore")
	require.Equal(t, map[string]any{}, on["pull_request"])
	require.Equal(t, map[string]any{}, on["workflow_dispatch"])
	require.Equal(t, 1, strings.Count(buildText, "  push:\n"), "build has exactly one push trigger")
	require.Equal(t, 1, strings.Count(buildText, "  pull_request: {}\n"), "build has exactly one pull_request trigger")
	require.Equal(t, 0, strings.Count(allWorkflowText, "paths-ignore:"), "no workflow has paths-ignore")
	gateRuns := map[string]string{
		"lint": "make lint", "race": "make test_race",
		"openapi and codegen": "make check_openapi && make check_codegen",
		"SDKs":                "make build_sdks", "examples": "make test_examples",
		"provider": "make provider", "vulnerability": "make govulncheck", "license": "make license",
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
	require.Equal(t, []any{"v*.*.*", "!v*.*.*-*"}, release["on"].(map[string]any)["push"].(map[string]any)["tags"])
	require.Equal(t, []any{"v*.*.*-*"}, prerelease["on"].(map[string]any)["push"].(map[string]any)["tags"])
	for name, workflow := range map[string]map[string]any{"release.yml": release, "prerelease.yml": prerelease} {
		jobs := workflow["jobs"].(map[string]any)
		require.NoError(t, validateReleaseWorkflowContracts(workflow, name))
		publish := jobs["publish"].(map[string]any)
		require.Equal(t, "write", publish["permissions"].(map[string]any)["contents"], name)
		var goreleaserEnv map[string]any
		for _, rawStep := range publish["steps"].([]any) {
			step := rawStep.(map[string]any)
			if uses, _ := step["uses"].(string); strings.Contains(uses, "goreleaser/goreleaser-action@") {
				goreleaserEnv = step["env"].(map[string]any)
			}
		}
		require.NotNil(t, goreleaserEnv, name)
		require.Equal(t, "${{ secrets.GITHUB_TOKEN }}", goreleaserEnv["GITHUB_TOKEN"], name)
		require.Equal(t, "publish", jobs["publish_sdk"].(map[string]any)["needs"], name)
		require.Equal(t, "publish_sdk", jobs["publish_go_sdk"].(map[string]any)["needs"], name)

		goSDK := jobs["publish_go_sdk"].(map[string]any)
		var hasProviderVersionAction bool
		var goPublisherVersion any
		for _, rawStep := range goSDK["steps"].([]any) {
			step := rawStep.(map[string]any)
			uses, _ := step["uses"].(string)
			if strings.Contains(uses, "pulumi/provider-version-action@") && step["id"] == "version" {
				hasProviderVersionAction = true
			}
			if strings.Contains(uses, "pulumi/publish-go-sdk-action@") {
				goPublisherVersion = step["with"].(map[string]any)["version"]
			}
		}
		require.True(t, hasProviderVersionAction, name)
		require.Equal(t, "${{ steps.version.outputs.version }}", goPublisherVersion, name)
	}
	require.NoError(t, validateArtifactActionContracts())
	for _, workflowText := range []string{releaseText, prereleaseText} {
		require.NotContains(t, workflowText, "aws-actions/configure-aws-credentials")
		require.NotContains(t, workflowText, "AWS_")
	}
	for _, configName := range []string{".goreleaser.yml", ".goreleaser.prerelease.yml"} {
		config, err := os.ReadFile("../" + configName)
		require.NoError(t, err)
		configText := string(config)
		require.NotContains(t, configText, "blobs:", configName)
		require.Contains(t, configText, "release:\n  prerelease: auto\n  disable: false", configName)
	}
	acceptance, acceptanceText := readWorkflow(t, "run-acceptance-tests.yml")
	triggers, ok := acceptance["on"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, triggers, "workflow_dispatch")
	require.Len(t, triggers, 1, "acceptance workflow must only use workflow_dispatch")
	require.NotContains(t, acceptanceText, "  pull_request:\n")
	require.NotContains(t, acceptanceText, "repository_dispatch")
	require.NotContains(t, acceptanceText, "comment-notification")
	require.NotContains(t, acceptanceText, "sentinel")
	require.NotContains(t, acceptanceText, "esc-secrets")
	require.Contains(t, acceptanceText, "environment: dokploy-acceptance")
	require.Contains(t, acceptanceText, "mise exec -- go test ./provider -run TestLive -v -count=1")
	require.Contains(t, acceptanceText, `test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY" && test -n "$DOKPLOY_REGISTRY_URL" && test -n "$DOKPLOY_REGISTRY_USERNAME" && test -n "$DOKPLOY_REGISTRY_PASSWORD"`)
	for _, variable := range []string{
		"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY", "DOKPLOY_REGISTRY_URL",
		"DOKPLOY_REGISTRY_USERNAME", "DOKPLOY_REGISTRY_PASSWORD", "DOKPLOY_REGISTRY_IMAGE_PREFIX",
	} {
		require.Contains(t, acceptanceText, variable+": ${{ secrets."+variable+" }}")
	}
	jobs := acceptance["jobs"].(map[string]any)
	protectedJobs := make([]string, 0, 2)
	for jobName, rawJob := range jobs {
		job := rawJob.(map[string]any)
		if job["environment"] == "dokploy-acceptance" {
			protectedJobs = append(protectedJobs, jobName)
		}
	}
	require.ElementsMatch(t, []string{"prerequisites"}, protectedJobs)
	lintJob := jobs["lint"].(map[string]any)
	require.NotContains(t, lintJob, "environment", "lint must not use the acceptance environment")
	require.NotContains(t, lintJob, "secrets", "lint must not inherit caller secrets")
	for _, path := range []string{"../.ci-" + "mgmt.yaml", "../scripts/normalize_ci.py", "../scripts/test_normalize_ci.py"} {
		_, err := os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist, path)
	}
	makefile, err := os.ReadFile("../Makefile")
	require.NoError(t, err)
	require.NotRegexp(t, regexp.MustCompile(`(?m)^ci-mgmt[ \t]*:`), string(makefile))
	for _, section := range []string{
		"pulumi plugin install resource dokploy \"$VERSION\" \\\n  --server \"https://github.com/dimeskigj/pulumi-dokploy/releases/download/v$VERSION\"",
	} {
		require.Contains(t, docs, section, "README missing %q", section)
	}
	installation, err := os.ReadFile("../website/src/content/docs/getting-started/installation.mdx")
	require.NoError(t, err)
	require.Contains(t, string(installation), "pulumi plugin install resource dokploy \"$VERSION\" \\\n  --server \"https://github.com/dimeskigj/pulumi-dokploy/releases/download/v$VERSION\"")
	for _, file := range []string{".mise.toml", "go.mod", "examples/go/go.mod"} {
		content, err := os.ReadFile("../" + file)
		require.NoError(t, err)
		require.Contains(t, string(content), releaseGoVersion, file)
		require.NotContains(t, string(content), "1.25.11", file)
	}
}

func validateReleaseWorkflowContracts(workflow map[string]any, name string) error {
	j, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no jobs", name)
	}
	for jobName, rawJob := range j {
		job, ok := rawJob.(map[string]any)
		if !ok {
			return fmt.Errorf("%s job %s is not a mapping", name, jobName)
		}
		permissions, ok := job["permissions"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s job %s has no explicit permissions", name, jobName)
		}
		if _, exists := job["continue-on-error"]; exists {
			return fmt.Errorf("%s job %s must not continue on error", name, jobName)
		}
		expected := map[string]any{"contents": "read"}
		if jobName == "publish" || jobName == "publish_go_sdk" {
			expected = map[string]any{"contents": "write"}
		}
		if len(permissions) != len(expected) {
			return fmt.Errorf("%s job %s has unexpected permissions", name, jobName)
		}
		for permission, expectedValue := range expected {
			if permissions[permission] != expectedValue {
				return fmt.Errorf("%s job %s permission %s must be %v", name, jobName, permission, expectedValue)
			}
		}
	}

	prerequisites, ok := j["prerequisites"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no prerequisites job", name)
	}
	providerUpload := findWorkflowStep(prerequisites, "actions/upload-artifact@")
	if providerUpload == nil {
		return fmt.Errorf("%s prerequisites does not upload provider artifact", name)
	}
	providerWith, _ := providerUpload["with"].(map[string]any)
	if providerWith["name"] != "pulumi-${{ env.PROVIDER }}-provider.tar.gz" || providerWith["path"] != "${{ github.workspace }}/bin/provider.tar.gz" {
		return fmt.Errorf("%s provider artifact contract is incorrect", name)
	}

	buildSDKs, ok := j["build_sdks"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no build_sdks job", name)
	}
	sdkUpload := findWorkflowStep(buildSDKs, "actions/upload-artifact@")
	if sdkUpload == nil {
		return fmt.Errorf("%s build_sdks does not upload SDK artifact", name)
	}
	sdkWith, _ := sdkUpload["with"].(map[string]any)
	if sdkWith["name"] != "${{ matrix.language  }}-sdk.tar.gz" || sdkWith["path"] != "${{ github.workspace}}/sdk/${{ matrix.language }}.tar.gz" {
		return fmt.Errorf("%s SDK artifact producer contract is incorrect", name)
	}

	for jobName, language := range map[string]string{"publish_sdk": "python", "publish_java_sdk": "java", "publish_go_sdk": "go"} {
		job, ok := j[jobName].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has no %s job", name, jobName)
		}
		download := findWorkflowStep(job, "actions/download-artifact@")
		if download == nil {
			return fmt.Errorf("%s %s does not download SDK artifact", name, jobName)
		}
		with, _ := download["with"].(map[string]any)
		expectedName := language + "-sdk.tar.gz"
		if with["name"] != expectedName || with["path"] != "${{ github.workspace}}/sdk/" {
			return fmt.Errorf("%s %s SDK artifact consumer contract is incorrect", name, jobName)
		}
	}
	return nil
}

func validateAcceptanceWorkflowContracts(workflow map[string]any) error {
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("acceptance workflow has no jobs")
	}
	expectedJobs := map[string]bool{"prerequisites": true, "build_sdks": true, "test": true, "lint": true}
	if len(jobs) != len(expectedJobs) {
		return fmt.Errorf("acceptance workflow job set is not exhaustive")
	}
	for jobName := range expectedJobs {
		rawJob, exists := jobs[jobName]
		if !exists {
			return fmt.Errorf("acceptance job %s is missing", jobName)
		}
		job, ok := rawJob.(map[string]any)
		if !ok {
			return fmt.Errorf("acceptance job %s is not a mapping", jobName)
		}
		permissions, ok := job["permissions"].(map[string]any)
		if !ok {
			return fmt.Errorf("acceptance job %s has no explicit permissions", jobName)
		}
		if len(permissions) != 1 || permissions["contents"] != "read" {
			return fmt.Errorf("acceptance job %s has unexpected permissions", jobName)
		}
	}
	return nil
}

func validateArtifactActionContracts() error {
	providerContent, err := os.ReadFile("../.github/actions/download-provider/action.yml")
	if err != nil {
		return err
	}
	provider := map[string]any{}
	if err := yaml.Unmarshal(providerContent, &provider); err != nil {
		return err
	}
	providerStep := findCompositeStep(provider, "actions/download-artifact@")
	if providerStep == nil {
		return fmt.Errorf("provider artifact consumer action has no download step")
	}
	providerWith, _ := providerStep["with"].(map[string]any)
	if providerWith["name"] != "pulumi-${{ env.PROVIDER }}-provider.tar.gz" || providerWith["path"] != "${{ github.workspace }}/bin" {
		return fmt.Errorf("provider artifact consumer action contract is incorrect")
	}
	if !strings.Contains(string(providerContent), "pulumi plugin install resource ${{ env.PROVIDER }} 0.0.1-alpha.0+dev --file ${{ github.workspace }}/bin/pulumi-resource-${{ env.PROVIDER }} --reinstall") {
		return fmt.Errorf("provider artifact consumer action does not install the local provider plugin")
	}

	sdkContent, err := os.ReadFile("../.github/actions/download-sdk/action.yml")
	if err != nil {
		return err
	}
	sdk := map[string]any{}
	if err := yaml.Unmarshal(sdkContent, &sdk); err != nil {
		return err
	}
	sdkStep := findCompositeStep(sdk, "actions/download-artifact@")
	if sdkStep == nil {
		return fmt.Errorf("SDK artifact consumer action has no download step")
	}
	sdkWith, _ := sdkStep["with"].(map[string]any)
	if sdkWith["name"] != "${{ inputs.language }}-sdk.tar.gz" || sdkWith["path"] != "${{ github.workspace }}/sdk/" {
		return fmt.Errorf("SDK artifact consumer action contract is incorrect")
	}
	return nil
}

func findCompositeStep(action map[string]any, actionPrefix string) map[string]any {
	runs, _ := action["runs"].(map[string]any)
	steps, _ := runs["steps"].([]any)
	for _, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		uses, _ := step["uses"].(string)
		if strings.HasPrefix(uses, actionPrefix) {
			return step
		}
	}
	return nil
}

func findWorkflowStep(job map[string]any, actionPrefix string) map[string]any {
	steps, _ := job["steps"].([]any)
	for _, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		uses, _ := step["uses"].(string)
		if strings.HasPrefix(uses, actionPrefix) {
			return step
		}
	}
	return nil
}

func TestReleaseWorkflowContractsRejectRepresentativeDrift(t *testing.T) {
	for _, name := range []string{"release.yml", "prerelease.yml"} {
		t.Run(name+" permission drift", func(t *testing.T) {
			workflow, _ := readWorkflow(t, name)
			delete(workflow["jobs"].(map[string]any)["publish_java_sdk"].(map[string]any), "continue-on-error")
			jobs := workflow["jobs"].(map[string]any)
			prerequisites := jobs["prerequisites"].(map[string]any)
			permissions := prerequisites["permissions"].(map[string]any)
			permissions["pull-requests"] = "write"
			require.ErrorContains(t, validateReleaseWorkflowContracts(workflow, name), "unexpected permissions")
		})

		t.Run(name+" artifact drift", func(t *testing.T) {
			workflow, _ := readWorkflow(t, name)
			delete(workflow["jobs"].(map[string]any)["publish_java_sdk"].(map[string]any), "continue-on-error")
			jobs := workflow["jobs"].(map[string]any)
			prerequisites := jobs["prerequisites"].(map[string]any)
			providerUpload := findWorkflowStep(prerequisites, "actions/upload-artifact@")
			providerWith := providerUpload["with"].(map[string]any)
			providerWith["path"] = "wrong/provider.tar.gz"
			require.ErrorContains(t, validateReleaseWorkflowContracts(workflow, name), "provider artifact contract")
		})

		t.Run(name+" Java failure drift", func(t *testing.T) {
			workflow, _ := readWorkflow(t, name)
			jobs := workflow["jobs"].(map[string]any)
			jobs["publish_java_sdk"].(map[string]any)["continue-on-error"] = true
			require.ErrorContains(t, validateReleaseWorkflowContracts(workflow, name), "must not continue on error")
		})
	}
}

func TestAcceptanceWorkflowContractsRejectRepresentativePermissionDrift(t *testing.T) {
	workflow, _ := readWorkflow(t, "run-acceptance-tests.yml")
	jobs := workflow["jobs"].(map[string]any)
	for jobName := range map[string]bool{"build_sdks": true, "test": true, "lint": true} {
		jobs[jobName].(map[string]any)["permissions"] = map[string]any{"contents": "read"}
	}
	jobs["test"].(map[string]any)["permissions"] = map[string]any{"contents": "write"}
	require.ErrorContains(t, validateAcceptanceWorkflowContracts(workflow), "unexpected permissions")

	workflow, _ = readWorkflow(t, "run-acceptance-tests.yml")
	delete(workflow["jobs"].(map[string]any)["lint"].(map[string]any), "permissions")
	require.ErrorContains(t, validateAcceptanceWorkflowContracts(workflow), "no explicit permissions")
}

func TestWorkflowStepsDoNotHaveEmptyEnvMappings(t *testing.T) {
	entries, err := os.ReadDir("../.github/workflows")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		workflow, _ := readWorkflow(t, entry.Name())
		jobs, ok := workflow["jobs"].(map[string]any)
		require.True(t, ok, entry.Name())
		for jobName, rawJob := range jobs {
			job, ok := rawJob.(map[string]any)
			require.True(t, ok, "%s job %s", entry.Name(), jobName)
			steps, _ := job["steps"].([]any)
			for stepIndex, rawStep := range steps {
				step, ok := rawStep.(map[string]any)
				require.True(t, ok, "%s job %s step %d", entry.Name(), jobName, stepIndex)
				env, exists := step["env"]
				if !exists {
					continue
				}
				envMap, ok := env.(map[string]any)
				require.True(t, ok, "%s job %s step %d env must be a mapping", entry.Name(), jobName, stepIndex)
				require.NotEmpty(t, envMap, "%s job %s step %d has an empty env mapping", entry.Name(), jobName, stepIndex)
			}
		}
	}
}

func mustWorkflow(t *testing.T, name string) map[string]any {
	t.Helper()
	workflow, _ := readWorkflow(t, name)
	return workflow
}

func assertDokploySecretsAreAcceptanceOnly(t *testing.T, name string, workflow map[string]any) {
	t.Helper()
	jobs, ok := workflow["jobs"].(map[string]any)
	require.True(t, ok, name)
	workflowMetadata := map[string]any{}
	for key, value := range workflow {
		if key != "jobs" {
			workflowMetadata[key] = value
		}
	}
	require.NotContains(t, workflowValueText(workflowMetadata), "secrets.DOKPLOY_", "%s workflow metadata", name)
	for jobName, rawJob := range jobs {
		job, ok := rawJob.(map[string]any)
		require.True(t, ok, "%s job %s", name, jobName)
		if name == "run-acceptance-tests.yml" && jobName == "prerequisites" {
			require.Equal(t, "dokploy-acceptance", job["environment"], "%s job %s", name, jobName)
		} else {
			require.NotContains(t, workflowValueText(job), "secrets.DOKPLOY_", "%s job %s", name, jobName)
		}
	}
}

func workflowValueText(value any) string {
	switch value := value.(type) {
	case map[string]any:
		var parts []string
		for key, nested := range value {
			parts = append(parts, key, workflowValueText(nested))
		}
		return strings.Join(parts, " ")
	case []any:
		var parts []string
		for _, nested := range value {
			parts = append(parts, workflowValueText(nested))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprint(value)
	}
}

func assertCheckoutJobsHaveReadContents(t *testing.T, name string, workflow map[string]any, requiredJobs ...string) {
	t.Helper()
	jobs, ok := workflow["jobs"].(map[string]any)
	require.True(t, ok, name)
	for _, jobName := range requiredJobs {
		rawJob, exists := jobs[jobName]
		require.True(t, exists, "%s job %s", name, jobName)
		job, ok := rawJob.(map[string]any)
		require.True(t, ok, "%s job %s", name, jobName)
		steps, _ := job["steps"].([]any)
		hasCheckout := false
		for _, rawStep := range steps {
			step, _ := rawStep.(map[string]any)
			uses, _ := step["uses"].(string)
			if strings.HasPrefix(uses, "actions/checkout@") {
				hasCheckout = true
				permissions, ok := job["permissions"].(map[string]any)
				require.True(t, ok, "%s job %s checkout permissions", name, jobName)
				require.Equal(t, "read", permissions["contents"], "%s job %s checkout permissions", name, jobName)
				break
			}
		}
		require.True(t, hasCheckout, "%s job %s must contain checkout", name, jobName)
	}
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
