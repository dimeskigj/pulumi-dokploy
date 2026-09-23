package dokploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

type domainContractExperiment struct {
	field string
	body  generated.DomainCreateJSONRequestBody
}

type domainExperimentResult struct {
	field, status, code, category string
}

func (r domainExperimentResult) String() string {
	return fmt.Sprintf("field=%s;status=%s;code=%s;category=%s", r.field, r.status, r.code, r.category)
}

var errDomainExperimentMissingID = errors.New("domain experiment returned no resource id")

func domainContractExperimentCases(args DomainArgs, compose bool) []domainContractExperiment {
	base := domainCreateBody(args)
	domainTypeOmitted := base
	domainTypeOmitted.DomainType = nullable.NewNullNullable[generated.DomainCreateJSONBodyDomainType]()
	portOmitted := base
	portOmitted.Port = nullable.NewNullNullable[float32]()
	certificateChanged := base
	certificateChanged.CertificateType = ptr(generated.DomainCreateJSONBodyCertificateType(CertificateLetsencrypt))
	httpsChanged := base
	https := true
	httpsChanged.Https = &https
	stripPathChanged := base
	stripPath := true
	stripPathChanged.StripPath = &stripPath
	cases := []domainContractExperiment{
		{field: "domainType", body: domainTypeOmitted},
		{field: "port", body: portOmitted},
		{field: "certificateType", body: certificateChanged},
		{field: "https", body: httpsChanged},
		{field: "stripPath", body: stripPathChanged},
	}
	if compose {
		serviceNameOmitted := base
		serviceNameOmitted.ServiceName = nullable.NewNullNullable[string]()
		cases = append(cases, domainContractExperiment{field: domainServiceName, body: serviceNameOmitted})
	}
	return cases
}

func classifyDomainExperimentResult(statusClass string, hasID bool) (string, bool) {
	switch statusClass {
	case "2xx":
		if !hasID {
			return "cleanup-failure", false
		}
		return "accepted", false
	case "transport", "5xx":
		return "environment", false
	default:
		return "rejected", true
	}
}

func domainExperimentMustStop(result domainExperimentResult) bool {
	switch result.category {
	case "accepted", "cleanup-failure", "environment":
		return true
	default:
		return false
	}
}

func runDomainContractExperimentSequence(t *testing.T, baseline generated.DomainCreateJSONRequestBody, experiments []domainContractExperiment, run func(string, generated.DomainCreateJSONRequestBody) domainExperimentResult) {
	t.Helper()
	result := run("baseline", baseline)
	if domainExperimentMustStop(result) {
		return
	}
	for _, experiment := range experiments {
		if domainExperimentMustStop(run(experiment.field, experiment.body)) {
			return
		}
	}
}

func runDomainContractExperimentTargets(t *testing.T, names []string, run func(string)) {
	t.Helper()
	for _, name := range names {
		if heavyLiveTierStopped() {
			return
		}
		name := name
		t.Run(name, func(t *testing.T) { run(name) })
	}
}

