package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	dokploy "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type liveConfig struct {
	Endpoint        string
	APIKey          string
	NameSuffix      string
	SecretSentinels []string
}

type lifecycleRevisionValues struct {
	description string
	environment string
	color       string
}

func pulumiCLIAvailable(lookPath func(string) (string, error)) bool {
	_, err := lookPath("pulumi")
	return err == nil
}

func runLifecycleSmoke(t *testing.T, ctx context.Context, cfg liveConfig) {
	t.Helper()
	backend := t.TempDir()
	stackName := acceptanceStackName(cfg.NameSuffix)
	stack, err := auto.NewStackInlineSource(ctx, stackName, acceptanceProjectName(cfg.NameSuffix), lifecycleRevisionOne(cfg),
		auto.PulumiHome(filepath.Join(backend, "pulumi")),
		auto.EnvVars(acceptanceAutomationEnv(backend, uuid.NewString())),
	)
	if err != nil {
		t.Fatal(acceptanceStageError("stack-create", err))
	}
	// Register cleanup immediately, including for failures during configuration or
	// the first preview. Cleanup is deliberately bounded and uses no state edits.
	t.Cleanup(func() {
		destroyErr, exportErr, removeErr, listErr := cleanupLifecycleStack(2*time.Minute,
			func(ctx context.Context) (auto.DestroyResult, error) {
				return stack.Destroy(ctx)
			},
			func(ctx context.Context, _ auto.DestroyResult) error {
				deployment, err := stack.Export(ctx)
				if err != nil {
					return err
				}
				return validateDestroyedState(deployment)
			},
			func(ctx context.Context) error {
				return stack.Workspace().RemoveStack(ctx, stackName)
			},
			func(ctx context.Context) error {
				stacks, err := stack.Workspace().ListStacks(ctx)
				if err != nil {
					return err
				}
				return validateStackRemoved(stacks, stackName)
			})
		if destroyErr != nil {
			t.Error(acceptanceStageError("destroy", destroyErr))
		}
		if exportErr != nil {
			t.Error(acceptanceStageError("export", exportErr))
		}
		if removeErr != nil {
			t.Error(acceptanceStageError("remove-stack", removeErr))
		}
		if listErr != nil {
			t.Error(acceptanceStageError("list-stacks", listErr))
		}
	})
	if err := stack.SetConfig(ctx, "dokploy:endpoint", auto.ConfigValue{Value: cfg.Endpoint}); err != nil {
		t.Fatal(acceptanceStageError("configure-endpoint", err))
	}
	if err := stack.SetConfig(ctx, "dokploy:apiKey", auto.ConfigValue{Value: cfg.APIKey, Secret: true}); err != nil {
		t.Fatal(acceptanceStageError("configure-api-key", err))
	}
	revisionOnePreview, err := stack.Preview(ctx)
	if err != nil {
		t.Fatal(acceptanceStageError("preview-1", err))
	}
	assertPreviewAggregate(t, revisionOnePreview, "revision one")
	revisionOneUp, err := stack.Up(ctx)
	if err != nil {
		t.Fatal(acceptanceStageError("up-1", err))
	}
	assertUpdateAggregate(t, revisionOneUp, "revision one")
	revisionOne := assertLifecycleOutputs(t, revisionOneUp.Outputs, lifecycleRevisionOneValues(), cfg, nil)
	if _, err := stack.Refresh(ctx); err != nil {
		t.Fatal(acceptanceStageError("refresh-1", err))
	}
	assertRefreshedLifecycleOutputs(t, ctx, stack, "refresh-1", lifecycleRevisionOneValues(), cfg, &revisionOne)

	stack.Workspace().SetProgram(lifecycleRevisionTwo(cfg))
	revisionTwoPreview, err := stack.Preview(ctx)
	if err != nil {
		t.Fatal(acceptanceStageError("preview-2", err))
	}
	assertPreviewAggregate(t, revisionTwoPreview, "revision two")
	revisionTwoUp, err := stack.Up(ctx)
	if err != nil {
		t.Fatal(acceptanceStageError("up-2", err))
	}
	assertUpdateAggregate(t, revisionTwoUp, "revision two")
	assertLifecycleOutputs(t, revisionTwoUp.Outputs, lifecycleRevisionTwoValues(), cfg, &revisionOne)
	if _, err := stack.Refresh(ctx); err != nil {
		t.Fatal(acceptanceStageError("refresh-2", err))
	}
	assertRefreshedLifecycleOutputs(t, ctx, stack, "refresh-2", lifecycleRevisionTwoValues(), cfg, &revisionOne)
}

