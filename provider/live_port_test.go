package dokploy

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// The parent must consult this guard AFTER the Port subtest's cleanup and
// BEFORE deleting its project. Unknown identities and unresolved absence keep
// the bare application/project intact rather than cascading deletion.
type livePortCleanupGuard struct{ pending bool }

func (g *livePortCleanupGuard) retainProjectIfNeeded(release func()) {
	if g.pending {
		release()
	}
}

func runLivePortLifecycle(t *testing.T, ctx context.Context, api *client.Client, environmentID string) (*livePortCleanupGuard, error) {
	t.Helper()
	guard := &livePortCleanupGuard{}
	if strings.TrimSpace(environmentID) == "" {
		return guard, liveRecordLifecycleError("port", "missing-environment", nil)
	}
	guard.pending = true
	// Application.Create deploys, so use ONLY the generated bare-record create.
	fixture, createErr := api.ApplicationCreateWithResponse(ctx, generated.ApplicationCreateJSONRequestBody{
		Name: liveRunName("port-application"), EnvironmentId: environmentID,
	})
	applicationID := ""
	if fixture != nil && fixture.JSON200 != nil && fixture.JSON200.ApplicationId != nil {
		applicationID = *fixture.JSON200.ApplicationId
	}
	if strings.TrimSpace(applicationID) == "" {
		if liveRecordCreateDefinitelyRejected(createErr) {
			guard.pending = false
			return guard, liveRecordLifecycleError("port", "fixture-create-rejected", createErr)
		}
		recordStructuralCleanup("application", errors.New("bare fixture create ownership uncertain"))
		return guard, liveRecordLifecycleError("port", "fixture-create-ownership", createErr)
	}
	app := Application{client: fixedClient(api)}
	removeApp := func(c context.Context) error {
		_, e := app.Delete(c, infer.DeleteRequest[ApplicationState]{ID: applicationID})
		return e
	}
	readAppAbsent := func(c context.Context) (string, error) {
		v, e := app.Read(c, infer.ReadRequest[ApplicationArgs, ApplicationState]{ID: applicationID})
		return v.ID, e
	}
	portResolved := true
	appOwner := newLiveCleanupOwner(func() {
		if !portResolved {
			return // Preserve the owner of an uncertain or remaining mapping.
		}
		c, cancel := cleanupContext()
		defer cancel()
		if e := verifyLiveCleanup(c, removeApp, readAppAbsent); e != nil {
			reportLiveCleanup(t, "application", applicationID, e)
			return
		}
		guard.pending = false
	})
	t.Cleanup(appOwner.cleanupOnce)
	if createErr != nil {
		return guard, liveRecordLifecycleError("port", "fixture-create", createErr)
	}
	r := Port{client: fixedClient(api)}
	args := PortArgs{ApplicationID: applicationID, PublishedPort: 18080, TargetPort: 8080}
	portResolved = false
	created, err := r.Create(ctx, infer.CreateRequest[PortArgs]{Inputs: args})
	if strings.TrimSpace(created.ID) == "" {
		if liveRecordCreateDefinitelyRejected(err) {
			portResolved = true
			return guard, liveRecordLifecycleError("port", "create-rejected", err)
		}
		recordStructuralCleanup("port", errors.New("create ownership uncertain"))
		return guard, liveRecordLifecycleError("port", "create-ownership", err)
	}
	remove := func(c context.Context) error {
		_, e := r.Delete(c, infer.DeleteRequest[PortState]{ID: created.ID, State: created.Output})
		return e
	}
	readAbsent := func(c context.Context) (string, error) {
		v, e := r.Read(c, infer.ReadRequest[PortArgs, PortState]{ID: created.ID, State: created.Output})
		return v.ID, e
	}
	// Register before inspecting the partial-state error. LIFO cleanup removes
	// and verifies the mapping before considering its bare application.
	portOwner := newLiveCleanupOwner(func() {
		c, cancel := cleanupContext()
		defer cancel()
		if e := verifyLiveCleanup(c, remove, readAbsent); e != nil {
			reportLiveCleanup(t, "port", created.ID, e)
			return
		}
		portResolved = true
	})
	t.Cleanup(portOwner.cleanupOnce)
	if err != nil {
		return guard, liveRecordLifecycleError("port", "create", err)
	}
	read, err := r.Read(ctx, infer.ReadRequest[PortArgs, PortState]{ID: created.ID, State: created.Output})
	expected := PortArgs{ApplicationID: applicationID, PublishedPort: 18080, TargetPort: 8080, Protocol: ptr(PortProtocolTCP), PublishMode: ptr(PortPublishModeIngress)}
	if err != nil || read.ID != created.ID || read.State.PortID != created.ID || !reflect.DeepEqual(expected, read.Inputs) {
		return guard, liveRecordLifecycleError("port", "default-read", err)
	}
	updated := read.Inputs
	updated.PublishedPort, updated.TargetPort = 18081, 8081
	updated.Protocol, updated.PublishMode = ptr(PortProtocolUDP), ptr(PortPublishModeHost)
	changed, err := r.Update(ctx, infer.UpdateRequest[PortArgs, PortState]{ID: created.ID, Inputs: updated, State: read.State})
	if err != nil {
		return guard, liveRecordLifecycleError("port", "update", err)
	}
	postUpdate, err := r.Read(ctx, infer.ReadRequest[PortArgs, PortState]{ID: created.ID, State: changed.Output})
	if err != nil || postUpdate.ID != created.ID || !reflect.DeepEqual(updated, postUpdate.Inputs) {
		return guard, liveRecordLifecycleError("port", "update-read", err)
	}
	imported, err := r.Read(ctx, infer.ReadRequest[PortArgs, PortState]{ID: created.ID})
	if err != nil || imported.ID != created.ID || imported.State.PortID != created.ID || !reflect.DeepEqual(updated, imported.Inputs) {
		return guard, liveRecordLifecycleError("port", "import", err)
	}
	diff, err := r.Diff(ctx, infer.DiffRequest[PortArgs, PortState]{ID: created.ID, Inputs: imported.Inputs, State: imported.State})
	if err != nil || diff.HasChanges || len(diff.DetailedDiff) != 0 {
		return guard, liveRecordLifecycleError("port", "no-op-diff", err)
	}
	replacement := imported.Inputs
	// Diff only: no second application or owner mutation is needed.
	replacement.ApplicationID = applicationID + "-replacement"
	diff, err = r.Diff(ctx, infer.DiffRequest[PortArgs, PortState]{ID: created.ID, Inputs: replacement, State: imported.State})
	if err != nil || !diff.HasChanges || len(diff.DetailedDiff) != 1 || diff.DetailedDiff["applicationId"].Kind != p.UpdateReplace {
		return guard, liveRecordLifecycleError("port", "owner-replacement-diff", err)
	}
	cleanupCtx, cancel := cleanupContext()
	defer cancel()
	err = deleteAndVerifyLiveOwned(cleanupCtx, func() error { return remove(cleanupCtx) }, func() (string, error) { return readAbsent(cleanupCtx) }, func() {
		portResolved = true
		portOwner.release()
	})
	if err != nil {
		return guard, liveRecordLifecycleError("port", "delete", err)
	}
	if err = verifyLiveCleanup(cleanupCtx, remove, readAbsent); err != nil {
		return guard, liveRecordLifecycleError("port", "repeat-delete", err)
	}
	if err = verifyLiveCleanup(cleanupCtx, removeApp, readAppAbsent); err != nil {
		return guard, liveRecordLifecycleError("port", "fixture-delete", err)
	}
	guard.pending = false
	appOwner.release()
	return guard, nil
}
