package dokploy

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// TestLiveTier3Databases deliberately keeps every database lifecycle serial.
// Dokploy deploys a database as a container and a small acceptance server must
// never be asked to build more than one database at once.
func TestLiveTier3Databases(t *testing.T) {
	api := liveClient(t)
	ctx := liveContext(t, 50*time.Minute)
	_, environmentID, _ := liveProject(t, ctx, api)

	runLiveCase(t, "Postgres", func(t *testing.T) { livePostgresLifecycle(t, ctx, api, environmentID) })
	runLiveCase(t, "MySQL", func(t *testing.T) { liveMySQLLifecycle(t, ctx, api, environmentID) })
	runLiveCase(t, "MariaDB", func(t *testing.T) { liveMariaDBLifecycle(t, ctx, api, environmentID) })
	runLiveCase(t, "MongoDBReplicaSets", func(t *testing.T) {
		if os.Getenv("DOKPLOY_ACCEPTANCE_ALLOW_REPLICAS") != "1" {
			t.Skip("low-powered acceptance server: MongoDB replica sets require multiple containers (set DOKPLOY_ACCEPTANCE_ALLOW_REPLICAS=1 to opt in)")
		}
		liveMongoDBLifecycle(t, ctx, api, environmentID, true)
	})
	runLiveCase(t, "MongoDB", func(t *testing.T) { liveMongoDBLifecycle(t, ctx, api, environmentID, false) })
	runLiveCase(t, "Redis", func(t *testing.T) { liveRedisLifecycle(t, ctx, api, environmentID) })
}

