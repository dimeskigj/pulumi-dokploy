package dokploy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Returns only structural failures so offline failure tests can exercise the
// same helper without Fatal preventing them from inspecting fallback cleanup.
func runLiveServerLifecycle(t *testing.T, ctx context.Context, api *client.Client, lookups ...func(string, string)) error {
	t.Helper()
	r := Server{client: fixedClient(api)}
	args := ServerArgs{
		Name: liveRunName("server"), Description: stringPtr("disposable acceptance record"),
		IPAddress: liveDomainHost("server"), Port: 22, Username: "root", ServerType: serverTypeDeploy,
		EnableDockerCleanup: false,
	}
	created, err := r.Create(ctx, infer.CreateRequest[ServerArgs]{Inputs: args})
	if strings.TrimSpace(created.ID) == "" {
		if liveRecordCreateDefinitelyRejected(err) {
			return liveRecordLifecycleError("server", "create-rejected", err)
		}
		recordStructuralCleanup("server", errors.New("create ownership uncertain"))
		return liveRecordLifecycleError("server", "create-ownership", err)
	}
	remove := func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[ServerState]{ID: created.ID})
		return e
	}
	readAbsent := func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[ServerArgs, ServerState]{ID: created.ID})
		return v.ID, e
	}
	// An acknowledged partial create is owned even when readback failed.
	release := registerLiveCleanup(t, "server", created.ID, remove, readAbsent)
	if err != nil {
		return liveRecordLifecycleError("server", "create", err)
	}
	read, err := r.Read(ctx, infer.ReadRequest[ServerArgs, ServerState]{ID: created.ID, State: created.Output})
	if err != nil || read.ID != created.ID || read.State.ServerID != created.ID || !reflect.DeepEqual(args, read.Inputs) {
		return liveRecordLifecycleError("server", "read", err)
	}
	updated := read.Inputs
	updated.Name += "-updated"
	updated.Description = stringPtr("updated disposable acceptance record")
	updated.IPAddress, updated.Port, updated.Username, updated.ServerType = liveDomainHost("server-updated"), 2222, "acceptance", serverTypeBuild
	updated.EnableDockerCleanup = false
	changed, err := r.Update(ctx, infer.UpdateRequest[ServerArgs, ServerState]{ID: created.ID, Inputs: updated, State: read.State})
	if err != nil {
		return liveRecordLifecycleError("server", "update", err)
	}
	postUpdate, err := r.Read(ctx, infer.ReadRequest[ServerArgs, ServerState]{ID: created.ID, State: changed.Output})
	if err != nil || postUpdate.ID != created.ID || !reflect.DeepEqual(updated, postUpdate.Inputs) {
		return liveRecordLifecycleError("server", "update-read", err)
	}
	// Omission explicitly clears nullable metadata; never turn on cleanup.
	updated.Description = nil
	cleared, err := r.Update(ctx, infer.UpdateRequest[ServerArgs, ServerState]{ID: created.ID, Inputs: updated, State: postUpdate.State})
	if err != nil || !reflect.DeepEqual(updated, cleared.Output.ServerArgs) {
		return liveRecordLifecycleError("server", "clear-description", err)
	}
	imported, err := r.Read(ctx, infer.ReadRequest[ServerArgs, ServerState]{ID: created.ID})
	if err != nil || imported.ID != created.ID || imported.State.ServerID != created.ID || !reflect.DeepEqual(updated, imported.Inputs) {
		return liveRecordLifecycleError("server", "import", err)
	}
	for _, lookup := range lookups {
		lookup(created.ID, imported.Inputs.Name)
	}
	diff, err := r.Diff(ctx, infer.DiffRequest[ServerArgs, ServerState]{ID: created.ID, Inputs: imported.Inputs, State: imported.State})
	if err != nil || diff.HasChanges || len(diff.DetailedDiff) != 0 {
		return liveRecordLifecycleError("server", "no-op-diff", err)
	}
	cleanupCtx, cancel := cleanupContext()
	defer cancel()
	err = deleteAndVerifyLiveOwned(cleanupCtx, func() error { return remove(cleanupCtx) }, func() (string, error) { return readAbsent(cleanupCtx) }, release)
	if err != nil {
		return liveRecordLifecycleError("server", "delete", err)
	}
	if err = verifyLiveCleanup(cleanupCtx, remove, readAbsent); err != nil {
		return liveRecordLifecycleError("server", "repeat-delete", err)
	}
	return nil
}

func liveRecordLifecycleError(kind, phase string, err error) error {
	return fmt.Errorf("phase=%s;%s", phase, structuralLiveDiagnostic("lifecycle", kind, err))
}

// Only a definite pre-mutation API rejection rules out an owned record. Rate
// limits, server/transport failures, and missing successful acknowledgments do
// not establish absence and must retain the uncertainty stop behavior.
func liveRecordCreateDefinitelyRejected(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429
}
