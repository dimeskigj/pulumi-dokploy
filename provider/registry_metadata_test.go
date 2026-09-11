package dokploy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const releaseGoVersion = "1.26.6"

const pulumiScriptsRef = "9aeffd86afa793846dc4f001a6f374b3f135bfef"

const minimumGolangciLintVersion = "2.9.0"

func readWorkflow(t *testing.T, name string) (map[string]any, string) {
	t.Helper()
	content, err := os.ReadFile("../.github/workflows/" + name)
	require.NoError(t, err)
	workflow := map[string]any{}
	require.NoError(t, yaml.Unmarshal(content, &workflow))
	return workflow, string(content)
}

func TestLintToolSupportsReleaseGoVersion(t *testing.T) {
	content, err := os.ReadFile("../.mise.toml")
	require.NoError(t, err)

	match := regexp.MustCompile(`(?m)^golangci-lint = "([^"]+)"$`).FindStringSubmatch(string(content))
	require.Len(t, match, 2)
	require.Equal(t, minimumGolangciLintVersion, match[1])
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
	"release.yml":              {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true},
	"run-acceptance-tests.yml": {"prerequisites": true, "build_sdks": true, "test": true, "lint": true},
	"release-smoke.yml":        {"validate-version": false, "provider": false, "node": false, "python": false, "dotnet": false, "java": false, "go": true},
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

func validateReleaseSmokeWorkflow(workflow map[string]any, text string) error {
	on, ok := workflow["on"].(map[string]any)
	if !ok || len(on) != 1 {
		return fmt.Errorf("release smoke workflow must only use workflow_dispatch")
	}
	dispatch, ok := on["workflow_dispatch"].(map[string]any)
	if !ok {
		return fmt.Errorf("release smoke workflow must define workflow_dispatch inputs")
	}
	inputs, ok := dispatch["inputs"].(map[string]any)
	if !ok || len(inputs) != 1 {
		return fmt.Errorf("release smoke workflow must define exactly one input")
	}
	version, ok := inputs["version"].(map[string]any)
	if !ok || version["required"] != true || version["type"] != "string" {
		return fmt.Errorf("release smoke version input must be a required string")
	}
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok || len(jobs) != 7 {
		return fmt.Errorf("release smoke workflow must have one validator and six consumer jobs")
	}
	validator, ok := jobs["validate-version"].(map[string]any)
	if !ok || validator["needs"] != nil {
		return fmt.Errorf("release smoke version validator must be independent")
	}
	validatorEnv, _ := validator["env"].(map[string]any)
	validatorRun := workflowValueText(validator)
	if validatorEnv["VERSION_INPUT"] != "${{ inputs.version }}" || !strings.Contains(validatorRun, "GITHUB_OUTPUT") {
		return fmt.Errorf("release smoke version validator must safely emit a validated output")
	}
	for _, jobName := range []string{"provider", "node", "python", "dotnet", "java", "go"} {
		job, ok := jobs[jobName].(map[string]any)
		if !ok {
			return fmt.Errorf("release smoke job %q is missing", jobName)
		}
		if job["needs"] != "validate-version" {
			return fmt.Errorf("release smoke job %q must depend on validate-version", jobName)
		}
		permissions, ok := job["permissions"].(map[string]any)
		if !ok || len(permissions) != 1 || permissions["contents"] != "read" {
			return fmt.Errorf("release smoke job %q must have read-only contents permission", jobName)
		}
		jobEnv, ok := job["env"].(map[string]any)
		if !ok || jobEnv["VERSION"] != "${{ needs.validate-version.outputs.version }}" {
			return fmt.Errorf("release smoke job %q must use the validated version output", jobName)
		}
		isolatedCache := false
		for _, step := range workflowJobRunSteps(workflow, jobName) {
			if step.env["PULUMI_HOME"] == "${{ runner.temp }}/pulumi-home-${{ github.job }}" && strings.Contains(workflowValueText(step.env), "${{ runner.temp }}") {
				isolatedCache = true
			}
		}
		if !isolatedCache {
			return fmt.Errorf("release smoke job %q must use fresh runner-temp caches", jobName)
		}
		for _, step := range workflowJobRunSteps(workflow, jobName) {
			if strings.Contains(step.run, "inputs.version") {
				return fmt.Errorf("release smoke job %q must not interpolate the raw input in shell", jobName)
			}
		}
		if jobName == "provider" && !strings.Contains(workflowValueText(job), "pulumi plugin install") {
			return fmt.Errorf("release smoke job %q must verify provider plugin acquisition", jobName)
		}
		jobText := workflowValueText(job)
		requiredCommands := []string{"pulumi package get-schema", "PULUMI_HOME/plugins", "test -n"}
		if jobName != "provider" {
			requiredCommands = append(requiredCommands, "pulumi preview --non-interactive")
			if strings.Contains(jobText, "pulumi plugin install") {
				return fmt.Errorf("release smoke consumer job %q must rely on SDK metadata for plugin acquisition", jobName)
			}
		}
		for _, required := range requiredCommands {
			if !strings.Contains(jobText, required) {
				return fmt.Errorf("release smoke job %q is missing %q", jobName, required)
			}
		}
	}
	for jobName, markers := range map[string][]string{
		"node":   {"require(\"@pulumi/pulumi\")", "require(\"@dimeskigj/pulumi-dokploy\")", "new dokploy.Provider"},
		"python": {"import pulumi", "import pulumi_dokploy", "pulumi_dokploy.Provider"},
		"dotnet": {"using Pulumi;", "using Dimeskigj.Pulumi.Dokploy;", "new Provider"},
		"java":   {"import com.pulumi.Pulumi;", "import net.dimeski.pulumi.dokploy.Provider;", "new Provider"},
		"go":     {"pulumi.Run", "dokploy.NewProvider"},
	} {
		job := jobs[jobName].(map[string]any)
		jobText := workflowValueText(job)
		for _, marker := range markers {
			if !strings.Contains(jobText, marker) {
				return fmt.Errorf("release smoke job %q is missing SDK program marker %q", jobName, marker)
			}
		}
	}
	for jobName, contract := range map[string]struct {
		command string
		caches  []string
	}{
		"node":   {"npm install", []string{"npm_config_cache"}},
		"python": {"pip install", []string{"PIP_CACHE_DIR"}},
		"dotnet": {"dotnet add package", []string{"NUGET_PACKAGES"}},
		"java":   {"mvn -B -ntp", []string{"MAVEN_OPTS"}},
		"go":     {"go get", []string{"GOPATH", "GOMODCACHE"}},
	} {
		found := false
		for _, step := range workflowJobRunSteps(workflow, jobName) {
			if !strings.Contains(step.run, contract.command) {
				continue
			}
			found = true
			for _, cache := range contract.caches {
				if !strings.Contains(fmt.Sprint(step.env[cache]), "${{ runner.temp }}") {
					return fmt.Errorf("release smoke job %q must set %s before dependency resolution", jobName, cache)
				}
			}
		}
		if !found {
			return fmt.Errorf("release smoke job %q is missing dependency-resolution command %q", jobName, contract.command)
		}
	}
	javaText := workflowValueText(jobs["java"])
	for _, required := range []string{"<artifactId>dokploy</artifactId>", "<version>$VERSION</version>", "exec-maven-plugin", "<mainClass>smoke.Main</mainClass>", "runtime: java", "mvn -B -ntp package"} {
		if !strings.Contains(javaText, required) {
			return fmt.Errorf("release smoke Java consumer is missing runnable Maven metadata %q", required)
		}
	}
	if strings.Contains(text, "pulumi-v3.159.0-linux-x64.tar.gz") {
		return fmt.Errorf("release smoke workflow contains stale Pulumi CLI pin 3.159.0")
	}
	for _, required := range []string{
		"=~ ^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)",
		"pulumi-v3.259.0-linux-x64.tar.gz",
		"checksums.txt",
		"sha256sum",
		"pulumi package get-schema",
		"cd \"$RUNNER_TEMP/provider-download\"",
		"@dimeskigj/pulumi-dokploy@$VERSION",
		"pulumi_dokploy==$VERSION",
		"Dimeskigj.Pulumi.Dokploy --version \"$VERSION\"",
		"<groupId>net.dimeski.pulumi</groupId><artifactId>dokploy</artifactId><version>$VERSION</version>",
		"github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy@v$VERSION",
	} {
		if !strings.Contains(text, required) {
			return fmt.Errorf("release smoke workflow is missing %q", required)
		}
	}
	if strings.Count(text, "pulumi-v3.259.0-linux-x64.tar.gz") != 6 {
		return fmt.Errorf("release smoke workflow must pin Pulumi CLI 3.259.0 in every job")
	}
	if strings.Contains(text, "options:\n    main:") || strings.Contains(text, "options:\n    build:") {
		return fmt.Errorf("release smoke Java runtime must rely on Maven runtime detection")
	}
	for _, forbidden := range []string{"secrets.", "publish", "npm publish", "twine upload", "dotnet nuget push", "maven-publish"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			return fmt.Errorf("release smoke workflow contains forbidden %q", forbidden)
		}
	}
	return nil
}