func acceptanceAutomationEnv(backend, passphrase string) map[string]string {
	return map[string]string{
		"PULUMI_BACKEND_URL":       "file://" + backend,
		"PULUMI_CONFIG_PASSPHRASE": passphrase,
	}
}

func lifecycleSmokeProgram(cfg liveConfig, description, environmentName, tagColor string) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		project, err := dokploy.NewProject(ctx, acceptanceEntityName("project", cfg.NameSuffix), &dokploy.ProjectArgs{
			Name:        pulumi.String(acceptanceEntityName("project", cfg.NameSuffix)),
			Description: pulumi.String(description),
		})
		if err != nil {
			return err
		}
		environment, err := dokploy.NewEnvironment(ctx, acceptanceEntityName("environment", cfg.NameSuffix), &dokploy.EnvironmentArgs{
			ProjectId:   project.ProjectId,
			Name:        pulumi.String(acceptanceEntityName(environmentName, cfg.NameSuffix)),
			Description: pulumi.String("lifecycle smoke environment"),
		})
		if err != nil {
			return err
		}
		tag, err := dokploy.NewTag(ctx, acceptanceEntityName("tag", cfg.NameSuffix), &dokploy.TagArgs{
			Name:  pulumi.String(acceptanceEntityName("tag", cfg.NameSuffix)),
			Color: pulumi.String(tagColor),
		})
		if err != nil {
			return err
		}
		projectTag, err := dokploy.NewProjectTag(ctx, acceptanceEntityName("project-tag", cfg.NameSuffix), &dokploy.ProjectTagArgs{
			ProjectId: project.ProjectId,
			TagId:     tag.TagId,
		})
		if err != nil {
			return err
		}

		// These reads exercise the provider's normal import/read path using IDs
		// produced by resources, rather than modifying Pulumi state directly.
		if _, err = dokploy.GetProject(ctx, acceptanceEntityName("read-project", cfg.NameSuffix), asID(project.ProjectId), nil); err != nil {
			return err
		}
		if _, err = dokploy.GetEnvironment(ctx, acceptanceEntityName("read-environment", cfg.NameSuffix), asID(environment.EnvironmentId), nil); err != nil {
			return err
		}
		if _, err = dokploy.GetTag(ctx, acceptanceEntityName("read-tag", cfg.NameSuffix), asID(tag.TagId), nil); err != nil {
			return err
		}

		ctx.Export("projectId", project.ProjectId)
		ctx.Export("projectDescription", project.Description)
		ctx.Export("environmentId", environment.EnvironmentId)
		ctx.Export("environmentName", environment.Name)
		ctx.Export("tagId", tag.TagId)
		ctx.Export("tagColor", tag.Color)
		ctx.Export("associationProjectId", projectTag.ProjectId)
		ctx.Export("associationTagId", projectTag.TagId)
		return nil
	}
}

func lifecycleRevisionOne(cfg liveConfig) pulumi.RunFunc {
	values := lifecycleRevisionOneValues()
	return lifecycleSmokeProgram(cfg, values.description, values.environment, values.color)
}

func lifecycleRevisionTwo(cfg liveConfig) pulumi.RunFunc {
	values := lifecycleRevisionTwoValues()
	return lifecycleSmokeProgram(cfg, values.description, values.environment, values.color)
}

func lifecycleRevisionOneValues() lifecycleRevisionValues {
	return lifecycleRevisionValues{description: "lifecycle smoke one", environment: "staging", color: "#2dd4bf"}
}

func lifecycleRevisionTwoValues() lifecycleRevisionValues {
	return lifecycleRevisionValues{description: "lifecycle smoke two", environment: "staging-updated", color: "#f97316"}
}

func acceptanceEntityName(kind, suffix string) string {
	return "pulumi-acceptance-" + kind + "-" + suffix
}

func acceptanceStackName(suffix string) string {
	return acceptanceEntityName("stack", suffix)
}

func acceptanceProjectName(suffix string) string {
	return acceptanceEntityName("project", suffix)
}

