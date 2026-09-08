package dokploy

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// TestLiveTier2Workloads is intentionally one serial test. Workload creates
// deploy containers and mounts cause another deploy, so parallel subtests
// would needlessly increase load on the acceptance server.
func TestLiveTier2Workloads(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 30*time.Minute)
	projectID, environmentID := liveProject(t, ctx, api)
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
		created, err := r.Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: inputs})
		cleanupAfterCreateError(t, "application", created.ID, err, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
			return v.ID, e
		})
		requireNoError(t, err)
		require.NotEmpty(t, created.ID)
		applicationID = created.ID
		applicationStatus = created.Output.Status
		require.Equal(t, statusDone, created.Output.Status)

		read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID, State: created.Output})
		requireNoError(t, err)
		require.Equal(t, SourceDocker, read.Inputs.Source.Type)
		require.Equal(t, "nginx:1.27", read.Inputs.Source.Docker.Image)
		updated := read.Inputs
		updated.Name += "-updated"
		updatedDescription := "updated by live workload test"
		updated.Description = &updatedDescription
		changed, err := r.Update(ctx, infer.UpdateRequest[ApplicationArgs, ApplicationState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID, State: changed.Output})
		requireNoError(t, err)
		require.Equal(t, updated.Name, postUpdate.Inputs.Name)
		require.Equal(t, updatedDescription, value(postUpdate.Inputs.Description))
		imported, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.ApplicationID)
		require.Equal(t, postUpdate.Inputs.Name, imported.Inputs.Name)
		require.Equal(t, postUpdate.Inputs.EnvironmentID, imported.Inputs.EnvironmentID)
		require.Equal(t, SourceDocker, imported.Inputs.Source.Type)
		require.NotNil(t, imported.Inputs.Source.Docker)
		require.Equal(t, "nginx:1.27", imported.Inputs.Source.Docker.Image)

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
			require.Equal(t, p.Update, changedDiff.DetailedDiff[change.field].Kind)
		}
		replacement := postUpdate.Inputs
		replacement.Source = ApplicationSource{Type: SourceGit, Git: &GitApplicationSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main", Build: ApplicationBuild{Type: BuildNixpacks}}}
		diff, err = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: replacement, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
		environmentReplacement := postUpdate.Inputs
		environmentReplacement.Environment = stringPtr("REPLACED=1")
		diff, err = r.Diff(ctx, infer.DiffRequest[ApplicationArgs, ApplicationState]{Inputs: environmentReplacement, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.Update, diff.DetailedDiff["environment"].Kind)

	})

	t.Run("Compose", func(t *testing.T) {
		inputs := ComposeArgs{Name: liveRunName("compose"), EnvironmentID: environmentID, Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}}}
		t.Cleanup(registerLiveSecrets(value(inputs.Environment), "COMPOSE_ENV=1"))
		r := Compose{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: inputs})
		cleanupAfterCreateError(t, "compose", created.ID, err, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
			return v.ID, e
		})
		requireNoError(t, err)
		require.NotEmpty(t, created.ID)
		composeID = created.ID
		composeStatus = created.Output.Status
		require.Equal(t, statusDone, created.Output.Status)

		read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID, State: created.Output})
		requireNoError(t, err)
		require.Equal(t, ComposeSourceRaw, read.Inputs.Source.Type)
		updated := read.Inputs
		updated.Name += "-updated"
		updatedDescription := "updated by live workload test"
		updated.Description = &updatedDescription
		changed, err := r.Update(ctx, infer.UpdateRequest[ComposeArgs, ComposeState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID, State: changed.Output})
		requireNoError(t, err)
		require.Equal(t, updated.Name, postUpdate.Inputs.Name)
		imported, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.ComposeID)
		require.Equal(t, postUpdate.Inputs.Name, imported.Inputs.Name)
		require.Equal(t, postUpdate.Inputs.EnvironmentID, imported.Inputs.EnvironmentID)
		require.Equal(t, ComposeSourceRaw, imported.Inputs.Source.Type)
		require.NotNil(t, imported.Inputs.Source.Raw)
		require.Contains(t, imported.Inputs.Source.Raw.ComposeFile, "image: nginx:1.27")
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
			require.Equal(t, change.kind, changedDiff.DetailedDiff[change.field].Kind)
		}
		diff, err := r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		sourceReplacement, environmentReplacement := composeDiffInputs(postUpdate.Inputs)
		diff, err = r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: sourceReplacement, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["source.type"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[ComposeArgs, ComposeState]{Inputs: environmentReplacement, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.Update, diff.DetailedDiff["environment"].Kind)
	})

	workloadTargets := []struct {
		name, id, status string
		compose          bool
	}{{"application", applicationID, applicationStatus, false}, {"compose", composeID, composeStatus, true}}
	for _, target := range workloadTargets {
		target := target
		t.Run("Domain/"+target.name, func(t *testing.T) {
			if !workloadDependencyReady(target.id, target.status) {
				t.Skip("dependent workload target was not created successfully")
			}
			r := Domain{client: fixedClient(api)}
			args := DomainArgs{Host: liveRunName("domain") + ".example.invalid", Port: intPtr(80), CertificateType: CertificateNone, Enabled: true}
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
			if err != nil {
				classification, classificationErr := classifyWorkloadCreateError("domain", err, domainCreateRequestKeys(target.compose), true, true)
				require.NoError(t, classificationErr)
				recordLiveOutcome("Domain/"+target.name, classification)
			}
			requireNoError(t, err)
			read, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: created.Output})
			requireNoError(t, err)
			updated := read.Inputs
			updated.Host = liveRunName("updated-domain") + ".example.invalid"
			updated.Path, updated.InternalPath, updated.Port = stringPtr("/public"), stringPtr("/internal"), intPtr(8080)
			updated.Enabled = false
			updatedState, err := r.Update(ctx, infer.UpdateRequest[DomainArgs, DomainState]{ID: created.ID, Inputs: updated, State: read.State})
			requireNoError(t, err)
			postUpdate, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID, State: updatedState.Output})
			requireNoError(t, err)
			require.Equal(t, updated.Host, postUpdate.Inputs.Host)
			require.Equal(t, updated.Path, postUpdate.Inputs.Path)
			require.Equal(t, updated.InternalPath, postUpdate.Inputs.InternalPath)
			require.Equal(t, updated.Port, postUpdate.Inputs.Port)
			require.Equal(t, updated.Enabled, postUpdate.Inputs.Enabled)
			// Import-style reads must reconstruct state from the ID alone.
			imported, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
			requireNoError(t, err)
			require.Equal(t, updated.Host, imported.Inputs.Host)
			require.Equal(t, updated.Path, imported.Inputs.Path)
			require.Equal(t, updated.InternalPath, imported.Inputs.InternalPath)
			require.Equal(t, updated.Port, imported.Inputs.Port)
			require.Equal(t, updated.Enabled, imported.Inputs.Enabled)
			require.Equal(t, updated.CertificateType, imported.Inputs.CertificateType)
			if target.compose {
				require.Equal(t, target.id, value(imported.Inputs.ComposeID))
				require.Equal(t, "web", value(imported.Inputs.ServiceName))
			} else {
				require.Equal(t, target.id, value(imported.Inputs.ApplicationID))
			}
			_, err = r.Delete(ctx, infer.DeleteRequest[DomainState]{ID: created.ID})
			requireNoError(t, err)
			gone, err := r.Read(ctx, infer.ReadRequest[DomainArgs, DomainState]{ID: created.ID})
			requireNoError(t, err)
			require.Empty(t, gone.ID)
		})
	}

	for _, target := range workloadTargets {
		target := target
		t.Run("Mounts/"+target.name, func(t *testing.T) {
			if !workloadDependencyReady(target.id, target.status) {
				t.Skip("dependent workload target was not created successfully")
			}
			r := Mount{client: fixedClient(api)}
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
					t.Cleanup(registerLiveSecrets(value(inputs.Content), value(inputs.HostPath), value(inputs.VolumeName)))
					created, err := r.Create(ctx, infer.CreateRequest[MountArgs]{Inputs: inputs})
					cleanupAfterCreateError(t, "mount", created.ID, err, func(c context.Context) error {
						_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
						return e
					}, func(c context.Context) (string, error) {
						v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
						return v.ID, e
					})
					if err != nil {
						classification, classificationErr := classifyWorkloadCreateError("mount", err, mountCreateRequestKeys(inputs), true, true)
						require.NoError(t, classificationErr)
						recordLiveOutcome("Mount/"+target.name+"/"+inputs.Type, classification)
					}
					requireNoError(t, err)
					read, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: created.Output})
					requireNoError(t, err)
					updated := read.Inputs
					updated.MountPath += "-updated"
					changed, err := r.Update(ctx, infer.UpdateRequest[MountArgs, MountState]{ID: created.ID, Inputs: updated, State: read.State})
					requireNoError(t, err)
					postUpdate, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: changed.Output})
					requireNoError(t, err)
					require.Equal(t, updated.MountPath, postUpdate.Inputs.MountPath)
					require.Equal(t, updated.Type, postUpdate.Inputs.Type)
					switch updated.Type {
					case mountTypeBind:
						require.Equal(t, updated.HostPath, postUpdate.Inputs.HostPath)
					case mountTypeVolume:
						require.Equal(t, updated.VolumeName, postUpdate.Inputs.VolumeName)
					case mountTypeFile:
						require.Equal(t, updated.FilePath, postUpdate.Inputs.FilePath)
						requireLiveEqual(t, "mount.content", updated.Content, postUpdate.Inputs.Content)
					}
					imported, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
					requireNoError(t, err)
					require.Equal(t, created.ID, imported.State.MountID)
					replacement := mountReplacement(postUpdate.Inputs, inputs.Type)
					diff, err := r.Diff(ctx, infer.DiffRequest[MountArgs, MountState]{ID: created.ID, Inputs: replacement, State: postUpdate.State})
					requireNoError(t, err)
					require.Equal(t, p.UpdateReplace, diff.DetailedDiff["type"].Kind)
					targetReplacement := replacement
					if target.compose {
						targetReplacement.ApplicationID, targetReplacement.ComposeID = nil, &applicationID
					} else {
						targetReplacement.ApplicationID, targetReplacement.ComposeID = &composeID, nil
					}
					diff, err = r.Diff(ctx, infer.DiffRequest[MountArgs, MountState]{ID: created.ID, Inputs: targetReplacement, State: postUpdate.State})
					requireNoError(t, err)
					targetField := "applicationId"
					if target.compose {
						targetField = "composeId"
					}
					require.Equal(t, p.UpdateReplace, diff.DetailedDiff[targetField].Kind)
					_, err = r.Delete(ctx, infer.DeleteRequest[MountState]{ID: created.ID, State: imported.State})
					requireNoError(t, err)
					gone, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
					requireNoError(t, err)
					require.Empty(t, gone.ID)
				})
			}
		})
	}

	// Dispatch-only cases prove every supported target route without adding an
	// update/redeploy cycle to the matrix. Each database fixture is created and
	// removed before the next one is started.
	for _, target := range []string{"compose", "postgres", "mysql", "mariadb", "redis"} {
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
			serviceType := target
			mount := MountArgs{Type: mountTypeBind, MountPath: "/mnt/dispatch", HostPath: stringPtr(filepath.Join("/tmp", liveRunName("dispatch")))}
			t.Cleanup(registerLiveSecrets(value(mount.Content), value(mount.HostPath), value(mount.VolumeName)))
			setMountTarget(&mount, targetIDField(serviceType), targetID)
			r := Mount{client: fixedClient(api)}
			created, err := r.Create(ctx, infer.CreateRequest[MountArgs]{Inputs: mount})
			cleanupAfterCreateError(t, "mount", created.ID, err, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[MountState]{ID: created.ID, State: created.Output})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
				return v.ID, e
			})
			requireNoError(t, err)
			read, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: created.Output})
			requireNoError(t, err)
			imported, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID, State: read.State})
			requireNoError(t, err)
			require.Equal(t, targetID, mountTargetID(imported.Inputs, serviceType))
			_, err = r.Delete(ctx, infer.DeleteRequest[MountState]{ID: created.ID, State: imported.State})
			requireNoError(t, err)
			gone, err := r.Read(ctx, infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
			requireNoError(t, err)
			require.Empty(t, gone.ID)
			if target != "compose" {
				fixture.cleanup(t)
			}
		})
	}

	// Source variants are metadata-only checks. They deliberately configure a
	// bare resource and do not deploy or clone anything.
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
				body := generated.ApplicationCreateJSONRequestBody{Name: liveRunName("application-" + variant.name), EnvironmentId: environmentID}
				response, err := api.ApplicationCreateWithResponse(ctx, body)
				requireNoError(t, err)
				require.NotNil(t, response.JSON200)
				require.NotNil(t, response.JSON200.ApplicationId)
				id := *response.JSON200.ApplicationId
				cleanupDirectApplication(t, api, id)
				requireNoError(t, configureApplicationSource(ctx, api, id, variant.source))
				read, err := (Application{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
				requireNoError(t, err)
				require.Equal(t, variant.source.Type, read.Inputs.Source.Type)
			})
		}
		t.Run("compose-git", func(t *testing.T) {
			response, err := api.ComposeCreateWithResponse(ctx, generated.ComposeCreateJSONRequestBody{Name: liveRunName("compose-git"), EnvironmentId: environmentID, ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker))})
			requireNoError(t, err)
			require.NotNil(t, response.JSON200)
			require.NotNil(t, response.JSON200.ComposeId)
			id := *response.JSON200.ComposeId
			cleanupDirectCompose(t, api, id)
			source := ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main"}}
			requireNoError(t, configureComposeSource(ctx, api, id, source))
			requireNoError(t, fetchComposeSource(ctx, api, id, ComposeSourceGit))
			read, err := (Compose{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
			requireNoError(t, err)
			require.Equal(t, ComposeSourceGit, read.Inputs.Source.Type)
			require.NotNil(t, read.Inputs.Source.Git)
			require.Equal(t, source.Git.URL, read.Inputs.Source.Git.URL)
			require.Equal(t, source.Git.Branch, read.Inputs.Source.Git.Branch)
		})
		if composeGitLabSource.Type == ComposeSourceGitLab {
			t.Run("compose-gitlab", func(t *testing.T) {
				response, err := api.ComposeCreateWithResponse(ctx, generated.ComposeCreateJSONRequestBody{Name: liveRunName("compose-gitlab"), EnvironmentId: environmentID, ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker))})
				requireNoError(t, err)
				require.NotNil(t, response.JSON200)
				require.NotNil(t, response.JSON200.ComposeId)
				id := *response.JSON200.ComposeId
				t.Cleanup(func() {
					r := Compose{client: fixedClient(api)}
					liveCleanupVerified(t, "compose", id, func(ctx context.Context) error {
						_, err := r.Delete(ctx, infer.DeleteRequest[ComposeState]{ID: id})
						return err
					}, func(ctx context.Context) (string, error) {
						read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
						return read.ID, err
					})
				})
				requireNoError(t, configureComposeSource(ctx, api, id, composeGitLabSource))
				requireNoError(t, fetchComposeSource(ctx, api, id, ComposeSourceGitLab))
				read, err := (Compose{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
				requireNoError(t, err)
				require.Equal(t, ComposeSourceGitLab, read.Inputs.Source.Type)
			})
		} else {
			t.Run("compose-gitlab", func(t *testing.T) { t.Skip("dedicated GitLab source prerequisite variables are not configured") })
		}
	})

	// Keep both workloads alive through every dependent Domain/Mount and
	// metadata subtest; these are the final explicit lifecycle operations.
	if applicationID != "" {
		deleteAndReadApplication(t, ctx, Application{client: fixedClient(api)}, applicationID)
		applicationID = ""
	}
	if composeID != "" {
		deleteAndReadCompose(t, ctx, Compose{client: fixedClient(api)}, composeID)
		composeID = ""
	}
}

