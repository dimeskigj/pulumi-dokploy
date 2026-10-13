package dokploy

import (
	"net/http"
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

// The dispatch fixture must let Dokploy provision its source. A random bind
// source on the test runner does not exist on the Docker host and causes Swarm
// task rejection or rollback, even when postgres.deploy returns success.
func TestPostgresDispatchMountProvisionsFileThroughLifecycle(t *testing.T) {
	inputs := livePostgresDispatchMount("p1")
	mount := `{"mountId":"m1","type":"file","mountPath":"/mnt/postgres","filePath":"pulumi-postgres-live.conf","content":"LIVE_MOUNT=1","serviceType":"postgres","postgresId":"p1"}`
	updatedMount := `{"mountId":"m1","type":"file","mountPath":"/mnt/postgres-updated","filePath":"pulumi-postgres-live.conf","content":"LIVE_MOUNT=1","serviceType":"postgres","postgresId":"p1"}`
	status := `{"postgresId":"p1","applicationStatus":"done"}`
	s := newScriptedServer(t,
		expectPOST("/api/mounts.create", `{"type":"file","mountPath":"/mnt/postgres","filePath":"pulumi-postgres-live.conf","content":"LIVE_MOUNT=1","hostPath":null,"volumeName":null,"serviceType":"postgres","serviceId":"p1"}`, `{"mountId":"m1"}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, mount),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `{}`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectPOST("/api/mounts.update", `{"mountId":"m1","type":"file","mountPath":"/mnt/postgres-updated","filePath":"pulumi-postgres-live.conf","content":"LIVE_MOUNT=1","hostPath":null,"volumeName":null,"serviceType":"postgres","postgresId":"p1","applicationId":null,"composeId":null,"mysqlId":null,"mariadbId":null,"redisId":null}`, `{}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, updatedMount),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `{}`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, updatedMount),
		expectPOST("/api/mounts.remove", `{"mountId":"m1"}`, `{}`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectPOST("/api/postgres.deploy", `{"postgresId":"p1"}`, `{}`),
		expectGET("/api/postgres.one", map[string][]string{"postgresId": {"p1"}}, http.StatusOK, status),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusNotFound, `{"code":"NOT_FOUND"}`),
	)
	r := Mount{client: fixedClient(s.API())}
	created, err := r.Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: inputs})
	require.NoError(t, err)
	require.Equal(t, "m1", created.ID)
	require.Nil(t, created.Output.HostPath)
	require.Equal(t, "p1", value(created.Output.PostgresID))

	updated := created.Output.MountArgs
	updated.MountPath = "/mnt/postgres-updated"
	changed, err := r.Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: created.ID, Inputs: updated, State: created.Output})
	require.NoError(t, err)
	require.Equal(t, "/mnt/postgres-updated", changed.Output.MountPath)
	require.Equal(t, "LIVE_MOUNT=1", value(changed.Output.Content))

	_, err = r.Delete(t.Context(), infer.DeleteRequest[MountState]{ID: created.ID, State: changed.Output})
	require.NoError(t, err)
	gone, err := r.Read(t.Context(), infer.ReadRequest[MountArgs, MountState]{ID: created.ID})
	require.NoError(t, err)
	require.Empty(t, gone.ID)
}
