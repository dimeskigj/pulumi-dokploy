package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// This guard prevents project deletion from cascading through any application
// whose acknowledged absence is still unresolved, or whose create identity was
// never acknowledged. The live parent calls retainProjectIfNeeded before the
// project owner's cleanup callback runs.
type liveApplicationConfigurationGuard struct {
	mu        sync.Mutex
	pending   int
	uncertain bool
}

func newLiveApplicationConfigurationGuard() *liveApplicationConfigurationGuard {
	return &liveApplicationConfigurationGuard{}
}

func (g *liveApplicationConfigurationGuard) beginFixture() {
	g.mu.Lock()
	g.pending++
	g.mu.Unlock()
}

func (g *liveApplicationConfigurationGuard) resolveFixture() {
	g.mu.Lock()
	if g.pending > 0 {
		g.pending--
	}
	g.mu.Unlock()
}

func (g *liveApplicationConfigurationGuard) markUncertain() {
	g.mu.Lock()
	g.uncertain = true
	g.mu.Unlock()
}

func (g *liveApplicationConfigurationGuard) retainProjectIfNeeded(release func()) {
	g.mu.Lock()
	retain := g.pending > 0 || g.uncertain
	g.mu.Unlock()
	if retain {
		release()
	}
}

type liveApplicationConfigurationFixture struct {
	api      *client.Client
	id       string
	source   ApplicationSource
	guard    *liveApplicationConfigurationGuard
	owner    *liveCleanupOwner
	mu       sync.Mutex
	resolved bool
}

// createLiveApplicationConfigurationFixture only creates a bare application
// record. Application.Create is intentionally not used because it deploys.
// autoDeploy is disabled before any source or build configuration is attached.
func createLiveApplicationConfigurationFixture(t *testing.T, ctx context.Context, api *client.Client, environmentID string, source ApplicationSource, guard *liveApplicationConfigurationGuard) (*liveApplicationConfigurationFixture, error) {
	t.Helper()
	if guard == nil {
		return nil, liveRecordLifecycleError("application-configuration", "missing-guard", nil)
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, liveRecordLifecycleError("application-configuration", "missing-environment", nil)
	}
	if err := source.validate(); err != nil {
		return nil, liveRecordLifecycleError("application-configuration", "invalid-source", err)
	}
	t.Cleanup(registerLiveApplicationSourceMetadata(source))
	guard.beginFixture()
	response, createErr := api.ApplicationCreateWithResponse(ctx, generated.ApplicationCreateJSONRequestBody{
		Name: liveRunName("application-configuration"), EnvironmentId: environmentID,
	})
	applicationID := ""
	if response != nil && response.JSON200 != nil && response.JSON200.ApplicationId != nil {
		applicationID = *response.JSON200.ApplicationId
	}
	if strings.TrimSpace(applicationID) == "" {
		if liveRecordCreateDefinitelyRejected(createErr) {
			guard.resolveFixture()
			return nil, liveRecordLifecycleError("application-configuration", "fixture-create-rejected", createErr)
		}
		guard.resolveFixture()
		guard.markUncertain()
		recordStructuralCleanup("application", errors.New("bare application fixture create ownership uncertain"))
		return nil, liveRecordLifecycleError("application-configuration", "fixture-create-ownership", createErr)
	}
	fixture := &liveApplicationConfigurationFixture{api: api, id: applicationID, source: source, guard: guard}
	fixture.owner = newLiveCleanupOwner(func() {
		if err := fixture.cleanup(); err != nil {
			reportLiveCleanup(t, "application", fixture.id, err)
		}
	})
	t.Cleanup(fixture.owner.cleanupOnce)
	if createErr != nil {
		return fixture, liveRecordLifecycleError("application-configuration", "fixture-create", createErr)
	}
	if _, err := api.ApplicationUpdateWithResponse(ctx, generated.ApplicationUpdateJSONRequestBody{
		ApplicationId: applicationID,
		AutoDeploy:    nullable.NewNullableWithValue(false),
	}); err != nil {
		return fixture, liveRecordLifecycleError("application-configuration", "disable-auto-deploy", err)
	}
	autoDeploy, known, err := readLiveApplicationAutoDeploy(ctx, api, applicationID)
	if err != nil || !known || autoDeploy {
		return fixture, liveRecordLifecycleError("application-configuration", "verify-auto-deploy-disabled", err)
	}
	if err := configureApplicationSource(ctx, api, applicationID, source); err != nil {
		return fixture, liveRecordLifecycleError("application-configuration", "save-source", err)
	}
	if err := configureApplicationBuild(ctx, api, applicationID, source); err != nil {
		return fixture, liveRecordLifecycleError("application-configuration", "save-build", err)
	}
	return fixture, nil
}

