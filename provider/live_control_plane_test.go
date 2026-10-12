package dokploy

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func directEnvironmentUpdate(ctx context.Context, api *client.Client, environmentID, projectID, name string) error {
	_, err := api.EnvironmentUpdateWithResponse(ctx, generated.EnvironmentUpdateJSONRequestBody{
		EnvironmentId: environmentID,
		ProjectId:     &projectID,
		Name:          &name,
	})
	return err
}

func TestLiveRegistryUpdatedArgsRequiresCompleteAlternateConnection(t *testing.T) {
	base := RegistryArgs{Name: "registry", URL: "https://registry.example", Username: "user", Password: "password"}
	for name := range map[string]string{
		"url": "URL", "username": "USERNAME", "password": "PASSWORD", "prefix": "IMAGE_PREFIX",
	} {
		t.Setenv("DOKPLOY_REGISTRY_UPDATED_URL", "https://registry-updated.example")
		t.Setenv("DOKPLOY_REGISTRY_UPDATED_USERNAME", "updated-user")
		t.Setenv("DOKPLOY_REGISTRY_UPDATED_PASSWORD", "updated-password")
		t.Setenv("DOKPLOY_REGISTRY_UPDATED_IMAGE_PREFIX", "updated/")
		t.Run(name, func(t *testing.T) {
			t.Setenv("DOKPLOY_REGISTRY_UPDATED_"+map[string]string{"url": "URL", "username": "USERNAME", "password": "PASSWORD", "prefix": "IMAGE_PREFIX"}[name], "")
			_, ok := liveRegistryUpdatedArgs(base)
			require.False(t, ok)
		})
	}
}

func TestLiveRegistryUpdatedArgsAcceptsDistinctConfiguredConnection(t *testing.T) {
	base := RegistryArgs{Name: "registry", URL: "https://registry.example", Username: "user", Password: "password"}
	t.Setenv("DOKPLOY_REGISTRY_UPDATED_URL", "https://registry-updated.example")
	t.Setenv("DOKPLOY_REGISTRY_UPDATED_USERNAME", "updated-user")
	t.Setenv("DOKPLOY_REGISTRY_UPDATED_PASSWORD", "updated-password")
	t.Setenv("DOKPLOY_REGISTRY_UPDATED_IMAGE_PREFIX", "updated/")
	updated, ok := liveRegistryUpdatedArgs(base)
	require.True(t, ok)
	require.NotEqual(t, base.URL, updated.URL)
	require.NotEqual(t, base.Username, updated.Username)
	require.NotEqual(t, base.Password, updated.Password)
	require.NotEqual(t, value(base.ImagePrefix), value(updated.ImagePrefix))
}

func TestLiveDestinationUpdatedProviderRequiresExplicitAlternate(t *testing.T) {
	base := "s3"
	t.Setenv("DOKPLOY_ACCEPTANCE_DESTINATION_UPDATED_PROVIDER", "")
	updated, ok := liveDestinationUpdatedProvider(&base)
	require.False(t, ok)
	require.Equal(t, "s3", value(updated))
	t.Setenv("DOKPLOY_ACCEPTANCE_DESTINATION_UPDATED_PROVIDER", "compatible-provider")
	updated, ok = liveDestinationUpdatedProvider(&base)
	require.True(t, ok)
	require.Equal(t, "compatible-provider", value(updated))
}

func liveRegistryUpdatedArgs(base RegistryArgs) (RegistryArgs, bool) {
	url := os.Getenv("DOKPLOY_REGISTRY_UPDATED_URL")
	username := os.Getenv("DOKPLOY_REGISTRY_UPDATED_USERNAME")
	password := os.Getenv("DOKPLOY_REGISTRY_UPDATED_PASSWORD")
	prefix := os.Getenv("DOKPLOY_REGISTRY_UPDATED_IMAGE_PREFIX")
	if url == "" || username == "" || password == "" || prefix == "" ||
		url == base.URL || username == base.Username || password == base.Password || prefix == value(base.ImagePrefix) {
		return RegistryArgs{}, false
	}
	return RegistryArgs{URL: url, Username: username, Password: password, ImagePrefix: &prefix}, true
}

func liveDestinationUpdatedProvider(base *string) (*string, bool) {
	updated := os.Getenv("DOKPLOY_ACCEPTANCE_DESTINATION_UPDATED_PROVIDER")
	if updated == "" || (base != nil && updated == *base) {
		return base, false
	}
	return &updated, true
}

