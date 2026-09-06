package dokploy

import (
	"context"
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

// TestLiveTier1ControlPlane is deliberately serial: these are the low-load
// control-plane checks that establish fixtures for the heavier live tiers.
func TestLiveTier1ControlPlane(t *testing.T) {
	api := liveClient(t)

	t.Run("Project", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Project{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: liveRunName("project")}})
		if created.ID != "" {
			deferLiveDelete(t, "project", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		require.NotEmpty(t, created.ID)

		read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, read.State.ProjectID)
		updatedDescription := "updated by live test"
		updated, err := r.Update(ctx, infer.UpdateRequest[ProjectArgs, ProjectState]{ID: created.ID, Inputs: ProjectArgs{Name: read.Inputs.Name, Description: &updatedDescription}, State: read.State})
		requireNoError(t, err)
		require.Equal(t, created.ID, updated.Output.ProjectID)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, updatedDescription, value(postUpdate.Inputs.Description))
		imported, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.ProjectID)
		diff, err := r.Diff(ctx, infer.DiffRequest[ProjectArgs, ProjectState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadProject(t, ctx, r, created.ID)
	})

	t.Run("Environment", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		projectID, _ := liveProject(t, ctx, api)
		r := Environment{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{ProjectID: projectID, Name: liveRunName("environment")}})
		if created.ID != "" {
			deferLiveDelete(t, "environment", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[EnvironmentState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		updatedName := read.Inputs.Name + "-updated"
		updated, err := r.Update(ctx, infer.UpdateRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID, Inputs: EnvironmentArgs{ProjectID: read.Inputs.ProjectID, Name: updatedName}, State: read.State})
		if err != nil {
			directErr := directEnvironmentUpdate(ctx, api, created.ID, read.Inputs.ProjectID, updatedName)
			recordLiveOutcome("Environment", classifyEnvironmentUpdateComparison(err, directErr))
		}
		requireNoError(t, err)
		require.Equal(t, created.ID, updated.Output.EnvironmentID)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, updatedName, postUpdate.Inputs.Name)
		imported, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.EnvironmentID)
		diff, err := r.Diff(ctx, infer.DiffRequest[EnvironmentArgs, EnvironmentState]{Inputs: EnvironmentArgs{ProjectID: postUpdate.Inputs.ProjectID + "-replacement", Name: postUpdate.Inputs.Name}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
		deleteAndReadEnvironment(t, ctx, r, created.ID)
	})

	t.Run("Destination", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Destination{client: fixedClient(api)}
		inputs := DestinationArgs{Name: liveRunName("destination"), Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST", SecretAccessKey: "live-test-secret", Bucket: "live-test-bucket", Region: "us-east-1", Endpoint: "https://pulumi-acceptance.invalid"}
		t.Cleanup(registerLiveSecrets(inputs.AccessKey, inputs.SecretAccessKey, inputs.Endpoint))
		created, err := r.Create(ctx, infer.CreateRequest[DestinationArgs]{Inputs: inputs})
		if created.ID != "" {
			deferLiveDelete(t, "destination", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[DestinationState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		updatedInputs := read.Inputs
		updatedInputs.Bucket += "-updated"
		_, err = r.Update(ctx, infer.UpdateRequest[DestinationArgs, DestinationState]{ID: created.ID, Inputs: updatedInputs, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, updatedInputs.Bucket, postUpdate.Inputs.Bucket)
		imported, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.DestinationID)
		diff, err := r.Diff(ctx, infer.DiffRequest[DestinationArgs, DestinationState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadDestination(t, ctx, r, created.ID)
	})

	t.Run("SSHKey", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		organization, err := api.OrganizationActiveWithResponse(ctx)
		requireNoError(t, err)
		if organization == nil || classifyOrganizationActiveShape(organization.GetBody()) != "flat-non-empty-id" {
			recordLiveOutcome("SSHKey", "organization-active-shape-incompatible")
			t.Skip("organization.active did not return a flat non-empty organization ID")
		}
		privateKey, publicKey := liveSSHKeyPair(t)
		r := SSHKey{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[SSHKeyArgs]{Inputs: SSHKeyArgs{Name: liveRunName("ssh-key"), PrivateKey: privateKey, PublicKey: publicKey}})
		if created.ID != "" {
			deferLiveDelete(t, "ssh-key", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[SSHKeyState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		description := "updated by live test"
		_, err = r.Update(ctx, infer.UpdateRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID, Inputs: SSHKeyArgs{Name: read.Inputs.Name, Description: &description, PrivateKey: privateKey, PublicKey: publicKey}, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, description, value(postUpdate.Inputs.Description))
		requireLiveEqual(t, "sshKey.privateKey", privateKey, postUpdate.Inputs.PrivateKey)
		requireLiveEqual(t, "sshKey.publicKey", publicKey, postUpdate.Inputs.PublicKey)
		imported, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.SSHKeyID)
		diff, err := r.Diff(ctx, infer.DiffRequest[SSHKeyArgs, SSHKeyState]{Inputs: SSHKeyArgs{Name: postUpdate.Inputs.Name, PrivateKey: privateKey + "-replacement", PublicKey: publicKey}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["privateKey"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[SSHKeyArgs, SSHKeyState]{Inputs: SSHKeyArgs{Name: postUpdate.Inputs.Name, PrivateKey: privateKey, PublicKey: publicKey + "-replacement"}, State: postUpdate.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["publicKey"].Kind)
		deleteAndReadSSHKey(t, ctx, r, created.ID, imported.State)
	})

	t.Run("Registry", func(t *testing.T) {
		args, ok := liveRegistryArgs(t)
		if !ok {
			t.Skip("dedicated registry credentials are not configured")
		}
		ctx := liveContext(t, 2*time.Minute)
		r := Registry{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[RegistryArgs]{Inputs: args})
		if created.ID != "" {
			deferLiveDelete(t, "registry", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[RegistryState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
		requireNoError(t, err)
		updatedInputs := read.Inputs
		updatedInputs.Name += "-updated"
		_, err = r.Update(ctx, infer.UpdateRequest[RegistryArgs, RegistryState]{ID: created.ID, Inputs: updatedInputs, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, updatedInputs.Name, postUpdate.Inputs.Name)
		imported, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.RegistryID)
		diff, err := r.Diff(ctx, infer.DiffRequest[RegistryArgs, RegistryState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadRegistry(t, ctx, r, created.ID, imported.State)
	})

	t.Run("Tag", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		r := Tag{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: liveRunName("tag"), Color: stringPtr("#123456")}})
		if created.ID != "" {
			deferLiveDelete(t, "tag", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[TagState]{ID: created.ID, State: created.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		read, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		updated := read.Inputs
		updated.Color = stringPtr("#654321")
		_, err = r.Update(ctx, infer.UpdateRequest[TagArgs, TagState]{ID: created.ID, Inputs: updated, State: read.State})
		requireNoError(t, err)
		postUpdate, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, value(postUpdate.Inputs.Color), "#654321")
		imported, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: created.ID})
		requireNoError(t, err)
		require.Equal(t, created.ID, imported.State.TagID)
		diff, err := r.Diff(ctx, infer.DiffRequest[TagArgs, TagState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
		requireNoError(t, err)
		require.False(t, diff.HasChanges)
		deleteAndReadTag(t, ctx, r, created.ID)
	})

	t.Run("ProjectTag", func(t *testing.T) {
		ctx := liveContext(t, 2*time.Minute)
		projectID, _ := liveProject(t, ctx, api)
		tag := Tag{client: fixedClient(api)}
		tagCreated, err := tag.Create(ctx, infer.CreateRequest[TagArgs]{Inputs: TagArgs{Name: liveRunName("tag"), Color: stringPtr("#123456")}})
		if tagCreated.ID != "" {
			deferLiveDelete(t, "tag", tagCreated.ID, func(ctx context.Context) error {
				_, err := tag.Delete(ctx, infer.DeleteRequest[TagState]{ID: tagCreated.ID, State: tagCreated.Output})
				return err
			}, func(ctx context.Context) (string, error) {
				v, err := tag.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: tagCreated.ID})
				return v.ID, err
			})
		}
		requireNoError(t, err)
		r := ProjectTag{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ProjectTagArgs]{Inputs: ProjectTagArgs{ProjectID: projectID, TagID: tagCreated.ID}})
		if err != nil && created.ID != "" {
			_, classification := pollProjectTagAssociation(ctx, func(readCtx context.Context) (bool, error) {
				read, readErr := r.Read(readCtx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: created.ID})
				return read.ID != "", readErr
			})
			recordLiveOutcome("ProjectTag", classification)
		}
		if created.ID != "" {
			deferLiveDelete(t, "project-tag", created.ID, func(ctx context.Context) error {
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
		require.Equal(t, created.ID, read.ID)
		diff, err := r.Diff(ctx, infer.DiffRequest[ProjectTagArgs, ProjectTagState]{Inputs: ProjectTagArgs{ProjectID: projectID + "-replacement", TagID: tagCreated.ID}, State: read.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["projectId"].Kind)
		diff, err = r.Diff(ctx, infer.DiffRequest[ProjectTagArgs, ProjectTagState]{Inputs: ProjectTagArgs{ProjectID: projectID, TagID: tagCreated.ID + "-replacement"}, State: read.State})
		requireNoError(t, err)
		require.Equal(t, p.UpdateReplace, diff.DetailedDiff["tagId"].Kind)
		deleteAndReadProjectTag(t, ctx, r, created.ID, read.State)
	})
}

func deferLiveDelete(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	t.Cleanup(func() { liveCleanupVerified(t, kind, id, remove, read) })
}
func deleteAndReadProject(t *testing.T, ctx context.Context, r Project, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadEnvironment(t *testing.T, ctx context.Context, r Environment, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[EnvironmentState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[EnvironmentArgs, EnvironmentState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadDestination(t *testing.T, ctx context.Context, r Destination, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[DestinationState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadSSHKey(t *testing.T, ctx context.Context, r SSHKey, id string, state SSHKeyState) {
	_, err := r.Delete(ctx, infer.DeleteRequest[SSHKeyState]{ID: id, State: state})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[SSHKeyArgs, SSHKeyState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadRegistry(t *testing.T, ctx context.Context, r Registry, id string, state RegistryState) {
	_, err := r.Delete(ctx, infer.DeleteRequest[RegistryState]{ID: id, State: state})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[RegistryArgs, RegistryState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadTag(t *testing.T, ctx context.Context, r Tag, id string) {
	_, err := r.Delete(ctx, infer.DeleteRequest[TagState]{ID: id})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[TagArgs, TagState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
func deleteAndReadProjectTag(t *testing.T, ctx context.Context, r ProjectTag, id string, state ProjectTagState) {
	_, err := r.Delete(ctx, infer.DeleteRequest[ProjectTagState]{ID: id, State: state})
	requireNoError(t, err)
	read, err := r.Read(ctx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, "", read.ID)
}