func readLiveApplicationAutoDeploy(ctx context.Context, api *client.Client, id string) (bool, bool, error) {
	response, err := api.ApplicationOneWithResponse(ctx, &generated.ApplicationOneParams{ApplicationId: id})
	if err != nil {
		return false, false, err
	}
	if response == nil || response.JSON200 == nil || response.JSON200.ApplicationId == nil || strings.TrimSpace(*response.JSON200.ApplicationId) == "" {
		return false, false, errors.New("application.one returned incomplete identity")
	}
	if *response.JSON200.ApplicationId != id {
		return false, false, errors.New("application.one returned mismatched identity")
	}
	var properties map[string]any
	if err := json.Unmarshal(response.Body, &properties); err != nil {
		return false, false, err
	}
	autoDeploy, known := properties["autoDeploy"].(bool)
	return autoDeploy, known, nil
}

func (fixture *liveApplicationConfigurationFixture) cleanup() error {
	if fixture == nil || fixture.id == "" {
		return errors.New("application fixture has no acknowledged identity")
	}
	fixture.mu.Lock()
	if fixture.resolved {
		fixture.mu.Unlock()
		return nil
	}
	fixture.mu.Unlock()

	ctx, cancel := cleanupContext()
	defer cancel()
	const leaseKind = "application-configuration-cleanup"
	if _, err := acquireLiveHeavyOperation(ctx, leaseKind, liveServerHealthProbe(fixture.api)); err != nil {
		return err
	}
	defer endLiveHeavyOperation(leaseKind)
	remove := func(c context.Context) error {
		_, err := fixture.api.ApplicationDeleteWithResponse(c, generated.ApplicationDeleteJSONRequestBody{ApplicationId: fixture.id})
		return err
	}
	readAbsent := func(c context.Context) (string, error) {
		response, err := fixture.api.ApplicationOneWithResponse(c, &generated.ApplicationOneParams{ApplicationId: fixture.id})
		if client.IsNotFound(err) {
			return "", nil
		}
		if err != nil {
			return fixture.id, err
		}
		if response == nil || response.JSON200 == nil || response.JSON200.ApplicationId == nil || strings.TrimSpace(*response.JSON200.ApplicationId) == "" {
			return fixture.id, errors.New("application.one returned incomplete identity")
		}
		return *response.JSON200.ApplicationId, nil
	}
	if err := verifyLiveCleanup(ctx, remove, readAbsent); err != nil {
		return err
	}
	fixture.mu.Lock()
	if !fixture.resolved {
		fixture.resolved = true
		fixture.guard.resolveFixture()
	}
	fixture.mu.Unlock()
	if fixture.owner != nil {
		fixture.owner.release()
	}
	return nil
}

func liveApplicationBuildSequence() []ApplicationBuild {
	return []ApplicationBuild{
		{Type: BuildNixpacks, PublishDirectory: stringPtr("dist"), IsStaticSpa: appBoolPtr(true)},
		{Type: BuildNixpacks, PublishDirectory: stringPtr("public"), IsStaticSpa: appBoolPtr(true)},
		{Type: BuildDockerfile, Dockerfile: stringPtr("Containerfile"), DockerContextPath: stringPtr("services/api"), DockerBuildStage: stringPtr("runtime")},
		{Type: BuildHerokuBuildpacks, HerokuVersion: stringPtr("24")},
		{Type: BuildPaketoBuildpacks},
		{Type: BuildStatic, IsStaticSpa: appBoolPtr(true)},
		{Type: BuildRailpack, RailpackVersion: stringPtr("stable")},
		{Type: BuildNixpacks},
	}
}

func runLiveApplicationConfigurationLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string, source ApplicationSource, guard *liveApplicationConfigurationGuard) error {
	t.Helper()
	if err := source.validate(); err != nil {
		return liveRecordLifecycleError("application-configuration", "invalid-source", err)
	}
	registerLiveApplicationConfigurationMutationMetadata(t, source)
	lease := beginLiveHeavyOperation(t, "application-configuration-"+string(source.Type), liveServerHealthProbe(api))
	defer lease.releaseIfNeeded(t)
	fixture, err := createLiveApplicationConfigurationFixture(t, ctx, api, environmentID, source, guard)
	if err != nil {
		return processLiveApplicationConfigurationError(t, lease, api, "fixture-setup", err)
	}
	r := Application{client: fixedClient(api)}
	falseValue := false
	state, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, State: ApplicationState{ApplicationArgs: ApplicationArgs{Source: source, AutoDeploy: &falseValue}}})
	if err != nil {
		return processLiveApplicationConfigurationError(t, lease, api, "initial-read", err)
	}
	assertLiveApplicationSource(t, string(source.Type), source, state.Inputs.Source)
	requireLiveEqual(t, "application.autoDeploy.initial", &falseValue, state.Inputs.AutoDeploy)
	if !liveApplicationAutoDeployIsFalse(state.Inputs.AutoDeploy) {
		return liveRecordLifecycleError("application-configuration", "initial-auto-deploy", nil)
	}

	current := state
	if current.Inputs.Source.Type == SourceGit {
		for index, build := range liveApplicationBuildSequence() {
			if !liveApplicationAutoDeployIsFalse(current.Inputs.AutoDeploy) {
				return liveRecordLifecycleError("application-configuration", "builder-auto-deploy", nil)
			}
			nextSource := current.Inputs.Source
			gitSource := *current.Inputs.Source.Git
			gitSource.Build = build
			nextSource.Git = &gitSource
			inputs := current.Inputs
			inputs.Source = nextSource
			diff, diffErr := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: inputs, State: current.State})
			sourceDiff, hasSourceDiff := diff.DetailedDiff["source"]
			if diffErr != nil || !diff.HasChanges || !hasSourceDiff || sourceDiff.Kind != p.Update {
				t.Errorf("builder diff contract mismatch: error=%t changes=%t sourceDiff=%t updateKind=%t", diffErr != nil, diff.HasChanges, hasSourceDiff, hasSourceDiff && sourceDiff.Kind == p.Update)
				return liveRecordLifecycleError("application-configuration", fmt.Sprintf("builder-diff-%d", index), diffErr)
			}
			if err = configureApplicationBuild(ctx, api, fixture.id, nextSource); err != nil {
				return processLiveApplicationConfigurationError(t, lease, api, fmt.Sprintf("builder-save-%d", index), err)
			}
			read, readErr := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, State: ApplicationState{ApplicationArgs: ApplicationArgs{Source: nextSource}}})
			if readErr != nil {
				return processLiveApplicationConfigurationError(t, lease, api, fmt.Sprintf("builder-read-%d", index), readErr)
			}
			assertLiveApplicationSource(t, string(build.Type), nextSource, read.Inputs.Source)
			requireLiveEqual(t, "application.autoDeploy.builder", &falseValue, read.Inputs.AutoDeploy)
			if !liveApplicationAutoDeployIsFalse(read.Inputs.AutoDeploy) {
				return liveRecordLifecycleError("application-configuration", fmt.Sprintf("builder-auto-deploy-%d", index), nil)
			}
			imported, importErr := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id})
			if importErr != nil {
				return processLiveApplicationConfigurationError(t, lease, api, fmt.Sprintf("builder-import-%d", index), importErr)
			}
			assertLiveApplicationSource(t, string(build.Type)+" import", nextSource, imported.Inputs.Source)
			if !liveApplicationAutoDeployIsFalse(imported.Inputs.AutoDeploy) {
				return liveRecordLifecycleError("application-configuration", fmt.Sprintf("builder-import-auto-deploy-%d", index), nil)
			}
			diff, diffErr = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: imported.Inputs, State: imported.State})
			if diffErr != nil || diff.HasChanges || len(diff.DetailedDiff) != 0 {
				return liveRecordLifecycleError("application-configuration", fmt.Sprintf("builder-no-op-diff-%d", index), diffErr)
			}
			current = imported
		}
	} else {
		if !liveApplicationAutoDeployIsFalse(current.Inputs.AutoDeploy) {
			return liveRecordLifecycleError("application-configuration", "source-auto-deploy", nil)
		}
		mutated := mutateLiveApplicationSource(current.Inputs.Source)
		inputs := current.Inputs
		inputs.Source = mutated
		diff, diffErr := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: inputs, State: current.State})
		if diffErr != nil || !diff.HasChanges || diff.DetailedDiff["source"].Kind != p.Update {
			return liveRecordLifecycleError("application-configuration", "source-mutation-diff", diffErr)
		}
		if err = configureApplicationSource(ctx, api, fixture.id, mutated); err != nil {
			return processLiveApplicationConfigurationError(t, lease, api, "source-mutation-save", err)
		}
		if err = configureApplicationBuild(ctx, api, fixture.id, mutated); err != nil {
			return processLiveApplicationConfigurationError(t, lease, api, "source-mutation-build", err)
		}
		read, readErr := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, State: ApplicationState{ApplicationArgs: ApplicationArgs{Source: mutated}}})
		if readErr != nil {
			return processLiveApplicationConfigurationError(t, lease, api, "source-mutation-read", readErr)
		}
		assertLiveApplicationSource(t, string(mutated.Type)+" updated", mutated, read.Inputs.Source)
		imported, importErr := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id})
		if importErr != nil {
			return processLiveApplicationConfigurationError(t, lease, api, "source-mutation-import", importErr)
		}
		assertLiveApplicationSource(t, string(mutated.Type)+" updated import", mutated, imported.Inputs.Source)
		diff, diffErr = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: imported.Inputs, State: imported.State})
		if diffErr != nil || diff.HasChanges || len(diff.DetailedDiff) != 0 {
			return liveRecordLifecycleError("application-configuration", "source-mutation-no-op-diff", diffErr)
		}
		current = imported
	}

	// This is only the supported autoDeploy=false setting update: every runtime
	// input is unchanged, so Application.Update cannot enter its redeploy path.
	if !liveApplicationAutoDeployIsFalse(current.Inputs.AutoDeploy) {
		return liveRecordLifecycleError("application-configuration", "update-auto-deploy", nil)
	}
	managedInputs := current.Inputs
	managedInputs.AutoDeploy = &falseValue
	unmanagedState := current.State
	unmanagedState.AutoDeploy = nil
	autoDeployDiff, err := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: managedInputs, State: unmanagedState})
	if err != nil || !autoDeployDiff.HasChanges || autoDeployDiff.DetailedDiff["autoDeploy"].Kind != p.Update {
		return liveRecordLifecycleError("application-configuration", "auto-deploy-diff", err)
	}
	updated, err := r.Update(ctx, infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: managedInputs, State: unmanagedState})
	if err != nil {
		return processLiveApplicationConfigurationError(t, lease, api, "auto-deploy-update", err)
	}
	requireLiveEqual(t, "application.autoDeploy.update", &falseValue, updated.Output.AutoDeploy)
	read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, State: updated.Output})
	if err != nil {
		return processLiveApplicationConfigurationError(t, lease, api, "auto-deploy-read", err)
	}
	requireLiveEqual(t, "application.autoDeploy.persisted", &falseValue, read.Inputs.AutoDeploy)
	if !liveApplicationAutoDeployIsFalse(read.Inputs.AutoDeploy) {
		return liveRecordLifecycleError("application-configuration", "persisted-auto-deploy", nil)
	}
	noOp, err := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{ID: fixture.id, Inputs: read.Inputs, State: read.State})
	if err != nil || noOp.HasChanges || len(noOp.DetailedDiff) != 0 {
		return liveRecordLifecycleError("application-configuration", "auto-deploy-no-op-diff", err)
	}

	lease.releaseIfNeeded(t)
	if err := fixture.cleanup(); err != nil {
		reportLiveCleanup(t, "application", fixture.id, err)
		return liveRecordLifecycleError("application-configuration", "delete", err)
	}
	return nil
}