// Invoke only after acknowledged ownership has been registered for cleanup.
func confirmLiveControlPlaneCreate(kind, id string, err error) error {
	if id == "" {
		if !liveCreateDefinitelyRejected(err) {
			recordLiveCleanupFailure(kind, "", err)
		}
		if err == nil {
			err = errors.New("live create returned no confirmed identity")
		}
	}
	return err
}

// TestLiveTier1ControlPlane is deliberately serial: these are the low-load
// control-plane checks that establish fixtures for the heavier live tiers.
func TestLiveTier1ControlPlane(t *testing.T) {
	api := liveClient(t)
	runLiveCase(t, "Server", func(t *testing.T) {
		requireLiveAcceptance(t)
		ctx := liveContext(t, 2*time.Minute)
		err := runLiveServerLifecycle(t, ctx, api, func(id, name string) {
			t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Server", id, name) })
		})
		requireLiveLifecycleNoError(t, "Server", "lifecycle", err)
	})
	runLiveCase(t, "Notification", func(t *testing.T) {
		requireLiveAcceptance(t)
		runLiveNotificationCases(t, liveContext(t, 10*time.Minute), api)
	})

	runLiveCase(t, "Project", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Project{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: liveRunName("project")}})
		_, _, release := finalizeLiveProjectCreate(t, api, created, err)
		requireLivePresent(t, "project.id", created.ID)

		read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "project.id", created.ID, read.State.ProjectID)
		updatedName := read.Inputs.Name + "-updated"
		updatedDescription := "updated by live test"
		updated, err := r.Update(ctx, infer.UpdateRequest[ProjectArgs, ProjectState]{ID: created.ID, Inputs: ProjectArgs{Name: updatedName, Description: &updatedDescription}, State: read.State})
		requireNoError(t, err)
		requireLiveEqual(t, "project.id", created.ID, updated.Output.ProjectID)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "project.name", updatedName, postUpdate.Inputs.Name)
		requireLiveEqual(t, "project.description", updatedDescription, value(postUpdate.Inputs.Description))
		t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Project", created.ID, postUpdate.Inputs.Name) })
		imported, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "project.id", created.ID, imported.State.ProjectID)
		diff, err := r.Diff(ctx, infer.DiffRequest[ProjectArgs, ProjectState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadProject(t, ctx, r, created.ID, release)
	})

	runLiveCase(t, "Environment", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		projectID, _, releaseProject := liveProject(t, ctx, api)
		r := Environment{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: projectID, Name: liveRunName("environment")}})
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "environment", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[EnvironmentState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
				return v.ID, err
			})
		}
		createErr := confirmLiveControlPlaneCreate("environment", created.ID, err)
		if created.ID == "" && heavyLiveTierStopped() {
			releaseProject() // Do not cascade through an unidentified environment.
		}
		requireNoError(t, createErr)
		read, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		updatedName := read.Inputs.Name + "-updated"
		updated, err := r.Update(ctx, infer.UpdateRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID, Inputs: EnvironmentArgs{ProjectID: read.Inputs.ProjectID, Name: updatedName}, State: read.State})
		if err != nil {
			directErr := directEnvironmentUpdate(ctx, api, created.ID, read.Inputs.ProjectID, updatedName)
			recordLiveOutcome("Environment", classifyEnvironmentUpdateComparison(err, directErr))
		}
		requireNoError(t, err)
		requireLiveEqual(t, "environment.id", created.ID, updated.Output.EnvironmentID)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "environment.name", updatedName, postUpdate.Inputs.Name)
		t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Environment", created.ID, postUpdate.Inputs.Name) })
		imported, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "environment.id", created.ID, imported.State.EnvironmentID)
		diff, err := r.Diff(ctx, infer.DiffRequest[EnvironmentArgs, EnvironmentState]{Inputs: EnvironmentArgs{ProjectID: postUpdate.Inputs.ProjectID + "-replacement", Name: postUpdate.Inputs.Name}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
		deleteAndReadEnvironment(t, ctx, r, created.ID, release)
	})

	runLiveCase(t, "Destination", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Destination{client: fixedClient(api)}
		inputs := DestinationArgs{Name: liveRunName("destination"), Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST", SecretAccessKey: "live-test-secret", Bucket: "live-test-bucket", Region: "us-east-1", Endpoint: "https://pulumi-acceptance.invalid"}
		if serverID := os.Getenv("DOKPLOY_ACCEPTANCE_SERVER_ID"); serverID != "" {
			inputs.ServerID = &serverID
		}
		t.Cleanup(registerLiveSecrets(inputs.AccessKey, inputs.SecretAccessKey, inputs.Endpoint))
		created, err := r.Create(ctx, infer.CreateRequest[DestinationArgs]{Inputs: inputs})
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "destination", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[DestinationState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, confirmLiveControlPlaneCreate("destination", created.ID, err))
		read, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		updatedInputs := read.Inputs
		updatedInputs.Name += "-updated"
		updatedProvider, providerConfigured := liveDestinationUpdatedProvider(updatedInputs.Provider)
		if !providerConfigured {
			t.Run("provider mutation", func(t *testing.T) {
				t.Skip("DOKPLOY_ACCEPTANCE_DESTINATION_UPDATED_PROVIDER is not configured with a safe accepted provider")
			})
		}
		updatedInputs.Provider = updatedProvider
		updatedInputs.AccessKey = "AKIALIVETESTUPDATED"
		updatedInputs.SecretAccessKey = "live-test-secret-updated"
		updatedInputs.Bucket += "-updated"
		updatedInputs.Region = "us-west-2"
		updatedInputs.Endpoint = "https://pulumi-acceptance-updated.invalid"
		updatedInputs.AdditionalFlags = []string{"--checksum"}
		if serverID := os.Getenv("DOKPLOY_ACCEPTANCE_SERVER_ID"); serverID != "" {
			updatedInputs.ServerID = &serverID
		}
		t.Cleanup(registerLiveSecrets(updatedInputs.AccessKey, updatedInputs.SecretAccessKey, updatedInputs.Endpoint))
		_, err = r.Update(ctx, infer.UpdateRequest[DestinationArgs, DestinationState]{ID: created.ID, Inputs: updatedInputs, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "destination.name", updatedInputs.Name, postUpdate.Inputs.Name)
		requireLiveEqual(t, "destination.provider", value(updatedInputs.Provider), value(postUpdate.Inputs.Provider))
		requireLiveEqual(t, "destination.accessKey", updatedInputs.AccessKey, postUpdate.Inputs.AccessKey)
		requireLiveEqual(t, "destination.secretAccessKey", updatedInputs.SecretAccessKey, postUpdate.Inputs.SecretAccessKey)
		requireLiveEqual(t, "destination.bucket", updatedInputs.Bucket, postUpdate.Inputs.Bucket)
		requireLiveEqual(t, "destination.region", updatedInputs.Region, postUpdate.Inputs.Region)
		requireLiveEqual(t, "destination.endpoint", updatedInputs.Endpoint, postUpdate.Inputs.Endpoint)
		requireLiveEqual(t, "destination.additionalFlags", updatedInputs.AdditionalFlags, postUpdate.Inputs.AdditionalFlags)
		requireLiveEqual(t, "destination.serverId", value(updatedInputs.ServerID), value(postUpdate.Inputs.ServerID))
		imported, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "destination.id", created.ID, imported.State.DestinationID)
		diff, err := r.Diff(ctx, infer.DiffRequest[DestinationArgs, DestinationState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadDestination(t, ctx, r, created.ID, release)
	})

	runLiveCase(t, "SSHKey", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		organization, err := api.OrganizationActiveWithResponse(ctx)
		requireNoError(t, err)
		shape := "missing-id"
		if organization != nil {
			shape = classifyOrganizationActiveShape(organization.GetBody())
		}
		if shape != "flat-id" && shape != "flat-organization-id" {
			recordLiveOutcome("SSHKey", "organization-active-shape-incompatible")
			t.Skip("organization.active did not return a flat non-empty organization ID")
		}
		privateKey, publicKey := liveSSHKeyPair(t)
		r := SSHKey{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: liveRunName("ssh-key"), PrivateKey: privateKey, PublicKey: publicKey}})
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "ssh-key", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[SSHKeyState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, confirmLiveControlPlaneCreate("ssh-key", created.ID, err))
		read, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		description := "updated by live test"
		_, err = r.Update(ctx, infer.UpdateRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID, Inputs: SSHKeyArgs{Name: read.Inputs.Name, Description: &description, PrivateKey: privateKey, PublicKey: publicKey}, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "sshKey.description", description, value(postUpdate.Inputs.Description))
		t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "SSHKey", created.ID, postUpdate.Inputs.Name) })
		requireLiveEqual(t, "sshKey.privateKey", privateKey, postUpdate.Inputs.PrivateKey)
		requireLiveEqual(t, "sshKey.publicKey", publicKey, postUpdate.Inputs.PublicKey)
		imported, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "sshKey.id", created.ID, imported.State.SSHKeyID)
		diff, err := r.Diff(ctx, infer.DiffRequest[SSHKeyArgs, SSHKeyState]{Inputs: SSHKeyArgs{Name: postUpdate.Inputs.Name, PrivateKey: privateKey + "-replacement", PublicKey: publicKey}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["privateKey"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[SSHKeyArgs, SSHKeyState]{Inputs: SSHKeyArgs{Name: postUpdate.Inputs.Name, PrivateKey: privateKey, PublicKey: publicKey + "-replacement"}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["publicKey"].Kind)
		deleteAndReadSSHKey(t, ctx, r, created.ID, imported.State, release)
	})

	runLiveCase(t, "Registry", func(t *testing.T) {
		args, ok := liveRegistryArgs(t)
		if !ok {
			t.Skip("dedicated registry credentials are not configured")
		}
		ctx := liveContext(t, 2*time.Minute)
		r := Registry{client: fixedClient(api)}
		if serverID := os.Getenv("DOKPLOY_ACCEPTANCE_SERVER_ID"); serverID != "" {
			args.ServerID = &serverID
		}
		t.Cleanup(registerLiveSecrets(args.Username, args.Password, args.URL, value(args.ImagePrefix)))
		created, err := r.Create(ctx, infer.CreateRequest[RegistryArgs]{Inputs: args})
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "registry", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[RegistryState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, confirmLiveControlPlaneCreate("registry", created.ID, err))
		read, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
		requireNoError(t, err)
		postUpdate := read
		t.Run("mutation", func(t *testing.T) {
			updatedInputs, completeMutation := liveRegistryUpdatedArgs(read.Inputs)
			if !completeMutation {
				t.Skip("complete Registry mutation requires distinct valid DOKPLOY_REGISTRY_UPDATED_URL, DOKPLOY_REGISTRY_UPDATED_USERNAME, DOKPLOY_REGISTRY_UPDATED_PASSWORD, and DOKPLOY_REGISTRY_UPDATED_IMAGE_PREFIX")
			}
			updatedInputs.Name = read.Inputs.Name + "-updated"
			if serverID := os.Getenv("DOKPLOY_ACCEPTANCE_SERVER_ID"); serverID != "" {
				updatedInputs.ServerID = &serverID
			}
			t.Cleanup(registerLiveSecrets(updatedInputs.Username, updatedInputs.Password, updatedInputs.URL, value(updatedInputs.ImagePrefix)))
			_, err = r.Update(ctx, infer.UpdateRequest[RegistryArgs, RegistryState]{ID: created.ID, Inputs: updatedInputs, State: read.State})
			requireNoError(t, err)
			postUpdate, err = r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
			requireNoError(t, err)
			requireLiveEqual(t, "registry.name", updatedInputs.Name, postUpdate.Inputs.Name)
			requireLiveEqual(t, "registry.username", updatedInputs.Username, postUpdate.Inputs.Username)
			requireLiveEqual(t, "registry.password", updatedInputs.Password, postUpdate.Inputs.Password)
			requireLiveEqual(t, "registry.url", updatedInputs.URL, postUpdate.Inputs.URL)
			requireLiveEqual(t, "registry.imagePrefix", value(updatedInputs.ImagePrefix), value(postUpdate.Inputs.ImagePrefix))
			requireLiveEqual(t, "registry.serverId", value(updatedInputs.ServerID), value(postUpdate.Inputs.ServerID))
		})
		imported, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "registry.id", created.ID, imported.State.RegistryID)
		t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Registry", created.ID, imported.Inputs.Name) })
		diff, err := r.Diff(ctx, infer.DiffRequest[RegistryArgs, RegistryState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadRegistry(t, ctx, r, created.ID, imported.State, release)
	})

	runLiveCase(t, "Tag", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Tag{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: liveRunName("tag"), Color: stringPtr("#123456")}})
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "tag", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[TagState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, confirmLiveControlPlaneCreate("tag", created.ID, err))
		read, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		updated := read.Inputs
		updated.Name += "-updated"
		updated.Color = stringPtr("#654321")
		_, err = r.Update(ctx, infer.UpdateRequest[TagArgs, TagState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "tag.name", updated.Name, postUpdate.Inputs.Name)
		requireLiveEqual(t, "tag.color", "#654321", value(postUpdate.Inputs.Color))
		imported, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "tag.id", created.ID, imported.State.TagID)
		diff, err := r.Diff(ctx, infer.DiffRequest[TagArgs, TagState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadTag(t, ctx, r, created.ID, release)
	})

	runLiveCase(t, "ProjectTag", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		projectID, _, _ := liveProject(t, ctx, api)
		tag := Tag{client: fixedClient(api)}
		tagCreated, err := tag.Create(ctx, infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: liveRunName("tag"), Color: stringPtr("#123456")}})
		tagRelease := func() {}
		if tagCreated.ID != "" {
			tagRelease = deferLiveDelete(t, "tag", tagCreated.ID, func(ctx context.Context) error {
				_, err := tag.Delete(ctx, infer.DeleteRequest[TagState]{ID: tagCreated.ID, State: tagCreated.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := tag.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: tagCreated.ID})
				return v.ID, err
			})
		}
		requireNoError(t, confirmLiveControlPlaneCreate("tag", tagCreated.ID, err))
		r := ProjectTag{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ProjectTagArgs]{Inputs: ProjectTagArgs{ProjectID: projectID, TagID: tagCreated.ID}})
		if err != nil && created.ID != "" {
			_, classification := pollProjectTagAssociation(ctx, func(readCtx context.Context) (bool, error) {
				read, readErr := r.Read(readCtx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: created.ID})
				return read.ID != "", readErr
			})
			recordLiveOutcome("ProjectTag", classification)
		}
		release := func() {}
		if created.ID != "" {
			release = deferLiveDelete(t, "project-tag", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[ProjectTagState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: created.ID})
		requireNoError(t, err)
		requireLiveEqual(t, "projectTag.id", created.ID, read.ID)
		diff, err := r.Diff(ctx, infer.DiffRequest[ProjectTagArgs, ProjectTagState]{Inputs: ProjectTagArgs{ProjectID: projectID + "-replacement", TagID: tagCreated.ID}, State: read.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[ProjectTagArgs, ProjectTagState]{Inputs: ProjectTagArgs{ProjectID: projectID, TagID: tagCreated.ID + "-replacement"}, State: read.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["tagId"].Kind)
		deleteAndReadProjectTag(t, ctx, r, created.ID, read.State, release)
		deleteAndReadTag(t, ctx, tag, tagCreated.ID, tagRelease)
	})
}

