package dokploy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// TestLiveTier4Backups is deliberately serial. A backup target is a real
// database (or deployed workload), and the small acceptance server must not
// be asked to create several heavy resources concurrently.
func TestLiveTier4Backups(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 50*time.Minute)
	_, environmentID := liveProject(t, ctx, api)
	destinationID := liveBackupDestination(t, ctx, api)

	t.Run("Backup/Postgres", func(t *testing.T) { liveBackupForPostgres(t, ctx, api, environmentID, destinationID) })
	t.Run("Backup/MySQL", func(t *testing.T) { liveBackupForMySQL(t, ctx, api, environmentID, destinationID) })
	t.Run("Backup/MariaDB", func(t *testing.T) { liveBackupForMariaDB(t, ctx, api, environmentID, destinationID) })
	t.Run("Backup/MongoDB", func(t *testing.T) { liveBackupForMongoDB(t, ctx, api, environmentID, destinationID) })
	t.Run("VolumeBackup/Application", func(t *testing.T) { liveVolumeBackupForApplication(t, ctx, api, environmentID, destinationID) })
	t.Run("VolumeBackup/Compose", func(t *testing.T) { liveVolumeBackupForCompose(t, ctx, api, environmentID, destinationID) })
}

func liveBackupDestination(t *testing.T, ctx context.Context, api *client.Client) string {
	t.Helper()
	r := Destination{client: fixedClient(api)}
	created, err := r.Create(ctx, infer.CreateRequest[DestinationArgs]{Inputs: DestinationArgs{
		Name: liveRunName("backup-destination"), Provider: stringPtr("s3"), AccessKey: "AKIALIVETEST",
		SecretAccessKey: "live-test-secret", Bucket: "live-test-bucket", Region: "us-east-1",
		Endpoint: "https://pulumi-dokploy-backup.invalid",
	}})
	if created.ID != "" {
		id := created.ID
		t.Cleanup(func() {
			liveCleanupVerified(t, "destination", id, func(c context.Context) error {
				_, e := r.Delete(c, infer.DeleteRequest[DestinationState]{ID: id})
				return e
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[DestinationArgs, DestinationState]{ID: id})
				return v.ID, e
			})
		})
	}
	if err != nil {
		if isExternalDestinationConnectivityValidation(err) {
			t.Skip("Dokploy validates the backup destination network before lifecycle tests: " + sanitizeLiveDiagnostic(err.Error()))
		}
		requireNoError(t, err)
	}
	require.NotEmpty(t, created.ID)
	return created.ID
}

func isExternalDestinationConnectivityValidation(err error) bool {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.Operation != "destination.create" {
		return false
	}
	if apiErr.StatusCode != 400 && apiErr.StatusCode != 422 {
		return false
	}
	code := strings.ToLower(apiErr.Code)
	switch code {
	case "destination_connection_failed", "destination_connection_error", "destination_network_unreachable", "s3_connection_failed", "storage_connection_failed":
		return true
	}
	message := strings.ToLower(apiErr.Message)
	return strings.Contains(message, "failed to connect") ||
		strings.Contains(message, "could not connect") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "network unreachable") ||
		strings.Contains(message, "i/o timeout")
}

type liveBackupTarget struct {
	id       string
	lease    *liveHeavyOperationLease
	remove   func(context.Context) error
	readID   func(context.Context) (string, error)
	resource string
}

type liveCleanupID struct {
	mu       sync.Mutex
	id       string
	cleaning bool
}

func (c *liveCleanupID) current() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

func (c *liveCleanupID) clear() {
	c.mu.Lock()
	c.id = ""
	c.mu.Unlock()
}

func (c *liveCleanupID) begin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cleaning {
		return ""
	}
	id := c.id
	if id != "" {
		c.cleaning = true
	}
	return id
}

func (c *liveCleanupID) finish(id string, clear bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if clear && c.id == id {
		c.id = ""
	}
	c.cleaning = false
}