func registerLiveApplicationConfigurationMutationMetadata(t *testing.T, source ApplicationSource) {
	t.Helper()
	if source.Type == SourceGit && source.Git != nil {
		for _, build := range liveApplicationBuildSequence() {
			updated := source
			git := *source.Git
			git.Build = build
			updated.Git = &git
			t.Cleanup(registerLiveApplicationSourceMetadata(updated))
		}
		return
	}
	t.Cleanup(registerLiveApplicationSourceMetadata(mutateLiveApplicationSource(source)))
}

func liveApplicationAutoDeployIsFalse(value *bool) bool {
	return value != nil && !*value
}

func processLiveApplicationConfigurationError(t *testing.T, lease *liveHeavyOperationLease, api *client.Client, phase string, err error) error {
	t.Helper()
	processed := processLiveHeavyOperationError(t, lease, err, nil, liveServerHealthProbe(api))
	return liveRecordLifecycleError("application-configuration", phase, processed)
}

func mutateLiveApplicationSource(source ApplicationSource) ApplicationSource {
	updated := source
	switch source.Type {
	case SourceGitLab:
		gitLab := *source.GitLab
		gitLab.BuildPath = stringPtr("services/api")
		gitLab.WatchPaths = []string{"services/api/**"}
		updated.GitLab = &gitLab
	case SourceGitHub:
		github := *source.GitHub
		github.BuildPath = stringPtr("services/api")
		github.WatchPaths = []string{"services/api/**"}
		updated.GitHub = &github
	case SourceGitea:
		gitea := *source.Gitea
		gitea.BuildPath = stringPtr("services/api")
		gitea.WatchPaths = []string{"services/api/**"}
		updated.Gitea = &gitea
	case SourceBitbucket:
		bitbucket := *source.Bitbucket
		bitbucket.BuildPath = stringPtr("services/api")
		bitbucket.WatchPaths = []string{"services/api/**"}
		updated.Bitbucket = &bitbucket
	}
	return updated
}