func acceptanceFailure(phase string, field ...string) string {
	if len(field) == 0 || field[0] == "" {
		return "Pulumi acceptance " + phase + " failed"
	}
	return "Pulumi acceptance " + phase + " failed for field " + field[0]
}

var acceptanceStages = map[string]struct{}{
	"workspace":          {},
	"plugin-discovery":   {},
	"stack-create":       {},
	"configure-endpoint": {},
	"configure-api-key":  {},
	"preview-1":          {},
	"up-1":               {},
	"refresh-1":          {},
	"preview-2":          {},
	"up-2":               {},
	"refresh-2":          {},
	"destroy":            {},
	"export":             {},
	"remove-stack":       {},
	"list-stacks":        {},
}

func acceptanceStageError(stage string, err error) error {
	if _, ok := acceptanceStages[stage]; !ok {
		stage = "unknown"
	}
	category := "unknown"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
	case errors.Is(err, context.Canceled):
		category = "canceled"
	case err != nil:
		category = "process"
	}
	return fmt.Errorf("Pulumi acceptance failed at stage %s: category=%s", stage, category)
}

func acceptanceRefreshOutputError(stage string, err error) error {
	return acceptanceStageError(stage, err)
}

func sanitizeAcceptanceDiagnostic(diagnostic string, cfg liveConfig) string {
	sentinels := append([]string{cfg.APIKey, cfg.Endpoint}, cfg.SecretSentinels...)
	for _, env := range os.Environ() {
		name, value, ok := strings.Cut(env, "=")
		if ok && value != "" && strings.HasPrefix(name, "DOKPLOY_") && name != "DOKPLOY_ACCEPTANCE" {
			sentinels = append(sentinels, value)
		}
	}
	sort.SliceStable(sentinels, func(i, j int) bool { return len(sentinels[i]) > len(sentinels[j]) })
	for _, sentinel := range sentinels {
		if sentinel != "" {
			diagnostic = strings.ReplaceAll(diagnostic, sentinel, "[REDACTED]")
		}
	}
	return diagnostic
}

func cleanupLifecycleStack(
	timeout time.Duration,
	destroy func(context.Context) (auto.DestroyResult, error),
	validateExport func(context.Context, auto.DestroyResult) error,
	remove func(context.Context) error,
	list func(context.Context) error,
) (error, error, error, error) {
	destroyCtx, cancelDestroy := context.WithTimeout(context.Background(), timeout)
	destroyResult, destroyErr := destroy(destroyCtx)
	cancelDestroy()

	exportCtx, cancelExport := context.WithTimeout(context.Background(), timeout)
	exportErr := validateExport(exportCtx, destroyResult)
	cancelExport()

	removeCtx, cancelRemove := context.WithTimeout(context.Background(), timeout)
	removeErr := remove(removeCtx)
	cancelRemove()

	listCtx, cancelList := context.WithTimeout(context.Background(), timeout)
	listErr := list(listCtx)
	cancelList()
	return destroyErr, exportErr, removeErr, listErr
}

func validateLifecycleAggregate(phase string, changes map[string]int) error {
	// ChangeSummary aggregates custom, provider, and default resources, so it
	// cannot prove custom-resource counts. The stable proxy is at least one
	// expected mutating operation; exact custom counts and identities come from
	// mock resource capture and live output assertions instead.
	expectedOperation := ""
	switch phase {
	case "revision one":
		expectedOperation = "create"
	case "revision two":
		expectedOperation = "update"
	default:
		return fmt.Errorf("unsupported lifecycle phase %q", phase)
	}
	operations := make([]string, 0, len(changes))
	for operation := range changes {
		operations = append(operations, operation)
	}
	sort.Strings(operations)
	for _, operation := range operations {
		count := changes[operation]
		if count < 0 {
			return fmt.Errorf("%s has negative %s count: %d", phase, operation, count)
		}
		switch operation {
		case "read", "same", "noop":
			// These operations do not claim a custom-resource mutation.
		case "create", "update":
			if operation != expectedOperation {
				return fmt.Errorf("%s contains unexpected %s operations: %d", phase, operation, count)
			}
			if count == 0 {
				return fmt.Errorf("%s expected at least one %s operation", phase, expectedOperation)
			}
		case "replace":
			return fmt.Errorf("%s contains replacement operations: %d", phase, count)
		case "delete":
			return fmt.Errorf("%s contains deletion operations: %d", phase, count)
		default:
			return fmt.Errorf("%s contains unsupported %s operation: %d", phase, operation, count)
		}
	}
	if changes[expectedOperation] == 0 {
		return fmt.Errorf("%s expected at least one %s operation", phase, expectedOperation)
	}
	return nil
}