func waitForDatabaseAbsence(ctx context.Context, read func(context.Context) (string, error)) error {
	for {
		id, err := read(ctx)
		if err != nil {
			if client.IsNotFound(err) {
				return nil
			}
			return err
		}
		if id == "" {
			return nil
		}
		timer := time.NewTimer(liveCleanupPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func finishDatabaseCleanup(t *testing.T, lease *liveHeavyOperationLease, kind, id string, remove func(context.Context) error, read func(context.Context) (string, error)) bool {
	t.Helper()
	if id == "" {
		return false
	}
	ctx, cancel := cleanupContext()
	defer cancel()
	if err := remove(ctx); err != nil && !client.IsNotFound(err) {
		reportLiveCleanup(t, kind, id, err)
		return false
	}
	if err := waitForDatabaseAbsence(ctx, read); err != nil {
		reportLiveCleanup(t, kind, id, err)
		return false
	}
	if lease.holdForFollowUp {
		lease.release(t)
		return false
	}
	return lease.release(t)
}

func livePostgresLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string) {
	r := Postgres{client: fixedClient(api)}
	inputs := PostgresArgs{Name: liveRunName("postgres"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "postgres:18", Environment: stringPtr("LIVE=1")}
	t.Cleanup(registerLiveSecrets(inputs.DatabasePassword, value(inputs.Environment)))
	lease := beginLiveHeavyOperation(t, "postgres", liveServerHealthProbe(api))
	id := ""
	defer func() {
		if id == "" {
			lease.releaseIfNeeded(t)
		}
	}()
	created, err := r.Create(ctx, infer.CreateRequest[PostgresArgs]{Inputs: inputs})
	id = created.ID
	handleLiveHeavyCreateError(t, lease, id, err, func() {
		lease.released = finishDatabaseCleanup(t, lease, "postgres", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[PostgresState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, err)
	requireLiveEqual(t, "postgres.status", statusDone, created.Output.Status)
	t.Cleanup(func() {
		finishDatabaseCleanup(t, lease, "postgres", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[PostgresState]{ID: id})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
			return v.ID, e
		})
	})
	read, err := r.Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id, State: created.Output})
	requireNoError(t, err)
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated by database live test")
	updated.Environment = stringPtr("LIVE=2")
	changed, err := r.Update(ctx, infer.UpdateRequest[PostgresArgs, PostgresState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "postgres.environment", "LIVE=2", value(postUpdate.Inputs.Environment))
	requireLiveEqual(t, "postgres.name", updated.Name, postUpdate.Inputs.Name)
	requireLiveEqual(t, "postgres.description", value(updated.Description), value(postUpdate.Inputs.Description))
	t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Postgres", id, postUpdate.Inputs.Name) })
	imported, err := r.Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "postgres.id", id, imported.State.PostgresID)
	requireLiveEqual(t, "postgres.environment import", "LIVE=2", value(imported.Inputs.Environment))
	requireLiveEqual(t, "postgres.name import", postUpdate.Inputs.Name, imported.Inputs.Name)
	requireLiveEqual(t, "postgres.description import", value(postUpdate.Inputs.Description), value(imported.Inputs.Description))
	diff, err := r.Diff(ctx, infer.DiffRequest[PostgresArgs, PostgresState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := postUpdate.Inputs
	replacement.EnvironmentID = environmentID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[PostgresArgs, PostgresState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "environmentId")
	replacement = postUpdate.Inputs
	replacement.ServerID = stringPtr("server-replacement")
	diff, err = r.Diff(ctx, infer.DiffRequest[PostgresArgs, PostgresState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "serverId")
	_, err = r.Delete(ctx, infer.DeleteRequest[PostgresState]{ID: id})
	requireNoError(t, err)
	gone, err := r.Read(ctx, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "postgres.id after delete", "", gone.ID)
	require.NoError(t, waitForDatabaseAbsence(ctx, func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[PostgresArgs, PostgresState]{ID: id})
		return v.ID, e
	}))
	require.True(t, lease.release(t))
	id = ""
}

func requireDatabaseReplacementDiff(t *testing.T, response infer.DiffResponse, err error, field string) {
	t.Helper()
	requireNoError(t, err)
	require.Equal(t, p.UpdateReplace, response.DetailedDiff[field].Kind)
}

func liveMySQLLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string) {
	r := MySQL{client: fixedClient(api)}
	root := "live-test-root-password"
	inputs := MySQLArgs{Name: liveRunName("mysql"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DatabaseRootPassword: &root, DockerImage: "mysql:8", Environment: stringPtr("LIVE=1")}
	t.Cleanup(registerLiveSecrets(inputs.DatabasePassword, *inputs.DatabaseRootPassword, value(inputs.Environment)))
	lease := beginLiveHeavyOperation(t, "mysql", liveServerHealthProbe(api))
	id := ""
	defer func() {
		if id == "" {
			lease.releaseIfNeeded(t)
		}
	}()
	created, err := r.Create(ctx, infer.CreateRequest[MySQLArgs]{Inputs: inputs})
	id = created.ID
	handleLiveHeavyCreateError(t, lease, id, err, func() {
		lease.released = finishDatabaseCleanup(t, lease, "mysql", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MySQLState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, err)
	requireLiveEqual(t, "mysql.status", statusDone, created.Output.Status)
	t.Cleanup(func() {
		finishDatabaseCleanup(t, lease, "mysql", id, func(c context.Context) error { _, e := r.Delete(c, infer.DeleteRequest[MySQLState]{ID: id}); return e }, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
			return v.ID, e
		})
	})
	read, err := r.Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id, State: created.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "mysql.databaseRootPassword", root, value(read.Inputs.DatabaseRootPassword))
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated by database live test")
	updated.Environment = stringPtr("LIVE=2")
	changed, err := r.Update(ctx, infer.UpdateRequest[MySQLArgs, MySQLState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "mysql.environment", "LIVE=2", value(postUpdate.Inputs.Environment))
	requireLiveEqual(t, "mysql.name", updated.Name, postUpdate.Inputs.Name)
	requireLiveEqual(t, "mysql.description", value(updated.Description), value(postUpdate.Inputs.Description))
	t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "MySQL", id, postUpdate.Inputs.Name) })
	imported, err := r.Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mysql.id", id, imported.State.MySQLID)
	requireLiveEqual(t, "mysql.environment import", "LIVE=2", value(imported.Inputs.Environment))
	requireLiveEqual(t, "mysql.name import", postUpdate.Inputs.Name, imported.Inputs.Name)
	requireLiveEqual(t, "mysql.description import", value(postUpdate.Inputs.Description), value(imported.Inputs.Description))
	requireLiveEqual(t, "mysql.databaseRootPassword", root, value(imported.Inputs.DatabaseRootPassword))
	diff, err := r.Diff(ctx, infer.DiffRequest[MySQLArgs, MySQLState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := postUpdate.Inputs
	replacement.EnvironmentID = environmentID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[MySQLArgs, MySQLState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "environmentId")
	replacement = postUpdate.Inputs
	replacement.ServerID = stringPtr("server-replacement")
	diff, err = r.Diff(ctx, infer.DiffRequest[MySQLArgs, MySQLState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "serverId")
	_, err = r.Delete(ctx, infer.DeleteRequest[MySQLState]{ID: id})
	requireNoError(t, err)
	gone, err := r.Read(ctx, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mysql.id after delete", "", gone.ID)
	require.NoError(t, waitForDatabaseAbsence(ctx, func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MySQLArgs, MySQLState]{ID: id})
		return v.ID, e
	}))
	require.True(t, lease.release(t))
	id = ""
}

func liveMariaDBLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string) {
	r := MariaDB{client: fixedClient(api)}
	inputs := MariaDBArgs{Name: liveRunName("mariadb"), EnvironmentID: environmentID, DatabaseName: "app", DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mariadb:11", Environment: stringPtr("LIVE=1")}
	t.Cleanup(registerLiveSecrets(inputs.DatabasePassword, value(inputs.Environment)))
	lease := beginLiveHeavyOperation(t, "mariadb", liveServerHealthProbe(api))
	id := ""
	defer func() {
		if id == "" {
			lease.releaseIfNeeded(t)
		}
	}()
	created, err := r.Create(ctx, infer.CreateRequest[MariaDBArgs]{Inputs: inputs})
	id = created.ID
	handleLiveHeavyCreateError(t, lease, id, err, func() {
		lease.released = finishDatabaseCleanup(t, lease, "mariadb", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MariaDBState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, err)
	requireLiveEqual(t, "mariadb.status", statusDone, created.Output.Status)
	t.Cleanup(func() {
		finishDatabaseCleanup(t, lease, "mariadb", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MariaDBState]{ID: id})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
			return v.ID, e
		})
	})
	read, err := r.Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id, State: created.Output})
	requireNoError(t, err)
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated by database live test")
	updated.Environment = stringPtr("LIVE=2")
	changed, err := r.Update(ctx, infer.UpdateRequest[MariaDBArgs, MariaDBState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "mariadb.environment", "LIVE=2", value(postUpdate.Inputs.Environment))
	requireLiveEqual(t, "mariadb.name", updated.Name, postUpdate.Inputs.Name)
	requireLiveEqual(t, "mariadb.description", value(updated.Description), value(postUpdate.Inputs.Description))
	t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "MariaDB", id, postUpdate.Inputs.Name) })
	imported, err := r.Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mariadb.id", id, imported.State.MariaDBID)
	requireLiveEqual(t, "mariadb.environment import", "LIVE=2", value(imported.Inputs.Environment))
	requireLiveEqual(t, "mariadb.name import", postUpdate.Inputs.Name, imported.Inputs.Name)
	requireLiveEqual(t, "mariadb.description import", value(postUpdate.Inputs.Description), value(imported.Inputs.Description))
	diff, err := r.Diff(ctx, infer.DiffRequest[MariaDBArgs, MariaDBState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := postUpdate.Inputs
	replacement.EnvironmentID = environmentID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[MariaDBArgs, MariaDBState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "environmentId")
	replacement = postUpdate.Inputs
	replacement.ServerID = stringPtr("server-replacement")
	diff, err = r.Diff(ctx, infer.DiffRequest[MariaDBArgs, MariaDBState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "serverId")
	_, err = r.Delete(ctx, infer.DeleteRequest[MariaDBState]{ID: id})
	requireNoError(t, err)
	gone, err := r.Read(ctx, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mariadb.id after delete", "", gone.ID)
	require.NoError(t, waitForDatabaseAbsence(ctx, func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MariaDBArgs, MariaDBState]{ID: id})
		return v.ID, e
	}))
	require.True(t, lease.release(t))
	id = ""
}

func liveMongoDBLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string, replicaSets bool) {
	r := MongoDB{client: fixedClient(api)}
	inputs := MongoDBArgs{Name: liveRunName("mongodb"), EnvironmentID: environmentID, DatabaseUser: "app", DatabasePassword: "live-test-password", DockerImage: "mongo:8", Environment: stringPtr("LIVE=1"), ReplicaSets: &replicaSets}
	t.Cleanup(registerLiveSecrets(inputs.DatabasePassword, value(inputs.Environment)))
	lease := beginLiveHeavyOperation(t, "mongodb", liveServerHealthProbe(api))
	id := ""
	defer func() {
		if id == "" {
			lease.releaseIfNeeded(t)
		}
	}()
	created, err := r.Create(ctx, infer.CreateRequest[MongoDBArgs]{Inputs: inputs})
	id = created.ID
	handleLiveHeavyCreateError(t, lease, id, err, func() {
		lease.released = finishDatabaseCleanup(t, lease, "mongodb", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MongoDBState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, err)
	requireLiveEqual(t, "mongodb.status", statusDone, created.Output.Status)
	t.Cleanup(func() {
		finishDatabaseCleanup(t, lease, "mongodb", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[MongoDBState]{ID: id})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id})
			return v.ID, e
		})
	})
	read, err := r.Read(ctx, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id, State: created.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "mongodb.databaseUser", "app", read.Inputs.DatabaseUser)
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated by database live test")
	updated.Environment = stringPtr("LIVE=2")
	changed, err := r.Update(ctx, infer.UpdateRequest[MongoDBArgs, MongoDBState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "mongodb.environment", "LIVE=2", value(postUpdate.Inputs.Environment))
	requireLiveEqual(t, "mongodb.name", updated.Name, postUpdate.Inputs.Name)
	requireLiveEqual(t, "mongodb.description", value(updated.Description), value(postUpdate.Inputs.Description))
	t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "MongoDB", id, postUpdate.Inputs.Name) })
	imported, err := r.Read(ctx, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mongodb.id", id, imported.State.MongoDBID)
	requireLiveEqual(t, "mongodb.databaseUser import", "app", imported.Inputs.DatabaseUser)
	requireLiveEqual(t, "mongodb.environment import", "LIVE=2", value(imported.Inputs.Environment))
	requireLiveEqual(t, "mongodb.name import", postUpdate.Inputs.Name, imported.Inputs.Name)
	requireLiveEqual(t, "mongodb.description import", value(postUpdate.Inputs.Description), value(imported.Inputs.Description))
	diff, err := r.Diff(ctx, infer.DiffRequest[MongoDBArgs, MongoDBState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := postUpdate.Inputs
	replacement.EnvironmentID = environmentID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[MongoDBArgs, MongoDBState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "environmentId")
	replacement = postUpdate.Inputs
	replacement.ServerID = stringPtr("server-replacement")
	diff, err = r.Diff(ctx, infer.DiffRequest[MongoDBArgs, MongoDBState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "serverId")
	_, err = r.Delete(ctx, infer.DeleteRequest[MongoDBState]{ID: id})
	requireNoError(t, err)
	gone, err := r.Read(ctx, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "mongodb.id after delete", "", gone.ID)
	require.NoError(t, waitForDatabaseAbsence(ctx, func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[MongoDBArgs, MongoDBState]{ID: id})
		return v.ID, e
	}))
	require.True(t, lease.release(t))
	id = ""
}

func liveRedisLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string) {
	r := Redis{client: fixedClient(api)}
	inputs := RedisArgs{Name: liveRunName("redis"), EnvironmentID: environmentID, DatabasePassword: "live-test-password", DockerImage: "redis:8", Environment: stringPtr("LIVE=1")}
	t.Cleanup(registerLiveSecrets(inputs.DatabasePassword, value(inputs.Environment)))
	lease := beginLiveHeavyOperation(t, "redis", liveServerHealthProbe(api))
	id := ""
	defer func() {
		if id == "" {
			lease.releaseIfNeeded(t)
		}
	}()
	created, err := r.Create(ctx, infer.CreateRequest[RedisArgs]{Inputs: inputs})
	id = created.ID
	handleLiveHeavyCreateError(t, lease, id, err, func() {
		lease.released = finishDatabaseCleanup(t, lease, "redis", id, func(c context.Context) error {
			_, e := r.Delete(c, infer.DeleteRequest[RedisState]{ID: created.ID})
			return e
		}, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
			return v.ID, e
		})
	}, liveServerHealthProbe(api))
	requireNoError(t, err)
	requireLiveEqual(t, "redis.status", statusDone, created.Output.Status)
	t.Cleanup(func() {
		finishDatabaseCleanup(t, lease, "redis", id, func(c context.Context) error { _, e := r.Delete(c, infer.DeleteRequest[RedisState]{ID: id}); return e }, func(c context.Context) (string, error) {
			v, e := r.Read(c, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
			return v.ID, e
		})
	})
	read, err := r.Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id, State: created.Output})
	requireNoError(t, err)
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated by database live test")
	updated.Environment = stringPtr("LIVE=2")
	changed, err := r.Update(ctx, infer.UpdateRequest[RedisArgs, RedisState]{ID: id, Inputs: updated, State: read.State})
	requireNoError(t, err)
	postUpdate, err := r.Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id, State: changed.Output})
	requireNoError(t, err)
	requireLiveEqual(t, "redis.environment", "LIVE=2", value(postUpdate.Inputs.Environment))
	requireLiveEqual(t, "redis.name", updated.Name, postUpdate.Inputs.Name)
	requireLiveEqual(t, "redis.description", value(updated.Description), value(postUpdate.Inputs.Description))
	t.Run("Lookup", func(t *testing.T) { runLiveLookup(t, ctx, api, "Redis", id, postUpdate.Inputs.Name) })
	imported, err := r.Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "redis.id", id, imported.State.RedisID)
	requireLiveEqual(t, "redis.environment import", "LIVE=2", value(imported.Inputs.Environment))
	requireLiveEqual(t, "redis.name import", postUpdate.Inputs.Name, imported.Inputs.Name)
	requireLiveEqual(t, "redis.description import", value(postUpdate.Inputs.Description), value(imported.Inputs.Description))
	diff, err := r.Diff(ctx, infer.DiffRequest[RedisArgs, RedisState]{Inputs: postUpdate.Inputs, State: postUpdate.State})
	requireNoError(t, err)
	require.False(t, diff.HasChanges)
	replacement := postUpdate.Inputs
	replacement.EnvironmentID = environmentID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[RedisArgs, RedisState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "environmentId")
	replacement = postUpdate.Inputs
	replacement.ServerID = stringPtr("server-replacement")
	diff, err = r.Diff(ctx, infer.DiffRequest[RedisArgs, RedisState]{Inputs: replacement, State: postUpdate.State})
	requireDatabaseReplacementDiff(t, diff, err, "serverId")
	_, err = r.Delete(ctx, infer.DeleteRequest[RedisState]{ID: id})
	requireNoError(t, err)
	gone, err := r.Read(ctx, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
	requireNoError(t, err)
	requireLiveEqual(t, "redis.id after delete", "", gone.ID)
	require.NoError(t, waitForDatabaseAbsence(ctx, func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[RedisArgs, RedisState]{ID: id})
		return v.ID, e
	}))
	require.True(t, lease.release(t))
	id = ""
}