func TestReleaseSmokeWorkflowContracts(t *testing.T) {
	workflow, text := readWorkflow(t, "release-smoke.yml")
	require.NoError(t, validateReleaseSmokeWorkflow(workflow, text))
}

func TestReleaseSmokeWorkflowRejectsPolicyDrift(t *testing.T) {
	workflow, text := readWorkflow(t, "release-smoke.yml")
	jobs := workflow["jobs"].(map[string]any)
	delete(jobs, "go")
	require.ErrorContains(t, validateReleaseSmokeWorkflow(workflow, text), "one validator and six consumer jobs")
}

func TestReleaseSmokeWorkflowRejectsStalePulumiPin(t *testing.T) {
	workflow, text := readWorkflow(t, "release-smoke.yml")
	stale := strings.ReplaceAll(text, "pulumi-v3.259.0-linux-x64.tar.gz", "pulumi-v3.159.0-linux-x64.tar.gz")
	require.ErrorContains(t, validateReleaseSmokeWorkflow(workflow, stale), "stale Pulumi CLI pin 3.159.0")
}

func TestJavaExampleFixtureHasRunnableMavenMetadata(t *testing.T) {
	pom, err := os.ReadFile("../examples/java/pom.xml")
	require.NoError(t, err)
	for _, marker := range []string{"exec-maven-plugin", "<mainClass>${mainClass}</mainClass>", "<artifactId>pulumi</artifactId>"} {
		require.Contains(t, string(pom), marker)
	}
	pulumiYAML, err := os.ReadFile("../examples/java/Pulumi.yaml")
	require.NoError(t, err)
	require.Contains(t, string(pulumiYAML), "runtime: java")
	program, err := os.ReadFile("../examples/java/src/main/java/generated_program/App.java")
	require.NoError(t, err)
	require.Contains(t, string(program), "Pulumi.run")
}