func runLiveApplicationConfigurationCases(t *testing.T, ctx context.Context, api *client.Client, environmentID string, guard *liveApplicationConfigurationGuard) {
	t.Helper()
	runLiveCase(t, "Builders/Git", func(t *testing.T) {
		requireLiveAcceptance(t)
		err := runLiveApplicationConfigurationLifecycle(t, ctx, api, environmentID, offlineConfigurationGitSource(), guard)
		requireLiveLifecycleNoError(t, "Application configuration", "Git builder matrix", err)
	})
	for _, kind := range []ApplicationSourceType{SourceGitHub, SourceGitea, SourceBitbucket, SourceGitLab} {
		kind := kind
		runLiveCase(t, "Sources/"+string(kind), func(t *testing.T) {
			requireLiveAcceptance(t)
			if kind == SourceGitLab {
				t.Cleanup(registerLiveSecrets(os.Getenv("DOKPLOY_GITLAB_USERNAME"), os.Getenv("DOKPLOY_GITLAB_TOKEN")))
			}
			source, configured, err := liveApplicationSourceFromEnvironment(kind)
			if err != nil {
				t.Fatalf("invalid %s application source prerequisites", kind)
			}
			if !configured {
				t.Skip(string(kind) + " application source prerequisites are not configured")
			}
			err = runLiveApplicationConfigurationLifecycle(t, ctx, api, environmentID, source, guard)
			requireLiveLifecycleNoError(t, "Application configuration", string(kind), err)
		})
	}
}

func liveApplicationSourceFromEnvironment(kind ApplicationSourceType) (ApplicationSource, bool, error) {
	values := map[string]string{}
	for _, key := range liveApplicationSourcePrerequisiteKeys(kind) {
		values[key] = os.Getenv(key)
	}
	return parseLiveApplicationSourcePrerequisites(kind, values)
}

