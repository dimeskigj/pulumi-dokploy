package dokploy

// This file exercises the provider against a real, already-running Dokploy
// instance instead of the scripted HTTP fixtures used everywhere else in
// this package. Scripted fixtures encode assumptions about Dokploy's wire
// format; they cannot catch it when those assumptions are wrong. That is
// exactly how the nine defects in
// docs/superpowers/plans/2026-08-30-live-dokploy-bug-reports.md shipped
// undetected. Each test below targets one of those fixes directly against
// the real API and fails if it regresses.
//
// None of this runs automatically: every test calls liveClient, which skips
// immediately unless DOKPLOY_ENDPOINT and DOKPLOY_API_KEY are set. To run
// them locally against your own instance:
//
//	DOKPLOY_ENDPOINT=https://dokploy.example.com DOKPLOY_API_KEY=... \
//	    go test ./provider/... -run TestLive -v
//
// These tests create and destroy real projects, applications, databases,
// and domains on the target instance. Point them only at a disposable or
// test Dokploy instance you control.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/google/uuid"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// requireNoError is require.NoError, except that when err wraps the
// provider's ResourceInitFailedError it also surfaces the underlying
// Reasons: that error's Error() method deliberately discards them, so a
// bare require.NoError would otherwise only ever report the useless
// "resource failed to initialize".
func requireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	var initErr infer.ResourceInitFailedError
	if errors.As(err, &initErr) {
		t.Fatalf("%v (reasons: %v)", err, initErr.Reasons)
	}
	require.NoError(t, err, msgAndArgs...)
}

// liveClient returns a Client wired to a real Dokploy instance, or skips the
// calling test when credentials are not configured.
func liveClient(t *testing.T) *client.Client {
	t.Helper()
	endpoint := os.Getenv("DOKPLOY_ENDPOINT")
	apiKey := os.Getenv("DOKPLOY_API_KEY")
	if endpoint == "" || apiKey == "" {
		t.Skip("live Dokploy credentials are not configured (set DOKPLOY_ENDPOINT and DOKPLOY_API_KEY)")
	}
	api, err := client.New(endpoint, apiKey)
	require.NoError(t, err)
	return api
}