// TestLiveTier2Workloads is intentionally one serial test. Workload creates
// deploy containers and mounts cause another deploy, so parallel subtests
// would needlessly increase load on the acceptance server.
func TestLiveTier2Workloads(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 30*time.Minute)
	projectID, environmentID, _ := liveProject(t, ctx, api)
	_ = projectID

	var applicationID, composeID string
	var applicationStatus, composeStatus string
	t.Cleanup(func() {
		// This is a failure-path backstop. The normal path explicitly deletes
		// both workloads below, after every dependent subtest has finished.
		if applicationID != "" {
			r := Application{client: fixedClient(api)}
			liveCleanupVerified(t, "application", applicationID, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: applicationID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: applicationID})
				return v.ID, e
			})
		}
		if composeID != "" {
			r := Compose{client: fixedClient(api)}
			liveCleanupVerified(t, "compose", composeID, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: composeID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: composeID})
				return v.ID, e
			})
		}
	})
	t.Run("Application", func(t *testing.T) {
		inputs := ApplicationArgs{Name: liveRunName("application"), EnvironmentID: environmentID, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}}}
		t.Cleanup(registerLiveSecrets(value(inputs.Environment), value(inputs.BuildArgs), value(inputs.BuildSecrets), "APP_ENV_SENTINEL=1", "APP_BUILD_ARG_SENTINEL=1", "APP_BUILD_SECRET_SENTINEL=1"))
		r := Application{client: fixedClient(api)}
		lease := beginLiveHeavyOperation(t, "application-create", liveServerHealthProbe(api))
		created, err := r.Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: inputs})
		processErr := processLiveHeavyOperationError(t, lease, err, func() {
			cleanupAfterCreateError(t, "application", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
				return v.ID, e
			})
		}, liveServerHealthProbe(api))
		requireNoError(t, processErr)
		lease.releaseIfNeeded(t)
		requireLivePresent(t, "application.id", created.ID)
		applicationID = created.ID
		applicationStatus = created.Output.Status
		requireLiveEqual(t, "application.status", statusDone, created.Output.Status)

		read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID, State: created.Output})
		requireNoError(t, err)
		requireLiveEqual(t, "application.source.type", SourceDocker, read.Inputs.Source.Type)
		requireLiveEqual(t, "application.source.docker.image", "nginx:1.27", read.Inputs.Source.Docker.Image)
		updated := read.Inputs
		updated.Name += "-updated"
		updatedDescription := "updated by live workload test"
		updated.Description = &updatedDescription
		lease = beginLiveHeavyOperation(t, "application-update", liveServerHealthProbe(api))
		changed, err := r.Update(ctx, infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, processLiveHeavyOperationError(t, lease, err, nil, liveServerHealthProbe(api)))
		lease.releaseIfNeeded(t)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID, State: changed.Output})
		requireNoError(t, err)
		requireLiveEqual(t, "application.name", updated.Name, postUpdate.Inputs.Name)
		requireLiveEqual(t, "application.description", updatedDescription, value(postUpdate.Inputs.Description))
		imported, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "application.id", created.ID, imported.State.ApplicationID)
		requireLiveEqual(t, "application.name", postUpdate.Inputs.Name, imported.Inputs.Name)
		requireLiveEqual(t, "application.environmentId", postUpdate.Inputs.EnvironmentID, imported.Inputs.EnvironmentID)
		requireLiveEqual(t, "application.source.type import", SourceDocker, imported.Inputs.Source.Type)
		requireLivePresent(t, "application.source.docker", imported.Inputs.Source.Docker)
		requireLiveEqual(t, "application.source.docker.image import", "nginx:1.27", imported.Inputs.Source.Docker.Image)

		diff, err := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		// Mutable metadata is checked through the provider diff contract without
		// triggering four additional deploys on the low-power fixture.
		for _, change := range []struct {
			field string
			apply func(*ApplicationArgs)
		}{
			{"environment", func(a *ApplicationArgs) { a.Environment = stringPtr("APP_ENV_SENTINEL=1") }},
			{"buildArgs", func(a *ApplicationArgs) { a.BuildArgs = stringPtr("APP_BUILD_ARG_SENTINEL=1") }},
			{"buildSecrets", func(a *ApplicationArgs) { a.BuildSecrets = stringPtr("APP_BUILD_SECRET_SENTINEL=1") }},
			{"createEnvFile", func(a *ApplicationArgs) { a.CreateEnvFile = !a.CreateEnvFile }},
		} {
			changedInputs := postUpdate.Inputs
			change.apply(&changedInputs)
			changedDiff, diffErr := r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: changedInputs, State: postUpdate.State})
			requireNoError(t, diffErr)
			requireLiveEqual(t, "application.diff."+change.field, p.Update, changedDiff.DetailedDiff[change.field].Kind)
		}
		replacement := postUpdate.Inputs
		replacement.Source = ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}
		diff, err = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: replacement, State: postUpdate.State})
		requireNoError(t, err)
		requireLiveEqual(t, "application.diff.source.type", p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
		environmentReplacement := postUpdate.Inputs
		environmentReplacement.Environment = stringPtr("REPLACED=1")
		diff, err = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: environmentReplacement, State: postUpdate.State})
		requireNoError(t, err)
		requireLiveEqual(t, "application.diff.environment", p.Update, diff.DetailedDiff["environment"].Kind)

	})

	t.Run("Compose", func(t *testing.T) {
		inputs := ComposeArgs{Name: liveRunName("compose"), EnvironmentID: environmentID, Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}}}
		t.Cleanup(registerLiveSecrets(value(inputs.Environment), "COMPOSE_ENV=1"))
		r := Compose{client: fixedClient(api)}
		lease := beginLiveHeavyOperation(t, "compose-create", liveServerHealthProbe(api))
		created, err := r.Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: inputs})
		processErr := processLiveHeavyOperationError(t, lease, err, func() {
			cleanupAfterCreateError(t, "compose", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
				return v.ID, e
			})
		}, liveServerHealthProbe(api))
		requireNoError(t, processErr)
		lease.releaseIfNeeded(t)
		requireLivePresent(t, "compose.id", created.ID)
		composeID = created.ID
		composeStatus = created.Output.Status
		requireLiveEqual(t, "compose.status", statusDone, created.Output.Status)

		read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID, State: created.Output})
		requireNoError(t, err)
		requireLiveEqual(t, "compose.source.type", ComposeSourceRaw, read.Inputs.Source.Type)
		updated := read.Inputs
		updated.Name += "-updated"
		updatedDescription := "updated by live workload test"
		updated.Description = &updatedDescription
		lease = beginLiveHeavyOperation(t, "compose-update", liveServerHealthProbe(api))
		changed, err := r.Update(ctx, infer.UpdateRequest[ComposeArgs, ComposeState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, processLiveHeavyOperationError(t, lease, err, nil, liveServerHealthProbe(api)))
		lease.releaseIfNeeded(t)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID, State: changed.Output})
		requireNoError(t, err)
		requireLiveEqual(t, "compose.name", updated.Name, postUpdate.Inputs.Name)
		imported, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "compose.id", created.ID, imported.State.ComposeID)
		requireLiveEqual(t, "compose.name", postUpdate.Inputs.Name, imported.Inputs.Name)
		requireLiveEqual(t, "compose.environmentId", postUpdate.Inputs.EnvironmentID, imported.Inputs.EnvironmentID)
		requireLiveEqual(t, "compose.source.type import", ComposeSourceRaw, imported.Inputs.Source.Type)
		requireLivePresent(t, "compose.source.raw", imported.Inputs.Source.Raw)
		requireLiveContains(t, "compose.source.raw.composeFile", imported.Inputs.Source.Raw.ComposeFile, "image: nginx:1.27")
		for _, change := range []struct {
			field string
			apply func(*ComposeArgs)
			kind  p.DiffKind
		}{
			{"environment", func(a *ComposeArgs) { a.Environment = stringPtr("COMPOSE_ENV=1") }, p.Update},
			{"composeType", func(a *ComposeArgs) { a.ComposeType = ComposeStack }, p.UpdateReplace},
			{"deleteVolumesOnDestroy", func(a *ComposeArgs) { a.DeleteVolumesOnDestroy = !a.DeleteVolumesOnDestroy }, p.Update},
		} {
			changedInputs := postUpdate.Inputs
			change.apply(&changedInputs)
			changedDiff, diffErr := r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: changedInputs, State: postUpdate.State})
			requireNoError(t, diffErr)
			requireLiveEqual(t, "compose.diff."+change.field, change.kind, changedDiff.DetailedDiff[change.field].Kind)
		}
		diff, err := r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		sourceReplacement, environmentReplacement := composeDiffInputs(postUpdate.Inputs)
		diff, err = r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: sourceReplacement, State: postUpdate.State})
		requireNoError(t, err)
		requireLiveEqual(t, "compose.diff.source.type", p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: environmentReplacement, State: postUpdate.State})
		requireNoError(t, err)
		requireLiveEqual(t, "compose.diff.environment", p.Update, diff.DetailedDiff["environment"].Kind)
	})

	workloadTargets := []struct {
		name, id, status string
		compose          bool
	}{{"application", applicationID, applicationStatus, false}, {"compose", composeID, composeStatus, true}}
	for _, target := range workloadTargets {
		target := target
		t.Run("Domain/"+target.name, func(t *testing.T) {
			readiness := applicationTargetReadiness(api, target.id)
			if target.compose {
				readiness = composeTargetReadiness(api, target.id)
			}
			targetPresent, targetReady, readErr := readiness(ctx)
			requireLiveLifecycleNoError(t, "Domain", "read ready target", readErr)
			if !targetPresent || !targetReady {
				classification, classificationErr := classifyWorkloadCreateAttempt("domain", 0, "", domainCreateRequestKeys(target.compose), targetPresent, targetReady)
				requireNoError(t, classificationErr)
				t.Fatalf("workload target unavailable: %s", classification)
			}
			r := Domain{client: fixedClient(api)}
			args := DomainArgs{Host: liveDomainHost("domain"), Port: intPtr(80), CertificateType: CertificateNone, Enabled: true}
			if target.compose {
				args.ComposeID, args.ServiceName = &target.id, stringPtr("web")
			} else {
				args.ApplicationID = &target.id
			}
			created, err := r.Create(ctx, infer.CreateRequest[DomainArgs]{Inputs: args})
			cleanupAfterCreateError(t, "domain", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return v.ID, e
			})
			if err != nil && !heavyLiveTierStopped() {
				providerKeys, keysErr := domainCreateRequestKeysFromBody(domainCreateBody(args))
				requireNoError(t, keysErr)
				providerClassification, classificationErr := classifyWorkloadCreateError("domain", err, providerKeys, targetPresent, targetReady)
				requireNoError(t, classificationErr)
				providerResult := liveDomainCreateResult{path: "provider", target: target.name, classification: providerClassification, keys: providerKeys}
				directArgs := args
				directArgs.Host = liveDomainHost("domain-direct")
				var generatedResult liveDomainCreateResult
				comparison := compareDomainAttempts(func() liveDomainCreateResult {
					return providerResult
				}, func() liveDomainCreateResult {
					generatedResult = runGeneratedDomainCreateAttempt(t, ctx, api, target.name, directArgs)
					return generatedResult
				})
				_ = comparison
				recordLiveOutcome("domain-comparison", formatDomainComparisonEvidence(providerResult, generatedResult))
			}
			release := func() {}
			if err == nil {
				release = registerLiveCleanup(t, "domain", created.ID, func(c context.Context) error {
					_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: created.ID})
					return e
				}, func(c context.Context) (string, error) {
					v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
					return v.ID, e
				})
			}
			requireWorkloadCreateNoError(t, "domain", err, domainCreateRequestKeys(target.compose), targetPresent, targetReady, "Domain/"+target.name)
			read, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: created.Output})
			requireWorkloadLifecycleNoError(t, "domain", err)
			updated := read.Inputs
			updated.Host = liveDomainHost("updated-domain")
			updated.Path, updated.InternalPath, updated.Port = stringPtr("/public"), stringPtr("/internal"), intPtr(8080)
			updated.HTTPS = true
			updated.Enabled = false
			updatedState, err := r.Update(ctx, infer.UpdateRequest[DomainArgs, DomainState]{ID: created.ID, Inputs: updated, State: read.State})
			requireWorkloadLifecycleNoError(t, "domain", err)
			postUpdate, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: updatedState.Output})
			requireWorkloadLifecycleNoError(t, "domain", err)
			requireLiveEqual(t, "domain.host", updated.Host, postUpdate.Inputs.Host)
			requireLiveEqual(t, "domain.path", updated.Path, postUpdate.Inputs.Path)
			requireLiveEqual(t, "domain.internalPath", updated.InternalPath, postUpdate.Inputs.InternalPath)
			requireLiveEqual(t, "domain.port", updated.Port, postUpdate.Inputs.Port)
			requireLiveEqual(t, "domain.https", updated.HTTPS, postUpdate.Inputs.HTTPS)
			requireLiveEqual(t, "domain.enabled", updated.Enabled, postUpdate.Inputs.Enabled)
			updated = postUpdate.Inputs
			updated.StripPath = true
			stripPathState, err := r.Update(ctx, infer.UpdateRequest[DomainArgs, DomainState]{ID: created.ID, Inputs: updated, State: postUpdate.State})
			requireWorkloadLifecycleNoError(t, "domain", err)
			postStripPath, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: stripPathState.Output})
			requireWorkloadLifecycleNoError(t, "domain", err)
			requireLiveEqual(t, "domain.stripPath", updated.StripPath, postStripPath.Inputs.StripPath)
			postUpdate = postStripPath
			// Import-style reads must reconstruct state from the ID alone.
			imported, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
			requireWorkloadLifecycleNoError(t, "domain", err)
			requireLiveEqual(t, "domain.host", updated.Host, imported.Inputs.Host)
			requireLiveEqual(t, "domain.path", updated.Path, imported.Inputs.Path)
			requireLiveEqual(t, "domain.internalPath", updated.InternalPath, imported.Inputs.InternalPath)
			requireLiveEqual(t, "domain.port", updated.Port, imported.Inputs.Port)
			requireLiveEqual(t, "domain.enabled", updated.Enabled, imported.Inputs.Enabled)
			requireLiveEqual(t, "domain.https", updated.HTTPS, imported.Inputs.HTTPS)
			requireLiveEqual(t, "domain.stripPath", updated.StripPath, imported.Inputs.StripPath)
			requireLiveEqual(t, "domain.certificateType", updated.CertificateType, imported.Inputs.CertificateType)
			if target.compose {
				requireLiveEqual(t, "domain.composeId", target.id, value(imported.Inputs.ComposeID))
				requireLiveEqual(t, "domain.serviceName", "web", value(imported.Inputs.ServiceName))
			} else {
				requireLiveEqual(t, "domain.applicationId", target.id, value(imported.Inputs.ApplicationID))
			}
			err = deleteAndVerifyLiveOwned(ctx, func() error {
				_, e := r.Delete(ctx, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func() (string, error) {
				gone, e := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return gone.ID, e
			}, release)
			requireWorkloadLifecycleNoError(t, "domain", err)
			gone, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
			requireWorkloadLifecycleNoError(t, "domain", err)
			requireLiveEqual(t, "domain.id after delete", "", gone.ID)
		})
		t.Run("Domain/custom-certificate/"+target.name, func(t *testing.T) {
			resolver := os.Getenv("DOKPLOY_CUSTOM_CERT_RESOLVER")
			if resolver == "" {
				t.Skip("DOKPLOY_CUSTOM_CERT_RESOLVER is not configured")
			}
			register := registerLiveSecrets(resolver)
			t.Cleanup(register)
			r := Domain{client: fixedClient(api)}
			readiness := applicationTargetReadiness(api, target.id)
			if target.compose {
				readiness = composeTargetReadiness(api, target.id)
			}
			targetPresent, targetReady, readErr := readiness(ctx)
			requireLiveLifecycleNoError(t, "custom Domain", "read ready target", readErr)
			if !targetPresent || !targetReady {
				t.Skip("workload target is not ready for custom certificate coverage")
			}
			args := DomainArgs{Host: liveDomainHost("custom-domain"), HTTPS: true, CertificateType: CertificateCustom, CustomCertResolver: &resolver, StripPath: true, Enabled: true}
			if target.compose {
				args.ComposeID, args.ServiceName = &target.id, stringPtr("web")
			} else {
				args.ApplicationID = &target.id
			}
			created, err := r.Create(ctx, infer.CreateRequest[DomainArgs]{Inputs: args})
			cleanupAfterCreateError(t, "custom domain", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return v.ID, e
			})
			requireWorkloadCreateNoError(t, "custom domain", err, domainCreateRequestKeys(target.compose), targetPresent, targetReady, "Domain/custom-certificate/"+target.name)
			owner := registerLiveCleanup(t, "custom domain", created.ID, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return v.ID, e
			})
			read, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: created.Output})
			requireWorkloadLifecycleNoError(t, "custom domain", err)
			requireLiveEqual(t, "custom domain.certificateType", CertificateCustom, read.Inputs.CertificateType)
			requireLiveEqual(t, "custom domain.customCertResolver", &resolver, read.Inputs.CustomCertResolver)
			requireLiveEqual(t, "custom domain.https", true, read.Inputs.HTTPS)
			requireLiveEqual(t, "custom domain.stripPath", true, read.Inputs.StripPath)
			imported, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
			requireWorkloadLifecycleNoError(t, "custom domain", err)
			requireLiveEqual(t, "custom domain.certificateType import", CertificateCustom, imported.Inputs.CertificateType)
			requireLiveEqual(t, "custom domain.customCertResolver import", &resolver, imported.Inputs.CustomCertResolver)
			err = deleteAndVerifyLiveOwned(ctx, func() error {
				_, e := r.Delete(ctx, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func() (string, error) {
				gone, e := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return gone.ID, e
			}, owner)
			requireWorkloadLifecycleNoError(t, "custom domain", err)
		})
	}

	for _, target := range workloadTargets {
		target := target
		t.Run("Mounts/"+target.name, func(t *testing.T) {
			mounts := []MountArgs{
				{Type: mountTypeBind, MountPath: "/mnt/bind", HostPath: stringPtr(filepath.Join("/tmp", liveRunName("mount")))},
				{Type: mountTypeVolume, MountPath: "/mnt/volume", VolumeName: stringPtr(liveRunName("volume"))},
				{Type: mountTypeFile, MountPath: "/mnt/file", FilePath: stringPtr("/etc/pulumi-live.conf"), Content: stringPtr("LIVE_MOUNT=1")},
			}
			for _, inputs := range mounts {
				inputs := inputs
				t.Run(inputs.Type, func(t *testing.T) {
					t.Cleanup(func() {
						classification := "pass"
						if t.Failed() {
							classification = "fail"
						}
						recordLiveOutcome("Mount/"+inputs.Type, classification)
					})
					if target.compose {
						inputs.ComposeID = &target.id
					} else {
						inputs.ApplicationID = &target.id
					}
					targetReplacement := inputs
					if target.compose {
						targetReplacement.ApplicationID, targetReplacement.ComposeID = &applicationID, nil
					} else {
						targetReplacement.ApplicationID, targetReplacement.ComposeID = nil, &composeID
					}
					readiness := applicationTargetReadiness(api, target.id)
					if target.compose {
						readiness = composeTargetReadiness(api, target.id)
					}
					runLiveMountLifecycle(t, ctx, api, inputs, targetReplacement, readiness)
				})
			}
		})
	}

	// Dispatch-only cases prove every supported target route without adding an
	// update/redeploy cycle to the matrix. Each database fixture is created and
	// removed before the next one is started.
	t.Run("MountDispatch/postgres", func(t *testing.T) {
		fixture := createDispatchDatabase(t, ctx, api, environmentID, "postgres")
		// Register the fixture immediately. runLiveMountLifecycle registers its
		// mount owner after this one, so Go's LIFO cleanup guarantees the mount is
		// removed before the database fixture (and therefore before its lease is
		// released) if any lifecycle assertion fails.
		fixtureOwner := newLiveCleanupOwner(func() { fixture.cleanup(t) })
		t.Cleanup(fixtureOwner.cleanupOnce)
		fixture.releaseForDependentMount(t)
		mount := MountArgs{Type: mountTypeBind, MountPath: "/mnt/postgres", HostPath: stringPtr(filepath.Join("/tmp", liveRunName("postgres-mount"))), PostgresID: &fixture.id}
		t.Cleanup(registerLiveSecrets(value(mount.HostPath)))
		targetReplacement := mount
		targetReplacement.PostgresID = nil
		targetReplacement.ApplicationID = &applicationID
		runLiveMountLifecycle(t, ctx, api, mount, targetReplacement, fixture.readiness, true)
		// The mount helper disarms its owner after absence is verified. Only then
		// clean the fixture, exactly once, while retaining the heavy-operation
		// lease until the dependent mount is gone.
		fixtureOwner.cleanupOnce()
	})

	for _, target := range []string{"compose", "mysql", "mariadb", "redis"} {
		target := target
		t.Run("MountDispatch/"+target, func(t *testing.T) {
			var targetID string
			var fixture *liveDispatchFixture
			switch target {
			case "compose":
				targetID = composeID
			case "postgres", "mysql", "mariadb", "redis":
				fixture = createDispatchDatabase(t, ctx, api, environmentID, target)
				targetID = fixture.id
				t.Cleanup(func() { fixture.cleanup(t) })
			}
			if targetID == "" {
				t.Skip("dependent workload target was not created successfully")
			}
			if fixture != nil {
				fixture.releaseForDependentMount(t)
			}
			serviceType := target
			mount := MountArgs{Type: mountTypeBind, MountPath: "/mnt/dispatch", HostPath: stringPtr(filepath.Join("/tmp", liveRunName("dispatch")))}
			t.Cleanup(registerLiveSecrets(value(mount.Content), value(mount.HostPath), value(mount.VolumeName)))
			setMountTarget(&mount, targetIDField(serviceType), targetID)
			r := Mount{client: fixedClient(api)}
			var created infer.CreateResponse[MountState]
			var err error
			readiness := composeTargetReadiness(api, targetID)
			if fixture != nil {
				readiness = fixture.readiness
			}
			readyErr := readReadyMountTarget(func() (string, error) {
				present, ready, readErr := readiness(ctx)
				if readErr != nil {
					return "", readErr
				}
				if !present {
					return "", nil
				}
				if ready {
					return statusDone, nil
				}
				return "running", nil
			}, func() {
				created, err = r.Create(ctx, infer.CreateRequest[MountArgs]{Inputs: mount})
			})
			requireLiveLifecycleNoError(t, "Mount", "read ready target", readyErr)
			cleanupAfterCreateError(t, "mount", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
				return v.ID, e
			})
			release := func() {}
			if err == nil {
				release = registerLiveCleanup(t, "mount", created.ID, func(c context.Context) error {
					_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
					return e
				}, func(c context.Context) (string, error) {
					v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
					return v.ID, e
				})
			}
			requireWorkloadCreateNoError(t, "mount", err, mountCreateRequestKeys(mount), targetID != "", targetID != "", "MountDispatch/"+target)
			read, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: created.Output})
			requireWorkloadLifecycleNoError(t, "mount", err)
			imported, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: read.State})
			requireWorkloadLifecycleNoError(t, "mount", err)
			requireLiveEqual(t, "mount."+serviceType+"Id", targetID, mountTargetID(imported.Inputs, serviceType))
			err = deleteAndVerifyLiveOwned(ctx, func() error {
				_, e := r.Delete(ctx, infer.DeleteRequest[MountState]{ID: created.ID, State: imported.State})
				return e
			}, func() (string, error) {
				gone, e := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
				return gone.ID, e
			}, release)
			requireWorkloadLifecycleNoError(t, "mount", err)
			gone, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
			requireWorkloadLifecycleNoError(t, "mount", err)
			requireLiveEqual(t, "mount.id after delete", "", gone.ID)
			if target != "compose" {
				fixture.cleanup(t)
			}
		})
	}

	// Source variants configure bare resources and do not deploy or clone
	// anything. Git is always available; external integrations are gated.
	t.Run("SourceVariants", func(t *testing.T) {
		registrySource := liveRegistryApplicationSource()
		gitLabSource := liveGitLabApplicationSource()
		composeGitLabSource := gitLabComposeSource()
		variants := []struct {
			name   string
			source ApplicationSource
			ok     bool
		}{
			{name: "git", source: ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}, ok: true},
			{name: "registry", source: registrySource, ok: registrySource.Type != ""},
			{name: "gitlab", source: gitLabSource, ok: gitLabSource.Type != ""},
		}
		for _, variant := range variants {
			variant := variant
			t.Run(variant.name, func(t *testing.T) {
				if !variant.ok {
					t.Skip("dedicated source prerequisite variables are not configured")
				}
				t.Cleanup(registerLiveApplicationSourceMetadata(variant.source))
				createLease := beginLiveHeavyOperation(t, "application-source-"+variant.name+"-create", liveServerHealthProbe(api))
				defer createLease.releaseIfNeeded(t)
				body := generated.ApplicationCreateJSONRequestBody{Name: liveRunName("application-" + variant.name), EnvironmentId: environmentID}
				response, err := api.ApplicationCreateWithResponse(ctx, body)
				requireNoError(t, processLiveHeavyOperationError(t, createLease, err, nil, liveServerHealthProbe(api)))
				require.NotNil(t, response.JSON200)
				require.NotNil(t, response.JSON200.ApplicationId)
				id := *response.JSON200.ApplicationId
				r := Application{client: fixedClient(api)}
				release := registerLiveCleanup(t, "application", id, func(c context.Context) error {
					_, deleteErr := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: id})
					return deleteErr
				}, func(c context.Context) (string, error) {
					gone, readErr := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
					return gone.ID, readErr
				})
				requireNoError(t, processLiveHeavyOperationError(t, createLease, configureApplicationSource(ctx, api, id, variant.source), nil, liveServerHealthProbe(api)))
				requireNoError(t, processLiveHeavyOperationError(t, createLease, configureApplicationBuild(ctx, api, id, variant.source), nil, liveServerHealthProbe(api)))
				createLease.releaseIfNeeded(t)
				read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id, State: ApplicationState{ApplicationArgs: ApplicationArgs{Source: variant.source}}})
				requireNoError(t, err)
				assertLiveApplicationSource(t, variant.name, variant.source, read.Inputs.Source)
				if variant.source.Type == SourceDocker {
					requireLiveEqual(t, "application.source.docker.password", variant.source.Docker.Password, read.Inputs.Source.Docker.Password)
				}
				updated := read.Inputs
				updated.Description = stringPtr("source variant metadata update")
				updateLease := beginLiveHeavyOperation(t, "application-source-"+variant.name+"-update", liveServerHealthProbe(api))
				_, err = r.Update(ctx, infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: id, Inputs: updated, State: read.State})
				requireNoError(t, processLiveHeavyOperationError(t, updateLease, err, nil, liveServerHealthProbe(api)))
				updateLease.releaseIfNeeded(t)
				postUpdate, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id, State: read.State})
				requireNoError(t, err)
				assertLiveApplicationSource(t, variant.name, variant.source, postUpdate.Inputs.Source)
				imported, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
				requireNoError(t, err)
				assertLiveApplicationSource(t, variant.name, variant.source, imported.Inputs.Source)
				cleanupCtx, cancelCleanup := cleanupContext()
				defer cancelCleanup()
				err = deleteAndVerifyLiveOwned(cleanupCtx, func() error {
					_, deleteErr := r.Delete(cleanupCtx, infer.DeleteRequest[ApplicationState]{ID: id})
					return deleteErr
				}, func() (string, error) {
					gone, readErr := r.Read(cleanupCtx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
					return gone.ID, readErr
				}, release)
				requireNoError(t, err)
			})
		}
		t.Run("compose-git", func(t *testing.T) {
			createLease := beginLiveHeavyOperation(t, "compose-source-git-create", liveServerHealthProbe(api))
			defer createLease.releaseIfNeeded(t)
			response, err := api.ComposeCreateWithResponse(ctx, generated.ComposeCreateJSONRequestBody{Name: liveRunName("compose-git"), EnvironmentId: environmentID, ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker))})
			requireNoError(t, processLiveHeavyOperationError(t, createLease, err, nil, liveServerHealthProbe(api)))
			require.NotNil(t, response.JSON200)
			require.NotNil(t, response.JSON200.ComposeId)
			id := *response.JSON200.ComposeId
			r := Compose{client: fixedClient(api)}
			release := registerLiveCleanup(t, "compose", id, func(c context.Context) error {
				_, deleteErr := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: id})
				return deleteErr
			}, func(c context.Context) (string, error) {
				gone, readErr := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
				return gone.ID, readErr
			})
			source := ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main", ComposePath: defaultComposePath}}
			t.Cleanup(registerLiveComposeSourceMetadata(source))
			requireNoError(t, processLiveHeavyOperationError(t, createLease, configureComposeSource(ctx, api, id, source), nil, liveServerHealthProbe(api)))
			requireNoError(t, processLiveHeavyOperationError(t, createLease, fetchComposeSource(ctx, api, id, ComposeSourceGit), nil, liveServerHealthProbe(api)))
			createLease.releaseIfNeeded(t)
			read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id, State: ComposeState{ComposeArgs: ComposeArgs{Source: source}}})
			requireNoError(t, err)
			assertComposeSourceFields(t, source, read.Inputs.Source)
			updated := read.Inputs
			updated.Description = stringPtr("compose source metadata update")
			updateLease := beginLiveHeavyOperation(t, "compose-source-git-update", liveServerHealthProbe(api))
			_, err = r.Update(ctx, infer.UpdateRequest[ComposeArgs, ComposeState]{ID: id, Inputs: updated, State: read.State})
			requireNoError(t, processLiveHeavyOperationError(t, updateLease, err, nil, liveServerHealthProbe(api)))
			updateLease.releaseIfNeeded(t)
			postUpdate, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id, State: read.State})
			requireNoError(t, err)
			assertComposeSourceFields(t, source, postUpdate.Inputs.Source)
			imported, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
			requireNoError(t, err)
			assertComposeSourceFields(t, source, imported.Inputs.Source)
			cleanupCtx, cancelCleanup := cleanupContext()
			defer cancelCleanup()
			err = deleteAndVerifyLiveOwned(cleanupCtx, func() error {
				_, deleteErr := r.Delete(cleanupCtx, infer.DeleteRequest[ComposeState]{ID: id, State: imported.State})
				return deleteErr
			}, func() (string, error) {
				gone, readErr := r.Read(cleanupCtx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
				return gone.ID, readErr
			}, release)
			requireNoError(t, err)
		})
		if composeGitLabSource.Type == ComposeSourceGitLab {
			t.Run("compose-gitlab", func(t *testing.T) {
				createLease := beginLiveHeavyOperation(t, "compose-source-gitlab-create", liveServerHealthProbe(api))
				defer createLease.releaseIfNeeded(t)
				response, err := api.ComposeCreateWithResponse(ctx, generated.ComposeCreateJSONRequestBody{Name: liveRunName("compose-gitlab"), EnvironmentId: environmentID, ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker))})
				requireNoError(t, processLiveHeavyOperationError(t, createLease, err, nil, liveServerHealthProbe(api)))
				require.NotNil(t, response.JSON200)
				require.NotNil(t, response.JSON200.ComposeId)
				id := *response.JSON200.ComposeId
				r := Compose{client: fixedClient(api)}
				release := registerLiveCleanup(t, "compose", id, func(c context.Context) error {
					_, deleteErr := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: id})
					return deleteErr
				}, func(c context.Context) (string, error) {
					gone, readErr := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
					return gone.ID, readErr
				})
				t.Cleanup(registerLiveComposeSourceMetadata(composeGitLabSource))
				requireNoError(t, processLiveHeavyOperationError(t, createLease, configureComposeSource(ctx, api, id, composeGitLabSource), nil, liveServerHealthProbe(api)))
				requireNoError(t, processLiveHeavyOperationError(t, createLease, fetchComposeSource(ctx, api, id, ComposeSourceGitLab), nil, liveServerHealthProbe(api)))
				createLease.releaseIfNeeded(t)
				read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id, State: ComposeState{ComposeArgs: ComposeArgs{Source: composeGitLabSource}}})
				requireNoError(t, err)
				assertComposeSourceFields(t, composeGitLabSource, read.Inputs.Source)
				updated := read.Inputs
				updated.Description = stringPtr("compose gitlab source metadata update")
				updateLease := beginLiveHeavyOperation(t, "compose-source-gitlab-update", liveServerHealthProbe(api))
				_, err = r.Update(ctx, infer.UpdateRequest[ComposeArgs, ComposeState]{ID: id, Inputs: updated, State: read.State})
				requireNoError(t, processLiveHeavyOperationError(t, updateLease, err, nil, liveServerHealthProbe(api)))
				updateLease.releaseIfNeeded(t)
				postUpdate, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id, State: read.State})
				requireNoError(t, err)
				assertComposeSourceFields(t, composeGitLabSource, postUpdate.Inputs.Source)
				imported, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
				requireNoError(t, err)
				assertComposeSourceFields(t, composeGitLabSource, imported.Inputs.Source)
				cleanupCtx, cancelCleanup := cleanupContext()
				defer cancelCleanup()
				err = deleteAndVerifyLiveOwned(cleanupCtx, func() error {
					_, deleteErr := r.Delete(cleanupCtx, infer.DeleteRequest[ComposeState]{ID: id, State: imported.State})
					return deleteErr
				}, func() (string, error) {
					gone, readErr := r.Read(cleanupCtx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
					return gone.ID, readErr
				}, release)
				requireNoError(t, err)
			})
		} else {
			t.Run("compose-gitlab", func(t *testing.T) { t.Skip("dedicated GitLab source prerequisite variables are not configured") })
		}
	})

	// Keep both workloads alive through every dependent Domain/Mount and
	// metadata subtest; these are the final explicit lifecycle operations.
	if applicationID != "" {
		deleteAndReadApplication(t, ctx, Application{client: fixedClient(api)}, &applicationID)
		applicationID = ""
	}
	if composeID != "" {
		deleteAndReadCompose(t, ctx, Compose{client: fixedClient(api)}, &composeID)
		composeID = ""
	}
}