func runLiveCleanupOnce(state *liveCleanupID, remove func(string) error) error {
	id := state.begin()
	if id == "" {
		return nil
	}
	err := remove(id)
	state.finish(id, err == nil || client.IsNotFound(err))
	if client.IsNotFound(err) {
		return nil
	}
	return err
}

func (target liveBackupTarget) cleanup(t *testing.T) {
	t.Helper()
	if target.id == "" {
		target.lease.releaseIfNeeded(t)
		return
	}
	ctx, cancel := cleanupContext()
	defer cancel()
	if err := target.remove(ctx); err != nil && !client.IsNotFound(err) {
		reportLiveCleanup(t, target.resource, target.id, err)
		return
	}
	if err := waitForDatabaseAbsence(ctx, target.readID); err != nil {
		reportLiveCleanup(t, target.resource, target.id, err)
		return
	}
	target.lease.releaseIfNeeded(t)
}

func cleanupLiveResource(t *testing.T, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) {
	t.Helper()
	ctx, cancel := cleanupContext()
	defer cancel()
	if err := remove(ctx); err != nil && !client.IsNotFound(err) {
		reportLiveCleanup(t, kind, id, err)
		return
	}
	if err := waitForDatabaseAbsence(ctx, read); err != nil {
		reportLiveCleanup(t, kind, id, err)
	}
}

func liveBackupForPostgres(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	r := Postgres{client: fixedClient(api)}
	lease := beginLiveHeavyOperation(t, "backup-postgres")
	created, err := r.Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: PostgresArgs{Name: liveRunName("backup-postgres"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18"}})
	target := liveBackupTarget{id: created.ID, lease: lease, resource: "postgres", remove: func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[PostgresState]{ID: created.ID})
		return e
	}, readID: func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: created.ID})
		return v.ID, e
	}}
	handleLiveHeavyCreateError(t, lease, created.ID, err, func() { target.cleanup(t) })
	t.Cleanup(func() { target.cleanup(t) })
	requireNoError(t, err)
	require.Equal(t, statusDone, created.Output.Status)
	liveBackupCRUD(t, ctx, Backup{client: fixedClient(api)}, BackupArgs{Schedule: "0 0 * * *", Enabled: false, Prefix: "live-test-", DestinationID: destinationID, Database: "app", PostgresID: &created.ID}, created.ID, "postgresId", target)
}

func liveBackupForMySQL(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	r := MySQL{client: fixedClient(api)}
	root := "live-test-root-password"
	lease := beginLiveHeavyOperation(t, "backup-mysql")
	created, err := r.Create(ctx, infer.CreateRequest[MySQLArgs]{Inputs: MySQLArgs{Name: liveRunName("backup-mysql"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DatabaseRootPassword: &root, DockerImage: "mysql:8"}})
	target := liveBackupTarget{id: created.ID, lease: lease, resource: "mysql", remove: func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[MySQLState]{ID: created.ID})
		return e
	}, readID: func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: created.ID})
		return v.ID, e
	}}
	handleLiveHeavyCreateError(t, lease, created.ID, err, func() { target.cleanup(t) })
	t.Cleanup(func() { target.cleanup(t) })
	requireNoError(t, err)
	require.Equal(t, statusDone, created.Output.Status)
	liveBackupCRUD(t, ctx, Backup{client: fixedClient(api)}, BackupArgs{Schedule: "0 0 * * *", Enabled: false, Prefix: "live-test-", DestinationID: destinationID, Database: "app", MySQLID: &created.ID}, created.ID, "mysqlId", target)
}

