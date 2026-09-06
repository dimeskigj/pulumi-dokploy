package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	dokploy "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
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
		auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": "file://" + backend}),
	)
	if err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic(err.Error(), cfg))
	}
	// Register cleanup immediately, including for failures during configuration or
	// the first preview. Cleanup is deliberately bounded and uses no state edits.
	t.Cleanup(func() {
		destroyErr, removeErr := cleanupLifecycleStack(2*time.Minute,
			func(ctx context.Context) error {
				_, err := stack.Destroy(ctx)
				return err
			},
			func(ctx context.Context) error {
				return stack.Workspace().RemoveStack(ctx, stackName)
			})
		if destroyErr != nil {
			t.Errorf("%s", sanitizeAcceptanceDiagnostic("destroy lifecycle smoke stack: "+destroyErr.Error(), cfg))
		}
		if removeErr != nil {
			t.Errorf("%s", sanitizeAcceptanceDiagnostic("remove lifecycle smoke workspace: "+removeErr.Error(), cfg))
		}
	})
	if err := stack.SetConfig(ctx, "dokploy:endpoint", auto.ConfigValue{Value: cfg.Endpoint}); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("configure endpoint: "+err.Error(), cfg))
	}
	if err := stack.SetConfig(ctx, "dokploy:apiKey", auto.ConfigValue{Value: cfg.APIKey, Secret: true}); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("configure API key: "+err.Error(), cfg))
	}
	if _, err := stack.Preview(ctx); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("preview revision one with unresolved chained outputs: "+err.Error(), cfg))
	}
	revisionOneUp, err := stack.Up(ctx)
	if err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("up revision one: "+err.Error(), cfg))
	}
	revisionOne := assertLifecycleOutputs(t, revisionOneUp.Outputs, lifecycleRevisionOneValues(), cfg, nil)
	if _, err := stack.Refresh(ctx); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("refresh revision one: "+err.Error(), cfg))
	}
	assertRefreshedLifecycleOutputs(t, ctx, stack, lifecycleRevisionOneValues(), cfg, &revisionOne)

	stack.Workspace().SetProgram(lifecycleRevisionTwo(cfg))
	if _, err := stack.Preview(ctx); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("preview revision two: "+err.Error(), cfg))
	}
	revisionTwoUp, err := stack.Up(ctx)
	if err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("up revision two: "+err.Error(), cfg))
	}
	assertLifecycleOutputs(t, revisionTwoUp.Outputs, lifecycleRevisionTwoValues(), cfg, &revisionOne)
	if _, err := stack.Refresh(ctx); err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("refresh revision two: "+err.Error(), cfg))
	}
	assertRefreshedLifecycleOutputs(t, ctx, stack, lifecycleRevisionTwoValues(), cfg, &revisionOne)
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

func cleanupLifecycleStack(timeout time.Duration, destroy, remove func(context.Context) error) (error, error) {
	destroyCtx, cancelDestroy := context.WithTimeout(context.Background(), timeout)
	destroyErr := destroy(destroyCtx)
	cancelDestroy()

	removeCtx, cancelRemove := context.WithTimeout(context.Background(), timeout)
	removeErr := remove(removeCtx)
	cancelRemove()
	return destroyErr, removeErr
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
			t.Fatalf("%s", sanitizeAcceptanceDiagnostic(fmt.Sprintf("stack output %s was not a non-secret value: %#v", key, output), cfg))
		}
		value, ok := output.Value.(string)
		if !ok || value == "" {
			t.Fatalf("%s", sanitizeAcceptanceDiagnostic(fmt.Sprintf("stack output %s was not a non-empty string: %#v", key, output.Value), cfg))
		}
		*idKeys[key] = value
	}
	if ids.associationProject != ids.project || ids.associationTag != ids.tag {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic(fmt.Sprintf("association outputs do not preserve dependency IDs: %#v", outputs), cfg))
	}
	if previous != nil && *previous != ids {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic(fmt.Sprintf("revision changed resource IDs: revision one=%#v revision two=%#v", *previous, ids), cfg))
	}
	for key, want := range map[string]string{"projectDescription": expected.description, "environmentName": acceptanceEntityName(expected.environment, cfg.NameSuffix), "tagColor": expected.color} {
		output, ok := outputs[key]
		got, isString := output.Value.(string)
		if !ok || !isString || got != want {
			t.Errorf("%s", sanitizeAcceptanceDiagnostic(fmt.Sprintf("stack output %s = %#v, want %q", key, output.Value, want), cfg))
		}
	}
	return ids
}

func assertRefreshedLifecycleOutputs(t *testing.T, ctx context.Context, stack auto.Stack, expected lifecycleRevisionValues, cfg liveConfig, previous *lifecycleIDs) lifecycleIDs {
	t.Helper()
	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Fatalf("%s", sanitizeAcceptanceDiagnostic("read outputs after refresh: "+err.Error(), cfg))
	}
	return assertLifecycleOutputs(t, outputs, expected, cfg, previous)
}

func asID(value pulumi.StringOutput) pulumi.IDInput {
	return value.ApplyT(func(id string) pulumi.ID { return pulumi.ID(id) }).(pulumi.IDOutput)
}