func workloadDependencyReady(id, status string) bool {
	return id != "" && status == statusDone
}

type liveTargetReadiness func(context.Context) (present bool, ready bool, err error)

func applicationTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Application{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func composeTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Compose{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func postgresTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Postgres{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func mysqlTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (MySQL{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func mariadbTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (MariaDB{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func redisTargetReadiness(api *client.Client, id string) liveTargetReadiness {
	return func(ctx context.Context) (bool, bool, error) {
		read, err := (Redis{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
		return read.ID != "", read.State.Status == statusDone, err
	}
}

func domainCreateRequestKeys(compose bool) []string {
	keys := []string{"certificateType", "customCertResolver", "host", "https", "internalPath", "path", "port", "serviceName", "stripPath"}
	if compose {
		return append(keys, "composeId", "domainType")
	}
	return append(keys, "applicationId", "domainType")
}

func runGeneratedDomainCreateAttempt(t *testing.T, ctx context.Context, api *client.Client, target string, args DomainArgs) liveDomainCreateResult {
	t.Helper()
	body := domainCreateBody(args)
	keys, err := domainCreateRequestKeysFromBody(body)
	if err != nil {
		t.Fatalf("domain request keys unavailable")
	}
	response, requestErr := api.DomainCreateWithResponse(ctx, body)
	return finalizeGeneratedDomainCreateAttempt(t, target, keys, response, requestErr, func(id string) {
		registerGeneratedDomainCleanup(t, api, id)
	})
}

func registerGeneratedDomainCleanup(t *testing.T, api *client.Client, id string) {
	registerLiveCleanup(t, "domain", id, func(c context.Context) error {
		_, removeErr := api.DomainDeleteWithResponse(c, generated.DomainDeleteJSONRequestBody{DomainId: id})
		return removeErr
	}, func(c context.Context) (string, error) {
		read, readErr := api.DomainOneWithResponse(c, &generated.DomainOneParams{DomainId: id})
		if readErr != nil {
			return "", readErr
		}
		if read == nil || read.JSON200 == nil || read.JSON200.DomainId == nil {
			return "", nil
		}
		return *read.JSON200.DomainId, nil
	})
}

func finalizeGeneratedDomainCreateAttempt(t *testing.T, target string, keys []string, response *generated.DomainCreateResponse, requestErr error, registerCleanup func(string)) liveDomainCreateResult {
	t.Helper()
	status := 0
	if response != nil && response.HTTPResponse != nil {
		status = response.HTTPResponse.StatusCode
	}
	apiCode := ""
	var apiErr *client.APIError
	if errors.As(requestErr, &apiErr) {
		apiCode = apiErr.Code
		if status == 0 {
			status = apiErr.StatusCode
		}
	}
	created := false
	if response != nil && response.JSON200 != nil && response.JSON200.DomainId != nil && *response.JSON200.DomainId != "" {
		registerCleanup(*response.JSON200.DomainId)
		created = true
	}
	classification, classificationErr := classifyWorkloadCreateAttempt("domain", status, apiCode, keys, true, true)
	if classificationErr != nil {
		t.Fatalf("domain classification unavailable")
	}
	return liveDomainCreateResult{path: "generated", target: target, classification: classification, keys: keys, reason: sanitizeDomainValidationReason(requestErr), created: created}
}

func mountCreateRequestKeys(inputs MountArgs) []string {
	keys := []string{"mountPath", "serviceId", "serviceType", "type"}
	switch inputs.Type {
	case mountTypeBind:
		keys = append(keys, "hostPath")
	case mountTypeVolume:
		keys = append(keys, "volumeName")
	case mountTypeFile:
		keys = append(keys, "content", "filePath")
	}
	for _, target := range []struct {
		name string
		set  bool
	}{
		{name: "applicationId", set: inputs.ApplicationID != nil},
		{name: "composeId", set: inputs.ComposeID != nil},
		{name: "postgresId", set: inputs.PostgresID != nil},
		{name: "mysqlId", set: inputs.MySQLID != nil},
		{name: "mariadbId", set: inputs.MariaDBID != nil},
		{name: "redisId", set: inputs.RedisID != nil},
	} {
		if target.set {
			keys = append(keys, target.name)
		}
	}
	return keys
}

func composeDiffInputs(base ComposeArgs) (ComposeArgs, ComposeArgs) {
	sourceReplacement := base
	sourceReplacement.Source = ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main"}}
	environmentReplacement := base
	environmentReplacement.Environment = stringPtr("REPLACED=1")
	return sourceReplacement, environmentReplacement
}

func liveRegistryApplicationSource() (source ApplicationSource) {
	url, user, password := os.Getenv("DOKPLOY_REGISTRY_URL"), os.Getenv("DOKPLOY_REGISTRY_USERNAME"), os.Getenv("DOKPLOY_REGISTRY_PASSWORD")
	if url == "" || user == "" || password == "" {
		return ApplicationSource{}
	}
	return ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27", RegistryURL: &url, Username: &user, Password: &password}}
}

func registerLiveApplicationSourceMetadata(source ApplicationSource) func() {
	values := []string{}
	add := func(value string) {
		if value != "" {
			values = append(values, value)
		}
	}
	addBuild := func(build ApplicationBuild) {
		add(string(build.Type))
		if build.Dockerfile != nil {
			add(*build.Dockerfile)
		}
		if build.DockerContextPath != nil {
			add(*build.DockerContextPath)
		}
		if build.DockerBuildStage != nil {
			add(*build.DockerBuildStage)
		}
	}
	switch source.Type {
	case SourceGit:
		add(source.Git.URL)
		add(source.Git.Branch)
		if source.Git.BuildPath != nil {
			add(*source.Git.BuildPath)
		}
		if source.Git.SSHKeyID != nil {
			add(*source.Git.SSHKeyID)
		}
		for _, path := range source.Git.WatchPaths {
			add(path)
		}
		addBuild(source.Git.Build)
	case SourceDocker:
		add(source.Docker.Image)
		if source.Docker.RegistryURL != nil {
			add(*source.Docker.RegistryURL)
		}
		if source.Docker.Username != nil {
			add(*source.Docker.Username)
		}
		if source.Docker.Password != nil {
			add(*source.Docker.Password)
		}
	case SourceGitLab:
		add(source.GitLab.IntegrationID)
		add(strconv.Itoa(source.GitLab.ProjectID))
		add(source.GitLab.Owner)
		add(source.GitLab.Namespace)
		add(source.GitLab.Repository)
		add(source.GitLab.Branch)
		if source.GitLab.BuildPath != nil {
			add(*source.GitLab.BuildPath)
		}
		for _, path := range source.GitLab.WatchPaths {
			add(path)
		}
		addBuild(source.GitLab.Build)
	}
	return registerLiveSecrets(values...)
}

func registerLiveComposeSourceMetadata(source ComposeSource) func() {
	values := []string{}
	add := func(value string) {
		if value != "" {
			values = append(values, value)
		}
	}
	switch source.Type {
	case ComposeSourceGit:
		add(source.Git.URL)
		add(source.Git.Branch)
		add(source.Git.ComposePath)
		if source.Git.SSHKeyID != nil {
			add(*source.Git.SSHKeyID)
		}
		for _, path := range source.Git.WatchPaths {
			add(path)
		}
	case ComposeSourceGitLab:
		add(source.GitLab.IntegrationID)
		add(strconv.Itoa(source.GitLab.ProjectID))
		add(source.GitLab.Owner)
		add(source.GitLab.Namespace)
		add(source.GitLab.Repository)
		add(source.GitLab.Branch)
		add(source.GitLab.ComposePath)
		for _, path := range source.GitLab.WatchPaths {
			add(path)
		}
	}
	return registerLiveSecrets(values...)
}

func liveGitLabApplicationSource() (source ApplicationSource) {
	id, project, owner, namespace, repository, branch := os.Getenv("DOKPLOY_GITLAB_INTEGRATION_ID"), os.Getenv("DOKPLOY_GITLAB_PROJECT_ID"), os.Getenv("DOKPLOY_GITLAB_OWNER"), os.Getenv("DOKPLOY_GITLAB_NAMESPACE"), os.Getenv("DOKPLOY_GITLAB_REPOSITORY"), os.Getenv("DOKPLOY_GITLAB_BRANCH")
	projectID, err := strconv.Atoi(project)
	if id == "" || err != nil || projectID == 0 || owner == "" || namespace == "" || repository == "" || branch == "" {
		return ApplicationSource{}
	}
	return ApplicationSource{Type: SourceGitLab, GitLab: &GitLabAppSource{IntegrationID: id, ProjectID: projectID, Owner: owner, Namespace: namespace, Repository: repository, Branch: branch, Build: ApplicationBuild{Type: BuildNixpacks}}}
}

func gitLabSourceValues() (string, int, string, string, string, string, bool) {
	id, project, owner, namespace, repository, branch := os.Getenv("DOKPLOY_GITLAB_INTEGRATION_ID"), os.Getenv("DOKPLOY_GITLAB_PROJECT_ID"), os.Getenv("DOKPLOY_GITLAB_OWNER"), os.Getenv("DOKPLOY_GITLAB_NAMESPACE"), os.Getenv("DOKPLOY_GITLAB_REPOSITORY"), os.Getenv("DOKPLOY_GITLAB_BRANCH")
	projectID, err := strconv.Atoi(project)
	ok := id != "" && err == nil && projectID != 0 && owner != "" && namespace != "" && repository != "" && branch != ""
	return id, projectID, owner, namespace, repository, branch, ok
}

func gitLabComposeSource() ComposeSource {
	id, projectID, owner, namespace, repository, branch, ok := gitLabSourceValues()
	if !ok {
		return ComposeSource{}
	}
	return ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: id, ProjectID: projectID, Owner: owner, Namespace: namespace, Repository: repository, Branch: branch, ComposePath: defaultComposePath}}
}

func validateLiveTarget(ctx context.Context, operation string, keys []string, readiness liveTargetReadiness) (string, error) {
	present, ready, err := readiness(ctx)
	if err != nil {
		return "", err
	}
	if present && ready {
		return "", nil
	}
	return classifyWorkloadCreateAttempt(operation, 0, "", keys, present, ready)
}

func validateLiveTargetWithHealthProbe(ctx context.Context, operation string, keys []string, readiness liveTargetReadiness, probe func(context.Context) error) (string, error) {
	classification, err := validateLiveTarget(ctx, operation, keys, readiness)
	if err != nil {
		if probeErr := maybeVerifyLiveServerHealth(ctx, probe); probeErr != nil {
			var phaseErr *mountDispatchHealthProbeError
			if errors.As(probeErr, &phaseErr) {
				recordMountDispatchHealthFailure(probeErr)
			} else {
				recordServerHealthFailure(operation+"-target-read", probeErr)
			}
		}
	}
	return classification, err
}

func mountReplacement(input MountArgs, currentType string) MountArgs {
	input.HostPath, input.VolumeName, input.FilePath, input.Content = nil, nil, nil, nil
	switch currentType {
	case mountTypeBind:
		input.Type, input.VolumeName = mountTypeVolume, stringPtr(liveRunName("replacement-volume"))
	case mountTypeVolume:
		input.Type, input.FilePath, input.Content = mountTypeFile, stringPtr("/etc/pulumi-replacement.conf"), stringPtr("REPLACEMENT=1")
	default:
		input.Type, input.HostPath = mountTypeBind, stringPtr(filepath.Join("/tmp", liveRunName("replacement-bind")))
	}
	return input
}

func runLiveMountLifecycle(t *testing.T, ctx context.Context, api *client.Client, inputs, targetReplacement MountArgs, readiness liveTargetReadiness, dispatch ...bool) {
	t.Helper()
	isDispatch := len(dispatch) > 0 && dispatch[0]
	r := Mount{client: fixedClient(api)}
	healthProbe := liveServerHealthProbe(api)
	if isDispatch {
		healthProbe = mountDispatchHealthProbe(api)
	}
	t.Cleanup(registerLiveSecrets(value(inputs.Content), value(inputs.HostPath), value(inputs.VolumeName)))
	var targetPresent, targetReady bool
	var err error
	classification := ""
	classification, err = validateLiveTargetWithHealthProbe(ctx, "mount", mountCreateRequestKeys(inputs), func(ctx context.Context) (bool, bool, error) {
		targetPresent, targetReady, err = readiness(ctx)
		return targetPresent, targetReady, err
	}, healthProbe)
	if err != nil {
		if isDispatch {
			requireMountDispatchPhaseNoError(t, "target-read", err)
		}
		requireWorkloadLifecycleNoError(t, "mount", err)
	}
	if classification != "" {
		t.Fatalf("workload target unavailable: %s", classification)
	}
	createLease := beginLiveHeavyOperation(t, "mount-create", healthProbe)
	created, err := r.Create(ctx, infer.CreateRequest[MountArgs]{Inputs: inputs})
	// Ownership is registered before the create result is asserted so a partial
	// create cannot outlive this subtest.
	processErr := processLiveHeavyOperationError(t, createLease, err, func() {
		cleanupAfterCreateError(t, "mount", created.ID, err, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
			return v.ID, e
		})
	}, healthProbe)
	if isDispatch {
		requireMountDispatchPhaseNoError(t, "mount-create", processErr)
	}
	release := func() {}
	if processErr == nil {
		release = registerLiveCleanup(t, "mount", created.ID, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
			return v.ID, e
		})
	}
	requireWorkloadCreateNoError(t, "mount", processErr, mountCreateRequestKeys(inputs), targetPresent, targetReady, "Mount")
	createLease.releaseIfNeeded(t)

	read, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: created.Output})
	requireWorkloadLifecycleNoError(t, "mount", err)
	updated := read.Inputs
	updated.MountPath += "-updated"
	updateLease := beginLiveHeavyOperation(t, "mount-update", healthProbe)
	changed, err := r.Update(ctx, infer.UpdateRequest[MountArgs, MountState]{ID: created.ID, Inputs: updated, State: read.State})
	updateErr := processLiveHeavyOperationError(t, updateLease, err, nil, healthProbe)
	if isDispatch {
		requireMountDispatchPhaseNoError(t, "mount-update", updateErr)
	}
	requireWorkloadLifecycleNoError(t, "mount", updateErr)
	updateLease.releaseIfNeeded(t)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: changed.Output})
	requireWorkloadLifecycleNoError(t, "mount", err)
	requireLiveEqual(t, "mount.mountPath", updated.MountPath, postUpdate.Inputs.MountPath)
	requireLiveEqual(t, "mount.type", updated.Type, postUpdate.Inputs.Type)
	switch updated.Type {
	case mountTypeBind:
		requireLiveEqual(t, "mount.hostPath", updated.HostPath, postUpdate.Inputs.HostPath)
	case mountTypeVolume:
		requireLiveEqual(t, "mount.volumeName", updated.VolumeName, postUpdate.Inputs.VolumeName)
	case mountTypeFile:
		requireLiveEqual(t, "mount.filePath", updated.FilePath, postUpdate.Inputs.FilePath)
		requireLiveEqual(t, "mount.content", updated.Content, postUpdate.Inputs.Content)
	}
	imported, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
	requireWorkloadLifecycleNoError(t, "mount", err)
	requireLiveEqual(t, "mount.mountId", created.ID, imported.State.MountID)

	typeReplacement := mountReplacement(postUpdate.Inputs, postUpdate.Inputs.Type)
	diff, err := r.Diff(ctx, infer.DiffRequest[MountArgs, MountState]{ID: created.ID, Inputs: typeReplacement, State: postUpdate.State})
	requireWorkloadLifecycleNoError(t, "mount", err)
	requireLiveDiffKind(t, diff.DetailedDiff, "type", p.UpdateReplace)
	diff, err = r.Diff(ctx, infer.DiffRequest[MountArgs, MountState]{ID: created.ID, Inputs: targetReplacement, State: postUpdate.State})
	requireWorkloadLifecycleNoError(t, "mount", err)
	targetField := "applicationId"
	if targetReplacement.ComposeID != nil {
		targetField = "composeId"
	} else if targetReplacement.PostgresID != nil {
		targetField = "postgresId"
	}
	requireLiveDiffKind(t, diff.DetailedDiff, targetField, p.UpdateReplace)

	err = deleteAndVerifyLiveOwned(ctx, func() error {
		_, e := r.Delete(ctx, infer.DeleteRequest[MountState]{ID: created.ID, State: imported.State})
		return e
	}, func() (string, error) {
		gone, e := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
		return gone.ID, e
	}, release)
	if isDispatch {
		requireMountDispatchPhaseNoError(t, "mount-delete", err)
	}
	requireWorkloadLifecycleNoError(t, "mount", err)
	gone, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
	requireWorkloadLifecycleNoError(t, "mount", err)
	requireLiveEqual(t, "mount.id after delete", "", gone.ID)
}

func targetIDField(serviceType string) string {
	if serviceType == "compose" {
		return "composeId"
	}
	return serviceType + "Id"
}

func mountTargetID(args MountArgs, serviceType string) string {
	switch serviceType {
	case "compose":
		return value(args.ComposeID)
	case "postgres":
		return value(args.PostgresID)
	case "mysql":
		return value(args.MySQLID)
	case "mariadb":
		return value(args.MariaDBID)
	case "redis":
		return value(args.RedisID)
	default:
		return value(args.ApplicationID)
	}
}

type liveDispatchFixture struct {
	id        string
	api       *client.Client
	lease     *liveHeavyOperationLease
	remove    func(context.Context) error
	readID    func(context.Context) (string, error)
	readiness liveTargetReadiness
	cleaned   bool
}

func (fixture *liveDispatchFixture) releaseForDependentMount(t *testing.T) {
	t.Helper()
	fixture.lease.releaseIfNeeded(t)
}

func (fixture *liveDispatchFixture) cleanup(t *testing.T) {
	t.Helper()
	if fixture.cleaned {
		return
	}
	if fixture.id == "" {
		fixture.lease.releaseIfNeeded(t)
		fixture.cleaned = true
		return
	}
	if fixture.lease.released {
		fixture.lease = beginLiveHeavyOperation(t, "mount-dispatch-cleanup", mountDispatchHealthProbe(fixture.api))
	}
	if finishDatabaseCleanup(t, fixture.lease, "mount-dispatch", fixture.id, fixture.remove, fixture.readID) {
		fixture.cleaned = true
	} else {
		t.Errorf("%s", classifyMountDispatchPhase("fixture-delete", errLiveCleanup))
	}
}

func handleDispatchFixtureCreateError(t *testing.T, api *client.Client, lease *liveHeavyOperationLease, id string, createErr error, cleanup func()) {
	t.Helper()
	if err := processLiveHeavyCreateError(t, lease, id, createErr, cleanup, mountDispatchHealthProbe(api)); err != nil {
		requireMountDispatchPhaseNoError(t, "fixture-create", err)
	}
}

func createDispatchDatabase(t *testing.T, ctx context.Context, api *client.Client, environmentID, kind string) *liveDispatchFixture {
	t.Helper()
	lease := beginLiveHeavyOperation(t, "mount-dispatch-"+kind, mountDispatchHealthProbe(api))
	switch kind {
	case "postgres":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (Postgres{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{Name: liveRunName("postgres"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18"}})
		remove := func(c context.Context) error {
			_, e := (Postgres{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[PostgresState]{ID: created.ID})
			return e
		}
		fixture := newPostgresDispatchFixture(api, created.ID, lease, remove)
		handleDispatchFixtureCreateError(t, api, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "mysql":
		root := "live-test-root-password"
		t.Cleanup(registerLiveSecrets("live-test-password", root))
		created, err := (MySQL{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MySQLArgs]{Inputs: MySQLArgs{Name: liveRunName("mysql"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DatabaseRootPassword: &root, DockerImage: "mysql:8"}})
		remove := func(c context.Context) error {
			_, e := (MySQL{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[MySQLState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, api: api, lease: lease, remove: remove, readiness: mysqlTargetReadiness(api, created.ID), readID: func(c context.Context) (string, error) {
			v, e := (MySQL{client: fixedClient(api)}).Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: created.ID})
			return v.ID, e
		}}
		handleDispatchFixtureCreateError(t, api, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "mariadb":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (MariaDB{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: liveRunName("mariadb"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mariadb:11"}})
		remove := func(c context.Context) error {
			_, e := (MariaDB{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[MariaDBState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, api: api, lease: lease, remove: remove, readiness: mariadbTargetReadiness(api, created.ID), readID: func(c context.Context) (string, error) {
			v, e := (MariaDB{client: fixedClient(api)}).Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: created.ID})
			return v.ID, e
		}}
		handleDispatchFixtureCreateError(t, api, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "redis":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (Redis{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[RedisArgs]{Inputs: RedisArgs{Name: liveRunName("redis"), EnvironmentID: environmentID, DatabasePassword: "live-test-password", DockerImage: "redis:8"}})
		remove := func(c context.Context) error {
			_, e := (Redis{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[RedisState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, api: api, lease: lease, remove: remove, readiness: redisTargetReadiness(api, created.ID), readID: func(c context.Context) (string, error) {
			v, e := (Redis{client: fixedClient(api)}).Read(c, infer.ReadRequest[RedisArgs, RedisState]{ID: created.ID})
			return v.ID, e
		}}
		handleDispatchFixtureCreateError(t, api, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	default:
		t.Fatalf("unsupported dispatch database %q", kind)
		return &liveDispatchFixture{lease: lease}
	}
}

func newPostgresDispatchFixture(api *client.Client, id string, lease *liveHeavyOperationLease, remove func(context.Context) error) *liveDispatchFixture {
	return &liveDispatchFixture{
		id: id, api: api, lease: lease, remove: remove, readiness: postgresTargetReadiness(api, id),
		readID: func(c context.Context) (string, error) {
			v, e := (Postgres{client: fixedClient(api)}).Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
			return v.ID, e
		},
	}
}

func cleanupAfterCreateError(t *testing.T, kind, id string, createErr error, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	if !cleanupAfterCreateErrorNeedsImmediateCleanup(id, createErr) {
		return
	}
	// A provider create may return a partial ID. Clean it now, before
	// asserting the create error, rather than leaving a live workload
	// behind while the test continues. Successful workloads are owned by
	// the parent test and deleted after all dependent checks complete.
	liveCleanupVerified(t, kind, id, remove, read)
}

func deleteAndReadApplication(t *testing.T, ctx context.Context, r Application, ownedID *string) {
	id := *ownedID
	err := deleteAndVerifyOnce(func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[ApplicationState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
		return read.ID, err
	}, func() { *ownedID = "" })
	requireNoError(t, err)
}

func deleteAndReadCompose(t *testing.T, ctx context.Context, r Compose, ownedID *string) {
	id := *ownedID
	err := deleteAndVerifyOnce(func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[ComposeState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
		return read.ID, err
	}, func() { *ownedID = "" })
	requireNoError(t, err)
}

func TestLiveDomainContractExperiments(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 30*time.Minute)
	_, environmentID, releaseProject := liveProject(t, ctx, api)
	t.Cleanup(releaseProject)
	runDomainContractExperimentTargets(t, []string{"application", "compose"}, func(name string) {
		compose := name == "compose"
		runFocusedDomainTarget(t, func() (focusedDomainTarget, func()) {
			return createFocusedDomainTarget(t, ctx, api, environmentID, compose)
		}, func(target focusedDomainTarget) {
			args := DomainArgs{Host: liveDomainHost("experiment-domain"), Port: intPtr(80), CertificateType: CertificateNone, Enabled: true}
			if target.compose {
				args.ComposeID, args.ServiceName = &target.id, stringPtr("web")
			} else {
				args.ApplicationID = &target.id
			}
			baseline := domainCreateBody(args)
			runDomainContractExperimentSequence(t, baseline, domainContractExperimentCases(args, target.compose), func(field string, body generated.DomainCreateJSONRequestBody) domainExperimentResult {
				result := runDomainContractExperiment(t, ctx, api, target.name, field, body)
				t.Logf("%s", result)
				return result
			})
		})
	})
}

func runDomainContractExperiment(t *testing.T, ctx context.Context, api *client.Client, target, field string, body generated.DomainCreateJSONRequestBody) domainExperimentResult {
	t.Helper()
	keys, err := domainCreateRequestKeysFromBody(body)
	requireNoError(t, err)
	response, requestErr := api.DomainCreateWithResponse(ctx, body)
	status := 0
	if response != nil && response.HTTPResponse != nil {
		status = response.HTTPResponse.StatusCode
	}
	code := ""
	var apiErr *client.APIError
	if errors.As(requestErr, &apiErr) {
		code = apiErr.Code
		if status == 0 {
			status = apiErr.StatusCode
		}
	}
	classification, classificationErr := classifyWorkloadCreateAttempt("domain", status, code, keys, true, true)
	requireNoError(t, classificationErr)
	statusClass, safeCode := domainResultStatus(classification)
	hasID := response != nil && response.JSON200 != nil && response.JSON200.DomainId != nil && *response.JSON200.DomainId != ""
	category, _ := classifyDomainExperimentResult(statusClass, hasID)
	if category == "environment" {
		recordServerHealthFailure("domain-experiment", errLiveServerHealthProbe)
	}
	if category == "cleanup-failure" {
		recordCleanupResult("domain", errDomainExperimentMissingID)
		return domainExperimentResult{field: field, status: statusClass, code: safeCode, category: category}
	}
	if hasID {
		id := *response.JSON200.DomainId
		cleanupCtx, cancel := cleanupContext()
		cleanupErr := verifyLiveCleanup(cleanupCtx, func(c context.Context) error {
			_, e := api.DomainDeleteWithResponse(c, generated.DomainDeleteJSONRequestBody{DomainId: id})
			return e
		}, func(c context.Context) (string, error) {
			read, e := api.DomainOneWithResponse(c, &generated.DomainOneParams{DomainId: id})
			if e != nil || read == nil || read.JSON200 == nil || read.JSON200.DomainId == nil {
				return "", e
			}
			return *read.JSON200.DomainId, nil
		})
		cancel()
		if cleanupErr != nil {
			reportLiveCleanup(t, "domain", "experiment", cleanupErr)
			category = "cleanup-failure"
		}
	}
	return domainExperimentResult{field: field, status: statusClass, code: safeCode, category: category}
}

func TestLiveDomainFocused(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 30*time.Minute)
	_, environmentID, releaseProject := liveProject(t, ctx, api)
	t.Cleanup(releaseProject)
	for _, compose := range []bool{false, true} {
		compose := compose
		name := "application"
		if compose {
			name = "compose"
		}
		t.Run(name, func(t *testing.T) {
			runFocusedLiveDomainTarget(t, ctx, api, environmentID, compose)
		})
	}
}

func createFocusedDomainTarget(t *testing.T, ctx context.Context, api *client.Client, environmentID string, compose bool) (focusedDomainTarget, func()) {
	t.Helper()
	if compose {
		r := Compose{client: fixedClient(api)}
		inputs := ComposeArgs{Name: liveRunName("experiment-compose"), EnvironmentID: environmentID, Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}}}
		lease := beginLiveHeavyOperation(t, "experiment-compose-create", liveServerHealthProbe(api))
		created, err := r.Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: inputs})
		processErr := processLiveHeavyOperationError(t, lease, err, func() {
			cleanupAfterCreateError(t, "compose", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
				return v.ID, e
			})
		}, liveServerHealthProbe(api))
		requireNoError(t, processErr)
		lease.releaseIfNeeded(t)
		requireLiveEqual(t, "compose.status", statusDone, created.Output.Status)
		return focusedDomainTarget{id: created.ID, name: "compose", compose: true}, func() {
			liveCleanupVerified(t, "compose", created.ID, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
				return v.ID, e
			})
		}
	}
	r := Application{client: fixedClient(api)}
	inputs := ApplicationArgs{Name: liveRunName("experiment-application"), EnvironmentID: environmentID, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}}}
	lease := beginLiveHeavyOperation(t, "experiment-application-create", liveServerHealthProbe(api))
	created, err := r.Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: inputs})
	processErr := processLiveHeavyOperationError(t, lease, err, func() {
		cleanupAfterCreateError(t, "application", created.ID, err, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, processErr)
	lease.releaseIfNeeded(t)
	requireLiveEqual(t, "application.status", statusDone, created.Output.Status)
	return focusedDomainTarget{id: created.ID, name: "application"}, func() {
		liveCleanupVerified(t, "application", created.ID, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
			return v.ID, e
		})
	}
}

func runFocusedLiveDomainTarget(t *testing.T, ctx context.Context, api *client.Client, environmentID string, compose bool) {
	t.Helper()
	runFocusedDomainTarget(t, func() (focusedDomainTarget, func()) {
		if compose {
			r := Compose{client: fixedClient(api)}
			inputs := ComposeArgs{Name: liveRunName("focused-compose"), EnvironmentID: environmentID, Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}}}
			lease := beginLiveHeavyOperation(t, "focused-compose-create", liveServerHealthProbe(api))
			created, err := r.Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: inputs})
			processErr := processLiveHeavyOperationError(t, lease, err, func() {
				cleanupAfterCreateError(t, "compose", created.ID, err, func(c context.Context) error {
					_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
					return e
				}, func(c context.Context) (string, error) {
					v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
					return v.ID, e
				})
			}, liveServerHealthProbe(api))
			requireNoError(t, processErr)
			lease.releaseIfNeeded(t)
			requireLiveEqual(t, "compose.status", statusDone, created.Output.Status)
			return focusedDomainTarget{id: created.ID, name: "compose", compose: true}, func() {
				liveCleanupVerified(t, "compose", created.ID, func(c context.Context) error {
					_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
					return e
				}, func(c context.Context) (string, error) {
					v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
					return v.ID, e
				})
			}
		}

		r := Application{client: fixedClient(api)}
		inputs := ApplicationArgs{Name: liveRunName("focused-application"), EnvironmentID: environmentID, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}}}
		lease := beginLiveHeavyOperation(t, "focused-application-create", liveServerHealthProbe(api))
		created, err := r.Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: inputs})
		processErr := processLiveHeavyOperationError(t, lease, err, func() {
			cleanupAfterCreateError(t, "application", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
				return v.ID, e
			})
		}, liveServerHealthProbe(api))
		requireNoError(t, processErr)
		lease.releaseIfNeeded(t)
		requireLiveEqual(t, "application.status", statusDone, created.Output.Status)
		return focusedDomainTarget{id: created.ID, name: "application"}, func() {
			liveCleanupVerified(t, "application", created.ID, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
				return v.ID, e
			})
		}
	}, func(target focusedDomainTarget) {
		readiness := applicationTargetReadiness(api, target.id)
		if target.compose {
			readiness = composeTargetReadiness(api, target.id)
		}
		present, ready, err := readiness(ctx)
		requireLiveLifecycleNoError(t, "Domain", "read ready target", err)
		requireLiveEqual(t, "domain.target.present", true, present)
		requireLiveEqual(t, "domain.target.ready", true, ready)

		args := DomainArgs{Host: liveDomainHost("focused-domain"), Port: intPtr(80), CertificateType: CertificateNone, Enabled: true}
		if target.compose {
			args.ComposeID, args.ServiceName = &target.id, stringPtr("web")
		} else {
			args.ApplicationID = &target.id
		}
		r := Domain{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[DomainArgs]{Inputs: args})
		if err != nil {
			providerKeys, keysErr := domainCreateRequestKeysFromBody(domainCreateBody(args))
			requireNoError(t, keysErr)
			providerClassification, classificationErr := classifyWorkloadCreateError("domain", err, providerKeys, present, ready)
			requireNoError(t, classificationErr)
			providerResult := liveDomainCreateResult{path: "provider", target: target.name, classification: providerClassification, keys: providerKeys, reason: sanitizeDomainValidationReason(err)}
			directArgs := args
			directArgs.Host = liveDomainHost("focused-domain-direct")
			generatedResult := runGeneratedDomainCreateAttempt(t, ctx, api, target.name, directArgs)
			evidence := formatDomainComparisonEvidence(providerResult, generatedResult)
			t.Logf("domain comparison evidence: %s", evidence)
			recordLiveOutcome("domain-comparison", evidence)
			cleanupAfterCreateError(t, "domain", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: created.ID})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
				return v.ID, e
			})
		}
		requireWorkloadCreateNoError(t, "domain", err, domainCreateRequestKeys(target.compose), present, ready, "Domain/"+target.name)
		domainID := created.ID
		domainOwner := registerLiveCleanup(t, "domain", domainID, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[DomainState]{ID: domainID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[DomainArgs, DomainState]{ID: domainID})
			return v.ID, e
		})
		read, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: domainID, State: created.Output})
		requireWorkloadLifecycleNoError(t, "domain", err)
		updated := read.Inputs
		updated.Host = liveDomainHost("focused-domain-updated")
		updated.HTTPS = true
		changed, err := r.Update(ctx, infer.UpdateRequest[DomainArgs, DomainState]{ID: domainID, Inputs: updated, State: read.State})
		requireWorkloadLifecycleNoError(t, "domain", err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: domainID, State: changed.Output})
		requireWorkloadLifecycleNoError(t, "domain", err)
		requireLiveEqual(t, "domain.host", updated.Host, postUpdate.Inputs.Host)
		requireLiveEqual(t, "domain.https", updated.HTTPS, postUpdate.Inputs.HTTPS)
		imported, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: domainID})
		requireWorkloadLifecycleNoError(t, "domain", err)
		requireLiveEqual(t, "domain.import.id", domainID, imported.State.DomainID)
		domainOwner()
	})
}