func liveApplicationSourcePrerequisiteKeys(kind ApplicationSourceType) []string {
	switch kind {
	case SourceGitHub:
		return []string{"DOKPLOY_GITHUB_INTEGRATION_ID", "DOKPLOY_GITHUB_OWNER", "DOKPLOY_GITHUB_REPOSITORY", "DOKPLOY_GITHUB_BRANCH"}
	case SourceGitea:
		return []string{"DOKPLOY_GITEA_INTEGRATION_ID", "DOKPLOY_GITEA_OWNER", "DOKPLOY_GITEA_REPOSITORY", "DOKPLOY_GITEA_BRANCH"}
	case SourceBitbucket:
		return []string{"DOKPLOY_BITBUCKET_INTEGRATION_ID", "DOKPLOY_BITBUCKET_OWNER", "DOKPLOY_BITBUCKET_REPOSITORY", "DOKPLOY_BITBUCKET_REPOSITORY_SLUG", "DOKPLOY_BITBUCKET_BRANCH"}
	case SourceGitLab:
		return []string{"DOKPLOY_GITLAB_INTEGRATION_ID", "DOKPLOY_GITLAB_PROJECT_ID", "DOKPLOY_GITLAB_OWNER", "DOKPLOY_GITLAB_NAMESPACE", "DOKPLOY_GITLAB_REPOSITORY", "DOKPLOY_GITLAB_BRANCH", "DOKPLOY_GITLAB_USERNAME", "DOKPLOY_GITLAB_TOKEN"}
	default:
		return nil
	}
}

func parseLiveApplicationSourcePrerequisites(kind ApplicationSourceType, values map[string]string) (ApplicationSource, bool, error) {
	keys := liveApplicationSourcePrerequisiteKeys(kind)
	if len(keys) == 0 {
		return ApplicationSource{}, false, errors.New("unsupported application source prerequisite kind")
	}
	populated := 0
	for _, key := range keys {
		if values[key] != "" {
			populated++
		}
	}
	if populated == 0 {
		return ApplicationSource{}, false, nil
	}
	if populated != len(keys) {
		return ApplicationSource{}, true, errors.New("application source prerequisites are incomplete or invalid")
	}
	for _, key := range keys {
		value := values[key]
		if strings.TrimSpace(value) != value || value == "" || strings.ContainsAny(value, " \t\r\n") {
			return ApplicationSource{}, true, errors.New("application source prerequisites are incomplete or invalid")
		}
	}
	value := func(key string) string { return values[key] }
	build := ApplicationBuild{Type: BuildNixpacks}
	switch kind {
	case SourceGitHub:
		return ApplicationSource{Type: kind, GitHub: &GitHubAppSource{IntegrationID: value("DOKPLOY_GITHUB_INTEGRATION_ID"), Owner: value("DOKPLOY_GITHUB_OWNER"), Repository: value("DOKPLOY_GITHUB_REPOSITORY"), Branch: value("DOKPLOY_GITHUB_BRANCH"), Build: build}}, true, nil
	case SourceGitea:
		return ApplicationSource{Type: kind, Gitea: &GiteaAppSource{IntegrationID: value("DOKPLOY_GITEA_INTEGRATION_ID"), Owner: value("DOKPLOY_GITEA_OWNER"), Repository: value("DOKPLOY_GITEA_REPOSITORY"), Branch: value("DOKPLOY_GITEA_BRANCH"), Build: build}}, true, nil
	case SourceBitbucket:
		return ApplicationSource{Type: kind, Bitbucket: &BitbucketAppSource{IntegrationID: value("DOKPLOY_BITBUCKET_INTEGRATION_ID"), Owner: value("DOKPLOY_BITBUCKET_OWNER"), Repository: value("DOKPLOY_BITBUCKET_REPOSITORY"), RepositorySlug: value("DOKPLOY_BITBUCKET_REPOSITORY_SLUG"), Branch: value("DOKPLOY_BITBUCKET_BRANCH"), Build: build}}, true, nil
	case SourceGitLab:
		projectID, err := strconv.Atoi(value("DOKPLOY_GITLAB_PROJECT_ID"))
		if err != nil || projectID <= 0 {
			return ApplicationSource{}, true, errors.New("application source prerequisites are incomplete or invalid")
		}
		return ApplicationSource{Type: kind, GitLab: &GitLabAppSource{IntegrationID: value("DOKPLOY_GITLAB_INTEGRATION_ID"), ProjectID: projectID, Owner: value("DOKPLOY_GITLAB_OWNER"), Namespace: value("DOKPLOY_GITLAB_NAMESPACE"), Repository: value("DOKPLOY_GITLAB_REPOSITORY"), Branch: value("DOKPLOY_GITLAB_BRANCH"), Build: build}}, true, nil
	default:
		return ApplicationSource{}, false, errors.New("unsupported application source prerequisite kind")
	}
}