func deferLiveDelete(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) func() {
	t.Helper()
	return registerLiveCleanup(t, kind, id, remove, read)
}
func deleteAndReadProject(t *testing.T, ctx context.Context, r Project, id string, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "Project", "delete", err)
}
func deleteAndReadEnvironment(t *testing.T, ctx context.Context, r Environment, id string, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[EnvironmentState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "Environment", "delete", err)
}
func deleteAndReadDestination(t *testing.T, ctx context.Context, r Destination, id string, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[DestinationState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "Destination", "delete", err)
}
func deleteAndReadSSHKey(t *testing.T, ctx context.Context, r SSHKey, id string, state SSHKeyState, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[SSHKeyState]{ID: id, State: state})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "SSHKey", "delete", err)
}
func deleteAndReadRegistry(t *testing.T, ctx context.Context, r Registry, id string, state RegistryState, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[RegistryState]{ID: id, State: state})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "Registry", "delete", err)
}
func deleteAndReadTag(t *testing.T, ctx context.Context, r Tag, id string, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[TagState]{ID: id})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "Tag", "delete", err)
}
func deleteAndReadProjectTag(t *testing.T, ctx context.Context, r ProjectTag, id string, state ProjectTagState, release func()) {
	err := deleteAndVerifyLiveOwned(ctx, func() error {
		_, err := r.Delete(ctx, infer.DeleteRequest[ProjectTagState]{ID: id, State: state})
		return err
	}, func() (string, error) {
		read, err := r.Read(ctx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: id})
		return read.ID, err
	}, release)
	requireLiveLifecycleNoError(t, "ProjectTag", "delete", err)
}