func liveBackupForMariaDB(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	r := MariaDB{client: fixedClient(api)}
	lease := beginLiveHeavyOperation(t, "backup-mariadb")
	created, err := r.Create(ctx, infer.CreateRequest[MariaDBArgs]{Inputs: MariaDBArgs{Name: liveRunName("backup-mariadb"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mariadb:11"}})
	target := liveBackupTarget{id: created.ID, lease: lease, resource: "mariadb", remove: func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[MariaDBState]{ID: created.ID})
		return e
	}, readID: func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: created.ID})
		return v.ID, e
	}}
	handleLiveHeavyCreateError(t, lease, created.ID, err, func() { target.cleanup(t) })
	t.Cleanup(func() { target.cleanup(t) })
	requireNoError(t, err)
	require.Equal(t, statusDone, created.Output.Status)
	liveBackupCRUD(t, ctx, Backup{client: fixedClient(api)}, BackupArgs{Schedule: "0 0 * * *", Enabled: false, Prefix: "live-test-", DestinationID: destinationID, Database: "app", MariaDBID: &created.ID}, created.ID, "mariadbId", target)
}

func liveBackupForMongoDB(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	r := MongoDB{client: fixedClient(api)}
	replicas := false
	lease := beginLiveHeavyOperation(t, "backup-mongodb")
	created, err := r.Create(ctx, infer.CreateRequest[MongoDBArgs]{Inputs: MongoDBArgs{Name: liveRunName("backup-mongodb"), EnvironmentID: environmentID, DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mongo:8", ReplicaSets: &replicas}})
	target := liveBackupTarget{id: created.ID, lease: lease, resource: "mongodb", remove: func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[MongoDBState]{ID: created.ID})
		return e
	}, readID: func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: created.ID})
		return v.ID, e
	}}
	handleLiveHeavyCreateError(t, lease, created.ID, err, func() { target.cleanup(t) })
	t.Cleanup(func() { target.cleanup(t) })
	requireNoError(t, err)
	require.Equal(t, statusDone, created.Output.Status)
	liveBackupCRUD(t, ctx, Backup{client: fixedClient(api)}, BackupArgs{Schedule: "0 0 * * *", Enabled: false, Prefix: "live-test-", DestinationID: destinationID, Database: "app", MongoID: &created.ID}, created.ID, "mongoId", target)
}

func liveBackupCRUD(t *testing.T, ctx context.Context, r Backup, inputs BackupArgs, targetID, replacementField string, target liveBackupTarget) {
	t.Helper()
	created, err := r.Create(ctx, infer.CreateRequest[BackupArgs]{Inputs: inputs})
	backupID := created.ID
	cleanupID := &liveCleanupID{id: backupID}
	if backupID != "" {
		t.Cleanup(func() {
			id := cleanupID.current()
			if id == "" {
				return
			}
			liveCleanupVerified(t, "backup", id, func(c context.Context) error {
				return runLiveCleanupOnce(cleanupID, func(id string) error {
					_, e := r.Delete(c, infer.DeleteRequest[BackupState]{ID: id})
					return e
				})
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[BackupArgs, BackupState]{ID: id})
				return v.ID, e
			})
		})
	}
	requireNoError(t, err)
	require.NotEmpty(t, backupID)
	read, err := r.Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: backupID})
	requireNoError(t, err)
	require.False(t, read.Inputs.Enabled)
	updated := read.Inputs
	updated.Schedule = "0 1 * * *"
	updated.Prefix = "updated-"
	updated.KeepLatestCount = intPtr(3)
	updated.Enabled = false
	changed, err := r.Update(ctx, infer.UpdateRequest[BackupArgs, BackupState]{ID: backupID, Inputs: updated, State: read.State})
	requireNoError(t, err)
	post, err := r.Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: backupID, State: changed.Output})
	requireNoError(t, err)
	require.Equal(t, updated.Schedule, post.Inputs.Schedule)
	require.Equal(t, updated.Prefix, post.Inputs.Prefix)
	require.NotNil(t, post.Inputs.KeepLatestCount, "Backup post-update read omitted keepLatestCount")
	require.Equal(t, 3, *post.Inputs.KeepLatestCount, "Backup post-update keepLatestCount changed unexpectedly")
	require.False(t, post.Inputs.Enabled)
	imported, err := r.Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: backupID})
	requireNoError(t, err)
	require.Equal(t, backupID, imported.State.BackupID)
	require.NotNil(t, imported.Inputs.KeepLatestCount, "Backup ID-only import omitted keepLatestCount")
	require.Equal(t, 3, *imported.Inputs.KeepLatestCount, "Backup ID-only import keepLatestCount changed unexpectedly")
	require.False(t, imported.Inputs.Enabled)
	diff, err := r.Diff(ctx, infer.DiffRequest[BackupArgs, BackupState]{Inputs: post.Inputs, State: post.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := post.Inputs
	replacement.PostgresID, replacement.MySQLID, replacement.MariaDBID, replacement.MongoID = nil, nil, nil, nil
	switch replacementField {
	case "postgresId":
		replacement.PostgresID = stringPtr(targetID + "-replacement")
	case "mysqlId":
		replacement.MySQLID = stringPtr(targetID + "-replacement")
	case "mariadbId":
		replacement.MariaDBID = stringPtr(targetID + "-replacement")
	case "mongoId":
		replacement.MongoID = stringPtr(targetID + "-replacement")
	}
	diff, err = r.Diff(ctx, infer.DiffRequest[BackupArgs, BackupState]{Inputs: replacement, State: post.State})
	requireNoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff[replacementField].Kind)
	require.NoError(t, func() error { _, e := r.Delete(ctx, infer.DeleteRequest[BackupState]{ID: backupID}); return e }())
	backupID = ""
	cleanupID.clear()
	gone, err := r.Read(ctx, infer.ReadRequest[BackupArgs, BackupState]{ID: created.ID})
	requireNoError(t, err)
	require.Empty(t, gone.ID)
}