func domainCreateRequestKeys(compose bool) []string {
	keys := []string{"certificateType", "customCertResolver", "host", "https", "internalPath", "path", "port", "serviceName", "stripPath"}
	if compose {
		return append(keys, "composeId", "domainType")
	}
	return append(keys, "applicationId", "domainType")
}

func mountCreateRequestKeys(inputs MountArgs) []string {
	keys := []string{"mountPath", "serviceId", "serviceType", "type"}
	switch inputs.Type {
	case mountTypeBind:
		keys = append(keys, "hostPath")
	case mountTypeVolume:
		keys = append(keys, "volumeName")
	case mountTypeFile:
		keys = append(keys, "filePath", "content")
	}
	return keys
}

func workloadDependencyReady(id, status string) bool {
	return id != "" && status == statusDone
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
	return ComposeSource{Type: ComposeSourceGitLab, GitLab: &GitLabComposeSource{IntegrationID: id, ProjectID: projectID, Owner: owner, Namespace: namespace, Repository: repository, Branch: branch}}
}

func cleanupDirectApplication(t *testing.T, api *client.Client, id string) {
	t.Helper()
	t.Cleanup(func() {
		r := Application{client: fixedClient(api)}
		liveCleanupVerified(t, "application", id, func(ctx context.Context) error {
			_, err := r.Delete(ctx, infer.DeleteRequest[ApplicationState]{ID: id})
			return err
		}, func(ctx context.Context) (string, error) {
			read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
			return read.ID, err
		})
	})
}