func normalizeLifecycleChanges(changes map[apitype.OpType]int) map[string]int {
	result := make(map[string]int, len(changes))
	for operation, count := range changes {
		result[strings.ToLower(string(operation))] += count
	}
	return result
}

func validatePreviewAggregate(summary auto.PreviewResult, phase string) error {
	return validateLifecycleAggregate(phase, normalizeLifecycleChanges(summary.ChangeSummary))
}

func validateUpdateAggregate(summary auto.UpResult, phase string) error {
	if summary.Summary.ResourceChanges == nil {
		return errors.New("update summary has no resource changes")
	}
	return validateLifecycleAggregate(phase, *summary.Summary.ResourceChanges)
}

func assertPreviewAggregate(t *testing.T, summary auto.PreviewResult, phase string) {
	t.Helper()
	if err := validatePreviewAggregate(summary, phase); err != nil {
		t.Fatalf("%s preview summary: %v", phase, err)
	}
}

func assertUpdateAggregate(t *testing.T, summary auto.UpResult, phase string) {
	t.Helper()
	if err := validateUpdateAggregate(summary, phase); err != nil {
		t.Fatalf("%s update summary: %v", phase, err)
	}
}

func validateDestroyedState(deployment apitype.UntypedDeployment) error {
	var state struct {
		Resources []struct {
			Type string `json:"type"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(deployment.Deployment, &state); err != nil {
		return fmt.Errorf("decode exported destroy state: %w", err)
	}
	for _, resource := range state.Resources {
		if strings.HasPrefix(resource.Type, "dokploy:index:") {
			return errors.New("destroy state still contains managed resources")
		}
	}
	return nil
}

func validateStackRemoved(stacks []auto.StackSummary, stackName string) error {
	for _, stack := range stacks {
		if stack.Name == stackName {
			return errors.New("removed stack is still listed")
		}
	}
	return nil
}

type lifecycleIDs struct {
	project, environment, tag, associationProject, associationTag string
}

func assertLifecycleOutputs(t *testing.T, outputs auto.OutputMap, expected lifecycleRevisionValues, cfg liveConfig, previous *lifecycleIDs) lifecycleIDs {
	t.Helper()
	ids := lifecycleIDs{}
	idKeys := map[string]*string{"projectId": &ids.project, "environmentId": &ids.environment, "tagId": &ids.tag, "associationProjectId": &ids.associationProject, "associationTagId": &ids.associationTag}
	for _, key := range []string{"projectId", "environmentId", "tagId", "associationProjectId", "associationTagId"} {
		output, ok := outputs[key]
		if !ok || output.Value == nil || output.Secret {
			t.Fatal(acceptanceFailure("stack output", key))
		}
		value, ok := output.Value.(string)
		if !ok || value == "" {
			t.Fatal(acceptanceFailure("stack output", key))
		}
		*idKeys[key] = value
	}
	if ids.associationProject != ids.project || ids.associationTag != ids.tag {
		t.Fatal(acceptanceFailure("stack output dependencies"))
	}
	if previous != nil && *previous != ids {
		t.Fatal(acceptanceFailure("stable resource identity"))
	}
	for key, want := range map[string]string{"projectDescription": expected.description, "environmentName": acceptanceEntityName(expected.environment, cfg.NameSuffix), "tagColor": expected.color} {
		output, ok := outputs[key]
		got, isString := output.Value.(string)
		if !ok || !isString || got != want {
			t.Error(acceptanceFailure("stack output", key))
		}
	}
	return ids
}

func assertRefreshedLifecycleOutputs(t *testing.T, ctx context.Context, stack auto.Stack, stage string, expected lifecycleRevisionValues, cfg liveConfig, previous *lifecycleIDs) lifecycleIDs {
	t.Helper()
	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Fatal(acceptanceRefreshOutputError(stage, err))
	}
	return assertLifecycleOutputs(t, outputs, expected, cfg, previous)
}

func asID(value pulumi.StringOutput) pulumi.IDInput {
	return value.ApplyT(func(id string) pulumi.ID { return pulumi.ID(id) }).(pulumi.IDOutput)
}