func liveVolumeBackupForApplication(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	liveVolumeBackupTarget(t, ctx, api, environmentID, destinationID, true)
}
func liveVolumeBackupForCompose(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string) {
	liveVolumeBackupTarget(t, ctx, api, environmentID, destinationID, false)
}

func liveVolumeBackupTarget(t *testing.T, ctx context.Context, api *client.Client, environmentID, destinationID string, application bool) {
	var targetID string
	var remove func(context.Context) error
	var readID func(context.Context) (string, error)
	if application {
		r := Application{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ApplicationArgs]{Inputs: ApplicationArgs{Name: liveRunName("volume-target"), EnvironmentID: environmentID, Source: ApplicationSource{Type: SourceDocker, Docker: &DockerSource{Image: "nginx:1.27"}}}})
		targetID = created.ID
		remove = func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ApplicationState]{ID: targetID})
			return e
		}
		readID = func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: targetID})
			return v.ID, e
		}
		if targetID != "" {
			cleanupID := targetID
			t.Cleanup(func() { cleanupLiveResource(t, "application", cleanupID, remove, readID) })
		}
		requireNoError(t, err)
		require.NotEmpty(t, targetID, "Application prerequisite create must return an ID")
		require.Equal(t, statusDone, created.Output.Status, "Application prerequisite must reach statusDone before VolumeBackup creation")
	} else {
		r := Compose{client: fixedClient(api)}
		created, err := r.Create(ctx, infer.CreateRequest[ComposeArgs]{Inputs: ComposeArgs{Name: liveRunName("volume-compose-target"), EnvironmentID: environmentID, Source: ComposeSource{Type: ComposeSourceRaw, Raw: &RawComposeSource{ComposeFile: "services:\n  web:\n    image: nginx:1.27\n"}}}})
		targetID = created.ID
		remove = func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[ComposeState]{ID: targetID})
			return e
		}
		readID = func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[ComposeArgs, ComposeState]{ID: targetID})
			return v.ID, e
		}
		if targetID != "" {
			cleanupID := targetID
			t.Cleanup(func() { cleanupLiveResource(t, "compose", cleanupID, remove, readID) })
		}
		requireNoError(t, err)
		require.NotEmpty(t, targetID, "Compose prerequisite create must return an ID")
		require.Equal(t, statusDone, created.Output.Status, "Compose prerequisite must reach statusDone before VolumeBackup creation")
	}
	var appID, composeID, service string
	if application {
		appID = targetID
	} else {
		composeID, service = targetID, "web"
	}
	r := VolumeBackup{client: fixedClient(api)}
	inputs := VolumeBackupArgs{Name: liveRunName("volume-backup"), VolumeName: "live-test-volume", Prefix: "live-test-", DestinationID: destinationID, CronExpression: "0 0 * * *", Enabled: false, ApplicationID: optionalID(appID), ComposeID: optionalID(composeID), ServiceName: optionalID(service)}
	created, err := r.Create(ctx, infer.CreateRequest[VolumeBackupArgs]{Inputs: inputs})
	id := created.ID
	cleanupID := &liveCleanupID{id: id}
	if id != "" {
		t.Cleanup(func() {
			cleanupIDValue := cleanupID.current()
			if cleanupIDValue == "" {
				return
			}
			liveCleanupVerified(t, "volume-backup", cleanupIDValue, func(c context.Context) error {
				return runLiveCleanupOnce(cleanupID, func(id string) error {
					_, e := r.Delete(c, infer.DeleteRequest[VolumeBackupState]{ID: id})
					return e
				})
			}, func(c context.Context) (string, error) {
				v, e := r.Read(c, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: cleanupIDValue})
				return v.ID, e
			})
		})
	}
	requireNoError(t, err)
	require.NotEmpty(t, id)
	read, err := r.Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: id})
	requireNoError(t, err)
	require.False(t, read.Inputs.Enabled)
	updated := read.Inputs
	updated.CronExpression = "0 1 * * *"
	updated.Prefix = "updated-"
	updated.KeepLatestCount = intPtr(3)
	updated.Enabled = false
	changed, err := r.Update(ctx, infer.UpdateRequest[VolumeBackupArgs, VolumeBackupState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	post, err := r.Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	require.Equal(t, updated.CronExpression, post.Inputs.CronExpression)
	require.Equal(t, updated.Prefix, post.Inputs.Prefix)
	require.NotNil(t, post.Inputs.KeepLatestCount)
	require.Equal(t, 3, *post.Inputs.KeepLatestCount)
	require.False(t, post.Inputs.Enabled)
	imported, err := r.Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: id})
	requireNoError(t, err)
	require.Equal(t, id, imported.State.VolumeBackupID)
	require.NotNil(t, imported.Inputs.KeepLatestCount)
	require.Equal(t, 3, *imported.Inputs.KeepLatestCount)
	diff, err := r.Diff(ctx, infer.DiffRequest[VolumeBackupArgs, VolumeBackupState]{Inputs: post.Inputs, State: post.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := post.Inputs
	replacement.ApplicationID, replacement.ComposeID = nil, nil
	if application {
		replacement.ComposeID = stringPtr(targetID + "-replacement")
		replacement.ServiceName = stringPtr("web")
	} else {
		replacement.ApplicationID = stringPtr(targetID + "-replacement")
		replacement.ServiceName = nil
	}
	diff, err = r.Diff(ctx, infer.DiffRequest[VolumeBackupArgs, VolumeBackupState]{Inputs: replacement, State: post.State})
	requireNoError(t, err)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["applicationId"].Kind)
	require.Equal(t, p.UpdateReplace, diff.DetailedDiff["composeId"].Kind)
	require.NoError(t, func() error { _, e := r.Delete(ctx, infer.DeleteRequest[VolumeBackupState]{ID: id}); return e }())
	cleanupID.clear()
	gone, err := r.Read(ctx, infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]{ID: id})
	requireNoError(t, err)
	require.Empty(t, gone.ID)
}

func optionalID(value string) *string {
	if value == "" {
		return nil
	}
	return stringPtr(value)
}