func validateSchemaCompatibilityStep(workflow map[string]any) error {
	jobs, ok := workflow["jobs"].(map[string]any)
	if !ok {
		return fmt.Errorf("build workflow has no jobs")
	}
	prerequisites, ok := jobs["prerequisites"].(map[string]any)
	if !ok {
		return fmt.Errorf("build workflow has no prerequisites job")
	}
	steps, ok := prerequisites["steps"].([]any)
	if !ok {
		return fmt.Errorf("build prerequisites has no steps")
	}
	buildSchemaIndex := -1
	checkCount := 0
	for i, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		if step["name"] == "Build Schema" {
			buildSchemaIndex = i
		}
		if step["name"] == "Check Schema is Valid" {
			checkCount++
		}
	}
	if buildSchemaIndex < 0 {
		return fmt.Errorf("build workflow has no schema generation step")
	}
	if checkCount != 1 {
		return fmt.Errorf("build workflow must have exactly one schema compatibility check, found %d", checkCount)
	}
	comparisonCount := 0
	for i, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		if step["name"] != "Check Schema is Valid" {
			continue
		}
		if i <= buildSchemaIndex {
			return fmt.Errorf("schema compatibility check must follow schema generation")
		}
		if step["if"] != "github.event_name == 'pull_request'" {
			return fmt.Errorf("schema compatibility check must be pull-request-only")
		}
		run, _ := step["run"].(string)
		comparisonCount += strings.Count(run, "schema-tools compare")
		for _, required := range []string{
			"schema-tools compare",
			"cat \"$RUNNER_TEMP/schema-check-report.md\"",
			"Looking good! No breaking changes found.",
			"exit 1",
		} {
			if !strings.Contains(run, required) {
				return fmt.Errorf("schema compatibility check is missing %q", required)
			}
		}
		if env, ok := step["env"].(map[string]any); !ok || env["GITHUB_TOKEN"] != "${{ secrets.GITHUB_TOKEN }}" {
			return fmt.Errorf("schema compatibility check must receive GITHUB_TOKEN")
		}
		if comparisonCount != 1 {
			return fmt.Errorf("build workflow must have exactly one schema-tools comparison, found %d", comparisonCount)
		}
		return nil
	}
	return fmt.Errorf("build workflow has no schema compatibility check")
}

func runSchemaCompatibilityCheck(t *testing.T, workflow map[string]any, output string, status string) (int, string) {
	t.Helper()
	j := workflow["jobs"].(map[string]any)
	steps := j["prerequisites"].(map[string]any)["steps"].([]any)
	var run string
	for _, rawStep := range steps {
		step := rawStep.(map[string]any)
		if step["name"] == "Check Schema is Valid" {
			run = step["run"].(string)
		}
	}
	require.NotEmpty(t, run)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	require.NoError(t, os.Mkdir(bin, 0o755))
	schemaTools := filepath.Join(bin, "schema-tools")
	require.NoError(t, os.WriteFile(schemaTools, []byte("#!/bin/sh\nprintf '%s\\n' \"$SCHEMA_TOOLS_OUTPUT\"\nexit \"$SCHEMA_TOOLS_STATUS\"\n"), 0o600))
	require.NoError(t, os.Chmod(schemaTools, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "provider.json"), []byte("{}"), 0o600))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RUNNER_TEMP", tmp)
	t.Setenv("PROVIDER", "dokploy")
	t.Setenv("DEFAULT_BRANCH", "main")
	t.Setenv("SCHEMA_TOOLS_OUTPUT", output)
	t.Setenv("SCHEMA_TOOLS_STATUS", status)
	command := exec.Command("bash", "-euo", "pipefail", "-c", run)
	err := command.Run()
	result := 0
	if err != nil {
		result = command.ProcessState.ExitCode()
	}
	report, readErr := os.ReadFile(filepath.Join(tmp, "schema-check-report.md"))
	require.NoError(t, readErr)
	return result, string(report)
}

func TestSchemaCompatibilityCheckExecutesItsResult(t *testing.T) {
	workflow, _ := readWorkflow(t, "build.yml")
	t.Run("success marker passes", func(t *testing.T) {
		status, report := runSchemaCompatibilityCheck(t, workflow, "Looking good! No breaking changes found.", "0")
		require.Equal(t, 0, status)
		require.Contains(t, report, "Looking good! No breaking changes found.")
	})
	t.Run("incompatible result fails and retains report", func(t *testing.T) {
		status, report := runSchemaCompatibilityCheck(t, workflow, "Breaking change detected", "1")
		require.NotEqual(t, 0, status)
		require.Contains(t, report, "Breaking change detected")
	})
}