func cleanupDirectCompose(t *testing.T, api *client.Client, id string) {
	t.Helper()
	t.Cleanup(func() {
		r := Compose{client: fixedClient(api)}
		liveCleanupVerified(t, "compose", id, func(ctx context.Context) error {
			_, err := r.Delete(ctx, infer.DeleteRequest[ComposeState]{ID: id})
			return err
		}, func(ctx context.Context) (string, error) {
			read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
			return read.ID, err
		})
	})
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
	id      string
	lease   *liveHeavyOperationLease
	remove  func(context.Context) error
	readID  func(context.Context) (string, error)
	cleaned bool
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
	if finishDatabaseCleanup(t, fixture.lease, "mount-dispatch", fixture.id, fixture.remove, fixture.readID) {
		fixture.cleaned = true
	}
}

func createDispatchDatabase(t *testing.T, ctx context.Context, api *client.Client, environmentID, kind string) *liveDispatchFixture {
	t.Helper()
	lease := beginLiveHeavyOperation(t, "mount-dispatch-"+kind)
	switch kind {
	case "postgres":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (Postgres{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{Name: liveRunName("postgres"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18"}})
		remove := func(c context.Context) error {
			_, e := (Postgres{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[PostgresState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, lease: lease, remove: remove, readID: func(c context.Context) (string, error) {
			v, e := (Postgres{client: fixedClient(api)}).Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: created.ID})
			return v.ID, e
		}}
		handleLiveHeavyCreateError(t, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "mysql":
		root := "live-test-root-password"
		t.Cleanup(registerLiveSecrets("live-test-password", root))
		created, err := (MySQL{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MySQLArgs]{Inputs: MySQLArgs{Name: liveRunName("mysql"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DatabaseRootPassword: &root, DockerImage: "mysql:8"}})
		remove := func(c context.Context) error {
			_, e := (MySQL{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[MySQLState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, lease: lease, remove: remove, readID: func(c context.Context) (string, error) {
			v, e := (MySQL{client: fixedClient(api)}).Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: created.ID})
			return v.ID, e
		}}
		handleLiveHeavyCreateError(t, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "mariadb":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (MariaDB{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: liveRunName("mariadb"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mariadb:11"}})
		remove := func(c context.Context) error {
			_, e := (MariaDB{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[MariaDBState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, lease: lease, remove: remove, readID: func(c context.Context) (string, error) {
			v, e := (MariaDB{client: fixedClient(api)}).Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: created.ID})
			return v.ID, e
		}}
		handleLiveHeavyCreateError(t, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	case "redis":
		t.Cleanup(registerLiveSecrets("live-test-password"))
		created, err := (Redis{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[RedisArgs]{Inputs: RedisArgs{Name: liveRunName("redis"), EnvironmentID: environmentID, DatabasePassword: "live-test-password", DockerImage: "redis:8"}})
		remove := func(c context.Context) error {
			_, e := (Redis{client: fixedClient(api)}).Delete(c, infer.DeleteRequest[RedisState]{ID: created.ID})
			return e
		}
		fixture := &liveDispatchFixture{id: created.ID, lease: lease, remove: remove, readID: func(c context.Context) (string, error) {
			v, e := (Redis{client: fixedClient(api)}).Read(c, infer.ReadRequest[RedisArgs, RedisState]{ID: created.ID})
			return v.ID, e
		}}
		handleLiveHeavyCreateError(t, lease, created.ID, err, func() { fixture.cleanup(t) })
		return fixture
	default:
		t.Fatalf("unsupported dispatch database %q", kind)
		return &liveDispatchFixture{lease: lease}
	}
}

func cleanupAfterCreateError(t *testing.T, kind, id string, createErr error, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	if id != "" {
		registerLiveCleanup(t, kind, id, remove, read)
		if createErr != nil {
			// A provider create may return a partial ID. Clean it now, before
			// asserting the create error, rather than leaving a live workload
			// behind while the test continues.
			liveCleanupVerified(t, kind, id, remove, read)
			return
		}
	}
}

func deleteAndReadApplication(t *testing.T, ctx context.Context, r Application, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[ApplicationState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: id})
	requireNoError(t, err)
	require.Empty(t, read.ID)
}

func deleteAndReadCompose(t *testing.T, ctx context.Context, r Compose, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[ComposeState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: id})
	requireNoError(t, err)
	require.Empty(t, read.ID)
}