// liveContext bounds a live test to timeout, since a misbehaving live server
// (rather than a bug in this provider) should not hang the test suite
// forever.
func liveContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// liveProject creates a scratch Project (and its default Environment) for a
// single live test run and registers its cleanup. Every other live test
// hangs its resources off the returned default environment ID so a single
// project.remove call (bug 3) at the end tears down everything left behind
// by a failed assertion.
func liveProject(t *testing.T, ctx context.Context, api *client.Client) (id, defaultEnvironmentID string) {
	t.Helper()
	created, err := (Project{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[ProjectArgs]{
		Inputs: ProjectArgs{Name: "live-test-" + uuid.NewString()},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if _, err := (Project{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[ProjectState]{ID: created.ID}); err != nil {
			t.Errorf("cleanup: project.remove for %s: %v", created.ID, err)
		}
	})
	return created.ID, created.Output.DefaultEnvironmentID
}

func TestLiveApplicationDockerLifecycleDeploysAndReadsFlattenedSource(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	created, err := (Application{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{
		Name: "app", EnvironmentID: environmentID,
		Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}},
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Application{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[ApplicationState]{ID: id}); err != nil {
				t.Errorf("cleanup: application.delete for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive application.deploy's empty response body (bug 2) and poll applicationStatus, not status (bug 4)")
	require.NotEmpty(t, created.ID)
	require.Equal(t, statusDone, created.Output.Status)

	appID := created.ID
	domain, err := (Domain{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[DomainArgs]{Inputs: DomainArgs{
		ApplicationID: &appID, Host: "live-test-" + uuid.NewString() + ".example.invalid",
		Port: intPtr(80), CertificateType: CertificateNone, Enabled: true,
	}})
	if domain.ID != "" {
		id := domain.ID
		t.Cleanup(func() {
			if _, err := (Domain{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[DomainState]{ID: id}); err != nil {
				t.Errorf("cleanup: domain.delete for %s: %v", id, err)
			}
		})
	}
	require.NoError(t, err)
	_, err = (Domain{client: fixedClient(api)}).Delete(ctx, infer.DeleteRequest[DomainState]{ID: domain.ID})
	require.NoError(t, err, "domain.delete must succeed even though Dokploy echoes back the full entity, not a boolean (bug 3)")

	read, err := (Application{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: created.ID})
	require.NoError(t, err, "Read must reconstruct source from flattened top-level fields, not a nested source object (bug 5)")
	require.NotNil(t, read.Inputs.Source.Docker)
	require.Equal(t, "nginx:1.27", read.Inputs.Source.Docker.Image)
}

func TestLiveComposeRawSourceDeploysWithCorrectSourceType(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	created, err := (Compose{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{
		Name: "stack", EnvironmentID: environmentID,
		Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}},
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Compose{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[ComposeState]{ID: id}); err != nil {
				t.Errorf("cleanup: compose.delete for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive compose.deploy's empty response body (bug 2) and poll composeStatus, not status (bug 4)")
	require.Equal(t, statusDone, created.Output.Status)

	read, err := (Compose{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[ComposeArgs, ComposeState]{ID: created.ID})
	require.NoError(t, err, "Read must reconstruct source from flattened top-level fields, not a nested source object (bug 5)")
	require.Equal(t, ComposeSourceRaw, read.Inputs.Source.Type,
		"the deployed compose must actually carry sourceType=raw, not Dokploy's github default that a raw create silently produced before the fix (bug 6)")
	require.NotNil(t, read.Inputs.Source.Raw)
}

func TestLiveComposeGitSourceConfiguresProviderWithoutTypeMismatch(t *testing.T) {
	api := liveClient(t)
	t.Parallel()
	ctx := liveContext(t, 2*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	// Create the bare compose directly (skipping Compose.Create's deploy and
	// poll steps) since this test only needs to exercise source
	// configuration and fetchSourceType, not a full, potentially slow,
	// git clone and build.
	resp, err := api.ComposeCreateWithResponse(ctx, generated.ComposeCreateJSONRequestBody{
		Name: "git-stack", EnvironmentId: environmentID,
		ComposeType: ptr(generated.ComposeCreateJSONBodyComposeType(ComposeDocker)),
	})
	require.NoError(t, err)
	require.NotNil(t, resp.JSON200)
	require.NotNil(t, resp.JSON200.ComposeId)
	composeID := *resp.JSON200.ComposeId
	t.Cleanup(func() {
		if _, err := api.ComposeDeleteWithResponse(context.Background(), generated.ComposeDeleteJSONRequestBody{ComposeId: composeID}); err != nil {
			t.Errorf("cleanup: compose.delete for %s: %v", composeID, err)
		}
	})

	source := ComposeSource{Type: ComposeSourceGit, Git: &GitComposeSource{URL: "https://github.com/dimeskigj/pulumi-dokploy", Branch: "main"}}
	require.NoError(t, configureComposeSource(ctx, api, composeID, source))
	require.NoError(t, fetchComposeSource(ctx, api, composeID, ComposeSourceGit),
		"compose.fetchSourceType must decode its response as a string, not a boolean (bug 7)")

	one, err := api.ComposeOneWithResponse(ctx, &generated.ComposeOneParams{ComposeId: composeID})
	require.NoError(t, err)
	require.NotNil(t, one.JSON200)
	require.Equal(t, "git", stringValue(one.JSON200.AdditionalProperties, "sourceType", "type"),
		"the git source configured above must actually persist on the compose")
}

func TestLivePostgresLifecycleDeploysAndReportsStatus(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	// DockerImage is supplied explicitly because these tests call Create
	// directly and never go through Check, which is where the "postgres:18"
	// schema default actually gets applied.
	created, err := (Postgres{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{
		Name: "db", EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Postgres{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[PostgresState]{ID: id}); err != nil {
				t.Errorf("cleanup: postgres.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive postgres.deploy's empty response body (bug 2) and poll the shared applicationStatus field, not status or postgresStatus (bug 4)")
	require.Equal(t, statusDone, created.Output.Status)
}

func TestLiveMySQLLifecycleDeploysAndReportsStatus(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	rootPassword := "live-test-root-password"
	created, err := (MySQL{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MySQLArgs]{Inputs: MySQLArgs{
		Name: "db", EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DatabaseRootPassword: &rootPassword, DockerImage: "mysql:8",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (MySQL{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[MySQLState]{ID: id}); err != nil {
				t.Errorf("cleanup: mysql.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive mysql.deploy's empty response body and poll the shared applicationStatus field")
	require.Equal(t, statusDone, created.Output.Status)

	read, err := (MySQL{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: created.ID})
	require.NoError(t, err, "Read must reconstruct the observed databaseRootPassword")
	require.NotNil(t, read.Inputs.DatabaseRootPassword)
	require.Equal(t, rootPassword, *read.Inputs.DatabaseRootPassword)
}

func TestLiveMariaDBLifecycleDeploysAndReportsStatus(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	created, err := (MariaDB{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{
		Name: "db", EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mariadb:11",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (MariaDB{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[MariaDBState]{ID: id}); err != nil {
				t.Errorf("cleanup: mariadb.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive mariadb.deploy's empty response body and poll the shared applicationStatus field")
	require.Equal(t, statusDone, created.Output.Status)
}

func TestLiveMongoDBLifecycleDeploysAndReportsStatus(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	created, err := (MongoDB{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[MongoDBArgs]{Inputs: MongoDBArgs{
		Name: "db", EnvironmentID: environmentID, DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mongo:8",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (MongoDB{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[MongoDBState]{ID: id}); err != nil {
				t.Errorf("cleanup: mongo.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive mongo.deploy's empty response body and poll the shared applicationStatus field")
	require.Equal(t, statusDone, created.Output.Status)

	read, err := (MongoDB{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: created.ID})
	require.NoError(t, err, "Read must reconstruct the MongoDB entity without a databaseName field")
	require.Equal(t, "app", read.Inputs.DatabaseUser)
}

func TestLiveRedisLifecycleDeploysAndReportsStatus(t *testing.T) {
	// This closes the report's explicit open follow-up: Redis's status field
	// name (applicationStatus, shared with Postgres) was inferred by
	// symmetry but never independently confirmed against a live Redis
	// resource.
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)

	created, err := (Redis{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[RedisArgs]{Inputs: RedisArgs{
		Name: "cache", EnvironmentID: environmentID, DatabasePassword: "live-test-password", DockerImage: "redis:8",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Redis{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[RedisState]{ID: id}); err != nil {
				t.Errorf("cleanup: redis.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err, "Create must survive redis.deploy's empty response body (bug 2) and poll applicationStatus, not status or redisStatus (bug 4)")
	require.Equal(t, statusDone, created.Output.Status)
}

// liveDestination creates a scratch Destination for a live test run and
// registers its cleanup.
func liveDestination(t *testing.T, ctx context.Context, api *client.Client) string {
	t.Helper()
	created, err := (Destination{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[DestinationArgs]{Inputs: DestinationArgs{
		Name: "live-test-" + uuid.NewString(), Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST", SecretAccessKey: "live-test-secret",
		Bucket: "live-test-bucket", Region: "us-east-1", Endpoint: "https://s3.us-east-1.amazonaws.com",
	}})
	require.NoError(t, err)
	t.Cleanup(func() {
		if _, err := (Destination{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[DestinationState]{ID: created.ID}); err != nil {
			t.Errorf("cleanup: destination.remove for %s: %v", created.ID, err)
		}
	})
	return created.ID
}

func TestLiveDestinationLifecycle(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 2*time.Minute)

	created, err := (Destination{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[DestinationArgs]{Inputs: DestinationArgs{
		Name: "live-test-" + uuid.NewString(), Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST", SecretAccessKey: "live-test-secret",
		Bucket: "live-test-bucket", Region: "us-east-1", Endpoint: "https://s3.us-east-1.amazonaws.com",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Destination{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[DestinationState]{ID: id}); err != nil {
				t.Errorf("cleanup: destination.remove for %s: %v", id, err)
			}
		})
	}
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	read, err := (Destination{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
	require.NoError(t, err, "destination.one must echo back secretAccessKey in plaintext")
	require.Equal(t, "live-test-secret", read.Inputs.SecretAccessKey)
	require.Equal(t, "live-test-bucket", read.Inputs.Bucket)

	_, err = (Destination{client: fixedClient(api)}).Update(ctx, infer.UpdateRequest[DestinationArgs, DestinationState]{ID: created.ID, Inputs: DestinationArgs{
		Name: read.Inputs.Name, Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST", SecretAccessKey: "live-test-secret",
		Bucket: "live-test-bucket-2", Region: "us-east-1", Endpoint: "https://s3.us-east-1.amazonaws.com",
	}, State: DestinationState{DestinationArgs: read.Inputs, DestinationID: created.ID}})
	require.NoError(t, err)

	read, err = (Destination{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[DestinationArgs, DestinationState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, "live-test-bucket-2", read.Inputs.Bucket)
}

func TestLiveBackupLifecycleResolvesIDAfterEmptyCreateResponse(t *testing.T) {
	// backup.create returns HTTP 200 with an empty body on this Dokploy
	// instance (confirmed by direct probing), unlike every other create
	// endpoint this provider calls. This test is the regression guard for
	// Backup.Create's before/after diff workaround that recovers the new
	// backupId from the target database's nested backups list.
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)
	destinationID := liveDestination(t, ctx, api)

	pg, err := (Postgres{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{
		Name: "db", EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18",
	}})
	if pg.ID != "" {
		id := pg.ID
		t.Cleanup(func() {
			if _, err := (Postgres{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[PostgresState]{ID: id}); err != nil {
				t.Errorf("cleanup: postgres.remove for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err)

	created, err := (Backup{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: BackupArgs{
		Schedule: "0 0 * * *", Enabled: true, Prefix: "live-test-", DestinationID: destinationID, Database: "app", PostgresID: &pg.ID,
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (Backup{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[BackupState]{ID: id}); err != nil {
				t.Errorf("cleanup: backup.remove for %s: %v", id, err)
			}
		})
	}
	require.NoError(t, err, "Create must recover the backupId despite backup.create's empty response body")
	require.NotEmpty(t, created.ID)

	read, err := (Backup{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, pg.ID, *read.Inputs.PostgresID)
	require.Equal(t, "app", read.Inputs.Database)

	_, err = (Backup{client: fixedClient(api)}).Update(ctx, infer.UpdateRequest[BackupArgs, BackupState]{ID: created.ID, Inputs: BackupArgs{
		Schedule: "0 1 * * *", Enabled: false, Prefix: "live-test-", DestinationID: destinationID, Database: "app", PostgresID: &pg.ID,
	}, State: BackupState{BackupArgs: read.Inputs, BackupID: created.ID}})
	require.NoError(t, err, "backup.update must succeed despite also returning an empty response body")

	read, err = (Backup{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, "0 1 * * *", read.Inputs.Schedule)
	require.False(t, read.Inputs.Enabled)
}

func TestLiveVolumeBackupLifecycleForApplication(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)
	destinationID := liveDestination(t, ctx, api)

	app, err := (Application{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{
		Name: "app", EnvironmentID: environmentID,
		Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}},
	}})
	if app.ID != "" {
		id := app.ID
		t.Cleanup(func() {
			if _, err := (Application{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[ApplicationState]{ID: id}); err != nil {
				t.Errorf("cleanup: application.delete for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err)

	created, err := (VolumeBackup{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[VolumeBackupArgs]{Inputs: VolumeBackupArgs{
		Name: "live-test-" + uuid.NewString(), VolumeName: "live-test-volume", Prefix: "live-test-", DestinationID: destinationID,
		CronExpression: "0 0 * * *", Enabled: true, ApplicationID: &app.ID,
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (VolumeBackup{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[VolumeBackupState]{ID: id}); err != nil {
				t.Errorf("cleanup: volumeBackups.delete for %s: %v", id, err)
			}
		})
	}
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	read, err := (VolumeBackup{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, app.ID, *read.Inputs.ApplicationID)
	require.Equal(t, "live-test-volume", read.Inputs.VolumeName)

	_, err = (VolumeBackup{client: fixedClient(api)}).Update(ctx, infer.UpdateRequest[VolumeBackupArgs, VolumeBackupState]{ID: created.ID, Inputs: VolumeBackupArgs{
		Name: read.Inputs.Name, VolumeName: "live-test-volume", Prefix: "live-test-", DestinationID: destinationID,
		CronExpression: "0 1 * * *", Enabled: false, ApplicationID: &app.ID,
	}, State: VolumeBackupState{VolumeBackupArgs: read.Inputs, VolumeBackupID: created.ID}})
	require.NoError(t, err)

	read, err = (VolumeBackup{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, "0 1 * * *", read.Inputs.CronExpression)
	require.False(t, read.Inputs.Enabled)
}

func TestLiveVolumeBackupLifecycleForCompose(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 10*time.Minute)
	_, environmentID := liveProject(t, ctx, api)
	destinationID := liveDestination(t, ctx, api)

	compose, err := (Compose{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{
		Name: "stack", EnvironmentID: environmentID,
		Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}},
	}})
	if compose.ID != "" {
		id := compose.ID
		t.Cleanup(func() {
			if _, err := (Compose{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[ComposeState]{ID: id}); err != nil {
				t.Errorf("cleanup: compose.delete for %s: %v", id, err)
			}
		})
	}
	requireNoError(t, err)

	created, err := (VolumeBackup{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[VolumeBackupArgs]{Inputs: VolumeBackupArgs{
		Name: "live-test-" + uuid.NewString(), VolumeName: "live-test-volume", Prefix: "live-test-", DestinationID: destinationID,
		CronExpression: "0 0 * * *", Enabled: true, ComposeID: &compose.ID, ServiceName: stringPtr("web"),
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			if _, err := (VolumeBackup{client: fixedClient(api)}).Delete(context.Background(), infer.DeleteRequest[VolumeBackupState]{ID: id}); err != nil {
				t.Errorf("cleanup: volumeBackups.delete for %s: %v", id, err)
			}
		})
	}
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	read, err := (VolumeBackup{client: fixedClient(api)}).Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: created.ID})
	require.NoError(t, err)
	require.Equal(t, compose.ID, *read.Inputs.ComposeID)
	require.Equal(t, "web", *read.Inputs.ServiceName)
	require.Nil(t, read.Inputs.ApplicationID)
}

func TestLiveEnvironmentLifecycle(t *testing.T) {
	api := liveClient(t)
	t.Parallel()
	ctx := liveContext(t, 2*time.Minute)
	projectID, _ := liveProject(t, ctx, api)

	created, err := (Environment{client: fixedClient(api)}).Create(ctx, infer.CreateRequest[EnvironmentArgs]{Inputs: EnvironmentArgs{
		ProjectID: projectID, Name: "staging",
	}})
	require.NoError(t, err)

	_, err = (Environment{client: fixedClient(api)}).Delete(ctx, infer.DeleteRequest[EnvironmentState]{ID: created.ID})
	require.NoError(t, err, "environment.remove must succeed even though Dokploy echoes back the full entity, not a boolean (bug 3)")
}