func TestSchemaCompatibilityValidationRejectsDuplicateChecks(t *testing.T) {
	workflow, _ := readWorkflow(t, "build.yml")
	jobs := workflow["jobs"].(map[string]any)
	steps := jobs["prerequisites"].(map[string]any)["steps"].([]any)
	for _, rawStep := range steps {
		step := rawStep.(map[string]any)
		if step["name"] == "Check Schema is Valid" {
			jobs["prerequisites"].(map[string]any)["steps"] = append(steps, step)
			break
		}
	}
	require.ErrorContains(t, validateSchemaCompatibilityStep(workflow), "exactly one schema compatibility check")
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
	require.Equal(t, "Dimeskigj.Pulumi.Dokploy", languageSetting(spec, "csharp", "packageName"))
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
	require.NoError(t, validateSchemaCompatibilityStep(build))
	require.Contains(t, buildText, "schema-tools compare")
	require.Contains(t, buildText, "github.event_name == 'pull_request'")
	require.Contains(t, buildText, "Looking good! No breaking changes found.")
	require.Contains(t, buildText, "exit 1", "schema incompatibility must fail the PR workflow")
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
	for _, workflowText := range []string{releaseText, prereleaseText} {
		for _, unused := range []string{"AZURE_SIGNING_", "SKIP_SIGNING", "JAVA_SIGNING_KEY_ID"} {
			require.NotContains(t, workflowText, unused)
		}
	}
	require.NotContains(t, releaseText, "dispatch_docs_build")
	require.NotContains(t, releaseText, "pulumictl create docs-build")
	require.NotContains(t, releaseText, "schema-tools compare")
	require.NotContains(t, prereleaseText, "schema-tools compare")
	sdkTestCommand := "cd examples && $GO_TEST_EXEC -tags=${{ matrix.language }} -v -count=1 -coverprofile=coverage.txt ."
	for name, workflow := range map[string]map[string]any{
		"build.yml":      build,
		"release.yml":    release,
		"prerelease.yml": prerelease,
	} {
		require.True(t, hasExactRunCommand(workflowJobRunSteps(workflow, "test"), sdkTestCommand), "%s SDK test must run from examples", name)
	}
	for name, workflow := range map[string]map[string]any{"release.yml": release, "prerelease.yml": prerelease} {
		jobs := workflow["jobs"].(map[string]any)
		require.NoError(t, validateReleaseWorkflowContracts(workflow, name))
		publish := jobs["publish"].(map[string]any)
		require.Equal(t, "write", publish["permissions"].(map[string]any)["contents"], name)
		var goreleaserEnv map[string]any
		var goreleaserVersion any
		for _, rawStep := range publish["steps"].([]any) {
			step := rawStep.(map[string]any)
			if uses, _ := step["uses"].(string); strings.Contains(uses, "goreleaser/goreleaser-action@") {
				goreleaserEnv = step["env"].(map[string]any)
				goreleaserVersion = step["with"].(map[string]any)["version"]
			}
		}
		require.NotNil(t, goreleaserEnv, name)
		require.Equal(t, "${{ secrets.GITHUB_TOKEN }}", goreleaserEnv["GITHUB_TOKEN"], name)
		require.Equal(t, "~> v2", goreleaserVersion, name)
		require.Equal(t, "publish", jobs["publish_sdk"].(map[string]any)["needs"], name)
		require.Equal(t, []any{"publish_sdk", "publish_java_sdk"}, jobs["publish_go_sdk"].(map[string]any)["needs"], name)
		scriptsCheckout := findWorkflowStepWithRepository(jobs["publish_sdk"].(map[string]any), "actions/checkout@", "pulumi/scripts")
		require.NotNil(t, scriptsCheckout, name)
		scriptsWith := scriptsCheckout["with"].(map[string]any)
		require.Equal(t, "pulumi/scripts", scriptsWith["repository"], name)
		require.Equal(t, "ci-scripts", scriptsWith["path"], name)
		require.Equal(t, pulumiScriptsRef, scriptsWith["ref"], name)

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
	require.Contains(t, acceptanceText, "DOKPLOY_ENDPOINT: ${{ secrets.DOKPLOY_ENDPOINT }}")
	require.Contains(t, acceptanceText, "DOKPLOY_API_KEY: ${{ secrets.DOKPLOY_API_KEY }}")
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
	contributing, err := os.ReadFile("../CONTRIBUTING.md")
	require.NoError(t, err)
	contributingText := string(contributing)
	require.Contains(t, contributingText, "Settings > Secrets and variables > Actions")
	for _, secret := range []string{
		"NUGET_USER", "PYPI_API_TOKEN",
		"JAVA_SIGNING_KEY", "JAVA_SIGNING_PASSWORD",
		"OSSRH_USERNAME", "OSSRH_PASSWORD", "CODECOV_TOKEN",
	} {
		require.Contains(t, contributingText, "`"+secret+"`")
	}
	require.NotContains(t, contributingText, "NUGET_PUBLISH_KEY")
	require.NotContains(t, contributingText, "NPM_TOKEN")
	require.Contains(t, contributingText, "`GITHUB_TOKEN` is created automatically")
	require.Contains(t, contributingText, "`CODECOV_TOKEN` is optional")
	require.Contains(t, contributingText, "separate protected")
	require.Contains(t, contributingText, "`dokploy-acceptance` environment")
	require.NotContains(t, contributingText, "AZURE_SIGNING_")
	require.NotContains(t, contributingText, "JAVA_SIGNING_KEY_ID")
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

func TestOwnedWorkflow(t *testing.T) {
	workflow, _ := readWorkflow(t, "run-acceptance-tests.yml")

	jobs := workflow["jobs"].(map[string]any)
	prerequisites := jobs["prerequisites"].(map[string]any)
	steps := prerequisites["steps"].([]any)
	markerPath := "${{ runner.temp }}/dokploy-acceptance-stop-${{ github.run_id }}-${{ github.run_attempt }}"
	prepareIndex := findStepIndex(steps, "Prepare acceptance stop marker")
	require.GreaterOrEqual(t, prepareIndex, 0)
	prepare := steps[prepareIndex].(map[string]any)
	require.Equal(t, "prepare_stop_marker", prepare["id"])
	require.Equal(t, map[string]any{"DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath}, prepare["env"])
	require.Contains(t, prepare["run"], "rm -f")
	require.Contains(t, prepare["run"], "test -w")
	require.Contains(t, prepare["run"], "test ! -e")
	preflight := map[string]any{}
	for _, rawStep := range steps {
		step := rawStep.(map[string]any)
		if step["name"] == "Require Dokploy acceptance credentials" {
			preflight = step
		}
	}
	require.NotEmpty(t, preflight)
	require.Equal(t, "preflight", preflight["id"])
	require.Equal(t, "test -n \"$DOKPLOY_ENDPOINT\" && test -n \"$DOKPLOY_API_KEY\"", preflight["run"])
	require.Equal(t, map[string]any{
		"DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}",
		"DOKPLOY_API_KEY":  "${{ secrets.DOKPLOY_API_KEY }}",
	}, preflight["env"])
	type liveStep struct {
		name string
		run  string
		env  map[string]any
	}
	var liveSteps []liveStep
	for _, rawStep := range steps {
		step := rawStep.(map[string]any)
		run, _ := step["run"].(string)
		if strings.Contains(run, "go test ./provider") || strings.Contains(run, "go test ./tests") {
			liveSteps = append(liveSteps, liveStep{name: step["name"].(string), run: run, env: step["env"].(map[string]any)})
		}
	}
	require.Len(t, liveSteps, 5)
	expected := []struct {
		name string
		run  string
		env  map[string]any
	}{
		{"Test live Tier 1 control plane", "mise exec -- go test ./provider -run TestLiveTier1ControlPlane -parallel=1 -count=1 -v", map[string]any{
			"DOKPLOY_ACCEPTANCE": "1", "DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath, "DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}", "DOKPLOY_API_KEY": "${{ secrets.DOKPLOY_API_KEY }}", "DOKPLOY_REGISTRY_URL": "${{ secrets.DOKPLOY_REGISTRY_URL }}", "DOKPLOY_REGISTRY_USERNAME": "${{ secrets.DOKPLOY_REGISTRY_USERNAME }}", "DOKPLOY_REGISTRY_PASSWORD": "${{ secrets.DOKPLOY_REGISTRY_PASSWORD }}", "DOKPLOY_REGISTRY_IMAGE_PREFIX": "${{ secrets.DOKPLOY_REGISTRY_IMAGE_PREFIX }}",
		}},
		{"Test live Tier 2 workloads", "mise exec -- go test ./provider -run TestLiveTier2Workloads -parallel=1 -count=1 -v", map[string]any{
			"DOKPLOY_ACCEPTANCE": "1", "DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath, "DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}", "DOKPLOY_API_KEY": "${{ secrets.DOKPLOY_API_KEY }}", "DOKPLOY_REGISTRY_URL": "${{ secrets.DOKPLOY_REGISTRY_URL }}", "DOKPLOY_REGISTRY_USERNAME": "${{ secrets.DOKPLOY_REGISTRY_USERNAME }}", "DOKPLOY_REGISTRY_PASSWORD": "${{ secrets.DOKPLOY_REGISTRY_PASSWORD }}", "DOKPLOY_REGISTRY_IMAGE_PREFIX": "${{ secrets.DOKPLOY_REGISTRY_IMAGE_PREFIX }}",
		}},
		{"Test live Tier 3 databases", "mise exec -- go test ./provider -run TestLiveTier3Databases -parallel=1 -count=1 -v", map[string]any{
			"DOKPLOY_ACCEPTANCE": "1", "DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath, "DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}", "DOKPLOY_API_KEY": "${{ secrets.DOKPLOY_API_KEY }}",
		}},
		{"Test live Tier 4 backups", "mise exec -- go test ./provider -run TestLiveTier4Backups -parallel=1 -count=1 -v", map[string]any{
			"DOKPLOY_ACCEPTANCE": "1", "DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath, "DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}", "DOKPLOY_API_KEY": "${{ secrets.DOKPLOY_API_KEY }}",
		}},
		{"Test Pulumi lifecycle smoke", "mise exec -- go test ./tests -run TestAccLifecycleSmoke -parallel=1 -count=1 -v", map[string]any{
			"DOKPLOY_ACCEPTANCE": "1", "DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath, "DOKPLOY_ENDPOINT": "${{ secrets.DOKPLOY_ENDPOINT }}", "DOKPLOY_API_KEY": "${{ secrets.DOKPLOY_API_KEY }}",
		}},
	}
	expectedIDs := []string{"tier1", "tier2", "tier3", "tier4", "smoke"}
	for i, want := range expected {
		step := steps[findStepIndex(steps, liveSteps[i].name)].(map[string]any)
		require.Equal(t, want.name, liveSteps[i].name)
		require.Equal(t, want.run, liveSteps[i].run)
		require.Equal(t, want.env, liveSteps[i].env)
		require.Equal(t, expectedIDs[i], step["id"])
		if i == 0 {
			require.NotContains(t, step, "if")
		} else {
			require.Equal(t, "${{ steps."+expectedIDs[i-1]+"_gate.outputs.marker_absent == 'true' }}", step["if"])
			require.NotContains(t, step["if"], "!= 'true'")
		}
		require.Equal(t, true, step["continue-on-error"])
		require.Contains(t, liveSteps[i].run, "-parallel=1 -count=1 -v")
	}
	for i, name := range []string{"Tier 1", "Tier 2", "Tier 3", "Tier 4"} {
		gate := steps[findStepIndex(steps, "Check "+name+" stop marker")].(map[string]any)
		require.Equal(t, expectedIDs[i]+"_gate", gate["id"])
		require.Equal(t, "${{ always() }}", gate["if"])
		require.NotContains(t, gate, "continue-on-error")
		require.Equal(t, map[string]any{"DOKPLOY_ACCEPTANCE_STOP_FILE": markerPath}, gate["env"])
		require.Contains(t, gate["run"], "marker_absent=true")
	}
	orderedNames := []string{"Checkout Repo", "Set Provider Version", "Prepare acceptance stop marker", "Require Dokploy acceptance credentials", "Setup Tools", "Verify Pulumi CLI", "Build codegen binaries", "Build Schema", "Build Provider", "Expose local provider", "Test Provider Library", "Test live Tier 1 control plane", "Check Tier 1 stop marker", "Test live Tier 2 workloads", "Check Tier 2 stop marker", "Test live Tier 3 databases", "Check Tier 3 stop marker", "Test live Tier 4 backups", "Check Tier 4 stop marker", "Test Pulumi lifecycle smoke"}
	previous := -1
	for _, name := range orderedNames {
		index := findStepIndex(steps, name)
		require.Greater(t, index, previous, "workflow step order for %s", name)
		if !strings.HasPrefix(name, "Test live ") && name != "Test Pulumi lifecycle smoke" && !strings.HasPrefix(name, "Check Tier ") {
			require.NotContains(t, steps[index].(map[string]any), "if", name)
		}
		if strings.HasPrefix(name, "Test live ") || name == "Test Pulumi lifecycle smoke" {
			require.Equal(t, true, steps[index].(map[string]any)["continue-on-error"], name)
		} else {
			require.NotContains(t, steps[index].(map[string]any), "continue-on-error", name)
		}
		previous = index
	}
	pulumiCLI := steps[findStepIndex(steps, "Verify Pulumi CLI")].(map[string]any)
	require.Equal(t, "mise exec -- pulumi version", pulumiCLI["run"])
	localProvider := steps[findStepIndex(steps, "Expose local provider")].(map[string]any)
	require.Equal(t, `echo "${{ github.workspace }}/bin" >> "$GITHUB_PATH"`, localProvider["run"])
	require.NotContains(t, localProvider["run"], "ls")
	require.NotContains(t, localProvider["run"], "find")
	for i, name := range []string{"Tier 1", "Tier 2", "Tier 3", "Tier 4"} {
		gateIndex := findStepIndex(steps, "Check "+name+" stop marker")
		tierIndex := findStepIndex(steps, expected[i+1].name)
		require.Equal(t, tierIndex-1, gateIndex)
		gateID := expectedIDs[i] + "_gate"
		tier := steps[tierIndex].(map[string]any)
		require.Equal(t, "${{ steps."+gateID+".outputs.marker_absent == 'true' }}", tier["if"])
		require.NotContains(t, tier["if"], "!= 'true'")
	}
	reportIndex := findStepIndex(steps, "Report acceptance failures")
	require.Equal(t, findStepIndex(steps, "Test Pulumi lifecycle smoke")+1, reportIndex)
	report := steps[reportIndex].(map[string]any)
	require.Equal(t, "${{ always() }}", report["if"])
	reportRun := report["run"].(string)
	for _, outcome := range []string{"steps.tier1.outcome", "steps.tier2.outcome", "steps.tier3.outcome", "steps.tier4.outcome", "steps.smoke.outcome"} {
		require.Contains(t, reportRun, "${{ "+outcome+" }}")
	}
	require.Contains(t, reportRun, `[ "$outcome" = "failure" ] || [ "$outcome" = "cancelled" ]`)
	require.Contains(t, reportRun, `exit "$failed"`)
	for jobName, rawJob := range jobs {
		job := rawJob.(map[string]any)
		jobSteps, _ := job["steps"].([]any)
		for _, rawStep := range jobSteps {
			step := rawStep.(map[string]any)
			name, _ := step["name"].(string)
			run, _ := step["run"].(string)
			require.NotContains(t, run, ".env", "%s/%s must not load .env", jobName, name)
			if jobName != "prerequisites" || (name != "Require Dokploy acceptance credentials" && !strings.HasPrefix(name, "Test live ") && name != "Test Pulumi lifecycle smoke") {
				env, _ := step["env"].(map[string]any)
				for key := range env {
					if key != "DOKPLOY_ACCEPTANCE_STOP_FILE" {
						require.NotContains(t, key, "DOKPLOY_")
					}
				}
			}
		}
	}
}

func findStepIndex(steps []any, name string) int {
	for i, rawStep := range steps {
		if rawStep.(map[string]any)["name"] == name {
			return i
		}
	}
	return -1
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
		switch jobName {
		case "publish":
			expected = map[string]any{"contents": "write", "id-token": "write", "attestations": "write"}
		case "publish_go_sdk":
			expected = map[string]any{"contents": "write"}
		case "publish_sdk":
			expected = map[string]any{"contents": "read", "id-token": "write"}
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

func findWorkflowStepWithRepository(job map[string]any, actionPrefix string, repository string) map[string]any {
	steps, _ := job["steps"].([]any)
	for _, rawStep := range steps {
		step, _ := rawStep.(map[string]any)
		uses, _ := step["uses"].(string)
		with, _ := step["with"].(map[string]any)
		if strings.HasPrefix(uses, actionPrefix) && with["repository"] == repository {
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

func TestExampleTestWorkflowsRunFromExamplesDirectory(t *testing.T) {
	for _, test := range []struct {
		name       string
		requireSet bool
	}{
		{name: "build.yml", requireSet: true},
		{name: "release.yml", requireSet: true},
		{name: "prerelease.yml", requireSet: true},
		{name: "run-acceptance-tests.yml"},
	} {
		workflow, _ := readWorkflow(t, test.name)
		steps := workflowRunSteps(workflow)
		var runTests []string
		for _, step := range steps {
			if strings.Contains(step.run, "GO_TEST_EXEC") {
				runTests = append(runTests, step.run)
			}
		}
		require.Len(t, runTests, 1, test.name)
		if test.requireSet {
			require.Contains(t, runTests[0], "set -euo pipefail\n", test.name)
			require.Contains(t, runTests[0], "\ncd examples &&", test.name)
		} else {
			require.True(t, strings.HasPrefix(runTests[0], "cd examples &&"), test.name)
			require.Contains(t, runTests[0], "-parallel 4", test.name)
			require.NotContains(t, runTests[0], "-parallel=1", test.name)
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

func TestGoReleaserConfigsPublishChecksumsSbomsAndNeutralWindowsBuild(t *testing.T) {
	for _, name := range []string{".goreleaser.yml", ".goreleaser.prerelease.yml"} {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile("../" + name)
			require.NoError(t, err)
			text := string(content)
			require.NotContains(t, text, "sign-windows")
			require.NotContains(t, text, "sign-goreleaser")

			config := map[string]any{}
			require.NoError(t, yaml.Unmarshal(content, &config))
			require.Equal(t, 2, config["version"])
			checksum, ok := config["checksum"].(map[string]any)
			require.True(t, ok, "checksum must be a mapping")
			require.Equal(t, "checksums.txt", checksum["name_template"])
			require.Equal(t, "sha256", checksum["algorithm"])
			archives, ok := config["archives"].([]any)
			require.True(t, ok, "archives must be a list")
			if !ok {
				return
			}
			require.Len(t, archives, 1)
			archive, ok := archives[0].(map[string]any)
			require.True(t, ok, "archive must be a mapping")
			if !ok {
				return
			}
			require.Equal(t, "{{ .Binary }}-{{ .Tag }}-{{ .Os }}-{{ .Arch }}", archive["name_template"])
			sboms, ok := config["sboms"].([]any)
			require.True(t, ok, "sboms must be a list")
			if !ok {
				return
			}
			require.Len(t, sboms, 1)
			sbom, ok := sboms[0].(map[string]any)
			require.True(t, ok, "sbom must be a mapping")
			if !ok {
				return
			}
			require.Equal(t, "archive", sbom["artifacts"])

			builds, ok := config["builds"].([]any)
			require.True(t, ok, "builds must be a list")
			if !ok {
				return
			}
			var windowsBuild map[string]any
			for _, rawBuild := range builds {
				build, ok := rawBuild.(map[string]any)
				require.True(t, ok, "build must be a mapping")
				if !ok {
					return
				}
				goos, ok := build["goos"].([]any)
				require.True(t, ok, "build goos must be a list")
				if !ok {
					return
				}
				if len(goos) == 1 && goos[0] == "windows" {
					windowsBuild = build
				}
			}
			require.NotNil(t, windowsBuild)
			require.Equal(t, "build-provider-windows", windowsBuild["id"])
			require.NotContains(t, windowsBuild, "hooks")
		})
	}
}

func TestReleasePublishJobsProvisionSyftBeforeGoReleaser(t *testing.T) {
	toolsContent, err := os.ReadFile("../.mise.toml")
	require.NoError(t, err)
	match := regexp.MustCompile(`(?m)^syft = "([^"]+)"$`).FindStringSubmatch(string(toolsContent))
	require.Len(t, match, 2)
	require.Equal(t, "1.51.1", match[1])

	setupContent, err := os.ReadFile("../.github/actions/setup-tools/action.yml")
	require.NoError(t, err)
	require.Contains(t, string(setupContent), "jdx/mise-action@")
	require.NotContains(t, string(setupContent), "install: false")
	for _, name := range []string{"release.yml", "prerelease.yml"} {
		t.Run(name, func(t *testing.T) {
			workflow, _ := readWorkflow(t, name)
			publish := workflow["jobs"].(map[string]any)["publish"].(map[string]any)
			steps := publish["steps"].([]any)
			setupIndex := -1
			goreleaserIndex := -1
			goreleaserArgs := ""
			for i, rawStep := range steps {
				step := rawStep.(map[string]any)
				uses, _ := step["uses"].(string)
				if uses == "./.github/actions/setup-tools" {
					setupIndex = i
				}
				if strings.Contains(uses, "goreleaser/goreleaser-action@") {
					goreleaserIndex = i
					goreleaserArgs = step["with"].(map[string]any)["args"].(string)
				}
			}
			require.GreaterOrEqual(t, setupIndex, 0)
			require.Greater(t, goreleaserIndex, setupIndex)
			if name == "prerelease.yml" {
				require.NotContains(t, goreleaserArgs, "--skip=validate")
			}
		})
	}
}

func TestSetupToolsPersistsMiseConfigTrust(t *testing.T) {
	setupContent, err := os.ReadFile("../.github/actions/setup-tools/action.yml")
	require.NoError(t, err)
	setup := map[string]any{}
	require.NoError(t, yaml.Unmarshal(setupContent, &setup))
	steps := setup["runs"].(map[string]any)["steps"].([]any)
	miseIndex := -1
	trustIndex := -1
	for i, rawStep := range steps {
		step := rawStep.(map[string]any)
		if uses, _ := step["uses"].(string); strings.Contains(uses, "jdx/mise-action@") {
			miseIndex = i
		}
		if run, _ := step["run"].(string); run == `mise trust "$GITHUB_WORKSPACE/.mise.toml"` {
			trustIndex = i
			require.Equal(t, "bash", step["shell"])
		}
	}
	require.GreaterOrEqual(t, miseIndex, 0)
	require.Equal(t, miseIndex+1, trustIndex)
}

func TestReleasePublishJobsAttestAllProviderArtifacts(t *testing.T) {
	shaPattern := regexp.MustCompile(`^actions/attest-build-provenance@[0-9a-f]{40}$`)
	for _, name := range []string{"release.yml", "prerelease.yml"} {
		t.Run(name, func(t *testing.T) {
			workflow, _ := readWorkflow(t, name)
			publish := workflow["jobs"].(map[string]any)["publish"].(map[string]any)
			require.Equal(t, map[string]any{
				"contents":     "write",
				"id-token":     "write",
				"attestations": "write",
			}, publish["permissions"])

			steps := publish["steps"].([]any)
			goreleaserIndex := -1
			attestationIndex := -1
			for i, rawStep := range steps {
				step := rawStep.(map[string]any)
				uses, _ := step["uses"].(string)
				if strings.Contains(uses, "goreleaser/goreleaser-action@") {
					goreleaserIndex = i
				}
				if strings.HasPrefix(uses, "actions/attest-build-provenance@") {
					attestationIndex = i
					require.Regexp(t, shaPattern, uses)
					require.NotContains(t, workflowValueText(step), "secrets.")
					with := step["with"].(map[string]any)
					subjectPath := with["subject-path"].(string)
					for _, requiredPath := range []string{"dist/*.tar.gz", "dist/*.zip", "dist/checksums.txt", "dist/*.sbom.json"} {
						require.Contains(t, subjectPath, requiredPath)
					}
				}
			}
			require.GreaterOrEqual(t, goreleaserIndex, 0)
			require.Greater(t, attestationIndex, goreleaserIndex)
		})
	}
}

func TestReleaseSnapshotArtifactsMatchAttestationPatterns(t *testing.T) {
	if os.Getenv("RELEASE_SNAPSHOT_CHECK") != "1" {
		t.Skip("set RELEASE_SNAPSHOT_CHECK=1 after a local GoReleaser snapshot")
	}

	dist := "../dist"
	archives, err := filepath.Glob(filepath.Join(dist, "*.tar.gz"))
	require.NoError(t, err)
	zips, err := filepath.Glob(filepath.Join(dist, "*.zip"))
	require.NoError(t, err)
	sboms, err := filepath.Glob(filepath.Join(dist, "*.sbom.json"))
	require.NoError(t, err)
	checksums, err := filepath.Glob(filepath.Join(dist, "checksums.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, archives, "snapshot must produce tar.gz release archives")
	require.NotEmpty(t, sboms, "snapshot must produce SBOMs for release archives")
	require.Len(t, checksums, 1)

	checksumEntries := map[string]string{}
	for _, line := range strings.Split(string(mustReadFile(t, checksums[0])), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			checksumEntries[fields[1]] = fields[0]
		}
	}
	for _, path := range append(archives, sboms...) {
		name := filepath.Base(path)
		expected, ok := checksumEntries[name]
		require.True(t, ok, "checksum missing %s", name)
		hash := sha256.Sum256(mustReadFile(t, path))
		require.Equal(t, hex.EncodeToString(hash[:]), expected, name)
	}
	for _, path := range append(archives, zips...) {
		require.True(t, strings.HasPrefix(filepath.Base(path), "pulumi-resource-dokploy-"), path)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return content
}
